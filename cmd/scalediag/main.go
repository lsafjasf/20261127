package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/lsafjasf/20261127/internal/analyzer"
)

const usage = `scalediag compares source frames with a scaled display capture.

Usage:
  scalediag --source DIR_OR_FILES --scaled DIR_OR_FILES [options]

Each --source/--scaled value may be repeated or comma-separated. Directories are
expanded and naturally sorted, so frame_2.png comes before frame_10.png. The two
sequences must contain the same number of frames and cover the same viewport.

Examples:
  scalediag --source native/ --scaled captured/ --roi 120,80,640,200
  scalediag --source a.png,b.png,c.png --scaled ../scaled/ --json

Options:
  --source string       source frame directory or comma-separated image paths
  --scaled string       scaled frame directory or comma-separated image paths
  --roi x,y,w,h         analysis region in source-frame pixels
  --block-sizes string  candidate codec block pitches (default "8,16,32")
  --json                emit JSON instead of text
`

type pathList []string

func (p *pathList) String() string { return strings.Join(*p, ",") }

func (p *pathList) Set(value string) error {
	*p = append(*p, value)
	return nil
}

func main() {
	var sources, scaled pathList
	var roiText, blockText string
	var jsonOutput bool
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usage)
	}
	flag.Var(&sources, "source", "source frame directory or comma-separated image paths")
	flag.Var(&scaled, "scaled", "scaled frame directory or comma-separated image paths")
	flag.StringVar(&roiText, "roi", "", "analysis region in source-frame pixels: x,y,w,h")
	flag.StringVar(&blockText, "block-sizes", "8,16,32", "candidate codec block pitches")
	flag.BoolVar(&jsonOutput, "json", false, "emit JSON")
	flag.Parse()

	if len(sources) == 0 || len(scaled) == 0 {
		fmt.Fprintln(os.Stderr, "error: --source and --scaled are required")
		flag.Usage()
		os.Exit(2)
	}
	roi, err := parseROI(roiText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	blockSizes, err := parseBlockSizes(blockText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	report, err := analyzer.Analyze(analyzer.Options{
		SourcePaths: sources,
		ScaledPaths: scaled,
		ROI:         roi,
		BlockSizes:  blockSizes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if jsonOutput {
		err = analyzer.WriteJSON(os.Stdout, report)
	} else {
		analyzer.WriteText(os.Stdout, report)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: write report: %v\n", err)
		os.Exit(2)
	}
}

func parseROI(value string) (*analyzer.Rect, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return nil, fmt.Errorf("--roi must be x,y,w,h")
	}
	values := make([]int, 4)
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("--roi component %q is not an integer", part)
		}
		values[i] = n
	}
	if values[2] <= 0 || values[3] <= 0 {
		return nil, fmt.Errorf("--roi width and height must be positive")
	}
	return &analyzer.Rect{X: values[0], Y: values[1], W: values[2], H: values[3]}, nil
}

func parseBlockSizes(value string) ([]int, error) {
	var sizes []int
	for _, part := range strings.Split(value, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 4 {
			return nil, fmt.Errorf("invalid --block-sizes value %q", part)
		}
		sizes = append(sizes, n)
	}
	return sizes, nil
}
