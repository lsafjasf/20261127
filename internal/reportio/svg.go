package reportio

import (
	"clipforensic/internal/analyze"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteSVGs(dir string, report analyze.Report) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for fi, file := range report.Files {
		all := append(append([]analyze.Candidate{}, file.Candidates...), file.Artistic...)
		for _, cand := range all {
			name := fmt.Sprintf("file%d_%s_%s.svg", fi+1, cand.ID, cand.Status)
			path := filepath.Join(dir, name)
			if err := writeSVG(path, file, cand); err != nil {
				return paths, err
			}
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func writeSVG(path string, file analyze.FileReport, cand analyze.Candidate) error {
	const width, height = 1180, 380
	const left, right, top, bottom = 70, 30, 58, 55
	plotW := width - left - right
	plotH := height - top - bottom
	values := cand.Waveform.Channels[cand.Channel]
	mask := cand.Waveform.PlateauMask[cand.Channel]
	n := len(values)
	if n == 0 {
		return fmt.Errorf("empty waveform for %s", cand.ID)
	}
	xy := func(i int, v float64) (int, int) {
		x := left + int(float64(i)/float64(maxInt(1, n-1))*float64(plotW))
		y := top + plotH/2 - int(v*float64(plotH)*0.46)
		return x, y
	}

	var line strings.Builder
	for i, v := range values {
		x, y := xy(i, v)
		if i == 0 {
			fmt.Fprintf(&line, "M%d,%d", x, y)
		} else {
			fmt.Fprintf(&line, "L%d,%d", x, y)
		}
	}
	var plateaus strings.Builder
	for i, v := range values {
		if i < len(mask) && mask[i] {
			x, y := xy(i, v)
			fmt.Fprintf(&plateaus, `<circle cx="%d" cy="%d" r="2.4" fill="#d92d20"/>`, x, y)
		}
	}
	evIndex := cand.Waveform.EvidenceSample - cand.Waveform.FirstSample
	evX, _ := xy(clampInt(evIndex, 0, n-1), 0)
	title := fmt.Sprintf("%s %s %.3f–%.3fs", baseName(file.File), cand.ID, cand.StartSec, cand.EndSec)
	classification := cand.Classification

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
<style>text{font:13px sans-serif;fill:#1f2937}.small{font-size:11px;fill:#667085}.title{font-size:18px;font-weight:700}.ok{fill:#067647}.bad{fill:#b42318}</style>
<rect width="100%%" height="100%%" fill="white"/>
<text x="24" y="28" class="title">%s</text>
<text x="24" y="48">%s · confidence %.2f · plateau runs %d / longest %d / exact-FS %d</text>
<rect x="%d" y="%d" width="%d" height="%d" fill="#f8fafc" stroke="#d0d5dd"/>
<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#98a2b3" stroke-width="1"/>
<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#f79009" stroke-width="1.5" stroke-dasharray="5,4"/>
<path d="%s" fill="none" stroke="#175cd3" stroke-width="1.4"/>
%s
<text x="%d" y="%d" class="small">-1.0</text>
<text x="%d" y="%d" class="small">0</text>
<text x="%d" y="%d" class="small">+1.0</text>
<text x="%d" y="%d" class="small">sample %d (%.6fs)</text>
<text x="24" y="%d" class="%s">%s</text>
</svg>
`, width, height, width, height,
		xml(title), xml(classification), cand.Confidence, cand.Plateau.RunCount,
		cand.Plateau.LongestRun, cand.Plateau.ExactFullScaleSamples,
		left, top, plotW, plotH,
		left, top+plotH/2, left+plotW, top+plotH/2,
		evX, top, evX, top+plotH,
		line.String(), plateaus.String(),
		18, top+plotH+4, 34, top+plotH/2+4, 34, top+8,
		evX+6, top+plotH+18, cand.EvidenceSample,
		float64(cand.EvidenceSample)/float64(file.SampleRate),
		height-18, statusClass(cand.Status), xml(cand.Attribution))
	return os.WriteFile(path, []byte(svg), 0o644)
}

func statusClass(status string) string {
	if status == "incident" {
		return "bad"
	}
	if status == "artistic_distortion" {
		return "ok"
	}
	return ""
}

func baseName(p string) string { return filepath.Base(p) }

func xml(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
