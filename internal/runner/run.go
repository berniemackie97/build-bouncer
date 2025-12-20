package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"build-bouncer/internal/config"
)

type ProgressEvent struct {
	Stage    string // start | end
	Index    int
	Total    int
	Check    string
	ExitCode int
}

type Options struct {
	CI       bool
	Verbose  bool
	LogDir   string
	Progress func(e ProgressEvent)
}

type Report struct {
	Failures     []string
	FailureTails map[string]string // checkName -> output tail
	LogFiles     map[string]string // checkName -> full log (only on failure)
}

type limitedBuffer struct {
	max int
	buf []byte
}

func newLimitedBuffer(max int) *limitedBuffer {
	return &limitedBuffer{max: max, buf: make([]byte, 0, max)}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		return len(p), nil
	}
	if len(p) >= b.max {
		b.buf = append(b.buf[:0], p[len(p)-b.max:]...)
		return len(p), nil
	}
	needed := len(b.buf) + len(p) - b.max
	if needed > 0 {
		b.buf = b.buf[needed:]
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *limitedBuffer) String() string { return string(b.buf) }

func RunAllReport(root string, cfg *config.Config, opts Options) (Report, error) {
	rep := Report{
		Failures:     []string{},
		FailureTails: map[string]string{},
		LogFiles:     map[string]string{},
	}

	total := len(cfg.Checks)

	for i, c := range cfg.Checks {
		if opts.Progress != nil {
			opts.Progress(ProgressEvent{
				Stage: "start",
				Index: i + 1,
				Total: total,
				Check: c.Name,
			})
		}

		if opts.Verbose {
			fmt.Printf("==> %s\n", c.Name)
		}

		dir := root
		if strings.TrimSpace(c.Cwd) != "" {
			dir = filepath.Join(root, filepath.FromSlash(c.Cwd))
		}

		exitCode, tail, logPath, err := runOne(root, dir, i, c.Name, c.Run, c.Env, opts)
		if err != nil {
			return Report{}, err
		}

		if opts.Progress != nil {
			opts.Progress(ProgressEvent{
				Stage:    "end",
				Index:    i + 1,
				Total:    total,
				Check:    c.Name,
				ExitCode: exitCode,
			})
		}

		if exitCode != 0 {
			rep.Failures = append(rep.Failures, c.Name)
			rep.FailureTails[c.Name] = tail
			if logPath != "" {
				rep.LogFiles[c.Name] = logPath
			}
			if opts.Verbose {
				fmt.Printf("!! %s failed (exit %d)\n\n", c.Name, exitCode)
			}
		} else if opts.Verbose {
			fmt.Printf("OK %s\n\n", c.Name)
		}
	}

	return rep, nil
}

func runOne(repoRoot string, dir string, idx int, checkName string, command string, env map[string]string, opts Options) (int, string, string, error) {
	tailBuf := newLimitedBuffer(128 * 1024)

	logDir := opts.LogDir
	if strings.TrimSpace(logDir) == "" {
		logDir = resolveDefaultLogDir(repoRoot)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return 1, "", "", err
	}

	logName := fmt.Sprintf("%s_%02d_%s.log", time.Now().Format("20060102_150405"), idx, sanitize(checkName))
	logPath := filepath.Join(logDir, logName)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return 1, "", "", err
	}

	var w io.Writer = io.MultiWriter(logFile, tailBuf)
	if opts.Verbose {
		w = io.MultiWriter(os.Stdout, logFile, tailBuf)
	}

	name, args := shellCommand(command)
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = w

	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	runErr := cmd.Run()
	_ = logFile.Close()

	if runErr == nil {
		_ = os.Remove(logPath)
		return 0, tailBuf.String(), "", nil
	}

	if ee, ok := runErr.(*exec.ExitError); ok {
		return ee.ExitCode(), tailBuf.String(), logPath, nil
	}

	return 1, tailBuf.String(), logPath, runErr
}
