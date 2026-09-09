package clipboard

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

const benchEntryCount = 2000

var benchWords = []string{
	"terminal", "browser", "editor", "settings", "network", "system",
	"music", "video", "image", "document", "calendar", "notes",
	"firefox", "code", "mail", "chat", "files", "maps",
	"alpha", "bravo", "charlie", "delta", "echo", "foxtrot",
}

func benchPreview(i int) string {
	w1 := benchWords[i%len(benchWords)]
	w2 := benchWords[(i*7+3)%len(benchWords)]
	w3 := benchWords[(i*13+5)%len(benchWords)]
	if isImage := i%50 == 0; isImage {
		return fmt.Sprintf("[[ image %d B png 800x600 ]] screenshot %s %d", 1024+i, w1, i)
	}
	if i%37 == 0 {
		return fmt.Sprintf("Café résumé %s %s number %d naïve façade", w1, w2, i)
	}
	if i%19 == 0 {
		return fmt.Sprintf("terminal session log %s %s entry %d output", w1, w2, i)
	}
	return fmt.Sprintf("%s %s %s clipboard entry number %d with extra words for preview length", w1, w2, w3, i)
}

func benchEntry(i int) Entry {
	preview := benchPreview(i)
	isImage := i%50 == 0
	mime := "text/plain;charset=utf-8"
	if isImage {
		mime = "image/png"
	}
	data := []byte(fmt.Sprintf("bench-data-%08d %s", i, preview))
	return Entry{
		ID:        uint64(i),
		Data:      data,
		MimeType:  mime,
		Preview:   preview,
		Size:      len(data),
		Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
		IsImage:   isImage,
		Hash:      computeHash(data),
		Pinned:    i%20 == 0,
	}
}

func newBenchManager(b *testing.B, n int) *Manager {
	b.Helper()
	db, err := openDB(filepath.Join(b.TempDir(), "bench-clipboard.db"))
	if err != nil {
		b.Fatalf("openDB: %v", err)
	}
	b.Cleanup(func() { db.Close() })
	m := &Manager{config: DefaultConfig(), db: db}
	m.config.MaxHistory = n * 10
	if err := db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte("clipboard"))
		if bk == nil {
			return fmt.Errorf("clipboard bucket missing")
		}
		for i := 1; i <= n; i++ {
			enc, err := encodeEntry(benchEntry(i))
			if err != nil {
				return err
			}
			if err := bk.Put(itob(uint64(i)), enc); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		b.Fatalf("seed: %v", err)
	}
	return m
}

func BenchmarkSearch_UnfilteredFirstPage(b *testing.B) {
	m := newBenchManager(b, benchEntryCount)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := m.Search(SearchParams{Limit: 50})
		if len(res.Entries) == 0 {
			b.Fatal("expected entries")
		}
	}
}

func BenchmarkSearch_QueryMatch(b *testing.B) {
	m := newBenchManager(b, benchEntryCount)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := m.Search(SearchParams{Query: "terminal", Limit: 50})
		if len(res.Entries) == 0 {
			b.Fatal("expected matches")
		}
	}
}

func BenchmarkSearch_QueryNoMatch(b *testing.B) {
	m := newBenchManager(b, benchEntryCount)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := m.Search(SearchParams{Query: "zzzqxj", Limit: 50})
		if len(res.Entries) != 0 {
			b.Fatal("expected no matches")
		}
	}
}

func BenchmarkSearch_OffsetDeep(b *testing.B) {
	m := newBenchManager(b, benchEntryCount)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := m.Search(SearchParams{Limit: 50, Offset: 1500})
		if len(res.Entries) == 0 {
			b.Fatal("expected entries")
		}
	}
}

func BenchmarkSearch_MimeImage(b *testing.B) {
	m := newBenchManager(b, benchEntryCount)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := m.Search(SearchParams{MimeType: "image", Limit: 50})
		if len(res.Entries) == 0 {
			b.Fatal("expected matches")
		}
	}
}

func BenchmarkStoreEntry_Into1k(b *testing.B) {
	m := newBenchManager(b, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := benchEntry(1000000 + i)
		e.ID = 0
		if err := m.storeEntry(e); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeEntryMeta(b *testing.B) {
	enc, err := encodeEntry(benchEntry(1))
	if err != nil {
		b.Fatalf("encode: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := decodeEntryMeta(enc); err != nil {
			b.Fatal(err)
		}
	}
}
