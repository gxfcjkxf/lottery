# 佣金更正差额执行

0059把已就绪的佣金更正计划交给独立资金执行任务。每个代理只补发或追回计划差额，只移动佣金可用积分；原核算、原派发、原人工修正、旧批准与流水均保留。Go服务、平台worker、正式管理HTTP及PC/移动操作页面已接入，管理细节见[24号合同](24-commission-correction-management.md)。0060补偿历史通知与实际账本报表见[25号合同](25-commission-correction-observability.md)。

## 启用与授权

brand_commission_correction_policies是独立的资金更正开关。升级及新品牌均从enabled=false、version=1开始，没有历史任务或资金回填。原佣金派发开关打开不代表同意自动追回；管理员必须明确开启新开关，且原派发开关也开启、品牌未disabled，才能新建或推进资金任务。paused品牌可处理已有财务流程。关闭开关不改旧批准与已入账记录，也不阻止过期任务标记stale。

执行模式冻结计划的manual、automatic、mixed或none事实。manual和mixed先进入awaiting_approval，必须针对这个新计划、版本与核算代次取得新的单人批准，旧派发批准不能复用。automatic或none在明确启用资金开关后由系统登记授权；遇到周期暂停仍不能执行。

所有人工操作都需要commission.view品牌或平台查看权限，并需要普通管理员对应的独立品牌权限：

| 操作 | 权限 | 前置状态 |
| --- | --- | --- |
| 修改资金更正开关 | commission_correction_policy.write.brand | 提供当前政策版本及原因 |
| 批准新计划 | commission_correction.approve.brand | awaiting_approval |
| 继续余额不足的周期 | commission_correction.continue.brand | 当前任务paused且已批准 |
| 重试技术失败 | commission_correction.execute_retry.brand | 当前任务failed且已批准 |

准备阶段的commission_correction.retry.brand不授予资金重试、批准或继续权限。服务再次读取数据库当前管理员active和非超管状态，不信任缓存的普通身份。超级管理员仅按查看权读取。服务器引导品牌角色可获得四项新权限，自定义及平台角色不自动扩权。正式HTTP层执行会话、权限、查询审计及等待审计后再次授权，不允许从内部方法旁路接入。

## 状态及整周期暂停

| 状态 | 含义 |
| --- | --- |
| awaiting_approval | 等待新计划的明确人工批准 |
| applying | 每步执行一个代理目标，包括零差额见证 |
| completed | 全部目标完成，数量及正负金额一致 |
| paused | 佣金可用不足或继承当前周期暂停，等待显式继续 |
| failed | 当前步骤完整回滚，等待技术故障处理及显式重试 |
| stale | 原计划或核算代次已经变化，保留已执行资金历史 |

负差额大于该会员当前commission.available时，当前步不创建资金目标、不扣款、不改变净额头，而是写入COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT、计划目标编号和周期暂停记录。整个周期停止，不扣充值、中奖、赠送、人工冻结、系统冻结或提现占用，不自动解冻，也不形成负余额。

commission_correction_cycle_holds保存独立周期暂停，不能由充值、解冻、定时器或新开奖结果清除。再次更正可使原任务stale，但新任务仍继承暂停：自动任务以COMMISSION_CORRECTION_CYCLE_HELD停留paused；人工或混合任务先等待新批准，批准后仍paused。只有管理员对当前已批准任务显式continue，才解除当前周期暂停。继续后余额仍不足，下一真实执行步会再次暂停。技术retry不能代替continue。

## 差额与多次更正

commission_correction_balance_heads表示周期已经实际授予每个代理的净额，不是钱包余额。首次补偿使用原已paid目标的净额头；尚未支付的代理以零为基数。每个已执行目标原子推进新的financial_version和last_execution_target_id，后续计划冻结previous_correction_target_id及financial_version。

例如原实际授予10，新核算8，真实追回2后实际净额为8。再次核算9，下一差额为+1，而不是继续从原10追回1。若上一补偿只执行一部分便遇到再次更正，新计划按各代理已经实际执行到的净额分别计算；未执行部分不当作已入账。原来未获原派发、后来补发的代理，即使在新核算中不再出现，也必须参与追回计算。

原paid目标、实际更正净额头和新run的earning按代理合并，会员身份必须一致。实际历史资金目标、原账本、精确16桶变动及审计核对通过才可用于基数；历史任务后来stale不删除合法入账事实。来源损坏属于failed，不借用stale掩盖故障。原人工修正版本大于1的OPEN-117净额组合仍整期blocked，三个差额汇总为null，不选择覆盖或保留修正。

单代理前后金额为非负int64整数，差额可正、负、零。周期正差额、负差额绝对额及净差额采用numeric并以字符串读取。零差额完成实际目标和净额版本，但不创建零积分流水。

## 事务与资金见证

commission_correction_executions冻结plan、run、evidence_epoch、plan_version、模式、目标总数、正负总额及新批准。不可变execution_steps逐版本绑定状态、操作、目标、原因、人员、请求与审计。每个execution_target冻结计划目标、会员、前后净额、差额和基数版本；pending只能存在于正在执行的同一事务，成功后为applied。

每一步依次锁周期与任务、验证当前代次和开关、核对资金来源、锁钱包，再写apply步骤、目标、资金流水、目标审计、净额头和任务版本。资金entry_type为commission_correction，reference_type为commission_correction_target，operation_key为commission-correction:加目标编号；只变commission.available。流水保留完整16桶before、delta、after与正绝对额来源分配。差额不是原整笔的reversal_of，不能冒充全额冲正。

任何资金、目标审计、净额头或提交约束失败都会回滚整个步骤，不留下pending目标或半笔资金。独立失败记录只在观察到的任务版本仍未推进时写入；提交结果未知但后来已成功的版本不被旧失败覆盖。已提交的其他代理目标保留，显式重试不重复执行。延迟约束拒绝孤立任务、步骤、资金目标或流水；身份、金额和历史见证不可修改或删除。

钱包前后值与最新流水必须一致。正差额服从当前总余额封顶；负差额降低已有超限余额可以执行，但不得减为负值。无权限、跨品牌、旧版本、旧核算、旧计划或错误资金标签不能授权资金移动。

## 内部接入与升级

平台commission worker增加独立的执行循环，每秒检查一次、每次最多20步、单轮10秒上下文；ProcessCorrectionExecutions只允许1至100步。周期使用SKIP LOCKED，交叉锁NOWAIT，忙锁延后1秒而不标财务失败。暂停与失败不自动选择，过期任务的失效保存不依赖资金开关。

执行worker明确开启READ COMMITTED事务，认领周期行后，用新的SQL语句在该周期锁内重新读取执行与计划及当前资格，再决定注册或推进。单次联表快照中的执行为空不能作为注册授权：其他worker可能已经提交该计划的执行记录。新的语句快照语义见[PostgreSQL 17事务隔离文档](https://www.postgresql.org/docs/17/transaction-iso.html#XACT-READ-COMMITTED)。重新核验后不再符合条件时整体回滚，不写审计、执行步骤或资金流水，也不计作已提交步骤；不通过忽略唯一冲突或自动重试财务失败来掩盖竞态。服务端默认隔离级别不同也不能改变这一策略。

内部方法为CorrectionExecutionPolicyTx、UpdateCorrectionExecutionPolicyTx、CorrectionExecutionsTx、CorrectionExecutionTx、CorrectionExecutionTargetsTx、ApproveCorrectionExecutionTx、ContinueCorrectionExecutionTx及RetryCorrectionExecutionTx。写入需要当前版本、非空原因与真实管理员元数据。列表金额、总数和epoch为精确字符串，版本为安全整数，时间为UTC，空列表为[]。读取同时返回冻结target_count、实际applied_count及实际正负入账金额；cycle_hold_active表示现在的周期暂停，不是假称原任务创建时的历史状态。

正常部署当前版本须migrate至0060并协调API、worker和前端；0059及0060不改旧迁移校验和、不启用新开关、不迁移旧批准为新授权、不补造历史消息、不接入外部支付。管理HTTP已提供同会话原请求恢复与幂等回执，页面明确区分原派发、计划、当前执行和历史资金。0060补偿通知与报表取真实执行目标而非计划金额。该交付不代表整个佣金或项目完成，客户人工审核及生产验收继续保留。
