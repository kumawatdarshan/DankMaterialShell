package utils

import (
	"unicode"
	"unicode/utf8"
)

// FoldSubsequence reports whether every rune of query appears in text in
// order, comparing case-insensitively. It folds on the fly instead of
// lowering both sides up front, so hot search loops avoid per-item
// allocations. An empty query matches everything.
func FoldSubsequence(query, text string) bool {
	if query == "" {
		return true
	}
	rest := query
	for _, tr := range text {
		if rest == "" {
			return true
		}
		qr, size := utf8.DecodeRuneInString(rest)
		if unicode.ToLower(qr) == unicode.ToLower(tr) {
			rest = rest[size:]
		}
	}
	return rest == ""
}
