package dsp

import "math"

func NextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// MagnitudeSpectrum returns a one-sided magnitude spectrum of len(n/2)+1.
// The input is Hann-windowed and zero padded to a power of two.
func MagnitudeSpectrum(x []float64) (mags []float64, freqs []float64, fftSize int) {
	n := NextPow2(len(x))
	re := make([]float64, n)
	im := make([]float64, n)
	for i, v := range x {
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(len(x)-1))
		re[i] = v * w
	}
	FFT(re, im)
	mags = make([]float64, n/2+1)
	freqs = make([]float64, n/2+1)
	for i := range mags {
		mags[i] = math.Hypot(re[i], im[i]) / float64(len(x))
		freqs[i] = float64(i) / float64(n)
	}
	return mags, freqs, n
}

func FFT(re, im []float64) {
	n := len(re)
	if n == 0 || n&(n-1) != 0 {
		return
	}
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		half := length / 2
		step := -2 * math.Pi / float64(length)
		for start := 0; start < n; start += length {
			for k := 0; k < half; k++ {
				wr := math.Cos(step * float64(k))
				wi := math.Sin(step * float64(k))
				a := start + k
				b := a + half
				tr := wr*re[b] - wi*im[b]
				ti := wr*im[b] + wi*re[b]
				re[b] = re[a] - tr
				im[b] = im[a] - ti
				re[a] += tr
				im[a] += ti
			}
		}
	}
}

func Median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j-1] > cp[j]; j-- {
			cp[j-1], cp[j] = cp[j], cp[j-1]
		}
	}
	return cp[len(cp)/2]
}

func Percentile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j-1] > cp[j]; j-- {
			cp[j-1], cp[j] = cp[j], cp[j-1]
		}
	}
	pos := q * float64(len(cp)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return cp[lo]
	}
	t := pos - float64(lo)
	return cp[lo]*(1-t) + cp[hi]*t
}
