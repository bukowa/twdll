import ida_loader
ida_loader.set_database_flag(ida_loader.DBFL_TEMP)

import json
import os
import time
import ida_auto
import ida_bytes
import ida_funcs
import ida_lines
import ida_nalt
import ida_segment
import ida_typeinf
import idaapi
import idautils
import idc

t0 = time.time()
ida_auto.auto_wait()

out_path = os.environ["TWDLL_SILVER_OUT"]
asm_out_path = os.environ.get("TWDLL_SILVER_ASM_OUT", "").strip()
strings_out_path = os.environ.get("TWDLL_SILVER_STRINGS_OUT", "").strip()
total_out_path = os.environ.get("TWDLL_SILVER_TOTAL_OUT", "").strip()
if total_out_path:
    try:
        with open(total_out_path, "w", encoding="utf-8") as tf:
            tf.write(str(int(ida_funcs.get_func_qty())))
    except Exception:
        pass

CC_MAP = {
    ida_typeinf.CM_CC_CDECL: "__cdecl",
    ida_typeinf.CM_CC_ELLIPSIS: "__cdecl",
    ida_typeinf.CM_CC_VOIDARG: "__cdecl",
    ida_typeinf.CM_CC_STDCALL: "__stdcall",
    ida_typeinf.CM_CC_PASCAL: "__pascal",
    ida_typeinf.CM_CC_FASTCALL: "__fastcall",
    ida_typeinf.CM_CC_THISCALL: "__thiscall",
    ida_typeinf.CM_CC_SWIFT: "__swiftcall",
    ida_typeinf.CM_CC_GOLANG: "__golang",
    ida_typeinf.CM_CC_GOSTK: "__gostk",
    ida_typeinf.CM_CC_RUST: "__rustcall",
    ida_typeinf.CM_CC_SPECIAL: "__usercall",
    ida_typeinf.CM_CC_SPECIALE: "__usercall",
    ida_typeinf.CM_CC_SPECIALP: "__userpurge",
}

DSM_FLAGS = ida_lines.GENDSM_FORCE_CODE | ida_lines.GENDSM_REMOVE_TAGS

count = 0
tinfo = ida_typeinf.tinfo_t()
asm_f = open(asm_out_path, "w", encoding="utf-8") if asm_out_path else None
try:
    with open(out_path, "w", encoding="utf-8") as out_f:
        for ea in idautils.Functions():
            fn = ida_funcs.get_func(ea)
            if fn is None:
                continue

            ea_int = int(fn.start_ea)
            ea_hex = hex(ea_int)
            ranges = [
                [hex(int(ch.start_ea)), hex(int(ch.end_ea))]
                for ch in ida_funcs.func_tail_iterator_t(fn)
                if ch is not None and ch.end_ea > ch.start_ea
            ]
            size = int(ida_funcs.calc_func_size(fn))
            name = ida_funcs.get_func_name(fn.start_ea) or ""

            cc = ""
            sig = ""
            if ida_nalt.get_tinfo(tinfo, fn.start_ea) and tinfo.is_func():
                cc = CC_MAP.get(tinfo.get_cc(), "")
                sig = (
                    ida_typeinf.print_type(fn.start_ea, ida_typeinf.PRTYPE_1LINE)
                    or tinfo._print(name, ida_typeinf.PRTYPE_1LINE)
                    or ""
                )

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
                "ea": ea_hex,
                "size": size,
                "ranges": ranges,
                "name": name,
                "signature": sig,
                "calling_conv": cc,
                "stack_purge": stack_purge,
                "is_thunk": is_thunk,
                "is_noret": is_noret,
                "thunk_target_ea": hex(thunk_target),
            }
            out_f.write(json.dumps(rec) + "\n")

            if asm_f is not None:
                asm_lines = []
                for item_ea in ida_funcs.func_item_iterator_t(fn):
                    dis = ida_lines.generate_disasm_line(item_ea, DSM_FLAGS) or ""
                    asm_lines.append(f"{int(item_ea):x}: {dis}")
                asm_f.write(
                    json.dumps({
                        "ea": ea_hex,
                        "insn_count": len(asm_lines),
                        "raw_asm": "\n".join(asm_lines),
                    }) + "\n"
                )

            count += 1
            if count % 1000 == 0:
                out_f.flush()
        out_f.flush()
        if asm_f is not None:
            asm_f.flush()
finally:
    if asm_f is not None:
        asm_f.close()

print(f"export_ida_silver: exported {count} functions in {time.time() - t0:.2f}s", flush=True)

if strings_out_path:
    def _str_encoding(strtype, val):
        try:
            code = ida_nalt.get_str_type_code(strtype)
        except Exception:
            code = strtype & 0xFF
        if code == getattr(ida_nalt, "STRTYPE_C_16", 1):
            return "utf16le"
        if code == getattr(ida_nalt, "STRTYPE_C_32", 2):
            return "utf32le"
        if any(ord(ch) > 0x7F for ch in val):
            return "utf8"
        return "ascii"

    def _decode_strlit(ea, strtype, length=-1):
        try:
            raw = ida_bytes.get_strlit_contents(ea, length, strtype)
        except Exception:
            raw = None
        if not raw:
            return ""
        if isinstance(raw, bytes):
            return raw.decode("utf-8", errors="replace")
        return str(raw)

    str_records = {}
    for seg_ea in idautils.Segments():
        seg = ida_segment.getseg(seg_ea)
        if seg is None or seg.type in (ida_segment.SEG_CODE, ida_segment.SEG_XTRN, ida_segment.SEG_BSS):
            continue
        for head_ea in idautils.Heads(seg.start_ea, seg.end_ea):
            flags = ida_bytes.get_full_flags(head_ea)
            if not ida_bytes.is_strlit(flags):
                continue
            ea_int = int(head_ea)
            st = ida_nalt.get_str_type(ea_int)
            if st is None or st == idaapi.BADADDR or st < 0:
                st = ida_nalt.STRTYPE_C
            val = _decode_strlit(ea_int, st)
            if not val:
                continue
            ref_cnt = sum(1 for _ in idautils.DataRefsTo(ea_int))
            str_records[ea_int] = {
                "ea": hex(ea_int),
                "encoding": _str_encoding(st, val),
                "length": len(val),
                "value": val,
                "ref_count": ref_cnt,
            }

    try:
        ida_strs = idautils.Strings(default_setup=True)
        try:
            ida_strs.setup(
                strtypes=[ida_nalt.STRTYPE_C, ida_nalt.STRTYPE_C_16, ida_nalt.STRTYPE_C_32],
                minlen=4,
                only_7bit=False,
            )
        except Exception:
            try:
                ida_strs.setup(minlen=4)
            except Exception:
                pass
        for item in ida_strs:
            ea_int = int(item.ea)
            if ea_int in str_records:
                continue
            st = int(getattr(item, "strtype", ida_nalt.STRTYPE_C))
            val = _decode_strlit(ea_int, st, int(getattr(item, "length", -1)))
            if not val:
                val = str(item)
            if not val:
                continue
            ref_cnt = sum(1 for _ in idautils.DataRefsTo(ea_int))
            str_records[ea_int] = {
                "ea": hex(ea_int),
                "encoding": _str_encoding(st, val),
                "length": len(val),
                "value": val,
                "ref_count": ref_cnt,
            }
    except Exception:
        pass

    with open(strings_out_path, "w", encoding="utf-8") as sf:
        for ea_int in sorted(str_records):
            sf.write(json.dumps(str_records[ea_int]) + "\n")
        sf.flush()
    print(f"export_ida_silver: exported {len(str_records)} strings", flush=True)

idc.qexit(0)
