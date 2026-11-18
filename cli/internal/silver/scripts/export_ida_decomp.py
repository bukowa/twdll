import ida_loader
ida_loader.set_database_flag(ida_loader.DBFL_TEMP)

import json
import os
import re
import time
import ida_auto
import ida_funcs
import ida_hexrays
import ida_ida
import ida_lines
import ida_name
import ida_nalt
import ida_typeinf
import ida_xref
import idaapi
import idc

t0 = time.time()
ida_auto.auto_wait()

if not ida_hexrays.init_hexrays_plugin():
    raise RuntimeError("Hex-Rays decompiler not available")

IS_64BIT = bool(hasattr(ida_ida, "inf_is_64bit") and ida_ida.inf_is_64bit())
if IS_64BIT:
    U64_TIF = ida_typeinf.tinfo_t(ida_typeinf.BTF_UINT64)
    _void_const = ida_typeinf.tinfo_t(ida_typeinf.BTF_VOID)
    _void_const.set_const()
    CONST_VOID_PTR_TIF = ida_typeinf.tinfo_t()
    CONST_VOID_PTR_TIF.create_ptr(_void_const)
    PRETYPED_DREF_EAS = set()
    PRETYPED_UDT_EAS = set()


def _pretype_unnamed_udt_args(func_ea):
    if func_ea in PRETYPED_UDT_EAS:
        return
    PRETYPED_UDT_EAS.add(func_ea)
    try:
        ftif = ida_typeinf.tinfo_t()
        if ida_nalt.get_tinfo(ftif, func_ea) and ftif.is_func():
            ftd = ida_typeinf.func_type_data_t()
            if ftif.get_func_details(ftd):
                changed = False
                for a in ftd:
                    if a.type.is_udt() and a.type.get_type_name() is None and a.type.get_size() == 8:
                        a.type = U64_TIF
                        changed = True
                if changed:
                    new_ftif = ida_typeinf.tinfo_t()
                    if new_ftif.create_func(ftd):
                        ida_typeinf.apply_tinfo(func_ea, new_ftif, ida_typeinf.TINFO_DEFINITE)
    except Exception:
        pass


def pretype_64bit_stalls(fn):
    try:
        for item_ea in ida_funcs.func_item_iterator_t(fn):
            xref = ida_xref.get_first_dref_from(item_ea)
            while xref != idaapi.BADADDR:
                if xref not in PRETYPED_DREF_EAS:
                    PRETYPED_DREF_EAS.add(xref)
                    if ida_funcs.get_func(xref) is None and not ida_nalt.get_tinfo(ida_typeinf.tinfo_t(), xref):
                        gname = ida_name.get_name(xref) or ""
                        if gname.startswith(("_ZGV", "_ZZ")):
                            ida_typeinf.apply_tinfo(xref, U64_TIF, ida_typeinf.TINFO_DEFINITE)
                        elif gname.startswith(("off_", "_ZTV", "_ZTI", "_ZTS")):
                            ida_typeinf.apply_tinfo(xref, CONST_VOID_PTR_TIF, ida_typeinf.TINFO_DEFINITE)
                xref = ida_xref.get_next_dref_from(item_ea, xref)

            fx = ida_xref.get_first_fcref_from(item_ea)
            while fx != idaapi.BADADDR:
                if fx not in PRETYPED_UDT_EAS:
                    callee = ida_funcs.get_func(fx)
                    if callee is not None and int(callee.start_ea) == int(fx):
                        _pretype_unnamed_udt_args(int(fx))
                    else:
                        PRETYPED_UDT_EAS.add(fx)
                fx = ida_xref.get_next_fcref_from(item_ea, fx)
    except Exception:
        pass
    _pretype_unnamed_udt_args(int(fn.start_ea))


in_path = os.environ["TWDLL_DECOMP_IN"]
out_path = os.environ["TWDLL_DECOMP_OUT"]

targets = []
with open(in_path, "r", encoding="utf-8") as f:
    for ln in f:
        ln = ln.strip()
        if not ln:
            continue
        parts = ln.split("\t", 2)
        ea_hex = parts[0].strip()
        ea_val = int(ea_hex, 16)
        sz = int(parts[1].strip()) if len(parts) > 1 else 0
        nm = parts[2] if len(parts) > 2 else ""
        targets.append((hex(ea_val), ea_val, sz, nm))

re_addr = re.compile(r"\x01\x28([0-9A-Fa-f]{8,16})")


def extract_vars(cfunc):
    out = []
    try:
        stk_delta = 0
        try:
            stk_delta = int(cfunc.get_stkoff_delta())
        except Exception:
            stk_delta = 0
        for lv in cfunc.get_lvars():
            stor = ""
            try:
                if lv.is_reg_var():
                    mreg = lv.get_reg1()
                    width = max(1, int(lv.width))
                    reg_name = ida_hexrays.get_mreg_name(mreg, width) or ""
                    if reg_name:
                        stor = "reg:" + reg_name.lower()
                elif lv.is_stk_var():
                    off = int(lv.get_stkoff()) - stk_delta
                    stor = f"stack:{off:+d}"
            except Exception:
                stor = ""
            out.append({
                "name": lv.name or "",
                "type": str(lv.type()),
                "is_arg": bool(lv.is_arg_var),
                "size": int(lv.width),
                "storage": stor,
            })
    except Exception:
        pass
    return out


ok_count = 0
fail_count = 0
with open(out_path, "w", encoding="utf-8") as out_f:
    for ea_hex, ea, sz, name in targets:
        out_f.write(json.dumps({"event": "begin", "ea": ea_hex, "size": sz, "name": name}) + "\n")
        out_f.flush()

        tf0 = time.time()
        fn = ida_funcs.get_func(ea)
        if fn is None or int(fn.start_ea) != ea:
            fail_count += 1
            out_f.write(json.dumps({
                "event": "result",
                "ea": ea_hex,
                "size": sz,
                "name": name,
                "status": "failed",
                "elapsed_ms": int((time.time() - tf0) * 1000),
                "error": "get_func did not find function at entry EA",
            }) + "\n")
            out_f.flush()
            continue

        if sz <= 0:
            sz = int(ida_funcs.calc_func_size(fn))

        hf = ida_hexrays.hexrays_failure_t()
        cfunc = None
        last_ex = None
        try:
            if IS_64BIT:
                pretype_64bit_stalls(fn)
            cfunc = ida_hexrays.decompile(fn.start_ea, hf, ida_hexrays.DECOMP_NO_WAIT)
        except Exception as ex:
            last_ex = ex

        if cfunc is None and last_ex is not None:
            fail_count += 1
            out_f.write(json.dumps({
                "event": "result",
                "ea": ea_hex,
                "size": sz,
                "name": name,
                "status": "failed",
                "elapsed_ms": int((time.time() - tf0) * 1000),
                "error": f"decompile error: {last_ex}",
            }) + "\n")
            out_f.flush()
            continue

        if cfunc is None:
            fail_count += 1
            desc = ""
            try:
                desc = hf.desc()
            except Exception:
                desc = ""
            out_f.write(json.dumps({
                "event": "result",
                "ea": ea_hex,
                "size": sz,
                "name": name,
                "status": "failed",
                "elapsed_ms": int((time.time() - tf0) * 1000),
                "error": desc or "decompile returned None",
            }) + "\n")
            out_f.flush()
            continue

        sv = cfunc.get_pseudocode()
        lines = []
        line_ea = []
        tree_items = cfunc.treeitems
        num_items = len(tree_items)
        for y, sl in enumerate(sv):
            raw_sl = sl.line
            lines.append(ida_lines.tag_remove(raw_sl))
            eas = []
            for m in re_addr.finditer(raw_sl):
                anchor = int(m.group(1), 16)
                if 0 <= anchor < num_items:
                    it_ea = tree_items[anchor].ea
                    if it_ea != idaapi.BADADDR:
                        eas.append(int(it_ea))
            if eas:
                line_ea.append([y + 1, hex(min(eas)), hex(max(eas))])

        raw_c = "\n".join(lines).replace("\r\n", "\n").replace("\r", "\n")
        vars_list = extract_vars(cfunc)
        el_ms = int((time.time() - tf0) * 1000)
        ok_count += 1
        out_f.write(json.dumps({
            "event": "result",
            "ea": ea_hex,
            "size": sz,
            "name": name,
            "status": "ok",
            "elapsed_ms": el_ms,
            "raw_c": raw_c,
            "line_ea": line_ea,
            "vars": vars_list,
        }) + "\n")
        out_f.flush()
        if ok_count % 250 == 0:
            try:
                ida_hexrays.clear_cached_cfuncs()
            except Exception:
                pass

print(f"export_ida_decomp: ok={ok_count} fail={fail_count} total={len(targets)} in {time.time() - t0:.2f}s", flush=True)
idc.qexit(0)
