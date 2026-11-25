"""Unit tests for `twdll.domain` DWARF models, staging paths, and exporter resolution."""

from __future__ import annotations

from dataclasses import replace
from pathlib import Path

import pytest

from twdll.domain import (
    DWARF_EXPORTER_BUILD_REL_DIR,
    DWARF_EXPORTER_DIR_NAME,
    DWARF_EXPORTER_EXECUTABLES,
    BinaryName,
    Decompiler,
    DwarfUnitRow,
    FileSuffix,
    HostOS,
    WorkspaceContext,
    resolve_dwarf_exporter,
    to_dict,
)


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_extract_staging_db_path(tmp_path: Path, host_os: HostOS) -> None:
    ctx = replace(WorkspaceContext.from_root(tmp_path), host_os=host_os)
    staging = ctx.paths.extract_staging_db_path(Decompiler.GHIDRA, BinaryName.ATTILA_LINUX_64)
    final = ctx.paths.extract_db_path(Decompiler.GHIDRA, BinaryName.ATTILA_LINUX_64)

    assert staging.parent == ctx.paths.extract_dir
    assert staging.name.endswith(FileSuffix.SQLITE_STAGING.value)
    assert final.name.endswith(FileSuffix.SQLITE.value)
    assert staging != final


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_resolve_dwarf_exporter_prefers_tools_dir_over_repo_build(
    tmp_path: Path,
    host_os: HostOS,
) -> None:
    ctx = replace(WorkspaceContext.from_root(tmp_path), host_os=host_os)
    exe = DWARF_EXPORTER_EXECUTABLES[host_os]

    repo_bin = tmp_path.joinpath(*DWARF_EXPORTER_BUILD_REL_DIR, exe)
    repo_bin.parent.mkdir(parents=True, exist_ok=True)
    repo_bin.write_bytes(b"repo-bin")

    assert resolve_dwarf_exporter(ctx) == repo_bin

    tools_bin = ctx.paths.tools_dir / DWARF_EXPORTER_DIR_NAME / exe
    tools_bin.parent.mkdir(parents=True, exist_ok=True)
    tools_bin.write_bytes(b"tools-bin")

    assert resolve_dwarf_exporter(ctx) == tools_bin


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_resolve_dwarf_exporter_missing_raises_with_searched_paths(
    tmp_path: Path,
    host_os: HostOS,
) -> None:
    ctx = replace(WorkspaceContext.from_root(tmp_path), host_os=host_os)
    exe = DWARF_EXPORTER_EXECUTABLES[host_os]
    with pytest.raises(RuntimeError, match=exe):
        resolve_dwarf_exporter(ctx)


def test_dwarf_row_serialization() -> None:
    unit = DwarfUnitRow(
        unit_id=1,
        name="Source/UnityEmpireCampaign9.cpp",
        comp_dir="/build",
        producer="Ubuntu clang version 3.4.2",
        language=4,
    )
    assert to_dict(unit)["unit_id"] == 1
