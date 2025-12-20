package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"build-bouncer/internal/banter"
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
	app.Register(newHookCommand())
}

func newSetupCommand() cli.Command {
	return cli.Command{
		Name:    "setup",
		Usage:   "setup [--force] [--no-copy] [--ci]",
		Summary: "Init config/packs, install hook, then run checks.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "setup")
			force := fs.Bool("force", false, "overwrite existing config + default packs")
			noCopy := fs.Bool("no-copy", false, "do not copy the build-bouncer binary into .git/hooks/bin")
			ci := fs.Bool("ci", false, "CI mode")
			if err := fs.Parse(args); err != nil {
				return exitUsage
			}
			return runSetup(*force, *noCopy, *ci, ctx)
		},
	}
}

func runSetup(force bool, noCopy bool, ci bool, ctx cli.Context) int {
	root, err := git.FindRepoRoot()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "setup:", err)
		return exitUsage
	}

	cfgPath := filepath.Join(root, ".buildbouncer.yaml")

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) || force {
		if code := runInit(force, ctx); code != exitOK {
			return code
		}
	} else if err != nil {
		fmt.Fprintln(ctx.Stderr, "setup:", err)
		return exitUsage
	} else {
		fmt.Fprintln(ctx.Stdout, "Config exists:", cfgPath)
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
		Usage:   "init [--force]",
		Summary: "Create .buildbouncer.yaml and default insult/banter packs.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "init")
			force := fs.Bool("force", false, "overwrite existing config + default packs")
			if err := fs.Parse(args); err != nil {
				return exitUsage
			}
			return runInit(*force, ctx)
		},
	}
}

func runInit(force bool, ctx cli.Context) int {
	root, err := git.FindRepoRootOrCwd()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}

	cfgPath := filepath.Join(root, ".buildbouncer.yaml")
	insultsPath := filepath.Join(root, "assets", "insults", "default.json")
	banterPath := filepath.Join(root, "assets", "banter", "default.json")

	if !force {
		if _, err := os.Stat(cfgPath); err == nil {
			fmt.Fprintln(ctx.Stderr, "init: .buildbouncer.yaml already exists (use --force to overwrite)")
			return exitUsage
		}
	}

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

	defaultCfg := `version: 1

checks:
  - name: "tests"
    run: "go test ./..."
  - name: "lint"
    run: "go vet ./..."

insults:
  mode: "snarky"   # polite | snarky | nuclear
  file: "assets/insults/default.json"
  locale: "en"

banter:
  file: "assets/banter/default.json"
  locale: "en"
`
	if err := os.WriteFile(cfgPath, []byte(defaultCfg), 0o644); err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
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
		Usage:   "check [--ci] [--verbose] [--hook] [--log-dir DIR] [--tail N]",
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
	tail := fs.Int("tail", 30, "number of output lines to show per failed check")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfgPath, cfgDir, err := config.FindConfigFromCwd(".buildbouncer.yaml")
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
		CI:      *ci,
		Verbose: *verbose || *ci,
		LogDir:  *logDir,
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
		fmt.Fprintln(ctx.Stderr, "")
		fmt.Fprintln(ctx.Stderr, "Blocked. Failed checks:")
		for _, f := range rep.Failures {
			fmt.Fprintf(ctx.Stderr, "  - %s\n", f)
		}

		if !opts.Verbose {
			for _, f := range rep.Failures {
				tailText := runner.TailLines(rep.FailureTails[f], *tail)
				if strings.TrimSpace(tailText) != "" {
					fmt.Fprintln(ctx.Stderr, "")
					fmt.Fprintf(ctx.Stderr, "-- %s (tail)\n", f)
					fmt.Fprintln(ctx.Stderr, tailText)
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
