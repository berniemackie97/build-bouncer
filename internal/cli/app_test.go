package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestAppRunsRegisteredCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := NewApp("build-bouncer", "v-test", &stdout, &stderr, 2)

	app.Register(Command{
		Name:    "echo",
		Usage:   "echo [args]",
		Summary: "test command",
		Run: func(ctx Context, args []string) int {
			if len(args) != 2 || args[0] != "a" || args[1] != "b" {
				t.Fatalf("unexpected args: %v", args)
			}
			ctx.Stdout.Write([]byte("ok"))
			return 7
		},
	})

	code := app.Run([]string{"echo", "a", "b"})
	if code != 7 {
		t.Fatalf("expected exit 7, got %d", code)
	}
	if strings.TrimSpace(stdout.String()) != "ok" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestAppUnknownCommandPrintsHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := NewApp("build-bouncer", "v-test", &stdout, &stderr, 3)
	app.Register(Command{
		Name:    "known",
		Usage:   "known",
		Summary: "known command",
		Run: func(ctx Context, args []string) int {
			return 0
		},
	})

	code := app.Run([]string{"missing"})
	if code != 3 {
		t.Fatalf("expected usage exit code 3, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Unknown command") {
		t.Fatalf("expected unknown command message, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Commands:") {
		t.Fatalf("expected help output, got %q", stdout.String())
	}
}
