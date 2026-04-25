package semindex

import (
	"strings"
	"unicode"

	"github.com/RoaringBitmap/roaring"
)

// tokenize splits text into lowercase words and returns a bitmap of all
// token_ids found in the index word map. Unknown words are silently skipped.
func tokenize(text string, words map[string]int32) *roaring.Bitmap {
	bm := roaring.New()
	for _, word := range splitWords(text) {
		if id, ok := words[word]; ok {
			bm.Add(uint32(id))
		}
	}
	return bm
}

// splitWords lowercases text and splits on any non-letter, non-digit rune.
func splitWords(text string) []string {
	lower := strings.ToLower(text)
	return strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
