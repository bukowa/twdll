package fslink

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnsureDirLinkAndReadThrough(t *testing.T) {
	base := t.TempDir()
	targetDir := filepath.Join(base, "primary", ".twdll", "tools")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir targetDir: %v", err)
	}

	sentinelPath := filepath.Join(targetDir, "go", "bin", "go.exe")
	if err := os.MkdirAll(filepath.Dir(sentinelPath), 0o755); err != nil {
		t.Fatalf("mkdir sentinel parent: %v", err)
	}
	sentinelBytes := []byte("sentinel-go-toolchain-binary-payload")
	if err := os.WriteFile(sentinelPath, sentinelBytes, 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	linkPath := filepath.Join(base, "wt", ".twdll", "tools")
	if err := EnsureDirLink(targetDir, linkPath); err != nil {
		t.Fatalf("EnsureDirLink: %v", err)
	}

	gotBytes, err := os.ReadFile(filepath.Join(linkPath, "go", "bin", "go.exe"))
	if err != nil {
		t.Fatalf("read file through link: %v", err)
	}
	if !bytes.Equal(gotBytes, sentinelBytes) {
		t.Fatalf("file content mismatch through link: got %q, want %q", gotBytes, sentinelBytes)
	}

	// EnsureDirLink must fail if linkPath already exists.
	if err := EnsureDirLink(targetDir, linkPath); err == nil {
		t.Fatalf("expected error when linkPath already exists, got nil")
	}
}

func TestEnsureDirLinkValidation(t *testing.T) {
	base := t.TempDir()
	regularFile := filepath.Join(base, "file.txt")
	if err := os.WriteFile(regularFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write regularFile: %v", err)
	}

	if err := EnsureDirLink("", filepath.Join(base, "link")); err == nil {
		t.Fatalf("expected error for empty targetDir")
	}
	if err := EnsureDirLink(base, ""); err == nil {
		t.Fatalf("expected error for empty linkPath")
	}
	if err := EnsureDirLink(filepath.Join(base, "missing"), filepath.Join(base, "link")); err == nil {
		t.Fatalf("expected error for missing targetDir")
	}
	if err := EnsureDirLink(regularFile, filepath.Join(base, "link")); err == nil {
		t.Fatalf("expected error when targetDir is a regular file")
	}
}

func TestIsLinkOrReparsePoint(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real_dir")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir realDir: %v", err)
	}

	realFile := filepath.Join(base, "real_file.txt")
	if err := os.WriteFile(realFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("write realFile: %v", err)
	}

	linkDir := filepath.Join(base, "link_dir")
	if err := EnsureDirLink(realDir, linkDir); err != nil {
		t.Fatalf("EnsureDirLink: %v", err)
	}

	isLink, err := IsLinkOrReparsePoint(linkDir)
	if err != nil {
		t.Fatalf("IsLinkOrReparsePoint(linkDir): %v", err)
	}
	if !isLink {
		t.Fatalf("expected IsLinkOrReparsePoint(linkDir) == true, got false")
	}

	isLink, err = IsLinkOrReparsePoint(realDir)
	if err != nil {
		t.Fatalf("IsLinkOrReparsePoint(realDir): %v", err)
	}
	if isLink {
		t.Fatalf("expected IsLinkOrReparsePoint(realDir) == false, got true")
	}

	isLink, err = IsLinkOrReparsePoint(realFile)
	if err != nil {
		t.Fatalf("IsLinkOrReparsePoint(realFile): %v", err)
	}
	if isLink {
		t.Fatalf("expected IsLinkOrReparsePoint(realFile) == false, got true")
	}

	_, err = IsLinkOrReparsePoint(filepath.Join(base, "nonexistent"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist on missing path, got %v", err)
	}
}

func TestReadDirLinkAndSamePath(t *testing.T) {
	base := t.TempDir()
	targetDir := filepath.Join(base, "PrimaryTarget", "SubDir")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir targetDir: %v", err)
	}

	linkDir := filepath.Join(base, "wt", "linked_subdir")
	if err := EnsureDirLink(targetDir, linkDir); err != nil {
		t.Fatalf("EnsureDirLink: %v", err)
	}

	readTarget, err := ReadDirLink(linkDir)
	if err != nil {
		t.Fatalf("ReadDirLink: %v", err)
	}
	if strings.HasPrefix(readTarget, `\??\`) || strings.HasPrefix(readTarget, `\\?\`) {
		t.Fatalf("ReadDirLink returned unstripped prefix: %q", readTarget)
	}
	if !SamePath(readTarget, targetDir) {
		t.Fatalf("SamePath(%q, %q) = false, want true", readTarget, targetDir)
	}
	if !SamePath(linkDir, targetDir) {
		t.Fatalf("SamePath(linkDir=%q, targetDir=%q) = false, want true", linkDir, targetDir)
	}

	// Relative path with .. segments
	relTarget := filepath.Join(targetDir, "..", "SubDir")
	if !SamePath(relTarget, targetDir) {
		t.Fatalf("SamePath(relTarget=%q, targetDir=%q) = false, want true", relTarget, targetDir)
	}

	// Non-existent child under linked directory vs target directory
	nonExistentViaLink := filepath.Join(linkDir, "future", "child.db")
	nonExistentViaTarget := filepath.Join(targetDir, "future", "child.db")
	if !SamePath(nonExistentViaLink, nonExistentViaTarget) {
		t.Fatalf("SamePath on non-existent children through link failed: %q vs %q", nonExistentViaLink, nonExistentViaTarget)
	}

	if runtime.GOOS == "windows" {
		upper := strings.ToUpper(targetDir)
		lower := strings.ToLower(targetDir)
		if !SamePath(upper, lower) {
			t.Fatalf("SamePath case-insensitive check failed on Windows: %q vs %q", upper, lower)
		}
		nonExistentUpper := strings.ToUpper(nonExistentViaTarget)
		nonExistentLower := strings.ToLower(nonExistentViaTarget)
		if !SamePath(nonExistentUpper, nonExistentLower) {
			t.Fatalf("SamePath non-existent case-insensitive check failed on Windows: %q vs %q", nonExistentUpper, nonExistentLower)
		}
	}

	if SamePath(targetDir, filepath.Join(base, "other")) {
		t.Fatalf("SamePath returned true for distinct paths")
	}
	if SamePath("", targetDir) || SamePath(targetDir, "") {
		t.Fatalf("SamePath returned true for empty path")
	}
}

func TestRemoveLinkOnlyPreservesTargetAndRefusesRealDir(t *testing.T) {
	base := t.TempDir()
	targetDir := filepath.Join(base, "shared_db")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir targetDir: %v", err)
	}
	sentinel := filepath.Join(targetDir, "rome2.gpr")
	sentinelContent := []byte("ghidra-project-sentinel-bytes")
	if err := os.WriteFile(sentinel, sentinelContent, 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	// Refuses to remove a real directory.
	if err := RemoveLinkOnly(targetDir); err == nil {
		t.Fatalf("RemoveLinkOnly succeeded on a real directory, expected error")
	}
	// Verify real directory and sentinel still exist.
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel missing after refused RemoveLinkOnly: %v", err)
	}

	// Create link and remove only the link.
	linkDir := filepath.Join(base, "wt", ".twdll", "db")
	if err := EnsureDirLink(targetDir, linkDir); err != nil {
		t.Fatalf("EnsureDirLink: %v", err)
	}
	if err := RemoveLinkOnly(linkDir); err != nil {
		t.Fatalf("RemoveLinkOnly(linkDir): %v", err)
	}

	if _, err := os.Lstat(linkDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected linkDir to be removed, got err=%v", err)
	}

	gotBytes, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("sentinel file missing after RemoveLinkOnly: %v", err)
	}
	if !bytes.Equal(gotBytes, sentinelContent) {
		t.Fatalf("sentinel bytes corrupted: got %q, want %q", gotBytes, sentinelContent)
	}
}

func TestRemoveAllNoFollowMixedWorktreeTree(t *testing.T) {
	base := t.TempDir()
	primaryDataDir := filepath.Join(base, "primary", ".twdll")
	wtDataDir := filepath.Join(base, "wt", ".twdll")

	sharedDirs := []string{"tools", "binaries", "db", "extracted"}
	sentinels := make(map[string][]byte, len(sharedDirs))

	for _, name := range sharedDirs {
		sharedPath := filepath.Join(primaryDataDir, name, "nested")
		if err := os.MkdirAll(sharedPath, 0o755); err != nil {
			t.Fatalf("mkdir shared %s: %v", name, err)
		}
		sentinelFile := filepath.Join(sharedPath, name+"-sentinel.dat")
		payload := []byte("precious-shared-data-" + name + "-do-not-delete")
		if err := os.WriteFile(sentinelFile, payload, 0o644); err != nil {
			t.Fatalf("write sentinel %s: %v", sentinelFile, err)
		}
		sentinels[sentinelFile] = payload

		linkPath := filepath.Join(wtDataDir, name)
		if err := EnsureDirLink(filepath.Join(primaryDataDir, name), linkPath); err != nil {
			t.Fatalf("EnsureDirLink %s: %v", name, err)
		}
	}

	// Populate real local files and directories inside wtDataDir.
	localFiles := map[string][]byte{
		filepath.Join(wtDataDir, "bin", "twdll.exe"): []byte("compiled-local-binary"),
		filepath.Join(wtDataDir, "logs", "run.log"):  []byte("worktree-log-output"),
		filepath.Join(wtDataDir, "state.json"):       []byte(`{"schema_version":1}`),
	}
	for path, content := range localFiles {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir local dir for %s: %v", path, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("write local file %s: %v", path, err)
		}
	}

	// Execute RemoveAllNoFollow on wtDataDir.
	if err := RemoveAllNoFollow(wtDataDir); err != nil {
		t.Fatalf("RemoveAllNoFollow(wtDataDir): %v", err)
	}

	// Assert wtDataDir is completely gone.
	if _, err := os.Lstat(wtDataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected wtDataDir to be removed, got err=%v", err)
	}

	// Calling RemoveAllNoFollow on an already-removed path must be a no-op.
	if err := RemoveAllNoFollow(wtDataDir); err != nil {
		t.Fatalf("RemoveAllNoFollow on non-existent path returned error: %v", err)
	}

	// Assert every sentinel file in primaryDataDir survived byte-for-byte.
	for sentinelFile, wantPayload := range sentinels {
		gotPayload, err := os.ReadFile(sentinelFile)
		if err != nil {
			t.Fatalf("shared sentinel %s was deleted or unreadable: %v", sentinelFile, err)
		}
		if !bytes.Equal(gotPayload, wantPayload) {
			t.Fatalf("shared sentinel %s content mismatch: got %q, want %q", sentinelFile, gotPayload, wantPayload)
		}
	}
}
