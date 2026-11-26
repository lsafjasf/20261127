package laddercheck

import (
	"bytes"
	"testing"
)

func TestFragmentedMP4Parse(t *testing.T) {
	// Five tiny samples; only sample 0 is sync/IDR.
	nals := [][]byte{
		{0x65, 0xAA, 0xBB}, // IDR
		{0x41, 0xCC},
		{0x41, 0xCC},
		{0x41, 0xCC},
		{0x41, 0xCC},
	}
	var mdatPayload []byte
	sizes := make([]uint32, len(nals))
	for i, n := range nals {
		s := make([]byte, 4+len(n))
		s[3] = byte(len(n))
		copy(s[4:], n)
		sizes[i] = uint32(len(s))
		mdatPayload = append(mdatPayload, s...)
	}

	buildMoof := func(dataOffset uint32) *fbox {
		w := &fbw{}
		w.u32(uint32(len(nals)))
		w.u32(dataOffset) // trun data_offset (flag 0x1)
		for i := range nals {
			w.u32(1000) // duration
			w.u32(sizes[i])
			if i == 0 {
				w.u32(0x00000000) // sync
			} else {
				w.u32(0x00010000) // non-sync
			}
		}
		trun := fullBox("trun", 0, 0x000701, w.b) // data-offset|duration|size|sample-flags

		tfhdBody := &fbw{}
		tfhdBody.u32(1) // track id
		tfhd := fullBox("tfhd", 0, 0x020000, tfhdBody.b)
		tfdtBody := &fbw{}
		tfdtBody.u32(0)
		tfdt := fullBox("tfdt", 0, 0, tfdtBody.b)
		traf := container("traf", tfhd, tfdt, trun)
		return container("moof", traf)
	}

	moofMeasure := buildMoof(0).encode()
	moof := buildMoof(uint32(len(moofMeasure) + 8)).encode()

	trexBody := &fbw{}
	trexBody.u32(1)
	trexBody.u32(1)
	trexBody.u32(1000)
	trexBody.u32(0)
	trexBody.u32(0x00010000)
	trex := fullBox("trex", 0, 0, trexBody.b)
	mvex := container("mvex", trex)

	stbl := container("stbl", fixtureSTSD("avc", 320, 180))
	mdia := container("mdia", mdhdBox(30000, 5000), hdlrBox(), container("minf", stbl))
	trak := container("trak", tkhdBox(1, 5000, 320, 180), mdia)
	moov := container("moov", mvhdBox(1000, 167), trak, mvex)

	var data []byte
	data = append(data, ftypBox().encode()...)
	data = append(data, moov.encode()...)
	data = append(data, moof...)
	mdatHead := make([]byte, 8)
	mdatSize := uint32(8 + len(mdatPayload))
	mdatHead[0] = byte(mdatSize >> 24)
	mdatHead[1] = byte(mdatSize >> 16)
	mdatHead[2] = byte(mdatSize >> 8)
	mdatHead[3] = byte(mdatSize)
	copy(mdatHead[4:8], "mdat")
	data = append(data, mdatHead...)
	data = append(data, mdatPayload...)

	f, err := ParseMP4(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("ParseMP4 fragmented: %v", err)
	}
	if f.SampleCount != 5 {
		t.Fatalf("sample count=%d want 5 (warnings=%v)", f.SampleCount, f.Warnings)
	}
	if len(f.Keyframes) != 1 || f.Keyframes[0].Sample != 0 {
		t.Fatalf("keyframes=%+v want only sample 0", f.Keyframes)
	}
	if f.Keyframes[0].IDR == nil || !*f.Keyframes[0].IDR {
		t.Fatalf("fragment keyframe should classify as IDR")
	}
	if f.FPS.String() != "30" {
		t.Fatalf("fps=%s want 30", f.FPS)
	}
}
