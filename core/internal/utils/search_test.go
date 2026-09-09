package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkFoldSubsequence(b *testing.B) {
	cases := [][2]string{
		{"fire", "Firefox Web Browser"},
		{"net", "Network"},
		{"moz", "Mozilla Corporation"},
		{"web", "A fast, private web browser from Mozilla"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, c := range cases {
			if !FoldSubsequence(c[0], c[1]) {
				b.Fatal("expected match")
			}
		}
		if FoldSubsequence("zzz-no-match", cases[0][1]) {
			b.Fatal("expected no match")
		}
	}
}
func TestFoldSubsequence(t *testing.T) {
	tests := []struct {
		name  string
		query string
		text  string
		want  bool
	}{
		{"empty query matches", "", "anything", true},
		{"empty text no match", "a", "", false},
		{"exact", "term", "term", true},
		{"subsequence", "tmk", "tamuk", true},
		{"out of order", "bca", "abc", false},
		{"query longer than text", "abcdef", "abc", false},
		{"case insensitive both ways", "FiRe", "firefox", true},
		{"upper text lower query", "fire", "FIREFOX", true},
		{"multibyte query", "café", "Café au lait", true},
		{"multibyte case fold", "RÉSUMÉ", "résumé draft", true},
		{"cjk subsequence", "日本", "日本語テスト", true},
		{"cjk no match", "韓国", "日本語テスト", false},
		{"multibyte byte trap", "é", "e", false},
		{"repeated runes", "aa", "aba", true},
		{"repeated runes exhausted", "aaa", "aba", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FoldSubsequence(tt.query, tt.text))
		})
	}
}
