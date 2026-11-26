# HDR 切点 AU/GOP 追踪器（仅 Go 标准库）

按访问单元（Access Unit, AU）追踪 `transfer_characteristics`、峰值亮度元数据和
色域基色，而不是读取首帧或容器里的单一全局标签。内置复现把十分钟切点放在
30 fps 的 AU `18000`：前一个 IDR 是 AU `17940`（598.000s），后一个 IDR 是
AU `18060`（602.000s），因此切点位于 GOP 内，偏移 60 个 AU。

题面没有给出真实峰值和色域坐标，所以不能只凭 “HLG/PQ” 名字推断母版峰值。
工具把“传递函数表示范围”“ST 2086 母版音量”“MaxCLL/MaxFALL 实测值”分开；
内置样例采用常见合同：切点前 HLG/BT.2020/1000 nits，切点后
PQ/BT.2020/4000 nits。`-sample gamut` 另演示切点后改为 P3-D65。

## 先判定 GOP 风险

运行：

```bash
go test ./...
go run ./cmd/hdrswitch -plan
go run ./cmd/hdrswitch -in examples/hlg_to_pq_inside_gop.json -check
```

风险样本的第一份输出就是：

```text
[error] switch_inside_gop: switch AU 18000 at 600.000000s lies inside GOP
"gop-17940" (GOP start AU 17940, offset 60 AU)
[error] player_sticky_first_frame_metadata: ... first stale AU=18000
```

这时不能先改全局 HLG/PQ 标签。P/B AU 仍可能引用切点前的参考图像，播放器或
解码器也可能继续使用首个 CVS/首帧的 VUI/SEI。正确顺序是：

1. 报告切点落在 GOP 内，给出前/后 IDR 或 RAP；
2. 在精确的 600s 处重编码出 IDR/CVS，并确认 closed GOP、无跨边界参考；
3. 在该 IDR 写新的 SPS/VUI（`transfer_characteristics=16`、
   `colour_primaries=9` 等）、PPS、mastering display / CLL SEI；
4. 让播放器在新 CVS/IDR 重置颜色元数据状态，逐 AU 接收新值；
5. 若不重编码，只能把切换点移到已有 IDR/RAP（样例中为 598s 或 602s），
   这会改变创作切点，必须单独确认。

非 IDR 的 CRA/BLA 只有在 closed GOP、无跨边界参考/RASL 依赖且播放链路验证
通过时才可能用于拼接；默认策略 `policy.require_idr=true` 会将其判为不安全。

## 切点前后的合法范围

| 层级 | 600s 前：HLG | 600s 后：PQ |
| --- | --- | --- |
| H.273 transfer | `18` / ARIB STD-B67 HLG | `16` / SMPTE ST 2084 PQ |
| 10-bit limited Y/R/G/B | code `64..940` | code `64..940` |
| 10-bit limited Cb/Cr | code `64..960`，中性点 `512` | code `64..960`，中性点 `512` |
| 10-bit full range | code `0..1023` | code `0..1023` |
| 非线性信号 `E'` | `[0,1]` | `[0,1]` |
| 线性/场景域 | 场景线性 `E ∈ [0,1]` | PQ 归一化亮度 `0..1` |
| 绝对亮度 | HLG 不固定绝对 nit；按声明 `Lw` 适配 | `E'=1` 固定为 `10000 cd/m²` |
| 样例声明峰值 | `0.005..1000 nits`，`Lw=1000` 时 gamma≈1.2 | PQ 表示上限 `10000`；样例母版/MaxCLL `4000 nits` |
| 典型色域 | BT.2020 / primaries `9` | BT.2020 `9`；AU 标 P3-D65 时以 AU 为准 |
| CIE xy / SEI 坐标 | `xy ∈ [0,1]`；SEI `0..50000`，单位 `1/50000` | 同左 |

关键区分：

- `64..940/960` 是窄范围量化码段；`941..1023` 等码值可能仍可编码，但不是
  nominal/legal picture level。
- PQ 的标准表示范围永远是 `0..10000 nits`；`MaxCLL=4000` 或 ST 2086
  max mastering luminance 是该标题/母版的更窄声明音量，不是 PQ 全部范围。
- HLG 是场景参考，没有 PQ 式固定绝对峰值。`1000 nits` 来自母版/合同或
  `nominal_peak_nits`；不能把“HLG=10000 nits”当成标准固定上限。
- 色域和传递函数相互独立。HLG/PQ 都可搭配 BT.2020、P3 或 BT.709；必须读取
  每个 AU/CVS 生效的 `colour_primaries` 与 ST 2086 基色坐标。
- CIE xy 的 `[0,1]` 只是坐标编码边界；真实颜色还必须位于光谱轨迹和
  R/G/B 主色三角形内。

## CLI

```bash
# 默认：内置 HLG -> PQ、切点落在 GOP 中的风险样本
go run ./cmd/hdrswitch -plan

# 切点正好是带参数集更新的 IDR；GOP 安全，但若播放器仍粘首帧则 -check 失败
go run ./cmd/hdrswitch -sample safe

# GOP/参数集/播放器都正确的目标状态；-check 退出 0
go run ./cmd/hdrswitch -sample valid -check

# IDR 安全且 BT.2020 -> P3-D65 的色域变化样本
go run ./cmd/hdrswitch -sample gamut -json

# 读取真实 AU/SEI/VUI 导出的 JSON
go run ./cmd/hdrswitch -in evidence.json -json
go run ./cmd/hdrswitch -in evidence.json -plan -plan-out repair.json
go run ./cmd/hdrswitch -in evidence.json -check
```

`-check` 在存在 error 级发现时以状态码 `1` 退出，可用于交付前质检。JSON 报告的
核心字段为：

- `access_units[]`：每个 AU 的精确 PTS、segment、GOP/IDR/RAP、继承后的元数据、
  字段级变化和播放器是否仍使用旧值；
- `gop_risk`：切点 AU、GOP 起点、AU 偏移、开放 GOP/跨边界参考、前/后 RAP；
- `before.legal_range` / `after.legal_range`：切点两侧的码段、EOTF/OOTF、
  峰值和色域范围；
- `regimes[]`：若同一侧还有额外峰值、传递函数或色域变化，按稳定元数据区间
  分别给出法律范围；
- `player`：首帧/全局粘性元数据与所有失配 AU；
- `findings[]`：按严重级别列出的结构、元数据和合同问题。

## 输入证据怎么来

一个 AU 记录至少需要 `index` 和 `pts`（或 `pts_ticks + time_base`）。元数据可只在
变化的 AU 上给出，工具会沿 AU 顺序继承并做字段级合并：
`access_units` 应按码流/解码顺序提供，以便正确判断 GOP 和跨参考关系；报告中的
切点和分段会按 PTS 排序，兼容 B 帧导致的显示顺序重排。

```json
{
  "switch_at": "600",
  "frame_rate": "30/1",
  "bit_depth": 10,
  "full_range": false,
  "timeline_complete": false,
  "player": { "uses_first_frame_metadata": true },
  "access_units": [
    {
      "index": 18000,
      "pts_ticks": 18000,
      "time_base": "1/30",
      "idr": false,
      "random_access": false,
      "gop_id": "gop-17982",
      "metadata": {
        "transfer": "PQ",
        "primaries": "BT.2020",
        "max_cll": "1000",
        "max_fall": "400",
        "mastering": {
          "max_luminance_nits": "1000",
          "min_luminance_nits": "0.005"
        }
      }
    }
  ]
}
```

对真实长片，应从码流解析或 side-data 导出以下事实，再汇总成该 JSON：

1. 每个 AU/frame 的 PTS、DTS、关键帧/RAP 类型、GOP 编号和参考关系；
2. SPS/VUI 中的 `colour_description_present_flag`、`colour_primaries`、
   `transfer_characteristics`、`matrix_coefficients` 和 full/limited range；
3. mastering display colour volume SEI 中的 RGB/白点坐标和亮度；
4. content light level information SEI 中的 MaxCLL、MaxFALL；
5. 播放器实际生效的元数据，或确认其只读取首帧/容器全局标签。

SEI 整数单位已在校验中处理：xy 坐标为 `value / 50000`，亮度为
`value / 10000 cd/m²`；MaxCLL/MaxFALL 的 JSON 直接使用 nits。ST 2086 的标称
母版最大亮度范围为 `[5,10000] nits`，最小亮度非零时标称范围为
`[0.0001,5] nits`。

## 边界

本模块只使用 Go 标准库，不调用 FFmpeg、Python 或外部媒体程序，也不直接解析
MP4/MKV 容器或 HEVC/AV1 二进制码流。仓库初始没有真实长片，因此它消费可由
码流探针、SEI/VUI 解析器或播放器日志生成的 AU 证据。仅有“十分钟从 HLG 变 PQ”
或容器标签变化，不足以判断 GOP 风险；必须提供切点 AU、RAP/IDR 和参考边界。

若证据显示切点是安全 IDR/CVS，工具仍会检查 SPS/VUI/SEI 更新和播放器粘性；
若切点在 GOP 内，`PlanRepair` 会保持 `safe_to_retag=false`，只输出
“先修 GOP/重编码/移动到 RAP”的计划。它不会把修改全局标签描述成修复。
