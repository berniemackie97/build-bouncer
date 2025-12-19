package banter

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	File   string
	Locale string
}

type Pack struct {
	Version         int     `json:"version"`
	MaxHistory      int     `json:"maxHistory"`
	DefaultCooldown int     `json:"defaultCooldown"`
	Entries         []Entry `json:"entries"`
}

type Entry struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // intro | loading | success
	Locales  []string `json:"locales"`
	Weight   int      `json:"weight"`
	Cooldown int      `json:"cooldown"`
	Text     string   `json:"text"`
}

type State struct {
	Version int                 `json:"version"`
	Recent  map[string][]string `json:"recent"` // type -> most-recent-first IDs
}

type Picker struct {
	root      string
	cfg       Config
	pack      Pack
	state     State
	statePath string
}

func Load(root string, cfg Config) (*Picker, error) {
	p := &Picker{
		root: root,
		cfg:  cfg,
	}

	packPath := filepath.Join(root, filepath.FromSlash(cfg.File))
	b, err := os.ReadFile(packPath)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(b, &p.pack); err != nil {
		return nil, err
	}

	if p.pack.MaxHistory <= 0 {
		p.pack.MaxHistory = 50
	}
	if p.pack.DefaultCooldown < 0 {
		p.pack.DefaultCooldown = 0
	}
	if strings.TrimSpace(p.cfg.Locale) == "" {
		p.cfg.Locale = "en"
	}

	p.statePath = resolveStatePath(root)
	p.state = loadState(p.statePath)
	if p.state.Recent == nil {
		p.state.Recent = map[string][]string{}
	}

	return p, nil
}

func (p *Picker) Pick(entryType string) string {
	entryType = strings.ToLower(strings.TrimSpace(entryType))
	if entryType == "" {
		return ""
	}

	candidates := p.filter(entryType, true)
	if len(candidates) == 0 {
		// relax cooldown
		candidates = p.filter(entryType, false)
	}
	if len(candidates) == 0 {
		return ""
	}

	chosen := weightedPick(candidates)
	text := strings.TrimSpace(chosen.Text)
	if text == "" {
		return ""
	}

	if chosen.ID != "" {
		p.state.Recent[entryType] = append([]string{chosen.ID}, p.state.Recent[entryType]...)
		if len(p.state.Recent[entryType]) > p.pack.MaxHistory {
			p.state.Recent[entryType] = p.state.Recent[entryType][:p.pack.MaxHistory]
		}
		_ = saveState(p.statePath, p.state)
	}

	return text
}

func (p *Picker) filter(entryType string, enforceCooldown bool) []Entry {
	var out []Entry
	for _, e := range p.pack.Entries {
		e = normalizeEntry(p.pack, e)

		if strings.TrimSpace(e.Text) == "" {
			continue
		}
		if strings.ToLower(strings.TrimSpace(e.Type)) != entryType {
			continue
		}
		if !matchesLocale(e, p.cfg.Locale) {
			continue
		}
		if enforceCooldown && inCooldown(p.state, entryType, e.ID, e.Cooldown) {
			continue
		}

		out = append(out, e)
	}
	return out
}

func normalizeEntry(pack Pack, e Entry) Entry {
	if e.Weight <= 0 {
		e.Weight = 1
	}
	if e.Cooldown == 0 {
		e.Cooldown = pack.DefaultCooldown
	}
	if e.Cooldown < 0 {
		e.Cooldown = 0
	}
	return e
}

func matchesLocale(e Entry, locale string) bool {
	if len(e.Locales) == 0 {
		return true
	}
	for _, l := range e.Locales {
		if strings.EqualFold(strings.TrimSpace(l), locale) {
			return true
		}
	}
	return false
}

func inCooldown(st State, entryType string, id string, cooldown int) bool {
	if id == "" || cooldown <= 0 {
		return false
	}

	recent := st.Recent[entryType]
	limit := cooldown
	if limit > len(recent) {
		limit = len(recent)
	}

	for i := 0; i < limit; i++ {
		if recent[i] == id {
			return true
		}
	}
	return false
}

func weightedPick(entries []Entry) Entry {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	total := 0
	for _, e := range entries {
		total += e.Weight
	}
	if total <= 0 {
		return entries[r.Intn(len(entries))]
	}

	n := r.Intn(total)
	acc := 0
	for _, e := range entries {
		acc += e.Weight
		if n < acc {
			return e
		}
	}
	return entries[len(entries)-1]
}

func resolveStatePath(root string) string {
	gitDir := filepath.Join(root, ".git")
	if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
		return filepath.Join(gitDir, "build-bouncer", "banter_state.json")
	}
	return filepath.Join(root, ".buildbouncer_banter_state.json")
}

func loadState(path string) State {
	b, err := os.ReadFile(path)
	if err != nil {
		return State{Version: 1, Recent: map[string][]string{}}
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{Version: 1, Recent: map[string][]string{}}
	}
	if st.Version <= 0 {
		st.Version = 1
	}
	if st.Recent == nil {
		st.Recent = map[string][]string{}
	}
	return st
}

func saveState(path string, st State) error {
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
