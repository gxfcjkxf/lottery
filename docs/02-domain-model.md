# 领域模型与数据库实现约定

## 1. 数据库约定

- PostgreSQL；所有时间 `timestamptz`，数据库统一 UTC。
- 主键默认 UUIDv7；外部展示的期次号、订单号可以另设不可猜测业务编号。
- 积分字段 `bigint`，不得使用浮点；积分变动必须允许正负，但账户最终余额不能小于 0。
- JSONB 只用于规则、选号快照、开奖结果和外部原始数据引用；可查询字段仍需结构化保存。
- 所有带品牌业务的表必须有 `brand_id`，并建立品牌范围索引。
- 业务记录不物理删除；使用 `status`、`disabled_at` 或冲正关联表示失效。

## 2. 核心实体

### 全局身份

`global_users`

- `id`
- `username` nullable, global unique
- `phone` nullable, global unique
- `password_hash` nullable
- `telegram_user_id` nullable, global unique
- `status`: active/disabled/deleted
- `username_set_at`, `phone_set_at`
- `created_at`, `updated_at`

约束：至少存在一种可用登录方式；用户名/手机号首次补充后不可普通修改；Telegram 不允许解绑。

### 品牌与成员

`brands`

- `id`, `code`, `name`, `status`: active/paused/disabled
- `default_locale`, `timezone`
- `config_version`
- `created_at`, `updated_at`

新建品牌使用平台权限 `brand.create.platform`，初始 `paused/config_version=1`。现有INSERT触发器初始化积分、投注、提现、结算、代理、展示和合规七项配置；结算mode保持NULL，代理disabled、合规检查全关闭，不建立域名、管理员范围、会员、游戏或积分账户。

`brand_creation_records` 保存 `brand_id,code,name,default_locale,timezone,status,version,created_by,reason,audit_log_id,created_at` 初始证据，品牌唯一且不可更新/删除。写入必须匹配实际初始品牌和同事务 `brand.create` 审计。后续品牌配置修改不改写创建记录，不将创建时暂停/v1当成最新状态。

`brand_domains`

- `id`, `brand_id`, `domain`, `is_primary`, `status`
- 全局唯一 `domain`

域名管理沿用实际表的 `enabled` 布尔字段表示启用/禁用，不新增第二套状态列。绑定的编号、品牌和主机名不可修改或删除；禁用也继续保留全局归属。最多100条绑定/品牌（含禁用），平台入口的主机名同样不能被新增品牌绑定占用。原localhost/IP绑定保持原值且可调整启停/主域名标志；新绑定仅接受规范的小写ASCII多标签DNS形式主机名。

`brand_domain_revisions` 保存 `id,brand_id,version,changed_by,reason,audit_log_id,created_at,before_domains,domains`，前后绑定数组各含 `id,domain,enabled,is_primary` 并按域名/编号排序。操作沿用品牌共享版本和品牌行锁，主域名切换自动将旧主域名降为普通绑定，不禁用旧域名。绑定更新与不可改写审计历史在同一事务提交；版本与品牌唯一，历史不可更新或删除。数据库更新/删除约束保护绑定身份与更新历史；初始化的直接插入不补造人工审核记录，运营新增必须经过受审计服务。

`brand_members`

- `id`, `brand_id`, `global_user_id`
- `status`: normal/frozen/disabled/expired/cancelled
- `display_name`, `notes`, `profile_snapshot` JSONB
- `join_method`: domain/agent_code/referral_code/operator
- `join_domain`, `agent_id`, `referral_id`, `joined_at`, `created_by`
- `terms_accepted`, `accepted_at`, `privacy_policy_version`, `service_terms_version`；运营新增成员在本人首次确认前为 false/NULL，不可由运营代同意。
- unique `(brand_id, global_user_id)`

归属字段在加入时写快照；后续代理关系修改不能影响历史注单。

S6-f 使用现有 attribution_snapshot 保存不可改写的 schema_version/legacy/join_method/join_domain/joined_at、code_id/version/text/kind、owner_member_id、agent_id/referrer_member_id、agent_path 和加入时配置证据；agent_id/referral_id 为快照内部字段，不新增可任意重写的成员列。新加入由数据库锁定当前品牌政策、编码和来源身份，记录同一个实际数据库时刻与有效性；只在首次入品牌确定来源。运营新增仍为operator，编码类型另存，created_by真实且terms_accepted=false。存量原payload保存在legacy_payload，不推断任何旧代码或代理链；其他成员字段保持不变。

新 bet_orders.attribution_snapshot 保存会员原归属、提交时品牌代理政策及沿路径节点配置/版本、captured_at；旧注单仅显式legacy，不补造财务规则。commission_policy仍null，不能据配置快照直接派佣金。私有路径和比例不进入普通用户注单DTO或公开归属接口，取消/结算/更正不能改写此字段。

### 后台账号与权限

`admin_accounts`, `roles`, `permissions`, `admin_account_roles`, `role_permissions`, `admin_brand_scopes`

- 权限键格式：`resource.action.scope`
- 账号最终权限为角色权限并集去重。
- `admin_brand_scopes` 限制品牌范围；平台范围权限不自动授予写权限。
- 角色含 `brand_id`（平台角色为 NULL）、`status`、`version`、`is_bootstrap`，普通账号有 version。
- 品牌角色的 `(brand_id, code)` 唯一，brand_id/code 创建后不可修改；管理员必须先拥有对应品牌范围才可绑定角色，绑定期间不可移除该范围。权限必须在品牌作用域内计算，不能把其他品牌角色并集当作本品牌授权。

### 品牌配置

`brand_operation_revisions` 保存品牌运行状态变更：`id,brand_id,version,previous_status,status,changed_by,reason,audit_log_id,created_at`；品牌和版本唯一，历史不可修改或删除，写入须匹配实际品牌状态及同事务审计证据。此模块只提供 active 与 paused 之间的切换，不提供禁用品牌；品牌创建和域名配置使用独立模块。

运行状态沿用 `brands.config_version`，与认证配置共享版本和品牌行锁，因此历史版本可有间隔。暂停和恢复递增版本，保留名称、语言、主题、认证配置、成员和全部资金数据；初始化不补造人工操作或审计记录。

S3-b 已实现 `brand_point_policies`：brand_id 主键，version 与 max_balance_points/max_recharge_points/max_adjustment_points（nullable bigint，正值或不限）。现存品牌迁移初始化，新品牌数据库触发器初始化；版本独立于认证配置，修改在同事务内审计。

`config_versions`

- `id`, `scope_type`, `scope_id`, `config_key`, `config_value` JSONB
- `version`, `status`: draft/pending/active/expired/rejected
- `effective_at`, `effective_period_id`, `created_by`, `approved_by`

配置查找顺序：平台 → 品牌 → 彩种 → 玩法。最终读取结果必须带配置版本。

### 合规配置与显式检查

`brand_compliance_policies` 保存 `brand_id,version,config,updated_at`，版本独立于品牌共享版本。config恰好age_enabled/minimum_age/region_enabled/allowed_countries/identity_enabled五键；初始false/null/false/[]/false。`compliance_policy_revisions` 保存每版config、changed_by、reason、audit_log_id、created_at；初始v1为系统记录、人员/审计为NULL，后续必须有匹配同事务审计，不可更新/删除。配置修改+1且必须有对应历史，数据库延迟约束禁止只更新投影。

`compliance_decisions` 保存id、brand_id、policy_version、config快照、operation、decision、三项checks、adapter_mode、created_by、reason、audit_log_id、created_at。关联同品牌政策版本，检查与审计相符且不可改写。当前adapter_mode固定stub：关闭项allow/CHECK_DISABLED，开启项review/ADAPTER_NOT_CONFIGURED，整体有开启项才review。allow不是用户已验证，review不创建队列，保留的deny/freeze枚举不表示实际用户禁用或资金冻结。

查询按品牌和可选operation分页，数量为规范非负整数字符串；单响应在只读RepeatableRead事务中读取计数/记录。管理员显式检查不启动业务；真实准入另接下述拒绝记录，不收集证件、生日、IP归属或真实验证结果。

`compliance_gate_rejections` 保存真实新业务拒绝的id/brand_id/policy_version/config/operation/action/decision/checks/adapter_mode/actor_type/actor_id/member_id/request_id/audit_log_id/created_at，关联不可变政策修订；与显式管理检查分表。action为register/join/operator_join/bet_preview/bet_place，operation仅registration/betting。anonymous注册人员/会员均NULL，运营新增为实际admin且会员NULL，首次入品牌为已存在user（待确认会员可有member_id），投注必须有同品牌user/member配对。决策固定review、至少一检查开启，配置/检查和匹配审计不可改写；不保存用户名、密码、证件、原请求键或IP到该DTO。

业务检查在事务中FOR SHARE锁政策，配置启用更新等待已用旧政策的事务结束；启用完成后的新业务不能依据旧预览或前端声明绕过。拒绝先回滚业务，再保存历史政策版本的证据及幂等回执；记录时允许当前政策已变化，但必须匹配检查时的不可变修订，不伪装为新版本。全部关闭的检查跳过，不产生拒绝记录，也不表示实际验证通过。投注预览没有幂等键，每次观察可有独立拒绝记录；普通写入同键重放不重复证据。

### 彩种、玩法和规则

`games`

- `id`, `brand_id`, `code`, `name`, `status`, `model`, `timezone`
- `version`, `started_sequence`, `schedule_id`, `draw_source_set_id`

`game_schedules` 保存不可变的日历定义修订：`id`, `brand_id`, `game_id`, `revision`, `spec`, `created_by`, `created_at`；同一彩种的 revision 唯一。修改日历会新增修订并更新 `games.schedule_id`，不会改写旧修订。

`play_definitions`

- `id`, `brand_id`, `game_id`, `code`, `name`
- `rule_version_id`, `status`, `cancel_policy`, `limit_policy`

`rule_versions`

- `id`, `play_id`, `version_no`, `definition` JSONB
- `status`: draft/pending/approved/active/expired/rejected/rolled_back
- `effect_mode`: immediate/next_period
- `effective_at`, `effective_period_id`
- `created_by`, `reviewed_by`, `review_comment`

### 期次与开奖

同品牌、同彩种的下一期实际开放，以旧期业务终结为前提。旧期`settled`表示结算目标全部终结（含按既定规则排除的异常注单）；旧期取消则要求`period_cancellations.state=completed`，不能仅看期次取消状态。自动跳过的未开放期次没有注单、没有退款任务，也可视为终结。未来pending按计划开奖时间判定先后，不以创建序号阻塞较早期次；其他已经开始但未完成的期次仍阻止重叠。

门禁共享实现为`internal/periodgate`，从主库读取期次、取消任务和注单。在彩种行锁下检查：内部开期与Tick独占，投注预览和最终扣分共享；旧期修正的独占锁同样与投注串行化。门禁不改变历史状态或积分。pending因旧期未完成而延迟时，原投注窗口不变；窗口已过由Tick判定取消，不开期、不激活下期规则。现有SQL历史约束继续生效，此门禁不是人工直接SQL改写业务状态的授权。

`periods`

- `id`, `brand_id`, `game_id`, `period_no`, `sequence`
- `bet_start_at`, `bet_end_at`, `draw_at`
- `schedule_id`：创建期次时采用的不可变日历修订快照
- `status`: pending/betting/closed/waiting_draw/drawn/settling/settled/bet_cancelled/judged_cancelled
- `version`, `state_reason`, `created_at`
- unique `(brand_id, game_id, period_no)`

日历按 IANA timezone 中的民用时间展开为 UTC 期次，`period_no` 也使用 UTC instant。支持 daily 或 interval、weekday、显式 pause/holiday dates、holiday skip/normal policy 和投注窗口。展开范围最多 7 天且最多 10,000 个 slot。DST 不存在的本地时间跳过，重复时间取较早 UTC 实例。Interval 基线锚定本地午夜，busy window 在 `[start,end)` 内用窗口起点重新锚定；窗口不能跨午夜，结束时间必须早于 `24:00`。因窗口右边界不包含，结束设为 `23:59:59` 时该秒的 slot 已在窗口之外；`24:00` 当前不接受。日期列表是显式配置，不按国家推断节假日。

Period `sequence` 是期次创建顺序；`games.started_sequence` 只在实际成功开出投注窗口时递增。因此规则的“下期”绑定实际开期序号，不由预生成期次数决定。漏过完整投注窗口的 pending 期次转为 `judged_cancelled`，不会补开，也不会递增实际开期序号。pending 不接受投注，因此该自动转换不涉及订单退款。S5-a4 运营整期取消使用持久化任务，立即关闭期次，逐笔原来源退款；任务 completed 才代表处理全部完成。

`draw_sources`

- `id`, `brand_id`, `game_id`, `type`: api/dom/manual
- S4-c2 中仅保存不可变身份，同一彩种最多一个 manual 来源。不删除身份，不允许变更类型或所属品牌/彩种。

`draw_source_sets`：完整配置的不可变修订，含 id/brand_id/game_id/revision/sources JSONB/created_by/created_at；sources 含 id/name/type/priority/enabled/endpoint/selector/credential_ref。修改新增修订并更新 games.draw_source_set_id；移除来源只影响新修订，不删除历史。

`draw_results`

S5-b 的公开投影只读取各期当前 `periods.draw_result_id`，不直接公开此内部表或全部纠正历史。公开字段为结果编号、公开彩种/期次、regular/special/digits 数组、实际开奖时间和人工/外部分类；来源/人员/抓取/纠正链均不公开。取消期次保留当前号码但显式取消，没有结果保持 null。历史查询只显示已开始窗口，以主库时间判断；`0016` 增加品牌/彩种下按计划开奖时间排序的索引，不修改既有业务数据或迁移。

- `id`, `brand_id`, `game_id`, `period_id`, `source_id`
- S4-c2 实际字段：kind api/dom/manual、result JSONB、result_hash、drawn_at、created_at、created_by（系统为空）、corrected_from_id（人工在结算前覆盖外部结果时引用旧行）。只追加；当前结果由 periods.draw_result_id 决定，不根据创建时间猜测。
- JSONB 使用 regular/special/digits 三个数组；哈希基于规范化结果，非有序号码组排序，数字位置保留顺序。

`draw_attempt_batches`：id/brand_id/game_id/period_id/source_set_id/observed_period_version/status/attempts/created_at；status 为 accepted/no_data/failed/discarded。attempts 仅含 source_id/status/code，不保存密钥、原始响应或底层错误文本。配置替换或人工生效后的旧采集记 discarded，不改变当前结果。复合外键限制同品牌、彩种、期次。

periods 另含 draw_result_id、draw_claim_token/draw_claim_until 和 draw_next_poll_at。租约仅是采集元数据，不增加业务 version；drawn/settling/settled 必须存在结果指针。结果和采集证据不可改写或删除。已结算结果纠正及回溯属于 S5，不能改写旧行实现。

同一期只能有一个锁定结果；纠正时保留旧结果并建立 `corrected_from_id`。

### 注单与结算

`bet_orders`

- `id`, `brand_id`, `global_user_id`, `brand_member_id`
- `game_id`, `period_id`, `play_id`, `rule_version_id`
- `status`: placed/settled/won/lost/bet_cancelled/judged_cancelled/abnormal
- `selection_raw` JSONB, `selection_normalized` JSONB, `expanded_bets` JSONB
- `unit_points`, `combination_count`, `multiplier`, `total_points`
- `deduction_allocation` JSONB
- S5-a1 实际幂等字段为 `client_key`，unique `(brand_id, brand_member_id, client_key)`；同时保留 `version`、`placed_at`、`cancelled_at`、`cancel_reason`。
- `account_id`、`debit_entry_id`、`refund_entry_id` 通过品牌/账户复合外键关联账本。
- `definition_snapshot`、`definition_hash`、`policy_snapshot`、`brand_policy_version`、`game_policy_version` 保存每单确认时的规则/限额快照；开期引用只保留开期审计，不覆盖每单规则。
- 号码、复式展开、积分、来源分配、快照和身份不可改写或删除；取消必须关联原借记的全额原路退款账本，递增版本。当前可执行 placed → abnormal、placed/abnormal → bet_cancelled（异常仅允许运营取消）、单注或整期任务 placed/abnormal → judged_cancelled；结算仍待后续，`settled_at` 尚未落表。

`bet_order_exceptions`（0013）：id、brand_id、order_id（唯一）、order_version、marked_by、reason、created_at。人工标记只追加证据和递增注单版本，不退款、不派奖、不释放尚未退款的额度；原因非空且最多 500 UTF-8 字节。异常注单不进入普通结算或重试，不能解除异常或改写证据，但可以由具备取消权限的品牌运营全额原路退款。数据库触发器禁止单独无证据变状态或提交孤立证据；取消后原异常证据仍保留。

`bet_order_judgments`（0015）：id、brand_id、game_id、period_id、order_id（唯一）、order_version（判定后的版本）、cause（no_result/invalid_result）、draw_result_id（当时锁定结果引用，可空）、judged_by、reason、created_at。单人品牌运营判定只影响本单，不取消整期或其他注单；no_result 要求当时没有锁定结果，invalid_result 为运营明确认定的异常（可包含未被接纳的外部候选）。处于 settling/settled 的期次不能走此退款路径。单注记录不可改写/删除，必须与该单状态、版本、原因及原借记全额退款同事务提交；普通取消与整期退款竞争时只能退一次。整期任务不需要制造逐单人工判定记录，其证据是持久化任务及目标集合。

`brand_bet_policies`：brand_id、version、config JSONB、updated_at。Config 包含 min_bet_points（默认 1）、max_bet_points / max_period_points / max_user_period_points（默认 null，无上限）、user_cancel_allowed（默认 false）。积分为规范整数字符串。

`game_bet_policies`：brand_id、game_id、version、config JSONB、updated_at。各限额配置 mode 为 inherit / value / unlimited，value 带正整数 points；最小投注不允许 unlimited。取消开关 null 表示继承。彩种覆盖品牌默认，包含设置更大值或 unlimited，不是额外品牌硬上限；规则自身 limits 仍独立校验。品牌修改必须保持所有彩种继承后的最小投注不超过任何生效限额。

当前期次/用户期次限额统计实际尚未退款的下注积分；这不是提现或佣金的“有效流水”。全额退款释放额度。全局期次限额启用时，按期次加事务锁串行化准入和退款；未设置该限额时不持有全局期次互斥锁。单用户余额仍按账户串行记账。

`period_cancellations`（0014）：id、brand_id、game_id、period_id（品牌内唯一）、period_version（取消后的期次版本）、draw_result_id（保留原指针）、mode、cause、state、version（独立任务版本）、target_count、reason、created_by、created_at、completed_at、last_error_code。state 为 processing/failed/completed；投注取消仅 betting + operator_cancel，判定取消使用 no_result 或 invalid_result；有锁定结果时不能声称 no_result，drawn 仅允许 invalid_result。settling/settled 不接受此操作。

`period_cancellation_targets`：取消任务、品牌、期次、注单的不可变复合身份，state（pending/refunded/already_refunded/failed）、version、refund_entry_id、error_code。起始目标是该期全部 placed/abnormal，原本已取消的不纳入；开始后其他入口已退款的目标核实后标 already_refunded，不重复入账。`period_cancellation_failures` 追加保存失败目标、观察到的任务版本、错误码和时间，不能改写/删除。任务状态与提交后的目标进度有数据库延迟约束；目标未处理完不得宣称 completed。摘要计数从同一查询快照派生，任务版本与期次版本不可混用。

`settlements`

- `id`, `order_id`, `draw_result_id`, `status`
- `matched_rules` JSONB, `prize_tiers` JSONB
- `gross_points`, `capped_points`, `rounded_points`
- `error_code`, `retry_count`, `reversal_id`

S5-c1 的 `settlement_previews` 是正式结算之前的不可改写核算证据，不是上表 settlements，也不执行派奖或状态迁移。字段包含品牌/彩种/期次/注单/开奖结果、两种业务版本与状态快照、规则和开奖 hash、号码、outcome（won/lost/abnormal/excluded）、安全异常码、完整 calculation JSONB、创建者/原因/审计/时间。结果概要中的 `applied` 固定 false；`current` 根据当前注单/期次版本和结果指针派生，只表示观察依据未变，不表示有派奖资格或已经结算。

核算使用注单保存的规则、选号、复式展开、倍数和扣款分配，并与原借记账本逐桶快照比对。当前玩法后来生效的新赔率不参与旧单计算。异常快照的整注计算置空，不保存部分奖金；人工异常或已取消订单只写 excluded 核对证据、不运行普通引擎。预览不自动把实际订单改成异常，正式结算模块仍需接入该分类事务。数据库要求创建时订单/期次/结果快照与审核证据一致，不可删除或编辑；明细按页读取，旧依据变化后仅作为历史记录保留。

### 账户与账本

`point_accounts`

- `id`, `brand_id`, `brand_member_id`
- `version`, `updated_at`
- unique `(brand_id, brand_member_id)`

S3 实际余额只存于 `point_buckets(brand_id, account_id, source, state, points)`，3 种来源 × 4 种状态共 12 行。显示/可用/冻结/提现汇总与来源可用余额由这 12 行派生，不再持有多份冗余余额。每次记账先锁账户行，完整 12 桶必须存在，version 与追加账本版本一致。

`point_ledger_entries`

- `id`, `brand_id`, `brand_member_id`, `account_id`
- `entry_type`: recharge/bet/refund/win/gift/freeze/unfreeze/withdrawal/adjustment/reversal
- `source_type`, `source_id`, `reference_type`, `reference_id`
- `before_snapshot` JSONB, `delta_snapshot` JSONB, `after_snapshot` JSONB
- `reason_code`, `operator_id`, `idempotency_key`, `created_at`
- append-only；同一业务动作 unique 幂等键

实际字段为 member_id、entry_type、reference_type/reference_id、operation_key、version、request_hash、actor_type/actor_id、request_id、reason、reversal_of、source_allocation；每个 before/delta/after 快照为完整 12 桶十进制字符串 JSONB。`(brand_id, account_id, version)` 唯一，补偿引用强制同品牌同账户，原流水最多一笔全额补偿。账本和余额不能分开提交。

`point_balance_repairs`（S3-b）

- id、brand_id、account_id、member_id，复合外键与账户品牌/成员一致。
- version 为恢复后的账本版本；before_snapshot 包含实测原账户版本和存在的分项，after_snapshot 包含原账本重建的版本及完整矩阵。
- reason、actor_id、request_id、created_at；追加后不可修改/删除，品牌/成员/时间索引用于分页查询。
- 只修复余额投影，不改变经济账本；完整性无法证明时禁止重建。真实积分增减仍通过追加账本实现。

### 品牌钱包批量对账

0038的`point_reconciliation_jobs`保存品牌、状态、版本、固定target_count、创建人员/原因/时间、首次开始/完成时间、创建审计、最近失败指针、内部creation_xid及轮转调度时间。targets保存同品牌账户/成员、状态、尝试次数及下次检查时间；只允许创建事务捕获，不晚加或重开已检查目标。results保存不可变outcome/preview/checked_at/audit_log_id，failures和retries保存追加的失败尝试及人工恢复证据。结果completed表示全部检查完成，不表示全部钱包一致，也不修改余额或经济账本。完整字段、查询和故障边界见[批量对账交接](13-wallet-reconciliation.md)。

### 充值与提现

`recharge_orders`

- `id`, `brand_id`, `brand_member_id`, `points`, `status`
- `proof_reference`, `remark`, `created_by`, `confirmed_by`, `confirmed_at`

第一期由后台人工创建并确认；确认时写入充值积分和账本。实际字段为 member_id、account_id、points、state（pending/confirmed/cancelled）、proof_reference、remark、created_by、confirmed_by、version、created_at、confirmed_at、ledger_entry_id。金额不可通过确认操作修改；确认必须匹配 pending 版本并原子完成账本、余额和审计。S3-b 支持待确认单按原版本取消，仅改状态、递增版本并审计，不产生余额/流水；取消与确认竞争只有一个成功。当前没有支付或文件上传，凭证仅可选文本引用。

`withdrawal_orders`

- `id`, `brand_id`, `brand_member_id`, `points`
- `status`: reviewing/processing/paid/rejected/failed/cancelled
- `source_allocation` JSONB, `turnover_cutoff_at`
- `reject_reason`, `process_result`, `created_at`, `reviewed_at`, `completed_at`
- 同一品牌用户的 reviewing/processing 状态只能有一条，使用部分唯一索引或等价锁。

提现规则、内部资金状态机及HTTP申请/查询/运营处理与页面已经接入；正式流水资格器仍未配置，默认拒绝新申请且不占用积分。`brand_withdrawal_policies` 保存 brand_id/version/config/updated_at；config 为 enabled、min_points、max_points（null 无上限）、allowed_sources（充值/中奖/赠送的非空唯一列表）、review_mode（manual/automatic）、turnover_multiple（N）。初始 disabled、下限 1、上限 null、三来源、manual、N="1"。

0039新增`withdrawal_orders`，实际字段使用member_id/account_id/state；请求金额、原始来源分配、资格证据、政策快照、提交时间和reserve_version不可改写。reviewing/processing按品牌会员唯一；reserve_entry_id证明available→withdrawal，paid_entry_id证明仅消耗withdrawal，release_entry_id必须是原reserve的全额反向流水，不能换来源。取消/驳回/失败不删除原记录。`withdrawal_order_transitions`保存每次状态、版本、操作者、理由和审计，投影必须有连续完整历史；`withdrawal_operation_receipts`保存原始回执和请求摘要，相同键重放返回原提交/操作状态，而非后来状态，改变正文拒绝。

`withdrawal_turnover_cycles`仅在paid后更新，cutoff_at为该申请提交时间，cutoff_version为该申请占用积分的账本序号，不使用审核或出款时间，不清除投注事实。后续资格器同时得到上次成功截止时间/序号、当前锁定钱包版本及申请截止时间，避免并发事务时间重叠造成流水漏计或重复计。失败、取消和驳回不移动截止点。内部OrderService要求服务端资格适配器；默认nil安全拒绝且不占用积分。来源分配必须显式提供并精确合计，不默认为提现引入投注扣款优先级；正式申请分配/资格计算待业务口径确认后接入。启用但未接入的合规检查仍拒绝申请，测试资格适配器不能绕过它。automatic审核只推进到processing并记录系统审计，不自动标记paid或执行外部支付。

`game_withdrawal_policies` 按 brand_id/game_id 保存独立 version/config/updated_at，只覆盖 N；null 继承品牌，"0" 是明确配置零，不与继承混淆。有效值返回 source 和双方版本，来自已保存主库一致快照。N 为 0–1000000 的规范十进制字符串，最多六位小数，不带符号/指数/多余前导或末尾零；积分仍为 int64 整数字符串，不使用浮点。

`withdrawal_policy_revisions`：id、brand_id、game_id（品牌级 null）、version、config、changed_by、reason、created_at；范围/版本唯一（null 范围也唯一）。初始系统记录，后续必须有后台账号。历史不可修改/删除；数据库延迟约束禁止孤立下一版本或无对应历史修改当前配置，审计失败整体回滚。`0017` 为旧及新品牌/彩种初始化配置和历史，不改旧账本或已应用迁移。门槛基数、跨彩种 N 合并、N=0 语义仍待确认，不得把配置当资格结论。

### 代理、佣金和奖励

S6-e 实表为 `brand_agent_policies`、`agent_nodes`、`agent_config_revisions`。节点使用member_id/parent_id/depth作为下列设计字段的实际命名，path为UUID[]；同品牌成员唯一，身份/父级/路径不可改写，最多32级工程上限。配置存JSONB：品牌enabled/max_depth/ratio_cap/mode/cycle，节点ratio/nullable mode/status/can_create_children。比值为0–1精确规范字符串，最多6位小数；继承模式读取最近节点覆盖，周期仍品牌级。每次更新递增版本并保留实际操作者/理由/审计证明；不存在现金或积分佣金余额。初始disabled/比值上限0，没有自动开启财务工作流。

父/子/品牌配置更新先取得品牌代理政策独占锁，服务和数据库均拒绝新超限及会破坏既有下级的降限；不靠前端判断或自动缩放。用户只改自己直属active下级的比值/模式，所有祖先须active；can_create_children不是绕过后台晋升流程的授权。S6-f保存加入及新注单归属快照；用户晋升/重新挂接和财务计算仍待后续，不能用当前代理树为旧注单补造归属。

`join_codes`保存同品牌唯一24位随机大写十六进制编码、不可改写kind/owner_member_id/agent_id和创建人、status、starts_at/expires_at、递增version与时间；agent码须节点属于同会员，referral码不得挂代理。有效范围当前品牌，开始含/到期不含。`join_code_revisions`保存每版本生命周期、真实操作者/理由/审计，历史不可修改或删除；当前版本须有匹配审计历史，原编码不回收复用。编码管理不写积分、不生成佣金/奖励任务。

`agent_nodes`

- `id`, `brand_id`, `brand_member_id`, `parent_agent_id`, `level`, `path`
- `status`, `can_create_children`

`commission_rule_versions`

- `id`, `brand_id`, `mode`: loss/turnover
- `ratio`, `cycle`: weekly/monthly
- `game_scope`, `effective_at`, `status`, `version_no`

第一阶段对外名称使用“输赢/流水”；输赢模式实际只计算有效输钱注单。

`commission_records`

- `id`, `brand_id`, `agent_id`, `user_member_id`, `order_id`, `period_id`
- `rule_version_id`, `cycle_start`, `cycle_end`
- `base_points`, `ratio`, `calculated_points`, `adjusted_points`
- `status`: pending/settled/voided/adjusted
- 不允许最终金额小于 0；人工修正创建独立 adjustment 记录。

`reward_records` 与佣金记录类似，但必须区分奖励类型和触发来源。

### 审计与通知

`audit_logs`

- `id`, `brand_id`, `actor_type`, `actor_id`, `action`, `resource_type`, `resource_id`
- `before_json`, `after_json`, `reason`, `request_id`, `ip`, `user_agent`, `created_at`

`notifications`

- 当前站内信：`id`, `brand_id`, `member_id`, `event_id`, `event_type`, `template_key`, `template_version`, `content`与`payload` JSONB、`created_at`、`read_at`。content是落库时复制的双语模板源文案，与同品牌/key/version修订外键关联；迁移前v1消息保持null及固定旧文案，原字段不改写。
- `(brand_id,member_id)` 外键绑定品牌成员，`(event_id,member_id)` 唯一；内容与来源不可改写/删除，只允许首次填写已读时间。payload 仅含业务资源编号及规范整数字符串积分（入品牌通知为 null），不含人员、凭证、内部理由或支付证明。
- `notification_deliveries` 单独保存此消费者的 `event_id`, `brand_id`, `status` pending/sent/failed、`attempt_count`, `last_error`, `next_attempt_at`, `sent_at`；与 outbox 关联。不占用未来消息发布器的 `published_at`。
- 站内落库、消费者去重确认和 sent 状态同事务；失败回滚消息后持久化安全错误码/退避时间。失败重试必须由有品牌权限的运营人员填写理由，和审计同事务。
- S6-c 正额派奖和全额奖金冲正 outbox 引用原 calculation/job/ledger/correction，不复用下注金额；验证不可变目标/账本归属而非当前注单投影。旧消息保留、新代次独立记录，零额/失败不生成奖金消息。0025 不补造历史消息。
- `notification_templates` 按品牌/key保存独立version、content、updated_at；`notification_template_revisions`保留操作者、原因、审计与完整双语旧内容。更新必须对应同事务修订，初始v1为系统，随后版本不可覆盖；新品牌自动初始化八份默认模板及历史，不生成业务消息。复制模板与更新用行共享/独占锁串行，见 [模板合同](12-notification-templates.md)。
- 邮件/短信/Telegram、开奖受众/提现通知为后续功能，不能把当前站内 sent 状态解释成外部发送成功。

## 3. 必备索引

- 所有主业务表：`(brand_id, created_at)`。
- 期次：`(brand_id, game_id, status, bet_end_at)`。
- 注单：`(brand_id, brand_member_id, created_at)`、`(brand_id, period_id, status)`。
- 账本：`(brand_id, brand_member_id, created_at)`、`(reference_type, reference_id)`。
- 提现：`(brand_id, brand_member_id, status)`。
- 审计：`(brand_id, resource_type, resource_id, created_at)`。
- 报表大表按时间分区；是否按品牌分区由压测决定。

S6-d 当前查询直接读取已有事实，无独立假统计表：原placed_at划分注单cohort，最终奖额限定当前已完成代次和无待处理更正；ledger.created_at划分实际入账/冲正窗口。品牌时区只决定日分组，不改显式UTC时间范围。聚合用PostgreSQL numeric并返回字符串，不沿用单账户int64上限。响应内汇总/分组/计数/当前桶余额是一个语句快照；两个端点和历史月份实时统计不是不可变月结或全链对账证明。0026增加品牌时间索引与两组独立读取权限，不写资金或回填假统计。

## S5-c2 已实现的正式结算持久化

0022 新增 `brand_settlement_policies`（初始 null）及不可改写 `settlement_policy_history`。显式选择模式后才允许启动；历史政策通过品牌/版本复合键被任务引用。0023 追加派奖证据守卫，以整份 12 桶精确 delta、前后快照算术、账本上一版本/当前账户版本、实际余额桶和 points.prize 审计校验非零派奖；不完整 JSON/SQL NULL 一律拒绝。

`settlement_jobs` 保存期次、当前 draw_result_id、启动后期次版本、政策版本/模式和创建人，普通结算每期最多一个。`settlement_targets` 固定当时全部注单，pending/ready/paid/excluded/failed，已排除和已应用不得重开。`settlement_calculations` 保存逐单购买快照版本/hash、结果 hash、完整精确 Simulation、中奖标记/整数金额，禁止修改/删除；`settlement_failures` 保存失败阶段、观察到的任务版本和安全错误码，不写敏感数据库错误。

注单扩展 settlement_calculation_id、payout_entry_id、prize_points、settled_at；非结算状态这些字段为空/0。placed→won/lost 仅在 paying、期次结果匹配、有效计算及（非零时）中奖账本证据齐备时允许。账本只增加 winning.available、记录全部 12 桶前/后值；零额不建流水。人工异常与系统异常共用不可改写异常证据，source/system job_id/error_code 与 manual marked_by 互斥。

worker 一事务一个目标或阶段转换；锁序 game→独占 period→job→wallet→order，不批量持有多会员钱包。取消/人工异常先 period→job，再 wallet/order，排除未入账目标并增加任务版本。SQL 延迟约束要求任务/目标计数及终态和期次一起提交；无任务不能直接推进 settling，未入账目标不能提前 settled。原取消/判定证据守卫继续保留。结果纠正的版本代次和派奖冲正另行设计，不覆盖旧计算或绕过终态守卫。

## S5-c3 结果更正及结算代次

0024 把普通每期唯一 job 扩展为 `(brand_id,period_id,generation)` 唯一；generation1 的 lineage 为空，新代次必须关联 previous_job_id/correction_id。periods.current_settlement_job_id/current_correction_id 作为当前投影指针，迁移只回填元数据、不增加旧业务版本或改钱包。原 job/目标/计算保留，普通已应用终态不能重新打开；更正专用证据只允许当前注单 projection 在准确旧版本下 won/lost→placed，原 stake/debit 不动。

draw_corrections 保存旧/新结果、旧 job、启动后的期次版本、政策/模式、创建人/原因和阶段。draw_correction_targets 保存当时各注单的旧状态/版本、旧 calculation/prize/ledger，以及实际 reversal/reset 证据；终态不可改写。draw_correction_failures 保存失败观察版本/错误码/目标，可为空目标的发布失败亦留存。一个期次最多一个未完成更正；全部旧目标处理完才允许新 job、结果指针和更正 resettling 同事务提交，新 job completed 则更正 completed 与期次 settled 同事务。

冲正 worker 每目标独立事务：game共享→period独占→correction→wallet→order；发布阶段从头用 game独占→period→correction，避免共享锁升级。普通 worker/批准/重试检查 current 指针和活动更正，历史代次不会继续计算/派奖，迟到失败也不能写入新代次。SQL 保留原期次取消/判定证据限制，新增更正审计、完整 reversal、金额/来源、实际余额及 current 代次守卫；NULL 证据按拒绝处理。

只反向 winning.available 的原 prize，保留原 source_allocation/reversal_of/全桶前后值；余额不足冻结停止。模型结果、原实际开奖时间保留，旧结果和已计算金额不可重写；无 job 的 drawn 可只更正结果，不自动启用派奖。报表与后续代理/奖励模块必须以当前代次/补偿事实计算净值，不能把每个历史 paid 当新收入。
