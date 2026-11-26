// mediaqacli validates one media file against a machine-readable contract.
//
// Usage:
//
//	mediaqacli -contract platform.json -media movie.mkv
//	mediaqacli -contract platform.json -probe probe.json
//
// Exit codes: 0 = all items PASS, 1 = at least one FAIL,
// 2 = UNKNOWN items without any FAIL, 3 = usage or input error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"mediaqacontract/checker"
	"mediaqacontract/contract"
	"mediaqacontract/media"
)

func main() {
	contractPath := flag.String("contract", "", "path to the JSON contract")
	mediaPath := flag.String("media", "", "media file to inspect with ffprobe")
	probePath := flag.String("probe", "", "pre-built tool-neutral probe JSON")
	ffprobeBin := flag.String("ffprobe", "ffprobe", "ffprobe binary path")
	flag.Parse()

	if *contractPath == "" || (*mediaPath == "" && *probePath == "") {
		flag.Usage()
		os.Exit(3)
	}

	contractData, err := os.ReadFile(*contractPath)
	if err != nil {
		fatal("read contract: %v", err)
	}
	spec, err := contract.Load(contractData)
	if err != nil {
		fatal("load contract: %v", err)
	}

	probe, err := loadProbe(*mediaPath, *probePath, *ffprobeBin)
	if err != nil {
		fatal("load probe: %v", err)
	}

	report := checker.Verify(spec, probe)
	out, err := report.JSON()
	if err != nil {
		fatal("render report: %v", err)
	}
	fmt.Println(string(out))

	pass, fail, unknown := report.Counts()
	fmt.Fprintf(os.Stderr, "summary: %d pass, %d fail, %d unknown; overall=%s\n",
		pass, fail, unknown, report.Overall)

	switch report.Overall {
	case checker.StatusPass:
		os.Exit(0)
	case checker.StatusFail:
		os.Exit(1)
	default:
		os.Exit(2)
	}
}

func loadProbe(mediaPath, probePath, bin string) (*media.Probe, error) {
	if probePath != "" {
		data, err := os.ReadFile(probePath)
		if err != nil {
			return nil, err
		}
		var probe media.Probe
		if err := json.Unmarshal(data, &probe); err != nil {
			return nil, fmt.Errorf("parse probe json: %w", err)
		}
		return &probe, nil
	}
	return media.ProbeWithFFProbe(bin, mediaPath)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(3)
}
