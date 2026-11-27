"""Unit tests for `twdll.domain` DWARF models, staging paths, and exporter resolution."""

from __future__ import annotations

import os
from dataclasses import FrozenInstanceError, replace
from pathlib import Path

import pytest

from twdll.domain import (
    DWARF_EXPORTER_BUILD_REL_DIR,
    DWARF_EXPORTER_DIR_NAME,
    DWARF_EXPORTER_EXECUTABLES,
    BinaryName,
    Decompiler,
    DwarfGlobalRow,
    DwarfInjectionReport,
    DwarfPassName,
    DwarfPassStats,
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
def test_resolve_dwarf_exporter_prefers_newer_build(
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

    # Equal mtime prefers repo build; newer tools build wins; newer repo build wins.
    os.utime(repo_bin, ns=(1_000_000_000, 1_000_000_000))
    os.utime(tools_bin, ns=(1_000_000_000, 1_000_000_000))
    assert resolve_dwarf_exporter(ctx) == repo_bin

    os.utime(tools_bin, ns=(2_000_000_000, 2_000_000_000))
    assert resolve_dwarf_exporter(ctx) == tools_bin

    os.utime(repo_bin, ns=(3_000_000_000, 3_000_000_000))
    assert resolve_dwarf_exporter(ctx) == repo_bin


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_resolve_dwarf_exporter_tools_only_fallback(
    tmp_path: Path,
    host_os: HostOS,
) -> None:
    ctx = replace(WorkspaceContext.from_root(tmp_path), host_os=host_os)
    exe = DWARF_EXPORTER_EXECUTABLES[host_os]
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


def test_dwarf_row_and_report_serialization() -> None:
    unit = DwarfUnitRow(
        unit_id=1,
        name="Source/UnityEmpireCampaign9.cpp",
        comp_dir="/build",
        producer="Ubuntu clang version 3.4.2",
        language=4,
    )
    assert to_dict(unit)["unit_id"] == 1

    global_row = DwarfGlobalRow(
        global_id=1,
        unit_id=1,
        address=0x57D0A0,
        linkage_name="_ZN17EMPIRE_APP_MODULE10s_instanceE",
        qualified_name="EMPIRE_APP_MODULE::s_instance",
        type_id=10,
        byte_size=8,
        is_static_member=True,
    )
    assert to_dict(global_row)["is_static_member"] is True

    p1 = DwarfPassStats(
        pass_name=DwarfPassName.PASS1_TYPES,
        counts=(("created", 12),),
        error_counts=(("enum_failed", 1),),
        sample_errors=("enum X: ValueError: bad",),
    )
    p2 = DwarfPassStats(pass_name=DwarfPassName.PASS2_LAYOUTS)
    p3 = DwarfPassStats(pass_name=DwarfPassName.PASS3_SIGNATURES)
    p4 = DwarfPassStats(pass_name=DwarfPassName.PASS4_LOCALS)
    report = DwarfInjectionReport(pass1=p1, pass2=p2, pass3=p3, pass4=p4, total_errors=1)

    assert p1.to_dict() == {
        "pass_name": "pass1_types",
        "counts": {"created": 12},
        "error_counts": {"enum_failed": 1},
        "sample_errors": ["enum X: ValueError: bad"],
    }
    assert report.to_dict()["total_errors"] == 1
    attr_name = "counts"
    with pytest.raises(FrozenInstanceError):
        setattr(p1, attr_name, ())
