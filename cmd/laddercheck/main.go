package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	lc "laddercheck"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(0)
	}
	var err error
	switch os.Args[1] {
	case "analyze":
		err = runAnalyze(os.Args[2:])
	case "demo":
		err = runDemo(os.Args[2:])
	case "inspect":
		err = runInspect(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		var ce *checkError
		if errors.As(err, &ce) {
			fmt.Fprintln(os.Stderr, ce.Error())
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "laddercheck: %v\n", err)
		os.Exit(2)
	}
}

type checkError struct{ msg string }

func (e *checkError) Error() string { return e.msg }

func usage() {
	fmt.Print(`laddercheck — align transcoding-ladder keyframes without guessing

Usage:
  laddercheck analyze [flags] <file.mp4|dir>...
  laddercheck demo    [flags]
  laddercheck inspect <file.mp4>

analyze flags:
  -in PATH          JSON manifest/evidence (see examples/ladder.evidence.json)
  -gop RAT          unified GOP in seconds, e.g. 2 or 3000/1001
  -fps RAT          common frame rate, e.g. 30/1 or 30000/1001
  -gop-frames N     unified GOP length in frames (wins with -fps over -gop)
  -tol RAT          alignment tolerance seconds (default: half frame or 1ms)
  -origin RAT       grid origin seconds (default 0)
  -horizon RAT      emit explicit grid times up to origin+horizon
  -r                recurse into directories
  -json PATH        write JSON report; "-" prints JSON to stdout only
  -md PATH          write Markdown report
  -check            exit 1 unless every rung is compliant (safe to switch now)

demo flags:
  -dir PATH         fixture MP4 output dir (default artifacts/demo-ladder)
  -json PATH        JSON report (default artifacts/demo-report.json)
  -md PATH          Markdown report (default artifacts/demo-report.md)

inspect prints normalized per-track facts as JSON.
`)
}

type analyzeCfg struct {
	in        string
	gop, fps  string
	gopFrames int64
	tol       string
	origin    string
	horizon   string
	recursive bool
	jsonOut   string
	mdOut     string
	check     bool
}

func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	var cfg analyzeCfg
	fs.StringVar(&cfg.in, "in", "", "JSON manifest")
	fs.StringVar(&cfg.gop, "gop", "", "GOP seconds")
	fs.StringVar(&cfg.fps, "fps", "", "frame rate")
	fs.Int64Var(&cfg.gopFrames, "gop-frames", 0, "GOP frames")
	fs.StringVar(&cfg.tol, "tol", "", "tolerance seconds")
	fs.StringVar(&cfg.origin, "origin", "", "origin seconds")
	fs.StringVar(&cfg.horizon, "horizon", "", "horizon seconds")
	fs.BoolVar(&cfg.recursive, "r", false, "recursive directory scan")
	fs.StringVar(&cfg.jsonOut, "json", "", "JSON report path or -")
	fs.StringVar(&cfg.mdOut, "md", "", "Markdown report path")
	fs.BoolVar(&cfg.check, "check", false, "fail unless switch-safe now")
	if err := fs.Parse(args); err != nil {
		return err
	}

	doc, opts, err := loadManifest(cfg.in)
	if err != nil {
		return err
	}
	if err := applyRatFlags(fs, &opts, cfg); err != nil {
		return err
	}

	paths, err := collectPaths(fs.Args(), cfg.recursive)
	if err != nil {
		return err
	}
	facts, _, err := lc.LoadLadder(doc, paths)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		return fmt.Errorf("no renditions to analyze (pass media paths or -in manifest)")
	}

	rep := lc.EvaluateLadder(manifestLabel(cfg.in, fs.Args()), facts, opts)
	return outputReport(rep, cfg.jsonOut, cfg.mdOut, cfg.check)
}

func loadManifest(path string) (*lc.LadderInput, lc.ConstraintOptions, error) {
	var opts lc.ConstraintOptions
	if path == "" {
		return nil, opts, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, opts, err
	}
	var doc lc.LadderInput
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, opts, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Constraint != nil {
		opts = *doc.Constraint
	}
	return &doc, opts, nil
}

func applyRatFlags(fs *flag.FlagSet, opts *lc.ConstraintOptions, cfg analyzeCfg) error {
	parse := func(name, s string, dst *lc.Rat) error {
		if s == "" {
			return nil
		}
		r, err := lc.ParseRat(s)
		if err != nil {
			return fmt.Errorf("-%s: %w", name, err)
		}
		*dst = r
		return nil
	}
	if err := parse("gop", cfg.gop, &opts.GOPSeconds); err != nil {
		return err
	}
	if err := parse("fps", cfg.fps, &opts.FPS); err != nil {
		return err
	}
	if cfg.gopFrames > 0 {
		opts.GOPFrames = cfg.gopFrames
	}
	if err := parse("tol", cfg.tol, &opts.ToleranceSeconds); err != nil {
		return err
	}
	if err := parse("origin", cfg.origin, &opts.OriginSeconds); err != nil {
		return err
	}
	if err := parse("horizon", cfg.horizon, &opts.HorizonSeconds); err != nil {
		return err
	}
	return nil
}

func collectPaths(args []string, recursive bool) ([]string, error) {
	var paths []string
	for _, a := range args {
		st, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if st.IsDir() {
			found, err := lc.ScanMedia(a, recursive)
			if err != nil {
				return nil, err
			}
			paths = append(paths, found...)
		} else {
			paths = append(paths, a)
		}
	}
	return paths, nil
}

func manifestLabel(in string, args []string) string {
	if in != "" {
		return in
	}
	if len(args) > 0 {
		return args[0]
	}
	return "cli"
}

func outputReport(rep *lc.Report, jsonOut, mdOut string, check bool) error {
	if mdOut != "" {
		if err := os.MkdirAll(filepath.Dir(mdOut), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(mdOut, []byte(lc.RenderMarkdown(rep)), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote Markdown report: %s\n", mdOut)
	}
	switch jsonOut {
	case "-":
		raw, err := lc.PrettyJSON(rep)
		if err != nil {
			return err
		}
		fmt.Println(string(raw))
		return checkGate(rep, check)
	case "":
		fmt.Print(lc.RenderText(rep))
	default:
		if err := os.MkdirAll(filepath.Dir(jsonOut), 0o755); err != nil {
			return err
		}
		raw, err := lc.PrettyJSON(rep)
		if err != nil {
			return err
		}
		if err := os.WriteFile(jsonOut, raw, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote JSON report: %s\n", jsonOut)
		fmt.Print(lc.RenderText(rep))
	}
	return checkGate(rep, check)
}

func checkGate(rep *lc.Report, check bool) error {
	if check && !rep.Summary.SwitchSafe {
		return &checkError{msg: "laddercheck -check: ladder is NOT switch-safe now (see requires_reencode / remux_only lists)"}
	}
	return nil
}

func runDemo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	dir := fs.String("dir", "artifacts/demo-ladder", "fixture directory")
	jsonOut := fs.String("json", "artifacts/demo-report.json", "JSON report")
	mdOut := fs.String("md", "artifacts/demo-report.md", "Markdown report")
	constraintOut := fs.String("constraint", "artifacts/unified-keyframes.json", "constraint JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rep, paths, err := lc.GenerateDemo(*dir)
	if err != nil {
		return err
	}
	for _, p := range paths {
		fmt.Fprintf(os.Stderr, "wrote fixture rung: %s\n", p)
	}
	if *constraintOut != "" {
		if err := os.MkdirAll(filepath.Dir(*constraintOut), 0o755); err != nil {
			return err
		}
		raw, _ := lc.PrettyJSON(rep.Constraint)
		if err := os.WriteFile(*constraintOut, raw, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote constraint: %s\n", *constraintOut)
	}
	return outputReport(rep, *jsonOut, *mdOut, false)
}

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("inspect requires exactly one media file")
	}
	f, err := lc.ParseMP4File(fs.Arg(0))
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}
