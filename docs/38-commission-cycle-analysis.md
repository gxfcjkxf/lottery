# 佣金跨周期核算与实际入账分析

> 0074、正式查询和导出接口、SDK及双语管理组件已接入开发验证；真实API下1440px PC与360px移动浏览器专项已通过，生产容量及操作系统真机仍须独立验收。

佣金周期分析按保存的完整周期选择当前核算及历史实际资金记录，区分核算、人工修正、实际派发和更正补发/追回。它不使用钱包余额计算欠付，不改变比例、周期、旧代次或资金，也不批准任何派发、更正或提现。

## 选定周期和权限

GET `/api/v1/admin/reports/commission-analysis`及同路径`.csv`。管理会话与X-Brand-ID必需；查看同时要求commission.view和report_commission.view，各自允许明确品牌或平台授权，导出另需report_commission.export。身份/超级管理员标记不推导授权；旧入账时间报表权限和合同不变。

from、to、group_by必填；from含、to不含，完整UTC区间最多93天，**按保存的cycle.window_to选择整周期**，不是只取窗口内的入账或投注。group_by为cycle或agent，不按天拆分已取整周期佣金。可选agent_id、member_id、cycle_id须属于当前品牌；JSON分页limit默认20、最多100，offset默认0、最多1000000，CSV拒绝分页参数。未知、重复、空或非规范参数及GET正文拒绝。

选中周期的资金记录取全部历史实际入账，包含窗口外稍后补发/追回，不能只看本次from/to内的ledger.created_at。过去已入账目标不因原任务后来blocked/stale而消失，零额合法已完成目标不造流水；普通钱包调整及冻结/解冻不冒充佣金收益。

## 响应与完整性

Report恰好brand_id、snapshot_at、timezone、query、coverage、summary、items、total_groups。query恰好from、to、group_by、limit、offset、agent_id、member_id、cycle_id，后三项可空。coverage恰好selected_cycle_count、ready_cycle_count、unready_cycle_count，满足selected=ready+unready。

summary和每组totals恰好十八字段，数量与积分均规范十进制字符串，除下表说明的空值外不含null。items恰好key、label、totals，key/label为保存周期或受益代理UUID，按C顺序分页；summary覆盖完整筛选，不是当前页合计。

| 字段 | 含义 |
| --- | --- |
| observed_calculated_points | 仅当前完整就绪证据的已知核算积分合计，不以部分代次补足 |
| calculated_points | 完整核算合计；未就绪/过期的周期使其null |
| paid_entry_count、paid_points | 原派发真实非零流水数量及积分 |
| adjustment_entry_count、adjustment_credit_points、adjustment_debit_points | 独立人工修正真实非零流水，正负分别统计 |
| correction_entry_count、correction_credit_points、correction_debit_points | 已实际执行的更正非零流水，补发/追回分别统计 |
| posting_entry_count | 上述三类流水数量之和，每条只计一次 |
| actual_net_points | paid+adjustment_credit-adjustment_debit+correction_credit-correction_debit；整周期全部历史的已授予净额，不是余额 |
| manual_adjustment_net_points | adjustment_credit-adjustment_debit，允许带符号 |
| effective_target_points | 同一核算代次内包含合法人工修正；开奖结果更正产生新代次后，以新核算为目标，不叠加旧代次人工偏移；未就绪或证据不完整则null |
| calculation_minus_actual_points | calculated-actual的数学差，允许负值或null，不是授权追回/补发 |
| effective_minus_actual_points | 已知effective_target-actual的数学差，允许负值或null，不是金融执行权限 |
| calculation_complete、effective_target_complete | 两个布尔值，分别对应calculated及effective目标是否完整可解释；后者true要求前者true |

完整有效证据沿用当前周期/run、epoch和最终注单来源复核，不以状态ready或今天比例代替。资金从原目标、人工修正及已应用更正独立枚举，非零项须验证实际ledger、品牌/账户/会员、类型、操作键、金额/来源分配及业务审计，不能内连接丢失损坏引用后报零。重复/缺失或无法归属的资金证据应失败关闭，不释放部份汇总。

0074复核完整关闭窗口清单、投注扣款及中奖流水、16桶前后链、保存政策/代理修订与归属快照、分配分数及每代理一次取整的收益。仅当前完整就绪证据参与已知核算；旧核算、部分生成或epoch过期不得补足。资金来源包含合法零额见证但不伪造零积分流水；重复账本绑定集合化检查。反向引用必须与正式账本指针一致，零目标不存在流水时任何反向账本也应拒绝，不能仅因引用了已知目标就视作合法资金。无法确定归属周期的佣金标签流水使整个品牌查询失败，不能假设其在筛选窗口外；已证明确属其他周期的来源不会混入当前统计。当前已知核算来源或资金见证损坏返回409 `COMMISSION_ANALYSIS_INTEGRITY`，而正常未就绪保留null。

group_by=cycle时逐周期保留空值；group_by=agent时任何选中未就绪周期都可能尚未给出全部受益人，因此组内calculated完整性随整个选定周期范围保守处理。只展示已保存受益人，不从今天代理树猜测未知代理；summary的coverage始终说明完整范围。各组可累加积分/流水计数；coverage的周期计数独立，不将多受益人重复累加成多个周期。

周期分组始终保留每个选定周期，受益人筛选无匹配时显示完整零值或未就绪null；因此total_groups等于selected_cycle_count。代理分组只枚举已经保存的身份；可能没有分组但仍有未就绪覆盖，summary不能因此变成完整零值。JSON分页不要求当前页合计等于summary；完整CSV才逐组校验总计及null传播。

## 人工修正与结果更正

原核算10、人工修正后实际净额12；结果更正产生新代次8时，新核算覆盖旧人工修正差额，实际净额仍如实为12，差额为追回4，最终目标为8。`manual_adjustment_net_points`及原人工修正账本仍独立保留，不改写；更正目标在新批准和资金门控后执行，见[39号合同](39-commission-manual-recalculation-policy.md)。当前基线不提供旧政策未决计划的升级或恢复流程。

若没有新的开奖结果更正，原代次人工修正10至12后，有效目标仍为12、已授予净额为12，有效差额0；不能只因报表读取而把已批准人工目标还原成10。

人工偏移是否仍适用按保存的evidence_epoch判断，而非只比较run UUID：同一证据epoch的技术重算不能擅自清除已批准修正；结算证据改变后，新核算取代旧偏移。原派发0后人工实际入账也属于业务资金，必须防止错误注册另一全额派发，见[40号合同](40-zero-original-commission-evidence.md)。

其他尚未结清、证据过期、准备计划ready、暂停或技术失败不等于资金完成。读取当前已入账与核算不改变原blocked历史、运行开关、批准或暂停门闩。

## CSV和界面

CSV为版本1，UTF8 BOM，完整筛选最多10000组及4MiB，超限413而非截断。列顺序：record_type、brand_id、snapshot_at、timezone、from、to、group_by、agent_id、member_id、cycle_id、key、label；再重复三个coverage字段；最后上述十八totals字段按表中顺序（布尔两字段最后），共33列。summary的key/label空，后续group按key排序；nullable金额写空单元格，布尔恰好true/false，有符号负值使用单引号安全前缀。全部分组金额/流水数量须匹配summary；任一组完整金额为null则对应summary也为null，不把空值当0。

导出具备品牌、kind=commission_analysis、时间、时区、组数、字节数、SHA256、format_version=1和已提交审计回执头；失败不释放文件。管理端使用原报表入口的独立周期分析区，中英PC/360px支持明确查询、分页及完整已提交筛选导出；改草稿、范围/权限/账号或迟到401清理旧响应和文件，不重放资金操作。

验收采用真实多层投注/核算/原派发/人工修正/结果更正及实际差额建立来源，验证跨期入账仍归原周期、不会多修正倍增、旧blocked保持、零额不造流水、未就绪/null及未重试的历史未决计划、不用当前比例/余额重算。直接READ ONLY及HTTP审计/权限/严格参数、完整CSV/摘要、双端未知或迟到读取与资金保持须独立验证。当前规格不证明生产容量或客户人工审核完成。

HTTP使用主库READ COMMITTED，单条SQL同时生成覆盖、完整汇总、分页和数量；整个请求15秒限时。查询失败通过保存点恢复后记录结果，权限/会话在查询后及审计后再复核，审计提交前不输出数据或文件。客户端逐项核验18字段、四个覆盖计数、精确算术、完整性及CSV摘要；筛选变化、账号/品牌/权限变化和迟到401隔离旧结果。

本模块的数据库结构已包含在当前 `0001_baseline.up.sql`。按[空库安装](../README.md#空库安装)初始化新数据库，再运行匹配版本的API、worker和前端；程序不自动迁移，不提供旧开发库的增量升级，也不清空旧库。不自动开启运行开关；正常业务历史、财务账本及封存报告仍按不可改写规则保留。未声明金额链、大历史容量、真机及客户人工审核另行验收。

## 双端浏览器验收

`tests/browser/commission-analysis.spec.ts`在独立合成库内，接续真实周期、派发和人工修正流程运行，不模拟API响应或直接插入成功资金记录。原派发1积分、三笔人工修正分别+1、+2、-4，实际净入账0、保存核算1、同代次有效目标0；十八项指标分别核对，不用钱包余额推算。

查询仅选一秒的周期结束时间窗口，独立账本查询证明四笔入账全部发生在窗口之后；周期与受益代理分组仍纳入这四笔历史记录。完整33列CSV验证BOM、品牌及已提交审计头、组数、字节数、SHA256、负数安全前缀和全量金额。修改筛选清空原结果和导出资格，中英实际查询及移动端展开明细均通过；页面宽度与固定1440/360px比较，不用溢出后的innerWidth放宽判断。

只读步骤前后比较账户、全部余额桶、账本经济指纹，以及固定26张佣金业务表的独立指纹，后者包括核算、批准、派发、人工修正和更正历史；查询审计允许追加，不列入不可变业务指纹。CI在原desktop/mobile隔离矩阵中串行追加该专项，单worker、零重试，并要求有通过用例且零跳过。当前合成浏览器流程不包含未结清周期和结果更正后的资金执行；这些分支已有真实数据库验收，不能把本浏览器专项作为它们的UI证据。详细命令和结果见[实施记录](implementation-progress.md)。
