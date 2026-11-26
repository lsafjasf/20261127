# subdeliver

片方双交付工具：从**同一份源清单**同时生成内嵌字幕与 sidecar 字幕，并在发布前
逐条回读比对，文本、时间或语言任何一项不一致即阻断发布（非零退出）。

## 交付物

对清单中的每个语言轨生成：

- sidecar：`<media_id>.<lang>.srt` 与 `<media_id>.<lang>.vtt`
- 内嵌容器：单个 `<media_id>.esub`（自描述二进制容器，多语言轨，纳秒级时间）

两种交付物均由同一份规范化 Cue 模型渲染，杜绝两套版本各写各的。

## 用法

```bash
# 生成并立即回读校验；任何不一致直接失败，不产生可发布结果
go run ./cmd/subdeliver build -manifest examples/manifest.json -out dist

# 对已存在的交付物独立复验（CI/发布闸门使用）
go run ./cmd/subdeliver verify -manifest examples/manifest.json -out dist
```

## 阻断规则

校验器使用与渲染器相互独立的解析器回读每个产物，逐条 Cue 比较：

- `language`：语言标签（BCP 47，如 `zh`、`en-US`、`zh-Hant`）
- `start` / `end`：起止时间，统一纳秒表示，精度丢失也算漂移
- `text`：文本内容，先做 CRLF/行尾空白规范化再比较
- `cue_count` / `track_count` / `default`：条目数、轨数与默认轨标记

每条差异输出 `[deliverable] lang=.. cue=.. field=..: expected .., got ..`，
进程以退出码 1 结束，便于直接接入发布流水线。

## 源清单

见 `examples/manifest.json`。时间码支持 SRT（`HH:MM:SS,mmm`）与
WebVTT（`HH:MM:SS.mmm`，亦接受 `MM:SS.mmm`/`SS.mmm`）写法；加载时即校验
语言标签、时间顺序与空文本。

## 模块结构

```
cmd/subdeliver      CLI：build / verify
internal/manifest   清单加载、校验与规范化 Cue 构建（唯一数据源）
internal/subtitle   时间码解析/渲染、语言校验、文本规范化
internal/sidecar    SRT / WebVTT 渲染器与独立回读解析器
internal/embedded   ESUB 内嵌容器的序列化与独立反序列化
internal/verify     逐条语言/时间/文本比对与阻断报告
```

## 测试

```bash
go test ./...
go vet ./...
```

测试覆盖时间码往返、各格式回读、容器损坏、以及 sidecar 文本篡改与时间漂移的
端到端阻断场景。
