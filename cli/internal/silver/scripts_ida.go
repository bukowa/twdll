package silver

const exportIDASilverPy = `import ida_loader
ida_loader.set_database_flag(ida_loader.DBFL_TEMP)

import json
import os
import time
import ida_auto
import ida_funcs
import ida_nalt
import ida_typeinf
import idaapi
import idautils
import idc

t0 = time.time()
ida_auto.auto_wait()

out_path = os.environ["TWDLL_SILVER_OUT"]

CC_MAP = {
    ida_typeinf.CM_CC_THISCALL: "__thiscall",
    ida_typeinf.CM_CC_STDCALL: "__stdcall",
    ida_typeinf.CM_CC_FASTCALL: "__fastcall",
    ida_typeinf.CM_CC_CDECL: "__cdecl",
}

def get_saved_cc(ea):
    tinfo = ida_typeinf.tinfo_t()
    if ida_nalt.get_tinfo(tinfo, ea) and tinfo.is_func():
        ftd = ida_typeinf.func_type_data_t()
        if tinfo.get_func_details(ftd):
            return CC_MAP.get(ftd.get_cc() & ida_typeinf.CM_CC_MASK, "")
    return ""

count = 0
with open(out_path, "w", encoding="utf-8") as out_f:
    for ea in idautils.Functions():
        fn = ida_funcs.get_func(ea)
        if fn is None:
            continue
        ranges = []
        fii = ida_funcs.func_tail_iterator_t(fn)
        ok = fii.first()
        while ok:
            ch = fii.chunk()
            if ch is not None and ch.end_ea > ch.start_ea:
                ranges.append([hex(int(ch.start_ea)), hex(int(ch.end_ea))])
            ok = fii.next()
        size = int(fn.end_ea - fn.start_ea)
        name = ida_funcs.get_func_name(fn.start_ea) or ""
        cc = get_saved_cc(fn.start_ea)
        stack_purge = int(getattr(fn, "argsize", 0) or 0)
        is_thunk = bool(fn.flags & ida_funcs.FUNC_THUNK)
        is_noret = bool(fn.flags & ida_funcs.FUNC_NORET)
        thunk_target = 0
        if is_thunk:
            try:
                res = ida_funcs.calc_thunk_func_target(fn)
                tgt = res[0] if isinstance(res, tuple) else res
                if tgt is not None and tgt != idaapi.BADADDR:
                    thunk_target = int(tgt)
            except Exception:
                thunk_target = 0

        rec = {
            "ea": hex(int(fn.start_ea)),
            "size": size,
            "ranges": ranges,
            "name": name,
            "calling_conv": cc,
            "stack_purge": stack_purge,
            "is_thunk": is_thunk,
            "is_noret": is_noret,
            "thunk_target_ea": hex(thunk_target),
        }
        out_f.write(json.dumps(rec) + "\n")
        count += 1
    out_f.flush()

print(f"export_ida_silver: exported {count} functions in {time.time() - t0:.2f}s", flush=True)
idc.qexit(0)
`

const exportIDADecompPy = `import ida_loader
ida_loader.set_database_flag(ida_loader.DBFL_TEMP)

import json
import os
import re
import time
import ida_auto
import ida_funcs
import ida_hexrays
import ida_idp
import ida_lines
import idaapi
import idc

t0 = time.time()
ida_auto.auto_wait()

if not ida_hexrays.init_hexrays_plugin():
    raise RuntimeError("Hex-Rays decompiler not available")

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
                    reg_name = ida_idp.get_reg_name(lv.get_reg1(), max(1, int(lv.width))) or ""
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
            try:
                ida_funcs.add_func(ea)
            except Exception:
                pass
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
                "error": "get_func/add_func could not create function at entry EA",
            }) + "\n")
            out_f.flush()
            continue

        actual_sz = int(fn.end_ea - fn.start_ea)
        if sz <= 0:
            sz = actual_sz

        hf = ida_hexrays.hexrays_failure_t()
        try:
            cfunc = ida_hexrays.decompile(fn.start_ea, hf, ida_hexrays.DECOMP_NO_WAIT)
        except Exception as ex:
            fail_count += 1
            out_f.write(json.dumps({
                "event": "result",
                "ea": ea_hex,
                "size": sz,
                "name": name,
                "status": "failed",
                "elapsed_ms": int((time.time() - tf0) * 1000),
                "error": f"decompile error: {ex}",
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

print(f"export_ida_decomp: ok={ok_count} fail={fail_count} total={len(targets)} in {time.time() - t0:.2f}s", flush=True)
idc.qexit(0)
`
