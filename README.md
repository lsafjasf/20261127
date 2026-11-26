# colorfix

针对 720×576 存档视频的色度矩阵核查与保守重标工具。它不会依据“SD 视频通常是 601”就批量改动，而是联合检查：

1. 画面中是否存在可识别的 75%/100% 七条彩条；
2. 同一组彩条像素分别按 BT.601、BT.709 解码后，与标准 RGB 模板的 RMS 残差；
3. 容器/码流声明的矩阵系数。

只有残差差距明确时才写入修正标签；接近或缺少彩条时保留候选，不生成强制修正版。

## 快速运行

```bash
go run ./cmd/colorfix -out colorfix-out archive/
```

也可以传多个文件或目录。默认输出到 `colorfix-out/`，先密集扫描片头 12 秒；片头没有彩条时再全片抽样。

常用参数：

```text
-open-seconds 12       片头密集扫描时长
-open-fps 2            片头抽样帧率
-fallback-count 16     未找到彩条时的全片抽样数
-skip-relabel          只分析、只出对照帧，不写修正媒体
-max-bar-residual .075 可信彩条拟合的最大 RMS 残差
-min-residual-margin .015 唯一判定所需的最小绝对残差差
-min-residual-ratio 1.60  唯一判定所需的败者/胜者残差比
```

## 判定结果

- `verified`：彩条胜者与声明一致，无需重标。
- `relabeled`：彩条给出唯一胜者且与声明冲突，输出修正媒体。
- `labeled`：原文件没有可用矩阵声明，彩条给出唯一胜者。
- `ambiguous`：彩条存在，但两种矩阵残差过近；两个候选都保留，不写修正媒体。
- `insufficient_evidence`：没有可信彩条；不写修正媒体。

默认只改矩阵系数：625 线目标写为 `bt470bg`（FFmpeg/H.264 系数值 `5`）。原文件的 primaries、transfer、range 保持不变，避免把色条无法证明的信息一起改写。H.264、HEVC、MPEG-2 会同时改写码流 VUI 和容器标签；其他编码会尝试改写容器标签，`bitstream_updated` 标明是否改了码流。

## 输出

```text
colorfix-out/
  report.json              完整证据、采样 RGB、候选和输出路径
  report.md                可读摘要
  index.html               带图对照页
  <id>/
    label.json             修复后的标签或并列候选
    fixed-bt601.mp4        仅在唯一结论时生成
    bars-bt709.png         彩条帧按 BT.709 解码
    bars-bt601.png         彩条帧按 BT.601 解码
    bars-compare.png       左 709 / 右 601
    content-bt709.png      内容帧按 BT.709 解码
    content-bt601.png      内容帧按 BT.601 解码
    content-compare.png    左 709 / 右 601
```

`ambiguous` 的 `label.json` 中 `recommended` 为空，`candidates` 同时列出 `bt601` 与 `bt709`。

## 演示样片

仓库没有用户原始视频，因此附带一个 Go 生成的已知真值样片：YUV 按 BT.601 编码，H.264/容器却声明 BT.709。

```bash
go run ./cmd/colorfixture -out demo/mislabeled-601-as-709.mp4
go run ./cmd/colorfix -out demo/output demo/mislabeled-601-as-709.mp4
```

另有一个位于 601/709 之间的混合样片，用于验证“不强制更正”：

```bash
go run ./cmd/colorfixture -blend 0.5 -out demo/ambiguous-601-709.mp4
go run ./cmd/colorfix -out demo/ambiguous-output demo/ambiguous-601-709.mp4
```

已有演示产物位于 `demo/output/` 与 `demo/ambiguous-output/`，可直接打开各自的 `index.html`。

## 验证

```bash
go test ./...
go vet ./...
```

端到端测试会用 FFmpeg 生成真实 720×576 H.264 文件，并检查：601 残差更低、修正文件回读为 `bt470bg`、对照 PNG 为 1450×602、混合样本保留两个候选且没有修正媒体。

## 限制

- 面向 YUV 编码的存档视频；RGB 编码文件不存在同一类 YUV 矩阵误标问题。
- 彩条需为标准七条、75% 或 100% 模板；非标准测试图会落入“证据不足”。
- 默认目标为 720×576/PAL 的 `bt470bg`。720×480/NTSC 如要写 `smpte170m`，需要改目标映射，当前不自动推断制式。
- 抽帧后统一转为 8-bit YUV444 分析；高位深源可用，但残差精度按 8-bit 计算。
