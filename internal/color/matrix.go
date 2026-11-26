package color

import (
	"fmt"
	"math"
	"strings"
)

// Matrix identifies the luma/chroma conversion used to interpret YUV samples.
type Matrix string

const (
	BT601 Matrix = "bt601"
	BT709 Matrix = "bt709"
)

type Coefficients struct {
	Kr, Kg, Kb float64
	RU, RV     float64
	GU, GV     float64
}

var matrices = map[Matrix]Coefficients{
	BT601: coefficients(0.299, 0.587, 0.114),
	BT709: coefficients(0.2126, 0.7152, 0.0722),
}

func coefficients(kr, kg, kb float64) Coefficients {
	c := Coefficients{Kr: kr, Kg: kg, Kb: kb}
	c.RU = 2 * (1 - kb)
	c.RV = 2 * (1 - kr)
	c.GU = -c.RU * kb / kg
	c.GV = -c.RV * kr / kg
	return c
}

func (m Matrix) Coefficients() Coefficients {
	c, ok := matrices[m]
	if !ok {
		panic("unknown matrix: " + string(m))
	}
	return c
}

// Decode converts normalized YUV to normalized nonlinear R'G'B'. Values are
// deliberately not clamped: out-of-gamut values are useful diagnostic evidence.
func Decode(m Matrix, y, u, v float64) (r, g, b float64) {
	c := m.Coefficients()
	r = y + c.RV*v
	b = y + c.RU*u
	g = y + c.GU*u + c.GV*v
	return r, g, b
}

func Encode(m Matrix, r, g, b float64) (y, u, v float64) {
	c := m.Coefficients()
	y = c.Kr*r + c.Kg*g + c.Kb*b
	u = (b - y) / (2 * (1 - c.Kb))
	v = (r - y) / (2 * (1 - c.Kr))
	return y, u, v
}

func Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func ClampByte(v float64) uint8 {
	if math.IsNaN(v) || v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(math.Round(v))
}

// FromFFprobe maps FFmpeg/ffprobe colour_space names to a supported matrix.
func FromFFprobe(name string) Matrix {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bt470bg", "smpte170m", "bt601", "rec601", "pal", "ntsc":
		return BT601
	case "bt709", "rec709":
		return BT709
	default:
		return ""
	}
}

// FFprobeName is the PAL/SD name used when writing corrected 720x576 metadata.
func (m Matrix) FFprobeName() string {
	switch m {
	case BT601:
		return "bt470bg"
	case BT709:
		return "bt709"
	default:
		return ""
	}
}

func (m Matrix) Label() string {
	switch m {
	case BT601:
		return "Rec.601 / BT.470BG"
	case BT709:
		return "Rec.709"
	default:
		return string(m)
	}
}

func ParseMatrix(name string) (Matrix, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bt601", "rec601", "bt470bg", "smpte170m":
		return BT601, nil
	case "bt709", "rec709":
		return BT709, nil
	default:
		return "", fmt.Errorf("unknown matrix %q (use bt601 or bt709)", name)
	}
}
