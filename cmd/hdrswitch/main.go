package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"hdrswitch"
)

type jsonOutput struct {
	Report *hdrswitch.Report     `json:"report"`
	Plan   *hdrswitch.RepairPlan `json:"plan,omitempty"`
}

func main() {
	inPath := flag.String("in", "", "path to AU/GOP/metadata evidence JSON")
	sample := flag.String("sample", "risky", "built-in sample when -in is empty: risky or safe")
	asJSON := flag.Bool("json", false, "emit report JSON")
	withPlan := flag.Bool("plan", false, "include/print remediation plan")
	planOut := flag.String("plan-out", "", "write remediation plan JSON to this path")
	check := flag.Bool("check", false, "exit 1 when report contains error-severity findings")
	flag.Parse()

	if err := run(*inPath, *sample, *asJSON, *withPlan, *planOut, *check); err != nil {
		fmt.Fprintln(os.Stderr, "hdrswitch:", err)
		os.Exit(2)
	}
}

func run(inPath, sampleName string, asJSON, withPlan bool, planOut string, check bool) error {
	var evidence hdrswitch.Evidence
	if inPath != "" {
		data, err := os.ReadFile(inPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &evidence); err != nil {
			return fmt.Errorf("parse evidence: %w", err)
		}
	} else {
		switch sampleName {
		case "risky", "inside-gop", "":
			evidence = hdrswitch.SampleEvidence()
		case "safe", "idr":
			evidence = hdrswitch.SafeSampleEvidence()
		case "valid", "pass", "correct-player":
			evidence = hdrswitch.ValidSampleEvidence()
		case "gamut", "p3":
			evidence = hdrswitch.GamutChangeSampleEvidence()
		default:
			return fmt.Errorf("unknown -sample %q (want risky, safe, valid, or gamut)", sampleName)
		}
	}

	report, err := hdrswitch.Analyze(evidence)
	if err != nil {
		return err
	}

	var plan *hdrswitch.RepairPlan
	if withPlan || planOut != "" {
		plan, report, err = hdrswitch.PlanRepair(evidence)
		if err != nil {
			return err
		}
		if planOut != "" {
			if err := writeJSON(planOut, plan); err != nil {
				return err
			}
		}
	}

	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(jsonOutput{Report: report, Plan: plan}); err != nil {
			return err
		}
	} else {
		fmt.Print(hdrswitch.RenderText(report))
		if plan != nil {
			fmt.Print(hdrswitch.RenderRepairPlan(plan))
			if planOut != "" {
				fmt.Printf("\n修复计划 JSON 已写入: %s\n", planOut)
			}
		}
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
