//go:build windows
// +build windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")

	msvcrt    = syscall.NewLazyDLL("msvcrt.dll")
	procGetch = msvcrt.NewProc("_getch")
)

type KeyType int

const (
	KeyUp KeyType = iota
	KeyDown
	KeyEnter
	KeyCancel
	KeyOther
)

type KeyEvent struct {
	Type KeyType
	Char rune
}

func initTerminalColors() {
	handle := syscall.Handle(os.Stdout.Fd())
	var mode uint32
	r1, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if r1 != 0 {
		const enableVirtualTerminalProcessing = 0x0004
		_, _, _ = procSetConsoleMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminalProcessing))
	}
}

func readConsoleKey() KeyEvent {
	r, _, _ := procGetch.Call()
	ch := int(r)
	if ch == 3 { // Ctrl+C
		return KeyEvent{Type: KeyCancel}
	}
	if ch == 13 || ch == 10 { // Enter
		return KeyEvent{Type: KeyEnter}
	}
	if ch == 0 || ch == 224 { // Extended key prefix on Windows
		r2, _, _ := procGetch.Call()
		switch int(r2) {
		case 72: // Up arrow
			return KeyEvent{Type: KeyUp}
		case 80: // Down arrow
			return KeyEvent{Type: KeyDown}
		default:
			return KeyEvent{Type: KeyOther}
		}
	}
	// 'k' or 'w' as Up
	if ch == 'k' || ch == 'w' || ch == 'K' || ch == 'W' {
		return KeyEvent{Type: KeyUp}
	}
	// 'j' or 's' as Down
	if ch == 'j' || ch == 's' || ch == 'J' || ch == 'S' {
		return KeyEvent{Type: KeyDown}
	}
	return KeyEvent{Type: KeyOther, Char: rune(ch)}
}
