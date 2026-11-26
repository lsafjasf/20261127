# renderbatch

崩溃安全、可断点续跑的批量渲染引擎（Go 单模块，仅依赖标准库）。

核心机制：片段级暂存 + fsync、append-only 事务日志（CRC32 校验）、原子 `rename`
发布、原子替换的清单与自摘要。重试只处理 WAL 中未提交的片段，纯函数渲染保证
重复执行的哈希与清单逐字节一致。设计细节见 [DESIGN.md](DESIGN.md)。

## 快速开始

```bash
go test ./...
go build .

# 正常运行
./renderbatch run --spec examples/spec.json --out ./out

# 模拟在首个片段日志后崩溃，再恢复
./renderbatch run --spec examples/spec.json --out ./out --crash-after after_fragment_wal
./renderbatch run --spec examples/spec.json --out ./out

# 模拟瞬时渲染失败（同一片段失败 2 次后成功）
./renderbatch run --spec examples/spec.json --out ./out --fail 'report/detail.bin#0=2'

# 查看 WAL 投影 / 校验磁盘与全量重渲染哈希
./renderbatch inspect --out ./out
./renderbatch verify  --out ./out --spec examples/spec.json
```

## 代码结构

- `internal/batch`：引擎、WAL、原子 IO、文件锁、确定性渲染器、清单
- `internal/spec`：spec 加载与不变量校验
- `main.go` / `main_cmds.go`：`run`、`verify`、`inspect`、`recover` 子命令
- `examples/spec.json`：示例任务
