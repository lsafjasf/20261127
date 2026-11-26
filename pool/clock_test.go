package pool

// setClockForTest replaces the pool's wall clock with a controllable fake and
// returns a function that advances the fake time by the given unix seconds.
func (p *Pool) setClockForTest(start int64) func(advance int64) int64 {
	current := start
	p.nowFunc = func() int64 { return current }
	return func(advance int64) int64 {
		current += advance
		return current
	}
}
