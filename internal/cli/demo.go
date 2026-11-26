package cli

import (
	"flag"
	"fmt"
	"path/filepath"

	"proalpha/internal/check"
	"proalpha/internal/fixture"
)

func runDemo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	dir := fs.String("dir", "examples/demo", "demo output root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	fixtureDir := filepath.Join(*dir, "fixtures")
	reportDir := filepath.Join(*dir, "report")
	paths, err := fixture.Generate(fixtureDir, 4, 96, 54)
	if err != nil {
		return err
	}
	report, err := check.Run(check.Options{
		SourcePath:   paths.SourceProRes,
		VariantPaths: []string{paths.DroppedProxy, paths.PremulProxy},
		OutputDir:    reportDir,
		MaxFrames:    4,
		RepairVideo:  true,
	})
	if err != nil {
		return err
	}
	fmt.Print(check.RenderText(report))
	fmt.Printf("\nDemo report: %s\n", filepath.Join(reportDir, "report.json"))
	return nil
}
