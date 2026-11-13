package term

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSpinner_NoOpAndZeroEscapes_whenNonTTYOrColorNever(t *testing.T) {
	for _, mode := range []ColorMode{ColorAuto, ColorAlways, ColorNever} {
		var stdout, stderr bytes.Buffer
		ui := NewUI(Streams{
			In:        strings.NewReader(""),
			Out:       &stdout,
			Err:       &stderr,
			ColorMode: mode,
		})
		if ui.CanAnimate() {
			t.Fatalf("expected CanAnimate() = false for non-TTY buffer in mode %v", mode)
		}

		sp := ui.StartSpinner("Analyzing attila_64")
		sp.SetText("Analyzing attila_64: Decompiling FUN_140001000")
		ui.Progress("direct progress update")
		sp.Stop()
		sp.Stop()

		if stdout.Len() != 0 {
			t.Fatalf("expected 0 stdout bytes, got %q", stdout.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("expected 0 stderr bytes from spinner/progress in non-TTY mode %v, got %q", mode, stderr.String())
		}
	}

	// Even when stderr is a TTY, ColorNever must disable animation and emit zero \r or \033[.
	var ttyErr bytes.Buffer
	ttyNoColorUI := &UI{
		in:        strings.NewReader(""),
		out:       &bytes.Buffer{},
		err:       &ttyErr,
		isTTYErr:  true,
		colorMode: ColorNever,
	}
	if ttyNoColorUI.CanAnimate() {
		t.Fatalf("expected CanAnimate() = false when ColorNever even on TTY")
	}
	sp := ttyNoColorUI.StartSpinner("Analyzing")
	sp.SetText("Step 1")
	ttyNoColorUI.Progress("progress")
	sp.Stop()
	if ttyErr.Len() != 0 {
		t.Fatalf("expected 0 stderr bytes when ColorNever on TTY, got %q", ttyErr.String())
	}
}

func TestSpinner_AnimatedMode_PreservesStatusAcrossLogLinesAndClearsOnStop(t *testing.T) {
	var stderr bytes.Buffer
	ui := &UI{
		in:        strings.NewReader(""),
		out:       &bytes.Buffer{},
		err:       &stderr,
		isTTYErr:  true,
		colorMode: ColorAlways,
	}
	if !ui.CanAnimate() {
		t.Fatalf("expected CanAnimate() = true when isTTYErr=true and ColorAlways")
	}

	sp := ui.StartSpinner("Starting batch")
	sp.SetText("Step 1 running\r\nwith newline and tab\tand very long text " + strings.Repeat("x", 120))
	sp.render(1)
	ui.Info("Target finished cleanly")
	sp.Stop()
	sp.Stop()

	out := stderr.String()
	if !strings.Contains(out, "\r\033[2K") {
		t.Fatalf("expected carriage-return line clear in animated TTY mode, got %q", out)
	}
	if !strings.Contains(out, "(0s)") {
		t.Fatalf("expected elapsed suffix not to be truncated on long spinner text, got %q", out)
	}
	if !strings.Contains(out, "Target finished cleanly\n") {
		t.Fatalf("expected Info log line in output, got %q", out)
	}
	if !strings.HasSuffix(out, "\r\033[2K") {
		t.Fatalf("expected final Stop() to clear progress line, got %q", out)
	}
}

func TestSpinner_NilSafeAndConcurrentSetTextStop(t *testing.T) {
	var nilUI *UI
	sp := nilUI.StartSpinner("noop")
	sp.SetText("still noop")
	sp.Stop()

	var stderr bytes.Buffer
	ui := &UI{
		in:        strings.NewReader(""),
		out:       &bytes.Buffer{},
		err:       &stderr,
		isTTYErr:  true,
		colorMode: ColorAlways,
	}
	active := ui.StartSpinner("concurrent")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range 20 {
				active.SetText(strings.Repeat("a", id+1))
			}
		}(i)
	}
	wg.Wait()
	active.Stop()
	active.Stop()
}

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{9 * time.Second, "9s"},
		{65 * time.Second, "1m05s"},
		{3661 * time.Second, "1h01m01s"},
	}
	for _, tc := range cases {
		if got := formatElapsed(tc.d); got != tc.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestTruncatePlainRunes(t *testing.T) {
	got := truncatePlainRunes("  hello\r\n\tworld   foo  ", 84)
	if got != "hello world foo" {
		t.Fatalf("unexpected sanitized string: %q", got)
	}

	long := strings.Repeat("a", 90)
	trunc := truncatePlainRunes(long, 84)
	if len([]rune(trunc)) != 84 || !strings.HasSuffix(trunc, "...") {
		t.Fatalf("expected 84 runes ending with '...', got len=%d %q", len([]rune(trunc)), trunc)
	}
}

func TestSanitizeProgressLine_MultibyteUTF8AndANSIReset(t *testing.T) {
	st := NewStyle(true)
	line := st.Cyan("⠋") + " " + st.Bold("αβγδεζηθικλμνξοπ")
	got := sanitizeProgressLine(line, 10)
	if !utf8.ValidString(got) {
		t.Fatalf("expected valid UTF-8 after truncation, got %q", got)
	}
	if !strings.HasSuffix(got, "\033[0m...") {
		t.Fatalf("expected truncated ANSI line to end with reset and ellipsis, got %q", got)
	}
	want := "\033[36m⠋\033[0m \033[1mαβγδε\033[0m..."
	if got != want {
		t.Fatalf("sanitizeProgressLine = %q, want %q", got, want)
	}
}
