package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"twdll/cli/internal/term"
)

type fakeCodeGraphRunner struct {
	available      bool
	availableCalls int
	initCalls      []string
	syncCalls      []string
	uninitCalls    []string
	initFn         func(ctx context.Context, wtPath string) error
	syncFn         func(ctx context.Context, wtPath string) error
	uninitFn       func(ctx context.Context, wtPath string) error
}

func (f *fakeCodeGraphRunner) Available() bool {
	f.availableCalls++
	return f.available
}

func (f *fakeCodeGraphRunner) Init(ctx context.Context, wtPath string) error {
	f.initCalls = append(f.initCalls, wtPath)
	if f.initFn != nil {
		return f.initFn(ctx, wtPath)
	}
	dotCG := filepath.Join(wtPath, ".codegraph")
	if err := os.MkdirAll(dotCG, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dotCG, "codegraph.db"), []byte("sqlite-mock"), 0o644)
}

func (f *fakeCodeGraphRunner) Sync(ctx context.Context, wtPath string) error {
	f.syncCalls = append(f.syncCalls, wtPath)
	if f.syncFn != nil {
		return f.syncFn(ctx, wtPath)
	}
	return nil
}

func (f *fakeCodeGraphRunner) Uninit(ctx context.Context, wtPath string) error {
	f.uninitCalls = append(f.uninitCalls, wtPath)
	if f.uninitFn != nil {
		return f.uninitFn(ctx, wtPath)
	}
	return os.RemoveAll(filepath.Join(wtPath, ".codegraph"))
}

func TestCodeGraph_AddAndSyncBehavior(t *testing.T) {
	ctx := context.Background()
	fx := setupTestRepo(t, "")

	// Ensure .codegraph/ is gitignored in the test repo just like a real repo.
	gitignorePath := filepath.Join(fx.primaryRoot, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(".codegraph/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	mustGit(t, ctx, fx.primaryRoot, "add", ".gitignore")
	mustGit(t, ctx, fx.primaryRoot, "commit", "-m", "ignore .codegraph")

	// 1. Add when <primary>/.codegraph is absent -> runner never called, Status.HasCodeGraph == false.
	runner1 := &fakeCodeGraphRunner{available: true}
	res1, err := Add(ctx, AddOptions{
		RepoDir:         fx.primaryRoot,
		Name:            "feat/no-primary-cg",
		CodeGraphRunner: runner1,
	})
	if err != nil {
		t.Fatalf("Add without primary .codegraph failed: %v", err)
	}
	if len(runner1.initCalls) != 0 || len(runner1.syncCalls) != 0 || len(runner1.uninitCalls) != 0 {
		t.Fatalf("expected zero runner calls when primary .codegraph is absent, got init=%v sync=%v uninit=%v",
			runner1.initCalls, runner1.syncCalls, runner1.uninitCalls)
	}
	if res1.Status.HasCodeGraph {
		t.Fatalf("expected Status.HasCodeGraph == false when primary .codegraph is absent")
	}

	// Create <primary>/.codegraph/codegraph.db to simulate an indexed primary workspace.
	primaryCG := filepath.Join(fx.primaryRoot, ".codegraph")
	if err := os.MkdirAll(primaryCG, 0o755); err != nil {
		t.Fatalf("mkdir primary .codegraph: %v", err)
	}
	if err := os.WriteFile(filepath.Join(primaryCG, "codegraph.db"), []byte("primary-db"), 0o644); err != nil {
		t.Fatalf("write primary codegraph.db: %v", err)
	}

	// 2. Add when <primary>/.codegraph exists + fake Available() == true -> auto-runs Init, sets Status.HasCodeGraph == true.
	runner2 := &fakeCodeGraphRunner{available: true}
	res2, err := Add(ctx, AddOptions{
		RepoDir:         fx.primaryRoot,
		Name:            "feat/auto-cg",
		CodeGraphRunner: runner2,
	})
	if err != nil {
		t.Fatalf("Add with primary .codegraph failed: %v", err)
	}
	if len(runner2.initCalls) != 1 || runner2.initCalls[0] != filepath.Clean(res2.Worktree.Path) {
		t.Fatalf("expected 1 Init call for %s, got %v", res2.Worktree.Path, runner2.initCalls)
	}
	if !res2.Status.HasCodeGraph || !res2.Status.Ready {
		t.Fatalf("expected HasCodeGraph=true and Ready=true, got %+v", res2.Status)
	}

	// 3. Add when <primary>/.codegraph exists + NoCodeGraph: true -> runner never called.
	runner3 := &fakeCodeGraphRunner{available: true}
	res3, err := Add(ctx, AddOptions{
		RepoDir:         fx.primaryRoot,
		Name:            "feat/skip-cg",
		NoCodeGraph:     true,
		CodeGraphRunner: runner3,
	})
	if err != nil {
		t.Fatalf("Add with NoCodeGraph failed: %v", err)
	}
	if len(runner3.initCalls) != 0 || len(runner3.syncCalls) != 0 {
		t.Fatalf("expected zero runner calls when NoCodeGraph=true, got init=%v sync=%v", runner3.initCalls, runner3.syncCalls)
	}
	if res3.Status.HasCodeGraph || !res3.Status.Ready {
		t.Fatalf("expected HasCodeGraph=false and Ready=true when NoCodeGraph=true, got %+v", res3.Status)
	}

	// 4. Add when Init fails after creating partial .codegraph/ -> transactional rollback calls Uninit,
	// unlinks .twdll, and unregisters git worktree.
	runner4 := &fakeCodeGraphRunner{
		available: true,
		initFn: func(_ context.Context, wtPath string) error {
			dotCG := filepath.Join(wtPath, ".codegraph")
			_ = os.MkdirAll(dotCG, 0o755)
			_ = os.WriteFile(filepath.Join(dotCG, "partial.db"), []byte("partial"), 0o644)
			return errors.New("simulated codegraph init failure")
		},
	}
	failedWTPath, err := DefaultWorktreePath(fx.primaryRoot, "feat/fail-cg")
	if err != nil {
		t.Fatalf("DefaultWorktreePath: %v", err)
	}
	_, err = Add(ctx, AddOptions{
		RepoDir:         fx.primaryRoot,
		Name:            "feat/fail-cg",
		CodeGraphRunner: runner4,
	})
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError on Init failure, got %v", err)
	}
	if len(runner4.uninitCalls) != 1 || runner4.uninitCalls[0] != filepath.Clean(failedWTPath) {
		t.Fatalf("expected rollback to call Uninit(%s), got %v", failedWTPath, runner4.uninitCalls)
	}
	if _, statErr := os.Lstat(failedWTPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected rolled-back worktree directory %s to be removed, stat err = %v", failedWTPath, statErr)
	}
	listAfterFail, err := List(ctx, fx.primaryRoot)
	if err != nil {
		t.Fatalf("List after rollback: %v", err)
	}
	for _, wt := range listAfterFail {
		if filepath.Clean(wt.Path) == filepath.Clean(failedWTPath) {
			t.Fatalf("expected failed worktree %s to be unregistered from git worktree list", failedWTPath)
		}
	}
	fx.assertSentinelsIntact(t)

	// 5. Sync auto-runs Init when codegraph.db absent (res3) and Sync when codegraph.db present (res2);
	// skips when NoCodeGraph: true.
	runnerSyncSkip := &fakeCodeGraphRunner{available: true}
	if _, err := Sync(ctx, SyncOptions{
		RepoDir:         fx.primaryRoot,
		CWD:             fx.primaryRoot,
		All:             true,
		NoCodeGraph:     true,
		CodeGraphRunner: runnerSyncSkip,
	}); err != nil {
		t.Fatalf("Sync --all --no-codegraph failed: %v", err)
	}
	if len(runnerSyncSkip.initCalls) != 0 || len(runnerSyncSkip.syncCalls) != 0 {
		t.Fatalf("expected zero runner calls on Sync with NoCodeGraph=true, got init=%v sync=%v",
			runnerSyncSkip.initCalls, runnerSyncSkip.syncCalls)
	}

	runnerSync := &fakeCodeGraphRunner{available: true}
	// Sync res3 (no codegraph.db yet -> calls Init)
	syncRes3, err := Sync(ctx, SyncOptions{
		RepoDir:         fx.primaryRoot,
		Target:          "feat/skip-cg",
		CWD:             fx.primaryRoot,
		CodeGraphRunner: runnerSync,
	})
	if err != nil {
		t.Fatalf("Sync feat/skip-cg failed: %v", err)
	}
	if len(runnerSync.initCalls) != 1 || len(runnerSync.syncCalls) != 0 {
		t.Fatalf("expected 1 Init and 0 Sync calls for feat/skip-cg, got init=%v sync=%v",
			runnerSync.initCalls, runnerSync.syncCalls)
	}
	if !syncRes3.Synced[0].Status.HasCodeGraph {
		t.Fatalf("expected HasCodeGraph=true after Sync initialized CodeGraph")
	}

	// Sync res2 (already has codegraph.db -> calls Sync)
	runnerSync2 := &fakeCodeGraphRunner{available: true}
	syncRes2, err := Sync(ctx, SyncOptions{
		RepoDir:         fx.primaryRoot,
		Target:          "feat/auto-cg",
		CWD:             fx.primaryRoot,
		CodeGraphRunner: runnerSync2,
	})
	if err != nil {
		t.Fatalf("Sync feat/auto-cg failed: %v", err)
	}
	if len(runnerSync2.syncCalls) != 1 || len(runnerSync2.initCalls) != 0 {
		t.Fatalf("expected 1 Sync and 0 Init calls for feat/auto-cg, got init=%v sync=%v",
			runnerSync2.initCalls, runnerSync2.syncCalls)
	}
	if !syncRes2.Synced[0].Status.HasCodeGraph {
		t.Fatalf("expected HasCodeGraph=true after Sync")
	}
}

func TestCodeGraph_RemoveTeardownAndLockSafety(t *testing.T) {
	ctx := context.Background()
	fx := setupTestRepo(t, "")

	gitignorePath := filepath.Join(fx.primaryRoot, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(".codegraph/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	mustGit(t, ctx, fx.primaryRoot, "add", ".gitignore")
	mustGit(t, ctx, fx.primaryRoot, "commit", "-m", "ignore .codegraph")

	primaryCG := filepath.Join(fx.primaryRoot, ".codegraph")
	if err := os.MkdirAll(primaryCG, 0o755); err != nil {
		t.Fatalf("mkdir primary .codegraph: %v", err)
	}
	if err := os.WriteFile(filepath.Join(primaryCG, "codegraph.db"), []byte("primary-db"), 0o644); err != nil {
		t.Fatalf("write primary codegraph.db: %v", err)
	}

	// 6. Remove (WITHOUT --force!) when gitignored <wt>/.codegraph/codegraph.db exists ->
	// Uninit runs, .codegraph/ is removed, and `git worktree remove` succeeds without --force.
	addRunner := &fakeCodeGraphRunner{available: true}
	wt1, err := Add(ctx, AddOptions{
		RepoDir:         fx.primaryRoot,
		Name:            "feat/rm-with-cg",
		CodeGraphRunner: addRunner,
	})
	if err != nil {
		t.Fatalf("Add feat/rm-with-cg failed: %v", err)
	}
	if !wt1.Status.HasCodeGraph {
		t.Fatalf("expected wt1 to have CodeGraph initialized")
	}

	rmRunner1 := &fakeCodeGraphRunner{available: true}
	if _, err := Remove(ctx, RemoveOptions{
		RepoDir:         fx.primaryRoot,
		Target:          "feat/rm-with-cg",
		CWD:             fx.primaryRoot,
		Force:           false,
		DeleteBranch:    true,
		CodeGraphRunner: rmRunner1,
	}); err != nil {
		t.Fatalf("Remove without --force failed when .codegraph existed: %v", err)
	}
	if len(rmRunner1.uninitCalls) != 1 || rmRunner1.uninitCalls[0] != filepath.Clean(wt1.Worktree.Path) {
		t.Fatalf("expected 1 Uninit call for %s, got %v", wt1.Worktree.Path, rmRunner1.uninitCalls)
	}
	if _, statErr := os.Lstat(wt1.Worktree.Path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected worktree %s to be completely removed, stat err = %v", wt1.Worktree.Path, statErr)
	}
	fx.assertSentinelsIntact(t)

	// 7. Remove when Available() == false and unlocked <wt>/.codegraph/ exists ->
	// RemoveAllNoFollow cleans .codegraph/ and removal succeeds without --force.
	wt2, err := Add(ctx, AddOptions{
		RepoDir:         fx.primaryRoot,
		Name:            "feat/rm-no-cli",
		CodeGraphRunner: addRunner,
	})
	if err != nil {
		t.Fatalf("Add feat/rm-no-cli failed: %v", err)
	}
	rmRunnerUnavailable := &fakeCodeGraphRunner{available: false}
	if _, err := Remove(ctx, RemoveOptions{
		RepoDir:         fx.primaryRoot,
		Target:          "feat/rm-no-cli",
		CWD:             fx.primaryRoot,
		Force:           false,
		DeleteBranch:    true,
		CodeGraphRunner: rmRunnerUnavailable,
	}); err != nil {
		t.Fatalf("Remove when codegraph CLI unavailable failed: %v", err)
	}
	if len(rmRunnerUnavailable.uninitCalls) != 0 {
		t.Fatalf("expected 0 Uninit calls when Available()==false, got %v", rmRunnerUnavailable.uninitCalls)
	}
	if _, statErr := os.Lstat(wt2.Worktree.Path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected worktree %s to be completely removed, stat err = %v", wt2.Worktree.Path, statErr)
	}
	fx.assertSentinelsIntact(t)

	// 8. Remove when TeardownCodeGraph cannot clear .codegraph/ ->
	// returns *term.DiagnosticError and leaves .twdll/ links untouched!
	wt3, err := Add(ctx, AddOptions{
		RepoDir:         fx.primaryRoot,
		Name:            "feat/rm-locked-cg",
		CodeGraphRunner: addRunner,
	})
	if err != nil {
		t.Fatalf("Add feat/rm-locked-cg failed: %v", err)
	}
	dotCG3 := filepath.Join(wt3.Worktree.Path, ".codegraph")
	dbPath3 := filepath.Join(dotCG3, "codegraph.db")
	// Make codegraph.db read-only (blocks os.Remove on Windows) and .codegraph read-only (blocks unlink on POSIX).
	if err := os.Chmod(dbPath3, 0o444); err != nil {
		t.Fatalf("chmod codegraph.db: %v", err)
	}
	if err := os.Chmod(dotCG3, 0o555); err != nil {
		t.Fatalf("chmod .codegraph: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dotCG3, 0o755)
		_ = os.Chmod(dbPath3, 0o644)
	})

	rmRunnerLocked := &fakeCodeGraphRunner{
		available: true,
		uninitFn: func(_ context.Context, _ string) error {
			return errors.New("database is locked by background daemon")
		},
	}
	_, err = Remove(ctx, RemoveOptions{
		RepoDir:         fx.primaryRoot,
		Target:          "feat/rm-locked-cg",
		CWD:             fx.primaryRoot,
		Force:           false,
		CodeGraphRunner: rmRunnerLocked,
	})
	var diag *term.DiagnosticError
	if !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError when .codegraph cannot be removed, got %v", err)
	}
	// Verify .twdll/ links and twdll.toml were left 100% intact because TeardownCodeGraph aborted before TeardownData!
	assertWorktreeProvisioned(t, wt3.Worktree.Path, fx.dataDir)
	fx.assertSentinelsIntact(t)
}
