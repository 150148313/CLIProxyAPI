# Auth Files 401 一键清理方案

## 目标

为管理端增加一个“清理无效凭证”的能力：

- 基于当前 `GET /v0/management/auth-files` 返回的凭证列表。
- 自动识别其中已经失效、且状态表现为 `401` 的凭证。
- 支持一键删除这些无效凭证对应的 auth file。
- 删除后同步从运行时 `authManager` 和持久化存储中移除。

## 当前现状

### 现有接口

- 列表接口：`GET /v0/management/auth-files`
- 单个/批量删除接口：`DELETE /v0/management/auth-files`

当前删除接口已经支持：

- 单个删除：`?name=xxx.json`
- 批量删除：多个 `name` 参数或 JSON body 传 `names`
- 全部删除：`?all=true`

### 列表返回的关键字段

当前 `auth-files` 列表里已返回：

- `name`
- `status`
- `status_message`
- `disabled`
- `unavailable`

但目前没有明确暴露一个稳定的 HTTP 状态字段用于直接判断 `401`。

## 核心判断建议

### 不建议

不建议只靠 `status_message` 是否包含字符串 `401` 来判断：

- `status_message` 更偏向人类可读文本，格式可能变化。
- 不同 provider 的报错文案可能不同。
- 未来文案一变，前端或接口逻辑就容易误判。

### 建议

优先基于运行时错误对象里的 HTTP 状态码判断：

- `auth.LastError.HTTPStatus == 401`

如果 `ListAuthFiles` 当前没有暴露该字段，则先在列表返回中补出只读字段，例如：

- `last_error_http_status`
- 可选再补：`last_error_code`
- 可选再补：`last_error_message`

这样后续前端和批量清理接口都可以基于结构化字段判断，而不是解析文案。

### 针对当前实际返回的补充判断

结合当前 `codex` 凭证的实际返回样例，`status_message` 里可能不是普通文本，而是一段 JSON 字符串，例如：

```json
{
  "error": {
    "message": "Encountered invalidated oauth token for user, failing request",
    "code": "token_revoked"
  },
  "status": 401
}
```

这类数据说明：

- `status_message` 虽然名义上是文本字段
- 但实际上已经承载了结构化错误信息
- 对当前项目来说，它可以作为识别“无效 OAuth 凭证”的有效依据

因此建议将判断优先级设计为：

1. 优先使用 `last_error_http_status == 401`
2. 若该字段暂未暴露，则尝试把 `status_message` 解析为 JSON
3. 当解析成功且满足以下任一条件时，视为命中：
   - `status == 401`
   - `error.code == "token_revoked"`
4. 若 `status_message` 不是 JSON，则第一版不要只靠普通字符串模糊匹配做真实删除

也就是说，针对你提供的这个样例：

- `status == error`
- `source == file`
- `runtime_only == false`
- `status_message` 可解析出 `status: 401`
- `error.code` 为 `token_revoked`

这类凭证应当进入“一键清理”的删除候选集。

## 风险与边界

这部分是为了避免“一键清理”误删仍可用的凭证。

### 单模型 401 不等于整份凭证失效

当前运行时里，某个模型执行失败时，错误可能会被提升到 auth 级别状态中。

这意味着：

- 某个模型返回了 `401`
- 并不一定代表整份 auth file 对所有模型都已经失效

因此第一版方案不应只看“最近一次错误是否为 `401`”，而要尽量区分：

- 是 auth 级别失败
- 还是某个单独模型失败

### authManager 不可用时无法安全判断

`GET /v0/management/auth-files` 在 `authManager == nil` 时会退化为直接扫磁盘文件。

这类返回只包含：

- 文件名
- 基础元数据
- JSON 里的部分字段

但不包含稳定的运行时错误状态，因此：

- “清理 401 凭证”接口必须依赖 `authManager`
- 不能在纯磁盘回退模式下执行真实清理

建议在这种情况下直接返回：

- `503 Service Unavailable`
- 错误信息：`core auth manager unavailable`

### 只允许删除文件型凭证

管理端列表里同时存在：

- 文件型凭证
- 仅运行时存在的凭证（runtime only）

一键清理的目标是“删除 auth-files”，所以第一版必须限定：

- 只处理 `source == file`
- 只处理 `runtime_only == false`

不应误删纯内存态或运行时合成凭证。

## 删除资格条件

为了降低误删概率，建议第一版不要只用一个条件，而采用更收敛的命中规则。

### 建议命中条件

只有同时满足以下条件才允许进入删除候选集：

- `source == file`
- `runtime_only == false`
- `disabled == false`
- `status == error`
- 且满足以下任一条件：
  - `last_error_http_status == 401`
  - `status_message` 解析为 JSON 后 `status == 401`
  - `status_message` 解析为 JSON 后 `error.code == "token_revoked"`

### 关于模型级错误的额外约束

如果后端能区分 auth 级错误和 model 级错误，建议进一步收紧：

- 第一版仅处理 auth 级 `401`
- 不处理仅单个模型 `401` 的情况

如果短期内不方便精确区分，也建议至少：

- 先只做 `dry_run`
- 在确认弹窗里明确展示候选文件和原因
- 等观察一轮实际数据后再开放真实删除

## 返回信息补充建议

为了方便排查误删和前端确认，清理接口除了 `matched/deleted/files/failed` 外，建议再返回候选明细。

建议增加：

- `candidates`
- 每项包含 `name`
- 每项包含 `id`
- 每项包含 `status`
- 每项包含 `status_message`
- 每项包含 `last_error_http_status`
- 每项包含 `source`

示例：

```json
{
  "dry_run": true,
  "matched": 2,
  "candidates": [
    {
      "name": "alpha.json",
      "id": "alpha.json",
      "status": "error",
      "status_message": "unauthorized",
      "last_error_http_status": 401,
      "source": "file"
    }
  ]
}
```

## 推荐实现方案

### 方案目标

最小化改动，尽量复用现有删除逻辑，不重复发明“删除 auth file”的底层能力。

### 分阶段方案

#### 第一阶段：补齐列表判断字段

在 `buildAuthFileEntry` 里向 `GET /v0/management/auth-files` 返回值补充：

- `last_error_http_status`
- `last_error_code`
- `last_error_message`
- 可选保留原始 `status_message`，供兼容解析

如有需要，也可补：

- `has_model_errors`
- `model_error_summary`

用途：

- 前端/TUI 可以直观看到该凭证最近一次错误是不是 `401`
- 为后续“一键清理 401”提供稳定筛选依据

#### 第二阶段：增加专用清理接口

新增一个专用管理接口，例如：

- `POST /v0/management/auth-files/cleanup`

建议 body：

```json
{
  "match": {
    "last_error_http_status": 401
  },
  "dry_run": false
}
```

接口行为：

- 若 `authManager == nil`，直接返回 `503`
- 遍历当前 `authManager.List()` 结果
- 只保留文件型 auth
- 筛出满足“删除资格条件”的 auth
- 必要时兼容解析 `status_message` 中的 JSON 错误对象
- 收集其 `name` / `path` / `id`
- 复用现有 `deleteAuthFileByName()` 完成删除
- 返回删除结果汇总

建议返回结构：

```json
{
  "status": "ok",
  "matched": 3,
  "deleted": 3,
  "files": ["a.json", "b.json", "c.json"],
  "failed": []
}
```

#### 第三阶段：支持 dry-run

`dry_run=true` 时：

- 只返回将要删除的文件列表
- 不实际删除

用途：

- 前端点击“一键清理”前可先预览
- 降低误删风险

## 为什么推荐新增专用清理接口

虽然前端也可以先调用 `GET /v0/management/auth-files`，筛出 `401`，再调用已有批量删除接口，但专用接口更合适：

- 判断逻辑统一放在服务端
- 避免前端重复实现筛选规则
- 后续想从 `401` 扩展到 `403` / `invalid_grant` / `token revoked` 时更容易演进
- 可直接支持 `dry_run`
- 可在服务端统一控制“只删文件型凭证”“排除 runtime-only”“限制 auth 级 401”

## 删除匹配规则建议

建议第一版只删除满足以下条件的凭证：

- `source == file`
- `runtime_only == false`
- `disabled == false`
- `status == error`
- 并满足以下任一条件：
  - `last_error_http_status == 401`
  - `status_message` 可解析 JSON 且 `status == 401`
  - `status_message` 可解析 JSON 且 `error.code == "token_revoked"`

不建议第一版混入过多模糊条件，例如：

- `status == error`
- `status_message contains "unauthorized"`
- `status_message contains "expired"`

原因：

- 这些条件误伤概率更高
- `status == error` 单独使用过于宽泛
- `status_message` 属于文案字段，不适合做唯一判定依据
- 先把结构化 `401` 路径做稳更安全
- 但若 `status_message` 本身就是结构化 JSON，则可以作为兼容判断来源

## 返回结果建议

专用清理接口建议返回：

- `matched`：命中的无效凭证数量
- `deleted`：实际删除成功数量
- `files`：成功删除的文件名列表
- `failed`：删除失败列表，包含 `name` 和 `error`
- `dry_run`：是否为预演模式
- `candidates`：命中候选及命中原因，用于确认和审计

如果部分失败，可沿用现有删除接口风格，返回 `207 Multi-Status`。

## 前端 / 管理页建议

管理页可新增一个按钮：

- 文案：`清理 401 凭证`

交互建议：

- 点击后先调用 `dry_run=true`
- 弹窗展示命中的文件数量和文件名
- 弹窗同时展示命中原因，例如 `status=error`、`http_status=401`
- 用户确认后再发起真实删除
- 删除成功后刷新 `auth-files` 列表

## 实现落点建议

### 后端

- 路由注册：`internal/api/server.go`
- 处理逻辑：`internal/api/handlers/management/auth_files.go`

### 复用点

- 复用 `ListAuthFiles` 的数据组织方式
- 复用 `deleteAuthFileByName()` 的删除逻辑
- 复用现有批量删除返回结构风格

## 测试建议

建议新增以下测试：

1. `ListAuthFiles` 返回 `last_error_http_status`
2. `cleanup` 在 `authManager == nil` 时返回 `503`
3. `cleanup` 仅匹配文件型 `401`，不删除 `runtime_only`
4. `cleanup` 能识别 `status_message` 中的 JSON `status: 401`
5. `cleanup` 能识别 `status_message.error.code == token_revoked`
6. `cleanup` 不删除 `429/500`
7. `cleanup` 在 `dry_run=true` 时不实际删除文件
8. `cleanup` 删除后，文件已从磁盘移除，列表也不再返回
9. 部分删除失败时返回 `207`，并包含 `failed` 明细
10. 若存在仅单模型 `401` 的场景，验证第一版不会误删

测试文件建议放在：

- `internal/api/handlers/management/auth_files_delete_test.go`
- 或新建 `internal/api/handlers/management/auth_files_cleanup_test.go`

## 建议的最小落地版本

如果希望先快速上线，建议按下面顺序做：

1. `GET /v0/management/auth-files` 增加 `last_error_http_status`
2. 新增 `POST /v0/management/auth-files/cleanup`
3. 第一版只处理文件型、auth 级 `401`，并兼容 `status_message` 中的 JSON `401/token_revoked`
4. 支持 `dry_run`
5. 返回 `candidates` 供前端确认
6. 管理页增加“清理 401 凭证”按钮

这样能以较小改动完成“一键删除无用凭证”的目标，同时保证判断逻辑足够稳定。
