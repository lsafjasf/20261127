# 媒体交付合同逐项验证器

不同平台对帧率、码率、字幕、音频布局和文件名的要求各不相同，人工检查容易
漏项。本项目把平台要求表达为**机器可读合同（JSON）**，由验证器对每个检查项
独立给出三类结论：

| 状态 | 含义 |
| --- | --- |
| `PASS` | 探针测到的值满足合同 |
| `FAIL` | 探针测到的值明确违反合同 |
| `UNKNOWN` | 探针缺少判定所需信息（如未报码率/语言/布局），不可判定 |

关键原则：**缺数据只能是 `UNKNOWN`，不能误报 `FAIL`**；总体结论按
`FAIL` > `UNKNOWN` > `PASS` 聚合。

## 目录结构

```
contract/          合同类型、JSON 加载与合同自校验
media/             工具中立的探针模型 + ffprobe 适配器
checker/           逐项验证器与三态报告
cmd/mediaqacli/    命令行入口
examples/
  contracts/       两个平台的示例合同
  probes/          good / failing / undecidable 三个探针样本
```

## 合同字段

- `video`：必备性、允许的编码器、精确帧率（分数 + 容差）、码率上下限
- `audio`：流数量上下限、编码器、声道布局、码率范围
- `subtitles`：必备性、字幕编码器、语言代码
- `filename`：正则、扩展名、最大长度
- `containers`：允许的容器格式（支持 ffprobe 逗号分隔别名中的任意一个）

所有范围均为闭区间，零值表示“不约束”，因此可选规则可直接从 JSON 省略。
合同在加载时先做自校验（非法版本号、倒挂区间、非法正则等会被直接拒绝）。

## 使用

```bash
# 直接调用 ffprobe 检查真实文件
go run ./cmd/mediaqacli \
  -contract examples/contracts/streaming-hd.json \
  -media /path/to/movie.mp4

# 或使用预先导出的工具中立探针 JSON
go run ./cmd/mediaqacli \
  -contract examples/contracts/streaming-hd.json \
  -probe examples/probes/good.json
```

退出码：`0` 全部通过；`1` 存在失败项；`2` 无失败但有不可判定项；`3` 用法或
输入错误。报告输出到 stdout 的 JSON，汇总行输出到 stderr，便于流水线采集。

## 三态示例

```bash
go run ./cmd/mediaqacli -contract examples/contracts/streaming-hd.json \
  -probe examples/probes/good.json        # 13 pass, overall=PASS
go run ./cmd/mediaqacli -contract examples/contracts/streaming-hd.json \
  -probe examples/probes/failing.json     # 含 FAIL, overall=FAIL
go run ./cmd/mediaqacli -contract examples/contracts/streaming-hd.json \
  -probe examples/probes/undecidable.json # 4 unknown, overall=UNKNOWN
```

## 测试

```bash
go test ./...
```

测试覆盖帧率容差、码率缺失不可判定、字幕必备性失败、声道布局归一化与回退、
流数量边界、文件名规则、FAIL 优先于 UNKNOWN 的聚合，以及 ffprobe JSON 适配。

## 扩展新探针

验证器只依赖 `media.Probe` 中立模型。接入新探针时实现一个把厂商输出翻译成
`media.Probe` 的适配器即可（参考 `media/ffprobe.go`），验证器与合同无需改动。
