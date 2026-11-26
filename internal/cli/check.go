package cli

import (
	"flag"
	"fmt"

	"proalpha/internal/check"
)

type stringList []string

func (values *stringList) String() string {
	return fmt.Sprint([]string(*values))
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	source := fs.String("source", "", "reference ProRes MOV/MP4 with color and alpha markers")
	out := fs.String("out", "alpha-output", "report and repaired-frame output directory")
	maxFrames := fs.Int("max-frames", 3, "maximum initial frames to decode per variant")
	noExitCode := fs.Bool("no-exit-code", false, "do not exit non-zero when error-level findings exist")
	repairVideo := fs.Bool("repair-video", false, "also encode repaired-alpha-proxy.mov from the reference source")
	var variants stringList
	fs.Var(&variants, "variant", "converted proxy path; repeat for multiple variants")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *source == "" && len(variants) == 0 && fs.NArg() == 0 {
		return fmt.Errorf("check requires --source and/or at least one --variant")
	}

	paths := append([]string{}, variants...)
	paths = append(paths, fs.Args()...)
	report, err := check.Run(check.Options{
		SourcePath:   *source,
		VariantPaths: paths,
		OutputDir:    *out,
		MaxFrames:    *maxFrames,
		RepairVideo:  *repairVideo,
	})
	if err != nil {
		return err
	}
	fmt.Print(check.RenderText(report))
	if report.Status == "fail" && !*noExitCode {
		return fmt.Errorf("error-level findings written to %s", *out)
	}
	return nil
}
