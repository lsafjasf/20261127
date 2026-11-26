# hdrframe — HDR10+ 帧级动态元数据核对器

只使用 Go 标准库。它把**源端应有元数据**、**转码/转封装后仍存在的元数据时间线**和
**播放器实际应用证据**放到同一帧时间轴上，回答三个问题：

1. 每个目标帧对应哪一版 HDR10+ 参数；
2. 场景号、帧/序列号和生效时间是否一致；
3. 播放器最终实际应用了哪一版，是否导致高光亮度/色相停留在旧参数。

仓库没有提供实际成片或播放器日志，因此下面的结论来自工具构造的重复、缺帧和跳号样本。
接入真实导出的 HDR10+ JSON 与播放器 `HDR10+`/T.35 调试日志后，可用同一程序复核。

## 快速运行

```bash
go test -count=1 ./...

# 生成 examples/*.json，并输出 artifacts/hdrframe-report.{json,txt}
go run ./cmd/hdrframe demo

# 或分步执行
go run ./cmd/hdrframe sample -dir examples
go run ./cmd/hdrframe audit \
  -input examples/samples.json \
  -json artifacts/report.json \
  -text artifacts/report.txt

# CI 门禁请使用编译后的二进制；直接 go run 可能把退出码 3 包装为 1
go build -o bin/hdrframe ./cmd/hdrframe
./bin/hdrframe audit -input examples/samples.json -strict
```

`-strict` 在发现实际应用版本与应有参数不一致时返回退出码 `3`，可接入转码验收流水线。

## 构造样本与结论

三个样本都属于同一场景：`scene_id=42`。

| 样本 | 目标帧 | 生效时间 | 异常 | 源端应有 | 转码时间线所选 | 播放器实际应用 | 结论 |
| --- | ---: | ---: | --- | --- | --- | --- | --- |
| 重复冲突 | 122 | `5.083333s` / `00:00:05:02` @24fps | 同序列 122 有 A/B 两版冲突参数 | `HM-122-B-right` | `HM-122-B-right`（按 bitstream/ingest order 选最新） | `HM-122-A-old`，播放器日志证实 | **用错重复旧版**：800 nits/gain 1.1/hue +9，而非 2200 nits/gain 2.4/hue -8 |
| 缺帧 | 202 | `8.425083s` / `00:00:08:10` @`24000/1001` | 转码后没有帧 202 的动态元数据 | `HM-202-required` | `SRC-201`（hold-last） | `SRC-201`，播放器日志证实 | **停留在上一版**：980 nits/gain 1.3/hue +2，而非 2400 nits/gain 2.7/hue -10 |
| 跳号 | 302 | `12.080000s` / `00:00:12:02` @25fps | 序列号 `301 -> 305`，缺 `302,303,304` | `JMP-302-frame-right` | `JMP-302-frame-right` | 同版本，实际序列号 `305`，日志证实 | **跳号本身不是错版**：帧号/PTS/参数摘要正确，应报告跳号但允许生效 |

机器可读结果在 `artifacts/hdrframe-report.json`，文本结果在
`artifacts/hdrframe-report.txt`。每个 `frames[]` 项目都包含：

- `expected`：源端/作者期望的参数版本；
- `selected`：根据转码后记录、生效时间和策略选出的版本；
- `applied`：播放器日志证实的版本，或无日志时按策略推断的版本；
- `applied.verified`：`true` 表示有播放器日志/摘要直接证明，`false` 只是 hold-last 等推断；
- `issues[]`：`duplicate_conflict_resolved`、`missing_exact_frame_metadata`、
  `metadata_delivery_drift`、`sequence_jump`、`scene_id_mismatch` 等。

## 为什么不能只看场景号

同一场景内 HDR10+ 的统计和 tone mapping 参数可以逐帧变化；`scene_id` 只用于标识场景，
不能作为参数版本。选择顺序是：

1. 用目标帧 PTS/时间码和 fps 换算目标时刻；
2. 只允许 `effective_seconds <= target_seconds + tolerance` 的记录生效；
3. 同一生效时刻按 sequence、bitstream/ingest order 选择；
4. 重复序列比较参数指纹，相同则去重，冲突则按 `policy.duplicate` 处理；
5. 缺精确帧记录时按 `policy.missing_metadata` 推断：默认 `hold_last`；
6. 跳号单独报告；若帧号/PTS 和参数摘要匹配，不因序列号跳变而否定该帧；
7. 若有播放器事件，以事件中的版本/摘要作为实际应用证据。

没有播放器日志或输出参数指纹时，程序只能给出“按常见播放器策略推断的版本”，不会把它
冒充成已证实事实。

## 输入格式

规范 JSON 示例见 `examples/duplicate.json`、`examples/missing-frame.json` 和
`examples/sequence-jump.json`。核心结构：

```json
{
  "fps": "24000/1001",
  "policy": {"duplicate": "latest", "missing_metadata": "hold_last", "sequence_gap": "accept"},
  "targets": [{"frame": 202, "expected_scene_id": 42}],
  "expected_metadata": [{"version": "HM-202-required", "frame": 202, "scene_id": 42}],
  "metadata": [{"version": "SRC-201", "frame": 201, "scene_id": 42}],
  "player_events": [{"target_frame": 202, "applied_version": "SRC-201", "source": "player_log"}]
}
```

也兼容常见 HDR10+ 工具导出的 PascalCase 字段（`Metadata`、`SceneInfo`、
`SequenceFrameIndex`、`SceneId`、`SceneFrameIndex`）：

```bash
go run ./cmd/hdrframe audit -input hdr10plus_metadata.json -format hdr10plus -fps 24000/1001
```

真实排查时，还应保留播放器调试 OSD/log、T.35 SEI 导出和目标帧 PTS；若只能看到转码后画面，
可对高光区域亮度和色相做测量，作为 `player_events[].evidence` 的旁证。
