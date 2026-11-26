package mp4

import (
	"fmt"
	"io"
)

func readDecodeTimes(r io.ReaderAt, stts *Box) ([]uint64, []uint32, error) {
	if stts == nil {
		return nil, nil, fmt.Errorf("stts is missing")
	}
	if err := payloadLimit(stts, 8); err != nil {
		return nil, nil, err
	}
	count, err := readU32(r, stts.Payload()+4)
	if err != nil {
		return nil, nil, err
	}
	if int64(count)*8 > stts.End()-(stts.Payload()+8) {
		return nil, nil, fmt.Errorf("stts entry count %d exceeds box payload", count)
	}
	var times []uint64
	var durations []uint32
	pos := stts.Payload() + 8
	var decodeTime uint64
	for i := uint32(0); i < count; i++ {
		sampleCount, err := readU32(r, pos)
		if err != nil {
			return nil, nil, err
		}
		delta, err := readU32(r, pos+4)
		if err != nil {
			return nil, nil, err
		}
		if int64(len(times))+int64(sampleCount) > 50000000 {
			return nil, nil, fmt.Errorf("stts describes too many samples")
		}
		for j := uint32(0); j < sampleCount; j++ {
			times = append(times, decodeTime)
			durations = append(durations, delta)
			decodeTime += uint64(delta)
		}
		pos += 8
	}
	return times, durations, nil
}

func readCompositionOffsets(r io.ReaderAt, ctts *Box) ([]int64, error) {
	if ctts == nil {
		return nil, nil
	}
	if err := payloadLimit(ctts, 8); err != nil {
		return nil, err
	}
	version, err := fullBoxVersion(r, ctts)
	if err != nil {
		return nil, err
	}
	count, err := readU32(r, ctts.Payload()+4)
	if err != nil {
		return nil, err
	}
	if int64(count)*8 > ctts.End()-(ctts.Payload()+8) {
		return nil, fmt.Errorf("ctts entry count %d exceeds box payload", count)
	}
	var offsets []int64
	pos := ctts.Payload() + 8
	for i := uint32(0); i < count; i++ {
		sampleCount, err := readU32(r, pos)
		if err != nil {
			return nil, err
		}
		var offset int64
		if version == 0 {
			v, rerr := readU32(r, pos+4)
			offset, err = int64(v), rerr
		} else if version == 1 {
			var v int32
			v, err = readI32(r, pos+4)
			offset = int64(v)
		} else {
			return nil, fmt.Errorf("unsupported ctts version %d", version)
		}
		if err != nil {
			return nil, err
		}
		if int64(len(offsets))+int64(sampleCount) > 50000000 {
			return nil, fmt.Errorf("ctts describes too many samples")
		}
		for j := uint32(0); j < sampleCount; j++ {
			offsets = append(offsets, offset)
		}
		pos += 8
	}
	return offsets, nil
}

func parseSyncSamples(r io.ReaderAt, stss *Box, t *Track) error {
	if stss == nil {
		return nil
	}
	if err := payloadLimit(stss, 8); err != nil {
		return err
	}
	count, err := readU32(r, stss.Payload()+4)
	if err != nil {
		return err
	}
	if int64(count)*4 > stss.End()-(stss.Payload()+8) {
		return fmt.Errorf("track %d: stss entry count %d exceeds box payload", t.ID, count)
	}
	t.SyncSamples = make([]uint32, 0, count)
	pos := stss.Payload() + 8
	for i := uint32(0); i < count; i++ {
		n, err := readU32(r, pos)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("track %d: stss contains sample number 0", t.ID)
		}
		t.SyncSamples = append(t.SyncSamples, n-1)
		pos += 4
	}
	return nil
}
