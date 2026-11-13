package toolchain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ValidateIDADir(dir string) error {
	_, err := FindIDAExecutable(dir, true)
	return err
}

func FindIDAExecutable(idaDir string, is64Bit bool) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(idaDir))
	if clean == "" || clean == "." {
		return "", errors.New("IDA directory path is empty")
	}
	candidates := []string{"idat.exe", "ida.exe", "idat64.exe", "ida64.exe", "idat", "ida", "idat64", "ida64"}
	if is64Bit {
		candidates = []string{"idat64.exe", "idat.exe", "ida64.exe", "ida.exe", "idat64", "idat", "ida64", "ida"}
	}
	for _, name := range candidates {
		full := filepath.Join(clean, name)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return filepath.ToSlash(full), nil
		}
	}
	return "", fmt.Errorf("no IDA executable (idat/ida) found in %s", clean)
}

func AutoDetectIDADir() string {
	roots := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), `C:\Program Files`, `C:\Program Files (x86)`}
	var matches []string
	seen := map[string]bool{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(strings.ToUpper(entry.Name()), "IDA") {
				continue
			}
			full := filepath.Join(root, entry.Name())
			key := strings.ToLower(filepath.Clean(full))
			if !seen[key] && ValidateIDADir(full) == nil {
				seen[key] = true
				matches = append(matches, filepath.ToSlash(full))
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return compareIDAPathsDesc(matches[i], matches[j]) })
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func compareIDAPathsDesc(a, b string) bool {
	va, vb := parseVersionNumbers(filepath.Base(a)), parseVersionNumbers(filepath.Base(b))
	for k := 0; k < len(va) && k < len(vb); k++ {
		if va[k] != vb[k] {
			return va[k] > vb[k]
		}
	}
	if len(va) != len(vb) {
		return len(va) > len(vb)
	}
	return a > b
}

func parseVersionNumbers(s string) []int {
	var nums []int
	cur := -1
	for _, r := range s {
		if r >= '0' && r <= '9' {
			if cur < 0 {
				cur = 0
			}
			cur = cur*10 + int(r-'0')
		} else if cur >= 0 {
			nums = append(nums, cur)
			cur = -1
		}
	}
	if cur >= 0 {
		nums = append(nums, cur)
	}
	return nums
}
