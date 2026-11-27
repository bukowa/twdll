"""Pass 1: DWARF type-shell materialization and pointer-to-member sizing."""

from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass
from typing import Any

from twdll._ghidra import lifecycle
from twdll.domain import (
    DwarfEnumeratorRow,
    DwarfPassName,
    DwarfPassStats,
    DwarfTypeRow,
)

_PRIMITIVE_DATA_TYPES: dict[str, str] = {
    "void": "ghidra.program.model.data.VoidDataType",
    "bool": "ghidra.program.model.data.BooleanDataType",
    "_Bool": "ghidra.program.model.data.BooleanDataType",
    "char": "ghidra.program.model.data.CharDataType",
    "signed char": "ghidra.program.model.data.CharDataType",
    "unsigned char": "ghidra.program.model.data.UnsignedCharDataType",
    "short": "ghidra.program.model.data.ShortDataType",
    "short int": "ghidra.program.model.data.ShortDataType",
    "signed short": "ghidra.program.model.data.ShortDataType",
    "unsigned short": "ghidra.program.model.data.UnsignedShortDataType",
    "short unsigned int": "ghidra.program.model.data.UnsignedShortDataType",
    "int": "ghidra.program.model.data.IntegerDataType",
    "signed int": "ghidra.program.model.data.IntegerDataType",
    "wchar_t": "ghidra.program.model.data.IntegerDataType",
    "char32_t": "ghidra.program.model.data.IntegerDataType",
    "unsigned int": "ghidra.program.model.data.UnsignedIntegerDataType",
    "unsigned": "ghidra.program.model.data.UnsignedIntegerDataType",
    "long": "ghidra.program.model.data.LongLongDataType",
    "long int": "ghidra.program.model.data.LongLongDataType",
    "signed long": "ghidra.program.model.data.LongLongDataType",
    "long long": "ghidra.program.model.data.LongLongDataType",
    "long long int": "ghidra.program.model.data.LongLongDataType",
    "signed long long": "ghidra.program.model.data.LongLongDataType",
    "unsigned long": "ghidra.program.model.data.UnsignedLongLongDataType",
    "long unsigned int": "ghidra.program.model.data.UnsignedLongLongDataType",
    "long long unsigned int": "ghidra.program.model.data.UnsignedLongLongDataType",
    "unsigned long long": "ghidra.program.model.data.UnsignedLongLongDataType",
    "sizetype": "ghidra.program.model.data.UnsignedLongLongDataType",
    "float": "ghidra.program.model.data.FloatDataType",
    "double": "ghidra.program.model.data.DoubleDataType",
    "long double": "ghidra.program.model.data.LongDoubleDataType",
}

_SIZE_FALLBACK_TYPES: dict[int, str] = {
    1: "ghidra.program.model.data.UnsignedCharDataType",
    2: "ghidra.program.model.data.UnsignedShortDataType",
    4: "ghidra.program.model.data.UnsignedIntegerDataType",
    8: "ghidra.program.model.data.UnsignedLongLongDataType",
}


def split_cpp_namespace(qualified_name: str) -> tuple[tuple[str, ...], str]:
    """Split a C++ qualified name at top-level `::` into `(namespace_parts, leaf)`."""
    parts: list[str] = []
    angle = 0
    paren = 0
    start = 0
    i = 0
    n = len(qualified_name)
    while i < n:
        ch = qualified_name[i]
        if ch == "<":
            angle += 1
        elif ch == ">" and angle > 0:
            angle -= 1
        elif ch == "(":
            paren += 1
        elif ch == ")" and paren > 0:
            paren -= 1
        elif ch == ":" and angle == 0 and paren == 0 and qualified_name[i : i + 2] == "::":
            seg = qualified_name[start:i].replace("/", "_")
            if seg:
                parts.append(seg)
            i += 2
            start = i
            continue
        i += 1
    leaf = qualified_name[start:].replace("/", "_") or "unnamed"
    return tuple(parts), leaf


def make_category_path(category_path_cls: Any, ns_parts: tuple[str, ...]) -> Any:
    """Build a Ghidra `/dwarf/...` `CategoryPath` from namespace segments."""
    safe = [p[:120] for p in ns_parts if p]
    if not safe:
        return category_path_cls("/dwarf")
    return category_path_cls("/dwarf/" + "/".join(safe))


def map_primitive(qname: str, byte_size: int | None, dtm: Any) -> Any:
    """Map a DWARF primitive name/size to a Ghidra built-in `DataType`."""
    cls_name = _PRIMITIVE_DATA_TYPES.get(qname.strip())
    if cls_name is not None:
        return lifecycle.jclass(cls_name).dataType
    fallback_cls = _SIZE_FALLBACK_TYPES.get(byte_size or 4)
    if fallback_cls is not None:
        return lifecycle.jclass(fallback_cls).dataType
    void_dt = lifecycle.jclass("ghidra.program.model.data.VoidDataType").dataType
    return dtm.getPointer(void_dt)


def unwrap_type_row(tid: int | None, type_rows: dict[int, DwarfTypeRow]) -> DwarfTypeRow | None:
    """Follow `typedef`/`const`/`volatile` wrappers up to 8 hops."""
    cur = tid
    for _ in range(8):
        if cur is None or cur not in type_rows:
            return None
        t = type_rows[cur]
        if t.kind in ("typedef", "const", "volatile"):
            cur = t.target_type_id
            continue
        return t
    return None


def is_ptr_to_member_func(t: DwarfTypeRow, type_rows: dict[int, DwarfTypeRow]) -> bool:
    """Return True if a `ptr_to_member` targets a subroutine (16-byte Itanium ABI)."""
    if t.target_type_id is not None:
        target = unwrap_type_row(t.target_type_id, type_rows)
        if target is not None:
            return target.kind == "subroutine"
    return t.byte_size == 16


def unwrap_type_size(tid: int | None, type_rows: dict[int, DwarfTypeRow]) -> int | None:
    """Return ABI byte size after unwrapping `typedef`/`const`/`volatile`."""
    t = unwrap_type_row(tid, type_rows)
    if t is None:
        return None
    if t.kind in ("pointer", "reference", "rvalue_reference"):
        return 8
    if t.kind == "ptr_to_member":
        return 16 if is_ptr_to_member_func(t, type_rows) else 8
    return t.byte_size


@dataclass
class _Pass1Ctx:
    dtm: Any
    handler: Any
    category_path_cls: Any
    void_dt: Any
    type_map: dict[int, Any]
    type_rows: dict[int, DwarfTypeRow]
    enums_by_tid: dict[int, list[DwarfEnumeratorRow]]
    stats: lifecycle.StatsCollector
    ptm_func_dt: Any = None


def _get_ptr_to_member_func_dt(pctx: _Pass1Ctx) -> Any:
    if pctx.ptm_func_dt is not None:
        return pctx.ptm_func_dt
    struct_cls = lifecycle.jclass("ghidra.program.model.data.StructureDataType")
    long_long_dt = lifecycle.jclass("ghidra.program.model.data.LongLongDataType").dataType
    cat = pctx.category_path_cls("/dwarf/_abi")
    sd = struct_cls(cat, "__ptr_to_member_func", 16)
    sd.replaceAtOffset(0, pctx.dtm.getPointer(pctx.void_dt, 8), 8, "ptr", None)
    sd.replaceAtOffset(8, long_long_dt, 8, "adj", None)
    pctx.ptm_func_dt = pctx.dtm.addDataType(sd, pctx.handler)
    return pctx.ptm_func_dt


def _add_enum_shell(pctx: _Pass1Ctx, t: DwarfTypeRow, cat: Any, leaf: str) -> Any:
    enum_cls = lifecycle.jclass("ghidra.program.model.data.EnumDataType")
    sz = t.byte_size if t.byte_size in (1, 2, 4, 8) else 4
    ed = enum_cls(cat, leaf, sz)
    for ev in pctx.enums_by_tid.get(t.type_id, ()):
        val = ev.value
        if val >= (1 << 63):
            val -= 1 << 64
        elif val < -(1 << 63):
            val = val & ((1 << 63) - 1)
        try:
            ed.add(ev.name or f"ENUM_{ev.ordinal}", val)
        except Exception as exc:
            pctx.stats.record_error("enum_add_err", f"{t.qualified_name}::{ev.name}", exc)
    pctx.stats.inc("enums_created")
    return pctx.dtm.addDataType(ed, pctx.handler)


def _create_base_shell(pctx: _Pass1Ctx, t: DwarfTypeRow) -> None:
    ns, leaf = split_cpp_namespace(t.qualified_name)
    cat = make_category_path(pctx.category_path_cls, ns)
    match t.kind:
        case "primitive":
            pctx.type_map[t.type_id] = map_primitive(t.qualified_name, t.byte_size, pctx.dtm)
        case "enum":
            pctx.type_map[t.type_id] = _add_enum_shell(pctx, t, cat, leaf)
        case "struct" | "class":
            struct_cls = lifecycle.jclass("ghidra.program.model.data.StructureDataType")
            sd = struct_cls(cat, leaf, max(t.byte_size or 1, 1))
            pctx.type_map[t.type_id] = pctx.dtm.addDataType(sd, pctx.handler)
            pctx.stats.inc("structs_created")
        case "union":
            union_cls = lifecycle.jclass("ghidra.program.model.data.UnionDataType")
            pctx.type_map[t.type_id] = pctx.dtm.addDataType(union_cls(cat, leaf), pctx.handler)
            pctx.stats.inc("unions_created")
        case "subroutine":
            func_def_cls = lifecycle.jclass("ghidra.program.model.data.FunctionDefinitionDataType")
            pctx.type_map[t.type_id] = pctx.dtm.addDataType(func_def_cls(cat, leaf), pctx.handler)
            pctx.stats.inc("subroutines_created")
        case _:
            pass


def _resolve_ptr_or_array(pctx: _Pass1Ctx, t: DwarfTypeRow, target_dt: Any) -> bool:
    if t.kind in ("const", "volatile"):
        if target_dt is None:
            return False
        pctx.type_map[t.type_id] = target_dt
        return True
    if t.kind in ("pointer", "reference", "rvalue_reference"):
        if target_dt is not None:
            pctx.type_map[t.type_id] = pctx.dtm.getPointer(target_dt, 8)
            return True
        if t.target_type_id is None:
            pctx.type_map[t.type_id] = pctx.dtm.getPointer(pctx.void_dt, 8)
            return True
        return False
    if target_dt is None:
        return False
    array_cls = lifecycle.jclass("ghidra.program.model.data.ArrayDataType")
    cnt = max(t.array_count or 1, 1)
    elem_len = max(int(target_dt.getLength()), 1)
    if target_dt == pctx.void_dt:
        pctx.type_map[t.type_id] = pctx.dtm.getPointer(pctx.void_dt, 8)
    else:
        pctx.type_map[t.type_id] = array_cls(target_dt, cnt, elem_len)
    return True


def _resolve_derived_type(pctx: _Pass1Ctx, t: DwarfTypeRow) -> bool:
    target_dt = pctx.type_map.get(t.target_type_id) if t.target_type_id is not None else None
    if t.kind == "ptr_to_member":
        if is_ptr_to_member_func(t, pctx.type_rows):
            pctx.type_map[t.type_id] = _get_ptr_to_member_func_dt(pctx)
            pctx.stats.inc("ptr_to_member_func")
        else:
            ll_dt = lifecycle.jclass("ghidra.program.model.data.LongLongDataType").dataType
            pctx.type_map[t.type_id] = ll_dt
            pctx.stats.inc("ptr_to_member_data")
        return True
    if t.kind == "typedef":
        if target_dt is None:
            return False
        if target_dt == pctx.void_dt:
            pctx.type_map[t.type_id] = pctx.void_dt
        else:
            ns, leaf = split_cpp_namespace(t.qualified_name)
            cat = make_category_path(pctx.category_path_cls, ns)
            td_cls = lifecycle.jclass("ghidra.program.model.data.TypedefDataType")
            pctx.type_map[t.type_id] = pctx.dtm.addDataType(
                td_cls(cat, leaf, target_dt), pctx.handler
            )
        return True
    if t.kind in ("const", "volatile", "pointer", "reference", "rvalue_reference", "array"):
        return _resolve_ptr_or_array(pctx, t, target_dt)
    pctx.type_map[t.type_id] = pctx.void_dt
    return True


def pass1_type_shells(
    program: Any,
    types: tuple[DwarfTypeRow, ...],
    enumerators: tuple[DwarfEnumeratorRow, ...],
) -> tuple[dict[int, Any], dict[int, DwarfTypeRow], DwarfPassStats]:
    """Materialize DWARF type shells into Ghidra's `DataTypeManager`."""
    dtm = program.getDataTypeManager()
    handler = lifecycle.jclass("ghidra.program.model.data.DataTypeConflictHandler").KEEP_HANDLER
    category_path_cls = lifecycle.jclass("ghidra.program.model.data.CategoryPath")
    void_dt = lifecycle.jclass("ghidra.program.model.data.VoidDataType").dataType
    enums_by_tid: dict[int, list[DwarfEnumeratorRow]] = defaultdict(list)
    for e in enumerators:
        enums_by_tid[e.enum_type_id].append(e)

    pctx = _Pass1Ctx(
        dtm=dtm,
        handler=handler,
        category_path_cls=category_path_cls,
        void_dt=void_dt,
        type_map={},
        type_rows={t.type_id: t for t in types},
        enums_by_tid=enums_by_tid,
        stats=lifecycle.StatsCollector(DwarfPassName.PASS1_TYPES),
    )

    tx = int(program.startTransaction("Pass 1: DWARF type shells"))
    try:
        for t in types:
            _create_base_shell(pctx, t)
        unresolved = [t for t in types if t.type_id not in pctx.type_map]
        for _ in range(8):
            next_unresolved = [t for t in unresolved if not _resolve_derived_type(pctx, t)]
            if not next_unresolved or len(next_unresolved) == len(unresolved):
                unresolved = next_unresolved
                break
            unresolved = next_unresolved
        for t in unresolved:
            is_ptr = t.kind in ("pointer", "reference", "rvalue_reference")
            pctx.type_map[t.type_id] = dtm.getPointer(void_dt, 8) if is_ptr else void_dt
            pctx.stats.inc("unresolved_fallback")
        _finalize_subroutine_returns(pctx, types)
    finally:
        program.endTransaction(tx, True)
    program.clearUndo()
    return pctx.type_map, pctx.type_rows, pctx.stats.freeze()


def _finalize_subroutine_returns(pctx: _Pass1Ctx, types: tuple[DwarfTypeRow, ...]) -> None:
    for t in types:
        if t.kind != "subroutine":
            continue
        fd = pctx.type_map.get(t.type_id)
        if fd is None or not hasattr(fd, "setReturnType"):
            continue
        ret_dt = (
            pctx.type_map.get(t.target_type_id, pctx.void_dt)
            if t.target_type_id is not None
            else pctx.void_dt
        )
        try:
            fd.setReturnType(ret_dt)
        except Exception as exc:
            pctx.stats.record_error("subroutine_ret_err", t.qualified_name, exc)
