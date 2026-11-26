# HDR/SDR 双版本交付清单

用 Go 编写的交付验收与清单生成工具。它读取 HDR、SDR 两个母版和共享音频/字幕资产，逐项核对色彩元数据、共享时间线、切点和默认轨，并输出 JSON 或 Markdown 清单。

## 核心原则

- 媒体事实来自 `ffprobe`；音轨/字幕共享关系来自 `ffmpeg streamhash`，不是只看文件名或配置声明。
- HDR 与 SDR 的每个必需检查项都会独立记录，包含检查项 ID、期望值、实际值、逐项结果和失败原因。
- 只有 HDR、SDR 和共享项全部通过，顶层才会出现 `"status": "complete"` 与 `"complete": true`。
- 任一版本缺少音轨、字幕、默认轨、切点、色彩元数据或共享资产时，清单为 `incomplete`，进程默认返回退出码 `1`。
- 即使不完整，清单仍会完整写出，失败项会进入顶层 `failures`，便于修复后重新验收。

## 构建与运行

依赖：

- Go 1.22+
- `ffprobe`、`ffmpeg`（建议 FFmpeg 6+）

```bash
go build -o bin/hdr-sdr-manifest .

bin/hdr-sdr-manifest \
  -config examples/delivery.example.json \
  -out out/manifest.json

bin/hdr-sdr-manifest \
  -config examples/delivery.example.json \
  -format markdown \
  -out out/manifest.md
```

常用参数：

| 参数 | 说明 |
|---|---|
| `-config` | 必填，交付输入 JSON |
| `-out` | 输出路径，`-` 表示 stdout，默认 `-` |
| `-format` | `json` 或 `markdown`，默认 `json` |
| `-ffprobe` | ffprobe 路径，默认 `$FFPROBE` 或 `ffprobe` |
| `-ffmpeg` | ffmpeg 路径，默认 `$FFMPEG` 或 `ffmpeg` |
| `-allow-incomplete` | 不完整时仍返回 `0`，清单状态不会被改写 |

退出码：

| 退出码 | 含义 |
|---|---|
| `0` | 清单完整；或使用了 `-allow-incomplete` 且成功产出清单 |
| `1` | 清单已产出，但存在必需项缺失/不一致 |
| `2` | 参数、配置、切点清单格式或执行环境错误 |

配置中的相对路径均相对于配置文件所在目录解析。

## 输入配置

完整示例见 `examples/delivery.example.json`。最小结构：

> 示例中的 `media/`、`shared/` 路径是结构占位，需替换为真实交付文件；未提供真实媒体时不会产出 `complete`。

```json
{
  "project": {
    "id": "feature-2026-001",
    "title": "Feature"
  },
  "hdr": {
    "path": "media/feature_HDR10.mxf",
    "label": "HDR10"
  },
  "sdr": {
    "path": "media/feature_SDR.mxf",
    "label": "SDR"
  },
  "shared": {
    "audio": [
      {
        "id": "main-5.1",
        "path": "shared/feature_5.1_48k.mxf",
        "language": "zho",
        "title": "Main 5.1",
        "default": true
      }
    ],
    "subtitles": [
      {
        "id": "zh-hans",
        "path": "shared/feature_zh-Hans.ass",
        "language": "zho",
        "title": "简体中文",
        "default": true
      }
    ]
  },
  "cuts": {
    "path": "edit/cuts.json",
    "tolerance_ms": 2
  }
}
```

`shared` 可以省略。省略时，工具仍会要求两版各至少有一条音轨和一条字幕，并比较容器内全部音轨/字幕流的哈希与元数据。

声明外部共享资产时，资产文件中的目标流必须真实内嵌于 HDR 和 SDR 两个版本，且各自恰好出现一次。工具会核对：

- 共享资产文件 SHA-256；
- 目标流的 `SHA-256` 流哈希；
- 该流在 HDR/SDR 容器中的存在性和唯一性；
- 配置声明的语言、标题和 `default` 标记。

`language` 支持常见 ISO 639-1/639-2 写法，例如 `zh`、`zho`、`chi` 会归一化比较。

## 色彩核对

未显式覆盖时使用以下严格默认值：

| 项目 | HDR | SDR |
|---|---|---|
| primaries | `bt2020` | `bt709` |
| transfer | `smpte2084` 或 `arib-std-b67` | `bt709` |
| matrix | `bt2020nc` | `bt709` |
| range | `tv` | `tv` |
| bit depth | `10` | `8` |

PQ/HDR10 默认还要求首帧存在 mastering display metadata 与 content light level metadata；HLG 默认不强制这两项。每一项都生成独立检查：

`color.primaries`、`color.transfer`、`color.matrix`、`color.range`、`color.bit_depth`、`color.mastering_display`、`color.content_light`。

可在版本内通过 `color` 覆盖，例如要求 12-bit 或只接受 PQ：

```json
{
  "hdr": {
    "path": "media/feature_HDR10.mxf",
    "color": {
      "primaries": "bt2020",
      "transfer": "smpte2084",
      "matrix": "bt2020nc",
      "range": "tv",
      "bit_depth": 10,
      "require_mastering_display": true,
      "require_content_light": true
    }
  }
}
```

`transfer` 与 `allowed_transfers` 互斥；需要接受多个传输函数时使用后者。

## 共享时间线

工具比较两版视频的：

- 时长与起点，默认容差分别为 `40 ms`、`100 ms`；
- 帧率，默认容差 `0.0001 fps`；
- 帧数：通过 `ffprobe -count_frames` 读取 `nb_read_frames` 并要求精确相等；若不可用，再尝试容器 `nb_frames`。两者都不可用时记录 `frame_count_available` 失败，不使用估算值宣称完整。

配置示例：

```json
{
  "timeline": {
    "duration_tolerance_ms": 40,
    "start_tolerance_ms": 100,
    "frame_rate_tolerance": 0.0001
  }
}
```

时间线任一值不可用、超差或帧数不一致，都使共享状态和总体状态变为 `incomplete`。估算帧数仍会出现在清单中供排查，但不能替代精确帧数通过验收。

## 切点核对

切点来源有两种：

1. 显式清单（推荐）：`cuts.path` 指向 JSON，或在配置内直接写 `cuts.points`。
2. 章节对拍：不配置 `cuts.path`/`points`，或设置 `"source": "paired"`。HDR 章节必须非空并作为基线，SDR 章节点必须逐项匹配。

切点清单格式：

```json
{
  "cuts": [
    {"id": "C001", "time_ms": 0, "label": "Opening"},
    {"id": "C002", "time_ms": 125000, "label": "Reel 2"}
  ]
}
```

`time_ms` 使用毫秒整数。默认容差 `2 ms`；任一切点缺失或出现未声明切点都会失败。若确需允许额外章节，可设置 `"allow_extra": true`，但缺失切点仍会失败。

外部切点清单缺失或损坏时，仍会生成清单，但 HDR/SDR 均记录 `cuts.source_available` 失败，总体为 `incomplete`。

## 默认轨与共享流

- HDR、SDR 各自必须恰好有一条默认音轨和一条默认字幕轨。
- 两版默认音轨的流哈希、编解码、语言、标题、声道布局和采样率必须一致。
- 两版默认字幕的流哈希、编解码、语言和标题必须一致。
- 默认轨双方都暴露起始时间/时长时，还会按共享时间线容差核对；单侧未暴露会记录在 `defaults.*_timing` 的实际值中，不伪造数值。
- 两版全部音轨和字幕流的哈希与元数据集合必须一致；顺序可以不同。
- 声明外部共享资产时，还会核对资产是否真实内嵌、语言/标题和声明默认标记。

## 清单与完整性

JSON 顶层关键字段：

- `status` / `complete`：总体状态。只有所有必需检查通过时才为 `complete` / `true`。
- `summary`：总检查数、通过数、失败数，以及 HDR/SDR/共享三个子状态。
- `shared.timeline`：两版时间线实际值与逐项检查。
- `shared.assets`：共享资产文件哈希、流哈希、两版内嵌位置与默认标记。
- `versions.hdr` / `versions.sdr`：视频信息、色彩实际值与检查、切点、默认轨、容器内音轨/字幕摘要。
- `failures`：所有未通过项的扁平列表，含 `scope`、`check_id`、期望值、实际值和原因。

Markdown 输出包含总览、失败项、时间线、版本摘要、共享资产和完整逐项检查表。

## 已知边界

- 章节对拍要求切点以容器章节的形式存在；没有章节时请使用显式切点清单。
- 流哈希会读取完整音轨/字幕包，精确帧数会扫描完整视频流，长片验收需要相应 I/O 时间；视频文件整体 SHA-256 默认关闭，可通过 `"checks": {"media_hashes": true}` 开启。
- 工具验证交付结构、元数据与共享一致性，不评估画质、色调映射效果或主观 HDR/SDR 视觉匹配。
- 外部共享资产按“单个目标流”建模；一个文件包含多条同类型流时会判为不明确并失败。
- 所有要求均为硬性要求，没有“警告但可完整”的降级路径。

## 测试

```bash
go test ./...
go vet ./...
```
