# clipforensic

纯 Go 标准库实现的削波/失真取证工具。它不会只看整片峰值，而是在候选时间窗内同时检查：

- **平台计数**：近满刻度、相邻样本几乎不变的连续平台段数量、样本数、最长长度和精确 0 dBFS 样本数。
- **上升沿**：进入平台前的最大跳变、局部斜率、10–90% 上升时间、90% 到平台的拐角样本数和陡沿比例。
- **频谱证据**：频带能量、频谱质心/平坦度/滚降、基频、谐波强度、奇偶次谐波比，以及事件窗相对前后干净邻段的 2–12 kHz 扩散差。
- **艺术性失真保护**：检测拨弦/音符音头、稳定基频和谐波序列；没有数字天花板平台时归为艺术性失真，不生成事故事件。
- **编码/母带归因**：提供编码前母带与平台分发版时，自动对齐样本并比较同位置平台是否为新增。

当前工作区初始化时没有用户的真实音频，因此 `artifacts/demo` 是一个确定性 Go 合成样例：
`music_limiter_clip.wav` 在两个已知时间窗被总线驱动到 0 dBFS 硬平台，
`distorted_guitar.wav` 是带音头、基频和密集谐波的软失真吉他。

## 快速开始

```bash
go test ./...
go run ./cmd/clipforensic demo --out artifacts/demo
```

分析真实的两段 WAV：

```bash
mkdir -p run
go run ./cmd/clipforensic analyze \
  --report run/report.json \
  --candidates-csv run/candidates.csv \
  --waveform-csv run/waveform_evidence.csv \
  --svg-dir run/waveforms \
  --markdown run/evidence.md \
  music.wav distorted_guitar.wav
```

终端会列出事故候选和被保护的艺术性失真区间。完整逐样本波形在
`waveform_evidence.csv`，可直接画图的波形在 `waveforms/*.svg`。

支持 RIFF/WAVE 的 PCM 8/16/24/32-bit 与 IEEE float 32/64-bit。MP3/AAC/Opus
需先解码成 WAV，例如：

```bash
ffmpeg -i platform.mp3 -c:a pcm_f32le platform_decoded.wav
```

这一步只是输入解码；检测器、FFT、对齐、分类和证据输出均为 Go 标准库实现。

## Demo 结论

最新一次确定性运行结果如下。同样的两文件分析也输出在 `artifacts/result.json`、
`artifacts/evidence.md`、`artifacts/candidates.csv` 和 `artifacts/waveforms/`。

| 文件 | 候选/保护 ID | 时间 (s) | 结论 |
|---|---|---:|---|
| `music_limiter_clip.wav` | `C001` | `0.482500–0.789479` | 母带/限幅器硬削波候选，置信度 `0.81` |
| `music_limiter_clip.wav` | `C002` | `1.328000–1.751479` | 母带/限幅器硬削波候选，置信度 `0.81` |
| `distorted_guitar.wav` | `A001–A007` | `0.145–2.550` | 7 个失真吉他音，`incident=false` |

音乐文件的全局证据为峰值 `0.999969`、`8452` 个精确满刻度样本、`383` 组平台段；
两个候选分别有 `139` 和 `150` samples 的最长平台。吉他峰值约 `-1.34 dBFS`，
但精确满刻度样本和近顶平台段均为 `0`，每个音都有可估计基频（约 `165–329 Hz`）
和 `0.96` 左右的谐波强度，因此不会被标成事故。

打开这些文件核查：

- `artifacts/demo/evidence.md`：人读报告和 SVG 索引。
- `artifacts/demo/report.json`：完整指标、原因和逐样本波形数组。
- `artifacts/demo/candidates.csv`：候选时间、置信度和特征汇总。
- `artifacts/demo/waveform_evidence.csv`：每个候选前后真实样本、平台掩码和证据点。
- `artifacts/demo/waveforms/`：平台点标红、证据上升沿标橙的 SVG。
- `artifacts/demo/demo_truth.json`：合成时注入的已知时间窗。

## 判断编码还是母带

只有一个分发文件时，信号中的精确同值/满刻度长平台更符合母带总线限幅或硬削波；
但无法仅凭解码后的 PCM **证明**某个有损编码器生成了它。有损编码通常表现为边缘圆角、
振铃/预回声、少量过冲或 codec frame grid 关联，而不是一长串完全相同的 16-bit 天花板样本。

有编码前母带时运行：

```bash
go run ./cmd/clipforensic compare \
  --master master.wav \
  --encoded platform_decoded.wav \
  --codec-frame-samples 1024 \
  --report compare/report.json \
  --markdown compare/compare.md \
  --svg-dir compare/waveforms
```

`--codec-frame-samples` 可按格式填：AAC 常用 `1024`，MP3 常用 `1152`。
比对器先用首个音头做粗对齐，再用归一化互相关在 ±192 samples 内细化。

归因规则：

- `plateau_present_in_master__mastering_or_prefile_limiter`：母带同位置已有同尺度平台，削波来自母带/限幅器或更早处理，不是平台编码新增。
- `new_plateau_in_distribution__encode_decode_or_transcode_candidate`：母带干净而分发解码版出现新平台，支持编码、转码、采样率转换或母带后增益引入。
- `master_clip_with_distribution_change`：母带已有削波，编码/转码又改变了平台长度或边缘。
- 无母带参考时只输出 `mastering_limiter_clip` 信号分类和置信度，不把可能性写成已证实的编码事故。

## 防误报设计

高频能量、失真音色或低频谱平坦度本身不能作为事故依据。吉他保护区间必须同时满足：

1. 有拨弦/音符上升沿或清晰的音头分割；
2. 能估计出 `55–1200 Hz` 的稳定基频；
3. 谐波强度足够高，且奇次谐波/高频结构符合失真音色；
4. 缺少精确数字天花板平台和长同值平台。

阈值可通过 `--ceiling-threshold`、`--flat-tolerance`、`--min-run` 和
`--min-confidence` 调整。建议先看 CSV/SVG 中的真实平台与上升沿，再为具体母带链微调，
不要只提高或降低单一峰值阈值。

## 代码结构

- `internal/wav`：纯 Go WAV 解码和 PCM16 写出。
- `internal/dsp`：radix-2 FFT、分位数等基础 DSP。
- `internal/analyze`：平台段、上升沿、频谱、吉他保护、候选合并和母带对齐。
- `internal/reportio`：JSON、CSV、Markdown 和 SVG 证据输出。
- `internal/demo`：确定性的削波音乐与失真吉他合成器。
- `cmd/clipforensic`：`analyze`、`compare`、`demo` 命令。
