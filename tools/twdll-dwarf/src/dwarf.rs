//! Four-pass streaming DWARF v4 and `.symtab` `_ZTV` extractor for `attila_linux_64`.

use std::collections::{HashMap, HashSet, VecDeque};
use fnv::{FnvHashMap, FnvHashSet};
use object::{Object, ObjectSection};

use crate::db::{
    DwarfBatch, EnumeratorRow, FunctionRow, InlineRow, ParameterRow, TypeRow, UnitRow, VariableRow,
    VtableEntryRow,
};
use crate::layout::{is_stl_stub_name, resolve_struct_layout, RawInheritSpec, RawMemberSpec};
use crate::vtable::build_elf_vtable_index;

#[derive(Clone, Debug)]
struct CuMember {
    name: String,
    type_off: Option<usize>,
    byte_offset: i64,
    byte_size: Option<i64>,
    bit_offset: Option<i64>,
    bit_size: Option<i64>,
    is_artificial: bool,
    is_decl: bool,
}

#[derive(Clone, Debug)]
struct CuInherit {
    type_off: Option<usize>,
    byte_offset: Option<i64>,
    vbase_vtable_offset: Option<i64>,
    is_virtual: bool,
}

#[derive(Clone, Debug)]
struct CuVirtualMethod {
    slot_index: i64,
    name: String,
    linkage_name: Option<String>,
    is_pure_virtual: bool,
}

#[derive(Clone, Debug)]
struct CuType {
    kind: &'static str,
    name: Option<String>,
    qual_name: String,
    byte_size: Option<i64>,
    target_off: Option<usize>,
    array_count: Option<i64>,
    containing_off: Option<usize>,
    is_decl: bool,
    has_virtual_method: bool,
    has_destructor: bool,
    has_user_copy_or_move_ctor: bool,
    members: Vec<CuMember>,
    inherits: Vec<CuInherit>,
    enumerators: Vec<(String, i64)>,
    virtual_methods: Vec<CuVirtualMethod>,
}

#[derive(Clone, Debug)]
struct CuParam {
    name: Option<String>,
    type_off: Option<usize>,
    is_artificial: bool,
    die_off: usize,
    abstract_origin_off: Option<usize>,
    storage_kind: &'static str,
    dwarf_reg: Option<i64>,
    reg_name: Option<String>,
    stack_offset: Option<i64>,
}

#[derive(Clone, Debug)]
struct CuVarRange {
    name: String,
    type_off: Option<usize>,
    range_index: i64,
    start_address: i64,
    end_address: i64,
    storage_kind: &'static str,
    reg_name: Option<String>,
    stack_offset: Option<i64>,
    const_value: Option<i64>,
}

#[derive(Clone, Debug)]
struct CuInline {
    abstract_origin: Option<usize>,
    callee_name: Option<String>,
    start_address: i64,
    end_address: i64,
    call_file: Option<String>,
    call_line: Option<i64>,
}

#[derive(Clone, Debug)]
struct CuSubprog {
    name: Option<String>,
    qual_name: String,
    linkage_name: Option<String>,
    enclosing_type_off: Option<usize>,
    containing_type_off: Option<usize>,
    return_type_off: Option<usize>,
    specification_off: Option<usize>,
    abstract_origin_off: Option<usize>,
    object_pointer_off: Option<usize>,
    low_pc: Option<i64>,
    byte_size: Option<i64>,
    vtable_slot_index: Option<i64>,
    virtuality: i64,
    is_inline: bool,
    is_decl: bool,
    is_variadic: bool,
    is_artificial: bool,
    decl_file: Option<String>,
    decl_line: Option<i64>,
    params: Vec<CuParam>,
    variables: Vec<CuVarRange>,
    inlines: Vec<CuInline>,
}

fn ghidra_reg_name(reg: u16) -> Option<String> {
    gimli::X86_64::register_name(gimli::Register(reg)).map(|s| s.to_ascii_uppercase())
}

fn ref_to_section_offset<R: gimli::Reader<Offset = usize>>(
    unit: &gimli::UnitRef<'_, R>,
    val: &gimli::AttributeValue<R>,
) -> Option<usize> {
    match val {
        gimli::AttributeValue::UnitRef(uo) => Some(uo.to_unit_section_offset(unit).0),
        gimli::AttributeValue::DebugInfoRef(doff) => Some(doff.0),
        _ => None,
    }
}

fn eval_member_location<R: gimli::Reader<Offset = usize>>(
    unit: &gimli::UnitRef<'_, R>,
    val: &gimli::AttributeValue<R>,
) -> (Option<i64>, Option<i64>) {
    match val {
        gimli::AttributeValue::Udata(v) => (Some(*v as i64), None),
        gimli::AttributeValue::Sdata(v) => (Some(*v), None),
        gimli::AttributeValue::Exprloc(expr) => {
            let mut ops = expr.clone().operations(unit.encoding());
            let mut ops_vec = Vec::new();
            while let Ok(Some(op)) = ops.next() {
                ops_vec.push(op);
            }
            if ops_vec.len() == 1 {
                match ops_vec[0] {
                    gimli::Operation::PlusConstant { value }
                    | gimli::Operation::UnsignedConstant { value } => {
                        return (Some(value as i64), None);
                    }
                    gimli::Operation::SignedConstant { value } => return (Some(value), None),
                    _ => {}
                }
            }
            // Virtual base vtable offset pattern: `Pick(0), Deref(..), UnsignedConstant(off), Minus, ...`
            for window in ops_vec.windows(2) {
                if let (
                    gimli::Operation::UnsignedConstant { value },
                    gimli::Operation::Minus,
                ) = (&window[0], &window[1])
                {
                    return (None, Some(*value as i64));
                }
            }
            (None, None)
        }
        _ => (None, None),
    }
}

fn eval_vtable_slot<R: gimli::Reader<Offset = usize>>(
    unit: &gimli::UnitRef<'_, R>,
    val: &gimli::AttributeValue<R>,
) -> Option<i64> {
    match val {
        gimli::AttributeValue::Udata(v) => Some(*v as i64),
        gimli::AttributeValue::Sdata(v) => Some(*v),
        gimli::AttributeValue::Exprloc(expr) => {
            let mut ops = expr.clone().operations(unit.encoding());
            while let Ok(Some(op)) = ops.next() {
                match op {
                    gimli::Operation::UnsignedConstant { value }
                    | gimli::Operation::PlusConstant { value } => return Some(value as i64),
                    gimli::Operation::SignedConstant { value } => return Some(value),
                    _ => {}
                }
            }
            None
        }
        _ => None,
    }
}

fn eval_frame_base_cfa_adjust<R: gimli::Reader<Offset = usize>>(
    unit: &gimli::UnitRef<'_, R>,
    val: &gimli::AttributeValue<R>,
) -> i64 {
    if let gimli::AttributeValue::Exprloc(expr) = val {
        let mut ops = expr.clone().operations(unit.encoding());
        while let Ok(Some(op)) = ops.next() {
            match op {
                gimli::Operation::CallFrameCFA => return 8,
                gimli::Operation::RegisterOffset {
                    register: gimli::Register(7),
                    offset,
                    ..
                } => return offset,
                gimli::Operation::RegisterOffset {
                    register: gimli::Register(6),
                    offset,
                    ..
                } => return offset + 16,
                _ => {}
            }
        }
    }
    // Default System V x86-64 `DW_AT_frame_base` is `CFA = RSP_entry + 8`.
    8
}

#[derive(Clone, Debug)]
struct ParsedExprLoc {
    storage_kind: &'static str,
    dwarf_reg: Option<i64>,
    reg_name: Option<String>,
    stack_offset: Option<i64>,
    const_value: Option<i64>,
}

fn parse_single_expr<R: gimli::Reader<Offset = usize>>(
    unit: &gimli::UnitRef<'_, R>,
    expr: gimli::Expression<R>,
    cfa_adjust: i64,
) -> ParsedExprLoc {
    let mut ops = expr.operations(unit.encoding());
    let mut reg: Option<u16> = None;
    let mut fb_off: Option<i64> = None;
    let mut const_val: Option<i64> = None;
    let mut has_stack_val = false;

    while let Ok(Some(op)) = ops.next() {
        match op {
            gimli::Operation::Register { register } => {
                if reg.is_none() && fb_off.is_none() {
                    reg = Some(register.0);
                }
            }
            gimli::Operation::FrameOffset { offset } => {
                if fb_off.is_none() && reg.is_none() {
                    fb_off = Some(offset + cfa_adjust);
                }
            }
            gimli::Operation::RegisterOffset {
                register: gimli::Register(7),
                offset,
                ..
            } => {
                if fb_off.is_none() && reg.is_none() {
                    fb_off = Some(offset);
                }
            }
            gimli::Operation::UnsignedConstant { value } => {
                const_val = Some(value as i64);
            }
            gimli::Operation::SignedConstant { value } => {
                const_val = Some(value);
            }
            gimli::Operation::StackValue => {
                has_stack_val = true;
            }
            _ => {}
        }
    }

    if has_stack_val && const_val.is_some() {
        return ParsedExprLoc {
            storage_kind: "constant",
            dwarf_reg: None,
            reg_name: None,
            stack_offset: None,
            const_value: const_val,
        };
    }
    if let Some(r) = reg {
        return ParsedExprLoc {
            storage_kind: "register",
            dwarf_reg: Some(i64::from(r)),
            reg_name: ghidra_reg_name(r),
            stack_offset: None,
            const_value: None,
        };
    }
    if let Some(off) = fb_off {
        return ParsedExprLoc {
            storage_kind: "stack",
            dwarf_reg: None,
            reg_name: None,
            stack_offset: Some(off),
            const_value: None,
        };
    }
    ParsedExprLoc {
        storage_kind: "none",
        dwarf_reg: None,
        reg_name: None,
        stack_offset: None,
        const_value: None,
    }
}

fn resolve_param_location<R: gimli::Reader<Offset = usize>>(
    unit: &gimli::UnitRef<'_, R>,
    val: gimli::AttributeValue<R>,
    low_pc: Option<i64>,
    cfa_adjust: i64,
) -> ParsedExprLoc {
    match val {
        gimli::AttributeValue::Exprloc(expr) => parse_single_expr(unit, expr, cfa_adjust),
        other => {
            if let Ok(Some(off)) = unit.attr_locations_offset(other)
                && let Ok(mut locs) = unit.locations(off)
            {
                let mut first: Option<ParsedExprLoc> = None;
                while let Ok(Some(loc)) = locs.next() {
                    let parsed = parse_single_expr(unit, loc.data, cfa_adjust);
                    if parsed.storage_kind == "none" {
                        continue;
                    }
                    if let Some(pc) = low_pc
                        && (loc.range.begin as i64) <= pc
                        && pc < (loc.range.end as i64)
                    {
                        return parsed;
                    }
                    if first.is_none() {
                        first = Some(parsed);
                    }
                }
                if let Some(f) = first {
                    return f;
                }
            }
            ParsedExprLoc {
                storage_kind: "none",
                dwarf_reg: None,
                reg_name: None,
                stack_offset: None,
                const_value: None,
            }
        }
    }
}

fn resolve_variable_ranges<R: gimli::Reader<Offset = usize>>(
    unit: &gimli::UnitRef<'_, R>,
    name: String,
    type_off: Option<usize>,
    val: gimli::AttributeValue<R>,
    func_low_pc: i64,
    func_high_pc: i64,
    cfa_adjust: i64,
) -> Vec<CuVarRange> {
    let mut out = Vec::new();
    match val {
        gimli::AttributeValue::Exprloc(expr) => {
            let parsed = parse_single_expr(unit, expr, cfa_adjust);
            if parsed.storage_kind != "none" {
                out.push(CuVarRange {
                    name,
                    type_off,
                    range_index: 0,
                    start_address: func_low_pc,
                    end_address: func_high_pc.max(func_low_pc + 1),
                    storage_kind: parsed.storage_kind,
                    reg_name: parsed.reg_name,
                    stack_offset: parsed.stack_offset,
                    const_value: parsed.const_value,
                });
            }
        }
        other => {
            if let Ok(Some(off)) = unit.attr_locations_offset(other)
                && let Ok(mut locs) = unit.locations(off)
            {
                let mut idx = 0i64;
                while let Ok(Some(loc)) = locs.next() {
                    let start = loc.range.begin as i64;
                    let end = loc.range.end as i64;
                    if end <= start || start < func_low_pc {
                        continue;
                    }
                    let parsed = parse_single_expr(unit, loc.data, cfa_adjust);
                    if parsed.storage_kind != "none" {
                        out.push(CuVarRange {
                            name: name.clone(),
                            type_off,
                            range_index: idx,
                            start_address: start,
                            end_address: end,
                            storage_kind: parsed.storage_kind,
                            reg_name: parsed.reg_name,
                            stack_offset: parsed.stack_offset,
                            const_value: parsed.const_value,
                        });
                        idx += 1;
                    }
                }
            }
        }
    }
    out
}

fn file_path_from_unit<R: gimli::Reader<Offset = usize>>(
    unit: &gimli::UnitRef<'_, R>,
    file_index: u64,
) -> Option<String> {
    let header = unit.line_program.as_ref()?.header();
    let file = header.file(file_index)?;
    let file_str = unit.attr_string(file.path_name()).ok()?;
    let file_name = file_str.to_string().ok()?;
    if let Some(dir_attr) = file.directory(header)
        && let Ok(dir_s) = unit.attr_string(dir_attr)
        && let Ok(dir_name) = dir_s.to_string()
        && !dir_name.is_empty()
    {
        return Some(format!("{dir_name}/{file_name}"));
    }
    Some(file_name.to_string())
}

#[allow(clippy::too_many_arguments)]
fn walk_cu_node<R: gimli::Reader<Offset = usize>>(
    unit_id: i64,
    unit: &gimli::UnitRef<'_, R>,
    node: gimli::EntriesTreeNode<'_, '_, R>,
    ns_stack: &mut Vec<String>,
    enclosing_type_off: Option<usize>,
    in_function_scope: bool,
    cu_types: &mut FnvHashMap<usize, CuType>,
    cu_subprogs: &mut FnvHashMap<usize, CuSubprog>,
) {
    let entry = node.entry();
    let tag = entry.tag();
    let off = entry.offset().to_unit_section_offset(unit).0;

    let mut name: Option<String> = None;
    let mut linkage_name: Option<String> = None;
    let mut is_decl = false;
    let mut is_artificial = false;
    let mut byte_size: Option<i64> = None;
    let mut type_off: Option<usize> = None;
    let mut containing_off: Option<usize> = None;
    let mut specification_off: Option<usize> = None;
    let mut abstract_origin_off: Option<usize> = None;
    let mut object_pointer_off: Option<usize> = None;
    let mut low_pc: Option<i64> = None;
    let mut high_pc_size: Option<i64> = None;
    let mut virtuality: i64 = 0;
    let mut vtable_slot_index: Option<i64> = None;
    let mut is_inline = false;
    let mut cfa_adjust: i64 = 8;
    let mut decl_file: Option<String> = None;
    let mut decl_line: Option<i64> = None;

    for attr in entry.attrs() {
        match attr.name() {
            gimli::DW_AT_name => {
                if let Ok(s) = unit.attr_string(attr.value())
                    && let Ok(st) = s.to_string()
                {
                    name = Some(st.to_string());
                }
            }
            gimli::DW_AT_linkage_name | gimli::DW_AT_MIPS_linkage_name => {
                if let Ok(s) = unit.attr_string(attr.value())
                    && let Ok(st) = s.to_string()
                {
                    linkage_name = Some(st.to_string());
                }
            }
            gimli::DW_AT_declaration => {
                if let gimli::AttributeValue::Flag(f) = attr.value() {
                    is_decl = f;
                }
            }
            gimli::DW_AT_artificial => {
                if let gimli::AttributeValue::Flag(f) = attr.value() {
                    is_artificial = f;
                }
            }
            gimli::DW_AT_byte_size => {
                byte_size = attr.udata_value().map(|v| v as i64);
            }
            gimli::DW_AT_type => {
                type_off = ref_to_section_offset(unit, &attr.value());
            }
            gimli::DW_AT_containing_type => {
                containing_off = ref_to_section_offset(unit, &attr.value());
            }
            gimli::DW_AT_specification => {
                specification_off = ref_to_section_offset(unit, &attr.value());
            }
            gimli::DW_AT_abstract_origin => {
                abstract_origin_off = ref_to_section_offset(unit, &attr.value());
            }
            gimli::DW_AT_object_pointer => {
                object_pointer_off = ref_to_section_offset(unit, &attr.value());
            }
            gimli::DW_AT_low_pc => {
                if let Ok(Some(addr)) = unit.attr_address(attr.value())
                    && addr != 0
                {
                    low_pc = Some(addr as i64);
                }
            }
            gimli::DW_AT_high_pc => match attr.value() {
                gimli::AttributeValue::Udata(sz) => high_pc_size = Some(sz as i64),
                other => {
                    if let Ok(Some(end_addr)) = unit.attr_address(other)
                        && let Some(start) = low_pc
                        && (end_addr as i64) >= start
                    {
                        high_pc_size = Some((end_addr as i64) - start);
                    }
                }
            },
            gimli::DW_AT_virtuality => {
                if let gimli::AttributeValue::Virtuality(v) = attr.value() {
                    virtuality = i64::from(v.0);
                }
            }
            gimli::DW_AT_vtable_elem_location => {
                vtable_slot_index = eval_vtable_slot(unit, &attr.value());
            }
            gimli::DW_AT_inline => {
                if let gimli::AttributeValue::Inline(inl) = attr.value() {
                    is_inline = inl.0 != 0;
                }
            }
            gimli::DW_AT_frame_base => {
                cfa_adjust = eval_frame_base_cfa_adjust(unit, &attr.value());
            }
            gimli::DW_AT_decl_file => {
                if let gimli::AttributeValue::FileIndex(fidx) = attr.value() {
                    decl_file = file_path_from_unit(unit, fidx);
                }
            }
            gimli::DW_AT_decl_line => {
                decl_line = attr.udata_value().map(|v| v as i64);
            }
            _ => {}
        }
    }

    let make_qual = |stack: &[String], n: Option<&str>, anon_off: usize| -> String {
        match n {
            Some(s) if !in_function_scope => {
                if stack.is_empty() {
                    s.to_string()
                } else {
                    format!("{}::{s}", stack.join("::"))
                }
            }
            Some(s) => format!("local::{unit_id}::0x{anon_off:x}::{s}"),
            None => format!("anon::{unit_id}::0x{anon_off:x}"),
        }
    };

    match tag {
        gimli::DW_TAG_compile_unit => {
            let mut ch = node.children();
            while let Ok(Some(c)) = ch.next() {
                walk_cu_node(
                    unit_id,
                    unit,
                    c,
                    ns_stack,
                    None,
                    false,
                    cu_types,
                    cu_subprogs,
                );
            }
        }
        gimli::DW_TAG_namespace => {
            ns_stack.push(name.unwrap_or_else(|| "(anonymous namespace)".to_string()));
            let mut ch = node.children();
            while let Ok(Some(c)) = ch.next() {
                walk_cu_node(
                    unit_id,
                    unit,
                    c,
                    ns_stack,
                    enclosing_type_off,
                    in_function_scope,
                    cu_types,
                    cu_subprogs,
                );
            }
            ns_stack.pop();
        }
        gimli::DW_TAG_structure_type
        | gimli::DW_TAG_class_type
        | gimli::DW_TAG_union_type
        | gimli::DW_TAG_enumeration_type => {
            let kind = match tag {
                gimli::DW_TAG_structure_type => "struct",
                gimli::DW_TAG_class_type => "class",
                gimli::DW_TAG_union_type => "union",
                gimli::DW_TAG_enumeration_type => "enum",
                _ => "struct",
            };
            let qual_name = make_qual(ns_stack, name.as_deref(), off);
            let push_name = name
                .clone()
                .unwrap_or_else(|| format!("anon_0x{off:x}"));

            let mut members = Vec::new();
            let mut inherits = Vec::new();
            let mut enumerators = Vec::new();
            let mut virtual_methods = Vec::new();
            let mut has_virtual_method = false;
            let mut has_destructor = false;
            let mut child_subprogs = Vec::new();

            ns_stack.push(push_name);
            let mut ch = node.children();
            while let Ok(Some(c)) = ch.next() {
                let ctag = c.entry().tag();
                match ctag {
                    gimli::DW_TAG_member => {
                        let mut m = CuMember {
                            name: String::new(),
                            type_off: None,
                            byte_offset: 0,
                            byte_size: None,
                            bit_offset: None,
                            bit_size: None,
                            is_artificial: false,
                            is_decl: false,
                        };
                        for a in c.entry().attrs() {
                            match a.name() {
                                gimli::DW_AT_name => {
                                    if let Ok(s) = unit.attr_string(a.value())
                                        && let Ok(st) = s.to_string()
                                    {
                                        m.name = st.to_string();
                                    }
                                }
                                gimli::DW_AT_type => {
                                    m.type_off = ref_to_section_offset(unit, &a.value());
                                }
                                gimli::DW_AT_data_member_location => {
                                    let (bo, _) = eval_member_location(unit, &a.value());
                                    m.byte_offset = bo.unwrap_or(0);
                                }
                                gimli::DW_AT_byte_size => {
                                    m.byte_size = a.udata_value().map(|v| v as i64);
                                }
                                gimli::DW_AT_bit_offset | gimli::DW_AT_data_bit_offset => {
                                    m.bit_offset = a.udata_value().map(|v| v as i64);
                                }
                                gimli::DW_AT_bit_size => {
                                    m.bit_size = a.udata_value().map(|v| v as i64);
                                }
                                gimli::DW_AT_artificial => {
                                    if let gimli::AttributeValue::Flag(f) = a.value() {
                                        m.is_artificial = f;
                                    }
                                }
                                gimli::DW_AT_declaration => {
                                    if let gimli::AttributeValue::Flag(f) = a.value() {
                                        m.is_decl = f;
                                    }
                                }
                                _ => {}
                            }
                        }
                        if m.name.is_empty() {
                            m.name = format!("anon_member_0x{:x}", m.byte_offset);
                        }
                        members.push(m);
                    }
                    gimli::DW_TAG_inheritance => {
                        let mut inh = CuInherit {
                            type_off: None,
                            byte_offset: Some(0),
                            vbase_vtable_offset: None,
                            is_virtual: false,
                        };
                        for a in c.entry().attrs() {
                            match a.name() {
                                gimli::DW_AT_type => {
                                    inh.type_off = ref_to_section_offset(unit, &a.value());
                                }
                                gimli::DW_AT_data_member_location => {
                                    let (bo, vbo) = eval_member_location(unit, &a.value());
                                    inh.byte_offset = bo;
                                    inh.vbase_vtable_offset = vbo;
                                }
                                gimli::DW_AT_virtuality => {
                                    if let gimli::AttributeValue::Virtuality(v) = a.value() {
                                        inh.is_virtual = v.0 != 0;
                                    }
                                }
                                _ => {}
                            }
                        }
                        if inh.is_virtual {
                            inh.byte_offset = None;
                        }
                        inherits.push(inh);
                    }
                    gimli::DW_TAG_enumerator => {
                        let mut ename = String::new();
                        let mut eval: i64 = 0;
                        for a in c.entry().attrs() {
                            match a.name() {
                                gimli::DW_AT_name => {
                                    if let Ok(s) = unit.attr_string(a.value())
                                        && let Ok(st) = s.to_string()
                                    {
                                        ename = st.to_string();
                                    }
                                }
                                gimli::DW_AT_const_value => {
                                    eval = a
                                        .sdata_value()
                                        .or_else(|| a.udata_value().map(|u| u as i64))
                                        .unwrap_or(0);
                                }
                                _ => {}
                            }
                        }
                        if !ename.is_empty() {
                            enumerators.push((ename, eval));
                        }
                    }
                    gimli::DW_TAG_subprogram => {
                        let child_off = c.entry().offset().to_unit_section_offset(unit).0;
                        walk_cu_node(
                            unit_id,
                            unit,
                            c,
                            ns_stack,
                            Some(off),
                            in_function_scope,
                            cu_types,
                            cu_subprogs,
                        );
                        child_subprogs.push(child_off);
                    }
                    _ => {
                        walk_cu_node(
                            unit_id,
                            unit,
                            c,
                            ns_stack,
                            Some(off),
                            in_function_scope,
                            cu_types,
                            cu_subprogs,
                        );
                    }
                }
            }
            ns_stack.pop();

            for sp_off in &child_subprogs {
                if let Some(sp) = cu_subprogs.get(sp_off) {
                    if let Some(n) = &sp.name
                        && n.starts_with('~')
                    {
                        has_destructor = true;
                    }
                    if sp.virtuality != 0 {
                        has_virtual_method = true;
                        if let Some(slot) = sp.vtable_slot_index {
                            virtual_methods.push(CuVirtualMethod {
                                slot_index: slot,
                                name: sp
                                    .name
                                    .clone()
                                    .unwrap_or_else(|| format!("vfunc_{slot}")),
                                linkage_name: sp.linkage_name.clone(),
                                is_pure_virtual: sp.virtuality == 2,
                            });
                        }
                    }
                }
            }

            cu_types.insert(
                off,
                CuType {
                    kind,
                    name,
                    qual_name,
                    byte_size,
                    target_off: type_off,
                    array_count: None,
                    containing_off,
                    is_decl,
                    has_virtual_method,
                    has_destructor,
                    has_user_copy_or_move_ctor: false,
                    members,
                    inherits,
                    enumerators,
                    virtual_methods,
                },
            );
        }
        gimli::DW_TAG_base_type
        | gimli::DW_TAG_unspecified_type
        | gimli::DW_TAG_typedef
        | gimli::DW_TAG_pointer_type
        | gimli::DW_TAG_reference_type
        | gimli::DW_TAG_rvalue_reference_type
        | gimli::DW_TAG_const_type
        | gimli::DW_TAG_volatile_type
        | gimli::DW_TAG_restrict_type
        | gimli::DW_TAG_subroutine_type
        | gimli::DW_TAG_ptr_to_member_type
        | gimli::DW_TAG_array_type => {
            let kind = match tag {
                gimli::DW_TAG_base_type | gimli::DW_TAG_unspecified_type => "primitive",
                gimli::DW_TAG_typedef => "typedef",
                gimli::DW_TAG_pointer_type => "pointer",
                gimli::DW_TAG_reference_type => "reference",
                gimli::DW_TAG_rvalue_reference_type => "rvalue_reference",
                gimli::DW_TAG_const_type => "const",
                gimli::DW_TAG_volatile_type | gimli::DW_TAG_restrict_type => "volatile",
                gimli::DW_TAG_subroutine_type => "subroutine",
                gimli::DW_TAG_ptr_to_member_type => "ptr_to_member",
                gimli::DW_TAG_array_type => "array",
                _ => "primitive",
            };
            let mut array_count: Option<i64> = None;
            if tag == gimli::DW_TAG_array_type {
                let mut total_elems: i64 = 1;
                let mut found_dim = false;
                let mut ch = node.children();
                while let Ok(Some(c)) = ch.next() {
                    if c.entry().tag() == gimli::DW_TAG_subrange_type {
                        for a in c.entry().attrs() {
                            match a.name() {
                                gimli::DW_AT_count => {
                                    if let Some(cnt) = a.udata_value() {
                                        total_elems = total_elems.saturating_mul(cnt as i64);
                                        found_dim = true;
                                    }
                                }
                                gimli::DW_AT_upper_bound => {
                                    if let Some(ub) = a
                                        .udata_value()
                                        .map(|u| u as i64)
                                        .or_else(|| a.sdata_value())
                                        && ub >= 0
                                    {
                                        total_elems = total_elems.saturating_mul(ub + 1);
                                        found_dim = true;
                                    }
                                }
                                _ => {}
                            }
                        }
                    }
                }
                if found_dim {
                    array_count = Some(total_elems);
                }
            }
            let qual_name = match tag {
                gimli::DW_TAG_base_type | gimli::DW_TAG_unspecified_type => {
                    name.clone().unwrap_or_else(|| "void".to_string())
                }
                gimli::DW_TAG_typedef => make_qual(ns_stack, name.as_deref(), off),
                _ => format!("{kind}::unit{unit_id}::0x{off:x}"),
            };
            let default_size = match kind {
                "pointer" | "reference" | "rvalue_reference" => Some(8),
                "ptr_to_member" => Some(byte_size.unwrap_or(16)),
                _ => byte_size,
            };
            cu_types.insert(
                off,
                CuType {
                    kind,
                    name,
                    qual_name,
                    byte_size: default_size,
                    target_off: type_off,
                    array_count,
                    containing_off,
                    is_decl,
                    has_virtual_method: false,
                    has_destructor: false,
                    has_user_copy_or_move_ctor: false,
                    members: Vec::new(),
                    inherits: Vec::new(),
                    enumerators: Vec::new(),
                    virtual_methods: Vec::new(),
                },
            );
        }
        gimli::DW_TAG_subprogram => {
            let qual_name = name
                .as_deref()
                .map(|s| {
                    if ns_stack.is_empty() {
                        s.to_string()
                    } else {
                        format!("{}::{s}", ns_stack.join("::"))
                    }
                })
                .unwrap_or_else(|| format!("subprog::unit{unit_id}::0x{off:x}"));

            let mut params = Vec::new();
            let mut variables = Vec::new();
            let mut inlines = Vec::new();
            let mut is_variadic = false;

            let func_low = low_pc.unwrap_or(0);
            let func_high = func_low + high_pc_size.unwrap_or(0);

            walk_subprog_body(
                unit_id,
                unit,
                node,
                ns_stack,
                low_pc,
                func_low,
                func_high,
                cfa_adjust,
                true,
                &mut params,
                &mut is_variadic,
                &mut variables,
                &mut inlines,
                cu_types,
                cu_subprogs,
            );

            cu_subprogs.insert(
                off,
                CuSubprog {
                    name,
                    qual_name,
                    linkage_name,
                    enclosing_type_off,
                    containing_type_off: containing_off.or(enclosing_type_off),
                    return_type_off: type_off,
                    specification_off,
                    abstract_origin_off,
                    object_pointer_off,
                    low_pc,
                    byte_size: high_pc_size,
                    vtable_slot_index,
                    virtuality,
                    is_inline,
                    is_decl,
                    is_variadic,
                    is_artificial,
                    decl_file,
                    decl_line,
                    params,
                    variables,
                    inlines,
                },
            );
        }
        _ => {
            let mut ch = node.children();
            while let Ok(Some(c)) = ch.next() {
                walk_cu_node(
                    unit_id,
                    unit,
                    c,
                    ns_stack,
                    enclosing_type_off,
                    in_function_scope,
                    cu_types,
                    cu_subprogs,
                );
            }
        }
    }
}

#[allow(clippy::too_many_arguments)]
fn walk_subprog_body<R: gimli::Reader<Offset = usize>>(
    unit_id: i64,
    unit: &gimli::UnitRef<'_, R>,
    node: gimli::EntriesTreeNode<'_, '_, R>,
    ns_stack: &mut Vec<String>,
    low_pc: Option<i64>,
    func_low: i64,
    func_high: i64,
    cfa_adjust: i64,
    is_top_subprog: bool,
    params: &mut Vec<CuParam>,
    is_variadic: &mut bool,
    variables: &mut Vec<CuVarRange>,
    inlines: &mut Vec<CuInline>,
    cu_types: &mut FnvHashMap<usize, CuType>,
    cu_subprogs: &mut FnvHashMap<usize, CuSubprog>,
) {
    let mut ch = node.children();
    while let Ok(Some(c)) = ch.next() {
        let ctag = c.entry().tag();
        let coff = c.entry().offset().to_unit_section_offset(unit).0;
        match ctag {
            gimli::DW_TAG_formal_parameter if is_top_subprog => {
                let mut pname: Option<String> = None;
                let mut ptype: Option<usize> = None;
                let mut part = false;
                let mut porigin: Option<usize> = None;
                let mut ploc = ParsedExprLoc {
                    storage_kind: "none",
                    dwarf_reg: None,
                    reg_name: None,
                    stack_offset: None,
                    const_value: None,
                };
                for a in c.entry().attrs() {
                    match a.name() {
                        gimli::DW_AT_name => {
                            if let Ok(s) = unit.attr_string(a.value())
                                && let Ok(st) = s.to_string()
                            {
                                pname = Some(st.to_string());
                            }
                        }
                        gimli::DW_AT_type => {
                            ptype = ref_to_section_offset(unit, &a.value());
                        }
                        gimli::DW_AT_artificial => {
                            if let gimli::AttributeValue::Flag(f) = a.value() {
                                part = f;
                            }
                        }
                        gimli::DW_AT_abstract_origin => {
                            porigin = ref_to_section_offset(unit, &a.value());
                        }
                        gimli::DW_AT_location if low_pc.is_some() => {
                            ploc = resolve_param_location(unit, a.value(), low_pc, cfa_adjust);
                        }
                        _ => {}
                    }
                }
                params.push(CuParam {
                    name: pname,
                    type_off: ptype,
                    is_artificial: part,
                    die_off: coff,
                    abstract_origin_off: porigin,
                    storage_kind: ploc.storage_kind,
                    dwarf_reg: ploc.dwarf_reg,
                    reg_name: ploc.reg_name,
                    stack_offset: ploc.stack_offset,
                });
            }
            gimli::DW_TAG_unspecified_parameters if is_top_subprog => {
                *is_variadic = true;
            }
            gimli::DW_TAG_variable if low_pc.is_some() => {
                let mut vname: Option<String> = None;
                let mut vtype: Option<usize> = None;
                let mut vloc: Option<gimli::AttributeValue<R>> = None;
                let mut vconst: Option<i64> = None;
                for a in c.entry().attrs() {
                    match a.name() {
                        gimli::DW_AT_name => {
                            if let Ok(s) = unit.attr_string(a.value())
                                && let Ok(st) = s.to_string()
                            {
                                vname = Some(st.to_string());
                            }
                        }
                        gimli::DW_AT_type => {
                            vtype = ref_to_section_offset(unit, &a.value());
                        }
                        gimli::DW_AT_location => {
                            vloc = Some(a.value());
                        }
                        gimli::DW_AT_const_value => {
                            vconst = a
                                .sdata_value()
                                .or_else(|| a.udata_value().map(|u| u as i64));
                        }
                        _ => {}
                    }
                }
                if let Some(vn) = vname {
                    if let Some(loc_attr) = vloc {
                        let ranges = resolve_variable_ranges(
                            unit, vn, vtype, loc_attr, func_low, func_high, cfa_adjust,
                        );
                        variables.extend(ranges);
                    } else if let Some(cv) = vconst {
                        variables.push(CuVarRange {
                            name: vn,
                            type_off: vtype,
                            range_index: 0,
                            start_address: func_low,
                            end_address: func_high.max(func_low + 1),
                            storage_kind: "constant",
                            reg_name: None,
                            stack_offset: None,
                            const_value: Some(cv),
                        });
                    }
                }
            }
            gimli::DW_TAG_inlined_subroutine if low_pc.is_some() => {
                let mut origin: Option<usize> = None;
                let mut ilow: Option<i64> = None;
                let mut ihigh_sz: Option<i64> = None;
                let mut iranges: Option<gimli::RangeListsOffset> = None;
                let mut cfile: Option<String> = None;
                let mut cline: Option<i64> = None;
                for a in c.entry().attrs() {
                    match a.name() {
                        gimli::DW_AT_abstract_origin => {
                            origin = ref_to_section_offset(unit, &a.value());
                        }
                        gimli::DW_AT_low_pc => {
                            if let Ok(Some(addr)) = unit.attr_address(a.value())
                                && addr != 0
                            {
                                ilow = Some(addr as i64);
                            }
                        }
                        gimli::DW_AT_high_pc => match a.value() {
                            gimli::AttributeValue::Udata(sz) => ihigh_sz = Some(sz as i64),
                            other => {
                                if let Ok(Some(end_addr)) = unit.attr_address(other)
                                    && let Some(st) = ilow
                                    && (end_addr as i64) >= st
                                {
                                    ihigh_sz = Some((end_addr as i64) - st);
                                }
                            }
                        },
                        gimli::DW_AT_ranges => {
                            if let Ok(Some(roff)) = unit.attr_ranges_offset(a.value()) {
                                iranges = Some(roff);
                            }
                        }
                        gimli::DW_AT_call_file => {
                            if let gimli::AttributeValue::FileIndex(fidx) = a.value() {
                                cfile = file_path_from_unit(unit, fidx);
                            }
                        }
                        gimli::DW_AT_call_line => {
                            cline = a.udata_value().map(|v| v as i64);
                        }
                        _ => {}
                    }
                }
                if let Some(st) = ilow {
                    let end = st + ihigh_sz.unwrap_or(1).max(1);
                    inlines.push(CuInline {
                        abstract_origin: origin,
                        callee_name: None,
                        start_address: st,
                        end_address: end,
                        call_file: cfile.clone(),
                        call_line: cline,
                    });
                } else if let Some(roff) = iranges
                    && let Ok(mut riter) = unit.ranges(roff)
                {
                    while let Ok(Some(r)) = riter.next() {
                        if r.end > r.begin && r.begin != 0 {
                            inlines.push(CuInline {
                                abstract_origin: origin,
                                callee_name: None,
                                start_address: r.begin as i64,
                                end_address: r.end as i64,
                                call_file: cfile.clone(),
                                call_line: cline,
                            });
                        }
                    }
                }
                walk_subprog_body(
                    unit_id,
                    unit,
                    c,
                    ns_stack,
                    low_pc,
                    func_low,
                    func_high,
                    cfa_adjust,
                    false,
                    params,
                    is_variadic,
                    variables,
                    inlines,
                    cu_types,
                    cu_subprogs,
                );
            }
            gimli::DW_TAG_lexical_block if low_pc.is_some() => {
                walk_subprog_body(
                    unit_id,
                    unit,
                    c,
                    ns_stack,
                    low_pc,
                    func_low,
                    func_high,
                    cfa_adjust,
                    false,
                    params,
                    is_variadic,
                    variables,
                    inlines,
                    cu_types,
                    cu_subprogs,
                );
            }
            _ if low_pc.is_some() => {
                walk_cu_node(
                    unit_id,
                    unit,
                    c,
                    ns_stack,
                    None,
                    true,
                    cu_types,
                    cu_subprogs,
                );
            }
            _ => {}
        }
    }
}

fn unwrap_cv_typedef(cu_types: &FnvHashMap<usize, CuType>, mut off: usize) -> Option<usize> {
    for _ in 0..12 {
        let t = cu_types.get(&off)?;
        if matches!(t.kind, "const" | "volatile" | "typedef") {
            off = t.target_off?;
        } else {
            return Some(off);
        }
    }
    None
}

fn detect_copy_or_move_ctors(
    cu_types: &mut FnvHashMap<usize, CuType>,
    cu_subprogs: &FnvHashMap<usize, CuSubprog>,
) {
    let mut copy_ctor_classes = Vec::new();
    for sp in cu_subprogs.values() {
        if sp.is_artificial {
            continue;
        }
        let Some(cls_off) = sp.enclosing_type_off else {
            continue;
        };
        let Some(cls) = cu_types.get(&cls_off) else {
            continue;
        };
        if sp.name.as_deref() != cls.name.as_deref() || sp.params.len() < 2 {
            continue;
        }
        // Check if param[1] (after `this`) is `&Class` or `&&Class` (possibly `const`).
        if let Some(p1_off) = sp.params[1].type_off
            && let Some(p1_t) = cu_types.get(&p1_off)
            && matches!(p1_t.kind, "reference" | "rvalue_reference")
            && let Some(target_off) = p1_t.target_off
            && let Some(unwrapped) = unwrap_cv_typedef(cu_types, target_off)
            && unwrapped == cls_off
        {
            copy_ctor_classes.push(cls_off);
        }
    }
    for cls_off in copy_ctor_classes {
        if let Some(cls) = cu_types.get_mut(&cls_off) {
            cls.has_user_copy_or_move_ctor = true;
        }
    }
}

pub fn extract_elf_dwarf(elf_bytes: &[u8]) -> Result<DwarfBatch, String> {
    let obj = object::File::parse(elf_bytes).map_err(|e| format!("Failed parsing ELF: {e}"))?;
    if !obj.is_64() {
        return Err("Target binary is not 64-bit ELF".to_string());
    }

    let vtable_index = build_elf_vtable_index(&obj);
    let endian = gimli::RunTimeEndian::Little;
    let load_sec =
        |id: gimli::SectionId| -> gimli::Result<gimli::EndianSlice<'_, gimli::RunTimeEndian>> {
            let bytes = obj
                .section_by_name(id.name())
                .and_then(|s| s.uncompressed_data().ok())
                .map(|c| match c {
                    std::borrow::Cow::Borrowed(b) => b,
                    std::borrow::Cow::Owned(_) => &[],
                })
                .unwrap_or(&[]);
            Ok(gimli::EndianSlice::new(bytes, endian))
        };
    let dwarf = gimli::Dwarf::load(load_sec).map_err(|e| format!("Failed loading DWARF: {e}"))?;

    let mut batch = DwarfBatch::default();

    // Global deduplication indexes
    // Named aggregates/enums/primitives/typedefs -> canonical type_id
    let mut named_type_ids: HashMap<&'static str, HashMap<String, i64>> = HashMap::new();
    // Structural modifier/pointer/array types: `(kind, target_type_id, array_count, containing_type_id)` -> canonical type_id
    let mut structural_type_ids: FnvHashMap<(&'static str, Option<i64>, Option<i64>, Option<i64>), i64> =
        FnvHashMap::default();

    // Raw layout & vtable specs per canonical `type_id` (populated once when concrete definition is seen)
    let mut type_populated_def: FnvHashSet<i64> = FnvHashSet::default();
    let mut raw_direct_members: FnvHashMap<i64, Vec<RawMemberSpec>> = FnvHashMap::default();
    let mut raw_direct_inherits: FnvHashMap<i64, Vec<(i64, Option<i64>, Option<i64>, bool)>> =
        FnvHashMap::default();
    let mut raw_vtable_decls: FnvHashMap<i64, Vec<(i64, String, Option<String>, bool, Option<i64>)>> =
        FnvHashMap::default();

    // Ensure `void` exists as type_id 1
    batch.types.push(TypeRow {
        type_id: 1,
        kind: "primitive",
        qualified_name: "void".to_string(),
        byte_size: Some(0),
        target_type_id: None,
        array_count: None,
        containing_type_id: None,
        vtable_symbol: None,
        vtable_address: None,
        is_declaration: false,
        is_non_trivial_for_calls: false,
        is_stl_stub: false,
        is_referenced: true,
    });
    named_type_ids
        .entry("primitive")
        .or_default()
        .insert("void".to_string(), 1);

    let mut next_unit_id: i64 = 1;
    let mut next_function_id: i64 = 1;
    let mut next_variable_id: i64 = 1;
    let mut next_inline_id: i64 = 1;

    // Linkage name -> concrete `function_id` and `(function_id, address)` for vtable binding
    let mut func_id_by_linkage: HashMap<String, i64> = HashMap::new();
    let mut func_id_by_address: FnvHashMap<i64, i64> = FnvHashMap::default();
    let mut pending_inline_linkages: Vec<(usize, String)> = Vec::new();

    let mut unit_headers = dwarf.units();
    while let Some(header) = unit_headers
        .next()
        .map_err(|e| format!("Failed reading CU header: {e}"))?
    {
        let unit_id = next_unit_id;
        next_unit_id += 1;
        let unit = dwarf
            .unit(header)
            .map_err(|e| format!("Failed parsing CU {unit_id}: {e}"))?;
        let uref = unit.unit_ref(&dwarf);

        let mut tree = uref
            .entries_tree(None)
            .map_err(|e| format!("Failed reading DIE tree in CU {unit_id}: {e}"))?;
        let root = tree
            .root()
            .map_err(|e| format!("Failed reading CU root in CU {unit_id}: {e}"))?;

        let mut cu_name = String::new();
        let mut comp_dir = String::new();
        let mut producer = String::new();
        let mut language: i64 = 0;

        for a in root.entry().attrs() {
            match a.name() {
                gimli::DW_AT_name => {
                    if let Ok(s) = uref.attr_string(a.value())
                        && let Ok(st) = s.to_string()
                    {
                        cu_name = st.to_string();
                    }
                }
                gimli::DW_AT_comp_dir => {
                    if let Ok(s) = uref.attr_string(a.value())
                        && let Ok(st) = s.to_string()
                    {
                        comp_dir = st.to_string();
                    }
                }
                gimli::DW_AT_producer => {
                    if let Ok(s) = uref.attr_string(a.value())
                        && let Ok(st) = s.to_string()
                    {
                        producer = st.to_string();
                    }
                }
                gimli::DW_AT_language => {
                    if let gimli::AttributeValue::Language(lang) = a.value() {
                        language = i64::from(lang.0);
                    }
                }
                _ => {}
            }
        }

        batch.units.push(UnitRow {
            unit_id,
            name: cu_name,
            comp_dir,
            producer,
            language,
        });

        let mut ns_stack = Vec::new();
        let mut cu_types: FnvHashMap<usize, CuType> = FnvHashMap::default();
        let mut cu_subprogs: FnvHashMap<usize, CuSubprog> = FnvHashMap::default();

        walk_cu_node(
            unit_id,
            &uref,
            root,
            &mut ns_stack,
            None,
            false,
            &mut cu_types,
            &mut cu_subprogs,
        );
        detect_copy_or_move_ctors(&mut cu_types, &cu_subprogs);

        // Map CU DIE offsets -> global canonical `type_id`
        let mut cu_to_global_type: FnvHashMap<usize, i64> =
            FnvHashMap::with_capacity_and_hasher(cu_types.len(), Default::default());

        fn intern_cu_type(
            off: usize,
            cu_types: &FnvHashMap<usize, CuType>,
            cu_to_global: &mut FnvHashMap<usize, i64>,
            named_ids: &mut HashMap<&'static str, HashMap<String, i64>>,
            structural_ids: &mut FnvHashMap<
                (&'static str, Option<i64>, Option<i64>, Option<i64>),
                i64,
            >,
            types_out: &mut Vec<TypeRow>,
            ztv_by_class: &HashMap<String, crate::vtable::ZtvSymbolInfo>,
            depth: usize,
        ) -> i64 {
            if let Some(&existing) = cu_to_global.get(&off) {
                return existing;
            }
            if depth > 48 {
                return 1;
            }
            let Some(ct) = cu_types.get(&off) else {
                return 1;
            };

            let is_named_kind = matches!(
                ct.kind,
                "struct" | "class" | "union" | "enum" | "primitive" | "typedef"
            );
            if is_named_kind {
                // Coalesce `struct` and `class` under a shared namespace key so forward declarations
                // (`struct Foo;`) and definitions (`class Foo { ... };`) merge to the same type_id.
                let dedup_kind = if ct.kind == "class" {
                    "struct"
                } else {
                    ct.kind
                };
                if let Some(&tid) = named_ids
                    .get(dedup_kind)
                    .and_then(|m| m.get(ct.qual_name.as_str()))
                {
                    cu_to_global.insert(off, tid);
                    let idx = (tid - 1) as usize;
                    if !ct.is_decl && types_out[idx].is_declaration {
                        types_out[idx].is_declaration = false;
                        types_out[idx].kind = ct.kind;
                        if ct.byte_size.is_some() {
                            types_out[idx].byte_size = ct.byte_size;
                        }
                    }
                    if ct.has_virtual_method
                        || ct.has_destructor
                        || ct.has_user_copy_or_move_ctor
                        || ct.byte_size.unwrap_or(0) > 16
                    {
                        types_out[idx].is_non_trivial_for_calls = true;
                    }
                    if types_out[idx].target_type_id.is_none()
                        && let Some(toff) = ct.target_off
                    {
                        let target_id = intern_cu_type(
                            toff,
                            cu_types,
                            cu_to_global,
                            named_ids,
                            structural_ids,
                            types_out,
                            ztv_by_class,
                            depth + 1,
                        );
                        types_out[idx].target_type_id = Some(target_id);
                    }
                    return tid;
                }

                let tid = (types_out.len() as i64) + 1;
                named_ids
                    .entry(dedup_kind)
                    .or_default()
                    .insert(ct.qual_name.clone(), tid);
                cu_to_global.insert(off, tid);

                let (vtable_symbol, vtable_address) = ztv_by_class
                    .get(&ct.qual_name)
                    .map(|z| (Some(z.symbol_name.clone()), Some(z.vtable_address as i64)))
                    .unwrap_or((None, None));

                let direct_non_trivial = ct.byte_size.unwrap_or(0) > 16
                    || ct.has_virtual_method
                    || ct.has_destructor
                    || ct.has_user_copy_or_move_ctor;
                let stl_stub = is_stl_stub_name(&ct.qual_name);

                types_out.push(TypeRow {
                    type_id: tid,
                    kind: ct.kind,
                    qualified_name: ct.qual_name.clone(),
                    byte_size: ct.byte_size,
                    target_type_id: None,
                    array_count: None,
                    containing_type_id: None,
                    vtable_symbol,
                    vtable_address,
                    is_declaration: ct.is_decl,
                    is_non_trivial_for_calls: direct_non_trivial,
                    is_stl_stub: stl_stub,
                    is_referenced: false,
                });

                if let Some(toff) = ct.target_off {
                    let target_id = intern_cu_type(
                        toff,
                        cu_types,
                        cu_to_global,
                        named_ids,
                        structural_ids,
                        types_out,
                        ztv_by_class,
                        depth + 1,
                    );
                    types_out[(tid - 1) as usize].target_type_id = Some(target_id);
                }
                if let Some(coff) = ct.containing_off {
                    let c_id = intern_cu_type(
                        coff,
                        cu_types,
                        cu_to_global,
                        named_ids,
                        structural_ids,
                        types_out,
                        ztv_by_class,
                        depth + 1,
                    );
                    types_out[(tid - 1) as usize].containing_type_id = Some(c_id);
                }
                tid
            } else {
                let target_id = ct.target_off.map(|toff| {
                    intern_cu_type(
                        toff,
                        cu_types,
                        cu_to_global,
                        named_ids,
                        structural_ids,
                        types_out,
                        ztv_by_class,
                        depth + 1,
                    )
                });
                let containing_id = ct.containing_off.map(|coff| {
                    intern_cu_type(
                        coff,
                        cu_types,
                        cu_to_global,
                        named_ids,
                        structural_ids,
                        types_out,
                        ztv_by_class,
                        depth + 1,
                    )
                });
                let key = (ct.kind, target_id, ct.array_count, containing_id);
                if let Some(&tid) = structural_ids.get(&key) {
                    cu_to_global.insert(off, tid);
                    return tid;
                }
                let tid = (types_out.len() as i64) + 1;
                structural_ids.insert(key, tid);
                cu_to_global.insert(off, tid);

                let target_name = target_id
                    .and_then(|id| types_out.get((id - 1) as usize))
                    .map(|t| t.qualified_name.as_str())
                    .unwrap_or("void");
                let qual_name = match ct.kind {
                    "pointer" => format!("{target_name} *"),
                    "reference" => format!("{target_name} &"),
                    "rvalue_reference" => format!("{target_name} &&"),
                    "const" => format!("const {target_name}"),
                    "volatile" => format!("volatile {target_name}"),
                    "array" => format!("{target_name}[{}]", ct.array_count.unwrap_or(1)),
                    _ => ct.qual_name.clone(),
                };
                types_out.push(TypeRow {
                    type_id: tid,
                    kind: ct.kind,
                    qualified_name: qual_name,
                    byte_size: ct.byte_size,
                    target_type_id: target_id,
                    array_count: ct.array_count,
                    containing_type_id: containing_id,
                    vtable_symbol: None,
                    vtable_address: None,
                    is_declaration: false,
                    is_non_trivial_for_calls: false,
                    is_stl_stub: false,
                    is_referenced: false,
                });
                tid
            }
        }

        let cu_type_offs: Vec<usize> = cu_types.keys().copied().collect();
        for off in &cu_type_offs {
            intern_cu_type(
                *off,
                &cu_types,
                &mut cu_to_global_type,
                &mut named_type_ids,
                &mut structural_type_ids,
                &mut batch.types,
                &vtable_index.ztv_by_class,
                0,
            );
        }

        // Record members, inheritances, enumerators, and vtable declarations for newly defined types
        for (&off, ct) in &cu_types {
            let Some(&tid) = cu_to_global_type.get(&off) else {
                continue;
            };
            if ct.is_decl || !type_populated_def.insert(tid) {
                continue;
            }
            if ct.kind == "enum" {
                for (ord, (ename, eval)) in ct.enumerators.iter().enumerate() {
                    batch.enumerators.push(EnumeratorRow {
                        enum_type_id: tid,
                        ordinal: ord as i64,
                        name: ename.clone(),
                        value: *eval,
                    });
                }
            } else if matches!(ct.kind, "struct" | "class" | "union") {
                let mut mspecs = Vec::new();
                for m in &ct.members {
                    if m.is_decl {
                        continue;
                    }
                    let mtid = m
                        .type_off
                        .and_then(|mo| cu_to_global_type.get(&mo).copied())
                        .unwrap_or(1);
                    let msize = m.byte_size.unwrap_or(1);
                    mspecs.push(RawMemberSpec {
                        name: m.name.clone(),
                        member_type_id: mtid,
                        byte_offset: m.byte_offset,
                        byte_size: msize,
                        bit_offset: m.bit_offset,
                        bit_size: m.bit_size,
                        is_artificial: m.is_artificial,
                    });
                }
                raw_direct_members.insert(tid, mspecs);

                let mut ispecs = Vec::new();
                for inh in &ct.inherits {
                    if let Some(bo) = inh.type_off
                        && let Some(&btid) = cu_to_global_type.get(&bo)
                    {
                        ispecs.push((
                            btid,
                            inh.byte_offset,
                            inh.vbase_vtable_offset,
                            inh.is_virtual,
                        ));
                    }
                }
                raw_direct_inherits.insert(tid, ispecs);

                if !ct.virtual_methods.is_empty() {
                    let vdecls = ct
                        .virtual_methods
                        .iter()
                        .map(|vm| {
                            (
                                vm.slot_index,
                                vm.name.clone(),
                                vm.linkage_name.clone(),
                                vm.is_pure_virtual,
                                None,
                            )
                        })
                        .collect();
                    raw_vtable_decls.insert(tid, vdecls);
                }
            }
        }

        // Process concrete functions in this CU (merging 2-hop `DW_AT_specification` and `DW_AT_abstract_origin`)
        let mut cu_subprog_to_func_id: FnvHashMap<usize, i64> = FnvHashMap::default();
        let mut emitted_concrete_subprogs: Vec<(usize, i64)> = Vec::new();
        let mut subprog_offs: Vec<usize> = cu_subprogs.keys().copied().collect();
        subprog_offs.sort_unstable();

        for &sp_off in &subprog_offs {
            let sp = &cu_subprogs[&sp_off];
            let Some(addr) = sp.low_pc else {
                continue;
            };
            if sp.is_decl {
                continue;
            }

            let spec1_off = sp.specification_off.or(sp.abstract_origin_off);
            let spec1 = spec1_off.and_then(|o| cu_subprogs.get(&o));
            let spec2_off = spec1.and_then(|s1| s1.specification_off.or(s1.abstract_origin_off));
            let spec2 = spec2_off.and_then(|o| cu_subprogs.get(&o));

            let linkage_name = sp
                .linkage_name
                .clone()
                .or_else(|| spec1.and_then(|s| s.linkage_name.clone()))
                .or_else(|| spec2.and_then(|s| s.linkage_name.clone()));

            if let Some(&existing_fid) = func_id_by_address.get(&addr) {
                cu_subprog_to_func_id.insert(sp_off, existing_fid);
                if let Some(o1) = spec1_off {
                    cu_subprog_to_func_id.entry(o1).or_insert(existing_fid);
                }
                if let Some(o2) = spec2_off {
                    cu_subprog_to_func_id.entry(o2).or_insert(existing_fid);
                }
                if let Some(ln) = &linkage_name {
                    func_id_by_linkage.entry(ln.clone()).or_insert(existing_fid);
                }
                continue;
            }

            let qual_name = spec2
                .map(|s| s.qual_name.clone())
                .or_else(|| {
                    spec1
                        .filter(|_s| {
                            sp.specification_off.is_some()
                                || sp.abstract_origin_off.is_some()
                                || sp.name.is_none()
                        })
                        .map(|s| s.qual_name.clone())
                })
                .unwrap_or_else(|| sp.qual_name.clone());
            let containing_off = sp
                .containing_type_off
                .or_else(|| spec1.and_then(|s| s.containing_type_off))
                .or_else(|| spec2.and_then(|s| s.containing_type_off));
            let containing_type_id =
                containing_off.and_then(|co| cu_to_global_type.get(&co).copied());
            let ret_off = sp
                .return_type_off
                .or_else(|| spec1.and_then(|s| s.return_type_off))
                .or_else(|| spec2.and_then(|s| s.return_type_off));
            let return_type_id = ret_off.and_then(|ro| cu_to_global_type.get(&ro).copied());
            let vtable_slot_index = sp
                .vtable_slot_index
                .or_else(|| spec1.and_then(|s| s.vtable_slot_index))
                .or_else(|| spec2.and_then(|s| s.vtable_slot_index));
            let virtuality = if sp.virtuality != 0 {
                sp.virtuality
            } else if let Some(s1) = spec1
                && s1.virtuality != 0
            {
                s1.virtuality
            } else {
                spec2.map(|s| s.virtuality).unwrap_or(0)
            };
            let is_variadic = sp.is_variadic
                || spec1.is_some_and(|s| s.is_variadic)
                || spec2.is_some_and(|s| s.is_variadic);
            let obj_ptr_off = sp
                .object_pointer_off
                .or_else(|| spec1.and_then(|s| s.object_pointer_off))
                .or_else(|| spec2.and_then(|s| s.object_pointer_off));

            let effective_params = if !sp.params.is_empty() {
                &sp.params
            } else if let Some(s1) = spec1
                && !s1.params.is_empty()
            {
                &s1.params
            } else if let Some(s2) = spec2 {
                &s2.params
            } else {
                &sp.params
            };

            let fid = next_function_id;
            next_function_id += 1;
            cu_subprog_to_func_id.insert(sp_off, fid);
            if let Some(o1) = spec1_off {
                cu_subprog_to_func_id.entry(o1).or_insert(fid);
            }
            if let Some(o2) = spec2_off {
                cu_subprog_to_func_id.entry(o2).or_insert(fid);
            }
            if let Some(ln) = &linkage_name {
                func_id_by_linkage.entry(ln.clone()).or_insert(fid);
            }
            func_id_by_address.insert(addr, fid);
            emitted_concrete_subprogs.push((sp_off, fid));

            let mut resolved_has_obj_ptr = obj_ptr_off.is_some();
            for (ord, p) in effective_params.iter().enumerate() {
                let fb1 = spec1.and_then(|s1| {
                    p.abstract_origin_off
                        .and_then(|ao| s1.params.iter().find(|cand| cand.die_off == ao))
                        .or_else(|| s1.params.get(ord))
                });
                let fb2 = spec2.and_then(|s2| {
                    p.abstract_origin_off
                        .or_else(|| fb1.and_then(|f| f.abstract_origin_off))
                        .and_then(|ao| s2.params.iter().find(|cand| cand.die_off == ao))
                        .or_else(|| s2.params.get(ord))
                });
                let pname = p
                    .name
                    .clone()
                    .or_else(|| fb1.and_then(|f| f.name.clone()))
                    .or_else(|| fb2.and_then(|f| f.name.clone()));
                let ptype_off = p
                    .type_off
                    .or_else(|| fb1.and_then(|f| f.type_off))
                    .or_else(|| fb2.and_then(|f| f.type_off));
                let part = p.is_artificial
                    || fb1.is_some_and(|f| f.is_artificial)
                    || fb2.is_some_and(|f| f.is_artificial);
                if ord == 0
                    && containing_type_id.is_some()
                    && (part || pname.as_deref() == Some("this"))
                {
                    resolved_has_obj_ptr = true;
                }
                let ptid = ptype_off
                    .and_then(|po| cu_to_global_type.get(&po).copied())
                    .unwrap_or(1);
                let matches_obj_ptr_die = obj_ptr_off.is_some_and(|op| {
                    op == p.die_off
                        || p.abstract_origin_off == Some(op)
                        || fb1.is_some_and(|f| f.die_off == op)
                        || fb2.is_some_and(|f| f.die_off == op)
                });
                let is_obj_ptr = matches_obj_ptr_die
                    || (ord == 0
                        && resolved_has_obj_ptr
                        && (part || pname.as_deref() == Some("this")));
                batch.parameters.push(ParameterRow {
                    function_id: fid,
                    ordinal: ord as i64,
                    name: pname,
                    type_id: ptid,
                    is_artificial: part || is_obj_ptr,
                    is_object_pointer: is_obj_ptr,
                    storage_kind: p.storage_kind,
                    dwarf_reg: p.dwarf_reg,
                    reg_name: p.reg_name.clone(),
                    stack_offset: p.stack_offset,
                });
            }

            let param0_in_rsi = effective_params
                .first()
                .and_then(|p| p.dwarf_reg)
                == Some(4);

            batch.functions.push(FunctionRow {
                function_id: fid,
                unit_id,
                address: Some(addr),
                byte_size: sp.byte_size,
                linkage_name,
                qualified_name: qual_name,
                containing_type_id,
                return_type_id,
                vtable_slot_index,
                virtuality,
                is_inline: sp.is_inline
                    || spec1.is_some_and(|s| s.is_inline)
                    || spec2.is_some_and(|s| s.is_inline),
                is_declaration: false,
                is_variadic,
                has_object_pointer: resolved_has_obj_ptr,
                is_sret: param0_in_rsi,
                decl_file: sp
                    .decl_file
                    .clone()
                    .or_else(|| spec1.and_then(|s| s.decl_file.clone()))
                    .or_else(|| spec2.and_then(|s| s.decl_file.clone())),
                decl_line: sp
                    .decl_line
                    .or_else(|| spec1.and_then(|s| s.decl_line))
                    .or_else(|| spec2.and_then(|s| s.decl_line)),
            });

            for vr in &sp.variables {
                let vtid = vr
                    .type_off
                    .and_then(|vo| cu_to_global_type.get(&vo).copied())
                    .unwrap_or(1);
                let vid = next_variable_id;
                next_variable_id += 1;
                batch.variables.push(VariableRow {
                    variable_id: vid,
                    function_id: fid,
                    name: vr.name.clone(),
                    type_id: vtid,
                    range_index: vr.range_index,
                    start_address: vr.start_address,
                    end_address: vr.end_address,
                    first_use_offset: (vr.start_address - addr).max(0),
                    storage_kind: vr.storage_kind,
                    reg_name: vr.reg_name.clone(),
                    stack_offset: vr.stack_offset,
                    const_value: vr.const_value,
                });
            }
        }

        // Second subprogram pass inside CU to resolve `dwarf_inlines` for newly emitted concrete functions
        for &(sp_off, caller_fid) in &emitted_concrete_subprogs {
            let sp = &cu_subprogs[&sp_off];
            if sp.inlines.is_empty() {
                continue;
            }
            for inl in &sp.inlines {
                let (callee_fid, callee_name, callee_linkage) = inl
                    .abstract_origin
                    .and_then(|ao| {
                        let s1 = cu_subprogs.get(&ao)?;
                        let s2_off = s1.specification_off.or(s1.abstract_origin_off);
                        let s2 = s2_off.and_then(|o| cu_subprogs.get(&o));
                        let cfid = cu_subprog_to_func_id
                            .get(&ao)
                            .copied()
                            .or_else(|| s2_off.and_then(|o| cu_subprog_to_func_id.get(&o).copied()));
                        let cname = s2
                            .map(|spec| spec.qual_name.clone())
                            .unwrap_or_else(|| s1.qual_name.clone());
                        let clink = s1
                            .linkage_name
                            .clone()
                            .or_else(|| s2.and_then(|spec| spec.linkage_name.clone()));
                        Some((cfid, cname, clink))
                    })
                    .unwrap_or_else(|| {
                        (
                            None,
                            inl.callee_name
                                .clone()
                                .unwrap_or_else(|| "<inlined>".to_string()),
                            None,
                        )
                    });

                let iid = next_inline_id;
                next_inline_id += 1;
                let inl_idx = batch.inlines.len();
                if callee_fid.is_none()
                    && let Some(ln) = callee_linkage
                {
                    pending_inline_linkages.push((inl_idx, ln));
                }
                batch.inlines.push(InlineRow {
                    inline_id: iid,
                    caller_function_id: caller_fid,
                    callee_function_id: callee_fid,
                    callee_name,
                    start_address: inl.start_address,
                    end_address: inl.end_address,
                    call_file: inl.call_file.clone(),
                    call_line: inl.call_line,
                });
            }
        }
    }

    for (inl_idx, ln) in pending_inline_linkages {
        if batch.inlines[inl_idx].callee_function_id.is_none() {
            batch.inlines[inl_idx].callee_function_id = func_id_by_linkage.get(&ln).copied();
        }
    }

    // Resolve concrete member byte sizes and propagate `is_non_trivial_for_calls` across the global type graph
    let resolve_global_underlying = |types: &[TypeRow], mut tid: i64| -> i64 {
        for _ in 0..12 {
            let Some(t) = types.get((tid - 1) as usize) else {
                break;
            };
            if matches!(t.kind, "typedef" | "const" | "volatile")
                && let Some(target) = t.target_type_id
            {
                tid = target;
            } else {
                break;
            }
        }
        tid
    };

    let type_byte_size = |types: &[TypeRow], mut tid: i64| -> i64 {
        let mut mul: i64 = 1;
        for _ in 0..12 {
            let Some(t) = types.get((tid - 1) as usize) else {
                break;
            };
            if t.kind == "array" {
                mul = mul.saturating_mul(t.array_count.unwrap_or(1).max(1));
                if let Some(target) = t.target_type_id {
                    tid = target;
                    continue;
                }
            }
            if let Some(sz) = t.byte_size
                && sz > 0
            {
                return sz.saturating_mul(mul);
            }
            if let Some(target) = t.target_type_id {
                tid = target;
            } else {
                break;
            }
        }
        mul.max(1)
    };

    for mvec in raw_direct_members.values_mut() {
        for m in mvec.iter_mut() {
            if m.byte_size <= 1 {
                m.byte_size = type_byte_size(&batch.types, m.member_type_id);
            }
        }
    }

    // Fixed-point propagation of `is_non_trivial_for_calls` across bases and members
    for _ in 0..6 {
        let mut changed = false;
        for tid in 1..=(batch.types.len() as i64) {
            let idx = (tid - 1) as usize;
            if batch.types[idx].is_non_trivial_for_calls
                || !matches!(batch.types[idx].kind, "struct" | "class" | "union")
            {
                continue;
            }
            let base_nt = raw_direct_inherits.get(&tid).is_some_and(|inhs| {
                inhs.iter().any(|&(btid, _, _, is_virt)| {
                    is_virt || {
                        let u = resolve_global_underlying(&batch.types, btid);
                        batch
                            .types
                            .get((u - 1) as usize)
                            .is_some_and(|t| t.is_non_trivial_for_calls)
                    }
                })
            });
            let mem_nt = raw_direct_members.get(&tid).is_some_and(|mems| {
                mems.iter().any(|m| {
                    let u = resolve_global_underlying(&batch.types, m.member_type_id);
                    batch
                        .types
                        .get((u - 1) as usize)
                        .is_some_and(|t| t.is_non_trivial_for_calls)
                })
            });
            if base_nt || mem_nt {
                batch.types[idx].is_non_trivial_for_calls = true;
                changed = true;
            }
        }
        if !changed {
            break;
        }
    }

    // Finalize `dwarf_functions.is_sret` using both `param[0] == RSI` and return type `is_non_trivial_for_calls`
    for f in &mut batch.functions {
        let Some(rt_id) = f.return_type_id else {
            f.is_sret = false;
            continue;
        };
        let u_rt = resolve_global_underlying(&batch.types, rt_id);
        let Some(rt) = batch.types.get((u_rt - 1) as usize) else {
            f.is_sret = false;
            continue;
        };
        if matches!(rt.kind, "struct" | "class" | "union") {
            if rt.is_non_trivial_for_calls || rt.byte_size.unwrap_or(0) > 16 {
                f.is_sret = true;
            }
        } else {
            f.is_sret = false;
        }
    }

    // Pass 3: Resolve pre-flattened struct layouts and synthesize `<Class>_vtbl` entries
    let mut next_member_id: i64 = 1;
    let mut next_inherit_id: i64 = 1;

    let mut all_struct_tids: Vec<i64> = raw_direct_members
        .keys()
        .chain(raw_direct_inherits.keys())
        .copied()
        .collect::<HashSet<_>>()
        .into_iter()
        .collect();
    all_struct_tids.sort_unstable();

    for tid in all_struct_tids {
        let idx = (tid - 1) as usize;
        let is_union = batch.types[idx].kind == "union";
        let parent_size = batch.types[idx].byte_size.unwrap_or(0);
        let d_members = raw_direct_members.get(&tid).cloned().unwrap_or_default();
        let d_inh_raw = raw_direct_inherits.get(&tid).cloned().unwrap_or_default();

        let mut d_inherits = Vec::with_capacity(d_inh_raw.len());
        for (btid, boff, vboff, is_virt) in d_inh_raw {
            let u_base = resolve_global_underlying(&batch.types, btid);
            let bsize = batch
                .types
                .get((u_base - 1) as usize)
                .and_then(|t| t.byte_size)
                .unwrap_or(1);
            let bmems = raw_direct_members.get(&u_base).cloned().unwrap_or_default();
            let b_inhs = raw_direct_inherits.get(&u_base).map_or(0, Vec::len);
            let b_empty = bsize == 1 && bmems.is_empty() && b_inhs == 0;
            d_inherits.push(RawInheritSpec {
                base_type_id: u_base,
                base_byte_size: bsize,
                base_is_empty: b_empty,
                byte_offset: boff,
                vbase_vtable_offset: vboff,
                is_virtual: is_virt,
                base_members: bmems,
            });
        }

        let (m_rows, i_rows) = resolve_struct_layout(
            tid,
            is_union,
            parent_size,
            &d_members,
            &d_inherits,
            &mut next_member_id,
            &mut next_inherit_id,
        );
        batch.members.extend(m_rows);
        batch.inheritances.extend(i_rows);
    }

    // Synthesize `dwarf_vtable_entries` (inheriting primary base vtable slots + `_ZTV` D1Ev/D0Ev)
    let resolve_class_vtable_slots = |tid: i64,
                                      raw_vtable_decls: &FnvHashMap<
        i64,
        Vec<(i64, String, Option<String>, bool, Option<i64>)>,
    >,
                                      raw_direct_inherits: &FnvHashMap<
        i64,
        Vec<(i64, Option<i64>, Option<i64>, bool)>,
    >,
                                      types: &[TypeRow]|
     -> Vec<(i64, String, Option<String>, bool)> {
        let mut chain = Vec::new();
        let mut cur = Some(tid);
        for _ in 0..16 {
            let Some(c) = cur else { break };
            chain.push(c);
            let primary_base = raw_direct_inherits.get(&c).and_then(|inhs| {
                inhs.iter()
                    .find(|&&(_, boff, _, is_virt)| !is_virt && boff == Some(0))
                    .map(|&(btid, _, _, _)| resolve_global_underlying(types, btid))
            });
            if primary_base.is_some_and(|pb| !chain.contains(&pb)) {
                cur = primary_base;
            } else {
                break;
            }
        }
        chain.reverse();
        let mut slots: HashMap<i64, (String, Option<String>, bool)> = HashMap::new();
        for c in chain {
            if let Some(decls) = raw_vtable_decls.get(&c) {
                for (slot, mname, lname, is_pv, _) in decls {
                    slots.insert(*slot, (mname.clone(), lname.clone(), *is_pv));
                }
            }
        }
        let mut out: Vec<_> = slots
            .into_iter()
            .map(|(s, (m, l, pv))| (s, m, l, pv))
            .collect();
        out.sort_by_key(|x| x.0);
        out
    };

    let mut poly_tids: Vec<i64> = (1..=(batch.types.len() as i64))
        .filter(|&tid| {
            raw_vtable_decls.contains_key(&tid)
                || batch.types[(tid - 1) as usize].vtable_address.is_some()
        })
        .collect();
    poly_tids.sort_unstable();

    for tid in poly_tids {
        let qn = &batch.types[(tid - 1) as usize].qualified_name;
        let ztv_info = vtable_index.ztv_by_class.get(qn);
        let decl_slots =
            resolve_class_vtable_slots(tid, &raw_vtable_decls, &raw_direct_inherits, &batch.types);

        let mut slot_map: HashMap<i64, VtableEntryRow> = HashMap::new();
        for (slot_idx, mname, lname, is_pv) in decl_slots {
            let target_addr = ztv_info
                .and_then(|z| z.slot_targets.get(slot_idx as usize).copied())
                .filter(|&a| a != 0);
            let is_pure = is_pv
                || target_addr.is_some_and(|a| vtable_index.pure_virtual_addrs.contains(&a));
            let fid = lname
                .as_ref()
                .and_then(|l| func_id_by_linkage.get(l).copied())
                .or_else(|| target_addr.and_then(|a| func_id_by_address.get(&(a as i64)).copied()));

            slot_map.insert(
                slot_idx,
                VtableEntryRow {
                    class_type_id: tid,
                    slot_index: slot_idx,
                    method_name: mname.clone(),
                    linkage_name: lname,
                    function_id: fid,
                    target_address: target_addr.map(|a| a as i64),
                    is_pure_virtual: is_pure,
                },
            );

            // Itanium dual virtual destructor slot (`D1Ev` at `slot_idx`, `D0Ev` deleting dtor at `slot_idx + 1`)
            if mname.starts_with('~') {
                let d0_slot = slot_idx + 1;
                let d0_target = ztv_info
                    .and_then(|z| z.slot_targets.get(d0_slot as usize).copied())
                    .filter(|&a| a != 0);
                let d0_sym = d0_target
                    .and_then(|a| vtable_index.func_sym_by_addr.get(&a).cloned());
                let d0_fid = d0_sym
                    .as_ref()
                    .and_then(|l| func_id_by_linkage.get(l).copied())
                    .or_else(|| {
                        d0_target.and_then(|a| func_id_by_address.get(&(a as i64)).copied())
                    });
                slot_map.entry(d0_slot).or_insert(VtableEntryRow {
                    class_type_id: tid,
                    slot_index: d0_slot,
                    method_name: format!("{mname}_deleting"),
                    linkage_name: d0_sym,
                    function_id: d0_fid,
                    target_address: d0_target.map(|a| a as i64),
                    is_pure_virtual: is_pure,
                });
            }
        }

        if let Some(z) = ztv_info {
            for (slot_idx, &target_va) in z.slot_targets.iter().enumerate() {
                let s = slot_idx as i64;
                if slot_map.contains_key(&s) || target_va == 0 {
                    continue;
                }
                let sym = vtable_index.func_sym_by_addr.get(&target_va).cloned();
                let is_pure = vtable_index.pure_virtual_addrs.contains(&target_va);
                let fid = sym
                    .as_ref()
                    .and_then(|l| func_id_by_linkage.get(l).copied())
                    .or_else(|| func_id_by_address.get(&(target_va as i64)).copied());
                let mname = fid
                    .and_then(|id| batch.functions.get((id - 1) as usize))
                    .map(|f| f.qualified_name.clone())
                    .or_else(|| sym.clone())
                    .unwrap_or_else(|| format!("vfunc_slot_{s}"));
                slot_map.insert(
                    s,
                    VtableEntryRow {
                        class_type_id: tid,
                        slot_index: s,
                        method_name: mname,
                        linkage_name: sym,
                        function_id: fid,
                        target_address: Some(target_va as i64),
                        is_pure_virtual: is_pure,
                    },
                );
            }
        }

        let mut rows: Vec<_> = slot_map.into_values().collect();
        rows.sort_by_key(|r| r.slot_index);
        batch.vtable_entries.extend(rows);
    }

    // Mark transitive `is_referenced` on `dwarf_types`
    let mut adj: HashMap<i64, Vec<i64>> = HashMap::new();
    for m in &batch.members {
        if !m.is_layout_conflict {
            adj.entry(m.parent_type_id)
                .or_default()
                .push(m.member_type_id);
        }
    }
    for i in &batch.inheritances {
        if !i.is_virtual && !i.is_empty_base && !i.has_tail_padding_overlap {
            adj.entry(i.derived_type_id)
                .or_default()
                .push(i.base_type_id);
        }
    }

    let mut queue: VecDeque<i64> = VecDeque::new();
    let enqueue = |tid: i64, types: &mut [TypeRow], q: &mut VecDeque<i64>| {
        if tid >= 1
            && (tid as usize) <= types.len()
            && !types[(tid - 1) as usize].is_referenced
        {
            types[(tid - 1) as usize].is_referenced = true;
            q.push_back(tid);
        }
    };

    for f in &batch.functions {
        if f.address.is_some() && !f.is_declaration {
            if let Some(rt) = f.return_type_id {
                enqueue(rt, &mut batch.types, &mut queue);
            }
            if let Some(ct) = f.containing_type_id {
                enqueue(ct, &mut batch.types, &mut queue);
            }
        }
    }
    for p in &batch.parameters {
        enqueue(p.type_id, &mut batch.types, &mut queue);
    }
    for v in &batch.vtable_entries {
        enqueue(v.class_type_id, &mut batch.types, &mut queue);
    }
    for var in &batch.variables {
        enqueue(var.type_id, &mut batch.types, &mut queue);
    }

    while let Some(tid) = queue.pop_front() {
        let idx = (tid - 1) as usize;
        if let Some(target) = batch.types[idx].target_type_id {
            enqueue(target, &mut batch.types, &mut queue);
        }
        if let Some(cont) = batch.types[idx].containing_type_id {
            enqueue(cont, &mut batch.types, &mut queue);
        }
        if !batch.types[idx].is_stl_stub
            && let Some(children) = adj.get(&tid)
        {
            for &child in children {
                enqueue(child, &mut batch.types, &mut queue);
            }
        }
    }

    Ok(batch)
}
