package silver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"twdll/cli/internal/analyze"
	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

// Session carries per-run state, active enrichers, and backend export hooks for Aspects.
type Session struct {
	Opts         Options
	Enrichers    []EnrichScript
	Result       *Result
	Funcs        []SilverFunc
	exportFuncs  func(ctx context.Context, opts Options, enrichers []EnrichScript, asmOutPath, stringsOutPath string) ([]SilverFunc, error)
	exportDecomp func(ctx context.Context, opts Options, enrichers []EnrichScript, pending []extractdb.FuncSpec, outCh chan<- extractdb.DecompRecord) (uint64, error)
}

// DefaultAspects returns the built-in silver aspects ("funcs" and optionally "decomp").
func DefaultAspects(noDecomp bool) []Aspect {
	aspects := []Aspect{funcsAspect{}}
	if !noDecomp {
		aspects = append(aspects, decompAspect{})
	}
	return aspects
}

type (
	funcsExporter  func(ctx context.Context, opts Options, enrichers []EnrichScript, asmOutPath, stringsOutPath string) ([]SilverFunc, error)
	decompExporter func(ctx context.Context, opts Options, enrichers []EnrichScript, pending []extractdb.FuncSpec, outCh chan<- extractdb.DecompRecord) (uint64, error)
)

// ExtractTarget extracts functions and decompiler artifacts from IDA or Ghidra
// directly into the engine's isolated silver_<engine>.sqlite store.
func ExtractTarget(ctx context.Context, opts Options) (*Result, error) {
	return extractWithHooks(ctx, opts, defaultExportFuncs, defaultExportDecomp)
}

func extractWithHooks(ctx context.Context, opts Options, fnExp funcsExporter, decompExp decompExporter) (retRes *Result, retErr error) {
	releasePower := analyze.AcquirePowerAssertion()
	defer releasePower()

	t0 := time.Now()
	if !opts.Target.IsValid() {
		return nil, fmt.Errorf("invalid target id %q", opts.Target)
	}
	if !opts.Engine.IsValid() {
		return nil, fmt.Errorf("invalid silver engine %q", opts.Engine)
	}
	if opts.SilverDBPath == "" {
		return nil, errors.New("SilverDBPath is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	binSHA := strings.TrimSpace(opts.BinarySHA256)
	if binSHA == "" {
		if opts.BinaryPath == "" {
			return nil, errors.New("BinarySHA256 or BinaryPath is required")
		}
		var err error
		binSHA, err = workspace.ComputeFileSHA256(opts.BinaryPath)
		if err != nil {
			return nil, fmt.Errorf("hash binary %s: %w", opts.BinaryPath, err)
		}
	}

	var enricherIDs []string
	var enrichScripts []EnrichScript
	for _, en := range opts.Enrichers {
		if en == nil || !en.AppliesTo(opts.Target, opts.Engine) {
			continue
		}
		sc, err := en.Script(opts.Target, opts.Engine)
		if err != nil {
			return nil, fmt.Errorf("prepare enricher %s: %w", en.ID(), err)
		}
		if strings.TrimSpace(sc.Filename) == "" || strings.TrimSpace(sc.Body) == "" {
			return nil, fmt.Errorf("enricher %s returned empty filename or script body", en.ID())
		}
		enricherIDs = append(enricherIDs, en.ID())
		enrichScripts = append(enrichScripts, sc)
	}
	enrichersKey := strings.Join(enricherIDs, ",")

	store, err := extractdb.OpenSilverStore(
		ctx,
		opts.SilverDBPath,
		string(opts.Target),
		string(opts.Engine),
		binSHA,
		enrichersKey,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cErr := store.Close(); cErr != nil && retErr == nil {
			retRes = nil
			retErr = fmt.Errorf("close silver store: %w", cErr)
		}
	}()

	aspects := opts.Aspects
	if len(aspects) == 0 {
		aspects = DefaultAspects(opts.NoDecomp)
	}

	res := &Result{
		Target:       opts.Target,
		Engine:       opts.Engine,
		BinarySHA256: binSHA,
		Enrichers:    enricherIDs,
		Aspects:      make([]string, 0, len(aspects)),
	}
	sess := &Session{
		Opts:         opts,
		Enrichers:    enrichScripts,
		Result:       res,
		exportFuncs:  fnExp,
		exportDecomp: decompExp,
	}

	for _, asp := range aspects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if asp == nil {
			continue
		}
		if err := asp.Run(ctx, sess, store); err != nil {
			return nil, fmt.Errorf("silver aspect %s (%s): %w", asp.ID(), opts.Engine, err)
		}
		res.Aspects = append(res.Aspects, asp.ID())
	}
	if err := store.SetMeta(ctx, [2]string{extractdb.MetaAspectsApplied, strings.Join(res.Aspects, ",")}); err != nil {
		return nil, err
	}

	fnCount, err := store.FuncCount(ctx)
	if err != nil {
		return nil, err
	}
	res.FuncCount = int(fnCount)
	asmCount, err := store.AsmCount(ctx)
	if err != nil {
		return nil, err
	}
	res.AsmCount = int(asmCntFallback(asmCount))
	strCount, err := store.StringCount(ctx)
	if err != nil {
		return nil, err
	}
	res.StringCount = int(strCount)
	counts, err := store.StatusCounts(ctx)
	if err != nil {
		return nil, err
	}
	res.StatusCounts = counts
	res.Elapsed = time.Since(t0)
	return res, nil
}

type funcsAspect struct{}

func (funcsAspect) ID() string { return "funcs" }

func (funcsAspect) Run(ctx context.Context, sess *Session, store *extractdb.SilverStore) error {
	hasFuncs, err := store.HasFuncs(ctx)
	if err != nil {
		return err
	}
	hasAsm, err := store.HasAsm(ctx)
	if err != nil {
		return err
	}
	hasStrings, err := store.HasStrings(ctx)
	if err != nil {
		return err
	}
	if hasFuncs && hasAsm && hasStrings && !sess.Opts.Force {
		return nil
	}

	tmpBase := resolveTmpBase(sess.Opts.Paths, sess.Opts.TmpBaseDir)
	asmOutPath, cleanupAsm, err := createTempJSONLOutPath(tmpBase, "twdll_silver_asm_*.jsonl")
	if err != nil {
		return err
	}
	defer cleanupAsm()

	stringsOutPath, cleanupStrings, err := createTempJSONLOutPath(tmpBase, "twdll_silver_strings_*.jsonl")
	if err != nil {
		return err
	}
	defer cleanupStrings()

	if sess.Opts.OnPhase != nil {
		sess.Opts.OnPhase("funcs+asm+str")
	}
	rawFuncs, err := sess.exportFuncs(ctx, sess.Opts, sess.Enrichers, asmOutPath, stringsOutPath)
	if err != nil {
		return err
	}
	sess.Funcs = rawFuncs

	if sess.Opts.OnPhase != nil {
		sess.Opts.OnPhase("saving db")
	}
	funcRows, rangeRows := normalizeSilverFuncs(rawFuncs)
	if err := store.ReplaceFuncs(ctx, funcRows, rangeRows, "funcs"); err != nil {
		return err
	}
	sess.Result.FuncsRan = true

	if info, statErr := os.Stat(asmOutPath); statErr == nil && !info.IsDir() {
		if err := importSilverAsmJSONL(ctx, store, asmOutPath, "funcs"); err != nil {
			return err
		}
		asmCnt, err := store.AsmCount(ctx)
		if err != nil {
			return err
		}
		sess.Result.AsmRan = true
		sess.Result.AsmCount = int(asmCnt)
	}
	if info, statErr := os.Stat(stringsOutPath); statErr == nil && !info.IsDir() {
		if err := importSilverStringsJSONL(ctx, store, stringsOutPath, "funcs"); err != nil {
			return err
		}
		strCnt, err := store.StringCount(ctx)
		if err != nil {
			return err
		}
		sess.Result.StringsRan = true
		sess.Result.StringCount = int(strCnt)
	}
	return nil
}

func createTempJSONLOutPath(tmpBase, pattern string) (string, func(), error) {
	if tmpBase != "" {
		if err := os.MkdirAll(tmpBase, 0o755); err != nil {
			return "", nil, fmt.Errorf("ensure tmp base dir %s: %w", tmpBase, err)
		}
	}
	f, err := os.CreateTemp(tmpBase, pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create temp jsonl path: %w", err)
	}
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path)
	return path, func() { _ = os.Remove(path) }, nil
}

func asmCntFallback(v int64) int64 { return v }

func importSilverAsmJSONL(ctx context.Context, store *extractdb.SilverStore, path, aspectsKey string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open silver asm jsonl %s: %w", path, err)
	}
	defer f.Close()

	return store.ReplaceAsmStream(ctx, func(emit func(extractdb.SilverAsmRow) error) error {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 256*1024), 64*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var rec silverAsmJSONLRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				return fmt.Errorf("decode silver asm jsonl record: %w", err)
			}
			if rec.EA == 0 {
				continue
			}
			if err := emit(extractdb.SilverAsmRow{
				FuncEA:    uint64(rec.EA),
				InsnCount: rec.InsnCount,
				RawAsm:    NormalizeLineEndings(rec.RawAsm),
			}); err != nil {
				return err
			}
		}
		if err := sc.Err(); err != nil {
			return fmt.Errorf("scan silver asm jsonl %s: %w", path, err)
		}
		return nil
	}, aspectsKey)
}

func importSilverStringsJSONL(ctx context.Context, store *extractdb.SilverStore, path, aspectsKey string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open silver strings jsonl %s: %w", path, err)
	}
	defer f.Close()

	return store.ReplaceStringsStream(ctx, func(emit func(extractdb.SilverStringRow) error) error {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var rec silverStringJSONLRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				return fmt.Errorf("decode silver string jsonl record: %w", err)
			}
			if rec.EA == 0 || rec.Value == "" {
				continue
			}
			enc := strings.ToLower(strings.TrimSpace(rec.Encoding))
			if enc == "" {
				enc = extractdb.StringEncASCII
			}
			length := rec.Length
			if length <= 0 {
				length = len([]rune(rec.Value))
			}
			if err := emit(extractdb.SilverStringRow{
				EA:       uint64(rec.EA),
				Encoding: enc,
				Length:   length,
				Value:    NormalizeLineEndings(rec.Value),
				RefCount: rec.RefCount,
			}); err != nil {
				return err
			}
		}
		if err := sc.Err(); err != nil {
			return fmt.Errorf("scan silver strings jsonl %s: %w", path, err)
		}
		return nil
	}, aspectsKey)
}

type decompAspect struct{}

func (decompAspect) ID() string { return "decomp" }

func (decompAspect) Run(ctx context.Context, sess *Session, store *extractdb.SilverStore) error {
	var specs []extractdb.FuncSpec
	if sess.Opts.Selector != nil {
		// Load existing silver_funcs if funcs aspect didn't run this turn.
		candidates := sess.Funcs
		if len(candidates) == 0 {
			loaded, err := loadSilverFuncsFromStore(ctx, store)
			if err != nil {
				return err
			}
			candidates = loaded
		}
		selected := sess.Opts.Selector.Select(candidates)
		specs = make([]extractdb.FuncSpec, 0, len(selected))
		for _, f := range selected {
			specs = append(specs, extractdb.FuncSpec{EA: f.EA, Size: f.Size, Name: f.Name})
		}
	} else {
		var err error
		specs, err = store.ListFuncSpecs(ctx)
		if err != nil {
			return err
		}
	}

	pending, resume, err := store.PlanWork(ctx, specs, extractdb.PlanOptions{
		Force:         sess.Opts.Force,
		RetryFailed:   sess.Opts.RetryFailed,
		RetryTimeouts: sess.Opts.RetryTimeouts,
		PruneOrphans:  sess.Opts.PruneOrphans,
	})
	if err != nil {
		return err
	}
	sess.Result.Resume = resume

	if len(pending) == 0 {
		return nil
	}

	if sess.Opts.OnPhase != nil {
		sess.Opts.OnPhase("decomp")
	}
	cachedCount := max(0, resume.Total-len(pending))
	if sess.Opts.OnResume != nil && cachedCount > 0 {
		sess.Opts.OnResume(cachedCount, len(pending))
	}
	var totalBytes uint64
	for _, sp := range pending {
		totalBytes += sp.Size
	}
	if sess.Opts.OnBytes != nil && totalBytes > 0 {
		sess.Opts.OnBytes(0, totalBytes)
	}
	if sess.Opts.OnProgress != nil {
		sess.Opts.OnProgress(0, len(pending))
	}

	decompCtx, cancelDecomp := context.WithCancel(ctx)
	defer cancelDecomp()

	outCh := make(chan extractdb.DecompRecord, 4096)
	writerIn := (<-chan extractdb.DecompRecord)(outCh)
	var byteTapDone chan struct{}
	if sess.Opts.OnBytes != nil && totalBytes > 0 {
		tappedCh := make(chan extractdb.DecompRecord, 4096)
		byteTapDone = make(chan struct{})
		writerIn = tappedCh
		go func() {
			defer close(byteTapDone)
			defer close(tappedCh)
			var doneBytes uint64
			for rec := range outCh {
				doneBytes += rec.FuncSize
				sess.Opts.OnBytes(doneBytes, totalBytes)
				tappedCh <- rec
			}
		}()
	}

	type writerOut struct {
		written int
		err     error
	}
	writerDone := make(chan writerOut, 1)
	go func() {
		n, wErr := store.RunBatchWriter(ctx, writerIn, sess.Opts.BatchSize, 0)
		if wErr != nil {
			cancelDecomp()
		}
		writerDone <- writerOut{written: n, err: wErr}
	}()

	peakMem, runErr := sess.exportDecomp(decompCtx, sess.Opts, sess.Enrichers, pending, outCh)
	close(outCh)
	if byteTapDone != nil {
		<-byteTapDone
	}
	wRes := <-writerDone
	if wRes.err != nil {
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			return fmt.Errorf("%w (export: %v)", wRes.err, runErr)
		}
		return wRes.err
	}
	if runErr != nil {
		return runErr
	}

	sess.Result.DecompRan = true
	sess.Result.DecompWritten = wRes.written
	sess.Result.PeakJobMemBytes = peakMem
	return nil
}

func watchJSONLLineProgress(ctx context.Context, path, totalPath string, stop <-chan struct{}, onCount func(done, total int)) {
	if onCount == nil || path == "" {
		return
	}
	var offset int64
	var count int
	var total int
	buf := make([]byte, 64*1024)
	readTotal := func() int {
		if total > 0 || totalPath == "" {
			return total
		}
		raw, err := os.ReadFile(totalPath)
		if err != nil {
			return 0
		}
		if v, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && v > 0 {
			total = v
		}
		return total
	}
	drain := func() {
		prevTotal := total
		curTotal := readTotal()
		f, err := os.Open(path)
		if err != nil {
			if curTotal > 0 && curTotal != prevTotal {
				onCount(count, curTotal)
			}
			return
		}
		defer f.Close()
		if offset > 0 {
			if _, err := f.Seek(offset, 0); err != nil {
				return
			}
		}
		prev := count
		for {
			n, rErr := f.Read(buf)
			if n > 0 {
				offset += int64(n)
				for _, b := range buf[:n] {
					if b == '\n' {
						count++
					}
				}
			}
			if rErr != nil {
				break
			}
		}
		if count > prev || curTotal != prevTotal {
			onCount(count, curTotal)
		}
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			drain()
			return
		case <-ctx.Done():
			drain()
			return
		case <-ticker.C:
			drain()
		}
	}
}

func defaultExportFuncs(ctx context.Context, opts Options, enrichers []EnrichScript, asmOutPath, stringsOutPath string) ([]SilverFunc, error) {
	switch opts.Engine {
	case workspace.DecompilerGhidra:
		return runGhidraSilverExport(ctx, opts, enrichers, asmOutPath, stringsOutPath)
	case workspace.DecompilerIDA:
		return runIDASilverExport(ctx, opts, enrichers, asmOutPath, stringsOutPath)
	default:
		return nil, fmt.Errorf("unsupported engine %q", opts.Engine)
	}
}

func defaultExportDecomp(ctx context.Context, opts Options, enrichers []EnrichScript, pending []extractdb.FuncSpec, outCh chan<- extractdb.DecompRecord) (uint64, error) {
	switch opts.Engine {
	case workspace.DecompilerGhidra:
		return runGhidraDecompExport(ctx, opts, enrichers, pending, outCh)
	case workspace.DecompilerIDA:
		return runIDADecompExport(ctx, opts, enrichers, pending, outCh)
	default:
		return 0, fmt.Errorf("unsupported engine %q", opts.Engine)
	}
}

func normalizeSilverFuncs(raw []SilverFunc) ([]extractdb.SilverFuncRow, []extractdb.SilverFuncRangeRow) {
	byEA := make(map[uint64]SilverFunc, len(raw))
	for _, sf := range raw {
		if sf.EA == 0 {
			continue
		}
		if prev, exists := byEA[sf.EA]; exists {
			if len(sf.Ranges) > len(prev.Ranges) || sf.Size > prev.Size || (prev.Name == "" && sf.Name != "") {
				byEA[sf.EA] = sf
			}
			continue
		}
		byEA[sf.EA] = sf
	}

	eas := make([]uint64, 0, len(byEA))
	for ea := range byEA {
		eas = append(eas, ea)
	}
	sort.Slice(eas, func(i, j int) bool { return eas[i] < eas[j] })

	funcRows := make([]extractdb.SilverFuncRow, 0, len(eas))
	rangeRows := make([]extractdb.SilverFuncRangeRow, 0, len(eas))
	for _, ea := range eas {
		sf := byEA[ea]
		fnName := strings.TrimSpace(sf.Name)
		if fnName == "" {
			fnName = fmt.Sprintf("sub_%x", ea)
		}

		var validRanges []SilverRange
		for _, rg := range sf.Ranges {
			if rg.EndEA > rg.StartEA {
				validRanges = append(validRanges, rg)
			}
		}
		if len(validRanges) == 0 && sf.Size > 0 {
			validRanges = append(validRanges, SilverRange{StartEA: ea, EndEA: ea + sf.Size})
		}

		size := sf.Size
		if size == 0 && len(validRanges) > 0 {
			size = validRanges[0].EndEA - validRanges[0].StartEA
		}

		funcRows = append(funcRows, extractdb.SilverFuncRow{
			EA:            ea,
			Size:          size,
			Name:          fnName,
			Signature:     strings.TrimSpace(sf.Signature),
			CallingConv:   normalizeSilverCallingConv(sf.CallingConv),
			StackPurge:    sf.StackPurge,
			IsThunk:       sf.IsThunk,
			IsNoRet:       sf.IsNoRet,
			ThunkTargetEA: sf.ThunkTargetEA,
		})
		for _, rg := range validRanges {
			rangeRows = append(rangeRows, extractdb.SilverFuncRangeRow{
				StartEA: rg.StartEA,
				EndEA:   rg.EndEA,
				FuncEA:  ea,
			})
		}
	}
	return funcRows, rangeRows
}

func loadSilverFuncsFromStore(ctx context.Context, store *extractdb.SilverStore) ([]SilverFunc, error) {
	rows, err := store.ListFuncs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SilverFunc, 0, len(rows))
	for _, r := range rows {
		out = append(out, SilverFunc{
			EA:            r.EA,
			Size:          r.Size,
			Name:          r.Name,
			Signature:     r.Signature,
			CallingConv:   r.CallingConv,
			StackPurge:    r.StackPurge,
			IsThunk:       r.IsThunk,
			IsNoRet:       r.IsNoRet,
			ThunkTargetEA: r.ThunkTargetEA,
		})
	}
	return out, nil
}

func normalizeSilverCallingConv(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "__thiscall", "thiscall":
		return "__thiscall"
	case "__cdecl", "cdecl", "default":
		return "__cdecl"
	case "__stdcall", "stdcall":
		return "__stdcall"
	case "__fastcall", "fastcall":
		return "__fastcall"
	case "__usercall", "usercall":
		return "__usercall"
	case "__userpurge", "userpurge":
		return "__userpurge"
	case "sysv_amd64", "__amd64", "__sysv":
		return "sysv_amd64"
	default:
		return strings.TrimSpace(raw)
	}
}

type jsonlTailer struct {
	path    string
	offset  int64
	lineBuf strings.Builder
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
			t.lineBuf.Write(buf[:n])
			s := t.lineBuf.String()
			start := 0
			for {
				nl := strings.IndexByte(s[start:], '\n')
				if nl < 0 {
					break
				}
				rawLine := strings.TrimSpace(s[start : start+nl])
				start += nl + 1
				if rawLine == "" {
					continue
				}
				var frame decompFrameJSON
				if jErr := json.Unmarshal([]byte(rawLine), &frame); jErr != nil {
					return fmt.Errorf("decode decomp jsonl frame: %w", jErr)
				}
				onFrame(frame)
			}
			if start > 0 {
				var rem string
				if start < len(s) {
					rem = strings.Clone(s[start:])
				}
				t.lineBuf.Reset()
				if rem != "" {
					t.lineBuf.WriteString(rem)
				}
			}
		}
		if rErr != nil {
			break
		}
	}
	return nil
}
