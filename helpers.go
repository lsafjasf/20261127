package hdrframe

// Int64Ptr makes sample and test literals more readable.
func Int64Ptr(v int64) *int64 { return &v }

// Float64Ptr makes optional floating-point fields in literals readable.
func Float64Ptr(v float64) *float64 { return &v }

func cloneMetadata(in []Metadata) []Metadata {
	out := make([]Metadata, len(in))
	copy(out, in)
	return out
}
