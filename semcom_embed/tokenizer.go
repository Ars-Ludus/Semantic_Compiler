package semindex

import (
	"strings"
	"unicode"

	"github.com/RoaringBitmap/roaring"
)

// tokenize splits text into lowercase words and returns a bitmap of all
// token_ids found in the index word map, plus a list of unknown words.
// Multi-word phrases in the vocabulary are matched greedily: at each position
// the longest matching phrase wins before falling back to a single-word lookup.
func tokenize(text string, words map[string]int32) (*roaring.Bitmap, []string) {
	tokens := SplitWords(text)
	bm := roaring.New()
	var oov []string

	for i := 0; i < len(tokens); {
		// Try longest match first, shrinking the window down to 1.
		matched := false
		for end := len(tokens); end > i; end-- {
			phrase := strings.Join(tokens[i:end], " ")
			if id, ok := words[phrase]; ok {
				bm.Add(uint32(id))
				i = end
				matched = true
				break
			}
		}
		if !matched {
			oov = append(oov, tokens[i])
			i++
		}
	}

	return bm, oov
}

// SplitWords lowercases text and splits on any non-letter, non-digit rune.
func SplitWords(text string) []string {
	return SplitWordsPreserveCase(strings.ToLower(text))
}

// SplitWordsPreserveCase splits on any non-letter, non-digit rune but preserves original casing.
func SplitWordsPreserveCase(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
