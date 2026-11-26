# 20261127

Repository initialized for 20261127.

## mediabundle：可重复媒体打包与差异分类

同一输入连续打包两次哈希不同，根因通常不是媒体变化，而是打包器注入的
**时间戳、容器顺序、随机 ID**。本仓库用 Go 实现一个确定性打包器和差异
分类器，在**保留全部必要元数据**（源文件 mtime、权限、内容哈希、媒体类型）
的前提下实现字节级可重复输出。完整契约见 [SPEC.md](SPEC.md)。

### 快速开始

```sh
go test ./...                                   # 单元测试与裁决矩阵
go run ./cmd/mediabundle verify --src <目录> --gzip   # 可重复性自检
./scripts/demo.sh                               # 端到端演示（含问题复现）
```

### 子命令

- `pack --src DIR --out FILE [--gzip]`：规范打包，重复执行哈希恒定；
- `legacy-pack`：故意非确定的对照打包器（UUID + 洗牌 + 墙上时钟），用于复现问题；
- `diff --a A --b B [--json]`：逐轴分类，输出 `identical` /
  `cosmetic-equivalent` / `metadata-different` / `media-different` 裁决；
- `verify`：同目录打两次包并比对哈希，失败时非零退出；
- `inspect --bundle FILE`：查看清单、容器顺序与条目元数据。

### 代码布局

- `internal/bundle/snapshot.go`：快照、清单与内容派生 BundleID；
- `internal/bundle/pack.go`：确定性 tar/gzip 打包；
- `internal/bundle/legacy.go`：非确定对照实现；
- `internal/bundle/diff.go`：四类差异裁决；
- `internal/bundle/read.go` / `media.go`：容器解析与魔数媒体识别；
- `cmd/mediabundle/main.go`：CLI。
