# 四端页面文案保留候选

用户端、品牌后台、总后台、代理端及其共享组件的说明文案已收敛。下表 35 组暂时保留，供逐项决定删除、缩短或保留；编号可直接用于反馈。

## 保留候选

| 编号 | 页面或操作 | 当前中文原文 | 暂时保留的理由 | 位置 |
| --- | --- | --- | --- | --- |
| 01 | 四端账号创建与改密 | 账号须为 3–32 位，以小写英文字母开头，仅可包含小写英文字母、数字或下划线。<br>密码须为 6–30 个字符。 | 账号和密码存在非直观格式限制，提交前能看见可减少反复试错。 | [AccountCreateFields.vue](../shared/src/AccountCreateFields.vue#L8) |
| 02 | 品牌后台 添加会员 | 账号或手机号至少填写一项。 | 说明账号与手机号至少填写一项，两者都不是单独必填。 | [MemberProvision.vue](../admin-web/src/MemberProvision.vue#L117) |
| 03 | 用户端 验证码 | 有效期至 {expires}。提交失败后请使用新图片。 | 验证码有有效期，提交失败后需要更换图片。 | [Page.vue](../user-web/src/Page.vue#L899) |
| 04 | 用户端 加入已有账号 | 此账号已存在。加入当前品牌需同意以下条款： | 提示正在加入另一品牌，需要会员再次确认条款。 | [Page.vue](../user-web/src/Page.vue#L931) |
| 05 | 用户端 充值 | 充值由客服登记。 | 页面没有自助充值入口，需要告诉用户由客服登记。 | [RechargePanel.vue](../user-web/src/RechargePanel.vue#L179) |
| 06 | 用户端 帮助中心 | 仅限成年人，量力参与，适时休息。 | 参与年龄与适度参与提示；是否保留由你判断。 | [Page.vue](../user-web/src/Page.vue#L1250) |
| 07 | 用户端 提现申请 | 提现积分须为 {step} 的整数倍。 | 品牌可设置整十、整百等积分步长，影响当前可填金额。 | [WithdrawalPanel.vue](../user-web/src/WithdrawalPanel.vue#L282) |
| 08 | 品牌后台 提现设置 | 1 为整数，10 为整十，100 为整百，1000 为整千。 | 解释步长数值的含义，避免把步长当成最低提现额。 | [WithdrawalPolicySettings.vue](../admin-web/src/WithdrawalPolicySettings.vue#L806) |
| 09 | 品牌后台 Logo 上传 | PNG 或 JPG，最大 1 MB、单边 4096 像素。保存后生效。 | 文件类型、体积与尺寸限制在文件选择前可知。 | [PresentationFields.vue](../admin-web/src/PresentationFields.vue#L90) |
| 10 | 品牌后台 恢复默认展示 | 将全部展示设置恢复为默认值？保存后生效。 | 确认恢复范围，并说明需要保存才生效。 | [PresentationFields.vue](../admin-web/src/PresentationFields.vue#L120) |
| 11 | 总后台 通知模板编辑 | 平台模板由所有品牌共用。修改仅影响新生成的通知，已发送消息保留原内容。 | 修改影响所有品牌；仅编辑弹窗保留，列表重复说明已移除。 | [NotificationTemplatesPanel.vue](../platform-web/src/NotificationTemplatesPanel.vue#L74) |
| 12 | 总后台 通知模板编辑 | 请保留事件所需的 {points}、{resource_id} 占位符，并同时填写中英文文案。 | 事件占位符影响生成消息内容，需要在编辑时提醒。 | [NotificationTemplatesPanel.vue](../platform-web/src/NotificationTemplatesPanel.vue#L83) |
| 13 | 总后台 删除域名绑定 | 该入口会立即失效，域名可由任何品牌重新绑定，历史审计记录保留。 | 删除会使入口失效，并将域名释放给其它品牌。 | [BrandDomains.vue](../admin-web/src/BrandDomains.vue#L189) |
| 14 | 总后台 停用域名 | 停用此入口可能导致管理后台无法访问。若它是当前管理域名，请改用另一条已启用入口。 | 停用当前入口可能导致无法进入后台。 | [BrandDomains.vue](../admin-web/src/BrandDomains.vue#L192) |
| 15 | 品牌后台 踢出会员会话 | 踢出后，该会员需要重新登录当前品牌。 | 说明操作会使该会员重新登录。 | [App.vue](../admin-web/src/App.vue#L1024) |
| 16 | 品牌后台 重置会员全局密码 | 修改全局密码会影响所有品牌并注销全部会话，需拥有该会员所属全部品牌的重置权限。 | 操作跨品牌影响全部会话，需要明确作用范围。 | [App.vue](../admin-web/src/App.vue#L1040) |
| 17 | 总后台 删除品牌 | 此操作会永久删除品牌、会员、账号、订单、流水、配置、审计记录及其它相关数据。 | 永久删除品牌与关联数据，需要在确认时说明后果。 | [App.vue](../platform-web/src/App.vue#L722) |
| 18 | 品牌后台 启停品牌 | 暂停新投注；已有注单、退款、开奖和结算继续处理，提现按独立策略处理。<br>恢复接受新投注，具体以彩种配置和销售期次为准。 | 说明新投注和存量注单、结算、提现的处理区别。 | [BrandOperation.vue](../admin-web/src/BrandOperation.vue#L228) |
| 19 | 总后台 停用品牌 | 立即停止运营，阻止用户登录和发起新业务操作；总后台可以重新启用。 | 停用会阻止登录和新业务，影响比暂停更大。 | [BrandOperationPanel.vue](../platform-web/src/BrandOperationPanel.vue#L135) |
| 20 | 总后台 品牌定时恢复 | 到期自动恢复，手动变更状态会取消原计划。 | 手动变更会取消原恢复计划，影响执行时间。 | [BrandOperationPanel.vue](../platform-web/src/BrandOperationPanel.vue#L131) |
| 21 | 品牌后台 标记提现已付 | 仅为内部账本操作，不会进行外部转账。 | 避免把内部记账理解为已经完成外部转账；重复悬浮说明已移除。 | [WithdrawalManagement.vue](../admin-web/src/WithdrawalManagement.vue#L187) |
| 22 | 品牌后台 开启佣金派发 | 开启此运行开关可能立即接入历史 ready 且未派发周期。若派发模式为自动，后台可能在无需逐任务审批的情况下实际给账号入账积分。关闭开关只暂停后续入账，不撤销已有审批或已入账积分。 | 开关可能立即处理已有待派发周期，直接影响积分入账。 | [CommissionPaymentsManagement.vue](../admin-web/src/CommissionPaymentsManagement.vue#L390) |
| 23 | 品牌后台 佣金派发异常 | 混合周期须整期审批，自动部分不能单独派发。<br>部分目标已入账，重试仅处理未完成目标。 | 整期审批限制与部分入账重试范围影响资金操作。 | [CommissionPaymentsManagement.vue](../admin-web/src/CommissionPaymentsManagement.vue#L374) |
| 24 | 品牌后台 佣金人工修正 | 派发完成后可进行人工修正 | 说明派发未完成时无法修正的原因。 | [CommissionPaymentsManagement.vue](../admin-web/src/CommissionPaymentsManagement.vue#L377) |
| 25 | 品牌后台 佣金更正 | 差额用于核对，不代表已授权执行。<br>新核算覆盖旧人工修正目标，差额可能补发或追回。 | 重新核算可能覆盖人工目标，并发生补发或追回。 | [CommissionCorrectionsManagement.vue](../admin-web/src/CommissionCorrectionsManagement.vue#L417) |
| 26 | 总后台 收回品牌充值额度 | 只能收回剩余未使用的额度，已充值给会员的积分不受影响。 | 区分未使用额度与已经充值给会员的积分。 | [RechargeQuotaPanel.vue](../shared/src/RechargeQuotaPanel.vue#L89) |
| 27 | 品牌后台 一级代理切换模式 | 请逐个填写全部下级的新比例。整条代理线从下个佣金周期统一切换，当前周期继续使用原模式和原比例；需先配置结算周期。代理状态和发展下级设置沿用当前已保存配置。 | 整条代理线同时切换，生效在下个周期，并非立即生效。 | [AgentManagement.vue](../admin-web/src/AgentManagement.vue#L367) |
| 28 | 总后台 认证与风控 | 尚未接入验证服务的检查项，启用后会要求审核，并阻止新的注册或投注。 | 检查服务未接入时启用仍会阻断注册或投注，需要在启用前知道。 | [SecuritySettingsPanel.vue](../platform-web/src/SecuritySettingsPanel.vue#L121) |
| 29 | 品牌后台 风控组件 | 真实检查服务尚未接入；已启用的政策仍会限制注册、投注和提现。 | 同类开关会限制业务；该组件的现有能力提示暂保留。 | [CompliancePolicy.vue](../admin-web/src/CompliancePolicy.vue#L103) |
| 30 | 总后台 彩种上下架与启用 | 从用户列表隐藏，停止新投注及预约新自动期次。已有期次和注单继续开奖结算，历史记录和钱包流水保留。<br>恢复列表展示，仍按已有销售及自动开奖配置决定是否开放投注。<br>从用户列表隐藏并停止新投注。已有注单继续结算，历史记录和钱包流水保留。 | 说明是否停止新投注，以及已有注单是否继续结算。 | [PlatformLottery.vue](../shared/src/PlatformLottery.vue#L83) |
| 31 | 总后台 号码模型 | 已有保存规则或期次的彩种，不能直接覆盖号码模型。 | 解释已经存在规则或期次时不能编辑模型的原因。 | [PlatformLottery.vue](../shared/src/PlatformLottery.vue#L255) |
| 32 | 总后台 待开奖期次 | 当前显示最早的50个待开奖期次，完整排期请查看彩种的期次列表。 | 当前仅显示最早50期，避免把可见列表误认为全部排期。 | [PlatformDrawOverview.vue](../shared/src/PlatformDrawOverview.vue#L60) |
| 33 | 总后台 设置开盘日期时间 | 仅修改本期，自动期次同样适用。已产生注单、已封盘或已开奖的期次不可修改。 | 说明修改只作用于本期，以及已投注等情况下不可改。 | [PeriodOpeningDialog.vue](../shared/src/PeriodOpeningDialog.vue#L43) |
| 34 | 总后台 规则试算 | 演示属性及价格均为人工样例，不是真实赔率。 | 防止把人工样例属性和价格误认为正式赔率。 | [ConfiguredRuleWorkbench.vue](../shared/src/ConfiguredRuleWorkbench.vue#L136) |
| 35 | 品牌后台 销售配置 | 尚未配置。请核对停用状态的草稿，明确保存后才能接受投注。<br>已发布规则已变化，请核对当前价格后重新保存。 | 明确首次配置与规则更新后的阻塞状态，避免把未保存草稿当成在售价格。 | [ConfiguredPublishing.vue](../shared/src/ConfiguredPublishing.vue#L75) |

## 必要反馈和功能正文

字段名称、操作名称、真实业务数据与状态、加载及空状态、输入校验、读取失败、权限不足、版本冲突和提交结果未知等操作反馈保留。未知结果提示只在相应请求待确认时显示，不作为常驻教程。

帮助中心问答、会员主动打开的玩法规则、服务条款和隐私政策属于对应入口的正文，暂时保留，没有挪到其它页面或增加说明入口。

## 已移除的说明

- 用户端：Telegram 授权操作的重复提示和授权有效期说明。
- 品牌后台：添加会员的整块教程与条款说明侧栏、报表查询教程、运营工作台脚注、佣金舍入说明、权限合并原理、只读操作介绍、百分比填写教程，以及配置页面的冗余说明。
- 总后台：报表统计口径常驻段落、CSV 导出介绍、重复的平台模板作用域说明、彩种统一维护介绍、品牌创建的审计说明和日历每日同步介绍。
- 代理端：会员归属与首次登录教程、净输赢口径段落、百分比填写教程。
- 共享组件：规则配置帮助、数据源 JSON/CSS/DOM/正则与时间解析教程、自动开期教程、归档口径段落、额度用途介绍、二维码长按保存提示，以及后台纯技术编号信息区。佣金计算业务明细及分页仍保留。

说明文案的移除同时覆盖中英文；已无调用的翻译项同步清理。业务校验、权限、原请求重试、资金计算和已有记录未改变。

## 验证

四端及共享组件类型检查、1,146 项单元测试、226 项静态检查通过。页面回归中 200 项通过；另 12 项登录与空状态检查在更新既有选择控件断言后复核通过。覆盖中英文、安卓与 iOS 横竖屏、不同宽高及 320 CSS px 重排。另核对了移除教程侧栏后的会员创建表单。浏览器验证使用模拟接口，没有登录开发者账号或写入正常开发库。
