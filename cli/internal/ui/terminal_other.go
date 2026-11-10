//go:build !windows
// +build !windows

package ui

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

func initTerminalColors() {}

func readConsoleKey() KeyEvent {
	return KeyEvent{Type: KeyOther}
}
