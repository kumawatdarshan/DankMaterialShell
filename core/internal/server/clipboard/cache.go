package clipboard

import (
	"slices"

	bolt "go.etcd.io/bbolt"
)

func insertPinnedDesc(slice []Entry, entry Entry) []Entry {
	for i, e := range slice {
		if e.ID == entry.ID {
			slice[i] = entry
			return slice
		}
		if e.ID < entry.ID {
			return slices.Insert(slice, i, entry)
		}
	}
	return append(slice, entry)
}

func (m *Manager) initCache() {
	if m.db == nil {
		return
	}

	var pinned []Entry
	unpinned := 0

	_ = m.dbView(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("clipboard"))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			h, ok := parseEntryHeader(v)
			if !ok {
				entry, err := decodeEntryMeta(v)
				if err == nil {
					if entry.Pinned {
						pinned = append(pinned, entry)
					} else {
						unpinned++
					}
				}
				continue
			}

			if h.pinned {
				entry, err := decodeEntryMeta(v)
				if err == nil {
					pinned = append(pinned, entry)
				}
			} else {
				unpinned++
			}
		}
		return nil
	})

	m.cacheMutex.Lock()
	m.pinnedCache = pinned
	m.unpinnedCount = unpinned
	m.cacheMutex.Unlock()
}

func (m *Manager) getUnpinnedCount() int {
	m.cacheMutex.RLock()
	defer m.cacheMutex.RUnlock()
	return m.unpinnedCount
}

func (m *Manager) adjustUnpinnedCount(delta int) {
	if delta == 0 {
		return
	}
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()
	m.unpinnedCount += delta
	if m.unpinnedCount < 0 {
		m.unpinnedCount = 0
	}
}

func (m *Manager) GetPinnedEntries() []Entry {
	m.cacheMutex.RLock()
	defer m.cacheMutex.RUnlock()
	res := make([]Entry, len(m.pinnedCache))
	copy(res, m.pinnedCache)
	return res
}

func (m *Manager) GetPinnedCount() int {
	m.cacheMutex.RLock()
	defer m.cacheMutex.RUnlock()
	return len(m.pinnedCache)
}
