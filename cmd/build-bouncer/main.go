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
	fmt.Println(`build-bouncer

A terminal bouncer for your repo: runs checks and blocks git push when things fail.

Usage:
  build-bouncer init [--force]
  build-bouncer check [--ci]
  build-bouncer hook install [--no-copy]

Notes:
  - 'hook install' installs a git pre-push hook that runs 'build-bouncer check'.
  - Config lives in .buildbouncer.yaml (YAML).
`)
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
		fmt.Fprintln(os.Stderr, "hook: missing subcommand (expected: install)")
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
