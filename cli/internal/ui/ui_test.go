package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDiagnosticErrorFormatting(t *testing.T) {
	var errBuf bytes.Buffer
	p := &Printer{
		Err: &errBuf,
	}

	// Standard error
	p.Error(errors.New("simple error"))
	if got := errBuf.String(); got != "error: simple error\n" {
		t.Errorf("got %q, want %q", got, "error: simple error\n")
	}

	// DiagnosticError
	errBuf.Reset()
	diag := &DiagnosticError{
		Message:  "missing reference binary",
		Expected: "C:/Steam/Attila",
		Help: []string{
			"run download_depot in Steam Console",
			"re-run twdll init",
		},
	}
	p.Error(diag)

	expectedOutput := "error: missing reference binary\n  expected: C:/Steam/Attila\n  help: run download_depot in Steam Console\n  help: re-run twdll init\n"
	if got := errBuf.String(); got != expectedOutput {
		t.Errorf("got %q, want %q", got, expectedOutput)
	}
}

func TestPrompter(t *testing.T) {
	// 1. Confirm: empty input returns default (true)
	in := strings.NewReader("\n")
	out := &bytes.Buffer{}
	prompter := NewTerminalPrompter(in, out)

	ok, err := prompter.Confirm("Proceed?", true)
	if err != nil || !ok {
		t.Errorf("Confirm default failed: got %v, want true", ok)
	}

	// 2. Select: empty input returns default
	in = strings.NewReader("\n")
	prompter = NewTerminalPrompter(in, out)
	idx, err := prompter.Select("Pick one", []string{"A", "B", "C"}, 1)
	if err != nil || idx != 1 {
		t.Errorf("Select default failed: got %d, want 1", idx)
	}

	// 3. Input: non-empty input returns typed string
	in = strings.NewReader("custom_value\n")
	prompter = NewTerminalPrompter(in, out)
	val, err := prompter.Input("Name", "default_val")
	if err != nil || val != "custom_value" {
		t.Errorf("Input custom failed: got %q, want custom_value", val)
	}
}

func TestColorHelpers(t *testing.T) {
	// 1. Non-TTY or NO_COLOR: strings are untouched
	pNonTTY := &Printer{IsTTYOut: false}
	if got := pNonTTY.Green("ok"); got != "ok" {
		t.Errorf("expected plain string, got %q", got)
	}

	// 2. TTY with colors enabled
	pTTY := &Printer{IsTTYOut: true}
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	if got := pTTY.Green("ok"); got != "\033[32mok\033[0m" {
		t.Errorf("expected green ANSI, got %q", got)
	}
	if got := pTTY.Red("fail"); got != "\033[31mfail\033[0m" {
		t.Errorf("expected red ANSI, got %q", got)
	}
	if got := pTTY.Yellow("warn"); got != "\033[33mwarn\033[0m" {
		t.Errorf("expected yellow ANSI, got %q", got)
	}
	if got := pTTY.Bold("header"); got != "\033[1mheader\033[0m" {
		t.Errorf("expected bold ANSI, got %q", got)
	}
	if got := pTTY.Dim("sub"); got != "\033[90msub\033[0m" {
		t.Errorf("expected dim ANSI, got %q", got)
	}

	// 3. NO_COLOR env var suppresses colors even on TTY
	t.Setenv("NO_COLOR", "1")
	if got := pTTY.Green("ok"); got != "ok" {
		t.Errorf("expected plain string with NO_COLOR, got %q", got)
	}
}
