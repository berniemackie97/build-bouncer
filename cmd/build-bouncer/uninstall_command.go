package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/git"
	"github.com/berniemackie97/build-bouncer/internal/hooks"
)

type uninstallOptions struct {
	force  bool
	purge  bool
	yes    bool
	dryRun bool
}

func newUninstallCommand() cli.Command {
	return cli.Command{
		Name:    "uninstall",
		Usage:   "uninstall [--force] [--purge --yes] [--dry-run]",
		Summary: "Remove the pre-push hook; optionally purge BuildBouncer data.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "uninstall")
			force := fs.Bool("force", false, "allow removal of a hook not installed by build-bouncer")
			purge := fs.Bool("purge", false, "also delete the project's configuration, assets, and state")
			yes := fs.Bool("yes", false, "confirm permanent deletion of configuration, assets, and state")
			dryRun := fs.Bool("dry-run", false, "show what would be removed without changing files")
			if err := fs.Parse(args); err != nil {
				return exitUsage
			}
			return runUninstallWithOptions(uninstallOptions{force: *force, purge: *purge, yes: *yes, dryRun: *dryRun}, ctx)
		},
	}
}

func runUninstall(force bool, ctx cli.Context) int {
	return runUninstallWithOptions(uninstallOptions{force: force}, ctx)
}

func runUninstallWithOptions(opts uninstallOptions, ctx cli.Context) int {
	if opts.purge && !opts.yes && !opts.dryRun {
		fmt.Fprintln(ctx.Stderr, "uninstall: --purge requires --yes; this permanently deletes configuration and user-edited packs")
		return exitUsage
	}
	root, err := git.FindRepoRootOrCwd()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "uninstall:", err)
		return exitUsage
	}
	repoRoot, repoErr := git.FindRepoRoot()
	if repoErr == nil {
		status, err := hooks.GetStatus()
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "uninstall hook:", err)
			return exitUsage
		}
		if status.Installed && !status.Ours && !opts.force {
			fmt.Fprintln(ctx.Stderr, "uninstall: pre-push hook was not installed by build-bouncer (use --force to remove)")
			return exitUsage
		}
		if status.Installed {
			if opts.dryRun {
				fmt.Fprintln(ctx.Stdout, "Would remove hook:", status.HookPath)
			} else if err := hooks.UninstallPrePushHook(opts.force); err != nil {
				fmt.Fprintln(ctx.Stderr, "uninstall hook:", err)
				return exitUsage
			} else {
				fmt.Fprintln(ctx.Stdout, "Removed hook:", status.HookPath)
			}
		} else {
			fmt.Fprintln(ctx.Stdout, "No installed pre-push hook found")
		}
	}

	if !opts.purge {
		fmt.Fprintln(ctx.Stdout, "Preserved configuration and state (use --purge --yes to delete)")
		return exitOK
	}
	var paths []string
	if repoErr == nil {
		stateDir, err := resolveGitStateDir(repoRoot)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "uninstall:", err)
			return exitUsage
		}
		paths = append(paths, stateDir)
	}
	paths = append(paths, config.ConfigDir(root), config.LegacyConfigPath(root))

	if opts.dryRun {
		for _, path := range paths {
			fmt.Fprintln(ctx.Stdout, "Would purge:", path)
		}
		return exitOK
	}

	var failures []error
	for _, path := range paths {
		removed, err := purgeArtifact(path, path == config.LegacyConfigPath(root))
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if removed {
			fmt.Fprintln(ctx.Stdout, "Removed:", path)
		}
	}
	if err := errors.Join(failures...); err != nil {
		fmt.Fprintln(ctx.Stderr, "uninstall cleanup:", err)
		return exitUsage
	}
	return exitOK
}

func resolveGitStateDir(root string) (string, error) {
	output, err := exec.Command("git", "-C", root, "rev-parse", "--git-path", "build-bouncer").Output()
	if err != nil {
		return "", fmt.Errorf("resolve Git state directory: %w", err)
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", errors.New("Git returned an empty state path")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return filepath.Clean(path), nil
}

func purgeArtifact(path string, regularFile bool) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if regularFile {
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("refusing to remove non-regular configuration file")
		}
		return true, os.Remove(path)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("refusing to remove non-directory data path")
	}
	if err := os.RemoveAll(path); err != nil {
		return false, err
	}
	return true, nil
}
