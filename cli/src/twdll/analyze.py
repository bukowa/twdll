"""Headless decompiler analysis step (`.twdll/analyze/`)."""

from __future__ import annotations

import json
import os
import shutil
import tempfile
import time
from datetime import UTC, datetime
from pathlib import Path
from typing import cast

from twdll._ghidra import (
    analyze_and_save_project,
    ghidra_temp_dir,
    make_task_monitor,
    remove_ghidra_project,
    require_ghidra_toolchain,
    start_ghidra,
)
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
    WorkspaceContext,
    active_binaries,
    compute_file_sha256,
    noop_progress,
)


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


def _require_sha256(path: Path) -> str:
    digest = compute_file_sha256(path)
    if digest is None:
        raise FileNotFoundError(f"Unable to hash staged binary: {path}")
    return digest


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

    configured = active_binaries(ctx.config)
    staged_targets = tuple(
        sorted(
            (binary for binary in configured if ctx.paths.staged_binary_path(binary).is_file()),
            key=lambda binary: 0 if BINARY_SPECS[binary].arch == BinaryArch.PE32 else 1,
        )
    )
    if not staged_targets:
        raise FileNotFoundError(
            f"No staged binaries found in {ctx.paths.binaries_dir}; run 'twdll init' first."
        )
    return staged_targets


def _is_meta_valid(meta_path: Path, binary: BinaryName, expected_sha256: str) -> bool:
    if not meta_path.is_file():
        return False
    try:
        raw: object = json.loads(meta_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return False
    if not isinstance(raw, dict):
        return False
    data = cast(dict[str, object], raw)
    return (
        data.get("decompiler") == Decompiler.GHIDRA.value
        and data.get("binary") == binary.value
        and data.get("binary_sha256") == expected_sha256
    )


def _write_meta_atomic(meta_path: Path, binary: BinaryName, binary_sha256: str) -> None:
    payload = {
        "decompiler": Decompiler.GHIDRA.value,
        "binary": binary.value,
        "binary_sha256": binary_sha256,
        "analyzed_at": datetime.now(UTC).isoformat(),
    }
    meta_path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(
        dir=meta_path.parent,
        prefix=f"{meta_path.stem}_",
        suffix=".tmp",
    )
    tmp_path = Path(tmp_name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(payload, handle, indent=2)
            handle.write("\n")
        tmp_path.replace(meta_path)
    except Exception:
        tmp_path.unlink(missing_ok=True)
        raise


def _can_skip_target(ctx: WorkspaceContext, binary: BinaryName, expected_sha256: str) -> bool:
    gpr_path = ctx.paths.ghidra_gpr_path(binary)
    rep_dir = ctx.paths.ghidra_rep_dir(binary)
    meta_path = ctx.paths.analyze_meta_path(Decompiler.GHIDRA, binary)
    return (
        gpr_path.is_file()
        and rep_dir.is_dir()
        and _is_meta_valid(meta_path, binary, expected_sha256)
    )


def _analyze_single_binary(
    ctx: WorkspaceContext,
    binary: BinaryName,
    expected_sha256: str,
    progress_pos: tuple[int, int],
    on_progress: ProgressCallback,
) -> AnalyzeTargetResult:
    index, total = progress_pos
    prefix = f"[{index}/{total}] {binary.value}"
    gpr_path = ctx.paths.ghidra_gpr_path(binary)
    rep_dir = ctx.paths.ghidra_rep_dir(binary)
    meta_path = ctx.paths.analyze_meta_path(Decompiler.GHIDRA, binary)

    if gpr_path.exists() or rep_dir.exists() or meta_path.exists():
        _emit(on_progress, f"{prefix}: incomplete project found, recreating...")
        remove_ghidra_project(gpr_path, rep_dir)
        meta_path.unlink(missing_ok=True)

    ctx.paths.analyze_dir.mkdir(parents=True, exist_ok=True)
    staged_path = ctx.paths.staged_binary_path(binary)
    stem = ctx.paths.analyze_stem(Decompiler.GHIDRA, binary)

    _emit(
        on_progress, f"{prefix}: running Ghidra auto-analysis...", completed=index - 1, total=total
    )
    started = time.monotonic()
    disable_dwarf = BINARY_SPECS[binary].arch == BinaryArch.ELF64
    with ghidra_temp_dir(ctx.host_os) as tmp_dir:
        monitor = make_task_monitor(binary, on_progress, task_key="analyze_detail")
        analyze_and_save_project(
            tmp_dir,
            staged_path,
            stem,
            monitor,
            disable_dwarf=disable_dwarf,
        )
        shutil.move(str(tmp_dir / gpr_path.name), str(gpr_path))
        shutil.move(str(tmp_dir / rep_dir.name), str(rep_dir))
    _write_meta_atomic(meta_path, binary, expected_sha256)
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


def _skipped_result(ctx: WorkspaceContext, binary: BinaryName) -> AnalyzeTargetResult:
    return AnalyzeTargetResult(
        binary=binary,
        decompiler=Decompiler.GHIDRA,
        project_path=ctx.paths.ghidra_gpr_path(binary),
        analyzed=True,
        skipped=True,
        duration_seconds=0.0,
    )


def analyze(
    ctx: WorkspaceContext,
    binaries: tuple[BinaryName, ...] = (),
    on_progress: ProgressCallback = noop_progress,
) -> AnalyzeResult:
    """Run headless decompiler analysis on selected or active staged binaries."""
    match ctx.decompiler:
        case Decompiler.IDA:
            raise NotImplementedError("IDA analysis is not implemented yet.")
        case Decompiler.GHIDRA:
            pass

    require_ghidra_toolchain(ctx)
    targets = _resolve_targets(ctx, binaries)
    hashes = {binary: _require_sha256(ctx.paths.staged_binary_path(binary)) for binary in targets}
    skippable = {binary: _can_skip_target(ctx, binary, hashes[binary]) for binary in targets}

    if not all(skippable.values()):
        ghidra_dir = start_ghidra(ctx)
        _emit(on_progress, f"Starting Ghidra ({ghidra_dir})...")

    total = len(targets)
    results: list[AnalyzeTargetResult] = []
    for idx, binary in enumerate(targets, start=1):
        if skippable[binary]:
            _emit(
                on_progress,
                f"[{idx}/{total}] {binary.value}: skipped (already analyzed)",
                completed=idx,
                total=total,
            )
            results.append(_skipped_result(ctx, binary))
            continue
        results.append(
            _analyze_single_binary(ctx, binary, hashes[binary], (idx, total), on_progress)
        )
    return AnalyzeResult(decompiler=ctx.decompiler, targets=tuple(results))
