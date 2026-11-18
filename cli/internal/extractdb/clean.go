package extractdb

import (
	"errors"
	"fmt"
	"os"

	"twdll/cli/internal/workspace"
)

// CleanWorkspace removes all extracted SQLite databases and staging artifacts
// under paths.ExtractedDir. It returns true if anything was removed.
func CleanWorkspace(paths workspace.Paths) (bool, error) {
	if _, err := os.Stat(paths.ExtractedDir); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("stat extracted dir %s: %w", paths.ExtractedDir, err)
	}
	if err := os.RemoveAll(paths.ExtractedDir); err != nil {
		return false, fmt.Errorf("remove extracted dir %s: %w", paths.ExtractedDir, err)
	}
	return true, nil
}

// ClearTarget removes bronze, manifest, derived, and selected silver SQLite
// artifacts for target before a clean re-extraction while preserving sibling
// engine silver databases not in engines.
func ClearTarget(paths workspace.Paths, target workspace.TargetID, engines []workspace.DecompilerID) (bool, error) {
	removed := false
	candidates := []string{
		paths.BronzeDBPath(target),
		paths.TargetManifestPath(target),
		paths.DerivedDBPath(),
	}
	for _, engine := range engines {
		candidates = append(candidates, paths.SilverDBPath(target, engine))
	}
	for _, path := range candidates {
		ok, err := removeFileAndSQLiteSidecars(path)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = true
		}
	}
	_ = os.Remove(paths.ExtractedTargetDir(target))
	return removed, nil
}

func removeFileAndSQLiteSidecars(path string) (bool, error) {
	removed := false
	for _, candidate := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if err := os.Remove(candidate); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return removed, fmt.Errorf("remove %s: %w", candidate, err)
		}
		removed = true
	}
	return removed, nil
}
