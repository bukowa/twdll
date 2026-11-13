package analyze

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

func TestLineSink_ChunkedCRLFAndANSIStripping(t *testing.T) {
	var lines []string
	sink := newLineSink(func(line string) {
		lines = append(lines, line)
	})

	// Chunk 1 splits mid-line and mid-CRLF, includes ANSI codes and blank lines
	_, _ = sink.Write([]byte("\033[36mINFO  Importing\033[0m"))
	_, _ = sink.Write([]byte(" binary\r\n   \r\n\tWARN: \033[33mslow\033[0m\r\nprogress 10%\rprogress 50%\npartial_tail"))
	sink.Flush()

	want := []string{
		"INFO  Importing binary",
		"WARN: slow",
		"progress 50%",
		"partial_tail",
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines %q, want %d %q", len(lines), lines, len(want), want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestTailLogFile_StreamsAppendsFlushesTailAndReleasesHandle(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "analyze_ida_attila_32.log")

	var got []string
	stop := make(chan struct{})
	defer close(stop)

	flush := tailLogFile(stop, logPath, func(line string) {
		got = append(got, line)
	})

	// Create log file after tailer starts (matching IDA -L behavior)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("Loading processor metapc...\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	deadline := time.Now().Add(2 * time.Second)
	for len(got) == 0 && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if len(got) == 0 || got[0] != "Loading processor metapc..." {
		t.Fatalf("expected initial tailed line, got %q", got)
	}

	// Append final partial line without trailing newline and flush
	f2, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f2.WriteString("Autoanalysis complete"); err != nil {
		t.Fatal(err)
	}
	_ = f2.Close()

	flush()
	if len(got) != 2 || got[1] != "Autoanalysis complete" {
		t.Fatalf("expected flushed partial tail line, got %q", got)
	}

	// Verify no lingering Windows file handle prevents removing the log file
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("expected log file handle released after flush: %v", err)
	}
}

func TestBatchObserver_NonTTY_ZeroEscapesAndEmitsHeartbeat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{
		In:        strings.NewReader(""),
		Out:       &stdout,
		Err:       &stderr,
		ColorMode: term.ColorNever,
	})

	targets := []workspace.TargetID{workspace.TargetRome2_32, workspace.TargetAttila32}
	obs := newBatchObserver(ui, workspace.DecompilerGhidra, "[1/1] ghidra 32-bit (2 workers)", targets)
	obs.heartbeatInterval = 20 * time.Millisecond
	obs.Start()
	obs.TargetStarted(workspace.TargetRome2_32)
	obs.TargetStarted(workspace.TargetAttila32)
	obs.OnLine(workspace.TargetRome2_32, "INFO  Importing rome2.dll")
	obs.OnLine(workspace.TargetAttila32, "INFO  Decompiling FUN_10001000")
	obs.OnSample(workspace.TargetAttila32, LiveProcessStats{
		CPUPercent:         125.0,
		CurrentMemoryBytes: 1500 * 1024 * 1024,
		PeakMemoryBytes:    1800 * 1024 * 1024,
	})

	deadline := time.Now().Add(2 * time.Second)
	for (!strings.Contains(stderr.String(), "Still analyzing rome2_32") || !strings.Contains(stderr.String(), "Still analyzing attila_32")) && time.Now().Before(deadline) {
		time.Sleep(15 * time.Millisecond)
	}
	obs.TargetFinished(workspace.TargetRome2_32, targetOK, ProcessMetrics{}, "4G", "")
	obs.TargetFinished(workspace.TargetAttila32, targetOK, ProcessMetrics{}, "4G", "")
	obs.Close()

	if stdout.Len() != 0 {
		t.Fatalf("expected 0 stdout bytes, got %q", stdout.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "\r") || strings.Contains(errOut, "\033[") {
		t.Fatalf("non-TTY batchObserver must emit zero CR/ANSI escapes, got %q", errOut)
	}
	if !strings.Contains(errOut, "Still analyzing rome2_32") || !strings.Contains(errOut, "INFO  Importing rome2.dll") {
		t.Fatalf("expected rome2_32 heartbeat with latest line, got %q", errOut)
	}
	if !strings.Contains(errOut, "Still analyzing attila_32") || !strings.Contains(errOut, "cpu 125%") || !strings.Contains(errOut, "INFO  Decompiling FUN_10001000") {
		t.Fatalf("expected attila_32 heartbeat with live CPU/RAM stats and latest line, got %q", errOut)
	}
}

func TestBatchObserver_AnimatedTTY_RendersPerTargetRowsAndCommitsOnClose(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{
		In:          strings.NewReader(""),
		Out:         &stdout,
		Err:         &stderr,
		ColorMode:   term.ColorAlways,
		ForceTTYErr: true,
	})

	targets := []workspace.TargetID{workspace.TargetRome2_32, workspace.TargetAttila32}
	obs := newBatchObserver(ui, workspace.DecompilerGhidra, "[1/1] ghidra 32-bit (2 workers)", targets)
	obs.Start()
	obs.TargetStarted(workspace.TargetRome2_32)
	obs.OnLine(workspace.TargetRome2_32, "INFO  Analyzing functions")
	obs.OnSample(workspace.TargetRome2_32, LiveProcessStats{
		CPUPercent:         98.0,
		CurrentMemoryBytes: 512 * 1024 * 1024,
		PeakMemoryBytes:    640 * 1024 * 1024,
		AvailPhysBytes:     16 * 1024 * 1024 * 1024,
	})
	obs.TargetFinished(workspace.TargetRome2_32, targetOK, ProcessMetrics{
		WallDuration:       12 * time.Second,
		CPUTime:            20 * time.Second,
		PeakJobMemoryBytes: 640 * 1024 * 1024,
	}, "4G", "")
	obs.TargetFinished(workspace.TargetAttila32, targetCanceled, ProcessMetrics{}, "", "canceled")
	obs.Close()

	if stdout.Len() != 0 {
		t.Fatalf("expected 0 stdout bytes, got %q", stdout.String())
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "[1/1] ghidra 32-bit (2 workers)") {
		t.Fatalf("expected batch header in stderr, got %q", errOut)
	}
	if !strings.Contains(errOut, "rome2_32") || !strings.Contains(errOut, "attila_32") {
		t.Fatalf("expected per-target rows in stderr, got %q", errOut)
	}
	if !strings.Contains(errOut, "peak 640 MB") || !strings.Contains(errOut, "canceled") {
		t.Fatalf("expected committed final target states in stderr, got %q", errOut)
	}
	if !strings.HasSuffix(errOut, "\n") {
		t.Fatalf("expected CommitLiveLines trailing newline on Close, got %q", errOut)
	}
}
