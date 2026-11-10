package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Prompter defines interactive prompt methods for TTY sessions.
type Prompter interface {
	Confirm(prompt string, defaultValue bool) (bool, error)
	Select(prompt string, options []string, defaultIndex int) (int, error)
	Input(prompt string, defaultValue string) (string, error)
}

// DiagnosticError represents a structured, actionable error following the gh/clig.dev standard.
type DiagnosticError struct {
	Message  string
	Expected string
	Help     []string
}

func (e *DiagnosticError) Error() string {
	return e.Message
}

// ColorMode controls whether ANSI colors are emitted.
type ColorMode int

const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

// Printer formats progress and diagnostics cleanly for both interactive terminals and non-TTY agents.
type Printer struct {
	Out       io.Writer
	Err       io.Writer
	IsTTYOut  bool
	IsTTYErr  bool
	ColorMode ColorMode
}

// NewDefaultPrinter creates a Printer attached to os.Stdout and os.Stderr.
func NewDefaultPrinter() *Printer {
	initTerminalColors()
	return &Printer{
		Out:      os.Stdout,
		Err:      os.Stderr,
		IsTTYOut: isTerminal(os.Stdout),
		IsTTYErr: isTerminal(os.Stderr),
	}
}

// CanColor reports whether colored ANSI formatting should be emitted.
func (p *Printer) CanColor() bool {
	if p.ColorMode == ColorNever {
		return false
	}
	if p.ColorMode == ColorAlways {
		return true
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if force := os.Getenv("CLICOLOR_FORCE"); force != "" && force != "0" {
		return true
	}
	return p.IsTTYOut
}

// Green wraps text in green ANSI codes if colors are supported.
func (p *Printer) Green(s string) string {
	if !p.CanColor() {
		return s
	}
	return "\033[32m" + s + "\033[0m"
}

// Red wraps text in red ANSI codes if colors are supported.
func (p *Printer) Red(s string) string {
	if !p.CanColor() {
		return s
	}
	return "\033[31m" + s + "\033[0m"
}

// Yellow wraps text in yellow ANSI codes if colors are supported.
func (p *Printer) Yellow(s string) string {
	if !p.CanColor() {
		return s
	}
	return "\033[33m" + s + "\033[0m"
}

// Cyan wraps text in cyan ANSI codes if colors are supported.
func (p *Printer) Cyan(s string) string {
	if !p.CanColor() {
		return s
	}
	return "\033[36m" + s + "\033[0m"
}

// Bold wraps text in bold ANSI codes if colors are supported.
func (p *Printer) Bold(s string) string {
	if !p.CanColor() {
		return s
	}
	return "\033[1m" + s + "\033[0m"
}

// Dim wraps text in dim/gray ANSI codes if colors are supported.
func (p *Printer) Dim(s string) string {
	if !p.CanColor() {
		return s
	}
	return "\033[90m" + s + "\033[0m"
}

// Info prints a standard progress or informational line.
func (p *Printer) Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	_, _ = io.WriteString(p.Out, msg)
}

// Success prints a completion or milestone line.
func (p *Printer) Success(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	_, _ = io.WriteString(p.Out, msg)
}

// Error prints an error message. If err is a *DiagnosticError, it renders expected/help blocks.
func (p *Printer) Error(err error) {
	if err == nil {
		return
	}

	diag, ok := err.(*DiagnosticError)
	if !ok {
		_, _ = fmt.Fprintf(p.Err, "error: %v\n", err)
		return
	}

	_, _ = fmt.Fprintf(p.Err, "error: %s\n", diag.Message)
	if diag.Expected != "" {
		_, _ = fmt.Fprintf(p.Err, "  expected: %s\n", diag.Expected)
	}
	for _, h := range diag.Help {
		_, _ = fmt.Fprintf(p.Err, "  help: %s\n", h)
	}
}

// TerminalPrompter reads interactive inputs from a reader (defaults to os.Stdin).
type TerminalPrompter struct {
	In     io.Reader
	Out    io.Writer
	reader *bufio.Reader
}

// NewTerminalPrompter constructs a prompter reading from in and writing prompts to out.
func NewTerminalPrompter(in io.Reader, out io.Writer) *TerminalPrompter {
	return &TerminalPrompter{
		In:     in,
		Out:    out,
		reader: bufio.NewReader(in),
	}
}

// Confirm prompts for a yes/no question.
func (tp *TerminalPrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	hint := " [Y/n]: "
	if !defaultValue {
		hint = " [y/N]: "
	}
	_, _ = fmt.Fprint(tp.Out, prompt+hint)

	line, err := tp.reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return defaultValue, err
	}
	ans := strings.TrimSpace(line)
	if ans == "" {
		return defaultValue, nil
	}
	switch strings.ToLower(ans) {
	case "y", "yes", "true", "1":
		return true, nil
	case "n", "no", "false", "0":
		return false, nil
	default:
		return defaultValue, nil
	}
}

// Select prompts the user to choose an item from a list of options.
// In an interactive terminal, it supports Up/Down arrow keys, number keys (1-9), and Enter.
// In non-interactive mode, it falls back to reading a line of text.
func (tp *TerminalPrompter) Select(prompt string, options []string, defaultIndex int) (int, error) {
	if len(options) == 0 {
		return -1, nil
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	// Non-TTY or mock/piped input fallback (e.g. automated tests or redirected stdin)
	if tp.In != os.Stdin || !IsStdinTerminal() {
		_, _ = fmt.Fprintf(tp.Out, "%s:\n", prompt)
		for i, opt := range options {
			marker := "  "
			if i == defaultIndex {
				marker = "* "
			}
			_, _ = fmt.Fprintf(tp.Out, "  %s[%d] %s\n", marker, i+1, opt)
		}
		_, _ = fmt.Fprintf(tp.Out, "Select [1-%d] (default %d): ", len(options), defaultIndex+1)

		line, err := tp.reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return defaultIndex, err
		}
		ans := strings.TrimSpace(line)
		if ans == "" {
			return defaultIndex, nil
		}
		var choice int
		if _, err := fmt.Sscanf(ans, "%d", &choice); err == nil && choice >= 1 && choice <= len(options) {
			return choice - 1, nil
		}
		return defaultIndex, nil
	}

	// Interactive arrow-key navigation in real console
	_, _ = fmt.Fprintf(tp.Out, "%s (use ↑/↓ arrow keys, Enter to confirm):\n", prompt)
	current := defaultIndex

	render := func() {
		for i, opt := range options {
			if i == current {
				_, _ = fmt.Fprintf(tp.Out, "  \033[36m* [%d] %s\033[0m\n", i+1, opt)
			} else {
				_, _ = fmt.Fprintf(tp.Out, "    [%d] %s\n", i+1, opt)
			}
		}
	}

	render()

	for {
		key := readConsoleKey()
		switch key.Type {
		case KeyCancel:
			os.Exit(130)
		case KeyEnter:
			return current, nil
		case KeyUp:
			current = (current - 1 + len(options)) % len(options)
			_, _ = fmt.Fprintf(tp.Out, "\033[%dA", len(options))
			render()
		case KeyDown:
			current = (current + 1) % len(options)
			_, _ = fmt.Fprintf(tp.Out, "\033[%dA", len(options))
			render()
		case KeyOther:
			if key.Char >= '1' && int(key.Char-'0') <= len(options) {
				current = int(key.Char - '0' - 1)
				_, _ = fmt.Fprintf(tp.Out, "\033[%dA", len(options))
				render()
			}
		}
	}
}

// Input prompts for a free-form string with a default fallback.
func (tp *TerminalPrompter) Input(prompt string, defaultValue string) (string, error) {
	if defaultValue != "" {
		_, _ = fmt.Fprintf(tp.Out, "%s [%s]: ", prompt, defaultValue)
	} else {
		_, _ = fmt.Fprintf(tp.Out, "%s: ", prompt)
	}

	line, err := tp.reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return defaultValue, err
	}
	ans := strings.TrimSpace(line)
	if ans == "" {
		return defaultValue, nil
	}
	return ans, nil
}

// isTerminal checks if f is connected to a character device (console/terminal).
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// IsStdinTerminal checks if os.Stdin is an interactive terminal.
func IsStdinTerminal() bool {
	return isTerminal(os.Stdin)
}
