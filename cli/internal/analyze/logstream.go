package analyze

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

type lineSink struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	onLine func(string)
}

func newLineSink(onLine func(string)) *lineSink {
	return &lineSink{onLine: onLine}
}

func (s *lineSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, b := range p {
		if b == '\n' {
			s.emitLineLocked(s.buf.String())
			s.buf.Reset()
			continue
		}
		s.buf.WriteByte(b)
	}
	return len(p), nil
}

func (s *lineSink) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf.Len() == 0 {
		return
	}
	s.emitLineLocked(s.buf.String())
	s.buf.Reset()
}

func (s *lineSink) emitLineLocked(raw string) {
	if s.onLine == nil {
		return
	}
	trimmedCR := strings.TrimRight(raw, "\r")
	if strings.ContainsRune(trimmedCR, '\r') {
		parts := strings.Split(trimmedCR, "\r")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := cleanLogLine(parts[i])
			if candidate != "" {
				s.onLine(candidate)
				return
			}
		}
		return
	}
	line := cleanLogLine(trimmedCR)
	if line != "" {
		s.onLine(line)
	}
}

func cleanLogLine(s string) string {
	noANSI := stripANSI(s)
	r := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ")
	return strings.TrimSpace(r.Replace(noANSI))
}

func stripANSI(s string) string {
	if !strings.ContainsRune(s, '\033') {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7E) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func tailLogFile(stop <-chan struct{}, path string, onLine func(string)) (flush func()) {
	sink := newLineSink(onLine)
	stopOwn := make(chan struct{})
	doneCh := make(chan struct{})
	var stopOnce sync.Once
	var offset int64

	readDelta := func() {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return
		}
		size := info.Size()
		if size < offset {
			offset = 0
		}
		if size == offset {
			return
		}
		f, err := os.Open(path)
		if err != nil {
			return
		}
		if offset > 0 {
			if _, sErr := f.Seek(offset, io.SeekStart); sErr != nil {
				_ = f.Close()
				return
			}
		}
		chunk, rErr := io.ReadAll(f)
		_ = f.Close()
		if len(chunk) > 0 {
			offset += int64(len(chunk))
			_, _ = sink.Write(chunk)
		}
		_ = rErr
	}

	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-stopOwn:
				return
			case <-ticker.C:
				readDelta()
			}
		}
	}()

	return func() {
		stopOnce.Do(func() {
			close(stopOwn)
			<-doneCh
			readDelta()
			sink.Flush()
		})
	}
}

type targetState int

const (
	targetQueued targetState = iota
	targetRunning
	targetOK
	targetFailed
	targetCanceled
)

var batchSpinnerFrames = [4]string{"|", "/", "-", "\\"}

type targetRowState struct {
	status     targetState
	startedAt  time.Time
	finishedAt time.Time
	lastLine   string
	stats      LiveProcessStats
	hasSample  bool
	metrics    ProcessMetrics
	maxMem     string
	detail     string
}

type batchObserver struct {
	ui                *term.UI
	engine            workspace.DecompilerID
	header            string
	heartbeatInterval time.Duration

	mu        sync.Mutex
	order     []workspace.TargetID
	states    map[workspace.TargetID]*targetRowState
	availPhys uint64
	frameIdx  int
	start     time.Time
	stopOnce  sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
}

func newBatchObserver(ui *term.UI, engine workspace.DecompilerID, header string, targets []workspace.TargetID) *batchObserver {
	order := slices.Clone(targets)
	states := make(map[workspace.TargetID]*targetRowState, len(targets))
	for _, t := range order {
		states[t] = &targetRowState{status: targetQueued}
	}
	return &batchObserver{
		ui:                ui,
		engine:            engine,
		header:            header,
		heartbeatInterval: 30 * time.Second,
		order:             order,
		states:            states,
		availPhys:         ProbeSystemMemory().AvailPhysBytes,
		stopCh:            make(chan struct{}),
		doneCh:            make(chan struct{}),
	}
}

func (o *batchObserver) Start() {
	if o == nil || o.ui == nil {
		if o != nil {
			close(o.doneCh)
		}
		return
	}
	o.mu.Lock()
	o.start = time.Now()
	animated := o.ui.CanAnimate()
	if animated {
		o.renderLocked()
	}
	interval := o.heartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	o.mu.Unlock()

	if animated {
		go func() {
			defer close(o.doneCh)
			ticker := time.NewTicker(125 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-o.stopCh:
					return
				case <-ticker.C:
					o.mu.Lock()
					o.frameIdx = (o.frameIdx + 1) % len(batchSpinnerFrames)
					o.renderLocked()
					o.mu.Unlock()
				}
			}
		}()
		return
	}

	go func() {
		defer close(o.doneCh)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-o.stopCh:
				return
			case <-ticker.C:
				o.emitHeartbeat()
			}
		}
	}()
}

func (o *batchObserver) Close() {
	if o == nil {
		return
	}
	o.stopOnce.Do(func() {
		close(o.stopCh)
		<-o.doneCh
		if o.ui != nil && o.ui.CanAnimate() {
			o.mu.Lock()
			o.renderLocked()
			o.mu.Unlock()
			o.ui.CommitLiveLines()
		}
	})
}

func (o *batchObserver) ensureTargetLocked(target workspace.TargetID) *targetRowState {
	if o.states == nil {
		o.states = make(map[workspace.TargetID]*targetRowState)
	}
	ts, ok := o.states[target]
	if !ok {
		ts = &targetRowState{status: targetQueued}
		o.states[target] = ts
		o.order = append(o.order, target)
	}
	return ts
}

func (o *batchObserver) TargetStarted(target workspace.TargetID) {
	if o == nil {
		return
	}
	o.mu.Lock()
	ts := o.ensureTargetLocked(target)
	ts.status = targetRunning
	ts.startedAt = time.Now()
	if o.ui != nil && o.ui.CanAnimate() {
		o.renderLocked()
	}
	o.mu.Unlock()
}

func (o *batchObserver) OnLine(target workspace.TargetID, line string) {
	if o == nil || line == "" {
		return
	}
	o.mu.Lock()
	ts := o.ensureTargetLocked(target)
	ts.lastLine = line
	o.mu.Unlock()
}

func (o *batchObserver) OnSample(target workspace.TargetID, stats LiveProcessStats) {
	if o == nil {
		return
	}
	o.mu.Lock()
	ts := o.ensureTargetLocked(target)
	ts.stats = stats
	ts.hasSample = true
	if stats.AvailPhysBytes > 0 {
		o.availPhys = stats.AvailPhysBytes
	}
	o.mu.Unlock()
}

func (o *batchObserver) TargetFinished(target workspace.TargetID, status targetState, metrics ProcessMetrics, maxMem, detail string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	ts := o.ensureTargetLocked(target)
	ts.status = status
	ts.finishedAt = time.Now()
	ts.metrics = metrics
	ts.maxMem = maxMem
	ts.detail = detail
	if metrics.AvailPhysAfterBytes > 0 {
		o.availPhys = metrics.AvailPhysAfterBytes
	}
	if o.ui != nil && o.ui.CanAnimate() {
		o.renderLocked()
	}
	o.mu.Unlock()
}

func (o *batchObserver) renderLocked() {
	if o.ui == nil || !o.ui.CanAnimate() {
		return
	}
	st := o.ui.ErrStyle()
	hdr := o.header
	if hdr == "" {
		hdr = fmt.Sprintf("Analyzing with %s", o.engine)
	}
	headerLine := st.Bold(hdr)
	if o.availPhys > 0 {
		headerLine += " " + st.Dim("| free RAM "+formatMemoryBytes(o.availPhys))
	}

	lines := make([]string, 0, 1+len(o.order))
	lines = append(lines, headerLine)
	now := time.Now()
	spinChar := st.Cyan(batchSpinnerFrames[o.frameIdx%len(batchSpinnerFrames)])

	for _, t := range o.order {
		ts := o.states[t]
		if ts == nil {
			continue
		}
		name := fmt.Sprintf("%-9s", t)
		switch ts.status {
		case targetQueued:
			lines = append(lines, fmt.Sprintf("  %s %s  %s", st.Dim("[..]"), name, st.Dim("queued")))
		case targetRunning:
			el := formatBatchElapsed(targetElapsed(ts, now))
			last := ts.lastLine
			if last == "" {
				last = "starting..."
			}
			if ts.hasSample && (ts.stats.CurrentMemoryBytes > 0 || ts.stats.PeakMemoryBytes > 0) {
				lines = append(lines, fmt.Sprintf(
					"  [%s]  %s  %5s | cpu %3.0f%% | ram %s (pk %s) | %s",
					spinChar,
					name,
					el,
					ts.stats.CPUPercent,
					formatMemoryBytes(ts.stats.CurrentMemoryBytes),
					formatMemoryBytes(ts.stats.PeakMemoryBytes),
					last,
				))
			} else {
				lines = append(lines, fmt.Sprintf("  [%s]  %s  %5s | %s", spinChar, name, el, last))
			}
		case targetOK:
			el := formatBatchElapsed(targetElapsed(ts, now))
			summary := formatResourceSummary(ts.metrics, ts.maxMem)
			if summary == "" {
				summary = "done"
			}
			lines = append(lines, fmt.Sprintf("  %s %s  %5s | %s", st.Green("[ok]"), name, el, summary))
		case targetFailed:
			el := formatBatchElapsed(targetElapsed(ts, now))
			detail := ts.detail
			if detail == "" {
				detail = ts.lastLine
			}
			if detail == "" {
				detail = "failed"
			}
			lines = append(lines, fmt.Sprintf("  %s %s  %5s | failed: %s", st.Red("[!!]"), name, el, detail))
		case targetCanceled:
			el := formatBatchElapsed(targetElapsed(ts, now))
			lines = append(lines, fmt.Sprintf("  %s %s  %5s | canceled", st.Yellow("[--]"), name, el))
		}
	}
	o.ui.SetLiveLines(lines)
}

func targetElapsed(ts *targetRowState, now time.Time) time.Duration {
	if ts.metrics.WallDuration > 0 && ts.status != targetRunning {
		return ts.metrics.WallDuration
	}
	if ts.startedAt.IsZero() {
		return 0
	}
	if !ts.finishedAt.IsZero() && ts.status != targetRunning {
		return ts.finishedAt.Sub(ts.startedAt)
	}
	return now.Sub(ts.startedAt)
}

func formatBatchElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int(d.Round(time.Second).Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	remSecs := secs % 60
	if mins < 60 {
		return fmt.Sprintf("%dm%02ds", mins, remSecs)
	}
	hours := mins / 60
	remMins := mins % 60
	return fmt.Sprintf("%dh%02dm%02ds", hours, remMins, remSecs)
}

func (o *batchObserver) emitHeartbeat() {
	type activeSnapshot struct {
		target    workspace.TargetID
		elapsed   string
		hasSample bool
		stats     LiveProcessStats
		lastLine  string
	}
	o.mu.Lock()
	if o.ui == nil {
		o.mu.Unlock()
		return
	}
	now := time.Now()
	var active []activeSnapshot
	for _, t := range o.order {
		ts := o.states[t]
		if ts == nil || ts.status != targetRunning {
			continue
		}
		el := now.Sub(ts.startedAt)
		if ts.startedAt.IsZero() {
			el = now.Sub(o.start)
		}
		active = append(active, activeSnapshot{
			target:    t,
			elapsed:   el.Round(time.Second).String(),
			hasSample: ts.hasSample && (ts.stats.CurrentMemoryBytes > 0 || ts.stats.PeakMemoryBytes > 0),
			stats:     ts.stats,
			lastLine:  ts.lastLine,
		})
	}
	o.mu.Unlock()

	for _, snap := range active {
		meta := snap.elapsed
		if snap.hasSample {
			meta = fmt.Sprintf(
				"%s, cpu %.0f%%, ram %s/pk %s",
				snap.elapsed,
				snap.stats.CPUPercent,
				formatMemoryBytes(snap.stats.CurrentMemoryBytes),
				formatMemoryBytes(snap.stats.PeakMemoryBytes),
			)
		}
		if snap.lastLine != "" {
			o.ui.Info("Still analyzing %s (%s): %s", snap.target, meta, snap.lastLine)
		} else {
			o.ui.Info("Still analyzing %s (%s)", snap.target, meta)
		}
	}
}
