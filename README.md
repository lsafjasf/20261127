# laddercheck — 转码 Ladder 统一关键帧核对器

纯 Go 标准库实现（零第三方依赖）。比较各转码档的 **GOP、IDR、编辑列表（`elst`）和首帧时间**，
在统一的精确有理数时间线上对齐关键帧，生成“统一关键帧约束”，并逐档判定：

- `compliant`：已对齐，可安全切换；
- `remux_only_fixable`：IDR/GOP 编码结构正确，只是 `elst`/首帧 PTS/时间基标签偏移，**不重编码**即可修正；
- `requires_reencode`：统一切换点缺少闭合 IDR，或关键帧是 CRA/BLA/非 IDR I（开放 GOP），**不重编码无法满足**；
- `insufficient_evidence`：无法解析或证据不足，不臆断。

它直接解析 ISO-BMFF/MP4/MOV（含 `stts/ctts/stss/stsz/stsc/stco/co64/elst` 与
HLS/DASH 常见的分片 MP4 `moof/traf/tfdt/trun`），并通过样本中的 H.264/HEVC NAL 类型确认真 IDR；
也支持用一份 JSON 证据描述无法直接解析的 MPEG-TS/外部上报数据。

## 为什么切换会黑屏

ABR/HLS/DASH 只在 GOP/分段边界切换，新档必须从一个**闭合 IDR** 开始解码。两类问题都会黑屏：

1. **真错位（必须重编码）**：各档 GOP 长度/相位不同、统一切换时刻根本没有 IDR，或该点只有依赖前文的
   CRA/非 IDR I（开放 GOP）。封装/时间戳操作不可能“造出”一个 IDR。
2. **假错位（只需重封装）**：关键帧对应的内容帧完全一致，但 `elst` 的空编辑/`media_time`、
   初始 PTS（编码器延迟补偿）或时间基不同，使同一 IDR 在各档的**显示时刻**差一个恒定平移。

### 判定用的时间线（关键设计）

```
内容相对时间 content_t = 关键帧媒体 PTS − 首帧 PTS(EarliestPTS)     # 不受时间基/起始标签影响
显示时间   display_t  = 经 elst 映射后的 movie 时间                 # 播放器/切换实际看到的时间
```

- 用 `content_t` 在统一 GOP 网格上找“该内容时刻是否存在闭合 IDR”，决定 GOP/IDR 结构是否合格；
- 用 `display_t` 与统一绝对网格 `origin + n·GOP` 比较，决定当前是否可安全切换；
- 结构合格但显示有恒定偏差 ⇒ 偏差完全来自标签层 ⇒ `remux_only_fixable`，并给出要写入的
  `elst media_time`/PTS 平移量；
- 以 **PTS（显示序）** 而非 `stss` 的解码样本序号对齐：B 帧重排会让解码序号不等于 `GOP×n`
  （实测 x264/x265 会出现第 57 号样本对应第 2 秒 GOP 点），但 PTS 内容网格不受影响。

所有时间比较都用 `math/big` 风格的精确有理数（自带精简 `Rat`），不存在浮点误判。

## 快速开始

```bash
go test ./...
go build ./cmd/laddercheck

# 运行内置 5 档事故复现：生成纯 Go 合成的真实 MP4 字节并输出报告到 artifacts/
go run ./cmd/laddercheck demo
#   artifacts/demo-ladder/rung-*.mp4
#   artifacts/demo-report.json
#   artifacts/demo-report.md
#   artifacts/unified-keyframes.json

# 直接分析一个真实 ladder 目录或若干文件/分片合并文件
go run ./cmd/laddercheck analyze /path/to/rungs_dir
go run ./cmd/laddercheck analyze -fps 30/1 -gop-frames 60 1080p.mp4 720p.mp4 480p.mp4

# 用 JSON 证据（无需随仓库提供媒体文件）
go run ./cmd/laddercheck analyze -in examples/ladder.evidence.json
go run ./cmd/laddercheck analyze -in examples/ladder.aligned.json -check   # 退出码 0
go run ./cmd/laddercheck analyze -in examples/ladder.evidence.json -check  # 非全 compliant，退出码 1

# 查看单档解析出的全部事实（GOP/IDR/elst/PTS）
go run ./cmd/laddercheck inspect some.mp4
```

`-check` 仅在**所有档当前都 `compliant`（现在就能安全切换）**时退出 0，否则退出 1，可直接做交付前门禁。

## 内置事故 ladder 的结论

`go run ./cmd/laddercheck demo` 构造 30fps、目标 GOP=60 帧（2s）的 5 个档：

| 档 | 注入的差异 | 判定 | 不重编码能否满足 |
| --- | --- | --- | --- |
| `rung-1080p` | 基准：IDR@0,60,120，零起始，无偏移 elst | `compliant` | 已满足 |
| `rung-720p` | 相同 IDR 结构，但 elst 多 1 帧空编辑，显示整体 +1/30s | `remux_only_fixable` | 能：重写/归一化 `elst` |
| `rung-480p` | 相同 IDR 结构，但初始 PTS=0.1s（CTO 平移、无 elst） | `remux_only_fixable` | 能：`elst media_time=参考IDR PTS` 或平移 PTS |
| `rung-360p` | GOP=48 帧（1.6s），IDR@0,48,96,144，错过 2s/4s 点 | `requires_reencode` | **不能** |
| `rung-240p` | 网格点 60/120 是 CRA（开放 GOP）而非闭合 IDR | `requires_reencode` | **不能** |

因此本 ladder 中 **`rung-360p` 与 `rung-240p` 无法在不重编码的情况下满足统一关键帧约束**；
`rung-720p`、`rung-480p` 可仅靠封装/编辑列表修正。该结论由 `go test` 锁定
（含“仅改 `elst` 后即变 compliant”的反证）。

## 输入 JSON（manifest / evidence）

`-in` 接受 `LadderInput`：既可用 `renditions[].path` 引用真实文件由 Go 解析，也可直接内联证据。
有理数可写为 `"30000/1001"`、`"30"`、`"0.5"` 或 JSON 数字。完整示例见
`examples/ladder.evidence.json`，精简字段如下：

```json
{
  "constraint": { "fps": "30/1", "gop_frames": 60, "gop_seconds": "2", "horizon_seconds": "5" },
  "renditions": [
    { "id": "720p", "path": "/abs/or/rel/rung.mp4", "role": "1280x720" },
    {
      "id": "480p", "fps": "30/1", "sample_count": 150,
      "first_pts": "1/10", "keyframe_samples": [0, 60, 120]
    },
    {
      "id": "240p-open-gop", "fps": "30/1", "sample_count": 150,
      "keyframe_samples": [0, 60, 120], "non_idr_samples": [60, 120]
    },
    {
      "id": "with-elst", "fps": "30/1", "sample_count": 150,
      "keyframe_samples": [0, 60, 120],
      "edits": [
        { "empty": true, "segment_duration": "1/30" },
        { "segment_duration": "5", "media_time": "0", "media_rate": "1" }
      ]
    }
  ]
}
```

证据字段还支持 `unknown_idr_samples`（无法确认是否 IDR → 判 `insufficient_evidence` 而非误判）、
`variable_frame_duration`、显式 `keyframes[]`（含逐帧 `pts/display_pts/idr`）。

## 统一关键帧约束与编码器落地

未显式指定 `-gop/-fps/-gop-frames` 时，工具取各档**众数统一 GOP** 与共同帧率推断网格，
并在报告中标注 `constraint.source`。推荐显式固定，避免“从错误 ladder 推断出错误标准”。
生成的约束（`artifacts/unified-keyframes.json`）要点：

- 所有档相同 CFR 帧率；GOP 用**帧数**固定（避免 29.97fps 下“2 秒≠整数帧”的坑，60 帧=2002/1000s）；
- 帧 0 为闭合 IDR，此后每 `GOPFrames` 帧一个闭合 IDR，且每个分段边界都落在 IDR 上；
- 关闭场景切换插帧（x264 `scenecut=0` 且 `keyint=min-keyint`），避免插帧打乱节奏；
- HEVC 关闭 open-GOP（`--no-open-gop`），保证网格点是 IDR 而非 CRA；
- 统一起始显示 PTS 到原点，`elst` 要么没有、要么各档一致（单一恒等映射），不得有档特有的空编辑/偏移。

参考编码参数（30fps、60 帧 GOP）：

```bash
# H.264
ffmpeg -i in.mov -r 30 -fps_mode cfr -c:v libx264 -pix_fmt yuv420p \
  -g 60 -keyint_min 60 -sc_threshold 0 \
  -x264-params keyint=60:min-keyint=60:scenecut=0:force-cfr=1 out.mp4
# HEVC：追加 -x265-params keyint=60:min-keyint=60:scenecut=0:no-open-gop=1
```

免重编码修正的合同（工具会为每个 `remux_only_fixable` 档给出精确数值）：用一个 rate=1 的 edit，
其 movie 起点为 `origin`、`media_time` 等于参考 IDR 的媒体 PTS，使
`display = origin + (media_pts − ref_idr_media_pts)`；等价于整体平移 PTS 并去掉档特有的空编辑。
代表命令为 `ffmpeg -i rung.mp4 -map 0 -c copy -avoid_negative_ts make_zero aligned.mp4`，
但具体 `elst/PTS` 结果务必再用 `laddercheck analyze` 复核。

## 输出与退出码

- 终端：对比表 + 逐档原因/动作；`-json report.json`（或 `-json -`）输出完整机器报告；
  `-md report.md` 输出中文完整报告。
- 退出码：`0` 成功（加 `-check` 时表示当前 switch-safe）；`1` `-check` 发现非 compliant；
  `2` 输入/解析错误。

## 真实性校验

除 Go 单测与纯 Go 合成 MP4 的“写入→解析→判定→改 elst 反证”闭环外，解析器还用本机
ffmpeg/x264/x265 7.0.2 生成的真实文件交叉核对过：普通 MP4、B 帧重排（解码序号 57 而 PTS 在 2s）、
x265 默认 open-GOP（正确识别网格点为 CRA/非 IDR → 重编码）、`empty_moov` 分片 MP4，以及
HLS fMP4 的 `init.mp4 + *.m4s` 合并输入，均能得到正确的 PTS/IDR/判定。ffmpeg 仅用于人工/可选的
造数与交叉验证，**不是运行期依赖，核心代码全部为 Go**。

## 代码结构

| 文件 | 职责 |
| --- | --- |
| `rational.go` | 精确有理数、解析、网格距离、JSON |
| `model.go` | Facts / Constraint / Verdict / Report 模式 |
| `mp4_box.go` `mp4_track.go` `mp4_tables.go` `mp4_facts.go` | ISO-BMFF box 树与 progressive 样本表解析、elst 映射 |
| `mp4_frag.go` | 分片 MP4：`tfdt/tfhd/trun`、sample flags、data offset |
| `nal.go` | AVC/HEVC NAL 走查与真 IDR/CRA 判定 |
| `constraint.go` | 统一关键帧网格生成、GOP 推断、编码器提示 |
| `evaluate.go` | 内容 PTS 对齐与 compliant/remux/reencode 分类 |
| `evidence.go` | JSON manifest/证据、目录扫描、加载合并 |
| `fixture.go` `fixture_box.go` `demo.go` | 纯 Go 确定性 MP4 造数与 5 档事故 ladder |
| `render.go` `render_md.go` | 终端 / Markdown 报告 |
| `cmd/laddercheck/main.go` | `analyze` / `demo` / `inspect` CLI |

## 范围与限制

- 完整支持 progressive MP4/MOV/M4V 与自包含分片 MP4，以及“init+分片拼接”的 fMP4；
  分离的 init 与多个 `.m4s` 需拼接或在 JSON 证据中描述。
- MPEG-TS 不直接解复用（PAT/PMT/PES），可用 ffprobe/产线数据导出为 `renditions[]` 证据后得到同样判定。
- 工具给的是精确的元数据修正**合同与数值**，未实现保留全部音轨的通用二进制 remux 重写器，
  以免在多轨文件上丢轨；落地请用 MP4Box/Bento4/ffmpeg 或自有封装服务按该合同改写。
- IDR 判定需要能读到样本字节；当 `stco/co64`/样本大小缺失时降级为 `idr=null`，
  相关网格点会进入 `insufficient_evidence` 而不是被乐观放行。
