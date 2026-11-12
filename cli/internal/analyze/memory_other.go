//go:build !windows

package analyze

func getSystemMemory() (SystemMemory, error) {
	return SystemMemory{}, nil
}
