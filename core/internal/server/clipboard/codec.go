package clipboard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

// longTextThreshold mirrors the QML clipboard type filter: entries larger
// than this (and not images) are "long_text".
const longTextThreshold = 200

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

func (h entryHeader) entryType() string {
	if h.isImage {
		return "image"
	}
	if h.size > longTextThreshold {
		return "long_text"
	}
	return "text"
}

func (h entryHeader) matches(query, mimeFilter, entryType string, isImage, pinned *bool, before, after *int64) bool {
	if isImage != nil && h.isImage != *isImage {
		return false
	}
	if pinned != nil && h.pinned != *pinned {
		return false
	}
	if entryType != "" && entryType != "all" && h.entryType() != entryType {
		return false
	}
	if before != nil && h.ts >= *before {
		return false
	}
	if after != nil && h.ts <= *after {
		return false
	}
	if mimeFilter != "" && !bytesContainsLower(h.mimeType, mimeFilter) {
		return false
	}
	if query != "" && !bytesContainsLower(h.preview, query) {
		return false
	}
	return true
}

func entryTypeOf(e Entry) string {
	if e.IsImage {
		return "image"
	}
	if e.Size > longTextThreshold {
		return "long_text"
	}
	return "text"
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
	h, ok := parseEntryHeader(data)
	if !ok {
		return decodeEntryFields(data, false)
	}
	e := Entry{
		ID:        h.id,
		MimeType:  string(h.mimeType),
		Preview:   string(h.preview),
		Size:      h.size,
		Timestamp: time.Unix(h.ts, 0),
		IsImage:   h.isImage,
		Hash:      h.hash,
		Pinned:    h.pinned,
	}

	dataLen := int(binary.BigEndian.Uint32(data[8:12]))
	mimeLen := len(h.mimeType)
	prevLen := len(h.preview)
	tailOffset := 12 + dataLen + 4 + mimeLen + 4 + prevLen + 22

	if len(data) >= tailOffset+4 {
		altMimeLen := int(binary.BigEndian.Uint32(data[tailOffset : tailOffset+4]))
		if len(data) >= tailOffset+4+altMimeLen {
			e.AltMimeType = string(data[tailOffset+4 : tailOffset+4+altMimeLen])
		}
	}

	return e, nil
}

func decodeEntryFields(data []byte, withData bool) (Entry, error) {
	buf := bytes.NewReader(data)
	var e Entry

	binary.Read(buf, binary.BigEndian, &e.ID)

	var dataLen uint32
	binary.Read(buf, binary.BigEndian, &dataLen)
	switch {
	case withData:
		e.Data = make([]byte, dataLen)
		buf.Read(e.Data)
	default:
		if _, err := buf.Seek(int64(dataLen), io.SeekCurrent); err != nil {
			return e, err
		}
	}

	var mimeLen uint32
	binary.Read(buf, binary.BigEndian, &mimeLen)
	mimeBytes := make([]byte, mimeLen)
	buf.Read(mimeBytes)
	e.MimeType = string(mimeBytes)

	var prevLen uint32
	binary.Read(buf, binary.BigEndian, &prevLen)
	prevBytes := make([]byte, prevLen)
	buf.Read(prevBytes)
	e.Preview = string(prevBytes)

	var size int32
	binary.Read(buf, binary.BigEndian, &size)
	e.Size = int(size)

	var timestamp int64
	binary.Read(buf, binary.BigEndian, &timestamp)
	e.Timestamp = time.Unix(timestamp, 0)

	var isImage byte
	binary.Read(buf, binary.BigEndian, &isImage)
	e.IsImage = isImage == 1

	if buf.Len() >= 8 {
		binary.Read(buf, binary.BigEndian, &e.Hash)
	}

	if buf.Len() >= 1 {
		var pinnedByte byte
		binary.Read(buf, binary.BigEndian, &pinnedByte)
		e.Pinned = pinnedByte == 1
	}

	if buf.Len() >= 4 {
		var altMimeLen uint32
		binary.Read(buf, binary.BigEndian, &altMimeLen)
		altMimeBytes := make([]byte, altMimeLen)
		buf.Read(altMimeBytes)
		e.AltMimeType = string(altMimeBytes)

		var altDataLen uint32
		binary.Read(buf, binary.BigEndian, &altDataLen)
		if withData {
			e.AltData = make([]byte, altDataLen)
			buf.Read(e.AltData)
		}
	}

	return e, nil
}

func parseEntryHeader(data []byte) (entryHeader, bool) {
	if len(data) < 12 {
		return entryHeader{}, false
	}
	id := binary.BigEndian.Uint64(data[0:8])
	dataLen := int(binary.BigEndian.Uint32(data[8:12]))

	mimeLenOffset := 12 + dataLen
	if len(data) < mimeLenOffset+4 {
		return entryHeader{}, false
	}
	mimeLen := int(binary.BigEndian.Uint32(data[mimeLenOffset : mimeLenOffset+4]))

	prevLenOffset := mimeLenOffset + 4 + mimeLen
	if len(data) < prevLenOffset+4 {
		return entryHeader{}, false
	}
	prevLen := int(binary.BigEndian.Uint32(data[prevLenOffset : prevLenOffset+4]))

	tailOffset := prevLenOffset + 4 + prevLen
	if len(data) < tailOffset+22 {
		return entryHeader{}, false
	}

	return entryHeader{
		id:       id,
		mimeType: data[mimeLenOffset+4 : mimeLenOffset+4+mimeLen],
		preview:  data[prevLenOffset+4 : prevLenOffset+4+prevLen],
		size:     int(int32(binary.BigEndian.Uint32(data[tailOffset : tailOffset+4]))),
		ts:       int64(binary.BigEndian.Uint64(data[tailOffset+4 : tailOffset+12])),
		isImage:  data[tailOffset+12] == 1,
		hash:     binary.BigEndian.Uint64(data[tailOffset+13 : tailOffset+21]),
		pinned:   data[tailOffset+21] == 1,
	}, true
}

func extractPinned(data []byte) bool {
	if h, ok := parseEntryHeader(data); ok {
		return h.pinned
	}
	e, err := decodeEntryMeta(data)
	return err == nil && e.Pinned
}

func extractHash(data []byte) uint64 {
	if h, ok := parseEntryHeader(data); ok {
		return h.hash
	}
	e, err := decodeEntryMeta(data)
	if err != nil {
		return 0
	}
	return e.Hash
}

func bytesContainsLower(src []byte, lowerSubstr string) bool {
	if lowerSubstr == "" {
		return true
	}
	return bytes.Contains(bytes.ToLower(src), []byte(lowerSubstr))
}

func matchesHeaderFast(data []byte, query, mimeFilter, entryType string, isImage, pinned *bool, before, after *int64) bool {
	h, ok := parseEntryHeader(data)
	if !ok {
		e, err := decodeEntryMeta(data)
		if err != nil {
			return false
		}
		return matchesSearch(e, query, mimeFilter, entryType, isImage, pinned, before, after)
	}
	return h.matches(query, mimeFilter, entryType, isImage, pinned, before, after)
}

func matchesSearch(e Entry, query, mimeFilter, entryType string, isImage, pinned *bool, before, after *int64) bool {
	h := entryHeader{
		id:       e.ID,
		mimeType: []byte(e.MimeType),
		preview:  []byte(e.Preview),
		size:     e.Size,
		ts:       e.Timestamp.Unix(),
		isImage:  e.IsImage,
		hash:     e.Hash,
		pinned:   e.Pinned,
	}
	return h.matches(query, mimeFilter, entryType, isImage, pinned, before, after)
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

func textPreview(data []byte) string {
	text := string(data)
	text = strings.TrimSpace(text)
	text = strings.Join(strings.Fields(text), " ")

	if len(text) > 100 {
		return text[:100] + "…"
	}
	return text
}

func imagePreview(data []byte, format string) string {
	config, imgFmt, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Sprintf("[[ image %s %s ]]", sizeStr(len(data)), format)
	}
	return fmt.Sprintf("[[ image %s %s %dx%d ]]", sizeStr(len(data)), imgFmt, config.Width, config.Height)
}

func sizeStr(size int) string {
	units := []string{"B", "KiB", "MiB"}
	var i int
	fsize := float64(size)
	for fsize >= 1024 && i < len(units)-1 {
		fsize /= 1024
		i++
	}
	return fmt.Sprintf("%.0f %s", fsize, units[i])
}
