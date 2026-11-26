# mediafp —— 媒体载荷指纹与结构指纹

纯 Go(零第三方依赖)实现:解决"文件只改变容器封装顺序,整文件哈希变化
但音视频内容未变"的比对问题,区分**正常重封装 / 元数据修改 / 真实帧替换**,
并生成可验证的**证据链**。

## 四级指纹定义

| 指纹 | 覆盖范围 | 不变量 | 敏感于 |
|------|----------|--------|--------|
| `payload_fp` 载荷指纹 | 逐样本 SHA-256,按 (轨, DTS) 规范排序,附编解码标识与时基 | 盒顺序、chunk 布局、mdat 位置、元数据、填充 | 任何一帧内容/时长/数量变化 |
| `structure_fp` 结构指纹 | 盒树全量骨架(类型/尺寸/序)+ 叶子盒内容哈希(mdat 只记尺寸) | 仅完全相同的封装 | 任何容器层改动 |
| `layout_fp` 布局指纹 | 结构指纹剔除元数据子树(udta/meta)、填充盒(free/skip),mvhd/tkhd/mdhd 时间戳归零 | 元数据修改、填充变化 | 盒序、chunk 布局、stco、样本表变化 |
| `metadata_fp` 元数据指纹 | udta/meta 子树内容、mvhd/tkhd/mdhd 时间戳、hdlr 名称 | 重封装、载荷变化 | 标签/标题/时间戳修改 |

载荷指纹规范序列化域分隔:`MEDIAFP/PAYLOAD/v1`;样本哈希先于轨内按
(DTS, 序号) 排序再入轨哈希,轨按 TrackID 排序入文件哈希,因此与
chunk 划分、轨交错方式、moov 位置完全解耦。

## 判定矩阵(compare A B)

| 载荷 | 布局 | 元数据 | 结构 | 判定 |
|------|------|--------|------|------|
| = | = | = | = | `IDENTICAL` 完全一致 |
| = | = | ≠ | ≠ | `METADATA_MODIFIED` 元数据修改 |
| = | ≠ | 任意 | ≠ | `REMUX` 正常重封装 |
| ≠ | 任意 | 任意 | ≠ | `CONTENT_REPLACED` 真实帧替换 |

`CONTENT_REPLACED` 时输出样本级证据:差异轨、首差异样本序号、差异样本
索引列表(截断至 64 条)、各轨样本计数,可定位被替换的帧。

## 证据链

`compare --chain evidence.jsonl [--sign key]` 将每次比对追加为一条 JSONL
记录:

```
{seq, time, action, subjects, file_hashes, payload_fps, verdict, detail,
 prev_hash, hash, signature}
```

- `hash = SHA-256(上述字段的规范 JSON)`,构成哈希链,删改/重排/插入即断链;
- `signature = ed25519(hash)`,防止持文件者伪造整条链;
- `verify-chain` 重算全部哈希并验签。

## 用法

```sh
go build ./cmd/mediafp

mediafp fp video.mp4                      # 计算四级指纹
mediafp compare old.mp4 new.mp4           # 判定差异类型
mediafp keygen k.priv k.pub               # 生成证据签名密钥
mediafp compare a.mp4 b.mp4 --chain ev.jsonl --sign k.priv
mediafp verify-chain ev.jsonl --pub k.pub
```

## 范围与限制

- 支持非分片 MP4/MOV(stco/co64);分片 MP4(fMP4)与其他容器(MKV/TS)
  可在 `internal/mp4` 旁新增解析器复用同一指纹框架;
- 不解码音视频,载荷指纹作用于压缩码流样本层,重编码(转码)会被判为
  内容变化——这符合"帧替换"语义:码流不同即不可互证。

## 测试

```sh
go test ./...
```

合成 MP4 生成器(`internal/testutil`)构造重封装/元数据修改/帧替换/增删帧
场景;另已用 ffmpeg 真实文件(faststart 重封装、`-metadata` 修改)验证判定。
