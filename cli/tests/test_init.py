"""Parametrized cross-platform tests (Linux & Windows) for Slice 1: init, status, clean."""

from __future__ import annotations

import json
import struct
import tarfile
import zipfile
from pathlib import Path
from typing import cast

import pytest
from typer.testing import CliRunner

from twdll.cli import app
from twdll.database import list_sqlite_tables
from twdll.domain import (
    BinaryArch,
    BinaryName,
    BinarySource,
    CleanTarget,
    Decompiler,
    HostOS,
    ProgressCallback,
    ProgressEvent,
    ProgressKind,
    StepName,
    ToolchainConfig,
    ToolSource,
    WorkspaceConfig,
    WorkspacePaths,
)
from twdll.init import (
    candidate_steam_roots,
    clean,
    discover_steamapps_dirs,
    load_config,
    provision_toolchain,
    resolve_all_binaries,
    resolve_toolchain,
    save_config,
    status,
    verify_binary_header,
)

runner = CliRunner()


def make_fake_elf64(path: Path) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    header = bytearray(64)
    header[0:4] = b"\x7fELF"
    header[4] = 2  # ELFCLASS64
    header[5] = 1  # ELFDATA2LSB
    struct.pack_into("<H", header, 18, 0x3E)  # EM_X86_64
    path.write_bytes(bytes(header))
    return path


def make_fake_pe32(path: Path) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    header = bytearray(128)
    header[0:2] = b"MZ"
    struct.pack_into("<I", header, 0x3C, 64)
    header[64:68] = b"PE\x00\x00"
    struct.pack_into("<H", header, 68, 0x014C)  # IMAGE_FILE_MACHINE_I386
    path.write_bytes(bytes(header))
    return path


def _populate_fake_steam_libraries(steam_root: Path, second_library: Path) -> None:
    steamapps = steam_root / "steamapps"
    steamapps.mkdir(parents=True)
    (second_library / "steamapps").mkdir(parents=True)

    vdf_text = (
        '"libraryfolders"\n{\n'
        f'  "0"\n  {{\n    "path" "{steam_root.as_posix()}"\n  }}\n'
        f'  "1"\n  {{\n    "path" "{second_library.as_posix()}"\n  }}\n'
        "}\n"
    )
    (steamapps / "libraryfolders.vdf").write_text(vdf_text, encoding="utf-8")

    make_fake_elf64(steamapps / "common" / "Total War Attila" / "Attila")
    make_fake_pe32(steamapps / "content" / "app_325610" / "depot_325612" / "empire.retail.dll")

    acf_text = '"AppState"\n{\n  "appid" "214950"\n  "installdir" "Custom Rome II Folder"\n}\n'
    (second_library / "steamapps" / "appmanifest_214950.acf").write_text(acf_text, encoding="utf-8")
    make_fake_pe32(
        second_library / "steamapps" / "common" / "Custom Rome II Folder" / "empire.retail.dll"
    )


def test_verify_binary_header(tmp_path: Path) -> None:
    elf_file = make_fake_elf64(tmp_path / "Attila")
    pe_file = make_fake_pe32(tmp_path / "empire.retail.dll")
    bad_file = tmp_path / "corrupt.bin"
    bad_file.write_bytes(b"not a binary")

    assert verify_binary_header(elf_file, BinaryArch.ELF64) is True
    assert verify_binary_header(elf_file, BinaryArch.PE32) is False
    assert verify_binary_header(pe_file, BinaryArch.PE32) is True
    assert verify_binary_header(pe_file, BinaryArch.ELF64) is False
    assert verify_binary_header(bad_file, BinaryArch.ELF64) is False
    assert verify_binary_header(bad_file, BinaryArch.PE32) is False


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_steam_discovery_and_staging_across_os(tmp_path: Path, host_os: HostOS) -> None:
    workspace_root = tmp_path / "workspace"
    workspace_root.mkdir()
    home_dir = tmp_path / "home"
    home_dir.mkdir()

    steam_root = (
        home_dir / ".local" / "share" / "Steam"
        if host_os is HostOS.LINUX
        else tmp_path / "ProgramFilesSteam"
    )
    _populate_fake_steam_libraries(steam_root, tmp_path / "steam_lib_2")

    extra = (steam_root,) if host_os is HostOS.WINDOWS else None
    roots = candidate_steam_roots(host_os, home_dir=home_dir, extra_roots=extra)
    steamapps_dirs = discover_steamapps_dirs(roots)

    paths = WorkspacePaths(workspace_root)
    binaries = resolve_all_binaries(paths, WorkspaceConfig(), steamapps_dirs, stage=True)
    by_name = {b.name: b for b in binaries}

    for found in (BinaryName.ATTILA_64, BinaryName.ATTILA_32, BinaryName.ROME2_32):
        assert by_name[found].staged is True
        assert by_name[found].source is BinarySource.STEAM

    assert by_name[BinaryName.TOB_32].staged is False
    assert by_name[BinaryName.TOB_32].source is BinarySource.MISSING
    assert verify_binary_header(paths.staged_binary_path(BinaryName.ATTILA_64), BinaryArch.ELF64)
    assert verify_binary_header(paths.staged_binary_path(BinaryName.ATTILA_32), BinaryArch.PE32)
    assert verify_binary_header(paths.staged_binary_path(BinaryName.ROME2_32), BinaryArch.PE32)


def _write_fake_tool_archive(
    url: str,
    destination: Path,
    host_os: HostOS,
    scratch_dir: Path,
) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    if destination.name.endswith(".tar.gz"):
        exe = "java.exe" if host_os is HostOS.WINDOWS else "java"
        inner_file = scratch_dir / exe
        inner_file.write_text("echo java", encoding="utf-8")
        with tarfile.open(destination, "w:gz") as tar:
            tar.add(inner_file, arcname=f"jdk-25.0.5.1+2/bin/{exe}")
        return

    with zipfile.ZipFile(destination, "w") as archive:
        if "ghidra" in url.lower():
            script = "analyzeHeadless.bat" if host_os is HostOS.WINDOWS else "analyzeHeadless"
            archive.writestr(f"ghidra_12.2_PUBLIC/support/{script}", "echo ghidra")
        else:
            exe = "java.exe" if host_os is HostOS.WINDOWS else "java"
            archive.writestr(f"jdk-25.0.5.1+2/bin/{exe}", "echo java")


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_toolchain_provisioning_and_resolution_without_path(
    tmp_path: Path, host_os: HostOS
) -> None:
    paths = WorkspacePaths(tmp_path / "workspace")
    events: list[ProgressEvent] = []

    def downloader(url: str, destination: Path, on_progress: ProgressCallback) -> None:
        on_progress(
            ProgressEvent(
                kind=ProgressKind.DOWNLOAD,
                task_key=f"download:{destination.name}",
                label=f"Downloading {destination.name}",
                completed=1024,
                total=1024,
            )
        )
        _write_fake_tool_archive(url, destination, host_os, tmp_path)

    ghidra_status = provision_toolchain(
        paths=paths,
        config=WorkspaceConfig(decompiler=Decompiler.GHIDRA),
        host_os=host_os,
        downloader=downloader,
        on_progress=events.append,
    )
    assert ghidra_status.ready is True
    assert any(e.kind is ProgressKind.DOWNLOAD for e in events)
    assert any(e.kind is ProgressKind.EXTRACT_ARCHIVE for e in events)
    for tool in ghidra_status.tools:
        assert tool.ready is True
        assert tool.source is ToolSource.PORTABLE

    ida_default_dir = tmp_path / "ida95"
    ida_default_dir.mkdir()
    ida_bin = "idat64.exe" if host_os is HostOS.WINDOWS else "idat64"
    (ida_default_dir / ida_bin).write_text("fake ida", encoding="utf-8")

    ida_status = resolve_toolchain(
        paths=paths,
        config=WorkspaceConfig(decompiler=Decompiler.IDA, toolchain=ToolchainConfig()),
        host_os=host_os,
        ida_dirs=(ida_default_dir,),
    )
    assert ida_status.ready is True
    assert ida_status.tools[0].source is ToolSource.DEFAULT_DIR
    assert ida_status.tools[0].path == ida_default_dir

    ws_status = status(
        paths.root,
        host_os=host_os,
        steam_roots=(),
        ida_dirs=(ida_default_dir,),
    )
    assert ws_status.host_os is host_os


def test_step_directory_clean_mapping(tmp_path: Path) -> None:
    paths = WorkspacePaths(tmp_path)
    save_config(tmp_path, WorkspaceConfig())

    (paths.uv_dir / "bin").mkdir(parents=True)
    for step in StepName:
        step_dir = paths.step_dir(step)
        step_dir.mkdir(parents=True)
        (step_dir / "marker.txt").write_text(step.value, encoding="utf-8")

    res_analyze = clean(tmp_path, CleanTarget.ANALYZE)
    assert res_analyze.removed_directories == (paths.analyze_dir,)
    assert not paths.analyze_dir.exists()
    assert paths.init_dir.exists()
    assert paths.extract_dir.exists()
    assert paths.derive_dir.exists()
    assert paths.uv_dir.exists()

    res_all = clean(tmp_path, CleanTarget.ALL)
    assert len(res_all.removed_directories) == 3
    for step in StepName:
        assert not paths.step_dir(step).exists()
    assert paths.uv_dir.exists()
    assert paths.config_file.is_file()


def test_cli_init_status_clean_json(tmp_path: Path) -> None:
    init_out = runner.invoke(
        app,
        [
            "init",
            "--no-toolchain",
            "--no-binaries",
            "--json",
            "--workspace",
            str(tmp_path),
        ],
    )
    assert init_out.exit_code == 0
    init_payload = cast("dict[str, object]", json.loads(init_out.stdout))
    assert init_payload["config_created"] is True
    assert init_payload["decompiler"] == "ghidra"
    assert load_config(tmp_path).decompiler is Decompiler.GHIDRA

    status_out = runner.invoke(
        app,
        ["status", "--json", "--workspace", str(tmp_path)],
    )
    assert status_out.exit_code == 0
    status_payload = cast("dict[str, object]", json.loads(status_out.stdout))
    assert status_payload["config_exists"] is True

    clean_out = runner.invoke(
        app,
        ["clean", "init", "--json", "--workspace", str(tmp_path)],
    )
    assert clean_out.exit_code == 0
    clean_payload = cast("dict[str, object]", json.loads(clean_out.stdout))
    assert clean_payload["target"] == "init"
    removed = cast("list[str]", clean_payload["removed_directories"])
    assert len(removed) == 1
    assert list_sqlite_tables(tmp_path / "nonexistent.sqlite") == ()
