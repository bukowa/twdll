"""Tests for the twdll MCP server adapter (`cli/src/twdll/mcp.py`)."""

from __future__ import annotations

import asyncio
from pathlib import Path
from typing import cast

from mcp.server import MCPServer
from mcp.types import CallToolResult

from twdll.domain import BinaryName, CleanTarget, Decompiler, WorkspaceContext
from twdll.mcp import build_mcp_server


def _call_tool(server: MCPServer, name: str, arguments: dict[str, object]) -> dict[str, object]:
    res = asyncio.run(server.call_tool(name, arguments))
    assert isinstance(res, CallToolResult)
    assert res.structured_content is not None
    return cast("dict[str, object]", res.structured_content)


def test_mcp_lists_slice1_tools_with_enum_schemas(tmp_path: Path) -> None:
    server = build_mcp_server(WorkspaceContext.from_root(tmp_path))
    tools = asyncio.run(server.list_tools())
    by_name = {tool.name: tool for tool in tools}

    assert set(by_name) == {"twdll_init", "twdll_status", "twdll_clean"}

    clean_schema = by_name["twdll_clean"].input_schema
    assert "properties" in clean_schema
    assert "target" in clean_schema["properties"]
    assert clean_schema.get("required") == ["target"]

    clean_out = by_name["twdll_clean"].output_schema
    assert clean_out is not None
    assert "properties" in clean_out
    assert "removed_directories" in clean_out["properties"]

    status_out = by_name["twdll_status"].output_schema
    assert status_out is not None
    assert "binaries" in status_out["properties"]


def test_mcp_calls_init_status_clean(tmp_path: Path) -> None:
    server = build_mcp_server(WorkspaceContext.from_root(tmp_path))

    init_payload = _call_tool(
        server,
        "twdll_init",
        {
            "decompiler": Decompiler.GHIDRA.value,
            "targets": [BinaryName.ROME2_WINDOWS_32.value],
            "skip_toolchain": True,
            "skip_binaries": True,
        },
    )
    assert init_payload["config_created"] is True
    assert init_payload["decompiler"] == Decompiler.GHIDRA.value
    init_bins = cast("list[dict[str, object]]", init_payload["binaries"])
    assert [b["name"] for b in init_bins] == [
        BinaryName.ATTILA_LINUX_64.value,
        BinaryName.ROME2_WINDOWS_32.value,
    ]

    status_payload = _call_tool(server, "twdll_status", {})
    assert status_payload["config_exists"] is True
    status_bins = cast("list[dict[str, object]]", status_payload["binaries"])
    assert [b["name"] for b in status_bins] == [
        BinaryName.ATTILA_LINUX_64.value,
        BinaryName.ROME2_WINDOWS_32.value,
    ]

    clean_payload = _call_tool(server, "twdll_clean", {"target": CleanTarget.INIT.value})
    assert clean_payload["target"] == CleanTarget.INIT.value
    removed = cast("list[str]", clean_payload["removed_directories"])
    assert len(removed) == 1
