package toolchain

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type ExtractProgress struct {
	DoneBytes  int64
	TotalBytes int64
	DoneFiles  int
	TotalFiles int
}

type ExtractSpec struct {
	ArchivePath string
	ArchiveType string
	DestDir     string
	StripDirs   int
	OnProgress  func(ExtractProgress)
}

type countingReader struct {
	r      io.Reader
	onRead func(int64)
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	if n > 0 && cr.onRead != nil {
		cr.onRead(int64(n))
	}
	return n, err
}

func ExtractArchive(ctx context.Context, spec ExtractSpec) error {
	if err := os.MkdirAll(spec.DestDir, 0o755); err != nil {
		return fmt.Errorf("create extract destination %s: %w", spec.DestDir, err)
	}
	switch strings.ToLower(strings.TrimSpace(spec.ArchiveType)) {
	case "zip":
		return extractZip(ctx, spec)
	case "tar.gz":
		return extractTarGz(ctx, spec)
	case "tar.xz":
		return extractTarXz(ctx, spec)
	default:
		return fmt.Errorf("unsupported archive type %q", spec.ArchiveType)
	}
}

func extractZip(ctx context.Context, spec ExtractSpec) error {
	zr, err := zip.OpenReader(spec.ArchivePath)
	if err != nil {
		return fmt.Errorf("open zip %s: %w", spec.ArchivePath, err)
	}
	defer zr.Close()

	var totalBytes int64
	var totalFiles int
	for _, f := range zr.File {
		_, ok, err := stripAndValidateRelPath(f.Name, spec.StripDirs)
		if err != nil {
			return err
		}
		if !ok || f.FileInfo().IsDir() {
			continue
		}
		totalFiles++
		totalBytes += int64(f.UncompressedSize64)
	}

	var doneBytes int64
	var doneFiles int
	report := func() {
		if spec.OnProgress != nil {
			spec.OnProgress(ExtractProgress{
				DoneBytes:  doneBytes,
				TotalBytes: totalBytes,
				DoneFiles:  doneFiles,
				TotalFiles: totalFiles,
			})
		}
	}
	if totalBytes > 0 || totalFiles > 0 {
		report()
	}

	if err := os.MkdirAll(spec.DestDir, 0o755); err != nil {
		return fmt.Errorf("create extract destination %s: %w", spec.DestDir, err)
	}
	root, err := os.OpenRoot(spec.DestDir)
	if err != nil {
		return fmt.Errorf("open extract root %s: %w", spec.DestDir, err)
	}
	defer root.Close()

	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, ok, err := stripAndValidateRelPath(f.Name, spec.StripDirs)
		if err != nil || !ok {
			if err != nil {
				return err
			}
			continue
		}
		if f.FileInfo().IsDir() {
			if err := root.MkdirAll(rel, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", rel, err)
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", f.Name, err)
		}
		cr := &countingReader{
			r: rc,
			onRead: func(n int64) {
				doneBytes += n
				if doneBytes > totalBytes && totalBytes > 0 {
					doneBytes = totalBytes
				}
				report()
			},
		}
		wErr := writeArchiveFile(root, rel, f.Mode(), cr)
		_ = rc.Close()
		if wErr != nil {
			return wErr
		}
		doneFiles++
		report()
	}
	if totalBytes > 0 || totalFiles > 0 {
		doneBytes = totalBytes
		doneFiles = totalFiles
		report()
	}
	return nil
}

func extractTarGz(ctx context.Context, spec ExtractSpec) error {
	f, err := os.Open(spec.ArchivePath)
	if err != nil {
		return fmt.Errorf("open tar.gz %s: %w", spec.ArchivePath, err)
	}
	defer f.Close()

	var totalBytes int64
	if info, sErr := f.Stat(); sErr == nil {
		totalBytes = info.Size()
	}
	var doneBytes int64
	var doneFiles int
	report := func() {
		if spec.OnProgress != nil {
			spec.OnProgress(ExtractProgress{
				DoneBytes:  doneBytes,
				TotalBytes: totalBytes,
				DoneFiles:  doneFiles,
			})
		}
	}
	cr := &countingReader{
		r: f,
		onRead: func(n int64) {
			doneBytes += n
			if doneBytes > totalBytes && totalBytes > 0 {
				doneBytes = totalBytes
			}
			report()
		},
	}

	gz, err := gzip.NewReader(cr)
	if err != nil {
		return fmt.Errorf("open gzip stream %s: %w", spec.ArchivePath, err)
	}
	defer gz.Close()

	if err := os.MkdirAll(spec.DestDir, 0o755); err != nil {
		return fmt.Errorf("create extract destination %s: %w", spec.DestDir, err)
	}
	root, err := os.OpenRoot(spec.DestDir)
	if err != nil {
		return fmt.Errorf("open extract root %s: %w", spec.DestDir, err)
	}
	defer root.Close()

	tr := tar.NewReader(gz)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			if totalBytes > 0 || doneFiles > 0 {
				doneBytes = totalBytes
				report()
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry in %s: %w", spec.ArchivePath, err)
		}
		rel, ok, err := stripAndValidateRelPath(hdr.Name, spec.StripDirs)
		if err != nil || !ok {
			if err != nil {
				return err
			}
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(rel, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", rel, err)
			}
		case tar.TypeReg:
			if err := writeArchiveFile(root, rel, os.FileMode(hdr.Mode), tr); err != nil {
				return err
			}
			doneFiles++
			report()
		}
	}
}

func extractTarXz(ctx context.Context, spec ExtractSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.Open(spec.ArchivePath)
	if err != nil {
		return fmt.Errorf("open tar.xz %s: %w", spec.ArchivePath, err)
	}
	defer f.Close()

	var totalBytes int64
	if info, sErr := f.Stat(); sErr == nil {
		totalBytes = info.Size()
	}
	var doneBytes int64
	report := func() {
		if spec.OnProgress != nil {
			spec.OnProgress(ExtractProgress{
				DoneBytes:  doneBytes,
				TotalBytes: totalBytes,
			})
		}
	}
	cr := &countingReader{
		r: f,
		onRead: func(n int64) {
			doneBytes += n
			if doneBytes > totalBytes && totalBytes > 0 {
				doneBytes = totalBytes
			}
			report()
		},
	}

	tarBin := "tar"
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(`C:\Windows\System32\tar.exe`); err == nil {
			tarBin = `C:\Windows\System32\tar.exe`
		}
	}
	cmd := exec.CommandContext(ctx, tarBin, "-xJf", "-", "-C", spec.DestDir, "--strip-components="+strconv.Itoa(spec.StripDirs))
	cmd.Stdin = cr
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		outStr := strings.TrimSpace(stdoutBuf.String() + "\n" + stderrBuf.String())
		return fmt.Errorf("extract tar.xz %s with %s: %w (%s)", spec.ArchivePath, tarBin, err, outStr)
	}
	if totalBytes > 0 {
		doneBytes = totalBytes
		report()
	}
	return nil
}

func stripAndValidateRelPath(rawName string, stripDirs int) (string, bool, error) {
	trimmed := strings.Trim(strings.TrimPrefix(strings.ReplaceAll(rawName, "\\", "/"), "./"), "/")
	if trimmed == "" {
		return "", false, nil
	}
	var cleanParts []string
	for _, p := range strings.Split(trimmed, "/") {
		if p == "" || p == "." {
			continue
		}
		if p == ".." || strings.ContainsRune(p, ':') {
			return "", false, fmt.Errorf("unsafe archive entry path %q", rawName)
		}
		cleanParts = append(cleanParts, p)
	}
	if len(cleanParts) <= stripDirs {
		return "", false, nil
	}
	rel := filepath.Join(cleanParts[stripDirs:]...)
	if !filepath.IsLocal(rel) {
		return "", false, fmt.Errorf("unsafe archive entry path %q", rawName)
	}
	return rel, true, nil
}

func writeArchiveFile(root *os.Root, relPath string, mode fs.FileMode, r io.Reader) error {
	if dir := filepath.Dir(relPath); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir parent for %s: %w", relPath, err)
		}
	}
	perm := mode.Perm()
	if perm == 0 {
		perm = 0o644
	}
	f, err := root.OpenFile(relPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("create extracted file %s: %w", relPath, err)
	}
	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("write extracted file %s: %w", relPath, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close extracted file %s: %w", relPath, closeErr)
	}
	return nil
}
