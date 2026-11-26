# 可重复媒体打包规范（mediabundle）

## 1. 问题定义

同一输入目录连续打包两次，文件哈希不同，通常由以下三类**非内容噪声**导致：

| 噪声类别 | 典型来源 | 是否代表媒体差异 |
| --- | --- | --- |
| 时间戳 | 打包时刻 `packedAt`、gzip 头 MTIME、tar 头 mtime | 否 |
| 容器顺序 | map 遍历、目录读取顺序、并发写入顺序 | 否 |
| 随机 ID | UUID v4、随机 nonce、临时文件名 | 否 |
| **真实媒体差异** | 文件内容增删改（SHA-256 变化） | **是** |
| **必要元数据差异** | 源文件 `mtime`、权限 `mode` 变化 | 否（媒体等价，但属真实元数据变化，必须保留并单独报告） |

原则：**通过规范化消除噪声，而不是删除必要元数据。** 源文件的 `mtime`
与权限是溯源与还原所必需的，必须原样保留；被消除的只是打包器自身注入的
环境噪声。

## 2. 容器布局

容器为 POSIX tar（可选确定性 gzip，扩展名 `.tar.gz`），固定结构：

1. 第一个条目恒为 `manifest.json`；
2. 其后按条目路径的字节序（lexicographic）排列所有载荷文件；
3. 路径统一使用 `/` 分隔，`path.Clean` 规范化，禁止 `..` 段；
4. 只打包常规文件，目录、符号链接、设备节点不进入容器。

## 3. 确定性规则

### 3.1 排序

- 清单 `entries` 与 tar 容器顺序均按 `path` 升序，与文件系统返回顺序无关。

### 3.2 时间戳

- 打包过程**不读取墙上时钟**；
- 载荷 tar 头 `mtime` = 源文件 mtime，归一化为 UTC 整秒（必要元数据，保留）；
- `manifest.json` 的 tar 头 `mtime` 固定为 Unix epoch（1970-01-01T00:00:00Z）；
- tar `atime`/`ctime` 留空（Go 零值），`uid/gid/uname/gname/devmajor/devminor` 全部为 0；
- gzip 头：`MTIME=0`、`NAME` 与 `COMMENT` 为空、压缩级别固定
  （`gzip.BestCompression`）、XFL/OS 由 Go 标准库固定输出。

### 3.3 标识

- `bundleId = "sha256:" + hex(sha256(canonicalJSON(entries)))`，
  完全由内容（含源 mtime/mode/哈希/媒体类型）派生，不含任何随机数；
- `canonicalJSON` 为紧凑 JSON（无缩进、无 HTML 转义），字段顺序由结构体定义固定；
- `sourceRoot` 为**可选**溯源标签，默认省略；仅当调用方显式传入
  `--source-root` 时写入，且不参与 `bundleId` 计算，因此检出路径不会污染字节。

### 3.4 媒体识别

- 通过文件魔数（PNG/JPEG/GIF/WebP/ISO-BMFF/Matroska/OGG/FLAC/ID3/WAV）
  判定 `mediaKind`（image/audio/video/other），不依赖扩展名；
- 每个条目记录独立的内容 SHA-256，差异判定以哈希为准，重命名无法掩盖内容变化。

## 4. 清单 schema

```json
{
  "manifestVersion": 1,
  "tool": "mediabundle",
  "bundleId": "sha256:<64 hex>",
  "entries": [
    {
      "path": "nested/a.txt",
      "size": 5,
      "mode": 420,
      "modTime": "2026-11-26T17:19:52Z",
      "sha256": "<64 hex>",
      "mediaKind": "other"
    }
  ]
}
```

`sourceRoot` 存在时追加在 `bundleId` 之后。schema 不兼容变更时提升
`manifestVersion`。

## 5. 差异裁决（`mediabundle diff`）

解析两个容器（自动识别 gzip），逐轴比较并给出唯一裁决：

| 裁决 | 条件 |
| --- | --- |
| `identical` | 容器字节级完全相同 |
| `media-different` | 存在内容哈希变化、新增或删除的载荷文件 |
| `metadata-different` | 媒体字节完全一致，但源 `mtime` 或 `mode` 不同 |
| `cosmetic-equivalent` | 媒体与必要元数据均一致，差异仅限打包时间戳、容器顺序或随机 ID |

报告同时分别列出：

- `packagingTimestampDiffs`：`manifest.packedAt`、`gzip.modTime`、
  `tar.manifest.modTime` 三类**打包注入**时间戳；
- `containerOrderDiff`：tar 中载荷条目顺序差异；
- `bundleIdDifference`：随机/派生标识差异；
- `sourceMetadataDiffs`：源文件 `modTime`/`mode`（必要元数据）；
- `mediaDiffs`：`content` / `added-in-b` / `removed-in-b`。

## 6. 验证方法

```sh
# 单元测试：可重复性、四类裁决、跨路径可移植、模式/mtime 元数据
go test ./...

# CLI 自检：同一目录打两次包，tar 与 gzip 哈希必须分别相等
go run ./cmd/mediabundle verify --src <目录> --gzip

# 人工对照：传统非确定打包器复现噪声，再用 diff 解释
go run ./cmd/mediabundle legacy-pack --src <目录> --out a.tar.gz --gzip
go run ./cmd/mediabundle legacy-pack --src <目录> --out b.tar.gz --gzip
go run ./cmd/mediabundle diff --a a.tar.gz --b b.tar.gz
```

`verify` 在任何一轮哈希不一致时以非零状态退出。

## 7. 明确不做的事

- 不删除、不冻结源文件 `mtime` 与权限（必要元数据，按 UTC 整秒保留）；
- 不通过「剥离所有元数据」换取相同哈希；
- 不在清单或容器中写入本机绝对路径、主机名、用户名；
- 不使用随机数、UUID、当前时间参与规范打包的任何字节。
