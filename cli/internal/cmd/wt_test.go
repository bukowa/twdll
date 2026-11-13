package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"twdll/cli/internal/fslink"
	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
	"twdll/cli/internal/worktree"
)

type wtTestRepo struct {
	primaryRoot string
	dataDir     string
	layout      workspace.Layout
	sentinels   map[string]string
}

func setupWTTestRepo(t *testing.T) wtTestRepo {
	t.Helper()
	t.Setenv("TWDLL_ROOT", "")
	t.Setenv("TWDLL_WRAPPER_ROOT", "")
	t.Setenv("TWDLL_CONFIG", "")
	t.Setenv("TWDLL_DATA_DIR", "")

	base := t.TempDir()
	primaryRoot := filepath.Join(base, "twdll")
	if err := os.MkdirAll(filepath.Join(primaryRoot, "cli"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := worktree.RunGit(ctx, primaryRoot, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	goMod := filepath.Join(primaryRoot, "cli", "go.mod")
	if err := os.WriteFile(goMod, []byte("module twdll/cli\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.RunGit(ctx, primaryRoot, "add", "cli/go.mod"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.RunGit(ctx, primaryRoot, "commit", "-m", "init"); err != nil {
		t.Fatal(err)
	}

	dataDir := filepath.Join(primaryRoot, ".twdll")
	sentinels := make(map[string]string, len(worktree.SharedSubdirs))
	for _, sub := range worktree.SharedSubdirs {
		subDir := filepath.Join(dataDir, sub)
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatal(err)
		}
		sentinel := filepath.Join(subDir, "sentinel-"+sub+".txt")
		if err := os.WriteFile(sentinel, []byte("sentinel-"+sub), 0o644); err != nil {
			t.Fatal(err)
		}
		sentinels[sub] = sentinel
	}

	if err := os.WriteFile(filepath.Join(dataDir, "state.json"), []byte(`{"version":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := workspace.SaveConfig(filepath.Join(primaryRoot, "twdll.toml"), workspace.Config{
		DefaultGame: workspace.GameAttila,
		Decompiler:  workspace.DecompilerGhidra,
	}); err != nil {
		t.Fatal(err)
	}

	layout, err := workspace.Resolve(workspace.ResolveSpec{
		RootOverride: primaryRoot,
		WorkingDir:   primaryRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	return wtTestRepo{
		primaryRoot: primaryRoot,
		dataDir:     dataDir,
		layout:      layout,
		sentinels:   sentinels,
	}
}

func (r wtTestRepo) assertSentinels(t *testing.T) {
	t.Helper()
	for sub, p := range r.sentinels {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("sentinel %s (%s) missing: %v", sub, p, err)
		}
		if string(b) != "sentinel-"+sub {
			t.Fatalf("sentinel %s = %q, want %q", sub, string(b), "sentinel-"+sub)
		}
	}
}

func TestRunWT_LifecycleAndStreamSeparation(t *testing.T) {
	repo := setupWTTestRepo(t)
	ctx := context.Background()

	// 1. RunWTAdd (human mode: path on stdout, status on stderr, no ANSI)
	var outBuf, errBuf bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &outBuf, Err: &errBuf, ColorMode: term.ColorNever})

	addRes, err := RunWTAdd(ctx, WTAddRequest{
		Layout: repo.layout,
		Options: WTAddOptions{
			Name: "feat/alpha",
		},
		UI: ui,
	})
	if err != nil {
		t.Fatalf("RunWTAdd failed: %v", err)
	}
	gotAddOut := strings.TrimSpace(outBuf.String())
	if !fslink.SamePath(gotAddOut, addRes.Worktree.Path) {
		t.Fatalf("RunWTAdd stdout = %q, want %q", gotAddOut, addRes.Worktree.Path)
	}
	if errBuf.Len() == 0 || strings.Contains(outBuf.String(), "\033") || strings.Contains(errBuf.String(), "\033") {
		t.Fatalf("expected non-empty stderr and zero ANSI escapes; out=%q err=%q", outBuf.String(), errBuf.String())
	}

	// 2. RunWTAdd (--json mode: JSON on stdout, empty stderr)
	outBuf.Reset()
	errBuf.Reset()
	addJSONRes, err := RunWTAdd(ctx, WTAddRequest{
		Layout: repo.layout,
		Options: WTAddOptions{
			Name: "feat/beta",
			JSON: true,
		},
		UI: ui,
	})
	if err != nil {
		t.Fatalf("RunWTAdd --json failed: %v", err)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected empty stderr on RunWTAdd --json, got %q", errBuf.String())
	}
	var decodedAdd worktree.AddResult
	if err := json.Unmarshal(outBuf.Bytes(), &decodedAdd); err != nil {
		t.Fatalf("invalid JSON from RunWTAdd: %v\nraw: %s", err, outBuf.String())
	}
	if !decodedAdd.Status.Ready || !fslink.SamePath(decodedAdd.Worktree.Path, addJSONRes.Worktree.Path) {
		t.Fatalf("unexpected decoded AddResult: %+v", decodedAdd)
	}

	// 3. RunWTList (--json and human table)
	outBuf.Reset()
	errBuf.Reset()
	listRep, err := RunWTList(ctx, WTListRequest{
		Layout: repo.layout,
		JSON:   true,
		UI:     ui,
	})
	if err != nil {
		t.Fatalf("RunWTList --json failed: %v", err)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected empty stderr on RunWTList --json, got %q", errBuf.String())
	}
	var decodedList WTListReport
	if err := json.Unmarshal(outBuf.Bytes(), &decodedList); err != nil {
		t.Fatalf("invalid JSON from RunWTList: %v\nraw: %s", err, outBuf.String())
	}
	if len(decodedList.Worktrees) != 3 || len(listRep.Worktrees) != 3 {
		t.Fatalf("expected 3 worktrees, got %d", len(decodedList.Worktrees))
	}

	outBuf.Reset()
	errBuf.Reset()
	if _, err := RunWTList(ctx, WTListRequest{
		Layout: repo.layout,
		JSON:   false,
		UI:     ui,
	}); err != nil {
		t.Fatalf("RunWTList human failed: %v", err)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected empty stderr on RunWTList human table, got %q", errBuf.String())
	}
	tableOut := outBuf.String()
	if strings.Contains(tableOut, "\033") {
		t.Fatalf("expected zero ANSI escapes in human table when ColorNever, got %q", tableOut)
	}
	for _, wantSub := range []string{"BRANCH", "STATUS", "primary", "ready", "feat/alpha", "feat/beta"} {
		if !strings.Contains(tableOut, wantSub) {
			t.Fatalf("expected human table to contain %q, got:\n%s", wantSub, tableOut)
		}
	}

	// 4. RunWTPath
	outBuf.Reset()
	errBuf.Reset()
	resolvedPath, err := RunWTPath(ctx, WTPathRequest{
		Layout: repo.layout,
		Options: WTPathOptions{
			Target: "feat/alpha",
			CWD:    repo.primaryRoot,
		},
		UI: ui,
	})
	if err != nil {
		t.Fatalf("RunWTPath failed: %v", err)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected empty stderr on RunWTPath, got %q", errBuf.String())
	}
	if !fslink.SamePath(strings.TrimSpace(outBuf.String()), addRes.Worktree.Path) || !fslink.SamePath(resolvedPath, addRes.Worktree.Path) {
		t.Fatalf("RunWTPath = %q (stdout %q), want %q", resolvedPath, outBuf.String(), addRes.Worktree.Path)
	}

	// 5. RunWTSync (break a link first, then repair via --all and --json)
	if err := fslink.RemoveLinkOnly(filepath.Join(addRes.Worktree.Path, ".twdll", "db")); err != nil {
		t.Fatalf("RemoveLinkOnly: %v", err)
	}
	outBuf.Reset()
	errBuf.Reset()
	syncRes, err := RunWTSync(ctx, WTSyncRequest{
		Layout: repo.layout,
		Options: WTSyncOptions{
			All:  true,
			JSON: true,
			CWD:  repo.primaryRoot,
		},
		UI: ui,
	})
	if err != nil {
		t.Fatalf("RunWTSync --all --json failed: %v", err)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected empty stderr on RunWTSync --json, got %q", errBuf.String())
	}
	if len(syncRes.Synced) != 2 {
		t.Fatalf("expected 2 synced worktrees, got %d", len(syncRes.Synced))
	}

	// Also verify human mode for RunWTSync
	outBuf.Reset()
	errBuf.Reset()
	if _, err := RunWTSync(ctx, WTSyncRequest{
		Layout: repo.layout,
		Options: WTSyncOptions{
			Target: "feat/alpha",
			CWD:    repo.primaryRoot,
		},
		UI: ui,
	}); err != nil {
		t.Fatalf("RunWTSync human failed: %v", err)
	}
	if outBuf.Len() != 0 || errBuf.Len() == 0 {
		t.Fatalf("expected empty stdout and non-empty stderr on RunWTSync human; out=%q err=%q", outBuf.String(), errBuf.String())
	}

	// 6. RunWTRemove guards (CWD inside worktree, then dirty worktree)
	_, err = RunWTRemove(ctx, WTRemoveRequest{
		Layout: repo.layout,
		Options: WTRemoveOptions{
			Target: "feat/alpha",
			CWD:    addRes.Worktree.Path,
		},
		UI: ui,
	})
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when CWD is inside target worktree, got %v", err)
	}

	dirtyFile := filepath.Join(addRes.Worktree.Path, "untracked.txt")
	if err := os.WriteFile(dirtyFile, []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = RunWTRemove(ctx, WTRemoveRequest{
		Layout: repo.layout,
		Options: WTRemoveOptions{
			Target: "feat/alpha",
			CWD:    repo.primaryRoot,
		},
		UI: ui,
	})
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when worktree is dirty without --force, got %v", err)
	}

	// 7. RunWTRemove (--force human and --json)
	outBuf.Reset()
	errBuf.Reset()
	if _, err := RunWTRemove(ctx, WTRemoveRequest{
		Layout: repo.layout,
		Options: WTRemoveOptions{
			Target:       "feat/alpha",
			Force:        true,
			DeleteBranch: true,
			CWD:          repo.primaryRoot,
		},
		UI: ui,
	}); err != nil {
		t.Fatalf("RunWTRemove --force failed: %v", err)
	}
	if outBuf.Len() != 0 || errBuf.Len() == 0 {
		t.Fatalf("expected empty stdout and non-empty stderr on RunWTRemove human; out=%q err=%q", outBuf.String(), errBuf.String())
	}

	outBuf.Reset()
	errBuf.Reset()
	if _, err := RunWTRemove(ctx, WTRemoveRequest{
		Layout: repo.layout,
		Options: WTRemoveOptions{
			Target:       "feat/beta",
			DeleteBranch: true,
			JSON:         true,
			CWD:          repo.primaryRoot,
		},
		UI: ui,
	}); err != nil {
		t.Fatalf("RunWTRemove --json failed: %v", err)
	}
	if errBuf.Len() != 0 || outBuf.Len() == 0 {
		t.Fatalf("expected JSON on stdout and empty stderr on RunWTRemove --json; out=%q err=%q", outBuf.String(), errBuf.String())
	}

	repo.assertSentinels(t)
}

type fakeCmdCodeGraphRunner struct {
	available   bool
	initCalls   []string
	syncCalls   []string
	uninitCalls []string
}

func (f *fakeCmdCodeGraphRunner) Available() bool { return f.available }

func (f *fakeCmdCodeGraphRunner) Init(_ context.Context, wtPath string) error {
	f.initCalls = append(f.initCalls, wtPath)
	dotCG := filepath.Join(wtPath, ".codegraph")
	if err := os.MkdirAll(dotCG, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dotCG, "codegraph.db"), []byte("db"), 0o644)
}

func (f *fakeCmdCodeGraphRunner) Sync(_ context.Context, wtPath string) error {
	f.syncCalls = append(f.syncCalls, wtPath)
	return nil
}

func (f *fakeCmdCodeGraphRunner) Uninit(_ context.Context, wtPath string) error {
	f.uninitCalls = append(f.uninitCalls, wtPath)
	return os.RemoveAll(filepath.Join(wtPath, ".codegraph"))
}

func TestRunWT_CodeGraphAndSnakeCaseJSON(t *testing.T) {
	repo := setupWTTestRepo(t)
	ctx := context.Background()

	// Seed .gitignore and primary .codegraph directory so auto-CodeGraph triggers.
	if err := os.WriteFile(filepath.Join(repo.primaryRoot, ".gitignore"), []byte(".codegraph/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.RunGit(ctx, repo.primaryRoot, "add", ".gitignore"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.RunGit(ctx, repo.primaryRoot, "commit", "-m", "ignore .codegraph"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo.primaryRoot, ".codegraph"), 0o755); err != nil {
		t.Fatal(err)
	}

	var outBuf, errBuf bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &outBuf, Err: &errBuf, ColorMode: term.ColorNever})
	cgRunner := &fakeCmdCodeGraphRunner{available: true}

	// 1. RunWTAdd human mode with CodeGraph enabled:
	// stdout must ONLY contain "<cleanPath>\n", while stderr contains the CodeGraph initialization message.
	addHumanRes, err := RunWTAdd(ctx, WTAddRequest{
		Layout: repo.layout,
		Options: WTAddOptions{
			Name:            "feat/cg-human",
			CodeGraphRunner: cgRunner,
		},
		UI: ui,
	})
	if err != nil {
		t.Fatalf("RunWTAdd human with CodeGraph failed: %v", err)
	}
	if outBuf.String() != addHumanRes.Worktree.Path+"\n" {
		t.Fatalf("RunWTAdd stdout = %q, want exact %q", outBuf.String(), addHumanRes.Worktree.Path+"\n")
	}
	if !strings.Contains(errBuf.String(), "Initialized CodeGraph index in "+addHumanRes.Worktree.Path) {
		t.Fatalf("expected stderr to mention CodeGraph initialization, got %q", errBuf.String())
	}

	// 2. RunWTAdd --json mode: verify snake_case LinkStatus keys and has_codegraph=true.
	outBuf.Reset()
	errBuf.Reset()
	_, err = RunWTAdd(ctx, WTAddRequest{
		Layout: repo.layout,
		Options: WTAddOptions{
			Name:            "feat/cg-json",
			JSON:            true,
			CodeGraphRunner: cgRunner,
		},
		UI: ui,
	})
	if err != nil {
		t.Fatalf("RunWTAdd --json with CodeGraph failed: %v", err)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected empty stderr in --json mode, got %q", errBuf.String())
	}
	rawAddJSON := outBuf.String()
	for _, wantKey := range []string{`"ready": true`, `"missing_links":`, `"mismatched_links":`, `"has_config": true`, `"has_state": true`, `"has_codegraph": true`} {
		if !strings.Contains(rawAddJSON, wantKey) {
			t.Fatalf("expected RunWTAdd JSON to contain %s, got:\n%s", wantKey, rawAddJSON)
		}
	}

	// 3. RunWTList --json mode: verify snake_case status fields across worktrees.
	outBuf.Reset()
	errBuf.Reset()
	if _, err := RunWTList(ctx, WTListRequest{
		Layout: repo.layout,
		JSON:   true,
		UI:     ui,
	}); err != nil {
		t.Fatalf("RunWTList --json failed: %v", err)
	}
	rawListJSON := outBuf.String()
	for _, wantKey := range []string{`"ready": true`, `"missing_links":`, `"mismatched_links":`, `"has_config": true`, `"has_state": true`, `"has_codegraph": true`} {
		if !strings.Contains(rawListJSON, wantKey) {
			t.Fatalf("expected RunWTList JSON to contain %s, got:\n%s", wantKey, rawListJSON)
		}
	}
}
