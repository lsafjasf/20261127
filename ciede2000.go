package colorcompare

import "math"

const ciede2000Pow25To7 = 6103515625.0 // 25^7

// DeltaEComponents exposes the terms used by CIEDE2000 without rescaling them.
// LightnessTerm, ChromaTerm and HueTerm are already divided by SL, SC and SH.
// RotationTerm is the signed RT*ChromaTerm*HueTerm interaction.
type DeltaEComponents struct {
	DeltaL        float64
	DeltaC        float64
	DeltaH        float64
	DeltaHueAngle float64
	LightnessTerm float64
	ChromaTerm    float64
	HueTerm       float64
	RotationTerm  float64
	SL            float64
	SC            float64
	SH            float64
	RT            float64
	HueMean       float64
	Hue1          float64
	Hue2          float64
	Chroma1       float64
	Chroma2       float64
}

// SquaredSum reconstructs DeltaE2000 squared from the component terms.
func (c DeltaEComponents) SquaredSum() float64 {
	return c.LightnessTerm*c.LightnessTerm +
		c.ChromaTerm*c.ChromaTerm +
		c.HueTerm*c.HueTerm +
		c.RotationTerm
}

// LightnessContribution, ChromaContribution and HueContribution are squared
// normalized terms. RotationContribution is the interaction term. The four
// values add up exactly to DeltaE2000 squared.
func (c DeltaEComponents) LightnessContribution() float64 {
	return c.LightnessTerm * c.LightnessTerm
}

func (c DeltaEComponents) ChromaContribution() float64 {
	return c.ChromaTerm * c.ChromaTerm
}

func (c DeltaEComponents) HueContribution() float64 {
	return c.HueTerm * c.HueTerm
}

func (c DeltaEComponents) RotationContribution() float64 {
	return c.RotationTerm
}

// Dimension identifies the largest normalized CIEDE2000 component.
type Dimension string

const (
	DimensionNone      Dimension = ""
	DimensionLightness Dimension = "lightness"
	DimensionChroma    Dimension = "chroma"
	DimensionHue       Dimension = "hue"
)

// Dominant returns the dimension with the largest absolute normalized term.
// Rotation is an interaction term and is never assigned to one dimension.
func (c DeltaEComponents) Dominant() Dimension {
	dl := math.Abs(c.LightnessTerm)
	dc := math.Abs(c.ChromaTerm)
	dh := math.Abs(c.HueTerm)
	if math.Max(dl, math.Max(dc, dh)) == 0 {
		return DimensionNone
	}
	if dl >= dc && dl >= dh {
		return DimensionLightness
	}
	if dc >= dh {
		return DimensionChroma
	}
	return DimensionHue
}

// DeltaE2000 returns the standard CIEDE2000 color difference with kL=kC=kH=1.
// The result is not normalized, clamped, gamma-corrected or otherwise scaled.
func DeltaE2000(lab1, lab2 Lab) float64 {
	delta, _ := DeltaE2000Details(lab1, lab2)
	return delta
}

// CIEDE2000 is an alias for DeltaE2000.
func CIEDE2000(lab1, lab2 Lab) float64 {
	return DeltaE2000(lab1, lab2)
}

// DeltaE2000RGB converts two RGB24 samples to Lab and applies CIEDE2000.
func DeltaE2000RGB(a, b RGB24) float64 {
	return DeltaE2000(LabFromRGB(a), LabFromRGB(b))
}

// DeltaE2000Details returns both the standard distance and its exact terms.
func DeltaE2000Details(lab1, lab2 Lab) (float64, DeltaEComponents) {
	c1 := math.Hypot(lab1.A, lab1.B)
	c2 := math.Hypot(lab2.A, lab2.B)
	cMean := (c1 + c2) / 2
	cMean7 := math.Pow(cMean, 7)
	g := 0.5 * (1 - math.Sqrt(cMean7/(cMean7+ciede2000Pow25To7)))

	a1Prime := (1 + g) * lab1.A
	a2Prime := (1 + g) * lab2.A
	c1Prime := math.Hypot(a1Prime, lab1.B)
	c2Prime := math.Hypot(a2Prime, lab2.B)
	h1Prime := hueDegrees(lab1.B, a1Prime)
	h2Prime := hueDegrees(lab2.B, a2Prime)

	deltaL := lab2.L - lab1.L
	deltaC := c2Prime - c1Prime
	deltaHue := hueDifferenceDegrees(h1Prime, h2Prime, c1Prime, c2Prime)
	deltaH := 2 * math.Sqrt(c1Prime*c2Prime) *
		math.Sin(degreesToRadians(deltaHue)/2)

	lMean := (lab1.L + lab2.L) / 2
	cMeanPrime := (c1Prime + c2Prime) / 2
	hMean := meanHueDegrees(h1Prime, h2Prime, c1Prime, c2Prime)

	t := 1 -
		0.17*math.Cos(degreesToRadians(hMean-30)) +
		0.24*math.Cos(degreesToRadians(2*hMean)) +
		0.32*math.Cos(degreesToRadians(3*hMean+6)) -
		0.20*math.Cos(degreesToRadians(4*hMean-63))

	deltaTheta := 30 * math.Exp(-math.Pow((hMean-275)/25, 2))
	cMeanPrime7 := math.Pow(cMeanPrime, 7)
	rc := 2 * math.Sqrt(cMeanPrime7/(cMeanPrime7+ciede2000Pow25To7))
	rt := -math.Sin(degreesToRadians(2*deltaTheta)) * rc

	sl := 1 + (0.015*math.Pow(lMean-50, 2))/math.Sqrt(20+math.Pow(lMean-50, 2))
	sc := 1 + 0.045*cMeanPrime
	sh := 1 + 0.015*cMeanPrime*t

	lightnessTerm := deltaL / sl
	chromaTerm := deltaC / sc
	hueTerm := deltaH / sh
	rotationTerm := rt * chromaTerm * hueTerm

	squared := lightnessTerm*lightnessTerm +
		chromaTerm*chromaTerm +
		hueTerm*hueTerm +
		rotationTerm

	return math.Sqrt(squared), DeltaEComponents{
		DeltaL:        deltaL,
		DeltaC:        deltaC,
		DeltaH:        deltaH,
		DeltaHueAngle: deltaHue,
		LightnessTerm: lightnessTerm,
		ChromaTerm:    chromaTerm,
		HueTerm:       hueTerm,
		RotationTerm:  rotationTerm,
		SL:            sl,
		SC:            sc,
		SH:            sh,
		RT:            rt,
		HueMean:       hMean,
		Hue1:          h1Prime,
		Hue2:          h2Prime,
		Chroma1:       c1Prime,
		Chroma2:       c2Prime,
	}
}

func hueDegrees(b, aPrime float64) float64 {
	if aPrime == 0 && b == 0 {
		return 0
	}
	h := math.Atan2(b, aPrime) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

func hueDifferenceDegrees(h1, h2, c1, c2 float64) float64 {
	if c1*c2 == 0 {
		return 0
	}
	d := h2 - h1
	switch {
	case math.Abs(d) <= 180:
		return d
	case d > 180:
		return d - 360
	default:
		return d + 360
	}
}

func meanHueDegrees(h1, h2, c1, c2 float64) float64 {
	if c1*c2 == 0 {
		return h1 + h2
	}
	sum := h1 + h2
	switch {
	case math.Abs(h1-h2) <= 180:
		return sum / 2
	case sum < 360:
		return (sum + 360) / 2
	default:
		return (sum - 360) / 2
	}
}

func degreesToRadians(v float64) float64 {
	return v * math.Pi / 180
}
