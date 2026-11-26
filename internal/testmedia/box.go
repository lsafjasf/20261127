package testmedia

import (
	"encoding/binary"
)

func Box(typ string, parts ...[]byte) []byte {
	size := 8
	for _, part := range parts {
		size += len(part)
	}
	out := make([]byte, 8, size)
	binary.BigEndian.PutUint32(out[:4], uint32(size))
	copy(out[4:8], typ)
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func FullBox(typ string, version byte, flags uint32, payload []byte) []byte {
	head := make([]byte, 4)
	head[0] = version
	head[1] = byte(flags >> 16)
	head[2] = byte(flags >> 8)
	head[3] = byte(flags)
	return Box(typ, head, payload)
}

func U16(v uint16) []byte {
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, v)
	return out
}

func U32(v uint32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, v)
	return out
}

func U64(v uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, v)
	return out
}

func Join(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}
