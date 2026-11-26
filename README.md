# mp4seek

扫描非分片 MP4 的真实随机访问点（GOP 起点）、样本依赖和 seek 命中的文件偏移，报告坏 `stss` 造成的不可精确落点区间，并在不修改媒体 payload 的前提下修复样本索引。

实现只依赖 Go 标准库。

## 构建

```bash
go build -o mp4seek ./cmd/mp4seek
```

## 扫描

```bash
./mp4seek scan long-gop.mp4
./mp4seek scan -json long-gop.mp4 > scan.json
./mp4seek scan -samples long-gop.mp4
```

扫描内容：

- `GOP starts`：从 AVC/HEVC 码流识别 IDR、BLA、CRA 等真实随机访问点，同时给出样本号、展示时间、文件偏移以及是否出现在原 `stss` 中。
- `seek hit offsets`：列出 `stss` 和真实码流随机访问点最终命中的样本及文件偏移。`actual_rap=false` 表示原索引会把非关键帧当成关键帧。
- `broken index ranges`：旧索引漏标真实 GOP 起点、或误标非关键帧后，播放器仍可能返回“成功”的时间区间。
- `ranges that cannot land exactly`：长 GOP 内部无法直接落到随机访问点的展示时间区间。区间起点是关键帧，起点本身可精确落点；`(start, end)` 内只能回退或向前解码。
- `sample dependencies`：在 `-samples` 或 JSON 中给出每个样本的 NAL 类型、`independent` / `previous` / `pre-rap` 依赖关系。

JSON 主要字段：

- `gops`：真实 GOP 起点与命中偏移。
- `seek_hits`：`stss` / `bitstream` 两套命中结果。
- `broken_ranges`：坏索引影响的展示时间区间。
- `inexact_ranges`：自然 GOP 间隔内无法直接落关键帧的区间。
- `missing_sync_samples` / `false_sync_samples`：漏标的真实关键帧 / 误标的非关键帧。

## 随机 seek 检查

时间支持秒数、`500ms` 或 `00:01:02.500`：

```bash
./mp4seek seek long-gop.mp4 1.2 3.75 00:00:05.000
./mp4seek seek -random 20 -seed 7 long-gop.mp4
```

`false_success=true` 表示原索引给出了一个实际不是随机访问点的命中位置。

## 修复索引

```bash
./mp4seek repair broken.mp4 fixed.mp4
./mp4seek repair -json broken.mp4 fixed.mp4
```

修复规则：

- 只重写 `moov/trak/mdia/minf/stbl/stss`；`mdat` 和所有未修改 box 均按原始字节搬移。
- 若 `stss` 增删导致 `moov` 长度变化，只同步修正 `stco` / `co64` 的绝对 chunk 偏移，不触碰样本 payload。
- 写盘前重新解析输出文件，校验 `mdat` SHA-256、每个样本的字节内容和样本数。任一不一致都会失败且不覆盖目标文件。
- 码流扫描不完整、编码不受支持或找不到真实随机访问点时拒绝修复，避免凭猜测删改索引。

输出通过同目录临时文件原子替换，允许 `INPUT` 与 `OUTPUT` 相同。

## 支持范围

- 非分片 MP4（样本表在 `moov` 中）。
- AVC：`avc1` / `avc3`，识别 IDR。
- HEVC：`hvc1` / `hev1`，识别 BLA、IDR、CRA 和 RASL 依赖。
- 样本表支持 `stsz` / `stz2`、`stco` / `co64`、`ctts` 有符号/无符号版本。
- 展示时间应用首个非空 `elst` 编辑；多个非空 edit 会输出警告并采用首个 edit。
- 其他编码可以读取索引并报告，但无法从码流确认真实关键帧时不会自动修复。

## 验证

```bash
go test ./...
go vet ./...
```

测试覆盖坏 `stss`、缺失 `stss`、chunk 偏移随 `moov` 长度变化、`elst` 展示时间映射、误命中和 payload 完整性。
