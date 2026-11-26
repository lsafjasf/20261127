package seek

import (
	"fmt"
	"math"
	"sort"
	"time"

	"seekdiag/internal/mp4"
)

type Position struct {
	TrackID          uint32  `json:"track_id"`
	Kind             string  `json:"kind"`
	Codec            string  `json:"codec"`
	EditIndex        int     `json:"edit_index"`
	MediaTimeTicks   int64   `json:"media_time_ticks"`
	MediaTimescale   uint32  `json:"media_timescale"`
	SampleRate       uint32  `json:"sample_rate,omitempty"`
	SampleIndex      uint32  `json:"sample_index"`
	SamplePTSMS      float64 `json:"sample_pts_ms"`
	SampleDurationMS float64 `json:"sample_duration_ms"`
	TrimBeforeMS     float64 `json:"trim_before_ms"`
	FileOffset       uint64  `json:"file_offset"`
	SampleSize       uint32  `json:"sample_size"`
	Chunk            uint32  `json:"chunk"`
	OffsetInChunk    uint64  `json:"offset_in_chunk"`
	PCMStart         int64   `json:"pcm_start"`
	Keyframe         bool    `json:"keyframe"`
}

type Boundary struct {
	Previous *Position `json:"previous,omitempty"`
	Landing  *Position `json:"landing,omitempty"`
	Next     *Position `json:"next,omitempty"`
}

type TrackPlan struct {
	TrackID       uint32         `json:"track_id"`
	Kind          string         `json:"kind"`
	EditListFound bool           `json:"edit_list_found"`
	EditSpans     []mp4.EditSpan `json:"edit_spans,omitempty"`
	Boundary      Boundary       `json:"boundary"`
}

type Plan struct {
	TargetMS        float64     `json:"target_ms"`
	MovieTimescale  uint32      `json:"movie_timescale"`
	TargetMovieTick int64       `json:"target_movie_tick"`
	Tracks          []TrackPlan `json:"tracks"`
}

func At(f *mp4.File, target time.Duration) (*Plan, error) {
	if target < 0 {
		return nil, fmt.Errorf("seek target must not be negative")
	}
	if f.Movie.Timescale == 0 {
		return nil, fmt.Errorf("movie timescale is zero")
	}
	movieTick := int64(math.Round(float64(target) * float64(f.Movie.Timescale) / float64(time.Second)))
	plan := &Plan{
		TargetMS:        durationMS(target),
		MovieTimescale:  f.Movie.Timescale,
		TargetMovieTick: movieTick,
	}
	for _, track := range f.Tracks {
		if track.Kind() != "audio" && track.Kind() != "video" {
			continue
		}
		tp, err := planTrack(track, movieTick)
		if err != nil {
			return nil, err
		}
		plan.Tracks = append(plan.Tracks, tp)
	}
	return plan, nil
}

func planTrack(t *mp4.Track, targetMovie int64) (TrackPlan, error) {
	if err := t.Validate(); err != nil {
		return TrackPlan{}, err
	}
	spans := t.EditSpans()
	tp := TrackPlan{
		TrackID:       t.ID,
		Kind:          t.Kind(),
		EditListFound: len(t.Edits) > 0,
		EditSpans:     spans,
	}
	mediaTarget, _, ok := t.MovieToMedia(targetMovie)
	if !ok {
		return tp, fmt.Errorf("track %d: seek target is outside its edited presentation", t.ID)
	}
	landing := findLanding(t, mediaTarget)
	if landing < 0 || landing >= len(t.Samples) {
		return tp, fmt.Errorf("track %d: no sample covers media target %d", t.ID, mediaTarget)
	}
	next := landing + 1
	prev := landing - 1
	if prev < 0 || prev >= len(t.Samples) {
		prev = -1
	}
	if prev >= 0 {
		tp.Boundary.Previous = PositionAt(t, prev)
	}
	tp.Boundary.Landing = PositionAt(t, landing)
	if next >= 0 && next < len(t.Samples) {
		tp.Boundary.Next = PositionAt(t, next)
	}
	tp.Boundary.Landing.TrimBeforeMS = math.Max(0, targetMovieMS(t, targetMovie)-tp.Boundary.Landing.SamplePTSMS)
	return tp, nil
}

func findLanding(t *mp4.Track, mediaTarget int64) int {
	if t.Kind() == "video" {
		sync := t.SyncSamples
		idx := sort.Search(len(sync), func(i int) bool {
			si := int(sync[i])
			return t.Samples[si].PTS() > mediaTarget
		})
		if idx > 0 {
			return int(sync[idx-1])
		}
		return int(sync[0])
	}
	return sort.Search(len(t.Samples), func(i int) bool {
		return t.Samples[i].PTS() > mediaTarget
	}) - 1
}

func PositionAt(t *mp4.Track, index int) *Position {
	if index < 0 || index >= len(t.Samples) {
		return nil
	}
	s := t.Samples[index]
	moviePTS, _ := t.MediaToMovie(s.PTS())
	editIndex := editIndexFor(t, s.PTS())
	p := &Position{
		TrackID:          t.ID,
		Kind:             t.Kind(),
		Codec:            t.Codec,
		EditIndex:        editIndex,
		MediaTimeTicks:   s.PTS(),
		MediaTimescale:   t.Timescale,
		SampleRate:       t.SampleRate,
		SampleIndex:      s.Index,
		SamplePTSMS:      ticksMS(moviePTS, t.MovieTimescale),
		SampleDurationMS: ticksMS(int64(s.Duration), t.Timescale),
		FileOffset:       s.FileOffset,
		SampleSize:       s.Size,
		Chunk:            s.Chunk,
		OffsetInChunk:    s.OffsetInChunk,
		PCMStart:         moviePCM(t, moviePTS),
		Keyframe:         isSync(t, index),
	}
	return p
}

func editIndexFor(t *mp4.Track, mediaTick int64) int {
	for _, span := range t.EditSpans() {
		if span.Empty || span.Unsupported {
			continue
		}
		if mediaTick >= span.MediaStart && mediaTick < span.MediaEnd {
			return span.Index
		}
	}
	return -1
}

func moviePCM(t *mp4.Track, movieTick int64) int64 {
	rate := t.SampleRate
	if rate == 0 && t.Kind() == "audio" {
		rate = t.Timescale
	}
	if rate == 0 {
		return 0
	}
	return int64(float64(movieTick) * float64(rate) / float64(t.MovieTimescale))
}

func isSync(t *mp4.Track, sample int) bool {
	if t.Kind() != "video" {
		return true
	}
	i := sort.Search(len(t.SyncSamples), func(i int) bool { return int(t.SyncSamples[i]) >= sample })
	return i < len(t.SyncSamples) && int(t.SyncSamples[i]) == sample
}

func targetMovieMS(t *mp4.Track, targetMovie int64) float64 {
	return ticksMS(targetMovie, t.MovieTimescale)
}

func ticksMS(ticks int64, scale uint32) float64 {
	if scale == 0 {
		return 0
	}
	return float64(ticks) * 1000 / float64(scale)
}

func durationMS(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
