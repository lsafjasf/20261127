package cli

import (
	"flag"
	"fmt"

	"proalpha/internal/fixture"
)

func runFixtures(args []string) error {
	fs := flag.NewFlagSet("fixtures", flag.ContinueOnError)
	dir := fs.String("dir", "testdata/fixtures", "directory for generated PNG and MOV fixtures")
	frames := fs.Int("frames", 4, "number of frames")
	width := fs.Int("width", 96, "frame width")
	height := fs.Int("height", 54, "frame height")
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths, err := fixture.Generate(*dir, *frames, *width, *height)
	if err != nil {
		return err
	}
	fmt.Printf("fixtures written under %s\n", paths.Root)
	fmt.Printf("source: %s\n", paths.SourceProRes)
	fmt.Printf("dropped-alpha/color variant: %s\n", paths.DroppedProxy)
	fmt.Printf("premultiplied variant: %s\n", paths.PremulProxy)
	return nil
}
