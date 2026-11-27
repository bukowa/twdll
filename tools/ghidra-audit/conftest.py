"""Pytest configuration and local marker registration for `tools/ghidra-audit`."""

from __future__ import annotations

import sys
from pathlib import Path

_AUDIT_DIR = Path(__file__).resolve().parent
_CLI_SRC = _AUDIT_DIR.parents[1] / "cli" / "src"

for _path in (_AUDIT_DIR, _CLI_SRC):
    _text = str(_path)
    if _text not in sys.path:
        sys.path.insert(0, _text)


def pytest_configure(config: object) -> None:
    """Register local-only Layer 2/3 markers without modifying `cli/pyproject.toml`."""
    add_line = getattr(config, "addinivalue_line", None)
    if add_line is None:
        return
    add_line(
        "markers",
        "local_dwarf: local Layer 2 DWARF SQLite check against attila_dwarf.sqlite",
    )
    add_line(
        "markers",
        "local_golden: local Layer 3 Ghidra decompilation golden-set verification",
    )
