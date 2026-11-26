# 崩溃安全的批量渲染：原子输出、阶段提交与事务日志

## 问题模型

批量渲染在中途失败时，旧实现会留下两类脏状态：

- 清单已标记「部分完成」，但对应文件只写了一半，消费者无法区分完整文件与半成品。
- 重试无法判断哪些片段已经成功，只能整批重跑；而重跑如果引入时间戳/随机内容，哈希还会漂移。

本设计把每个输出文件拆成独立片段，用「暂存 + 事务日志 + 原子发布」把失败恢复变成确定性的状态机问题。

## 目录布局

```
<out>/
  lock                  # flock 排他锁，进程死亡由内核自动释放
  txn/txn.log           # append-only 事务日志（长度前缀 + CRC32 + fsync）
  stage/<file>.fragNNNN # 已 fsync 的片段暂存文件
  files/<path>          # 组装后原子发布的最终产物
  manifest.json         # 原子替换的提交清单（含自摘要）
```

`files/` 下永远只出现完整文件：半成品只存在于 `stage/`，消费者只读 `files/` 与 `manifest.json`。

## 阶段提交状态机

每个片段与每个文件都沿单向状态迁移，状态来源只有 WAL：

1. `batch_open`：写入批次 ID（取规范化 spec 哈希前 16 位）与 spec 哈希。
2. 片段阶段（每个片段独立）：
   1. 纯函数渲染片段字节；
   2. 写入 `stage/` 同级临时文件，`fsync` 后 `rename` 到正式片段名，并 `fsync` 目录——原子且耐久；
   3. 追加 `fragment_commit(path, index, size, sha256)` 并 `fsync`。
3. 文件阶段：全部片段到位后，顺序拼接并实时计算整文件 SHA-256，临时文件 `fsync` 后原子 `rename` 到 `files/<path>`，再追加 `file_commit`。
4. 批次阶段：先原子替换 `manifest.json`，再追加 `batch_commit(manifest_digest)`。

日志先于或等于对应副作用持久化：任何崩溃点之后重放，投影都不会声称存在未耐久的数据。

## 恢复与「只重试未完成片段」

启动时先获取 `flock`，然后重放 WAL 重建内存投影：

- `fragment_commit` 中存在的片段直接跳过，不重新渲染；
- 已 `file_commit` 的文件整体跳过；
- 已 `batch_commit` 时直接返回现有清单，整个运行是零写入的 no-op；
- 重放遇到撕裂尾记录（崩溃时的半截 append）时，在有效字节处截断日志再继续，完整记录不受影响。

因此重试成本与「真正缺失的片段数」成正比，而不是与批次大小成正比。

## 哈希一致性（重复执行幂等）

幂等性由三个约束共同保证：

- 渲染器是纯函数：`DeterministicRenderer` 仅由文件路径、片段序号和确定性 PRNG（splitmix64）派生字节，不接触时间、随机源或环境。
- 规范化哈希：spec 先按路径排序再做 JSON 哈希；清单按路径排序，摘要只覆盖除 `digest` 外的规范化 JSON，字段顺序固定。
- 片段与整文件都记录 SHA-256；`verify` 会重新渲染全部片段并逐一比对，证明重跑产物逐字节相同。

同一 spec 在任意全新目录、任意崩溃/重试序列后，`manifest.json` 逐字节一致。

## WAL 记录格式

每条记录为一帧：`[4 字节大端长度][4 字节 CRC32(IEEE)][JSON 负载]`，追加后立即 `fsync`。
CRC 防止静默损坏，长度前缀保证可以精确找到撕裂边界；不引入任何外部依赖。

## 失败语义与并发

- `flock(LOCK_EX|LOCK_NB)` 阻止两个引擎并发写同一输出目录；进程异常退出时内核自动释放，无需陈旧锁清理。
- 所有发布动作都是「临时文件 + fsync + rename + 目录 fsync」，读者只能看到旧文件或完整新文件。
- spec 路径经 `ValidatePath` 校验，拒绝绝对路径与 `..` 穿越，片段名中的分隔符被编码为 `__`。
- spec 哈希不匹配时拒绝在同一输出目录运行，防止不同任务互相污染。

## CLI

```
renderbatch run     --spec spec.json --out ./out [--fail path#index=N]... [--crash-after HOOK]
renderbatch verify  --out ./out [--spec spec.json]   # 校验磁盘哈希，可全量重渲染比对
renderbatch inspect --out ./out                       # 输出 WAL 投影与清单状态
renderbatch recover --spec spec.json --out ./out      # 显式恢复（等价于无注入的 run）
```

## 测试覆盖

`internal/batch` 下的测试覆盖：

- 正常提交与清单自摘要校验；
- 崩溃后恢复，且只渲染缺失片段、再次运行为零渲染；
- 每个崩溃钩子点（片段暂存后、片段日志后、组装后、文件日志后、清单写入后）均可恢复且摘要不变；
- 瞬时渲染故障（FlakyRenderer）重试后与干净运行哈希一致；
- 独立目录与 spec 重排的逐字节幂等；
- WAL 撕裂尾截断重放、排他锁竞争、危险路径拒绝。
