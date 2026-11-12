<script setup lang="ts">
import ThemeSwitcher from "@lottery/shared/ThemeSwitcher.vue"
import CashConsole from "@lottery/shared/CashConsole.vue";
import { effectiveBrandPermissions as cashPermissions } from "./point-policy-api";
import BetDistribution from "./BetDistribution.vue";
import { formatPoints } from "@lottery/shared";
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import OperationToastHost from "./OperationToastHost.vue";
import { showOperationToast, dismissOperationToast } from "@lottery/shared";
import ManagementList from "@lottery/shared/ManagementList.vue"
import BatchDelete from "@lottery/shared/BatchDelete.vue"
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
import LoginCaptcha from '@lottery/shared/LoginCaptcha.vue'
import { vWriteGuard } from "@lottery/shared";
import { associationContextKey } from '@lottery/shared';
import { provide as provideAssociationContext } from 'vue';
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
import DetailModal from "@lottery/shared/DetailModal.vue";
import LanguageSwitcher from '@lottery/shared/LanguageSwitcher.vue';
import {
  computed,
  onMounted,
  nextTick,
  ref,
} from "vue";
import { brandPermissionSet } from "./brand-permissions";
import ManualRechargeForm from "./ManualRechargeForm.vue";
import { canWriteFinance, canViewWallet, createFinanceApi, FinanceApiError, type Wallet } from "./finance-api";
import PlatformLottery from "@lottery/shared/PlatformLottery.vue";
import AccessManagement from "./AccessManagement.vue";
import MemberActionsMenu from "@lottery/shared/MemberActionsMenu.vue";
import MemberProvision from "./MemberProvision.vue";
import BrandOperation from "./BrandOperation.vue";
import AnnouncementsManagement from "@lottery/shared/AnnouncementsManagement.vue";
import BrandContacts from "./BrandContacts.vue";
import BrandPresentation from "./BrandPresentation.vue";
import {watch,watchEffect} from "vue";
import {buildBrandCssTokens,defaultBrand,safeBrandAssetUrl,applyBrandPresentation,applyBrowserBranding,nativeLocaleName,type BrandTheme} from "@lottery/shared";
import {type BrandPresentationRecord} from "./brand-presentation-api";
import PointPolicySettings from "./PointPolicySettings.vue";
import FinanceManagement from "./FinanceManagement.vue";
import BrandDrawHistory from "./BrandDrawHistory.vue";
import WithdrawalManagement from "./WithdrawalManagement.vue";

import BetOrderManagement from "./BetOrderManagement.vue";
import BetPolicySettings from "./BetPolicySettings.vue";
import OverviewPanel from "./OverviewPanel.vue";
import ReportsManagement from "./ReportsManagement.vue";
import AuditManagement from "./AuditManagement.vue";
import AgentManagement from "./AgentManagement.vue";
import CommissionPolicySettings from "./CommissionPolicySettings.vue";
import CommissionManagement from "./CommissionManagement.vue";
import RewardsManagement from "./RewardsManagement.vue";
import { clearAllPendingCommissionCycleWrites } from "./commission-cycles-state";
import { clearAllPendingCommissionAdjustments } from "./commission-adjustments-state";
import { clearAllCommissionPaymentIntents, hideCommissionPaymentIntentsForScope } from "./commissionPayments-state";
import { clearAllRewardIntents } from "./rewards-state";
import { clearAllCommissionCorrectionIntents, hideCommissionCorrectionIntentsForScope } from "./commission-corrections-state";
import JoinCodeManagement from "./JoinCodeManagement.vue";
import { clearAllPendingAgentWrites } from "./agents-state";
import { clearAllPendingJoinCodeWrites } from "./join-codes-state";
import { clearAllPendingBrandOperationWrites } from "./brand-operation-state";
import { clearAllPendingCommissionWrites } from "./commission-policy-state";
import { adminKey, useAdminI18n } from "./i18n";
import RechargeQuotaPanel from "@lottery/shared/RechargeQuotaPanel.vue";
import WithdrawalPolicySettings from "./WithdrawalPolicySettings.vue";
import {
  AdminApiError,
  createAdminApi,
  createIdempotencyKey,
  type AdminAccount,
  type AdminBrand,
  type Member,
  type MemberStatus,
} from "./admin-api";
const memberPageSize = ref(100)


const { t, message, display, locale, availableLocales, setLocale, configure, resetBrand } = useAdminI18n();
const navLabels: Record<string, string> = {
  "开奖管理": "admin.copy.26bac4a5",
  "结算管理": "admin.nav.settlement",
  "开奖更正": "admin.nav.corrections",
  "期次取消": "admin.nav.cancellation",
  "充值记录": "admin.console.recharges",
  "额度管理": "admin.console.quota",
  "提现管理": "admin.withdrawal.title",
  "角色权限": "admin.copy.78c84d5e",
  "首页": "overview.home",
  "系统管理": "admin.console.group.7",
  "报表管理": "admin.console.group.5",
  "资金管理": "admin.console.group.4",
  "投注管理": "admin.console.group.3",
  "品牌设置": "admin.settings.title", "会员管理": "admin.nav.users",
  "代理管理": "admin.nav.agents", "代理列表": "admin.nav.agentList", "彩种管理": "admin.nav.games", "盘口赔率": "admin.nav.odds", "规则配置": "admin.copy.db7e35eb",
  "开奖记录": "admin.nav.drawHistory", "期次管理": "admin.nav.periods", "注单管理": "admin.copy.f83e2039", "注单分布": "distribution.title",
  "佣金管理": "admin.commission.ui.management",
  "人工奖励": "admin.copy.3fecab01", "业务报表": "admin.nav.reports", "管理账号": "admin.copy.f8e4afe4",
  "审计日志": "admin.copy.315a9366",
 "平台": "admin.copy.ed823559",
  "运营": "admin.copy.4b7fa234", "资金": "admin.copy.c9a0911c", "管理": "admin.copy.0f05b3f1",
};
const navLabel = (value: string) => t(adminKey(navLabels[value]!));

type Page =
  | "角色权限"
  | "提现管理"
  | "充值记录"
  | "额度管理"
  | "期次取消"
  | "开奖更正"
  | "结算管理"
  | "开奖管理"
  | "首页"
  | "品牌设置"
  | "会员管理"
  | "代理列表"
  | "开奖记录"
  | "彩种管理"
  | "盘口赔率"
  | "规则配置"
  | "期次管理"
  | "注单管理"
  | "注单分布"
  | "佣金管理"
  | "人工奖励"
  | "业务报表"
  | "管理账号"
  | "审计日志";
const nav: { name: Page; icon: SiteIconName; group: string | null }[] = [
  { name: "首页", icon: "home", group: null },
  { name: "品牌设置", icon: "brands", group: null },
  { name: "会员管理", icon: "users", group: null },
  { name: "代理列表", icon: "network", group: "代理管理" },
  { name: "彩种管理", icon: "ticket", group: "投注管理" },
  { name: "开奖记录", icon: "ticket", group: "投注管理" },
  { name: "盘口赔率", icon: "ticket", group: "投注管理" },
  { name: "注单管理", icon: "ticket", group: "投注管理" },
  { name: "注单分布", icon: "chart", group: "投注管理" },
  { name: "充值记录", icon: "wallet", group: "资金管理" },
  { name: "额度管理", icon: "wallet", group: "资金管理" },
  { name: "提现管理", icon: "wallet", group: "资金管理" },
  { name: "佣金管理", icon: "trend", group: null },
  { name: "人工奖励", icon: "gift", group: "资金管理" },
  { name: "业务报表", icon: "chart", group: "报表管理" },
  { name: "管理账号", icon: "shield", group: "系统管理" },
  { name: "角色权限", icon: "shield", group: "系统管理" },
  { name: "审计日志", icon: "audit", group: "系统管理" },
];
const page = ref<Page>("首页");
const financePages = { '充值记录': 'recharges', '提现管理': 'withdrawals' } as const;
const financeView = computed(() => financePages[page.value as keyof typeof financePages]);
const memberCreateOpen = ref(false);
const brandSettingsView = ref<'operation' | 'presentation' | 'contacts' | 'announcements' | 'funds' | 'withdrawal' | 'betting' | 'agents'>('operation');
const brandSettingsTabs = computed(() => [
  { id: 'operation', label: t('admin.settings.operation') },
  { id: 'presentation', label: t('admin.settings.presentation') },
  { id: 'funds', label: t('admin.settings.funds') },
  { id: 'withdrawal', label: t('admin.settings.withdrawal') },
  { id: 'betting', label: t('admin.console.betPolicies') },
  { id: 'agents', label: t('admin.nav.agentSettings') },
  { id: 'announcements', label: t('announcements.title') },
  { id: 'contacts', label: t('contacts.title') },
] as const);
const brandsLoading = ref(false);
const mobileMore = ref(false);
const mobileMenu = ref<HTMLElement | null>(null);
watch(mobileMore, async open => {
  if (open) { await nextTick(); mobileMenu.value?.focus(); }
});
const search = ref("");

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
const loginCaptcha = ref<InstanceType<typeof LoginCaptcha> | null>(null);
const captchaReady = ref(false);
const logoutBusy = ref(false);
const loginIdempotencyKey = ref(createIdempotencyKey());
watch([loginIdentifier, loginPassword], () => {
  // Editing credentials is a new login intent, not a replay of the old body.
  if (!loginBusy.value) loginIdempotencyKey.value = createIdempotencyKey();
});
const adminBrands = ref<AdminBrand[]>([]);
const selectedBrandId = ref("");
const rechargeMember = ref<Member | null>(null);
const rechargePending = ref(false);
const canRechargeMember = computed(() => Boolean(account.value && selectedBrandId.value && canWriteFinance(account.value, selectedBrandId.value, 'recharge.write.brand')));
watch(selectedBrandId, () => { rechargeMember.value = null; dismissOperationToast(); }, { flush: 'sync' });
const memberFinance = ref<{ member: Member; view: 'wallet' | 'ledger' } | null>(null);
async function closeMemberFinance() {
  const memberId = memberFinance.value?.member.id;
  memberFinance.value = null;
  await nextTick();
  document.querySelector<HTMLButtonElement>(`button[aria-controls="member-actions-${memberId}"]`)?.focus({ preventScroll: true });
}
let adminBrandLoadGeneration = 0;
const selectedBrand = computed(() => adminBrands.value.find((item) => item.id === selectedBrandId.value) ?? null);
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
const resetIdempotencyKey = ref("");
const resetBusy = ref(false);
const authorizedBrands = computed(() => account.value && !account.value.super_admin
  ? adminBrands.value.filter(item => account.value!.brand_ids.includes(item.id)) : []);
const contextBrand = computed(() => authorizedBrands.value.find(item => item.id === selectedBrandId.value) ?? null);
watchEffect(()=>{if(!skin.value){const brand=contextBrand.value;if(brand)applyBrowserBranding(brand.name,brand.name.slice(0,1),null,defaultBrand.primary);else document.title=t('admin.ui.administration')}});
const contextState = ref<"connected" | "unavailable">("connected");
const brandScopeLoading = computed(() => authLoading.value || brandsLoading.value);
watch([account, authorizedBrands], () => {
  const choices = authorizedBrands.value;
  if (!choices.some(item => item.id === selectedBrandId.value))
    selectedBrandId.value = choices.length === 1 ? choices[0]!.id : '';
});
watch(selectedBrandId, id => {
  memberReadGeneration++;
  memberFinance.value = null;
  search.value = ''; memberQuery.value = ''; memberOnlineFilter.value = ''; memberOnlineQuery.value = ''; membersTotal.value = '0';
  memberCreateOpen.value = false;
  memberOffset.value = 0;
  members.value = [];
  if (id && account.value && brandPermissionSet(account.value, id).has('user.view.brand')) void loadMembers();
});
async function retryBrandScope() {
  if (account.value) await loadBrands();
}
onMounted(() => { void restoreAdminSession(); });
const groups = computed(() => [...new Set(visibleNav.value.map((item) => item.group).filter(group => group !== null))]);
const canViewJoinCodes = computed(() => {
  if (!account.value || !selectedBrandId.value) return false;
  return brandPermissionSet(account.value, selectedBrandId.value).has("join_code.view.brand");
});
const visibleNav = computed(() => nav.filter(item => (item.name !== "开奖记录" || hasPermission("platform_game.view.brand")) && (item.name !== "额度管理" || hasPermission("recharge.view.brand")) && (item.name !== "注单分布" || hasPermission("bet.view.brand"))));
const mobileNavPages = computed<Page[]>(() => ['首页', '会员管理', '彩种管理']);
const memberDetail = ref<Member | null>(null);
watch(() => [account.value?.id, selectedBrandId.value], () => { memberFinance.value = null; memberDetail.value = null; editTarget.value = null; kickTarget.value = null; resetTarget.value = null; memberActionError.value = ''; });
const go = (target: Page) => {
  if (!nav.some(item => item.name === target)) throw new Error(`Unsupported administration page: ${target}`);
  page.value = target;
  memberCreateOpen.value = false;
  memberFinance.value = null;
  dismissOperationToast();
  mobileMore.value = false;
};
const canViewMemberWallet = computed(() => Boolean(account.value && selectedBrandId.value && brandPermissionSet(account.value, selectedBrandId.value).has('user.view.brand') && canViewWallet(account.value, selectedBrandId.value)));
const memberWallet = ref<Wallet | null>(null);
const memberWalletError = ref<ReturnType<typeof publicErrorMessage> | ''>('');
watch(() => [memberDetail.value?.id, selectedBrandId.value, account.value?.id, canViewMemberWallet.value] as const, async ([memberId, brandId, , allowed], _, onCleanup) => {
  let active = true;
  onCleanup(() => { active = false; });
  memberWallet.value = null; memberWalletError.value = '';
  if (!memberId || !brandId || !allowed) return;
  try {
    const wallet = await createFinanceApi().wallet(brandId, memberId);
    if (!active) return;
    if (wallet.brand_id !== brandId || wallet.member_id !== memberId) throw new FinanceApiError('Invalid member wallet response', 502, 'INVALID_RESPONSE');
    memberWallet.value = wallet;
  } catch (cause) {
    if (!active) return;
    memberWalletError.value = publicErrorMessage(cause, 'read');
    if ((cause instanceof FinanceApiError || cause instanceof AdminApiError) && cause.status === 401) clearAdminData();
  }
});
const memberPointsHint = computed(() => memberWallet.value && memberWallet.value.display_points !== memberWallet.value.available_points
  ? t('admin.members.pointsHint', { current: memberWallet.value.display_points, available: memberWallet.value.available_points }) : undefined);
const toast = (text: ReturnType<typeof message>) => showOperationToast(() => display(text), 'success');
function handleQuotaFailure(cause: unknown) {
  if (cause instanceof Error && 'status' in cause && cause.status === 401) clearAdminData();
}
const hasPermission = (permission: string) => {
  if (!account.value) return false;
  return brandPermissionSet(account.value, selectedBrandId.value).has(permission);
};
const canDeleteMembers = computed(() => Boolean(account.value && !account.value.super_admin && (hasPermission('user.delete.brand') || hasPermission('user.delete.platform'))));
function batchMemberDeleted(member: Member) {
  members.value = members.value.filter(item => item.id !== member.id);
  membersTotal.value = String(BigInt(membersTotal.value) - 1n);
}
async function refreshBatchMembers() {
  if (BigInt(memberOffset.value) >= BigInt(membersTotal.value)) memberOffset.value = Math.max(0, memberOffset.value - memberPageSize.value);
  await loadMembers();
}
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
  dismissOperationToast();
  page.value = "首页";
  memberReadGeneration++;
  brandsLoading.value = false;
	clearAllPendingCommissionAdjustments();
	clearAllPendingCommissionCycleWrites();
	clearAllCommissionPaymentIntents();
	clearAllRewardIntents();
	clearAllCommissionCorrectionIntents();
	clearAllPendingCommissionWrites();


	presentationReadGeneration+=1;presentationSkin.value=null;
  adminBrandLoadGeneration += 1;
  clearAllPendingAgentWrites();
  clearAllPendingJoinCodeWrites();

  clearAllPendingBrandOperationWrites();
  search.value = "";
  memberFinance.value = null;
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
    contextState.value = "connected";
  } catch (error) {
    if (generation !== adminBrandLoadGeneration || account.value?.id !== requestedAccountId) return;
    if (error instanceof AdminApiError && error.status === 401)
      clearAdminData();
    authError.value = apiErrorText(error);
    contextState.value = "unavailable";
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
      clearAllPendingCommissionCycleWrites();
      clearAllPendingCommissionAdjustments();
      clearAllCommissionPaymentIntents();
      clearAllRewardIntents();
      clearAllCommissionCorrectionIntents();
      clearAllPendingJoinCodeWrites();
      clearAllPendingBrandOperationWrites();



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
  if (loginBusy.value || !captchaReady.value) return;
  sessionReplaced.value = false;
  loginBusy.value = true;
  authError.value = "";
  try {
    await api.login(
      loginIdentifier.value.trim(),
      loginPassword.value,
      loginIdempotencyKey.value,
      loginCaptcha.value?.input(),
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
      clearAllPendingCommissionCycleWrites();
      clearAllPendingCommissionAdjustments();
      clearAllCommissionPaymentIntents();
      clearAllRewardIntents();
      clearAllCommissionCorrectionIntents();
      clearAllPendingJoinCodeWrites();
      clearAllPendingBrandOperationWrites();



    }
    page.value = "首页";
    account.value = result.account;
    selectedBrandId.value = "";
    loginIdempotencyKey.value = createIdempotencyKey();
    await loadBrands();
    toast(message('admin.feedback.signedIn'));
  } catch (error) {
    clearAdminData();
    authError.value = error instanceof AdminApiError && error.code === "ADMIN_ENTRY_MISMATCH"
      ? message('admin.copy.51a5468e')
      : apiErrorText(error);
    if (error instanceof AdminApiError && error.status >= 400 && error.status < 500) {
      loginIdempotencyKey.value = createIdempotencyKey();
      await loginCaptcha.value?.refresh();
    }
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
    toast(message('admin.feedback.signedOut'));
  } catch (error) {
    authError.value = apiErrorText(error);
  } finally { logoutBusy.value = false; }
};
const submitPasswordChange = async () => {
  passwordError.value = "";
  const characters = Array.from(newPassword.value).length;
  if (!currentPassword.value || characters < 6 || characters > 30 || newPassword.value !== confirmPassword.value) {
    passwordError.value = message('admin.console.invalidPassword');
    return;
  }
  passwordBusy.value = true;
  try {
    await api.changePassword(currentPassword.value, newPassword.value);
    currentPassword.value = ""; newPassword.value = ""; confirmPassword.value = "";
    changePasswordOpen.value = false;
    clearAdminData();
    toast(message('admin.feedback.passwordChanged'));
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
    const result = await api.users(brandId, memberPageSize.value, memberOffset.value, "", { query: memberQuery.value, online: memberOnlineQuery.value });
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
  resetIdempotencyKey.value = createIdempotencyKey();
};
const resetMemberPassword = async () => {
  if (
    !resetTarget.value ||
    !selectedBrandId.value ||
    !resetPasswordValue.value  ||
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
const changeMemberPage = async (direction: number) => {
  memberOffset.value = Math.max(0, memberOffset.value + direction * memberPageSize.value);
  await loadMembers();
};

useOperationToast(() => authError.value, value => display(value), 'error');
useOperationToast(() => membersError.value, value => display(value), 'error');
useOperationToast(() => memberWalletError.value, value => displayPublicError(value), 'error');
useOperationToast(() => memberActionError.value, value => display(value), 'error');
useOperationToast(() => passwordError.value, value => display(value), 'error');
</script>

<template>

  <GlobalLoadingOverlay :active="authLoading" :label="authLoading ? t('admin.copy.3f7a199d') : t('admin.copy.57f408ff')" />
  <LoginEntry v-if="authLoading || !account" :ready="!authLoading" language-test-id="admin-language" :brand-name="t('admin.ui.administration')" :title="t('admin.copy.5b5aac39')" title-id="admin-login-title" :locale="locale" :language-label="t('admin.copy.65749d82')" :languages="availableLocales.map(code => ({code,label:nativeLocaleName(code)}))" @update:locale="setLocale">
        <form v-write-guard="loginBusy" novalidate v-inline-validation class="login-entry__form" @submit.prevent="withGlobalLoading(login)">
          <LoginCredentials v-model:identifier="loginIdentifier" v-model:password="loginPassword" :identifier-label="t('admin.copy.851c6121')" :password-label="t('admin.copy.66467891')" />
          <p v-if="sessionReplaced" class="form-error" role="alert">{{ t('auth.sessionReplaced') }}</p>
          <LoginCaptcha ref="loginCaptcha" v-model:ready="captchaReady" :disabled="loginBusy" :locale="locale" config-url="/api/v1/admin/auth/config" challenge-url="/api/v1/admin/auth/challenge" />
          <button class="button button-primary" :disabled="loginBusy || !captchaReady">
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
          <SelectControl v-if="authorizedBrands.length > 1" v-model="selectedBrandId" class="admin-brand-picker" :label="t('admin.copy.b5720d4c')" :options="[{value:'',label:t('admin.copy.0cf7b8b5'),disabled:true},...authorizedBrands.map(item=>({value:item.id,label:item.name}))]" />
          <ThemeSwitcher :locale="locale" /><LanguageSwitcher data-testid="admin-language" :model-value="locale" :label="t('admin.copy.65749d82')"
            :options="availableLocales.map(code => ({ code, label: nativeLocaleName(code) }))"
            @update:model-value="setLocale" />
          <AccountMenu :username="account.username || t('association.unnamed')"><button type="button" role="menuitem" @click="accountProfileOpen = true">{{ t('admin.copy.55de09a4') }}</button><button type="button" role="menuitem" @click="changePasswordOpen = true">{{ t('admin.copy.14ce8ba8') }}</button><button v-write-guard="logoutBusy" type="button" role="menuitem" class="popover-logout" @click="withGlobalLoading(logout)">{{ t('admin.copy.6824d914') }}</button></AccountMenu>
        </div>
      </header>
      <section v-if="!selectedBrandId" v-global-loading="brandScopeLoading" class="page-content console-brand-scope">
        <article v-if="!brandScopeLoading && authorizedBrands.length > 1 && !authError" class="panel directory-state">
          <h1>{{ t('admin.copy.0cf7b8b5') }}</h1>
        </article>
        <article v-else-if="!brandScopeLoading" class="panel directory-state" role="alert">
          <h1>{{ t('admin.console.brandUnavailable') }}</h1>
          <p>{{ authError ? display(authError) : contextState === 'unavailable' ? t('admin.console.brandContextFailed') : t('admin.console.brandUnauthorized') }}</p>
          <button type="button" v-refresh-button="t('admin.copy.855b9442')" class="button button-secondary" @click="withGlobalLoading(retryBrandScope)">{{ t('admin.copy.855b9442') }}</button>
        </article>
      </section>

      <template v-if="selectedBrandId">
      <section v-if="page === '首页'" class="page-content">
        <OverviewPanel v-if="account" :key="account.id + ':' + selectedBrandId" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" @open-withdrawals="go('提现管理')" />
      </section>
      <section v-else-if="page === '品牌设置'" class="page-content">
        <div class="console-page-heading page-heading"><h1 class="console-page-title">{{ t('admin.settings.title') }}</h1></div>
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

          </article>
          <BrandPresentation v-if="selectedBrandId && brandSettingsView === 'presentation'" :key="`${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" @loaded="acceptPresentation"/>

          <AnnouncementsManagement v-if="selectedBrandId && brandSettingsView === 'announcements'" base-path="/api/v1/admin" :key="`announcements:${account.id}:${selectedBrandId}`" :actor-id="account.id" :brand-id="selectedBrandId" :locale="locale" :permissions="[...cashPermissions(account,selectedBrandId)]" @failure="handleQuotaFailure" />
          <BrandContacts v-if="brandSettingsView === 'contacts'" :key="`contacts:${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
          <div v-if="brandSettingsView === 'funds'" class="console-stack">
            <CashConsole base-path="/api/v1/admin" mode="settings" :key="`cash:${account.id}:${selectedBrandId}`" :actor-id="account.id" :brand-id="selectedBrandId" :locale="locale" :permissions="[...cashPermissions(account,selectedBrandId)]" @failure="handleQuotaFailure" />
            <PointPolicySettings :key="`points:${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
          </div>
          <div v-if="brandSettingsView === 'withdrawal'" class="console-stack">
            <WithdrawalPolicySettings :key="`withdrawal-policy:${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
          </div>
          <BetPolicySettings v-if="brandSettingsView === 'betting'" embedded :key="`bet-policy:${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
          <div v-if="brandSettingsView === 'agents'" class="console-stack">
            <AgentManagement :key="`agent-settings:${account.id}:${selectedBrandId}`" mode="settings" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
            <CommissionPolicySettings :key="`commission-policy:${account.id}:${selectedBrandId}`" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
          </div>
      </section>

      <section
        v-else-if="page === '会员管理'"
        class="page-content directory-page"
      >
        <div class="console-page-heading page-heading">
          <div>
            <h1 class="console-page-title">{{ t('admin.nav.users') }}</h1>
          </div>
          <div class="console-page-actions">


          </div>
        </div>
          <template v-if="selectedBrandId">
            <DetailModal class="ui-form-dialog" v-if="selectedBrandId" id="member-create-form" :open="memberCreateOpen" :title="t('admin.copy.40acb767')" :close-label="t('association.close')" @close="memberCreateOpen = false">
              <MemberProvision :key="selectedBrandId" :account="account" :brand-id="selectedBrandId" @created="memberCreateOpen = false; withGlobalLoading(loadMembers)" @session-invalid="clearAdminData" />
            </DetailModal>

            <ManagementList :busy="membersLoading"><template v-if="hasPermission('user.create.brand')" #actions><button v-if="hasPermission('user.create.brand')" type="button" class="button button-primary" :aria-expanded="memberCreateOpen" aria-controls="member-create-form" @click="memberCreateOpen = !memberCreateOpen"><SiteIcon :name="memberCreateOpen ? 'close' : 'plus'" />{{ t('admin.copy.40acb767') }}</button></template><template #filters>
              <DirectorySearch :locale="locale" v-model="search" :label="t('admin.members.search')" :query-label="t('admin.members.query')" :busy="membersLoading" :search="searchMembers"><SelectControl v-model="memberOnlineFilter" :label="t('admin.members.onlineStatus')" :options="[{ value: '', label: (String(t('admin.members.allOnline'))).trim() }, { value: 'true', label: (String(t('admin.members.online'))).trim() }, { value: 'false', label: (String(t('admin.members.offline'))).trim() }]" /></DirectorySearch></template>
              <BatchDelete subject="accounts" :entry-label="member => member.username || member.phone" :scope-key="String(memberOffset) + JSON.stringify(search)" feedback="toast" :key="account.id + ':' + selectedBrandId" :entries="members" :enabled="canDeleteMembers" :locale="locale" :loading="membersLoading" :delete-entry="(member, key) => api.deleteMember(member.brand_id, member.id, key)" :error-text="cause => display(apiErrorText(cause))" :is-unknown="cause => !(cause instanceof AdminApiError) || cause.status === 0 || cause.status >= 500 || cause.code === 'INVALID_RESPONSE'" @deleted="batchMemberDeleted" @finished="withGlobalLoading(refreshBatchMembers)" v-slot="{ selected, toggle, selectLabel, locked }">
              <ManagementTable table-class="member-directory-table" :entries="members" :columns="[{key:'account',label:t('admin.copy.851c6121')},{key:'online',label:t('admin.members.onlineStatus')},{key:'points',label:t('admin.members.points'),cellClass:'member-points'},{key:'phone',label:t('admin.copy.d794da3d')},{key:'status',label:t('admin.copy.cf6d9c6d')},{key:'joined',label:t('admin.copy.ac08e5ed')},{key:'notes',label:t('admin.copy.fd2d2c8b'),cellClass:'member-notes'},{key:'tags',label:t('admin.copy.b96f43c6')},{key:'actions',label:t('admin.copy.d21fcbef')}]" :empty="!membersLoading && !membersError && !members.length" :empty-label="t('admin.copy.ef8be9d6')">
                <template #account="{entry:m}"><span class="member-account-select"><label v-if="canDeleteMembers" class="member-select-control"><input type="checkbox" :checked="selected.includes(m.id)" :aria-label="selectLabel(m.username || m.phone)" :disabled="locked" @change="toggle(m.id)" /></label><button type="button" class="text-button member-account" @click="memberDetail=m">{{m.username || m.phone || t('association.unnamed')}}</button></span></template>
                <template #online="{entry:m}"><span class="member-presence" :class="{'is-online':m.online}"><i aria-hidden="true" />{{t(m.online?'admin.members.online':'admin.members.offline')}}</span></template>
                <template #points="{entry:m}">{{m.display_points===null?t('admin.members.pointsRestricted'):formatPoints(m.display_points,locale)}}</template>
                <template #phone="{entry:m}">{{m.phone || '—'}}</template>
                <template #status="{entry:m}"><span class="badge" :class="statusClass(m.status)">{{t(adminKey(statusLabel[m.status]))}}</span></template>
                <template #joined="{entry:m}">{{formatDisplayDateTime(m.joined_at,displayLocale)}}</template>
                <template #notes="{entry:m}">{{m.notes || '—'}}</template>
                <template #tags="{entry:m}"><span v-for="tag in m.tags" :key="tag" class="member-tag">{{tag}}</span><span v-if="!m.tags.length">—</span></template>
                <template #actions="{entry:m}"><MemberActionsMenu :id="`member-actions-${m.id}`" :label="t('admin.members.actions',{account:m.username || m.phone})"><button v-if="canViewMemberWallet" type="button" role="menuitem" @click="memberFinance = { member: m, view: 'wallet' }">{{t('admin.nav.points')}}</button><button v-if="canViewMemberWallet" type="button" role="menuitem" @click="memberFinance = { member: m, view: 'ledger' }">{{t('admin.nav.ledger')}}</button><button v-if="canRechargeMember" type="button" role="menuitem" @click="rechargeMember = m">{{t('admin.recharge.menu')}}</button><button type="button" role="menuitem" :disabled="!canEditUsers && !canResetPasswords" @click="openEdit(m)">{{t('admin.copy.24dfbf6c')}}</button><button v-if="m.online && canKickUsers" type="button" role="menuitem" @click="openKick(m)">{{t('admin.copy.d192029b')}}</button></MemberActionsMenu></template>
              </ManagementTable>

              </BatchDelete>
            <template #pagination><PaginationControls :total="membersTotal" :page="Math.floor(memberOffset / memberPageSize + 1)" :count="members.length" :locale="locale" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.ee97c955')" :busy="membersLoading" :has-previous="memberOffset>0" :has-next="memberHasNext"  :page-size="memberPageSize" :go-to="(target: number) => changeMemberPage(target - (Math.floor(memberOffset / memberPageSize + 1)))" :change-page-size="async (size: number) => { memberPageSize = size; memberOffset = 0; await loadMembers() }" /></template></ManagementList>
          </template>
      </section>

      <section v-else-if="page === '代理列表'" class="page-content">
        <AgentManagement :key="`agents:${account.id}:${selectedBrandId}`" mode="directory" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
      </section>

      <section v-else-if="page === '彩种管理' || page === '盘口赔率'" class="page-content" data-testid="brand-game-management">
        <PlatformLottery
          v-if="account && selectedBrandId && hasPermission('platform_game.view.brand')"
          :key="`${account.id}:${selectedBrandId}:${page}`"
          :odds-only="page === '盘口赔率'"
          surface="admin"
          :brand-id="selectedBrandId"
          :locale="displayLocale"
          :can-manage="false"
          :can-connect="hasPermission('platform_game.connect.brand')"
          :can-sell="hasPermission('platform_game.sell.brand')"
          @failure="clearAdminData"
        />
        <p v-else class="panel directory-state" role="status">{{ t('admin.games.viewDenied') }}</p>
      </section>

      <section v-else-if="page === '开奖记录'" class="page-content">
        <BrandDrawHistory v-if="account && selectedBrandId && hasPermission('platform_game.view.brand')" :key="`${account.id}:${selectedBrandId}`" :brand-id="selectedBrandId" :locale="displayLocale" @failure="clearAdminData" />
        <p v-else class="panel directory-state" role="status">{{ t('admin.games.viewDenied') }}</p>
      </section>

      <section v-else-if="page === '注单分布'" class="page-content">
        <BetDistribution v-if="account && selectedBrandId && hasPermission('bet.view.brand')" :key="`${account.id}:${selectedBrandId}`" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
      </section>

      <section v-else-if="page === '注单管理'" class="page-content">
        <BetOrderManagement
          v-if="account && selectedBrandId"
          :key="`bet-orders-${account.id}:${page}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else-if="account" class="panel directory-state"> {{ t('admin.copy.5f930822') }} </div>
        <div v-else class="panel directory-state"> {{ t('admin.copy.93fc2a9d') }} <button class="button button-secondary" @click="go('会员管理')"> {{ t('admin.copy.9cb30737') }} </button>
        </div>
      </section>


      <section v-else-if="page === '额度管理'" class="page-content">
        <RechargeQuotaPanel v-if="account && selectedBrandId && hasPermission('recharge.view.brand')" standalone
          :key="`${account.id}:${selectedBrandId}`" :brand-id="selectedBrandId" :locale="displayLocale"
          base-path="/api/v1/admin" @failure="handleQuotaFailure" />
      </section>

      <section v-else-if="financeView" class="page-content">
        <CashConsole v-if="account && selectedBrandId && ['withdrawals','recharges'].includes(financeView || '')" base-path="/api/v1/admin" :mode="financeView === 'withdrawals' ? 'withdrawal' : 'deposit'" :key="`cash:${account.id}:${selectedBrandId}:${financeView}`" :actor-id="account.id" :brand-id="selectedBrandId" :locale="locale" :permissions="[...cashPermissions(account,selectedBrandId)]" @failure="handleQuotaFailure" />
        <WithdrawalManagement
          :active="financeView === 'withdrawals'"
          v-if="account && selectedBrandId && financeView === 'withdrawals'"
          :key="`withdrawals:${account.id}:${selectedBrandId}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <FinanceManagement
          v-if="account && selectedBrandId && financeView === 'recharges'"
          view="recharges"
          :key="`${selectedBrandId}:${page}`"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-if="!account || !selectedBrandId" class="panel directory-state">
          <h1>{{ navLabel(page) }}</h1>
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
            @click="go('会员管理')"
          > {{ t('admin.copy.5fc0fd8e') }} </button>
        </div>
      </section>

      <section v-else-if="page === '佣金管理'" class="page-content">
        <CommissionManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" :brand-status="selectedBrand?.status" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state">
          <h1>{{ t('admin.commission.ui.management') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.ea2574e4') }}</p>
        </div>
      </section>


      <section v-else-if="page === '人工奖励'" class="page-content">
        <RewardsManagement v-if="account && selectedBrandId" :key="`rewards:${account.id}:${selectedBrandId}`"
          :account="account" :brand-id="selectedBrandId" :brand-status="selectedBrand?.status" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ t('admin.copy.3fecab01') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.197fd776') }}</p>
        </div>
      </section>


      <section v-else-if="page === '业务报表'" class="page-content">
        <ReportsManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId"
          :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else class="panel directory-state"><h1>{{ t('admin.nav.reports') }}</h1>
          <p>{{ account ? t('admin.copy.8d09da6f') : t('admin.copy.d9310eab') }}</p>
          <button v-if="!account" class="button button-primary" @click="go('会员管理')">{{ t('admin.copy.5fc0fd8e') }}</button>
        </div>
      </section>

      <section v-else-if="page === '管理账号' || page === '角色权限'" class="page-content">
        <AccessManagement
          :section="page === '角色权限' ? 'roles' : 'accounts'"
          :key="`${selectedBrandId}:${page}`"
          v-if="account && selectedBrandId"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else class="panel directory-state">
          <h1>{{ navLabel(page) }}</h1>
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
            @click="go('会员管理')"
          > {{ t('admin.copy.5fc0fd8e') }} </button>
        </div>
      </section>

      <section v-else class="page-content">
        <AuditManagement v-if="account && selectedBrandId" :key="account.id + ':' + selectedBrandId" :account="account" :brand-id="selectedBrandId" @session-invalid="clearAdminData" />
        <div v-else-if="account" class="panel directory-state"><h1>{{ t('admin.copy.315a9366') }}</h1><p>{{ t('admin.copy.8d09da6f') }}</p></div>
      </section>

      </template>
  <FinanceManagement v-if="memberFinance && account && selectedBrandId && canViewMemberWallet"
    :key="`${account.id}:${selectedBrandId}:${memberFinance.member.id}:${memberFinance.view}`"
    :account="account" :brand-id="selectedBrandId" :view="memberFinance.view"
    :initial-member-id="memberFinance.member.id" :initial-member-username="memberFinance.member.username || memberFinance.member.phone"
    @close="closeMemberFinance" @changed="withGlobalLoading(loadMembers)" @session-invalid="clearAdminData" />

  <DetailModal class="console-member-dialog ui-form-dialog" v-if="rechargeMember && account && canRechargeMember" :title="t('admin.recharge.manual')" :close-label="t('admin.recharge.close')" :close-disabled="rechargePending" @close="!rechargePending && (rechargeMember = null)">
    <ManualRechargeForm :account-id="account.id" :key="`${selectedBrandId}:${rechargeMember.id}`" :brand-id="selectedBrandId" :member-id="rechargeMember.id" :username="rechargeMember.username || rechargeMember.phone" @session-invalid="clearAdminData" @pending="rechargePending = $event" @credited="rechargeMember = null; withGlobalLoading(loadMembers)" />
  </DetailModal>

      <DetailModal v-if="memberDetail && account && selectedBrandId" :title="t('admin.members.details')" :close-label="t('association.close')" @close="memberDetail = null">
        <dl class="confirm-list member-details">
          <div><dt>{{ t('admin.copy.851c6121') }}</dt><dd>{{ memberDetail.username || memberDetail.phone || t('association.unnamed') }}</dd></div>
          <div><dt>{{ t('admin.copy.b60e278d') }}</dt><dd>{{ memberDetail.display_name || '—' }}</dd></div>
          <div><dt>{{ t('admin.members.onlineStatus') }}</dt><dd>{{ t(memberDetail.online ? 'admin.members.online' : 'admin.members.offline') }}</dd></div>
          <div><dt>{{ t('admin.members.points') }}</dt><dd><span v-if="memberWallet" :title="memberPointsHint" :tabindex="memberPointsHint ? 0 : undefined">{{ formatPoints(memberWallet.display_points,locale) }}<template v-if="memberPointsHint"> / {{ memberWallet.available_points }}</template></span><template v-else-if="memberWalletError" /><span v-else>{{ canViewMemberWallet ? '—' : t('admin.members.pointsRestricted') }}</span></dd></div>
          <div><dt>{{ t('admin.copy.d794da3d') }}</dt><dd>{{ memberDetail.phone || '—' }}</dd></div>
          <div><dt>{{ t('admin.copy.cf6d9c6d') }}</dt><dd>{{ t(adminKey(statusLabel[memberDetail.status])) }}</dd></div>
          <div><dt>{{ t('admin.copy.ac08e5ed') }}</dt><dd>{{ formatDisplayDateTime(memberDetail.joined_at, displayLocale) }}</dd></div>
          <div><dt>{{ t('admin.copy.fd2d2c8b') }}</dt><dd>{{ memberDetail.notes || '—' }}</dd></div>
          <div><dt>{{ t('admin.copy.b96f43c6') }}</dt><dd>{{ memberDetail.tags.join(' · ') || '—' }}</dd></div>
        </dl>
        <JoinCodeManagement v-if="canViewJoinCodes" :key="account.id + ':' + selectedBrandId + ':' + memberDetail.id" :account="account" :brand-id="selectedBrandId" :member-id="memberDetail.id" kind="referral" @session-invalid="clearAdminData" />
      </DetailModal>
      <DetailModal v-if="editTarget" :open="!resetTarget" class="console-member-dialog ui-form-dialog" :title="t('admin.copy.0ebe57dc')" :close-label="t('association.close')" :close-disabled="editBusy" @close="!editBusy && (editTarget = null)">
          <p>{{ editTarget.username || editTarget.phone || t('association.unnamed') }}</p>
          <label class="modal-label"
            >{{ t('admin.copy.cf6d9c6d') }}<SelectControl v-model="editStatus" :disabled="!canEditUsers" :label="String(t('admin.copy.cf6d9c6d'))" :options="Object.entries(statusLabel).map(([status, label]) => ({ value: status, label: (String(t(adminKey(label)))).trim() }))" /></label
          ><label class="modal-label"
            >{{ t('admin.copy.fd2d2c8b') }}<textarea
              v-model="editNotes" :disabled="!canEditUsers"
              rows="3"
              maxlength="2000"
            ></textarea></label
          >
          <button v-if="canResetPasswords" type="button" class="button button-secondary" :disabled="editBusy" @click="openReset(editTarget)">{{ t('admin.copy.d1e2ba79') }}</button>

          <div class="modal-actions ui-form-actions">
            <button
              v-write-guard="editBusy"
              class="button button-primary"
              :disabled="editBusy || !canEditUsers"
              @click="withGlobalLoading(saveMember)"
            >
              {{ editBusy ? t('admin.copy.2462134b') : t('admin.copy.7eeb35b3') }}
            </button>
          </div>
      </DetailModal>
      <DetailModal v-if="kickTarget" class="console-member-dialog ui-form-dialog" :title="t('admin.copy.b7cf2b50')" :close-label="t('association.close')" :close-disabled="kickBusy" @close="!kickBusy && (kickTarget = null)">
          <p>{{ kickTarget.username || kickTarget.phone || t('association.unnamed') }}</p>
          <p class="danger-note"> {{ t('admin.copy.cf1837eb') }} </p>

          <div class="modal-actions ui-form-actions">
            <button
              v-write-guard="kickBusy"
              class="button button-danger"
              :disabled="kickBusy "
              @click="withGlobalLoading(kickMember)"
            >
              {{ kickBusy ? t('admin.copy.57f408ff') : t('admin.copy.6f261b8f') }}
            </button>
          </div>
      </DetailModal>
      <DetailModal v-if="resetTarget" class="console-member-dialog ui-form-dialog" :title="t('admin.copy.c608f380')" :close-label="t('association.close')" :close-disabled="resetBusy" @close="!resetBusy && (resetTarget = null)">
          <p>{{ resetTarget.username || resetTarget.phone || t('association.unnamed') }}</p>
          <label class="modal-label"
            >{{ t('admin.copy.6b88b89d') }} <span>{{ t('admin.copy.2ed0ac9e') }}</span
            ><input
              v-model="resetPasswordValue"
              class="field"
              type="password"
              autocomplete="new-password"
              required /></label
          >

          <div class="modal-actions ui-form-actions">
            <button
              v-write-guard="resetBusy"
              class="button button-danger"
              :disabled="
                resetBusy ||
                !resetPasswordValue
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
          ><button type="button" class="icon-button" :aria-label="t('admin.copy.c621342b')" @click="mobileMore = false"><SiteIcon name="close" /></button>
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
    <nav class="mobile-nav" :style="{ '--console-nav-count': mobileNavPages.length + 1 }" :aria-label="t('admin.copy.12b204e9')">
      <button
        v-for="target in mobileNavPages"
        :key="target"
        :class="{ active: page === target }"
        @click="go(target)"
      >
        <span><SiteIcon :name="nav.find((item) => item.name === target)!.icon" /></span
        ><small>{{ target === "会员管理" ? t("admin.nav.membersShort") : target === "彩种管理" ? t("admin.nav.gamesShort") : target === "期次管理" ? t("admin.nav.periodsShort") : navLabel(target) }}</small></button
      ><button
        :class="{ active: mobileMore }"
        :aria-expanded="mobileMore"
        @click="mobileMore = !mobileMore"
      >
        <span><SiteIcon name="menu" /></span><small>{{ t('admin.copy.258d2d94') }}</small>
      </button>
    </nav>
    <AccountProfileDialog v-if="accountProfileOpen" :title="t('admin.copy.55de09a4')" :close-label="t('association.close')" :fields="[{label:t('admin.copy.851c6121'),value:account.username || t('association.unnamed')},{label:t('admin.copy.d48825ff'),value:account.super_admin ? t('admin.copy.543831fc') : t('admin.copy.d48825ff')}]" @close="accountProfileOpen=false" />
    <ChangePasswordDialog v-if="changePasswordOpen" v-model:current="currentPassword" v-model:next="newPassword" v-model:confirmation="confirmPassword" :title="t('admin.copy.14ce8ba8')" :close-label="t('association.close')" :current-label="t('admin.copy.7e7f11f6')" :new-label="t('admin.copy.683caabd')" :confirm-label="t('admin.copy.edc96e77')" :submit-label="passwordBusy ? t('admin.copy.2462134b') : t('admin.copy.e93fc68f')" :busy="passwordBusy" @close="changePasswordOpen=false" :submit="submitPasswordChange"></ChangePasswordDialog>
  </ManagementFrame>
  <OperationToastHost />
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
.brand-operation-list__identity small { color: #718096; font-size: var(--body-font-size); }
.brand-operation-list__mark { display: grid; width: 36px; height: 36px; flex: 0 0 36px; place-items: center; border-radius: 10px; background: #ecfdf5; color: #047857; font-weight: 700; }
@media (max-width: 700px) { .brand-operation-list li { grid-template-columns: minmax(0, 1fr) auto; gap: 10px; } .brand-operation-list li > .text-button { grid-column: 1 / -1; justify-self: start; } }
@media (min-width: 901px) and (pointer: fine) {
  .brand-operation-list li { padding-block: 8px; }
}
</style>
