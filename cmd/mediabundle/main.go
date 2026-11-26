package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mediabundle/internal/bundle"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "pack":
		err = cmdPack(os.Args[2:])
	case "legacy-pack":
		err = cmdLegacyPack(os.Args[2:])
	case "diff":
		err = cmdDiff(os.Args[2:])
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "inspect":
		err = cmdInspect(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mediabundle:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `mediabundle - reproducible media bundling and difference classification

Usage:
  mediabundle pack        --src DIR --out FILE [--gzip]
  mediabundle legacy-pack --src DIR --out FILE [--gzip]
  mediabundle diff        --a FILE --b FILE [--json]
  mediabundle verify      --src DIR [--gzip]
  mediabundle inspect     --bundle FILE
`)
}

type packFlags struct {
	src   string
	out   string
	gzip  bool
	jsonO bool
	root  string
}

func bindPack(fs *flag.FlagSet) *packFlags {
	p := &packFlags{}
	fs.StringVar(&p.src, "src", "", "source directory to package")
	fs.StringVar(&p.out, "out", "", "output bundle path")
	fs.BoolVar(&p.gzip, "gzip", false, "gzip-compress the bundle (deterministic)")
	fs.BoolVar(&p.jsonO, "json", false, "emit machine-readable JSON summary")
	fs.StringVar(&p.root, "source-root", "",
		"provenance label stored in the manifest (defaults to the src base name)")
	return p
}

// sourceRootLabel returns explicit provenance only. The default is empty
// (omitted from the manifest) because any path derived from the local
// checkout would be machine-specific noise. This field never feeds the
// BundleID.
func sourceRootLabel(p *packFlags) string {
	return p.root
}

func compression(gz bool) string {
	if gz {
		return "gzip"
	}
	return ""
}

func cmdPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	p := bindPack(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if p.src == "" || p.out == "" {
		return fmt.Errorf("pack requires --src and --out")
	}
	manifest, err := bundle.Pack(p.src, p.out, bundle.Options{
		Compression: compression(p.gzip), SourceRoot: sourceRootLabel(p),
	})
	if err != nil {
		return err
	}
	hash, err := bundle.HashFile(p.out)
	if err != nil {
		return err
	}
	if p.jsonO {
		return emitJSON(map[string]any{
			"out": p.out, "sha256": hash, "bundleId": manifest.BundleID,
			"entries": len(manifest.Entries),
		})
	}
	fmt.Printf("out=%s sha256=%s bundleId=%s entries=%d\n",
		p.out, hash, manifest.BundleID, len(manifest.Entries))
	return nil
}

func cmdLegacyPack(args []string) error {
	fs := flag.NewFlagSet("legacy-pack", flag.ContinueOnError)
	p := bindPack(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if p.src == "" || p.out == "" {
		return fmt.Errorf("legacy-pack requires --src and --out")
	}
	manifest, err := bundle.LegacyPack(p.src, p.out, bundle.Options{
		Compression: compression(p.gzip), SourceRoot: sourceRootLabel(p),
	})
	if err != nil {
		return err
	}
	hash, err := bundle.HashFile(p.out)
	if err != nil {
		return err
	}
	if p.jsonO {
		return emitJSON(map[string]any{
			"out": p.out, "sha256": hash, "bundleId": manifest.BundleID,
			"packedAt": manifest.PackedAt, "entries": len(manifest.Entries),
		})
	}
	fmt.Printf("out=%s sha256=%s bundleId=%s packedAt=%s entries=%d\n",
		p.out, hash, manifest.BundleID, manifest.PackedAt, len(manifest.Entries))
	return nil
}

func cmdDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	a := fs.String("a", "", "first bundle")
	b := fs.String("b", "", "second bundle")
	asJSON := fs.Bool("json", false, "emit the full structured report as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *a == "" || *b == "" {
		return fmt.Errorf("diff requires --a and --b")
	}
	report, err := bundle.Compare(*a, *b)
	if err != nil {
		return err
	}
	if *asJSON {
		return emitJSON(report)
	}
	printReport(report)
	return nil
}

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	src := fs.String("src", "", "source directory to verify reproducibility on")
	gz := fs.Bool("gzip", false, "verify the gzip-compressed layout too")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *src == "" {
		return fmt.Errorf("verify requires --src")
	}
	dir, err := os.MkdirTemp("", "mediabundle-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	out1 := filepath.Join(dir, "one.tar")
	out2 := filepath.Join(dir, "two.tar")
	if _, err := bundle.Pack(*src, out1, bundle.Options{}); err != nil {
		return err
	}
	if _, err := bundle.Pack(*src, out2, bundle.Options{}); err != nil {
		return err
	}
	h1, _ := bundle.HashFile(out1)
	h2, _ := bundle.HashFile(out2)

	result := map[string]any{
		"tarRun1":      h1,
		"tarRun2":      h2,
		"reproducible": h1 == h2,
	}
	if *gz {
		g1 := filepath.Join(dir, "one.tar.gz")
		g2 := filepath.Join(dir, "two.tar.gz")
		if _, err := bundle.Pack(*src, g1, bundle.Options{Compression: "gzip"}); err != nil {
			return err
		}
		if _, err := bundle.Pack(*src, g2, bundle.Options{Compression: "gzip"}); err != nil {
			return err
		}
		gh1, _ := bundle.HashFile(g1)
		gh2, _ := bundle.HashFile(g2)
		result["gzipRun1"] = gh1
		result["gzipRun2"] = gh2
		result["gzipReproducible"] = gh1 == gh2
	}

	ok, _ := result["reproducible"].(bool)
	if *gz {
		gok, _ := result["gzipReproducible"].(bool)
		ok = ok && gok
	}
	result["ok"] = ok
	if err := emitJSON(result); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("reproducibility check failed")
	}
	return nil
}

func cmdInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	path := fs.String("bundle", "", "bundle file to inspect")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return fmt.Errorf("inspect requires --bundle")
	}
	pb, err := bundle.ReadBundle(*path)
	if err != nil {
		return err
	}
	return emitJSON(map[string]any{
		"path":           pb.Path,
		"gzip":           pb.Gzip,
		"legacy":         pb.Legacy,
		"bundleId":       pb.BundleID,
		"packedAt":       pb.PackedAt,
		"containerOrder": pb.EntryOrder,
		"manifestOrder":  pb.ManifestOrder(),
		"entries":        pb.Entries,
	})
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printReport(r *bundle.Report) {
	fmt.Printf("verdict:            %s\n", r.Verdict)
	fmt.Printf("rawIdentical:       %t\n", r.RawIdentical)
	fmt.Printf("mediaEquivalent:    %t\n", r.MediaEquivalent)
	fmt.Printf("bundleIdDifference: %t  (%s vs %s)\n", r.BundleIDDifference, r.BundleIDA, r.BundleIDB)
	fmt.Printf("containerOrderDiff: %t\n", r.ContainerOrderDiff)
	fmt.Printf("packagingTimestamps: %d\n", len(r.PackagingTimestamps))
	for _, d := range r.PackagingTimestamps {
		fmt.Printf("  - %s: %s != %s\n", d.Where, d.A, d.B)
	}
	fmt.Printf("sourceMetadataDiffs: %d\n", len(r.SourceMetadataDiffs))
	for _, d := range r.SourceMetadataDiffs {
		fmt.Printf("  - %s %s: %s != %s\n", d.Path, d.Field, d.A, d.B)
	}
	fmt.Printf("mediaDiffs:         %d\n", len(r.MediaDiffs))
	for _, d := range r.MediaDiffs {
		fmt.Printf("  - %s: %s\n", d.Path, d.Kind)
	}
}
