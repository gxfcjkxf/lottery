<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
  onMounted,
  ref,
} from "vue";
import { brandPermissionSet } from "./brand-permissions";
import { canViewWallet } from "./finance-api";
const AccessManagement = defineAsyncComponent(() => import("./AccessManagement.vue"));
import MemberProvision from "./MemberProvision.vue";
import AuthSettings from "./AuthSettings.vue";
import BrandOperation from "./BrandOperation.vue";
import BrandPresentation from "./BrandPresentation.vue";
import CompliancePolicy from "./CompliancePolicy.vue";
import { clearAllPendingComplianceIntents } from "./compliance-state";
const BrandDomains = defineAsyncComponent(() => import("./BrandDomains.vue"));
import {clearAllPendingBrandDomainsWrites} from "./brand-domains-state";
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
const ReportArchivesManagement = defineAsyncComponent(() => import("./ReportArchivesManagement.vue"));
import { clearAllArchiveIntents } from "./report-archives-state";
const ReportArchiveTasksManagement = defineAsyncComponent(() => import("./ReportArchiveTasksManagement.vue"));
import { clearAllReportArchiveTaskRetryIntents } from "./report-archive-tasks-state";
import { clearAllReportArchivePolicyIntents } from "./report-archive-policy-state";
import {clearPendingReconciliationWrites} from "./reconciliation-state";
const ReportsManagement = defineAsyncComponent(() => import("./ReportsManagement.vue"));
const AuditManagement = defineAsyncComponent(() => import("./AuditManagement.vue"));
const AgentManagement = defineAsyncComponent(() => import("./AgentManagement.vue"));
const CommissionPolicySettings = defineAsyncComponent(() => import("./CommissionPolicySettings.vue"));
const CommissionCyclesManagement = defineAsyncComponent(() => import("./CommissionCyclesManagement.vue"));
const CommissionPaymentsManagement = defineAsyncComponent(() => import("./CommissionPaymentsManagement.vue"));
const CommissionCorrectionsManagement = defineAsyncComponent(() => import("./CommissionCorrectionsManagement.vue"));
const RewardsManagement = defineAsyncComponent(() => import("./RewardsManagement.vue"));
import { clearAllPendingCommissionCycleWrites } from "./commission-cycles-state";
import { clearAllPendingCommissionAdjustments } from "./commission-adjustments-state";
import { clearAllCommissionPaymentIntents, hideCommissionPaymentIntentsForScope } from "./commissionPayments-state";
import { clearAllRewardIntents } from "./rewards-state";
import { clearAllCommissionCorrectionIntents, hideCommissionCorrectionIntentsForScope } from "./commission-corrections-state";
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
  AdminApiError,
  createAdminApi,
  createIdempotencyKey,
  type AdminAccount,
  type AdminBrand,
  type Member,
  type MemberStatus,
} from "./admin-api";

const { t, message, locale, availableLocales, setLocale, configure, resetBrand } = useAdminI18n();
const englishUi: Record<string, string> = {
  "请先登录后台账号查看佣金周期。": "Sign in to view commission cycles.",
  "控制台包含已接入流程与原型。提现资格与状态流程已接入，品牌策略默认关闭且不支持真实支付；佣金核算与独立派发后台已接入，真实派发策略默认关闭。": "The console combines connected workflows and prototypes. Withdrawal qualification and status are connected; brand policy is disabled by default and real payments are unavailable. Commission calculations and the separate payout console are connected; live payout policy is disabled by default.",
  "此页显示真实成员；账号权限、积分账本、认证设置和投注已接入。": "This page shows live members. Account permissions, the points ledger, authentication settings, and betting are connected.",
  "成员创建与管理为真实操作；投注已接入。": "Member creation and management are live operations. Betting is connected.",
  "审计日志为真实后台数据；投注已接入，提现资格与状态流程已接入，品牌策略默认关闭且无真实支付。": "Audit logs are live admin data; betting and withdrawal qualification/status are connected; brand policy is disabled by default and real payments are unavailable.",
  "账号与角色变更为真实操作；投注已接入，提现资格与状态流程已接入，品牌策略默认关闭且无真实支付。": "Account and role changes are live operations; betting and withdrawal qualification/status are connected; brand policy is disabled by default and real payments are unavailable.",
  "认证、品牌展示及域名绑定为真实配置；提现资格与状态流程已接入，品牌策略默认关闭且无真实支付。": "Authentication, brand presentation, and domain binding are live settings; withdrawal qualification/status are connected; brand policy is disabled by default and real payments are unavailable.",
  "人工充值、冻结、调整与账本为真实操作；提现资格与状态流程已接入，品牌策略默认关闭且无真实支付。": "Manual deposits, freezes, adjustments, and ledger actions are live; withdrawal qualification/status are connected; brand policy is disabled by default and real payments are unavailable.",
  "内部积分，未接入真实支付": "Internal points · No real payments",
  "切换品牌": "Switch brand", "真实品牌": "Live brand", "选择品牌": "Select a brand",
  "没有可访问的品牌": "No accessible brands", "主导航": "Main navigation",
  "概览": "Overview", "平台": "Platform", "运营": "Operations", "资金": "Finance", "管理": "Management",
  "帮助与反馈": "Help & feedback", "超级管理员": "Super administrator", "管理员": "Administrator",
  "退出登录": "Sign out", "退出": "Sign out", "运营控制台": "Operations console", "品牌后台": "Brand administration",
  "搜索用户、注单、期次": "Search users, orders, periods", "搜索": "Search", "通知": "Notifications", "帮助中心": "Help center", "关闭说明": "Dismiss notice",
  "后端品牌上下文": "Backend brand context", "正在读取 GET /api/v1/context": "Reading GET /api/v1/context", "平台品牌上下文": "Platform brand context",
  "当前入口没有可用的公开用户品牌上下文；后台管理认证独立，不受影响。请选择需要管理的品牌。": "No public user brand context is available at this entry point. Admin authentication is independent. Select a brand to manage.",
  "后端品牌上下文未连接": "Backend brand context unavailable", "暂不可用；该接口仅用于只读配置状态，不影响管理员认证和真实用户目录。": "is unavailable. This read-only endpoint reports configuration and does not affect admin authentication or the live user directory.",
  "只读接口 GET /api/v1/context": "Read-only endpoint GET /api/v1/context", "配置版本": "Config version", "真实后台品牌": "Live admin brand", "选择真实后台品牌": "Select live admin brand", "请选择品牌": "Select a brand",
  "运营工作台": "Operations workbench", "请先登录后台账号查看运营工作台。": "Sign in to view the operations workbench.", "请先选择真实后台品牌查看运营工作台。": "Select a live admin brand to view the operations workbench.",
  "分": "points", "提现": "Withdrawals", "异常注单": "Abnormal order", "待开奖": "Awaiting draw", "注单": "Orders", "投注中": "Betting open", "待处理": "Pending",
  "投注趋势": "Betting trend", "每日总投注积分": "Daily betting points", "趋势时间范围": "Trend time range", "近 7 天": "Last 7 days", "近 30 天": "Last 30 days", "近七日投注量趋势": "Betting volume over the last seven days", "开奖源健康": "Draw source health", "正常": "Normal", "主开奖源": "Primary draw source", "备用开奖源": "Backup draw source", "最近检查": "Last checked", "连续可用": "Uptime",
  "请先选择真实品牌。": "Select a live brand first.", "请先登录后台账号并选择真实品牌。": "Sign in to the admin account and select a live brand.",
  "品牌和域名": "Brands & domains", "真实品牌列表、运行状态、展示与域名绑定": "Live brands, operating status, presentation, and domain binding", "选择一个品牌": "Choose a brand", "选择下方真实品牌后，可查看其运行状态与操作记录。": "Select a live brand below to view its status and activity.", "来自管理员品牌接口": "From the admin brands API", "刷新列表": "Refresh list", "正在读取品牌…": "Loading brands…", "当前账号未返回可管理品牌。": "This account has no manageable brands.", "运行中": "Running", "已暂停": "Paused", "已停用": "Disabled", "当前品牌": "Current brand", "查看运行状态 →": "View status →",
  "管理员登录": "Admin sign in", "使用后台管理员账号登录。": "Sign in with an admin account.", "账号": "Account", "密码": "Password", "登录中…": "Signing in…", "登录并加载真实成员": "Sign in and load members", "请选择真实品牌": "Select a live brand", "成员请求不会使用后端默认品牌。请从侧栏品牌选择器中选择一个有权访问的品牌。": "Member requests never use the backend's default brand. Choose an accessible brand from the sidebar selector.", "进入管理员登录": "Go to admin sign in", "正在检查管理员登录状态…": "Checking admin sign-in…", "平台超级管理员请使用平台管理入口。": "Platform super administrators must use the platform admin portal.", "平台管理入口": "Platform admin portal", "品牌管理员 · 运营 · 财务 · 客服": "Brand administrators · Operations · Finance · Support", "已登录": "Signed in", "当前真实品牌": "Current live brand", "读取中…": "Loading…", "刷新成员": "Refresh members", "品牌成员": "Brand members", "条本页": "on this page", "搜索用户名 / 手机号 / ID": "Search username / phone / ID", "搜索成员": "Search members", "正在读取品牌成员…": "Loading brand members…", "该品牌当前没有可显示的成员。": "This brand has no members to display.", "成员": "Member", "成员 ID": "Member ID", "手机号": "Phone", "状态": "Status", "加入时间": "Joined", "备注": "Notes", "标签": "Tags", "操作": "Actions", "编辑": "Edit", "踢出": "Remove", "重置密码": "Reset password", "上一页": "Previous", "下一页": "Next", "每页最多": "Up to", "条": "records",
  "代理管理": "Agent management", "代理树": "Agent tree", "加入码管理": "Join code management", "规则配置": "Rule configuration", "期次和开奖": "Periods & draws", "注单和异常": "Orders & exceptions", "资金与账本": "Funds & ledger", "批量对账": "Bulk reconciliation", "佣金和奖励": "Commissions & rewards", "佣金派发": "Commission payouts", "报表和对账": "Reports & reconciliation", "账号与权限": "Accounts and permissions", "审计日志": "Audit log", "通知投递": "Notification delivery", "通知模板": "Notification templates", "风控与合规": "Risk & compliance", "工作台": "Dashboard", "用户和成员": "Users and members",
  "请先登录后台账号查看真实佣金派发记录。": "Sign in to view live commission payout records.",
  "人工奖励": "Manual rewards", "请先登录后台账号查看人工奖励订单。": "Sign in to view manual reward orders.",
  "佣金更正": "Commission corrections", "请先登录后台账号查看佣金更正计划和执行。": "Sign in to view commission correction plans and executions.",
  "刷新日志": "Refresh log", "按选中品牌读取真实后台日志。": "Load live admin logs for the selected brand.", "请先从侧栏选择品牌；审计请求始终携带明确的 X-Brand-ID。": "Select a brand in the sidebar. Audit requests always include an explicit X-Brand-ID.", "正在读取审计日志…": "Loading audit log…", "所选品牌没有可显示的审计记录。": "No audit records are available for this brand.", "资源": "Resource", "操作人": "Actor", "原因": "Reason", "时间": "Time", "演示日志不会显示为真实后台记录。": "Demo logs are not shown as live admin records.",
  "关闭": "Close", "取消": "Cancel", "必填": "Required", "保存中…": "Saving…", "保存到后台": "Save to admin", "处理中…": "Processing…", "确认踢出": "Confirm removal", "确认重置全局密码": "Confirm global password reset", "编辑成员状态和备注": "Edit member status and notes", "踢出品牌会话": "Revoke brand session", "重置全局密码": "Reset global password", "这会调用后台踢出接口，撤销该用户在当前品牌的会话。": "This calls the admin removal API and revokes this user's session for the current brand.", "新密码": "New password", "重置原因": "Reset reason", "我确认此重置影响该全局账号在所有品牌的密码和会话。": "I understand this resets the global account password and sessions across all brands.",
  "申请积分": "Requested points", "积分来源": "Points source", "来源": "Source", "特别号码": "Special numbers", "开奖来源": "Draw source",
  "全部管理页面": "All admin pages", "关闭导航": "Close navigation", "移动端主导航": "Mobile main navigation", "用户": "Users", "期次": "Periods", "审核": "Review", "更多": "More",
  "域名绑定不配置 DNS、证书或重定向；展示仅支持固定预设与中英文文案，不支持任意 CSS、HTML 或上传素材。": "Domain binding does not configure DNS, certificates, or redirects. Presentation supports fixed presets and bilingual copy, not arbitrary CSS, HTML, or uploaded assets.",
  "品牌": "Brand", "时区": "Time zone",
  "此页显示真实成员；账号权限、积分账本、认证设置和投注已接入，提现仍为原型。": "This page shows live members. Account permissions, the points ledger, authentication settings, and betting are connected; withdrawals remain a prototype.", "会话由同源 HttpOnly Cookie 维护，此页面不会保存访问令牌。": "The session uses a same-origin HttpOnly cookie. This page does not store access tokens.", "请先登录后台账号。": "Sign in to the admin account first.", "请先登录后台账号查看代理配置。": "Sign in to view agent settings.", "当前账号没有此品牌的加入码查看权限。": "This account cannot view join codes for this brand.", "请先登录后台账号，才能配置站内通知模板。": "Sign in to configure in-app notification templates.", "请先登录后台账号，才能查看真实运营报表。": "Sign in to view live operations reports.", "请先登录后台账号，才能查看真实投递记录。": "Sign in to view live delivery records.", "请先登录后台账号，才能管理真实角色和权限。": "Sign in to manage live roles and permissions.", "请登录并选择有钱包查看权限的品牌。": "Sign in and select a brand with wallet viewing permission.", "请选择一个有权访问的品牌以查看注单。": "Choose an accessible brand to view orders.", "请先登录后台账号。投注策略和注单均从真实 API 读取，不使用演示数据。": "Sign in first. Betting policies and orders load from the live API; demo data is not used.", "前往登录": "Go to sign in",
  "真实规则模拟": "Live rule simulation", "请先登录后台并选择品牌，才能查看真实规则版本和模拟。": "Sign in and select a brand to view live rule versions and simulations.",   "真实期次计划": "Live period plan", "请先登录后台账号并选择品牌，才能保存计划、生成和查询期次。": "Sign in and select a brand to save plans, generate, and view periods.", "查看期次时间线、来源校验与开奖操作": "View period timeline, source checks, and draw actions", "人工开奖": "Manual draw", "生成期次": "Generate period", "复制期次编号": "Copy period ID", "投注开始": "Betting opens", "投注截止": "Betting closes", "开奖时间": "Draw time", "结算完成": "Settlement complete", "投注订单": "Bet orders", "投注积分": "Bet points", "关联规则": "Linked rule", "品牌时区": "Brand time zone", "优先级与最近检查时间": "Priority and last check", "切换记录": "Change history", "主来源 · API": "Primary source · API", "备用来源 · API": "Backup source · API", "上次校验": "Last checked", "健康": "Healthy", "单人操作，必须记录审计原因": "Single operator; an audit reason is required", "录入": "Enter result", "当前开奖结果": "Current draw result", "上一期": "Previous period", "已确认": "Confirmed", "来源：主开奖源": "Source: primary draw source", "校验项：号码范围 ✓　重复校验 ✓　签名 ✓": "Checks: number range ✓ duplicates ✓ signature ✓", "确认时间": "Confirmed at", "确认并结算 →": "Confirm and settle →", "状态时间线": "Status timeline", "开奖结果已确认": "Draw result confirmed", "来源结果通过校验": "Source result passed validation", "期次投注已截止": "Period betting closed", "人工开奖和结果纠正均属于高风险演示操作。确认前必须查看影响范围并填写原因。": "Manual draws and result corrections are high-risk demos. Review the impact and enter a reason before confirming.",
  "周期规则、试算与记录调整概览": "Overview of cycle rules, estimates, and record adjustments", "新建佣金规则": "Create commission rule", "本周期预计佣金": "Estimated commission this cycle", "待结算记录": "Records awaiting settlement", "覆盖 142 位代理": "Covers 142 agents", "已结算佣金": "Settled commission", "本月累计 · 演示数据": "Month to date · demo data", "佣金规则版本": "Commission rule versions", "修改会创建新版本，已结算历史记录保留原始金额": "Changes create a new version; settled history retains original amounts", "版本历史 →": "Version history →", "只计算有效输钱注单 · 品牌范围": "Counts eligible losing orders only · brand scope", "输赢模式": "Win/loss model", "比例": "Rate", "周期": "Cycle", "每周": "Weekly", "每月": "Monthly", "生效中 · v4": "Active · v4", "生效中 · v2": "Active · v2", "详情 →": "Details →", "基于有效投注流水 · 品牌范围": "Based on eligible betting turnover · brand scope", "流水模式": "Turnover model", "近期佣金记录": "Recent commission records", "人工修正会建立独立 adjustment 记录": "Manual corrections create a separate adjustment record", "导出 ↓": "Export ↓", "代理": "Agent", "模式": "Model", "计算基数": "Calculation base", "佣金积分": "Commission points", "待结算": "Pending settlement", "已结算": "Settled",
  "真实代理配置与历史记录；佣金核算已接入，审核和派发尚未实现。": "Live agent settings and history; commission calculations are connected, but approval and payouts are not implemented.", "真实注单结果及账本流水；汇总余额不是完整逐账户对账证明。": "Live order results and ledger entries; aggregate balances are not a complete account-by-account reconciliation.", "真实站内通知投递记录；外部发送渠道尚未接入。": "Live in-app notification delivery records; external channels are not connected.", "真实版本化站内通知模板；已生成消息保留原文案，不触发新通知或资金变化。": "Live versioned in-app notification templates; generated messages retain their original copy and do not send notifications or change funds.", "人工充值、冻结、调整与账本为真实操作；提现尚未接入。": "Manual deposits, freezes, adjustments, and ledger actions are live; withdrawals are not connected.",
  "加入码": "Join codes", "冻结": "Frozen", "已过期": "Expired", "已注销": "Cancelled", "待审核": "Awaiting review", "已通过": "Approved", "已驳回": "Rejected",
  "已登录管理员账号": "Signed in as admin", "已退出管理员账号": "Signed out of admin", "成员资料已更新": "Member details updated", "该成员的品牌会话已踢出": "The member's brand session was revoked", "全局密码已重置，所有品牌会话已撤销": "Global password reset; sessions revoked across all brands", "审核失败": "Review failed", "提交失败": "Submission failed",
  "帮助服务尚未连接。": "Help is unavailable because no help service is connected.", "页面范围以各模块状态为准；积分为站内内部积分，不接入外部支付。": "Scope varies by module. Points are internal to the platform; external payments are not connected.", "认证设置、品牌展示和域名绑定为真实配置；积分为站内内部积分，不接入外部支付。": "Authentication, brand presentation, and domain binding are live settings. Points are internal to the platform; external payments are not connected.", "充值、冻结、调整和账本是后台操作；资金使用站内内部积分，不接入外部支付。提现资格和状态流程已接入。": "Deposits, freezes, adjustments, and ledger entries are admin operations using internal points; external payments are not connected. Withdrawal qualification and status workflows are connected.", "审计日志读取当前品牌的真实后台记录。": "Audit logs load live admin records for the selected brand.", "账号和角色权限按当前品牌权限执行真实后台变更。": "Account and role changes use the permissions for the selected brand.",
  "安全 / 审计轨迹": "SECURITY / AUDIT TRAIL", "平台 / 品牌配置": "PLATFORM / BRAND CONFIG", "操作控制台": "OPERATIONS CONSOLE",
};
Object.assign(englishUi, {
  "日月归档": "Daily and monthly archives",
  "自动归档任务": "Automatic archive tasks",
  "请登录并选择有归档查看权限的品牌。": "Sign in and choose a brand with archive viewing permission.",
  "演": "D", "品": "B",
  "请先选择真实后台品牌。": "Select a live admin brand first.",
  "请先登录后台账号，才能查看和操作真实积分。": "Sign in with an administrator account to view and manage live points.",
  "帮助中心 ↗": "Help center ↗",
  "只读接口 GET /api/v1/context 暂不可用；该接口仅用于只读配置状态，不影响管理员认证和真实用户目录。": "Read-only GET /api/v1/context is unavailable. It reports configuration only and does not affect admin authentication or the live user directory.",
  "已登录 ·": "Signed in ·",
  "使用后台管理员账号登录。会话由同源 HttpOnly Cookie 维护，此页面不会保存访问令牌。": "Sign in with an administrator account. The session uses a same-origin HttpOnly cookie; this page does not store access tokens.",
  "第": "Page", "页 · 每页最多 100 条": "· up to 100 records per page",
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
  | "日月归档"
  | "自动归档任务"
  | "佣金和奖励"
  | "佣金派发"
  | "佣金更正"
  | "人工奖励"
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
  { name: "日月归档", icon: "▣", group: "管理" },
  { name: "自动归档任务", icon: "◷", group: "管理" },
  { name: "佣金和奖励", icon: "↗", group: "资金" },
  { name: "佣金派发", icon: "⇧", group: "资金" },
  { name: "佣金更正", icon: "⇄", group: "资金" },
  { name: "人工奖励", icon: "✧", group: "资金" },
  { name: "报表和对账", icon: "▥", group: "管理" },
  { name: "账号与权限", icon: "♧", group: "管理" },
  { name: "审计日志", icon: "≡", group: "管理" },
  { name: "通知投递", icon: "♧", group: "管理" },
  { name: "通知模板", icon: "♧", group: "管理" },
  { name: "风控与合规", icon: "⚖", group: "管理" },
];
const page = ref<Page>("工作台");
const brandMenu = ref(false);
const mobileMore = ref(false);
const search = ref("");
const pageSearch = ref("");
const notice = ref<ReturnType<typeof message> | string>("");
const api = createAdminApi();
const account = ref<AdminAccount | null>(null);
const authLoading = ref(true);
const authError = ref<string | ReturnType<typeof message>>("");
const loginIdentifier = ref("");
const loginPassword = ref("");
const loginBusy = ref(false);
const loginIdempotencyKey = ref(createIdempotencyKey());
watch([loginIdentifier, loginPassword], () => {
  // Editing credentials is a new login intent, not a replay of the old body.
  if (!loginBusy.value) loginIdempotencyKey.value = createIdempotencyKey();
});
const adminBrands = ref<AdminBrand[]>([]);
const selectedBrandId = ref("");
const financeMemberId = ref("");
let adminBrandLoadGeneration = 0;
const correctionSettlementPeriod = ref<{brandId:string;periodId:string;nonce:number}|null>(null);
const brand = computed(
  () =>
    adminBrands.value.find((item) => item.id === selectedBrandId.value)?.name ??
    ui("请选择品牌"),
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
watch(() => [account.value?.id, selectedBrandId.value, JSON.stringify(account.value?.permissions_by_brand), JSON.stringify(account.value?.platform_permissions)] as const,
  (next, previous) => {
    if (previous[0] && previous[1] && previous[0] === next[0] && (previous[1] !== next[1] || previous[2] !== next[2] || previous[3] !== next[3])) {
      hideCommissionPaymentIntentsForScope(previous[0], previous[1]);
      hideCommissionCorrectionIntentsForScope(previous[0], previous[1]);
    }
  });
function skinTheme():BrandTheme|null{const e=skin.value;return e?{...defaultBrand,name:e.display_name,logoText:e.logo_text,logoUrl:e.logo_url??undefined,faviconUrl:e.favicon_url??undefined,primary:e.primary_color,accent:e.accent_color,success:e.success_color,warning:e.warning_color,danger:e.danger_color,fontFamily:e.font_family,fontScale:e.font_scale,radius:e.radius,shadow:e.shadow}:null}
const skinStyle=computed(()=>{const theme=skinTheme();return theme?{...buildBrandCssTokens(theme),fontFamily:"var(--font-family)",fontSize:"calc(13px * var(--brand-font-scale-factor, 1))"}:{}});
const skinLogo=computed(()=>safeBrandAssetUrl(skin.value?.logo_url));
const failedSkinLogo=ref<string|null>(null);
watch(skinLogo,()=>{failedSkinLogo.value=null});
function skinLogoError(event:Event){if(event.target instanceof HTMLImageElement&&event.target.getAttribute('src')===skinLogo.value)failedSkinLogo.value=skinLogo.value}
function acceptPresentation(value:{accountId:string;record:BrandPresentationRecord}){if(account.value?.id!==value.accountId||selectedBrandId.value!==value.record.brand_id)return;if(presentationSkin.value?.accountId===value.accountId&&presentationSkin.value.record.brand_id===value.record.brand_id&&presentationSkin.value.record.version>value.record.version)return;presentationSkin.value=value;const theme=skinTheme();if(theme)applyBrandPresentation({brand:theme,paused:value.record.status==='paused',availableLanguages:value.record.effective.available_locales},document.querySelector<HTMLElement>('.app-shell')??document.documentElement)}
watch(()=>[account.value?.id,selectedBrandId.value],()=>{const generation=++presentationReadGeneration;presentationSkin.value=null;document.querySelector('link[rel="icon"][data-brand-favicon]')?.remove();const current=account.value,id=selectedBrandId.value;if(!current||!id||!brandPresentationPermissions(current,id).view)return;void presentationApi.get(id).then(record=>{if(generation===presentationReadGeneration&&account.value?.id===current.id&&selectedBrandId.value===id)acceptPresentation({accountId:current.id,record})}).catch(cause=>{if(generation===presentationReadGeneration&&account.value?.id===current.id&&selectedBrandId.value===id&&cause instanceof AdminApiError&&cause.status===401)clearAdminData()})});
const members = ref<Member[]>([]);
const filteredMembers = computed(() => {
  const query = search.value.trim().toLowerCase();
  return members.value.filter((item) => !query ||
    `${item.display_name} ${item.username} ${item.phone} ${item.id} ${item.global_user_id}`.toLowerCase().includes(query));
});
const membersLoading = ref(false);
const membersError = ref<string | ReturnType<typeof message>>("");
const memberOffset = ref(0);
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
onMounted(() => {
  void loadBrandContext();
  void restoreAdminSession();
});
const groups = computed(() => [...new Set(nav.map((item) => item.group))]);
const canViewJoinCodes = computed(() => {
  if (!account.value || !selectedBrandId.value) return false;
  return brandPermissionSet(account.value, selectedBrandId.value).has("join_code.view.brand");
});
const visibleNav = computed(() => nav.filter((item) => item.name !== "加入码" || canViewJoinCodes.value));
const go = (target: Page) => {
  page.value = target;
  if (target !== "资金与账本") financeMemberId.value = "";
  pageSearch.value = "";
  notice.value = "";
  mobileMore.value = false;
};
const canViewMemberWallet = computed(() => Boolean(account.value && selectedBrandId.value && canViewWallet(account.value, selectedBrandId.value)));
function openMemberWallet(member: Member) {
  if (!canViewMemberWallet.value) return;
  financeMemberId.value = member.id;
  go("资金与账本");
}
function openSearchedPage() {
  const query = pageSearch.value.trim().toLowerCase();
  const target = visibleNav.value.find((item) => ui(item.name).toLowerCase() === query);
  if (target) go(target.name);
}
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
  return brandPermissionSet(account.value, selectedBrandId.value).has(permission);
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
	clearAllArchiveIntents();
	clearAllReportArchiveTaskRetryIntents();
	clearAllReportArchivePolicyIntents();
	clearAllPendingCommissionAdjustments();
	clearAllPendingCommissionCycleWrites();
	clearAllCommissionPaymentIntents();
	clearAllRewardIntents();
	clearAllCommissionCorrectionIntents();
	clearAllPendingCommissionWrites();
	clearAllPendingPresentationWrites();
	clearAllPendingComplianceIntents();
	clearAllPendingBrandDomainsWrites();
	presentationReadGeneration+=1;presentationSkin.value=null;
  adminBrandLoadGeneration += 1;
  clearAllPendingAgentWrites();
  clearAllPendingJoinCodeWrites();
  clearAllPendingDeliveryRetries();
  clearAllPendingTemplateWrites();
  clearPendingReconciliationWrites();
  clearAllPendingBrandOperationWrites();
  correctionSettlementPeriod.value = null;
  pageSearch.value = "";
  search.value = "";
  financeMemberId.value = "";
  account.value = null;
  adminBrands.value = [];
  selectedBrandId.value = "";
  members.value = [];
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
    if (result.account.super_admin) {
      clearAdminData();
      authError.value = message("平台超级管理员请使用平台管理入口。", "Platform super administrators must use the platform admin portal.");
      return;
    }
    if (account.value?.id !== result.account.id) {
      adminBrandLoadGeneration += 1;
      clearAllArchiveIntents();
      clearAllReportArchiveTaskRetryIntents();
      clearAllReportArchivePolicyIntents();
      clearAllPendingCommissionCycleWrites();
      clearAllPendingCommissionAdjustments();
      clearAllCommissionPaymentIntents();
      clearAllRewardIntents();
      clearAllCommissionCorrectionIntents();
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
    if (result.account.super_admin) {
      clearAdminData();
      authError.value = message("平台超级管理员请使用平台管理入口。", "Platform super administrators must use the platform admin portal.");
      return;
    }
    if (account.value?.id !== result.account.id) {
      adminBrandLoadGeneration += 1;
      clearAllArchiveIntents();
      clearAllReportArchiveTaskRetryIntents();
      clearAllReportArchivePolicyIntents();
      clearAllPendingCommissionCycleWrites();
      clearAllPendingCommissionAdjustments();
      clearAllCommissionPaymentIntents();
      clearAllRewardIntents();
      clearAllCommissionCorrectionIntents();
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
    authError.value = error instanceof AdminApiError && error.code === "ADMIN_ENTRY_MISMATCH"
      ? message("平台超级管理员请使用平台管理入口。", "Platform super administrators must use the platform admin portal.")
      : apiErrorText(error);
  } finally {
    loginBusy.value = false;
  }
};
const onBrandOperationChanged = (change: { accountId: string }) => {
  if (account.value?.id === change.accountId) void loadBrands();
};
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
  financeMemberId.value = "";
  search.value = "";
  brandMenu.value = false;
  memberOffset.value = 0;
  members.value = [];
  await loadMembers();
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
    await loadMembers();
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
    await loadMembers();
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
    await loadMembers();
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
</script>

<template>
  <div v-if="authLoading || !account" class="login-entry">
    <header class="login-entry__topbar">
      <a class="login-entry__brand" href="#" aria-label="Northstar admin">
        <span class="login-entry__mark" aria-hidden="true">N</span>
        <span><b>northstar</b><small>ADMINISTRATION</small></span>
      </a>
      <label class="admin-language">
        <span class="sr-only">{{ t('语言', 'Language') }}</span>
        <select data-testid="admin-language" :aria-label="t('语言', 'Language')" :value="locale" @change="setLocale(($event.target as HTMLSelectElement).value)">
          <option v-for="code in availableLocales" :key="code" :value="code">{{ code === 'en' ? 'English' : '简体中文' }}</option>
        </select>
      </label>
    </header>
    <main class="login-entry__main">
      <section v-if="authLoading" class="login-entry__loading" role="status" aria-live="polite">
        <span class="login-entry__spinner" aria-hidden="true"></span>
        <p>{{ ui("正在检查管理员登录状态…") }}</p>
      </section>
      <article v-else class="login-entry__card" aria-labelledby="admin-login-title">
        <div class="login-entry__eyebrow">ADMINISTRATION</div>
        <h1 id="admin-login-title">{{ ui("管理员登录") }}</h1>
        <p class="login-entry__description">{{ ui("使用后台管理员账号登录。会话由同源 HttpOnly Cookie 维护，此页面不会保存访问令牌。") }}</p>
        <form class="login-entry__form" @submit.prevent="login">
          <label class="login-entry__field">{{ ui("账号") }}<input v-model="loginIdentifier" class="field" autocomplete="username" required /></label>
          <label class="login-entry__field">{{ ui("密码") }}<input v-model="loginPassword" class="field" type="password" autocomplete="current-password" required /></label>
          <p v-if="authError" class="form-error" role="alert">{{ t(authError) }}</p>
          <button class="button button-primary" :disabled="loginBusy">
            {{ loginBusy ? ui("登录中…") : ui("登录并加载真实成员") }}
          </button>
        </form>
      </article>
      <p class="login-entry__footer">{{ ui("品牌管理员 · 运营 · 财务 · 客服") }}</p>
    </main>
  </div>
  <div v-else class="app-shell" :style="skinStyle">
    <aside class="sidebar">
      <a class="brand-lockup" href="#" @click.prevent="go('工作台')"
        ><img v-if="skinLogo&&failedSkinLogo!==skinLogo" class="presentation-logo" :src="skinLogo" :alt="skin?.logo_text" crossorigin="anonymous" referrerpolicy="no-referrer" @error="skinLogoError"/>
        <span v-else class="brand-mark">{{skin?.logo_text??'N'}}</span>
        <span><b>{{skin?.display_name??'northstar'}}</b><small>OPERATIONS CONSOLE</small></span></a
      >
      <div v-if="page !== '工作台'" class="demo-chip">
        <span class="pulse"></span>{{ ui("内部积分，未接入真实支付") }} <span class="demo-chip-end">·</span>
      </div>
      <div class="brand-switch-wrap">
        <button
          class="brand-switch"
          @click="brandMenu = !brandMenu"
          :aria-label="ui('切换品牌')"
        >
          <span class="brand-avatar">{{ brand.slice(0, 1) }}</span
          ><span class="brand-switch-text"
            ><small>{{ ui("真实品牌") }}</small
            ><b>{{ !selectedBrandId ? ui("请选择品牌") : brand }}</b></span
          ><span class="chevron">⌄</span>
        </button>
        <div v-if="brandMenu" class="brand-dropdown">
          ><button
              v-for="item in adminBrands"
              :key="item.id"
              @click="selectBrand(item.id)"
            >
              {{ item.name }} <span v-if="selectedBrandId === item.id">✓</span
              ><small>{{ item.code }} · {{ item.status }}</small>
            </button>
            <p v-if="!adminBrands.length">{{ ui("没有可访问的品牌") }}</p>
        </div>
      </div>
      <nav class="side-nav" :aria-label="ui('主导航')">
        <template v-for="group in groups" :key="group"
          ><p class="nav-heading">{{ ui(group) }}</p>
          <button
            v-for="item in visibleNav.filter((entry) => entry.group === group)"
            :key="item.name"
            class="nav-item"
            :aria-label="ui(item.name)"
            :title="ui(item.name)"
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
        <button class="support-link" @click="toast('帮助服务尚未连接。')">
          ◌ <span>{{ ui("帮助与反馈") }}</span><span class="external">↗</span>
        </button>
        <div class="profile">
          <div class="profile-avatar">{{ account.id.slice(0, 1).toUpperCase() }}</div>
          <span class="profile-name"
            ><b>{{ account.id }}</b
            ><small>{{ account.super_admin ? ui("超级管理员") : ui("管理员") }}</small></span
          ><button
            class="dots"
            :aria-label="ui('退出登录')"
            @click="logout"
          > {{ ui("退出") }}</button
          >
        </div>
      </div>
    </aside>

    <main class="main-shell">
      <header class="topbar">
        <div class="breadcrumbs">
          <span>{{ ui("运营控制台") }}</span><span class="crumb-slash">/</span
          ><b>{{ ui(page) }}</b
          ><span class="prototype-badge">{{ ui("品牌后台") }}</span>
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
              v-model="pageSearch"
              list="admin-page-options"
              :placeholder="t('查找管理页面', 'Find an admin page')"
              :aria-label="t('查找管理页面', 'Find an admin page')"
              @change="openSearchedPage"
              @keydown.enter.prevent="openSearchedPage"
              @keydown.esc="pageSearch = ''"
            />
            <datalist id="admin-page-options">
              <option v-for="item in visibleNav" :key="item.name" :value="ui(item.name)" />
            </datalist></label
          ><button
            class="icon-button"
            :aria-label="ui('通知')"
            @click="go('通知投递')"
          >
            ♧</button
          ><span class="top-divider"></span
          ><button class="help-button" @click="toast('帮助服务尚未连接。')"> {{ ui("帮助中心 ↗") }} </button>
        </div>
      </header>
      <div v-if="locale === 'en'" class="admin-locale-coverage" role="status">
        {{ t("部分功能面板来自独立模块，可能仍显示中文。", "Some feature panels are provided by separate modules and may still appear in Chinese.") }}
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
        >
      </div>
      <div class="directory-brand-bar">
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
          <div class="page-heading">
            <div>
              <div class="eyebrow">PLATFORM / BRAND CONFIG</div>
              <h1>{{ ui("品牌和域名") }}</h1>
              <p>{{ ui("真实品牌列表、运行状态、展示与域名绑定") }}</p>
            </div>
          </div>
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
                    aria-describedby="member-search-scope"
                  />
                  <button v-if="search" type="button" class="button button-secondary" @click="search = ''">{{ t('清除筛选', 'Clear filter') }}</button>
                </div>
              </div>
              <p id="member-search-scope" class="member-search-note">{{ t('仅筛选当前已加载页面的成员；可翻页查看其他成员。', 'Filters members on the currently loaded page only. Use pagination to view other members.') }}</p>
              <div v-if="membersLoading" class="directory-state"> {{ ui("正在读取品牌成员…") }} </div>
              <div v-else-if="!members.length" class="directory-state"> {{ ui("该品牌当前没有可显示的成员。") }} </div>
              <div v-else-if="!filteredMembers.length" class="directory-state" role="status">{{ t('本页没有匹配的成员，请修改或清除筛选。', 'No matching members on this page. Change or clear the filter.') }}</div>
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
                      v-for="m in filteredMembers"
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
                          ><button v-if="canViewMemberWallet" class="text-button" @click="openMemberWallet(m)">{{ t('查看积分', 'View points') }}</button
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
          <p>{{ ui("请先登录后台并选择品牌，才能查看真实规则版本和模拟。") }}</p>
        </div>
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

      <section v-else-if="page === '日月归档'" class="page-content">
        <ReportArchivesManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">{{ ui("请登录并选择有归档查看权限的品牌。") }}</div>
      </section>

      <section v-else-if="page === '自动归档任务'" class="page-content">
        <ReportArchiveTasksManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">{{ ui("请登录并选择有归档查看权限的品牌。") }}</div>
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
          :initial-member-id="financeMemberId"
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

      <section v-else-if="page === '佣金派发'" class="page-content">
        <CommissionPaymentsManagement v-if="account && selectedBrandId" :key="`commission-payments:${account.id}:${selectedBrandId}`"
          :account="account" :brand-id="selectedBrandId" :brand-status="selectedBrand?.status" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ ui("佣金派发") }}</h1>
          <p>{{ account ? ui("请先选择真实后台品牌。") : ui("请先登录后台账号查看真实佣金派发记录。") }}</p>
        </div>
      </section>

      <section v-else-if="page === '佣金更正'" class="page-content">
        <CommissionCorrectionsManagement v-if="account && selectedBrandId" :key="`commission-corrections:${account.id}:${selectedBrandId}`"
          :account="account" :brand-id="selectedBrandId" :brand-status="selectedBrand?.status" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ ui("佣金更正") }}</h1>
          <p>{{ account ? ui("请先选择真实后台品牌。") : ui("请先登录后台账号查看佣金更正计划和执行。") }}</p>
        </div>
      </section>

      <section v-else-if="page === '人工奖励'" class="page-content">
        <RewardsManagement v-if="account && selectedBrandId" :key="`rewards:${account.id}:${selectedBrandId}`"
          :account="account" :brand-id="selectedBrandId" :brand-status="selectedBrand?.status" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ ui("人工奖励") }}</h1>
          <p>{{ account ? ui("请先选择真实后台品牌。") : ui("请先登录后台账号查看人工奖励订单。") }}</p>
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
        <AuditManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else-if="account" class="panel directory-state"><h1>{{ ui("审计日志") }}</h1><p>{{ ui("请先从侧栏选择品牌；审计请求始终携带明确的 X-Brand-ID。") }}</p></div>
      </section>

      <footer v-if="page !== '加入码' && page !== '工作台'" class="page-footer">
        <span>{{ brand }} {{ ui("运营控制台") }} <b>·</b> {{ ui("内部积分，未接入真实支付") }}</span
        ><span>{{
          page === "用户和成员"
              ? ui("成员创建与管理为真实操作；投注已接入。")
            : page === "代理树"
              ? t("真实代理配置与历史记录；佣金核算和独立派发后台已接入，派发运行开关默认关闭。", "Live agent settings and history; commission calculations and the separate payout console are connected, with its runtime switch off by default.")
            : page === "报表和对账"
              ? ui("真实注单结果及账本流水；汇总余额不是完整逐账户对账证明。")
            : page === "通知投递"
              ? ui("真实站内通知投递记录；外部发送渠道尚未接入。")
            : page === "通知模板"
              ? ui("真实版本化站内通知模板；已生成消息保留原文案，不触发新通知或资金变化。")
            : page === "审计日志"
              ? ui("审计日志读取当前品牌的真实后台记录。")
            : page === "账号与权限"
              ? ui("账号和角色权限按当前品牌权限执行真实后台变更。")
            : page === "品牌和域名"
              ? ui("认证设置、品牌展示和域名绑定为真实配置；积分为站内内部积分，不接入外部支付。")
            : page === "资金与账本"
              ? ui("充值、冻结、调整和账本是后台操作；资金使用站内内部积分，不接入外部支付。提现资格和状态流程已接入。")
            : ui("页面范围以各模块状态为准；积分为站内内部积分，不接入外部支付。")
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

  </div>
</template>

<style scoped>
.login-entry { min-height: 100vh; color: #20283a; background: radial-gradient(ellipse at 50% 0, #edf2ff 0, #f7f8fc 44%, #f8f9fb 100%); }
.login-entry__topbar { display: flex; min-height: 76px; align-items: center; justify-content: space-between; gap: 16px; padding: 14px clamp(20px, 5vw, 72px); border-bottom: 1px solid #e6eaf2; background: rgba(255,255,255,.78); }
.login-entry__brand { display: flex; align-items: center; gap: 11px; color: inherit; text-decoration: none; }
.login-entry__mark { display: grid; width: 40px; height: 40px; place-items: center; border-radius: 13px; background: linear-gradient(145deg,#6879ee,#4658ce); color: white; font-size: 21px; font-weight: 800; box-shadow: 0 5px 15px #5969df30; }
.login-entry__brand b,.login-entry__brand small { display: block; }
.login-entry__brand b { font-size: 16px; letter-spacing: -.5px; }
.login-entry__brand small { margin-top: 3px; color: #8992a4; font-size: 9px; letter-spacing: 1.2px; }
.login-entry__main { display: grid; min-height: calc(100vh - 77px); align-content: center; justify-items: center; gap: 24px; padding: 42px 20px 64px; }
.login-entry__card { width: min(100%, 430px); padding: clamp(25px, 5vw, 38px); border: 1px solid #e8ebf2; border-radius: 18px; background: white; box-shadow: 0 18px 55px rgba(37,49,88,.09); }
.login-entry__eyebrow { color: #6372d8; font-size: 10px; font-weight: 700; letter-spacing: 1.55px; }
.login-entry__card h1 { margin: 10px 0 8px; color: #20283a; font-size: clamp(23px, 5vw, 28px); letter-spacing: -.7px; }
.login-entry__description { margin: 0; color: #737b8c; line-height: 1.65; }
.login-entry__form { display: grid; gap: 17px; margin-top: 27px; }
.login-entry__field { display: grid; gap: 8px; color: #394256; font-size: 13px; font-weight: 600; }
.login-entry__field .field { width: 100%; min-height: 44px; }
.login-entry__form .form-error { margin: -3px 0 0; }
.login-entry__form .button { width: 100%; min-height: 45px; margin-top: 2px; }
.login-entry__loading { display: grid; justify-items: center; gap: 14px; color: #687287; }
.login-entry__loading p { margin: 0; }
.login-entry__spinner { width: 24px; height: 24px; border: 2px solid #dfe4f1; border-top-color: #5969df; border-radius: 50%; animation: login-entry-spin .75s linear infinite; }
.login-entry__footer { margin: 0; color: #8992a4; font-size: 12px; text-align: center; }
@keyframes login-entry-spin { to { transform: rotate(360deg); } }
@media (max-width: 480px) { .login-entry__topbar { min-height: 66px; padding-inline: 17px; } .login-entry__main { min-height: calc(100vh - 67px); padding: 30px 16px 44px; } .login-entry__card { border-radius: 15px; } }
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
