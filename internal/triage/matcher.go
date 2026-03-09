package triage

import (
	"strings"
	"unicode"
)

// tokenize splits an intent string into lowercase word tokens, stripping
// punctuation and common stop words.
func tokenize(intent string) []string {
	words := strings.FieldsFunc(strings.ToLower(intent), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var tokens []string
	for _, w := range words {
		if !stopWord(w) {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

// jaccardScore returns the Jaccard similarity between intent tokens and
// tool keywords: |intersection| / |union|. Keywords are pre-lowered at
// registration time.
func jaccardScore(intentTokens, keywords []string) float64 {
	if len(intentTokens) == 0 || len(keywords) == 0 {
		return 0
	}

	kwSet := make(map[string]bool, len(keywords))
	for _, k := range keywords {
		kwSet[strings.ToLower(k)] = true
	}

	intersection := 0
	intentSet := make(map[string]bool, len(intentTokens))
	for _, t := range intentTokens {
		intentSet[t] = true
		if kwSet[t] {
			intersection++
		}
		// Prefix matching: "perform" matches "performance"
		if intersection == 0 || !kwSet[t] {
			for kw := range kwSet {
				if strings.HasPrefix(kw, t) || strings.HasPrefix(t, kw) {
					intersection++
					break
				}
			}
		}
	}
	if intersection == 0 {
		return 0
	}

	union := len(intentSet)
	for k := range kwSet {
		if !intentSet[k] {
			union++
		}
	}
	return float64(intersection) / float64(union)
}

var stops = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"in": true, "on": true, "at": true, "to": true, "for": true,
	"of": true, "and": true, "or": true, "my": true, "me": true,
	"i": true, "it": true, "do": true, "does": true, "this": true,
	"that": true, "with": true, "from": true, "can": true, "how": true,
}

func stopWord(w string) bool {
	return stops[w]
}
