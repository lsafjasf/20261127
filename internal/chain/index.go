// Package chain builds the hash index and verifies the complete evidence
// chain: sealed originals, derived links, renamed files, rearranged
// containers, localized tampering, watchlist hashes and human judgments.
package chain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"evidencechain/internal/digestx"
	"evidencechain/internal/fuzzyx"
	"evidencechain/internal/model"
)

// IndexRel is the case-relative path of the hash index.
const IndexRel = "index/hash-index.json"

// BuildIndex hashes every file under 00-originals and writes the index.
func BuildIndex(caseDir, caseID string, chunkSize int) (*model.Index, error) {
	if chunkSize <= 0 {
		chunkSize = 256
	}
	originalsDir := filepath.Join(caseDir, "00-originals")
	var entries []model.IndexEntry
	err := filepath.WalkDir(originalsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		base := filepath.Base(path)
		id := strings.SplitN(base, "_", 2)[0]
		chunks := digestx.ChunkBytes(data, chunkSize)
		recs := make([]model.ChunkRecord, 0, len(chunks))
		for _, c := range chunks {
			recs = append(recs, model.ChunkRecord{
				Index: c.Index, Start: c.Start, End: c.End, Size: c.Size, SHA256: c.SHA256,
			})
		}
		fz := fuzzyx.Sum(data)
		relPath, rerr := filepath.Rel(caseDir, path)
		if rerr != nil {
			return rerr
		}
		entries = append(entries, model.IndexEntry{
			EvidenceID:  id,
			LogicalName: base,
			RelPath:     filepath.ToSlash(relPath),
			Size:        int64(len(data)),
			SHA256:      digestx.SHA256Bytes(data),
			ChunkRoot:   digestx.ChunkRoot(chunks),
			Chunks:      recs,
			Fuzzy: model.FuzzyRecord{
				Algorithm: fz.Algorithm, SimHash: fz.SimHash,
				Features: fz.Features, Unique: fz.Unique,
			},
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no original materials found under %s", originalsDir)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].EvidenceID < entries[j].EvidenceID })

	idx := &model.Index{
		Schema: model.Schema, Tool: model.ToolName, Version: model.ToolVersion,
		CaseID: caseID, Algorithm: "sha256", ChunkSize: chunkSize,
		ChunkRootAlg: "sha256-domain-separated",
		Note:         "信任基准：00-originals 下的原始材料；任何对该层的改动都会导致封印校验失败。",
		Entries:      entries,
	}
	if err := WriteJSON(filepath.Join(caseDir, IndexRel), idx); err != nil {
		return nil, err
	}
	return idx, nil
}

// LoadIndex reads the hash index.
func LoadIndex(caseDir string) (*model.Index, error) {
	var idx model.Index
	if err := ReadJSON(filepath.Join(caseDir, IndexRel), &idx); err != nil {
		return nil, fmt.Errorf("load hash index (run `index` first): %w", err)
	}
	return &idx, nil
}

// WriteJSON writes indented JSON, creating parent directories.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// ReadJSON reads a JSON file into v.
func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
