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

`brand_domains`

- `id`, `brand_id`, `domain`, `is_primary`, `status`
- 全局唯一 `domain`

`brand_members`

- `id`, `brand_id`, `global_user_id`
- `status`: normal/frozen/disabled/expired/cancelled
- `display_name`, `notes`, `profile_snapshot` JSONB
- `join_method`: domain/agent_code/referral_code/operator
- `join_domain`, `agent_id`, `referral_id`, `joined_at`, `created_by`
- `terms_accepted`, `accepted_at`, `privacy_policy_version`, `service_terms_version`；运营新增成员在本人首次确认前为 false/NULL，不可由运营代同意。
- unique `(brand_id, global_user_id)`

归属字段在加入时写快照；后续代理关系修改不能影响历史注单。

### 后台账号与权限

`admin_accounts`, `roles`, `permissions`, `admin_account_roles`, `role_permissions`, `admin_brand_scopes`

- 权限键格式：`resource.action.scope`
- 账号最终权限为角色权限并集去重。
- `admin_brand_scopes` 限制品牌范围；平台范围权限不自动授予写权限。
- 角色含 `brand_id`（平台角色为 NULL）、`status`、`version`、`is_bootstrap`，普通账号有 version。
- 品牌角色的 `(brand_id, code)` 唯一，brand_id/code 创建后不可修改；管理员必须先拥有对应品牌范围才可绑定角色，绑定期间不可移除该范围。权限必须在品牌作用域内计算，不能把其他品牌角色并集当作本品牌授权。

### 品牌配置

S3-b 已实现 `brand_point_policies`：brand_id 主键，version 与 max_balance_points/max_recharge_points/max_adjustment_points（nullable bigint，正值或不限）。现存品牌迁移初始化，新品牌数据库触发器初始化；版本独立于认证配置，修改在同事务内审计。

`config_versions`

- `id`, `scope_type`, `scope_id`, `config_key`, `config_value` JSONB
- `version`, `status`: draft/pending/active/expired/rejected
- `effective_at`, `effective_period_id`, `created_by`, `approved_by`

配置查找顺序：平台 → 品牌 → 彩种 → 玩法。最终读取结果必须带配置版本。

### 彩种、玩法和规则

`games`

- `id`, `brand_id`, `code`, `name`, `status`, `model`, `timezone`
- `version`, `started_sequence`, `schedule_id`

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

`periods`

- `id`, `brand_id`, `game_id`, `period_no`, `sequence`
- `bet_start_at`, `bet_end_at`, `draw_at`
- `schedule_id`：创建期次时采用的不可变日历修订快照
- `status`: pending/betting/closed/waiting_draw/drawn/settling/settled/bet_cancelled/judged_cancelled
- `version`, `state_reason`, `created_at`
- unique `(brand_id, game_id, period_no)`

日历按 IANA timezone 中的民用时间展开为 UTC 期次，`period_no` 也使用 UTC instant。支持 daily 或 interval、weekday、显式 pause/holiday dates、holiday skip/normal policy 和投注窗口。展开范围最多 7 天且最多 10,000 个 slot。DST 不存在的本地时间跳过，重复时间取较早 UTC 实例。Interval 基线锚定本地午夜，busy window 在 `[start,end)` 内用窗口起点重新锚定；窗口不能跨午夜，结束时间必须早于 `24:00`。因窗口右边界不包含，结束设为 `23:59:59` 时该秒的 slot 已在窗口之外；`24:00` 当前不接受。日期列表是显式配置，不按国家推断节假日。

Period `sequence` 是期次创建顺序；`games.started_sequence` 只在实际成功开出投注窗口时递增。因此规则的“下期”绑定实际开期序号，不由预生成期次数决定。漏过完整投注窗口的 pending 期次转为 `judged_cancelled`，不会补开，也不会递增实际开期序号。投注订单尚未接入，因此该转换当前不涉及订单退款。

`draw_sources`

- `id`, `brand_id`, `game_id`, `type`: api/dom/manual
- `priority`, `endpoint_config`, `credential_ref`, `enabled`

`draw_results`

- `id`, `brand_id`, `game_id`, `period_id`, `source_id`
- `result_json` JSONB, `result_hash`, `drawn_at`
- `status`: received/validated/abnormal/confirmed/locked/corrected
- `validation_json`, `raw_reference`, `created_by`

同一期只能有一个锁定结果；纠正时保留旧结果并建立 `corrected_from_id`。

### 注单与结算

`bet_orders`

- `id`, `brand_id`, `global_user_id`, `brand_member_id`
- `game_id`, `period_id`, `play_id`, `rule_version_id`
- `status`: placed/settled/won/lost/bet_cancelled/judged_cancelled/abnormal
- `selection_raw` JSONB, `selection_normalized` JSONB, `expanded_bets` JSONB
- `unit_points`, `combination_count`, `multiplier`, `total_points`
- `deduction_allocation` JSONB
- `idempotency_key`, `placed_at`, `cancelled_at`, `settled_at`
- unique `(brand_id, brand_member_id, idempotency_key)`

`settlements`

- `id`, `order_id`, `draw_result_id`, `status`
- `matched_rules` JSONB, `prize_tiers` JSONB
- `gross_points`, `capped_points`, `rounded_points`
- `error_code`, `retry_count`, `reversal_id`

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

### 代理、佣金和奖励

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

- `id`, `brand_id`, `brand_member_id`, `channel`, `template_key`, `payload` JSONB
- `status`: pending/sent/failed, `attempt_count`, `last_error`, `sent_at`

## 3. 必备索引

- 所有主业务表：`(brand_id, created_at)`。
- 期次：`(brand_id, game_id, status, bet_end_at)`。
- 注单：`(brand_id, brand_member_id, created_at)`、`(brand_id, period_id, status)`。
- 账本：`(brand_id, brand_member_id, created_at)`、`(reference_type, reference_id)`。
- 提现：`(brand_id, brand_member_id, status)`。
- 审计：`(brand_id, resource_type, resource_id, created_at)`。
- 报表大表按时间分区；是否按品牌分区由压测决定。
