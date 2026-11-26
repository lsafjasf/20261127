# observermap — CIE 2° ↔ 10° 标准观察者的实验式映射（Go）

> ⚠️ **实验项目 / EXPERIMENTAL**
> 本仓库给出的 2°↔10° 映射是**经验最小二乘近似**，**不是 CIE 或 ICC
> 标准转换**，也不存在唯一的“标准矩阵”。由于**同色异谱（observer
> metamerism）**，仅靠一组三刺激值无法唯一确定另一观察者下的三刺激值。
> 所有映射结果都带有 `empirical=true`、`standard_conversion=false`
> 与误差指标；缺 profile 或白点不受支持时显式标记 `UNCERTAIN`。

## 背景

- CIE 1931 **2°** 标准观察者与 CIE 1964 **10°** 补充标准观察者的色度匹配
  函数不同，同一光谱刺激在两者下的 `xy` 色度坐标和 L\*a\*b\* 色差可能差异明显
  （尤其在蓝紫、绿波段）。
- ICC profile 的头部与标签表**没有记录**制作/转换时使用了哪一种标准观察者
  （ICC.1 没有标准的 observer 字段）。因此只拿到 ICC 转换后的 XYZ/RGB 时，
  观察者来源不可追溯。

本工具做四件事：

1. 内嵌 CIE 1931 / CIE 1964 CMF 与 D50/D65 光源（5 nm，380–780 nm），
   对示例光谱**直接积分**出两种观察者的“真值”，展示真实差异。
2. 在确定性生成的合成平滑反射率总体上，拟合 `XYZ_10 ≈ M·XYZ_2` 的
   **实验式 3×3 矩阵**（并独立拟合反向 10°→2° 矩阵，D50/D65 各一套），
   用留出集给出 CIEDE2000 误差分布。
3. 对 sRGB 色板/任意 XYZ 应用该映射，列出 `xy` 坐标、`Δxy` 与
   CIEDE2000 色差。
4. 解析 ICC profile 的版本/类别/色彩空间/PCS/`wtpt` 白点标签，核对是否为
   支持的 D50/D65；**profile 缺失、无法读取、缺 `wtpt` 或白点不支持时，
   一律标记不确定**。

## 快速开始

需要 Go 1.22+（开发版本为 go1.27，仅用标准库，无第三方依赖）。

```bash
go test ./...                                   # 单元 + CLI 端到端测试
go run ./cmd/observermap demo                   # 终端表格
go run ./cmd/observermap demo --json            # 结构化 JSON
go run ./cmd/observermap report                  # 生成 observer-report.html（含 SVG 色度图）

# ICC 来源核查（仓库自带最小夹具，也可指向系统真实 profile）
go run ./cmd/observermap inspect testdata/monitor-d65.icc
go run ./cmd/observermap inspect /usr/share/color/icc/ghostscript/srgb.icc
go run ./cmd/observermap inspect testdata/illuminant-a-white.icc --strict

# 单点映射
go run ./cmd/observermap map --hex '#0000FF'
go run ./cmd/observermap map --x 18.044 --y 7.218 --z 95.03 --wp D65
go run ./cmd/observermap map --x 96.42 --y 100 --z 82.49 --wp D50 \
    --profile testdata/monitor-d50.icc --dir 2to10 --json
```

`examples/` 下有一份示例文本输出和一份自包含 HTML 报告，可直接打开。

## 命令与退出码

| 命令 | 作用 |
| --- | --- |
| `demo [--profile P] [--json]` | 光谱直接对比 + sRGB 色板经验映射 + 拟合质量 |
| `inspect <p.icc> [--json] [--strict]` | 解析 ICC 头/标签与 `wtpt`，给出来源与不确定判定 |
| `map --hex '#rrggbb'` 或 `--x/--y/--z` | 单点实验映射，支持 `--dir 2to10|10to2`、`--wp D65|D50`、`--profile` |
| `report [--out f.html] [--profile P]` | 生成含内联 SVG `xy` 色度图的 HTML 报告 |

退出码：`0` 正常；`1` profile 缺失/不可读或运行失败；`2` 参数错误；
`inspect`/`map` 加 `--strict` 时，若判定 `UNCERTAIN` 则以 `3` 退出，
便于脚本/CI 卡点。标志放在位置参数前后均可。

## 何时标记 `UNCERTAIN`

| 情形 | `uncertain` | 说明 |
| --- | --- | --- |
| 未提供 `--profile` | 是 | 观察者与介质白点均不可核实；白点退化为显式 `--wp` 假设 |
| 路径不存在/读取或解析失败 | 是 | 原因写入 `profile_error` 与 `reasons` |
| profile 无 `wtpt` 标签 | 是 | 无法核对介质白点（`white_point=UNKNOWN`） |
| `wtpt` 的 `xy` 不接近 D50/D65 | 是 | 标记 `UNSUPPORTED`，容差 `Δxy≤0.015`（D50↔D65 距离约 0.044） |
| `wtpt` 为 D50/D65 | 否（硬性来源不确定解除） | 但观察者未记录的告诫与“实验映射、非标准”标签始终存在 |

此外，ICC.1 本身不记录 2°/10° 观察者，因此 `observer_recorded` 恒为
`false`（代码仅保留对假想 `obsv` 标签的探测位）。所有经验映射值另有
`approximate/empirical` 标注和留出集误差，不与“来源确定”混为一谈。

## 方法与实测精度

- 积分：`XYZ = k·Σ S(λ)R(λ)cmf(λ)Δλ`，`k=100/ΣS·ȳ·Δλ`，
  5 nm 等间距；完全反射体 `Y=100`。积分白点与 CIE 公布值吻合到
  `±0.0003 xy`（见 `tables_test.go`）。
- 训练总体：固定随机种子的 512 条合成平滑反射率（随机高斯峰/吸收带 +
  logistic 边沿），另取 256 条独立样本做留出验证；D50/D65 分别拟合。
- 映射：法方程 + 部分主元消去解 3×3 最小二乘（带极轻 ridge）。
  当前 D65 正向矩阵约为

  ```text
  [  0.90021   0.07758   0.01311]
  [ -0.03798   0.97835   0.05366]
  [  0.06882  -0.09233   1.00695]
  ```

- 留出集对“同一光谱直接积分出的 10° 真值”的误差（确定性数值）：
  D65 正向 CIEDE2000 中位 ≈ **0.88**、p95 ≈ **4.4**、最大 ≈ **8.1**；
  反向与 D50 量级相近。这说明线性映射对多数平滑反射色可用，但对窄波段/
  强同色异谱样本误差可能超过观察者差异本身——报告中逐行给出
  `map residual ΔE00`。
- 色差：CIEDE2000 实现通过 Sharma–Wu–Dalal 标准测试向量前 6 组
  （`TestDeltaE2000SharmaVectors`）。跨观察者 ΔE 使用各自积分白点下的
  L\*a\*b\*，**仅作差异量级的展示指标，并非 CIE/ISO 标准色差用法**。

## 局限（请在引用结果前阅读）

- **不存在唯一的 2°↔10° 三刺激值映射**：映射依赖训练光谱总体与光源；
  本实现只用合成反射率，未覆盖真实材料/荧光/窄带光源，误差统计偏乐观或
  偏窄，不能外推为精度保证。
- 这不是色彩管理系统：不做完整 ICC 标签应用、PCS 编码、色域映射或
  色适应变换；`--profile` 只用于来源与白点核查。`--hex` 输入固定为
  sRGB/D65，D50 profile 不会改变其编码；原始 `--x/y/z` 才允许 profile
  白点覆盖 `--wp`。
- 支持的白点只有 D50 与 D65；其它（如 illuminant A、纸张白偏移）直接
  标记 `UNSUPPORTED/UNCERTAIN`，不会静默假设。

## 项目结构

```text
color.go       XYZ/xy/Lab、sRGB→XYZ、CIE76 与 CIEDE2000
tables.go      go:embed 的 CIE CMF / D50 / D65 表与白点积分
spectrum.go    光谱积分、解析示例光谱、确定性合成反射率总体
mapping.go     双向 3×3 经验最小二乘映射、求解与留出集误差
icc.go         ICC 头部/标签表/wtpt 的最小解析器
assessment.go  D50/D65 白点分类与 UNCERTAIN 判定
analyze.go     demo/map 的用例编排与 JSON 模型
render.go      终端表格/JSON 输出
html.go svg.go 自包含 HTML 报告与内联 xy 色度图
cmd/observermap/main.go  CLI（demo/inspect/map/report）
data/          内嵌 CSV 与来源说明（PROVENANCE.md）
testdata/      最小 ICC 夹具（D50/D65/illuminant-A 白点）
```

## 测试

```bash
go test -count=1 ./...
```

覆盖：数据维度与白点数值、sRGB 参考值、CIEDE2000 标准向量、拟合误差界、
ICC 夹具/损坏文件/系统 Ghostscript `srgb.icc` 解析、缺 profile 不确定标记、
以及编译真实二进制执行的 CLI 退出码与 JSON 契约。

## 参考

- CIE 15:2004, *Colorimetry*；CIE 1931 与 CIE 1964 标准色度观察者。
- Sharma, Wu, Dalal (2005), *The CIEDE2000 Color-Difference Formula*.
- ICC.1 规范：128 字节头、标签表、`wtpt` XYZType 与 s15Fixed16 编码
  （ICC 头偏移 36 处的 `acsp` 签名、偏移 68 的 PCS illuminant）。
- 数据镜像：`colour-science/colour`（BSD-3-Clause），详见
  `data/PROVENANCE.md`。
