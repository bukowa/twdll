"""Centralized SQLite database access layer for twdll.

By architectural contract (enforced via import-linter), this is the ONLY module
in twdll allowed to import `sqlite3`.
"""

from __future__ import annotations

import sqlite3
from pathlib import Path


def list_sqlite_tables(db_path: Path) -> tuple[str, ...]:
    """Return user table names in a SQLite file if it exists, otherwise empty."""
    if not db_path.is_file():
        return ()
    uri = f"file:{db_path.as_posix()}?mode=ro"
    with sqlite3.connect(uri, uri=True) as conn:
        query = (
            "SELECT name FROM sqlite_master "
            "WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
        )
        rows = conn.execute(query).fetchall()
    return tuple(str(row[0]) for row in rows)
