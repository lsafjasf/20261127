// Package render builds checkpointed multi-chapter rendering graphs on top
// of the generic checkpoint runner.
package render

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"rendercheckpoint/checkpoint"
)

type Stage string

const (
	StageScript Stage = "script"
	StageVisual Stage = "visual"
	StageEncode Stage = "encode"
)

// Chapter is one independently checkpointed rendering segment.
type Chapter struct {
	ID     string   `json:"id,omitempty"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Scenes []string `json:"scenes,omitempty"`
}

// StageFunc performs one rendering stage. Production code can replace the
// simulated implementation with real parser, renderer, or encoder calls.
type StageFunc func(ctx context.Context, rc *checkpoint.RunContext, chapter Chapter, stage Stage, input *checkpoint.Artifact) (*checkpoint.Artifact, error)

// Pipeline configures the chapter graph.
type Pipeline struct {
	JobID               string
	Chapters            []Chapter
	Resolution          string
	FPS                 int
	Format              string
	RendererVersion     string
	WorkDelay           time.Duration
	Stage               StageFunc
	PublishChapterFiles bool
	PublishManifest     bool
}

type ManifestChapter struct {
	Index          int                  `json:"index"`
	ID             string               `json:"id"`
	Title          string               `json:"title"`
	EncodeArtifact *checkpoint.Artifact `json:"encode_artifact"`
	OutputFile     string               `json:"output_file,omitempty"`
}

type Manifest struct {
	JobID    string            `json:"job_id"`
	Format   string            `json:"format"`
	Chapters []ManifestChapter `json:"chapters"`
}

var fileSafePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func ChapterID(chapter Chapter, index int) string {
	if chapter.ID != "" {
		return chapter.ID
	}
	return fmt.Sprintf("ch-%03d", index+1)
}

func fileBase(chapter Chapter, index int) string {
	id := ChapterID(chapter, index)
	base := fileSafePattern.ReplaceAllString(id, "-")
	if base == "" || base == "." || base == ".." {
		return fmt.Sprintf("ch-%03d", index+1)
	}
	return base
}

func waitDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func mediaType(stage Stage) string {
	switch stage {
	case StageScript:
		return "application/json"
	case StageVisual:
		return "application/x-render-visual"
	case StageEncode:
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

func stageNodeID(chapterID string, stage Stage) string {
	return chapterID + ":" + string(stage)
}

func (p Pipeline) withDefaults() Pipeline {
	if p.Resolution == "" {
		p.Resolution = "1920x1080"
	}
	if p.FPS == 0 {
		p.FPS = 30
	}
	if p.Format == "" {
		p.Format = "mp4"
	}
	if p.RendererVersion == "" {
		p.RendererVersion = "simulated-renderer-v1"
	}
	if p.Stage == nil {
		p.Stage = SimulatedStage(p.WorkDelay)
	}
	return p
}

// Tasks builds script -> visual -> encode for every chapter, plus one final
// manifest node that depends on every encoded chapter.
func (p Pipeline) Tasks() ([]checkpoint.Task, error) {
	if p.JobID == "" {
		return nil, fmt.Errorf("pipeline JobID is required")
	}
	if len(p.Chapters) == 0 {
		return nil, fmt.Errorf("pipeline requires at least one chapter")
	}
	p = p.withDefaults()
	tasks := make([]checkpoint.Task, 0, len(p.Chapters)*3+1)
	encodeIDs := make([]string, 0, len(p.Chapters))

	for index, chapter := range p.Chapters {
		chapter.ID = ChapterID(chapter, index)
		chapterID := chapter.ID
		scriptID := stageNodeID(chapterID, StageScript)
		visualID := stageNodeID(chapterID, StageVisual)
		encodeID := stageNodeID(chapterID, StageEncode)
		encodeIDs = append(encodeIDs, encodeID)

		tasks = append(tasks, checkpoint.Task{
			ID:      scriptID,
			Version: p.RendererVersion + "/script-v1",
			Inputs: map[string]any{
				"chapter":    chapter,
				"resolution": p.Resolution,
			},
			Run: p.runStage(chapter, index, StageScript, ""),
		})
		tasks = append(tasks, checkpoint.Task{
			ID:        visualID,
			DependsOn: []string{scriptID},
			Version:   p.RendererVersion + "/visual-v1",
			Inputs: map[string]any{
				"resolution": p.Resolution,
				"fps":        p.FPS,
			},
			Run: p.runStage(chapter, index, StageVisual, scriptID),
		})
		tasks = append(tasks, checkpoint.Task{
			ID:        encodeID,
			DependsOn: []string{visualID},
			Version:   p.RendererVersion + "/encode-v1",
			Inputs: map[string]any{
				"format":  p.Format,
				"chapter": chapterID,
			},
			Run: p.runEncode(chapter, index, visualID),
		})
	}

	tasks = append(tasks, checkpoint.Task{
		ID:        "final:manifest",
		DependsOn: encodeIDs,
		Version:   p.RendererVersion + "/manifest-v1",
		Inputs: map[string]any{
			"job_id":   p.JobID,
			"format":   p.Format,
			"chapters": p.Chapters,
		},
		Run: p.runManifest(encodeIDs),
	})
	return tasks, nil
}

func (p Pipeline) runStage(chapter Chapter, index int, stage Stage, dependencyID string) checkpoint.TaskFunc {
	return func(ctx context.Context, rc *checkpoint.RunContext) (*checkpoint.Artifact, error) {
		var input *checkpoint.Artifact
		if dependencyID != "" {
			artifact, err := rc.Dependency(dependencyID)
			if err != nil {
				return nil, err
			}
			input = artifact
		}
		return p.Stage(ctx, rc, chapter, stage, input)
	}
}

func (p Pipeline) runEncode(chapter Chapter, index int, dependencyID string) checkpoint.TaskFunc {
	return func(ctx context.Context, rc *checkpoint.RunContext) (*checkpoint.Artifact, error) {
		input, err := rc.Dependency(dependencyID)
		if err != nil {
			return nil, err
		}
		artifact, err := p.Stage(ctx, rc, chapter, StageEncode, input)
		if err != nil {
			return nil, err
		}
		if p.PublishChapterFiles {
			name := filepath.ToSlash(filepath.Join("chapters", fileBase(chapter, index)+"."+p.Format))
			if _, err := rc.Export(ctx, name, artifact); err != nil {
				return nil, err
			}
		}
		return artifact, nil
	}
}

func (p Pipeline) runManifest(encodeIDs []string) checkpoint.TaskFunc {
	return func(ctx context.Context, rc *checkpoint.RunContext) (*checkpoint.Artifact, error) {
		manifest := Manifest{JobID: p.JobID, Format: p.Format}
		for index, encodeID := range encodeIDs {
			artifact, err := rc.Dependency(encodeID)
			if err != nil {
				return nil, err
			}
			chapter := p.Chapters[index]
			entry := ManifestChapter{
				Index:          index + 1,
				ID:             ChapterID(chapter, index),
				Title:          chapter.Title,
				EncodeArtifact: artifact,
			}
			if p.PublishChapterFiles {
				entry.OutputFile = filepath.ToSlash(filepath.Join("chapters", fileBase(chapter, index)+"."+p.Format))
			}
			manifest.Chapters = append(manifest.Chapters, entry)
		}

		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode manifest: %w", err)
		}
		artifact, err := rc.PutBytes(ctx, data, "application/json")
		if err != nil {
			return nil, err
		}
		if p.PublishManifest {
			if _, err := rc.Export(ctx, p.JobID+"-manifest.json", artifact); err != nil {
				return nil, err
			}
		}
		return artifact, nil
	}
}
