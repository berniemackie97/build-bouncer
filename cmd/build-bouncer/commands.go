package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"build-bouncer/internal/banter"
	"build-bouncer/internal/ci"
	"build-bouncer/internal/cli"
	"build-bouncer/internal/config"
	"build-bouncer/internal/git"
	"build-bouncer/internal/hooks"
	"build-bouncer/internal/runner"
	"build-bouncer/internal/ui"
)

func registerCommands(app *cli.App) {
	app.Register(newSetupCommand())
	app.Register(newInitCommand())
	app.Register(newCheckCommand())
	app.Register(newValidateCommand())
	app.Register(newCICommand())
	app.Register(newHookCommand())
	app.Register(newUninstallCommand())
}

func newSetupCommand() cli.Command {
	return cli.Command{
		Name:    "setup",
		Usage:   "setup [--force] [--no-copy] [--ci] [--template-flag]",
		Summary: "Init config/packs, install hook, then run checks.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "setup")
			force := fs.Bool("force", false, "overwrite default insult/banter packs")
			noCopy := fs.Bool("no-copy", false, "do not copy the build-bouncer binary into .git/hooks/bin")
			ci := fs.Bool("ci", false, "CI mode")
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
			return runSetup(*force, *noCopy, *ci, templateID, ctx)
		},
	}
}

func runSetup(force bool, noCopy bool, ci bool, templateID string, ctx cli.Context) int {
	root, err := git.FindRepoRoot()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "setup:", err)
		return exitUsage
	}

	cfgPath, cfgExists := config.FindConfigInRoot(root)

	if cfgExists {
		if templateID != "" {
			if code := runInit(force, templateID, ctx); code != exitOK {
				return code
			}
		} else {
			fmt.Fprintln(ctx.Stdout, "Config exists:", cfgPath)
		}
	} else {
		if templateID == "" {
			fmt.Fprintln(ctx.Stderr, "setup: missing template flag (try: build-bouncer setup --go)")
			printInitHelp(ctx)
			return exitUsage
		}
		if code := runInit(force, templateID, ctx); code != exitOK {
			return code
		}
	}

	opts := hooks.InstallOptions{CopySelf: !noCopy}
	if err := hooks.InstallPrePushHook(opts); err != nil {
		fmt.Fprintln(ctx.Stderr, "setup:", err)
		return exitUsage
	}
	fmt.Fprintln(ctx.Stdout, "Installed git pre-push hook.")

	checkArgs := []string{}
	if ci {
		checkArgs = append(checkArgs, "--ci")
	}
	return runCheck(checkArgs, ctx)
}

func newInitCommand() cli.Command {
	return cli.Command{
		Name:    "init",
		Usage:   "init [--force] [--template-flag]",
		Summary: "Create .buildbouncer/config.yaml and default insult/banter packs.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "init")
			force := fs.Bool("force", false, "overwrite default insult/banter packs")
			selector := registerTemplateFlags(fs)
			if err := fs.Parse(args); err != nil {
				return exitUsage
			}
			templateID, err := selector.Selected()
			if err != nil {
				fmt.Fprintln(ctx.Stderr, "init:", err)
				printInitHelp(ctx)
				return exitUsage
			}
			if templateID == "" {
				printInitHelp(ctx)
				return exitOK
			}
			return runInit(*force, templateID, ctx)
		},
	}
}

func runInit(force bool, templateID string, ctx cli.Context) int {
	root, err := git.FindRepoRootOrCwd()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}

	cfgPath := config.DefaultConfigPath(root)
	assetsDir := config.DefaultAssetsPath(root)
	insultsPath := filepath.Join(assetsDir, "insults", "default.json")
	banterPath := filepath.Join(assetsDir, "banter", "default.json")

	if err := os.MkdirAll(filepath.Dir(insultsPath), 0o755); err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}
	if err := os.MkdirAll(filepath.Dir(banterPath), 0o755); err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}

	if err := ensureDefaultPack(root, insultsPath, "insults_default.json", force); err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}
	if err := ensureDefaultPack(root, banterPath, "banter_default.json", force); err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}

	tmpl, ok := findConfigTemplate(templateID)
	if !ok {
		fmt.Fprintln(ctx.Stderr, "init: unknown template:", templateID)
		printInitHelp(ctx)
		return exitUsage
	}

	templateBytes, err := loadTemplateBytes(root, tmpl.File)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}

	ciChecks, err := ci.ChecksFromGitHubActions(root)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}

	cfg, err := config.Parse(templateBytes)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}
	applyTemplateOverrides(root, templateID, cfg)
	if templateID == "manual" && len(ciChecks) > 0 {
		cfg.Checks = stripManualPlaceholder(cfg.Checks)
	}
	merge := mergeChecks(cfg.Checks, ciChecks)
	cfg.Checks = merge.Merged
	if err := config.Save(cfgPath, cfg); err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}
	if len(merge.Added) > 0 {
		fmt.Fprintf(ctx.Stdout, "Added %d CI checks from .github/workflows\n", len(merge.Added))
	}
	if len(merge.Skipped) > 0 {
		fmt.Fprintf(ctx.Stdout, "Skipped %d duplicate CI checks\n", len(merge.Skipped))
	}

	fmt.Fprintln(ctx.Stdout, "Created:", cfgPath)
	fmt.Fprintln(ctx.Stdout, "Ensured:", insultsPath)
	fmt.Fprintln(ctx.Stdout, "Ensured:", banterPath)
	fmt.Fprintln(ctx.Stdout, "Next: build-bouncer hook install")
	return exitOK
}

func newCheckCommand() cli.Command {
	return cli.Command{
		Name:    "check",
		Usage:   "check [--ci] [--verbose] [--hook] [--log-dir DIR] [--tail N] [--parallel N] [--fail-fast]",
		Summary: "Run configured checks.",
		Run: func(ctx cli.Context, args []string) int {
			return runCheck(args, ctx)
		},
	}
}

func runCheck(args []string, ctx cli.Context) int {
	fs := cli.NewFlagSet(ctx, "check")
	ci := fs.Bool("ci", false, "CI mode (no spinner/banter; no random insult)")
	verbose := fs.Bool("verbose", false, "stream full tool output to the terminal")
	hook := fs.Bool("hook", false, "hook mode (force spinner/banter even if stdout doesn't look like a TTY)")
	logDir := fs.String("log-dir", "", "directory to write failure logs (default: .git/build-bouncer/logs)")
	tail := fs.Int("tail", 0, "extra tail lines per failed check (verbose only)")
	parallel := fs.Int("parallel", 0, "max concurrent checks (default: 1 or config)")
	failFast := fs.Bool("fail-fast", false, "cancel remaining checks on first failure")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfgPath, cfgDir, err := config.FindConfigFromCwd()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "check:", err)
		return exitUsage
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "check:", err)
		return exitUsage
	}

	quietUI := !*verbose && !*ci && (*hook || ui.IsTerminal(os.Stdout))

	var bp *banter.Picker
	if quietUI {
		if p, err := banter.Load(cfgDir, banter.Config{File: cfg.Banter.File, Locale: cfg.Banter.Locale}); err == nil {
			bp = p
		}
	}

	var sp *ui.Spinner
	if quietUI {
		sp = ui.NewSpinner(os.Stdout)

		intro := ""
		if bp != nil {
			intro = bp.Pick("intro")
		}
		if strings.TrimSpace(intro) == "" {
			intro = "Checking the list..."
		}

		sp.Start(intro)
	}

	opts := runner.Options{
		CI:          *ci,
		Verbose:     *verbose || *ci,
		LogDir:      *logDir,
		MaxParallel: cfg.Runner.MaxParallel,
		FailFast:    cfg.Runner.FailFast || *failFast,
		Progress: func(e runner.ProgressEvent) {
			if sp == nil || bp == nil {
				return
			}
			if e.Stage == "start" {
				msg := bp.Pick("loading")
				if strings.TrimSpace(msg) == "" {
					msg = "Checking the list"
				}
				sp.SetMessage(fmt.Sprintf("%s (%d/%d)", msg, e.Index, e.Total))
			}
		},
	}
	if *parallel > 0 {
		opts.MaxParallel = *parallel
	}
	if opts.MaxParallel <= 0 {
		opts.MaxParallel = 1
	}

	rep, err := runner.RunAllReport(cfgDir, cfg, opts)
	if sp != nil {
		sp.Stop()
		fmt.Fprintln(ctx.Stdout, "")
	}
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "check:", err)
		return exitUsage
	}

	if len(rep.Failures) > 0 {
		if *verbose || *ci {
			fmt.Fprintln(ctx.Stderr, "")
			fmt.Fprintln(ctx.Stderr, "Blocked. Failed checks:")
			for _, f := range rep.Failures {
				fmt.Fprintf(ctx.Stderr, "  - %s\n", f)
			}
			if len(rep.Canceled) > 0 {
				fmt.Fprintln(ctx.Stderr, "")
				fmt.Fprintln(ctx.Stderr, "Canceled checks:")
				for _, c := range rep.Canceled {
					fmt.Fprintf(ctx.Stderr, "  - %s\n", c)
				}
			}
		}

		if *verbose {
			for _, f := range rep.Failures {
				reason := strings.TrimSpace(runner.ExtractWhy(f, rep.FailureTails[f]))
				if reason == "" {
					reason = strings.TrimSpace(rep.FailureHeadlines[f])
				}
				if reason != "" {
					fmt.Fprintln(ctx.Stderr, "")
					fmt.Fprintf(ctx.Stderr, "-- %s (why)\n", f)
					fmt.Fprintln(ctx.Stderr, reason)
				}
				if *tail > 0 {
					tailText := runner.TailLines(rep.FailureTails[f], *tail)
					if strings.TrimSpace(tailText) != "" {
						fmt.Fprintln(ctx.Stderr, "")
						fmt.Fprintf(ctx.Stderr, "-- %s (tail)\n", f)
						fmt.Fprintln(ctx.Stderr, tailText)
					}
				}
				if p := rep.LogFiles[f]; strings.TrimSpace(p) != "" {
					fmt.Fprintf(ctx.Stderr, "\nLog: %s\n", p)
				}
			}
		}

		if !*ci {
			insult := runner.PickInsult(cfgDir, cfg.Insults, rep)
			insult = strings.TrimSpace(insult)
			if insult != "" {
				fmt.Fprintln(ctx.Stderr, "")
				fmt.Fprintln(ctx.Stderr, insult)
			}
		}

		return exitRunFailed
	}

	if quietUI && bp != nil {
		if msg := strings.TrimSpace(bp.Pick("success")); msg != "" {
			fmt.Fprintln(ctx.Stdout, msg)
			return exitOK
		}
	}

	fmt.Fprintln(ctx.Stdout, "All checks passed.")
	return exitOK
}

func newHookCommand() cli.Command {
	return cli.Command{
		Name:    "hook",
		Usage:   "hook <install|status|uninstall> [options]",
		Summary: "Manage the git pre-push hook.",
		Run: func(ctx cli.Context, args []string) int {
			return runHook(args, ctx)
		},
	}
}

func runHook(args []string, ctx cli.Context) int {
	if len(args) < 1 {
		fmt.Fprintln(ctx.Stderr, "hook: missing subcommand (expected: install|status|uninstall)")
		return exitUsage
	}

	switch args[0] {
	case "install":
		fs := cli.NewFlagSet(ctx, "hook install")
		noCopy := fs.Bool("no-copy", false, "do not copy the build-bouncer binary into .git/hooks/bin")
		if err := fs.Parse(args[1:]); err != nil {
			return exitUsage
		}

		opts := hooks.InstallOptions{CopySelf: !*noCopy}
		if err := hooks.InstallPrePushHook(opts); err != nil {
			fmt.Fprintln(ctx.Stderr, "hook install:", err)
			return exitUsage
		}

		fmt.Fprintln(ctx.Stdout, "Installed git pre-push hook.")
		return exitOK

	case "status":
		st, err := hooks.GetStatus()
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "hook status:", err)
			return exitUsage
		}

		if !st.Installed {
			fmt.Fprintln(ctx.Stdout, "pre-push hook: not installed")
			return exitOK
		}

		fmt.Fprintln(ctx.Stdout, "pre-push hook: installed")
		fmt.Fprintln(ctx.Stdout, "path:", st.HookPath)
		fmt.Fprintln(ctx.Stdout, "installed by build-bouncer:", st.Ours)
		fmt.Fprintln(ctx.Stdout, "copied binary present:", st.CopiedBinary)
		return exitOK

	case "uninstall":
		fs := cli.NewFlagSet(ctx, "hook uninstall")
		force := fs.Bool("force", false, "remove hook even if it wasn't installed by build-bouncer")
		if err := fs.Parse(args[1:]); err != nil {
			return exitUsage
		}

		if err := hooks.UninstallPrePushHook(*force); err != nil {
			fmt.Fprintln(ctx.Stderr, "hook uninstall:", err)
			return exitUsage
		}

		fmt.Fprintln(ctx.Stdout, "Uninstalled git pre-push hook (and cleaned up hook binaries when possible).")
		return exitOK

	default:
		fmt.Fprintf(ctx.Stderr, "hook: unknown subcommand: %s\n", args[0])
		return exitUsage
	}
}
