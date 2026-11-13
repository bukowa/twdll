package term

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mattn/go-isatty"
	xterm "golang.org/x/term"
)

type ColorMode int

const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

func ParseColorMode(raw string) (ColorMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return ColorAuto, nil
	case "always":
		return ColorAlways, nil
	case "never":
		return ColorNever, nil
	default:
		return ColorAuto, fmt.Errorf("invalid color mode %q (expected auto, always, or never)", raw)
	}
}

type Style struct {
	enabled bool
}

func NewStyle(enabled bool) Style {
	return Style{enabled: enabled}
}

func (s Style) Enabled() bool { return s.enabled }

func (s Style) wrap(code, text string) string {
	if !s.enabled || text == "" {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

func (s Style) Bold(t string) string   { return s.wrap("1", t) }
func (s Style) Dim(t string) string    { return s.wrap("90", t) }
func (s Style) Green(t string) string  { return s.wrap("32", t) }
func (s Style) Yellow(t string) string { return s.wrap("33", t) }
func (s Style) Red(t string) string    { return s.wrap("31", t) }
func (s Style) Cyan(t string) string   { return s.wrap("36", t) }

type Streams struct {
	In          io.Reader
	Out         io.Writer
	Err         io.Writer
	ColorMode   ColorMode
	ForceTTYErr bool
}

// UI enforces strict stream separation: Out receives ONLY command result data,
// while Err receives ALL operational logs, progress, prompts, and diagnostics.
type UI struct {
	mu            sync.Mutex
	in            io.Reader
	out           io.Writer
	err           io.Writer
	isTTYIn       bool
	isTTYOut      bool
	isTTYErr      bool
	colorMode     ColorMode
	statusLine    string
	liveLines     []string
	renderedLines int
}

func NewStandardUI(mode ColorMode) *UI {
	return NewUI(Streams{
		In:        os.Stdin,
		Out:       os.Stdout,
		Err:       os.Stderr,
		ColorMode: mode,
	})
}

func NewUI(s Streams) *UI {
	in := s.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := s.Out
	if out == nil {
		out = io.Discard
	}
	errW := s.Err
	if errW == nil {
		errW = io.Discard
	}
	return &UI{
		in:        in,
		out:       out,
		err:       errW,
		isTTYIn:   detectTTY(in),
		isTTYOut:  detectTTY(out),
		isTTYErr:  s.ForceTTYErr || detectTTY(errW),
		colorMode: s.ColorMode,
	}
}

func (u *UI) Out() io.Writer      { return u.out }
func (u *UI) Err() io.Writer      { return u.err }
func (u *UI) In() io.Reader       { return u.in }
func (u *UI) IsInteractive() bool { return u.isTTYIn }
func (u *UI) IsTTYErr() bool      { return u.isTTYErr }
func (u *UI) CanAnimate() bool    { return u.isTTYErr && u.canColor(u.isTTYErr) }
func (u *UI) OutStyle() Style     { return Style{enabled: u.canColor(u.isTTYOut)} }
func (u *UI) ErrStyle() Style     { return Style{enabled: u.canColor(u.isTTYErr)} }

func (u *UI) canColor(isTTY bool) bool {
	if u.colorMode == ColorNever {
		return false
	}
	if u.colorMode == ColorAlways {
		return true
	}
	if os.Getenv("NO_COLOR") != "" || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	if force := os.Getenv("CLICOLOR_FORCE"); force != "" && force != "0" {
		return true
	}
	return isTTY
}

func (u *UI) WriteData(format string, args ...any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	_, _ = fmt.Fprintf(u.out, format, args...)
}

func (u *UI) Info(format string, args ...any) {
	u.writeErrLine(fmt.Sprintf(format, args...))
}

func (u *UI) Step(index, total int, title string) {
	st := u.ErrStyle()
	tag := st.Cyan(fmt.Sprintf("[%d/%d]", index, total))
	u.writeErrLine(fmt.Sprintf("%s %s", tag, st.Bold(title)))
}

func (u *UI) Success(format string, args ...any) {
	st := u.ErrStyle()
	msg := fmt.Sprintf(format, args...)
	u.writeErrLine(st.Green("[ok]") + " " + msg)
}

func (u *UI) Warn(format string, args ...any) {
	st := u.ErrStyle()
	msg := fmt.Sprintf(format, args...)
	u.writeErrLine(st.Yellow("warning:") + " " + msg)
}

func (u *UI) Error(err error) {
	if err == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.clearProgressLocked()
	u.clearLiveLocked()
	WriteError(u.err, u.ErrStyle(), err)
}

const maxProgressVisibleRunes = 96

func (u *UI) progressMaxRunes() int {
	if f, ok := u.err.(*os.File); ok {
		if w, _, err := xterm.GetSize(int(f.Fd())); err == nil && w > 10 {
			return w - 2
		}
	}
	return maxProgressVisibleRunes
}

func (u *UI) Progress(line string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.CanAnimate() {
		return
	}
	if u.renderedLines > 0 {
		u.clearLiveLocked()
	}
	clean := sanitizeProgressLine(line, u.progressMaxRunes())
	u.statusLine = clean
	_, _ = fmt.Fprintf(u.err, "\r\033[2K%s", clean)
}

func (u *UI) ClearProgress() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.clearProgressLocked()
}

func (u *UI) writeErrLine(msg string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	prevStatus := u.statusLine
	prevLive := slices.Clone(u.liveLines)
	u.clearProgressLocked()
	u.clearLiveLocked()
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	_, _ = io.WriteString(u.err, msg)
	if u.CanAnimate() {
		if len(prevLive) > 0 {
			u.renderLiveLocked(prevLive)
		} else if prevStatus != "" {
			u.statusLine = prevStatus
			_, _ = fmt.Fprintf(u.err, "\r\033[2K%s", prevStatus)
		}
	}
}

func (u *UI) clearProgressLocked() {
	if u.statusLine != "" && u.CanAnimate() {
		_, _ = io.WriteString(u.err, "\r\033[2K")
	}
	u.statusLine = ""
}

func sanitizeProgressLine(line string, maxVisible int) string {
	r := strings.NewReplacer("\r", " ", "\n", " ", "\t", " ")
	clean := r.Replace(line)
	if maxVisible <= 3 {
		return clean
	}
	var out strings.Builder
	out.Grow(len(clean))
	visible := 0
	sawANSI := false
	truncated := false
	for i := 0; i < len(clean); {
		if clean[i] == '\033' && i+1 < len(clean) && clean[i+1] == '[' {
			sawANSI = true
			j := i + 2
			for j < len(clean) && (clean[j] < 0x40 || clean[j] > 0x7E) {
				j++
			}
			if j < len(clean) {
				j++
			}
			if !truncated {
				out.WriteString(clean[i:j])
			}
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(clean[i:])
		visible++
		if visible > maxVisible {
			truncated = true
			break
		}
		out.WriteString(clean[i : i+size])
		i += size
	}
	if !truncated {
		return clean
	}
	// Rebuild up to maxVisible - 3 visible runes so suffix fits within maxVisible.
	out.Reset()
	limit := maxVisible - 3
	visible = 0
	for i := 0; i < len(clean) && visible < limit; {
		if clean[i] == '\033' && i+1 < len(clean) && clean[i+1] == '[' {
			j := i + 2
			for j < len(clean) && (clean[j] < 0x40 || clean[j] > 0x7E) {
				j++
			}
			if j < len(clean) {
				j++
			}
			out.WriteString(clean[i:j])
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(clean[i:])
		visible++
		out.WriteString(clean[i : i+size])
		i += size
	}
	if sawANSI {
		out.WriteString("\033[0m")
	}
	out.WriteString("...")
	return out.String()
}

func detectTTY(stream any) bool {
	f, ok := stream.(*os.File)
	if !ok || f == nil {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
