# camsync

纯 Go 标准库实现的四机位无时码分段对齐器。它把拍手、闪光、持续环境声以及
上游检测器给出的候选观测，解算成“各设备本地时间 -> 母时间线”的分段线性映射，
并明确输出无法可靠判定的区段。

## 快速开始

```bash
go test ./...
go run ./cmd/camsync -in examples/four-camera.json -format text
go run ./cmd/camsync -in examples/four-camera.json -out alignment.json
```

输入默认从 stdin 读取；`-format` 支持 `json`（默认）和 `text`。

## 输入模型

输入是一份证据清单，所有时间单位均为秒：

```json
{
  "master_device": "audio",
  "events": [
    {"id": "clap-01", "kind": "clap"},
    {"id": "flash-01", "kind": "flash"},
    {"id": "amb-01", "kind": "ambient"}
  ],
  "devices": [
    {
      "id": "cam-a",
      "segments": [
        {
          "id": "cam-a-1",
          "start": 0,
          "end": 1800,
          "observations": [
            {"event": "clap-01", "local": 12.413, "confidence": 0.97},
            {"event": "flash-01", "local": 241.881, "confidence": 0.95}
          ]
        }
      ]
    }
  ]
}
```

- `master_device`：母时间线的参考设备，通常是独立现场录音。其本地时间就是母时间。
- `segments`：一次不间断录制。相机重启后必须新建分段，新分段重新拟合自己的偏移和漂移。
- `observations[].event`：跨设备共享的物理事件 ID；同名 ID 才可能建立对应关系。
- `confidence`：上游检测/匹配置信度，省略时按 `1.0` 处理。
- `events[].master_time`：可选。已有绝对时间码或人工确认时可直接给出；否则从母设备观测播种。
- `kind`：`clap`、`flash`、`ambient`、`marker`。拍手和环境声通常由母录音直接定出时间；
  闪光可先由已锁定机位推算，再传播给其他机位。

信号提取层应由画面亮度检测器、音频起音检测器、音频指纹/包络相关器输出上述观测。
对齐核心不依赖第三方 Go 包，也不内置视频编解码器。

## 解算方法

1. 母设备观测直接建立事件母时间；缺失事件通过已锁定分段的闪光/环境声预测传播。
2. 每个相机分段只使用本分段观测，独立做 RANSAC 内点筛选和置信度加权最小二乘：

   ```text
   master = slope * local + intercept
   drift_ppm = (slope - 1) * 1_000_000
   ```

3. 默认至少两个一致锚点才 `locked`，至少三个锚点且跨度不少于 60 秒才确认漂移；
   单锚点分段是 `provisional`，没有一致解是 `unresolved`。
4. 首个可信锚点是稳健内点中本地时间最早的一个。`offset_at_local_start_sec` 给出分段起点
   外推后的偏移，`drift_ppm` 给出后续时钟斜率。
5. 映射覆盖不是无限外推：每个锚点按误差预算形成覆盖半径，超预算或锚点间距过大的部分
   写入 `indeterminate`。

## 输出

主要字段：

- `status`：`master`、`locked`、`provisional` 或 `unresolved`。
- `first_trusted_anchor`：首个可信锚点的事件、本地时间、母时间、偏移和残差。
- `offset_at_local_start_sec`：按拟合线回推到分段起点的偏移。
- `drift_ppm` / `drift_confirmed`：估计漂移及是否达到确认条件。
- `fit`：斜率、截距、内点数、被拒锚点数、基线跨度与残差。
- `map[]`：半开区间 `[local_start, local_end)` 的线性映射及最大误差预算。
- `indeterminate[]`：不可判定半开区间，原因包括无可用锚点、锚点冲突、首锚点之前、
  末锚点之后、锚点间距过大或误差超预算。

`map` 与 `indeterminate` 对同一分段互补，不会同时覆盖同一时间。

## 配置

可在输入顶层增加 `config`，只写需要覆盖的项：

| 字段 | 默认值 | 含义 |
| --- | ---: | --- |
| `min_anchor_confidence` | 0.55 | 最低有效锚点置信度 |
| `min_offset_anchors` | 2 | 确认偏移所需一致锚点数 |
| `min_drift_anchors` | 3 | 确认漂移所需一致锚点数 |
| `min_drift_baseline_sec` | 60 | 确认漂移所需最短锚点跨度 |
| `max_drift_ppm` | 1500 | 物理上合理的最大相对漂移 |
| `anchor_tolerance_ms` | 25 | RANSAC 内点残差阈值 |
| `base_uncertainty_ms` | 8 | 检测/对应关系的基础误差 |
| `max_unverified_drift_ppm` | 500 | 未确认漂移时的保守误差斜率 |
| `confirmed_drift_uncertainty_ppm` | 40 | 已确认漂移后的残余误差斜率 |
| `max_anchor_gap_sec` | 90 | 相邻锚点最大可信间隔 |
| `max_extrapolation_sec` | 60 | 单锚点覆盖半径上限 |
| `max_mapping_error_ms` | 20 | 映射误差预算，默认接近 25 fps 半帧 |
| `propagation_factor` | 0.85 | 跨机位传播时的置信度折减 |
| `ambient_confidence_factor` | 0.9 | 环境声匹配的置信度折减 |

## 示例结果

`examples/four-camera.json` 包含四台机位和一条母录音。`cam-b` 中途重启，`cam-d`
只有闪光可观测。示例输出摘要：

```text
DEVICE cam-b
  SEGMENT cam-b-1 ... status=locked
    first trusted: clap-01 (clap) local=9.599 master=10.000 offset=+0.401 s
    drift: +100.0 ppm (confirmed=true)
  SEGMENT cam-b-2 ... status=locked
    first trusted: clap-03 (clap) local=19.554 master=200.000 offset=+180.446 s
    drift: -200.0 ppm (confirmed=true)

DEVICE cam-d
  SEGMENT cam-d-1 ... status=locked
    first trusted: flash-01 (flash) local=54.373 master=55.000 offset=+0.627 s
    drift: +500.0 ppm (confirmed=true)
    undetermined:
      local [314.273, 340.000): after_last_anchor_coverage
```

## 边界说明

- 没有外部绝对时钟时，只能测相对漂移；母录音声卡自身的慢漂会并入母时间线定义。
  若需要真实时间，请把母设备校准到可信时钟，或选择稳定时钟作为 `master_device`。
- 时钟模型是分段线性。明显非线性温漂应在更短的 `segments` 内重拟合。
- 事件 ID 必须代表同一物理刺激；错误合并的 ID 会被稳健拟合尽量拒绝，但无法凭空恢复。
- 单锚点只能给出暂定偏移，不能确认后续漂移；相关区段会保留为不可判定。
