# 佣金账本报表与完整导出

0053提供真实佣金派发与人工修正流水的运营报表，0060接入实际更正补发与追回。报表按账本created_at的入账时间筛选，三类入账及净变动分别统计。它不是按投注时间或佣金周期归属的核算报表，不代表当前钱包余额、未派发收益、利润或最终有效应付额；周期核算与历史代次仍使用既有佣金周期接口。

## 数据来源与统计口径

原派发仅统计paid目标绑定的真实commission ledger，修正仅统计commission_adjustments绑定的真实commission_adjustment ledger，更正仅统计applied非零执行目标绑定的真实commission_correction ledger。每条流水只计一次，三类记录先独立选择再合并，不能将多条修正与原目标直接展开相乘。核算ready、待批准、零额无ledger目标、人工普通钱包调整及冻结/解冻不冒充佣金业务入账。

原目标及ledger不可改写。周期后来blocked、结果更正或代理停用时，过去确已入账的记录仍保留在报表；读取不以今天的代理比例、当前结果或有效证据覆盖它。原派发和人工修正的身份与周期来自原支付目标，更正来自保存的实际执行目标；后来补发的代理可以没有原支付目标，不能因此漏算，也不从当前代理配置重建。

时间区间采用开始含、结束不含，以point_ledger_entries.created_at为准，不能替换为修正记录created_at、注单placed_at或周期window_to。账本时间不是事务提交时刻，导出是读取时的快照，不是封存的日/月财务档案。PostgreSQL存储到微秒，纳秒筛选上下界均向上对齐微秒，以保持半开区间语义。

## 查询与权限

| 方法 | 路径 | 语义 |
| --- | --- | --- |
| GET | /api/v1/admin/reports/commission | 分页分组与完整筛选范围汇总 |
| GET | /api/v1/admin/reports/commission.csv | 全部分组的CSV，不是当前页面导出 |

两条接口均需真实管理会话和X-Brand-ID。查看要求report_commission.view.brand或显式platform授权；导出还要求对应report_commission.export。超级管理员身份不绕过授权，停用品牌仍可读取历史。0053只给服务器引导角色补对应范围的查看/导出权，自定义角色不扩权；新管理员CLI同样显式按品牌或平台授予。

from、to、group_by必填。时间为RFC3339Nano，UTC区间必须非空且不超过93天；group_by只接受day、agent、cycle。可选agent_id、member_id、cycle_id必须属于当前品牌，未知或外品牌资源404，合法资源在窗口内无流水返回零汇总及空数组。day按报表快照时品牌时区分组，agent/cycle使用原UUID，label与key相同。后台时间控件使用设备时区，提交转为UTC并显示回显区间；它不自动把输入当作品牌午夜。

JSON查询limit默认20、上限100，offset默认0、上限1000000；规范非负整数字面值之外、空值、重复或未知查询参数均拒绝。CSV拒绝limit/offset，包括显式0；不接受game_id或编造按彩种分配已取整周期佣金的查询。每次操作走主库，以单条SQL快照生成时区、汇总、分组、计数和分页；同一Read Committed授权事务持有真实会话/角色保护，读取前后重新校验并提交审计，之后才返回正文。

## 响应和精确金额

JSON为brand_id、snapshot_at、timezone、query、summary、items、total_groups。query完整回显from、to、group_by、limit、offset及三个可空UUID筛选。items为key、label、totals；按key的C排序分页。summary与total_groups覆盖完整筛选范围，翻到空页时可能仍有非零总计，不能将当前页的金额当品牌总额。

totals恰好包含entry_count、paid_entry_count、paid_points、adjustment_entry_count、adjustment_credit_points、adjustment_debit_points、correction_entry_count、correction_credit_points、correction_debit_points、net_points。前九项为非负规范整数字符串；net_points可为负。SQL使用numeric累加，Go及浏览器保持十进制字符串/任意精度整数，不限制累计总额为单笔int64，也不转换JavaScript Number。

entry_count等于派发、人工修正及更正三项条目之和；net_points等于paid_points加adjustment_credit_points减adjustment_debit_points，再加correction_credit_points减correction_debit_points。例如原派发1，随后人工修正+2、+1、-4，完整窗口为4条、派发1、上调3、下调4、净0。若窗口只包含最后的-4，则净变动为-4；这不是负佣金计算或负余额，因为之前的入账位于本次窗口之外。

## CSV与审计证据

CSV使用UTF8 BOM及标准引号转义，第一行为列名，第二行为summary，之后每个group一行。列顺序为record_type、brand_id、snapshot_at、timezone、from、to、group_by、agent_id、member_id、cycle_id、key、label，接上述十个totals字段，合计22列。summary的key/label为空；所有行重复相同快照和筛选元数据。负net_points以单引号前缀写入，例如`'-4`，客户端只在这个字段按规范移除此安全前缀，不能将任意公式当数值。

导出最多10000分组和4MiB。超限413，不生成截断文件或部分成功；零分组也有完整summary行。服务验证全部分组金额之和等于summary。响应提供Content-Disposition、Content-Length、no-store、nosniff以及X-Report-Brand-ID、Kind=commission、Snapshot-At、Timezone、Group-Count、Byte-Count、SHA256、Format-Version=2、Audit-ID。摘要包含BOM和全部字节，审计保存筛选、快照、时区、组数、字节数及摘要；审计失败503，不泄露JSON/CSV数据或下载回执。旧导出审计仍为版本1，其他报表也不升级；新版客户端须与API协调部署。

浏览器导出冻结已成功查询的筛选，不采用尚未点击查询的草稿，分页不限制导出范围。下载前核验完整列、每行范围、分组排序、规范数值、总计关系、全部分组之和、字节数、SHA256和真实审计头。导出是另一份最新服务端快照，不要求它与较早JSON快照相同。账号、品牌、权限、查询或读取代次变化后，迟到数据及下载均丢弃；401清理可见数据并交由后台退出流程处理。

## 升级与剩余范围

0053只增加权限及按品牌/账本时间的佣金业务部分索引，不改余额、核算、目标、修正、旧ledger、审计或outbox。索引由事务迁移创建，部署大库应安排迁移窗口及锁等待评估，不宣称零停机。原0052金融数据和普通角色在升级及重复迁移后保持，已有金融开关不因报表启用。

报表读取和导出不解锁blocked、不批准派发、不修正金额。0059实际更正差额及0060补偿通知/报表、0055至0057人工奖励订单/撤销/通知/报表已接入；具体历史与升级边界见[25号合同](25-commission-correction-observability.md)。OPEN-117特殊净额组合仍阻止；按投注归属的跨彩种佣金分析、不可变日/月归档及完整生产容量仍需后续实现验收。
