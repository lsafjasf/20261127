package fingerprint

import (
	"crypto/sha256"
	"io"

	"mediafp/internal/mp4"
)

// 结构指纹遍历模式。
const (
	modeStructure = iota // 全量:所有盒 + 叶子内容
	modeLayout           // 剔除元数据子树与填充盒,时间戳字段归零
)

// metadataSubtree 内的盒整体视为元数据,不参与布局指纹。
var metadataSubtree = map[string]bool{
	"udta": true, "meta": true, "keys": true, "ilst": true,
}

// paddingBoxes 为填充盒,布局指纹忽略(重封装常增删填充)。
var paddingBoxes = map[string]bool{
	"free": true, "skip": true, "wide": true,
}

// timedBoxes 的创建/修改时间戳属元数据,布局指纹中归零。
var timedBoxes = map[string]bool{
	"mvhd": true, "tkhd": true, "mdhd": true,
}

// structureHash 按文件序遍历盒树计算结构/布局指纹。
func structureHash(r io.ReaderAt, boxes []*mp4.Box, mode int) string {
	domain := "MEDIAFP/STRUCTURE/v1"
	if mode == modeLayout {
		domain = "MEDIAFP/LAYOUT/v1"
	}
	c := newCanon(domain)
	walkBoxes(c, r, boxes, 0, mode)
	return hexSum(c)
}

func walkBoxes(c *canon, r io.ReaderAt, boxes []*mp4.Box, depth, mode int) {
	for _, b := range boxes {
		if mode == modeLayout && (metadataSubtree[b.Type] || paddingBoxes[b.Type]) {
			continue
		}
		c.u32(uint32(depth))
		c.str(b.Type)
		if len(b.Children) > 0 {
			if mode == modeStructure {
				// 结构指纹记录容器尺寸;布局指纹不记,
				// 避免元数据子树撑大容器尺寸而污染布局。
				c.u64(uint64(b.HeaderSize))
				c.u64(uint64(b.DataSize()))
			}
			walkBoxes(c, r, b.Children, depth+1, mode)
			continue
		}
		c.u64(uint64(b.HeaderSize))
		c.u64(uint64(b.DataSize()))
		if b.Type == "mdat" {
			c.str("MDAT-SKIP") // 载荷由载荷指纹覆盖,结构只记尺寸
			continue
		}
		data, err := mp4.ReadData(r, b)
		if err != nil {
			c.str("UNREADABLE")
			continue
		}
		if mode == modeLayout && timedBoxes[b.Type] {
			data = zeroTimestamps(b.Type, data)
		}
		sum := sha256.Sum256(data)
		c.raw(sum[:])
	}
}

// zeroTimestamps 将 mvhd/tkhd/mdhd 的创建与修改时间字段归零。
// v0: 偏移 4..12(各 4 字节);v1: 偏移 4..20(各 8 字节)。
func zeroTimestamps(_ string, data []byte) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	if len(data) < 12 {
		return out
	}
	if data[0] == 1 && len(data) >= 20 {
		for i := 4; i < 20; i++ {
			out[i] = 0
		}
		return out
	}
	for i := 4; i < 12; i++ {
		out[i] = 0
	}
	return out
}

// metadataHash 仅覆盖元数据成分:udta/meta 子树内容、mvhd/tkhd/mdhd
// 时间戳、hdlr 名称字段。
func metadataHash(r io.ReaderAt, boxes []*mp4.Box) string {
	c := newCanon("MEDIAFP/METADATA/v1")
	walkMetadata(c, r, boxes, "")
	return hexSum(c)
}

func walkMetadata(c *canon, r io.ReaderAt, boxes []*mp4.Box, parent string) {
	for _, b := range boxes {
		switch {
		case metadataSubtree[b.Type]:
			hashSubtree(c, r, b)
		case timedBoxes[b.Type]:
			data, err := mp4.ReadData(r, b)
			if err != nil || len(data) < 12 {
				continue
			}
			c.str(b.Type)
			if data[0] == 1 && len(data) >= 20 {
				c.raw(data[4:20])
			} else {
				c.raw(data[4:12])
			}
		case b.Type == "hdlr":
			data, err := mp4.ReadData(r, b)
			if err == nil && len(data) > 24 {
				c.str("hdlr-name")
				c.raw(data[24:]) // 名称字段
			}
		}
		if len(b.Children) > 0 && !metadataSubtree[b.Type] {
			walkMetadata(c, r, b.Children, b.Type)
		}
	}
}

// hashSubtree 对整棵子树(含盒结构与子盒内容)做哈希。
func hashSubtree(c *canon, r io.ReaderAt, b *mp4.Box) {
	c.str(b.Type)
	c.u64(uint64(b.DataSize()))
	if len(b.Children) > 0 {
		for _, ch := range b.Children {
			hashSubtree(c, r, ch)
		}
		return
	}
	data, err := mp4.ReadData(r, b)
	if err != nil {
		return
	}
	sum := sha256.Sum256(data)
	c.raw(sum[:])
}

func hexSum(c *canon) string {
	sum := c.sum()
	const hexdig = "0123456789abcdef"
	out := make([]byte, 64)
	for i, v := range sum {
		out[i*2] = hexdig[v>>4]
		out[i*2+1] = hexdig[v&0xf]
	}
	return string(out)
}
