# 奖励报表与工作台

0057提供两种只读奖励报表及完整 CSV，供运营和财务核对人工奖励。实际入账报表按账本时间统计发放和撤销；订单报表按创建时间筛选订单，再统计这些订单在查询快照时的状态。两者不能互相替代，也不能作为当前可用余额、提现资格或已封存日月结的证明。

## 两种统计口径

| 报表 | 时间筛选依据 | 分组 | 汇总字段 |
| --- | --- | --- | --- |
| rewards | 实际 ledger.created_at | day、member、order | entry_count、grant_entry_count、grant_points、reversal_entry_count、reversal_points、net_points |
| reward_orders | 原订单 created_at | day、member、state | order_count、original_points、granted_count、granted_points、pending_count、pending_points、revoked_count、revoked_points |

实际入账只统计有奖励订单、不可变动作及对应人工审计见证的 reward_grant 和 reward_reversal，精确匹配赠送可用来源及完整16桶变动。冻结、解冻、普通赠送调整、待处理尝试和站内通知不计入；动作与 ledger 直接绑定，每笔只计一次。订单后来撤销不会抹掉原发放记录。净变动为发放减撤销，一个窗口只有撤销时可以为负，但不表示负余额或负奖励。

订单报表每个订单只计一次。order_count 等于三种状态订单数之和，original_points 等于三种状态原额之和。pending_points 表示当前待处理订单的原奖励金额，不表示已经冻结、预留或扣除；revoked_points 也不表示这段创建时间内发生的撤销入账。改变日期区间筛选的是创建队列，不是状态变化时间。

所有计数和金额均为规范十进制整数字符串，聚合不受单笔 int64 上限约束。仅 rewards.net_points 可以带负号；不接受负零、前导零、小数、指数或 JavaScript Number。日期分组使用所选品牌 IANA 时区，键为 YYYY-MM-DD；会员和订单键使用 UUID，状态键为 granted、revocation_pending、revoked。标签与键一致，按 C 排序。

## 接口与授权

| 方法与路径 | 结果 |
| --- | --- |
| GET /api/v1/admin/reports/rewards | 实际入账分页 JSON |
| GET /api/v1/admin/reports/rewards.csv | 实际入账完整 CSV |
| GET /api/v1/admin/reports/reward-orders | 当前订单队列分页 JSON |
| GET /api/v1/admin/reports/reward-orders.csv | 当前订单队列完整 CSV |

四个接口都需真实管理会话和 X-Brand-ID。查看独立要求 report_reward.view.brand 或显式 report_reward.view.platform；导出还要求对应 report_reward.export 权限。reward.view、wallet.view、佣金权限及超级管理员身份本身不推导报表权限。报表只读授权不能发放、撤销或继续奖励。

from、to、group_by 必填，区间为左闭右开，最长93天。时间采用严格 RFC3339，最多9位小数，偏移量转为 UTC 后精确保留纳秒；数据库微秒边界采用向上取整保持半开区间语义。member_id、order_id 可选，必须属于所选品牌。未知、重复、空值、GET正文和不支持的参数拒绝；无品牌权限先403，不通过筛选泄露外品牌资源。已授权但筛选资源不存在返回404。

JSON 的 limit 默认20、最大100，offset 默认0、最大1000000。响应闭合为 brand_id、snapshot_at、timezone、query、summary、items、total_groups；query 含规范化区间、分组、分页及显式 null 的未选会员/订单。summary 和 total_groups 始终表示完整筛选范围，不只是当前页。

汇总、分组、总组数、时区和快照时间由主库同一 SQL 快照产生。Read Committed 事务在读取前后及审计等待后核验当前会话与角色，审计成功提交后才释放 JSON 或 CSV。审计失败返回503；等待期间会话过期返回401且回滚，不返回已算出的数据。查询和导出不改变余额、原订单、账本或奖励动作。

## 完整 CSV 合同

导出不接受 limit 或 offset，不导出当前页。超过10000组或4MiB返回413，没有截断文件。文件采用 UTF-8 BOM、版本1，首行为列名，其后恰好一行 summary 和全部 group；零组仍保留汇总行。前11列为：

`record_type,brand_id,snapshot_at,timezone,from,to,group_by,member_id,order_id,key,label`

随后按上表字段顺序追加6列或8列。summary 的 key/label 为空，各行均回显相同品牌、快照、时区和筛选。负 net_points 在 CSV 加前置单引号防止电子表格公式解释；客户端验证后按有符号整数读取。

响应包含 Content-Disposition、Content-Length、Cache-Control:no-store、X-Content-Type-Options:nosniff，以及 X-Report-Brand-ID、Kind、Snapshot-At、Timezone、Group-Count、Byte-Count、SHA256、Format-Version、Audit-ID、From、To、Group-By。带筛选时另有 X-Report-Member-ID 或 X-Report-Order-ID。SHA256 覆盖含 BOM 的完整响应字节，审计编号指向已提交的本次导出记录。文件名为 lottery-kind-brand-UTC快照.csv。

客户端在下载前核对字节数、SHA256、BOM、完整组数、列与元数据、已提交查询、规范整数、分组排序及所有分项总计。不能把散列匹配但范围错误、少组或总额不一致的文件当成成功下载。

## 管理页面与工作台

后台“报表和对账”内的“奖励报表”支持实际入账和当前订单切换，展示全部汇总字段及分组，提供会员/订单筛选、分页和完整 CSV。时间输入明确使用 UTC；品牌时区另行显示。导出绑定最近一次已提交查询，不使用未提交的输入草稿。切换类型、品牌、账号或权限，以及退出页面，会取消在途读取和导出、清除旧数据，迟到响应不得覆盖新范围或自动下载。

工作台的奖励卡片只按所有订单的当前状态显示 granted_count、pending_count、revoked_count，不带日期筛选或金额，也不统计尝试次数。该卡片独立要求 reward.view.brand 或 platform，报表权限不代替它；无权返回 forbidden/null，不显示假零。确实无订单且有权时才显示三个零。管理跳转进入“人工奖励”，不直接产生资金操作。佣金工作台摘要仍未实现。

## 升级与验收边界

先正常 migrate 至0057，再重启 API 与 worker。新增迁移仅增加四个报表权限、服务器引导角色授权及账本窗口索引，不回填奖励、不改积分或旧历史；自定义角色须显式授权。旧迁移保持原校验和，新 CLI 引导品牌账号获得品牌查看/导出，平台账号仅获得平台查看/导出。

PC 与360px移动视口的真实浏览器验收使用独立合成库和正常接口创建发放、全额撤销、冻结及不足待处理，核对两种金额口径、完整 CSV 与查询前后资金证据。此范围不替代真机、生产容量、自动活动奖励、不可变日月结或客户人工代码审核；不接入真实支付。
