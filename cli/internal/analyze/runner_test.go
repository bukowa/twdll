package analyze

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

func TestRun_GhidraEndToEnd_StagesDotFreeKeepsWorkspaceCleanAndCheckpointsState(t *testing.T) {
	// Given a workspace inside a .twdll directory with fake Ghidra + JDK tools and two targets
	root := t.TempDir()
	layout := newTestLayout(t, root, workspace.DecompilerGhidra)
	installFakeGhidraAndJDK(t, layout.Paths)

	sha64 := stageFakeBinary(t, layout.Paths, workspace.TargetAttila64, []byte("ELF64-OK"))
	sha32 := stageFakeBinary(t, layout.Paths, workspace.TargetAttila32, []byte("PE32-OK"))
	seed := workspace.NewState()
	seed.UpsertBinary(workspace.TargetAttila32, "C:/Games/Attila/empire.retail.dll", sha32)
	if err := workspace.SaveState(layout.Paths.StateFile, seed); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever})

	// When Run executes headless Ghidra analysis
	if err := Run(context.Background(), Request{Layout: layout, UI: ui}); err != nil {
		t.Fatalf("Run(ghidra): %v\nstderr:\n%s", err, stderr.String())
	}

	// Then stdout is 0 bytes, .twdll/binaries and .twdll root remain clean, DBs + logs exist, and state is recorded
	if stdout.Len() != 0 {
		t.Fatalf("expected 0 bytes on stdout, got %q", stdout.String())
	}
	assertCleanWorkspace(t, layout.Paths)
	for _, tid := range []workspace.TargetID{workspace.TargetAttila64, workspace.TargetAttila32} {
		if !layout.Paths.HasValidDatabase(workspace.DecompilerGhidra, tid) {
			t.Fatalf("expected valid Ghidra database for %s", tid)
		}
		if info, err := os.Stat(layout.Paths.AnalyzeLogPath(workspace.DecompilerGhidra, tid)); err != nil || info.Size() == 0 {
			t.Fatalf("expected non-empty log for %s", tid)
		}
	}
	after, _, err := workspace.LoadState(layout.Paths.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if after.Binaries[workspace.TargetAttila64].Analyzed.Ghidra != sha64 || after.Binaries[workspace.TargetAttila32].Analyzed.Ghidra != sha32 {
		t.Fatalf("unexpected analyzed state: %+v", after.Binaries)
	}
	if after.Binaries[workspace.TargetAttila32].SourcePath != "C:/Games/Attila/empire.retail.dll" {
		t.Fatalf("expected SourcePath preserved, got %q", after.Binaries[workspace.TargetAttila32].SourcePath)
	}

	// When Run is invoked a second time without changes, Then it skips immediately without needing tools
	_ = os.RemoveAll(layout.Paths.ToolsDir)
	stderr.Reset()
	if err := Run(context.Background(), Request{Layout: layout, UI: ui}); err != nil {
		t.Fatalf("second Run should skip cleanly: %v", err)
	}
	if !strings.Contains(stderr.String(), "already analyzed") || stdout.Len() != 0 {
		t.Fatalf("expected skip message on stderr only, stderr=%q stdout=%q", stderr.String(), stdout.String())
	}
}

func TestRun_IDAEndToEnd_PreservesCompletedCheckpointWhenSiblingTargetFails(t *testing.T) {
	// Given an IDA directory containing spaces, one healthy target (attila_32), and one failing target (rome2_32)
	root := t.TempDir()
	layout := newTestLayout(t, root, workspace.DecompilerIDA)
	idaDir := filepath.Join(root, "Fake IDA Pro 9.0")
	installFakeIDA(t, idaDir)
	layout.Config.IDADir = idaDir

	sha32 := stageFakeBinary(t, layout.Paths, workspace.TargetAttila32, []byte("PE32-OK"))
	stageFakeBinary(t, layout.Paths, workspace.TargetRome2_32, []byte("FAIL-PE32"))

	var stdout, stderr bytes.Buffer
	ui := term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever})

	// When Run analyzes both targets concurrently
	err := Run(context.Background(), Request{
		Layout:  layout,
		Options: Options{Decompiler: "ida", Jobs: 2, Targets: []string{"attila_32", "rome2_32"}},
		UI:      ui,
	})
	if err == nil {
		t.Fatalf("expected non-nil error when rome2_32 exits 1")
	}

	// Then stdout stays empty, binaries dir has zero IDA sidecar leaks, attila_32 is checkpointed, and rome2_32 is unrecorded
	if stdout.Len() != 0 {
		t.Fatalf("expected 0 bytes on stdout, got %q", stdout.String())
	}
	assertCleanWorkspace(t, layout.Paths)
	after, _, sErr := workspace.LoadState(layout.Paths.StateFile)
	if sErr != nil {
		t.Fatal(sErr)
	}
	if after.Binaries[workspace.TargetAttila32].Analyzed.IDA != sha32 {
		t.Fatalf("expected attila_32 checkpointed with %s, got %+v", sha32, after.Binaries[workspace.TargetAttila32])
	}
	if after.Binaries[workspace.TargetRome2_32].Analyzed.IDA != "" {
		t.Fatalf("expected failing rome2_32 to remain unrecorded, got %+v", after.Binaries[workspace.TargetRome2_32])
	}
}

func TestRun_Executes32BitBefore64BitSolo_ForGhidraAndIDA(t *testing.T) {
	for _, engine := range []workspace.DecompilerID{workspace.DecompilerGhidra, workspace.DecompilerIDA} {
		t.Run(string(engine), func(t *testing.T) {
			root := t.TempDir()
			layout := newTestLayout(t, root, engine)
			installFakeGhidraAndJDK(t, layout.Paths)
			idaDir := filepath.Join(root, "FakeIDA")
			installFakeIDA(t, idaDir)
			layout.Config.IDADir = idaDir

			// Stage fast attila_64 and slower 32-bit binaries; if 64-bit ran first or concurrently, it would finish first
			stageFakeBinary(t, layout.Paths, workspace.TargetAttila64, []byte("ELF64-OK"))
			stageFakeBinary(t, layout.Paths, workspace.TargetAttila32, []byte("PE32-SLOW32-ATTILA"))
			stageFakeBinary(t, layout.Paths, workspace.TargetRome2_32, []byte("PE32-SLOW32-ROME2"))

			var stdout, stderr bytes.Buffer
			ui := term.NewUI(term.Streams{Out: &stdout, Err: &stderr, ColorMode: term.ColorNever})

			if err := Run(context.Background(), Request{
				Layout: layout,
				Options: Options{
					Decompiler: string(engine),
					Jobs:       3,
					Targets:    []string{"attila_64", "attila_32", "rome2_32"},
				},
				UI: ui,
			}); err != nil {
				t.Fatalf("Run(%s): %v\nstderr:\n%s", engine, err, stderr.String())
			}

			if stdout.Len() != 0 {
				t.Fatalf("expected 0 bytes on stdout, got %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), "analyze_resources.jsonl") {
				t.Fatalf("expected resource telemetry path on stderr, got:\n%s", stderr.String())
			}
			assertCleanWorkspace(t, layout.Paths)

			raw, err := os.ReadFile(layout.Paths.AnalyzeResourceLogPath())
			if err != nil {
				t.Fatalf("read analyze_resources.jsonl: %v", err)
			}
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(lines) != 3 {
				t.Fatalf("expected 3 JSONL records, got %d:\n%s", len(lines), string(raw))
			}

			records := make([]ResourceRecord, 0, 3)
			for i, line := range lines {
				var rec ResourceRecord
				if err := json.Unmarshal([]byte(line), &rec); err != nil {
					t.Fatalf("unmarshal record %d: %v", i, err)
				}
				if rec.Engine != string(engine) || rec.Status != "ok" {
					t.Fatalf("unexpected record[%d]: %+v", i, rec)
				}
				records = append(records, rec)
			}

			// First two completed targets must be attila_32 and rome2_32 with concurrent_jobs == 2
			firstTwo := map[string]bool{records[0].Target: true, records[1].Target: true}
			if !firstTwo["attila_32"] || !firstTwo["rome2_32"] {
				t.Fatalf("expected [attila_32, rome2_32] to complete before attila_64, got order [%s, %s, %s]",
					records[0].Target, records[1].Target, records[2].Target)
			}
			if records[0].ConcurrentJobs != 2 || records[1].ConcurrentJobs != 2 {
				t.Fatalf("expected 32-bit batch concurrent_jobs=2, got %d and %d", records[0].ConcurrentJobs, records[1].ConcurrentJobs)
			}

			// Final target must be attila_64 running solo (concurrent_jobs == 1)
			if records[2].Target != "attila_64" || records[2].ConcurrentJobs != 1 {
				t.Fatalf("expected final solo attila_64 record with concurrent_jobs=1, got %+v", records[2])
			}
		})
	}
}
