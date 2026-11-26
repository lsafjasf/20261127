package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

const dhashFrameBytes = 9 * 8 // 9x8 gray pixels; 8 horizontal comparisons per row

// ExtractVideo decodes a video via ffmpeg, computes a 64-bit dHash per frame,
// reads PTS via ffprobe and writes framedoctor JSONL to out.
func ExtractVideo(ctx context.Context, video string, out io.Writer) (int, error) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0, fmt.Errorf("未找到 ffprobe，请先安装 ffmpeg 或直接使用帧轨迹文件: %w", err)
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return 0, fmt.Errorf("未找到 ffmpeg: %w", err)
	}

	pts, err := probePTS(ctx, ffprobe, video)
	if err != nil {
		return 0, err
	}

	cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-i", video,
		"-an", "-vf", "scale=9:8:flags=area,format=gray", "-f", "rawvideo", "pipe:1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("启动 ffmpeg 失败: %w", err)
	}
	raw, err := io.ReadAll(stdout)
	if err != nil {
		return 0, err
	}
	if err := cmd.Wait(); err != nil {
		return 0, fmt.Errorf("ffmpeg 解码失败: %w", err)
	}
	if len(raw)%dhashFrameBytes != 0 {
		return 0, fmt.Errorf("rawvideo 数据长度 %d 不是 %d 字节/帧的整数倍", len(raw), dhashFrameBytes)
	}
	n := len(raw) / dhashFrameBytes
	enc := json.NewEncoder(out)
	for i := 0; i < n; i++ {
		rec := map[string]any{"seq": i, "hash": fmt.Sprintf("%016x", dhashRaw(raw[i*dhashFrameBytes:]))}
		if i < len(pts) {
			rec["pts"] = pts[i]
		}
		if err := enc.Encode(rec); err != nil {
			return i, err
		}
	}
	return n, nil
}

func probePTS(ctx context.Context, ffprobe, video string) ([]float64, error) {
	cmd := exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "frame=pts_time,best_effort_timestamp_time",
		"-of", "csv=p=0", video)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe 读取 PTS 失败: %w", err)
	}
	var pts []float64
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		first := strings.Split(line, ",")[0]
		if first == "N/A" || first == "" {
			if parts := strings.Split(line, ","); len(parts) > 1 {
				first = parts[1]
			}
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(first), 64); err == nil {
			pts = append(pts, v)
		}
	}
	return pts, nil
}

// dhashRaw computes difference hash: bit=1 when left pixel is brighter than right.
func dhashRaw(b []byte) uint64 {
	var h uint64
	bit := 63
	for r := 0; r < 8; r++ {
		row := r * 9
		for c := 0; c < 8; c++ {
			if b[row+c] > b[row+c+1] {
				h |= 1 << uint(bit)
			}
			bit--
		}
	}
	return h
}
