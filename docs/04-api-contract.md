# API 与异步任务契约

## 运营工作台快照

`GET /api/v1/admin/workbench`要求后台认证和UUID格式的`X-Brand-ID`，不接受查询参数，固定读取主库并在响应前提交审计。数据包含`brand_id,snapshot_at,timezone,day_from`以及13个区块：brand、periods、orders、today_bets、settlement、recharges、ledger、balances、reconciliation、sources、withdrawals、commissions、rewards。

前10区块分别使用brand、period、bet、report_betting、settlement、recharge、report_ledger、report_ledger、wallet、draw_source的显式view权限；有权限返回`ready`及对象，无权限返回`forbidden`及null。withdrawals使用独立withdrawal.view.brand/platform权限，返回当前reviewing_count/reviewing_points/processing_count/processing_points四个非负整数字符串；未授权不查询提现表。rewards使用独立reward.view.brand/platform权限，返回全部当前订单的granted_count/pending_count/revoked_count，不是今日入账或余额；报表权限不推导该卡片授权。仅commissions固定`not_implemented`及null。至少有一个可查看资源才允许请求，平台身份本身不授权。今日数据窗口为品牌时区午夜到快照时间的半开区间，今日投注用placed_at，账本用created_at；余额及待处理状态为当前汇总，提现订单积分不代表资格、实际出款或利润；对账显示最新历史任务及检查结果，不等于当前所有余额状态。所有计数及积分为规范十进制整数字符串，net_points允许负值。来源状态固定stub，不证明上游健康。查询或审计失败为503，不返回零值或部分数据；审计等待后会话过期为401且不释放数据。奖励独立报表及CSV另见[奖励报表合同](21-reward-reports.md)，其他精确字段以OpenAPI为准。

## 1. 基础约定

- Base URL：/api/v1。
- 用户端品牌由访问域名解析；管理端品牌操作必须显式提供品牌上下文。平台品牌列表、品牌创建和管理认证不以选中品牌作为输入。
- 所有接口返回 JSON；时间使用 UTC ISO-8601；积分使用十进制整数字符串。
- 认证：支持 `Authorization: Bearer <access_token>`；浏览器默认使用 HttpOnly、SameSite=Strict Cookie，不在 Web Storage 保存令牌。
- 请求追踪：X-Request-ID 必填或由网关生成。
- 所有有副作用的 POST/PUT/PATCH 必须支持 Idempotency-Key；当前键格式为 8–128 个 ASCII 字母、数字、`_ : . -`。
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
  "rule_version_id": "0199a000-0000-7000-8000-000000000003",
  "selection": {
    "regular": [1, 2, 3, 4, 5, 6],
    "special": [7]
  },
  "multiplier": "2",
  "policy_versions": { "brand": 1, "game": 1 },
  "actor_context": "从投注预览响应复制，不自行生成"
}
~~~

服务端不得信任客户端提供的注数、赔率、总积分或规则计算结果；这些字段只能由服务端重新计算。

S5-a1 已注册上述 POST bet-previews、POST/GET bet-orders、GET 单笔、POST cancel（含 `/b/{brandCode}` 等价路径）；S5-a2 接入用户彩种目录/当前期次，S5-b 接入公开开奖和历史期次。结算明细尚未接入；接口表不代表全部已实现。用户只能查询/取消自己在当前品牌的订单，不能从 body 或品牌头切换会员身份。规则引用为 UUID，不接受旧示例的历史序号 `rule_version`。

S5-a2 的 GET `/games` 为公开品牌目录，响应 `{items,limit,offset}`，只含 id/code/name/model/timezone/status，包含 active/paused 彩种。GET `/games/{id}` 返回 `{game,plays,period,server_time,brand_status,policy,policy_versions}`；plays 仅含当前 active 玩法的 id/game_id/code/name/rule_version_id/definition_hash/definition，不暴露草稿、贡献者、审核者或开奖源信息。GET `/games/{id}/plays` 返回 `{items}`，GET `/games/{id}/periods/current` 返回 `{period,server_time,brand_status}`。没有玩法返回空数组，没有期次返回 null；本期优先实际开放窗口，再选最近未来 pending，最后选最近进行中/完成期次。pending 不代表可投注。公开目录在主库只读 repeatable-read 事务读取一致快照，不持有行锁、不预留额度，提交时仍重新校验。

预览响应新增 `actor_context`：服务端 HMAC 绑定品牌、全局用户及品牌会员，不是登录凭据或钱包选择参数。Place 必须原样携带；在幂等锁前后、当前会话的事务认证内验证，缺失/伪造/切换到另一会员返回 403 `BET_CONFIRMATION_ACCOUNT_CHANGED`，不扣分也不保留幂等业务记录。同一会员正常重新登录仍可重放原操作；客户端必须同时拥有有效会话。它堵住另一标签页切换账户时，旧确认可能扣新账户的窗口；单独的客户端 `/me` 检查不能代替该服务端约束。Preview 请求可以不带此字段，响应以当前会话签发的值为准。

Preview 与 Place 均使用 `{period_id,play_id,rule_version_id,selection,multiplier,policy_versions}`。Preview 可以不带 policy_versions，并返回当前版本；Place 必须带确认过的两个版本。金额和倍数为十进制字符串，禁止 Number 计算。Preview 返回 normalized、expanded_bets、combination_count、unit_points、multiplier、bet_points，以及 period、definition_hash、policy、policy_versions；它不预留积分或期次额度，也不判断中奖。实际提交重新检查全部配置、窗口、余额和额度。

Place 成功返回 201 Order，包含不可变定义/策略快照、两种选号、展开注单、总积分、原扣款分配和 debit_entry_id。客户端使用每个确认意图独立的 Idempotency-Key，网络重试保留同键和完全相同请求体；同键异体 409，同内容不同键可以合法重复购买。订单、余额、账本、审计与 outbox 在同一事务提交。会话在事务内以及等待幂等锁后重新认证；自然过期或超过截止时间的余额锁等待不产生扣款。响应缓存为当时结果，客户端应另刷新钱包和订单状态。

Cancel 请求 `{version,reason}`（带幂等键）。用户取消按订单保存的 user_cancel_allowed，要求数据库时间早于 draw_at、无已锁定结果，期次为 betting/closed/waiting_draw；因此投注截止后、开奖前仍可取消。账户冻结/品牌暂停不禁止此类退款。用户仅取消未结算 placed；品牌运营可用独立 cancel 权限取消 placed/abnormal，不受用户开关或时间窗限制。退款引用原 debit，恢复每种来源的 available，不允许改金额。单注判定取消和整期任务退款均已接入；已结算订单回溯仍待后续。

管理接口新增 GET/PUT `/admin/bet-policy`、GET/PUT `/admin/games/{id}/bet-policy`（写 body `{version,config,reason}`）；GET `/admin/bet-orders`（可选 member_id、limit、offset）、GET 单笔、POST 单笔 cancel（`{version,reason}`）。查询需 bet_policy.view 或 bet.view 的显式品牌/平台权限；写需 bet_policy.write.brand 或 bet.cancel.brand，超级管理员仅查看。管理员读取和修改均记录审计。整数配置与继承结构见领域模型。

S5-a3 新增 GET `/admin/bet-orders/{id}/exception`（bet.view.brand / bet.view.platform），返回 `{exception:null}` 或 `{exception:{id,brand_id,order_id,order_version,marked_by,reason,created_at}}`。不属于当前品牌的注单返回 404；读取会审计。POST `/admin/bet-orders/{id}/abnormal` 需独立 `bet.mark_abnormal.brand`，带幂等键和 `{version,reason}`；仅 placed 可标记，返回新 Order（abnormal，version + 1）。证据、状态、审计和 outbox 同事务提交，不改余额或扣款快照；相同请求重放不追加证据。即使是同键缓存重试，也重新检查当前权限/会话；超级管理员不能标记。确定 409 后读取新版本再确认，网络/5xx 不确定时保留原 body/键重试。后台页面在具备读取权限时显示真实策略、50 条分页注单、原始快照与异常证据；写权限不隐含读取权限。

业务错误：400 BET_INPUT_INVALID；403 BRAND_PAUSED（新投注或预览遇到品牌暂停）/ BET_OPERATION_DENIED；404 BET_RESOURCE_NOT_FOUND；409 BET_VERSION_CONFLICT / BET_PERIOD_CLOSED / BET_LIMIT_EXCEEDED / BET_STATE_CONFLICT。积分不足、上限和损坏映射现有 POINTS_* 错误；幂等键格式/异体映射 IDEMPOTENCY_*；失效会话 401 AUTH_SESSION_REVOKED，临时数据库错误 503 且不缓存。

S5-a5：POST `/admin/bet-orders/{id}/judge-cancel` 需独立 `bet.judge_cancel.brand`，body `{version,cause,reason}` 与幂等键；cause 为 no_result/invalid_result，仅 placed/abnormal 单可执行，返回 200 新 Order（judged_cancelled、version + 1、refund_entry_id）。期次 settling/settled、已有结果却使用 no_result 或已取消的单被拒绝；不改变期次或其他单。退款、判定证据、状态、审计及 outbox 同事务。重复请求仍验证权限/会话，超管只读。

GET `/admin/bet-orders/{id}/judgment` 需显式 bet.view.brand / bet.view.platform，返回 `{judgment:null|Judgment}`；Judgment 包含 id、brand_id、game_id、period_id、order_id、order_version（结果版本）、cause、draw_result_id（无结果时空字符串）、judged_by、reason、created_at、refund_entry_id。普通单或整期任务取消的单可返回 null，不表示证据丢失。只有匹配原单/品牌、操作者、提交版本 + 1、原因类型/说明和退款引用的单注证据，才可核实未知的单注判定意图；不能用整期取消或其他人的结果替代本次回执。

### S5-b 已接入：公开开奖结果与已开始期次

无需登录的 GET `/draw-results`、GET `/draw-results/{id}`、GET `/games/{id}/periods` 均有 `/b/{brandCode}` 等价路径。品牌由已绑定 Host 或平台路径解析，不采信用户品牌头；仅 active/paused 品牌和 active/paused 彩种可读，跨品牌 UUID、禁用品牌和已替换结果 ID 返回 404。

列表参数：limit 1–100（默认 50）、offset 0–1000000（默认 0）；period_no 为可选精确期号，非空白且最多 80 UTF-8 字节、不做模糊搜索或数字转换。开奖列表另支持 game_id UUID；历史列表由路径确定彩种。上述参数重复返回 400。排序为计划 draw_at 降序、sequence 降序、期次 id 降序；has_more 通过多读一行确定，没有全表计数或无限返回。

`PublicDrawResult = {id,game:CatalogGame,period:Period,result:{regular:number[],special:number[],digits:number[]},drawn_at,origin:"manual"|"external"}`。不适用号码组为 [] 而不是 null；零、重复和位置顺序保持原始记录。game/period 为上节的公开字段，不含品牌后台人员、来源 ID/地址/认证、抓取原文、内部 claim、结果 hash 或纠正链。origin 只区分人工/外部，不公开具体采集源。

- 开奖列表返回 `{items:PublicDrawResult[],limit,offset,has_more,server_time,brand_status}`，仅发布当前 `period.draw_result_id`，状态为 drawn/settling/settled/bet_cancelled/judged_cancelled。被替换旧记录仍保留在管理员历史中，不作为公开最新结果。
- 单笔返回 `{item:PublicDrawResult,server_time,brand_status}`，同样要求仍被当前期次引用；结果详情重新读取，不将列表旧快照当作确认。
- 历史列表返回 `{game:CatalogGame,items:[{period:Period,draw:PublicDrawResult|null}],limit,offset,has_more,server_time,brand_status}`；仅显示 `bet_start_at <= server_time` 的已开始期次，包括进行中、等待开奖和取消，尚未开始的未来预留期次不列入。没有结果为 null，不制造号码。

取消期次仍可显示保留的号码及取消状态，但必须标明仅作记录、不能视为有效中奖或派奖结果。读取在主库只读 repeatable-read 事务内完成，不写审计、账本、订单、锁定结果或状态。查询校验/不存在复用 BET_INPUT_INVALID / BET_RESOURCE_NOT_FOUND；未配置服务返回 503 DRAWS_UNAVAILABLE，存储错误返回 503 SERVICE_UNAVAILABLE。该 API 不执行结算或派发积分。

### 积分、充值和提现（业务接口）

后台提现规则、资金状态机、用户申请/查询及运营审核接口和页面已接入。平台命令配置真实TurnoverChecker，品牌政策初始仍关闭，须明确启用；显式nil依赖的服务仍返回409 WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED，不占用积分。启用政策不等于已达流水或符合合规/余额条件，请求不能传入“合格”布尔值、N快照或资格证据。第一期仅内部积分处理，不接外部支付。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /wallet | 显示、可用、冻结、提现和来源积分 |
| GET | /wallet/ledger | 分页账本，支持来源/业务类型筛选 |
| GET | /recharges | 用户充值记录 |
| GET | /recharges/{id} | 本人当前品牌充值记录详情；不返回后台私有字段 |
| GET | /withdrawal-availability | 当前账号的入口状态与确认上下文；不是正式流水资格证明 |
| GET | /withdrawal-qualification | 本人当前流水条件的只读精确快照，不是提交授权 |
| POST | /withdrawals | 提交申请并重新检查实际资格；未配置依赖时安全拒绝 |
| GET | /withdrawals | 提现列表 |
| GET | /withdrawals/{withdrawalId} | 提现详情和状态变化 |
| GET | /withdrawals/{withdrawalId}/history | 不可变状态历史 |

充值用户查询已实现两条主库 GET，完整路径为 `/api/v1/recharges`、`/api/v1/recharges/{id}`，并有 `/api/v1/b/{brandCode}` 等价入口。只能读取当前已认证品牌会员的记录；没有用户创建、凭证上传、支付、确认或取消接口，不接受正文或客户端 member_id。暂停品牌、冻结会员仍可按现有认证规则查询；禁用品牌/会员、全局禁用、协议未接受以及失效会话不能绕过认证。

列表只接受 state、limit、offset。state 为 pending/confirmed/cancelled，省略表示全部；limit 默认20、范围1–100，offset 默认0、范围0–1000000。未知、重复、空参数、空问号、带符号或前导零的分页数字返回400。详情只接受 UUID 路径编号，不接受查询参数或正文；不存在或其他会员/品牌的编号统一404，不透露归属。

单条记录严格为 `{id,brand_id,member_id,points,state,version,created_at,confirmed_at,ledger_entry_id}`。points/version 是正 int64 十进制字符串，不经浮点数转换；时间为 UTC RFC3339。仅 confirmed 有非空 confirmed_at 和 ledger_entry_id，其余状态显式返回 null。列表严格为 `{brand_id,member_id,snapshot_at,state,items,limit,offset,total_count}`，全部状态的 state 为 null，空 items 为数组，总数为精确非负字符串。创建时间降序、UUID降序排列；页、总数和查询时间来自单条SQL的同一快照。翻页是新的当前状态查询，不承诺跨页冻结历史快照。

用户投影不包含 account_id、proof_reference、remark、created_by/confirmed_by、操作理由、内部审计ID或完整账本。confirmed 表示曾经人工确认并记入平台积分，不是外部付款凭证或当前余额。成功查询与合法404提交脱敏 `finance.recharge.user.view/detail` 审计，其他人的编号不写入资源ID。审计写入等待后再次按实际时钟复核会话，提交后才释放DTO；失效返回401，审计/存储故障返回503且不返回数据。查询不改变订单、余额或积分流水。复用0006表及现有会员/时间索引，无新增迁移。

上述提现用户接口都有 `/b/{brandCode}` 等价路径，只允许当前会话品牌会员读取自己的记录，不接受member_id查询或客户端品牌覆盖。POST正文恰好为 `{points,source_allocation}`；金额为正int64十进制字符串，来源分配为1至4项 `{source,state:"available",points}`，按recharge/winning/gift/commission排序、来源唯一且精确合计。没有隐含的提现来源扣除优先级。必须发送Idempotency-Key、同源Origin以及从GET入口取得的X-Withdrawal-Actor-Context；上下文绑定品牌、全局用户和品牌会员，换账号后不能重放旧确认。

GET入口返回 `{brand_id,member_id,policy_enabled,eligibility_configured,can_apply,reason_code,min_points,max_points,allowed_sources,real_payments:false,actor_context}`。can_apply只表示入口条件具备，实际申请仍需事务内的资格、合规、状态、额度和余额检查；未配置资格不是“剩余流水0”。列表只接受limit1..100（默认20）、offset0..1000000（默认0）及六种state之一；返回 `{brand_id,items,limit,offset,has_more}`。其他提现用户接口不接受查询参数。

GET `/withdrawal-qualification`及品牌路径不接受查询参数或正文，只从主库读取当前会话会员，钱包共享锁与期次NOWAIT锁保持到读取事务结束。十三字段为`brand_id,member_id,account_id,base_points,valid_points,valid_order_count,credit_numerator,credit_denominator,meets_turnover,cycle_from_at,cycle_from_version,cutoff_at,cutoff_version`。金额/序号为精确字符串，累计金额/分数可超过int64；分母必须为正，`meets_turnover`恰为分子≥基数×分母，周期序号≤当前截止序号。只表示流水条件，不表示余额足够、合规允许、已有申请不存在或出款授权；POST不接受此结果作为凭证，重新检查当时的钱包与流水。没有原始N快照、规则ID或内部摘要；读取后再次复核会话，失效时不返回数据。

期次锁冲突返回503 `WITHDRAWAL_TURNOVER_BUSY`，业务事务回滚且不缓存此临时失败，可用原键/正文重试；不能先将503包装成终态结果提交给幂等引擎。历史证据缺失/非法返回409 `WITHDRAWAL_TURNOVER_EVIDENCE_INVALID`且无占用。客户端对丢失回执、503或未知结果继续保留原意图，读取最新流水条件不能确认该写入；即使最新条件不达标，也必须能显式重放已提交的原请求。

管理端GET `/admin/withdrawals`、`/{id}`、`/{id}/history`需明确withdrawal.view.brand或view.platform，X-Brand-ID必填；列表另支持member_id。POST `/{id}/{approve|reject|cancel|fail|mark-paid}`正文恰好为 `{version,reason}`，reason非空且最多500 UTF-8字节，分别要求对应withdrawal动作的品牌权限，其中mark-paid对应mark_paid。超管不能执行这些写操作。读取在主库复核会话/权限并提交审计后才返回；写入及原键重放同样复核当前授权，钱包等待后再次检查会话有效期。

申请201、管理操作200返回原操作快照；查询返回当前状态，两者不能混用。reviewing/v1审核通过后为processing/v2，mark-paid后为paid/v3；automatic审核可直接返回processing/v2，不自动出款。驳回、取消或失败全额反向原reserve流水，来源不变；paid只消耗提现预留并移动成功周期截止点。未知写响应、畸形回执或网络失败只允许原正文/原键显式重放，刷新查询不能确认或丢弃未知意图。

OrderView字段为 `id,brand_id,member_id,account_id,points,state,version,source_allocation,reserve_entry_id,release_entry_id,paid_entry_id,cycle_from_at,cycle_from_version,reserve_version,created_at,updated_at,reviewed_at,completed_at,decision_reason,audit_log_id`。未产生的release/paid引用及时间为null；账本序号为精确十进制字符串。HistoryView为 `{brand_id,order_id,items}`，每项含id/version/from_state/to_state/reason/actor_type/created_at/audit_log_id，初始from_state为""。两端均不返回原始资格证据、政策快照、后台账号ID或原幂等键；用户只显示驳回/失败/取消理由，其他内部原因为空。

领域错误另包括400 WITHDRAWAL_INPUT_INVALID、403 WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED/PERMISSION_DENIED、404 WITHDRAWAL_NOT_FOUND、409 WITHDRAWAL_VERSION_CONFLICT/WITHDRAWAL_STATE_CONFLICT/WITHDRAWAL_ACTIVE_ORDER/WITHDRAWAL_INELIGIBLE；会话、Origin、幂等冲突、合规及余额错误复用既有约定。资格缺失或拒绝不会产生资金占用。

### S6-a 已接入：提现规则与不可变版本

管理 GET/PUT `/admin/withdrawal-policy` 与 `/admin/games/{id}/withdrawal-policy`，以及两者追加 `/history` 的 GET。读取需 withdrawal_policy.view.brand 或 view.platform，修改仅 withdrawal_policy.write.brand，超管只读；game.view 独立，仅影响读取彩种目录，不由规则读写权推导。

`BrandWithdrawalPolicy={brand_id,version,config:{enabled,min_points,max_points,allowed_sources,review_mode,turnover_multiple},updated_at,audit_log_id?}`。初始 enabled=false、min_points="1"、max_points=null、allowed_sources=["recharge","winning","gift"]、review_mode="manual"、N="1"。上下限正 int64 字符串，上限不低于下限；来源非空、合法、不重复，审核配置 manual/automatic，但本阶段不执行审核。

`GameWithdrawalPolicy={brand_id,game_id,version,config:{turnover_multiple:string|null},effective:{turnover_multiple:string,source:"brand"|"game",brand_version,game_version},updated_at,audit_log_id?}`。null 继承，显式正值覆盖品牌。新写入 N 必须大于 0 且不超过 1000000，最多六位小数，禁止符号/指数/多余前导零和小数末尾零。品牌及彩种写请求拒绝 "0"；已保存的旧零值及不可变历史仍原样可读，不自动转成继承或 1，需授权管理员显式修改当前配置。OpenAPI分开描述新写配置与兼容历史读取的配置。

PUT 完整替换 `{version,config,reason}`，nullable 字段也必须显式提供；未知/重复字段、缺省或 null 标量拒绝。原因非空、最多 500 UTF-8 字节；响应 200、配置版本加一、审计引用。版本是对应配置版本，不是彩种业务版本。需要原 Idempotency-Key、X-Brand-ID 及同源 Cookie/Origin；同键同正文重放原回执，同键异体 409。权限/会话在等待幂等锁前后复验，撤权或变为超管不能重放旧回执；临时错误不缓存。

历史 limit 1–100（默认 50）、offset 0–1000000（默认 0），返回 `{items:PolicyRevision[],limit,offset}`，范围内按 version 降序。`PolicyRevision={id,brand_id,game_id,version,config,changed_by,reason,created_at}`；品牌级 game_id=""，初始系统 changed_by=""，后续为后台账号。配置、不可变历史及审计原子提交，不变更钱包/账本/订单；GET 记录后台读取审计。

错误：400 WITHDRAWAL_POLICY_INPUT_INVALID / REQUEST_INVALID；403 PERMISSION_DENIED；404 WITHDRAWAL_POLICY_NOT_FOUND；409 WITHDRAWAL_POLICY_VERSION_CONFLICT / IDEMPOTENCY_CONFLICT；401 AUTH_SESSION_REVOKED；503 SERVICE_UNAVAILABLE。基数为申请占用前全部可用充值＋赠送余额，服务端锁钱包并保存快照；公开订单不暴露内部证据。所有来源有效投注按原N快照精确折算，N必须大于0，修改只影响新投注。平台已配置真实资格器；客户端不能提交达标额度代替判断，品牌默认关闭且没有外部出款。

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

Reconciliation 返回 consistent、account_id、member_id、version、entry_count、expected/actual（完整矩阵）、issues。关键查询走主库；对账在共享账户锁内扫描版本链，写入在账户排他锁内校验上一条 after 与当前余额。没有账本却有余额、缺失桶或不一致时停止新增记账，返回 `POINTS_RECONCILIATION_REQUIRED`，不能用人工调整绕过损坏。S3-b 提供下述明确修复流程与品牌限额；现已接入品牌钱包批量异步检查，五个API、固定范围、不可变观察和人工重试见[批量对账交接](13-wallet-reconciliation.md)。全业务订单与账本对账及大范围负载仍待独立验收。

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

### S4-c1 已接入：日历、期次生成与状态调度

以下管理路由以 `/api/v1` 为前缀，均要求有效管理会话和 `X-Brand-ID`。读取权限精确为 `schedule.view.brand` / `period.view.brand`，或平台 scope 的 `schedule.view.platform` / `period.view.platform`。平台 scope 同样要求角色中存在精确权限；S4-c1 bootstrap 仅将这两项平台读取权限授予超级管理员默认角色，`is_super_admin` 标记本身不授予读取权限。写入分别要求 `schedule.write.brand` 与 `period.generate.brand`；超级管理员写入拒绝。写请求要求非空 `reason`、`Idempotency-Key` 和标准管理写请求校验。

| 方法 | 路径 | 授权 / 用途 |
|---|---|---|
| GET | `/admin/games/{id}/schedule` | `schedule.view.brand` 或 `.platform`；读取当前修订 |
| PUT | `/admin/games/{id}/schedule` | `schedule.write.brand`；乐观锁更新并创建新修订 |
| POST | `/admin/games/{id}/periods/generate` | `period.generate.brand`；按当前日历保留期次 |
| GET | `/admin/games/{id}/periods` | `period.view.brand` 或 `.platform`；分页查询 |

日历 PUT 请求为 `{version,spec,reason}`；`version` 是彩种乐观锁版本，`spec.timezone` 必须等于彩种 timezone。Spec 支持 `mode` 为 `daily` 或 `interval`，`daily_draw_times`、`interval_seconds`、最多 128 个不重叠 `busy_windows`、`bet_open_before_seconds`、`bet_close_before_seconds`、`weekdays`（0 周日至 6 周六）、`pause_dates`、`holiday_dates` 和 `holiday_policy`（`skip` / `normal`）。Daily 与窗口时间为 `HH:MM:SS`；busy window 只能是同日且 `start < end`，不支持跨午夜，`24:00` 不接受。窗口为半开 `[start,end)`，所以结束设为 `23:59:59` 时该秒的 slot 不在窗口内。interval 为 30–86400 秒。IANA 时区由服务端 tzdata 加载；DST gap 跳过，fold 取较早 UTC 实例。Interval 每个本地日的基线锚定午夜；busy window 在 `[start,end)` 替换基线并锚定窗口起点，end 回到基线。节假日只按显式日期配置，不做国家或地区自动查询。

模式字段互斥：daily 要求非空且不重复的 daily_draw_times、interval_seconds=0、busy_windows 为空；interval 要求 daily_draw_times 为空。投注开放提前秒数为 1–86400，截止提前秒数为 0 至开放提前秒数减 1；`bet_start_at=draw_at-open_before`，`bet_end_at=draw_at-close_before`。weekdays 必须至少选一天；日期列表使用有效且不重复的 YYYY-MM-DD。保存创建新修订，不改写任何已经生成的期次。

GET/PUT schedule 成功的 data 均为 `{id,brand_id,game_id,revision,spec,game_version,created_at}`；game_version 是读取/保存时的当前彩种版本，开期也会改变它，调用方遇到版本冲突应重新读取。尚未配置日历与不存在彩种均返回 404。Period 返回 `{id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status,version,state_reason}`，有日历快照时另含 schedule_id。管理界面选取彩种另需独立的 game.view.brand / game.view.platform 权限，不能由排期写权限推导目录读权限。

生成请求为 `{from,to,reason}`，from/to 是带时区的 RFC3339 时间；期次时刻与 `period_no` 均按 UTC instant 保存/生成。时间窗最长 7 天、展开最多 10,000 个 slot，且不得指定已过去的 to 或超过当前数据库时间 7 天的边界。响应 `data` 含 `created`、`existing` 汇总计数和至多 100 条 `periods`；计数可高于返回数组长度。已有相同期次窗口计入 existing 并保留原 schedule 快照；同一期次号时间窗口不同返回 409 `PERIOD_STATE_CONFLICT`，不覆盖旧期次。生成只创建仍有投注窗口的 pending 期次。列表响应为 `{periods,limit,offset}`，limit 1–100（默认 50）、offset 0–1,000,000，按 bet_start_at 倒序分页。

彩种版本不匹配返回 409 `SCHEDULE_VERSION_CONFLICT`；无权限返回 403 `PERMISSION_DENIED`；缺失彩种/资源返回 404 `RESOURCE_NOT_FOUND`。其余错误码保持通用管理 API 约定，日历或范围无效为 400 `SCHEDULE_INVALID`。

`period.sequence` 是创建序号；只有 worker 实际转入 `betting` 时，`games.started_sequence` 才递增，用于规则下期激活。worker 以主数据库时钟驱动：pending 到点、彩种 active 且该彩种上期结算或全额退款已完成时进入 betting；否则保持 pending。若投注窗口已过则转 `judged_cancelled`，不延长窗口、不增加实际开期序号、不激活下期规则；betting 到 bet_end 转 closed，closed 到 draw_at 转 waiting_draw。彩种暂停时不开放投注，窗口过期后取消 pending。状态更新受数据库转移约束并写审计。pending 尚无注单；已有投注的期次必须用下述整期取消任务，不允许直接 SQL 改为取消而遗漏退款。

同品牌、同彩种的未完成旧期阻止下一期开启；仅关闭投注、已有开奖结果、计算完成、等待审核或已批准但未完成派奖均不放行。取消期次须其退款任务 completed；自动跳过且无任何注单的未开放期次无需退款。顺序依据计划开奖时间，不以创建序号误判未来日历为上一期；已开始的未完成期次始终阻止重叠开放。内部OpenPeriod返回领域ErrState，定时Tick保持pending；用户预览与新投注最终检查使用现有409 BET_PERIOD_CLOSED，不扣积分。已成功订单的原键重放仍读取原回执，不算新投注。不同品牌或彩种独立处理。

S5-a4 实际整期取消接口：GET `/admin/periods/{id}` 返回当前 Period（包括真实状态、期次 version）；GET `/admin/periods/{id}/cancellation` 返回 `{cancellation:null|Cancellation}`。均需显式 period.view.brand / period.view.platform，并记录读取审计。POST `/admin/periods/{id}/cancel` 需 period.cancel.brand，body `{version,mode,cause,reason}`，version 是期次版本；mode 为 bet_cancelled / judged_cancelled，cause 为 operator_cancel / no_result / invalid_result。合法组合和状态见领域模型；单人操作，原因非空且不超过 500 UTF-8 字节。有待退款目标返回 202 processing，无目标返回 200 completed。

Cancellation 包含 id/brand_id/game_id/period_id/period_version、mode/cause、state/version、reason/created_by/created_at/completed_at/last_error_code，以及 total_count/pending_count/refunded_count/already_refunded_count/failed_count。摘要的 version 是独立任务版本。期次关闭和目标集合同事务落库，清除开奖采集租约，保留已有开奖结果与尝试历史；之后不得投注或提交新结果。每个目标退款、注单状态、账本、审计及事件单独原子提交；全部目标完成后任务才 completed。中断可恢复，业务或交易失败暂停为 failed，不自动重试失败任务。POST `/admin/periods/{id}/cancellation/retry` 需 period.cancel_retry.brand，body `{version,reason}` 使用任务版本，返回 202；仅失败任务可人工恢复，不撤销之前成功退款。读写权独立，超管只读。

上述写入需幂等键；缓存重放仍重验会话和权限。202 重放是起始快照，必须另 GET 摘要获取最新进度。400 PERIOD_CANCEL_INPUT_INVALID、404 PERIOD_CANCEL_NOT_FOUND、409 PERIOD_CANCEL_VERSION_CONFLICT / PERIOD_CANCEL_STATE_CONFLICT；权限/会话/幂等错误沿用公共错误。worker 错误码只公开结构化类别，不公开底层 SQL、连接字符串或栈。

### S4-c2 已接入：来源与人工结果

下列路由以 `/api/v1` 为前缀，要求管理会话、X-Brand-ID；写入沿用 Idempotency-Key、Cookie 同源 Origin、16 KiB 闭合 JSON、非空 reason 和事务内权限重查。超级管理员只读，精确读取权限不由写权限推导。

| 方法 | 路径 | 精确权限 / 正文 |
|---|---|---|
| GET | `/admin/games/{id}/draw-sources` | draw_source.view.brand 或 .platform |
| PUT | `/admin/games/{id}/draw-sources` | draw_source.write.brand；`{version,sources,reason}` |
| GET | `/admin/periods/{id}/draw` | draw.view.brand 或 .platform；limit/offset |
| POST | `/admin/periods/{id}/manual-draw` | draw.manual_create.brand；`{version,period_no,result,drawn_at,reason}` |

Sources GET/PUT data 为 `{id,brand_id,game_id,revision,game_version,created_at,sources}`；尚无配置返回 404。version 是当前彩种版本，不是 revision。sources 可为空，最多 16 项 api/dom；来源 ID 为 UUID，类型和归属创建后不可变。优先级 1–10000 且唯一，禁用项也校验。name 最多 120 UTF-8 字节；endpoint 最多 2048 字节，限 HTTPS、公共 DNS 名、默认/443 端口，禁 IP/内网保留域/userinfo/query/fragment。DOM selector 非空且最多 500 字节，API selector 必须为空。credential_ref 是非敏感引用，最长 120 ASCII 字符，匹配 `^[a-zA-Z][a-zA-Z0-9_.:/-]{0,119}$`，不是实际凭据。校验不访问网络或 DNS；未来真实适配器须另验证 DNS、重定向及目的 IP。

人工 version 是期次业务版本；result 为匹配彩种的 regular/special/digits 数组，drawn_at 接受 RFC3339。只允许 waiting_draw，或已有外部结果但尚未结算的 drawn；禁止覆盖人工结果、settling/settled。校验期次号、数量、范围、重复约束和与上一期相同的结果；上一期按 draw_at 而非创建序号查找。时间不得早于本期计划 draw_at 或晚于当前主库时间。成功 201 并转 drawn/version+1，结果、当前指针、审计和幂等响应同事务；覆盖外部结果保留旧行及 corrected_from_id。不结算、不派奖、不改积分。

DrawResult 含 id/brand_id/game_id/period_id/source_id/kind/result/result_hash/drawn_at/created_at，人工另含 created_by，覆盖时另含 corrected_from_id。时间返回 UTC RFC3339Nano（分数秒位数可变）。GET draw 返回 `{current,history,attempts,limit,offset}`；current 可为 null，不受分页影响；history/attempts 分别按同一 offset/limit 分页，默认 50、最多 100，读取使用一致性快照。AttemptBatch 含 source_set_id、observed_period_version、status、attempts（每条 source_id/status/code）和创建时间。

worker 当前调用 API/DOM 无网络 stub，只产生 no_data 尝试证据，不代表第三方接口已接入。人工优先，旧在途结果记 discarded。错误：400 DRAW_INVALID；409 DRAW_ABNORMAL/DRAW_VERSION_CONFLICT/DRAW_STATE_CONFLICT；404 DRAW_NOT_FOUND；403 PERMISSION_DENIED；摘要冲突为 409 IDEMPOTENCY_CONFLICT。UI 还分别需要 game.view、period.view 目录权限。

### 期次和开奖（后续实现）

来源与人工结果按上节接入；S5-c3 更正及回溯合同见 4.3，重开仍未实现。S5-c1 核算预览不入账；S5-c2 正式 settle、审批与 retry 见 4.2，旧 `/settlements/{id}/retry` 设计路径未注册。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /admin/periods/{id}/close | 截止投注 |
| POST | /admin/periods/{id}/cancel | 期次投注取消/判定取消并退款 |
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

### S4-b 已接入：彩种、玩法与规则版本工作流

以下路径以 `/api/v1` 为前缀。全部要求有效管理会话和 `X-Brand-ID` UUID，平台查看也必须选品牌。权限精确匹配，写入/验证/审核权限不隐含读取权限；仅有 `.platform` 查看权限不能写。全部彩种和规则工作流写入拒绝超级管理员，即使误配品牌写权限；独立模拟的 `rule.simulate.platform` 不改变此边界。

| 方法 | 路径 | 精确授权 / 请求 |
|---|---|---|
| GET | /admin/games | `game.view.brand` 或 `game.view.platform`；分页 |
| POST | /admin/games | `game.write.brand`；code、name、model、timezone、reason |
| GET | /admin/games/{id}/plays | `game.view.brand` 或 `game.view.platform`；分页 |
| POST | /admin/games/{id}/plays | `game.write.brand`；code、name、reason |
| GET | /admin/plays/{id}/rule-versions | `rule.view.brand` 或 `rule.view.platform`；分页，version_no 倒序 |
| GET | /admin/rule-versions/{id} | `rule.view.brand` 或 `rule.view.platform`；完整版本及验证报告 |
| POST | /admin/rule-versions | `rule.write.brand`；play_id、definition、effect_mode、reason |
| PUT | /admin/rule-versions/{id} | `rule.write.brand`；version、完整 definition、effect_mode、reason；仅 draft |
| POST | /admin/rule-versions/{id}/validate | `rule.validate.brand`；version、cases、reason；仅 draft |
| POST | /admin/rule-versions/{id}/submit-review | `rule.submit.brand`；version、reason |
| POST | /admin/rule-versions/{id}/approve | `rule.review.brand`；version、reason、warnings_acknowledged |
| POST | /admin/rule-versions/{id}/reject | `rule.review.brand`；version、reason；不要求确认警告 |
| POST | /admin/rule-versions/{id}/clone | `rule.write.brand`；effect_mode、reason；旧定义克隆为新草稿 |

写请求要求 `application/json`、`Idempotency-Key`，正文上限 16 KiB；携带 Cookie 的请求必须带与 Host 同源的 Origin（CSRF 校验）。未知字段、畸形/尾随 JSON 拒绝；Definition 各层闭合且拒绝重复字段。模型不适用的空字段可省略，不要求发送所有空选号/开奖组。完整计算 DSL 见 [03-rule-engine.md §8](03-rule-engine.md#8-s4-a-可执行-dsl-v1-实际实现)，工作流与激活边界见该文档 §9。

所有写入都有非空 reason，最多 500 UTF-8 字节；彩种/玩法 code 匹配 `^[a-z][a-z0-9_]{0,47}$`，name 非空且最多 120 UTF-8 字节。timezone 必须是服务端可加载的时区；model 为合法三类 Model。创建或修改规则的 Definition 必须合法且完整 Model 与所属彩种一致；effect_mode 只能为 `immediate` 或 `next_period`。列表支持 limit（1–100，默认 50）及 offset（0–1000000），分别返回 `{games,limit,offset}`、`{plays,limit,offset}`、`{versions,limit,offset}`。

成功使用标准 `{success,data,request_id}`：创建彩种、玩法、草稿及 clone 返回 201，其余成功返回 200。Game 含 id、brand_id、code、name、model、timezone、status、version、started_sequence；Play 含 id、brand_id、game_id、code、name、status、active_version_id、version。规则 Version 含 id、brand_id、game_id、play_id、version_no、version、definition、definition_hash、status、effect_mode、created_by、reviewed_by、review_comment、created_at、updated_at，以及可选 effective_at、effective_period_id、effective_sequence、source_version_id、validation、audit_log_id。

`version_no` 是每玩法不可变的历史序号；请求中的 `version` 是正整数记录乐观锁，不是 version_no。更新、验证、送审和审核成功后返回新的 version，下一动作须使用它。仅 draft 可更新，更新清除旧 validation；非 draft 的 Definition、生效模式、验证证据不可改写。历史记录不可删除，rejected 不支持原地恢复草稿。

#### 已保存定义的用例验证

validate 不从正文接受新 Definition，只执行目标草稿当前已保存的定义。请求形状如下（示例 selection/draw 对应上节特别号玩法）：

~~~json
{
  "version": 1,
  "reason": "核对特别号命中与倍率",
  "cases": [{
    "name": "特别号命中",
    "selection": { "special": [7, 19] },
    "draw": { "regular": [1, 2, 3, 4, 5, 6], "special": [7] },
    "multiplier": "2",
    "expected_bet_points": "4",
    "expected_prize_points": "70",
    "expected_won": true
  }]
}
~~~

cases 必须有 1–32 项；name 非空、去空白后唯一、最多 120 UTF-8 字节。selection/draw 必须符合模型及玩法模式，multiplier 为规范正整数字符串。三个 expected 字段都必填；预期积分是规范非负整数字符串，中奖状态为 JSON boolean。整个用例集共享最多 200000 个解释节点的预算。

验证返回 Version，`data.validation` 保存完整 `ValidationReport`：passed、definition_hash、cases、warnings、findings。每例包含 name、完整原始 input（selection、draw、multiplier 和三个预期值）、expected_bet_points、actual_bet_points、expected_prize_points、actual_prize_points、expected_won、actual_won、matched、simulation；每项 finding 含 code、message、blocking。simulation 含规范选号、计算输出、完整奖级及条件 Trace；raw/capped 字段沿用上节精确 RatString 格式，可为 `n/d`，不是整数金额。案例预期不符仍返回 200 并保存 `passed=false`；非法请求、计算溢出或超预算才是错误响应。

definition_hash 是确定性类型化 Definition JSON 的 SHA-256，不是请求原始字节 hash。静态检查仅对可证明的标量约束矛盾/模型不变量不可达产生阻断 `TIER_UNREACHABLE`；案例不符为阻断 `CASE_MISMATCH`；重复或同标量可证明相交的条件为非阻断 `TIER_OVERLAP`。报告保留样例不能证明完整覆盖及商业赔率合理性的风险提示，不声称任意复合条件全覆盖。完整 input/output 与报告持久化供审核人查看，送审后证据不可改写。

#### 送审、审核、生效与克隆

submit-review 只允许 draft，要求 validation.passed=true 且报告 definition_hash 与当前定义一致，成功变为 pending_review。创建者及所有成功更新过草稿的编辑者（contributors，含仅改生效模式者）均不能 approve 或 reject；只执行验证不自动加入 contributors。通过与驳回都要求当前有效验证报告。存在 warnings 时通过必须显式 `warnings_acknowledged=true`；通用风险提示始终存在，不能绕过确认。reject 记录原因并进入 rejected，不要求 warnings_acknowledged。

后台新建草稿默认选择 `immediate`；后端没有默认生效模式，创建、PUT 更新及 clone 必须显式发送 `effect_mode`（仅 `immediate` / `next_period`），遗漏或空值拒绝。通过时 `immediate` 在同一事务直接成为 active 并替换玩法有效版本，没有独立 publish 步骤；`next_period` 成为 approved，绑定彩种下一实际开期序号。每玩法最多一个 approved 队列项；已有待生效项时后续 approve（包括 immediate）返回 409 `RULE_STATE_CONFLICT`，不覆盖它。

下期激活仅由内部开期事务在彩种行锁下执行，与审核串行化：使用 PostgreSQL `clock_timestamp()` 检查实际窗口 `bet_start_at <= now < bet_end_at`、`bet_end_at <= draw_at`，开期时激活满足序号的 approved 版本并保存不可变 period_rule_versions。S4-c1 日历、期次生成和状态 worker，以及 S4-c2 来源配置与人工结果已接入；没有公开强制 activate-now 路由。投注、结算与已结算结果纠正已按 S5 合同接入，见对应章节；其余未来 API 表不代表已注册。

clone 仅接受 active/expired/rolled_back 来源，返回新的 draft、version_no、source_version_id，不直接激活旧版本。新草稿定义继承源定义且不可修改，draft 阶段可修改生效模式，但必须重新验证、送审、由非贡献者审核。普通版本替换时旧 active 标记 expired；来源克隆版本生效时被替换的旧 active 标记 rolled_back。不删除历史、不改变积分、不创建投注订单或派奖。

#### 审计、幂等与错误

成功查询追加 `rule.workflow.view` 审计。成功写入在同一事务追加 `game.create`、`play.create`、`rule.draft.create/update/validate`、`rule.review.submit/approve/reject` 或 `rule.rollback.draft`，包含操作者、原因、请求 ID 与变更摘要；规则版本写响应含 audit_log_id，彩种/玩法写响应以 request_id 追踪审计。内部开期激活写 system actor 的 `rule.period.activate`。版本、报告、审核、审计与幂等结果共同提交或回滚；同键同请求重放不重复执行/审计，同键不同正文返回 409 `IDEMPOTENCY_CONFLICT`。最终业务失败可缓存，修改正文（包括刷新后的 version）必须用新键；瞬时/存储失败返回 503，不封存为最终结果。写事务重查当前权限和管理会话。

实际业务错误：400 `RULE_INVALID`、`RULE_EXECUTION_LIMIT`、`RULE_POINTS_OVERFLOW`；404 `RESOURCE_NOT_FOUND`；409 `RULE_VERSION_CONFLICT`（乐观版本或目录编号冲突）、`RULE_STATE_CONFLICT`（状态不符、队列已有项、修改源克隆定义等）、`RULE_VALIDATION_REQUIRED`（报告无效或批准未确认警告）；403 `PERMISSION_DENIED`（含贡献者审核及超级管理员写入）。通用解析、CSRF、认证、幂等与 503 错误沿用 S4-a 约定。

S4-d 管理端六个快捷模板与通用可视化编辑器并存；通用模式覆盖当前 schema-v1 所有定义/条件字段与最多 32 组独立验证用例，调用本节相同 API，没有跳过验证或审核的新入口。不能无损还原的定义自动使用通用模式，普通草稿可编辑，来源回滚草稿仍不可改写。前端保存原定义语义、阻断未解析输入，并检查整个请求 16 KiB 上限；最终模型一致性、可达性、版本/权限/报告 hash 仍由后端判断。

### 玩法版本的旧设计路径（未实现）

以下保留为未来 API 设计，不是已注册路由，也不能作为绕过审核的调用方式。当前立即/下期生效使用上述 approve 与内部 OpenPeriod，回滚草稿使用 clone。

| 方法 | 路径 | 未来契约边界 |
|---|---|---|
| POST | /admin/rule-versions/{id}/publish | 未实现；不是当前审批后的必调步骤 |
| POST | /admin/rule-versions/{id}/rollback | 未实现；当前使用 clone 创建需重新验证审核的草稿 |

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

## 4.1 S5-c1 已实现的真实注单核算预览

| 方法 | 管理路径 | 当前行为 |
|---|---|---|
| GET | /bet-orders/{id}/settlement-context | 当前订单/期次版本、状态、旧规则 hash、当前开奖结果及 hash，可预览标志 |
| POST | /bet-orders/{id}/settlement-previews | `{version,period_version,draw_result_id,reason}`，创建不可变证据，成功 201；必须显式闭合字段、可信 Origin 和幂等键 |
| GET | /bet-orders/{id}/settlement-previews?limit=20&offset=0 | 本品牌注单历史，items/limit/offset/has_more |
| GET | /settlement-previews/{id} | 历史概要和实时派生 current |
| GET | /settlement-previews/{id}/lines?limit=20&offset=0 | 逐注选号/命中奖级/排他选择/精确金额/条件 trace 分页，附 total |

所有路径均位于 `/api/v1/admin` 并要求 X-Brand-ID。读取独立 `settlement.view.brand/platform` 权限，品牌执行 `settlement.preview.brand`；超管禁止创建，即使误授写权限。POST 在幂等锁前后验证会话和权限，退出、撤权不能复用缓存。用户客户端和前端不能指定规则、金额、开奖结果正文或操作者；只引用已经锁定的结果 ID。

当前仅 drawn 期次可创建预览。当前注单/期次版本或结果 ID 不符返回 409；同键异体 409。明确金融状态为 `applied:false`，不修改注单或期次、不写账本、不通知中奖。outcome 为 won/lost 时附 `calculation:{won,combination_count,multiplier,bet_points,prize_points,raw_prize_points,capped_prize_points}`；积分 int64 字符串，中间金额为非负规范有理数字符串。abnormal/excluded 的 calculation=null，保留 error_code，不能拿部分计算结果派奖。

核心重新验证原定义 hash、PrepareBet 的规范选号与全部复式/金额、充值→中奖→赠送分配以及原 debit 的账户/会员/引用/12 桶 before/delta/after。已有人工异常和已取消订单直接 excluded，不重新运行普通中奖计算。存储错误/超时整事务回滚返回 503；只在运营再次明确发请求时重试，无后台自动预览任务。

GET 保留历史证据，current 是该计算的注单/期次版本与当前指针仍相同，不是已结算或可支付保证。幂等成功缓存中的 current 可能是历史值，客户端拿到直接匹配回执后必须重新 GET 当前记录；GET 失败显示“已保存但读取失败”，不能把已收到有效回执降为未知。丢失回执才保留精确 body/context/key；读到同样历史不能解除未知意图。

正式整期结算、派奖、结算任务失败重试与已结算结果回溯已通过专用合同接入；这些不是本预览接口的隐式后续动作。

## 4.2 S6-b/S6-c 已实现的站内通知

用户路径同时支持 `/api/v1` 和 `/api/v1/b/{brandCode}`；账户从实际品牌会话解析，禁止客户端指定目标会员。读取和写入均走主库。

| 方法 | 路径 | 当前契约 |
|---|---|---|
| GET | /notifications?limit=20&offset=0 | 当前会员消息页与一致快照未读总数 |
| POST | /notifications/read | `{ids:[UUID...]}`，1–100 个不重复、已观察到的消息编号，整批归属校验 |
| GET | /admin/notification-deliveries | 品牌内投递状态分页；显式 `notification.view.brand/platform` 与查询审计 |
| POST | /admin/notification-deliveries/{event_id}/retry | `{attempt_count:整数,reason:非空文本}`；仅 failed 状态，显式 `notification.retry.brand`，超管禁止 |

列表数据 `{brand_id,member_id,items,unread_count,limit,offset}`；`unread_count` 为规范非负 int64 字符串。item 为 `{id,brand_id,member_id,event_type,template_key,template_version,content,payload:{resource_id,points},created_at,read_at}`，已读时间初始 null。template_version为正安全整数，content为不可变双语源文案；仅旧非提现/佣金/奖励v1消息可为null。原八种事件、六种withdrawal.order状态事件、两种commission事件及reward.order.granted/revocation_pending/revoked，共十九种；入品牌积分为null，佣金修正为规范带符号非零int64字符串，其余为规范正int64字符串。奖励三种事件均用原订单UUID和原奖励正额，待处理说明本次没有积分变动，不表示当前状态。用户不能编辑快照或业务事实，详见 [模板与旧消息合同](12-notification-templates.md)、[佣金通知](18-commission-adjustments.md)及[奖励合同](20-manual-reward-orders.md)。

`won` 仅在实际正额中奖入账事务中生成，points 是实派奖金额而非下注金额；待批准的核算、零额及未中奖不生成此消息。`prize_reversed` 仅在实际全额冲回旧 prize 的事务生成，points 为原正额奖金；来源不足、回滚和零额不会生成。resource_id 都是注单 ID；私有 outbox 另保存 calculation/ledger/job/correction 引用，消费者校验同品牌同会员不可变目标与账本，而非注单当前 won 状态。延迟消费在更正后仍能验证旧入账，原通知不会删除/覆盖；新代次再中奖是新的独立消息。双语文案明确历史入账/冲正事实不代表当前钱包余额或最终中奖状态。不制造迁移前的历史奖金通知。

已读回执 `{brand_id,member_id,ids,changed,unread_count}`，ids 顺序等于请求。重复读不改原读时间；任一消息不属于账户或不存在则整批 404、不做部分写入。当前 UI 明确为“本页标为已读”，只处理本次已加载编号；新到消息不会被无界 UPDATE 误吞。其他页可分页继续处理。

两个 POST 都需要幂等键、可信 Origin/JSON；等待幂等锁前后重查会话/权限，撤权或退出后不能重放旧成功。客户端冻结原 ids/context/key，丢失或畸形成功回执视为未知结果；只读刷新不解除未知意图。确定拒绝可重载后发起新操作。

投递列表只公开运行状态、次数、安全错误码及时间，不公开原始事件/内部业务材料。正常数据库错误自动退避 2/4/8/16 秒后第 5 次失败终止；无效业务事件立即 failed，不确认消费。人工重试保留累计次数和旧错误，成功后清除最后错误。人工重试及查询可由 request ID/审计追溯。

管理投递列表数据 `{items:[{event_id,brand_id,status,attempt_count,last_error,next_attempt_at,sent_at}]}`；status 为 pending/sent/failed，sent_at 仅 sent 非空。重试回执仍为首次 pending、次数不变，保留 last_error；累计次数和只读最新状态可以已推进，不能把缓存回执当作已投递。S6-c 后台铃铛和“通知投递”菜单接入查询、分页及失败项原因/二次确认/同键恢复；未知意图按账号＋品牌保存在页内内存，刷新列表/切换页面不解除，退出或会话失效清除，不写浏览器持久存储。

S7-l 新增后台GET `/notification-templates`、GET `/notification-templates/{key}/history`及PUT `/notification-templates/{key}`，独立查看/品牌修改权限、乐观锁、不可变历史、审计和原键重放。消费者落库时复制当前模板，不覆盖已有消息；完整字段、错误与生效时点见 [通知模板合同](12-notification-templates.md)。0040接入新提现状态事件，依据不可变状态历史而非当前状态验证；原键重放不重复入队，通知写入失败使同事务资金处理回滚。尚未接入开奖受众及外部渠道，不把内部paid事件当作银行或虚拟币转账证据。

### 提现申请报表与完整导出

GET `/api/v1/admin/reports/withdrawal`和`/export`使用后台会话、X-Brand-ID与主库。查看需明确report_withdrawal.view.brand/platform；导出同时要求对应view及独立export授权。0041只注册权限，不自动分配角色。查询在READ COMMITTED事务中复核持久授权、读取一条SQL一致快照、再次校验会话并提交审计后返回，超管身份不隐含授权。

from/to为RFC3339时间、半开区间[from,to)，最多93天，按申请created_at筛选。group_by为day/member/state，day按品牌IANA时区分组；可选member_id，禁止game_id。JSON列表另支持limit默认20、1..100与offset默认0、0..1000000，未知/重复参数拒绝。响应data为 `{brand_id,snapshot_at,timezone,query,summary,items,total_groups}`，query完整回显且game_id=null；summary覆盖完整筛选而非当前页，items为key/label/totals。即使分页越过末页，summary也不必为零。

Totals含order_count/requested_points，以及reviewing、processing、paid、rejected、failed、cancelled各自的_count/_points，共14个非负规范整数字符串，汇总可超过int64。此为申请时间范围内订单的当前状态投影，不是不可变日月结账、出款时间报表或外部转账记录；之后状态变化会改变同一范围的分布。

导出不接受limit/offset，完整筛选超过10000组或4MiB返回413，不截断。UTF-8 BOM CSV固定列依次为record_type/brand_id/snapshot_at/timezone/from/to/group_by/member_id/key/label及上述14个Totals字段；summary与每条group都回显会员筛选，无game_id。响应为CSV字节而非JSON，提供X-Report-Brand-ID/Kind/Snapshot-At/Group-Count/SHA256/Format-Version/Audit-ID及下载文件名。客户端校验完整摘要、范围、列和组汇总后下载，积分列按文本导入。范围内无会员返回经审计的404，不输出半份CSV；查询或审计失败503。

## 4.4 S6-d 真实运营报表

仅后台 GET，走主库；权限分离为 `report_betting.view.brand/platform`、`report_ledger.view.brand/platform`。品牌授权必须匹配当前品牌成员范围，平台授权须显式授予；超级管理员标记本身不授予报表权限。新权限只自动补给对应服务器引导角色，自定义角色不扩权。成功查询写审计；审计失败返回503，不输出半份报表。

| 方法 | 路径 | 分组与筛选 |
|---|---|---|
| GET | /admin/reports/betting | group_by=day/game/member，可选 game_id/member_id |
| GET | /admin/reports/ledger | group_by=day/entry_type，可选 member_id；拒绝 game_id |

两者必须传 RFC3339 `from/to`，半开区间 `[from,to)`，to>from 且不超过93天；limit 默认20、范围1–100，offset 默认0、最大1000000。未知/重复参数、空UUID或不适用分组均400；品牌内无此会员/彩种404，不能静默扩大范围。积分/计数聚合为规范十进制字符串，允许超过单账户int64；net_points 可以为负，其他金额/计数非负。

公共返回 `{brand_id,snapshot_at,timezone,query:{from,to,group_by,limit,offset,game_id,member_id},summary,items:[{key,label,totals}],total_groups}`。query中无筛选的UUID为null；snapshot_at是数据库语句时间，timezone是品牌分组时区。每个响应的汇总/分组/分页总数来自同一SQL语句快照；两个端点是独立快照，不宣称跨端点原子月结。day键按品牌时区，game键为彩种UUID、label为当前彩种名；member仅UUID，无用户私密资料。分页按key稳定排序，不输出伪增长或利润率。

投注 totals 字段：`order_count,stake_points,placed_count,won_count,lost_count,abnormal_count,cancelled_count,refund_points,settled_stake_points,unfinalized_stake_points,abnormal_stake_points,current_prize_points,correction_open_count`。时间按原注单 placed_at（不是结算时间）；取消包括投注取消/判定取消，退款取有原refund引用的全额投注。最终投注/奖金仅计 period已结算、当前job已完成且注单计算属于此代次、没有未完成更正的won/lost；更正中旧已付不冒充最终结果。未完成投注包含placed及非最终won/lost，不含取消/异常。金额分区：总投注=已最终结算投注+未完成投注+异常投注+已退回投注。correction_open_count是涉及未完成更正的注单数，不是期次数。更正可更新旧时间区间的实时统计，不等于不可变日/月结凭证；这里不是提现/代理有效流水资格算法。

账本 totals 字段：`entry_count,net_points,recharge_points,prize_credit_points,prize_reversal_points,refund_points`。时间按实际ledger.created_at，net为全部来源状态的delta之和（新16桶、旧完整12桶）；冻结/解冻状态转移净值0，prize/prize_reversal正负分别保留，不能累计历史paid代次替代净变动。另返回 `balances:{account_count,available_points,frozen_points,withdrawal_points,total_points}`，是同语句快照的当前品牌/会员桶汇总，不受from/to限制，也不是全链对账“一致”证明。完整单会员对账仍使用钱包reconciliation接口。

客户端校验品牌、回显筛选、精确整数及金额/状态分区；日期按精确纳秒时间值比较，允许等价UTC/时区或尾零格式，拒绝真实边界变化。服务端按PostgreSQL微秒网格将两个边界向上取整，保持原半开区间语义；不让驱动截断纳秒改变结果。旧结果在新筛选请求发起时清除，失败/401/跨品牌切换不能保留旧数据冒充新范围。草稿不改变已显示范围，只读刷新使用已提交条件。

S7-k 接入 GET `/api/v1/admin/reports/betting/export` 与 `/api/v1/admin/reports/ledger/export`，完整筛选范围、不接受分页参数；view与export分别授权，200直接返回CSV而非JSON信封。列定义、大小限制、快照与审计要求见 [CSV导出合同](11-report-csv-exports.md)。后续已接入提现申请状态报表及CSV，不代表外部出款；佣金/奖励报表、代理分组、不可变日月结及大规模异步全链对账仍未实现，不能据此宣称EPIC-11整体完成。

## 4.7 品牌运行状态

### 平台品牌创建

`POST /api/v1/admin/brands` 要求当前有效管理会话、精确 `brand.create.platform` 权限及 `Idempotency-Key`；禁止非空 `X-Brand-ID` 和任何查询参数。不因 `super_admin` 标记或品牌角色中的同名权限而放行。该接口不是用户、资金或规则写权限的替代品。

正文必须恰好包含 `{code,name,default_locale,timezone,reason}` 五个字符串，拒绝重复、未知、缺失或null字段。code为小写字母开头的1–48位小写字母/数字/下划线；name非空、UTF-8≤120字节；reason非空、≤500字节；名称和原因拒绝首尾空白及控制字符。语言仅en/zh-CN，timezone须为有效命名时区、非Local、≤80字节且无首尾空白/控制字符。服务端随二进制提供时区数据，不依赖主机安装。

201返回 `{id,code,name,status:"paused",default_locale,timezone,version:1,created_at,audit_log_id}`。品牌、七项初始配置、不可变创建证据、审计与加密幂等回执同事务提交；不自动建立管理员范围、会员、域名、游戏或积分。结算mode=null，代理配置disabled、合规检查全关闭。编号已被其他请求占用返回409 `BRAND_CODE_CONFLICT`，同键异体409 `IDEMPOTENCY_CONFLICT`；模型解析错误400 `REQUEST_INVALID`，服务语义错误400 `BRAND_CREATE_INPUT_INVALID`，无权限403 `PERMISSION_DENIED`。

平台幂等分区以账号/操作/键唯一，不伪造品牌UUID；加密绑定分区、正文摘要与操作。锁前后均检查当前会话和权限，撤销后不能重放旧成功。缓存回执是首次创建状态，品牌后来恢复或改配置后仍返回该快照；必须独立查询当前状态。未知提交结果只允许原正文/键重试。

实际Host属于启用的 `platform_domains` 时，即使没有可用公开用户品牌，也可管理登录/退出、恢复会话、读取品牌列表及创建。管理入口仍需先被服务器拥有者配置，未知Host或伪造转发Host不获得访问。用户端保持原有解析，裸 `/api/v1/context` 不会自动切到新品牌；平台 `/b/{brandCode}` 仍按原规则解析。

### 品牌运行状态接口

路径均在 `/api/v1/admin`，必须通过 `X-Brand-ID` 指定品牌。读取需 `brand_operation.view.brand/platform`，写入需 `brand_operation.write.brand/platform`，品牌权限还要求对应品牌范围。超级管理员不自动获得授权；显式平台写权限可以管理不同品牌的运行状态，不授予用户、积分或其他业务写权限。

| 方法 | 路径 | 合同 |
|---|---|---|
| GET | /brand-operation | `{brand_id,version,name,status,updated_at,audit_log_id?}`；返回真实品牌当前状态与共享配置版本 |
| PATCH | /brand-operation | `{version,status,reason}`，status 仅 active/paused；200 返回原操作回执及 audit_log_id；必须有幂等键 |
| GET | /brand-operation/history | `{items:[{id,brand_id,version,previous_status,status,changed_by,reason,audit_log_id,created_at}],limit,offset}`；默认20，limit1..100，offset0..1000000 |

变更立即生效，不要求双人审批。提交前后及原键缓存重放均重新验证当前管理会话和权限。请求拒绝未知、缺失、重复和 null 字段；原因必须为非空白 UTF-8 且不超过500字节。查询拒绝未知、重复或空分页参数；当前状态 GET 和 PATCH 不接受查询参数。

运行状态版本与认证配置共享，因此验证码或 Telegram 配置变化也可能造成版本冲突。PATCH 在品牌行独占锁下检查版本和状态，状态、版本、审计及历史同事务提交；已有持有品牌共享锁的用户操作先完成，暂停成功之后的新投注不得扣分。暂停不撤销登录会话，不自动取消订单、期次或阻止原路退款和已有开奖结果结算，不代表尚未实现的提现出款已经可用。

同状态重复操作或禁用品牌的切换返回409 BRAND_OPERATION_STATE_CONFLICT；无效输入400 BRAND_OPERATION_INPUT_INVALID；旧版本409 BRAND_OPERATION_VERSION_CONFLICT；无权限403 PERMISSION_DENIED。未知结果始终用原正文与原键重试。历史成功回执可能早于最新状态，核对回执后须独立 GET；不得把旧的暂停回执当成品牌现在仍暂停。

## 4.5 S6-e 代理身份、层级与配置（不计算佣金）

后台权限分别为 `agent.view.brand/platform`、`agent.write.brand`、`agent_policy.view.brand/platform`、`agent_policy.write.brand`；超级管理员只能显式平台读取。默认政策 `{enabled:false,max_depth:5,ratio_cap:"0",mode:"loss",cycle:"monthly"}` 仅提供初始配置，不代表已选择生产佣金方案；enabled 只启用代理配置管理，绝不启用派发。

| 方法 | 后台路径 | 合同 |
|---|---|---|
| GET/PUT | /admin/agent-policy | 读取政策；PUT `{version,config,reason}` |
| GET | /admin/agent-policy/history | 品牌政策不可改写历史 |
| GET | /admin/agents/tree | 只分页根节点或 `parent_id=UUID` 的直属节点，不无界加载整树 |
| GET | /admin/agents/{id} | 节点和当前继承模式 |
| POST | /admin/agents | `{policy_version,member_id,parent_id,parent_version,config,reason}`，201 |
| PUT | /admin/agents/{id} | `{version,policy_version,parent_version,config,reason}`，200 |
| GET | /admin/agents/{id}/history | 节点不可改写配置历史 |

政策返回 `{brand_id,version,config:{enabled,max_depth,ratio_cap,mode,cycle},updated_at,audit_log_id?}`。max_depth 1–32是配置/工程安全界限；ratio_cap 是0–1最多6位规范小数字符串，0.1=10%，不接受多余末尾0、正号、数字型JSON。mode loss/turnover（输赢/流水）、cycle weekly/monthly；周期仅保存设置，尚不生成周/月结任务。

节点返回 `{id,brand_id,member_id,parent_id,depth,path,version,config:{ratio,mode,status,can_create_children},effective_mode,mode_source_agent_id,policy_version,parent_version,created_by,created_at,updated_at,audit_log_id?}`。parent_id/parent_version 根为显式null，子为UUID/正版本；path为根到自身UUID数组。mode为null（最近祖先或品牌继承）/loss/turnover；effective_mode与来源是读取时或首次回执时的派生信息，不是不可变财务基数。status active/disabled，发展下级标志只控制新增，不等同所有财务权限。每品牌会员最多一个代理节点，父/成员/路径不可直接重写；新增须正常品牌成员、启用政策、活跃祖先且直接父级允许发展。

新比例不超过直接父级和品牌上限、层级不超过政策。根节点可覆盖品牌默认模式；子级只能继承或显式使用与父级相同的有效模式，不同模式返回400 AGENT_INPUT_INVALID。父级/品牌模式变更若使现存路径混合则409 AGENT_LIMIT_CONFLICT。0045保持旧混合配置与历史原值，相关新写入安全拒绝，不自动覆盖或迁移历史。降低父比值、品牌上限或最大层级若会使已有节点超限则409，不自动改下级。所有写入先锁品牌代理政策，再检查版本/节点；互斥独立于投注账本锁。SQL同样约束路径、身份、父子上限、版本+1、不可改写历史与匹配审计，不能用绕过服务的普通UPDATE提交无历史的新配置。

树返回 `{brand_id,parent_id,items:Node[],limit,offset,total_count:string}`；不存在或外品牌父级404，分页limit默认20/1–100、offset默认0/最大1000000，未知或重复参数拒绝。历史返回 `{brand_id,agent_id,items:[{id,brand_id,agent_id,version,config,actor_type,actor_id,reason,created_at,audit_log_id}],limit,offset,total_count}`；政策agent_id为null、初始系统记录actor_id/audit为null；后续写入及节点创建均须实际审计。后台查询可审计，用户端不暴露历史理由。

用户路径同时支持 `/api/v1` 与 `/api/v1/b/{brandCode}`：

| 方法 | 用户路径 | 范围 |
|---|---|---|
| GET | /agent/me | 自己的品牌成员对应节点，无代理404 |
| GET | /agent/children | 仅自己的直属节点，只有分页参数，拒绝任意parent_id |
| PUT | /agent/children/{id}/config | `{version,policy_version,parent_version,ratio,mode,reason}`，只改直属下级比例/模式 |

用户写入必须全局账号active、品牌成员normal、政策启用、自身和祖先代理active、目标是active直属下级；不能改自己的比值、兄弟/孙级、状态、发展标志、成员或父级，也不提供用户自行晋升/创建代理接口。会话/权限在幂等锁前后重新检查，停用/撤权后即使旧成功缓存也403/401；品牌从实际用户域名/路径会话解析，不信任伪造X-Brand-ID。

所有PUT/POST需要幂等键、理由、匹配版本；同键不同内容409，缓存保存首次回执而非实时派生状态。SDK不先GET改写请求；未知结果保留原正文/键，GET不是确认，匹配回执后再独立读取。错误或不匹配的200仍属未知，不因为“收到200”丢掉原键。页内内存按账号/品牌/目标保存，退出清除，迟到回调不能清除新登录后的替代请求。

API当前不写积分、不生成佣金/奖励记录，不修改加入归属或历史注单。代理码/推荐码加入、用户晋升、重新挂接及注单归属快照仍待后续；佣金差额/独立分配、周期边界、实际计算与发放尚未启用，不把可配置树当作完整代理运营平台。

### 4.6 S6-f 加入码与归属（已接入）

- 后台 GET/POST `/api/v1/admin/join-codes`、GET/PUT `/join-codes/{id}`、GET `/join-codes/{id}/history`；独立 `join_code.view.brand/platform` 与 `join_code.write.brand`，超管只读。所有写入有幂等、原因、版本、不可改写历史与审计。
- Code DTO：`{id,brand_id,kind:"agent"|"referral",code,owner_member_id,agent_id:null|UUID,status:"active"|"disabled",starts_at:null|RFC3339,expires_at:null|RFC3339,version,usable:boolean,created_at,updated_at,audit_log_id?}`。code为服务器生成的24位大写十六进制字符串；身份/文本/类型/会员/代理不可换绑，需创建新码再停用旧码。usable仅为查询时状态，提交仍重新验证。
- POST body：`{kind,owner_member_id,agent_id:null|UUID,starts_at:null|RFC3339,expires_at:null|RFC3339,reason}`；agent必须属于同品牌owner会员，referral的agent_id必须null；初始active、version1，成功201。PUT body：`{version,status,starts_at,expires_at,reason}`，成功200和version+1。日期允许显式null，起止均有值时要求start<expire；精度最多微秒，开始含/到期不含。回执日期按同一微秒时刻匹配，允许等价时区和尾零表示，不能按毫秒抹去真实差异；写回执必须含有效audit_log_id，GET可省略。
- 列表 query仅 `kind`/`owner_member_id`/`limit`/`offset`，前两项可省；响应 `{brand_id,kind:null|kind,owner_member_id:null|UUID,items,limit,offset,total_count:string}`。分页默认20、limit1–100、offset0–1000000；未知/重复筛选拒绝。
- 历史响应 `{brand_id,code_id,items:[{id,brand_id,code_id,version,status,starts_at,expires_at,actor_id,reason,audit_log_id,created_at}],limit,offset,total_count:string}`；仅limit/offset分页。
- 用户 GET `/me/join-codes?limit&offset` 返回自身会员的Code分页 `{brand_id,member_id,items,limit,offset,total_count:string}`，不公开其他人的编码、审核理由或管理员资料。GET `/me/attribution` 返回 `{brand_id,member_id,join_method,joined_at,code_id:null|UUID,source_code:null|string,legacy:boolean}`，不公开上级配置和私人树。
- register/login首次加入及telegram首入增加可选 `agent_code` 或 `referral_code`，互斥；空字段等同未选，非空规范为去首尾空白并大写。编码必须属于当前品牌、在有效期内且启用；来源会员/全局身份正常，agent还须品牌代理政策及全部祖先启用。无效/外品牌/禁用/过期均统一 `JOIN_CODE_UNAVAILABLE`，不泄露来源身份。
- 已存在品牌成员携带非空加入码时返回409 `JOIN_ATTRIBUTION_FIXED`，不能换归属；重新正常登录需用户去掉编码并提交新操作。未接受品牌条款不能入品牌，运营新增成员仍需本人首次同意。运营新增可选上述编码，但join_method仍operator，快照记录code_kind及来源；不代同意、不改全局身份。
- 加入码不自动晋升代理，不覆盖旧成员或旧注单；停用编码仅阻止新的归属建立。会员初始归属与新注单的提交时代理政策/路径/配置版本由数据库保存不可改写快照；历史缺失只标legacy，不补造代理/佣金事实。此阶段无资金入账、奖励或佣金任务。
- 错误：`JOIN_CODE_INPUT_INVALID`400、`JOIN_CODE_NOT_FOUND`404、`JOIN_CODE_DENIED`403、`JOIN_CODE_VERSION_CONFLICT`409、`JOIN_CODE_STATE_CONFLICT`409、存储失败503。Code读写与原请求确认分开，断网/畸形回执保留原正文/键，单纯GET不能替代原回执；换品牌/会话及迟到回调隔离。

## 4.8 合规配置与显式伪检查

管理路由在 `/api/v1/admin`，均须真实会话和X-Brand-ID。读分别需compliance_policy.view.brand/platform、compliance_check.view.brand/platform；写compliance_policy.write.brand，检查compliance_check.run.brand，超级管理员不能写或运行，仅显式平台权限读取。配置和检查是独立权限，不复用用户、资金或规则权限。

| 方法 | 路径 | 合同 |
|---|---|---|
| GET | /compliance-policy | `{brand_id,version,config,updated_at,audit_log_id?}`；不接受查询参数 |
| PUT | /compliance-policy | `{version,config,reason}`完整替换；200下一版及audit_log_id |
| GET | /compliance-policy/history | `{brand_id,items:[{id,brand_id,version,config,changed_by:null或UUID,reason,audit_log_id:null或UUID,created_at}],limit,offset,total_count}` |
| POST | /compliance-checks | `{version,operation,reason}`；201不可变决策快照，不接受用户敏感资料 |
| GET | /compliance-checks | `{brand_id,operation:null或筛选值,items:Decision[],limit,offset,total_count}` |
| GET | /compliance-gates | `{brand_id,operation:null或registration/betting,items:GateRecord[],limit,offset,total_count}`；真实业务准入拒绝，使用compliance_check.view.brand/platform |

config必须包含五键 `{age_enabled:boolean,minimum_age:null或18..120整数,region_enabled:boolean,allowed_countries:有序唯一字符串数组,identity_enabled:boolean}`。年龄开关开启时年龄必填，地区开关开启时名单非空；名单最多250项，每项两位大写ASCII字母并严格升序。它们是工程配置限制，不是年龄/国家法律结论或已获授权市场清单。默认false/null/false/[]/false。拒绝未知/缺失/重复/null错误字段、控制字符和原因首尾空白；原因非空UTF-8≤500字节。版本独立于品牌共享版本。

Decision字段 `{id,brand_id,policy_version,config,operation,decision,checks,adapter_mode:"stub",created_by,reason,audit_log_id,created_at}`。operation为registration/betting/withdrawal，仅检查场景标签，不启动相关业务。checks严格按age/region/identity排序，各含check/enabled/decision/reason_code；关闭allow/CHECK_DISABLED，开启review/ADAPTER_NOT_CONFIGURED，整体全关闭才allow。deny/freeze为接口扩展枚举，当前不会生成这两个结果或执行冻结；allow不代表用户验证完成，review不创建审核队列。

查询默认limit20、1..100，offset0..1000000；历史仅接受分页，检查列表另可operation筛选，拒绝未知/重复/空参数，total_count为精确字符串。配置和决策与审计、历史、加密幂等回执同事务；检查锁定当前政策版本，改版后新检查需新版本，旧键则重放原决策快照。未知写只允许原正文/键重试，撤权/会话变化/品牌停用后不得用旧缓存绕过；停用品牌可只读。配置错误400 COMPLIANCE_INPUT_INVALID、缺失404 COMPLIANCE_NOT_FOUND、版本409 COMPLIANCE_VERSION_CONFLICT、状态409 COMPLIANCE_STATE_CONFLICT，正文解析失败仍400 REQUEST_INVALID。

真实业务准入使用同一品牌政策：任一检查开启而真实适配器未配置时，新注册、首次入品牌（密码/Telegram）、运营新增成员、待确认成员首次接受条款，以及投注预览和最终新提交均409 COMPLIANCE_REVIEW_REQUIRED。既有已接受条款会员的登录、查询、绑定、取消/退款和已有业务处理不因该开关被拦截；原有账号状态、条款、品牌暂停、期次和权限检查仍执行。旧预览不授予最终下注许可，提交重新检查当前政策；客户端不能发送identity_verified、年龄或地区等声明绕过。

GateRecord含 `{id,brand_id,policy_version,config,operation,action,decision:"review",checks,adapter_mode:"stub",actor_type,actor_id:null或UUID,member_id:null或UUID,request_id,audit_log_id,created_at}`。action register/join/operator_join/bet_preview/bet_place，人员/会员的组合见领域模型。按同品牌及可选registration/betting过滤，拒绝其他/空/重复参数；计数和记录同快照。该列表不混入显式管理模拟记录，不含用户名、凭证、证件、原幂等键或IP。

普通写拒绝由服务端私有事务回调在ROLLBACK TO SAVEPOINT business之后保存证据，再与加密负回执共同提交；回调不属于JSON协议。证据失败整事务回滚、返回503，不缓存不完整拒绝。首次拒绝的原键在政策关闭后仍重放原409，要发起新的业务意图须新键；旧成功写原键只恢复历史结果，不产生新业务，仍遵守原会话/权限检查。投注预览不是幂等写，每次观察独立记录；若记录失败返回503而不是无证据409。默认关闭不生成拒绝记录，不构成验证通过。

提现尚未实现，不伪造提现闸门；真实证件/年龄/地区服务、复核队列、资金冻结及完整责任博彩继续待验收。运营不能据启用开关或一条allow记录宣称平台已满足生产合规。

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
- NOTIFICATION_INPUT_INVALID
- NOTIFICATION_NOT_FOUND
- NOTIFICATION_STATE_CONFLICT

## 6. 事务、幂等和重试

- 幂等键必须保存请求摘要；相同键不同请求内容返回 IDEMPOTENCY_CONFLICT。
- 相同键相同内容返回第一次成功或最终失败结果，不重新执行业务。
- 事务提交后才发布 Outbox 事件；消费者按事件 ID 幂等。
- 外部开奖源请求可以重试；人工开奖和结果纠正不得自动重试。
- 结算失败由运营人员手动触发重试；异常注单不进入普通重试。
- 从库延迟时，写操作返回主库结果，前端在短时间内使用主库粘滞读取。

## 4.2 S5-c2 正式期次结算与整数积分派奖

均在 `/api/v1/admin`，要求真实后台会话、X-Brand-ID；读 `settlement.view.brand/platform`，模式读 `settlement_policy.view.brand/platform`。写分别要求 `settlement.run.brand`、`settlement.approve.brand`、`settlement.retry.brand`、`settlement_policy.write.brand`，超管禁止全部写。所有写带可信 Origin、幂等键、必填 reason（UTF-8 ≤500 字节），事务/幂等锁前后重新验证权限与会话，未知/重复字段拒绝。

| 方法 | 路径 | 合同 |
|---|---|---|
| GET | /settlement-policy | `{brand_id,version,mode,updated_at}`；初始 v1、mode=null，不启用 |
| PUT | /settlement-policy | `{version,mode:null\|automatic\|manual,reason}`；200，扁平下一版本政策及 audit_log_id；mode 字段不得省略 |
| GET | /periods/{id}/settlement-context | 品牌/彩种/期次、期次版本/状态、draw_result_id、当前政策版本/模式、can_start |
| POST | /periods/{id}/settle | `{version,policy_version,draw_result_id,reason}`；201 Job，要求 drawn、当前结果/政策和非空模式，原子锁定目标并转 settling |
| GET | /periods/{id}/settlement | `{settlement:Job\|null}`，原始期次不存在返回 404 |
| GET | /settlement-jobs/{id} | 实时 Job；不能以首次幂等写回执代表 worker 当前状态 |
| POST | /settlement-jobs/{id}/approve | `{version,reason}`；manual 的 awaiting_approval→paying，200 Job |
| POST | /settlement-jobs/{id}/retry | `{version,reason}`；仅 failed 重新进入保存的 processing/paying 阶段，200 Job；不重新核算 excluded 异常 |
| GET | /settlement-jobs/{id}/targets?limit=20&offset=0 | `{brand_id,job_id,items,limit,offset,has_more}`，1..100，offset≤1000000 |

Job 包含 `id,brand_id,game_id,period_id,draw_result_id,period_version,policy_version,mode,state,version,target_count,created_by,approved_by,reason,created_at,completed_at,last_error_code,pending_count,ready_count,paid_count,excluded_count,failed_count,prize_points,paid_points,can_retry`。nullable 字段明确返回 null。状态 processing→automatic paying / manual awaiting_approval；人工批准后 paying→completed，处理中任一目标存储/账本失败→failed。各状态计数之和等于目标数。金额汇总使用任意精度非负十进制字符串，单注金额为 int64 十进制字符串，禁止 Number/浮点汇总。

Target 包含 `order_id,member_id,state,version,calculation_id,order_version,order_status,won,prize_points,payout_entry_id,error_code`。paid 代表结算已应用，不保证有非零账本；未中奖或零额中奖不创建零金额流水。异常/已取消为 excluded；已计算后被取消的计算证据仍保留，但不计入应付/已付汇总。未入账注单取消/人工标异常排除目标并增加任务版本，旧审批请求 409。

每单核算保存购买时版本及完整精确计算。中奖仅增加 winning.available，同事务写账本前/后值、注单 won/lost、目标 paid、审计和 Outbox；稳定 operation_key 为 `settlement-payout:<calculation_id>`。全部 paid/excluded 才提交 Job completed 和期次 settled。系统核算异常写 `source=system,marked_by="",job_id,error_code`，人工异常仍为 source=manual；不把操作者伪装成系统异常的人工标记者。失败历史不可改写，worker 重启不自动重试失败。阶段转换/期次收尾失败也记录 job failed（失败证据 order_id=null）；若积分已入账，保留 paid/paid_points，人工重试仅收尾，不重新派奖。

写幂等回执保存首次响应：启动回执 processing/v1、批准回执 paying/下一版可能早于当前服务器进度。同键异体 409；客户端验证匹配后必须 GET 读取实时结果。配置不会自动启动任务，也不改变已启动任务快照。S5-c3 更正/冲正使用下节专用路径；普通取消/重试仍不能撤回已派奖订单。重开未实现。

## 4.3 S5-c3 结果更正、冲正及重新结算

路径在 `/api/v1/admin`，读 `draw.view.brand/platform`，写 `draw.correct.brand` / `draw.correction_retry.brand`；有原结算代次时另需 `settlement.run.brand`。两项权限在幂等锁前后重新验证，包括缓存回执；超管只读。写带可信 Origin、幂等键、必填 UTF-8≤500字节原因，拒绝重复/未知/缺失字段。result 必须显式含 regular/special/digits 三个整数数组，非适用组为 []。

| 方法 | 路径 | 合同 |
|---|---|---|
| GET | /periods/{id}/correction-context | 品牌/彩种/期次、版本/状态、当前结果及模型、当前 job ID/version、当前策略/模式、requires_resettlement、can_correct |
| POST | /draw-results/{id}/correct | `{version:期次版本,policy_version:number\|null,result:{regular:[],special:[],digits:[]},reason}`，201 Correction；URL 必须是本期当前结果 |
| GET | /periods/{id}/corrections?limit=20&offset=0 | `{brand_id,period_id,items,limit,offset,has_more}`，每页1..100、offset≤1000000 |
| GET | /corrections/{id} | 实时 Correction，与首次缓存回执区别 |
| GET | /corrections/{id}/targets?limit=20&offset=0 | 品牌、更正 ID、逐单原状态/金额/旧核算与账本 ID、冲正 ID/重置版本、状态/错误码及分页 |
| POST | /corrections/{id}/retry | `{version:更正版本,reason}`，200 原始下一版 reversing 回执；只恢复 failed 冲正阶段，不重新冲正已完成目标 |

无原 job 的 drawn 期次仅更正结果：policy_version 必须 null，不要求启用派奖，不创建结算任务或改钱包，原子返回 completed/v1。已有 settling/settled job 则要求显式配置非空模式、匹配政策版本；返回 reversing/v1，冻结旧代次，保存目标快照，并把期次置/保留 settling。取消期次不通过更正复活注单。禁止无变化更正，按模型验证数量/范围/重复/上一期异常，普通/特别无序号码规范排序，数字位序保持。新记录保留实际 drawn_at，created_at 表示更正时间；corrected_from_id 引用旧结果，旧记录不可覆盖。

Correction 含 `id,brand_id,game_id,period_id,previous_draw_result_id,draw_result_id,result,period_version,previous_job_id,new_job_id,policy_version,mode,state,version,target_count,created_by,reason,created_at,completed_at,last_error_code,pending_count,reversed_count,unchanged_count,excluded_count,failed_count,reverse_points,reversed_points,can_retry,new_job_state,new_job_version,new_job_error_code`。状态 reversing→resettling→completed；冲正/发布失败→failed，只人工重试。new_job_* 为实时关联状态：新结算失败在正常结算 retry 路径处理，不把整个更正重新执行。所有计数总和等于 target_count；汇总非负整数字符串可超 int64，逐单仍为 int64 字符串。

Target 含 `order_id,member_id,state,version,old_order_version,old_order_status,old_calculation_id,old_payout_entry_id,old_prize_points,reversal_entry_id,reset_order_version,error_code`。状态 pending/reversed/unchanged/excluded/failed。旧 won/lost 全额撤回已发中奖积分后，当前注单 projection 重置 placed 并增加版本；旧未应用 placed 为 unchanged，不撤 stake；人工/系统异常及已取消 excluded，不重新核算。旧规则/选号/扣款/来源分配不变，stake 只扣一次。正额冲正 entry_type=prize_reversal、reference_type=draw_correction、reference_id=更正ID、reversal_of=原prize、operation_key=`draw-correction:<id>:<order_id>`，只扣 winning.available，新16桶前后值与审计同事务，旧12桶证据保留并仅在比较时规范化；零奖/lost 无零金额冲正流水。

全部 reversed/unchanged/excluded 后，才发布新 current draw 并创建下一 generation（原 generation+1）job，使用更正启动时的政策/模式快照。新 job 沿用普通核算/批准/派奖流程，完成后更正及期次才完成。Job 增加 `generation,previous_job_id,correction_id,current`；旧 job/计算/目标/账本不改写，历史 paid_points 是曾入账金额，不代表当前余额。只有 current=true 可计算、批准、重试；新任务 ID 指针决定当前代次，不根据时间或最大 ID 猜测。

原中奖可用不足、被使用或冻结时停止并记 `WINNING_AVAILABLE_INSUFFICIENT`，不扣充值/赠送、不自动解冻、不产生负余额/欠款。失败可能已有其他目标完成冲正；原结果仍是当前，直到全部成功才发布新结果。运营处理后按原目标继续，不重做已冲回记录；失败历史及错误码可查。未来欠款/追偿业务规则仍未决，不据本实现擅自上线。

POST 回执保存首次结果，SDK 不在写方法内重新取上下文/改写回执；未收到回执时始终用冻结的旧 draw ID、正文、键重试，即使 worker 已推进。匹配回执确认后再 GET 实时数据，后读失败保持“已确认”。审计/旧证据留存；现有投注通知不因更正被删除（原投注事实仍有效）。后续佣金、奖励、财务报表和中奖通知接入时必须按 generation/current 与补偿事件增加对应回溯，不能累计所有历史代次冒充净值。

## 佣金金融政策当前接口

0047注册GET/PUT `/api/v1/admin/commission-policy`及GET `/api/v1/admin/commission-policy/history`。读取要求commission_policy.view.brand/platform，写入要求commission_policy.write.brand且超级管理员不能写；初始化管理员包含对应范围的显式授权。版本、config、reason完整替换，幂等回执前重验实际会话和权限，审计/修订/政策一起提交。启用须显式日历并匹配已启用代理政策周期；历史NULL不补造授权。封闭字段、错误、分页和私有投注快照详见[金融政策合同](16-commission-policy-snapshot.md)及生成OpenAPI。这里不是设计中的commission-rules或派发接口。

0048注册GET/POST `/api/v1/admin/commission-cycles`、GET `/{id}`、GET `/{id}/earnings`、POST `/{id}/retry`、GET `/{id}/runs`和GET `/{id}/runs/{runID}/calculations`。读取要求commission.view.brand/platform；登记/重试分别要求commission.run.brand/commission.retry.brand及品牌查看权，平台超级管理员不能写。写入须原始正文/幂等键匹配并重新授权；ready只是核算结果，须结合evidence_current阅读，不能当成批准或积分入账。完整字段、分页、封闭窗口、历史代次及失败规则见[周期核算合同](17-commission-cycles.md)。派发使用独立0050路由，不复用周期执行权限。

0049注册GET `/api/v1/admin/commission-discovery`及POST `/{id}/retry`，复用commission.view品牌/平台读权及品牌commission.retry写权。列表只含操作状态及引用，不泄露快照；retry仅处理failed且重验版本、权限与品牌状态，原键返回原pending回执。系统自动登记周期的创建者为空，creation_actor_type为system；人工及历史周期为admin。旧0048回执可缺少新增来源字段，不改写旧回执。完整队列调度与边界合同见[周期核算](17-commission-cycles.md)。

0050注册GET/PUT `/api/v1/admin/commission-payment-policy`、GET `/commission-payments`、GET `/{id}`、POST `/{id}/approve`及POST `/{id}/retry`。读取使用commission.view品牌或平台；策略写入、批准及重试分别要求品牌commission_payment_policy.write、commission_payment.approve、commission_payment.retry加查看权。品牌派发开关默认false；全人工等待单人审核，全自动在开关启用后由系统处理。0054使混合模式整周期等待人工审核，保留mixed身份，不拆分自动部分；旧MODE_UNRESOLVED记录在无批准、无目标且证据有效时只能显式approve。每目标正额实际写入佣金available和完整账本，零额仅审计。已有正额入账的更正仍blocked，不能以approve或retry绕过。原键返回原回执，当前状态另读；具体字段、状态、锁及待补偿边界见[派发合同](17-commission-cycles.md)和OpenAPI。

0055注册后台`/reward-orders`六项接口：GET/POST集合、GET单条、GET动作分页、POST撤销及POST人工继续。奖励权限独立，X-Reward-Actor-ID固定操作者；人工发放201、撤销或继续200，余额不足亦为持久化revocation_pending成功回执，不由客户端自动重试。原grant及回执保留，只有gift.available可反向全额；原键重放重新授权并返回原状态，当前状态另读。无用户奖励提交、自动奖励或支付接口，具体数据与待完成页面/通知/报表见[奖励交接合同](20-manual-reward-orders.md)及OpenAPI。

0051追加GET `/api/v1/admin/commission-payments/{id}/targets`及GET/POST `/api/v1/admin/commission-payment-targets/{id}/adjustments`，写入为独立commission_adjustment.write品牌权限、version/points/reason闭合正文、确认账号及幂等键；成功201。只接受当前已paid且证据有效的目标，精确差额记入佣金available，原派发总额不修改。0052仅为实际完成的入账/修正生成不可变通知，完整状态、字段和错误见[人工修正合同](18-commission-adjustments.md)。

0053追加GET `/api/v1/admin/reports/commission`及`/reports/commission.csv`，独立report_commission.view/export品牌或平台权限。from/to按真实ledger.created_at半开入账窗口筛选，group_by为day/agent/cycle，可选agent_id/member_id/cycle_id；前六项计数和积分非负、net_points可为负的精确字符串。CSV导出完整筛选范围，不接受分页，最多10000组/4MiB，审计后才输出且可核验SHA256；原派发不因多条修正重复累计，完整合同见[佣金账本报表](19-commission-posting-reports.md)。
