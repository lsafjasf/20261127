# 多设备拼接音画同步分析

这个 Go 工具接收拼接后统一时间轴上的音频脉冲时间和画面事件时间，按切点
分段对齐两组事件，并**逐段**估计音画偏移和时钟漂移。它不会把所有片段
压成一个全局常数；输出中的 `global_after` 只是用于证明单一全局模型不足。

模型（每一段独立）：

```text
e(t) = visual(t) - pulse(t)
     = offset_s + drift_s * (t - segment_start_s)

offset_s : 段起点处的毫秒偏移，正值表示画面事件晚于音频脉冲
drift_s  : 无量纲时钟增益误差，输出同时给出 ppm 和 ms/s
```

## 运行

```bash
go run ./cmd/avsync examples/splice.json
go run ./cmd/avsync -json examples/splice.json
```

也可以从 stdin 读取：

```bash
cat examples/splice.json | go run ./cmd/avsync -json
```

构建和测试：

```bash
go test ./...
go build ./cmd/avsync
```

## 输入格式

时间戳应位于视频拼接后的输出时间轴。`segments` 使用半开区间
`[start, end)`；段与段可以相邻，也可以有空洞，不能重叠。

```json
{
  "time_unit": "ms",
  "segments": [
    {"id": "A", "source": "cam-a", "start": 0, "end": 6000},
    {"id": "B", "source": "cam-b", "start": 6000, "end": 12000}
  ],
  "pulses": [500, 1500, 2500],
  "events": [503.03, 1503.09, 2503.15],
  "options": {
    "match_window_ms": 250,
    "offset_tolerance_ms": 10,
    "drift_tolerance_ppm": 1000,
    "min_pairs": 3,
    "min_drift_span_ms": 1000
  }
}
```

- `time_unit` 支持 `s`、`ms`、`us`，省略时为 `s`。输出固定为毫秒。
- `pulses` 可写成 `audio_pulses`，`events` 可写成 `video_events`。
- 不提供 `segments` 时，可用 `"cuts": [6.0, 12.0]` 按切点自动分段。
- `source` 省略时使用 `id`；`id` 省略时自动生成 `segment-N`。
- `options` 可省略，默认值就是上面列出的值。

## 方法和异常判定

1. 按各段时间边界筛选脉冲和画面事件。
2. 在每段内用单调动态规划配对，允许漏检或误检造成的未配对事件。
3. 对配对后的误差做 Theil-Sen 初拟合，再用 Huber 迭代加权最小二乘
   估计该段自己的 `offset` 和 `drift`，可抑制少量离群点。
4. 用拟合结果重新预测并配对，迭代至配对稳定。
5. 将每段的估计偏移和漂移分别减去，得到逐段修复后的残差。

以下任一条件会使一段被标记为异常：

- 段起点或段末端绝对偏移超过 `offset_tolerance_ms`；
- 有可靠漂移估计且漂移超过 `drift_tolerance_ppm`；
- 修复后仍有残差超过 `offset_tolerance_ms`；
- 与上一段在切点处的预测偏移跳变超过容差；
- 该段有事件但有效配对数少于 `min_pairs`。

`first_anomaly` 是时间顺序上第一个异常段，输出其 `source`、段起点、首次
原始误差超差样本和全部原因。修复误差同时给出所有段的汇总 RMS/MAX 以及
每段 RMS/MAX。

## 示例结果

`examples/splice.json` 是三段合成数据。运行结果是：

```text
首个异常源: cam-b（段 B，起点 6000.000 ms）
修复前误差: RMS 34.428 ms, MAX 44.925 ms
逐段修复后误差: RMS 0.000 ms, MAX 0.000 ms
单一全局偏移/漂移对照: RMS 10.866 ms, MAX 20.256 ms
```

示例没有加入检测噪声，所以逐段修复误差为零。真实输入应使用音频脉冲检测器
和画面闪光/事件检测器产生的首帧或采样点时间；若检测量本身有抖动，修复后
误差会反映该抖动，而不是被强制归零。

## CLI 参数

JSON 内的 `options` 可由命令行覆盖：

```text
-match-window-ms
-offset-tolerance-ms
-drift-tolerance-ppm
-min-pairs
-min-drift-span-ms
```

`-json` 输出完整结构化结果。`go run ./cmd/avsync -h` 可查看帮助。
