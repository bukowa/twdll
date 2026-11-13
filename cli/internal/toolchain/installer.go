package toolchain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

type InstallEnv struct {
	Paths workspace.Paths
	State *workspace.State
	UI    *term.UI
}

type stageRequest struct {
	paths       workspace.Paths
	tool        ToolDef
	archivePath string
	ui          *term.UI
}

func IsInstalled(paths workspace.Paths, tool ToolDef) bool {
	if tool.MarkerRel == "" {
		return false
	}
	marker := filepath.Join(paths.ToolDir(tool.ID), filepath.FromSlash(tool.MarkerRel))
	info, err := os.Stat(marker)
	return err == nil && !info.IsDir()
}

func IsXWinProvisioned(paths workspace.Paths) bool {
	xwinDir := paths.ToolDir(workspace.ToolXwin)
	direct := filepath.Join(xwinDir, "sdk", "crt", "lib", "x86", "libcmt.lib")
	if info, err := os.Stat(direct); err == nil && !info.IsDir() {
		return true
	}
	matches, err := filepath.Glob(filepath.Join(xwinDir, "sdk", "VC", "Tools", "MSVC", "*", "lib", "x86", "libcmt.lib"))
	return err == nil && len(matches) > 0
}

func EnsureTool(ctx context.Context, env InstallEnv, tool ToolDef) error {
	if IsInstalled(env.Paths, tool) {
		if env.State != nil && env.State.Tools[tool.ID].Version != tool.Version {
			env.State.SetTool(tool.ID, workspace.ToolState{Version: tool.Version, SHA256: tool.SHA256})
		}
		if env.UI != nil {
			env.UI.Success("%s %s ready", tool.Name, tool.Version)
		}
		return nil
	}
	if err := os.MkdirAll(env.Paths.ToolsDir, 0o755); err != nil {
		return fmt.Errorf("ensure tools dir %s: %w", env.Paths.ToolsDir, err)
	}
	archivePath, err := downloadToolArchive(ctx, env, tool)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(archivePath) }()

	sp := env.UI.StartSpinner("Verifying " + tool.Name + " SHA-256")
	gotSHA, err := workspace.ComputeFileSHA256(archivePath)
	sp.Stop()
	if err != nil {
		return fmt.Errorf("compute SHA-256 for %s: %w", tool.Name, err)
	}
	if !strings.EqualFold(gotSHA, tool.SHA256) {
		return fmt.Errorf("%s SHA-256 mismatch: got %s, expected %s", tool.Name, gotSHA, tool.SHA256)
	}
	if err := stageAndSwapTool(ctx, stageRequest{paths: env.Paths, tool: tool, archivePath: archivePath, ui: env.UI}); err != nil {
		return err
	}
	if env.State != nil {
		env.State.SetTool(tool.ID, workspace.ToolState{Version: tool.Version, SHA256: tool.SHA256})
	}
	if env.UI != nil {
		env.UI.Success("Installed %s %s", tool.Name, tool.Version)
	}
	return nil
}

func downloadToolArchive(ctx context.Context, env InstallEnv, tool ToolDef) (string, error) {
	if env.UI != nil {
		env.UI.Info("Downloading %s %s...", tool.Name, tool.Version)
	}
	sp := env.UI.StartSpinner("Downloading " + tool.Name + " " + tool.Version)
	defer sp.Stop()

	tmpFile, err := os.CreateTemp(env.Paths.ToolsDir, string(tool.ID)+"-*.archive")
	if err != nil {
		return "", fmt.Errorf("create temp download file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	client := &http.Client{Timeout: 30 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tool.URL, nil)
	if err != nil {
		return "", fmt.Errorf("build download request for %s: %w", tool.Name, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", tool.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s failed with HTTP %d", tool.Name, resp.StatusCode)
	}
	if err := copyWithProgress(tmpFile, resp.Body, downloadLog{ui: env.UI, sp: sp, label: tool.Name, total: resp.ContentLength}); err != nil {
		return "", fmt.Errorf("save %s archive: %w", tool.Name, err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close %s archive: %w", tool.Name, err)
	}
	cleanup = false
	return tmpPath, nil
}

type downloadLog struct {
	ui    *term.UI
	sp    *term.Spinner
	label string
	total int64
}

func copyWithProgress(dst io.Writer, src io.Reader, log downloadLog) error {
	buf := make([]byte, 64*1024)
	var written int64
	nextMilestone := 25
	animated := log.ui != nil && log.ui.CanAnimate()
	for {
		n, rErr := src.Read(buf)
		if n > 0 {
			if _, wErr := dst.Write(buf[:n]); wErr != nil {
				return wErr
			}
			written += int64(n)
			if animated {
				if log.total > 0 {
					pct := int((written * 100) / log.total)
					if log.sp != nil {
						log.sp.SetText(fmt.Sprintf("Downloading %s: %d%% (%d / %d MB)", log.label, pct, written/(1024*1024), log.total/(1024*1024)))
					}
				} else if log.sp != nil {
					log.sp.SetText(fmt.Sprintf("Downloading %s: %d MB", log.label, written/(1024*1024)))
				}
			} else if log.ui != nil && log.total > 0 {
				pct := int((written * 100) / log.total)
				for pct >= nextMilestone && nextMilestone <= 100 {
					log.ui.Info("  %s: %d%%", log.label, nextMilestone)
					nextMilestone += 25
				}
			}
		}
		if errors.Is(rErr, io.EOF) {
			return nil
		}
		if rErr != nil {
			return rErr
		}
	}
}

func stageAndSwapTool(ctx context.Context, req stageRequest) error {
	stageDir, err := os.MkdirTemp(req.paths.ToolsDir, string(req.tool.ID)+"-stage-*")
	if err != nil {
		return fmt.Errorf("create staging dir for %s: %w", req.tool.Name, err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()

	sp := req.ui.StartSpinner("Extracting " + req.tool.Name + " " + req.tool.Version)
	defer sp.Stop()

	animated := req.ui != nil && req.ui.CanAnimate()
	loggedExtractHeader := false
	nextMilestone := 25
	spec := ExtractSpec{
		ArchivePath: req.archivePath,
		ArchiveType: req.tool.ArchiveType,
		DestDir:     stageDir,
		StripDirs:   req.tool.StripDirs,
		OnProgress: func(p ExtractProgress) {
			if animated {
				if p.TotalBytes > 0 {
					pct := int((p.DoneBytes * 100) / p.TotalBytes)
					sp.SetText(fmt.Sprintf("Extracting %s: %d%%", req.tool.Name, pct))
				} else {
					sp.SetText(fmt.Sprintf("Extracting %s: %d files", req.tool.Name, p.DoneFiles))
				}
				return
			}
			if req.ui == nil {
				return
			}
			if !loggedExtractHeader {
				req.ui.Info("Extracting %s %s...", req.tool.Name, req.tool.Version)
				loggedExtractHeader = true
			}
			if p.TotalBytes > 0 {
				pct := int((p.DoneBytes * 100) / p.TotalBytes)
				for pct >= nextMilestone && nextMilestone <= 100 {
					req.ui.Info("  %s extract: %d%%", req.tool.Name, nextMilestone)
					nextMilestone += 25
				}
			}
		},
	}
	if err := ExtractArchive(ctx, spec); err != nil {
		return fmt.Errorf("extract %s: %w", req.tool.Name, err)
	}
	markerPath := filepath.Join(stageDir, filepath.FromSlash(req.tool.MarkerRel))
	if info, err := os.Stat(markerPath); err != nil || info.IsDir() {
		return fmt.Errorf("extracted %s is missing expected marker %s", req.tool.Name, req.tool.MarkerRel)
	}
	sp.SetText("Installing " + req.tool.Name + "...")
	finalDir := req.paths.ToolDir(req.tool.ID)
	_ = os.RemoveAll(finalDir)
	if err := os.Rename(stageDir, finalDir); err != nil {
		return fmt.Errorf("install %s into %s: %w", req.tool.Name, finalDir, err)
	}
	return nil
}

// ProvisionXWin passes --disable-symlinks to avoid Windows privilege error 1314 on non-elevated shells.
func ProvisionXWin(ctx context.Context, paths workspace.Paths, ui *term.UI) error {
	if IsXWinProvisioned(paths) {
		if ui != nil {
			ui.Success("MSVC SDK (xwin x86) ready")
		}
		return nil
	}
	xwinDir := paths.ToolDir(workspace.ToolXwin)
	xwinBin := filepath.Join(xwinDir, "xwin"+workspace.ExeSuffix())
	sdkDir := filepath.Join(xwinDir, "sdk")
	if ui != nil {
		ui.Info("Provisioning 32-bit MSVC & Windows SDK via xwin...")
	}
	sp := ui.StartSpinner("Provisioning MSVC & Windows SDK (xwin x86)")
	defer sp.Stop()

	cmd := exec.CommandContext(ctx, xwinBin, "--accept-license", "--arch", "x86", "splat", "--output", sdkDir, "--use-winsysroot-style", "--disable-symlinks")
	var diagBuf bytes.Buffer
	lw := &xwinLineWriter{onLine: func(line string) {
		sp.SetText("Provisioning MSVC SDK: " + line)
	}}
	mw := io.MultiWriter(&diagBuf, lw)
	cmd.Stdout = mw
	cmd.Stderr = mw
	runErr := cmd.Run()
	lw.Flush()
	if runErr != nil {
		return fmt.Errorf("xwin splat failed: %w (%s)", runErr, strings.TrimSpace(diagBuf.String()))
	}
	if !IsXWinProvisioned(paths) {
		return errors.New("xwin completed but libcmt.lib was not found in x86 SDK output")
	}
	if ui != nil {
		ui.Success("Provisioned MSVC SDK (xwin x86)")
	}
	return nil
}

type xwinLineWriter struct {
	buf    bytes.Buffer
	onLine func(string)
}

func (w *xwinLineWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' || b == '\r' {
			w.emit()
			continue
		}
		w.buf.WriteByte(b)
	}
	return len(p), nil
}

func (w *xwinLineWriter) Flush() {
	w.emit()
}

func (w *xwinLineWriter) emit() {
	if w.buf.Len() == 0 {
		return
	}
	line := strings.TrimSpace(w.buf.String())
	w.buf.Reset()
	if line != "" && w.onLine != nil {
		w.onLine(line)
	}
}
