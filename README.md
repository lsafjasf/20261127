# blacklift

局地黑位（black level）补偿工具，纯 Go 标准库实现，无第三方依赖。

解决的问题：裁切后的素材露出被抬升的黑位并呈现灰雾；传统“按全帧最低值统一减黑”
会把真正的夜景（黑位本来就在传感器地板、且有明显低照噪声）压死。`blacklift`
结合 **黑电平估计、镜头遮罩（径向）模型、低照噪声地板** 做逐像素局地补偿，并自动
识别和保留**有意抬黑的风格镜头**。所有补偿量都有硬上限与噪声地板双重约束（限定 Go）。

## 工作原理

1. **瓦片统计**：把画面分成网格（默认 16x9，瓦片不小于 24px），统计每瓦片均值、
   最小值、P1/P5，并用水平邻域差分的 MAD 估计高斯噪声 σ（Var(差分)=2σ²）。
2. **镜头遮罩拟合**：把像素按到画面中心的归一化半径 ρ（角点 ρ=1）分桶，取每桶
   最低 2% 的均值作为局地黑地板，加权最小二乘拟合径向二次曲面
   `b(ρ)=c0+c1ρ+c2ρ²`，再做一轮 Huber 鲁棒重加权。亮瓦片自动降权。
3. **低照噪声地板**：每像素补偿 `c = min(b(ρ) − k·σ, 上限)`，且不为负。即只扣除
   高出噪声包络的“确定性灰雾”，不把噪声本身减掉——夜景因此不会被压死。
4. **黑位保护（BlackGuard）**：任何像素补偿后亮度不低于 `BlackGuard`（默认 2/255），
   避免新增零值截断；超过保护的部分计入 `guard_frac`。
5. **风格镜头识别**：综合全帧 P1、零像素占比、噪声强度、遮罩空间一致性（spread）
   打分；均匀、干净、无噪声、无径向梯度的抬黑判定为有意风格，经时域多数投票（默认
   5 帧窗口）后整段保留（默认补偿比例 0）。可用 `-force-style` 手动覆盖。
6. **时域平滑**：对原始补偿图做前向+后向 EMA（零相位），消除帧间闪烁且不产生拖影。

## 用法

```bash
# 1. 生成三类合成测试素材（灰雾事故 / 真夜景 / 有意抬黑）
go run ./cmd/genscenes -out testdata

# 2. 运行补偿
go run ./cmd/blacklift \
  -in testdata/fog_cropped -out out/fog_cropped \
  -csv out/fog_cropped.csv -json out/fog_cropped.json

# 只分析出报告、不写校正帧
go run ./cmd/blacklift -in <frames> -dry-run
```

支持输入：PNG、PNM（PGM/PPM，P2/P3/P5/P6）。输出统一为 8-bit PNG。

### 关键参数

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `-max-comp-8bit` | 16 | 单像素最大减黑量（8-bit 级），硬上限 |
| `-noise-k` | 1.5 | 噪声地板系数，保留 k·σ 不减 |
| `-style-scale` | 0 | 风格镜头的补偿比例（0=完全保留） |
| `-style-thresh` | 0.62 | 风格门判定阈值 |
| `-smooth-alpha` | 0.65 | 时域 EMA 系数，1 关闭平滑 |
| `-vote-window` | 5 | 风格时域投票窗口（帧） |
| `-force-style` | auto | `auto`/`lift`/`normal` 手动覆盖 |

## 报告字段（逐帧 CSV / JSON）

- 补偿量：`raw_mean/max_8bit`（遮罩原始值）、`bounded_mean/max_8bit`（噪声+上限约束后）、
  `applied_mean/max_8bit`（BlackGuard 后实际作用值）。
- 遮罩：`mask_base/corner/spread_8bit`、`mask_r2`。
- 噪声：`sigma_8bit`（全帧中位瓦片噪声）。
- 暗部细节：`shadow_detail_before/after_8bit`（亮度 <10% 像素的平均亮度）。
- 新增截断：`new_clipped_luma_px/frac`（亮度被压到 0 的像素）、
  `new_clipped_anychan_px/frac`（任一 RGB 通道被压到 0）。
- `guard_frac`：触发 BlackGuard 被限制补偿的像素占比。

## 边界保证

- 补偿永不为负、永不超过 `-max-comp-8bit`、永不进入 `k·σ` 噪声地板以下。
- 真夜景（黑位贴近 0、噪声大）：补偿自动趋于 0，像素原样保留。
- 有意抬黑（均匀、低噪、无径向梯度）：风格门整段保留，不做减黑。
- 所有截断像素逐帧计数并写入报告，可直接用于质量门禁。

## 测试

```bash
go test ./...
```

包含噪声估计、径向遮罩拟合、风格分类、补偿上限/噪声地板/BlackGuard 的单元测试，
以及基于合成素材的端到端集成测试。
