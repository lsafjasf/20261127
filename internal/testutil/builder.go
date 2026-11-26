// Package testutil 生成合成 MP4 文件,用于在无外部依赖下
// 构造重封装、元数据修改、帧替换等测试场景。
package testutil

import (
	"encoding/binary"
)

// TrackSpec 描述一条合成媒体轨。
type TrackSpec struct {
	Handler   string // "vide" / "soun"
	Codec     string // "avc1" / "mp4a"
	TrackID   uint32
	Timescale uint32
	Delta     uint32 // 每样本时长
	Samples   [][]byte
	ChunkSize int // 每 chunk 样本数
}

// Options 控制容器布局与元数据。
type Options struct {
	MoovFirst bool   // moov 置于 mdat 前(faststart)
	AddFree   bool   // 插入 free 填充盒
	ModTime   uint32 // mvhd/tkhd/mdhd 时间戳
	Udta      []byte // 非空时写入 udta 元数据盒
}

func box(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out, uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

func fullbox(typ string, version byte, flags uint32, payload []byte) []byte {
	head := []byte{version, byte(flags >> 16), byte(flags >> 8), byte(flags)}
	return box(typ, append(head, payload...))
}

func u32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// chunkLayout 计算每轨 chunk 划分;返回各 chunk 内样本。
func chunkLayout(t TrackSpec) [][]int {
	spc := t.ChunkSize
	if spc <= 0 {
		spc = 1
	}
	var chunks [][]int
	for i := 0; i < len(t.Samples); i += spc {
		end := i + spc
		if end > len(t.Samples) {
			end = len(t.Samples)
		}
		var idxs []int
		for j := i; j < end; j++ {
			idxs = append(idxs, j)
		}
		chunks = append(chunks, idxs)
	}
	return chunks
}

// Build 生成一个合法 MP4 文件字节流。
func Build(tracks []TrackSpec, opts Options) []byte {
	ftyp := box("ftyp", cat([]byte("isom"), u32(0x200), []byte("isomiso2")))
	free := []byte(nil)
	if opts.AddFree {
		free = box("free", make([]byte, 32))
	}

	// 两遍构建:先以等长零 stco 占位得到 moov 尺寸,再回填真实偏移。
	zeroStcos := make([][]int64, len(tracks))
	for i, t := range tracks {
		zeroStcos[i] = make([]int64, len(chunkLayout(t)))
	}
	moov := buildMoov(tracks, opts, zeroStcos)
	mdatHeader := 8
	mdatBase := int64(len(ftyp) + len(free) + mdatHeader)
	if opts.MoovFirst {
		mdatBase += int64(len(moov))
	}

	// 逐轨顺序排布 chunk,计算绝对偏移。
	stcos := make([][]int64, len(tracks))
	off := mdatBase
	var mdatPayload []byte
	for ti, t := range tracks {
		for _, chunk := range chunkLayout(t) {
			stcos[ti] = append(stcos[ti], off)
			for _, si := range chunk {
				mdatPayload = append(mdatPayload, t.Samples[si]...)
				off += int64(len(t.Samples[si]))
			}
		}
	}
	moov = buildMoov(tracks, opts, stcos)
	mdat := box("mdat", mdatPayload)

	if opts.MoovFirst {
		return cat(ftyp, free, moov, mdat)
	}
	return cat(ftyp, free, mdat, moov)
}
