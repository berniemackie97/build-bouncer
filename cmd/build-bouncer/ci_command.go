package main

import (
	"fmt"

	"github.com/berniemackie97/build-bouncer/internal/ci"
	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func newCICommand() cli.Command {
	return cli.Command{
		Name:    "ci",
		Usage:   "ci <sync>",
		Summary: "Manage CI-derived checks.",
		Run: func(ctx cli.Context, args []string) int {
			return runCI(args, ctx)
		},
	}
}

func runCI(args []string, ctx cli.Context) int {
	if len(args) < 1 {
		fmt.Fprintln(ctx.Stderr, "ci: missing subcommand (expected: sync)")
		return exitUsage
	}

	switch args[0] {
	case "sync":
		fs := cli.NewFlagSet(ctx, "ci sync")
		if err := fs.Parse(args[1:]); err != nil {
			return exitUsage
		}
		return runCISync(ctx)
	default:
		fmt.Fprintf(ctx.Stderr, "ci: unknown subcommand: %s\n", args[0])
		return exitUsage
	}
}

func runCISync(ctx cli.Context) int {
	cfgPath, cfgDir, err := config.FindConfigFromCwd()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "ci sync:", err)
		return exitUsage
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "ci sync:", err)
		return exitUsage
	}

	ciChecks, err := ci.ChecksFromGitHubActions(cfgDir)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "ci sync:", err)
		return exitUsage
	}
	if len(ciChecks) == 0 {
		fmt.Fprintln(ctx.Stdout, "No CI checks found in .github/workflows.")
		return exitOK
	}

	ciChecks = stampGeneratedChecks(ciChecks, "ci")

	mergedBase, removed := stripCIChecks(cfg.Checks)
	mergedBase = stripManualPlaceholder(mergedBase)

	merge := mergeChecks(mergedBase, ciChecks)
	cfg.Checks = merge.Merged

	if err := config.Save(cfgPath, cfg); err != nil {
		fmt.Fprintln(ctx.Stderr, "ci sync:", err)
		return exitUsage
	}

	if len(merge.Added) > 0 {
		fmt.Fprintf(ctx.Stdout, "Added %d CI checks from .github/workflows\n", len(merge.Added))
	}
	if len(merge.Skipped) > 0 {
		fmt.Fprintf(ctx.Stdout, "Skipped %d duplicate CI checks\n", len(merge.Skipped))
	}
	if removed > 0 {
		fmt.Fprintf(ctx.Stdout, "Removed %d stale CI checks\n", removed)
	}
	if len(merge.Added) == 0 {
		fmt.Fprintln(ctx.Stdout, "No new CI checks to add.")
	}

	return exitOK
}
