//go:build windows

package analyze

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunObservedCommand_WindowsJobObjectKillsBatchTreeCleanly(t *testing.T) {
	// Given a .bat wrapper spawning a long-running child process
	dir := t.TempDir()
	batPath := filepath.Join(dir, "sleep_tree.bat")
	batContent := "@echo off\r\necho started\r\nping -n 30 127.0.0.1 >NUL\r\n"
	if err := os.WriteFile(batPath, []byte(batContent), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, batPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	// When runObservedCommand cancels via Windows Job Object
	start := time.Now()
	metrics, err := runObservedCommand(ctx, cmd, nil)
	elapsed := time.Since(start)

	// Then it returns quickly without "Terminate batch job" prompts and records JobAssigned
	if err == nil {
		t.Fatalf("expected error on canceled command")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("expected fast job termination (<3s), took %s", elapsed)
	}
	if strings.Contains(out.String(), "Terminate batch job") {
		t.Fatalf("unexpected batch prompt in output: %q", out.String())
	}
	if !metrics.JobAssigned {
		t.Fatalf("expected JobAssigned=true on Windows")
	}
}

func TestRunObservedCommand_CollectsMetricsOnQuickCommand(t *testing.T) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, compiledFakeHelper, "-c")
	cmd.Dir = t.TempDir()
	metrics, _ := runObservedCommand(ctx, cmd, nil)

	if metrics.WallDuration <= 0 {
		t.Fatalf("expected WallDuration > 0, got %s", metrics.WallDuration)
	}
	if metrics.AvailPhysBeforeBytes == 0 || metrics.AvailPhysAfterBytes == 0 || metrics.AvailPhysMinBytes == 0 {
		t.Fatalf("expected non-zero available physical memory samples, got %+v", metrics)
	}
}

func TestRunObservedCommand_StreamsLiveProcessSamples(t *testing.T) {
	dir := t.TempDir()
	batPath := filepath.Join(dir, "live_sample.bat")
	batContent := "@echo off\r\nping -n 3 127.0.0.1 >NUL\r\n"
	if err := os.WriteFile(batPath, []byte(batContent), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, batPath)
	var samples []LiveProcessStats
	metrics, err := runObservedCommand(ctx, cmd, func(s LiveProcessStats) {
		samples = append(samples, s)
	})
	if err != nil {
		t.Fatalf("unexpected command error: %v", err)
	}
	if !metrics.JobAssigned {
		t.Fatalf("expected JobAssigned=true on Windows")
	}
	if len(samples) == 0 {
		t.Fatalf("expected at least 1 live sample from active child process")
	}
	sawMem := false
	for _, s := range samples {
		if s.CurrentMemoryBytes > 0 || s.PeakMemoryBytes > 0 {
			sawMem = true
			break
		}
	}
	if !sawMem {
		t.Fatalf("expected live samples to report non-zero working set or peak memory, got %+v", samples)
	}
}
