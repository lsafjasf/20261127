# ProRes Alpha / Color Consistency Check

用 Go 检查 ProRes 源代理与转码代理之间的颜色说明、Alpha 标记和像素通道一致性，输出：

- `wrong-params.json`：错误/风险参数、期望值、实际值和可直接参考的 FFmpeg 修复参数。
- `report.json` / `report.txt`：元数据、逐帧 Alpha 统计、直通/预乘模型误差和修复动作。
- `*_repaired.png`：归一化为直通 Alpha 的修复帧。
- `*_alpha.png`：修复后 Alpha 蒙版。
- `*_checker_before_after.png`：原解码帧与修复帧在棋盘底上的前后对照。
- `repaired-alpha-proxy.mov`（可选）：从基准源重编码的 ProRes 4444 Alpha 代理。
- `repair-from-source.sh`：带显式颜色和 Alpha 参数的可执行修复命令。

核心元数据解析、通道统计、Alpha 修复和 PNG 输出只使用 Go 标准库；真实 ProRes 解码/编码通过 `ffmpeg`/`ffprobe` 完成。

## 快速运行

```bash
go test ./...
go run ./cmd/proalpha demo --dir examples/demo
# 或
go run . demo --dir examples/demo
```

Demo 会生成一个带 BT.709 `colr` 和 `alpha_channel_type=2` 的 ProRes 4444 源，以及两个典型坏代理：

- ProRes Proxy / `yuv422p10le`：Alpha 被丢弃，颜色标签被改成 SMPTE 170M。
- ProRes 4444：Alpha 仍在，但 RGB 是预乘值；按直通 Alpha 播放时边缘发黑。

已生成的报告在 `examples/demo/report/report.txt`，错误参数在 `examples/demo/report/wrong-params.json`。

## 检查自己的素材

```bash
go run ./cmd/proalpha check \
  --source source-prores4444.mov \
  --variant converted-proxy-a.mov \
  --variant converted-proxy-b.mov \
  --out alpha-report \
  --max-frames 10 \
  --repair-video
```

同一变体可重复传 `--variant`。位置参数也会被当作待检查变体：

```bash
go run ./cmd/proalpha check --source source.mov bad-proxy.mov --out alpha-report
```

存在 `error` 级问题时命令默认返回非零，便于接入批处理；只想取得报告可加 `--no-exit-code`。

## 检查项

### 元数据

- FFprobe 的 `codec_name`、`profile`、`pix_fmt`、颜色四元组和帧率/尺寸。
- MOV/MP4 `colr` box 中的 `nclc`/`nclx` 参数。
- ProRes `icpf` 帧头中的 `alpha_channel_type`：`0` 为无 Alpha，`1` 为 8-bit，`2` 为 16-bit。
- 注意：`alpha_channel_type` 只说明 Alpha 位深，不提供可靠的“直通/预乘”解释；因此还要做下面的通道模型比较。
- “帧头有 Alpha，但 `pix_fmt` 无 Alpha”等元数据自相矛盾。
- 源与转码件之间 `color_primaries`、`color_trc`、`colorspace`、`color_range` 的缺失或冲突。

### 像素通道

FFmpeg 输出统一解码为 `rgba64le`，Go 侧按直通 NRGBA64 语义分析：

- Alpha 最小/最大值、透明/半透明/不透明像素数。
- 变体 Alpha 是否整体丢失。
- 半透明像素中是否大量出现 `max(R,G,B) > A`，这是否定预乘表示的强证据。
- 与源帧比较直通模型 `RGB_variant ≈ RGB_source` 和预乘模型 `RGB_variant ≈ RGB_source × A_source / 65535` 的平均误差。
- Alpha 通道平均绝对误差。

## 修复策略

- **Alpha 被丢弃**：把对齐缩放到变体尺寸的源 Alpha 挂回变体 RGB，输出直通 PNG。
- **RGB 已预乘却被当成直通**：执行 `RGB_straight = clamp(RGB_premultiplied × 65535 / A)`，Alpha 保持不变。
- **没有参考源且只有单文件迹象**：可能是深色直通内容，报告标为低置信度，不假装确定。
- **元数据/编码格式错误**：报告给出重编码参数；`--repair-video` 会从源生成显式 BT.709、ProRes 4444、`yuva444p10le`、16-bit Alpha 的 MOV。

推荐的 Alpha 代理参数：

```bash
ffmpeg -i source.mov -map 0:v:0 -an \
  -c:v prores_ks -profile:v 4444 \
  -pix_fmt yuva444p10le -alpha_bits 16 \
  -color_primaries bt709 -color_trc bt709 \
  -colorspace bt709 -color_range tv \
  repaired-alpha-proxy.mov
```

如果交付链路必须使用 H.264/H.265 等不支持 Alpha 的代理，应另外输出 QTRLE/PNG/ProRes 4444 Alpha 母版，或把素材显式合成到约定背景上；不能只依赖播放器对隐藏 Alpha 标记的猜测。

## 限制

- 真实 ProRes 码流解码依赖外部 `ffmpeg`/`ffprobe`；缺少它们时仍可解析部分 MOV 元数据，但不能做像素检查。
- 若转码时已经把透明区域合成到未知背景，单靠挂回 Alpha 不能恢复被背景污染的 RGB；报告保留该限制，生产修复应从原始 ProRes 重新编码。
- 颜色标签决定 YUV→RGB 矩阵和范围解释；本工具报告标签冲突和最终 RGBA 差异，但不反向猜测所有设备的私有色彩 LUT。

## 项目结构

- `cmd/proalpha`：`check`、`fixtures`、`demo` CLI。
- `internal/mediameta`：FFprobe JSON 与纯 Go MOV/ProRes 元数据解析。
- `internal/mediaio`：FFmpeg raw RGBA64 解码和修复代理编码。
- `internal/imagex`：Alpha 统计、预乘/反预乘、棋盘合成、PNG 输出。
- `internal/check`：一致性规则、逐帧修复和 JSON/TXT 报告。
- `internal/synthetic` / `internal/fixture`：可复现的 Alpha/颜色测试画面与媒体夹具。

## 规格参考

- SMPTE RDD 36, Apple ProRes Bitstream Syntax and Decoding Process: <https://pub.smpte.org/latest/rdd36/rdd36-2022.pdf>
- Library of Congress, Apple ProRes 4444 Codec Family: <https://www.loc.gov/preservation/digital/formats/fdd/fdd000527.shtml>
- Apple ProRes White Paper: <https://www.apple.com/final-cut-pro/docs/Apple_ProRes.pdf>
