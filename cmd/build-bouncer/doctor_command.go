package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/runner"
	"github.com/berniemackie97/build-bouncer/internal/shell"
)

func newDoctorCommand() cli.Command {
	return cli.Command{
		Name:    "doctor",
		Usage:   "doctor [--config PATH]",
		Summary: "Diagnose shells, paths, and missing tools for checks.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "doctor")
			cfgPath := fs.String("config", "", "path to .buildbouncer/config.yaml")
			if err := fs.Parse(args); err != nil {
				return exitUsage
			}
			return runDoctor(*cfgPath, ctx)
		},
	}
}

func runDoctor(cfgPath string, ctx cli.Context) int {
	cfgDir := ""
	if cfgPath == "" {
		var err error
		cfgPath, cfgDir, err = config.FindConfigFromCwd()
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "doctor:", err)
			return exitUsage
		}
	} else {
		cfgDir = filepath.Dir(cfgPath)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "doctor:", err)
		return exitUsage
	}

	fmt.Fprintln(ctx.Stdout, "Config:", cfgPath)
	fmt.Fprintln(ctx.Stdout, "Repo:", cfgDir)
	fmt.Fprintln(ctx.Stdout, "OS:", runtime.GOOS)
	fmt.Fprintln(ctx.Stdout, "PATH:", os.Getenv("PATH"))

	for i, check := range cfg.Checks {
		fmt.Fprintln(ctx.Stdout, "")
		fmt.Fprintf(ctx.Stdout, "[%d] %s\n", i+1, check.Name)
		if strings.TrimSpace(check.ID) != "" {
			fmt.Fprintln(ctx.Stdout, "  id:", check.ID)
		}
		if strings.TrimSpace(check.Source) != "" {
			fmt.Fprintln(ctx.Stdout, "  source:", check.Source)
		}
		fmt.Fprintln(ctx.Stdout, "  run:", check.Run)
		fmt.Fprintln(ctx.Stdout, "  shell:", resolvedShell(check))
		fmt.Fprintln(ctx.Stdout, "  cwd:", resolvedCwd(cfgDir, check.Cwd))
		if len(check.OS) > 0 {
			fmt.Fprintln(ctx.Stdout, "  os:", strings.Join(check.OS, ","))
		}
		if len(check.Requires) > 0 {
			fmt.Fprintln(ctx.Stdout, "  requires:", strings.Join(check.Requires, ","))
		}
		if missing := runner.MissingTools(check); len(missing) > 0 {
			fmt.Fprintln(ctx.Stdout, "  missing:", strings.Join(missing, ", "))
		}
		if reason := runner.SkipReason(check); strings.TrimSpace(reason) != "" {
			fmt.Fprintln(ctx.Stdout, "  skip:", reason)
		}
	}

	return exitOK
}

func resolvedShell(check config.Check) string {
	if strings.TrimSpace(check.Shell) != "" {
		return shell.Normalize(check.Shell)
	}
	return shell.DefaultForOS(runtime.GOOS)
}

func resolvedCwd(root string, cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(cwd))
}
