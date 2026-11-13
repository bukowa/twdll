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
	"twdll/cli/internal/workspace"
)

// SharedSubdirs lists the .twdll subdirectories that are directory-linked from a worktree
// to the primary workspace's resolved DataDir.
var SharedSubdirs = []string{"tools", "binaries", "db", "extracted"}

// LocalSubdirs lists the .twdll subdirectories that remain real local directories per worktree.
var LocalSubdirs = []string{"bin", "logs"}

// LinkStatus summarizes the provisioning health of a worktree's .twdll directory and config.
type LinkStatus struct {
	Ready           bool
	MissingLinks    []string
	MismatchedLinks []string
	HasConfig       bool
	HasState        bool
}

// ResolvePrimaryLayout resolves the workspace Layout strictly anchored at primaryRoot,
// never inheriting a linked worktree's working directory or config path.
func ResolvePrimaryLayout(primaryRoot string) (workspace.Layout, error) {
	cleanPrimary := filepath.Clean(primaryRoot)
	return workspace.Resolve(workspace.ResolveSpec{
		RootOverride:   cleanPrimary,
		ConfigOverride: filepath.Join(cleanPrimary, "twdll.toml"),
		WorkingDir:     cleanPrimary,
	})
}

// Provision configures `<wtPath>/.twdll` with local `{bin,logs}` directories, directory links
// `{tools,binaries,db,extracted}` targeting primaryLayout.Paths.DataDir, and seeded `twdll.toml`
// (with `DataDir` cleared) and `.twdll/state.json` copies when missing or when force is true.
func Provision(wtPath string, primaryLayout workspace.Layout, force bool) error {
	if strings.TrimSpace(wtPath) == "" {
		return &term.DiagnosticError{Message: "worktree path must not be empty"}
	}
	primaryDataDir := strings.TrimSpace(primaryLayout.Paths.DataDir)
	if primaryDataDir == "" {
		return &term.DiagnosticError{Message: "primary workspace DataDir must not be empty"}
	}

	cleanWT := filepath.Clean(wtPath)
	cleanPrimaryData := filepath.Clean(primaryDataDir)
	wtDotDir := filepath.Join(cleanWT, ".twdll")

	if fslink.SamePath(wtDotDir, cleanPrimaryData) {
		return &term.DiagnosticError{
			Message:  fmt.Sprintf("cannot provision worktree links at %s because it is the primary data directory", wtDotDir),
			Expected: "a secondary git worktree distinct from the primary workspace data directory",
		}
	}

	// 1. Ensure shared subdirectories exist in the primary DataDir.
	for _, sub := range SharedSubdirs {
		target := filepath.Join(cleanPrimaryData, sub)
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("create primary shared directory %s: %w", target, err)
		}
	}

	// 2. Ensure <wtPath>/.twdll exists as a real non-link directory.
	if err := ensureRealDir(wtDotDir, force); err != nil {
		return err
	}

	// 3. Ensure <wtPath>/.twdll/{bin,logs} exist as real non-link directories.
	for _, sub := range LocalSubdirs {
		localDir := filepath.Join(wtDotDir, sub)
		if err := ensureRealDir(localDir, force); err != nil {
			return err
		}
	}

	// 4. Provision directory links for each shared subdirectory.
	for _, sub := range SharedSubdirs {
		target := filepath.Join(cleanPrimaryData, sub)
		link := filepath.Join(wtDotDir, sub)
		if err := provisionSharedLink(target, link, sub, force); err != nil {
			return err
		}
	}

	// 5. Seed <wtPath>/twdll.toml when primary config exists (if missing or force).
	if primaryLayout.ConfigExists {
		wtConfigPath := filepath.Join(cleanWT, "twdll.toml")
		if err := seedWorktreeConfig(wtConfigPath, primaryLayout.Config, force); err != nil {
			return err
		}
	}

	// 6. Seed <wtPath>/.twdll/state.json when primary state.json exists (if missing or force).
	if primaryState := strings.TrimSpace(primaryLayout.Paths.StateFile); primaryState != "" {
		if fi, err := os.Stat(primaryState); err == nil && !fi.IsDir() {
			wtStatePath := filepath.Join(wtDotDir, "state.json")
			if err := seedWorktreeState(primaryState, wtStatePath, force); err != nil {
				return err
			}
		}
	}

	return nil
}

func ensureRealDir(dirPath string, force bool) error {
	fi, err := os.Lstat(dirPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.MkdirAll(dirPath, 0o755)
		}
		return fmt.Errorf("stat %s: %w", dirPath, err)
	}

	isLink, err := fslink.IsLinkOrReparsePoint(dirPath)
	if err != nil {
		return fmt.Errorf("check link status of %s: %w", dirPath, err)
	}
	if isLink {
		if !force {
			return &term.DiagnosticError{
				Message:  fmt.Sprintf("%s is a symlink or junction rather than a local directory", dirPath),
				Expected: "a real local directory",
				Help: []string{
					"Re-run with '--force' to unlink and recreate it as a local directory.",
				},
			}
		}
		if err := fslink.RemoveLinkOnly(dirPath); err != nil {
			return err
		}
		return os.MkdirAll(dirPath, 0o755)
	}

	if !fi.IsDir() {
		if !force {
			return &term.DiagnosticError{
				Message:  fmt.Sprintf("%s exists as a file rather than a directory", dirPath),
				Expected: "a directory",
				Help: []string{
					"Remove the file or re-run with '--force'.",
				},
			}
		}
		if err := os.Remove(dirPath); err != nil {
			return fmt.Errorf("remove file %s: %w", dirPath, err)
		}
		return os.MkdirAll(dirPath, 0o755)
	}

	return nil
}

func provisionSharedLink(target, link, sub string, force bool) error {
	fi, err := os.Lstat(link)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fslink.EnsureDirLink(target, link)
		}
		return fmt.Errorf("stat %s: %w", link, err)
	}

	isLink, err := fslink.IsLinkOrReparsePoint(link)
	if err != nil {
		return fmt.Errorf("check link status of %s: %w", link, err)
	}
	if isLink {
		if current, err := fslink.ReadDirLink(link); err == nil && fslink.SamePath(current, target) {
			if _, statErr := os.Stat(link); statErr == nil {
				return nil
			}
		}
		if err := fslink.RemoveLinkOnly(link); err != nil {
			return err
		}
		return fslink.EnsureDirLink(target, link)
	}

	if fi.IsDir() {
		entries, err := os.ReadDir(link)
		if err != nil {
			return fmt.Errorf("read directory %s: %w", link, err)
		}
		if len(entries) == 0 {
			if err := os.Remove(link); err != nil {
				return fmt.Errorf("remove empty directory %s: %w", link, err)
			}
			return fslink.EnsureDirLink(target, link)
		}
		if !force {
			return &term.DiagnosticError{
				Message:  fmt.Sprintf("refusing to overwrite non-empty local directory %s (.twdll/%s)", link, sub),
				Expected: fmt.Sprintf("an empty directory or directory link targeting %s", target),
				Help: []string{
					"Move any local files you want to keep, or pass '--force' to replace the directory with a link.",
				},
			}
		}
		if err := fslink.RemoveAllNoFollow(link); err != nil {
			return fmt.Errorf("remove local directory %s: %w", link, err)
		}
		return fslink.EnsureDirLink(target, link)
	}

	// Existing regular file at link path.
	if !force {
		return &term.DiagnosticError{
			Message:  fmt.Sprintf("refusing to overwrite existing file at %s", link),
			Expected: fmt.Sprintf("a directory link targeting %s", target),
			Help: []string{
				"Remove the file or pass '--force' to replace it with a directory link.",
			},
		}
	}
	if err := os.Remove(link); err != nil {
		return fmt.Errorf("remove file %s: %w", link, err)
	}
	return fslink.EnsureDirLink(target, link)
}

func seedWorktreeConfig(wtConfigPath string, primaryCfg workspace.Config, force bool) error {
	_, err := os.Lstat(wtConfigPath)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", wtConfigPath, err)
	}
	if exists && !force {
		return nil
	}
	if exists {
		if isLink, lerr := fslink.IsLinkOrReparsePoint(wtConfigPath); lerr == nil && isLink {
			if err := fslink.RemoveLinkOnly(wtConfigPath); err != nil {
				return err
			}
		}
	}
	cfg := primaryCfg
	cfg.DataDir = ""
	return workspace.SaveConfig(wtConfigPath, cfg)
}

func seedWorktreeState(primaryStatePath, wtStatePath string, force bool) error {
	_, err := os.Lstat(wtStatePath)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", wtStatePath, err)
	}
	if exists && !force {
		return nil
	}
	if exists {
		if isLink, lerr := fslink.IsLinkOrReparsePoint(wtStatePath); lerr == nil && isLink {
			if err := fslink.RemoveLinkOnly(wtStatePath); err != nil {
				return err
			}
		}
	}
	return workspace.CopyFileAtomic(primaryStatePath, wtStatePath)
}

// Inspect performs a non-mutating check of a worktree's shared links, local subdirectories,
// twdll.toml, and .twdll/state.json.
func Inspect(wtPath, primaryDataDir string) LinkStatus {
	var status LinkStatus
	if strings.TrimSpace(wtPath) == "" || strings.TrimSpace(primaryDataDir) == "" {
		return status
	}

	cleanWT := filepath.Clean(wtPath)
	cleanPrimaryData := filepath.Clean(primaryDataDir)
	wtDotDir := filepath.Join(cleanWT, ".twdll")

	if fi, err := os.Stat(filepath.Join(cleanWT, "twdll.toml")); err == nil && !fi.IsDir() {
		status.HasConfig = true
	}
	if fi, err := os.Stat(filepath.Join(wtDotDir, "state.json")); err == nil && !fi.IsDir() {
		status.HasState = true
	}

	localReady := true
	if dotFi, err := os.Lstat(wtDotDir); err != nil || !dotFi.IsDir() {
		localReady = false
	} else if isLink, err := fslink.IsLinkOrReparsePoint(wtDotDir); err != nil || isLink {
		localReady = false
	}

	for _, sub := range LocalSubdirs {
		localDir := filepath.Join(wtDotDir, sub)
		fi, err := os.Lstat(localDir)
		if err != nil || !fi.IsDir() {
			localReady = false
			continue
		}
		if isLink, err := fslink.IsLinkOrReparsePoint(localDir); err != nil || isLink {
			localReady = false
		}
	}

	for _, sub := range SharedSubdirs {
		expectedTarget := filepath.Join(cleanPrimaryData, sub)
		linkPath := filepath.Join(wtDotDir, sub)

		if _, err := os.Lstat(linkPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				status.MissingLinks = append(status.MissingLinks, sub)
			} else {
				status.MismatchedLinks = append(status.MismatchedLinks, sub)
			}
			continue
		}

		isLink, err := fslink.IsLinkOrReparsePoint(linkPath)
		if err != nil || !isLink {
			status.MismatchedLinks = append(status.MismatchedLinks, sub)
			continue
		}

		actualTarget, err := fslink.ReadDirLink(linkPath)
		if err != nil || !fslink.SamePath(actualTarget, expectedTarget) {
			status.MismatchedLinks = append(status.MismatchedLinks, sub)
			continue
		}

		if fi, err := os.Stat(linkPath); err != nil || !fi.IsDir() {
			status.MismatchedLinks = append(status.MismatchedLinks, sub)
		}
	}

	status.Ready = localReady && len(status.MissingLinks) == 0 && len(status.MismatchedLinks) == 0
	return status
}

// TeardownData safely removes `<wtPath>/.twdll` without traversing into any directory links
// and removes `<wtPath>/twdll.toml` if it is not tracked by git.
func TeardownData(ctx context.Context, wtPath string) error {
	if strings.TrimSpace(wtPath) == "" {
		return nil
	}
	cleanWT := filepath.Clean(wtPath)
	wtDotDir := filepath.Join(cleanWT, ".twdll")

	if _, err := os.Lstat(wtDotDir); err == nil {
		isLink, lerr := fslink.IsLinkOrReparsePoint(wtDotDir)
		if lerr != nil {
			return lerr
		}
		if isLink {
			if err := fslink.RemoveLinkOnly(wtDotDir); err != nil {
				return err
			}
		} else {
			if err := fslink.RemoveAllNoFollow(wtDotDir); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	cfgPath := filepath.Join(cleanWT, "twdll.toml")
	if _, err := os.Lstat(cfgPath); err == nil {
		if !IsTrackedFile(ctx, cleanWT, "twdll.toml") {
			isLink, lerr := fslink.IsLinkOrReparsePoint(cfgPath)
			if lerr != nil {
				return lerr
			}
			if isLink {
				if err := fslink.RemoveLinkOnly(cfgPath); err != nil {
					return err
				}
			} else if err := os.Remove(cfgPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
