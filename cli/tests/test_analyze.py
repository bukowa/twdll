"""Unit tests for `twdll.analyze`."""

from __future__ import annotations

import json
import sys
from collections.abc import Generator
from contextlib import contextmanager
from dataclasses import replace
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock

import pytest
from typer.testing import CliRunner

from twdll.analyze import analyze
from twdll.cli import app
from twdll.domain import (
    BinaryName,
    Decompiler,
    FileSuffix,
    HostOS,
    ToolchainConfig,
    WorkspaceConfig,
    WorkspaceContext,
)


def _make_ctx(
    tmp_path: Path,
    *,
    host_os: HostOS = HostOS.LINUX,
    decompiler: Decompiler = Decompiler.GHIDRA,
    with_toolchain: bool = True,
) -> WorkspaceContext:
    ghidra_dir = tmp_path / "tools" / "ghidra"
    java_home = tmp_path / "tools" / "jdk"
    if with_toolchain:
        ghidra_dir.mkdir(parents=True, exist_ok=True)
        java_home.mkdir(parents=True, exist_ok=True)

    config = WorkspaceConfig(
        decompiler=decompiler,
        targets=(BinaryName.ATTILA_LINUX_64,),
        toolchain=ToolchainConfig(
            ghidra_dir=ghidra_dir if with_toolchain else None,
            java_home=java_home if with_toolchain else None,
        ),
    )
    ctx = WorkspaceContext.from_root(tmp_path)
    return replace(ctx, host_os=host_os, config=config)


def _stage_binary(ctx: WorkspaceContext, binary: BinaryName) -> Path:
    staged = ctx.paths.staged_binary_path(binary)
    staged.parent.mkdir(parents=True, exist_ok=True)
    staged.write_bytes(b"binary-data")
    return staged


def _make_loader_builder() -> MagicMock:
    loader_cm = MagicMock()
    loader_cm.__enter__ = MagicMock(return_value=MagicMock())
    loader_cm.__exit__ = MagicMock(return_value=False)
    builder = MagicMock()
    builder.project.return_value = builder
    builder.source.return_value = builder
    builder.load.return_value = loader_cm
    return builder


def _install_fake_pyghidra(
    monkeypatch: pytest.MonkeyPatch,
    *,
    analyzed_state: bool = True,
) -> tuple[MagicMock, list[tuple[Path, Path, str, bool]]]:
    calls: list[tuple[Path, Path, str, bool]] = []
    start_mock = MagicMock()

    @contextmanager
    def _open_project(
        project_location: Path,
        project_name: str,
        *,
        create: bool = False,
    ) -> Generator[MagicMock]:
        calls.append((project_location, project_location, project_name, create))
        if create:
            (project_location / f"{project_name}{FileSuffix.GHIDRA_GPR.value}").write_text(
                "gpr", encoding="utf-8"
            )
            (project_location / f"{project_name}{FileSuffix.GHIDRA_REP.value}").mkdir(
                parents=True, exist_ok=True
            )
        yield MagicMock()

    @contextmanager
    def _program_context(_project: object, _path: str) -> Generator[MagicMock]:
        yield MagicMock()

    fake_pyghidra = ModuleType("pyghidra")
    fake_pyghidra.__dict__.update(
        {
            "start": start_mock,
            "open_project": _open_project,
            "program_context": _program_context,
            "program_loader": MagicMock(return_value=_make_loader_builder()),
            "analyze": MagicMock(),
            "task_monitor": MagicMock(),
        }
    )

    fake_ghidra = ModuleType("ghidra")
    fake_program = ModuleType("ghidra.program")
    fake_util = ModuleType("ghidra.program.util")

    class _FakeGhidraProgramUtilities:
        @staticmethod
        def isAnalyzed(_program: object) -> bool:
            return analyzed_state

    fake_util.__dict__.update({"GhidraProgramUtilities": _FakeGhidraProgramUtilities})

    monkeypatch.setitem(sys.modules, "pyghidra", fake_pyghidra)
    monkeypatch.setitem(sys.modules, "ghidra", fake_ghidra)
    monkeypatch.setitem(sys.modules, "ghidra.program", fake_program)
    monkeypatch.setitem(sys.modules, "ghidra.program.util", fake_util)
    return start_mock, calls


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_analyze_ida_not_implemented(tmp_path: Path, host_os: HostOS) -> None:
    ctx = _make_ctx(tmp_path, host_os=host_os, decompiler=Decompiler.IDA)
    with pytest.raises(NotImplementedError, match="IDA"):
        analyze(ctx)


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_analyze_missing_toolchain_raises(tmp_path: Path, host_os: HostOS) -> None:
    ctx = _make_ctx(tmp_path, host_os=host_os, with_toolchain=False)
    with pytest.raises(FileNotFoundError, match="Ghidra"):
        analyze(ctx)


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_analyze_missing_staged_binary_raises(tmp_path: Path, host_os: HostOS) -> None:
    ctx = _make_ctx(tmp_path, host_os=host_os)
    with pytest.raises(FileNotFoundError, match="No staged binaries"):
        analyze(ctx)


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_analyze_fresh_and_skip_when_analyzed(
    tmp_path: Path,
    host_os: HostOS,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    ctx = _make_ctx(tmp_path, host_os=host_os)
    _stage_binary(ctx, BinaryName.ATTILA_LINUX_64)
    start_mock, calls = _install_fake_pyghidra(monkeypatch, analyzed_state=True)

    messages: list[str] = []
    first = analyze(ctx, on_progress=lambda e: messages.append(e.label))
    assert len(first.targets) == 1
    assert first.targets[0].analyzed is True
    assert first.targets[0].skipped is False
    assert first.targets[0].project_path == ctx.paths.ghidra_gpr_path(BinaryName.ATTILA_LINUX_64)
    assert ctx.paths.analyze_meta_path(Decompiler.GHIDRA, BinaryName.ATTILA_LINUX_64).is_file()
    assert start_mock.call_count == 1
    assert len(calls) == 1
    assert calls[-1][3] is True

    second = analyze(ctx)
    assert len(second.targets) == 1
    assert second.targets[0].analyzed is True
    assert second.targets[0].skipped is True
    # O(1) .meta.json skip check must not start Ghidra or open the project.
    assert start_mock.call_count == 1
    assert len(calls) == 1


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_analyze_recreates_incomplete_or_corrupt_meta_project(
    tmp_path: Path,
    host_os: HostOS,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    ctx = _make_ctx(tmp_path, host_os=host_os)
    _stage_binary(ctx, BinaryName.ATTILA_LINUX_64)
    gpr = ctx.paths.ghidra_gpr_path(BinaryName.ATTILA_LINUX_64)
    rep = ctx.paths.ghidra_rep_dir(BinaryName.ATTILA_LINUX_64)
    meta = ctx.paths.analyze_meta_path(Decompiler.GHIDRA, BinaryName.ATTILA_LINUX_64)
    gpr.parent.mkdir(parents=True, exist_ok=True)
    gpr.write_text("stale", encoding="utf-8")
    rep.mkdir(parents=True, exist_ok=True)
    (rep / "stale.txt").write_text("stale", encoding="utf-8")
    meta.write_text("{corrupt-json", encoding="utf-8")

    _, calls = _install_fake_pyghidra(monkeypatch, analyzed_state=False)
    result = analyze(ctx)

    assert len(result.targets) == 1
    assert result.targets[0].analyzed is True
    assert result.targets[0].skipped is False
    assert not (rep / "stale.txt").exists()
    assert meta.is_file()
    assert [c[3] for c in calls] == [True]


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_analyze_filters_by_active_binaries(
    tmp_path: Path,
    host_os: HostOS,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    ctx = _make_ctx(tmp_path, host_os=host_os)
    _stage_binary(ctx, BinaryName.ATTILA_LINUX_64)
    _stage_binary(ctx, BinaryName.ATTILA_WINDOWS_32)
    _install_fake_pyghidra(monkeypatch, analyzed_state=True)

    # _make_ctx configures targets=(ATTILA_LINUX_64,), so ATTILA_WINDOWS_32 is not active.
    result = analyze(ctx)
    assert [target.binary for target in result.targets] == [BinaryName.ATTILA_LINUX_64]


def test_cli_analyze_json(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    ctx = _make_ctx(tmp_path)
    _stage_binary(ctx, BinaryName.ATTILA_LINUX_64)
    _install_fake_pyghidra(monkeypatch, analyzed_state=True)

    monkeypatch.chdir(tmp_path)
    runner = CliRunner()
    res = runner.invoke(
        app,
        [
            "--ghidra-dir",
            str(ctx.config.toolchain.ghidra_dir),
            "--java-home",
            str(ctx.config.toolchain.java_home),
            "analyze",
            "--json",
        ],
    )
    assert res.exit_code == 0, res.output
    payload = json.loads(res.stdout)
    assert payload["decompiler"] == Decompiler.GHIDRA.value
    assert len(payload["targets"]) == 1
    assert payload["targets"][0]["binary"] == BinaryName.ATTILA_LINUX_64.value
    assert payload["targets"][0]["analyzed"] is True
