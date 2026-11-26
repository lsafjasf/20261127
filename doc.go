// Package colorcompare implements unmodified CIEDE2000 color differences for
// RGB24 samples and exposes the formula's lightness, chroma and hue terms.
//
// RGB24 inputs are interpreted as sRGB and converted to CIE L*a*b* using D65.
// DeltaE2000 uses kL=kC=kH=1 and performs no normalization or amplification.
// CompareRGB also reports raw RGB Euclidean and CIE76 Lab Euclidean distances.
package colorcompare
