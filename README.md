# flatfield

仅使用 Go 标准库的平场二维空间响应估计与校正表导出工具。它用于校正监视器四角/面板亮度不均和拍摄链路的镜头暗角，同时避免把真实场景渐变误当成固定缺陷。

## 快速使用

```bash
go test ./...

go run ./cmd/flatfield \
  -flat monitor-a-flat.png \
  -dark monitor-a-dark.png \
  -lens-table lens-table.json \
  -in scene.png \
  -out artifacts/scene-corrected.png \
  -table-out artifacts/monitor-a-table.json \
  -report-out artifacts/monitor-a-report.txt \
  -csv-out artifacts/monitor-a-gain-1920x1080.csv \
  -csv-width 1920 -csv-height 1080
```

输入支持 PNG、JPEG 和 GIF 首帧。省略 `-in/-out` 时只估计并导出校正表。

已有 JSON 表可直接复用到其他尺寸的素材，并额外导出场景渐变诊断：

```bash
go run ./cmd/flatfield \
  -apply-table artifacts/monitor-a-table.json \
  -in scene-4k.png -out artifacts/scene-4k-corrected.png \
  -csv-out artifacts/monitor-a-gain-3840x2160.csv \
  -csv-width 3840 -csv-height 2160 \
  -scene-gradient-out artifacts/scene-gradient.json
```

每台监视器分别拍摄平场并导出自己的表即可：

```bash
for f in flats/monitor-*.png; do
  name=$(basename "$f" .png)
  go run ./cmd/flatfield -flat "$f" -kind panel \
    -table-out "artifacts/${name}.json" \
    -report-out "artifacts/${name}.txt" -quiet
done
```

常用 `-kind`：

- `device`（默认）：完整平场响应，即镜头与面板等固定空间响应的乘积，最适合校正拍摄到的屏幕画面。
- `panel`：扣除径向镜头模型后的完整面板场，包含面板方向斜坡。
- `panel_residual`：再扣除仿射渐变后的非仿射面板残差。
- `lens`：径向镜头暗角分量。
- `device_no_gradient`：从完整响应中扣除仿射渐变，仅在该渐变来自平场靶面照明/环境光而非面板时使用。
- `observed` / `gradient`：原始归一化响应和仿射渐变诊断分量。

## 尺寸复用

校正表保存低分辨率网格和归一化像素中心坐标：

```text
u = (x + 0.5) / width
v = (y + 0.5) / height
```

运行时用双线性插值采样 `GainAt(u, v)`，因此同画幅、同取景的 1080p、4K 或其他缩放尺寸可复用同一份 JSON。`ResampleGain(width, height)` 可预生成任意精确尺寸的增益矩阵；CLI 的 `-csv-width/-csv-height` 会导出该矩阵。

不同宽高比或发生裁切、移轴、光学中心变化时，不能直接拉伸复用，应重新标定或变换归一化坐标/光学中心。

## 响应模型与归因

在线性光中，平场观测可写成：

```text
I_c(u,v) = Flat_c * Response(u,v)
log Response = b0 + a*dx + b*dy
             + c2*r^2 + c4*r^4 + c6*r^6
             + PanelResidual(u,v)
```

- `c2,c4,c6` 拟合以光学中心为圆心的径向项，归为镜头暗角。
- `PanelResponse = Response / LensResponse`，表示扣除镜头后的面板不均匀。
- `a,b` 是仿射方向渐变；它可能是平场靶面照明不均、环境渐变，也可能是面板的不对称斜坡。
- `PanelResidualResponse = PanelResponse / GradientResponse` 保留非仿射面板 Mura/角部差异。

单张“相机拍摄监视器平场图”中，镜头暗角和面板径向衰减在数学上是相乘且耦合的；如果面板缺陷也严格径向对称，仅凭该图不能唯一归属。报告中的 `identifiability` 会明确标注：

- 未提供独立镜头表：使用“径向项归镜头、残差归面板”的模型归属，置信度中等。
- 提供 `-lens-table`：镜头来自独立标定，剩余场归面板/照明，归属更确定。

可用同一镜头拍摄均匀漫射靶生成镜头表，或复用本库导出的 `kind=lens` JSON。

## 真实场景渐变

真实场景渐变属于素材内容，不会出现在均匀平场图案中。校正采用乘法除法：

```text
Observed = SceneGradient(u,v) * DeviceResponse(u,v)
Corrected = Observed * Gain(u,v) ≈ SceneGradient(u,v)
```

因此只要增益对应设备响应，场景自身的渐变会被保留，而不是被“拉平”。`EstimateSceneGradient` 可额外估计普通画面的仿射对数渐变用于报告，但它不参与校正表生成。

## 不产生新色偏

校正表是空间标量场；每个像素对 R、G、B 使用同一个增益：

```go
out.R = in.R * gain
out.G = in.G * gain
out.B = in.B * gain
```

这会保持线性 RGB 通道比例和色度，不会施加每通道白平衡曲线。图片输出在即将裁切时对三通道再乘相同保护系数，避免单通道高光裁切造成色偏。空间彩色 Mura 不属于亮度标量校正；若要校正，需要额外的每通道标定并承担改变白平衡的风险。

## Go API

```go
img, _ := flatfield.LoadImage("flat.png")
tab, err := flatfield.Estimate(
    flatfield.ImageRaster{Image: img},
    nil, // 可选 dark Raster
    flatfield.DefaultOptions(),
)
if err != nil { panic(err) }

_ = tab.SaveJSON("artifacts/monitor-a-table.json")
gain := tab.GainAt(0.12, 0.08)
matrix, _ := tab.ResampleGain(3840, 2160)

scene, _ := flatfield.LoadImage("scene.png")
corrected, _ := flatfield.CorrectImage(scene, tab)
_ = flatfield.SaveImage("artifacts/scene-corrected.png", corrected, 95)
```

JSON 中同时保存 `observed_response`、`lens_response`、`gradient_response`、`panel_response`、`panel_residual_response`、主 `response/gain` 和四角统计，便于审计为什么做出某个归因。
