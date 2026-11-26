package observermap

import (
	"fmt"
	"math"
	"strings"
)

// svgChromaticity renders an inline SVG xy chromaticity diagram showing the
// 2° and 10° spectral loci and arrows x2,y2 -> x10,y10 for report rows.
func svgChromaticity(r *DemoReport) string {
	sd, err := LoadStandardData()
	if err != nil {
		return `<p class="note">chromaticity diagram unavailable: ` + err.Error() + `</p>`
	}

	const (
		width  = 720
		height = 660
		ml, mr = 52.0, 24.0
		mt, mb = 20.0, 46.0
		domain = 0.9
	)
	plot := width - ml - mr
	scale := plot / domain
	yPlot := height - mt - mb
	if hScale := yPlot / domain; hScale < scale {
		scale = hScale
	}

	px := func(x float64) float64 { return ml + x*scale }
	py := func(y float64) float64 { return mt + (domain-y)*scale }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %d %d" role="img" aria-label="xy chromaticity diagram" style="width:100%%;max-width:760px;background:#fff;border:1px solid #d9dee7;border-radius:8px">`, width, height)
	b.WriteString(`<defs><marker id="arrowSpec" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto"><path d="M0,0 L6,3 L0,6 Z" fill="#d1495b"/></marker>`)
	b.WriteString(`<marker id="arrowSw" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto"><path d="M0,0 L6,3 L0,6 Z" fill="#3567b8"/></marker></defs>`)

	// Grid and tick labels.
	for i := 0; i <= 9; i++ {
		v := float64(i) / 10
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#e8ecf3" stroke-width="1"/>`, px(v), py(0), px(v), py(domain))
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#e8ecf3" stroke-width="1"/>`, px(0), py(v), px(domain), py(v))
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="10" fill="#7a8494" text-anchor="middle">%.1f</text>`, px(v), py(0)+16, v)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="10" fill="#7a8494" text-anchor="end" dominant-baseline="middle">%.1f</text>`, px(0)-7, py(v), v)
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%d" font-size="12" fill="#5a6572" text-anchor="middle">x</text>`, ml+plot/2, height-8)
	fmt.Fprintf(&b, `<text x="14" y="%.1f" font-size="12" fill="#5a6572" text-anchor="middle" transform="rotate(-90 14 %.1f)">y</text>`, mt+yPlot/2+mb/2, mt+yPlot/2+mb/2)

	locusPath := func(cmf *CMFTable) string {
		var p strings.Builder
		started := false
		for i := range cmf.Wavelength {
			s := cmf.Xbar[i] + cmf.Ybar[i] + cmf.Zbar[i]
			if s <= 0 {
				continue
			}
			x := cmf.Xbar[i] / s
			y := cmf.Ybar[i] / s
			if math.IsNaN(x) || math.IsNaN(y) {
				continue
			}
			cmd := "L"
			if !started {
				cmd = "M"
				started = true
			}
			fmt.Fprintf(&p, `%s%.1f,%.1f `, cmd, px(x), py(y))
		}
		p.WriteString("Z")
		return p.String()
	}

	fmt.Fprintf(&b, `<path d="%s" fill="rgba(53,103,184,0.07)" stroke="#3567b8" stroke-width="2"/>`, locusPath(sd.CMF2))
	fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="#d1495b" stroke-width="1.6" stroke-dasharray="6 4"/>`, locusPath(sd.CMF10))

	drawArrows := func(rows []ColorRow, color, marker string, radius float64) {
		for _, row := range rows {
			x1, y1 := px(row.X2), py(row.Y2)
			x2, y2 := px(row.X10), py(row.Y10)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.4" marker-end="url(#%s)" opacity="0.85"/>`,
				x1, y1, x2, y2, color, marker)
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"><title>%s 2° (%.4f,%.4f) → 10° (%.4f,%.4f), ΔE00=%.2f</title></circle>`,
				x1, y1, radius, color, row.Name, row.X2, row.Y2, row.X10, row.Y10, row.DeltaE00)
		}
	}
	drawArrows(r.Spectral, "#d1495b", "arrowSpec", 3.2)
	drawArrows(r.Swatches, "#3567b8", "arrowSw", 2.8)

	// Legend (lower-right empty region of the diagram), with explicitly
	// positioned text.
	lx := float64(width - mr - 262)
	ly := float64(height - mb - 92)
	b.WriteString(`<g font-size="11.5" fill="#33404f">`)
	fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="252" height="88" rx="6" fill="rgba(255,255,255,0.92)" stroke="#d9dee7"/>`, lx, ly)
	legendRow := func(i int, color, dash, marker, label string) {
		cy := ly + 18 + float64(i)*19
		attrs := ""
		if dash != "" {
			attrs += ` stroke-dasharray="` + dash + `"`
		}
		if marker != "" {
			attrs += ` marker-end="url(#` + marker + `)"`
		}
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.8"%s/>`,
			lx+12, cy, lx+40, cy, color, attrs)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" dominant-baseline="middle">%s</text>`, lx+48, cy+0.5, label)
	}
	legendRow(0, "#3567b8", "", "", "2° 光谱轨迹")
	legendRow(1, "#d1495b", "6 4", "", "10° 光谱轨迹")
	legendRow(2, "#d1495b", "", "arrowSpec", "光谱样本位移（直接真值）")
	legendRow(3, "#3567b8", "", "arrowSw", "色板位移（经验估计）")
	b.WriteString(`</g>`)

	b.WriteString(`</svg>`)
	return b.String()
}
