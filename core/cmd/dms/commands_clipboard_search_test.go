package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSearchParams(t *testing.T) {
	p := buildSearchParams("hello", 50, false, false)
	assert.Equal(t, map[string]any{"limit": 50, "query": "hello"}, p)

	p = buildSearchParams("", 25, true, false)
	assert.Equal(t, map[string]any{"limit": 25, "entryType": "image"}, p)

	p = buildSearchParams("q", 10, false, true)
	assert.Equal(t, map[string]any{"limit": 10, "query": "q", "entryType": "text"}, p)

	p = buildSearchParams("", 50, true, true)
	assert.Equal(t, "image", p["entryType"])
}

func TestLastEntryID(t *testing.T) {
	id, ok := lastEntryID([]any{
		map[string]any{"id": float64(10)},
		map[string]any{"id": float64(7)},
	})
	require.True(t, ok)
	assert.Equal(t, uint64(7), id)

	_, ok = lastEntryID(nil)
	assert.False(t, ok)

	_, ok = lastEntryID([]any{map[string]any{"noid": true}})
	assert.False(t, ok)

	_, ok = lastEntryID([]any{"not-a-map"})
	assert.False(t, ok)
}

func TestCollectSearchPages(t *testing.T) {
	pages := [][]any{
		{map[string]any{"id": float64(3)}, map[string]any{"id": float64(2)}},
		{map[string]any{"id": float64(1)}},
	}
	var anchors []*uint64
	all, err := collectSearchPages(func(beforeID *uint64) ([]any, bool, error) {
		anchors = append(anchors, beforeID)
		if beforeID == nil {
			return pages[0], true, nil
		}
		require.Equal(t, uint64(2), *beforeID)
		return pages[1], false, nil
	})
	require.NoError(t, err)
	assert.Len(t, all, 3)
	require.Len(t, anchors, 2)
	assert.Nil(t, anchors[0])
	require.NotNil(t, anchors[1])
	assert.Equal(t, uint64(2), *anchors[1])
}

func TestCollectSearchPages_Empty(t *testing.T) {
	calls := 0
	all, err := collectSearchPages(func(beforeID *uint64) ([]any, bool, error) {
		calls++
		return nil, false, nil
	})
	require.NoError(t, err)
	assert.Empty(t, all)
	assert.Equal(t, 1, calls)
}

func TestCollectSearchPages_Error(t *testing.T) {
	boom := errors.New("boom")
	_, err := collectSearchPages(func(beforeID *uint64) ([]any, bool, error) {
		if beforeID == nil {
			return []any{map[string]any{"id": float64(5)}}, true, nil
		}
		return nil, false, boom
	})
	assert.ErrorIs(t, err, boom)
}

func TestCollectSearchPages_MalformedTail(t *testing.T) {
	all, err := collectSearchPages(func(beforeID *uint64) ([]any, bool, error) {
		if beforeID == nil {
			return []any{map[string]any{"id": float64(5)}}, true, nil
		}
		return []any{"garbage"}, true, nil
	})
	require.NoError(t, err)
	assert.Len(t, all, 2)
}
