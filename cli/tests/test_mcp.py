"""Tests for the twdll MCP server adapter (`cli/src/twdll/mcp.py`)."""

from __future__ import annotations

import asyncio
import json
import subprocess
import sys
from pathlib import Path
from typing import Any, cast

import pytest
from mcp.server import MCPServer
from mcp.types import CallToolResult

import twdll.init as init_mod
import twdll.mcp as mcp_mod
from twdll.domain import BinaryName, CleanTarget, Decompiler, HostOS, WorkspaceContext
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

    assert set(by_name) == {"twdll_init", "twdll_status", "twdll_clean", "twdll_analyze"}

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


def test_mcp_calls_init_status_clean(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_steam_roots(_host_os: HostOS) -> tuple[Path, ...]:
        return ()

    monkeypatch.setattr(init_mod, "candidate_steam_roots", fake_steam_roots)
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

    # External edits to twdll.toml should be reflected immediately on subsequent MCP calls.
    config_file = WorkspaceContext.from_root(tmp_path).paths.config_file
    config_file.write_text(
        config_file.read_text(encoding="utf-8").replace(
            f'targets = ["{BinaryName.ROME2_WINDOWS_32.value}"]',
            f'targets = ["{BinaryName.TOB_WINDOWS_32.value}"]',
        ),
        encoding="utf-8",
    )
    reloaded_payload = _call_tool(server, "twdll_status", {})
    reloaded_bins = cast("list[dict[str, object]]", reloaded_payload["binaries"])
    assert [b["name"] for b in reloaded_bins] == [
        BinaryName.ATTILA_LINUX_64.value,
        BinaryName.TOB_WINDOWS_32.value,
    ]

    clean_payload = _call_tool(server, "twdll_clean", {"target": CleanTarget.INIT.value})
    assert clean_payload["target"] == CleanTarget.INIT.value
    removed = cast("list[str]", clean_payload["removed_directories"])
    assert len(removed) == 1


def test_mcp_analyze_runs_worker_subprocess_with_cwd_and_explicit_overrides(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured: dict[str, Any] = {}
    expected_gpr = tmp_path / ".twdll" / "analyze" / "ghidra_attila_linux_64.gpr"
    worker_payload = {
        "decompiler": Decompiler.GHIDRA.value,
        "targets": [
            {
                "binary": BinaryName.ATTILA_LINUX_64.value,
                "decompiler": Decompiler.GHIDRA.value,
                "project_path": str(expected_gpr),
                "analyzed": True,
                "skipped": True,
                "duration_seconds": 0.0,
            }
        ],
    }

    class _FakePopen:
        def __init__(self, cmd: list[str], **kwargs: Any) -> None:
            captured["cmd"] = cmd
            captured["kwargs"] = kwargs
            self.pid = 12345
            self.returncode = 0

        def communicate(self, timeout: float | None = None) -> tuple[str, str]:
            captured["timeout"] = timeout
            return json.dumps(worker_payload), ""

    monkeypatch.setattr(mcp_mod.subprocess, "Popen", _FakePopen)
    server = build_mcp_server(
        WorkspaceContext.from_root(tmp_path),
        cli_overrides=("--decompiler", "ghidra"),
    )

    result = _call_tool(server, "twdll_analyze", {"binaries": [BinaryName.ATTILA_LINUX_64.value]})
    assert result["decompiler"] == Decompiler.GHIDRA.value
    assert captured["cmd"] == [
        sys.executable,
        "-m",
        "twdll.cli",
        "--decompiler",
        "ghidra",
        "analyze",
        BinaryName.ATTILA_LINUX_64.value,
        "--json",
    ]
    assert captured["kwargs"]["cwd"] == tmp_path
    assert captured["kwargs"]["stdin"] is subprocess.DEVNULL


def test_mcp_analyze_surfaces_non_json_stderr_and_timeout_errors(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    class _FailingPopen:
        def __init__(self, _cmd: list[str], **_kwargs: Any) -> None:
            self.pid = 12345
            self.returncode = 1

        def communicate(self, timeout: float | None = None) -> tuple[str, str]:
            return "", "fatal: boom"

    monkeypatch.setattr(mcp_mod.subprocess, "Popen", _FailingPopen)
    server = build_mcp_server(WorkspaceContext.from_root(tmp_path))

    with pytest.raises(Exception, match="Error executing tool twdll_analyze") as fail_exc:
        _call_tool(server, "twdll_analyze", {})
    assert "fatal: boom" in str(fail_exc.value.__cause__)

    killed: list[int] = []

    class _TimeoutPopen:
        def __init__(self, _cmd: list[str], **_kwargs: Any) -> None:
            self.pid = 99999
            self.returncode = -15
            self._calls = 0

        def communicate(self, timeout: float | None = None) -> tuple[str, str]:
            self._calls += 1
            if self._calls == 1:
                raise subprocess.TimeoutExpired(cmd="twdll", timeout=timeout or 1.0)
            return "", ""

    def _fake_terminate(proc: subprocess.Popen[str]) -> None:
        killed.append(proc.pid)

    monkeypatch.setattr(mcp_mod.subprocess, "Popen", _TimeoutPopen)
    monkeypatch.setattr(mcp_mod, "_terminate_process_tree", _fake_terminate)

    with pytest.raises(Exception, match="Error executing tool twdll_analyze") as timeout_exc:
        _call_tool(server, "twdll_analyze", {})
    assert "timed out" in str(timeout_exc.value.__cause__)
    assert killed == [99999]
