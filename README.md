# evidencechain — 改名、容器重排、局部篡改下的可验证取证证据链

纯 Go（**零第三方依赖，仅标准库**）实现的电子数据取证证据链工具箱。
它把**原始材料、派生副本、哈希索引、人工判读**串成可复算的证据链，
在文件改名、zip/tar 容器重排/跨容器迁移、局部字节篡改之后仍给出有置信边界的结论，
并从规则上禁止把**相似哈希/模糊哈希/哈希前缀**当成同一来源。

完整案例报告：[`FORENSIC-REPORT.md`](FORENSIC-REPORT.md)（由工具根据仓库内真实文件哈希生成）。

> **检材来源声明**：本工作区初始只有脚手架，没有真实案件检材。因此仓库内置了一个
> 完全确定性生成的**虚构教学/自检案例 `CASE-2026-1127`**（公司、人员、电话、编号均为合成）。
> 报告中的每个哈希、块差异与审计结论都是对仓库内实际文件计算所得，可一键复现；
> 同一套命令可直接用于真实检材（见下文“用于真实案件”）。

## 三十秒验证

```bash
bash scripts/offline-build.sh                 # GOPROXY=off 下 build + vet + test
go run ./cmd/evidencechain verify --case case # 校验完整证据链与人工判读
go run ./cmd/evidencechain report --case case --out FORENSIC-REPORT.md
```

根目录也有薄入口，`go build .` / `go run . verify --case case` 同样可用。
证据链自洽时 `verify` 退出码为 `0`；封印被破坏、保管链断裂或人工判读过度断言时退出码为 `3`。

## 四类难题与对策

| 难题 | 单纯整文件 SHA-256 的问题 | 本工具对策 |
|---|---|---|
| 文件改名/改扩展名 | 路径对不上就“失联” | 只按完整 SHA-256 认定内容，名字仅记录为 `renamed_from` |
| 容器重排/改名/跨 zip·tar 迁移 | 重打包后整包哈希全变 | 成员级哈希 + 与顺序/名称无关的 `memberset_sha256` 规范摘要 |
| 局部篡改 | 整文件哈希只告诉你“不同” | 索引保存对齐块哈希与块根，输出失配块与字节区间 |
| 相似哈希诱饵 | 前缀相同/模糊指纹接近极易被误报为同源 | 完整 64 位值才命中；前缀判 `rejected-prefix-only`；SimHash/Jaccard 仅作线索并审计人工结论 |

参考案例 `case/30-field-returns/` 同时埋入了：改名文件、乱序改名的 zip、
跨格式 tar 子集、等长局部篡改件（loose 与压缩包成员各一）、高相似“同题仿作”、
未知随机件，以及含“同前缀诱饵哈希”的协查通报。

## 证据链分层

| 层 | 路径 | 作用 |
|---|---|---|
| 原始材料（信任根） | `case/00-originals/` | 建索引后只读；每次验证重算 SHA-256 封印 |
| 派生副本 | `case/10-derived/`、`case/20-containers/` | 字节副本/EOL 规范化/PNG 重编码/封存包，关系记入 `custody/provenance.json` |
| 哈希索引 | `case/index/hash-index.json` | 全量 SHA-256、对齐块、块根、ecsh1 模糊指纹 |
| 人工判读 | `case/review/judgments.json`、`case/review/analyst-notes.md` | 结论/置信度/依据/不确定项；机器反向审计 |
| 回传检材 | `case/30-field-returns/` | 改名、重排、篡改、仿作、未知件 |
| 外部协查 | `case/watchlist/reference-hashes.json` | 完整命中、前缀诱饵、模糊指纹、无命中四类 |
| 机器结论 | `case/out/verification.json` | 结构化验证结果 |

## 哈希层级与“相似哈希”政策

1. **完整 SHA-256**：64 位十六进制全值相等才证明位级同一；截断值/前缀永不匹配
   （见 `internal/digestx` 与 `EqualDigest`）。
2. **对齐块 SHA-256 + 块根**：固定偏移分块，把局部篡改定位到块与字节区间；
   块根用域分隔串 `EVIDENCECHAIN/CHUNKROOT/v1` 绑定块序。
3. **ecsh1 相似指纹**：8 字节 shingle 的 Jaccard 重叠率 + 64 位 SimHash 汉明距离。
   阈值 `25%` 只表示“值得人工比对”，**不是同一/同源判据**。
4. **内容级派生校验**：EOL 规范化可确定性复算；PNG 重编码通过解码后逐像素比对确认。

判定纪律由 `internal/chain/judge.go` 强制执行：

- `identity-confirmed` / `rename-confirmed` / `watchlist-hit` 必须存在完整哈希精确命中事实；
- `tamper-localized` 必须存在非空块差异；
- 仅凭模糊或前缀哈希宣称同一/同源 ⇒ 审计为 `overclaim`/`contradiction`，`verify` 退出码 `3`；
- 证据不足时使用 `uncertain-origin`，审计为 `uncertain-allowed` 并在报告中显式保留。

## CLI

```text
evidencechain init-case --dir case [--force]   # 生成确定性参考案例（不写 review/）
evidencechain index --case case --chunk-size 128
evidencechain verify --case case [--write-json case/out/verification.json]
evidencechain report --case case --out FORENSIC-REPORT.md
evidencechain hash <file> [--chunks N]          # SHA-256/块根/ecsh1
evidencechain container <archive.zip|.tar>      # 成员与 memberset/pathset 摘要
evidencechain compare <a> <b>                   # Jaccard + SimHash 距离
```

## 离线构建

模块没有任何 `require`，仅导入标准库，因此可完全离线：

```bash
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local CGO_ENABLED=0 go build ./...
GOPROXY=off go test ./...
# 或一键：
bash scripts/offline-build.sh
```

## 用于真实案件

1. 把原始检材放入一个新目录的 `00-originals/`（建议 `EV-编号_名称` 命名，编号会被读取）；
2. `evidencechain index --case <dir> --chunk-size 65536`（大文件用 64 KiB 或更大块）；
3. 把派生副本与处理步骤写入 `custody/provenance.json`，回传件放入 `30-field-returns/`，
   外部哈希写入 `watchlist/reference-hashes.json`；
4. 在 `review/judgments.json` 记录人工结论与依据；
5. `evidencechain verify --case <dir>` 与 `evidencechain report` 出报告。

## 测试

`go test ./...` 覆盖：SHA-256 标准向量与前缀拒绝、篡改块定位、模糊相似特性、
zip/tar 规范摘要的顺序无关/改名敏感，以及一个完整端到端案例
（含“把未知件过度断言为同一”必须导致验证失败的反向用例）。

## 仍然不确定的结论（摘要）

- 高相似“同题仿作”是否与原件同源/抄袭：**不确定**，需元数据或传播链证据；
- 协查 WL-2 的同前缀哈希是否本想指向 EV-003：**不确定**，需核对完整通报值；
- 未知二进制的来源与用途、篡改行为人/设备/精确时间：**无法从哈希证据确定**；
- 固定分块对插入/删除会错位，精确定位能力有限；SHA-256 同一只是技术事实，
  不等同法律意义上的真实性或行为人归属。完整清单见报告第 9.5 节。
