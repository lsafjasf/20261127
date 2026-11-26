package clock

import (
	"errors"
	"sync"
	"time"
)

var ErrAlreadyAnchored = errors.New("timeline clock is already anchored; create a new clock for an explicit seek")

// Clock maps monotonic wall time to movie presentation time. Anchor is called
// exactly once for each explicit user seek. It deliberately has no periodic
// resync API: repeated corrections would hide a bad container or decoder PTS.
type Clock struct {
	mu            sync.Mutex
	anchored      bool
	anchorMediaMS float64
	anchorWall    time.Time
	rate          float64
	now           func() time.Time
}

func New(now func() time.Time) *Clock {
	if now == nil {
		now = time.Now
	}
	return &Clock{rate: 1, now: now}
}

func (c *Clock) Anchor(mediaMS float64, wall time.Time, rate float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.anchored {
		return ErrAlreadyAnchored
	}
	if rate <= 0 {
		rate = 1
	}
	c.anchored = true
	c.anchorMediaMS = mediaMS
	c.anchorWall = wall
	c.rate = rate
	return nil
}

func (c *Clock) MediaMS() (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.anchored {
		return 0, false
	}
	elapsed := c.now().Sub(c.anchorWall)
	return c.anchorMediaMS + float64(elapsed)/float64(time.Millisecond)*c.rate, true
}

func (c *Clock) Anchored() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.anchored
}

func (c *Clock) Rate() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rate
}

// AudioTimeline is the preferred audio master clock. The anchor is the first
// PCM frame that is actually audible after the seek trim, so AAC priming and
// edit-list trimming never enter the steady-state timeline.
type AudioTimeline struct {
	AnchorMovieMS float64
	AnchorPCM     int64
	SampleRate    uint32
}

func (a AudioTimeline) MovieMS(framesPlayed int64) float64 {
	if a.SampleRate == 0 {
		return a.AnchorMovieMS
	}
	return a.AnchorMovieMS + float64(framesPlayed-a.AnchorPCM)*1000/float64(a.SampleRate)
}

func (a AudioTimeline) PCMAt(movieMS float64) int64 {
	if a.SampleRate == 0 {
		return a.AnchorPCM
	}
	return a.AnchorPCM + int64((movieMS-a.AnchorMovieMS)*float64(a.SampleRate)/1000)
}
