package toolchain

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

func TestEnsureTool_DownloadsVerifiesStripsAndSkipsWhenInstalled(t *testing.T) {
	// Given a zip archive served over HTTP containing a top-level directory and bin/cmake.exe
	zipBytes, zipSHA := buildTestZip(t, map[string]string{
		"cmake-4.4.5-windows-x86_64/bin/cmake.exe": "fake-cmake-binary",
	})
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Length", strconv.Itoa(len(zipBytes)))
		_, _ = w.Write(zipBytes)
	}))
	defer srv.Close()

	root := t.TempDir()
	paths := workspace.NewPaths(root, "", filepath.Join(root, ".twdll"))
	st := workspace.NewState()
	var stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Err: &stderr, ColorMode: term.ColorNever})

	toolDef := ToolDef{
		ID:          workspace.ToolCMake,
		Name:        "CMake",
		Version:     "4.4.5",
		URL:         srv.URL + "/cmake.zip",
		SHA256:      zipSHA,
		ArchiveType: "zip",
		StripDirs:   1,
		MarkerRel:   "bin/cmake.exe",
	}
	env := InstallEnv{Paths: paths, State: &st, UI: ui}

	// When EnsureTool runs the first time, then a second time, and then with a mismatched SHA-256 on a missing tool
	if err := EnsureTool(context.Background(), env, toolDef); err != nil {
		t.Fatalf("first EnsureTool failed: %v", err)
	}
	st.Tools = map[workspace.ToolID]workspace.ToolState{}
	if err := EnsureTool(context.Background(), env, toolDef); err != nil {
		t.Fatalf("second EnsureTool failed: %v", err)
	}

	badTool := toolDef
	badTool.ID = workspace.ToolNinja
	badTool.SHA256 = strings.Repeat("0", 64)
	badErr := EnsureTool(context.Background(), env, badTool)

	// Then the marker exists, StripDirs=1 flattened the prefix, the second run skipped HTTP download and backfilled state, and bad SHA failed
	if !IsInstalled(paths, toolDef) {
		t.Fatalf("expected IsInstalled(CMake) to be true")
	}
	if hits.Load() != 2 {
		t.Fatalf("HTTP hits = %d, want 2 (1 for initial CMake + 1 for bad Ninja, 0 for idempotent CMake)", hits.Load())
	}
	if st.Tools[workspace.ToolCMake].Version != "4.4.5" || st.Tools[workspace.ToolCMake].SHA256 != zipSHA {
		t.Fatalf("state not updated for CMake: %+v", st.Tools[workspace.ToolCMake])
	}
	if badErr == nil || !strings.Contains(badErr.Error(), "SHA-256 mismatch") {
		t.Fatalf("expected SHA-256 mismatch error, got: %v", badErr)
	}
}

func TestExtractArchive_RejectsZipSlipTraversal(t *testing.T) {
	t.Run("zip_traversal", func(t *testing.T) {
		zipBytes, _ := buildTestZip(t, map[string]string{
			"root/../../evil.txt": "pwned",
		})
		dir := t.TempDir()
		archivePath := filepath.Join(dir, "evil.zip")
		if err := os.WriteFile(archivePath, zipBytes, 0o644); err != nil {
			t.Fatal(err)
		}

		err := ExtractArchive(context.Background(), ExtractSpec{
			ArchivePath: archivePath,
			ArchiveType: "zip",
			DestDir:     filepath.Join(dir, "out"),
			StripDirs:   1,
		})
		if err == nil {
			t.Fatalf("expected ExtractArchive to reject zip-slip path traversal")
		}
		if _, statErr := os.Stat(filepath.Join(dir, "evil.txt")); statErr == nil {
			t.Fatalf("zip-slip wrote outside destination directory")
		}
	})

	t.Run("targz_traversal", func(t *testing.T) {
		tgzBytes := buildTestTarGz(t, map[string]string{
			"root/../../evil.txt": "pwned",
		})
		dir := t.TempDir()
		archivePath := filepath.Join(dir, "evil.tar.gz")
		if err := os.WriteFile(archivePath, tgzBytes, 0o644); err != nil {
			t.Fatal(err)
		}

		err := ExtractArchive(context.Background(), ExtractSpec{
			ArchivePath: archivePath,
			ArchiveType: "tar.gz",
			DestDir:     filepath.Join(dir, "out"),
			StripDirs:   1,
		})
		if err == nil {
			t.Fatalf("expected ExtractArchive to reject tar.gz path traversal")
		}
		if _, statErr := os.Stat(filepath.Join(dir, "evil.txt")); statErr == nil {
			t.Fatalf("tar.gz traversal wrote outside destination directory")
		}
	})

	if runtime.GOOS == "windows" {
		t.Run("windows_reserved_device_name", func(t *testing.T) {
			zipBytes, _ := buildTestZip(t, map[string]string{
				"root/NUL": "pwned",
			})
			dir := t.TempDir()
			archivePath := filepath.Join(dir, "reserved.zip")
			if err := os.WriteFile(archivePath, zipBytes, 0o644); err != nil {
				t.Fatal(err)
			}

			err := ExtractArchive(context.Background(), ExtractSpec{
				ArchivePath: archivePath,
				ArchiveType: "zip",
				DestDir:     filepath.Join(dir, "out"),
				StripDirs:   1,
			})
			if err == nil {
				t.Fatalf("expected ExtractArchive to reject Windows reserved device name NUL")
			}
		})
	}
}

func TestFindIDAExecutable_ResolvesHeadlessAndOrdersVersionsDescending(t *testing.T) {
	// Given fake IDA 8.4 and IDA 9.5 directories under ProgramFiles
	pf := t.TempDir()
	t.Setenv("ProgramFiles", pf)
	t.Setenv("ProgramFiles(x86)", pf)

	ida84 := filepath.Join(pf, "IDA Pro 8.4")
	ida95 := filepath.Join(pf, "IDA Professional 9.5")
	if err := os.MkdirAll(ida84, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ida95, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ida84, "idat64.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ida95, "idat.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When FindIDAExecutable and AutoDetectIDADir run
	exe64, err := FindIDAExecutable(ida95, true)
	if err != nil {
		t.Fatalf("FindIDAExecutable failed: %v", err)
	}
	bestDir := AutoDetectIDADir()

	// Then idat.exe is resolved for IDA 9.5 and AutoDetectIDADir selects 9.5 over 8.4
	if exe64 != filepath.ToSlash(filepath.Join(ida95, "idat.exe")) {
		t.Fatalf("FindIDAExecutable = %q, want idat.exe in IDA 9.5", exe64)
	}
	if bestDir != filepath.ToSlash(ida95) {
		t.Fatalf("AutoDetectIDADir = %q, want %q", bestDir, filepath.ToSlash(ida95))
	}
}

func buildTestZip(t *testing.T, files map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func TestExtractArchive_OnProgress_ReportsMonotonicBytesForZipTarGzAndTarXz(t *testing.T) {
	files := map[string]string{
		"pkg-1.0/bin/tool.exe": strings.Repeat("A", 12000),
		"pkg-1.0/lib/core.a":   strings.Repeat("B", 8000),
	}

	t.Run("zip", func(t *testing.T) {
		dir := t.TempDir()
		zipBytes, _ := buildTestZip(t, files)
		archivePath := filepath.Join(dir, "pkg.zip")
		if err := os.WriteFile(archivePath, zipBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		assertExtractProgressMonotonic(t, archivePath, "zip", filepath.Join(dir, "out"))
	})

	t.Run("tar.gz", func(t *testing.T) {
		dir := t.TempDir()
		tgzBytes := buildTestTarGz(t, files)
		archivePath := filepath.Join(dir, "pkg.tar.gz")
		if err := os.WriteFile(archivePath, tgzBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		assertExtractProgressMonotonic(t, archivePath, "tar.gz", filepath.Join(dir, "out"))
	})

	t.Run("tar.xz", func(t *testing.T) {
		tarBin := "tar"
		if runtime.GOOS == "windows" {
			if _, err := os.Stat(`C:\Windows\System32\tar.exe`); err == nil {
				tarBin = `C:\Windows\System32\tar.exe`
			}
		}
		if _, err := exec.LookPath(tarBin); err != nil {
			t.Skip("tar binary not available on host")
		}
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "src")
		for rel, body := range files {
			p := filepath.Join(srcDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		archivePath := filepath.Join(dir, "pkg.tar.xz")
		cmd := exec.Command(tarBin, "-cJf", archivePath, "-C", srcDir, "pkg-1.0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("host tar does not support -cJf: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		assertExtractProgressMonotonic(t, archivePath, "tar.xz", filepath.Join(dir, "out"))
	})
}

func assertExtractProgressMonotonic(t *testing.T, archivePath, archiveType, destDir string) {
	t.Helper()
	var events []ExtractProgress
	err := ExtractArchive(context.Background(), ExtractSpec{
		ArchivePath: archivePath,
		ArchiveType: archiveType,
		DestDir:     destDir,
		StripDirs:   1,
		OnProgress: func(p ExtractProgress) {
			events = append(events, p)
		},
	})
	if err != nil {
		t.Fatalf("ExtractArchive(%s) failed: %v", archiveType, err)
	}
	if len(events) == 0 {
		t.Fatalf("ExtractArchive(%s) emitted 0 progress events", archiveType)
	}
	var prev int64
	for i, ev := range events {
		if ev.DoneBytes < prev {
			t.Fatalf("event %d DoneBytes regressed: %d < %d", i, ev.DoneBytes, prev)
		}
		prev = ev.DoneBytes
	}
	last := events[len(events)-1]
	if last.TotalBytes <= 0 || last.DoneBytes != last.TotalBytes {
		t.Fatalf("final progress for %s = %+v, want DoneBytes == TotalBytes > 0", archiveType, last)
	}
	if _, err := os.Stat(filepath.Join(destDir, "bin", "tool.exe")); err != nil {
		t.Fatalf("expected extracted bin/tool.exe: %v", err)
	}
}

func buildTestTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
