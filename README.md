# 20261127

Repository initialized for 20261127.

## matchframe

从成片反查镜头原始 source 的小工具（Go 标准库，无第三方依赖）。

- 按序列时间反推源时间：每个片段用 `srcIn/srcOut` 相对 `seqIn/seqOut`
  的线性映射表示任意恒速变速（`srcOut < srcIn` 即倒放），
  `MapSeqToSrc` 把时间线帧号换算回源帧号。
- 首尾帧指纹缩小候选：片段记录首/尾时间线帧的 dHash 指纹，
  只有源文件在**两个**反推源帧位置上的指纹都吻合才算候选；
  文件名绝不参与判定（`nameAgrees` 仅供参考）。
- 不能唯一匹配的片段（0 个或 ≥2 个候选）单独列在 stderr 汇总里。

```sh
go run . match testdata/sequence.json testdata/pool.json   # 全片匹配报告
go run . map testdata/sequence.json testdata/pool.json 60  # 单帧反查
go run . fingerprint frame001.png                          # 计算帧指纹
go test ./...
```

输入格式见 `matchframe -h`；示例数据 `testdata/` 覆盖同名文件、
2x 变速、倒放、指纹歧义、无法匹配五种情形。
