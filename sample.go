package hdrswitch

func int64Ptr(v int64) *int64 { return &v }

func sampleHLGMetadata() *ColorMetadata {
	return &ColorMetadata{
		Transfer: TransferHLG, TransferCode: 18,
		Primaries: PrimariesBT2020, PrimariesCode: 9,
		Matrix: "BT.2020_NCL", MatrixCode: 9,
		FullRange: boolPtr(false), BitDepth: 10,
		NominalPeakNits: ratPtr(MustRational(1000, 1)),
		MaxCLL:          ratPtr(MustRational(1000, 1)),
		MaxFALL:         ratPtr(MustRational(600, 1)),
		Mastering: &MasteringVolume{
			Primaries:        PrimariesBT2020,
			MaxLuminanceNits: ratPtr(MustRational(1000, 1)),
			MinLuminanceNits: ratPtr(MustRational(5, 1000)),
		},
	}
}

func samplePQMetadata() *ColorMetadata {
	return &ColorMetadata{
		Transfer: TransferPQ, TransferCode: 16,
		Primaries: PrimariesBT2020, PrimariesCode: 9,
		Matrix: "BT.2020_NCL", MatrixCode: 9,
		FullRange: boolPtr(false), BitDepth: 10,
		NominalPeakNits: ratPtr(MustRational(4000, 1)),
		MaxCLL:          ratPtr(MustRational(4000, 1)),
		MaxFALL:         ratPtr(MustRational(1500, 1)),
		Mastering: &MasteringVolume{
			Primaries:        PrimariesBT2020,
			MaxLuminanceNits: ratPtr(MustRational(4000, 1)),
			MinLuminanceNits: ratPtr(MustRational(5, 1000)),
		},
	}
}

func samplePTS(frame int64) *Rational {
	pts := MustRational(frame, 1).Div(MustRational(30, 1))
	return &pts
}

// SampleEvidence reproduces a 30 fps feature where 600 s is frame/AU 18000.
// The metadata changes there, but the preceding IDR is frame 17940 (598 s),
// so the switch AU is 60 access units inside a 120-frame GOP.
func SampleEvidence() Evidence {
	frameRate := MustRational(30, 1)
	closed := true

	return Evidence{
		SwitchAt:               MustRational(600, 1),
		ExpectedBeforeTransfer: TransferHLG,
		ExpectedAfterTransfer:  TransferPQ,
		FrameRate:              &frameRate,
		BitDepth:               10,
		FullRange:              boolPtr(false),
		Player: PlayerEvidence{
			UsesFirstFrameMetadata: true,
		},
		AccessUnits: []AccessUnit{
			{
				Index: 0, PTS: samplePTS(0), GOPID: "gop-0", IDR: true,
				RandomAccess: true, RAPType: "IDR_W_RADL", ClosedGOP: &closed,
				ParameterSets: true, Metadata: sampleHLGMetadata(),
			},
			{
				Index: 17940, PTS: samplePTS(17940), GOPID: "gop-17940", IDR: true,
				RandomAccess: true, RAPType: "IDR_W_RADL", ClosedGOP: &closed,
				ParameterSets: true,
			},
			{Index: 17999, PTS: samplePTS(17999), GOPID: "gop-17940"},
			{
				Index: 18000, PTS: samplePTS(18000), GOPID: "gop-17940",
				RAPType:  "TRAIL_R",
				Metadata: samplePQMetadata(),
				Note:     "ten-minute presentation cut; new VUI/SEI cannot safely take effect by global label alone",
			},
			{Index: 18001, PTS: samplePTS(18001), GOPID: "gop-17940"},
			{
				Index: 18060, PTS: samplePTS(18060), GOPID: "gop-18060", IDR: true,
				RandomAccess: true, RAPType: "IDR_N_LP", ClosedGOP: &closed,
				ParameterSets: true,
			},
		},
	}
}

// SafeSampleEvidence moves the HLG->PQ change onto an IDR/CVS boundary and
// includes the required VUI/SPS and HDR metadata update marker.
func SafeSampleEvidence() Evidence {
	evidence := SampleEvidence()
	closed := true
	for i := range evidence.AccessUnits {
		if evidence.AccessUnits[i].Index == 18000 {
			evidence.AccessUnits[i].IDR = true
			evidence.AccessUnits[i].RandomAccess = true
			evidence.AccessUnits[i].RAPType = "IDR_W_RADL"
			evidence.AccessUnits[i].ClosedGOP = &closed
			evidence.AccessUnits[i].ParameterSets = true
			evidence.AccessUnits[i].GOPID = "gop-18000"
		}
		if evidence.AccessUnits[i].Index == 18001 {
			evidence.AccessUnits[i].GOPID = "gop-18000"
			evidence.AccessUnits[i].ClosedGOP = &closed
		}
	}
	return evidence
}

// GamutChangeSampleEvidence is IDR-safe but also changes primaries at the
// cut, exercising gamut tracking separately from transfer and peak tracking.
func GamutChangeSampleEvidence() Evidence {
	evidence := SafeSampleEvidence()
	for i := range evidence.AccessUnits {
		if evidence.AccessUnits[i].Index != 18000 || evidence.AccessUnits[i].Metadata == nil {
			continue
		}
		evidence.AccessUnits[i].Metadata.Primaries = PrimariesP3D65
		evidence.AccessUnits[i].Metadata.PrimariesCode = 12
		if evidence.AccessUnits[i].Metadata.Mastering != nil {
			evidence.AccessUnits[i].Metadata.Mastering.Primaries = PrimariesP3D65
		}
	}
	return evidence
}

// ValidSampleEvidence is an IDR-safe, metadata-tracking player configuration;
// it is the clean delivery target and exits zero under -check.
func ValidSampleEvidence() Evidence {
	evidence := SafeSampleEvidence()
	evidence.Player.UsesFirstFrameMetadata = false
	evidence.Player.StickyMetadata = nil
	return evidence
}
