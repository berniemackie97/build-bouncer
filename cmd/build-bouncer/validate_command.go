package main

import (
	"fmt"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func newValidateCommand() cli.Command {
	return cli.Command{
		Name:    "validate",
		Usage:   "validate [--config PATH]",
		Summary: "Validate .buildbouncer/config.yaml (or legacy .buildbouncer.yaml).",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "validate")
			cfgPath := fs.String("config", "", "path to .buildbouncer/config.yaml")
			if err := fs.Parse(args); err != nil {
				return exitUsage
			}
			return runValidate(*cfgPath, ctx)
		},
	}
}

func runValidate(cfgPath string, ctx cli.Context) int {
	root := configRootForPath(cfgPath)
	if cfgPath == "" {
		var err error
		cfgPath, root, err = config.FindConfigFromCwd()
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "validate:", err)
			return exitUsage
		}
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "validate:", err)
		return exitUsage
	}
	if err := generatedAdmission(root, cfg); err != nil {
		fmt.Fprintln(ctx.Stderr, "validate:", err)
		return exitUsage
	}

	fmt.Fprintln(ctx.Stdout, "Config OK:", cfgPath)
	fmt.Fprintf(ctx.Stdout, "Checks: %d\n", len(cfg.Checks))
	return exitOK
}
