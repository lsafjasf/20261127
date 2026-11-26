package testmedia

import "encoding/binary"

func box(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

func fullbox(typ string, version byte, flags uint32, payload []byte) []byte {
	head := make([]byte, 4)
	binary.BigEndian.PutUint32(head, uint32(version)<<24|flags)
	return box(typ, append(head, payload...))
}

func u16(v uint16) []byte { return []byte{byte(v >> 8), byte(v)} }

func u32(v uint32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, v)
	return out
}

func zeros(n int) []byte { return make([]byte, n) }

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func matrix() []byte {
	return concat(
		u32(0x00010000), u32(0), u32(0),
		u32(0), u32(0x00010000), u32(0),
		u32(0), u32(0), u32(0x40000000),
	)
}
