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
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	return &App{
		name:          strings.TrimSpace(name),
		version:       strings.TrimSpace(version),
		usageExitCode: usageExitCode,
		commands:      map[string]Command{},
		ctx: Context{
			Stdout: stdout,
			Stderr: stderr,
		},
	}
}

func (a *App) Register(cmd Command) {
	cmdName := strings.TrimSpace(cmd.Name)
	if cmdName == "" {
		return
	}
	cmd.Name = cmdName
	a.commands[cmdName] = cmd
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		a.printHelp()
		return a.usageExitCode
	}

	first := strings.TrimSpace(args[0])
	switch first {
	case "-h", "--help":
		a.printHelp()
		return 0
	case "help":
		if len(args) >= 2 {
			return a.printHelpFor(args[1])
		}
		a.printHelp()
		return 0
	case "version", "--version", "-v":
		_, _ = fmt.Fprintf(a.ctx.Stdout, "%s %s\n", a.name, a.version)
		return 0
	}

	cmd, ok := a.commands[first]
	if !ok {
		_, _ = fmt.Fprintf(a.ctx.Stderr, "Unknown command: %s\n\n", first)
		a.printHelp()
		return a.usageExitCode
	}

	return cmd.Run(a.ctx, args[1:])
}

func (a *App) printHelp() {
	_, _ = fmt.Fprintf(a.ctx.Stdout, "%s %s\n\n", a.name, a.version)
	_, _ = fmt.Fprintf(a.ctx.Stdout, "Usage:\n")
	_, _ = fmt.Fprintf(a.ctx.Stdout, "  %s <command> [options]\n\n", a.name)
	_, _ = fmt.Fprintf(a.ctx.Stdout, "Commands:\n")

	names := make([]string, 0, len(a.commands))
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
		_, _ = fmt.Fprintf(a.ctx.Stdout, "  %-18s %s\n", usage, cmd.Summary)
	}

	_, _ = fmt.Fprintf(a.ctx.Stdout, "\nHelp:\n")
	_, _ = fmt.Fprintf(a.ctx.Stdout, "  %s help\n", a.name)
	_, _ = fmt.Fprintf(a.ctx.Stdout, "  %s help <command>\n", a.name)
}

func (a *App) printHelpFor(commandName string) int {
	name := strings.TrimSpace(commandName)
	if name == "" {
		a.printHelp()
		return 0
	}

	cmd, ok := a.commands[name]
	if !ok {
		_, _ = fmt.Fprintf(a.ctx.Stderr, "Unknown command: %s\n\n", name)
		a.printHelp()
		return a.usageExitCode
	}

	usage := strings.TrimSpace(cmd.Usage)
	if usage == "" {
		usage = cmd.Name
	}

	_, _ = fmt.Fprintf(a.ctx.Stdout, "%s %s\n\n", a.name, a.version)
	_, _ = fmt.Fprintf(a.ctx.Stdout, "Usage:\n")
	_, _ = fmt.Fprintf(a.ctx.Stdout, "  %s %s\n\n", a.name, usage)
	if strings.TrimSpace(cmd.Summary) != "" {
		_, _ = fmt.Fprintf(a.ctx.Stdout, "%s\n", strings.TrimSpace(cmd.Summary))
	}
	return 0
}

func NewFlagSet(ctx Context, name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if ctx.Stderr != nil {
		fs.SetOutput(ctx.Stderr)
	} else {
		fs.SetOutput(io.Discard)
	}
	return fs
}
