package binaries

import (
	"bytes"
	"debug/elf"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"twdll/cli/internal/workspace"
)

func TestParseVDF_HandlesCommentsEscapesAndInsertionOrder(t *testing.T) {
	// Given a Valve KeyValues document with comments, escapes, bare words, and nested sections
	raw := `
// Top comment
"LibraryFolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"label"		"Main\tDrive\nLine2"
	}
	"1"
	{
		"path"		"G:\\SteamLibrary"
		bare_flag	42
	}
}
`

	// When ParseVDF parses the text
	root, err := ParseVDF(raw)
	if err != nil {
		t.Fatalf("ParseVDF failed: %v", err)
	}

	// Then case-insensitive lookup, escapes, integer parsing, and child order work
	lf := root.Get("libraryfolders")
	if lf == nil {
		t.Fatalf("expected case-insensitive lookup for libraryfolders")
	}
	children := lf.ChildrenOrdered()
	if len(children) != 2 || children[0].Name != "0" || children[1].Name != "1" {
		t.Fatalf("unexpected children order: %+v", children)
	}
	if got := children[0].GetString("PATH", ""); got != `C:\Program Files (x86)\Steam` {
		t.Fatalf("escaped path = %q, want C:\\Program Files (x86)\\Steam", got)
	}
	if got := children[0].GetString("label", ""); got != "Main\tDrive\nLine2" {
		t.Fatalf("escaped label = %q", got)
	}
	if got := children[1].GetInt("bare_flag", 0); got != 42 {
		t.Fatalf("bare_flag = %d, want 42", got)
	}
}

func TestSteamDir_FindGameDir_FiltersByStateFlagsAndBinaryPresence(t *testing.T) {
	// Given a temporary Steam installation with two libraries and depot content
	steamRoot := t.TempDir()
	lib2 := t.TempDir()
	t.Setenv("STEAM_DIR", steamRoot)

	if err := os.MkdirAll(filepath.Join(steamRoot, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(lib2, "steamapps", "common", "Total War Attila"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(lib2, "steamapps", "common", "Total War Rome II"), 0o755); err != nil {
		t.Fatal(err)
	}

	vdfText := `"libraryfolders" { "0" { "path" "` + filepath.ToSlash(steamRoot) + `" } "1" { "path" "` + filepath.ToSlash(lib2) + `" } }`
	if err := os.WriteFile(filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"), []byte(vdfText), 0o644); err != nil {
		t.Fatal(err)
	}

	attilaACF := `"AppState" { "appid" "325610" "StateFlags" "4" "installdir" "Total War Attila" }`
	if err := os.WriteFile(filepath.Join(lib2, "steamapps", "appmanifest_325610.acf"), []byte(attilaACF), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib2, "steamapps", "common", "Total War Attila", "empire.retail.dll"), []byte("pe"), 0o644); err != nil {
		t.Fatal(err)
	}

	rome2CorruptACF := `"AppState" { "appid" "214950" "StateFlags" "36" "installdir" "Total War Rome II" }`
	if err := os.WriteFile(filepath.Join(lib2, "steamapps", "appmanifest_214950.acf"), []byte(rome2CorruptACF), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib2, "steamapps", "common", "Total War Rome II", "empire.retail.dll"), []byte("pe"), 0o644); err != nil {
		t.Fatal(err)
	}

	depotBin := filepath.Join(steamRoot, "steamapps", "content", "app_325610", "depot_325624", "Attila")
	if err := os.MkdirAll(filepath.Dir(depotBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(depotBin, []byte("elf"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When LocateSteam, FindGameDir, and FindAttila64Depot are called
	sd, err := LocateSteam()
	if err != nil {
		t.Fatalf("LocateSteam: %v", err)
	}
	attilaDir, attilaOK := sd.FindGameDir(workspace.GameAttila)
	_, rome2OK := sd.FindGameDir(workspace.GameRome2)
	gotDepot, depotOK := sd.FindAttila64Depot()

	// Then Attila resolves from the secondary library, corrupt Rome 2 is rejected, and the 64-bit depot is found
	wantAttila := filepath.ToSlash(filepath.Join(lib2, "steamapps", "common", "Total War Attila"))
	if !attilaOK || attilaDir != wantAttila {
		t.Fatalf("FindGameDir(Attila) = (%q, %v), want (%q, true)", attilaDir, attilaOK, wantAttila)
	}
	if rome2OK {
		t.Fatalf("expected corrupt StateFlags=36 Rome 2 install to be rejected")
	}
	if !depotOK || gotDepot != filepath.ToSlash(depotBin) {
		t.Fatalf("FindAttila64Depot = (%q, %v), want (%q, true)", gotDepot, depotOK, filepath.ToSlash(depotBin))
	}
	if DepotCommand() != "download_depot 325610 325624 3804137149572368482" {
		t.Fatalf("unexpected DepotCommand: %q", DepotCommand())
	}
}

func TestVerifyPE32_AcceptsI386AndRejectsAMD64(t *testing.T) {
	// Given synthetic i386 and amd64 PE files
	dir := t.TempDir()
	pe32Path := filepath.Join(dir, "i386.dll")
	pe64Path := filepath.Join(dir, "amd64.dll")
	if err := os.WriteFile(pe32Path, buildSyntheticPE(pe.IMAGE_FILE_MACHINE_I386), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pe64Path, buildSyntheticPE(pe.IMAGE_FILE_MACHINE_AMD64), 0o644); err != nil {
		t.Fatal(err)
	}

	// When VerifyPE32 inspects both binaries
	info, err := VerifyPE32(pe32Path, workspace.TargetAttila32)
	_, err64 := VerifyPE32(pe64Path, workspace.TargetAttila32)

	// Then i386 succeeds with a computed SHA-256 while amd64 fails
	if err != nil {
		t.Fatalf("VerifyPE32(i386) failed: %v", err)
	}
	if info.Machine != pe.IMAGE_FILE_MACHINE_I386 || len(info.SHA256) != 64 || info.KnownOfficial {
		t.Fatalf("unexpected PE32Info: %+v", info)
	}
	if err64 == nil {
		t.Fatalf("expected VerifyPE32 to reject amd64 binary")
	}
	if !IsKnownOfficialHash(workspace.TargetAttila32, KnownOfficialHashes[workspace.TargetAttila32]) {
		t.Fatalf("expected KnownOfficialHashes[TargetAttila32] to be recognized")
	}
}

func TestVerifyELF64DWARF_AcceptsDebugInfoAndRejectsStripped(t *testing.T) {
	// Given a synthetic 64-bit x86_64 ELF with .debug_info and a stripped one without it
	dir := t.TempDir()
	dwarfPath := filepath.Join(dir, "Attila.dwarf")
	strippedPath := filepath.Join(dir, "Attila.stripped")
	if err := os.WriteFile(dwarfPath, buildSyntheticELF64(true), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(strippedPath, buildSyntheticELF64(false), 0o644); err != nil {
		t.Fatal(err)
	}

	// When VerifyELF64DWARF validates both files
	info, err := VerifyELF64DWARF(dwarfPath)
	_, strippedErr := VerifyELF64DWARF(strippedPath)

	// Then the DWARF binary is accepted and the stripped binary is rejected
	if err != nil {
		t.Fatalf("VerifyELF64DWARF(dwarf) failed: %v", err)
	}
	if len(info.SHA256) != 64 || info.KnownOfficial {
		t.Fatalf("unexpected ELF64Info: %+v", info)
	}
	if strippedErr == nil {
		t.Fatalf("expected VerifyELF64DWARF to reject stripped ELF64 without .debug_info")
	}
}

func buildSyntheticPE(machine uint16) []byte {
	buf := make([]byte, 128)
	buf[0] = 'M'
	buf[1] = 'Z'
	binary.LittleEndian.PutUint32(buf[0x3c:], 0x40)
	copy(buf[0x40:], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(buf[0x44:], machine)
	return buf
}

func buildSyntheticELF64(withDebugInfo bool) []byte {
	shstrtab := []byte("\x00.shstrtab\x00.debug_info\x00")
	debugPayload := []byte{1, 2, 3, 4}

	var hdr elf.Header64
	hdr.Ident[0] = 0x7f
	hdr.Ident[1] = 'E'
	hdr.Ident[2] = 'L'
	hdr.Ident[3] = 'F'
	hdr.Ident[elf.EI_CLASS] = byte(elf.ELFCLASS64)
	hdr.Ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	hdr.Ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	hdr.Type = uint16(elf.ET_EXEC)
	hdr.Machine = uint16(elf.EM_X86_64)
	hdr.Version = uint32(elf.EV_CURRENT)
	hdr.Ehsize = 64
	hdr.Shentsize = 64
	hdr.Shoff = 64
	hdr.Shstrndx = 1
	numSec := uint16(2)
	if withDebugInfo {
		numSec = 3
	}
	hdr.Shnum = numSec

	dataStart := uint64(64 + int(numSec)*64)
	shstrSec := elf.Section64{
		Name:      1,
		Type:      uint32(elf.SHT_STRTAB),
		Off:       dataStart,
		Size:      uint64(len(shstrtab)),
		Addralign: 1,
	}

	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, &hdr)
	_ = binary.Write(&out, binary.LittleEndian, &elf.Section64{})
	_ = binary.Write(&out, binary.LittleEndian, &shstrSec)
	if withDebugInfo {
		dbgSec := elf.Section64{
			Name:      11,
			Type:      uint32(elf.SHT_PROGBITS),
			Off:       dataStart + uint64(len(shstrtab)),
			Size:      uint64(len(debugPayload)),
			Addralign: 1,
		}
		_ = binary.Write(&out, binary.LittleEndian, &dbgSec)
	}
	out.Write(shstrtab)
	if withDebugInfo {
		out.Write(debugPayload)
	}
	return out.Bytes()
}
