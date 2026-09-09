package clipboard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"time"

	bolt "go.etcd.io/bbolt"
)

// entryHeader exposes the leading fields of an encoded row without decoding
// the payload. mimeType and preview alias the input slice: no allocation.
type entryHeader struct {
	id       uint64
	mimeType []byte
	preview  []byte
	size     int
	ts       int64
	isImage  bool
	hash     uint64
	pinned   bool
}

type cursor struct {
	data []byte
	off  int
	err  bool
}

func (c *cursor) take(n int) ([]byte, bool) {
	if c.err || n < 0 || len(c.data)-c.off < n {
		c.err = true
		return nil, false
	}
	b := c.data[c.off : c.off+n]
	c.off += n
	return b, true
}

func (c *cursor) u64() (uint64, bool) {
	b, ok := c.take(8)
	if !ok {
		return 0, false
	}
	return binary.BigEndian.Uint64(b), true
}

func (c *cursor) u32() (uint32, bool) {
	b, ok := c.take(4)
	if !ok {
		return 0, false
	}
	return binary.BigEndian.Uint32(b), true
}

func parseEntryHeader(data []byte) (entryHeader, bool) {
	var h entryHeader
	c := &cursor{data: data}

	var ok bool
	if h.id, ok = c.u64(); !ok {
		return h, false
	}
	var dataLen uint32
	if dataLen, ok = c.u32(); !ok {
		return h, false
	}
	if _, ok = c.take(int(dataLen)); !ok {
		return h, false
	}
	var mimeLen uint32
	if mimeLen, ok = c.u32(); !ok {
		return h, false
	}
	if h.mimeType, ok = c.take(int(mimeLen)); !ok {
		return h, false
	}
	var prevLen uint32
	if prevLen, ok = c.u32(); !ok {
		return h, false
	}
	if h.preview, ok = c.take(int(prevLen)); !ok {
		return h, false
	}
	var size uint32
	if size, ok = c.u32(); !ok {
		return h, false
	}
	h.size = int(size)
	if h.ts, ok = c.i64(); !ok {
		return h, false
	}
	var img byte
	if img, ok = c.u8(); !ok {
		return h, false
	}
	h.isImage = img == 1
	if h.hash, ok = c.u64(); !ok {
		return h, false
	}
	var pinned byte
	if pinned, ok = c.u8(); !ok {
		return h, false
	}
	h.pinned = pinned == 1
	return h, true
}

func (c *cursor) i64() (int64, bool) {
	b, ok := c.take(8)
	if !ok {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(b)), true
}

func (c *cursor) u8() (byte, bool) {
	b, ok := c.take(1)
	if !ok {
		return 0, false
	}
	return b[0], true
}

func encodeEntry(e Entry) ([]byte, error) {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, e.ID)
	binary.Write(buf, binary.BigEndian, uint32(len(e.Data)))
	buf.Write(e.Data)
	binary.Write(buf, binary.BigEndian, uint32(len(e.MimeType)))
	buf.WriteString(e.MimeType)
	binary.Write(buf, binary.BigEndian, uint32(len(e.Preview)))
	buf.WriteString(e.Preview)
	binary.Write(buf, binary.BigEndian, int32(e.Size))
	binary.Write(buf, binary.BigEndian, e.Timestamp.Unix())
	if e.IsImage {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	binary.Write(buf, binary.BigEndian, e.Hash)
	if e.Pinned {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	if e.AltMimeType != "" {
		binary.Write(buf, binary.BigEndian, uint32(len(e.AltMimeType)))
		buf.WriteString(e.AltMimeType)
		binary.Write(buf, binary.BigEndian, uint32(len(e.AltData)))
		buf.Write(e.AltData)
	}

	return buf.Bytes(), nil
}

func decodeEntry(data []byte) (Entry, error) {
	return decodeEntryFields(data, true)
}

func decodeEntryMeta(data []byte) (Entry, error) {
	return decodeEntryFields(data, false)
}

func decodeEntryFields(data []byte, withData bool) (Entry, error) {
	var e Entry
	c := &cursor{data: data}

	var ok bool
	if e.ID, ok = c.u64(); !ok {
		return e, fmt.Errorf("short entry id")
	}
	var dataLen uint32
	if dataLen, ok = c.u32(); !ok {
		return e, fmt.Errorf("short entry data length")
	}
	if withData {
		var raw []byte
		if raw, ok = c.take(int(dataLen)); !ok {
			return e, fmt.Errorf("short entry data")
		}
		e.Data = append([]byte(nil), raw...)
	} else if _, ok = c.take(int(dataLen)); !ok {
		return e, fmt.Errorf("short entry data")
	}

	var mimeLen uint32
	if mimeLen, ok = c.u32(); !ok {
		return e, fmt.Errorf("short entry mime length")
	}
	var mimeRaw []byte
	if mimeRaw, ok = c.take(int(mimeLen)); !ok {
		return e, fmt.Errorf("short entry mime")
	}
	e.MimeType = string(mimeRaw)

	var prevLen uint32
	if prevLen, ok = c.u32(); !ok {
		return e, fmt.Errorf("short entry preview length")
	}
	var prevRaw []byte
	if prevRaw, ok = c.take(int(prevLen)); !ok {
		return e, fmt.Errorf("short entry preview")
	}
	e.Preview = string(prevRaw)

	var size uint32
	if size, ok = c.u32(); !ok {
		return e, fmt.Errorf("short entry size")
	}
	e.Size = int(int32(size))

	var timestamp int64
	if timestamp, ok = c.i64(); !ok {
		return e, fmt.Errorf("short entry timestamp")
	}
	e.Timestamp = time.Unix(timestamp, 0)

	var isImage byte
	if isImage, ok = c.u8(); !ok {
		return e, fmt.Errorf("short entry image flag")
	}
	e.IsImage = isImage == 1

	if len(c.data)-c.off == 0 {
		return e, nil
	}
	if e.Hash, ok = c.u64(); !ok {
		return e, fmt.Errorf("short entry hash")
	}
	if len(c.data)-c.off == 0 {
		return e, nil
	}
	var pinnedByte byte
	if pinnedByte, ok = c.u8(); !ok {
		return e, fmt.Errorf("short entry pinned flag")
	}
	e.Pinned = pinnedByte == 1

	if len(c.data)-c.off == 0 {
		return e, nil
	}
	var altMimeLen uint32
	if altMimeLen, ok = c.u32(); !ok {
		return e, fmt.Errorf("short entry alt mime length")
	}
	var altMimeRaw []byte
	if altMimeRaw, ok = c.take(int(altMimeLen)); !ok {
		return e, fmt.Errorf("short entry alt mime")
	}
	e.AltMimeType = string(altMimeRaw)

	var altDataLen uint32
	if altDataLen, ok = c.u32(); !ok {
		return e, fmt.Errorf("short entry alt data length")
	}
	var altRaw []byte
	if altRaw, ok = c.take(int(altDataLen)); !ok {
		return e, fmt.Errorf("short entry alt data")
	}
	e.AltData = append([]byte(nil), altRaw...)

	return e, nil
}

func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func computeHash(data []byte) uint64 {
	h := fnv.New64a()
	h.Write(data)
	return h.Sum64()
}

func extractHash(data []byte) uint64 {
	h, ok := parseEntryHeader(data)
	if !ok {
		return 0
	}
	return h.hash
}

// dedupByHash deletes unpinned rows carrying hash, skipping pinned rows.
// Keys are collected first and deleted after the cursor walk.
func dedupByHash(b *bolt.Bucket, hash uint64) (int, error) {
	var keys [][]byte
	c := b.Cursor()
	for k, v := c.Last(); k != nil; k, v = c.Prev() {
		if h, ok := parseEntryHeader(v); ok {
			if h.hash != hash || h.pinned {
				continue
			}
		} else if e, err := decodeEntryMeta(v); err != nil || e.Pinned || e.Hash != hash {
			continue
		}
		keys = append(keys, append([]byte(nil), k...))
	}
	deleted := 0
	for _, k := range keys {
		if err := b.Delete(k); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// trimUnpinned keeps the newest maxHistory unpinned rows and deletes older
// ones. Pinned rows are never touched. maxHistory < 0 disables trimming.
func trimUnpinned(b *bolt.Bucket, maxHistory int) (int, error) {
	if maxHistory < 0 {
		return 0, nil
	}
	var keys [][]byte
	count := 0
	c := b.Cursor()
	for k, v := c.Last(); k != nil; k, v = c.Prev() {
		pinned := false
		if h, ok := parseEntryHeader(v); ok {
			pinned = h.pinned
		} else if e, err := decodeEntryMeta(v); err == nil {
			pinned = e.Pinned
		}
		if pinned {
			continue
		}
		if count < maxHistory {
			count++
			continue
		}
		keys = append(keys, append([]byte(nil), k...))
	}
	deleted := 0
	for _, k := range keys {
		if err := b.Delete(k); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}
