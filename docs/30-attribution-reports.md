# 投注历史归属统计与完整导出

本模块按投注时间查询跨彩种的运营统计，使用每个注单保存的归属快照，不按今天的代理树、比例或会员加入方式重建历史。它展示当前注单状态及当前最终结算代次，不是佣金应付额、派发资格、利润、当前余额或封存财务日月报；读取与导出不操作积分、不批准佣金。人工修正后重新核算的净额规则见[39号合同](39-commission-manual-recalculation-policy.md)，不由本运营报表计算。

## 查询范围和互斥分组

查询提供direct及downline两种范围。direct按投注时保存的直属代理过滤；downline要求明确agent_id，并按注单保存的代理链成员关系过滤。按agent分组始终使用保存的直属代理，即使过滤的是某上级的整条下级链，也不把同一注单展开到每个祖先分组。全品牌汇总和每个查询中的各组因此互斥，同一注单只计一次。

可按day、game、member、agent、join_method分组。day采用观察时品牌时区；game使用彩种UUID及当前名称，名称不是历史关系证据；member仅使用品牌会员UUID，不输出手机号等基本资料。agent键为保存的直属UUID或none，none表示已知没有代理。join_method只允许保存的domain、operator、agent_code、referral_code，运营创建且携带编码的会员仍按保存的实际加入方式归类。

只接受当前完整的投注归属快照：加入方式、直属身份及无重复且连续的本品牌代理链必须符合结构。缺少或损坏快照使整个已选择投注区间读取失败，不静默略过、按零处理或用当前会员关系补全。代理状态停用不删除其历史统计。公开用户归属接口仍只输出原有安全字段，不因后台统计新增私有代理ID、路径或比例。

## 指标及时间口径

十四项汇总均为规范非负整数字符串，不设聚合int64上限，不用JavaScript浮点数：order_count、stake_points、placed_count、won_count、lost_count、abnormal_count、cancelled_count、refund_points、settled_stake_points、unfinalized_stake_points、abnormal_stake_points、current_prize_points、correction_open_count、final_lost_stake_points。不提供旧格式归属计数。

投注cohort按placed_at落入UTC半开区间[from,to)，不是按派奖或佣金入账时间。原投注和状态计数保留取消、异常及待结观察；settled_stake_points只计入整期当前任务已完成、计算/目标/实际借记及奖金引用匹配、无退款且无待处理更正的won/lost。取消和异常不贡献最终投注或最终未中奖投注。final_lost_stake_points仅累加最终lost投注额，不把中奖金额低于投注额的won注单当作输钱，也不以投注减奖金计算净损失。

开奖结果正在更正时，旧最终代次及局部新代次不成为最终统计；更正完成后原投注时间区间按新当前代次重新观察。current_prize_points是当前最终代次奖金，不是历史累计派奖。该观察不能替代佣金模块的完整最终事实、原来源账本验证及周期核算授权。

## 正式接口和权限

GET `/api/v1/admin/reports/attribution`与GET `/api/v1/admin/reports/attribution/export`使用真实管理会话及X-Brand-ID。查看需report_attribution.view.brand或显式platform授权；导出还需对应report_attribution.export。超管身份不绕过授权。0068仅向引导角色增加相应查看/导出权，自定义角色不扩权，不开启任何金融政策。

from、to及group_by必填，时间采用RFC3339Nano且非空、最长93天。可选game_id、member_id、agent_id必须属于选定品牌；可选join_method使用上述枚举。agent_scope默认direct，downline必须有agent_id。列表默认limit20/offset0，分别限制为1至100及0至1000000；导出拒绝分页参数。重复、空、未知参数及GET正文拒绝，不静默扩大查询范围。

JSON恰好为brand_id、snapshot_at、timezone、query、summary、items、total_groups。query完整回显十字段，未使用的身份及加入方式过滤为null。total_groups是规范非负整数字符串，页面summary覆盖完整筛选cohort，不是当前页合计；越界页保留汇总并返回空items。

所有数据、计数、汇总、时区和观察时刻来自主库单一SQL语句，不拼接不同页或读取时刻。HTTP在查询与审计等待后复核当前会话和授权，提交report.attribution.view/export审计后才释放JSON或字节；读取失败及拒绝不返回可用部分结果。当前授权撤销或会话过期不能靠已有数据或下载回执绕过。

## 完整CSV及客户端核验

CSV格式1使用UTF-8 BOM及CRLF，最多10000组、4MiB，超限413且不输出截断文件。列为record_type、brand_id、snapshot_at、timezone、from、to、group_by、game_id、member_id、agent_id、agent_scope、join_method、key、label，随后十四项指标，共28列。summary后包含所有group，没有余额行；每个指标全组精确和必须等于summary。身份/来源过滤与分组必须一致，状态计数总和等于order_count。

响应提供Content-Disposition、Content-Length、no-store、nosniff，以及X-Report-Brand-ID、Kind、Snapshot-At、Timezone、Group-Count、Byte-Count、SHA256、Format-Version、Audit-ID和查询范围头。摘要包含BOM及全部字节。彩种标签按CSV规则引用，公式前缀转义，合法空白和多行名称保留，不改写业务名称；客户端验证完整列、所有范围/数值、组数、排序、汇总与摘要后才触发下载。

后台“报表和对账”接入独立中英归属面板。时间按设备时区显式输入并转为UTC；day分组仍采用服务端品牌时区。查询前固定表单，编辑草稿不改变已显示结果或下载范围；导出使用已提交过滤，不发送列表分页。账号/品牌/权限变化清理旧显示，迟到读取/下载/401不污染新范围；当前401交由应用退出。只读角色不能导出，平台授权与品牌范围明确分开。

## 初始化和验收边界

未发布项目使用当前单份完整基线初始化空库，API与前端使用同一提交，不提供增量升级或旧报表字段兼容。归属查询不改投注、资金、任务、审计、通知或已封存归档内容；查询和导出的访问审计仍按正常流程追加。生产容量、客户人工代码审核、真机及外部文件/财务模板另验。

核心数据库测试使用真实多级代理、两个彩种、投注、取消、异常、结算及结果更正，并验证后续代理停用不改历史归属。当前格式校验拒绝缺失归属和旧加入枚举，全部授权仍按账号/角色配置执行。双端浏览器采用全新独立合成库、当前基线/seed/显式管理员及真实业务接口；不伪造报表响应或私有客户材料。
