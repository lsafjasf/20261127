# 室外镜头亮度周期变化分析

用 Go 标准库区分室外视频中的三类亮度变化：

- LED 频闪：稳定背景中的窄带周期分量，与前景运动不相关。
- 自动曝光：短时、跨区域近似一致的亮度阶跃或回稳过程。
- 真实云影：持续存在、常伴随区域相位差或非均匀照度变化的慢变过程。

分析器直接读取 Y4M 的 Y 平面。JSON 同时输出三路独立证据、频带统计、逐帧时间曲线和保留的事件；CSV 可单独导出时间曲线。除容器转码外，所有分析代码只依赖 Go 标准库。

## 快速开始

```bash
go test ./...
go build -o lightanalyze ./cmd/lightanalyze

./lightanalyze -out report.json -csv curves.csv input.y4m
```

从常见视频生成 Y4M 并直接走标准输入：

```bash
ffmpeg -i outdoor.mp4 -vf fps=60 -pix_fmt yuv420p \
  -f yuv4mpegpipe - \
  | ./lightanalyze -out report.json -csv curves.csv -
```

`ffmpeg` 只负责解封装和转为 8-bit Y4M，亮度分析本身完全由本项目的 Go 标准库代码完成。

常用参数：

```text
-max-width 320          分析块的最大横向网格数
-motion-threshold 7     归一化帧差被判为运动块的阈值
-event-slope 0.18       持续光变的最低斜率（亮度/秒）
-event-min-amplitude 2.5
-slow-cutoff 0.5        慢变频带上界
-flicker-low 4          频闪频带下界
-pretty=false           输出紧凑 JSON
```

## 三路证据

### 稳定背景

- 相邻帧先做稳健增益/偏移归一化，消除全局曝光和频闪对帧差的影响。
- 通过运动种子、长期背景模型和连通域排除整块运动前景，而不是只排除物体边缘。
- 对剩余稳定块取加权中位数，避免少量前景或稳定区集合变化拉动背景基线。
- 输出 `background_rms_luma`、`sustained_range_luma`、`correlation_with_global` 以及稳定背景时间曲线。

### 前景运动

- 独立输出运动面积比例、前景残留亮度和与全局高频分量的相关性。
- LED 频闪应在稳定背景中强、在运动时间曲线上无可解释的周期；真实前景运动应主要出现在这一路。
- 大块前景移动可以显著改变全帧均值，但不应触发稳定背景上的持续光变事件。

### 全局亮度

- 全帧块均值用于全局时间曲线和全局频带，不替代稳定背景证据。
- 输出原始全局曲线、慢变全局曲线和 `global - sustained` 高频残差。
- 频带按 `global`、`stable_background`、`foreground_motion` 三路分别统计。

## 频带与曲线

默认频带：

| 名称 | 范围 | 主要含义 |
| --- | --- | --- |
| 慢变 | 0–0.5 Hz | 云影、缓慢日照变化 |
| 曝光 | 0.5–4 Hz | 曝光阶跃、回稳 |
| 频闪 | 4 Hz–Nyquist | LED、电源频率混叠 |

每个频带输出 RMS 亮度、相对功率、峰值频率、峰值集中度和是否落在频带边缘。时间曲线包含：

```text
time_s
global_luma
stable_background_luma
stable_background_fraction
foreground_luma
foreground_motion_fraction
sustained_luma
sustained_background_luma
fast_residual_luma
stable_region_luma[0..8]
```

区域亮度按行优先排列，默认 `3 x 3`。`-csv` 输出的列与上述曲线一一对应。

## 判定与保留

- `led_flicker`：背景频闪频带具有足够能量和高集中度峰值，且运动证据不能解释该周期。
- `auto_exposure`：光变在短时间内完成，区域幅度一致、跨区域相位差很小；高斜率可补偿平滑造成的时长扩张。
- `cloud_shadow`：光变持续、区域相位差较大或区域幅度不一致。
- `mixed`：LED 与曝光/云影证据同时存在。
- `stable`：没有超过阈值的周期或持续变化。

分类不是靠把慢变直接高通删除得到的。原始亮度、持续全局曲线、持续背景曲线，以及每个 `retained: true` 的事件都会保留；`fast_residual_luma` 只用于频带和相关性证据。

事件还附带：

```json
{
  "kind": "cloud_shadow",
  "amplitude_luma": -11.3,
  "retained": true,
  "evidence": {
    "region_uniformity": 0.88,
    "region_coverage": 1.0,
    "region_phase_spread_s": 1.56
  }
}
```

## 采样限制

- `y4m` 包支持 8-bit `420`、`422`、`444` 和 `mono`；10-bit 与 alpha 格式会被拒绝，避免把高位深字节当亮度。
- 频带只能识别采样后可见的频率。例如 30 fps 下 100 Hz LED 会混叠到 10 Hz，120 Hz 可能落到 0 Hz 而无法与恒定照明区分。要确认 LED 真实频率，应采集更高帧率或提供卷帘快门原始数据。
- 极快且空间均匀的云影与自动曝光在物理上可能相似；此时应结合区域相位差、斜率、持续时间和置信度判断，而不是只看单一峰值。
- 时间戳不均匀时会给出警告。FFT 频带假定帧间隔恒定，非均匀输入应先重采样。

## 测试覆盖

`go test ./...` 包含以下合成回归：

- 10 Hz LED + 移动前景：应识别为 `led_flicker`，前景运动独立存在。
- 均匀曝光阶跃：应识别为 `auto_exposure` 且事件保留。
- 横向扫过的慢速云影：应识别为 `cloud_shadow` 且慢变范围保留。
- LED 与云影叠加：应识别为 `mixed`，云影事件和持续曲线都不丢失。
- LED 与曝光阶跃叠加：两类证据同时输出。
- 大块前景移动：全帧均值明显变化，但不得误报为云影。
- Y4M 420/mono 解码、时间戳和非法输入。
