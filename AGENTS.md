# api-z 开发约定（AGENTS.md）

本文件对后续所有 agent/开发者生效。与 `Readme.md`、`Roadmap.md` 冲突时，**功能范围看 Roadmap，性能与工程约束看这里**。

api-z 是 [api-x](../Kotlin/api-x)（Kotlin + Compose Desktop）的替代品：Go + [mygo](https://github.com/egoist/mygo) 的本地优先 API 调试客户端，重点解决 600K+ SSE / AG-UI 流式响应的渲染与状态管理问题。

## 技术栈（不要随意改动）

- Go **1.27.3**（`go.mod` 的 `go` 指令）；`CGO_ENABLED=0`。
- mygo **v0.8.3**（固定 tag，不用伪版本；升级需先跑性能与 UI 回归）。
- `modernc.org/sqlite`（纯 Go，无 CGO）。
- `github.com/andybalholm/brotli`（响应解压）。
- mygo 工具：`go tool mygo vet .`（已写入 `go.mod` 的 `tool` 指令）。

```bash
make build     # CGO_ENABLED=0 go build -trimpath -o api-z .
make test      # CGO_ENABLED=0 go test -p 1 ./...
make vet       # go vet ./... + go tool mygo vet .
gofmt -l .     # 必须为空
```

**提交前必须全过。** 修改 UI 后再加跑 `go test ./ui`；修改 Session/SSE 后再加跑 `CGO_ENABLED=1 go test -race ./app ./httpclient`。

## 分层与 import 方向（硬约束）

```text
main.go ──> ui ──> app ──> httpclient
```

| 包 | 职责 | 允许依赖 |
|---|---|---|
| `httpclient/` | 协议无关：请求规格、`net/http` 流式读取、SSE 解析、AG-UI 状态机、JSON delta 提取、压缩 | 标准库 + `brotli` |
| `app/` | 纯逻辑与落盘：路径/日志、SQLite、api-x 迁移、环境/Auth/URL、Session、HAR | `httpclient` |
| `ui/` | mygo 视图层（唯一 import mygo 的地方）：集合树、请求编辑器、响应区、历史 | `app`、`httpclient`、mygo |
| `main.go` | 装配、flag、迁移、窗口 | 全部 |

- `ui/` 不写 SQL，`app/` 不 import mygo，`httpclient/` 不碰 UI 状态。
- 新增能力优先加在叶层；不要让 `httpclient` 反向依赖 `app`。

## 性能硬约束（★ 必读）

api-z 的存在理由就是这条链路。**不要退回全量字符串 + 每帧全量重算。**

1. **原始响应不整体驻留**：`app.Session` 写 spool 文件（`request/<id>/response/<ts>.raw`），内存只保留有上限的行列表（`maxDisplayLines`）。复制全文走 spool/`CopyText`。
2. **SSE 每行只解析一次**：`SSEParser.Line` → `AguiState.Apply` / `ModelState.Apply`，复杂度 O(delta)。禁止定时从全部行重建 AG-UI 状态。
3. **UI 只读快照**：`Session.Snapshot()` 返回标量；行通过 `Session.Line(i)` 按需读取。不要每帧 `CopyText()`。
4. **AG-UI 文本必须分块**：`ui/response.go` 的 `buildAguiRows` 按 8KB 分块（`splitText`），`ui.List` 只布局可见块。**禁止对 600KB 单篇直接 `markdown.View`**，Pango shaping 会阻塞数秒。
5. **超长行只显示截断**：`clipLine(line, 4096)`；`ui.Text(...).SingleLine()` 不能直接吃 600KB 单行。
6. **列表虚拟化**：响应原始行、AG-UI 行、树都用 `ui.List`；不要构建全量 `[]Element`。
7. **帧预算 16ms**（600KB / 1 万事件）。当前实测：10k 事件解析 81ms，600KB AGUI 单帧 p50 1.37ms。回退超过预算的改动必须解释并补测。
8. 大响应/大 JSON 的解析、格式化、HAR 写入都在后台 goroutine；UI 只投递 `win.Update`。

## 并发与 Session

- **一个请求一个 `app.Session`**（按 request id 缓存），不是每个 UI 一个。
- `Session` 的字段由自己 `mu` 保护；请求 goroutine 写，UI 线程读。
- `Session.SetNotify` 的回调只做 `win.Update(func(){})`；不要在回调里读 Session 字段。
- 不要在持有 `Session.mu` 时做磁盘 IO（HAR 写入在 `OnDone` 解锁后做）。
- UI 构建只在主线程；长活（发送、迁移、读 HAR）必须离开 UI 线程。
- 取消/超时要能中断流：`context` → `http.Client.Do` 返回；`OnDone` 必须只执行一次。
- 多请求并行是设计目标；切换 Tab 不能取消其他请求。

## 数据与迁移（★ 必读）

数据目录（XDG / APPDATA / Application Support）：

```text
<data>/api-z/
  api-z.db              # collections / folders / requests / exchanges / schema_meta
  environments.json     # 与 api-x 同构
  request/<id>/response/*.har
  logs/YYYY-MM/...
```

- 路径统一走 `app.DefaultPaths()` / `app.PathsAt()`；测试用 `t.TempDir()`。
- **api-x 迁移只读**：`app.MigrateFromApiX` 先把 `collections.db`（及 `-wal`）复制到临时目录，再以 `mode=ro` 打开。**禁止直接打开源库**——SQLite 会创建 `-shm/-wal`，破坏“原目录不动”的承诺。
- 迁移幂等：`schema_meta.migrated_from` 存在则跳过；目标库非空返回 `ErrDestinationNotEmpty`。
- 源数据永不删除、永不修改；测试必须断言源库 hash 不变且无 sidecar。
- 请求 `meta_json` 的 `auth` 语义与 api-x 一致（`inherit`/`noauth`/`bearer`/`basic`/`apikey`）。
- 新增持久化字段：先判断“删实体是否一起消失、是否按它查询”；结构化字段进 SQLite，瞬态进内存。

## HTTP 语义

- `headers_text`：一行 `Name: Value`；`! ` 前缀表示只编辑不发送。
- `params_text`：一行 `key: value`；发送时编码并拼到 URL（空格为 `+`，与 api-x 一致）。
- 环境替换：`{{name}}`，最多 32 轮（支持值里再含变量）；未知变量保留原文。
- Auth 继承：request → 父 folder → collection，第一个非 `inherit` 生效（`app.Tree.EffectiveAuth`）。
- 传输层禁止 header：`Content-Length`/`Transfer-Encoding`/`Connection`/`Upgrade`/`Expect`；`Host` 单独设置。
- 超时：当前请求总超时 120s；后续设置页可配。
- SSE 检测：`Content-Type: text/event-stream`，或首个 `data:`/`event:` 行；不用整包 JSON 解析来判断。
- 解压：gzip / deflate / br；Go 自动 gzip 时看 `resp.Uncompressed`。
- 代理：`http.ProxyFromEnvironment`；SOCKS/UI 代理尚未做。

## UI 约定

- 界面文案一律中文；不做 i18n。
- 颜色一律取 `c.Theme()`；禁止硬编码颜色（数据源/方法色除外，方法色集中在 `methodColor`）。
- 主题跟随系统；不要自行 SetTheme 覆盖桌面偏好。
- Body 用 mygo `ui/editor`（JSON 高亮、行号、查找）；Headers/Params 用 `TextArea`。
- 响应区三段：Raw / Model / AGUI；非 SSE 且 JSON 时提供 JSON 美化（只读 editor）。
- 原始行用等宽字体、`SingleLine`、`clipLine`；AGUI 文本用分块 markdown。
- 历史回看只读 HAR；不要假装支持重放。
- 树/列表选中、hover、空态要有明确视觉；破坏性操作必须二次确认（`ui/tree_actions.go` 的删除弹窗）。
- 树 CRUD 走 `app/store_crud.go`；UI 动作/弹窗在 `ui/tree_actions.go`，不要直接拼 SQL。
- 多标签最近切换用 `Root.mru`（`Ctrl+Tab` 在最近两个之间切换）；全局搜索用 `Ctrl+K`。
- 新增组件先看 db-g 的 `ui/` 实现（sidebar/tabs/result/settings），保持交互一致。

## 测试

- **能用 `ui.NewTester` 就别开真窗口**：
  ```go
  tt := ui.NewTester(root.View, 1200, 800)
  tt.Frame()
  tt.HasText("200 OK")
  tt.Click("发送")
  ```
- UI 测试不碰真实配置：`app.PathsAt(t.TempDir(), t.TempDir())`。
- HTTP/SSE 测试用 `httptest`；AG-UI 断言消息、工具调用、中断、事件计数。
- 性能回归：`TestRootAguiLargeStream`（600KB 单帧）与 `TestSessionManyEvents`（10k 事件）。
- 树 CRUD：`TestStoreTreeCRUD`（级联/复制/排序）与 `TestTreeCreateRenameDelete`（真实 UI 流程）。
- 修 bug 先写失败测试再改；不要用“看起来对”代替验证。
- 测试不要依赖用户真实数据目录、网络或当前时间。

## 日志与脱敏

- `app.LogInfo/LogWarn/LogError(tag, msg, kv...)`；tag：`startup` `migrate` `har` `conn` `exec` `panic`。
- URL 入日志前用 `app.RedactURL`；`Authorization`/`Cookie`/密码永不入日志。
- 日志写入失败不能影响调用方。
- 崩溃兜底后续加；当前 UI panic 会让窗口退出，不要静默吞异常。

## 提交与文档

- 提交信息中文，Conventional Commits：`feat: / fix: / perf: / ui: / docs: / chore:`；一次提交只做一件事。
- **不要提交**：编译产物 `api-z`、`.idea/`、日志、`*.db`/`*.db-shm`/`*.db-wal`、`request/` 运行时 artifacts、HAR/spool 测试输出。
- 完成一个阶段同步 `Readme.md` 功能清单与 `Roadmap.md` 状态/实测数字。
- 设计/计划放 `Roadmap.md` 或 `docs/`；不要只写在聊天里。

## 当前已知限制（不要当成已实现）

- JSON 折叠树、响应搜索、请求实际发送预览。
- Multipart、代理 UI、Cookie jar、OpenAPI/Postman 导入导出、`data/` Push/Pull。
- MCP（SSE / streamable HTTP / stdio）——当前 `method=MCP` 的请求会按普通 HTTP 发送。
- 工作流/断言/变量提取、压测、代码片段。
- AG-UI 自定义 `interrupt UI` 组件、`MESSAGES_SNAPSHOT` 的完整还原。
- 历史回看只读，不能重新执行或对比。

## 新增功能检查表

- [ ] 数据：是否需要迁移/SQLite 字段？是否只读源数据？
- [ ] 性能：是否引入全量字符串、每帧重算、超大单行/单篇布局？
- [ ] 并发：后台结果是否回投 UI 线程？Session 锁内是否做 IO？
- [ ] UI：颜色是否取主题？空态/只读原因是否可见？
- [ ] 测试：是否有 headless/httptest 覆盖？是否使用临时目录？
- [ ] 文档：`Readme.md`/`Roadmap.md` 是否同步？
- [ ] 全过：`make test && make vet && gofmt -l .`
