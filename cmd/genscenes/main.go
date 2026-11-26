// genscenes renders synthetic test frames that reproduce the three failure
// modes blacklift must distinguish: cropped-lens fog, a noisy true night
// shot, and an intentional lifted-black grade.
package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"

	"blacklift/internal/frameio"
)

func main() {
	out := flag.String("out", "testdata", "output root")
	w := flag.Int("w", 320, "frame width")
	h := flag.Int("h", 180, "frame height")
	frames := flag.Int("frames", 4, "frames per scene")
	seed := flag.Int64("seed", 7, "rng seed")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))
	scenes := []struct {
		name        string
		baseFog     float64
		cornerExtra float64
		noise       float64
		lift        bool
		trueNight   bool
	}{
		{name: "fog_cropped", baseFog: 14.0 / 255.0, cornerExtra: 7.0 / 255.0, noise: 2.5 / 255.0},
		{name: "true_night", baseFog: 1.5 / 255.0, cornerExtra: 1.0 / 255.0, noise: 7.5 / 255.0, trueNight: true},
		{name: "intentional_lift", baseFog: 20.0 / 255.0, cornerExtra: 1.2 / 255.0, noise: 1.6 / 255.0, lift: true},
	}
	for _, sc := range scenes {
		dir := filepath.Join(*out, sc.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			panic(err)
		}
		for f := 0; f < *frames; f++ {
			fr := render(*w, *h, rng, sc.baseFog, sc.cornerExtra, sc.noise, sc.trueNight, sc.lift, f)
			path := filepath.Join(dir, fmt.Sprintf("frame_%03d.png", f))
			if err := fr.SavePNG(path); err != nil {
				panic(err)
			}
		}
		fmt.Println("wrote", dir)
	}
}

// render builds one frame containing: a smooth gradient sky, a mid-tone
// structure block, and large near-black regions so black-level behavior is
// visible. Fog is added as base + radial lens offset; noise is Gaussian.
func render(w, h int, rng *rand.Rand, baseFog, cornerExtra, noise float64, trueNight, lift bool, frame int) *frameio.Frame {
	n := w * h
	fr := &frameio.Frame{Width: w, Height: h, R: make([]float64, n), G: make([]float64, n), B: make([]float64, n), Y: make([]float64, n)}
	cx, cy := float64(w)/2, float64(h)/2
	maxR := math.Hypot(cx, cy)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			r := math.Hypot(float64(x)-cx, float64(y)-cy) / maxR

			scene := 0.0
			// dim sky gradient in the upper third
			if y < h/3 {
				scene = 0.10 * (1 - float64(y)/(float64(h)/3))
			}
			// a faint rectangular structure with real shadow detail (5-9%)
			if x > w/4 && x < w/2 && y > h/3 && y < 2*h/3 {
				stripes := 0.0
				if (x/8)%2 == 0 {
					stripes = 0.035
				}
				scene = 0.05 + stripes
			}
			// sparse dim lights in night scenes (must survive)
			if trueNight && (x*31+y*17+frame*101)%977 == 0 {
				scene = 0.35 + 0.05*math.Sin(float64(frame))
			}

			fog := baseFog + cornerExtra*r*r
			// intentional grade is spatially flat; accidents keep lens shape
			if lift {
				fog = baseFog + cornerExtra*0.15*r*r
			}
			v := scene + fog + rng.NormFloat64()*noise
			if trueNight {
				// a fraction of true night pixels sit at the sensor floor
				if rng.Float64() < 0.04 {
					v = rng.NormFloat64() * noise
				}
			}
			v = clamp(v)
			// slight cool cast so color-channel clipping is exercised
			rv := clamp(v + 0.004)
			bv := clamp(v - 0.004)
			fr.R[i], fr.G[i], fr.B[i] = rv, v, bv
			fr.Y[i] = 0.2126*rv + 0.7152*v + 0.0722*bv
		}
	}
	return fr
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
