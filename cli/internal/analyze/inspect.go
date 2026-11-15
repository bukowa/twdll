package analyze

import (
	"debug/elf"
	"debug/pe"
	"os"
	"path/filepath"
	"strings"
)

const heavyDWARFThresholdBytes = 100 * 1024 * 1024 // 100 MiB

type BinaryTraits struct {
	Format        string // "elf", "pe", or "unknown"
	FileSizeBytes int64
	TextBytes     uint64
	DWARFBytes    uint64
	HasDWARF      bool
	IsHeavyDWARF  bool
	HasLocalPDB   bool
}

func InspectBinary(binaryPath, sourcePath string) BinaryTraits {
	info, err := os.Stat(binaryPath)
	if err != nil || info.IsDir() {
		return BinaryTraits{Format: "unknown"}
	}

	traits := BinaryTraits{
		Format:        "unknown",
		FileSizeBytes: info.Size(),
	}

	if ef, err := elf.Open(binaryPath); err == nil {
		defer func() { _ = ef.Close() }()
		traits.Format = "elf"
		for _, sec := range ef.Sections {
			if sec == nil {
				continue
			}
			if sec.Name == ".text" {
				traits.TextBytes += sec.Size
			}
			if strings.HasPrefix(sec.Name, ".debug_") || strings.HasPrefix(sec.Name, ".zdebug_") {
				traits.DWARFBytes += sec.Size
			}
		}
		traits.HasDWARF = traits.DWARFBytes > 0
		traits.IsHeavyDWARF = traits.DWARFBytes >= heavyDWARFThresholdBytes
		return traits
	}

	if pf, err := pe.Open(binaryPath); err == nil {
		defer func() { _ = pf.Close() }()
		traits.Format = "pe"
		for _, sec := range pf.Sections {
			if sec == nil {
				continue
			}
			if strings.TrimRight(sec.Name, "\x00") == ".text" {
				if sec.VirtualSize > 0 {
					traits.TextBytes += uint64(sec.VirtualSize)
				} else {
					traits.TextBytes += uint64(sec.Size)
				}
			}
		}
		traits.HasLocalPDB = hasSidecarPDB(binaryPath) || hasSidecarPDB(sourcePath)
		return traits
	}

	return traits
}

func hasSidecarPDB(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	pdbPath := strings.TrimSuffix(p, filepath.Ext(p)) + ".pdb"
	if info, err := os.Stat(pdbPath); err == nil && !info.IsDir() && info.Size() > 0 {
		return true
	}
	dir := filepath.Dir(pdbPath)
	wantName := filepath.Base(pdbPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), wantName) {
			if fi, err := e.Info(); err == nil && fi.Size() > 0 {
				return true
			}
		}
	}
	return false
}
