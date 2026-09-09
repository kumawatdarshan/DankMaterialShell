package clipboard

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func dbUnpinnedCount(t *testing.T, m *Manager) int {
	t.Helper()
	count := 0
	require.NoError(t, m.dbView(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("clipboard")).Cursor()
		for _, v := c.First(); v != nil; _, v = c.Next() {
			pinned := false
			if h, ok := parseEntryHeader(v); ok {
				pinned = h.pinned
			} else if e, err := decodeEntryMeta(v); err == nil {
				pinned = e.Pinned
			}
			if !pinned {
				count++
			}
		}
		return nil
	}))
	return count
}

func TestWithMutation_AdjustsCountAndNotifies(t *testing.T) {
	m := newTestManagerWithDB(t)
	seedBenchRows(t, m, []Entry{
		{ID: 1, Data: []byte("a"), MimeType: "text/plain", Preview: "a", Size: 1, Timestamp: time.Now(), Hash: computeHash([]byte("a"))},
		{ID: 2, Data: []byte("b"), MimeType: "text/plain", Preview: "b", Size: 1, Timestamp: time.Now(), Hash: computeHash([]byte("b"))},
		{ID: 3, Data: []byte("c"), MimeType: "text/plain", Preview: "c", Size: 1, Timestamp: time.Now(), Hash: computeHash([]byte("c"))},
	})
	m.initCache()
	require.Equal(t, int64(3), m.unpinnedCount.Load())

	require.NoError(t, m.DeleteEntry(2))
	assert.Equal(t, int64(2), m.unpinnedCount.Load())
	assert.Len(t, m.GetState().History, 2)

	require.NoError(t, m.DeleteEntry(999))
	assert.Equal(t, int64(2), m.unpinnedCount.Load())
}

func TestPinEntry_SingleTxLimit(t *testing.T) {
	m := newTestManagerWithDB(t)
	m.config.MaxPinned = 1
	seedBenchRows(t, m, []Entry{
		{ID: 1, Data: []byte("a"), MimeType: "text/plain", Preview: "a", Size: 1, Timestamp: time.Now(), Hash: computeHash([]byte("a"))},
		{ID: 2, Data: []byte("b"), MimeType: "text/plain", Preview: "b", Size: 1, Timestamp: time.Now(), Hash: computeHash([]byte("b"))},
	})
	m.initCache()

	require.NoError(t, m.PinEntry(1))
	assert.Equal(t, int64(1), m.unpinnedCount.Load())
	assert.Equal(t, 1, m.GetPinnedCount())

	require.ErrorContains(t, m.PinEntry(2), "maximum pinned entries")
	assert.Equal(t, int64(1), m.unpinnedCount.Load())

	require.NoError(t, m.PinEntry(1))
	assert.Equal(t, int64(1), m.unpinnedCount.Load())
	assert.Equal(t, 1, m.GetPinnedCount())

	require.Error(t, m.PinEntry(999))
}

func TestUnpinnedCount_DriftFuzz(t *testing.T) {
	m := newTestManagerWithDB(t)

	var entries []Entry
	for i := uint64(1); i <= 200; i++ {
		d := []byte(fmt.Sprintf("fuzz-seed-%d", i))
		entries = append(entries, Entry{ID: i, Data: d, MimeType: "text/plain", Preview: string(d), Size: len(d), Timestamp: time.Now(), Hash: computeHash(d)})
	}
	for _, id := range []uint64{1001, 1002} {
		d := []byte(fmt.Sprintf("fuzz-immune-%d", id))
		entries = append(entries, Entry{ID: id, Data: d, MimeType: "text/plain", Preview: string(d), Size: len(d), Timestamp: time.Now(), Hash: computeHash(d), Pinned: true})
	}
	seedBenchRows(t, m, entries)
	m.initCache()

	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < 500; i++ {
		id := uint64(rnd.Intn(1200) + 1)
		if id == 1001 || id == 1002 {
			continue
		}
		switch rnd.Intn(6) {
		case 0:
			_ = m.StoreData([]byte(fmt.Sprintf("fuzz-%d-%d", i, rnd.Intn(50))), "text/plain")
		case 1:
			_ = m.DeleteEntry(id)
		case 2:
			_, _ = m.DeleteEntries([]uint64{id, id + 1, id + 2})
		case 3:
			_ = m.PinEntry(id)
		case 4:
			_ = m.UnpinEntry(id)
		case 5:
			m.ClearHistory()
		}
		if i%50 == 0 {
			assert.Equal(t, dbUnpinnedCount(t, m), int(m.unpinnedCount.Load()), "drift at op %d", i)
		}
	}
	assert.Equal(t, dbUnpinnedCount(t, m), int(m.unpinnedCount.Load()))
}

func TestUpdateState_CapsBroadcast(t *testing.T) {
	m := newTestManagerWithDB(t)
	var entries []Entry
	for i := uint64(1); i <= 60; i++ {
		d := []byte(fmt.Sprintf("cap-%d", i))
		entries = append(entries, Entry{ID: i, Data: d, MimeType: "text/plain", Preview: string(d), Size: len(d), Timestamp: time.Now(), Hash: computeHash(d)})
	}
	seedBenchRows(t, m, entries)
	m.initCache()

	m.updateState()
	assert.Len(t, m.GetState().History, stateHeadLimit)
	assert.Len(t, m.GetHistory(), 60)
}
