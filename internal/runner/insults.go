package runner

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/config"
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

// One RNG, seeded once. We lock because rand.Rand is not concurrency safe.
var (
	randomMutex     sync.Mutex
	randomGenerator = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func nextRandomIntn(upperBoundExclusive int) int {
	if upperBoundExclusive <= 0 {
		return 0
	}
	randomMutex.Lock()
	randomValue := randomGenerator.Intn(upperBoundExclusive)
	randomMutex.Unlock()
	return randomValue
}

// PickInsult loads the pack, filters templates based on failures, then picks a weighted insult.
// If anything about the pack is busted, we fall back to a plain message.
// The goal here is never crash the run even if the insults are messed up.
func PickInsult(root string, insults config.Insults, report Report) string {
	insultPackPath := filepath.Join(root, filepath.FromSlash(insults.File))
	packData, packErr := loadInsultPack(insultPackPath)
	if packErr != nil || len(packData.Templates) == 0 {
		return formatInsult(insults.Mode, fallbackFailureSummary(report))
	}

	if packData.MaxHistory <= 0 {
		packData.MaxHistory = 40
	}
	if packData.DefaultCooldown < 0 {
		packData.DefaultCooldown = 0
	}

	category := categoryFromFailures(report.Failures)

	locale := strings.TrimSpace(insults.Locale)
	if locale == "" {
		locale = "en"
	}
	localeLanguage := baseLocale(locale)

	statePath := resolveInsultStatePath(root)
	state := loadInsultState(statePath)

	failingCheck := pickFailingCheck(report.Failures)

	// detail is best effort. If we can find a file:line, use it.
	// If we cannot, just use the check name to avoid empty placeholder replacements.
	failureDetail := extractDetail(category, report, failingCheck)
	if strings.TrimSpace(failureDetail) == "" {
		failureDetail = failingCheck
	}
	trimmedFailureDetail := trimDetail(failureDetail)

	// 1) Normal filtering with cooldown
	candidateTemplates := filterTemplatesWithLocaleFallback(packData, category, locale, localeLanguage, state)

	// 2) If we filtered ourselves into a corner due to cooldown history, relax it.
	if len(candidateTemplates) == 0 {
		candidateTemplates = filterTemplatesWithLocaleFallback(packData, category, locale, localeLanguage, insultState{Version: state.Version})
	}
	if len(candidateTemplates) == 0 {
		candidateTemplates = filterTemplatesWithLocaleFallback(packData, "any", locale, localeLanguage, insultState{Version: state.Version})
	}
	if len(candidateTemplates) == 0 {
		return formatInsult(insults.Mode, fallbackFailureSummary(report))
	}

	chosenTemplate := weightedPick(candidateTemplates)

	// Fill placeholders and make sure the message includes at least some context.
	message := strings.TrimSpace(chosenTemplate.Text)
	message = strings.ReplaceAll(message, "{check}", failingCheck)
	message = strings.ReplaceAll(message, "{checks}", strings.Join(report.Failures, ", "))
	message = strings.ReplaceAll(message, "{detail}", trimmedFailureDetail)
	message = ensureInsultContext(message, failingCheck, trimmedFailureDetail)

	// Record the template ID so we can enforce cooldown history next run.
	if chosenTemplate.ID != "" {
		state.Recent = append([]string{chosenTemplate.ID}, state.Recent...)
		if len(state.Recent) > packData.MaxHistory {
			state.Recent = state.Recent[:packData.MaxHistory]
		}
		_ = saveInsultState(statePath, state)
	}

	return formatInsult(insults.Mode, message)
}

func fallbackFailureSummary(report Report) string {
	joinedFailures := strings.TrimSpace(strings.Join(report.Failures, ", "))
	if joinedFailures == "" {
		return "Push blocked. Failed checks."
	}
	return "Push blocked. Failed: " + joinedFailures
}

func loadInsultPack(path string) (insultPack, error) {
	fileBytes, readErr := os.ReadFile(path)
	if readErr != nil {
		return insultPack{}, readErr
	}

	var packData insultPack
	if unmarshalErr := json.Unmarshal(fileBytes, &packData); unmarshalErr != nil {
		return insultPack{}, unmarshalErr
	}

	if packData.Version <= 0 {
		packData.Version = 1
	}
	return packData, nil
}

func resolveInsultStatePath(root string) string {
	if gitDir, ok := resolveGitDir(root); ok {
		return filepath.Join(gitDir, "build-bouncer", "insults_state.json")
	}
	return filepath.Join(root, config.ConfigDirName, "state", "insults_state.json")
}

func loadInsultState(path string) insultState {
	fileBytes, readErr := os.ReadFile(path)
	if readErr != nil {
		return insultState{Version: 1, Recent: nil}
	}

	var state insultState
	if unmarshalErr := json.Unmarshal(fileBytes, &state); unmarshalErr != nil {
		return insultState{Version: 1, Recent: nil}
	}

	if state.Version <= 0 {
		state.Version = 1
	}
	return state
}

// saveInsultState writes a temp file then swaps it in.
// We try to be safe on Windows too because rename does not overwrite existing files.
func saveInsultState(path string, state insultState) error {
	targetDir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(targetDir, 0o755); mkdirErr != nil {
		return mkdirErr
	}

	jsonBytes, marshalErr := json.MarshalIndent(state, "", "  ")
	if marshalErr != nil {
		return marshalErr
	}

	tempPath := path + ".tmp"
	backupPath := path + ".bak"

	_ = os.Remove(tempPath)
	if writeErr := os.WriteFile(tempPath, jsonBytes, 0o644); writeErr != nil {
		return writeErr
	}

	// If there's an existing file, move it out of the way first.
	// On Windows, rename over existing is not allowed.
	_ = os.Remove(backupPath)
	if renameOldErr := os.Rename(path, backupPath); renameOldErr != nil && !os.IsNotExist(renameOldErr) {
		_ = os.Remove(tempPath)
		return renameOldErr
	}

	if promoteErr := os.Rename(tempPath, path); promoteErr != nil {
		// Best effort restore.
		_ = os.Remove(tempPath)
		_ = os.Rename(backupPath, path)
		return promoteErr
	}

	_ = os.Remove(backupPath)
	return nil
}

func baseLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return ""
	}
	separatorIndex := strings.IndexAny(locale, "-_")
	if separatorIndex > 0 {
		return locale[:separatorIndex]
	}
	return locale
}

func filterTemplatesWithLocaleFallback(
	packData insultPack,
	category string,
	locale string,
	localeLanguage string,
	state insultState,
) []insultTemplate {
	candidatesExact := filterTemplates(packData, category, locale, state)
	if len(candidatesExact) > 0 {
		return candidatesExact
	}

	if localeLanguage != "" && !strings.EqualFold(localeLanguage, locale) {
		candidatesBase := filterTemplates(packData, category, localeLanguage, state)
		if len(candidatesBase) > 0 {
			return candidatesBase
		}
	}

	return nil
}

func filterTemplates(packData insultPack, category string, locale string, state insultState) []insultTemplate {
	var filteredTemplates []insultTemplate
	for _, template := range packData.Templates {
		normalizedTemplate := normalizeTemplate(packData, template)
		if strings.TrimSpace(normalizedTemplate.Text) == "" {
			continue
		}
		if !matchesLocaleInsult(normalizedTemplate, locale) {
			continue
		}
		if !matchesCategoryInsult(normalizedTemplate, category) {
			continue
		}
		if inCooldownInsult(packData, normalizedTemplate, state) {
			continue
		}
		filteredTemplates = append(filteredTemplates, normalizedTemplate)
	}
	return filteredTemplates
}

func normalizeTemplate(packData insultPack, template insultTemplate) insultTemplate {
	if template.Weight <= 0 {
		template.Weight = 1
	}
	if template.Cooldown == 0 {
		template.Cooldown = packData.DefaultCooldown
	}
	if template.Cooldown < 0 {
		template.Cooldown = 0
	}
	if len(template.Categories) == 0 {
		template.Categories = []string{"any"}
	}
	return template
}

func matchesCategoryInsult(template insultTemplate, category string) bool {
	desired := strings.ToLower(strings.TrimSpace(category))
	for _, rawCategory := range template.Categories {
		templateCategory := strings.ToLower(strings.TrimSpace(rawCategory))
		if templateCategory == "any" || templateCategory == "*" {
			return true
		}
		if templateCategory == desired {
			return true
		}
	}
	return false
}

func matchesLocaleInsult(template insultTemplate, locale string) bool {
	if len(template.Locales) == 0 {
		return true
	}
	for _, rawLocale := range template.Locales {
		if strings.EqualFold(strings.TrimSpace(rawLocale), locale) {
			return true
		}
	}
	return false
}

func inCooldownInsult(packData insultPack, template insultTemplate, state insultState) bool {
	if template.ID == "" || template.Cooldown <= 0 {
		return false
	}

	cooldownLimit := min(template.Cooldown, len(state.Recent))

	for recentIndex := range cooldownLimit {
		if state.Recent[recentIndex] == template.ID {
			return true
		}
	}
	return false
}

func weightedPick(candidateTemplates []insultTemplate) insultTemplate {
	// Assumes candidateTemplates is non empty.
	totalWeight := 0
	for _, candidate := range candidateTemplates {
		totalWeight += candidate.Weight
	}

	// Defensive fallback. With normalization, totalWeight should always be > 0.
	if totalWeight <= 0 {
		return candidateTemplates[nextRandomIntn(len(candidateTemplates))]
	}

	chosenWeightIndex := nextRandomIntn(totalWeight)
	weightAccumulator := 0

	for _, candidate := range candidateTemplates {
		weightAccumulator += candidate.Weight
		if chosenWeightIndex < weightAccumulator {
			return candidate
		}
	}

	return candidateTemplates[len(candidateTemplates)-1]
}

func categoryFromFailures(failures []string) string {
	for _, failureName := range failures {
		lowerFailure := strings.ToLower(failureName)
		if strings.Contains(lowerFailure, "test") || strings.Contains(lowerFailure, "spec") {
			return "tests"
		}
	}
	for _, failureName := range failures {
		lowerFailure := strings.ToLower(failureName)
		if strings.Contains(lowerFailure, "build") || strings.Contains(lowerFailure, "compile") {
			return "build"
		}
	}
	for _, failureName := range failures {
		lowerFailure := strings.ToLower(failureName)
		if strings.Contains(lowerFailure, "lint") ||
			strings.Contains(lowerFailure, "vet") ||
			strings.Contains(lowerFailure, "format") ||
			strings.Contains(lowerFailure, "fmt") {
			return "lint"
		}
	}
	for _, failureName := range failures {
		lowerFailure := strings.ToLower(failureName)
		if strings.Contains(lowerFailure, "ci") ||
			strings.Contains(lowerFailure, "workflow") ||
			strings.Contains(lowerFailure, "actions") {
			return "ci"
		}
	}
	return "any"
}

func pickFailingCheck(failures []string) string {
	if len(failures) == 0 {
		return "something"
	}
	return failures[nextRandomIntn(len(failures))]
}

// extractDetail tries to pull one useful human hint from failure output.
// Think file:line, failing test name, etc. It is intentionally best effort.
func extractDetail(category string, report Report, preferredCheck string) string {
	if outputText := report.FailureTails[preferredCheck]; strings.TrimSpace(outputText) != "" {
		if extracted := extractDetailFromOutput(category, outputText); extracted != "" {
			return extracted
		}
	}
	for _, outputText := range report.FailureTails {
		if extracted := extractDetailFromOutput(category, outputText); extracted != "" {
			return extracted
		}
	}
	if headlineText := strings.TrimSpace(report.FailureHeadlines[preferredCheck]); headlineText != "" {
		if extracted := extractLocationFromOutput(headlineText); extracted != "" {
			return extracted
		}
	}
	for _, headlineText := range report.FailureHeadlines {
		if extracted := extractLocationFromOutput(headlineText); extracted != "" {
			return extracted
		}
	}
	return ""
}

func extractDetailFromOutput(category string, outputText string) string {
	normalized := strings.ReplaceAll(outputText, "\r\n", "\n")

	if location := extractLocationFromOutput(normalized); location != "" {
		return location
	}

	if category == "tests" {
		if matchGroups := reGoTestFail.FindStringSubmatch(normalized); len(matchGroups) == 2 {
			return matchGroups[1]
		}
		if matchGroups := reDotnetFail.FindStringSubmatch(normalized); len(matchGroups) == 2 {
			return matchGroups[1]
		}
		if matchGroups := rePytestFail.FindStringSubmatch(normalized); len(matchGroups) == 2 {
			return matchGroups[1]
		}
		if matchGroups := reJestFail.FindStringSubmatch(normalized); len(matchGroups) == 2 {
			return strings.TrimSpace(matchGroups[1])
		}
	}

	return ""
}

func extractLocationFromOutput(outputText string) string {
	if matchGroups := reTscError.FindStringSubmatch(outputText); len(matchGroups) == 5 {
		return formatLocation(matchGroups[1], matchGroups[2], matchGroups[3])
	}
	if matchGroups := reDotnetBuildError.FindStringSubmatch(outputText); len(matchGroups) == 5 {
		return formatLocation(matchGroups[1], matchGroups[2], matchGroups[3])
	}
	if matchGroups := reMavenError.FindStringSubmatch(outputText); len(matchGroups) == 5 {
		return formatLocation(matchGroups[1], matchGroups[2], matchGroups[3])
	}
	if matchGroups := reRuffIssue.FindStringSubmatch(outputText); len(matchGroups) == 6 {
		return formatLocation(matchGroups[1], matchGroups[2], matchGroups[3])
	}
	if matchGroups := reGccError.FindStringSubmatch(outputText); len(matchGroups) == 5 {
		return formatLocation(matchGroups[1], matchGroups[2], matchGroups[3])
	}
	if matchGroups := reRustLocation.FindStringSubmatch(outputText); len(matchGroups) == 4 {
		return formatLocation(matchGroups[1], matchGroups[2], matchGroups[3])
	}
	if matchGroups := reFileLineCol.FindStringSubmatch(outputText); len(matchGroups) == 5 {
		return formatLocation(matchGroups[1], matchGroups[2], matchGroups[3])
	}
	if matchGroups := reFileLine.FindStringSubmatch(outputText); len(matchGroups) == 4 {
		return formatLocation(matchGroups[1], matchGroups[2], "")
	}
	if matchGroups := reBlackFormat.FindStringSubmatch(outputText); len(matchGroups) == 2 {
		return strings.TrimSpace(matchGroups[1])
	}
	if location := eslintLocation(outputText); location != "" {
		return location
	}
	return ""
}

func formatLocation(file string, line string, column string) string {
	file = strings.TrimSpace(file)
	line = strings.TrimSpace(line)
	column = strings.TrimSpace(column)
	if file == "" || line == "" {
		return ""
	}
	if column != "" {
		return file + ":" + line + ":" + column
	}
	return file + ":" + line
}

func eslintLocation(outputText string) string {
	lines := strings.Split(outputText, "\n")
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		currentLine := strings.TrimSpace(lines[lineIndex])
		if currentLine == "" {
			continue
		}
		if !reEslintFile.MatchString(currentLine) {
			continue
		}
		for lookaheadIndex := lineIndex + 1; lookaheadIndex < len(lines) && lookaheadIndex <= lineIndex+6; lookaheadIndex++ {
			nextLine := strings.TrimSpace(lines[lookaheadIndex])
			if nextLine == "" {
				continue
			}
			if matchGroups := reEslintIssue.FindStringSubmatch(nextLine); len(matchGroups) == 3 {
				return currentLine + ":" + matchGroups[1]
			}
		}
	}
	return ""
}

// ensureInsultContext makes sure the insult still points at something real.
// Templates can be funny, but the user should still know what failed.
func ensureInsultContext(message string, check string, detail string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}

	hasCheck := containsInsensitive(message, check)
	hasDetail := containsInsensitive(message, detail)

	switch {
	case hasCheck && hasDetail:
		return message
	case hasCheck && detail != "":
		return message + " (" + detail + ")"
	case hasDetail && check != "":
		return message + " (" + check + ")"
	default:
		context := check
		if detail != "" && detail != check {
			if context != "" {
				context = context + " @ " + detail
			} else {
				context = detail
			}
		}
		if strings.TrimSpace(context) == "" {
			return message
		}
		return message + " (" + context + ")"
	}
}

func containsInsensitive(haystack string, needle string) bool {
	haystack = strings.ToLower(strings.TrimSpace(haystack))
	needle = strings.ToLower(strings.TrimSpace(needle))
	if haystack == "" || needle == "" {
		return false
	}
	return strings.Contains(haystack, needle)
}

func trimDetail(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}

	const maxRunes = 80
	runes := []rune(text)
	if len(runes) > maxRunes {
		// Leave room for ...
		return string(runes[:maxRunes-3]) + "..."
	}
	return text
}

func formatInsult(mode string, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}

	modeKey := strings.ToLower(strings.TrimSpace(mode))
	switch modeKey {
	case "polite":
		if hasPrefixInsensitive(message, []string{"please", "sorry", "apologies"}) {
			return message
		}
		return "Please address the failing checks before pushing. " + message
	case "snarky":
		if hasPrefixInsensitive(message, []string{"no", "nope", "denied", "blocked", "push blocked", "not today"}) {
			return message
		}
		return "Yeah, no. " + message
	case "nuclear":
		return strings.ToUpper("ABSOLUTELY NOT. " + message)
	default:
		return message
	}
}

func hasPrefixInsensitive(message string, prefixes []string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	for _, rawPrefix := range prefixes {
		prefix := strings.ToLower(strings.TrimSpace(rawPrefix))
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}
