# 佣金结果更正补偿计划

0058为已有真实佣金入账、随后开奖结果更正的周期准备不可变差额计划。计划冻结原入账净额、新核算结果、代理与会员身份及来源见证，并逐页验证。准备本身不批准补偿、不增加或扣减积分、不生成入账通知，也不解除原派发的blocked。0059在独立默认关闭的资金开关下追加实际补发、追回和不足显式继续，见[差额执行合同](23-commission-correction-execution.md)；正式接口/后台与0060通知/报表分别见[24号](24-commission-correction-management.md)和[25号合同](25-commission-correction-observability.md)。

## 计划来源和金额

原派发必须因证据变化进入blocked，错误码为COMMISSION_PAYMENT_CORRECTION_REQUIRED，且至少有一笔真实正额佣金账本。更正后的周期必须完成新一代核算，所有注单已有最终结果；当前run及证据计数必须匹配。不能把旧ready读取或客户端金额当成新授权。

每个代理的之前积分先取该周期已经真实执行的更正净额头；尚未执行更正时，取原paid目标的净额头；两者均不存在时才以零为基数。新计算积分取自新run的不可变earning。三类来源按代理合并：原已发代理及后来补发的代理不能因新核算不再出现而丢失。计划目标冻结previous_correction_target_id和financial_version，已部分执行的旧任务失效后也使用实际已入账净额，不把未执行差额当作已经发放。代理对应会员必须一致，不能把旧款迁移给其他会员。

差额为新计算积分减之前积分。正差额计入credit_points，负差额的绝对额计入debit_points；net_points等于credit_points减debit_points，也等于calculated_points减before_points。单代理前后值保持非负int64整数，差额可为零或有符号整数；周期汇总采用numeric和精确字符串，不经浮点或int64截断。零差额仍可保留计划见证，不产生零积分流水。

这里的之前积分是已授予的业务净额，不是当前commission.available。代理可能已经投注、提现、冻结或另有积分入账；这些钱包动作不能替代本周期的入账见证。计划准备不读取或占用其可用余额，因此计划就绪不证明追回余额充足。

新计划接受经完整证据验证的人工修正净额头，并以新run核算额减当前实际授予净额计算差额；OPEN-117已确认新核算覆盖人工修正净额。原人工修正事实不变。迁移前已保存为`COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED`的blocked计划仍保持原状态、null金额及零目标；只有显式调用既有计划retry流程并通过来源、证据、精确冻结计数及开关守卫，才可在新版本重新准备，细则见[39号合同](39-commission-manual-recalculation-policy.md)。

## 模式和状态

计划保留原派发与新run的模式并集。none不增加审核约束，manual与automatic同时存在或任一原模式为mixed时，保留mixed。原人工模式不会因为新核算为none而变成自动模式；旧审核不授权新差额。后续执行阶段必须为人工或混合计划取得新的明确批准。

| 状态 | 含义 |
| --- | --- |
| planning | 按代理编号每页最多100条准备来源和差额 |
| ready | 全部代理、前后原额和差额汇总一致，仅计划完成 |
| blocked | 保留历史未决政策计划；不得由普通批准、自动恢复或新计划覆盖 |
| failed | 准备步骤发生技术或来源错误，保留已提交页，等待显式重试 |
| stale | 新核算或证据已经变化，保留原计划、目标与步骤，不继续使用 |

新计划按cycle/run唯一；同一周期只允许一条非stale计划。证据变化后旧计划和金额不可覆盖，新的有效run可生成新计划。分页末尾不是就绪证明；ready还要核对完整目标数、前后金额及正负差额总计。超过100000代理目标停止，不截断或冒充完整计划。

## 守卫和并发

commission_correction_plans保存当前准备状态与冻结金额；commission_correction_plan_targets保存每个代理的原目标、新earning、原净额头版本及前后差额；commission_correction_plan_steps保存每个版本的状态、游标、数量、原因、操作人和审计。所有目标和步骤不可修改或删除；计划只允许有对应不可变步骤的版本推进与内部调度时间更新。

SQL核对原paid目标、原ledger、人工审计、精确佣金可用来源、完整16桶变动、真实受益人、当前核算及每页来源；延迟约束拒绝孤立计划、步骤或目标。所有新增函数固定查找路径，临时表不能替代正式来源。0058拒绝使用佣金更正entry_type或计划/更正目标reference_type伪造入账；0059仅允许当前已批准执行任务在同事务创建的真实目标、精确差额及审计授权对应流水，计划本身始终不能入账。失败回滚当前页，不删已提交历史。

工作器采用周期锁、SKIP LOCKED及交叉锁NOWAIT，每步独立提交，每次最多100步。原派发工作器先保留旧入账并进入blocked；独立准备循环随后每秒检查、最多20步，不占用原核算或发现循环。锁忙延后而不标成财务失败。证据失效可在运行开关关闭或品牌禁用时保留为stale；新准备或重试要求品牌未禁用且派发开关已启用。暂停品牌可继续准备财务计划。

认领事务显式使用READ COMMITTED：周期行锁只保护周期，不使LEFT JOIN计划自动刷新；取得锁后须在新语句快照中重查该周期候选，再创建/推进计划。候选已失效则回滚且不增加已提交步数，不吞掉唯一约束错误。并发不会重复准备计划或创建审计，重复处理ready计划不改变历史；仍不授权资金执行。

## 内部读取和重试

Go服务的CorrectionPlansTx、CorrectionPlanTx、CorrectionPlanTargetsTx及RetryCorrectionPlanTx已由正式管理HTTP及后台接入，见[管理合同](24-commission-correction-management.md)。读取投影不暴露账户编号、原始规则、钱包桶或worker游标；总数和金额保持字符串，时间为UTC。HTTP层执行真实管理授权和查询审计，不能绕过该层把内部方法当成已认证的公开接口。

准备失败的重试要求commission.view及普通品牌管理员的commission_correction.retry.brand，提供当前版本和原因。超级管理员不能写，数据库当前账号状态及超管身份另行核对。普通技术failed按原规则显式重试。历史`COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED` blocked计划可由其既有retry流程在符合[39号合同](39-commission-manual-recalculation-policy.md)的全部条件时，追加新版本并重新准备；不得批准或记账，也不解除原派发blocked。

## 升级和后续执行

先正常升级，再协调API、worker与管理端。0073仅替换函数；不回填历史资金或事件，不修改原核算、派发、人工修正或账本，也不自动重试历史计划。两个资金开关保持原有状态，新品牌默认关闭，数据不会自动恢复；人工修正后重算的来源和显式历史retry守卫见[39号合同](39-commission-manual-recalculation-policy.md)。

0059已接入单人批准、独立执行任务、真实commission.available差额账本、已执行净额基数及多次更正链路。追回不足暂停整个周期，周期门闩跨新核算保留，运营处理后显式继续；不能扣其他来源、自动解冻、形成负余额或余额补足后自动恢复。正式管理接口、同会话未知请求恢复和双端页面、0060补偿历史通知及实际入账报表已接入；计划ready始终不能替代资金执行完成。
