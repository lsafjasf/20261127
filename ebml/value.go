package ebml

// SafeUint returns 0 for a nil element.
func (e *Element) SafeUint() uint64 {
	if e == nil {
		return 0
	}
	return e.Int()
}

// SafeString returns "" for a nil element.
func (e *Element) SafeString() string {
	if e == nil {
		return ""
	}
	return e.String()
}
