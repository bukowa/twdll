package silver

import _ "embed"

//go:embed scripts/export_ida_silver.py
var exportIDASilverPy string

//go:embed scripts/export_ida_decomp.py
var exportIDADecompPy string
