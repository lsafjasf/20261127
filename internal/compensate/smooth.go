package compensate

// SmoothMaps applies zero-phase temporal smoothing to per-frame raw
// compensation maps: a forward EMA followed by a backward EMA removes frame
// flicker without introducing a one-sided temporal lag/ghosting.
func SmoothMaps(maps [][]float64, alpha float64) {
	if len(maps) < 2 || alpha >= 1 {
		return
	}
	if alpha <= 0 {
		alpha = 0.5
	}
	n := len(maps[0])
	fwd := make([][]float64, len(maps))
	fwd[0] = append([]float64(nil), maps[0]...)
	for f := 1; f < len(maps); f++ {
		fwd[f] = make([]float64, n)
		for i := 0; i < n; i++ {
			fwd[f][i] = alpha*maps[f][i] + (1-alpha)*fwd[f-1][i]
		}
	}
	prev := fwd[len(maps)-1]
	for i := 0; i < n; i++ {
		maps[len(maps)-1][i] = prev[i]
	}
	for f := len(maps) - 2; f >= 0; f-- {
		cur := make([]float64, n)
		for i := 0; i < n; i++ {
			cur[i] = alpha*fwd[f][i] + (1-alpha)*prev[i]
		}
		maps[f] = cur
		prev = cur
	}
}
