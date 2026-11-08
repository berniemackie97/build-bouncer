package main

import (
	"fmt"
	"io"

	"github.com/berniemackie97/build-bouncer/internal/runner"
	"github.com/berniemackie97/build-bouncer/internal/tui"
)

func printCanceledChecks(output io.Writer, report runner.Report) {
	if len(report.Canceled) == 0 {
		return
	}
	fmt.Fprintln(output, "")
	fmt.Fprintln(output, tui.Section("Canceled Checks"))
	for _, name := range report.Canceled {
		fmt.Fprintln(output, tui.Bullet(name))
		if path := report.LogFiles[name]; path != "" {
			label := "Saved log: "
			if report.LogTruncated[name] {
				label = "Saved log (truncated): "
			}
			fmt.Fprintln(output, tui.Arrow(label+path))
		}
	}
}
