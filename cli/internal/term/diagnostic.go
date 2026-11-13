package term

import (
	"errors"
	"fmt"
	"io"
)

type DiagnosticError struct {
	Message  string
	Expected string
	Help     []string
}

func (e *DiagnosticError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func WriteError(w io.Writer, style Style, err error) {
	if err == nil || w == nil {
		return
	}

	var diag *DiagnosticError
	if !errors.As(err, &diag) {
		_, _ = fmt.Fprintf(w, "%s %v\n", style.Red("error:"), err)
		return
	}

	_, _ = fmt.Fprintf(w, "%s %s\n", style.Red("error:"), diag.Message)
	if diag.Expected != "" {
		_, _ = fmt.Fprintf(w, "  %s %s\n", style.Bold("expected:"), diag.Expected)
	}
	for _, h := range diag.Help {
		_, _ = fmt.Fprintf(w, "  %s %s\n", style.Cyan("help:"), h)
	}
}
