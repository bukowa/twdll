"""Single source of truth for all twdll domain types, enums, specs, and dataclasses.

This module uses only Python standard library modules so it can be imported
directly by the CLI, PyGhidra, IDAPython, and derivation scripts.
"""

from __future__ import annotations

import hashlib
import struct
import sys
import tomllib
from collections.abc import Callable, Sequence
from dataclasses import dataclass, field, fields, is_dataclass
from enum import StrEnum
from pathlib import Path
from typing import Final, NewType, cast

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


class WorkspaceName(StrEnum):
    CONFIG_FILE = "twdll.toml"
    DATA_DIR = ".twdll"
    UV_DIR = "uv"
    TOOLS_DIR = "tools"
    BINARIES_DIR = "binaries"


class FileSuffix(StrEnum):
    DLL = ".dll"
    SQLITE = ".sqlite"
    SQLITE_STAGING = ".sqlite.staging"
    GHIDRA_GPR = ".gpr"
    GHIDRA_REP = ".rep"
    IDA_I64 = ".i64"
    META_JSON = ".meta.json"


class CleanTarget(StrEnum):
    INIT = "init"
    ANALYZE = "analyze"
    EXTRACT = "extract"
    ALL = "all"

    def steps(self) -> tuple[StepName, ...]:
        match self:
            case CleanTarget.INIT:
                return (StepName.INIT,)
            case CleanTarget.ANALYZE:
                return (StepName.ANALYZE,)
            case CleanTarget.EXTRACT:
                return (StepName.EXTRACT,)
            case CleanTarget.ALL:
                return (
                    StepName.INIT,
                    StepName.ANALYZE,
                    StepName.EXTRACT,
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


STEAM_CONSOLE_URL = "steam://open/console"


@dataclass(frozen=True, slots=True)
class BinarySpec:
    name: BinaryName
    steam_app_id: SteamAppId
    default_install_dir: str
    file_name: str
    staged_file_name: str
    arch: BinaryArch
    depot_command: str | None = None
    expected_sha256: str | None = None
    is_reference: bool = False


BINARY_SPECS: dict[BinaryName, BinarySpec] = {
    BinaryName.ATTILA_LINUX_64: BinarySpec(
        name=BinaryName.ATTILA_LINUX_64,
        steam_app_id=SteamAppId("325610"),
        default_install_dir="Total War Attila",
        file_name="Attila",
        staged_file_name=BinaryName.ATTILA_LINUX_64.value,
        arch=BinaryArch.ELF64,
        depot_command="download_depot 325610 325624 3804137149572368482",
        expected_sha256="217e14350c26d5edfd4e4205516b3498e45e4b6c675923c0e0b0df7190fd8784",
        is_reference=True,
    ),
    BinaryName.ATTILA_WINDOWS_32: BinarySpec(
        name=BinaryName.ATTILA_WINDOWS_32,
        steam_app_id=SteamAppId("325610"),
        default_install_dir="Total War Attila",
        file_name="empire.retail.dll",
        staged_file_name=f"{BinaryName.ATTILA_WINDOWS_32.value}{FileSuffix.DLL.value}",
        arch=BinaryArch.PE32,
        depot_command="download_depot 325610 325612 4239124750842070752",
        expected_sha256="8a40cbcff108e1cba14cc7eb6c7f4dfb054c7745b236d1bfc0fde5c85e2a353d",
    ),
    BinaryName.ROME2_WINDOWS_32: BinarySpec(
        name=BinaryName.ROME2_WINDOWS_32,
        steam_app_id=SteamAppId("214950"),
        default_install_dir="Total War Rome II",
        file_name="empire.retail.dll",
        staged_file_name=f"{BinaryName.ROME2_WINDOWS_32.value}{FileSuffix.DLL.value}",
        arch=BinaryArch.PE32,
        depot_command="download_depot 214950 214952 7498603388646092391",
        expected_sha256="07cb2215b80591224a5f8889963344d560492f09c4d22696c988b957ca49b2b7",
    ),
    BinaryName.TOB_WINDOWS_32: BinarySpec(
        name=BinaryName.TOB_WINDOWS_32,
        steam_app_id=SteamAppId("712100"),
        default_install_dir="Total War Saga Thrones of Britannia",
        file_name="empire.retail.dll",
        staged_file_name=f"{BinaryName.TOB_WINDOWS_32.value}{FileSuffix.DLL.value}",
        arch=BinaryArch.PE32,
        depot_command="download_depot 712100 712101 5977806763661092520",
        expected_sha256="89b7d958d2f8906d301b4be3cbe7b36fe7a544c90ce5c92c7fbaa319c1427a03",
    ),
}

REFERENCE_BINARIES: tuple[BinaryName, ...] = tuple(
    name for name, spec in BINARY_SPECS.items() if spec.is_reference
)
SELECTABLE_TARGETS: tuple[BinaryName, ...] = tuple(
    name for name, spec in BINARY_SPECS.items() if not spec.is_reference
)


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


def default_steamapps_subdirs(host_os: HostOS) -> tuple[Path, ...]:
    """Relative `steamapps` directories under a Steam root (including Linux runtime depots)."""
    match host_os:
        case HostOS.LINUX:
            return (
                Path("steamapps"),
                Path("steamrt64") / "steamapps",
                Path("ubuntu12_32") / "steamapps",
            )
        case HostOS.WINDOWS:
            return (Path("steamapps"),)


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

    def analyze_meta_path(self, decompiler: Decompiler, binary: BinaryName) -> Path:
        stem = self.analyze_stem(decompiler, binary)
        return self.analyze_dir / f"{stem}{FileSuffix.META_JSON.value}"

    @property
    def extract_dir(self) -> Path:
        return self.step_dir(StepName.EXTRACT)

    def extract_db_path(self, decompiler: Decompiler, binary: BinaryName) -> Path:
        stem = self.analyze_stem(decompiler, binary)
        return self.extract_dir / f"{stem}{FileSuffix.SQLITE.value}"

    def extract_staging_db_path(self, decompiler: Decompiler, binary: BinaryName) -> Path:
        stem = self.analyze_stem(decompiler, binary)
        return self.extract_dir / f"{stem}{FileSuffix.SQLITE_STAGING.value}"


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
    targets: tuple[BinaryName, ...] = SELECTABLE_TARGETS
    binaries: dict[BinaryName, Path | None] = field(default_factory=_default_binary_paths)
    toolchain: ToolchainConfig = field(default_factory=ToolchainConfig)


def normalize_targets(targets: Sequence[BinaryName]) -> tuple[BinaryName, ...]:
    """Validate and deduplicate selectable target binaries (at least one required)."""
    selected = tuple(dict.fromkeys(t for t in targets if not BINARY_SPECS[t].is_reference))
    if not selected:
        valid = ", ".join(t.value for t in SELECTABLE_TARGETS)
        msg = f"Select at least one target binary from: {valid}"
        raise ValueError(msg)
    return selected


def active_binaries(config: WorkspaceConfig) -> tuple[BinaryName, ...]:
    """Return required reference binaries followed by selected workspace targets."""
    return tuple(dict.fromkeys((*REFERENCE_BINARIES, *config.targets)))


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
    depot_command: str | None = None
    instruction: str | None = None
    sha256: str | None = None
    sha256_match: bool | None = None


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
    duration_seconds: float = 0.0


@dataclass(frozen=True)
class AnalyzeResult:
    decompiler: Decompiler
    targets: tuple[AnalyzeTargetResult, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


@dataclass(frozen=True)
class ExtractTargetResult:
    binary: BinaryName
    db_path: Path
    skipped: bool = False
    duration_seconds: float = 0.0


@dataclass(frozen=True)
class ExtractResult:
    decompiler: Decompiler
    targets: tuple[ExtractTargetResult, ...]

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


SqlScalar = int | float | str | None
SqlRow = tuple[SqlScalar, ...]


@dataclass(frozen=True)
class SqlQueryResult:
    columns: tuple[str, ...]
    rows: tuple[SqlRow, ...]
    truncated: bool = False

    def to_dict(self) -> dict[str, object]:
        return to_dict(self)


DWARF_EXPORTER_DIR_NAME: Final[str] = "twdll-dwarf"
DWARF_EXPORTER_BUILD_REL_DIR: Final[tuple[str, ...]] = (
    "tools",
    "twdll-dwarf",
    "target",
    "release",
)
DWARF_EXPORTER_EXECUTABLES: Final[dict[HostOS, str]] = {
    HostOS.LINUX: "twdll-dwarf",
    HostOS.WINDOWS: "twdll-dwarf.exe",
}


def resolve_dwarf_exporter(ctx: WorkspaceContext) -> Path:
    """Resolve `twdll-dwarf` executable from repo `target/release/` or `.twdll/init/tools/`."""
    exe = DWARF_EXPORTER_EXECUTABLES[ctx.host_os]
    repo_bin = ctx.paths.root.joinpath(*DWARF_EXPORTER_BUILD_REL_DIR, exe)
    staged_bin = ctx.paths.tools_dir / DWARF_EXPORTER_DIR_NAME / exe
    repo_exists = repo_bin.is_file()
    staged_exists = staged_bin.is_file()
    if repo_exists and staged_exists:
        if repo_bin.stat().st_mtime_ns >= staged_bin.stat().st_mtime_ns:
            return repo_bin
        return staged_bin
    if repo_exists:
        return repo_bin
    if staged_exists:
        return staged_bin
    searched = ", ".join(str(path) for path in (repo_bin, staged_bin))
    msg = f"twdll-dwarf binary not found; searched: {searched}"
    raise RuntimeError(msg)


@dataclass(frozen=True, slots=True)
class DwarfUnitRow:
    unit_id: int
    name: str
    comp_dir: str
    producer: str
    language: int


@dataclass(frozen=True, slots=True)
class DwarfTypeRow:
    type_id: int
    kind: str
    qualified_name: str
    byte_size: int | None
    target_type_id: int | None
    array_count: int | None
    containing_type_id: int | None
    vtable_symbol: str | None
    vtable_address: int | None
    is_declaration: bool
    is_non_trivial_for_calls: bool
    is_stl_stub: bool
    is_referenced: bool
    abi_class: str | None = None


@dataclass(frozen=True, slots=True)
class DwarfMemberRow:
    member_id: int
    parent_type_id: int
    ordinal: int
    name: str
    member_type_id: int
    byte_offset: int
    byte_size: int
    bit_offset: int | None
    bit_size: int | None
    is_artificial: bool
    is_flattened_base_member: bool
    is_layout_conflict: bool


@dataclass(frozen=True, slots=True)
class DwarfInheritanceRow:
    inheritance_id: int
    derived_type_id: int
    base_type_id: int
    byte_offset: int | None
    vbase_vtable_offset: int | None
    is_virtual: bool
    is_empty_base: bool
    has_tail_padding_overlap: bool


@dataclass(frozen=True, slots=True)
class DwarfVtableEntryRow:
    class_type_id: int
    slot_index: int
    method_name: str
    linkage_name: str | None
    function_id: int | None
    target_address: int | None
    is_pure_virtual: bool


@dataclass(frozen=True, slots=True)
class DwarfEnumeratorRow:
    enum_type_id: int
    ordinal: int
    name: str
    value: int


@dataclass(frozen=True, slots=True)
class DwarfFunctionRow:
    function_id: int
    unit_id: int
    address: int | None
    byte_size: int | None
    linkage_name: str | None
    qualified_name: str
    containing_type_id: int | None
    return_type_id: int | None
    vtable_slot_index: int | None
    virtuality: int
    is_inline: bool
    is_declaration: bool
    is_variadic: bool
    has_object_pointer: bool
    is_sret: bool
    decl_file: str | None
    decl_line: int | None


@dataclass(frozen=True, slots=True)
class DwarfParameterRow:
    function_id: int
    ordinal: int
    name: str | None
    type_id: int
    is_artificial: bool
    is_object_pointer: bool
    storage_kind: str
    dwarf_reg: int | None
    reg_name: str | None
    stack_offset: int | None


@dataclass(frozen=True, slots=True)
class DwarfVariableRow:
    variable_id: int
    function_id: int
    name: str
    type_id: int
    range_index: int
    start_address: int
    end_address: int
    first_use_offset: int
    storage_kind: str
    reg_name: str | None
    stack_offset: int | None
    const_value: int | None


@dataclass(frozen=True, slots=True)
class DwarfInlineRow:
    inline_id: int
    caller_function_id: int
    callee_function_id: int | None
    callee_name: str
    start_address: int
    end_address: int
    call_file: str | None
    call_line: int | None


@dataclass(frozen=True, slots=True)
class DwarfGlobalRow:
    global_id: int
    unit_id: int
    address: int
    linkage_name: str | None
    qualified_name: str
    type_id: int
    byte_size: int | None
    is_static_member: bool


class DwarfPassName(StrEnum):
    PASS1_TYPES = "pass1_types"
    PASS2_LAYOUTS = "pass2_layouts"
    PASS3_SIGNATURES = "pass3_signatures"
    PASS4_LOCALS = "pass4_locals"


@dataclass(frozen=True)
class DwarfPassStats:
    pass_name: DwarfPassName
    counts: tuple[tuple[str, int], ...] = ()
    error_counts: tuple[tuple[str, int], ...] = ()
    sample_errors: tuple[str, ...] = ()

    def to_dict(self) -> dict[str, object]:
        return {
            "pass_name": self.pass_name.value,
            "counts": dict(self.counts),
            "error_counts": dict(self.error_counts),
            "sample_errors": list(self.sample_errors),
        }


@dataclass(frozen=True)
class DwarfInjectionReport:
    pass1: DwarfPassStats
    pass2: DwarfPassStats
    pass3: DwarfPassStats
    pass4: DwarfPassStats
    total_errors: int

    def to_dict(self) -> dict[str, object]:
        return {
            "pass1": self.pass1.to_dict(),
            "pass2": self.pass2.to_dict(),
            "pass3": self.pass3.to_dict(),
            "pass4": self.pass4.to_dict(),
            "total_errors": self.total_errors,
        }


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


def _parse_targets(raw: dict[str, object], config_file: Path) -> tuple[BinaryName, ...]:
    targets_obj = raw.get("targets")
    if targets_obj is None:
        return SELECTABLE_TARGETS
    if not isinstance(targets_obj, list):
        msg = f"Invalid 'targets' in {config_file}; expected a list of target binary names"
        raise ValueError(msg)
    parsed: list[BinaryName] = []
    raw_items = cast("list[object]", targets_obj)
    for item in raw_items:
        try:
            parsed.append(BinaryName(str(item)))
        except ValueError as exc:
            valid = ", ".join(t.value for t in SELECTABLE_TARGETS)
            msg = f"Invalid target {item!r} in {config_file}; expected one of: {valid}"
            raise ValueError(msg) from exc
    return normalize_targets(parsed)


def _load_config(root: Path) -> WorkspaceConfig:
    """Load `twdll.toml` from `root`, or return defaults if it does not exist."""
    paths = WorkspacePaths(root)
    if not paths.config_file.is_file():
        return WorkspaceConfig()
    raw = tomllib.loads(paths.config_file.read_text(encoding="utf-8"))
    return WorkspaceConfig(
        decompiler=_parse_decompiler(raw, paths.config_file),
        targets=_parse_targets(raw, paths.config_file),
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


def compute_file_sha256(path: Path) -> str | None:
    """Compute lowercase SHA-256 hex digest of `path`, or `None` if unreadable."""
    if not path.is_file():
        return None
    try:
        with path.open("rb") as handle:
            return hashlib.file_digest(handle, "sha256").hexdigest()
    except OSError:
        return None


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
