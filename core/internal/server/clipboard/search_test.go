package clipboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedSearchRows(t *testing.T, m *Manager, n int) {
	t.Helper()
	var entries []Entry
	for i := 1; i <= n; i++ {
		d := []byte(fmt.Sprintf("item %d body text", i))
		entries = append(entries, Entry{
			ID: uint64(i), Data: d, MimeType: "text/plain",
			Preview: string(d), Size: len(d),
			Timestamp: time.Now(), Hash: computeHash(d),
		})
	}
	seedBenchRows(t, m, entries)
	m.initCache()
}

func TestSearch_CursorPaging(t *testing.T) {
	m := newTestManagerWithDB(t)
	seedSearchRows(t, m, 5)
	unpinned := false

	first := m.Search(SearchParams{Limit: 2, Pinned: &unpinned})
	require.Len(t, first.Entries, 2)
	assert.Equal(t, uint64(5), first.Entries[0].ID)
	assert.Equal(t, uint64(4), first.Entries[1].ID)
	assert.Equal(t, 5, first.Total)
	assert.True(t, first.TotalKnown)
	assert.True(t, first.HasMore)

	anchor := first.Entries[1].ID
	second := m.Search(SearchParams{Limit: 2, BeforeID: &anchor, Pinned: &unpinned})
	require.Len(t, second.Entries, 2)
	assert.Equal(t, uint64(3), second.Entries[0].ID)
	assert.Equal(t, uint64(2), second.Entries[1].ID)
	assert.True(t, second.HasMore)

	anchor = second.Entries[1].ID
	last := m.Search(SearchParams{Limit: 2, BeforeID: &anchor, Pinned: &unpinned})
	require.Len(t, last.Entries, 1)
	assert.Equal(t, uint64(1), last.Entries[0].ID)
	assert.False(t, last.HasMore)
}

func TestSearch_CursorNoGapOnDelete(t *testing.T) {
	m := newTestManagerWithDB(t)
	seedSearchRows(t, m, 5)
	unpinned := false

	first := m.Search(SearchParams{Limit: 2, Pinned: &unpinned})
	require.Len(t, first.Entries, 2)
	anchor := first.Entries[1].ID

	require.NoError(t, m.DeleteEntry(anchor))

	second := m.Search(SearchParams{Limit: 2, BeforeID: &anchor, Pinned: &unpinned})
	require.Len(t, second.Entries, 2)
	assert.Equal(t, uint64(3), second.Entries[0].ID)
	assert.Equal(t, uint64(2), second.Entries[1].ID)
}

func TestSearch_FilteredTotalUnknown(t *testing.T) {
	m := newTestManagerWithDB(t)
	seedSearchRows(t, m, 5)

	res := m.Search(SearchParams{Query: "item", Limit: 50})
	assert.Len(t, res.Entries, 5)
	assert.Equal(t, -1, res.Total)
	assert.False(t, res.TotalKnown)
	assert.False(t, res.HasMore)

	res = m.Search(SearchParams{Query: "zzz-no-match", Limit: 50})
	assert.Empty(t, res.Entries)
	assert.Equal(t, -1, res.Total)
	assert.False(t, res.TotalKnown)
	assert.False(t, res.HasMore)
}

func TestSearch_EntryType(t *testing.T) {
	m := newTestManagerWithDB(t)
	text := []byte("short text")
	img := []byte{0x89, 0x50, 0x4E, 0x47}
	long := []byte(strings.Repeat("lorem ipsum dolor sit amet ", 20))
	seedBenchRows(t, m, []Entry{
		{ID: 1, Data: text, MimeType: "text/plain", Preview: "short text", Size: len(text), Timestamp: time.Now(), Hash: computeHash(text)},
		{ID: 2, Data: img, MimeType: "image/png", Preview: "[[ image ]]", Size: len(img), Timestamp: time.Now(), IsImage: true, Hash: computeHash(img)},
		{ID: 3, Data: long, MimeType: "text/plain", Preview: string(long[:50]), Size: len(long), Timestamp: time.Now(), Hash: computeHash(long)},
	})
	m.initCache()

	images := m.Search(SearchParams{EntryType: "image", Limit: 50})
	require.Len(t, images.Entries, 1)
	assert.Equal(t, uint64(2), images.Entries[0].ID)

	texts := m.Search(SearchParams{EntryType: "text", Limit: 50})
	require.Len(t, texts.Entries, 1)
	assert.Equal(t, uint64(1), texts.Entries[0].ID)

	longs := m.Search(SearchParams{EntryType: "long_text", Limit: 50})
	require.Len(t, longs.Entries, 1)
	assert.Equal(t, uint64(3), longs.Entries[0].ID)

	all := m.Search(SearchParams{EntryType: "all", Limit: 50})
	assert.Len(t, all.Entries, 3)

	unknown := m.Search(SearchParams{EntryType: "bogus", Limit: 50})
	assert.Empty(t, unknown.Entries)
	assert.False(t, unknown.HasMore)
}

func TestSearch_PinnedFilter(t *testing.T) {
	m := newTestManagerWithDB(t)
	seedSearchRows(t, m, 3)
	require.NoError(t, m.PinEntry(3))

	pinned := true
	onlyPinned := m.Search(SearchParams{Pinned: &pinned, Limit: 50})
	require.Len(t, onlyPinned.Entries, 1)
	assert.Equal(t, uint64(3), onlyPinned.Entries[0].ID)
	assert.Equal(t, -1, onlyPinned.Total)
	assert.False(t, onlyPinned.TotalKnown)

	unpinned := false
	rest := m.Search(SearchParams{Pinned: &unpinned, Limit: 50})
	assert.Len(t, rest.Entries, 2)
	assert.Equal(t, 2, rest.Total)
	assert.True(t, rest.TotalKnown)
}

func TestSearch_UnicodeFold(t *testing.T) {
	m := newTestManagerWithDB(t)
	rows := []struct {
		id      uint64
		preview string
	}{
		{1, "Café au lait notes"},
		{2, "RÉSUMÉ draft document"},
		{3, "plain ascii meeting"},
	}
	var entries []Entry
	for _, r := range rows {
		d := []byte(r.preview)
		entries = append(entries, Entry{ID: r.id, Data: d, MimeType: "text/plain", Preview: r.preview, Size: len(d), Timestamp: time.Now(), Hash: computeHash(d)})
	}
	seedBenchRows(t, m, entries)
	m.initCache()

	cafe := m.Search(SearchParams{Query: "café", Limit: 50})
	require.Len(t, cafe.Entries, 1)
	assert.Equal(t, uint64(1), cafe.Entries[0].ID)

	resume := m.Search(SearchParams{Query: "résumé", Limit: 50})
	require.Len(t, resume.Entries, 1)
	assert.Equal(t, uint64(2), resume.Entries[0].ID)

	upper := m.Search(SearchParams{Query: "CAFÉ", Limit: 50})
	require.Len(t, upper.Entries, 1)
	assert.Equal(t, uint64(1), upper.Entries[0].ID)
}

func TestSearch_LimitClamp(t *testing.T) {
	m := newTestManagerWithDB(t)
	seedSearchRows(t, m, 5)

	assert.Len(t, m.Search(SearchParams{Limit: 0}).Entries, 5)
	assert.Len(t, m.Search(SearchParams{Limit: 100000}).Entries, 5)
}

func TestDeleteMatching(t *testing.T) {
	m := newTestManagerWithDB(t)
	mk := func(id uint64, preview string, pinned bool) Entry {
		d := []byte(preview)
		return Entry{ID: id, Data: d, MimeType: "text/plain", Preview: preview, Size: len(d), Timestamp: time.Now(), Hash: computeHash(d), Pinned: pinned}
	}
	seedBenchRows(t, m, []Entry{
		mk(1, "delete me one", false),
		mk(2, "delete me two", false),
		mk(3, "delete me pinned", true),
		mk(4, "keep me", false),
	})
	m.initCache()
	require.Equal(t, int64(3), m.unpinnedCount.Load())

	deleted, err := m.DeleteMatching("delete me", "")
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)
	assert.Equal(t, int64(1), m.unpinnedCount.Load())

	rest := m.Search(SearchParams{Limit: 50})
	ids := []uint64{rest.Entries[0].ID, rest.Entries[1].ID}
	assert.ElementsMatch(t, []uint64{3, 4}, ids)

	deleted, err = m.DeleteMatching("nothing matches this", "")
	require.NoError(t, err)
	assert.Equal(t, 0, deleted)
	assert.Equal(t, int64(1), m.unpinnedCount.Load())
}

func TestDeleteMatching_EntryType(t *testing.T) {
	m := newTestManagerWithDB(t)
	img := []byte{0x89, 0x50, 0x4E, 0x47}
	text := []byte("keep this text")
	seedBenchRows(t, m, []Entry{
		{ID: 1, Data: img, MimeType: "image/png", Preview: "[[ image ]]", Size: len(img), Timestamp: time.Now(), IsImage: true, Hash: computeHash(img)},
		{ID: 2, Data: text, MimeType: "text/plain", Preview: "keep this text", Size: len(text), Timestamp: time.Now(), Hash: computeHash(text)},
	})
	m.initCache()

	deleted, err := m.DeleteMatching("", "image")
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	rest := m.Search(SearchParams{Limit: 50})
	require.Len(t, rest.Entries, 1)
	assert.Equal(t, uint64(2), rest.Entries[0].ID)
}
