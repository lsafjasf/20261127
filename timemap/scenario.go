package timemap

// DefaultScenario builds the three-track case described in the task:
// subtitle, audio and picture all stamp the same absolute edit time, while
// the as-built pipeline introduces a one-unit slip per track.
func DefaultScenario() (EditResolution, Rate, []TrackConfig, []TimelinePoint) {
	edit := EditResolution{TicksPerSecond: 1_000_000} // 1 tick = 1 microsecond
	refresh := NewRateInt(60)                         // output display vsync, 60 Hz

	videoRate := NewRate(24000, 1001) // 23.976... fps, exact NTSC rate
	audioRate := NewRateInt(48000)    // 48 kHz PCM
	subRate := NewRate(24000, 1001)   // subtitles locked to the picture rate
	wrongSubRate := NewRateInt(24)    // as-built treats the timeline as flat 24 fps

	tracks := []TrackConfig{
		{
			Name:             "video",
			MediaRate:        videoRate,
			DisplayRule:      RoundFloor,
			BuiltUsesFloat:   true,
			BuiltMediaRule:   RoundNearest, // math.Round instead of floor
			BuiltDisplayRule: RoundFloor,
		},
		{
			Name:             "audio",
			MediaRate:        audioRate,
			DisplayRule:      RoundFloor,
			BuiltUsesFloat:   true,
			BuiltMediaRule:   RoundNearest, // math.Round on sample numbers
			BuiltDisplayRule: RoundFloor,
		},
		{
			Name:             "subtitle",
			MediaRate:        subRate,
			DisplayRule:      RoundFloor,
			BuiltUsesFloat:   true,
			BuiltMediaRate:   &wrongSubRate, // hard-coded 24 fps, ignores 1001 timebase
			BuiltMediaRule:   RoundFloor,
			BuiltDisplayRule: RoundFloor,
		},
	}

	points := []TimelinePoint{
		{Label: "T0-origin", Tick: 0},
		{Label: "T1-first-frame-boundary", Tick: 41_708},
		{Label: "T2-cue-1s", Tick: 1_000_000},
		{Label: "T3-cut-2s", Tick: 2_000_000},
		{Label: "T4-audio-block-1.02s", Tick: 1_020_000},
		{Label: "T5-checkpoint-10s", Tick: 10_000_000},
		{Label: "T6-subtitle-drift-20.921s", Tick: 20_921_000},
		{Label: "T7-40s", Tick: 40_000_000},
		{Label: "T8-60s", Tick: 60_000_000},
	}

	return edit, refresh, tracks, points
}
