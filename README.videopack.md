# videopack

多语言视频交付包构建工具（纯 Go 标准库实现）。音频、字幕、章节、封面
可以来自互不相同的源目录，工具负责扫描、构建轨道矩阵、处理重复语言与
缺失字幕，并把全部产物以相对路径写入一个可移植交付包。

## 交付包结构

```
out/
├── manifest.json          # 清单与轨道矩阵，只含相对路径
├── audio/<lang>/<file>
├── subtitles/<lang>/<file>
├── chapters/<lang>/<file>
└── cover/<lang>/<file>
```

`manifest.json` 不记录任何源机器绝对路径，文件条目带 SHA-256，可整包
拷贝或打 zip 后在其他机器上校验。

## 用法

```bash
go run ./cmd/videopack \
  --video-id vid-001 \
  --audio-dir    /data/audio \
  --subtitle-dir /data/subs \
  --chapter-dir  /data/chapters \
  --cover-dir    /data/covers \
  --out /dist/vid-001 \
  --zip /dist/vid-001.zip
```

### 参数

- `--on-duplicate`：同类型同语言出现多个文件时，`prefer`（默认，按
  编码优先级 > 文件大小 > 路径裁决）或 `error`（直接失败）。
- `--on-missing-subtitle`：某音频语言没有字幕时，`warn`（默认，写入
  清单警告）或 `fail`（打包失败）。
- `--include-duplicates`：默认未入选的重复文件不进包；加上该参数会
  一并复制，并在清单中标记 `duplicate`。
- `--force`：覆盖已存在的输出目录或 zip。
- `--metadata key=value`：附加元数据，可重复传入。

语言标签从文件名解析（如 `movie.en-US.ass` → `en-US`），无法识别时
归为 `und`。

## 原子性与回滚

打包在输出目录旁的临时暂存目录中进行：扫描 → 矩阵裁决 → 复制并计算
哈希 → 写清单 → 可选 zip → 单次 `rename` 提交。任何步骤失败都会删除
暂存目录和半成品，最终输出路径保持“不存在或完整”两种状态，不会留下
部分交付包。

## 测试

```bash
go test ./...
```

测试覆盖重复语言裁决、缺失字幕策略、相对路径可移植性、复制失败回滚、
输出冲突与 zip 条目。
