//! ELF `.symtab` `_ZTV*` vtable scanner and Itanium C++ vtable name parser.

use std::collections::{HashMap, HashSet};
use object::{Object, ObjectSection, ObjectSymbol, SymbolKind};

#[derive(Clone, Debug)]
pub struct ZtvSymbolInfo {
    pub symbol_name: String,
    pub vtable_address: u64,
    pub slot_targets: Vec<u64>,
}

pub struct ElfVtableIndex {
    pub ztv_by_class: HashMap<String, ZtvSymbolInfo>,
    pub pure_virtual_addrs: HashSet<u64>,
    pub func_sym_by_addr: HashMap<u64, String>,
    pub data_sym_by_name: HashMap<String, u64>,
    pub ctor_sym_names: HashSet<String>,
}

fn is_indexable_data_symbol(name: &str, addr: u64, kind: SymbolKind) -> bool {
    addr != 0
        && !name.is_empty()
        && matches!(kind, SymbolKind::Data | SymbolKind::Unknown)
        && !name.starts_with("_ZTV")
        && !name.starts_with("_ZTI")
}

fn is_itanium_ctor_symbol(name: &str, addr: u64, kind: SymbolKind) -> bool {
    addr != 0
        && kind == SymbolKind::Text
        && name.starts_with("_Z")
        && (name.contains("C1E") || name.contains("C2E"))
}

fn parse_len_ident(s: &str) -> Option<(String, &str)> {
    let mut idx = 0;
    for (i, ch) in s.char_indices() {
        if ch.is_ascii_digit() {
            idx = i + 1;
        } else {
            break;
        }
    }
    if idx == 0 {
        return None;
    }
    let len: usize = s[..idx].parse().ok()?;
    let after = &s[idx..];
    if after.len() < len {
        return None;
    }
    Some((after[..len].to_string(), &after[len..]))
}

pub fn parse_itanium_ztv_name(sym: &str) -> Option<String> {
    let rest = sym.strip_prefix("_ZTV")?;
    if let Some(mut inner) = rest.strip_prefix('N') {
        let mut parts = Vec::new();
        while !inner.is_empty() && !inner.starts_with('E') {
            if let Some(rem) = inner.strip_prefix("St") {
                parts.push("std".to_string());
                inner = rem;
                continue;
            }
            if let Some((ident, rem)) = parse_len_ident(inner) {
                parts.push(ident);
                inner = rem;
            } else {
                return None;
            }
        }
        if inner == "E" && !parts.is_empty() {
            return Some(parts.join("::"));
        }
        None
    } else {
        let (ident, rem) = parse_len_ident(rest)?;
        if rem.is_empty() {
            Some(ident)
        } else {
            None
        }
    }
}

pub fn build_elf_vtable_index(obj: &object::File<'_>) -> ElfVtableIndex {
    let mut ztv_raw: Vec<(String, String, u64, u64)> = Vec::new();
    let mut rtti_addrs: HashSet<u64> = HashSet::new();
    let mut pure_virtual_addrs: HashSet<u64> = HashSet::new();
    let mut func_sym_by_addr: HashMap<u64, String> = HashMap::new();
    let mut data_sym_by_name: HashMap<String, u64> = HashMap::new();
    let mut ctor_sym_names: HashSet<String> = HashSet::new();

    for sym in obj.symbols() {
        let Ok(name) = sym.name() else { continue };
        let addr = sym.address();
        if name.starts_with("_ZTV") && addr != 0 && sym.size() >= 24 {
            if let Some(qn) = parse_itanium_ztv_name(name) {
                ztv_raw.push((qn, name.to_string(), addr, sym.size()));
            }
        } else if name.starts_with("_ZTI") && addr != 0 {
            rtti_addrs.insert(addr);
        } else if name.starts_with("__cxa_pure_virtual") && addr != 0 {
            pure_virtual_addrs.insert(addr);
        } else if addr != 0 && !name.is_empty() && sym.kind() == SymbolKind::Text {
            func_sym_by_addr
                .entry(addr)
                .or_insert_with(|| name.to_string());
        }

        if !sym.is_undefined() {
            if is_itanium_ctor_symbol(name, addr, sym.kind()) {
                ctor_sym_names.insert(name.to_string());
            }
            if is_indexable_data_symbol(name, addr, sym.kind()) {
                data_sym_by_name.entry(name.to_string()).or_insert(addr);
            }
        }
    }
    for sym in obj.dynamic_symbols() {
        if let Ok(name) = sym.name()
            && name.starts_with("__cxa_pure_virtual")
            && sym.address() != 0
        {
            pure_virtual_addrs.insert(sym.address());
        }
    }

    let sections: Vec<(u64, u64, &[u8])> = obj
        .sections()
        .filter_map(|sec| {
            let addr = sec.address();
            let size = sec.size();
            if addr == 0 || size < 8 {
                return None;
            }
            let data = sec.data().ok()?;
            Some((addr, size, data))
        })
        .collect();

    let read_u64 = |va: u64| -> Option<u64> {
        for &(saddr, ssize, data) in &sections {
            if va >= saddr && va + 8 <= saddr + ssize {
                let off = (va - saddr) as usize;
                let bytes: [u8; 8] = data[off..off + 8].try_into().ok()?;
                return Some(u64::from_le_bytes(bytes));
            }
        }
        None
    };

    let mut ztv_by_class = HashMap::with_capacity(ztv_raw.len());
    for (qn, sym_name, sym_addr, sym_size) in ztv_raw {
        let vptr_addr = sym_addr + 0x10;
        let max_words = ((sym_size - 0x10) / 8) as usize;
        let mut slot_targets = Vec::with_capacity(max_words);
        for slot in 0..max_words {
            let entry_va = vptr_addr + (slot as u64) * 8;
            let Some(val) = read_u64(entry_va) else {
                break;
            };
            // Under multiple inheritance, a secondary vtable header inside `_ZTV` begins with
            // a negative `offset_to_top` (or 0) followed immediately by the `_ZTI*` RTTI pointer.
            if slot + 1 < max_words
                && (val == 0 || (val as i64) < 0 && (val as i64) > -0x10_0000)
                && let Some(next_val) = read_u64(entry_va + 8)
                && (rtti_addrs.contains(&next_val) || next_val == 0)
            {
                break;
            }
            slot_targets.push(val);
        }
        ztv_by_class.insert(
            qn,
            ZtvSymbolInfo {
                symbol_name: sym_name,
                vtable_address: vptr_addr,
                slot_targets,
            },
        );
    }

    ElfVtableIndex {
        ztv_by_class,
        pure_virtual_addrs,
        func_sym_by_addr,
        data_sym_by_name,
        ctor_sym_names,
    }
}

#[cfg(test)]
mod tests {
    use super::{is_indexable_data_symbol, is_itanium_ctor_symbol, parse_itanium_ztv_name};
    use object::SymbolKind;

    #[test]
    fn test_parse_itanium_ztv_name() {
        assert_eq!(
            parse_itanium_ztv_name("_ZTVN12EMPIREBATTLE17VICTORY_CONDITIONE").as_deref(),
            Some("EMPIREBATTLE::VICTORY_CONDITION")
        );
        assert_eq!(
            parse_itanium_ztv_name("_ZTV4UNIT").as_deref(),
            Some("UNIT")
        );
        assert_eq!(
            parse_itanium_ztv_name("_ZTVNSt6locale5facetE").as_deref(),
            Some("std::locale::facet")
        );
    }

    #[test]
    fn test_data_and_ctor_symbol_filtering() {
        assert!(is_indexable_data_symbol(
            "_ZN17EMPIRE_APP_MODULE10s_instanceE",
            0x57d0a0,
            SymbolKind::Data
        ));
        assert!(is_indexable_data_symbol(
            "g_unknown_kind_data",
            0x3095a40,
            SymbolKind::Unknown
        ));
        assert!(!is_indexable_data_symbol(
            "_ZTV4UNIT",
            0x37a2fb0,
            SymbolKind::Data
        ));
        assert!(!is_indexable_data_symbol(
            "_ZTI4UNIT",
            0x37a2fc0,
            SymbolKind::Data
        ));
        assert!(!is_indexable_data_symbol(
            "g_zero",
            0,
            SymbolKind::Data
        ));
        assert!(!is_indexable_data_symbol("", 0x1000, SymbolKind::Data));
        assert!(!is_indexable_data_symbol(
            "_ZN4UNIT3fooEv",
            0x401000,
            SymbolKind::Text
        ));

        assert!(is_itanium_ctor_symbol(
            "_ZN15TEMPLATE_HANDLEC1ERKS_",
            0x402000,
            SymbolKind::Text
        ));
        assert!(is_itanium_ctor_symbol(
            "_ZN15TEMPLATE_HANDLEC2EOS_",
            0x402100,
            SymbolKind::Text
        ));
        assert!(!is_itanium_ctor_symbol(
            "_ZN4UNIT3fooEv",
            0x401000,
            SymbolKind::Text
        ));
        assert!(!is_itanium_ctor_symbol(
            "_ZN15TEMPLATE_HANDLEC1ERKS_",
            0,
            SymbolKind::Text
        ));
        assert!(!is_itanium_ctor_symbol(
            "_ZN15TEMPLATE_HANDLEC1ERKS_",
            0x402000,
            SymbolKind::Data
        ));
    }
}
