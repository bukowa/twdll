package term

import (
	"fmt"
	"io"
	"strings"
)

func (u *UI) SetLiveLines(lines []string) {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.CanAnimate() {
		return
	}
	if len(lines) == 0 {
		u.clearLiveLocked()
		return
	}
	u.renderLiveLocked(lines)
}

func (u *UI) ClearLiveLines() {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.clearLiveLocked()
}

func (u *UI) CommitLiveLines() {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.CanAnimate() && u.renderedLines > 0 {
		_, _ = io.WriteString(u.err, "\n")
	}
	u.liveLines = nil
	u.renderedLines = 0
}

func (u *UI) moveToLiveTopLocked(buf *strings.Builder) {
	if u.renderedLines == 1 {
		buf.WriteByte('\r')
	} else if u.renderedLines > 1 {
		fmt.Fprintf(buf, "\r\033[%dA", u.renderedLines-1)
	}
}

func (u *UI) clearLiveLocked() {
	if u.renderedLines > 0 && u.CanAnimate() {
		var buf strings.Builder
		u.moveToLiveTopLocked(&buf)
		buf.WriteString("\033[J")
		_, _ = io.WriteString(u.err, buf.String())
	}
	u.renderedLines = 0
	u.liveLines = nil
}

func (u *UI) renderLiveLocked(lines []string) {
	u.clearProgressLocked()
	maxW := u.progressMaxRunes()
	clean := make([]string, len(lines))
	for i := range lines {
		clean[i] = sanitizeProgressLine(lines[i], maxW)
	}

	var buf strings.Builder
	prev := u.renderedLines
	if prev > 0 {
		u.moveToLiveTopLocked(&buf)
	}
	for i, l := range clean {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString("\r\033[2K")
		buf.WriteString(l)
	}
	if len(clean) < prev {
		buf.WriteString("\033[J")
	}
	_, _ = io.WriteString(u.err, buf.String())
	u.liveLines = clean
	u.renderedLines = len(clean)
}
