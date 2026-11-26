package aperturemap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

var ErrInvalidBoxPayload = errors.New("invalid box payload")

// ParseCLAPPayload parses the 32-byte CleanApertureBox payload.
// Box size/type headers are intentionally handled by the caller's MP4 parser.
func ParseCLAPPayload(payload []byte) (*CleanAperture, error) {
	if len(payload) != 32 {
		return nil, fmt.Errorf("%w: clap payload length = %d, want 32", ErrInvalidBoxPayload, len(payload))
	}

	widthN := binary.BigEndian.Uint32(payload[0:4])
	widthD := binary.BigEndian.Uint32(payload[4:8])
	heightN := binary.BigEndian.Uint32(payload[8:12])
	heightD := binary.BigEndian.Uint32(payload[12:16])
	horizN := int32(binary.BigEndian.Uint32(payload[16:20]))
	horizD := binary.BigEndian.Uint32(payload[20:24])
	vertN := int32(binary.BigEndian.Uint32(payload[24:28]))
	vertD := binary.BigEndian.Uint32(payload[28:32])

	width, err := NewRatio(int64(widthN), int64(widthD))
	if err != nil {
		return nil, fmt.Errorf("clap width: %w", err)
	}
	height, err := NewRatio(int64(heightN), int64(heightD))
	if err != nil {
		return nil, fmt.Errorf("clap height: %w", err)
	}
	horiz, err := NewRatio(int64(horizN), int64(horizD))
	if err != nil {
		return nil, fmt.Errorf("clap horizontal offset: %w", err)
	}
	vert, err := NewRatio(int64(vertN), int64(vertD))
	if err != nil {
		return nil, fmt.Errorf("clap vertical offset: %w", err)
	}

	return &CleanAperture{
		Width:            width,
		Height:           height,
		HorizontalOffset: horiz,
		VerticalOffset:   vert,
	}, nil
}

// MarshalCLAPPayload encodes a clap payload using unreduced exact values.
func (ca CleanAperture) MarshalCLAPPayload() ([]byte, error) {
	ca.Width = ca.Width.canonical()
	ca.Height = ca.Height.canonical()
	ca.HorizontalOffset = ca.HorizontalOffset.canonical()
	ca.VerticalOffset = ca.VerticalOffset.canonical()
	if err := validateWireRatio(ca.Width, false); err != nil {
		return nil, fmt.Errorf("width: %w", err)
	}
	if err := validateWireRatio(ca.Height, false); err != nil {
		return nil, fmt.Errorf("height: %w", err)
	}
	if err := validateWireRatio(ca.HorizontalOffset, true); err != nil {
		return nil, fmt.Errorf("horizontal offset: %w", err)
	}
	if err := validateWireRatio(ca.VerticalOffset, true); err != nil {
		return nil, fmt.Errorf("vertical offset: %w", err)
	}

	payload := make([]byte, 32)
	binary.BigEndian.PutUint32(payload[0:4], uint32(ca.Width.N))
	binary.BigEndian.PutUint32(payload[4:8], uint32(ca.Width.D))
	binary.BigEndian.PutUint32(payload[8:12], uint32(ca.Height.N))
	binary.BigEndian.PutUint32(payload[12:16], uint32(ca.Height.D))
	binary.BigEndian.PutUint32(payload[16:20], uint32(int32(ca.HorizontalOffset.N)))
	binary.BigEndian.PutUint32(payload[20:24], uint32(ca.HorizontalOffset.D))
	binary.BigEndian.PutUint32(payload[24:28], uint32(int32(ca.VerticalOffset.N)))
	binary.BigEndian.PutUint32(payload[28:32], uint32(ca.VerticalOffset.D))
	return payload, nil
}

// ParsePASPPayload parses hSpacing/vSpacing into PAR = hSpacing/vSpacing.
func ParsePASPPayload(payload []byte) (Ratio, error) {
	if len(payload) != 8 {
		return Ratio{}, fmt.Errorf("%w: pasp payload length = %d, want 8", ErrInvalidBoxPayload, len(payload))
	}
	h := binary.BigEndian.Uint32(payload[0:4])
	v := binary.BigEndian.Uint32(payload[4:8])
	return NewRatio(int64(h), int64(v))
}

// MarshalPASPPayload encodes PAR as hSpacing/vSpacing.
func MarshalPASPPayload(par Ratio) ([]byte, error) {
	par = par.canonical()
	if err := validateWireRatio(par, false); err != nil {
		return nil, err
	}
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[0:4], uint32(par.N))
	binary.BigEndian.PutUint32(payload[4:8], uint32(par.D))
	return payload, nil
}

func validateWireRatio(r Ratio, signedNumerator bool) error {
	r = r.canonical()
	if r.D <= 0 || r.D > math.MaxUint32 {
		return fmt.Errorf("%w: denominator %d out of uint32 range", ErrInvalidGeometry, r.D)
	}
	if signedNumerator {
		if r.N < math.MinInt32 || r.N > math.MaxInt32 {
			return fmt.Errorf("%w: signed numerator %d out of int32 range", ErrInvalidGeometry, r.N)
		}
		return nil
	}
	if r.N < 0 || r.N > math.MaxUint32 {
		return fmt.Errorf("%w: unsigned numerator %d out of uint32 range", ErrInvalidGeometry, r.N)
	}
	return nil
}
