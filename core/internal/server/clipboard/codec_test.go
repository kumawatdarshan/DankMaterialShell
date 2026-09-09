package clipboard

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestDecodeEntryMeta_AltData(t *testing.T) {
	original := Entry{
		ID:          555,
		Data:        []byte("image bytes"),
		MimeType:    "image/png",
		Preview:     "[[ image ]]",
		Size:        11,
		Timestamp:   time.Now().Truncate(time.Second),
		IsImage:     true,
		Hash:        computeHash([]byte("image bytes")),
		AltData:     []byte("fallback text"),
		AltMimeType: "text/plain;charset=utf-8",
	}

	encoded, err := encodeEntry(original)
	require.NoError(t, err)

	meta, err := decodeEntryMeta(encoded)
	require.NoError(t, err)
	assert.Empty(t, meta.Data)
	assert.Equal(t, original.AltMimeType, meta.AltMimeType)
	assert.Equal(t, original.AltData, meta.AltData)
}

func TestParseEntryHeader_Fields(t *testing.T) {
	original := Entry{
		ID:        42,
		Data:      []byte("some data"),
		MimeType:  "text/plain",
		Preview:   "some data",
		Size:      9,
		Timestamp: time.Now().Truncate(time.Second),
		Hash:      computeHash([]byte("some data")),
		Pinned:    true,
	}

	encoded, err := encodeEntry(original)
	require.NoError(t, err)

	h, ok := parseEntryHeader(encoded)
	require.True(t, ok)
	assert.Equal(t, original.ID, h.id)
	assert.Equal(t, original.MimeType, string(h.mimeType))
	assert.Equal(t, original.Preview, string(h.preview))
	assert.Equal(t, original.Size, h.size)
	assert.Equal(t, original.Timestamp.Unix(), h.ts)
	assert.Equal(t, original.IsImage, h.isImage)
	assert.Equal(t, original.Hash, h.hash)
	assert.True(t, h.pinned)
}

func TestParseEntryHeader_Corrupt(t *testing.T) {
	original := Entry{
		ID:        7,
		Data:      []byte("payload"),
		MimeType:  "text/plain",
		Preview:   "payload",
		Size:      7,
		Timestamp: time.Now().Truncate(time.Second),
		Hash:      computeHash([]byte("payload")),
	}
	encoded, err := encodeEntry(original)
	require.NoError(t, err)

	cuts := []int{0, 1, 7, 8, 12, 20, len(encoded) - 5, len(encoded) - 1}
	for _, cut := range cuts {
		_, ok := parseEntryHeader(encoded[:cut])
		assert.False(t, ok, "cut at %d", cut)
	}

	_, ok := parseEntryHeader([]byte("garbage-not-an-entry-at-all!!!!!!!!"))
	assert.False(t, ok)
}

func seedBenchRows(t *testing.T, m *Manager, entries []Entry) {
	t.Helper()
	err := m.dbUpdate(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("clipboard"))
		for _, e := range entries {
			enc, err := encodeEntry(e)
			if err != nil {
				return err
			}
			if err := b.Put(itob(e.ID), enc); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)
}

func rowIDs(t *testing.T, m *Manager) []uint64 {
	t.Helper()
	var ids []uint64
	require.NoError(t, m.dbView(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("clipboard")).Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			ids = append(ids, binary.BigEndian.Uint64(k))
		}
		return nil
	}))
	return ids
}

func TestDedupByHash(t *testing.T) {
	m := newTestManagerWithDB(t)
	d1 := []byte("dupe")
	d2 := []byte("other")
	seedBenchRows(t, m, []Entry{
		{ID: 1, Data: d1, MimeType: "text/plain", Preview: "a", Size: 4, Timestamp: time.Now(), Hash: computeHash(d1)},
		{ID: 2, Data: d1, MimeType: "text/plain", Preview: "b", Size: 4, Timestamp: time.Now(), Hash: computeHash(d1)},
		{ID: 3, Data: d1, MimeType: "text/plain", Preview: "c", Size: 4, Timestamp: time.Now(), Hash: computeHash(d1), Pinned: true},
		{ID: 4, Data: d2, MimeType: "text/plain", Preview: "d", Size: 5, Timestamp: time.Now(), Hash: computeHash(d2)},
	})

	var deleted int
	require.NoError(t, m.dbUpdate(func(tx *bolt.Tx) error {
		var err error
		deleted, err = dedupByHash(tx.Bucket([]byte("clipboard")), computeHash(d1))
		return err
	}))
	assert.Equal(t, 2, deleted)
	assert.Equal(t, []uint64{3, 4}, rowIDs(t, m))
}

func TestTrimUnpinned(t *testing.T) {
	m := newTestManagerWithDB(t)
	var entries []Entry
	for i := uint64(1); i <= 6; i++ {
		d := []byte{byte(i)}
		entries = append(entries, Entry{ID: i, Data: d, MimeType: "text/plain", Preview: "u", Size: 1, Timestamp: time.Now(), Hash: computeHash(d)})
	}
	for i := uint64(7); i <= 8; i++ {
		d := []byte{byte(i)}
		entries = append(entries, Entry{ID: i, Data: d, MimeType: "text/plain", Preview: "p", Size: 1, Timestamp: time.Now(), Hash: computeHash(d), Pinned: true})
	}
	seedBenchRows(t, m, entries)

	var deleted int
	require.NoError(t, m.dbUpdate(func(tx *bolt.Tx) error {
		var err error
		deleted, err = trimUnpinned(tx.Bucket([]byte("clipboard")), 3)
		return err
	}))
	assert.Equal(t, 3, deleted)
	assert.Equal(t, []uint64{4, 5, 6, 7, 8}, rowIDs(t, m))

	require.NoError(t, m.dbUpdate(func(tx *bolt.Tx) error {
		var err error
		deleted, err = trimUnpinned(tx.Bucket([]byte("clipboard")), -1)
		return err
	}))
	assert.Equal(t, 0, deleted)
	assert.Equal(t, []uint64{4, 5, 6, 7, 8}, rowIDs(t, m))
}

func TestClearOldEntries_KeepsPinned(t *testing.T) {
	m := newTestManagerWithDB(t)
	old := time.Now().AddDate(0, 0, -10)
	seedBenchRows(t, m, []Entry{
		{ID: 1, Data: []byte("old"), MimeType: "text/plain", Preview: "old", Size: 3, Timestamp: old, Hash: computeHash([]byte("old"))},
		{ID: 2, Data: []byte("old pinned"), MimeType: "text/plain", Preview: "old pinned", Size: 10, Timestamp: old, Hash: computeHash([]byte("old pinned")), Pinned: true},
		{ID: 3, Data: []byte("new"), MimeType: "text/plain", Preview: "new", Size: 3, Timestamp: time.Now(), Hash: computeHash([]byte("new"))},
	})

	require.NoError(t, m.clearOldEntries(7))
	assert.Equal(t, []uint64{2, 3}, rowIDs(t, m))
}

func TestTextPreview_RuneBoundary(t *testing.T) {
	m := newTestManagerWithDB(t)

	short := strings.Repeat("é", 60)
	assert.Equal(t, short, m.textPreview([]byte(short)))

	long := strings.Repeat("é", 150)
	out := m.textPreview([]byte(long))
	assert.True(t, utf8.ValidString(out), "must not split multi-byte runes")
	assert.Equal(t, 101, len([]rune(out)))
	assert.True(t, strings.HasSuffix(out, "…"))
}

func TestUriListPreview_Forms(t *testing.T) {
	m := newTestManagerWithDB(t)

	preview, isImage := m.uriListPreview([]byte("file:///a\r\nfile:///b"))
	assert.False(t, isImage)
	assert.Equal(t, "[[ 2 files ]]", preview)

	preview, isImage = m.uriListPreview([]byte("file:///nonexistent-dms-entry"))
	assert.False(t, isImage)
	assert.Equal(t, "file:///nonexistent-dms-entry", preview)
}
