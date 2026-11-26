package imf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Workspace is the ordered union of the main package and its supplements.
// Package order defines inheritance precedence: later packages layer on top
// of earlier ones.
type Workspace struct {
	Packages []*Package
	Report   *Report

	assets map[string]*AssetRecord
}

// AssetRecord aggregates every declaration of one asset UUID.
type AssetRecord struct {
	ID          string
	Occurrences []AssetOccurrence
}

// NewWorkspace creates a workspace from ordered packages.
func NewWorkspace(pkgs ...*Package) *Workspace {
	return &Workspace{Packages: pkgs, Report: &Report{}, assets: map[string]*AssetRecord{}}
}

// AddPackage appends a package in inheritance order.
func (w *Workspace) AddPackage(p *Package) {
	w.Packages = append(w.Packages, p)
}

// AssetIDs returns all known asset UUIDs in sorted order.
func (w *Workspace) AssetIDs() []string {
	ids := make([]string, 0, len(w.assets))
	for id := range w.assets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Record returns the aggregated record for an asset id.
func (w *Workspace) Record(id string) *AssetRecord {
	return w.assets[NormalizeID(id)]
}

// BuildRegistry aggregates AssetMap + PKL data across all packages and emits
// missing/duplicate findings. It must be called before Resolve/Analyze.
func (w *Workspace) BuildRegistry() *Report {
	rep := w.Report
	if rep == nil {
		rep = &Report{}
		w.Report = rep
	}
	pathOwners := map[string][]string{}

	for _, pkg := range w.Packages {
		amIDs := map[string]int{}
		pklIDs := map[string]int{}

		if pkg.AssetMap != nil {
			for _, e := range pkg.AssetMap.Assets {
				if e.ID == "" {
					continue
				}
				amIDs[e.ID]++
				rec := w.touch(e.ID)
				occ := AssetOccurrence{
					PackageLabel: pkg.Label, PackageDir: pkg.Dir,
					InAssetMap: true, ChunkPaths: e.ChunkPaths,
				}
				for _, cp := range e.ChunkPaths {
					abs := cp
					if !filepath.IsAbs(abs) {
						abs = filepath.Join(pkg.Dir, cp)
					}
					if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
						occ.ResolvedPath = abs
						occ.FileExists = true
						pathOwners[filepath.Clean(abs)] = append(pathOwners[filepath.Clean(abs)], e.ID)
						break
					} else {
						rep.addMissing(MissingIssue{
							Kind: "file_not_found", AssetID: e.ID, Package: pkg.Label,
							Path:    abs,
							Message: fmt.Sprintf("AssetMap chunk %q for %s does not exist in package %q", cp, e.ID, pkg.Label),
						})
					}
				}
				if len(e.ChunkPaths) == 0 {
					rep.addWarning(WarningIssue{
						Kind: "asset_without_path", AssetID: e.ID, Package: pkg.Label,
						Message: "AssetMap entry has no Chunk/Path",
					})
				}
				rec.Occurrences = append(rec.Occurrences, occ)
			}
		}

		if pkg.PKL != nil {
			for _, e := range pkg.PKL.Assets {
				if e.ID == "" {
					continue
				}
				pklIDs[e.ID]++
				rec := w.touch(e.ID)
				idx := w.findOccurrenceIndex(rec, pkg.Label)
				if idx < 0 {
					rec.Occurrences = append(rec.Occurrences, AssetOccurrence{PackageLabel: pkg.Label, PackageDir: pkg.Dir})
					idx = len(rec.Occurrences) - 1
				}
				occ := &rec.Occurrences[idx]
				occ.InPKL = true
				occ.Hash = e.Hash
				occ.Size = e.Size
				occ.Type = e.Type
				if pkg.AssetMap == nil || amIDs[e.ID] == 0 {
					rep.addWarning(WarningIssue{
						Kind: "pkl_asset_without_assetmap", AssetID: e.ID, Package: pkg.Label,
						Message: "PKL lists asset but this package AssetMap does not map it (cross-package inheritance may still resolve it)",
					})
				}
			}
		}

		for id, n := range amIDs {
			if n > 1 {
				rep.addDuplicate(DuplicateIssue{
					Kind: "duplicate_assetmap_id", AssetID: id, Severity: SeverityWarning,
					Message: fmt.Sprintf("Asset UUID %s appears %d times in AssetMap of %q", id, n, pkg.Label),
				})
			}
		}
		for id, n := range pklIDs {
			if n > 1 {
				rep.addDuplicate(DuplicateIssue{
					Kind: "duplicate_pkl_id", AssetID: id, Severity: SeverityWarning,
					Message: fmt.Sprintf("Asset UUID %s appears %d times in PKL of %q", id, n, pkg.Label),
				})
			}
		}

		// Filename-UUID fallback: rescue resolution when ASSETMAP is absent
		// or its chunk path is stale but the UUID-named file is present.
		for id, paths := range pkg.FileIndex {
			rec := w.touch(id)
			idx := w.findOccurrenceIndex(rec, pkg.Label)
			if idx < 0 {
				rec.Occurrences = append(rec.Occurrences, AssetOccurrence{PackageLabel: pkg.Label, PackageDir: pkg.Dir})
				idx = len(rec.Occurrences) - 1
			}
			occ := &rec.Occurrences[idx]
			if occ.FileExists {
				continue
			}
			for _, p := range paths {
				if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
					occ.ResolvedPath = p
					occ.FileExists = true
					if len(occ.ChunkPaths) == 0 {
						if rel, rerr := filepath.Rel(pkg.Dir, p); rerr == nil {
							occ.ChunkPaths = []string{rel}
						}
					}
					break
				}
			}
		}
	}

	// Cross-package duplicate UUIDs.
	for _, id := range w.AssetIDs() {
		rec := w.assets[id]
		pkgSeen := map[string]bool{}
		var pkgs []string
		for _, o := range rec.Occurrences {
			if !pkgSeen[o.PackageLabel] {
				pkgSeen[o.PackageLabel] = true
				pkgs = append(pkgs, o.PackageLabel)
			}
		}
		if len(pkgs) > 1 {
			sev, msg := classifyOccurrences(rec)
			rep.addDuplicate(DuplicateIssue{
				Kind: "duplicate_asset_uuid", AssetID: id, Severity: sev,
				Message: msg, Occurrences: rec.Occurrences,
			})
		}
	}

	// Two different UUIDs mapped to the same file path.
	for p, ids := range pathOwners {
		uniq := uniqueStrings(ids)
		if len(uniq) > 1 {
			rep.addDuplicate(DuplicateIssue{
				Kind: "duplicate_path", Severity: SeverityConflict,
				Message: fmt.Sprintf("Path %s is mapped by multiple asset UUIDs: %s", p, strings.Join(uniq, ", ")),
			})
		}
	}

	// Same fixity hash declared under different asset UUIDs: likely the same
	// essence delivered twice with different identifiers.
	hashOwners := map[string][]string{}
	for _, id := range w.AssetIDs() {
		for _, o := range w.assets[id].Occurrences {
			if o.InPKL && o.Hash != "" {
				hashOwners[o.Hash] = append(hashOwners[o.Hash], id)
			}
		}
	}
	for h, ids := range hashOwners {
		uniq := uniqueStrings(ids)
		if len(uniq) > 1 {
			rep.addDuplicate(DuplicateIssue{
				Kind: "duplicate_content_hash", AssetID: uniq[0], Severity: SeverityWarning,
				Message: fmt.Sprintf("PKL hash %s is shared by distinct asset UUIDs %s", h, strings.Join(uniq, ", ")),
			})
		}
	}

	w.analyzeReferences()
	rep.finalizeCounts()
	return rep
}

func (w *Workspace) touch(id string) *AssetRecord {
	id = NormalizeID(id)
	rec := w.assets[id]
	if rec == nil {
		rec = &AssetRecord{ID: id}
		w.assets[id] = rec
	}
	return rec
}

func (w *Workspace) findOccurrenceIndex(rec *AssetRecord, label string) int {
	for i := range rec.Occurrences {
		if rec.Occurrences[i].PackageLabel == label {
			return i
		}
	}
	return -1
}

func classifyOccurrences(rec *AssetRecord) (string, string) {
	var hashes []string
	var sizes []int64
	pkgs := map[string]bool{}
	for _, o := range rec.Occurrences {
		pkgs[o.PackageLabel] = true
		if o.InPKL && o.Hash != "" {
			hashes = append(hashes, o.Hash)
		}
		if o.InPKL && o.Size > 0 {
			sizes = append(sizes, o.Size)
		}
	}
	hashConflict := len(uniqueStrings(hashes)) > 1
	sizeConflict := len(uniqueInt64(sizes)) > 1
	labels := make([]string, 0, len(pkgs))
	for p := range pkgs {
		labels = append(labels, p)
	}
	sort.Strings(labels)
	if hashConflict || sizeConflict {
		return SeverityConflict, fmt.Sprintf(
			"Asset %s is declared in packages %s with conflicting fixity (hash/size differs)",
			rec.ID, strings.Join(labels, ", "))
	}
	return SeverityIdentical, fmt.Sprintf(
		"Asset %s is inherited/reused across packages %s with consistent identity",
		rec.ID, strings.Join(labels, ", "))
}

// Resolve maps an asset UUID through PKL/AssetMap inheritance to a file.
func (w *Workspace) Resolve(id string) *ResolvedAsset {
	id = NormalizeID(id)
	res := &ResolvedAsset{AssetID: id}
	rec := w.assets[id]
	if rec == nil {
		return res
	}
	res.Found = true
	// Prefer an occurrence whose file actually exists; otherwise first.
	pick := -1
	for i, o := range rec.Occurrences {
		if o.InAssetMap {
			res.InAssetMap = true
		}
		if o.InPKL {
			res.InPKL = true
			res.PKLHash = o.Hash
			res.PKLSize = o.Size
		}
		if o.FileExists && pick < 0 {
			pick = i
		}
	}
	if pick < 0 && len(rec.Occurrences) > 0 {
		pick = 0
	}
	if pick >= 0 {
		o := rec.Occurrences[pick]
		res.PackageLabel = o.PackageLabel
		res.AbsolutePath = o.ResolvedPath
		res.FileExists = o.FileExists
		if len(o.ChunkPaths) > 0 {
			res.RelativePath = o.ChunkPaths[0]
		}
		if o.InPKL {
			res.PKLHash = o.Hash
			res.PKLSize = o.Size
		}
	}
	return res
}

func (w *Workspace) analyzeReferences() {
	rep := w.Report
	cplIDs := map[string]bool{}
	for _, pkg := range w.Packages {
		for _, cpl := range pkg.CPLs {
			cplIDs[cpl.ID] = true
			for _, seg := range cpl.Segments {
				for _, tr := range seg.Tracks {
					for _, res := range tr.Resources {
						if res.TrackFileID == "" {
							continue
						}
						if w.assets[res.TrackFileID] == nil {
							rep.addMissing(MissingIssue{
								Kind: "unresolved_reference", AssetID: res.TrackFileID,
								ReferencedBy: fmt.Sprintf("CPL %s / %s[%s] / resource %s",
									cpl.ID, tr.SequenceName, firstNonEmpty(tr.TrackID, tr.SequenceID), res.ID),
								Package: cpl.PackageLabel,
								Message: fmt.Sprintf("TrackFileId %s referenced by %s in CPL %s is not declared in any AssetMap/PKL of the merged packages",
									res.TrackFileID, tr.SequenceName, cpl.ID),
							})
						}
					}
				}
			}
		}
		for _, opl := range pkg.OPLs {
			for _, ref := range opl.CPLRefs {
				if !cplIDs[ref] {
					rep.addMissing(MissingIssue{
						Kind: "cpl_not_found", AssetID: ref, ReferencedBy: "OPL " + opl.ID,
						Package: opl.PackageLabel,
						Message: fmt.Sprintf("OPL %s references CompositionPlaylist %s which is not present in any package", opl.ID, ref),
					})
				}
			}
		}
	}
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func uniqueInt64(in []int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
