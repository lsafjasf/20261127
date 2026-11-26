package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"evidencechain/internal/chain"
)

func cmdReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	c := fs.String("case", "case", "案例目录")
	out := fs.String("out", "FORENSIC-REPORT.md", "报告输出路径")
	prefix := fs.String("link-prefix", "case/", "报告中案例文件链接前缀")
	if err := fs.Parse(args); err != nil {
		return err
	}
	md, v, err := chain.RenderReport(*c, *prefix)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
		return err
	}
	if err := chain.WriteJSON(filepath.Join(*c, "out", "verification.json"), v); err != nil {
		return err
	}
	fmt.Println("report written:", *out)
	fmt.Println("verification JSON:", filepath.Join(*c, "out", "verification.json"))
	if !v.OK {
		fmt.Fprintln(os.Stderr, "WARNING: chain has", len(v.Errors), "error(s); report still generated for review")
		os.Exit(3)
	}
	return nil
}
