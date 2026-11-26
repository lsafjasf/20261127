package render

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"rendercheckpoint/checkpoint"
)

func sampleChapters() []Chapter {
	return []Chapter{
		{Title: "序章", Body: "开始", Scenes: []string{"封面", "城市"}},
		{ID: "chapter-02", Title: "成长", Body: "冒险", Scenes: []string{"森林", "河流"}},
		{Title: "终章", Body: "归来", Scenes: []string{"家门"}},
	}
}

func runPipeline(t *testing.T, store *checkpoint.Store, pipeline Pipeline) (*checkpoint.Report, error) {
	t.Helper()
	tasks, err := pipeline.Tasks()
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	runner := &checkpoint.Runner{Store: store, JobID: pipeline.JobID, Tasks: tasks, Concurrency: 2}
	return runner.Run(context.Background())
}

func TestPipelineResumeAndChapterLocalInvalidation(t *testing.T) {
	store, err := checkpoint.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Unlock() })

	var stageCalls [9]atomic.Int64
	countingStage := func(ctx context.Context, rc *checkpoint.RunContext, chapter Chapter, stage Stage, input *checkpoint.Artifact) (*checkpoint.Artifact, error) {
		index := 0
		switch chapter.ID {
		case "ch-001":
			index = 1
		case "chapter-02":
			index = 2
		case "ch-003":
			index = 3
		}
		stageIndex := map[Stage]int{StageScript: 0, StageVisual: 1, StageEncode: 2}[stage]
		stageCalls[(index-1)*3+stageIndex].Add(1)
		return SimulatedStage(0)(ctx, rc, chapter, stage, input)
	}

	pipeline := Pipeline{
		JobID:               "demo-book",
		Chapters:            sampleChapters(),
		Stage:               countingStage,
		PublishChapterFiles: true,
		PublishManifest:     true,
	}
	if _, err := runPipeline(t, store, pipeline); err != nil {
		t.Fatalf("first pipeline: %v", err)
	}
	for i := range stageCalls {
		if got := stageCalls[i].Load(); got != 1 {
			t.Fatalf("stage %d first-run count = %d, want 1", i, got)
		}
	}

	resumeReport, err := runPipeline(t, store, pipeline)
	if err != nil {
		t.Fatalf("resume pipeline: %v", err)
	}
	if len(resumeReport.Ran) != 0 || len(resumeReport.Reused) != 10 {
		t.Fatalf("resume ran=%v reused=%v, want all 10 reused", resumeReport.Ran, resumeReport.Reused)
	}
	for i := range stageCalls {
		if got := stageCalls[i].Load(); got != 1 {
			t.Fatalf("stage %d redo count = %d after unchanged resume", i, got)
		}
	}

	changed := sampleChapters()
	changed[1].Body = "修改后的冒险内容"
	pipeline.Chapters = changed
	changeReport, err := runPipeline(t, store, pipeline)
	if err != nil {
		t.Fatalf("changed pipeline: %v", err)
	}
	for i := 3; i < 6; i++ {
		if got := stageCalls[i].Load(); got != 2 {
			t.Errorf("chapter-02 stage %d count = %d, want rerun", i, got)
		}
	}
	for _, i := range []int{0, 1, 2, 6, 7, 8} {
		if got := stageCalls[i].Load(); got != 1 {
			t.Errorf("unrelated stage %d count = %d, want 1", i, got)
		}
	}
	if !contains(changeReport.Ran, "final:manifest") {
		t.Errorf("final manifest did not rerun: %+v", changeReport)
	}
	if !contains(changeReport.Reused, "ch-001:encode") || !contains(changeReport.Reused, "ch-003:encode") {
		t.Errorf("unrelated chapter encodes not reused: %+v", changeReport)
	}

	manifestPath := filepath.Join(store.OutputDir(), "demo-book-manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("published manifest missing: %v", err)
	}
	chapterPath := filepath.Join(store.OutputDir(), "chapters", "chapter-02.mp4")
	if _, err := os.Stat(chapterPath); err != nil {
		t.Fatalf("published chapter missing: %v", err)
	}
}

func contains(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
