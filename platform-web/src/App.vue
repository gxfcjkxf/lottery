<script setup lang="ts">
import AuditFiltersForm from "@lottery/shared/AuditFilters.vue"
const auditFilters = ref<Record<string, string>>({})
function applyAuditFilters(next: Record<string, string>) { auditFilters.value = next; return loadAudit(selectedBrand.value, 0) }
import ThemeSwitcher from "@lottery/shared/ThemeSwitcher.vue"
import AnnouncementsManagement from "@lottery/shared/AnnouncementsManagement.vue";
import CashConsole from "@lottery/shared/CashConsole.vue"
import { formatPoints } from "@lottery/shared";

import ClearableInput from "@lottery/shared/ClearableInput.vue"
import { readPaginationTotal } from '@lottery/shared'
const auditTotal = ref('0')

import ManagementTable from "@lottery/shared/ManagementTable.vue"
import { auditReasonLabel, businessActionLabel, businessResourceLabel } from '@lottery/shared'
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import OperationToastHost from "@lottery/shared/OperationToastHost.vue"
import ManagementList from "@lottery/shared/ManagementList.vue"
import BatchDelete from "@lottery/shared/BatchDelete.vue"
import { formValidationText } from '@lottery/shared'
import DirectorySearch from "@lottery/shared/DirectorySearch.vue"
import AccountProfileDialog from "@lottery/shared/AccountProfileDialog.vue"
import ChangePasswordDialog from "@lottery/shared/ChangePasswordDialog.vue"
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import AccountMenu from "@lottery/shared/AccountMenu.vue";
import LoginEntry from '@lottery/shared/LoginEntry.vue'
import LoginCredentials from '@lottery/shared/LoginCredentials.vue'
import { vWriteGuard } from "@lottery/shared";
import {applyBrowserBranding,defaultBrand} from "@lottery/shared";
import { associationContextKey } from '@lottery/shared';
import { provide as provideAssociationContext } from 'vue';
import { usePlatformI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
import { vInlineValidation, platformDisplaySettings, setPlatformDisplaySettings } from '@lottery/shared'
import LanguageSwitcher from '@lottery/shared/LanguageSwitcher.vue'
import RechargeQuotaPanel from '@lottery/shared/RechargeQuotaPanel.vue'
import MemberActionsMenu from '@lottery/shared/MemberActionsMenu.vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import TablePagination from '@lottery/shared/TablePagination.vue'
import { nativeLocaleName, useSessionReplacement } from '@lottery/shared'
import { computed, onMounted, nextTick, ref, shallowRef, watch, watchEffect } from 'vue'
import { createPlatformApi, freezeBrandCreateRequest, PlatformApiError, type BrandCreateInput, type FrozenBrandCreateRequest, type PlatformAudit, type PlatformBrand, type PlatformMember, type PlatformAccount, type PlatformSettings, type PlatformPresentation } from './platform-api'
import { useBrandGames } from './use-brand-games'
import { platformErrorMessage } from './platform-error-message'
import { usePlatformI18n } from './i18n'
import RewardsPanel from './RewardsPanel.vue'
import WalletPanel from './WalletPanel.vue'
import RechargesPanel from './RechargesPanel.vue'
import ReportsPanel from './ReportsPanel.vue'
import BetOrdersPanel from './BetOrdersPanel.vue'
import DrawsPanel from './DrawsPanel.vue'
import PlatformLottery from '@lottery/shared/PlatformLottery.vue'
import OfficialDrawCalendar from './OfficialDrawCalendar.vue'
import WithdrawalsPanel from './WithdrawalsPanel.vue'
import CommissionPanel from './CommissionPanel.vue'
import DataRetentionPanel from './DataRetentionPanel.vue'
import NotificationsPanel from './NotificationsPanel.vue'
import {clearRetentionIntents} from './data-retention-api'
import AccessPanel from './AccessPanel.vue'
import BrandOperationPanel from './BrandOperationPanel.vue'
import BrandDomainsPanel from './BrandDomainsPanel.vue'
import { clearAllPendingBrandDomainsWrites } from '../../admin-web/src/brand-domains-state'
import type { BrandOperation } from './brand-operation-api'
import PlatformSettingsPanel from './PlatformSettingsPanel.vue'
import OverviewPanel from './OverviewPanel.vue'
import { securityIntents } from './security-settings-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'
const brandPageSize = ref(50)
const auditPageSize = ref(50)
const memberPageSize = ref(50)


const api = createPlatformApi()
const { locale, setLocale: updateLocale, t, message } = usePlatformI18n()
function setLocale(value: string) { updateLocale(value) }
const authReady = ref(false)
function createLang() {
  return {
  title: t('platform.copy.a220464b'), loginTitle: t('platform.copy.d62bdbd5'), identifier: t('platform.copy.eeaa7275'), password: t('platform.copy.2a771a31'), signIn: t('platform.copy.ec3b65e9'),
  brands: t('platform.copy.79dd4298'), users: t('platform.copy.d84b8968'), brandAccounts: t('platform.copy.9c2e4b71'), audit: t('platform.copy.8262ff2c'), rewards: t('platform.copy.25982fc9'), createBrand: t('platform.copy.767e7f67'), selectBrand: t('platform.copy.1c0a1845'), allBrands: t('platform.copy.0a8ccdef'), brandCount: t('platform.copy.6a2943a0'),
  name: t('platform.copy.33b6c8e3'), code: t('platform.copy.9ba39db0'), status: t('platform.copy.c7260510'), view: t('platform.copy.b9fbecbf'), membersAction: t('platform.copy.c4bf0312'), accountsAction: t('platform.copy.d944b7ec'), statusAction: t('platform.copy.c7260510'), deleteAction: t('platform.copy.9851501a'), deleteBrand: t('platform.copy.15adaf08'), deleteConfirm: t('platform.copy.ac1de974'), deleteWarning: t('platform.copy.133eb17e'), deleteReason: t('platform.copy.351c71c8'), deleteSuccess: t('platform.copy.e188a90c'), username: t('platform.copy.eeaa7275'), role: t('platform.copy.6fc23cbb'), displayName: t('platform.copy.bcd89469'), phone: t('platform.copy.d05d61ad'), memberCount: t('platform.copy.1bd8aaad'), brandTotal: t('platform.copy.fa68aba0'),
  locale: t('platform.copy.43eda349'), timezone: t('platform.copy.c67c04be'), reason: t('platform.copy.f546783b'), cancel: t('platform.copy.2230b1a7'), review: t('platform.copy.9b85d866'), confirm: t('platform.copy.312420fb'), confirmation: t('platform.copy.e204e6ed'), action: t('platform.copy.75b7f6d4'), actor: t('platform.copy.3177f27f'), resource: t('platform.copy.de6bef13'), date: t('platform.copy.7069c6b9'), empty: t('platform.copy.b4600402'), loading: t('platform.copy.fa02e7b1'), logout: t('platform.copy.2315867e'), close: t('platform.copy.27b2fd29'), platformNote: '', platformAdmin: t('platform.copy.45ec43b6'), settings: t('common.platformSettings'), error: t('platform.copy.8b844392'), retry: t('platform.copy.8c0fb218'), unknown: t('platform.copy.e6aabad3'), created: t('platform.copy.bb362b87'), localeEnglish: t('platform.copy.f7f79167'), localeChinese: t('platform.copy.01f1aac2'), directory: t('platform.copy.0ef111d5'), directoryDescription: '', brandDirectory: t('platform.copy.6c32f38f'), brandDirectoryDescription: '', platformDirectory: t('platform.copy.3916c3b1'), createDescription: t('platform.copy.8e3e641a'), codePlaceholder: t('platform.copy.3d397d5f'), timezonePlaceholder: t('platform.copy.990c4cb9'), accountLabel: t('platform.copy.ce2d66f3'), brandLabel: t('platform.copy.d5b68c0f'), statusNormal: t('platform.copy.0e1df295'), statusFrozen: t('platform.copy.d21ee5d7'), statusDisabled: t('platform.copy.6762439b'), statusExpired: t('platform.copy.02dd0e6a'), statusCancelled: t('platform.copy.d9b0562c'), statusPaused: t('platform.copy.3d294087'), statusActive: t('platform.copy.15a92cfa'), countSuffix: t('platform.copy.13d2abd9'), accountProfile: t('platform.copy.9f9d8eb6'), accountPassword: t('platform.copy.3f21ce3e'), accountId: t('platform.copy.f985024d'),
  deleteMember: t('common.deleteMember'), memberDeleted: t('common.memberDeleted'),
  }
}
type PlatformCopy = ReturnType<typeof createLang>
const lang = computed<PlatformCopy>(createLang)
const mobileNavigationOpen = ref(false)
function closeMobileNavigation(event: MouseEvent) {
  if (event.target instanceof Element && event.target.closest('button:not([aria-expanded])')) mobileNavigationOpen.value = false
}
const account = ref<PlatformAccount | null>(null)
const platformSettings = ref<PlatformSettings | null>(null)
const platformPresentation = ref<PlatformPresentation | null>(null)
const displaySettings = computed<PlatformDisplaySettings>(() => platformDisplaySettings.value!)
watch(platformSettings, value => { if (value) { setPlatformDisplaySettings(value); platformPresentation.value = { display_name: value.display_name, logo_text: value.logo_text, logo_url: value.logo_url } } })
watchEffect(() => {
  if (platformPresentation.value) { const value = platformPresentation.value; applyBrowserBranding(value.display_name, value.logo_text, value.logo_url, defaultBrand.primary) }
})
const brandOperationPanel = ref<{ openForBrand: (brandId: string) => void } | null>(null)
const accessScope = ref<'brand' | 'platform'>('platform')
const accountWriteLocked = ref(false)
const domainBrand = ref<PlatformBrand | null>(null)
const brands = ref<PlatformBrand[]>([])
const brandQuery = ref('')
const brandStatus = ref('')
const brandPage = ref(1), brandListLoading = ref(false)
const appliedBrandQuery = ref(''), appliedBrandStatus = ref('')
const filteredBrands = computed(() => {
  const query = appliedBrandQuery.value.trim().toLowerCase()
  return brands.value.filter(brand => (!appliedBrandStatus.value || brand.status === appliedBrandStatus.value) &&
    (!query || `${brand.name} ${brand.code}`.toLowerCase().includes(query)))
})
const brandEntries = computed(() => filteredBrands.value.slice((brandPage.value - 1) * brandPageSize.value, brandPage.value * brandPageSize.value))
watch(() => filteredBrands.value.length, count => { brandPage.value = Math.min(brandPage.value, Math.max(1, Math.ceil(count / brandPageSize.value))) })
async function queryBrands() {
  if (!await refreshBrandSummaries()) return
  appliedBrandQuery.value = brandQuery.value.trim(); appliedBrandStatus.value = brandStatus.value; brandPage.value = 1
}

async function pageBrands(target: number) { if (await refreshBrandSummaries()) brandPage.value = Math.min(target, Math.max(1, Math.ceil(filteredBrands.value.length / brandPageSize.value))) }
function batchBrandDeleted(brand: PlatformBrand) {
  brands.value = brands.value.filter(item => item.id !== brand.id)
  if (selectedBrand.value === brand.id) { selectedBrand.value = ''; members.value = []; [auditRows.value, auditTotal.value] = [[], '0'] }
}
const directoryCopy = computed(() => locale.value === 'en' ? {
  search: t('platform.copy.e572aa2e'), hint: t('platform.copy.607f1d49'), status: t('platform.copy.7ab7ba64'), all: t('platform.copy.69b1cb0f'), clear: t('platform.copy.51de817e'), submit: t('platform.copy.0e708b55'), empty: t('platform.copy.2aaccf31'),
} : {
  search: t('platform.copy.e572aa2e'), hint: t('platform.copy.607f1d49'), status: t('platform.copy.7ab7ba64'), all: t('platform.copy.69b1cb0f'), clear: t('platform.copy.51de817e'), submit: t('platform.copy.0e708b55'), empty: t('platform.copy.2aaccf31'),
})
const members = ref<PlatformMember[]>([])
const auditRows = ref<PlatformAudit[]>([])
const memberOffset = ref(0)
const auditOffset = ref(0)
const memberTotal = ref('0')
const memberSearch = ref(''), memberQuery = ref('')
const memberOnlineFilter = ref<'' | 'true' | 'false'>(''), memberOnlineQuery = ref<'' | 'true' | 'false'>('')
const memberDetail = ref<PlatformMember | null>(null)
const canViewMemberPoints = computed(() => account.value?.platform_permissions.includes('user.view.platform') === true && account.value.platform_permissions.includes('wallet.view.platform'))
const moreMembers = computed(() => BigInt(memberOffset.value) + BigInt(members.value.length) < BigInt(memberTotal.value))
function resetMemberFilters() { memberSearch.value = ''; memberQuery.value = ''; memberOnlineFilter.value = ''; memberOnlineQuery.value = ''; memberTotal.value = '0'; memberDetail.value = null }
async function searchMembers() { memberQuery.value = memberSearch.value.trim(); memberOnlineQuery.value = memberOnlineFilter.value; await loadMembers(selectedBrand.value) }

const moreAudit = ref(false)
const previousPage = computed(() => t('platform.pagination.previous'))
const nextPage = computed(() => t('platform.pagination.next'))
const selectedBrand = ref('')
const quotaBrand = ref<PlatformBrand | null>(null)
const canViewRechargeQuota = computed(() => account.value?.platform_permissions.includes('recharge.view.platform') === true)
const canGrantRechargeQuota = computed(() => account.value?.platform_permissions.includes('recharge_quota.write.platform') === true)

provideAssociationContext(associationContextKey, computed(() => brands.value.find(item => item.id === selectedBrand.value) ?? account.value));
const brandSelectionKey = 'lottery.platform.selectedBrand'
watch(selectedBrand, brandId => {
  if (brandId && account.value) sessionStorage.setItem(brandSelectionKey, JSON.stringify({ accountId: account.value.id, brandId }))
  else sessionStorage.removeItem(brandSelectionKey)
}, { flush: 'sync' })
const section = ref<'home' | 'brands' | 'users' | 'audit' | 'rewards' | 'wallets' | 'bets' | 'games' | 'draws' | 'withdrawals' | 'commission' | 'access' | 'settings'>('home')
const withdrawalInitialState = ref<'' | 'reviewing'>('')
const settingsTab = ref<'general' | 'auth' | 'commission' | 'currency'>('general')
const retentionPolicyVisible=ref(false)
const showBrandPicker = computed(() => section.value === 'settings' ? settingsTab.value === 'auth' : section.value === 'audit' && (auditView.value==='archives'&&retentionPolicyVisible.value || ['notifications', 'announcements', 'maintenance'].includes(auditView.value)) ? false : section.value === 'access' ? accessScope.value === 'brand' : !['home', 'brands', 'settings', 'games', 'draws'].includes(section.value))
const { games: brandGames, selectedGameId, loading: gamesLoading } = useBrandGames(selectedBrand, () => section.value === 'bets' ? section.value : '', handleFailure)
const auditView = ref<'audit' | 'archives' | 'notifications' | 'announcements' | 'deliveries' | 'maintenance'>('audit')
const expandedNavigation = ref<'brands' | 'operations' | 'wallets' | 'commission' | null>(null)
function navigationExpanded(group: NonNullable<typeof expandedNavigation.value>) {
  return computed({
    get: () => expandedNavigation.value === group,
    set: (open: boolean) => {
      if (open) expandedNavigation.value = group
      else if (expandedNavigation.value === group) expandedNavigation.value = null
    },
  })
}
const brandsExpanded = navigationExpanded('brands')
const brandSectionActive = computed(() => section.value === 'brands' || section.value === 'users' || (section.value === 'access' && accessScope.value === 'brand'))
const operationsExpanded = navigationExpanded('operations')
const walletView = ref<'recharges' | 'quota' | 'reports'>('recharges')
const walletsExpanded = navigationExpanded('wallets')
const commissionExpanded = navigationExpanded('commission')
const accessTab = ref<'accounts'>('accounts')
const commissionTab = ref<'agents' | 'cycles' | 'payments'>('agents')
const memberFinance = ref<{ member: PlatformMember; view: 'wallet' | 'ledger' } | null>(null)
watch(() => [account.value?.id, selectedBrand.value], () => { memberFinance.value = null }, { flush: 'sync' })
const walletLabel = computed(() => t('common.finance'))
const pointsAction = computed(() => t('common.points'))
const memberColumns = computed(() => [
  {key:'account',label:lang.value.username}, {key:'online',label:t('members.onlineStatus')},
  {key:'points',label:pointsAction.value,cellClass:'member-points'}, {key:'phone',label:lang.value.phone},
  {key:'status',label:lang.value.status}, {key:'joined',label:t('members.joined')}, {key:'actions',label:t('common.actions')},
])
const betLabel = computed(() => t('common.betOrders'))
const gamesLabel = computed(() => t('common.games'))
const drawsLabel = computed(() => t('common.draws'))
const withdrawalLabel = computed(() => t('common.withdrawals'))
const agentsLabel = computed(() => t('common.agents'))
const commissionLabel = computed(() => t('common.commission'))
const commissionPageLabel = computed(() => commissionTab.value === 'agents' ? agentsLabel.value : commissionTab.value === 'cycles' ? t('platform.copy.39ac5c23') : t('platform.copy.48f3ae6f'))
const operationsLabel = computed(() => t('common.operations'))
const operationLabel = computed(() => auditView.value === 'announcements' ? t('announcements.title') : auditView.value === 'maintenance' ? t('maintenance.tools') : auditView.value === 'deliveries' ? t('platform.copy.42e5c206') : auditView.value === 'audit' ? lang.value.audit : auditView.value === 'archives' ? t('retention.title') : t('maintenance.notificationTemplates'))
const walletViewLabel = computed(() => walletView.value === 'quota' ? t('platform.quota.management') : walletView.value === 'recharges' ? t('platform.copy.20be6d4c') : t('platform.copy.67516591'))
const accountsLabel = computed(() => t('common.platformAccounts'))
const accessLabel = computed(() => accessScope.value === 'brand' ? lang.value.brandAccounts : accountsLabel.value)
const sectionLabel = computed(() => section.value === 'home' ? t('overview.home') : section.value === 'access' ? accessLabel.value : section.value === 'audit' ? operationLabel.value : section.value === 'commission' ? commissionPageLabel.value : section.value === 'wallets' ? walletViewLabel.value : section.value === 'bets' ? betLabel.value : section.value === 'games' ? gamesLabel.value : section.value === 'draws' ? drawsLabel.value : section.value === 'withdrawals' ? withdrawalLabel.value : lang.value[section.value])
const busy = ref(false)
const loading = ref(false)
const errorCause = shallowRef<unknown>('')
const uncertainError = ref(false)
const error = computed({
  get: () => errorCause.value === '' ? '' : `${platformErrorMessage(errorCause.value, locale.value)}${uncertainError.value ? ` — ${lang.value.unknown}` : ''}`,
  set: (value: string) => { errorCause.value = value; uncertainError.value = false },
})
const notice = ref<string | import('@lottery/shared').LocalizedMessage>('')
const identifier = ref('')
const password = ref('')
const modal = ref(false)
const confirming = ref(false)
const retryPayload = ref<FrozenBrandCreateRequest | null>(null)
const form = ref<BrandCreateInput>({ code: '', name: '', default_locale: 'en', timezone: 'UTC' })
const deleteBrandTarget = ref<PlatformBrand | null>(null)
const deleteBusy = ref(false)
const deleteError = ref<string | import("@lottery/shared").LocalizedMessage>("")
const deleteMemberTarget = ref<PlatformMember | null>(null)
const deleteMemberBusy = ref(false)
const deleteMemberError = ref<string | import("@lottery/shared").LocalizedMessage>("")
const deleteMemberIntent = ref<{ id: string; brand: string; key: string } | null>(null)
const deleteMemberUnknown = ref(false)
const canCreateBrand = computed(() => account.value?.platform_permissions.includes('brand.create.platform') === true)
const canDeleteBrand = computed(() => account.value?.super_admin === true)
const canDeleteMember = computed(() => account.value?.platform_permissions.includes('user.delete.platform') === true)
const selected = computed(() => brands.value.find(brand => brand.id === selectedBrand.value))
const accountName = computed(() => account.value?.username || account.value?.display_name || t('common.administrator'))
function auditActorLabel(row: PlatformAudit) {
  if (row.actor_username) return row.actor_username
  if (row.actor_type === 'system') return t('common.system')
  if (row.actor_type === 'user') return t('common.user')
  if (row.actor_type === 'anonymous') return t('common.anonymous')
  return t('common.unknownActor')
}
const accountProfileOpen = ref(false)
const changePasswordOpen = ref(false)
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const passwordBusy = ref(false)
const passwordError = ref<string | import("@lottery/shared").LocalizedMessage>("")
const canManageBrandOperation = computed(() => account.value?.platform_permissions.includes('brand_operation.view.platform') === true && account.value.platform_permissions.includes('brand_operation.write.platform'))
let memberRequestGeneration = 0
let auditRequestGeneration = 0

function updateBrandQuota(quota: {brand_id:string;available:string}) {
 const brand=brands.value.find(item=>item.id===quota.brand_id)
 if(brand && canViewRechargeQuota.value) brand.recharge_quota_available=quota.available
 error.value=''
}
function clearPrivateState() {
  domainBrand.value = null
  clearAllPendingBrandDomainsWrites()

  quotaBrand.value = null
  securityIntents.clearAll()
  section.value = 'home'
  sessionStorage.removeItem(brandSelectionKey)
  accountWriteLocked.value = false
  accessScope.value = 'platform'
  brandsExpanded.value = false
  brandQuery.value = ''; brandStatus.value = ''
  auditView.value = 'audit'
  operationsExpanded.value = false
  walletView.value = 'recharges'
  walletsExpanded.value = false
  memberFinance.value = null
  resetMemberFilters()
  clearRetentionIntents(); account.value = null; platformSettings.value = null; brands.value = []; members.value = []; [auditRows.value,auditTotal.value]=[[],'0']; selectedBrand.value = ''
  retryPayload.value = null; modal.value = false; confirming.value = false
  deleteBrandTarget.value = null; deleteError.value = ''; deleteBusy.value = false
  deleteMemberTarget.value = null; deleteMemberError.value = ''; deleteMemberBusy.value = false
  deleteMemberIntent.value = null; deleteMemberUnknown.value = false
  loading.value = false
  memberRequestGeneration++; auditRequestGeneration++
  memberOffset.value = 0; auditOffset.value = 0; memberTotal.value = '0'; moreAudit.value = false
}
const sessionReplaced = useSessionReplacement(() => !!account.value, () => api.me(), clearPrivateState)
function handleFailure(cause: unknown) {
  if (cause instanceof Error && (('status' in cause && cause.status === 401) || ('code' in cause && cause.code === 'PLATFORM_ADMIN_REQUIRED'))) clearPrivateState()
  errorCause.value = cause; uncertainError.value = false
}
async function loadWorkspace() {
  loading.value = true; error.value = ''
  try {
    const currentAccount = await api.me()
    const currentSettings = await api.platformSettings()
    account.value = currentAccount
    platformSettings.value = currentSettings
    brands.value = await api.brands()
    const savedSelection = sessionStorage.getItem(brandSelectionKey)
    if (savedSelection) {
      const saved = JSON.parse(savedSelection) as { accountId: string; brandId: string }
      if (saved.accountId === currentAccount.id && brands.value.some(brand => brand.id === saved.brandId)) selectedBrand.value = saved.brandId
      else { selectedBrand.value = ''; sessionStorage.removeItem(brandSelectionKey) }
    }
    if (selectedBrand.value && !brands.value.some(brand => brand.id === selectedBrand.value)) { selectedBrand.value = ''; members.value = []; [auditRows.value,auditTotal.value]=[[],'0'] }
    if (selectedBrand.value && section.value === 'users') await loadMembers(selectedBrand.value)
    if (selectedBrand.value && section.value === 'audit' && auditView.value === 'audit') await loadAudit(selectedBrand.value)
  } catch (cause) { handleFailure(cause); throw cause } finally { loading.value = false }
}
async function login() {
  sessionReplaced.value = false
  busy.value = true; error.value = ''
  try { await api.login(identifier.value, password.value); clearPrivateState(); password.value = ''; await loadWorkspace() }
  catch (cause) { handleFailure(cause) } finally { busy.value = false }
}
async function signOut() {
  busy.value = true; error.value = ''
  try { await api.logout(); clearPrivateState() }
  catch (cause) { handleFailure(cause) } finally { busy.value = false }
}
async function submitPasswordChange() {
  passwordError.value = ''
  const characters = Array.from(newPassword.value).length
  if (!currentPassword.value || characters < 6 || characters > 30 || newPassword.value !== confirmPassword.value) {
    passwordError.value = t('common.passwordError')
    return
  }
  passwordBusy.value = true
  try {
    await api.changePassword(currentPassword.value, newPassword.value)
    currentPassword.value = ''; newPassword.value = ''; confirmPassword.value = ''
    changePasswordOpen.value = false
    clearPrivateState()
  } catch (cause) {
    passwordError.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
  } finally { passwordBusy.value = false }
}
async function refreshBrandSummaries() {
 const actor=account.value?.id
 brandListLoading.value = true; error.value = ''
 try {
  const items=await api.brands()
  if(account.value?.id !== actor) return false
  brands.value=items; return true
 } catch(cause) { if(account.value?.id===actor) handleFailure(cause); return false }
 finally { brandListLoading.value = false }
}

async function chooseSection(next: typeof section.value, withdrawalState: '' | 'reviewing' = '') {
  if (accountWriteLocked.value) return
  withdrawalInitialState.value = withdrawalState
  if (next !== section.value) resetMemberFilters()
  if (next === 'audit' && section.value !== 'audit') auditView.value = 'audit'
  memberFinance.value = null
  expandedNavigation.value = ['brands', 'users'].includes(next) || (next === 'access' && accessScope.value === 'brand') ? 'brands' : next === 'audit' ? 'operations' : next === 'wallets' ? 'wallets' : next === 'commission' ? 'commission' : null
  section.value = next; error.value = ''; notice.value = ''
  if(next === 'brands') await refreshBrandSummaries()
  memberOffset.value = 0; auditOffset.value = 0; memberTotal.value = '0'; moreAudit.value = false
  if (next !== 'users') { memberRequestGeneration++; members.value = [] }
  if (next !== 'audit') { auditRequestGeneration++; [auditRows.value,auditTotal.value]=[[],'0'] }
  if (!selectedBrand.value || (next !== 'audit' && next !== 'users')) loading.value = false
  if (selectedBrand.value && next === 'audit') await loadAudit(selectedBrand.value)
  if (selectedBrand.value && next === 'users') await loadMembers(selectedBrand.value)
}
async function chooseAccess(next: 'accounts') {
  accessTab.value = next
  accessScope.value = 'platform'
  await chooseSection('access')
}
async function chooseBrandAccounts() {
  accessTab.value = 'accounts'
  accessScope.value = 'brand'
  await chooseSection('access')
}
async function chooseCommission(next: 'agents' | 'cycles' | 'payments') {
  commissionTab.value = next
  commissionExpanded.value = true
  await chooseSection('commission')
}
async function toggleCommission() {
  if (accountWriteLocked.value) return
  commissionExpanded.value = !commissionExpanded.value
}
async function toggleOperations() {
  if (accountWriteLocked.value) return
  operationsExpanded.value = !operationsExpanded.value
}
async function toggleWallets() {
  if (accountWriteLocked.value) return
  walletsExpanded.value = !walletsExpanded.value
}
async function chooseWalletView(next: typeof walletView.value) {
  if (accountWriteLocked.value) return
  walletsExpanded.value = true
  section.value = 'wallets'
  walletView.value = next
  memberFinance.value = null
  error.value = ''; notice.value = ''
  if (!selectedBrand.value) loading.value = false
}
async function chooseBrand(id: string) {
  if (accountWriteLocked.value) return
  memberFinance.value = null
  resetMemberFilters()
  selectedBrand.value = id; members.value = []; [auditRows.value,auditTotal.value]=[[],'0']; error.value = ''; notice.value = ''
  memberRequestGeneration++; auditRequestGeneration++
  memberOffset.value = 0; auditOffset.value = 0; memberTotal.value = '0'; moreAudit.value = false
  if (!id) { loading.value = false; return }
  if (section.value === 'users') await loadMembers(id)
  if (section.value === 'audit' && auditView.value === 'audit') await loadAudit(id)
}
async function chooseAuditView(next: typeof auditView.value) {
  if (accountWriteLocked.value || ['maintenance', 'deliveries'].includes(next) && !account.value?.super_admin) return
  memberFinance.value = null
  operationsExpanded.value = true
  section.value = 'audit'
  auditView.value = next; auditRequestGeneration++; [auditRows.value,auditTotal.value]=[[],'0']
  auditOffset.value = 0; moreAudit.value = false; loading.value = false; error.value = ''
  if (next === 'audit' && selectedBrand.value) await loadAudit(selectedBrand.value)
}
async function loadMembers(brandId: string, offset = 0) {
  const generation = ++memberRequestGeneration
  loading.value = true; error.value = ''
  try {
    const result = await api.users(brandId, memberPageSize.value, offset, '', { query: memberQuery.value, online: memberOnlineQuery.value })
    if (generation === memberRequestGeneration && selectedBrand.value === brandId && section.value === 'users') {
      members.value = result.items
      memberOffset.value = offset
      memberTotal.value = result.total_count
    }
  } catch (cause) {
    if (cause instanceof PlatformApiError && cause.status === 401) handleFailure(cause)
    else if (generation === memberRequestGeneration && selectedBrand.value === brandId && section.value === 'users') handleFailure(cause)
  }
  finally { if (generation === memberRequestGeneration) loading.value = false }
}
function batchMemberDeleted(member: PlatformMember) {
  members.value = members.value.filter(item => item.id !== member.id)
  memberTotal.value = String(BigInt(memberTotal.value) - 1n)
}
async function refreshBatchMembers() {
  const offset = BigInt(memberOffset.value) >= BigInt(memberTotal.value) ? Math.max(0, memberOffset.value - memberPageSize.value) : memberOffset.value
  await loadMembers(selectedBrand.value, offset)
}
async function loadAudit(brandId: string, offset = 0) {
  const generation = ++auditRequestGeneration
  loading.value = true; error.value = ''
  try {
    const result = await api.audit(brandId, auditPageSize.value, offset, auditFilters.value)
    if (generation === auditRequestGeneration && selectedBrand.value === brandId && section.value === 'audit' && auditView.value === 'audit') {
      [auditRows.value, auditTotal.value] = [result.items, readPaginationTotal(result)]
      auditOffset.value = offset
      moreAudit.value = BigInt(offset + result.items.length) < BigInt(auditTotal.value)
    }
  } catch (cause) {
    if (cause instanceof PlatformApiError && cause.status === 401) handleFailure(cause)
    else if (generation === auditRequestGeneration && selectedBrand.value === brandId && section.value === 'audit' && auditView.value === 'audit') handleFailure(cause)
  }
  finally { if (generation === auditRequestGeneration) loading.value = false }
}
function statusLabel(status: string) {
  const keys: Record<string, keyof PlatformCopy> = { normal: 'statusNormal', frozen: 'statusFrozen', disabled: 'statusDisabled', expired: 'statusExpired', cancelled: 'statusCancelled', paused: 'statusPaused', active: 'statusActive' }
  const key = keys[status]
  return key ? lang.value[key] : status
}
async function openBrandMembers(brandId: string) { section.value = 'users'; await chooseBrand(brandId) }
async function openBrandAgents(brandId: string) {
  if (accountWriteLocked.value) return
  await chooseBrand(brandId)
  await chooseCommission('agents')
}
async function openBrandAccounts(brandId: string) { accessScope.value = 'brand'; await chooseSection('access'); await chooseBrand(brandId) }
function openBrandOperation(brandId: string) {
  notice.value = ''
  brandOperationPanel.value?.openForBrand(brandId)
}
async function refreshAfterBrandOperation(record: BrandOperation) {
  try {
    await loadWorkspace()
    notice.value = message('common.statusConfirmed', { name: record.name })
  } catch {
    brands.value = []
    members.value = [];
    [auditRows.value,auditTotal.value]=[[],'0']
    selectedBrand.value = ''
    memberRequestGeneration++
    auditRequestGeneration++
    notice.value = message('common.statusChanged')
  }
}
function openWallet(member: PlatformMember, view: 'wallet' | 'ledger') {
  if (!canViewMemberPoints.value) return
  memberFinance.value = { member, view }
}
async function closeMemberFinance() {
  const memberId = memberFinance.value?.member.id
  memberFinance.value = null
  await nextTick()
  document.querySelector<HTMLButtonElement>(`button[aria-controls="platform-member-actions-${memberId}-table"]`)?.focus({ preventScroll: true })
}
function beginCreate() {
  if (!canCreateBrand.value) return
  if (retryPayload.value) { confirming.value = true; modal.value = true; return }
  form.value = { code: '', name: '', default_locale: 'en', timezone: 'UTC' }; confirming.value = false; modal.value = true
}
function reviewCreate() { if (!canCreateBrand.value) return; error.value = ''; confirming.value = true }
function beginDelete(brand: PlatformBrand) {
  deleteBrandTarget.value = brand
  deleteError.value = ''
}
function beginMemberDelete(member: PlatformMember) {
  if (!canDeleteMember.value || deleteMemberBusy.value || deleteMemberUnknown.value) return
  deleteMemberTarget.value = member
  deleteMemberError.value = ''
  deleteMemberIntent.value = null
}
async function submitMemberDelete() {
  const member = deleteMemberTarget.value
  if (!member || !canDeleteMember.value || deleteMemberBusy.value) return
  if (!deleteMemberIntent.value) deleteMemberIntent.value = { id: member.global_user_id, brand: member.brand_id, key: crypto.randomUUID() }
  const intent = deleteMemberIntent.value, brand = selectedBrand.value
  deleteMemberBusy.value = true; deleteMemberError.value = ''; error.value = ''
  try {
    const deleted = await api.deleteMemberAccount(intent.id, intent.brand, intent.key)
    if (deleteMemberIntent.value !== intent) return
    deleteMemberTarget.value = null; deleteMemberIntent.value = null; deleteMemberUnknown.value = false
    notice.value = message('common.memberDeleted', { name: deleted.account_name })
    members.value = members.value.filter(item => item.global_user_id !== intent.id)
    if (selectedBrand.value === brand && section.value === 'users') await loadMembers(brand, memberOffset.value)
  } catch (cause) {
    if (deleteMemberIntent.value !== intent) return
    deleteMemberUnknown.value = cause instanceof PlatformApiError && (cause.network || cause.status >= 500 || cause.code === 'INVALID_RESPONSE')
    if (!deleteMemberUnknown.value) deleteMemberIntent.value = null
    deleteMemberError.value = platformErrorMessage(cause, locale.value)
  } finally { if (deleteMemberIntent.value === intent || !deleteMemberIntent.value) deleteMemberBusy.value = false }
}
async function submitDelete() {
  const brand = deleteBrandTarget.value
  if (!brand) return
  deleteBusy.value = true; deleteError.value = ''; error.value = ''
  try {
    await api.deleteBrand(brand.id, crypto.randomUUID())
    deleteBrandTarget.value = null; notice.value = message('platform.copy.e188a90c')
    if (selectedBrand.value === brand.id) { selectedBrand.value = ''; members.value = []; [auditRows.value,auditTotal.value]=[[],'0'] }
    await loadWorkspace()
  } catch (cause) {
    deleteError.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
  } finally { deleteBusy.value = false }
}
async function submitCreate() {
  if (!canCreateBrand.value || busy.value) return
  if (!retryPayload.value) retryPayload.value = freezeBrandCreateRequest(form.value)
  const pending = retryPayload.value
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api.createBrand(pending.body, pending.key)
    brandQuery.value = ''; brandStatus.value = ''; appliedBrandQuery.value = ''; appliedBrandStatus.value = ''; brandPage.value = 1
    modal.value = false; confirming.value = false; retryPayload.value = null; notice.value = message('platform.copy.bb362b87')
    await loadWorkspace()
  } catch (cause) {
    if (cause instanceof PlatformApiError && (cause.network || cause.status >= 500 || cause.code === 'INVALID_RESPONSE')) { errorCause.value = cause; uncertainError.value = true }
    else { handleFailure(cause); retryPayload.value = null }
  } finally { busy.value = false }
}
async function initialize() {
  authReady.value = false
  try {
    platformPresentation.value = await api.presentation()
    await loadWorkspace()
  } catch (cause) {
    handleFailure(cause)
    // A stale cookie on entry means sign-in is needed, not an interrupted active session.
    if (cause instanceof PlatformApiError && cause.status === 401 && (cause.code === 'AUTH_SESSION_REVOKED' || cause.code === 'AUTH_SESSION_REPLACED')) {
      clearPrivateState()
      error.value = ''
    }
  } finally { authReady.value = true }
}
onMounted(() => withGlobalLoading(initialize))
useOperationToast(notice, value => displayPublicError(value), 'success');
useOperationToast(error, value => value, 'error');

useOperationToast(passwordError, value => displayPublicError(value), 'error');
useOperationToast(deleteError, value => displayPublicError(value), 'error');
useOperationToast(deleteMemberError, value => displayPublicError(value), 'error');
</script>

<template>
  <GlobalLoadingOverlay :label="lang.loading" :hidden="accountWriteLocked" />
  <main v-if="!authReady" class="loading-shell" aria-busy="true"></main>
  <LoginEntry v-else-if="!account" class="platform-login" card-class="login-card" :brand-name="platformPresentation?.display_name ?? ''" :logo-text="platformPresentation?.logo_text" :logo-url="platformPresentation?.logo_url" :title="lang.signIn" title-id="platform-login-title" :locale="locale" :language-label="t('common.language')" :languages="(['en','zh-CN'] as const).map(code=>({code,label:nativeLocaleName(code)}))" @update:locale="setLocale">
      <template v-if="!platformPresentation"><p class="message error" role="alert">{{ t('platform.login.presentationUnavailable') }}</p><button class="button button-secondary" @click="withGlobalLoading(initialize)">{{ t('platform.login.retry') }}</button></template>
      <form v-write-guard="busy" v-else class="login-entry__form" novalidate v-inline-validation @submit.prevent="withGlobalLoading(login)">
        <LoginCredentials v-model:identifier="identifier" v-model:password="password" :identifier-label="lang.identifier" :password-label="lang.password" />
        <p v-if="sessionReplaced" class="message error" role="alert">{{ t('auth.sessionReplaced') }}</p>
        <button class="button button-primary full" :disabled="busy">{{ busy ? lang.loading : lang.signIn }}</button>
      </form>
  </LoginEntry>
  <div v-else class="app-frame management-console" :class="{ 'mobile-navigation-open': mobileNavigationOpen }" @keydown.esc="mobileNavigationOpen = false">
    <button v-if="mobileNavigationOpen" type="button" class="platform-nav-scrim" :aria-label="lang.close" @click="mobileNavigationOpen = false" />
    <aside id="platform-navigation" class="sidebar" @click="closeMobileNavigation">
      <div class="logo-row"><img v-if="platformSettings?.logo_url" class="brand-logo-image" :src="platformSettings.logo_url" alt="" /><div v-else class="brand-mark small">{{ platformSettings?.logo_text }}</div><div><strong>{{ platformSettings?.display_name }}</strong></div></div>
      <button type="button" :class="['nav-item', { selected: section === 'home' }]" @click="withGlobalLoading(chooseSection, 'home')"><span><SiteIcon name="home" /></span>{{ t('overview.home') }}</button>
      <button type="button" :class="['nav-item', { selected: brandSectionActive }]" :aria-expanded="brandsExpanded" aria-controls="platform-brand-tabs" @click="!accountWriteLocked && (brandsExpanded = !brandsExpanded)"><span><SiteIcon name="brands" /></span>{{ t('common.brands') }}<SiteIcon class="console-nav-chevron" name="down" /></button>
      <div v-if="brandsExpanded" id="platform-brand-tabs" class="nav-submenu" data-testid="platform-brand-tabs" role="group" :aria-label="t('common.brands')">
        <button type="button" class="nav-subitem" :class="{ selected: section === 'brands' }" @click="withGlobalLoading(chooseSection, 'brands')">{{ lang.brands }}</button>
        <button type="button" class="nav-subitem" :class="{ selected: section === 'access' && accessScope === 'brand' }" @click="withGlobalLoading(chooseBrandAccounts)">{{ lang.brandAccounts }}</button>
        <button type="button" class="nav-subitem" :class="{ selected: section === 'users' }" @click="withGlobalLoading(chooseSection, 'users')">{{ lang.users }}</button>
      </div>
      <button type="button" :class="['nav-item', { selected: section === 'audit' }]" :aria-expanded="operationsExpanded" @click="withGlobalLoading(toggleOperations)"><span><SiteIcon name="more" /></span>{{ operationsLabel }}<SiteIcon class="console-nav-chevron" name="down" /></button>
      <div v-if="operationsExpanded" class="nav-submenu" data-testid="platform-operations-tabs" role="group" :aria-label="operationsLabel">
        <button class="nav-subitem" :class="{ selected: auditView === 'audit' }" type="button" @click="withGlobalLoading(chooseAuditView, 'audit')">{{ lang.audit }}</button>
        <button v-if="account.platform_permissions.includes('platform_settings.view.platform')" class="nav-subitem" :class="{ selected: auditView === 'archives' }" type="button" @click="withGlobalLoading(chooseAuditView, 'archives')">{{ t('retention.title') }}</button>
        <button v-if="account.platform_permissions.includes('announcement.view.platform')" class="nav-subitem" :class="{ selected: auditView === 'announcements' }" type="button" @click="withGlobalLoading(chooseAuditView, 'announcements')">{{t('announcements.title')}}</button>
        <button class="nav-subitem" :class="{ selected: auditView === 'notifications' }" type="button" @click="withGlobalLoading(chooseAuditView, 'notifications')">{{ t('maintenance.notificationTemplates') }}</button>
      </div>
      <button :class="['nav-item', { selected: section === 'rewards' }]" @click="withGlobalLoading(chooseSection, 'rewards')"><span><SiteIcon name="gift" /></span>{{ lang.rewards }}</button>
      <button type="button" :class="['nav-item', { selected: section === 'wallets' }]" :aria-expanded="walletsExpanded" @click="withGlobalLoading(toggleWallets)"><span><SiteIcon name="wallet" /></span>{{ walletLabel }}<SiteIcon class="console-nav-chevron" name="down" /></button>
      <div v-if="walletsExpanded" class="nav-submenu" data-testid="platform-wallet-tabs" role="group" :aria-label="walletLabel">
        <button class="nav-subitem" :class="{ selected: walletView === 'recharges' }" type="button" @click="withGlobalLoading(chooseWalletView, 'recharges')">{{ t('platform.copy.20be6d4c') }}</button>
        <button v-if="canViewRechargeQuota" class="nav-subitem" :class="{ selected: walletView === 'quota' }" type="button" @click="withGlobalLoading(chooseWalletView, 'quota')">{{ t('platform.quota.management') }}</button>
        <button class="nav-subitem" :class="{ selected: walletView === 'reports' }" type="button" @click="withGlobalLoading(chooseWalletView, 'reports')">{{ t('platform.copy.67516591') }}</button>
      </div>
      <button :class="['nav-item', { selected: section === 'bets' }]" @click="withGlobalLoading(chooseSection, 'bets')"><span><SiteIcon name="ticket" /></span>{{ betLabel }}</button>
      <button :class="['nav-item', { selected: section === 'games' }]" @click="withGlobalLoading(chooseSection, 'games')"><span><SiteIcon name="rules" /></span>{{ gamesLabel }}</button>
      <button :class="['nav-item', { selected: section === 'draws' }]" @click="withGlobalLoading(chooseSection, 'draws')"><span><SiteIcon name="calendar" /></span>{{ drawsLabel }}</button>
      <button :class="['nav-item', { selected: section === 'withdrawals' }]" @click="withGlobalLoading(chooseSection, 'withdrawals')"><span><SiteIcon name="external" /></span>{{ withdrawalLabel }}</button>
      <button type="button" :class="['nav-item', { selected: section === 'commission' }]" :aria-expanded="commissionExpanded" @click="withGlobalLoading(toggleCommission)"><span><SiteIcon name="trend" /></span>{{ commissionLabel }}<SiteIcon class="console-nav-chevron" name="down" /></button>
      <div v-if="commissionExpanded" class="nav-submenu" data-testid="platform-commission-secondary-nav" role="group" :aria-label="commissionLabel">
        <button class="nav-subitem" :class="{ selected: section === 'commission' && commissionTab === 'agents' }" type="button" @click="withGlobalLoading(chooseCommission, 'agents')">{{ agentsLabel }}</button>
        <button class="nav-subitem" :class="{ selected: section === 'commission' && commissionTab === 'cycles' }" type="button" @click="withGlobalLoading(chooseCommission, 'cycles')">{{ t('platform.copy.39ac5c23') }}</button>
        <button class="nav-subitem" :class="{ selected: section === 'commission' && commissionTab === 'payments' }" type="button" @click="withGlobalLoading(chooseCommission, 'payments')">{{ t('platform.copy.48f3ae6f') }}</button>
      </div>
      <button :class="['nav-item', { selected: section === 'access' && accessScope === 'platform' }]" @click="withGlobalLoading(chooseAccess, 'accounts')"><span><SiteIcon name="shield" /></span>{{ accountsLabel }}</button>
      <button :class="['nav-item', { selected: section === 'settings' }]" @click="withGlobalLoading(chooseSection, 'settings')"><span><SiteIcon name="settings" /></span>{{ lang.settings }}</button>
      <div class="sidebar-bottom"></div>
    </aside>
    <nav class="platform-mobile-nav" :aria-label="t('common.navigation')">
      <button type="button" :class="{ selected: section === 'home' }" @click="withGlobalLoading(async () => { mobileNavigationOpen = false; await chooseSection('home') })"><SiteIcon name="home" /><span>{{ t('overview.home') }}</span></button>
      <button type="button" :class="{ selected: section === 'users' }" @click="withGlobalLoading(async () => { mobileNavigationOpen = false; await chooseSection('users') })"><SiteIcon name="users" /><span>{{ lang.users }}</span></button>
      <button type="button" :class="{ selected: section === 'bets' }" @click="withGlobalLoading(async () => { mobileNavigationOpen = false; await chooseSection('bets') })"><SiteIcon name="ticket" /><span>{{ betLabel }}</span></button>
      <button type="button" :class="{ selected: mobileNavigationOpen }" :aria-expanded="mobileNavigationOpen" aria-controls="platform-navigation" @click="mobileNavigationOpen = !mobileNavigationOpen"><SiteIcon :name="mobileNavigationOpen ? 'close' : 'menu'" /><span>{{ t('common.menu') }}</span></button>
    </nav>
    <section class="main-column">
      <header class="topbar platform-topbar">
        <nav class="breadcrumb" aria-label="Breadcrumb"><button type="button" @click="withGlobalLoading(chooseSection, 'home')">{{ platformSettings?.display_name }}</button><span>/</span><button type="button" @click="withGlobalLoading(chooseSection, section)">{{ sectionLabel }}</button></nav>
        <label v-if="showBrandPicker" class="topbar-brand-picker">
          <SelectControl :disabled="accountWriteLocked" :model-value="selectedBrand" :label="lang.brandLabel" @change="withGlobalLoading(chooseBrand, $event)" :options="[{ value: '', label: section==='audit'&&auditView==='archives'?t('retention.allBrands'):('\u2014 ' + String(lang.selectBrand) + ' \u2014').trim() }, ...((brands).map((brand) => ({ value: brand.id, label: (String(brand.name)).trim() })))]" />
        </label>
        <div class="top-actions">
          <ThemeSwitcher :locale="locale" /><LanguageSwitcher :model-value="locale" :label="t('common.language')" :options="[{code:'zh-CN',label:nativeLocaleName('zh-CN')},{code:'en',label:nativeLocaleName('en')}]" @update:model-value="setLocale" />
          <AccountMenu :username="accountName"><button v-if="account.super_admin" type="button" role="menuitem" :disabled="accountWriteLocked" @click="withGlobalLoading(chooseAuditView, 'maintenance')">{{ t('maintenance.tools') }}</button><button type="button" role="menuitem" @click="accountProfileOpen = true">{{ t('common.profile') }}</button><button type="button" role="menuitem" @click="changePasswordOpen = true">{{ t('common.changePassword') }}</button><button v-write-guard="busy" type="button" role="menuitem" class="popover-logout" :disabled="busy" @click="withGlobalLoading(signOut)">{{ lang.logout }}</button></AccountMenu>
        </div>
      </header>
      <main class="content">
        <OverviewPanel v-if="section === 'home' && platformSettings" :key="account.id" :can-view="account.platform_permissions.includes('overview.view.platform')" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" @brands="withGlobalLoading(chooseSection, 'brands')" @open-withdrawals="withGlobalLoading(chooseSection, 'withdrawals', 'reviewing')" />
        <template v-else-if="section === 'brands'">
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ lang.brands }}</h1></div>
          <ManagementList class="platform-brand-list" data-testid="platform-brand-list" :busy="loading || brandListLoading">
            <template v-if="canCreateBrand" #actions><button class="button button-primary" @click="beginCreate"><SiteIcon name="plus" /> {{ lang.createBrand }}</button></template>
            <template #filters>
            <form novalidate v-inline-validation class="wallet-query ui-filter-bar" @submit.prevent="withGlobalLoading(queryBrands)">
              <label class="query-field">
                <div class="ui-filter-input">
                  <ClearableInput :locale="locale" v-model="brandQuery" :aria-label="directoryCopy.search" :placeholder="directoryCopy.hint" />
                </div>
              </label>
              <label class="query-field query-field-status">
                <SelectControl :placeholder="directoryCopy.status" v-model="brandStatus" :label="directoryCopy.status" :options="[{ value: '', label: (String(directoryCopy.all)).trim() }, { value: 'active', label: (String(lang.statusActive)).trim() }, { value: 'paused', label: (String(lang.statusPaused)).trim() }, { value: 'disabled', label: (String(t('brandOperation.stopped'))).trim() }]" />
              </label>

              <button class="button button-primary query-submit" type="submit" :disabled="loading || brandListLoading">{{ directoryCopy.submit }}</button>

            </form></template>
            <BatchDelete subject="brands" :entry-label="brand => brand.name" :scope-key="JSON.stringify([brandPage, appliedBrandQuery, appliedBrandStatus])" :entries="brandEntries" :enabled="canDeleteBrand" :locale="locale" :loading="loading || brandListLoading" :warning="lang.deleteWarning" :delete-entry="(brand, key) => api.deleteBrand(brand.id, key)" :error-text="cause => displayPublicError(platformErrorMessage(cause, locale))" :is-unknown="cause => !(cause instanceof PlatformApiError) || cause.network || cause.status >= 500 || cause.code === 'INVALID_RESPONSE'" @deleted="batchBrandDeleted" @finished="withGlobalLoading(loadWorkspace)" v-slot="{ selected, toggle, selectLabel, locked }">
            <div class="table-wrap"><table class="console-table console-responsive-table platform-brand-table"><thead><tr><th>{{ lang.name }}</th><th><BusinessDetails :value="lang.status" :locale="locale" /></th><th>{{t('platform.brand.memberCount')}}</th><th>{{t('platform.brand.agentCount')}}</th><th>{{t('platform.brand.availableQuota')}}</th><th>{{t('platform.brand.created')}}</th><th>{{t('common.actions')}}</th></tr></thead><tbody>
              <tr v-for="brand in brandEntries" :key="brand.id"><td :data-label="lang.name"><span class="member-account-select"><label v-if="canDeleteBrand" class="member-select-control"><input type="checkbox" :checked="selected.includes(brand.id)" :aria-label="selectLabel(brand.name)" :disabled="locked" @change="toggle(brand.id)" /></label><strong>{{ brand.name }}</strong></span></td><td :data-label="lang.status"><span :class="['status-pill', brand.status]">{{ brand.status === 'disabled' ? t('brandOperation.stopped') : statusLabel(brand.status) }}</span></td>
              <td :data-label="t('platform.brand.memberCount')"><button type="button" class="text-button" :aria-label="t('platform.brand.openMembers', {brand:brand.name,count:new Intl.NumberFormat(locale).format(BigInt(brand.member_count))})" @click="withGlobalLoading(openBrandMembers, brand.id)">{{new Intl.NumberFormat(locale).format(BigInt(brand.member_count))}}</button></td>
              <td :data-label="t('platform.brand.agentCount')"><button type="button" class="text-button" :aria-label="t('platform.brand.openAgents', {brand:brand.name,count:new Intl.NumberFormat(locale).format(BigInt(brand.agent_count))})" @click="withGlobalLoading(openBrandAgents, brand.id)">{{new Intl.NumberFormat(locale).format(BigInt(brand.agent_count))}}</button></td>
              <td :data-label="t('platform.brand.availableQuota')"><button v-if="brand.recharge_quota_available !== null" type="button" class="text-button" :disabled="!canViewRechargeQuota" :aria-label="t('platform.brand.openQuota', {brand:brand.name})" @click="quotaBrand = brand">{{formatPoints(brand.recharge_quota_available,locale)}}</button><span v-else>{{t('platform.brand.quotaRestricted')}}</span></td>
              <td :data-label="t('platform.brand.created')">{{formatPlatformDateTime(brand.created_at,displaySettings,locale)}}</td><td :data-label="t('common.actions')">
                <MemberActionsMenu :id="`brand-actions-${brand.id}`" :label="t('platform.brand.actions', {brand:brand.name})">
                  <button type="button" role="menuitem" @click="withGlobalLoading(openBrandMembers, brand.id)">{{lang.membersAction}}</button>
                  <button type="button" role="menuitem" @click="withGlobalLoading(openBrandAccounts, brand.id)">{{lang.accountsAction}}</button>
                  <button v-if="account.platform_permissions.includes('brand_domains.view.platform')" type="button" role="menuitem" @click="domainBrand = brand">{{t('platform.brand.domains')}}</button>
                  <button v-if="canViewRechargeQuota" type="button" role="menuitem" @click="quotaBrand = brand">{{t('platform.brand.quota')}}</button>
                  <button v-if="canManageBrandOperation" type="button" role="menuitem" @click="openBrandOperation(brand.id)">{{lang.statusAction}}</button>
                  <button v-if="canDeleteBrand" type="button" role="menuitem" class="danger-text" @click="beginDelete(brand)">{{lang.deleteAction}}</button>
                </MemberActionsMenu>
              </td></tr>
              <tr v-if="!loading && !brandListLoading && !error && !brands.length"><td colspan="7" class="empty-state">{{ t('state.emptyBrands') }}</td></tr>
              <tr v-else-if="!loading && !brandListLoading && !error && !filteredBrands.length"><td colspan="7" class="empty-state" role="status">{{ directoryCopy.empty }}</td></tr>
            </tbody></table></div>
          </BatchDelete>
          <template #pagination><TablePagination :total="String(filteredBrands.length)" :page="brandPage" :count="brandEntries.length" :page-size="brandPageSize" :previous-disabled="loading || brandListLoading || brandPage === 1" :next-disabled="loading || brandListLoading || brandPage * brandPageSize >= filteredBrands.length" :go-to="pageBrands" :locale="locale" :aria-label="t('common.brandTableSummary')" :change-page-size="async (size: number) => { brandPageSize = size; brandPage = 1; await pageBrands(1) }" :busy="loading || brandListLoading" /></template></ManagementList>
        </template>
        <template v-else-if="section === 'access'">
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ accessLabel }}</h1></div>
          <AccessPanel v-if="accessScope === 'platform' || selectedBrand" :brand-id="selectedBrand" :locale="locale" :scope="accessScope" :account-id="account.id" :grants="account.platform_permissions" :initial-tab="accessTab" @locked="accountWriteLocked = $event" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'users'">
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ lang.users }}</h1></div>
          <ManagementList v-if="selectedBrand" :busy="loading">
          <template #filters>
            <DirectorySearch :locale="locale"  v-model="memberSearch" :label="t('members.search')" :query-label="t('members.query')" :busy="loading" :search="searchMembers"><SelectControl v-model="memberOnlineFilter" :label="t('members.onlineStatus')" :options="[{ value: '', label: (String(t('members.allOnline'))).trim() }, { value: 'true', label: (String(t('members.online'))).trim() }, { value: 'false', label: (String(t('members.offline'))).trim() }]" /></DirectorySearch></template>
            <BatchDelete subject="accounts" :entry-label="member => member.username || member.phone" :scope-key="String(memberOffset) + JSON.stringify(memberSearch)" :key="account.id + ':' + selectedBrand" :entries="members" :enabled="canDeleteMember" :locale="locale" :loading="loading" :delete-entry="(member, key) => api.deleteMemberAccount(member.global_user_id, member.brand_id, key)" :error-text="cause => displayPublicError(platformErrorMessage(cause, locale))" :is-unknown="cause => !(cause instanceof PlatformApiError) || cause.network || cause.status >= 500 || cause.code === 'INVALID_RESPONSE'" @deleted="batchMemberDeleted" @finished="withGlobalLoading(refreshBatchMembers)" v-slot="{ selected, toggle, selectLabel, locked }">
            <ManagementTable :entries="members" :columns="memberColumns" :empty="!loading && !error && !members.length" :empty-label="t('members.empty')" table-class="platform-member-table">
              <template #account="{entry:member}"><span class="member-account-select"><label v-if="canDeleteMember" class="member-select-control"><input type="checkbox" :checked="selected.includes(member.id)" :aria-label="selectLabel(member.username || member.phone)" :disabled="locked" @change="toggle(member.id)" /></label><button type="button" class="text-button member-account" @click="memberDetail = member">{{ member.username || member.phone || t('association.unnamed') }}</button></span></template>
              <template #online="{entry:member}"><span class="member-presence" :class="{ 'is-online': member.online }"><i aria-hidden="true" />{{ t(member.online ? 'members.online' : 'members.offline') }}</span></template>
              <template #points="{entry:member}">{{ member.display_points === null ? t('members.pointsRestricted') : formatPoints(member.display_points,locale) }}</template>
              <template #phone="{entry:member}">{{ member.phone || '—' }}</template>
              <template #status="{entry:member}"><span :class="['status-pill', member.status]">{{ statusLabel(member.status) }}</span></template>
              <template #joined="{entry:member}">{{ formatPlatformDateTime(member.joined_at, displaySettings, locale) }}</template>
              <template #actions="{entry:member}"><MemberActionsMenu :id="`platform-member-actions-${member.id}-table`" :label="t('members.actions', { account: member.username || member.phone })"><button type="button" role="menuitem" @click="memberDetail = member">{{ t('members.details') }}</button><button v-if="canViewMemberPoints" type="button" role="menuitem" @click="openWallet(member, 'wallet')">{{ t('common.memberPoints') }}</button><button v-if="canViewMemberPoints" type="button" role="menuitem" @click="openWallet(member, 'ledger')">{{ t('members.ledger') }}</button><button v-if="canDeleteMember" type="button" role="menuitem" class="danger-text" @click="beginMemberDelete(member)">{{ lang.deleteMember }}</button></MemberActionsMenu></template>
            </ManagementTable>


            </BatchDelete>
          <template #pagination><TablePagination data-testid="platform-member-pages" :total="memberTotal" :page="Math.floor(memberOffset / memberPageSize + 1)" :count="members.length" :locale="locale" :page-size="memberPageSize" :previous-disabled="loading || memberOffset === 0" :next-disabled="loading || !moreMembers" :previous-label="previousPage" :next-label="nextPage" :go-to="(target: number) => loadMembers(selectedBrand, (target - 1) * memberPageSize)" :change-page-size="async (size: number) => { memberPageSize = size; await loadMembers(selectedBrand, 0) }" :busy="loading" /></template></ManagementList>
          <DetailModal v-if="memberDetail" :title="t('members.details')" :close-label="lang.close" @close="memberDetail = null"><dl class="member-details"><div><dt>{{ lang.username }}</dt><dd>{{ memberDetail.username || memberDetail.phone || t('association.unnamed') }}</dd></div><div><dt>{{ lang.displayName }}</dt><dd>{{ memberDetail.display_name || '—' }}</dd></div><div><dt>{{ t('members.onlineStatus') }}</dt><dd>{{ t(memberDetail.online ? 'members.online' : 'members.offline') }}</dd></div><div><dt>{{ pointsAction }}</dt><dd>{{ memberDetail.display_points === null ? t('members.pointsRestricted') : memberDetail.display_points }}</dd></div><div><dt>{{ lang.phone }}</dt><dd>{{ memberDetail.phone || '—' }}</dd></div><div><dt><BusinessDetails :value="lang.status" :locale="locale" /></dt><dd>{{ statusLabel(memberDetail.status) }}</dd></div><div><dt>{{ t('members.joined') }}</dt><dd>{{ formatPlatformDateTime(memberDetail.joined_at, displaySettings, locale) }}</dd></div><div><dt>{{ t('members.notes') }}</dt><dd>{{ memberDetail.notes || '—' }}</dd></div><div><dt>{{ t('members.tags') }}</dt><dd>{{ memberDetail.tags.join(' · ') || '—' }}</dd></div></dl></DetailModal>
        </template>
        <template v-else-if="section === 'audit'">
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ operationLabel }}</h1></div>
          <section v-if="auditView === 'maintenance' && account.super_admin" data-testid="platform-maintenance-tools" class="panel console-panel console-stack">
            <div class="form-actions ui-form-actions">
              <button class="button button-secondary" type="button" @click="withGlobalLoading(chooseAuditView, 'deliveries')">{{t('platform.copy.42e5c206')}}</button>
            </div>
          </section>
          <DataRetentionPanel @view-change="retentionPolicyVisible=$event==='policy'" v-else-if="auditView === 'archives'" :brand-id="selectedBrand" :account-id="account.id" :can-manage="account.platform_permissions.includes('platform_settings.write.platform')" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
          <AnnouncementsManagement v-else-if="auditView === 'announcements'" platform base-path="/api/v1/platform" :key="`announcements:${account.id}`" :actor-id="account.id" :locale="locale" :permissions="account.platform_permissions" @failure="handleFailure" />
          <NotificationsPanel v-else-if="auditView === 'notifications' || (auditView === 'deliveries' && selectedBrand)" :key="account.id + ':' + auditView" :view="auditView === 'deliveries' ? 'deliveries' : 'templates'" :account-id="account.id" :can-manage="account.platform_permissions.includes('notification_template.write.platform')" :brand-id="selectedBrand" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
          <template v-else-if="auditView === 'audit'">
          <ManagementList v-if="selectedBrand" :busy="loading">
          <template #filters><AuditFiltersForm :locale="locale" :busy="loading" @apply="applyAuditFilters" /></template>
          <template v-if="error" #actions><button v-refresh-button="t('common.refresh')" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(loadAudit, selectedBrand)">{{ t('common.refresh') }}</button></template><div class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{ lang.action }}</th><th>{{ lang.actor }}</th><th>{{ lang.resource }}</th><th>{{ t('platform.copy.f3f300a4') }}</th><th>{{ lang.date }}</th></tr></thead><tbody><tr v-for="row in auditRows" :key="row.id"><td :data-label="String(lang.action)"><span class="action-label">{{ businessActionLabel(row.action, locale) }}</span></td><td :data-label="String(lang.actor)">{{ auditActorLabel(row) }}</td><td :data-label="String(lang.resource)">{{ businessResourceLabel(row.resource_type, locale, row.action) }}</td><td :data-label="t('platform.copy.f3f300a4')" class="reason-cell" :title="auditReasonLabel(row.reason, locale)">{{ auditReasonLabel(row.reason, locale) }}</td><td :data-label="String(lang.date)">{{ formatPlatformDateTime(row.created_at, displaySettings, locale) }}</td></tr><tr v-if="!loading && !error && !auditRows.length"><td colspan="5" class="empty-state">{{ t('state.emptyAudit') }}</td></tr></tbody></table></div>

          <template #pagination><TablePagination data-testid="platform-audit-pages" :total="auditTotal" :page="Math.floor(auditOffset / auditPageSize + 1)" :count="auditRows.length" :locale="locale" :page-size="auditPageSize" :previous-disabled="loading || auditOffset === 0" :next-disabled="loading || !moreAudit" :previous-label="previousPage" :next-label="nextPage" :go-to="(target: number) => loadAudit(selectedBrand, (target - 1) * auditPageSize)" :change-page-size="async (size: number) => { auditPageSize = size; await loadAudit(selectedBrand, 0) }" :busy="loading" /></template></ManagementList>
          </template>
        </template>
        <template v-else-if="section === 'rewards'">
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ lang.rewards }}</h1></div>
          <RewardsPanel v-if="selectedBrand" :brand-id="selectedBrand" :brand-name="selected?.name || ''" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'wallets'">
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ walletViewLabel }}</h1></div>
          <CashConsole v-if="selectedBrand && walletView === 'recharges'" :key="`cash:${account.id}:${selectedBrand}`" base-path="/api/v1/platform" platform mode="deposit" :actor-id="account.id" :brand-id="selectedBrand" :locale="locale" :permissions="account.platform_permissions" @failure="handleFailure" />
          <RechargesPanel v-if="selectedBrand && walletView === 'recharges'" :brand-id="selectedBrand" member-id="" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
          <RechargeQuotaPanel v-else-if="selectedBrand && walletView === 'quota' && canViewRechargeQuota" standalone
            :key="`${account.id}:${selectedBrand}`" :brand-id="selectedBrand" :actor-id="account.id" :locale="locale"
            base-path="/api/v1/platform" :can-grant="canGrantRechargeQuota" @updated="updateBrandQuota" @failure="handleFailure" />
          <ReportsPanel v-else-if="selectedBrand && walletView === 'reports'" :brand-id="selectedBrand" member-id="" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'bets'">
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ betLabel }}</h1></div>
          <div v-if="selectedBrand" class="brand-picker"><label><SelectControl v-model="selectedGameId" :label="gamesLabel" :disabled="gamesLoading" :options="[{ value: '', label: (String(t('common.allGames'))).trim() }, ...((brandGames).map((game) => ({ value: game.id, label: (String(game.name)).trim() })))]" /></label></div>
          <BetOrdersPanel v-if="selectedBrand" :brand-id="selectedBrand" :game-id="selectedGameId" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'games' || section === 'draws'">
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ section === 'games' ? gamesLabel : drawsLabel }}</h1></div>

          <PlatformLottery v-if="section === 'games' && account?.platform_permissions.includes('platform_game.view.platform')" surface="platform" brand-id="" :locale="locale" :display-settings="displaySettings" :can-manage="account.platform_permissions.includes('platform_game.write.platform')" :can-connect="false" unified-catalog @failure="handleFailure"><template #calendar><OfficialDrawCalendar :can-manage="account.platform_permissions.includes('platform_game.write.platform')" :display-settings="displaySettings" :locale="locale" @failure="handleFailure" /></template></PlatformLottery>
          <DrawsPanel v-if="section === 'draws'" :locale="locale" :display-settings="displaySettings" :can-view="account.platform_permissions.includes('platform_game.view.platform')" :can-manage="account.platform_permissions.includes('platform_game.write.platform')" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'commission'">
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ commissionPageLabel }}</h1></div>
          <CommissionPanel v-if="selectedBrand" :brand-id="selectedBrand" :locale="locale" :initial-tab="commissionTab" :display-settings="displaySettings" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'settings'">
          <PlatformSettingsPanel v-if="platformSettings" v-model:tab="settingsTab" :account="account" :brand-id="selectedBrand" :brand-status="selected?.status || ''" :initial="platformSettings" :locale="locale" :can-manage="account.platform_permissions.includes('platform_settings.write.platform')" @saved="platformSettings = $event; notice = message('common.settingsSaved')" @failure="handleFailure" />
        </template>
        <template v-else>
          <div class="console-page-heading page-heading compact-heading"><h1 class="console-page-title">{{ withdrawalLabel }}</h1></div>
          <CashConsole v-if="selectedBrand" :key="`cash-withdraw:${account.id}:${selectedBrand}`" base-path="/api/v1/platform" platform mode="withdrawal" :actor-id="account.id" :brand-id="selectedBrand" :locale="locale" :permissions="account.platform_permissions" @failure="handleFailure" />
          <WithdrawalsPanel v-if="selectedBrand" :brand-id="selectedBrand" :locale="locale" :display-settings="displaySettings" :initial-state="withdrawalInitialState || undefined" @failure="handleFailure" />
        </template>
        <p v-if="showBrandPicker && !selectedBrand && !['games', 'draws'].includes(section) && !(section === 'settings') && !(section==='audit'&&auditView==='archives')" class="empty-state" role="status" data-testid="platform-brand-required">{{ t('common.selectBrandFirst') }}</p>
      </main>
      <BrandOperationPanel ref="brandOperationPanel" :locale="locale" :allowed="canManageBrandOperation" :display-settings="displaySettings" @updated="withGlobalLoading(refreshAfterBrandOperation, $event)" @failure="handleFailure" />
    </section>
    <WalletPanel v-if="section === 'users' && memberFinance && selectedBrand && account && canViewMemberPoints"
      :key="`${account.id}:${selectedBrand}:${memberFinance.member.id}:${memberFinance.view}`"
      :brand-id="selectedBrand" :member-id="memberFinance.member.id" :member-name="memberFinance.member.username || memberFinance.member.phone"
      :locale="locale" :display-settings="displaySettings" :view="memberFinance.view" @close="closeMemberFinance" @failure="handleFailure" />
    <DetailModal v-if="domainBrand && account" :title="t('platform.brand.domainsTitle', {brand:domainBrand.name})" :close-label="lang.close" @close="domainBrand = null">
      <BrandDomainsPanel :account="account" :brand="domainBrand" :locale="locale" @session-invalid="clearPrivateState" />
    </DetailModal>
    <DetailModal v-if="quotaBrand && account" :title="t('platform.brand.quotaTitle', {brand:quotaBrand.name})" :close-label="lang.close" @close="quotaBrand = null">
      <RechargeQuotaPanel :key="`${account.id}:${quotaBrand.id}`" :brand-id="quotaBrand.id" :actor-id="account.id" :locale="locale" base-path="/api/v1/platform" :can-grant="canGrantRechargeQuota" @updated="updateBrandQuota" @failure="handleFailure" />
    </DetailModal>
    <AccountProfileDialog v-if="accountProfileOpen" :title="t('common.profile')" :close-label="lang.close" :fields="[{label:lang.username,value:accountName},{label:lang.role,value:lang.platformAdmin}]" @close="accountProfileOpen=false" />
    <ChangePasswordDialog v-if="changePasswordOpen" v-model:current="currentPassword" v-model:next="newPassword" v-model:confirmation="confirmPassword" :title="t('common.changePassword')" :close-label="lang.close" :current-label="t('common.currentPassword')" :new-label="t('common.newPassword')" :confirm-label="t('common.confirmNewPassword')" :submit-label="passwordBusy ? lang.loading : t('common.savePassword')" :busy="passwordBusy" @close="changePasswordOpen=false" :submit="submitPasswordChange"></ChangePasswordDialog>
    <DetailModal v-if="modal" class="confirmation-dialog brand-create-dialog ui-form-dialog" :title="confirming ? lang.confirmation : lang.createBrand" :close-label="lang.close" :close-disabled="busy" @close="!busy && (modal = false)">
      <template v-if="!confirming">
        <form novalidate v-inline-validation class="brand-form ui-form" @submit.prevent="withGlobalLoading(reviewCreate)"><label>{{ lang.code }}<input v-model.trim="form.code" required pattern="[a-z][a-z0-9_]{0,47}" :data-validation-format="formValidationText('codeFormat', {min: 1, max: 48})" :placeholder="lang.codePlaceholder" /></label><label>{{ lang.name }}<input v-model.trim="form.name" required maxlength="120" /></label><div class="form-row"><label>{{ lang.locale }}<SelectControl v-model="form.default_locale" :label="String(lang.locale)" :options="[{ value: 'en', label: (String(lang.localeEnglish)).trim() }, { value: 'zh-CN', label: (String(lang.localeChinese)).trim() }]" /></label><label>{{ lang.timezone }}<input v-model.trim="form.timezone" required :placeholder="lang.timezonePlaceholder" /></label></div><div class="form-actions ui-form-actions"><button class="button button-primary" :disabled="!canCreateBrand">{{ lang.review }} <span>→</span></button></div></form>
      </template><template v-else><dl class="confirm-list"><div><dt>{{ lang.code }}</dt><dd>{{ retryPayload?.body.code || form.code }}</dd></div><div><dt>{{ lang.name }}</dt><dd>{{ retryPayload?.body.name || form.name }}</dd></div><div><dt>{{ lang.locale }}</dt><dd>{{ retryPayload?.body.default_locale || form.default_locale }}</dd></div><div><dt>{{ lang.timezone }}</dt><dd>{{ retryPayload?.body.timezone || form.timezone }}</dd></div></dl><p v-if="retryPayload && error" class="message error">{{ displayPublicError(error) }}</p><div class="form-actions ui-form-actions"><button v-write-guard="busy" class="button button-primary" :disabled="busy || !canCreateBrand" @click="withGlobalLoading(submitCreate)">{{ busy ? lang.loading : retryPayload ? lang.retry : lang.confirm }}</button></div></template>
    </DetailModal>
    <DetailModal v-if="deleteBrandTarget" class="confirmation-dialog brand-delete-dialog" :title="lang.deleteConfirm" :close-label="lang.close" :close-disabled="deleteBusy" @close="!deleteBusy && (deleteBrandTarget = null)"><p class="modal-intro">{{ lang.deleteWarning }}</p><dl class="confirm-list"><div><dt>{{ lang.name }}</dt><dd>{{ deleteBrandTarget.name }}</dd></div><div><dt>{{ lang.code }}</dt><dd>{{ deleteBrandTarget.code }}</dd></div></dl><div class="form-actions ui-form-actions"><button v-write-guard="deleteBusy" class="button button-danger" :disabled="deleteBusy" @click="withGlobalLoading(submitDelete)">{{ deleteBusy ? lang.loading : lang.deleteBrand }}</button></div></DetailModal>
    <DetailModal v-if="deleteMemberTarget" class="confirmation-dialog member-delete-dialog" :title="lang.deleteMember" :close-label="lang.close" :close-disabled="deleteMemberBusy || deleteMemberUnknown" @close="!deleteMemberBusy && !deleteMemberUnknown && (deleteMemberTarget = null)"><dl class="confirm-list"><div><dt>{{ lang.username }}</dt><dd>{{ deleteMemberTarget.username || deleteMemberTarget.phone || t('association.unnamed') }}</dd></div><div><dt>{{ lang.displayName }}</dt><dd>{{ deleteMemberTarget.display_name || '—' }}</dd></div></dl><p v-if="deleteMemberUnknown" class="message" role="status">{{ t('common.uncertainRequest') }}</p><div class="form-actions ui-form-actions"><button v-write-guard="deleteMemberBusy" class="button button-danger" :disabled="deleteMemberBusy" @click="withGlobalLoading(submitMemberDelete)">{{ deleteMemberBusy ? lang.loading : deleteMemberUnknown ? t('common.retryMemberDelete') : lang.deleteMember }}</button></div></DetailModal>
  </div>
<OperationToastHost :locale="locale" />
</template>
