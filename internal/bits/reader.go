package bits

import (
	"errors"
	"fmt"
)

var ErrTruncated = errors.New("bitstream ended early")

type Reader struct {
	data []byte
	pos  int
}

func NewReader(data []byte) *Reader {
	return &Reader{data: data}
}

func (r *Reader) BitsLeft() int {
	return len(r.data)*8 - r.pos
}

func (r *Reader) BitPos() int {
	return r.pos
}

func (r *Reader) ByteAligned() bool {
	return r.pos%8 == 0
}

func (r *Reader) ReadBits(n int) (uint64, error) {
	if n < 0 || n > 64 {
		return 0, fmt.Errorf("invalid bit count %d", n)
	}
	if r.BitsLeft() < n {
		return 0, ErrTruncated
	}
	var v uint64
	for i := 0; i < n; i++ {
		byteIndex := r.pos / 8
		bitIndex := 7 - r.pos%8
		v = v<<1 | uint64((r.data[byteIndex]>>bitIndex)&1)
		r.pos++
	}
	return v, nil
}

func (r *Reader) ReadBool() (bool, error) {
	v, err := r.ReadBits(1)
	return v != 0, err
}

func (r *Reader) Skip(n int) error {
	if n < 0 || r.BitsLeft() < n {
		return ErrTruncated
	}
	r.pos += n
	return nil
}

func (r *Reader) AlignZero() error {
	for !r.ByteAligned() {
		v, err := r.ReadBits(1)
		if err != nil {
			return err
		}
		if v != 0 {
			return errors.New("alignment bit is not zero")
		}
	}
	return nil
}

func (r *Reader) ReadUE() (uint64, error) {
	zeros := 0
	for {
		b, err := r.ReadBits(1)
		if err != nil {
			return 0, err
		}
		if b != 0 {
			break
		}
		zeros++
		if zeros > 63 {
			return 0, errors.New("Exp-Golomb code is too long")
		}
	}
	if zeros == 0 {
		return 0, nil
	}
	suffix, err := r.ReadBits(zeros)
	if err != nil {
		return 0, err
	}
	return (uint64(1) << zeros) - 1 + suffix, nil
}
