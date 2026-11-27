"""Shared internal PyGhidra/JVM lifecycle, Java bridge, and pass telemetry helpers."""

from __future__ import annotations

import os
import shutil
import tempfile
import time
from collections import Counter
from collections.abc import Callable, Generator, Sequence
from contextlib import contextmanager
from pathlib import Path
from typing import Any, cast

from twdll.domain import (
    BinaryName,
    DwarfInjectionReport,
    DwarfPassName,
    DwarfPassStats,
    HostOS,
    ProgressCallback,
    ProgressEvent,
    ProgressKind,
    ToolName,
    WorkspaceContext,
    resolve_toolchain,
)

_MAX_DETERMINATE_TOTAL = 100_000_000


class _GhidraProgressMonitor:
    """Forwards Ghidra `TaskMonitor` updates to `ProgressCallback`."""

    def __init__(
        self,
        binary: BinaryName,
        on_progress: ProgressCallback,
        task_key: str = "ghidra_detail",
        on_message: Callable[[str], None] | None = None,
    ) -> None:
        self._binary = binary
        self._on_progress = on_progress
        self._task_key = task_key
        self._on_message = on_message
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
        total = (
            self._maximum
            if (0 < self._maximum <= _MAX_DETERMINATE_TOTAL and not self._indeterminate)
            else None
        )
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
            if self._on_message is not None:
                self._on_message(text)
            self._emit(force=True)

    def getMessage(self) -> str:
        return self._message

    def initialize(self, max_value: int, message: object = None) -> None:
        new_max = max(0, int(max_value))
        same_max = new_max == self._maximum and new_max > 0
        self._maximum = new_max
        self._progress = 0
        msg_changed = False
        if message is not None:
            text = str(message).strip()
            if text and text != self._message:
                self._message = text
                msg_changed = True
                if self._on_message is not None:
                    self._on_message(text)
        self._emit(force=msg_changed or not same_max)

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
    on_message: Callable[[str], None] | None = None,
) -> Any:
    """Create a Ghidra `TaskMonitor` proxy forwarding updates to `on_progress`."""
    try:
        import jpype

        proxy = jpype.JProxy(
            "ghidra.util.task.TaskMonitor",
            inst=_GhidraProgressMonitor(
                binary,
                on_progress,
                task_key=task_key,
                on_message=on_message,
            ),
        )
        return jpype.JObject(proxy, "ghidra.util.task.TaskMonitor")
    except Exception:
        import pyghidra

        return cast(Any, pyghidra).task_monitor()


def jclass(name: str) -> Any:
    """Load a Java class by fully-qualified name via `jpype.JClass`."""
    import jpype

    return cast(Any, jpype).JClass(name)


def jarray(cls_name: str, items: Sequence[Any]) -> Any:
    """Construct a typed Java array from `items` via `jpype.JArray`."""
    import jpype

    jp = cast(Any, jpype)
    return jp.JArray(jp.JClass(cls_name))(list(items))


class StatsCollector:
    """Mutable pass-local telemetry accumulator exporting frozen `DwarfPassStats`."""

    def __init__(self, pass_name: DwarfPassName, max_samples: int = 10) -> None:
        self._pass_name = pass_name
        self._max_samples = max_samples
        self._counts: Counter[str] = Counter()
        self._error_counts: Counter[str] = Counter()
        self._sample_errors: list[str] = []

    def inc(self, key: str, amount: int = 1) -> None:
        self._counts[key] += amount

    def record_error(self, key: str, context: str, exc: Exception) -> None:
        self._error_counts[key] += 1
        if len(self._sample_errors) < self._max_samples:
            self._sample_errors.append(f"{context}: {type(exc).__name__}: {exc}")

    def freeze(self) -> DwarfPassStats:
        return DwarfPassStats(
            pass_name=self._pass_name,
            counts=tuple(sorted(self._counts.items())),
            error_counts=tuple(sorted(self._error_counts.items())),
            sample_errors=tuple(self._sample_errors),
        )


def build_injection_report(
    pass1: DwarfPassStats,
    pass2: DwarfPassStats,
    pass3: DwarfPassStats,
    pass4: DwarfPassStats,
) -> DwarfInjectionReport:
    """Aggregate Pass 1-4 telemetry into an immutable `DwarfInjectionReport`."""
    total_errors = sum(count for s in (pass1, pass2, pass3, pass4) for _, count in s.error_counts)
    return DwarfInjectionReport(
        pass1=pass1,
        pass2=pass2,
        pass3=pass3,
        pass4=pass4,
        total_errors=total_errors,
    )


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

    pyg = cast(Any, pyghidra)
    launcher_cls = getattr(pyg, "HeadlessPyGhidraLauncher", None)
    if launcher_cls is not None:
        launcher = launcher_cls(install_dir=ghidra_dir)
        launcher.add_vmargs("-Xmx8G")
        launcher.start()
    else:
        pyg.start(install_dir=ghidra_dir)
    return ghidra_dir


def remove_ghidra_project(gpr_path: Path, rep_dir: Path) -> None:
    """Remove `.gpr` and `.rep/` artifacts if they exist."""
    gpr_path.unlink(missing_ok=True)
    if rep_dir.exists():
        shutil.rmtree(rep_dir)


@contextmanager
def ghidra_temp_dir(host_os: HostOS) -> Generator[Path]:
    """Yield a dot-free temporary directory for Ghidra project staging."""
    base_dir: Path | None = None
    match host_os:
        case HostOS.LINUX:
            var_tmp = Path("/var/tmp")
            if var_tmp.is_dir():
                base_dir = var_tmp
        case HostOS.WINDOWS:
            base_dir = None
    with tempfile.TemporaryDirectory(prefix="twdll_ghidra_", dir=base_dir) as tmp_str:
        yield Path(tmp_str)


def _disable_dwarf_analyzer(program: Any) -> None:
    """Disable redundant or broken ELF64 analyzers replaced by `twdll-dwarf`."""
    tx_id = int(program.startTransaction("Configure ELF64 low-RAM DWARF-external analyzers"))
    try:
        options = program.getOptions("Analyzers")
        options.setBoolean("DWARF", False)
        options.setBoolean("GCC Exception Handlers", False)
        options.setBoolean("Embedded Media", False)
        options.setBoolean("Call Convention ID", False)
        options.setBoolean("Demangler GNU.Apply Function Signatures", False)
        options.setBoolean("Demangler GNU.Apply Function Calling Conventions", False)
        options.setBoolean("Create Address Tables", False)
        options.setBoolean("Stack", False)
        options.setBoolean("Variadic Function Signature Override", False)
        options.setBoolean("Non-Returning Functions - Discovered", False)
    finally:
        program.endTransaction(tx_id, True)


def analyze_and_save_project(
    tmp_dir: Path,
    staged_path: Path,
    stem: str,
    monitor: Any,
    *,
    disable_dwarf: bool = False,
) -> None:
    """Create, load, analyze, and save a Ghidra project inside `tmp_dir`."""
    import pyghidra

    pyg = cast(Any, pyghidra)
    with pyg.open_project(tmp_dir, stem, create=True) as project:
        loader = pyg.program_loader().project(project).source(str(staged_path))
        if disable_dwarf:
            loader = loader.addLoaderArg("-loader-includeOtherBlocks", "false")
        with loader.load() as load_results:
            load_results.save(monitor)
        with pyg.program_context(project, f"/{staged_path.name}") as program:
            if disable_dwarf:
                _disable_dwarf_analyzer(program)
            pyg.analyze(program, monitor=monitor)
            monitor.setMessage("Saving project...")
            program.save("twdll initial analysis", monitor)
