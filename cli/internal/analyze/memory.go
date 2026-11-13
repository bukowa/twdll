package analyze

import (
	"fmt"

	"twdll/cli/internal/workspace"
)

const giB = uint64(1024 * 1024 * 1024)

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

func ComputeGhidraMaxMem(mem SystemMemory, target workspace.TargetID, concurrentJobs int) string {
	if mem.TotalPhysBytes == 0 && mem.AvailPhysBytes == 0 {
		if target == workspace.TargetAttila64 {
			return "16G"
		}
		return "8G"
	}

	if concurrentJobs <= 0 {
		concurrentJobs = 1
	}

	totalBudgetGB := 16
	if mem.TotalPhysBytes > 0 {
		physGB := mem.PhysGB()
		commitGB := mem.CommitGB()
		totalBudgetGB = (physGB * 3) / 4
		if physGB <= 16 && commitGB >= 24 && totalBudgetGB < 14 {
			totalBudgetGB = 14
		}
	}

	if target == workspace.TargetAttila64 {
		xmxGB := clampInt(totalBudgetGB, 8, 48)
		if mem.AvailPhysBytes > 0 {
			availCap := max(8, int(mem.AvailPhysBytes/giB)-2)
			xmxGB = clampInt(min(xmxGB, availCap/concurrentJobs), 8, 48)
		}
		return fmt.Sprintf("%dG", xmxGB)
	}

	xmxGB := clampInt(totalBudgetGB/concurrentJobs, 4, 8)
	if mem.AvailPhysBytes > 0 {
		availCap := max(4, int(mem.AvailPhysBytes/giB)-2)
		xmxGB = clampInt(min(xmxGB, availCap/concurrentJobs), 4, 8)
	}
	return fmt.Sprintf("%dG", xmxGB)
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
	workers := max(1, min(3, mem.PhysGB()/8))
	if mem.AvailPhysBytes > 0 {
		availGB := int(mem.AvailPhysBytes / giB)
		byAvail := max(1, min(3, max(0, availGB-2)/4))
		if mem.TotalPhysBytes > 0 {
			workers = min(workers, byAvail)
		} else {
			workers = byAvail
		}
	}
	return min(numItems, workers)
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
