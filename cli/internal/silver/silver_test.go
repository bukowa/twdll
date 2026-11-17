package silver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

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
		"ECX:4":         "reg:ecx",
		"RDI:8":         "reg:rdi",
		"reg:EBX":       "reg:ebx",
		"Stack[-0x14]:4": "stack:-20",
		"Stack[0x8]:4":  "stack:+8",
		"stack:-16":     "stack:-16",
		"stack:12":      "stack:+12",
		"":              "",
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
	rec := frame.toDecompRecord(workspace.DecompilerGhidra)
	if strings.Contains(rec.RawC, "\r") {
		t.Fatalf("expected LF-only RawC, got %q", rec.RawC)
	}
	if rec.LineEAJSON != `[[3,"0x10001000","0x10001004"]]` {
		t.Fatalf("unexpected LineEAJSON: %s", rec.LineEAJSON)
	}
	if !strings.Contains(rec.DecompilerVarsJSON, `"storage":"reg:ecx"`) {
		t.Fatalf("expected normalized storage in DecompilerVarsJSON: %s", rec.DecompilerVarsJSON)
	}
}

func TestStageGhidraReadOnly_RewritesOwnerAndHardlinksPayload(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(srcDBSub, "db.1.gbf"), gbfPayload, 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify dot-prefixed temp base paths (like .twdll/extracted/.tmp) automatically fall back out of dot segments.
	if !hasDotPathSegment(filepath.Join(tmpRoot, ".twdll", "extracted", "tmp")) {
		t.Fatalf("expected hasDotPathSegment to detect .twdll")
	}
	if hasDotPathSegment(filepath.Join(tmpRoot, "stage_tmp")) {
		t.Fatalf("unexpected hasDotPathSegment true on clean path")
	}

	stageDir, cleanup, err := stageGhidraReadOnly(paths, target, filepath.Join(tmpRoot, "tmp"))
	if err != nil {
		t.Fatalf("stageGhidraReadOnly: %v", err)
	}

	stagedPRP, err := os.ReadFile(filepath.Join(stageDir, string(target)+".rep", "project.prp"))
	if err != nil {
		t.Fatal(err)
	}
	wantOwner := `VALUE="` + currentOSUsername() + `"`
	if !strings.Contains(string(stagedPRP), wantOwner) {
		t.Fatalf("staged project.prp = %s, want %s", string(stagedPRP), wantOwner)
	}

	stagedGBF := filepath.Join(stageDir, string(target)+".rep", "idata", "00", "~00000000.db", "db.1.gbf")
	gotGBF, err := os.ReadFile(stagedGBF)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotGBF) != string(gbfPayload) {
		t.Fatalf("staged db.1.gbf content mismatch")
	}

	cleanup()
	if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
		t.Fatalf("expected stageDir removed on cleanup, stat err=%v", err)
	}
	// Original source project.prp must remain untouched.
	srcPRPCheck, err := os.ReadFile(filepath.Join(srcRep, "project.prp"))
	if err != nil {
		t.Fatal(err)
	}
	if string(srcPRPCheck) != origPRP {
		t.Fatalf("source project.prp was mutated!")
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
				// Simulate IDA unpacking sidecars and killing mid-attempt after fn_ok_1 + timeout on fn_timeout_2.
				for _, ext := range []string{".id0", ".id1", ".id2", ".nam", ".til"} {
					_ = os.WriteFile(filepath.Join(workerDir, "attila_32"+ext), []byte("dirty"), 0o644)
				}
				outCh <- extractdb.DecompRecord{FuncEA: 0x1000, Engine: "ida", FuncSize: 500, Status: extractdb.DecompStatusOK}
				outCh <- extractdb.DecompRecord{FuncEA: 0x2000, Engine: "ida", FuncSize: 25000, Status: extractdb.DecompStatusTimeout}
				return idaAttemptOutcome{completed: 2, peakMem: 1024}
			}
			// Attempt 1: verify unpacked sidecars were purged before respawn.
			sidecarsCleanedBeforeRespawn = true
			for _, ext := range []string{".id0", ".id1", ".id2", ".nam", ".til"} {
				if _, statErr := os.Stat(filepath.Join(workerDir, "attila_32"+ext)); !os.IsNotExist(statErr) {
					sidecarsCleanedBeforeRespawn = false
				}
			}
			if len(remaining) != 1 || remaining[0].EA != 0x3000 {
				t.Fatalf("attempt 1 remaining = %+v, want [0x3000]", remaining)
			}
			outCh <- extractdb.DecompRecord{FuncEA: 0x3000, Engine: "ida", FuncSize: 100, Status: extractdb.DecompStatusOK}
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

func TestSelectBenchmarkCorpus_StratifiedAndDeterministic(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "binary.sqlite")
	b, err := extractdb.NewAtomicBuilder(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := extractdb.InitFactsSchema(ctx, b.DB()); err != nil {
		t.Fatal(err)
	}

	db := b.DB()
	// Insert a thunk (must be excluded), FACTION::FACTION at 0x00f53830, and functions across all size buckets.
	if _, err := db.ExecContext(ctx, `INSERT INTO functions(ea, size, name, flags) VALUES (0x100, 6, 'j_thunk', ?)`, int64(extractdb.FuncFlagThunk)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO functions(ea, size, name, flags) VALUES (0x00f53830, 25329, 'FACTION::FACTION', 0)`); err != nil {
		t.Fatal(err)
	}
	ea := uint64(0x10000)
	for i := 0; i < 60; i++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO functions(ea, size, name, flags) VALUES (?, 32, 'lua_fn', 0)`, int64(ea)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO lua_regs(table_ea, method_name, func_ea) VALUES (0x9000, ?, ?)`, string(rune('a'+i%26))+string(rune('a'+i/26)), int64(ea)); err != nil {
			t.Fatal(err)
		}
		ea += 0x40
	}
	for i := 0; i < 60; i++ {
		if _, err := db.ExecContext(ctx, `INSERT INTO functions(ea, size, name, flags) VALUES (?, 128, 'vt_fn', 0)`, int64(ea)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO vtables(vtable_ea, slot, target_ea) VALUES (0xA000, ?, ?)`, i, int64(ea)); err != nil {
			t.Fatal(err)
		}
		ea += 0x100
	}
	sizes := []uint64{24, 256, 1024, 8192, 30000}
	for _, sz := range sizes {
		for i := 0; i < 50; i++ {
			if _, err := db.ExecContext(ctx, `INSERT INTO functions(ea, size, name, flags) VALUES (?, ?, 'sub_fn', 0)`, int64(ea), int64(sz)); err != nil {
				t.Fatal(err)
			}
			ea += 0x100
		}
	}
	if err := b.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	ro, err := extractdb.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()

	pick1, err := SelectBenchmarkCorpus(ctx, ro, workspace.TargetAttila64, 200)
	if err != nil {
		t.Fatal(err)
	}
	pick2, err := SelectBenchmarkCorpus(ctx, ro, workspace.TargetAttila64, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(pick1) != 200 {
		t.Fatalf("len(pick1) = %d, want 200", len(pick1))
	}
	foundFactionCtor := false
	for i := range pick1 {
		if pick1[i] != pick2[i] {
			t.Fatalf("non-deterministic pick at index %d: %+v != %+v", i, pick1[i], pick2[i])
		}
		if pick1[i].Spec.EA == 0x100 {
			t.Fatalf("thunk 0x100 must be excluded from benchmark corpus")
		}
		if pick1[i].Spec.EA == 0x00f53830 {
			foundFactionCtor = true
		}
	}
	if !foundFactionCtor {
		t.Fatalf("expected pinned 0x00f53830 (FACTION::FACTION) in attila_64 benchmark corpus")
	}
}

func TestResolveDecompTimeoutSecsAndSendDecompRecord(t *testing.T) {
	if got := resolveDecompTimeoutSecs(DecompOptions{}); got != DefaultFastTimeoutSecs {
		t.Fatalf("default timeout = %d, want %d", got, DefaultFastTimeoutSecs)
	}
	if got := resolveDecompTimeoutSecs(DecompOptions{TimeoutSecs: 30}); got != 30 {
		t.Fatalf("explicit timeout = %d, want 30", got)
	}
	if got := resolveDecompTimeoutSecs(DecompOptions{NoTimeout: true}); got != 0 {
		t.Fatalf("NoTimeout = %d, want 0", got)
	}
	if got := resolveDecompTimeoutSecs(DecompOptions{TimeoutSecs: 30, NoTimeout: true}); got != 0 {
		t.Fatalf("NoTimeout override = %d, want 0", got)
	}
	if got := resolveDecompTimeoutSecs(DecompOptions{TimeoutSecs: -1}); got != 0 {
		t.Fatalf("negative TimeoutSecs = %d, want 0", got)
	}

	// Cancelled context must not block when sending to an unbuffered channel.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	unbuffered := make(chan extractdb.DecompRecord)
	if ok := sendDecompRecord(ctx, unbuffered, extractdb.DecompRecord{FuncEA: 0x1000}); ok {
		t.Fatalf("expected sendDecompRecord to return false on cancelled context")
	}
}
