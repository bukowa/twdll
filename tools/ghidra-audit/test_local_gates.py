"""Local Layer 2 (DWARF SQLite) and Layer 3 (Ghidra Golden Set) verification gates."""

from __future__ import annotations

import json
import re
import sqlite3
from contextlib import closing

import audit_common
import pytest
from twdll import _ghidra
from twdll.domain import WorkspaceContext

_REQUIRED_SINGLETONS = (
    "EMPIRE_APP_MODULE::s_instance",
    "CAMPAIGN_UI::s_ui",
    "FRONT_END_UI::s_ui",
    "WS_CONTEXT::m_e_engine",
    "WS_PLATFORM::m_pf_device",
)
_VALID_ABI_CLASSES = {
    "int",
    "sse",
    "int,int",
    "sse,sse",
    "sse,int",
    "int,sse",
    "memory",
}


def _unwrap_sqlite_type(
    types_by_id: dict[int, tuple[str, int | None, int | None]],
    tid: int | None,
) -> tuple[str, int | None] | None:
    cur = tid
    for _ in range(12):
        if cur is None or cur not in types_by_id:
            return None
        kind, bsz, target_id = types_by_id[cur]
        if kind in ("typedef", "const", "volatile"):
            cur = target_id
            continue
        return kind, bsz
    return None


@pytest.mark.local_dwarf
def test_local_dwarf_sqlite_invariants() -> None:
    db_path = audit_common.DEFAULT_DWARF_DB
    if not db_path.is_file():
        pytest.fail(f"Local DWARF SQLite not found: {db_path}")

    uri = f"file:{db_path.resolve().as_posix()}?mode=ro"
    with closing(sqlite3.connect(uri, uri=True)) as conn:
        for table in (
            "dwarf_units",
            "dwarf_types",
            "dwarf_members",
            "dwarf_inheritance",
            "dwarf_vtable_entries",
            "dwarf_enumerators",
            "dwarf_functions",
            "dwarf_parameters",
            "dwarf_variables",
            "dwarf_inlines",
            "dwarf_globals",
        ):
            cnt = int(conn.execute(f"SELECT COUNT(*) FROM {table}").fetchone()[0])
            assert cnt > 0, f"Expected populated table {table}"

        types_by_id = {
            int(tid): (
                str(kind),
                (int(bsz) if bsz is not None else None),
                (int(tt) if tt is not None else None),
            )
            for tid, kind, bsz, tt in conn.execute(
                "SELECT type_id, kind, byte_size, target_type_id FROM dwarf_types"
            ).fetchall()
        }

        # 0 concrete <= 16-byte functions with param[0] == RDI (5) and is_sret == 1
        sret_rdi_rows = conn.execute(
            """
            SELECT f.function_id, f.address, f.qualified_name, f.return_type_id
            FROM dwarf_functions AS f
            INNER JOIN dwarf_parameters AS p
              ON p.function_id = f.function_id AND p.ordinal = 0
            WHERE f.address IS NOT NULL AND f.is_declaration = 0
              AND f.is_sret = 1 AND p.dwarf_reg = 5
            """
        ).fetchall()
        bad_sret = []
        for fid, addr, qname, rt_id in sret_rdi_rows:
            unwrapped = _unwrap_sqlite_type(
                types_by_id, int(rt_id) if rt_id is not None else None
            )
            if (
                unwrapped is not None
                and unwrapped[1] is not None
                and unwrapped[1] <= 16
            ):
                bad_sret.append((fid, hex(int(addr)), qname, unwrapped[1]))
        assert not bad_sret, (
            f"<= 16B functions with RDI param[0] marked is_sret: {bad_sret[:5]}"
        )

        # 0 <= 16B struct/class/union types with RSI-only witnesses marked trivial
        missed_non_trivial = conn.execute(
            """
            SELECT t.type_id, t.qualified_name, t.byte_size
            FROM dwarf_functions AS f
            INNER JOIN dwarf_types AS t ON f.return_type_id = t.type_id
            LEFT JOIN dwarf_parameters AS p
              ON p.function_id = f.function_id AND p.ordinal = 0
            WHERE f.address IS NOT NULL AND f.is_declaration = 0
              AND t.kind IN ('struct', 'class', 'union')
              AND t.byte_size IS NOT NULL AND t.byte_size <= 16
              AND t.is_non_trivial_for_calls = 0
            GROUP BY t.type_id
            HAVING SUM(CASE WHEN p.dwarf_reg = 4 THEN 1 ELSE 0 END) > 0
               AND SUM(CASE WHEN p.dwarf_reg = 5 THEN 1 ELSE 0 END) = 0
            """
        ).fetchall()
        assert not missed_non_trivial, (
            f"<= 16B RSI-witnessed types left trivial: {missed_non_trivial[:5]}"
        )

        # All 5 required Warscape singletons in dwarf_globals (is_referenced == 1)
        for name_suffix in _REQUIRED_SINGLETONS:
            rows = conn.execute(
                """
                SELECT g.address, g.qualified_name, t.is_referenced
                FROM dwarf_globals AS g
                INNER JOIN dwarf_types AS t ON t.type_id = g.type_id
                WHERE g.qualified_name = ? OR g.qualified_name LIKE ?
                """,
                (name_suffix, f"%::{name_suffix}"),
            ).fetchall()
            assert rows, f"Missing singleton {name_suffix} in dwarf_globals"
            assert any(int(r[0]) > 0 and int(r[2]) == 1 for r in rows), (
                f"Singleton {name_suffix} lacks non-zero referenced type: {rows}"
            )

        # ptr_to_member sizing invariant
        for tid, _kind, bsz, target_id in conn.execute(
            "SELECT type_id, kind, byte_size, target_type_id "
            "FROM dwarf_types WHERE kind = 'ptr_to_member'"
        ).fetchall():
            target = _unwrap_sqlite_type(
                types_by_id, int(target_id) if target_id is not None else None
            )
            expected_sz = (
                16 if (target is not None and target[0] == "subroutine") else 8
            )
            assert bsz == expected_sz, (
                f"ptr_to_member {tid} size={bsz}, expected={expected_sz}"
            )

        # Valid abi_class values
        bad_abi = conn.execute(
            "SELECT type_id, kind, abi_class FROM dwarf_types "
            "WHERE abi_class IS NOT NULL"
        ).fetchall()
        invalid = [r for r in bad_abi if r[2] not in _VALID_ABI_CLASSES]
        assert not invalid, f"Invalid abi_class values: {invalid[:5]}"


@pytest.mark.local_golden
def test_local_golden_decompilation_and_budgets() -> None:
    golden_path = audit_common.AUDIT_DIR / "golden_set.json"
    golden = json.loads(golden_path.read_text(encoding="utf-8"))
    ctx = WorkspaceContext.from_root(audit_common.REPO_ROOT)
    cache_dir, meta = audit_common.ensure_cached_snapshot(
        ctx, audit_common.DEFAULT_DWARF_DB, audit_common.DEFAULT_CACHE_DIR
    )

    pass_stats = meta["pass_stats"]
    budgets = golden["pass_error_budgets"]
    assert (
        pass_stats["pass1"]["total_errors"]
        if "total_errors" in pass_stats["pass1"]
        else sum(pass_stats["pass1"]["error_counts"].values())
        <= budgets["pass1_types"]["total"]
    )
    for pass_key, budget_map in (
        ("pass2", budgets["pass2_layouts"]),
        ("pass3", budgets["pass3_signatures"]),
        ("pass4", budgets["pass4_locals"]),
    ):
        err_counts = pass_stats[pass_key]["error_counts"]
        for err_name, limit in budget_map.items():
            actual = err_counts.get(err_name, 0)
            assert actual <= limit, (
                f"{pass_key}.{err_name}={actual} exceeded budget {limit}"
            )

    with audit_common.open_cached_program_readonly(cache_dir) as program:
        decomp = _ghidra.jclass("ghidra.app.decompiler.DecompInterface")()
        monitor = _ghidra.jclass("ghidra.util.task.ConsoleTaskMonitor")()
        fm = program.getFunctionManager()
        addr_space = program.getAddressFactory().getDefaultAddressSpace()
        decomp.openProgram(program)
        try:
            for item in golden["functions"]:
                addr = int(item["address"], 16)
                gaddr = addr_space.getAddress(addr)
                gh_func = fm.getFunctionAt(gaddr)
                assert gh_func is not None, (
                    f"Missing Ghidra function at {item['address']}"
                )
                res = decomp.decompileFunction(gh_func, 60, monitor)
                ok = bool(res is not None and res.decompileCompleted())
                if item.get("require_decompile_ok"):
                    assert ok, f"Decompile failed at {item['address']} ({item['name']})"
                c_code = str(res.getDecompiledFunction().getC()) if ok else ""
                for pat in item.get("must_contain_regex", []):
                    assert re.search(pat, c_code), (
                        f"{item['address']} missing pattern {pat!r}"
                    )
                for pat in item.get("must_not_contain_regex", []):
                    assert not re.search(pat, c_code), (
                        f"{item['address']} matched forbidden pattern {pat!r}"
                    )
        finally:
            decomp.dispose()
