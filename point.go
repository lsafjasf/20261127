package gamutcompare

import (
	"encoding/json"
	"fmt"
	"math"
)

// Point is a CIE 1931 xy chromaticity coordinate unless a report explicitly
// says that the Up/VP fields are being used.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// UnmarshalJSON accepts {"x":0.3,"y":0.4}, [0.3,0.4], and null.
func (p *Point) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*p = Point{}
		return nil
	}

	var arr [2]float64
	if err := json.Unmarshal(data, &arr); err == nil {
		p.X, p.Y = arr[0], arr[1]
		return nil
	}

	var obj struct {
		X *float64 `json:"x"`
		Y *float64 `json:"y"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("point must be [x,y] or {x,y}: %w", err)
	}
	if obj.X == nil || obj.Y == nil {
		return fmt.Errorf("point requires both x and y")
	}
	p.X, p.Y = *obj.X, *obj.Y
	return nil
}

func (p Point) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}{X: p.X, Y: p.Y})
}

// UpVP converts CIE 1931 xy to CIE 1976 u'v'.
func (p Point) UpVP() (up, vp float64) {
	den := -2*p.X + 12*p.Y + 3
	return 4 * p.X / den, 9 * p.Y / den
}

func (p Point) upVPPoint() Point {
	u, v := p.UpVP()
	return Point{X: u, Y: v}
}

func finitePoint(p Point) bool {
	return isFiniteFloat(p.X) && isFiniteFloat(p.Y)
}

func isFiniteFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func nearPoint(a, b Point, eps float64) bool {
	return math.Abs(a.X-b.X) <= eps && math.Abs(a.Y-b.Y) <= eps
}
