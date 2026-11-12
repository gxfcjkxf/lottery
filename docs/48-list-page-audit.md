# 管理端列表页完整核查登记（2026-11-02）

本清单依据页面入口、全部 Vue 表格、分页器及重复记录集合核查，包含共享组件和详情子列表，不以是否已有分页器作为列表识别条件。实施规则见 [设计规范 §3.8](48-ui-architecture-and-design-system.md#38-平台品牌代理列表页统一契约)。同一组件的多个业务视图分别列出；只有固定配置、日历和详情字段明确不适用记录分页。

## 品牌后台

| 源文件 | 核查对象及处理 |
| --- | --- |
| `admin-web/src/App.vue` | 会员目录：共享查询、批量删除、行菜单、空表和分页；新建在列表工具栏 |
| `admin-web/src/AccessManagement.vue` | 管理账号、角色：共享表格、行菜单、分页；权限详情不铺开在目录；保留现有编辑/改密权限 |
| `admin-web/src/AgentManagement.vue` | 全层级目录、上下级视图：共享容器、筛选和分页；层级上下文保留；代理配置修订是详情时间线 |
| `admin-web/src/FinanceManagement.vue` | 充值记录、会员积分流水：会员查询；额度管理由独立菜单进入；流水在详情中使用嵌入式列表和分页；来源余额及余额对账为有限维度快照 |
| `admin-web/src/WithdrawalManagement.vue` | 提现申请：筛选、空表、分页；确认与原请求重试留在详情 |
| `admin-web/src/RewardsManagement.vue` | 奖励记录、不可变操作历史：新建/刷新进入工具栏；空结果保留表头；详情历史使用嵌入式列表 |
| `admin-web/src/AuditManagement.vue` | 审计：导出与查询分区；保留空表和分页；翻页不卸载原记录 |
| `admin-web/src/ReportsManagement.vue` | 投注/归属、账本：筛选、导出、汇总、结果及分页归属同一列表；报表分区导航保留 |
| `admin-web/src/CommissionReport.vue` | 佣金流水报表：查询表单、清空、工具栏导出、汇总与分页；草稿不替换已确认的查询 |
| `admin-web/src/RewardReports.vue` | 奖励流水/奖励订单报表：子视图页签独立于筛选；导出及分页使用固定插槽 |
| `admin-web/src/WithdrawalReport.vue` | 提现报表：查询/清空、导出、汇总、空表及分页 |
| `admin-web/src/CommissionCyclesManagement.vue` | 核算目录、任务发现、核算结果、分摊、来源计算、运行历史及历史收益：每个集合独立拥有嵌入式列表与分页；现有查询范围和写入确认不变 |
| `admin-web/src/CommissionPaymentsManagement.vue` | 派发目录、操作历史：共享行菜单和分页；受益人列表由下项组件负责 |
| `admin-web/src/CommissionPaymentTargets.vue` | 派发受益人：真实查询表单、清空、空表和详情内分页 |
| `admin-web/src/CommissionCorrectionsManagement.vue` | 更正目录、计划明细、执行目标：共享行菜单，计划与执行各自分页，单页/空页不隐藏分页栏 |
| `admin-web/src/CommissionAdjustmentsManagement.vue` | 人工修正目标、不可变修正历史：统一查询、选择、行菜单；修复只在多页时显示分页的问题；冲突复核与原键重试保留 |
| `admin-web/src/BetOrderManagement.vue` | 注单入口：实际列表由 `shared/src/ConfiguredOrdersPanel.vue` 承载 |
| `admin-web/src/BrandDomains.vue` | 平台授权下复用的域名绑定目录：完整版本快照按 50 条分页，统一表格和编辑/删除菜单；修改仍逐次使用品牌配置版本和既有确认，不将其当作独立账号批量删除 |
| `admin-web/src/JoinCodeManagement.vue` | 单个会员已绑定的推荐码（0 或 1 个）是详情字段，不是可增长目录，不添加分页 |
| `admin-web/src/BetPolicySettings.vue` | 已关联彩种标签是配置字段，不是业务记录目录，不添加分页 |

## 平台总后台

| 源文件 | 核查对象及处理 |
| --- | --- |
| `platform-web/src/App.vue` | 品牌、品牌会员、审计：共享列表插槽；完整品牌快照分页；授权批量删除；空表与分页一致 |
| `platform-web/src/AccessPanel.vue` | 平台账号/角色、品牌账号/角色四个视图：页签/动作分离；保护当前账号与内置角色；批量处理及分页使用同一权限范围 |
| `platform-web/src/CommissionPanel.vue` | 代理目录、核算周期、派发记录及收益明细：目录查询、单页/空页分页、详情内嵌入式列表 |
| `platform-web/src/RechargesPanel.vue` | 充值记录：会员查询；额度管理由独立菜单进入；表头、空状态及分页保持可见结构 |
| `platform-web/src/WithdrawalsPanel.vue` | 提现记录、详情来源分配及历史：业务记录分页；来源分配是订单快照内有限明细 |
| `platform-web/src/RewardsPanel.vue` | 奖励记录及操作历史：刷新进入工具栏；历史属于详情内独立列表 |
| `platform-web/src/WalletPanel.vue` | 会员流水：详情内统一列表、刷新和分页；四来源×余额状态是固定维度表，不添加分页 |
| `platform-web/src/ArchivesPanel.vue` | 日/月归档：视图页签、刷新、记录和分页归属同一列表 |
| `platform-web/src/NotificationsPanel.vue` | 消息投递：刷新、空表及分页；失败保留已读取的记录 |
| `platform-web/src/NotificationTemplatesPanel.vue` | 通知模板是固定事件目录，展示全部已支持事件并使用共享表格/工具栏，不伪造分页 |
| `platform-web/src/ReportsPanel.vue` | 六类业务报表：查询、导出、汇总和结果属于同一列表，实际总数驱动分页 |
| `platform-web/src/BetOrdersPanel.vue` | 注单入口：实际列表由 `shared/src/ConfiguredOrdersPanel.vue` 承载 |
| `platform-web/src/BrandDomainsPanel.vue` | 平台域名入口：复用 `admin-web/src/BrandDomains.vue`，不维护第二套列表 |
| `platform-web/src/OfficialDrawCalendar.vue` | 月历是固定七列日历，不属于业务记录分页 |

## 独立代理端

| 源文件 | 核查对象及处理 |
| --- | --- |
| `agent-web/src/pages/MembersPage.vue` | 直属/全代理线会员：页签、查询/清空、完整范围汇总、行菜单和分页；只读权限不新增删除 |
| `agent-web/src/pages/ChildrenPage.vue` | 下级代理：统一刷新、表格、空结果及分页 |
| `agent-web/src/pages/CommissionPage.vue` | 佣金流水：汇总、刷新、表格及分页；保留旧页直到新页成功 |
| `agent-web/src/pages/MemberBetsPage.vue` | 会员注单：账号上下文、汇总、记录及游标分页；只允许跳转到已有游标 |

## 三端复用的列表与非列表组件

| 源文件 | 核查对象及处理 |
| --- | --- |
| `shared/src/PlatformLottery.vue` | 平台/品牌彩种目录使用管理端列表壳和完整快照分页；期次明细为嵌入式列表；用户端保持自身导航语义 |
| `shared/src/PlatformDrawOverview.vue` | 历史开奖列表使用嵌入式列表及分页；当前待开奖期次与来源检查为有限实时工作区 |
| `shared/src/ConfiguredOrdersPanel.vue` | 品牌/平台注单列表：共同刷新、表格、空状态和分页 |
| `shared/src/AutomationSettings.vue` | 数据源完整目录：名称表格、行菜单、单页/多页/空结果分页；新增入口在工具栏；采集字段配置不当作记录列表 |
| `shared/src/RechargeQuotaPanel.vue` | 额度管理独立页面与品牌列表额度弹窗共用列表容器，直接展示额度流水及分页；额度调整表单与记录查询分区 |
| `shared/src/ArchiveReport.vue` | 已保存归档快照的统计分组表，数据属于一次完整快照，不在前端截断或伪造服务端分页 |
| `shared/src/ConfiguredRuleWorkbench.vue` | 玩法配置行是完整规则文档中的编辑字段，不属于业务记录目录 |
| `shared/src/NumberModelEditor.vue` | 号码属性是完整号码模型的固定配置，不添加记录分页 |
| `shared/src/ManagementOverview.vue` | 概览指标和固定业务对比卡片，不属于逐条记录目录 |
| `shared/src/OrderHistoryRecord.vue`、`shared/src/ConfiguredOrderTicket.vue`、`shared/src/MarkSixOrders.vue`、`shared/src/CanadaOrders.vue` | 单条注单/一页注单的内容渲染，由所属业务页管理范围与分页，不再套第二层列表分页 |
| `shared/src/ManagementList.vue`、`shared/src/ManagementTable.vue`、`shared/src/PaginationControls.vue`、`shared/src/TablePagination.vue`、`shared/src/BatchDelete.vue` | 公共基础组件：固定插槽/表格/分页/选择行为，不能自行创建业务权限或截断数据 |
| `shared/src/BusinessDetails.vue` | 结构化字段值的展开呈现，不是独立业务记录目录 |

## 代理端完整页面复核（2026-11-02）

代理端必须从真实 `App.vue` 进入，使用本端 `main.ts` 的样式依赖，不能仅凭未传入动作插槽的组件壳验收，也不能引入品牌／平台样式掩盖缺失。`agent-list-review.spec.ts` 验证完整会员查询与添加入口、箭头／页码／跳页、空结果隐藏分页、行菜单、会员注单返回、下级代理及佣金页面；覆盖中英文、安卓／iOS 横竖屏和双引擎 320px 重排。静态结构扫描递归覆盖三端全部 Vue 文件，并识别 `DirectorySearch` 的筛选插槽归属。

## 验收约束

结构检查同时检查主页面与共享组件；分页必须属于实际列表的 `pagination` 插槽；查询必须是真实表单并处于 `filters` 插槽；嵌入式列表使用同一容器的 `embedded` 模式，不再靠业务类名跳过检查。表头不能随记录数组为空而卸载。固定配置和快照是明确的数据模型分类，不能用它们豁免新的可增长业务目录。

浏览器回归使用合成接口，覆盖中英文、安卓/iOS 横竖屏、Chromium/WebKit、320 CSS px 和桌面；写入回执、未知结果、版本冲突及权限范围继续执行原业务保护。未登录开发者账号，未向共享业务数据库写入测试数据。

## 平台、品牌及用户端真实入口验收（2026-11-02）

组件测试壳仅用于局部状态和请求契约，不作为整页布局完成的依据。`real-console-lists.spec.ts` 从真实 `index.html` / `main.ts` 进入，通过实际导航访问品牌后台全部列表菜单和报表页签；平台覆盖品牌、会员、平台及品牌账号／角色、充值、提现、奖励、六类报表、代理／周期／派发、审计、归档、通知模板、彩种及注单。品牌菜单数量与能力矩阵同时校验，新增菜单不能静默遗漏。接口均为独立合成数据，未知请求直接失败。

`finance-consistency.spec.ts`、`brand-logs.spec.ts`、`compact-lists.ts`、`platform-settings.ts` 及 `commission-management.spec.ts` 的核算／派发流程已改用真实应用入口与本端样式，分别验证有记录时的资金详情／审核确认、审计变更详情、代理及通知模板详情、平台设置和模板编辑。会员／代理目录现有真实页面测试继续验证查询、多页、详情、权限与写入结果，并共用布局断言。用户端证据登记在 [用户端列表核查](48-user-list-audit.md)。

`real-page-review.ts` 检查真实应用入口、外端样式混入、单一列表外框、筛选及动作位置、表头、空表列数、分页结构／位置和响应式；内容足够容纳时检查查询与全局动作同行，窄屏按实际控件宽度换行。弹窗内的独立详情列表有独立作用域。`real-page-guards.spec.ts` 在真实页面故意移除表头、替换箭头、将分页移入表格滚动区及插入嵌套列表，证明规则能够阻止这些退化，随后恢复 DOM。

本轮发现品牌彩种目录通过动态组件进入列表壳后仍使用独立卡片，已改为共享表格及行菜单，保留销售配置与启停权限。静态检查现在识别动态 `ManagementList` / `DetailModal` 及 `ManagementTable` 的条件显示；表单检查直接读取 Vue AST，不能由注释或 `data-class` 冒充通过。空列表组件夹具不再对未登记接口返回默认成功。

真实设备复核另发现报表空结果在手机被隐藏、宽表使空提示落到可视区外，以及报表表格再次套边框。品牌／平台报表的空表沿用共享响应式表格，提示允许自然换行，保留语义表头；表格与列表共用边界。布局断言现在同时检查空提示文字的实际范围及第二层表格边框，不能用元素仍在 DOM 中作为“可见”的依据。佣金筛选复用“查询／清空”文案，清空仍保留周期约束。

## 四端括号文案复核（2026-11-02）

递归检查四端及共享目录的 TypeScript 字符串和 Vue 脚本，读取 AST 解码后的文案，包含 Unicode 转义、行内目录和复用目录；同时检查实际页面中的标题、表头、详情字段、表单标签、按钮、选项、占位符和无障碍名称。原有“可选／长度／逗号分隔”等冗余说明已无残留，本轮继续清理以下引用：

| 目录／引用 | 处理与复核 |
| --- | --- |
| 品牌佣金派发模式 | “混合／Mixed”统一用于列表、详情和操作摘要；原有整周期审批保护保持，组件业务测试验证 |
| 平台佣金精确金额 | 删除分子／分母括号；真实周期详情表头与手机 `data-label` 同步，数据继续显示 `10 / 2` |
| 共享采集配置 | 精简辅助请求地址、起始索引、来源时区；JavaScript DOM、URL 编码正文及 Go 时间模板保留必要类型区别；映射编辑器与自动化目录共同引用中英文目录 |
| 品牌规则模板及待发现计数 | 三位数字直选、特别号属性直接命名，待发现计数删除重复说明；关联浏览器预期同步。当前无页面引用的旧目录项也纳入扫描 |
| 用户端及代理端 | 本端目录未发现需删除的括号说明；共享目录及真实入口的表单、列表和详情一并复核，不增加占位性改动 |

积分／秒等单位、截止不含边界、证据过期状态、位置起算及留空行为等必要信息保留。实际截图还发现会员备注标签下的“可选”及共享账号创建表单的常驻格式／长度说明，均已删除；输入约束与即时错误校验保留，代理真实表单分别验证非法账号与短密码仍被拒绝。没有将删去内容迁移到副标题、常驻提示或悬浮提示。`concise-copy.spec.ts` 验证真实平台导航中的采集配置和佣金详情，并实际输入超长名称证明长度校验仍生效；四端布局断言共用渲染文案检查。`ui-copy-contract.test.mjs` 覆盖解码扫描的反例及必要信息保留；`real-page-guards.spec.ts` 故意在当前设备可见的表头或按钮恢复冗余说明，证明整页验收拒绝该退化。

本轮验收：平台／品牌真实导航 24 项、反向退化检查 6 项、代理及文案专项 36 项、用户端真实页面 36 项均通过；另有 8 项文案／i18n／表单静态检查及 15 项佣金组件测试通过。页面验收覆盖中英文、Android／iOS 横竖屏及 Chromium／WebKit 的 320px 重排，使用独立检出隔离工作区持续修改；本次隔离构建与当前工作区四端构建均通过。

## 多币种充值列表复核（2026-11-11）

| 源文件 | 核查对象及处理 |
| --- | --- |
| `shared/src/CashConsole.vue` | 品牌收款账号与充值/提现订单：共享列表壳、权限动作、表格和分页；只有对应收款方式允许的币种才展示收款账号区域 |
| `shared/src/CashTronPanel.vue` | 专属充值地址：共享列表壳、完整表头、空状态和分页；加载失败时居中重试；钱包通过共享弹窗配置 |
| `shared/src/TronQueryKeysPanel.vue` | 平台查询 Key：工具栏新增、脱敏表格、编辑/测试行操作及共享分页；无批量删除能力，不提供选择框 |
| `shared/src/CashMemberBalances.vue` | 会员币种余额固定四项完整快照，不截断或伪造分页 |
| `shared/src/CashWallet.vue` | 用户充值记录使用用户端记录结构；代理端使用共享管理列表容器、表格和分页，详情在共享弹窗 |
| `shared/src/AnnouncementsManagement.vue` | 公告目录：共享筛选表单、可清除搜索、工具栏、完整表头和分页；配置在共享弹窗，保留发布权限和原请求重试 |
| `admin-web/src/BetDistribution.vue` | 投注分布：共享筛选表单、汇总与明细表，数据为当前期次完整分布，不伪造记录分页 |

中英文、安卓/iOS 横竖屏和 Chromium/WebKit 的320 CSS px 重排已通过实际页面验证。地址预览及 Key 设置沿用通用表单和设计令牌，未增加装饰动效。

| `shared/src/BettingHistoryTable.vue` | 四端共用注单表格，业务范围和分页由所属列表页管理，不重复创建列表容器 |
