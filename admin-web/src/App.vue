<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
  onMounted,
  onUnmounted,
  ref,
} from "vue";
const AccessManagement = defineAsyncComponent(() => import("./AccessManagement.vue"));
import MemberProvision from "./MemberProvision.vue";
import AuthSettings from "./AuthSettings.vue";
import BrandOperation from "./BrandOperation.vue";
import BrandPresentation from "./BrandPresentation.vue";
import CompliancePolicy from "./CompliancePolicy.vue";
import { clearAllPendingComplianceIntents } from "./compliance-state";
const BrandDomains = defineAsyncComponent(() => import("./BrandDomains.vue"));
const BrandCreation = defineAsyncComponent(() => import("./BrandCreation.vue"));
import {clearAllPendingBrandDomainsWrites} from "./brand-domains-state";
import { clearAllPendingBrandCreationWrites } from "./brand-creation-state";
import {watch} from "vue";
import {buildBrandCssTokens,defaultBrand,safeBrandAssetUrl,applyBrandPresentation,type BrandTheme} from "@lottery/shared";
import {createBrandPresentationApi,brandPresentationPermissions,type BrandPresentationRecord} from "./brand-presentation-api";
import {clearAllPendingPresentationWrites} from "./brand-presentation-state";
const FinanceManagement = defineAsyncComponent(() => import("./FinanceManagement.vue"));
const WithdrawalManagement = defineAsyncComponent(() => import("./WithdrawalManagement.vue"));
import BalanceRepair from "./BalanceRepair.vue";
const RuleSimulator = defineAsyncComponent(() => import("./RuleSimulator.vue"));
import PeriodSchedules from "./PeriodSchedules.vue";
import DrawManagement from "./DrawManagement.vue";
import BetOrderManagement from "./BetOrderManagement.vue";
import BetPolicySettings from "./BetPolicySettings.vue";
import PeriodCancellation from "./PeriodCancellation.vue";
import OperationsWorkbench from "./OperationsWorkbench.vue";
const SettlementManagement = defineAsyncComponent(() => import("./SettlementManagement.vue"));
const CorrectionManagement = defineAsyncComponent(() => import("./CorrectionManagement.vue"));
const NotificationDeliveries = defineAsyncComponent(() => import("./NotificationDeliveries.vue"));
const NotificationTemplates = defineAsyncComponent(() => import("./NotificationTemplates.vue"));
const ReconciliationManagement = defineAsyncComponent(() => import("./ReconciliationManagement.vue"));
import {clearPendingReconciliationWrites} from "./reconciliation-state";
const ReportsManagement = defineAsyncComponent(() => import("./ReportsManagement.vue"));
const AgentManagement = defineAsyncComponent(() => import("./AgentManagement.vue"));
const CommissionPolicySettings = defineAsyncComponent(() => import("./CommissionPolicySettings.vue"));
const CommissionCyclesManagement = defineAsyncComponent(() => import("./CommissionCyclesManagement.vue"));
import { clearAllPendingCommissionCycleWrites } from "./commission-cycles-state";
const JoinCodeManagement = defineAsyncComponent(() => import("./JoinCodeManagement.vue"));
import { clearAllPendingAgentWrites } from "./agents-state";
import { clearAllPendingJoinCodeWrites } from "./join-codes-state";
import { clearAllPendingDeliveryRetries } from "./notification-delivery-state";
import { clearAllPendingTemplateWrites } from "./notification-templates-state";
import { clearAllPendingBrandOperationWrites } from "./brand-operation-state";
import { clearAllPendingCommissionWrites } from "./commission-policy-state";
import { useAdminI18n } from "./i18n";
const RuleVersions = defineAsyncComponent(() => import("./RuleVersions.vue"));
const WithdrawalPolicySettings = defineAsyncComponent(
  () => import("./WithdrawalPolicySettings.vue"),
);
import {
  previewCorrection,
  reviewRule,
  simulateRule,
  submitRuleForReview,
  type RuleDraft,
} from "./workflows";
import {
  AdminApiError,
  createAdminApi,
  createIdempotencyKey,
  type AdminAccount,
  type AdminBrand,
  type AdminAuditRecord,
  type Member,
  type MemberStatus,
} from "./admin-api";

const { t, message, locale, availableLocales, setLocale, configure, resetBrand } = useAdminI18n();
const englishUi: Record<string, string> = {
  "请先登录后台账号查看佣金周期。": "Sign in to view commission cycles.",
  "控制台包含已接入流程与原型。提现状态接口已接入，资格规则待配置且不支持真实支付；佣金策略和周期核算已接入，审核和派发尚未实现。": "The console combines connected workflows and prototypes. Withdrawal status is connected; eligibility rules are pending and real payments are unavailable. Commission policy settings and cycle calculations are connected; approval and payouts are not implemented.",
  "此页显示真实成员；账号权限、积分账本、认证设置和投注已接入。": "This page shows live members. Account permissions, the points ledger, authentication settings, and betting are connected.",
  "成员创建与管理为真实操作；投注已接入。": "Member creation and management are live operations. Betting is connected.",
  "审计日志为真实后台数据；投注已接入，提现状态接口已接入，资格规则待配置且无真实支付。": "Audit logs are live admin data; betting and withdrawal status are connected, while eligibility rules are pending and real payments are unavailable.",
  "账号与角色变更为真实操作；投注已接入，提现状态接口已接入，资格规则待配置且无真实支付。": "Account and role changes are live operations; betting and withdrawal status are connected, while eligibility rules are pending and real payments are unavailable.",
  "认证、品牌展示及域名绑定为真实配置；提现状态接口已接入，资格规则待配置且无真实支付。": "Authentication, brand presentation, and domain binding are live settings; withdrawal status is connected, while eligibility rules are pending and real payments are unavailable.",
  "人工充值、冻结、调整与账本为真实操作；提现状态接口已接入，资格规则待配置且无真实支付。": "Manual deposits, freezes, adjustments, and ledger actions are live; withdrawal status is connected, while eligibility rules are pending and real payments are unavailable.",
  "真实提现状态接口；资格规则待配置，无真实支付": "Live withdrawal status API; eligibility rules are pending, with no real payments",
  "切换品牌": "Switch brand", "真实品牌": "Live brand", "演示品牌": "Demo brand", "选择品牌": "Select a brand",
  "没有可访问的品牌": "No accessible brands", "仅原型演示": "Prototype demo only", "主导航": "Main navigation",
  "概览": "Overview", "平台": "Platform", "运营": "Operations", "资金": "Finance", "管理": "Management",
  "帮助与反馈": "Help & feedback", "未登录": "Not signed in", "超级管理员": "Super administrator", "管理员": "Administrator", "原型演示": "Prototype demo",
  "退出登录": "Sign out", "账号菜单": "Account menu", "退出": "Sign out", "运营控制台": "Operations console", "演示原型": "Prototype demo",
  "搜索用户、注单、期次": "Search users, orders, periods", "搜索": "Search", "通知": "Notifications", "帮助中心": "Help center", "关闭说明": "Dismiss notice",
  "交互演示 · 非生产环境": "Interactive demo · non-production",
  "后端品牌上下文": "Backend brand context", "正在读取 GET /api/v1/context": "Reading GET /api/v1/context", "平台品牌上下文": "Platform brand context",
  "当前入口没有可用的公开用户品牌上下文；后台管理认证独立，不受影响。请选择需要管理的品牌。": "No public user brand context is available at this entry point. Admin authentication is independent. Select a brand to manage.",
  "后端品牌上下文未连接": "Backend brand context unavailable", "暂不可用；该接口仅用于只读配置状态，不影响管理员认证和真实用户目录。": "is unavailable. This read-only endpoint reports configuration and does not affect admin authentication or the live user directory.",
  "只读接口 GET /api/v1/context": "Read-only endpoint GET /api/v1/context", "配置版本": "Config version", "真实后台品牌": "Live admin brand", "选择真实后台品牌": "Select live admin brand", "请选择品牌": "Select a brand",
  "早上好，林岚": "Good morning, Lin Lan", "这是今天的运营概况，所有数据均为交互演示。": "Here is today's operations overview. All data is interactive demo data.", "刷新概览": "Refresh overview",
  "运营工作台": "Operations workbench", "请先登录后台账号查看运营工作台。": "Sign in to view the operations workbench.", "请先选择真实后台品牌查看运营工作台。": "Select a live admin brand to view the operations workbench.",
  "进行中期次": "Open periods", "期": "periods", "投注开放中": "Betting open", "期即将截止": "periods closing soon", "今日投注量": "Today's betting volume", "分": "points", "较昨日同期": "vs. same time yesterday",
  "待处理事项": "Pending tasks", "项": "items", "提现": "Withdrawals", "异常注单": "Flagged orders", "待开奖": "Awaiting draw", "账本对账差异": "Ledger reconciliation variance", "最近检查 10:32": "Last checked 10:32", "一切正常": "All clear",
  "期次监控": "Period monitor", "当前品牌 · 实时状态演示": "Current brand · live status demo", "全部期次": "All periods", "注单": "Orders", "等待开奖": "Awaiting draw", "剩余时间": "Time remaining", "投注中": "Betting open", "即将截止": "Closing soon", "待录入结果": "Awaiting result entry",
  "待处理": "Needs attention", "需要你关注的事项": "Items requiring your attention", "提现申请待审核": "Withdrawal requests awaiting review", "笔申请 · 最近 09:42": "requests · latest 09:42", "异常注单待处理": "Flagged orders need review", "笔订单需要核查": "orders need checking", "期次等待开奖": "Periods awaiting draw", "期期待结果确认": "period results need confirmation",
  "投注趋势": "Betting trend", "每日总投注积分": "Daily betting points", "趋势时间范围": "Trend time range", "近 7 天": "Last 7 days", "近 30 天": "Last 30 days", "近七日投注量趋势": "Betting volume over the last seven days", "开奖源健康": "Draw source health", "数据源连接状态演示": "Data source connection demo", "正常": "Normal", "主开奖源": "Primary draw source", "备用开奖源": "Backup draw source", "最近检查": "Last checked", "连续可用": "Uptime",
  "请先选择真实品牌。": "Select a live brand first.", "请先登录后台账号并选择真实品牌。": "Sign in to the admin account and select a live brand.",
  "品牌和域名": "Brands & domains", "真实品牌列表、运行状态、展示与域名绑定": "Live brands, operating status, presentation, and domain binding", "选择一个品牌": "Choose a brand", "选择下方真实品牌后，可查看其运行状态与操作记录。": "Select a live brand below to view its status and activity.", "来自管理员品牌接口": "From the admin brands API", "刷新列表": "Refresh list", "正在读取品牌…": "Loading brands…", "当前账号未返回可管理品牌。": "This account has no manageable brands.", "运行中": "Running", "已暂停": "Paused", "已停用": "Disabled", "当前品牌": "Current brand", "查看运行状态 →": "View status →",
  "管理员登录": "Admin sign in", "使用后台管理员账号登录。": "Sign in with an admin account.", "账号": "Account", "密码": "Password", "登录中…": "Signing in…", "登录并加载真实成员": "Sign in and load members", "请选择真实品牌": "Select a live brand", "成员请求不会使用后端默认品牌。请从侧栏品牌选择器中选择一个有权访问的品牌。": "Member requests never use the backend's default brand. Choose an accessible brand from the sidebar selector.", "进入管理员登录": "Go to admin sign in", "正在检查管理员登录状态…": "Checking admin sign-in…", "已登录": "Signed in", "当前真实品牌": "Current live brand", "读取中…": "Loading…", "刷新成员": "Refresh members", "品牌成员": "Brand members", "条本页": "on this page", "搜索用户名 / 手机号 / ID": "Search username / phone / ID", "搜索成员": "Search members", "正在读取品牌成员…": "Loading brand members…", "该品牌当前没有可显示的成员。": "This brand has no members to display.", "成员": "Member", "成员 ID": "Member ID", "手机号": "Phone", "状态": "Status", "加入时间": "Joined", "备注": "Notes", "标签": "Tags", "操作": "Actions", "编辑": "Edit", "踢出": "Remove", "重置密码": "Reset password", "上一页": "Previous", "下一页": "Next", "每页最多": "Up to", "条": "records",
  "代理管理": "Agent management", "代理树": "Agent tree", "加入码管理": "Join code management", "规则配置": "Rule configuration", "期次和开奖": "Periods & draws", "注单和异常": "Orders & exceptions", "资金与账本": "Funds & ledger", "批量对账": "Bulk reconciliation", "佣金和奖励": "Commissions & rewards", "报表和对账": "Reports & reconciliation", "账号与权限": "Accounts and permissions", "审计日志": "Audit log", "通知投递": "Notification delivery", "通知模板": "Notification templates", "风控与合规": "Risk & compliance", "工作台": "Dashboard", "用户和成员": "Users and members",
  "刷新日志": "Refresh log", "按选中品牌读取真实后台日志。": "Load live admin logs for the selected brand.", "未登录：以下是静态演示样例，不是后台记录。": "Signed out: these are static examples, not admin records.", "请先从侧栏选择品牌；审计请求始终携带明确的 X-Brand-ID。": "Select a brand in the sidebar. Audit requests always include an explicit X-Brand-ID.", "正在读取审计日志…": "Loading audit log…", "所选品牌没有可显示的审计记录。": "No audit records are available for this brand.", "资源": "Resource", "操作人": "Actor", "原因": "Reason", "时间": "Time", "演示日志不会显示为真实后台记录。": "Demo logs are not shown as live admin records.",
  "关闭": "Close", "取消": "Cancel", "必填": "Required", "保存中…": "Saving…", "保存到后台": "Save to admin", "处理中…": "Processing…", "确认踢出": "Confirm removal", "确认重置全局密码": "Confirm global password reset", "编辑成员状态和备注": "Edit member status and notes", "踢出品牌会话": "Revoke brand session", "重置全局密码": "Reset global password", "这会调用后台踢出接口，撤销该用户在当前品牌的会话。": "This calls the admin removal API and revokes this user's session for the current brand.", "新密码": "New password", "重置原因": "Reset reason", "我确认此重置影响该全局账号在所有品牌的密码和会话。": "I understand this resets the global account password and sessions across all brands.",
  "全部管理页面": "All admin pages", "关闭导航": "Close navigation", "移动端主导航": "Mobile main navigation", "用户": "Users", "期次": "Periods", "审核": "Review", "更多": "More",
  "审核提现申请": "Review withdrawal request", "申请编号": "Request ID", "申请人": "Applicant", "申请积分": "Requested points", "积分来源": "Points source", "审核意见 / 原因": "Review / reason", "说明来源核验结论或驳回原因": "Describe source verification or reason for rejection", "驳回申请": "Reject request", "通过审核": "Approve", "批准仅表示演示状态转为处理中；不会向外部付款或修改真实账本。": "Approval only moves the demo request to processing. No external payment or live ledger change occurs.",
  "纠正开奖结果": "Correct draw result", "原结果": "Original result", "新结果": "New result", "影响范围预览": "Impact preview", "预估值 · 非实际回溯结果": "Estimate · not an actual recalculation", "待回溯注单": "Orders to recalculate", "预计重新结算": "Estimated settlements", "账本处理方式": "Ledger handling", "保留原记录并创建冲正": "Keep original entry and create reversal", "纠正原因": "Correction reason", "记录结果来源、校验依据及纠正原因": "Record the result source, verification, and reason for correction", "确认纠正并创建回溯（演示）": "Confirm correction and create recalculation (demo)",
  "人工录入开奖结果": "Enter draw result manually", "来源": "Source", "人工录入（单人操作）": "Manual entry (single operator)", "提交后影响": "Impact after submission", "演示人工结果生效，本期停用外部来源": "Demo result takes effect; external sources are disabled for this period", "校验项": "Checks", "号码范围 · 重复 · 期次状态": "Number range · duplicates · period status", "后续操作": "Next steps", "记录操作者和原因，进入结算流程": "Record operator and reason, then continue to settlement", "人工录入原因": "Reason for manual entry", "说明人工开奖原因和数据核验依据": "Explain why the draw was entered manually and how the data was verified", "确认人工结果（演示）": "Confirm manual result (demo)",
  "域名绑定不配置 DNS、证书或重定向；展示仅支持固定预设与中英文文案，不支持任意 CSS、HTML 或上传素材。": "Domain binding does not configure DNS, certificates, or redirects. Presentation supports fixed presets and bilingual copy, not arbitrary CSS, HTML, or uploaded assets.",
  "管理品牌主题、域名及生效版本": "Manage brand themes, domains, and active versions", "新建品牌为演示入口": "Brand creation is a demo entry point", "新建品牌": "Create brand", "全部品牌": "All brands", "导出配置 ↓": "Export config ↓", "默认语言": "Default language", "主域名": "Primary domain", "品牌": "Brand", "时区": "Time zone", "平台默认 → 品牌覆盖　·　配置发布后清理缓存并返回新版本号": "Platform default → brand override · publishing clears the cache and returns a new version", "数据仅用于演示": "Demo data only", "管理 →": "Manage →",
  "此页显示真实成员；账号权限、积分账本、认证设置和投注已接入，提现仍为原型。": "This page shows live members. Account permissions, the points ledger, authentication settings, and betting are connected; withdrawals remain a prototype.", "会话由同源 HttpOnly Cookie 维护，此页面不会保存访问令牌。": "The session uses a same-origin HttpOnly cookie. This page does not store access tokens.", "请先登录后台账号。": "Sign in to the admin account first.", "请先登录后台账号查看代理配置。": "Sign in to view agent settings.", "当前账号没有此品牌的加入码查看权限。": "This account cannot view join codes for this brand.", "请先登录后台账号，才能配置站内通知模板。": "Sign in to configure in-app notification templates.", "请先登录后台账号，才能查看真实运营报表。": "Sign in to view live operations reports.", "请先登录后台账号，才能查看真实投递记录。": "Sign in to view live delivery records.", "请先登录后台账号，才能管理真实角色和权限。": "Sign in to manage live roles and permissions.", "请登录并选择有钱包查看权限的品牌。": "Sign in and select a brand with wallet viewing permission.", "请选择一个有权访问的品牌以查看注单。": "Choose an accessible brand to view orders.", "请先登录后台账号。投注策略和注单均从真实 API 读取，不使用演示数据。": "Sign in first. Betting policies and orders load from the live API; demo data is not used.", "前往登录": "Go to sign in",
  "真实规则模拟": "Live rule simulation", "请先登录后台并选择品牌；下面的规则审核向导仍为演示。": "Sign in and select a brand first. The rule review wizard below is still a demo.", "旧规则向导演示（不会保存、发布或审核真实规则）": "Legacy rule wizard demo (does not save, publish, or review live rules)", "创建、模拟并提交不可变规则版本供品牌管理员审核": "Create, simulate, and submit immutable rule versions for brand admin review", "版本历史 ↗": "Version history ↗", "草稿 · v13": "Draft · v13", "特别号命中": "Special number hit", "适用品牌：": "Brand: ", "生效方式：下一期期次": "Activation: next period", "草稿保存在本次页面会话中": "Draft saved for this page session", "自动保存演示": "Autosave demo", "移动端规则编辑为只读，可查看模拟并进行审核操作。": "Rule editing is read-only on mobile; simulation and review actions remain available.",
  "彩种模型": "Game model", "号码配置": "Number setup", "选号规则": "Selection rules", "中奖条件": "Winning conditions", "奖级赔率": "Prize tiers & odds", "限额取消": "Limits & cancellation", "测试案例": "Test cases", "模拟结果": "Simulation results", "选择彩种模型": "Choose a game model", "模型确定号码池结构和基本校验方式。": "The model defines the number pool and basic validation.", "普通号码与特别号码分别选择": "Choose regular and special numbers separately", "当前模型": "Current model", "从号码池中选取指定数量": "Choose a set number from the pool", "按位置选择有序数字": "Choose ordered digits by position", "彩种名称": "Game name", "玩法名称": "Play type", "配置号码池和位数": "Configure number pool and digits", "设置合法取值、重复规则及普通 / 特别号码数量。": "Set valid values, duplicate rules, and regular/special number counts.", "普通号码范围": "Regular number range", "普通号码个数": "Regular number count", "特别号码范围": "Special number range", "特别号码个数": "Special number count", "不允许号码重复": "Numbers cannot repeat", "配置选号与组合方式": "Configure selection and combinations", "设置排除、复式展开、号码属性和倍投规则。": "Set exclusions, combination expansion, number properties, and multipliers.", "选号方式": "Selection method", "手动选号 / 复式": "Manual / multiple selection", "单式选号": "Single selection", "单注积分": "Points per bet", "倍数下限": "Minimum multiplier", "倍数上限": "Maximum multiplier", "支持排除号码": "Allow number exclusions", "支持奇偶属性": "Allow odd/even properties", "配置中奖条件和结果特征": "Configure winning conditions and result properties", "仅允许使用规则组件，不执行自定义脚本。": "Only rule components are allowed; custom scripts are not executed.", "等于": "Equals", "包含": "Contains", "＋ 添加条件组件": "+ Add condition", "配置奖级、赔率和舍入": "Configure prize tiers, odds, and rounding", "奖级优先级、排他性、封顶值和舍入策略。": "Set tier priority, exclusivity, caps, and rounding.", "倍率": "Multiplier", "排他": "Exclusive", "封顶 50,000": "Cap 50,000", "不封顶": "No cap", "舍入规则": "Rounding rule", "四舍五入 · 整数积分": "Round half up · whole points", "设置限额与取消规则": "Set limits and cancellation rules", "提供用户、单注和单期期次边界。": "Set user, per-bet, and per-period limits.", "单注最低": "Minimum per bet", "单注最高": "Maximum per bet", "每人每期限额": "Per-user period limit", "截止前取消": "Cancel before cutoff", "允许，原路返还": "Allowed; refund to source", "不允许": "Not allowed", "输入测试选号和开奖结果": "Enter test selection and draw result", "本地原型模拟，不是真实规则引擎；不会创建注单或写入数据。": "Local prototype simulation, not the live rules engine. No orders or data are created.", "测试选号（空格分隔）": "Test selection (space-separated)", "测试开奖结果": "Test draw result", "倍数": "Multiplier", "单注积分（整数）": "Points per bet (integer)", "运行模拟": "Run simulation", "规则模拟完成 · 本地原型": "Rule simulation complete · local prototype", "不是实际规则引擎；结果不会写入注单或账本。": "This is not the live rules engine; results are not written to orders or the ledger.", "规则校验": "Rule validation", "通过": "Passed", "展开组合数": "Expanded combinations", "命中说明": "Match details", "预计积分": "Estimated points", "条件节点": "Condition nodes", "号码范围与重复检查": "Number range and duplicate checks", "封顶后积分与整数舍入": "Capped points and integer rounding", "上一步": "Previous step", "步骤": "Step", "保存草稿": "Save draft", "下一步": "Next", "提交审核": "Submit for review", "审核与发布": "Review & publish", "规则创建者不能审核自己的版本。审核通过后还需单独确认发布。": "Rule creators cannot review their own versions. Approval must be followed by a separate publish confirmation.", "创建人：林岚": "Creator: Lin Lan", "当前操作者": "Current operator", "已提交审核": "Submitted for review", "审核人：周宁": "Reviewer: Zhou Ning", "待审核 · 与创建者不同": "Awaiting review · separate from creator", "待分配审核": "Reviewer not assigned", "审核通过": "Approve", "驳回并填写意见": "Reject and add comments", "确认发布（演示）": "Confirm publish (demo)", "发布后新版本仅作用于新注单，历史订单继续使用原版本。": "The new version applies only to future orders; historical orders retain their original version.", "版本信息": "Version info", "当前生效版本": "Current version", "新建草稿": "New draft", "生效方式": "Activation", "对比版本差异 →": "Compare versions →",
  "真实期次计划": "Live period plan", "请先登录后台账号并选择品牌，才能保存计划、生成和查询期次。": "Sign in and select a brand to save plans, generate, and view periods.", "旧开奖工作台演示（不会开奖、纠正结果或结算真实注单）": "Legacy draw workspace demo (does not draw, correct results, or settle live orders)", "查看期次时间线、来源校验与开奖操作": "View period timeline, source checks, and draw actions", "人工开奖": "Manual draw", "生成期次": "Generate period", "复制期次编号": "Copy period ID", "投注开始": "Betting opens", "投注截止": "Betting closes", "开奖时间": "Draw time", "结算完成": "Settlement complete", "投注订单": "Bet orders", "投注积分": "Bet points", "关联规则": "Linked rule", "品牌时区": "Brand time zone", "优先级与最近检查时间": "Priority and last check", "切换记录": "Change history", "主来源 · API": "Primary source · API", "备用来源 · API": "Backup source · API", "上次校验": "Last checked", "健康": "Healthy", "单人操作，必须记录审计原因": "Single operator; an audit reason is required", "录入": "Enter result", "当前开奖结果": "Current draw result", "上一期": "Previous period", "已确认": "Confirmed", "来源：主开奖源": "Source: primary draw source", "校验项：号码范围 ✓　重复校验 ✓　签名 ✓": "Checks: number range ✓ duplicates ✓ signature ✓", "确认时间": "Confirmed at", "确认并结算 →": "Confirm and settle →", "状态时间线": "Status timeline", "开奖结果已确认": "Draw result confirmed", "来源结果通过校验": "Source result passed validation", "期次投注已截止": "Period betting closed", "人工开奖和结果纠正均属于高风险演示操作。确认前必须查看影响范围并填写原因。": "Manual draws and result corrections are high-risk demos. Review the impact and enter a reason before confirming.",
  "周期规则、试算与记录调整概览": "Overview of cycle rules, estimates, and record adjustments", "新建佣金规则": "Create commission rule", "本周期预计佣金": "Estimated commission this cycle", "待结算记录": "Records awaiting settlement", "覆盖 142 位代理": "Covers 142 agents", "已结算佣金": "Settled commission", "本月累计 · 演示数据": "Month to date · demo data", "佣金规则版本": "Commission rule versions", "修改会创建新版本，已结算历史记录保留原始金额": "Changes create a new version; settled history retains original amounts", "版本历史 →": "Version history →", "只计算有效输钱注单 · 品牌范围": "Counts eligible losing orders only · brand scope", "输赢模式": "Win/loss model", "比例": "Rate", "周期": "Cycle", "每周": "Weekly", "每月": "Monthly", "生效中 · v4": "Active · v4", "生效中 · v2": "Active · v2", "详情 →": "Details →", "基于有效投注流水 · 品牌范围": "Based on eligible betting turnover · brand scope", "流水模式": "Turnover model", "近期佣金记录": "Recent commission records", "人工修正会建立独立 adjustment 记录": "Manual corrections create a separate adjustment record", "导出 ↓": "Export ↓", "代理": "Agent", "模式": "Model", "计算基数": "Calculation base", "佣金积分": "Commission points", "待结算": "Pending settlement", "已结算": "Settled",
  "真实代理配置与历史记录；佣金核算已接入，审核和派发尚未实现。": "Live agent settings and history; commission calculations are connected, but approval and payouts are not implemented.", "真实注单结果及账本流水；汇总余额不是完整逐账户对账证明。": "Live order results and ledger entries; aggregate balances are not a complete account-by-account reconciliation.", "真实站内通知投递记录；外部发送渠道尚未接入。": "Live in-app notification delivery records; external channels are not connected.", "真实版本化站内通知模板；已生成消息保留原文案，不触发新通知或资金变化。": "Live versioned in-app notification templates; generated messages retain their original copy and do not send notifications or change funds.", "审计日志为真实后台数据；投注已接入，提现仍为演示。": "Audit logs are live admin data; betting is connected while withdrawals remain a demo.", "账号与角色变更为真实操作；投注已接入，提现仍为演示。": "Account and role changes are live operations; betting is connected while withdrawals remain a demo.", "认证、品牌展示及域名绑定为真实配置；提现仍为演示。": "Authentication, brand presentation, and domain binding are live settings; withdrawals remain a demo.", "人工充值、冻结、调整与账本为真实操作；提现尚未接入。": "Manual deposits, freezes, adjustments, and ledger actions are live; withdrawals are not connected.", "标为演示的功能不写入后台；账号、积分、规则版本和期次计划已接入真实 API。": "Features marked as demos do not write to admin systems. Accounts, points, rule versions, and period plans use the live API.",
  "加入码": "Join codes", "冻结": "Frozen", "已过期": "Expired", "已注销": "Cancelled", "待审核": "Awaiting review", "已通过": "Approved", "已驳回": "Rejected",
  "已登录管理员账号": "Signed in as admin", "已退出管理员账号": "Signed out of admin", "成员资料已更新": "Member details updated", "该成员的品牌会话已踢出": "The member's brand session was revoked", "全局密码已重置，所有品牌会话已撤销": "Global password reset; sessions revoked across all brands", "审核失败": "Review failed", "提交失败": "Submission failed", "纠正原因必填": "Correction reason is required", "人工开奖原因必填": "Manual draw reason is required", "人工结果已锁定本期（演示）": "Manual result locked for this period (demo)",
  "帮助中心为演示入口": "Help center is a demo entry point", "概览已刷新（演示数据保持不变）": "Overview refreshed (demo data is unchanged)", "配置导出成功（演示）": "Config exported (demo)", "规则草稿已暂存（当前页面演示）": "Rule draft saved temporarily (page demo)", "查看版本差异（演示）": "View version differences (demo)", "来源切换记录（演示）": "View source change history (demo)", "结算任务已加入队列（演示）": "Settlement task added to queue (demo)", "新建佣金规则草稿（演示）": "Create commission rule draft (demo)", "佣金版本历史（演示）": "Commission version history (demo)", "佣金规则详情（演示）": "Commission rule details (demo)", "记录报表导出完成（演示）": "Record report exported (demo)", "已打开规则版本列表（演示）": "Rule version list opened (demo)", "已生成下一期草稿（演示）": "Next period draft generated (demo)", "已复制期次编号": "Period ID copied", "已创建待确认发布版本；演示不会改变实际配置": "Publish version created for confirmation; demo does not change live settings", "规则已提交给周宁审核（演示）": "Rule submitted to Zhou Ning for review (demo)", "审核通过；待发布确认（演示）": "Review approved; awaiting publish confirmation (demo)", "规则已驳回（演示）": "Rule rejected (demo)",
  "安全 / 审计轨迹": "SECURITY / AUDIT TRAIL", "平台 / 品牌配置": "PLATFORM / BRAND CONFIG", "操作控制台": "OPERATIONS CONSOLE",
};
Object.assign(englishUi, {
  "演": "D", "品": "B",
  "请先选择真实后台品牌。": "Select a live admin brand first.",
  "请先登录后台账号，才能查看和操作真实积分。": "Sign in with an administrator account to view and manage live points.",
  "成员创建与管理为真实操作；投注已接入，提现仍为演示。": "Member creation and management are live operations. Betting is connected; withdrawals remain a demo.",
  "帮助中心 ↗": "Help center ↗",
  "只读接口 GET /api/v1/context 暂不可用；该接口仅用于只读配置状态，不影响管理员认证和真实用户目录。": "Read-only GET /api/v1/context is unavailable. It reports configuration only and does not affect admin authentication or the live user directory.",
  "· 2 期即将截止": "· 2 periods closing soon", "· 一切正常": "· All clear",
  "3 笔申请 · 最近 09:42": "3 requests · latest 09:42", "2 笔订单需要核查": "2 orders need investigation", "9 期期待结果确认": "9 periods awaiting result confirmation",
  "＋ 新建品牌": "+ Create brand", "已登录 ·": "Signed in ·",
  "使用后台管理员账号登录。会话由同源 HttpOnly Cookie 维护，此页面不会保存访问令牌。": "Sign in with an administrator account. The session uses a same-origin HttpOnly cookie; this page does not store access tokens.",
  "第": "Page", "页 · 每页最多 100 条": "· up to 100 records per page",
  "星彩 6+1 · 特别号命中": "Star 6+1 · special-number match", "● 自动保存演示": "● Autosave demo",
  "⌁ 移动端规则编辑为只读，可查看模拟并进行审核操作。": "⌁ Mobile rule editing is read-only; simulations and review actions remain available.",
  "1. 选择彩种模型": "1. Choose a game model", "M 选 N": "Pick N from M", "0–9 数字位": "0–9 position digits",
  "2. 配置号码池和位数": "2. Configure number pool and digits", "3. 配置选号与组合方式": "3. Configure selection and combinations", "4. 配置中奖条件和结果特征": "4. Configure winning conditions and result properties",
  "特别号码命中": "Special-number match", "特别号码": "Special numbers", "5. 配置奖级、赔率和舍入": "5. Configure prize tiers, odds, and rounding", "普通号命中": "Regular-number match",
  "6. 设置限额与取消规则": "6. Set limits and cancellation rules", "7. 输入测试选号和开奖结果": "7. Enter test selection and draw result", "▷ 运行模拟": "▷ Run simulation",
  "✓ 号码范围与重复检查": "✓ Number range and duplicate checks", "✓ 封顶后积分与整数舍入": "✓ Capped points and integer rounding",
  "← 上一步": "← Previous", "下一步 →": "Next →", "提交审核 →": "Submit for review →", "林": "L", "周": "Z", "✓ 审核通过": "✓ Approve review",
  "ⓘ 发布后新版本仅作用于新注单，历史订单继续使用原版本。": "ⓘ Published versions apply only to new orders; historical orders keep their original version.",
  "下一期期次": "Next period", "＋ 人工开奖": "+ Manual draw", "⟳ 生成期次": "⟳ Generate periods", "星彩 6+1": "Star 6+1", "复制编号 ⧉": "Copy ID ⧉",
  "10-05 11:00:00 · 18 分钟后": "10-05 11:00:00 · in 18 minutes", "笔": "records", "· 新加坡": "· Singapore", "开奖来源": "Draw source",
  "上次校验 10:32:18": "Last checked 10:32:18", "上次校验 10:31:54": "Last checked 10:31:54", "人工录入": "Manual entry", "上一期 · 20261005031": "Previous period · 20261005031",
  "确认时间：10-05 08:10:14": "Confirmed: 10-05 08:10:14", "↻ 纠正开奖结果": "↻ Correct draw result", "周宁 · 10-05 08:10": "Zhou Ning · 10-05 08:10", "系统演示 · 10-05 08:09": "System demo · 10-05 08:09", "系统演示 · 10-05 08:00": "System demo · 10-05 08:00",
  "⚠ 人工开奖和结果纠正均属于高风险演示操作。确认前必须查看影响范围并填写原因。": "⚠ Manual draws and result corrections are high-risk demo actions. Check the impact and enter a reason before confirming.",
  "＋ 新建佣金规则": "+ Create commission rule", "周结算 · 10/01 – 10/07": "Weekly settlement · 10/01 – 10/07", "输赢佣金 · 星河代理组": "Loss-based commission · Star agent group", "流水佣金 · 海风代理组": "Turnover commission · Sea Breeze agent group",
  "用户 ID": "User ID", "操作原因": "Reason for operation", "全局用户 ID": "Global user ID", "开奖结果": "Draw result",
  "此密码属于全局用户，将影响该用户在所有品牌的登录，并撤销其全部会话。后台要求你对该用户加入的每个品牌都有密码重置权限，否则拒绝操作。": "This global password affects sign-in across all brands and revokes all sessions. The backend requires password-reset permission for every brand the user has joined; otherwise it rejects the operation.",
});
const shellCopy = (key: string) => Object.prototype.hasOwnProperty.call(englishUi, key) ? englishUi[key] : undefined;
const ui = (key: string) => t(key, shellCopy(key));

type Page =
  | "工作台"
  | "品牌和域名"
  | "用户和成员"
  | "代理树"
  | "加入码"
  | "规则配置"
  | "期次和开奖"
  | "注单和异常"
  | "资金与账本"
  | "批量对账"
  | "佣金和奖励"
  | "报表和对账"
  | "账号与权限"
  | "审计日志"
  | "通知投递"
  | "通知模板"
  | "风控与合规";
const nav: { name: Page; icon: string; group: string }[] = [
  { name: "工作台", icon: "▦", group: "概览" },
  { name: "品牌和域名", icon: "◇", group: "平台" },
  { name: "用户和成员", icon: "♙", group: "平台" },
  { name: "代理树", icon: "⌘", group: "平台" },
  { name: "加入码", icon: "⌁", group: "平台" },
  { name: "规则配置", icon: "⌗", group: "运营" },
  { name: "期次和开奖", icon: "◷", group: "运营" },
  { name: "注单和异常", icon: "▤", group: "运营" },
  { name: "资金与账本", icon: "◈", group: "资金" },
  { name: "批量对账", icon: "≋", group: "资金" },
  { name: "佣金和奖励", icon: "↗", group: "资金" },
  { name: "报表和对账", icon: "▥", group: "管理" },
  { name: "账号与权限", icon: "♧", group: "管理" },
  { name: "审计日志", icon: "≡", group: "管理" },
  { name: "通知投递", icon: "♧", group: "管理" },
  { name: "通知模板", icon: "♧", group: "管理" },
  { name: "风控与合规", icon: "⚖", group: "管理" },
];
const page = ref<Page>("工作台");
const demoBrand = ref("Aurora");
const brandMenu = ref(false);
const mobileMore = ref(false);
const search = ref("");
const notice = ref<ReturnType<typeof message> | string>("");
const showDemoNotice = ref(true);
const api = createAdminApi();
const account = ref<AdminAccount | null>(null);
const authLoading = ref(true);
const authError = ref<string | ReturnType<typeof message>>("");
const loginIdentifier = ref("");
const loginPassword = ref("");
const loginBusy = ref(false);
const loginIdempotencyKey = ref(createIdempotencyKey());
const adminBrands = ref<AdminBrand[]>([]);
const selectedBrandId = ref("");
let adminBrandLoadGeneration = 0;
const correctionSettlementPeriod = ref<{brandId:string;periodId:string;nonce:number}|null>(null);
const brand = computed(
  () =>
    adminBrands.value.find((item) => item.id === selectedBrandId.value)?.name ??
    demoBrand.value,
);
const selectedBrand = computed(() => adminBrands.value.find((item) => item.id === selectedBrandId.value) ?? null);
const presentationApi=createBrandPresentationApi();
const presentationSkin=ref<{accountId:string;record:BrandPresentationRecord}|null>(null);
let presentationReadGeneration=0;
const skin=computed(()=>presentationSkin.value?.accountId===account.value?.id&&presentationSkin.value?.record.brand_id===selectedBrandId.value?presentationSkin.value.record.effective:null);
watch(skin, (presentation) => {
  if (presentation) configure(presentation.default_locale, presentation.available_locales);
  else resetBrand();
}, { immediate: true });
function skinTheme():BrandTheme|null{const e=skin.value;return e?{...defaultBrand,name:e.display_name,logoText:e.logo_text,logoUrl:e.logo_url??undefined,faviconUrl:e.favicon_url??undefined,primary:e.primary_color,accent:e.accent_color,success:e.success_color,warning:e.warning_color,danger:e.danger_color,fontFamily:e.font_family,fontScale:e.font_scale,radius:e.radius,shadow:e.shadow}:null}
const skinStyle=computed(()=>{const theme=skinTheme();return theme?{...buildBrandCssTokens(theme),fontFamily:"var(--font-family)",fontSize:"calc(13px * var(--brand-font-scale-factor, 1))"}:{}});
const skinLogo=computed(()=>safeBrandAssetUrl(skin.value?.logo_url));
const failedSkinLogo=ref<string|null>(null);
watch(skinLogo,()=>{failedSkinLogo.value=null});
function skinLogoError(event:Event){if(event.target instanceof HTMLImageElement&&event.target.getAttribute('src')===skinLogo.value)failedSkinLogo.value=skinLogo.value}
function acceptPresentation(value:{accountId:string;record:BrandPresentationRecord}){if(account.value?.id!==value.accountId||selectedBrandId.value!==value.record.brand_id)return;if(presentationSkin.value?.accountId===value.accountId&&presentationSkin.value.record.brand_id===value.record.brand_id&&presentationSkin.value.record.version>value.record.version)return;presentationSkin.value=value;const theme=skinTheme();if(theme)applyBrandPresentation({brand:theme,paused:value.record.status==='paused',availableLanguages:value.record.effective.available_locales},document.querySelector<HTMLElement>('.app-shell')??document.documentElement)}
watch(()=>[account.value?.id,selectedBrandId.value],()=>{const generation=++presentationReadGeneration;presentationSkin.value=null;document.querySelector('link[rel="icon"][data-brand-favicon]')?.remove();const current=account.value,id=selectedBrandId.value;if(!current||!id||!brandPresentationPermissions(current,id).view)return;void presentationApi.get(id).then(record=>{if(generation===presentationReadGeneration&&account.value?.id===current.id&&selectedBrandId.value===id)acceptPresentation({accountId:current.id,record})}).catch(cause=>{if(generation===presentationReadGeneration&&account.value?.id===current.id&&selectedBrandId.value===id&&cause instanceof AdminApiError&&cause.status===401)clearAdminData()})});
const members = ref<Member[]>([]);
const membersLoading = ref(false);
const membersError = ref<string | ReturnType<typeof message>>("");
const memberOffset = ref(0);
const auditRecords = ref<AdminAuditRecord[]>([]);
const auditLoading = ref(false);
const auditError = ref<string | ReturnType<typeof message>>("");
const editTarget = ref<Member | null>(null);
const editStatus = ref<MemberStatus>("normal");
const editNotes = ref("");
const editReason = ref("");
const editIdempotencyKey = ref("");
const editBusy = ref(false);
const kickTarget = ref<Member | null>(null);
const kickReason = ref("");
const kickIdempotencyKey = ref("");
const kickBusy = ref(false);
const resetTarget = ref<Member | null>(null);
const resetPasswordValue = ref("");
const resetReason = ref("");
const resetImpactConfirmed = ref(false);
const resetIdempotencyKey = ref("");
const resetBusy = ref(false);
interface ContextBrand {
  id: string;
  code: string;
  name: string;
  status: string;
  default_locale: string;
  timezone: string;
  theme: Record<string, unknown> | null;
  config_version: number;
}
const contextBrand = ref<ContextBrand | null>(null);
const contextState = ref<"loading" | "connected" | "unavailable">("loading");
const loadBrandContext = async () => {
  try {
    const response = await fetch("/api/v1/context", {
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
    const body = (await response.json()) as {
      success?: boolean;
      data?: { brand?: ContextBrand };
    };
    if (!response.ok || body.success === false || !body.data?.brand)
      throw new Error("品牌上下文不可用");
    contextBrand.value = body.data.brand;
    contextState.value = "connected";
  } catch {
    contextBrand.value = null;
    contextState.value = "unavailable";
  }
};
const isMobile = ref(
  typeof window !== "undefined" &&
    window.matchMedia("(max-width: 700px)").matches,
);
const updateMobile = () => {
  isMobile.value = window.matchMedia("(max-width: 700px)").matches;
};
onMounted(() => {
  window.addEventListener("resize", updateMobile);
  void loadBrandContext();
  void restoreAdminSession();
});
onUnmounted(() => window.removeEventListener("resize", updateMobile));
const groups = computed(() => [...new Set(nav.map((item) => item.group))]);
const canViewJoinCodes = computed(() => {
  if (!account.value || !selectedBrandId.value) return false;
  if (account.value.super_admin)
    return (account.value.platform_permissions ?? []).includes("join_code.view.platform");
  if (!account.value.brand_ids.includes(selectedBrandId.value)) return false;
  const permissions = account.value.permissions_by_brand === undefined
    ? account.value.permissions ?? []
    : account.value.permissions_by_brand[selectedBrandId.value] ?? [];
  return permissions.includes("join_code.view.brand");
});
const visibleNav = computed(() => nav.filter((item) => item.name !== "加入码" || canViewJoinCodes.value));
const currentIcon = computed(
  () => nav.find((item) => item.name === page.value)?.icon ?? "▦",
);
const go = (target: Page) => {
  page.value = target;
  notice.value = "";
  mobileMore.value = false;
};
const navigateWorkbench = (destination: string) => {
  const target = nav.find((item) => item.name === destination);
  if (target) go(target.name);
};
const toast = (text: string | ReturnType<typeof message>, english?: string) => {
  const next = typeof text === 'string' ? message(text, english ?? shellCopy(text) ?? text) : text;
  notice.value = next;
  const expected = notice.value;
  window.setTimeout(() => {
    if (notice.value === expected) notice.value = "";
  }, 3200);
};
const hasPermission = (permission: string) => {
  if (!account.value) return false;
  const grants =
    account.value.permissions_by_brand?.[selectedBrandId.value] ??
    (account.value.permissions_by_brand ? [] : account.value.permissions);
  return grants.includes(permission);
};
const canEditUsers = computed(() =>
  Boolean(
    account.value &&
      !account.value.super_admin &&
      hasPermission("user.write.brand"),
  ),
);
const canKickUsers = computed(() =>
  Boolean(
    account.value &&
      !account.value.super_admin &&
      hasPermission("user.kick.brand"),
  ),
);
const canResetPasswords = computed(() =>
  Boolean(
    account.value &&
      !account.value.super_admin &&
      hasPermission("user.password_reset.brand"),
  ),
);
const statusLabel: Record<MemberStatus, string> = {
  normal: "正常",
  frozen: "冻结",
  disabled: "已停用",
  expired: "已过期",
  cancelled: "已注销",
};
const statusClass = (status: MemberStatus) =>
  status === "normal"
    ? "badge-success"
    : status === "frozen"
      ? "badge-danger"
      : "badge-neutral";
const apiErrorText = (error: unknown) =>
  error instanceof Error ? message(error.message, error.message) : message("请求失败，请重试", "Request failed. Please try again.");
const clearAdminData = () => {
	clearAllPendingCommissionCycleWrites();
	clearAllPendingCommissionWrites();
	clearAllPendingPresentationWrites();
	clearAllPendingComplianceIntents();
	clearAllPendingBrandDomainsWrites();
	clearAllPendingBrandCreationWrites();
	presentationReadGeneration+=1;presentationSkin.value=null;
  adminBrandLoadGeneration += 1;
  clearAllPendingAgentWrites();
  clearAllPendingJoinCodeWrites();
  clearAllPendingDeliveryRetries();
  clearAllPendingTemplateWrites();
  clearPendingReconciliationWrites();
  clearAllPendingBrandOperationWrites();
  correctionSettlementPeriod.value = null;
  account.value = null;
  adminBrands.value = [];
  selectedBrandId.value = "";
  members.value = [];
  auditRecords.value = [];
};
const loadBrands = async () => {
  const requestedAccountId = account.value?.id;
  const generation = ++adminBrandLoadGeneration;
  if (!requestedAccountId) return;
  authError.value = "";
  try {
    const result = await api.brands();
    if (generation !== adminBrandLoadGeneration || account.value?.id !== requestedAccountId) return;
    adminBrands.value = result.items;
  } catch (error) {
    if (generation !== adminBrandLoadGeneration || account.value?.id !== requestedAccountId) return;
    if (error instanceof AdminApiError && error.status === 401)
      clearAdminData();
    authError.value = apiErrorText(error);
  }
};
const restoreAdminSession = async () => {
  authLoading.value = true;
  try {
    const result = await api.me();
    if (account.value?.id !== result.account.id) {
      adminBrandLoadGeneration += 1;
      clearAllPendingCommissionCycleWrites();
      clearAllPendingBrandCreationWrites();
      clearAllPendingJoinCodeWrites();
      clearAllPendingBrandOperationWrites();
      clearAllPendingPresentationWrites();
      clearAllPendingComplianceIntents();
      clearAllPendingTemplateWrites();
      clearPendingReconciliationWrites();
    }
    account.value = result.account;
    await loadBrands();
  } catch (error) {
    clearAdminData();
    if (!(error instanceof AdminApiError && error.status === 401))
      authError.value = apiErrorText(error);
  } finally {
    authLoading.value = false;
  }
};
const login = async () => {
  if (loginBusy.value) return;
  loginBusy.value = true;
  authError.value = "";
  try {
    await api.login(
      loginIdentifier.value.trim(),
      loginPassword.value,
      loginIdempotencyKey.value,
    );
    loginPassword.value = "";
    const result = await api.me();
    if (account.value?.id !== result.account.id) {
      adminBrandLoadGeneration += 1;
      clearAllPendingCommissionCycleWrites();
      clearAllPendingBrandCreationWrites();
      clearAllPendingJoinCodeWrites();
      clearAllPendingBrandOperationWrites();
      clearAllPendingPresentationWrites();
      clearAllPendingComplianceIntents();
      clearAllPendingTemplateWrites();
      clearPendingReconciliationWrites();
    }
    account.value = result.account;
    selectedBrandId.value = "";
    loginIdempotencyKey.value = createIdempotencyKey();
    await loadBrands();
    toast("已登录管理员账号");
  } catch (error) {
    clearAdminData();
    authError.value = apiErrorText(error);
  } finally {
    loginBusy.value = false;
  }
};
const onBrandOperationChanged = (change: { accountId: string }) => {
  if (account.value?.id === change.accountId) void loadBrands();
};
const onBrandCreated = () => { if (account.value) void loadBrands(); };
const logout = async () => {
  const key = createIdempotencyKey();
  // Invalidate mounted views and their pending downloads immediately. A slow
  // server revocation must not keep an old authenticated view alive locally.
  clearAdminData();
  try {
    await api.logout(key);
  } catch (error) {
    authError.value = apiErrorText(error);
  }
  toast("已退出管理员账号");
};
const selectBrand = async (brandId: string) => {
  selectedBrandId.value = brandId;
  brandMenu.value = false;
  memberOffset.value = 0;
  members.value = [];
  auditRecords.value = [];
  await Promise.all([loadMembers(), loadAudit()]);
};
const loadMembers = async () => {
  if (!account.value || !selectedBrandId.value) return;
  membersLoading.value = true;
  membersError.value = "";
  try {
    members.value = (
      await api.users(selectedBrandId.value, 100, memberOffset.value)
    ).items;
  } catch (error) {
    membersError.value = apiErrorText(error);
    if (error instanceof AdminApiError && error.status === 401)
      clearAdminData();
  } finally {
    membersLoading.value = false;
  }
};
const loadAudit = async () => {
  if (!account.value || !selectedBrandId.value) return;
  auditLoading.value = true;
  auditError.value = "";
  try {
    auditRecords.value = (await api.audit(selectedBrandId.value)).items;
  } catch (error) {
    auditError.value = apiErrorText(error);
    if (error instanceof AdminApiError && error.status === 401)
      clearAdminData();
  } finally {
    auditLoading.value = false;
  }
};
const openEdit = (member: Member) => {
  editTarget.value = member;
  editStatus.value = member.status;
  editNotes.value = member.notes;
  editReason.value = "";
  editIdempotencyKey.value = createIdempotencyKey();
};
const saveMember = async () => {
  if (
    !editTarget.value ||
    !selectedBrandId.value ||
    !editReason.value.trim() ||
    editBusy.value
  )
    return;
  editBusy.value = true;
  try {
    await api.updateUser(
      selectedBrandId.value,
      editTarget.value.id,
      {
        status: editStatus.value,
        notes: editNotes.value,
        reason: editReason.value.trim(),
      },
      editIdempotencyKey.value,
    );
    editTarget.value = null;
    await Promise.all([loadMembers(), loadAudit()]);
    toast("成员资料已更新");
  } catch (error) {
    toast(apiErrorText(error));
  } finally {
    editBusy.value = false;
  }
};
const openKick = (member: Member) => {
  kickTarget.value = member;
  kickReason.value = "";
  kickIdempotencyKey.value = createIdempotencyKey();
};
const kickMember = async () => {
  if (
    !kickTarget.value ||
    !selectedBrandId.value ||
    !kickReason.value.trim() ||
    kickBusy.value
  )
    return;
  kickBusy.value = true;
  try {
    await api.kickUser(
      selectedBrandId.value,
      kickTarget.value.id,
      kickReason.value.trim(),
      kickIdempotencyKey.value,
    );
    kickTarget.value = null;
    await Promise.all([loadMembers(), loadAudit()]);
    toast("该成员的品牌会话已踢出");
  } catch (error) {
    toast(apiErrorText(error));
  } finally {
    kickBusy.value = false;
  }
};
const openReset = (member: Member) => {
  resetTarget.value = member;
  resetPasswordValue.value = "";
  resetReason.value = "";
  resetImpactConfirmed.value = false;
  resetIdempotencyKey.value = createIdempotencyKey();
};
const resetMemberPassword = async () => {
  if (
    !resetTarget.value ||
    !selectedBrandId.value ||
    !resetPasswordValue.value ||
    !resetReason.value.trim() ||
    !resetImpactConfirmed.value ||
    resetBusy.value
  )
    return;
  resetBusy.value = true;
  try {
    await api.resetPassword(
      selectedBrandId.value,
      resetTarget.value.id,
      resetPasswordValue.value,
      resetReason.value.trim(),
      resetIdempotencyKey.value,
    );
    resetTarget.value = null;
    await Promise.all([loadMembers(), loadAudit()]);
    toast("全局密码已重置，所有品牌会话已撤销");
  } catch (error) {
    toast(apiErrorText(error));
  } finally {
    resetBusy.value = false;
  }
};
const changeMemberPage = async (direction: -1 | 1) => {
  memberOffset.value = Math.max(0, memberOffset.value + direction * 100);
  await loadMembers();
};
const ruleDraft = ref<RuleDraft>({
  creator: "林岚",
  reviewer: "",
  status: "draft",
  testSelection: "07 18 29",
  testResult: "07 12 29",
  unitPoints: "2",
  multiplier: "3",
});
const ruleStep = ref(1);
const simulation = computed(() => simulateRule(ruleDraft.value));
const simulate = () =>
  toast(
    `模拟完成：${simulation.value.explanation}，预计积分 ${simulation.value.points}`,
  );
const submitRule = () => {
  try {
    ruleDraft.value = submitRuleForReview({
      ...ruleDraft.value,
      reviewer: "周宁",
    });
    toast("规则已提交给周宁审核（演示）");
  } catch (error) {
    toast(error instanceof Error ? error.message : "提交失败");
  }
};
const decisionRule = (decision: "approve" | "reject") => {
  try {
    ruleDraft.value = reviewRule(ruleDraft.value, "周宁", decision);
    toast(
      decision === "approve"
        ? "审核通过；待发布确认（演示）"
        : "规则已驳回（演示）",
    );
  } catch (error) {
    toast(error instanceof Error ? error.message : "审核失败");
  }
};
const periodResult = ref("07 18 29 33 41 + 06");
const correctionResult = ref("07 18 29 33 41 + 09");
const correctionReason = ref("");
const correctionOpen = ref(false);
const correctionPreview = computed(() =>
  previewCorrection(periodResult.value, correctionResult.value, 126),
);
const correctResult = () => {
  if (!correctionReason.value.trim()) return toast("纠正原因必填");
  toast(
    `纠正已创建：预计回溯 ${correctionPreview.value.affectedOrders} 笔注单、重新结算 ${correctionPreview.value.estimatedSettlements} 项（演示）`,
  );
  periodResult.value = correctionResult.value;
  correctionOpen.value = false;
  correctionReason.value = "";
};
const manualOpen = ref(false);
const manualReason = ref("");
const confirmManual = () => {
  if (!manualReason.value.trim()) return toast("人工开奖原因必填");
  toast("人工结果已锁定本期（演示）");
  manualOpen.value = false;
  manualReason.value = "";
};
const ledgerTab = ref("账本流水");
const ledger = [
  {
    id: "LE-883140",
    type: "投注扣减",
    source: "注单 BO-61048219",
    before: "2,495",
    change: "-15",
    after: "2,480",
    actor: "系统演示",
    time: "10-05 10:26",
  },
  {
    id: "LE-883102",
    type: "中奖入账",
    source: "注单 BO-61047212",
    before: "2,483",
    change: "+12",
    after: "2,495",
    actor: "结算演示",
    time: "10-05 08:10",
  },
  {
    id: "LE-882988",
    type: "人工充值",
    source: "RC-2601041",
    before: "483",
    change: "+2,000",
    after: "2,483",
    actor: "周宁",
    time: "10-04 18:43",
  },
];
</script>

<template>
  <div class="app-shell" :style="skinStyle">
    <aside class="sidebar">
      <a class="brand-lockup" href="#" @click.prevent="go('工作台')"
        ><img v-if="skinLogo&&failedSkinLogo!==skinLogo" class="presentation-logo" :src="skinLogo" :alt="skin?.logo_text" crossorigin="anonymous" referrerpolicy="no-referrer" @error="skinLogoError"/>
        <span v-else class="brand-mark">{{skin?.logo_text??'N'}}</span>
        <span><b>{{skin?.display_name??'northstar'}}</b><small>OPERATIONS CONSOLE</small></span></a
      >
      <div v-if="page !== '工作台'" class="demo-chip">
        <span class="pulse"></span>{{ ui("真实提现状态接口；资格规则待配置，无真实支付") }} <span class="demo-chip-end">·</span>
      </div>
      <div class="brand-switch-wrap">
        <button
          class="brand-switch"
          @click="brandMenu = !brandMenu"
          :aria-label="ui('切换品牌')"
        >
          <span class="brand-avatar">{{ brand.slice(0, 1) }}</span
          ><span class="brand-switch-text"
            ><small>{{ account ? ui("真实品牌") : ui("演示品牌") }}</small
            ><b>{{ account && !selectedBrandId ? ui("选择品牌") : brand }}</b></span
          ><span class="chevron">⌄</span>
        </button>
        <div v-if="brandMenu" class="brand-dropdown">
          <template v-if="account"
            ><button
              v-for="item in adminBrands"
              :key="item.id"
              @click="selectBrand(item.id)"
            >
              {{ item.name }} <span v-if="selectedBrandId === item.id">✓</span
              ><small>{{ item.code }} · {{ item.status }}</small>
            </button>
            <p v-if="!adminBrands.length">{{ ui("没有可访问的品牌") }}</p></template
          ><template v-else
            ><button
              v-for="name in ['Aurora', 'Harbor']"
              :key="name"
              @click="
                demoBrand = name;
                brandMenu = false;
                toast(`当前本地原型品牌：${name}`);
              "
            >
              {{ name }}<small>{{ ui("仅原型演示") }}</small>
            </button></template
          >
        </div>
      </div>
      <nav class="side-nav" :aria-label="ui('主导航')">
        <template v-for="group in groups" :key="group"
          ><p class="nav-heading">{{ ui(group) }}</p>
          <button
            v-for="item in visibleNav.filter((entry) => entry.group === group)"
            :key="item.name"
            class="nav-item"
            :class="{ active: page === item.name }"
            @click="go(item.name)"
          >
            <span class="nav-icon">{{ item.icon }}</span
            ><span>{{ ui(item.name) }}</span
            >
          </button></template
        >
      </nav>
      <div class="sidebar-bottom">
        <button class="support-link" @click="toast('帮助中心为演示入口')">
          ◌ <span>{{ ui("帮助与反馈") }}</span><span class="external">↗</span>
        </button>
        <div class="profile">
          <div class="profile-avatar">
            {{ account ? account.id.slice(0, 1).toUpperCase() : ui("演") }}
          </div>
          <span class="profile-name"
            ><b>{{ account?.id ?? ui("未登录") }}</b
            ><small>{{
              account
                ? account.super_admin
                  ? ui("超级管理员")
                  : ui("管理员")
                : ui("原型演示")
            }}</small></span
          ><button
            v-if="account"
            class="dots"
            :aria-label="ui('退出登录')"
            @click="logout"
          > {{ ui("退出") }}</button
          ><button
            v-else
            class="dots"
            :aria-label="ui('账号菜单')"
            @click="go('账号与权限')"
          >
            ···
          </button>
        </div>
      </div>
    </aside>

    <main class="main-shell">
      <header class="topbar">
        <div class="breadcrumbs">
          <span>{{ ui("运营控制台") }}</span><span class="crumb-slash">/</span
          ><b>{{ ui(page) }}</b
          ><span
            v-if="
              !(account && (page === '品牌和域名' || page === '风控与合规')) &&
              page !== '工作台' &&
              page !== '用户和成员' &&
              page !== '审计日志' &&
              page !== '通知投递' &&
              page !== '通知模板' &&
              page !== '报表和对账' &&
              page !== '代理树' &&
              page !== '资金与账本' &&
              page !== '批量对账' &&
              page !== '账号与权限' &&
              page !== '加入码'
            "
            class="prototype-badge"
            >{{ ui("演示原型") }}</span
          >
        </div>
        <div class="top-actions">
          <label class="admin-language">
            <span class="sr-only">{{ t('语言', 'Language') }}</span>
            <select data-testid="admin-language" :aria-label="t('语言', 'Language')" :value="locale" @change="setLocale(($event.target as HTMLSelectElement).value)">
              <option v-for="code in availableLocales" :key="code" :value="code">{{ code === 'en' ? 'English' : '简体中文' }}</option>
            </select>
          </label>
          <label class="search-box"
            ><span>⌕</span
            ><input
              v-model="search"
              :placeholder="ui('搜索用户、注单、期次')"
              :aria-label="ui('搜索')"
            /><kbd>⌘ K</kbd></label
          ><button
            class="icon-button"
            :aria-label="ui('通知')"
            @click="go('通知投递')"
          >
            ♧</button
          ><span class="top-divider"></span
          ><button class="help-button" @click="toast('帮助中心为演示入口')"> {{ ui("帮助中心 ↗") }} </button>
        </div>
      </header>
      <div v-if="locale === 'en'" class="admin-locale-coverage" role="status">
        {{ t("部分功能面板来自独立模块，可能仍显示中文。", "Some feature panels are provided by separate modules and may still appear in Chinese.") }}
      </div>
      <div
        v-if="
          showDemoNotice &&
          !(account && (page === '品牌和域名' || page === '风控与合规')) &&
          page !== '工作台' &&
          page !== '用户和成员' &&
          page !== '审计日志' &&
          page !== '通知投递' &&
          page !== '通知模板' &&
          page !== '报表和对账' &&
          page !== '代理树' &&
          page !== '资金与账本' &&
          page !== '注单和异常' &&
          page !== '账号与权限' &&
          page !== '加入码'
        "
        class="demo-banner"
      >
        <span class="banner-icon">ⓘ</span
        ><span
          ><b>{{ ui("交互演示 · 非生产环境") }}</b
          ><span class="banner-copy"> {{ ui("控制台包含已接入流程与原型。提现状态接口已接入，资格规则待配置且不支持真实支付；佣金策略和周期核算已接入，审核和派发尚未实现。") }}</span
          ></span
        ><button :aria-label="ui('关闭说明')" @click="showDemoNotice = false">
          ×
        </button>
      </div>
      <div class="context-strip" :class="contextState">
        <span class="context-indicator"></span
        ><template v-if="contextState === 'connected' && contextBrand"
          ><b>{{ ui("后端品牌上下文") }}</b
          ><span>{{ contextBrand.name }} · {{ contextBrand.code }}</span
          ><span>{{ contextBrand.status }}</span
          ><span>{{ contextBrand.default_locale }}</span
          ><span>{{ contextBrand.timezone }}</span
          ><span>{{ ui("配置版本") }} {{ contextBrand.config_version }}</span></template
        ><template v-else-if="contextState === 'loading'"
          ><b>{{ ui("后端品牌上下文") }}</b
          ><span>{{ ui("正在读取 GET /api/v1/context") }}</span></template
        ><template v-else-if="account && !authLoading && !authError"
          ><b>{{ ui("平台品牌上下文") }}</b
          ><span>{{ ui("当前入口没有可用的公开用户品牌上下文；后台管理认证独立，不受影响。请选择需要管理的品牌。") }}</span></template
        ><template v-else
          ><b>{{ ui("后端品牌上下文未连接") }}</b
          ><span
            >{{ ui("只读接口 GET /api/v1/context 暂不可用；该接口仅用于只读配置状态，不影响管理员认证和真实用户目录。") }}</span
          ></template
        >
      </div>
      <div v-if="account" class="directory-brand-bar">
        <span>{{ ui("真实后台品牌") }}</span
        ><select
          :value="selectedBrandId"
          :aria-label="ui('选择真实后台品牌')"
          @change="selectBrand(($event.target as HTMLSelectElement).value)"
        >
          <option value="" disabled>{{ ui("请选择品牌") }}</option>
          <option v-for="item in adminBrands" :key="item.id" :value="item.id">
            {{ item.name }} · {{ item.code }}
          </option></select
        ><small v-if="selectedBrandId">X-Brand-ID: {{ selectedBrandId }}</small
        ><button class="text-button" @click="logout">{{ ui("退出登录") }}</button>
      </div>
      <div v-if="notice" class="toast" role="status">✓ &nbsp;{{ t(notice) }}</div>

      <section v-if="page === '工作台'" class="page-content">
        <OperationsWorkbench
          v-if="account && selectedBrandId"
          :key="account.id + ':' + selectedBrandId"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
          @navigate="navigateWorkbench"
        />
        <article v-else class="panel directory-state">
          <h1>{{ ui("运营工作台") }}</h1>
          <p v-if="authLoading">{{ ui("正在检查管理员登录状态…") }}</p>
          <template v-else-if="!account">
            <p>{{ ui("请先登录后台账号查看运营工作台。") }}</p>
            <button class="button button-primary" @click="go('用户和成员')">{{ ui("进入管理员登录") }}</button>
          </template>
          <p v-else>{{ adminBrands.length ? ui("请先选择真实后台品牌查看运营工作台。") : ui("当前账号未返回可管理品牌。") }}</p>
        </article>
      </section>

      <section v-else-if="page === '风控与合规'" class="page-content">
        <CompliancePolicy v-if="account && selectedBrandId" :key="'compliance:' + account.id + ':' + selectedBrandId" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <article v-else class="panel directory-state"><h2>{{ ui("选择品牌") }}</h2><p>{{ account ? ui("请先选择真实品牌。") : ui("请先登录后台账号并选择真实品牌。") }}</p></article>
      </section>
      <section v-else-if="page === '品牌和域名'" class="page-content">
        <AuthSettings
          v-if="account && selectedBrandId"
          :key="selectedBrandId"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <template v-if="account">
          <div class="page-heading">
            <div>
              <div class="eyebrow">PLATFORM / BRAND CONFIG</div>
              <h1>{{ ui("品牌和域名") }}</h1>
              <p>{{ ui("真实品牌列表、运行状态、展示与域名绑定") }}</p>
            </div>
          </div>
          <BrandCreation v-if="account" :key="`create:${account.id}`" :account="account" @session-invalid="clearAdminData" @created="onBrandCreated" />
          <BrandOperation
            v-if="selectedBrandId"
            :key="`${account.id}:${selectedBrandId}`"
            :account="account"
            :brand-id="selectedBrandId"
            :brand-status="selectedBrand?.status"
            @session-invalid="clearAdminData"
            @changed="onBrandOperationChanged"
          />
          <article v-else class="panel brand-operation-empty">
            <h2>{{ ui("选择一个品牌") }}</h2>
            <p>{{ ui("选择下方真实品牌后，可查看其运行状态与操作记录。") }}</p>
          </article>
          <BrandPresentation v-if="selectedBrandId" :key="`${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" @loaded="acceptPresentation"/>
          <BrandDomains v-if="selectedBrandId" :key="`domains:${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData"/>
          <article class="panel brand-operation-brands">
            <div class="panel-header">
              <div><h2>{{ ui("真实品牌") }}</h2><p>{{ ui("来自管理员品牌接口") }}</p></div>
              <button class="button button-secondary" @click="loadBrands">{{ ui("刷新列表") }}</button>
            </div>
            <p class="brand-operation-unavailable">{{ ui("域名绑定不配置 DNS、证书或重定向；展示仅支持固定预设与中英文文案，不支持任意 CSS、HTML 或上传素材。") }}</p>
            <p v-if="authLoading" class="directory-state">{{ ui("正在读取品牌…") }}</p>
            <p v-else-if="authError" class="directory-state" role="alert">{{ t(authError) }}</p>
            <p v-else-if="!adminBrands.length" class="directory-state">{{ ui("当前账号未返回可管理品牌。") }}</p>
            <ul v-else class="brand-operation-list">
              <li v-for="item in adminBrands" :key="item.id">
                <div class="brand-operation-list__identity">
                  <span class="brand-operation-list__mark" aria-hidden="true">{{ item.name.slice(0, 1) || ui("品") }}</span>
                  <span><strong>{{ item.name }}</strong><small>{{ item.code }} · {{ item.id }}</small></span>
                </div>
                <span class="badge" :class="item.status === 'active' ? 'badge-success' : 'badge-neutral'">{{ item.status === 'active' ? ui("运行中") : item.status === 'paused' ? ui("已暂停") : item.status === 'disabled' ? ui("已停用") : item.status }}</span>
                <button class="text-button" type="button" :aria-pressed="selectedBrandId === item.id" @click="selectBrand(item.id)">{{ selectedBrandId === item.id ? ui("当前品牌") : ui("查看运行状态 →") }}</button>
              </li>
            </ul>
          </article>
        </template>
        <template v-else>
          <div class="page-heading">
            <div>
              <div class="eyebrow">PLATFORM / BRAND CONFIG</div>
              <h1>{{ ui("品牌和域名") }}</h1>
              <p>{{ ui("管理品牌主题、域名及生效版本") }}</p>
            </div>
            <button
              class="button button-primary"
              @click="toast('新建品牌为演示入口')"
            > {{ ui("＋ 新建品牌") }} </button>
          </div>
          <article class="panel">
            <div class="table-toolbar">
              <div class="filter-tabs">
                <button class="selected">{{ ui("全部品牌") }} <span>3</span></button
                ><button>{{ ui("运行中") }}</button><button>{{ ui("已暂停") }}</button>
              </div>
              <button
                class="button button-secondary"
                @click="toast('配置导出成功（演示）')"
              > {{ ui("导出配置 ↓") }} </button>
            </div>
            <div class="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>{{ ui("品牌") }}</th>
                    <th>{{ ui("主域名") }}</th>
                    <th>{{ ui("状态") }}</th>
                    <th>{{ ui("默认语言") }}</th>
                    <th>{{ ui("时区") }}</th>
                    <th>{{ ui("配置版本") }}</th>
                    <th>{{ ui("操作") }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="b in [
                      { name: 'Aurora', domain: 'aurora.demo', state: '运行中', locale: '简体中文', tz: 'Asia/Singapore', version: 'v12' },
                      { name: 'Harbor · 月湾', domain: 'harbor.demo', state: '运行中', locale: 'English', tz: 'Asia/Kuala_Lumpur', version: 'v8' },
                      { name: 'Harbor Secondary · 晴野', domain: 'sunfield.demo', state: '已暂停', locale: '繁體中文', tz: 'Asia/Taipei', version: 'v4' },
                    ]"
                    :key="b.name"
                  >
                    <td><span class="table-brand"><i>{{ b.name[0] }}</i><b>{{ b.name }}</b></span></td>
                    <td class="mono">{{ b.domain }}</td>
                    <td><span class="badge" :class="b.state === '运行中' ? 'badge-success' : 'badge-neutral'">{{ b.state }}</span></td>
                    <td>{{ b.locale }}</td><td>{{ b.tz }}</td>
                    <td><span class="version-tag">{{ b.version }}</span></td>
                    <td><button class="text-button" @click="toast(`打开 ${b.name} 配置（演示）`)">{{ ui("管理 →") }}</button></td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="panel-foot">{{ ui("平台默认 → 品牌覆盖　·　配置发布后清理缓存并返回新版本号") }}<span>{{ ui("数据仅用于演示") }}</span></div>
          </article>
        </template>
      </section>

      <section
        v-else-if="page === '用户和成员'"
        class="page-content directory-page"
      >
        <div class="page-heading">
          <div>
            <div class="eyebrow">MEMBERS / LIVE DIRECTORY</div>
            <h1>{{ ui("用户和成员") }}</h1>
            <p> {{ ui("此页显示真实成员；账号权限、积分账本、认证设置和投注已接入。") }} </p>
          </div>
          <span v-if="account" class="live-pill"
            >{{ ui("已登录 ·") }} {{ account.id }}</span
          >
        </div>
        <div v-if="authLoading" class="panel directory-state"> {{ ui("正在检查管理员登录状态…") }} </div>
        <article v-else-if="!account" class="panel auth-panel">
          <div class="auth-copy">
            <div class="eyebrow">ADMIN AUTHENTICATION</div>
            <h2>{{ ui("管理员登录") }}</h2>
            <p> {{ ui("使用后台管理员账号登录。会话由同源 HttpOnly Cookie 维护，此页面不会保存访问令牌。") }} </p>
          </div>
          <form class="auth-form" @submit.prevent="login">
            <label class="modal-label"
              >{{ ui("账号") }}<input
                v-model="loginIdentifier"
                class="field"
                autocomplete="username"
                required /></label
            ><label class="modal-label"
              >{{ ui("密码") }}<input
                v-model="loginPassword"
                class="field"
                type="password"
                autocomplete="current-password"
                required
            /></label>
            <p v-if="authError" class="form-error" role="alert">
              {{ t(authError) }}
            </p>
            <button class="button button-primary" :disabled="loginBusy">
              {{ loginBusy ? ui("登录中…") : ui("登录并加载真实成员") }}
            </button>
          </form>
        </article>
        <template v-else>
          <div v-if="authError" class="directory-error" role="alert">
            {{ t(authError) }}
          </div>
          <div v-if="!selectedBrandId" class="panel directory-state">
            <b>{{ ui("请选择真实品牌") }}</b
            ><span
              >{{ ui("成员请求不会使用后端默认品牌。请从侧栏品牌选择器中选择一个有权访问的品牌。") }}</span
            ><button class="button button-secondary" @click="brandMenu = true"> {{ ui("选择品牌") }} </button>
          </div>
          <template v-else>
            <MemberProvision
              :key="selectedBrandId"
              :account="account"
              :brand-id="selectedBrandId"
              @created="loadMembers"
              @session-invalid="clearAdminData"
            />
            <div class="directory-toolbar">
              <div>
                <span>{{ ui("当前真实品牌") }}</span><b>{{ brand }}</b
                ><small>{{ selectedBrandId }}</small>
              </div>
              <button
                class="button button-secondary"
                :disabled="membersLoading"
                @click="loadMembers"
              >
                {{ membersLoading ? ui("读取中…") : ui("刷新成员") }}
              </button>
            </div>
            <div v-if="membersError" class="directory-error" role="alert">
              {{ t(membersError) }}
            </div>
            <article class="panel">
              <div class="table-toolbar">
                <div class="filter-tabs">
                  <span class="selected"
                    >{{ ui("品牌成员") }} <span>{{ members.length }} {{ ui("条本页") }}</span></span
                  >
                </div>
                <div class="toolbar-controls">
                  <input
                    class="field search-field"
                    :placeholder="ui('搜索用户名 / 手机号 / ID')"
                    v-model="search"
                    :aria-label="ui('搜索成员')"
                  />
                </div>
              </div>
              <div v-if="membersLoading" class="directory-state"> {{ ui("正在读取品牌成员…") }} </div>
              <div v-else-if="!members.length" class="directory-state"> {{ ui("该品牌当前没有可显示的成员。") }} </div>
              <div v-else class="table-wrap">
                <table class="member-directory-table">
                  <thead>
                    <tr>
                      <th>{{ ui("成员") }}</th>
                      <th>{{ ui("成员 ID") }}</th>
                      <th>{{ ui("手机号") }}</th>
                      <th>{{ ui("状态") }}</th>
                      <th>{{ ui("加入时间") }}</th>
                      <th>{{ ui("备注") }}</th>
                      <th>{{ ui("标签") }}</th>
                      <th>{{ ui("操作") }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="m in members.filter(
                        (item) =>
                          !search ||
                          `${item.display_name} ${item.username} ${item.phone} ${item.id} ${item.global_user_id}`
                            .toLowerCase()
                            .includes(search.toLowerCase()),
                      )"
                      :key="m.id"
                    >
                      <td>
                        <span class="member-cell"
                          ><i>{{
                            (m.display_name || m.username || "?").slice(0, 1)
                          }}</i
                          ><span
                            ><b>{{ m.display_name || m.username }}</b
                            ><small
                              >@{{ m.username }} · {{ m.global_user_id }}</small
                            ></span
                          ></span
                        >
                      </td>
                      <td class="mono">{{ m.id }}</td>
                      <td>{{ m.phone || "—" }}</td>
                      <td>
                        <span class="badge" :class="statusClass(m.status)">{{
                          ui(statusLabel[m.status])
                        }}</span>
                      </td>
                      <td>{{ new Date(m.joined_at).toLocaleString() }}</td>
                      <td class="member-notes">{{ m.notes || "—" }}</td>
                      <td>
                        <span
                          v-for="tag in m.tags"
                          :key="tag"
                          class="member-tag"
                          >{{ tag }}</span
                        ><span v-if="!m.tags.length">—</span>
                      </td>
                      <td>
                        <div class="member-actions">
                          <button
                            class="text-button"
                            :disabled="!canEditUsers"
                            :title="
                              account.super_admin
                                ? '超级管理员不可编辑成员状态或备注'
                                : !canEditUsers
                                  ? '缺少 user.write.brand 权限'
                                  : ''
                            "
                            @click="openEdit(m)"
                          > {{ ui("编辑") }}</button
                          ><button
                            class="text-button"
                            :disabled="!canKickUsers"
                            :title="
                              account.super_admin
                                ? '超级管理员不可踢出成员'
                                : !canKickUsers
                                  ? '缺少 user.kick.brand 权限'
                                  : ''
                            "
                            @click="openKick(m)"
                          > {{ ui("踢出") }}</button
                          ><button
                            class="text-button"
                            :disabled="!canResetPasswords"
                            :title="
                              !canResetPasswords
                                ? '缺少 user.password_reset.brand 权限'
                                : ''
                            "
                            @click="openReset(m)"
                          > {{ ui("重置密码") }} </button>
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <div class="pagination">
                <span
                  >{{ ui("第") }} {{ Math.floor(memberOffset / 100) + 1 }} {{ ui("页 · 每页最多 100 条") }}</span
                ><button
                  :disabled="memberOffset === 0 || membersLoading"
                  @click="changeMemberPage(-1)"
                > {{ ui("上一页") }}</button
                ><button
                  :disabled="members.length < 100 || membersLoading"
                  @click="changeMemberPage(1)"
                > {{ ui("下一页") }} </button>
              </div>
            </article>
          </template>
        </template>
      </section>

      <section v-else-if="page === '代理树'" class="page-content">
        <AgentManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <CommissionPolicySettings v-if="account && selectedBrandId" :key="`commission-policy:${account.id}:${selectedBrandId}`"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ ui("代理管理") }}</h1>
          <p>{{ account ? ui("请先选择真实后台品牌。") : ui("请先登录后台账号查看代理配置。") }}</p>
          <button v-if="!account" class="button button-primary" @click="go('用户和成员')">{{ ui("进入管理员登录") }}</button>
        </div>
      </section>

      <section v-else-if="page === '加入码'" class="page-content">
        <JoinCodeManagement v-if="account && selectedBrandId && canViewJoinCodes" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ ui("加入码管理") }}</h1>
          <p>{{ !account ? ui("请先登录后台账号。") : !selectedBrandId ? ui("请先选择真实后台品牌。") : ui("当前账号没有此品牌的加入码查看权限。") }}</p>
          <button v-if="!account" class="button button-primary" @click="go('用户和成员')">{{ ui("进入管理员登录") }}</button>
        </div>
      </section>

      <section v-else-if="page === '规则配置'" class="page-content">
        <RuleVersions
          v-if="account && selectedBrandId"
          :key="`rule-versions-${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <RuleSimulator
          v-if="account && selectedBrandId"
          :key="`rule-simulator-${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else class="panel directory-state">
          <h2>{{ ui("真实规则模拟") }}</h2>
          <p>{{ ui("请先登录后台并选择品牌；下面的规则审核向导仍为演示。") }}</p>
        </div>
        <details class="demo-rule-workbench">
          <summary>{{ ui("旧规则向导演示（不会保存、发布或审核真实规则）") }}</summary>
          <div class="page-heading">
            <div>
              <div class="eyebrow">GAMES / RULE WORKBENCH</div>
              <h1>{{ ui("规则配置") }}</h1>
              <p>{{ ui("创建、模拟并提交不可变规则版本供品牌管理员审核") }}</p>
            </div>
            <button
              class="button button-secondary"
              @click="toast('已打开规则版本列表（演示）')"
            > {{ ui("版本历史 ↗") }} </button>
          </div>
          <div class="rule-layout">
            <article class="panel rule-editor">
              <div class="rule-top">
                <div>
                  <span class="badge badge-neutral">{{ ui("草稿 · v13") }}</span>
                  <h2>{{ ui("星彩 6+1 · 特别号命中") }}</h2>
                  <p> {{ ui("适用品牌：") }}{{ brand }} <span>·</span> {{ ui("生效方式：下一期期次") }} </p>
                </div>
                <button
                  class="save-status"
                  @click="toast('草稿保存在本次页面会话中')"
                > {{ ui("● 自动保存演示") }} </button>
              </div>
              <div v-if="isMobile" class="mobile-readonly"> {{ ui("⌁ 移动端规则编辑为只读，可查看模拟并进行审核操作。") }} </div>
              <div class="stepper">
                <button
                  v-for="i in 8"
                  :key="i"
                  :class="{ done: i < ruleStep, active: i === ruleStep }"
                  @click="ruleStep = i"
                >
                  <span>{{ i < ruleStep ? "✓" : i }}</span
                  ><small>{{
                    [
                      ui("彩种模型"),
                      ui("号码配置"),
                      ui("选号规则"),
                      ui("中奖条件"),
                      ui("奖级赔率"),
                      ui("限额取消"),
                      ui("测试案例"),
                      ui("模拟结果"),
                    ][i - 1]
                  }}</small>
                </button>
              </div>
              <fieldset :disabled="isMobile" class="rule-fieldset">
                <template v-if="ruleStep === 1"
                  ><div class="form-section">
                    <h3>{{ ui("1. 选择彩种模型") }}</h3>
                    <p>{{ ui("模型确定号码池结构和基本校验方式。") }}</p>
                    <div class="model-options">
                      <button class="model-option selected" @click.prevent>
                        <b>X + Y</b><small>{{ ui("普通号码与特别号码分别选择") }}</small
                        ><span>{{ ui("当前模型") }}</span></button
                      ><button class="model-option" @click.prevent>
                        <b>{{ ui("M 选 N") }}</b
                        ><small>{{ ui("从号码池中选取指定数量") }}</small></button
                      ><button class="model-option" @click.prevent>
                        <b>{{ ui("0–9 数字位") }}</b><small>{{ ui("按位置选择有序数字") }}</small>
                      </button>
                    </div>
                  </div>
                  <div class="form-grid">
                    <label
                      >{{ ui("彩种名称") }}<input class="field" value="星彩 6+1" /></label
                    ><label
                      >{{ ui("玩法名称") }}<input class="field" value="特别号命中"
                    /></label></div
                ></template>
                <template v-else-if="ruleStep === 2"
                  ><div class="form-section">
                    <h3>{{ ui("2. 配置号码池和位数") }}</h3>
                    <p>{{ ui("设置合法取值、重复规则及普通 / 特别号码数量。") }}</p>
                  </div>
                  <div class="form-grid">
                    <label
                      >{{ ui("普通号码范围") }}<input class="field" value="1 – 49" /></label
                    ><label>{{ ui("普通号码个数") }}<input class="field" value="6" /></label
                    ><label
                      >{{ ui("特别号码范围") }}<input class="field" value="1 – 10" /></label
                    ><label
                      >{{ ui("特别号码个数") }}<input class="field" value="1"
                    /></label>
                  </div>
                  <label class="check-row"
                    ><input type="checkbox" checked /> {{ ui("不允许号码重复") }}</label
                  ></template
                >
                <template v-else-if="ruleStep === 3"
                  ><div class="form-section">
                    <h3>{{ ui("3. 配置选号与组合方式") }}</h3>
                    <p>{{ ui("设置排除、复式展开、号码属性和倍投规则。") }}</p>
                  </div>
                  <div class="form-grid">
                    <label
                      >{{ ui("选号方式") }}<select class="field">
                        <option value="手动选号 / 复式">{{ ui("手动选号 / 复式") }}</option>
                        <option value="单式选号">{{ ui("单式选号") }}</option>
                      </select></label
                    ><label
                      >{{ ui("单注积分") }}<input
                        class="field"
                        v-model="ruleDraft.unitPoints"
                        inputmode="numeric" /></label
                    ><label>{{ ui("倍数下限") }}<input class="field" value="1" /></label
                    ><label>{{ ui("倍数上限") }}<input class="field" value="100" /></label>
                  </div>
                  <label class="check-row"
                    ><input type="checkbox" checked /> {{ ui("支持排除号码") }}　　<input
                      type="checkbox"
                      checked
                    /> {{ ui("支持奇偶属性") }}</label
                  ></template
                >
                <template v-else-if="ruleStep === 4"
                  ><div class="form-section">
                    <h3>{{ ui("4. 配置中奖条件和结果特征") }}</h3>
                    <p>{{ ui("仅允许使用规则组件，不执行自定义脚本。") }}</p>
                  </div>
                  <div class="condition-row">
                    <span class="condition-index">01</span
                    ><select class="field">
                      <option value="特别号码命中">{{ ui("特别号码命中") }}</option></select
                    ><select class="field">
                      <option value="等于">{{ ui("等于") }}</option>
                      <option value="包含">{{ ui("包含") }}</option></select
                    ><span class="condition-value">{{ ui("特别号码") }}</span
                    ><button class="more-action" @click.prevent>···</button>
                  </div>
                  <button class="button button-secondary" @click.prevent> {{ ui("＋ 添加条件组件") }} </button></template
                >
                <template v-else-if="ruleStep === 5"
                  ><div class="form-section">
                    <h3>{{ ui("5. 配置奖级、赔率和舍入") }}</h3>
                    <p>{{ ui("奖级优先级、排他性、封顶值和舍入策略。") }}</p>
                  </div>
                  <div class="tier-row">
                    <b>{{ ui("特别号命中") }}</b><span>{{ ui("倍率") }}</span
                    ><input class="field" value="40" /><label
                      ><input type="checkbox" checked /> {{ ui("排他") }}</label
                    ><input class="field" value="封顶 50,000" />
                  </div>
                  <div class="tier-row">
                    <b>{{ ui("普通号命中") }}</b><span>{{ ui("倍率") }}</span
                    ><input class="field" value="2" /><label
                      ><input type="checkbox" /> {{ ui("排他") }}</label
                    ><input class="field" value="不封顶" />
                  </div>
                  <div class="rounding-note"> {{ ui("舍入规则") }} <b>{{ ui("四舍五入 · 整数积分") }}</b>
                  </div></template
                >
                <template v-else-if="ruleStep === 6"
                  ><div class="form-section">
                    <h3>{{ ui("6. 设置限额与取消规则") }}</h3>
                    <p>{{ ui("提供用户、单注和单期期次边界。") }}</p>
                  </div>
                  <div class="form-grid">
                    <label>{{ ui("单注最低") }}<input class="field" value="1" /></label
                    ><label
                      >{{ ui("单注最高") }}<input class="field" value="10,000" /></label
                    ><label
                      >{{ ui("每人每期限额") }}<input class="field" value="50,000" /></label
                    ><label
                      >{{ ui("截止前取消") }}<select class="field">
                        <option value="允许，原路返还">{{ ui("允许，原路返还") }}</option>
                        <option value="不允许">{{ ui("不允许") }}</option>
                      </select></label
                    >
                  </div></template
                >
                <template v-else-if="ruleStep === 7"
                  ><div class="form-section">
                    <h3>{{ ui("7. 输入测试选号和开奖结果") }}</h3>
                    <p> {{ ui("本地原型模拟，不是真实规则引擎；不会创建注单或写入数据。") }} </p>
                  </div>
                  <div class="form-grid">
                    <label
                      >{{ ui("测试选号（空格分隔）") }}<input
                        class="field"
                        v-model="ruleDraft.testSelection" /></label
                    ><label
                      >{{ ui("测试开奖结果") }}<input
                        class="field"
                        v-model="ruleDraft.testResult" /></label
                    ><label
                      >{{ ui("倍数") }}<input
                        class="field"
                        v-model="ruleDraft.multiplier"
                        inputmode="numeric" /></label
                    ><label
                      >{{ ui("单注积分（整数）") }}<input
                        class="field"
                        v-model="ruleDraft.unitPoints"
                        inputmode="numeric"
                    /></label>
                  </div>
                  <button
                    class="button button-primary"
                    @click.prevent="simulate"
                  > {{ ui("▷ 运行模拟") }} </button></template
                >
                <template v-else
                  ><div class="simulation-result">
                    <div class="simulation-head">
                      <span class="success-check">✓</span
                      ><span
                        ><b>{{ ui("规则模拟完成 · 本地原型") }}</b
                        ><small
                          >{{ ui("不是实际规则引擎；结果不会写入注单或账本。") }}</small
                        ></span
                      >
                    </div>
                    <div class="sim-stats">
                      <div><small>{{ ui("规则校验") }}</small><b>{{ ui("通过") }}</b></div>
                      <div>
                        <small>{{ ui("展开组合数") }}</small
                        ><b>{{ simulation.combinations }}</b>
                      </div>
                      <div>
                        <small>{{ ui("命中说明") }}</small
                        ><b>{{ simulation.matches }} {{ ui("项") }}</b>
                      </div>
                      <div>
                        <small>{{ ui("预计积分") }}</small
                        ><b>{{ simulation.points }} <small>{{ ui("分") }}</small></b>
                      </div>
                    </div>
                    <div class="simulation-explain">
                      <b>{{ ui("条件节点") }}</b><span>{{ ui("✓ 号码范围与重复检查") }}</span
                      ><span>✓ {{ simulation.explanation }}</span
                      ><span>{{ ui("✓ 封顶后积分与整数舍入") }}</span>
                    </div>
                  </div></template
                >
              </fieldset>
              <div class="rule-actions">
                <button
                  class="button button-secondary"
                  @click="ruleStep = Math.max(1, ruleStep - 1)"
                > {{ ui("← 上一步") }}</button
                ><span>{{ ui("步骤") }} {{ ruleStep }} / 8</span>
                <div>
                  <button
                    class="button button-secondary"
                    @click="toast('规则草稿已暂存（当前页面演示）')"
                  > {{ ui("保存草稿") }}</button
                  ><button
                    v-if="ruleStep < 8"
                    class="button button-primary"
                    @click="ruleStep++"
                  > {{ ui("下一步 →") }}</button
                  ><button
                    v-else
                    class="button button-primary"
                    :disabled="ruleDraft.status === 'pending_review'"
                    @click="submitRule"
                  > {{ ui("提交审核 →") }} </button>
                </div>
              </div>
            </article>
            <aside class="rule-side">
              <article class="panel approval-card">
                <div class="side-card-icon">✓</div>
                <h3>{{ ui("审核与发布") }}</h3>
                <p> {{ ui("规则创建者不能审核自己的版本。审核通过后还需单独确认发布。") }} </p>
                <div class="approval-line">
                  <span class="avatar-small">{{ ui("林") }}</span
                  ><span
                    ><b>{{ ui("创建人：林岚") }}</b
                    ><small>{{
                      ruleDraft.status === "draft" ? ui("当前操作者") : ui("已提交审核")
                    }}</small></span
                  >
                </div>
                <div class="approval-line">
                  <span class="avatar-small reviewer">{{ ui("周") }}</span
                  ><span
                    ><b>{{ ui("审核人：周宁") }}</b
                    ><small>{{
                      ruleDraft.status === "pending_review"
                        ? ui("待审核 · 与创建者不同")
                        : ui("待分配审核")
                    }}</small></span
                  >
                </div>
                <template v-if="ruleDraft.status === 'pending_review'"
                  ><button
                    class="button button-primary full-button"
                    @click="decisionRule('approve')"
                  > {{ ui("✓ 审核通过") }}</button
                  ><button
                    class="button button-secondary full-button"
                    @click="decisionRule('reject')"
                  > {{ ui("驳回并填写意见") }} </button></template
                ><button
                  v-if="ruleDraft.status === 'approved'"
                  class="button button-primary full-button"
                  @click="toast('已创建待确认发布版本；演示不会改变实际配置')"
                > {{ ui("确认发布（演示）") }} </button>
                <div class="approval-note"> {{ ui("ⓘ 发布后新版本仅作用于新注单，历史订单继续使用原版本。") }} </div>
              </article>
              <article class="panel version-card">
                <h3>{{ ui("版本信息") }}</h3>
                <div><span>{{ ui("当前生效版本") }}</span><b>v12</b></div>
                <div><span>{{ ui("新建草稿") }}</span><b>v13</b></div>
                <div><span>{{ ui("生效方式") }}</span><b>{{ ui("下一期期次") }}</b></div>
                <button
                  class="text-button"
                  @click="toast('查看版本差异（演示）')"
                > {{ ui("对比版本差异 →") }} </button>
              </article>
            </aside>
          </div>
        </details>
      </section>

      <section v-else-if="page === '期次和开奖'" class="page-content">
        <CorrectionManagement
          v-if="account && selectedBrandId"
          :key="`correction-${account.id}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
          @open-settlement="periodId => { correctionSettlementPeriod = { brandId:selectedBrandId,periodId,nonce:(correctionSettlementPeriod?.nonce ?? 0)+1 }; }"
        />
        <SettlementManagement
          v-if="account && selectedBrandId"
          :key="`settlement-${account.id}-${correctionSettlementPeriod?.nonce ?? 0}`"
          :account="account"
          :brand-id="selectedBrandId"
          :initial-period-id="correctionSettlementPeriod?.brandId === selectedBrandId ? correctionSettlementPeriod.periodId : undefined"
          @session-invalid="clearAdminData"
        />
        <PeriodCancellation
          v-if="account && selectedBrandId"
          :key="`period-cancel-${account.id}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <PeriodSchedules
          v-if="account && selectedBrandId"
          :key="`periods-${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else class="panel directory-state">
          <h1>{{ ui("真实期次计划") }}</h1>
          <p>{{ ui("请先登录后台账号并选择品牌，才能保存计划、生成和查询期次。") }}</p>
          <button
            v-if="!account"
            class="button button-primary"
            @click="go('用户和成员')"
          > {{ ui("进入管理员登录") }} </button>
        </div>
        <DrawManagement
          v-if="account && selectedBrandId"
          :key="`draws-${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <details class="demo-rule-workbench">
          <summary> {{ ui("旧开奖工作台演示（不会开奖、纠正结果或结算真实注单）") }} </summary>
          <div class="page-heading">
            <div>
              <div class="eyebrow">DRAW / PERIOD CONTROL</div>
              <h1>{{ ui("期次和开奖") }}</h1>
              <p>{{ ui("查看期次时间线、来源校验与开奖操作") }}</p>
            </div>
            <div class="heading-actions">
              <button
                class="button button-secondary"
                @click="manualOpen = true"
              > {{ ui("＋ 人工开奖") }}</button
              ><button
                class="button button-primary"
                @click="toast('已生成下一期草稿（演示）')"
              > {{ ui("⟳ 生成期次") }} </button>
            </div>
          </div>
          <div class="period-summary">
            <div class="panel period-main">
              <div class="period-title">
                <div>
                  <span class="badge badge-success">{{ ui("投注中") }}</span>
                  <h2>{{ ui("星彩 6+1") }} <small>20261005032</small></h2>
                </div>
                <button class="text-button" @click="toast('已复制期次编号')"> {{ ui("复制编号 ⧉") }} </button>
              </div>
              <div class="timeline">
                <div class="timeline-stage complete">
                  <i>✓</i
                  ><span><b>{{ ui("投注开始") }}</b><small>10-05 08:00:00</small></span>
                </div>
                <div class="timeline-connector active"></div>
                <div class="timeline-stage current">
                  <i>2</i
                  ><span
                    ><b>{{ ui("投注截止") }}</b
                    ><small>{{ ui("10-05 11:00:00 · 18 分钟后") }}</small></span
                  >
                </div>
                <div class="timeline-connector"></div>
                <div class="timeline-stage">
                  <i>3</i
                  ><span><b>{{ ui("开奖时间") }}</b><small>10-05 11:10:00</small></span>
                </div>
                <div class="timeline-connector"></div>
                <div class="timeline-stage">
                  <i>4</i><span><b>{{ ui("结算完成") }}</b><small>{{ ui("等待开奖") }}</small></span>
                </div>
              </div>
              <div class="period-kpis">
                <div>
                  <small>{{ ui("投注订单") }}</small><b>1,284 <small>{{ ui("笔") }}</small></b>
                </div>
                <div>
                  <small>{{ ui("投注积分") }}</small><b>28,450 <small>{{ ui("分") }}</small></b>
                </div>
                <div>
                  <small>{{ ui("关联规则") }}</small><b>v12 <small>· 6+1</small></b>
                </div>
                <div>
                  <small>{{ ui("品牌时区") }}</small
                  ><b>UTC+08:00 <small>{{ ui("· 新加坡") }}</small></b>
                </div>
              </div>
            </div>
            <aside class="panel source-health">
              <div class="panel-header">
                <div>
                  <h2>{{ ui("开奖来源") }}</h2>
                  <p>{{ ui("优先级与最近检查时间") }}</p>
                </div>
                <button
                  class="text-button"
                  @click="toast('来源切换记录（演示）')"
                > {{ ui("切换记录") }} </button>
              </div>
              <div class="source-row">
                <span class="source-mark">1</span
                ><span><b>{{ ui("主来源 · API") }}</b><small>{{ ui("上次校验 10:32:18") }}</small></span
                ><span class="badge badge-success">{{ ui("健康") }}</span>
              </div>
              <div class="source-row">
                <span class="source-mark source-alt">2</span
                ><span
                  ><b>{{ ui("备用来源 · API") }}</b><small>{{ ui("上次校验 10:31:54") }}</small></span
                ><span class="badge badge-success">{{ ui("健康") }}</span>
              </div>
              <div class="source-row">
                <span class="source-mark source-manual">M</span
                ><span
                  ><b>{{ ui("人工录入") }}</b
                  ><small>{{ ui("单人操作，必须记录审计原因") }}</small></span
                ><button class="text-button" @click="manualOpen = true"> {{ ui("录入") }} </button>
              </div>
            </aside>
          </div>
          <div class="result-layout">
            <article class="panel result-card">
              <div class="panel-header">
                <div>
                  <h2>{{ ui("当前开奖结果") }}</h2>
                  <p>{{ ui("上一期 · 20261005031") }}</p>
                </div>
                <span class="badge badge-success">{{ ui("已确认") }}</span>
              </div>
              <div class="result-balls">
                <span v-for="n in ['07', '18', '29', '33', '41']" :key="n">{{
                  n
                }}</span
                ><i>+</i><span class="special-ball">06</span>
              </div>
              <div class="result-meta">
                <span>{{ ui("来源：主开奖源") }}</span
                ><span>{{ ui("校验项：号码范围 ✓　重复校验 ✓　签名 ✓") }}</span
                ><span>{{ ui("确认时间：10-05 08:10:14") }}</span>
              </div>
              <div class="result-actions">
                <button
                  class="button button-secondary"
                  @click="correctionOpen = true"
                > {{ ui("↻ 纠正开奖结果") }}</button
                ><button
                  class="button button-primary"
                  @click="toast('结算任务已加入队列（演示）')"
                > {{ ui("确认并结算 →") }} </button>
              </div>
            </article>
            <article class="panel timeline-card">
              <h2>{{ ui("状态时间线") }}</h2>
              <div class="audit-timeline">
                <div>
                  <i class="event-green">✓</i
                  ><span
                    ><b>{{ ui("开奖结果已确认") }}</b
                    ><small>{{ ui("周宁 · 10-05 08:10") }}</small></span
                  >
                </div>
                <div>
                  <i>↻</i
                  ><span
                    ><b>{{ ui("来源结果通过校验") }}</b
                    ><small>{{ ui("系统演示 · 10-05 08:09") }}</small></span
                  >
                </div>
                <div>
                  <i>◷</i
                  ><span
                    ><b>{{ ui("期次投注已截止") }}</b
                    ><small>{{ ui("系统演示 · 10-05 08:00") }}</small></span
                  >
                </div>
              </div>
            </article>
          </div>
          <div class="danger-note"> {{ ui("⚠ 人工开奖和结果纠正均属于高风险演示操作。确认前必须查看影响范围并填写原因。") }} </div>
        </details>
      </section>

      <section v-else-if="page === '注单和异常'" class="page-content">
        <BetPolicySettings
          v-if="account && selectedBrandId"
          :key="`bet-policy-${account.id}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <BetOrderManagement
          v-if="account && selectedBrandId"
          :key="`bet-orders-${account.id}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else-if="account" class="panel directory-state"> {{ ui("请选择一个有权访问的品牌以查看注单。") }} </div>
        <div v-else class="panel directory-state"> {{ ui("请先登录后台账号。投注策略和注单均从真实 API 读取，不使用演示数据。") }} <button class="button button-secondary" @click="go('用户和成员')"> {{ ui("前往登录") }} </button>
        </div>
      </section>

      <section v-else-if="page === '批量对账'" class="page-content">
        <ReconciliationManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">{{ ui("请登录并选择有钱包查看权限的品牌。") }}</div>
      </section>

      <section v-else-if="page === '资金与账本'" class="page-content">
        <WithdrawalManagement
          v-if="account && selectedBrandId"
          :key="`withdrawals:${account.id}:${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <FinanceManagement
          v-if="account && selectedBrandId"
          :key="selectedBrandId"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <BalanceRepair
          v-if="account && selectedBrandId"
          :key="`repair-${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <WithdrawalPolicySettings
          v-if="account && selectedBrandId"
          :key="`withdrawal-policy-${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else class="panel directory-state">
          <h1>{{ ui("资金与账本") }}</h1>
          <p>
            {{
              account
                ? ui("请先选择真实后台品牌。")
                : ui("请先登录后台账号，才能查看和操作真实积分。")
            }}
          </p>
          <button
            v-if="!account"
            class="button button-primary"
            @click="go('用户和成员')"
          > {{ ui("进入管理员登录") }} </button>
        </div>
      </section>

      <section v-else-if="page === '佣金和奖励'" class="page-content">
        <CommissionCyclesManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">
          <h1>{{ ui("佣金和奖励") }}</h1>
          <p>{{ account ? ui("请先选择真实后台品牌。") : ui("请先登录后台账号查看佣金周期。") }}</p>
        </div>
      </section>

      <section v-else-if="page === '通知模板'" class="page-content">
        <NotificationTemplates v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ ui("通知模板") }}</h1>
          <p>{{ account ? ui("请先选择真实后台品牌。") : ui("请先登录后台账号，才能配置站内通知模板。") }}</p>
          <button v-if="!account" class="button button-primary" @click="go('用户和成员')">{{ ui("进入管理员登录") }}</button>
        </div>
      </section>

      <section v-else-if="page === '报表和对账'" class="page-content">
        <ReportsManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ ui("报表和对账") }}</h1>
          <p>{{ account ? ui("请先选择真实后台品牌。") : ui("请先登录后台账号，才能查看真实运营报表。") }}</p>
          <button v-if="!account" class="button button-primary" @click="go('用户和成员')">{{ ui("进入管理员登录") }}</button>
        </div>
      </section>

      <section v-else-if="page === '账号与权限'" class="page-content">
        <AccessManagement
          v-if="account && selectedBrandId"
          :key="selectedBrandId"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else class="panel directory-state">
          <h1>{{ ui("账号与权限") }}</h1>
          <p>
            {{
              account
                ? ui("请先选择真实后台品牌。")
                : ui("请先登录后台账号，才能管理真实角色和权限。")
            }}
          </p>
          <button
            v-if="!account"
            class="button button-primary"
            @click="go('用户和成员')"
          > {{ ui("进入管理员登录") }} </button>
        </div>
      </section>

      <section v-else-if="page === '通知投递'" class="page-content">
        <NotificationDeliveries v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ ui("通知投递") }}</h1>
          <p>{{ account ? ui("请先选择真实后台品牌。") : ui("请先登录后台账号，才能查看真实投递记录。") }}</p>
          <button v-if="!account" class="button button-primary" @click="go('用户和成员')">{{ ui("进入管理员登录") }}</button>
        </div>
      </section>

      <section v-else class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">SECURITY / AUDIT TRAIL</div>
            <h1>{{ ui("审计日志") }}</h1>
            <p>
              {{
                account
                  ? ui("按选中品牌读取真实后台日志。")
                  : ui("未登录：以下是静态演示样例，不是后台记录。")
              }}
            </p>
          </div>
          <button
            v-if="account"
            class="button button-secondary"
            :disabled="auditLoading || !selectedBrandId"
            @click="loadAudit"
          > {{ ui("刷新日志") }} </button>
        </div>
        <div v-if="account && !selectedBrandId" class="panel directory-state"> {{ ui("请先从侧栏选择品牌；审计请求始终携带明确的 X-Brand-ID。") }} </div>
        <div
          v-else-if="account && auditError"
          class="directory-error"
          role="alert"
        >
          {{ t(auditError) }}
        </div>
        <article v-if="account && selectedBrandId" class="panel">
          <div v-if="auditLoading" class="directory-state"> {{ ui("正在读取审计日志…") }} </div>
          <div v-else-if="!auditRecords.length" class="directory-state"> {{ ui("所选品牌没有可显示的审计记录。") }} </div>
          <div v-else class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{{ ui("操作") }}</th>
                  <th>{{ ui("资源") }}</th>
                  <th>{{ ui("操作人") }}</th>
                  <th>{{ ui("原因") }}</th>
                  <th>Request ID</th>
                  <th>IP</th>
                  <th>{{ ui("时间") }}</th>
                </tr>
              </thead>
                  <tbody>
                    <tr v-for="entry in auditRecords" :key="entry.id">
                  <td>{{ entry.action }}</td>
                  <td>{{ entry.resource_type }} · {{ entry.resource_id }}</td>
                  <td>{{ entry.actor_type }} · {{ entry.actor_id }}</td>
                  <td>{{ entry.reason }}</td>
                  <td>{{ entry.request_id }}</td>
                  <td>{{ entry.ip_address }}</td>
                  <td>{{ new Date(entry.created_at).toLocaleString() }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </article>
        <div v-if="!account" class="panel directory-state"> {{ ui("演示日志不会显示为真实后台记录。") }} </div>
      </section>

      <footer v-if="page !== '加入码' && page !== '工作台'" class="page-footer">
        <span>Aurora Operations Console <b>·</b> Prototype v0.1</span
        ><span>{{
          page === "用户和成员" && account
              ? ui("成员创建与管理为真实操作；投注已接入。")
            : page === "代理树" && account
              ? ui("真实代理配置与历史记录；佣金核算已接入，审核和派发尚未实现。")
            : page === "报表和对账" && account
              ? ui("真实注单结果及账本流水；汇总余额不是完整逐账户对账证明。")
            : page === "通知投递" && account
              ? ui("真实站内通知投递记录；外部发送渠道尚未接入。")
            : page === "通知模板" && account
              ? ui("真实版本化站内通知模板；已生成消息保留原文案，不触发新通知或资金变化。")
            : page === "审计日志" && account
              ? ui("审计日志为真实后台数据；投注已接入，提现状态接口已接入，资格规则待配置且无真实支付。")
              : page === "账号与权限" && account
                ? ui("账号与角色变更为真实操作；投注已接入，提现状态接口已接入，资格规则待配置且无真实支付。")
                : page === "品牌和域名" && account
                  ? ui("认证、品牌展示及域名绑定为真实配置；提现状态接口已接入，资格规则待配置且无真实支付。")
                  : page === "资金与账本" && account
                    ? ui("人工充值、冻结、调整与账本为真实操作；提现状态接口已接入，资格规则待配置且无真实支付。")
                    : ui("标为演示的功能不写入后台；账号、积分、规则版本和期次计划已接入真实 API。")
        }}</span>
      </footer>
      <div
        v-if="editTarget"
        class="modal-backdrop"
        @click.self="editTarget = null"
      >
        <section
          class="modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="member-edit-title"
        >
          <div class="modal-heading">
            <div>
              <span class="eyebrow">REAL MEMBER · {{ selectedBrandId }}</span>
              <h2 id="member-edit-title">{{ ui("编辑成员状态和备注") }}</h2>
            </div>
            <button
              class="modal-close"
              :aria-label="ui('关闭')"
              @click="editTarget = null"
            >
              ×
            </button>
          </div>
          <div class="modal-summary">
            <div>
              <small>{{ ui("成员") }}</small
              ><b>{{ editTarget.display_name || editTarget.username }}</b>
            </div>
            <div>
              <small>{{ ui("用户 ID") }}</small><b>{{ editTarget.global_user_id }}</b>
            </div>
          </div>
          <label class="modal-label"
            >{{ ui("状态") }}<select v-model="editStatus" class="field">
              <option
                v-for="(label, status) in statusLabel"
                :key="status"
                :value="status"
              >
                {{ ui(label) }}
              </option>
            </select></label
          ><label class="modal-label"
            >{{ ui("备注") }}<textarea
              v-model="editNotes"
              rows="3"
              maxlength="2000"
            ></textarea></label
          ><label class="modal-label"
            >{{ ui("操作原因") }} <span>{{ ui("必填") }}</span
            ><textarea v-model="editReason" rows="2" required></textarea>
          </label>
          <div class="modal-actions">
            <button class="button button-secondary" @click="editTarget = null"> {{ ui("取消") }}</button
            ><button
              class="button button-primary"
              :disabled="editBusy || !editReason.trim()"
              @click="saveMember"
            >
              {{ editBusy ? ui("保存中…") : ui("保存到后台") }}
            </button>
          </div>
        </section>
      </div>
      <div
        v-if="kickTarget"
        class="modal-backdrop"
        @click.self="kickTarget = null"
      >
        <section
          class="modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="member-kick-title"
        >
          <div class="modal-heading">
            <div>
              <span class="eyebrow">REAL MEMBER · {{ selectedBrandId }}</span>
              <h2 id="member-kick-title">{{ ui("踢出品牌会话") }}</h2>
            </div>
            <button
              class="modal-close"
              :aria-label="ui('关闭')"
              @click="kickTarget = null"
            >
              ×
            </button>
          </div>
          <p class="danger-note"> {{ ui("这会调用后台踢出接口，撤销该用户在当前品牌的会话。") }} </p>
          <label class="modal-label"
            >{{ ui("原因") }} <span>{{ ui("必填") }}</span
            ><textarea v-model="kickReason" rows="3" required></textarea>
          </label>
          <div class="modal-actions">
            <button class="button button-secondary" @click="kickTarget = null"> {{ ui("取消") }}</button
            ><button
              class="button button-danger"
              :disabled="kickBusy || !kickReason.trim()"
              @click="kickMember"
            >
              {{ kickBusy ? ui("处理中…") : ui("确认踢出") }}
            </button>
          </div>
        </section>
      </div>
      <div
        v-if="resetTarget"
        class="modal-backdrop"
        @click.self="resetTarget = null"
      >
        <section
          class="modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="member-reset-title"
        >
          <div class="modal-heading">
            <div>
              <span class="eyebrow">GLOBAL PASSWORD · HIGH IMPACT</span>
              <h2 id="member-reset-title">{{ ui("重置全局密码") }}</h2>
            </div>
            <button
              class="modal-close"
              :aria-label="ui('关闭')"
              @click="resetTarget = null"
            >
              ×
            </button>
          </div>
          <div class="danger-note"> {{ ui("此密码属于全局用户，将影响该用户在所有品牌的登录，并撤销其全部会话。后台要求你对该用户加入的每个品牌都有密码重置权限，否则拒绝操作。") }} </div>
          <div class="modal-summary">
            <div>
              <small>{{ ui("成员") }}</small
              ><b>{{ resetTarget.display_name || resetTarget.username }}</b>
            </div>
            <div>
              <small>{{ ui("全局用户 ID") }}</small><b>{{ resetTarget.global_user_id }}</b>
            </div>
          </div>
          <label class="modal-label"
            >{{ ui("新密码") }} <span>{{ ui("必填") }}</span
            ><input
              v-model="resetPasswordValue"
              class="field"
              type="password"
              autocomplete="new-password"
              required /></label
          ><label class="modal-label"
            >{{ ui("重置原因") }} <span>{{ ui("必填") }}</span
            ><textarea
              v-model="resetReason"
              rows="2"
              required
            ></textarea></label
          ><label class="impact-confirm"
            ><input
              v-model="resetImpactConfirmed"
              type="checkbox"
            />{{ ui("我确认此重置影响该全局账号在所有品牌的密码和会话。") }}</label
          >
          <div class="modal-actions">
            <button class="button button-secondary" @click="resetTarget = null"> {{ ui("取消") }}</button
            ><button
              class="button button-danger"
              :disabled="
                resetBusy ||
                !resetPasswordValue ||
                !resetReason.trim() ||
                !resetImpactConfirmed
              "
              @click="resetMemberPassword"
            >
              {{ resetBusy ? ui("处理中…") : ui("确认重置全局密码") }}
            </button>
          </div>
        </section>
      </div>
    </main>

    <div
      v-if="mobileMore"
      class="mobile-more-backdrop"
      @click.self="mobileMore = false"
    >
      <nav class="mobile-more-menu" :aria-label="ui('全部管理页面')">
        <div>
          <b>{{ ui("全部管理页面") }}</b
          ><button :aria-label="ui('关闭导航')" @click="mobileMore = false">×</button>
        </div>
        <button
          v-for="item in visibleNav.filter(
            (entry) =>
              !(
                ['工作台', '用户和成员', '期次和开奖', '资金与账本'] as Page[]
              ).includes(entry.name),
          )"
          :key="item.name"
          @click="go(item.name)"
        >
          <span>{{ item.icon }}</span
          >{{ ui(item.name) }}<i>›</i>
        </button>
      </nav>
    </div>
    <nav class="mobile-nav" :aria-label="ui('移动端主导航')">
      <button
        v-for="target in [
          '工作台',
          '用户和成员',
          '期次和开奖',
          '资金与账本',
        ] as Page[]"
        :key="target"
        :class="{ active: page === target }"
        @click="go(target)"
      >
        <span>{{ nav.find((item) => item.name === target)?.icon }}</span
        ><small>{{ ui(
          target === "用户和成员"
            ? "用户"
            : target === "期次和开奖"
              ? "期次"
              : target === "资金与账本"
                ? "审核"
                : target)
        }}</small></button
      ><button
        :class="{ active: mobileMore }"
        @click="mobileMore = !mobileMore"
      >
        <span>☰</span><small>{{ ui("更多") }}</small>
      </button>
    </nav>

    <div
      v-if="correctionOpen"
      class="modal-backdrop"
      @click.self="correctionOpen = false"
    >
      <section
        class="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="correction-title"
      >
        <div class="modal-heading">
          <div>
            <span class="eyebrow">RESULT CORRECTION · DEMO</span>
            <h2 id="correction-title">{{ ui("纠正开奖结果") }}</h2>
          </div>
          <button class="modal-close" @click="correctionOpen = false">×</button>
        </div>
        <div class="correction-values">
          <label
            >{{ ui("原结果") }}<input
              class="field"
              v-model="periodResult"
              readonly /></label
          ><span>→</span
          ><label
            >{{ ui("新结果") }}<input class="field" v-model="correctionResult"
          /></label>
        </div>
        <div class="impact-preview">
          <div class="impact-head">
            <b>{{ ui("影响范围预览") }}</b><span>{{ ui("预估值 · 非实际回溯结果") }}</span>
          </div>
          <div>
            <span>{{ ui("待回溯注单") }}</span
            ><b>{{ correctionPreview.affectedOrders }} {{ ui("笔") }}</b>
          </div>
          <div>
            <span>{{ ui("预计重新结算") }}</span
            ><b>{{ correctionPreview.estimatedSettlements }} {{ ui("项") }}</b>
          </div>
          <div><span>{{ ui("账本处理方式") }}</span><b>{{ ui("保留原记录并创建冲正") }}</b></div>
        </div>
        <label class="modal-label"
          >{{ ui("纠正原因") }} <span>{{ ui("必填") }}</span
          ><textarea
            v-model="correctionReason"
            rows="3"
            :placeholder="ui('记录结果来源、校验依据及纠正原因')"
          ></textarea>
        </label>
        <div class="modal-actions">
          <button
            class="button button-secondary"
            @click="correctionOpen = false"
          > {{ ui("取消") }}</button
          ><button class="button button-danger" @click="correctResult"> {{ ui("确认纠正并创建回溯（演示）") }} </button>
        </div>
      </section>
    </div>
    <div
      v-if="manualOpen"
      class="modal-backdrop"
      @click.self="manualOpen = false"
    >
      <section
        class="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="manual-title"
      >
        <div class="modal-heading">
          <div>
            <span class="eyebrow">MANUAL DRAW · DEMO</span>
            <h2 id="manual-title">{{ ui("人工录入开奖结果") }}</h2>
          </div>
          <button class="modal-close" @click="manualOpen = false">×</button>
        </div>
        <div class="form-grid">
          <label>{{ ui("期次") }}<input class="field" value="20261005032" readonly /></label
          ><label
            >{{ ui("来源") }}<select class="field">
              <option value="人工录入（单人操作）">{{ ui("人工录入（单人操作）") }}</option>
            </select></label
          ><label class="span-two"
            >{{ ui("开奖结果") }}<input class="field" v-model="correctionResult"
          /></label>
        </div>
        <div class="impact-preview">
          <div class="impact-head">
            <b>{{ ui("提交后影响") }}</b><span>{{ ui("演示人工结果生效，本期停用外部来源") }}</span>
          </div>
          <div><span>{{ ui("校验项") }}</span><b>{{ ui("号码范围 · 重复 · 期次状态") }}</b></div>
          <div><span>{{ ui("后续操作") }}</span><b>{{ ui("记录操作者和原因，进入结算流程") }}</b></div>
        </div>
        <label class="modal-label"
          >{{ ui("人工录入原因") }} <span>{{ ui("必填") }}</span
          ><textarea
            v-model="manualReason"
            rows="3"
            :placeholder="ui('说明人工开奖原因和数据核验依据')"
          ></textarea>
        </label>
        <div class="modal-actions">
          <button class="button button-secondary" @click="manualOpen = false"> {{ ui("取消") }}</button
          ><button class="button button-primary" @click="confirmManual"> {{ ui("确认人工结果（演示）") }} </button>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.brand-operation-empty { margin: 14px 0; padding: 18px; }
.brand-operation-empty h2 { margin: 0 0 6px; }
.brand-operation-empty p { margin: 0; color: #718096; }
.brand-operation-brands { min-width: 0; margin-top: 16px; padding: 18px; }
.brand-operation-unavailable { padding: 10px 12px; border-radius: 8px; background: #f3f6fa; color: #5f6d80; line-height: 1.5; }
.brand-operation-list { margin: 0; padding: 0; list-style: none; }
.brand-operation-list li { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 14px; min-width: 0; padding: 13px 0; border-top: 1px solid #e9edf2; }
.brand-operation-list__identity { display: flex; align-items: center; gap: 11px; min-width: 0; }
.brand-operation-list__identity > span:last-child { display: grid; gap: 4px; min-width: 0; }
.brand-operation-list__identity strong, .brand-operation-list__identity small { overflow-wrap: anywhere; }
.brand-operation-list__identity small { color: #718096; font-size: 12px; }
.brand-operation-list__mark { display: grid; width: 36px; height: 36px; flex: 0 0 36px; place-items: center; border-radius: 10px; background: #edf3ff; color: #315fbd; font-weight: 700; }
@media (max-width: 700px) { .brand-operation-list li { grid-template-columns: minmax(0, 1fr) auto; gap: 10px; } .brand-operation-list li > .text-button { grid-column: 1 / -1; justify-self: start; } }
</style>
