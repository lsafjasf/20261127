package main

import (
	"bytes"
	"image"
	"image/png"
	"math"
)

const (
	sampleWidth  = 1440
	sampleHeight = 960

	refX = 0.203
	refY = 0.715
)

type rgb struct {
	r, g, b float64
}

func (c rgb) add(o rgb) rgb {
	return rgb{c.r + o.r, c.g + o.g, c.b + o.b}
}

func (c rgb) mul(v float64) rgb {
	return rgb{c.r * v, c.g * v, c.b * v}
}

func (c rgb) blend(o rgb, t float64) rgb {
	t = clamp01(t)
	return rgb{
		r: c.r + (o.r-c.r)*t,
		g: c.g + (o.g-c.g)*t,
		b: c.b + (o.b-c.b)*t,
	}
}

func buildSampleScene() ([]byte, scenePayload, error) {
	img := image.NewRGBA(image.Rect(0, 0, sampleWidth, sampleHeight))
	for y := 0; y < sampleHeight; y++ {
		for x := 0; x < sampleWidth; x++ {
			u := (float64(x) + 0.5) / sampleWidth
			v := (float64(y) + 0.5) / sampleHeight
			col := studioScene(float64(x), float64(y), u, v)
			col = gradeAndGrain(col, x, y, u, v)

			offset := y*img.Stride + x*4
			img.Pix[offset] = encodeSRGB(col.r)
			img.Pix[offset+1] = encodeSRGB(col.g)
			img.Pix[offset+2] = encodeSRGB(col.b)
			img.Pix[offset+3] = 255
		}
	}

	var out bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := encoder.Encode(&out, img); err != nil {
		return nil, scenePayload{}, err
	}
	info := scenePayload{
		ImageURL: "/sample.png",
		Width:    sampleWidth,
		Height:   sampleHeight,
		Reference: referenceInfo{
			X:     refX,
			Y:     refY,
			Width: 0.052,
			Label: "18% 中性灰卡",
		},
		Description: "棚拍静物 · 右侧窗光 · 18% 参考灰",
	}
	return out.Bytes(), info, nil
}

func studioScene(x, y, u, v float64) rgb {
	wall := rgb{0.016, 0.022, 0.027}.blend(rgb{0.045, 0.051, 0.061}, smoothstep(0.0, 0.72, v))
	wall = wall.add(rgb{0.012, 0.015, 0.022}.mul(smoothstep(0.25, 1.0, u)))
	col := wall

	// A broad, out-of-focus window supplies a believable local overexposure.
	window := roundedBox(x, y, 0.775*float64(sampleWidth), 0.278*float64(sampleHeight),
		0.121*float64(sampleWidth), 0.205*float64(sampleHeight), 25)
	windowGlow := math.Exp(-math.Pow(distanceToBox(x, y,
		0.775*float64(sampleWidth), 0.278*float64(sampleHeight),
		0.139*float64(sampleWidth), 0.225*float64(sampleHeight))/68, 2))
	col = col.add(rgb{0.50, 0.56, 0.61}.mul(windowGlow))

	if window > 0 {
		inner := rgb{1.24, 1.30, 1.36}
		inner = inner.add(rgb{0.45, 0.35, 0.23}.mul(smoothstep(0.1, 0.9, v)))
		inner = inner.mul(0.91 + 0.16*smoothstep(0.60, 0.88, u))
		mullionV := smoothstep(0.012, 0.030, math.Abs(u-0.775))
		mullionH := smoothstep(0.010, 0.026, math.Abs(v-0.278))
		inner = inner.mul(math.Min(mullionV, mullionH))
		col = col.blend(inner, window*0.96)
	}

	col = col.add(bokeh(x, y, 0.125, 0.125, 0.060))
	col = col.add(bokeh(x, y, 0.285, 0.205, 0.033).mul(0.58))
	col = col.add(bokeh(x, y, 0.923, 0.115, 0.045).mul(0.55))
	col = col.add(bokeh(x, y, 0.930, 0.505, 0.025).mul(0.72))

	horizon := 0.665 + 0.015*math.Sin((u-0.5)*4.4)
	if v > horizon {
		table := tableSurface(x, y, u, v)
		col = table
		shadow := math.Exp(-math.Pow((v-horizon-0.018)/0.030, 2))
		col = col.mul(1 - 0.48*shadow)
		reflection := math.Exp(-math.Pow((u-0.78)/0.10, 2)) *
			math.Exp(-math.Pow((v-horizon-0.070)/0.080, 2))
		col = col.add(rgb{0.20, 0.22, 0.23}.mul(reflection))
	}

	col = grayCard(col, x, y, u, v)
	col = bottle(col, x, y, u, v)
	col = ceramicCup(col, x, y, u, v)
	col = chromeObject(col, x, y, u, v)
	col = tableShadows(col, x, y)
	return col
}

func tableSurface(x, y, u, v float64) rgb {
	wood := rgb{0.034, 0.020, 0.014}.blend(rgb{0.105, 0.058, 0.034}, smoothstep(0.68, 1.0, v))
	fibers := math.Sin((x*0.043+math.Sin(y*0.015)*5.0))*0.5 + 0.5
	wood = wood.mul(0.84 + fibers*0.20)
	warmLight := math.Exp(-math.Pow((u-0.78)/0.36, 2)) * smoothstep(0.66, 0.84, v)
	return wood.add(rgb{0.12, 0.075, 0.036}.mul(warmLight))
}

func grayCard(col rgb, x, y, u, v float64) rgb {
	card := roundedBox(x, y, 0.205*float64(sampleWidth), 0.715*float64(sampleHeight),
		0.106*float64(sampleWidth), 0.123*float64(sampleHeight), 9)
	if card <= 0 {
		return col
	}
	// A quiet neutral card in the key light: useful as a known 18% anchor.
	patch := rgb{0.190, 0.190, 0.190}
	fine := 1 + (noise(x*0.32, y*0.32, 41)-0.5)*0.012
	patch = patch.mul(fine)
	distance := distanceToBox(x, y,
		0.205*float64(sampleWidth), 0.715*float64(sampleHeight),
		0.106*float64(sampleWidth), 0.123*float64(sampleHeight))
	border := smoothstep(-5.0, -1.5, distance)
	patch = patch.blend(rgb{0.50, 0.49, 0.46}, border*0.5)
	return col.blend(patch, card)
}

func bottle(col rgb, x, y, u, v float64) rgb {
	body := roundedBox(x, y, 0.445*float64(sampleWidth), 0.492*float64(sampleHeight),
		0.062*float64(sampleWidth), 0.139*float64(sampleHeight), 22)
	neck := roundedBox(x, y, 0.445*float64(sampleWidth), 0.302*float64(sampleHeight),
		0.020*float64(sampleWidth), 0.075*float64(sampleHeight), 10)
	shape := math.Max(body, neck)
	if shape <= 0 {
		return col
	}

	glass := rgb{0.013, 0.075, 0.052}
	glass = glass.add(rgb{0.018, 0.16, 0.095}.mul(smoothstep(0.30, 0.82, u)))
	edgeLight := math.Exp(-math.Pow((math.Abs(u-0.445)-0.059)/0.007, 2))
	glass = glass.add(rgb{0.17, 0.75, 0.52}.mul(edgeLight))
	glass = glass.add(rgb{0.80, 0.95, 0.90}.mul(
		math.Exp(-math.Pow((u-0.423)/0.0045, 2)) *
			smoothstep(0.36, 0.45, v) * (1 - smoothstep(0.56, 0.65, v))))
	glass = glass.add(rgb{1.40, 1.62, 1.66}.mul(
		math.Exp(-math.Pow((u-0.472)/0.0032, 2)) *
			math.Exp(-math.Pow((v-0.405)/0.070, 2))))
	col = col.blend(glass, shape)

	cap := roundedBox(x, y, 0.445*float64(sampleWidth), 0.225*float64(sampleHeight),
		0.023*float64(sampleWidth), 0.024*float64(sampleHeight), 7)
	if cap > 0 {
		metal := rgb{0.12, 0.105, 0.075}.add(rgb{0.44, 0.32, 0.14}.mul(
			math.Exp(-math.Pow((u-0.438)/0.008, 2))))
		col = col.blend(metal, cap)
	}
	return col
}

func ceramicCup(col rgb, x, y, u, v float64) rgb {
	body := roundedBox(x, y, 0.625*float64(sampleWidth), 0.582*float64(sampleHeight),
		0.062*float64(sampleWidth), 0.086*float64(sampleHeight), 24)
	if body <= 0 {
		return col
	}
	ceramic := rgb{0.24, 0.205, 0.155}
	ceramic = ceramic.add(rgb{0.50, 0.42, 0.30}.mul(
		math.Exp(-math.Pow((u-0.602)/0.030, 2))))
	ceramic = ceramic.add(rgb{0.30, 0.28, 0.23}.mul(smoothstep(0.50, 0.63, u)))
	rim := ellipse(x, y, 0.625*float64(sampleWidth), 0.503*float64(sampleHeight),
		0.060*float64(sampleWidth), 0.017*float64(sampleHeight))
	if rim > 0 {
		ceramic = ceramic.blend(rgb{0.60, 0.52, 0.39}, rim*0.72)
	}
	return col.blend(ceramic, body)
}

func chromeObject(col rgb, x, y, u, v float64) rgb {
	cx, cy := 0.765*float64(sampleWidth), 0.660*float64(sampleHeight)
	r := 0.041 * float64(sampleWidth)
	dx, dy := x-cx, y-cy
	dist := math.Hypot(dx, dy)
	shape := 1 - smoothstep(r-1.2, r+0.8, dist)
	if shape <= 0 {
		return col
	}
	chrome := rgb{0.055, 0.060, 0.058}
	chrome = chrome.add(rgb{0.52, 0.56, 0.55}.mul(
		math.Exp(-math.Pow((dx+0.30*r)/0.35/r, 2)) *
			math.Exp(-math.Pow((dy+0.27*r)/0.45/r, 2))))
	chrome = chrome.add(rgb{0.18, 0.13, 0.075}.mul(smoothstep(0.1, 0.9, v)))
	chrome = chrome.add(rgb{1.75, 1.80, 1.72}.mul(
		math.Exp(-math.Pow((dist-0.40*r)/(0.055*r), 2)) *
			smoothstep(-0.9, 0.1, dx/r) * (1 - smoothstep(0.15, 0.8, dy/r))))
	spot := math.Exp(-math.Pow((dx+0.34*r)/(0.055*r), 2)) *
		math.Exp(-math.Pow((dy+0.31*r)/(0.048*r), 2))
	chrome = chrome.add(rgb{2.2, 2.2, 2.1}.mul(spot))
	return col.blend(chrome, shape)
}

func tableShadows(col rgb, x, y float64) rgb {
	shadow1 := math.Exp(-math.Pow((x-0.455*float64(sampleWidth))/62, 2)) *
		math.Exp(-math.Pow((y-0.712*float64(sampleHeight))/95, 2))
	col = col.mul(1 - 0.34*shadow1)
	shadow2 := math.Exp(-math.Pow((x-0.626*float64(sampleWidth))/76, 2)) *
		math.Exp(-math.Pow((y-0.725*float64(sampleHeight))/86, 2))
	col = col.mul(1 - 0.30*shadow2)
	return col
}

func gradeAndGrain(col rgb, x, y int, u, v float64) rgb {
	vignette := 1 - 0.24*math.Pow(math.Hypot((u-0.5)*1.20, (v-0.48)*0.92), 2)
	col = col.mul(vignette)
	grain := (noise(float64(x)*0.88, float64(y)*0.88, 7) - 0.5) * 0.007
	col = col.add(rgb{grain, grain, grain})
	// Slight cool shadows / warm highlights, kept subtle.
	luma := col.r*0.2126 + col.g*0.7152 + col.b*0.0722
	col = col.blend(rgb{luma * 0.92, luma * 0.99, luma * 1.03}, 0.10)
	return col
}

func bokeh(x, y, cx, cy, radius float64) rgb {
	dx := (x/float64(sampleWidth) - cx) * float64(sampleWidth)
	dy := (y/float64(sampleHeight) - cy) * float64(sampleHeight)
	r := radius * float64(sampleWidth)
	d := math.Hypot(dx, dy)
	ring := math.Exp(-math.Pow((d-r/1.9)/(r*0.30), 2))
	core := 1 - smoothstep(r*0.84, r*1.04, d)
	return rgb{0.38, 0.145, 0.034}.mul(ring*0.55 + core*0.40)
}

func roundedBox(x, y, cx, cy, hx, hy, radius float64) float64 {
	qx := math.Abs(x-cx) - (hx - radius)
	qy := math.Abs(y-cy) - (hy - radius)
	outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	inside := math.Min(math.Max(qx, qy), 0)
	distance := outside + inside - radius
	return 1 - smoothstep(-1.2, 0.8, distance)
}

func distanceToBox(x, y, cx, cy, hx, hy float64) float64 {
	qx := math.Abs(x-cx) - hx
	qy := math.Abs(y-cy) - hy
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0)
}

func ellipse(x, y, cx, cy, rx, ry float64) float64 {
	d := math.Hypot((x-cx)/rx, (y-cy)/ry)
	return 1 - smoothstep(0.94, 1.02, d)
}

func encodeSRGB(v float64) uint8 {
	v = math.Max(0, v)
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return uint8(clamp01(v)*255 + 0.5)
}

func smoothstep(edge0, edge1, x float64) float64 {
	if edge0 == edge1 {
		if x < edge0 {
			return 0
		}
		return 1
	}
	t := clamp01((x - edge0) / (edge1 - edge0))
	return t * t * (3 - 2*t)
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

func noise(x, y float64, seed uint32) float64 {
	xi, yi := int64(math.Floor(x)), int64(math.Floor(y))
	xf, yf := x-float64(xi), y-float64(yi)
	u := xf * xf * (3 - 2*xf)
	v := yf * yf * (3 - 2*yf)
	a := hash2(xi, yi, seed)
	b := hash2(xi+1, yi, seed)
	c := hash2(xi, yi+1, seed)
	d := hash2(xi+1, yi+1, seed)
	return (a*(1-u)+b*u)*(1-v) + (c*(1-u)+d*u)*v
}

func hash2(x, y int64, seed uint32) float64 {
	h := uint64(uint32(x))*0x9e3779b185ebca87 ^ uint64(uint32(y))*0xc2b2ae3d27d4eb4f ^ uint64(seed)*0x165667b19e3779f9
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return float64(h>>11) / float64(1<<53)
}
