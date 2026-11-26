package imf

import (
	"regexp"
	"strings"
)

var urnUUIDRe = regexp.MustCompile(`(?i)urn:uuid:([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)
var bareUUIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NormalizeID canonicalizes IMF identifiers to lowercase bare UUID form.
// It accepts "urn:uuid:...", brace-wrapped UUIDs and bare UUIDs. Values that
// are not UUIDs are returned trimmed (but still lower-cased when they look
// like a URN).
func NormalizeID(v string) string {
	s := strings.TrimSpace(v)
	if m := urnUUIDRe.FindStringSubmatch(s); m != nil {
		return strings.ToLower(m[1])
	}
	s = strings.TrimPrefix(strings.ToLower(s), "urn:uuid:")
	s = strings.Trim(s, "{}() \t\r\n")
	return s
}

// URN returns the canonical urn:uuid form for a normalized id.
func URN(id string) string {
	id = NormalizeID(id)
	if id == "" {
		return ""
	}
	if bareUUIDRe.MatchString(id) {
		return "urn:uuid:" + id
	}
	return id
}

// IsUUID reports whether v is (or contains) a canonical UUID.
func IsUUID(v string) bool {
	return bareUUIDRe.MatchString(NormalizeID(v))
}

// ExtractUUIDRefs pulls every urn:uuid reference out of a text blob.
func ExtractUUIDRefs(blob string) []string {
	matches := urnUUIDRe.FindAllStringSubmatch(blob, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		id := strings.ToLower(m[1])
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

var anyUUIDRe = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// ExtractAnyUUIDs finds both urn:uuid and bare UUIDs (e.g. UUID-shaped
// filenames such as CPL_<uuid>.xml or <TrackFileId>.mxf).
func ExtractAnyUUIDs(blob string) []string {
	matches := anyUUIDRe.FindAllString(blob, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		id := strings.ToLower(m)
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
