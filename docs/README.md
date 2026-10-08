# Lottery 实现交接文档

> 用途：交给第三方开发人员或 AI 作为第一阶段实现依据。  
> 基线日期：2026-10-05  
> 状态：实现规格草案，业务未决项集中列在 `08-acceptance-and-open-items.md`。

## 文档优先级

1. 本目录下的实现交接文档：用于开发、测试和验收。
2. `requirements-draft.md`：完整讨论记录，用于追溯需求来源；如与交接文档冲突，以交接文档为准，并记录修订原因。
3. 客户牌照、授权文件和未获授权的保密材料：不进入仓库，不作为 AI 的输入。

## 文档索引

- [01-product-spec.md](01-product-spec.md)：产品范围、业务规则和状态机。
- [02-domain-model.md](02-domain-model.md)：领域对象、字段、约束和索引。
- [03-rule-engine.md](03-rule-engine.md)：彩种模型、玩法配置和结算规则引擎。
- [04-api-contract.md](04-api-contract.md)：用户端、管理端和异步任务接口约定。
- [05-ui-spec.md](05-ui-spec.md)：PWA 用户端、管理端和交互原型要求。
- [06-architecture.md](06-architecture.md)：技术架构、部署、高并发和运维。
- [07-security-risk-compliance.md](07-security-risk-compliance.md)：权限、审计、风控和合规接口边界。
- [08-acceptance-and-open-items.md](08-acceptance-and-open-items.md)：验收标准、测试场景和未决事项。
- [09-openapi-handover.md](09-openapi-handover.md)：已实现接口的 OpenAPI、接入限制、生成和 CI 契约检查。
- [09-implementation-backlog.md](09-implementation-backlog.md)：按依赖拆分的第一阶段开发任务。
- [10-replication-and-recovery.md](10-replication-and-recovery.md)：隔离复制、手动提升与备份恢复命令、证据和生产验收边界。
- [13-wallet-reconciliation.md](13-wallet-reconciliation.md)：固定品牌账户范围的只读钱包与资金业务双向关联检查、不可变观察、失败人工恢复和旧回执兼容。
- [14-commission-facts.md](14-commission-facts.md)：最终结算事实、计佣基数和内部锁约束；内部事实本身不授权佣金派发。
- [15-four-source-ledger.md](15-four-source-ledger.md)：四来源16桶、旧流水与幂等摘要兼容、原路退款、提现来源和协调升级要求。
- [16-commission-policy-snapshot.md](16-commission-policy-snapshot.md)：品牌金融政策、不可变修订、私有投注快照和已确认的周期收尾/汇总规则。
- [17-commission-cycles.md](17-commission-cycles.md)：真实周期发现、待结清、分页核算、代次历史、独立派发开关、单人审核及实际佣金来源入账；更正补偿见22与23号合同。
- [18-commission-adjustments.md](18-commission-adjustments.md)：已派发目标的独立人工差额修正、不可变证据、PC/移动确认与真实佣金入账通知；不解锁结果更正待处理任务。
- [19-commission-posting-reports.md](19-commission-posting-reports.md)：实际佣金账本的入账时间统计、品牌/代理/会员/周期筛选、精确净变动及完整CSV审计导出；不是未派发收益或封存日月报。
- [20-manual-reward-orders.md](20-manual-reward-orders.md)：人工奖励订单、赠送来源入账与完整撤销、真实不足待处理、显式继续、双语后台、双端请求恢复与历史站内通知。
- [21-reward-reports.md](21-reward-reports.md)：实际奖励入账与订单创建队列的当前状态报表、独立查看和导出权限、完整CSV验证及工作台当前状态计数。
- [22-commission-correction-plans.md](22-commission-correction-plans.md)：开奖结果更正后的真实佣金差额计划、来源守卫及不可变分页；计划就绪不代表批准或实际补偿。
- [23-commission-correction-execution.md](23-commission-correction-execution.md)：独立默认关闭的资金更正开关、实际差额执行、整周期暂停与显式继续、多次更正净额基数。
- [24-commission-correction-management.md](24-commission-correction-management.md)：十二条正式更正管理接口、独立权限与当前授权复核、PC/移动双语操作、原回执及同会话未知请求恢复；通知和报表另见25号合同。
- [25-commission-correction-observability.md](25-commission-correction-observability.md)：真实非零差额的原子事件、不可变历史站内消息、实际补发/追回报表、22列CSV版本2及无旧消息回填的协调升级。
- [26-report-archive-core.md](26-report-archive-core.md)：日/月不可变快照与追加版本、单一统计观察、四条管理接口、PC/移动原请求恢复、独立原JSON下载及后续自动任务边界。
- [27-automatic-report-archive-core.md](27-automatic-report-archive-core.md)：默认关闭的日/月配置与任务、保存起点及原范围、自动发现/执行、人工失败重试和系统来源。
- [28-report-archive-task-management.md](28-report-archive-task-management.md)：四条配置只读/任务管理接口、授权与审计、原pending回执恢复及双端任务页面。
- [29-report-archive-activation.md](29-report-archive-activation.md)：首次启用从品牌当前日/月开始的正式PUT、数据库时钟与保存起点、双端配置编辑及原回执恢复。

## 第一阶段实现顺序

1. 工程骨架、环境配置、数据库迁移、品牌上下文和审计基础设施。
2. 全局身份、品牌成员、后台账号、角色权限和会话。
3. 积分账户、账本、人工充值、冻结/解冻和并发控制。
4. 彩种、玩法版本、期次生成、品牌暂停和投注限额。
5. 选号、确认、下注、取消、幂等和注单查询。
6. 外部开奖适配器伪实现、人工开奖、结果校验和结果锁定。
7. 中奖判断、结算、派奖、异常注单和结果纠正回溯。
8. 提现申请、审核、内部出款模拟、原路退回和流水周期。
9. 代理关系、佣金/奖励、周结/月结和报表。
10. UI 原型还原、压力测试、安全测试、部署和验收。

## 通用实现约定

- 所有时间使用 UTC 存储，接口返回 ISO-8601；品牌配置时区只影响展示和期次计算。
- 所有业务主键使用 UUIDv7 或等价的可排序唯一 ID；不得使用用户可猜测的连续业务编号作为 API 主键。
- 积分使用有符号 64 位整数存储，业务层禁止浮点计算；第一阶段不支持小数积分。
- 金额和积分在 API 中使用字符串或整数传输，禁止 JavaScript 浮点数参与账本计算。
- 写操作必须支持幂等键；账本、投注、提现、结算、佣金发放不得依赖前端防重复。
- 所有配置、规则、状态变化、人工操作和反向流水都必须可审计。
- 任何“删除”均为逻辑失效或冲正，不物理删除已产生业务影响的记录。
