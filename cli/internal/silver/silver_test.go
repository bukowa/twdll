package silver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

func init() {
	switch os.Getenv("TWDLL_TEST_FAKE_IDA") {
	case "exit0_silent":
		os.Exit(0)
	case "hang":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "hang_after_begin":
		if out := os.Getenv("TWDLL_DECOMP_OUT"); out != "" {
			_ = os.WriteFile(out, []byte(`{"event":"begin","ea":"0x1000","size":32,"name":"fn_1"}`+"\n"), 0o644)
		}
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
}

func TestBuildWorkerShards_BalancedBySizeAsc(t *testing.T) {
	specs := []extractdb.FuncSpec{
		{EA: 0x100, Size: 10, Name: "tiny_a"},
		{EA: 0x200, Size: 50000, Name: "giant_a"},
		{EA: 0x300, Size: 20, Name: "tiny_b"},
		{EA: 0x400, Size: 40000, Name: "giant_b"},
		{EA: 0x500, Size: 1000, Name: "med_a"},
		{EA: 0x600, Size: 900, Name: "med_b"},
	}

	shards := BuildWorkerShards(specs, 2)
	if len(shards) != 2 {
		t.Fatalf("len(shards) = %d, want 2", len(shards))
	}
	if shards[0][0].EA != 0x100 || shards[1][0].EA != 0x300 {
		t.Fatalf("expected smallest functions distributed across shards[0] and shards[1], got %+v / %+v", shards[0][0], shards[1][0])
	}
	if shards[0][2].EA != 0x400 || shards[1][2].EA != 0x200 {
		t.Fatalf("expected largest functions placed at end of shards[0] and shards[1], got %+v / %+v", shards[0][2], shards[1][2])
	}
	if len(shards[0]) != 3 || len(shards[1]) != 3 {
		t.Fatalf("expected balanced 3/3 shards, got %d/%d", len(shards[0]), len(shards[1]))
	}
}

func TestNormalizeLineEndingsAndStorage(t *testing.T) {
	gotC := NormalizeLineEndings("line1\r\nline2\rline3\n")
	if gotC != "line1\nline2\nline3\n" {
		t.Fatalf("NormalizeLineEndings = %q", gotC)
	}

	cases := map[string]string{
		"ECX:4":          "reg:ecx",
		"RDI:8":          "reg:rdi",
		"reg:EBX":        "reg:ebx",
		"Stack[-0x14]:4": "stack:-20",
		"Stack[0x8]:4":   "stack:+8",
		"stack:-16":      "stack:-16",
		"stack:12":       "stack:+12",
		"":               "",
	}
	for in, want := range cases {
		if got := NormalizeStorage(in); got != want {
			t.Errorf("NormalizeStorage(%q) = %q, want %q", in, got, want)
		}
	}

	frame := decompFrameJSON{
		Event:     "result",
		EA:        flexUint64(0x10001000),
		Size:      flexUint64(32),
		Name:      "FACTION::GetGold",
		Status:    extractdb.DecompStatusOK,
		ElapsedMs: 12,
		RawC:      "int __thiscall FACTION::GetGold(void *this)\r\n{\r\n  return *(int *)((int)this + 0x10);\r\n}\r\n",
		LineEA: []json.RawMessage{
			json.RawMessage(`[3, "0x10001000", "0x10001004"]`),
		},
		Vars: []extractdb.DecompVar{
			{Name: "this", Type: "void *", IsArg: true, Size: 4, Storage: "ECX:4"},
		},
	}
	rec := frame.toDecompRecord()
	if strings.Contains(rec.RawC, "\r") {
		t.Fatalf("expected LF-only RawC, got %q", rec.RawC)
	}
	if rec.LineEA != `[[3,"0x10001000","0x10001004"]]` {
		t.Fatalf("unexpected LineEA: %s", rec.LineEA)
	}
	if !strings.Contains(rec.VarsJSON, `"storage":"reg:ecx"`) {
		t.Fatalf("expected normalized storage in VarsJSON: %s", rec.VarsJSON)
	}
}

func TestStageGhidraDir_ReadOnlyVsWritableCopySafety(t *testing.T) {
	tmpRoot := t.TempDir()
	paths := workspace.NewPaths(tmpRoot, "", filepath.Join(tmpRoot, ".twdll"))
	target := workspace.TargetAttila32

	srcDir := filepath.Join(paths.DBDir, "ghidra", string(target))
	srcRep := filepath.Join(srcDir, string(target)+".rep")
	srcDBSub := filepath.Join(srcRep, "idata", "00", "~00000000.db")
	for _, d := range []string{
		srcDBSub,
		filepath.Join(srcRep, "user"),
		filepath.Join(srcRep, "versioned"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(srcDir, string(target)+".gpr"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	origPRP := `<?xml version="1.0" encoding="UTF-8"?><FILE_INFO><BASIC_INFO><STATE NAME="OWNER" TYPE="string" VALUE="original_owner" /></BASIC_INFO></FILE_INFO>`
	if err := os.WriteFile(filepath.Join(srcRep, "project.prp"), []byte(origPRP), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcRep, "idata", "~index.dat"), []byte("idx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcRep, "idata", "00", "00000000.prp"), []byte("<FILE_INFO/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	gbfPayload := []byte("GHIDRA_GBF_TEST_PAYLOAD")
	srcGBF := filepath.Join(srcDBSub, "db.1.gbf")
	if err := os.WriteFile(srcGBF, gbfPayload, 0o644); err != nil {
		t.Fatal(err)
	}

	if !hasDotPathSegment(filepath.Join(tmpRoot, ".twdll", "extracted", "tmp")) {
		t.Fatalf("expected hasDotPathSegment to detect .twdll")
	}
	if hasDotPathSegment(filepath.Join(tmpRoot, "stage_tmp")) {
		t.Fatalf("unexpected hasDotPathSegment true on clean path")
	}

	// 1. Read-only stage preserves original project.prp.
	roStage, roCleanup, err := stageGhidraReadOnly(paths, target, filepath.Join(tmpRoot, "tmp"))
	if err != nil {
		t.Fatalf("stageGhidraReadOnly: %v", err)
	}
	stagedPRP, err := os.ReadFile(filepath.Join(roStage, string(target)+".rep", "project.prp"))
	if err != nil {
		t.Fatal(err)
	}
	wantOwner := `VALUE="` + currentOSUsername() + `"`
	if !strings.Contains(string(stagedPRP), wantOwner) {
		t.Fatalf("staged project.prp = %s, want %s", string(stagedPRP), wantOwner)
	}
	roCleanup()
	if _, err := os.Stat(roStage); !os.IsNotExist(err) {
		t.Fatalf("expected stageDir removed on cleanup, stat err=%v", err)
	}

	// 2. Writable stage (for Enrichers) must copy db.1.gbf instead of hardlinking so .twdll/db/ cannot be mutated.
	rwStage, rwCleanup, err := stageGhidraDir(paths, target, filepath.Join(tmpRoot, "tmp"), true)
	if err != nil {
		t.Fatalf("stageGhidraDir(writable=true): %v", err)
	}
	defer rwCleanup()

	rwGBF := filepath.Join(rwStage, string(target)+".rep", "idata", "00", "~00000000.db", "db.1.gbf")
	if err := os.WriteFile(rwGBF, []byte("ENRICHED_MODIFIED_COPY"), 0o644); err != nil {
		t.Fatal(err)
	}
	origAfter, err := os.ReadFile(srcGBF)
	if err != nil {
		t.Fatal(err)
	}
	if string(origAfter) != string(gbfPayload) {
		t.Fatalf("writable stage shared inode with master .twdll/db payload!")
	}
}

func TestStageEnrichers_GhidraAndIDA(t *testing.T) {
	dir := t.TempDir()
	envMap := make(map[string]string)
	preArgs, err := stageGhidraEnrichers(dir, envMap, []EnrichScript{{
		Filename: "SeedTypes.java",
		Body:     "// ghidra enrich",
		Env:      map[string]string{"SEED_HINT": "1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preArgs) != 2 || preArgs[0] != "-preScript" || preArgs[1] != "SeedTypes.java" || envMap["SEED_HINT"] != "1" {
		t.Fatalf("unexpected ghidra enricher staging: preArgs=%v env=%v", preArgs, envMap)
	}

	// Reject reserved exporter filenames (case-insensitively) and duplicate basenames.
	for _, bad := range [][]EnrichScript{
		{{Filename: "TwdllSilverExport.java", Body: "// clash"}},
		{{Filename: "sub/twdlldecompexport.java", Body: "// clash"}},
		{
			{Filename: "a/SeedTypes.java", Body: "// 1"},
			{Filename: "b/seedtypes.java", Body: "// 2"},
		},
	} {
		if _, err := stageGhidraEnrichers(dir, make(map[string]string), bad); err == nil {
			t.Fatalf("expected stageGhidraEnrichers to reject %+v", bad)
		}
	}

	idaEnv := make(map[string]string)
	entry, err := stageIDAScriptsAndEnv(dir, "export_ida_silver.py", "# export", []EnrichScript{{
		Filename: "seed_ida.py",
		Body:     "# enrich",
		Env:      map[string]string{"IDA_SEED": "ok"},
	}}, idaEnv)
	if err != nil {
		t.Fatal(err)
	}
	if entry != "twdll_ida_run.py" || idaEnv["IDA_SEED"] != "ok" {
		t.Fatalf("unexpected ida wrapper entry=%q env=%v", entry, idaEnv)
	}
	wrapperBody, err := os.ReadFile(filepath.Join(dir, entry))
	if err != nil {
		t.Fatal(err)
	}
	wrapper := string(wrapperBody)
	for _, want := range []string{
		"os.path.abspath(__file__)",
		"enrich_0_seed_ida.py",
		"export_ida_silver.py",
		"except SystemExit:",
		"traceback.print_exc()",
		"idc.qexit(1)",
	} {
		if !strings.Contains(wrapper, want) {
			t.Fatalf("wrapper script missing %q:\n%s", want, wrapper)
		}
	}
}

func TestDriveIDAWorkerShardWithWatchdog_RecoversFromTimeoutAndPurgesSidecars(t *testing.T) {
	workerDir := t.TempDir()
	stagedI64 := filepath.Join(workerDir, "attila_32.i64")
	srcI64 := filepath.Join(t.TempDir(), "attila_32.i64")
	origData := []byte("FAKE_I64_BINARY_IMAGE")
	if err := os.WriteFile(srcI64, origData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedI64, origData, 0o644); err != nil {
		t.Fatal(err)
	}

	specs := []extractdb.FuncSpec{
		{EA: 0x1000, Size: 500, Name: "fn_ok_1"},
		{EA: 0x2000, Size: 25000, Name: "fn_timeout_2"},
		{EA: 0x3000, Size: 100, Name: "fn_ok_3"},
	}

	outCh := make(chan extractdb.DecompRecord, 10)
	var attemptSeen []int
	var sidecarsCleanedBeforeRespawn bool

	cfg := idaWorkerConfig{
		workerID:   0,
		srcI64:     srcI64,
		srcI64Size: int64(len(origData)),
		target:     workspace.TargetAttila32,
		engine:     workspace.DecompilerIDA,
		shard:      specs,
		outCh:      outCh,
	}

	_, err := driveIDAWorkerShardWithWatchdog(context.Background(), workerDir, stagedI64, cfg,
		func(ctx context.Context, attempt int, remaining []extractdb.FuncSpec) idaAttemptOutcome {
			attemptSeen = append(attemptSeen, attempt)
			if attempt == 0 {
				if len(remaining) != 3 {
					t.Fatalf("attempt 0 remaining = %d, want 3", len(remaining))
				}
				for _, ext := range []string{".id0", ".id1", ".id2", ".nam", ".til"} {
					_ = os.WriteFile(filepath.Join(workerDir, "attila_32"+ext), []byte("dirty"), 0o644)
				}
				outCh <- extractdb.DecompRecord{FuncEA: 0x1000, FuncSize: 500, Status: extractdb.DecompStatusOK}
				outCh <- extractdb.DecompRecord{FuncEA: 0x2000, FuncSize: 25000, Status: extractdb.DecompStatusTimeout}
				return idaAttemptOutcome{completed: 2, peakMem: 1024}
			}
			sidecarsCleanedBeforeRespawn = true
			for _, ext := range []string{".id0", ".id1", ".id2", ".nam", ".til"} {
				if _, statErr := os.Stat(filepath.Join(workerDir, "attila_32"+ext)); !os.IsNotExist(statErr) {
					sidecarsCleanedBeforeRespawn = false
				}
			}
			if len(remaining) != 1 || remaining[0].EA != 0x3000 {
				t.Fatalf("attempt 1 remaining = %+v, want [0x3000]", remaining)
			}
			outCh <- extractdb.DecompRecord{FuncEA: 0x3000, FuncSize: 100, Status: extractdb.DecompStatusOK}
			return idaAttemptOutcome{completed: 1, peakMem: 512}
		})
	close(outCh)

	if err != nil {
		t.Fatalf("driveIDAWorkerShardWithWatchdog failed: %v", err)
	}
	if len(attemptSeen) != 2 {
		t.Fatalf("attempts = %v, want [0 1]", attemptSeen)
	}
	if !sidecarsCleanedBeforeRespawn {
		t.Fatalf("expected .id0/.id1/.id2/.nam/.til sidecars purged before respawn")
	}

	var recs []extractdb.DecompRecord
	for r := range outCh {
		recs = append(recs, r)
	}
	if len(recs) != 3 || recs[1].Status != extractdb.DecompStatusTimeout || recs[2].Status != extractdb.DecompStatusOK {
		t.Fatalf("unexpected records: %+v", recs)
	}
}

func TestResolveDecompTimeoutSecsAndSendDecompRecord(t *testing.T) {
	if got := resolveDecompTimeoutSecs(Options{}); got != DefaultFastTimeoutSecs {
		t.Fatalf("default timeout = %d, want %d", got, DefaultFastTimeoutSecs)
	}
	if got := resolveDecompTimeoutSecs(Options{TimeoutSecs: 30}); got != 30 {
		t.Fatalf("explicit timeout = %d, want 30", got)
	}
	if got := resolveDecompTimeoutSecs(Options{NoTimeout: true}); got != 0 {
		t.Fatalf("NoTimeout = %d, want 0", got)
	}
	if got := resolveDecompTimeoutSecs(Options{TimeoutSecs: 30, NoTimeout: true}); got != 0 {
		t.Fatalf("NoTimeout override = %d, want 0", got)
	}
	if got := resolveDecompTimeoutSecs(Options{TimeoutSecs: -1}); got != 0 {
		t.Fatalf("negative TimeoutSecs = %d, want 0", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	unbuffered := make(chan extractdb.DecompRecord)
	if ok := sendDecompRecord(ctx, unbuffered, extractdb.DecompRecord{FuncEA: 0x1000}); ok {
		t.Fatalf("expected sendDecompRecord to return false on cancelled context")
	}
}

type fakeEnricher struct {
	id   string
	file string
}

func (f fakeEnricher) ID() string { return f.id }
func (f fakeEnricher) AppliesTo(_ workspace.TargetID, _ workspace.DecompilerID) bool {
	return true
}
func (f fakeEnricher) Script(_ workspace.TargetID, _ workspace.DecompilerID) (EnrichScript, error) {
	return EnrichScript{Filename: f.file, Body: "// seed"}, nil
}

func TestExtractWithHooks_PluggableEnricherSelectorAndAspects(t *testing.T) {
	ctx := context.Background()
	silverPath := filepath.Join(t.TempDir(), "silver_ida.sqlite")

	var decompSawEAs []uint64
	var enricherSeen string

	res, err := extractWithHooks(ctx, Options{
		Target:       workspace.TargetAttila32,
		Engine:       workspace.DecompilerIDA,
		BinarySHA256: "sha_silver_test",
		SilverDBPath: silverPath,
		Enrichers:    []Enricher{fakeEnricher{id: "test_seed", file: "seed.py"}},
		Selector:     EASelector(0x2000),
	},
		func(_ context.Context, _ Options, enrichers []EnrichScript, asmOutPath, stringsOutPath string) ([]SilverFunc, error) {
			if len(enrichers) == 1 {
				enricherSeen = enrichers[0].Filename
			}
			if asmOutPath != "" {
				asmLines := `{"ea":"0x1000","insn_count":1,"raw_asm":"1000: ret"}` + "\n" +
					`{"ea":"0x2000","insn_count":2,"raw_asm":"2000: push ecx\r\n2001: ret"}` + "\n"
				if wErr := os.WriteFile(asmOutPath, []byte(asmLines), 0o644); wErr != nil {
					return nil, wErr
				}
			}
			if stringsOutPath != "" {
				strLines := `{"ea":"0x3000","encoding":"ascii","length":2,"value":"ok","ref_count":3}` + "\n" +
					`{"ea":"0x3004","encoding":"utf16le","length":7,"value":"CONOUT$","ref_count":1}` + "\n"
				if wErr := os.WriteFile(stringsOutPath, []byte(strLines), 0o644); wErr != nil {
					return nil, wErr
				}
			}
			return []SilverFunc{
				{EA: 0x1000, Size: 16, Name: "fn_one", CallingConv: "cdecl"},
				{EA: 0x2000, Size: 32, Name: "fn_two", CallingConv: "thiscall", StackPurge: 4, Ranges: []SilverRange{{StartEA: 0x2000, EndEA: 0x2020}}},
			}, nil
		},
		func(_ context.Context, _ Options, _ []EnrichScript, pending []extractdb.FuncSpec, outCh chan<- extractdb.DecompRecord) (uint64, error) {
			for _, sp := range pending {
				decompSawEAs = append(decompSawEAs, sp.EA)
				outCh <- extractdb.DecompRecord{
					FuncEA:   sp.EA,
					FuncSize: sp.Size,
					FuncName: sp.Name,
					RawC:     "int __thiscall fn_two(void *this) { return 1; }",
					Status:   extractdb.DecompStatusOK,
				}
			}
			return 0, nil
		},
	)
	if err != nil {
		t.Fatalf("extractWithHooks: %v", err)
	}

	if enricherSeen != "seed.py" {
		t.Fatalf("enricherSeen = %q, want seed.py", enricherSeen)
	}
	if len(decompSawEAs) != 1 || decompSawEAs[0] != 0x2000 {
		t.Fatalf("decompSawEAs = %v, want [0x2000] via EASelector", decompSawEAs)
	}
	if !res.FuncsRan || !res.AsmRan || !res.StringsRan || !res.DecompRan || res.FuncCount != 2 || res.AsmCount != 2 || res.StringCount != 2 || res.DecompWritten != 1 {
		t.Fatalf("unexpected silver Result: %+v", res)
	}

	roDB, err := extractdb.OpenReadOnly(silverPath)
	if err != nil {
		t.Fatal(err)
	}
	defer roDB.Close()

	var insnCnt int
	var rawAsm string
	if err := roDB.QueryRowContext(ctx, `SELECT insn_count, raw_asm FROM silver_asm WHERE func_ea = 0x2000`).Scan(&insnCnt, &rawAsm); err != nil {
		t.Fatal(err)
	}
	if insnCnt != 2 || rawAsm != "2000: push ecx\n2001: ret" {
		t.Fatalf("silver_asm(0x2000) = (%d, %q), want (2, LF-normalized)", insnCnt, rawAsm)
	}

	var normCC string
	if err := roDB.QueryRowContext(ctx, `SELECT calling_conv FROM silver_funcs WHERE ea = 0x2000`).Scan(&normCC); err != nil {
		t.Fatal(err)
	}
	if normCC != "__thiscall" {
		t.Fatalf("calling_conv = %q, want __thiscall", normCC)
	}
	var strVal, strEnc string
	var strRef int
	if err := roDB.QueryRowContext(ctx, `SELECT encoding, value, ref_count FROM silver_strings WHERE ea = 0x3004`).Scan(&strEnc, &strVal, &strRef); err != nil {
		t.Fatal(err)
	}
	if strEnc != "utf16le" || strVal != "CONOUT$" || strRef != 1 {
		t.Fatalf("silver_strings(0x3004) = (%q, %q, %d), want (utf16le, CONOUT$, 1)", strEnc, strVal, strRef)
	}
	var ftsStrHits int
	if err := roDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM silver_strings_fts WHERE silver_strings_fts MATCH 'CONOUT'`).Scan(&ftsStrHits); err != nil {
		t.Fatal(err)
	}
	if ftsStrHits != 1 {
		t.Fatalf("silver_strings_fts MATCH CONOUT = %d, want 1", ftsStrHits)
	}
	var aspectsApplied string
	if err := roDB.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, extractdb.MetaAspectsApplied).Scan(&aspectsApplied); err != nil {
		t.Fatal(err)
	}
	if aspectsApplied != "funcs,decomp" {
		t.Fatalf("aspects_applied = %q, want funcs,decomp", aspectsApplied)
	}

	// Follow-up run without Selector must see the complete silver_funcs and decompile remaining 0x1000.
	var secondSawEAs []uint64
	res2, err := extractWithHooks(ctx, Options{
		Target:       workspace.TargetAttila32,
		Engine:       workspace.DecompilerIDA,
		BinarySHA256: "sha_silver_test",
		SilverDBPath: silverPath,
		Enrichers:    []Enricher{fakeEnricher{id: "test_seed", file: "seed.py"}},
	},
		func(_ context.Context, _ Options, _ []EnrichScript, _, _ string) ([]SilverFunc, error) {
			t.Fatalf("expected funcsAspect to skip re-export when silver_funcs, silver_asm, and silver_strings already exist")
			return nil, nil
		},
		func(_ context.Context, _ Options, _ []EnrichScript, pending []extractdb.FuncSpec, outCh chan<- extractdb.DecompRecord) (uint64, error) {
			for _, sp := range pending {
				secondSawEAs = append(secondSawEAs, sp.EA)
				outCh <- extractdb.DecompRecord{
					FuncEA:   sp.EA,
					FuncSize: sp.Size,
					FuncName: sp.Name,
					RawC:     "int fn_one(void) { return 0; }",
					Status:   extractdb.DecompStatusOK,
				}
			}
			return 0, nil
		},
	)
	if err != nil {
		t.Fatalf("second extractWithHooks: %v", err)
	}
	if res2.FuncsRan || !res2.DecompRan || len(secondSawEAs) != 1 || secondSawEAs[0] != 0x1000 {
		t.Fatalf("unexpected second run: res=%+v secondSawEAs=%v", res2, secondSawEAs)
	}
}

func TestDecompAspect_CancelsExportOnBatchWriterError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	silverPath := filepath.Join(t.TempDir(), "silver_ida.sqlite")
	store, err := extractdb.OpenSilverStore(ctx, silverPath, "attila_32", "ida", "sha_fail_writer", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		CREATE TRIGGER fail_decomp_insert BEFORE INSERT ON decomp_funcs
		BEGIN
			SELECT RAISE(ABORT, 'simulated batch write error');
		END;
	`); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	const totalFuncs = 700 // exceeds outCh buffer (512) to verify cancellation unblocks producer
	funcs := make([]SilverFunc, 0, totalFuncs)
	for i := 0; i < totalFuncs; i++ {
		ea := uint64(0x10000 + i*0x20)
		funcs = append(funcs, SilverFunc{EA: ea, Size: 16, Name: fmt.Sprintf("fn_%d", i)})
	}

	_, err = extractWithHooks(ctx, Options{
		Target:       workspace.TargetAttila32,
		Engine:       workspace.DecompilerIDA,
		BinarySHA256: "sha_fail_writer",
		SilverDBPath: silverPath,
		BatchSize:    10,
	},
		func(_ context.Context, _ Options, _ []EnrichScript, _, _ string) ([]SilverFunc, error) {
			return funcs, nil
		},
		func(dCtx context.Context, _ Options, _ []EnrichScript, pending []extractdb.FuncSpec, outCh chan<- extractdb.DecompRecord) (uint64, error) {
			for _, sp := range pending {
				rec := extractdb.DecompRecord{
					FuncEA:   sp.EA,
					FuncSize: sp.Size,
					FuncName: sp.Name,
					RawC:     "void f(void) {}",
					Status:   extractdb.DecompStatusOK,
				}
				if !sendDecompRecord(dCtx, outCh, rec) {
					return 0, dCtx.Err()
				}
			}
			return 0, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "simulated batch write error") {
		t.Fatalf("expected simulated batch write error without deadlock, got: %v", err)
	}
}

func TestDriveIDAWorkerShardWithWatchdog_ZeroProgressFails(t *testing.T) {
	workerDir := t.TempDir()
	stagedI64 := filepath.Join(workerDir, "attila_32.i64")
	srcI64 := filepath.Join(t.TempDir(), "attila_32.i64")
	origData := []byte("FAKE_I64_BINARY_IMAGE")
	if err := os.WriteFile(srcI64, origData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedI64, origData, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := idaWorkerConfig{
		workerID:   0,
		srcI64:     srcI64,
		srcI64Size: int64(len(origData)),
		target:     workspace.TargetAttila32,
		engine:     workspace.DecompilerIDA,
		shard:      []extractdb.FuncSpec{{EA: 0x1000, Size: 32, Name: "fn_1"}},
		outCh:      make(chan extractdb.DecompRecord, 4),
	}

	_, err := driveIDAWorkerShardWithWatchdog(context.Background(), workerDir, stagedI64, cfg,
		func(_ context.Context, _ int, _ []extractdb.FuncSpec) idaAttemptOutcome {
			return idaAttemptOutcome{completed: 0, err: nil}
		})
	if !errors.Is(err, errIDANoProgressFatal) {
		t.Fatalf("expected errIDANoProgressFatal on zero-progress attempts, got: %v", err)
	}
}

func TestExecuteIDAAttempt_ZeroProgressExit0AndStartupTimeout(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	specs := []extractdb.FuncSpec{{EA: 0x1000, Size: 32, Name: "fn_1"}}

	t.Run("exit 0 without JSONL progress fails", func(t *testing.T) {
		workerDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(workerDir, "idausr"), 0o755); err != nil {
			t.Fatal(err)
		}
		stagedI64 := filepath.Join(workerDir, "attila_32.i64")
		if err := os.WriteFile(stagedI64, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
		outCh := make(chan extractdb.DecompRecord, 4)
		outcome := executeIDAAttempt(
			context.Background(),
			workerDir,
			stagedI64,
			"export_ida_decomp.py",
			map[string]string{"TWDLL_TEST_FAKE_IDA": "exit0_silent"},
			0,
			specs,
			idaWorkerConfig{
				workerID: 0,
				idaExe:   exe,
				target:   workspace.TargetAttila32,
				shard:    specs,
				outCh:    outCh,
			},
		)
		if outcome.completed != 0 || !errors.Is(outcome.err, errIDANoProgressFatal) {
			t.Fatalf("expected completed=0 and errIDANoProgressFatal, got %+v", outcome)
		}
	})

	t.Run("startup grace timeout kills hung worker before begin frame", func(t *testing.T) {
		workerDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(workerDir, "idausr"), 0o755); err != nil {
			t.Fatal(err)
		}
		stagedI64 := filepath.Join(workerDir, "attila_32.i64")
		if err := os.WriteFile(stagedI64, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
		outCh := make(chan extractdb.DecompRecord, 4)
		start := time.Now()
		outcome := executeIDAAttempt(
			context.Background(),
			workerDir,
			stagedI64,
			"export_ida_decomp.py",
			map[string]string{"TWDLL_TEST_FAKE_IDA": "hang"},
			0,
			specs,
			idaWorkerConfig{
				workerID:     0,
				idaExe:       exe,
				target:       workspace.TargetAttila32,
				timeoutSecs:  1,
				startupGrace: 120 * time.Millisecond,
				shard:        specs,
				outCh:        outCh,
			},
		)
		if time.Since(start) > 3*time.Second {
			t.Fatalf("startup grace timeout took too long: %s", time.Since(start))
		}
		if outcome.completed != 0 || !errors.Is(outcome.err, errIDANoProgressFatal) || !strings.Contains(outcome.err.Error(), "startup timeout") {
			t.Fatalf("expected startup timeout error, got %+v", outcome)
		}
	})

	t.Run("in-flight timeout after begin frame uses perFuncDeadline without 15s startup grace", func(t *testing.T) {
		workerDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(workerDir, "idausr"), 0o755); err != nil {
			t.Fatal(err)
		}
		stagedI64 := filepath.Join(workerDir, "attila_32.i64")
		if err := os.WriteFile(stagedI64, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
		outCh := make(chan extractdb.DecompRecord, 4)
		start := time.Now()
		outcome := executeIDAAttempt(
			context.Background(),
			workerDir,
			stagedI64,
			"export_ida_decomp.py",
			map[string]string{"TWDLL_TEST_FAKE_IDA": "hang_after_begin"},
			0,
			specs,
			idaWorkerConfig{
				workerID:     0,
				idaExe:       exe,
				target:       workspace.TargetAttila32,
				timeoutSecs:  1,
				startupGrace: 15 * time.Second,
				shard:        specs,
				outCh:        outCh,
			},
		)
		if elapsed := time.Since(start); elapsed >= 5*time.Second {
			t.Fatalf("in-flight timeout after begin frame took too long: %s", elapsed)
		}
		if outcome.err != nil || outcome.completed != 1 {
			t.Fatalf("expected completed=1 and err=nil, got %+v", outcome)
		}
		select {
		case rec := <-outCh:
			if rec.Status != extractdb.DecompStatusTimeout {
				t.Fatalf("expected record status %q, got %q (rec=%+v)", extractdb.DecompStatusTimeout, rec.Status, rec)
			}
		default:
			t.Fatal("expected DecompRecord emitted on outCh, got none")
		}
	})
}

func TestStageIDAWorkerDir_WritesHexraysCfg(t *testing.T) {
	tmpRoot := t.TempDir()
	srcI64 := filepath.Join(tmpRoot, "attila_32.i64")
	if err := os.WriteFile(srcI64, []byte("fake-i64"), 0o644); err != nil {
		t.Fatal(err)
	}

	workerDir, stagedI64, cleanup, err := stageIDAWorkerDir(srcI64, workspace.TargetAttila32, tmpRoot, "test_worker")
	if err != nil {
		t.Fatalf("stageIDAWorkerDir failed: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(stagedI64); err != nil {
		t.Fatalf("staged i64 missing: %v", err)
	}
	cfgPath := filepath.Join(workerDir, "idausr", "cfg", "hexrays.cfg")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read hexrays.cfg: %v", err)
	}
	if !strings.Contains(string(raw), "MAX_FUNCSIZE = 128") || !strings.Contains(string(raw), "ARG_HINTS_MODE = 0") {
		t.Fatalf("hexrays.cfg = %q, want MAX_FUNCSIZE = 128 and ARG_HINTS_MODE = 0", string(raw))
	}
}

func TestJSONLTailer_PartialChunksWithBuilder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.jsonl")
	tailer := &jsonlTailer{path: path}

	var got []uint64
	collect := func(f decompFrameJSON) {
		got = append(got, uint64(f.EA))
	}

	if err := os.WriteFile(path, []byte(`{"event":"begin","ea":"0x1000"}`+"\n"+`{"event":"result","ea":"0x`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tailer.Drain(collect); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 0x1000 {
		t.Fatalf("after partial write: got=%v, want [0x1000]", got)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`1000","status":"ok"}` + "\n" + `{"event":"result","ea":"0x2000","status":"ok"}` + "\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	if err := tailer.Drain(collect); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1] != 0x1000 || got[2] != 0x2000 {
		t.Fatalf("after completion write: got=%v, want [0x1000 0x1000 0x2000]", got)
	}
}

func TestGhidraScripts_NativeAPIsAndInvariants(t *testing.T) {
	for _, want := range []string{
		"GsonBuilder",
		"disableHtmlEscaping()",
		"fn.getPrototypeString(false, true)",
		"CodeUnitFormatOptions",
		"TWDLL_SILVER_ASM_OUT",
		"false, // Browser default",
	} {
		if !strings.Contains(twdllSilverExportJava, want) {
			t.Fatalf("twdllSilverExportJava missing %q", want)
		}
	}
	for _, want := range []string{
		"opts.grabFromProgram(currentProgram);",
		"opts.setMaxPayloadMBytes(256);",
		"ParallelDecompiler.decompileFunctions(",
		"DecompilerCallback<Void>",
		"GThreadPool.getSharedThreadPool",
		"res.isTimedOut()",
		"PrettyPrinter",
		"GsonBuilder",
		"getDataTypeCount(true)",
		"done % 32 == 0 || done == totalJobs",
	} {
		if !strings.Contains(twdllDecompExportJava, want) {
			t.Fatalf("twdllDecompExportJava missing %q", want)
		}
	}
	for _, forbidden := range []string{"flushCache", "computeLineShift", "private static String esc("} {
		if strings.Contains(twdllDecompExportJava, forbidden) {
			t.Fatalf("twdllDecompExportJava must not contain %q", forbidden)
		}
	}
}

func TestIDAScripts_NativeAPIsAnd64BitPretypeInvariants(t *testing.T) {
	for _, want := range []string{
		"func_tail_iterator_t(fn)",
		"calc_func_size(fn)",
		"tinfo.get_cc()",
		"CM_CC_SPECIALP",
		"CM_CC_ELLIPSIS",
		"PRTYPE_1LINE",
		"GENDSM_FORCE_CODE | ida_lines.GENDSM_REMOVE_TAGS",
		"TWDLL_SILVER_ASM_OUT",
	} {
		if !strings.Contains(exportIDASilverPy, want) {
			t.Fatalf("exportIDASilverPy missing %q", want)
		}
	}
	for _, want := range []string{
		"inf_is_64bit()",
		"pretype_64bit_stalls",
		"calc_func_size(fn)",
		"_ZGV",
		"_ZZ",
		"off_",
		"_ZTV",
		"_ZTI",
		"_ZTS",
		"get_first_fcref_from",
		"_pretype_unnamed_udt_args",
		"is_udt()",
		"get_type_name()",
		"BTF_UINT64",
		"get_mreg_name",
	} {
		if !strings.Contains(exportIDADecompPy, want) {
			t.Fatalf("exportIDADecompPy missing %q", want)
		}
	}
	if strings.Contains(exportIDADecompPy, "ida_idp") {
		t.Fatalf("exportIDADecompPy must not contain dead ida_idp fallback")
	}
}

func TestWatchJSONLLineProgress_TracksAppendedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "silver.jsonl")
	totalPath := filepath.Join(dir, "silver.total")
	if err := os.WriteFile(totalPath, []byte("5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	doneCh := make(chan struct{})
	var counts [][2]int
	go func() {
		defer close(doneCh)
		watchJSONLLineProgress(context.Background(), path, totalPath, stop, func(done, total int) {
			counts = append(counts, [2]int{done, total})
		})
	}()

	if err := os.WriteFile(path, []byte(`{"ea":"0x1000"}`+"\n"+`{"ea":"0x2000"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"ea":"0x3000"}` + "\n")
	_ = f.Close()

	close(stop)
	<-doneCh
	if len(counts) == 0 || counts[len(counts)-1] != [2]int{3, 5} {
		t.Fatalf("counts = %v, want final [3 5]", counts)
	}
}

func TestExtractWithHooks_EmitsOnResumeWhenResuming(t *testing.T) {
	ctx := context.Background()
	silverPath := filepath.Join(t.TempDir(), "silver_ghidra.sqlite")

	fnExp := func(_ context.Context, _ Options, _ []EnrichScript, _, _ string) ([]SilverFunc, error) {
		return []SilverFunc{
			{EA: 0x1000, Size: 16, Name: "fn_one"},
			{EA: 0x2000, Size: 32, Name: "fn_two"},
			{EA: 0x3000, Size: 48, Name: "fn_three"},
		}, nil
	}

	// First pass: decompile only 0x1000 via Selector.
	_, err := extractWithHooks(ctx, Options{
		Target:       workspace.TargetAttila64,
		Engine:       workspace.DecompilerGhidra,
		BinarySHA256: "sha_resume_cb",
		SilverDBPath: silverPath,
		Selector:     EASelector(0x1000),
	}, fnExp, func(_ context.Context, _ Options, _ []EnrichScript, pending []extractdb.FuncSpec, outCh chan<- extractdb.DecompRecord) (uint64, error) {
		for _, sp := range pending {
			outCh <- extractdb.DecompRecord{FuncEA: sp.EA, FuncSize: sp.Size, FuncName: sp.Name, RawC: "void f(){}", Status: extractdb.DecompStatusOK}
		}
		return 0, nil
	})
	if err != nil {
		t.Fatalf("seed pass: %v", err)
	}

	// Second pass: resume full target, expect OnResume(1, 2).
	var gotCached, gotPending int
	_, err = extractWithHooks(ctx, Options{
		Target:       workspace.TargetAttila64,
		Engine:       workspace.DecompilerGhidra,
		BinarySHA256: "sha_resume_cb",
		SilverDBPath: silverPath,
		OnResume: func(cached, pending int) {
			gotCached = cached
			gotPending = pending
		},
	}, fnExp, func(_ context.Context, _ Options, _ []EnrichScript, pending []extractdb.FuncSpec, outCh chan<- extractdb.DecompRecord) (uint64, error) {
		for _, sp := range pending {
			outCh <- extractdb.DecompRecord{FuncEA: sp.EA, FuncSize: sp.Size, FuncName: sp.Name, RawC: "void f(){}", Status: extractdb.DecompStatusOK}
		}
		return 0, nil
	})
	if err != nil {
		t.Fatalf("resume pass: %v", err)
	}
	if gotCached != 1 || gotPending != 2 {
		t.Fatalf("OnResume = (%d, %d), want (1, 2)", gotCached, gotPending)
	}
}
