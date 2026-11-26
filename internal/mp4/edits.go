package mp4

import (
	"fmt"
	"io"
	"math"
)

func parseEditList(r io.ReaderAt, elst *Box, t *Track) error {
	version, err := fullBoxVersion(r, elst)
	if err != nil {
		return err
	}
	count, err := readU32(r, elst.Payload()+4)
	if err != nil {
		return fmt.Errorf("track %d: read elst count: %w", t.ID, err)
	}
	entrySize := int64(12)
	if version == 1 {
		entrySize = 20
	} else if version != 0 {
		return fmt.Errorf("unsupported elst version %d", version)
	}
	if count > 100000 || int64(count)*entrySize > elst.End()-(elst.Payload()+8) {
		return fmt.Errorf("track %d: invalid elst entry count %d", t.ID, count)
	}
	pos := elst.Payload() + 8
	for i := uint32(0); i < count; i++ {
		e := Edit{}
		if version == 0 {
			duration, rerr := readU32(r, pos)
			mediaTime, merr := readI32(r, pos+4)
			if rerr != nil || merr != nil {
				return fmt.Errorf("track %d: parse elst: %w", t.ID, firstErr(rerr, merr))
			}
			e.SegmentDuration = uint64(duration)
			e.MediaTime = int64(mediaTime)
		} else {
			duration, rerr := readU64(r, pos)
			mediaTime, merr := readI64(r, pos+8)
			if rerr != nil || merr != nil {
				return fmt.Errorf("track %d: parse elst: %w", t.ID, firstErr(rerr, merr))
			}
			e.SegmentDuration = duration
			e.MediaTime = mediaTime
		}
		rateInt, rerr := readU16(r, pos+entrySize-4)
		rateFrac, ferr := readU16(r, pos+entrySize-2)
		if rerr != nil || ferr != nil {
			return fmt.Errorf("track %d: parse elst rate: %w", t.ID, firstErr(rerr, ferr))
		}
		e.MediaRate = float64(int16(rateInt)) + float64(rateFrac)/65536
		if e.MediaRate == 0 {
			e.MediaRate = 0
		}
		t.Edits = append(t.Edits, e)
		pos += entrySize
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

type EditSpan struct {
	Index       int   `json:"index"`
	MediaStart  int64 `json:"media_start"`
	MediaEnd    int64 `json:"media_end"`
	MovieStart  int64 `json:"movie_start"`
	MovieEnd    int64 `json:"movie_end"`
	Empty       bool  `json:"empty"`
	Unsupported bool  `json:"unsupported"`
}

func (t *Track) EditSpans() []EditSpan {
	var spans []EditSpan
	if len(t.Edits) == 0 {
		spans = append(spans, EditSpan{
			MediaStart: 0,
			MediaEnd:   int64(t.Duration),
			MovieStart: 0,
			MovieEnd:   mediaToMovie(int64(t.Duration), t.Timescale, t.MovieTimescale),
		})
		return spans
	}
	var moviePos int64
	for i, e := range t.Edits {
		span := EditSpan{Index: i, MovieStart: moviePos}
		span.MovieEnd = moviePos + int64(e.SegmentDuration)
		if e.Empty() {
			span.Empty = true
		} else {
			span.MediaStart = e.MediaTime
			if math.Abs(e.MediaRate-1) > 1e-6 {
				span.Unsupported = true
				span.MediaEnd = e.MediaTime + int64(float64(e.SegmentDuration)*float64(t.Timescale)/float64(t.MovieTimescale)*e.MediaRate)
			} else {
				span.MediaEnd = e.MediaTime + movieToMedia(int64(e.SegmentDuration), t.MovieTimescale, t.Timescale)
			}
		}
		spans = append(spans, span)
		moviePos = span.MovieEnd
	}
	return spans
}

func (t *Track) MediaToMovie(mediaTick int64) (int64, bool) {
	for _, span := range t.EditSpans() {
		if span.Empty || span.Unsupported {
			continue
		}
		if mediaTick >= span.MediaStart && mediaTick < span.MediaEnd {
			delta := movieToMedia(mediaTick-span.MediaStart, t.Timescale, t.MovieTimescale)
			return span.MovieStart + delta, true
		}
	}
	return 0, false
}

func (t *Track) MovieToMedia(movieTick int64) (int64, int, bool) {
	for _, span := range t.EditSpans() {
		if movieTick >= span.MovieStart && movieTick < span.MovieEnd {
			if span.Empty || span.Unsupported {
				return 0, span.Index, false
			}
			target := mediaToMovie(movieTick-span.MovieStart, t.MovieTimescale, t.Timescale)
			return span.MediaStart + target, span.Index, true
		}
	}
	return 0, -1, false
}

func mediaToMovie(media int64, mediaScale, movieScale uint32) int64 {
	return int64(float64(media) * float64(movieScale) / float64(mediaScale))
}

func movieToMedia(movie int64, movieScale, mediaScale uint32) int64 {
	return int64(float64(movie) * float64(mediaScale) / float64(movieScale))
}
