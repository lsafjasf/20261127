package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"hdrframe"
)

const helpText = `hdrframe — HDR10+ 帧级动态元数据/场景号/生效时间核对

用法:
  hdrframe sample [-dir examples]
  hdrframe audit -input examples/samples.json [-json report.json] [-text report.txt] [-strict]
  hdrframe demo

子命令也可写成 generate / run。仅使用 Go 标准库。
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, helpText)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "sample", "generate":
		err = runSample(os.Args[2:])
	case "audit", "run":
		err = runAudit(os.Args[2:])
	case "demo":
		err = runDemo()
	case "-h", "--help", "help":
		fmt.Print(helpText)
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		var coded exitCode
		if errors.As(err, &coded) {
			fmt.Fprintln(os.Stderr, "错误:", coded.err)
			os.Exit(coded.code)
		}
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func runSample(args []string) error {
	fs := flag.NewFlagSet("sample", flag.ContinueOnError)
	dir := fs.String("dir", "examples", "sample output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := hdrframe.WriteSampleDir(*dir); err != nil {
		return err
	}
	fmt.Printf("已写入重复/缺帧/跳号样本到 %s\n", *dir)
	return nil
}

func runAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	input := fs.String("input", "examples/samples.json", "single timeline or sample batch JSON")
	jsonOut := fs.String("json", "", "write machine-readable JSON report here")
	textOut := fs.String("text", "", "write Chinese text report here")
	strict := fs.Bool("strict", false, "exit 3 when an applied-version mismatch is found")
	format := fs.String("format", "auto", "input format: auto|canonical|hdr10plus")
	fpsOverride := fs.String("fps", "", "override fps, e.g. 24000/1001")
	if err := fs.Parse(args); err != nil {
		return err
	}
	file, err := hdrframe.LoadSampleFileAuto(*input, *format, *fpsOverride)
	if err != nil {
		return err
	}
	if *fpsOverride != "" {
		for i := range file.Samples {
			file.Samples[i].FPS = *fpsOverride
		}
	}
	batch, err := hdrframe.AuditInputs(file.Samples...)
	if err != nil {
		return err
	}
	if *jsonOut != "" {
		if err := hdrframe.WriteJSON(*jsonOut, batch); err != nil {
			return err
		}
	}
	text := batch.Text()
	if *textOut != "" {
		if err := os.WriteFile(*textOut, []byte(text), 0o644); err != nil {
			return err
		}
	}
	fmt.Print(text)
	if *strict && batch.HasMismatch() {
		return exitCode{code: 3, err: errors.New("发现实际应用版本与应有参数不一致")}
	}
	return nil
}

func runDemo() error {
	if err := hdrframe.WriteSampleDir("examples"); err != nil {
		return err
	}
	batch, err := hdrframe.AuditInputs(hdrframe.Samples()...)
	if err != nil {
		return err
	}
	if err := hdrframe.WriteJSON("artifacts/hdrframe-report.json", batch); err != nil {
		return err
	}
	if err := os.WriteFile("artifacts/hdrframe-report.txt", []byte(batch.Text()), 0o644); err != nil {
		return err
	}
	fmt.Println("样本: examples/{duplicate,missing-frame,sequence-jump,samples}.json")
	fmt.Println("报告: artifacts/hdrframe-report.{json,txt}")
	fmt.Println()
	fmt.Print(batch.Text())
	return nil
}

type exitCode struct {
	code int
	err  error
}

func (e exitCode) Error() string { return e.err.Error() }
