# 管理端列表页完整核查登记（2026-11-02）

本清单依据页面入口、全部 Vue 表格、分页器及重复记录集合核查，包含共享组件和详情子列表，不以是否已有分页器作为列表识别条件。实施规则见 [设计规范 §3.8](48-ui-architecture-and-design-system.md#38-平台品牌代理列表页统一契约)。同一组件的多个业务视图分别列出；只有固定配置、日历和详情字段明确不适用记录分页。

## 品牌后台

| 源文件 | 核查对象及处理 |
| --- | --- |
| `admin-web/src/App.vue` | 会员目录：共享查询、批量删除、行菜单、空表和分页；新建在列表工具栏 |
| `admin-web/src/AccessManagement.vue` | 管理账号、角色：共享表格、行菜单、分页；权限详情不铺开在目录；保留现有编辑/改密权限 |
| `admin-web/src/AgentManagement.vue` | 全层级目录、上下级视图：共享容器、筛选和分页；层级上下文保留；代理配置修订是详情时间线 |
| `admin-web/src/FinanceManagement.vue` | 充值记录、会员积分流水：查询与额度操作分区；流水在详情中使用嵌入式列表和分页；来源余额及余额对账为有限维度快照 |
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
| `platform-web/src/RechargesPanel.vue` | 充值记录：查询与额度入口分区；表头、空状态及分页保持可见结构 |
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
| `shared/src/RechargeQuotaPanel.vue` | 额度流水在展开区/弹窗使用嵌入式列表和分页；额度调整表单与记录查询分区 |
| `shared/src/ArchiveReport.vue` | 已保存归档快照的统计分组表，数据属于一次完整快照，不在前端截断或伪造服务端分页 |
| `shared/src/ConfiguredRuleWorkbench.vue` | 玩法配置行是完整规则文档中的编辑字段，不属于业务记录目录 |
| `shared/src/NumberModelEditor.vue` | 号码属性是完整号码模型的固定配置，不添加记录分页 |
| `shared/src/ManagementOverview.vue` | 概览指标和固定业务对比卡片，不属于逐条记录目录 |
| `shared/src/OrderHistoryRecord.vue`、`shared/src/ConfiguredOrderTicket.vue`、`shared/src/MarkSixOrders.vue`、`shared/src/CanadaOrders.vue` | 单条注单/一页注单的内容渲染，由所属业务页管理范围与分页，不再套第二层列表分页 |
| `shared/src/ManagementList.vue`、`shared/src/ManagementTable.vue`、`shared/src/PaginationControls.vue`、`shared/src/TablePagination.vue`、`shared/src/BatchDelete.vue` | 公共基础组件：固定插槽/表格/分页/选择行为，不能自行创建业务权限或截断数据 |
| `shared/src/BusinessDetails.vue` | 结构化字段值的展开呈现，不是独立业务记录目录 |

## 验收约束

结构检查同时检查主页面与共享组件；分页必须属于实际列表的 `pagination` 插槽；查询必须是真实表单并处于 `filters` 插槽；嵌入式列表使用同一容器的 `embedded` 模式，不再靠业务类名跳过检查。表头不能随记录数组为空而卸载。固定配置和快照是明确的数据模型分类，不能用它们豁免新的可增长业务目录。

浏览器回归使用合成接口，覆盖中英文、安卓/iOS 横竖屏、Chromium/WebKit、320 CSS px 和桌面；写入回执、未知结果、版本冲突及权限范围继续执行原业务保护。未登录开发者账号，未向共享业务数据库写入测试数据。
