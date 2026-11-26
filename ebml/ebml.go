// Package ebml implements the small subset of EBML needed by WebM files.
package ebml

import (
	"errors"
	"fmt"
)

var (
	ErrTruncated = errors.New("ebml: truncated element")
	ErrVINT      = errors.New("ebml: invalid variable-length integer")
)

// ID is an EBML element identifier, including its length marker bit.
type ID uint32

// Element is a parsed EBML element. Data aliases the input byte slice.
type Element struct {
	ID          ID
	Offset      int
	HeaderSize  int
	Size        uint64
	UnknownSize bool
	Data        []byte
	Children    []*Element
}

func (e *Element) Int() uint64 {
	return readUint(e.Data)
}

func (e *Element) String() string {
	return string(e.Data)
}

// Find returns the first direct child with id.
func (e *Element) Find(id ID) *Element {
	for _, child := range e.Children {
		if child.ID == id {
			return child
		}
	}
	return nil
}

// FindAll returns all direct children with id.
func (e *Element) FindAll(id ID) []*Element {
	var out []*Element
	for _, child := range e.Children {
		if child.ID == id {
			out = append(out, child)
		}
	}
	return out
}

// Walk calls fn for every descendant. Return false from fn to skip children.
func (e *Element) Walk(fn func(*Element) bool) {
	for _, child := range e.Children {
		if fn(child) {
			child.Walk(fn)
		}
	}
}

// Parse parses a complete EBML document or element range.
func Parse(data []byte) ([]*Element, error) {
	return parseRange(data, 0, len(data), 0)
}

func parseRange(data []byte, start, end, depth int) ([]*Element, error) {
	if depth > 32 {
		return nil, fmt.Errorf("ebml: element nesting too deep")
	}
	var out []*Element
	off := start
	for off < end {
		if end-off < 1 {
			break
		}
		el, next, err := parseElement(data, off, end, depth)
		if err != nil {
			return out, err
		}
		out = append(out, el)
		off = next
	}
	return out, nil
}

func parseElement(data []byte, off, end, depth int) (*Element, int, error) {
	id, idN, err := readID(data, off, end)
	if err != nil {
		return nil, off, err
	}
	size, sizeN, unknown, err := readSize(data, off+idN, end)
	if err != nil {
		return nil, off, err
	}
	header := idN + sizeN
	dataOff := off + header
	el := &Element{ID: id, Offset: off, HeaderSize: header, UnknownSize: unknown}

	if unknown {
		el.Size = uint64(end - dataOff)
	} else {
		el.Size = size
	}
	dataEnd := dataOff + int(el.Size)
	if el.Size > uint64(end-dataOff) || dataEnd > end || dataOff > end {
		return nil, off, fmt.Errorf("%w: id %x at %d needs %d bytes, %d available",
			ErrTruncated, id, off, el.Size, max(0, end-dataOff))
	}
	el.Data = data[dataOff:dataEnd]
	if containers[id] {
		children, err := parseRange(data, dataOff, dataEnd, depth+1)
		if err != nil {
			return nil, off, fmt.Errorf("element %x: %w", id, err)
		}
		el.Children = children
	}
	return el, dataEnd, nil
}

func readID(data []byte, off, end int) (ID, int, error) {
	if off >= end {
		return 0, 0, ErrTruncated
	}
	n := vintWidth(data[off])
	if n < 1 || n > 4 || off+n > end {
		return 0, 0, fmt.Errorf("%w: element id at %d", ErrVINT, off)
	}
	var id uint32
	for i := 0; i < n; i++ {
		id = id<<8 | uint32(data[off+i])
	}
	return ID(id), n, nil
}

func readSize(data []byte, off, end int) (value uint64, n int, unknown bool, err error) {
	if off >= end {
		return 0, 0, false, ErrTruncated
	}
	n = vintWidth(data[off])
	if n < 1 || n > 8 || off+n > end {
		return 0, n, false, fmt.Errorf("%w: size at %d", ErrVINT, off)
	}
	value = uint64(data[off] & (0xff >> n))
	for i := 1; i < n; i++ {
		value = value<<8 | uint64(data[off+i])
	}
	maxValue := uint64(1)<<uint(7*n) - 1
	if value == maxValue {
		return value, n, true, nil
	}
	return value, n, false, nil
}

func vintWidth(first byte) int {
	for n := 1; n <= 8; n++ {
		if first&(0x80>>uint(n-1)) != 0 {
			return n
		}
	}
	return 0
}

func readUint(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
