# Api-Z

Go + [mygo](https://github.com/egoist/mygo) 的本地优先 API 调试客户端，替代 [api-x](https://github.com/Kuangcp/api-x)（Kotlin + Compose Desktop）。

面向 Spring Boot / Spring AI / AgentScope / MCP 调试：HTTP、SSE、AG-UI 流式响应、集合/环境、认证继承、HAR 历史。
设计与阶段见 [`Roadmap.md`](Roadmap.md)。

## 为什么

api-x 在 600K+ SSE / AGUI 响应上受 Compose 的全量文本重建限制。Api-Z 把数据通路改成：

- 每请求 goroutine + `context` 取消，多请求状态隔离。
- SSE 每行只解析一次，AG-UI 状态 O(delta) 更新。
- 原始字节落盘，列表按行虚拟化；AGUI 文本按块渲染，不再整篇布局。
- 单二进制、无 JVM/Skiko 基线。

实测（headless，Linux）：

| 场景 | 结果 |
|---|---|
| 10,000 个 SSE 事件解析 | 81ms |
| 600KB AGUI 文本，单帧渲染 | 中位 1.37ms / 最大 1.84ms |
| 536 个真实请求迁移 | 3 集合 / 59 文件夹 / 536 请求 / 100 artifacts |

## 构建与运行

```bash
make build      # CGO_ENABLED=0 go build -trimpath -o api-z .
make run
make test
make vet        # go vet + go tool mygo vet .
```

只读演练迁移 api-x 数据：

```bash
go run . -migrate \
  -api-x-home ~/.local/share/api-x \
  -data-dir /tmp/api-z-smoke \
  -config-dir /tmp/api-z-smoke-config
```

数据目录：Linux `~/.local/share/api-z`，Windows `%APPDATA%\api-z`，macOS `~/Library/Application Support/api-z`。
可用 `-data-dir` / `-config-dir` 或 `API_Z_DATA_DIR` / `API_Z_CONFIG_DIR` 覆盖。

## 功能（当前）

- 集合 / 文件夹 / 请求树（搜索、展开、键盘/鼠标选择）。
- 请求编辑：方法、URL、Params、Headers、Body（JSON 高亮编辑器）、Auth（inherit/noauth/bearer/basic/apikey）。
- 环境变量 `{{name}}` 替换、环境切换、集合/文件夹/请求级 Auth 继承。
- 发送 / 取消 / 120s 超时；状态码、耗时、大小、TTFT、SSE 事件数。
- 响应：Raw（虚拟化、自动跟随末尾）、Model（OpenAI/Responses delta 提取）、AG-UI（文本/工具调用/推理/中断卡片）、JSON 美化（只读高亮编辑器）。
- 多标签，每个标签独立的编辑与流式响应状态。
- HAR 落盘与最近 10 次历史回看。
- api-x 只读识别 + 一次性迁移（SQLite、environments.json、response artifacts）。

## 数据兼容

- 迁移只读打开 api-x 的 `collections.db`，不修改源目录。
- api-z 使用自己的 `api-z.db`（WAL）与 `request/<id>/response/*.har`。
- `environments.json` 与 api-x 同构；请求 `meta_json` 的 `auth` 语义保持一致。
- 迁移幂等：已有 `migrated_from` 时跳过；目标库非空时拒绝覆盖。

## 分层

```text
main.go -> ui -> app -> httpclient
```

- `httpclient/`：请求规格、`net/http` 执行器、SSE 解析、AG-UI 状态机、JSON delta 提取。
- `app/`：路径/日志、SQLite、迁移、环境/auth/URL、Session（spool + 行索引 + HAR）。
- `ui/`：mygo 视图层；请求编辑器、虚拟化响应列表、AG-UI 分块渲染、历史。

## 已知限制

- 原始超长行（>4096 字节）只显示截断，复制仍为全文。
- 暂无：Multipart、代理 UI、OpenAPI/Postman 导入导出、MCP、工作流/压测、请求发送前预览、响应搜索。
- AG-UI 的 `MESSAGES_SNAPSHOT` 只做基础还原；复杂 UI 组件（如自定义 interrupt UI）尚未渲染。
- 历史回看只读 HAR，不支持重新执行。
