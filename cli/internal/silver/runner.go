package silver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"twdll/cli/internal/analyze"
	"twdll/cli/internal/bronze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

func ExtractSilverTopology(ctx context.Context, opts SilverOptions) (*bronze.SilverMergeResult, error) {
	releasePower := analyze.AcquirePowerAssertion()
	defer releasePower()

	if !opts.Target.IsValid() {
		return nil, fmt.Errorf("invalid target id %q", opts.Target)
	}
	if !opts.Engine.IsValid() {
		return nil, fmt.Errorf("invalid silver engine %q", opts.Engine)
	}
	if opts.BinaryPath == "" || opts.BinaryDBPath == "" {
		return nil, errors.New("BinaryPath and BinaryDBPath are required")
	}

	var (
		silverFuncs []bronze.SilverFunc
		err         error
	)
	switch opts.Engine {
	case workspace.DecompilerGhidra:
		silverFuncs, err = runGhidraSilverExport(ctx, opts)
	case workspace.DecompilerIDA:
		silverFuncs, err = runIDASilverExport(ctx, opts)
	default:
		return nil, fmt.Errorf("unsupported engine %q", opts.Engine)
	}
	if err != nil {
		return nil, err
	}

	return bronze.ApplySilverTopology(ctx, bronze.SilverMergeOptions{
		Target:       opts.Target,
		BinaryPath:   opts.BinaryPath,
		BinaryDBPath: opts.BinaryDBPath,
		ManifestPath: opts.ManifestPath,
		Engine:       opts.Engine,
		Funcs:        silverFuncs,
	})
}

func RunDecompPass(ctx context.Context, opts DecompOptions) (*DecompPassResult, error) {
	releasePower := analyze.AcquirePowerAssertion()
	defer releasePower()

	t0 := time.Now()
	if !opts.Target.IsValid() {
		return nil, fmt.Errorf("invalid target id %q", opts.Target)
	}
	if !opts.Engine.IsValid() {
		return nil, fmt.Errorf("invalid decomp engine %q", opts.Engine)
	}
	if opts.BinaryDBPath == "" || opts.DecompDBPath == "" {
		return nil, errors.New("BinaryDBPath and DecompDBPath are required")
	}

	binSHA, allSpecs, err := loadSpecsFromFacts(ctx, opts.BinaryDBPath)
	if err != nil {
		return nil, err
	}
	specs := allSpecs
	if len(opts.FilterSpecs) > 0 {
		specs = opts.FilterSpecs
	}

	store, err := extractdb.OpenDecompStore(ctx, opts.DecompDBPath, string(opts.Target), binSHA)
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()

	pending, resume, err := store.PlanWork(ctx, string(opts.Engine), specs, extractdb.PlanOptions{
		Force:         opts.Force,
		RetryFailed:   opts.RetryFailed,
		RetryTimeouts: opts.RetryTimeouts,
	})
	if err != nil {
		return nil, err
	}

	if len(pending) == 0 {
		counts, countErr := queryEngineStatusCounts(ctx, store.DB(), string(opts.Engine))
		if countErr != nil {
			return nil, countErr
		}
		return &DecompPassResult{
			Target:       opts.Target,
			Engine:       opts.Engine,
			Resume:       resume,
			Written:      0,
			StatusCounts: counts,
			Elapsed:      time.Since(t0),
		}, nil
	}

	outCh := make(chan extractdb.DecompRecord, 512)
	type writerOut struct {
		written int
		err     error
	}
	writerDone := make(chan writerOut, 1)
	go func() {
		n, wErr := store.RunBatchWriter(ctx, outCh, opts.BatchSize, 0)
		writerDone <- writerOut{written: n, err: wErr}
	}()

	var (
		peakMem uint64
		runErr  error
	)
	switch opts.Engine {
	case workspace.DecompilerGhidra:
		peakMem, runErr = runGhidraDecompExport(ctx, opts, pending, outCh)
	case workspace.DecompilerIDA:
		peakMem, runErr = runIDADecompExport(ctx, opts, pending, outCh)
	}
	close(outCh)
	wRes := <-writerDone
	if runErr != nil {
		return nil, runErr
	}
	if wRes.err != nil {
		return nil, wRes.err
	}

	counts, err := queryEngineStatusCounts(ctx, store.DB(), string(opts.Engine))
	if err != nil {
		return nil, err
	}

	return &DecompPassResult{
		Target:          opts.Target,
		Engine:          opts.Engine,
		Resume:          resume,
		Written:         wRes.written,
		StatusCounts:    counts,
		Elapsed:         time.Since(t0),
		PeakJobMemBytes: peakMem,
	}, nil
}

func loadSpecsFromFacts(ctx context.Context, factsPath string) (string, []extractdb.FuncSpec, error) {
	db, err := extractdb.OpenReadOnly(factsPath)
	if err != nil {
		return "", nil, err
	}
	defer db.Close()

	var binSHA string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, extractdb.MetaBinarySHA256).Scan(&binSHA); err != nil {
		return "", nil, fmt.Errorf("read binary_sha256 from %s: %w", factsPath, err)
	}

	rows, err := db.QueryContext(ctx, `SELECT ea, size, name FROM functions ORDER BY ea ASC`)
	if err != nil {
		return "", nil, fmt.Errorf("query functions from %s: %w", factsPath, err)
	}
	defer rows.Close()

	var specs []extractdb.FuncSpec
	for rows.Next() {
		var ea, sz int64
		var name string
		if err := rows.Scan(&ea, &sz, &name); err != nil {
			return "", nil, fmt.Errorf("scan function spec: %w", err)
		}
		specs = append(specs, extractdb.FuncSpec{
			EA:   uint64(ea),
			Size: uint64(sz),
			Name: name,
		})
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	return binSHA, specs, nil
}

func queryEngineStatusCounts(ctx context.Context, db *sql.DB, engine string) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM decomp_funcs WHERE engine = ? GROUP BY status
	`, engine)
	if err != nil {
		return nil, fmt.Errorf("query decomp status counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var st string
		var cnt int
		if err := rows.Scan(&st, &cnt); err != nil {
			return nil, err
		}
		counts[st] = cnt
	}
	return counts, rows.Err()
}

type jsonlTailer struct {
	path    string
	offset  int64
	lineBuf string
}

func (t *jsonlTailer) Drain(onFrame func(decompFrameJSON)) error {
	f, err := os.Open(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	if t.offset > 0 {
		if _, err := f.Seek(t.offset, 0); err != nil {
			return err
		}
	}
	buf := make([]byte, 64*1024)
	for {
		n, rErr := f.Read(buf)
		if n > 0 {
			t.offset += int64(n)
			t.lineBuf += string(buf[:n])
			for {
				nl := strings.IndexByte(t.lineBuf, '\n')
				if nl < 0 {
					break
				}
				rawLine := strings.TrimSpace(t.lineBuf[:nl])
				t.lineBuf = t.lineBuf[nl+1:]
				if rawLine == "" {
					continue
				}
				var frame decompFrameJSON
				if jErr := json.Unmarshal([]byte(rawLine), &frame); jErr != nil {
					return fmt.Errorf("decode decomp jsonl frame: %w", jErr)
				}
				onFrame(frame)
			}
		}
		if rErr != nil {
			break
		}
	}
	return nil
}
