//! Itanium C++ ABI struct layout flattening (EBO, tail-padding, bitfield/union conflicts)
//! and STL stub / calling-convention non-triviality helpers.

use crate::db::{InheritanceRow, MemberRow};

pub fn is_stl_stub_name(qualified_name: &str) -> bool {
    qualified_name.starts_with("std::__detail::")
        || qualified_name.starts_with("__gnu_cxx::")
}

#[derive(Clone, Debug)]
pub struct RawMemberSpec {
    pub name: String,
    pub member_type_id: i64,
    pub byte_offset: i64,
    pub byte_size: i64,
    pub bit_offset: Option<i64>,
    pub bit_size: Option<i64>,
    pub is_artificial: bool,
}

#[derive(Clone, Debug)]
pub struct RawInheritSpec {
    pub base_type_id: i64,
    pub base_byte_size: i64,
    pub base_is_empty: bool,
    pub byte_offset: Option<i64>,
    pub vbase_vtable_offset: Option<i64>,
    pub is_virtual: bool,
    pub base_members: Vec<RawMemberSpec>,
}

pub fn resolve_struct_layout(
    parent_type_id: i64,
    parent_is_union: bool,
    parent_byte_size: i64,
    direct_members: &[RawMemberSpec],
    direct_inherits: &[RawInheritSpec],
    next_member_id: &mut i64,
    next_inherit_id: &mut i64,
) -> (Vec<MemberRow>, Vec<InheritanceRow>) {
    let mut out_inherits = Vec::with_capacity(direct_inherits.len());
    let mut candidate_members: Vec<(RawMemberSpec, bool)> = Vec::new();

    for (inh_idx, inh) in direct_inherits.iter().enumerate() {
        let inherit_id = *next_inherit_id;
        *next_inherit_id += 1;

        if inh.is_virtual {
            out_inherits.push(InheritanceRow {
                inheritance_id: inherit_id,
                derived_type_id: parent_type_id,
                base_type_id: inh.base_type_id,
                byte_offset: None,
                vbase_vtable_offset: inh.vbase_vtable_offset,
                is_virtual: true,
                is_empty_base: false,
                has_tail_padding_overlap: false,
            });
            continue;
        }

        let base_off = inh.byte_offset.unwrap_or(0);
        let base_end = base_off + inh.base_byte_size.max(1);
        let overlaps_member_at_same_offset = direct_members
            .iter()
            .any(|m| m.byte_offset == base_off);
        let overlaps_sibling_base_at_same_offset = direct_inherits
            .iter()
            .enumerate()
            .any(|(other_idx, other)| {
                other_idx != inh_idx && !other.is_virtual && other.byte_offset == Some(base_off)
            });

        let is_empty_base = inh.base_is_empty
            && (overlaps_member_at_same_offset || overlaps_sibling_base_at_same_offset);

        let has_tail_padding_overlap = !is_empty_base
            && inh.base_byte_size > 1
            && direct_members.iter().any(|m| {
                m.byte_offset >= base_off && m.byte_offset < base_end
            });

        if has_tail_padding_overlap {
            for bm in &inh.base_members {
                let mut promoted = bm.clone();
                promoted.byte_offset += base_off;
                candidate_members.push((promoted, true));
            }
        }

        out_inherits.push(InheritanceRow {
            inheritance_id: inherit_id,
            derived_type_id: parent_type_id,
            base_type_id: inh.base_type_id,
            byte_offset: Some(base_off),
            vbase_vtable_offset: None,
            is_virtual: false,
            is_empty_base,
            has_tail_padding_overlap,
        });
    }

    for dm in direct_members {
        candidate_members.push((dm.clone(), false));
    }

    // Track occupied bit intervals `[start_bit, end_bit)` from valid non-overlapping bases first.
    let mut occupied_bits: Vec<(i64, i64)> = out_inherits
        .iter()
        .filter(|i| !i.is_virtual && !i.is_empty_base && !i.has_tail_padding_overlap)
        .filter_map(|i| {
            let start = i.byte_offset?;
            let bspec = direct_inherits
                .iter()
                .find(|d| d.base_type_id == i.base_type_id && d.byte_offset == Some(start))?;
            let len = bspec.base_byte_size.max(1);
            if start >= 0 && (parent_byte_size <= 0 || start + len <= parent_byte_size) {
                Some((start * 8, (start + len) * 8))
            } else {
                None
            }
        })
        .collect();

    let mut out_members = Vec::with_capacity(candidate_members.len());
    for (idx, (m, is_flattened)) in candidate_members.into_iter().enumerate() {
        let member_id = *next_member_id;
        *next_member_id += 1;

        let len = m.byte_size.max(1);
        let start = m.byte_offset;
        let (bit_start, bit_end, out_of_bounds, invalid_zero_bitfield) = match m.bit_size {
            Some(bs) if bs <= 0 => (start * 8, start * 8, false, true),
            Some(bs) => {
                let bo = m.bit_offset.unwrap_or(0);
                let b_start = start * 8 + bo;
                let b_end = b_start + bs;
                let oob = b_start < 0 || (parent_byte_size > 0 && b_end > parent_byte_size * 8);
                (b_start, b_end, oob, false)
            }
            None => {
                let end = start + len;
                let oob = start < 0 || (parent_byte_size > 0 && end > parent_byte_size);
                (start * 8, end * 8, oob, false)
            }
        };
        let interval_overlap = if parent_is_union {
            false
        } else {
            occupied_bits
                .iter()
                .any(|&(os, oe)| bit_start < oe && os < bit_end)
        };
        let is_conflict = out_of_bounds || invalid_zero_bitfield || interval_overlap;
        if !is_conflict && !parent_is_union {
            occupied_bits.push((bit_start, bit_end));
        }

        out_members.push(MemberRow {
            member_id,
            parent_type_id,
            ordinal: idx as i64,
            name: m.name,
            member_type_id: m.member_type_id,
            byte_offset: start,
            byte_size: len,
            bit_offset: m.bit_offset,
            bit_size: m.bit_size,
            is_artificial: m.is_artificial,
            is_flattened_base_member: is_flattened,
            is_layout_conflict: is_conflict,
        });
    }

    (out_members, out_inherits)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ebo_and_tail_padding_resolution() {
        let base_members = vec![RawMemberSpec {
            name: "m_base_val".to_string(),
            member_type_id: 2,
            byte_offset: 0,
            byte_size: 5,
            bit_offset: None,
            bit_size: None,
            is_artificial: false,
        }];
        let inherits = vec![
            RawInheritSpec {
                base_type_id: 10,
                base_byte_size: 1,
                base_is_empty: true,
                byte_offset: Some(0),
                vbase_vtable_offset: None,
                is_virtual: false,
                base_members: Vec::new(),
            },
            RawInheritSpec {
                base_type_id: 11,
                base_byte_size: 8,
                base_is_empty: false,
                byte_offset: Some(0),
                vbase_vtable_offset: None,
                is_virtual: false,
                base_members,
            },
        ];
        let direct_members = vec![RawMemberSpec {
            name: "m_derived_tail".to_string(),
            member_type_id: 3,
            byte_offset: 5,
            byte_size: 1,
            bit_offset: None,
            bit_size: None,
            is_artificial: false,
        }];

        let mut next_m = 1;
        let mut next_i = 1;
        let (members, inh_rows) = resolve_struct_layout(
            100,
            false,
            8,
            &direct_members,
            &inherits,
            &mut next_m,
            &mut next_i,
        );

        assert!(inh_rows[0].is_empty_base);
        assert!(inh_rows[1].has_tail_padding_overlap);
        assert_eq!(members.len(), 2);
        assert!(members[0].is_flattened_base_member);
        assert!(!members[0].is_layout_conflict);
        assert!(!members[1].is_layout_conflict);
    }

    #[test]
    fn test_bitfield_intervals_and_union_arms() {
        let bitfield_members = vec![
            RawMemberSpec {
                name: "flag_a".to_string(),
                member_type_id: 2,
                byte_offset: 0,
                byte_size: 1,
                bit_offset: Some(0),
                bit_size: Some(4),
                is_artificial: false,
            },
            RawMemberSpec {
                name: "flag_b".to_string(),
                member_type_id: 2,
                byte_offset: 0,
                byte_size: 2,
                bit_offset: Some(4),
                bit_size: Some(6),
                is_artificial: false,
            },
            RawMemberSpec {
                name: "overlap_c".to_string(),
                member_type_id: 2,
                byte_offset: 0,
                byte_size: 1,
                bit_offset: Some(2),
                bit_size: Some(2),
                is_artificial: false,
            },
            RawMemberSpec {
                name: "zero_bf".to_string(),
                member_type_id: 2,
                byte_offset: 2,
                byte_size: 1,
                bit_offset: Some(0),
                bit_size: Some(0),
                is_artificial: false,
            },
            RawMemberSpec {
                name: "oob_bf".to_string(),
                member_type_id: 2,
                byte_offset: 3,
                byte_size: 1,
                bit_offset: Some(6),
                bit_size: Some(4),
                is_artificial: false,
            },
        ];

        let mut next_m = 1;
        let mut next_i = 1;
        let (members, _) = resolve_struct_layout(
            200,
            false,
            4,
            &bitfield_members,
            &[],
            &mut next_m,
            &mut next_i,
        );
        assert!(!members[0].is_layout_conflict);
        assert!(!members[1].is_layout_conflict);
        assert!(members[2].is_layout_conflict);
        assert!(members[3].is_layout_conflict);
        assert!(members[4].is_layout_conflict);

        let union_members = vec![
            RawMemberSpec {
                name: "as_int".to_string(),
                member_type_id: 2,
                byte_offset: 0,
                byte_size: 4,
                bit_offset: None,
                bit_size: None,
                is_artificial: false,
            },
            RawMemberSpec {
                name: "as_float".to_string(),
                member_type_id: 3,
                byte_offset: 0,
                byte_size: 4,
                bit_offset: None,
                bit_size: None,
                is_artificial: false,
            },
            RawMemberSpec {
                name: "as_bit".to_string(),
                member_type_id: 2,
                byte_offset: 0,
                byte_size: 1,
                bit_offset: Some(0),
                bit_size: Some(1),
                is_artificial: false,
            },
            RawMemberSpec {
                name: "too_big".to_string(),
                member_type_id: 4,
                byte_offset: 0,
                byte_size: 8,
                bit_offset: None,
                bit_size: None,
                is_artificial: false,
            },
        ];
        let (u_members, _) = resolve_struct_layout(
            201,
            true,
            4,
            &union_members,
            &[],
            &mut next_m,
            &mut next_i,
        );
        assert!(!u_members[0].is_layout_conflict);
        assert!(!u_members[1].is_layout_conflict);
        assert!(!u_members[2].is_layout_conflict);
        assert!(u_members[3].is_layout_conflict);
    }
}
