package mp4seek

import (
	"fmt"
)

type nalClass struct {
	types      []int
	random     bool
	kind       string
	dependency string
	rasl       bool
}

func (f *File) scanTrackBitstream(track *Track) {
	if track.Handler != "vide" {
		return
	}
	supported := track.Codec == "avc1" || track.Codec == "avc3" ||
		track.Codec == "hvc1" || track.Codec == "hev1"
	if !supported {
		track.ActualSyncSamples = append([]uint32(nil), track.IndexedSyncSamples...)
		track.Warnings = append(track.Warnings,
			fmt.Sprintf("codec %q is not bitstream-scannable; actual sync points are unknown", track.Codec))
		return
	}
	if !track.configSeen {
		track.Warnings = append(track.Warnings,
			fmt.Sprintf("missing %s configuration; assuming %d-byte NAL lengths", codecConfigName(track), track.nalLength))
	}
	if track.nalLength < 1 || track.nalLength > 4 {
		track.Warnings = append(track.Warnings,
			fmt.Sprintf("invalid NAL length size %d; bitstream scan skipped", track.nalLength))
		return
	}

	complete := true
	inOpenGOP := false
	for i := range track.Samples {
		sample := &track.Samples[i]
		payload, err := f.sampleBytes(*sample)
		if err != nil {
			complete = false
			track.Warnings = append(track.Warnings, fmt.Sprintf("sample %d: %v", sample.Number, err))
			continue
		}
		nals, err := splitSampleNALs(payload, track.nalLength)
		if err != nil {
			complete = false
			track.Warnings = append(track.Warnings,
				fmt.Sprintf("sample %d: cannot split NAL units: %v", sample.Number, err))
			continue
		}
		class := classifyNALs(nals, track.hevc)
		sample.Kind = class.kind
		sample.Dependency = class.dependency
		sample.NALTypes = class.types
		sample.RandomAccess = class.random
		sample.BitstreamScanned = true
		if track.hevc {
			if class.random && class.kind == "cra" {
				inOpenGOP = true
			} else if class.rasl && inOpenGOP {
				sample.LeadingRASL = true
			} else if !class.rasl && len(class.types) > 0 {
				inOpenGOP = false
			}
		}
	}
	track.ScanComplete = complete
	track.ActualSyncSamples = sortedSampleNumbers(track.Samples, func(s Sample) bool {
		return s.RandomAccess
	})
	if complete && len(track.ActualSyncSamples) == 0 && len(track.Samples) > 0 {
		track.Warnings = append(track.Warnings, "no random access point was found in the scanned video samples")
	}
}

func (f *File) sampleBytes(sample Sample) ([]byte, error) {
	end := sample.Offset + uint64(sample.Size)
	if end < sample.Offset || end > uint64(len(f.data)) {
		return nil, fmt.Errorf("sample range [%d,%d) is outside file", sample.Offset, end)
	}
	return f.data[sample.Offset:end], nil
}

func codecConfigName(track *Track) string {
	if track.hevc {
		return "hvcC"
	}
	return "avcC"
}

func splitSampleNALs(payload []byte, lengthSize int) ([][]byte, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	if nals, err := splitLengthPrefixed(payload, lengthSize); err == nil {
		return nals, nil
	}
	if hasAnnexBStartCode(payload) {
		return splitAnnexB(payload), nil
	}
	return nil, fmt.Errorf("neither %d-byte length prefixes nor Annex B start codes are valid", lengthSize)
}

func splitLengthPrefixed(payload []byte, lengthSize int) ([][]byte, error) {
	var nals [][]byte
	for pos := 0; pos < len(payload); {
		if len(payload)-pos < lengthSize {
			return nil, fmt.Errorf("truncated length prefix at byte %d", pos)
		}
		var length uint32
		for i := 0; i < lengthSize; i++ {
			length = length<<8 | uint32(payload[pos+i])
		}
		pos += lengthSize
		if length == 0 {
			return nil, fmt.Errorf("zero-length NAL unit at byte %d", pos-lengthSize)
		}
		if uint64(length) > uint64(len(payload)-pos) {
			return nil, fmt.Errorf("NAL length %d exceeds remaining %d bytes", length, len(payload)-pos)
		}
		nals = append(nals, payload[pos:pos+int(length)])
		pos += int(length)
	}
	if len(nals) == 0 {
		return nil, fmt.Errorf("no NAL units")
	}
	return nals, nil
}

func hasAnnexBStartCode(payload []byte) bool {
	for i := 0; i+3 <= len(payload); i++ {
		if payload[i] == 0 && payload[i+1] == 0 && payload[i+2] == 1 {
			return true
		}
	}
	return false
}

func splitAnnexB(payload []byte) [][]byte {
	var starts []struct {
		at     int
		prefix int
	}
	for i := 0; i+3 <= len(payload); i++ {
		if payload[i] != 0 || payload[i+1] != 0 {
			continue
		}
		if payload[i+2] == 1 {
			starts = append(starts, struct {
				at     int
				prefix int
			}{at: i, prefix: 3})
			i += 2
		} else if i+4 <= len(payload) && payload[i+2] == 0 && payload[i+3] == 1 {
			starts = append(starts, struct {
				at     int
				prefix int
			}{at: i, prefix: 4})
			i += 3
		}
	}
	nals := make([][]byte, 0, len(starts))
	for i, start := range starts {
		begin := start.at + start.prefix
		end := len(payload)
		if i+1 < len(starts) {
			end = starts[i+1].at
		}
		for end > begin && payload[end-1] == 0 {
			end--
		}
		if end > begin {
			nals = append(nals, payload[begin:end])
		}
	}
	return nals
}

func classifyNALs(nals [][]byte, hevc bool) nalClass {
	if hevc {
		return classifyHEVC(nals)
	}
	return classifyAVC(nals)
}

func classifyAVC(nals [][]byte) nalClass {
	class := nalClass{kind: "empty", dependency: "none"}
	hasVCL := false
	for _, nal := range nals {
		if len(nal) == 0 || nal[0]&0x80 != 0 {
			continue
		}
		nalType := int(nal[0] & 0x1f)
		class.types = append(class.types, nalType)
		switch {
		case nalType == 5:
			class.random = true
			class.kind = "idr"
			hasVCL = true
		case nalType >= 1 && nalType <= 4:
			hasVCL = true
		}
	}
	switch {
	case class.random:
		class.dependency = "independent"
	case hasVCL:
		class.kind = "delta"
		class.dependency = "previous"
	case len(class.types) > 0:
		class.kind = "parameter-set"
	}
	return class
}

func classifyHEVC(nals [][]byte) nalClass {
	class := nalClass{kind: "empty", dependency: "none"}
	hasVCL := false
	for _, nal := range nals {
		if len(nal) < 2 || nal[0]&0x80 != 0 {
			continue
		}
		nalType := int((nal[0] >> 1) & 0x3f)
		class.types = append(class.types, nalType)
		switch {
		case nalType == 16 || nalType == 17 || nalType == 18:
			class.random = true
			class.kind = "bla"
			hasVCL = true
		case nalType == 19 || nalType == 20:
			class.random = true
			class.kind = "idr"
			hasVCL = true
		case nalType == 21:
			class.random = true
			class.kind = "cra"
			hasVCL = true
		case nalType == 22 || nalType == 23:
			class.random = true
			class.kind = "irap"
			hasVCL = true
		case nalType >= 0 && nalType <= 9:
			hasVCL = true
			if nalType == 8 || nalType == 9 {
				class.rasl = true
			}
		}
	}
	switch {
	case class.random:
		class.dependency = "independent"
	case class.rasl:
		class.kind = "rasl"
		class.dependency = "pre-rap"
	case hasVCL:
		class.kind = "delta"
		class.dependency = "previous"
	case len(class.types) > 0:
		class.kind = "parameter-set"
	}
	return class
}
