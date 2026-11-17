package bronze

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

type Options struct {
	Target       workspace.TargetID
	BinaryPath   string
	BinaryDBPath string
	ManifestPath string
}

type FunctionSeed struct {
	EA            uint64
	Name          string
	CallingConv   string
	CCSource      string
	StackPurge    int
	DiscoveryMask uint32
	ThunkTargetEA uint64
	VtableEA      uint64
	VtableSlot    int
	Flags         uint32
}

type Result struct {
	Target       workspace.TargetID
	BinarySHA256 string
	ImageBase    uint64
	Sources      []string
	Counts       map[string]int64
	Seeds        []FunctionSeed
	Elapsed      time.Duration
}

type targetExtractResult struct {
	imageBase uint64
	arch      string
	format    string
	sources   []string
	counts    map[string]int64
	seeds     []FunctionSeed
}

func ExtractTarget(ctx context.Context, opts Options) (*Result, error) {
	t0 := time.Now()
	if !opts.Target.IsValid() {
		return nil, fmt.Errorf("invalid target id %q", opts.Target)
	}
	if opts.BinaryPath == "" {
		return nil, fmt.Errorf("binary path is required for %s", opts.Target)
	}
	if opts.BinaryDBPath == "" {
		return nil, fmt.Errorf("binary db path is required for %s", opts.Target)
	}

	binSHA, err := workspace.ComputeFileSHA256(opts.BinaryPath)
	if err != nil {
		return nil, fmt.Errorf("hash binary %s: %w", opts.BinaryPath, err)
	}

	builder, err := extractdb.NewAtomicBuilder(opts.BinaryDBPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = builder.Abort() }()

	db := builder.DB()
	if err := extractdb.InitFactsSchema(ctx, db); err != nil {
		return nil, err
	}

	var extRes *targetExtractResult
	if opts.Target.Is64Bit() {
		extRes, err = extractELF64(ctx, db, opts.BinaryPath)
	} else {
		extRes, err = extractPE32(ctx, db, opts.BinaryPath)
	}
	if err != nil {
		return nil, err
	}

	extractedAt := time.Now().UTC().Format(time.RFC3339)
	metaPairs := [][2]string{
		{extractdb.MetaTarget, string(opts.Target)},
		{extractdb.MetaBinarySHA256, binSHA},
		{extractdb.MetaImageBase, fmt.Sprintf("0x%x", extRes.imageBase)},
		{extractdb.MetaArch, extRes.arch},
		{extractdb.MetaFormat, extRes.format},
		{extractdb.MetaBronzeExtractedAt, extractedAt},
	}
	for _, kv := range metaPairs {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			kv[0], kv[1],
		); err != nil {
			return nil, fmt.Errorf("insert meta %s: %w", kv[0], err)
		}
	}

	if err := populateTableCounts(ctx, db, extRes.counts); err != nil {
		return nil, err
	}

	if err := extractdb.RebuildFactsFTS(ctx, db); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `ANALYZE;`); err != nil {
		return nil, fmt.Errorf("analyze facts sqlite: %w", err)
	}

	if err := builder.Commit(ctx); err != nil {
		return nil, err
	}

	if opts.ManifestPath != "" {
		manifestCounts := make(map[string]int, len(extRes.counts))
		for k, v := range extRes.counts {
			manifestCounts[k] = int(v)
		}
		m := extractdb.TargetManifest{
			SchemaVersion:      extractdb.ManifestSchemaVersion,
			Target:             string(opts.Target),
			BinarySHA256:       binSHA,
			ExtractedAt:        extractedAt,
			FactsSchemaVersion: extractdb.FactsSchemaVersion,
			Sources:            extRes.sources,
			Counts:             manifestCounts,
		}
		if err := extractdb.WriteManifestAtomic(opts.ManifestPath, m); err != nil {
			return nil, err
		}
	}

	return &Result{
		Target:       opts.Target,
		BinarySHA256: binSHA,
		ImageBase:    extRes.imageBase,
		Sources:      extRes.sources,
		Counts:       extRes.counts,
		Seeds:        extRes.seeds,
		Elapsed:      time.Since(t0),
	}, nil
}

func populateTableCounts(ctx context.Context, db *sql.DB, counts map[string]int64) error {
	tables := []string{
		"sections", "relocs", "imports", "exports",
		"functions", "func_ranges", "types", "type_members", "enum_values",
		"func_vars", "source_files", "names", "inlines",
		"strings", "string_xrefs", "call_sites", "data_xrefs",
		"vtables", "lua_regs",
	}
	for _, tbl := range tables {
		var cnt int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tbl).Scan(&cnt); err != nil {
			return fmt.Errorf("count %s: %w", tbl, err)
		}
		counts[tbl] = cnt
	}
	return nil
}

func LoadSeedsFromFacts(ctx context.Context, db *sql.DB) ([]FunctionSeed, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT ea, name, calling_conv, cc_source, stack_purge, discovery_mask,
		       thunk_target_ea, vtable_ea, vtable_slot, flags
		FROM functions
		ORDER BY ea ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query functions for seeds: %w", err)
	}
	defer rows.Close()

	var seeds []FunctionSeed
	for rows.Next() {
		var (
			ea            int64
			name          string
			cc            string
			ccSrc         string
			stackPurge    int
			discMask      int64
			thunkTargetEA sql.NullInt64
			vtableEA      sql.NullInt64
			vtableSlot    sql.NullInt64
			flags         int64
		)
		if err := rows.Scan(
			&ea, &name, &cc, &ccSrc, &stackPurge, &discMask,
			&thunkTargetEA, &vtableEA, &vtableSlot, &flags,
		); err != nil {
			return nil, fmt.Errorf("scan function seed: %w", err)
		}
		seed := FunctionSeed{
			EA:            uint64(ea),
			Name:          name,
			CallingConv:   cc,
			CCSource:      ccSrc,
			StackPurge:    stackPurge,
			DiscoveryMask: uint32(discMask),
			VtableSlot:    -1,
			Flags:         uint32(flags),
		}
		if thunkTargetEA.Valid {
			seed.ThunkTargetEA = uint64(thunkTargetEA.Int64)
		}
		if vtableEA.Valid {
			seed.VtableEA = uint64(vtableEA.Int64)
		}
		if vtableSlot.Valid {
			seed.VtableSlot = int(vtableSlot.Int64)
		}
		seeds = append(seeds, seed)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate function seeds: %w", err)
	}
	return seeds, nil
}

func nullInt64(v int64, valid bool) sql.NullInt64 {
	if !valid {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

func nullUint64NonZero(v uint64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(v), Valid: true}
}

func formatSubName(ea uint64) string {
	return "sub_" + strconv.FormatUint(ea, 16)
}
