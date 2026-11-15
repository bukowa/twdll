//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"twdll/cli/internal/analyze"
	"twdll/cli/internal/binaries"
	"twdll/cli/internal/cmd"
	"twdll/cli/internal/toolchain"
	"twdll/cli/internal/workspace"
)

type fileSnapshot struct {
	path    string
	exists  bool
	sha256  string
	modTime time.Time
}

type e2eEnv struct {
	repoRoot string
	toolsDir string
	idaDir   string
}

type sandbox struct {
	root   string
	repo   string
	config string
	data   string
	paths  workspace.Paths
	cliBin string
	env    e2eEnv
}

func snapshotFile(t *testing.T, path string) fileSnapshot {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{path: path, exists: false}
		}
		t.Fatalf("stat snapshot target %s: %v", path, err)
	}
	sum, err := workspace.ComputeFileSHA256(path)
	if err != nil {
		t.Fatalf("hash snapshot target %s: %v", path, err)
	}
	return fileSnapshot{
		path:    path,
		exists:  true,
		sha256:  sum,
		modTime: info.ModTime(),
	}
}

func assertSnapshotUnchanged(t *testing.T, snap fileSnapshot) {
	t.Helper()
	info, err := os.Stat(snap.path)
	if !snap.exists {
		if err == nil || !os.IsNotExist(err) {
			t.Errorf("live file %s was unexpectedly created during E2E test", snap.path)
		}
		return
	}
	if err != nil {
		t.Errorf("live file %s disappeared or failed stat after E2E test: %v", snap.path, err)
		return
	}
	sum, err := workspace.ComputeFileSHA256(snap.path)
	if err != nil {
		t.Errorf("hash live file %s after E2E test: %v", snap.path, err)
		return
	}
	if sum != snap.sha256 {
		t.Errorf("live file %s SHA-256 mutated: got %s, want %s", snap.path, sum, snap.sha256)
	}
	if !info.ModTime().Equal(snap.modTime) {
		t.Errorf("live file %s mtime mutated: got %v, want %v", snap.path, info.ModTime(), snap.modTime)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cur, err := filepath.Abs(wd)
	if err != nil {
		t.Fatalf("abs wd: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(cur, "cli", "go.mod")); err == nil && !info.IsDir() {
			return filepath.Clean(cur)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			t.Fatalf("could not locate repo root containing cli/go.mod starting from %s", wd)
		}
		cur = parent
	}
}

func resolveTestEnv(t *testing.T) e2eEnv {
	t.Helper()
	repoRoot := findRepoRoot(t)

	liveConfigSnap := snapshotFile(t, filepath.Join(repoRoot, "twdll.toml"))
	liveStateSnap := snapshotFile(t, filepath.Join(repoRoot, ".twdll", "state.json"))
	t.Cleanup(func() {
		assertSnapshotUnchanged(t, liveConfigSnap)
		assertSnapshotUnchanged(t, liveStateSnap)
	})

	toolsDir := strings.TrimSpace(os.Getenv("TWDLL_TOOLS_DIR"))
	if toolsDir == "" {
		toolsDir = filepath.Join(repoRoot, ".twdll", "tools")
	} else if abs, err := filepath.Abs(filepath.FromSlash(toolsDir)); err == nil {
		toolsDir = filepath.Clean(abs)
	} else {
		toolsDir = filepath.Clean(filepath.FromSlash(toolsDir))
	}

	idaDir := strings.TrimSpace(os.Getenv("TWDLL_E2E_IDA_DIR"))
	if idaDir == "" {
		if cfg, _, err := workspace.LoadConfig(filepath.Join(repoRoot, "twdll.toml")); err == nil && strings.TrimSpace(cfg.IDADir) != "" {
			idaDir = strings.TrimSpace(cfg.IDADir)
		}
	}
	if idaDir == "" {
		idaDir = toolchain.AutoDetectIDADir()
	}

	requiredFiles := []string{
		filepath.Join(toolsDir, "llvm", "bin", "clang++.exe"),
		filepath.Join(toolsDir, "llvm", "bin", "lld-link.exe"),
		filepath.Join(toolsDir, "llvm", "bin", "ld.lld.exe"),
		filepath.Join(toolsDir, "go", "bin", "go.exe"),
		filepath.Join(toolsDir, "ghidra", "support", "analyzeHeadless.bat"),
		filepath.Join(toolsDir, "jdk", "bin", "java.exe"),
	}
	for _, req := range requiredFiles {
		info, err := os.Stat(req)
		if err != nil || info.IsDir() {
			t.Fatalf("required E2E tool binary missing at %s: %v", req, err)
		}
	}
	if !toolchain.IsXWinProvisioned(workspace.Paths{ToolsDir: toolsDir}) {
		t.Fatalf("xwin SDK is not provisioned in %s", toolsDir)
	}
	if err := toolchain.ValidateIDADir(idaDir); err != nil {
		t.Fatalf("valid IDA installation required for real-decompiler E2E (checked %q): %v", idaDir, err)
	}

	return e2eEnv{
		repoRoot: repoRoot,
		toolsDir: toolsDir,
		idaDir:   idaDir,
	}
}

func resolveCLIBinary(t *testing.T, env e2eEnv) string {
	t.Helper()
	if prebuilt := strings.TrimSpace(os.Getenv("TWDLL_E2E_CLI_BIN")); prebuilt != "" {
		abs, err := filepath.Abs(filepath.FromSlash(prebuilt))
		if err == nil {
			prebuilt = filepath.Clean(abs)
		}
		if info, err := os.Stat(prebuilt); err == nil && !info.IsDir() {
			return prebuilt
		}
		t.Fatalf("TWDLL_E2E_CLI_BIN=%q does not point to an existing binary", prebuilt)
	}

	outBin := filepath.Join(t.TempDir(), "twdll.exe")
	goExe := filepath.Join(env.toolsDir, "go", "bin", "go.exe")
	buildCmd := exec.Command(goExe, "build", "-buildvcs=false", "-o", outBin, ".")
	buildCmd.Dir = filepath.Join(env.repoRoot, "cli")
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compile twdll.exe failed: %v\n%s", err, string(out))
	}
	return outBin
}

func compileMiniFixtures(t *testing.T, env e2eEnv, outDir string) (peDir, elfPath string) {
	t.Helper()
	srcPath := filepath.Join(env.repoRoot, "cli", "internal", "e2e", "testdata", "mini_warscape.cpp")
	if _, err := os.Stat(srcPath); err != nil {
		t.Fatalf("fixture source missing at %s: %v", srcPath, err)
	}

	peDir = filepath.Join(outDir, "pe32")
	elfDir := filepath.Join(outDir, "elf64")
	if err := os.MkdirAll(peDir, 0o755); err != nil {
		t.Fatalf("mkdir peDir: %v", err)
	}
	if err := os.MkdirAll(elfDir, 0o755); err != nil {
		t.Fatalf("mkdir elfDir: %v", err)
	}

	clangXX := filepath.Join(env.toolsDir, "llvm", "bin", "clang++.exe")
	lldLink := filepath.Join(env.toolsDir, "llvm", "bin", "lld-link.exe")
	ldLLD := filepath.Join(env.toolsDir, "llvm", "bin", "ld.lld.exe")

	peObj := filepath.Join(peDir, "mini_pe32.obj")
	peDLL := filepath.Join(peDir, "empire.retail.dll")
	runToolCmd(t, clangXX,
		"--target=i686-pc-windows-msvc",
		"-O1", "-g0",
		"-ffreestanding", "-fno-exceptions", "-fno-rtti", "-fno-threadsafe-statics",
		"-c", srcPath,
		"-o", peObj,
	)
	runToolCmd(t, lldLink,
		"/dll",
		"/machine:x86",
		"/entry:MiniDllEntry",
		"/opt:noref",
		"/opt:noicf",
		"/out:"+peDLL,
		peObj,
	)

	elfObj := filepath.Join(elfDir, "mini_elf64.o")
	elfPath = filepath.Join(elfDir, "Attila")
	runToolCmd(t, clangXX,
		"--target=x86_64-unknown-linux-gnu",
		"-O1", "-gdwarf-4", "-g",
		"-ffreestanding", "-fno-exceptions", "-fno-rtti", "-fno-threadsafe-statics", "-fno-pic",
		"-c", srcPath,
		"-o", elfObj,
	)
	runToolCmd(t, ldLLD,
		"-m", "elf_x86_64",
		"--build-id=sha1",
		"-e", "_start",
		"-o", elfPath,
		elfObj,
	)

	peInfo, err := binaries.VerifyPE32(peDLL, workspace.TargetAttila32)
	if err != nil {
		t.Fatalf("VerifyPE32 failed on compiled mini fixture %s: %v", peDLL, err)
	}
	if peInfo.KnownOfficial {
		t.Fatalf("expected synthetic mini PE32 KnownOfficial == false, got true")
	}

	elfInfo, err := binaries.VerifyELF64DWARF(elfPath)
	if err != nil {
		t.Fatalf("VerifyELF64DWARF failed on compiled mini fixture %s: %v", elfPath, err)
	}
	if elfInfo.KnownOfficial {
		t.Fatalf("expected synthetic mini ELF64 KnownOfficial == false, got true")
	}

	return peDir, elfPath
}

func runToolCmd(t *testing.T, exe string, args ...string) {
	t.Helper()
	c := exec.Command(exe, args...)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v failed: %v\n%s", exe, args, err, string(out))
	}
}

func newSandbox(t *testing.T, env e2eEnv, cliBin string) sandbox {
	t.Helper()
	root := strings.TrimSpace(os.Getenv("TWDLL_E2E_SANDBOX_DIR"))
	if root == "" {
		root = t.TempDir()
	} else {
		abs, err := filepath.Abs(filepath.FromSlash(root))
		if err != nil {
			t.Fatalf("abs TWDLL_E2E_SANDBOX_DIR: %v", err)
		}
		root = filepath.Clean(abs)
	}

	repo := filepath.Join(root, "repo")
	config := filepath.Join(repo, "twdll.toml")
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir sandbox repo: %v", err)
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatalf("mkdir sandbox data: %v", err)
	}

	paths := workspace.NewPaths(repo, config, data)
	paths.ToolsDir = env.toolsDir
	return sandbox{
		root:   root,
		repo:   repo,
		config: config,
		data:   data,
		paths:  paths,
		cliBin: cliBin,
		env:    env,
	}
}

func sanitizedEnv(toolsDir string) []string {
	blocked := map[string]bool{
		"TWDLL_ROOT":         true,
		"TWDLL_WRAPPER_ROOT": true,
		"TWDLL_CONFIG":       true,
		"TWDLL_DATA_DIR":     true,
		"TWDLL_TOOLS_DIR":    true,
		"TWDLL_NO_REBUILD":   true,
	}
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if blocked[strings.ToUpper(k)] {
			continue
		}
		out = append(out, kv)
	}
	out = append(out,
		"TWDLL_TOOLS_DIR="+toolsDir,
		"TWDLL_NO_REBUILD=1",
	)
	return out
}

func runCLI(t *testing.T, sb sandbox, args ...string) (string, string) {
	t.Helper()
	fullArgs := []string{
		"--root", sb.repo,
		"--config", sb.config,
		"--data-dir", sb.data,
		"--color", "never",
	}
	fullArgs = append(fullArgs, args...)

	c := exec.Command(sb.cliBin, fullArgs...)
	c.Dir = sb.repo
	c.Env = sanitizedEnv(sb.env.toolsDir)

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		t.Fatalf("twdll %v failed: %v\nstdout:\n%s\nstderr:\n%s", args, err, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

func parseStatusReport(t *testing.T, sb sandbox) cmd.StatusReport {
	t.Helper()
	stdout, _ := runCLI(t, sb, "status", "--json")
	var rep cmd.StatusReport
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("unmarshal status --json failed: %v\nraw:\n%s", err, stdout)
	}
	return rep
}

func readResourceRecords(t *testing.T, path string) ([]analyze.ResourceRecord, []byte) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read resource log %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var records []analyze.ResourceRecord
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec analyze.ResourceRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal resource log line %d: %v\nline: %s", i+1, err, line)
		}
		records = append(records, rec)
	}
	return records, raw
}

func TestE2E_MiniWarscape_RealDecompilers(t *testing.T) {
	env := resolveTestEnv(t)
	cliBin := resolveCLIBinary(t, env)
	sb := newSandbox(t, env, cliBin)

	peDir, elfPath := compileMiniFixtures(t, env, filepath.Join(sb.root, "fixtures"))

	// 1. Initialize workspace (Ghidra then IDA) with explicit game/reference flags.
	runCLI(t, sb,
		"init", "--no-input",
		"--default-game", "attila",
		"--decompiler", "ghidra",
		"--ida-dir", env.idaDir,
		"--attila-dir", peDir,
		"--rome2-dir", peDir,
		"--tob-dir", peDir,
		"--attila-64", elfPath,
	)
	runCLI(t, sb,
		"init", "--no-input",
		"--decompiler", "ida",
		"--ida-dir", env.idaDir,
	)

	// 2. Analyze attila_32 and attila_64 with headless IDA.
	runCLI(t, sb, "analyze", "-d", "ida", "attila_32", "attila_64")
	for _, target := range []workspace.TargetID{workspace.TargetAttila32, workspace.TargetAttila64} {
		if !sb.paths.HasValidDatabase(workspace.DecompilerIDA, target) {
			t.Fatalf("expected valid IDA database for %s at %s", target, sb.paths.DatabaseDir(workspace.DecompilerIDA, target))
		}
		i64Path := filepath.Join(sb.paths.DatabaseDir(workspace.DecompilerIDA, target), string(target)+".i64")
		if info, err := os.Stat(i64Path); err != nil || info.Size() == 0 {
			t.Fatalf("expected non-empty .i64 database at %s (err=%v)", i64Path, err)
		}
		logPath := sb.paths.AnalyzeLogPath(workspace.DecompilerIDA, target)
		if info, err := os.Stat(logPath); err != nil || info.Size() == 0 {
			t.Fatalf("expected non-empty IDA analyze log at %s (err=%v)", logPath, err)
		}
	}

	// 3. Analyze attila_32 and attila_64 with headless Ghidra.
	runCLI(t, sb, "analyze", "-d", "ghidra", "attila_32", "attila_64")
	for _, target := range []workspace.TargetID{workspace.TargetAttila32, workspace.TargetAttila64} {
		if !sb.paths.HasValidDatabase(workspace.DecompilerGhidra, target) {
			t.Fatalf("expected valid Ghidra database for %s at %s", target, sb.paths.DatabaseDir(workspace.DecompilerGhidra, target))
		}
		gprPath := filepath.Join(sb.paths.DatabaseDir(workspace.DecompilerGhidra, target), string(target)+".gpr")
		repDir := filepath.Join(sb.paths.DatabaseDir(workspace.DecompilerGhidra, target), string(target)+".rep")
		if info, err := os.Stat(gprPath); err != nil || info.IsDir() {
			t.Fatalf("expected .gpr file at %s (err=%v)", gprPath, err)
		}
		if info, err := os.Stat(repDir); err != nil || !info.IsDir() {
			t.Fatalf("expected .rep directory at %s (err=%v)", repDir, err)
		}
		logPath := sb.paths.AnalyzeLogPath(workspace.DecompilerGhidra, target)
		if info, err := os.Stat(logPath); err != nil || info.Size() == 0 {
			t.Fatalf("expected non-empty Ghidra analyze log at %s (err=%v)", logPath, err)
		}
	}

	// 4. Re-run IDA analyze to verify fast checkpoint skip and validate analyze_resources.jsonl.
	runCLI(t, sb, "analyze", "-d", "ida", "attila_32", "attila_64")
	records, rawResourceBytes := readResourceRecords(t, sb.paths.AnalyzeResourceLogPath())
	if len(records) != 4 {
		t.Fatalf("expected exactly 4 ResourceRecord entries (skipping already-analyzed targets), got %d", len(records))
	}
	engineCounts := map[string]int{}
	for i, rec := range records {
		engineCounts[rec.Engine]++
		if rec.Status != "ok" {
			t.Fatalf("record %d (%s/%s) expected status \"ok\", got %q (err=%s)", i, rec.Engine, rec.Target, rec.Status, rec.Error)
		}
		if rec.WallSeconds <= 0 {
			t.Fatalf("record %d (%s/%s) expected WallSeconds > 0, got %f", i, rec.Engine, rec.Target, rec.WallSeconds)
		}
	}
	if engineCounts["ida"] != 2 || engineCounts["ghidra"] != 2 {
		t.Fatalf("expected 2 ida and 2 ghidra resource records, got %+v", engineCounts)
	}

	// 5. Inspect status --json.
	rep := parseStatusReport(t, sb)
	if !rep.Initialized {
		t.Fatalf("expected status Initialized == true")
	}
	if !rep.XWinProvisioned {
		t.Fatalf("expected status XWinProvisioned == true")
	}
	if !rep.IDAValid {
		t.Fatalf("expected status IDAValid == true")
	}
	for _, tool := range rep.Tools {
		if !tool.Installed {
			t.Fatalf("expected tool %s Installed == true", tool.ID)
		}
	}
	for _, bin := range rep.Binaries {
		if !bin.Present {
			t.Fatalf("expected binary %s Present == true", bin.Target)
		}
		if bin.KnownOfficial {
			t.Fatalf("expected synthetic binary %s KnownOfficial == false, got true", bin.Target)
		}
	}
	for _, db := range rep.Databases {
		wantAnalyzed := db.Target == workspace.TargetAttila32 || db.Target == workspace.TargetAttila64
		if db.Analyzed != wantAnalyzed {
			t.Fatalf("database %s/%s Analyzed = %v, want %v", db.Engine, db.Target, db.Analyzed, wantAnalyzed)
		}
	}

	// 6. Selective clean: clean -d ida attila_32.
	runCLI(t, sb, "clean", "-d", "ida", "attila_32")
	repAfterClean := parseStatusReport(t, sb)
	for _, db := range repAfterClean.Databases {
		wantAnalyzed := false
		if db.Target == workspace.TargetAttila64 && (db.Engine == "ida" || db.Engine == "ghidra") {
			wantAnalyzed = true
		}
		if db.Target == workspace.TargetAttila32 && db.Engine == "ghidra" {
			wantAnalyzed = true
		}
		if db.Analyzed != wantAnalyzed {
			t.Fatalf("after selective clean, database %s/%s Analyzed = %v, want %v", db.Engine, db.Target, db.Analyzed, wantAnalyzed)
		}
	}

	// 7. Re-initialize with init --clear: must preserve external TWDLL_TOOLS_DIR, analyze logs, and analyzed databases.
	runCLI(t, sb,
		"init", "--clear", "--no-input",
		"--default-game", "attila",
		"--decompiler", "ghidra",
		"--ida-dir", env.idaDir,
		"--attila-dir", peDir,
		"--rome2-dir", peDir,
		"--tob-dir", peDir,
		"--attila-64", elfPath,
	)
	for _, preservedTool := range []string{
		filepath.Join(env.toolsDir, "ghidra", "support", "analyzeHeadless.bat"),
		filepath.Join(env.toolsDir, "llvm", "bin", "clang++.exe"),
	} {
		if info, err := os.Stat(preservedTool); err != nil || info.IsDir() {
			t.Fatalf("external tool %s was missing after init --clear: %v", preservedTool, err)
		}
	}
	if info, err := os.Stat(sb.paths.AnalyzeResourceLogPath()); err != nil || info.Size() == 0 {
		t.Fatalf("expected %s to remain intact after init --clear (err=%v)", sb.paths.AnalyzeResourceLogPath(), err)
	}
	repAfterClear := parseStatusReport(t, sb)
	if !repAfterClear.Initialized {
		t.Fatalf("expected Initialized == true after init --clear")
	}
	for _, db := range repAfterClear.Databases {
		wantAnalyzed := false
		if db.Target == workspace.TargetAttila64 && (db.Engine == "ida" || db.Engine == "ghidra") {
			wantAnalyzed = true
		}
		if db.Target == workspace.TargetAttila32 && db.Engine == "ghidra" {
			wantAnalyzed = true
		}
		if db.Analyzed != wantAnalyzed {
			t.Fatalf("after init --clear, database %s/%s Analyzed = %v, want %v", db.Engine, db.Target, db.Analyzed, wantAnalyzed)
		}
	}

	// 8. Full workspace wipe: clean --all removes config, binaries, state, databases, and logs while preserving external TWDLL_TOOLS_DIR.
	runCLI(t, sb, "clean", "--all")
	for _, preservedTool := range []string{
		filepath.Join(env.toolsDir, "ghidra", "support", "analyzeHeadless.bat"),
		filepath.Join(env.toolsDir, "llvm", "bin", "clang++.exe"),
	} {
		if info, err := os.Stat(preservedTool); err != nil || info.IsDir() {
			t.Fatalf("external tool %s was missing after clean --all: %v", preservedTool, err)
		}
	}
	repAfterCleanAll := parseStatusReport(t, sb)
	if repAfterCleanAll.Initialized {
		t.Fatalf("expected Initialized == false after clean --all")
	}
	for _, bin := range repAfterCleanAll.Binaries {
		if bin.Present {
			t.Fatalf("expected binary %s Present == false after clean --all", bin.Target)
		}
	}
	for _, db := range repAfterCleanAll.Databases {
		if db.Analyzed {
			t.Fatalf("expected database %s/%s Analyzed == false after clean --all", db.Engine, db.Target)
		}
	}

	// Restore archived telemetry log so run-e2e-bg.ps1 -Mode mini can collect analyze_resources.jsonl.
	if err := workspace.WriteFileAtomic(sb.paths.AnalyzeResourceLogPath(), rawResourceBytes, 0o644); err != nil {
		t.Fatalf("restore resource log after clean --all: %v", err)
	}
}

func TestE2E_RealBinaries_Soak(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TWDLL_E2E_SOAK")) != "1" {
		t.Skip("skipping Tier 2 real-binary soak test (set TWDLL_E2E_SOAK=1 to run)")
	}

	env := resolveTestEnv(t)
	cliBin := resolveCLIBinary(t, env)
	sb := newSandbox(t, env, cliBin)

	rawTargets := strings.TrimSpace(os.Getenv("TWDLL_E2E_TARGETS"))
	if rawTargets == "" {
		rawTargets = "attila_32"
	}
	var targets []string
	wantedTarget := map[workspace.TargetID]bool{}
	for _, part := range strings.Split(rawTargets, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		tid := workspace.TargetID(trimmed)
		if !tid.IsValid() {
			t.Fatalf("invalid target %q in TWDLL_E2E_TARGETS=%q", trimmed, rawTargets)
		}
		if !wantedTarget[tid] {
			wantedTarget[tid] = true
			targets = append(targets, trimmed)
		}
	}
	if len(targets) == 0 {
		t.Fatalf("no targets resolved from TWDLL_E2E_TARGETS=%q", rawTargets)
	}

	liveBinsDir := strings.TrimSpace(os.Getenv("TWDLL_E2E_LIVE_BINS_DIR"))
	if liveBinsDir == "" {
		liveBinsDir = filepath.Join(env.repoRoot, ".twdll", "binaries")
	} else if abs, err := filepath.Abs(filepath.FromSlash(liveBinsDir)); err == nil {
		liveBinsDir = filepath.Clean(abs)
	}

	miniPEDir, miniELFPath := compileMiniFixtures(t, env, filepath.Join(sb.root, "fixtures"))

	// Physically copy real attila_32 empire.retail.dll into sandbox source folder.
	realAttila32Src := filepath.Join(liveBinsDir, string(workspace.TargetAttila32), "empire.retail.dll")
	attilaSrcDir := filepath.Join(sb.root, "src_games", "attila")
	attilaStagedSrc := filepath.Join(attilaSrcDir, "empire.retail.dll")
	if err := workspace.CopyFileAtomic(realAttila32Src, attilaStagedSrc); err != nil {
		t.Fatalf("copy real attila_32 binary from %s: %v", realAttila32Src, err)
	}

	initArgs := []string{
		"init", "--no-input",
		"--default-game", "attila",
		"--decompiler", "ghidra",
		"--ida-dir", env.idaDir,
		"--attila-dir", attilaSrcDir,
		"--rome2-dir", miniPEDir,
		"--tob-dir", miniPEDir,
	}

	includeAttila64 := wantedTarget[workspace.TargetAttila64]
	if includeAttila64 {
		// Pre-stage real attila_64 directly into <sb.data>/binaries/attila_64/Attila so init verifies it in place.
		realAttila64Src := filepath.Join(liveBinsDir, string(workspace.TargetAttila64), "Attila")
		destAttila64 := sb.paths.BinaryPath(workspace.TargetAttila64)
		if err := workspace.CopyFileAtomic(realAttila64Src, destAttila64); err != nil {
			t.Fatalf("pre-stage real attila_64 binary from %s: %v", realAttila64Src, err)
		}
	} else {
		initArgs = append(initArgs, "--attila-64", miniELFPath)
	}

	runCLI(t, sb, initArgs...)
	runCLI(t, sb, "init", "--no-input", "--decompiler", "ida", "--ida-dir", env.idaDir)

	idaArgs := append([]string{"analyze", "-d", "ida"}, targets...)
	runCLI(t, sb, idaArgs...)

	ghidraArgs := append([]string{"analyze", "-d", "ghidra"}, targets...)
	runCLI(t, sb, ghidraArgs...)

	rep := parseStatusReport(t, sb)
	if !rep.Initialized || !rep.IDAValid || !rep.XWinProvisioned {
		t.Fatalf("unexpected status flags in soak report: Initialized=%v IDAValid=%v XWinProvisioned=%v",
			rep.Initialized, rep.IDAValid, rep.XWinProvisioned)
	}
	for _, bin := range rep.Binaries {
		if !bin.Present {
			t.Fatalf("expected binary %s Present == true", bin.Target)
		}
		wantOfficial := bin.Target == workspace.TargetAttila64 && includeAttila64
		if bin.KnownOfficial != wantOfficial {
			t.Fatalf("binary %s KnownOfficial = %v, want %v", bin.Target, bin.KnownOfficial, wantOfficial)
		}
	}
	for _, db := range rep.Databases {
		wantAnalyzed := wantedTarget[db.Target] && (db.Engine == "ida" || db.Engine == "ghidra")
		if db.Analyzed != wantAnalyzed {
			t.Fatalf("soak database %s/%s Analyzed = %v, want %v", db.Engine, db.Target, db.Analyzed, wantAnalyzed)
		}
	}

	records, _ := readResourceRecords(t, sb.paths.AnalyzeResourceLogPath())
	wantRecords := len(targets) * 2
	if len(records) != wantRecords {
		t.Fatalf("expected %d ResourceRecord entries, got %d", wantRecords, len(records))
	}
	for i, rec := range records {
		if rec.Status != "ok" {
			t.Fatalf("soak resource record %d (%s/%s) status = %q, want \"ok\" (err=%s)",
				i, rec.Engine, rec.Target, rec.Status, rec.Error)
		}
	}
}
