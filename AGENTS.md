# AGENTS.md

本文件为在本仓库内工作的智能代理提供最小且实用的协作说明。

## 项目定位

- 本项目是一个 Go 实现的代理服务，为 CLI/SDK 提供 OpenAI、Gemini、Claude、Codex 等兼容接口。
- 入口程序位于 `cmd/server/main.go`。
- Go 模块名为 `github.com/router-for-me/CLIProxyAPI/v6`。
- 配置文件以根目录 `config.yaml` / `config.example.yaml` 为主。

## 目录结构

- `cmd/server`：程序入口、参数解析、登录模式、TUI 启动。
- `internal/api`：HTTP 服务、路由、管理接口。
- `internal/api/modules/amp`：Amp CLI / IDE 扩展相关集成。
- `internal/cmd`：服务启动和各类登录命令。
- `internal/config`：配置定义与加载。
- `internal/registry`：模型注册表与模型目录；`internal/registry/models/models.json` 是模型目录数据。
- `internal/translator`：协议/格式转换层，结构复杂，除非任务明确要求，否则不要随意改动。
- `internal/tui`：终端管理界面。
- `sdk/`：可复用 SDK，包含认证、代理服务、处理器、翻译注册等能力。
- `examples/`：SDK/自定义 provider 示例。
- `test/`：跨模块集成测试。
- `docs/`：SDK 和高级用法文档。

## 工作原则

- 优先做最小改动，保持现有包边界、命名风格和目录职责不变。
- 先修根因，不做表层补丁；不要顺手重构无关代码。
- 新逻辑优先放在已有包内，避免新增抽象层，除非当前模式已经明确要求。
- 修改 API、路由、鉴权或模型选择逻辑时，优先查看同目录下已有测试和相邻实现。
- 变更配置项时，同时更新 `config.example.yaml` 和相关文档说明。
- 若修改用户可见行为，优先补充或更新对应测试。

## 特别注意

- `config.yaml` 可能包含本地真实密钥、认证目录或代理配置；读取时注意脱敏，除非用户明确要求，不要把敏感值写回文档或回复里。
- 新增配置示例时，应写入 `config.example.yaml`，不要把示例值直接塞进用户本地 `config.yaml`。
- `internal/translator/**` 在 PR 工作流中有路径保护；除非任务明确落在翻译层，否则尽量避免修改该目录。
- `internal/registry/models/models.json` 在 CI 中会从外部模型仓库刷新；不要因为临时问题随意手工大改该文件。
- 管理接口涉及安全边界：`remote-management`、`secret-key`、localhost 限制等配置变更应保持默认安全策略。

## 常用命令

- 启动服务：`go run ./cmd/server -config config.yaml`
- 查看参数：`go run ./cmd/server -h`
- 构建入口：`go build ./cmd/server`
- 全量测试：`go test ./...`
- 定向测试：`go test ./internal/api/... ./sdk/...`
- 单包测试：`go test ./internal/api/modules/amp`
- 格式化：`gofmt -w <file>`

## 验证建议

- 优先运行与改动最接近的包测试，再考虑全量 `go test ./...`。
- 若只改入口、配置加载或管理接口，至少验证 `go build ./cmd/server`。
- 若改动流式响应、handler、auth 轮换、Amp 模块或 SDK builder，尽量补跑对应包测试。
- 若改动 `config.example.yaml` 或文档，检查示例字段名是否与 `internal/config` 中定义一致。

## 文档与示例

- 项目总览优先看 `README.md`。
- SDK 用法看 `docs/sdk-usage.md` 和 `docs/sdk-advanced.md`。
- 访问控制与 watcher 相关改动分别参考 `docs/sdk-access.md`、`docs/sdk-watcher.md`。
- 自定义 provider 或嵌入式使用方式可参考 `examples/custom-provider`。

## 提交前检查

- 代码已 `gofmt`。
- 改动仅覆盖当前需求，没有顺带修 unrelated 问题。
- 涉及配置、接口或行为变更时，示例/文档/测试已同步。
- 未泄露本地密钥、token、cookie、OAuth 文件路径等敏感信息。

## 提交信息规范

- 优先遵循历史提交的 Conventional Commits 风格：`type(scope): summary`。
- `type` 常用 `feat`、`fix`、`docs`、`refactor`、`chore`；仅在没有合适 scope 时使用 `type: summary`。
- `scope` 使用受影响模块或目录职责，如 `api`、`auth`、`codex`、`openai-compat`、`translator`。
- `summary` 使用英文、现在时、简洁描述单一改动，首字母不强制大写，末尾不加句号。
- 若改动直接对应某个历史 issue，且仓库当前已有相同先例，可使用 `Fixed: #1234`；否则默认仍用 Conventional Commits。
- 一次提交聚焦一个主题；不要把无关重构、格式化或顺手修复混进同一条提交。
