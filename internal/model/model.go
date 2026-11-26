// Package model defines the on-disk JSON contracts of the evidence chain.
package model

// ToolName/Version are stamped into every generated document.
const (
	ToolName    = "evidencechain"
	ToolVersion = "1.0.0"
	Schema      = "evidencechain/v1"
)

// Index is the hash index rooted at the sealed original materials.
type Index struct {
	Schema       string       `json:"schema"`
	Tool         string       `json:"tool"`
	Version      string       `json:"version"`
	CaseID       string       `json:"case_id"`
	Algorithm    string       `json:"algorithm"`
	ChunkSize    int          `json:"chunk_size"`
	ChunkRootAlg string       `json:"chunk_root_algorithm"`
	Note         string       `json:"note,omitempty"`
	Entries      []IndexEntry `json:"entries"`
}

// IndexEntry is one trusted original item with exact and locality digests.
type IndexEntry struct {
	EvidenceID  string        `json:"evidence_id"`
	LogicalName string        `json:"logical_name"`
	RelPath     string        `json:"rel_path"`
	Size        int64         `json:"size"`
	SHA256      string        `json:"sha256"`
	ChunkRoot   string        `json:"chunk_root"`
	Chunks      []ChunkRecord `json:"chunks,omitempty"`
	Fuzzy       FuzzyRecord   `json:"fuzzy"`
}

// ChunkRecord mirrors digestx.Chunk but lives in the stable contract package.
type ChunkRecord struct {
	Index  int    `json:"index"`
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

// FuzzyRecord stores the non-identifying similarity digest.
type FuzzyRecord struct {
	Algorithm string `json:"algorithm"`
	SimHash   string `json:"simhash"`
	Features  int    `json:"features"`
	Unique    int    `json:"unique"`
}

// Provenance is the chain-of-custody / derivation graph.
type Provenance struct {
	Schema string `json:"schema"`
	CaseID string `json:"case_id"`
	Note   string `json:"note,omitempty"`
	Links  []Link `json:"links"`
}

// Link is one declared derivation or handling step.
type Link struct {
	ID       string   `json:"id"`
	From     []string `json:"from"`
	To       string   `json:"to"`
	Relation string   `json:"relation"`
	Operator string   `json:"operator,omitempty"`
	At       string   `json:"at,omitempty"`
	Details  string   `json:"details,omitempty"`
}

// Judgments is the human-review record set. Machine verification audits it;
// it never invents verdicts.
type Judgments struct {
	Schema     string     `json:"schema"`
	CaseID     string     `json:"case_id"`
	Analyst    string     `json:"analyst"`
	ReviewedAt string     `json:"reviewed_at"`
	Items      []Judgment `json:"judgments"`
}

// Judgment is one signed-off human conclusion.
type Judgment struct {
	ID         string   `json:"id"`
	Target     string   `json:"target"`
	Verdict    string   `json:"verdict"`
	Confidence string   `json:"confidence"`
	Basis      []string `json:"basis"`
	Rationale  string   `json:"rationale"`
}

// Watchlist is an external reference hash bulletin.
type Watchlist struct {
	Schema  string          `json:"schema"`
	CaseID  string          `json:"case_id"`
	Source  string          `json:"source"`
	Entries []WatchlistHash `json:"hashes"`
}

// WatchlistHash is one externally supplied hash, possibly malformed/partial.
type WatchlistHash struct {
	Label    string `json:"label"`
	Kind     string `json:"kind"` // sha256 | sha256-lookalike | fuzzy
	Value    string `json:"value"`
	Evidence string `json:"intended_evidence,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Verification is the machine result document.
type Verification struct {
	Schema     string            `json:"schema"`
	Tool       string            `json:"tool"`
	Version    string            `json:"version"`
	CaseID     string            `json:"case_id"`
	OK         bool              `json:"ok"`
	Summary    map[string]int    `json:"summary"`
	Anchors    []AnchorResult    `json:"anchors"`
	Links      []LinkResult      `json:"links"`
	Files      []FileResult      `json:"field_files"`
	Containers []ContainerResult `json:"containers"`
	Watchlist  []WatchResult     `json:"watchlist"`
	Judgments  []JudgeResult     `json:"judgments"`
	Findings   []Finding         `json:"findings"`
	Errors     []string          `json:"errors,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
}

// AnchorResult is the seal check of one original.
type AnchorResult struct {
	EvidenceID string `json:"evidence_id"`
	RelPath    string `json:"rel_path"`
	Status     string `json:"status"` // intact | tampered | missing
	Detail     string `json:"detail,omitempty"`
}

// LinkResult is the checked state of one provenance link.
type LinkResult struct {
	ID       string `json:"id"`
	To       string `json:"to"`
	Relation string `json:"relation"`
	Status   string `json:"status"` // verified | broken | unsupported
	Detail   string `json:"detail,omitempty"`
}

// BlockDiff localizes tampering against a candidate anchor.
type BlockDiff struct {
	ChunkIndex int    `json:"chunk_index"`
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
	Status     string `json:"status"` // match | differ | extra
}

// FileResult classifies one loose field file.
type FileResult struct {
	Path            string        `json:"path"`
	Size            int64         `json:"size"`
	SHA256          string        `json:"sha256"`
	Status          string        `json:"status"`
	MatchedEvidence string        `json:"matched_evidence,omitempty"`
	Candidate       string        `json:"candidate,omitempty"`
	RenamedFrom     string        `json:"renamed_from,omitempty"`
	Fuzzy           *FuzzyVerdict `json:"fuzzy,omitempty"`
	BlockDiffs      []BlockDiff   `json:"block_diffs,omitempty"`
	MatchedBlocks   int           `json:"matched_blocks,omitempty"`
	TotalBlocks     int           `json:"total_blocks,omitempty"`
	Detail          string        `json:"detail,omitempty"`
}

// FuzzyVerdict carries similarity-only metrics.
type FuzzyVerdict struct {
	JaccardPercent float64 `json:"jaccard_percent"`
	HammingBits    int     `json:"hamming_bits"`
	Lead           bool    `json:"lead"`
}

// ContainerResult describes one zip/tar and its members.
type ContainerResult struct {
	Path             string         `json:"path"`
	Format           string         `json:"format"`
	FileSHA256       string         `json:"file_sha256"`
	MemberSetSHA256  string         `json:"memberset_sha256"`
	PathSetSHA256    string         `json:"pathset_sha256"`
	MatchesAnchorSet string         `json:"matches_anchor_set,omitempty"`
	Status           string         `json:"status"`
	Members          []MemberResult `json:"members"`
	Detail           string         `json:"detail,omitempty"`
}

// MemberResult classifies one container entry.
type MemberResult struct {
	Name            string        `json:"name"`
	Size            int64         `json:"size"`
	SHA256          string        `json:"sha256"`
	Status          string        `json:"status"`
	MatchedEvidence string        `json:"matched_evidence,omitempty"`
	RenamedFrom     string        `json:"renamed_from,omitempty"`
	Candidate       string        `json:"candidate,omitempty"`
	Fuzzy           *FuzzyVerdict `json:"fuzzy,omitempty"`
	BlockDiffs      []BlockDiff   `json:"block_diffs,omitempty"`
	MatchedBlocks   int           `json:"matched_blocks,omitempty"`
	TotalBlocks     int           `json:"total_blocks,omitempty"`
}

// WatchResult is one audited external hash.
type WatchResult struct {
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Value   string `json:"value"`
	Status  string `json:"status"` // exact-hit | rejected-prefix-only | similarity-lead | no-match | malformed
	Against string `json:"against,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// JudgeResult audits one human judgment against machine facts.
type JudgeResult struct {
	ID      string `json:"id"`
	Target  string `json:"target"`
	Verdict string `json:"verdict"`
	Status  string `json:"status"` // supported | uncertain-allowed | contradiction | overclaim | unknown-target
	Detail  string `json:"detail,omitempty"`
}

// Finding is a report-ready, cross-referenced conclusion atom.
type Finding struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Severity string   `json:"severity"` // info | confirmed | warning | critical
	Status   string   `json:"status"`   // confirmed | likely | uncertain
	Summary  string   `json:"summary"`
	Evidence []string `json:"evidence,omitempty"`
}
