# 多机位婚礼素材对齐

纯 Go 标准库实现。输入六机位的机器、镜头、片段、拍手观测，输出每条机位在
母时间线上仍可覆盖的区间。每台相机的机内时间可以完全不同；片段之间没有素材
的空档会保留，不会用相邻片段或团队偏移补齐。

## 运行

```bash
go run ./cmd/multicam -in examples/wedding.json -pretty
go test ./...
```

也可以从标准输入读取，默认向标准输出写 JSON：

```bash
go run ./cmd/multicam < examples/wedding.json
```

## 四层映射

```text
sequence
└── machine               一个机位 / 相机机身
    └── lens              该机位在序列中使用的镜头
        └── clip          机内时间轴上的连续录制片段
```

- `sequence.id` 绑定序列；所有片段的 `sequence_id` 必须与它一致。
- `lens.machine_id` 指定镜头所属机器。
- `clip.machine_id`、`clip.lens_id` 和 `clip.sequence_id` 必须同时匹配。
- 同一机位可换镜头；输出按镜头分组，同时给出该机位合并后的总覆盖。

## 输入时间基准

母时间线区间为 `[0, master_duration_seconds)`，原点按使用约定取共同拍手位置。
`clip.start_seconds`、`clip.end_seconds` 和
`clap.observations[].local_seconds` 都使用该相机自己的连续机内时间，不使用
统一时间码。若相机分文件后重置时间，应在导入前把片段换算到同一机内基准。

`claps[].master_seconds` 是拍手在母时间线上的位置。同一段婚礼可以录入多次
共同拍手，以检验时钟漂移或误检。

## 对齐与容差

对每一路机器独立计算：

```text
单次观测偏移 = clap.master_seconds - observation.local_seconds
通道偏移     = 所有单次观测偏移的中位数
残差         = |clap.master_seconds - (local_seconds + 通道偏移)|
最大误差     = max(观测 error_seconds, 残差)
```

只有 `最大误差 <= sequence.tolerance_seconds` 时才应用该路偏移。超限时输出：

- `status: "tolerance_exceeded"`
- `alignment.applied: false`
- `coverage: []`
- 每个片段的 `master_interval` 和 `coverage_interval` 均省略

实现中没有团队/多机位中位数偏移回退，因此某一路失败不会借用其他机位的偏移。
没有该机位拍手观测时输出 `status: "missing_clap"`，同样不套用任何偏移。

## 输出覆盖

每路先用 `local_interval + offset` 投影到母时间线，再裁到
`[0, master_duration_seconds)`。重叠或首尾相接的区间会合并，真实停机的空档保留
为多个区间。示例中 `cam-b` 的 60 秒停机输出为：

```json
"coverage": [
  {"start_seconds": 0, "end_seconds": 60},
  {"start_seconds": 120, "end_seconds": 360}
]
```

区间是半开区间 `[start_seconds, end_seconds)`。输出同时包含
`covered_seconds`、逐镜头片段映射和每路对齐证据，便于追查是哪台机器、哪支
镜头、哪个片段贡献了覆盖。

示例中的 `cam-f` 两次拍手相差 0.20 秒，中位数残差为 0.10 秒，超过 0.08 秒
容差；它会以 `tolerance_exceeded` 返回，不会被自动对齐。

## JSON 输入

完整六机位样例见 `examples/wedding.json`。顶层字段为：

- `sequence`：序列、母时间线长度和容差。
- `machines`：相机机身。
- `lenses`：镜头及其所属机身。
- `clips`：片段、所属序列/机身/镜头及机内起止秒。
- `claps`：共同拍手在母时间线的位置，以及各机位的机内观测和测量误差。

JSON 解码是严格的：未知字段和多余 JSON 文档会直接报错，避免元数据拼写错误
被静默忽略。
