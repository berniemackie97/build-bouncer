package main

import (
	"fmt"
	"reflect"

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

	var merge mergeResult
	var removed, imported int
	err = config.WithEditLock(cfgPath, func() error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		ciChecks, err := ci.ChecksFromGitHubActions(cfgDir)
		if err != nil {
			return err
		}
		imported = len(ciChecks)
		ciChecks = stampGeneratedChecks(ciChecks, "ci")

		mergedBase, count, err := stripCIChecks(cfg.Checks)
		if err != nil {
			return err
		}
		removed = count
		if imported == 0 {
			if removed == 0 {
				return nil
			}
			if len(mergedBase) == 0 {
				return fmt.Errorf("removing stale CI checks would leave no checks configured; add a local check before syncing")
			}
		} else {
			mergedBase = stripManualPlaceholder(mergedBase)
		}

		merge, err = mergeChecks(mergedBase, ciChecks)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(cfg.Checks, merge.Merged) {
			return nil
		}
		after := *cfg
		after.Checks = merge.Merged
		return saveConfiguredLocked(cfgPath, cfg, &after)
	})
	if err != nil {
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
	if imported == 0 {
		fmt.Fprintln(ctx.Stdout, "No CI checks found in .github/workflows.")
	} else if len(merge.Added) == 0 {
		fmt.Fprintln(ctx.Stdout, "No new CI checks to add.")
	}

	return exitOK
}
