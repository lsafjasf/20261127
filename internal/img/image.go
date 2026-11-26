// Package img 定义线性光 float64 RGB 图像容器及基础几何操作。
package img

// Image 是三平面线性光 RGB 图像，像素值无内置上限（HDR 可超过 1）。
type Image struct {
	W, H int
	R    []float64
	G    []float64
	B    []float64
}

// New 创建指定尺寸、初始为零的图像。
func New(w, h int) *Image {
	n := w * h
	return &Image{W: w, H: h, R: make([]float64, n), G: make([]float64, n), B: make([]float64, n)}
}

func (m *Image) idx(x, y int) int { return y*m.W + x }

// Set 写入一个像素。
func (m *Image) Set(x, y int, r, g, b float64) {
	i := m.idx(x, y)
	m.R[i], m.G[i], m.B[i] = r, g, b
}

// At 读取一个像素。
func (m *Image) At(x, y int) (r, g, b float64) {
	i := m.idx(x, y)
	return m.R[i], m.G[i], m.B[i]
}

// Clone 返回深拷贝。
func (m *Image) Clone() *Image {
	out := New(m.W, m.H)
	copy(out.R, m.R)
	copy(out.G, m.G)
	copy(out.B, m.B)
	return out
}

// Rect 表示像素矩形 [X0,X1) x [Y0,Y1)。
type Rect struct {
	X0, Y0, X1, Y1 int
}

// FillRect 用给定颜色填充矩形。
func (m *Image) FillRect(rc Rect, r, g, b float64) {
	for y := rc.Y0; y < rc.Y1; y++ {
		for x := rc.X0; x < rc.X1; x++ {
			m.Set(x, y, r, g, b)
		}
	}
}

// MeanRect 返回矩形内各通道均值。
func (m *Image) MeanRect(rc Rect) (r, g, b float64) {
	n := 0
	for y := rc.Y0; y < rc.Y1; y++ {
		for x := rc.X0; x < rc.X1; x++ {
			i := m.idx(x, y)
			r += m.R[i]
			g += m.G[i]
			b += m.B[i]
			n++
		}
	}
	if n == 0 {
		return 0, 0, 0
	}
	inv := 1.0 / float64(n)
	return r * inv, g * inv, b * inv
}
