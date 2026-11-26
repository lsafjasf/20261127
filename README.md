# clickdetect

用于查找音频切点处偶发点击声的 Go 命令行工具。它不依赖平均响度或整段频谱，而是检查切点附近的：

- **样本不连续**：一阶差分的局部稳健离群值。
- **相位跳变**：200–1000、800–3000、2500–8000 Hz 三个频带中，切点两侧解析信号相位的线性拟合外推差。
- **低频冲激/阶跃**：180 Hz 低通信号的 2 ms 滑动均值边缘、切点前后 8 ms 均值阶跃，以及局部低频峰值。

同时使用爆发持续样本数、静音后持续起音和衰减尾部来保护正常鼓点和打击乐。被保护的瞬态不会计为点击，会记录在 `suppressed_transients` 中。

## 构建与运行

```bash
go build -o clickdetect .
./clickdetect song.wav
```

默认输出点击样本、指标和点击样本前后 12 个采样点的波形。`<` 标记点击所在样本：

```text
Detected clicks: 1

Click 1: sample 22050 (0.459375 s), channel(s) 0, confidence 0.700
  indicators: sample_discontinuity
  jump=0.658356 (57.19x local p99, z=156.5), phase=0.29 rad (coherence 0.76)
  low-frequency step=0.00400124 (z=0.1), impulse=0.0469627 (z=1.2), burst samples=2
  waveform samples 22042..22058 ('<' marks click sample):
    sample          ch0
      22049    0.1330566
  <   22050    0.7914124
      22051    0.1493225
```

JSON 模式适合批处理或画图：

```bash
./clickdetect --json --before 24 --after 24 song.wav > result.json
```

常用参数：

```text
--json                 输出 JSON
--before N             点击前保留 N 个样本，默认 12
--after N              点击后保留 N 个样本，默认 12
--min-confidence F     置信度阈值，默认 0.50
```

## 输出约定

- 样本编号从 0 开始；`sample` 是切点后第一个样本（即不连续边界右侧样本）。
- `waveform.samples` 按声道存放，范围包含 `sample - before` 到 `sample + after`。
- `flags` 可能包含 `sample_discontinuity`、`phase_jump`、`low_frequency_impulse`。
- `suppressed_total` 是被鼓点/持续起音保护规则排除的瞬态数。可见的抑制明细在 `suppressed_transients` 中。

## 输入格式

标准库直接解析 WAV：PCM 8/16/24/32 位、IEEE float 32/64 位，以及 `WAVE_FORMAT_EXTENSIBLE`。非 WAV 文件可先转换：

```bash
ffmpeg -i input.mp3 -c:a pcm_s16le -ar 48000 output.wav
./clickdetect output.wav
```

音频按声道独立分析，同一声道的相邻事件会合并。多声道同一时刻的点击会合并成一个事件，并在 `channels` 中列出所有命中的声道。

## 算法要点

1. 以 10 ms 分块统计一阶/二阶差分及低通差分的局部中位数和 p99，再用相邻块的中位数作为基线，避免把整体响度当作点击判据。
2. 对候选边界计算局部稳健 Z 值、相对 p99 跳变、5 ms 内的高差分爆发样本数。
3. 用 Hilbert 解析信号在三个频带内分别拟合切点前后的相位，并以拟合残差作为相干性门限；低相干噪声不会仅凭相位被判为点击。
4. 用 180 Hz 低通信号的 2 ms 滑动均值边缘、切点前后 8 ms 均值差和 ±4 ms 峰值检测低频阶跃/冲激；平滑的 1 ms 阶跃也能检出，位置精度约为 1 ms。
5. 若候选是“从安静到持续声音”的起音，或 5 ms 内存在大量高差分样本，则按打击乐瞬态抑制。抑制规则只排除持续爆发，孤立样本跳变仍会保留。

该规则面向编辑切点产生的 1–数个样本的断裂。严重削波、颗粒合成、黑胶爆音等本身具有密集宽带瞬态的素材，可能需要通过 `--min-confidence` 调整灵敏度。

## 测试

```bash
go test ./...
go vet ./...
```

测试覆盖纯音零误报、单样本点击、相位拼接、硬/平滑低频阶跃、44.1 kHz、立体声合并、WAV 多格式解码，以及平滑起音、无淡入硬鼓点、鼓组叠加持续铺底不误报和鼓组叠加点击仍可检出的场景。
