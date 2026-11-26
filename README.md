# framedoctor — 直播录制掉帧 / 重复 / 跳帧审计工具

直播录制偶发掉帧时，输出文件里的**帧号连续递增、时间戳单调递增**，
`ffprobe` 看不到丢帧，人工翻日志也很难发现。`framedoctor` 用 Go 编写、
零第三方依赖，按固定时间窗对账三件事：

- **应有帧数** `want = fps × 窗长`
- **实际帧数** `got`：窗内真实观测到的帧数
- **画面推进量** `fresh / advance_u`：有多少帧带来了新画面、累计视觉变化相当于多少个正常帧间隔

再通过**采集端 → 录制端 → 播放端**三条轨迹的帧哈希序列对齐，区分三类故障，
并给出**首个异常**的窗口、时刻、PTS 与帧号。

## 三类故障的判定签名

| 类型 | 时间窗帧数 | 画面推进 | 多端对齐证据 |
|---|---|---|---|
| **采集丢帧** `capture_drop` | 采集/录制窗 `got < want` | `fresh≈got`，缺失处常见一次大跳变 | 采集端墙钟窗缺帧或有 `dropped` 标记；与录制缺帧窗合并后**确认** |
| **编码重复** `encode_duplicate` | `got == want`（数量可能完全正常） | 连续 `dup` 零推进边，`fresh < got`，常有补偿跳变 | 录制端多个帧哈希映射到同一个采集帧（many-to-one），**确认** |
| **播放器跳帧** `player_skip` | 播放窗 `got < want`，而录制窗三项全正常 | 播放序列中出现跨哈希大跳变 | 播放哈希序列是录制序列的子序列，对齐出被跳过的录制帧号/PTS，**确认** |

仅有录制文件时也能工作：

- 窗内 `got < want` → `frame_loss`（高度可能为采集/上游丢帧）；
- 帧号/PTS 完美平滑但出现 `jump ≥ 3×基线` 的中等跳变（非镜头切换）
  → `suspected_capture_restamp`，即丢帧后时间戳被重建的最隐蔽情形；
- 连续相同哈希且周围有运动 → `encode_duplicate`；静止画面下的相同帧只报 `static_repeat` 信息。

> 单靠一个录制文件无法 100% 区分采集丢帧与封装/编码丢帧，也无法证明播放器跳帧；
> 工具会明确给出 `confirmed / likely / suspected / info` 置信度，而不是武断下结论。

## 构建与自测

```bash
go build -o framedoctor .
go test ./...
```

需要 Go 1.23+，仅用标准库。`extract` 子命令可选依赖 `ffmpeg/ffprobe`。

## 30 秒体验

```bash
# 生成三端合成场景（3s 采集丢帧 / 6s 编码重复 / 9s 播放跳帧）
./framedoctor gen --out examples/scenario

# 直接审计整个目录（按文件名自动识别 capture/record/player）
./framedoctor analyze examples/scenario --all-windows

# 零落地演示：生成临时场景并立即出报告
./framedoctor demo

# CI / 程序化消费 JSON；发现异常时 --strict 返回 1
./framedoctor analyze examples/scenario --json --strict
```

合成场景的报告结论（节选）：

```
★ [F1][采集丢帧·确认 97%] window=3 t=3.000s pts=3.000s seq=90 n≈3
      证据: 应有=30 实际=27 估算缺帧=3 丢帧号=[90,91,92] 跳变倍数=3.33 ...
  [F3][编码重复帧·确认 98%] window=6 t=6.033s seq=178
      证据: 额外复制份=2 边距离=[0.000,0.000] 映射采集帧号=180 ...
  [F4][播放器跳帧·确认 97%] window=9 t=9.000s pts=9.000s seq=267 n≈4
      证据: 跳过录制帧=4 跳过帧号样本=[267,268,269,270] ...
```

## 输入轨迹格式

每个观测点一份 CSV/TSV 或 JSONL（也接受 JSON 数组 / `{"frames":[...]}`）。
列名/键名大小写、下划线、中英文均可，别名宽松匹配。最小字段：

| 含义 | 支持的字段名（举例） |
|---|---|
| 帧号 | `seq`, `frame`, `frame_no`, `frame_number`, `n`, `帧号`, `序号` |
| 流时间戳（秒） | `pts`, `pts_time`, `dts`, `timestamp`, `time`, `时间戳` |
| 流时间戳（毫秒） | `pts_ms`, `timestamp_ms`, `t_ms`（或数值 `>1e11` 自动按毫秒） |
| 墙钟（秒/毫秒） | `wall_s`, `wall_time`, `capture_time`, `render_time`, `wall_ms`, `采集时间`, `渲染时间` |
| 画面指纹 | `hash`, `phash`, `dhash`, `md5`, `signature`, `帧哈希`, `感知哈希` |
| 预计算推进量 | `diff`, `advance`, `motion`, `hamming`(原始 bit 数/64), `推进量`, `画面变化` |
| 显式丢帧标记 | `dropped`, `is_dropped`, `skipped`, `lost`, `丢帧`, `掉帧` |

哈希约定：

- 16 位 hex → 64 位感知哈希，相邻帧距离 = 汉明距离 / 64；
- 32 位以上 hex（MD5/SHA）→ 精确指纹，只判等/不等；
- 没有哈希时可用 `diff`/`advance` 列直接给归一化的相邻帧变化量。

CSV 示例（采集端，含显式丢帧行）：

```csv
seq,wall_s,hash,dropped
89,1764000002.966667,3f2c...,false
90,1764000003.000000,,true
91,1764000003.033333,,true
92,1764000003.066667,,true
93,1764000003.100000,a19d...,false
```

JSONL 示例（录制端：帧号连续、PTS 单调，但第 181/182 帧复制了第 180 帧画面）：

```json
{"seq":177,"pts":6.0,"hash":"9c10..."}
{"seq":178,"pts":6.033333,"hash":"9c10..."}
{"seq":179,"pts":6.066667,"hash":"9c10..."}
```

## 直接分析真实录像（可选 ffmpeg）

```bash
# ffprobe 取 PTS，ffmpeg 降采样 9x8 灰度并在 Go 内计算 dHash
./framedoctor extract live.mp4 --out live.trace.jsonl
./framedoctor analyze --record live.trace.jsonl --window 1s
```

生产环境建议在采集/播放日志里直接输出 pHash 或帧间 `diff`，
比 9x8 dHash 对低细节画面更灵敏。

## 命令与参数

```
framedoctor analyze [选项] <文件或目录...>
  -c, --capture <f>   采集端轨迹
  -r, --record  <f>   录制/编码端轨迹（位置参数默认也是它）
  -p, --player  <f>   播放/渲染端轨迹
      --fps <n>       标称帧率；缺省按时间戳中位数稳健估计
      --window <d>    时间窗：1s / 500ms / 0.5（默认 1s）
      --clock <axis>  auto | pts | wall（默认 auto）
      --dup-bits <n>  感知哈希汉明距离 ≤ n bit 视为同一画面（默认 2）
      --jump-ratio <x> 大跳变判定：≥ x 倍基线步长（默认 3）
      --all-windows   文本报告输出每个窗口（长报告默认只显示异常窗）
      --format json / --json   输出结构化 JSON（含全部窗口与证据）
      --out <f>       输出到文件
      --strict        存在异常时退出码 1（参数/IO 错误为 2，干净为 0）

framedoctor gen --out <dir> [--fps 30] [--seconds 12]
framedoctor extract <video> [--out trace.jsonl]
framedoctor demo
```

文件名包含 `capture/采集/cam`、`record/encode/录制`、`player/render/播放`
时，目录扫描会自动归到对应阶段。

## JSON 输出要点

```json
{
  "schema": "framedoctor/v1",
  "window_s": 1,
  "windows": { "record": [ { "index": 3, "want_frames": 30,
      "actual_frames": 27, "missing_frames": 3,
      "fresh_frames": 27, "duplicate_frames": 0,
      "advance_frame_units": 29.33, "jump_edges": 1 } ] },
  "findings": [ { "kind": "capture_drop", "severity": "confirmed",
      "window": 3, "time_s": 3, "pts_s": 3, "seq": 90,
      "confidence": 0.97, "anomaly": true, "evidence": { } } ],
  "first_anomaly": { "...": "..." },
  "summary": { "capture_drop": 1, "encode_duplicate": 1, "player_skip": 1 }
}
```

## 判定原理简述

1. 每条轨迹选定时间轴（PTS 或墙钟），按起点切固定半开窗口；
   `want = fps × 窗口与时间线的重叠时长`，首尾非整窗自动缩放。
2. 相邻帧计算归一化视觉距离；以“非零步长的中位数”为运动基线，
   把累计距离换算成 `advance_frame_units`，并统计零推进边 `dup` 与新画面 `fresh`。
3. 缺帧看 `want-got`，重复看连续零推进边，跳帧看跨端哈希子序列对齐。
4. 三端用单调哈希匹配对齐：录制→采集允许 many-to-one（重复帧映射同一采集帧），
   播放→录制按顺序一对一推进；对齐缺口即被播放器跳过的精确录制帧。
5. 所有结论按可映射到录制时间线的时刻排序，最早的异常写入 `first_anomaly`。

## 项目结构

```
main.go            入口
cli*.go            命令行（analyze / gen / extract / demo）
trace.go           阶段与帧模型
fields.go          中英文别名字段解析、时间格式
parse.go           CSV/TSV/JSON/JSONL 与目录加载
sig.go             pHash/精确指纹与汉明距离
window.go          时间窗、应有/实际/推进量
analyze.go         缺帧/重复/跳变基础检测
reconcile.go       三端序列对齐与归因
finding.go         异常模型、合并去重、首个异常
report_text.go     中文文本报告
synth.go           合成故障场景
ffmpeg.go          ffmpeg/ffprobe 抽帧与 dHash
*_test.go          14 个单元/端到端/冒烟测试
```
