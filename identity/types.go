// Package identity builds stable track/chapter identities and migrates
// references independently of mutable line numbers in an MKV Tracks list.
package identity

import "mkvidentity/timepoint"

type Kind string

const (
	KindTrack   Kind = "track"
	KindEdition Kind = "edition"
	KindChapter Kind = "chapter"
)

type Ref string

func TrackRef(uid uint64) Ref   { return Ref("track:uid:" + uintHex(uid)) }
func ChapterRef(uid uint64) Ref { return Ref("chapter:uid:" + uintHex(uid)) }
func EditionRef(uid uint64) Ref { return Ref("edition:uid:" + uintHex(uid)) }

func uintHex(v uint64) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v&0xf]
		v >>= 4
	}
	return string(buf[i:])
}

type TrackType uint64

const (
	TrackVideo    TrackType = 1
	TrackAudio    TrackType = 2
	TrackSubtitle TrackType = 17
)

// Track is the identity-relevant subset of an MKV TrackEntry.
type Track struct {
	UID          uint64    `json:"uid"`
	Number       uint64    `json:"number,omitempty"`
	Type         TrackType `json:"type,omitempty"`
	CodecID      string    `json:"codec_id,omitempty"`
	Language     string    `json:"language,omitempty"`
	LanguageIETF string    `json:"language_ietf,omitempty"`
	Name         string    `json:"name,omitempty"`
	Default      bool      `json:"default"`
	DefaultSet   bool      `json:"default_set"`
	Enabled      bool      `json:"enabled"`
	Forced       bool      `json:"forced"`
	Fingerprint  string    `json:"fingerprint,omitempty"`
	LegacyLine   int       `json:"legacy_line,omitempty"`
}

type ChapterDisplay struct {
	String       string `json:"string,omitempty"`
	Language     string `json:"language,omitempty"`
	LanguageIETF string `json:"language_ietf,omitempty"`
}

// Chapter keeps both native UIDs and tolerant time parsing results.
type Chapter struct {
	UID        uint64           `json:"uid"`
	StringUID  string           `json:"string_uid,omitempty"`
	EditionUID uint64           `json:"edition_uid,omitempty"`
	ParentUID  uint64           `json:"parent_uid,omitempty"`
	Ordinal    int              `json:"ordinal,omitempty"`
	LegacyLine int              `json:"legacy_line,omitempty"`
	Start      timepoint.Point  `json:"start"`
	End        *timepoint.Point `json:"end,omitempty"`
	TrackUIDs  []uint64         `json:"track_uids,omitempty"`
	Displays   []ChapterDisplay `json:"displays,omitempty"`
	Hidden     bool             `json:"hidden,omitempty"`
	Enabled    bool             `json:"enabled"`
}

type Edition struct {
	UID         uint64   `json:"uid"`
	Default     bool     `json:"default,omitempty"`
	Ordered     bool     `json:"ordered,omitempty"`
	ChapterUIDs []uint64 `json:"chapter_uids,omitempty"`
}

// Inventory is the metadata surface needed for identity migration. It can
// be populated directly from MKV EBML or from a JSON sidecar.
type Inventory struct {
	Path            string    `json:"path,omitempty"`
	TimecodeScaleNS uint64    `json:"timecode_scale_ns,omitempty"`
	DurationNS      *uint64   `json:"duration_ns,omitempty"`
	Tracks          []Track   `json:"tracks"`
	Editions        []Edition `json:"editions,omitempty"`
	Chapters        []Chapter `json:"chapters,omitempty"`
}

type Severity string

const (
	SeverityError      Severity = "error"
	SeverityWarning    Severity = "warning"
	SeverityUnresolved Severity = "unresolved"
)

type Issue struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Subject  Ref      `json:"subject,omitempty"`
	Field    string   `json:"field,omitempty"`
	Message  string   `json:"message"`
}

type Edge struct {
	From    Ref    `json:"from"`
	To      Ref    `json:"to"`
	Type    string `json:"type"`
	Context string `json:"context,omitempty"`
}

type Node struct {
	Ref      Ref    `json:"ref"`
	Kind     Kind   `json:"kind"`
	UID      uint64 `json:"uid,omitempty"`
	StableID string `json:"stable_id,omitempty"`
	Label    string `json:"label,omitempty"`
}

type Graph struct {
	Nodes  []Node  `json:"nodes"`
	Edges  []Edge  `json:"edges"`
	Issues []Issue `json:"issues,omitempty"`
}
