package extractdb

// Meta table keys and canonical values shared by bronze.sqlite and silver_<engine>.sqlite.
const (
	MetaTarget            = "target"
	MetaBinarySHA256      = "binary_sha256"
	MetaImageBase         = "image_base"
	MetaArch              = "arch"
	MetaFormat            = "format"
	MetaExtractedAt       = "extracted_at"
	MetaExtractorsApplied = "extractors_applied"
	MetaSilverEngine       = "silver_engine"
	MetaFuncsExtractedAt   = "funcs_extracted_at"
	MetaAsmExtractedAt     = "asm_extracted_at"
	MetaStringsExtractedAt = "strings_extracted_at"
	MetaEnrichersApplied   = "enrichers_applied"
	MetaAspectsApplied    = "aspects_applied"

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
	StringEncUTF8    = "utf8"
	StringEncUTF16LE = "utf16le"
	StringEncUTF32LE = "utf32le"
)

// decomp_funcs.status string enum constants.
const (
	DecompStatusOK      = "ok"
	DecompStatusFailed  = "failed"
	DecompStatusTimeout = "timeout"
)

// DecompVar represents one normalized variable entry in vars_json.
type DecompVar struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	IsArg   bool   `json:"is_arg"`
	Size    int    `json:"size"`
	Storage string `json:"storage"`
}
