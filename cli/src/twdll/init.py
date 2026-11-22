"""`twdll init`, `twdll status`, and `twdll clean` implementation (`cli/src/twdll/init.py`).

Maps 1-to-1 to `.twdll/init/` (`tools/` and `binaries/<name>/`) and `twdll.toml`.
"""

from __future__ import annotations

import importlib
import shutil
import tarfile
import urllib.request
import zipfile
from collections.abc import Callable, Sequence
from contextlib import AbstractContextManager
from pathlib import Path
from typing import cast

import vdf

from twdll.domain import (
    BINARY_SPECS,
    GHIDRA_PORTABLE_SPEC,
    JDK_PORTABLE_SPECS,
    BinaryArch,
    BinaryName,
    BinarySource,
    BinarySpec,
    BinaryStatus,
    CleanResult,
    CleanTarget,
    Decompiler,
    HostOS,
    InitResult,
    PortableArchiveSpec,
    ProgressCallback,
    ProgressEvent,
    ProgressKind,
    StepName,
    StepSummary,
    ToolchainStatus,
    ToolName,
    WorkspaceConfig,
    WorkspaceContext,
    WorkspacePaths,
    WorkspaceStatus,
    default_steam_roots,
    noop_progress,
    optional_path,
    resolve_toolchain,
    verify_binary_header,
)

Downloader = Callable[[str, Path, ProgressCallback], None]
_VdfLoads = Callable[[str], dict[str, object]]


def _escape_toml_path(path: Path | None) -> str:
    raw = str(path) if path is not None else ""
    escaped = raw.replace("\\", "\\\\").replace('"', '\\"')
    return f'"{escaped}"'


def format_config_toml(config: WorkspaceConfig) -> str:
    """Serialize `WorkspaceConfig` to deterministic TOML."""
    lines = [
        '# Active decompiler: "ghidra" or "ida"',
        f'decompiler = "{config.decompiler.value}"',
        "",
        "[binaries]",
        "# Optional explicit file paths (auto-discovered from Steam if empty)",
    ]
    for name in BinaryName:
        lines.append(f"{name.value} = {_escape_toml_path(config.binaries.get(name))}")
    lines.extend(
        [
            "",
            "[toolchain]",
            "# Optional explicit tool directory overrides (never resolved from user PATH)",
            f"ghidra_dir = {_escape_toml_path(config.toolchain.ghidra_dir)}",
            f"java_home = {_escape_toml_path(config.toolchain.java_home)}",
            f"ida_dir = {_escape_toml_path(config.toolchain.ida_dir)}",
            "",
        ]
    )
    return "\n".join(lines)


def save_config(root: Path, config: WorkspaceConfig) -> Path:
    """Write `twdll.toml` to `root`."""
    paths = WorkspacePaths(root)
    paths.config_file.write_text(format_config_toml(config), encoding="utf-8")
    return paths.config_file


def clean(ctx: WorkspaceContext, target: CleanTarget) -> CleanResult:
    """Remove `.twdll/<step>/` directories for `target`, keeping `.twdll/uv/` and `twdll.toml`."""
    removed: list[Path] = []
    for step in target.steps():
        step_dir = ctx.paths.step_dir(step)
        if step_dir.exists():
            shutil.rmtree(step_dir)
            removed.append(step_dir)
    return CleanResult(target=target, removed_directories=tuple(removed))


def inspect_step_directories(paths: WorkspacePaths) -> tuple[StepSummary, ...]:
    """Summarize each `.twdll/<step>/` directory for `twdll status`."""
    summaries: list[StepSummary] = []
    for step in StepName:
        step_dir = paths.step_dir(step)
        if step_dir.is_dir():
            entries = tuple(sorted(child.name for child in step_dir.iterdir()))
            summaries.append(
                StepSummary(step=step, directory=step_dir, exists=True, entries=entries)
            )
        else:
            summaries.append(StepSummary(step=step, directory=step_dir, exists=False, entries=()))
    return tuple(summaries)


def _default_download(
    url: str,
    destination: Path,
    on_progress: ProgressCallback = noop_progress,
) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    task_key = f"download:{destination.name}"
    label = f"Downloading {destination.name}"
    with urllib.request.urlopen(url) as response, destination.open("wb") as out_file:
        raw_len = response.headers.get("Content-Length")
        total = int(raw_len) if raw_len is not None and raw_len.isdigit() else None
        completed = 0
        on_progress(ProgressEvent(ProgressKind.DOWNLOAD, task_key, label, 0, total))
        while chunk := response.read(64 * 1024):
            out_file.write(chunk)
            completed += len(chunk)
            on_progress(ProgressEvent(ProgressKind.DOWNLOAD, task_key, label, completed, total))


def _unpack_tar_gz(archive_path: Path, tools_dir: Path, on_progress: ProgressCallback) -> None:
    task_key = f"extract:{archive_path.name}"
    label = f"Extracting {archive_path.name}"
    with tarfile.open(archive_path, "r:gz") as tar:
        members = tar.getmembers()
        total = len(members)
        on_progress(ProgressEvent(ProgressKind.EXTRACT_ARCHIVE, task_key, label, 0, total))
        for idx, member in enumerate(members, start=1):
            tar.extract(member, tools_dir, set_attrs=True, filter="data")
            if idx == total or idx % 50 == 0:
                on_progress(
                    ProgressEvent(ProgressKind.EXTRACT_ARCHIVE, task_key, label, idx, total)
                )


def _extract_zip_member(zf: zipfile.ZipFile, info: zipfile.ZipInfo, tools_dir: Path) -> None:
    extracted = Path(zf.extract(info, tools_dir))
    unix_mode = (info.external_attr >> 16) & 0o777
    if unix_mode and extracted.is_file():
        extracted.chmod(unix_mode)


def _unpack_zip(archive_path: Path, tools_dir: Path, on_progress: ProgressCallback) -> None:
    task_key = f"extract:{archive_path.name}"
    label = f"Extracting {archive_path.name}"
    with zipfile.ZipFile(archive_path, "r") as zf:
        members = zf.infolist()
        total = len(members)
        on_progress(ProgressEvent(ProgressKind.EXTRACT_ARCHIVE, task_key, label, 0, total))
        for idx, info in enumerate(members, start=1):
            _extract_zip_member(zf, info, tools_dir)
            if idx == total or idx % 50 == 0:
                on_progress(
                    ProgressEvent(ProgressKind.EXTRACT_ARCHIVE, task_key, label, idx, total)
                )


def unpack_portable_archive(
    archive_path: Path,
    tools_dir: Path,
    on_progress: ProgressCallback = noop_progress,
) -> None:
    """Extract `.tar.gz` (with `filter='data'`) or `.zip` into `tools_dir` with progress."""
    if archive_path.name.endswith(".tar.gz"):
        _unpack_tar_gz(archive_path, tools_dir, on_progress)
        return
    _unpack_zip(archive_path, tools_dir, on_progress)


def _download_and_extract(
    spec: PortableArchiveSpec,
    tools_dir: Path,
    downloader: Downloader,
    on_progress: ProgressCallback,
) -> None:
    tools_dir.mkdir(parents=True, exist_ok=True)
    archive_path = tools_dir / spec.archive_name
    downloader(spec.url, archive_path, on_progress)
    try:
        unpack_portable_archive(archive_path, tools_dir, on_progress)
    finally:
        if archive_path.is_file():
            archive_path.unlink()


def provision_toolchain(
    paths: WorkspacePaths,
    config: WorkspaceConfig,
    host_os: HostOS,
    downloader: Downloader = _default_download,
    on_progress: ProgressCallback = noop_progress,
) -> ToolchainStatus:
    """Provision missing portable Ghidra 12.2 + JDK 25 into `.twdll/init/tools/`."""
    current = resolve_toolchain(paths=paths, config=config, host_os=host_os)
    if current.ready or config.decompiler is not Decompiler.GHIDRA:
        return current

    by_name = {tool.name: tool for tool in current.tools}
    if not by_name[ToolName.GHIDRA].ready and config.toolchain.ghidra_dir is None:
        _download_and_extract(GHIDRA_PORTABLE_SPEC, paths.tools_dir, downloader, on_progress)
    if not by_name[ToolName.JAVA].ready and config.toolchain.java_home is None:
        _download_and_extract(JDK_PORTABLE_SPECS[host_os], paths.tools_dir, downloader, on_progress)

    return resolve_toolchain(paths=paths, config=config, host_os=host_os)


def _query_winreg_key(
    mod_dict: dict[str, object],
    root_name: str,
    subkey: str,
    value_name: str,
) -> Path | None:
    root_key = mod_dict.get(root_name)
    open_fn = mod_dict.get("OpenKey")
    query_fn = mod_dict.get("QueryValueEx")
    if root_key is None or not callable(open_fn) or not callable(query_fn):
        return None
    open_key = cast("Callable[[object, str], AbstractContextManager[object]]", open_fn)
    query_val = cast("Callable[[object, str], tuple[object, object]]", query_fn)
    try:
        with open_key(root_key, subkey) as handle:
            raw_val, _ = query_val(handle, value_name)
            return optional_path(raw_val)
    except OSError:
        return None


def _windows_registry_steam_roots() -> tuple[Path, ...]:
    try:
        winreg_mod = importlib.import_module("winreg")
    except ImportError:
        return ()
    mod_dict = vars(winreg_mod)
    queries = (
        ("HKEY_CURRENT_USER", r"Software\Valve\Steam", "SteamPath"),
        ("HKEY_LOCAL_MACHINE", r"SOFTWARE\WOW6432Node\Valve\Steam", "InstallPath"),
        ("HKEY_LOCAL_MACHINE", r"SOFTWARE\Valve\Steam", "InstallPath"),
    )
    paths = (_query_winreg_key(mod_dict, r, sub, val) for r, sub, val in queries)
    return tuple(p for p in paths if p is not None)


def candidate_steam_roots(
    host_os: HostOS,
    home_dir: Path | None = None,
    extra_roots: Sequence[Path] | None = None,
) -> tuple[Path, ...]:
    """Return deduplicated candidate Steam root directories for `host_os`."""
    resolved_home = home_dir if home_dir is not None else Path.home()
    candidates: list[Path] = list(extra_roots) if extra_roots else []
    if host_os is HostOS.WINDOWS:
        candidates.extend(_windows_registry_steam_roots())
    candidates.extend(default_steam_roots(host_os, resolved_home))
    return tuple(dict.fromkeys(candidates))


def _parse_vdf_dict(text: str) -> dict[str, object]:
    loads_fn = cast("_VdfLoads", vdf.loads)
    return loads_fn(text)


def _extract_vdf_library_path(entry: object) -> Path | None:
    if isinstance(entry, dict):
        entry_dict = cast("dict[str, object]", entry)
        raw = entry_dict.get("path")
    elif isinstance(entry, str):
        raw = entry
    else:
        return None
    if not isinstance(raw, str) or not raw.strip():
        return None
    return Path(raw.strip().replace("\\\\", "\\"))


def parse_library_folders(vdf_text: str) -> tuple[Path, ...]:
    """Parse Steam `libraryfolders.vdf` text into library root `Path`s."""
    try:
        parsed = _parse_vdf_dict(vdf_text)
    except Exception:
        return ()
    folders_obj = parsed.get("libraryfolders") or parsed.get("LibraryFolders")
    if not isinstance(folders_obj, dict):
        return ()
    folders = cast("dict[str, object]", folders_obj)
    extracted = (_extract_vdf_library_path(entry) for entry in folders.values())
    return tuple(p for p in extracted if p is not None)


def discover_steamapps_dirs(steam_roots: Sequence[Path]) -> tuple[Path, ...]:
    """Expand Steam root directories into existing `steamapps` directories."""
    candidates: list[Path] = []
    for root in steam_roots:
        candidates.append(root / "steamapps")
        for rel in ("steamapps/libraryfolders.vdf", "config/libraryfolders.vdf"):
            vdf_file = root / rel
            if vdf_file.is_file():
                text = vdf_file.read_text(encoding="utf-8", errors="replace")
                candidates.extend(lib / "steamapps" for lib in parse_library_folders(text))
    return tuple(dict.fromkeys(d for d in candidates if d.is_dir()))


def parse_acf_installdir(acf_text: str) -> str | None:
    """Extract `installdir` from a Steam `appmanifest_<appid>.acf` file's text."""
    try:
        parsed = _parse_vdf_dict(acf_text)
    except Exception:
        return None
    app_state_obj = parsed.get("AppState") or parsed.get("appstate")
    if not isinstance(app_state_obj, dict):
        return None
    app_state = cast("dict[str, object]", app_state_obj)
    installdir = app_state.get("installdir")
    if isinstance(installdir, str) and installdir.strip():
        return installdir.strip()
    return None


def _candidate_binary_paths_in_library(steamapps_dir: Path, spec: BinarySpec) -> tuple[Path, ...]:
    candidates: list[Path] = []
    acf_path = steamapps_dir / f"appmanifest_{spec.steam_app_id}.acf"
    if acf_path.is_file():
        acf_dir = parse_acf_installdir(acf_path.read_text(encoding="utf-8", errors="replace"))
        if acf_dir:
            candidates.append(steamapps_dir / "common" / acf_dir / spec.file_name)

    candidates.append(steamapps_dir / "common" / spec.default_install_dir / spec.file_name)

    content_dir = steamapps_dir / "content" / f"app_{spec.steam_app_id}"
    if content_dir.is_dir():
        depots = sorted(
            (d for d in content_dir.iterdir() if d.is_dir() and d.name.startswith("depot_")),
            key=lambda p: p.name,
        )
        candidates.extend(depot / spec.file_name for depot in depots)
    return tuple(dict.fromkeys(candidates))


def find_binary_in_steam(spec: BinarySpec, steamapps_dirs: Sequence[Path]) -> Path | None:
    """Find the first header-verified binary matching `spec` across `steamapps_dirs`."""
    for steamapps_dir in steamapps_dirs:
        for candidate in _candidate_binary_paths_in_library(steamapps_dir, spec):
            if verify_binary_header(candidate, spec.arch):
                return candidate
    return None


def resolve_binary(
    spec: BinarySpec,
    configured_path: Path | None,
    staged_path: Path,
    steamapps_dirs: Sequence[Path],
) -> BinaryStatus:
    """Read-only resolution of a single binary from config, Steam, or existing staged file."""
    discovered: Path | None = None
    source = BinarySource.MISSING
    header_ok = False

    if configured_path is not None:
        header_ok = verify_binary_header(configured_path, spec.arch)
        discovered = configured_path if header_ok else None
        source = BinarySource.TOML if header_ok else BinarySource.TOML_INVALID
    else:
        steam_match = find_binary_in_steam(spec, steamapps_dirs)
        if steam_match is not None:
            discovered = steam_match
            header_ok = True
            source = BinarySource.STEAM

    staged_ok = verify_binary_header(staged_path, spec.arch)
    if staged_ok and source is BinarySource.MISSING:
        source = BinarySource.STAGED

    return BinaryStatus(
        name=spec.name,
        arch=spec.arch,
        staged=staged_ok,
        staged_path=staged_path if staged_ok else None,
        discovered_path=discovered,
        source=source,
        valid_header=header_ok or staged_ok,
    )


def stage_binary(
    binary_status: BinaryStatus,
    target_path: Path,
    arch: BinaryArch,
    on_progress: ProgressCallback = noop_progress,
) -> BinaryStatus:
    """Copy a discovered valid binary into `.twdll/init/binaries/<name>/` and return status."""
    if binary_status.discovered_path is None:
        return binary_status
    task_key = f"stage:{binary_status.name.value}"
    label = f"Staging {binary_status.name.value} ({target_path.name})"
    on_progress(ProgressEvent(ProgressKind.STAGE_BINARY, task_key, label, 0, 1))
    target_path.parent.mkdir(parents=True, exist_ok=True)
    if not target_path.exists() or target_path.resolve() != binary_status.discovered_path.resolve():
        shutil.copy2(binary_status.discovered_path, target_path)
    staged_ok = verify_binary_header(target_path, arch)
    on_progress(ProgressEvent(ProgressKind.STAGE_BINARY, task_key, label, 1, 1))
    return BinaryStatus(
        name=binary_status.name,
        arch=binary_status.arch,
        staged=staged_ok,
        staged_path=target_path if staged_ok else None,
        discovered_path=binary_status.discovered_path,
        source=binary_status.source,
        valid_header=binary_status.valid_header or staged_ok,
    )


def resolve_all_binaries(
    paths: WorkspacePaths,
    config: WorkspaceConfig,
    steamapps_dirs: Sequence[Path],
    stage: bool,
    on_progress: ProgressCallback = noop_progress,
) -> tuple[BinaryStatus, ...]:
    """Resolve all 4 Warscape binaries and optionally stage discovered ones."""
    results: list[BinaryStatus] = []
    for name, spec in BINARY_SPECS.items():
        staged_path = paths.staged_binary_path(name)
        item = resolve_binary(spec, config.binaries.get(name), staged_path, steamapps_dirs)
        if stage:
            item = stage_binary(item, staged_path, spec.arch, on_progress=on_progress)
        results.append(item)
    return tuple(results)


def status(
    ctx: WorkspaceContext,
    steam_roots: Sequence[Path] | None = None,
    ida_dirs: Sequence[Path] | None = None,
) -> WorkspaceStatus:
    """Read-only workspace status inspection (`twdll status`)."""
    roots = tuple(steam_roots) if steam_roots is not None else candidate_steam_roots(ctx.host_os)
    steamapps_dirs = discover_steamapps_dirs(roots)
    return WorkspaceStatus(
        root=ctx.root,
        config_file=ctx.paths.config_file,
        config_exists=ctx.paths.config_file.is_file(),
        host_os=ctx.host_os,
        decompiler=ctx.decompiler,
        toolchain=resolve_toolchain(ctx.paths, ctx.config, ctx.host_os, ida_dirs=ida_dirs),
        binaries=resolve_all_binaries(ctx.paths, ctx.config, steamapps_dirs, stage=False),
        steps=inspect_step_directories(ctx.paths),
    )


def init(
    ctx: WorkspaceContext,
    *,
    persist_config: bool = False,
    skip_toolchain: bool = False,
    skip_binaries: bool = False,
    on_progress: ProgressCallback = noop_progress,
) -> InitResult:
    """Initialize `twdll.toml`, `.twdll/init/tools/`, and `.twdll/init/binaries/` (`twdll init`)."""
    config_existed = ctx.paths.config_file.is_file()
    if not config_existed or persist_config:
        save_config(ctx.root, ctx.config)

    ctx.paths.init_dir.mkdir(parents=True, exist_ok=True)
    toolchain = (
        resolve_toolchain(ctx.paths, ctx.config, ctx.host_os)
        if skip_toolchain
        else provision_toolchain(ctx.paths, ctx.config, ctx.host_os, on_progress=on_progress)
    )
    steamapps_dirs = discover_steamapps_dirs(candidate_steam_roots(ctx.host_os))
    binaries = resolve_all_binaries(
        ctx.paths,
        ctx.config,
        steamapps_dirs,
        stage=not skip_binaries,
        on_progress=on_progress,
    )
    return InitResult(
        config_file=ctx.paths.config_file,
        config_created=not config_existed,
        decompiler=ctx.decompiler,
        toolchain=toolchain,
        binaries=binaries,
    )
