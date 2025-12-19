package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"build-bouncer/internal/banter"
	"build-bouncer/internal/config"
	"build-bouncer/internal/git"
	"build-bouncer/internal/hooks"
	"build-bouncer/internal/runner"
	"build-bouncer/internal/ui"
)

const (
	exitOK        = 0
	exitUsage     = 2
	exitRunFailed = 10
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(exitUsage)
	}

	switch os.Args[1] {
	case "-h", "--help", "help":
		printHelp()
		os.Exit(exitOK)
	case "setup":
		os.Exit(cmdSetup(os.Args[2:]))
	case "init":
		os.Exit(cmdInit(os.Args[2:]))
	case "check":
		os.Exit(cmdCheck(os.Args[2:]))
	case "hook":
		os.Exit(cmdHook(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("build-bouncer v0.1.0-dev")
		os.Exit(exitOK)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printHelp()
		os.Exit(exitUsage)
	}
}

func printHelp() {
	fmt.Print(`build-bouncer

A terminal bouncer for your repo: runs checks and blocks git push when things fail.

Usage:
  build-bouncer setup [--force] [--no-copy] [--ci]
  build-bouncer init [--force]
  build-bouncer check [--ci] [--verbose] [--log-dir DIR] [--tail N]
  build-bouncer hook install [--no-copy]
  build-bouncer hook status
  build-bouncer hook uninstall [--force]

Notes:
  - Quiet mode (default) shows banter + spinner instead of raw output.
  - --verbose streams full tool output.
  - Failure logs are written to .git/build-bouncer/logs by default.
  - Config lives in .buildbouncer.yaml (YAML).
`)
}

func cmdSetup(args []string) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite existing config + default packs")
	noCopy := fs.Bool("no-copy", false, "do not copy the build-bouncer binary into .git/hooks/bin")
	ci := fs.Bool("ci", false, "CI mode")
	_ = fs.Parse(args)

	root, err := git.FindRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		return exitUsage
	}

	cfgPath := filepath.Join(root, ".buildbouncer.yaml")

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) || *force {
		initArgs := []string{}
		if *force {
			initArgs = append(initArgs, "--force")
		}
		if code := cmdInit(initArgs); code != exitOK {
			return code
		}
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		return exitUsage
	} else {
		fmt.Println("Config exists:", cfgPath)
	}

	opts := hooks.InstallOptions{CopySelf: !*noCopy}
	if err := hooks.InstallPrePushHook(opts); err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		return exitUsage
	}
	fmt.Println("Installed git pre-push hook.")

	checkArgs := []string{}
	if *ci {
		checkArgs = append(checkArgs, "--ci")
	}
	return cmdCheck(checkArgs)
}

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite existing config + default packs")
	_ = fs.Parse(args)

	root, err := git.FindRepoRootOrCwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}

	cfgPath := filepath.Join(root, ".buildbouncer.yaml")
	insultsPath := filepath.Join(root, "assets", "insults", "default.json")
	banterPath := filepath.Join(root, "assets", "banter", "default.json")

	if !*force {
		if _, err := os.Stat(cfgPath); err == nil {
			fmt.Fprintln(os.Stderr, "init: .buildbouncer.yaml already exists (use --force to overwrite)")
			return exitUsage
		}
	}

	if err := os.MkdirAll(filepath.Dir(insultsPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}
	if err := os.MkdirAll(filepath.Dir(banterPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}

	if err := ensureDefaultPack(root, insultsPath, "insults_default.json", *force); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}
	if err := ensureDefaultPack(root, banterPath, "banter_default.json", *force); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
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
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}

	fmt.Println("Created:", cfgPath)
	fmt.Println("Ensured:", insultsPath)
	fmt.Println("Ensured:", banterPath)
	fmt.Println("Next: build-bouncer hook install")
	return exitOK
}

func ensureDefaultPack(targetRoot string, destPath string, templateName string, force bool) error {
	if !force {
		if _, err := os.Stat(destPath); err == nil {
			return nil
		}
	}

	templateBytes, err := loadTemplateBytes(targetRoot, templateName)
	if err != nil {
		return err
	}

	return os.WriteFile(destPath, templateBytes, 0o644)
}

func loadTemplateBytes(targetRoot string, templateName string) ([]byte, error) {
	candidates := []string{
		filepath.Join(targetRoot, "assets", "templates", templateName),
	}

	if dir := strings.TrimSpace(os.Getenv("BUILDBOUNCER_TEMPLATES_DIR")); dir != "" {
		candidates = append(candidates,
			filepath.Join(dir, templateName),
			filepath.Join(dir, "templates", templateName),
		)
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "templates", templateName),
			filepath.Join(exeDir, "assets", "templates", templateName),
			filepath.Join(exeDir, "..", "share", "build-bouncer", "templates", templateName),
			filepath.Join(exeDir, "..", "libexec", "build-bouncer", "templates", templateName),
		)
	}

	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return b, nil
		}
	}

	return nil, errors.New("template not found: " + templateName + " (expected assets/templates or set BUILDBOUNCER_TEMPLATES_DIR)")
}

func cmdHook(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "hook: missing subcommand (expected: install|status|uninstall)")
		return exitUsage
	}

	switch args[0] {
	case "install":
		fs := flag.NewFlagSet("hook install", flag.ContinueOnError)
		noCopy := fs.Bool("no-copy", false, "do not copy the build-bouncer binary into .git/hooks/bin")
		_ = fs.Parse(args[1:])

		opts := hooks.InstallOptions{CopySelf: !*noCopy}
		if err := hooks.InstallPrePushHook(opts); err != nil {
			fmt.Fprintln(os.Stderr, "hook install:", err)
			return exitUsage
		}

		fmt.Println("Installed git pre-push hook.")
		return exitOK

	case "status":
		st, err := hooks.GetStatus()
		if err != nil {
			fmt.Fprintln(os.Stderr, "hook status:", err)
			return exitUsage
		}

		if !st.Installed {
			fmt.Println("pre-push hook: not installed")
			return exitOK
		}

		fmt.Println("pre-push hook: installed")
		fmt.Println("path:", st.HookPath)
		fmt.Println("installed by build-bouncer:", st.Ours)
		fmt.Println("copied binary present:", st.CopiedBinary)
		return exitOK

	case "uninstall":
		fs := flag.NewFlagSet("hook uninstall", flag.ContinueOnError)
		force := fs.Bool("force", false, "remove hook even if it wasn't installed by build-bouncer")
		_ = fs.Parse(args[1:])

		if err := hooks.UninstallPrePushHook(*force); err != nil {
			fmt.Fprintln(os.Stderr, "hook uninstall:", err)
			return exitUsage
		}

		fmt.Println("Uninstalled git pre-push hook (and cleaned up hook binaries when possible).")
		return exitOK

	default:
		fmt.Fprintf(os.Stderr, "hook: unknown subcommand: %s\n", args[0])
		return exitUsage
	}
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	ci := fs.Bool("ci", false, "CI mode (no spinner/banter; no random insult)")
	verbose := fs.Bool("verbose", false, "stream full tool output to the terminal")
	logDir := fs.String("log-dir", "", "directory to write failure logs (default: .git/build-bouncer/logs)")
	tail := fs.Int("tail", 30, "number of output lines to show per failed check")
	_ = fs.Parse(args)

	cfgPath, cfgDir, err := config.FindConfigFromCwd(".buildbouncer.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return exitUsage
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return exitUsage
	}

	quietUI := !*verbose && !*ci && ui.IsTerminal(os.Stdout)

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
				// Don’t leak check names in quiet mode.
				sp.SetMessage(fmt.Sprintf("%s (%d/%d)", msg, e.Index, e.Total))
			}
		},
	}

	rep, err := runner.RunAllReport(cfgDir, cfg, opts)
	if sp != nil {
		sp.Stop()
		fmt.Println("")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return exitUsage
	}

	if len(rep.Failures) > 0 {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Blocked. Failed checks:")
		for _, f := range rep.Failures {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}

		if !opts.Verbose {
			for _, f := range rep.Failures {
				tailText := runner.TailLines(rep.FailureTails[f], *tail)
				if strings.TrimSpace(tailText) != "" {
					fmt.Fprintln(os.Stderr, "")
					fmt.Fprintf(os.Stderr, "-- %s (tail)\n", f)
					fmt.Fprintln(os.Stderr, tailText)
				}
				if p := rep.LogFiles[f]; strings.TrimSpace(p) != "" {
					fmt.Fprintf(os.Stderr, "\nLog: %s\n", p)
				}
			}
		}

		if !*ci {
			insult := runner.PickInsult(cfgDir, cfg.Insults, rep)
			insult = strings.TrimSpace(insult)
			if insult != "" {
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, insult)
			}
		}

		return exitRunFailed
	}

	// Success output:
	if quietUI && bp != nil {
		if msg := strings.TrimSpace(bp.Pick("success")); msg != "" {
			fmt.Println(msg)
			return exitOK
		}
	}

	fmt.Println("All checks passed.")
	return exitOK
}
