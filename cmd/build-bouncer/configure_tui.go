package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/cli"
)

func interactiveConfigure(ctx cli.Context, input *bufio.Reader) (configureOptions, error) {
	fmt.Fprintln(ctx.Stdout, "Configure BuildBouncer")
	fmt.Fprintln(ctx.Stdout, "  1. Refresh checks from a .NET solution")
	fmt.Fprintln(ctx.Stdout, "  2. Change push protection")
	fmt.Fprintln(ctx.Stdout, "  3. Add or update a custom check")
	fmt.Fprintln(ctx.Stdout, "  4. Remove a custom check")
	fmt.Fprintln(ctx.Stdout, "  5. Change runner settings")
	fmt.Fprintln(ctx.Stdout, "  6. Cancel")
	choice, err := readConfigureValue(ctx, input, "Choose [1-6]: ")
	if err != nil {
		return configureOptions{}, err
	}
	var opts configureOptions
	switch choice {
	case "1":
		opts.dotnet = true
		opts.solution, err = readConfigureValue(ctx, input, "Solution filename (Enter to detect): ")
	case "2":
		opts.protection, err = readConfigureValue(ctx, input, "Protection [strict/moderate/lax]: ")
	case "3":
		opts.check, err = readConfigureValue(ctx, input, "Check name: ")
		if err == nil {
			opts.run, err = readConfigureValue(ctx, input, "Command: ")
		}
	case "4":
		opts.remove, err = readConfigureValue(ctx, input, "Check name to remove: ")
	case "5":
		opts.parallel, err = readConfigureValue(ctx, input, "Maximum parallel checks (1-32, Enter to keep): ")
		if err == nil {
			opts.failFast, err = readConfigureValue(ctx, input, "Fail fast (true/false, Enter to keep): ")
		}
	case "6", "":
		return configureOptions{}, nil
	default:
		return configureOptions{}, fmt.Errorf("unknown choice %q", choice)
	}
	return opts, err
}

func readConfigureValue(ctx cli.Context, input *bufio.Reader, question string) (string, error) {
	fmt.Fprint(ctx.Stdout, question)
	answer, err := input.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF && answer == "" {
		return "", io.EOF
	}
	return strings.TrimSpace(answer), nil
}
