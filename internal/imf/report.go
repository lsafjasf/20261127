package imf

// Severity values for duplicate findings.
const (
	SeverityIdentical = "identical" // same UUID and consistent fixity across packages
	SeverityConflict  = "conflict"  // same UUID but different hash/size
	SeverityWarning   = "warning"
)

// MissingIssue is one missing/broken reference or asset.
type MissingIssue struct {
	Kind         string `json:"kind"`
	AssetID      string `json:"asset_id,omitempty"`
	ReferencedBy string `json:"referenced_by,omitempty"`
	Package      string `json:"package,omitempty"`
	Path         string `json:"path,omitempty"`
	Message      string `json:"message"`
}

// AssetOccurrence records one package's declaration of an asset UUID.
type AssetOccurrence struct {
	PackageLabel string   `json:"package_label"`
	PackageDir   string   `json:"package_dir"`
	InAssetMap   bool     `json:"in_asset_map"`
	InPKL        bool     `json:"in_pkl"`
	ChunkPaths   []string `json:"chunk_paths,omitempty"`
	ResolvedPath string   `json:"resolved_path,omitempty"`
	FileExists   bool     `json:"file_exists"`
	Hash         string   `json:"hash,omitempty"`
	Size         int64    `json:"size,omitempty"`
	Type         string   `json:"type,omitempty"`
}

// DuplicateIssue describes a repeated UUID/path/resource.
type DuplicateIssue struct {
	Kind        string            `json:"kind"`
	AssetID     string            `json:"asset_id,omitempty"`
	Severity    string            `json:"severity"`
	Message     string            `json:"message"`
	Occurrences []AssetOccurrence `json:"occurrences,omitempty"`
}

// Report is the missing + duplicate asset report.
type Report struct {
	Missing    []MissingIssue   `json:"missing"`
	Duplicates []DuplicateIssue `json:"duplicates"`
	Warnings   []WarningIssue   `json:"warnings,omitempty"`
	Summary    ReportSummary    `json:"summary"`
}

// WarningIssue is a non-fatal packaging inconsistency.
type WarningIssue struct {
	Kind    string `json:"kind"`
	AssetID string `json:"asset_id,omitempty"`
	Package string `json:"package,omitempty"`
	Message string `json:"message"`
}

// ReportSummary gives cheap counts for CI / strict mode.
type ReportSummary struct {
	MissingTotal       int `json:"missing_total"`
	UnresolvedRefs     int `json:"unresolved_references"`
	MissingFiles       int `json:"missing_files"`
	DuplicateTotal     int `json:"duplicate_total"`
	DuplicateIdentical int `json:"duplicate_identical"`
	DuplicateConflicts int `json:"duplicate_conflicts"`
}

func (r *Report) addMissing(m MissingIssue) {
	r.Missing = append(r.Missing, m)
}

func (r *Report) addDuplicate(d DuplicateIssue) {
	r.Duplicates = append(r.Duplicates, d)
}

func (r *Report) addWarning(w WarningIssue) {
	r.Warnings = append(r.Warnings, w)
}

func (r *Report) finalizeCounts() {
	r.Summary.MissingTotal = len(r.Missing)
	for _, m := range r.Missing {
		switch m.Kind {
		case "unresolved_reference", "cpl_not_found":
			r.Summary.UnresolvedRefs++
		case "file_not_found":
			r.Summary.MissingFiles++
		}
	}
	r.Summary.DuplicateTotal = len(r.Duplicates)
	for _, d := range r.Duplicates {
		switch d.Severity {
		case SeverityIdentical:
			r.Summary.DuplicateIdentical++
		case SeverityConflict:
			r.Summary.DuplicateConflicts++
		}
	}
}

// HasBlockingFindings reports whether strict mode should fail.
func (r *Report) HasBlockingFindings() bool {
	return len(r.Missing) > 0 || r.Summary.DuplicateConflicts > 0
}
