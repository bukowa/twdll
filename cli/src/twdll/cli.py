"""Typer CLI entrypoint for twdll (`cli/src/twdll/cli.py`).

By architectural contract (enforced via import-linter), this is the ONLY module
in twdll allowed to import `typer` or `rich`.
"""

from __future__ import annotations

import json
from collections.abc import Generator, Sequence
from contextlib import contextmanager
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
    noop_progress,
)
from twdll.init import clean, init, status

app = typer.Typer(
    name="twdll",
    help="Cross-platform reverse-engineering CLI for Warscape Total War.",
    no_args_is_help=True,
    add_completion=False,
)


def _resolve_root(workspace: Path | None) -> Path:
    return (workspace if workspace is not None else Path.cwd()).resolve()


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


def _print_steps(steps: Sequence[StepSummary]) -> None:
    typer.echo("Step Directories:")
    for item in steps:
        state = "exists" if item.exists else "empty"
        typer.echo(f"  - {item.step.value}: {state} [{item.directory}]")


@app.command("init")
def init_command(
    decompiler: Annotated[
        Decompiler | None,
        typer.Option("--decompiler", help="Active decompiler to configure ('ghidra' or 'ida')."),
    ] = None,
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
    workspace: Annotated[
        Path | None,
        typer.Option("--workspace", help="Workspace root directory (defaults to current dir)."),
    ] = None,
) -> None:
    """Initialize twdll.toml, provision .twdll/init/tools/, and stage discovered game binaries."""
    with _cli_progress(enabled=not json_output) as on_progress:
        result = init(
            root=_resolve_root(workspace),
            decompiler=decompiler,
            skip_toolchain=no_toolchain,
            skip_binaries=no_binaries,
            on_progress=on_progress,
        )
    if json_output:
        typer.echo(json.dumps(result.to_dict(), indent=2))
        return

    action = "Created" if result.config_created else "Loaded"
    typer.echo(f"{action} config: {result.config_file} (decompiler={result.decompiler.value})")
    _print_toolchain(result.toolchain)
    _print_binaries(result.binaries)


@app.command("status")
def status_command(
    json_output: Annotated[
        bool,
        typer.Option("--json", help="Emit machine-readable JSON output."),
    ] = False,
    workspace: Annotated[
        Path | None,
        typer.Option("--workspace", help="Workspace root directory (defaults to current dir)."),
    ] = None,
) -> None:
    """Show read-only status of workspace, toolchain, staged/discovered binaries, and steps."""
    result = status(root=_resolve_root(workspace))
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
    target: Annotated[
        CleanTarget,
        typer.Argument(help="Step directory to clean: init, analyze, extract, derive, or all."),
    ],
    json_output: Annotated[
        bool,
        typer.Option("--json", help="Emit machine-readable JSON output."),
    ] = False,
    workspace: Annotated[
        Path | None,
        typer.Option("--workspace", help="Workspace root directory (defaults to current dir)."),
    ] = None,
) -> None:
    """Delete .twdll/<step>/ artifacts created by a pipeline step (or all steps)."""
    result = clean(root=_resolve_root(workspace), target=target)
    if json_output:
        typer.echo(json.dumps(result.to_dict(), indent=2))
        return

    if not result.removed_directories:
        typer.echo(f"Nothing to clean for target '{result.target.value}'.")
        return

    for removed in result.removed_directories:
        typer.echo(f"Removed: {removed}")


def main() -> None:
    app()


if __name__ == "__main__":
    main()
