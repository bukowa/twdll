"""Single source of truth for all twdll domain types, enums, specs, and dataclasses.

This module uses only Python standard library modules so it can be imported
directly by the CLI, PyGhidra, IDAPython, and derivation scripts.
"""

from __future__ import annotations

import sys
from collections.abc import Callable
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


@dataclass(frozen=True, slots=True)
class ToolLocation:
    name: ToolName
    ready: bool
    path: Path | None
    source: ToolSource


@dataclass(frozen=True, slots=True)
class ToolchainStatus:
    decompiler: Decompiler
    ready: bool
    tools: tuple[ToolLocation, ...]


@dataclass(frozen=True, slots=True)
class BinaryStatus:
    name: BinaryName
    arch: BinaryArch
    staged: bool
    staged_path: Path | None
    discovered_path: Path | None
    source: BinarySource
    valid_header: bool


@dataclass(frozen=True, slots=True)
class StepSummary:
    step: StepName
    directory: Path
    exists: bool
    entries: tuple[str, ...]


@dataclass(frozen=True, slots=True)
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


@dataclass(frozen=True, slots=True)
class InitResult:
    config_file: Path
    config_created: bool
    decompiler: Decompiler
    toolchain: ToolchainStatus
    binaries: tuple[BinaryStatus, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True, slots=True)
class CleanResult:
    target: CleanTarget
    removed_directories: tuple[Path, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True, slots=True)
class AnalyzeTargetResult:
    binary: BinaryName
    decompiler: Decompiler
    project_dir: Path
    analyzed: bool
    skipped: bool


@dataclass(frozen=True, slots=True)
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
