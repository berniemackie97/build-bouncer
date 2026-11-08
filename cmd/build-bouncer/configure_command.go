package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

type configureOptions struct {
	dotnet     bool
	node       bool
	solution   string
	protection string
	parallel   string
	failFast   string
	check      string
	run        string
	remove     string
	yes        bool
}

func newConfigureCommand() cli.Command {
	return cli.Command{
		Name:    "configure",
		Usage:   "configure [--dotnet] [--node] [--solution FILE] [--protection LEVEL] [--parallel N] [--fail-fast true|false] [--check NAME --run CMD | --remove-check NAME] [--yes]",
		Summary: "Review and update repository checks and protection without editing YAML.",
		Run: func(ctx cli.Context, args []string) int {
			fs := cli.NewFlagSet(ctx, "configure")
			var opts configureOptions
			fs.BoolVar(&opts.dotnet, "dotnet", false, "refresh checks from a selected or unique .NET solution")
			fs.BoolVar(&opts.node, "node", false, "refresh checks from root package.json scripts")
			fs.StringVar(&opts.solution, "solution", "", "select a repository-relative .sln/.slnx path")
			fs.StringVar(&opts.protection, "protection", "", "set the protection policy: strict, moderate, or lax")
			fs.StringVar(&opts.parallel, "parallel", "", "maximum simultaneous checks (1-32)")
			fs.StringVar(&opts.failFast, "fail-fast", "", "cancel remaining checks after failure (true or false)")
			fs.StringVar(&opts.check, "check", "", "create or update a user-owned check")
			fs.StringVar(&opts.run, "run", "", "command for --check")
			fs.StringVar(&opts.remove, "remove-check", "", "remove a user-owned check")
			fs.BoolVar(&opts.yes, "yes", false, "apply the reviewed change without prompting")
			if err := fs.Parse(args); err != nil {
				return exitUsage
			}
			if fs.NArg() != 0 {
				fmt.Fprintln(ctx.Stderr, "configure: unexpected arguments:", fs.Args())
				return exitUsage
			}
			return runConfigure(opts, ctx, bufio.NewReader(os.Stdin))
		},
	}
}

func runConfigure(opts configureOptions, ctx cli.Context, input *bufio.Reader) int {
	if !opts.dotnet && !opts.node && opts.solution == "" && opts.protection == "" && opts.parallel == "" && opts.failFast == "" &&
		opts.check == "" && opts.run == "" && opts.remove == "" && !opts.yes {
		var err error
		opts, err = interactiveConfigure(ctx, input)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "configure:", err)
			return exitUsage
		}
		if !opts.dotnet && !opts.node && opts.protection == "" && opts.parallel == "" && opts.failFast == "" && opts.check == "" && opts.remove == "" {
			return exitOK
		}
	}
	if err := validateConfigureOptions(opts); err != nil {
		fmt.Fprintln(ctx.Stderr, "configure:", err)
		return exitUsage
	}
	path, root, err := config.FindConfigFromCwd()
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "configure:", err)
		return exitUsage
	}
	before, err := config.Load(path)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "configure:", err)
		return exitUsage
	}
	after := *before
	after.Checks = append([]config.Check(nil), before.Checks...)
	if opts.node {
		namespaced := opts.dotnet
		for _, check := range after.Checks {
			if check.Source == "template:dotnet" {
				namespaced = true
				break
			}
		}
		nodeChecks, inputs, err := rootNodeProfile(root, namespaced)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "configure:", err)
			return exitUsage
		}
		updated, err := replaceNodeChecks(after.Checks, stampGeneratedChecks(nodeChecks, "detector:node-scripts"))
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "configure:", err)
			return exitUsage
		}
		after.Checks = updated
		previousInputs := mergeInputs(after.Meta.Inputs, nil)
		delete(previousInputs, "node.workspace")
		delete(previousInputs, "node.turbo")
		after.Meta.Inputs = mergeInputs(previousInputs, inputs)
	}
	if opts.dotnet {
		profile, solution, err := dotnetProfile(root, opts.solution)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "configure:", err)
			return exitUsage
		}
		updated, err := replaceDotNetChecks(after.Checks, stampGeneratedChecks(profile, "template:dotnet"))
		if err != nil {
			fmt.Fprintln(ctx.Stderr, "configure:", err)
			return exitUsage
		}
		after.Checks = updated
		after.Meta.Inputs = mergeInputs(after.Meta.Inputs, map[string]string{"dotnet.solution": solution})
		if after.Runner.MaxParallel == 0 && opts.parallel == "" {
			after.Runner.MaxParallel = 1
		}
		if !after.Runner.FailFast && opts.failFast == "" {
			after.Runner.FailFast = true
		}
	}
	if opts.check != "" {
		if err := upsertCustomCheck(&after, opts.check, opts.run); err != nil {
			fmt.Fprintln(ctx.Stderr, "configure:", err)
			return exitUsage
		}
	}
	if opts.remove != "" {
		if err := removeCustomCheck(&after, opts.remove); err != nil {
			fmt.Fprintln(ctx.Stderr, "configure:", err)
			return exitUsage
		}
	}
	if opts.protection != "" {
		after.Protection.Level = opts.protection
	}
	if opts.parallel != "" {
		parallel, _ := strconv.Atoi(opts.parallel)
		after.Runner.MaxParallel = parallel
	}
	if opts.failFast != "" {
		after.Runner.FailFast = opts.failFast == "true"
	}
	if after.Runner.MaxParallel > 1 {
		for _, check := range after.Checks {
			if check.Source == "template:dotnet" && strings.Contains(check.Run, "--no-restore") {
				fmt.Fprintln(ctx.Stderr, "configure: dependent .NET checks require --parallel 1")
				return exitUsage
			}
		}
	}
	if reflect.DeepEqual(before, &after) {
		fmt.Fprintln(ctx.Stdout, "Configuration is already up to date.")
		return exitOK
	}
	fmt.Fprintln(ctx.Stdout, "Proposed configuration:")
	for _, check := range after.Checks {
		fmt.Fprintf(ctx.Stdout, "  %s: %s\n", check.Name, check.Run)
	}
	fmt.Fprintf(ctx.Stdout, "  protection: %s\n", after.Protection.ProtectionLevel())
	fmt.Fprintf(ctx.Stdout, "  runner: parallel=%d, failFast=%t\n", after.Runner.MaxParallel, after.Runner.FailFast)
	if !opts.yes {
		fmt.Fprint(ctx.Stdout, "Apply changes? [y/N]: ")
		answer, err := input.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintln(ctx.Stderr, "configure:", err)
			return exitUsage
		}
		if strings.ToLower(strings.TrimSpace(answer)) != "y" &&
			strings.ToLower(strings.TrimSpace(answer)) != "yes" {
			fmt.Fprintln(ctx.Stdout, "Configuration unchanged.")
			return exitOK
		}
	}
	if err := saveConfigured(path, before, &after); err != nil {
		fmt.Fprintln(ctx.Stderr, "configure:", err)
		return exitUsage
	}
	fmt.Fprintln(ctx.Stdout, "Updated:", path)
	fmt.Fprintln(ctx.Stdout, "Next: build-bouncer validate && build-bouncer check --verbose")
	return exitOK
}

func validateConfigureOptions(opts configureOptions) error {
	actions := 0
	if opts.dotnet || opts.node {
		actions++
	}
	if opts.check != "" || opts.run != "" {
		actions++
		if strings.TrimSpace(opts.check) == "" || strings.TrimSpace(opts.run) == "" {
			return fmt.Errorf("--check and --run must be used together")
		}
	}
	if opts.remove != "" {
		actions++
	}
	if actions > 1 {
		return fmt.Errorf("choose one check operation at a time")
	}
	if opts.solution != "" && !opts.dotnet {
		return fmt.Errorf("--solution requires --dotnet")
	}
	if opts.protection != "" && opts.protection != "strict" && opts.protection != "moderate" && opts.protection != "lax" {
		return fmt.Errorf("protection must be strict, moderate, or lax")
	}
	if opts.parallel != "" {
		max, err := strconv.Atoi(opts.parallel)
		if err != nil || max < 1 || max > 32 {
			return fmt.Errorf("--parallel must be between 1 and 32")
		}
	}
	if opts.failFast != "" && opts.failFast != "true" && opts.failFast != "false" {
		return fmt.Errorf("--fail-fast must be true or false")
	}
	if actions == 0 && opts.protection == "" && opts.parallel == "" && opts.failFast == "" {
		return fmt.Errorf("choose a check operation, --protection, --parallel, or --fail-fast")
	}
	return nil
}

func replaceDotNetChecks(existing, profile []config.Check) ([]config.Check, error) {
	return replaceGeneratedChecks(existing, profile, "template:dotnet")
}

func replaceNodeChecks(existing, profile []config.Check) ([]config.Check, error) {
	return replaceGeneratedChecks(existing, profile, "template:node", "detector:node-scripts")
}

func replaceGeneratedChecks(existing, profile []config.Check, sources ...string) ([]config.Check, error) {
	ownedSources := make(map[string]bool, len(sources))
	for _, source := range sources {
		ownedSources[source] = true
	}
	owned := make(map[string]bool)
	for _, check := range existing {
		if !ownedSources[check.Source] {
			continue
		}
		if check.ID == "" || check.ID != stableCheckID(check.Source, check) {
			return nil, fmt.Errorf("generated check %q has been modified; preserve it as a custom check before refreshing", check.Name)
		}
		owned[check.Name] = true
	}
	desired := make(map[string]bool, len(profile))
	for _, check := range profile {
		desired[check.Name] = true
	}
	for _, check := range existing {
		if !owned[check.Name] && desired[check.Name] {
			return nil, fmt.Errorf("check %q is already owned; cannot overwrite it with the selected profile", check.Name)
		}
	}
	result := make([]config.Check, 0, len(existing)-len(owned)+len(profile))
	inserted := false
	for _, check := range existing {
		if owned[check.Name] {
			if !inserted {
				result = append(result, profile...)
				inserted = true
			}
			continue
		}
		result = append(result, check)
	}
	if !inserted {
		result = append(result, profile...)
	}
	return result, nil
}

func upsertCustomCheck(cfg *config.Config, name, run string) error {
	name = strings.TrimSpace(name)
	run = strings.TrimSpace(run)
	for i, existing := range cfg.Checks {
		if existing.Name != name {
			continue
		}
		if existing.Source != "" {
			return fmt.Errorf("check %q is generated; use its profile to refresh it or edit YAML explicitly", name)
		}
		cfg.Checks[i].Run = run
		return nil
	}
	cfg.Checks = append(cfg.Checks, config.Check{Name: name, Run: run})
	return nil
}

func removeCustomCheck(cfg *config.Config, name string) error {
	for i, existing := range cfg.Checks {
		if existing.Name != name {
			continue
		}
		if existing.Source != "" {
			return fmt.Errorf("check %q is generated; remove or change its source explicitly in YAML", name)
		}
		cfg.Checks = append(cfg.Checks[:i:i], cfg.Checks[i+1:]...)
		if len(cfg.Checks) == 0 {
			return fmt.Errorf("cannot remove the last configured check")
		}
		return nil
	}
	return fmt.Errorf("check %q not found", name)
}
