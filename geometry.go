package aperturemap

import (
	"errors"
	"fmt"
)

// Size is an integer pixel-grid dimension.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// IntRect is an integer rectangle using half-open pixel-grid coordinates.
type IntRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (r IntRect) Canonical() IntRect {
	if r.Width < 0 {
		r.X += r.Width
		r.Width = -r.Width
	}
	if r.Height < 0 {
		r.Y += r.Height
		r.Height = -r.Height
	}
	return r
}

// Point is an exact point in a rational coordinate system.
type Point struct {
	X Ratio `json:"x"`
	Y Ratio `json:"y"`
}

// Rect is an axis-aligned rectangle represented by its outer edges.
type Rect struct {
	Min Point `json:"min"`
	Max Point `json:"max"`
}

func NewRect(x0, y0, x1, y1 Ratio) Rect {
	return Rect{Min: Point{X: x0, Y: y0}, Max: Point{X: x1, Y: y1}}
}

// PixelGridRect returns the outer-edge rectangle of an integer pixel grid.
// Pixel centers are at integer coordinates, so the first outer edge is -1/2.
func PixelGridRect(width, height int) Rect {
	half := MustRatio(1, 2)
	return Rect{
		Min: Point{X: half.Mul(MustRatio(-1, 1)), Y: half.Mul(MustRatio(-1, 1))},
		Max: Point{
			X: MustRatio(int64(width), 1).Sub(half),
			Y: MustRatio(int64(height), 1).Sub(half),
		},
	}
}

func (r Rect) Width() Ratio  { return r.Max.X.Sub(r.Min.X) }
func (r Rect) Height() Ratio { return r.Max.Y.Sub(r.Min.Y) }

func (r Rect) Contains(other Rect) bool {
	return r.Min.X.Cmp(other.Min.X) <= 0 &&
		r.Min.Y.Cmp(other.Min.Y) <= 0 &&
		r.Max.X.Cmp(other.Max.X) >= 0 &&
		r.Max.Y.Cmp(other.Max.Y) >= 0
}

func (r Rect) Intersect(other Rect) Rect {
	minX := r.Min.X
	if other.Min.X.Cmp(minX) > 0 {
		minX = other.Min.X
	}
	minY := r.Min.Y
	if other.Min.Y.Cmp(minY) > 0 {
		minY = other.Min.Y
	}
	maxX := r.Max.X
	if other.Max.X.Cmp(maxX) < 0 {
		maxX = other.Max.X
	}
	maxY := r.Max.Y
	if other.Max.Y.Cmp(maxY) < 0 {
		maxY = other.Max.Y
	}
	return NewRect(minX, minY, maxX, maxY)
}

// Affine is an axis-aligned scale and translation:
//
//	X' = ScaleX*X + TranslateX
//	Y' = ScaleY*Y + TranslateY
type Affine struct {
	ScaleX     Ratio `json:"scale_x"`
	ScaleY     Ratio `json:"scale_y"`
	TranslateX Ratio `json:"translate_x"`
	TranslateY Ratio `json:"translate_y"`
}

// MapRect returns the rectangle obtained by applying the affine transform.
func (a Affine) MapRect(r Rect) Rect {
	return NewRect(
		a.ScaleX.Mul(r.Min.X).Add(a.TranslateX),
		a.ScaleY.Mul(r.Min.Y).Add(a.TranslateY),
		a.ScaleX.Mul(r.Max.X).Add(a.TranslateX),
		a.ScaleY.Mul(r.Max.Y).Add(a.TranslateY),
	)
}

// Apply maps a single point through the affine transform.
func (a Affine) Apply(p Point) Point {
	return Point{
		X: a.ScaleX.Mul(p.X).Add(a.TranslateX),
		Y: a.ScaleY.Mul(p.Y).Add(a.TranslateY),
	}
}

// Then returns the transform "a followed by next".
func (a Affine) Then(next Affine) Affine {
	return Affine{
		ScaleX:     a.ScaleX.Mul(next.ScaleX),
		ScaleY:     a.ScaleY.Mul(next.ScaleY),
		TranslateX: next.ScaleX.Mul(a.TranslateX).Add(next.TranslateX),
		TranslateY: next.ScaleY.Mul(a.TranslateY).Add(next.TranslateY),
	}
}

// Inverse returns the inverse transform.
func (a Affine) Inverse() Affine {
	return Affine{
		ScaleX:     MustRatio(1, 1).Div(a.ScaleX),
		ScaleY:     MustRatio(1, 1).Div(a.ScaleY),
		TranslateX: Ratio{},
		TranslateY: Ratio{},
	}.withInverseTranslation(a)
}

func (inv Affine) withInverseTranslation(forward Affine) Affine {
	inv.TranslateX = inv.ScaleX.Mul(forward.TranslateX).Mul(MustRatio(-1, 1))
	inv.TranslateY = inv.ScaleY.Mul(forward.TranslateY).Mul(MustRatio(-1, 1))
	return inv
}

// RectMap constructs the exact affine map from one rectangle to another.
func RectMap(from, to Rect) (Affine, error) {
	fw, fh := from.Width(), from.Height()
	tw, th := to.Width(), to.Height()
	if !fw.Positive() || !fh.Positive() || !tw.Positive() || !th.Positive() {
		return Affine{}, fmt.Errorf("%w: cannot map degenerate rectangle", ErrInvalidGeometry)
	}
	sx := tw.Div(fw)
	sy := th.Div(fh)
	return Affine{
		ScaleX:     sx,
		ScaleY:     sy,
		TranslateX: to.Min.X.Sub(sx.Mul(from.Min.X)),
		TranslateY: to.Min.Y.Sub(sy.Mul(from.Min.Y)),
	}, nil
}

var (
	ErrInvalidGeometry        = errors.New("invalid geometry")
	ErrInconsistentRatio      = errors.New("inconsistent aspect-ratio declarations")
	ErrApertureOutOfBounds    = errors.New("clean aperture lies outside effective pixel grid")
	ErrPresentationImpossible = errors.New("presentation constraints cannot be satisfied")
)

// FitMode selects how a source aspect ratio is fitted into a player viewport.
type FitMode string

const (
	// FitExact requires equal aspect ratios and uniform full-image mapping.
	FitExact FitMode = "exact"
	// FitContain preserves the whole image and uniform scale, leaving bars.
	FitContain FitMode = "contain"
	// FitCover preserves uniform scale and fills the viewport, cropping overflow.
	FitCover FitMode = "cover"
	// FitStretch maps the whole image to the viewport, allowing distortion.
	FitStretch FitMode = "stretch"
)
