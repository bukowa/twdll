package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"twdll/cli/internal/fslink"
	"twdll/cli/internal/term"
)

// CodeGraphRunner abstracts execution of the optional external `codegraph` CLI.
type CodeGraphRunner interface {
	Available() bool
	Init(ctx context.Context, wtPath string) error
	Sync(ctx context.Context, wtPath string) error
	Uninit(ctx context.Context, wtPath string) error
}

type execCodeGraphRunner struct{}

// DefaultCodeGraphRunner returns a CodeGraphRunner backed by `os/exec`.
func DefaultCodeGraphRunner() CodeGraphRunner {
	return &execCodeGraphRunner{}
}

func (r *execCodeGraphRunner) Available() bool {
	_, err := exec.LookPath("codegraph")
	return err == nil
}

func (r *execCodeGraphRunner) Init(ctx context.Context, wtPath string) error {
	return r.runCodeGraph(ctx, 60*time.Second, "init", "-y", filepath.Clean(wtPath))
}

func (r *execCodeGraphRunner) Sync(ctx context.Context, wtPath string) error {
	return r.runCodeGraph(ctx, 60*time.Second, "sync", filepath.Clean(wtPath))
}

func (r *execCodeGraphRunner) Uninit(ctx context.Context, wtPath string) error {
	return r.runCodeGraph(ctx, 15*time.Second, "uninit", "--force", filepath.Clean(wtPath))
}

func (r *execCodeGraphRunner) runCodeGraph(ctx context.Context, timeout time.Duration, args ...string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fullArgs := append([]string{"--no-color"}, args...)
	cmd := exec.CommandContext(runCtx, "codegraph", fullArgs...)
	cmd.Stdin = nil
	cmd.WaitDelay = 3 * time.Second

	env := os.Environ()
	filtered := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "NO_COLOR=") {
			filtered = append(filtered, kv)
		}
	}
	filtered = append(filtered, "NO_COLOR=1")
	cmd.Env = filtered

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(out.String())
		if detail != "" {
			return fmt.Errorf("codegraph %s: %w: %s", strings.Join(args, " "), err, detail)
		}
		return fmt.Errorf("codegraph %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// ShouldAutoCodeGraph reports whether CodeGraph should be automatically initialized/synced
// for linked worktrees of primaryRoot.
func ShouldAutoCodeGraph(primaryRoot string, runner CodeGraphRunner) bool {
	if runner == nil || strings.TrimSpace(primaryRoot) == "" {
		return false
	}
	fi, err := os.Stat(filepath.Join(filepath.Clean(primaryRoot), ".codegraph"))
	if err != nil || !fi.IsDir() {
		return false
	}
	return runner.Available()
}

func hasCodeGraphDB(wtPath string) bool {
	if strings.TrimSpace(wtPath) == "" {
		return false
	}
	dbPath := filepath.Join(filepath.Clean(wtPath), ".codegraph", "codegraph.db")
	fi, err := os.Stat(dbPath)
	return err == nil && !fi.IsDir()
}

// ProvisionCodeGraph initializes or syncs the CodeGraph index inside wtPath.
func ProvisionCodeGraph(ctx context.Context, wtPath string, runner CodeGraphRunner) error {
	if strings.TrimSpace(wtPath) == "" {
		return &term.DiagnosticError{Message: "worktree path must not be empty"}
	}
	if runner == nil {
		runner = DefaultCodeGraphRunner()
	}

	cleanWT := filepath.Clean(wtPath)
	var err error
	action := "initialize"
	if hasCodeGraphDB(cleanWT) {
		action = "sync"
		err = runner.Sync(ctx, cleanWT)
	} else {
		err = runner.Init(ctx, cleanWT)
	}
	if err != nil {
		return &term.DiagnosticError{
			Message: fmt.Sprintf("failed to %s CodeGraph index in %s: %s", action, cleanWT, strings.TrimSpace(err.Error())),
			Help: []string{
				"Re-run with '--no-codegraph' to skip automatic CodeGraph indexing.",
			},
		}
	}
	return nil
}

// TeardownCodeGraph safely stops any CodeGraph daemon for wtPath and removes `<wtPath>/.codegraph`
// before `git worktree remove` runs.
func TeardownCodeGraph(ctx context.Context, wtPath string, runner CodeGraphRunner) error {
	if strings.TrimSpace(wtPath) == "" {
		return nil
	}
	cleanWT := filepath.Clean(wtPath)
	dotCG := filepath.Join(cleanWT, ".codegraph")

	if _, err := os.Lstat(dotCG); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	isLink, lerr := fslink.IsLinkOrReparsePoint(dotCG)
	if lerr != nil {
		return lerr
	}
	if isLink {
		return fslink.RemoveLinkOnly(dotCG)
	}

	if runner == nil {
		runner = DefaultCodeGraphRunner()
	}

	var uninitErr error
	available := runner.Available()
	if available {
		uninitErr = runner.Uninit(ctx, cleanWT)
	}

	if _, err := os.Lstat(dotCG); err == nil {
		for attempt := 0; attempt < 4; attempt++ {
			if attempt > 0 {
				time.Sleep(100 * time.Millisecond)
			}
			_ = fslink.RemoveAllNoFollow(dotCG)
			if _, statErr := os.Lstat(dotCG); errors.Is(statErr, os.ErrNotExist) {
				return nil
			}
		}
	} else if errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return err
	}

	if _, err := os.Lstat(dotCG); err == nil {
		msg := fmt.Sprintf("failed to remove %s due to active file locks", dotCG)
		if uninitErr != nil {
			msg = fmt.Sprintf("%s (codegraph uninit failed: %s)", msg, strings.TrimSpace(uninitErr.Error()))
		} else if !available {
			msg = fmt.Sprintf("%s (codegraph executable not found on PATH)", msg)
		}
		return &term.DiagnosticError{
			Message:  msg,
			Expected: fmt.Sprintf("no active background watcher or file locks on %s", dotCG),
			Help: []string{
				"Stop any active CodeGraph background watcher via 'codegraph daemon' before re-running 'twdll wt remove'.",
			},
		}
	}

	return nil
}
