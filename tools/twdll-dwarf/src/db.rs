//! SQLite prepared-statement batch writer for `twdll-dwarf`.
//!
//! Opens an already-initialized `.sqlite.staging` database created by `twdll.database`
//! and bulk-inserts all 11 `dwarf_*` tables in a single transaction without touching
//! `meta` or renaming the staging file.

use std::ffi::{CStr, CString};
use std::os::raw::{c_char, c_int, c_void};
use std::path::Path;
use std::ptr;

#[repr(C)]
struct Sqlite3 {
    _private: [u8; 0],
}

#[repr(C)]
struct Sqlite3Stmt {
    _private: [u8; 0],
}

const SQLITE_OK: c_int = 0;
const SQLITE_ROW: c_int = 100;
const SQLITE_DONE: c_int = 101;
const SQLITE_OPEN_READWRITE: c_int = 0x0000_0002;
const SQLITE_TRANSIENT: isize = -1;

#[link(name = "sqlite3")]
unsafe extern "C" {
    fn sqlite3_open_v2(
        filename: *const c_char,
        pp_db: *mut *mut Sqlite3,
        flags: c_int,
        z_vfs: *const c_char,
    ) -> c_int;
    fn sqlite3_close(db: *mut Sqlite3) -> c_int;
    fn sqlite3_errmsg(db: *mut Sqlite3) -> *const c_char;
    fn sqlite3_exec(
        db: *mut Sqlite3,
        sql: *const c_char,
        callback: Option<unsafe extern "C" fn(*mut c_void, c_int, *mut *mut c_char, *mut *mut c_char) -> c_int>,
        arg: *mut c_void,
        errmsg: *mut *mut c_char,
    ) -> c_int;
    fn sqlite3_prepare_v2(
        db: *mut Sqlite3,
        z_sql: *const c_char,
        n_byte: c_int,
        pp_stmt: *mut *mut Sqlite3Stmt,
        pz_tail: *mut *const c_char,
    ) -> c_int;
    fn sqlite3_step(stmt: *mut Sqlite3Stmt) -> c_int;
    fn sqlite3_reset(stmt: *mut Sqlite3Stmt) -> c_int;
    fn sqlite3_clear_bindings(stmt: *mut Sqlite3Stmt) -> c_int;
    fn sqlite3_finalize(stmt: *mut Sqlite3Stmt) -> c_int;
    fn sqlite3_bind_int64(stmt: *mut Sqlite3Stmt, idx: c_int, val: i64) -> c_int;
    fn sqlite3_bind_null(stmt: *mut Sqlite3Stmt, idx: c_int) -> c_int;
    fn sqlite3_bind_text(
        stmt: *mut Sqlite3Stmt,
        idx: c_int,
        val: *const c_char,
        n: c_int,
        destructor: *const c_void,
    ) -> c_int;
    fn sqlite3_column_int64(stmt: *mut Sqlite3Stmt, i_col: c_int) -> i64;
}

#[derive(Clone, Debug)]
pub struct UnitRow {
    pub unit_id: i64,
    pub name: String,
    pub comp_dir: String,
    pub producer: String,
    pub language: i64,
}

#[derive(Clone, Debug)]
pub struct TypeRow {
    pub type_id: i64,
    pub kind: &'static str,
    pub qualified_name: String,
    pub byte_size: Option<i64>,
    pub target_type_id: Option<i64>,
    pub array_count: Option<i64>,
    pub containing_type_id: Option<i64>,
    pub vtable_symbol: Option<String>,
    pub vtable_address: Option<i64>,
    pub is_declaration: bool,
    pub is_non_trivial_for_calls: bool,
    pub is_stl_stub: bool,
    pub is_referenced: bool,
    pub abi_class: Option<String>,
}

#[derive(Clone, Debug)]
pub struct MemberRow {
    pub member_id: i64,
    pub parent_type_id: i64,
    pub ordinal: i64,
    pub name: String,
    pub member_type_id: i64,
    pub byte_offset: i64,
    pub byte_size: i64,
    pub bit_offset: Option<i64>,
    pub bit_size: Option<i64>,
    pub is_artificial: bool,
    pub is_flattened_base_member: bool,
    pub is_layout_conflict: bool,
}

#[derive(Clone, Debug)]
pub struct InheritanceRow {
    pub inheritance_id: i64,
    pub derived_type_id: i64,
    pub base_type_id: i64,
    pub byte_offset: Option<i64>,
    pub vbase_vtable_offset: Option<i64>,
    pub is_virtual: bool,
    pub is_empty_base: bool,
    pub has_tail_padding_overlap: bool,
}

#[derive(Clone, Debug)]
pub struct VtableEntryRow {
    pub class_type_id: i64,
    pub slot_index: i64,
    pub method_name: String,
    pub linkage_name: Option<String>,
    pub function_id: Option<i64>,
    pub target_address: Option<i64>,
    pub is_pure_virtual: bool,
}

#[derive(Clone, Debug)]
pub struct EnumeratorRow {
    pub enum_type_id: i64,
    pub ordinal: i64,
    pub name: String,
    pub value: i64,
}

#[derive(Clone, Debug)]
pub struct FunctionRow {
    pub function_id: i64,
    pub unit_id: i64,
    pub address: Option<i64>,
    pub byte_size: Option<i64>,
    pub linkage_name: Option<String>,
    pub qualified_name: String,
    pub containing_type_id: Option<i64>,
    pub return_type_id: Option<i64>,
    pub vtable_slot_index: Option<i64>,
    pub virtuality: i64,
    pub is_inline: bool,
    pub is_declaration: bool,
    pub is_variadic: bool,
    pub has_object_pointer: bool,
    pub is_sret: bool,
    pub decl_file: Option<String>,
    pub decl_line: Option<i64>,
}

#[derive(Clone, Debug)]
pub struct ParameterRow {
    pub function_id: i64,
    pub ordinal: i64,
    pub name: Option<String>,
    pub type_id: i64,
    pub is_artificial: bool,
    pub is_object_pointer: bool,
    pub storage_kind: &'static str,
    pub dwarf_reg: Option<i64>,
    pub reg_name: Option<String>,
    pub stack_offset: Option<i64>,
}

#[derive(Clone, Debug)]
pub struct VariableRow {
    pub variable_id: i64,
    pub function_id: i64,
    pub name: String,
    pub type_id: i64,
    pub range_index: i64,
    pub start_address: i64,
    pub end_address: i64,
    pub first_use_offset: i64,
    pub storage_kind: &'static str,
    pub reg_name: Option<String>,
    pub stack_offset: Option<i64>,
    pub const_value: Option<i64>,
}

#[derive(Clone, Debug)]
pub struct InlineRow {
    pub inline_id: i64,
    pub caller_function_id: i64,
    pub callee_function_id: Option<i64>,
    pub callee_name: String,
    pub start_address: i64,
    pub end_address: i64,
    pub call_file: Option<String>,
    pub call_line: Option<i64>,
}

#[derive(Clone, Debug)]
pub struct GlobalRow {
    pub global_id: i64,
    pub unit_id: i64,
    pub address: i64,
    pub linkage_name: Option<String>,
    pub qualified_name: String,
    pub type_id: i64,
    pub byte_size: Option<i64>,
    pub is_static_member: bool,
}

#[derive(Default)]
pub struct DwarfBatch {
    pub units: Vec<UnitRow>,
    pub types: Vec<TypeRow>,
    pub members: Vec<MemberRow>,
    pub inheritances: Vec<InheritanceRow>,
    pub vtable_entries: Vec<VtableEntryRow>,
    pub enumerators: Vec<EnumeratorRow>,
    pub functions: Vec<FunctionRow>,
    pub parameters: Vec<ParameterRow>,
    pub variables: Vec<VariableRow>,
    pub inlines: Vec<InlineRow>,
    pub globals: Vec<GlobalRow>,
}

struct DbConn {
    ptr: *mut Sqlite3,
}

impl Drop for DbConn {
    fn drop(&mut self) {
        if !self.ptr.is_null() {
            unsafe {
                sqlite3_close(self.ptr);
            }
        }
    }
}

struct Stmt {
    db: *mut Sqlite3,
    ptr: *mut Sqlite3Stmt,
}

impl Drop for Stmt {
    fn drop(&mut self) {
        if !self.ptr.is_null() {
            unsafe {
                sqlite3_finalize(self.ptr);
            }
        }
    }
}

fn db_err(db: *mut Sqlite3, context: &str) -> String {
    if db.is_null() {
        return format!("{context}: null database handle");
    }
    let msg = unsafe {
        let p = sqlite3_errmsg(db);
        if p.is_null() {
            "unknown sqlite error".to_string()
        } else {
            CStr::from_ptr(p).to_string_lossy().into_owned()
        }
    };
    format!("{context}: {msg}")
}

impl DbConn {
    fn open_existing(path: &Path) -> Result<Self, String> {
        if !path.is_file() {
            return Err(format!(
                "Staging SQLite database does not exist: {}",
                path.display()
            ));
        }
        let c_path = CString::new(path.to_string_lossy().as_bytes())
            .map_err(|e| format!("Invalid path: {e}"))?;
        let mut ptr: *mut Sqlite3 = ptr::null_mut();
        let rc = unsafe {
            sqlite3_open_v2(
                c_path.as_ptr(),
                &mut ptr,
                SQLITE_OPEN_READWRITE,
                ptr::null(),
            )
        };
        let conn = Self { ptr };
        if rc != SQLITE_OK {
            return Err(db_err(conn.ptr, "sqlite3_open_v2 failed"));
        }
        Ok(conn)
    }

    fn exec(&self, sql: &str) -> Result<(), String> {
        let c_sql = CString::new(sql).map_err(|e| format!("Invalid SQL string: {e}"))?;
        let rc = unsafe {
            sqlite3_exec(
                self.ptr,
                c_sql.as_ptr(),
                None,
                ptr::null_mut(),
                ptr::null_mut(),
            )
        };
        if rc != SQLITE_OK {
            return Err(db_err(self.ptr, sql));
        }
        Ok(())
    }

    fn prepare(&self, sql: &str) -> Result<Stmt, String> {
        let mut stmt_ptr: *mut Sqlite3Stmt = ptr::null_mut();
        let rc = unsafe {
            sqlite3_prepare_v2(
                self.ptr,
                sql.as_ptr().cast::<c_char>(),
                sql.len() as c_int,
                &mut stmt_ptr,
                ptr::null_mut(),
            )
        };
        if rc != SQLITE_OK {
            return Err(db_err(self.ptr, sql));
        }
        Ok(Stmt {
            db: self.ptr,
            ptr: stmt_ptr,
        })
    }

    fn verify_required_tables(&self) -> Result<(), String> {
        let stmt = self.prepare(
            "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name LIKE 'dwarf_%'",
        )?;
        let rc = unsafe { sqlite3_step(stmt.ptr) };
        if rc != SQLITE_ROW {
            return Err(db_err(self.ptr, "Failed querying sqlite_master"));
        }
        let count = unsafe { sqlite3_column_int64(stmt.ptr, 0) };
        if count < 11 {
            return Err(format!(
                "Staging SQLite database is missing pre-initialized dwarf_* tables (found {count}/11)"
            ));
        }
        Ok(())
    }
}

impl Stmt {
    fn bind_i64(&self, idx: c_int, val: i64) -> Result<(), String> {
        let rc = unsafe { sqlite3_bind_int64(self.ptr, idx, val) };
        if rc != SQLITE_OK {
            return Err(db_err(self.db, "sqlite3_bind_int64"));
        }
        Ok(())
    }

    fn bind_bool(&self, idx: c_int, val: bool) -> Result<(), String> {
        self.bind_i64(idx, i64::from(val))
    }

    fn bind_opt_i64(&self, idx: c_int, val: Option<i64>) -> Result<(), String> {
        match val {
            Some(v) => self.bind_i64(idx, v),
            None => {
                let rc = unsafe { sqlite3_bind_null(self.ptr, idx) };
                if rc != SQLITE_OK {
                    return Err(db_err(self.db, "sqlite3_bind_null"));
                }
                Ok(())
            }
        }
    }

    fn bind_str(&self, idx: c_int, val: &str) -> Result<(), String> {
        let destructor = SQLITE_TRANSIENT as *const c_void;
        let rc = unsafe {
            sqlite3_bind_text(
                self.ptr,
                idx,
                val.as_ptr().cast::<c_char>(),
                val.len() as c_int,
                destructor,
            )
        };
        if rc != SQLITE_OK {
            return Err(db_err(self.db, "sqlite3_bind_text"));
        }
        Ok(())
    }

    fn bind_opt_str(&self, idx: c_int, val: Option<&str>) -> Result<(), String> {
        match val {
            Some(s) => self.bind_str(idx, s),
            None => {
                let rc = unsafe { sqlite3_bind_null(self.ptr, idx) };
                if rc != SQLITE_OK {
                    return Err(db_err(self.db, "sqlite3_bind_null"));
                }
                Ok(())
            }
        }
    }

    fn step_and_reset(&self) -> Result<(), String> {
        let rc = unsafe { sqlite3_step(self.ptr) };
        if rc != SQLITE_DONE {
            return Err(db_err(self.db, "sqlite3_step"));
        }
        unsafe {
            sqlite3_reset(self.ptr);
            sqlite3_clear_bindings(self.ptr);
        }
        Ok(())
    }
}

pub fn write_batch_to_staging_db(staging_path: &Path, batch: &DwarfBatch) -> Result<(), String> {
    let conn = DbConn::open_existing(staging_path)?;
    conn.verify_required_tables()?;
    conn.exec("PRAGMA synchronous = NORMAL;")?;
    conn.exec("BEGIN IMMEDIATE TRANSACTION;")?;

    let res = (|| -> Result<(), String> {
        {
            let stmt = conn.prepare(
                "INSERT INTO dwarf_units (unit_id, name, comp_dir, producer, language) VALUES (?1, ?2, ?3, ?4, ?5)",
            )?;
            for u in &batch.units {
                stmt.bind_i64(1, u.unit_id)?;
                stmt.bind_str(2, &u.name)?;
                stmt.bind_str(3, &u.comp_dir)?;
                stmt.bind_str(4, &u.producer)?;
                stmt.bind_i64(5, u.language)?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT INTO dwarf_types (type_id, kind, qualified_name, byte_size, target_type_id, array_count, containing_type_id, vtable_symbol, vtable_address, is_declaration, is_non_trivial_for_calls, is_stl_stub, is_referenced, abi_class) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14)",
            )?;
            for t in &batch.types {
                stmt.bind_i64(1, t.type_id)?;
                stmt.bind_str(2, t.kind)?;
                stmt.bind_str(3, &t.qualified_name)?;
                stmt.bind_opt_i64(4, t.byte_size)?;
                stmt.bind_opt_i64(5, t.target_type_id)?;
                stmt.bind_opt_i64(6, t.array_count)?;
                stmt.bind_opt_i64(7, t.containing_type_id)?;
                stmt.bind_opt_str(8, t.vtable_symbol.as_deref())?;
                stmt.bind_opt_i64(9, t.vtable_address)?;
                stmt.bind_bool(10, t.is_declaration)?;
                stmt.bind_bool(11, t.is_non_trivial_for_calls)?;
                stmt.bind_bool(12, t.is_stl_stub)?;
                stmt.bind_bool(13, t.is_referenced)?;
                stmt.bind_opt_str(14, t.abi_class.as_deref())?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT INTO dwarf_members (member_id, parent_type_id, ordinal, name, member_type_id, byte_offset, byte_size, bit_offset, bit_size, is_artificial, is_flattened_base_member, is_layout_conflict) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)",
            )?;
            for m in &batch.members {
                stmt.bind_i64(1, m.member_id)?;
                stmt.bind_i64(2, m.parent_type_id)?;
                stmt.bind_i64(3, m.ordinal)?;
                stmt.bind_str(4, &m.name)?;
                stmt.bind_i64(5, m.member_type_id)?;
                stmt.bind_i64(6, m.byte_offset)?;
                stmt.bind_i64(7, m.byte_size)?;
                stmt.bind_opt_i64(8, m.bit_offset)?;
                stmt.bind_opt_i64(9, m.bit_size)?;
                stmt.bind_bool(10, m.is_artificial)?;
                stmt.bind_bool(11, m.is_flattened_base_member)?;
                stmt.bind_bool(12, m.is_layout_conflict)?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT INTO dwarf_inheritance (inheritance_id, derived_type_id, base_type_id, byte_offset, vbase_vtable_offset, is_virtual, is_empty_base, has_tail_padding_overlap) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
            )?;
            for i in &batch.inheritances {
                stmt.bind_i64(1, i.inheritance_id)?;
                stmt.bind_i64(2, i.derived_type_id)?;
                stmt.bind_i64(3, i.base_type_id)?;
                stmt.bind_opt_i64(4, i.byte_offset)?;
                stmt.bind_opt_i64(5, i.vbase_vtable_offset)?;
                stmt.bind_bool(6, i.is_virtual)?;
                stmt.bind_bool(7, i.is_empty_base)?;
                stmt.bind_bool(8, i.has_tail_padding_overlap)?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT OR REPLACE INTO dwarf_vtable_entries (class_type_id, slot_index, method_name, linkage_name, function_id, target_address, is_pure_virtual) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
            )?;
            for v in &batch.vtable_entries {
                stmt.bind_i64(1, v.class_type_id)?;
                stmt.bind_i64(2, v.slot_index)?;
                stmt.bind_str(3, &v.method_name)?;
                stmt.bind_opt_str(4, v.linkage_name.as_deref())?;
                stmt.bind_opt_i64(5, v.function_id)?;
                stmt.bind_opt_i64(6, v.target_address)?;
                stmt.bind_bool(7, v.is_pure_virtual)?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT OR REPLACE INTO dwarf_enumerators (enum_type_id, ordinal, name, value) VALUES (?1, ?2, ?3, ?4)",
            )?;
            for e in &batch.enumerators {
                stmt.bind_i64(1, e.enum_type_id)?;
                stmt.bind_i64(2, e.ordinal)?;
                stmt.bind_str(3, &e.name)?;
                stmt.bind_i64(4, e.value)?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT INTO dwarf_functions (function_id, unit_id, address, byte_size, linkage_name, qualified_name, containing_type_id, return_type_id, vtable_slot_index, virtuality, is_inline, is_declaration, is_variadic, has_object_pointer, is_sret, decl_file, decl_line) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17)",
            )?;
            for f in &batch.functions {
                stmt.bind_i64(1, f.function_id)?;
                stmt.bind_i64(2, f.unit_id)?;
                stmt.bind_opt_i64(3, f.address)?;
                stmt.bind_opt_i64(4, f.byte_size)?;
                stmt.bind_opt_str(5, f.linkage_name.as_deref())?;
                stmt.bind_str(6, &f.qualified_name)?;
                stmt.bind_opt_i64(7, f.containing_type_id)?;
                stmt.bind_opt_i64(8, f.return_type_id)?;
                stmt.bind_opt_i64(9, f.vtable_slot_index)?;
                stmt.bind_i64(10, f.virtuality)?;
                stmt.bind_bool(11, f.is_inline)?;
                stmt.bind_bool(12, f.is_declaration)?;
                stmt.bind_bool(13, f.is_variadic)?;
                stmt.bind_bool(14, f.has_object_pointer)?;
                stmt.bind_bool(15, f.is_sret)?;
                stmt.bind_opt_str(16, f.decl_file.as_deref())?;
                stmt.bind_opt_i64(17, f.decl_line)?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT OR REPLACE INTO dwarf_parameters (function_id, ordinal, name, type_id, is_artificial, is_object_pointer, storage_kind, dwarf_reg, reg_name, stack_offset) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)",
            )?;
            for p in &batch.parameters {
                stmt.bind_i64(1, p.function_id)?;
                stmt.bind_i64(2, p.ordinal)?;
                stmt.bind_opt_str(3, p.name.as_deref())?;
                stmt.bind_i64(4, p.type_id)?;
                stmt.bind_bool(5, p.is_artificial)?;
                stmt.bind_bool(6, p.is_object_pointer)?;
                stmt.bind_str(7, p.storage_kind)?;
                stmt.bind_opt_i64(8, p.dwarf_reg)?;
                stmt.bind_opt_str(9, p.reg_name.as_deref())?;
                stmt.bind_opt_i64(10, p.stack_offset)?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT INTO dwarf_variables (variable_id, function_id, name, type_id, range_index, start_address, end_address, first_use_offset, storage_kind, reg_name, stack_offset, const_value) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)",
            )?;
            for v in &batch.variables {
                stmt.bind_i64(1, v.variable_id)?;
                stmt.bind_i64(2, v.function_id)?;
                stmt.bind_str(3, &v.name)?;
                stmt.bind_i64(4, v.type_id)?;
                stmt.bind_i64(5, v.range_index)?;
                stmt.bind_i64(6, v.start_address)?;
                stmt.bind_i64(7, v.end_address)?;
                stmt.bind_i64(8, v.first_use_offset)?;
                stmt.bind_str(9, v.storage_kind)?;
                stmt.bind_opt_str(10, v.reg_name.as_deref())?;
                stmt.bind_opt_i64(11, v.stack_offset)?;
                stmt.bind_opt_i64(12, v.const_value)?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT INTO dwarf_inlines (inline_id, caller_function_id, callee_function_id, callee_name, start_address, end_address, call_file, call_line) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
            )?;
            for i in &batch.inlines {
                stmt.bind_i64(1, i.inline_id)?;
                stmt.bind_i64(2, i.caller_function_id)?;
                stmt.bind_opt_i64(3, i.callee_function_id)?;
                stmt.bind_str(4, &i.callee_name)?;
                stmt.bind_i64(5, i.start_address)?;
                stmt.bind_i64(6, i.end_address)?;
                stmt.bind_opt_str(7, i.call_file.as_deref())?;
                stmt.bind_opt_i64(8, i.call_line)?;
                stmt.step_and_reset()?;
            }
        }
        {
            let stmt = conn.prepare(
                "INSERT INTO dwarf_globals (global_id, unit_id, address, linkage_name, qualified_name, type_id, byte_size, is_static_member) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
            )?;
            for g in &batch.globals {
                stmt.bind_i64(1, g.global_id)?;
                stmt.bind_i64(2, g.unit_id)?;
                stmt.bind_i64(3, g.address)?;
                stmt.bind_opt_str(4, g.linkage_name.as_deref())?;
                stmt.bind_str(5, &g.qualified_name)?;
                stmt.bind_i64(6, g.type_id)?;
                stmt.bind_opt_i64(7, g.byte_size)?;
                stmt.bind_bool(8, g.is_static_member)?;
                stmt.step_and_reset()?;
            }
        }
        Ok(())
    })();

    if let Err(err) = res {
        let _ = conn.exec("ROLLBACK;");
        return Err(err);
    }
    conn.exec("COMMIT;")?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    use std::sync::atomic::{AtomicU64, Ordering};

    const SQLITE_OPEN_CREATE: c_int = 0x0000_0004;

    #[link(name = "sqlite3")]
    unsafe extern "C" {
        fn sqlite3_column_text(stmt: *mut Sqlite3Stmt, i_col: c_int) -> *const c_char;
    }

    static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

    struct TempDb {
        path: PathBuf,
    }

    impl TempDb {
        fn new(name: &str) -> Self {
            let n = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
            let path = std::env::temp_dir().join(format!(
                "twdll_dwarf_test_{name}_{}_{n}.sqlite",
                std::process::id()
            ));
            let _ = std::fs::remove_file(&path);
            Self { path }
        }

        fn open_with_ddl(&self, ddl: &str) -> DbConn {
            let c_path = CString::new(self.path.to_string_lossy().as_bytes()).unwrap();
            let mut ptr: *mut Sqlite3 = ptr::null_mut();
            let rc = unsafe {
                sqlite3_open_v2(
                    c_path.as_ptr(),
                    &mut ptr,
                    SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE,
                    ptr::null(),
                )
            };
            let conn = DbConn { ptr };
            assert_eq!(rc, SQLITE_OK);
            conn.exec(ddl).unwrap();
            conn
        }
    }

    impl Drop for TempDb {
        fn drop(&mut self) {
            let _ = std::fs::remove_file(&self.path);
        }
    }

    const DDL_11_TABLES: &str = "
        CREATE TABLE dwarf_units (
            unit_id INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            comp_dir TEXT NOT NULL,
            producer TEXT NOT NULL,
            language INTEGER NOT NULL
        );
        CREATE TABLE dwarf_types (
            type_id INTEGER PRIMARY KEY,
            kind TEXT NOT NULL,
            qualified_name TEXT NOT NULL,
            byte_size INTEGER,
            target_type_id INTEGER,
            array_count INTEGER,
            containing_type_id INTEGER,
            vtable_symbol TEXT,
            vtable_address INTEGER,
            is_declaration INTEGER NOT NULL DEFAULT 0,
            is_non_trivial_for_calls INTEGER NOT NULL DEFAULT 0,
            is_stl_stub INTEGER NOT NULL DEFAULT 0,
            is_referenced INTEGER NOT NULL DEFAULT 0,
            abi_class TEXT
        );
        CREATE TABLE dwarf_members (
            member_id INTEGER PRIMARY KEY,
            parent_type_id INTEGER NOT NULL,
            ordinal INTEGER NOT NULL,
            name TEXT NOT NULL,
            member_type_id INTEGER NOT NULL,
            byte_offset INTEGER NOT NULL,
            byte_size INTEGER NOT NULL,
            bit_offset INTEGER,
            bit_size INTEGER,
            is_artificial INTEGER NOT NULL DEFAULT 0,
            is_flattened_base_member INTEGER NOT NULL DEFAULT 0,
            is_layout_conflict INTEGER NOT NULL DEFAULT 0
        );
        CREATE TABLE dwarf_inheritance (
            inheritance_id INTEGER PRIMARY KEY,
            derived_type_id INTEGER NOT NULL,
            base_type_id INTEGER NOT NULL,
            byte_offset INTEGER,
            vbase_vtable_offset INTEGER,
            is_virtual INTEGER NOT NULL DEFAULT 0,
            is_empty_base INTEGER NOT NULL DEFAULT 0,
            has_tail_padding_overlap INTEGER NOT NULL DEFAULT 0
        );
        CREATE TABLE dwarf_vtable_entries (
            class_type_id INTEGER NOT NULL,
            slot_index INTEGER NOT NULL,
            method_name TEXT NOT NULL,
            linkage_name TEXT,
            function_id INTEGER,
            target_address INTEGER,
            is_pure_virtual INTEGER NOT NULL DEFAULT 0,
            PRIMARY KEY (class_type_id, slot_index)
        );
        CREATE TABLE dwarf_enumerators (
            enum_type_id INTEGER NOT NULL,
            ordinal INTEGER NOT NULL,
            name TEXT NOT NULL,
            value INTEGER NOT NULL,
            PRIMARY KEY (enum_type_id, ordinal)
        );
        CREATE TABLE dwarf_functions (
            function_id INTEGER PRIMARY KEY,
            unit_id INTEGER NOT NULL,
            address INTEGER,
            byte_size INTEGER,
            linkage_name TEXT,
            qualified_name TEXT NOT NULL,
            containing_type_id INTEGER,
            return_type_id INTEGER,
            vtable_slot_index INTEGER,
            virtuality INTEGER NOT NULL DEFAULT 0,
            is_inline INTEGER NOT NULL DEFAULT 0,
            is_declaration INTEGER NOT NULL DEFAULT 0,
            is_variadic INTEGER NOT NULL DEFAULT 0,
            has_object_pointer INTEGER NOT NULL DEFAULT 0,
            is_sret INTEGER NOT NULL DEFAULT 0,
            decl_file TEXT,
            decl_line INTEGER
        );
        CREATE TABLE dwarf_parameters (
            function_id INTEGER NOT NULL,
            ordinal INTEGER NOT NULL,
            name TEXT,
            type_id INTEGER NOT NULL,
            is_artificial INTEGER NOT NULL DEFAULT 0,
            is_object_pointer INTEGER NOT NULL DEFAULT 0,
            storage_kind TEXT NOT NULL,
            dwarf_reg INTEGER,
            reg_name TEXT,
            stack_offset INTEGER,
            PRIMARY KEY (function_id, ordinal)
        );
        CREATE TABLE dwarf_variables (
            variable_id INTEGER PRIMARY KEY,
            function_id INTEGER NOT NULL,
            name TEXT NOT NULL,
            type_id INTEGER NOT NULL,
            range_index INTEGER NOT NULL DEFAULT 0,
            start_address INTEGER NOT NULL,
            end_address INTEGER NOT NULL,
            first_use_offset INTEGER NOT NULL,
            storage_kind TEXT NOT NULL,
            reg_name TEXT,
            stack_offset INTEGER,
            const_value INTEGER
        );
        CREATE TABLE dwarf_inlines (
            inline_id INTEGER PRIMARY KEY,
            caller_function_id INTEGER NOT NULL,
            callee_function_id INTEGER,
            callee_name TEXT NOT NULL,
            start_address INTEGER NOT NULL,
            end_address INTEGER NOT NULL,
            call_file TEXT,
            call_line INTEGER
        );
        CREATE TABLE dwarf_globals (
            global_id INTEGER PRIMARY KEY,
            unit_id INTEGER NOT NULL,
            address INTEGER NOT NULL UNIQUE,
            linkage_name TEXT,
            qualified_name TEXT NOT NULL,
            type_id INTEGER NOT NULL,
            byte_size INTEGER,
            is_static_member INTEGER NOT NULL DEFAULT 0
        );
    ";

    fn sample_batch() -> DwarfBatch {
        DwarfBatch {
            units: vec![UnitRow {
                unit_id: 1,
                name: "unit.cpp".to_string(),
                comp_dir: "/build".to_string(),
                producer: "clang 3.4.2".to_string(),
                language: 4,
            }],
            types: vec![TypeRow {
                type_id: 1,
                kind: "struct",
                qualified_name: "Vec2".to_string(),
                byte_size: Some(16),
                target_type_id: None,
                array_count: None,
                containing_type_id: None,
                vtable_symbol: None,
                vtable_address: None,
                is_declaration: false,
                is_non_trivial_for_calls: false,
                is_stl_stub: false,
                is_referenced: true,
                abi_class: Some("sse,sse".to_string()),
            }],
            members: vec![MemberRow {
                member_id: 1,
                parent_type_id: 1,
                ordinal: 0,
                name: "x".to_string(),
                member_type_id: 1,
                byte_offset: 0,
                byte_size: 8,
                bit_offset: None,
                bit_size: None,
                is_artificial: false,
                is_flattened_base_member: false,
                is_layout_conflict: false,
            }],
            inheritances: vec![InheritanceRow {
                inheritance_id: 1,
                derived_type_id: 1,
                base_type_id: 1,
                byte_offset: Some(0),
                vbase_vtable_offset: None,
                is_virtual: false,
                is_empty_base: false,
                has_tail_padding_overlap: false,
            }],
            vtable_entries: vec![VtableEntryRow {
                class_type_id: 1,
                slot_index: 0,
                method_name: "f".to_string(),
                linkage_name: None,
                function_id: Some(1),
                target_address: Some(0x401000),
                is_pure_virtual: false,
            }],
            enumerators: vec![EnumeratorRow {
                enum_type_id: 1,
                ordinal: 0,
                name: "A".to_string(),
                value: 0,
            }],
            functions: vec![FunctionRow {
                function_id: 1,
                unit_id: 1,
                address: Some(0x401000),
                byte_size: Some(16),
                linkage_name: None,
                qualified_name: "Vec2::f".to_string(),
                containing_type_id: Some(1),
                return_type_id: Some(1),
                vtable_slot_index: None,
                virtuality: 0,
                is_inline: false,
                is_declaration: false,
                is_variadic: false,
                has_object_pointer: true,
                is_sret: false,
                decl_file: None,
                decl_line: None,
            }],
            parameters: vec![ParameterRow {
                function_id: 1,
                ordinal: 0,
                name: Some("this".to_string()),
                type_id: 1,
                is_artificial: true,
                is_object_pointer: true,
                storage_kind: "register",
                dwarf_reg: Some(5),
                reg_name: Some("RDI".to_string()),
                stack_offset: None,
            }],
            variables: vec![VariableRow {
                variable_id: 1,
                function_id: 1,
                name: "v".to_string(),
                type_id: 1,
                range_index: 0,
                start_address: 0x401000,
                end_address: 0x401010,
                first_use_offset: 0,
                storage_kind: "register",
                reg_name: Some("RAX".to_string()),
                stack_offset: None,
                const_value: None,
            }],
            inlines: vec![InlineRow {
                inline_id: 1,
                caller_function_id: 1,
                callee_function_id: None,
                callee_name: "helper".to_string(),
                start_address: 0x401004,
                end_address: 0x401008,
                call_file: None,
                call_line: None,
            }],
            globals: vec![GlobalRow {
                global_id: 1,
                unit_id: 1,
                address: 0x57d0a0,
                linkage_name: Some("_ZN17EMPIRE_APP_MODULE10s_instanceE".to_string()),
                qualified_name: "EMPIRE_APP_MODULE::s_instance".to_string(),
                type_id: 1,
                byte_size: Some(16),
                is_static_member: true,
            }],
        }
    }

    #[test]
    fn test_write_batch_roundtrip_all_11_tables() {
        let tmp = TempDb::new("roundtrip");
        let conn = tmp.open_with_ddl(DDL_11_TABLES);
        drop(conn);

        let batch = sample_batch();
        write_batch_to_staging_db(&tmp.path, &batch).unwrap();

        let verify_conn = DbConn::open_existing(&tmp.path).unwrap();
        for table in [
            "dwarf_units",
            "dwarf_types",
            "dwarf_members",
            "dwarf_inheritance",
            "dwarf_vtable_entries",
            "dwarf_enumerators",
            "dwarf_functions",
            "dwarf_parameters",
            "dwarf_variables",
            "dwarf_inlines",
            "dwarf_globals",
        ] {
            let stmt = verify_conn
                .prepare(&format!("SELECT COUNT(*) FROM {table}"))
                .unwrap();
            assert_eq!(unsafe { sqlite3_step(stmt.ptr) }, SQLITE_ROW);
            assert_eq!(unsafe { sqlite3_column_int64(stmt.ptr, 0) }, 1);
        }

        let stmt = verify_conn
            .prepare("SELECT abi_class FROM dwarf_types WHERE type_id = 1")
            .unwrap();
        assert_eq!(unsafe { sqlite3_step(stmt.ptr) }, SQLITE_ROW);
        let text_ptr = unsafe { sqlite3_column_text(stmt.ptr, 0) };
        assert!(!text_ptr.is_null());
        let abi = unsafe { CStr::from_ptr(text_ptr) }.to_str().unwrap();
        assert_eq!(abi, "sse,sse");

        let gstmt = verify_conn
            .prepare("SELECT address, qualified_name, is_static_member FROM dwarf_globals WHERE global_id = 1")
            .unwrap();
        assert_eq!(unsafe { sqlite3_step(gstmt.ptr) }, SQLITE_ROW);
        assert_eq!(unsafe { sqlite3_column_int64(gstmt.ptr, 0) }, 0x57d0a0);
        let qn_ptr = unsafe { sqlite3_column_text(gstmt.ptr, 1) };
        assert_eq!(
            unsafe { CStr::from_ptr(qn_ptr) }.to_str().unwrap(),
            "EMPIRE_APP_MODULE::s_instance"
        );
        assert_eq!(unsafe { sqlite3_column_int64(gstmt.ptr, 2) }, 1);
    }

    #[test]
    fn test_dwarf_globals_unique_address_enforced() {
        let tmp = TempDb::new("unique_addr");
        let conn = tmp.open_with_ddl(DDL_11_TABLES);
        drop(conn);

        let mut batch = sample_batch();
        batch.globals.push(GlobalRow {
            global_id: 2,
            unit_id: 1,
            address: 0x57d0a0,
            linkage_name: None,
            qualified_name: "dup".to_string(),
            type_id: 1,
            byte_size: Some(8),
            is_static_member: false,
        });
        let err = write_batch_to_staging_db(&tmp.path, &batch).unwrap_err();
        assert!(err.contains("UNIQUE"), "unexpected error: {err}");

        let verify_conn = DbConn::open_existing(&tmp.path).unwrap();
        let stmt = verify_conn
            .prepare("SELECT COUNT(*) FROM dwarf_globals")
            .unwrap();
        assert_eq!(unsafe { sqlite3_step(stmt.ptr) }, SQLITE_ROW);
        assert_eq!(unsafe { sqlite3_column_int64(stmt.ptr, 0) }, 0);
    }

    #[test]
    fn test_verify_required_tables_rejects_fewer_than_11() {
        let tmp = TempDb::new("missing_table");
        let ddl_10 = DDL_11_TABLES
            .split("CREATE TABLE dwarf_globals")
            .next()
            .unwrap();
        let conn = tmp.open_with_ddl(ddl_10);
        drop(conn);

        let err = write_batch_to_staging_db(&tmp.path, &DwarfBatch::default()).unwrap_err();
        assert!(err.contains("10/11"), "unexpected error: {err}");
    }
}
