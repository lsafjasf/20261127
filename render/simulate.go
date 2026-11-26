package render

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rendercheckpoint/checkpoint"
)

// SimulatedStage returns deterministic fake bytes for script/visual/encode.
// Replace it with a real renderer without changing checkpoint semantics.
func SimulatedStage(delay time.Duration) StageFunc {
	return func(ctx context.Context, rc *checkpoint.RunContext, chapter Chapter, stage Stage, input *checkpoint.Artifact) (*checkpoint.Artifact, error) {
		if err := waitDelay(ctx, delay); err != nil {
			return nil, err
		}

		var payload []byte
		var err error
		switch stage {
		case StageScript:
			payload, err = json.MarshalIndent(struct {
				ID     string   `json:"id"`
				Title  string   `json:"title"`
				Body   string   `json:"body"`
				Scenes []string `json:"scenes"`
			}{
				ID:     chapter.ID,
				Title:  chapter.Title,
				Body:   chapter.Body,
				Scenes: chapter.Scenes,
			}, "", "  ")
			if err != nil {
				return nil, err
			}

		case StageVisual:
			if input == nil {
				return nil, fmt.Errorf("visual stage for %s missing script", chapter.ID)
			}
			frames := 24 + len(chapter.Body) + len(chapter.Scenes)*12
			payload = []byte(fmt.Sprintf(
				"RENDER-CHECKPOINT-VISUAL\nchapter=%s\nsource=%s\nframes=%d\n",
				ChapterID(chapter, 0), input.Digest, frames,
			))

		case StageEncode:
			if input == nil {
				return nil, fmt.Errorf("encode stage for %s missing visual", chapter.ID)
			}
			payload = []byte(fmt.Sprintf(
				"RENDER-CHECKPOINT-MEDIA\nchapter=%s\ncodec=h264\nsource=%s\n",
				ChapterID(chapter, 0), input.Digest,
			))

		default:
			return nil, fmt.Errorf("unknown render stage %q", stage)
		}

		return rc.PutBytes(ctx, payload, mediaType(stage))
	}
}
