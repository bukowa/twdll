package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHasNewerSources_ReturnsFalseWhenBinaryIsNewer(t *testing.T) {
	cliDir := t.TempDir()
	binTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	olderTime := binTime.Add(-10 * time.Minute)

	writeTimedFile(t, filepath.Join(cliDir, "main.go"), "package main\n", olderTime)
	writeTimedFile(t, filepath.Join(cliDir, "go.mod"), "module twdll/cli\n", olderTime)
	writeTimedFile(t, filepath.Join(cliDir, "go.sum"), "", olderTime)
	writeTimedFile(t, filepath.Join(cliDir, "internal", "analyze", "Export.java"), "// ghidra\n", olderTime)
	writeTimedFile(t, filepath.Join(cliDir, "internal", "analyze", "analyze.idc"), "// ida\n", olderTime)

	if hasNewerSources(cliDir, binTime) {
		t.Fatalf("expected hasNewerSources=false when all tracked sources are older than binary")
	}
}

func TestHasNewerSources_IgnoresNewerTestGoFiles(t *testing.T) {
	cliDir := t.TempDir()
	binTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	olderTime := binTime.Add(-10 * time.Minute)
	newerTime := binTime.Add(10 * time.Minute)

	writeTimedFile(t, filepath.Join(cliDir, "main.go"), "package main\n", olderTime)
	writeTimedFile(t, filepath.Join(cliDir, "main_test.go"), "package main\n", newerTime)
	writeTimedFile(t, filepath.Join(cliDir, "internal", "cmd", "cmd_test.go"), "package cmd\n", newerTime)
	writeTimedFile(t, filepath.Join(cliDir, "README.md"), "# notes\n", newerTime)

	if hasNewerSources(cliDir, binTime) {
		t.Fatalf("expected hasNewerSources=false when only _test.go or non-source files are newer")
	}
}

func TestHasNewerSources_DetectsNewerTrackedFiles(t *testing.T) {
	binTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	olderTime := binTime.Add(-10 * time.Minute)
	newerTime := binTime.Add(10 * time.Minute)

	cases := []struct {
		name    string
		relPath string
	}{
		{name: "go source", relPath: filepath.Join("internal", "cmd", "status.go")},
		{name: "go.mod", relPath: "go.mod"},
		{name: "go.sum", relPath: "go.sum"},
		{name: "embedded java script", relPath: filepath.Join("internal", "analyze", "Headless.java")},
		{name: "embedded idc script", relPath: filepath.Join("internal", "analyze", "headless.idc")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cliDir := t.TempDir()
			writeTimedFile(t, filepath.Join(cliDir, "main.go"), "package main\n", olderTime)
			writeTimedFile(t, filepath.Join(cliDir, tc.relPath), "// updated\n", newerTime)

			if !hasNewerSources(cliDir, binTime) {
				t.Fatalf("expected hasNewerSources=true when %s is newer than binary", tc.relPath)
			}
		})
	}
}

func TestMaybeRebuildAndExec_NoRebuildEnvGuard(t *testing.T) {
	t.Setenv("TWDLL_NO_REBUILD", "1")
	rebuilt, code := MaybeRebuildAndExec(t.TempDir(), []string{"status"})
	if rebuilt || code != 0 {
		t.Fatalf("expected (false, 0) when TWDLL_NO_REBUILD=1, got (%v, %d)", rebuilt, code)
	}
}

func TestResolveGoToolchain_PrefersPortableGoOverSystemPath(t *testing.T) {
	repoRoot := t.TempDir()
	portable := portableGoPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(portable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(portable, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := resolveGoToolchain(repoRoot)
	if got != portable {
		t.Fatalf("resolveGoToolchain = %q, want portable Go %q", got, portable)
	}
}

func TestResolveGoToolchain_FallsBackToSystemPathWhenPortableMissing(t *testing.T) {
	repoRoot := t.TempDir()
	fakeBin := t.TempDir()
	sysGo := filepath.Join(fakeBin, filepath.Base(portableGoPath(repoRoot)))
	if err := os.WriteFile(sysGo, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)

	got := resolveGoToolchain(repoRoot)
	if got != sysGo {
		t.Fatalf("resolveGoToolchain = %q, want fallback %q", got, sysGo)
	}
}

func writeTimedFile(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}
