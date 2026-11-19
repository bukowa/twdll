package bronze

import (
	"context"
	"debug/dwarf"
	"encoding/binary"
	"fmt"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"twdll/cli/internal/extractdb"
)

// ELF64DWARF4Extractor extracts pure DWARF4 debug metadata (types, members, enums,
// subprograms, ranges, locals/params, inlines, and source paths) from an ELF64 binary.
type ELF64DWARF4Extractor struct{}

func (ELF64DWARF4Extractor) ID() string { return "elf64.dwarf4" }

func (ELF64DWARF4Extractor) AppliesTo(src *Source) bool {
	return src != nil && src.ELF != nil
}

func (ELF64DWARF4Extractor) Extract(ctx context.Context, src *Source, store *extractdb.BronzeStore) error {
	dw, err := src.ELF.DWARF()
	if err != nil || dw == nil {
		return nil
	}
	p1, err := runDWARFPass1(ctx, store, dw)
	if err != nil {
		return err
	}
	byteOrder := src.ELF.ByteOrder
	if byteOrder == nil {
		byteOrder = binary.LittleEndian
	}
	if err := runDWARFPass2(ctx, store, dw, p1, byteOrder); err != nil {
		return err
	}
	p1 = nil
	dw = nil
	runtime.GC()
	return nil
}

const (
	attrLinkageName     dwarf.Attr = 0x6e
	attrMIPSLinkageName dwarf.Attr = 0x2007
)

type rawTypeNode struct {
	offset               dwarf.Offset
	targetOff            dwarf.Offset
	byteSize             int64
	arrayCount           int64
	name                 string
	qualName             string
	kind                 string
	isDecl               bool
	isLocal              bool
	hasChildren          bool
	hasMemberOrEnumChild bool
}

type canonTypeInfo struct {
	id                   int64
	kind                 string
	name                 string
	byteSize             int64
	targetID             int64
	isDecl               bool
	rawIsDecl            bool
	hasMemberOrEnumChild bool
	defOffset            dwarf.Offset
}

type subprogSpec struct {
	name        string
	linkageName string
	className   string
	declFile    int
	declLine    int
	isVirtual   bool
	refOff      dwarf.Offset
	cuIdx       int
}

type varSpec struct {
	name    string
	typeOff dwarf.Offset
	refOff  dwarf.Offset
	cuIdx   int
}

type dwarfPass1State struct {
	offsetToCanonID map[dwarf.Offset]int64
	canonTypes      []canonTypeInfo // 1-indexed (index 0 unused)
	canonDefByOff   map[dwarf.Offset]int64
	subprogSpecs    map[dwarf.Offset]subprogSpec
	varSpecs        map[dwarf.Offset]varSpec
}

func (st *dwarfPass1State) formatDisplayType(canonID int64, depth int) string {
	if canonID <= 0 || int(canonID) >= len(st.canonTypes) || depth > 8 {
		return ""
	}
	ct := st.canonTypes[canonID]
	switch ct.kind {
	case extractdb.TypeKindPointer:
		inner := st.formatDisplayType(ct.targetID, depth+1)
		if inner == "" {
			inner = "void"
		}
		return inner + "*"
	case extractdb.TypeKindReference:
		inner := st.formatDisplayType(ct.targetID, depth+1)
		if inner == "" {
			inner = "void"
		}
		return inner + "&"
	case extractdb.TypeKindRValueRef:
		inner := st.formatDisplayType(ct.targetID, depth+1)
		if inner == "" {
			inner = "void"
		}
		return inner + "&&"
	case extractdb.TypeKindConst:
		inner := st.formatDisplayType(ct.targetID, depth+1)
		if inner == "" {
			return "const"
		}
		if ct.targetID > 0 && int(ct.targetID) < len(st.canonTypes) {
			tk := st.canonTypes[ct.targetID].kind
			if tk == extractdb.TypeKindPointer || tk == extractdb.TypeKindReference || tk == extractdb.TypeKindRValueRef {
				return inner + " const"
			}
		}
		return "const " + inner
	case extractdb.TypeKindVolatile:
		inner := st.formatDisplayType(ct.targetID, depth+1)
		if inner == "" {
			return "volatile"
		}
		if ct.targetID > 0 && int(ct.targetID) < len(st.canonTypes) {
			tk := st.canonTypes[ct.targetID].kind
			if tk == extractdb.TypeKindPointer || tk == extractdb.TypeKindReference || tk == extractdb.TypeKindRValueRef {
				return inner + " volatile"
			}
		}
		return "volatile " + inner
	case extractdb.TypeKindArray:
		inner := st.formatDisplayType(ct.targetID, depth+1)
		if ct.targetID > 0 && int(ct.targetID) < len(st.canonTypes) {
			elemSz := st.canonTypes[ct.targetID].byteSize
			if elemSz > 0 && ct.byteSize > 0 {
				return fmt.Sprintf("%s[%d]", inner, ct.byteSize/elemSz)
			}
		}
		return inner + "[]"
	default:
		if ct.name != "" {
			return ct.name
		}
		if ct.targetID > 0 {
			return st.formatDisplayType(ct.targetID, depth+1)
		}
		return ct.kind
	}
}

type pass1ScopeFrame struct {
	tag        dwarf.Tag
	qualName   string
	isFuncBody bool
	inAnonNS   bool
	arrayRawID int32
	compRawID  int32
}

func runDWARFPass1(ctx context.Context, store *extractdb.BronzeStore, dw *dwarf.Data) (*dwarfPass1State, error) {
	var (
		rawTypes     = make([]rawTypeNode, 1, 65536) // 1-indexed
		offToRawIdx  = make(map[dwarf.Offset]int32, 65536)
		subprogSpecs = make(map[dwarf.Offset]subprogSpec, 65536)
		varSpecs     = make(map[dwarf.Offset]varSpec, 65536)
		stack        []pass1ScopeFrame
		cuIdx        int
	)

	currentQualScope := func() (string, bool, bool) {
		var qual string
		var inFunc, inAnon bool
		for i := range stack {
			if stack[i].isFuncBody {
				inFunc = true
			}
			if stack[i].inAnonNS {
				inAnon = true
			}
			if stack[i].qualName != "" {
				qual = stack[i].qualName
			}
		}
		return qual, inFunc, inAnon
	}

	currentArrayRawID := func() int32 {
		if len(stack) == 0 {
			return 0
		}
		return stack[len(stack)-1].arrayRawID
	}

	r := dw.Reader()
	for {
		e, err := r.Next()
		if err != nil {
			return nil, fmt.Errorf("dwarf pass1 read: %w", err)
		}
		if e == nil {
			break
		}
		if e.Tag == 0 {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}

		if e.Tag == dwarf.TagCompileUnit {
			cuIdx++
		}

		// Mark immediate parent composite/enum frame when encountering member, inheritance, or enumerator.
		if e.Tag == dwarf.TagMember || e.Tag == dwarf.TagInheritance || e.Tag == dwarf.TagEnumerator {
			if len(stack) > 0 {
				if compID := stack[len(stack)-1].compRawID; compID > 0 {
					rawTypes[compID].hasMemberOrEnumChild = true
				}
			}
		}

		// Handle array subrange child.
		if e.Tag == dwarf.TagSubrangeType {
			if arrID := currentArrayRawID(); arrID > 0 {
				cnt := extractSubrangeCount(e)
				if cnt > 0 {
					if rawTypes[arrID].arrayCount <= 0 {
						rawTypes[arrID].arrayCount = cnt
					} else {
						rawTypes[arrID].arrayCount *= cnt
					}
				}
			}
		}

		parentQual, inFunc, inAnonNS := currentQualScope()
		rawName, _ := e.Val(dwarf.AttrName).(string)

		// Record subprogram specification metadata for cross-DIE lookup in Pass 2.
		if e.Tag == dwarf.TagSubprogram {
			linkName := extractLinkageName(e)
			declFile, _ := intValOK(e, dwarf.AttrDeclFile)
			declLine, _ := intValOK(e, dwarf.AttrDeclLine)
			virt, _ := intValOK(e, dwarf.AttrVirtuality)
			hasVtblLoc := e.AttrField(dwarf.AttrVtableElemLoc) != nil
			refOff := extractSpecOrOriginOff(e)
			if rawName != "" || linkName != "" || declFile > 0 || virt != 0 || hasVtblLoc || refOff != 0 {
				subprogSpecs[e.Offset] = subprogSpec{
					name:        rawName,
					linkageName: linkName,
					className:   parentQual,
					declFile:    int(declFile),
					declLine:    int(declLine),
					isVirtual:   virt != 0 || hasVtblLoc,
					refOff:      refOff,
					cuIdx:       cuIdx,
				}
			}
		} else if e.Tag == dwarf.TagFormalParameter || e.Tag == dwarf.TagVariable {
			typeOff, _ := e.Val(dwarf.AttrType).(dwarf.Offset)
			refOff := extractSpecOrOriginOff(e)
			if rawName != "" || typeOff != 0 || refOff != 0 {
				varSpecs[e.Offset] = varSpec{
					name:    rawName,
					typeOff: typeOff,
					refOff:  refOff,
					cuIdx:   cuIdx,
				}
			}
		}

		kind, isType := dwarfTagToTypeKind(e.Tag)
		var thisRawID int32
		var nextQualScope = parentQual
		var frameInAnonNS = inAnonNS
		if isType {
			qualName := rawName
			if rawName != "" && parentQual != "" && isScopedNamedTypeKind(kind) {
				qualName = parentQual + "::" + rawName
			}
			if isNamespaceOrAggregateTag(e.Tag) && rawName != "" && !inFunc {
				nextQualScope = qualName
			}
			targetOff, _ := e.Val(dwarf.AttrType).(dwarf.Offset)
			byteSz, _ := intValOK(e, dwarf.AttrByteSize)
			isDecl := boolVal(e, dwarf.AttrDeclaration)
			thisRawID = int32(len(rawTypes))
			rawTypes = append(rawTypes, rawTypeNode{
				offset:      e.Offset,
				targetOff:   targetOff,
				byteSize:    byteSz,
				name:        rawName,
				qualName:    qualName,
				kind:        kind,
				isDecl:      isDecl,
				isLocal:     inFunc || inAnonNS,
				hasChildren: e.Children,
			})
			offToRawIdx[e.Offset] = thisRawID
		} else if e.Tag == dwarf.TagNamespace && !inFunc {
			seg := rawName
			if seg == "" {
				seg = "(anonymous namespace)"
				frameInAnonNS = true
			}
			if parentQual != "" {
				nextQualScope = parentQual + "::" + seg
			} else {
				nextQualScope = seg
			}
		}

		if e.Children {
			isFunc := inFunc || e.Tag == dwarf.TagSubprogram
			var arrID, compID int32
			if e.Tag == dwarf.TagArrayType {
				arrID = thisRawID
			}
			switch e.Tag {
			case dwarf.TagClassType, dwarf.TagStructType, dwarf.TagUnionType, dwarf.TagEnumerationType:
				compID = thisRawID
			}
			stack = append(stack, pass1ScopeFrame{
				tag:        e.Tag,
				qualName:   nextQualScope,
				isFuncBody: isFunc,
				inAnonNS:   frameInAnonNS,
				arrayRawID: arrID,
				compRawID:  compID,
			})
		}
	}

	// Canonicalize and deduplicate all collected type nodes.
	numRaw := len(rawTypes) - 1
	rawToCanon := make([]int64, len(rawTypes))
	canonTypes := make([]canonTypeInfo, 1, numRaw/4+16) // 1-indexed
	canonDefByOff := make(map[dwarf.Offset]int64, 32768)

	type compositeKey struct {
		group string
		name  string
	}
	compositeByKey := make(map[compositeKey]int64, 32768)

	type emptyAnonKey struct {
		kind     string
		byteSize int64
	}
	emptyAnonByKey := make(map[emptyAnonKey]int64, 16)

	// Step 1: Named composite & enum types (class/struct share "composite" group).
	for id := 1; id <= numRaw; id++ {
		n := &rawTypes[id]
		grp, isComp := compositeKindGroup(n.kind)
		if !isComp {
			continue
		}
		isConcrete := !n.isDecl || n.hasMemberOrEnumChild
		if n.qualName == "" && !n.hasChildren && n.byteSize == 0 {
			ek := emptyAnonKey{kind: n.kind, byteSize: n.byteSize}
			cID, exists := emptyAnonByKey[ek]
			if !exists {
				cID = int64(len(canonTypes))
				emptyAnonByKey[ek] = cID
				canonTypes = append(canonTypes, canonTypeInfo{
					id:        cID,
					kind:      n.kind,
					name:      "",
					byteSize:  0,
					isDecl:    !isConcrete,
					rawIsDecl: n.isDecl,
				})
			} else if isConcrete && canonTypes[cID].isDecl {
				canonTypes[cID].isDecl = false
				canonTypes[cID].rawIsDecl = n.isDecl
			}
			rawToCanon[id] = cID
			continue
		}
		if n.qualName == "" || n.isLocal {
			// Non-empty anonymous or function-local composite/enum gets its own unique canonID.
			cID := int64(len(canonTypes))
			canonTypes = append(canonTypes, canonTypeInfo{
				id:                   cID,
				kind:                 n.kind,
				name:                 n.qualName,
				byteSize:             n.byteSize,
				isDecl:               !isConcrete,
				rawIsDecl:            n.isDecl,
				hasMemberOrEnumChild: n.hasMemberOrEnumChild,
				defOffset:            n.offset,
			})
			rawToCanon[id] = cID
			canonDefByOff[n.offset] = cID
			continue
		}

		key := compositeKey{group: grp, name: n.qualName}
		cID, exists := compositeByKey[key]
		if !exists {
			cID = int64(len(canonTypes))
			compositeByKey[key] = cID
			canonTypes = append(canonTypes, canonTypeInfo{
				id:                   cID,
				kind:                 n.kind,
				name:                 n.qualName,
				byteSize:             n.byteSize,
				isDecl:               !isConcrete,
				rawIsDecl:            n.isDecl,
				hasMemberOrEnumChild: n.hasMemberOrEnumChild,
				defOffset:            n.offset,
			})
		} else {
			ct := &canonTypes[cID]
			if isBetterCompositeCandidate(n, ct) {
				ct.kind = n.kind
				ct.byteSize = n.byteSize
				ct.isDecl = !isConcrete
				ct.rawIsDecl = n.isDecl
				ct.hasMemberOrEnumChild = n.hasMemberOrEnumChild
				ct.defOffset = n.offset
			}
		}
		rawToCanon[id] = cID
	}
	for cID := 1; cID < len(canonTypes); cID++ {
		ct := &canonTypes[cID]
		if ct.defOffset != 0 {
			canonDefByOff[ct.defOffset] = int64(cID)
		}
	}

	// Step 2: Base types deduplicated by (name, byteSize).
	type baseKey struct {
		name string
		size int64
	}
	baseByKey := make(map[baseKey]int64, 128)
	for id := 1; id <= numRaw; id++ {
		n := &rawTypes[id]
		if n.kind != extractdb.TypeKindBase {
			continue
		}
		bk := baseKey{name: n.name, size: n.byteSize}
		cID, exists := baseByKey[bk]
		if !exists {
			cID = int64(len(canonTypes))
			baseByKey[bk] = cID
			canonTypes = append(canonTypes, canonTypeInfo{
				id:       cID,
				kind:     extractdb.TypeKindBase,
				name:     n.name,
				byteSize: n.byteSize,
			})
		}
		rawToCanon[id] = cID
	}

	// Step 3: Wrapper/derived types via cycle-safe memoized resolution.
	type wrapperKey struct {
		kind     string
		name     string
		targetID int64
		byteSize int64
	}
	wrapperByKey := make(map[wrapperKey]int64, 65536)
	visitState := make([]uint8, len(rawTypes)) // 0=unvisited, 1=visiting, 2=done

	var resolveRaw func(id int32) int64
	resolveRaw = func(id int32) int64 {
		if id <= 0 || int(id) >= len(rawTypes) {
			return 0
		}
		if rawToCanon[id] != 0 {
			return rawToCanon[id]
		}
		if visitState[id] == 1 {
			return 0
		}
		visitState[id] = 1
		n := &rawTypes[id]
		var targetCanon int64
		if n.targetOff != 0 {
			if tRaw, ok := offToRawIdx[n.targetOff]; ok {
				targetCanon = resolveRaw(tRaw)
			}
		}
		byteSz := n.byteSize
		if n.kind == extractdb.TypeKindArray && byteSz == 0 && n.arrayCount > 0 && targetCanon > 0 {
			elemSz := canonTypes[targetCanon].byteSize
			if elemSz > 0 {
				byteSz = elemSz * n.arrayCount
			}
		} else if (n.kind == extractdb.TypeKindPointer || n.kind == extractdb.TypeKindReference || n.kind == extractdb.TypeKindRValueRef) && byteSz == 0 {
			byteSz = 8
		} else if (n.kind == extractdb.TypeKindTypedef || n.kind == extractdb.TypeKindConst || n.kind == extractdb.TypeKindVolatile) && byteSz == 0 && targetCanon > 0 {
			byteSz = canonTypes[targetCanon].byteSize
		}

		name := n.qualName
		if name == "" {
			name = n.name
		}
		wk := wrapperKey{
			kind:     n.kind,
			name:     name,
			targetID: targetCanon,
			byteSize: byteSz,
		}
		cID, exists := wrapperByKey[wk]
		if !exists {
			cID = int64(len(canonTypes))
			wrapperByKey[wk] = cID
			canonTypes = append(canonTypes, canonTypeInfo{
				id:       cID,
				kind:     n.kind,
				name:     name,
				byteSize: byteSz,
				targetID: targetCanon,
			})
		}
		rawToCanon[id] = cID
		visitState[id] = 2
		return cID
	}

	offsetToCanonID := make(map[dwarf.Offset]int64, numRaw)
	for id := int32(1); id <= int32(numRaw); id++ {
		cID := resolveRaw(id)
		offsetToCanonID[rawTypes[id].offset] = cID
	}

	// Stream canonical types into BronzeStore in bounded chunks.
	const typeChunkSize = 16384
	if len(canonTypes) > 1 {
		typeChunk := make([]extractdb.BronzeTypeRow, 0, min(len(canonTypes)-1, typeChunkSize))
		for cID := 1; cID < len(canonTypes); cID++ {
			ct := &canonTypes[cID]
			typeChunk = append(typeChunk, extractdb.BronzeTypeRow{
				TypeID:        ct.id,
				Kind:          ct.kind,
				Name:          ct.name,
				ByteSize:      ct.byteSize,
				TargetTypeID:  ct.targetID,
				IsDeclaration: ct.isDecl,
			})
			if len(typeChunk) >= typeChunkSize {
				if err := store.InsertDwarfTypes(ctx, typeChunk); err != nil {
					return nil, err
				}
				typeChunk = typeChunk[:0]
			}
		}
		if len(typeChunk) > 0 {
			if err := store.InsertDwarfTypes(ctx, typeChunk); err != nil {
				return nil, err
			}
		}
	}

	return &dwarfPass1State{
		offsetToCanonID: offsetToCanonID,
		canonTypes:      canonTypes,
		canonDefByOff:   canonDefByOff,
		subprogSpecs:    subprogSpecs,
		varSpecs:        varSpecs,
	}, nil
}

type cuLineRec struct {
	addr   uint64
	fileID int64
	line   int
}

type pass2StackFrame struct {
	tag          dwarf.Tag
	qualName     string
	compCanonID  int64
	isCanonDef   bool
	memberOrd    int
	enumOrd      int
	isActiveFunc bool
	isInline     bool
	inlineIdx    int
}

type elfFunctionAccum struct {
	row         extractdb.BronzeDwarfFuncRow
	ranges      []extractdb.BronzeDwarfFuncRangeRow
	validRanges [][2]uint64
	declFile    int64
	declLine    int
}

type funcState struct {
	ea           uint64
	varOrd       int
	inlIdx       int
	inlineDepth  int
	inlineRanges [][2]uint64
}

func runDWARFPass2(
	ctx context.Context,
	store *extractdb.BronzeStore,
	dw *dwarf.Data,
	p1 *dwarfPass1State,
	byteOrder binary.ByteOrder,
) error {
	w := store.NewDWARFPass2Writer(ctx)
	defer func() { _ = w.Close() }()

	fileIDByPath := make(map[string]int64, 16384)
	var nextFileID int64 = 1
	internSourceFile := func(compDir, rawPath string) (int64, error) {
		norm := normalizeDWARFPath(compDir, rawPath)
		if norm == "" {
			return 0, nil
		}
		if id, ok := fileIDByPath[norm]; ok {
			return id, nil
		}
		id := nextFileID
		nextFileID++
		base := path.Base(norm)
		if err := w.AddSourceFile(extractdb.BronzeSourceFileRow{
			FileID:   id,
			NormPath: norm,
			Basename: base,
		}); err != nil {
			return 0, err
		}
		fileIDByPath[norm] = id
		return id, nil
	}

	nameIDByVal := make(map[string]int64, 32768)
	var nextNameID int64 = 1
	internName := func(val string) (int64, error) {
		if val == "" {
			val = "<anonymous>"
		}
		if id, ok := nameIDByVal[val]; ok {
			return id, nil
		}
		id := nextNameID
		nextNameID++
		if err := w.AddName(extractdb.BronzeNameRow{
			NameID: id,
			Value:  val,
		}); err != nil {
			return 0, err
		}
		nameIDByVal[val] = id
		return id, nil
	}

	funcMap := make(map[uint64]*elfFunctionAccum, 65536)
	var (
		stack     []pass2StackFrame
		funcStack []funcState
		cuFileMap []int64
		cuLines   []cuLineRec
		curCUIdx  int
	)

	curFunc := func() *funcState {
		if len(funcStack) == 0 || funcStack[len(funcStack)-1].ea == 0 {
			return nil
		}
		return &funcStack[len(funcStack)-1]
	}

	currentCompositeFrame := func() *pass2StackFrame {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].compCanonID != 0 {
				return &stack[i]
			}
			if stack[i].isActiveFunc {
				return nil
			}
		}
		return nil
	}

	currentInlineParentIdx := func() int {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].isInline && stack[i].inlineIdx >= 0 {
				return stack[i].inlineIdx
			}
			if stack[i].isActiveFunc {
				break
			}
		}
		return -1
	}

	currentScopeQual := func() string {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].qualName != "" {
				return stack[i].qualName
			}
		}
		return ""
	}

	resolveCUFileID := func(idx int) int64 {
		if idx > 0 && idx < len(cuFileMap) && cuFileMap[idx] > 0 {
			return cuFileMap[idx]
		}
		return 0
	}

	r := dw.Reader()
	for {
		e, err := r.Next()
		if err != nil {
			return fmt.Errorf("dwarf pass2 read: %w", err)
		}
		if e == nil {
			break
		}
		if e.Tag == 0 {
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top.isActiveFunc {
					if len(funcStack) > 0 {
						popped := funcStack[len(funcStack)-1]
						funcStack = funcStack[:len(funcStack)-1]
						if popped.ea != 0 && len(popped.inlineRanges) > 0 {
							if acc, ok := funcMap[popped.ea]; ok {
								lowLine, highLine, lineFile := computeFuncLineSpan(cuLines, acc.validRanges, popped.inlineRanges, acc.declFile)
								acc.row.LowLine = lowLine
								acc.row.HighLine = highLine
								if lineFile > 0 && (acc.declFile <= 0 || lineFile != acc.declFile) {
									acc.row.DeclFileID = lineFile
									acc.row.DeclLine = 0
								} else {
									acc.row.DeclFileID = acc.declFile
									acc.row.DeclLine = acc.declLine
								}
							}
						}
					}
				} else if top.isInline {
					if cf := curFunc(); cf != nil {
						cf.inlineDepth--
					}
				}
			}
			continue
		}

		// Handle Compile Unit (.debug_line file table & line records).
		if e.Tag == dwarf.TagCompileUnit {
			curCUIdx++
			cuFileMap = cuFileMap[:0]
			cuLines = cuLines[:0]
			compDir, _ := e.Val(dwarf.AttrCompDir).(string)
			if lr, err := dw.LineReader(e); err == nil && lr != nil {
				files := lr.Files()
				cuFileMap = make([]int64, len(files))
				for idx, lf := range files {
					if lf == nil || lf.Name == "" {
						continue
					}
					fID, err := internSourceFile(compDir, lf.Name)
					if err != nil {
						return err
					}
					cuFileMap[idx] = fID
				}
				var le dwarf.LineEntry
				for {
					if err := lr.Next(&le); err != nil {
						break
					}
					if le.EndSequence || le.Address == 0 || le.Line <= 0 {
						continue
					}
					var fID int64
					if le.File != nil && le.File.Name != "" {
						if id, ok := fileIDByPath[normalizeDWARFPath(compDir, le.File.Name)]; ok {
							fID = id
						}
					}
					cuLines = append(cuLines, cuLineRec{
						addr:   le.Address,
						fileID: fID,
						line:   le.Line,
					})
				}
				sort.Slice(cuLines, func(i, j int) bool { return cuLines[i].addr < cuLines[j].addr })
			}
		}

		var activeFuncEA uint64
		if cf := curFunc(); cf != nil {
			activeFuncEA = cf.ea
		}

		rawName, _ := e.Val(dwarf.AttrName).(string)
		parentQual := currentScopeQual()
		nextQual := parentQual
		if e.Tag == dwarf.TagNamespace && len(funcStack) == 0 && activeFuncEA == 0 {
			seg := rawName
			if seg == "" {
				seg = "(anonymous namespace)"
			}
			if parentQual != "" {
				nextQual = parentQual + "::" + seg
			} else {
				nextQual = seg
			}
		} else if isNamespaceOrAggregateTag(e.Tag) && rawName != "" && len(funcStack) == 0 && activeFuncEA == 0 {
			if parentQual != "" {
				nextQual = parentQual + "::" + rawName
			} else {
				nextQual = rawName
			}
		}

		var (
			frameCompCanonID int64
			frameIsCanonDef  bool
			frameIsFunc      bool
			frameFuncEA      uint64
			frameIsInline    bool
			frameInlineIdx   = -1
		)

		switch e.Tag {
		case dwarf.TagClassType, dwarf.TagStructType, dwarf.TagUnionType, dwarf.TagEnumerationType:
			frameCompCanonID = p1.offsetToCanonID[e.Offset]
			if defCID, ok := p1.canonDefByOff[e.Offset]; ok && defCID == frameCompCanonID {
				frameIsCanonDef = true
			}

		case dwarf.TagMember:
			if cf := currentCompositeFrame(); cf != nil && cf.isCanonDef {
				ord := cf.memberOrd
				cf.memberOrd++
				if err := emitDWARFMember(w, cf.compCanonID, cf.tag, ord, e, p1, byteOrder); err != nil {
					return err
				}
			}

		case dwarf.TagInheritance:
			if cf := currentCompositeFrame(); cf != nil && cf.isCanonDef {
				ord := cf.memberOrd
				cf.memberOrd++
				if err := emitDWARFInheritance(w, cf.compCanonID, ord, e, p1, byteOrder); err != nil {
					return err
				}
			}

		case dwarf.TagEnumerator:
			if cf := currentCompositeFrame(); cf != nil && cf.isCanonDef {
				ord := cf.enumOrd
				cf.enumOrd++
				val, _ := intValOK(e, dwarf.AttrConstValue)
				if err := w.AddEnumValue(extractdb.BronzeEnumValueRow{
					TypeID:  cf.compCanonID,
					Ordinal: ord,
					Name:    rawName,
					Value:   val,
				}); err != nil {
					return err
				}
			}

		case dwarf.TagSubprogram:
			frameIsFunc = true
			if !boolVal(e, dwarf.AttrDeclaration) {
				ea, size, validRanges := extractSubprogramRanges(dw, e)
				if ea != 0 && size > 0 {
					spec := resolveSubprogSpec(e, p1.subprogSpecs, parentQual, curCUIdx)
					fnName := spec.name
					if spec.className != "" && fnName != "" && !strings.Contains(fnName, "::") {
						fnName = spec.className + "::" + fnName
					}
					if fnName == "" {
						if spec.linkageName != "" {
							fnName = spec.linkageName
						} else {
							fnName = fmt.Sprintf("sub_%x", ea)
						}
					}
					declFile := resolveCUFileID(spec.declFile)
					declLine := spec.declLine
					lowLine, highLine, lineFile := computeFuncLineSpan(cuLines, validRanges, nil, declFile)
					srcFile := declFile
					srcLine := declLine
					if lineFile > 0 && (declFile <= 0 || lineFile != declFile) {
						srcFile = lineFile
						srcLine = 0
					}

					if _, exists := funcMap[ea]; !exists {
						acc := &elfFunctionAccum{
							row: extractdb.BronzeDwarfFuncRow{
								EA:          ea,
								Size:        size,
								Name:        fnName,
								LinkageName: spec.linkageName,
								ClassName:   spec.className,
								DeclFileID:  srcFile,
								DeclLine:    srcLine,
								LowLine:     lowLine,
								HighLine:    highLine,
							},
							validRanges: validRanges,
							declFile:    declFile,
							declLine:    declLine,
						}
						for _, rg := range validRanges {
							acc.ranges = append(acc.ranges, extractdb.BronzeDwarfFuncRangeRow{
								StartEA: rg[0],
								EndEA:   rg[1],
								FuncEA:  ea,
							})
						}
						funcMap[ea] = acc
						frameFuncEA = ea
					}
				}
			}

		case dwarf.TagFormalParameter, dwarf.TagVariable:
			if cf := curFunc(); cf != nil && cf.inlineDepth == 0 {
				if e.Tag == dwarf.TagVariable && (boolVal(e, dwarf.AttrExternal) || boolVal(e, dwarf.AttrDeclaration)) {
					break
				}
				ord := cf.varOrd
				cf.varOrd++
				if err := emitDWARFFuncVar(w, cf.ea, ord, e, p1); err != nil {
					return err
				}
			}

		case dwarf.TagInlinedSubroutine:
			if cf := curFunc(); cf != nil {
				parentIdx := currentInlineParentIdx()
				inlDepth := cf.inlineDepth + 1
				if e.Children {
					cf.inlineDepth = inlDepth
					frameIsInline = true
				}

				calleeName := resolveInlinedCalleeName(e, p1.subprogSpecs, curCUIdx)
				calleeNameID, err := internName(calleeName)
				if err != nil {
					return err
				}
				callFileIdx, _ := intValOK(e, dwarf.AttrCallFile)
				callLine, _ := intValOK(e, dwarf.AttrCallLine)
				callFileID := resolveCUFileID(int(callFileIdx))

				_, _, inlRanges := extractSubprogramRanges(dw, e)
				cf.inlineRanges = append(cf.inlineRanges, inlRanges...)
				for _, rg := range inlRanges {
					idx := cf.inlIdx
					cf.inlIdx++
					if frameInlineIdx < 0 {
						frameInlineIdx = idx
					}
					if err := w.AddInline(extractdb.BronzeInlineRow{
						HostEA:       cf.ea,
						InlineIdx:    idx,
						ParentIdx:    parentIdx,
						StartEA:      rg[0],
						EndEA:        rg[1],
						Depth:        inlDepth,
						CalleeNameID: calleeNameID,
						CallFileID:   callFileID,
						CallLine:     int(callLine),
					}); err != nil {
						return err
					}
				}
			}
		}

		if e.Children {
			if frameIsFunc {
				funcStack = append(funcStack, funcState{ea: frameFuncEA})
			}
			stack = append(stack, pass2StackFrame{
				tag:          e.Tag,
				qualName:     nextQual,
				compCanonID:  frameCompCanonID,
				isCanonDef:   frameIsCanonDef,
				isActiveFunc: frameIsFunc,
				isInline:     frameIsInline,
				inlineIdx:    frameInlineIdx,
			})
		}
	}

	if err := w.Flush(); err != nil {
		return err
	}

	sortedEAs := make([]uint64, 0, len(funcMap))
	for ea := range funcMap {
		sortedEAs = append(sortedEAs, ea)
	}
	sort.Slice(sortedEAs, func(i, j int) bool { return sortedEAs[i] < sortedEAs[j] })

	funcRows := make([]extractdb.BronzeDwarfFuncRow, 0, len(sortedEAs))
	allRanges := make([]extractdb.BronzeDwarfFuncRangeRow, 0, len(sortedEAs))
	for _, ea := range sortedEAs {
		acc := funcMap[ea]
		funcRows = append(funcRows, acc.row)
		allRanges = append(allRanges, acc.ranges...)
	}

	sort.Slice(allRanges, func(i, j int) bool {
		if allRanges[i].StartEA != allRanges[j].StartEA {
			return allRanges[i].StartEA < allRanges[j].StartEA
		}
		iPrimary := allRanges[i].StartEA == allRanges[i].FuncEA
		jPrimary := allRanges[j].StartEA == allRanges[j].FuncEA
		if iPrimary != jPrimary {
			return iPrimary
		}
		return allRanges[i].EndEA > allRanges[j].EndEA
	})
	if len(allRanges) > 1 {
		dedup := allRanges[:1]
		for i := 1; i < len(allRanges); i++ {
			if allRanges[i].StartEA != dedup[len(dedup)-1].StartEA {
				dedup = append(dedup, allRanges[i])
			}
		}
		allRanges = dedup
	}

	if err := store.InsertDwarfFuncs(ctx, funcRows); err != nil {
		return err
	}
	return store.InsertDwarfFuncRanges(ctx, allRanges)
}

func emitDWARFMember(
	w *extractdb.BronzeDWARFPass2Writer,
	typeID int64,
	parentTag dwarf.Tag,
	ord int,
	e *dwarf.Entry,
	p1 *dwarfPass1State,
	byteOrder binary.ByteOrder,
) error {
	name, _ := e.Val(dwarf.AttrName).(string)
	var mTypeID int64
	if to, ok := e.Val(dwarf.AttrType).(dwarf.Offset); ok {
		mTypeID = p1.offsetToCanonID[to]
	}
	dispType := p1.formatDisplayType(mTypeID, 0)

	// Static member check: DW_AT_external or DW_AT_declaration on DW_TAG_member.
	if boolVal(e, dwarf.AttrExternal) || boolVal(e, dwarf.AttrDeclaration) {
		return w.AddMember(extractdb.BronzeTypeMemberRow{
			TypeID:       typeID,
			Ordinal:      ord,
			MemberKind:   extractdb.MemberKindStatic,
			Name:         name,
			MemberTypeID: mTypeID,
			DisplayType:  dispType,
		})
	}

	var memberTypeByteSize int64
	if mTypeID > 0 && int(mTypeID) < len(p1.canonTypes) {
		memberTypeByteSize = p1.canonTypes[mTypeID].byteSize
	}
	byteOff, hasByteOff, bitOff, bitSize := resolveDWARFMemberLayout(e, parentTag, memberTypeByteSize, byteOrder)

	return w.AddMember(extractdb.BronzeTypeMemberRow{
		TypeID:        typeID,
		Ordinal:       ord,
		MemberKind:    extractdb.MemberKindField,
		ByteOffset:    byteOff,
		HasByteOffset: hasByteOff,
		BitOffset:     bitOff,
		BitSize:       bitSize,
		Name:          name,
		MemberTypeID:  mTypeID,
		DisplayType:   dispType,
	})
}

// resolveDWARFMemberLayout computes normalized (byte_offset, bit_offset, bit_size)
// for a non-static DW_TAG_member using the DWARF4 LSB convention:
//   - byte_offset is the byte offset from the start of the enclosing composite (NULL when unknown/unparsed).
//   - bit_offset is the 0..7 LSB bit offset within byte_offset (0 for non-bitfields).
//   - bit_size is the bitfield width in bits (0 for non-bitfields).
func resolveDWARFMemberLayout(
	e *dwarf.Entry,
	parentTag dwarf.Tag,
	memberTypeByteSize int64,
	byteOrder binary.ByteOrder,
) (byteOff int64, hasByteOff bool, bitOff int, bitSize int) {
	if byteOrder == nil {
		byteOrder = binary.LittleEndian
	}
	byteOff, hasByteOff, complexExpr := parseDWARFDataMemberLoc(e, byteOrder)
	if !hasByteOff && complexExpr == "" && parentTag == dwarf.TagUnionType {
		// In DWARF, union members omit DW_AT_data_member_location because all members start at byte 0.
		byteOff = 0
		hasByteOff = true
	}

	rawBitSize, hasBitSize := intValOK(e, dwarf.AttrBitSize)
	if !hasBitSize || rawBitSize < 0 {
		rawBitSize = 0
	}

	// DWARF4 DW_AT_data_bit_offset is measured in bits from the start of the enclosing object.
	if dbo, ok := intValOK(e, dwarf.AttrDataBitOffset); ok && dbo >= 0 {
		byteOff = dbo / 8
		hasByteOff = true
		bitOff = int(dbo % 8)
		bitSize = int(rawBitSize)
		return byteOff, hasByteOff, bitOff, bitSize
	}

	// Legacy DWARF2/3 DW_AT_bit_offset is measured from the MSB of the storage unit to the MSB of the field.
	if bo, ok := intValOK(e, dwarf.AttrBitOffset); ok {
		attrByteSize, hasAttrByteSize := intValOK(e, dwarf.AttrByteSize)
		// GCC 4.x emits DW_AT_byte_size=0, DW_AT_bit_size=N*8, DW_AT_bit_offset=-N*8 on some
		// non-bitfield fields (e.g. aligned structs, pointer-to-member, decltype(nullptr)).
		if hasAttrByteSize && attrByteSize == 0 && rawBitSize > 0 && bo == -rawBitSize && rawBitSize%8 == 0 {
			return byteOff, hasByteOff, 0, 0
		}

		storageBytes := attrByteSize
		if !hasAttrByteSize || storageBytes <= 0 {
			storageBytes = memberTypeByteSize
		}

		var lsb int64 = -1
		if byteOrder == binary.BigEndian {
			if bo >= 0 {
				lsb = bo
			}
		} else if storageBytes > 0 && rawBitSize > 0 {
			lsb = storageBytes*8 - bo - rawBitSize
		}

		if lsb >= 0 {
			if hasByteOff {
				byteOff += lsb / 8
				bitOff = int(lsb % 8)
			} else {
				bitOff = int(lsb)
			}
		}
		bitSize = int(rawBitSize)
		return byteOff, hasByteOff, bitOff, bitSize
	}

	if rawBitSize > 0 {
		bitSize = int(rawBitSize)
	}
	return byteOff, hasByteOff, bitOff, bitSize
}

func emitDWARFInheritance(
	w *extractdb.BronzeDWARFPass2Writer,
	typeID int64,
	ord int,
	e *dwarf.Entry,
	p1 *dwarfPass1State,
	byteOrder binary.ByteOrder,
) error {
	var mTypeID int64
	if to, ok := e.Val(dwarf.AttrType).(dwarf.Offset); ok {
		mTypeID = p1.offsetToCanonID[to]
	}
	dispType := p1.formatDisplayType(mTypeID, 0)
	baseName := dispType

	virt, _ := intValOK(e, dwarf.AttrVirtuality)
	byteOff, hasConstOff, exprSummary := parseDWARFDataMemberLoc(e, byteOrder)
	isVirtual := virt != 0 || exprSummary != ""
	if isVirtual {
		if exprSummary == "" {
			exprSummary = "virtual"
		}
		return w.AddMember(extractdb.BronzeTypeMemberRow{
			TypeID:        typeID,
			Ordinal:       ord,
			MemberKind:    extractdb.MemberKindBase,
			Name:          baseName,
			MemberTypeID:  mTypeID,
			DisplayType:   dispType,
			IsVirtualBase: true,
			VBaseExpr:     exprSummary,
		})
	}

	if !hasConstOff {
		byteOff = 0
	}
	return w.AddMember(extractdb.BronzeTypeMemberRow{
		TypeID:        typeID,
		Ordinal:       ord,
		MemberKind:    extractdb.MemberKindBase,
		ByteOffset:    byteOff,
		HasByteOffset: true,
		Name:          baseName,
		MemberTypeID:  mTypeID,
		DisplayType:   dispType,
	})
}

func emitDWARFFuncVar(w *extractdb.BronzeDWARFPass2Writer, funcEA uint64, ord int, e *dwarf.Entry, p1 *dwarfPass1State) error {
	kind := extractdb.VarKindLocal
	if e.Tag == dwarf.TagFormalParameter {
		kind = extractdb.VarKindParam
	}
	name, _ := e.Val(dwarf.AttrName).(string)
	typeOff, _ := e.Val(dwarf.AttrType).(dwarf.Offset)

	// Follow DW_AT_abstract_origin or DW_AT_specification fallback for name & type.
	refOff := extractSpecOrOriginOff(e)
	for hops := 0; hops < 4 && refOff != 0 && (name == "" || typeOff == 0); hops++ {
		vs, exists := p1.varSpecs[refOff]
		if !exists {
			break
		}
		if name == "" {
			name = vs.name
		}
		if typeOff == 0 {
			typeOff = vs.typeOff
		}
		refOff = vs.refOff
	}
	if name == "" {
		if kind == extractdb.VarKindParam {
			name = fmt.Sprintf("arg_%d", ord)
		} else {
			name = fmt.Sprintf("var_%d", ord)
		}
	}

	var typeID int64
	if typeOff != 0 {
		typeID = p1.offsetToCanonID[typeOff]
	}
	dispType := p1.formatDisplayType(typeID, 0)

	constVal := formatAttrConstVal(e.AttrField(dwarf.AttrConstValue))
	locSummary := summarizeDWARFLocField(e.AttrField(dwarf.AttrLocation))
	isOptOut := constVal == "" && locSummary == ""

	return w.AddFuncVar(extractdb.BronzeFuncVarRow{
		FuncEA:         funcEA,
		Ordinal:        ord,
		Kind:           kind,
		Name:           name,
		TypeID:         typeID,
		DisplayType:    dispType,
		ConstVal:       constVal,
		LocSummary:     locSummary,
		IsOptimizedOut: isOptOut,
	})
}

func parseDWARFDataMemberLoc(e *dwarf.Entry, byteOrder binary.ByteOrder) (offset int64, hasConst bool, complexExpr string) {
	if byteOrder == nil {
		byteOrder = binary.LittleEndian
	}
	f := e.AttrField(dwarf.AttrDataMemberLoc)
	if f == nil {
		return 0, false, ""
	}
	switch v := f.Val.(type) {
	case int64:
		return v, true, ""
	case uint64:
		return int64(v), true, ""
	case []byte:
		if len(v) == 0 {
			return 0, true, ""
		}
		// DW_OP_plus_uconst (0x23 <uleb128>)
		if v[0] == 0x23 {
			uval, n := binary.Uvarint(v[1:])
			if n > 0 && 1+n == len(v) {
				return int64(uval), true, ""
			}
		}
		// DW_OP_lit0..31 (0x30..0x4f) + DW_OP_plus (0x22)
		if len(v) == 2 && v[0] >= 0x30 && v[0] <= 0x4F && v[1] == 0x22 {
			return int64(v[0] - 0x30), true, ""
		}
		// DW_OP_const1u (0x08 <u8>) + DW_OP_plus (0x22)
		if len(v) == 3 && v[0] == 0x08 && v[2] == 0x22 {
			return int64(v[1]), true, ""
		}
		// DW_OP_const2u (0x0a <u16>) + DW_OP_plus (0x22)
		if len(v) == 4 && v[0] == 0x0a && v[3] == 0x22 {
			return int64(byteOrder.Uint16(v[1:3])), true, ""
		}
		// DW_OP_const4u (0x0c <u32>) + DW_OP_plus (0x22)
		if len(v) == 6 && v[0] == 0x0c && v[5] == 0x22 {
			return int64(byteOrder.Uint32(v[1:5])), true, ""
		}
		// DW_OP_constu (0x10 <uleb128>) + DW_OP_plus (0x22)
		if len(v) >= 3 && v[0] == 0x10 && v[len(v)-1] == 0x22 {
			uval, n := binary.Uvarint(v[1 : len(v)-1])
			if n > 0 && 1+n == len(v)-1 {
				return int64(uval), true, ""
			}
		}
		return 0, false, fmt.Sprintf("expr:%x", v)
	default:
		return 0, false, ""
	}
}

func formatAttrConstVal(f *dwarf.Field) string {
	if f == nil {
		return ""
	}
	switch v := f.Val.(type) {
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return fmt.Sprintf("0x%x", v)
	case string:
		return v
	case []byte:
		return fmt.Sprintf("0x%x", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func decodeSLEB128(b []byte) (int64, int) {
	var val uint64
	var shift uint
	for i, c := range b {
		val |= uint64(c&0x7f) << shift
		shift += 7
		if c&0x80 == 0 {
			if shift < 64 && (c&0x40) != 0 {
				val |= ^uint64(0) << shift
			}
			return int64(val), i + 1
		}
		if shift >= 64 {
			return 0, 0
		}
	}
	return 0, 0
}

func summarizeDWARFLocField(f *dwarf.Field) string {
	if f == nil {
		return ""
	}
	switch v := f.Val.(type) {
	case int64:
		return fmt.Sprintf("loclist:0x%x", v)
	case []byte:
		if len(v) == 0 {
			return ""
		}
		op := v[0]
		switch {
		case op >= 0x50 && op <= 0x6F:
			return fmt.Sprintf("reg%d", op-0x50)
		case op >= 0x70 && op <= 0x8F:
			if sval, n := decodeSLEB128(v[1:]); n > 0 && 1+n == len(v) {
				return fmt.Sprintf("breg%d%+d", op-0x70, sval)
			}
			return fmt.Sprintf("expr:%x", v)
		case op == 0x91: // DW_OP_fbreg
			if sval, n := decodeSLEB128(v[1:]); n > 0 && 1+n == len(v) {
				return fmt.Sprintf("fbreg%+d", sval)
			}
			return fmt.Sprintf("expr:%x", v)
		case op == 0x03 && len(v) >= 9: // DW_OP_addr (64-bit)
			addr := binary.LittleEndian.Uint64(v[1:9])
			return fmt.Sprintf("addr:0x%x", addr)
		default:
			return fmt.Sprintf("expr:%x", v)
		}
	default:
		return "loc"
	}
}

func extractSubprogramRanges(dw *dwarf.Data, e *dwarf.Entry) (primaryEA uint64, totalSize uint64, validRanges [][2]uint64) {
	rgs, err := dw.Ranges(e)
	if err == nil && len(rgs) > 0 {
		for _, rg := range rgs {
			if rg[0] != 0 && rg[1] > rg[0] {
				validRanges = append(validRanges, rg)
				totalSize += rg[1] - rg[0]
			}
		}
	}
	lowPC, hasLow := e.Val(dwarf.AttrLowpc).(uint64)
	if hasLow && lowPC != 0 {
		primaryEA = lowPC
		if len(validRanges) == 0 {
			var highPC uint64
			switch hv := e.Val(dwarf.AttrHighpc).(type) {
			case uint64:
				highPC = hv
			case int64:
				highPC = lowPC + uint64(hv)
			}
			if highPC > lowPC {
				validRanges = append(validRanges, [2]uint64{lowPC, highPC})
				totalSize = highPC - lowPC
			}
		}
	} else if len(validRanges) > 0 {
		primaryEA = validRanges[0][0]
	}
	return primaryEA, totalSize, validRanges
}

func isBetterCompositeCandidate(n *rawTypeNode, ct *canonTypeInfo) bool {
	if !n.isDecl != !ct.rawIsDecl {
		return !n.isDecl
	}
	if n.hasMemberOrEnumChild != ct.hasMemberOrEnumChild {
		return n.hasMemberOrEnumChild
	}
	return n.byteSize > ct.byteSize
}

func extractSpecOrOriginOff(e *dwarf.Entry) dwarf.Offset {
	if sp, ok := e.Val(dwarf.AttrSpecification).(dwarf.Offset); ok && sp != 0 {
		return sp
	}
	if ao, ok := e.Val(dwarf.AttrAbstractOrigin).(dwarf.Offset); ok && ao != 0 {
		return ao
	}
	return 0
}

func resolveSubprogSpec(e *dwarf.Entry, specs map[dwarf.Offset]subprogSpec, parentQual string, curCUIdx int) subprogSpec {
	name, _ := e.Val(dwarf.AttrName).(string)
	linkName := extractLinkageName(e)
	declFile, _ := intValOK(e, dwarf.AttrDeclFile)
	declLine, _ := intValOK(e, dwarf.AttrDeclLine)
	virt, _ := intValOK(e, dwarf.AttrVirtuality)
	hasVtblLoc := e.AttrField(dwarf.AttrVtableElemLoc) != nil
	res := subprogSpec{
		name:        name,
		linkageName: linkName,
		className:   parentQual,
		declFile:    int(declFile),
		declLine:    int(declLine),
		isVirtual:   virt != 0 || hasVtblLoc,
		cuIdx:       curCUIdx,
	}

	curOff := extractSpecOrOriginOff(e)
	for hops := 0; hops < 4 && curOff != 0; hops++ {
		sp, ok := specs[curOff]
		if !ok {
			break
		}
		if res.name == "" {
			res.name = sp.name
		}
		if res.linkageName == "" {
			res.linkageName = sp.linkageName
		}
		if res.className == "" || (sp.className != "" && strings.HasPrefix(sp.className, res.className+"::")) {
			res.className = sp.className
		}
		if curCUIdx == 0 || sp.cuIdx == curCUIdx {
			if res.declFile == 0 && sp.declFile > 0 {
				res.declFile = sp.declFile
			}
			if res.declLine == 0 && sp.declLine > 0 {
				res.declLine = sp.declLine
			}
		}
		if sp.isVirtual {
			res.isVirtual = true
		}
		curOff = sp.refOff
	}
	return res
}

func resolveInlinedCalleeName(e *dwarf.Entry, specs map[dwarf.Offset]subprogSpec, curCUIdx int) string {
	sp := resolveSubprogSpec(e, specs, "", curCUIdx)
	if sp.className != "" && sp.name != "" && !strings.Contains(sp.name, "::") {
		return sp.className + "::" + sp.name
	}
	if sp.name != "" {
		return sp.name
	}
	if sp.linkageName != "" {
		return sp.linkageName
	}
	return "<inlined>"
}

func isAddrInRanges(addr uint64, ranges [][2]uint64) bool {
	for _, rg := range ranges {
		if addr >= rg[0] && addr < rg[1] {
			return true
		}
	}
	return false
}

func computeFuncLineSpan(
	cuLines []cuLineRec,
	ranges [][2]uint64,
	inlineRanges [][2]uint64,
	declFile int64,
) (lowLine, highLine int, fallbackFile int64) {
	if len(cuLines) == 0 || len(ranges) == 0 {
		return 0, 0, 0
	}
	type fileSpan struct {
		fileID int64
		low    int
		high   int
		count  int
	}
	var spans []fileSpan
	addHit := func(fileID int64, line int) {
		for i := range spans {
			if spans[i].fileID == fileID {
				spans[i].count++
				if spans[i].low == 0 || line < spans[i].low {
					spans[i].low = line
				}
				if line > spans[i].high {
					spans[i].high = line
				}
				return
			}
		}
		spans = append(spans, fileSpan{
			fileID: fileID,
			low:    line,
			high:   line,
			count:  1,
		})
	}

	for _, rg := range ranges {
		idx := sort.Search(len(cuLines), func(i int) bool {
			return cuLines[i].addr >= rg[0]
		})
		for i := idx; i < len(cuLines) && cuLines[i].addr < rg[1]; i++ {
			rec := cuLines[i]
			if len(inlineRanges) > 0 && isAddrInRanges(rec.addr, inlineRanges) {
				continue
			}
			addHit(rec.fileID, rec.line)
		}
	}
	if declFile > 0 {
		for _, s := range spans {
			if s.fileID == declFile && s.count > 0 {
				return s.low, s.high, declFile
			}
		}
	}
	var best fileSpan
	for _, s := range spans {
		if s.count > best.count || (s.count == best.count && best.fileID == 0 && s.fileID > 0) {
			best = s
		}
	}
	if best.count > 0 {
		return best.low, best.high, best.fileID
	}
	if len(inlineRanges) > 0 {
		return computeFuncLineSpan(cuLines, ranges, nil, declFile)
	}
	return 0, 0, 0
}

func normalizeDWARFPath(compDir, filePath string) string {
	p := strings.ReplaceAll(strings.TrimSpace(filePath), "\\", "/")
	if p == "" {
		return ""
	}
	c := strings.ReplaceAll(strings.TrimSpace(compDir), "\\", "/")
	var joined string
	if strings.HasPrefix(p, "/") || (len(p) >= 2 && p[1] == ':') || c == "" {
		joined = path.Clean(p)
	} else {
		joined = path.Clean(c + "/" + p)
	}
	return joined
}

func extractLinkageName(e *dwarf.Entry) string {
	if s, ok := e.Val(attrLinkageName).(string); ok && s != "" {
		return s
	}
	if s, ok := e.Val(attrMIPSLinkageName).(string); ok && s != "" {
		return s
	}
	return ""
}

func extractSubrangeCount(e *dwarf.Entry) int64 {
	if cnt, ok := intValOK(e, dwarf.AttrCount); ok && cnt > 0 && cnt < (1<<30) {
		return cnt
	}
	if ub, ok := intValOK(e, dwarf.AttrUpperBound); ok && ub >= 0 && ub < (1<<30) {
		return ub + 1
	}
	return 0
}

func dwarfTagToTypeKind(tag dwarf.Tag) (string, bool) {
	switch tag {
	case dwarf.TagClassType:
		return extractdb.TypeKindClass, true
	case dwarf.TagStructType:
		return extractdb.TypeKindStruct, true
	case dwarf.TagUnionType:
		return extractdb.TypeKindUnion, true
	case dwarf.TagEnumerationType:
		return extractdb.TypeKindEnum, true
	case dwarf.TagTypedef:
		return extractdb.TypeKindTypedef, true
	case dwarf.TagPointerType:
		return extractdb.TypeKindPointer, true
	case dwarf.TagReferenceType:
		return extractdb.TypeKindReference, true
	case dwarf.TagRvalueReferenceType:
		return extractdb.TypeKindRValueRef, true
	case dwarf.TagConstType:
		return extractdb.TypeKindConst, true
	case dwarf.TagVolatileType:
		return extractdb.TypeKindVolatile, true
	case dwarf.TagArrayType:
		return extractdb.TypeKindArray, true
	case dwarf.TagBaseType:
		return extractdb.TypeKindBase, true
	case dwarf.TagSubroutineType, dwarf.TagPtrToMemberType:
		return extractdb.TypeKindSubroutine, true
	case dwarf.TagUnspecifiedType, dwarf.TagRestrictType:
		return extractdb.TypeKindUnspecified, true
	default:
		return "", false
	}
}

func compositeKindGroup(kind string) (string, bool) {
	switch kind {
	case extractdb.TypeKindClass, extractdb.TypeKindStruct:
		return "composite", true
	case extractdb.TypeKindUnion:
		return "union", true
	case extractdb.TypeKindEnum:
		return "enum", true
	default:
		return "", false
	}
}

func isScopedNamedTypeKind(kind string) bool {
	switch kind {
	case extractdb.TypeKindClass, extractdb.TypeKindStruct, extractdb.TypeKindUnion,
		extractdb.TypeKindEnum, extractdb.TypeKindTypedef:
		return true
	default:
		return false
	}
}

func isNamespaceOrAggregateTag(tag dwarf.Tag) bool {
	switch tag {
	case dwarf.TagNamespace, dwarf.TagClassType, dwarf.TagStructType, dwarf.TagUnionType:
		return true
	default:
		return false
	}
}

func intValOK(e *dwarf.Entry, attr dwarf.Attr) (int64, bool) {
	f := e.AttrField(attr)
	if f == nil {
		return 0, false
	}
	switch v := f.Val.(type) {
	case int64:
		return v, true
	case uint64:
		return int64(v), true
	default:
		return 0, false
	}
}

func boolVal(e *dwarf.Entry, attr dwarf.Attr) bool {
	v, ok := e.Val(attr).(bool)
	return ok && v
}
