# API 与异步任务契约

## 1. 基础约定

- Base URL：/api/v1。
- 用户端品牌由访问域名解析；管理端必须显式提供品牌上下文。
- 所有接口返回 JSON；时间使用 UTC ISO-8601；积分使用十进制整数字符串。
- 认证：支持 `Authorization: Bearer <access_token>`；浏览器默认使用 HttpOnly、SameSite=Strict Cookie，不在 Web Storage 保存令牌。
- 请求追踪：X-Request-ID 必填或由网关生成。
- 所有有副作用的 POST/PATCH 必须支持 Idempotency-Key；当前键格式为 8–128 个 ASCII 字母、数字、`_ : . -`。
- 品牌后台请求使用 X-Brand-ID；服务端必须校验操作者是否拥有该品牌权限。
- 不接受客户端传入的 brand_id 作为唯一授权依据；品牌必须由域名、令牌和权限共同确定。

本文件同时包含已实现接口与后续阶段设计；当前接入范围以 [实施与验收记录](implementation-progress.md) 为准，不得将接口规格表视为全部已实现。

## 2. 响应格式

成功：

~~~json
{
  "success": true,
  "data": {},
  "request_id": "019..."
}
~~~

失败：

~~~json
{
  "success": false,
  "error": {
    "code": "BET_PERIOD_CLOSED",
    "message": "投注已截止",
    "details": {}
  },
  "request_id": "019..."
}
~~~

## 3. 用户端接口

### 品牌和认证

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /context | 根据域名返回品牌主题、语言、功能开关和登录配置 |
| POST | /auth/register | 用户注册并创建品牌成员 |
| POST | /auth/login | 用户名/手机号 + 密码登录 |
| POST | /auth/telegram | Telegram 授权登录 |
| POST | /auth/logout | 当前会话失效 |
| GET | /auth/challenge | 品牌启用验证码时返回一次性图形挑战 |
| GET | /auth/telegram/challenge | 品牌启用 Telegram 时返回一次性 OIDC nonce |
| GET | /me | 当前全局身份和品牌资料 |
| PATCH | /me/profile | 只允许补充首次未填写的用户名或手机号 |

注册请求必须包含 privacy_policy_version 和 service_terms_version。

### S2-a 已接入认证约定

- 用户路径同时支持 `/api/v1/...` 和平台入口 `/api/v1/b/{brandCode}/...`。路径品牌仅在已配置的平台入口可选；普通品牌域名不能借此访问另一品牌。
- 用户 Cookie 名为 `lottery_user_<无连字符品牌UUID>`，管理 Cookie 名为 `lottery_admin`；二者互不授予权限。用户会话 24 小时，管理会话 12 小时，无刷新端点。
- 所有变更请求要求 `application/json`。携带任何 Cookie 的变更请求须带同 Host 的 `Origin`；无 Cookie 的非浏览器 Bearer 客户端可不带 Origin。未知 JSON 字段、超过 16 KiB、尾随对象均拒绝。
- 注册须提供 username 或 phone 至少一项；用户端表单择一填写，API 允许同时提供两者。用户名为 3–32 个字符、首字符为字母，后续允许字母、数字和下划线，并规范为小写。手机号使用明确的 E.164 `+` 国家码。密码为 10–128 UTF-8 字节。
- 注册/首次加入品牌的政策版本必须匹配 `/context` 当前值，不允许客户端省略或提交历史版本。`dev-1` 仅为开发占位版本。
- 注册返回 201；登录返回 200，数据包括 `user`、`member`、`access_token`、`token_type`、`expires_at`。令牌仅供非浏览器客户端使用，浏览器使用响应 Cookie。
- 已有全局账号首次在另一品牌登录时返回 409 `BRAND_JOIN_REQUIRED`；用户接受该品牌条款后重新提交新的幂等操作，创建不同成员资料但共用全局凭证。
- `PATCH /me/profile` 仅补填从未填写的 username/phone，返回 `audit_log_id`；客户端随后 GET `/me`。已填写字段不可修改。
- 验证码请求字段为 `captcha_id`、`captcha_answer`；错误答案或错误密码均消耗挑战，五分钟过期，跨品牌不能使用。
- Telegram 请求字段为 `id_token`、`challenge_id`、`nonce`、两项政策版本；可通过 `bind: true` 将验证身份绑定到当前已登录全局账号。禁止换绑/解绑，不接收任意用户名作为授权证据。真实应用配置未提供时默认关闭。
- 认证限流持久化到主库，多实例共享；初始每 IP 30 次、每标识账号 10 次/5 分钟。同一幂等操作重放不重复计数。代理 IP 仅信任显式配置的代理网段。
- 幂等摘要使用 HMAC，最终响应加密保存。相同键同内容重放原结果；同键不同内容返回 409。会话已撤销/过期后重放登录响应不会让旧会话恢复有效。

注册示例：

~~~json
{
  "username": "demo_member",
  "password": "example-not-a-production-password",
  "privacy_policy_version": "dev-1",
  "service_terms_version": "dev-1"
}
~~~

### 游戏和投注

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /games | 当前品牌已启用彩种 |
| GET | /games/{gameId} | 彩种展示配置和可用玩法 |
| GET | /games/{gameId}/periods/current | 当前期次及投注时间 |
| GET | /games/{gameId}/plays | 当前有效玩法版本 |
| POST | /bet-previews | 校验选号、展开复式、计算总积分，不产生订单 |
| POST | /bet-orders | 幂等创建注单并扣除积分 |
| GET | /bet-orders | 当前品牌注单列表 |
| GET | /bet-orders/{orderId} | 注单详情和结算明细 |
| POST | /bet-orders/{orderId}/cancel | 按配置取消并原路退款 |
| GET | /draw-results | 开奖结果列表 |

投注创建请求示例：

~~~json
{
  "period_id": "019...",
  "play_id": "019...",
  "rule_version": 3,
  "selection": {
    "regular": [1, 2, 3, 4, 5, 6],
    "special": [7]
  },
  "multiplier": 2
}
~~~

服务端不得信任客户端提供的注数、赔率、总积分或规则计算结果；这些字段只能由服务端重新计算。

### 积分、充值和提现

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /wallet | 显示、可用、冻结、提现和来源积分 |
| GET | /wallet/ledger | 分页账本，支持来源/业务类型筛选 |
| GET | /recharges | 用户充值记录 |
| POST | /withdrawals | 提交提现申请 |
| GET | /withdrawals | 提现列表 |
| GET | /withdrawals/{withdrawalId} | 提现详情和状态变化 |

提现提交必须在事务中完成：校验资格 → 锁定账户 → 计算来源分配 → 转入提现积分 → 写流水 → 创建申请。

## 4. 管理端接口

### S2-a 已接入管理账号与成员操作

| 方法 | 路径 | 授权/行为 |
|---|---|---|
| POST | /admin/auth/login | 管理账号密码认证，独立 Cookie |
| POST | /admin/auth/logout | 撤销当前管理会话 |
| GET | /admin/me | 当前管理账号、品牌范围、多个角色去重后的权限 |
| GET | /admin/brands | 仅列出已授权品牌，平台查看权限可跨品牌 |
| GET | /admin/users | `user.view.brand` 或 `user.view.platform` |
| PATCH | /admin/users/{id} | `user.write.brand`；status、notes、reason |
| POST | /admin/users/{id}/kick | `user.kick.brand`；reason 必填，仅撤销该品牌成员会话 |
| POST | /admin/users/{id}/reset-password | `user.password_reset.brand`；password、reason，修改全局密码并撤销该用户全部品牌会话 |
| GET | /admin/audit | `audit.view.brand` 或 `audit.view.platform` |

上述 `{id}` 是品牌成员 UUID，不是全局用户 UUID。品牌操作必须传 `X-Brand-ID`，且服务端校验角色与品牌范围。平台查看权限可不带品牌头查询所有品牌；超级管理员不能修改用户、重置密码或踢人，即使误配相应角色也拒绝。

成员状态值为 `normal`、`frozen`、`disabled`、`expired`、`cancelled`（注销）。冻结仍可登录/查询；禁用、过期、注销后会话验证失败。备注最大 2000 字节，原因必填且不超过 500 字节。修改和踢人返回 `audit_log_id`。查询支持 `limit`（1–100，默认 50）和 `offset`（0–1000000），响应 `{items: [...]}`。成功查询和修改都有审计记录，密码/令牌不进入明文审计。

共享密码安全保护：当前默认要求管理员对该用户所有已加入品牌均拥有 `user.password_reset.brand`，不满足返回 403 `CREDENTIAL_SCOPE_REQUIRED`，不改密码、不撤销会话。此跨品牌权限规则已向用户提出确认，未确认前保留限制；不能仅凭当前品牌的重置权限接管其他品牌身份。超级管理员仍不能重置用户密码。

管理账号当前由显式 CLI 引导，无内置密码。角色/权限编辑、管理账号管理、后台新增用户、拒绝访问专项安全审计仍属于 S2 后续工作。

### 期次和开奖

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /admin/periods/generate | 按计划幂等生成期次 |
| POST | /admin/periods/{id}/close | 截止投注 |
| POST | /admin/periods/{id}/cancel | 期次投注取消/判定取消并退款 |
| POST | /admin/periods/{id}/manual-draw | 人工开奖 |
| POST | /admin/draw-results/{id}/confirm | 确认外部/人工结果 |
| POST | /admin/draw-results/{id}/correct | 纠正结果并建立回溯任务 |
| POST | /admin/periods/{id}/settle | 触发期次结算 |
| POST | /admin/settlements/{id}/retry | 仅允许重试可重试失败，不重试异常注单 |
| GET | /admin/periods/{id}/timeline | 状态、开奖、结算和审计时间线 |

### 玩法和配置

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /admin/rule-versions | 创建草稿 |
| POST | /admin/rule-versions/{id}/validate | 校验和模拟 |
| POST | /admin/rule-versions/{id}/submit-review | 提交审核 |
| POST | /admin/rule-versions/{id}/approve | 品牌管理员审核通过 |
| POST | /admin/rule-versions/{id}/reject | 驳回并记录原因 |
| POST | /admin/rule-versions/{id}/publish | 立即或下期生效 |
| POST | /admin/rule-versions/{id}/rollback | 创建回滚版本 |

### 资金、用户和代理

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /admin/recharges | 创建人工充值单 |
| POST | /admin/recharges/{id}/confirm | 单人确认入账 |
| POST | /admin/withdrawals/{id}/approve | 审核通过 |
| POST | /admin/withdrawals/{id}/reject | 驳回并填写理由 |
| POST | /admin/withdrawals/{id}/cancel | 取消处理 |
| POST | /admin/withdrawals/{id}/mark-paid | 第一阶段内部处理标记成功 |
| GET | /admin/agents/tree | 代理树 |
| POST | /admin/commission-rules | 创建佣金版本 |
| POST | /admin/commission-cycles/{id}/settle | 周/月结算 |
| POST | /admin/commission-records/{id}/adjust | 人工修正并保留原记录 |

所有管理端动作必须做权限检查并写审计日志；返回结果应包含 audit_log_id 或可追踪 request ID。

## 5. 错误码

至少定义以下稳定错误码：

- BRAND_PAUSED
- BRAND_ACCESS_DENIED
- AUTH_INVALID_CREDENTIALS
- AUTH_SESSION_REVOKED
- PERMISSION_DENIED
- PERIOD_NOT_FOUND
- PERIOD_NOT_BETTING
- PERIOD_CLOSED
- RULE_VERSION_INVALID
- SELECTION_INVALID
- BET_LIMIT_EXCEEDED
- INSUFFICIENT_POINTS
- IDEMPOTENCY_CONFLICT
- ORDER_ALREADY_CANCELLED
- CANCEL_NOT_ALLOWED
- WITHDRAWAL_IN_PROGRESS
- WITHDRAWAL_TURNOVER_NOT_MET
- RESULT_INVALID
- SETTLEMENT_NOT_RETRYABLE
- LEDGER_CONFLICT

## 6. 事务、幂等和重试

- 幂等键必须保存请求摘要；相同键不同请求内容返回 IDEMPOTENCY_CONFLICT。
- 相同键相同内容返回第一次成功或最终失败结果，不重新执行业务。
- 事务提交后才发布 Outbox 事件；消费者按事件 ID 幂等。
- 外部开奖源请求可以重试；人工开奖和结果纠正不得自动重试。
- 结算失败由运营人员手动触发重试；异常注单不进入普通重试。
- 从库延迟时，写操作返回主库结果，前端在短时间内使用主库粘滞读取。
