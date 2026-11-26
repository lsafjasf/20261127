package hdrswitch

import (
	"fmt"
	"math"
	"strings"
)

type gamutProfile struct {
	Name string
	Code int
	R    Point
	G    Point
	B    Point
	W    Point
}

var gamutProfiles = map[string]gamutProfile{
	PrimariesBT709: {
		Name: PrimariesBT709, Code: 1,
		R: Point{X: 0.640, Y: 0.330}, G: Point{X: 0.300, Y: 0.600},
		B: Point{X: 0.150, Y: 0.060}, W: Point{X: 0.3127, Y: 0.3290},
	},
	PrimariesBT2020: {
		Name: PrimariesBT2020, Code: 9,
		R: Point{X: 0.708, Y: 0.292}, G: Point{X: 0.170, Y: 0.797},
		B: Point{X: 0.131, Y: 0.046}, W: Point{X: 0.3127, Y: 0.3290},
	},
	PrimariesP3D65: {
		Name: PrimariesP3D65, Code: 12,
		R: Point{X: 0.680, Y: 0.320}, G: Point{X: 0.265, Y: 0.690},
		B: Point{X: 0.150, Y: 0.060}, W: Point{X: 0.3127, Y: 0.3290},
	},
	PrimariesDCIP3: {
		Name: PrimariesDCIP3, Code: 11,
		R: Point{X: 0.680, Y: 0.320}, G: Point{X: 0.265, Y: 0.690},
		B: Point{X: 0.150, Y: 0.060}, W: Point{X: 0.314, Y: 0.351},
	},
}

func boolValue(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func boolPtr(v bool) *bool { return &v }

func ratPtr(v Rational) *Rational { return &v }

func normalizeTransfer(meta *ColorMetadata) (string, int, error) {
	key := strings.ToUpper(strings.TrimSpace(meta.Transfer))
	key = strings.NewReplacer("-", "", "_", "", " ", "").Replace(key)
	code := meta.TransferCode

	switch key {
	case "HLG", "ARIBB67", "BT2100HLG":
		return TransferHLG, 18, nil
	case "PQ", "SMPTE2084", "ST2084", "BT2100PQ":
		return TransferPQ, 16, nil
	case "":
		switch code {
		case 18:
			return TransferHLG, 18, nil
		case 16:
			return TransferPQ, 16, nil
		case 0:
			return "", 0, fmt.Errorf("color metadata requires transfer (HLG/PQ or H.273 code 18/16)")
		default:
			return "", code, fmt.Errorf("unsupported transfer_code %d for this HDR switch report", code)
		}
	default:
		return "", code, fmt.Errorf("unsupported transfer %q", meta.Transfer)
	}
}

func normalizePrimaries(meta *ColorMetadata) (gamutProfile, error) {
	key := strings.ToUpper(strings.TrimSpace(meta.Primaries))
	key = strings.NewReplacer(".", "", "-", "", "_", "", " ", "").Replace(key)

	switch key {
	case "BT709", "REC709", "HDTV":
		return gamutProfiles[PrimariesBT709], nil
	case "BT2020", "REC2020", "BT2100", "REC2100":
		return gamutProfiles[PrimariesBT2020], nil
	case "P3D65", "DISPLAYP3", "SMPTE432":
		return gamutProfiles[PrimariesP3D65], nil
	case "DCIP3", "P3", "SMPTE431", "RP431":
		return gamutProfiles[PrimariesDCIP3], nil
	case "":
		switch meta.PrimariesCode {
		case 1:
			return gamutProfiles[PrimariesBT709], nil
		case 9:
			return gamutProfiles[PrimariesBT2020], nil
		case 11:
			return gamutProfiles[PrimariesDCIP3], nil
		case 12:
			return gamutProfiles[PrimariesP3D65], nil
		case 0:
			return gamutProfile{}, fmt.Errorf("color metadata requires color_primaries")
		default:
			return gamutProfile{}, fmt.Errorf("unsupported primaries_code %d", meta.PrimariesCode)
		}
	default:
		return gamutProfile{}, fmt.Errorf("unsupported color_primaries %q", meta.Primaries)
	}
}

func componentCodeRange(bitDepth int, fullRange bool) (CodeRange, error) {
	if bitDepth == 0 {
		bitDepth = 10
	}
	if bitDepth != 8 && bitDepth != 10 && bitDepth != 12 {
		return CodeRange{}, fmt.Errorf("bit_depth must be 8, 10, or 12, got %d", bitDepth)
	}
	maxCode := 1<<bitDepth - 1
	if fullRange {
		whole := IntRange{Min: 0, Max: maxCode, Unit: "code value", Basis: "full range"}
		return CodeRange{BitDepth: bitDepth, Quantization: "full", Luma: whole, Chroma: whole, RGB: whole}, nil
	}
	shift := uint(bitDepth - 8)
	black := 16 << shift
	lumaPeak := 235 << shift
	chromaPeak := 240 << shift
	return CodeRange{
		BitDepth:     bitDepth,
		Quantization: "limited",
		Luma:         IntRange{Min: black, Max: lumaPeak, Unit: "code value", Basis: "BT.2100/BT.601 narrow range Y/R/G/B nominal"},
		RGB:          IntRange{Min: black, Max: lumaPeak, Unit: "code value", Basis: "narrow range R/G/B nominal"},
		Chroma:       IntRange{Min: black, Max: chromaPeak, Unit: "code value", Basis: "narrow range Cb/Cr; neutral at 512 in 10-bit"},
	}, nil
}

func pointerLess(a, b *Rational) bool {
	return a != nil && b != nil && a.Cmp(*b) < 0
}

func buildEnvelope(meta ColorMetadata, defaultBitDepth int, defaultFullRange bool) (LegalEnvelope, error) {
	transfer, code, err := normalizeTransfer(&meta)
	if err != nil {
		return LegalEnvelope{}, err
	}
	primaryQuery := meta
	if primaryQuery.Primaries == "" && primaryQuery.PrimariesCode == 0 &&
		meta.Mastering != nil && meta.Mastering.Primaries != "" {
		primaryQuery.Primaries = meta.Mastering.Primaries
	}
	profile, err := normalizePrimaries(&primaryQuery)
	if err != nil {
		if meta.Mastering == nil || len(meta.Mastering.PrimariesXY) != 3 {
			return LegalEnvelope{}, err
		}
		profile = gamutProfile{
			Name: "custom_explicit_mastering_volume",
			Code: meta.PrimariesCode,
			R:    meta.Mastering.PrimariesXY[0],
			G:    meta.Mastering.PrimariesXY[1],
			B:    meta.Mastering.PrimariesXY[2],
			W:    Point{X: 0.3127, Y: 0.3290},
		}
	}
	bitDepth := meta.BitDepth
	if bitDepth == 0 {
		bitDepth = defaultBitDepth
	}
	fullRange := defaultFullRange
	if meta.FullRange != nil {
		fullRange = *meta.FullRange
	}
	codes, err := componentCodeRange(bitDepth, fullRange)
	if err != nil {
		return LegalEnvelope{}, err
	}

	gamut := GamutRange{
		Primaries: profile.Name, PrimariesCode: profile.Code,
		Red: profile.R, Green: profile.G, Blue: profile.B, White: profile.W,
		CIECoordinates: FloatRange{Min: 0, Max: 1, Unit: "CIE 1931 xy", Basis: "encoding coordinate bounds; physical colors additionally lie in the spectral locus/primary triangle"},
		SEICoordinates: IntRange{Min: 0, Max: 50000, Unit: "SEI coordinate code", Basis: "xy * 50000, unit 0.00002"},
	}
	if meta.Mastering != nil {
		if len(meta.Mastering.PrimariesXY) == 3 {
			gamut.Red = meta.Mastering.PrimariesXY[0]
			gamut.Green = meta.Mastering.PrimariesXY[1]
			gamut.Blue = meta.Mastering.PrimariesXY[2]
			gamut.Primaries = firstNonEmpty(meta.Mastering.Primaries, "explicit_mastering_volume")
		}
		if meta.Mastering.WhitePoint != nil {
			gamut.White = *meta.Mastering.WhitePoint
		}
	}

	peak := PeakSummary{
		NominalPeakNits:      meta.NominalPeakNits,
		MaxCLL:               meta.MaxCLL,
		MaxFALL:              meta.MaxFALL,
		EffectiveCeilingNits: ZeroRational(),
		CeilingBasis:         "unknown",
	}
	if meta.Mastering != nil {
		peak.MasteringMaxNits = meta.Mastering.MaxLuminanceNits
		peak.MasteringMinNits = meta.Mastering.MinLuminanceNits
	}

	env := LegalEnvelope{
		Transfer: transfer, H273TransferCode: code, ComponentCodes: codes,
		NonlinearSignal: FloatRange{Min: 0, Max: 1, Unit: "normalized E'", Basis: "BT.2100 signal container; legal codes map black to 0 and nominal peak to 1"},
		Gamut:           gamut, Peak: peak,
	}

	switch transfer {
	case TransferPQ:
		env.AbsoluteLuminanceFixed = true
		env.RepresentationLuminance = &FloatRange{Min: 0, Max: 10000, Unit: "cd/m² (nits)", Basis: "SMPTE ST 2084 PQ; E'=1 is normalized luminance 10000 nits"}
		env.Peak.PQRepresentationMax = 10000
		ceiling := MustRational(10000, 1)
		basis := "PQ representation ceiling"
		for _, candidate := range []*Rational{peak.MasteringMaxNits, peak.MaxCLL, peak.NominalPeakNits} {
			if candidate != nil && candidate.Cmp(ceiling) < 0 && candidate.Num > 0 {
				ceiling = *candidate
			}
		}
		if peak.MasteringMaxNits != nil || peak.MaxCLL != nil || peak.NominalPeakNits != nil {
			basis = "minimum of PQ 10000 nit ceiling and positive declared mastering/MaxCLL/nominal peak"
			env.ReferenceLuminance = &FloatRange{Min: luminanceMin(peak), Max: ceiling.Float64(), Unit: "cd/m² (nits)", Basis: "declared title/mastering envelope; PQ itself can represent up to 10000 nits"}
		}
		env.Peak.EffectiveCeilingNits = ceiling
		env.Peak.CeilingBasis = basis

	case TransferHLG:
		env.AbsoluteLuminanceFixed = false
		env.SceneLinearSignal = &FloatRange{Min: 0, Max: 1, Unit: "normalized scene-linear E", Basis: "ARIB STD-B67/BT.2100 HLG OETF domain"}
		ceiling := Rational{}
		basis := "HLG is scene-referred; no fixed absolute nit ceiling without nominal display peak/mastering metadata"
		for _, candidate := range []*Rational{peak.NominalPeakNits, peak.MasteringMaxNits, peak.MaxCLL} {
			if candidate != nil && candidate.Num > 0 && (ceiling.Den == 0 || candidate.Cmp(ceiling) < 0 || ceiling.Num == 0) {
				ceiling = *candidate
			}
		}
		if ceiling.Num > 0 {
			gamma := 1.2 + 0.42*math.Log10(ceiling.Float64()/1000.0)
			peak.HLGSystemGamma = &gamma
			basis = "HLG reference OOTF at declared nominal peak Lw; actual display adaptation may differ"
			env.ReferenceLuminance = &FloatRange{Min: luminanceMin(peak), Max: ceiling.Float64(), Unit: "cd/m² (nits)", Basis: basis}
			env.Peak.EffectiveCeilingNits = ceiling
		} else {
			env.Peak.EffectiveCeilingNits = ZeroRational()
		}
		env.Peak.CeilingBasis = basis
	}

	return env, nil
}

func luminanceMin(peak PeakSummary) float64 {
	if peak.MasteringMinNits != nil && peak.MasteringMinNits.Num > 0 {
		return peak.MasteringMinNits.Float64()
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func rationalStrings(values []*Rational) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value == nil {
			continue
		}
		text := value.String()
		if !seen[text] {
			seen[text] = true
			out = append(out, text)
		}
	}
	return out
}
