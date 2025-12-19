package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

func resolveDefaultLogDir(repoRoot string) string {
	gitDir := filepath.Join(repoRoot, ".git")
	if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
		return filepath.Join(gitDir, "build-bouncer", "logs")
	}
	return filepath.Join(repoRoot, ".build-bouncer", "logs")
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "check"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

func TailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// --------------------
// Insults (unchanged)
// --------------------

type insultPack struct {
	Version         int              `json:"version"`
	MaxHistory      int              `json:"maxHistory"`
	DefaultCooldown int              `json:"defaultCooldown"`
	Templates       []insultTemplate `json:"templates"`
}

type insultTemplate struct {
	ID         string   `json:"id"`
	Categories []string `json:"categories"`
	Locales    []string `json:"locales"`
	Weight     int      `json:"weight"`
	Cooldown   int      `json:"cooldown"`
	Text       string   `json:"text"`
}

type insultState struct {
	Version int      `json:"version"`
	Recent  []string `json:"recent"`
}

func PickInsult(root string, ins config.Insults, rep Report) string {
	packPath := filepath.Join(root, filepath.FromSlash(ins.File))
	pack, err := loadInsultPack(packPath)
	if err != nil || len(pack.Templates) == 0 {
		return formatInsult(ins.Mode, "Push blocked. Failed: "+strings.Join(rep.Failures, ", "))
	}

	if pack.MaxHistory <= 0 {
		pack.MaxHistory = 40
	}
	if pack.DefaultCooldown < 0 {
		pack.DefaultCooldown = 0
	}

	category := categoryFromFailures(rep.Failures)

	locale := strings.TrimSpace(ins.Locale)
	if locale == "" {
		locale = "en"
	}

	statePath := resolveInsultStatePath(root)
	st := loadInsultState(statePath)

	check := pickFailingCheck(rep.Failures)
	detail := extractDetail(category, rep, check)
	if strings.TrimSpace(detail) == "" {
		detail = check
	}

	candidates := filterTemplates(pack, category, locale, st)
	if len(candidates) == 0 {
		candidates = filterTemplates(pack, category, locale, insultState{Version: st.Version})
	}
	if len(candidates) == 0 {
		candidates = filterTemplates(pack, "any", locale, insultState{Version: st.Version})
	}
	if len(candidates) == 0 {
		return formatInsult(ins.Mode, "Push blocked. Failed: "+strings.Join(rep.Failures, ", "))
	}

	chosen := weightedPick(candidates)

	msg := strings.TrimSpace(chosen.Text)
	msg = strings.ReplaceAll(msg, "{check}", check)
	msg = strings.ReplaceAll(msg, "{checks}", strings.Join(rep.Failures, ", "))
	msg = strings.ReplaceAll(msg, "{detail}", trimDetail(detail))

	if chosen.ID != "" {
		st.Recent = append([]string{chosen.ID}, st.Recent...)
		if len(st.Recent) > pack.MaxHistory {
			st.Recent = st.Recent[:pack.MaxHistory]
		}
		_ = saveInsultState(statePath, st)
	}

	return formatInsult(ins.Mode, msg)
}

func loadInsultPack(path string) (insultPack, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return insultPack{}, err
	}
	var p insultPack
	if err := json.Unmarshal(b, &p); err != nil {
		return insultPack{}, err
	}
	if p.Version <= 0 {
		p.Version = 1
	}
	return p, nil
}

func resolveInsultStatePath(root string) string {
	gitDir := filepath.Join(root, ".git")
	if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
		return filepath.Join(gitDir, "build-bouncer", "insults_state.json")
	}
	return filepath.Join(root, ".buildbouncer_insults_state.json")
}

func loadInsultState(path string) insultState {
	b, err := os.ReadFile(path)
	if err != nil {
		return insultState{Version: 1, Recent: nil}
	}
	var st insultState
	if err := json.Unmarshal(b, &st); err != nil {
		return insultState{Version: 1, Recent: nil}
	}
	if st.Version <= 0 {
		st.Version = 1
	}
	return st
}

func saveInsultState(path string, st insultState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func filterTemplates(pack insultPack, category string, locale string, st insultState) []insultTemplate {
	var out []insultTemplate
	for _, t := range pack.Templates {
		t = normalizeTemplate(pack, t)
		if strings.TrimSpace(t.Text) == "" {
			continue
		}
		if !matchesLocaleInsult(t, locale) {
			continue
		}
		if !matchesCategoryInsult(t, category) {
			continue
		}
		if inCooldownInsult(pack, t, st) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func normalizeTemplate(pack insultPack, t insultTemplate) insultTemplate {
	if t.Weight <= 0 {
		t.Weight = 1
	}
	if t.Cooldown == 0 {
		t.Cooldown = pack.DefaultCooldown
	}
	if t.Cooldown < 0 {
		t.Cooldown = 0
	}
	if len(t.Categories) == 0 {
		t.Categories = []string{"any"}
	}
	return t
}

func matchesCategoryInsult(t insultTemplate, category string) bool {
	want := strings.ToLower(strings.TrimSpace(category))
	for _, c := range t.Categories {
		cc := strings.ToLower(strings.TrimSpace(c))
		if cc == "any" || cc == "*" {
			return true
		}
		if cc == want {
			return true
		}
	}
	return false
}

func matchesLocaleInsult(t insultTemplate, locale string) bool {
	if len(t.Locales) == 0 {
		return true
	}
	for _, l := range t.Locales {
		if strings.EqualFold(strings.TrimSpace(l), locale) {
			return true
		}
	}
	return false
}

func inCooldownInsult(pack insultPack, t insultTemplate, st insultState) bool {
	if t.ID == "" || t.Cooldown <= 0 {
		return false
	}
	limit := t.Cooldown
	if limit > len(st.Recent) {
		limit = len(st.Recent)
	}
	for i := 0; i < limit; i++ {
		if st.Recent[i] == t.ID {
			return true
		}
	}
	return false
}

func weightedPick(candidates []insultTemplate) insultTemplate {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	total := 0
	for _, c := range candidates {
		total += c.Weight
	}
	if total <= 0 {
		return candidates[r.Intn(len(candidates))]
	}
	n := r.Intn(total)
	acc := 0
	for _, c := range candidates {
		acc += c.Weight
		if n < acc {
			return c
		}
	}
	return candidates[len(candidates)-1]
}

var (
	reGoTestFail = regexp.MustCompile(`(?m)^--- FAIL: ([^\s]+)`)
	reDotnetFail = regexp.MustCompile(`(?m)^\s*Failed\s+([^\s]+)`)
	rePytestFail = regexp.MustCompile(`(?m)^FAILED\s+([^\s]+)`)
	reJestFail   = regexp.MustCompile(`(?m)^FAIL\s+(.+)$`)
	reFirstError = regexp.MustCompile(`(?mi)^\s*(?:error|fatal|panic):\s*(.+)$`)
)

func categoryFromFailures(failures []string) string {
	for _, f := range failures {
		s := strings.ToLower(f)
		if strings.Contains(s, "test") || strings.Contains(s, "spec") {
			return "tests"
		}
	}
	for _, f := range failures {
		s := strings.ToLower(f)
		if strings.Contains(s, "build") || strings.Contains(s, "compile") {
			return "build"
		}
	}
	for _, f := range failures {
		s := strings.ToLower(f)
		if strings.Contains(s, "lint") || strings.Contains(s, "vet") || strings.Contains(s, "format") || strings.Contains(s, "fmt") {
			return "lint"
		}
	}
	for _, f := range failures {
		s := strings.ToLower(f)
		if strings.Contains(s, "ci") || strings.Contains(s, "workflow") || strings.Contains(s, "actions") {
			return "ci"
		}
	}
	return "any"
}

func pickFailingCheck(failures []string) string {
	if len(failures) == 0 {
		return "something"
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return failures[r.Intn(len(failures))]
}

func extractDetail(category string, rep Report, preferredCheck string) string {
	if out := rep.FailureTails[preferredCheck]; strings.TrimSpace(out) != "" {
		if d := extractDetailFromOutput(category, out); d != "" {
			return d
		}
	}
	for _, out := range rep.FailureTails {
		if d := extractDetailFromOutput(category, out); d != "" {
			return d
		}
	}
	return ""
}

func extractDetailFromOutput(category string, out string) string {
	out = strings.ReplaceAll(out, "\r\n", "\n")

	if category == "tests" {
		if m := reGoTestFail.FindStringSubmatch(out); len(m) == 2 {
			return m[1]
		}
		if m := reDotnetFail.FindStringSubmatch(out); len(m) == 2 {
			return m[1]
		}
		if m := rePytestFail.FindStringSubmatch(out); len(m) == 2 {
			return m[1]
		}
		if m := reJestFail.FindStringSubmatch(out); len(m) == 2 {
			return strings.TrimSpace(m[1])
		}
	}

	if m := reFirstError.FindStringSubmatch(out); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func trimDetail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	const max = 80
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
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
