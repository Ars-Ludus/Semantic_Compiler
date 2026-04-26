package semindex

import (
	"strings"
	"unicode"

	"github.com/RoaringBitmap/roaring"
)

// tokenize splits text into lowercase words and returns a bitmap of all
// token_ids found in the index word map, plus a list of unknown words.
func tokenize(text string, words map[string]int32) (*roaring.Bitmap, []string) {
	bm := roaring.New()
	var oov []string
	for _, word := range SplitWords(text) {
		if id, ok := words[word]; ok {
			bm.Add(uint32(id))
		} else {
			oov = append(oov, word)
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
