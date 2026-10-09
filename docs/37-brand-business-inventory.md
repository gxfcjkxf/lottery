# 品牌资金业务引用检查

品牌资金业务引用检查从当前品牌的资金业务记录出发检查结构关联，不以存在钱包账户为前提。它补充已有逐账户钱包与业务对账，识别缺失账户、会员、上级业务记录及实际资金动作必需的账本引用；不检查完整账本金额链，不自动补账、冻结或修复资金，也不把数据库约束正常等同于全部财务正常。

## 接口与观察边界

`GET /api/v1/admin/reconciliations/business-inventory`已注册，后台会话与X-Brand-ID必需；只接受无正文、无查询串的请求。授权沿用明确wallet.view.brand或wallet.view.platform，不按超级管理员身份推导。它仅观察，不需要wallet.reconcile写权限。

一次主库语句固定所选品牌来源、引用和观察时间。完整来源行超过100000或查询超时则失败，不截断来源后宣称正常；缺表、缺函数或存储失败不能返回假零。查询和权限复核、审计提交之后才释放响应。旧批量任务、观察、检查范围及原幂等回执不变，不为它们补造品牌检查结论。

API总上下文最多5秒，数据查询最多2秒；失败使用保存点回滚查询，再尝试提交read_failed审计。审计等待后仍检查当前会话和权限，失效不释放结果。失败审计无原SQL或内部错误；成功审计只保存范围、时间、计数及摘要。主要错误为401 AUTH_SESSION_REVOKED、403 PERMISSION_DENIED、404 BUSINESS_INVENTORY_NOT_FOUND、413 BUSINESS_INVENTORY_TOO_LARGE及503 BUSINESS_INVENTORY_UNAVAILABLE。

响应为闭合对象：brand_id、snapshot_at、schema_version（固定1）、source_row_count、reference_count、issue_count、issues_truncated、consistent、fingerprint、coverage、issues。数量为规范非负十进制字符串；coverage按source_table排序，各项恰好source_table、source_row_count、reference_count、issue_count。每条问题恰好source_table、source_id、code、reference_key、parent_table，不公开缺失父记录的品牌/身份资料或业务原文。

code为MISSING_PARENT_REFERENCE或MISSING_REQUIRED_LEDGER_REFERENCE。源记录总数不是投注数或资金笔数；一条记录可以有多个引用及问题。issues稳定排序并最多返回100项，总问题数完整统计，超出时明确issues_truncated；没有问题的定义仅限本版声明的结构引用。fingerprint绑定品牌、版本、全部来源和完整问题，包括未展示的尾部，不是修复令牌。

## 覆盖规则

覆盖固定列出的已实现资金业务表及其保存的外键引用，直接读取品牌源记录，不通过账户、会员或父记录的内连接过滤源。复合品牌外键须核对全部列；合法可空引用不因为NULL而报告缺失。约束定义固定于迁移生成的检查代码，后续损坏或删除约束不能让检查范围缩小。不存在父行、引用其他品牌或错误会员/账户组合均记录问题；原合法待处理及零额目标不得被当作缺账。

0072校验完整来源主键、外键数量、源/父表、品牌归属、顺序列元组及本地schema，再生成STABLE/INVOKER函数。外键被等数量替换也拒绝迁移，不以数量相同推断结构相同。函数固定pg_catalog搜索路径并明确限定业务schema，使用原生列比较及集合查询，运行时不按目录重建范围；复合来源标识按主键列顺序以`/`连接。未来增加业务表或改变引用需新迁移和契约版本，不静默扩展v1。

| 组 | 来源表 |
| --- | --- |
| 钱包 | point_accounts、point_buckets、point_ledger_entries |
| 充值 | recharge_orders |
| 注单 | bet_orders、bet_order_exceptions、bet_order_judgments |
| 整期取消 | period_cancellations、period_cancellation_targets |
| 奖金结算 | settlement_jobs、settlement_calculations、settlement_targets、settlement_failures |
| 开奖更正 | draw_corrections、draw_correction_targets、draw_correction_failures |
| 提现 | withdrawal_orders、withdrawal_order_transitions、withdrawal_turnover_cycles、withdrawal_operation_receipts |
| 佣金核算与派发 | commission_cycles、commission_cycle_targets、commission_runs、commission_cycle_steps、commission_calculations、commission_allocations、commission_earnings、commission_payments、commission_payment_targets |
| 佣金人工修正 | commission_adjustment_heads、commission_adjustments |
| 佣金更正 | commission_correction_plans、commission_correction_plan_steps、commission_correction_plan_targets、commission_correction_executions、commission_correction_execution_targets、commission_correction_execution_steps、commission_correction_balance_heads、commission_correction_cycle_holds |
| 奖励 | reward_orders、reward_order_actions |

实际资金动作另核对必需账本指针：投注借记、已取消退款、已确认充值、提现预留/已成功出款/已释放、奖励发放/已撤销、非零已派佣金、非零人工差额及非零已执行更正、非零历史已派奖及已冲正奖金。缺失指针明确报告；这只是所需引用存在性，不代替逐账户金额、审计及来源分配核对。

## 管理端与验收

“批量对账”页面增加独立的“品牌业务引用检查”，用户明确加载，不自动执行任何资金或修复动作。中英显示覆盖、完整数量、截断说明及问题；没有钱包、金额和父记录隐私投影。账号、品牌、权限或读取代次变化时清理旧结果并丢弃迟到响应；失败刷新不保留旧值冒充最新结果。

验收须由正常业务建立真实引用，再只在独立测试schema注入缺父行、跨品牌引用及必需指针缺失，核对源记录仍被发现。合法待处理/零额不得误报；100项之外问题纳入数量及摘要。直接READ ONLY查询、权限拒绝、存储/审计失败、会话等待以及双端显示均需验收，检查前后账户、余额、账本、业务记录及事件保持。

本检查是当前观察，不新增不可变任务或归档；连续请求及分页外页面可能观察到后续状态，不把它作为全品牌财务结账。生产容量、客户人工审核及未实现业务仍独立验收。

升级先正常migrate至0072，再协调API/worker与前端；启动检查不执行自动迁移，旧检查和资金不改写。源码级browserfixture仅在明确owned合成库注入一条没有真实奖励资金动作的孤立子记录，普通构建不含它。专项CI沿用独立对账库，真实原任务及品牌检查均零重试，前后通过独立只读SHA-256比较全部41来源及outbox；不是对真实客户数据注入差错。
