# colorcompare

无第三方依赖的 Go 颜色差异分析器，用于比较两个把颜色转换为 **RGB24** 的转换器。它实现标准 CIEDE2000，并逐样本给出：

- RGB 立方体上的普通欧氏距离；
- Lab 空间中的 CIE76 欧氏距离；
- CIEDE2000 总色差；
- 亮度、色度（饱和度）和色相各自的归一化分量；
- 两种距离对样本的排名，以及排序相反的样本对。

RGB24 按 8 位 sRGB 解释，转换到 D65 白点的 CIE L\*a\*b\*。CIEDE2000 使用 `kL=kC=kH=1`，没有归一化、截断、放大或暗部补偿。

## 快速使用

```bash
go test ./...
go run ./cmd/colorcompare -demo
```

demo 会复现一个暗色小色差排序反转：暗蓝样本在 RGB 欧氏距离下更近，但 CIEDE2000 认为另一个亮黄小色差样本更近。

比较实际转换器输出时，把每个测试色作为一行 CSV：

```csv
id,converter_a,converter_b
dark-small,#0E0C25,#0E0C26
light-small,#DED376,#DFD378
near-gray,#808080,#808081
high-sat,#FF0000,#00FF00
```

```bash
go run ./cmd/colorcompare -input samples.csv
go run ./cmd/colorcompare -input samples.csv -json
```

也可以用 `-input -` 或省略 `-input` 从标准输入读取。CSV 的三列固定为 `id,converter_a,converter_b`，颜色支持 `#RRGGBB` 和 `RRGGBB`。

## 输出解释

文本输出包含：

| 列 | 含义 |
| --- | --- |
| `RGB-E` | RGB 三通道差值的普通欧氏距离，单位是 0..255 码值 |
| `Lab-E` | 转换到 Lab 后的 CIE76 欧氏距离 |
| `DE2000` | 标准 CIEDE2000 色差 |
| `RGBrank` / `DErank` | 样本按对应距离升序排列的名次，1 最近 |
| `dominant` | 绝对值最大的归一化分量：`lightness`、`chroma` 或 `hue` |
| `dL*` / `dC*` | CIEDE2000 修正后的 `ΔL'`、`ΔC'` |
| `dh(deg)` | 修正色相角差 |
| `Lterm` / `Cterm` / `Hterm` | 分别除以 `SL`、`SC`、`SH` 后的有符号项 |
| `rotation` | `RT × Cterm × Hterm` 交互项 |

CIEDE2000 的分量满足精确恒等式：

```text
DE2000² = Lterm² + Cterm² + Hterm² + rotation
```

因此分析器不会把旋转交互项强行分摊给某一个维度。`chroma` 是 CIEDE2000 使用的色度/彩度，不是 HSL 的百分比饱和度；在灰轴附近它对应很小的颜色偏移。

`Ordering reversals` 会对所有样本两两比较：如果 A 在 RGB 欧氏距离下比 B 近，但在 CIEDE2000 下比 B 远，就列出一条反转。这里直接使用原始距离值，不乘任何校正系数。

## API

```go
a := colorcompare.RGB24{R: 0x0E, G: 0x0C, B: 0x25}
b := colorcompare.RGB24{R: 0x0E, G: 0x0C, B: 0x26}

result := colorcompare.CompareRGB(a, b)
fmt.Println(result.EuclideanRGB, result.DeltaE2000)
fmt.Println(result.Components.DeltaL, result.Components.DeltaC, result.Components.DeltaHueAngle)

analysis, err := colorcompare.AnalyzeSamples([]colorcompare.Sample{
	{ID: "dark", A: a, B: b},
})
```

底层函数包括 `LabFromRGB`、`DeltaE2000`、`DeltaE2000Details`、`EuclideanRGB` 和 `EuclideanLab`。

## 验证

- 官方 Sharma/Wu/Dalal 34 组 CIEDE2000 测试向量，容差 `1e-4`；
- 每个向量同时检查交换两个颜色后结果不变；
- 接近灰轴的 1 码值偏移：色度项主导、结果有限；
- 高饱和红/绿比较：色相项主导；
- 黑、白亮度边界：结果有限，黑到白的 CIEDE2000 约为 100，没有额外放大；
- 分量平方和能精确重建总色差；
- CSV/JSON CLI 和暗色排序反转回归测试。

限制：比较仅覆盖 sRGB/D65 下的颜色数值，不模拟观看环境、屏幕亮度、色域映射或人眼适应。它适合定位两个转换器输出之间的数值和排序差异，不代表绝对视觉可接受度。
