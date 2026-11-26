package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"rendercheckpoint/checkpoint"
)

type commonFlags struct {
	root string
	job  string
	json bool
}

func bindCommonFlags(flags *flag.FlagSet) *commonFlags {
	common := &commonFlags{}
	flags.StringVar(&common.root, "root", ".render-workspace", "checkpoint workspace root")
	flags.StringVar(&common.job, "job", "demo-book", "job id")
	flags.BoolVar(&common.json, "json", false, "emit JSON")
	return common
}

func statusCommand(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	common := bindCommonFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	store, err := checkpoint.OpenStore(common.root)
	if err != nil {
		return err
	}
	checkpointState, err := store.LoadCheckpoint(common.job)
	if err != nil {
		return err
	}

	if common.json {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(checkpointState)
	}

	ids := make([]string, 0, len(checkpointState.Nodes))
	for id := range checkpointState.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Printf("Job: %s\nUpdated: %s\n\n", checkpointState.JobID, checkpointState.UpdatedAt)
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "NODE\tSTATUS\tATTEMPT\tARTIFACT\tERROR")
	for _, id := range ids {
		node := checkpointState.Nodes[id]
		artifact := "-"
		if node.Artifact != nil {
			artifact = shortDigest(node.Artifact)
		}
		errorText := node.LastError
		if len(errorText) > 80 {
			errorText = errorText[:77] + "..."
		}
		fmt.Fprintf(writer, "%s\t%s\t%d\t%s\t%s\n", id, node.Status, node.Attempt, artifact, errorText)
	}
	return writer.Flush()
}

func recoverCommand(args []string) error {
	flags := flag.NewFlagSet("recover", flag.ContinueOnError)
	common := bindCommonFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	store, err := checkpoint.OpenStore(common.root)
	if err != nil {
		return err
	}
	ctx := commandContext()
	if err := store.Lock(ctx); err != nil {
		return err
	}
	defer func() { _ = store.Unlock() }()

	_, report, err := store.Recover(ctx, common.job)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
