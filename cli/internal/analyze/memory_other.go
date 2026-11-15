//go:build !windows

package analyze

import "os"

func ProbeSystemMemory() SystemMemory {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return SystemMemory{}
	}
	return parseProcMeminfo(string(data))
}
