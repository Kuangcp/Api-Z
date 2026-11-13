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
- 环境颜色：`Environment.Color` 持久化在 environments.json；空值由 `app.EnvironmentColorHint` 按名称/tag 推断（prod 红、test 绿、local/dev 蓝），UI 用 `envTextColor` 映射到主题色。环境管理器颜色只改 draft，按「保存环境」落盘。
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
- 请求/响应默认左右 `ui.SplitPercent`（`Root.requestPercent`）；请求区在左、响应区在右。
- Body 用 mygo `ui/editor`（JSON 高亮、行号、查找），顶部「类型」选择会 upsert/remove Content-Type；JSON 提供格式化按钮。Headers/Params 用 `ui/kv_editor.go` 的文本/表格双模式：表格行带启用开关、增删与 `! ` 禁用语义，文本仍是唯一持久化格式。
- 响应区三段：Raw / Model / AGUI；非 SSE 且 JSON 时提供 JSON 美化（只读 editor）。
- SSE 的 Raw 显示在 `data:` 行前加 `HH:mm:ss.SSS`；`Session` 维护 `lineEvent`/`sseEvents` 映射，点击事件行选中整组行，JSON 事件在底部用只读高亮编辑器展示，高度由 `SplitVerticalPercent`（`Editor.EventSplit`）可拖动。
- 请求编辑顶部使用发送图标，不提供独立「保存」「代码」按钮；手动保存只保留快捷键，代码导出从请求右键菜单进入。
- JSON 响应（`app.IsJSONContentType`、非 SSE/二进制、已完成）在 `Editor.maybeAutoOpenJSON` 中每个 spool 自动打开一次；用户手动关闭后同响应不再自动打开。
- 响应区固定 Body / Headers / Request 三页签；Headers 同时渲染请求头与响应头，HAR `entry.request.headers`/`_apiX.requestHeadersFullText` 必须保存实际发送头。Request 读取 `Session.Spec()` 并用 `app.FormatHTTP` 展示实际发送内容，HAR 历史读取 request 快照，不要重新拼编辑器原文。
- 二进制响应用 `Snap.FileName`（Content-Disposition/URL/Content-Type 推导），保存/打开走 `ui/binary.go`，复制文件用 `app.CopyFile`。
- 原始行用等宽字体、`SingleLine`、`clipLine`；AGUI 文本用分块 markdown。
- 历史回看只读 HAR；不要假装支持重放。
- 代码导出（cURL / Raw HTTP）走 `app/curl.go` 的 `FormatCurl` / `FormatHTTP`，输入必须是发送时的 `httpclient.Spec`（已替换环境、合并 Params、注入 Auth），不要拼编辑器原始文本。
- `data/` 目录 Push/Pull 走 `app/data_sync.go`：集合 `data/collection/<id>.json`、环境 `data/env/environments.json`；Pull 按集合 id upsert、环境按 id 合并且不删除本地项。UI 动作在 `ui/data_sync.go`。
- Postman 导入/导出走 `app/postman.go` 的 `PortableCollection`；不要直接改 JSON 或复制 api-x 字段逻辑。导入生成新 id，导出保留 `_api_x_id` 供回导/合并；禁用 Header/query 用 `! ` 前缀；auth 为空/inherit 时导出省略。
- 请求右键菜单只保留复制请求/复制 cURL/移动到文件夹/删除；打开用单击，重命名用双击请求行；复制 cURL 走 `app.BuildSpec` + `FormatCurl`，不要另写一套变量/Auth 解析。
- cURL 解析走 `app/curl.go`（sh tokenizer、ANSI-C 引号、常见 flags）；UI 在 `ui/curl.go`。标题栏 cURL 图标与树右键「解析 cURL」都直接读取剪贴板最新内容，目标是显式 collection/folder 或当前选中的 collection/folder；标题栏入口若选中 Request，则用 `nextSiblingID` + `MoveNodeBefore` 插到该请求下方；无选中时落到当前集合根。暂不支持 `-F/--form`，遇到应明确报错，不要静默生成错误请求。
- 图标用 Lucide 描线，集中放在 `ui/icons.go`，通过 `iconButton` / `iconButtonPlain` / `iconMenuButton` 使用；图标按钮必须有 `Label`（读屏）和 `Tooltip`，禁止手写零散 SVG。
- 应用图标源文件 `assets/icon.svg`，`tools/genicon.sh` 用 rsvg-convert 生成 `icon-128/256/512.png`；`main.go` 嵌入 256 图标。不要手改 PNG。
- OpenAPI 解析/合并走 `app/openapi.go` + `app/openapi_store.go`：tag→folder、确定性 ID、`METHOD normalizePath` syncKey。刷新只 upsert，不删除；保留本地 URL/Headers/Params/Body/Auth，只更新 name/method/meta/folder/sort。文件夹 ID 不得跨集合复用。
- 树/列表选中、hover、空态要有明确视觉；破坏性操作必须二次确认（`ui/tree_actions.go` 的删除弹窗）。
- 窗口为无边框自绘标题栏（`Root.titleBar`，38 DIP）：左侧应用图标 + Api-Z，右侧环境级联菜单/数据目录/设置与窗口按钮；整条 `DragWindow`。窗口底色用 `WindowBackgroundColor(themePrefs)`，圆角用 `WindowCornerRadius`。
- 环境切换走 `ui/env_menu.go`：按钮显示 `标签 | 环境名`，弹出两级菜单（左标签、右环境），不要退回平铺 `ui.Select`；无环境和“管理环境…”放在左列底部；不再在标题栏重复放环境管理图标。
- 主窗口用 `StateKey: "main"` + `windowStateStore` 恢复位置/大小/最大化；`session.json` 只存 `lastRequest`，打开/切换/关闭 Tab 后调用 `rememberActiveRequest`，缺失时保持空白态。
- 启动恢复 Request 时必须调用 `revealRequestInTree`：展开所有祖先文件夹并设置 `Root.reveal`，由 sidebar 在 List 构建后 `ScrollTo` 定位；不要在 openRequest 时无条件滚动。
- 树 CRUD 走 `app/store_crud.go`；UI 动作/弹窗在 `ui/tree_actions.go`，不要直接拼 SQL。
- 集合/文件夹设置走 `ui/node_settings.go` + `Store.UpdateNodeMeta`：Auth 用 `app.MergeAuth`，OpenAPI 绑定用 `app.SetCollectionOpenAPI`（保留 auth/color 等其它 meta）；设置弹窗不要直接改数据库。
- 左侧集合 Tab 由 `ui/collection_tabs.go` 管理：`Root.activeCollection` / `colMRU`，树只渲染当前集合；Tab 放不下时保留最近使用项，其余进「更多集合」下拉。新增集合/导入/OpenAPI 刷新/打开其他集合的请求后必须调用 `activateCollection`。
- 切换编辑器 Tab（点击/Ctrl+Tab/关闭后激活相邻 Tab）必须调用 `Root.focusRequest`：切集合、选中 Request、展开并滚动树。
- 多标签最近切换用 `Root.mru`（`Ctrl+Tab` 在最近两个之间切换）；全局搜索用 `Ctrl+K`，从搜索结果打开请求后必须调用 `revealRequestInTree`，展开祖先目录并在左侧树定位。
- URL 输入用 `ui/url_input.go` 在 `{{var}}` 下绘制主题强调色底纹并用强调色重绘变量文字；仅在文本未超出输入框时绘制，光标位于变量内时跳过该变量，避免横向滚动/光标遮挡。
- 主题：MyGo 每帧都会把 Context 主题重置为桌面默认，`applyTheme` 必须每帧 `SetTheme`，不能用 themeKey 跳过；themeKey 只用于 SetZoom 与原生窗口底色。
- 变量补全：URL/Headers/Params 用 `ui.Popover` + `Element.TextSelection/SetTextSelection`；Body 用 `editor.OnCompletion`。插入格式统一 `{{key}}`，不要在每个输入框里各写一套。
- 字体：`theme.json` 的 `treeFont/treeFontSize` 只作用于左侧请求树；`bodyFont/bodyFontSize` 作用于请求 Body 编辑器、响应 Raw/Model、JSON/事件编辑器；空字体/0 字号走默认。
- 主题/字体/缩放/快捷键：持久化到 `<config>/theme.json` / `keymap.json`；`theme.json` 含 `mode`/`accent`/`font`/`scale`（百分比），字体用 draft 字段避免逐键 trim，缩放走 `Context.SetZoom`；动作表在 `ui/keymap.go`，不要在 View 里散写 `c.Shortcut`。录制用 `detectChord`（`Context.KeyDown` + `Modifiers`），冲突时删除旧绑定。
- 主题只在 `themeKey` 变化时 `SetTheme`；系统模式保留启动时捕获的 `baseTheme`。强调色必须是 `#rrggbb`。
- 环境管理器编辑的是 `Root.envDraft` 深拷贝，按「保存环境」才 `Clean()` 落盘；不要在视图里直接改 `Root.env`。
- 树节点移动走 `Store.MoveNodeTo`（请求/文件夹，跨集合/层级）；拖拽插入走 `Store.MoveNodeBefore`。文件夹移动必须同步子树的 `collection_id`，并阻止移入自身/子树。UI 在 `ui/move.go` / `ui/tree_drag.go`。
- 拖拽目标规则：文件夹/请求可拖入集合根或文件夹；文件夹之间可上下排序，请求之间可上下排序；文件夹不能与请求混排（树显示文件夹在前）。
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
- Postman：`TestPostmanImportExport`（字段/auth/禁用项/回导）与 `TestPostmanImportExportUI`（文件导入/导出）。
- 环境：`TestEnvironmentManager`（新增/变量/保存）与 `TestEnvironmentManagerDeleteActive`（删除当前环境）。
- 移动：`TestStoreMoveNodeTo`（跨集合/子树更新/环检测）、`TestStoreMoveNodeBefore`（插入排序）、`TestMoveRequestToFolderUI` / `TestMoveFolderHidesSubtreeUI` / `TestDragRequestIntoFolderUI`。
- URL 输入用 `ui/url_input.go` 在 `{{var}}` 下绘制主题强调色底纹并用强调色重绘变量文字；仅在文本未超出输入框时绘制，光标位于变量内时跳过该变量，避免横向滚动/光标遮挡。
- 主题：MyGo 每帧都会把 Context 主题重置为桌面默认，`applyTheme` 必须每帧 `SetTheme`，不能用 themeKey 跳过；themeKey 只用于 SetZoom 与原生窗口底色。
- 变量补全：`TestFindVarPrefixInsert`（前后缀/光标）、`TestVariableAutocompleteUI` / `TestVariableAutocompleteTextArea`（弹出/点击插入）、`TestVariableAutocompleteBodyEditor`（编辑器 completion）。
- 设置：`TestChordParseFormat`、`TestResolveTheme`、`TestSettingsThemeAndKeymap`、`TestThemeResetsToSystem`。
- OpenAPI：`TestParseOpenAPI`、`TestNormalizeOpenAPIPath`、`TestMergeOpenAPIPreservesLocalEdits`、`TestRefreshOpenAPICollection`、`TestOpenAPIImportAndRefreshUI`、`TestOpenAPIImportDialog`。
- cURL：`TestParseCurlBasic`、`TestParseCurlAnsiCAndContinuation`、`TestParseCurlAuthCookieGet`、`TestParseCurlJSONMode`、`TestParseCurlErrors`、`TestCurlImportUI`、`TestCurlDialog`。
- 修 bug 先写失败测试再改；不要用“看起来对”代替验证。
- 测试不要依赖用户真实数据目录、网络或当前时间。

## 日志与脱敏

- `app.LogInfo/LogWarn/LogError(tag, msg, kv...)`；tag：`startup` `migrate` `har` `conn` `exec` `panic`。
- URL 入日志前用 `app.RedactURL`；`Authorization`/`Cookie`/密码永不入日志。
- 日志写入失败不能影响调用方。每个请求在 `Session.Start` / `OnDone` 记录 method、`RedactURL` 后的 URL、status、duration、bytes、SSE events；panic 用 `app.LogPanic` 记录堆栈。
- 崩溃兜底：后台 goroutine 用 `app.SafeGo(tag, fn)`；请求 goroutine 捕获 panic 后调用 `finishPanic`；UI View/启动回调 panic 记录到日志并显示兜底提示，不要静默吞异常。

## 提交与文档

- 提交信息中文，Conventional Commits：`feat: / fix: / perf: / ui: / docs: / chore:`；一次提交只做一件事。
- **不要提交**：编译产物 `api-z`、`.idea/`、日志、`*.db`/`*.db-shm`/`*.db-wal`、`request/` 运行时 artifacts、HAR/spool 测试输出。
- 完成一个阶段同步 `Readme.md` 功能清单与 `Roadmap.md` 状态/实测数字。
- 设计/计划放 `Roadmap.md` 或 `docs/`；不要只写在聊天里。

## 当前已知限制（不要当成已实现）

- JSON 折叠树、响应搜索。
- Multipart、代理 UI、Cookie jar。
- MCP（SSE / streamable HTTP / stdio）——当前 `method=MCP` 的请求会按普通 HTTP 发送。
- 工作流/断言/变量提取、压测。
- 历史回看只读，不能重新执行或对比。

## 新增功能检查表

- [ ] 数据：是否需要迁移/SQLite 字段？是否只读源数据？
- [ ] 性能：是否引入全量字符串、每帧重算、超大单行/单篇布局？
- [ ] 并发：后台结果是否回投 UI 线程？Session 锁内是否做 IO？
- [ ] UI：颜色是否取主题？空态/只读原因是否可见？
- [ ] 测试：是否有 headless/httptest 覆盖？是否使用临时目录？
- [ ] 文档：`Readme.md`/`Roadmap.md` 是否同步？
- [ ] 全过：`make test && make vet && gofmt -l .`
