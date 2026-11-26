// Package imf loads IMF/IMP packages (AssetMap, PKL, CPL, OPL), resolves the
// cross-package resource inheritance between a main package and supplemental
// packages, and builds the final merged playlist together with missing and
// duplicate asset reports.
package imf

// PackageRole describes where a package sits in the inheritance chain.
type PackageRole string

const (
	RoleMain       PackageRole = "main"
	RoleSupplement PackageRole = "supplemental"
	RoleExtra      PackageRole = "package"
)

// Package is one IMF package directory (one ASSETMAP + PKL set).
type Package struct {
	Dir      string      `json:"dir"`
	Label    string      `json:"label"`
	Role     PackageRole `json:"role"`
	AssetMap *AssetMap   `json:"asset_map,omitempty"`
	PKL      *PKL        `json:"pkl,omitempty"`
	CPLs     []*CPL      `json:"cpls,omitempty"`
	OPLs     []*OPL      `json:"opls,omitempty"`
	XMLFiles []string    `json:"xml_files,omitempty"`
	// FileIndex maps UUIDs found in file/directory names to absolute paths.
	// It is a fallback when ASSETMAP/PKL metadata is partial or absent.
	FileIndex map[string][]string `json:"-"`
}

// MapEntry is one AssetMap asset → chunk path mapping.
type MapEntry struct {
	ID          string   `json:"id"`
	PackingList bool     `json:"packing_list,omitempty"`
	ChunkPaths  []string `json:"chunk_paths,omitempty"`
	SourceFile  string   `json:"source_file,omitempty"`
}

// AssetMap is a parsed ASSETMAP.
type AssetMap struct {
	ID         string      `json:"id"`
	SourceFile string      `json:"source_file"`
	Assets     []*MapEntry `json:"assets"`
}

// PKLEntry is one PKL asset record (fixity metadata).
type PKLEntry struct {
	ID               string `json:"id"`
	Hash             string `json:"hash,omitempty"`
	Size             int64  `json:"size,omitempty"`
	Type             string `json:"type,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
	SourceFile       string `json:"source_file,omitempty"`
}

// PKL is a parsed Packing List.
type PKL struct {
	ID         string      `json:"id"`
	SourceFile string      `json:"source_file"`
	Assets     []*PKLEntry `json:"assets"`
}

// Resource is one CPL resource. Only TrackFileResource entries reference
// package assets via TrackFileID; other UUID fields (e.g. SourceEncoding)
// reference in-CPL descriptors and must not be treated as missing assets.
type Resource struct {
	Index             int            `json:"index"`
	Kind              string         `json:"kind,omitempty"`
	ID                string         `json:"id,omitempty"`
	XsiType           string         `json:"xsi_type,omitempty"`
	TrackFileID       string         `json:"track_file_id,omitempty"`
	EditRate          string         `json:"edit_rate,omitempty"`
	IntrinsicDuration int64          `json:"intrinsic_duration,omitempty"`
	EntryPoint        int64          `json:"entry_point,omitempty"`
	SourceDuration    int64          `json:"source_duration,omitempty"`
	RepeatCount       int            `json:"repeat_count,omitempty"`
	SourceEncoding    string         `json:"source_encoding,omitempty"`
	Resolved          *ResolvedAsset `json:"resolved,omitempty"`
}

// Track is one CPL virtual track (*Sequence element).
type Track struct {
	SequenceName string      `json:"sequence_name"`
	Kind         string      `json:"kind"`
	SequenceID   string      `json:"sequence_id,omitempty"`
	TrackID      string      `json:"track_id,omitempty"`
	Resources    []*Resource `json:"resources"`
	// Provenance lists package labels/CPL ids that contributed this track.
	Provenance []string `json:"provenance,omitempty"`
}

// Segment is a CPL segment.
type Segment struct {
	ID     string   `json:"id,omitempty"`
	Tracks []*Track `json:"tracks"`
}

// CPL is a parsed Composition Playlist.
type CPL struct {
	SourceFile   string     `json:"source_file"`
	PackageLabel string     `json:"package_label,omitempty"`
	ID           string     `json:"id"`
	Annotation   string     `json:"annotation,omitempty"`
	EditRate     string     `json:"edit_rate,omitempty"`
	ParentRefs   []string   `json:"parent_refs,omitempty"`
	Segments     []*Segment `json:"segments"`
}

// OPL is a parsed Output Profile List.
type OPL struct {
	SourceFile   string   `json:"source_file"`
	PackageLabel string   `json:"package_label,omitempty"`
	ID           string   `json:"id"`
	Annotation   string   `json:"annotation,omitempty"`
	CPLRefs      []string `json:"cpl_refs,omitempty"`
	TrackRefs    []string `json:"track_refs,omitempty"`
	ProfileNames []string `json:"profile_names,omitempty"`
}

// ResolvedAsset tells where an asset UUID ended up resolvable.
type ResolvedAsset struct {
	AssetID      string `json:"asset_id"`
	Found        bool   `json:"found"`
	PackageLabel string `json:"package_label,omitempty"`
	RelativePath string `json:"relative_path,omitempty"`
	AbsolutePath string `json:"absolute_path,omitempty"`
	FileExists   bool   `json:"file_exists"`
	OutputPath   string `json:"output_path,omitempty"`
	InAssetMap   bool   `json:"in_asset_map"`
	InPKL        bool   `json:"in_pkl"`
	PKLHash      string `json:"pkl_hash,omitempty"`
	PKLSize      int64  `json:"pkl_size,omitempty"`
}
