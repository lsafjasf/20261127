// Command subdeliver produces matching embedded and sidecar subtitle
// deliverables from one source manifest and blocks release when the two
// diverge in language, timing or text.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"subdeliver/internal/embedded"
	"subdeliver/internal/manifest"
	"subdeliver/internal/sidecar"
	"subdeliver/internal/verify"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "subdeliver: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch args[0] {
	case "build":
		return cmdBuild(args[1:])
	case "verify":
		return cmdVerify(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `subdeliver - dual subtitle delivery from one source manifest

Usage:
  subdeliver build  -manifest <manifest.json> -out <dir>
  subdeliver verify -manifest <manifest.json> -out <dir>

build   generates sidecar (.srt/.vtt) and embedded (.esub) deliverables,
        then re-parses every output and blocks (non-zero exit) on any
        language, timing or text mismatch.
verify  re-parses existing deliverables in <out> against the manifest.
`)
}

type options struct {
	manifestPath string
	outDir       string
}

func parseFlags(args []string) (options, error) {
	var o options
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-manifest", "--manifest":
			i++
			if i >= len(args) {
				return o, fmt.Errorf("-manifest needs a value")
			}
			o.manifestPath = args[i]
		case "-out", "--out":
			i++
			if i >= len(args) {
				return o, fmt.Errorf("-out needs a value")
			}
			o.outDir = args[i]
		default:
			return o, fmt.Errorf("unknown flag %q", args[i])
		}
	}
	if o.manifestPath == "" || o.outDir == "" {
		return o, fmt.Errorf("-manifest and -out are required")
	}
	return o, nil
}

func cmdBuild(args []string) error {
	o, err := parseFlags(args)
	if err != nil {
		return err
	}
	m, err := manifest.Load(o.manifestPath)
	if err != nil {
		return err
	}
	sets, err := m.Build()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(o.outDir, 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	for _, set := range sets {
		base := filepath.Join(o.outDir, m.MediaID+"."+set.Language)
		if err := os.WriteFile(base+".srt", sidecar.RenderSRT(set), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(base+".vtt", sidecar.RenderVTT(set), 0o644); err != nil {
			return err
		}
	}
	esubPath := filepath.Join(o.outDir, m.MediaID+".esub")
	container, err := embedded.Marshal(m.MediaID, sets)
	if err != nil {
		return err
	}
	if err := os.WriteFile(esubPath, container, 0o644); err != nil {
		return err
	}

	report := runVerification(m, sets, o.outDir)
	if !report.OK() {
		return fmt.Errorf("release blocked, deliverables do not match source manifest:\n%s",
			report.Text())
	}
	fmt.Printf("build OK: %d track(s), sidecar + embedded verified in %s\n",
		len(sets), o.outDir)
	return nil
}

func cmdVerify(args []string) error {
	o, err := parseFlags(args)
	if err != nil {
		return err
	}
	m, err := manifest.Load(o.manifestPath)
	if err != nil {
		return err
	}
	sets, err := m.Build()
	if err != nil {
		return err
	}
	report := runVerification(m, sets, o.outDir)
	if !report.OK() {
		return fmt.Errorf("verification failed:\n%s", report.Text())
	}
	fmt.Println("verify OK: all deliverables match the source manifest")
	return nil
}

// runVerification re-parses every generated artifact through its own
// independent reader and compares it cue by cue with the canonical source.
func runVerification(m *manifest.Manifest, sets []manifest.CueSet, outDir string) *verify.Report {
	report := &verify.Report{}

	for _, set := range sets {
		base := filepath.Join(outDir, m.MediaID+"."+set.Language)
		srtData, err := os.ReadFile(base + ".srt")
		if err != nil {
			report.Add("sidecar:srt", set.Language, 0, "file", "readable", err.Error())
		} else if cues, err := sidecar.ParseSRT(srtData, set.Language); err != nil {
			report.Add("sidecar:srt", set.Language, 0, "parse", "valid", err.Error())
		} else {
			report.Sidecar("sidecar:srt", set, cues)
		}

		vttData, err := os.ReadFile(base + ".vtt")
		if err != nil {
			report.Add("sidecar:vtt", set.Language, 0, "file", "readable", err.Error())
		} else if cues, err := sidecar.ParseVTT(vttData, set.Language); err != nil {
			report.Add("sidecar:vtt", set.Language, 0, "parse", "valid", err.Error())
		} else {
			report.Sidecar("sidecar:vtt", set, cues)
		}
	}

	esubPath := filepath.Join(outDir, m.MediaID+".esub")
	data, err := os.ReadFile(esubPath)
	if err != nil {
		report.Add("embedded", "-", 0, "file", "readable", err.Error())
		return report
	}
	gotMediaID, gotSets, err := embedded.Unmarshal(data)
	if err != nil {
		report.Add("embedded", "-", 0, "parse", "valid", err.Error())
		return report
	}
	if gotMediaID != m.MediaID {
		report.Add("embedded", "-", 0, "media_id", m.MediaID, gotMediaID)
	}
	report.Embedded(sets, gotSets)
	return report
}
