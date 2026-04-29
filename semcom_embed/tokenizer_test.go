package semindex

import (
	"testing"

	"github.com/RoaringBitmap/roaring"
)

func TestTokenizePhraseMatch(t *testing.T) {
	words := map[string]int32{
		"new":          1,
		"york":         2,
		"new york":     3,
		"city":         4,
		"new york city": 5,
		"alice":        6,
	}

	t.Run("single word", func(t *testing.T) {
		bm, oov := tokenize("alice", words)
		assertBitmap(t, bm, []uint32{6})
		assertOOV(t, oov, nil)
	})

	t.Run("phrase beats constituent words", func(t *testing.T) {
		bm, oov := tokenize("new york", words)
		// should emit phrase token 3, not individual tokens 1 and 2
		assertBitmap(t, bm, []uint32{3})
		assertOOV(t, oov, nil)
	})

	t.Run("longest phrase wins", func(t *testing.T) {
		bm, oov := tokenize("new york city", words)
		// should emit phrase token 5, not 3+4 or 1+2+4
		assertBitmap(t, bm, []uint32{5})
		assertOOV(t, oov, nil)
	})

	t.Run("phrase mid-sentence", func(t *testing.T) {
		bm, oov := tokenize("alice visited new york city", words)
		assertBitmap(t, bm, []uint32{6, 5})
		assertOOV(t, oov, []string{"visited"})
	})

	t.Run("oov when no match", func(t *testing.T) {
		bm, oov := tokenize("foo bar", words)
		if !bm.IsEmpty() {
			t.Errorf("expected empty bitmap, got %v", bm)
		}
		assertOOV(t, oov, []string{"foo", "bar"})
	})
}

func assertBitmap(t *testing.T, bm *roaring.Bitmap, want []uint32) {
	t.Helper()
	got := bm.ToArray()
	if len(got) != len(want) {
		t.Errorf("bitmap: got %v, want %v", got, want)
		return
	}
	wantSet := make(map[uint32]bool, len(want))
	for _, v := range want {
		wantSet[v] = true
	}
	for _, v := range got {
		if !wantSet[v] {
			t.Errorf("bitmap: unexpected token %d (got %v, want %v)", v, got, want)
		}
	}
}

func assertOOV(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("oov: got %v, want %v", got, want)
		return
	}
	wantSet := make(map[string]bool, len(want))
	for _, v := range want {
		wantSet[v] = true
	}
	for _, v := range got {
		if !wantSet[v] {
			t.Errorf("oov: unexpected word %q (got %v, want %v)", v, got, want)
		}
	}
}
