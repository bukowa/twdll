package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"twdll/cli/internal/fslink"
	"twdll/cli/internal/term"
)

// GitWorktree represents a single worktree entry parsed from `git worktree list --porcelain`.
type GitWorktree struct {
	Path        string
	Head        string
	Branch      string // short branch name (refs/heads/ stripped) when on a branch
	Detached    bool
	Bare        bool
	Locked      bool
	Prunable    bool
	IsPrimary   bool
	LockReason  string
	PruneReason string
}

// ParsePorcelain parses the output of `git worktree list --porcelain`.
// It handles both LF and CRLF line endings cleanly and marks index 0 as IsPrimary.
// If the primary repository is bare, it returns a *term.DiagnosticError.
func ParsePorcelain(raw string) ([]GitWorktree, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	var list []GitWorktree
	var current *GitWorktree

	flush := func() {
		if current != nil && current.Path != "" {
			list = append(list, *current)
		}
		current = nil
	}

	for _, line := range lines {
		if line == "" {
			flush()
			continue
		}

		key, rest, hasSpace := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			wtPath := strings.TrimSpace(rest)
			if wtPath != "" {
				wtPath = filepath.Clean(filepath.FromSlash(wtPath))
			}
			current = &GitWorktree{
				Path: wtPath,
			}
		case "HEAD":
			if current != nil {
				current.Head = strings.TrimSpace(rest)
			}
		case "branch":
			if current != nil {
				ref := strings.TrimSpace(rest)
				current.Branch = strings.TrimPrefix(ref, "refs/heads/")
			}
		case "detached":
			if current != nil {
				current.Detached = true
			}
		case "bare":
			if current != nil {
				current.Bare = true
			}
		case "locked":
			if current != nil {
				current.Locked = true
				if hasSpace {
					current.LockReason = strings.TrimSpace(rest)
				}
			}
		case "prunable":
			if current != nil {
				current.Prunable = true
				if hasSpace {
					current.PruneReason = strings.TrimSpace(rest)
				}
			}
		}
	}
	flush()

	if len(list) > 0 {
		list[0].IsPrimary = true
		if list[0].Bare {
			return nil, &term.DiagnosticError{
				Message:  fmt.Sprintf("primary repository at %s is a bare git repository", list[0].Path),
				Expected: "a non-bare primary checkout containing the shared .twdll workspace",
				Help: []string{
					"Convert or clone a standard non-bare primary repository before using 'twdll wt'.",
				},
			}
		}
	}

	return list, nil
}

// List runs `git -C <repoDir> worktree list --porcelain` and returns all registered worktrees.
func List(ctx context.Context, repoDir string) ([]GitWorktree, error) {
	if strings.TrimSpace(repoDir) == "" {
		return nil, &term.DiagnosticError{
			Message: "repository directory must not be empty",
		}
	}
	out, err := RunGit(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	list, err := ParsePorcelain(out)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, &term.DiagnosticError{
			Message: fmt.Sprintf("no git worktrees found in %s", repoDir),
		}
	}
	return list, nil
}

// ResolveTarget matches query against list in priority order:
//  1. Path match via fslink.SamePath (resolving relative query against cwd)
//  2. Exact short Branch match
//  3. Directory basename match (filepath.Base(wt.Path))
//
// Returns *term.DiagnosticError if ambiguous or not found.
func ResolveTarget(list []GitWorktree, query, cwd string) (GitWorktree, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return GitWorktree{}, &term.DiagnosticError{
			Message:  "worktree target must not be empty",
			Expected: "a worktree path, branch name, or directory basename",
			Help: []string{
				"Run 'twdll wt list' to see registered worktrees.",
			},
		}
	}

	candidatePath := trimmed
	if !filepath.IsAbs(candidatePath) && strings.TrimSpace(cwd) != "" {
		candidatePath = filepath.Join(cwd, candidatePath)
	}
	candidatePath = filepath.Clean(candidatePath)

	// 1. Path match via fslink.SamePath.
	var pathMatches []GitWorktree
	for _, wt := range list {
		if fslink.SamePath(wt.Path, candidatePath) {
			pathMatches = append(pathMatches, wt)
		}
	}
	if len(pathMatches) == 1 {
		return pathMatches[0], nil
	}
	if len(pathMatches) > 1 {
		return GitWorktree{}, ambiguousTargetError(trimmed, pathMatches)
	}

	// 2. Exact short Branch match.
	var branchMatches []GitWorktree
	for _, wt := range list {
		if wt.Branch != "" && wt.Branch == trimmed {
			branchMatches = append(branchMatches, wt)
		}
	}
	if len(branchMatches) == 1 {
		return branchMatches[0], nil
	}
	if len(branchMatches) > 1 {
		return GitWorktree{}, ambiguousTargetError(trimmed, branchMatches)
	}

	// 3. Directory basename match.
	var baseMatches []GitWorktree
	for _, wt := range list {
		if filepath.Base(wt.Path) == trimmed {
			baseMatches = append(baseMatches, wt)
		}
	}
	if len(baseMatches) == 1 {
		return baseMatches[0], nil
	}
	if len(baseMatches) > 1 {
		return GitWorktree{}, ambiguousTargetError(trimmed, baseMatches)
	}

	return GitWorktree{}, &term.DiagnosticError{
		Message:  fmt.Sprintf("worktree %q not found", trimmed),
		Expected: "an existing worktree path, branch name, or directory basename",
		Help: []string{
			"Run 'twdll wt list' to view registered worktrees.",
		},
	}
}

func ambiguousTargetError(query string, matches []GitWorktree) error {
	paths := make([]string, 0, len(matches))
	for _, m := range matches {
		paths = append(paths, m.Path)
	}
	return &term.DiagnosticError{
		Message:  fmt.Sprintf("worktree target %q is ambiguous (matched %d worktrees: %s)", query, len(matches), strings.Join(paths, ", ")),
		Expected: "an unambiguous worktree path or branch name",
		Help: []string{
			"Specify the full worktree path to disambiguate.",
		},
	}
}

// DefaultWorktreePath computes `<parent>/<repo>.wt/<slug>` from primaryRoot and name.
// It replaces `/` and `\` with `-`, trims leading/trailing whitespace and `-`, and
// rejects empty names, `.`, `..`, colons, invalid path characters, or ASCII control chars.
func DefaultWorktreePath(primaryRoot, name string) (string, error) {
	if strings.TrimSpace(primaryRoot) == "" {
		return "", &term.DiagnosticError{
			Message: "primary repository root must not be empty",
		}
	}

	trimmed := strings.TrimSpace(name)
	if trimmed == "" || trimmed == "." || trimmed == ".." {
		return "", invalidWorktreeNameError(name)
	}

	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`:<>|"?*`, r) {
			return "", invalidWorktreeNameError(name)
		}
	}

	// Reject any `.` or `..` path segment before slugging.
	normalizedSlashes := strings.ReplaceAll(trimmed, `\`, `/`)
	for _, seg := range strings.Split(normalizedSlashes, "/") {
		if seg == "." || seg == ".." {
			return "", invalidWorktreeNameError(name)
		}
	}

	slug := strings.ReplaceAll(normalizedSlashes, "/", "-")
	slug = strings.Trim(slug, " \t\r\n-")
	if slug == "" || slug == "." || slug == ".." || strings.ContainsAny(slug, `/\:`) {
		return "", invalidWorktreeNameError(name)
	}

	cleanPrimary := filepath.Clean(primaryRoot)
	parentDir := filepath.Dir(cleanPrimary)
	baseName := filepath.Base(cleanPrimary)
	if baseName == "" || baseName == "." || baseName == string(filepath.Separator) {
		return "", &term.DiagnosticError{
			Message: fmt.Sprintf("cannot derive default worktree container from primary root %q", primaryRoot),
		}
	}

	return filepath.Join(parentDir, baseName+".wt", slug), nil
}

func invalidWorktreeNameError(name string) error {
	return &term.DiagnosticError{
		Message:  fmt.Sprintf("invalid worktree or branch name %q", name),
		Expected: "a valid branch or directory name (without '..', colons, or control characters)",
	}
}

// IsDirty reports whether wtPath has uncommitted or untracked changes according to
// `git -C <wtPath> status --porcelain --untracked-files=normal`, ignoring untracked
// `.twdll/`, `.codegraph/`, and `twdll.toml` artifacts managed by twdll itself.
func IsDirty(ctx context.Context, wtPath string) (bool, error) {
	out, err := RunGit(ctx, wtPath, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return false, err
	}
	normalized := strings.ReplaceAll(out, "\r\n", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line == "?? .twdll" || line == "?? .twdll/" || strings.HasPrefix(line, "?? .twdll/") ||
			line == "?? .codegraph" || line == "?? .codegraph/" || strings.HasPrefix(line, "?? .codegraph/") ||
			line == "?? twdll.toml" {
			continue
		}
		return true, nil
	}
	return false, nil
}

// IsTrackedFile reports whether relPath is tracked by git in wtPath.
func IsTrackedFile(ctx context.Context, wtPath, relPath string) bool {
	if strings.TrimSpace(wtPath) == "" || strings.TrimSpace(relPath) == "" {
		return false
	}
	_, err := RunGit(ctx, wtPath, "ls-files", "--error-unmatch", "--", relPath)
	return err == nil
}

// BranchExists reports whether a local branch `refs/heads/<branch>` exists in repoDir.
func BranchExists(ctx context.Context, repoDir, branch string) bool {
	if strings.TrimSpace(repoDir) == "" || strings.TrimSpace(branch) == "" {
		return false
	}
	_, err := RunGit(ctx, repoDir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// RunGit executes a git subprocess with GIT_TERMINAL_PROMPT=0 and stdin disconnected.
func RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	var fullArgs []string
	if strings.TrimSpace(dir) != "" {
		fullArgs = append(fullArgs, "-C", dir)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	cmd.Stdin = nil

	env := os.Environ()
	filtered := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "GIT_TERMINAL_PROMPT=") {
			filtered = append(filtered, kv)
		}
	}
	filtered = append(filtered, "GIT_TERMINAL_PROMPT=0")
	cmd.Env = filtered

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, detail)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return stdout.String(), nil
}
