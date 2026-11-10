package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/alecthomas/kong"
	"twdll/cli/internal/cmd"
	"twdll/cli/internal/ui"
	"twdll/cli/internal/workspace"
)

type Context struct {
	Paths   workspace.Paths
	Printer *ui.Printer
}

type StatusCmd struct {
	JSON bool `help:"Output machine-readable JSON status."`
}

func (c *StatusCmd) Run(ctx *Context) error {
	_, err := cmd.RunStatus(context.Background(), ctx.Paths, c.JSON, ctx.Printer)
	return err
}

type InitCmd struct {
	Clear       bool   `help:"Clear all existing tools, binaries, databases, and config to re-initialize from scratch."`
	Clean       bool   `help:"Alias for --clear."`
	NoInput     bool   `help:"Disable interactive prompts."`
	DefaultGame string `enum:"attila,rome2,tob" default:"attila" help:"Default game (attila, rome2, tob)."`
	Decompiler  string `enum:"ghidra,ida" default:"ghidra" help:"Decompiler backend (ghidra, ida)."`
	IDADir      string `name:"ida-dir" help:"Directory containing IDA Pro installation."`
	AttilaDir   string `name:"attila-dir" help:"Install directory for Total War: Attila."`
	Rome2Dir    string `name:"rome2-dir" help:"Install directory for Total War: Rome II."`
	ToBDir      string `name:"tob-dir" help:"Install directory for Total War: Thrones of Britannia."`
	Attila64Src string `name:"attila-64" help:"Path to 64-bit Linux Attila ELF binary with DWARF."`
}

func (c *InitCmd) Run(ctx *Context) error {
	opts := cmd.InitOptions{
		Clear:       c.Clear || c.Clean,
		NoInput:     c.NoInput,
		DefaultGame: c.DefaultGame,
		Decompiler:  c.Decompiler,
		IDADir:      c.IDADir,
		AttilaDir:   c.AttilaDir,
		Rome2Dir:    c.Rome2Dir,
		ToBDir:      c.ToBDir,
		Attila64Src: c.Attila64Src,
	}
	var prompter ui.Prompter
	if ui.IsStdinTerminal() {
		prompter = ui.NewTerminalPrompter(os.Stdin, os.Stdout)
	}
	return cmd.RunInit(context.Background(), ctx.Paths, opts, prompter, ctx.Printer)
}

type AnalyzeCmd struct {
	Force   bool     `help:"Force re-analysis even if hash matches."`
	Jobs    int      `short:"j" help:"Max concurrent decompiler jobs (0 for auto)."`
	Targets []string `arg:"" optional:"" help:"Specific targets to analyze (e.g. attila_64, attila_32)."`
}

func (c *AnalyzeCmd) Run(ctx *Context) error {
	ctx.Printer.Info("[analyze] Placeholder - wiring analyze command next")
	return nil
}

type CLI struct {
	Version kong.VersionFlag `short:"v" help:"Show version."`
	Color   string           `enum:"auto,always,never" default:"auto" help:"Color output mode (auto, always, never)."`

	Status  StatusCmd  `cmd:"" help:"Check workspace readiness, tools, and binary status."`
	Init    InitCmd    `cmd:"" help:"Initialize portable toolchains and acquire game binaries."`
	Analyze AnalyzeCmd `cmd:"" help:"Build headless decompiler databases from scratch."`
}

const Version = "0.1.0"

func ensureLatestBinary(repoRoot string) {
	if os.Getenv("TWDLL_NO_REBUILD") == "1" {
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		return
	}
	exeInfo, err := os.Stat(exePath)
	if err != nil {
		return
	}
	exeTime := exeInfo.ModTime()

	_ = os.Remove(exePath + ".old")

	cliDir := filepath.Join(repoRoot, "cli")
	needsRebuild := false
	_ = filepath.Walk(cliDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		name := filepath.Base(path)
		if ext == ".go" || name == "go.mod" || name == "go.sum" {
			if info.ModTime().After(exeTime) {
				needsRebuild = true
				return filepath.SkipAll
			}
		}
		return nil
	})

	if !needsRebuild {
		return
	}

	goBin := filepath.Join(repoRoot, ".twdll", "tools", "go", "bin", "go.exe")
	if runtime.GOOS != "windows" {
		goBin = filepath.Join(repoRoot, ".twdll", "tools", "go", "bin", "go")
	}
	if _, err := os.Stat(goBin); err != nil {
		return
	}

	fmt.Fprintln(os.Stderr, "[twdll] Sources modified, recompiling CLI...")
	tmpExe := exePath + ".tmp"
	_ = os.Remove(tmpExe)
	buildCmd := exec.Command(goBin, "build", "-buildvcs=false", "-o", tmpExe, ".")
	buildCmd.Dir = cliDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[twdll] Recompilation failed: %v\n", err)
		return
	}

	oldExe := exePath + ".old"
	_ = os.Remove(oldExe)
	if err := os.Rename(exePath, oldExe); err != nil {
		_ = os.Remove(tmpExe)
		return
	}
	if err := os.Rename(tmpExe, exePath); err != nil {
		_ = os.Rename(oldExe, exePath)
		return
	}

	reExec := exec.Command(exePath, os.Args[1:]...)
	reExec.Stdin = os.Stdin
	reExec.Stdout = os.Stdout
	reExec.Stderr = os.Stderr
	reExec.Env = append(os.Environ(), "TWDLL_NO_REBUILD=1")
	if err := reExec.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func main() {
	var cli CLI
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ensureLatestBinary(repoRoot)

	appCtx := &Context{
		Paths:   workspace.NewPaths(repoRoot),
		Printer: ui.NewDefaultPrinter(),
	}

	parser, err := kong.New(&cli,
		kong.Name("twdll"),
		kong.Description("Total War Script Extender & Reverse-Engineering Framework"),
		kong.Vars{"version": Version},
		kong.UsageOnError(),
	)
	if err != nil {
		appCtx.Printer.Error(err)
		os.Exit(1)
	}

	kctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	switch cli.Color {
	case "always":
		appCtx.Printer.ColorMode = ui.ColorAlways
	case "never":
		appCtx.Printer.ColorMode = ui.ColorNever
	default:
		appCtx.Printer.ColorMode = ui.ColorAuto
	}

	if err := kctx.Run(appCtx); err != nil {
		appCtx.Printer.Error(err)
		os.Exit(1)
	}
}
