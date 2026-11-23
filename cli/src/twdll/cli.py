"""Typer CLI entrypoint for twdll (`cli/src/twdll/cli.py`).

By architectural contract (enforced via import-linter), this is the ONLY module
in twdll allowed to import `typer` or `rich`.
"""

from __future__ import annotations

import json
import queue
import sys
import threading
from collections.abc import Generator, Sequence
from contextlib import contextmanager
from dataclasses import replace
from pathlib import Path
from typing import Annotated

import typer
from rich.console import Console
from rich.panel import Panel
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
from rich.prompt import Prompt
from rich.table import Table

from twdll.domain import (
    BINARY_SPECS,
    REFERENCE_BINARIES,
    SELECTABLE_TARGETS,
    STEAM_CONSOLE_URL,
    BinaryName,
    BinarySpec,
    BinaryStatus,
    CleanTarget,
    Decompiler,
    HostOS,
    ProgressCallback,
    ProgressEvent,
    ProgressKind,
    StepSummary,
    ToolchainStatus,
    WorkspaceContext,
    active_binaries,
    noop_progress,
    normalize_targets,
    resolve_toolchain,
    verify_binary_header,
)
from twdll.init import (
    candidate_steam_roots,
    clean,
    discover_steamapps_dirs,
    find_binary_in_steam,
    init,
    open_depot_console,
    resolve_binary,
    status,
)

_console = Console()

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


def _binary_state_badge(binary: BinaryStatus) -> str:
    if binary.staged:
        if binary.sha256_match is False:
            return "[bold yellow]staged (sha256 mismatch)[/]"
        return "[bold green]staged[/]"
    if binary.discovered_path is not None:
        return "[bold yellow]discovered[/]"
    return "[bold red]missing[/]"


def _print_toolchain(toolchain: ToolchainStatus) -> None:
    badge = "[bold green]READY[/]" if toolchain.ready else "[bold red]MISSING[/]"
    table = Table(
        title=f"Toolchain ({toolchain.decompiler.value}): {badge}",
        title_justify="left",
        show_header=True,
        header_style="bold cyan",
    )
    table.add_column("Tool", style="bold")
    table.add_column("Status")
    table.add_column("Source", style="dim")
    table.add_column("Location")
    for tool in toolchain.tools:
        tool_state = "[green]ready[/]" if tool.ready else "[red]missing[/]"
        loc = str(tool.path) if tool.path is not None else "-"
        table.add_row(tool.name.value, tool_state, tool.source.value, loc)
    _console.print(table)


def _print_binaries(binaries: Sequence[BinaryStatus]) -> None:
    staged_count = sum(1 for b in binaries if b.staged)
    count_style = "green" if staged_count == len(binaries) else "yellow"
    table = Table(
        title=f"Binaries ([bold {count_style}]{staged_count}/{len(binaries)} staged[/])",
        title_justify="left",
        show_header=True,
        header_style="bold cyan",
    )
    table.add_column("Binary", style="bold")
    table.add_column("Arch", style="dim")
    table.add_column("Status")
    table.add_column("Source", style="dim")
    table.add_column("Location")
    for binary in binaries:
        loc_path = binary.staged_path or binary.discovered_path
        loc = str(loc_path) if loc_path is not None else "-"
        table.add_row(
            binary.name.value,
            binary.arch.value,
            _binary_state_badge(binary),
            binary.source.value,
            loc,
        )
    _console.print(table)
    for binary in binaries:
        if binary.instruction is not None:
            color = "yellow" if binary.staged and binary.sha256_match is False else "cyan"
            _console.print(f"  [{color}]-> {binary.name.value}:[/] {binary.instruction}")


def _print_steps(steps: Sequence[StepSummary]) -> None:
    table = Table(
        title="Step Directories",
        title_justify="left",
        show_header=True,
        header_style="bold cyan",
    )
    table.add_column("Step", style="bold")
    table.add_column("State")
    table.add_column("Directory")
    for item in steps:
        state = "[green]exists[/]" if item.exists else "[dim]empty[/]"
        table.add_row(item.step.value, state, str(item.directory))
    _console.print(table)


def _is_interactive_tty() -> bool:
    return sys.stdin.isatty() and sys.stdout.isatty()


def parse_target_selection(
    raw: str,
    default_targets: tuple[BinaryName, ...],
) -> tuple[BinaryName, ...] | None:
    cleaned = raw.strip()
    if not cleaned:
        return default_targets
    if cleaned.lower() == "all":
        return SELECTABLE_TARGETS
    tokens = [tok for tok in cleaned.replace(",", " ").split() if tok]
    chosen: list[BinaryName] = []
    for tok in tokens:
        if tok.isdigit():
            idx = int(tok) - 1
            if not (0 <= idx < len(SELECTABLE_TARGETS)):
                return None
            chosen.append(SELECTABLE_TARGETS[idx])
            continue
        try:
            chosen.append(BinaryName(tok))
        except ValueError:
            return None
    try:
        return normalize_targets(chosen)
    except ValueError:
        return None


def _prompt_decompiler(ctx: WorkspaceContext) -> WorkspaceContext:
    chosen_raw = Prompt.ask(
        "[bold]Decompiler[/]",
        choices=[d.value for d in Decompiler],
        default=ctx.decompiler.value,
        console=_console,
    )
    decompiler = Decompiler(chosen_raw)
    updated_ctx = replace(ctx, config=replace(ctx.config, decompiler=decompiler))
    if decompiler is not Decompiler.IDA or resolve_toolchain(updated_ctx).ready:
        return updated_ctx
    ida_raw = Prompt.ask(
        "[yellow]IDA 9.5 directory not auto-detected. Enter path (Enter to skip)[/]",
        default=str(updated_ctx.config.toolchain.ida_dir or ""),
        console=_console,
    ).strip()
    if not ida_raw:
        return updated_ctx
    toolchain = replace(updated_ctx.config.toolchain, ida_dir=Path(ida_raw))
    return replace(updated_ctx, config=replace(updated_ctx.config, toolchain=toolchain))


def _default_wizard_targets(
    ctx: WorkspaceContext,
    steamapps_dirs: Sequence[Path],
) -> tuple[BinaryName, ...]:
    if ctx.paths.config_file.is_file():
        return ctx.config.targets
    found = tuple(
        name
        for name in SELECTABLE_TARGETS
        if find_binary_in_steam(BINARY_SPECS[name], steamapps_dirs) is not None
    )
    return found if found else SELECTABLE_TARGETS


def _prompt_targets(
    ctx: WorkspaceContext,
    steamapps_dirs: Sequence[Path],
) -> tuple[BinaryName, ...]:
    ref_names = ", ".join(name.value for name in REFERENCE_BINARIES)
    _console.print(f"[dim]Reference binary (always included):[/] [green]{ref_names}[/]")
    _console.print("[bold]Select 32-bit target(s) to reverse-engineer:[/]")
    for idx, name in enumerate(SELECTABLE_TARGETS, start=1):
        spec = BINARY_SPECS[name]
        found = find_binary_in_steam(spec, steamapps_dirs) is not None
        tag = "[green](found in Steam)[/]" if found else "[dim](not found in Steam)[/]"
        _console.print(f"  [cyan]{idx}[/]. {name.value} ({spec.default_install_dir}) {tag}")

    defaults = _default_wizard_targets(ctx, steamapps_dirs)
    default_indices = ",".join(str(SELECTABLE_TARGETS.index(t) + 1) for t in defaults)
    while True:
        raw = Prompt.ask(
            "[bold]Targets[/] (comma-separated numbers, names, or 'all')",
            default=default_indices,
            console=_console,
        )
        parsed = parse_target_selection(raw, defaults)
        if parsed is not None:
            return parsed
        _console.print("[red]Invalid selection. Choose at least one valid 32-bit target.[/]")


def _poll_stable_depot_binary(
    spec: BinarySpec,
    host_os: HostOS,
    prev_size: int | None,
) -> tuple[Path | None, int | None]:
    steamapps_dirs = discover_steamapps_dirs(candidate_steam_roots(host_os), host_os=host_os)
    found = find_binary_in_steam(spec, steamapps_dirs)
    if found is None:
        return None, None
    try:
        size = found.stat().st_size
    except OSError:
        return None, None
    if prev_size is not None and size > 0 and size == prev_size:
        return found, size
    return None, size


def _handle_depot_user_input(
    line: str,
    spec: BinarySpec,
    host_os: HostOS,
) -> tuple[bool, Path | None]:
    cleaned = line.strip()
    if cleaned.lower() == "skip":
        return True, None
    if not cleaned:
        steamapps_dirs = discover_steamapps_dirs(candidate_steam_roots(host_os), host_os=host_os)
        if find_binary_in_steam(spec, steamapps_dirs) is not None:
            return True, None
        _console.print("[yellow]Not found in Steam depot yet—still watching...[/]")
        return False, None
    candidate = Path(cleaned).expanduser()
    if verify_binary_header(candidate, spec.arch):
        _console.print(f"[green]Verified {spec.arch.value} binary:[/] {candidate}")
        return True, candidate
    _console.print(f"[red]Invalid {spec.arch.value} binary:[/] {candidate}")
    return False, None


def _start_stdin_reader() -> queue.Queue[str]:
    input_queue: queue.Queue[str] = queue.Queue()

    def _read_lines() -> None:
        for line in sys.stdin:
            input_queue.put(line)

    threading.Thread(target=_read_lines, daemon=True).start()
    return input_queue


def _prompt_local_binary_path(spec: BinarySpec) -> Path | None:
    _console.print(
        f"[yellow]Steam client not detected for {spec.name.value} ({spec.file_name}).[/]"
    )
    while True:
        raw = Prompt.ask(
            f"[bold]Local path to {spec.file_name}[/] (Enter to skip)",
            default="",
            console=_console,
        ).strip()
        if not raw:
            return None
        candidate = Path(raw).expanduser()
        if verify_binary_header(candidate, spec.arch):
            _console.print(f"[green]Verified {spec.arch.value} binary:[/] {candidate}")
            return candidate
        _console.print(f"[red]Invalid {spec.arch.value} binary:[/] {candidate}")


def _watch_steam_depot(
    spec: BinarySpec,
    depot_command: str,
    host_os: HostOS,
    input_queue: queue.Queue[str],
    *,
    clipboard_copied: bool,
) -> Path | None:
    status_line = (
        "[green]Steam Console opened & command copied to clipboard.[/]"
        if clipboard_copied
        else f"[yellow]Steam Console opened ({STEAM_CONSOLE_URL}). Paste the command below.[/]"
    )
    _console.print(
        Panel(
            f"{status_line}\n\n"
            f"[bold cyan]{depot_command}[/]\n\n"
            "[dim]Waiting for Steam depot download (auto-checking every 1s). "
            "Press Enter to check now, paste a custom path, or type 'skip'.[/]",
            title=f"Binary Required: {spec.name.value} ({spec.default_install_dir})",
            border_style="cyan",
        )
    )
    prev_size: int | None = None
    try:
        with _console.status(f"[cyan]Waiting for {spec.name.value} download...[/]"):
            while True:
                found, prev_size = _poll_stable_depot_binary(spec, host_os, prev_size)
                if found is not None:
                    _console.print(f"[bold green]Detected completed depot binary:[/] {found}")
                    return None
                try:
                    line = input_queue.get(timeout=1.0)
                except queue.Empty:
                    continue
                done, custom_path = _handle_depot_user_input(line, spec, host_os)
                if done:
                    return custom_path
    except KeyboardInterrupt:
        _console.print(f"[yellow]Skipping wait for {spec.name.value}.[/]")
        return None


def _find_missing_active_specs(
    ctx: WorkspaceContext,
    binaries: dict[BinaryName, Path | None],
    steamapps_dirs: Sequence[Path],
) -> list[BinarySpec]:
    missing: list[BinarySpec] = []
    for name in active_binaries(ctx.config):
        spec = BINARY_SPECS[name]
        current = resolve_binary(
            spec,
            binaries.get(name),
            ctx.paths.staged_binary_path(name),
            steamapps_dirs,
        )
        if not current.staged and current.discovered_path is None:
            missing.append(spec)
    return missing


def _resolve_missing_binaries(
    ctx: WorkspaceContext,
    steamapps_dirs: Sequence[Path],
) -> dict[BinaryName, Path | None]:
    updated = dict(ctx.config.binaries)
    missing = _find_missing_active_specs(ctx, updated, steamapps_dirs)
    input_queue: queue.Queue[str] | None = None
    for spec in missing:
        if spec.depot_command is None:
            continue
        opened, copied = open_depot_console(spec.depot_command, ctx.host_os)
        if not opened and input_queue is None:
            custom_path = _prompt_local_binary_path(spec)
        else:
            input_queue = input_queue if input_queue is not None else _start_stdin_reader()
            custom_path = _watch_steam_depot(
                spec,
                spec.depot_command,
                ctx.host_os,
                input_queue,
                clipboard_copied=copied,
            )
        if custom_path is not None:
            updated[spec.name] = custom_path
    return updated


def _run_init_wizard(ctx: WorkspaceContext, *, skip_binaries: bool) -> WorkspaceContext:
    _console.print(Panel.fit("[bold cyan]twdll init[/] — Workspace Setup", border_style="cyan"))
    steamapps_dirs = discover_steamapps_dirs(
        candidate_steam_roots(ctx.host_os),
        host_os=ctx.host_os,
    )
    updated_ctx = _prompt_decompiler(ctx)
    targets = _prompt_targets(updated_ctx, steamapps_dirs)
    updated_ctx = replace(updated_ctx, config=replace(updated_ctx.config, targets=targets))
    if skip_binaries:
        return updated_ctx
    binaries = _resolve_missing_binaries(updated_ctx, steamapps_dirs)
    return replace(updated_ctx, config=replace(updated_ctx.config, binaries=binaries))


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
    """Initialize twdll.toml, stage active binaries, and provision .twdll/init/tools/."""
    ctx = _get_context(cli_ctx)
    persist_config = False
    if not json_output and _is_interactive_tty():
        ctx = _run_init_wizard(ctx, skip_binaries=no_binaries)
        persist_config = True
    with _cli_progress(enabled=not json_output) as on_progress:
        result = init(
            ctx=ctx,
            persist_config=persist_config,
            skip_toolchain=no_toolchain,
            skip_binaries=no_binaries,
            on_progress=on_progress,
        )
    if json_output:
        typer.echo(json.dumps(result.to_dict(), indent=2))
        return

    action = "Created" if result.config_created else "Updated"
    _console.print(
        f"[bold green]{action} config:[/] {result.config_file} "
        f"[dim](decompiler={result.decompiler.value})[/]"
    )
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

    cfg_state = "[green]present[/]" if result.config_exists else "[yellow]missing (defaults)[/]"
    _console.print(
        Panel(
            f"[bold]Workspace:[/] {result.root}\n"
            f"[bold]Config:[/]    {result.config_file} ({cfg_state})\n"
            f"[bold]Host OS:[/]   {result.host_os.value}  |  "
            f"[bold]Decompiler:[/] {result.decompiler.value}",
            title="[bold cyan]twdll status[/]",
            border_style="cyan",
        )
    )
    _print_binaries(result.binaries)
    _print_toolchain(result.toolchain)
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
        _console.print(f"[yellow]Nothing to clean for target '{result.target.value}'.[/]")
        return

    for removed in result.removed_directories:
        _console.print(f"[green]Removed:[/] {removed}")


@app.command("mcp")
def mcp_command(cli_ctx: typer.Context) -> None:
    """Run the twdll Model Context Protocol (MCP) server over stdio."""
    from twdll.mcp import serve_mcp

    serve_mcp(ctx=_get_context(cli_ctx))


def main() -> None:
    app()


if __name__ == "__main__":
    main()
