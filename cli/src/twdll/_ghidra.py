"""Shared internal PyGhidra/JVM lifecycle and project helpers."""

from __future__ import annotations

import os
import shutil
import time
from pathlib import Path
from typing import Any, cast

from twdll.domain import (
    BinaryName,
    ProgressCallback,
    ProgressEvent,
    ProgressKind,
    ToolName,
    WorkspaceContext,
    resolve_toolchain,
)


class _GhidraProgressMonitor:
    """Forwards Ghidra `TaskMonitor` updates to `ProgressCallback`."""

    def __init__(
        self,
        binary: BinaryName,
        on_progress: ProgressCallback,
        task_key: str = "ghidra_detail",
    ) -> None:
        self._binary = binary
        self._on_progress = on_progress
        self._task_key = task_key
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
                task_key=self._task_key,
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


def make_task_monitor(
    binary: BinaryName,
    on_progress: ProgressCallback,
    task_key: str = "ghidra_detail",
) -> Any:
    """Create a Ghidra `TaskMonitor` proxy forwarding updates to `on_progress`."""
    try:
        import jpype

        return jpype.JProxy(
            "ghidra.util.task.TaskMonitor",
            inst=_GhidraProgressMonitor(binary, on_progress, task_key=task_key),
        )
    except Exception:
        import pyghidra

        return cast(Any, pyghidra).task_monitor()


def require_ghidra_toolchain(ctx: WorkspaceContext) -> tuple[Path, Path]:
    """Resolve and validate the Ghidra and JDK directories for `ctx`."""
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


def start_ghidra(ctx: WorkspaceContext) -> Path:
    """Configure JVM environment variables and start `pyghidra`."""
    ghidra_dir, java_home = require_ghidra_toolchain(ctx)
    os.environ["JAVA_HOME"] = str(java_home)
    os.environ["JAVA_HOME_OVERRIDE"] = str(java_home)
    import pyghidra

    pyghidra.start(install_dir=ghidra_dir)
    return ghidra_dir


def remove_ghidra_project(gpr_path: Path, rep_dir: Path) -> None:
    """Remove `.gpr` and `.rep/` artifacts if they exist."""
    gpr_path.unlink(missing_ok=True)
    if rep_dir.exists():
        shutil.rmtree(rep_dir)


def analyze_and_save_project(
    tmp_dir: Path,
    staged_path: Path,
    stem: str,
    monitor: Any,
) -> None:
    """Create, load, analyze, and save a Ghidra project inside `tmp_dir`."""
    import pyghidra

    pyg = cast(Any, pyghidra)
    with pyg.open_project(tmp_dir, stem, create=True) as project:
        loader = pyg.program_loader().project(project).source(str(staged_path))
        with loader.load() as load_results:
            load_results.save(pyg.task_monitor())
        with pyg.program_context(project, f"/{staged_path.name}") as program:
            pyg.analyze(program, monitor=monitor)
            program.save("twdll initial analysis", pyg.task_monitor())
