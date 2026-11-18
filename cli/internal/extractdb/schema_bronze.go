package extractdb

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

const bronzeSchemaDDL = `
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

CREATE TABLE IF NOT EXISTS strings(
	str_id INTEGER PRIMARY KEY,
	ea INTEGER NOT NULL,
	encoding TEXT NOT NULL,
	value TEXT NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_strings_ea ON strings(ea);

CREATE VIRTUAL TABLE IF NOT EXISTS strings_fts USING fts5(
	value,
	content='strings',
	content_rowid='str_id',
	tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS elf_symbols(
	sym_idx INTEGER PRIMARY KEY,
	ea INTEGER NOT NULL,
	size INTEGER NOT NULL,
	name TEXT NOT NULL,
	sym_type INTEGER NOT NULL,
	sym_bind INTEGER NOT NULL,
	sec_idx INTEGER NOT NULL,
	table_kind TEXT NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_elf_symbols_ea ON elf_symbols(ea);
CREATE INDEX IF NOT EXISTS idx_elf_symbols_name ON elf_symbols(name) WHERE name != '';

CREATE TABLE IF NOT EXISTS dwarf_funcs(
	ea INTEGER PRIMARY KEY,
	size INTEGER NOT NULL,
	name TEXT NOT NULL,
	linkage_name TEXT NOT NULL DEFAULT '',
	class_name TEXT NOT NULL DEFAULT '',
	decl_file_id INTEGER,
	decl_line INTEGER NOT NULL DEFAULT 0,
	low_line INTEGER NOT NULL DEFAULT 0,
	high_line INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX IF NOT EXISTS idx_dwarf_funcs_name ON dwarf_funcs(name);
CREATE INDEX IF NOT EXISTS idx_dwarf_funcs_linkage ON dwarf_funcs(linkage_name) WHERE linkage_name != '';
CREATE INDEX IF NOT EXISTS idx_dwarf_funcs_class ON dwarf_funcs(class_name) WHERE class_name != '';

CREATE VIRTUAL TABLE IF NOT EXISTS dwarf_funcs_fts USING fts5(
	name,
	linkage_name,
	class_name,
	content='dwarf_funcs',
	content_rowid='ea',
	tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS dwarf_func_ranges(
	start_ea INTEGER PRIMARY KEY,
	end_ea INTEGER NOT NULL,
	func_ea INTEGER NOT NULL
) STRICT;
CREATE INDEX IF NOT EXISTS idx_dwarf_func_ranges_func ON dwarf_func_ranges(func_ea);

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
`

var trackedBronzeTables = []string{
	"sections", "relocs", "imports", "exports", "strings",
	"elf_symbols", "dwarf_funcs", "dwarf_func_ranges",
	"types", "type_members", "enum_values", "func_vars",
	"source_files", "names", "inlines",
}

func InitBronzeSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, bronzeSchemaDDL); err != nil {
		return fmt.Errorf("create bronze schema: %w", err)
	}
	return nil
}

func RebuildBronzeFTS(ctx context.Context, db *sql.DB) error {
	for _, table := range []string{"strings_fts", "dwarf_funcs_fts", "types_fts"} {
		stmt := fmt.Sprintf("INSERT INTO %s(%s) VALUES('rebuild');", table, table)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("rebuild %s: %w", table, err)
		}
	}
	return nil
}

func RefreshBronzeCounts(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	counts := make(map[string]int64, len(trackedBronzeTables))
	for _, tbl := range trackedBronzeTables {
		var n int64
		if err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, tbl)).Scan(&n); err != nil {
			return nil, fmt.Errorf("count bronze table %s: %w", tbl, err)
		}
		counts[tbl] = n
		if _, err := db.ExecContext(ctx,
			`INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			"count_"+tbl, strconv.FormatInt(n, 10),
		); err != nil {
			return nil, fmt.Errorf("store meta count_%s: %w", tbl, err)
		}
	}
	return counts, nil
}
