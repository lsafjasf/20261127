// Package fingerprint 定义并计算媒体文件的四级指纹:
//
//	载荷指纹 Payload   —— 容器无关:按 (轨,DTS) 规范排序后,对每轨编解码标识
//	                     与逐样本 SHA-256 做规范序列化的哈希。重封装、改元数据、
//	                     调盒顺序、重排 chunk 均不改变它;替换任何一帧会改变。
//	结构指纹 Structure —— 盒树全量骨架:按文件序遍历盒(类型/头长/数据长),
//	                     叶子盒(mdat 除外)附内容哈希。任何容器层改动都会改变。
//	布局指纹 Layout    —— 结构指纹剔除元数据子树、填充盒,并将 mvhd/tkhd/mdhd
//	                     的时间戳字段归零。仅重封装(盒序/chunk 布局/stco)会改变。
//	元数据指纹 Metadata —— 仅覆盖元数据部分:udta/meta 子树内容、mvhd/tkhd/mdhd
//	                     时间戳、hdlr 名称。用于区分"元数据修改"与"重封装"。
package fingerprint

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"os"
	"sort"

	"mediafp/internal/mp4"
)

// SampleDigest 是单样本的载荷摘要。
type SampleDigest struct {
	DTS      uint64
	Duration uint32
	Size     int64
	SHA256   [32]byte
}

// TrackDigest 是一轨的载荷摘要与轨道指纹。
type TrackDigest struct {
	TrackID   uint32
	Handler   string
	Codec     string
	Timescale uint32
	Samples   []SampleDigest
	Hash      string // 该轨载荷指纹
}

// Fingerprints 是一个文件的四级指纹集合。
type Fingerprints struct {
	FileSize  int64  `json:"file_size"`
	WholeFile string `json:"whole_file_sha256"` // 整文件哈希(对照:重封装后必然变化)
	Payload   string `json:"payload_fp"`
	Structure string `json:"structure_fp"`
	Layout    string `json:"layout_fp"`
	Metadata  string `json:"metadata_fp"`
}

// Media 是解析后的文件指纹与载荷明细,用于比对。
type Media struct {
	Path   string
	FP     Fingerprints
	Tracks []TrackDigest
}

// Compute 解析文件并计算全部指纹。
func Compute(path string) (*Media, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	m := &Media{Path: path}
	m.FP.FileSize = st.Size()
	m.FP.WholeFile, err = hashWholeFile(f)
	if err != nil {
		return nil, err
	}

	boxes, err := mp4.ParseFile(f, st.Size())
	if err != nil {
		return nil, fmt.Errorf("解析容器失败: %w", err)
	}
	var moov *mp4.Box
	for _, b := range boxes {
		if b.Type == "moov" {
			moov = b
			break
		}
	}
	if moov == nil {
		return nil, fmt.Errorf("非 MP4/MOV 文件或缺少 moov 盒")
	}
	tracks, err := mp4.ExtractTracks(f, moov)
	if err != nil {
		return nil, err
	}

	payload := newCanon("MEDIAFP/PAYLOAD/v1")
	for _, t := range tracks {
		td, err := digestTrack(f, t)
		if err != nil {
			return nil, err
		}
		m.Tracks = append(m.Tracks, td)
	}
	sort.Slice(m.Tracks, func(i, j int) bool { return m.Tracks[i].TrackID < m.Tracks[j].TrackID })
	for i := range m.Tracks {
		td := &m.Tracks[i]
		payload.str(td.Handler)
		payload.str(td.Codec)
		payload.u32(td.TrackID)
		payload.u32(td.Timescale)
		payload.u64(uint64(len(td.Samples)))
		track := newCanon("MEDIAFP/TRACK/v1")
		for _, s := range td.Samples {
			track.u64(s.DTS)
			track.u32(s.Duration)
			track.u64(uint64(s.Size))
			track.raw(s.SHA256[:])
		}
		sum := track.sum()
		td.Hash = fmt.Sprintf("%x", sum)
		payload.raw(sum[:])
	}
	m.FP.Payload = fmt.Sprintf("%x", payload.sum())

	m.FP.Structure = structureHash(f, boxes, modeStructure)
	m.FP.Layout = structureHash(f, boxes, modeLayout)
	m.FP.Metadata = metadataHash(f, boxes)
	return m, nil
}

func hashWholeFile(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// digestTrack 逐样本读取并哈希,按 (DTS,序号) 规范排序,与 chunk 布局无关。
func digestTrack(r io.ReaderAt, t *mp4.Track) (TrackDigest, error) {
	td := TrackDigest{
		TrackID: t.TrackID, Handler: t.Handler,
		Codec: t.Codec, Timescale: t.Timescale,
	}
	for _, s := range t.Samples {
		buf := make([]byte, s.Size)
		if _, err := r.ReadAt(buf, s.Offset); err != nil {
			return td, fmt.Errorf("读样本失败 轨%d 样本%d: %w", t.TrackID, s.Index, err)
		}
		td.Samples = append(td.Samples, SampleDigest{
			DTS: s.DTS, Duration: s.Duration, Size: s.Size,
			SHA256: sha256.Sum256(buf),
		})
	}
	sort.Slice(td.Samples, func(i, j int) bool {
		if td.Samples[i].DTS != td.Samples[j].DTS {
			return td.Samples[i].DTS < td.Samples[j].DTS
		}
		return td.Samples[i].SHA256[0] < td.Samples[j].SHA256[0]
	})
	return td, nil
}

// canon 是规范二进制序列化器,保证指纹跨平台可复现。
type canon struct{ h hash.Hash }

func newCanon(domain string) *canon {
	c := &canon{h: sha256.New()}
	c.str(domain)
	return c
}

func (c *canon) str(s string) {
	c.u32(uint32(len(s)))
	c.h.Write([]byte(s))
}
func (c *canon) u32(v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	c.h.Write(b[:])
}
func (c *canon) u64(v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	c.h.Write(b[:])
}
func (c *canon) raw(b []byte) { c.h.Write(b) }
func (c *canon) sum() [32]byte {
	var out [32]byte
	copy(out[:], c.h.Sum(nil))
	return out
}
