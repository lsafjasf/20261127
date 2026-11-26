# dvdiag：Dolby Vision 分段可用性诊断

`dvdiag` 用纯 Go 标准库读取 ISO BMFF/MP4 中的 Dolby Vision 小样，解析：

- `dvcC` / `dvvC`：`dv_profile`、`dv_level`、RPU/EL/BL 声明、兼容基座 CCID、元数据压缩；
- `hvcC` / `hvcE`：HEVC Main10、profile/level、VPS/SPS/PPS、附件流配置；
- `colr`、样本表和时间轴：分辨率、帧率、实际样本位置；
- HEVC NAL type 62：RPU 头、CRC32、推导 profile、EL/MEL 特征；
- RPU 的 `use_previous_rpu` / `previous_rpu_id` 引用链；
- `encv`、`sinf/frma/schm`、`pssh`：只识别受保护媒体，不读取、解密或绕过 DRM。

输出按“文件 → 视频轨道 → 样本描述”分段，先区分证据类别，再给出该段的完整 DV
条件和最低可用路径。

仓库不附带媒体样片；有输入文件时直接运行：

```bash
go build -o dvdiag ./cmd/dvdiag
./dvdiag sample-1.mp4 sample-2.mp4
./dvdiag -json sample-1.mp4 sample-2.mp4 > report.json
```

设备能力矩阵可选：

```bash
./dvdiag -caps devices.json sample-1.mp4 sample-2.mp4
```

## 三类问题

JSON 中的 `issues[].category`（文本中的 `findings:`）只使用以下分类，避免把不同原因混成一个“黑屏”结论：

| 类别 | 含义 | 典型代码 |
| --- | --- | --- |
| `container_missing` | 容器没有提供必要声明或配置 | `missing_dv_config`、`missing_hvcc`、`missing_sps`、`missing_colr` |
| `bitstream_conflict` | 容器声明与码流实际内容互相矛盾 | `rpu_profile_conflict`、`rpu_reference_missing`、`hevc_level_conflict`、`color_ccid_conflict` |
| `player_capability` | 文件自洽，但某台设备没有相应能力 | `dv_profile_unsupported`、`dv_el_unsupported`、`dv_entry_unsupported` |

另外会输出 `drm`（仅冻结在配置/信号层）和 `evidence`（扫描范围或限制说明）。

## 每段结论

`full_dolby_vision` 表示完整 DV 链是否成立：

| status | 含义 |
| --- | --- |
| `full_dv` | 容器、HEVC 配置和已扫描 RPU 引用链均一致 |
| `conditional` | 配置看起来成立，但 RPU 扫描未覆盖全部样本，或当前只支持 HEVC 的 RPU 校验 |
| `blocked` | 存在容器缺项、码流矛盾或 RPU 错误 |
| `not_standalone` | 这是只有 EL/RPU 的附件轨道，不能单独显示 |
| `auth_required` | 受 DRM 保护，需走授权解密路径；本工具不尝试解密 |

`minimum_availability` 表示“要让这段至少显示一幅正确画面，最低需要什么”：

| status | mode | 含义 |
| --- | --- | --- |
| `base_layer` | `hdr10` / `hlg` / `sdr` | 标准兼容基座可独立播放 |
| `dv_only` | `dolby_vision` | 没有标准兼容基座，必须有 DV 解码能力 |
| `auth_required` | `drm_authorization` | 必须先完成授权，静态码流不能证明可播 |
| `unusable` | `none` | 容器或码流没有可建立的有效路径 |
| `not_standalone` | `dolby_vision` | 附件流，需要与基座轨道组合 |

注意：`base_layer` 是“基座信号存在且标准兼容”。若样本描述仍是 `dvhe/dvh1`，
普通播放器可能不识别该 entry；此时置信度为 `conditional`，设备结果会再检查
`dv_entry_base_fallback`。

## 判定规则

### Profile 与兼容基座

| profile | 结构 | 常见 CCID | 无 DV 时的最低路径 |
| --- | --- | --- | --- |
| 4 | HEVC BL+EL+RPU | 2 | SDR 基座 |
| 5 | HEVC RPU-only，IPTPQc2 | 0 | 无，必须 DV |
| 7 | HEVC BL+EL+RPU，UHD Blu-ray | 6 | HDR10 基座（取决于 entry 支持） |
| 8 | HEVC RPU-only，兼容基座 | 1=HDR10，2=SDR，4/5=HLG | 对应 HDR10/SDR/HLG |
| 9 | AVC RPU-only | 2 | SDR 基座 |
| 10 | AV1 RPU-only | 0/1/4 | DV-only 或对应 HDR 基座 |
| 20 | HEVC Multiview | 0/1/4 | DV-only 或对应 HDR 基座 |

关键交叉检查：

- profile 5 必须是单层、CCID 0；若容器写成 `hvc1/hev1`，普通解码器会把 IPTPQc2
  基座当普通视频显示，常见结果是黑屏、绿紫或严重错色。
- profile 8.1/8.4 应使用兼容 entry `hvc1/hev1`；写成 `dvhe/dvh1` 会削弱普通播放器
  对 HDR10/HLG 基座的自动发现，判为信号矛盾（通常不是码流损坏）。
- profile 7 若 `el_present_flag=0`，只声明了基座；完整 DV 不成立，但 HDR10 基座仍可能可播。
- `hvcE` 只对 profile 7 有定义；出现 `hvcE` 但 EL 未声明会判为矛盾。
- `dv_version`、`dv_level`、HEVC `hvcC/SPS` profile/level、`colr` 传递函数都必须自洽。
- `dv_level` 会按宽度、高度和平均帧率检查像素率上限；码率上限不通过样本表估算。

### RPU 引用

每段从第一个被扫描的样本建立 mapping 锚点。若第一个 RPU 就使用
`use_previous_rpu=1`，或引用一个本段尚未出现的 `previous_rpu_id`，判为
`rpu_reference_missing`：这会让独立分段/随机接入失去可解码元数据。
CRC32 不匹配、`rpu_format`/位深异常、RPU 推导 profile 与容器不同，都会阻断
`full_dv`，但标准兼容基座仍可单独评估。

## 设备能力 JSON

文件可以是单个对象或对象数组：

```json
[
  {
    "name": "living-room-tv",
    "dolby_vision": true,
    "profiles": [5, 8],
    "max_level": 9,
    "compatibility_ids": [0, 1, 4],
    "supports_enhancement_layer": false,
    "supports_compressed_metadata": false,
    "sample_entries": ["dvh1", "hvc1"],
    "base_formats": ["hdr10", "hlg"],
    "hevc_main10": true,
    "max_hevc_level_idc": 153,
    "dv_entry_base_fallback": false
  },
  {
    "name": "hdr10-only-box",
    "dolby_vision": false,
    "sample_entries": ["hvc1"],
    "base_formats": ["hdr10"],
    "hevc_main10": true,
    "max_hevc_level_idc": 153
  }
]
```

空数组字段表示“不限制”：

- `profiles`：空表示支持全部 DV profile；非空必须包含流的 profile。
- `compatibility_ids`：空表示不按 CCID 限制。
- `sample_entries`：空表示不限制 entry 名称。
- `base_formats`：空表示不支持标准基座回退。

设备结果：

| status | 含义 |
| --- | --- |
| `full_dv` | 该设备可走完整 DV 路径 |
| `base_layer` | 该设备回退到 HDR10/HLG/SDR 基座 |
| `black` | 该设备既无 DV 路径，也无可用兼容基座 |
| `auth_required` | 需要授权 DRM 路径 |
| `inconclusive` | 能力满足但码流证据不足，不能下确定结论 |

## 扫描选项与限制

```bash
./dvdiag -scan-samples 0 -max-scan-mib 64 sample.mp4
```

- `-scan-samples 0` 扫描该段全部样本；也可限制到前 N 个样本。
- `-max-scan-mib` 限制每段读取的样本字节总量，避免大文件占用过多内存。
- 当前 RPU NAL 解析面向 HEVC；AVC/AV1 的 DV 配置和兼容基座仍会解析，但 RPU
  校验结果会标为 `conditional`。
- 当前读取普通 `moov/stbl` 样本表；fragmented-only 文件会明确告警，不能被当作
  完整诊断。
- `colr` 缺失时会告警，但不会假装读取了 SPS VUI；这是保守处理。
- 帧率由 `stts` 平均得出；变帧率素材的 `dv_level` 像素率检查是近似值。
- 静态分析不能证明某台设备一定黑屏；它给出“声明、码流、能力”三层的最小充分
  结论。最终设备行为仍需用该设备的实际播放器验证。

## DRM 边界

工具只读取未加密的容器配置和信号字段。遇到 `encv`、`sinf/frma/schm` 或 `pssh`
时：

- 不读取受保护样本，不猜测密钥，不调用外部解密器；
- 继续报告容器中未加密的 DV profile/level/entry 声明；
- 每段最低结论为 `auth_required`，而不是把 DRM 保护误判成码流损坏。

## 开发

```bash
go test ./...
go vet ./...
go build ./cmd/dvdiag
```

测试使用纯 Go 合成 MP4 和带 CRC32 的 RPU，覆盖 profile 8.1 回退、profile 5
DV-only、profile 7 + `hvcE`、缺失 `dvcC`、RPU profile 冲突、首个 RPU 前向引用、
SPS/hvcC level 冲突以及 DRM entry 不读取样本。
