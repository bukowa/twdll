package term

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	xterm "golang.org/x/term"
)

var ErrInputAborted = errors.New("interactive input aborted")

type Prompter interface {
	Confirm(prompt string, defaultVal bool) (bool, error)
	Select(prompt string, options []string, defaultIdx int) (int, error)
	Input(prompt string, defaultVal string) (string, error)
}

type LinePrompter struct {
	ui     *UI
	reader *bufio.Reader
	errOut io.Writer
	style  Style
}

func NewLinePrompter(ui *UI) *LinePrompter {
	return &LinePrompter{
		ui:     ui,
		reader: bufio.NewReader(ui.In()),
		errOut: ui.Err(),
		style:  ui.ErrStyle(),
	}
}

func (p *LinePrompter) Confirm(prompt string, defaultVal bool) (bool, error) {
	hint := "[Y/n]"
	if !defaultVal {
		hint = "[y/N]"
	}
	for {
		_, _ = fmt.Fprintf(p.errOut, "%s %s: ", p.style.Bold(prompt), p.style.Dim(hint))
		line, err := p.readLine()
		if err != nil {
			return false, err
		}
		if line == "" {
			return defaultVal, nil
		}
		switch strings.ToLower(line) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			_, _ = fmt.Fprintf(p.errOut, "  please enter 'y' or 'n'\n")
		}
	}
}

func (p *LinePrompter) Select(prompt string, options []string, defaultIdx int) (int, error) {
	if len(options) == 0 {
		return 0, errors.New("select requires at least one option")
	}
	if defaultIdx < 0 || defaultIdx >= len(options) {
		defaultIdx = 0
	}

	if p.ui != nil && p.ui.IsInteractive() && p.ui.CanAnimate() {
		inFile, okIn := p.ui.In().(*os.File)
		errFile, okErr := p.ui.Err().(*os.File)
		if okIn && okErr {
			width, height, sizeErr := xterm.GetSize(int(errFile.Fd()))
			maxWidth := maxProgressVisibleRunes
			if sizeErr == nil && width > 10 {
				maxWidth = width - 2
			}
			if sizeErr == nil && height > 0 && len(options)+1 >= height {
				return p.selectLine(prompt, options, defaultIdx)
			}
			oldState, err := xterm.MakeRaw(int(inFile.Fd()))
			if err != nil {
				return p.selectLine(prompt, options, defaultIdx)
			}
			_, _ = io.WriteString(p.errOut, "\033[?25l")
			var restoreOnce sync.Once
			restore := func() {
				restoreOnce.Do(func() {
					_, _ = io.WriteString(p.errOut, "\033[?25h")
					_ = xterm.Restore(int(inFile.Fd()), oldState)
				})
			}
			defer restore()

			readChunk := func(buf []byte) (int, error) {
				if n := p.reader.Buffered(); n > 0 {
					return p.reader.Read(buf[:min(len(buf), n)])
				}
				return inFile.Read(buf)
			}
			idx, loopErr := p.selectInteractiveLoop(readChunk, prompt, options, defaultIdx, maxWidth)
			restore()
			if loopErr != nil {
				_, _ = fmt.Fprintf(p.errOut, "%s: %s\n", p.style.Bold(prompt), p.style.Yellow("aborted"))
				return 0, loopErr
			}
			_, _ = fmt.Fprintf(p.errOut, "%s: %s\n", p.style.Bold(prompt), p.style.Cyan(options[idx]))
			return idx, nil
		}
	}

	return p.selectLine(prompt, options, defaultIdx)
}

func (p *LinePrompter) selectLine(prompt string, options []string, defaultIdx int) (int, error) {
	_, _ = fmt.Fprintf(p.errOut, "%s:\n", p.style.Bold(prompt))
	for i, opt := range options {
		marker := " "
		label := opt
		if i == defaultIdx {
			marker = "*"
			label = opt + " " + p.style.Dim("(default)")
		}
		_, _ = fmt.Fprintf(p.errOut, "  %s [%d] %s\n", p.style.Cyan(marker), i+1, label)
	}

	for {
		_, _ = fmt.Fprintf(p.errOut, "Choose [1-%d] (Enter = %d): ", len(options), defaultIdx+1)
		line, err := p.readLine()
		if err != nil {
			return 0, err
		}
		if line == "" {
			return defaultIdx, nil
		}
		for i, opt := range options {
			if strings.EqualFold(line, opt) {
				return i, nil
			}
		}
		n, convErr := strconv.Atoi(line)
		if convErr == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		_, _ = fmt.Fprintf(p.errOut, "  invalid choice %q; enter a number between 1 and %d\n", line, len(options))
	}
}

type selectEventKind uint8

const (
	selectEventNone selectEventKind = iota
	selectEventUp
	selectEventDown
	selectEventHome
	selectEventEnd
	selectEventDigit
	selectEventEnter
	selectEventAbort
)

func nextSelectEvent(b []byte) (kind selectEventKind, digit int, consumed int) {
	if len(b) == 0 {
		return selectEventNone, 0, 0
	}
	switch b[0] {
	case 0x03, 0x04:
		return selectEventAbort, 0, 1
	case '\r':
		if len(b) >= 2 && b[1] == '\n' {
			return selectEventEnter, 0, 2
		}
		return selectEventEnter, 0, 1
	case '\n':
		return selectEventEnter, 0, 1
	case 'k', 'K':
		return selectEventUp, 0, 1
	case 'j', 'J':
		return selectEventDown, 0, 1
	case 0x1b:
		if len(b) == 1 {
			return selectEventNone, 0, 1
		}
		switch b[1] {
		case '[':
			if len(b) >= 3 {
				switch b[2] {
				case 'A':
					return selectEventUp, 0, 3
				case 'B':
					return selectEventDown, 0, 3
				case 'H':
					return selectEventHome, 0, 3
				case 'F':
					return selectEventEnd, 0, 3
				}
			}
			if len(b) >= 4 && b[3] == '~' {
				switch b[2] {
				case '1', '7':
					return selectEventHome, 0, 4
				case '4', '8':
					return selectEventEnd, 0, 4
				}
			}
			j := 2
			for j < len(b) && b[j] >= 0x20 && b[j] <= 0x3F {
				j++
			}
			if j < len(b) && b[j] >= 0x40 && b[j] <= 0x7E {
				j++
			}
			return selectEventNone, 0, j
		case 'O':
			if len(b) >= 3 {
				switch b[2] {
				case 'A':
					return selectEventUp, 0, 3
				case 'B':
					return selectEventDown, 0, 3
				case 'H':
					return selectEventHome, 0, 3
				case 'F':
					return selectEventEnd, 0, 3
				}
				return selectEventNone, 0, 3
			}
			return selectEventNone, 0, len(b)
		default:
			return selectEventNone, 0, 2
		}
	default:
		if b[0] >= '1' && b[0] <= '9' {
			return selectEventDigit, int(b[0] - '1'), 1
		}
		return selectEventNone, 0, 1
	}
}

func (p *LinePrompter) selectInteractiveLoop(readChunk func([]byte) (int, error), prompt string, options []string, defaultIdx int, maxWidth int) (int, error) {
	idx := defaultIdx
	totalLines := len(options) + 1
	var frame bytes.Buffer

	render := func(first bool) {
		frame.Reset()
		if !first {
			_, _ = fmt.Fprintf(&frame, "\033[%dA\r", totalLines)
		}
		header := sanitizeProgressLine(fmt.Sprintf("%s %s", p.style.Bold(prompt), p.style.Dim("(↑/↓ or 1-9, Enter to confirm)")), maxWidth)
		_, _ = fmt.Fprintf(&frame, "\r\033[2K%s\r\n", header)
		for i, opt := range options {
			var row string
			if i == idx {
				row = fmt.Sprintf("  %s [%d] %s", p.style.Cyan(">"), i+1, p.style.Bold(opt))
			} else {
				row = fmt.Sprintf("    [%d] %s", i+1, opt)
			}
			if i == defaultIdx {
				row += " " + p.style.Dim("(default)")
			}
			_, _ = fmt.Fprintf(&frame, "\r\033[2K%s\r\n", sanitizeProgressLine(row, maxWidth))
		}
		_, _ = p.errOut.Write(frame.Bytes())
	}

	clearMenu := func() {
		_, _ = fmt.Fprintf(p.errOut, "\033[%dA\r\033[J", totalLines)
	}

	render(true)

	var buf [64]byte
	for {
		n, err := readChunk(buf[:])
		chunk := buf[:n]
		for len(chunk) > 0 {
			ev, digit, consumed := nextSelectEvent(chunk)
			if consumed <= 0 {
				consumed = 1
			}
			chunk = chunk[consumed:]
			switch ev {
			case selectEventUp:
				idx = (idx - 1 + len(options)) % len(options)
				render(false)
			case selectEventDown:
				idx = (idx + 1) % len(options)
				render(false)
			case selectEventHome:
				idx = 0
				render(false)
			case selectEventEnd:
				idx = len(options) - 1
				render(false)
			case selectEventDigit:
				if digit >= 0 && digit < len(options) {
					idx = digit
					render(false)
				}
			case selectEventEnter:
				clearMenu()
				return idx, nil
			case selectEventAbort:
				clearMenu()
				return 0, ErrInputAborted
			}
		}
		if err != nil {
			clearMenu()
			if errors.Is(err, io.EOF) {
				return 0, ErrInputAborted
			}
			return 0, err
		}
	}
}

func (p *LinePrompter) Input(prompt string, defaultVal string) (string, error) {
	if defaultVal != "" {
		_, _ = fmt.Fprintf(p.errOut, "%s %s: ", p.style.Bold(prompt), p.style.Dim("["+defaultVal+"]"))
	} else {
		_, _ = fmt.Fprintf(p.errOut, "%s: ", p.style.Bold(prompt))
	}
	line, err := p.readLine()
	if err != nil {
		return "", err
	}
	if line == "" {
		return defaultVal, nil
	}
	return strings.Trim(line, `"`), nil
}

func (p *LinePrompter) readLine() (string, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			if len(line) > 0 {
				return strings.TrimSpace(line), nil
			}
			return "", ErrInputAborted
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}
