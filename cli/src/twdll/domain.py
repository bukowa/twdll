"""Single source of truth for all twdll domain types, enums, specs, and dataclasses.

This module uses only Python standard library modules so it can be imported
directly by the CLI, PyGhidra, IDAPython, and derivation scripts.
"""

from __future__ import annotations

import struct
import sys
import tomllib
from collections.abc import Callable, Sequence
from dataclasses import dataclass, field, fields, is_dataclass
from enum import StrEnum
from pathlib import Path
from typing import NewType, cast

Address = NewType("Address", int)
SteamAppId = NewType("SteamAppId", str)


class Decompiler(StrEnum):
    GHIDRA = "ghidra"
    IDA = "ida"


class BinaryName(StrEnum):
    ATTILA_LINUX_64 = "attila_linux_64"
    ATTILA_WINDOWS_32 = "attila_windows_32"
    ROME2_WINDOWS_32 = "rome2_windows_32"
    TOB_WINDOWS_32 = "tob_windows_32"


class BinaryArch(StrEnum):
    ELF64 = "elf64"
    PE32 = "pe32"


class HostOS(StrEnum):
    LINUX = "linux"
    WINDOWS = "windows"

    @classmethod
    def current(cls) -> HostOS:
        if sys.platform.startswith("win"):
            return cls.WINDOWS
        return cls.LINUX


class StepName(StrEnum):
    INIT = "init"
    ANALYZE = "analyze"
    EXTRACT = "extract"
    DERIVE = "derive"


class WorkspaceName(StrEnum):
    CONFIG_FILE = "twdll.toml"
    DATA_DIR = ".twdll"
    UV_DIR = "uv"
    TOOLS_DIR = "tools"
    BINARIES_DIR = "binaries"


class FileSuffix(StrEnum):
    DLL = ".dll"
    SQLITE = ".sqlite"
    GHIDRA_GPR = ".gpr"
    GHIDRA_REP = ".rep"
    IDA_I64 = ".i64"


class CleanTarget(StrEnum):
    INIT = "init"
    ANALYZE = "analyze"
    EXTRACT = "extract"
    DERIVE = "derive"
    ALL = "all"

    def steps(self) -> tuple[StepName, ...]:
        match self:
            case CleanTarget.INIT:
                return (StepName.INIT,)
            case CleanTarget.ANALYZE:
                return (StepName.ANALYZE,)
            case CleanTarget.EXTRACT:
                return (StepName.EXTRACT,)
            case CleanTarget.DERIVE:
                return (StepName.DERIVE,)
            case CleanTarget.ALL:
                return (
                    StepName.INIT,
                    StepName.ANALYZE,
                    StepName.EXTRACT,
                    StepName.DERIVE,
                )


class ToolName(StrEnum):
    GHIDRA = "ghidra"
    JAVA = "java"
    IDA = "ida"


class ToolSource(StrEnum):
    TOML = "twdll.toml"
    PORTABLE = ".twdll/init/tools"
    DEFAULT_DIR = "default_install_dir"
    MISSING = "missing"


class BinarySource(StrEnum):
    TOML = "twdll.toml"
    TOML_INVALID = "twdll.toml (invalid)"
    STEAM = "steam"
    STAGED = "staged"
    MISSING = "missing"


class SymbolKind(StrEnum):
    FUNCTION = "function"
    DATA = "data"
    IMPORT = "import"
    EXPORT = "export"
    LABEL = "label"


class XrefKind(StrEnum):
    CALL = "call"
    DATA_READ = "data_read"
    DATA_WRITE = "data_write"
    OFFSET = "offset"


class StringEncoding(StrEnum):
    ASCII = "ascii"
    UTF8 = "utf8"
    UTF16_LE = "utf16_le"


class MatchMethod(StrEnum):
    EXACT_SYMBOL = "exact_symbol"
    UNIQUE_STRING = "unique_string"
    CALLGRAPH_NEIGHBOR = "callgraph_neighbor"
    VTABLE_SLOT = "vtable_slot"


class SearchKind(StrEnum):
    FUNCTION = "function"
    SYMBOL = "symbol"
    STRING = "string"
    VTABLE = "vtable"


class DatabaseTarget(StrEnum):
    EXTRACT = "extract"
    DERIVE = "derive"


class ProgressKind(StrEnum):
    DOWNLOAD = "download"
    EXTRACT_ARCHIVE = "extract_archive"
    STAGE_BINARY = "stage_binary"
    STEP = "step"


@dataclass(frozen=True, slots=True)
class ProgressEvent:
    kind: ProgressKind
    task_key: str
    label: str
    completed: int
    total: int | None


ProgressCallback = Callable[[ProgressEvent], None]


def noop_progress(_event: ProgressEvent) -> None:
    """No-op progress callback used when terminal progress display is disabled."""


@dataclass(frozen=True, slots=True)
class BinarySpec:
    name: BinaryName
    steam_app_id: SteamAppId
    default_install_dir: str
    file_name: str
    staged_file_name: str
    arch: BinaryArch


BINARY_SPECS: dict[BinaryName, BinarySpec] = {
    BinaryName.ATTILA_LINUX_64: BinarySpec(
        name=BinaryName.ATTILA_LINUX_64,
        steam_app_id=SteamAppId("325610"),
        default_install_dir="Total War Attila",
        file_name="Attila",
        staged_file_name=BinaryName.ATTILA_LINUX_64.value,
        arch=BinaryArch.ELF64,
    ),
    BinaryName.ATTILA_WINDOWS_32: BinarySpec(
        name=BinaryName.ATTILA_WINDOWS_32,
        steam_app_id=SteamAppId("325610"),
        default_install_dir="Total War Attila",
        file_name="empire.retail.dll",
        staged_file_name=f"{BinaryName.ATTILA_WINDOWS_32.value}{FileSuffix.DLL.value}",
        arch=BinaryArch.PE32,
    ),
    BinaryName.ROME2_WINDOWS_32: BinarySpec(
        name=BinaryName.ROME2_WINDOWS_32,
        steam_app_id=SteamAppId("214950"),
        default_install_dir="Total War Rome II",
        file_name="empire.retail.dll",
        staged_file_name=f"{BinaryName.ROME2_WINDOWS_32.value}{FileSuffix.DLL.value}",
        arch=BinaryArch.PE32,
    ),
    BinaryName.TOB_WINDOWS_32: BinarySpec(
        name=BinaryName.TOB_WINDOWS_32,
        steam_app_id=SteamAppId("712100"),
        default_install_dir="Total War Saga Thrones of Britannia",
        file_name="empire.retail.dll",
        staged_file_name=f"{BinaryName.TOB_WINDOWS_32.value}{FileSuffix.DLL.value}",
        arch=BinaryArch.PE32,
    ),
}


@dataclass(frozen=True, slots=True)
class PortableArchiveSpec:
    tool: ToolName
    url: str
    archive_name: str
    extracted_prefix: str


GHIDRA_PORTABLE_SPEC = PortableArchiveSpec(
    tool=ToolName.GHIDRA,
    url="https://github.com/NationalSecurityAgency/ghidra/releases/download/Ghidra_12.2_build/ghidra_12.2_PUBLIC_20261016.zip",
    archive_name="ghidra_12.2_PUBLIC_20261016.zip",
    extracted_prefix="ghidra_12.2_PUBLIC",
)

JDK_PORTABLE_SPECS: dict[HostOS, PortableArchiveSpec] = {
    HostOS.LINUX: PortableArchiveSpec(
        tool=ToolName.JAVA,
        url="https://github.com/adoptium/temurin25-binaries/releases/download/jdk-25.0.5.1%2B2/OpenJDK25U-jdk_x64_linux_hotspot_25.0.5.1_2.tar.gz",
        archive_name="OpenJDK25U-jdk_x64_linux_hotspot_25.0.5.1_2.tar.gz",
        extracted_prefix="jdk-25",
    ),
    HostOS.WINDOWS: PortableArchiveSpec(
        tool=ToolName.JAVA,
        url="https://github.com/adoptium/temurin25-binaries/releases/download/jdk-25.0.5.1%2B2/OpenJDK25U-jdk_x64_windows_hotspot_25.0.5.1_2.zip",
        archive_name="OpenJDK25U-jdk_x64_windows_hotspot_25.0.5.1_2.zip",
        extracted_prefix="jdk-25",
    ),
}


def default_ida_dirs(host_os: HostOS, home_dir: Path) -> tuple[Path, ...]:
    match host_os:
        case HostOS.WINDOWS:
            return (
                Path("C:/Program Files/IDA Professional 9.5"),
                Path("C:/Program Files/IDA Pro 9.5"),
            )
        case HostOS.LINUX:
            return (
                Path("/opt/idapro-9.5"),
                Path("/opt/ida-9.5"),
                home_dir / "idapro-9.5",
                home_dir / "ida-9.5",
            )


def default_steam_roots(host_os: HostOS, home_dir: Path) -> tuple[Path, ...]:
    match host_os:
        case HostOS.LINUX:
            return (
                home_dir / ".local" / "share" / "Steam",
                home_dir / ".steam" / "steam",
                home_dir / ".steam" / "root",
                home_dir
                / ".var"
                / "app"
                / "com.valvesoftware.Steam"
                / ".local"
                / "share"
                / "Steam",
                home_dir / "snap" / "steam" / "common" / ".local" / "share" / "Steam",
            )
        case HostOS.WINDOWS:
            return (
                Path("C:/Program Files (x86)/Steam"),
                Path("C:/Program Files/Steam"),
            )


@dataclass(frozen=True, slots=True)
class WorkspacePaths:
    root: Path

    @property
    def config_file(self) -> Path:
        return self.root / WorkspaceName.CONFIG_FILE.value

    @property
    def data_dir(self) -> Path:
        return self.root / WorkspaceName.DATA_DIR.value

    @property
    def uv_dir(self) -> Path:
        return self.data_dir / WorkspaceName.UV_DIR.value

    def step_dir(self, step: StepName) -> Path:
        return self.data_dir / step.value

    @property
    def init_dir(self) -> Path:
        return self.step_dir(StepName.INIT)

    @property
    def tools_dir(self) -> Path:
        return self.init_dir / WorkspaceName.TOOLS_DIR.value

    @property
    def binaries_dir(self) -> Path:
        return self.init_dir / WorkspaceName.BINARIES_DIR.value

    def staged_binary_path(self, binary: BinaryName) -> Path:
        spec = BINARY_SPECS[binary]
        return self.binaries_dir / spec.staged_file_name

    @property
    def analyze_dir(self) -> Path:
        return self.step_dir(StepName.ANALYZE)

    def analyze_stem(self, decompiler: Decompiler, binary: BinaryName) -> str:
        return f"{decompiler.value}_{binary.value}"

    def ghidra_gpr_path(self, binary: BinaryName) -> Path:
        stem = self.analyze_stem(Decompiler.GHIDRA, binary)
        return self.analyze_dir / f"{stem}{FileSuffix.GHIDRA_GPR.value}"

    def ghidra_rep_dir(self, binary: BinaryName) -> Path:
        stem = self.analyze_stem(Decompiler.GHIDRA, binary)
        return self.analyze_dir / f"{stem}{FileSuffix.GHIDRA_REP.value}"

    def ida_db_path(self, binary: BinaryName) -> Path:
        stem = self.analyze_stem(Decompiler.IDA, binary)
        return self.analyze_dir / f"{stem}{FileSuffix.IDA_I64.value}"

    def analyze_artifact_path(self, decompiler: Decompiler, binary: BinaryName) -> Path:
        match decompiler:
            case Decompiler.GHIDRA:
                return self.ghidra_gpr_path(binary)
            case Decompiler.IDA:
                return self.ida_db_path(binary)

    @property
    def extract_dir(self) -> Path:
        return self.step_dir(StepName.EXTRACT)

    def extract_db_path(self, decompiler: Decompiler, binary: BinaryName) -> Path:
        stem = self.analyze_stem(decompiler, binary)
        return self.extract_dir / f"{stem}{FileSuffix.SQLITE.value}"

    @property
    def derive_dir(self) -> Path:
        return self.step_dir(StepName.DERIVE)

    def derive_db_path(self, decompiler: Decompiler) -> Path:
        return self.derive_dir / f"{decompiler.value}{FileSuffix.SQLITE.value}"


def serialize_value(value: object) -> object:
    """Convert domain objects (`Path`, `StrEnum`, dataclasses, tuples) to JSON-ready primitives."""
    if isinstance(value, Path):
        return str(value)
    if isinstance(value, StrEnum):
        return value.value
    if is_dataclass(value) and not isinstance(value, type):
        return {
            f.name: serialize_value(cast("object", getattr(value, f.name))) for f in fields(value)
        }
    if isinstance(value, tuple | list):
        seq = cast("tuple[object, ...] | list[object]", value)
        return [serialize_value(item) for item in seq]
    if isinstance(value, dict):
        mapping = cast("dict[object, object]", value)
        return {str(serialize_value(k)): serialize_value(v) for k, v in mapping.items()}
    return value


def to_dict(instance: object) -> dict[str, object]:
    """Serialize a domain dataclass instance into a JSON-compatible dictionary."""
    if not is_dataclass(instance) or isinstance(instance, type):
        msg = f"Expected dataclass instance, got {type(instance)!r}"
        raise TypeError(msg)
    return {
        f.name: serialize_value(cast("object", getattr(instance, f.name))) for f in fields(instance)
    }


@dataclass(frozen=True, slots=True)
class ToolchainConfig:
    ghidra_dir: Path | None = None
    java_home: Path | None = None
    ida_dir: Path | None = None


def _default_binary_paths() -> dict[BinaryName, Path | None]:
    return dict.fromkeys(BinaryName)


@dataclass(frozen=True, slots=True)
class WorkspaceConfig:
    decompiler: Decompiler = Decompiler.GHIDRA
    binaries: dict[BinaryName, Path | None] = field(default_factory=_default_binary_paths)
    toolchain: ToolchainConfig = field(default_factory=ToolchainConfig)


@dataclass(frozen=True)
class ToolLocation:
    name: ToolName
    ready: bool
    path: Path | None
    source: ToolSource


@dataclass(frozen=True)
class ToolchainStatus:
    decompiler: Decompiler
    ready: bool
    tools: tuple[ToolLocation, ...]


@dataclass(frozen=True, slots=True)
class WorkspaceContext:
    paths: WorkspacePaths
    config: WorkspaceConfig = field(default_factory=WorkspaceConfig)
    host_os: HostOS = field(default_factory=HostOS.current)

    @property
    def root(self) -> Path:
        return self.paths.root

    @property
    def decompiler(self) -> Decompiler:
        return self.config.decompiler

    @classmethod
    def from_root(cls, root: Path) -> WorkspaceContext:
        return cls(
            paths=WorkspacePaths(root),
            config=_load_config(root),
            host_os=HostOS.current(),
        )


@dataclass(frozen=True)
class BinaryStatus:
    name: BinaryName
    arch: BinaryArch
    staged: bool
    staged_path: Path | None
    discovered_path: Path | None
    source: BinarySource
    valid_header: bool


@dataclass(frozen=True)
class StepSummary:
    step: StepName
    directory: Path
    exists: bool
    entries: tuple[str, ...]


@dataclass(frozen=True)
class WorkspaceStatus:
    root: Path
    config_file: Path
    config_exists: bool
    host_os: HostOS
    decompiler: Decompiler
    toolchain: ToolchainStatus
    binaries: tuple[BinaryStatus, ...]
    steps: tuple[StepSummary, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True)
class InitResult:
    config_file: Path
    config_created: bool
    decompiler: Decompiler
    toolchain: ToolchainStatus
    binaries: tuple[BinaryStatus, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True)
class CleanResult:
    target: CleanTarget
    removed_directories: tuple[Path, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True)
class AnalyzeTargetResult:
    binary: BinaryName
    decompiler: Decompiler
    project_path: Path
    analyzed: bool
    skipped: bool


@dataclass(frozen=True)
class AnalyzeResult:
    decompiler: Decompiler
    targets: tuple[AnalyzeTargetResult, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True, slots=True)
class ExtractedBinaryRow:
    binary: BinaryName
    arch: BinaryArch
    decompiler: Decompiler
    image_base: Address
    analyzed_at: str


@dataclass(frozen=True, slots=True)
class FunctionRow:
    binary: BinaryName
    address: Address
    name: str
    demangled_name: str | None
    size: int
    signature: str | None
    is_thunk: bool


@dataclass(frozen=True, slots=True)
class SymbolRow:
    binary: BinaryName
    address: Address
    name: str
    demangled_name: str | None
    kind: SymbolKind


@dataclass(frozen=True, slots=True)
class StringRow:
    binary: BinaryName
    address: Address
    value: str
    encoding: StringEncoding


@dataclass(frozen=True, slots=True)
class XrefRow:
    binary: BinaryName
    from_address: Address
    to_address: Address
    kind: XrefKind


@dataclass(frozen=True, slots=True)
class ExtractTargetResult:
    binary: BinaryName
    functions_count: int
    symbols_count: int
    strings_count: int
    xrefs_count: int


@dataclass(frozen=True, slots=True)
class ExtractResult:
    decompiler: Decompiler
    db_path: Path
    targets: tuple[ExtractTargetResult, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True, slots=True)
class CallEdgeRow:
    binary: BinaryName
    caller_address: Address
    callee_address: Address


@dataclass(frozen=True, slots=True)
class VtableRow:
    binary: BinaryName
    address: Address
    class_name: str
    entry_count: int


@dataclass(frozen=True, slots=True)
class VtableEntryRow:
    binary: BinaryName
    vtable_address: Address
    slot_index: int
    target_address: Address


@dataclass(frozen=True, slots=True)
class FunctionMatchRow:
    source_binary: BinaryName
    source_address: Address
    target_binary: BinaryName
    target_address: Address
    confidence: float
    method: MatchMethod


@dataclass(frozen=True, slots=True)
class DeriveResult:
    decompiler: Decompiler
    db_path: Path
    call_edges_count: int
    vtables_count: int
    matches_count: int

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True, slots=True)
class SearchHit:
    binary: BinaryName
    kind: SearchKind
    address: Address
    name: str
    detail: str | None

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True, slots=True)
class FunctionDetail:
    function: FunctionRow
    callers: tuple[Address, ...]
    callees: tuple[Address, ...]
    strings: tuple[StringRow, ...]
    matches: tuple[FunctionMatchRow, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


SqlScalar = int | float | str | bytes | None


@dataclass(frozen=True, slots=True)
class SqlQueryResult:
    columns: tuple[str, ...]
    rows: tuple[tuple[SqlScalar, ...], ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


_ELF_MAGIC = b"\x7fELF"
_ELF_CLASS_64 = 2
_ELF_DATA_LSB = 1
_ELF_MACHINE_X86_64 = 0x3E
_PE_MACHINE_I386 = 0x014C


def optional_path(raw_value: object) -> Path | None:
    if not isinstance(raw_value, str):
        return None
    stripped = raw_value.strip()
    return Path(stripped) if stripped else None


def _parse_decompiler(raw: dict[str, object], config_file: Path) -> Decompiler:
    decompiler_raw = str(raw.get("decompiler", Decompiler.GHIDRA.value))
    try:
        return Decompiler(decompiler_raw)
    except ValueError as exc:
        valid = ", ".join(d.value for d in Decompiler)
        msg = f"Invalid decompiler {decompiler_raw!r} in {config_file}; expected one of: {valid}"
        raise ValueError(msg) from exc


def _parse_binaries_section(raw: dict[str, object]) -> dict[BinaryName, Path | None]:
    sec_obj = raw.get("binaries")
    sec = cast("dict[str, object]", sec_obj) if isinstance(sec_obj, dict) else {}
    return {name: optional_path(sec.get(name.value)) for name in BinaryName}


def _parse_toolchain_section(raw: dict[str, object]) -> ToolchainConfig:
    sec_obj = raw.get("toolchain")
    if not isinstance(sec_obj, dict):
        return ToolchainConfig()
    sec = cast("dict[str, object]", sec_obj)
    return ToolchainConfig(
        ghidra_dir=optional_path(sec.get("ghidra_dir")),
        java_home=optional_path(sec.get("java_home")),
        ida_dir=optional_path(sec.get("ida_dir")),
    )


def _load_config(root: Path) -> WorkspaceConfig:
    """Load `twdll.toml` from `root`, or return defaults if it does not exist."""
    paths = WorkspacePaths(root)
    if not paths.config_file.is_file():
        return WorkspaceConfig()
    raw = tomllib.loads(paths.config_file.read_text(encoding="utf-8"))
    return WorkspaceConfig(
        decompiler=_parse_decompiler(raw, paths.config_file),
        binaries=_parse_binaries_section(raw),
        toolchain=_parse_toolchain_section(raw),
    )


def _is_valid_elf64(header: bytes) -> bool:
    if len(header) < 20 or header[:4] != _ELF_MAGIC:
        return False
    if header[4] != _ELF_CLASS_64 or header[5] != _ELF_DATA_LSB:
        return False
    (e_machine,) = struct.unpack_from("<H", header, 18)
    return int(e_machine) == _ELF_MACHINE_X86_64


def _read_pe_signature_chunk(path: Path, header: bytes, pe_offset: int) -> bytes:
    if pe_offset + 6 <= len(header):
        return header[pe_offset : pe_offset + 6]
    try:
        with path.open("rb") as handle:
            handle.seek(pe_offset)
            return handle.read(6)
    except OSError:
        return b""


def _is_valid_pe32(path: Path, header: bytes) -> bool:
    if len(header) < 64 or header[:2] != b"MZ":
        return False
    (pe_offset,) = struct.unpack_from("<I", header, 0x3C)
    pe_chunk = _read_pe_signature_chunk(path, header, int(pe_offset))
    if len(pe_chunk) < 6 or pe_chunk[:4] != b"PE\x00\x00":
        return False
    (machine,) = struct.unpack_from("<H", pe_chunk, 4)
    return int(machine) == _PE_MACHINE_I386


def verify_binary_header(path: Path, expected_arch: BinaryArch) -> bool:
    """Verify that `path` has a valid ELF64 (x86_64) or PE32 (i386) header."""
    if not path.is_file():
        return False
    try:
        with path.open("rb") as handle:
            header = handle.read(4096)
    except OSError:
        return False

    match expected_arch:
        case BinaryArch.ELF64:
            return _is_valid_elf64(header)
        case BinaryArch.PE32:
            return _is_valid_pe32(path, header)


def _ida_executable(ida_dir: Path, host_os: HostOS) -> Path:
    match host_os:
        case HostOS.WINDOWS:
            candidates = ("idat64.exe", "ida64.exe", "ida.exe")
        case HostOS.LINUX:
            candidates = ("idat64", "ida64", "ida")
    for name in candidates:
        candidate = ida_dir / name
        if candidate.is_file():
            return candidate
    return ida_dir / candidates[0]


def _tool_executable(tool: ToolName, tool_dir: Path, host_os: HostOS) -> Path:
    match tool:
        case ToolName.GHIDRA:
            script = "analyzeHeadless.bat" if host_os is HostOS.WINDOWS else "analyzeHeadless"
            return tool_dir / "support" / script
        case ToolName.JAVA:
            exe = "java.exe" if host_os is HostOS.WINDOWS else "java"
            return tool_dir / "bin" / exe
        case ToolName.IDA:
            return _ida_executable(tool_dir, host_os)


def _find_prefixed_subdirs(tools_dir: Path, prefix: str) -> tuple[Path, ...]:
    if not tools_dir.is_dir():
        return ()
    return tuple(
        sorted(
            (c for c in tools_dir.iterdir() if c.is_dir() and c.name.startswith(prefix)),
            key=lambda p: p.name,
            reverse=True,
        )
    )


def resolve_tool(
    tool: ToolName,
    configured_dir: Path | None,
    candidate_dirs: Sequence[Path],
    fallback_source: ToolSource,
    host_os: HostOS,
) -> ToolLocation:
    """Resolve a single tool strictly from `twdll.toml` or `candidate_dirs` (never user PATH)."""
    if configured_dir is not None:
        ready = _tool_executable(tool, configured_dir, host_os).is_file()
        return ToolLocation(name=tool, ready=ready, path=configured_dir, source=ToolSource.TOML)

    for candidate in candidate_dirs:
        if _tool_executable(tool, candidate, host_os).is_file():
            return ToolLocation(name=tool, ready=True, path=candidate, source=fallback_source)

    return ToolLocation(name=tool, ready=False, path=None, source=ToolSource.MISSING)


def resolve_toolchain(
    ctx: WorkspaceContext,
    ida_dirs: Sequence[Path] | None = None,
) -> ToolchainStatus:
    """Resolve active decompiler toolchain without inspecting user PATH."""
    match ctx.decompiler:
        case Decompiler.GHIDRA:
            ghidra_loc = resolve_tool(
                tool=ToolName.GHIDRA,
                configured_dir=ctx.config.toolchain.ghidra_dir,
                candidate_dirs=_find_prefixed_subdirs(ctx.paths.tools_dir, "ghidra_"),
                fallback_source=ToolSource.PORTABLE,
                host_os=ctx.host_os,
            )
            java_loc = resolve_tool(
                tool=ToolName.JAVA,
                configured_dir=ctx.config.toolchain.java_home,
                candidate_dirs=_find_prefixed_subdirs(ctx.paths.tools_dir, "jdk-"),
                fallback_source=ToolSource.PORTABLE,
                host_os=ctx.host_os,
            )
            tools = (ghidra_loc, java_loc)
            return ToolchainStatus(
                decompiler=Decompiler.GHIDRA,
                ready=all(t.ready for t in tools),
                tools=tools,
            )
        case Decompiler.IDA:
            search_dirs = (
                tuple(ida_dirs)
                if ida_dirs is not None
                else default_ida_dirs(ctx.host_os, Path.home())
            )
            ida_loc = resolve_tool(
                tool=ToolName.IDA,
                configured_dir=ctx.config.toolchain.ida_dir,
                candidate_dirs=search_dirs,
                fallback_source=ToolSource.DEFAULT_DIR,
                host_os=ctx.host_os,
            )
            return ToolchainStatus(
                decompiler=Decompiler.IDA,
                ready=ida_loc.ready,
                tools=(ida_loc,),
            )
