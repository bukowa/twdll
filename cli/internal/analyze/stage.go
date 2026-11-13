package analyze

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"twdll/cli/internal/workspace"
)

// prepareStageEnv isolates temporary directories inside stageDir.
// isolateProfile redirects APPDATA/LOCALAPPDATA/XDG_* for JVM/Ghidra caches,
// while leaving isolateProfile=false for IDA Pro preserves %APPDATA%\Hex-Rays license access.
func prepareStageEnv(stageDir string, isolateProfile bool, extra map[string]string) ([]string, error) {
	envRoot := filepath.Join(stageDir, "env")
	tmpDir := filepath.Join(envRoot, "tmp")
	dirs := []string{tmpDir}

	overrides := map[string]string{
		"TEMP":   tmpDir,
		"TMP":    tmpDir,
		"TMPDIR": tmpDir,
	}
	if isolateProfile {
		appData := filepath.Join(envRoot, "appdata")
		localAppData := filepath.Join(envRoot, "localappdata")
		xdgConfig := filepath.Join(envRoot, "xdg_config")
		xdgCache := filepath.Join(envRoot, "xdg_cache")
		dirs = append(dirs, appData, localAppData, xdgConfig, xdgCache)
		overrides["APPDATA"] = appData
		overrides["LOCALAPPDATA"] = localAppData
		overrides["XDG_CONFIG_HOME"] = xdgConfig
		overrides["XDG_CACHE_HOME"] = xdgCache
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create isolated env dir %s: %w", dir, err)
		}
	}
	for k, v := range extra {
		overrides[k] = v
	}

	normKey := func(k string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(k)
		}
		return k
	}

	overridden := make(map[string]bool, len(overrides))
	for k := range overrides {
		overridden[normKey(k)] = true
	}

	base := os.Environ()
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			out = append(out, kv)
			continue
		}
		if overridden[normKey(k)] {
			continue
		}
		out = append(out, kv)
	}

	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+overrides[k])
	}
	return out, nil
}

func moveFile(src, dst string) error {
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("ensure dir %s: %w", dstDir, err)
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	_ = os.Remove(dst)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := workspace.CopyFileAtomic(src, dst); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

func moveDir(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("ensure parent dir for %s: %w", dst, err)
	}
	_ = os.RemoveAll(dst)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		return workspace.CopyFileAtomic(path, targetPath)
	}); err != nil {
		return fmt.Errorf("copy directory %s to %s: %w", src, dst, err)
	}
	return os.RemoveAll(src)
}
