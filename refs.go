package main

import (
	"os"
	"path/filepath"
)

func checkReference(pkg *Package, id string) ReferenceReport {
	ref := ReferenceReport{Paths: []PathReport{}}

	pklInfo, inPKL := pkg.PKLAssets[id]
	ref.InPKL = inPKL
	if pkg.PKL != nil && !inPKL {
		ref.Problems = append(ref.Problems, "asset Id is absent from PKL")
	}

	chunks, inAssetMap := pkg.Chunks[id]
	ref.InAssetMap = inAssetMap
	if pkg.AssetMap != nil && !inAssetMap {
		ref.Problems = append(ref.Problems, "asset Id is absent from AssetMap")
	}
	if inAssetMap && len(chunks) == 0 {
		ref.Problems = append(ref.Problems, "AssetMap entry has no Chunk/Path")
	}

	assetMapDir := pkg.Root
	if pkg.AssetMapPath != "" {
		assetMapDir = filepath.Dir(pkg.AssetMapPath)
	}

	var actualTotal int64
	allExist := true
	for _, chunk := range chunks {
		resolved := chunk.Path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(assetMapDir, filepath.FromSlash(chunk.Path))
		}
		pathReport := PathReport{
			Path:         filepath.ToSlash(chunk.Path),
			ResolvedPath: resolved,
			SizeOK:       true,
		}
		if chunk.Length > 0 {
			pathReport.ExpectedSize = chunk.Length
		}

		info, err := os.Stat(resolved)
		if err != nil {
			allExist = false
			ref.Problems = append(ref.Problems, "chunk is not readable: "+chunk.Path)
			pathReport.SizeOK = false
		} else {
			pathReport.Exists = true
			pathReport.ActualSize = info.Size()
			actualTotal += info.Size()
			if chunk.Length > 0 && info.Size() != chunk.Length {
				pathReport.SizeOK = false
				ref.Problems = append(ref.Problems, "chunk length differs from AssetMap: "+chunk.Path)
			}
		}
		ref.Paths = append(ref.Paths, pathReport)
	}

	if allExist && inPKL && pklInfo.Size > 0 && actualTotal != pklInfo.Size {
		ref.Problems = append(ref.Problems, "chunk total size differs from PKL")
	}

	return ref
}
