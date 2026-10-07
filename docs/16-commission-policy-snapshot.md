# 佣金金融政策与投注快照

0047接入品牌金融政策、不可变历史、管理API及每笔新投注的私有金融快照。政策修改不入账、不创建周期批次；0048另行接入[周期核算](17-commission-cycles.md)，`automatic`仍只保存未来派发方式。审核、调整、积分派发和已派发更正回溯仍需实现。

## 已确认的结佣口径

注单按投注时间归属配置的周或月周期。边界结束后等待该周期所有注单最终完成再结佣，不把未完成注单当零额或移到下期。取消、判定取消、异常和无效等最终排除结果保留处理证据，不贡献基数。每个代理的精确差额佣金在周期内先累加，最后统一四舍五入为整数积分；不是逐注舍入。代理停用只限制代理操作，仍按各笔投注快照计佣。

内部`Aggregate([]ExactAmount)`对约分后的规范分数使用任意精度累加，返回精确总分数和一次half-up整数结果；结果超过int64拒绝。两笔3/10得到3/5及1积分，1/3加2/3得到1，排列不影响结果。0048在全量明细核算后按代理调用；结果不是已入账积分或派发授权。

彩票期次结清或取消退款后的下一期开放不等待佣金周期。完整周期批处理须按最终结算代次绑定证据，不能把分页列表尾或缓存事实当作结清/派发授权。

## 品牌金融政策

`brand_commission_policies`保存brand_id、递增version、config、created_at和updated_at。config严格包含enabled、calendar、payout_mode。安装时为`false/null/manual`；不擅自选择周期边界或开启金融流程。calendar为null只允许disabled；非空必须包含timezone、cycle、boundary_time、weekday、month_day、short_month。

周周期要求weekday为0至6，month_day为null，short_month为空；月周期要求weekday为null、month_day为1至31、short_month为last_day或skip。使用有效时区和严格HH:mm:ss本地边界，日历计算拒绝夏令时不存在/重复边界。启用金融政策还要求代理管理已启用且周期类型一致。修改代理周期或关闭代理管理前，必须先关闭金融政策，防止两个配置悄悄漂移；历史快照不受影响。

`commission_policy_revisions`保存每次配置、品牌/版本、changed_by、原因、audit_log_id和时间。初始禁用版本允许操作者/审计为空；后续修订必须匹配真实管理员、动作、品牌、配置、版本和修订编号的审计。当前版本必须有匹配历史，孤立的未来修订不能提交；记录禁止更新或删除。配置、修订和审计在同事务提交。

## 管理接口与权限

- GET `/api/v1/admin/commission-policy`读取当前品牌政策。
- PUT相同路径完整替换version、config、reason，要求Idempotency-Key。相同键和正文返回原回执，正文变化冲突；重放前重新核验当前会话及品牌权限。
- GET `/api/v1/admin/commission-policy/history`按版本降序读取历史，limit默认20、上限100，offset上限1000000。

读取使用commission_policy.view.brand；超级管理员须显式commission_policy.view.platform，只查看不写。修改要求commission_policy.write.brand，超级管理员和平台写权限不能代替品牌授权。管理页面提供中英配置与历史；写请求先冻结版本/正文/键，未知结果只用原意图重试，切换账号/品牌不能让迟到回调污染新视图。配置页面明确不提供已完成派发或虚构收益。

## 新投注的私有快照

`bet_orders.commission_rule_snapshot`为新增JSONB列。旧注单保留NULL，不从当前规则补造；旧attribution_snapshot及其commission_policy:null原样保留。新投注的字段由数据库覆盖客户端值并捕获，之后不可修改。

快照schema_version为1，保存brand_id、member_id、captured_at等于placed_at；financial_policy和agency_policy各保存revision_id、字符串version和完整config。agent_path按祖先到直属代理排序，每节点保存id、member_id、parent_id、depth、revision_id、字符串version、config和effective_mode。无代理路径保存空数组；停用节点仍保留，不能静默移除。

`Source.PolicySnapshotTx`必须在调用者事务内读取可信品牌和注单。它校验封闭形状、规范版本/UUID、购买时间、父子链、深度、比例上限/递减及同模式，再核验每个不可变修订的品牌/身份/版本/配置/创建时间，最后与原购买归属绑定。读取不按今天的比例重建；当前节点仅用于核验不可变身份/父级/深度，不用当前可变配置。

未知或跨品牌注单返回ErrNotFound；参数错误返回ErrInvalid；旧NULL、缺失修订或配置/归属/时间不匹配返回ErrPolicyEvidence，不降级为零额佣金或默认授权。字段不进入用户订单DTO、事件和公开归属接口。

## 事务与部署边界

政策写入与代理操作使用金融政策→品牌代理政策的锁顺序。投注先锁金融和代理政策及提现政策，再锁钱包；提交时数据库捕获相应修订。幂等投注重放仍返回旧订单，不按新配置重新捕获。财务读事实时沿用期次NOWAIT锁约束，见[最终注单事实](14-commission-facts.md)。

0047及0048为追加迁移，须与新后端协调升级，不修改已提交迁移摘要。隔离测试不自动升级原开发服务或生产。周期身份、分页及相关更正代次已有核算实现；幂等派发、审核、调整和金融补偿仍待完成，不能将政策或ready核算视为完整佣金闭环。
