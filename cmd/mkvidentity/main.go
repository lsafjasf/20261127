package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "inspect":
		err = runInspect(os.Args[2:])
	case "validate":
		err = runValidate(os.Args[2:])
	case "migrate":
		err = runMigrate(os.Args[2:])
	case "apply":
		err = runApply(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "mkvidentity:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `mkvidentity — stable Matroska track/chapter identity migration

Usage:
  mkvidentity inspect <file.mkv|inventory.json> [--json] [--inventory-out inv.json]
  mkvidentity validate <file.mkv|inventory.json> [--json] [--strict]
  mkvidentity migrate --old old.mkv --new new.mkv [options]
  mkvidentity apply --input target.mkv --patch patch.json --out migrated.mkv

Migrate options:
  --legacy-rules rules.json   resolve old line numbers into UIDs once
  --timeline map.json         cut/offset map for target timeline
  --fps 24000/1001            frame rate for HH:MM:SS:FF legacy times
  --map-track OLD=NEW         explicit TrackUID map (repeatable)
  --map-chapter OLD=NEW       explicit ChapterUID map (repeatable)
  --map-edition OLD=NEW       explicit EditionUID map (repeatable)
  --copy-chapters             copy/remap chapters when target has none
  --language                  migrate source language tags (default true)
  --default-flags             migrate default-track flags (default true)
  --clear-competing-default   clear other defaults of same type (default true)
  --patch patch.json          write EBML patch document
  --report report.json        write machine-readable migration report
  --out migrated.mkv          rewrite target MKV directly in pure Go
  --json                      print JSON report to stdout
`)
}
