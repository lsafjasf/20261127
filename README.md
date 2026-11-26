# Cadence Segmenter

面向 60i 封装素材的 Go 场频/cadence 分段器。它不会仅凭
`interlaced` 标记把整片当作隔行，而是分别判断：

- `24p_3:2`：24p 经过 3:2 pulldown 后的场型；
- `30p_2:2`：真实 30p 以场对形式封装；
- `unknown`：当前局部证据不足，只标记该段。

## 运行

需要 `ffmpeg` 和 `ffprobe`。分析器本身用 Go 实现，`ffmpeg` 只负责解码并把
场拆成低分辨率灰度图。

```bash
go run ./cmd/cadence-segment -i input.mp4
go run ./cmd/cadence-segment -i input.mp4 -format text
go build -o cadence-segment ./cmd/cadence-segment
```

默认输出 JSON。常用参数：

```text
-window 16          每个局部决策使用的帧数
-min-segment 4      最短段帧数
-change-penalty .32 cadence 切换代价
-scene-cut .24      场景切换阈值
-field-width 32     解码后的场宽
-field-height 18    解码后的场高
-fail-on-unknown    任一段 unknown 时退出码为 3
```

## 输出

每段独立给出 `cadence`、`field_order`、`confidence`、`recommended_action`
和可审计的 `evidence`：

```json
{
  "cadence": "24p_3:2",
  "field_order": "tff",
  "confidence": 0.837,
  "evidence": {
    "basis": [
      "35/36 expected mixed field-pair slots match the 3:2 pattern at phase 2",
      "repeating 10-field cycle fit 0.99",
      "field order tff supported by motion continuity"
    ],
    "mixed_field_pairs": 35,
    "coherent_field_pairs": 54,
    "within_pair_contrast": 3.879,
    "period_fields": 10,
    "field_order_score": 1
  },
  "recommended_action": "inverse_telecine_24p"
}
```

关键字段：

- `repeat_*`：五帧型中混合场对位置的命中情况。
- `within_pair_contrast`：帧内上下场差异的周期性起伏；3:2 明显，2:2 很小。
- `within_pair_motion` / `across_pair_motion`：帧内与跨帧场差中位数。
- `motion_continuity`：局部运动连续性得分。
- `field_order_score` / `field_order_source`：场序判定的强度与依据。
- `recommended_action`：`inverse_telecine_24p`、`treat_as_30p_field_pairs`
  或 `isolate_and_review`。

## 局部失败语义

判定按段独立完成。某个短段出现解码损坏、60i 原生场、静止画面或证据冲突时，
只输出该段 `cadence: "unknown"` 和 `recommended_action: "isolate_and_review"`；
前后已确认的 24p/30p 段不会被整片回退，也不会因为一个坏段丢失。

`unknown` 段不会自动套用相邻段的处理方式。后续处理应按帧范围跳过、隔离或人工复核。

## 判定方法

1. `ffmpeg separatefields` 按时间输出场，再降采样成灰度场图。Go 侧不读取
   `field_order` 作为结论，只把它当作先验。
2. 同时构造 TFF 和 BFF 两种候选场序，按连续场差判断运动方向与连续性。
3. 3:2 的周期不是简单数“全片是否隔行”，而是看五个编码帧里的场对结构：
   三个同源场对加两个跨源混合场对。混合位置必须稳定重复，并要求帧内场差有
   明显周期起伏。
4. 2:2 则要求帧内场对一致、跨帧场对运动明显更高，且不存在稳定的 3:2
   五帧调制。
5. 每个时间窗口计算局部分数，用带切换代价的 Viterbi 平滑；再对弱段合并、
   重新判型并精修边界。边界附近很短的“未知过渡”会收敛到两侧，真正缺少证据
   的长段仍保持 `unknown`。

`confidence` 综合三部分：本段模型分数、与另一 cadence 的领先幅度、核心周期
拟合度。`basis` 保留实际命中数、周期相位、场运动量、场序来源和场景切换数，
因此不是只有一个黑盒小数。

## 场序说明

- TFF/BFF 主要由场序列的运动连续性和 3:2 场型共同判定。
- 真实 2:2 的上下场来自同一时间采样，仅凭画面通常无法区分“先上”还是“先下”。
  容器给出 TFF/BFF 时采用元数据；没有元数据时默认 TFF，并在
  `field_order_source` 中明确写出该限制。
- 若片段确实发生场序变化，场序置信度会体现在 `field_order_score`，不会把一个
  低置信场序当作强结论。

## 限制

- 当前按恒定帧率处理，时间由 `场号 / (2 × frame_rate)` 计算；VFR 素材应先在
  上游归一化。
- 完全静止且没有重复场结构的片段无法仅靠画面确定 cadence，会局部标记
  `unknown`，这是预期行为。
- 分辨率极低或压缩非常重时，场差噪声会降低置信度，但局部结论仍只影响本段。

## 验证

```bash
go test ./...
go test -short ./...   # 跳过依赖 ffmpeg 的集成测试
```

测试覆盖：

- 24p 3:2、真实 30p、原生 60i 混合时，坏段只局部隔离；
- TFF/BFF 3:2 的场序判定；
- 实际由 ffmpeg 生成的 24p→3:2 与 30p 拼接素材，检查段边界和 cadence。
