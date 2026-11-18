package cmd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"twdll/cli/internal/analyze"
	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

type extractRowStatus int

const (
	extractStepBronze = "bronze"

	extractQueued extractRowStatus = iota
	extractRunning
	extractOK
	extractSkipped
	extractFailed
	extractCanceled
)

type extractRowKey struct {
	target workspace.TargetID
	step   string
}

var extractSpinnerFrames = [4]string{"|", "/", "-", "\\"}

type progressSample struct {
	at   time.Time
	done uint64
}

type extractRowState struct {
	status      extractRowStatus
	startedAt   time.Time
	finishedAt  time.Time
	stage       string
	phase       string
	hasStatus   bool
	cached      int
	done        int
	total       int
	doneBytes   uint64
	totalBytes  uint64
	hasProgress bool
	samples     []progressSample
	byteSamples []progressSample
	etaSmooth   time.Duration
	lastETAAt   time.Time
	summary     string
	detail      string
}

type extractObserver struct {
	ui     *term.UI
	header string

	mu        sync.Mutex
	order     []extractRowKey
	states    map[extractRowKey]*extractRowState
	availPhys uint64
	lastMemAt time.Time
	frameIdx  int
	stopOnce  sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
}

func newExtractObserver(ui *term.UI, engines []workspace.DecompilerID, opts ExtractOptions, planned []plannedExtractTarget) *extractObserver {
	order := make([]extractRowKey, 0, len(planned)*(1+len(engines)))
	states := make(map[extractRowKey]*extractRowState, len(planned)*(1+len(engines)))
	for _, p := range planned {
		bKey := extractRowKey{target: p.target, step: extractStepBronze}
		order = append(order, bKey)
		states[bKey] = &extractRowState{status: extractQueued}
		for _, eng := range engines {
			sKey := extractRowKey{target: p.target, step: string(eng)}
			order = append(order, sKey)
			states[sKey] = &extractRowState{status: extractQueued}
		}
	}

	engNames := make([]string, 0, len(engines))
	for _, e := range engines {
		engNames = append(engNames, string(e))
	}
	var tags []string
	if opts.NoDecomp {
		tags = append(tags, "no-decomp")
	}
	header := fmt.Sprintf("Extracting (%s)", strings.Join(engNames, ", "))
	if len(tags) > 0 {
		header += " [" + strings.Join(tags, ", ") + "]"
	}

	return &extractObserver{
		ui:        ui,
		header:    header,
		order:     order,
		states:    states,
		availPhys: analyze.ProbeSystemMemory().AvailPhysBytes,
		lastMemAt: time.Now(),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

func (o *extractObserver) Start() {
	if o == nil || o.ui == nil || !o.ui.CanAnimate() {
		if o != nil {
			close(o.doneCh)
		}
		return
	}

	o.mu.Lock()
	o.renderLocked()
	o.mu.Unlock()

	go func() {
		defer close(o.doneCh)
		ticker := time.NewTicker(125 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-o.stopCh:
				return
			case <-ticker.C:
				now := time.Now()
				var sampled uint64
				o.mu.Lock()
				needMem := now.Sub(o.lastMemAt) >= 2*time.Second
				o.mu.Unlock()
				if needMem {
					sampled = analyze.ProbeSystemMemory().AvailPhysBytes
				}

				o.mu.Lock()
				if needMem && sampled > 0 {
					o.availPhys = sampled
					o.lastMemAt = now
				}
				o.frameIdx = (o.frameIdx + 1) % len(extractSpinnerFrames)
				o.renderLocked()
				o.mu.Unlock()
			}
		}
	}()
}

func (o *extractObserver) Close() {
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

func resetRowProgressState(rs *extractRowState) {
	rs.hasStatus = false
	rs.cached = 0
	rs.done = 0
	rs.total = 0
	rs.doneBytes = 0
	rs.totalBytes = 0
	rs.hasProgress = false
	rs.samples = rs.samples[:0]
	rs.byteSamples = rs.byteSamples[:0]
	rs.etaSmooth = 0
	rs.lastETAAt = time.Time{}
}

func (o *extractObserver) RowStarted(key extractRowKey, phase string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if rs := o.states[key]; rs != nil {
		rs.status = extractRunning
		rs.startedAt = time.Now()
		rs.finishedAt = time.Time{}
		rs.stage = strings.TrimSuffix(phase, "...")
		rs.phase = phase
		resetRowProgressState(rs)
		if o.ui != nil && o.ui.CanAnimate() {
			o.renderLocked()
		}
	}
	o.mu.Unlock()
}

func (o *extractObserver) RowPhase(key extractRowKey, phase string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if rs := o.states[key]; rs != nil {
		rs.stage = strings.TrimSuffix(phase, "...")
		rs.phase = phase
		resetRowProgressState(rs)
	}
	o.mu.Unlock()
}

func (o *extractObserver) RowStatus(key extractRowKey, status string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if rs := o.states[key]; rs != nil && status != "" {
		rs.phase = status
		rs.hasStatus = true
	}
	o.mu.Unlock()
}

func (o *extractObserver) RowResume(key extractRowKey, cached, pending int) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if rs := o.states[key]; rs != nil {
		if cached > 0 {
			rs.cached = cached
		}
		if pending > 0 {
			rs.total = pending
		}
		if rs.cached > 0 || rs.total > 0 {
			rs.hasProgress = true
		}
	}
	o.mu.Unlock()
}

func (o *extractObserver) RowProgress(key extractRowKey, phase string, done, total int) {
	if o == nil {
		return
	}
	now := time.Now()
	o.mu.Lock()
	if rs := o.states[key]; rs != nil {
		stageKey := strings.TrimSuffix(phase, "...")
		sameStage := rs.stage == "" || rs.stage == stageKey
		sameTotal := rs.total == 0 || total == 0 || rs.total == total
		if !sameStage || !sameTotal {
			resetRowProgressState(rs)
		}
		if rs.stage == "" || !sameStage {
			rs.stage = stageKey
		}
		if !rs.hasStatus || !sameStage {
			rs.phase = phase
		}
		if total > 0 && done > total {
			done = total
		}
		if total > 0 {
			rs.total = total
		}
		rs.hasProgress = rs.total > 0 || done > 0 || rs.cached > 0
		if done > rs.done {
			rs.done = done
			rs.samples = appendProgressSample(rs.samples, now, uint64(done))
		}
	}
	o.mu.Unlock()
}

func (o *extractObserver) RowBytes(key extractRowKey, doneBytes, totalBytes uint64) {
	if o == nil {
		return
	}
	now := time.Now()
	o.mu.Lock()
	if rs := o.states[key]; rs != nil {
		if rs.totalBytes > 0 && totalBytes > 0 && rs.totalBytes != totalBytes {
			rs.byteSamples = rs.byteSamples[:0]
			rs.doneBytes = 0
			rs.etaSmooth = 0
			rs.lastETAAt = time.Time{}
		}
		if totalBytes > 0 && doneBytes > totalBytes {
			doneBytes = totalBytes
		}
		if totalBytes > 0 {
			rs.totalBytes = totalBytes
		}
		if doneBytes > rs.doneBytes {
			rs.doneBytes = doneBytes
			rs.byteSamples = appendProgressSample(rs.byteSamples, now, doneBytes)
		}
	}
	o.mu.Unlock()
}

const (
	etaWindow        = 15 * time.Second
	etaMinWarmup     = 3 * time.Second
	etaSampleSpacing = 1 * time.Second
)

func appendProgressSample(samples []progressSample, now time.Time, done uint64) []progressSample {
	if n := len(samples); n == 0 {
		samples = append(samples, progressSample{at: now, done: done})
	} else if now.Sub(samples[n-1].at) >= etaSampleSpacing {
		samples = append(samples, progressSample{at: now, done: done})
	}
	cutoff := now.Add(-etaWindow)
	// Keep one anchor sample at or just before cutoff so the window stays >= etaWindow wide.
	drop := 0
	for drop+1 < len(samples) && samples[drop+1].at.Before(cutoff) {
		drop++
	}
	if drop > 0 {
		samples = append(samples[:0], samples[drop:]...)
	}
	return samples
}

func computeRowETA(ts *extractRowState, now time.Time) (time.Duration, bool) {
	if ts.total <= 0 || ts.done >= ts.total {
		return 0, false
	}

	var rawETA time.Duration
	haveRaw := false

	// Prefer byte-weighted ETA when byte progress is available so smallest-first sorting
	// does not produce overly optimistic ETAs from early tiny functions.
	if ts.totalBytes > 0 && ts.doneBytes > 0 && ts.doneBytes < ts.totalBytes && len(ts.byteSamples) > 0 {
		anchor := ts.byteSamples[0]
		dt := now.Sub(anchor.at).Seconds()
		if dt >= etaMinWarmup.Seconds() && ts.doneBytes > anchor.done {
			rate := float64(ts.doneBytes-anchor.done) / dt
			if rate > 0 {
				remBytes := ts.totalBytes - ts.doneBytes
				rawETA = time.Duration(float64(remBytes)/rate*float64(time.Second)).Round(time.Second)
				haveRaw = true
			}
		}
	}
	if !haveRaw && len(ts.samples) > 0 {
		anchor := ts.samples[0]
		dt := now.Sub(anchor.at).Seconds()
		if dt >= etaMinWarmup.Seconds() && uint64(ts.done) > anchor.done && uint64(ts.done)-anchor.done >= 10 {
			rate := float64(uint64(ts.done)-anchor.done) / dt
			if rate > 0 {
				rem := ts.total - ts.done
				rawETA = time.Duration(float64(rem)/rate*float64(time.Second)).Round(time.Second)
				haveRaw = true
			}
		}
	}

	if !haveRaw {
		return 0, false
	}
	if rawETA < 0 {
		rawETA = 0
	}
	if ts.etaSmooth == 0 {
		ts.etaSmooth = rawETA
		ts.lastETAAt = now
	} else if now.Sub(ts.lastETAAt) >= 500*time.Millisecond {
		blended := time.Duration(0.65*float64(ts.etaSmooth) + 0.35*float64(rawETA))
		ts.etaSmooth = blended.Round(time.Second)
		ts.lastETAAt = now
	}
	return ts.etaSmooth, true
}

func (o *extractObserver) RowFinished(key extractRowKey, summary string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if rs := o.states[key]; rs != nil {
		rs.status = extractOK
		if !rs.startedAt.IsZero() {
			rs.finishedAt = time.Now()
		}
		rs.summary = summary
		if o.ui != nil && o.ui.CanAnimate() {
			o.renderLocked()
		}
	}
	o.mu.Unlock()
}

func (o *extractObserver) RowSkipped(key extractRowKey, summary string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if rs := o.states[key]; rs != nil {
		rs.status = extractSkipped
		rs.summary = summary
		if o.ui != nil && o.ui.CanAnimate() {
			o.renderLocked()
		}
	}
	o.mu.Unlock()
}

func (o *extractObserver) RowFailed(key extractRowKey, canceled bool, detail string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if rs := o.states[key]; rs != nil {
		if canceled {
			rs.status = extractCanceled
		} else {
			rs.status = extractFailed
		}
		if !rs.startedAt.IsZero() {
			rs.finishedAt = time.Now()
		}
		rs.detail = detail
		if o.ui != nil && o.ui.CanAnimate() {
			o.renderLocked()
		}
	}
	o.mu.Unlock()
}

func (o *extractObserver) CancelTargetRemaining(tid workspace.TargetID) {
	if o == nil {
		return
	}
	now := time.Now()
	o.mu.Lock()
	for _, key := range o.order {
		if key.target != tid {
			continue
		}
		rs := o.states[key]
		if rs == nil {
			continue
		}
		if rs.status == extractQueued || rs.status == extractRunning {
			rs.status = extractCanceled
			if !rs.startedAt.IsZero() {
				rs.finishedAt = now
			}
		}
	}
	if o.ui != nil && o.ui.CanAnimate() {
		o.renderLocked()
	}
	o.mu.Unlock()
}

func (o *extractObserver) renderLocked() {
	if o.ui == nil || !o.ui.CanAnimate() {
		return
	}
	st := o.ui.ErrStyle()
	headerLine := st.Bold(o.header)
	if o.availPhys > 0 {
		headerLine += " " + st.Dim("| free RAM "+formatExtractBytes(o.availPhys))
	}

	lines := make([]string, 0, 1+len(o.order))
	lines = append(lines, headerLine)
	now := time.Now()
	spinChar := st.Cyan(extractSpinnerFrames[o.frameIdx%len(extractSpinnerFrames)])

	for _, key := range o.order {
		rs := o.states[key]
		if rs == nil {
			continue
		}
		name := fmt.Sprintf("%-9s", key.target)
		step := fmt.Sprintf("%-15s", formatExtractStepLabel(key.step))
		el := ""
		if !rs.startedAt.IsZero() {
			el = formatExtractElapsed(extractRowElapsed(rs, now))
		}
		switch rs.status {
		case extractQueued:
			lines = append(lines, fmt.Sprintf("  %s %s  %s  %6s | %s", st.Dim("[..]"), name, step, "", st.Dim("queued")))
		case extractRunning:
			statusText := rs.phase
			if statusText == "" {
				statusText = "running..."
			}
			if rs.hasProgress {
				label := strings.TrimSuffix(statusText, "...")
				if label == "" {
					label = "progress"
				}
				if rs.total > 0 {
					dispDone := rs.cached + rs.done
					dispTotal := rs.cached + rs.total
					pct := float64(dispDone) * 100.0 / float64(dispTotal)
					rem := rs.total - rs.done
					if rem > 0 {
						etaPart := ""
						if etaDur, ok := computeRowETA(rs, now); ok {
							etaPart = ", ~" + formatExtractElapsed(etaDur)
						}
						statusText = fmt.Sprintf("%s: %d/%d (%.1f%%, %d left%s)", label, dispDone, dispTotal, pct, rem, etaPart)
					} else {
						statusText = fmt.Sprintf("%s: %d/%d (%.1f%%)", label, dispDone, dispTotal, pct)
					}
				} else if rs.done > 0 {
					statusText = fmt.Sprintf("%s: %d fn", label, rs.done)
				}
			}
			lines = append(lines, fmt.Sprintf("  [%s]  %s  %s  %6s | %s", spinChar, name, step, el, statusText))
		case extractOK:
			summary := rs.summary
			if summary == "" {
				summary = "done"
			}
			lines = append(lines, fmt.Sprintf("  %s %s  %s  %6s | %s", st.Green("[ok]"), name, step, el, summary))
		case extractSkipped:
			summary := rs.summary
			if summary == "" {
				summary = "skipped"
			}
			lines = append(lines, fmt.Sprintf("  %s %s  %s  %6s | %s", st.Dim("[--]"), name, step, el, st.Dim(summary)))
		case extractFailed:
			detail := rs.detail
			if detail == "" {
				detail = "failed"
			}
			lines = append(lines, fmt.Sprintf("  %s %s  %s  %6s | failed: %s", st.Red("[!!]"), name, step, el, detail))
		case extractCanceled:
			lines = append(lines, fmt.Sprintf("  %s %s  %s  %6s | canceled", st.Yellow("[--]"), name, step, el))
		}
	}
	o.ui.SetLiveLines(lines)
}

func formatExtractStepLabel(step string) string {
	if step == extractStepBronze {
		return "bronze (format)"
	}
	return "silver (" + step + ")"
}

func extractRowElapsed(ts *extractRowState, now time.Time) time.Duration {
	if ts.startedAt.IsZero() {
		return 0
	}
	if !ts.finishedAt.IsZero() && ts.status != extractRunning {
		return ts.finishedAt.Sub(ts.startedAt)
	}
	return now.Sub(ts.startedAt)
}

func formatExtractElapsed(d time.Duration) string {
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
	return fmt.Sprintf("%dh%02dm", hours, remMins)
}

func formatExtractBytes(b uint64) string {
	const (
		miB = uint64(1024 * 1024)
		giB = uint64(1024 * 1024 * 1024)
	)
	if b >= giB {
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(giB))
	}
	return fmt.Sprintf("%d MiB", (b+miB/2)/miB)
}
