# iccproof

Go 实现的 ICC 软打样与逐样本色差/边界报告工具。它把同一张照片或同一组色块分别用：

- `perceptual`（感知，ICC intent `0`）
- `relative-colorimetric`（相对色度，ICC intent `1`）
- `absolute-colorimetric`（绝对色度，ICC intent `3`）

转换到指定媒体/设备 profile，并为每个 `样本 × intent` 输出 CIEDE2000 色差、媒体色域警告、设备裁切通道和来源侧不可表示错误。

## 不可妥协的 profile 契约

`source_profile` 和 `media_profile` 都必须显式提供。可以是：

1. 磁盘上的 `.icc` / `.icm` 文件；
2. JSON 请求中的 profile 字节（API）；
3. 显式的 `descriptor` 基色/白点/gamma 描述；
4. 照片内嵌的 PNG `iCCP` 或 JPEG `ICC_PROFILE`（仅当文件中确实存在）。

如果对应 profile 缺失、路径不存在或字节不是 ICC，程序返回
`ErrProfileRequired` / `ErrProfileNotFound` / `ErrInvalidProfile`，**不会创建或回退到默认 sRGB**。
内置的 BT.709/sRGB 数学只出现在测试夹具或用户显式填写的 descriptor 中，不会由空配置自动生成。

## 快速开始

```bash
go test ./...
go build -o bin/iccproof ./cmd/iccproof

# 无外部 ICC 也可运行的显式 descriptor 示例
go run ./cmd/iccproof demo --format text
go run ./cmd/iccproof proof \
  --input examples/request.descriptor.json \
  --format json --out artifacts/demo-report.json

# 真实 ICC 软打样（需要 liblcms2-2 与开发头文件）
go run ./cmd/iccproof proof \
  --photo photo.jpg \
  --source /path/to/source-or-camera.icc \
  --media  /path/to/paper-or-printer.icc \
  --display /path/to/monitor.icc \
  --intent all \
  --format csv --out artifacts/samples.csv
```

`--intent` 也接受逗号分隔列表：

```bash
--intent perceptual
--intent relative-colorimetric,absolute-colorimetric
--intent 0,1,3
```

库调用如果没有设置 `Request.Intents`，会返回 `ErrInvalidIntent`；CLI 的
`--intent all` 会明确展开为题面要求的三种 intent。

## 边界分类

每条结果都有 `boundary.classification`，取值互斥：

| classification | 含义 | 是否计算 ΔE |
| --- | --- | --- |
| `ok` | 来源可表示，媒体色域内，无通道裁切 | 是 |
| `source_unrepresentable` | 输入码值超出源编码域，或 Lab/XYZ 参考色不在 source profile 色域内 | 给出外推 ΔE，但 `delta_e_reliable=false` |
| `device_out_of_gamut` | 来源合法，但目标媒体不能表示；当前 intent 做了压缩而未硬裁切 | 是 |
| `device_clipped` | 目标转换值越界并在量化/编码时被夹到 `0` 或满量程 | 是 |
| `error` | profile/引擎错误，错误文本在 `error` 字段 | 通常无 |

关键字段：

- `boundary.source_violations[]`：源通道、实际值、合法范围和超出量；
- `boundary.source_gamut_alarm`：Lab/XYZ 参考色是否落在 source profile 外；
- `boundary.media_in_gamut` / `media_gamut_alarm`：媒体色域边界；
- `boundary.media_violations[]`：固定用相对色度得到的未约束设备值越界详情；
- `boundary.clipped_channels[]`：转换前值、裁切后值、是否贴边；
- `delta_e` / `delta_e00`：同一个 CIEDE2000 色差；来源越界时仍给外推值，并用 `delta_e_reliable=false` 标记；汇总的均值/最大值只统计可靠样本；
- `intent_icc`：ICC intent 编号 `0`、`1` 或 `3`；
- `delta_e76`：可选的欧氏 Lab 距离；
- `media_values`：intent 映射后的归一化设备值（CMYK 已从 LCMS 百分比归一化）；
- `media_codes`：实际 8/16-bit 整数码值；
- `display_rgb` / `display_codes`：仅当显式提供 `display_profile` 时输出的显示器软打样模拟；
- `source_profile.sha256` / `media_profile.sha256`：报告中的 profile 溯源。

## 为什么 perceptual 仍报告 out-of-gamut

“这个源颜色超出媒体物理色域”与“当前 intent 是否硬裁切”是两件事。报告始终用独立的相对色度边界/gamut alarm 检查原始颜色是否在媒体色域外；perceptual 可以通过压缩/去饱和避免硬裁切，但该样本仍标记为 `media_out_of_gamut`。relative/absolute colorimetric 常见行为是贴边，因此额外标记为 `device_clipped`。

relative 与 absolute 的差异主要来自白点/纸白适应；当两个 profile 白点相同或 profile 的 intent 表给出相同映射时，数值可能相同。这是 CMM/profile 的真实结果，工具不会人为伪造差异。真实生产 ICC（尤其打印机/CMYK CLUT profile）通常会让三种 intent 在高饱和区域明显分化。

## JSON 请求格式

`source_profile` / `media_profile` 二选一使用 `path` 或显式 `descriptor`：

```json
{
  "photo_path": "photo.jpg",
  "intents": ["perceptual", "relative-colorimetric", "absolute-colorimetric"],
  "source_profile": {"path": "/icc/source.icc"},
  "media_profile": {"path": "/icc/media.icc"},
  "sample_grid": {"stride": 16, "max_samples": 4096},
  "config": {"output_bit_depth": 8},
  "samples": [
    {"id": "green-patch", "rgb": [0.0, 1.0, 0.0]},
    {"id": "measured-lab", "lab": {"L": 62.6, "a": 90.4, "b": 78.1}}
  ]
}
```

也可用 `code_rgb` + `bit_depth` 输入整数码值。照片输入支持 PNG/JPEG；默认最多取约 256 个网格样本，`--all-pixels` 才会逐像素输出。

CSV 每行是一个 `sample × intent`，可直接用 pandas/Excel 按 `intent` 分组比较：

```bash
go run ./cmd/iccproof proof --input req.json --format csv --out samples.csv
```

## Go API

```go
report, err := iccproof.Proof(iccproof.Request{
    Source:  iccproof.ProfileRef{Path: "/icc/source.icc"},
    Media:   iccproof.ProfileRef{Path: "/icc/media.icc"},
    Intents: iccproof.AllProofIntents(),
    Samples: []iccproof.Sample{{ID: "green", RGB: []float64{0, 1, 0}}},
})
if err != nil {
    // 缺少 profile 时在这里失败；不存在 iccproof.DefaultSRGBProfile 之类的回退。
    log.Fatal(err)
}
for _, row := range report.Results {
    fmt.Println(row.Intent, row.SampleID, row.Boundary.Classification, row.DeltaE00)
}
```

## ICC 后端

默认构建使用 cgo + Little CMS 2（`pkg-config: lcms2`）完成真实 ICC 变换：

- `cmsOpenProfileFromFile` / `cmsOpenProfileFromMem` 打开显式 profile；
- `cmsCreateTransform` 按所选 ICC intent 创建变换；
- `cmsCreateProofingTransform` + `cmsFLAGS_GAMUTCHECK` 检测媒体 gamut alarm；
- RGB 与 CMYK 输出，8/16-bit 量化和 Lab 回读；
- `cmsIsIntentSupported` 可由外部 profile 检查逻辑使用，报告保留 profile 标签信息。

如果使用 `CGO_ENABLED=0` 构建，库和 CLI 仍可编译，显式 `descriptor`
样本/测试引擎仍可运行；直接变换二进制 ICC 会返回
`ErrCGODisabled`，而不是偷偷换成 sRGB。

`descriptor` 参考引擎只实现常见 RGB matrix/shaper 路径和可解释的 gamut 压缩/裁切模型，适合测试、CI 和无 LCMS 环境；它不是完整 ICC CLUT/CMYK CMM。真实生产软打样请使用 cgo/Little CMS 和真实媒体 profile。

## 验证

```bash
go test -count=1 ./...
CGO_ENABLED=0 go test -count=1 ./...
go vet ./...
```

测试覆盖：

- 三种 intent 名称/别名/ICC 编号解析，空 intent 拒绝；
- 缺 source/media profile、路径不存在、垃圾 ICC 均失败；
- CIEDE2000 Sharma 标准样例；
- source 编码越界 vs 媒体色域外 vs 设备通道裁切；
- PNG/JPEG 像素采样和 PNG/JPEG 内嵌 ICC 提取；
- JSON/CSV/CLI 输出；
- 本机存在 Ghostscript ICC 时，自动运行真实 Little CMS RGB/CMYK profile 测试。
