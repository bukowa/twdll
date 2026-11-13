package cmd

import (
	"context"

	"twdll/cli/internal/analyze"
)

type AnalyzeOptions = analyze.Options

type AnalyzeRequest = analyze.Request

func RunAnalyze(ctx context.Context, req AnalyzeRequest) error {
	return analyze.Run(ctx, req)
}
