"""Unit tests for `twdll.database` DWARF DDL, staging lifecycle, and pass-scoped loaders."""

from __future__ import annotations

import sqlite3
from contextlib import closing
from pathlib import Path

import pytest

from twdll.database import (
    finalize_extract_staging_db,
    init_extract_staging_db,
    is_extract_db_complete,
    list_sqlite_tables,
    load_dwarf_function_signatures,
    load_dwarf_locals_and_inlines,
    load_dwarf_struct_layouts,
    load_dwarf_type_shells,
)
from twdll.domain import (
    BinaryName,
    Decompiler,
    HostOS,
    WorkspaceContext,
)


def _seed_sample_dwarf_rows(staging: Path) -> None:
    with closing(sqlite3.connect(staging)) as conn:
        conn.executemany(
            "INSERT INTO dwarf_types "
            "(type_id, kind, qualified_name, byte_size, target_type_id, array_count, "
            "containing_type_id, vtable_symbol, vtable_address, is_declaration, "
            "is_non_trivial_for_calls, is_stl_stub, is_referenced) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (
                    1,
                    "class",
                    "EMPIREBATTLE::UNIT",
                    64,
                    None,
                    None,
                    1,
                    "_ZTV4UNIT",
                    0x37A2FB0,
                    0,
                    1,
                    0,
                    1,
                ),
                (
                    2,
                    "struct",
                    "std::_Rb_tree_node_base",
                    32,
                    None,
                    None,
                    None,
                    None,
                    None,
                    0,
                    0,
                    1,
                    1,
                ),
                (3, "enum", "CA::STATE", 4, None, None, None, None, None, 0, 0, 0, 1),
            ),
        )
        conn.execute(
            "INSERT INTO dwarf_enumerators (enum_type_id, ordinal, name, value) "
            "VALUES (3, 0, 'ACTIVE', 1)"
        )
        conn.executemany(
            "INSERT INTO dwarf_members "
            "(member_id, parent_type_id, ordinal, name, member_type_id, byte_offset, byte_size, "
            "bit_offset, bit_size, is_artificial, is_flattened_base_member, is_layout_conflict) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (10, 1, 0, "_vptr$UNIT", 1, 0, 8, None, None, 1, 0, 0),
                (11, 1, 1, "m_conflict_arm", 3, 0, 4, None, None, 0, 0, 1),
                (12, 2, 0, "_M_color", 3, 0, 4, None, None, 0, 0, 0),
            ),
        )
        conn.executemany(
            "INSERT INTO dwarf_inheritance "
            "(inheritance_id, derived_type_id, base_type_id, byte_offset, vbase_vtable_offset, "
            "is_virtual, is_empty_base, has_tail_padding_overlap) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (20, 1, 3, 8, None, 0, 0, 0),
                (21, 1, 3, 0, None, 0, 1, 0),
                (22, 1, 3, None, 24, 1, 0, 0),
            ),
        )
        conn.execute(
            "INSERT INTO dwarf_vtable_entries "
            "(class_type_id, slot_index, method_name, linkage_name, function_id, "
            "target_address, is_pure_virtual) "
            "VALUES (1, 0, '~UNIT', '_ZN4UNITD2Ev', 100, 4200000, 0)"
        )
        conn.executemany(
            "INSERT INTO dwarf_functions "
            "(function_id, unit_id, address, byte_size, linkage_name, qualified_name, "
            "containing_type_id, return_type_id, vtable_slot_index, virtuality, is_inline, "
            "is_declaration, is_variadic, has_object_pointer, is_sret, decl_file, decl_line) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (
                    100,
                    1,
                    0x401000,
                    32,
                    "_ZN4UNIT3fooEv",
                    "EMPIREBATTLE::UNIT::foo",
                    1,
                    1,
                    0,
                    1,
                    0,
                    0,
                    0,
                    1,
                    1,
                    "unit.cpp",
                    10,
                ),
                (
                    101,
                    1,
                    None,
                    None,
                    None,
                    "EMPIREBATTLE::UNIT::inlined_decl",
                    1,
                    None,
                    None,
                    0,
                    1,
                    1,
                    0,
                    1,
                    0,
                    "unit.h",
                    20,
                ),
            ),
        )
        conn.execute(
            "INSERT INTO dwarf_parameters "
            "(function_id, ordinal, name, type_id, is_artificial, is_object_pointer, "
            "storage_kind, dwarf_reg, reg_name, stack_offset) "
            "VALUES (100, 0, 'this', 1, 1, 1, 'register', 4, 'RSI', NULL)"
        )
        conn.executemany(
            "INSERT INTO dwarf_variables "
            "(variable_id, function_id, name, type_id, range_index, start_address, "
            "end_address, first_use_offset, storage_kind, reg_name, stack_offset, const_value) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (300, 100, "local_reg", 3, 0, 0x401004, 0x401010, 4, "register", "RBX", None, None),
                (301, 100, "const_only", 3, 0, 0x401000, 0x401020, 0, "constant", None, None, 42),
            ),
        )
        conn.execute(
            "INSERT INTO dwarf_inlines "
            "(inline_id, caller_function_id, callee_function_id, callee_name, "
            "start_address, end_address, call_file, call_line) "
            "VALUES (400, 100, 101, 'EMPIREBATTLE::UNIT::inlined_decl', "
            "4198408, 4198420, 'unit.h', 20)"
        )
        conn.commit()


@pytest.mark.parametrize("host_os", [HostOS.LINUX, HostOS.WINDOWS])
def test_staging_db_lifecycle_and_wal_cleanup(tmp_path: Path, host_os: HostOS) -> None:
    _ = host_os
    paths = WorkspaceContext.from_root(tmp_path).paths
    staging = paths.extract_staging_db_path(Decompiler.GHIDRA, BinaryName.ATTILA_LINUX_64)
    final = paths.extract_db_path(Decompiler.GHIDRA, BinaryName.ATTILA_LINUX_64)

    init_extract_staging_db(staging)
    assert staging.is_file()
    assert not is_extract_db_complete(final, "abc123")

    tables = list_sqlite_tables(staging)
    expected_tables = (
        "dwarf_enumerators",
        "dwarf_functions",
        "dwarf_inheritance",
        "dwarf_inlines",
        "dwarf_members",
        "dwarf_parameters",
        "dwarf_types",
        "dwarf_units",
        "dwarf_variables",
        "dwarf_vtable_entries",
        "meta",
    )
    assert tables == expected_tables

    finalize_extract_staging_db(
        staging,
        final,
        binary_sha256="abc123",
        image_base=0x400000,
    )
    assert not staging.exists()
    assert not Path(f"{staging}-wal").exists()
    assert not Path(f"{staging}-shm").exists()
    assert final.is_file()
    assert is_extract_db_complete(final, "abc123")
    assert not is_extract_db_complete(final, "wrong-sha")


def test_pass_scoped_dwarf_loaders_filter_and_roundtrip(tmp_path: Path) -> None:
    paths = WorkspaceContext.from_root(tmp_path).paths
    staging = paths.extract_staging_db_path(Decompiler.GHIDRA, BinaryName.ATTILA_LINUX_64)
    init_extract_staging_db(staging)
    _seed_sample_dwarf_rows(staging)

    types, enumerators = load_dwarf_type_shells(staging)
    assert len(types) == 3
    assert types[0].is_non_trivial_for_calls is True
    assert types[1].is_stl_stub is True
    assert len(enumerators) == 1
    assert enumerators[0].name == "ACTIVE"

    vtables, members, inheritances = load_dwarf_struct_layouts(staging)
    assert len(vtables) == 1
    assert vtables[0].method_name == "~UNIT"
    assert [m.member_id for m in members] == [10]
    assert [i.inheritance_id for i in inheritances] == [20]

    functions, params = load_dwarf_function_signatures(staging)
    assert [f.function_id for f in functions] == [100]
    assert functions[0].is_sret is True
    assert len(params) == 1
    assert params[0].reg_name == "RSI"

    variables, inlines = load_dwarf_locals_and_inlines(staging)
    assert [v.variable_id for v in variables] == [300]
    assert len(inlines) == 1
    assert inlines[0].callee_name == "EMPIREBATTLE::UNIT::inlined_decl"
