package avsync

import (
	"math"
	"sort"
)

func matchPoints(pulses, events []float64, pred lineFit, anchorMS, windowMS float64) []pair {
	if len(pulses) == 0 || len(events) == 0 || windowMS <= 0 {
		return nil
	}
	n, m := len(pulses), len(events)
	const inf = math.MaxFloat64 / 4
	const gapPenalty = 0.6

	width := m + 1
	dp := make([]float64, (n+1)*width)
	back := make([]uint8, (n+1)*width)
	for i := 0; i <= n; i++ {
		for j := 0; j <= m; j++ {
			dp[i*width+j] = inf
		}
	}
	dp[0] = 0
	for i := 1; i <= n; i++ {
		dp[i*width] = float64(i) * gapPenalty
		back[i*width] = 1
	}
	for j := 1; j <= m; j++ {
		dp[j] = float64(j) * gapPenalty
		back[j] = 2
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			idx := i*width + j
			best := dp[(i-1)*width+j] + gapPenalty
			move := uint8(1)
			if v := dp[i*width+j-1] + gapPenalty; v < best {
				best = v
				move = 2
			}

			resid := events[j-1] - pulses[i-1] - pred.predict(pulses[i-1]-anchorMS)
			if math.Abs(resid) <= windowMS {
				scaled := resid / windowMS
				cost := scaled * scaled
				if cost > 1 {
					cost = 1
				}
				if v := dp[(i-1)*width+j-1] + cost; v < best {
					best = v
					move = 3
				}
			}
			dp[idx] = best
			back[idx] = move
		}
	}

	out := make([]pair, 0, minInt(n, m))
	i, j := n, m
	for i > 0 || j > 0 {
		move := back[i*width+j]
		switch move {
		case 1:
			i--
		case 2:
			j--
		case 3:
			out = append(out, pair{pulseMS: pulses[i-1], eventMS: events[j-1]})
			i--
			j--
		default:
			return nil
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].pulseMS < out[b].pulseMS })
	return out
}

func initialOffset(pulses, events []float64, windowMS float64) float64 {
	deltas := make([]float64, 0, len(pulses))
	for _, p := range pulses {
		j := sort.SearchFloat64s(events, p)
		best := math.Inf(1)
		for _, k := range []int{j - 1, j} {
			if k < 0 || k >= len(events) {
				continue
			}
			d := events[k] - p
			if math.Abs(d) < math.Abs(best) {
				best = d
			}
		}
		if math.Abs(best) <= windowMS {
			deltas = append(deltas, best)
		}
	}
	if len(deltas) == 0 {
		return median(events) - median(pulses)
	}
	return median(deltas)
}
