//go:build !windows

package fslink

import "os"

// IsLinkOrReparsePoint reports whether path itself (without following links) is a symbolic link.
func IsLinkOrReparsePoint(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return isFileInfoLinkOrReparse(fi), nil
}

func isFileInfoLinkOrReparse(fi os.FileInfo) bool {
	return fi.Mode()&os.ModeSymlink != 0
}

func createDirLink(targetAbs, linkPath string) error {
	return os.Symlink(targetAbs, linkPath)
}
