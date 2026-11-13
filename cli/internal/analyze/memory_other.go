//go:build !windows

package analyze

func ProbeSystemMemory() SystemMemory {
	return SystemMemory{}
}
