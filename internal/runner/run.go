// Package runner executes configured checks and produces a report of failures, skips, and logs.
// It is the core runtime that powers build-bouncer.
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

	"github.com/berniemackie97/build-bouncer/internal/config"
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
	maxBytes int
	buffer   []byte
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
	index int
	check config.Check
}

type checkResult struct {
	index   int
	name    string
	outcome runOutcome
	runErr  error
}

func newLimitedBuffer(maxBytes int) *limitedBuffer {
	return &limitedBuffer{
		maxBytes: maxBytes,
		buffer:   make([]byte, 0, max(0, maxBytes)),
	}
}

func (buf *limitedBuffer) Write(payload []byte) (int, error) {
	if buf.maxBytes <= 0 {
		return len(payload), nil
	}

	if len(payload) >= buf.maxBytes {
		buf.buffer = append(buf.buffer[:0], payload[len(payload)-buf.maxBytes:]...)
		return len(payload), nil
	}

	excessBytes := (len(buf.buffer) + len(payload)) - buf.maxBytes
	if excessBytes > 0 {
		buf.buffer = buf.buffer[excessBytes:]
	}

	buf.buffer = append(buf.buffer, payload...)
	return len(payload), nil
}

func (buf *limitedBuffer) String() string {
	return string(buf.buffer)
}

func RunAllReport(repoRoot string, configuration *config.Config, options Options) (Report, error) {
	report := Report{
		Failures:         []string{},
		FailureTails:     map[string]string{},
		FailureHeadlines: map[string]string{},
		LogFiles:         map[string]string{},
		SkipReasons:      map[string]string{},
	}

	totalChecks := len(configuration.Checks)
	if totalChecks == 0 {
		return report, nil
	}

	maxParallel := options.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 1
	}

	runContext, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	jobsChannel := make(chan checkJob)
	resultsChannel := make(chan checkResult)

	stopDispatch := make(chan struct{})
	dispatchDone := make(chan struct{})

	var workerGroup sync.WaitGroup
	var outputMutex sync.Mutex

	workerFn := func() {
		defer workerGroup.Done()

		for job := range jobsChannel {
			checkDefinition := job.check
			checkName := checkDefinition.Name

			if options.Progress != nil {
				options.Progress(ProgressEvent{
					Stage: "start",
					Index: job.index + 1,
					Total: totalChecks,
					Check: checkName,
				})
			}

			if skipReason := checkSkipReason(checkDefinition); strings.TrimSpace(skipReason) != "" {
				if options.Progress != nil {
					options.Progress(ProgressEvent{
						Stage:    "end",
						Index:    job.index + 1,
						Total:    totalChecks,
						Check:    checkName,
						ExitCode: 0,
					})
				}

				if options.Verbose {
					outputMutex.Lock()
					fmt.Printf("~~ %s skipped (%s)\n\n", checkName, skipReason)
					outputMutex.Unlock()
				}

				resultsChannel <- checkResult{
					index: job.index,
					name:  checkName,
					outcome: runOutcome{
						ExitCode: 0,
						Skipped:  true,
						Reason:   skipReason,
					},
					runErr: nil,
				}
				continue
			}

			if options.Verbose {
				outputMutex.Lock()
				fmt.Printf("==> %s\n", checkName)
				outputMutex.Unlock()
			}

			workingDirectory := repoRoot
			if strings.TrimSpace(checkDefinition.Cwd) != "" {
				workingDirectory = filepath.Join(repoRoot, filepath.FromSlash(checkDefinition.Cwd))
			}

			outcome, runErr := runOne(
				runContext,
				repoRoot,
				workingDirectory,
				job.index,
				checkName,
				checkDefinition.Run,
				checkDefinition.Shell,
				checkDefinition.Env,
				checkDefinition.Timeout,
				options,
			)

			if options.Progress != nil {
				options.Progress(ProgressEvent{
					Stage:    "end",
					Index:    job.index + 1,
					Total:    totalChecks,
					Check:    checkName,
					ExitCode: outcome.ExitCode,
				})
			}

			if options.Verbose {
				outputMutex.Lock()
				switch {
				case runErr != nil:
					fmt.Printf("!! %s error: %v\n\n", checkName, runErr)
				case outcome.Canceled:
					fmt.Printf("!! %s canceled\n\n", checkName)
				case outcome.TimedOut:
					fmt.Printf("!! %s timed out\n\n", checkName)
				case outcome.ExitCode != 0:
					fmt.Printf("!! %s failed (exit %d)\n\n", checkName, outcome.ExitCode)
				default:
					fmt.Printf("OK %s\n\n", checkName)
				}
				outputMutex.Unlock()
			}

			resultsChannel <- checkResult{
				index:   job.index,
				name:    checkName,
				outcome: outcome,
				runErr:  runErr,
			}
		}
	}

	for workerIndex := 0; workerIndex < maxParallel; workerIndex++ {
		workerGroup.Add(1)
		go workerFn()
	}

	go func() {
		workerGroup.Wait()
		close(resultsChannel)
	}()

	jobScheduled := make([]bool, totalChecks)

	go func() {
		defer close(dispatchDone)

		for checkIndex, checkDefinition := range configuration.Checks {
			select {
			case <-stopDispatch:
				close(jobsChannel)
				return
			case jobsChannel <- checkJob{index: checkIndex, check: checkDefinition}:
				jobScheduled[checkIndex] = true
			}
		}

		close(jobsChannel)
	}()

	var stopOnce sync.Once
	var firstFatalError error
	failFastTriggered := false

	stopAll := func() {
		stopOnce.Do(func() {
			close(stopDispatch)
			cancelRun()
		})
	}

	for result := range resultsChannel {
		if result.runErr != nil && firstFatalError == nil {
			firstFatalError = result.runErr
			stopAll()
		}
		if result.runErr != nil {
			continue
		}

		if result.outcome.Skipped {
			report.Skipped = append(report.Skipped, result.name)
			if strings.TrimSpace(result.outcome.Reason) != "" {
				report.SkipReasons[result.name] = result.outcome.Reason
			}
			continue
		}

		if result.outcome.Canceled {
			report.Canceled = append(report.Canceled, result.name)
			continue
		}

		if result.outcome.ExitCode != 0 || result.outcome.TimedOut {
			report.Failures = append(report.Failures, result.name)
			report.FailureTails[result.name] = result.outcome.Tail

			if strings.TrimSpace(result.outcome.LogPath) != "" {
				report.LogFiles[result.name] = result.outcome.LogPath
			}

			if result.outcome.TimedOut {
				report.FailureHeadlines[result.name] = fmt.Sprintf("Timed out after %s", result.outcome.Timeout)
			} else if headline := strings.TrimSpace(ExtractHeadline(result.name, result.outcome.Tail)); headline != "" {
				report.FailureHeadlines[result.name] = headline
			}

			if options.FailFast && !failFastTriggered {
				failFastTriggered = true
				stopAll()
			}
		}
	}

	<-dispatchDone

	if failFastTriggered {
		for checkIndex, checkDefinition := range configuration.Checks {
			if !jobScheduled[checkIndex] {
				report.Canceled = append(report.Canceled, checkDefinition.Name)
			}
		}
	}

	checkOrder := make(map[string]int, totalChecks)
	for checkIndex, checkDefinition := range configuration.Checks {
		checkOrder[checkDefinition.Name] = checkIndex
	}

	sort.SliceStable(report.Failures, func(leftIndex, rightIndex int) bool {
		return checkOrder[report.Failures[leftIndex]] < checkOrder[report.Failures[rightIndex]]
	})
	sort.SliceStable(report.Canceled, func(leftIndex, rightIndex int) bool {
		return checkOrder[report.Canceled[leftIndex]] < checkOrder[report.Canceled[rightIndex]]
	})
	sort.SliceStable(report.Skipped, func(leftIndex, rightIndex int) bool {
		return checkOrder[report.Skipped[leftIndex]] < checkOrder[report.Skipped[rightIndex]]
	})

	if firstFatalError != nil {
		return Report{}, firstFatalError
	}

	return report, nil
}

func runOne(
	parentContext context.Context,
	repoRoot string,
	workingDirectory string,
	checkIndex int,
	checkName string,
	commandText string,
	explicitShell string,
	environmentOverrides map[string]string,
	timeoutDuration time.Duration,
	options Options,
) (runOutcome, error) {
	if parentContext.Err() != nil {
		return runOutcome{ExitCode: 1, Canceled: true}, nil
	}

	tailBuffer := newLimitedBuffer(128 * 1024)

	logDirectory := strings.TrimSpace(options.LogDir)
	if logDirectory == "" {
		logDirectory = resolveDefaultLogDir(repoRoot)
	}
	if err := os.MkdirAll(logDirectory, 0o755); err != nil {
		return runOutcome{ExitCode: 1}, err
	}

	logFileName := fmt.Sprintf(
		"%s_%02d_%s.log",
		time.Now().Format("20060102_150405"),
		checkIndex,
		sanitize(checkName),
	)
	logPath := filepath.Join(logDirectory, logFileName)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return runOutcome{ExitCode: 1}, err
	}

	// Important on Windows: you cannot delete the file while it is still open.
	// We close explicitly on every return path.
	closeLogFile := func() {
		_ = logFile.Close()
	}

	outputWriter := io.MultiWriter(logFile, tailBuffer)
	if options.Verbose {
		outputWriter = io.MultiWriter(os.Stdout, logFile, tailBuffer)
	}

	runContext := parentContext
	var cancelRun context.CancelFunc
	if timeoutDuration > 0 {
		runContext, cancelRun = context.WithTimeout(parentContext, timeoutDuration)
	}
	if cancelRun != nil {
		defer cancelRun()
	}

	// Enterprise behavior: no implicit defaults. If shell is not configured, we use OS default.
	execName, execArgs := resolveCommand(explicitShell, commandText, "")

	command := exec.Command(execName, execArgs...) //nolint:gosec // command comes from user config by design
	command.Dir = workingDirectory
	command.Stdout = outputWriter
	command.Stderr = outputWriter

	command.Env = applyEnvOverrides(os.Environ(), environmentOverrides)
	command.Env = adjustEnvForShell(execName, command.Env)

	if err := command.Start(); err != nil {
		closeLogFile()
		_ = os.Remove(logPath) // best-effort cleanup for an empty log created before spawn
		return runOutcome{ExitCode: 1}, err
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()

	select {
	case <-runContext.Done():
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		<-waitDone

		closeLogFile()

		outcome := runOutcome{
			ExitCode: 1,
			Tail:     tailBuffer.String(),
			LogPath:  logPath,
		}

		if runContext.Err() == context.DeadlineExceeded {
			outcome.TimedOut = true
			outcome.Timeout = timeoutDuration
		} else {
			outcome.Canceled = true
		}

		return outcome, nil

	case waitErr := <-waitDone:
		if waitErr == nil {
			closeLogFile()

			if err := removeFileWithRetries(logPath); err != nil {
				// This should be rare, but if we promised success logs are removed,
				// failing to remove it is a real error.
				return runOutcome{ExitCode: 0, Tail: tailBuffer.String()}, err
			}

			return runOutcome{ExitCode: 0, Tail: tailBuffer.String()}, nil
		}

		closeLogFile()

		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return runOutcome{
				ExitCode: exitErr.ExitCode(),
				Tail:     tailBuffer.String(),
				LogPath:  logPath,
			}, nil
		}

		return runOutcome{
			ExitCode: 1,
			Tail:     tailBuffer.String(),
			LogPath:  logPath,
		}, waitErr
	}
}

func removeFileWithRetries(path string) error {
	// On Windows you can still hit short-lived locks (AV, indexing, etc).
	// A few tiny retries makes this deterministic for tests and less annoying in real life.
	const attempts = 8
	const delay = 15 * time.Millisecond

	var lastErr error
	for attempt := range attempts {
		lastErr = os.Remove(path)
		if lastErr == nil || os.IsNotExist(lastErr) {
			return nil
		}
		// Do not sleep after the last attempt.
		if attempt < attempts-1 {
			time.Sleep(delay)
		}
	}
	return lastErr
}

func applyEnvOverrides(baseEnv []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return baseEnv
	}

	mergedEnv := make([]string, len(baseEnv))
	copy(mergedEnv, baseEnv)

	envIndexByKey := make(map[string]int, len(mergedEnv))
	for entryIndex, entry := range mergedEnv {
		key, _, hasSeparator := strings.Cut(entry, "=")
		if !hasSeparator {
			continue
		}
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		// Last one wins for existing duplicates in the base env.
		envIndexByKey[trimmedKey] = entryIndex
	}

	for key, value := range overrides {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}

		formatted := fmt.Sprintf("%s=%s", trimmedKey, value)
		if existingIndex, found := envIndexByKey[trimmedKey]; found {
			mergedEnv[existingIndex] = formatted
			continue
		}

		envIndexByKey[trimmedKey] = len(mergedEnv)
		mergedEnv = append(mergedEnv, formatted)
	}

	return mergedEnv
}
