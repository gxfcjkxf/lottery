# 投注和账本报表的 CSV 导出合同

现有后台投注和账本报表可导出整个筛选范围的一次数据库快照，不按当前页面拼接，也不代表财务关账或全链对账完成。此合同供第三方开发人员实现下载、校验和导入；报表业务统计口径沿用 [API合同](04-api-contract.md#44-s6-d-真实运营报表)。

## 接口和权限

GET `/api/v1/admin/reports/betting/export` 与 `/api/v1/admin/reports/ledger/export` 要求后台登录、有效 `X-Brand-ID`。必须提供RFC3339 from/to，半开范围[from,to)、大于0且最多93天；投注group_by为day/game/member，可选game_id/member_id；账本为day/entry_type，可选member_id，不能提供game_id。

不接受limit或offset，包括值为0的分页参数。未知、重复或不适用参数400 REPORT_QUERY_INVALID；其他品牌的会员或彩种404 REPORT_SCOPE_NOT_FOUND，不能默默扩大筛选。空范围返回零值汇总和账本当前余额。

对应report_betting或report_ledger的view和export都必须满足。每个动作独立接受当前品牌`.brand`或显式`.platform`权限，两种范围可组合；只有其中一个动作不够，超级管理员标记本身不授权。品牌权限必须匹配当前账号的品牌范围和角色。0036迁移只扩展服务器引导角色，不给自定义角色扩权。

授权和审计事务为READ COMMITTED，等待授权锁后读取当前权限，返回前再检查会话；不依赖页面过去读取的权限。数据汇总、完整分组、数量及当前余额来自同一个主库SQL语句快照。两个导出是独立快照，不保证彼此或历史页面完全相同，也不是原子月结。

## 响应和限制

成功200直接返回`text/csv; charset=utf-8`文件，不套JSON成功信封；失败仍为标准JSON错误。

- 最多10,000个分组，整个文件含BOM最多4 MiB。超限413 REPORT_EXPORT_TOO_LARGE，不返回截断附件；请缩小筛选。
- 编码、存储或审计失败503 REPORT_EXPORT_UNAVAILABLE，不发送部分附件。
- Content-Disposition为`attachment; filename="lottery-{kind}-{brandUUID}-{YYYYMMDDTHHmmssZ}.csv"`，时间为该快照的UTC秒。
- Content-Length为准确完整字节数，Cache-Control=no-store，X-Content-Type-Options=nosniff。
- X-Report-Brand-ID、X-Report-Kind、X-Report-Snapshot-At、X-Report-Group-Count、X-Report-SHA256、X-Report-Format-Version、X-Report-Audit-ID分别给出品牌、betting/ledger、UTC快照、规范十进制数量、64位小写十六进制摘要、格式版本1和审计UUID。

SHA256覆盖完整含BOM文件。审计保存原查询、快照、品牌时区、分组数、字节数和摘要，提交成功后才发送附件；摘要便于核对传输一致性，不是独立数字签名、收到文件的证明或财务关账证明。网络中断后重试会生成新的快照和审计，不声称旧下载已经成功。

部署的反向代理须对导出响应关闭压缩和内容重写，并保留报告元数据头、准确Content-Length和no-store。当前浏览器SDK用完整未压缩CSV长度验证；若代理改为压缩传输却保留压缩字节长度，浏览器解压后的长度会不符并安全拒绝下载。

服务端不保存文件、公开下载链接或异步任务。浏览器下载后的访问控制和保留由客户管理；保留年限、财务模板和不可变归档仍为待确认范围。

## 列定义和记录

UTF-8 BOM、逗号分隔、LF行尾。文本内的逗号、双引号、换行使用CSV引号转义，不按简单split换行解析。

公共列按以下顺序：

`record_type,brand_id,snapshot_at,timezone,from,to,group_by,game_id,member_id,key,label`

投注追加13列：

`order_count,stake_points,placed_count,won_count,lost_count,abnormal_count,cancelled_count,refund_points,settled_stake_points,unfinalized_stake_points,abnormal_stake_points,current_prize_points,correction_open_count`

账本追加11列：

`entry_count,net_points,recharge_points,prize_credit_points,prize_reversal_points,refund_points,account_count,available_points,frozen_points,withdrawal_points,total_points`

首行为列名，第二行为summary。账本第三行为balances，其中六个区间流水字段为空；summary及group的五个余额字段为空。随后为原始分组key排序的全部group。每个记录重复品牌、UTC范围、数据库快照时间、品牌时区和分组方式；没有资源筛选时game_id/member_id为空。summary和balances的key/label为空。

分组按原始ASCII key升序，SQL显式COLLATE C，不受数据库默认语言排序影响。分组之和必须精确等于summary各字段，余额total_points必须等于available_points＋frozen_points＋withdrawal_points。余额是当前账户合计，与时间区间的账本净变动含义不同；不能把两者不相等判成账本错误。当前代次奖金也不等于历史所有派奖之和。

## 精度和文本安全

金额和计数都是规范十进制整数文本，可超过单账户int64；net_points可为负，其他列非负。不得转换浮点、科学记数法，禁止负零或有前导零的非零数字。Excel等表格软件导入时必须把积分列指定为“文本”，否则其自动转换可能舍入超过15位的整数；CSV本身保留全部数字。

key和label去除前导空白、控制字符或BOM后若以`= + - @`开始，原文本前加一个单引号，防止电子表格当作公式执行。真实负数积分不加单引号，保持数值字符串。客户端排序校验按原始key，而非转义后key；合法的负号开头类型不能因转义被误判。

## 后台下载行为

按钮按已提交筛选和分组导出，不使用未提交草稿或当前分页。客户端在创建下载前校验UTF-8、BOM、列顺序、精确整数、元数据、完整分组数量、总计和SHA256。品牌、会话、报表种类或已提交筛选变化后，迟到响应不得触发旧范围下载；切换报表再切回仍撤销原导出意图。点击退出立即清除本机管理状态并卸载旧视图，再等待服务端注销，不能在注销等待期间继续下载。只读页面权限和导出权限分开显示；前端禁用不替代后端授权。

客户端下载使用临时Blob URL并释放，不保存令牌、文件或查询到localStorage、IndexedDB或Service Worker缓存。响应摘要和同响应提供的摘要头不构成防恶意代理篡改的独立验证。
