package gamutcompare

import _ "embed"

// SampleInputJSON is a runnable procurement-template example. The monitor
// values are standard-gamut-like samples, not measurements of a real model.
//
//go:embed examples/sample_input.json
var SampleInputJSON []byte
