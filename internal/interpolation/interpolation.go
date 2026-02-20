package interpolation

import (
	"fmt"
	"regexp"
	"strings"
)

// Mapping stores the original placeholder and its safe replacement.
type Mapping struct {
	Original    string
	Placeholder string
	Index       int
}

// varMatch stores a detected interpolation variable position.
type varMatch struct {
	start, end int
	value      string
}

// patterns to detect interpolation variables in game strings.
var patterns = []*regexp.Regexp{
	regexp.MustCompile(`\$\{[a-zA-Z_][a-zA-Z0-9_]*\}`),         // ${value}
	regexp.MustCompile(`\{[0-9]+\}`),                           // {0}, {1}
	regexp.MustCompile(`%[-+0-9]*\.?[0-9]*[dsfieEgGxXoubcpq]`), // %d, %s, %f, %2d, etc.
	regexp.MustCompile(`%%`),                                   // escaped percent literal
}

// Protect replaces all interpolation variables with safe {{var_N}} placeholders.
// Returns the safe string and a mapping to restore originals after translation.
func Protect(text string) (string, []Mapping) {
	var allMatches []varMatch
	for _, p := range patterns {
		locs := p.FindAllStringIndex(text, -1)
		for _, loc := range locs {
			allMatches = append(allMatches, varMatch{
				start: loc[0],
				end:   loc[1],
				value: text[loc[0]:loc[1]],
			})
		}
	}

	if len(allMatches) == 0 {
		return text, nil
	}

	// Sort by position to ensure deterministic ordering.
	sortVarMatches(allMatches)

	// Remove overlapping matches (keep the first/longest).
	var filtered []varMatch
	lastEnd := -1
	for _, m := range allMatches {
		if m.start >= lastEnd {
			filtered = append(filtered, m)
			lastEnd = m.end
		}
	}

	var mappings []Mapping
	result := text
	// Replace in reverse order to preserve indices.
	for i := len(filtered) - 1; i >= 0; i-- {
		m := filtered[i]
		placeholder := fmt.Sprintf("{{var_%d}}", i+1)
		mappings = append([]Mapping{{
			Original:    m.value,
			Placeholder: placeholder,
			Index:       i + 1,
		}}, mappings...)
		result = result[:m.start] + placeholder + result[m.end:]
	}

	return result, mappings
}

// Restore replaces {{var_N}} placeholders back with the original interpolation variables.
func Restore(translated string, mappings []Mapping) string {
	result := translated
	for _, m := range mappings {
		result = strings.Replace(result, m.Placeholder, m.Original, 1)
	}
	return result
}

// sortVarMatches sorts by start position, then by length (descending) for overlaps.
func sortVarMatches(matches []varMatch) {
	for i := 1; i < len(matches); i++ {
		key := matches[i]
		j := i - 1
		for j >= 0 && (matches[j].start > key.start ||
			(matches[j].start == key.start && (matches[j].end-matches[j].start) < (key.end-key.start))) {
			matches[j+1] = matches[j]
			j--
		}
		matches[j+1] = key
	}
}
