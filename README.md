# IMF Playlist Merge / IMF 最终播放清单构建器

主包（main IMP）与增补包（supplemental IMP）各自只含部分虚拟轨道时，直接做
文件级覆盖合并常常会丢掉增补包里的字幕引用。本工具用 Go 实现，按 IMF 的
资源继承链

```
OPL → CPL → Segment / Sequence(Track) → TrackFileResource(TrackFileId)
     → PKL(Hash/Size) → AssetMap(UUID → Chunk Path) → 资产文件
```

把多个包视为一个有序的资源共同体：后载入的增补包叠加/补全主包轨道，所有
`TrackFileId` 跨全部包的 AssetMap/PKL 解析，最终输出合并后的播放清单，并
报告**缺失资产**与**重复资产**。工具对原包目录只读，结果必须写入独立目录。

## 快速开始

需要 Go 1.22+（开发环境为 Go 1.27，无第三方依赖）。

```bash
# 构建
go build -o imfplaylist ./cmd/imfplaylist

# 用内置示例（主包画面+声音，增补包字幕）
./imfplaylist \
  -main testdata/main-imp \
  -supp testdata/supp-imp \
  -out demo-output -copy-assets

# 也可直接 go run；不带任何包参数时默认跑 testdata 下的两个示例包
go run ./cmd/imfplaylist

# 跑测试
go test -count=1 ./...
```

包目录也可以用位置参数给出，flag 可放在位置参数前后：

```bash
./imfplaylist path/to/main path/to/supp1 path/to/supp2 -out /tmp/final-imp
```

## 命令行参数

| 参数 | 说明 |
| --- | --- |
| `-main <dir>` | IMF 主包目录（含 `ASSETMAP`/PKL/CPL/OPL） |
| `-supp <dir>` | 增补包目录，可重复指定 |
| `-pkg <dir>` | 任意包目录，可重复；顺序即继承叠加顺序 |
| 位置参数 | 等价于按顺序给出的包目录 |
| `-out <dir>` | 输出目录，默认 `imf-output`；不允许等于或位于任一输入包内 |
| `-cpl <uuid>` | 显式指定作为合并基底的 CPL |
| `-opl <uuid>` | 显式指定要跟随的 OPL |
| `-copy-assets` | 把可解析的资产文件复制到 `<out>/assets/` |
| `-strict` | 存在缺失资产或冲突重复时退出码为 `3` |
| `-quiet` | 不向 stdout 打印完整文本报告 |

退出码：`0` 正常（默认即使有缺失/重复也返回，发现项写在报告里）；
`2` 参数或加载/写入错误；`3` `-strict` 下存在阻断性发现。

## 输出产物

写入 `-out` 指定的**新目录**：

- `playlist.json` — 最终播放清单：合并后的 Segment/Track/Resource、每个
  `TrackFileId` 的解析结果（来自哪个包、磁盘路径、是否存在）、OPL→CPL→资产
  的继承链、来源 CPL 记录。
- `CPL_final.xml` — 合并后重新生成的 CPL（保留 `MainImageSequence`、
  `MainAudioSequence`、`MainSubtitleSequence` 等原始序列名，字幕轨道不丢失）。
- `report.json` — 机器可读的缺失/重复报告与计数。
- `report.txt` — 中文核对报告（最终轨道清单 + 缺失 + 重复 + 警告）。
- `assets/` — 指定 `-copy-assets` 时复制出的可解析资产。

## 合并与继承规则

1. **发现文档**：扫描包目录内 `*.xml` 以及无扩展名的 `ASSETMAP`，按根元素
   本地名识别 `AssetMap` / `PackingList` / `CompositionPlaylist` /
   `OutputProfileList`；命名空间和前缀（`cc:`、`pkl:`、`assetmap:`）不敏感，
   兼容 ST 2067-3 的 2013/2016 写法（`<TrackFileResource>` 与
   `<Resource xsi:type="TrackFileResourceType">` 都支持）。
2. **选择根**：优先 `-opl/-cpl`；否则跟随“引用了存在 CPL”的 OPL；再否则用
   第一个包（主包）的第一个 CPL 作为基底。
3. **合并轨道**：相同 `Segment Id`（或双方都只有单个 Segment 时）合并；轨道
   以 `TrackId`（无则序列元素名）为键做并集。相同资源（按资源 `Id`，或
   `TrackFileId+入点+时长` 元组）去重，因此增补 CPL 里原样继承的轨道不会
   重复出现；新类型轨道（如字幕）直接补入。
4. **跨包解析**：`TrackFileId` 在所有包的 AssetMap/PKL 聚合注册表中解析，
   优先选择磁盘上真实存在的副本。缺少 ASSETMAP 时，还用文件名中的 UUID
   （如 `<TrackFileId>.mxf`）做兜底索引。

## 报告的问题类型

缺失类（`missing[].kind`）：

- `unresolved_reference` — CPL 的 `TrackFileId` 在所有包的 AssetMap/PKL/
  文件名索引中都不存在（典型的“字幕引用丢失”）。
- `file_not_found` — AssetMap 映射了路径，但资产字节不在磁盘上。
- `cpl_not_found` — OPL 引用的 CPL 在任何包中都不存在。

重复类（`duplicates[].kind`，带 `severity`）：

- `duplicate_asset_uuid` — 同一资产 UUID 出现在多个包中。所有 PKL
  hash/size 一致时为 `identical`（主包/增补包之间合法的资产复用继承）；
  hash 或 size 不一致时为 `conflict`（同一 UUID 内容不同，必须处理）。
- `duplicate_assetmap_id` / `duplicate_pkl_id` — 同一清单内 UUID 重复。
- `duplicate_path` — 不同 UUID 映射到同一文件路径。
- `duplicate_content_hash` — 不同 UUID 声明了相同 PKL 哈希。

另有 `warnings`（如 PKL 有条目但本包 AssetMap 未映射，跨包继承可能仍可
解析）。

## 内置示例与复现的事故

`testdata/main-imp`：主 CPL 只有画面与声音；含视频/音频/Logo 资产，以及一个
只在清单中存在、文件未交付的 `44444444-…`。OPL 指向主 CPL。

`testdata/supp-imp`：增补 CPL 只有 `MainSubtitleSequence`，其中：

- 字幕资源 `55555555-…` 在增补包内真实存在 → 合并后字幕轨道完整可播放；
- 字幕资源 `66666666-…` 两个包都未交付 → 报告为 `unresolved_reference`；
- 音频 `22222222-…` 在两个包中重复且 hash/size 一致 → `identical` 重复；
- Logo `33333333-…` 在两个包中 hash/size 不一致 → `conflict` 重复。

只解析主包时播放清单里没有任何字幕轨道（见
`TestMainOnlyPlaylistHasNoSubtitle`）；按继承关系合并后得到 3 条轨道、
4 个资源，其中 3 个可解析、1 个缺失，另有 1 个文件缺失、1 个一致重复、
1 个冲突重复。`demo-output/` 是一次实际运行的产物样例。

## 不覆盖原包的保证

- 所有输入包只以只读方式打开；代码中不存在对包目录的写操作。
- 运行前校验 `-out` 不得等于任一输入包目录、也不得位于其内部，否则直接
  以退出码 `2` 拒绝且不会创建任何目录。
- 测试 `TestWriteOutputsAndSourceImmutability` 在运行前后对整个示例包做
  SHA-256 树快照比对。

## 项目结构

```
main.go               根命令入口（支持 go run .）
cmd/imfplaylist/      同一 CLI 的子目录入口
internal/cli/         参数解析、退出码、输出编排与 CLI 测试
internal/imf/         无第三方依赖的核心库
  xmlnode.go          命名空间宽容的 XML 节点树
  uuid.go             urn:uuid / 裸 UUID 规范化与提取
  assetmap.go         ASSETMAP 解析
  pkl.go              PKL 解析
  cpl.go              CPL / 虚拟轨道 / 资源解析
  opl.go              OPL 及其 CPL/虚拟轨道引用解析
  package.go          包目录发现、文档分类、文件名 UUID 索引
  workspace.go        跨包资产注册表、继承解析、缺失/重复检测
  merge.go            轨道合并与最终播放清单
  cplxml.go           CPL_final.xml 生成
  output.go           JSON/文件输出、资产复制、输出目录隔离
  textreport.go       中文文本报告
testdata/main-imp/    示例主包
testdata/supp-imp/    示例增补包
```

## 标准背景

CPL 对应 SMPTE ST 2067-3，核心约束（`MainImageSequence`、
`MainAudioSequence`、字幕/标题序列等虚拟轨道）对应 ST 2067-2；OPL 对应
SMPTE ST 2067-100；AssetMap/PKL 沿用 SMPTE 429-9 / 429-8 体系。本实现是面向
“多包继承合并 + 资产核对”的轻量工具，不校验完整 XSD 与媒体本质元数据。
