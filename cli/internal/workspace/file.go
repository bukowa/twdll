package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to a temporary file in the same directory and renames it to path.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure dir %s: %w", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmpFile.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("write temp file %s: %w", tmpName, err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file %s: %w", tmpName, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file %s: %w", tmpName, err)
	}

	// On Windows, if destination exists, rename may fail if locked; remove before rename or rename directly
	if err := os.Rename(tmpName, path); err != nil {
		// Fallback for Windows if destination already exists and rename blocks
		_ = os.Remove(path)
		if err := os.Rename(tmpName, path); err != nil {
			return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
		}
	}

	cleanup = false
	return nil
}
