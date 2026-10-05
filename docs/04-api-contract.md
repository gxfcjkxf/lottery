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

共享密码安全保护（用户已确认）：要求管理员对该用户所有已加入品牌均拥有 `user.password_reset.brand`，不满足返回 403 `CREDENTIAL_SCOPE_REQUIRED`，不改密码、不撤销会话。不能仅凭当前品牌的重置权限接管其他品牌身份。超级管理员仍不能重置用户密码。

### S2-b 已接入账号、角色与认证配置

下表均要求有效的 `X-Brand-ID` UUID，即使操作者拥有平台权限也必须明确选择目标品牌。权限为精确的 `resource.action.scope`，不隐式授予查询权或通配权。

| 方法 | 路径 | 授权/请求 |
|---|---|---|
| GET | /admin/permissions | `role.view.brand/platform`；返回登记的品牌权限键 `{items}` |
| GET | /admin/roles | `role.view.brand/platform`；分页 `{items}` |
| POST | /admin/roles | `role.write.brand/platform`；code、name、permissions、reason；status 默认 active |
| PATCH | /admin/roles/{id} | 同上；version、name、status、permissions、reason；code 不可修改 |
| GET | /admin/accounts | `admin.view.brand/platform`；分页 `{items}` |
| POST | /admin/accounts | `admin.write.brand/platform`；username、password、role_ids、reason；创建普通 active 账号 |
| PATCH | /admin/accounts/{id} | 同上；version、status、role_ids、reason；username 不可修改 |
| POST | /admin/accounts/{id}/reset-password | 同上；version、password、reason；撤销目标账号全部会话 |
| POST | /admin/users | 仅 `user.create.brand` 且非超级管理员；username/phone 至少一个、password、可选 display_name/notes、reason |
| GET | /admin/auth-settings | `auth_config.view.brand/platform`；当前 version、认证设置和只读条款版本 |
| PATCH | /admin/auth-settings | `auth_config.write.brand/platform`；version、captcha_enabled、telegram_enabled、telegram_client_id、reason |

管理写入沿用持久化幂等、审计和 JSON 严格解析；查询分页与成员列表一致。Role 响应包含 id、brand_id、code、name、status、version、is_bootstrap、permissions；Admin 响应包含 id、username、status、version、super_admin、brand_ids、role_ids、role_codes。成功写入返回 audit_log_id。版本冲突或唯一键冲突返回 409 `VERSION_CONFLICT`；认证配置版本冲突为 `CONFIG_VERSION_CONFLICT`。

安全边界：

- 角色绑定单个品牌；角色归属和 code 在数据库层不可迁移。品牌角色不能把权限传播给同一管理员所属的其他品牌。平台角色由服务器端维护，不在品牌角色编辑器创建。
- 品牌操作者只能授予自己在目标品牌已拥有的权限，包括分配已有角色；也不能修改旧权限超出自身的角色，或通过状态修改/重置密码接管更高权限管理员。平台对应 write 权限可管理品牌角色和普通账号，但仍不能编辑自身账号、自身所用角色、引导角色或超级管理员账号。
- 品牌管理员只能管理单品牌目标管理账号；多品牌账号由具有平台管理权限的操作者管理。跨品牌/平台角色的目标账号不能被局部品牌权限接管。普通 API 不创建超级管理员，不修改账号品牌范围；显式服务器 CLI 引导无默认密码。
- 管理账号密码 16–128 字节；username 匹配 `[a-z][a-z0-9_]{2,31}`，role code 匹配 `[a-z][a-z0-9_]{2,47}`，角色名最多 120 UTF-8 字节；原因 1–500 字节，权限/角色数组最多 100 项，去重后处理。
- `/admin/me` 的 `permissions_by_brand` 与 `platform_permissions` 是授权显示的权威字段；旧 `permissions` 仅为兼容的扁平并集，不能用它推导任意品牌可写。
- 写事务在权限读写锁内重查当前角色和会话：普通业务管理写取共享锁，角色/账号权限变更取排他锁。此锁不用于用户投注，不能由此推导业务吞吐。权限变更后会话实时读取最新权限；账号角色/状态/密码变更同时撤销目标会话。
- 已认证但无权限的请求写独立 `access.denied` 审计，不记录请求正文、密码、令牌或无权访问对象内容；业务回滚后拒绝审计仍保留。

运营新增用户只创建全局新身份、当前品牌成员与零积分账户；全局用户名/手机号已存在时返回 409 `IDENTITY_EXISTS`，不得覆盖/关联旧身份。`join_method=operator`，`terms_accepted=false`，不签发会话、不产生资金流水。用户首次登录须本人确认当前条款，再记录实际同意时间。

认证配置保存立即生效，只允许上述三个公开登录字段，不修改条款文本/版本；version 对应品牌 config_version。Client ID 使用十进制字符串，非空必须是无前导零的正安全整数（≤9007199254740991），启用 Telegram 时不得为空。公开 ID 不是秘密凭证；真实外部应用授权仍需独立验收。

### S3-a 已接入积分账本与人工财务

积分参数/响应一律是 canonical 十进制字符串，禁止 JSON number、浮点、指数、正号或多余前导零；正金额范围 1–9223372036854775807。余额和所有 12 桶的合计不得超过 int64 上限，源/状态桶最终不得为负。积分不过期。

| 方法 | 路径 | 授权/请求 |
|---|---|---|
| GET | /wallet | 当前品牌已登录成员的真实 Wallet |
| GET | /wallet/ledger | 本人当前品牌追加账本；limit/offset 分页 |
| GET | /admin/wallets/{memberID} | `wallet.view.brand/platform`；必须指定 X-Brand-ID |
| GET | /admin/wallets/{memberID}/ledger | 同上；倒序分页 |
| GET | /admin/wallets/{memberID}/reconciliation | 同上；根据完整账本重建并检查余额，不改写数据 |
| GET | /admin/recharges | `recharge.view.brand/platform`；可选 member_id 与分页 |
| POST | /admin/recharges | `recharge.write.brand`；member_id、points、reason，可选 proof_reference/remark |
| POST | /admin/recharges/{id}/confirm | 同上；version、reason；pending → confirmed 且事务内入账 |
| POST | /admin/wallets/{memberID}/freeze | `wallet.freeze.brand`；points、reason；默认来源顺序转至 manual_frozen |
| POST | /admin/wallets/{memberID}/unfreeze | 同上；entry_id、reason；仅原人工冻结整笔原路返还 |
| POST | /admin/wallets/{memberID}/adjust | `wallet.adjust.brand`；source、delta、reason；仅调整指定来源 available 桶 |

用户接口同时支持既有 `/api/v1/b/{brandCode}` 入口，后台全部要求有效品牌 UUID。首次运营创建但未同意条款的成员不能通过钱包认证。财务写入只开放品牌范围；目前超级管理员只读，即使误配品牌写权限仍拒绝（其资金管理权限的最终业务范围待确认）。系统冻结、投注扣款、派奖和通用冲正只提供后端事务原语，不开放客户端万能余额变更接口。

Wallet 含 account_id、brand_id、member_id、version 与 display_points、available_points、frozen_points、withdrawal_points、recharge_points、winning_points、gift_points、manual_frozen_points、system_frozen_points。后三种来源字段是各来源的**可用**余额，不能当作额外的一份积分；by_source 才是完整矩阵：

~~~json
{"recharge":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"},"winning":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"},"gift":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"}}
~~~

显示积分=可用+人工冻结+系统冻结，不包括提现中。Entry 含 id、品牌/账户/成员 ID、version、entry_type、业务引用、operation_key、before_snapshot、delta_snapshot、after_snapshot（全部 12 桶）、source_allocation、reason、actor_type/actor_id、request_id、created_at、可选 reversal_of。delta 允许负数字符串。退款/冲正必须关联原记录、使用原分配的精确反向变动，原流水不可覆盖；同一原流水最多一笔全额补偿。

充值单创建返回 201 pending，不增加余额；确认返回 confirmed、ledger_entry_id、audit_log_id。版本冲突或新操作键重复确认返回 409，不二次到账；同一幂等键原请求重放原成功结果。凭证引用可选，不提供文件上传或真实支付；备注可选，创建和确认原因都必填（1–500 UTF-8 字节）。单人可创建并确认。

Reconciliation 返回 consistent、account_id、member_id、version、entry_count、expected/actual（完整矩阵）、issues。关键查询走主库；对账在共享账户锁内扫描版本链，写入在账户排他锁内校验上一条 after 与当前余额。没有账本却有余额、缺失桶或不一致时停止新增记账，返回 `POINTS_RECONCILIATION_REQUIRED`，不能用人工调整绕过损坏。S3-b 提供下述明确修复流程与品牌限额；大规模异步对账仍待后续实现。

错误：400 `POINTS_INPUT_INVALID`/`POINTS_LIMIT_EXCEEDED`；404 `POINTS_RECORD_NOT_FOUND`；409 `POINTS_INSUFFICIENT`/`POINTS_OPERATION_CONFLICT`/`POINTS_RECONCILIATION_REQUIRED`。资金写入复用持久化幂等与新权限重查，业务单、桶余额、追加账本、账户版本和审计共同提交或回滚。

### S3-b 品牌积分限额、充值取消与差错修复

| 方法 | 路径 | 授权/请求 |
|---|---|---|
| GET | /admin/point-policy | `point_policy.view.brand/platform`；X-Brand-ID 必填 |
| PUT | /admin/point-policy | `point_policy.write.brand`；version、三个完整限额字段、reason 必填 |
| POST | /admin/recharges/{id}/cancel | `recharge.write.brand`；version、reason；仅 pending → cancelled，不产生积分 |
| GET | /admin/wallets/{memberID}/repair-preview | `wallet.view.brand/platform`；只读完整账本重建与差错预览 |
| POST | /admin/wallets/{memberID}/repair | `wallet.repair.brand`；version、token、reason，不接受新余额 |
| GET | /admin/wallets/{memberID}/repairs | `wallet.view.brand/platform`；limit/offset，追加修复记录查询 |

Policy 返回 brand_id、version、max_balance_points、max_recharge_points、max_adjustment_points，以及保存后的 audit_log_id。三个限额均为规范正整数字符串或显式 null（不限），默认 null；PUT 是完整替换，漏字段、未知字段和重复键拒绝，不以漏键隐式清空限制。配置仅针对本品牌，版本比较、成功审计与幂等共同提交，立即生效。

- 余额上限针对每个品牌成员全部 12 个分项合计；只限制净增加的非冲正记账。降低上限不会扣已有余额或阻止扣款、状态迁移和原路退款；所有路径仍受非负和 int64 安全上限约束。
- 单笔充值上限在创建和确认时都检查最新配置；确认时若配置已降低，失败后充值保持 pending、余额不变。取消仍允许。
- 单次调整上限针对人工调整绝对金额，正负调整都检查，不替代来源余额不足校验。
- 超限返回 409 `POINTS_POLICY_LIMIT_EXCEEDED`；版本/状态/修复预览冲突返回 409 `POINTS_OPERATION_CONFLICT`。失败不创建半笔资金记录。不同幂等键的重复取消/确认均不能再次改变状态。

RepairPreview 含 account_id、member_id、version（观察到的账户版本）、ledger_version、actual（实际存在的来源/状态分项，缺失不补零）、expected（原账本完整重建矩阵）、consistent、repairable、issues、token。预览验证完整版本链、请求摘要、前后快照、来源分配和冲正引用；账本无法证明完整时拒绝修复。token 绑定品牌、账户、全部观察值及原流水摘要。提交修复在账户排他锁内重新计算；余额、版本或流水变化则拒绝，要求重新预览，不允许使用过时结果。

差错修复仅重建账本的余额投影及账户版本，不属于充值/派奖/调账，因此不伪造经济流水、不覆盖或删除旧流水。修复会追加不可改写的 point_balance_repairs（含完整实测前快照、重建后快照、原因、操作人、请求和时间）及审计，并与余额恢复同事务提交；缺失分项可按完整原账本补建。真实经济金额修正仍须使用追加调整/冲正流水。无差错时不可重复修复，账本自身损坏则停写并调查；后续专门离线恢复流程不得靠自填金额替代。

管理页面提供品牌限额编辑、待确认充值取消、差错会员预览/明确确认和修复记录查询，权限独立。当前超级管理员资金写入的临时边界与 S3-a 相同，业务最终范围仍待确认。

### 期次和开奖（后续实现）

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

### S4-a 已接入：后台玩法模拟（仅计算）

| 方法 | 路径 | 授权/请求 |
|---|---|---|
| POST | `/api/v1/admin/rule-simulations` | `rule.simulate.brand`（目标品牌角色）或 `rule.simulate.platform`（平台角色）；必须传 `X-Brand-ID` |

此端点要求有效管理会话（`lottery_admin` Cookie 或 Bearer 管理令牌），并在服务器按所选品牌重查会话和权限。平台权限也必须显式选择目标品牌；`brand_id` 不放在请求正文中。前端使用 same-origin 凭证并发送 `X-Brand-ID`、`Idempotency-Key`。若使用 Cookie，变更请求必须有与 Host 同源的 `Origin`；不可信来源返回 403 `CSRF_REJECTED`。请求需为 `application/json`，最大 16 KiB；未知字段、畸形/尾随 JSON 或超限返回 400 `REQUEST_INVALID`。玩法 Definition 的各层 schema 闭合，未知字段和重复键拒绝；完整 DSL 和边界见 [玩法规则引擎 S4-a DSL](03-rule-engine.md#8-s4-a-可执行-dsl-v1-实际实现)。

请求 JSON 的外层结构与类型如下；积分金额和倍数是规范十进制字符串，赔率是精确十进制字符串，不是 JSON number：

~~~json
{
  "definition": {
    "schema_version": 1,
    "model": {
      "model": "X_PLUS_Y",
      "regular_pool": { "min": 1, "max": 49, "values": [], "allow_repeat": false },
      "special_pool": { "min": 1, "max": 49, "values": [], "allow_repeat": false },
      "regular_count": 6, "special_count": 1, "pool_size": 0, "total_count": 0, "length": 0,
      "allow_repeat": false, "ordered": false
    },
    "selection": { "mode": "numbers", "regular_count": 0, "special_count": 1, "exclude_count": 0, "attribute_groups": [], "feature_choices": {} },
    "number_attributes": {},
    "unit_points": "1",
    "prize_tiers": [{ "code": "SPECIAL_MATCH", "condition": { "op": "equals", "field": "special_match", "value": 1 }, "odds": "35", "exclusive": true, "cap_points": null }],
    "mixed_tier_policy": "max_all", "cap_points": null,
    "rounding": "half_up", "rounding_scope": "order",
    "limits": { "max_combinations": 100, "max_multiplier": "1000", "max_bet_points": null }
  },
  "selection": { "regular": [], "special": [7, 19], "digits": [], "exclude": [], "attributes": {}, "features": {} },
  "draw": { "regular": [1, 2, 3, 4, 5, 6], "special": [7], "digits": [] },
  "multiplier": "2"
}
~~~

`definition` 使用 `rules.Definition` 的 schema-v1 结构；号码模型、投注选择、奖级和 DSL 条件字段以 [03-rule-engine.md](03-rule-engine.md) 为准。`selection` 接受 regular/special/digits/exclude/attributes/features 六类字段，但不要求发送不适用的空字段；由 selection mode 和 model 要求的选号组必须提供并通过校验，其他组可省略（或保持空值）。`draw` 同理，只需提供当前 model 要求的开奖结果组，其他组可省略或为空。倍数为正整数字符串且不得超过 definition 的 max_multiplier。

成功返回 HTTP 200，标准 `{success,data,request_id}` envelope；`data` 是完整模拟结果，不含 `audit_log_id`：

- 顶层：`won`、`normalized`、`combination_count`、`multiplier`、`bet_points`、`prize_points`、`raw_prize_points`、`capped_prize_points`、`lines`、`warnings`。所有积分/精度字段均为 JSON 字符串：`multiplier`、`bet_points`、`prize_points` 是规范整数金额；`raw_prize_points`、`capped_prize_points` 是 `math/big.Rat.RatString` 精确值，可能为 `"n/d"`，分母为 1 时为整数文本。
- `lines[]` 含展开后的 `selection`、整数舍入后的 `points` 及每个奖级的 `hits[]`；每个 hit 含 `code`、`exclusive`、`matched`、`selected`、`raw_points`、`capped_points`、`points`、`trace`。`raw_points` 与 `capped_points` 同为 RatString 精确有理数（可为 `"n/d"`）；hit 的 `points` 是整数舍入值字符串。Trace 返回实际值、匹配结果和所有子节点；all/any 不短路隐藏解释。

成功计算在同一事务追加 `rule.simulate` 审计，资源类型为 `rule_simulation`；审计记录规则定义 SHA-256 摘要及组合数、投注积分、派奖积分、是否中奖，不记录完整请求正文。相同幂等请求重放返回原结果而不重复审计。幂等键为 8–128 个 ASCII 字母/数字或 `_ : . -`；同键不同正文返回 409 `IDEMPOTENCY_CONFLICT`。计算产生的最终 4xx 结果会按幂等键保存；改动输入后应使用新键。瞬时/存储错误返回 503 `SERVICE_UNAVAILABLE`，不作为成功结果重放。

该接口只运行模拟：不创建注单/订单、不扣款或写任何积分桶/账本、不派奖、不发布玩法，也不提交审核或批准规则。审计是模拟操作记录，不代表规则版本生命周期动作。它不会证明商业赔率安全、所有组合的完整可达性或实际开奖正确性。

主要错误（均为 `{success:false,error:{code,message},request_id}`）：400 `RULE_INVALID`（规则、选号或开奖结果无效）、`RULE_EXECUTION_LIMIT`、`RULE_POINTS_OVERFLOW`；400 `REQUEST_INVALID`/`IDEMPOTENCY_KEY_INVALID`；401 `AUTH_SESSION_REVOKED`；403 `PERMISSION_DENIED`/`CSRF_REJECTED`；409 `IDEMPOTENCY_CONFLICT`；415 `CONTENT_TYPE_INVALID`；503 `SERVICE_UNAVAILABLE`。管理端调用使用 [`rule-simulation-api.ts`](../admin-web/src/rule-simulation-api.ts) 与规则模拟器页面；调用方可通过响应 HTTP status 和 `error.code` 区分错误。

### 玩法和配置（完整版本生命周期：后续实现）

下面的规则版本草稿、校验工作流、送审、审核、发布、生效和回滚仍是后续 API 契约，不由上述已接入的独立模拟端点实现。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /admin/rule-versions | 创建草稿 |
| POST | /admin/rule-versions/{id}/validate | （后续）校验已保存草稿并模拟 |
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
