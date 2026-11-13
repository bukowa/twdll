package fslink

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EnsureDirLink creates a directory junction (on Windows) or directory symlink (on POSIX)
// at linkPath pointing to targetDir.
func EnsureDirLink(targetDir, linkPath string) error {
	if strings.TrimSpace(targetDir) == "" {
		return errors.New("fslink: targetDir must not be empty")
	}
	if strings.TrimSpace(linkPath) == "" {
		return errors.New("fslink: linkPath must not be empty")
	}

	targetAbs, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("fslink: resolve target path %q: %w", targetDir, err)
	}
	targetAbs = filepath.Clean(targetAbs)

	fi, err := os.Stat(targetAbs)
	if err != nil {
		return fmt.Errorf("fslink: stat target %q: %w", targetAbs, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("fslink: target %q is not a directory", targetAbs)
	}

	cleanLink := filepath.Clean(linkPath)
	parentDir := filepath.Dir(cleanLink)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("fslink: create parent directory %q: %w", parentDir, err)
	}

	return createDirLink(targetAbs, cleanLink)
}

// ReadDirLink reads the target of a directory symlink or Windows junction at linkPath,
// resolving relative targets against filepath.Dir(linkPath).
func ReadDirLink(linkPath string) (string, error) {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		target = strings.TrimPrefix(target, `\??\`)
		target = strings.TrimPrefix(target, `\\?\`)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}
	return filepath.Clean(target), nil
}

// RemoveLinkOnly removes linkPath if and only if it is a symlink or reparse point,
// never traversing into or deleting a real target directory.
func RemoveLinkOnly(linkPath string) error {
	isLink, err := IsLinkOrReparsePoint(linkPath)
	if err != nil {
		return err
	}
	if !isLink {
		return fmt.Errorf("fslink: %q is not a symlink or reparse point", linkPath)
	}
	if err := os.Remove(linkPath); err != nil {
		return fmt.Errorf("fslink: remove link %q: %w", linkPath, err)
	}
	if _, err := os.Lstat(linkPath); err == nil {
		return fmt.Errorf("fslink: link %q still exists after removal", linkPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("fslink: verify removal of %q: %w", linkPath, err)
	}
	return nil
}

// RemoveAllNoFollow recursively deletes root without following any symlink or reparse point.
// Any encountered symlink or junction is unlinked via RemoveLinkOnly while its target remains untouched.
func RemoveAllNoFollow(root string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("fslink: root must not be empty")
	}
	fi, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if isFileInfoLinkOrReparse(fi) {
		return RemoveLinkOnly(root)
	}

	if fi.IsDir() {
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := filepath.Join(root, entry.Name())
			if err := RemoveAllNoFollow(child); err != nil {
				return err
			}
		}
		return os.Remove(root)
	}

	return os.Remove(root)
}

// SamePath reports whether a and b refer to the same filesystem location.
// When both exist on disk, os.SameFile is used; otherwise existing ancestor directories
// are resolved via filepath.EvalSymlinks and compared with OS-appropriate case sensitivity.
func SamePath(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}

	absA, errA := filepath.Abs(a)
	if errA != nil {
		absA = filepath.Clean(a)
	}
	absB, errB := filepath.Abs(b)
	if errB != nil {
		absB = filepath.Clean(b)
	}

	fiA, statErrA := os.Stat(absA)
	fiB, statErrB := os.Stat(absB)
	if statErrA == nil && statErrB == nil {
		if os.SameFile(fiA, fiB) {
			return true
		}
	}

	normA := resolveExistingAncestors(absA)
	normB := resolveExistingAncestors(absB)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(normA, normB)
	}
	return normA == normB
}

func resolveExistingAncestors(p string) string {
	curr := filepath.Clean(p)
	var tail []string

	for {
		if _, err := os.Lstat(curr); err == nil {
			curr = expandReparseAncestors(curr)
			if resolved, err := filepath.EvalSymlinks(curr); err == nil {
				curr = resolved
			}
			curr = trimWinDevicePrefix(curr)
			for i := len(tail) - 1; i >= 0; i-- {
				curr = filepath.Join(curr, tail[i])
			}
			return filepath.Clean(curr)
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		tail = append(tail, filepath.Base(curr))
		curr = parent
	}

	return filepath.Clean(trimWinDevicePrefix(p))
}

func expandReparseAncestors(existingPath string) string {
	curr := filepath.Clean(existingPath)
	// Guard against circular links with a bounded hop count.
	for range 16 {
		replaced := false
		probe := curr
		var subTail []string
		for {
			if isLink, err := IsLinkOrReparsePoint(probe); err == nil && isLink {
				if target, err := ReadDirLink(probe); err == nil {
					for i := len(subTail) - 1; i >= 0; i-- {
						target = filepath.Join(target, subTail[i])
					}
					curr = filepath.Clean(target)
					replaced = true
					break
				}
			}
			parent := filepath.Dir(probe)
			if parent == probe {
				break
			}
			subTail = append(subTail, filepath.Base(probe))
			probe = parent
		}
		if !replaced {
			break
		}
	}
	return curr
}

func trimWinDevicePrefix(p string) string {
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, `\??\`)
		p = strings.TrimPrefix(p, `\\?\`)
	}
	return p
}
