package binaries

import (
	"debug/elf"
	"debug/pe"
	"fmt"
	"path/filepath"
	"strings"

	"twdll/cli/internal/workspace"
)

var KnownOfficialHashes = map[workspace.TargetID]string{
	workspace.TargetAttila64: "217e14350c26d5edfd4e4205516b3498e45e4b6c675923c0e0b0df7190fd8784",
	workspace.TargetAttila32: "8a40cbcff108e1cba14cc7eb6c7f4dfb054c7745b236d1bfc0fde5c85e2a353d",
	workspace.TargetRome2_32: "07cb2215b80591224a5f8889963344d560492f09c4d22696c988b957ca49b2b7",
	workspace.TargetToB32:    "89b7d958d2f8906d301b4be3cbe7b36fe7a544c90ce5c92c7fbaa319c1427a03",
}

func IsKnownOfficialHash(target workspace.TargetID, sha256Hex string) bool {
	expected, ok := KnownOfficialHashes[target]
	if !ok || expected == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(sha256Hex), expected)
}

type PE32Info struct {
	Path          string
	Machine       uint16
	SHA256        string
	KnownOfficial bool
}

func VerifyPE32(path string, target workspace.TargetID) (PE32Info, error) {
	cleanPath := filepath.Clean(path)
	f, err := pe.Open(cleanPath)
	if err != nil {
		return PE32Info{}, fmt.Errorf("open PE binary %s: %w", cleanPath, err)
	}
	machine := f.FileHeader.Machine
	_ = f.Close()

	if machine != pe.IMAGE_FILE_MACHINE_I386 {
		return PE32Info{}, fmt.Errorf("invalid PE machine 0x%x in %s (expected 32-bit i386 0x%x)", machine, cleanPath, pe.IMAGE_FILE_MACHINE_I386)
	}

	digest, err := workspace.ComputeFileSHA256(cleanPath)
	if err != nil {
		return PE32Info{}, fmt.Errorf("hash PE32 %s: %w", cleanPath, err)
	}

	return PE32Info{
		Path:          filepath.ToSlash(cleanPath),
		Machine:       machine,
		SHA256:        digest,
		KnownOfficial: IsKnownOfficialHash(target, digest),
	}, nil
}

type ELF64Info struct {
	Path          string
	SHA256        string
	KnownOfficial bool
}

func VerifyELF64DWARF(path string) (ELF64Info, error) {
	cleanPath := filepath.Clean(path)
	f, err := elf.Open(cleanPath)
	if err != nil {
		return ELF64Info{}, fmt.Errorf("open ELF binary %s: %w", cleanPath, err)
	}
	cls := f.Class
	machine := f.Machine
	debugSec := f.Section(".debug_info")
	var debugSize uint64
	if debugSec != nil {
		debugSize = debugSec.Size
	}
	_ = f.Close()

	if cls != elf.ELFCLASS64 || machine != elf.EM_X86_64 {
		return ELF64Info{}, fmt.Errorf("invalid ELF header (%v, %v) in %s (expected ELFCLASS64 EM_X86_64)", cls, machine, cleanPath)
	}
	if debugSec == nil || debugSize == 0 {
		return ELF64Info{}, fmt.Errorf("ELF binary %s is missing non-empty .debug_info DWARF symbols", cleanPath)
	}

	digest, err := workspace.ComputeFileSHA256(cleanPath)
	if err != nil {
		return ELF64Info{}, fmt.Errorf("hash ELF64 %s: %w", cleanPath, err)
	}

	return ELF64Info{
		Path:          filepath.ToSlash(cleanPath),
		SHA256:        digest,
		KnownOfficial: IsKnownOfficialHash(workspace.TargetAttila64, digest),
	}, nil
}
