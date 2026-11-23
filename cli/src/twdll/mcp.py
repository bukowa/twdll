"""MCP (Model Context Protocol) stdio server adapter for twdll (`cli/src/twdll/mcp.py`).

By architectural contract (enforced via import-linter), this is the ONLY module
in twdll allowed to import `mcp`. Every tool delegates to typed step entrypoints
(`twdll.init` in-process; JVM-bound steps via isolated worker subprocesses) and
returns frozen `domain.py` dataclasses so `mcp` 2.x generates full schemas.
"""

from __future__ import annotations

import json
import os
import signal
import subprocess
import sys
from dataclasses import replace
from pathlib import Path
from typing import cast

from mcp.server import MCPServer

from twdll.domain import (
    AnalyzeResult,
    AnalyzeTargetResult,
    BinaryName,
    CleanResult,
    CleanTarget,
    Decompiler,
    InitResult,
    WorkspaceContext,
    WorkspaceStatus,
    normalize_targets,
)
from twdll.init import clean, init, status

_WORKER_TIMEOUT_SECONDS = 1800.0


def _refresh_context(base_ctx: WorkspaceContext) -> WorkspaceContext:
    if not base_ctx.paths.config_file.is_file():
        return base_ctx
    return replace(WorkspaceContext.from_root(base_ctx.root), host_os=base_ctx.host_os)


def _apply_init_overrides(
    ctx: WorkspaceContext,
    decompiler: Decompiler | None,
    targets: list[BinaryName] | None,
) -> tuple[WorkspaceContext, bool]:
    if decompiler is None and targets is None:
        return ctx, False
    updated_config = replace(
        ctx.config,
        decompiler=decompiler if decompiler is not None else ctx.config.decompiler,
        targets=normalize_targets(targets) if targets is not None else ctx.config.targets,
    )
    return replace(ctx, config=updated_config), True


def _build_worker_cmd(
    cli_overrides: tuple[str, ...],
    subcommand: str,
    args: tuple[str, ...],
) -> list[str]:
    return [sys.executable, "-m", "twdll.cli", *cli_overrides, subcommand, *args, "--json"]


def _terminate_process_tree(proc: subprocess.Popen[str]) -> None:
    if os.name == "nt":
        subprocess.run(
            ["taskkill", "/F", "/T", "/PID", str(proc.pid)],
            check=False,
            capture_output=True,
        )
        return
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    try:
        proc.wait(timeout=5.0)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            return


def _run_worker_json(
    ctx: WorkspaceContext,
    cmd: list[str],
    timeout_seconds: float = _WORKER_TIMEOUT_SECONDS,
) -> dict[str, object]:
    proc = subprocess.Popen(
        cmd,
        cwd=ctx.root,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        encoding="utf-8",
        start_new_session=(os.name != "nt"),
    )
    try:
        stdout, stderr = proc.communicate(timeout=timeout_seconds)
    except subprocess.TimeoutExpired as exc:
        _terminate_process_tree(proc)
        proc.communicate()
        raise RuntimeError(
            f"Worker command timed out after {timeout_seconds:.0f}s: {' '.join(cmd)}"
        ) from exc

    if proc.returncode != 0:
        detail = stderr.strip() or stdout.strip() or f"exit code {proc.returncode}"
        raise RuntimeError(f"Worker command failed (exit {proc.returncode}): {detail}")

    try:
        parsed: object = json.loads(stdout)
    except json.JSONDecodeError as exc:
        detail = stderr.strip() or stdout.strip()
        raise RuntimeError(f"Worker command returned invalid JSON: {detail}") from exc

    if not isinstance(parsed, dict):
        raise RuntimeError("Worker command returned non-object JSON payload.")
    return cast(dict[str, object], parsed)


def _parse_analyze_result(payload: dict[str, object]) -> AnalyzeResult:
    raw_targets = payload.get("targets")
    if not isinstance(raw_targets, list):
        raise RuntimeError("Worker analyze payload missing 'targets' list.")
    targets: list[AnalyzeTargetResult] = []
    for item in cast(list[object], raw_targets):
        if not isinstance(item, dict):
            raise RuntimeError("Worker analyze target entry is not an object.")
        entry = cast(dict[str, object], item)
        raw_duration = entry.get("duration_seconds", 0.0)
        duration = float(raw_duration) if isinstance(raw_duration, (int, float)) else 0.0
        targets.append(
            AnalyzeTargetResult(
                binary=BinaryName(str(entry["binary"])),
                decompiler=Decompiler(str(entry["decompiler"])),
                project_path=Path(str(entry["project_path"])),
                analyzed=bool(entry["analyzed"]),
                skipped=bool(entry["skipped"]),
                duration_seconds=duration,
            )
        )
    return AnalyzeResult(
        decompiler=Decompiler(str(payload["decompiler"])),
        targets=tuple(targets),
    )


def build_mcp_server(
    ctx: WorkspaceContext,
    cli_overrides: tuple[str, ...] = (),
) -> MCPServer:
    """Create an `MCPServer` exposing twdll domain entrypoints bound to `ctx`."""
    server = MCPServer("twdll")

    @server.tool(structured_output=True)
    def twdll_init(
        decompiler: Decompiler | None = None,
        targets: list[BinaryName] | None = None,
        skip_toolchain: bool = False,
        skip_binaries: bool = False,
    ) -> InitResult:
        """Initialize twdll.toml, stage active binaries, and provision .twdll/init/tools/.

        Reloads twdll.toml before running so external edits to [binaries] or [toolchain] apply.
        `attila_linux_64` (reference binary) is always included alongside `targets` (1+ 32-bit
        targets). When `skip_binaries=False`, missing binaries trigger Steam Console + clipboard
        copy for the first missing depot command. When `skip_toolchain=False`, missing portable
        Ghidra 12.2 + JDK 25 are downloaded into `.twdll/init/tools/`.
        """
        current_ctx = _refresh_context(ctx)
        updated_ctx, persist = _apply_init_overrides(current_ctx, decompiler, targets)
        return init(
            ctx=updated_ctx,
            persist_config=persist,
            skip_toolchain=skip_toolchain,
            skip_binaries=skip_binaries,
        )

    @server.tool(structured_output=True)
    def twdll_status() -> WorkspaceStatus:
        """Show read-only status of workspace, toolchain, active binaries, and pipeline steps."""
        return status(ctx=_refresh_context(ctx))

    @server.tool(structured_output=True)
    def twdll_clean(target: CleanTarget) -> CleanResult:
        """Delete .twdll/<step>/ artifacts for a pipeline step (or all steps)."""
        return clean(ctx=_refresh_context(ctx), target=target)

    @server.tool(structured_output=True)
    def twdll_analyze(binaries: list[BinaryName] | None = None) -> AnalyzeResult:
        """Run headless decompiler analysis in an isolated worker subprocess.

        Note: Full auto-analysis of large binaries can take several minutes.
        Prefer passing a single binary at a time or instructing the user to
        run `twdll analyze` in their terminal for a full multi-binary run.
        """
        args = tuple(binary.value for binary in binaries) if binaries else ()
        cmd = _build_worker_cmd(cli_overrides, "analyze", args)
        payload = _run_worker_json(ctx, cmd)
        return _parse_analyze_result(payload)

    return server


def serve_mcp(
    ctx: WorkspaceContext,
    cli_overrides: tuple[str, ...] = (),
) -> None:
    """Run the twdll MCP server over stdio transport."""
    build_mcp_server(ctx, cli_overrides=cli_overrides).run(transport="stdio")
