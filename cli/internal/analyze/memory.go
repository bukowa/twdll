package analyze

import (
	"fmt"

	"twdll/cli/internal/workspace"
)

// SystemMemory captures physical RAM and virtual commit capacity reported by the OS.
type SystemMemory struct {
	TotalPhysBytes   uint64
	AvailPhysBytes   uint64
	TotalCommitBytes uint64 // Physical RAM + pagefile / swap commit limit
	AvailCommitBytes uint64
}

// ComputeGhidraMaxMem determines the optimal JVM max heap string (e.g. "14G", "48G")
// for the given target and concurrency budget, using physical RAM and pagefile headroom.
func ComputeGhidraMaxMem(mem SystemMemory, target workspace.TargetID, concurrentJobs int) string {
	const gb = 1024 * 1024 * 1024

	if mem.TotalPhysBytes == 0 {
		if target == workspace.TargetAttila64 {
			return "16G"
		}
		return "8G"
	}

	physGB := int(mem.TotalPhysBytes / gb)
	commitGB := int(mem.TotalCommitBytes / gb)

	if concurrentJobs < 1 {
		concurrentJobs = 1
	}

	if target == workspace.TargetAttila64 {
		// attila_64 is a 758 MB ELF with complete DWARF info; give it generous headroom
		// (~75% of physical RAM, leveraging Windows pagefile when physical RAM <= 16 GB).
		budgetGB := physGB * 3 / 4
		if physGB <= 16 && commitGB >= 24 && budgetGB < 14 {
			budgetGB = 14
		}
		if budgetGB < 8 {
			budgetGB = 8
		}
		if budgetGB > 48 {
			budgetGB = 48
		}
		return fmt.Sprintf("%dG", budgetGB)
	}

	// 32-bit targets (~30 MB DLLs) analyze reliably in 6-8 GB per worker.
	totalBudgetGB := physGB * 3 / 4
	if physGB <= 16 && commitGB >= 24 && totalBudgetGB < 14 {
		totalBudgetGB = 14
	}
	perJobGB := totalBudgetGB / concurrentJobs
	if perJobGB < 4 {
		perJobGB = 4
	}
	if perJobGB > 8 {
		perJobGB = 8
	}
	return fmt.Sprintf("%dG", perJobGB)
}
