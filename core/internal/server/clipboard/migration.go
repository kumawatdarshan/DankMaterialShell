package clipboard

import (
	"github.com/AvengeMedia/dankgo/log"
	bolt "go.etcd.io/bbolt"
)

func (m *Manager) migrateHashes() error {
	if m.db == nil {
		return nil
	}

	var needsMigration bool
	if err := m.dbView(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("clipboard"))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if extractHash(v) == 0 {
				needsMigration = true
				return nil
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if !needsMigration {
		return nil
	}

	log.Info("Migrating clipboard entries to add hashes...")

	return m.dbUpdate(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("clipboard"))
		if b == nil {
			return nil
		}

		var updates []struct {
			key   []byte
			entry Entry
		}

		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			entry, err := decodeEntry(v)
			if err != nil {
				continue
			}
			if entry.Hash != 0 {
				continue
			}
			entry.Hash = computeHash(entry.Data)
			keyCopy := make([]byte, len(k))
			copy(keyCopy, k)
			updates = append(updates, struct {
				key   []byte
				entry Entry
			}{keyCopy, entry})
		}

		for _, u := range updates {
			encoded, err := encodeEntry(u.entry)
			if err != nil {
				continue
			}
			if err := b.Put(u.key, encoded); err != nil {
				return err
			}
		}

		log.Infof("Migrated %d clipboard entries", len(updates))
		return nil
	})
}
