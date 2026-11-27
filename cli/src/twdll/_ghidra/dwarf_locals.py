"""Pass 4: stack/register local variables and inlined subroutine pre-comments."""

from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass
from typing import Any

from twdll._ghidra import lifecycle
from twdll.domain import (
    DwarfFunctionRow,
    DwarfInlineRow,
    DwarfPassName,
    DwarfPassStats,
    DwarfVariableRow,
)


@dataclass(frozen=True)
class DwarfLocalInputs:
    type_map: dict[int, Any]
    functions: tuple[DwarfFunctionRow, ...]
    variables: tuple[DwarfVariableRow, ...]
    inlines: tuple[DwarfInlineRow, ...]


@dataclass
class _LocalCtx:
    program: Any
    void_dt: Any
    default_ptr_dt: Any
    local_var_cls: Any
    var_storage_cls: Any
    source_imported: Any
    type_map: dict[int, Any]
    stats: lifecycle.StatsCollector


def _local_var_name(vr: DwarfVariableRow) -> str:
    return vr.name if vr.range_index == 0 else f"{vr.name}${vr.range_index}"


def _try_add_stack_local(
    lctx: _LocalCtx,
    gh_func: Any,
    vr: DwarfVariableRow,
    vdt: Any,
    used_stack: list[tuple[int, int]],
) -> bool:
    sz = max(int(vdt.getLength()), 1)
    if vr.stack_offset is None:
        return False
    off = int(vr.stack_offset)
    if not (off < 0 and off + sz <= 0):
        lctx.stats.inc("local_stack_oob")
        return False
    end = off + sz
    if any(off < oe and os_ < end for os_, oe in used_stack):
        lctx.stats.inc("local_stack_overlap")
        return False
    storage = lctx.var_storage_cls(lctx.program, off, sz)
    lv = lctx.local_var_cls(_local_var_name(vr), 0, vdt, storage, lctx.program)
    gh_func.addLocalVariable(lv, lctx.source_imported)
    used_stack.append((off, end))
    lctx.stats.inc("locals_stack_added")
    return True


def _try_add_register_local(
    lctx: _LocalCtx,
    gh_func: Any,
    vr: DwarfVariableRow,
    vdt: Any,
) -> bool:
    if not vr.reg_name:
        return False
    if vr.first_use_offset <= 0:
        lctx.stats.inc("local_reg_entry_skip")
        return False
    sz = max(int(vdt.getLength()), 1)
    reg = lctx.program.getRegister(vr.reg_name)
    if reg is None or int(reg.getMinimumByteSize()) < sz:
        return False
    storage = lctx.var_storage_cls(lctx.program, reg)
    lv = lctx.local_var_cls(
        _local_var_name(vr), int(vr.first_use_offset), vdt, storage, lctx.program
    )
    gh_func.addLocalVariable(lv, lctx.source_imported)
    lctx.stats.inc("locals_reg_added")
    return True


def _populate_function_locals(
    lctx: _LocalCtx,
    gh_func: Any,
    vlist: list[DwarfVariableRow],
) -> None:
    used_stack: list[tuple[int, int]] = []
    used_names: set[str] = {str(p.getName()) for p in gh_func.getParameters()}
    for vr in vlist:
        vdt = lctx.type_map.get(vr.type_id, lctx.default_ptr_dt)
        if vdt == lctx.void_dt:
            continue
        vname = _local_var_name(vr)
        if vname in used_names:
            continue
        try:
            added = (
                _try_add_stack_local(lctx, gh_func, vr, vdt, used_stack)
                if vr.storage_kind == "stack"
                else _try_add_register_local(lctx, gh_func, vr, vdt)
            )
            if added:
                used_names.add(vname)
        except Exception as exc:
            lctx.stats.record_error("local_err", f"{gh_func.getName()}:{vname}", exc)


def _apply_inline_comments(
    program: Any,
    inlines: tuple[DwarfInlineRow, ...],
    stats: lifecycle.StatsCollector,
) -> None:
    listing = program.getListing()
    addr_space = program.getAddressFactory().getDefaultAddressSpace()
    comment_pre = lifecycle.jclass("ghidra.program.model.listing.CommentType").PRE
    inlines_by_start: dict[int, list[str]] = defaultdict(list)
    for inl in inlines:
        loc = f" @ {inl.call_file}:{inl.call_line}" if inl.call_file and inl.call_line else ""
        line = f"inline {inl.callee_name}{loc}"
        bucket = inlines_by_start[inl.start_address]
        if line not in bucket:
            bucket.append(line)

    for saddr, lines in inlines_by_start.items():
        try:
            cu = listing.getCodeUnitAt(addr_space.getAddress(saddr))
            if cu is not None:
                cu.setComment(comment_pre, "\n".join(lines))
                stats.inc("inline_comments_set")
        except Exception as exc:
            stats.record_error("inline_comment_err", f"0x{saddr:x}", exc)


def pass4_locals_and_inlines(program: Any, inputs: DwarfLocalInputs) -> DwarfPassStats:
    """Apply DWARF local variable declarations and inline pre-comments to `program`."""
    void_dt = lifecycle.jclass("ghidra.program.model.data.VoidDataType").dataType
    dtm = program.getDataTypeManager()
    stats = lifecycle.StatsCollector(DwarfPassName.PASS4_LOCALS)
    lctx = _LocalCtx(
        program=program,
        void_dt=void_dt,
        default_ptr_dt=dtm.getPointer(void_dt, 8),
        local_var_cls=lifecycle.jclass("ghidra.program.model.listing.LocalVariableImpl"),
        var_storage_cls=lifecycle.jclass("ghidra.program.model.listing.VariableStorage"),
        source_imported=lifecycle.jclass("ghidra.program.model.symbol.SourceType").IMPORTED,
        type_map=inputs.type_map,
        stats=stats,
    )

    fm = program.getFunctionManager()
    addr_space = program.getAddressFactory().getDefaultAddressSpace()
    func_addr_by_id = {f.function_id: f.address for f in inputs.functions if f.address is not None}
    vars_by_fid: dict[int, list[DwarfVariableRow]] = defaultdict(list)
    for v in inputs.variables:
        vars_by_fid[v.function_id].append(v)

    tx = int(program.startTransaction("Pass 4: locals & inline pre-comments"))
    try:
        for fid, vlist in vars_by_fid.items():
            faddr = func_addr_by_id.get(fid)
            if faddr is None:
                continue
            gh_func = fm.getFunctionAt(addr_space.getAddress(faddr))
            if gh_func is not None:
                _populate_function_locals(lctx, gh_func, vlist)
        _apply_inline_comments(program, inputs.inlines, stats)
    finally:
        program.endTransaction(tx, True)
    program.clearUndo()
    return stats.freeze()
