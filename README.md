# aperturemap

用精确有理数拆分并校验相机有效像素、像素宽高比（PAR）、显示比例（DAR）和
ISO BMFF/QuickTime `clap` clean aperture 声明，输出从有效像素到播放器视口的
最终仿射几何映射。

## 四个独立事实

- `effective_pixels`：当前相机裁切/有效光栅的整数像素数 `W×H`。它只表示像素计数，
  不代表方形像素，也不代表最终显示比例。
- `pixel_aspect_ratio`：`pasp` 的 `hSpacing/vSpacing`，即单个像素的物理宽/高。
  缺省为 `1/1`。
- `display_aspect_ratio`：外部声明的最终 DAR；提供时必须与 clean aperture 几何
  推导出的比例精确相等。
- `clean_aperture`：`clap` 中的精确分数宽高和中心偏移，可能是分数像素，不能先
  四舍五入再校验。

核心关系：

```text
effective pixel AR = W / H
full aperture DAR  = W / H × PAR
clean pixel AR     = clapWidth / clapHeight
clean DAR          = clapWidth / clapHeight × PAR
```

## clap 坐标

像素中心位于整数坐标，整帧外缘是 `[-1/2, W-1/2] × [-1/2, H-1/2]`。按
ISO/IEC 14496-12：

```text
cx = horizontalOffset + (W - 1) / 2
cy = verticalOffset   + (H - 1) / 2

clean outer rect =
  [cx - clapWidth/2, cx + clapWidth/2]
  × [cy - clapHeight/2, cy + clapHeight/2]
```

例如 `1920×1088` 编码帧用 `1920×1080`、`vertical_offset=-4` 的 clap 去掉底部
8 行填充，得到的精确外缘是 `x=[-1/2,3839/2]`、`y=[-1/2,2159/2]`，映射到
`1920×1080` 视口时平移 `(+1/2,+1/2)`、缩放为 `1`。

## 四类组合

`CanonicalCombinations()` 构造以下四个用例，并由 `Resolve` 输出完整 JSON：

1. `effective-only-square`：`3840×2160`、`PAR=1`、无 clap，DAR 为 `16/9`。
2. `effective-par-anamorphic`：`1440×1080`、`PAR=4/3`，像素计数比是 `4/3`，
   PAR 修正后 DAR 为 `16/9`。
3. `effective-clap-padding`：`1920×1088` 中声明 `1920×1080` clap，偏移 `-4`，
   防止播放器把编码填充误当成画面。
4. `all-declarations-compound`：`1440×1080`、`PAR=4/3`、`1368×1026` clap 和
   显式 `DAR=16/9` 同时校验。

输出中的 `transform` 是从 `source_rect_in_effective_pixel_coordinates` 到
`target_rect_in_viewport_coordinates` 的最终轴对齐仿射：

```text
Xviewport = scale_x × Xeffective + translate_x
Yviewport = scale_y × Yeffective + translate_y
```

`mappings` 还拆分了三个阶段：aperture crop、PAR/归一化、viewport placement。

## 拉宽与再次裁切

同一个 `16:9` 源放入更宽的 `2000×1000` 视口时：

- `fit: "stretch"`：整幅映射到视口，`diagnosis.physical_stretch_factor=9/8`，
  即 PAR 修正后横向仍被拉宽 `12.5%`。
- `fit: "cover"`：保持均匀缩放并填满视口，上下溢出；诊断给出
  `crop_height_fraction=1/9`，即 clean aperture 高度的 `11.1%` 被播放器二次裁切。
- `fit: "exact"`（缺省）：要求比例完全相同；视口比例不等于 clean DAR 时返回
  `ErrPresentationImpossible`。
- `fit: "contain"`：保持全画面和均匀缩放，用黑边补足比例差异。

注意：非方形像素的原始 `scale_x != scale_y` 不代表拉伸。诊断会除以 PAR，
只有 `physical_stretch_factor != 1` 才是物理画面变形。

## 必须拒绝的情况

`Resolve` 不做静默近似；以下情况返回可由 `errors.Is` 判断的错误：

- 宽高、PAR、clap 宽高或视口宽高非正：`ErrInvalidGeometry`。
- 显式 DAR 与 `clapWidth/clapHeight × PAR` 不精确相等：
  `ErrInconsistentRatio`。
- clap 精确外缘超出有效像素整帧：`ErrApertureOutOfBounds`。
- `fit:"exact"` 下播放器视口比例与 clean DAR 不同：
  `ErrPresentationImpossible`。

所有比例均使用约分后的精确整数分数；不会用 `1.777...` 之类浮点值通过校验。

## 运行

```sh
go test ./...
go run ./cmd/aperturemap                 # 输出四类组合、播放器策略和拒绝样例
go run ./cmd/aperturemap -in examples/compound.json
go run ./cmd/aperturemap -in examples/conflict.json
```

也可以在 Go 代码中直接使用：

```go
dar := aperturemap.MustRatio(16, 9)
resolved, err := aperturemap.Resolve(aperturemap.Spec{
    EffectivePixels: aperturemap.Size{Width: 1440, Height: 1080},
    PixelAspectRatio: aperturemap.MustRatio(4, 3),
    DisplayAspectRatio: &dar,
    CleanAperture: &aperturemap.CleanAperture{
        Width: aperturemap.MustRatio(1368, 1),
        Height: aperturemap.MustRatio(1026, 1),
    },
})
```

`ParseCLAPPayload` / `CleanAperture.MarshalCLAPPayload` 和
`ParsePASPPayload` / `MarshalPASPPayload` 支持 32 位 ISOBMFF rational 载荷；
负的 clap offset 分子按有符号 `int32` 处理。
