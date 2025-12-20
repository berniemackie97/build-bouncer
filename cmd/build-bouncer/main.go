package main

import (
	"os"

	"build-bouncer/internal/cli"
)

const (
	appName = "build-bouncer"
	version = "v0.1.0-dev"

	exitOK        = 0
	exitUsage     = 2
	exitRunFailed = 10
)

func main() {
	app := cli.NewApp(appName, version, os.Stdout, os.Stderr, exitUsage)
	registerCommands(app)
	os.Exit(app.Run(os.Args[1:]))
}
