// Command renderbatch runs crash-safe, resumable batch rendering jobs with
// an append-only transaction log and an atomically committed manifest.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"renderbatch/internal/batch"
	"renderbatch/internal/spec"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = runCmd(os.Args[2:])
	case "verify":
		err = verifyCmd(os.Args[2:])
	case "inspect":
		err = inspectCmd(os.Args[2:])
	case "recover":
		err = recoverCmd(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "renderbatch: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `renderbatch - crash-safe resumable batch renderer

Usage:
  renderbatch run      --spec spec.json --out ./out [--fail path#index=N]... [--crash-after HOOK]
  renderbatch verify   --out ./out [--spec spec.json]
  renderbatch inspect  --out ./out
  renderbatch recover  --out ./out

Hooks for --crash-after:
  after_fragment_stage, after_fragment_wal, after_assemble,
  after_file_wal, after_manifest_write, after_batch_wal
`)
}

type runOptions struct {
	specPath   string
	outDir     string
	failures   map[string]int
	crashAfter string
}

func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	specPath := fs.String("spec", "spec.json", "path to the batch spec")
	outDir := fs.String("out", "out", "output directory")
	failFlag := fs.String("fail", "", "transient failure spec, e.g. report.pdf#2=1 (repeatable, comma separated)")
	crashHook := fs.String("crash-after", "", "abort once after the named hook to simulate a crash")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s, err := spec.Load(*specPath)
	if err != nil {
		return err
	}
	failures, err := parseFailures(*failFlag)
	if err != nil {
		return err
	}

	renderer := batch.Renderer(batch.NewDeterministicRenderer())
	if len(failures) > 0 {
		renderer = batch.NewFlakyRenderer(renderer, failures)
	}

	var hook batch.HookFunc
	if *crashHook != "" {
		fired := false
		hook = func(name string, _ batch.FileSpec, _ int) error {
			if !fired && name == *crashHook {
				fired = true
				return fmt.Errorf("%w: simulated crash at %s", errSimulatedCrash, name)
			}
			return nil
		}
	}

	engine, err := batch.NewEngine(batch.Config{
		OutputDir: *outDir,
		Renderer:  renderer,
		Hook:      hook,
	})
	if err != nil {
		return err
	}
	result, err := engine.Run(s)
	if err != nil {
		if errors.Is(err, errSimulatedCrash) {
			fmt.Fprintln(os.Stderr, "run aborted by simulated crash; rerun to resume")
		}
		return err
	}

	fmt.Printf("batch_id=%s resumed=%-5v fragments_rendered=%d files=%d digest=%s\n",
		result.BatchID, result.Resumed, result.FragmentsRendered,
		len(result.Manifest.Files), result.Manifest.Digest)
	return nil
}

var errSimulatedCrash = errors.New("simulated crash")

func parseFailures(raw string) (map[string]int, error) {
	out := map[string]int{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --fail entry %q (want path#index=N)", item)
		}
		count, err := strconv.Atoi(value)
		if err != nil || count < 0 {
			return nil, fmt.Errorf("invalid --fail count in %q", item)
		}
		out[key] = count
	}
	return out, nil
}
