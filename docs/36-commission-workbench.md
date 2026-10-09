# 工作台佣金任务汇总

运营工作台的commissions区显示所选品牌当前保存的发现、核算、派发和更正任务状态。它用于定位处理队列，不汇总资金，不表示今日计佣、已入账额、钱包余额、欠付金额或执行授权。原派发blocked可能在更正完成后长期保留，不能据这个数量认定仍有未解决资金。

## 查看权限和数据观察

沿用 `GET /api/v1/admin/workbench`、管理会话和X-Brand-ID，不新增路由、权限、迁移或金融开关。佣金区独立要求commission.view.brand或显式platform授权；wallet、report_commission及超级管理员身份不推导权限。只有commission查看权也可以读取工作台，其余区块按自身权限保持不可用。

有权限返回ready及完整19字段对象；无权限返回forbidden/null，并且不生成访问佣金表的SQL片段。授权来源缺失、超时、查询或审计失败时整体503，不填虚假零值或返回部分快照。查询与其他已授权区块共用一条主库SELECT、同一statement_timestamp观察；响应前重新核验会话/权限并提交审计。它不是持续实时锁定的队列，也不保证读取后状态不再变化。

旧快照的not_implemented/null保留可读兼容，仅表示旧版本没有接入；新版服务不生成这种正常状态。新版客户端不能将旧空值变成0，API/前端应协调发布。工作台不保存资金回执，也不重写旧审计或归档。

## 计数映射

全部字段都是规范非负十进制整数字符串，按任务计数，不按目标、受益人或流水条数。各来源先独立汇总，避免一任务有多个目标时重复相乘。时间范围为全部当前已保存任务，不使用工作台today窗口过滤。

| 来源 | 字段 | 保存状态 |
| --- | --- | --- |
| commission_discovery | discovery_pending_count、discovery_failed_count | pending、failed；pending包含未来到期等待 |
| commission_cycles | cycle_processing_count | enumerating、calculating、summarizing |
| commission_cycles | cycle_waiting_count、cycle_failed_count | waiting、failed |
| commission_cycles及其current_run | cycle_ready_count | 周期ready，当前run为ready且evidence_epoch相同 |
| commission_cycles及其current_run | cycle_stale_count | 周期ready，但当前run缺失、状态或epoch不满足上项；与ready数量互斥 |
| commission_payments | payment_awaiting_approval_count、payment_processing_count、payment_blocked_count、payment_failed_count | awaiting_approval、paying、blocked、failed |
| commission_correction_plans | plan_processing_count、plan_ready_count、plan_blocked_count、plan_failed_count | planning、ready、blocked、failed |
| commission_correction_executions | execution_awaiting_approval_count、execution_processing_count、execution_paused_count、execution_failed_count | awaiting_approval、applying、paused、failed |

cycle_ready_count只是当前状态和epoch观察，不替代完整金融来源复核，更不代表可立即派发。计划ready也只是准备完成，必须另查当前证据、开关和批准。paused是等待显式运营处理，不等于技术failed；历史blocked计划可能保留原未决政策错误码，并按[39号合同](39-commission-manual-recalculation-policy.md)显式处理。paid/completed/stale等历史终态不计为待处理，历史记录仍可在各管理页面查询。

## 管理端和验收

佣金卡片中英文显示19项，解释当前队列与资金的区别；有commission查看权才可打开“佣金和奖励”。没有新增批准、继续、派发按钮或自动写请求。品牌、账号、权限及读取代次变化后清除旧快照，迟到响应不能泄露旧品牌数据；失败刷新不保留旧值冒充最新数据。

验收使用真实投注/周期/审核/派发/结果更正及资金执行建立状态，验证多目标任务只计一次、证据过期ready移入stale、暂停与技术失败分别显示、更正完成后旧派发blocked仍保留。只读查询前后钱包、账本、事件及原业务证据保持。无权限时即使来源表不可用，也不得访问该表；有权限则失败关闭。HTTP再验证独立权限、会话等待与审计失败，PC/360px真实浏览器验证真实数值、双语、导航和失败清理。

该汇总不实现跨周期佣金应付净额分析；实际积分入账继续使用佣金业务账本报表及原金融流程。人工修正后重新核算规则的实现状态见39号合同，客户生产审核另行完成。
