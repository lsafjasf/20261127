package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rendercheckpoint/checkpoint"
	"rendercheckpoint/render"
)

type runOptions struct {
	root        string
	job         string
	chapters    int
	delay       time.Duration
	concurrency int
	cancelAfter time.Duration
	noPublish   bool
}

func runCommand(args []string) error {
	options := runOptions{}
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.StringVar(&options.root, "root", ".render-workspace", "checkpoint workspace root")
	flags.StringVar(&options.job, "job", "demo-book", "job id")
	flags.IntVar(&options.chapters, "chapters", 4, "number of demo chapters")
	flags.DurationVar(&options.delay, "delay", 25*time.Millisecond, "simulated stage delay")
	flags.IntVar(&options.concurrency, "concurrency", 2, "node concurrency")
	flags.DurationVar(&options.cancelAfter, "cancel-after", 0, "automatically cancel after duration")
	flags.BoolVar(&options.noPublish, "no-publish", false, "do not publish outputs")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if options.chapters < 1 {
		return errors.New("--chapters must be at least 1")
	}

	store, err := checkpoint.OpenStore(options.root)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if options.cancelAfter > 0 {
		cancelCtx, cancel := context.WithCancel(ctx)
		ctx = cancelCtx
		time.AfterFunc(options.cancelAfter, cancel)
		defer cancel()
	}

	pipeline := render.Pipeline{
		JobID:               options.job,
		Chapters:            demoChapters(options.chapters),
		WorkDelay:           options.delay,
		PublishChapterFiles: !options.noPublish,
		PublishManifest:     !options.noPublish,
	}
	tasks, err := pipeline.Tasks()
	if err != nil {
		return err
	}

	runner := &checkpoint.Runner{
		Store:       store,
		JobID:       options.job,
		Tasks:       tasks,
		Concurrency: options.concurrency,
		OnEvent:     printEvent,
	}

	fmt.Printf("渲染任务 %s 启动：%d 章，工作区 %s\n", options.job, options.chapters, options.root)
	report, runErr := runner.Run(ctx)
	printRecovery(report)
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			fmt.Fprintln(os.Stderr, "\n任务已取消：已完成分段已保留，重新执行相同命令即可续跑；未完成文件已隔离。")
			return silentExit{code: 130, err: runErr}
		}
		return runErr
	}

	fmt.Printf("\n完成：新执行 %d 段，检查点复用 %d 段。\n", len(report.Ran), len(report.Reused))
	if !options.noPublish {
		fmt.Printf("清单：%s\n", store.OutputDir()+"/"+options.job+"-manifest.json")
		fmt.Printf("章节目录：%s/chapters\n", store.OutputDir())
	}
	return nil
}

type silentExit struct {
	code int
	err  error
}

func (e silentExit) Error() string { return e.err.Error() }

func demoChapters(count int) []render.Chapter {
	titles := []string{"起源", "迷雾", "相遇", "试炼", "深渊", "破晓", "归途"}
	chapters := make([]render.Chapter, 0, count)
	for i := 0; i < count; i++ {
		title := titles[i%len(titles)]
		chapters = append(chapters, render.Chapter{
			ID:     fmt.Sprintf("ch-%03d", i+1),
			Title:  fmt.Sprintf("第 %d 章 · %s", i+1, title),
			Body:   "由 renderctl 生成的可恢复演示章节",
			Scenes: []string{"开场", "冲突", "收束"},
		})
	}
	return chapters
}

func printEvent(event checkpoint.Event) {
	switch event.Type {
	case checkpoint.EventRecovery:
		if event.Recovery != nil && (len(event.Recovery.QuarantinePaths) > 0 || len(event.Recovery.InterruptedNodes) > 0) {
			fmt.Printf("[recover] 隔离 %d 个残留文件，重置中断节点 %v\n", len(event.Recovery.QuarantinePaths), event.Recovery.InterruptedNodes)
		}
	case checkpoint.EventReused:
		fmt.Printf("[reuse]   %s\n", event.TaskID)
	case checkpoint.EventStarted:
		fmt.Printf("[render]  %s (attempt %d)\n", event.TaskID, event.Attempt)
	case checkpoint.EventCompleted:
		fmt.Printf("[done]    %s %s\n", event.TaskID, shortDigest(event.Artifact))
	case checkpoint.EventFailed:
		fmt.Fprintf(os.Stderr, "[failed]  %s: %v\n", event.TaskID, event.Err)
	case checkpoint.EventCanceled:
		fmt.Printf("[cancel]  %s\n", event.TaskID)
	case checkpoint.EventSkipped:
		fmt.Printf("[skip]    %s\n", event.TaskID)
	}
}

func printRecovery(report *checkpoint.Report) {
	if report == nil || report.Recovery == nil {
		return
	}
	recovery := report.Recovery
	if len(recovery.QuarantinePaths) > 0 {
		fmt.Printf("恢复：已隔离 %d 个不完整/损坏输出到 quarantine/\n", len(recovery.QuarantinePaths))
	}
}

func shortDigest(artifact *checkpoint.Artifact) string {
	if artifact == nil {
		return ""
	}
	digest := artifact.Digest
	if len(digest) > 19 {
		return digest[:19] + "…"
	}
	return digest
}
