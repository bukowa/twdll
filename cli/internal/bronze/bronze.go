package bronze

import (
	"context"
	"database/sql"
	"debug/elf"
	"debug/pe"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

// DataSlice exposes a contiguous data section for deterministic string scanning.
type DataSlice struct {
	Name       string
	VA         uint64
	Data       []byte
	Is64Bit    bool
	HasRelocAt func(off int) bool
}

// Source holds open binary format handles and shared deterministic metadata populated
// when the binary is opened during a single ExtractTarget run.
type Source struct {
	Target     workspace.TargetID
	BinaryPath string
	ImageBase  uint64
	Arch       string
	Format     string
	PE         *pe.File
	ELF        *elf.File
	DataSlices []DataSlice

	pe32 *pe32Prepared
}

// Extractor populates one deterministic format or debug-info facet into bronze.sqlite.
// Extractors must never disassemble code or run heuristic recovery.
type Extractor interface {
	ID() string
	AppliesTo(src *Source) bool
	Extract(ctx context.Context, src *Source, db *sql.DB) error
}

type Options struct {
	Target             workspace.TargetID
	BinaryPath         string
	BinarySHA256       string
	BronzeDBPath       string
	ManifestPath       string
	Extractors         []Extractor
	DisabledExtractors []string
}

type Result struct {
	Target       workspace.TargetID
	BinarySHA256 string
	ImageBase    uint64
	Extractors   []string
	Counts       map[string]int64
	Elapsed      time.Duration
}

// DefaultExtractors returns the ordered built-in binary and DWARF extractors.
func DefaultExtractors() []Extractor {
	return []Extractor{
		PE32Extractor{},
		ELF64Extractor{},
		StringsExtractor{},
		ELF64DWARF4Extractor{},
	}
}

func ExtractTarget(ctx context.Context, opts Options) (*Result, error) {
	t0 := time.Now()
	if !opts.Target.IsValid() {
		return nil, fmt.Errorf("invalid target id %q", opts.Target)
	}
	if opts.BinaryPath == "" || opts.BronzeDBPath == "" {
		return nil, errors.New("BinaryPath and BronzeDBPath are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sha := strings.TrimSpace(opts.BinarySHA256)
	if sha == "" {
		var err error
		sha, err = workspace.ComputeFileSHA256(opts.BinaryPath)
		if err != nil {
			return nil, fmt.Errorf("hash binary %s: %w", opts.BinaryPath, err)
		}
	}

	src, closeSrc, err := openSource(opts.Target, opts.BinaryPath)
	if err != nil {
		return nil, err
	}
	defer closeSrc()

	extractors := opts.Extractors
	if len(extractors) == 0 {
		extractors = DefaultExtractors()
	}
	disabled := make(map[string]bool, len(opts.DisabledExtractors))
	for _, id := range opts.DisabledExtractors {
		if id = strings.TrimSpace(id); id != "" {
			disabled[id] = true
		}
	}

	builder, err := extractdb.NewAtomicBuilder(opts.BronzeDBPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = builder.Abort() }()

	db := builder.DB()
	if err := extractdb.InitBronzeSchema(ctx, db); err != nil {
		return nil, err
	}

	applied := make([]string, 0, len(extractors))
	for _, ext := range extractors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if ext == nil || disabled[ext.ID()] || !ext.AppliesTo(src) {
			continue
		}
		if err := ext.Extract(ctx, src, db); err != nil {
			return nil, fmt.Errorf("bronze extractor %s: %w", ext.ID(), err)
		}
		applied = append(applied, ext.ID())
	}

	now := time.Now().UTC().Format(time.RFC3339)
	appliedJSON, err := json.Marshal(applied)
	if err != nil {
		return nil, fmt.Errorf("marshal extractors_applied: %w", err)
	}
	for _, kv := range [][2]string{
		{extractdb.MetaTarget, string(opts.Target)},
		{extractdb.MetaBinarySHA256, sha},
		{extractdb.MetaImageBase, fmt.Sprintf("0x%x", src.ImageBase)},
		{extractdb.MetaArch, src.Arch},
		{extractdb.MetaFormat, src.Format},
		{extractdb.MetaExtractedAt, now},
		{extractdb.MetaExtractorsApplied, string(appliedJSON)},
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			kv[0], kv[1],
		); err != nil {
			return nil, fmt.Errorf("store bronze meta %s: %w", kv[0], err)
		}
	}

	counts, err := extractdb.RefreshBronzeCounts(ctx, db)
	if err != nil {
		return nil, err
	}
	if err := extractdb.RebuildBronzeFTS(ctx, db); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `ANALYZE;`); err != nil {
		return nil, fmt.Errorf("analyze bronze db: %w", err)
	}
	if err := builder.Commit(ctx); err != nil {
		return nil, err
	}

	if opts.ManifestPath != "" {
		mCounts := make(map[string]int, len(counts))
		for k, v := range counts {
			mCounts[k] = int(v)
		}
		if err := extractdb.WriteManifestAtomic(opts.ManifestPath, extractdb.TargetManifest{
			Target:       string(opts.Target),
			BinarySHA256: sha,
			ExtractedAt:  now,
			Extractors:   applied,
			Counts:       mCounts,
		}); err != nil {
			return nil, err
		}
	}

	return &Result{
		Target:       opts.Target,
		BinarySHA256: sha,
		ImageBase:    src.ImageBase,
		Extractors:   applied,
		Counts:       counts,
		Elapsed:      time.Since(t0),
	}, nil
}

func openSource(target workspace.TargetID, binaryPath string) (*Source, func(), error) {
	src := &Source{
		Target:     target,
		BinaryPath: binaryPath,
	}
	if target.Is64Bit() {
		ef, err := elf.Open(binaryPath)
		if err != nil {
			return nil, func() {}, fmt.Errorf("open elf64 %s: %w", binaryPath, err)
		}
		src.ELF = ef
		if err := prepareELF64(src); err != nil {
			_ = ef.Close()
			return nil, func() {}, err
		}
		return src, func() { _ = ef.Close() }, nil
	}

	pf, err := pe.Open(binaryPath)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open pe32 %s: %w", binaryPath, err)
	}
	src.PE = pf
	if err := preparePE32(src); err != nil {
		_ = pf.Close()
		return nil, func() {}, err
	}
	return src, func() { _ = pf.Close() }, nil
}
