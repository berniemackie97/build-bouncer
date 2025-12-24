package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/git"
	"github.com/berniemackie97/build-bouncer/internal/hooks"
)

func newUninstallCommand() cli.Command {
	return cli.Command{
		Name:    "uninstall",
		Usage:   "uninstall [--force]",
		Summary: "Remove build-bouncer artifacts from this repo.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "uninstall")
			force := fs.Bool("force", false, "remove hook even if it wasn't installed by build-bouncer")
			if err := fs.Parse(args); err != nil {
				return exitUsage
			}
			return runUninstall(*force, ctx)
		},
	}
}

func runUninstall(force bool, ctx cli.Context) int {
	root, err := git.FindRepoRootOrCwd()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "uninstall:", err)
		return exitUsage
	}

	var hookErr error
	if isDir(filepath.Join(root, ".git")) {
		if err := hooks.UninstallPrePushHook(force); err != nil {
			hookErr = err
			fmt.Fprintln(ctx.Stderr, "uninstall hook:", err)
		} else {
			fmt.Fprintln(ctx.Stdout, "Removed: .git/hooks/pre-push")
		}
		if removed := removePath(filepath.Join(root, ".git", "build-bouncer")); removed {
			fmt.Fprintln(ctx.Stdout, "Removed: .git/build-bouncer")
		}
	}

	if removed := removePath(config.ConfigDir(root)); removed {
		fmt.Fprintln(ctx.Stdout, "Removed:", config.ConfigDirName)
	}
	if removed := removeFile(config.LegacyConfigPath(root)); removed {
		fmt.Fprintln(ctx.Stdout, "Removed:", config.LegacyConfigName)
	}

	if hookErr != nil {
		return exitUsage
	}

	return exitOK
}

func removePath(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	_ = os.RemoveAll(path)
	return true
}

func removeFile(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	_ = os.Remove(path)
	return true
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
