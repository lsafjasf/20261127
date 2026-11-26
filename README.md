# streamcheck

分层比较**容器元数据、码流 CRC、解码结果、像素校验**的 Go 工具。它不把“CRC 通过”解释成“内容正确”，并将“异常定位”和“相同像素是否可重构”分开判定。

> 当前工作区没有实际码流、容器或校验报告，因此不能负责任地说出真实最早帧号。运行工具并传入真实证据 JSON 后可得到具体帧号；`examples/incident.example.json` 只是按题述构造的样例。

## 快速运行

```bash
go test ./...
go run ./cmd/streamcheck examples/incident.example.json
go run ./cmd/streamcheck -format json examples/incident.example.json
cat evidence.json | go run ./cmd/streamcheck -
```

## 分层方法

1. `container`：MP4 box、Matroska/WebM element、sample table、offset/size、PTS/DTS、duration、keyframe flag 等。
2. `bitstream_crc`：码流或包级 CRC/校验值，必须记录校验覆盖的字节范围和期望值来源。
3. `decode`：解码器错误、警告、丢帧、错误隐藏和参考帧管理事件。
4. `pixels`：解码后原始像素的可信 MD5/SHA-256/framehash，或逐像素比较结果。

报告同时给出：

- `earliest`：按解码/因果序找到的最早 `fail`，同帧按容器 → 码流 → 解码 → 像素排序。
- `earliest_display`：按 `display_order`、PTS 或解码序得到的显示时间线最早异常。
- `earliest_by_layer`：每层各自的最早失败帧。

P/B 帧或带参考关系的帧不能只按显示帧号判断影响面；还需要参考帧依赖和后续 IDR/keyframe 位置。
可在 `frame.references` 中列出该帧直接参考的帧 key；工具会沿参考图计算 `impacted_by_failures`。祖先帧硬失败时，下游帧即使自身 CRC 通过，也不能据此排除错误传播。

## 证据 JSON

```json
{
  "observations": [
    {
      "layer": "bitstream_crc",
      "status": "fail",
      "check": "slice_crc",
      "expected": "9f2c",
      "actual": "21a7",
      "coverage": ["NAL bytes covered by slice CRC"],
      "frame": {"key": "frame-0012", "decode_order": 12}
    },
    {
      "layer": "pixels",
      "status": "fail",
      "trusted_reference": true,
      "check": "sha256_framehash",
      "frame": {"key": "frame-0012", "decode_order": 12}
    }
  ],
  "recovery": [
    {"frame_key": "frame-0012", "good_packet_available": true, "bit_exact_decoder": true}
  ]
}
```

字段约束：

- `layer`：`container`、`bitstream_crc`、`decode`、`pixels`。
- `status`：`pass`、`fail`、`warn`、`unknown`。
- 像素 `pass` 只有在 `trusted_reference: true` 时才可作为“像素已相同”的结论。
- `recovery` 用来说明是否存在独立好源；工具不会假设存在备份、FEC 或冗余元数据。

## 判定矩阵

| 证据组合 | 对相同像素的结论 |
| --- | --- |
| 仅容器校验失败；码流/解码通过，且像素与可信基准一致 | `yes`：像素已正确，无损重封装或修元数据即可 |
| CRC 失败且可信像素校验失败，当前文件无独立好源 | `no_from_current_evidence`：当前码流/CRC 不能反推原始像素 |
| CRC 通过但像素校验失败 | 仍按内容错误处理；排查覆盖范围、陈旧期望值、CRC 字段、归一化、解码器确定性/碰撞 |
| CRC 通过但没有可信像素校验 | `unknown_needs_trusted_source`：CRC 不能证明像素相同 |
| 有独立已知良好像素 | `yes`：直接复制可信像素 |
| 有独立已知良好码包，且解码器位精确 | `yes`：用好包重解 |
| 只有好码包但未保证解码器位精确 | `unknown_needs_trusted_source` |

## 采集真实证据

```bash
ffprobe -v error -show_format -show_streams -show_packets -of json input.container > ffprobe.json
ffmpeg -v info -i input.container -f framehash -hash sha256 decoded.sha256
ffmpeg -v error -i input.container -f null - 2> decode-errors.log
```

若有可信母版或原始输出，再对同一时间基、像素格式、裁剪和缩放规则生成 framehash 并比较。码流 CRC 应使用编码格式或设备实际规定的覆盖范围验证；不要把容器 CRC、传输包 CRC 和码流内部 CRC 混为一层。
