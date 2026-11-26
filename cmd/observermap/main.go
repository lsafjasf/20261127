// Command observermap demonstrates an experimental empirical mapping between
// the CIE 1931 2° and CIE 1964 10° standard observers.
//
// It is deliberately explicit that its mapping is NOT a standard conversion
// and that missing ICC profiles or unsupported white points make results
// uncertain.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"observermap"
)

func main() {
	if len(os.Args) < 2 {
		runDemo(nil)
		return
	}
	switch os.Args[1] {
	case "demo":
		runDemo(os.Args[2:])
	case "inspect", "icc":
		runInspect(os.Args[2:])
	case "map":
		runMap(os.Args[2:])
	case "report":
		runReport(os.Args[2:])
	case "-h", "--help", "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprintln(w, `observermap — experimental CIE 2°↔10° observer mapping

USAGE
  observermap [demo] [--profile file.icc] [--json]
  observermap inspect <profile.icc> [--json] [--strict]
  observermap map (--hex '#rrggbb' | --x X --y Y --z Z) \
                 [--dir 2to10|10to2] [--wp D65|D50] \
                 [--profile file.icc] [--json] [--strict]
  observermap report [--out report.html] [--profile file.icc]

All observer mappings are empirical approximations, NOT CIE/ICC standards.
Missing/unreadable profiles and unsupported white points are flagged UNCERTAIN.`)
}

func runDemo(args []string) {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	profile := fs.String("profile", "", "path to an ICC profile (optional provenance input)")
	asJSON := fs.Bool("json", false, "emit JSON instead of text tables")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"profile": true})); err != nil {
		os.Exit(2)
	}
	rep, err := observermap.BuildDemo(*profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "demo failed: %v\n", err)
		os.Exit(1)
	}
	if *asJSON {
		must(observermap.WriteJSON(os.Stdout, rep))
		return
	}
	observermap.WriteDemoText(os.Stdout, rep)
}

func runInspect(args []string) {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	strict := fs.Bool("strict", false, "exit 3 when the assessment is uncertain")
	if err := fs.Parse(reorderFlags(args, nil)); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: observermap inspect <profile.icc>")
		os.Exit(2)
	}
	cert := observermap.AssessProfile(fs.Arg(0))
	if *asJSON {
		must(observermap.WriteJSON(os.Stdout, cert))
	} else {
		observermap.WriteCertaintyText(os.Stdout, cert)
	}
	if cert.ProfileError != "" {
		os.Exit(1)
	}
	if *strict && cert.Uncertain {
		os.Exit(3)
	}
}

func runMap(args []string) {
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	hex := fs.String("hex", "", "sRGB colour as #rgb/#rrggbb")
	x := fs.Float64("x", 0, "XYZ X (Y=100 scale); requires --y and --z")
	y := fs.Float64("y", 0, "XYZ Y")
	z := fs.Float64("z", 0, "XYZ Z")
	wp := fs.String("wp", "D65", "assumed white point when profile does not override: D65 or D50")
	dir := fs.String("dir", "2to10", "mapping direction: 2to10 (default) or 10to2 (--x/y/z only)")
	profile := fs.String("profile", "", "optional ICC profile")
	asJSON := fs.Bool("json", false, "emit JSON")
	strict := fs.Bool("strict", false, "exit 3 when uncertain")
	if err := fs.Parse(reorderFlags(args, map[string]bool{
		"hex": true, "x": true, "y": true, "z": true, "wp": true, "dir": true, "profile": true,
	})); err != nil {
		os.Exit(2)
	}

	kind, input, xyz, err := resolveMapInput(*hex, *x, *y, *z)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}
	res, err := observermap.MapQuery(kind, input, *dir, *wp, *profile, xyz)
	if err != nil {
		fmt.Fprintf(os.Stderr, "map failed: %v\n", err)
		os.Exit(1)
	}
	if *asJSON {
		must(observermap.WriteJSON(os.Stdout, res))
	} else {
		observermap.WriteMapResultText(os.Stdout, res)
	}
	if *strict && res.Uncertain {
		os.Exit(3)
	}
}

func resolveMapInput(hex string, x, y, z float64) (kind, input string, xyz observermap.XYZ, err error) {
	if hex != "" {
		if _, _, _, e := observermap.ParseHexRGB(hex); e != nil {
			return "", "", observermap.XYZ{}, fmt.Errorf("invalid --hex: %w", e)
		}
		return "hex", hex, observermap.XYZ{}, nil
	}
	if x != 0 || y != 0 || z != 0 {
		return "xyz", "xyz", observermap.XYZ{X: x, Y: y, Z: z}, nil
	}
	return "", "", observermap.XYZ{}, fmt.Errorf("provide --hex '#rrggbb' or non-zero --x/--y/--z")
}

func runReport(args []string) {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	out := fs.String("out", "observer-report.html", "output HTML path")
	profile := fs.String("profile", "", "optional ICC profile")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"out": true, "profile": true})); err != nil {
		os.Exit(2)
	}
	rep, err := observermap.BuildDemo(*profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "report failed: %v\n", err)
		os.Exit(1)
	}
	if err := observermap.WriteHTMLReport(*out, rep); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", *out)
}

// reorderFlags moves flag tokens (and their values for names in valueFlags)
// ahead of positional arguments so flags may appear after a positional path.
func reorderFlags(args []string, valueFlags map[string]bool) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && a[0] == '-' {
			name := a
			if len(name) > 1 && name[1] == '-' {
				name = name[2:]
			} else {
				name = name[1:]
			}
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name = name[:eq]
				flags = append(flags, a)
				continue
			}
			flags = append(flags, a)
			if valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "output failed: %v\n", err)
		os.Exit(1)
	}
}
