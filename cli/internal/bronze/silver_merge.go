package bronze

import (
	"context"
	"database/sql"
	"debug/elf"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"twdll/cli/internal/extractdb"
	"twdll/cli/internal/workspace"
)

type SilverRange struct {
	StartEA uint64 `json:"start_ea"`
	EndEA   uint64 `json:"end_ea"`
}

type SilverFunc struct {
	EA            uint64        `json:"ea"`
	Size          uint64        `json:"size"`
	Ranges        []SilverRange `json:"ranges"`
	Name          string        `json:"name"`
	CallingConv   string        `json:"calling_conv"`
	StackPurge    int           `json:"stack_purge"`
	IsThunk       bool          `json:"is_thunk"`
	IsNoRet       bool          `json:"is_noret"`
	ThunkTargetEA uint64        `json:"thunk_target_ea"`
}

type SilverMergeOptions struct {
	Target       workspace.TargetID
	BinaryPath   string
	BinaryDBPath string
	ManifestPath string
	Engine       workspace.DecompilerID // "ghidra" or "ida"
	Funcs        []SilverFunc
}

type SilverMergeResult struct {
	TotalFunctions  int
	UpdatedExisting int
	InsertedNew     int
	Elapsed         time.Duration
}

type existingFuncRow struct {
	ea            uint64
	size          uint64
	name          string
	callingConv   string
	ccSource      string
	stackPurge    int
	discoveryMask uint32
	thunkTargetEA uint64
	flags         uint32
}

// ApplySilverTopology merges decompiler-recovered function boundaries, names,
// calling conventions, and multi-chunk ranges into an existing Bronze binary.sqlite
// atomically without clobbering Bronze ground truth (DWARF/vtable/lua_reg/x86asm).
func ApplySilverTopology(ctx context.Context, opts SilverMergeOptions) (*SilverMergeResult, error) {
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
	if !opts.Engine.IsValid() {
		return nil, fmt.Errorf("invalid silver engine %q", opts.Engine)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	builder, err := extractdb.NewAtomicBuilderFromExisting(opts.BinaryDBPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = builder.Abort() }()

	db := builder.DB()

	var binSHA string
	_ = db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, extractdb.MetaBinarySHA256).Scan(&binSHA)

	existingFuncs, err := loadExistingFunctions(ctx, db)
	if err != nil {
		return nil, err
	}
	strIdx, err := loadExistingStringIndex(ctx, db)
	if err != nil {
		return nil, err
	}

	var (
		peImg      *peImageInfo
		iatMapPE   map[uint32]importRow
		elfImg     *elfImageInfo
		pltGotByVA map[uint64]uint64
		roSections [][2]uint64
	)
	if opts.Target.Is64Bit() {
		ef, err := elf.Open(opts.BinaryPath)
		if err != nil {
			return nil, fmt.Errorf("open elf64 %s: %w", opts.BinaryPath, err)
		}
		elfImg = loadELF64ImageOnly(ef)
		_, _, iatSymByVA, relErr := parseAndInsertELFRelocsImportsExports(ctx, nil, ef)
		_ = ef.Close()
		if relErr != nil {
			return nil, relErr
		}
		pltGotByVA = scanELFPLTStubs(elfImg, iatSymByVA)
		for i := range elfImg.sections {
			s := &elfImg.sections[i]
			if s.name == ".rodata" || (s.flags&extractdb.SecFlagWrite) == 0 && s.vsize > 0 {
				roSections = append(roSections, [2]uint64{s.va, s.va + s.vsize})
			}
		}
	} else {
		peImg, _, _, iatMapPE, _, err = loadPE32Image(ctx, nil, opts.BinaryPath)
		if err != nil {
			return nil, err
		}
		for i := range peImg.sections {
			s := &peImg.sections[i]
			if s.name == ".rdata" || (s.flags&extractdb.SecFlagWrite) == 0 && s.vsize > 0 {
				roSections = append(roSections, [2]uint64{uint64(s.va), uint64(s.va) + uint64(s.vsize)})
			}
		}
	}

	inValidCode := func(ea uint64) bool {
		if opts.Target.Is64Bit() {
			return elfImg.inTextOrPLT(ea)
		}
		return peImg.inText(uint32(ea))
	}

	// Deduplicate and filter SilverFuncs to valid executable sections.
	silverByEA := make(map[uint64]SilverFunc, len(opts.Funcs))
	for _, sf := range opts.Funcs {
		if sf.EA == 0 || !inValidCode(sf.EA) {
			continue
		}
		if prev, exists := silverByEA[sf.EA]; exists {
			if len(sf.Ranges) > len(prev.Ranges) || (isDefaultFuncName(prev.Name) && !isDefaultFuncName(sf.Name)) || sf.Size > prev.Size {
				silverByEA[sf.EA] = sf
			}
			continue
		}
		silverByEA[sf.EA] = sf
	}

	// Collect sorted union of all known function entry points for boundary clipping.
	allStartsSet := make(map[uint64]struct{}, len(existingFuncs)+len(silverByEA))
	for ea := range existingFuncs {
		allStartsSet[ea] = struct{}{}
	}
	for ea := range silverByEA {
		allStartsSet[ea] = struct{}{}
	}
	allStarts := make([]uint64, 0, len(allStartsSet))
	for ea := range allStartsSet {
		allStarts = append(allStarts, ea)
	}
	sort.Slice(allStarts, func(i, j int) bool { return allStarts[i] < allStarts[j] })

	nextStartAfter := func(ea uint64) (uint64, bool) {
		idx := sort.Search(len(allStarts), func(i int) bool { return allStarts[i] > ea })
		if idx < len(allStarts) {
			return allStarts[idx], true
		}
		return 0, false
	}

	discBit := extractdb.DiscoveryGhidra
	silverCCSrc := extractdb.CCSourceGhidra
	if opts.Engine == workspace.DecompilerIDA {
		discBit = extractdb.DiscoveryIDA
		silverCCSrc = extractdb.CCSourceIDA
	}

	sortedSilverEAs := make([]uint64, 0, len(silverByEA))
	for ea := range silverByEA {
		sortedSilverEAs = append(sortedSilverEAs, ea)
	}
	sort.Slice(sortedSilverEAs, func(i, j int) bool { return sortedSilverEAs[i] < sortedSilverEAs[j] })

	type existingUpdate struct {
		row           functionRow
		updatePrimary bool
		primaryEndEA  uint64
		tailRanges    []funcRangeRow
	}

	var (
		updates       []existingUpdate
		newFuncRows   []functionRow
		newRangeRows  []funcRangeRow
		newCallSites  []callSiteRow
		newDataXrefs  []dataXrefRow
		newStrXrefs   []stringXrefRow
		helperCache32 = make(map[uint32]bool, 64)
	)

	normalizeRanges := func(sf SilverFunc, clipToNext bool) []SilverRange {
		var out []SilverRange
		for _, rg := range sf.Ranges {
			if rg.EndEA <= rg.StartEA || !inValidCode(rg.StartEA) {
				continue
			}
			endEA := rg.EndEA
			if clipToNext {
				if nxt, ok := nextStartAfter(rg.StartEA); ok && endEA > nxt {
					endEA = nxt
				}
			}
			if endEA > rg.StartEA {
				out = append(out, SilverRange{StartEA: rg.StartEA, EndEA: endEA})
			}
		}
		if len(out) == 0 && sf.Size > 0 {
			endEA := sf.EA + sf.Size
			if clipToNext {
				if nxt, ok := nextStartAfter(sf.EA); ok && endEA > nxt {
					endEA = nxt
				}
			}
			if endEA > sf.EA {
				out = append(out, SilverRange{StartEA: sf.EA, EndEA: endEA})
			}
		}
		return out
	}

	for _, ea := range sortedSilverEAs {
		sf := silverByEA[ea]
		normCC := normalizeSilverCallingConv(sf.CallingConv)

		if ex, ok := existingFuncs[ea]; ok {
			ranges := normalizeRanges(sf, !opts.Target.Is64Bit())
			primarySize := ex.size
			updatePrimary := false
			var primaryEndEA uint64
			var tailRanges []funcRangeRow

			for _, rg := range ranges {
				if rg.StartEA == ea {
					if !opts.Target.Is64Bit() {
						primarySize = rg.EndEA - rg.StartEA
						primaryEndEA = rg.EndEA
						updatePrimary = true
					}
				} else {
					if !opts.Target.Is64Bit() || (ex.discoveryMask&extractdb.DiscoveryDWARF) == 0 {
						tailRanges = append(tailRanges, funcRangeRow{
							StartEA: rg.StartEA,
							EndEA:   rg.EndEA,
							FuncEA:  ea,
						})
					}
				}
			}

			newName := ex.name
			if isDefaultFuncName(newName) && sf.Name != "" && !isDefaultFuncName(sf.Name) {
				newName = sf.Name
			}

			newFlags := ex.flags
			if sf.IsThunk {
				newFlags |= extractdb.FuncFlagThunk
			}
			if sf.IsNoRet {
				newFlags |= extractdb.FuncFlagNoReturn
			}

			newThunk := ex.thunkTargetEA
			if newThunk == 0 && sf.ThunkTargetEA != 0 {
				newThunk = sf.ThunkTargetEA
			}

			newCC := ex.callingConv
			newCCSrc := ex.ccSource
			newPurge := ex.stackPurge
			if (ex.flags & extractdb.FuncFlagNoSeedOverride) == 0 {
				if shouldAdoptSilverCC(ex.ccSource, ex.callingConv, normCC) {
					newCC = normCC
					newCCSrc = silverCCSrc
				}
				if !opts.Target.Is64Bit() && newPurge <= 0 && sf.StackPurge > 0 && sf.StackPurge <= 256 && sf.StackPurge%4 == 0 {
					newPurge = sf.StackPurge
				}
			}

			updates = append(updates, existingUpdate{
				row: functionRow{
					EA:            ea,
					Size:          primarySize,
					Name:          newName,
					CallingConv:   newCC,
					CCSource:      newCCSrc,
					StackPurge:    newPurge,
					DiscoveryMask: ex.discoveryMask | discBit,
					ThunkTargetEA: nullUint64NonZero(newThunk),
					Flags:         newFlags,
				},
				updatePrimary: updatePrimary,
				primaryEndEA:  primaryEndEA,
				tailRanges:    tailRanges,
			})
			continue
		}

		// Silver-only function (not discovered by Bronze).
		ranges := normalizeRanges(sf, true)
		if len(ranges) == 0 {
			if nxt, ok := nextStartAfter(ea); ok && nxt > ea && nxt-ea <= 64 {
				ranges = []SilverRange{{StartEA: ea, EndEA: nxt}}
			} else {
				ranges = []SilverRange{{StartEA: ea, EndEA: ea + 1}}
			}
		}

		var fnSize uint64
		for _, rg := range ranges {
			if rg.StartEA == ea {
				fnSize = rg.EndEA - rg.StartEA
				break
			}
		}
		if fnSize == 0 {
			fnSize = ranges[0].EndEA - ranges[0].StartEA
		}

		fnName := sf.Name
		if fnName == "" || isDefaultFuncName(fnName) {
			fnName = formatSubName(ea)
		}

		var flags uint32
		if sf.IsThunk {
			flags |= extractdb.FuncFlagThunk
		}
		if sf.IsNoRet {
			flags |= extractdb.FuncFlagNoReturn
		}

		var (
			cc          string
			ccSrc       string
			stackPurge  int
			thunkTarget = sf.ThunkTargetEA
		)

		if opts.Target.Is64Bit() {
			cc = "sysv_amd64"
			ccSrc = silverCCSrc
			if gotVA, ok := pltGotByVA[ea]; ok {
				flags |= extractdb.FuncFlagThunk | extractdb.FuncFlagExternal
				thunkTarget = gotVA
			}
			if elfImg.textSec != nil {
				tb := elfImg.textSec.data
				textVA := elfImg.textSec.va
				textEnd := textVA + uint64(len(tb))
				for _, rg := range ranges {
					if rg.StartEA < textVA || rg.StartEA >= textEnd {
						continue
					}
					endEA := rg.EndEA
					if endEA > textEnd {
						endEA = textEnd
					}
					startOff := int(rg.StartEA - textVA)
					endOff := int(endEA - textVA)
					if rg.StartEA == ea && endOff-startOff <= 16 {
						if isThunk, tgt := detectELF64Thunk(tb[startOff:endOff], rg.StartEA, elfImg); isThunk {
							flags |= extractdb.FuncFlagThunk
							if thunkTarget == 0 {
								thunkTarget = tgt
							}
							if _, isExt := pltGotByVA[tgt]; isExt {
								flags |= extractdb.FuncFlagExternal
							}
						}
					}
					var chunkOut elfWorkerChunkResult
					decodeELF64Range(tb, startOff, endOff, rg.StartEA, endEA, ea, elfImg, strIdx, &chunkOut)
					newCallSites = append(newCallSites, chunkOut.callSites...)
					for _, dx := range chunkOut.dataXrefs {
						newDataXrefs = append(newDataXrefs, clampReadOnlyDataXref(dx, roSections))
					}
					newStrXrefs = append(newStrXrefs, chunkOut.stringXrefs...)
				}
			}
		} else {
			tb := peImg.textSec.data
			textVA := peImg.textSec.va
			va32 := uint32(ea)
			startOff := int(va32 - textVA)

			isThunk, tTarget, thunkSz := detectPE32Thunk(tb, startOff, textVA, peImg)
			if isThunk {
				flags |= extractdb.FuncFlagThunk
				if thunkTarget == 0 {
					thunkTarget = tTarget
				}
				if fnSize == 0 || fnSize > uint64(thunkSz) {
					fnSize = uint64(thunkSz)
					if len(ranges) > 0 && ranges[0].StartEA == ea {
						ranges[0].EndEA = ea + fnSize
					}
				}
				if imp, ok := iatMapPE[uint32(thunkTarget)]; ok {
					flags |= extractdb.FuncFlagExternal
					if isDefaultFuncName(fnName) && imp.SymName != "" {
						fnName = "j_" + imp.SymName
					}
				}
				cc = "__cdecl"
				ccSrc = extractdb.CCSourceX86Asm
			} else {
				seed := ClassifyPE32FunctionAt(tb, textVA, peImg.textSec.rva, peImg, va32)
				flags |= seed.Flags

				var (
					readsECX         bool
					hasConsistentRet bool
					bodySize         uint32
				)
				for _, rg := range ranges {
					if !peImg.inText(uint32(rg.StartEA)) {
						continue
					}
					rStartOff := int(uint32(rg.StartEA) - textVA)
					rEndOff := int(uint32(rg.EndEA) - textVA)
					if rEndOff > len(tb) {
						rEndOff = len(tb)
					}
					if rEndOff <= rStartOff {
						continue
					}
					res := analyzePE32Body(tb, rStartOff, rEndOff, va32, peImg, strIdx, helperCache32)
					if rg.StartEA == ea {
						readsECX = res.readsECX
						hasConsistentRet = res.hasConsistentRet
						stackPurge = res.stackPurge
						bodySize = res.size
					}
					newCallSites = append(newCallSites, res.callSites...)
					for _, dx := range res.dataXrefs {
						newDataXrefs = append(newDataXrefs, clampReadOnlyDataXref(dx, roSections))
					}
					newStrXrefs = append(newStrXrefs, res.stringXrefs...)
				}
				if fnSize == 0 && bodySize > 0 {
					fnSize = uint64(bodySize)
				}
				if (flags & extractdb.FuncFlagNoSeedOverride) != 0 {
					cc = seed.CallingConv
					ccSrc = extractdb.CCSourceX86Asm
				} else if readsECX {
					cc = "__thiscall"
					ccSrc = extractdb.CCSourceX86Asm
				} else if hasConsistentRet && stackPurge > 0 {
					cc = "__stdcall"
					ccSrc = extractdb.CCSourceX86Asm
				} else if normCC == "__thiscall" || normCC == "__fastcall" || normCC == "__stdcall" {
					cc = normCC
					ccSrc = silverCCSrc
					if stackPurge <= 0 && sf.StackPurge > 0 && sf.StackPurge <= 256 && sf.StackPurge%4 == 0 {
						stackPurge = sf.StackPurge
					}
				} else {
					cc = "__cdecl"
					ccSrc = extractdb.CCSourceX86Asm
				}
			}
		}

		newFuncRows = append(newFuncRows, functionRow{
			EA:            ea,
			Size:          fnSize,
			Name:          fnName,
			CallingConv:   cc,
			CCSource:      ccSrc,
			StackPurge:    stackPurge,
			DiscoveryMask: discBit,
			ThunkTargetEA: nullUint64NonZero(thunkTarget),
			Flags:         flags,
		})
		for _, rg := range ranges {
			newRangeRows = append(newRangeRows, funcRangeRow{
				StartEA: rg.StartEA,
				EndEA:   rg.EndEA,
				FuncEA:  ea,
			})
		}
	}

	// On PE32, also clip any Bronze-only function whose primary span crosses a newly inserted Silver function start.
	if !opts.Target.Is64Bit() {
		bronzeOnlyEAs := make([]uint64, 0)
		for ea := range existingFuncs {
			if _, inSilver := silverByEA[ea]; !inSilver {
				bronzeOnlyEAs = append(bronzeOnlyEAs, ea)
			}
		}
		sort.Slice(bronzeOnlyEAs, func(i, j int) bool { return bronzeOnlyEAs[i] < bronzeOnlyEAs[j] })
		for _, ea := range bronzeOnlyEAs {
			ex := existingFuncs[ea]
			if nxt, ok := nextStartAfter(ea); ok && ex.size > 0 && ea+ex.size > nxt {
				clipped := nxt - ea
				updates = append(updates, existingUpdate{
					row: functionRow{
						EA:            ea,
						Size:          clipped,
						Name:          ex.name,
						CallingConv:   ex.callingConv,
						CCSource:      ex.ccSource,
						StackPurge:    ex.stackPurge,
						DiscoveryMask: ex.discoveryMask,
						ThunkTargetEA: nullUint64NonZero(ex.thunkTargetEA),
						Flags:         ex.flags,
					},
					updatePrimary: true,
					primaryEndEA:  nxt,
				})
			}
		}
	}

	// 1. Apply updates to existing Bronze functions.
	if len(updates) > 0 {
		updFuncBatch := newMultiStmtTxBatcher(ctx, db,
			`UPDATE functions SET size=?, name=?, calling_conv=?, cc_source=?, stack_purge=?, discovery_mask=?, thunk_target_ea=?, flags=? WHERE ea=?`,
			`INSERT INTO func_ranges(start_ea, end_ea, func_ea) VALUES (?, ?, ?) ON CONFLICT(start_ea) DO UPDATE SET end_ea=excluded.end_ea, func_ea=excluded.func_ea`,
			`INSERT OR IGNORE INTO func_ranges(start_ea, end_ea, func_ea) VALUES (?, ?, ?)`,
		)
		defer func() { _ = updFuncBatch.Close() }()

		for _, u := range updates {
			r := u.row
			if err := updFuncBatch.Exec(0, int64(r.Size), r.Name, r.CallingConv, r.CCSource, r.StackPurge, int64(r.DiscoveryMask), r.ThunkTargetEA, int64(r.Flags), int64(r.EA)); err != nil {
				return nil, fmt.Errorf("update function 0x%x: %w", r.EA, err)
			}
			if u.updatePrimary && u.primaryEndEA > r.EA {
				if err := updFuncBatch.Exec(1, int64(r.EA), int64(u.primaryEndEA), int64(r.EA)); err != nil {
					return nil, fmt.Errorf("update primary func_range 0x%x: %w", r.EA, err)
				}
			}
			for _, tr := range u.tailRanges {
				if err := updFuncBatch.Exec(2, int64(tr.StartEA), int64(tr.EndEA), int64(tr.FuncEA)); err != nil {
					return nil, fmt.Errorf("insert tail func_range [0x%x,0x%x): %w", tr.StartEA, tr.EndEA, err)
				}
			}
		}
		if err := updFuncBatch.Flush(); err != nil {
			return nil, err
		}
	}

	// 2. Insert Silver-only functions, ranges, call_sites, and data_xrefs.
	if err := insertFunctions(ctx, db, newFuncRows); err != nil {
		return nil, err
	}
	if err := insertFuncRanges(ctx, db, newRangeRows); err != nil {
		return nil, err
	}
	if err := insertCallSites(ctx, db, newCallSites); err != nil {
		return nil, err
	}
	if err := insertDataXrefs(ctx, db, newDataXrefs); err != nil {
		return nil, err
	}

	// 3. Re-attribute existing call_sites and data_xrefs covered by newly discovered Silver ranges
	// (including tail chunks added to existing functions).
	var reattrRanges []funcRangeRow
	reattrRanges = append(reattrRanges, newRangeRows...)
	for _, u := range updates {
		reattrRanges = append(reattrRanges, u.tailRanges...)
	}
	if len(reattrRanges) > 0 {
		reattrBatch := newMultiStmtTxBatcher(ctx, db,
			`UPDATE call_sites SET caller_ea = ? WHERE call_ea >= ? AND call_ea < ? AND caller_ea != ?`,
			`UPDATE data_xrefs SET func_ea = ? WHERE from_ea >= ? AND from_ea < ? AND func_ea != ?`,
		)
		defer func() { _ = reattrBatch.Close() }()
		for _, rg := range reattrRanges {
			if err := reattrBatch.Exec(0, int64(rg.FuncEA), int64(rg.StartEA), int64(rg.EndEA), int64(rg.FuncEA)); err != nil {
				return nil, fmt.Errorf("reattribute call_sites for 0x%x: %w", rg.FuncEA, err)
			}
			if err := reattrBatch.Exec(1, int64(rg.FuncEA), int64(rg.StartEA), int64(rg.EndEA), int64(rg.FuncEA)); err != nil {
				return nil, fmt.Errorf("reattribute data_xrefs for 0x%x: %w", rg.FuncEA, err)
			}
		}
		if err := reattrBatch.Flush(); err != nil {
			return nil, err
		}
	}

	// 4. Insert new string_xrefs and refresh df_count for touched strings.
	if err := insertSilverStringXrefsAndRefreshDF(ctx, db, newStrXrefs); err != nil {
		return nil, err
	}

	// 5. Refresh counts, meta, FTS5 indexes, and SQLite query planner stats.
	counts := make(map[string]int64)
	if err := populateTableCounts(ctx, db, counts); err != nil {
		return nil, err
	}
	silverAt := time.Now().UTC().Format(time.RFC3339)
	for _, kv := range [][2]string{
		{extractdb.MetaSilverEngine, string(opts.Engine)},
		{extractdb.MetaSilverExtractedAt, silverAt},
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			kv[0], kv[1],
		); err != nil {
			return nil, fmt.Errorf("update meta %s: %w", kv[0], err)
		}
	}
	if err := extractdb.RebuildFactsFTS(ctx, db); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `ANALYZE;`); err != nil {
		return nil, fmt.Errorf("analyze silver facts sqlite: %w", err)
	}
	if err := builder.Commit(ctx); err != nil {
		return nil, err
	}

	if opts.ManifestPath != "" {
		var m extractdb.TargetManifest
		if existingM, err := extractdb.ReadManifest(opts.ManifestPath); err == nil {
			m = existingM
		} else if !os.IsNotExist(err) {
			m = extractdb.TargetManifest{}
		}
		m.SchemaVersion = extractdb.ManifestSchemaVersion
		m.Target = string(opts.Target)
		if m.BinarySHA256 == "" {
			m.BinarySHA256 = binSHA
		}
		m.ExtractedAt = silverAt
		m.FactsSchemaVersion = extractdb.FactsSchemaVersion
		m.Sources = appendUniqueString(m.Sources, "silver_"+string(opts.Engine))
		m.Counts = make(map[string]int, len(counts))
		for k, v := range counts {
			m.Counts[k] = int(v)
		}
		if err := extractdb.WriteManifestAtomic(opts.ManifestPath, m); err != nil {
			return nil, err
		}
	}

	return &SilverMergeResult{
		TotalFunctions:  int(counts["functions"]),
		UpdatedExisting: len(updates),
		InsertedNew:     len(newFuncRows),
		Elapsed:         time.Since(t0),
	}, nil
}

func loadExistingFunctions(ctx context.Context, db *sql.DB) (map[uint64]*existingFuncRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT ea, size, name, calling_conv, cc_source, stack_purge, discovery_mask, thunk_target_ea, flags
		FROM functions
	`)
	if err != nil {
		return nil, fmt.Errorf("load existing functions: %w", err)
	}
	defer rows.Close()

	out := make(map[uint64]*existingFuncRow, 131072)
	for rows.Next() {
		var (
			ea, sz, mask, flags int64
			purge               int
			name, cc, ccSrc     string
			thunkEA             sql.NullInt64
		)
		if err := rows.Scan(&ea, &sz, &name, &cc, &ccSrc, &purge, &mask, &thunkEA, &flags); err != nil {
			return nil, fmt.Errorf("scan existing function: %w", err)
		}
		var tEA uint64
		if thunkEA.Valid {
			tEA = uint64(thunkEA.Int64)
		}
		uEA := uint64(ea)
		out[uEA] = &existingFuncRow{
			ea:            uEA,
			size:          uint64(sz),
			name:          name,
			callingConv:   cc,
			ccSource:      ccSrc,
			stackPurge:    purge,
			discoveryMask: uint32(mask),
			thunkTargetEA: tEA,
			flags:         uint32(flags),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing functions: %w", err)
	}
	return out, nil
}

func loadExistingStringIndex(ctx context.Context, db *sql.DB) (*stringIndex, error) {
	rows, err := db.QueryContext(ctx, `SELECT str_id, ea FROM strings ORDER BY ea ASC`)
	if err != nil {
		return nil, fmt.Errorf("load existing strings index: %w", err)
	}
	defer rows.Close()

	entries := make([]stringEntry, 0, 65536)
	for rows.Next() {
		var id, ea int64
		if err := rows.Scan(&id, &ea); err != nil {
			return nil, fmt.Errorf("scan string entry: %w", err)
		}
		entries = append(entries, stringEntry{strID: id, ea: uint64(ea)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate string entries: %w", err)
	}
	return &stringIndex{entries: entries}, nil
}

func loadELF64ImageOnly(ef *elf.File) *elfImageInfo {
	img, _ := loadELF64Image(ef)
	return img
}

func insertSilverStringXrefsAndRefreshDF(ctx context.Context, db *sql.DB, xrefs []stringXrefRow) error {
	if len(xrefs) == 0 {
		return nil
	}
	sort.Slice(xrefs, func(i, j int) bool {
		if xrefs[i].StrID != xrefs[j].StrID {
			return xrefs[i].StrID < xrefs[j].StrID
		}
		return xrefs[i].FuncEA < xrefs[j].FuncEA
	})

	ins := newTxStmtBatcher(ctx, db, `INSERT OR IGNORE INTO string_xrefs(str_id, func_ea) VALUES (?, ?)`)
	defer func() { _ = ins.Close() }()

	touched := make(map[int64]struct{})
	var prev stringXrefRow
	for i, xr := range xrefs {
		if i > 0 && xr.StrID == prev.StrID && xr.FuncEA == prev.FuncEA {
			continue
		}
		prev = xr
		if err := ins.Exec(xr.StrID, int64(xr.FuncEA)); err != nil {
			return fmt.Errorf("insert silver string_xref (%d, 0x%x): %w", xr.StrID, xr.FuncEA, err)
		}
		touched[xr.StrID] = struct{}{}
	}
	if err := ins.Flush(); err != nil {
		return err
	}

	upd := newTxStmtBatcher(ctx, db, `UPDATE strings SET df_count = (SELECT COUNT(*) FROM string_xrefs sx WHERE sx.str_id = strings.str_id) WHERE str_id = ?`)
	defer func() { _ = upd.Close() }()
	strIDs := make([]int64, 0, len(touched))
	for id := range touched {
		strIDs = append(strIDs, id)
	}
	sort.Slice(strIDs, func(i, j int) bool { return strIDs[i] < strIDs[j] })
	for _, id := range strIDs {
		if err := upd.Exec(id); err != nil {
			return fmt.Errorf("refresh df_count for string %d: %w", id, err)
		}
	}
	return upd.Flush()
}

func clampReadOnlyDataXref(dx dataXrefRow, roSections [][2]uint64) dataXrefRow {
	if dx.AccessKind == extractdb.AccessKindWrite {
		for _, r := range roSections {
			if dx.ToEA >= r[0] && dx.ToEA < r[1] {
				dx.AccessKind = extractdb.AccessKindRead
				break
			}
		}
	}
	return dx
}

func isDefaultFuncName(name string) bool {
	if name == "" {
		return true
	}
	return strings.HasPrefix(name, "sub_") ||
		strings.HasPrefix(name, "FUN_") ||
		strings.HasPrefix(name, "j_sub_") ||
		strings.HasPrefix(name, "thunk_FUN_") ||
		strings.HasPrefix(name, "nullsub_")
}

func normalizeSilverCallingConv(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "__")
	switch s {
	case "thiscall":
		return "__thiscall"
	case "stdcall":
		return "__stdcall"
	case "fastcall":
		return "__fastcall"
	case "cdecl":
		return "__cdecl"
	case "sysv_amd64", "amd64", "x86_64", "systemv":
		return "sysv_amd64"
	default:
		return ""
	}
}

func shouldAdoptSilverCC(curSrc, curCC, silverCC string) bool {
	if silverCC != "__thiscall" && silverCC != "__fastcall" && silverCC != "__stdcall" {
		return false
	}
	switch curSrc {
	case extractdb.CCSourceDWARF, extractdb.CCSourceVtable, extractdb.CCSourceLuaReg:
		return false
	case extractdb.CCSourceX86Asm:
		if curCC == "__thiscall" || curCC == "__stdcall" || curCC == "sysv_amd64" {
			return false
		}
	}
	return curCC == "" || curCC == "__cdecl"
}

func appendUniqueString(list []string, v string) []string {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}
