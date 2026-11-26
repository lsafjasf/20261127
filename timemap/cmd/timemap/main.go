package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"uid4-0019/timemap"
)

func main() {
	outDir := flag.String("out", "artifacts/timemap", "output directory for CSV and Markdown reports")
	flag.Parse()

	edit, refresh, tracks, points := timemap.DefaultScenario()
	pairs, findings := timemap.BuildPairs(tracks, edit, refresh, points)

	timemap.WriteConsoleTable(os.Stdout, pairs)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output dir: %v\n", err)
		os.Exit(1)
	}
	csvPath := filepath.Join(*outDir, "timemap.csv")
	if err := timemap.WriteCSV(csvPath, pairs); err != nil {
		fmt.Fprintf(os.Stderr, "write csv: %v\n", err)
		os.Exit(1)
	}

	mdPath := filepath.Join(*outDir, "findings.md")
	md, err := os.Create(mdPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create markdown: %v\n", err)
		os.Exit(1)
	}
	timemap.WriteMarkdown(md, edit, refresh, tracks, points, findings)
	if err := md.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close markdown: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("findings:")
	for _, f := range findings {
		fmt.Printf("- %s: stage=%s cause=%s divergent=%d/%d\n",
			f.Track, f.Stage, f.Cause, f.DivergentPoints, f.TotalPoints)
	}
	fmt.Printf("\nwrote %s\nwrote %s\n", csvPath, mdPath)
}
