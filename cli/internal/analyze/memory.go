package analyze

import (
	"fmt"
	"strconv"
	"strings"

	"twdll/cli/internal/workspace"
)

const (
	giB                           = uint64(1024 * 1024 * 1024)
	warnGhidraAttila64PhysGiB     = 13 // 8-12 GB hosts warn & rely on OS swap; >=13 GiB covers 16 GB iGPU & desktop PCs
	warnGhidraAttila64FullPhysGiB = 28 // --full-analysis keeps AddressTable cascade enabled
	warnGhidra32PhysGiB           = 7  // <7 GiB hosts warn on 32-bit Ghidra
	osReserveGiB                  = 4
	ghidra32WorkerBudgetGiB       = 7 // ~6G heap + ~0.6-1G native decompile.exe overhead -> 1w@<=16G, 2w@24G, 3w@32G+
)

type SystemMemory struct {
	TotalPhysBytes   uint64
	AvailPhysBytes   uint64
	TotalCommitBytes uint64
	AvailCommitBytes uint64
}

func (m SystemMemory) PhysGB() int {
	if m.TotalPhysBytes == 0 {
		return 0
	}
	gb := int((m.TotalPhysBytes + giB/2) / giB)
	if gb < 1 {
		return 1
	}
	return gb
}

func (m SystemMemory) CommitGB() int {
	if m.TotalCommitBytes == 0 {
		return 0
	}
	return int((m.TotalCommitBytes + giB/2) / giB)
}

func ComputeGhidraMaxMem(mem SystemMemory, target workspace.TargetID, fullAnalysis bool, concurrentJobs int) string {
	if mem.TotalPhysBytes == 0 && mem.AvailPhysBytes == 0 {
		if target == workspace.TargetAttila64 {
			if fullAnalysis {
				return "28G"
			}
			return "12G"
		}
		return "6G"
	}

	if target == workspace.TargetAttila64 {
		baseGB := mem.PhysGB()
		if baseGB == 0 {
			baseGB = int(mem.AvailPhysBytes / giB)
		}
		if fullAnalysis {
			return fmt.Sprintf("%dG", clampInt(baseGB-6, 22, 36))
		}
		return fmt.Sprintf("%dG", clampInt(baseGB-6, 8, 16))
	}

	concurrentJobs = max(1, concurrentJobs)
	perWorker := 6
	if mem.TotalPhysBytes > 0 {
		perWorker = (max(6, mem.PhysGB()-osReserveGiB) / concurrentJobs) - 1
	}
	if mem.AvailPhysBytes > 0 {
		availPerWorker := (max(0, int(mem.AvailPhysBytes/giB)-osReserveGiB) / concurrentJobs) - 1
		if mem.TotalPhysBytes > 0 {
			perWorker = min(perWorker, max(5, availPerWorker))
		} else {
			perWorker = availPerWorker
		}
	}
	return fmt.Sprintf("%dG", clampInt(perWorker, 5, 6))
}

func resolveIDA32WorkerCount(numItems, explicitJobs, numCPU int, mem SystemMemory) int {
	if numItems <= 0 {
		return 0
	}
	if explicitJobs > 0 {
		return min(numItems, explicitJobs)
	}
	byCPU := max(1, min(3, numCPU/2))
	if mem.AvailPhysBytes > 0 {
		availGB := int(mem.AvailPhysBytes / giB)
		byAvail := max(1, min(3, max(0, availGB-2)/2))
		byCPU = min(byCPU, byAvail)
	} else if mem.TotalPhysBytes > 0 {
		byCPU = min(byCPU, max(1, min(3, mem.PhysGB()/4)))
	}
	return min(numItems, byCPU)
}

func resolveGhidra32WorkerCount(numItems, explicitJobs int, mem SystemMemory) int {
	if numItems <= 0 {
		return 0
	}
	if explicitJobs > 0 {
		return min(numItems, explicitJobs)
	}
	if mem.TotalPhysBytes == 0 && mem.AvailPhysBytes == 0 {
		return min(numItems, 2)
	}
	workers := 3
	if mem.TotalPhysBytes > 0 {
		workers = min(workers, max(1, (mem.PhysGB()-osReserveGiB)/ghidra32WorkerBudgetGiB))
	}
	if mem.AvailPhysBytes > 0 {
		workers = min(workers, max(1, (int(mem.AvailPhysBytes/giB)-osReserveGiB)/ghidra32WorkerBudgetGiB))
	}
	return min(numItems, workers)
}

func CollectGhidraMemoryWarnings(mem SystemMemory, toRun []PlanItem, opts Options) []string {
	if strings.TrimSpace(opts.MaxMem) != "" || mem.TotalPhysBytes == 0 {
		return nil
	}

	var hasHeavyAttila64, has32Bit bool
	for _, item := range toRun {
		if !item.NeedsRun {
			continue
		}
		if item.Target == workspace.TargetAttila64 && item.Traits.IsHeavyDWARF {
			hasHeavyAttila64 = true
		}
		if !item.Target.Is64Bit() {
			has32Bit = true
		}
	}

	physGiB := float64(mem.TotalPhysBytes) / float64(giB)
	var warnings []string

	if hasHeavyAttila64 {
		maxMem64 := ComputeGhidraMaxMem(mem, workspace.TargetAttila64, opts.FullAnalysis, 1)
		if !opts.FullAnalysis && mem.TotalPhysBytes < warnGhidraAttila64PhysGiB*giB {
			warnings = append(warnings, fmt.Sprintf(
				"Low physical RAM detected (%.1f GiB): attila_64 Ghidra analysis will proceed with -Xmx%s relying on OS swap/pagefile (ensure a system-managed pagefile/swap is enabled; --decompiler ida is an optional faster alternative if available).",
				physGiB, maxMem64,
			))
		} else if opts.FullAnalysis && mem.TotalPhysBytes < warnGhidraAttila64FullPhysGiB*giB {
			warnings = append(warnings, fmt.Sprintf(
				"Low physical RAM detected (%.1f GiB) for --full-analysis: attila_64 Ghidra analysis will proceed with -Xmx%s via OS swap/pagefile; omitting --full-analysis keeps all DWARF types/signatures within ~10.6 GiB RAM.",
				physGiB, maxMem64,
			))
		}
	}

	if has32Bit && mem.TotalPhysBytes < warnGhidra32PhysGiB*giB {
		warnings = append(warnings, fmt.Sprintf(
			"Low physical RAM detected (%.1f GiB): 32-bit Ghidra analysis will proceed with -Xmx5G relying on OS swap/pagefile.",
			physGiB,
		))
	}

	return warnings
}

func parseProcMeminfo(raw string) SystemMemory {
	var mem SystemMemory
	var commitLimit, committedAS uint64
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		valKB, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		valBytes := valKB * 1024
		switch fields[0] {
		case "MemTotal:":
			mem.TotalPhysBytes = valBytes
		case "MemAvailable:":
			mem.AvailPhysBytes = valBytes
		case "CommitLimit:":
			commitLimit = valBytes
		case "Committed_AS:":
			committedAS = valBytes
		}
	}
	mem.TotalCommitBytes = commitLimit
	if commitLimit > committedAS {
		mem.AvailCommitBytes = commitLimit - committedAS
	}
	return mem
}

func clampInt(val, low, high int) int {
	if val < low {
		return low
	}
	if val > high {
		return high
	}
	return val
}
