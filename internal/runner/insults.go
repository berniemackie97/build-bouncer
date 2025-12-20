package runner

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"build-bouncer/internal/config"
)

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
