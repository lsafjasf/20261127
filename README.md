# lfecheck：低频效果声泄漏定位与修复

`lfecheck` 是一个纯 Go（仅标准库）的多声道 WAV 诊断工具。它不是只看节目总电平，
而是分别比较每个声道的低频能量，检查低频与对白声道的时间相关、时延和增益，再用
中心声道对白频带做门控，输出泄漏源、持续样本区间和修复前后证据。

## 演示结论

`artifacts/demo` 内是可复现的 48 kHz、16-bit、5.1 样本。生成器在 FC（对白）中注入了
`0.950–2.050 s` 的 LFE 泄漏，增益 `0.28`、延迟 `13` samples。

工具自动得到：

- 泄漏源：`LFE`（channel 3），不是由总电平猜测。
- 持续区间：samples `44592–99168`，即 `0.929–2.066 s`。
- 时延与相关：`13 samples`，相关系数 `0.999982`。
- 拟合泄漏增益：`0.280003`，与注入的 `0.28` 一致。
- 修复后 FC 低频 `20–120 Hz`：`-27.61 -> -61.98 dBFS`，降低 `34.37 dB`。
- 修复后与 LFE 的相关：`0.999982 -> 0`；再次分析无泄漏区间。
- FC 对白频带 `150–4000 Hz` 变化：`-0.000027 dB`，对白主体保持不变。
- `sample_evidence.csv` 保存 48 个实际写入样本的 onset/peak/release 前后值。

打开这些文件可直接核查：

- `artifacts/demo/program.wav`：修复前
- `artifacts/demo/program_repaired.wav`：修复后
- `artifacts/demo/repair_report.json`：完整前后审计
- `artifacts/demo/sample_evidence.csv`：逐样本证据
- `artifacts/demo/frames_before.csv` / `frames_after.csv`：逐帧各声道低频、相关和泄漏判定

## 方法

1. 解析 PCM/IEEE-float WAV，按 WAVE speaker mask 映射 `FL/FR/FC/LFE/...`；
   标准 5.1 顺序缺省为 `FL FR FC LFE BL BR`。
2. 对每个声道做零相位 Butterworth 带通，默认提取 `20–120 Hz`；对白存在性来自
   FC 的 `150–4000 Hz` 频带。
3. 以 `100 ms` 窗、`20 ms` 跳步计算每个声道的低频 RMS dBFS；中心对白用噪声底和
   峰值自适应门控。
4. 对每个非对白声道，在与对白重叠的帧内搜索 `±20 ms` 时延，用归一化互相关排序。
   自动模式选择低频时间相关最强的声道，也可用 `--source` 强制指定。
5. 相关绝对值超过阈值、且低频高于可听底噪/对白动态门限的连续帧合并为事件。事件边界
   再用 `10 ms` 包络细化到样本。
6. 修复只在 FC 的已定位区间内，从原始样本中减去按最小二乘估计、按样本对齐的源声道
   低频分量；区间边缘做 `10 ms` 升余弦淡入淡出。其他声道不修改。
7. 修复结果先编码，再解码回样本并重新分析。报告中的 `after` 和逐样本证据来自这个
   实际输出，而不是量化前的临时缓冲。

## 使用

```bash
go test ./...
go run ./cmd/lfecheck demo
```

分析已有 WAV：

```bash
go run ./cmd/lfecheck analyze \
  --in program.wav \
  --dialogue FC \
  --source auto \
  --report analysis.json \
  --frames-csv frames.csv
```

定位并修复：

```bash
go run ./cmd/lfecheck repair \
  --in program.wav \
  --out repaired.wav \
  --dialogue FC \
  --source auto \
  --report repair_report.json \
  --evidence-csv sample_evidence.csv
```

常用参数：

```text
--low 20 --high 120            低频带
--window 100ms --hop 20ms      分析帧
--max-lag 20ms                 最大声道时延
--min-corr 0.55                最小绝对帧相关
--min-duration 120ms           最短泄漏事件
--merge-gap 80ms               合并相邻事件
--low-floor -60                低频可听底噪 dBFS
--leak-margin 3                泄漏高出可见度门限的余量
--leak-below-dialogue 30       泄漏最多低于中心对白 30 dB
--fade 10ms                    修复边缘淡变
```

声道可传名称或从零开始的索引，例如 `--dialogue 2 --source 3`。没有 speaker mask 的
6 声道文件按标准 5.1 顺序解析。

## 证据格式

`sample_evidence.csv` 每行对应一个真实样本：

```text
sample,time_sec,dialogue_before,source_low_aligned,removed,dialogue_after,...
```

其中 `removed = dialogue_before - dialogue_after`，`source_low_aligned` 是按时延对齐
后的 LFE 低频。JSON 同时给出修复区的 RMS、相关、对白频带变化和修复后事件数，能区分
“低频降到噪声底”与“只是降低总电平”。

## 支持范围

- RIFF/WAVE：PCM 8/16/24/32-bit、float 32/64-bit、WAVE_FORMAT_EXTENSIBLE。
- 任意声道数；标准 5.1/7.1 speaker mask 自动命名。
- 非 WAV 容器需先转为 WAV；项目本身不引入外部音频库或非 Go 组件。
- 自动定位依赖源声道低频与对白低频的时间相关。若生产链路的泄漏源已知，建议用
  `--source LFE` 固定来源，避免把对白自身的低频成分误判为源。

## 代码结构

- `internal/audio/wav*.go`：WAV 编解码、声道映射。
- `internal/audio/dsp.go`：零相位滤波、RMS、互相关。
- `internal/audio/analyze*.go`：逐声道低频、对白门控、源排序和事件。
- `internal/audio/repair*.go`：定向抵消、量化后复验、逐样本证据。
- `internal/fixture/program.go`：确定性 5.1 注入样本。
- `cmd/lfecheck`：`analyze`、`repair`、`demo` 命令。
