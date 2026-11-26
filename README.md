# tearcheck

零第三方 Go 依赖的横向撕裂归因检测器。它把采集时间、帧内断层、相邻帧重复和编码块边界合并成可解释证据，区分：

- `hardware_sync`：采集/帧缓冲/扫描输出不同步；
- `encoding_error`：码流错误、丢包、解码隐藏或宏块/编码块损伤；
- `content_effect`：游戏或后期故意加入的撕裂、色差、Glitch 特效；
- `manual_review`：证据不足或多个假设冲突，保留原始证据和候选原因。

## 快速使用

```bash
go test ./...
go build -o tearcheck ./cmd/tearcheck

# 分析预计算证据/外部系统特征
./tearcheck signals --input examples/request.json --format text

# 直接读取视频；解封装/解码委托给 PATH 中的 ffprobe/ffmpeg
./tearcheck video camera.mp4 \
  --metadata capture-metadata.json \
  --frames 600 \
  --format json > report.json
```

也可以作为库使用：

```go
report := tearcheck.AnalyzeLumaFrames(frames, tearcheck.Metadata{
    Source: "camera.mp4",
    Codec:  "h264",
})
for _, item := range report.ManualReviews {
    // 进入人工复核队列，item.Manual.PossibleCauses 保留候选原因。
    _ = item
}
```

## 判定信号

| 维度 | 实现 | 更支持 |
| --- | --- | --- |
| 采集时间 | 采集间隔、PTS 间隔、采集时钟/PTS 漂移、掉帧间隔 | 硬件/采集不稳定；单独异常只生成人工项 |
| 帧内断层 | 行边界 MAD、强边缘列覆盖率、局部峰值抑制、任意扫描线位置 | 真实横向断层 |
| 跨帧重复 | 断层上/下条带与前帧的 SAD、水平位移搜索、两侧匹配对比、编码块跨帧重复 | 单侧精确重复旧帧偏向硬件；对齐块持续重复偏向编码损伤 |
| 编码块边界 | 4/8/16/32/64 网格对齐、水平/垂直块边界能量、H.264 MB 解码日志坐标 | 编码错误 |
| 内容特效 | 可信特效窗口/游戏标记、高饱和色差边缘、多带一致性、缺少帧缓冲拼接特征 | 游戏内容特效 |

## 低置信度策略

分类器输出每个原因的 `0..1` 分数、分差和逐条权重证据。满足以下任一条件时不会强行归因：

- 最高分低于 `0.68`；
- 前两名分差低于 `0.10`；
- 两个强假设冲突。

低置信度结果的 `cause` 为 `manual_review`，`confidence` 为 `low`，并同时出现在 `findings` 与 `manual_reviews`。`manual.observation` 保存完整归一化证据，`manual.possible_causes` 保存候选方向。

强证据可以直接定性，例如：

- 单侧条带精确匹配相邻帧、另一侧明显不同，且断层不在编码网格上；
- 解码器报告 MB 错误，且断层对齐 16/64 像素块边界；
- 游戏日志或特效遮罩以高置信度标记特效时间窗，且没有旧帧缓冲拼接证据。

## 元数据格式

视频文件通常不包含可靠的“摄像头采集时刻”。可通过 `--metadata` 传入侧车 JSON：

```json
{
  "frame_timing": [
    {"frame_index": 42, "pts_seconds": 1.4, "capture_seconds": 1764200001.401, "expected_interval": 0.0333}
  ],
  "content_effect_windows": [
    {"start_seconds": 10.2, "end_seconds": 11.0, "confidence": 0.95, "label": "scripted tear effect"}
  ],
  "codec_events": [
    {"frame_index": 88, "y": 512, "confidence": 1.0, "message": "error while decoding MB 32 32"}
  ]
}
```

`signals` 命令还接受 `observations` 数组，可接入已有的 CV、播放器日志、码流分析或人工标注系统。字段定义见 `model.go`，完整示例见 `examples/request.json`。

## 设计边界

纯像素分析不能证明“意图”，也不能在没有码流/解码器日志时绝对区分罕见的网格对齐硬件撕裂与编码损伤。因此本项目采用保守策略：证据闭合才自动分类，冲突场景保留人工项。视频命令只把 ffmpeg 用作解码器，检测、评分和报告均在 Go 包内完成。
