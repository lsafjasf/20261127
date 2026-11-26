package iccproof

import (
	"encoding/json"
	"fmt"
	"io"
)

// ParseRequestJSON decodes a request document. Intent strings are normalized
// only when Proof runs; unknown values produce a stable error.
func ParseRequestJSON(r io.Reader) (Request, error) {
	var req Request
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return Request{}, fmt.Errorf("parse request JSON: %w", err)
	}
	if len(req.Intents) == 0 {
		req.Intents = AllProofIntents()
	}
	return req, nil
}
