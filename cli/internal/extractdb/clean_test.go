package extractdb

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"twdll/cli/internal/workspace"
)

func TestCleanWorkspace_And_ClearTarget(t *testing.T) {
	t.Run("ClearTarget removes bronze, manifest, derived, and selected silver sidecars while preserving sibling engines", func(t *testing.T) {
		root := t.TempDir()
		paths := workspace.NewPaths(root, filepath.Join(root, "twdll.toml"), filepath.Join(root, ".twdll"))

		for _, tid := range []workspace.TargetID{workspace.TargetAttila32, workspace.TargetAttila64} {
			for _, p := range []string{
				paths.BronzeDBPath(tid),
				paths.BronzeDBPath(tid) + "-wal",
				paths.BronzeDBPath(tid) + "-shm",
				paths.TargetManifestPath(tid),
				paths.SilverDBPath(tid, workspace.DecompilerGhidra),
				paths.SilverDBPath(tid, workspace.DecompilerGhidra) + "-wal",
				paths.SilverDBPath(tid, workspace.DecompilerIDA),
			} {
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := os.WriteFile(paths.DerivedDBPath(), []byte("derived"), 0o644); err != nil {
			t.Fatal(err)
		}

		removed, err := ClearTarget(paths, workspace.TargetAttila32, []workspace.DecompilerID{workspace.DecompilerGhidra})
		if err != nil {
			t.Fatalf("ClearTarget: %v", err)
		}
		if !removed {
			t.Fatalf("expected ClearTarget removed = true")
		}

		for _, gone := range []string{
			paths.BronzeDBPath(workspace.TargetAttila32),
			paths.BronzeDBPath(workspace.TargetAttila32) + "-wal",
			paths.BronzeDBPath(workspace.TargetAttila32) + "-shm",
			paths.TargetManifestPath(workspace.TargetAttila32),
			paths.SilverDBPath(workspace.TargetAttila32, workspace.DecompilerGhidra),
			paths.SilverDBPath(workspace.TargetAttila32, workspace.DecompilerGhidra) + "-wal",
			paths.DerivedDBPath(),
		} {
			if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("expected %s deleted, stat err = %v", gone, err)
			}
		}

		for _, kept := range []string{
			paths.SilverDBPath(workspace.TargetAttila32, workspace.DecompilerIDA),
			paths.BronzeDBPath(workspace.TargetAttila64),
			paths.SilverDBPath(workspace.TargetAttila64, workspace.DecompilerGhidra),
			paths.SilverDBPath(workspace.TargetAttila64, workspace.DecompilerIDA),
		} {
			if _, err := os.Stat(kept); err != nil {
				t.Fatalf("expected sibling artifact %s preserved: %v", kept, err)
			}
		}
	})

	t.Run("CleanWorkspace removes ExtractedDir and reports false when already clean", func(t *testing.T) {
		root := t.TempDir()
		paths := workspace.NewPaths(root, filepath.Join(root, "twdll.toml"), filepath.Join(root, ".twdll"))

		removed, err := CleanWorkspace(paths)
		if err != nil || removed {
			t.Fatalf("CleanWorkspace on empty dir = (%v, %v), want (false, nil)", removed, err)
		}

		bronze := paths.BronzeDBPath(workspace.TargetAttila64)
		if err := os.MkdirAll(filepath.Dir(bronze), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bronze, []byte("sqlite"), 0o644); err != nil {
			t.Fatal(err)
		}

		removed, err = CleanWorkspace(paths)
		if err != nil || !removed {
			t.Fatalf("CleanWorkspace = (%v, %v), want (true, nil)", removed, err)
		}
		if _, err := os.Stat(paths.ExtractedDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s deleted, stat err = %v", paths.ExtractedDir, err)
		}
	})
}
