<script setup lang="ts">
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
import MemberActionsMenu from '@lottery/shared/MemberActionsMenu.vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import TablePagination from '@lottery/shared/TablePagination.vue'
import { nativeLocaleName, useSessionReplacement } from '@lottery/shared'
import { computed, onMounted, ref, shallowRef, watch, watchEffect } from 'vue'
import { createPlatformApi, freezeBrandCreateRequest, PlatformApiError, type BrandCreateInput, type FrozenBrandCreateRequest, type PlatformAudit, type PlatformBrand, type PlatformMember, type PlatformAccount, type PlatformSettings, type PlatformPresentation } from './platform-api'
import { useBrandGames } from './use-brand-games'
import { platformErrorMessage } from './platform-error-message'
import { usePlatformI18n } from './i18n'
import RewardsPanel from './RewardsPanel.vue'
import WalletPanel from './WalletPanel.vue'
import BetOrdersPanel from './BetOrdersPanel.vue'
import LotteryPanel from './LotteryPanel.vue'
import PlatformLottery from '@lottery/shared/PlatformLottery.vue'
import WithdrawalsPanel from './WithdrawalsPanel.vue'
import CommissionPanel from './CommissionPanel.vue'
import ArchivesPanel from './ArchivesPanel.vue'
import NotificationsPanel from './NotificationsPanel.vue'
import ArchiveTasksPanel from './ArchiveTasksPanel.vue'
import FinancialPoliciesPanel from './FinancialPoliciesPanel.vue'
import OperationalPoliciesPanel from './OperationalPoliciesPanel.vue'
import AccessPanel from './AccessPanel.vue'
import BrandOperationPanel from './BrandOperationPanel.vue'
import Toast from '@lottery/shared/Toast.vue'
import type { BrandOperation } from './brand-operation-api'
import PlatformSettingsPanel from './PlatformSettingsPanel.vue'
import OverviewPanel from './OverviewPanel.vue'
import SecuritySettingsPanel from './SecuritySettingsPanel.vue'
import { securityIntents } from './security-settings-api'
import { formatPlatformDateTime, type PlatformDisplaySettings } from './platform-format'

const api = createPlatformApi()
const { locale, setLocale: updateLocale, t } = usePlatformI18n()
function setLocale(value: string) { updateLocale(value) }
const authReady = ref(false)
function createLang() {
  return {
  title: t('platform.copy.a220464b'), loginTitle: t('platform.copy.d62bdbd5'), identifier: t('platform.copy.eeaa7275'), password: t('platform.copy.2a771a31'), signIn: t('platform.copy.ec3b65e9'),
  brands: t('platform.copy.79dd4298'), users: t('platform.copy.d84b8968'), brandAccounts: t('platform.copy.9c2e4b71'), audit: t('platform.copy.8262ff2c'), rewards: t('platform.copy.25982fc9'), createBrand: t('platform.copy.767e7f67'), selectBrand: t('platform.copy.1c0a1845'), allBrands: t('platform.copy.0a8ccdef'), brandCount: t('platform.copy.6a2943a0'),
  name: t('platform.copy.33b6c8e3'), code: t('platform.copy.9ba39db0'), status: t('platform.copy.c7260510'), view: t('platform.copy.b9fbecbf'), membersAction: t('platform.copy.c4bf0312'), accountsAction: t('platform.copy.d944b7ec'), statusAction: t('platform.copy.c7260510'), deleteAction: t('platform.copy.9851501a'), deleteBrand: t('platform.copy.15adaf08'), deleteConfirm: t('platform.copy.ac1de974'), deleteWarning: t('platform.copy.133eb17e'), deleteReason: t('platform.copy.351c71c8'), deleteSuccess: t('platform.copy.e188a90c'), username: t('platform.copy.eeaa7275'), role: t('platform.copy.6fc23cbb'), displayName: t('platform.copy.bcd89469'), phone: t('platform.copy.d05d61ad'), memberCount: t('platform.copy.1bd8aaad'), brandTotal: t('platform.copy.fa68aba0'),
  locale: t('platform.copy.43eda349'), timezone: t('platform.copy.c67c04be'), reason: t('platform.copy.f546783b'), cancel: t('platform.copy.2230b1a7'), review: t('platform.copy.9b85d866'), confirm: t('platform.copy.312420fb'), confirmation: t('platform.copy.e204e6ed'), confirmCopy: t('platform.copy.46ea856b'),
  action: t('platform.copy.75b7f6d4'), actor: t('platform.copy.3177f27f'), resource: t('platform.copy.de6bef13'), date: t('platform.copy.7069c6b9'), empty: t('platform.copy.b4600402'), loading: t('platform.copy.fa02e7b1'), logout: t('platform.copy.2315867e'), close: t('platform.copy.27b2fd29'), platformNote: '', platformAdmin: t('platform.copy.45ec43b6'), settings: t('common.platformSettings'), error: t('platform.copy.8b844392'), retry: t('platform.copy.8c0fb218'), unknown: t('platform.copy.e6aabad3'), created: t('platform.copy.bb362b87'), localeEnglish: t('platform.copy.f7f79167'), localeChinese: t('platform.copy.01f1aac2'), directory: t('platform.copy.0ef111d5'), directoryDescription: '', brandDirectory: t('platform.copy.6c32f38f'), brandDirectoryDescription: '', platformDirectory: t('platform.copy.3916c3b1'), createDescription: t('platform.copy.8e3e641a'), codePlaceholder: t('platform.copy.3d397d5f'), timezonePlaceholder: t('platform.copy.990c4cb9'), accountLabel: t('platform.copy.ce2d66f3'), brandLabel: t('platform.copy.d5b68c0f'), statusNormal: t('platform.copy.0e1df295'), statusFrozen: t('platform.copy.d21ee5d7'), statusDisabled: t('platform.copy.6762439b'), statusExpired: t('platform.copy.02dd0e6a'), statusCancelled: t('platform.copy.d9b0562c'), statusPaused: t('platform.copy.3d294087'), statusActive: t('platform.copy.15a92cfa'), countSuffix: t('platform.copy.13d2abd9'), accountProfile: t('platform.copy.9f9d8eb6'), accountPassword: t('platform.copy.3f21ce3e'), accountId: t('platform.copy.f985024d'),
  deleteMember: t('common.deleteMember'), deleteMemberConfirm: t('common.deleteMemberConfirm'), memberDeleted: t('common.memberDeleted'),
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
const brands = ref<PlatformBrand[]>([])
const brandQuery = ref('')
const brandStatus = ref('')
const filteredBrands = computed(() => {
  const query = brandQuery.value.trim().toLowerCase()
  return brands.value.filter(brand => (!brandStatus.value || brand.status === brandStatus.value) &&
    (!query || `${brand.name} ${brand.code}`.toLowerCase().includes(query)))
})
const directoryCopy = computed(() => locale.value === 'en' ? {
  search: t('platform.copy.e572aa2e'), hint: t('platform.copy.607f1d49'), status: t('platform.copy.7ab7ba64'), all: t('platform.copy.69b1cb0f'), clear: t('platform.copy.51de817e'), submit: t('platform.copy.0e708b55'), empty: t('platform.copy.2aaccf31'),
} : {
  search: t('platform.copy.e572aa2e'), hint: t('platform.copy.607f1d49'), status: t('platform.copy.7ab7ba64'), all: t('platform.copy.69b1cb0f'), clear: t('platform.copy.51de817e'), submit: t('platform.copy.0e708b55'), empty: t('platform.copy.2aaccf31'),
})
const members = ref<PlatformMember[]>([])
const auditRows = ref<PlatformAudit[]>([])
const pageSize = 50
const memberOffset = ref(0)
const auditOffset = ref(0)
const memberTotal = ref('0')
const memberSearch = ref(''), memberQuery = ref('')
const memberOnlineFilter = ref<'' | 'true' | 'false'>(''), memberOnlineQuery = ref<'' | 'true' | 'false'>('')
const memberDetail = ref<PlatformMember | null>(null)
const canViewMemberPoints = computed(() => account.value?.platform_permissions.includes('wallet.view.platform') === true)
const moreMembers = computed(() => BigInt(memberOffset.value) + BigInt(members.value.length) < BigInt(memberTotal.value))
function resetMemberFilters() { memberSearch.value = ''; memberQuery.value = ''; memberOnlineFilter.value = ''; memberOnlineQuery.value = ''; memberTotal.value = '0'; memberDetail.value = null }
async function searchMembers() { memberQuery.value = memberSearch.value.trim(); memberOnlineQuery.value = memberOnlineFilter.value; await loadMembers(selectedBrand.value) }
async function clearMemberFilters() { resetMemberFilters(); await searchMembers() }
const moreAudit = ref(false)
const previousPage = computed(() => t('platform.pagination.previous'))
const nextPage = computed(() => t('platform.pagination.next'))
const thisPage = computed(() => t('platform.pagination.current'))
const selectedBrand = ref('')
provideAssociationContext(associationContextKey, computed(() => brands.value.find(item => item.id === selectedBrand.value) ?? account.value));
const brandSelectionKey = 'lottery.platform.selectedBrand'
watch(selectedBrand, brandId => {
  if (brandId && account.value) sessionStorage.setItem(brandSelectionKey, JSON.stringify({ accountId: account.value.id, brandId }))
  else sessionStorage.removeItem(brandSelectionKey)
}, { flush: 'sync' })
const section = ref<'home' | 'brands' | 'users' | 'audit' | 'rewards' | 'wallets' | 'bets' | 'games' | 'draws' | 'withdrawals' | 'commission' | 'access' | 'settings'>('home')
const showBrandPicker = computed(() => section.value === 'access' ? accessScope.value === 'brand' : !['home', 'brands', 'settings'].includes(section.value))
const { games: brandGames, selectedGameId, loading: gamesLoading, refresh: refreshBrandGames } = useBrandGames(selectedBrand, () => ['games', 'draws', 'bets'].includes(section.value) ? section.value : '', handleFailure)
const brandLotteryDetails = ref<InstanceType<typeof LotteryPanel> | null>(null)
function openBrandGameDetail(id: string, view: 'periods' | 'plays' | 'model') {
  const game = brandGames.value.find(row => row.id === id)
  if (!game || !brandLotteryDetails.value) throw new Error('Brand game detail is unavailable')
  brandLotteryDetails.value.openGameDetail(game, view === 'plays' ? 'rules' : view)
}
const auditView = ref<'audit' | 'archives' | 'archive-tasks' | 'financial-policies' | 'operational-policies' | 'security' | 'notifications'>('audit')
const brandsExpanded = ref(false)
const brandSectionActive = computed(() => section.value === 'brands' || section.value === 'users' || (section.value === 'access' && accessScope.value === 'brand'))
const operationsExpanded = ref(false)
const walletView = ref<'wallet' | 'recharges' | 'reports'>('wallet')
const walletsExpanded = ref(false)
const commissionExpanded = ref(false)
const accessTab = ref<'accounts'>('accounts')
const commissionTab = ref<'agents' | 'cycles' | 'payments'>('agents')
const walletMember = ref('')
const walletLabel = computed(() => t('common.memberPoints'))
const pointsAction = computed(() => t('common.points'))
const betLabel = computed(() => t('common.betOrders'))
const gamesLabel = computed(() => t('common.games'))
const drawsLabel = computed(() => t('common.draws'))
const withdrawalLabel = computed(() => t('common.withdrawals'))
const agentsLabel = computed(() => t('common.agents'))
const commissionLabel = computed(() => t('common.commission'))
const commissionPageLabel = computed(() => commissionTab.value === 'agents' ? agentsLabel.value : commissionTab.value === 'cycles' ? t('platform.copy.39ac5c23') : t('platform.copy.48f3ae6f'))
const operationsLabel = computed(() => t('common.operations'))
const operationLabel = computed(() => auditView.value === 'audit' ? lang.value.audit : auditView.value === 'archives' ? t('common.reportArchives') : auditView.value === 'archive-tasks' ? t('common.archiveTasks') : auditView.value === 'financial-policies' ? t('common.financialPolicies') : auditView.value === 'operational-policies' ? t('common.operationalPolicies') : auditView.value === 'security' ? t('security.title') : t('common.notifications'))
const walletViewLabel = computed(() => walletView.value === 'wallet' ? t('platform.copy.c0368f3e') : walletView.value === 'recharges' ? t('platform.copy.20be6d4c') : t('platform.copy.67516591'))
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
const notice = ref('')
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
const deleteMemberIntent = ref<{ id: string; key: string } | null>(null)
const deleteMemberUnknown = ref(false)
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

function clearPrivateState() {
  securityIntents.clearAll()
  section.value = 'home'
  sessionStorage.removeItem(brandSelectionKey)
  accountWriteLocked.value = false
  accessScope.value = 'platform'
  brandsExpanded.value = false
  brandQuery.value = ''; brandStatus.value = ''
  auditView.value = 'audit'
  operationsExpanded.value = false
  walletView.value = 'wallet'
  walletsExpanded.value = false
  walletMember.value = ''
  resetMemberFilters()
  account.value = null; platformSettings.value = null; brands.value = []; members.value = []; auditRows.value = []; selectedBrand.value = ''
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
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) clearPrivateState()
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
    if (selectedBrand.value && !brands.value.some(brand => brand.id === selectedBrand.value)) { selectedBrand.value = ''; members.value = []; auditRows.value = [] }
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
  const bytes = new TextEncoder().encode(newPassword.value).length
  if (!currentPassword.value || bytes < 6 || bytes > 30 || newPassword.value !== confirmPassword.value) {
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
async function chooseSection(next: typeof section.value) {
  if (accountWriteLocked.value) return
  if (next !== section.value) resetMemberFilters()
  if (next === 'audit' && section.value !== 'audit') auditView.value = 'audit'
  walletMember.value = ''
  if (next !== 'audit') operationsExpanded.value = false
  if (next !== 'wallets') walletsExpanded.value = false
  if (next !== 'commission') commissionExpanded.value = false
  section.value = next; error.value = ''; notice.value = ''
  memberOffset.value = 0; auditOffset.value = 0; memberTotal.value = '0'; moreAudit.value = false
  if (next !== 'users') { memberRequestGeneration++; members.value = [] }
  if (next !== 'audit') { auditRequestGeneration++; auditRows.value = [] }
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
  error.value = ''; notice.value = ''
  if (!selectedBrand.value) loading.value = false
}
async function chooseBrand(id: string) {
  if (accountWriteLocked.value) return
  walletMember.value = ''
  resetMemberFilters()
  selectedBrand.value = id; members.value = []; auditRows.value = []; error.value = ''; notice.value = ''
  memberRequestGeneration++; auditRequestGeneration++
  memberOffset.value = 0; auditOffset.value = 0; memberTotal.value = '0'; moreAudit.value = false
  if (!id) { loading.value = false; return }
  if (section.value === 'users') await loadMembers(id)
  if (section.value === 'audit' && auditView.value === 'audit') await loadAudit(id)
}
async function chooseAuditView(next: typeof auditView.value) {
  if (accountWriteLocked.value) return
  walletMember.value = ''
  operationsExpanded.value = true
  section.value = 'audit'
  auditView.value = next; auditRequestGeneration++; auditRows.value = []
  auditOffset.value = 0; moreAudit.value = false; loading.value = false; error.value = ''
  if (next === 'audit' && selectedBrand.value) await loadAudit(selectedBrand.value)
}
async function loadMembers(brandId: string, offset = 0) {
  const generation = ++memberRequestGeneration
  members.value = []; loading.value = true; memberTotal.value = '0'; error.value = ''
  try {
    const result = await api.users(brandId, pageSize, offset, '', { query: memberQuery.value, online: memberOnlineQuery.value })
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
async function loadAudit(brandId: string, offset = 0) {
  const generation = ++auditRequestGeneration
  auditRows.value = []; loading.value = true; moreAudit.value = false; error.value = ''
  try {
    const result = await api.audit(brandId, pageSize + 1, offset)
    if (generation === auditRequestGeneration && selectedBrand.value === brandId && section.value === 'audit' && auditView.value === 'audit') {
      auditRows.value = result.slice(0, pageSize)
      auditOffset.value = offset
      moreAudit.value = result.length > pageSize
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
async function openBrandAccounts(brandId: string) { accessScope.value = 'brand'; await chooseSection('access'); await chooseBrand(brandId) }
async function openBrandAudit(brandId: string) { section.value = 'audit'; operationsExpanded.value = true; await chooseBrand(brandId) }
function openBrandOperation(brandId: string) {
  notice.value = ''
  brandOperationPanel.value?.openForBrand(brandId)
}
async function refreshAfterBrandOperation(record: BrandOperation) {
  try {
    await loadWorkspace()
    notice.value = t('common.statusConfirmed', { name: record.name })
  } catch {
    brands.value = []
    members.value = []
    auditRows.value = []
    selectedBrand.value = ''
    memberRequestGeneration++
    auditRequestGeneration++
    notice.value = t('common.statusChanged')
  }
}
function openWallet(memberId: string) { walletMember.value = memberId; section.value = 'wallets'; walletView.value = 'wallet'; walletsExpanded.value = true; memberRequestGeneration++; auditRequestGeneration++; loading.value = false; error.value = '' }
function beginCreate() {
  if (retryPayload.value) { confirming.value = true; modal.value = true; return }
  form.value = { code: '', name: '', default_locale: 'en', timezone: 'UTC' }; confirming.value = false; modal.value = true
}
function reviewCreate() { error.value = ''; confirming.value = true }
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
  if (!deleteMemberIntent.value) deleteMemberIntent.value = { id: member.global_user_id, key: crypto.randomUUID() }
  const intent = deleteMemberIntent.value, brand = selectedBrand.value
  deleteMemberBusy.value = true; deleteMemberError.value = ''; error.value = ''
  try {
    const deleted = await api.deleteMemberAccount(intent.id, intent.key)
    if (deleteMemberIntent.value !== intent) return
    deleteMemberTarget.value = null; deleteMemberIntent.value = null; deleteMemberUnknown.value = false
    notice.value = t('common.memberDeleted', { name: deleted.account_name })
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
    deleteBrandTarget.value = null; notice.value = lang.value.deleteSuccess
    if (selectedBrand.value === brand.id) { selectedBrand.value = ''; members.value = []; auditRows.value = [] }
    await loadWorkspace()
  } catch (cause) {
    deleteError.value = cause instanceof Error ? publicErrorMessage(cause) : publicErrorMessage(cause)
  } finally { deleteBusy.value = false }
}
async function submitCreate() {
  if (!retryPayload.value) retryPayload.value = freezeBrandCreateRequest(form.value)
  const pending = retryPayload.value
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api.createBrand(pending.body, pending.key)
    brandQuery.value = ''; brandStatus.value = ''
    modal.value = false; confirming.value = false; retryPayload.value = null; notice.value = lang.value.created
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
</script>

<template>
  <GlobalLoadingOverlay :label="lang.loading" :hidden="accountWriteLocked" />
  <main v-if="!authReady" class="loading-shell" aria-busy="true"></main>
  <LoginEntry v-else-if="!account" class="platform-login" card-class="login-card" :brand-name="platformPresentation?.display_name ?? ''" :logo-text="platformPresentation?.logo_text" :logo-url="platformPresentation?.logo_url" :title="lang.signIn" title-id="platform-login-title" :locale="locale" :language-label="t('common.language')" :languages="(['en','zh-CN'] as const).map(code=>({code,label:nativeLocaleName(code)}))" @update:locale="setLocale">
      <template v-if="!platformPresentation"><p class="message error" role="alert">{{ t('platform.login.presentationUnavailable') }}</p><button class="button button-secondary" @click="withGlobalLoading(initialize)">{{ t('platform.login.retry') }}</button></template>
      <form v-write-guard="busy" v-else class="login-entry__form" novalidate v-inline-validation @submit.prevent="withGlobalLoading(login)">
        <LoginCredentials v-model:identifier="identifier" v-model:password="password" :identifier-label="lang.identifier" :password-label="lang.password" />
        <p v-if="sessionReplaced" class="message error" role="alert">{{ t('auth.sessionReplaced') }}</p>
        <p v-if="error && !sessionReplaced" class="message error" role="alert">{{ displayPublicError(error) }}</p><button class="button button-primary full" :disabled="busy">{{ busy ? lang.loading : lang.signIn }}</button>
      </form>
  </LoginEntry>
  <div v-else class="app-frame management-console" :class="{ 'mobile-navigation-open': mobileNavigationOpen }" @keydown.esc="mobileNavigationOpen = false">
    <button v-if="mobileNavigationOpen" type="button" class="platform-nav-scrim" :aria-label="lang.close" @click="mobileNavigationOpen = false" />
    <aside id="platform-navigation" class="sidebar" @click="closeMobileNavigation">
      <div class="logo-row"><img v-if="platformSettings?.logo_url" class="brand-logo-image" :src="platformSettings.logo_url" alt="" /><div v-else class="brand-mark small">{{ platformSettings?.logo_text }}</div><div><strong>{{ platformSettings?.display_name }}</strong></div></div>
      <button type="button" :class="['nav-item', { selected: section === 'home' }]" @click="withGlobalLoading(chooseSection, 'home')"><span><SiteIcon name="home" /></span>{{ t('overview.home') }}</button>
      <button type="button" :class="['nav-item', { selected: brandSectionActive }]" :aria-expanded="brandsExpanded" aria-controls="platform-brand-tabs" @click="!accountWriteLocked && (brandsExpanded = !brandsExpanded)"><span><SiteIcon name="brands" /></span>{{ t('common.brands') }}</button>
      <div v-if="brandsExpanded" id="platform-brand-tabs" class="nav-submenu" data-testid="platform-brand-tabs" role="group" :aria-label="t('common.brands')">
        <button type="button" class="nav-subitem" :class="{ selected: section === 'brands' }" @click="withGlobalLoading(chooseSection, 'brands')">{{ lang.brands }}</button>
        <button type="button" class="nav-subitem" :class="{ selected: section === 'access' && accessScope === 'brand' }" @click="withGlobalLoading(chooseBrandAccounts)">{{ lang.brandAccounts }}</button>
        <button type="button" class="nav-subitem" :class="{ selected: section === 'users' }" @click="withGlobalLoading(chooseSection, 'users')">{{ lang.users }}</button>
      </div>
      <button type="button" :class="['nav-item', { selected: section === 'audit' }]" :aria-expanded="operationsExpanded" @click="withGlobalLoading(toggleOperations)"><span><SiteIcon name="more" /></span>{{ operationsLabel }}</button>
      <div v-if="operationsExpanded" class="nav-submenu" data-testid="platform-operations-tabs" role="group" :aria-label="operationsLabel">
        <button class="nav-subitem" :class="{ selected: auditView === 'audit' }" type="button" @click="withGlobalLoading(chooseAuditView, 'audit')">{{ lang.audit }}</button>
        <button class="nav-subitem" :class="{ selected: auditView === 'archives' }" type="button" @click="withGlobalLoading(chooseAuditView, 'archives')">{{ t('common.reportArchives') }}</button>
        <button class="nav-subitem" :class="{ selected: auditView === 'archive-tasks' }" type="button" @click="withGlobalLoading(chooseAuditView, 'archive-tasks')">{{ t('common.archiveTasks') }}</button>
        <button class="nav-subitem" :class="{ selected: auditView === 'financial-policies' }" type="button" @click="withGlobalLoading(chooseAuditView, 'financial-policies')">{{ t('common.financialPolicies') }}</button>
        <button class="nav-subitem" :class="{ selected: auditView === 'operational-policies' }" type="button" @click="withGlobalLoading(chooseAuditView, 'operational-policies')">{{ t('common.operationalPolicies') }}</button>
        <button class="nav-subitem" :class="{ selected: auditView === 'security' }" type="button" @click="withGlobalLoading(chooseAuditView, 'security')">{{ t('security.title') }}</button>
        <button class="nav-subitem" :class="{ selected: auditView === 'notifications' }" type="button" @click="withGlobalLoading(chooseAuditView, 'notifications')">{{ t('common.notifications') }}</button>
      </div>
      <button :class="['nav-item', { selected: section === 'rewards' }]" @click="withGlobalLoading(chooseSection, 'rewards')"><span><SiteIcon name="gift" /></span>{{ lang.rewards }}</button>
      <button type="button" :class="['nav-item', { selected: section === 'wallets' }]" :aria-expanded="walletsExpanded" @click="withGlobalLoading(toggleWallets)"><span><SiteIcon name="wallet" /></span>{{ walletLabel }}</button>
      <div v-if="walletsExpanded" class="nav-submenu" data-testid="platform-wallet-tabs" role="group" :aria-label="walletLabel">
        <button class="nav-subitem" :class="{ selected: walletView === 'wallet' }" type="button" @click="withGlobalLoading(chooseWalletView, 'wallet')">{{ t('platform.copy.c0368f3e') }}</button>
        <button class="nav-subitem" :class="{ selected: walletView === 'recharges' }" type="button" @click="withGlobalLoading(chooseWalletView, 'recharges')">{{ t('platform.copy.20be6d4c') }}</button>
        <button class="nav-subitem" :class="{ selected: walletView === 'reports' }" type="button" @click="withGlobalLoading(chooseWalletView, 'reports')">{{ t('platform.copy.67516591') }}</button>
      </div>
      <button :class="['nav-item', { selected: section === 'bets' }]" @click="withGlobalLoading(chooseSection, 'bets')"><span><SiteIcon name="ticket" /></span>{{ betLabel }}</button>
      <button :class="['nav-item', { selected: section === 'games' }]" @click="withGlobalLoading(chooseSection, 'games')"><span><SiteIcon name="rules" /></span>{{ gamesLabel }}</button>
      <button :class="['nav-item', { selected: section === 'draws' }]" @click="withGlobalLoading(chooseSection, 'draws')"><span><SiteIcon name="calendar" /></span>{{ drawsLabel }}</button>
      <button :class="['nav-item', { selected: section === 'withdrawals' }]" @click="withGlobalLoading(chooseSection, 'withdrawals')"><span><SiteIcon name="external" /></span>{{ withdrawalLabel }}</button>
      <button type="button" :class="['nav-item', { selected: section === 'commission' }]" :aria-expanded="commissionExpanded" @click="withGlobalLoading(toggleCommission)"><span><SiteIcon name="trend" /></span>{{ commissionLabel }}</button>
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
      <button type="button" :class="{ selected: section === 'wallets' }" @click="withGlobalLoading(async () => { mobileNavigationOpen = false; await chooseWalletView('wallet') })"><SiteIcon name="wallet" /><span>{{ walletLabel }}</span></button>
      <button type="button" :class="{ selected: section === 'bets' }" @click="withGlobalLoading(async () => { mobileNavigationOpen = false; await chooseSection('bets') })"><SiteIcon name="ticket" /><span>{{ betLabel }}</span></button>
      <button type="button" :class="{ selected: mobileNavigationOpen }" :aria-expanded="mobileNavigationOpen" aria-controls="platform-navigation" @click="mobileNavigationOpen = !mobileNavigationOpen"><SiteIcon :name="mobileNavigationOpen ? 'close' : 'menu'" /><span>{{ t('common.menu') }}</span></button>
    </nav>
    <section class="main-column">
      <header class="topbar">
        <nav class="breadcrumb" aria-label="Breadcrumb"><button type="button" @click="withGlobalLoading(chooseSection, 'home')">{{ platformSettings?.display_name }}</button><span>/</span><button type="button" @click="withGlobalLoading(chooseSection, section)">{{ sectionLabel }}</button></nav>
        <label v-if="showBrandPicker" class="topbar-brand-picker">
          <select :disabled="accountWriteLocked" :value="selectedBrand" :aria-label="lang.brandLabel" @change="withGlobalLoading(chooseBrand, ($event.target as HTMLSelectElement).value)">
            <option value="">— {{ lang.selectBrand }} —</option>
            <option v-for="brand in brands" :key="brand.id" :value="brand.id">{{ brand.name }}</option>
          </select>
        </label>
        <div class="top-actions">
          <LanguageSwitcher :model-value="locale" :label="t('common.language')" :options="[{code:'zh-CN',label:nativeLocaleName('zh-CN')},{code:'en',label:nativeLocaleName('en')}]" @update:model-value="setLocale" />
          <AccountMenu :username="accountName"><button type="button" role="menuitem" @click="accountProfileOpen = true">{{ t('common.profile') }}</button><button type="button" role="menuitem" @click="changePasswordOpen = true">{{ t('common.changePassword') }}</button><button v-write-guard="busy" type="button" role="menuitem" class="popover-logout" :disabled="busy" @click="withGlobalLoading(signOut)">{{ lang.logout }}</button></AccountMenu>
        </div>
      </header>
      <main class="content">
        <OverviewPanel v-if="section === 'home' && platformSettings" :key="account.id" :can-view="account.platform_permissions.includes('overview.view.platform')" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" @brands="withGlobalLoading(chooseSection, 'brands')" />
        <template v-else-if="section === 'brands'">
          <div class="page-heading compact-heading"><h1>{{ lang.brands }}</h1><button class="button button-primary" @click="beginCreate"><SiteIcon name="plus" /> {{ lang.createBrand }}</button></div>
          <section class="panel" data-testid="platform-brand-list">
            <form novalidate v-inline-validation class="wallet-query" @submit.prevent="withGlobalLoading(async () => { return brandQuery = brandQuery.trim() })">
              <label class="query-field">
                <div class="input-with-icon">
                  <span class="input-icon"><SiteIcon name="search" /></span>
                  <input v-model="brandQuery" :aria-label="directoryCopy.search" :placeholder="directoryCopy.hint" type="search" />
                </div>
              </label>
              <label class="query-field query-field-status">
                <span class="field-label"><BusinessDetails :value="directoryCopy.status" :locale="locale" /></span>
                <select v-model="brandStatus">
                  <option value="">{{ directoryCopy.all }}</option>
                  <option value="active">{{ lang.statusActive }}</option>
                  <option value="paused">{{ lang.statusPaused }}</option>
                  <option value="disabled">{{ t('brandOperation.stopped') }}</option>
                </select>
              </label>
              <button v-if="brandQuery || brandStatus" type="button" class="button button-secondary clear-btn" @click="brandQuery = ''; brandStatus = ''">{{ directoryCopy.clear }}</button>
              <button class="button button-primary query-submit" type="submit">{{ directoryCopy.submit }}</button>
            </form>
            <div class="table-wrap"><table><thead><tr><th>{{ lang.name }}</th><th><BusinessDetails :value="lang.status" :locale="locale" /></th><th></th></tr></thead><tbody>
              <tr v-for="brand in filteredBrands" :key="brand.id"><td><strong>{{ brand.name }}</strong></td><td><span :class="['status-pill', brand.status]">{{ brand.status === 'disabled' ? t('brandOperation.stopped') : statusLabel(brand.status) }}</span></td><td><button class="row-action" :aria-label="lang.view" @click="withGlobalLoading(openBrandMembers, brand.id)"><span aria-hidden="true"><SiteIcon name="users" /></span> {{ lang.membersAction }}</button><button class="row-action" :aria-label="t('common.viewAccounts')" @click="withGlobalLoading(openBrandAccounts, brand.id)"><span aria-hidden="true"><SiteIcon name="shield" /></span> {{ lang.accountsAction }}</button><button v-if="canManageBrandOperation" class="row-action" :aria-label="t('common.manageStatus')" @click="openBrandOperation(brand.id)"><span aria-hidden="true"><SiteIcon name="settings" /></span> {{ lang.statusAction }}</button><button v-if="canDeleteBrand" class="row-action danger-text" :aria-label="lang.deleteBrand" @click="beginDelete(brand)"><span aria-hidden="true"><SiteIcon name="close" /></span> {{ lang.deleteAction }}</button></td></tr>
              <tr v-if="!brands.length"><td colspan="3" class="empty-state">{{ lang.empty }}</td></tr>
              <tr v-else-if="!filteredBrands.length"><td colspan="3" class="empty-state" role="status">{{ directoryCopy.empty }}</td></tr>
            </tbody></table></div><TablePagination :summary="`${filteredBrands.length} ${lang.brandTotal}`" :aria-label="t('common.brandTableSummary')" />
          </section>
        </template>
        <template v-else-if="section === 'access'">
          <div class="page-heading compact-heading"><h1>{{ accessLabel }}</h1></div>
          <AccessPanel v-if="accessScope === 'platform' || selectedBrand" :brand-id="selectedBrand" :locale="locale" :scope="accessScope" :account-id="account.id" :grants="account.platform_permissions" :initial-tab="accessTab" @locked="accountWriteLocked = $event" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'users'">
          <div class="page-heading compact-heading"><h1>{{ lang.users }}</h1></div>
          <section v-if="selectedBrand" class="panel">
            <DirectorySearch  v-model="memberSearch" :label="t('members.search')" :query-label="t('members.query')" :clear-label="t('members.clear')" :busy="loading" :active="Boolean(memberSearch||memberOnlineFilter)" :search="searchMembers" :clear="clearMemberFilters"><select v-model="memberOnlineFilter" :aria-label="t('members.onlineStatus')"><option value="">{{t('members.allOnline')}}</option><option value="true">{{t('members.online')}}</option><option value="false">{{t('members.offline')}}</option></select></DirectorySearch>
            <div class="table-wrap desktop-table"><table class="platform-member-table"><thead><tr><th>{{ lang.username }}</th><th>{{ t('members.onlineStatus') }}</th><th>{{ pointsAction }}</th><th>{{ lang.phone }}</th><th><BusinessDetails :value="lang.status" :locale="locale" /></th><th>{{ t('members.joined') }}</th><th>{{ t('common.actions') }}</th></tr></thead><tbody><tr v-for="member in members" :key="member.id"><td><button type="button" class="text-button member-account" @click="memberDetail = member">{{ member.username || member.phone || t('association.unnamed') }}</button></td><td><span class="member-presence" :class="{ 'is-online': member.online }"><i aria-hidden="true" />{{ t(member.online ? 'members.online' : 'members.offline') }}</span></td><td class="member-points">{{ member.display_points === null ? t('members.pointsRestricted') : member.display_points }}</td><td>{{ member.phone || '—' }}</td><td><span :class="['status-pill', member.status]">{{ statusLabel(member.status) }}</span></td><td>{{ formatPlatformDateTime(member.joined_at, displaySettings, locale) }}</td><td><MemberActionsMenu :id="`platform-member-actions-${member.id}-table`" :label="t('members.actions', { account: member.username || member.phone })"><button type="button" role="menuitem" @click="memberDetail = member">{{ t('members.details') }}</button><button v-if="canViewMemberPoints" type="button" role="menuitem" @click="openWallet(member.id)">{{ t('members.viewPoints') }}</button><button v-if="canDeleteMember" type="button" role="menuitem" class="danger-text" @click="beginMemberDelete(member)">{{ lang.deleteMember }}</button></MemberActionsMenu></td></tr></tbody></table></div>
            <div class="mobile-cards platform-member-cards"><article v-for="member in members" :key="member.id" class="user-card"><div class="card-title"><button type="button" class="text-button member-account" @click="memberDetail = member">{{ member.username || member.phone || t('association.unnamed') }}</button><MemberActionsMenu :id="`platform-member-actions-${member.id}-card`" :label="t('members.actions', { account: member.username || member.phone })"><button type="button" role="menuitem" @click="memberDetail = member">{{ t('members.details') }}</button><button v-if="canViewMemberPoints" type="button" role="menuitem" @click="openWallet(member.id)">{{ t('members.viewPoints') }}</button><button v-if="canDeleteMember" type="button" role="menuitem" class="danger-text" @click="beginMemberDelete(member)">{{ lang.deleteMember }}</button></MemberActionsMenu></div><dl class="member-details"><div><dt>{{ t('members.onlineStatus') }}</dt><dd><span class="member-presence" :class="{ 'is-online': member.online }"><i aria-hidden="true" />{{ t(member.online ? 'members.online' : 'members.offline') }}</span></dd></div><div><dt>{{ pointsAction }}</dt><dd class="member-points">{{ member.display_points === null ? t('members.pointsRestricted') : member.display_points }}</dd></div><div><dt>{{ lang.phone }}</dt><dd>{{ member.phone || '—' }}</dd></div><div><dt><BusinessDetails :value="lang.status" :locale="locale" /></dt><dd>{{ statusLabel(member.status) }}</dd></div><div><dt>{{ t('members.joined') }}</dt><dd>{{ formatPlatformDateTime(member.joined_at, displaySettings, locale) }}</dd></div></dl></article></div>
            <p v-if="!members.length && !loading" class="empty-state">{{ t('members.empty') }}</p>
            <TablePagination data-testid="platform-member-pages" :summary="t('members.page', { page: String(Math.floor(memberOffset / pageSize) + 1), count: String(members.length), total: memberTotal })"><template #actions><button type="button" class="button button-secondary" :disabled="loading || memberOffset === 0" @click="withGlobalLoading(loadMembers, selectedBrand, memberOffset - pageSize)">{{ previousPage }}</button><button type="button" class="button button-secondary" :disabled="loading || !moreMembers" @click="withGlobalLoading(loadMembers, selectedBrand, memberOffset + pageSize)">{{ nextPage }}</button></template></TablePagination>
          </section>
          <DetailModal v-if="memberDetail" :title="t('members.details')" :close-label="lang.close" @close="memberDetail = null"><dl class="member-details"><div><dt>{{ lang.username }}</dt><dd>{{ memberDetail.username || memberDetail.phone || t('association.unnamed') }}</dd></div><div><dt>{{ lang.displayName }}</dt><dd>{{ memberDetail.display_name || '—' }}</dd></div><div><dt>{{ t('members.onlineStatus') }}</dt><dd>{{ t(memberDetail.online ? 'members.online' : 'members.offline') }}</dd></div><div><dt>{{ pointsAction }}</dt><dd>{{ memberDetail.display_points === null ? t('members.pointsRestricted') : memberDetail.display_points }}</dd></div><div><dt>{{ lang.phone }}</dt><dd>{{ memberDetail.phone || '—' }}</dd></div><div><dt><BusinessDetails :value="lang.status" :locale="locale" /></dt><dd>{{ statusLabel(memberDetail.status) }}</dd></div><div><dt>{{ t('members.joined') }}</dt><dd>{{ formatPlatformDateTime(memberDetail.joined_at, displaySettings, locale) }}</dd></div><div><dt>{{ t('members.notes') }}</dt><dd>{{ memberDetail.notes || '—' }}</dd></div><div><dt>{{ t('members.tags') }}</dt><dd>{{ memberDetail.tags.join(' · ') || '—' }}</dd></div></dl></DetailModal>
        </template>
        <template v-else-if="section === 'audit'">
          <div class="page-heading compact-heading"><h1>{{ operationLabel }}</h1></div>
          <ArchivesPanel v-if="selectedBrand && auditView === 'archives'" :brand-id="selectedBrand" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
          <ArchiveTasksPanel v-else-if="selectedBrand && auditView === 'archive-tasks'" :brand-id="selectedBrand" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
          <FinancialPoliciesPanel v-else-if="selectedBrand && auditView === 'financial-policies'" :brand-id="selectedBrand" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
          <OperationalPoliciesPanel v-else-if="selectedBrand && auditView === 'operational-policies'" :brand-id="selectedBrand" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
          <SecuritySettingsPanel v-else-if="selectedBrand && auditView === 'security'" :brand-id="selectedBrand" :account="account" :status="selected?.status || ''" @failure="handleFailure" />
          <NotificationsPanel v-else-if="selectedBrand && auditView === 'notifications'" :brand-id="selectedBrand" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
          <template v-else-if="auditView === 'audit'">
          <section v-if="selectedBrand" class="panel"><div class="table-wrap"><table><thead><tr><th>{{ lang.action }}</th><th>{{ lang.actor }}</th><th>{{ lang.resource }}</th><th>{{ lang.reason }}</th><th>{{ lang.date }}</th></tr></thead><tbody><tr v-for="row in auditRows" :key="row.id"><td><span class="action-label">{{ row.action }}</span></td><td>{{ auditActorLabel(row) }}</td><td>{{ row.resource_type }}</td><td class="reason-cell">{{ row.reason || '—' }}</td><td>{{ formatPlatformDateTime(row.created_at, displaySettings, locale) }}</td></tr><tr v-if="!auditRows.length"><td colspan="5" class="empty-state">{{ lang.empty }}</td></tr></tbody></table></div></section>
          <div v-if="selectedBrand" class="reward-list-actions" data-testid="platform-audit-pages">
            <span>{{ thisPage }} {{ auditRows.length }}</span>
            <button class="button button-secondary" :disabled="loading || auditOffset === 0" @click="withGlobalLoading(loadAudit, selectedBrand, auditOffset - pageSize)">{{ previousPage }}</button>
            <span>{{ Math.floor(auditOffset / pageSize) + 1 }}</span>
            <button class="button button-secondary" :disabled="loading || !moreAudit" @click="withGlobalLoading(loadAudit, selectedBrand, auditOffset + pageSize)">{{ nextPage }}</button>
            <button v-refresh-button="t('common.refresh')" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(loadAudit, selectedBrand)">{{ t('common.refresh') }}</button>
          </div>
          </template>
        </template>
        <template v-else-if="section === 'rewards'">
          <div class="page-heading compact-heading"><h1>{{ lang.rewards }}</h1></div>
          <RewardsPanel v-if="selectedBrand" :brand-id="selectedBrand" :brand-name="selected?.name || ''" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'wallets'">
          <div class="page-heading compact-heading"><h1>{{ walletViewLabel }}</h1></div>
          <WalletPanel v-if="selectedBrand" :brand-id="selectedBrand" :member-id="walletMember" :locale="locale" :display-settings="displaySettings" :view="walletView" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'bets'">
          <div class="page-heading compact-heading"><h1>{{ betLabel }}</h1></div>
          <div v-if="selectedBrand" class="brand-picker"><label><select v-model="selectedGameId" :aria-label="gamesLabel" :disabled="gamesLoading"><option value="">{{ t('common.allGames') }}</option><option v-for="game in brandGames" :key="game.id" :value="game.id">{{ game.name }}</option></select></label></div>
          <BetOrdersPanel v-if="selectedBrand" :brand-id="selectedBrand" :game-id="selectedGameId" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'games' || section === 'draws'">
          <div class="page-heading compact-heading"><h1>{{ section === 'games' ? gamesLabel : drawsLabel }}</h1></div>
          <div v-if="section === 'draws' && selectedBrand" class="brand-picker"><label><select v-model="selectedGameId" :aria-label="gamesLabel" :disabled="gamesLoading || !brandGames.length"><option value="">— {{ gamesLabel }} —</option><option v-for="game in brandGames" :key="game.id" :value="game.id">{{ game.name }}</option></select></label></div>
          <PlatformLottery v-if="section === 'games' && account?.platform_permissions.includes('platform_game.view.platform')" surface="platform" brand-id="" :locale="locale" :display-settings="displaySettings" :can-manage="account.platform_permissions.includes('platform_game.write.platform')" :can-connect="false" unified-catalog :catalog-brand-id="selectedBrand" :brand-games="brandGames" :brand-name="brands.find(brand => brand.id === selectedBrand)?.name" :brand-loading="gamesLoading" @brand-detail="openBrandGameDetail" @refresh-brand="refreshBrandGames" @failure="handleFailure" />
          <LotteryPanel v-if="selectedBrand" ref="brandLotteryDetails" :brand-id="selectedBrand" :selected-game="section === 'draws' ? brandGames.find(game => game.id === selectedGameId) : undefined" :locale="locale" :mode="section === 'games' ? 'games' : 'draws'" :details-only="section === 'games'" :display-settings="displaySettings" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'commission'">
          <div class="page-heading compact-heading"><h1>{{ commissionPageLabel }}</h1></div>
          <CommissionPanel v-if="selectedBrand" :brand-id="selectedBrand" :locale="locale" :initial-tab="commissionTab" :display-settings="displaySettings" @failure="handleFailure" />
        </template>
        <template v-else-if="section === 'settings'">
          <PlatformSettingsPanel v-if="platformSettings" :initial="platformSettings" :locale="locale" @saved="platformSettings = $event; notice = t('common.settingsSaved')" @failure="handleFailure" />
        </template>
        <template v-else>
          <div class="page-heading compact-heading"><h1>{{ withdrawalLabel }}</h1></div>
          <WithdrawalsPanel v-if="selectedBrand" :brand-id="selectedBrand" :locale="locale" :display-settings="displaySettings" @failure="handleFailure" />
        </template>
        <p v-if="showBrandPicker && !selectedBrand" class="empty-state" role="status" data-testid="platform-brand-required">{{ t('common.selectBrandFirst') }}</p>
      </main>
      <BrandOperationPanel ref="brandOperationPanel" :locale="locale" :allowed="canManageBrandOperation" :display-settings="displaySettings" @updated="withGlobalLoading(refreshAfterBrandOperation, $event)" @failure="handleFailure" />
    </section>
    <AccountProfileDialog v-if="accountProfileOpen" :title="t('common.profile')" :close-label="lang.close" :fields="[{label:lang.username,value:accountName},{label:lang.role,value:lang.platformAdmin}]" @close="accountProfileOpen=false" />
    <ChangePasswordDialog v-if="changePasswordOpen" v-model:current="currentPassword" v-model:next="newPassword" v-model:confirmation="confirmPassword" :title="t('common.changePassword')" :close-label="lang.close" :current-label="t('common.currentPassword')" :new-label="t('common.newPassword')" :confirm-label="t('common.confirmNewPassword')" :cancel-label="lang.cancel" :submit-label="passwordBusy ? lang.loading : t('common.savePassword')" :busy="passwordBusy" @close="changePasswordOpen=false" :submit="submitPasswordChange"><p v-if="passwordError" class="form-error" role="alert">{{ displayPublicError(passwordError) }}</p></ChangePasswordDialog>
    <div v-if="modal" class="modal-scrim" @click.self="!busy && (modal = false)"><section class="modal-card" role="dialog" aria-modal="true" :aria-label="confirming ? lang.confirmation : lang.createBrand">
      <template v-if="!confirming"><div class="modal-heading"><div><div class="eyebrow">{{ lang.platformDirectory }}</div><h2>{{ lang.createBrand }}</h2></div><button class="icon-button" @click="modal = false"><SiteIcon name="close" /></button></div>
        <form novalidate v-inline-validation class="brand-form" @submit.prevent="withGlobalLoading(reviewCreate)"><label>{{ lang.code }}<input v-model.trim="form.code" required pattern="[a-z][a-z0-9_]{0,47}" :placeholder="lang.codePlaceholder" /></label><label>{{ lang.name }}<input v-model.trim="form.name" required maxlength="120" /></label><div class="form-row"><label>{{ lang.locale }}<select v-model="form.default_locale"><option value="en">{{ lang.localeEnglish }}</option><option value="zh-CN">{{ lang.localeChinese }}</option></select></label><label>{{ lang.timezone }}<input v-model.trim="form.timezone" required :placeholder="lang.timezonePlaceholder" /></label></div><div class="form-actions"><button type="button" class="button button-secondary" @click="modal = false">{{ lang.cancel }}</button><button class="button button-primary">{{ lang.review }} <span>→</span></button></div></form>
      </template><template v-else><div class="modal-heading"><div><div class="eyebrow">{{ lang.confirmation }}</div><h2>{{ lang.confirmation }}</h2></div><button class="icon-button" :disabled="busy" :aria-label="lang.close" @click="modal = false"><SiteIcon name="close" /></button></div><p class="modal-intro">{{ lang.confirmCopy }}</p><dl class="confirm-list"><div><dt>{{ lang.code }}</dt><dd>{{ retryPayload?.body.code || form.code }}</dd></div><div><dt>{{ lang.name }}</dt><dd>{{ retryPayload?.body.name || form.name }}</dd></div><div><dt>{{ lang.locale }}</dt><dd>{{ retryPayload?.body.default_locale || form.default_locale }}</dd></div><div><dt>{{ lang.timezone }}</dt><dd>{{ retryPayload?.body.timezone || form.timezone }}</dd></div></dl><p v-if="retryPayload && error" class="message error">{{ displayPublicError(error) }}</p><div class="form-actions"><button v-if="!retryPayload" class="button button-secondary" :disabled="busy" @click="confirming = false">{{ lang.cancel }}</button><button v-write-guard="busy" class="button button-primary" :disabled="busy" @click="withGlobalLoading(submitCreate)">{{ busy ? lang.loading : retryPayload ? lang.retry : lang.confirm }}</button></div></template>
    </section></div>
    <div v-if="deleteBrandTarget" class="modal-scrim" @click.self="!deleteBusy && (deleteBrandTarget = null)"><section class="modal-card delete-modal" role="dialog" aria-modal="true" :aria-label="lang.deleteConfirm"><div class="modal-heading"><h2>{{ lang.deleteConfirm }}</h2><button class="icon-button" :disabled="deleteBusy" :aria-label="lang.close" @click="deleteBrandTarget = null"><SiteIcon name="close" /></button></div><p class="modal-intro">{{ lang.deleteWarning }}</p><dl class="confirm-list"><div><dt>{{ lang.name }}</dt><dd>{{ deleteBrandTarget.name }}</dd></div><div><dt>{{ lang.code }}</dt><dd>{{ deleteBrandTarget.name }}</dd></div></dl><p v-if="deleteError" class="message error">{{ displayPublicError(deleteError) }}</p><div class="form-actions"><button type="button" class="button button-secondary" :disabled="deleteBusy" @click="deleteBrandTarget = null">{{ lang.cancel }}</button><button v-write-guard="deleteBusy" class="button button-danger" :disabled="deleteBusy" @click="withGlobalLoading(submitDelete)">{{ deleteBusy ? lang.loading : lang.deleteBrand }}</button></div></section></div>
    <div v-if="deleteMemberTarget" class="modal-scrim" @click.self="!deleteMemberBusy && !deleteMemberUnknown && (deleteMemberTarget = null)"><section class="modal-card delete-modal" role="dialog" aria-modal="true" :aria-label="lang.deleteMember"><div class="modal-heading"><h2>{{ lang.deleteMember }}</h2><button class="icon-button" :disabled="deleteMemberBusy || deleteMemberUnknown" :aria-label="lang.close" @click="deleteMemberTarget = null"><SiteIcon name="close" /></button></div><p class="modal-intro">{{ lang.deleteMemberConfirm }}</p><dl class="confirm-list"><div><dt>{{ lang.username }}</dt><dd>{{ deleteMemberTarget.username || deleteMemberTarget.phone || t('association.unnamed') }}</dd></div><div><dt>{{ lang.displayName }}</dt><dd>{{ deleteMemberTarget.display_name || '—' }}</dd></div></dl><p v-if="deleteMemberError" class="message error" role="alert">{{ displayPublicError(deleteMemberError) }}</p><p v-if="deleteMemberUnknown" class="message" role="status">{{ t('common.uncertainRequest') }}</p><div class="form-actions"><button type="button" class="button button-secondary" :disabled="deleteMemberBusy || deleteMemberUnknown" @click="deleteMemberTarget = null">{{ lang.cancel }}</button><button v-write-guard="deleteMemberBusy" class="button button-danger" :disabled="deleteMemberBusy" @click="withGlobalLoading(submitMemberDelete)">{{ deleteMemberBusy ? lang.loading : deleteMemberUnknown ? t('common.retryMemberDelete') : lang.deleteMember }}</button></div></section></div>
    <Toast v-if="notice" :title="t('common.operationSuccess')" :message="notice" :close-label="lang.close" @dismiss="notice = ''" />
    <Toast v-if="error" tone="error" :title="t('common.operationFailed')" :message="error" :close-label="lang.close" @dismiss="error = ''" />
  </div>
</template>
