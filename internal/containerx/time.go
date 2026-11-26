package containerx

import "time"

// fixedTime gives archive members reproducible metadata so fixture archives
// are byte-reproducible across runs.
func fixedTime() time.Time {
	return time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
}
