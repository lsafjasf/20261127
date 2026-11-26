package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"audiomap/routing"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "apply":
		return runApply(args[1:], stdout, stderr)
	case "render":
		return runRender(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspacePath := flags.String("workspace", "", "workspace JSON file")
	asJSON := flags.Bool("json", false, "emit machine-readable diagnostics")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "check: unexpected argument %q\n", flags.Arg(0))
		return 2
	}
	if err := requireWorkspace(*workspacePath); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	workspace, err := routing.LoadWorkspaceFile(*workspacePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	issues := workspace.Diagnostics()
	if *asJSON {
		if issues == nil {
			issues = []routing.Issue{}
		}
		result := struct {
			Valid  bool            `json:"valid"`
			Tracks int             `json:"tracks"`
			Buses  int             `json:"buses"`
			Routes int             `json:"routes"`
			Issues []routing.Issue `json:"issues"`
		}{
			Valid:  len(issues) == 0,
			Tracks: len(workspace.Tracks),
			Buses:  len(workspace.Buses),
			Routes: len(workspace.Routes),
			Issues: issues,
		}
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else if len(issues) == 0 {
		fmt.Fprintf(stdout, "OK: %d track(s), %d bus(es), %d route(s); no cycles or missing endpoints\n",
			len(workspace.Tracks), len(workspace.Buses), len(workspace.Routes))
	} else {
		printIssues(stderr, issues)
	}
	if len(issues) > 0 {
		return 1
	}
	return 0
}

func runApply(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspacePath := flags.String("workspace", "", "workspace JSON file")
	opsPath := flags.String("ops", "", "operations JSON file")
	outputPath := flags.String("out", "", "updated workspace path (default stdout)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "apply: unexpected argument %q\n", flags.Arg(0))
		return 2
	}
	if err := requireWorkspace(*workspacePath); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *opsPath == "" {
		fmt.Fprintln(stderr, "apply: -ops is required")
		return 2
	}
	workspace, err := routing.LoadWorkspaceFile(*workspacePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	operations, err := routing.LoadOperationsFile(*opsPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	updated, err := routing.ApplyOperations(workspace, operations)
	if err != nil {
		fmt.Fprintln(stderr, "batch rejected; workspace unchanged")
		printError(stderr, err)
		return 1
	}
	if *outputPath == "" {
		if err := routing.SaveWorkspace(stdout, updated); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	err = writeAtomic(*outputPath, func(writer io.Writer) error {
		return routing.SaveWorkspace(writer, updated)
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stderr, "applied %d operation(s) to %s\n", len(operations), *outputPath)
	return 0
}

func runRender(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspacePath := flags.String("workspace", "", "workspace JSON file")
	opsPath := flags.String("ops", "", "optional operations JSON file")
	outputPath := flags.String("out", "", "HTML or SVG path (default stdout)")
	format := flags.String("format", "html", "output format: html or svg")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "render: unexpected argument %q\n", flags.Arg(0))
		return 2
	}
	if err := requireWorkspace(*workspacePath); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *format != "html" && *format != "svg" {
		fmt.Fprintln(stderr, "render: -format must be html or svg")
		return 2
	}
	workspace, err := routing.LoadWorkspaceFile(*workspacePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *opsPath != "" {
		operations, loadErr := routing.LoadOperationsFile(*opsPath)
		if loadErr != nil {
			fmt.Fprintln(stderr, loadErr)
			return 1
		}
		workspace, err = routing.ApplyOperations(workspace, operations)
		if err != nil {
			fmt.Fprintln(stderr, "batch rejected; refusing to render a partial migration")
			printError(stderr, err)
			return 1
		}
	}

	if issues := workspace.Diagnostics(); len(issues) > 0 {
		fmt.Fprintf(stderr, "warning: workspace has %d issue(s); rendering diagnostics for review\n", len(issues))
	}
	render := func(writer io.Writer) error {
		if *format == "svg" {
			return routing.RenderSVG(writer, workspace)
		}
		return routing.RenderHTML(writer, workspace)
	}
	if *outputPath == "" {
		if err := render(stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if err := writeAtomic(*outputPath, render); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stderr, "wrote %s\n", *outputPath)
	return 0
}

func requireWorkspace(path string) error {
	if path == "" {
		return errors.New("-workspace is required")
	}
	return nil
}

func printError(writer io.Writer, err error) {
	var diagnostics *routing.DiagnosticsError
	if errors.As(err, &diagnostics) {
		printIssues(writer, diagnostics.Issues)
		return
	}
	fmt.Fprintln(writer, err)
}

func printIssues(writer io.Writer, issues []routing.Issue) {
	for _, issue := range issues {
		fmt.Fprintf(writer, "[%s] %s\n", issue.Code, issue.Message)
	}
}

func writeAtomic(path string, write func(io.Writer) error) error {
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := write(temp); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		return fmt.Errorf("chmod output: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace output: %w", err)
	}
	return nil
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, `audio routing mapper

Usage:
  routing check  -workspace workspace.json [-json]
  routing apply  -workspace workspace.json -ops operations.json [-out updated.json]
  routing render -workspace workspace.json [-ops operations.json] [-format html|svg] [-out map.html]

All node and route endpoints resolve by stable IDs, never display names.
An apply batch is rejected in full when any operation adds a missing endpoint or cycle.`)
}
