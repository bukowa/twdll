package analyze

import (
	"bytes"
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

	// When --clear is passed, Then NeedsRun becomes true
	_, clearItems, err := BuildPlan(PlanInput{Layout: layout, State: state, Options: Options{Clear: true}})
	if err != nil {
		t.Fatalf("BuildPlan(Clear): %v", err)
	}
	if !clearItems[0].NeedsRun || !clearItems[1].NeedsRun {
		t.Fatalf("expected NeedsRun=true for Clear, got %+v", clearItems)
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

	// Glob patterns resolve against configured targets (skipping unconfigured 32-bit targets like tob_32).
	_, globItems, err := BuildPlan(PlanInput{Layout: layout, State: state, Options: Options{Targets: []string{"*_32"}}})
	if err != nil {
		t.Fatalf("BuildPlan(*_32): %v", err)
	}
	if len(globItems) != 1 || globItems[0].Target != workspace.TargetAttila32 {
		t.Fatalf("BuildPlan(*_32) = %+v, want only attila_32", globItems)
	}
	_, attilaItems, err := BuildPlan(PlanInput{Layout: layout, State: state, Options: Options{Targets: []string{"attila_*"}}})
	if err != nil {
		t.Fatalf("BuildPlan(attila_*): %v", err)
	}
	if len(attilaItems) != 2 || attilaItems[0].Target != workspace.TargetAttila64 || attilaItems[1].Target != workspace.TargetAttila32 {
		t.Fatalf("BuildPlan(attila_*) = %+v, want [attila_64, attila_32]", attilaItems)
	}
}

func TestComputeGhidraMaxMem_ScalesAcrossRAMAndPagefileTiers(t *testing.T) {
	// Given zero, 8GB, 12GB, 14GB, 15GB, 16GB, 24GB, 32GB, and 64GB memory profiles
	cases := []struct {
		name   string
		mem    SystemMemory
		target workspace.TargetID
		full   bool
		jobs   int
		want   string
	}{
		{"zero_fallback_64_default", SystemMemory{}, workspace.TargetAttila64, false, 1, "12G"},
		{"zero_fallback_64_full", SystemMemory{}, workspace.TargetAttila64, true, 1, "28G"},
		{"zero_fallback_32", SystemMemory{}, workspace.TargetAttila32, false, 2, "6G"},
		{"8gb_64_default", SystemMemory{TotalPhysBytes: 8 * giB}, workspace.TargetAttila64, false, 1, "8G"},
		{"12gb_64_default", SystemMemory{TotalPhysBytes: 12 * giB}, workspace.TargetAttila64, false, 1, "8G"},
		{"14gb_64_default", SystemMemory{TotalPhysBytes: 14 * giB}, workspace.TargetAttila64, false, 1, "8G"},
		{"15gb_64_default", SystemMemory{TotalPhysBytes: 15 * giB}, workspace.TargetAttila64, false, 1, "9G"},
		{"16gb_64_default", SystemMemory{TotalPhysBytes: 16 * giB}, workspace.TargetAttila64, false, 1, "10G"},
		{"24gb_64_default", SystemMemory{TotalPhysBytes: 24 * giB}, workspace.TargetAttila64, false, 1, "16G"},
		{"32gb_64_default", SystemMemory{TotalPhysBytes: 32 * giB}, workspace.TargetAttila64, false, 1, "16G"},
		{"64gb_64_default", SystemMemory{TotalPhysBytes: 64 * giB}, workspace.TargetAttila64, false, 1, "16G"},
		{"64gb_low_avail_64_immune", SystemMemory{TotalPhysBytes: 64 * giB, AvailPhysBytes: 9 * giB}, workspace.TargetAttila64, false, 1, "16G"},
		{"16gb_64_full", SystemMemory{TotalPhysBytes: 16 * giB}, workspace.TargetAttila64, true, 1, "22G"},
		{"32gb_64_full", SystemMemory{TotalPhysBytes: 32 * giB}, workspace.TargetAttila64, true, 1, "26G"},
		{"64gb_64_full", SystemMemory{TotalPhysBytes: 64 * giB}, workspace.TargetAttila64, true, 1, "36G"},
		{"8gb_32_1job", SystemMemory{TotalPhysBytes: 8 * giB}, workspace.TargetAttila32, false, 1, "5G"},
		{"12gb_32_1job", SystemMemory{TotalPhysBytes: 12 * giB}, workspace.TargetAttila32, false, 1, "6G"},
		{"16gb_32_1job", SystemMemory{TotalPhysBytes: 16 * giB}, workspace.TargetAttila32, false, 1, "6G"},
		{"16gb_32_2jobs", SystemMemory{TotalPhysBytes: 16 * giB}, workspace.TargetAttila32, false, 2, "5G"},
		{"24gb_32_2jobs", SystemMemory{TotalPhysBytes: 24 * giB}, workspace.TargetRome2_32, false, 2, "6G"},
		{"32gb_32_3jobs", SystemMemory{TotalPhysBytes: 32 * giB}, workspace.TargetAttila32, false, 3, "6G"},
		{"64gb_32_3jobs", SystemMemory{TotalPhysBytes: 64 * giB}, workspace.TargetToB32, false, 3, "6G"},
	}

	// When computing max heap per target/concurrency, Then each tier matches exact bounds
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputeGhidraMaxMem(tc.mem, tc.target, tc.full, tc.jobs); got != tc.want {
				t.Fatalf("ComputeGhidraMaxMem(%+v, %s, full=%v, jobs=%d) = %q, want %q", tc.mem, tc.target, tc.full, tc.jobs, got, tc.want)
			}
		})
	}
}

func TestCollectGhidraMemoryWarnings_WarnsOnLowRAMWithoutBlocking(t *testing.T) {
	heavy64 := []PlanItem{{
		Target:   workspace.TargetAttila64,
		NeedsRun: true,
		Traits:   BinaryTraits{Format: "elf", HasDWARF: true, IsHeavyDWARF: true},
	}}
	pe32 := []PlanItem{{
		Target:   workspace.TargetAttila32,
		NeedsRun: true,
		Traits:   BinaryTraits{Format: "pe"},
	}}

	// 8 GB and 12 GB hosts warn on heavy-DWARF attila_64
	for _, gb := range []uint64{8, 12} {
		warns := CollectGhidraMemoryWarnings(SystemMemory{TotalPhysBytes: gb * giB}, heavy64, Options{})
		if len(warns) != 1 || !strings.Contains(warns[0], "-Xmx8G") || !strings.Contains(warns[0], "swap/pagefile") {
			t.Fatalf("expected 1 swap warning on %d GiB host, got %#v", gb, warns)
		}
	}

	// 14 GB and 16 GB hosts emit zero warnings for default attila_64, but warn on --full-analysis
	for _, gb := range []uint64{14, 16} {
		if warns := CollectGhidraMemoryWarnings(SystemMemory{TotalPhysBytes: gb * giB}, heavy64, Options{}); len(warns) != 0 {
			t.Fatalf("expected 0 warnings on %d GiB default attila_64, got %#v", gb, warns)
		}
		if warns := CollectGhidraMemoryWarnings(SystemMemory{TotalPhysBytes: gb * giB}, heavy64, Options{FullAnalysis: true}); len(warns) != 1 || !strings.Contains(warns[0], "--full-analysis") {
			t.Fatalf("expected --full-analysis warning on %d GiB host, got %#v", gb, warns)
		}
	}

	// < 7 GiB host warns on 32-bit target; >= 8 GiB does not
	if warns := CollectGhidraMemoryWarnings(SystemMemory{TotalPhysBytes: 6 * giB}, pe32, Options{}); len(warns) != 1 || !strings.Contains(warns[0], "-Xmx5G") {
		t.Fatalf("expected 32-bit low-RAM warning on 6 GiB host, got %#v", warns)
	}
	if warns := CollectGhidraMemoryWarnings(SystemMemory{TotalPhysBytes: 8 * giB}, pe32, Options{}); len(warns) != 0 {
		t.Fatalf("expected 0 warnings on 8 GiB 32-bit host, got %#v", warns)
	}

	// Explicit --max-mem suppresses automatic memory warnings
	if warns := CollectGhidraMemoryWarnings(SystemMemory{TotalPhysBytes: 8 * giB}, heavy64, Options{MaxMem: "10G"}); len(warns) != 0 {
		t.Fatalf("expected --max-mem to suppress warnings, got %#v", warns)
	}

	// Verify /proc/meminfo parser extracts physical and commit metrics cleanly
	procSample := "MemTotal:       16384000 kB\nMemFree:         2048000 kB\nMemAvailable:   12288000 kB\nCommitLimit:    24576000 kB\nCommitted_AS:    8192000 kB\n"
	parsed := parseProcMeminfo(procSample)
	if parsed.TotalPhysBytes != 16384000*1024 || parsed.AvailPhysBytes != 12288000*1024 || parsed.TotalCommitBytes != 24576000*1024 || parsed.AvailCommitBytes != 16384000*1024 {
		t.Fatalf("unexpected parseProcMeminfo result: %+v", parsed)
	}
}

func TestGhidraTuneScriptAndJavaOptions_Configures19AnalyzersAndG1GC(t *testing.T) {
	required := []string{
		"DWARF.Import Data Types",
		"DWARF.Import Functions",
		"DWARF.Create Function Signatures",
	}
	for _, name := range required {
		if !strings.Contains(tuneReferenceAnalysisJava, `"`+name+`"`) {
			t.Fatalf("TuneReferenceAnalysis.java missing required analyzer %q", name)
		}
	}
	disabled := []string{
		"Decompiler Switch Analysis",
		"Decompiler Parameter ID",
		"Call Convention ID",
		"Non-Returning Functions - Discovered",
		"Stack",
		"Shared Return Calls",
		"Aggressive Instruction Finder",
		"Create Address Tables",
		"Create Address Tables - One Time",
		"GCC Exception Handlers",
		"Variadic Function Signature Override",
		"x86 Constant Reference Analyzer",
		"DWARF.Import Local Variable Info",
		"DWARF.Import Source Line Info",
		"DWARF.Output Source Info",
		"DWARF.Output DWARF DIE Info",
		"DWARF.Output Storage Info",
		"DWARF.Add Inlined Functions Comments",
		"DWARF.Add Lexical Block Comments",
	}
	for _, name := range disabled {
		if !strings.Contains(tuneReferenceAnalysisJava, `"`+name+`"`) {
			t.Fatalf("TuneReferenceAnalysis.java missing disabled analyzer %q", name)
		}
	}

	optsNoSpace := buildGhidraJavaOptions("/tmp/twdll_ghidra_attila_64_123", 12)
	if !strings.Contains(optsNoSpace, "-XX:+UseG1GC -XX:+UseStringDeduplication -XX:ParallelGCThreads=4") ||
		!strings.Contains(optsNoSpace, "-Djava.io.tmpdir=") ||
		!strings.Contains(optsNoSpace, "-Duser.home=") {
		t.Fatalf("unexpected Java options for space-free dir: %q", optsNoSpace)
	}

	optsWithSpace := buildGhidraJavaOptions(`C:\Users\Mod Author\Temp\twdll_ghidra`, 2)
	if optsWithSpace != "-XX:+UseG1GC -XX:+UseStringDeduplication -XX:ParallelGCThreads=2" {
		t.Fatalf("expected space-containing stageDir to omit unquoted -D path properties, got %q", optsWithSpace)
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

	// When building IDA CLI arguments for 32-bit PE without local PDB
	scriptPath := `C:\Users\Mod Author\AppData\Local\Temp\twdll ida\analyze_batch.idc`
	dbPath := `C:\Users\Mod Author\AppData\Local\Temp\twdll ida\attila_32.i64`
	logPath := `G:\Projects\TW Attila\.twdll\logs\analyze_ida_attila_32.log`
	binPath := `G:\Projects\TW Attila\.twdll\binaries\attila_32\empire.retail.dll`
	args32 := buildIDAArgs(workspace.TargetAttila32, BinaryTraits{Format: "pe", HasLocalPDB: false}, false, scriptPath, dbPath, logPath, binPath)

	// Then flags include -Opdb:off and carry zero inner quotes
	for _, arg := range args32 {
		if strings.Contains(arg, `"`) {
			t.Fatalf("IDA argument %q must not contain inner quotes", arg)
		}
	}
	if len(args32) != 7 || args32[2] != "-Opdb:off" || args32[3] != "-S"+scriptPath || args32[4] != "-o"+dbPath || args32[5] != "-L"+logPath || args32[6] != binPath {
		t.Fatalf("unexpected 32-bit IDA args without PDB: %#v", args32)
	}

	// When a local sidecar .pdb is present, -Opdb:off is omitted
	args32WithPDB := buildIDAArgs(workspace.TargetAttila32, BinaryTraits{Format: "pe", HasLocalPDB: true}, false, scriptPath, dbPath, logPath, binPath)
	for _, arg := range args32WithPDB {
		if arg == "-Opdb:off" {
			t.Fatalf("expected -Opdb:off omitted when HasLocalPDB=true: %#v", args32WithPDB)
		}
	}

	// When building IDA CLI arguments for attila_64 default vs --full-analysis
	args64Default := buildIDAArgs(workspace.TargetAttila64, BinaryTraits{Format: "elf", HasDWARF: true, IsHeavyDWARF: true}, false, scriptPath, dbPath, logPath, binPath)
	if len(args64Default) != 7 || args64Default[2] != "-Odwarf:import_lnnums=0" {
		t.Fatalf("expected -Odwarf:import_lnnums=0 for default attila_64, got %#v", args64Default)
	}
	for _, arg := range args64Default {
		if strings.Contains(arg, "lazy_types") || strings.Contains(arg, "ANALYSIS2") {
			t.Fatalf("must not pass destructive lazy_types or ANALYSIS2 flags: %#v", args64Default)
		}
	}
	args64Full := buildIDAArgs(workspace.TargetAttila64, BinaryTraits{Format: "elf", HasDWARF: true, IsHeavyDWARF: true}, true, scriptPath, dbPath, logPath, binPath)
	if len(args64Full) != 6 {
		t.Fatalf("expected no DWARF line suppression under --full-analysis, got %#v", args64Full)
	}
}

func TestResolveWorkerCounts_RespectsAvailableMemoryAndExplicitJobs(t *testing.T) {
	// Given 8-64 GB total and available physical RAM tiers plus explicit job overrides
	if got := resolveGhidra32WorkerCount(3, 2, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 4 * giB}); got != 2 {
		t.Fatalf("explicit jobs should win for Ghidra, got %d", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{}); got != 2 {
		t.Fatalf("zero memory fallback for Ghidra should be 2, got %d", got)
	}
	for _, gb := range []uint64{8, 12, 16} {
		if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{TotalPhysBytes: gb * giB}); got != 1 {
			t.Fatalf("%dGB total Ghidra workers = %d, want 1", gb, got)
		}
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{TotalPhysBytes: 24 * giB, AvailPhysBytes: 20 * giB}); got != 2 {
		t.Fatalf("24GB total / 20GB avail Ghidra workers = %d, want 2", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 26 * giB}); got != 3 {
		t.Fatalf("32GB total / 26GB avail Ghidra workers = %d, want 3", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{TotalPhysBytes: 64 * giB, AvailPhysBytes: 48 * giB}); got != 3 {
		t.Fatalf("64GB total / 48GB avail Ghidra workers = %d, want 3", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 18 * giB}); got != 2 {
		t.Fatalf("32GB total / 18GB avail Ghidra workers = %d, want 2", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 10 * giB}); got != 1 {
		t.Fatalf("32GB total / 10GB avail Ghidra workers = %d, want 1", got)
	}
	if got := resolveGhidra32WorkerCount(3, 0, SystemMemory{AvailPhysBytes: 26 * giB}); got != 3 {
		t.Fatalf("avail-only 26GB Ghidra workers = %d, want 3", got)
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

	// And ComputeGhidraMaxMem caps 32-bit Xmx when AvailPhysBytes is constrained, while attila_64 stays immune
	if got := ComputeGhidraMaxMem(SystemMemory{TotalPhysBytes: 32 * giB, AvailPhysBytes: 12 * giB}, workspace.TargetAttila32, false, 2); got != "5G" {
		t.Fatalf("ComputeGhidraMaxMem(32GB total, 12GB avail, attila_32, 2) = %q, want 5G", got)
	}
	if got := ComputeGhidraMaxMem(SystemMemory{TotalPhysBytes: 64 * giB, AvailPhysBytes: 14 * giB}, workspace.TargetAttila64, false, 1); got != "16G" {
		t.Fatalf("ComputeGhidraMaxMem(64GB total, 14GB avail, attila_64, 1) = %q, want 16G", got)
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

func TestClearPlannedTargets_RemovesTargetDBAndLogWhilePreservingSiblingsAndResourceLog(t *testing.T) {
	root := t.TempDir()
	layout := newTestLayout(t, root, workspace.DecompilerGhidra)
	paths := layout.Paths

	for _, tid := range []workspace.TargetID{workspace.TargetAttila32, workspace.TargetAttila64} {
		for _, eng := range []workspace.DecompilerID{workspace.DecompilerGhidra, workspace.DecompilerIDA} {
			dbDir := paths.DatabaseDir(eng, tid)
			if err := os.MkdirAll(dbDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dbDir, "marker"), []byte("db"), 0o644); err != nil {
				t.Fatal(err)
			}
			logPath := paths.AnalyzeLogPath(eng, tid)
			if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(logPath, []byte("log"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	resLog := paths.AnalyzeResourceLogPath()
	if err := os.WriteFile(resLog, []byte("{\"status\":\"ok\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	state := workspace.NewState()
	state.UpsertBinary(workspace.TargetAttila32, "/src/attila32", "sha32")
	state.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerGhidra, "sha32")
	state.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerIDA, "sha32")
	state.UpsertBinary(workspace.TargetAttila64, "/src/attila64", "sha64")
	state.SetAnalyzed(workspace.TargetAttila64, workspace.DecompilerGhidra, "sha64")
	state.SetAnalyzed(workspace.TargetAttila64, workspace.DecompilerIDA, "sha64")
	if err := workspace.SaveState(paths.StateFile, state); err != nil {
		t.Fatal(err)
	}

	ui := term.NewUI(term.Streams{ColorMode: term.ColorNever})
	err := clearPlannedTargets(paths, &state, workspace.DecompilerGhidra, []PlanItem{
		{
			Target:      workspace.TargetAttila32,
			DatabaseDir: paths.DatabaseDir(workspace.DecompilerGhidra, workspace.TargetAttila32),
			LogPath:     paths.AnalyzeLogPath(workspace.DecompilerGhidra, workspace.TargetAttila32),
		},
	}, ui)
	if err != nil {
		t.Fatalf("clearPlannedTargets: %v", err)
	}

	if _, err := os.Stat(paths.DatabaseDir(workspace.DecompilerGhidra, workspace.TargetAttila32)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ghidra/attila_32 DB dir removed, stat err = %v", err)
	}
	if _, err := os.Stat(paths.AnalyzeLogPath(workspace.DecompilerGhidra, workspace.TargetAttila32)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ghidra/attila_32 log removed, stat err = %v", err)
	}

	// Sibling engine (ida/attila_32), sibling target (ghidra/attila_64), and analyze_resources.jsonl must remain intact
	for _, preserved := range []string{
		paths.DatabaseDir(workspace.DecompilerIDA, workspace.TargetAttila32),
		paths.AnalyzeLogPath(workspace.DecompilerIDA, workspace.TargetAttila32),
		paths.DatabaseDir(workspace.DecompilerGhidra, workspace.TargetAttila64),
		paths.AnalyzeLogPath(workspace.DecompilerGhidra, workspace.TargetAttila64),
		resLog,
	} {
		if _, err := os.Stat(preserved); err != nil {
			t.Fatalf("expected %s to remain intact, stat err = %v", preserved, err)
		}
	}

	reloaded, _, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Binaries[workspace.TargetAttila32].Analyzed.Ghidra != "" {
		t.Fatalf("expected ghidra/attila_32 Analyzed cleared, got %q", reloaded.Binaries[workspace.TargetAttila32].Analyzed.Ghidra)
	}
	if reloaded.Binaries[workspace.TargetAttila32].Analyzed.IDA != "sha32" {
		t.Fatalf("expected ida/attila_32 Analyzed preserved, got %q", reloaded.Binaries[workspace.TargetAttila32].Analyzed.IDA)
	}
	if reloaded.Binaries[workspace.TargetAttila64].Analyzed.Ghidra != "sha64" {
		t.Fatalf("expected ghidra/attila_64 Analyzed preserved, got %q", reloaded.Binaries[workspace.TargetAttila64].Analyzed.Ghidra)
	}
}

func TestCleanWorkspace_RemovesAnalyzeArtifactsAndPreservesInitArtifacts(t *testing.T) {
	root := t.TempDir()
	layout := newTestLayout(t, root, workspace.DecompilerGhidra)
	paths := layout.Paths

	sha := stageFakeBinary(t, paths, workspace.TargetAttila32, []byte("pe32"))
	dbMarker := paths.DatabaseMarkerPath(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(dbMarker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbMarker, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := paths.AnalyzeLogPath(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	resLog := paths.AnalyzeResourceLogPath()
	if err := os.WriteFile(resLog, []byte("telemetry"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := workspace.NewState()
	st.UpsertBinary(workspace.TargetAttila32, "/src/attila32", sha)
	st.SetAnalyzed(workspace.TargetAttila32, workspace.DecompilerIDA, sha)
	if err := workspace.SaveState(paths.StateFile, st); err != nil {
		t.Fatal(err)
	}

	removed, err := CleanWorkspace(paths, term.NewUI(term.Streams{ColorMode: term.ColorNever}))
	if err != nil || !removed {
		t.Fatalf("CleanWorkspace = (%v, %v), want (true, nil)", removed, err)
	}

	for _, gone := range []string{paths.DBDir, logPath, resLog} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s deleted, stat err = %v", gone, err)
		}
	}
	for _, kept := range []string{paths.Config, paths.BinaryPath(workspace.TargetAttila32), paths.StateFile} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("expected %s preserved: %v", kept, err)
		}
	}
	reloaded, _, err := workspace.LoadState(paths.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Binaries[workspace.TargetAttila32].Analyzed.HasAny() {
		t.Fatalf("expected Analyzed cleared, got %+v", reloaded.Binaries[workspace.TargetAttila32].Analyzed)
	}
	if reloaded.Binaries[workspace.TargetAttila32].BinarySHA256 != sha {
		t.Fatalf("expected BinarySHA256 preserved, got %q", reloaded.Binaries[workspace.TargetAttila32].BinarySHA256)
	}
}

func TestCleanWorkspace_ToleratesCorruptStateJSON(t *testing.T) {
	root := t.TempDir()
	layout := newTestLayout(t, root, workspace.DecompilerGhidra)
	paths := layout.Paths

	dbMarker := paths.DatabaseMarkerPath(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(dbMarker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbMarker, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := paths.AnalyzeLogPath(workspace.DecompilerIDA, workspace.TargetAttila32)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.StateFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.StateFile, []byte("{corrupt json"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Err: &stderr, ColorMode: term.ColorNever})

	removed, err := CleanWorkspace(paths, ui)
	if err != nil {
		t.Fatalf("CleanWorkspace returned error on corrupt state.json: %v", err)
	}
	if !removed {
		t.Fatalf("expected CleanWorkspace removed = true")
	}
	for _, gone := range []string{paths.DBDir, logPath} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s deleted, stat err = %v", gone, err)
		}
	}
	if !strings.Contains(stderr.String(), "Ignoring unreadable state file") {
		t.Fatalf("expected warning about unreadable state file, got stderr:\n%s", stderr.String())
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
