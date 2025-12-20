package cli

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Command struct {
	Name    string
	Usage   string
	Summary string
	Run     func(ctx Context, args []string) int
}

type Context struct {
	Stdout io.Writer
	Stderr io.Writer
}

type App struct {
	name          string
	version       string
	usageExitCode int
	commands      map[string]Command
	ctx           Context
}

func NewApp(name string, version string, stdout io.Writer, stderr io.Writer, usageExitCode int) *App {
	return &App{
		name:          name,
		version:       version,
		usageExitCode: usageExitCode,
		commands:      map[string]Command{},
		ctx: Context{
			Stdout: stdout,
			Stderr: stderr,
		},
	}
}

func (a *App) Register(cmd Command) {
	if strings.TrimSpace(cmd.Name) == "" {
		return
	}
	a.commands[cmd.Name] = cmd
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		a.printHelp()
		return a.usageExitCode
	}

	switch args[0] {
	case "-h", "--help", "help":
		a.printHelp()
		return 0
	case "version", "--version", "-v":
		fmt.Fprintf(a.ctx.Stdout, "%s %s\n", a.name, a.version)
		return 0
	}

	cmd, ok := a.commands[args[0]]
	if !ok {
		fmt.Fprintf(a.ctx.Stderr, "Unknown command: %s\n\n", args[0])
		a.printHelp()
		return a.usageExitCode
	}

	return cmd.Run(a.ctx, args[1:])
}

func (a *App) printHelp() {
	fmt.Fprintf(a.ctx.Stdout, "%s %s\n\n", a.name, a.version)
	fmt.Fprintf(a.ctx.Stdout, "Usage:\n")
	fmt.Fprintf(a.ctx.Stdout, "  %s <command> [options]\n\n", a.name)
	fmt.Fprintf(a.ctx.Stdout, "Commands:\n")

	var names []string
	for name := range a.commands {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		cmd := a.commands[name]
		usage := strings.TrimSpace(cmd.Usage)
		if usage == "" {
			usage = cmd.Name
		}
		fmt.Fprintf(a.ctx.Stdout, "  %-18s %s\n", usage, cmd.Summary)
	}

	fmt.Fprintf(a.ctx.Stdout, "\nHelp:\n  %s help\n", a.name)
}

func NewFlagSet(ctx Context, name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(ctx.Stderr)
	return fs
}
