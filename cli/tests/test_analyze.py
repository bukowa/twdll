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

from twdll._ghidra import ghidra_temp_dir, make_task_monitor
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
    ghidra_dir = tmp_path / "opt_ghidra"
    java_home = tmp_path / "opt_jdk"
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
    builder.addLoaderArg.return_value = builder
    builder.load.return_value = loader_cm
    return builder


def _install_fake_pyghidra(
    monkeypatch: pytest.MonkeyPatch,
    *,
    analyzed_state: bool = True,
) -> tuple[MagicMock, list[tuple[Path, Path, str, bool]], list[MagicMock], MagicMock]:
    calls: list[tuple[Path, Path, str, bool]] = []
    programs: list[MagicMock] = []
    start_mock = MagicMock()
    loader_builder = _make_loader_builder()
    launcher_cls = MagicMock(return_value=MagicMock(start=start_mock))

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
        program = MagicMock()
        program.startTransaction.return_value = 7
        options = MagicMock()
        program.getOptions.return_value = options
        programs.append(program)
        yield program

    fake_pyghidra = ModuleType("pyghidra")
    fake_pyghidra.__dict__.update(
        {
            "start": start_mock,
            "HeadlessPyGhidraLauncher": launcher_cls,
            "open_project": _open_project,
            "program_context": _program_context,
            "program_loader": MagicMock(return_value=loader_builder),
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
    return start_mock, calls, programs, loader_builder


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
    start_mock, calls, programs, loader_builder = _install_fake_pyghidra(
        monkeypatch, analyzed_state=True
    )

    messages: list[str] = []
    first = analyze(ctx, on_progress=lambda e: messages.append(e.label))
    assert len(first.targets) == 1
    assert (first.targets[0].analyzed, first.targets[0].skipped) == (True, False)
    assert first.targets[0].project_path == ctx.paths.ghidra_gpr_path(BinaryName.ATTILA_LINUX_64)
    meta_path = ctx.paths.analyze_meta_path(Decompiler.GHIDRA, BinaryName.ATTILA_LINUX_64)
    assert meta_path.is_file()
    assert "schema_version" not in json.loads(meta_path.read_text(encoding="utf-8"))
    assert (start_mock.call_count, len(calls), calls[-1][3], len(programs)) == (1, 1, True, 1)
    loader_builder.addLoaderArg.assert_called_once_with("-loader-includeOtherBlocks", "false")
    programs[0].startTransaction.assert_called_once_with(
        "Configure ELF64 low-RAM DWARF-external analyzers"
    )
    programs[0].getOptions.assert_called_once_with("Analyzers")
    set_bool = programs[0].getOptions.return_value.setBoolean
    for opt_name in (
        "DWARF",
        "GCC Exception Handlers",
        "Embedded Media",
        "Call Convention ID",
        "Demangler GNU.Apply Function Signatures",
        "Demangler GNU.Apply Function Calling Conventions",
        "Create Address Tables",
        "Stack",
        "Variadic Function Signature Override",
        "Non-Returning Functions - Discovered",
    ):
        set_bool.assert_any_call(opt_name, False)
    programs[0].endTransaction.assert_called_once_with(7, True)

    second = analyze(ctx)
    assert len(second.targets) == 1
    assert second.targets[0].analyzed is True
    assert second.targets[0].skipped is True
    # O(1) .meta.json skip check must not start Ghidra or open the project.
    assert start_mock.call_count == 1
    assert len(calls) == 1


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_analyze_pe32_keeps_dwarf_option_untouched(
    tmp_path: Path,
    host_os: HostOS,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    base_ctx = _make_ctx(tmp_path, host_os=host_os)
    ctx = replace(
        base_ctx,
        config=replace(base_ctx.config, targets=(BinaryName.ATTILA_WINDOWS_32,)),
    )
    _stage_binary(ctx, BinaryName.ATTILA_WINDOWS_32)
    _, _, programs, loader_builder = _install_fake_pyghidra(monkeypatch, analyzed_state=True)

    result = analyze(ctx, binaries=(BinaryName.ATTILA_WINDOWS_32,))
    assert len(result.targets) == 1
    assert result.targets[0].binary == BinaryName.ATTILA_WINDOWS_32
    assert len(programs) == 1
    loader_builder.addLoaderArg.assert_not_called()
    programs[0].startTransaction.assert_not_called()
    programs[0].getOptions.assert_not_called()


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_ghidra_temp_dir_is_dot_free(host_os: HostOS) -> None:
    with ghidra_temp_dir(host_os) as tmp_dir:
        assert tmp_dir.is_dir()
        assert all(not part.startswith(".") for part in tmp_dir.parts)
    assert not tmp_dir.exists()


def test_make_task_monitor_wraps_jproxy_in_jobject(monkeypatch: pytest.MonkeyPatch) -> None:
    fake_jpype = ModuleType("jpype")
    jproxy_calls: list[tuple[str, object]] = []
    jobject_calls: list[tuple[object, str]] = []
    seen_messages: list[str] = []

    def _fake_jproxy(iface: str, *, inst: object) -> object:
        jproxy_calls.append((iface, inst))
        return ("proxy", inst)

    def _fake_jobject(proxy: tuple[str, object], iface: str) -> object:
        jobject_calls.append((proxy, iface))
        return proxy[1]

    fake_jpype.__dict__.update({"JProxy": _fake_jproxy, "JObject": _fake_jobject})
    monkeypatch.setitem(sys.modules, "jpype", fake_jpype)

    monitor = make_task_monitor(
        BinaryName.ATTILA_LINUX_64,
        lambda _e: None,
        on_message=lambda msg: seen_messages.append(msg),
    )
    assert len(jproxy_calls) == 1
    assert len(jobject_calls) == 1
    assert jobject_calls[0][1] == "ghidra.util.task.TaskMonitor"
    monitor.setMessage("Disassembling Entry Points")
    monitor.initialize(100, "Searching for Strings")
    assert seen_messages == ["Disassembling Entry Points", "Searching for Strings"]
    assert monitor.isCancelled() is False
    monitor.cancel()
    assert monitor.isCancelled() is True


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

    _, calls, _, _ = _install_fake_pyghidra(monkeypatch, analyzed_state=False)
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
