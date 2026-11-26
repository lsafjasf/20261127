package laddercheck

import "encoding/binary"

// fbox is a minimal ISO-BMFF box used by the deterministic fixture muxer.
// Either body is used directly or kids are concatenated.
type fbox struct {
	typ  string
	body []byte
	kids []*fbox
}

func boxBytes(typ string, body []byte) *fbox { return &fbox{typ: typ, body: body} }

func container(typ string, kids ...*fbox) *fbox { return &fbox{typ: typ, kids: kids} }

func fullBox(typ string, version byte, flags uint32, body []byte) *fbox {
	head := []byte{version, byte(flags >> 16), byte(flags >> 8), byte(flags)}
	return &fbox{typ: typ, body: append(head, body...)}
}

func (b *fbox) encode() []byte {
	var payload []byte
	if b.kids != nil {
		for _, k := range b.kids {
			payload = append(payload, k.encode()...)
		}
	} else {
		payload = b.body
	}
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], []byte(b.typ))
	copy(out[8:], payload)
	return out
}

type fbw struct{ b []byte }

func (w *fbw) u8(v byte) { w.b = append(w.b, v) }
func (w *fbw) u16(v uint16) {
	var x [2]byte
	binary.BigEndian.PutUint16(x[:], v)
	w.b = append(w.b, x[:]...)
}
func (w *fbw) u32(v uint32) {
	var x [4]byte
	binary.BigEndian.PutUint32(x[:], v)
	w.b = append(w.b, x[:]...)
}
func (w *fbw) bytes(p []byte) { w.b = append(w.b, p...) }

func ftypBox() *fbox {
	w := &fbw{}
	w.bytes([]byte("isom"))
	w.u32(0x00000200)
	w.bytes([]byte("isomiso2avc1mp41"))
	return boxBytes("ftyp", w.b)
}

func mvhdBox(timescale, duration uint32) *fbox {
	w := &fbw{}
	w.u32(0) // creation
	w.u32(0) // modification
	w.u32(timescale)
	w.u32(duration)
	w.u32(0x00010000) // rate 1.0
	w.u16(0x0100)     // volume 1.0
	w.u16(0)
	w.u32(0)
	w.u32(0) // reserved 10 bytes total after volume? u16 + two u32
	// Identity matrix (9 u32).
	matrix := []uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000}
	for _, m := range matrix {
		w.u32(m)
	}
	for i := 0; i < 6; i++ {
		w.u32(0) // pre-defined
	}
	w.u32(2) // next track id
	return fullBox("mvhd", 0, 0, w.b)
}

func tkhdBox(trackID, duration uint32, width, height uint16) *fbox {
	w := &fbw{}
	w.u32(0) // creation
	w.u32(0) // modification
	w.u32(trackID)
	w.u32(0) // reserved
	w.u32(duration)
	w.u32(0)
	w.u32(0) // reserved 8
	w.u16(0) // layer
	w.u16(0) // alternate group
	w.u16(0) // volume
	w.u16(0) // reserved
	matrix := []uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000}
	for _, m := range matrix {
		w.u32(m)
	}
	w.u32(uint32(width) << 16)
	w.u32(uint32(height) << 16)
	return fullBox("tkhd", 0, 0x000007, w.b) // enabled|in movie|in preview
}

func mdhdBox(timescale, duration uint32) *fbox {
	w := &fbw{}
	w.u32(0)
	w.u32(0)
	w.u32(timescale)
	w.u32(duration)
	w.u16(0x55C4) // language "und"
	w.u16(0)
	return fullBox("mdhd", 0, 0, w.b)
}

func hdlrBox() *fbox {
	w := &fbw{}
	w.u32(0) // pre-defined
	w.bytes([]byte("vide"))
	w.u32(0)
	w.u32(0)
	w.u32(0) // reserved 12
	w.bytes([]byte("VideoHandler\x00"))
	return fullBox("hdlr", 0, 0, w.b)
}
