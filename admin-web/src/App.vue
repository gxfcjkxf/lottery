<script setup lang="ts">
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import DirectorySearch from "@lottery/shared/DirectorySearch.vue"
import PaginationControls from "@lottery/shared/PaginationControls.vue"
import ManagementFrame from "@lottery/shared/ManagementFrame.vue"
import AccountProfileDialog from "@lottery/shared/AccountProfileDialog.vue"
import ChangePasswordDialog from "@lottery/shared/ChangePasswordDialog.vue"
import BrandLockup from '@lottery/shared/BrandLockup.vue'
import AccountMenu from "@lottery/shared/AccountMenu.vue";
import LoginEntry from '@lottery/shared/LoginEntry.vue'
import LoginCredentials from '@lottery/shared/LoginCredentials.vue'
import { vWriteGuard } from "@lottery/shared";
import { readAdminEnvelope } from './admin-api'
import { associationContextKey } from '@lottery/shared';
import { provide as provideAssociationContext } from 'vue';
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon, { type SiteIconName } from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from '@lottery/shared';
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
import { useSessionReplacement } from "@lottery/shared";
import TablePagination from "@lottery/shared/TablePagination.vue";
import DetailModal from "@lottery/shared/DetailModal.vue";
import LanguageSwitcher from '@lottery/shared/LanguageSwitcher.vue';
import {
  computed,
  defineAsyncComponent,
  onMounted,
  nextTick,
  ref,
} from "vue";
import { brandPermissionSet } from "./brand-permissions";
import { canViewWallet } from "./finance-api";
const AccessManagement = defineAsyncComponent(() => import("./AccessManagement.vue"));
import MemberActionsMenu from "@lottery/shared/MemberActionsMenu.vue";
import MemberProvision from "./MemberProvision.vue";
import BrandOperation from "./BrandOperation.vue";
import BrandPresentation from "./BrandPresentation.vue";
import { clearAllPendingComplianceIntents } from "./compliance-state";
const BrandDomains = defineAsyncComponent(() => import("./BrandDomains.vue"));
import {clearAllPendingBrandDomainsWrites} from "./brand-domains-state";
import {watch,watchEffect} from "vue";
import {buildBrandCssTokens,defaultBrand,safeBrandAssetUrl,applyBrandPresentation,applyBrowserBranding,nativeLocaleName,type BrandTheme} from "@lottery/shared";
import {createBrandPresentationApi,brandPresentationPermissions,type BrandPresentationRecord} from "./brand-presentation-api";
import {clearAllPendingPresentationWrites} from "./brand-presentation-state";
const FinanceManagement = defineAsyncComponent(() => import("./FinanceManagement.vue"));
const WithdrawalManagement = defineAsyncComponent(() => import("./WithdrawalManagement.vue"));
import BalanceRepair from "./BalanceRepair.vue";

import PeriodSchedules from "./PeriodSchedules.vue";
import DrawManagement from "./DrawManagement.vue";
import BetOrderManagement from "./BetOrderManagement.vue";
import BetPolicySettings from "./BetPolicySettings.vue";
import PeriodCancellation from "./PeriodCancellation.vue";
import OverviewPanel from "./OverviewPanel.vue";
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
import { adminKey, useAdminI18n } from "./i18n";
const RulesPage = defineAsyncComponent(() => import("./RulesPage.vue"));
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

const { t, message, display, locale, availableLocales, setLocale, configure, resetBrand } = useAdminI18n();
const ui = (key: string) => t(adminKey(key));
const navLabels: Record<string, string> = {
  "首页": "overview.home",
  "系统管理": "admin.console.group.7",
  "佣金与奖励": "admin.console.group.6",
  "报表与归档": "admin.console.group.5",
  "资金管理": "admin.console.group.4",
  "彩种与投注": "admin.console.group.3",
  "会员与推广": "admin.console.group.2",
  "品牌设置": "admin.settings.title", "用户和成员": "admin.nav.users",
  "代理管理": "admin.nav.agents", "代理列表": "admin.nav.agentList", "代理设置": "admin.nav.agentSettings", "规则配置": "admin.copy.db7e35eb",
  "期次和开奖": "admin.copy.051cdfc6", "注单和异常": "admin.copy.f7a666c3", "资金与账本": "admin.copy.783ae8f1",
  "批量对账": "admin.copy.2b3decaf", "日月归档": "admin.copy.d8f53155", "自动归档任务": "admin.copy.6477d963",
  "佣金和奖励": "admin.copy.5c8e261e", "佣金派发": "admin.copy.61a05916", "佣金更正": "admin.copy.b095cecc",
  "人工奖励": "admin.copy.3fecab01", "报表和对账": "admin.copy.931e1e2b", "账号与权限": "admin.copy.9c6ee39c",
  "审计日志": "admin.copy.315a9366", "通知投递": "admin.copy.63871b72", "通知模板": "admin.copy.e4f1dc7c",
 "平台": "admin.copy.ed823559",
  "运营": "admin.copy.4b7fa234", "资金": "admin.copy.c9a0911c", "管理": "admin.copy.0f05b3f1",
};
const navLabel = (value: string) => t(adminKey(navLabels[value]!));

type Page =
  | "首页"
  | "品牌设置"
  | "用户和成员"
  | "代理列表"
  | "代理设置"
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
  | "通知模板";
const nav: { name: Page; icon: SiteIconName; group: string | null }[] = [
  { name: "首页", icon: "home", group: null },
  { name: "品牌设置", icon: "brands", group: null },
  { name: "用户和成员", icon: "users", group: null },
  { name: "代理列表", icon: "network", group: "代理管理" },
  { name: "代理设置", icon: "settings", group: "代理管理" },
  { name: "规则配置", icon: "rules", group: "彩种与投注" },
  { name: "期次和开奖", icon: "calendar", group: "彩种与投注" },
  { name: "注单和异常", icon: "ticket", group: "彩种与投注" },
  { name: "资金与账本", icon: "wallet", group: "资金管理" },
  { name: "批量对账", icon: "reconcile", group: "资金管理" },
  { name: "日月归档", icon: "archive", group: "报表与归档" },
  { name: "自动归档任务", icon: "schedule", group: "报表与归档" },
  { name: "佣金和奖励", icon: "trend", group: "佣金与奖励" },
  { name: "佣金派发", icon: "send", group: "佣金与奖励" },
  { name: "佣金更正", icon: "transfer", group: "佣金与奖励" },
  { name: "人工奖励", icon: "gift", group: "佣金与奖励" },
  { name: "报表和对账", icon: "chart", group: "报表与归档" },
  { name: "账号与权限", icon: "shield", group: "系统管理" },
  { name: "审计日志", icon: "audit", group: "系统管理" },
  { name: "通知投递", icon: "send", group: "系统管理" },
  { name: "通知模板", icon: "mail", group: "系统管理" },
];
const page = ref<Page>("首页");
const agentRulesRevision = ref(0);
const financeView = ref<'wallet' | 'recharges' | 'withdrawals' | 'policies' | 'repair'>('wallet');
const memberCreateOpen = ref(false);
const brandSettingsView = ref<'operation' | 'presentation' | 'domains'>('operation');
const periodView = ref<'schedules' | 'draws' | 'settlement' | 'correction' | 'cancellation'>('schedules');
const brandSettingsTabs = computed(() => [
  { id: 'operation', label: t('admin.copy.febb1b95') },
  { id: 'presentation', label: t('admin.copy.5f829ca8') },
  { id: 'domains', label: t('admin.copy.802d25a8') },
] as const);
const periodTabs = computed(() => [
  { id: 'schedules', label: t('admin.copy.18d26d2c') },
  { id: 'draws', label: t('admin.copy.26bac4a5') },
  { id: 'settlement', label: t('admin.copy.a215c7cc') },
  { id: 'correction', label: t('admin.copy.fc54e9ea') },
  { id: 'cancellation', label: t('admin.copy.7ed0de73') },
] as const);
const financeTabs = computed(() => [
  { id: 'wallet', label: t('admin.console.wallet') },
  { id: 'recharges', label: t('admin.console.recharges') },
  { id: 'withdrawals', label: t('admin.withdrawal.title') },
  { id: 'policies', label: t('admin.console.financialPolicies') },
  { id: 'repair', label: t('admin.console.balanceRepair') },
] as const);
const brandsLoading = ref(false);
const mobileMore = ref(false);
const mobileMenu = ref<HTMLElement | null>(null);
watch(mobileMore, async open => {
  if (open) { await nextTick(); mobileMenu.value?.focus(); }
});
const search = ref("");
const notice = ref<ReturnType<typeof message> | string>("");
const api = createAdminApi();
const account = ref<AdminAccount | null>(null);
provideAssociationContext(associationContextKey, computed(() => account.value));
const accountProfileOpen = ref(false);
const changePasswordOpen = ref(false);
const currentPassword = ref("");
const newPassword = ref("");
const confirmPassword = ref("");
const passwordBusy = ref(false);
const passwordError = ref<string | ReturnType<typeof message>>("");
const authLoading = ref(true);
const authError = ref<string | ReturnType<typeof message>>("");
const loginIdentifier = ref("");
const loginPassword = ref("");
const loginBusy = ref(false);
const logoutBusy = ref(false);
const loginIdempotencyKey = ref(createIdempotencyKey());
watch([loginIdentifier, loginPassword], () => {
  // Editing credentials is a new login intent, not a replay of the old body.
  if (!loginBusy.value) loginIdempotencyKey.value = createIdempotencyKey();
});
const adminBrands = ref<AdminBrand[]>([]);
const selectedBrandId = ref("");
const financeMemberId = ref("");
let adminBrandLoadGeneration = 0;
const selectedOperationPeriod = ref<{brandId:string;periodId:string;nonce:number}|null>(null);
const brand = computed(
  () =>
    adminBrands.value.find((item) => item.id === selectedBrandId.value)?.name ??
    t('admin.copy.0cf7b8b5'),
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
function skinTheme(): BrandTheme | null {
  const presentation = skin.value;
  return presentation ? { ...defaultBrand, name: presentation.display_name, logoText: presentation.logo_text,
    logoUrl: presentation.logo_url, faviconUrl: presentation.favicon_url } : null;
}
const skinStyle=computed(()=>{const theme=skinTheme();return theme?{...buildBrandCssTokens(theme),fontFamily:"var(--font-family)",fontSize:"calc(14px * var(--brand-font-scale-factor, 1))"}:{}});
const skinLogo=computed(()=>safeBrandAssetUrl(skin.value?.logo_url));
const failedSkinLogo=ref<string|null>(null);
watch(skinLogo,()=>{failedSkinLogo.value=null});
function skinLogoError(event:Event){if(event.target instanceof HTMLImageElement&&event.target.getAttribute('src')===skinLogo.value)failedSkinLogo.value=skinLogo.value}
function acceptPresentation(value:{accountId:string;record:BrandPresentationRecord}){if(account.value?.id!==value.accountId||selectedBrandId.value!==value.record.brand_id)return;if(presentationSkin.value?.accountId===value.accountId&&presentationSkin.value.record.brand_id===value.record.brand_id&&presentationSkin.value.record.version>value.record.version)return;presentationSkin.value=value;const theme=skinTheme();if(theme)applyBrandPresentation({brand:theme,paused:value.record.status==='paused',availableLanguages:value.record.effective.available_locales},document.querySelector<HTMLElement>('.app-shell')??document.documentElement)}
watch(()=>[account.value?.id,selectedBrandId.value],()=>{const generation=++presentationReadGeneration;presentationSkin.value=null;document.querySelector('link[rel="icon"][data-brand-favicon]')?.remove();const current=account.value,id=selectedBrandId.value;if(!current||!id||!brandPresentationPermissions(current,id).view)return;void presentationApi.get(id).then(record=>{if(generation===presentationReadGeneration&&account.value?.id===current.id&&selectedBrandId.value===id)acceptPresentation({accountId:current.id,record})}).catch(cause=>{if(generation===presentationReadGeneration&&account.value?.id===current.id&&selectedBrandId.value===id&&cause instanceof AdminApiError&&cause.status===401)clearAdminData()})});
const members = ref<Member[]>([]);
const membersTotal = ref('0');
const memberOnlineFilter = ref<'' | 'true' | 'false'>('');
const memberQuery = ref(''), memberOnlineQuery = ref<'' | 'true' | 'false'>('');
const memberHasNext = computed(() => BigInt(memberOffset.value) + BigInt(members.value.length) < BigInt(membersTotal.value));
const memberActionError = ref<string | ReturnType<typeof message>>('');
const membersLoading = ref(false);
const membersError = ref<string | ReturnType<typeof message>>("");
const memberOffset = ref(0);
let memberReadGeneration = 0;
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
watchEffect(()=>{if(!skin.value){const brand=contextBrand.value;if(brand)applyBrowserBranding(brand.name,brand.name.slice(0,1),null,defaultBrand.primary);else document.title=t('admin.ui.administration')}});

const contextState = ref<"loading" | "connected" | "unavailable">("loading");
const loadBrandContext = async () => {
  contextBrand.value = null;
  contextState.value = "loading";
  try {
    const response = await fetch("/api/v1/context", {
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
    const body = await readAdminEnvelope<{brand: ContextBrand}>(response);
    if (!response.ok || body.success !== true || !body.data?.brand ||
      typeof body.data.brand.id !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(body.data.brand.id))
      throw new Error("Brand context is unavailable");
    contextBrand.value = body.data.brand;
    contextState.value = "connected";
  } catch {
    contextBrand.value = null;
    contextState.value = "unavailable";
  }
};
const brandScopeLoading = computed(() => authLoading.value || brandsLoading.value || contextState.value === 'loading');
watch([account, contextBrand, adminBrands], () => {
  const current = account.value;
  const context = contextBrand.value;
  const authorized = current && !current.super_admin && context &&
    current.brand_ids.includes(context.id) && adminBrands.value.some(item => item.id === context.id);
  const id = authorized ? context.id : '';
  if (selectedBrandId.value === id) return;
  selectedBrandId.value = id;
  memberReadGeneration++;
  financeMemberId.value = '';
  search.value = ''; memberQuery.value = ''; memberOnlineFilter.value = ''; memberOnlineQuery.value = ''; membersTotal.value = '0';
  memberCreateOpen.value = false;
  memberOffset.value = 0;
  members.value = [];
  if (id && current && brandPermissionSet(current, id).has('user.view.brand')) void loadMembers();
});
async function retryBrandScope() {
  await loadBrandContext();
  if (account.value) await loadBrands();
}
onMounted(() => {
  void loadBrandContext();
  void restoreAdminSession();
});
const groups = computed(() => [...new Set(visibleNav.value.map((item) => item.group).filter(group => group !== null))]);
const canViewJoinCodes = computed(() => {
  if (!account.value || !selectedBrandId.value) return false;
  return brandPermissionSet(account.value, selectedBrandId.value).has("join_code.view.brand");
});
const visibleNav = computed(() => nav);
const memberDetail = ref<Member | null>(null);
watch(() => [account.value?.id, selectedBrandId.value], () => { memberDetail.value = null; editTarget.value = null; kickTarget.value = null; resetTarget.value = null; memberActionError.value = ''; });
const go = (target: Page) => {
  page.value = target;
  if (target === "资金与账本") financeView.value = "wallet";
  memberCreateOpen.value = false;
  if (target !== "资金与账本") financeMemberId.value = "";
  notice.value = "";
  mobileMore.value = false;
};
const canViewMemberWallet = computed(() => Boolean(account.value && selectedBrandId.value && canViewWallet(account.value, selectedBrandId.value)));
function openMemberWallet(member: Member) {
  if (!canViewMemberWallet.value) return;
  financeMemberId.value = member.id;
  go("资金与账本");
}
const toast = (text: string | ReturnType<typeof message>) => {
  const next = text;
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
const statusLabel = {
  normal: "admin.member.status.normal",
  frozen: "admin.member.status.frozen",
  disabled: "admin.member.status.disabled",
  expired: "admin.member.status.expired",
  cancelled: "admin.member.status.cancelled",
} satisfies Record<MemberStatus, import("./i18n").AdminTranslationKey>;
const statusClass = (status: MemberStatus) =>
  status === "normal"
    ? "badge-success"
    : status === "frozen"
      ? "badge-danger"
      : "badge-neutral";
const apiErrorText = (error: unknown) =>
  error instanceof Error ? publicErrorMessage(error) : display(message('admin.copy.5dabb92e'));
const clearAdminData = () => {
  page.value = "首页";
  memberReadGeneration++;
  brandsLoading.value = false;
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
  selectedOperationPeriod.value = null;
  search.value = "";
  financeMemberId.value = "";
  account.value = null;
  adminBrands.value = [];
  selectedBrandId.value = "";
  members.value = [];
};
const sessionReplaced = useSessionReplacement(() => !!account.value, () => api.me(), clearAdminData);
const loadBrands = async () => {
  const requestedAccountId = account.value?.id;
  const generation = ++adminBrandLoadGeneration;
  if (!requestedAccountId) return;
  authError.value = "";
  brandsLoading.value = true;
  try {
    const result = await api.brands();
    if (generation !== adminBrandLoadGeneration || account.value?.id !== requestedAccountId) return;
    adminBrands.value = result.items;
  } catch (error) {
    if (generation !== adminBrandLoadGeneration || account.value?.id !== requestedAccountId) return;
    if (error instanceof AdminApiError && error.status === 401)
      clearAdminData();
    authError.value = apiErrorText(error);
  } finally {
    if (generation === adminBrandLoadGeneration) brandsLoading.value = false;
  }
};
const restoreAdminSession = async () => {
  authLoading.value = true;
  try {
    const result = await api.me();
    if (result.account.super_admin) {
      clearAdminData();
      authError.value = message('admin.copy.51a5468e');
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
    page.value = "首页";
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
  sessionReplaced.value = false;
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
      authError.value = message('admin.copy.51a5468e');
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
    page.value = "首页";
    account.value = result.account;
    selectedBrandId.value = "";
    loginIdempotencyKey.value = createIdempotencyKey();
    await loadBrands();
    toast("已登录管理员账号");
  } catch (error) {
    clearAdminData();
    authError.value = error instanceof AdminApiError && error.code === "ADMIN_ENTRY_MISMATCH"
      ? message('admin.copy.51a5468e')
      : apiErrorText(error);
  } finally {
    loginBusy.value = false;
  }
};
const onBrandOperationChanged = (change: { accountId: string }) => {
  if (account.value?.id === change.accountId) void loadBrands();
};
const logout = async () => {
  if (logoutBusy.value) return;
  logoutBusy.value = true;
  const key = createIdempotencyKey();
  // Invalidate mounted views and their pending downloads immediately. A slow
  // server revocation must not keep an old authenticated view alive locally.
  clearAdminData();
  try {
    await api.logout(key);
  } catch (error) {
    authError.value = apiErrorText(error);
  } finally { logoutBusy.value = false; }
  toast("已退出管理员账号");
};
const submitPasswordChange = async () => {
  passwordError.value = "";
  const bytes = new TextEncoder().encode(newPassword.value).length;
  if (!currentPassword.value || bytes < 6 || bytes > 30 || newPassword.value !== confirmPassword.value) {
    passwordError.value = message('admin.console.invalidPassword');
    return;
  }
  passwordBusy.value = true;
  try {
    await api.changePassword(currentPassword.value, newPassword.value);
    currentPassword.value = ""; newPassword.value = ""; confirmPassword.value = "";
    changePasswordOpen.value = false;
    clearAdminData();
  } catch (error) {
    passwordError.value = apiErrorText(error);
  } finally { passwordBusy.value = false; }
};
const loadMembers = async () => {
  const current = account.value, brandId = selectedBrandId.value;
  if (!current || !brandId) return;
  const generation = ++memberReadGeneration;
  const isCurrent = () => generation === memberReadGeneration && account.value?.id === current.id && selectedBrandId.value === brandId;
  membersLoading.value = true;
  membersError.value = "";
  try {
    const result = await api.users(brandId, 100, memberOffset.value, "", { query: memberQuery.value, online: memberOnlineQuery.value });
    if (isCurrent()) { members.value = result.items; membersTotal.value = result.total_count; }
  } catch (error) {
    if (!isCurrent()) return;
    membersError.value = apiErrorText(error);
    if (error instanceof AdminApiError && error.status === 401) clearAdminData();
  } finally {
    if (isCurrent()) membersLoading.value = false;
  }
};
const searchMembers = async () => { memberOffset.value = 0; memberQuery.value = search.value.trim(); memberOnlineQuery.value = memberOnlineFilter.value; await loadMembers(); };
const clearMemberSearch = async () => { search.value = ''; memberOnlineFilter.value = ''; await searchMembers(); };
const openEdit = (member: Member) => {
  memberActionError.value = ""; editTarget.value = member;
  editStatus.value = member.status;
  editNotes.value = member.notes;
  editReason.value = "";
  editIdempotencyKey.value = createIdempotencyKey();
};
const saveMember = async () => {
  if (
    !editTarget.value ||
    !selectedBrandId.value  ||
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
    toast(message('admin.members.saved'));
  } catch (error) {
    memberActionError.value = apiErrorText(error);
  } finally {
    editBusy.value = false;
  }
};
const openKick = (member: Member) => {
  if (!member.online || !canKickUsers.value) return;
  memberActionError.value = ""; kickTarget.value = member;
  kickReason.value = "";
  kickIdempotencyKey.value = createIdempotencyKey();
};
const kickMember = async () => {
  if (
    !kickTarget.value ||
    !selectedBrandId.value  ||
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
    toast(message('admin.members.kicked'));
  } catch (error) {
    memberActionError.value = apiErrorText(error);
  } finally {
    kickBusy.value = false;
  }
};
const openReset = (member: Member) => {
  memberActionError.value = ""; resetTarget.value = member;
  resetPasswordValue.value = "";
  resetReason.value = "";
  resetImpactConfirmed.value = false;
  resetIdempotencyKey.value = createIdempotencyKey();
};
const resetMemberPassword = async () => {
  if (
    !resetTarget.value ||
    !selectedBrandId.value ||
    !resetPasswordValue.value  ||
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
    toast(message('admin.members.passwordReset'));
  } catch (error) {
    memberActionError.value = apiErrorText(error);
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
  <GlobalLoadingOverlay :active="authLoading" :label="t('admin.copy.3f7a199d')" />
  <LoginEntry v-if="authLoading || !account" :ready="!authLoading" language-test-id="admin-language" :brand-name="contextBrand?.name ?? t('admin.ui.administration')" :logo-text="contextBrand?.name.slice(0,1)" :subtitle="t('admin.ui.administration')" :title="t('admin.copy.5b5aac39')" title-id="admin-login-title" :locale="locale" :language-label="t('admin.copy.65749d82')" :languages="availableLocales.map(code => ({code,label:nativeLocaleName(code)}))" @update:locale="setLocale">
        <form v-write-guard="loginBusy" novalidate v-inline-validation class="login-entry__form" @submit.prevent="withGlobalLoading(login)">
          <LoginCredentials v-model:identifier="loginIdentifier" v-model:password="loginPassword" :identifier-label="t('admin.copy.851c6121')" :password-label="t('admin.copy.66467891')" />
          <p v-if="sessionReplaced" class="form-error" role="alert">{{ t('auth.sessionReplaced') }}</p>
          <p v-else-if="authError" class="form-error" role="alert">{{ display(authError) }}</p>
          <button class="button button-primary" :disabled="loginBusy">
            {{ loginBusy ? t('admin.copy.488b07c6') : t('admin.copy.a182c1e1') }}
          </button>
        </form>
  </LoginEntry>
  <ManagementFrame v-else :style="skinStyle" @keydown.esc="mobileMore = false">
    <template #sidebar>
      <BrandLockup href="#" :name="skin?.display_name??contextBrand?.name??t('admin.ui.administration')" :logo-text="skin?.logo_text??contextBrand?.name.slice(0,1)" :logo-url="skinLogo&&failedSkinLogo!==skinLogo?skinLogo:null" @logo-error="skinLogoError" @click.prevent="go('首页')"/>
      <nav class="side-nav" :aria-label="t('admin.copy.f4627e31')">
        <button v-for="item in visibleNav.filter(entry => entry.group === null)" :key="item.name" class="nav-item" :aria-label="navLabel(item.name)" :title="navLabel(item.name)" :class="{ active: page === item.name }" @click="go(item.name)">
          <span class="nav-icon"><SiteIcon :name="item.icon" /></span><span>{{ navLabel(item.name) }}</span>
        </button>
        <details v-for="group in groups" :key="group" class="console-nav-group" open>
          <summary>{{ navLabel(group) }}<SiteIcon name="down" /></summary>
          <button
            v-for="item in visibleNav.filter((entry) => entry.group === group)"
            :key="item.name"
            class="nav-item"
            :aria-label="navLabel(item.name)"
            :title="navLabel(item.name)"
            :class="{ active: page === item.name }"
            @click="go(item.name)"
          >
            <span class="nav-icon"><SiteIcon :name="item.icon" /></span
            ><span>{{ navLabel(item.name) }}</span
            >
          </button></details>
      </nav>
    </template>

    <template #main>
      <header class="topbar">
        <div class="breadcrumbs">
          <span>{{ contextBrand?.name ?? t('admin.copy.6bf26efc') }}</span><span class="crumb-slash">/</span
          ><b>{{ navLabel(page) }}</b
          >
        </div>
        <div class="top-actions">
          <LanguageSwitcher data-testid="admin-language" :model-value="locale" :label="t('admin.copy.65749d82')"
            :options="availableLocales.map(code => ({ code, label: nativeLocaleName(code) }))"
            @update:model-value="setLocale" />
          <button
            class="icon-button"
            :aria-label="t('admin.copy.e118a5cc')"
            @click="go('通知投递')"
          >
            <SiteIcon name="bell" /></button
          ><span class="top-divider"></span
          ><AccountMenu :username="account.username || t('association.unnamed')"><button type="button" role="menuitem" @click="accountProfileOpen = true">{{ t('admin.copy.55de09a4') }}</button><button type="button" role="menuitem" @click="changePasswordOpen = true">{{ t('admin.copy.14ce8ba8') }}</button><button v-write-guard="logoutBusy" type="button" role="menuitem" class="popover-logout" @click="withGlobalLoading(logout)">{{ t('admin.copy.6824d914') }}</button></AccountMenu>
        </div>
      </header>
      <section v-if="!selectedBrandId" v-global-loading="brandScopeLoading" class="page-content console-brand-scope">
        <article v-if="!brandScopeLoading" class="panel directory-state" role="alert">
          <h1>{{ t('admin.console.brandUnavailable') }}</h1>
          <p>{{ authError ? display(authError) : contextState === 'unavailable' ? t('admin.console.brandContextFailed') : t('admin.console.brandUnauthorized') }}</p>
          <button type="button" v-refresh-button="t('admin.copy.855b9442')" class="button button-secondary" @click="withGlobalLoading(retryBrandScope)">{{ t('admin.copy.855b9442') }}</button>
        </article>
      </section>
      <div v-if="notice" class="toast" role="status"><SiteIcon name="check" /> {{ display(notice) }}</div>

      <template v-if="selectedBrandId">
      <section v-if="page === '首页'" class="page-content">
        <OverviewPanel v-if="account" :key="account.id + ':' + selectedBrandId" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
      </section>
      <section v-else-if="page === '品牌设置'" class="page-content">
        <div class="page-heading"><h1>{{ t('admin.settings.title') }}</h1></div>
        <nav class="console-tabs" :aria-label="t('admin.settings.title')">
          <button v-for="tab in brandSettingsTabs" :key="tab.id" type="button" :class="{ selected: brandSettingsView === tab.id }" :aria-current="brandSettingsView === tab.id ? 'page' : undefined" @click="brandSettingsView = tab.id">{{ tab.label }}</button>
        </nav>
          <BrandOperation
            v-if="selectedBrandId && brandSettingsView === 'operation'"
            :key="`${account.id}:${selectedBrandId}`"
            :account="account"
            :brand-id="selectedBrandId"
            :brand-status="selectedBrand?.status"
            @session-invalid="clearAdminData"
            @changed="onBrandOperationChanged"
          />
          <article v-if="!selectedBrandId" class="panel brand-operation-empty">
            <h2>{{ t('admin.copy.93b5af0f') }}</h2>
            <p>{{ t('admin.copy.8f6f2f3a') }}</p>
          </article>
          <BrandPresentation v-if="selectedBrandId && brandSettingsView === 'presentation'" :key="`${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" @loaded="acceptPresentation"/>
          <BrandDomains v-if="selectedBrandId && brandSettingsView === 'domains'" :key="`domains:${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData"/>

      </section>

      <section
        v-else-if="page === '用户和成员'"
        class="page-content directory-page"
      >
        <div class="page-heading">
          <div>
            <h1>{{ t('admin.nav.users') }}</h1>
          </div>
          <div class="console-page-actions">

          <button v-if="hasPermission('user.create.brand')" type="button" class="button button-primary" :aria-expanded="memberCreateOpen" aria-controls="member-create-form" @click="memberCreateOpen = !memberCreateOpen"><SiteIcon :name="memberCreateOpen ? 'close' : 'plus'" />{{ t('admin.copy.40acb767') }}</button>
          </div>
        </div>
          <div v-if="authError" class="directory-error" role="alert">
            {{ display(authError) }}
          </div>
          <template v-if="selectedBrandId">
            <DetailModal v-if="selectedBrandId" id="member-create-form" :open="memberCreateOpen" :title="t('admin.copy.40acb767')" :close-label="t('association.close')" @close="memberCreateOpen = false">
              <MemberProvision :key="selectedBrandId" :account="account" :brand-id="selectedBrandId" @created="withGlobalLoading(loadMembers)" @session-invalid="clearAdminData" />
            </DetailModal>
            <div v-if="membersError" class="directory-error" role="alert">
              {{ display(membersError) }}
            </div>
            <article class="panel console-panel console-stack">
              <DirectorySearch v-model="search" :label="t('admin.members.search')" :query-label="t('admin.members.query')" :clear-label="t('admin.copy.b089734d')" :busy="membersLoading" :active="Boolean(search||memberOnlineFilter)" :search="searchMembers" :clear="clearMemberSearch"><select v-model="memberOnlineFilter" :aria-label="t('admin.members.onlineStatus')"><option value="">{{t('admin.members.allOnline')}}</option><option value="true">{{t('admin.members.online')}}</option><option value="false">{{t('admin.members.offline')}}</option></select></DirectorySearch>
              <p v-if="!members.length" class="directory-state empty-state">{{ t('admin.copy.ef8be9d6') }}</p>
              <ManagementTable v-else table-class="member-directory-table" :entries="members" :columns="[{key:'account',label:t('admin.copy.851c6121')},{key:'online',label:t('admin.members.onlineStatus')},{key:'points',label:t('admin.members.points'),cellClass:'member-points'},{key:'phone',label:t('admin.copy.d794da3d')},{key:'status',label:t('admin.copy.cf6d9c6d')},{key:'joined',label:t('admin.copy.ac08e5ed')},{key:'notes',label:t('admin.copy.fd2d2c8b'),cellClass:'member-notes'},{key:'tags',label:t('admin.copy.b96f43c6')},{key:'actions',label:t('admin.copy.d21fcbef')}]" :empty="false" :empty-label="t('admin.copy.ef8be9d6')">
                <template #account="{entry:m}"><button type="button" class="text-button member-account" @click="memberDetail=m">{{m.username || m.phone || t('association.unnamed')}}</button></template>
                <template #online="{entry:m}"><span class="member-presence" :class="{'is-online':m.online}"><i aria-hidden="true" />{{t(m.online?'admin.members.online':'admin.members.offline')}}</span></template>
                <template #points="{entry:m}">{{m.display_points===null?t('admin.members.pointsRestricted'):m.display_points}}</template>
                <template #phone="{entry:m}">{{m.phone || '—'}}</template>
                <template #status="{entry:m}"><span class="badge" :class="statusClass(m.status)">{{t(adminKey(statusLabel[m.status]))}}</span></template>
                <template #joined="{entry:m}">{{formatDisplayDateTime(m.joined_at,displayLocale)}}</template>
                <template #notes="{entry:m}">{{m.notes || '—'}}</template>
                <template #tags="{entry:m}"><span v-for="tag in m.tags" :key="tag" class="member-tag">{{tag}}</span><span v-if="!m.tags.length">—</span></template>
                <template #actions="{entry:m}"><MemberActionsMenu :id="`member-actions-${m.id}`" :label="t('admin.members.actions',{account:m.username || m.phone})"><button type="button" role="menuitem" :disabled="!canEditUsers && !canResetPasswords" @click="openEdit(m)">{{t('admin.copy.24dfbf6c')}}</button><button v-if="m.online && canKickUsers" type="button" role="menuitem" @click="openKick(m)">{{t('admin.copy.d192029b')}}</button></MemberActionsMenu></template>
              </ManagementTable>
              <PaginationControls :summary="t('admin.members.page',{page:String(Math.floor(memberOffset/100)+1),count:String(members.length),total:membersTotal})" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.ee97c955')" :busy="membersLoading" :has-previous="memberOffset>0" :has-next="memberHasNext" :previous="()=>changeMemberPage(-1)" :next="()=>changeMemberPage(1)" />
            </article>
          </template>
      </section>

      <section v-else-if="page === '代理列表' || page === '代理设置'" class="page-content" :class="{ 'agent-settings-page': page === '代理设置' }">
        <template v-if="account && selectedBrandId">
          <AgentManagement :key="account.id + ':' + selectedBrandId + ':' + page + ':' + agentRulesRevision" :mode="page === '代理设置' ? 'settings' : 'directory'" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
          <CommissionPolicySettings v-if="page === '代理设置'" :key="`commission-policy:${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" @saved="agentRulesRevision++" />
        </template>
        <div v-else class="panel directory-state"><h1>{{ navLabel(page) }}</h1><p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.49ccc3b3') }}</p></div>
      </section>

      <section v-else-if="page === '规则配置'" class="page-content">
        <RulesPage v-if="account && selectedBrandId" :key="`${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">
          <h2>{{ t('admin.copy.bc4357ba') }}</h2>
          <p>{{ t('admin.copy.6b0d5c12') }}</p>
        </div>
      </section>

      <section v-else-if="page === '期次和开奖'" class="page-content">
        <nav class="console-tabs" :aria-label="t('admin.copy.051cdfc6')">
          <button v-for="tab in periodTabs" :key="tab.id" type="button" :class="{ selected: periodView === tab.id }" :aria-current="periodView === tab.id ? 'page' : undefined" @click="periodView = tab.id">{{ tab.label }}</button>
        </nav>
        <CorrectionManagement
          v-if="account && selectedBrandId && periodView === 'correction'"
          :key="`correction-${account.id}`"
          :account="account"
          :brand-id="selectedBrandId"
          :initial-period-id="selectedOperationPeriod?.brandId === selectedBrandId ? selectedOperationPeriod.periodId : undefined"
          @session-invalid="clearAdminData"
          @open-settlement="periodId => { periodView = 'settlement'; selectedOperationPeriod = { brandId:selectedBrandId,periodId,nonce:(selectedOperationPeriod?.nonce ?? 0)+1 }; }"
        />
        <SettlementManagement
          v-if="account && selectedBrandId && periodView === 'settlement'"
          :key="`settlement-${account.id}-${selectedOperationPeriod?.nonce ?? 0}`"
          :account="account"
          :brand-id="selectedBrandId"
          :initial-period-id="selectedOperationPeriod?.brandId === selectedBrandId ? selectedOperationPeriod.periodId : undefined"
          @session-invalid="clearAdminData"
        />
        <PeriodCancellation
          v-if="account && selectedBrandId && periodView === 'cancellation'"
          :key="`period-cancel-${account.id}`"
          :account="account"
          :brand-id="selectedBrandId"
          :initial-period-id="selectedOperationPeriod?.brandId === selectedBrandId ? selectedOperationPeriod.periodId : undefined"
          @session-invalid="clearAdminData"
        />
        <PeriodSchedules
          v-show="periodView === 'schedules'"
          v-if="account && selectedBrandId"
          :key="`periods-${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else class="panel directory-state">
          <h1>{{ t('admin.copy.6318b767') }}</h1>
          <p>{{ t('admin.copy.fb856775') }}</p>
          <button
            v-if="!account"
            class="button button-primary"
            @click="go('用户和成员')"
          > {{ t('admin.copy.5fc0fd8e') }} </button>
        </div>
        <DrawManagement
          v-show="periodView === 'draws'"
          v-if="account && selectedBrandId"
          :key="`draws-${selectedBrandId}`"
          :active="periodView === 'draws'"
          :account="account"
          :brand-id="selectedBrandId"
          @select-period="periodId => { if (periodId !== selectedOperationPeriod?.periodId || selectedOperationPeriod?.brandId !== selectedBrandId) selectedOperationPeriod = periodId ? { brandId:selectedBrandId,periodId,nonce:(selectedOperationPeriod?.nonce ?? 0)+1 } : null; }"
          @session-invalid="clearAdminData"
        />
      </section>

      <section v-else-if="page === '注单和异常'" class="page-content">
        <BetOrderManagement
          v-if="account && selectedBrandId"
          :key="`bet-orders-${account.id}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else-if="account" class="panel directory-state"> {{ t('admin.copy.5f930822') }} </div>
        <div v-else class="panel directory-state"> {{ t('admin.copy.93fc2a9d') }} <button class="button button-secondary" @click="go('用户和成员')"> {{ t('admin.copy.9cb30737') }} </button>
        </div>
        <details class="console-disclosure"><summary>{{ t('admin.console.betPolicies') }}<SiteIcon name="down" /></summary>
        <BetPolicySettings
          v-if="account && selectedBrandId"
          :key="`bet-policy-${account.id}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        </details>
      </section>

      <section v-else-if="page === '批量对账'" class="page-content">
        <ReconciliationManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">{{ t('admin.copy.e415d2fb') }}</div>
      </section>

      <section v-else-if="page === '日月归档'" class="page-content">
        <ReportArchivesManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">{{ t('admin.copy.44fb2a96') }}</div>
      </section>

      <section v-else-if="page === '自动归档任务'" class="page-content">
        <ReportArchiveTasksManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">{{ t('admin.copy.44fb2a96') }}</div>
      </section>

      <section v-else-if="page === '资金与账本'" class="page-content">
        <nav class="console-tabs" :aria-label="t('admin.console.financeNavigation')">
          <button v-for="tab in financeTabs" :key="tab.id" type="button" :class="{ selected: financeView === tab.id }" :aria-current="financeView === tab.id ? 'page' : undefined" @click="financeView = tab.id">{{ tab.label }}</button>
        </nav>
        <WithdrawalManagement
          v-show="financeView === 'withdrawals'"
          :active="financeView === 'withdrawals'"
          v-if="account && selectedBrandId"
          :key="`withdrawals:${account.id}:${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <FinanceManagement
          v-show="['wallet', 'recharges', 'policies'].includes(financeView)"
          :view="financeView === 'recharges' ? 'recharges' : financeView === 'policies' ? 'policy' : 'wallet'"
          v-if="account && selectedBrandId"
          :key="selectedBrandId"
          :account="account"
          :brand-id="selectedBrandId"
          :initial-member-id="financeMemberId"
          :initial-member-username="members.find(member => member.id === financeMemberId)?.username"
          @session-invalid="clearAdminData"
        />
        <BalanceRepair
          v-show="financeView === 'repair'"
          v-if="account && selectedBrandId"
          :key="`repair-${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <WithdrawalPolicySettings
          v-show="financeView === 'policies'"
          v-if="account && selectedBrandId"
          :key="`withdrawal-policy-${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else class="panel directory-state">
          <h1>{{ t('admin.copy.783ae8f1') }}</h1>
          <p>
            {{
              account
                ? t('admin.copy.8d09da6f')
                : t('admin.copy.cc1c32a0')
            }}
          </p>
          <button
            v-if="!account"
            class="button button-primary"
            @click="go('用户和成员')"
          > {{ t('admin.copy.5fc0fd8e') }} </button>
        </div>
      </section>

      <section v-else-if="page === '佣金和奖励'" class="page-content">
        <CommissionCyclesManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">
          <h1>{{ t('admin.copy.5c8e261e') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.ea2574e4') }}</p>
        </div>
      </section>

      <section v-else-if="page === '佣金派发'" class="page-content">
        <CommissionPaymentsManagement v-if="account && selectedBrandId" :key="`commission-payments:${account.id}:${selectedBrandId}`"
          :account="account" :brand-id="selectedBrandId" :brand-status="selectedBrand?.status" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ t('admin.copy.61a05916') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.a015944d') }}</p>
        </div>
      </section>

      <section v-else-if="page === '佣金更正'" class="page-content">
        <CommissionCorrectionsManagement v-if="account && selectedBrandId" :key="`commission-corrections:${account.id}:${selectedBrandId}`"
          :account="account" :brand-id="selectedBrandId" :brand-status="selectedBrand?.status" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ t('admin.copy.b095cecc') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.9e480e11') }}</p>
        </div>
      </section>

      <section v-else-if="page === '人工奖励'" class="page-content">
        <RewardsManagement v-if="account && selectedBrandId" :key="`rewards:${account.id}:${selectedBrandId}`"
          :account="account" :brand-id="selectedBrandId" :brand-status="selectedBrand?.status" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ t('admin.copy.3fecab01') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.197fd776') }}</p>
        </div>
      </section>

      <section v-else-if="page === '通知模板'" class="page-content">
        <NotificationTemplates v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ t('admin.copy.e4f1dc7c') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.4436f1d3') }}</p>
          <button v-if="!account" class="button button-primary" @click="go('用户和成员')">{{ t('admin.copy.5fc0fd8e') }}</button>
        </div>
      </section>

      <section v-else-if="page === '报表和对账'" class="page-content">
        <ReportsManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ t('admin.copy.931e1e2b') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.d9310eab') }}</p>
          <button v-if="!account" class="button button-primary" @click="go('用户和成员')">{{ t('admin.copy.5fc0fd8e') }}</button>
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
          <h1>{{ t('admin.copy.9c6ee39c') }}</h1>
          <p>
            {{
              account
                ? t('admin.copy.8d09da6f')
                : t('admin.copy.d9c0555a')
            }}
          </p>
          <button
            v-if="!account"
            class="button button-primary"
            @click="go('用户和成员')"
          > {{ t('admin.copy.5fc0fd8e') }} </button>
        </div>
      </section>

      <section v-else-if="page === '通知投递'" class="page-content">
        <NotificationDeliveries v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ t('admin.copy.63871b72') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.9007bb20') }}</p>
          <button v-if="!account" class="button button-primary" @click="go('用户和成员')">{{ t('admin.copy.5fc0fd8e') }}</button>
        </div>
      </section>

      <section v-else class="page-content">
        <AuditManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else-if="account" class="panel directory-state"><h1>{{ t('admin.copy.315a9366') }}</h1><p>{{ t('admin.copy.f82425eb') }}</p></div>
      </section>

      </template>
      <DetailModal v-if="memberDetail && account && selectedBrandId" :title="t('admin.members.details')" :close-label="t('association.close')" @close="memberDetail = null">
        <dl class="confirm-list member-details">
          <div><dt>{{ t('admin.copy.851c6121') }}</dt><dd>{{ memberDetail.username || memberDetail.phone || t('association.unnamed') }}</dd></div>
          <div><dt>{{ t('admin.copy.b60e278d') }}</dt><dd>{{ memberDetail.display_name || '—' }}</dd></div>
          <div><dt>{{ t('admin.members.onlineStatus') }}</dt><dd>{{ t(memberDetail.online ? 'admin.members.online' : 'admin.members.offline') }}</dd></div>
          <div><dt>{{ t('admin.members.points') }}</dt><dd>{{ memberDetail.display_points === null ? t('admin.members.pointsRestricted') : memberDetail.display_points }}</dd></div>
          <div><dt>{{ t('admin.copy.d794da3d') }}</dt><dd>{{ memberDetail.phone || '—' }}</dd></div>
          <div><dt>{{ t('admin.copy.cf6d9c6d') }}</dt><dd>{{ t(adminKey(statusLabel[memberDetail.status])) }}</dd></div>
          <div><dt>{{ t('admin.copy.ac08e5ed') }}</dt><dd>{{ formatDisplayDateTime(memberDetail.joined_at, displayLocale) }}</dd></div>
          <div><dt>{{ t('admin.copy.fd2d2c8b') }}</dt><dd>{{ memberDetail.notes || '—' }}</dd></div>
          <div><dt>{{ t('admin.copy.b96f43c6') }}</dt><dd>{{ memberDetail.tags.join(' · ') || '—' }}</dd></div>
        </dl>
        <button v-if="canViewMemberWallet" type="button" class="button button-secondary" @click="openMemberWallet(memberDetail); memberDetail = null">{{ t('admin.copy.91b1cc2c') }}</button>
        <JoinCodeManagement v-if="canViewJoinCodes" :key="account.id + ':' + selectedBrandId + ':' + memberDetail.id" :account="account" :brand-id="selectedBrandId" :member-id="memberDetail.id" kind="referral" :owner-record="memberDetail" @session-invalid="clearAdminData" />
      </DetailModal>
      <DetailModal v-if="editTarget" :open="!resetTarget" class="console-member-dialog" :title="t('admin.copy.0ebe57dc')" :close-label="t('association.close')" @close="!editBusy && (editTarget = null)">
          <p>{{ editTarget.username || editTarget.phone || t('association.unnamed') }}</p>
          <label class="modal-label"
            >{{ t('admin.copy.cf6d9c6d') }}<select v-model="editStatus" class="field" :disabled="!canEditUsers">
              <option
                v-for="(label, status) in statusLabel"
                :key="status"
                :value="status"
              >
                {{ t(adminKey(label)) }}
              </option>
            </select></label
          ><label class="modal-label"
            >{{ t('admin.copy.fd2d2c8b') }}<textarea
              v-model="editNotes" :disabled="!canEditUsers"
              rows="3"
              maxlength="2000"
            ></textarea></label
          >
          <button v-if="canResetPasswords" type="button" class="button button-secondary" :disabled="editBusy" @click="openReset(editTarget)">{{ t('admin.copy.d1e2ba79') }}</button>
          <p v-if="memberActionError" class="message error" role="alert">{{ display(memberActionError) }}</p>
          <div class="modal-actions">
            <button class="button button-secondary" :disabled="editBusy" @click="editTarget = null"> {{ t('admin.copy.e2f391e7') }}</button
            ><button
              v-write-guard="editBusy"
              class="button button-primary"
              :disabled="editBusy || !canEditUsers"
              @click="withGlobalLoading(saveMember)"
            >
              {{ editBusy ? t('admin.copy.2462134b') : t('admin.copy.7eeb35b3') }}
            </button>
          </div>
      </DetailModal>
      <DetailModal v-if="kickTarget" class="console-member-dialog" :title="t('admin.copy.b7cf2b50')" :close-label="t('association.close')" @close="!kickBusy && (kickTarget = null)">
          <p>{{ kickTarget.username || kickTarget.phone || t('association.unnamed') }}</p>
          <p class="danger-note"> {{ t('admin.copy.cf1837eb') }} </p>

          <p v-if="memberActionError" class="message error" role="alert">{{ display(memberActionError) }}</p>
          <div class="modal-actions">
            <button class="button button-secondary" :disabled="kickBusy" @click="kickTarget = null"> {{ t('admin.copy.e2f391e7') }}</button
            ><button
              v-write-guard="kickBusy"
              class="button button-danger"
              :disabled="kickBusy "
              @click="withGlobalLoading(kickMember)"
            >
              {{ kickBusy ? t('admin.copy.57f408ff') : t('admin.copy.6f261b8f') }}
            </button>
          </div>
      </DetailModal>
      <DetailModal v-if="resetTarget" class="console-member-dialog" :title="t('admin.copy.c608f380')" :close-label="t('association.close')" @close="!resetBusy && (resetTarget = null)">
          <p>{{ resetTarget.username || resetTarget.phone || t('association.unnamed') }}</p>
          <div class="danger-note"> {{ t('admin.copy.b042a9e3') }} </div>
          <label class="modal-label"
            >{{ t('admin.copy.6b88b89d') }} <span>{{ t('admin.copy.2ed0ac9e') }}</span
            ><input
              v-model="resetPasswordValue"
              class="field"
              type="password"
              autocomplete="new-password"
              required /></label
          ><label class="impact-confirm"
            ><input
              v-model="resetImpactConfirmed"
              type="checkbox"
            />{{ t('admin.copy.22cd3a6c') }}</label
          >
          <p v-if="memberActionError" class="message error" role="alert">{{ display(memberActionError) }}</p>
          <div class="modal-actions">
            <button class="button button-secondary" :disabled="resetBusy" @click="resetTarget = null"> {{ t('admin.copy.e2f391e7') }}</button
            ><button
              v-write-guard="resetBusy"
              class="button button-danger"
              :disabled="
                resetBusy ||
                !resetPasswordValue  ||
                !resetImpactConfirmed
              "
              @click="withGlobalLoading(resetMemberPassword)"
            >
              {{ resetBusy ? t('admin.copy.57f408ff') : t('admin.copy.db124c4e') }}
            </button>
          </div>
      </DetailModal>
    </template>

    <div
      v-if="mobileMore"
      class="mobile-more-backdrop"
      @click.self="mobileMore = false"
    >
      <nav ref="mobileMenu" tabindex="-1" class="mobile-more-menu" :aria-label="t('admin.copy.1106fe21')">
        <div>
          <b>{{ t('admin.copy.1106fe21') }}</b
          ><button :aria-label="t('admin.copy.c621342b')" @click="mobileMore = false"><SiteIcon name="close" /></button>
        </div>
        <section class="console-mobile-group">
          <button v-for="item in visibleNav.filter(entry => entry.group === null)" :key="item.name" type="button" :class="{ active: page === item.name }" :aria-current="page === item.name ? 'page' : undefined" @click="go(item.name)"><SiteIcon :name="item.icon" /><span>{{ navLabel(item.name) }}</span><SiteIcon name="next" /></button>
        </section>
        <details v-for="group in groups" :key="group" class="console-nav-group console-mobile-group" open>
          <summary>{{ navLabel(group) }}<SiteIcon name="down" /></summary>
          <button v-for="item in visibleNav.filter(entry => entry.group === group)" :key="item.name" type="button" :class="{ active: page === item.name }" :aria-current="page === item.name ? 'page' : undefined" @click="go(item.name)"><SiteIcon :name="item.icon" /><span>{{ navLabel(item.name) }}</span><SiteIcon name="next" /></button>
        </details>
      </nav>
    </div>
    <nav class="mobile-nav" :aria-label="t('admin.copy.12b204e9')">
      <button
        v-for="target in [
          '首页',
          '用户和成员',
          '期次和开奖',
          '资金与账本',
        ] as Page[]"
        :key="target"
        :class="{ active: page === target }"
        @click="go(target)"
      >
        <span><SiteIcon :name="nav.find((item) => item.name === target)!.icon" /></span
        ><small>{{ target === "用户和成员" ? t('admin.nav.users') : target === "期次和开奖" ? t('admin.copy.ce59921f') : target === "资金与账本" ? t('admin.copy.c9a0911c') : navLabel(target) }}</small></button
      ><button
        :class="{ active: mobileMore }"
        :aria-expanded="mobileMore"
        @click="mobileMore = !mobileMore"
      >
        <span><SiteIcon name="menu" /></span><small>{{ t('admin.copy.258d2d94') }}</small>
      </button>
    </nav>
    <AccountProfileDialog v-if="accountProfileOpen" :title="t('admin.copy.55de09a4')" :close-label="t('association.close')" :fields="[{label:t('admin.copy.851c6121'),value:account.username || t('association.unnamed')},{label:t('admin.copy.d48825ff'),value:account.super_admin ? t('admin.copy.543831fc') : t('admin.copy.d48825ff')}]" @close="accountProfileOpen=false" />
    <ChangePasswordDialog v-if="changePasswordOpen" v-model:current="currentPassword" v-model:next="newPassword" v-model:confirmation="confirmPassword" :title="t('admin.copy.14ce8ba8')" :close-label="t('association.close')" :current-label="t('admin.copy.7e7f11f6')" :new-label="t('admin.copy.683caabd')" :confirm-label="t('admin.copy.edc96e77')" :cancel-label="t('admin.copy.e2f391e7')" :submit-label="passwordBusy ? t('admin.copy.2462134b') : t('admin.copy.e93fc68f')" :busy="passwordBusy" @close="changePasswordOpen=false" :submit="submitPasswordChange"><p v-if="passwordError" class="form-error" role="alert">{{ display(passwordError) }}</p></ChangePasswordDialog>
  </ManagementFrame>
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
