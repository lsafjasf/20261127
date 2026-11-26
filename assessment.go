package observermap

import (
	"fmt"
	"math"
)

// Reference CIE xy chromaticities for supported white points (2° values).
var (
	WhiteD65XY = [2]float64{0.31270, 0.32900}
	WhiteD50XY = [2]float64{0.34567, 0.35850}
)

// whiteMatchTolerance is an xy distance. D50<->D65 distance is ~0.044, so
// 0.015 keeps the two classes well separated while allowing encoding/paper
// deviations typical of tagged profiles.
const whiteMatchTolerance = 0.015

// Certainty records profile/white-point provenance and whether a result must
// be treated as uncertain.
type Certainty struct {
	ProfilePath      string       `json:"profile_path,omitempty"`
	ProfilePresent   bool         `json:"profile_present"`
	ProfileReadable  bool         `json:"profile_readable"`
	ProfileError     string       `json:"profile_error,omitempty"`
	WhitePoint       string       `json:"white_point"`
	WhitePointXY     *[2]float64  `json:"white_point_xy,omitempty"`
	WhiteSupported   bool         `json:"white_supported"`
	ObserverRecorded bool         `json:"observer_recorded"`
	Uncertain        bool         `json:"uncertain"`
	Reasons          []string     `json:"reasons"`
	Profile          *ProfileInfo `json:"profile,omitempty"`
}

// AssessProfile inspects path (empty means "no profile supplied") and derives
// the uncertainty flags required for observer mapping results.
func AssessProfile(path string) Certainty {
	c := Certainty{ProfilePath: path, WhitePoint: "NO_PROFILE"}

	if path == "" {
		c.Uncertain = true
		c.Reasons = append(c.Reasons,
			"no ICC profile supplied: observer and media white point are unverified; values use an explicit assumed white point")
		c.addObserverCaveat()
		return c
	}

	c.ProfilePresent = true
	pi, err := ParseProfileFile(path)
	if err != nil {
		c.ProfileReadable = false
		c.ProfileError = err.Error()
		c.WhitePoint = "READ_ERROR"
		c.Uncertain = true
		c.Reasons = append(c.Reasons, fmt.Sprintf("ICC profile missing or unreadable (%v); observer/white-point provenance unknown", err))
		c.addObserverCaveat()
		return c
	}

	c.ProfileReadable = true
	c.Profile = pi
	c.ObserverRecorded = pi.HasObserverField
	c.addObserverCaveat()

	if pi.MediaWhitePoint == nil {
		c.WhitePoint = "UNKNOWN"
		c.Uncertain = true
		c.Reasons = append(c.Reasons, "ICC profile has no wtpt (media white point) tag; cannot verify supported white point")
		return c
	}

	x, y, err := pi.MediaWhitePoint.Chromaticity()
	if err != nil {
		c.WhitePoint = "INVALID"
		c.Uncertain = true
		c.Reasons = append(c.Reasons, "wtpt tristimulus values are invalid")
		return c
	}
	c.WhitePointXY = &[2]float64{x, y}

	switch classifyWhiteXY(x, y) {
	case "D50":
		c.WhitePoint = "D50"
		c.WhiteSupported = true
	case "D65":
		c.WhitePoint = "D65"
		c.WhiteSupported = true
	default:
		c.WhitePoint = "UNSUPPORTED"
		c.Uncertain = true
		c.Reasons = append(c.Reasons, fmt.Sprintf(
			"media white point xy=(%.4f,%.4f) is outside supported D50/D65 classes (tol=%.3f); no reliable observer map is defined for it",
			x, y, whiteMatchTolerance))
	}
	return c
}

func (c *Certainty) addObserverCaveat() {
	if !c.ObserverRecorded {
		c.Reasons = append(c.Reasons,
			"ICC.1 headers/tags do not record whether CIE 1931 2° or CIE 1964 10° observer data produced the profile transform; observer provenance is unknown")
	}
}

func classifyWhiteXY(x, y float64) string {
	if math.Hypot(x-WhiteD50XY[0], y-WhiteD50XY[1]) <= whiteMatchTolerance {
		return "D50"
	}
	if math.Hypot(x-WhiteD65XY[0], y-WhiteD65XY[1]) <= whiteMatchTolerance {
		return "D65"
	}
	return "OTHER"
}

// AssessAssumedWhite builds a certainty for an explicitly assumed white point
// when no usable profile is available.
func AssessAssumedWhite(name string, base Certainty) Certainty {
	switch name {
	case "D65", "D50":
		base.Reasons = append(base.Reasons,
			fmt.Sprintf("white point %s is an explicit assumption, not verified from an ICC profile", name))
	default:
		base.WhitePoint = "UNSUPPORTED"
		base.WhiteSupported = false
		base.Uncertain = true
		base.Reasons = append(base.Reasons, fmt.Sprintf("assumed white point %q is not supported (use D50 or D65)", name))
	}
	return base
}
