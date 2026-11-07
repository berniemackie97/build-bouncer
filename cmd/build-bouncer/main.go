package main

import (
	"os"

	"github.com/berniemackie97/build-bouncer/internal/cli"
)

const (
	appName = "build-bouncer"

	exitOK        = 0
	exitUsage     = 2
	exitRunFailed = 10
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	displayVersion := version
	if commit != "unknown" {
		displayVersion += " (" + commit + ")"
	}
	app := cli.NewApp(appName, displayVersion, os.Stdout, os.Stderr, exitUsage)
	registerCommands(app)
	os.Exit(app.Run(os.Args[1:]))
}
