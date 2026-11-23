"""MCP (Model Context Protocol) stdio server adapter for twdll (`cli/src/twdll/mcp.py`).

By architectural contract (enforced via import-linter), this is the ONLY module
in twdll allowed to import `mcp`. Every tool delegates directly to the typed
step entrypoints (`twdll.init`, etc.) and returns frozen `domain.py` dataclasses
so `mcp` 2.x generates full `inputSchema` and `outputSchema` automatically.
"""

from __future__ import annotations

from mcp.server import MCPServer

from twdll.domain import (
    CleanResult,
    CleanTarget,
    InitResult,
    WorkspaceContext,
    WorkspaceStatus,
)
from twdll.init import clean, init, status


def build_mcp_server(ctx: WorkspaceContext) -> MCPServer:
    """Create an `MCPServer` exposing twdll domain entrypoints bound to `ctx`."""
    server = MCPServer("twdll")

    @server.tool(structured_output=True)
    def twdll_init(
        skip_toolchain: bool = False,
        skip_binaries: bool = False,
    ) -> InitResult:
        """Initialize twdll.toml, provision .twdll/init/tools/, and stage discovered binaries."""
        return init(
            ctx=ctx,
            skip_toolchain=skip_toolchain,
            skip_binaries=skip_binaries,
        )

    @server.tool(structured_output=True)
    def twdll_status() -> WorkspaceStatus:
        """Show read-only status of workspace, toolchain, binaries, and pipeline steps."""
        return status(ctx=ctx)

    @server.tool(structured_output=True)
    def twdll_clean(target: CleanTarget) -> CleanResult:
        """Delete .twdll/<step>/ artifacts for a pipeline step (or all steps)."""
        return clean(ctx=ctx, target=target)

    return server


def serve_mcp(ctx: WorkspaceContext) -> None:
    """Run the twdll MCP server over stdio transport."""
    build_mcp_server(ctx).run(transport="stdio")
