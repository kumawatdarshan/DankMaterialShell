package clipboard

import (
	"fmt"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/AvengeMedia/DankMaterialShell/core/internal/utils"
)

// StoreStandalone stores clipboard data directly to the bbolt DB without
// requiring a running server instance. Used by CLI commands (import, migrate,
// watch --store).
func StoreStandalone(data []byte, mimeType string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("clipboard db panic: %v", r)
		}
	}()

	if len(data) == 0 {
		return nil
	}

	cfg := LoadConfig()
	if int64(len(data)) > cfg.MaxEntrySize {
		return fmt.Errorf("data too large: %d > %d", len(data), cfg.MaxEntrySize)
	}

	dbPath, err := utils.ClipboardDBPath()
	if err != nil {
		return fmt.Errorf("get db path: %w", err)
	}

	db, err := bolt.Open(dbPath, 0o644, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	isImage := strings.HasPrefix(mimeType, "image/")
	entry := Entry{
		Data:      data,
		MimeType:  mimeType,
		Size:      len(data),
		Timestamp: time.Now(),
		IsImage:   isImage,
		Hash:      computeHash(data),
	}

	if isImage {
		entry.Preview = imagePreview(data, mimeType)
	} else {
		entry.Preview = textPreview(data)
	}

	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return err
		}

		if err := deduplicateStandalone(b, entry.Hash); err != nil {
			return err
		}

		id, err := b.NextSequence()
		if err != nil {
			return err
		}
		entry.ID = id

		encoded, err := encodeEntry(entry)
		if err != nil {
			return err
		}

		if err := b.Put(itob(id), encoded); err != nil {
			return err
		}

		return trimLengthStandalone(b, cfg.MaxHistory)
	})
}

func deduplicateStandalone(b *bolt.Bucket, hash uint64) error {
	c := b.Cursor()
	for k, v := c.Last(); k != nil; k, v = c.Prev() {
		if extractHash(v) != hash {
			continue
		}
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return nil
}

func trimLengthStandalone(b *bolt.Bucket, maxHistory int) error {
	c := b.Cursor()
	var count int
	for k, v := c.Last(); k != nil; k, v = c.Prev() {
		if extractPinned(v) {
			continue
		}
		if count < maxHistory {
			count++
			continue
		}
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return nil
}
