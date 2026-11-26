# Insert / Overwrite 波纹重算

本目录是一个无外部依赖的 Go 参考实现，用来复算插入式拖放后的轨道片段、转场、淡化、关键帧、包络和标记。

## 根因

时间轴只有一个共享时间坐标。Insert 增加了素材时长 `D`，如果只移动目标片段，或只移动片段而没有对下游的时间绑定对象应用同一条时间映射，后面的转场边界、音频 fade、关键帧和 envelope 点就会停在旧坐标，表现为“整体错位”。

## 两种语义

设拖入点为 `p`，素材时长为 `D`，原时间为 `t`。

- Overwrite：替换目标轨道的 `[p, p+D)`，下游不波纹。
  - `t < p`：不变。
  - `p <= t < p+D`：属于被替换区间，按切割/覆盖处理。
  - `t >= p+D`：坐标不变。
  - 总时长通常不变；只有 `p+D` 超过原片尾时，时长增加超出部分。
- Insert：在 `p` 处撑开 `D`，下游整体波纹。
  - `t < p`：不变。
  - `t == p`：按下游边界处理，映射到 `p+D`。
  - `t > p`：`t' = t + D`。
  - 总时长增加 `D`。

对应映射：

```text
insert:    f(t) = t       if t < p
                  t + D   if t >= p

overwrite: f(t) = t       for all downstream t
```

## 逐项重算顺序

实现入口是 `ripple.Apply`，顺序固定为：

1. 校验编辑前时间轴，并生成转场/fade 的旧位置快照。
2. 重算 `Clip`：
   - Insert：`Start >= p` 的片段右移；跨过 `p` 的片段切成左右两段，右段右移。
   - Overwrite：在 `p` 和 `p+D` 处切割，删除/修剪被覆盖部分，右段不波纹。
3. 用片段血缘重挂 `Transition`：
   - 转场不保存独立的自由时间；它由左右相邻片段边界派生。
   - 普通下游边界随片段移动，重新计算 center/end 对齐。
   - Insert 恰好落在旧转场点时，若新片段补在该轨道空隙中，则把转场重挂到新片段与右片段之间；跨轨道波纹在该轨道留下空隙时，记录并移除无法成立的转场。
   - Overwrite 覆盖原锚点时，移除失去锚点的转场。
4. 重算 `Fade`：
   - fade-in 跟随片段入点血缘，fade-out 跟随出点血缘。
   - 片段被切短时，fade 长度钳制到片段长度并写审计。
5. 重算 `Keyframe`：
   - Insert 且关键帧属于波纹轨道时，`time >= p` 的点加 `D`。
   - Overwrite 不移动下游关键帧。
6. 重算 `Envelope`：
   - 旧点应用相同映射。
   - 跨插入点的线段先在 `p` 处求插值，再在 `p` 与 `p+D` 放两个同值 split-hold 点，避免原曲线被错误拉伸到新素材区间。
7. 按策略处理 `Marker`，并向每个标记的历史链追加一步。
8. 重新校验无重叠、片段 ID、转场相邻、fade 长度、包络排序和总时长。

## 跨轨道是否联动

共享叙事时间的轨道必须使用同一个波纹位移。默认 `RippleTrackIDs == nil` 表示所有未锁定轨道一起波纹：V1、A1、A2 上 `t >= p` 的对象都加同一个 `D`。这不是把素材自动复制到所有轨道，而是保证下游视频、同期声、音乐、关键帧和标记仍处于同一个时间坐标。

只有以下情况可以不联动：

- 显式执行 track-local ripple，例如独立副标题/音效故事线；
- 用户先解除 A/V link，再单独波纹其中一条；
- 轨道锁定且产品策略明确要求排除，但此时必须提示可能声画不同步。

`ripple.CheckLinkedSync` 会按 `LinkGroup` 比较各轨道片段的 `(start, duration)` 签名。单轨 insert 会产生可追踪的 sync issue；单轨 overwrite 可能是有意的 split edit/J-cut/L-cut，因此只作为诊断，不伪装成时间波纹。

Overwrite 的“不联动”仅指下游时间不位移。若拖入的是 A/V compound clip，选择/链接到的目标轨道仍应同步覆盖同一个 `[p,p+D)` 区间；这是目标轨道选择，不是波纹。

## 二十次随机操作与可追溯性

运行：

```bash
go test ./...
go run ./cmd/ripplecheck -seed 20261127 -n 20 -out artifacts/ripple-audit.json
```

程序用同一个确定性随机脚本分别跑 insert 和 overwrite。每次操作都记录：

- 模式、位置、素材时长、波纹轨道；
- 每个 clip/transition/fade/keyframe/envelope point/marker 的 old → new；
- 每步前后总时长、时长增量和台账；
- 每个标记从初始状态到第 20 步的完整历史链。

`ripple.VerifyTrace` 会核对：

- 20 条 record 的序号和时长台账连续；
- 初始时长 + 所有 `DurationDelta` 等于最终重算时长；
- 每个标记的 `oldTime` 等于上一步 `newTime`；
- 标记历史末端等于最终时间轴上的标记位置。

种子 `20261127` 的参考结果为：insert 总时长 `24 -> 67`，overwrite 总时长 `24 -> 28`；两者 verification 均为 `ok=true`。测试还会用 80 个额外种子各跑 20 步，检查切割、锚点和历史链。
