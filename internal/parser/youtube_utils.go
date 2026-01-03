package parser

import (
	"regexp"
	"strings"
)

var (
	// Noise reduction regex
	noiseRegex = regexp.MustCompile(`(?i)\((official video|official audio|audio|video|lyrics|HD|Remastered|Remaster(ed)?)\)|\[(official video|official audio|audio|video|lyrics|HD|Remastered|Remaster(ed)?)\]`)
	featRegex  = regexp.MustCompile(`(?i)\bfeat\.?\b`)
	spaceRegex = regexp.MustCompile(`\s{2,}`)
	splitRegex = regexp.MustCompile(`\s+[-|–—|:]\s+`)
)

// NormalizeYTTitle replicates YouTubeParserV3._normalize_title
func NormalizeYTTitle(rawTitle string, uploader string) (string, string) {
	t := rawTitle

	// 1. Clean noise
	t = noiseRegex.ReplaceAllString(t, "")
	t = featRegex.ReplaceAllString(t, "ft.")
	t = spaceRegex.ReplaceAllString(t, " ")
	t = strings.TrimSpace(t)

	// 2. Strong heuristic: literal "Artist - Title"
	// (this is your original function's logic, kept intentionally)
	if strings.Contains(t, " - ") {
		parts := strings.SplitN(t, " - ", 2)
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])

		if left != "" && right != "" {
			return capWords(left), capWords(right)
		}
	}

	// 3. Flexible split using regex + heuristics
	parts := splitRegex.Split(t, 2)
	if len(parts) == 2 {
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])

		if looksLikeArtist(left, right) {
			return capWords(left), capWords(right)
		}
		return capWords(right), capWords(left)
	}

	// 4. Fallback: uploader as artist
	if uploader != "" {
		return capWords(uploader), capWords(t)
	}

	// 5. Last resort
	return "", capWords(t)
}

// looksLikeArtist replicates the heuristic: 
// if left contains commas/ft or is short (<=4 words) while right is longer
func looksLikeArtist(left, right string) bool {
	leftLower := strings.ToLower(left)
	if strings.Contains(left, ",") || strings.Contains(leftLower, "ft.") || strings.Contains(leftLower, "feat.") {
		return true
	}

	leftWords := len(strings.Fields(left))
	rightWords := len(strings.Fields(right))

	if leftWords <= 4 && rightWords >= 2 {
		return true
	}
	return false
}

// capWords replicates Python's string.capwords but preserves small acronyms
func capWords(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		// If it's already all caps and short (like ISRC or DJ), keep it
		if w == strings.ToUpper(w) && len(w) <= 4 {
			continue
		}
		words[i] = strings.Title(strings.ToLower(w))
	}
	return strings.Join(words, " ")
}
