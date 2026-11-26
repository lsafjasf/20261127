package flatfield

import (
	"fmt"
	"math"
)

func (t *Table) gridIndex(u, v float64) (int, int, float64, float64) {
	u = clampFloat(u, 0, 1)
	v = clampFloat(v, 0, 1)
	fx := u*float64(t.GridWidth) - 0.5
	fy := v*float64(t.GridHeight) - 0.5
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	tx := fx - float64(x0)
	ty := fy - float64(y0)
	if x0 < 0 {
		x0, tx = 0, 0
	}
	if y0 < 0 {
		y0, ty = 0, 0
	}
	x1 := x0 + 1
	y1 := y0 + 1
	if x0 >= t.GridWidth-1 {
		x0, x1, tx = t.GridWidth-1, t.GridWidth-1, 0
	}
	if x1 >= t.GridWidth {
		x1 = t.GridWidth - 1
	}
	if y0 >= t.GridHeight-1 {
		y0, y1, ty = t.GridHeight-1, t.GridHeight-1, 0
	}
	if y1 >= t.GridHeight {
		y1 = t.GridHeight - 1
	}
	return x0, y0, tx, ty
}

func sampleGridArray(a []float64, gw, gh int, u, v float64) float64 {
	if gw < 1 || gh < 1 || len(a) != gw*gh {
		return math.NaN()
	}
	u = clampFloat(u, 0, 1)
	v = clampFloat(v, 0, 1)
	fx := u*float64(gw) - 0.5
	fy := v*float64(gh) - 0.5
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	tx := fx - float64(x0)
	ty := fy - float64(y0)
	if x0 < 0 {
		x0, tx = 0, 0
	}
	if y0 < 0 {
		y0, ty = 0, 0
	}
	x1 := x0 + 1
	y1 := y0 + 1
	if x0 >= gw-1 {
		x0, x1, tx = gw-1, gw-1, 0
	}
	if x1 >= gw {
		x1 = gw - 1
	}
	if y0 >= gh-1 {
		y0, y1, ty = gh-1, gh-1, 0
	}
	if y1 >= gh {
		y1 = gh - 1
	}
	at := func(x, y int) float64 { return a[y*gw+x] }
	top := at(x0, y0)*(1-tx) + at(x1, y0)*tx
	bottom := at(x0, y1)*(1-tx) + at(x1, y1)*tx
	return top*(1-ty) + bottom*ty
}

// SampleArray performs bilinear interpolation in normalized coordinates.
func (t *Table) SampleArray(a []float64, u, v float64) float64 {
	return sampleGridArray(a, t.GridWidth, t.GridHeight, u, v)
}

// ResponseAt returns the selected normalized response at normalized coords.
func (t *Table) ResponseAt(u, v float64) float64 {
	return t.SampleArray(t.Response, u, v)
}

// GainAt returns the selected correction gain at normalized coords.
func (t *Table) GainAt(u, v float64) float64 {
	return t.SampleArray(t.Gain, u, v)
}

// ComponentResponseAt samples one decomposed response.
func (t *Table) ComponentResponseAt(kind Kind, u, v float64) (float64, error) {
	var a []float64
	switch kind {
	case KindObserved:
		a = t.ObservedResponse
		if len(a) == 0 && t.Kind == KindObserved {
			a = t.Response
		}
	case KindGradient:
		a = t.GradientResponse
		if len(a) == 0 && t.Kind == KindGradient {
			a = t.Response
		}
	case KindLens:
		a = t.LensResponse
		if len(a) == 0 && t.Kind == KindLens {
			a = t.Response
		}
	case KindPanel:
		a = t.PanelResponse
		if len(a) == 0 && t.Kind == KindPanel {
			a = t.Response
		}
	case KindPanelResidual:
		a = t.PanelResidualResponse
		if len(a) == 0 {
			if len(t.PanelResponse) > 0 && len(t.GradientResponse) > 0 {
				panel := t.SampleArray(t.PanelResponse, u, v)
				grad := t.SampleArray(t.GradientResponse, u, v)
				if grad > 0 {
					return panel / grad, nil
				}
			}
			if t.Kind == KindPanelResidual {
				return t.SampleArray(t.Response, u, v), nil
			}
			return 0, fmt.Errorf("flatfield: panel_residual response is absent from table")
		}
	case KindDevice:
		if t.Kind == KindDevice && len(t.Response) > 0 {
			return t.SampleArray(t.Response, u, v), nil
		}
		if len(t.ObservedResponse) > 0 {
			return t.SampleArray(t.ObservedResponse, u, v), nil
		}
		if len(t.LensResponse) == 0 || len(t.PanelResponse) == 0 || len(t.ObservedResponse) == 0 {
			return 0, fmt.Errorf("flatfield: device components are absent")
		}
		return t.SampleArray(t.LensResponse, u, v) * t.SampleArray(t.PanelResponse, u, v), nil
	case KindDeviceNoGradient:
		if len(t.ObservedResponse) > 0 && len(t.GradientResponse) > 0 {
			obs := t.SampleArray(t.ObservedResponse, u, v)
			grad := t.SampleArray(t.GradientResponse, u, v)
			if grad > 0 {
				return obs / grad, nil
			}
		}
		if len(t.LensResponse) > 0 && len(t.PanelResidualResponse) > 0 {
			return t.SampleArray(t.LensResponse, u, v) * t.SampleArray(t.PanelResidualResponse, u, v), nil
		}
		if t.Kind == KindDeviceNoGradient {
			return t.SampleArray(t.Response, u, v), nil
		}
		return 0, fmt.Errorf("flatfield: device_no_gradient components are absent")
	default:
		return 0, fmt.Errorf("flatfield: unknown component %q", kind)
	}
	if len(a) == 0 {
		return 0, fmt.Errorf("flatfield: %q response is absent from table", kind)
	}
	return t.SampleArray(a, u, v), nil
}

func (t *Table) componentArray(kind Kind) ([]float64, error) {
	n := t.GridWidth * t.GridHeight
	out := make([]float64, n)
	for i := range out {
		u := (float64(i%t.GridWidth) + 0.5) / float64(t.GridWidth)
		v := (float64(i/t.GridWidth) + 0.5) / float64(t.GridHeight)
		r, err := t.ComponentResponseAt(kind, u, v)
		if err != nil {
			return nil, err
		}
		out[i] = r
	}
	return out, nil
}

// ForKind returns a copy whose primary Response/Gain select one component.
// A positive maxGain limits brightening; zero uses the table default and a
// negative value disables gain limiting.
func (t *Table) ForKind(kind Kind, maxGain float64) (*Table, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	resp, err := t.componentArray(kind)
	if err != nil {
		return nil, err
	}
	if maxGain == 0 {
		maxGain = t.MaxGain
	}
	out := *t
	out.Kind = kind
	out.Response = resp
	out.Gain = make([]float64, len(resp))
	out.MaxGain = maxGain
	for i, r := range resp {
		gain := 1.0 / r
		if maxGain > 0 && gain > maxGain {
			gain = maxGain
		}
		out.Gain[i] = gain
	}
	return &out, nil
}

// ResampleGain returns a gain matrix for exact target pixel dimensions.
// The returned slice is row-major with length width*height.
func (t *Table) ResampleGain(width, height int) ([]float64, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("flatfield: invalid resample dimensions %dx%d", width, height)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	out := make([]float64, width*height)
	for y := 0; y < height; y++ {
		v := (float64(y) + 0.5) / float64(height)
		for x := 0; x < width; x++ {
			u := (float64(x) + 0.5) / float64(width)
			out[y*width+x] = t.GainAt(u, v)
		}
	}
	return out, nil
}
