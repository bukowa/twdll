package extractdb

// Meta table keys and canonical values.
const (
	MetaSchemaVersion     = "schema_version"
	MetaTarget            = "target"
	MetaBinarySHA256      = "binary_sha256"
	MetaImageBase         = "image_base"
	MetaArch              = "arch"
	MetaFormat            = "format"
	MetaBronzeExtractedAt = "bronze_extracted_at"
	MetaSilverEngine      = "silver_engine"
	MetaSilverExtractedAt = "silver_extracted_at"

	ArchX86_64 = "x86_64"
	ArchX86    = "x86"

	FormatELF64 = "elf64"
	FormatPE32  = "pe32"
)

// sections.flags bitmask constants.
const (
	SecFlagRead  uint32 = 1 << 0
	SecFlagWrite uint32 = 1 << 1
	SecFlagExec  uint32 = 1 << 2
	SecFlagAlloc uint32 = 1 << 3
)

// functions.discovery_mask bitmask constants.
const (
	DiscoveryDWARF      uint32 = 1 << 0
	DiscoverySymtab     uint32 = 1 << 1
	DiscoveryExport     uint32 = 1 << 2
	DiscoveryVtable     uint32 = 1 << 3
	DiscoveryLuaReg     uint32 = 1 << 4
	DiscoveryCallTarget uint32 = 1 << 5
	DiscoveryDataPtr    uint32 = 1 << 6
	DiscoveryPrologue   uint32 = 1 << 7
	DiscoveryGhidra     uint32 = 1 << 8
	DiscoveryIDA        uint32 = 1 << 9
)

// functions.flags bitmask constants.
const (
	FuncFlagThunk          uint32 = 1 << 0
	FuncFlagVirtual        uint32 = 1 << 1
	FuncFlagNoReturn       uint32 = 1 << 2
	FuncFlagExternal       uint32 = 1 << 3
	FuncFlagICFCandidate   uint32 = 1 << 4
	FuncFlagNoSeedOverride uint32 = 1 << 5
)

// functions.cc_source string enum constants.
const (
	CCSourceDWARF  = "bronze_dwarf"
	CCSourceVtable = "bronze_vtable"
	CCSourceLuaReg = "bronze_lua_reg"
	CCSourceX86Asm = "bronze_x86asm"
	CCSourceGhidra = "silver_ghidra"
	CCSourceIDA    = "silver_ida"
)

// types.kind string enum constants.
const (
	TypeKindClass       = "class"
	TypeKindStruct      = "struct"
	TypeKindUnion       = "union"
	TypeKindEnum        = "enum"
	TypeKindTypedef     = "typedef"
	TypeKindPointer     = "pointer"
	TypeKindReference   = "reference"
	TypeKindRValueRef   = "rvalue_ref"
	TypeKindConst       = "const"
	TypeKindVolatile    = "volatile"
	TypeKindArray       = "array"
	TypeKindBase        = "base"
	TypeKindSubroutine  = "subroutine"
	TypeKindUnspecified = "unspecified"
)

// type_members.member_kind string enum constants.
const (
	MemberKindField  = "field"
	MemberKindBase   = "base"
	MemberKindStatic = "static"
)

// func_vars.kind string enum constants.
const (
	VarKindParam = "param"
	VarKindLocal = "local"
)

// strings.encoding string enum constants.
const (
	StringEncASCII   = "ascii"
	StringEncUTF16LE = "utf16le"
	StringEncUTF32LE = "utf32le"
)

// call_sites.call_kind string enum constants.
const (
	CallKindDirect      = "direct"
	CallKindIndirectReg = "indirect_reg"
	CallKindIndirectMem = "indirect_mem"
	CallKindTailJump    = "tail_jmp"
)

// data_xrefs.access_kind string enum constants.
const (
	AccessKindAddr  = "addr"
	AccessKindRead  = "read"
	AccessKindWrite = "write"
)

// decomp_funcs.status string enum constants.
const (
	DecompStatusOK      = "ok"
	DecompStatusFailed  = "failed"
	DecompStatusTimeout = "timeout"
)

// DecompVar represents one normalized variable entry in decompiler_vars_json.
type DecompVar struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	IsArg   bool   `json:"is_arg"`
	Size    int    `json:"size"`
	Storage string `json:"storage"`
}
