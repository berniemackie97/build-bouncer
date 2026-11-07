package main

import (
	"bytes"
	"fmt"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/git"
	"github.com/berniemackie97/build-bouncer/internal/hooks"
	"github.com/berniemackie97/build-bouncer/internal/tui"
)

func newSetupCommand() cli.Command {
	return cli.Command{
		Name:    "setup",
		Usage:   "setup [--force] [--no-copy] [--ci] [--import-ci] [--template-flag]",
		Summary: "Init config/packs, install hook, then run checks.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "setup")
			force := fs.Bool("force", false, "replace existing configuration and default packs")
			noCopy := fs.Bool("no-copy", false, "do not copy the build-bouncer binary into .git/hooks/bin")
			ci := fs.Bool("ci", false, "non-interactive check execution")
			importCI := fs.Bool("import-ci", false, "import supported GitHub Actions checks during initialization")
			selector := registerTemplateFlags(fs)
			if err := fs.Parse(args); err != nil {
				return exitUsage
			}
			templateID, err := selector.Selected()
			if err != nil {
				fmt.Fprintln(ctx.Stderr, "setup:", err)
				printInitHelp(ctx)
				return exitUsage
			}
			return runSetup(*force, *noCopy, *ci, *importCI, templateID, ctx)
		},
	}
}

func runSetup(force, noCopy, ci, importCI bool, templateID string, ctx cli.Context) int {
	root, err := git.FindRepoRoot()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "setup:", err)
		return exitUsage
	}

	if importCI && templateID == "" {
		fmt.Fprintln(ctx.Stderr, "setup: --import-ci requires a template; use build-bouncer ci sync to import into an existing config")
		return exitUsage
	}
	cfgPath, cfgExists := config.FindConfigInRoot(root)
	var before *setupSnapshot
	var output bytes.Buffer
	if templateID != "" {
		snapshot, err := snapshotSetup(root)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "setup:", err)
			return exitUsage
		}
		before = &snapshot
	}
	initContext := ctx
	initContext.Stdout = &output

	if cfgExists {
		if templateID != "" {
			if code := runInit(force, importCI, templateID, initContext); code != exitOK {
				return code
			}
		} else {
			fmt.Fprintln(ctx.Stdout, tui.Check("Config exists: "+cfgPath))
		}
	} else {
		if templateID == "" {
			fmt.Fprintln(ctx.Stderr, "setup: missing template flag (try: build-bouncer setup --go)")
			printInitHelp(ctx)
			return exitUsage
		}
		if code := runInit(force, importCI, templateID, initContext); code != exitOK {
			return code
		}
	}

	opts := hooks.InstallOptions{
		CopySelf: !noCopy,
		Force:    force,
	}
	if err := hooks.InstallPrePushHook(opts); err != nil {
		fmt.Fprintln(ctx.Stderr, tui.Error("setup: "+err.Error()))
		if before != nil {
			if rollbackErr := before.restore(); rollbackErr != nil {
				fmt.Fprintln(ctx.Stderr, "setup rollback:", rollbackErr)
			}
		}
		return exitUsage
	}
	if output.Len() > 0 {
		_, _ = fmt.Fprint(ctx.Stdout, output.String())
	}
	fmt.Fprintln(ctx.Stdout, tui.Success("✓ Installed git pre-push hook"))

	checkArgs := []string{}
	if ci {
		checkArgs = append(checkArgs, "--ci")
	}
	return runCheck(checkArgs, ctx)
}
