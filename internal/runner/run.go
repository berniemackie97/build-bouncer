package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
	CI          bool
	Verbose     bool
	LogDir      string
	MaxParallel int
	FailFast    bool
	Progress    func(e ProgressEvent)
}

type Report struct {
	Failures         []string
	FailureTails     map[string]string // checkName -> output tail
	FailureHeadlines map[string]string // checkName -> headline
	Canceled         []string
	Skipped          []string
	SkipReasons      map[string]string // checkName -> reason
	LogFiles         map[string]string // checkName -> full log (only on failure)
}

type limitedBuffer struct {
	max int
	buf []byte
}

type runOutcome struct {
	ExitCode int
	Tail     string
	LogPath  string
	TimedOut bool
	Timeout  time.Duration
	Canceled bool
	Skipped  bool
	Reason   string
}

type checkJob struct {
	idx   int
	check config.Check
}

type checkResult struct {
	idx     int
	name    string
	outcome runOutcome
	err     error
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
		Failures:         []string{},
		FailureTails:     map[string]string{},
		FailureHeadlines: map[string]string{},
		LogFiles:         map[string]string{},
		SkipReasons:      map[string]string{},
	}

	total := len(cfg.Checks)
	if total == 0 {
		return rep, nil
	}

	maxParallel := opts.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := make(chan checkJob)
	results := make(chan checkResult)
	stopCh := make(chan struct{})
	dispatchDone := make(chan struct{})

	var wg sync.WaitGroup
	printMu := &sync.Mutex{}

	worker := func() {
		defer wg.Done()
		for job := range jobs {
			if opts.Progress != nil {
				opts.Progress(ProgressEvent{
					Stage: "start",
					Index: job.idx + 1,
					Total: total,
					Check: job.check.Name,
				})
			}

			if reason := checkSkipReason(job.check); reason != "" {
				if opts.Progress != nil {
					opts.Progress(ProgressEvent{
						Stage:    "end",
						Index:    job.idx + 1,
						Total:    total,
						Check:    job.check.Name,
						ExitCode: 0,
					})
				}
				if opts.Verbose {
					printMu.Lock()
					fmt.Printf("~~ %s skipped (%s)\n\n", job.check.Name, reason)
					printMu.Unlock()
				}
				results <- checkResult{
					idx:     job.idx,
					name:    job.check.Name,
					outcome: runOutcome{ExitCode: 0, Skipped: true, Reason: reason},
					err:     nil,
				}
				continue
			}

			if opts.Verbose {
				printMu.Lock()
				fmt.Printf("==> %s\n", job.check.Name)
				printMu.Unlock()
			}

			dir := root
			if strings.TrimSpace(job.check.Cwd) != "" {
				dir = filepath.Join(root, filepath.FromSlash(job.check.Cwd))
			}

			outcome, err := runOne(ctx, root, dir, job.idx, job.check.Name, job.check.Run, job.check.Shell, job.check.Env, job.check.Timeout, opts)

			if opts.Progress != nil {
				opts.Progress(ProgressEvent{
					Stage:    "end",
					Index:    job.idx + 1,
					Total:    total,
					Check:    job.check.Name,
					ExitCode: outcome.ExitCode,
				})
			}

			if opts.Verbose {
				printMu.Lock()
				switch {
				case err != nil:
					fmt.Printf("!! %s error: %v\n\n", job.check.Name, err)
				case outcome.Canceled:
					fmt.Printf("!! %s canceled\n\n", job.check.Name)
				case outcome.TimedOut:
					fmt.Printf("!! %s timed out\n\n", job.check.Name)
				case outcome.ExitCode != 0:
					fmt.Printf("!! %s failed (exit %d)\n\n", job.check.Name, outcome.ExitCode)
				default:
					fmt.Printf("OK %s\n\n", job.check.Name)
				}
				printMu.Unlock()
			}

			results <- checkResult{
				idx:     job.idx,
				name:    job.check.Name,
				outcome: outcome,
				err:     err,
			}
		}
	}

	for i := 0; i < maxParallel; i++ {
		wg.Add(1)
		go worker()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	scheduled := make([]bool, total)

	go func() {
		defer close(dispatchDone)
		for i, c := range cfg.Checks {
			select {
			case <-stopCh:
				close(jobs)
				return
			case jobs <- checkJob{idx: i, check: c}:
				scheduled[i] = true
			}
		}
		close(jobs)
	}()

	var stopOnce sync.Once
	var firstErr error
	failFastTriggered := false

	for res := range results {
		if res.err != nil && firstErr == nil {
			firstErr = res.err
			stopOnce.Do(func() {
				close(stopCh)
				cancel()
			})
		}
		if res.err != nil {
			continue
		}

		if res.outcome.Skipped {
			rep.Skipped = append(rep.Skipped, res.name)
			if strings.TrimSpace(res.outcome.Reason) != "" {
				rep.SkipReasons[res.name] = res.outcome.Reason
			}
			continue
		}

		if res.outcome.Canceled {
			rep.Canceled = append(rep.Canceled, res.name)
			continue
		}

		if res.outcome.ExitCode != 0 || res.outcome.TimedOut {
			rep.Failures = append(rep.Failures, res.name)
			rep.FailureTails[res.name] = res.outcome.Tail
			if res.outcome.LogPath != "" {
				rep.LogFiles[res.name] = res.outcome.LogPath
			}
			if res.outcome.TimedOut {
				rep.FailureHeadlines[res.name] = fmt.Sprintf("Timed out after %s", res.outcome.Timeout)
			} else if headline := ExtractHeadline(res.name, res.outcome.Tail); strings.TrimSpace(headline) != "" {
				rep.FailureHeadlines[res.name] = headline
			}

			if opts.FailFast && !failFastTriggered {
				failFastTriggered = true
				stopOnce.Do(func() {
					close(stopCh)
					cancel()
				})
			}
		}
	}

	<-dispatchDone

	if failFastTriggered {
		for i, c := range cfg.Checks {
			if !scheduled[i] {
				rep.Canceled = append(rep.Canceled, c.Name)
			}
		}
	}

	order := make(map[string]int, total)
	for i, c := range cfg.Checks {
		order[c.Name] = i
	}
	sort.SliceStable(rep.Failures, func(i, j int) bool {
		return order[rep.Failures[i]] < order[rep.Failures[j]]
	})
	sort.SliceStable(rep.Canceled, func(i, j int) bool {
		return order[rep.Canceled[i]] < order[rep.Canceled[j]]
	})
	sort.SliceStable(rep.Skipped, func(i, j int) bool {
		return order[rep.Skipped[i]] < order[rep.Skipped[j]]
	})

	if firstErr != nil {
		return Report{}, firstErr
	}

	return rep, nil
}

func runOne(ctx context.Context, repoRoot string, dir string, idx int, checkName string, command string, shell string, env map[string]string, timeout time.Duration, opts Options) (runOutcome, error) {
	if ctx.Err() != nil {
		return runOutcome{ExitCode: 1, Canceled: true}, nil
	}

	tailBuf := newLimitedBuffer(128 * 1024)

	logDir := opts.LogDir
	if strings.TrimSpace(logDir) == "" {
		logDir = resolveDefaultLogDir(repoRoot)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return runOutcome{ExitCode: 1}, err
	}

	logName := fmt.Sprintf("%s_%02d_%s.log", time.Now().Format("20060102_150405"), idx, sanitize(checkName))
	logPath := filepath.Join(logDir, logName)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return runOutcome{ExitCode: 1}, err
	}

	var w io.Writer = io.MultiWriter(logFile, tailBuf)
	if opts.Verbose {
		w = io.MultiWriter(os.Stdout, logFile, tailBuf)
	}

	ctxRun := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		ctxRun, cancel = context.WithTimeout(ctx, timeout)
	}
	if cancel != nil {
		defer cancel()
	}

	fallbackShell := ""
	if strings.TrimSpace(shell) == "" {
		fallbackShell = defaultShellForCheck(checkName)
	}
	name, args := resolveCommand(shell, command, fallbackShell)
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = w

	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = adjustEnvForShell(name, cmd.Env)

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return runOutcome{ExitCode: 1}, err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctxRun.Done():
		_ = cmd.Process.Kill()
		<-done
		_ = logFile.Close()
		outcome := runOutcome{
			ExitCode: 1,
			Tail:     tailBuf.String(),
			LogPath:  logPath,
		}
		if ctxRun.Err() == context.DeadlineExceeded {
			outcome.TimedOut = true
			outcome.Timeout = timeout
		} else {
			outcome.Canceled = true
		}
		return outcome, nil
	case runErr := <-done:
		_ = logFile.Close()
		if runErr == nil {
			_ = os.Remove(logPath)
			return runOutcome{ExitCode: 0, Tail: tailBuf.String()}, nil
		}
		if ee, ok := runErr.(*exec.ExitError); ok {
			return runOutcome{ExitCode: ee.ExitCode(), Tail: tailBuf.String(), LogPath: logPath}, nil
		}
		return runOutcome{ExitCode: 1, Tail: tailBuf.String(), LogPath: logPath}, runErr
	}
}
