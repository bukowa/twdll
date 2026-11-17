package bronze

import (
	"context"
	"path/filepath"
	"testing"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

func TestApplySilverTopology_PreservesBronzeAndAddsSilverOnly(t *testing.T) {
	ctx := context.Context(t.Context())
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "empire.retail.dll")
	binaryDBPath := filepath.Join(tmpDir, "binary.sqlite")
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	fix := buildSyntheticPE32DLL(t, binPath)

	if _, err := ExtractTarget(ctx, Options{
		Target:       workspace.TargetAttila32,
		BinaryPath:   binPath,
		BinaryDBPath: binaryDBPath,
		ManifestPath: manifestPath,
	}); err != nil {
		t.Fatalf("ExtractTarget: %v", err)
	}

	silverFuncs := []SilverFunc{
		{
			// VTable method: Bronze vtable __thiscall must NOT be overwritten by Silver __stdcall.
			EA:          uint64(fix.fnVirt0VA),
			Size:        4,
			Name:        "FACTION::GetGold",
			CallingConv: "__stdcall",
			StackPurge:  8,
			Ranges:      []SilverRange{{StartEA: uint64(fix.fnVirt0VA), EndEA: uint64(fix.fnVirt0VA + 4)}},
		},
		{
			// Plain bronze_x86asm __cdecl: may adopt Silver __fastcall / name / stack_purge.
			EA:          uint64(fix.fnSeteCdeclVA),
			Size:        24,
			Name:        "CAMPAIGN::CheckFlag",
			CallingConv: "__fastcall",
			StackPurge:  4,
			Ranges:      []SilverRange{{StartEA: uint64(fix.fnSeteCdeclVA), EndEA: uint64(fix.fnSeteCdeclVA + 24)}},
		},
		{
			// FuncFlagNoSeedOverride (__chkstk): must NOT adopt Silver CC or stack_purge.
			EA:          uint64(fix.fnChkstkVA),
			Size:        10,
			Name:        "__chkstk",
			CallingConv: "__stdcall",
			StackPurge:  8,
			Ranges:      []SilverRange{{StartEA: uint64(fix.fnChkstkVA), EndEA: uint64(fix.fnChkstkVA + 10)}},
		},
		{
			// Silver-only function inside .text (not seeded by Bronze), plus check range clipping.
			EA:          uint64(fix.swCaseVAs[1]),
			Size:        3,
			Name:        "ColdHelper",
			CallingConv: "__cdecl",
			Ranges:      []SilverRange{{StartEA: uint64(fix.swCaseVAs[1]), EndEA: uint64(fix.swCaseVAs[1] + 3)}},
		},
		{
			// External/invalid EA outside .text: must be ignored cleanly.
			EA:          0x7FFF0000,
			Size:        16,
			Name:        "EXTERNAL::Bogus",
			CallingConv: "__cdecl",
		},
	}

	res, err := ApplySilverTopology(ctx, SilverMergeOptions{
		Target:       workspace.TargetAttila32,
		BinaryPath:   binPath,
		BinaryDBPath: binaryDBPath,
		ManifestPath: manifestPath,
		Engine:       workspace.DecompilerIDA,
		Funcs:        silverFuncs,
	})
	if err != nil {
		t.Fatalf("ApplySilverTopology: %v", err)
	}
	if res.UpdatedExisting != 3 || res.InsertedNew != 1 {
		t.Fatalf("unexpected SilverMergeResult: %+v (want UpdatedExisting=3, InsertedNew=1)", res)
	}

	db, err := extractdb.OpenReadOnly(binaryDBPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	// 1. VTable method preserves __thiscall + bronze_vtable while adopting non-default name & DiscoveryIDA.
	var cc, ccSrc, name string
	var mask uint32
	if err := db.QueryRowContext(ctx, `SELECT name, calling_conv, cc_source, discovery_mask FROM functions WHERE ea = ?`, int64(fix.fnVirt0VA)).
		Scan(&name, &cc, &ccSrc, &mask); err != nil {
		t.Fatal(err)
	}
	if cc != "__thiscall" || ccSrc != extractdb.CCSourceVtable || name != "FACTION::GetGold" || mask&extractdb.DiscoveryIDA == 0 {
		t.Errorf("vtable method = (name=%q cc=%q ccSrc=%q mask=0x%x), want __thiscall / bronze_vtable / FACTION::GetGold / DiscoveryIDA", name, cc, ccSrc, mask)
	}

	// 2. Plain __cdecl upgrades to __fastcall + silver_ida.
	var purge int
	if err := db.QueryRowContext(ctx, `SELECT name, calling_conv, cc_source, stack_purge FROM functions WHERE ea = ?`, int64(fix.fnSeteCdeclVA)).
		Scan(&name, &cc, &ccSrc, &purge); err != nil {
		t.Fatal(err)
	}
	if cc != "__fastcall" || ccSrc != extractdb.CCSourceIDA || name != "CAMPAIGN::CheckFlag" || purge != 4 {
		t.Errorf("upgraded __cdecl = (name=%q cc=%q ccSrc=%q purge=%d), want CAMPAIGN::CheckFlag / __fastcall / silver_ida / 4", name, cc, ccSrc, purge)
	}

	// 3. __chkstk (FuncFlagNoSeedOverride) stays __cdecl with purge=0.
	if err := db.QueryRowContext(ctx, `SELECT calling_conv, stack_purge FROM functions WHERE ea = ?`, int64(fix.fnChkstkVA)).
		Scan(&cc, &purge); err != nil {
		t.Fatal(err)
	}
	if cc != "__cdecl" || purge != 0 {
		t.Errorf("__chkstk = (cc=%q purge=%d), want __cdecl / 0", cc, purge)
	}

	// 4. Silver-only ColdHelper was inserted and external bogus EA was skipped.
	var coldSize int
	if err := db.QueryRowContext(ctx, `SELECT name, size, discovery_mask FROM functions WHERE ea = ?`, int64(fix.swCaseVAs[1])).
		Scan(&name, &coldSize, &mask); err != nil {
		t.Fatalf("query ColdHelper: %v", err)
	}
	if name != "ColdHelper" || coldSize != 3 || mask != extractdb.DiscoveryIDA {
		t.Errorf("ColdHelper = (name=%q size=%d mask=0x%x)", name, coldSize, mask)
	}

	var bogusCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM functions WHERE ea = ?`, int64(0x7FFF0000)).Scan(&bogusCount); err != nil {
		t.Fatal(err)
	}
	if bogusCount != 0 {
		t.Errorf("bogus external EA count = %d, want 0", bogusCount)
	}

	// 5. Verify FTS5 functions_fts includes newly merged names and roWrites == 0.
	var ftsCnt, roWrites int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM functions_fts WHERE functions_fts MATCH 'ColdHelper'`).Scan(&ftsCnt); err != nil {
		t.Fatal(err)
	}
	if ftsCnt != 1 {
		t.Errorf("functions_fts MATCH 'ColdHelper' = %d, want 1", ftsCnt)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM data_xrefs dx
		JOIN sections s ON dx.to_ea >= s.va AND dx.to_ea < s.va + s.virtual_size
		WHERE s.name IN ('.rdata', '.rodata') AND dx.access_kind = 'write'
	`).Scan(&roWrites); err != nil {
		t.Fatal(err)
	}
	if roWrites != 0 {
		t.Errorf("roWrites = %d, want 0", roWrites)
	}

	// 6. Verify meta and manifest.json updated.
	var silverEng string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, extractdb.MetaSilverEngine).Scan(&silverEng); err != nil {
		t.Fatal(err)
	}
	if silverEng != "ida" {
		t.Errorf("meta silver_engine = %q, want ida", silverEng)
	}
	m, err := extractdb.ReadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if m.Counts["functions"] != res.TotalFunctions {
		t.Errorf("manifest functions count = %d, want %d", m.Counts["functions"], res.TotalFunctions)
	}
	hasSilverSrc := false
	for _, s := range m.Sources {
		if s == "silver_ida" {
			hasSilverSrc = true
		}
	}
	if !hasSilverSrc {
		t.Errorf("manifest sources = %v, want silver_ida included", m.Sources)
	}
}
