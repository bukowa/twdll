package binaries

import (
	"crypto/sha256"
	"debug/elf"
	"debug/pe"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"twdll/cli/internal/workspace"
)

// Known official Steam release SHA-256 digests (lowercase).
var KnownOfficialHashes = map[workspace.TargetID]map[string]string{
	workspace.TargetAttila64: {
		"latest": "217e14350c26d5edfd4e4205516b3498e45e4b6c675923c0e0b0df7190fd8784",
	},
	workspace.TargetAttila32: {
		"latest": "8a40cbcff108e1cba14cc7eb6c7f4dfb054c7745b236d1bfc0fde5c85e2a353d",
	},
	workspace.TargetRome2_32: {
		"latest": "07cb2215b80591224a5f8889963344d560492f09c4d22696c988b957ca49b2b7",
	},
	workspace.TargetToB32: {
		"latest": "89b7d958d2f8906d301b4be3cbe7b36fe7a544c90ce5c92c7fbaa319c1427a03",
	},
}

// PE32Info contains the verification details for a 32-bit Windows PE binary.
type PE32Info struct {
	Path          string
	Machine       uint16
	SHA256        string
	KnownOfficial bool
}

// ELF64Info contains the verification details for a 64-bit Linux ELF binary.
type ELF64Info struct {
	Path          string
	HasDWARF      bool
	SHA256        string
	KnownOfficial bool
}

// ComputeFileSHA256 streams the file at path through SHA-256 and returns the lowercase hex string.
func ComputeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open for hash %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyPE32 verifies that path is a valid 32-bit Windows x86 PE executable and computes its SHA-256.
func VerifyPE32(path string, target workspace.TargetID) (*PE32Info, error) {
	peFile, err := pe.Open(path)
	if err != nil {
		return nil, fmt.Errorf("invalid PE binary %s: %w", path, err)
	}
	defer peFile.Close()

	if peFile.FileHeader.Machine != pe.IMAGE_FILE_MACHINE_I386 {
		return nil, fmt.Errorf("expected 32-bit x86 PE binary (machine 0x014c), got machine 0x%04x", peFile.FileHeader.Machine)
	}

	hash, err := ComputeFileSHA256(path)
	if err != nil {
		return nil, err
	}

	isOfficial := false
	if known, ok := KnownOfficialHashes[target]; ok {
		for _, officialHash := range known {
			if officialHash != "" && strings.EqualFold(hash, officialHash) {
				isOfficial = true
				break
			}
		}
	}

	return &PE32Info{
		Path:          path,
		Machine:       peFile.FileHeader.Machine,
		SHA256:        hash,
		KnownOfficial: isOfficial,
	}, nil
}

// VerifyELF64DWARF verifies that path is a 64-bit x86-64 ELF with DWARF debug info (.debug_info) and computes its SHA-256.
func VerifyELF64DWARF(path string) (*ELF64Info, error) {
	elfFile, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("invalid ELF binary %s: %w", path, err)
	}
	defer elfFile.Close()

	if elfFile.Class != elf.ELFCLASS64 {
		return nil, fmt.Errorf("expected 64-bit ELF (ELFCLASS64), got %v", elfFile.Class)
	}
	if elfFile.Machine != elf.EM_X86_64 {
		return nil, fmt.Errorf("expected x86-64 ELF architecture (EM_X86_64), got %v", elfFile.Machine)
	}

	debugInfoSec := elfFile.Section(".debug_info")
	if debugInfoSec == nil || debugInfoSec.Size == 0 {
		return nil, fmt.Errorf("ELF binary %s does not contain a non-empty .debug_info DWARF section (stripped)", path)
	}

	hash, err := ComputeFileSHA256(path)
	if err != nil {
		return nil, err
	}

	isOfficial := false
	if known, ok := KnownOfficialHashes[workspace.TargetAttila64]; ok {
		for _, officialHash := range known {
			if officialHash != "" && strings.EqualFold(hash, officialHash) {
				isOfficial = true
				break
			}
		}
	}

	return &ELF64Info{
		Path:          path,
		HasDWARF:      true,
		SHA256:        hash,
		KnownOfficial: isOfficial,
	}, nil
}
