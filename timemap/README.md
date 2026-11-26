# timemap — 编辑/媒体/显示三时钟映射与一帧错位定位

全部逻辑为纯 Go（标准库），参考管线使用 `math/big.Rat` 精确有理运算，不经过 float64。

## 三个时间域

| 时间域 | 含义 | 换算 |
|---|---|---|
| 编辑时间 edit | 编辑器绝对时间轴，所有轨道共用 | tick / 1,000,000 秒 |
| 媒体时间 media | 视频帧号 / 音频采样号 / 字幕帧号 | `index = floor(edit_s × media_rate)` |
| 显示时间 display | 输出端 vsync 调度槽（60 Hz） | `slot = floor(media_s × 60)` |

## 双管线

- 参考管线（`ModeReference`）：精确 `big.Rat` + floor，作为标准答案。
- 实际管线（`ModeAsBuilt`）：float64 乘法 + 每轨道配置的缺陷（错误速率或 `math.Round`）。

定位器逐阶段比较两条管线，在**首个出现分歧的环节**归因，并用精确假设复核：
把真实速率分别与 floor/nearest 组合重算全部点，能完整复现实际索引的假设即根因。

## 运行

```bash
go test ./...
go run ./timemap/cmd/timemap            # 输出到 artifacts/timemap/
go run ./timemap/cmd/timemap -out /tmp/x
```

产物：

- `artifacts/timemap/timemap.csv`：逐点可复核宽表，含精确分数索引、float64 值、整数索引、逐级 delta。
- `artifacts/timemap/findings.md`：中文诊断报告与复核方法。

## 手算示例（T1 = 41708 µs）

视频真实速率 24000/1001 fps：

- 精确缩放：`41708/1e6 × 24000/1001 = 125124/125125 = 0.999992`
- floor（正确）→ 帧号 0；nearest（实际实现）→ 帧号 1，正好差一帧。

音频 48 kHz：

- 缩放：`41708/1e6 × 48000 = 2001.984`
- floor → 2001；nearest → 2002，差一个采样。

字幕（实际管线误用 24 fps）在 60 s 处：

- 正确：`floor(60 × 24000/1001) = 1438`；错误：`floor(60 × 24) = 1440`，漂移累积为 2 帧。

## 修复准则

1. 媒体环节一律使用真实有理速率（保留 1001 时间基），禁止把 24000/1001 折成 24。
2. 时间到整数单元的量化一律 floor（槽 `n` 拥有区间 `[n/rate, (n+1)/rate)`）。
3. 帧号、采样号、显示槽均 0 基。
满足三条后重跑，CSV 中所有 delta 列为 0（由 `TestCleanPipelineHasZeroDeltas` 固化）。
