"""Parametrized cross-platform tests (Linux & Windows) for Slice 1: init, status, clean."""

from __future__ import annotations

import json
import os
import shutil
import struct
import subprocess
import tarfile
import zipfile
from dataclasses import replace
from pathlib import Path
from typing import cast

import pytest
from typer.testing import CliRunner

import twdll.init as init_mod
from twdll.cli import app, parse_target_selection
from twdll.database import list_sqlite_tables
from twdll.domain import (
    BINARY_SPECS,
    SELECTABLE_TARGETS,
    STEAM_CONSOLE_URL,
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
    WorkspaceContext,
    active_binaries,
    compute_file_sha256,
    normalize_targets,
    resolve_toolchain,
    verify_binary_header,
)
from twdll.init import (
    candidate_steam_roots,
    clean,
    copy_to_clipboard,
    discover_steamapps_dirs,
    init,
    open_steam_console,
    provision_toolchain,
    resolve_all_binaries,
    save_config,
    status,
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

    ctx = replace(WorkspaceContext.from_root(workspace_root), host_os=host_os)
    paths = ctx.paths
    binaries = resolve_all_binaries(ctx, steamapps_dirs, stage=True)
    by_name = {b.name: b for b in binaries}

    for found in (
        BinaryName.ATTILA_LINUX_64,
        BinaryName.ATTILA_WINDOWS_32,
        BinaryName.ROME2_WINDOWS_32,
    ):
        assert by_name[found].staged is True
        assert by_name[found].source is BinarySource.STEAM

    assert by_name[BinaryName.TOB_WINDOWS_32].staged is False
    assert by_name[BinaryName.TOB_WINDOWS_32].source is BinarySource.MISSING
    assert verify_binary_header(
        paths.staged_binary_path(BinaryName.ATTILA_LINUX_64),
        BinaryArch.ELF64,
    )
    assert verify_binary_header(
        paths.staged_binary_path(BinaryName.ATTILA_WINDOWS_32),
        BinaryArch.PE32,
    )
    assert verify_binary_header(
        paths.staged_binary_path(BinaryName.ROME2_WINDOWS_32),
        BinaryArch.PE32,
    )
    assert paths.staged_binary_path(BinaryName.ATTILA_LINUX_64).parent == paths.binaries_dir
    assert paths.staged_binary_path(BinaryName.ATTILA_WINDOWS_32).parent == paths.binaries_dir


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
    ghidra_ctx = replace(
        WorkspaceContext.from_root(tmp_path / "workspace"),
        config=WorkspaceConfig(decompiler=Decompiler.GHIDRA),
        host_os=host_os,
    )
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
        ctx=ghidra_ctx,
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

    ida_ctx = replace(
        ghidra_ctx,
        config=WorkspaceConfig(decompiler=Decompiler.IDA, toolchain=ToolchainConfig()),
    )
    ida_status = resolve_toolchain(
        ctx=ida_ctx,
        ida_dirs=(ida_default_dir,),
    )
    assert ida_status.ready is True
    assert ida_status.tools[0].source is ToolSource.DEFAULT_DIR
    assert ida_status.tools[0].path == ida_default_dir

    ws_status = status(
        ghidra_ctx,
        steam_roots=(),
        ida_dirs=(ida_default_dir,),
    )
    assert ws_status.host_os is host_os


def test_step_directory_clean_mapping(tmp_path: Path) -> None:
    ctx = WorkspaceContext.from_root(tmp_path)
    paths = ctx.paths
    save_config(ctx)

    (paths.uv_dir / "bin").mkdir(parents=True)
    for step in StepName:
        step_dir = paths.step_dir(step)
        step_dir.mkdir(parents=True)
        (step_dir / "marker.txt").write_text(step.value, encoding="utf-8")

    res_analyze = clean(ctx, CleanTarget.ANALYZE)
    assert res_analyze.removed_directories == (paths.analyze_dir,)
    assert not paths.analyze_dir.exists()
    assert paths.init_dir.exists()
    assert paths.extract_dir.exists()
    assert paths.derive_dir.exists()
    assert paths.uv_dir.exists()

    res_all = clean(ctx, CleanTarget.ALL)
    assert len(res_all.removed_directories) == 3
    for step in StepName:
        assert not paths.step_dir(step).exists()
    assert paths.uv_dir.exists()
    assert paths.config_file.is_file()


def test_workspace_context_from_root_and_cli_overrides(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    def fake_steam_roots(_host_os: HostOS) -> tuple[Path, ...]:
        return ()

    monkeypatch.setattr(init_mod, "candidate_steam_roots", fake_steam_roots)
    ctx = WorkspaceContext.from_root(tmp_path)
    assert ctx.root == tmp_path
    assert ctx.decompiler is Decompiler.GHIDRA

    ida_ctx = replace(ctx, config=replace(ctx.config, decompiler=Decompiler.IDA))
    assert ida_ctx.decompiler is Decompiler.IDA

    monkeypatch.chdir(tmp_path)
    override_out = runner.invoke(
        app,
        ["--decompiler", Decompiler.IDA.value, "status", "--json"],
    )
    assert override_out.exit_code == 0
    payload = cast("dict[str, object]", json.loads(override_out.stdout))
    assert payload["decompiler"] == Decompiler.IDA.value


def test_cli_init_status_clean_json(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_steam_roots(_host_os: HostOS) -> tuple[Path, ...]:
        return ()

    monkeypatch.setattr(init_mod, "candidate_steam_roots", fake_steam_roots)
    monkeypatch.chdir(tmp_path)
    init_out = runner.invoke(
        app,
        [
            "init",
            "--no-toolchain",
            "--no-binaries",
            "--json",
        ],
    )
    assert init_out.exit_code == 0
    init_payload = cast("dict[str, object]", json.loads(init_out.stdout))
    assert init_payload["config_created"] is True
    assert init_payload["decompiler"] == Decompiler.GHIDRA.value
    assert WorkspaceContext.from_root(tmp_path).decompiler is Decompiler.GHIDRA

    status_out = runner.invoke(
        app,
        ["status", "--json"],
    )
    assert status_out.exit_code == 0
    status_payload = cast("dict[str, object]", json.loads(status_out.stdout))
    assert status_payload["config_exists"] is True

    clean_out = runner.invoke(
        app,
        ["clean", CleanTarget.INIT.value, "--json"],
    )
    assert clean_out.exit_code == 0
    clean_payload = cast("dict[str, object]", json.loads(clean_out.stdout))
    assert clean_payload["target"] == CleanTarget.INIT.value
    removed = cast("list[str]", clean_payload["removed_directories"])
    assert len(removed) == 1
    assert list_sqlite_tables(tmp_path / "nonexistent.sqlite") == ()


def test_init_launches_steam_console_and_copies_depot_command_when_attila_linux_missing(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    launched: list[tuple[str, HostOS]] = []

    def fake_open_depot_console(depot_command: str, host_os: HostOS) -> tuple[bool, bool]:
        launched.append((depot_command, host_os))
        return True, True

    def fake_steam_roots(_host_os: HostOS) -> tuple[Path, ...]:
        return ()

    monkeypatch.setattr(init_mod, "open_depot_console", fake_open_depot_console)
    monkeypatch.setattr(init_mod, "candidate_steam_roots", fake_steam_roots)

    ctx = WorkspaceContext.from_root(tmp_path)
    res = init(ctx, skip_toolchain=True, skip_binaries=False)
    expected_cmd = BINARY_SPECS[BinaryName.ATTILA_LINUX_64].depot_command
    assert expected_cmd is not None
    assert launched == [(expected_cmd, ctx.host_os)]

    by_name = {b.name: b for b in res.binaries}
    attila_linux = by_name[BinaryName.ATTILA_LINUX_64]
    assert attila_linux.staged is False
    assert attila_linux.depot_command == expected_cmd
    assert attila_linux.instruction is not None
    assert STEAM_CONSOLE_URL in attila_linux.instruction
    assert expected_cmd in attila_linux.instruction
    assert "copied" in attila_linux.instruction
    assert by_name[BinaryName.TOB_WINDOWS_32].depot_command is not None
    assert by_name[BinaryName.TOB_WINDOWS_32].instruction is not None


def test_init_stages_binaries_before_provisioning_toolchain(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    order: list[str] = []

    def fake_steam_roots(_host_os: HostOS) -> tuple[Path, ...]:
        return ()

    def fake_open_depot_console(_cmd: str, _host_os: HostOS) -> tuple[bool, bool]:
        order.append("binaries")
        return True, True

    def fake_provision_toolchain(
        ctx: WorkspaceContext,
        on_progress: ProgressCallback = init_mod.noop_progress,
    ) -> object:
        _ = on_progress
        order.append("toolchain")
        return resolve_toolchain(ctx)

    monkeypatch.setattr(init_mod, "candidate_steam_roots", fake_steam_roots)
    monkeypatch.setattr(init_mod, "open_depot_console", fake_open_depot_console)
    monkeypatch.setattr(init_mod, "provision_toolchain", fake_provision_toolchain)

    init(WorkspaceContext.from_root(tmp_path), skip_toolchain=False, skip_binaries=False)
    assert order == ["binaries", "toolchain"]


def test_open_steam_console_linux_uses_systemd_run(monkeypatch: pytest.MonkeyPatch) -> None:
    ran: list[tuple[str, ...]] = []

    def fake_which(name: str) -> str | None:
        mapping = {"steam": "/usr/bin/steam", "systemd-run": "/usr/bin/systemd-run"}
        return mapping.get(name)

    def fake_run(cmd: tuple[str, ...], **_kwargs: object) -> subprocess.CompletedProcess[str]:
        ran.append(cmd)
        return subprocess.CompletedProcess(cmd, 0)

    monkeypatch.setattr(shutil, "which", fake_which)
    monkeypatch.setattr(subprocess, "run", fake_run)

    assert open_steam_console(HostOS.LINUX) is True
    assert ran == [
        (
            "/usr/bin/systemd-run",
            "--user",
            "--collect",
            "--no-block",
            "--",
            "/usr/bin/steam",
            "-console",
            STEAM_CONSOLE_URL,
        )
    ]


def test_open_steam_console_windows_uses_startfile(monkeypatch: pytest.MonkeyPatch) -> None:
    started: list[str] = []

    def fake_startfile(url: str) -> None:
        started.append(url)

    monkeypatch.setattr(os, "startfile", fake_startfile, raising=False)

    assert open_steam_console(HostOS.WINDOWS) is True
    assert started == [STEAM_CONSOLE_URL]


@pytest.mark.parametrize(
    ("host_os", "expected_cmd"),
    [
        (HostOS.LINUX, ("xclip", "-selection", "clipboard")),
        (HostOS.WINDOWS, ("clip.exe",)),
    ],
)
def test_copy_to_clipboard_across_os(
    monkeypatch: pytest.MonkeyPatch,
    host_os: HostOS,
    expected_cmd: tuple[str, ...],
) -> None:
    calls: list[tuple[tuple[str, ...], object, object, object]] = []

    def fake_which(name: str) -> str | None:
        return "/usr/bin/xclip" if name == "xclip" else None

    def fake_run(
        cmd: tuple[str, ...],
        *,
        input: str,
        stdout: object,
        stderr: object,
        **_kwargs: object,
    ) -> subprocess.CompletedProcess[str]:
        calls.append((cmd, input, stdout, stderr))
        return subprocess.CompletedProcess(cmd, 0)

    monkeypatch.setattr(shutil, "which", fake_which)
    monkeypatch.setattr(subprocess, "run", fake_run)

    assert copy_to_clipboard("download_depot 325610 325624 3804137149572368482", host_os) is True
    assert calls == [
        (
            expected_cmd,
            "download_depot 325610 325624 3804137149572368482",
            subprocess.DEVNULL,
            subprocess.DEVNULL,
        )
    ]


def test_targets_config_roundtrip_and_active_binaries(tmp_path: Path) -> None:
    ctx = replace(
        WorkspaceContext.from_root(tmp_path),
        config=WorkspaceConfig(targets=(BinaryName.ROME2_WINDOWS_32,)),
    )
    save_config(ctx)

    reloaded = WorkspaceContext.from_root(tmp_path)
    assert reloaded.config.targets == (BinaryName.ROME2_WINDOWS_32,)
    assert active_binaries(reloaded.config) == (
        BinaryName.ATTILA_LINUX_64,
        BinaryName.ROME2_WINDOWS_32,
    )
    resolved = resolve_all_binaries(reloaded, (), stage=False)
    assert tuple(b.name for b in resolved) == (
        BinaryName.ATTILA_LINUX_64,
        BinaryName.ROME2_WINDOWS_32,
    )

    with pytest.raises(ValueError, match="Select at least one target"):
        normalize_targets((BinaryName.ATTILA_LINUX_64,))


def test_linux_steamrt64_depot_auto_discovery(tmp_path: Path) -> None:
    steam_root = tmp_path / "Steam"
    depot_bin = (
        steam_root
        / "steamrt64"
        / "steamapps"
        / "content"
        / "app_325610"
        / "depot_325624"
        / "Attila"
    )
    make_fake_elf64(depot_bin)

    steamapps_dirs = discover_steamapps_dirs((steam_root,), host_os=HostOS.LINUX)
    ctx = replace(
        WorkspaceContext.from_root(tmp_path / "ws"),
        config=WorkspaceConfig(targets=(BinaryName.ROME2_WINDOWS_32,)),
        host_os=HostOS.LINUX,
    )
    ctx.root.mkdir()
    resolved = resolve_all_binaries(ctx, steamapps_dirs, stage=True)
    by_name = {b.name: b for b in resolved}
    assert by_name[BinaryName.ATTILA_LINUX_64].staged is True
    assert by_name[BinaryName.ATTILA_LINUX_64].source is BinarySource.STEAM
    assert by_name[BinaryName.ATTILA_LINUX_64].discovered_path == depot_bin


def test_wizard_parse_target_selection() -> None:
    defaults = (BinaryName.ATTILA_WINDOWS_32,)
    assert parse_target_selection("", defaults) == defaults
    assert parse_target_selection("all", defaults) == SELECTABLE_TARGETS
    assert parse_target_selection("2", defaults) == (BinaryName.ROME2_WINDOWS_32,)
    assert parse_target_selection("1, 3", defaults) == (
        BinaryName.ATTILA_WINDOWS_32,
        BinaryName.TOB_WINDOWS_32,
    )
    assert parse_target_selection(BinaryName.ROME2_WINDOWS_32.value, defaults) == (
        BinaryName.ROME2_WINDOWS_32,
    )
    assert parse_target_selection(BinaryName.ATTILA_LINUX_64.value, defaults) is None
    assert parse_target_selection("99", defaults) is None


def test_binary_sha256_verification_and_mismatch_warning(tmp_path: Path) -> None:
    elf_file = make_fake_elf64(tmp_path / "Attila")
    actual_sha = compute_file_sha256(elf_file)
    assert actual_sha is not None

    base_spec = BINARY_SPECS[BinaryName.ATTILA_LINUX_64]
    matched_spec = replace(base_spec, expected_sha256=actual_sha)
    staged_path = tmp_path / "staged_attila"
    staged_path.write_bytes(elf_file.read_bytes())

    ok_status = init_mod.resolve_binary(matched_spec, None, staged_path, ())
    assert ok_status.staged is True
    assert ok_status.sha256 == actual_sha
    assert ok_status.sha256_match is True
    assert ok_status.instruction is None

    mismatch_spec = replace(base_spec, expected_sha256="0" * 64)
    bad_status = init_mod.resolve_binary(mismatch_spec, None, staged_path, ())
    assert bad_status.staged is True
    assert bad_status.sha256 == actual_sha
    assert bad_status.sha256_match is False
    assert bad_status.instruction is not None
    assert "SHA-256 mismatch" in bad_status.instruction
    assert "updated" in bad_status.instruction


def test_stage_binary_skips_copy_when_unchanged(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    src = make_fake_pe32(tmp_path / "src" / "empire.retail.dll")
    dst = tmp_path / "staged" / "rome2_windows_32.dll"
    spec = BINARY_SPECS[BinaryName.ROME2_WINDOWS_32]

    first_status = init_mod.resolve_binary(spec, src, dst, ())
    staged = init_mod.stage_binary(first_status, dst, spec.arch)
    assert staged.staged is True

    copy_calls: list[tuple[Path, Path]] = []

    def fail_copy2(s: Path, d: Path) -> None:
        copy_calls.append((s, d))

    monkeypatch.setattr(shutil, "copy2", fail_copy2)
    second_status = init_mod.resolve_binary(spec, src, dst, ())
    res = init_mod.stage_binary(second_status, dst, spec.arch)
    assert res.staged is True
    assert copy_calls == []
