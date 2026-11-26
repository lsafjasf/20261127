// Package mp4 提供零依赖的 ISO BMFF(MP4/MOV)盒结构解析。
// 仅解析盒头与样本表,不解码音视频,用于计算容器无关的载荷指纹。
package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Box 表示一个 MP4 盒。Data 偏移指向盒内容(不含盒头)。
type Box struct {
	Type       string // 4 字符盒类型
	Offset     int64  // 盒头在文件中的偏移
	HeaderSize int64  // 盒头长度(8 或 16,uuid 另加 16)
	Size       int64  // 盒总长度(含盒头)
	Children   []*Box // 容器盒的子盒
}

// DataOffset 返回盒内容起始偏移。
func (b *Box) DataOffset() int64 { return b.Offset + b.HeaderSize }

// DataSize 返回盒内容长度。
func (b *Box) DataSize() int64 { return b.Size - b.HeaderSize }

// containerBoxes 为需要递归解析子盒的容器类型。
var containerBoxes = map[string]bool{
	"moov": true, "trak": true, "mdia": true, "minf": true,
	"stbl": true, "edts": true, "dinf": true, "udta": true,
	"moof": true, "traf": true, "mfra": true, "skip": true,
	"ilst": true,
}

// ParseFile 解析整个文件的顶层盒树。
func ParseFile(r io.ReaderAt, fileSize int64) ([]*Box, error) {
	return parseBoxes(r, 0, fileSize, 0)
}

func parseBoxes(r io.ReaderAt, start, end int64, depth int) ([]*Box, error) {
	if depth > 32 {
		return nil, fmt.Errorf("mp4: 盒嵌套过深")
	}
	var boxes []*Box
	pos := start
	for pos < end {
		box, err := parseBox(r, pos, end)
		if err != nil {
			return nil, err
		}
		boxes = append(boxes, box)
		pos += box.Size
	}
	if pos != end {
		return nil, fmt.Errorf("mp4: 盒边界不对齐 @%d", pos)
	}
	return boxes, nil
}

func parseBox(r io.ReaderAt, offset, limit int64) (*Box, error) {
	var hdr [16]byte
	if _, err := r.ReadAt(hdr[:8], offset); err != nil {
		return nil, fmt.Errorf("mp4: 读盒头失败 @%d: %w", offset, err)
	}
	size := int64(binary.BigEndian.Uint32(hdr[0:4]))
	typ := string(hdr[4:8])
	header := int64(8)
	if size == 1 {
		if _, err := r.ReadAt(hdr[8:16], offset+8); err != nil {
			return nil, fmt.Errorf("mp4: 读扩展长度失败 @%d: %w", offset, err)
		}
		size = int64(binary.BigEndian.Uint64(hdr[8:16]))
		header = 16
	} else if size == 0 {
		size = limit - offset // 延伸至父盒/文件末尾
	}
	if typ == "uuid" {
		header += 16
	}
	if size < header || offset+size > limit {
		return nil, fmt.Errorf("mp4: 非法盒 %q 大小 %d @%d", typ, size, offset)
	}
	box := &Box{Type: typ, Offset: offset, HeaderSize: header, Size: size}

	childStart := offset + header
	if typ == "meta" {
		childStart += 4 // meta 是 FullBox,跳过 version/flags
	}
	if containerBoxes[typ] && box.DataSize() > 0 {
		children, err := parseBoxes(r, childStart, offset+size, 1)
		if err != nil {
			// 非标准内容(如 udta 内嵌私有数据)时退化为叶子盒
			return box, nil
		}
		box.Children = children
	}
	return box, nil
}

// Find 返回首个匹配类型的直接子盒。
func (b *Box) Find(typ string) *Box {
	for _, c := range b.Children {
		if c.Type == typ {
			return c
		}
	}
	return nil
}

// FindAll 返回所有匹配类型的直接子盒。
func (b *Box) FindAll(typ string) []*Box {
	var out []*Box
	for _, c := range b.Children {
		if c.Type == typ {
			out = append(out, c)
		}
	}
	return out
}

// ReadData 读取盒内容。
func ReadData(r io.ReaderAt, b *Box) ([]byte, error) {
	buf := make([]byte, b.DataSize())
	_, err := r.ReadAt(buf, b.DataOffset())
	return buf, err
}
