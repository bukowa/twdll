package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"twdll/cli/internal/fslink"
	"twdll/cli/internal/term"
)

// AddOptions configures the creation and provisioning of a new git worktree.
type AddOptions struct {
	RepoDir string
	Name    string
	Base    string
	Path    string
	Detach  bool
	Force   bool
}

// AddResult describes a newly created and provisioned git worktree.
type AddResult struct {
	Worktree    GitWorktree
	PrimaryRoot string
	Status      LinkStatus
}

// RemoveOptions configures the safe teardown and removal of an existing git worktree.
type RemoveOptions struct {
	RepoDir      string
	Target       string
	CWD          string
	ExePath      string
	Force        bool
	DeleteBranch bool
}

// RemoveResult describes a removed git worktree and any deleted local branch.
type RemoveResult struct {
	Removed       GitWorktree
	DeletedBranch string
}

// SyncOptions configures the provisioning/repair of one or all linked worktrees.
type SyncOptions struct {
	RepoDir string
	Target  string
	CWD     string
	All     bool
	Force   bool
}

// SyncEntry records the updated link status of a synced git worktree.
type SyncEntry struct {
	Worktree GitWorktree
	Status   LinkStatus
}

// SyncResult describes all worktrees synced in a Sync operation.
type SyncResult struct {
	PrimaryRoot string
	Synced      []SyncEntry
}

// Add creates a git worktree anchored to the primary repository and provisions its .twdll
// directory links, twdll.toml, and state.json transactionally.
func Add(ctx context.Context, opts AddOptions) (AddResult, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return AddResult{}, &term.DiagnosticError{
			Message:  "worktree name must not be empty",
			Expected: "a branch or worktree name (e.g. 'twdll wt add feat-rome2')",
		}
	}

	list, err := List(ctx, opts.RepoDir)
	if err != nil {
		return AddResult{}, err
	}
	primary := list[0]

	primaryLayout, err := ResolvePrimaryLayout(primary.Path)
	if err != nil {
		return AddResult{}, err
	}

	var targetPath string
	if rawPath := strings.TrimSpace(opts.Path); rawPath != "" {
		if filepath.IsAbs(rawPath) {
			targetPath = filepath.Clean(rawPath)
		} else {
			baseDir := strings.TrimSpace(opts.RepoDir)
			if baseDir == "" {
				baseDir = primary.Path
			}
			targetPath = filepath.Clean(filepath.Join(baseDir, rawPath))
		}
	} else {
		targetPath, err = DefaultWorktreePath(primary.Path, name)
		if err != nil {
			return AddResult{}, err
		}
	}

	for _, existing := range list {
		if fslink.SamePath(existing.Path, targetPath) {
			return AddResult{}, &term.DiagnosticError{
				Message:  fmt.Sprintf("worktree already registered at %s", existing.Path),
				Expected: "a new worktree path",
				Help: []string{
					fmt.Sprintf("Run 'twdll wt sync %s' to repair an existing worktree, or choose a different name/path.", existing.Path),
				},
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return AddResult{}, fmt.Errorf("create worktree parent directory %s: %w", filepath.Dir(targetPath), err)
	}

	baseRef := strings.TrimSpace(opts.Base)
	var gitArgs []string
	gitArgs = append(gitArgs, "worktree", "add")
	if opts.Force {
		gitArgs = append(gitArgs, "--force")
	}

	if opts.Detach {
		ref := baseRef
		if ref == "" {
			ref = "HEAD"
		}
		gitArgs = append(gitArgs, "--detach", targetPath, ref)
	} else if BranchExists(ctx, primary.Path, name) {
		if baseRef != "" {
			return AddResult{}, &term.DiagnosticError{
				Message:  fmt.Sprintf("branch %q already exists; cannot combine existing branch with --base %q", name, baseRef),
				Expected: "either omit --base to check out the existing branch, or choose a new branch name",
			}
		}
		gitArgs = append(gitArgs, targetPath, name)
	} else {
		gitArgs = append(gitArgs, "-b", name, targetPath)
		if baseRef != "" {
			gitArgs = append(gitArgs, baseRef)
		}
	}

	if _, err := RunGit(ctx, primary.Path, gitArgs...); err != nil {
		return AddResult{}, err
	}

	if err := Provision(targetPath, primaryLayout, opts.Force); err != nil {
		_ = TeardownData(ctx, targetPath)
		_, _ = RunGit(ctx, primary.Path, "worktree", "remove", "--force", targetPath)
		return AddResult{}, err
	}

	updatedList, err := List(ctx, primary.Path)
	if err != nil {
		return AddResult{}, err
	}

	addedWT := GitWorktree{
		Path:     targetPath,
		Branch:   name,
		Detached: opts.Detach,
	}
	if opts.Detach {
		addedWT.Branch = ""
	}
	for _, wt := range updatedList {
		if fslink.SamePath(wt.Path, targetPath) {
			addedWT = wt
			break
		}
	}

	status := Inspect(addedWT.Path, primaryLayout.Paths.DataDir)
	return AddResult{
		Worktree:    addedWT,
		PrimaryRoot: primary.Path,
		Status:      status,
	}, nil
}

// Remove safely unlinks a worktree's .twdll directory without following junctions/symlinks
// and removes the worktree registration (and optionally its local branch).
func Remove(ctx context.Context, opts RemoveOptions) (RemoveResult, error) {
	list, err := List(ctx, opts.RepoDir)
	if err != nil {
		return RemoveResult{}, err
	}
	primary := list[0]

	wt, err := ResolveTarget(list, opts.Target, opts.CWD)
	if err != nil {
		return RemoveResult{}, err
	}

	if wt.IsPrimary {
		return RemoveResult{}, &term.DiagnosticError{
			Message:  fmt.Sprintf("refusing to remove primary worktree at %s", wt.Path),
			Expected: "a secondary linked worktree",
			Help: []string{
				"The primary worktree owns the shared .twdll data and repository metadata.",
			},
		}
	}

	if strings.TrimSpace(opts.CWD) != "" && IsPathWithinOrEqual(opts.CWD, wt.Path) {
		return RemoveResult{}, &term.DiagnosticError{
			Message:  fmt.Sprintf("refusing to remove worktree %s because the current working directory (%s) is inside it", wt.Path, opts.CWD),
			Expected: "current working directory outside the worktree being removed",
			Help: []string{
				fmt.Sprintf("Change directory to %s first, then re-run 'twdll wt remove'.", primary.Path),
			},
		}
	}

	if strings.TrimSpace(opts.ExePath) != "" && IsPathWithinOrEqual(opts.ExePath, wt.Path) {
		return RemoveResult{}, &term.DiagnosticError{
			Message:  fmt.Sprintf("refusing to remove worktree %s because the running executable (%s) resides inside it", wt.Path, opts.ExePath),
			Expected: "twdll invoked from outside the worktree being removed",
			Help: []string{
				fmt.Sprintf("Run twdll from %s to remove this worktree.", primary.Path),
			},
		}
	}

	_, statErr := os.Lstat(wt.Path)
	pathExists := statErr == nil

	if !opts.Force && !wt.Prunable && pathExists {
		if wt.Locked {
			reason := wt.LockReason
			if reason == "" {
				reason = "locked"
			}
			return RemoveResult{}, &term.DiagnosticError{
				Message:  fmt.Sprintf("worktree %s is locked (%s)", wt.Path, reason),
				Expected: "an unlocked worktree or --force",
				Help: []string{
					"Unlock the worktree with 'git worktree unlock' or pass '--force'.",
				},
			}
		}

		dirty, err := IsDirty(ctx, wt.Path)
		if err != nil {
			return RemoveResult{}, err
		}
		if dirty {
			return RemoveResult{}, &term.DiagnosticError{
				Message:  fmt.Sprintf("worktree %s has uncommitted or untracked changes", wt.Path),
				Expected: "a clean working tree or --force",
				Help: []string{
					"Commit or stash your changes, or pass '--force' to discard them.",
				},
			}
		}
	}

	// Unlink .twdll junctions/symlinks before `git worktree remove` touches the filesystem.
	if pathExists {
		if err := TeardownData(ctx, wt.Path); err != nil {
			return RemoveResult{}, err
		}
	}

	if wt.Prunable || !pathExists {
		if pathExists {
			if err := fslink.RemoveAllNoFollow(wt.Path); err != nil {
				return RemoveResult{}, err
			}
		}
		if _, err := RunGit(ctx, primary.Path, "worktree", "prune"); err != nil {
			return RemoveResult{}, err
		}
	} else {
		rmArgs := []string{"worktree", "remove"}
		if opts.Force {
			rmArgs = append(rmArgs, "--force")
			if wt.Locked {
				rmArgs = append(rmArgs, "--force")
			}
		}
		rmArgs = append(rmArgs, wt.Path)
		if _, err := RunGit(ctx, primary.Path, rmArgs...); err != nil {
			return RemoveResult{}, err
		}
	}

	res := RemoveResult{Removed: wt}
	if opts.DeleteBranch && wt.Branch != "" {
		branchFlag := "-d"
		if opts.Force {
			branchFlag = "-D"
		}
		if _, err := RunGit(ctx, primary.Path, "branch", branchFlag, wt.Branch); err != nil {
			return res, err
		}
		res.DeletedBranch = wt.Branch
	}

	return res, nil
}

// Sync provisions or repairs .twdll directory links, twdll.toml, and state.json for a target
// worktree, the current linked worktree, or all linked worktrees (--all).
func Sync(ctx context.Context, opts SyncOptions) (SyncResult, error) {
	if opts.All && strings.TrimSpace(opts.Target) != "" {
		return SyncResult{}, &term.DiagnosticError{
			Message:  "cannot specify both a target worktree and --all",
			Expected: "either 'twdll wt sync <target>' or 'twdll wt sync --all'",
		}
	}

	list, err := List(ctx, opts.RepoDir)
	if err != nil {
		return SyncResult{}, err
	}
	primary := list[0]

	primaryLayout, err := ResolvePrimaryLayout(primary.Path)
	if err != nil {
		return SyncResult{}, err
	}

	var targets []GitWorktree
	switch {
	case opts.All:
		for _, wt := range list {
			if wt.IsPrimary || wt.Prunable {
				continue
			}
			if fi, err := os.Stat(wt.Path); err == nil && fi.IsDir() {
				targets = append(targets, wt)
			}
		}
	case strings.TrimSpace(opts.Target) != "":
		wt, err := ResolveTarget(list, opts.Target, opts.CWD)
		if err != nil {
			return SyncResult{}, err
		}
		if wt.IsPrimary {
			return SyncResult{}, &term.DiagnosticError{
				Message:  fmt.Sprintf("worktree %s is the primary repository and cannot be synced as a linked worktree", wt.Path),
				Expected: "a secondary linked worktree or --all",
				Help: []string{
					"Run 'twdll wt sync --all' to sync all linked worktrees.",
				},
			}
		}
		if wt.Prunable {
			return SyncResult{}, &term.DiagnosticError{
				Message:  fmt.Sprintf("worktree %s is marked prunable and its directory is missing", wt.Path),
				Expected: "an existing linked worktree on disk",
				Help: []string{
					fmt.Sprintf("Run 'twdll wt remove %s' to clean up stale worktree metadata.", wt.Path),
				},
			}
		}
		if fi, err := os.Stat(wt.Path); err != nil || !fi.IsDir() {
			if errors.Is(err, os.ErrNotExist) {
				return SyncResult{}, &term.DiagnosticError{
					Message:  fmt.Sprintf("worktree directory %s does not exist on disk", wt.Path),
					Expected: "an existing worktree directory",
				}
			}
			return SyncResult{}, fmt.Errorf("stat worktree %s: %w", wt.Path, err)
		}
		targets = append(targets, wt)
	default:
		currentWT, found := findWorktreeContainingPath(list, opts.CWD)
		if !found || currentWT.IsPrimary {
			return SyncResult{}, &term.DiagnosticError{
				Message:  "cannot sync without a target when running from the primary worktree",
				Expected: "a target worktree name/path or --all",
				Help: []string{
					"Run 'twdll wt sync <target>' to sync a specific worktree, or 'twdll wt sync --all' to sync all linked worktrees.",
				},
			}
		}
		targets = append(targets, currentWT)
	}

	res := SyncResult{
		PrimaryRoot: primary.Path,
		Synced:      make([]SyncEntry, 0, len(targets)),
	}
	for _, wt := range targets {
		if err := Provision(wt.Path, primaryLayout, opts.Force); err != nil {
			return res, err
		}
		status := Inspect(wt.Path, primaryLayout.Paths.DataDir)
		res.Synced = append(res.Synced, SyncEntry{
			Worktree: wt,
			Status:   status,
		})
	}

	return res, nil
}

// IsPathWithinOrEqual reports whether candidatePath is equal to or nested inside rootDir.
func IsPathWithinOrEqual(candidatePath, rootDir string) bool {
	if strings.TrimSpace(candidatePath) == "" || strings.TrimSpace(rootDir) == "" {
		return false
	}

	cur, err := filepath.Abs(candidatePath)
	if err != nil {
		cur = filepath.Clean(candidatePath)
	} else {
		cur = filepath.Clean(cur)
	}

	for {
		if fslink.SamePath(cur, rootDir) {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return false
}

func findWorktreeContainingPath(list []GitWorktree, path string) (GitWorktree, bool) {
	if strings.TrimSpace(path) == "" {
		return GitWorktree{}, false
	}
	// Check non-primary worktrees first in case a worktree is nested inside the primary tree.
	for _, wt := range list {
		if !wt.IsPrimary && IsPathWithinOrEqual(path, wt.Path) {
			return wt, true
		}
	}
	for _, wt := range list {
		if wt.IsPrimary && IsPathWithinOrEqual(path, wt.Path) {
			return wt, true
		}
	}
	return GitWorktree{}, false
}
