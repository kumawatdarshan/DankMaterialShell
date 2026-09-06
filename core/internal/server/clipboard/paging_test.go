package clipboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storeImageTestEntry(t *testing.T, m *Manager, preview string) uint64 {
	t.Helper()

	require.NoError(t, m.storeEntry(Entry{
		Data:      []byte{0x89, 0x50, 0x4E, 0x47},
		MimeType:  "image/png",
		Preview:   preview,
		Size:      4,
		Timestamp: time.Now().Truncate(time.Second),
		IsImage:   true,
	}))

	for _, entry := range m.GetHistory() {
		if entry.Preview == preview {
			return entry.ID
		}
	}

	t.Fatalf("stored image entry %q not found in history", preview)
	return 0
}

func TestEntryTypeOf_ClassifiesEntries(t *testing.T) {
	assert.Equal(t, "image", entryTypeOf(Entry{IsImage: true, Size: 1}))
	assert.Equal(t, "long_text", entryTypeOf(Entry{Size: longTextThreshold + 1}))
	assert.Equal(t, "text", entryTypeOf(Entry{Size: longTextThreshold}))
	assert.Equal(t, "text", entryTypeOf(Entry{Size: 10}))
}

func TestSearch_PaginatesNewestFirst(t *testing.T) {
	m := newTestManagerWithDB(t)

	for _, text := range []string{"item 0", "item 1", "item 2", "item 3", "item 4"} {
		storeTestEntry(t, m, text)
	}

	first := m.Search(SearchParams{Limit: 2})
	require.Len(t, first.Entries, 2)
	assert.Equal(t, "item 4", first.Entries[0].Preview)
	assert.Equal(t, "item 3", first.Entries[1].Preview)
	assert.Equal(t, 5, first.Total)
	assert.True(t, first.HasMore)

	second := m.Search(SearchParams{Limit: 2, Offset: 2})
	require.Len(t, second.Entries, 2)
	assert.Equal(t, "item 2", second.Entries[0].Preview)
	assert.Equal(t, "item 1", second.Entries[1].Preview)
	assert.Equal(t, 5, second.Total)
	assert.True(t, second.HasMore)

	last := m.Search(SearchParams{Limit: 2, Offset: 4})
	require.Len(t, last.Entries, 1)
	assert.Equal(t, "item 0", last.Entries[0].Preview)
	assert.Equal(t, 5, last.Total)
	assert.False(t, last.HasMore)

	empty := m.Search(SearchParams{Limit: 2, Offset: 10})
	assert.Empty(t, empty.Entries)
	assert.Equal(t, 5, empty.Total)
	assert.False(t, empty.HasMore)
}

func TestSearch_QueryFiltersCaseInsensitive(t *testing.T) {
	m := newTestManagerWithDB(t)

	storeTestEntry(t, m, "Hello World")
	storeTestEntry(t, m, "something else")

	result := m.Search(SearchParams{Query: "hello", Limit: 50})
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "Hello World", result.Entries[0].Preview)
	assert.Equal(t, 1, result.Total)
	assert.False(t, result.HasMore)
}

func TestSearch_EntryTypeFilter(t *testing.T) {
	m := newTestManagerWithDB(t)

	storeTestEntry(t, m, "short text")
	storeTestEntry(t, m, strings.Repeat("l", longTextThreshold+50))
	storeImageTestEntry(t, m, "[[ image pic ]]")

	images := m.Search(SearchParams{EntryType: "image", Limit: 50})
	require.Len(t, images.Entries, 1)
	assert.Equal(t, 1, images.Total)

	longTexts := m.Search(SearchParams{EntryType: "long_text", Limit: 50})
	require.Len(t, longTexts.Entries, 1)
	assert.Equal(t, 1, longTexts.Total)

	texts := m.Search(SearchParams{EntryType: "text", Limit: 50})
	require.Len(t, texts.Entries, 1)
	assert.Equal(t, 1, texts.Total)

	all := m.Search(SearchParams{EntryType: "all", Limit: 50})
	assert.Equal(t, 3, all.Total)
}

func TestSearch_PinnedFilter(t *testing.T) {
	m := newTestManagerWithDB(t)

	plain := storeTestEntry(t, m, "plain")
	pinned := storeTestEntry(t, m, "saved")
	require.NoError(t, m.PinEntry(pinned))

	unpinnedOnly := m.Search(SearchParams{Pinned: boolPtr(false), Limit: 50})
	require.Len(t, unpinnedOnly.Entries, 1)
	assert.Equal(t, plain, unpinnedOnly.Entries[0].ID)

	pinnedOnly := m.Search(SearchParams{Pinned: boolPtr(true), Limit: 50})
	require.Len(t, pinnedOnly.Entries, 1)
	assert.Equal(t, pinned, pinnedOnly.Entries[0].ID)
}

func boolPtr(v bool) *bool {
	return &v
}

func TestDeleteMatching_DeletesOnlyMatches(t *testing.T) {
	m := newTestManagerWithDB(t)

	storeTestEntry(t, m, "alpha one")
	storeTestEntry(t, m, "alpha two")
	storeTestEntry(t, m, "beta")
	keep := storeTestEntry(t, m, "alpha pinned")
	require.NoError(t, m.PinEntry(keep))

	deleted, err := m.DeleteMatching("alpha", "")
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

	remaining := m.Search(SearchParams{Limit: 50})
	assert.Equal(t, 2, remaining.Total)
}

func TestDeleteMatching_RespectsEntryType(t *testing.T) {
	m := newTestManagerWithDB(t)

	storeTestEntry(t, m, "match short")
	storeImageTestEntry(t, m, "match pic")

	deleted, err := m.DeleteMatching("match", "image")
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	remaining := m.Search(SearchParams{Limit: 50})
	require.Len(t, remaining.Entries, 1)
	assert.False(t, remaining.Entries[0].IsImage)
}

func TestUpdateState_CapsHistoryWithCounts(t *testing.T) {
	m := newTestManagerWithDB(t)
	m.config.MaxHistory = stateHeadLimit + 1000

	const stored = stateHeadLimit + 50
	for i := 0; i < stored; i++ {
		storeTestEntry(t, m, fmt.Sprintf("entry %d", i))
	}

	m.updateState()
	state := m.GetState()
	assert.Len(t, state.History, stateHeadLimit)
	assert.Equal(t, stored, state.TotalCount)
	assert.Equal(t, 0, state.PinnedCount)
}

func TestSearch_CursorPaginationWithBeforeID(t *testing.T) {
	m := newTestManagerWithDB(t)

	for _, text := range []string{"item 0", "item 1", "item 2", "item 3", "item 4"} {
		storeTestEntry(t, m, text)
	}

	first := m.Search(SearchParams{Limit: 2})
	require.Len(t, first.Entries, 2)
	assert.Equal(t, "item 4", first.Entries[0].Preview)
	assert.Equal(t, "item 3", first.Entries[1].Preview)
	assert.True(t, first.HasMore)

	lastID := first.Entries[1].ID
	second := m.Search(SearchParams{Limit: 2, BeforeID: &lastID})
	require.Len(t, second.Entries, 2)
	assert.Equal(t, "item 2", second.Entries[0].Preview)
	assert.Equal(t, "item 1", second.Entries[1].Preview)
	assert.True(t, second.HasMore)

	lastID = second.Entries[1].ID
	third := m.Search(SearchParams{Limit: 2, BeforeID: &lastID})
	require.Len(t, third.Entries, 1)
	assert.Equal(t, "item 0", third.Entries[0].Preview)
	assert.False(t, third.HasMore)
}

func TestSearch_CursorPaginationNoDuplicateOnDelete(t *testing.T) {
	m := newTestManagerWithDB(t)

	var ids []uint64
	for _, text := range []string{"item 0", "item 1", "item 2", "item 3", "item 4"} {
		ids = append(ids, storeTestEntry(t, m, text))
	}

	first := m.Search(SearchParams{Limit: 2})
	require.Len(t, first.Entries, 2)
	assert.Equal(t, "item 4", first.Entries[0].Preview)
	assert.Equal(t, "item 3", first.Entries[1].Preview)

	// Delete item 4 from the first page
	require.NoError(t, m.DeleteEntry(ids[4]))

	// Cursor pagination from item 3's ID still accurately fetches page 2 without duplicates
	cursorID := first.Entries[1].ID
	second := m.Search(SearchParams{Limit: 2, BeforeID: &cursorID})
	require.Len(t, second.Entries, 2)
	assert.Equal(t, "item 2", second.Entries[0].Preview)
	assert.Equal(t, "item 1", second.Entries[1].Preview)
}

func TestPinnedCache_Consistency(t *testing.T) {
	m := newTestManagerWithDB(t)

	id1 := storeTestEntry(t, m, "first")
	id2 := storeTestEntry(t, m, "second")

	assert.Equal(t, 2, m.getUnpinnedCount())
	assert.Equal(t, 0, m.GetPinnedCount())

	require.NoError(t, m.PinEntry(id1))
	assert.Equal(t, 1, m.getUnpinnedCount())
	assert.Equal(t, 1, m.GetPinnedCount())
	assert.Len(t, m.GetPinnedEntries(), 1)
	assert.Equal(t, id1, m.GetPinnedEntries()[0].ID)

	require.NoError(t, m.UnpinEntry(id1))
	assert.Equal(t, 2, m.getUnpinnedCount())
	assert.Equal(t, 0, m.GetPinnedCount())
	assert.Empty(t, m.GetPinnedEntries())

	require.NoError(t, m.PinEntry(id2))
	assert.Equal(t, 1, m.GetPinnedCount())
	require.NoError(t, m.DeleteEntry(id2))
	assert.Equal(t, 0, m.GetPinnedCount())
	assert.Empty(t, m.GetPinnedEntries())
}

func TestSearch_QueryFiltersUnicodeCaseInsensitive(t *testing.T) {
	m := newTestManagerWithDB(t)

	storeTestEntry(t, m, "Café résumé")
	storeTestEntry(t, m, "regular english text")

	result := m.Search(SearchParams{Query: "café", Limit: 10})
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "Café résumé", result.Entries[0].Preview)

	resultUpper := m.Search(SearchParams{Query: "RÉSUMÉ", Limit: 10})
	require.Len(t, resultUpper.Entries, 1)
	assert.Equal(t, "Café résumé", resultUpper.Entries[0].Preview)
}

