package runner

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"build-bouncer/internal/config"
)

type Options struct {
	CI bool
}

func RunAll(root string, cfg *config.Config, _ Options) ([]string, error) {
	var failed []string

	for _, c := range cfg.Checks {
		fmt.Printf("==> %s\n", c.Name)

		dir := root
		if strings.TrimSpace(c.Cwd) != "" {
			dir = filepath.Join(root, filepath.FromSlash(c.Cwd))
		}

		exitCode, err := runOne(dir, c.Run, c.Env)
		if err != nil {
			return nil, err
		}

		if exitCode != 0 {
			failed = append(failed, c.Name)
			fmt.Printf("!! %s failed (exit %d)\n\n", c.Name, exitCode)
		} else {
			fmt.Printf("OK %s\n\n", c.Name)
		}
	}

	return failed, nil
}

func runOne(dir string, command string, env map[string]string) (int, error) {
	name, args := shellCommand(command)
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}

	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}

	return -1, err
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

func PickInsult(root string, ins config.Insults) string {
	path := filepath.Join(root, filepath.FromSlash(ins.File))
	b, err := os.ReadFile(path)
	if err != nil {
		return formatInsult(ins.Mode, "Nope. Something failed.")
	}

	var lines []string
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if lin != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return formatInsult(ins.Mode, "Nope. Something failed.")
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	chosen := lines[r.Intn(len(lines))]
	return formatInsult(ins.Mode, chosen)
}

func formatInsult(mode string, msg string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "polite":
		return "Blocked: " + msg
	case "nuclear":
		return strings.ToUpper(msg)
	default:
		return msg
	}
}
