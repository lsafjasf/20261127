# silencedetect

纯 Go 标准库实现的音频静音/停顿分析工具，用于排查“两段看似相同的静音长度相差几十毫秒、
导致后续字幕提早”这类问题。不依赖任何第三方包，支持 WAV（PCM 8/16/24/32-bit、IEEE
float32/64、`WAVEFORMATEXTENSIBLE`）和裸 PCM 输入。

## 检测模型

每个分析窗口（默认 10 ms）按峰值电平（可用 `-rms` 改为 RMS）归入互斥的三档之一，
避免同一段静音在多个阈值下重复报告：

| 档位 | 判定条件 | 含义 | 处置建议 |
| --- | --- | --- | --- |
| `digital` | 窗口内全部样本精确为 0 | 真正的数字静音 | `removable`，可无损裁掉 |
| `noise` | 峰值 ≤ `-noise-db`（默认 -60 dBFS） | 低电平底噪，非真静音 | `removable`，属底噪 |
| `pause` | 峰值 ≤ `-pause-db`（默认 -45 dBFS） | 有人声内容的停顿/换气 | 按时长分级，见下 |

区间先按窗口生成，再做样本级边界修正，因此起止时间和时长精确到单个采样点（48 kHz 下
约 0.02 ms），足以区分几十毫秒的长度差异。短于 `-min-ms`（默认 30 ms）的区间直接丢弃；
停顿内部长度不超过 `-bridge-ms`（默认 15 ms）且不超阈值 6 dB 的瞬时点击窗口会被桥接，
不打断一个停顿区间。

## 自然停顿分级

仅对 `pause` 档的**内部**区间按时长分级（默认值可调）：

| kind | 时长范围 | verdict |
| --- | --- | --- |
| `micro-gap` | < `-micro-ms`（200 ms） | `acceptable` |
| `natural pause` | ≤ `-natural-ms`（800 ms） | `acceptable` |
| `long pause` | ≤ `-long-ms`（2000 ms） | `review` |
| `extended pause` | > 2000 ms | `suspect`，优先排查的同步隐患 |

位于文件头部/尾部的区间（`lead-in`、`trailing`、`whole-file`）统一标为 `boundary`，
提示先核对文件偏移与拼接点，而不是当作自然停顿。

## 输出

- 文本报告：每个声道（`ch=0..n-1`）外加一行 `ch=-1` 的“节目级”视图——该帧只有在
  所有声道同时安静时才算安静，这是字幕对齐决策应使用的口径。
- 每行包含起止秒（6 位小数）、毫秒时长、峰值/RMS（dBFS，真静音显示 `-inf`）、零样本
  百分比、位置、kind 和 verdict。
- 传入两个及以上文件时，末尾输出 `cross-file comparison`，按顺序对齐同名档位区间并
  给出相对首个文件的时长差（`delta`，毫秒），几十毫秒的偏差直接可见。
- `-json` 输出机器可读 JSON；真静音的 dBFS 序列化为 `null`。

## 用法

```sh
go build -o silencedetect .

# 单文件文本报告
./silencedetect track.wav

# 两个版本对比，定位长度差异
./silencedetect v1.wav v2.wav

# 更严格的底噪门限 + RMS 判定
./silencedetect -noise-db -65 -rms track.wav

# 裸 PCM（无 WAV 头）
./silencedetect -raw-rate 48000 -raw-channels 2 -raw-bits 16 track.pcm

# JSON
./silencedetect -json track.wav
```

全部参数：`-noise-db`、`-pause-db`、`-window-ms`、`-bridge-ms`、`-min-ms`、
`-micro-ms`、`-natural-ms`、`-long-ms`、`-rms`、`-json`，以及裸 PCM 用的
`-raw-rate`、`-raw-channels`、`-raw-bits`、`-raw-float`。

## 测试

```sh
go test ./...
```

测试用标准库合成立体声信号，覆盖数字静音样本级边界、-70 dBFS 底噪、-50 dBFS 自然停顿、
两文件 50 ms 静音差、最小长度过滤、WAV 多 chunk 解析与 CLI 端到端输出。
