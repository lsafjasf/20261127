package estim

import (
	"math"
	"sort"
)

// Tile describes one spatial cell of the estimation grid.
type Tile struct {
	I, J              int
	X0, Y0, X1, Y1    int
	Mean, Min, P1, P5 float64
	Sigma             float64
	Used              bool
}

// Grid is a uniform tile partition of a frame.
type Grid struct {
	W, H         int
	Cols, Rows   int
	TileW, TileH int
	Tiles        []*Tile
}

// BuildGrid partitions w*h into at most cols*rows tiles, each at least
// minTile pixels on a side.
func BuildGrid(w, h, cols, rows, minTile int) *Grid {
	if cols < 2 {
		cols = 2
	}
	if rows < 2 {
		rows = 2
	}
	for cols > 2 && w/cols < minTile {
		cols--
	}
	for rows > 2 && h/rows < minTile {
		rows--
	}
	tw, th := w/cols, h/rows
	g := &Grid{W: w, H: h, Cols: cols, Rows: rows, TileW: tw, TileH: th}
	for j := 0; j < rows; j++ {
		y0 := j * th
		y1 := (j + 1) * th
		if j == rows-1 {
			y1 = h
		}
		for i := 0; i < cols; i++ {
			x0 := i * tw
			x1 := (i + 1) * tw
			if i == cols-1 {
				x1 = w
			}
			g.Tiles = append(g.Tiles, &Tile{I: i, J: j, X0: x0, Y0: y0, X1: x1, Y1: y1, Used: true})
		}
	}
	return g
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := q * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// Stats computes robust location and spread statistics for every tile.
func (g *Grid) Stats(y []float64) {
	for _, t := range g.Tiles {
		vals := make([]float64, 0, (t.X1-t.X0)*(t.Y1-t.Y0))
		sum := 0.0
		min := math.Inf(1)
		for row := t.Y0; row < t.Y1; row++ {
			base := row * g.W
			for col := t.X0; col < t.X1; col++ {
				v := y[base+col]
				vals = append(vals, v)
				sum += v
				if v < min {
					min = v
				}
			}
		}
		sort.Float64s(vals)
		t.Mean = sum / float64(len(vals))
		t.Min = min
		t.P1 = percentile(vals, 0.01)
		t.P5 = percentile(vals, 0.05)
		t.Sigma = neighborSigma(y, g.W, t)
	}
}

// neighborSigma estimates Gaussian noise sigma from MAD of horizontal
// neighbor differences inside the tile. For i.i.d. noise Var(diff)=2*sigma^2.
func neighborSigma(y []float64, stride int, t *Tile) float64 {
	diffs := make([]float64, 0, (t.X1-t.X0-1)*(t.Y1-t.Y0))
	for row := t.Y0; row < t.Y1; row++ {
		base := row*stride + t.X0
		for col := 0; col < t.X1-t.X0-1; col++ {
			diffs = append(diffs, y[base+col+1]-y[base+col])
		}
	}
	if len(diffs) < 8 {
		return 0
	}
	sort.Float64s(diffs)
	med := percentile(diffs, 0.5)
	abs := make([]float64, len(diffs))
	for i, d := range diffs {
		abs[i] = math.Abs(d - med)
	}
	sort.Float64s(abs)
	mad := percentile(abs, 0.5)
	return mad / (0.6745 * math.Sqrt2)
}

// FrameSigma is the median per-tile noise estimate (robust to textured tiles).
func (g *Grid) FrameSigma() float64 {
	vals := make([]float64, 0, len(g.Tiles))
	for _, t := range g.Tiles {
		if t.Sigma > 0 {
			vals = append(vals, t.Sigma)
		}
	}
	if len(vals) == 0 {
		return 0
	}
	sort.Float64s(vals)
	return percentile(vals, 0.5)
}
