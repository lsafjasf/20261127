package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"mkvidentity/identity"
	"mkvidentity/matroska"
)

type inspectEnvelope struct {
	Inventory *identity.Inventory `json:"inventory"`
	Graph     identity.Graph      `json:"graph"`
}

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	inventoryOut := fs.String("inventory-out", "", "write normalized inventory JSON")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("inspect requires one MKV or inventory JSON path")
	}
	inv, err := matroska.LoadInventory(fs.Arg(0))
	if err != nil {
		return err
	}
	if *inventoryOut != "" {
		if err := matroska.SaveInventoryJSON(*inventoryOut, inv); err != nil {
			return err
		}
	}
	graph := identity.BuildGraph(inv)
	envelope := inspectEnvelope{Inventory: inv, Graph: graph}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(envelope)
	}
	printInventoryText(inv, &graph)
	return nil
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	strict := fs.Bool("strict", false, "fail on unresolved items as well as errors")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("validate requires one MKV or inventory JSON path")
	}
	inv, err := matroska.LoadInventory(fs.Arg(0))
	if err != nil {
		return err
	}
	graph := identity.BuildGraph(inv)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(graph); err != nil {
			return err
		}
	} else {
		printGraphText(&graph)
	}
	hasError := false
	hasUnresolved := false
	for _, issue := range graph.Issues {
		if issue.Severity == identity.SeverityError {
			hasError = true
		}
		if issue.Severity == identity.SeverityUnresolved {
			hasUnresolved = true
		}
	}
	if hasError || (*strict && hasUnresolved) {
		os.Exit(1)
	}
	return nil
}
