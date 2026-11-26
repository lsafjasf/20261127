package hdrframe

// Report is the complete per-target reconciliation result.
type Report struct {
	SchemaVersion int           `json:"schema_version"`
	Name          string        `json:"name,omitempty"`
	FPS           string        `json:"fps,omitempty"`
	FPSFloat      float64       `json:"fps_float,omitempty"`
	Policy        Policy        `json:"policy"`
	Frames        []FrameReport `json:"frames"`
	Duplicates    []Duplicate   `json:"duplicates,omitempty"`
	SequenceGaps  []SequenceGap `json:"sequence_gaps,omitempty"`
	SceneChanges  []SceneChange `json:"scene_changes,omitempty"`
	Diagnostics   []Issue       `json:"diagnostics,omitempty"`
	Summary       Summary       `json:"summary"`
}

// FrameReport answers which metadata should apply and which version did apply.
type FrameReport struct {
	Frame           int64       `json:"frame"`
	PTSSeconds      float64     `json:"pts_seconds"`
	Timecode        string      `json:"timecode,omitempty"`
	Label           string      `json:"label,omitempty"`
	ExpectedSceneID *int64      `json:"expected_scene_id,omitempty"`
	Expected        *VersionRef `json:"expected,omitempty"`
	Selected        *VersionRef `json:"selected,omitempty"`
	Applied         *AppliedRef `json:"applied,omitempty"`
	Status          string      `json:"status"`
	TimelineVerdict string      `json:"timeline_verdict"`
	ParameterDiffs  []ParamDiff `json:"parameter_diffs,omitempty"`
	Issues          []Issue     `json:"issues,omitempty"`
	SceneID         *int64      `json:"scene_id,omitempty"`
	Sequence        int64       `json:"sequence,omitempty"`
	ExpectedVersion string      `json:"expected_version,omitempty"`
	SelectedVersion string      `json:"selected_version,omitempty"`
	AppliedVersion  string      `json:"applied_version,omitempty"`
	AppliedChecksum string      `json:"applied_checksum,omitempty"`
	AppliedSource   string      `json:"applied_source,omitempty"`
	AppliedVerified bool        `json:"applied_verified,omitempty"`
}

// ParamDiff explains the visual parameter gap between required and applied versions.
type ParamDiff struct {
	Field    string  `json:"field"`
	Label    string  `json:"label"`
	Expected float64 `json:"expected"`
	Applied  float64 `json:"applied"`
	Delta    float64 `json:"delta"`
	Effect   string  `json:"effect,omitempty"`
}

// VersionRef is the comparable identity and timing of one metadata version.
type VersionRef struct {
	RecordID          string          `json:"record_id,omitempty"`
	Version           string          `json:"version"`
	Sequence          int64           `json:"sequence,omitempty"`
	Frame             *int64          `json:"frame,omitempty"`
	SceneID           *int64          `json:"scene_id,omitempty"`
	SceneFrameIndex   *int64          `json:"scene_frame_index,omitempty"`
	EffectiveSeconds  float64         `json:"effective_seconds"`
	EffectiveTimecode string          `json:"effective_timecode,omitempty"`
	Checksum          string          `json:"checksum,omitempty"`
	SelectionBasis    string          `json:"selection_basis"`
	Source            string          `json:"source,omitempty"`
	Highlight         HighlightParams `json:"highlight,omitempty"`
	Color             ColorParams     `json:"color,omitempty"`
}

// AppliedRef identifies the version the player actually used, or an inferred one.
type AppliedRef struct {
	VersionRef
	EvidenceSource  string `json:"evidence_source"`
	Evidence        string `json:"evidence,omitempty"`
	Verified        bool   `json:"verified"`
	MatchesSelected bool   `json:"matches_selected"`
	MatchesExpected bool   `json:"matches_expected"`
	Reason          string `json:"reason,omitempty"`
}

// Duplicate describes repeated sequence/frame metadata records.
type Duplicate struct {
	Key           string   `json:"key"`
	Sequence      int64    `json:"sequence,omitempty"`
	Frame         *int64   `json:"frame,omitempty"`
	Versions      []string `json:"versions"`
	Checksums     []string `json:"checksums,omitempty"`
	Identical     bool     `json:"identical"`
	Resolution    string   `json:"resolution"`
	ChosenVersion string   `json:"chosen_version,omitempty"`
}

// SequenceGap describes a jump between consecutive metadata sequence numbers.
type SequenceGap struct {
	AfterSequence  int64   `json:"after_sequence"`
	BeforeSequence int64   `json:"before_sequence"`
	MissingCount   int64   `json:"missing_count"`
	Missing        []int64 `json:"missing,omitempty"`
	AfterFrame     *int64  `json:"after_frame,omitempty"`
	BeforeFrame    *int64  `json:"before_frame,omitempty"`
	Action         string  `json:"action"`
}

// SceneChange marks a change of scene number in metadata.
type SceneChange struct {
	Frame     int64  `json:"frame"`
	FromScene *int64 `json:"from_scene,omitempty"`
	ToScene   *int64 `json:"to_scene,omitempty"`
}

// Summary contains roll-up counts.
type Summary struct {
	TargetFrames       int `json:"target_frames"`
	Matched            int `json:"matched"`
	Mismatched         int `json:"mismatched"`
	Unverified         int `json:"unverified"`
	MissingExactRecord int `json:"missing_exact_record"`
	DuplicateKeys      int `json:"duplicate_keys"`
	DuplicateConflicts int `json:"duplicate_conflicts"`
	SequenceGaps       int `json:"sequence_gaps"`
	SceneMismatches    int `json:"scene_mismatches"`
}

func (f *FrameReport) flatten() {
	if f.Expected != nil {
		f.ExpectedVersion = f.Expected.Version
	}
	if f.Selected != nil {
		f.SelectedVersion = f.Selected.Version
		f.SceneID = f.Selected.SceneID
		f.Sequence = f.Selected.Sequence
	}
	if f.Applied != nil {
		f.AppliedVersion = f.Applied.Version
		f.AppliedChecksum = f.Applied.Checksum
		f.AppliedSource = f.Applied.EvidenceSource
		f.AppliedVerified = f.Applied.Verified
		if f.Applied.SceneID != nil {
			f.SceneID = f.Applied.SceneID
		}
		if f.Applied.Sequence != 0 {
			f.Sequence = f.Applied.Sequence
		}
	}
	if f.SceneID == nil && f.Expected != nil {
		f.SceneID = f.Expected.SceneID
	}
}
