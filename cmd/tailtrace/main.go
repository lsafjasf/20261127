package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"tailtrace"
)

type jsonOutput struct {
	Report      *tailtrace.Report  `json:"report"`
	Fix         *tailtrace.FixPlan `json:"fix,omitempty"`
	FixedReport *tailtrace.Report  `json:"fixed_report,omitempty"`
}

func main() {
	inPath := flag.String("in", "", "path to evidence JSON; omit to run the built-in two-black-frame sample")
	asJSON := flag.Bool("json", false, "emit JSON")
	fixMode := flag.String("fix", "none", "remediation plan: none, trim, contract")
	check := flag.Bool("check", false, "exit 1 when the evidence violates the supplied output contract")
	fixOut := flag.String("fix-out", "", "write fixed evidence (trim) or revised contract (contract) as JSON")
	flag.Parse()

	if err := run(*inPath, *asJSON, *fixMode, *check, *fixOut); err != nil {
		fmt.Fprintln(os.Stderr, "tailtrace:", err)
		os.Exit(2)
	}
}

func run(inPath string, asJSON bool, fixMode string, check bool, fixOut string) error {
	var evidence tailtrace.Evidence
	if inPath != "" {
		data, err := os.ReadFile(inPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &evidence); err != nil {
			return fmt.Errorf("parse evidence: %w", err)
		}
	} else {
		evidence = tailtrace.SampleEvidence()
	}

	report, err := tailtrace.Analyze(evidence)
	if err != nil {
		return err
	}

	var plan *tailtrace.FixPlan
	var fixedReport *tailtrace.Report
	switch fixMode {
	case "", "none":
	case "trim":
		plan, err = tailtrace.PlanTrim(evidence)
		if err != nil {
			return err
		}
		fixedReport, err = tailtrace.Analyze(plan.FixedEvidence)
		if err != nil {
			return err
		}
		if fixOut != "" {
			if err := writeJSON(fixOut, plan.FixedEvidence); err != nil {
				return err
			}
		}
	case "contract":
		plan, err = tailtrace.PlanContract(evidence)
		if err != nil {
			return err
		}
		if fixOut != "" {
			if err := writeJSON(fixOut, plan.RevisedContract); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported -fix %q (want none, trim, contract)", fixMode)
	}

	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(jsonOutput{Report: report, Fix: plan, FixedReport: fixedReport})
	}

	fmt.Print(tailtrace.RenderText(report))
	if plan != nil {
		fmt.Printf("\n修复方案 [%s]: %s\n", plan.Mode, plan.Action)
		fmt.Printf("  %s\n", plan.Explanation)
		if len(plan.RemoveIndices) > 0 {
			fmt.Printf("  删除样本索引: %v\n", plan.RemoveIndices)
		}
		if fixOut != "" {
			fmt.Printf("  JSON 已写入: %s\n", fixOut)
		}
	}
	if fixedReport != nil {
		fmt.Println("\n修剪后复检:")
		fmt.Print(tailtrace.RenderText(fixedReport))
	}
	if check && !report.OK {
		os.Exit(1)
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
