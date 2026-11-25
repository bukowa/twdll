"""Centralized SQLite database access layer for twdll.

By architectural contract (enforced via import-linter), this is the ONLY module
in twdll allowed to import `sqlite3`.
"""

from __future__ import annotations

import sqlite3
from contextlib import closing
from pathlib import Path
from typing import Final

from twdll.domain import (
    DwarfEnumeratorRow,
    DwarfFunctionRow,
    DwarfInheritanceRow,
    DwarfInlineRow,
    DwarfMemberRow,
    DwarfParameterRow,
    DwarfTypeRow,
    DwarfVariableRow,
    DwarfVtableEntryRow,
)

EXTRACT_STAGING_DDL: Final[str] = """
CREATE TABLE IF NOT EXISTS meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS dwarf_units (
    unit_id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    comp_dir TEXT NOT NULL,
    producer TEXT NOT NULL,
    language INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS dwarf_types (
    type_id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL,
    qualified_name TEXT NOT NULL,
    byte_size INTEGER,
    target_type_id INTEGER,
    array_count INTEGER,
    containing_type_id INTEGER,
    vtable_symbol TEXT,
    vtable_address INTEGER,
    is_declaration INTEGER NOT NULL DEFAULT 0,
    is_non_trivial_for_calls INTEGER NOT NULL DEFAULT 0,
    is_stl_stub INTEGER NOT NULL DEFAULT 0,
    is_referenced INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_dwarf_types_qualified_name
    ON dwarf_types (qualified_name);

CREATE TABLE IF NOT EXISTS dwarf_members (
    member_id INTEGER PRIMARY KEY,
    parent_type_id INTEGER NOT NULL,
    ordinal INTEGER NOT NULL,
    name TEXT NOT NULL,
    member_type_id INTEGER NOT NULL,
    byte_offset INTEGER NOT NULL,
    byte_size INTEGER NOT NULL,
    bit_offset INTEGER,
    bit_size INTEGER,
    is_artificial INTEGER NOT NULL DEFAULT 0,
    is_flattened_base_member INTEGER NOT NULL DEFAULT 0,
    is_layout_conflict INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_dwarf_members_parent
    ON dwarf_members (parent_type_id, ordinal);

CREATE TABLE IF NOT EXISTS dwarf_inheritance (
    inheritance_id INTEGER PRIMARY KEY,
    derived_type_id INTEGER NOT NULL,
    base_type_id INTEGER NOT NULL,
    byte_offset INTEGER,
    vbase_vtable_offset INTEGER,
    is_virtual INTEGER NOT NULL DEFAULT 0,
    is_empty_base INTEGER NOT NULL DEFAULT 0,
    has_tail_padding_overlap INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_dwarf_inheritance_derived
    ON dwarf_inheritance (derived_type_id);

CREATE TABLE IF NOT EXISTS dwarf_vtable_entries (
    class_type_id INTEGER NOT NULL,
    slot_index INTEGER NOT NULL,
    method_name TEXT NOT NULL,
    linkage_name TEXT,
    function_id INTEGER,
    target_address INTEGER,
    is_pure_virtual INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (class_type_id, slot_index)
);

CREATE TABLE IF NOT EXISTS dwarf_enumerators (
    enum_type_id INTEGER NOT NULL,
    ordinal INTEGER NOT NULL,
    name TEXT NOT NULL,
    value INTEGER NOT NULL,
    PRIMARY KEY (enum_type_id, ordinal)
);

CREATE TABLE IF NOT EXISTS dwarf_functions (
    function_id INTEGER PRIMARY KEY,
    unit_id INTEGER NOT NULL,
    address INTEGER,
    byte_size INTEGER,
    linkage_name TEXT,
    qualified_name TEXT NOT NULL,
    containing_type_id INTEGER,
    return_type_id INTEGER,
    vtable_slot_index INTEGER,
    virtuality INTEGER NOT NULL DEFAULT 0,
    is_inline INTEGER NOT NULL DEFAULT 0,
    is_declaration INTEGER NOT NULL DEFAULT 0,
    is_variadic INTEGER NOT NULL DEFAULT 0,
    has_object_pointer INTEGER NOT NULL DEFAULT 0,
    is_sret INTEGER NOT NULL DEFAULT 0,
    decl_file TEXT,
    decl_line INTEGER
);
CREATE INDEX IF NOT EXISTS idx_dwarf_functions_address
    ON dwarf_functions (address);
CREATE INDEX IF NOT EXISTS idx_dwarf_functions_linkage_name
    ON dwarf_functions (linkage_name);

CREATE TABLE IF NOT EXISTS dwarf_parameters (
    function_id INTEGER NOT NULL,
    ordinal INTEGER NOT NULL,
    name TEXT,
    type_id INTEGER NOT NULL,
    is_artificial INTEGER NOT NULL DEFAULT 0,
    is_object_pointer INTEGER NOT NULL DEFAULT 0,
    storage_kind TEXT NOT NULL,
    dwarf_reg INTEGER,
    reg_name TEXT,
    stack_offset INTEGER,
    PRIMARY KEY (function_id, ordinal)
);

CREATE TABLE IF NOT EXISTS dwarf_variables (
    variable_id INTEGER PRIMARY KEY,
    function_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type_id INTEGER NOT NULL,
    range_index INTEGER NOT NULL DEFAULT 0,
    start_address INTEGER NOT NULL,
    end_address INTEGER NOT NULL,
    first_use_offset INTEGER NOT NULL,
    storage_kind TEXT NOT NULL,
    reg_name TEXT,
    stack_offset INTEGER,
    const_value INTEGER
);
CREATE INDEX IF NOT EXISTS idx_dwarf_variables_function
    ON dwarf_variables (function_id);

CREATE TABLE IF NOT EXISTS dwarf_inlines (
    inline_id INTEGER PRIMARY KEY,
    caller_function_id INTEGER NOT NULL,
    callee_function_id INTEGER,
    callee_name TEXT NOT NULL,
    start_address INTEGER NOT NULL,
    end_address INTEGER NOT NULL,
    call_file TEXT,
    call_line INTEGER
);
CREATE INDEX IF NOT EXISTS idx_dwarf_inlines_caller
    ON dwarf_inlines (caller_function_id);
CREATE INDEX IF NOT EXISTS idx_dwarf_inlines_start_address
    ON dwarf_inlines (start_address);
"""


def _unlink_staging_files(staging_path: Path) -> None:
    for candidate in (
        staging_path,
        Path(f"{staging_path}-wal"),
        Path(f"{staging_path}-shm"),
    ):
        candidate.unlink(missing_ok=True)


def init_extract_staging_db(staging_path: Path) -> None:
    """Create a fresh `.sqlite.staging` database initialized with the extract DDL."""
    _unlink_staging_files(staging_path)
    staging_path.parent.mkdir(parents=True, exist_ok=True)
    with closing(sqlite3.connect(staging_path)) as conn:
        conn.execute("PRAGMA journal_mode = WAL")
        conn.execute("PRAGMA synchronous = NORMAL")
        conn.executescript(EXTRACT_STAGING_DDL)
        conn.commit()


def finalize_extract_staging_db(
    staging_path: Path,
    final_path: Path,
    *,
    binary_sha256: str,
    image_base: int,
) -> None:
    """Write completion metadata, checkpoint WAL, and atomically rename to `final_path`."""
    final_path.parent.mkdir(parents=True, exist_ok=True)
    with closing(sqlite3.connect(staging_path)) as conn:
        conn.executemany(
            "INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)",
            (
                ("completed", "true"),
                ("binary_sha256", binary_sha256),
                ("image_base", str(image_base)),
            ),
        )
        conn.commit()
        conn.execute("PRAGMA wal_checkpoint(TRUNCATE)")
        conn.execute("PRAGMA journal_mode = DELETE")
    Path(f"{staging_path}-wal").unlink(missing_ok=True)
    Path(f"{staging_path}-shm").unlink(missing_ok=True)
    staging_path.replace(final_path)


def is_extract_db_complete(db_path: Path, expected_sha256: str) -> bool:
    """Return True if `db_path` exists and `meta` marks completion for `expected_sha256`."""
    if not db_path.is_file():
        return False
    uri = f"file:{db_path.resolve().as_posix()}?mode=ro"
    try:
        with closing(sqlite3.connect(uri, uri=True)) as conn:
            rows = conn.execute(
                "SELECT key, value FROM meta WHERE key IN ('completed', 'binary_sha256')"
            ).fetchall()
    except (sqlite3.Error, OSError):
        return False
    meta = {str(key): str(val) for key, val in rows}
    return meta.get("completed") == "true" and meta.get("binary_sha256") == expected_sha256


def list_sqlite_tables(db_path: Path) -> tuple[str, ...]:
    """Return user table names in a SQLite file if it exists, otherwise empty."""
    if not db_path.is_file():
        return ()
    uri = f"file:{db_path.resolve().as_posix()}?mode=ro"
    with closing(sqlite3.connect(uri, uri=True)) as conn:
        query = (
            "SELECT name FROM sqlite_master "
            "WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
        )
        rows = conn.execute(query).fetchall()
    return tuple(str(row[0]) for row in rows)


def _row_to_dwarf_type(row: tuple[object, ...]) -> DwarfTypeRow:
    return DwarfTypeRow(
        type_id=int(str(row[0])),
        kind=str(row[1]),
        qualified_name=str(row[2]),
        byte_size=int(str(row[3])) if row[3] is not None else None,
        target_type_id=int(str(row[4])) if row[4] is not None else None,
        array_count=int(str(row[5])) if row[5] is not None else None,
        containing_type_id=int(str(row[6])) if row[6] is not None else None,
        vtable_symbol=str(row[7]) if row[7] is not None else None,
        vtable_address=int(str(row[8])) if row[8] is not None else None,
        is_declaration=bool(row[9]),
        is_non_trivial_for_calls=bool(row[10]),
        is_stl_stub=bool(row[11]),
        is_referenced=bool(row[12]),
    )


def load_dwarf_type_shells(
    staging_path: Path,
) -> tuple[tuple[DwarfTypeRow, ...], tuple[DwarfEnumeratorRow, ...]]:
    """Pass 1 loader: stream `dwarf_types` and `dwarf_enumerators` from `staging_path`."""
    with closing(sqlite3.connect(staging_path)) as conn:
        type_rows = conn.execute(
            "SELECT type_id, kind, qualified_name, byte_size, target_type_id, array_count, "
            "containing_type_id, vtable_symbol, vtable_address, is_declaration, "
            "is_non_trivial_for_calls, is_stl_stub, is_referenced "
            "FROM dwarf_types ORDER BY type_id"
        ).fetchall()
        enum_rows = conn.execute(
            "SELECT enum_type_id, ordinal, name, value "
            "FROM dwarf_enumerators ORDER BY enum_type_id, ordinal"
        ).fetchall()
    types = tuple(_row_to_dwarf_type(row) for row in type_rows)
    enumerators = tuple(
        DwarfEnumeratorRow(
            enum_type_id=int(str(row[0])),
            ordinal=int(str(row[1])),
            name=str(row[2]),
            value=int(str(row[3])),
        )
        for row in enum_rows
    )
    return types, enumerators


def _row_to_dwarf_member(row: tuple[object, ...]) -> DwarfMemberRow:
    return DwarfMemberRow(
        member_id=int(str(row[0])),
        parent_type_id=int(str(row[1])),
        ordinal=int(str(row[2])),
        name=str(row[3]),
        member_type_id=int(str(row[4])),
        byte_offset=int(str(row[5])),
        byte_size=int(str(row[6])),
        bit_offset=int(str(row[7])) if row[7] is not None else None,
        bit_size=int(str(row[8])) if row[8] is not None else None,
        is_artificial=bool(row[9]),
        is_flattened_base_member=bool(row[10]),
        is_layout_conflict=bool(row[11]),
    )


def _row_to_dwarf_inheritance(row: tuple[object, ...]) -> DwarfInheritanceRow:
    return DwarfInheritanceRow(
        inheritance_id=int(str(row[0])),
        derived_type_id=int(str(row[1])),
        base_type_id=int(str(row[2])),
        byte_offset=int(str(row[3])) if row[3] is not None else None,
        vbase_vtable_offset=int(str(row[4])) if row[4] is not None else None,
        is_virtual=bool(row[5]),
        is_empty_base=bool(row[6]),
        has_tail_padding_overlap=bool(row[7]),
    )


def load_dwarf_struct_layouts(
    staging_path: Path,
) -> tuple[
    tuple[DwarfVtableEntryRow, ...],
    tuple[DwarfMemberRow, ...],
    tuple[DwarfInheritanceRow, ...],
]:
    """Pass 2 loader: stream vtable slots and non-conflicting referenced struct layouts."""
    with closing(sqlite3.connect(staging_path)) as conn:
        vtable_rows = conn.execute(
            "SELECT class_type_id, slot_index, method_name, linkage_name, "
            "function_id, target_address, is_pure_virtual "
            "FROM dwarf_vtable_entries ORDER BY class_type_id, slot_index"
        ).fetchall()
        member_rows = conn.execute(
            "SELECT m.member_id, m.parent_type_id, m.ordinal, m.name, m.member_type_id, "
            "m.byte_offset, m.byte_size, m.bit_offset, m.bit_size, m.is_artificial, "
            "m.is_flattened_base_member, m.is_layout_conflict "
            "FROM dwarf_members AS m "
            "INNER JOIN dwarf_types AS t ON t.type_id = m.parent_type_id "
            "WHERE t.is_referenced = 1 AND t.is_stl_stub = 0 AND t.is_declaration = 0 "
            "AND m.is_layout_conflict = 0 "
            "ORDER BY m.parent_type_id, m.byte_offset, m.ordinal"
        ).fetchall()
        inheritance_rows = conn.execute(
            "SELECT i.inheritance_id, i.derived_type_id, i.base_type_id, i.byte_offset, "
            "i.vbase_vtable_offset, i.is_virtual, i.is_empty_base, i.has_tail_padding_overlap "
            "FROM dwarf_inheritance AS i "
            "INNER JOIN dwarf_types AS t ON t.type_id = i.derived_type_id "
            "WHERE t.is_referenced = 1 AND t.is_stl_stub = 0 AND t.is_declaration = 0 "
            "AND i.is_virtual = 0 AND i.is_empty_base = 0 AND i.has_tail_padding_overlap = 0 "
            "ORDER BY i.derived_type_id, i.byte_offset, i.inheritance_id"
        ).fetchall()
    vtables = tuple(
        DwarfVtableEntryRow(
            class_type_id=int(str(r[0])),
            slot_index=int(str(r[1])),
            method_name=str(r[2]),
            linkage_name=str(r[3]) if r[3] is not None else None,
            function_id=int(str(r[4])) if r[4] is not None else None,
            target_address=int(str(r[5])) if r[5] is not None else None,
            is_pure_virtual=bool(r[6]),
        )
        for r in vtable_rows
    )
    return (
        vtables,
        tuple(_row_to_dwarf_member(r) for r in member_rows),
        tuple(_row_to_dwarf_inheritance(r) for r in inheritance_rows),
    )


def _row_to_dwarf_function(row: tuple[object, ...]) -> DwarfFunctionRow:
    return DwarfFunctionRow(
        function_id=int(str(row[0])),
        unit_id=int(str(row[1])),
        address=int(str(row[2])) if row[2] is not None else None,
        byte_size=int(str(row[3])) if row[3] is not None else None,
        linkage_name=str(row[4]) if row[4] is not None else None,
        qualified_name=str(row[5]),
        containing_type_id=int(str(row[6])) if row[6] is not None else None,
        return_type_id=int(str(row[7])) if row[7] is not None else None,
        vtable_slot_index=int(str(row[8])) if row[8] is not None else None,
        virtuality=int(str(row[9])),
        is_inline=bool(row[10]),
        is_declaration=bool(row[11]),
        is_variadic=bool(row[12]),
        has_object_pointer=bool(row[13]),
        is_sret=bool(row[14]),
        decl_file=str(row[15]) if row[15] is not None else None,
        decl_line=int(str(row[16])) if row[16] is not None else None,
    )


def _row_to_dwarf_parameter(row: tuple[object, ...]) -> DwarfParameterRow:
    return DwarfParameterRow(
        function_id=int(str(row[0])),
        ordinal=int(str(row[1])),
        name=str(row[2]) if row[2] is not None else None,
        type_id=int(str(row[3])),
        is_artificial=bool(row[4]),
        is_object_pointer=bool(row[5]),
        storage_kind=str(row[6]),
        dwarf_reg=int(str(row[7])) if row[7] is not None else None,
        reg_name=str(row[8]) if row[8] is not None else None,
        stack_offset=int(str(row[9])) if row[9] is not None else None,
    )


def load_dwarf_function_signatures(
    staging_path: Path,
) -> tuple[tuple[DwarfFunctionRow, ...], tuple[DwarfParameterRow, ...]]:
    """Pass 3 loader: stream concrete functions (`address IS NOT NULL`) and their parameters."""
    with closing(sqlite3.connect(staging_path)) as conn:
        func_rows = conn.execute(
            "SELECT function_id, unit_id, address, byte_size, linkage_name, qualified_name, "
            "containing_type_id, return_type_id, vtable_slot_index, virtuality, is_inline, "
            "is_declaration, is_variadic, has_object_pointer, is_sret, decl_file, decl_line "
            "FROM dwarf_functions "
            "WHERE address IS NOT NULL AND is_declaration = 0 "
            "ORDER BY address, function_id"
        ).fetchall()
        param_rows = conn.execute(
            "SELECT p.function_id, p.ordinal, p.name, p.type_id, p.is_artificial, "
            "p.is_object_pointer, p.storage_kind, p.dwarf_reg, p.reg_name, p.stack_offset "
            "FROM dwarf_parameters AS p "
            "INNER JOIN dwarf_functions AS f ON f.function_id = p.function_id "
            "WHERE f.address IS NOT NULL AND f.is_declaration = 0 "
            "ORDER BY p.function_id, p.ordinal"
        ).fetchall()
    return (
        tuple(_row_to_dwarf_function(r) for r in func_rows),
        tuple(_row_to_dwarf_parameter(r) for r in param_rows),
    )


def _row_to_dwarf_variable(row: tuple[object, ...]) -> DwarfVariableRow:
    return DwarfVariableRow(
        variable_id=int(str(row[0])),
        function_id=int(str(row[1])),
        name=str(row[2]),
        type_id=int(str(row[3])),
        range_index=int(str(row[4])),
        start_address=int(str(row[5])),
        end_address=int(str(row[6])),
        first_use_offset=int(str(row[7])),
        storage_kind=str(row[8]),
        reg_name=str(row[9]) if row[9] is not None else None,
        stack_offset=int(str(row[10])) if row[10] is not None else None,
        const_value=int(str(row[11])) if row[11] is not None else None,
    )


def load_dwarf_locals_and_inlines(
    staging_path: Path,
) -> tuple[tuple[DwarfVariableRow, ...], tuple[DwarfInlineRow, ...]]:
    """Pass 4 loader: stream stack/register local variable ranges and inlined call sites."""
    with closing(sqlite3.connect(staging_path)) as conn:
        var_rows = conn.execute(
            "SELECT variable_id, function_id, name, type_id, range_index, start_address, "
            "end_address, first_use_offset, storage_kind, reg_name, stack_offset, const_value "
            "FROM dwarf_variables "
            "WHERE storage_kind IN ('stack', 'register') "
            "ORDER BY function_id, variable_id, range_index"
        ).fetchall()
        inline_rows = conn.execute(
            "SELECT inline_id, caller_function_id, callee_function_id, callee_name, "
            "start_address, end_address, call_file, call_line "
            "FROM dwarf_inlines "
            "ORDER BY start_address, inline_id"
        ).fetchall()
    inlines = tuple(
        DwarfInlineRow(
            inline_id=int(str(r[0])),
            caller_function_id=int(str(r[1])),
            callee_function_id=int(str(r[2])) if r[2] is not None else None,
            callee_name=str(r[3]),
            start_address=int(str(r[4])),
            end_address=int(str(r[5])),
            call_file=str(r[6]) if r[6] is not None else None,
            call_line=int(str(r[7])) if r[7] is not None else None,
        )
        for r in inline_rows
    )
    return tuple(_row_to_dwarf_variable(r) for r in var_rows), inlines
