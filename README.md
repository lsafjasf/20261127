# 可恢复的分段渲染任务（Go）

这个仓库用纯 Go 标准库实现了一个带检查点、依赖恢复、取消续跑和输出隔离的渲染任务框架。渲染被建模为 DAG：示例管线把每一章拆成 `script -> visual -> encode`，最后由 `final:manifest` 依赖所有章节。

核心目标：

- 断电或 `SIGKILL` 后不重做已完成章节。
- 每个节点完成后立即原子提交检查点和 SHA-256 产物记录。
- 上游输入或产物变化时，只失效该节点及其下游；无关章节继续复用。
- `context.Context` 取消后，已完成节点保留，未完成节点下次重跑。
- 所有半成品只写入 `staging/`；启动恢复时移动到 `quarantine/`，不会污染 `objects/` 或 `output/`。
- 不依赖数据库、CGo、FFmpeg 或第三方 Go module。

## 目录结构

```text
.render-workspace/
├── checkpoints/<job-id>.json   # 分段检查点，temp + fsync + rename 原子更新
├── staging/                    # 正在写入的半成品；崩溃后这里可能有 .part
├── objects/<sha256>            # 已提交、不可变、按摘要存储的中间结果
├── output/                     # 原子发布的完整最终文件
├── quarantine/                 # 残留/损坏输出及 .reason.json 说明
└── render.lock                 # flock 工作区锁；进程死亡后由内核释放
```

关键包：

- `checkpoint/`：通用 DAG 运行器、原子存储、校验、隔离恢复和文件锁。
- `render/`：多章节渲染图和可替换的模拟渲染阶段。
- `cmd/renderctl/`：运行、取消演示、状态、恢复和隔离区查看命令。

## 恢复判定

一个旧节点会被安全复用，必须同时满足：

1. 检查点状态是 `completed`。
2. 任务版本和输入生成的 `CacheKey` 未变化。
3. 产物文件存在，大小和 SHA-256 与检查点一致。
4. 当前 DAG 的直接依赖集合未变化。
5. 每个依赖产物的摘要和大小与该节点完成时记录的依赖快照一致。

如果上游重算后产出的字节完全相同，下游仍可按内容摘要复用；如果上游内容变化，依赖它的节点会自动重跑。

## 快速开始

```bash
# 运行 4 章演示渲染
go run ./cmd/renderctl run --root .render-workspace --job demo-book --chapters 4

# 查看每个分段的状态、尝试次数和产物摘要
go run ./cmd/renderctl status --root .render-workspace --job demo-book

# 再次运行：未变化的节点全部显示 [reuse]
go run ./cmd/renderctl run --root .render-workspace --job demo-book --chapters 4
```

取消并续跑：

```bash
# 运行 180ms 后自动发送取消；也可以直接按 Ctrl-C
go run ./cmd/renderctl run \
  --root .render-workspace \
  --job cancel-demo \
  --chapters 6 \
  --delay 40ms \
  --concurrency 1 \
  --cancel-after 180ms

# 重新执行同一条 run 命令即可续跑
go run ./cmd/renderctl run --root .render-workspace --job cancel-demo --chapters 6 --delay 10ms
```

断电演练：

1. 在任务执行中直接终止进程，例如 `kill -9 <renderctl-pid>`。
2. 重新执行相同的 `run` 命令。
3. 运行器先恢复 `staging/`，把残留文件移动到 `quarantine/`。
4. 已完成章节显示 `[reuse]`，只有 `running`、`canceled`、`failed`、产物丢失/损坏或依赖变化的节点重跑。

也可以手动执行恢复并查看 JSON 报告：

```bash
go run ./cmd/renderctl recover --root .render-workspace --job demo-book
go run ./cmd/renderctl quarantine --root .render-workspace
```

## 接入真实渲染器

把 `render.Pipeline.Stage` 替换为真实实现即可。每个阶段应通过 `checkpoint.RunContext` 读取依赖，并通过 `PutBytes`、`PutReader` 或 `Export` 提交结果：

```go
pipeline := render.Pipeline{
    JobID:    "audiobook-render",
    Chapters: chapters,
    Stage: func(ctx context.Context, rc *checkpoint.RunContext, ch render.Chapter, stage render.Stage, input *checkpoint.Artifact) (*checkpoint.Artifact, error) {
        switch stage {
        case render.StageScript:
            // 生成脚本/时间轴
        case render.StageVisual:
            // 调用纯 Go 绘图或已有渲染库
        case render.StageEncode:
            // 调用编码器；输出必须写入 rc 提供的安全提交 API
        }
    },
}
tasks, _ := pipeline.Tasks()
report, err := (&checkpoint.Runner{
    Store: store, JobID: pipeline.JobID, Tasks: tasks, Concurrency: 2,
}).Run(ctx)
```

如果真实工具只能写指定文件路径，可以让它写 `store` 下的独立临时路径，成功后用 `Store.PutReader` 提交；不要直接把未完成文件写到 `output/`。

## 验证

```bash
go test ./... -count=1
go test -race ./... -count=1
```

测试覆盖：

- 取消后已完成章节不重做。
- 真实 `SIGKILL` 子进程后的检查点续跑和临时文件隔离。
- 上游输入变化导致下游失效，无关章节保持复用。
- 产物 SHA-256 损坏后隔离并重建。
- 写入失败时半成品进入 `quarantine/`。
- 并发 DAG 调度和竞态检测。
