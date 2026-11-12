package analyze

import "twdll/cli/internal/workspace"

// Item describes a concrete binary target to be analyzed.
type Item struct {
	Target       workspace.TargetID     // e.g. "attila_32", "rome2_32"
	Decompiler   workspace.DecompilerID // "ida" or "ghidra"
	BinaryPath   string                 // path to staged .twdll/binaries/<target>/...
	BinarySHA256 string                 // verified SHA-256 of the binary
	OutputDir    string                 // destination .twdll/db/<engine>/<target>/
	MarkerFile   string                 // canonical database file that proves completion
	LogFile      string                 // path to .twdll/logs/analyze_<engine>_<target>.log
	NeedsRun     bool                   // false if already analyzed and hash matches
	SkipReason   string                 // human-readable reason when skipped
}

// Options configures the analyze command run.
type Options struct {
	Decompiler   string   // optional decompiler backend override ("ghidra", "ida")
	Clean        bool     // wipe existing database before analyzing
	Force        bool     // ignore existing state and re-analyze
	Jobs         int      // concurrency limit (default 1 for Ghidra, up to 2 for IDA)
	MaxMem       string   // optional Ghidra max JVM heap override (e.g. "16G", "32G")
	FullAnalysis bool     // keep all default Ghidra analyzers enabled even on attila_64
	Targets      []string // optional target filter (e.g. "attila_32")
}
