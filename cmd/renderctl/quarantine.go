package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

func commandContext() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx
}

func quarantineCommand(args []string) error {
	flags := flag.NewFlagSet("quarantine", flag.ContinueOnError)
	root := flags.String("root", ".render-workspace", "checkpoint workspace root")
	if err := flags.Parse(args); err != nil {
		return err
	}

	dir := filepath.Join(*root, "quarantine")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	fmt.Printf("Quarantine: %s\n", dir)
	count := 0
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Printf("%s\t%8d bytes\t%s\n", info.ModTime().Format(time.RFC3339), info.Size(), entry.Name())
		count++
	}
	if count == 0 {
		fmt.Println("(empty)")
	}
	return nil
}
