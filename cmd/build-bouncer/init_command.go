package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/ci"
	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/git"
	"github.com/berniemackie97/build-bouncer/internal/tui"
)

func newInitCommand() cli.Command {
	return cli.Command{
		Name:    "init",
		Usage:   "init [--force] [--import-ci] [--dotnet [--solution PATH] [--node] | --template-flag]",
		Summary: "Create .buildbouncer/config.yaml and default insult/banter packs.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "init")
			force := fs.Bool("force", false, "replace existing configuration and default packs")
			importCI := fs.Bool("import-ci", false, "import supported GitHub Actions checks")
			solution := fs.String("solution", "", "select a repository-relative .sln or .slnx file with --dotnet")
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
			if *solution != "" && templateID != "dotnet" && templateID != "dotnet+node" {
				fmt.Fprintln(ctx.Stderr, "init: --solution requires --dotnet")
				return exitUsage
			}
			if templateID == "" {
				printInitHelp(ctx)
				return exitOK
			}
			return runInit(*force, *importCI, templateID, *solution, ctx)
		},
	}
}

func runInit(force, importCI bool, templateID, selected string, ctx cli.Context) (code int) {
	root, err := git.FindRepoRootOrCwd()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}

	cfgPath := config.DefaultConfigPath(root)
	if !force {
		for _, existingPath := range []string{cfgPath, config.LegacyConfigPath(root)} {
			if _, err := os.Lstat(existingPath); err == nil {
				fmt.Fprintln(ctx.Stderr, "init: configuration already exists (use --force to replace it):", existingPath)
				return exitUsage
			} else if !os.IsNotExist(err) {
				fmt.Fprintln(ctx.Stderr, "init:", err)
				return exitUsage
			}
		}
	}
	snapshot, err := snapshotSetup(root)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}
	modified := false
	defer func() {
		if code != exitOK && modified {
			if rollbackErr := snapshot.restore(); rollbackErr != nil {
				fmt.Fprintln(ctx.Stderr, "init rollback:", rollbackErr)
			}
		}
	}()
	assetsDir := config.DefaultAssetsPath(root)
	insultsPath := filepath.Join(assetsDir, "insults", "default.json")
	banterPath := filepath.Join(assetsDir, "banter", "default.json")

	mixed := templateID == "dotnet+node"
	baseTemplate := templateID
	if mixed {
		baseTemplate = "dotnet"
	}
	tmpl, ok := findConfigTemplate(baseTemplate)
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

	var ciChecks []config.Check
	if importCI {
		ciChecks, err = ci.ChecksFromGitHubActions(root)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "init:", err)
			return exitUsage
		}
	}

	cfg, err := config.Parse(templateBytes)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "init:", err)
		return exitUsage
	}
	adjust := applyTemplateOverrides(root, baseTemplate, cfg.Checks)
	if baseTemplate == "dotnet" {
		profile, solution, err := dotnetProfile(root, selected)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "init:", err)
			return exitUsage
		}
		adjust.Checks = profile
		adjust.Inputs["dotnet.solution"] = solution
	}
	if mixed {
		nodeChecks, nodeInputs, err := rootNodeProfile(root, true)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "init:", err)
			return exitUsage
		}
		adjust.Checks = append(adjust.Checks, stampGeneratedChecks(nodeChecks, "detector:node-scripts")...)
		adjust.Inputs = mergeInputs(adjust.Inputs, nodeInputs)
		adjust.Inputs["template.id"] = templateID
	}
	cfg.Checks = adjust.Checks
	if baseTemplate == "dotnet" && adjust.Inputs["dotnet.solution"] != "" {
		cfg.Runner.MaxParallel = 1
		cfg.Runner.FailFast = true
	}
	cfg.Meta.Template.ID = templateID
	cfg.Meta.Inputs = mergeInputs(cfg.Meta.Inputs, adjust.Inputs)
	templateSource := "template:" + baseTemplate
	if strings.TrimSpace(adjust.Source) != "" {
		templateSource = adjust.Source
	}
	cfg.Checks = stampGeneratedChecks(cfg.Checks, templateSource)
	if templateID == "manual" && len(ciChecks) > 0 {
		cfg.Checks = stripManualPlaceholder(cfg.Checks)
	}
	ciChecks = stampGeneratedChecks(ciChecks, "ci")
	merge := mergeChecks(cfg.Checks, ciChecks)
	cfg.Checks = merge.Merged
	modified = true
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
	save := config.Create
	if force {
		save = config.Save
	}
	if err := save(cfgPath, cfg); err != nil {
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
	fmt.Fprintln(ctx.Stdout, tui.Check("Insults: "+insultsPath))
	fmt.Fprintln(ctx.Stdout, tui.Check("Banter: "+banterPath))
	fmt.Fprintln(ctx.Stdout, "")
	fmt.Fprintln(ctx.Stdout, tui.Info("Next: build-bouncer hook install"))
	return exitOK
}
