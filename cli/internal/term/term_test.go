package term

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestUI_StrictStreamSeparation_whenLoggingAndWritingData(t *testing.T) {
	// Given non-file buffers with terminal environment variables set
	var stdout, stderr bytes.Buffer
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("MSYSTEM", "MINGW64")

	ui := NewUI(Streams{
		In:        strings.NewReader(""),
		Out:       &stdout,
		Err:       &stderr,
		ColorMode: ColorAuto,
	})

	// When operational logs, progress updates, errors, and structured data are emitted
	ui.Step(1, 3, "Checking tools")
	ui.Info("found cmake")
	ui.Progress("transient spinner frame that must not appear in non-TTY")
	ui.Warn("minor notice")
	ui.Success("step finished")
	ui.Error(&DiagnosticError{
		Message:  "missing binary",
		Expected: "Attila ELF64",
		Help:     []string{"run twdll init"},
	})
	ui.WriteData(`{"ok":true}` + "\n")

	// Then stdout receives exclusively structured data and non-TTY stderr omits ANSI/progress
	if ui.IsInteractive() || ui.IsTTYErr() {
		t.Fatalf("expected non-file streams to report non-TTY even when TERM/MSYSTEM are set")
	}
	if stdout.String() != "{\"ok\":true}\n" {
		t.Fatalf("stdout polluted by operational logs: %q", stdout.String())
	}

	errText := stderr.String()
	if strings.Contains(errText, "\033[") {
		t.Fatalf("non-TTY stderr must not contain ANSI escape sequences: %q", errText)
	}
	if strings.Contains(errText, "transient spinner") {
		t.Fatalf("non-TTY stderr must suppress transient progress lines: %q", errText)
	}
	for _, want := range []string{"[1/3] Checking tools", "found cmake", "warning: minor notice", "[ok] step finished", "error: missing binary", "expected: Attila ELF64", "help: run twdll init"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("stderr missing %q in:\n%s", want, errText)
		}
	}
}

func TestLinePrompter_SelectAndConfirm_whenRedirected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ui := NewUI(Streams{
		In:        strings.NewReader("bad\n2\n\n\"C:\\Games\\Attila\"\n"),
		Out:       &stdout,
		Err:       &stderr,
		ColorMode: ColorNever,
	})
	p := NewLinePrompter(ui)

	idx, err := p.Select("Select backend", []string{"ghidra", "ida"}, 0)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if idx != 1 {
		t.Fatalf("Select index = %d, want 1", idx)
	}

	ok, err := p.Confirm("Proceed?", true)
	if err != nil || !ok {
		t.Fatalf("Confirm = (%v, %v), want (true, nil)", ok, err)
	}

	dir, err := p.Input("Game path", "")
	if err != nil || dir != `C:\Games\Attila` {
		t.Fatalf("Input = (%q, %v), want C:\\Games\\Attila", dir, err)
	}

	if stdout.Len() != 0 {
		t.Fatalf("prompts must write to stderr only, got stdout: %q", stdout.String())
	}
}

func TestLinePrompter_ReturnsAbortedOnEOF(t *testing.T) {
	ui := NewUI(Streams{In: strings.NewReader("")})
	p := NewLinePrompter(ui)

	_, err := p.Input("Path", "default")
	if !errors.Is(err, ErrInputAborted) {
		t.Fatalf("got err %v, want ErrInputAborted", err)
	}
}

func TestNextSelectEvent(t *testing.T) {
	cases := []struct {
		name     string
		in       []byte
		wantKind selectEventKind
		wantDig  int
		wantCons int
	}{
		{"up_csi", []byte("\x1b[A"), selectEventUp, 0, 3},
		{"down_ss3", []byte("\x1bOB"), selectEventDown, 0, 3},
		{"up_k", []byte("k"), selectEventUp, 0, 1},
		{"down_j", []byte("J"), selectEventDown, 0, 1},
		{"home_csi", []byte("\x1b[H"), selectEventHome, 0, 3},
		{"end_ss3", []byte("\x1bOF"), selectEventEnd, 0, 3},
		{"home_tilde", []byte("\x1b[1~"), selectEventHome, 0, 4},
		{"end_tilde", []byte("\x1b[4~"), selectEventEnd, 0, 4},
		{"digit_1", []byte("1"), selectEventDigit, 0, 1},
		{"digit_9", []byte("9"), selectEventDigit, 8, 1},
		{"csi_delete_no_leak", []byte("\x1b[3~"), selectEventNone, 0, 4},
		{"csi_ctrl_up_no_leak", []byte("\x1b[1;5A"), selectEventNone, 0, 6},
		{"bare_esc", []byte("\x1b"), selectEventNone, 0, 1},
		{"crlf", []byte("\r\n"), selectEventEnter, 0, 2},
		{"ctrl_c", []byte{0x03}, selectEventAbort, 0, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, dig, cons := nextSelectEvent(tc.in)
			if kind != tc.wantKind || dig != tc.wantDig || cons != tc.wantCons {
				t.Fatalf("nextSelectEvent(%q) = (%v, %d, %d), want (%v, %d, %d)",
					tc.in, kind, dig, cons, tc.wantKind, tc.wantDig, tc.wantCons)
			}
		})
	}
}

func TestSelectInteractiveLoop(t *testing.T) {
	options := []string{"attila", "rome2", "tob"}

	t.Run("wrap_navigation_and_digit_confirm", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ui := &UI{
			in:        strings.NewReader(""),
			out:       &stdout,
			err:       &stderr,
			isTTYIn:   true,
			isTTYErr:  true,
			colorMode: ColorAlways,
		}
		p := NewLinePrompter(ui)

		// Start at 0 (attila) -> Up wraps to 2 (tob) -> Down wraps to 0 (attila) -> Delete CSI ignored -> '2' highlights rome2 (idx 1) -> Enter confirms
		input := bytes.NewReader([]byte("\x1b[A\x1b[B\x1b[3~2\r\n"))
		idx, err := p.selectInteractiveLoop(input.Read, "Default game", options, 0, 96)
		if err != nil {
			t.Fatalf("selectInteractiveLoop failed: %v", err)
		}
		if idx != 1 {
			t.Fatalf("selected idx = %d, want 1", idx)
		}
		if stdout.Len() != 0 {
			t.Fatalf("expected 0 stdout bytes, got %q", stdout.String())
		}
		if !strings.HasSuffix(stderr.String(), "\033[4A\r\033[J") {
			t.Fatalf("expected menu erase suffix \\033[4A\\r\\033[J, got %q", stderr.String())
		}
	})

	t.Run("ctrl_c_aborts_and_erases_menu", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ui := &UI{
			in:        strings.NewReader(""),
			out:       &stdout,
			err:       &stderr,
			isTTYIn:   true,
			isTTYErr:  true,
			colorMode: ColorAlways,
		}
		p := NewLinePrompter(ui)

		input := bytes.NewReader([]byte{0x03})
		_, err := p.selectInteractiveLoop(input.Read, "Default game", options, 0, 96)
		if !errors.Is(err, ErrInputAborted) {
			t.Fatalf("got err %v, want ErrInputAborted", err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("expected 0 stdout bytes, got %q", stdout.String())
		}
		if !strings.HasSuffix(stderr.String(), "\033[4A\r\033[J") {
			t.Fatalf("expected menu erase suffix \\033[4A\\r\\033[J on abort, got %q", stderr.String())
		}
	})
}
