package silver

import _ "embed"

//go:embed scripts/TwdllSilverExport.java
var twdllSilverExportJava string

//go:embed scripts/TwdllDecompExport.java
var twdllDecompExportJava string
