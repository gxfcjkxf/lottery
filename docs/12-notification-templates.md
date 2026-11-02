# 站内通知模板配置和历史消息合同

平台运营人员可统一维护二十一类站内通知的中英文标题和正文，包含六种提现状态、三种佣金入账/人工修正/结果更正、三种奖励历史及开奖公布/更正。发布模板不创建通知、不修改积分；消费者在真实业务事件落库后生成消息，并保存当时模板版本和文案。本文供第三方接入配置、重试和消息展示，不包含外部渠道或真实支付。

## 配置接口和权限

所有接口要求真实平台后台会话，不要求 X-Brand-ID。当前配置与更新使用主库；不可变修订历史沿用已配置的历史读取路由。路由没有用户品牌路径别名：

| 方法 | 路径 | 响应 |
|---|---|---|
| GET | /api/v1/platform/notification-templates | items包含平台共用的二十二个模板 |
| GET | /api/v1/platform/notification-templates/{key}/history | items为按version降序的不可变修订 |
| PUT | /api/v1/platform/notification-templates/{key} | 原提交产生的Template回执 |

列表不接受查询参数。历史只接受limit（默认20，1–100）及offset（默认0，最大1000000），拒绝未知或重复参数。PUT不接受查询参数，正文恰好为version、content、reason，需原幂等键、JSON及匹配Host的Origin；非法key不是新建任意事件的入口。

查看要求 `notification_template.view.platform`，写入要求 `notification_template.write.platform`；权限必须显式授予，超级管理员身份不隐含权限。模板独立于品牌选择、成员关系及品牌状态，所有品牌共用一套。品牌后台不提供入口，原品牌模板接口不再注册。每次读写及原回执重放均重新校验平台会话和权限，并保留审计。

平台初始化二十一类默认模板和系统修订，新品牌直接使用平台模板，不再初始化独立配置。原品牌配置、修订、消息和积分保留，不补造历史通知。数据库须 UTF8。平台模板修改权限显式授予；开发库升级时，为已有平台设置修改角色同步授予模板修改权限，品牌模板权限撤销。

## 模板标识和内容

一期精简后，结算仅提供即时试算；批量对账、在线余额修复和奖励待追缴已移除。人工奖励余额不足时明确失败且不改变资金记录，见[当前精简范围](64-betting-points-simplification.md)。

content恰好为en和zh-CN，每种语言恰好为title和body。两种语言均必填，不自动翻译或复用另一语言文案。标题1–120 字符，正文1–1200字符，须非空、无首尾空白；标题禁止控制字符，正文只允许中间的换行和制表符。不接受HTML尖括号、外部地址标记https?:、javascript:、data:、www.；客户不能通过模板发布富文本或链接。

插值仅支持字面占位符{points}和{resource_id}，不执行脚本、表达式或任意对象访问。未知、未闭合或嵌套花括号拒绝。欢迎通知标题和正文均不能引用points；开奖公布/更正同样禁止points，两语言正文各须resource_id；其他十九类的两种语言正文各须包含points。重复引用允许，不同语言的标题可独立选用合法占位符。实际开奖数字来自独立保存载荷，不是任意模板变量；用户名、手机号、后台人员、证明、内部原因、密码和令牌均不可插值。

Template 为 key、version、content、updated_at、audit_log_id；初始版本 1 的审计 ID 为 null，后续版本必须为非空 UUID。每类事件在平台独立递增版本，最大 9007199254740991。

Revision 为 id、key、version、content、changed_by、reason、audit_log_id、created_at。v1 操作者和审计 ID 均为 null，后续均非空。原因允许空字符串、最多 500 字符且拒绝控制字符。修订和审计与模板更新在同一事务提交，不可改写历史。

## 生效时点和不可变消息

模板在配置提交后对尚未生成的站内消息生效，不按事件最初发生时间选版本。事件已进入队列但尚未落库时，消费者或人工重试使用当时的当前版本；不能将新模板发布宣称为向历史用户再次发送通知。

消费者持有模板行共享锁并复制content和version；配置更新与复制串行。消息内容、消费确认、sent状态仍在同一事务。新消息通过 platform_template_revision_id 关联同 key 的平台不可变修订；消息本身仍按品牌和会员隔离；新插入必须复制当前配置，失败不能保留半条消息或错误的消费确认。重复消费不改写已有消息，包括原来的旧版消息。

用户Item增加content（双语模板源文案或null），template_version改为正安全整数。旧非提现/佣金/奖励/开奖v1消息保持原字段与read_at，content为null，客户端继续固定v1文案；兼容旧接口缺失content仅限这类旧消息。所有提现/佣金/奖励/开奖消息及版本大于1必须有完整快照，不能从当前模板配置补写旧消息。

用户端按快照的语言插值并作为纯文本显示；积分用精确整数字符串分组，不转换浮点。派奖和冲正消息另外显示不可编辑的系统事实说明，明确实际历史入账或全额冲正，不表示当前钱包余额或保证最终中奖。旧消息保留，更正的新代次另记，模板不会改变实际账本金额。

每次新提现状态变化与资金、审计和outbox在同一事务提交；未提交、资格拒绝及原请求重放不产生新消息。数据库要求每个新状态版本有唯一且匹配的事件，事件事实不可改写或删除。消费者校验同品牌、同会员、金额、历史状态版本与审计引用；当前订单已paid或失败时，仍能验证更早的reviewing/processing历史。用户载荷仅resource_id（提现订单ID）和正整数points，不含审核原因、后台账号、来源分配或资格证据。

全部提现消息固定追加不可编辑的双语历史说明：记录的是内部积分状态，不代表当前状态、当前余额或真实外部转账；最新状态须查询提现订单。paid不证明银行、法币或虚拟币出款。模板编辑不能删除这项系统说明，消息与状态历史均保留。

佣金入账、人工修正和结果更正仅从实际非零账本提交生成。paid为正积分，adjusted及corrected为带符号非零差额；均有固定双语历史说明，不代表当前余额、新收入或外部付款。resource_id分别为原派发目标、人工修正及实际更正目标UUID，用户载荷不暴露会员/ledger/目标的私有见证。0060更正事件与金融步骤同事务且拒绝旧目标补造，零见证不生成消息；消费及升级约束见[人工修正](18-commission-adjustments.md)和[更正通知合同](25-commission-correction-observability.md)。

## 奖励历史事件

一期精简后，结算仅提供即时试算；批量对账、在线余额修复和奖励待追缴已移除。人工奖励余额不足时明确失败且不改变资金记录，见[当前精简范围](64-betting-points-simplification.md)。

事件以动作UUID作aggregate_id，resource_id仍为原奖励订单UUID。私有载荷恰好是member_id、resource_id、points、action_id、version和audit_log_id；points始终是原奖励正int64字符串，撤销方向由事件类型表示。版本使用正安全整数。数据库和消费者核对同品牌、原会员、订单原额、不可变动作、审计及实际原赠送账本；待处理动作必须没有ledger。消费者验证历史动作，不与今日订单状态或余额比较，所以后来已撤销也不使过去的待处理消息失效。

事件、动作、积分和审计原子提交；事件生成失败则整笔回滚。事件仅published_at可修改，事实不可删除或改写；新动作提交必须有匹配事件。普通INSERT不能借旧动作或临时表伪造新事件。0056不回填旧奖励，旧待处理订单在升级后经运营明确继续时，只记录这个新动作的事件。

用户载荷仍仅resource_id和points，不公开原操作人、原因、action、audit或ledger见证。三种消息包括v1均要求非空不可变文案快照，不能用今天的配置补显示。用户端和后台固定预览另外保留不可编辑的历史说明；尤其待处理明确“本次没有积分变动”，不是当前状态、余额或外部付款证明。模板编辑、消息读取、标为已读和重复消费都不改变钱包。详细奖励流程见[人工奖励合同](20-manual-reward-orders.md)。

## 原回执和未知请求

PUT成功返回原版本加一、提交的双语文案、提交时间和审计ID。缓存重放仍检查当前平台会话与显式权限；同键不同key或正文冲突。后续模板已到更高版本时，原键仍返回原修订回执，不读取当前配置代替它。

正文或占位符非法400 NOTIFICATION_TEMPLATE_INPUT_INVALID；配置不存在404 NOTIFICATION_TEMPLATE_NOT_FOUND；版本冲突409 NOTIFICATION_TEMPLATE_VERSION_CONFLICT。会话和权限使用标准401/403，存储或审计错误503，不返回部分成功。

后台按账号、事件在页内内存保留冻结正文和原键。发送中禁用重复提交；未知结果保留原请求，重新打开相同事件可显式重试。读取当前配置不会确认未知写入，退出或换账号清理全部意图。

平台通知页默认打开模板列表，中英文同时编辑并直接保存；详情保留不可变修订历史。投递状态仍按品牌查询。

## 尚未包含的功能

开奖受众与正式发布已接入[31号合同](31-draw-result-notifications.md)：本期投注会员每人每个已公布结果一条，包含取消/异常，结果更正另记且原历史保留。新公开payload为resource_id、points:null和闭合draw历史对象，不公开后台见证；实际号码及非中奖/派奖说明不可由模板删除。

本模块不接邮件、短信或Telegram发送，不添加自动代理/推荐/活动奖励事件，不替代真实身份与地区验证。站内消息不启用提现、佣金金融运行开关或支付；正式资格与金融流程须在对应模块验收。运营可编辑的是现有站内文案，不代表完整通知渠道、财务或整个平台已完成。外部凭据、保留年限和对应新事件要按后续真实业务模块单独实现和验收。

## 平台统一模板迁移（2026-10-27）

基线创建 `platform_notification_templates` 和 `platform_notification_template_revisions`，从现有二十一类双语系统默认文案初始化平台 v1；不擅自选取某品牌覆盖其他品牌。原品牌模板和修订作为只读历史保留，已有通知内容、版本、已读状态及业务数据不变，不再参与新消息生成。新品牌不再初始化独立模板。新消息只接受当前平台模板快照，并关联平台修订；资金处理和用户收件箱权限不变。

已有开发库先备份、在副本升级并与新空库基线比较结构，核对原业务列内容完整后显式同步迁移记录，再应用和重启本地 API/worker。


当前支持21种事件：`bet.order.abnormal`、`bet.order.cancelled`、`bet.order.judged_cancelled`、`bet.order.placed`、`bet.order.prize_reversed`、`bet.order.won`、`commission.adjusted`、`commission.corrected`、`commission.paid`、`draw.result.corrected`、`draw.result.published`、`member.joined`、`recharge.confirmed`、`reward.order.granted`、`reward.order.revoked`、`withdrawal.order.cancelled`、`withdrawal.order.failed`、`withdrawal.order.paid`、`withdrawal.order.processing`、`withdrawal.order.rejected`、`withdrawal.order.reviewing`。奖励仅成功发放和成功撤销产生通知。
