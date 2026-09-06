package clipboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"

	"github.com/AvengeMedia/DankMaterialShell/core/internal/utils"
)

func TestStoreStandalone(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	err := StoreStandalone([]byte("hello standalone"), "text/plain")
	require.NoError(t, err)

	dbPath, err := utils.ClipboardDBPath()
	require.NoError(t, err)

	db, err := bolt.Open(dbPath, 0o644, nil)
	require.NoError(t, err)
	defer db.Close()

	var entries []Entry
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		require.NotNil(t, b)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			e, err := decodeEntry(v)
			require.NoError(t, err)
			entries = append(entries, e)
		}
		return nil
	})
	require.NoError(t, err)

	require.Len(t, entries, 1)
	assert.Equal(t, "hello standalone", string(entries[0].Data))
	assert.Equal(t, "text/plain", entries[0].MimeType)
	assert.Equal(t, "hello standalone", entries[0].Preview)
	assert.False(t, entries[0].Pinned)
}
