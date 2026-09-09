package clipboard

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func bucketCount(t *testing.T, path string) int {
	t.Helper()
	db, err := bolt.Open(path, 0o644, &bolt.Options{Timeout: time.Second})
	require.NoError(t, err)
	defer db.Close()
	count := 0
	require.NoError(t, db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("clipboard"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, _ []byte) error {
			count++
			return nil
		})
	}))
	return count
}

func TestStoreBatch(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := DefaultStoreConfig()

	var items []BatchItem
	for i := 0; i < 10; i++ {
		items = append(items, BatchItem{Data: []byte(fmt.Sprintf("batch item %d", i)), MimeType: "text/plain"})
	}
	stored, err := StoreBatch(items, cfg)
	require.NoError(t, err)
	assert.Equal(t, 10, stored)

	path, err := GetDBPath()
	require.NoError(t, err)
	assert.Equal(t, 10, bucketCount(t, path))
}

func TestStoreBatch_DedupAndTrim(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := DefaultStoreConfig()
	cfg.MaxHistory = 5

	var items []BatchItem
	for i := 0; i < 7; i++ {
		items = append(items, BatchItem{Data: []byte(fmt.Sprintf("trim item %d", i)), MimeType: "text/plain"})
	}
	items = append(items, BatchItem{Data: []byte("trim item 0"), MimeType: "text/plain"})

	stored, err := StoreBatch(items, cfg)
	require.NoError(t, err)
	assert.Equal(t, 8, stored)

	path, err := GetDBPath()
	require.NoError(t, err)
	assert.Equal(t, 5, bucketCount(t, path))
}

func TestStoreBatch_SkipsEmptyAndOversize(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := DefaultStoreConfig()
	cfg.MaxEntrySize = 8

	stored, err := StoreBatch([]BatchItem{
		{Data: []byte("ok123456"), MimeType: "text/plain"},
		{Data: nil, MimeType: "text/plain"},
		{Data: []byte("way too long for the limit"), MimeType: "text/plain"},
	}, cfg)
	require.NoError(t, err)
	assert.Equal(t, 1, stored)
}

func BenchmarkStoreBatch(b *testing.B) {
	b.StopTimer()
	b.Setenv("XDG_CACHE_HOME", b.TempDir())
	cfg := DefaultStoreConfig()
	var items []BatchItem
	for i := 0; i < 100; i++ {
		items = append(items, BatchItem{Data: []byte(fmt.Sprintf("bench batch item %d with some words", i)), MimeType: "text/plain"})
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		if _, err := StoreBatch(items, cfg); err != nil {
			b.Fatal(err)
		}
	}
}
