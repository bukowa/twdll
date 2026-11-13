package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"twdll/cli/internal/fslink"
	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

func TestParsePorcelain(t *testing.T) {
	raw := "worktree C:/repos/twdll\r\n" +
		"HEAD 1111111111111111111111111111111111111111\r\n" +
		"branch refs/heads/main\r\n" +
		"\r\n" +
		"worktree C:/repos/twdll.wt/feat-rome2\n" +
		"HEAD 2222222222222222222222222222222222222222\n" +
		"branch refs/heads/feat/rome2\n" +
		"locked working on rome2\n" +
		"\n" +
		"worktree C:/repos/twdll.wt/exp-detached\r\n" +
		"HEAD 3333333333333333333333333333333333333333\r\n" +
		"detached\r\n" +
		"prunable gitdir file points to non-existent location\r\n"

	list, err := ParsePorcelain(raw)
	if err != nil {
		t.Fatalf("ParsePorcelain failed: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 worktrees, got %d", len(list))
	}

	if !list[0].IsPrimary || list[0].Branch != "main" || list[0].Detached {
		t.Errorf("unexpected primary entry: %+v", list[0])
	}
	if list[1].IsPrimary || list[1].Branch != "feat/rome2" || !list[1].Locked || list[1].LockReason != "working on rome2" {
		t.Errorf("unexpected second entry: %+v", list[1])
	}
	if list[2].IsPrimary || !list[2].Detached || !list[2].Prunable || list[2].PruneReason == "" {
		t.Errorf("unexpected third entry: %+v", list[2])
	}

	// Bare primary repository should return a *term.DiagnosticError.
	bareRaw := "worktree C:/repos/bare.git\nHEAD 1111111111111111111111111111111111111111\nbare\n"
	_, err = ParsePorcelain(bareRaw)
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError for bare primary repo, got %v", err)
	}
}

func TestDefaultWorktreePathAndResolveTarget(t *testing.T) {
	tmp := t.TempDir()
	primary := filepath.Join(tmp, "twdll")

	p, err := DefaultWorktreePath(primary, "feat/rome2-ui")
	if err != nil {
		t.Fatalf("DefaultWorktreePath failed: %v", err)
	}
	want := filepath.Join(tmp, "twdll.wt", "feat-rome2-ui")
	if p != want {
		t.Fatalf("DefaultWorktreePath = %q, want %q", p, want)
	}

	for _, bad := range []string{"", "   ", ".", "..", "---", "feat/../bad", "C:bad", "bad\x00name", "bad\nname"} {
		if _, err := DefaultWorktreePath(primary, bad); err == nil {
			t.Errorf("DefaultWorktreePath(%q) succeeded, expected error", bad)
		}
	}

	wt1Dir := filepath.Join(tmp, "twdll.wt", "feat-alpha")
	wt2Dir := filepath.Join(tmp, "other", "feat-alpha")
	list := []GitWorktree{
		{Path: primary, Branch: "main", IsPrimary: true},
		{Path: wt1Dir, Branch: "feat/alpha"},
		{Path: wt2Dir, Branch: "feat/beta"},
	}

	// Match by exact branch name.
	got, err := ResolveTarget(list, "feat/alpha", tmp)
	if err != nil || got.Path != wt1Dir {
		t.Fatalf("ResolveTarget by branch = %+v, %v", got, err)
	}

	// Match by relative path from cwd.
	got, err = ResolveTarget(list, filepath.Join("other", "feat-alpha"), tmp)
	if err != nil || got.Path != wt2Dir {
		t.Fatalf("ResolveTarget by relative path = %+v, %v", got, err)
	}

	// Ambiguous basename (`feat-alpha` matches both wt1Dir and wt2Dir).
	_, err = ResolveTarget(list, "feat-alpha", tmp)
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected ambiguous *term.DiagnosticError, got %v", err)
	}

	// Missing target.
	_, err = ResolveTarget(list, "nonexistent", tmp)
	if !errors.As(err, &diag) {
		t.Fatalf("expected not-found *term.DiagnosticError, got %v", err)
	}
}

type testFixture struct {
	primaryRoot string
	dataDir     string
	sentinels   map[string]string
	stateFile   string
	configFile  string
}

func setupTestRepo(t *testing.T, customDataDir string) testFixture {
	t.Helper()
	t.Setenv("TWDLL_ROOT", "")
	t.Setenv("TWDLL_WRAPPER_ROOT", "")
	t.Setenv("TWDLL_CONFIG", "")
	t.Setenv("TWDLL_DATA_DIR", "")

	base := t.TempDir()
	primaryRoot := filepath.Join(base, "twdll")
	if err := os.MkdirAll(filepath.Join(primaryRoot, "cli"), 0o755); err != nil {
		t.Fatalf("mkdir primaryRoot: %v", err)
	}

	ctx := context.Background()
	mustGit(t, ctx, primaryRoot, "init")
	mustGit(t, ctx, primaryRoot, "config", "user.email", "test@example.com")
	mustGit(t, ctx, primaryRoot, "config", "user.name", "Test User")
	mustGit(t, ctx, primaryRoot, "config", "commit.gpgsign", "false")

	goModPath := filepath.Join(primaryRoot, "cli", "go.mod")
	if err := os.WriteFile(goModPath, []byte("module github.com/buk/twdll\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mustGit(t, ctx, primaryRoot, "add", "cli/go.mod")
	mustGit(t, ctx, primaryRoot, "commit", "-m", "initial commit")

	dataDir := customDataDir
	if dataDir == "" {
		dataDir = filepath.Join(primaryRoot, ".twdll")
	}

	sentinels := make(map[string]string, len(SharedSubdirs))
	for _, sub := range SharedSubdirs {
		subDir := filepath.Join(dataDir, sub)
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", subDir, err)
		}
		sentinelPath := filepath.Join(subDir, "sentinel-"+sub+".txt")
		content := "primary-sentinel-" + sub
		if err := os.WriteFile(sentinelPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write sentinel %s: %v", sentinelPath, err)
		}
		sentinels[sub] = sentinelPath
	}

	stateFile := filepath.Join(dataDir, "state.json")
	if err := os.WriteFile(stateFile, []byte(`{"version":1,"tools":{"go":{"version":"1.26.1"}}}`+"\n"), 0o644); err != nil {
		t.Fatalf("write state.json: %v", err)
	}

	cfg := workspace.Config{
		DataDir: customDataDir,
		IDADir:  "C:/IDA",
		Games: workspace.GamesConfig{
			Attila: "C:/Games/Attila",
			Rome2:  "C:/Games/Rome2",
		},
	}
	configFile := filepath.Join(primaryRoot, "twdll.toml")
	if err := workspace.SaveConfig(configFile, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	return testFixture{
		primaryRoot: primaryRoot,
		dataDir:     dataDir,
		sentinels:   sentinels,
		stateFile:   stateFile,
		configFile:  configFile,
	}
}

func (f testFixture) assertSentinelsIntact(t *testing.T) {
	t.Helper()
	for sub, path := range f.sentinels {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("primary sentinel for %s (%s) missing or unreadable: %v", sub, path, err)
		}
		want := "primary-sentinel-" + sub
		if string(data) != want {
			t.Fatalf("primary sentinel %s content = %q, want %q", sub, string(data), want)
		}
	}
	if _, err := os.Stat(f.stateFile); err != nil {
		t.Fatalf("primary state.json missing: %v", err)
	}
	if _, err := os.Stat(f.configFile); err != nil {
		t.Fatalf("primary twdll.toml missing: %v", err)
	}
}

func assertWorktreeProvisioned(t *testing.T, wtPath, expectedPrimaryDataDir string) {
	t.Helper()
	status := Inspect(wtPath, expectedPrimaryDataDir)
	if !status.Ready {
		t.Fatalf("Inspect(%s) not ready: %+v", wtPath, status)
	}
	if !status.HasConfig || !status.HasState {
		t.Fatalf("expected HasConfig and HasState true, got %+v", status)
	}

	// LocalSubdirs must be real directories, NOT links.
	for _, sub := range LocalSubdirs {
		p := filepath.Join(wtPath, ".twdll", sub)
		fi, err := os.Lstat(p)
		if err != nil || !fi.IsDir() {
			t.Fatalf("local subdir %s missing or not dir: %v", p, err)
		}
		isLink, err := fslink.IsLinkOrReparsePoint(p)
		if err != nil || isLink {
			t.Fatalf("local subdir %s must not be a link (isLink=%v, err=%v)", p, isLink, err)
		}
	}

	// SharedSubdirs must be links pointing directly to expectedPrimaryDataDir/<sub>.
	for _, sub := range SharedSubdirs {
		linkPath := filepath.Join(wtPath, ".twdll", sub)
		isLink, err := fslink.IsLinkOrReparsePoint(linkPath)
		if err != nil || !isLink {
			t.Fatalf("shared subdir %s is not a link (isLink=%v, err=%v)", linkPath, isLink, err)
		}
		target, err := fslink.ReadDirLink(linkPath)
		if err != nil {
			t.Fatalf("ReadDirLink(%s): %v", linkPath, err)
		}
		wantTarget := filepath.Join(expectedPrimaryDataDir, sub)
		if !fslink.SamePath(target, wantTarget) {
			t.Fatalf("link %s targets %s, want %s", linkPath, target, wantTarget)
		}

		// Sentinel file must be readable through the link.
		sentinel := filepath.Join(linkPath, "sentinel-"+sub+".txt")
		if _, err := os.ReadFile(sentinel); err != nil {
			t.Fatalf("read sentinel through link %s: %v", sentinel, err)
		}
	}

	// Seeded twdll.toml must clear DataDir while preserving game/IDA paths.
	wtCfg, exists, err := workspace.LoadConfig(filepath.Join(wtPath, "twdll.toml"))
	if err != nil || !exists {
		t.Fatalf("LoadConfig worktree twdll.toml: exists=%v, err=%v", exists, err)
	}
	if wtCfg.DataDir != "" {
		t.Fatalf("expected worktree twdll.toml DataDir to be empty, got %q", wtCfg.DataDir)
	}
	if wtCfg.Games.Attila != "C:/Games/Attila" || wtCfg.IDADir != "C:/IDA" {
		t.Fatalf("worktree twdll.toml did not preserve settings: %+v", wtCfg)
	}

	// Seeded state.json must be a regular file, not a symlink/reparse point.
	wtState := filepath.Join(wtPath, ".twdll", "state.json")
	if isLink, err := fslink.IsLinkOrReparsePoint(wtState); err != nil || isLink {
		t.Fatalf("worktree state.json must be a real file copy (isLink=%v, err=%v)", isLink, err)
	}
}

func TestAddVariantsAndChainingFromLinkedWorktree(t *testing.T) {
	ctx := context.Background()
	fx := setupTestRepo(t, "")

	// 1. Add a new branch worktree at the default path (<parent>/twdll.wt/feat-rome2).
	res1, err := Add(ctx, AddOptions{
		RepoDir: fx.primaryRoot,
		Name:    "feat/rome2",
	})
	if err != nil {
		t.Fatalf("Add feat/rome2 failed: %v", err)
	}
	if res1.Worktree.Branch != "feat/rome2" {
		t.Fatalf("expected branch feat/rome2, got %q", res1.Worktree.Branch)
	}
	assertWorktreeProvisioned(t, res1.Worktree.Path, fx.dataDir)

	// 2. Invoke Add from INSIDE worktree 1 (res1.Worktree.Path) to create worktree 2.
	// Verify worktree 2 links directly to primary's .twdll, NOT worktree 1's .twdll.
	res2, err := Add(ctx, AddOptions{
		RepoDir: res1.Worktree.Path,
		Name:    "feat/attila",
	})
	if err != nil {
		t.Fatalf("Add feat/attila from worktree 1 failed: %v", err)
	}
	assertWorktreeProvisioned(t, res2.Worktree.Path, fx.dataDir)
	for _, sub := range SharedSubdirs {
		target, err := fslink.ReadDirLink(filepath.Join(res2.Worktree.Path, ".twdll", sub))
		if err != nil {
			t.Fatalf("ReadDirLink on wt2 %s: %v", sub, err)
		}
		wt1Sub := filepath.Join(res1.Worktree.Path, ".twdll", sub)
		// Raw ReadDirLink target string must match primaryRoot/.twdll/<sub>, not wt1's path.
		wantPrimarySub := filepath.Join(fx.dataDir, sub)
		if filepath.Clean(target) != filepath.Clean(wantPrimarySub) {
			t.Fatalf("wt2 link %s raw target = %q, want direct primary path %q (wt1=%q)", sub, target, wantPrimarySub, wt1Sub)
		}
	}

	// 3. Add worktree for an existing branch; verify combining existing branch + --base fails.
	mustGit(t, ctx, fx.primaryRoot, "branch", "existing-branch")
	_, err = Add(ctx, AddOptions{
		RepoDir: fx.primaryRoot,
		Name:    "existing-branch",
		Base:    "HEAD",
	})
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when combining existing branch with --base, got %v", err)
	}

	resExisting, err := Add(ctx, AddOptions{
		RepoDir: fx.primaryRoot,
		Name:    "existing-branch",
	})
	if err != nil {
		t.Fatalf("Add existing-branch failed: %v", err)
	}
	if resExisting.Worktree.Branch != "existing-branch" {
		t.Fatalf("expected branch existing-branch, got %q", resExisting.Worktree.Branch)
	}
	assertWorktreeProvisioned(t, resExisting.Worktree.Path, fx.dataDir)

	// 4. Add detached worktree at custom path with explicit --base.
	customPath := filepath.Join(t.TempDir(), "custom-detached")
	resDetached, err := Add(ctx, AddOptions{
		RepoDir: fx.primaryRoot,
		Name:    "detached-exp",
		Base:    "HEAD",
		Path:    customPath,
		Detach:  true,
	})
	if err != nil {
		t.Fatalf("Add detached worktree failed: %v", err)
	}
	if !resDetached.Worktree.Detached || resDetached.Worktree.Branch != "" {
		t.Fatalf("expected detached worktree, got %+v", resDetached.Worktree)
	}
	assertWorktreeProvisioned(t, resDetached.Worktree.Path, fx.dataDir)
	fx.assertSentinelsIntact(t)
}

func TestAddWithCustomExternalDataDir(t *testing.T) {
	ctx := context.Background()
	externalDataDir := filepath.Join(t.TempDir(), "external-twdll-data")
	fx := setupTestRepo(t, externalDataDir)

	res, err := Add(ctx, AddOptions{
		RepoDir: fx.primaryRoot,
		Name:    "feat/external-data",
	})
	if err != nil {
		t.Fatalf("Add with custom external data_dir failed: %v", err)
	}

	// Links must target externalDataDir/<sub>, while <wt>/twdll.toml clears DataDir.
	assertWorktreeProvisioned(t, res.Worktree.Path, externalDataDir)
	fx.assertSentinelsIntact(t)
}

func TestRemoveGuardsAndSentinelSurvival(t *testing.T) {
	ctx := context.Background()
	fx := setupTestRepo(t, "")

	wtClean, err := Add(ctx, AddOptions{
		RepoDir: fx.primaryRoot,
		Name:    "feat/clean-rm",
	})
	if err != nil {
		t.Fatalf("Add feat/clean-rm: %v", err)
	}

	wtDirty, err := Add(ctx, AddOptions{
		RepoDir: fx.primaryRoot,
		Name:    "feat/dirty-rm",
	})
	if err != nil {
		t.Fatalf("Add feat/dirty-rm: %v", err)
	}

	var diag *term.DiagnosticError

	// 1. Refuse removing primary worktree.
	_, err = Remove(ctx, RemoveOptions{
		RepoDir: fx.primaryRoot,
		Target:  fx.primaryRoot,
		CWD:     t.TempDir(),
	})
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when removing primary worktree, got %v", err)
	}

	// 2. Refuse removing worktree when CWD is inside it.
	_, err = Remove(ctx, RemoveOptions{
		RepoDir: fx.primaryRoot,
		Target:  "feat/clean-rm",
		CWD:     filepath.Join(wtClean.Worktree.Path, "cli"),
	})
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when CWD is inside target worktree, got %v", err)
	}

	// 3. Refuse removing worktree when ExePath is inside it.
	_, err = Remove(ctx, RemoveOptions{
		RepoDir: fx.primaryRoot,
		Target:  "feat/clean-rm",
		CWD:     fx.primaryRoot,
		ExePath: filepath.Join(wtClean.Worktree.Path, ".twdll", "bin", "twdll.exe"),
	})
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when ExePath is inside target worktree, got %v", err)
	}

	// 4. Refuse removing dirty worktree without --force BEFORE touching .twdll/.
	dirtyFile := filepath.Join(wtDirty.Worktree.Path, "uncommitted.txt")
	if err := os.WriteFile(dirtyFile, []byte("dirty work\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	_, err = Remove(ctx, RemoveOptions{
		RepoDir: fx.primaryRoot,
		Target:  "feat/dirty-rm",
		CWD:     fx.primaryRoot,
		Force:   false,
	})
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when removing dirty worktree without --force, got %v", err)
	}
	// Verify .twdll/ and twdll.toml were NOT touched before the dirty check failed.
	assertWorktreeProvisioned(t, wtDirty.Worktree.Path, fx.dataDir)

	// 5. Clean removal of wtClean (without --force) preserves 100% of primary sentinels.
	rmCleanRes, err := Remove(ctx, RemoveOptions{
		RepoDir:      fx.primaryRoot,
		Target:       "feat/clean-rm",
		CWD:          fx.primaryRoot,
		DeleteBranch: true,
	})
	if err != nil {
		t.Fatalf("Remove clean worktree failed: %v", err)
	}
	if rmCleanRes.DeletedBranch != "feat/clean-rm" {
		t.Fatalf("expected DeletedBranch feat/clean-rm, got %q", rmCleanRes.DeletedBranch)
	}
	if _, err := os.Lstat(wtClean.Worktree.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be removed, stat err = %v", wtClean.Worktree.Path, err)
	}
	if BranchExists(ctx, fx.primaryRoot, "feat/clean-rm") {
		t.Fatalf("expected branch feat/clean-rm to be deleted")
	}
	fx.assertSentinelsIntact(t)

	// 6. Force removal of wtDirty (+ --delete-branch) succeeds and preserves 100% of primary sentinels.
	rmDirtyRes, err := Remove(ctx, RemoveOptions{
		RepoDir:      fx.primaryRoot,
		Target:       "feat/dirty-rm",
		CWD:          fx.primaryRoot,
		Force:        true,
		DeleteBranch: true,
	})
	if err != nil {
		t.Fatalf("Remove dirty worktree with --force failed: %v", err)
	}
	if rmDirtyRes.DeletedBranch != "feat/dirty-rm" {
		t.Fatalf("expected DeletedBranch feat/dirty-rm, got %q", rmDirtyRes.DeletedBranch)
	}
	if _, err := os.Lstat(wtDirty.Worktree.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be removed, stat err = %v", wtDirty.Worktree.Path, err)
	}
	fx.assertSentinelsIntact(t)
}

func TestSyncRawWorktreeAndAll(t *testing.T) {
	ctx := context.Background()
	fx := setupTestRepo(t, "")

	// Create a raw unprovisioned worktree directly via `git worktree add`.
	rawPath, err := DefaultWorktreePath(fx.primaryRoot, "raw-wt")
	if err != nil {
		t.Fatalf("DefaultWorktreePath: %v", err)
	}
	mustGit(t, ctx, fx.primaryRoot, "worktree", "add", "-b", "raw-wt", rawPath)

	statusBefore := Inspect(rawPath, fx.dataDir)
	if statusBefore.Ready || len(statusBefore.MissingLinks) != len(SharedSubdirs) {
		t.Fatalf("expected raw worktree to be unprovisioned, got %+v", statusBefore)
	}

	// Calling Sync from primary without target or --all must return a *term.DiagnosticError.
	_, err = Sync(ctx, SyncOptions{
		RepoDir: fx.primaryRoot,
		CWD:     fx.primaryRoot,
	})
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when syncing from primary without target/--all, got %v", err)
	}

	// Sync raw-wt by target name.
	syncRes, err := Sync(ctx, SyncOptions{
		RepoDir: fx.primaryRoot,
		Target:  "raw-wt",
		CWD:     fx.primaryRoot,
	})
	if err != nil {
		t.Fatalf("Sync raw-wt failed: %v", err)
	}
	if len(syncRes.Synced) != 1 || !syncRes.Synced[0].Status.Ready {
		t.Fatalf("expected 1 ready synced worktree, got %+v", syncRes)
	}
	assertWorktreeProvisioned(t, rawPath, fx.dataDir)

	// Simulate a broken link replaced by a non-empty local directory in raw-wt.
	dbLink := filepath.Join(rawPath, ".twdll", "db")
	if err := fslink.RemoveLinkOnly(dbLink); err != nil {
		t.Fatalf("RemoveLinkOnly dbLink: %v", err)
	}
	if err := os.MkdirAll(dbLink, 0o755); err != nil {
		t.Fatalf("mkdir local db dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dbLink, "local.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write local.txt: %v", err)
	}

	// Sync without --force must refuse to clobber non-empty local directory.
	_, err = Sync(ctx, SyncOptions{
		RepoDir: fx.primaryRoot,
		Target:  "raw-wt",
		CWD:     fx.primaryRoot,
		Force:   false,
	})
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when non-empty local dir exists without --force, got %v", err)
	}

	// Sync --all --force repairs it and syncs all non-primary worktrees.
	syncAllRes, err := Sync(ctx, SyncOptions{
		RepoDir: fx.primaryRoot,
		CWD:     fx.primaryRoot,
		All:     true,
		Force:   true,
	})
	if err != nil {
		t.Fatalf("Sync --all --force failed: %v", err)
	}
	if len(syncAllRes.Synced) != 1 || !syncAllRes.Synced[0].Status.Ready {
		t.Fatalf("expected 1 ready worktree after Sync --all --force, got %+v", syncAllRes)
	}
	assertWorktreeProvisioned(t, rawPath, fx.dataDir)
	fx.assertSentinelsIntact(t)
}

func mustGit(t *testing.T, ctx context.Context, dir string, args ...string) string {
	t.Helper()
	out, err := RunGit(ctx, dir, args...)
	if err != nil {
		t.Fatalf("git %v in %s failed: %v", args, dir, err)
	}
	return out
}
