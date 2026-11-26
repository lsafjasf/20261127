# alphacheck — WebM Alpha / 裁剪 / 帧时间核对

纯 Go 标准库实现的 WebM(EBML/Matroska)检查器。它不会因为 `CodecID=V_VP8/V_VP9`
就认定文件支持或包含 Alpha，而是同时核对：

- `TrackEntry/Video/AlphaMode`；
- `BlockGroup/BlockAdditions/BlockMore/BlockAddID=1/BlockAdditional` 中的 Alpha 载荷；
- `PixelWidth/PixelHeight`、`PixelCropLeft/Right/Top/Bottom` 与裁剪后可见尺寸；
- VP8/VP9 keyframe header 中的实际编码尺寸；
- `TimecodeScale`、Cluster Timecode、Block relative timecode、`DefaultDuration`；
- PTS 倒退、重复、缺口、间隔抖动以及 Info.Duration 一致性。

工作区最初没有指定的真实 WebM 附件，因此 `artifacts/demo/` 中提供了一套可复现样本、
JSON 报告和黑白底预览。所有 Go 代码只导入标准库，没有 `go.sum` 或第三方 Go 依赖。

## 快速开始

```bash
go test ./...
go build -o alphacheck ./cmd/alphacheck

./alphacheck inspect artifacts/demo/webm/alpha-ok.webm
./alphacheck inspect -json -all-frames input.webm > report.json
```

文本输出会给出轨信息、AlphaMode、编码尺寸、裁剪尺寸、帧时间汇总和按层分类的问题。

## 生成黑白底预览

Go 标准库不含 VP8/VP9 解码器，因此结构解析、IVF 导出、RGBA/Gray 合成、指标和 PNG
编码均由 Go 完成；演示中的 VPX 像素解码使用系统 `ffmpeg`。已提交的 PNG 是直接证据，
重新生成完整证据目录可运行：

```bash
./scripts/make_demo.sh
```

手动导出某一帧：

```bash
./alphacheck ivf input.webm /tmp/frame
ffmpeg -i /tmp/frame.color.ivf -frames:v 1 -f rawvideo -pix_fmt rgba /tmp/color.rgba
ffmpeg -i /tmp/frame.alpha.ivf -frames:v 1 -f rawvideo -pix_fmt gray /tmp/alpha.gray

./alphacheck composite \
  --color /tmp/color.rgba --alpha /tmp/alpha.gray \
  --width 96 --height 64 --crop 8,8,0,0 --require-variation \
  --out /tmp/preview > /tmp/preview.metrics.json

./alphacheck inspect \
  --preview-metrics /tmp/preview.metrics.json --require-variation \
  input.webm
```

这会写出 `/tmp/preview.black.png`、`/tmp/preview.white.png` 和指标。透明区域在黑底上
呈黑色、白底上呈白色；半透明区域会向各自背景混合。若解码出的 Alpha 全为 255，两张证明图
相同且 `black_white_mean_diff=0`，说明元数据虽合法，透明效果并未被编码/导出。

## 归因口径

| 分类 | 层级 | 典型条件 |
| --- | --- | --- |
| `muxing_error` | 合成/封装合同 | `AlphaMode` 与 `BlockAdditional` 不一致、裁剪越界、容器尺寸与 VPX 帧头不符、PTS 倒退/缺口/重复 |
| `encoding_error` | 编码/码流/导出 | VP8/VP9 帧头损坏、Alpha 帧头尺寸不符、Alpha 无法解码、解码后 Alpha 平面恒为 255/0 |
| `composition_error` | 画面合成 | Alpha 平面有效，但 straight/premultiplied 约定错误，白/黑底边缘出现异常暗边或亮边 |
| `composition_ok` / 预览评估 | 黑白底合成 | Alpha 有变化且黑白底按预期分离 |

Codec 名称只用于选择帧头解析器，不能作为 Alpha 存在的证据。

## 演示证据矩阵

总览图：`artifacts/demo/previews/contact-sheet.png`。上排是正确 Alpha：同一半透明橙色块在
黑底上偏棕、白底上偏桃色；下排是“Alpha 轨存在但解码值全为 255”，所谓白底证明仍为黑色。

| 样本 | 核对到的事实 | 结论 | 证据 |
| --- | --- | --- | --- |
| `artifacts/demo/webm/alpha-ok.webm` | `AlphaMode=1`，96×64，crop=0，12/12 帧有 AddID=1 Alpha，12 fps，平均间隔 83.364 ms | PASS | `artifacts/demo/reports/alpha-ok.json`；`artifacts/demo/previews/alpha-ok.black.png`；`artifacts/demo/previews/alpha-ok.white.png` |
| `artifacts/demo/webm/mux-alpha-mode-zero.webm` | 12 个 Alpha `BlockAdditional` 仍在，但 `AlphaMode=0` | `muxing_error: alpha_payload_without_mode` | `artifacts/demo/reports/mux-alpha-mode-zero.json` |
| `artifacts/demo/webm/mux-crop-valid.webm` | `CropLeft=8/CropRight=8`，编码 96×64，可见 80×64 | PASS | `artifacts/demo/reports/mux-crop-valid.json`；`artifacts/demo/previews/alpha-ok.black.cropped.png` |
| `artifacts/demo/webm/mux-crop-invalid.webm` | L+R=100 ≥ `PixelWidth=96` | `muxing_error: invalid_horizontal_crop` | `artifacts/demo/reports/mux-crop-invalid.json` |
| `artifacts/demo/webm/mux-pts-gap.webm` | 第 3 帧间隔 334 ms，为默认帧时长的 4.01×，约缺 3 个帧槽 | `muxing_error: pts_gap` | `artifacts/demo/reports/mux-pts-gap.json` |
| 双重预乘合成证明 | Alpha 本身仍为 0–255，但错误地把预乘色再乘一次；白底下边缘亮度残差约 `-40.73` | `composition_error: dark_white_halo_double_premultiplication` | `artifacts/demo/previews/composition-comparison.png`；`artifacts/demo/previews/composition-double-alpha.metrics.json` |
| `artifacts/demo/webm/encode-alpha-opaque.webm` | 容器层 PASS，但 Alpha min/max/mean=255/255/255，黑白底平均差 0 | `encoding_error: alpha_constant_opaque`（预览评估） | `artifacts/demo/previews/encode-alpha-opaque.metrics.json`；黑白 PNG 完全同形 |
| `artifacts/demo/webm/encode-corrupt-alpha-header.webm` | 容器和 PTS 正常；Alpha VP9 sync code 被破坏为 `b68342` | `encoding_error: bad_alpha_header` | `artifacts/demo/reports/encode-corrupt-alpha-header.json`；`artifacts/demo/previews/encode-corrupt-alpha.decode-error.log` |

正确样本的预览指标为 Alpha 0–255、4024 个全透明像素、1160 个不透明像素、960 个半透明像素，
黑白底平均通道差约 `184.04`；全不透明样本的对应差值为 `0`。这些数值来自实际导出的 VP9/Alpha
IVF 帧，而不是只读取文件扩展名或 `CodecID`。

## 目录

- `ebml/`：EBML VINT、元素树和 WebM ID。
- `webm/`：Track、AlphaMode、crop、Block/PTS、VP8/VP9 帧头和归因规则。
- `preview/`：straight-alpha 黑白底合成、统计和预览评估。
- `cmd/alphacheck/`：`inspect`、`ivf`、`composite` CLI。
- `cmd/mkfixtures/`：生成透明 PNG 帧和确定性的元数据/码流缺陷样本。
- `scripts/make_demo.sh`：重建 `artifacts/demo/` 的一键脚本。

## 规范依据

- WebM Alpha Channel：<https://wiki.webmproject.org/alpha-channel>
- WebM Container Guidelines：<https://www.webmproject.org/docs/container/>
- CELLAR Codec Bindings（`V_VP8` 的 `BlockAddID=1` Alpha 约定）：<https://datatracker.ietf.org/doc/html/draft-ietf-cellar-codec-15>

Repository initialized for 20261127.
