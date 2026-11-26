# rangecheck — Limited Range 逐帧取证器

只使用 Go 标准库实现核心统计、连通区域、跨帧轨迹和分类；不依赖第三方 Go 包。

当前仓库没有附带待分析的交付片，因此不能诚实地给出“某一具体视频”的时间码结论。把交付片路径传给下面的命令，即可得到逐帧 CSV/JSON、异常段、轨迹、画面矩形和 overlay PNG。

## 检查什么

- 合法范围：8-bit limited/TV range 下统计 `Y: 16–235`、`Cb/Cr: 16–240`；同时记录 `0/255` 锚点、近合法墙、均值和极值。
- 逐帧证据：每一帧都有 below/within/above 计数与比例，不能用整片峰值掩盖短段异常。
- 画面区域：对 `Y<16` 和 `Y>235` 做连通域、字形邻近聚合、外接矩形和局部对比统计。
- 像素来源：区分下区烧录字幕候选、普通图形/UI 文本、高光候选、压缩边缘过冲、大面积风格化影调以及全局范围标签/转换异常。
- 跨帧定位：生成 track、连续异常 segment、代表帧和 `x,y,w,h`；可用 overlay PNG 直接回看画面区域。
- 帧率：读取 `r_frame_rate`、`avg_frame_rate` 和逐帧 PTS，计算中位帧率、平均帧率、帧间隔和 VFR 分数。

## 构建与测试

```bash
go test -count=1 ./...
go build -o bin/rangecheck ./cmd/rangecheck
go run . -h
```

## 分析 MP4/MOV/MKV

视频适配会调用本机 `ffmpeg` 仅将目标轨解码成 `yuv444p` raw stream，并调用 `ffprobe` 读取元数据/PTS；所有判定逻辑都在 Go 包内。建议直接分析原始 Y 码值，不要只看播放器转换后的 RGB 截图，因为超黑/超白在 RGB 转换后可能已被裁切。

```bash
go run ./cmd/rangecheck \
  -input delivery.mp4 \
  -json artifacts/report.json \
  -csv artifacts/frames.csv \
  -markdown artifacts/report.md \
  -overlay-dir artifacts/overlays
```

如果 `ffprobe` 不可用，但知道帧率和尺寸，可显式传入：

```bash
go run ./cmd/rangecheck -input delivery.mp4 \
  -width 1920 -height 1080 -fps 30000/1001
```

## 分析 PNG/JPEG 序列或 Raw YUV

```bash
go run ./cmd/rangecheck -input ./frames -fps 24000/1001 -overlay-dir ./overlays

go run ./cmd/rangecheck \
  -input delivery.yuv -kind raw \
  -width 1920 -height 1080 -pix-fmt yuv420p -fps 25 \
  -csv frames.csv -json report.json
```

PNG/JPEG 通常已经是 RGB；工具会按 `-matrix bt601|bt709|bt2020` 派生 limited Y'。若要做法证级码值判断，优先使用 raw YUV 或工具的 ffmpeg YUV 管道。

## 可复现合成样例

生成一段含正常段、超黑字幕、超白高光和合法风格化墙的 YUV420P：

```bash
go run ./cmd/rangecheck-demo -out /tmp/rangecheck-demo.yuv -frames 36 -width 320 -height 180
go run ./cmd/rangecheck \
  -input /tmp/rangecheck-demo.yuv -kind raw \
  -width 320 -height 180 -pix-fmt yuv420p -fps 30 \
  -json /tmp/rangecheck.json -csv /tmp/frames.csv \
  -markdown /tmp/rangecheck.md -overlay-dir /tmp/overlays
```

## 如何理解分类

- `burned_subtitle_candidate`：超黑像素组成多字形水平行框，且位于下方字幕安全区；跨帧稳定会提高置信度。典型成因是 RGB 黑 `(0,0,0)` 被直接写入 Y=0，而不是 limited black Y=16。
- `scene_highlight_candidate`：超白像素是与邻近高光相连的团块。它可能是素材高光余量，也可能是未合法化的全范围转换；需要母带/色彩元数据做最终责任判定。
- `codec_edge_overshoot`：高对比边缘附近的细窄、浅度过冲，常见于压缩 ringing 或反复编码。
- `stylized_grade_candidate`：大面积连续影调区域更像压黑/提白风格。即使意图成立，`Y<16/Y>235` 对声明为 limited range 的交付仍可能不合规。
- `style_intervals`：像素压在合法墙 `Y=16–18` 或 `Y=233–235`，但没有明显越界；这是故意风格化/硬裁切的证据，不应与编码错误混为一谈。

自动结论是取证线索而非最终司法结论。最终仍应核对源工程、字幕/图形层、H.264 VUI、容器 `colr` 和播放器色彩管理。
