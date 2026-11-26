# 20261127

Repository initialized for 20261127.

## 渲染机代理回收池（`pool` 包）

渲染机池会回收旧代理，但同名文件可能来自不同输入版本，按文件名或修改时间
清理会误删仍被引用的文件。本包只用 Go 标准库实现，以**输入内容指纹**为身份，
配合租约、引用计数和两阶段回收解决该问题。

### 核心机制

- 输入指纹：对输入字节计算 SHA-256（`FingerprintBytes`）。同名不同输入得到不同
  指纹，对应不同 blob；相同内容自动去重。全程**不读取或比较文件修改时间**。
- 租约（Lease）：`AcquireLease`/`RenewLease`/`ReleaseLease`，持绝对过期时间戳并
  持久化；租约期内的代理即使被标记回收也不会删除。
- 引用计数：由持久化的依赖图边集合推导（`AddReference`/`RemoveReference`），
  计数即入边数量，重开池时从图重建，不存在计数漂移。
- 待回收状态：`Retire` 仅把代理标记为 `retired`；`Sweep` 只删除同时满足
  “已退休 + 引用计数为 0 + 无有效租约”的代理，并沿依赖图级联释放边。
- 崩溃恢复：`Open` 时执行 `reconcile`——清理过期租约、剔除 blob 缺失的记录及
  悬空边、重建引用计数、删除无记录的孤儿 blob（崩溃在写 blob 与提交状态之间的
  典型残留），并返回 `RecoveryReport`。

### 持久性与并发

- 布局：`state.json`（元数据 + 依赖图）、`blobs/<指纹>`（内容 blob）、`lock`（进程锁）。
- 状态写入走临时文件 + `rename` 原子替换；blob 同样先写临时文件再改名，崩溃只会
  留下可被恢复流程清理的临时/孤儿文件。
- 进程内用 `sync.RWMutex`，跨进程用 `flock`（Unix，进程退出由内核自动释放，
  崩溃不留死锁）；非 Unix 平台有基于独占文件的回退实现。

### 典型流程

```go
p, report, err := pool.Open("/var/cache/render-pool") // report 描述崩溃修复
defer p.Close()

proxy, _ := p.Put("scene/color.dat", inputBytes)     // 身份=输入指纹
lease, _ := p.AcquireLease(proxy.Fingerprint, time.Hour)
_ = p.AddReference(parentFP, proxy.Fingerprint)      // 被引用即受保护

p.Retire(proxy.Fingerprint)                           // 标记待回收，不立即删除
removed, _ := p.Sweep()                               // 仅清无引用且无租约者
_ = p.ReleaseLease(proxy.Fingerprint, lease.ID)
```

### 测试

`go test -race ./...` 覆盖：同名多版本隔离、去重、不依赖 mtime、租约过期与保护、
引用计数阻断回收、级联 Sweep、并发压力，以及三类崩溃恢复（过期租约、孤儿 blob、
blob 缺失与悬空边）。
