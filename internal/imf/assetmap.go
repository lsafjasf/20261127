package imf

import (
	"os"
	"path/filepath"
)

func parseAssetMapFile(path string) (*AssetMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	root, err := ParseNode(f)
	if err != nil {
		return nil, err
	}
	am := &AssetMap{SourceFile: path}
	am.ID = NormalizeID(root.ChildText("Id"))
	if am.ID == "" {
		am.ID = NormalizeID(root.ChildText("AssetMapId"))
	}
	for _, assetNode := range root.All("Asset") {
		e := &MapEntry{ID: NormalizeID(assetNode.ChildText("Id"))}
		if pt := assetNode.First("PackingList"); pt != nil {
			e.PackingList = pt.Text == "true" || pt.Text == "1"
		}
		if cl := assetNode.First("ChunkList"); cl != nil {
			for _, chunk := range cl.All("Chunk") {
				if p := chunk.ChildText("Path"); p != "" {
					e.ChunkPaths = append(e.ChunkPaths, filepath.FromSlash(p))
				}
			}
		}
		if len(e.ChunkPaths) == 0 {
			if p := assetNode.ChildText("Path"); p != "" {
				e.ChunkPaths = append(e.ChunkPaths, filepath.FromSlash(p))
			}
		}
		am.Assets = append(am.Assets, e)
	}
	return am, nil
}

func parsePKLFile(path string) (*PKL, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	root, err := ParseNode(f)
	if err != nil {
		return nil, err
	}
	pkl := &PKL{SourceFile: path}
	pkl.ID = NormalizeID(root.ChildText("Id"))
	for _, assetNode := range root.All("Asset") {
		e := &PKLEntry{
			ID:               NormalizeID(assetNode.ChildText("Id")),
			Hash:             assetNode.ChildText("Hash"),
			Type:             assetNode.ChildText("Type"),
			OriginalFilename: assetNode.ChildText("OriginalFilename"),
			SourceFile:       path,
		}
		if sz := assetNode.ChildText("Size"); sz != "" {
			e.Size = parseInt64(sz)
		}
		pkl.Assets = append(pkl.Assets, e)
	}
	return pkl, nil
}

func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
