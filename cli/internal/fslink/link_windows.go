//go:build windows

package fslink

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// IsLinkOrReparsePoint reports whether path itself (without following links) is a symlink
// or Windows reparse point (such as an NTFS junction).
func IsLinkOrReparsePoint(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return isFileInfoLinkOrReparse(fi), nil
}

func isFileInfoLinkOrReparse(fi os.FileInfo) bool {
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if sys, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		if sys.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return true
		}
	}
	return false
}

func createDirLink(targetAbs, linkPath string) error {
	cleanTarget := strings.TrimPrefix(targetAbs, `\\?\`)
	cleanTarget = strings.TrimPrefix(cleanTarget, `\??\`)

	substituteName := `\??\` + cleanTarget
	printName := cleanTarget

	subUTF16, err := windows.UTF16FromString(substituteName)
	if err != nil {
		return fmt.Errorf("fslink: encode substitute name %q: %w", substituteName, err)
	}
	printUTF16, err := windows.UTF16FromString(printName)
	if err != nil {
		return fmt.Errorf("fslink: encode print name %q: %w", printName, err)
	}

	// UTF16FromString includes a trailing NUL (1 uint16).
	// Length fields in MOUNT_POINT_REPARSE_BUFFER exclude the trailing NUL,
	// while the PathBuffer stores both strings with their trailing NULs.
	subByteLen := (len(subUTF16) - 1) * 2
	printByteLen := (len(printUTF16) - 1) * 2
	pathBufferBytes := len(subUTF16)*2 + len(printUTF16)*2

	// Header layout:
	//   0..3:   ReparseTag (uint32)
	//   4..5:   ReparseDataLength (uint16) = 8 + pathBufferBytes
	//   6..7:   Reserved (uint16)
	//   8..9:   SubstituteNameOffset (uint16) = 0
	//   10..11: SubstituteNameLength (uint16) = subByteLen
	//   12..13: PrintNameOffset (uint16) = subByteLen + 2
	//   14..15: PrintNameLength (uint16) = printByteLen
	//   16..:   PathBuffer
	reparseDataLen := 8 + pathBufferBytes
	totalSize := 8 + reparseDataLen
	if totalSize > windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE {
		return fmt.Errorf("fslink: reparse buffer size %d exceeds maximum %d", totalSize, windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	}

	buf := make([]byte, totalSize)
	binary.LittleEndian.PutUint32(buf[0:4], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buf[4:6], uint16(reparseDataLen))
	binary.LittleEndian.PutUint16(buf[6:8], 0)
	binary.LittleEndian.PutUint16(buf[8:10], 0)
	binary.LittleEndian.PutUint16(buf[10:12], uint16(subByteLen))
	binary.LittleEndian.PutUint16(buf[12:14], uint16(subByteLen+2))
	binary.LittleEndian.PutUint16(buf[14:16], uint16(printByteLen))

	offset := 16
	for _, u := range subUTF16 {
		binary.LittleEndian.PutUint16(buf[offset:offset+2], u)
		offset += 2
	}
	for _, u := range printUTF16 {
		binary.LittleEndian.PutUint16(buf[offset:offset+2], u)
		offset += 2
	}

	if err := os.Mkdir(linkPath, 0o755); err != nil {
		return fmt.Errorf("fslink: create junction directory %q: %w", linkPath, err)
	}

	linkUTF16, err := windows.UTF16PtrFromString(linkPath)
	if err != nil {
		_ = os.Remove(linkPath)
		return fmt.Errorf("fslink: encode link path %q: %w", linkPath, err)
	}

	handle, err := windows.CreateFile(
		linkUTF16,
		windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		_ = os.Remove(linkPath)
		return fmt.Errorf("fslink: open junction directory %q: %w", linkPath, err)
	}

	var bytesReturned uint32
	ioctlErr := windows.DeviceIoControl(
		handle,
		windows.FSCTL_SET_REPARSE_POINT,
		&buf[0],
		uint32(len(buf)),
		nil,
		0,
		&bytesReturned,
		nil,
	)
	closeErr := windows.CloseHandle(handle)
	if ioctlErr != nil {
		_ = os.Remove(linkPath)
		return fmt.Errorf("fslink: set reparse point on %q -> %q: %w", linkPath, cleanTarget, ioctlErr)
	}
	if closeErr != nil {
		return fmt.Errorf("fslink: close junction handle %q: %w", linkPath, closeErr)
	}

	return nil
}
