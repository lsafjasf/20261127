package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

type PKLInfo struct {
	Size int64
	Hash string
	Type string
}

type ChunkRef struct {
	Path   string
	Length int64
}

type Package struct {
	Root         string
	CPLPath      string
	PKLPath      string
	AssetMapPath string
	CPL          *xmlNode
	PKL          *xmlNode
	AssetMap     *xmlNode
	PKLAssets    map[string]PKLInfo
	Chunks       map[string][]ChunkRef
}

func childTextAny(node *xmlNode, names ...string) string {
	for _, name := range names {
		if value := node.childText(name); value != "" {
			return value
		}
	}
	return ""
}

func discoverCPL(root string) (string, error) {
	paths, err := findFiles(root, looksLikeCPL)
	if err != nil {
		return "", err
	}
	if len(paths) == 1 {
		return paths[0], nil
	}
	if len(paths) > 1 {
		return "", fmt.Errorf("%d CPL files found in %s; pass -cpl", len(paths), root)
	}

	xmlPaths, err := findFiles(root, func(path string) bool {
		return strings.EqualFold(filepath.Ext(path), ".xml")
	})
	if err != nil {
		return "", err
	}
	var matches []string
	for _, path := range xmlPaths {
		node, err := readXML(path)
		if err == nil && node.localName() == "CompositionPlaylist" {
			matches = append(matches, path)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%d CompositionPlaylist documents found in %s; pass -cpl", len(matches), root)
	}
	return "", fmt.Errorf("no CPL file found in %s", root)
}

func LoadPackage(root, cplPath string) (*Package, error) {
	var err error
	if strings.TrimSpace(cplPath) == "" {
		cplPath, err = discoverCPL(root)
		if err != nil {
			return nil, err
		}
	} else {
		cplPath, err = filepath.Abs(cplPath)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(root) == "" {
			root = filepath.Dir(cplPath)
		}
	}
	if strings.TrimSpace(root) == "" {
		root = filepath.Dir(cplPath)
	}

	cpl, err := readXML(cplPath)
	if err != nil {
		return nil, err
	}

	pkg := &Package{
		Root:      root,
		CPLPath:   cplPath,
		CPL:       cpl,
		PKLAssets: make(map[string]PKLInfo),
		Chunks:    make(map[string][]ChunkRef),
	}

	pkg.PKLPath, err = firstFile(root, func(path string) bool {
		name := strings.ToLower(filepath.Base(path))
		return strings.HasPrefix(name, "pkl_") && strings.HasSuffix(name, ".xml")
	})
	if err != nil {
		return nil, err
	}
	if pkg.PKLPath != "" {
		pkg.PKL, err = readXML(pkg.PKLPath)
		if err != nil {
			return nil, err
		}
		readPKL(pkg)
	}

	pkg.AssetMapPath, err = firstFile(root, func(path string) bool {
		name := strings.ToLower(filepath.Base(path))
		return name == "assetmap" || strings.HasPrefix(name, "assetmap.")
	})
	if err != nil {
		return nil, err
	}
	if pkg.AssetMapPath != "" {
		pkg.AssetMap, err = readXML(pkg.AssetMapPath)
		if err != nil {
			return nil, err
		}
		readAssetMap(pkg)
	}

	return pkg, nil
}

func readPKL(pkg *Package) {
	assetList := pkg.PKL.child("AssetList")
	if assetList == nil {
		assetList = pkg.PKL.child("AssetList")
	}
	if assetList == nil {
		return
	}
	for _, asset := range assetList.children("Asset") {
		id := normalizeID(childTextAny(&asset, "Id", "ID"))
		if id == "" {
			continue
		}
		info := PKLInfo{
			Hash: childTextAny(&asset, "Hash"),
			Type: childTextAny(&asset, "Type"),
		}
		if sizeText := childTextAny(&asset, "Size"); sizeText != "" {
			_, _ = fmt.Sscanf(sizeText, "%d", &info.Size)
		}
		pkg.PKLAssets[id] = info
	}
}

func readAssetMap(pkg *Package) {
	assetList := pkg.AssetMap.child("AssetList")
	if assetList == nil {
		assetList = pkg.AssetMap.child("Assets")
	}
	if assetList == nil {
		return
	}
	for _, asset := range assetList.children("Asset") {
		id := normalizeID(childTextAny(&asset, "Id", "ID"))
		if id == "" {
			continue
		}
		chunks := asset.child("ChunkList")
		if chunks == nil {
			continue
		}
		for _, chunk := range chunks.children("Chunk") {
			path := filepath.FromSlash(chunk.childText("Path"))
			if path == "" {
				continue
			}
			chunkRef := ChunkRef{Path: path}
			if length, hasLength, err := chunk.childInt("Length"); err == nil && hasLength {
				chunkRef.Length = length
			}
			pkg.Chunks[id] = append(pkg.Chunks[id], chunkRef)
		}
	}
}
