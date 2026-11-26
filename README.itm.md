# 逆色调映射（Inverse Tone Mapping）实验

纯 Go 标准库实现的 SDR→HDR 逆色调映射对比实验。旧 SDR 素材进入 HDR 平台时，
直接做 gamma 反向（等价于线性域固定增益扩展）会把人脸中间调、天空漫反射和
暗部噪声整体抬亮。本项目用带已知真值的合成场景，定量比较三种逆映射策略。

## 三种策略

| 策略 | 实现 | 行为 |
|---|---|---|
| 线性扩展 `linear_expand` | `x = gain * y` | gamma 反向的线性等价物；全亮度同比例放大，噪声与中间调被一起抬亮 |
| 软肩重建 `soft_shoulder` | 二分反演广义 Reinhard 曲线 `F(x)=x(1+(x/w)^n)/(1+x)` | 中间调近似 1:1 通过，仅高光沿软肩外推；SDR 裁切（y=1）无法唯一反演，映射到外推上界 |
| 局部内容保护 `local_protect` | 软肩增益 × 局部衰减，判据为局部平均亮度与局部对比度（高斯估计的标准差/均值） | 亮且平坦的漫反射内容（天空、人脸）收缩增益；高对比度局部亮点（镜面高光）保留外推；三通道共用亮度增益，严格保持色相 |

## 测试场景（均带线性光真值）

- `exposure_staircase`：9 级曝光阶梯 `2^-5 … 2^3`，区分可反演高光与 SDR 裁切段。
- `controlled_highlights`：天空、肤色人脸、漫反射白、小镜面高光、暗区。
- `hue_chart`：六个饱和原色块加肤色块，评估色相角与色度（chroma）变化。
- `noise_chart`：线性域注入已知高斯噪声的中间调块与高光块，评估噪声放大。

SDR 素材由正向相机模型生成：广义 Reinhard 压缩 + sRGB 编码域 8 位量化 +
解码回线性光，因此包含真实素材会有的 gamma 域量化误差。

## 评估指标

- 亮度：线性光 RMSE、`log10` 亮度 RMSE、高光段相对误差。
- 色相：圆周色相角误差（度）、色度比 `chroma_est/chroma_gt`。
- 噪声：亮度标准差及其相对真值的放大倍数。

## 运行

```bash
go run ./cmd/itm -out output
go test ./...
```

产物：

- `output/metrics_detail.csv`：逐场景、逐区域、逐策略的全部指标。
- `output/report.md`：策略总览与分区对比表。
- `output/png/`：真值 / SDR / 三种策略的可视化（统一峰值归一化，仅用于查看）。

参数（肩指数、外推上界、线性增益、局部保护窗口等）集中在
`internal/experiment/experiment.go` 的 `DefaultParams()`。
