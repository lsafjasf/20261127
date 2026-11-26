# CFR 尾部黑场定位器

只用 Go 标准库，沿三条时间线核对“CFR 素材末尾多出两帧黑场”：

1. 帧 PTS：输出帧是否仍满足 `PTS_i = start + i / fps`；
2. 输出合同：内容帧数、交付帧数/时长、允许的尾部黑场数；
3. 播放器显示：每帧的半开显示区间、播放器上报时长和 EditList 截断点。

仓库初始没有渲染器源码或媒体样例，因此这里交付一个可运行的诊断/合同核对器，并用
“10 个内容帧 + 2 个尾部黑场帧”的标准复现锁定结论。

## 定位结论

设 CFR 帧率为 `p/q`，帧时长 `d = q/p`，内容有 `N` 帧：

| 对象 | 帧序号/PTS | 显示区间 |
| --- | ---: | --- |
| 最后一帧内容 | `N-1`, PTS `(N-1)d` | `[(N-1)d, Nd)` |
| 第 1 帧补黑 | `N`, PTS `Nd` | `[Nd, (N+1)d)` |
| 第 2 帧补黑 | `N+1`, PTS `(N+1)d` | `[(N+1)d, (N+2)d)` |
| 文件/排期结束 | — | `(N+2)d` |

因此：

- 多出的两帧来自**渲染器尾部补黑逻辑**，典型来源是 tail padding、GOP/handle 对齐、
  EOF flush 或“总时长按闭区间/向上取整后多分配帧”的分支；
- 它们的 PTS 连续、漂移为 0，且第一个额外 PTS 恰好等于内容时间线结束 `Nd`，不是 PTS 生成错误；
- 播放器按 PTS 显示，每帧持续到下一帧 PTS，末帧持续到 sample duration、容器结束或 EditList 结束；
- 客户看到的“总时长变长”是真实增加了两个显示帧，时长增量为 `2d`，不是播放器凭空多播；
- “最后一帧 PTS”不是总时长：末帧还要占一个帧时长，所以 sample/stream end 是
  `last_pts + d`。

30 fps、`N=10` 时：内容时长 `1/3s`，两帧黑场从 PTS `1/3s` 开始，交付结束于 `0.4s`。
`30000/1001` fps 时，两帧增量精确为 `1001/15000s ≈ 66.733ms`。

## 运行

```bash
go test ./...
go run ./cmd/tailtrace
go run ./cmd/tailtrace -in examples/cfr_tail_black.json
go run ./cmd/tailtrace -in evidence.json -json
go run ./cmd/tailtrace -in evidence.json -fix trim -fix-out fixed-evidence.json
go run ./cmd/tailtrace -in evidence.json -fix contract -fix-out revised-contract.json
go build ./cmd/tailtrace
./tailtrace -in evidence.json -check
```

`-check` 会在合同不满足时以状态码 `1` 退出，可用于交付前质检。

内置样例同时构造了 PTS ticks/time base、容器 duration、播放器上报时长和逐帧显示证据。

## 真实素材如何取证

输入 JSON 的完整字段可参考 `examples/cfr_tail_black.json`。有理数可写为 `"30000/1001"`、
`"1/90000"`、整数字符串或普通 JSON 数字；所有帧边界比较都使用精确有理数。

1. **帧 PTS 层**
   - 导出每一输出帧的 index、PTS（或 `pts_ticks + time_base`）、黑场检测结果和来源标记；
   - 核对 PTS 是否等于 `start_pts + index * frame_duration`；
   - 若末尾两帧 `black=true`、`origin="tail_pad"`，且 PTS 为 `Nd`、`(N+1)d`，即可定位到渲染器补尾；
   - 若 PTS 漂移不为 0，先查时间基/舍入，不能直接归因于黑场策略。

2. **输出合同层**
   - `content_frames`：正片/客户内容帧数；
   - `delivery_frames` 或 `delivery_duration`：合同规定的交付总帧数/总时长；
   - `allowed_tail_black_frames`：明确允许的尾部黑场/lead-out 帧数；
   - `container_duration`：容器或 `format=duration` 元数据。
   - `audio_duration`：音频/母版总时长；若它恰好等于补黑后的视频结束点，说明渲染器可能在做 A/V 总时长对齐。
   - 实际帧数超过 `delivery_frames` 或尾部黑场超过允许值时，工具判为合同违反。

3. **播放器显示层**
   - CFR 排期区间为 `[PTS_i, PTS_(i+1))`，最后一帧为 `[last_pts, last_pts+d)`；
   - 可录入播放器实际上屏时间 `shown_start/shown_end` 和 UI 上报时长；
   - 若有 EditList，录入 `player.edit_list_end`。合规播放器会在该点停止，但额外 sample
     仍留在容器中；这解释了“文件里有帧”和“正常播放看不到”的差异。

## 修复或合同解释

**根因修复：停止补帧并真正重写封装。**

- 关闭渲染器中无条件追加两帧黑场的 tail/GOP/EOF padding；
- 或在输出前裁掉 index `N`、`N+1` 两个 sample；
- 保留前 `N` 帧 PTS，不要平移内容时间戳；
- 同步更新 sample count、sample duration、`mdhd/tkhd/format duration`、索引和 EditList；
- 只改 duration 元数据不会删除黑场 sample，部分播放器/解码器仍会把它们解码或显示出来。

`-fix trim` 会生成目标证据并复检：输出恢复为 `N` 帧，结束时间为 `Nd`。

**合同解释：如果两帧是有意的播出 lead-out。**

把合同改成“`N` 个内容帧 + 2 个明确黑场帧”，并同时对客户报告：

- 内容时长：`N/fps`；
- 交付/容器/播放器时长（含黑场）：`(N+2)/fps`；
- 黑场显示时长：`2/fps`。

`-fix contract` 会生成对应的 `delivery_frames`、`delivery_duration` 和
`allowed_tail_black_frames=2`。若只希望文件保留 sample 但播放器不显示，应使用并验证
EditList/播出窗口，而不是口头解释总时长。

## 边界

本模块不直接解析 MP4/MKV 二进制容器或编码码流，也不调用外部程序；它消费由渲染器清单、
PTS 导出、黑场检测和播放器轨迹形成的 JSON 证据。仅有“末尾两帧是黑的”不足以证明它们是
补帧：正常创作也可能以黑场结束。根因判定需要内容帧边界、source/output 映射或
`origin=tail_pad` 标记中的至少一项。
