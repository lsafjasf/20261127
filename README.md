# mkvidentity

纯 Go、零第三方依赖的 Matroska 轨道/章节身份与引用迁移工具。它不调用
`mkvmerge`、`ffmpeg` 或其他外部混流器；MKV 的读取、EBML 改写和索引位置
修正都在模块内完成。

## 为什么不能按行号重写

重封装后 `TrackEntry` 的顺序、`TrackNumber` 甚至脚本里的文本行都可能变化，
但 Matroska 已有稳定身份：

- 轨道：`TrackUID`（`0x73C5`），`TrackNumber` 只是当前编号。
- 版本：`EditionUID`（`0x45BC`）。
- 章节：`ChapterUID`（`0x73C4`）或 `ChapterStringUID`。
- 章节到轨道：`ChapterTrack/ChapterTrackUID`。
- 章节层级：嵌套 `ChapterAtom` / 父 `ChapterUID`。

本工具先把旧脚本中的行号解析成 UID，再建立 `old UID -> new UID` 映射，最后
重写 `ChapterTrackUID`、父章节/版本引用和默认轨归属。无法唯一匹配或无法
映射的时间点会进入报告，不会被静默当成第 `0` 行或时间 `0`。

## 快速开始

```sh
go test ./...
go build -o bin/mkvidentity ./cmd/mkvidentity

# 读取真实 MKV 或规范化 inventory JSON
bin/mkvidentity inspect movie.mkv --json
bin/mkvidentity validate movie.mkv --strict

# 从旧清单/旧行号规则迁移到重封装后的新清单
bin/mkvidentity migrate \
  --old examples/old-inventory.json \
  --new examples/new-inventory.json \
  --legacy-rules examples/legacy-plan.json \
  --timeline examples/timeline.json \
  --report /tmp/report.json \
  --patch /tmp/patch.json \
  --json

# 直接改写目标 MKV（复制媒体 payload，不外部混流）
bin/mkvidentity apply \
  --input remuxed.mkv \
  --patch /tmp/patch.json \
  --out remuxed.metadata.mkv
```

`migrate --out out.mkv` 也可以在一条命令内完成迁移和 EBML 改写。若章节时间
因剪辑区间无法映射，默认拒绝写出含无效起点的章节；确认要保留原始 EBML
时间时可加 `--keep-raw-times`，报告仍会保留 `unresolved_time_raw_kept`。

## 身份匹配顺序

轨道和章节分别按以下依据匹配，所有依据都会写入报告：

1. `--map-track OLD=NEW` / `--map-chapter OLD=NEW` 显式映射；
2. 相同 `TrackUID` / `ChapterUID`；
3. 相同 `ChapterStringUID`；
4. 唯一指纹：轨道使用 `TrackType + CodecID + CodecPrivate SHA-256 +
   声道/采样率`，不包含语言、名称、默认旗标或行号；
5. 仍无法唯一确定时标记 `ambiguous` 或 `missing`，要求人工 UID 映射。

旧行号只通过 `identity.ApplyLegacyPlan` 使用一次：若 `legacy_line` 不匹配，
才回退到 `TrackNumber` 并产生警告。迁移图的边始终是 UID 引用。

## 无法解析的时间点

`timepoint.Parse` 支持：

- 秒：`75.25`
- `MM:SS[.frac]`
- `HH:MM:SS[.frac]`，小数点或逗号均可
- `HH:MM:SS:FF`：必须通过 `--fps 24000/1001` 等提供精确帧率

以下情况会形成独立 issue，而不是返回零值：

- 空值、格式错误、负数、组件越界；
- SMPTE 帧码缺少帧率或帧号超出帧率；
- 时间落在剪辑删除区间内部；
- 映射后为负或超出目标时长；
- MKV 中缺少必需的 `ChapterTimeStart`。

剪辑映射把 `[start,end)` 视为删除区间；`start` 和 `end` 两个边界解析到同一
接合点，区间内部标记 `time_removed_by_edit`，区间后的时间自动减去已删除
时长，也可再加全局 `offset_ns`。

## 报告结构

JSON 报告的关键字段：

- `tracks[].basis`、`tracks[].old_ref/new_ref`：身份匹配依据；
- `chapters[].refs[]`：每条章节-轨道/父章节引用的旧 UID、新 UID、状态；
- `unresolved_refs[]`：悬空或无法重写的 UID 引用；
- `unresolved_times[]`：无法解析/映射的章节时间点；
- `patch.track_patches[]`：目标轨道语言/IETF/默认旗标修改；
- `patch.track_ref_map`：可原地修复目标已有章节中的 `ChapterTrackUID`；
- `patch.chapters[]`：引用已重映射、可编码回 Matroska 的章节。

## 直接 MKV 改写的安全边界

`matroska.RewriteFile` 流式扫描 Segment 顶层元素，媒体 Cluster 和附件按原
字节复制。修改 `Tracks`/`Chapters` 后会重新计算 Segment 布局，并：

- 若目标已有章节，按 `track_ref_map` 原地重写 `ChapterTrackUID`；
- 仅在目标缺章节或要求复制时，用 `patch.chapters` 整体重建 Chapters；
- 将 `SeekHead/Seek/SeekPosition` 改成新的 Segment 相对位置；
- 将 `Cues/CuePoint/CueTrackPositions/CueClusterPosition` 匹配到原 Cluster
  起点后更新为新位置；
- 对位置字段使用固定 8 字节 uint，避免“填值导致长度变化、长度变化又改变
  位置”的循环；
- 找不到索引目标时写 warning，不伪造偏移。

合成 MKV 回读测试覆盖了 Tracks、Chapters、SeekHead、Cues 和 Cluster；真实
ffmpeg 生成文件也已手动验证可回读和解码。

## 主要包

- `timepoint`：宽容时间解析、帧率换算、剪辑/偏移时间轴。
- `identity`：身份、引用图、旧行号桥接、迁移报告和补丁模型。
- `ebml`：EBML VINT、Header、节点树和最小编解码。
- `matroska`：Matroska ID、元数据扫描、节点编码和流式改写。
- `cmd/mkvidentity`：`inspect`、`validate`、`migrate`、`apply`。

## 限制

当前聚焦归档元数据身份，不重新编码或重建媒体样本；未知 EBML 容器在元数据
节点树中按原始 payload 保留。分片/未知尺寸 Segment 的极端尾部布局、DVD 菜单
命令和 Tags 的完整目标 UID 改写尚未纳入 CLI；核心图模型已经能表达对应 UID
引用，可在 `identity.Edge` 上继续扩展。
