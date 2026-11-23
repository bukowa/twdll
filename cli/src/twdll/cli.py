"""Typer CLI entrypoint for twdll (`cli/src/twdll/cli.py`).

By architectural contract (enforced via import-linter), this is the ONLY module
in twdll allowed to import `typer` or `rich`.
"""

from __future__ import annotations

import json
from collections.abc import Generator, Sequence
from contextlib import contextmanager
from dataclasses import replace
from pathlib import Path
from typing import Annotated

import typer
from rich.console import Console
from rich.progress import (
    BarColumn,
    Progress,
    SpinnerColumn,
    TaskID,
    TaskProgressColumn,
    TextColumn,
    TimeElapsedColumn,
    TimeRemainingColumn,
)

from twdll.domain import (
    BinaryStatus,
    CleanTarget,
    Decompiler,
    ProgressCallback,
    ProgressEvent,
    ProgressKind,
    StepSummary,
    ToolchainStatus,
    WorkspaceContext,
    noop_progress,
)
from twdll.init import clean, init, status

app = typer.Typer(
    name="twdll",
    help="Cross-platform reverse-engineering CLI for Warscape Total War.",
    no_args_is_help=True,
    add_completion=False,
)


def _resolve_root() -> Path:
    return Path.cwd().resolve()


def _get_context(cli_ctx: typer.Context) -> WorkspaceContext:
    if isinstance(cli_ctx.obj, WorkspaceContext):
        return cli_ctx.obj
    return WorkspaceContext.from_root(_resolve_root())


@app.callback()
def main_callback(
    cli_ctx: typer.Context,
    decompiler: Annotated[
        Decompiler | None,
        typer.Option("--decompiler", help="Override active decompiler ('ghidra' or 'ida')."),
    ] = None,
    ghidra_dir: Annotated[
        Path | None,
        typer.Option("--ghidra-dir", help="Override Ghidra installation directory."),
    ] = None,
    java_home: Annotated[
        Path | None,
        typer.Option("--java-home", help="Override JDK installation directory (JAVA_HOME)."),
    ] = None,
    ida_dir: Annotated[
        Path | None,
        typer.Option("--ida-dir", help="Override IDA 9.5 installation directory."),
    ] = None,
) -> None:
    """Build WorkspaceContext from twdll.toml and root CLI overrides."""
    ctx = WorkspaceContext.from_root(_resolve_root())
    toolchain = replace(
        ctx.config.toolchain,
        ghidra_dir=ghidra_dir if ghidra_dir is not None else ctx.config.toolchain.ghidra_dir,
        java_home=java_home if java_home is not None else ctx.config.toolchain.java_home,
        ida_dir=ida_dir if ida_dir is not None else ctx.config.toolchain.ida_dir,
    )
    config = replace(
        ctx.config,
        decompiler=decompiler if decompiler is not None else ctx.config.decompiler,
        toolchain=toolchain,
    )
    cli_ctx.obj = replace(ctx, config=config)


def _format_progress_units(event: ProgressEvent) -> str:
    if event.kind is ProgressKind.DOWNLOAD:
        done_mb = event.completed / (1024 * 1024)
        if event.total is not None and event.total > 0:
            total_mb = event.total / (1024 * 1024)
            return f"{done_mb:6.1f}/{total_mb:.1f} MB"
        return f"{done_mb:6.1f} MB"
    if event.total is not None:
        return f"{event.completed}/{event.total}"
    return ""


@contextmanager
def _cli_progress(enabled: bool) -> Generator[ProgressCallback]:
    if not enabled:
        yield noop_progress
        return

    progress = Progress(
        SpinnerColumn(),
        TextColumn("[bold blue]{task.description}"),
        BarColumn(),
        TaskProgressColumn(),
        TextColumn("[cyan]{task.fields[units]}"),
        TimeElapsedColumn(),
        TimeRemainingColumn(),
        console=Console(stderr=True),
    )
    tasks: dict[str, TaskID] = {}

    def report(event: ProgressEvent) -> None:
        units = _format_progress_units(event)
        task_id = tasks.get(event.task_key)
        if task_id is None:
            tasks[event.task_key] = progress.add_task(
                event.label,
                total=event.total,
                completed=event.completed,
                units=units,
            )
            return
        progress.update(
            task_id,
            description=event.label,
            total=event.total,
            completed=event.completed,
            units=units,
        )

    with progress:
        yield report


def _print_toolchain(toolchain: ToolchainStatus) -> None:
    state = "READY" if toolchain.ready else "MISSING"
    typer.echo(f"Toolchain ({toolchain.decompiler.value}): {state}")
    for tool in toolchain.tools:
        tool_state = "ready" if tool.ready else "missing"
        loc = str(tool.path) if tool.path is not None else "-"
        typer.echo(f"  - {tool.name.value}: {tool_state} ({tool.source.value}) [{loc}]")


def _print_binaries(binaries: Sequence[BinaryStatus]) -> None:
    staged_count = sum(1 for b in binaries if b.staged)
    typer.echo(f"Binaries ({staged_count}/{len(binaries)} staged):")
    for binary in binaries:
        state = (
            "staged"
            if binary.staged
            else ("discovered" if binary.discovered_path is not None else "missing")
        )
        loc_path = binary.staged_path or binary.discovered_path
        loc = str(loc_path) if loc_path is not None else "-"
        typer.echo(
            f"  - {binary.name.value} ({binary.arch.value}): {state} "
            f"({binary.source.value}) [{loc}]"
        )
        if binary.instruction is not None:
            typer.echo(f"      -> {binary.instruction}")


def _print_steps(steps: Sequence[StepSummary]) -> None:
    typer.echo("Step Directories:")
    for item in steps:
        state = "exists" if item.exists else "empty"
        typer.echo(f"  - {item.step.value}: {state} [{item.directory}]")


@app.command("init")
def init_command(
    cli_ctx: typer.Context,
    no_toolchain: Annotated[
        bool,
        typer.Option("--no-toolchain", help="Skip portable toolchain download/provisioning."),
    ] = False,
    no_binaries: Annotated[
        bool,
        typer.Option("--no-binaries", help="Skip Steam binary discovery and staging."),
    ] = False,
    json_output: Annotated[
        bool,
        typer.Option("--json", help="Emit machine-readable JSON output."),
    ] = False,
) -> None:
    """Initialize twdll.toml, provision .twdll/init/tools/, and stage discovered game binaries."""
    ctx = _get_context(cli_ctx)
    with _cli_progress(enabled=not json_output) as on_progress:
        result = init(
            ctx=ctx,
            skip_toolchain=no_toolchain,
            skip_binaries=no_binaries,
            on_progress=on_progress,
        )
    if json_output:
        typer.echo(json.dumps(result.to_dict(), indent=2))
        return

    action = "Created" if result.config_created else "Loaded"
    typer.echo(f"{action} config: {result.config_file} (decompiler={result.decompiler.value})")
    _print_binaries(result.binaries)
    _print_toolchain(result.toolchain)


@app.command("status")
def status_command(
    cli_ctx: typer.Context,
    json_output: Annotated[
        bool,
        typer.Option("--json", help="Emit machine-readable JSON output."),
    ] = False,
) -> None:
    """Show read-only status of workspace, toolchain, staged/discovered binaries, and steps."""
    result = status(ctx=_get_context(cli_ctx))
    if json_output:
        typer.echo(json.dumps(result.to_dict(), indent=2))
        return

    cfg_state = "present" if result.config_exists else "missing (using defaults)"
    typer.echo(f"Workspace: {result.root}")
    typer.echo(f"Config: {result.config_file} [{cfg_state}]")
    typer.echo(f"Host OS: {result.host_os.value} | Decompiler: {result.decompiler.value}")
    _print_toolchain(result.toolchain)
    _print_binaries(result.binaries)
    _print_steps(result.steps)


@app.command("clean")
def clean_command(
    cli_ctx: typer.Context,
    target: Annotated[
        CleanTarget,
        typer.Argument(help="Step directory to clean: init, analyze, extract, derive, or all."),
    ],
    json_output: Annotated[
        bool,
        typer.Option("--json", help="Emit machine-readable JSON output."),
    ] = False,
) -> None:
    """Delete .twdll/<step>/ artifacts created by a pipeline step (or all steps)."""
    result = clean(ctx=_get_context(cli_ctx), target=target)
    if json_output:
        typer.echo(json.dumps(result.to_dict(), indent=2))
        return

    if not result.removed_directories:
        typer.echo(f"Nothing to clean for target '{result.target.value}'.")
        return

    for removed in result.removed_directories:
        typer.echo(f"Removed: {removed}")


@app.command("mcp")
def mcp_command(cli_ctx: typer.Context) -> None:
    """Run the twdll Model Context Protocol (MCP) server over stdio."""
    from twdll.mcp import serve_mcp

    serve_mcp(ctx=_get_context(cli_ctx))


def main() -> None:
    app()


if __name__ == "__main__":
    main()
