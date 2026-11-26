package lighting

import (
	"fmt"
	"math"
)

type frameSample struct {
	timeS          float64
	global         float64
	background     float64
	foreground     float64
	motionFraction float64
	stableFraction float64
	motionEnergy   float64
	regions        []float64
}

type Analyzer struct {
	cfg Config

	width, height int
	gridW, gridH  int
	blockSize     int
	weights       []float64
	regions       []int

	prev            []float64
	backgroundModel []float64
	foregroundMask  []bool
	activity        []float64
	initialized     bool

	firstTime float64
	lastTime  float64
	samples   []frameSample
}

// NewAnalyzer creates a streaming analyzer. Frames must be supplied in
// monotonic timestamp order.
func NewAnalyzer(cfg Config) (*Analyzer, error) {
	cfg, err := cfg.validate()
	if err != nil {
		return nil, err
	}
	return &Analyzer{cfg: cfg}, nil
}

// AddFrame computes all per-frame evidence without retaining the image.
func (a *Analyzer) AddFrame(f Frame) error {
	if f.Width < 2 || f.Height < 2 {
		return fmt.Errorf("frame dimensions must be at least 2x2")
	}
	if len(f.Luma) != f.Width*f.Height {
		return fmt.Errorf("luma length %d does not match %dx%d", len(f.Luma), f.Width, f.Height)
	}
	if !a.initialized {
		a.width, a.height = f.Width, f.Height
		a.firstTime, a.lastTime = f.Timestamp, f.Timestamp
		a.setupGrid()
		a.prev = make([]float64, a.gridW*a.gridH)
		a.backgroundModel = make([]float64, a.gridW*a.gridH)
		a.foregroundMask = make([]bool, a.gridW*a.gridH)
		a.activity = make([]float64, a.gridW*a.gridH)
		a.initialized = true
	} else {
		if f.Width != a.width || f.Height != a.height {
			return fmt.Errorf("frame dimensions changed from %dx%d to %dx%d",
				a.width, a.height, f.Width, f.Height)
		}
		if f.Timestamp <= a.lastTime {
			return fmt.Errorf("frame timestamp %.6f is not after %.6f", f.Timestamp, a.lastTime)
		}
		a.lastTime = f.Timestamp
	}

	current := a.blockMeans(f.Luma)
	if len(a.samples) == 0 {
		a.prev = append(a.prev[:0], current...)
		a.backgroundModel = append(a.backgroundModel[:0], current...)
		a.samples = append(a.samples, a.measure(current, nil, nil))
		return nil
	}

	motionResidual := normalizeFrame(current, a.prev, a.gridW*a.gridH)
	modelResidual := normalizeFrame(current, a.backgroundModel, a.gridW*a.gridH)
	rawMotion := make([]bool, len(current))
	strongAnomaly := make([]bool, len(current))
	seeds := make([]bool, len(current))
	strongThreshold := math.Max(2*a.cfg.MotionThreshold, 16)
	for i, r := range motionResidual {
		if r > a.cfg.MotionThreshold {
			rawMotion[i] = true
		}
		a.activity[i] = a.cfg.StableActivityDecay*a.activity[i] +
			(1-a.cfg.StableActivityDecay)*r
		strongAnomaly[i] = modelResidual[i] > strongThreshold
		seeds[i] = rawMotion[i] || a.foregroundMask[i] && strongAnomaly[i]
	}
	foreground := connectedMask(seeds, strongAnomaly, a.gridW, a.gridH)
	foreground = dilate(foreground, a.gridW, a.gridH, 1)
	motionEdges := dilate(rawMotion, a.gridW, a.gridH, 1)
	for i := range foreground {
		dilatedActivity := a.activity[i] > 2*a.cfg.MotionThreshold
		foreground[i] = foreground[i] || motionEdges[i] || dilatedActivity
	}
	s := a.measure(current, foreground, modelResidual)
	for i := range a.backgroundModel {
		if !foreground[i] {
			a.backgroundModel[i] += 0.08 * (current[i] - a.backgroundModel[i])
		}
	}
	a.prev = append(a.prev[:0], current...)
	a.foregroundMask = append(a.foregroundMask[:0], foreground...)
	a.samples = append(a.samples, s)
	return nil
}

func (a *Analyzer) setupGrid() {
	a.blockSize = (a.width + a.cfg.MaxAnalysisWidth - 1) / a.cfg.MaxAnalysisWidth
	if a.blockSize < 1 {
		a.blockSize = 1
	}
	a.gridW = (a.width + a.blockSize - 1) / a.blockSize
	a.gridH = (a.height + a.blockSize - 1) / a.blockSize
	a.weights = make([]float64, a.gridW*a.gridH)
	a.regions = make([]int, a.gridW*a.gridH)
	for by := 0; by < a.gridH; by++ {
		y0, y1 := by*a.blockSize, min((by+1)*a.blockSize, a.height)
		for bx := 0; bx < a.gridW; bx++ {
			x0, x1 := bx*a.blockSize, min((bx+1)*a.blockSize, a.width)
			i := by*a.gridW + bx
			a.weights[i] = float64((y1 - y0) * (x1 - x0))
			cx := (x0 + x1) / 2
			cy := (y0 + y1) / 2
			rx := min(a.cfg.RegionCols-1, cx*a.cfg.RegionCols/a.width)
			ry := min(a.cfg.RegionRows-1, cy*a.cfg.RegionRows/a.height)
			a.regions[i] = ry*a.cfg.RegionCols + rx
		}
	}
}

func (a *Analyzer) blockMeans(luma []byte) []float64 {
	out := make([]float64, a.gridW*a.gridH)
	for by := 0; by < a.gridH; by++ {
		y0, y1 := by*a.blockSize, min((by+1)*a.blockSize, a.height)
		for bx := 0; bx < a.gridW; bx++ {
			x0, x1 := bx*a.blockSize, min((bx+1)*a.blockSize, a.width)
			sum := 0
			for y := y0; y < y1; y++ {
				row := luma[y*a.width+x0 : y*a.width+x1]
				for _, v := range row {
					sum += int(v)
				}
			}
			out[by*a.gridW+bx] = float64(sum) / a.weights[by*a.gridW+bx]
		}
	}
	return out
}

func (a *Analyzer) measure(current []float64, motion []bool, motionResidual []float64) frameSample {
	nRegions := a.cfg.RegionRows * a.cfg.RegionCols
	s := frameSample{regions: make([]float64, nRegions)}
	globalSum, globalWeight := 0.0, 0.0
	bgWeight, fgSum, fgWeight := 0.0, 0.0, 0.0
	motionWeight, residualWeight := 0.0, 0.0
	regionSum := make([]float64, nRegions)
	regionWeight := make([]float64, nRegions)
	regionBgWeight := make([]float64, nRegions)
	const bins = 512
	bgHist := make([]int, bins)
	regionHist := make([]int, bins*nRegions)

	for i, v := range current {
		w := a.weights[i]
		globalSum += v * w
		globalWeight += w
		regionSum[a.regions[i]] += v * w
		regionWeight[a.regions[i]] += w
		if motion != nil && motion[i] {
			fgSum += v * w
			fgWeight += w
			motionWeight += w
			if motionResidual != nil {
				residualWeight += math.Abs(motionResidual[i]) * w
			}
			continue
		}
		bgWeight += w
		regionBgWeight[a.regions[i]] += w
		bin := int(clamp(v, 0, 255) * 2)
		if bin >= bins {
			bin = bins - 1
		}
		bgHist[bin] += int(w)
		regionHist[a.regions[i]*bins+bin] += int(w)
	}

	s.timeS = 0
	if a.initialized {
		s.timeS = a.lastTime - a.firstTime
	}
	s.global = globalSum / globalWeight
	if bgWeight > 0 {
		s.background = histMedian(bgHist, int(bgWeight))
	} else {
		s.background = s.global
	}
	s.stableFraction = bgWeight / globalWeight
	if fgWeight > 0 {
		s.foreground = fgSum / fgWeight
		s.motionFraction = motionWeight / globalWeight
		s.motionEnergy = residualWeight / motionWeight
	}
	for r := range s.regions {
		if regionBgWeight[r] > 0 {
			s.regions[r] = histMedian(regionHist[r*bins:(r+1)*bins],
				int(regionBgWeight[r]))
		} else if regionWeight[r] > 0 {
			s.regions[r] = regionSum[r] / regionWeight[r]
		} else {
			s.regions[r] = s.global
		}
	}
	return s
}

func histMedian(hist []int, total int) float64 {
	if total <= 0 {
		return 0
	}
	target := total / 2
	seen := 0
	for i, count := range hist {
		if seen+count > target {
			return (float64(i) + 0.5) / 2
		}
		seen += count
	}
	return 255
}

func normalizeFrame(current, prev []float64, n int) []float64 {
	ratios := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		if prev[i] > 2 {
			ratios = append(ratios, clamp(current[i]/prev[i], 0.25, 4))
		}
	}
	gain := 1.0
	if len(ratios) > 0 {
		gain = medianRange(ratios, 0.25, 4, 751)
	}
	diff := make([]float64, n)
	for i := 0; i < n; i++ {
		diff[i] = current[i] - gain*prev[i]
	}
	offset := medianRange(diff, -255, 255, 1021)
	residual := make([]float64, n)
	for i := 0; i < n; i++ {
		residual[i] = math.Abs(diff[i] - offset)
	}
	return residual
}

func medianRange(values []float64, lo, hi float64, bins int) float64 {
	if len(values) == 0 || bins < 2 || hi <= lo {
		return 0
	}
	hist := make([]int, bins)
	step := (hi - lo) / float64(bins)
	for _, v := range values {
		idx := int((v - lo) / step)
		if idx < 0 {
			idx = 0
		} else if idx >= bins {
			idx = bins - 1
		}
		hist[idx]++
	}
	target := len(values) / 2
	seen := 0
	for i, count := range hist {
		if seen+count > target {
			return lo + (float64(i)+0.5)*step
		}
		seen += count
	}
	return hi
}

func dilate(mask []bool, w, h, radius int) []bool {
	out := make([]bool, len(mask))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !mask[y*w+x] {
				continue
			}
			y0, y1 := max(0, y-radius), min(h-1, y+radius)
			x0, x1 := max(0, x-radius), min(w-1, x+radius)
			for yy := y0; yy <= y1; yy++ {
				for xx := x0; xx <= x1; xx++ {
					out[yy*w+xx] = true
				}
			}
		}
	}
	return out
}

func connectedMask(seeds, allowed []bool, w, h int) []bool {
	out := make([]bool, len(seeds))
	queue := make([]int, 0, len(seeds))
	for i, seed := range seeds {
		if seed && allowed[i] && !out[i] {
			out[i] = true
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		i := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		x := i % w
		neighbors := [4]int{-1, 1, -w, w}
		for _, d := range neighbors {
			j := i + d
			if d == -1 && x == 0 || d == 1 && x == w-1 ||
				j < 0 || j >= len(out) {
				continue
			}
			if allowed[j] && !out[j] {
				out[j] = true
				queue = append(queue, j)
			}
		}
	}
	return out
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
