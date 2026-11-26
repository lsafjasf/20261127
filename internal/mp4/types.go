package mp4

import (
	"fmt"
	"time"
)

const MovieTimescale = 1000

type File struct {
	Path   string
	Size   int64
	Movie  Movie
	Tracks []*Track
}

type Movie struct {
	Timescale uint32
	Duration  uint64
}

type Edit struct {
	SegmentDuration uint64
	MediaTime       int64
	MediaRate       float64
}

func (e Edit) Empty() bool { return e.MediaTime < 0 }

type Sample struct {
	Index           uint32
	DecodeTime      uint64
	Duration        uint32
	CompositionTime int64
	Size            uint32
	Chunk           uint32
	OffsetInChunk   uint64
	FileOffset      uint64
}

func (s Sample) PTS() int64 {
	return int64(s.DecodeTime) + s.CompositionTime
}

type Track struct {
	ID             uint32
	Handler        string
	Timescale      uint32
	Duration       uint64
	Language       string
	Codec          string
	Width          uint32
	Height         uint32
	ChannelCount   uint16
	SampleSize     uint16
	SampleRate     uint32
	Edits          []Edit
	Samples        []Sample
	SyncSamples    []uint32
	MovieTimescale uint32
}

func (t *Track) Kind() string {
	switch t.Handler {
	case "soun":
		return "audio"
	case "vide":
		return "video"
	default:
		if t.Handler == "" {
			return "unknown"
		}
		return t.Handler
	}
}

func (t *Track) Sample(index int) (Sample, bool) {
	if index < 0 || index >= len(t.Samples) {
		return Sample{}, false
	}
	return t.Samples[index], true
}

func (t *Track) DurationTime() time.Duration {
	if t.Timescale == 0 {
		return 0
	}
	return time.Duration(float64(t.Duration) / float64(t.Timescale) * float64(time.Second))
}

func (t *Track) Validate() error {
	if t.Timescale == 0 {
		return fmt.Errorf("track %d has zero media timescale", t.ID)
	}
	if len(t.Samples) == 0 {
		return fmt.Errorf("track %d has no samples", t.ID)
	}
	return nil
}
