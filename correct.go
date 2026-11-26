package flatfield

import "fmt"

// ApplyGain applies one spatial gain to all channels. Using the same scalar
// for R, G and B preserves channel ratios and therefore cannot introduce a
// new neutral/white-balance cast before output clipping.
func ApplyGain(c RGB, gain float64) RGB {
	return RGB{R: c.R * gain, G: c.G * gain, B: c.B * gain}
}

// CorrectInPlace applies the selected table gain to a linear frame.
func CorrectInPlace(f *Frame, t *Table) error {
	if f == nil {
		return fmt.Errorf("flatfield: nil frame")
	}
	if err := t.Validate(); err != nil {
		return err
	}
	w, h := f.Width, f.Height
	for y := 0; y < h; y++ {
		v := (float64(y) + 0.5) / float64(h)
		for x := 0; x < w; x++ {
			u := (float64(x) + 0.5) / float64(w)
			gain := t.GainAt(u, v)
			i := y*w + x
			f.Pix[i] = ApplyGain(f.Pix[i], gain)
		}
	}
	return nil
}

// CorrectFrame returns a corrected copy of f.
func CorrectFrame(f *Frame, t *Table) (*Frame, error) {
	if f == nil {
		return nil, fmt.Errorf("flatfield: nil frame")
	}
	out := &Frame{Width: f.Width, Height: f.Height, Pix: append([]RGB(nil), f.Pix...)}
	if err := CorrectInPlace(out, t); err != nil {
		return nil, err
	}
	return out, nil
}
