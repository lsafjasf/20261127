package diag

func ticksToMS(ticks int64, scale uint32) float64 {
	if scale == 0 {
		return 0
	}
	return float64(ticks) * 1000 / float64(scale)
}
