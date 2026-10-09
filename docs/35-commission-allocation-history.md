# 佣金核算收益和分配明细

按明确运行代次查询整周期代理收益及逐注单差额分配，供运营追溯核算来源。旧代次在结果更正后仍可读取，不替换为当前run，不从今天的代理配置重建。核算值不表示已派发、可用余额或最终资金授权；实际入账使用[19号报表](19-commission-posting-reports.md)，人工修正后重新核算及更正资金边界见[39号合同](39-commission-manual-recalculation-policy.md)。

## 接口和范围

| 方法 | 路径 | 返回 |
| --- | --- | --- |
| GET | /api/v1/admin/commission-cycles/{id}/runs/{runID}/earnings | 此run的每代理整周期精确合计和已舍入整数 |
| GET | /api/v1/admin/commission-cycles/{id}/runs/{runID}/allocations | 此run的逐注单受益人差额分配，不舍入 |

使用现有commission.view.brand或显式platform查看权、真实管理会话及X-Brand-ID。超级管理员不跳过授权。两条接口走主库，在授权事务中读取、重新验证会话/权限并提交查询审计后返回；审计失败不释放数据。没有新权限、迁移、派发操作、从库路由或钱包写入。

limit默认20，范围1–100；offset默认0，最多1000000，接受规范非负整数字面值。allocations另接受agent_id、order_id，只过滤该代次真实保存的分配；未知或外品牌资源404，同品牌但无匹配分配200空数组。earnings不接受这些过滤参数。重复、空、未知参数、非规范UUID、ForceQuery和GET正文400。周期或run不存在、run不属于周期/品牌均404，不能退回当前run。

earnings响应恰为brand_id、cycle_id、run_id、items、total_count、limit、offset，items沿用CommissionCycleEarning结构。原仅查询current-run的`/{id}/earnings`接口和历史回执保持不变。排序为created_at降序、id降序，时间精度须保留到服务端纳秒字段，不能仅凭JavaScript毫秒比较判定顺序。

allocations响应另回显可空agent_id和order_id；items按order_id、agent_id升序，total_count为完整过滤范围而非本页数量。每项只有：

- brand_id、cycle_id、run_id、calculation_id、order_id、agent_id。
- member_id为受益会员，bettor_member_id为投注会员，不能混用。
- base_points为该代次保存的有效计佣基数；mode为当时loss/turnover。
- agent_ratio、downstream_ratio、difference_ratio为保存路径上本节点比例、该投注路径的下游节点比例和差额，末节点下游为0。不是任意当前子代理的比例。
- exact_amount为约分的非负numerator/denominator；created_at为原计算记录时刻。

不返回完整金融快照、代理路径、账号ID、私有修订或worker游标。审计动作commission.cycle.run_earnings/run_allocations保存已校验的cycle/run、分页和筛选，不保存原始URL或任意正文。

## 精确分配与取整

差额比例=本节点比例−该投注路径下游比例，分配精确值=保存基数×差额比例。比例为0–1、最多六位规范小数；分数约分后分母不超过1000000且整除1000000。积分和计数均为规范十进制字符串，任意精度分子不可转换浮点。

例如共同基数1、上级0.3、下级0.1，该单分别分配1/5和1/10；不分别取整，也不把每个祖先的0.3再次计入。两个直属基数1、比例0.3的投注，各3/10，代理周期合计3/5，最终1积分。取消、异常等无分配的计算可在原calculations接口查到，不能为明细生成虚构零记录。真实已保存的零基数分配保留0/1。

读取用原计算快照和原注单快照核对，验证受益身份、当时模式/比例及分配精确值；缺失或损坏409，整页失败，不部分释放或以当前配置补算。读取不会重新判断当前开奖结果，也不改变原代次，因此后来结果更正不使历史证据消失。calculating/summarizing/abandoned代次只显示已保存的记录，不能把其分页尾或已存汇总当作整个周期完成证明；是否完成须查看run状态和周期证据。

## 管理页面和验收

现有佣金周期详情保留当前收益区。在代次列表明确选中run后，分别提供历史整周期收益及未舍入分配区、独立分页及代理/注单筛选，中英说明区分精确合计、舍入积分、入账和钱包。切换run、品牌、账号、权限、筛选或会话时清理旧数据，迟到成功或401不能污染新范围；不发起财务写请求。

验收须用真实多层代理投注/结算/核算，证明保存比例不受后来改比例/停用影响，分页无丢失或重复，同一注单多人份额不是重复下注。更正后分别读取旧/新run，旧收益与分配保持，新零基数如实显示；直接只读事务读取前后资金、审计和事件摘要保持。HTTP另验证授权、审计失败及严格参数，客户端核验上下文、规范比例、分数算术及排序；PC/360px真实浏览器前后财务证据保持。

这两条查询不是跨周期的应付额、已付净额或欠付分析，也不改变已封存日/月档案。后续报表须以这些不可变明细和实际账本分别证明来源，不能把核算总额减去当前钱包余额作为未付金额。生产容量与客户人工审核继续独立验收；人工修正后重算的资金政策按39号合同验收。
