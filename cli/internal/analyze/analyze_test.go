package analyze

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

func TestBuildPlan_SkipsUpToDateAndDetectsDriftFlagsAndMissingInputs(t *testing.T) {
	// Given an initialized workspace with staged attila_64 and attila_32 binaries
	root := t.TempDir()
	layout := newTestLayout(t, root, workspace.DecompilerGhidra)
	sha64 := stageFakeBinary(t, layout.Paths, workspace.TargetAttila64, []byte("ELF64-ATTILA"))
	sha32 := stageFakeBinary(t, layout.Paths, workspace.TargetAttila32, []byte("PE32-ATTILA"))

	// Seed valid 0-byte Ghidra .gpr + .rep/ and valid non-empty IDA .i64 for attila_64 & attila_32
	for _, tid := range []workspace.TargetID{workspace.TargetAttila64, workspace.TargetAttila32} {
		ghDir := layout.Paths.DatabaseDir(workspace.DecompilerGhidra, tid)
		if err := os.MkdirAll(filepath.Join(ghDir, string(tid)+".rep"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ghDir, string(tid)+".gpr"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		idaMarker := filepath.Join(layout.Paths.DatabaseDir(workspace.DecompilerIDA, tid), string(tid)+".i64")
		if err := os.MkdirAll(filepath.Dir(idaMarker), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(idaMarker, []byte("IDA64"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	state := workspace.NewState()
	state.UpsertBinary(workspace.TargetAttila64, "/src/Attila", sha64)
	state.SetAnalyzed(workspace.TargetAttila64, workspace.DecompilerGhidra, sha64)
	state.SetAnalyzed(workspace.TargetAttila64, workspace.DecompilerIDA, sha64)
	state.UpsertBinary(workspace.TargetAttila32, "/src/empire.retail.dll", sha32)
	state.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerGhidra, sha32)
	state.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerIDA, sha32)

	// When building default Ghidra and IDA plans with matching SHA-256 and valid DBs
	for _, dec := range []string{"ghidra", "ida"} {
		_, items, err := BuildPlan(PlanInput{Layout: layout, State: state, Options: Options{Decompiler: dec}})
		if err != nil {
			t.Fatalf("BuildPlan(%s): %v", dec, err)
		}
		if len(items) != 2 || items[0].NeedsRun || items[1].NeedsRun {
			t.Fatalf("expected both targets skipped for %s, got %+v", dec, items)
		}
	}

	// When --force or --clean is passed, Then NeedsRun becomes true
	for _, opts := range []Options{{Force: true}, {Clean: true}} {
		_, items, err := BuildPlan(PlanInput{Layout: layout, State: state, Options: opts})
		if err != nil {
			t.Fatalf("BuildPlan(%+v): %v", opts, err)
		}
		if !items[0].NeedsRun || !items[1].NeedsRun {
			t.Fatalf("expected NeedsRun=true for %+v, got %+v", opts, items)
		}
	}

	// When attila_32 binary drifts on disk, Then only attila_32 requires re-analysis
	stageFakeBinary(t, layout.Paths, workspace.TargetAttila32, []byte("PE32-PATCHED"))
	_, items, err := BuildPlan(PlanInput{Layout: layout, State: state})
	if err != nil {
		t.Fatalf("BuildPlan after drift: %v", err)
	}
	if items[0].NeedsRun || !items[1].NeedsRun {
		t.Fatalf("expected attila_64 skipped and attila_32 NeedsRun after drift, got %+v", items)
	}

	// When config is missing or requested binary is missing, Then *term.DiagnosticError is returned
	uninit := layout
	uninit.ConfigExists = false
	var diag *term.DiagnosticError
	if _, _, err := BuildPlan(PlanInput{Layout: uninit, State: state}); !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError on missing config, got %T (%v)", err, err)
	}
	if _, _, err := BuildPlan(PlanInput{Layout: layout, State: state, Options: Options{Targets: []string{"rome2_32"}}}); !errors.As(err, &diag) {
		t.Fatalf("expected *term.DiagnosticError on missing binary, got %T (%v)", err, err)
	}
	if _, _, err := BuildPlan(PlanInput{Layout: layout, State: state, Options: Options{Targets: []string{"bogus"}}}); err == nil {
		t.Fatalf("expected error on unknown target")
	}
}

func TestComputeGhidraMaxMem_ScalesAcrossRAMAndPagefileTiers(t *testing.T) {
	// Given zero, 16GB, 16GB+pagefile, 32GB, and 64GB memory profiles
	cases := []struct {
		name   string
		mem    SystemMemory
		target workspace.TargetID
		jobs   int
		want   string
	}{
		{"zero_fallback_64", SystemMemory{}, workspace.TargetAttila64, 1, "16G"},
		{"zero_fallback_32", SystemMemory{}, workspace.TargetAttila32, 2, "8G"},
		{"16gb_no_pagefile_64", SystemMemory{TotalPhysBytes: 16 * giB, TotalCommitBytes: 18 * giB}, workspace.TargetAttila64, 1, "12G"},
		{"16gb_with_pagefile_64", SystemMemory{TotalPhysBytes: 16 * giB, TotalCommitBytes: 28 * giB}, workspace.TargetAttila64, 1, "14G"},
		{"16gb_with_pagefile_32_2jobs", SystemMemory{TotalPhysBytes: 16 * giB, TotalCommitBytes: 24 * giB}, workspace.TargetAttila32, 2, "7G"},
		{"16gb_with_pagefile_32_3jobs", SystemMemory{TotalPhysBytes: 16 * giB, TotalCommitBytes: 24 * giB}, workspace.TargetRome2_32, 3, "4G"},
		{"32gb_64", SystemMemory{TotalPhysBytes: 32 * giB, TotalCommitBytes: 40 * giB}, workspace.TargetAttila64, 1, "24G"},
		{"32gb_32_3jobs", SystemMemory{TotalPhysBytes: 32 * giB, TotalCommitBytes: 40 * giB}, workspace.TargetAttila32, 3, "8G"},
		{"64gb_64", SystemMemory{TotalPhysBytes: 64 * giB, TotalCommitBytes: 72 * giB}, workspace.TargetAttila64, 1, "48G"},
		{"64gb_32_3jobs", SystemMemory{TotalPhysBytes: 64 * giB, TotalCommitBytes: 72 * giB}, workspace.TargetToB32, 3, "8G"},
	}

	// When computing max heap per target/concurrency, Then each tier matches exact bounds
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputeGhidraMaxMem(tc.mem, tc.target, tc.jobs); got != tc.want {
				t.Fatalf("ComputeGhidraMaxMem(%+v, %s, %d) = %q, want %q", tc.mem, tc.target, tc.jobs, got, tc.want)
			}
		})
	}
}

func TestBuildIDABatchScriptAndArgs_TogglesHeavyFlagsWithoutInnerQuotes(t *testing.T) {
	// Given stripped vs full IDC modes and space-containing Windows paths
	stripped := buildIDABatchScript(true)
	full := buildIDABatchScript(false)

	// Then stripped disables AF_STKARG | AF_REGARG while full preserves standard flags
	if !strings.Contains(stripped, "~(AF_STKARG | AF_REGARG)") || !strings.Contains(stripped, "qexit(0);") {
		t.Fatalf("unexpected stripped IDC script:\n%s", stripped)
	}
	if strings.Contains(full, "AF_STKARG") || !strings.Contains(full, "get_inf_attr(INF_AF) | AF_DODATA | AF_FINAL") {
		t.Fatalf("unexpected full IDC script:\n%s", full)
	}

	// When building IDA CLI arguments for paths containing spaces
	scriptPath := `C:\Users\Mod Author\AppData\Local\Temp\twdll ida\analyze_batch.idc`
	dbPath := `C:\Users\Mod Author\AppData\Local\Temp\twdll ida\attila_32.i64`
	logPath := `G:\Projects\TW Attila\.twdll\logs\analyze_ida_attila_32.log`
	binPath := `G:\Projects\TW Attila\.twdll\binaries\attila_32\empire.retail.dll`
	args := buildIDAArgs(scriptPath, dbPath, logPath, binPath)

	// Then flags carry zero inner quotes so Go's EscapeArg quotes each token once
	for _, arg := range args {
		if strings.Contains(arg, `"`) {
			t.Fatalf("IDA argument %q must not contain inner quotes", arg)
		}
	}
	if args[2] != "-S"+scriptPath || args[3] != "-o"+dbPath || args[4] != "-L"+logPath || args[5] != binPath {
		t.Fatalf("unexpected IDA args: %#v", args)
	}
}

func TestResolveWorkerCounts_RespectsAvailableMemoryAndExplicitJobs(t *testing.T) {
	// Given high, medium, and low available physical RAM tiers plus explicit job overrides
	if got := resolveGhidra32WorkerCount(3, 2, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 4 * giB}); got != 2 {
		t.Fatalf("explicit jobs should win for Ghidra, got %d", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{}); got != 1 {
		t.Fatalf("zero memory fallback for Ghidra should be 1, got %d", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 20 * giB}); got != 3 {
		t.Fatalf("32GB total / 20GB avail Ghidra workers = %d, want 3", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 11 * giB}); got != 2 {
		t.Fatalf("32GB total / 11GB avail Ghidra workers = %d, want 2", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 6 * giB}); got != 1 {
		t.Fatalf("32GB total / 6GB avail Ghidra workers = %d, want 1", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{AvailPhysBytes: 18 * giB}); got != 3 {
		t.Fatalf("avail-only 18GB Ghidra workers = %d, want 3", got)
	}

	if got := resolveIDA32WorkerCount(3, 3, 2, SystemMemory{AvailPhysBytes: 3 * giB}); got != 3 {
		t.Fatalf("explicit jobs should win for IDA, got %d", got)
	}
	if got := resolveIDA32WorkerCount(3, 0, 12, SystemMemory{}); got != 3 {
		t.Fatalf("zero memory fallback for IDA with 12 CPUs = %d, want 3", got)
	}
	if got := resolveIDA32WorkerCount(3, 0, 12, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 10 * giB}); got != 3 {
		t.Fatalf("10GB avail IDA workers = %d, want 3", got)
	}
	if got := resolveIDA32WorkerCount(3, 0, 12, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 6 * giB}); got != 2 {
		t.Fatalf("6GB avail IDA workers = %d, want 2", got)
	}
	if got := resolveIDA32WorkerCount(3, 0, 12, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 3 * giB}); got != 1 {
		t.Fatalf("3GB avail IDA workers = %d, want 1", got)
	}

	// And ComputeGhidraMaxMem caps Xmx when AvailPhysBytes is constrained
	if got := ComputeGhidraMaxMem(SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 12 * giB}, workspace.TargetAttila32, 2); got != "5G" {
		t.Fatalf("ComputeGhidraMaxMem(32GB total, 12GB avail, attila_32, 2) = %q, want 5G", got)
	}
	if got := ComputeGhidraMaxMem(SystemMemory{TotalPhysBytes: 64 * giB, AvailPhysBytes: 14 * giB}, workspace.TargetAttila64, 1); got != "12G" {
		t.Fatalf("ComputeGhidraMaxMem(64GB total, 14GB avail, attila_64, 1) = %q, want 12G", got)
	}
}

func TestFormatResourceSummaryAndJSONLAppend(t *testing.T) {
	// Given populated ProcessMetrics and empty ProcessMetrics
	summary := formatResourceSummary(ProcessMetrics{
		CPUTime:            12300 * time.Millisecond,
		PeakJobMemoryBytes: 5 * giB / 2,
		AvailPhysMinBytes:  18 * giB,
	}, "8G")
	if summary != "peak 2.5 GB, cpu 12.3s, min-avail 18.0 GB, Xmx=8G" {
		t.Fatalf("unexpected resource summary: %q", summary)
	}
	if empty := formatResourceSummary(ProcessMetrics{}, ""); empty != "" {
		t.Fatalf("expected empty summary for zero metrics, got %q", empty)
	}

	// When appending two ResourceRecords to analyze_resources.jsonl
	logPath := filepath.Join(t.TempDir(), "logs", "analyze_resources.jsonl")
	rec1 := ResourceRecord{Timestamp: "2026-11-13T00:00:00Z", Engine: "ghidra", Target: "attila_32", Status: "ok", ConcurrentJobs: 2, GhidraMaxMem: "8G", JobAssigned: true}
	rec2 := ResourceRecord{Timestamp: "2026-11-13T00:01:00Z", Engine: "ghidra", Target: "attila_64", Status: "ok", ConcurrentJobs: 1, GhidraMaxMem: "24G", JobAssigned: true}
	if err := appendResourceRecord(logPath, rec1); err != nil {
		t.Fatalf("appendResourceRecord(1): %v", err)
	}
	if err := appendResourceRecord(logPath, rec2); err != nil {
		t.Fatalf("appendResourceRecord(2): %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d (%q)", len(lines), string(raw))
	}
	var got1, got2 ResourceRecord
	if err := json.Unmarshal([]byte(lines[0]), &got1); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &got2); err != nil {
		t.Fatalf("unmarshal line 2: %v", err)
	}
	if got1.Target != "attila_32" || got1.ConcurrentJobs != 2 || got2.Target != "attila_64" || got2.ConcurrentJobs != 1 {
		t.Fatalf("unexpected JSONL records: %+v, %+v", got1, got2)
	}
}

func newTestLayout(t *testing.T, root string, engine workspace.DecompilerID) workspace.Layout {
	t.Helper()
	cfgPath := filepath.Join(root, "twdll.toml")
	cfg := workspace.Config{
		DataDir:     ".twdll",
		DefaultGame: workspace.GameAttila,
		Decompiler:  engine,
		Games:       workspace.GamesConfig{Attila: filepath.Join(root, "SteamAttila")},
	}
	if err := workspace.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	layout, err := workspace.Resolve(workspace.ResolveSpec{
		RootOverride:   root,
		ConfigOverride: cfgPath,
		WorkingDir:     root,
	})
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func stageFakeBinary(t *testing.T, paths workspace.Paths, target workspace.TargetID, content []byte) string {
	t.Helper()
	binPath := paths.BinaryPath(target)
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sha, err := workspace.ComputeFileSHA256(binPath)
	if err != nil {
		t.Fatal(err)
	}
	return sha
}
