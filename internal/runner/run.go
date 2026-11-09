// Package runner executes configured checks and produces a report of failures, skips, and logs.
// It is the core runtime that powers build-bouncer.
package runner

import (
	"context"
	"errors"
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

const (
	defaultCheckTimeout = 30 * time.Minute
	processWaitDelay    = 2 * time.Second
)

type ProgressEvent struct {
	Stage    string // start | end
	Index    int
	Total    int
	Check    string
	ExitCode int
}

type Options struct {
	Context          context.Context
	CI               bool
	Verbose          bool
	LogDir           string
	MaxLogBytes      int64
	MaxSavedLogs     int
	LogRetentionDays int
	MaxParallel      int
	FailFast         bool
	Progress         func(e ProgressEvent)
	Output           io.Writer
}

type Report struct {
	Failures         []string
	AllowedFailures  map[string]bool
	FailureTails     map[string]string // checkName -> output tail
	FailureHeadlines map[string]string // checkName -> headline
	Unavailable      map[string]string // checkName -> missing required tools
	Canceled         []string
	TimedOut         []string
	Skipped          []string
	SkipReasons      map[string]string // checkName -> reason
	LogFiles         map[string]string // checkName -> retained log for failures and cancellations
	LogTruncated     map[string]bool
}

type serializedWriter struct {
	mu     sync.Mutex
	writer io.Writer
	cancel context.CancelFunc
	err    error
}

func (w *serializedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
		if w.cancel != nil {
			w.cancel()
		}
	}
	return n, err
}

func (w *serializedWriter) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

type limitedBuffer struct {
	maxBytes int
	buffer   []byte
}

type runOutcome struct {
	ExitCode  int
	Tail      string
	LogPath   string
	Truncated bool
	TimedOut  bool
	Timeout   time.Duration
	Canceled  bool
	Skipped   bool
	Reason    string
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

var (
	ErrNoChecksExecuted = errors.New("no checks executed")
	ErrCheckExecution   = errors.New("check execution failed")
	ErrOutputWrite      = errors.New("runner output write failed")
)

func RunAllReport(repoRoot string, configuration *config.Config, options Options) (Report, error) {
	report := Report{
		Failures:         []string{},
		AllowedFailures:  map[string]bool{},
		FailureTails:     map[string]string{},
		FailureHeadlines: map[string]string{},
		Unavailable:      map[string]string{},
		LogFiles:         map[string]string{},
		LogTruncated:     map[string]bool{},
		SkipReasons:      map[string]string{},
	}

	totalChecks := len(configuration.Checks)
	if totalChecks == 0 {
		return report, fmt.Errorf("no checks configured")
	}

	maxParallel := options.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 1
	}
	if maxParallel > config.MaxParallelChecks {
		return report, fmt.Errorf("max parallel checks %d exceeds limit %d", maxParallel, config.MaxParallelChecks)
	}
	if maxParallel > totalChecks {
		maxParallel = totalChecks
	}

	limits, err := resolveLogLimits(options)
	if err != nil {
		return report, err
	}
	options.MaxLogBytes = limits.maxBytes
	options.MaxSavedLogs = limits.keep
	options.LogRetentionDays = limits.days
	logDirectory := options.LogDir
	if strings.TrimSpace(logDirectory) == "" {
		logDirectory = resolveDefaultLogDir(repoRoot)
	}
	if err := prepareLogDir(logDirectory, strings.TrimSpace(options.LogDir) == ""); err != nil {
		return report, err
	}
	if err := pruneCheckLogs(logDirectory, limits, time.Now()); err != nil {
		return report, err
	}

	parentContext := options.Context
	if parentContext == nil {
		parentContext = context.Background()
	}
	if err := parentContext.Err(); err != nil {
		return report, err
	}
	runContext, cancelRun := context.WithCancel(parentContext)
	defer cancelRun()
	writer := options.Output
	if writer == nil {
		writer = os.Stdout
	}
	output := &serializedWriter{writer: writer, cancel: cancelRun}
	options.Output = output

	jobsChannel := make(chan checkJob)
	resultsChannel := make(chan checkResult)

	stopDispatch := make(chan struct{})
	dispatchDone := make(chan struct{})

	var workerGroup sync.WaitGroup
	var progressMutex sync.Mutex
	emitProgress := func(event ProgressEvent) {
		if options.Progress == nil {
			return
		}
		progressMutex.Lock()
		defer progressMutex.Unlock()
		options.Progress(event)
	}
	var stopOnce sync.Once
	stopAll := func() {
		stopOnce.Do(func() {
			close(stopDispatch)
			cancelRun()
		})
	}

	workerFn := func() {
		defer workerGroup.Done()

		for job := range jobsChannel {
			checkDefinition := job.check
			checkName := checkDefinition.Name
			if runContext.Err() != nil {
				resultsChannel <- checkResult{index: job.index, name: checkName, outcome: runOutcome{Canceled: true}}
				continue
			}

			emitProgress(ProgressEvent{
				Stage: "start",
				Index: job.index + 1,
				Total: totalChecks,
				Check: checkName,
			})

			if reason, unavailable := checkReadiness(repoRoot, checkDefinition); reason != "" {
				outcome := runOutcome{Skipped: !unavailable, Reason: reason}
				if unavailable {
					outcome.ExitCode = 1
				}
				emitProgress(ProgressEvent{
					Stage:    "end",
					Index:    job.index + 1,
					Total:    totalChecks,
					Check:    checkName,
					ExitCode: outcome.ExitCode,
				})
				if options.Verbose {
					if unavailable {
						fmt.Fprintf(options.Output, "!! %s unavailable (%s)\n\n", checkName, reason)
					} else {
						fmt.Fprintf(options.Output, "~~ %s skipped (%s)\n\n", checkName, reason)
					}
				}
				if unavailable && options.FailFast {
					stopAll()
				}
				resultsChannel <- checkResult{index: job.index, name: checkName, outcome: outcome}
				continue
			}

			if options.Verbose {
				fmt.Fprintf(options.Output, "==> %s\n", checkName)
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
			if runErr != nil || (options.FailFast && (outcome.ExitCode != 0 || outcome.TimedOut)) {
				stopAll()
			}

			emitProgress(ProgressEvent{
				Stage:    "end",
				Index:    job.index + 1,
				Total:    totalChecks,
				Check:    checkName,
				ExitCode: outcome.ExitCode,
			})

			if options.Verbose {
				switch {
				case runErr != nil:
					fmt.Fprintf(options.Output, "!! %s error: %v\n\n", checkName, runErr)
				case outcome.Canceled:
					fmt.Fprintf(options.Output, "!! %s canceled\n\n", checkName)
				case outcome.TimedOut:
					fmt.Fprintf(options.Output, "!! %s timed out\n\n", checkName)
				case outcome.ExitCode != 0:
					fmt.Fprintf(options.Output, "!! %s failed (exit %d)\n\n", checkName, outcome.ExitCode)
				default:
					fmt.Fprintf(options.Output, "OK %s\n\n", checkName)
				}
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
			case <-runContext.Done():
				close(jobsChannel)
				return
			case jobsChannel <- checkJob{index: checkIndex, check: checkDefinition}:
				jobScheduled[checkIndex] = true
			}
		}

		close(jobsChannel)
	}()

	var firstFatalError error
	failFastTriggered := false

	for result := range resultsChannel {
		if result.runErr != nil {
			if firstFatalError == nil {
				firstFatalError = fmt.Errorf("check %q: %w", result.name, result.runErr)
				stopAll()
			}
			if result.outcome.ExitCode == 0 {
				result.outcome.ExitCode = 1
			}
			if result.outcome.Reason == "" {
				result.outcome.Reason = "execution error: " + result.runErr.Error()
			}
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
			report.FailureTails[result.name] = result.outcome.Tail
			if result.outcome.LogPath != "" {
				report.LogFiles[result.name] = result.outcome.LogPath
			}
			if result.outcome.Truncated {
				report.LogTruncated[result.name] = true
			}
			continue
		}

		if result.outcome.ExitCode != 0 || result.outcome.TimedOut {
			report.Failures = append(report.Failures, result.name)
			if result.outcome.TimedOut {
				report.TimedOut = append(report.TimedOut, result.name)
			}
			if configuration.Checks[result.index].AllowFailure &&
				!result.outcome.TimedOut && result.outcome.Reason == "" {
				report.AllowedFailures[result.name] = true
			}
			report.FailureTails[result.name] = result.outcome.Tail

			if strings.TrimSpace(result.outcome.LogPath) != "" {
				report.LogFiles[result.name] = result.outcome.LogPath
			}
			if result.outcome.Truncated {
				report.LogTruncated[result.name] = true
			}

			if result.outcome.Reason != "" {
				report.Unavailable[result.name] = result.outcome.Reason
				report.FailureHeadlines[result.name] = result.outcome.Reason
			} else if result.outcome.TimedOut {
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

	if runContext.Err() != nil || failFastTriggered {
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
	sort.SliceStable(report.TimedOut, func(leftIndex, rightIndex int) bool {
		return checkOrder[report.TimedOut[leftIndex]] < checkOrder[report.TimedOut[rightIndex]]
	})
	sort.SliceStable(report.Skipped, func(leftIndex, rightIndex int) bool {
		return checkOrder[report.Skipped[leftIndex]] < checkOrder[report.Skipped[rightIndex]]
	})

	if err := pruneCheckLogs(logDirectory, limits, time.Now()); err != nil {
		return report, err
	}
	for name, path := range report.LogFiles {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			delete(report.LogFiles, name)
		} else if err != nil {
			return report, err
		}
	}

	if err := output.Err(); err != nil {
		return report, fmt.Errorf("%w: %w", ErrOutputWrite, err)
	}
	if firstFatalError != nil {
		return report, fmt.Errorf("%w: %w", ErrCheckExecution, firstFatalError)
	}
	if err := parentContext.Err(); err != nil {
		return report, err
	}
	if len(report.Skipped) == totalChecks {
		return report, fmt.Errorf("%w: all %d configured checks were skipped", ErrNoChecksExecuted, totalChecks)
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
	if timeoutDuration <= 0 {
		timeoutDuration = defaultCheckTimeout
	}
	if timeoutDuration > config.MaxCheckTimeout {
		return runOutcome{ExitCode: 1}, fmt.Errorf("check %q timeout %s exceeds limit %s", checkName, timeoutDuration, config.MaxCheckTimeout)
	}
	workingInfo, err := os.Stat(workingDirectory)
	if err != nil {
		return runOutcome{ExitCode: 1}, fmt.Errorf("working directory %q: %w", workingDirectory, err)
	}
	if !workingInfo.IsDir() {
		return runOutcome{ExitCode: 1}, fmt.Errorf("working directory %q is not a directory", workingDirectory)
	}

	limits, err := resolveLogLimits(options)
	if err != nil {
		return runOutcome{ExitCode: 1}, err
	}
	tailBuffer := newLimitedBuffer(128 * 1024)

	logDirectory := strings.TrimSpace(options.LogDir)
	if logDirectory == "" {
		logDirectory = resolveDefaultLogDir(repoRoot)
	}
	if err := prepareLogDir(logDirectory, strings.TrimSpace(options.LogDir) == ""); err != nil {
		return runOutcome{ExitCode: 1}, err
	}

	logFile, err := os.CreateTemp(logDirectory, fmt.Sprintf(
		"build-bouncer-%s-%02d-%s-*.pending",
		time.Now().Format("20060102_150405"),
		checkIndex,
		sanitize(checkName),
	))
	if err != nil {
		return runOutcome{ExitCode: 1}, err
	}
	logPath := logFile.Name()
	diskWriter := &boundedLogWriter{writer: logFile, limit: limits.maxBytes}
	outputWriter := io.MultiWriter(tailBuffer, diskWriter)
	if options.Verbose {
		writer := options.Output
		if writer == nil {
			writer = os.Stdout
		}
		outputWriter = io.MultiWriter(tailBuffer, diskWriter, writer)
	}
	finishLog := func() error {
		closeErr := logFile.Close()
		if diskWriter.err != nil {
			return diskWriter.err
		}
		return closeErr
	}
	finishFailureLog := func() error {
		if err := finishLog(); err != nil {
			return fmt.Errorf("write check log: %w", err)
		}
		completedPath := strings.TrimSuffix(logPath, ".pending") + ".log"
		if err := os.Rename(logPath, completedPath); err != nil {
			return fmt.Errorf("retain failed check log: %w", err)
		}
		logPath = completedPath
		return nil
	}

	runContext, cancelRun := context.WithTimeout(parentContext, timeoutDuration)
	defer cancelRun()

	execName, execArgs, err := resolveCommand(explicitShell, commandText, "")
	if err == nil {
		execName = resolveExecutableAt(repoRoot, execName)
		err = validateWSLEnvironment(execName, environmentOverrides)
	}
	if err != nil {
		_ = finishLog()
		_ = os.Remove(logPath)
		return runOutcome{ExitCode: 1}, fmt.Errorf("resolve check shell: %w", err)
	}

	command := exec.Command(execName, execArgs...) //nolint:gosec // command comes from user config by design
	command.WaitDelay = processWaitDelay
	command.Dir = workingDirectory
	command.Stdout = outputWriter
	command.Stderr = outputWriter

	command.Env = applyEnvOverrides(os.Environ(), environmentOverrides)
	command.Env = adjustEnvForShell(execName, command.Env)

	process, err := startCheckProcess(command)
	if err != nil {
		_ = finishLog()
		_ = os.Remove(logPath)
		return runOutcome{ExitCode: 1}, err
	}
	defer process.close()

	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()

	select {
	case <-runContext.Done():
		process.terminate()
		<-waitDone

		logErr := finishFailureLog()
		outcome := runOutcome{
			ExitCode:  1,
			Tail:      tailBuffer.String(),
			LogPath:   logPath,
			Truncated: diskWriter.truncated,
		}
		if logErr != nil {
			return outcome, logErr
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
			if logErr := finishLog(); logErr != nil {
				return runOutcome{ExitCode: 1, Tail: tailBuffer.String(), LogPath: logPath, Truncated: diskWriter.truncated}, fmt.Errorf("write check log: %w", logErr)
			}
			if err := removeFileWithRetries(logPath); err != nil {
				return runOutcome{ExitCode: 1, Tail: tailBuffer.String(), LogPath: logPath}, fmt.Errorf("remove successful check log: %w", err)
			}
			return runOutcome{ExitCode: 0, Tail: tailBuffer.String()}, nil
		}
		if logErr := finishFailureLog(); logErr != nil {
			return runOutcome{ExitCode: 1, Tail: tailBuffer.String(), LogPath: logPath, Truncated: diskWriter.truncated}, logErr
		}

		if errors.Is(waitErr, exec.ErrWaitDelay) {
			return runOutcome{
				ExitCode:  1,
				Tail:      tailBuffer.String(),
				LogPath:   logPath,
				Truncated: diskWriter.truncated,
				Reason:    "output pipes stayed open after the command exited",
			}, nil
		}

		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return runOutcome{
				ExitCode:  exitErr.ExitCode(),
				Tail:      tailBuffer.String(),
				LogPath:   logPath,
				Truncated: diskWriter.truncated,
			}, nil
		}

		return runOutcome{
			ExitCode:  1,
			Tail:      tailBuffer.String(),
			LogPath:   logPath,
			Truncated: diskWriter.truncated,
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
