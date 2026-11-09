# Api-Z Roadmap

更新：2026-11-08
定位：对标 [api-x](../Kotlin/api-x)（Kotlin + Compose Desktop）的本地优先 API 调试客户端，技术栈 Go + [mygo](https://github.com/egoist/mygo)。
参考实现：`../db-g`（Go + mygo 数据库客户端）的分层、持久化、快捷键、headless 测试与性能约束。

## 1. 为什么做

api-x 在 **600K+ SSE / AGUI 流式响应** 上已经触到 Compose 的结构性上限：

| 证据 | 问题 |
|---|---|
| `ResponsePanel.kt:109-128` | 每次响应变化都 `responseLines.toList()` + `buildString` 全量拼接 |
| `ResponsePanel.kt:157-171` | AGUI 每 1s 从全部行重新 `extractAguiEvents` + `buildAguiRunState` |
| `ResponsePanel.kt:139-153` | Model 提取每次从头扫描全部行 |
| `ResponseBodyView.kt:460,535` | 每行 `LayoutCoordinates` 常驻，长连接泄漏 |
| `TODO.md` | 「AGUI 渲染时阻塞 UI，顶栏按钮直接无响应」「SSE 响应性能问题」 |
| `opt-v2.md` | 线程安全、无界增长、God Composable |

Go + mygo 的价值不是“语言更快”，而是让正确的数据通路更容易实现：

- 单文件、无 JVM/Skiko 基线（db-g 25MB；api-x 应用镜像 154MB、Deb 66MB）。
- 每请求 goroutine + `context` 取消，天然支持多请求并行与状态隔离。
- mygo 的列表、表格、编辑器、Markdown 都只布局可见行（百万行列表 14.5µs/帧；50KB Markdown 整帧 1.3ms）。
- headless `ui.Tester` 让 UI 回归可自动验证（api-x 目前几乎没有 UI 测试）。

**硬约束：不照搬“全量字符串 + 每帧全量重算”。** 换语言不会自动解决 600K 问题。

## 2. 已确认的决策

| 决策点 | 结论 |
|---|---|
| api-x 数据 | **只读识别 + 一次性迁移**；api-x 原目录不动，可回退 |
| MVP 范围 | **HTTP + 集合/环境 + SSE/AGUI 垂直切片**；MCP 下一阶段 |
| mygo 版本 | 固定 **v0.8.3**；需要时向上游提 PR |
| 界面语言 | 中文；不做 i18n |
| 打包 | `go build` 单文件；安装器按需，不进 MVP |
| 数据源 | HTTP / SSE；MCP 在其后；WebSocket 暂不做 |

## 3. 非目标（本 Roadmap 内）

- 与 api-x 同时读写同一份 SQLite（会冲突）。
- 1:1 像素级复刻 Compose 界面。
- 工作流/压测/脚本引擎/云同步/GUI 表设计器。
- 在 MVP 内做 i18n、多窗口、自动更新。

## 4. 架构

```text
main.go ──> ui ──> app ──> httpclient
        └──> _ internal/...
```

| 包 | 职责 | 允许依赖 |
|---|---|---|
| `httpclient/` | 协议无关执行器：请求规格、流式读取、SSE 解析、AGUI 事件状态机、HAR | 标准库 + `golang.org/x/net` |
| `app/` | 纯逻辑与落盘：数据目录、SQLite、迁移、环境变量、认证、历史、日志 | `httpclient` |
| `ui/` | mygo 视图层（唯一 import mygo 的层）：树、编辑器、响应区、设置 | `app`、`httpclient`、mygo |
| `internal/` | 生成器/一次性工具，不是运行时代码 | 标准库 |

Import 方向是硬约束：`ui` 不写 SQL，`app` 不 import mygo，`httpclient` 不碰 UI 状态。

### 请求状态机

每个请求一个 `Session`（不是每个 UI 一个）：

```text
idle -> running -> streaming -> done
                    |-> cancelled
                    |-> failed
                    |-> timeout
```

`Session` 持有：`context.CancelFunc`、原始响应 spool、行索引、SSE 事件环、AGUI/Model 派生状态、状态码/耗时/字节数。
UI 每帧只读 `Session.Snapshot()`，后台 goroutine 只写 `Session`。

### 流式数据通路

```mermaid
flowchart LR
  A[UI 发送] --> B[Session goroutine]
  B --> C[net/http 流]
  C --> D[bufio 行解析]
  D --> E[(raw spool / HAR)]
  D --> F[增量 SSE 事件 O(delta)]
  F --> G[AGUI/Model 状态]
  G --> H[不可变快照]
  E --> I[行索引]
  H --> J[UI 每帧只读]
  I --> J
  J --> K[ui.List / editor 只建可见行]
```

要点：

1. 原始字节先落 spool，显示按行窗口读取；`[]string` 只作为有上限的索引缓存。
2. SSE 每行只解析一次；AGUI 状态 O(delta) 更新，不再定时全量重算。
3. 不在 UI 线程做 JSON 解析、字符串拼接、Markdown 解析。
4. `Session` 版本号用 atomic；UI 只在版本变化时重绘需要重绘的部分。

### 性能预算（验收目标）

| 指标 | 目标 |
|---|---|
| 冷启动到可交互 | < 300ms |
| 空闲 RSS（Linux） | < 150MB |
| 600KB / 1 万 AGUI 事件，流式 p95 帧 | < 16ms |
| 单 600KB 响应额外内存 | < 10MB |
| 10k 行响应滚动 | 不分配整帧文本布局 |
| 536 请求树展开/过滤 | < 50ms |

## 5. 数据模型与迁移

api-z 使用自己的数据目录与 SQLite（WAL）：

```text
<data>/api-z/
  api-z.db              # collections / folders / requests / schema_meta / history
  environments.json     # 与 api-x 同构
  request/<id>/response/*.har
  logs/YYYY-MM/...
<config>/api-z/
  settings.json         # 窗口、布局、超时、上次环境
```

### 迁移规则

1. 启动时若 api-z DB 不存在，尝试识别 api-x 数据目录（XDG/APPDATA/Application Support，支持 `debugHome`）。
2. 以 **只读 URI** 打开 `collections.db`；复制 collections / folders / requests（保持 id、sort_order、meta_json）。
3. 复制 `environments.json`；复制 `request/` 下的 response artifacts。
4. 写入 `schema_meta(migrated_from, migrated_at, api_x_version)`。
5. 迁移失败不阻塞启动：记录日志，api-z 以空库启动；可重试。
6. 永不写 api-x 目录；不删除源数据。

### 请求字段

沿用 api-x 语义：

| 字段 | 含义 |
|---|---|
| `method`, `url` | 原始方法/URL（含 `{{var}}`） |
| `headers_text` | 一行 `Name: Value`；`! ` 前缀表示不发送 |
| `params_text` | 一行 `key: value`，发送时编码并拼到 URL |
| `body_text` | 原始正文 |
| `meta_json` | `auth`（PostmanAuth 子集）、`openapi`、`color` |

发送顺序：环境替换 → Params 合并 → Auth 解析 → Header 过滤 → Body 编码 → 执行。

## 6. 阶段

### P0 骨架（已完成）

- Go module、固定 mygo v0.8.3、分层目录。
- 数据目录/日志/设置、SQLite schema、api-x 只读迁移。
- headless 测试骨架；`Makefile`；`go build/vet/test`、`mygo vet`、`gofmt`。

验收：`go test ./...` 通过；迁移测试用临时 api-x fixture；`-migrate` 命令可对真实数据只读演练。

### P1 单请求闭环（已完成；JSON 折叠树待补）

- 集合/文件夹/请求树（虚拟化、展开状态、键盘上下）。
- 请求编辑器：方法、URL、Headers/Params/Body/Auth。
- 发送/取消/超时；状态码、耗时、字节数；多请求并行。
- 原始响应虚拟化、行复制、自动跟随末尾。
- HAR 落盘；历史列表（最近 10 次）。
- 环境切换与 `{{var}}` 替换；Auth 继承。

验收：真实 api-x 数据迁移后，任选请求完成「打开 → 发送 → 看响应 → 保存 → 重启恢复」。

### P2 集合/环境/多标签（进行中：CRUD / 多标签 / 搜索已完成）

- 集合/文件夹/请求 CRUD、拖拽排序、重命名、复制、删除。
- 多标签、最近请求切换（Ctrl+Tab）、全局搜索（Ctrl+K）。
- 环境管理器（多环境、tag 分组）、变量自动补全。
- 主题（浅/深/跟随系统）、字体、快捷键可改。
- Postman v2.1 导入/导出。

### P3 流式与 AI（核心已完成；TPOT / 树节点 loading 待补）

- SSE 增量解析、事件计数、TTFT/TPOT。
- Raw / Model / AGUI 三视图；AGUI 文本、工具调用、推理、中断卡片。
- 多请求并行时各自保留流状态；树节点 loading 标记。
- 600K/万事件基准与帧预算验证。

### P4 互操作

- OpenAPI 导入/刷新（`METHOD + normalized path` 合并，保留本地编辑）。
- `data/` 目录 Push/Pull（与 api-x 同构，Git 友好）。
- curl 导入/导出、代码片段。
- api-x HAR 历史读取。

### P5 MCP

- SSE / streamable HTTP / stdio 会话；catalog（tools/resources/prompts）。
- 参数表单 + JSON 草稿；协议日志；进程生命周期与 stderr。

### P6 按需

- 工作流/断言/变量提取、压测、代理、multipart/大文件、Cookie jar。

## 7. 风险与对策

| 风险 | 对策 |
|---|---|
| mygo pre-1.0 API 变动 | 固定 v0.8.3；UI 层隔离；锁定 go.sum |
| 自绘 UI 工作量大 | 复用 db-g 的 sidebar/tabs/settings/keymap 结构；先垂直切片 |
| 中英文输入法/CJK | MVP 首周在目标平台实测；字体走 mygo 主题 |
| Linux GTK3/OpenGL | 与 db-g 相同；Windows D3D11 路径不依赖 GTK |
| JSON 键序/精度 | 原始响应保留 bytes；JSON 树用 token 流保序 |
| 迁移破坏数据 | 只读源库；迁移写 api-z 自己的库；失败回退空库 |
| 功能面膨胀 | 每阶段有明确验收；MCP/工作流/压测后置 |

## 8. 工程约定

- `CGO_ENABLED=0`；`go test -p 1 ./...`；`go tool mygo vet .`；`gofmt -l .` 为空。
- 构建 UI 只在主线程；后台结果用 `win.Update` 回投。
- 长活必须带 `context.WithTimeout`；取消要能立即中断读流。
- 日志脱敏：URL 的 query、Authorization、Cookie、密码不入日志。
- 测试不碰真实配置：数据目录注入 `t.TempDir()`。
- 提交信息中文，前缀 `feat: / fix: / ui: / docs: / chore:`；一次提交一件事。

## 9. 验证命令

```bash
make build
make test          # CGO_ENABLED=0 go test -p 1 ./...
make vet           # go vet + go tool mygo vet .
gofmt -l .         # 必须为空

# 只读演练迁移真实 api-x 数据到临时目录
go run . -migrate -api-x-home ~/.local/share/api-x -data-dir /tmp/api-z-smoke
```

## 10. 当前状态

- [x] 方向、架构、性能预算、迁移策略确认
- [x] P0 骨架（Go module、mygo v0.8.3、SQLite、日志、迁移、headless 测试）
- [x] P1 单请求闭环（HTTP、集合树、编辑器、环境/Auth、HAR、历史、多标签）
- [ ] P1 补：JSON 折叠树、响应搜索、请求实际发送预览
- [x] P2 第一批：集合/文件夹/请求 CRUD、重命名、复制、上下移动、级联删除、Ctrl+Tab、Ctrl+K
- [x] P2 第二批：Postman v2.1 导入/导出
- [x] P2 第四批：主题/快捷键设置
- [x] P3 核心：SSE 增量、Raw/Model/AGUI、工具/推理/中断卡片、600KB/万事件验证
- [ ] P3 补：TPOT、树节点 loading、AG-UI 自定义 UI 组件
- [ ] P4 OpenAPI、data/ Push/Pull、curl/代码片段
- [ ] P5 MCP（SSE / streamable HTTP / stdio）

### 已实测

| 场景 | 结果 |
|---|---|
| 10,000 个 SSE 事件（Session 状态机） | 81ms |
| 600KB AGUI 单帧（headless） | p50 1.37ms / max 1.84ms |
| 真实 api-x 数据迁移 | 3 集合 / 59 文件夹 / 536 请求 / 100 artifacts / 94 exchanges |
| 请求闭环 | `ui.Tester` 打开 → 发送 → 200 → JSON → 历史 → 返回实时 |
