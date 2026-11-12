package analyze

import (
	"runtime"
	"testing"

	"twdll/cli/internal/workspace"
)

func TestComputeGhidraMaxMem(t *testing.T) {
	const gb = 1024 * 1024 * 1024

	tests := []struct {
		name           string
		mem            SystemMemory
		target         workspace.TargetID
		concurrentJobs int
		want           string
	}{
		{
			name:           "fallback zero memory 64bit",
			mem:            SystemMemory{},
			target:         workspace.TargetAttila64,
			concurrentJobs: 1,
			want:           "16G",
		},
		{
			name:           "fallback zero memory 32bit",
			mem:            SystemMemory{},
			target:         workspace.TargetAttila32,
			concurrentJobs: 2,
			want:           "8G",
		},
		{
			name: "16GB host with large pagefile attila_64",
			mem: SystemMemory{
				TotalPhysBytes:   16 * gb,
				TotalCommitBytes: 32 * gb,
			},
			target:         workspace.TargetAttila64,
			concurrentJobs: 1,
			want:           "14G",
		},
		{
			name: "16GB host small pagefile attila_64",
			mem: SystemMemory{
				TotalPhysBytes:   16 * gb,
				TotalCommitBytes: 18 * gb,
			},
			target:         workspace.TargetAttila64,
			concurrentJobs: 1,
			want:           "12G",
		},
		{
			name: "32GB host attila_64",
			mem: SystemMemory{
				TotalPhysBytes:   32 * gb,
				TotalCommitBytes: 48 * gb,
			},
			target:         workspace.TargetAttila64,
			concurrentJobs: 1,
			want:           "24G",
		},
		{
			name: "64GB host attila_64",
			mem: SystemMemory{
				TotalPhysBytes:   64 * gb,
				TotalCommitBytes: 74 * gb,
			},
			target:         workspace.TargetAttila64,
			concurrentJobs: 1,
			want:           "48G",
		},
		{
			name: "16GB host 32bit 2 jobs",
			mem: SystemMemory{
				TotalPhysBytes:   16 * gb,
				TotalCommitBytes: 20 * gb,
			},
			target:         workspace.TargetAttila32,
			concurrentJobs: 2,
			want:           "6G",
		},
		{
			name: "64GB host 32bit 2 jobs capped at 8G",
			mem: SystemMemory{
				TotalPhysBytes:   64 * gb,
				TotalCommitBytes: 74 * gb,
			},
			target:         workspace.TargetAttila32,
			concurrentJobs: 2,
			want:           "8G",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeGhidraMaxMem(tc.mem, tc.target, tc.concurrentJobs)
			if got != tc.want {
				t.Fatalf("ComputeGhidraMaxMem() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetSystemMemoryWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only memory status check")
	}
	mem, err := getSystemMemory()
	if err != nil {
		t.Fatalf("getSystemMemory() failed: %v", err)
	}
	if mem.TotalPhysBytes == 0 || mem.TotalCommitBytes == 0 {
		t.Fatalf("expected non-zero system memory stats, got %+v", mem)
	}
}
