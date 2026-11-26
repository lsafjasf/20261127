package imf

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WriteOptions controls side effects in the output directory.
type WriteOptions struct {
	CopyAssets bool
	Now        func() time.Time
}

type playlistEnvelope struct {
	Tool        string    `json:"tool"`
	GeneratedAt time.Time `json:"generated_at"`
	*Playlist
}

// EnsureOutputSeparate refuses to use a package directory (or a directory
// inside one) as the output location, so original packages are never
// overwritten.
func EnsureOutputSeparate(outDir string, pkgDirs []string) error {
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	for _, d := range pkgDirs {
		absPkg, err := filepath.Abs(d)
		if err != nil {
			return err
		}
		if samePath(absOut, absPkg) || isWithin(absOut, absPkg) {
			return fmt.Errorf("output dir %q must not be the package dir %q or inside it (refusing to overwrite original package)", absOut, absPkg)
		}
	}
	return nil
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func isWithin(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, "../")
}

// WriteOutputs writes playlist.json, report.json/txt and CPL_final.xml into
// a fresh/separate output directory. Source packages are never modified.
func WriteOutputs(outDir string, pl *Playlist, rep *Report, wopts WriteOptions) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	now := time.Now
	if wopts.Now != nil {
		now = wopts.Now
	}

	env := playlistEnvelope{Tool: "imfplaylist", GeneratedAt: now().UTC(), Playlist: pl}
	if err := writeJSON(filepath.Join(outDir, "playlist.json"), env); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "report.json"), rep); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.txt"), []byte(RenderTextReport(rep, pl)), 0o644); err != nil {
		return err
	}
	if err := WriteMergedCPL(filepath.Join(outDir, "CPL_final.xml"), pl); err != nil {
		return err
	}
	if wopts.CopyAssets {
		if err := copyReferencedAssets(outDir, pl); err != nil {
			return err
		}
		// Re-write playlist.json so output_path entries are persisted.
		if err := writeJSON(filepath.Join(outDir, "playlist.json"), env); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func copyReferencedAssets(outDir string, pl *Playlist) error {
	assetDir := filepath.Join(outDir, "assets")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		return err
	}
	usedNames := map[string]string{}
	for _, a := range pl.Assets {
		if a == nil || !a.Found || !a.FileExists || a.AbsolutePath == "" {
			continue
		}
		base := filepath.Base(a.AbsolutePath)
		destName := base
		if owner, clash := usedNames[destName]; clash && owner != a.AssetID {
			destName = shortID(a.AssetID) + "_" + base
		}
		usedNames[destName] = a.AssetID
		dest := filepath.Join(assetDir, destName)
		if err := copyFile(a.AbsolutePath, dest); err != nil {
			return fmt.Errorf("copy asset %s: %w", a.AssetID, err)
		}
		rel, _ := filepath.Rel(outDir, dest)
		a.OutputPath = rel
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
