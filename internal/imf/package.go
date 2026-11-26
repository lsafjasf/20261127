package imf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadPackage scans one IMF package directory and parses its ASSETMAP, PKL,
// CPL and OPL documents. The directory is only ever read.
func LoadPackage(dir, label string, role PackageRole) (*Package, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("package dir %q: %w", dir, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("package path %q is not a directory", dir)
	}
	if label == "" {
		label = filepath.Base(abs)
	}
	pkg := &Package{Dir: abs, Label: label, Role: role}
	pkg.FileIndex = map[string][]string{}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !isXMLCandidate(name) {
			continue
		}
		full := filepath.Join(abs, name)
		kind, err := sniffXMLKind(full)
		if err != nil {
			// Not a parseable XML document; skip non-fataly.
			continue
		}
		pkg.XMLFiles = append(pkg.XMLFiles, full)
		switch kind {
		case "assetmap":
			am, err := parseAssetMapFile(full)
			if err != nil {
				return nil, fmt.Errorf("parse AssetMap %s: %w", name, err)
			}
			pkg.AssetMap = am
		case "pkl":
			pkl, err := parsePKLFile(full)
			if err != nil {
				return nil, fmt.Errorf("parse PKL %s: %w", name, err)
			}
			pkg.PKL = pkl
		case "cpl":
			cpl, err := parseCPLFile(full, label)
			if err != nil {
				return nil, fmt.Errorf("parse CPL %s: %w", name, err)
			}
			pkg.CPLs = append(pkg.CPLs, cpl)
		case "opl":
			opl, err := parseOPLFile(full, label)
			if err != nil {
				return nil, fmt.Errorf("parse OPL %s: %w", name, err)
			}
			pkg.OPLs = append(pkg.OPLs, opl)
		}
	}

	// Filename-UUID fallback index, useful for packages with missing or
	// partial ASSETMAP/PKL metadata.
	_ = filepath.WalkDir(abs, func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d == nil {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != abs {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		for _, id := range ExtractAnyUUIDs(d.Name()) {
			pkg.FileIndex[id] = append(pkg.FileIndex[id], path)
		}
		return nil
	})
	return pkg, nil
}

func isXMLCandidate(name string) bool {
	upper := strings.ToUpper(name)
	if strings.HasSuffix(upper, ".XML") {
		return true
	}
	if upper == "ASSETMAP" || strings.HasPrefix(upper, "ASSETMAP.") {
		return true
	}
	return false
}

// LooksLikePackage reports whether dir contains at least one recognizable
// IMF document (AssetMap/PKL/CPL/OPL).
func LooksLikePackage(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, de := range entries {
		if de.IsDir() || strings.HasPrefix(de.Name(), ".") || !isXMLCandidate(de.Name()) {
			continue
		}
		if kind, err := sniffXMLKind(filepath.Join(dir, de.Name())); err == nil && !strings.HasPrefix(kind, "unknown") {
			return true
		}
	}
	return false
}

// DiscoverPackageDirs returns immediate subdirectories of root that look like
// IMF packages. It is used when a caller points the tool at a folder
// containing the main package and one or more supplemental packages.
func DiscoverPackageDirs(root string) ([]string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, de := range entries {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		child := filepath.Join(abs, de.Name())
		if LooksLikePackage(child) {
			dirs = append(dirs, child)
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func sniffXMLKind(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	root, err := ParseNode(f)
	if err != nil {
		return "", err
	}
	if root == nil {
		return "", fmt.Errorf("empty xml")
	}
	ln := strings.ToLower(root.Local)
	switch {
	case strings.Contains(ln, "assetmap"):
		return "assetmap", nil
	case strings.Contains(ln, "packinglist"):
		return "pkl", nil
	case strings.Contains(ln, "compositionplaylist"):
		return "cpl", nil
	case strings.Contains(ln, "outputprofilelist"):
		return "opl", nil
	default:
		return "unknown:" + root.Local, nil
	}
}
