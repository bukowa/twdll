package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"

	"twdll/cli/internal/cmd"
	"twdll/cli/internal/term"
	"twdll/cli/internal/workspace"
)

var Version = "0.1.0-dev"

type CLI struct {
	Version kong.VersionFlag `short:"v" name:"version" help:"Show CLI version."`
	Config  string           `short:"c" name:"config" help:"Path to configuration file (default: <root>/twdll.toml, env: TWDLL_CONFIG)."`
	DataDir string           `name:"data-dir" help:"Override workspace data directory (default: .twdll or config data_dir, env: TWDLL_DATA_DIR)."`
	Root    string           `name:"root" help:"Override repository root directory (env: TWDLL_ROOT)."`
	Color   string           `name:"color" enum:"auto,always,never" default:"auto" help:"Color output mode (auto, always, never)."`

	Status  StatusCmd  `cmd:"" help:"Show workspace, toolchain, binary, and database status."`
	Init    InitCmd    `cmd:"" help:"Initialize workspace toolchains, game binaries, and decompiler setup."`
	Analyze AnalyzeCmd `cmd:"" help:"Run headless decompiler analysis on staged binaries."`
	Extract ExtractCmd `cmd:"" help:"Extract binary facts and decompiled pseudo-C into SQLite databases."`
	Clean   CleanCmd   `cmd:"" help:"Wipe all workspace artifacts (requires confirmation or --yes)."`
}

type runContext struct {
	ctx    context.Context
	layout workspace.Layout
	ui     *term.UI
}

type StatusCmd struct {
	JSON bool `name:"json" help:"Emit machine-readable JSON to stdout."`
}

func (c *StatusCmd) Run(rctx *runContext) error {
	_, err := cmd.RunStatus(rctx.ctx, cmd.StatusRequest{
		Layout: rctx.layout,
		JSON:   c.JSON,
		UI:     rctx.ui,
	})
	return err
}

type InitCmd struct {
	Clear       bool     `name:"clear" help:"Clear initialized configuration, staged binaries, and local tools before re-initializing."`
	NoInput     bool     `name:"no-input" help:"Disable interactive prompts and fail fast on missing inputs."`
	JSON        bool     `name:"json" help:"Emit machine-readable JSON summary to stdout."`
	Games       []string `name:"games" placeholder:"GAMES" help:"32-bit target games to enable (comma-separated: attila,rome2,tob)."`
	DefaultGame string   `name:"default-game" placeholder:"GAME" help:"Default game (attila, rome2, tob)."`
	Decompiler  string   `name:"decompiler" placeholder:"ENGINE" help:"Decompiler backend (ghidra, ida)."`
	IDADir      string   `name:"ida-dir" help:"Path to IDA Pro installation directory."`
	AttilaDir   string   `name:"attila-dir" help:"Path to Total War: Attila installation directory."`
	Rome2Dir    string   `name:"rome2-dir" help:"Path to Total War: Rome II installation directory."`
	ToBDir      string   `name:"tob-dir" help:"Path to Total War Saga: Thrones of Britannia installation directory."`
	Attila64    string   `name:"attila-64" help:"Path to 64-bit Linux Attila ELF binary with DWARF symbols."`
}

func (c *InitCmd) Run(rctx *runContext) error {
	var prompter term.Prompter
	if rctx.ui.IsInteractive() && !c.NoInput {
		prompter = term.NewLinePrompter(rctx.ui)
	}
	return cmd.RunInit(rctx.ctx, cmd.InitRequest{
		Layout: rctx.layout,
		Options: cmd.InitOptions{
			Clear:       c.Clear,
			NoInput:     c.NoInput,
			JSON:        c.JSON,
			Games:       c.Games,
			DefaultGame: c.DefaultGame,
			Decompiler:  c.Decompiler,
			IDADir:      c.IDADir,
			AttilaDir:   c.AttilaDir,
			Rome2Dir:    c.Rome2Dir,
			ToBDir:      c.ToBDir,
			Attila64Src: c.Attila64,
		},
		Prompter: prompter,
		UI:       rctx.ui,
	})
}

type AnalyzeCmd struct {
	Decompiler   string   `short:"d" name:"decompiler" help:"Decompiler backend override (ghidra, ida)."`
	Clear        bool     `name:"clear" help:"Clear existing target databases and logs before analyzing."`
	Jobs         int      `short:"j" name:"jobs" help:"Maximum concurrent analysis jobs (0 = auto)."`
	MaxMem       string   `short:"m" name:"max-mem" help:"Override Ghidra JVM max heap (e.g. 16G, 32G)."`
	FullAnalysis bool     `name:"full-analysis" help:"Keep heavy reference/stack analysis passes enabled on attila_64."`
	Targets      []string `arg:"" optional:"" help:"Targets to analyze (attila_64, attila_32, rome2_32, tob_32, or globs like *_32, attila_*)."`
}

func (c *AnalyzeCmd) Run(rctx *runContext) error {
	return cmd.RunAnalyze(rctx.ctx, cmd.AnalyzeRequest{
		Layout: rctx.layout,
		Options: cmd.AnalyzeOptions{
			Decompiler:   c.Decompiler,
			Clear:        c.Clear,
			Jobs:         c.Jobs,
			MaxMem:       c.MaxMem,
			FullAnalysis: c.FullAnalysis,
			Targets:      c.Targets,
		},
		UI: rctx.ui,
	})
}

type ExtractCmd struct {
	Decompiler    string        `short:"d" name:"decompiler" help:"Decompiler backend override (ghidra, ida, all)."`
	Clear         bool          `name:"clear" help:"Clear extracted SQLite databases for the selected scope before re-extracting."`
	Jobs          int           `short:"j" name:"jobs" help:"Maximum concurrent decompilation workers (e.g. 0 = auto)."`
	MaxMem        string        `short:"m" name:"max-mem" help:"Override Ghidra JVM max heap per worker (e.g. 4G, 8G)."`
	NoDecomp      bool          `name:"no-decomp" help:"Extract bronze and silver metadata (funcs, asm, strings) without pseudo-C decompilation."`
	Timeout       time.Duration `name:"timeout" default:"15s" help:"Per-function decompilation timeout (e.g. 15s, 60s; 0s disables timeout)."`
	NoTimeout     bool          `name:"no-timeout" help:"Disable per-function decompilation timeout."`
	Retry         bool          `name:"retry" help:"Retry previously failed or timed-out functions during decompilation."`
	RetryTimeouts bool          `name:"retry-timeouts" help:"Retry previously timed-out functions during decompilation."`
	RetryFailed   bool          `name:"retry-failed" help:"Retry previously failed functions during decompilation."`
	JSON          bool          `name:"json" help:"Emit machine-readable JSON summary to stdout."`
	Targets       []string      `arg:"" optional:"" help:"Targets to extract (attila_64, attila_32, rome2_32, tob_32, or globs like *_32, attila_*)."`
}

func (c *ExtractCmd) Run(rctx *runContext) error {
	if c.Timeout < 0 {
		return &term.DiagnosticError{
			Message:  fmt.Sprintf("invalid --timeout %s", c.Timeout),
			Expected: "a non-negative duration (e.g. 15s, 60s, or 0s)",
			Help:     []string{"Pass `--timeout 60s` or `--no-timeout`."},
		}
	}
	noTimeout := c.NoTimeout || c.Timeout == 0
	var timeoutSecs int
	if !noTimeout {
		timeoutSecs = int((c.Timeout + time.Second - 1) / time.Second)
	}
	_, err := cmd.RunExtract(rctx.ctx, cmd.ExtractRequest{
		Layout: rctx.layout,
		Options: cmd.ExtractOptions{
			Decompiler:    c.Decompiler,
			Clear:         c.Clear,
			Jobs:          c.Jobs,
			MaxMem:        c.MaxMem,
			NoDecomp:      c.NoDecomp,
			TimeoutSecs:   timeoutSecs,
			NoTimeout:     noTimeout,
			Retry:         c.Retry,
			RetryTimeouts: c.RetryTimeouts,
			RetryFailed:   c.RetryFailed,
			JSON:          c.JSON,
			Targets:       c.Targets,
		},
		UI: rctx.ui,
	})
	return err
}

type CleanCmd struct {
	Yes bool `short:"y" name:"yes" help:"Skip confirmation prompt and wipe the workspace."`
}

func (c *CleanCmd) Run(rctx *runContext) error {
	var prompter term.Prompter
	if rctx.ui.IsInteractive() && !c.Yes {
		prompter = term.NewLinePrompter(rctx.ui)
	}
	return cmd.RunClean(rctx.ctx, cmd.CleanRequest{
		Layout: rctx.layout,
		Options: cmd.CleanOptions{
			Yes: c.Yes,
		},
		Prompter: prompter,
		UI:       rctx.ui,
	})
}

type kongExit struct {
	code int
}

func Execute(ctx context.Context, args []string, streams term.Streams) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if streams.Out == nil {
		streams.Out = io.Discard
	}
	if streams.Err == nil {
		streams.Err = io.Discard
	}

	var cli CLI
	var exited *kongExit

	parser, err := kong.New(
		&cli,
		kong.Name("twdll"),
		kong.Description("Unified reverse-engineering and build CLI for twdll."),
		kong.Vars{"version": "twdll " + Version},
		kong.Writers(streams.Out, streams.Err),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Exit(func(code int) {
			panic(kongExit{code: code})
		}),
	)
	if err != nil {
		ui := term.NewUI(streams)
		ui.Error(err)
		return 1
	}

	kctx, parseErr := func() (kctx *kong.Context, err error) {
		defer func() {
			if r := recover(); r != nil {
				if ke, ok := r.(kongExit); ok {
					exited = &ke
					return
				}
				panic(r)
			}
		}()
		return parser.Parse(args)
	}()
	if exited != nil {
		return exited.code
	}
	if parseErr != nil {
		ui := term.NewUI(streams)
		ui.Error(&term.DiagnosticError{
			Message: parseErr.Error(),
			Help:    []string{"run `twdll --help` to see available commands and flags"},
		})
		return 2
	}

	colorMode, err := term.ParseColorMode(cli.Color)
	if err != nil {
		ui := term.NewUI(streams)
		ui.Error(err)
		return 2
	}
	streams.ColorMode = colorMode
	ui := term.NewUI(streams)

	layout, err := workspace.Resolve(workspace.ResolveSpec{
		RootOverride:    cli.Root,
		ConfigOverride:  cli.Config,
		DataDirOverride: cli.DataDir,
	})
	if err != nil {
		ui.Error(err)
		return 1
	}

	if err := kctx.Run(&runContext{
		ctx:    ctx,
		layout: layout,
		ui:     ui,
	}); err != nil {
		ui.Error(err)
		return 1
	}
	return 0
}

func main() {
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCtx.Done()
		stop()
	}()
	code := Execute(sigCtx, os.Args[1:], term.Streams{
		In:        os.Stdin,
		Out:       os.Stdout,
		Err:       os.Stderr,
		ColorMode: term.ColorAuto,
	})
	stop()
	os.Exit(code)
}
