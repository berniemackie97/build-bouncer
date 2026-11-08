package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/git"

	"github.com/berniemackie97/build-bouncer/internal/hooks"
	"github.com/berniemackie97/build-bouncer/internal/prompt"
	"github.com/berniemackie97/build-bouncer/internal/runner"
	"github.com/berniemackie97/build-bouncer/internal/tui"
	"github.com/berniemackie97/build-bouncer/internal/ui"

	"github.com/berniemackie97/build-bouncer/internal/banter"
)

func registerCommands(app *cli.App) {
	app.Register(newSetupCommand())
	app.Register(newInitCommand())
	app.Register(newCheckCommand())
	app.Register(newValidateCommand())
	app.Register(newDoctorCommand())
	app.Register(newCICommand())
	app.Register(newConfigureCommand())
	app.Register(newHookCommand())
	app.Register(newUninstallCommand())
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
	if *parallel < 0 || *parallel > 32 {
		fmt.Fprintln(ctx.Stderr, "check: --parallel must be between 0 and 32")
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
	banterEnabled := cfg.Banter.Enabled == nil || *cfg.Banter.Enabled

	var bp *banter.Picker
	if quietUI && banterEnabled {
		if p, err := banter.Load(cfgDir, banter.Config{File: cfg.Banter.File, Locale: cfg.Banter.Locale}); err == nil {
			bp = p
		}
	}
	if bp != nil {
		defer func() {
			if err := bp.Flush(); err != nil {
				fmt.Fprintln(ctx.Stderr, "banter state:", err)
			}
		}()
	}

	var sp *ui.Spinner
	if quietUI {
		sp = ui.NewSpinner(os.Stdout)

		intro := ""
		if banterEnabled && bp != nil {
			intro = bp.Pick("intro")
		}
		if strings.TrimSpace(intro) == "" {
			if banterEnabled {
				intro = "Checking the list..."
			} else {
				intro = "Running checks..."
			}
		}

		sp.Start(intro)
	}

	checkContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	opts := runner.Options{
		Context:          checkContext,
		CI:               *ci,
		Verbose:          *verbose || *ci,
		Output:           ctx.Stdout,
		LogDir:           *logDir,
		MaxLogBytes:      cfg.Runner.MaxLogBytes,
		MaxSavedLogs:     cfg.Runner.MaxSavedLogs,
		LogRetentionDays: cfg.Runner.LogRetentionDays,
		MaxParallel:      cfg.Runner.MaxParallel,
		FailFast:         cfg.Runner.FailFast || *failFast,
		Progress: func(e runner.ProgressEvent) {
			if sp == nil {
				return
			}
			if e.Stage == "start" {
				msg := ""
				if banterEnabled && bp != nil {
					msg = bp.Pick("loading")
				}
				if strings.TrimSpace(msg) == "" {
					if banterEnabled {
						msg = "Checking the list"
					} else {
						msg = "Running checks"
					}
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
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			printCanceledChecks(ctx.Stderr, rep)
			return exitRunFailed
		}
		if errors.Is(err, runner.ErrNoChecksExecuted) {
			return exitRunFailed
		}
		return exitUsage
	}

	if len(rep.Failures) > 0 {
		// Determine if we should block based on protection level
		protectionLevel := cfg.Protection.ProtectionLevel()
		shouldBlock := *ci || prompt.ShouldBlock(rep, protectionLevel)

		if *verbose || *ci {
			fmt.Fprintln(ctx.Stderr, "")
			fmt.Fprintln(ctx.Stderr, tui.Section("Failed Checks"))
			for _, f := range rep.Failures {
				fmt.Fprintln(ctx.Stderr, tui.Cross(f))
			}
			if len(rep.Skipped) > 0 {
				fmt.Fprintln(ctx.Stderr, "")
				fmt.Fprintln(ctx.Stderr, tui.Section("Skipped Checks"))
				for _, s := range rep.Skipped {
					reason := strings.TrimSpace(rep.SkipReasons[s])
					if reason == "" {
						fmt.Fprintln(ctx.Stderr, tui.Bullet(s))
						continue
					}
					fmt.Fprintf(ctx.Stderr, "%s %s\n", tui.Bullet(s), tui.Dim("("+reason+")"))
				}
			}
		}

		printCanceledChecks(ctx.Stderr, rep)

		if *verbose {
			for _, f := range rep.Failures {
				reason := strings.TrimSpace(runner.ExtractWhy(f, rep.FailureTails[f]))
				if reason == "" {
					reason = strings.TrimSpace(rep.FailureHeadlines[f])
				}
				if reason != "" {
					fmt.Fprintln(ctx.Stderr, "")
					fmt.Fprintln(ctx.Stderr, tui.Bold("  "+f))
					fmt.Fprintln(ctx.Stderr, tui.Dim("  ───────────────────────────────────────────"))
					lines := strings.Split(reason, "\n")
					for _, line := range lines {
						fmt.Fprintln(ctx.Stderr, "  "+line)
					}
				}
				if *tail > 0 {
					tailText := runner.TailLines(rep.FailureTails[f], *tail)
					if strings.TrimSpace(tailText) != "" {
						fmt.Fprintln(ctx.Stderr, "")
						fmt.Fprintln(ctx.Stderr, tui.Dim("  Last "+fmt.Sprint(*tail)+" lines:"))
						lines := strings.Split(tailText, "\n")
						for _, line := range lines {
							fmt.Fprintln(ctx.Stderr, tui.Dim("  "+line))
						}
					}
				}
				if p := rep.LogFiles[f]; strings.TrimSpace(p) != "" {
					fmt.Fprintln(ctx.Stderr, "")
					label := "Full log: "
					if rep.LogTruncated[f] {
						label = "Full log (truncated): "
					}
					fmt.Fprintln(ctx.Stderr, tui.Arrow(label+p))
				}
			}
		}

		// Show formatted failure summary for non-verbose, non-CI mode
		if !*verbose && !*ci {
			fmt.Fprintln(ctx.Stderr, "")
			fmt.Fprintln(ctx.Stderr, tui.Section("Failed Checks"))
			for _, f := range rep.Failures {
				fmt.Fprintln(ctx.Stderr, tui.Cross(f))
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

		if !shouldBlock {
			fmt.Fprintln(ctx.Stdout, "")
			fmt.Fprintln(ctx.Stdout, "Lax mode: explicitly allowed check failures. Allowing push.")
			return exitOK
		}

		// Interactive override prompt (only in hook mode during git push)
		// Skip prompt in CI mode, manual mode, or if terminal is not available
		if *hook && !*ci && cfg.Protection.IsInteractive() {
			if !prompt.CanOverride(rep, protectionLevel) {
				fmt.Fprintln(ctx.Stderr, "Push blocked: unavailable, canceled, and timed-out checks cannot be overridden.")
				return exitRunFailed
			}
			if !ui.IsTerminal(os.Stdin) {
				return exitRunFailed
			}
			result, err := prompt.AskOverride(os.Stdin, ctx.Stdout, ctx.Stderr, rep, protectionLevel)
			if err != nil {
				fmt.Fprintln(ctx.Stderr, "")
				fmt.Fprintln(ctx.Stderr, "Error reading input:", err)
				return exitRunFailed
			}

			if result.Override {
				fmt.Fprintln(ctx.Stdout, "")
				fmt.Fprintln(ctx.Stdout, tui.Success("✓ Override accepted - Push proceeding"))
				return exitOK
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

	if (*verbose || *ci) && len(rep.Skipped) > 0 {
		fmt.Fprintln(ctx.Stdout, "")
		fmt.Fprintln(ctx.Stdout, tui.Section("Skipped Checks"))
		for _, s := range rep.Skipped {
			reason := strings.TrimSpace(rep.SkipReasons[s])
			if reason == "" {
				fmt.Fprintln(ctx.Stdout, tui.Bullet(s))
				continue
			}
			fmt.Fprintf(ctx.Stdout, "%s %s\n", tui.Bullet(s), tui.Dim("("+reason+")"))
		}
	}

	fmt.Fprintln(ctx.Stdout, "")
	fmt.Fprintln(ctx.Stdout, tui.Success("✓ All checks passed"))
	return exitOK
}

func newHookCommand() cli.Command {
	return cli.Command{
		Name:    "hook",
		Usage:   "hook <install|status|uninstall> [--force] [--no-copy]",
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
		force := fs.Bool("force", false, "overwrite foreign, modified, or older hooks")
		if err := fs.Parse(args[1:]); err != nil {
			return exitUsage
		}

		root, err := git.FindRepoRoot()
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "hook install:", err)
			return exitUsage
		}
		if err := hookAdmission(root); err != nil {
			fmt.Fprintln(ctx.Stderr, "hook install:", err)
			return exitUsage
		}
		opts := hooks.InstallOptions{
			CopySelf: !*noCopy,
			Force:    *force,
		}
		if err := hooks.InstallPrePushHook(opts); err != nil {
			fmt.Fprintln(ctx.Stderr, "hook install:", err)
			return exitUsage
		}

		fmt.Fprintln(ctx.Stdout, tui.Success("✓ Installed git pre-push hook"))
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

		switch {
		case !st.Executable:
			fmt.Fprintln(ctx.Stdout, "pre-push hook: present but not executable by Git")
		case !st.Ours:
			fmt.Fprintln(ctx.Stdout, "pre-push hook: foreign hook present")
		case st.LegacyGenerated:
			fmt.Fprintln(ctx.Stdout, "pre-push hook: unmodified v3 hook; reinstall to upgrade")
		case !st.Current:
			fmt.Fprintln(ctx.Stdout, "pre-push hook: outdated or modified; inspect and use --force to reinstall")
		case st.UsesCopiedBinary && (!st.CopiedBinary || !st.CopiedBinaryMatchesRecord):
			fmt.Fprintln(ctx.Stdout, "pre-push hook: copied executable missing or unverified (PATH fallback may apply)")
		default:
			fmt.Fprintln(ctx.Stdout, "pre-push hook: installed")
		}
		fmt.Fprintln(ctx.Stdout, "path:", st.HookPath)
		fmt.Fprintln(ctx.Stdout, "installed by build-bouncer:", st.Ours)
		fmt.Fprintln(ctx.Stdout, "current build-bouncer hook:", st.Current)
		fmt.Fprintln(ctx.Stdout, "executable by Git:", st.Executable)
		fmt.Fprintln(ctx.Stdout, "copied binary present for this OS:", st.CopiedBinary)
		if st.UsesCopiedBinary {
			fmt.Fprintln(ctx.Stdout, "copied binary matches ownership record:", st.CopiedBinaryMatchesRecord)
			if st.CopiedBinaryRecordOrphaned {
				fmt.Fprintln(ctx.Stdout, "orphaned ownership record: inspect before hook install --force or hook uninstall --force")
			}
		}
		return exitOK

	case "uninstall":
		fs := cli.NewFlagSet(ctx, "hook uninstall")
		force := fs.Bool("force", false, "remove foreign, modified, or older hooks")
		if err := fs.Parse(args[1:]); err != nil {
			return exitUsage
		}

		if err := hooks.UninstallPrePushHook(*force); err != nil {
			fmt.Fprintln(ctx.Stderr, "hook uninstall:", err)
			return exitUsage
		}

		fmt.Fprintln(ctx.Stdout, tui.Success("✓ Uninstalled git pre-push hook"))
		return exitOK

	default:
		fmt.Fprintf(ctx.Stderr, "hook: unknown subcommand: %s\n", args[0])
		return exitUsage
	}
}
