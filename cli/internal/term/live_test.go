package term

import (
	"bytes"
	"strings"
	"testing"
)

func TestLiveLines_NoOpWhenNonTTYOrColorNever(t *testing.T) {
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

		ui.SetLiveLines([]string{"header", "  [|] attila_64 running"})
		ui.CommitLiveLines()
		ui.ClearLiveLines()

		if stdout.Len() != 0 {
			t.Fatalf("expected 0 stdout bytes, got %q", stdout.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("expected 0 stderr bytes, got %q", stderr.String())
		}
	}

	var ttyErr bytes.Buffer
	ttyNoColorUI := &UI{
		in:        strings.NewReader(""),
		out:       &bytes.Buffer{},
		err:       &ttyErr,
		isTTYErr:  true,
		colorMode: ColorNever,
	}
	ttyNoColorUI.SetLiveLines([]string{"header", "row 1"})
	ttyNoColorUI.CommitLiveLines()
	ttyNoColorUI.ClearLiveLines()
	if ttyErr.Len() != 0 {
		t.Fatalf("expected 0 stderr bytes when ColorNever on TTY, got %q", ttyErr.String())
	}
}

func TestLiveLines_AnimatedGrowShrinkLogInterleavingAndCommit(t *testing.T) {
	var stderr bytes.Buffer
	ui := &UI{
		in:        strings.NewReader(""),
		out:       &bytes.Buffer{},
		err:       &stderr,
		isTTYErr:  true,
		colorMode: ColorAlways,
	}

	// Initial 2 lines
	ui.SetLiveLines([]string{"Header", "Row 1"})
	firstFrame := stderr.String()
	wantFirst := "\r\033[2KHeader\n\r\033[2KRow 1"
	if firstFrame != wantFirst {
		t.Fatalf("initial frame = %q, want %q", firstFrame, wantFirst)
	}
	stderr.Reset()

	// Grow 2 -> 4 lines: moves up 1 line (\r\033[1A) and redraws 4 lines without trailing \n
	ui.SetLiveLines([]string{"Header", "Row 1", "Row 2", "Row 3"})
	growFrame := stderr.String()
	wantGrow := "\r\033[1A\r\033[2KHeader\n\r\033[2KRow 1\n\r\033[2KRow 2\n\r\033[2KRow 3"
	if growFrame != wantGrow {
		t.Fatalf("grow frame = %q, want %q", growFrame, wantGrow)
	}
	stderr.Reset()

	// Shrink 4 -> 2 lines: moves up 3 lines (\r\033[3A), redraws 2 lines, ends with \033[J
	ui.SetLiveLines([]string{"Header", "Row 1 done"})
	shrinkFrame := stderr.String()
	wantShrink := "\r\033[3A\r\033[2KHeader\n\r\033[2KRow 1 done\033[J"
	if shrinkFrame != wantShrink {
		t.Fatalf("shrink frame = %q, want %q", shrinkFrame, wantShrink)
	}
	stderr.Reset()

	// Info log while live block active: clears block (\r\033[1A\033[J), writes log line, re-renders live block
	ui.Info("log message")
	interleaved := stderr.String()
	wantInterleaved := "\r\033[1A\033[Jlog message\n\r\033[2KHeader\n\r\033[2KRow 1 done"
	if interleaved != wantInterleaved {
		t.Fatalf("interleaved log frame = %q, want %q", interleaved, wantInterleaved)
	}
	stderr.Reset()

	// CommitLiveLines appends "\n" without clearing the block and resets renderedLines = 0
	ui.CommitLiveLines()
	if stderr.String() != "\n" {
		t.Fatalf("CommitLiveLines output = %q, want %q", stderr.String(), "\n")
	}
	stderr.Reset()

	// Subsequent ClearLiveLines is a no-op
	ui.ClearLiveLines()
	if stderr.Len() != 0 {
		t.Fatalf("expected ClearLiveLines after CommitLiveLines to emit 0 bytes, got %q", stderr.String())
	}
}
