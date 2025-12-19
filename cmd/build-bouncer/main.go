package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"build-bouncer/internal/config"
	"build-bouncer/internal/git"
	"build-bouncer/internal/hooks"
	"build-bouncer/internal/runner"
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
  build-bouncer check [--ci]
  build-bouncer hook install [--no-copy]
  build-bouncer hook status
  build-bouncer hook uninstall [--force]

Notes:
  - 'hook install' installs a git pre-push hook that runs 'build-bouncer check'.
  - Config lives in .buildbouncer.yaml (YAML).
`)
}

func cmdSetup(args []string) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite existing config + default insults file")
	noCopy := fs.Bool("no-copy", false, "do not copy the build-bouncer binary into .git/hooks/bin")
	ci := fs.Bool("ci", false, "CI mode (less sass)")
	_ = fs.Parse(args)

	// Setup requires a git repo (because it installs hooks).
	root, err := git.FindRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		return exitUsage
	}

	cfgPath := filepath.Join(root, ".buildbouncer.yaml")

	// Ensure config exists (create if missing; overwrite if --force).
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

	// Install hook.
	opts := hooks.InstallOptions{
		CopySelf: !*noCopy,
	}
	if err := hooks.InstallPrePushHook(opts); err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		return exitUsage
	}
	fmt.Println("Installed git pre-push hook.")

	// Run a check once so user sees it working.
	checkArgs := []string{}
	if *ci {
		checkArgs = append(checkArgs, "--ci")
	}
	return cmdCheck(checkArgs)
}

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite existing config + default insults file")
	_ = fs.Parse(args)

	root, err := git.FindRepoRootOrCwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}

	cfgPath := filepath.Join(root, ".buildbouncer.yaml")
	insultsPath := filepath.Join(root, "assets", "insults", "default.txt")

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

	defaultInsults := "Nice try. The build says no.\nYour tests are having a bad day. So are you.\nThat push is not going anywhere, champ.\n"
	if *force {
		_ = os.WriteFile(insultsPath, []byte(defaultInsults), 0o644)
	} else {
		if _, err := os.Stat(insultsPath); os.IsNotExist(err) {
			_ = os.WriteFile(insultsPath, []byte(defaultInsults), 0o644)
		}
	}

	defaultCfg := `version: 1

checks:
  - name: "tests"
    run: "go test ./..."
  - name: "lint"
    run: "go vet ./..."

insults:
  mode: "snarky"   # polite | snarky | nuclear
  file: "assets/insults/default.txt"
`
	if err := os.WriteFile(cfgPath, []byte(defaultCfg), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}

	fmt.Println("Created:", cfgPath)
	fmt.Println("Ensured:", insultsPath)
	fmt.Println("Next: build-bouncer hook install")
	return exitOK
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

		opts := hooks.InstallOptions{
			CopySelf: !*noCopy,
		}

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
	ci := fs.Bool("ci", false, "CI mode (less sass, no random insult)")
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

	failures, err := runner.RunAll(cfgDir, cfg, runner.Options{CI: *ci})
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return exitUsage
	}

	if len(failures) > 0 {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Blocked. Failed checks:")
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}

		if !*ci {
			insult := runner.PickInsult(cfgDir, cfg.Insults)
			insult = strings.TrimSpace(insult)
			if insult != "" {
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, insult)
			}
		}

		return exitRunFailed
	}

	fmt.Println("All checks passed.")
	return exitOK
}
