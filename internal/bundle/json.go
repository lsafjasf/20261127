package bundle

import (
	"encoding/json"
	"io"
)

// jsonEncoder returns a compact encoder: no indentation, no HTML
// escaping (manifest paths may legitimately contain '<' or '&').
func jsonEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "")
	enc.SetEscapeHTML(false)
	return enc
}
