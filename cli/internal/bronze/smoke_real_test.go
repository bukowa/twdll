package bronze

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

func TestSmokeRealBinaries(t *testing.T) {
	binsDir := os.Getenv("TWDLL_SMOKE_BINS")
	if binsDir == "" {
		t.Skip("TWDLL_SMOKE_BINS not set; skipping real-binary Bronze smoke suite")
	}

	ctx := context.Context(t.Context())
	outDir := t.TempDir()

	for _, target := range workspace.AllTargets() {
		target := target
		t.Run(string(target), func(t *testing.T) {
			binPath := filepath.Join(binsDir, string(target), target.BinaryFileName())
			if _, err := os.Stat(binPath); err != nil {
				t.Skipf("binary %s not present: %v", binPath, err)
			}

			factsPath := filepath.Join(outDir, string(target), "facts.sqlite")
			manifestPath := filepath.Join(outDir, string(target), "manifest.json")

			res, err := ExtractTarget(ctx, Options{
				Target:       target,
				BinaryPath:   binPath,
				FactsDBPath:  factsPath,
				ManifestPath: manifestPath,
			})
			if err != nil {
				t.Fatalf("ExtractTarget(%s) failed: %v", target, err)
			}

			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			t.Logf("[%s] elapsed=%v sha256=%s image_base=0x%x seeds=%d HeapInuse=%d MiB HeapAlloc=%d MiB counts=%v",
				target, res.Elapsed, res.BinarySHA256[:12], res.ImageBase, len(res.Seeds),
				ms.HeapInuse/(1024*1024), ms.HeapAlloc/(1024*1024), res.Counts)

			db, err := extractdb.OpenReadOnly(factsPath)
			if err != nil {
				t.Fatalf("OpenReadOnly(%s): %v", target, err)
			}
			defer db.Close()

			// Verify LoadSeedsFromFacts round-trip matches Result.Seeds.
			loadedSeeds, err := LoadSeedsFromFacts(ctx, db)
			if err != nil {
				t.Fatalf("LoadSeedsFromFacts(%s): %v", target, err)
			}
			if len(loadedSeeds) != len(res.Seeds) {
				t.Errorf("len(loadedSeeds)=%d != len(res.Seeds)=%d", len(loadedSeeds), len(res.Seeds))
			}

			// Universal data-integrity checks across all targets:
			var roWrites, badLineSpan, orphanInlines int
			if err := db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM data_xrefs dx
				JOIN sections s ON dx.to_ea >= s.va AND dx.to_ea < s.va + s.virtual_size
				WHERE s.name IN ('.rdata', '.rodata') AND dx.access_kind = 'write'
			`).Scan(&roWrites); err != nil {
				t.Fatal(err)
			}
			if roWrites != 0 {
				t.Errorf("[%s] write data_xrefs targeting .rdata/.rodata = %d, want 0", target, roWrites)
			}

			if err := db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM functions WHERE low_line > 0 AND high_line < low_line
			`).Scan(&badLineSpan); err != nil {
				t.Fatal(err)
			}
			if badLineSpan != 0 {
				t.Errorf("[%s] functions with high_line < low_line = %d, want 0", target, badLineSpan)
			}

			if err := db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM inlines i
				WHERE i.parent_idx != -1
				  AND NOT EXISTS (
					SELECT 1 FROM inlines p
					WHERE p.host_ea = i.host_ea AND p.inline_idx = i.parent_idx
				  )
			`).Scan(&orphanInlines); err != nil {
				t.Fatal(err)
			}
			if orphanInlines != 0 {
				t.Errorf("[%s] orphan inline parent_idx rows = %d, want 0", target, orphanInlines)
			}

			if target.Is64Bit() {
				// 1. Assert 0 dangling type references across types, type_members, and func_vars.
				var danglingTarget, danglingMember, danglingVar int
				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM types t
					WHERE t.target_type_id IS NOT NULL
					  AND NOT EXISTS (SELECT 1 FROM types p WHERE p.type_id = t.target_type_id)
				`).Scan(&danglingTarget); err != nil {
					t.Fatal(err)
				}
				if danglingTarget != 0 {
					t.Errorf("dangling types.target_type_id = %d, want 0", danglingTarget)
				}

				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM type_members tm
					WHERE tm.member_type_id IS NOT NULL
					  AND NOT EXISTS (SELECT 1 FROM types p WHERE p.type_id = tm.member_type_id)
				`).Scan(&danglingMember); err != nil {
					t.Fatal(err)
				}
				if danglingMember != 0 {
					t.Errorf("dangling type_members.member_type_id = %d, want 0", danglingMember)
				}

				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM func_vars fv
					WHERE fv.type_id IS NOT NULL
					  AND NOT EXISTS (SELECT 1 FROM types p WHERE p.type_id = fv.type_id)
				`).Scan(&danglingVar); err != nil {
					t.Fatal(err)
				}
				if danglingVar != 0 {
					t.Errorf("dangling func_vars.type_id = %d, want 0", danglingVar)
				}

				// 2. Assert 0 non-NULL byte_offset on static members or virtual bases.
				var badStaticOrVbase int
				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM type_members
					WHERE (member_kind = 'static' OR is_virtual_base = 1)
					  AND byte_offset IS NOT NULL
				`).Scan(&badStaticOrVbase); err != nil {
					t.Fatal(err)
				}
				if badStaticOrVbase != 0 {
					t.Errorf("non-NULL byte_offset on static/virtual-base rows = %d, want 0", badStaticOrVbase)
				}

				// 3. Assert FTS5 matches on functions_fts, types_fts, and strings_fts for FACTION.
				for _, q := range []string{
					`SELECT COUNT(*) FROM functions_fts WHERE functions_fts MATCH 'FACTION'`,
					`SELECT COUNT(*) FROM types_fts WHERE types_fts MATCH 'FACTION'`,
					`SELECT COUNT(*) FROM strings_fts WHERE strings_fts MATCH 'FACTION'`,
				} {
					var cnt int
					if err := db.QueryRowContext(ctx, q).Scan(&cnt); err != nil {
						t.Fatalf("FTS query %q failed: %v", q, err)
					}
					if cnt == 0 {
						t.Errorf("FTS query %q returned 0 matches", q)
					}
				}

				// 4. Assert concrete composite type_members and function DWARF fidelity on attila_64.
				var factionMembers, techMgrMembers, ptrVecMembers int
				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM type_members tm
					JOIN types t ON t.type_id = tm.type_id
					WHERE t.name = 'EMPIRECAMPAIGN::FACTION' AND t.is_declaration = 0
				`).Scan(&factionMembers); err != nil {
					t.Fatal(err)
				}
				if factionMembers <= 20 {
					t.Errorf("EMPIRECAMPAIGN::FACTION concrete member count = %d, want > 20", factionMembers)
				}

				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM type_members tm
					JOIN types t ON t.type_id = tm.type_id
					WHERE t.name = 'EMPIRECAMPAIGN::FACTION_TECHNOLOGY_MANAGER' AND t.is_declaration = 0
				`).Scan(&techMgrMembers); err != nil {
					t.Fatal(err)
				}
				if techMgrMembers <= 0 {
					t.Errorf("EMPIRECAMPAIGN::FACTION_TECHNOLOGY_MANAGER member count = %d, want > 0", techMgrMembers)
				}

				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM type_members tm
					JOIN types t ON t.type_id = tm.type_id
					WHERE t.name LIKE '%CONST_SAFE_PTR_VECTOR%' AND t.is_declaration = 0
				`).Scan(&ptrVecMembers); err != nil {
					t.Fatal(err)
				}
				if ptrVecMembers <= 0 {
					t.Errorf("CONST_SAFE_PTR_VECTOR member count = %d, want > 0", ptrVecMembers)
				}

				var eorEA int64
				var eorLowLine, eorHighLine, eorVars, eorInlines int
				if err := db.QueryRowContext(ctx, `
					SELECT ea, low_line, high_line FROM functions
					WHERE name = 'EMPIRECAMPAIGN::FACTION::end_of_round'
				`).Scan(&eorEA, &eorLowLine, &eorHighLine); err != nil {
					t.Fatalf("query EMPIRECAMPAIGN::FACTION::end_of_round: %v", err)
				}
				if eorLowLine <= 0 || eorHighLine <= 0 {
					t.Errorf("EMPIRECAMPAIGN::FACTION::end_of_round line span = [%d, %d], want > 0", eorLowLine, eorHighLine)
				}
				if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM func_vars WHERE func_ea = ?`, eorEA).Scan(&eorVars); err != nil {
					t.Fatal(err)
				}
				if eorVars <= 0 {
					t.Errorf("EMPIRECAMPAIGN::FACTION::end_of_round func_vars = %d, want > 0", eorVars)
				}
				if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inlines WHERE host_ea = ?`, eorEA).Scan(&eorInlines); err != nil {
					t.Fatal(err)
				}
				if eorInlines <= 0 {
					t.Errorf("EMPIRECAMPAIGN::FACTION::end_of_round inlines = %d, want > 0", eorInlines)
				}
				t.Logf("[%s] FACTION_members=%d TECH_MGR_members=%d PTR_VEC_members=%d end_of_round(ea=0x%x lines=[%d,%d] vars=%d inlines=%d)",
					target, factionMembers, techMgrMembers, ptrVecMembers, uint64(eorEA), eorLowLine, eorHighLine, eorVars, eorInlines)
			} else {
				// 32-bit PE DLL checks: lua_regs (>= 39 tables, >= 600 methods), vtables, functions, strings_fts.
				var luaTables, luaMethods int
				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(DISTINCT table_ea), COUNT(*) FROM lua_regs
				`).Scan(&luaTables, &luaMethods); err != nil {
					t.Fatal(err)
				}
				noSeedOverrideCount := 0
				for _, s := range res.Seeds {
					if s.Flags&extractdb.FuncFlagNoSeedOverride != 0 {
						noSeedOverrideCount++
					}
				}
				t.Logf("[%s] lua_regs: tables=%d methods=%d | vtables_rows=%d | functions=%d | no_seed_override=%d",
					target, luaTables, luaMethods, res.Counts["vtables"], res.Counts["functions"], noSeedOverrideCount)
				if luaTables < 39 {
					t.Errorf("[%s] lua_regs distinct tables = %d, want >= 39", target, luaTables)
				}
				if luaMethods < 600 {
					t.Errorf("[%s] lua_regs total methods = %d, want >= 600", target, luaMethods)
				}
				if res.Counts["vtables"] < 1000 {
					t.Errorf("[%s] vtables count = %d, want >= 1000", target, res.Counts["vtables"])
				}
				if len(res.Seeds) < 10000 {
					t.Errorf("[%s] seeds count = %d, want >= 10000", target, len(res.Seeds))
				}
				var ftsStrings int
				if err := db.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM strings_fts WHERE strings_fts MATCH 'faction'
				`).Scan(&ftsStrings); err != nil {
					t.Fatal(err)
				}
				if ftsStrings == 0 {
					t.Errorf("[%s] strings_fts MATCH 'faction' returned 0 matches", target)
				}
			}
		})
	}
}
