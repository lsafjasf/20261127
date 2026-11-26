package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
)

// Planted describes one synthetic fault with ground-truth location.
type Planted struct {
	Kind   string  `json:"kind"`
	AtS    float64 `json:"at_s"`
	Frames []int   `json:"source_frames"`
	Note   string  `json:"note,omitempty"`
}

// GroundTruth is written next to generated traces as scenario.json.
type GroundTruth struct {
	FPS     float64   `json:"fps"`
	WindowS float64   `json:"window_s"`
	Dir     string    `json:"dir"`
	First   Planted   `json:"first_anomaly"`
	Planted []Planted `json:"planted"`
}

type genRow struct {
	seq  int
	src  int
	pts  float64
	hash uint64
}

// GenerateScenario writes mutually consistent capture/record/player traces:
// capture drop at 3s, encode duplication at 6s, player skip at 9s.
func GenerateScenario(dir string, fps float64, seconds int) (*GroundTruth, error) {
	if fps <= 0 {
		fps = 30
	}
	if seconds < 4 {
		seconds = 12
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	n := int(fps * float64(seconds))
	rng := rand.New(rand.NewSource(20261127))

	hashes := make([]uint64, n)
	hashes[0] = rng.Uint64()
	for i := 1; i < n; i++ {
		hashes[i] = flipBits(hashes[i-1], 3, rng)
	}

	drop0 := int(3 * fps)
	drop := map[int]bool{}
	dropFrames := []int{}
	for k := 0; k < 3; k++ {
		drop[drop0+k] = true
		dropFrames = append(dropFrames, drop0+k)
	}
	dup0 := int(6 * fps)
	skip0 := int(9 * fps)
	skip := map[int]bool{}
	skipFrames := []int{}
	for k := 0; k < 4; k++ {
		skip[skip0+k] = true
		skipFrames = append(skipFrames, skip0+k)
	}

	recRows := buildRecordRows(fps, n, hashes, drop, dup0)
	if err := writeCaptureCSV(filepath.Join(dir, "capture.csv"), fps, n, hashes, drop); err != nil {
		return nil, err
	}
	if err := writeRecordJSONL(filepath.Join(dir, "record.jsonl"), recRows); err != nil {
		return nil, err
	}
	if err := writePlayerCSV(filepath.Join(dir, "player.csv"), recRows, skip); err != nil {
		return nil, err
	}

	gt := &GroundTruth{
		FPS: fps, WindowS: 1, Dir: dir,
		First: Planted{Kind: KindCaptureDrop, AtS: 3.0, Frames: dropFrames,
			Note: "采集端缺失3帧，录制端帧号连续、PTS单调但时间窗少3帧，并有跳变"},
		Planted: []Planted{
			{Kind: KindCaptureDrop, AtS: 3.0, Frames: dropFrames},
			{Kind: KindEncodeDuplicate, AtS: float64(dup0+1) / fps, Frames: []int{dup0 + 1, dup0 + 2},
				Note: "第180帧被额外编码2次，帧号/PTS照常递增，画面零推进"},
			{Kind: KindPlayerSkip, AtS: float64(skip0) / fps, Frames: skipFrames,
				Note: "录制文件完好，播放器未呈现对应4个录制帧"},
		},
	}
	if b, err := json.MarshalIndent(gt, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "scenario.json"), b, 0o644)
	}
	return gt, nil
}

func flipBits(v uint64, count int, rng *rand.Rand) uint64 {
	for _, p := range rng.Perm(64)[:count] {
		v ^= 1 << uint(p)
	}
	return v
}

func buildRecordRows(fps float64, n int, hashes []uint64, drop map[int]bool, dup0 int) []genRow {
	var rows []genRow
	seq := 0
	for i := 0; i < n; i++ {
		if drop[i] {
			continue
		}
		h := hashes[i]
		if i == dup0+1 || i == dup0+2 {
			h = hashes[dup0]
		}
		rows = append(rows, genRow{seq: seq, src: i, pts: round6(float64(i) / fps), hash: h})
		seq++
	}
	return rows
}

const synthWallBase = 1764000000.0 // unix seconds

func writeCaptureCSV(path string, fps float64, n int, hashes []uint64, drop map[int]bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"seq", "wall_s", "hash", "dropped"})
	for i := 0; i < n; i++ {
		wall := fmt.Sprintf("%.6f", synthWallBase+float64(i)/fps)
		if drop[i] {
			_ = w.Write([]string{fmt.Sprint(i), wall, "", "true"})
			continue
		}
		_ = w.Write([]string{fmt.Sprint(i), wall, fmt.Sprintf("%016x", hashes[i]), "false"})
	}
	w.Flush()
	return w.Error()
}

func writeRecordJSONL(path string, rows []genRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range rows {
		rec := map[string]any{
			"seq":  r.seq,
			"pts":  r.pts,
			"hash": fmt.Sprintf("%016x", r.hash),
		}
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}
	return nil
}

// writePlayerCSV derives presented frames from the recording itself,
// dropping the record rows whose source index is in skip.
func writePlayerCSV(path string, recRows []genRow, skip map[int]bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"seq", "pts", "wall_s", "hash"})
	seq := 0
	for _, r := range recRows {
		if skip[r.src] {
			continue
		}
		wall := fmt.Sprintf("%.6f", synthWallBase+200+r.pts)
		_ = w.Write([]string{
			fmt.Sprint(seq), fmt.Sprintf("%.6f", r.pts), wall,
			fmt.Sprintf("%016x", r.hash),
		})
		seq++
	}
	w.Flush()
	return w.Error()
}

func round6(v float64) float64 {
	return float64(int64(v*1e6+0.5)) / 1e6
}
