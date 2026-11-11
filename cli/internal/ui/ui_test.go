package ui

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
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

func TestProcessTracker(t *testing.T) {
	tracker := NewProcessTracker(os.Getpid())
	metrics := tracker.Sample()
	if metrics.RAMBytes == 0 {
		t.Logf("Process memory for current PID was 0 (expected > 0 on Windows)")
	} else {
		t.Logf("Current PID metrics: %s", metrics.Format())
	}
}

func TestMultiSpinner(t *testing.T) {
	tmp := t.TempDir()
	logPath := tmp + "/test.log"
	_ = os.WriteFile(logPath, []byte("line 1\nline 2: DWARF parsing\n\n"), 0o644)

	var buf bytes.Buffer
	printer := &Printer{
		Out:      &buf,
		IsTTYOut: true,
	}

	ms := NewMultiSpinner(printer)
	slot := ms.AddSlot(1, 1, "test_target", logPath)
	ms.Start()
	time.Sleep(150 * time.Millisecond)
	slot.Finish("test_target finished cleanly")
	ms.Stop()

	output := buf.String()
	if !strings.Contains(output, "test_target finished cleanly") {
		t.Errorf("expected output to contain final message, got: %q", output)
	}
}

func TestMultiSpinnerParallel(t *testing.T) {
	tmp := t.TempDir()
	log1 := tmp + "/test1.log"
	log2 := tmp + "/test2.log"
	_ = os.WriteFile(log1, []byte("target 1 line\n"), 0o644)
	_ = os.WriteFile(log2, []byte("target 2 line\n"), 0o644)

	var buf bytes.Buffer
	printer := &Printer{
		Out:      &buf,
		IsTTYOut: true,
	}

	ms := NewMultiSpinner(printer)
	slot1 := ms.AddSlot(1, 2, "target1", log1)
	slot2 := ms.AddSlot(2, 2, "target2", log2)
	ms.Start()
	slot1.Start()
	slot2.Start()
	time.Sleep(150 * time.Millisecond)
	slot1.Finish("  [1/2] target1 finished")
	slot2.Finish("  [2/2] target2 finished")
	ms.Stop()

	output := buf.String()
	if !strings.Contains(output, "target1 finished") || !strings.Contains(output, "target2 finished") {
		t.Errorf("expected multi-slot output to contain both finish messages, got: %q", output)
	}
}
