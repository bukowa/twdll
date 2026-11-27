"""Pass 3: static noreturn stamping, un-thunking, and O(1) abi_class signatures."""

from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass, field
from typing import Any

from twdll._ghidra import lifecycle
from twdll._ghidra.dwarf_types import unwrap_type_row
from twdll.domain import (
    DwarfFunctionRow,
    DwarfParameterRow,
    DwarfPassName,
    DwarfPassStats,
    DwarfTypeRow,
)

INT_FALLBACK_REGS = ("RDI", "RSI", "RDX", "RCX", "R8", "R9")
FLOAT_FALLBACK_REGS = tuple(f"XMM{i}" for i in range(8))

_SUBREG_MAP: dict[tuple[str, int], str] = {
    **{(r, 1): f"{r[0]}L" for r in ("RAX", "RBX", "RCX", "RDX")},
    **{(r, 2): f"{r[0]}X" for r in ("RAX", "RBX", "RCX", "RDX")},
    **{(r, 4): f"E{r[0]}X" for r in ("RAX", "RBX", "RCX", "RDX")},
    **{(r, 1): f"{r[1:]}L" for r in ("RSI", "RDI", "RBP", "RSP")},
    **{(r, 2): r[1:] for r in ("RSI", "RDI", "RBP", "RSP")},
    **{(r, 4): f"E{r[1:]}" for r in ("RSI", "RDI", "RBP", "RSP")},
    **{(f"R{i}", 1): f"R{i}B" for i in range(8, 16)},
    **{(f"R{i}", 2): f"R{i}W" for i in range(8, 16)},
    **{(f"R{i}", 4): f"R{i}D" for i in range(8, 16)},
    **{(f"XMM{i}", 4): f"XMM{i}_Da" for i in range(16)},
    **{(f"XMM{i}", 8): f"XMM{i}_Qa" for i in range(16)},
}

_REG_ROOTS: dict[str, str] = {
    **{r: r for r in ("RAX", "RBX", "RCX", "RDX", "RSI", "RDI", "RBP", "RSP")},
    **{sub: root for (root, _), sub in _SUBREG_MAP.items() if not root.startswith("XMM")},
    **{f"{r}H": f"R{r}X" for r in ("A", "B", "C", "D")},
    **{f"R{i}": f"R{i}" for i in range(8, 16)},
    **{f"XMM{i}": f"XMM{i}" for i in range(16)},
    **{f"YMM{i}": f"XMM{i}" for i in range(16)},
}

STATIC_NORETURN_NAMES: frozenset[str] = frozenset(
    {
        "__clang_call_terminate",
        "_Unwind_Resume",
        "__cxa_throw",
        "__cxa_rethrow",
        "__cxa_pure_virtual",
        "_ZSt9terminatev",
        "abort",
        "__assert_fail",
        "std::__throw_bad_alloc",
        "std::__throw_bad_cast",
        "std::__throw_bad_typeid",
        "std::__throw_bad_weak_ptr",
        "std::__throw_length_error",
        "std::__throw_logic_error",
        "std::__throw_out_of_range",
        "std::__throw_runtime_error",
        "std::__throw_bad_function_call",
        "std::__throw_system_error",
        "std::__throw_invalid_argument",
        "std::__throw_domain_error",
        "std::__throw_overflow_error",
        "std::__throw_underflow_error",
        "std::__throw_range_error",
        "std::__throw_future_error",
        "std::__throw_regex_error",
        "std::__throw_ios_failure",
        "std::__throw_bad_exception",
        "luaG_runerror",
        "luaL_error",
        "luaD_throw",
        "lua_error",
        "png_error",
        "png_chunk_error",
    }
)


@dataclass(frozen=True)
class DwarfSignatureInputs:
    type_map: dict[int, Any]
    type_rows: dict[int, DwarfTypeRow]
    functions: tuple[DwarfFunctionRow, ...]
    parameters: tuple[DwarfParameterRow, ...]


def resolve_dwarf_reg(program: Any, reg_name: str, byte_size: int | None) -> Any:
    """Resolve a Ghidra register or sub-register matching `byte_size`."""
    root = _REG_ROOTS.get(reg_name, reg_name)
    if byte_size is not None:
        sub_name = _SUBREG_MAP.get((root, byte_size))
        if sub_name is not None:
            sub = program.getRegister(sub_name)
            if sub is not None:
                return sub
    return program.getRegister(reg_name) or program.getRegister(root)


def _is_noreturn_name(name: str) -> bool:
    if not name:
        return False
    if name in STATIC_NORETURN_NAMES:
        return True
    if name.startswith("std::__throw_"):
        return True
    return name.startswith("_ZSt") and "__throw_" in name


def _strip_library_prefix(gf: Any) -> str:
    symbol_type_lib = lifecycle.jclass("ghidra.program.model.symbol.SymbolType").LIBRARY
    parts: list[str] = []
    ns = gf.getParentNamespace()
    while ns is not None:
        is_global = hasattr(ns, "isGlobal") and ns.isGlobal()
        if is_global:
            break
        ns_sym = ns.getSymbol() if hasattr(ns, "getSymbol") else None
        is_lib = ns_sym is not None and ns_sym.getSymbolType() == symbol_type_lib
        ns_name = str(ns.getName())
        if not is_lib and ns_name != "<EXTERNAL>" and ".so" not in ns_name:
            parts.append(ns_name)
        ns = ns.getParentNamespace()
    parts.reverse()
    short = str(gf.getName(False))
    return "::".join([*parts, short]) if parts else short


def _matches_noreturn_func(gf: Any) -> bool:
    candidates = [
        str(gf.getName(True)),
        str(gf.getName(False)),
        _strip_library_prefix(gf),
    ]
    if hasattr(gf, "getExternalLocation"):
        ext_loc = gf.getExternalLocation()
        if ext_loc is not None and hasattr(ext_loc, "getOriginalImportedName"):
            orig = ext_loc.getOriginalImportedName()
            if orig:
                candidates.append(str(orig))
    return any(_is_noreturn_name(cand) for cand in candidates)


def _apply_external_noreturn(fm: Any, stats: lifecycle.StatsCollector) -> None:
    for ef in fm.getExternalFunctions():
        if _matches_noreturn_func(ef) and not ef.hasNoReturn():
            ef.setNoReturn(True)
            stats.inc("static_noreturn_ext_set")


def _apply_thunk_noreturn(gf: Any, stats: lifecycle.StatsCollector) -> None:
    target = gf.getThunkedFunction(True)
    if target is None or not (_matches_noreturn_func(target) or target.hasNoReturn()):
        return
    if not target.hasNoReturn():
        target.setNoReturn(True)
        stats.inc("static_noreturn_ext_set")
    if not gf.hasNoReturn():
        gf.setNoReturn(True)
        stats.inc("static_noreturn_thunk_set")


def _apply_static_noreturn(program: Any, stats: lifecycle.StatsCollector) -> None:
    fm = program.getFunctionManager()
    listing = program.getListing()
    _apply_external_noreturn(fm, stats)
    for gf in fm.getFunctions(True):
        if gf.isThunk():
            _apply_thunk_noreturn(gf, stats)
        elif _matches_noreturn_func(gf):
            if not gf.hasNoReturn():
                gf.setNoReturn(True)
                stats.inc("static_noreturn_set")
        elif str(gf.getName(True)) == "panic" and gf.hasNoReturn() and not gf.isExternal():
            last_ins = listing.getInstructionContaining(gf.getBody().getMaxAddress())
            if last_ins is not None and last_ins.getFlowType().isTerminal():
                gf.setNoReturn(False)
                stats.inc("panic_noreturn_cleared")


def _build_one_reg_storage(program: Any, reg_name: str, total_len: int) -> Any:
    varnode_cls = lifecycle.jclass("ghidra.program.model.pcode.Varnode")
    var_storage_cls = lifecycle.jclass("ghidra.program.model.listing.VariableStorage")
    reg = program.getRegister(reg_name)
    vn = varnode_cls(reg.getAddress(), total_len)
    return var_storage_cls(program, lifecycle.jarray("ghidra.program.model.pcode.Varnode", [vn]))


def _build_two_reg_storage(
    program: Any,
    lo_reg_name: str,
    hi_reg_name: str,
    total_len: int,
) -> Any:
    varnode_cls = lifecycle.jclass("ghidra.program.model.pcode.Varnode")
    var_storage_cls = lifecycle.jclass("ghidra.program.model.listing.VariableStorage")
    lo_reg = program.getRegister(lo_reg_name)
    hi_reg = program.getRegister(hi_reg_name)
    hi_vn = varnode_cls(hi_reg.getAddress(), total_len - 8)
    lo_vn = varnode_cls(lo_reg.getAddress(), 8)
    arr = lifecycle.jarray("ghidra.program.model.pcode.Varnode", [hi_vn, lo_vn])
    return var_storage_cls(program, arr)


@dataclass
class _ParamAllocState:
    program: Any
    stats: lifecycle.StatsCollector
    is_sret: bool
    used_reg_roots: set[str] = field(default_factory=set)
    used_stack_ranges: list[tuple[int, int]] = field(default_factory=list)
    used_param_names: set[str] = field(default_factory=set)
    int_fb_idx: int = 0
    flt_fb_idx: int = 0
    next_fb_stack: int = 8

    def next_free_regs(self, pool: tuple[str, ...], start_idx: int, count: int) -> list[str] | None:
        found: list[str] = []
        for r in pool[start_idx:]:
            if r not in self.used_reg_roots:
                found.append(r)
                if len(found) == count:
                    return found
        return None

    def commit_regs(self, regs: list[str]) -> None:
        for r in regs:
            self.used_reg_roots.add(r)
        while self.int_fb_idx < len(INT_FALLBACK_REGS) and (
            INT_FALLBACK_REGS[self.int_fb_idx] in self.used_reg_roots
        ):
            self.int_fb_idx += 1
        while self.flt_fb_idx < len(FLOAT_FALLBACK_REGS) and (
            FLOAT_FALLBACK_REGS[self.flt_fb_idx] in self.used_reg_roots
        ):
            self.flt_fb_idx += 1

    def alloc_stack(self, byte_len: int) -> Any:
        var_storage_cls = lifecycle.jclass("ghidra.program.model.listing.VariableStorage")
        while any(
            self.next_fb_stack < ue and us < self.next_fb_stack + byte_len
            for us, ue in self.used_stack_ranges
        ):
            self.next_fb_stack += 8
        self.used_stack_ranges.append((self.next_fb_stack, self.next_fb_stack + byte_len))
        storage = var_storage_cls(self.program, self.next_fb_stack, byte_len)
        self.next_fb_stack += ((byte_len + 7) // 8) * 8
        return storage


def _build_return_param(
    program: Any,
    chosen: DwarfFunctionRow,
    type_map: dict[int, Any],
    type_rows: dict[int, DwarfTypeRow],
    state: _ParamAllocState,
) -> tuple[Any, Any | None]:
    void_dt = lifecycle.jclass("ghidra.program.model.data.VoidDataType").dataType
    param_impl_cls = lifecycle.jclass("ghidra.program.model.listing.ParameterImpl")
    ret_param_cls = lifecycle.jclass("ghidra.program.model.listing.ReturnParameterImpl")
    var_storage_cls = lifecycle.jclass("ghidra.program.model.listing.VariableStorage")
    dtm = program.getDataTypeManager()

    ret_dt = (
        type_map.get(chosen.return_type_id, void_dt)
        if chosen.return_type_id is not None
        else void_dt
    )
    if chosen.is_sret:
        ret_ptr_dt = dtm.getPointer(ret_dt, 8)
        state.commit_regs(["RDI"])
        state.used_param_names.add("__return_storage_ptr__")
        sret_storage = var_storage_cls(program, program.getRegister("RDI"))
        hidden_param = param_impl_cls("__return_storage_ptr__", ret_ptr_dt, sret_storage, program)
        ret_storage = var_storage_cls(program, program.getRegister("RAX"))
        return ret_param_cls(ret_ptr_dt, ret_storage, program), hidden_param

    if ret_dt == void_dt:
        return ret_param_cls(void_dt, var_storage_cls.VOID_STORAGE, program), None

    ret_row = unwrap_type_row(chosen.return_type_id, type_rows)
    abi_class = ret_row.abi_class if ret_row is not None else None
    ret_len = max(int(ret_dt.getLength()), 1)
    storage = _return_storage_for_abi(program, abi_class, ret_len)
    return ret_param_cls(ret_dt, storage, program), None


def _return_storage_for_abi(program: Any, abi_class: str | None, ret_len: int) -> Any:
    match abi_class:
        case "sse":
            return _build_one_reg_storage(program, "XMM0", min(ret_len, 8))
        case "sse,sse" if ret_len > 8:
            return _build_two_reg_storage(program, "XMM0", "XMM1", min(ret_len, 16))
        case "int,int" if ret_len > 8:
            return _build_two_reg_storage(program, "RAX", "RDX", min(ret_len, 16))
        case "sse,int" if ret_len > 8:
            return _build_two_reg_storage(program, "XMM0", "RAX", min(ret_len, 16))
        case "int,sse" if ret_len > 8:
            return _build_two_reg_storage(program, "RAX", "XMM0", min(ret_len, 16))
        case _:
            return _build_one_reg_storage(program, "RAX", min(ret_len, 8))


def _try_explicit_storage(
    state: _ParamAllocState,
    pr: DwarfParameterRow,
    abi_class: str | None,
    pdt_len: int,
) -> Any | None:
    var_storage_cls = lifecycle.jclass("ghidra.program.model.listing.VariableStorage")
    if pr.storage_kind == "register" and pr.reg_name and abi_class in ("int", "sse"):
        rname = (
            "RSI" if (state.is_sret and pr.ordinal == 0 and pr.reg_name == "RDI") else pr.reg_name
        )
        root = _REG_ROOTS.get(rname, rname)
        valid_pool = FLOAT_FALLBACK_REGS if abi_class == "sse" else INT_FALLBACK_REGS
        if root in valid_pool and root not in state.used_reg_roots and pdt_len <= 8:
            state.commit_regs([root])
            return _build_one_reg_storage(state.program, root, pdt_len)
    if pr.storage_kind == "stack" and pr.stack_offset is not None and pr.stack_offset >= 8:
        soff = int(pr.stack_offset)
        send = soff + pdt_len
        if not any(soff < ue and us < send for us, ue in state.used_stack_ranges):
            state.used_stack_ranges.append((soff, send))
            return var_storage_cls(state.program, soff, pdt_len)
    return None


def _try_two_reg_param_storage(
    state: _ParamAllocState,
    abi_class: str,
    pdt_len: int,
) -> Any | None:
    if not (9 <= pdt_len <= 16):
        return None
    if abi_class == "int,int":
        regs = state.next_free_regs(INT_FALLBACK_REGS, state.int_fb_idx, 2)
        if regs is None:
            return None
        state.commit_regs(regs)
        state.stats.inc("param_2reg_struct")
        return _build_two_reg_storage(state.program, regs[0], regs[1], pdt_len)
    if abi_class == "sse,sse":
        regs = state.next_free_regs(FLOAT_FALLBACK_REGS, state.flt_fb_idx, 2)
        if regs is None:
            return None
        state.commit_regs(regs)
        state.stats.inc("param_2reg_struct")
        return _build_two_reg_storage(state.program, regs[0], regs[1], pdt_len)
    int_reg = state.next_free_regs(INT_FALLBACK_REGS, state.int_fb_idx, 1)
    flt_reg = state.next_free_regs(FLOAT_FALLBACK_REGS, state.flt_fb_idx, 1)
    if int_reg is None or flt_reg is None:
        state.stats.inc("param_2reg_atomic_spill")
        return None
    state.commit_regs([int_reg[0], flt_reg[0]])
    state.stats.inc("param_2reg_struct")
    if abi_class == "sse,int":
        return _build_two_reg_storage(state.program, flt_reg[0], int_reg[0], pdt_len)
    return _build_two_reg_storage(state.program, int_reg[0], flt_reg[0], pdt_len)


def _alloc_param_storage(
    state: _ParamAllocState,
    pr: DwarfParameterRow,
    abi_class: str | None,
    pdt_len: int,
) -> Any:
    if abi_class == "memory":
        return state.alloc_stack(pdt_len)
    explicit = _try_explicit_storage(state, pr, abi_class, pdt_len)
    if explicit is not None:
        return explicit
    if abi_class in ("int,int", "sse,sse", "sse,int", "int,sse"):
        return _try_two_reg_param_storage(state, abi_class, pdt_len) or state.alloc_stack(pdt_len)
    if pdt_len <= 8:
        pool = FLOAT_FALLBACK_REGS if abi_class == "sse" else INT_FALLBACK_REGS
        start = state.flt_fb_idx if abi_class == "sse" else state.int_fb_idx
        regs = state.next_free_regs(pool, start, 1)
        if regs is not None:
            state.commit_regs(regs)
            return _build_one_reg_storage(state.program, regs[0], pdt_len)
    return state.alloc_stack(pdt_len)


def _is_adjusting_thunk(chosen: DwarfFunctionRow) -> bool:
    return bool(chosen.linkage_name and chosen.linkage_name.startswith(("_ZThn", "_ZTvn")))


def _resolve_param_type_and_abi(
    state: _ParamAllocState,
    chosen: DwarfFunctionRow,
    pr: DwarfParameterRow,
    inputs: DwarfSignatureInputs,
) -> tuple[Any, str | None]:
    void_dt = lifecycle.jclass("ghidra.program.model.data.VoidDataType").dataType
    dtm = state.program.getDataTypeManager()
    default_ptr = dtm.getPointer(void_dt, 8)
    if _is_adjusting_thunk(chosen) and (pr.is_object_pointer or pr.ordinal == 0):
        return default_ptr, "int"
    pdt = inputs.type_map.get(pr.type_id, default_ptr)
    if pdt == void_dt:
        pdt = default_ptr
    ut = unwrap_type_row(pr.type_id, inputs.type_rows)
    abi_class = ut.abi_class if ut is not None else "int"
    if ut is not None and ut.kind in ("struct", "class", "union") and ut.is_non_trivial_for_calls:
        return dtm.getPointer(pdt, 8), "int"
    return pdt, abi_class


def _is_phantom_param(pr: DwarfParameterRow, inputs: DwarfSignatureInputs) -> bool:
    if pr.name == "__in_chrg" and pr.is_artificial and not pr.is_object_pointer:
        return True
    if pr.storage_kind != "none":
        return False
    ut = unwrap_type_row(pr.type_id, inputs.type_rows)
    return ut is not None and ut.kind == "primitive" and ut.qualified_name == "void"


def _apply_single_function_sig(
    state: _ParamAllocState,
    gh_func: Any,
    chosen: DwarfFunctionRow,
    params: list[DwarfParameterRow],
    inputs: DwarfSignatureInputs,
) -> None:
    param_impl_cls = lifecycle.jclass("ghidra.program.model.listing.ParameterImpl")
    func_cls = lifecycle.jclass("ghidra.program.model.listing.Function")
    source_imported = lifecycle.jclass("ghidra.program.model.symbol.SourceType").IMPORTED
    array_list_cls = lifecycle.jclass("java.util.ArrayList")

    ret_param, hidden_sret = _build_return_param(
        state.program, chosen, inputs.type_map, inputs.type_rows, state
    )
    j_params = array_list_cls()
    if hidden_sret is not None:
        j_params.add(hidden_sret)

    for pr in params:
        if _is_phantom_param(pr, inputs):
            continue
        pdt, abi_class = _resolve_param_type_and_abi(state, chosen, pr, inputs)
        pdt_len = max(int(pdt.getLength()), 1)
        pname = pr.name or ("this" if pr.is_object_pointer else f"param_{pr.ordinal}")
        if pname in state.used_param_names:
            pname = f"{pname}_{pr.ordinal}"
        state.used_param_names.add(pname)
        storage = _alloc_param_storage(state, pr, abi_class, pdt_len)
        j_params.add(param_impl_cls(pname, pdt, storage, state.program))

    gh_func.updateFunction(
        func_cls.DEFAULT_CALLING_CONVENTION_STRING,
        ret_param,
        j_params,
        func_cls.FunctionUpdateType.CUSTOM_STORAGE,
        True,
        source_imported,
    )
    state.stats.inc("applied")


def _process_addr_candidates(
    program: Any,
    addr_int: int,
    cands: list[DwarfFunctionRow],
    stats: lifecycle.StatsCollector,
) -> tuple[Any, DwarfFunctionRow] | None:
    addr = program.getAddressFactory().getDefaultAddressSpace().getAddress(addr_int)
    gh_func = program.getFunctionManager().getFunctionAt(addr)
    if gh_func is None:
        stats.inc("no_ghidra_func")
        return None
    if gh_func.isThunk():
        try:
            gh_func.setThunkedFunction(None)
            stats.inc("unthunked")
        except Exception as exc:
            stats.record_error("unthunk_err", f"0x{addr_int:x}", exc)
    chosen = cands[0]
    if len(cands) > 1:
        syms = {str(s.getName()) for s in program.getSymbolTable().getSymbols(addr)}
        chosen = next((c for c in cands if c.linkage_name and c.linkage_name in syms), cands[0])
    return gh_func, chosen


def pass3_signatures(program: Any, inputs: DwarfSignatureInputs) -> DwarfPassStats:
    """Apply static noreturn flags, un-thunking, and `CUSTOM_STORAGE` signatures."""
    stats = lifecycle.StatsCollector(DwarfPassName.PASS3_SIGNATURES)
    funcs_by_addr: dict[int, list[DwarfFunctionRow]] = defaultdict(list)
    for f in inputs.functions:
        if f.address is not None:
            funcs_by_addr[f.address].append(f)
    params_by_fid: dict[int, list[DwarfParameterRow]] = defaultdict(list)
    for p in inputs.parameters:
        params_by_fid[p.function_id].append(p)

    tx = int(
        program.startTransaction("Pass 3: static noreturn, un-thunk & CUSTOM_STORAGE signatures")
    )
    try:
        _apply_static_noreturn(program, stats)
        for addr_int, cands in funcs_by_addr.items():
            resolved = _process_addr_candidates(program, addr_int, cands, stats)
            if resolved is None:
                continue
            gh_func, chosen = resolved
            state = _ParamAllocState(program=program, stats=stats, is_sret=chosen.is_sret)
            try:
                _apply_single_function_sig(
                    state, gh_func, chosen, params_by_fid.get(chosen.function_id, []), inputs
                )
            except Exception as exc:
                stats.record_error("sig_apply_err", f"0x{addr_int:x}:{chosen.qualified_name}", exc)
    finally:
        program.endTransaction(tx, True)
    program.clearUndo()
    return stats.freeze()
