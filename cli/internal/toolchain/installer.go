package toolchain

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"twdll/cli/internal/ui"
	"twdll/cli/internal/workspace"
)

// IsToolInstalled checks whether the tool's marker file exists.
func IsToolInstalled(paths workspace.Paths, tool ToolDef) bool {
	markerPath := filepath.Join(paths.ToolDir(tool.ID), filepath.FromSlash(tool.MarkerRel))
	info, err := os.Stat(markerPath)
	return err == nil && !info.IsDir()
}

// InstallTool downloads and extracts a tool into .twdll/tools/<id>/ if not already installed.
func InstallTool(ctx context.Context, paths workspace.Paths, tool ToolDef, printer *ui.Printer) error {
	if IsToolInstalled(paths, tool) {
		if printer != nil {
			printer.Info("Tool %s (%s) is already installed.", tool.Name, tool.Version)
		}
		return nil
	}

	targetDir := paths.ToolDir(tool.ID)
	toolsRoot := paths.ToolsDir
	if err := os.MkdirAll(toolsRoot, 0o755); err != nil {
		return fmt.Errorf("create tools directory: %w", err)
	}

	tempDownload := filepath.Join(toolsRoot, fmt.Sprintf("%s.download.tmp", tool.ID))
	tempExtract := filepath.Join(toolsRoot, fmt.Sprintf("%s.extract.tmp", tool.ID))

	// Clean up any stale temp files from previous interrupted runs
	_ = os.Remove(tempDownload)
	_ = os.RemoveAll(tempExtract)

	defer func() {
		_ = os.Remove(tempDownload)
		_ = os.RemoveAll(tempExtract)
	}()

	if printer != nil {
		printer.Info("Downloading %s %s from %s...", tool.Name, tool.Version, tool.URL)
	}

	if err := downloadFile(ctx, tool.URL, tempDownload, tool.Name, printer); err != nil {
		return fmt.Errorf("download %s: %w", tool.Name, err)
	}

	if tool.SHA256 != "" {
		if printer != nil {
			printer.Info("Verifying checksum for %s...", tool.Name)
		}
		if err := verifyFileSHA256(tempDownload, tool.SHA256); err != nil {
			return fmt.Errorf("verify %s: %w", tool.Name, err)
		}
	}

	if printer != nil {
		printer.Info("Extracting %s...", tool.Name)
	}

	if err := os.MkdirAll(tempExtract, 0o755); err != nil {
		return fmt.Errorf("create staging extract directory: %w", err)
	}

	switch tool.ArchiveType {
	case "zip":
		if err := extractZip(tempDownload, tempExtract, tool.StripDirs); err != nil {
			return fmt.Errorf("extract zip %s: %w", tool.Name, err)
		}
	case "tar.gz":
		if err := extractTarGz(tempDownload, tempExtract, tool.StripDirs); err != nil {
			return fmt.Errorf("extract tar.gz %s: %w", tool.Name, err)
		}
	default:
		return fmt.Errorf("unsupported archive type %q for tool %s", tool.ArchiveType, tool.Name)
	}

	// Verify marker exists in staging before swapping
	stagingMarker := filepath.Join(tempExtract, filepath.FromSlash(tool.MarkerRel))
	if _, err := os.Stat(stagingMarker); err != nil {
		return fmt.Errorf("marker file %q missing in extracted archive for %s", tool.MarkerRel, tool.Name)
	}

	// Swap staging folder to target destination
	_ = os.RemoveAll(targetDir)
	if err := os.Rename(tempExtract, targetDir); err != nil {
		return fmt.Errorf("install %s to %s: %w", tool.Name, targetDir, err)
	}

	if printer != nil {
		printer.Success("Installed %s %s to %s", tool.Name, tool.Version, targetDir)
	}

	return nil
}

// ProvisionXWin runs xwin splat to unpack 32-bit MSVC CRT and Windows SDK headers/libraries.
func ProvisionXWin(ctx context.Context, paths workspace.Paths, printer *ui.Printer) error {
	xwinDir := paths.ToolDir(workspace.ToolXwin)
	sdkDir := filepath.Join(xwinDir, "sdk")

	if isXWinProvisioned(sdkDir) {
		if printer != nil {
			printer.Info("xwin 32-bit CRT and Windows SDK already provisioned.")
		}
		return nil
	}

	xwinExe := filepath.Join(xwinDir, "xwin.exe")
	if runtime.GOOS != "windows" {
		xwinExe = filepath.Join(xwinDir, "xwin")
	}

	if _, err := os.Stat(xwinExe); err != nil {
		return fmt.Errorf("xwin executable not found at %s: run toolchain installer first", xwinExe)
	}

	if printer != nil {
		printer.Info("Provisioning 32-bit MSVC CRT and Windows SDK via xwin splat (may take a minute)...")
	}

	// Clean sdkDir first to remove any failed/partial attempts
	_ = os.RemoveAll(sdkDir)

	// --use-winsysroot-style and --disable-symlinks prevent Windows error 1314 (ERROR_PRIVILEGE_NOT_HELD)
	// by omitting casing symlinks and preserving official versioned directories without requiring Developer Mode or Admin.
	cmd := exec.CommandContext(ctx, xwinExe, "--accept-license", "--arch", "x86", "splat", "--output", sdkDir, "--use-winsysroot-style", "--disable-symlinks")
	cmd.Dir = xwinDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xwin splat failed: %w\nOutput: %s", err, string(output))
	}

	if !isXWinProvisioned(sdkDir) {
		return fmt.Errorf("xwin completed but 32-bit CRT libcmt.lib was not found in %s", sdkDir)
	}

	if printer != nil {
		printer.Success("Provisioned 32-bit MSVC CRT and Windows SDK in %s", sdkDir)
	}
	return nil
}

func isXWinProvisioned(sdkDir string) bool {
	legacyMarker := filepath.Join(sdkDir, "crt", "lib", "x86", "libcmt.lib")
	if _, err := os.Stat(legacyMarker); err == nil {
		return true
	}
	matches, err := filepath.Glob(filepath.Join(sdkDir, "VC", "Tools", "MSVC", "*", "lib", "x86", "libcmt.lib"))
	return err == nil && len(matches) > 0
}

// ValidateIDADir checks that the specified IDA Pro directory contains headless idat/idat64 or ida executables.
func ValidateIDADir(dir string) error {
	if dir == "" {
		return fmt.Errorf("ida_dir cannot be empty")
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("ida_dir %q is not a valid directory", dir)
	}

	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			filepath.Join(dir, "idat.exe"),
			filepath.Join(dir, "idat64.exe"),
			filepath.Join(dir, "ida.exe"),
			filepath.Join(dir, "ida64.exe"),
		}
	} else {
		candidates = []string{
			filepath.Join(dir, "idat"),
			filepath.Join(dir, "idat64"),
			filepath.Join(dir, "ida"),
			filepath.Join(dir, "ida64"),
		}
	}

	for _, c := range candidates {
		if stat, err := os.Stat(c); err == nil && !stat.IsDir() {
			return nil
		}
	}

	return fmt.Errorf("neither idat nor ida found in %s", dir)
}

// AutoDetectIDADir searches standard installation paths for IDA Pro and returns the latest valid directory.
func AutoDetectIDADir() string {
	if runtime.GOOS != "windows" {
		return ""
	}

	var searchRoots = []string{
		`C:\Program Files`,
		`C:\Program Files (x86)`,
	}

	var found []string
	for _, root := range searchRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if strings.Contains(name, "ida") {
				dirPath := filepath.Join(root, e.Name())
				if ValidateIDADir(dirPath) == nil {
					found = append(found, dirPath)
				}
			}
		}
	}

	if len(found) == 0 {
		return ""
	}

	// Sort descending so highest version (e.g. IDA Professional 9.5) comes first
	sort.Slice(found, func(i, j int) bool {
		return found[i] > found[j]
	})

	return found[0]
}

func downloadFile(ctx context.Context, url string, destFile string, toolName string, printer *ui.Printer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	out, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer out.Close()

	var reader io.Reader = resp.Body
	total := resp.ContentLength
	if total > 0 && printer != nil {
		reader = &progressTracker{
			reader:  resp.Body,
			total:   total,
			label:   toolName,
			printer: printer,
		}
	}

	if _, err := io.Copy(out, reader); err != nil {
		return err
	}
	return out.Sync()
}

type progressTracker struct {
	reader     io.Reader
	total      int64
	current    int64
	label      string
	printer    *ui.Printer
	lastLogged int
}

func (pt *progressTracker) Read(p []byte) (int, error) {
	n, err := pt.reader.Read(p)
	if n > 0 {
		pt.current += int64(n)
		if pt.total > 0 {
			pct := int((pt.current * 100) / pt.total)
			if pct >= pt.lastLogged+25 || (pct == 100 && pt.lastLogged < 100) {
				pt.lastLogged = (pct / 25) * 25
				if pt.printer != nil {
					mbCurrent := float64(pt.current) / (1024 * 1024)
					mbTotal := float64(pt.total) / (1024 * 1024)
					pt.printer.Info("%s: %d%% (%.1f / %.1f MB)", pt.label, pct, mbCurrent, mbTotal)
				}
			}
		}
	}
	return n, err
}

func verifyFileSHA256(filePath, expectedHex string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expectedHex) {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", actual, expectedHex)
	}
	return nil
}

func extractZip(zipPath, destDir string, stripDirs int) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		targetRel := stripPath(f.Name, stripDirs)
		if targetRel == "" {
			continue
		}

		targetPath, err := safePath(destDir, targetRel)
		if err != nil {
			return err
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(outFile, rc)
		rc.Close()
		closeErr := outFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func extractTarGz(tarGzPath, destDir string, stripDirs int) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		targetRel := stripPath(header.Name, stripDirs)
		if targetRel == "" {
			continue
		}

		targetPath, err := safePath(destDir, targetRel)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			if err := outFile.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func stripPath(p string, count int) string {
	clean := filepath.ToSlash(filepath.Clean(p))
	parts := strings.Split(clean, "/")
	if len(parts) <= count {
		return ""
	}
	return filepath.Join(parts[count:]...)
}

func safePath(destDir, relPath string) (string, error) {
	cleanDest := filepath.Clean(destDir)
	target := filepath.Clean(filepath.Join(cleanDest, relPath))
	if !strings.HasPrefix(target, cleanDest+string(filepath.Separator)) && target != cleanDest {
		return "", fmt.Errorf("illegal path escaping destination: %s", relPath)
	}
	return target, nil
}
