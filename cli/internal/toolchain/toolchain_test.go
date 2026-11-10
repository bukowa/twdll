package toolchain

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"twdll/cli/internal/workspace"
)

func TestIsToolInstalled(t *testing.T) {
	tmp := t.TempDir()
	paths := workspace.NewPaths(tmp)

	tool := ToolDef{
		ID:        workspace.ToolNinja,
		Name:      "Ninja",
		MarkerRel: "ninja.exe",
	}

	if IsToolInstalled(paths, tool) {
		t.Errorf("expected tool not installed")
	}

	toolDir := paths.ToolDir(tool.ID)
	_ = os.MkdirAll(toolDir, 0o755)
	_ = os.WriteFile(filepath.Join(toolDir, "ninja.exe"), []byte("mock_ninja"), 0o755)

	if !IsToolInstalled(paths, tool) {
		t.Errorf("expected tool to be reported as installed")
	}
}

func TestStripPath(t *testing.T) {
	tests := []struct {
		input    string
		count    int
		expected string
	}{
		{"cmake-3.31.5/bin/cmake.exe", 1, filepath.Join("bin", "cmake.exe")},
		{"ninja.exe", 0, "ninja.exe"},
		{"a/b/c/d.txt", 2, filepath.Join("c", "d.txt")},
		{"root/only", 1, "only"},
		{"single", 1, ""},
	}

	for _, tt := range tests {
		got := stripPath(tt.input, tt.count)
		if got != tt.expected {
			t.Errorf("stripPath(%q, %d) = %q, want %q", tt.input, tt.count, got, tt.expected)
		}
	}
}

func TestSafePath_ZipSlipPrevention(t *testing.T) {
	dest := filepath.Join(os.TempDir(), "twdll_test_safe")

	// Valid path
	valid, err := safePath(dest, "bin/tool.exe")
	if err != nil {
		t.Errorf("safePath failed for valid path: %v", err)
	}
	expected := filepath.Join(dest, "bin", "tool.exe")
	if valid != expected {
		t.Errorf("got %q, want %q", valid, expected)
	}

	// Zip-slip attacks
	attacks := []string{
		"../evil.exe",
		"../../windows/system32/cmd.exe",
		"sub/../../outside.txt",
	}

	for _, attack := range attacks {
		if _, err := safePath(dest, attack); err == nil {
			t.Errorf("safePath allowed malicious path %q", attack)
		}
	}
}

func TestExtractZipWithStrip(t *testing.T) {
	tmp := t.TempDir()
	zipFile := filepath.Join(tmp, "test.zip")
	destDir := filepath.Join(tmp, "out")

	// Create synthetic zip archive with root wrapper
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	f, err := zw.Create("wrapper-dir/bin/test.exe")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	_, _ = f.Write([]byte("binary_contents"))
	_ = zw.Close()

	if err := os.WriteFile(zipFile, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write test zip: %v", err)
	}

	// Extract stripping 1 level
	if err := extractZip(zipFile, destDir, 1); err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	extracted := filepath.Join(destDir, "bin", "test.exe")
	content, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(content) != "binary_contents" {
		t.Errorf("got %q, want 'binary_contents'", string(content))
	}
}

func TestExtractTarGzWithStrip(t *testing.T) {
	tmp := t.TempDir()
	tarFile := filepath.Join(tmp, "test.tar.gz")
	destDir := filepath.Join(tmp, "out")

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	body := []byte("tar_contents")
	hdr := &tar.Header{
		Name: "root-folder/xwin.exe",
		Mode: 0o755,
		Size: int64(len(body)),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gw.Close()

	if err := os.WriteFile(tarFile, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tar.gz: %v", err)
	}

	if err := extractTarGz(tarFile, destDir, 1); err != nil {
		t.Fatalf("extractTarGz failed: %v", err)
	}

	extracted := filepath.Join(destDir, "xwin.exe")
	content, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(content) != "tar_contents" {
		t.Errorf("got %q, want 'tar_contents'", string(content))
	}
}

func TestValidateIDADir(t *testing.T) {
	tmp := t.TempDir()

	// Empty dir fails
	if err := ValidateIDADir(tmp); err == nil {
		t.Errorf("expected error for empty dir")
	}

	// Add ida.exe (IDA 9.x unified)
	_ = os.WriteFile(filepath.Join(tmp, "ida.exe"), []byte("mock"), 0o755)
	if err := ValidateIDADir(tmp); err != nil {
		t.Errorf("expected success with ida.exe, got: %v", err)
	}
}

func TestAutoDetectIDADir(t *testing.T) {
	// AutoDetectIDADir returns a non-empty path if IDA is installed on this host
	detected := AutoDetectIDADir()
	if detected != "" {
		if err := ValidateIDADir(detected); err != nil {
			t.Errorf("AutoDetectIDADir returned invalid path %s: %v", detected, err)
		}
	}
}
