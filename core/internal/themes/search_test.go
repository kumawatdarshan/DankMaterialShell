package themes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuzzySearch(t *testing.T) {
	list := []Theme{
		{ID: "nord", Name: "Nord", Description: "Arctic theme", Author: "Arctic"},
		{ID: "plain", Name: "Plain", Description: "Nothing", Author: "Anon"},
	}

	assert.Len(t, FuzzySearch("", list), 2)
	assert.Equal(t, "nord", FuzzySearch("nord", list)[0].ID)
	assert.Equal(t, "nord", FuzzySearch("ARCTIC", list)[0].ID)
	assert.Empty(t, FuzzySearch("zzz-no-match", list))
}

func TestFuzzySearch_Multibyte(t *testing.T) {
	list := []Theme{
		{ID: "cafe", Name: "Café", Description: "Warm theme", Author: "José"},
		{ID: "other", Name: "Plain", Description: "Nothing", Author: "Anon"},
	}

	res := FuzzySearch("café", list)
	require.Len(t, res, 1)
	assert.Equal(t, "cafe", res[0].ID)
}
