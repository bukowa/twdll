package extractdb

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

const FactsSchemaVersion = 1

const factsSchemaDDL = `
CREATE TABLE IF NOT EXISTS meta(
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS sections(
	sec_idx INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	va INTEGER NOT NULL,
	virtual_size INTEGER NOT NULL,
	raw_offset INTEGER NOT NULL,
	raw_size INTEGER NOT NULL,
	flags INTEGER NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS relocs(
	va INTEGER PRIMARY KEY
) STRICT;

CREATE TABLE IF NOT EXISTS imports(
	iat_va INTEGER PRIMARY KEY,
	dll_name TEXT NOT NULL,
	sym_name TEXT NOT NULL,
	ordinal INTEGER NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS exports(
	export_id INTEGER PRIMARY KEY,
	ea INTEGER NOT NULL,
	name TEXT NOT NULL,
	ordinal INTEGER NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_exports_ea ON exports(ea);
CREATE INDEX IF NOT EXISTS idx_exports_name ON exports(name);

CREATE TABLE IF NOT EXISTS functions(
	ea INTEGER PRIMARY KEY,
	size INTEGER NOT NULL,
	name TEXT NOT NULL,
	mangled_name TEXT NOT NULL DEFAULT '',
	class_name TEXT NOT NULL DEFAULT '',
	signature TEXT NOT NULL DEFAULT '',
	calling_conv TEXT NOT NULL DEFAULT '',
	cc_source TEXT NOT NULL DEFAULT '',
	stack_purge INTEGER NOT NULL DEFAULT 0,
	discovery_mask INTEGER NOT NULL DEFAULT 0,
	thunk_target_ea INTEGER,
	source_file_id INTEGER,
	decl_line INTEGER NOT NULL DEFAULT 0,
	low_line INTEGER NOT NULL DEFAULT 0,
	high_line INTEGER NOT NULL DEFAULT 0,
	vtable_ea INTEGER,
	vtable_slot INTEGER,
	flags INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX IF NOT EXISTS idx_functions_name ON functions(name);
CREATE INDEX IF NOT EXISTS idx_functions_mangled ON functions(mangled_name) WHERE mangled_name != '';
CREATE INDEX IF NOT EXISTS idx_functions_class ON functions(class_name) WHERE class_name != '';

CREATE TABLE IF NOT EXISTS func_ranges(
	start_ea INTEGER PRIMARY KEY,
	end_ea INTEGER NOT NULL,
	func_ea INTEGER NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_func_ranges_func ON func_ranges(func_ea);

CREATE VIRTUAL TABLE IF NOT EXISTS functions_fts USING fts5(
	name,
	mangled_name,
	class_name,
	content='functions',
	content_rowid='ea',
	tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS types(
	type_id INTEGER PRIMARY KEY,
	kind TEXT NOT NULL,
	name TEXT NOT NULL,
	byte_size INTEGER NOT NULL,
	target_type_id INTEGER,
	is_declaration INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX IF NOT EXISTS idx_types_name ON types(name) WHERE name != '';

CREATE TABLE IF NOT EXISTS type_members(
	type_id INTEGER NOT NULL,
	ordinal INTEGER NOT NULL,
	member_kind TEXT NOT NULL,
	byte_offset INTEGER,
	bit_offset INTEGER NOT NULL DEFAULT 0,
	bit_size INTEGER NOT NULL DEFAULT 0,
	name TEXT NOT NULL DEFAULT '',
	member_type_id INTEGER,
	display_type TEXT NOT NULL DEFAULT '',
	is_virtual_base INTEGER NOT NULL DEFAULT 0,
	vbase_expr TEXT NOT NULL DEFAULT '',
	PRIMARY KEY(type_id, ordinal)
) STRICT, WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS enum_values(
	type_id INTEGER NOT NULL,
	ordinal INTEGER NOT NULL,
	name TEXT NOT NULL,
	value INTEGER NOT NULL,
	PRIMARY KEY(type_id, ordinal)
) STRICT, WITHOUT ROWID;

CREATE VIRTUAL TABLE IF NOT EXISTS types_fts USING fts5(
	name,
	content='types',
	content_rowid='type_id',
	tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS func_vars(
	func_ea INTEGER NOT NULL,
	ordinal INTEGER NOT NULL,
	kind TEXT NOT NULL,
	name TEXT NOT NULL,
	type_id INTEGER,
	display_type TEXT NOT NULL DEFAULT '',
	const_val TEXT NOT NULL DEFAULT '',
	loc_summary TEXT NOT NULL DEFAULT '',
	is_optimized_out INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY(func_ea, ordinal)
) STRICT, WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS source_files(
	file_id INTEGER PRIMARY KEY,
	norm_path TEXT NOT NULL UNIQUE,
	basename TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS names(
	name_id INTEGER PRIMARY KEY,
	value TEXT NOT NULL UNIQUE
) STRICT;

CREATE TABLE IF NOT EXISTS inlines(
	host_ea INTEGER NOT NULL,
	inline_idx INTEGER NOT NULL,
	parent_idx INTEGER NOT NULL DEFAULT -1,
	start_ea INTEGER NOT NULL,
	end_ea INTEGER NOT NULL,
	depth INTEGER NOT NULL,
	callee_name_id INTEGER NOT NULL,
	call_file_id INTEGER,
	call_line INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY(host_ea, inline_idx)
) STRICT, WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_inlines_callee ON inlines(callee_name_id);

CREATE TABLE IF NOT EXISTS strings(
	str_id INTEGER PRIMARY KEY,
	ea INTEGER NOT NULL,
	encoding TEXT NOT NULL,
	value TEXT NOT NULL,
	df_count INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX IF NOT EXISTS idx_strings_ea ON strings(ea);

CREATE TABLE IF NOT EXISTS string_xrefs(
	str_id INTEGER NOT NULL,
	func_ea INTEGER NOT NULL,
	PRIMARY KEY(str_id, func_ea)
) STRICT, WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_string_xrefs_func ON string_xrefs(func_ea, str_id);

CREATE VIRTUAL TABLE IF NOT EXISTS strings_fts USING fts5(
	value,
	content='strings',
	content_rowid='str_id',
	tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS call_sites(
	call_ea INTEGER PRIMARY KEY,
	caller_ea INTEGER NOT NULL,
	callee_ea INTEGER,
	call_kind TEXT NOT NULL,
	base_reg TEXT NOT NULL DEFAULT '',
	mem_disp INTEGER
) STRICT;
CREATE INDEX IF NOT EXISTS idx_call_sites_caller ON call_sites(caller_ea);
CREATE INDEX IF NOT EXISTS idx_call_sites_callee ON call_sites(callee_ea);

CREATE TABLE IF NOT EXISTS data_xrefs(
	from_ea INTEGER NOT NULL,
	func_ea INTEGER NOT NULL,
	to_ea INTEGER NOT NULL,
	access_kind TEXT NOT NULL,
	PRIMARY KEY(from_ea, to_ea, access_kind)
) STRICT, WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_data_xrefs_to ON data_xrefs(to_ea);
CREATE INDEX IF NOT EXISTS idx_data_xrefs_func ON data_xrefs(func_ea);

CREATE TABLE IF NOT EXISTS vtables(
	vtable_ea INTEGER NOT NULL,
	slot INTEGER NOT NULL,
	target_ea INTEGER NOT NULL,
	owner_name TEXT NOT NULL DEFAULT '',
	PRIMARY KEY(vtable_ea, slot)
) STRICT, WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_vtables_target ON vtables(target_ea);

CREATE TABLE IF NOT EXISTS lua_regs(
	table_ea INTEGER NOT NULL,
	method_name TEXT NOT NULL,
	func_ea INTEGER NOT NULL,
	PRIMARY KEY(table_ea, method_name, func_ea)
) STRICT, WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_lua_regs_func ON lua_regs(func_ea);
CREATE INDEX IF NOT EXISTS idx_lua_regs_method ON lua_regs(method_name);
`

func InitFactsSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, factsSchemaDDL); err != nil {
		return fmt.Errorf("create facts schema: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES ('schema_version', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		strconv.Itoa(FactsSchemaVersion),
	); err != nil {
		return fmt.Errorf("record facts schema_version: %w", err)
	}
	return nil
}

func RebuildFactsFTS(ctx context.Context, db *sql.DB) error {
	for _, table := range []string{"functions_fts", "types_fts", "strings_fts"} {
		stmt := fmt.Sprintf("INSERT INTO %s(%s) VALUES('rebuild');", table, table)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("rebuild %s: %w", table, err)
		}
	}
	return nil
}
