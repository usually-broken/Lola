// Package speak turns text into speech, and decides where a sentence ends.
//
// Splitting used to live in the page, one regular expression in JavaScript. It lives
// here now because it is a rule about language rather than about drawing, and because
// a rule in one place can be tested: a decimal point is not the end of a sentence, and
// neither is an abbreviation, and neither is a run-on that never stops.
package speak

import (
	"strings"
	"unicode"
)

// maxRunOn is where a sentence with no punctuation is cut, at a word boundary. Long
// enough to hold a clause, short enough that speech starts before the paragraph ends.
const maxRunOn = 240

// Split breaks text into the sentences it will be spoken as.
//
// A sentence ends at . ! or ? followed by whitespace, unless the dot belongs to a
// number (3.5) or a short abbreviation (e.g., Mr.). Anything left over that grows past
// maxRunOn is cut at the last space before the limit, so a model that forgets to
// punctuate still gets spoken in pieces rather than one enormous breath.
func Split(text string) []string {
	var out []string

	// Everything here works in runes. Slicing a string by a rune count cuts bytes, which
	// is how an em dash ended up split down the middle and arrived as mojibake.
	rest := []rune(strings.TrimSpace(text))

	for len(rest) > 0 {
		if cut := sentenceEnd(rest); cut > 0 {
			if sentence := strings.TrimSpace(string(rest[:cut])); sentence != "" {
				out = append(out, sentence)
			}
			rest = []rune(strings.TrimSpace(string(rest[cut:])))
			continue
		}

		if len(rest) > maxRunOn {
			if sentence, remainder := cutAtWord(string(rest)); sentence != "" {
				out = append(out, sentence)
				rest = []rune(remainder)
				continue
			}
		}

		out = append(out, strings.TrimSpace(string(rest)))
		break
	}

	return out
}

// sentenceEnd is where the first finished sentence ends, or -1 if none does.
func sentenceEnd(runes []rune) int {
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		// An em dash is an ending too — it is how somebody writes a spoken pause.
		if c == '—' {
			return i + 1
		}
		if c != '.' && c != '!' && c != '?' {
			continue
		}

		// A terminator followed by more text is only an ending if there is a space
		// after it: "3.5" and "e.g." are not ends, "done. next" is.
		if i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
			continue
		}
		if c == '.' && i > 0 && unicode.IsDigit(runes[i-1]) {
			continue
		}

		// A trailing dot in a short abbreviation belongs to the word, not the
		// sentence: "Mr. Smith" is one sentence.
		if c == '.' && isAbbreviation(runes[:i]) {
			continue
		}

		return i + 1
	}
	return -1
}

// isAbbreviation reports whether the word ending just before this dot is one of the
// short forms that carry their own full stop.
func isAbbreviation(before []rune) bool {
	start := len(before)
	for start > 0 && !unicode.IsSpace(before[start-1]) {
		start--
	}
	word := strings.ToLower(strings.TrimSpace(string(before[start:])))

	switch word {
	case "mr", "mrs", "ms", "dr", "prof", "sr", "jr", "st", "vs", "etc", "e.g", "i.e", "approx":
		return true
	}
	return false
}

// cutAtWord splits at the last space before the limit, so a run-on is cut between
// words rather than through one.
func cutAtWord(s string) (string, string) {
	runes := []rune(s)
	limit := maxRunOn
	if limit > len(runes) {
		limit = len(runes)
	}

	// Prefer a comma: a long sentence cut there sounds like a breath rather than a
	// glitch. Fall back to a space, then to the limit itself.
	for _, want := range []func(rune) bool{
		func(r rune) bool { return r == ',' || r == ';' || r == ':' },
		unicode.IsSpace,
	} {
		for i := limit; i > limit/3; i-- {
			if want(runes[i]) {
				return strings.TrimSpace(string(runes[:i+1])), strings.TrimSpace(string(runes[i+1:]))
			}
		}
	}
	return strings.TrimSpace(string(runes[:limit])), strings.TrimSpace(string(runes[limit:]))
}
