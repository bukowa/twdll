"""Headless decompiler analysis step (`.twdll/analyze/`)."""

from __future__ import annotations

import importlib
import os
import shutil
import tempfile
import time
from pathlib import Path
from typing import Any, cast

from twdll.domain import (
    BINARY_SPECS,
    AnalyzeResult,
    AnalyzeTargetResult,
    BinaryArch,
    BinaryName,
    Decompiler,
    ProgressCallback,
    ProgressEvent,
    ProgressKind,
    ToolName,
    WorkspaceContext,
    noop_progress,
    resolve_toolchain,
)


class _GhidraProgressMonitor:
    """Forwards Ghidra `TaskMonitor` updates to `ProgressCallback`."""

    def __init__(self, binary: BinaryName, on_progress: ProgressCallback) -> None:
        self._binary = binary
        self._on_progress = on_progress
        self._message = "initializing..."
        self._maximum = 0
        self._progress = 0
        self._indeterminate = False
        self._cancelled = False
        self._cancel_enabled = False
        self._last_emit = 0.0

    def _emit(self, *, force: bool = False) -> None:
        now = time.monotonic()
        if not force and (now - self._last_emit) < 0.25:
            return
        self._last_emit = now
        total = self._maximum if (self._maximum > 0 and not self._indeterminate) else None
        completed = min(self._progress, total) if total is not None else self._progress
        self._on_progress(
            ProgressEvent(
                kind=ProgressKind.STEP,
                task_key="analyze_detail",
                label=f"{self._binary.value}: {self._message}",
                completed=completed,
                total=total,
            )
        )

    def isCancelled(self) -> bool:
        return self._cancelled

    def setShowProgressValue(self, _show: bool) -> None:
        pass

    def setMessage(self, message: object) -> None:
        text = str(message).strip() if message is not None else ""
        if text and text != self._message:
            self._message = text
            self._emit(force=True)

    def getMessage(self) -> str:
        return self._message

    def initialize(self, max_value: int, message: object = None) -> None:
        self._maximum = max(0, int(max_value))
        self._progress = 0
        if message is not None:
            text = str(message).strip()
            if text:
                self._message = text
        self._emit(force=True)

    def setMaximum(self, max_value: int) -> None:
        self._maximum = max(0, int(max_value))
        self._emit()

    def getMaximum(self) -> int:
        return self._maximum

    def setIndeterminate(self, indeterminate: bool) -> None:
        self._indeterminate = bool(indeterminate)
        self._emit(force=True)

    def isIndeterminate(self) -> bool:
        return self._indeterminate

    def setProgress(self, value: int) -> None:
        self._progress = max(0, int(value))
        self._emit()

    def getProgress(self) -> int:
        return self._progress

    def incrementProgress(self, amount: int = 1) -> None:
        self._progress += int(amount)
        self._emit()

    def increment(self, amount: int = 1) -> None:
        self._progress += int(amount)
        self._emit()

    def checkCancelled(self) -> None:
        pass

    def cancel(self) -> None:
        self._cancelled = True

    def addCancelledListener(self, _listener: object) -> None:
        pass

    def removeCancelledListener(self, _listener: object) -> None:
        pass

    def setCancelEnabled(self, enabled: bool) -> None:
        self._cancel_enabled = bool(enabled)

    def isCancelEnabled(self) -> bool:
        return self._cancel_enabled

    def clearCancelled(self) -> None:
        self._cancelled = False


def _make_task_monitor(binary: BinaryName, on_progress: ProgressCallback) -> Any:
    try:
        import jpype

        return jpype.JProxy(
            "ghidra.util.task.TaskMonitor",
            inst=_GhidraProgressMonitor(binary, on_progress),
        )
    except Exception:
        import pyghidra

        return cast(Any, pyghidra).task_monitor()


def _emit(
    on_progress: ProgressCallback,
    label: str,
    *,
    completed: int = 0,
    total: int | None = None,
) -> None:
    on_progress(
        ProgressEvent(
            kind=ProgressKind.STEP,
            task_key="analyze",
            label=label,
            completed=completed,
            total=total,
        )
    )


def _require_ghidra_toolchain(ctx: WorkspaceContext) -> tuple[Path, Path]:
    tools = {tool.name: tool.path for tool in resolve_toolchain(ctx).tools}
    ghidra_dir = tools.get(ToolName.GHIDRA)
    if ghidra_dir is None or not ghidra_dir.is_dir():
        raise FileNotFoundError(
            "Ghidra installation not found; run 'twdll init' or set ghidra_dir."
        )
    java_home = tools.get(ToolName.JAVA)
    if java_home is None or not java_home.is_dir():
        raise FileNotFoundError("JDK installation not found; run 'twdll init' or set java_home.")
    return ghidra_dir, java_home


def _resolve_targets(
    ctx: WorkspaceContext,
    binaries: tuple[BinaryName, ...],
) -> tuple[BinaryName, ...]:
    if binaries:
        unique: list[BinaryName] = []
        for binary in binaries:
            if binary not in unique:
                staged = ctx.paths.staged_binary_path(binary)
                if not staged.is_file():
                    raise FileNotFoundError(f"Staged binary not found for {binary.value}: {staged}")
                unique.append(binary)
        return tuple(unique)

    staged_targets = tuple(
        sorted(
            (binary for binary in BinaryName if ctx.paths.staged_binary_path(binary).is_file()),
            key=lambda binary: 0 if BINARY_SPECS[binary].arch == BinaryArch.PE32 else 1,
        )
    )
    if not staged_targets:
        raise FileNotFoundError(
            f"No staged binaries found in {ctx.paths.binaries_dir}; run 'twdll init' first."
        )
    return staged_targets


def _remove_ghidra_project(gpr_path: Path, rep_dir: Path) -> None:
    gpr_path.unlink(missing_ok=True)
    if rep_dir.exists():
        shutil.rmtree(rep_dir)


def _is_program_analyzed(program: Any) -> bool:
    util = cast(Any, importlib.import_module("ghidra.program.util"))
    return bool(util.GhidraProgramUtilities.isAnalyzed(program))


def _check_existing_project(
    ctx: WorkspaceContext,
    binary: BinaryName,
    gpr_path: Path,
    rep_dir: Path,
) -> bool:
    if not (gpr_path.is_file() and rep_dir.is_dir()):
        return False

    import pyghidra

    pyg = cast(Any, pyghidra)
    staged_path = ctx.paths.staged_binary_path(binary)
    stem = ctx.paths.analyze_stem(Decompiler.GHIDRA, binary)
    try:
        with tempfile.TemporaryDirectory(prefix="twdll_ghidra_") as tmp_str:
            tmp_dir = Path(tmp_str)
            shutil.copy2(gpr_path, tmp_dir / gpr_path.name)
            shutil.copytree(rep_dir, tmp_dir / rep_dir.name)
            with (
                pyg.open_project(tmp_dir, stem, create=False) as project,
                pyg.program_context(project, f"/{staged_path.name}") as program,
            ):
                return _is_program_analyzed(program)
    except Exception:
        return False


def _analyze_single_binary(
    ctx: WorkspaceContext,
    binary: BinaryName,
    index: int,
    total: int,
    on_progress: ProgressCallback,
) -> AnalyzeTargetResult:
    import pyghidra

    pyg = cast(Any, pyghidra)
    prefix = f"[{index}/{total}] {binary.value}"
    gpr_path = ctx.paths.ghidra_gpr_path(binary)
    rep_dir = ctx.paths.ghidra_rep_dir(binary)

    if _check_existing_project(ctx, binary, gpr_path, rep_dir):
        _emit(on_progress, f"{prefix}: skipped (already analyzed)", completed=index, total=total)
        return AnalyzeTargetResult(
            binary=binary,
            decompiler=Decompiler.GHIDRA,
            project_path=gpr_path,
            analyzed=True,
            skipped=True,
            duration_seconds=0.0,
        )

    if gpr_path.exists() or rep_dir.exists():
        _emit(on_progress, f"{prefix}: incomplete project found, recreating...")
        _remove_ghidra_project(gpr_path, rep_dir)

    ctx.paths.analyze_dir.mkdir(parents=True, exist_ok=True)
    staged_path = ctx.paths.staged_binary_path(binary)
    stem = ctx.paths.analyze_stem(Decompiler.GHIDRA, binary)

    _emit(
        on_progress, f"{prefix}: running Ghidra auto-analysis...", completed=index - 1, total=total
    )
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="twdll_ghidra_") as tmp_str:
        tmp_dir = Path(tmp_str)
        with pyg.open_project(tmp_dir, stem, create=True) as project:
            loader = pyg.program_loader().project(project).source(str(staged_path))
            with loader.load() as load_results:
                load_results.save(pyg.task_monitor())
            with pyg.program_context(project, f"/{staged_path.name}") as program:
                monitor = _make_task_monitor(binary, on_progress)
                pyg.analyze(program, monitor=monitor)
                program.save("twdll initial analysis", pyg.task_monitor())
        shutil.move(str(tmp_dir / gpr_path.name), str(gpr_path))
        shutil.move(str(tmp_dir / rep_dir.name), str(rep_dir))
    duration = round(time.monotonic() - started, 2)
    _emit(on_progress, f"{prefix}: done ({duration:.2f}s)", completed=index, total=total)

    return AnalyzeTargetResult(
        binary=binary,
        decompiler=Decompiler.GHIDRA,
        project_path=gpr_path,
        analyzed=True,
        skipped=False,
        duration_seconds=duration,
    )


def analyze(
    ctx: WorkspaceContext,
    binaries: tuple[BinaryName, ...] = (),
    on_progress: ProgressCallback = noop_progress,
) -> AnalyzeResult:
    """Run headless decompiler analysis on selected or all staged binaries."""
    match ctx.decompiler:
        case Decompiler.IDA:
            raise NotImplementedError("IDA analysis is not implemented yet.")
        case Decompiler.GHIDRA:
            pass

    ghidra_dir, java_home = _require_ghidra_toolchain(ctx)
    targets = _resolve_targets(ctx, binaries)

    os.environ["JAVA_HOME"] = str(java_home)
    os.environ["JAVA_HOME_OVERRIDE"] = str(java_home)
    import pyghidra

    _emit(on_progress, f"Starting Ghidra ({ghidra_dir})...")
    pyghidra.start(install_dir=ghidra_dir)

    total = len(targets)
    results = tuple(
        _analyze_single_binary(ctx, binary, idx, total, on_progress)
        for idx, binary in enumerate(targets, start=1)
    )
    return AnalyzeResult(decompiler=ctx.decompiler, targets=results)
