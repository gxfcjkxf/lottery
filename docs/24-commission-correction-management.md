# 佣金更正管理接口与后台

佣金更正后台通过十二条正式管理接口连接0058差额计划和0059资金执行。页面将原支付、计划、执行任务、实际代理目标和原操作回执分开展示；计划ready不能当作批准或资金完成。用户端没有新资金写入入口，外部支付、补偿通知和补偿报表仍不属于这一管理交付。

## 接口与权限

以下路径均以/api/v1/admin为前缀，品牌通过X-Brand-ID指定，不增加品牌路径别名。查询直接读取主库，使用真实管理会话和已提交查询审计；未授权、查询或审计失败不返回数据。查询前、查询后及审计等待后重新核对身份与权限。

| 方法和路径 | 返回数据 | 额外写权限 |
| --- | --- | --- |
| GET /commission-correction-policy | 独立资金更正开关 | 无 |
| PUT /commission-correction-policy | 本次政策修改的原回执 | commission_correction_policy.write.brand |
| GET /commission-correction-plans | 计划分页 | 无 |
| GET /commission-correction-plans/{id} | 单个计划 | 无 |
| GET /commission-correction-plans/{id}/targets | 计划代理目标分页 | 无 |
| POST /commission-correction-plans/{id}/retry | 准备重试的原回执 | commission_correction.retry.brand |
| GET /commission-correction-executions | 执行任务分页 | 无 |
| GET /commission-correction-executions/{id} | 单个执行任务 | 无 |
| GET /commission-correction-executions/{id}/targets | 实际执行目标分页 | 无 |
| POST /commission-correction-executions/{id}/approve | 新批准的原回执 | commission_correction.approve.brand |
| POST /commission-correction-executions/{id}/continue | 显式解除周期暂停的原回执 | commission_correction.continue.brand |
| POST /commission-correction-executions/{id}/retry | 资金技术重试的原回执 | commission_correction.execute_retry.brand |

所有接口需要commission.view品牌或平台查看权。写入还需要表中的独立品牌权限、普通管理员身份以及当前active账号；超级管理员只读。品牌disabled可查看历史但不能写，paused可处理现存财务工作。计划重试不授予资金重试或批准；继续仅针对当前已批准的paused任务，不能替代failed的技术重试。

查询不接受正文。只有集合及目标列表接受limit与offset，默认20和0，上限100与1000000；重复值、前导零、负数、未知参数和空查询分隔符都拒绝。详情和政策不接受查询参数。写入要求同源application/json和完整UTF-8闭合对象：政策为version、enabled、reason，其他动作为version、reason；未知、重复、缺失、null必填字段、控制字符和超过500字节的原因均拒绝。

每个写入必须带Idempotency-Key和X-Commission-Correction-Actor-ID。后者是确认时的管理员编号，必须等于重新认证的当前账号；不能在重试时根据新cookie换人。幂等回执按品牌、管理员、操作、目标及原正文绑定、加密保存。重放先核对当前会话、权限和品牌状态，再释放原回执；同键不同正文返回409，SQL锁忙返回503且不缓存。新的业务写入在审计等待后再次验证会话，失效时整个事务回滚。

## 精确数据与实际状态

金额和汇总以十进制字符串返回，客户端用BigInt核对关系，不能转成JavaScript浮点数。计划的credit和debit分别累加正差额及负差额绝对额，两者可以同时非零；net等于credit减debit，也等于calculated减before。例如两个代理分别补100和追回100，净额为0，并不表示没有资金变化。

单代理前后值为非负int64，差额为有符号int64。周期汇总可超过int64，目标总数最多100000，实际已应用数不能超过冻结总数；completed还须完整计数及正负总额相等。epoch为非负int64字符串，版本为安全整数，时间保留纳秒。初始政策允许空audit_log_id，后续变更带真实审计编号。SDK拒绝非完整分页、重复编号、错误品牌或父记录、缺失审计、错误差额及非法状态组合，不以空数据冒充加载成功。

页面分别显示冻结计划总额和实际已应用金额。cycle_hold_active反映现在的整周期暂停，可以在旧stale任务上仍为true；不是任务创建时的快照。零差额有应用目标与财务版本，但没有零额流水。尚未应用的资金目标不预先持久化，awaiting_approval或paused时实际目标列表可以为空；计划目标仍可用于核对受益人和差额。

## 确认与未知请求恢复

后台“佣金更正”提供开关、计划、执行、详情和目标分页，PC与移动使用同一响应式网页。中文与英文跟随当前管理端语言，不改变机器状态、编号、请求正文或客户填写的原因。操作先填写原因，再核对品牌、原actor、目标、版本、完整正文和幂等键，明确勾选后才发送；加载中、权限不足、品牌禁用或版本未核对时不能提交。

网络失败、5xx或不合法成功响应视为结果未知。原请求只保存在同会话内存，冻结actor、品牌、操作、目标、普通JSON正文与幂等键，并锁定其他新资金意图。不使用localStorage保存操作，不自动重试，也不因为GET读到新状态就清除未知。离页返回需要重新查看并确认原请求，只能以原正文和原键重放。

409保留旧意图并停止重放。必须成功刷新政策、相关列表、详情和目标，再由管理员明确确认丢弃，才能按当前版本建立新意图。离页后迟到的409仍保留在原会话，不静默丢弃。logout或当前会话401清除敏感内存；旧会话迟到的401不能清掉新登录会话的请求。账号、品牌和权限变化隔离迟到响应，不将旧数据展示给新范围。

合法ACK确认的是原操作结果。随后资金worker可能完成、暂停或失败，页面通过独立GET读取现在的状态；原approve回执为applying时，当前任务完全可以已经paused或completed。ACK后GET失败仍保留回执，不重新变成未知、自动重发或开放旧详情版本继续操作。数据库中的原核算、派发、批准、人工修正及账本不改写。

## 部署与后续边界

本阶段沿用0059，无新迁移或资金回填。正常部署需要协调API与worker版本，但不自动开启原派发或新补偿开关。两个开关都必须明确开启才执行资金，关闭只是暂停后续工作，不撤销已有批准和积分。OPEN-117原人工修正后再核算的组合保持blocked及null差额，后台展示未决原因而不提供绕过按钮。

补偿历史通知、实际补偿报表、归属分析与封存日月归档继续实施。现有原佣金派发及人工修正报表不能代表新补偿的完整净变动。客户人工代码审核、真机和生产容量验收继续保留，浏览器专项不替代这些验收。
