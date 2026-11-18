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

			bronzeDBPath := filepath.Join(outDir, string(target), "bronze.sqlite")
			manifestPath := filepath.Join(outDir, string(target), "manifest.json")

			res, err := ExtractTarget(ctx, Options{
				Target:       target,
				BinaryPath:   binPath,
				BronzeDBPath: bronzeDBPath,
				ManifestPath: manifestPath,
			})
			if err != nil {
				t.Fatalf("ExtractTarget(%s) failed: %v", target, err)
			}

			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			t.Logf("[%s] elapsed=%v sha256=%s image_base=0x%x HeapInuse=%d MiB HeapAlloc=%d MiB counts=%v",
				target, res.Elapsed, res.BinarySHA256[:12], res.ImageBase,
				ms.HeapInuse/(1024*1024), ms.HeapAlloc/(1024*1024), res.Counts)

			db, err := extractdb.OpenReadOnly(bronzeDBPath)
			if err != nil {
				t.Fatalf("OpenReadOnly(%s): %v", target, err)
			}
			defer db.Close()

			if res.Counts["sections"] <= 0 {
				t.Errorf("[%s] sections count = %d, want > 0", target, res.Counts["sections"])
			}

			var badLineSpan, orphanInlines int
			if err := db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM dwarf_funcs WHERE low_line > 0 AND high_line < low_line
			`).Scan(&badLineSpan); err != nil {
				t.Fatal(err)
			}
			if badLineSpan != 0 {
				t.Errorf("[%s] dwarf_funcs with high_line < low_line = %d, want 0", target, badLineSpan)
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

				for _, q := range []string{
					`SELECT COUNT(*) FROM dwarf_funcs_fts WHERE dwarf_funcs_fts MATCH 'FACTION'`,
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

				var eorEA int64
				var eorLowLine, eorHighLine, eorVars, eorInlines int
				if err := db.QueryRowContext(ctx, `
					SELECT ea, low_line, high_line FROM dwarf_funcs
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
			} else {
				if res.Counts["dwarf_funcs"] != 0 || res.Counts["elf_symbols"] != 0 {
					t.Errorf("[%s] expected 0 DWARF/ELF rows on PE32, got counts=%v", target, res.Counts)
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
