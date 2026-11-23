"""MCP (Model Context Protocol) stdio server adapter for twdll (`cli/src/twdll/mcp.py`).

By architectural contract (enforced via import-linter), this is the ONLY module
in twdll allowed to import `mcp`. Every tool delegates directly to the typed
step entrypoints (`twdll.init`, etc.) and returns frozen `domain.py` dataclasses
so `mcp` 2.x generates full `inputSchema` and `outputSchema` automatically.
"""

from __future__ import annotations

from dataclasses import replace

from mcp.server import MCPServer

from twdll.domain import (
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


def build_mcp_server(ctx: WorkspaceContext) -> MCPServer:
    """Create an `MCPServer` exposing twdll domain entrypoints bound to `ctx`."""
    server = MCPServer("twdll")
    active_ctx = ctx

    @server.tool(structured_output=True)
    def twdll_init(
        decompiler: Decompiler | None = None,
        targets: list[BinaryName] | None = None,
        skip_toolchain: bool = False,
        skip_binaries: bool = False,
    ) -> InitResult:
        """Initialize twdll.toml, stage active binaries, and provision .twdll/init/tools/."""
        nonlocal active_ctx
        updated_ctx, persist = _apply_init_overrides(active_ctx, decompiler, targets)
        active_ctx = updated_ctx
        return init(
            ctx=active_ctx,
            persist_config=persist,
            skip_toolchain=skip_toolchain,
            skip_binaries=skip_binaries,
        )

    @server.tool(structured_output=True)
    def twdll_status() -> WorkspaceStatus:
        """Show read-only status of workspace, toolchain, binaries, and pipeline steps."""
        return status(ctx=active_ctx)

    @server.tool(structured_output=True)
    def twdll_clean(target: CleanTarget) -> CleanResult:
        """Delete .twdll/<step>/ artifacts for a pipeline step (or all steps)."""
        return clean(ctx=active_ctx, target=target)

    return server


def serve_mcp(ctx: WorkspaceContext) -> None:
    """Run the twdll MCP server over stdio transport."""
    build_mcp_server(ctx).run(transport="stdio")
