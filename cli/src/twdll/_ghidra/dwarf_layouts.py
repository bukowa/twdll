"""Pass 2: vtable synthesis, union/struct layout population, and global stamping."""

from __future__ import annotations

from collections import defaultdict, deque
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Any

from twdll._ghidra import lifecycle
from twdll._ghidra.dwarf_types import make_category_path, split_cpp_namespace, unwrap_type_row
from twdll.domain import (
    DwarfFunctionRow,
    DwarfGlobalRow,
    DwarfInheritanceRow,
    DwarfMemberRow,
    DwarfParameterRow,
    DwarfPassName,
    DwarfPassStats,
    DwarfTypeRow,
    DwarfVtableEntryRow,
)


@dataclass(frozen=True)
class DwarfLayoutInputs:
    type_map: dict[int, Any]
    type_rows: dict[int, DwarfTypeRow]
    members: tuple[DwarfMemberRow, ...]
    inheritances: tuple[DwarfInheritanceRow, ...]
    vtables: tuple[DwarfVtableEntryRow, ...]
    vtable_funcs: dict[int, DwarfFunctionRow]
    vtable_params: dict[int, tuple[DwarfParameterRow, ...]]
    globals_: tuple[DwarfGlobalRow, ...]


@dataclass
class _LayoutCtx:
    program: Any
    dtm: Any
    listing: Any
    addr_space: Any
    mem: Any
    inputs: DwarfLayoutInputs
    vt_by_class: dict[int, list[DwarfVtableEntryRow]]
    direct_members: dict[int, list[DwarfMemberRow]]
    direct_inhs: dict[int, list[DwarfInheritanceRow]]
    stats: lifecycle.StatsCollector
    void_dt: Any
    default_ptr_dt: Any
    category_path_cls: Any
    struct_cls: Any
    func_def_cls: Any
    param_def_cls: Any
    bitfield_cls: Any
    conflict_replace: Any
    undef1_dt: Any
    array_cls: Any
    vtbl_ptr_map: dict[int, Any]


def _valid_bitfield_base(mdt: Any, sz: int, bitfield_cls: Any) -> Any:
    if bitfield_cls.isValidBaseDataType(mdt):
        return mdt
    fallback_cls = {
        1: "ghidra.program.model.data.UnsignedCharDataType",
        2: "ghidra.program.model.data.UnsignedShortDataType",
        4: "ghidra.program.model.data.UnsignedIntegerDataType",
    }.get(sz, "ghidra.program.model.data.UnsignedLongLongDataType")
    return lifecycle.jclass(fallback_cls).dataType


def _populate_vtable_slot_fn(
    lctx: _LayoutCtx,
    fdef: Any,
    ctid: int,
    entry: DwarfVtableEntryRow,
) -> None:
    if entry.function_id is not None and entry.function_id in lctx.inputs.vtable_funcs:
        f = lctx.inputs.vtable_funcs[entry.function_id]
        ret_dt = (
            lctx.inputs.type_map.get(f.return_type_id, lctx.void_dt)
            if f.return_type_id is not None
            else lctx.void_dt
        )
        pdefs: list[Any] = []
        if f.is_sret:
            ret_ptr = lctx.dtm.getPointer(ret_dt, 8)
            pdefs.append(lctx.param_def_cls("__return_storage_ptr__", ret_ptr, None))
            fdef.setReturnType(ret_ptr)
        else:
            fdef.setReturnType(ret_dt)
        for pr in lctx.inputs.vtable_params.get(entry.function_id, ()):
            if pr.name == "__in_chrg" and pr.is_artificial and not pr.is_object_pointer:
                continue
            pdt = lctx.inputs.type_map.get(pr.type_id, lctx.default_ptr_dt)
            if pdt == lctx.void_dt:
                pdt = lctx.default_ptr_dt
            ut = unwrap_type_row(pr.type_id, lctx.inputs.type_rows)
            if (
                ut is not None
                and ut.kind in ("struct", "class", "union")
                and ut.is_non_trivial_for_calls
            ):
                pdt = lctx.dtm.getPointer(pdt, 8)
            pname = pr.name or ("this" if pr.is_object_pointer else f"param_{pr.ordinal}")
            pdefs.append(lctx.param_def_cls(pname, pdt, None))
        fdef.setArguments(pdefs)
        return

    this_dt = lctx.inputs.type_map.get(ctid)
    this_ptr = lctx.dtm.getPointer(this_dt, 8) if this_dt is not None else lctx.default_ptr_dt
    fdef.setReturnType(lctx.void_dt)
    fdef.setArguments([lctx.param_def_cls("this", this_ptr, None)])


def _stamp_vtable_at_addr(
    lctx: _LayoutCtx,
    trow: DwarfTypeRow,
    added_vt: Any,
    ztv_ranges: list[tuple[int, int]],
) -> None:
    if trow.vtable_address is None:
        return
    try:
        addr = lctx.addr_space.getAddress(trow.vtable_address)
        vt_len = int(added_vt.getLength())
        end_addr = addr.add(vt_len - 1)
        block = lctx.mem.getBlock(addr)
        if block is not None and block.isInitialized():
            lctx.listing.clearCodeUnits(addr, end_addr, False)
            lctx.listing.createData(addr, added_vt)
            ztv_ranges.append((trow.vtable_address, trow.vtable_address + vt_len))
            lctx.stats.inc("vtbl_stamped")
    except Exception as exc:
        lctx.stats.record_error("vtbl_stamp_err", trow.qualified_name, exc)


def _build_vtable_structs(lctx: _LayoutCtx) -> tuple[dict[int, Any], list[tuple[int, int]]]:
    vtbl_ptr_map: dict[int, Any] = {}
    ztv_ranges: list[tuple[int, int]] = []
    for ctid, entries in lctx.vt_by_class.items():
        trow = lctx.inputs.type_rows.get(ctid)
        if trow is None:
            continue
        ns, leaf = split_cpp_namespace(trow.qualified_name)
        cat = make_category_path(lctx.category_path_cls, ns)
        max_slot = max(e.slot_index for e in entries)
        vt_struct = lctx.struct_cls(cat, f"{leaf}_vtbl", (max_slot + 1) * 8)
        used_names: set[str] = set()
        for e in entries:
            _, mleaf = split_cpp_namespace(e.method_name)
            fdef = lctx.func_def_cls(cat, f"{leaf}::{mleaf}_{e.slot_index}")
            _populate_vtable_slot_fn(lctx, fdef, ctid, e)
            slot_ptr = lctx.dtm.getPointer(fdef, 8)
            off = e.slot_index * 8
            fname = mleaf if mleaf not in used_names else f"{mleaf}_{e.slot_index}"
            used_names.add(fname)
            if off >= 0 and off + 8 <= vt_struct.getLength():
                try:
                    vt_struct.replaceAtOffset(off, slot_ptr, 8, fname, None)
                    lctx.stats.inc("vtbl_slots")
                except Exception as exc:
                    lctx.stats.record_error("vtbl_slot_err", f"{trow.qualified_name}::{fname}", exc)
        added_vt = lctx.dtm.addDataType(vt_struct, lctx.conflict_replace)
        vtbl_ptr_map[ctid] = lctx.dtm.getPointer(added_vt, 8)
        lctx.stats.inc("vtbl_structs")
        _stamp_vtable_at_addr(lctx, trow, added_vt, ztv_ranges)
    return vtbl_ptr_map, ztv_ranges


def _topo_sort_unions(lctx: _LayoutCtx) -> list[int]:
    union_tids = sorted(
        (
            tid
            for tid in lctx.direct_members
            if (trow := lctx.inputs.type_rows.get(tid)) is not None
            and trow.kind == "union"
            and trow.is_referenced
            and not trow.is_stl_stub
            and not trow.is_declaration
            and (dt := lctx.inputs.type_map.get(tid)) is not None
            and hasattr(dt, "add")
        ),
        key=lambda tid: ((lctx.inputs.type_rows[tid].byte_size or 0), tid),
    )
    union_set = set(union_tids)
    ordered: list[int] = []
    visited: set[int] = set()
    visiting: set[int] = set()

    def visit(utid: int) -> None:
        if utid in visited or utid in visiting:
            return
        visiting.add(utid)
        for m in lctx.direct_members.get(utid, ()):
            mt = unwrap_type_row(m.member_type_id, lctx.inputs.type_rows)
            if mt is not None and mt.type_id in union_set:
                visit(mt.type_id)
        visiting.remove(utid)
        visited.add(utid)
        ordered.append(utid)

    for utid in union_tids:
        visit(utid)
    return ordered


def _place_union_arm(
    lctx: _LayoutCtx,
    dt: Any,
    trow: DwarfTypeRow,
    m: DwarfMemberRow,
    fname: str,
) -> None:
    mdt = lctx.inputs.type_map.get(m.member_type_id)
    if mdt is None or mdt == lctx.void_dt:
        return
    if (
        m.bit_size is not None
        and 0 < m.bit_size <= 64
        and lctx.bitfield_cls.isValidBaseDataType(mdt)
    ):
        base_dt = _valid_bitfield_base(mdt, max(m.byte_size, 1), lctx.bitfield_cls)
        eff_bits = min(m.bit_size, max(int(base_dt.getLength()), 1) * 8)
        try:
            dt.addBitField(base_dt, eff_bits, fname, None)
            lctx.stats.inc("union_bitfields_placed")
        except Exception as exc:
            lctx.stats.record_error("union_add_err", f"{trow.qualified_name}::{fname}", exc)
        return

    dt_len = int(mdt.getLength())
    comp_len = dt_len if dt_len > 0 else max(m.byte_size, 1)
    if trow.byte_size and trow.byte_size > 0 and comp_len > trow.byte_size:
        lctx.stats.inc("union_arm_oob")
        return
    try:
        dt.add(mdt, comp_len, fname, None)
        lctx.stats.inc("union_arms_placed")
    except Exception as exc:
        lctx.stats.record_error("union_add_err", f"{trow.qualified_name}::{fname}", exc)


def _populate_unions(lctx: _LayoutCtx) -> None:
    for tid in _topo_sort_unions(lctx):
        dt = lctx.inputs.type_map[tid]
        trow = lctx.inputs.type_rows[tid]
        used_names: set[str] = set()
        for m in lctx.direct_members.get(tid, ()):
            fname = m.name or f"arm_{m.ordinal}"
            if fname in used_names:
                fname = f"{fname}_{m.ordinal}"
            used_names.add(fname)
            _place_union_arm(lctx, dt, trow, m, fname)
        if trow.byte_size and trow.byte_size > 0 and int(dt.getLength()) == 0:
            try:
                raw_arr = lctx.array_cls(lctx.undef1_dt, trow.byte_size, 1)
                dt.add(raw_arr, trow.byte_size, "_raw", None)
                lctx.stats.inc("union_raw_fallback")
            except Exception as exc:
                lctx.stats.record_error("union_raw_err", trow.qualified_name, exc)
        lctx.stats.inc("unions_populated")


@dataclass
class _StructCandidate:
    off: int
    sz: int
    bit_off: int | None
    bit_sz: int | None
    name: str
    mdt: Any
    is_derived: bool


def _resolve_vptr_owner_dt(
    lctx: _LayoutCtx,
    root_tid: int,
    off: int,
    fallback_tid: int,
) -> Any | None:
    if off == 0:
        return lctx.vtbl_ptr_map.get(root_tid) or lctx.inputs.type_map.get(fallback_tid)
    queue: deque[tuple[int, int]] = deque([(root_tid, 0)])
    visited: set[tuple[int, int]] = {(root_tid, 0)}
    while queue:
        tid, boff = queue.popleft()
        if tid != root_tid and boff == off and tid in lctx.vtbl_ptr_map:
            return lctx.vtbl_ptr_map[tid]
        for inh in lctx.direct_inhs.get(tid, ()):
            if inh.byte_offset is None:
                continue
            nxt = (inh.base_type_id, boff + inh.byte_offset)
            if nxt[1] <= off and nxt not in visited:
                visited.add(nxt)
                queue.append(nxt)
    return lctx.inputs.type_map.get(fallback_tid)


def _member_to_candidate(
    lctx: _LayoutCtx,
    root_tid: int,
    cur_tid: int,
    base_off: int,
    m: DwarfMemberRow,
) -> _StructCandidate | None:
    off = base_off + m.byte_offset
    is_vptr = m.is_artificial and m.name.startswith("_vptr")
    if is_vptr:
        dt = _resolve_vptr_owner_dt(lctx, root_tid, off, m.member_type_id)
        mname = "_vptr" if off == 0 else m.name.replace("$", "_")
    else:
        dt = lctx.inputs.type_map.get(m.member_type_id)
        mname = m.name or f"field_0x{off:x}"
    if dt is None or dt == lctx.void_dt:
        return None
    is_derived = (
        (cur_tid == root_tid and off == 0) if is_vptr else (cur_tid == root_tid and base_off == 0)
    )
    return _StructCandidate(
        off=off,
        sz=max(m.byte_size, 1),
        bit_off=m.bit_offset,
        bit_sz=m.bit_size,
        name=mname,
        mdt=dt,
        is_derived=is_derived,
    )


def _collect_effective_members(
    lctx: _LayoutCtx,
    tid: int,
    vtbl_ptr_map: dict[int, Any],
) -> list[_StructCandidate]:
    def walk(cur_tid: int, base_off: int, seen: set[int]) -> list[_StructCandidate]:
        if cur_tid in seen:
            return []
        seen.add(cur_tid)
        out: list[_StructCandidate] = []
        for inh in lctx.direct_inhs.get(cur_tid, ()):
            if inh.byte_offset is not None:
                out.extend(walk(inh.base_type_id, base_off + inh.byte_offset, seen))
        for m in lctx.direct_members.get(cur_tid, ()):
            cand = _member_to_candidate(lctx, tid, cur_tid, base_off, m)
            if cand is not None:
                out.append(cand)
        seen.remove(cur_tid)
        return out

    candidates = walk(tid, 0, set())
    if tid in vtbl_ptr_map and not any(c.off == 0 and c.name == "_vptr" for c in candidates):
        candidates.append(_StructCandidate(0, 8, None, None, "_vptr", vtbl_ptr_map[tid], True))
    candidates.sort(
        key=lambda c: (
            c.off,
            c.bit_off if c.bit_off is not None else -1,
            0 if c.is_derived else 1,
            0 if c.name == "_vptr" else 1,
        )
    )
    return candidates


def _place_struct_bitfield(
    lctx: _LayoutCtx,
    dt: Any,
    c: _StructCandidate,
    occupied_bits: list[tuple[int, int]],
    used_names: set[str],
) -> None:
    struct_len = int(dt.getLength())
    bo = c.bit_off or 0
    base_dt = _valid_bitfield_base(c.mdt, c.sz, lctx.bitfield_cls)
    eff_bits = min(c.bit_sz or 1, max(int(base_dt.getLength()), 1) * 8)
    byte_width = max(1, (bo + eff_bits + 7) // 8)
    if c.off < 0 or c.off + byte_width > struct_len:
        lctx.stats.inc("member_oob")
        return
    b_start, b_end = c.off * 8 + bo, c.off * 8 + bo + eff_bits
    if any(b_start < oe and os_ < b_end for os_, oe in occupied_bits):
        lctx.stats.inc("member_overlap_skip")
        return
    fname = c.name if c.name not in used_names else f"{c.name}_{c.off:x}_{bo}"
    used_names.add(fname)
    try:
        dt.insertBitFieldAt(c.off, byte_width, bo, base_dt, eff_bits, fname, None)
        occupied_bits.append((b_start, b_end))
        lctx.stats.inc("bitfields_placed")
    except Exception as exc:
        lctx.stats.record_error("bitfield_insert_err", fname, exc)


def _place_struct_member(
    lctx: _LayoutCtx,
    dt: Any,
    c: _StructCandidate,
    occupied_bits: list[tuple[int, int]],
    used_names: set[str],
) -> None:
    if c.bit_sz is not None and 0 < c.bit_sz <= 64 and lctx.bitfield_cls.isValidBaseDataType(c.mdt):
        _place_struct_bitfield(lctx, dt, c, occupied_bits, used_names)
        return
    struct_len = int(dt.getLength())
    dt_len = int(c.mdt.getLength())
    comp_len = dt_len if dt_len > 0 else c.sz
    if c.off < 0 or comp_len <= 0 or c.off + comp_len > struct_len:
        lctx.stats.inc("member_oob")
        return
    b_start, b_end = c.off * 8, (c.off + comp_len) * 8
    if any(b_start < oe and os_ < b_end for os_, oe in occupied_bits):
        lctx.stats.inc("member_overlap_skip")
        return
    fname = c.name if c.name not in used_names else f"{c.name}_{c.off:x}"
    used_names.add(fname)
    try:
        dt.replaceAtOffset(c.off, c.mdt, comp_len, fname, None)
        occupied_bits.append((b_start, b_end))
        lctx.stats.inc("members_placed")
    except Exception as exc:
        lctx.stats.record_error("member_replace_err", fname, exc)


def _populate_structs(lctx: _LayoutCtx, vtbl_ptr_map: dict[int, Any]) -> None:
    target_tids = sorted(
        set(lctx.direct_members.keys()) | set(lctx.direct_inhs.keys()) | set(vtbl_ptr_map.keys())
    )
    for tid in target_tids:
        dt = lctx.inputs.type_map.get(tid)
        trow = lctx.inputs.type_rows.get(tid)
        if dt is None or trow is None or trow.kind == "union":
            continue
        if not (trow.is_referenced and not trow.is_stl_stub and not trow.is_declaration):
            continue
        if not hasattr(dt, "replaceAtOffset"):
            continue
        occupied_bits: list[tuple[int, int]] = []
        used_names: set[str] = set()
        for cand in _collect_effective_members(lctx, tid, vtbl_ptr_map):
            _place_struct_member(lctx, dt, cand, occupied_bits, used_names)


def _ensure_namespace(program: Any, ns_parts: tuple[str, ...]) -> Any:
    sym_table = program.getSymbolTable()
    source_imported = lifecycle.jclass("ghidra.program.model.symbol.SourceType").IMPORTED
    parent = program.getGlobalNamespace()
    for part in ns_parts:
        parent = sym_table.getOrCreateNameSpace(parent, part, source_imported)
    return parent


def _ranges_overlap(start: int, end: int, ranges: Sequence[tuple[int, int]]) -> bool:
    return any(start < r_end and r_start < end for r_start, r_end in ranges)


@dataclass
class _GlobalStampCtx:
    program: Any
    listing: Any
    mem: Any
    addr_space: Any
    sym_table: Any
    addr_set_cls: Any
    source_type_cls: Any
    ztv_ranges: Sequence[tuple[int, int]]
    stamped_ranges: list[tuple[int, int]]
    stats: lifecycle.StatsCollector


def _stamp_single_global(gctx: _GlobalStampCtx, g: DwarfGlobalRow, dt: Any) -> None:
    length = int(dt.getLength())
    if length <= 0:
        return
    addr = gctx.addr_space.getAddress(g.address)
    end_addr = addr.add(length - 1)
    block = gctx.mem.getBlock(addr)
    if block is None or block.isExecute() or gctx.mem.getBlock(end_addr) != block:
        gctx.stats.inc("globals_skipped_block")
        return
    g_end = g.address + length
    if _ranges_overlap(g.address, g_end, gctx.ztv_ranges) or _ranges_overlap(
        g.address, g_end, gctx.stamped_ranges
    ):
        gctx.stats.inc("globals_skipped_overlap")
        return
    if gctx.listing.getInstructions(gctx.addr_set_cls(addr, end_addr), True).hasNext():
        gctx.stats.inc("globals_skipped_code")
        return
    try:
        gctx.listing.clearCodeUnits(addr, end_addr, False)
        gctx.listing.createData(addr, dt)
        gctx.stamped_ranges.append((g.address, g_end))
        gctx.stats.inc("globals_stamped")
        primary = gctx.sym_table.getPrimarySymbol(addr)
        if primary is None or primary.getSource() == gctx.source_type_cls.DEFAULT:
            ns_parts, leaf = split_cpp_namespace(g.qualified_name)
            ns = _ensure_namespace(gctx.program, ns_parts)
            gctx.sym_table.createLabel(addr, leaf, ns, gctx.source_type_cls.IMPORTED)
            gctx.stats.inc("globals_labeled")
    except Exception as exc:
        gctx.stats.record_error("global_stamp_err", f"0x{g.address:x}:{g.qualified_name}", exc)


def stamp_dwarf_globals(
    program: Any,
    inputs: DwarfLayoutInputs,
    ztv_ranges: Sequence[tuple[int, int]],
    stats: lifecycle.StatsCollector,
) -> None:
    """Stamp non-executable `.data`/`.bss` DWARF globals and labels onto `program`."""
    void_dt = lifecycle.jclass("ghidra.program.model.data.VoidDataType").dataType
    gctx = _GlobalStampCtx(
        program=program,
        listing=program.getListing(),
        mem=program.getMemory(),
        addr_space=program.getAddressFactory().getDefaultAddressSpace(),
        sym_table=program.getSymbolTable(),
        addr_set_cls=lifecycle.jclass("ghidra.program.model.address.AddressSet"),
        source_type_cls=lifecycle.jclass("ghidra.program.model.symbol.SourceType"),
        ztv_ranges=ztv_ranges,
        stamped_ranges=[],
        stats=stats,
    )
    for g in inputs.globals_:
        dt = inputs.type_map.get(g.type_id)
        trow = inputs.type_rows.get(g.type_id)
        if dt is None or trow is None or trow.is_declaration or trow.is_stl_stub or dt == void_dt:
            continue
        _stamp_single_global(gctx, g, dt)


def pass2_vtables_and_structs(program: Any, inputs: DwarfLayoutInputs) -> DwarfPassStats:
    """Materialize vtables, unions, flattened structs, and globals in `program`."""
    void_dt = lifecycle.jclass("ghidra.program.model.data.VoidDataType").dataType
    dtm = program.getDataTypeManager()
    vt_by_class: dict[int, list[DwarfVtableEntryRow]] = defaultdict(list)
    for v in inputs.vtables:
        vt_by_class[v.class_type_id].append(v)
    direct_members: dict[int, list[DwarfMemberRow]] = defaultdict(list)
    for m in inputs.members:
        direct_members[m.parent_type_id].append(m)
    direct_inhs: dict[int, list[DwarfInheritanceRow]] = defaultdict(list)
    for inh in inputs.inheritances:
        direct_inhs[inh.derived_type_id].append(inh)

    lctx = _LayoutCtx(
        program=program,
        dtm=dtm,
        listing=program.getListing(),
        addr_space=program.getAddressFactory().getDefaultAddressSpace(),
        mem=program.getMemory(),
        inputs=inputs,
        vt_by_class=vt_by_class,
        direct_members=direct_members,
        direct_inhs=direct_inhs,
        stats=lifecycle.StatsCollector(DwarfPassName.PASS2_LAYOUTS),
        void_dt=void_dt,
        default_ptr_dt=dtm.getPointer(void_dt, 8),
        category_path_cls=lifecycle.jclass("ghidra.program.model.data.CategoryPath"),
        struct_cls=lifecycle.jclass("ghidra.program.model.data.StructureDataType"),
        func_def_cls=lifecycle.jclass("ghidra.program.model.data.FunctionDefinitionDataType"),
        param_def_cls=lifecycle.jclass("ghidra.program.model.data.ParameterDefinitionImpl"),
        bitfield_cls=lifecycle.jclass("ghidra.program.model.data.BitFieldDataType"),
        conflict_replace=lifecycle.jclass(
            "ghidra.program.model.data.DataTypeConflictHandler"
        ).REPLACE_HANDLER,
        undef1_dt=lifecycle.jclass("ghidra.program.model.data.Undefined1DataType").dataType,
        array_cls=lifecycle.jclass("ghidra.program.model.data.ArrayDataType"),
        vtbl_ptr_map={},
    )

    tx = int(program.startTransaction("Pass 2: vtables, unions, structs & globals"))
    try:
        vtbl_ptr_map, ztv_ranges = _build_vtable_structs(lctx)
        lctx.vtbl_ptr_map = vtbl_ptr_map
        _populate_unions(lctx)
        _populate_structs(lctx, vtbl_ptr_map)
        stamp_dwarf_globals(program, inputs, ztv_ranges, lctx.stats)
    finally:
        program.endTransaction(tx, True)
    program.clearUndo()
    return lctx.stats.freeze()
