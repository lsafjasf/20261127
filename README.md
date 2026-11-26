# Audio Routing Map

一个 Go 单模块的音频路由审阅工具，覆盖 `track patch`、`output bus` 与
`send`。它把每条连接表示为稳定的 `node ID` + `port ID` 到 `node ID` +
`port ID` 的有向边，并生成可直接打开的自包含 HTML/SVG 映射图。

针对“轨道名相近、迁移后左声道进入效果返回”的问题，解析从不使用显示名：
同名轨道只要 ID 不同就是不同节点，`main-l` 与 `main-left` 也不会被猜测为
同一端口。迁移批次中任何一个端点不存在，整个批次回滚。

## 规则

- **ID 才是身份**：`name`、`label` 只用于展示；路由端点只接受稳定 ID。
- **方向明确**：source 必须是 track/bus 的 output，destination 必须是
  output bus 的 input。`track_patch` 的 source 只能是 track；`send` 可以来自
  track 或 bus output，因此可以表达效果总线之间的串联和环路。
- **显式通道**：左声道、右声道、效果发送等均是独立端口，不使用列表位置或
  轨道名推断。
- **整批事务**：`add_route`/`remove_route`/`replace_route` 在一个副本上顺序
  执行。每个操作后都做完整校验；缺失节点、缺失端口、方向错误或新环路都会
  丢弃整个批次，原 workspace 与输出文件都不改变。
- **保守环路检测**：output bus 的任一 input 视为可到达其所有 output，再做
  有向图 DFS。比起猜测效果器内部通道路由，这个假设更安全，且能定位环路
  经过的 route ID。

## 数据模型

```json
{
  "schema_version": 1,
  "tracks": [
    {
      "id": "track:lead-b",
      "name": "Lead Vox",
      "outputs": [{"id": "main-left", "label": "Main L"}]
    }
  ],
  "buses": [
    {
      "id": "bus:master",
      "name": "Main Output",
      "inputs": [{"id": "main-l", "label": "L"}],
      "outputs": [{"id": "program-l", "label": "Program L"}]
    }
  ],
  "routes": [
    {
      "id": "patch-lead-b-left",
      "kind": "track_patch",
      "from": {"node": "track:lead-b", "port": "main-left"},
      "to": {"node": "bus:master", "port": "main-l"}
    }
  ]
}
```

完整示例见 `examples/workspace.json`。其中两条轨道都显示为 `Lead Vox`，但
端口分别是 `main-l/main-r` 与 `main-left/main-right`，图中仍能清楚区分。

## 批量操作

```json
[
  {
    "op": "replace_route",
    "route": {
      "id": "patch-lead-a-left",
      "kind": "track_patch",
      "from": {"node": "track:lead-b", "port": "main-left"},
      "to": {"node": "bus:master", "port": "main-l"}
    }
  }
]
```

支持 `add_route`、`remove_route`（使用 `route_id`）和 `replace_route`（使用
完整 `route`）。操作按数组顺序执行；任一步失败都会返回原 workspace。

`examples/reject-missing-port.json` 故意在第二个操作中把
`track:lead-b/main-l` 写成左声道 source。实际端口是 `main-left`，因此结果是：

```text
batch rejected; workspace unchanged
[missing_port] operation 2: route "...": source port track:lead-b/main-l does not exist
```

第一个操作已经做出的替换也不会落盘。

## CLI

```bash
# 完整校验；有缺失端点或环路时退出码为 1
go run ./cmd/routing check -workspace examples/workspace.json
go run ./cmd/routing check -workspace examples/workspace.json -json

# 成功时写出新 workspace；失败时不创建/覆盖输出
go run ./cmd/routing apply \
  -workspace examples/workspace.json \
  -ops examples/migrate-lead-b.json \
  -out /tmp/migrated.json

# 生成可交互审阅页，或独立 SVG
go run ./cmd/routing render \
  -workspace examples/workspace.json \
  -out /tmp/routing.html
go run ./cmd/routing render \
  -workspace examples/workspace.json \
  -format svg \
  -out /tmp/routing.svg
```

`render` 也接受 `-ops`。只有整个批次成功后才渲染，避免展示迁移到一半的状态。
如果输入的 workspace 本身有错误但不提供 `-ops`，页面仍会生成，并用红色虚线、
`MISSING NODE` 幽灵节点和诊断列表展示问题。

```bash
go run ./cmd/routing check -workspace examples/invalid-workspace.json
go run ./cmd/routing render \
  -workspace examples/invalid-workspace.json \
  -out /tmp/invalid-routing.html
```

该示例同时包含一个错误端口 `main-right` 和一条 bus → bus 环路。

## 可视化

- 蓝色实线：`track_patch`。
- 橙色虚线：`send`。
- 红色虚线：缺失端点、无效方向或环路。
- 每条边连接具体端口圆点；轨道与总线同时显示 `name` 和稳定 ID。
- 路由表列出 route ID、kind、source/destination 的 ID 与显示 label，可直接
  核对“左声道是否仍在 master L”。
- HTML 支持按 kind 过滤，并按 track/bus/route/port 文本搜索。

HTML 与 SVG 均无外部 CSS/JS/字体依赖。SVG 内嵌样式，可单独打开或嵌入文档。

## 验证

```bash
go test ./...
go vet ./...
```

测试覆盖：同名轨道不互相重定向、缺失节点/端口诊断、环路路径、有效声道迁移、
含缺失端口的整批回滚、先删后加失败时旧路由保留、HTML 转义和 SVG 独立输出。

## 设计边界

工具能判断端点是否存在、方向是否合法、图是否成环，但无法判断一个人工选择的
合法目的地是否符合听感意图。例如“左声道进入 FX Return”如果两个 ID 都真实
存在，它仍是合法边；可视化、端口标签和 route 表会让这个映射直接可见，便于
迁移审阅时发现。若要强制通道语义，可在端口上增加额外策略层，但不影响当前
路由事务和环路检测。
