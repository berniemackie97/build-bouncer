package main

import (
	"fmt"
	"strings"

	"build-bouncer/internal/cli"
)

func printInitHelp(ctx cli.Context) {
	fmt.Fprintln(ctx.Stdout, "build-bouncer init")
	fmt.Fprintln(ctx.Stdout, "")
	fmt.Fprintln(ctx.Stdout, "Choose a template:")
	for _, tmpl := range listConfigTemplates() {
		fmt.Fprintf(ctx.Stdout, "  %s\n", formatFlags(tmpl.Flags))
		fmt.Fprintf(ctx.Stdout, "    %s\n", tmpl.Summary)
	}
	fmt.Fprintln(ctx.Stdout, "")
	fmt.Fprintln(ctx.Stdout, "Examples:")
	fmt.Fprintln(ctx.Stdout, "  build-bouncer init --go")
	fmt.Fprintln(ctx.Stdout, "  build-bouncer setup --go")
}

func formatFlags(flags []string) string {
	if len(flags) == 0 {
		return ""
	}
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		out = append(out, "--"+strings.TrimPrefix(f, "--"))
	}
	return strings.Join(out, ", ")
}
