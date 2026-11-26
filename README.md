# effectinterp

一个仅使用 Go 标准库的效果参数插值库。预览和离线渲染共用同一个
`Scene.Sample` 求值入口，避免两条渲染路径产生不同帧。

## 快速开始

```go
package main

import (
	"fmt"
	"time"

	"effectinterp"
)

func main() {
	opacity, err := effectinterp.NewTrack(effectinterp.KindNumber, []effectinterp.Keyframe{
		{Time: 0, Value: effectinterp.NumberValue(0)},
		{Time: 2 * time.Second, Value: effectinterp.NumberValue(1)},
	})
	if err != nil {
		panic(err)
	}

	angle, err := effectinterp.NewTrack(effectinterp.KindAngle, []effectinterp.Keyframe{
		{Time: 0, Value: effectinterp.AngleValue(350)},
		{Time: 2 * time.Second, Value: effectinterp.AngleValue(370)},
	})
	if err != nil {
		panic(err)
	}

	scene, err := effectinterp.NewScene(map[string]effectinterp.Track{
		"opacity": opacity,
		"angle":   angle,
	})
	if err != nil {
		panic(err)
	}

	preview, err := scene.PreviewFrame(time.Second)
	if err != nil {
		panic(err)
	}
	offline, err := scene.RenderFrame(time.Second)
	if err != nil {
		panic(err)
	}

	fmt.Printf("preview: %#v\n", preview)
	fmt.Printf("offline: %#v\n", offline)
}
```

在示例中点处，透明度为 `0.5`，角度从 `350°` 经 `360°/0°` 继续到
`10°`，因此采样结果是 `0°`，而不是反向穿过 `180°`。

## 插值规则

| 类型 | 构造值 | 规则 |
| --- | --- | --- |
| 数值 | `NumberValue(float64)` | 首尾线性插值，`from + (to-from)*p`。 |
| 角度 | `AngleValue(degrees)` | 先把端点归一化到 `[0, 360)`，再沿最短圆弧插值。 |
| 布尔 | `BooleanValue(bool)` | 步进；在右关键帧时刻切换，不做混合。 |
| 颜色 | `ColorValue(Color{...})` 或 `RGBA(...)` | 在 sRGB 空间逐通道插值 R/G/B/A，四舍五入到最近的 8 位整数，恰好 `.5` 向上。 |

`Value.Number` 同时承载数值和角度；`Value.Boolean` 与 `Value.Color`
分别承载其余两类。`NewTrack` 会校验值类型与参数类型一致，并拒绝
`NaN`、正负无穷等非有限数值。

## 角度与 360°

- 角度端点和采样结果都归一化到 `[0, 360)`；`360°` 与 `0°`、
  `720°` 与 `0°` 等价。
- `350°` 到 `10°` 的差值为 `+20°`，中点经过 `0°`，不会反向经过
  `180°`。
- 两方向距离相等（相差 `180°`）时选择角度增加的方向。因此 `0°` 到
  `180°` 的中点是 `90°`，`180°` 到 `0°` 的中点是 `270°`。
- 如果需要保留多圈旋转（例如 `0°` 到 `720°` 转两圈），应使用数值
  插值而不是角度插值。

## 时间与重复关键帧

- 关键帧按时间稳定排序；输入切片不会被修改。
- 同一时间出现多条关键帧时，**最后提供的一条生效**。重复项会先折叠，
  因此查询该时刻及之后的时间只使用最后一条的值。
- 查询时刻早于首关键帧时保持首值；晚于尾关键帧时保持尾值。
- 时间使用 `time.Duration`，重复时间判断是精确的整数比较。

## 预览与离线一致性

`PreviewFrame(t)` 和 `RenderFrame(t)` 都直接调用 `Scene.Sample(t)`。
`RenderRange(start, end, step)` 也逐帧调用同一入口，且两端均包含。
因此对同一时刻调用预览或离线接口，会得到完全相同的 `Frame`。

## 验证

```sh
go test ./...
go vet ./...
```
