# seekdiag：音频 seek 渐进延迟诊断

这是一个纯 Go、无 cgo、无外部进程依赖的 ISO BMFF/MP4 诊断工具。它直接
解析 `moov`，记录 seek 目标前后的：

- 容器 PTS（`stts`/`ctts` 与 `mvhd`/`mdhd` 时间刻度）；
- 样本位置（1-based 样本号、文件字节偏移、chunk、chunk 内偏移、PCM 帧号）；
- 编辑列表（原始 `elst` 项及映射后的 movie/media 区间）；
- 解码后 PTS 与播放时钟探针（从播放器日志导入）；
- 对渐进漂移的线性斜率，并按证据定位到容器、解码器或播放时钟。

它不是通过“定期重同步”掩盖问题。时钟 API 在锚定一次后会拒绝再次锚定；
只有用户主动 seek 才允许创建新时钟。

## 判定规则

工具不会仅凭“20 分钟后延迟增加”猜测层级。探针必须使用同一条 movie
绝对时间轴，不能把 seek 后的本地 PTS 重置为 0。

1. **容器**：`audio_pts_ms` 与独立解析的样本表/编辑列表预测值不一致，
   或者 `elst` 越界、样本 PTS/PCM 帧长不一致。此时 demux 时间轴错误。
2. **解码器**：容器包 PTS 正确，但 `decoded_audio_pts_ms` 相对包 PTS
   渐进偏移。重点检查 seek 后 flush、重排和 PTS 重设路径。
3. **播放时钟**：容器与解码 PTS 一致，但 `playback_clock_ms` 相对解码
   PTS 渐进漂移。重点检查 seek 后时钟种子、音频主时钟和采样率映射。

每层对 seek 后的时间窗做最小二乘拟合。默认同时满足“累计变化 >= 10 ms”
且“斜率 >= 0.1 ms/s”才判为渐进故障；初始静态偏差超过 2 ms 会单独告警。
没有运行时探针时，即使容器正常也只会输出 `inconclusive`，因为容器数据
无法自证解码器或播放时钟的行为。

## 运行

```bash
go run ./cmd/seekdiag -seek 20m media.mp4
go run ./cmd/seekdiag -seek 20m -json media.mp4
```

加载播放器日志：

```bash
go run ./cmd/seekdiag -seek 20m -json \
  -observations probe.json media.mp4
```

构建和测试：

```bash
go test ./...
go test -race ./...
go build ./cmd/seekdiag
```

## 探针格式

```json
{
  "seek_ms": 1200000,
  "before": [
    {
      "label": "near-20m",
      "wall_ms": -1000,
      "audio_pts_ms": 1199000,
      "decoded_audio_pts_ms": 1199000,
      "playback_clock_ms": 1199000,
      "audio_sample_index": 56205
    }
  ],
  "after": [
    {
      "wall_ms": 0,
      "audio_pts_ms": 1200000,
      "decoded_audio_pts_ms": 1200000,
      "playback_clock_ms": 1200000,
      "audio_sample_index": 56252,
      "audio_pcm_offset": 57600000
    },
    {
      "wall_ms": 30000,
      "audio_pts_ms": 1230000,
      "decoded_audio_pts_ms": 1230000,
      "playback_clock_ms": 1230000,
      "audio_sample_index": 57658,
      "audio_pcm_offset": 59040000
    }
  ]
}
```

同一时刻可提供以下任意一种音频位置，用于从容器独立计算期望 PTS：

- `audio_sample_index`：1-based 编码样本序号；
- `audio_byte_offset`：样本文件字节偏移；
- `audio_pcm_offset`：movie 时间轴上的可听 PCM 帧号。

所有 PTS/时钟值都应是 movie 绝对毫秒值。`wall_ms` 是 seek 后单调时钟，
只用于拟合斜率，不参与媒体时间映射。

## 输出

文本模式会打印 `elst` 原项、seek 前/命中/后样本、一次性锚点、三层漂移量
和证据。`-json` 模式包含完整 `seek_plan`、`tracks`、`observations.drift`
与 `findings`。关键字段：

```text
seek_plan.tracks[].boundary.previous  seek 目标前一个样本
seek_plan.tracks[].boundary.landing   目标命中样本
seek_plan.tracks[].boundary.next      目标后一个样本
seek_anchor.movie_ms                  movie 时间轴目标
seek_anchor.audio.anchor_pcm          首个真正可听的 PCM 帧
verdict                               container/decoder/playback_clock
```

## 正确修复路径

1. seek 时用 `mvhd`/`mdhd` 时间刻度和 `elst` 计算 media 目标，不把本地
   PTS 清零。音频命中帧前的 `trim` 只裁剪一次。
2. 让音频主时钟从“首个可听 PCM 帧”开始，锚在用户请求的 movie 时间：

   ```go
   plan, _ := seek.At(file, 20*time.Minute)
   anchor, _ := clock.AnchorFromSeek(plan)
   // 后续只按 PCM 帧数推进：
   movieMS := anchor.Audio.MovieMS(framesPlayed)
   ```

3. 视频跟随同一 movie 时间轴；显式 seek 创建新 `Clock`，正常播放期间不得
   再次调用 `Anchor`。若设备晶振造成长期微漂，应使用连续的采样率微调
   （resampling/servo），而不是周期性跳变 PTS 或丢/补音频帧。

`internal/clock` 的第二次锚定会返回 `ErrAlreadyAnchored`。这从 API 层禁止了
“周期性重同步”修复，避免把容器或解码器错误藏起来。

## 实现范围

解析 `ftyp`、`moov`、`mvhd`、`trak`、`tkhd`、`edts/elst`、
`mdia/mdhd/hdlr`、`stbl/stsd/stts/ctts/stsc/stsz/stz2/stco/co64/stss`。
支持 32/64 位盒子与时间字段。当前不支持仅含 `moof` 的 fragmented-only
文件；这类文件应先拼接或显式解析分片时间轴。

合成测试构造了一个 30 分钟、48 kHz、84,375 个 AAC 样本、带
`media_time=1024` 编辑列表的 MP4，并验证 20 分钟处：

- 命中样本号、文件偏移和 `trim`；
- 低频容器漂移、解码器漂移、播放时钟漂移分别得到正确层级；
- 一次性 seek 锚点得到首个可听 PCM 帧；
- 第二次时钟锚定被拒绝。
