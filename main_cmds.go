package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"renderbatch/internal/batch"
	"renderbatch/internal/spec"
)

func verifyCmd(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	outDir := fs.String("out", "out", "output directory")
	specPath := fs.String("spec", "", "optional spec for full re-render verification")
	if err := fs.Parse(args); err != nil {
		return err
	}

	manifest, err := batch.ReadManifest(*outDir)
	if err != nil {
		return err
	}
	for _, entry := range manifest.Files {
		raw, err := os.ReadFile(filepath.Join(*outDir, "files", entry.Path))
		if err != nil {
			return fmt.Errorf("verify %s: %w", entry.Path, err)
		}
		sum := sha256.Sum256(raw)
		got := hex.EncodeToString(sum[:])
		if got != entry.SHA256 {
			return fmt.Errorf("verify %s: file hash mismatch (%s != %s)", entry.Path, got, entry.SHA256)
		}
		if int64(len(raw)) != entry.Size {
			return fmt.Errorf("verify %s: size mismatch", entry.Path)
		}
	}

	if *specPath != "" {
		s, err := spec.Load(*specPath)
		if err != nil {
			return err
		}
		specHash, err := batch.CanonicalSpecHash(s)
		if err != nil {
			return err
		}
		if specHash != manifest.SpecHash {
			return fmt.Errorf("spec hash %s does not match manifest %s", specHash, manifest.SpecHash)
		}
		renderer := batch.NewDeterministicRenderer()
		for _, file := range s.Files {
			for index := 0; index < file.Fragments; index++ {
				length := batch.FragmentLength(file, index)
				data, err := renderer.Render(file, index, length)
				if err != nil {
					return err
				}
				sum := sha256.Sum256(data)
				want := findFragmentHash(manifest, file.Path, index)
				if want == "" || want != hex.EncodeToString(sum[:]) {
					return fmt.Errorf("re-render mismatch at %s fragment %d", file.Path, index)
				}
			}
		}
	}

	fmt.Printf("OK: %d files verified, manifest digest %s\n", len(manifest.Files), manifest.Digest)
	return nil
}

func findFragmentHash(m batch.Manifest, path string, index int) string {
	for _, entry := range m.Files {
		if entry.Path != path {
			continue
		}
		for _, frag := range entry.Fragments {
			if frag.Index == index {
				return frag.SHA256
			}
		}
	}
	return ""
}

func inspectCmd(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	outDir := fs.String("out", "out", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	report := map[string]any{"dir": *outDir}
	manifest, err := batch.ReadManifest(*outDir)
	if err == nil {
		report["manifest"] = manifest
	} else {
		report["manifest"] = fmt.Sprintf("not committed: %v", err)
	}

	state, validBytes, err := batch.InspectWAL(filepath.Join(*outDir, "txn"))
	if err != nil {
		return err
	}
	report["wal_valid_bytes"] = validBytes
	report["wal_committed"] = state.Committed
	report["wal_batch_id"] = state.BatchID
	report["wal_files"] = state.Files

	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}

func recoverCmd(args []string) error {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	specPath := fs.String("spec", "spec.json", "path to the batch spec")
	outDir := fs.String("out", "out", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := spec.Load(*specPath)
	if err != nil {
		return err
	}
	engine, err := batch.NewEngine(batch.Config{
		OutputDir: *outDir,
		Renderer:  batch.NewDeterministicRenderer(),
	})
	if err != nil {
		return err
	}
	result, err := engine.Run(s)
	if err != nil {
		return err
	}
	fmt.Printf("recovered batch_id=%s fragments_rendered=%d digest=%s\n",
		result.BatchID, result.FragmentsRendered, result.Manifest.Digest)
	return nil
}
