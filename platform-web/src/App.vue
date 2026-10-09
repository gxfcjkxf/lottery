<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { createPlatformApi, freezeBrandCreateRequest, PlatformApiError, type BrandCreateInput, type FrozenBrandCreateRequest, type PlatformAudit, type PlatformBrand, type PlatformMember } from './platform-api'
import RewardsPanel from './RewardsPanel.vue'
import WalletPanel from './WalletPanel.vue'

const api = createPlatformApi()
const locale = ref<'en' | 'zh-CN'>('en')
const lang = computed(() => locale.value === 'en' ? en : zh)
const en = {
  title: 'Northstar', subtitle: 'Platform administration', loginTitle: 'Sign in to platform', identifier: 'Username', password: 'Password', signIn: 'Sign in', workspace: 'Workspace',
  brands: 'Brands', users: 'Brand members', audit: 'Audit log', rewards: 'Manual rewards', createBrand: 'New brand', selectBrand: 'Select a brand', allBrands: 'All brands', brandCount: 'brands in the platform directory',
  name: 'Brand name', code: 'Brand code', status: 'Status', view: 'View members', username: 'Username', displayName: 'Display name', phone: 'Phone', memberCount: 'members',
  locale: 'Default language', timezone: 'Timezone', reason: 'Reason for creation', cancel: 'Cancel', review: 'Review request', confirm: 'Confirm and create', confirmation: 'Confirm new brand', confirmCopy: 'This will create a paused brand and record the stated reason in the audit log.',
  action: 'Action', actor: 'Actor', resource: 'Resource', date: 'Date', empty: 'Nothing to show yet.', loading: 'Loading…', logout: 'Sign out', close: 'Close', platformNote: 'Platform access only · Brand staff should use their brand admin portal.', platformAdmin: 'Platform super admin', error: 'Something went wrong', retry: 'Retry same request', unknown: 'The server may have received this request. Retry only with the same frozen details and key.', created: 'Brand created in paused status.', localeEnglish: 'English', localeChinese: 'Chinese (Simplified)', directory: 'Directory', directoryDescription: 'Manage platform tenants and inspect their members.', brandDirectory: 'Brand directory', brandDirectoryDescription: 'Brands available in the platform directory', memberDirectory: 'Member directory', memberReadOnly: 'Read-only membership details', auditDescription: 'Audit history for the selected brand · read-only', traceability: 'Traceability', accessDirectory: 'Membership directory', platformDirectory: 'Platform directory', createDescription: 'A new brand starts paused. Brand staff access is managed in that brand’s admin portal.', codePlaceholder: 'northstar_shop', timezonePlaceholder: 'Asia/Singapore', accountLabel: 'Account', brandLabel: 'Brand', statusNormal: 'Normal', statusFrozen: 'Frozen', statusDisabled: 'Disabled', statusExpired: 'Expired', statusCancelled: 'Cancelled', statusPaused: 'Paused', statusActive: 'Active', countSuffix: 'records',
 }
const zh = {
  title: 'Northstar', subtitle: '平台管理', loginTitle: '登录平台管理', identifier: '用户名', password: '密码', signIn: '登录', workspace: '工作区',
  brands: '品牌', users: '品牌会员', audit: '审计日志', rewards: '人工奖励', createBrand: '新建品牌', selectBrand: '请选择品牌', allBrands: '全部品牌', brandCount: '个品牌',
  name: '品牌名称', code: '品牌代码', status: '状态', view: '查看会员', username: '用户名', displayName: '显示名称', phone: '手机号', memberCount: '位会员',
  locale: '默认语言', timezone: '时区', reason: '创建原因', cancel: '取消', review: '核对请求', confirm: '确认并创建', confirmation: '确认新建品牌', confirmCopy: '品牌将以暂停状态创建，填写的原因会写入审计日志。',
  action: '操作', actor: '操作人', resource: '资源', date: '时间', empty: '暂无数据。', loading: '加载中…', logout: '退出登录', close: '关闭', platformNote: '仅限平台管理；品牌员工请使用品牌管理后台。', platformAdmin: '平台超级管理员', error: '发生错误', retry: '使用同一请求重试', unknown: '服务器可能已收到请求。仅使用冻结的原始内容和请求键重试。', created: '品牌已创建并暂停。', localeEnglish: '英语', localeChinese: '简体中文', directory: '目录', directoryDescription: '管理平台租户并查看其会员。', brandDirectory: '品牌目录', brandDirectoryDescription: '平台目录中的品牌', memberDirectory: '会员目录', memberReadOnly: '只读会员信息', auditDescription: '所选品牌的审计记录 · 只读', traceability: '操作追踪', accessDirectory: '会员目录', platformDirectory: '平台目录', createDescription: '新品牌将以暂停状态创建。品牌员工权限在对应品牌后台管理。', codePlaceholder: 'northstar_shop', timezonePlaceholder: 'Asia/Singapore', accountLabel: '账号', brandLabel: '品牌', statusNormal: '正常', statusFrozen: '已冻结', statusDisabled: '已停用', statusExpired: '已过期', statusCancelled: '已取消', statusPaused: '已暂停', statusActive: '启用', countSuffix: '条记录',
 }
const account = ref<{ id: string; super_admin: true } | null>(null)
const brands = ref<PlatformBrand[]>([])
const members = ref<PlatformMember[]>([])
const auditRows = ref<PlatformAudit[]>([])
const pageSize = 50
const memberOffset = ref(0)
const auditOffset = ref(0)
const moreMembers = ref(false)
const moreAudit = ref(false)
const previousPage = computed(() => locale.value === 'en' ? 'Previous page' : '上一页')
const nextPage = computed(() => locale.value === 'en' ? 'Next page' : '下一页')
const thisPage = computed(() => locale.value === 'en' ? 'This page' : '本页')
const selectedBrand = ref('')
const section = ref<'brands' | 'users' | 'audit' | 'rewards' | 'wallets'>('brands')
const walletMember = ref('')
const walletLabel = computed(() => locale.value === 'en' ? 'Member points' : '会员积分')
const busy = ref(false)
const loading = ref(false)
const error = ref('')
const notice = ref('')
const identifier = ref('')
const password = ref('')
const modal = ref(false)
const confirming = ref(false)
const retryPayload = ref<FrozenBrandCreateRequest | null>(null)
const form = ref<BrandCreateInput>({ code: '', name: '', default_locale: 'en', timezone: 'UTC', reason: '' })
const selected = computed(() => brands.value.find(brand => brand.id === selectedBrand.value))
let memberRequestGeneration = 0
let auditRequestGeneration = 0

function messageOf(cause: unknown) { return cause instanceof Error ? cause.message : lang.value.error }
function clearPrivateState() {
  walletMember.value = ''
  account.value = null; brands.value = []; members.value = []; auditRows.value = []; selectedBrand.value = ''
  retryPayload.value = null; modal.value = false; confirming.value = false
  loading.value = false
  memberRequestGeneration++; auditRequestGeneration++
  memberOffset.value = 0; auditOffset.value = 0; moreMembers.value = false; moreAudit.value = false
}
function handleFailure(cause: unknown) {
  if (cause instanceof PlatformApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) clearPrivateState()
  error.value = messageOf(cause)
}
async function loadWorkspace() {
  loading.value = true; error.value = ''
  try {
    account.value = await api.me()
    brands.value = await api.brands()
    if (selectedBrand.value && !brands.value.some(brand => brand.id === selectedBrand.value)) { selectedBrand.value = ''; members.value = []; auditRows.value = [] }
    if (selectedBrand.value && section.value === 'users') await loadMembers(selectedBrand.value)
    if (selectedBrand.value && section.value === 'audit') await loadAudit(selectedBrand.value)
  } catch (cause) { handleFailure(cause); throw cause } finally { loading.value = false }
}
async function login() {
  busy.value = true; error.value = ''
  try { await api.login(identifier.value, password.value); password.value = ''; await loadWorkspace() }
  catch (cause) { handleFailure(cause) } finally { busy.value = false }
}
async function signOut() {
  busy.value = true; error.value = ''
  try { await api.logout(); clearPrivateState() }
  catch (cause) { handleFailure(cause) } finally { busy.value = false }
}
async function chooseSection(next: 'brands' | 'users' | 'audit' | 'rewards' | 'wallets') {
  walletMember.value = ''
  section.value = next; error.value = ''; notice.value = ''
  memberOffset.value = 0; auditOffset.value = 0; moreMembers.value = false; moreAudit.value = false
  if (next !== 'users') { memberRequestGeneration++; members.value = [] }
  if (next !== 'audit') { auditRequestGeneration++; auditRows.value = [] }
  if (!selectedBrand.value || (next !== 'audit' && next !== 'users')) loading.value = false
  if (selectedBrand.value && next === 'audit') await loadAudit(selectedBrand.value)
  if (selectedBrand.value && next === 'users') await loadMembers(selectedBrand.value)
}
async function chooseBrand(id: string) {
  walletMember.value = ''
  selectedBrand.value = id; members.value = []; auditRows.value = []; error.value = ''; notice.value = ''
  memberRequestGeneration++; auditRequestGeneration++
  memberOffset.value = 0; auditOffset.value = 0; moreMembers.value = false; moreAudit.value = false
  if (!id) { loading.value = false; return }
  if (section.value === 'users') await loadMembers(id)
  if (section.value === 'audit') await loadAudit(id)
}
async function loadMembers(brandId: string, offset = 0) {
  const generation = ++memberRequestGeneration
  members.value = []; loading.value = true; moreMembers.value = false; error.value = ''
  try {
    const result = await api.users(brandId, pageSize + 1, offset)
    if (generation === memberRequestGeneration && selectedBrand.value === brandId && section.value === 'users') {
      members.value = result.slice(0, pageSize)
      memberOffset.value = offset
      moreMembers.value = result.length > pageSize
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
    if (generation === auditRequestGeneration && selectedBrand.value === brandId && section.value === 'audit') {
      auditRows.value = result.slice(0, pageSize)
      auditOffset.value = offset
      moreAudit.value = result.length > pageSize
    }
  } catch (cause) {
    if (cause instanceof PlatformApiError && cause.status === 401) handleFailure(cause)
    else if (generation === auditRequestGeneration && selectedBrand.value === brandId && section.value === 'audit') handleFailure(cause)
  }
  finally { if (generation === auditRequestGeneration) loading.value = false }
}
function statusLabel(status: string) {
  const keys: Record<string, keyof typeof en> = { normal: 'statusNormal', frozen: 'statusFrozen', disabled: 'statusDisabled', expired: 'statusExpired', cancelled: 'statusCancelled', paused: 'statusPaused', active: 'statusActive' }
  const key = keys[status]
  return key ? lang.value[key] : status
}
async function openBrandMembers(brandId: string) { section.value = 'users'; await chooseBrand(brandId) }
async function openBrandAudit(brandId: string) { section.value = 'audit'; await chooseBrand(brandId) }
function openWallet(memberId: string) { walletMember.value = memberId; section.value = 'wallets'; memberRequestGeneration++; auditRequestGeneration++; loading.value = false; error.value = '' }
function beginCreate() {
  if (retryPayload.value) { confirming.value = true; modal.value = true; return }
  form.value = { code: '', name: '', default_locale: 'en', timezone: 'UTC', reason: '' }; confirming.value = false; modal.value = true
}
function reviewCreate() { error.value = ''; confirming.value = true }
async function submitCreate() {
  if (!retryPayload.value) retryPayload.value = freezeBrandCreateRequest(form.value)
  const pending = retryPayload.value
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await api.createBrand(pending.body, pending.key)
    modal.value = false; confirming.value = false; retryPayload.value = null; notice.value = lang.value.created
    await loadWorkspace()
  } catch (cause) {
    if (cause instanceof PlatformApiError && (cause.network || cause.status >= 500 || cause.code === 'INVALID_RESPONSE')) error.value = `${messageOf(cause)} — ${lang.value.unknown}`
    else { handleFailure(cause); retryPayload.value = null }
  } finally { busy.value = false }
}
onMounted(() => { loadWorkspace().catch(() => {}) })
</script>

<template>
  <main v-if="!account" class="login-shell">
    <div class="login-card">
      <div class="brand-mark">N</div><div class="eyebrow">{{ lang.subtitle }}</div><h1>{{ lang.loginTitle }}</h1>
      <form @submit.prevent="login">
        <label>{{ lang.identifier }}<input v-model="identifier" autocomplete="username" required /></label>
        <label>{{ lang.password }}<input v-model="password" type="password" autocomplete="current-password" required /></label>
        <p v-if="error" class="message error">{{ error }}</p><button class="primary full" :disabled="busy">{{ busy ? lang.loading : lang.signIn }}</button>
      </form>
      <button class="language" @click="locale = locale === 'en' ? 'zh-CN' : 'en'">{{ locale === 'en' ? '中文' : 'English' }}</button>
    </div>
  </main>
  <div v-else class="app-frame">
    <aside class="sidebar">
      <div class="logo-row"><div class="brand-mark small">N</div><div><strong>{{ lang.title }}</strong><span>{{ lang.subtitle }}</span></div></div>
      <div class="nav-label">{{ lang.workspace }}</div>
      <button :class="['nav-item', { selected: section === 'brands' }]" @click="chooseSection('brands')"><span>◫</span>{{ lang.brands }}</button>
      <button :class="['nav-item', { selected: section === 'users' }]" @click="chooseSection('users')"><span>◉</span>{{ lang.users }}</button>
      <button :class="['nav-item', { selected: section === 'audit' }]" @click="chooseSection('audit')"><span>≋</span>{{ lang.audit }}</button>
      <button :class="['nav-item', { selected: section === 'rewards' }]" @click="chooseSection('rewards')"><span>◇</span>{{ lang.rewards }}</button>
      <button :class="['nav-item', { selected: section === 'wallets' }]" @click="chooseSection('wallets')"><span>◎</span>{{ walletLabel }}</button>
      <div class="sidebar-bottom"><p>{{ lang.platformNote }}</p></div>
    </aside>
    <section class="main-column">
      <header class="topbar"><div class="breadcrumb">{{ lang.title }} <span>/</span> {{ section === 'wallets' ? walletLabel : lang[section] }}</div><div class="top-actions"><span class="secure"><i></i> {{ lang.platformAdmin }}</span><button class="language" @click="locale = locale === 'en' ? 'zh-CN' : 'en'">{{ locale === 'en' ? '中文' : 'EN' }}</button><button class="logout-button" :disabled="busy" @click="signOut">↗ {{ lang.logout }}</button></div></header>
      <main class="content">
        <div v-if="error" class="message error global-message">{{ error }}</div><div v-if="notice" class="message success global-message">{{ notice }}</div>
        <div v-if="loading" class="loading-line">{{ lang.loading }}</div>
        <template v-if="section === 'brands'">
          <div class="page-heading"><div><div class="eyebrow">{{ lang.directory }}</div><h1>{{ lang.brands }}</h1><p>{{ lang.directoryDescription }}</p></div><button class="primary" @click="beginCreate"><span>＋</span> {{ lang.createBrand }}</button></div>
          <section class="panel"><div class="panel-heading"><div><h2>{{ lang.brandDirectory }}</h2><p>{{ brands.length }} {{ lang.brandCount }}</p></div><span class="count-chip">{{ brands.length }}</span></div>
            <div class="table-wrap"><table><thead><tr><th>{{ lang.name }}</th><th>{{ lang.code }}</th><th>{{ lang.status }}</th><th></th></tr></thead><tbody>
              <tr v-for="brand in brands" :key="brand.id" class="clickable-row" @click="openBrandMembers(brand.id)"><td><strong>{{ brand.name }}</strong><small>{{ brand.id }}</small></td><td class="mono">{{ brand.code }}</td><td><span :class="['status-pill', brand.status]">{{ statusLabel(brand.status) }}</span></td><td><button class="row-action" @click.stop="openBrandMembers(brand.id)">{{ lang.view }} →</button></td></tr>
              <tr v-if="!brands.length"><td colspan="4" class="empty-state">{{ lang.empty }}</td></tr>
            </tbody></table></div>
          </section>
        </template>
        <template v-else-if="section === 'users'">
          <div class="page-heading"><div><div class="eyebrow">{{ lang.accessDirectory }}</div><h1>{{ lang.users }}</h1><p>{{ lang.memberReadOnly }}</p></div></div>
          <div class="brand-picker"><label>{{ lang.brandLabel }}<select :value="selectedBrand" @change="chooseBrand(($event.target as HTMLSelectElement).value)"><option value="">— {{ lang.selectBrand }} —</option><option v-for="brand in brands" :key="brand.id" :value="brand.id">{{ brand.name }} · {{ brand.code }}</option></select></label><span v-if="selected" class="selection-tag">{{ selected.name }}</span></div>
          <section v-if="selectedBrand" class="panel"><div class="panel-heading"><div><h2>{{ selected?.name }} <span class="subtle">/ {{ lang.memberDirectory }}</span></h2><p>{{ lang.memberReadOnly }}</p></div><span class="count-chip">{{ thisPage }} {{ members.length }} {{ lang.memberCount }}</span></div>
            <div class="table-wrap desktop-table"><table><thead><tr><th>{{ lang.username }}</th><th>{{ lang.displayName }}</th><th>{{ lang.phone }}</th><th>{{ lang.status }}</th></tr></thead><tbody><tr v-for="member in members" :key="member.id"><td><strong>{{ member.username || '—' }}</strong><small>{{ member.id }}</small><button class="row-action" @click="openWallet(member.id)">{{ walletLabel }}</button></td><td>{{ member.display_name || '—' }}</td><td>{{ member.phone || '—' }}</td><td><span :class="['status-pill', member.status]">{{ statusLabel(member.status) }}</span></td></tr><tr v-if="!members.length"><td colspan="4" class="empty-state">{{ lang.empty }}</td></tr></tbody></table></div>
            <div class="mobile-cards"><article v-for="member in members" :key="member.id" class="user-card"><div class="card-title"><div><strong>{{ member.display_name || member.username || '—' }}</strong><small>{{ member.id }}</small></div><span :class="['status-pill', member.status]">{{ statusLabel(member.status) }}</span></div><dl class="member-details"><div><dt>{{ lang.username }}</dt><dd>{{ member.username || '—' }}</dd></div><div><dt>{{ lang.displayName }}</dt><dd>{{ member.display_name || '—' }}</dd></div><div><dt>{{ lang.phone }}</dt><dd>{{ member.phone || '—' }}</dd></div></dl><button class="row-action" @click="openWallet(member.id)">{{ walletLabel }}</button></article><div v-if="!members.length" class="empty-state">{{ lang.empty }}</div></div>
            <div class="reward-list-actions" data-testid="platform-member-pages">
              <button class="secondary" :disabled="loading || memberOffset === 0" @click="loadMembers(selectedBrand, memberOffset - pageSize)">{{ previousPage }}</button>
              <span>{{ Math.floor(memberOffset / pageSize) + 1 }}</span>
              <button class="secondary" :disabled="loading || !moreMembers" @click="loadMembers(selectedBrand, memberOffset + pageSize)">{{ nextPage }}</button>
              <button class="secondary" :disabled="loading" @click="loadMembers(selectedBrand)">{{ locale === 'en' ? 'Refresh' : '刷新' }}</button>
            </div>
          </section>
        </template>
        <template v-else-if="section === 'audit'">
          <div class="page-heading"><div><div class="eyebrow">{{ lang.traceability }}</div><h1>{{ lang.audit }}</h1><p>{{ lang.auditDescription }}</p></div></div>
          <div class="brand-picker"><label>{{ lang.brandLabel }}<select :value="selectedBrand" @change="chooseBrand(($event.target as HTMLSelectElement).value)"><option value="">— {{ lang.selectBrand }} —</option><option v-for="brand in brands" :key="brand.id" :value="brand.id">{{ brand.name }} · {{ brand.code }}</option></select></label><span v-if="selected" class="selection-tag">{{ selected.name }}</span></div>
          <section v-if="selectedBrand" class="panel"><div class="panel-heading"><div><h2>{{ lang.audit }} <span class="subtle">/ {{ selected?.name }}</span></h2><p>{{ lang.auditDescription }}</p></div><span class="count-chip">{{ auditRows.length }} {{ lang.countSuffix }}</span></div><div class="table-wrap"><table><thead><tr><th>{{ lang.action }}</th><th>{{ lang.actor }}</th><th>{{ lang.resource }}</th><th>{{ lang.reason }}</th><th>{{ lang.date }}</th></tr></thead><tbody><tr v-for="row in auditRows" :key="row.id"><td><span class="action-label">{{ row.action }}</span><small>{{ row.id }}</small></td><td class="mono">{{ row.actor_id }}</td><td>{{ row.resource_type }}<small>{{ row.resource_id }}</small></td><td class="reason-cell">{{ row.reason || '—' }}</td><td>{{ row.created_at }}</td></tr><tr v-if="!auditRows.length"><td colspan="5" class="empty-state">{{ lang.empty }}</td></tr></tbody></table></div></section>
          <div v-if="selectedBrand" class="reward-list-actions" data-testid="platform-audit-pages">
            <span>{{ thisPage }} {{ auditRows.length }}</span>
            <button class="secondary" :disabled="loading || auditOffset === 0" @click="loadAudit(selectedBrand, auditOffset - pageSize)">{{ previousPage }}</button>
            <span>{{ Math.floor(auditOffset / pageSize) + 1 }}</span>
            <button class="secondary" :disabled="loading || !moreAudit" @click="loadAudit(selectedBrand, auditOffset + pageSize)">{{ nextPage }}</button>
            <button class="secondary" :disabled="loading" @click="loadAudit(selectedBrand)">{{ locale === 'en' ? 'Refresh' : '刷新' }}</button>
          </div>
        </template>
        <template v-else-if="section === 'rewards'">
          <div class="brand-picker"><label>{{ lang.brandLabel }}<select :value="selectedBrand" @change="chooseBrand(($event.target as HTMLSelectElement).value)"><option value="">— {{ lang.selectBrand }} —</option><option v-for="brand in brands" :key="brand.id" :value="brand.id">{{ brand.name }} · {{ brand.code }}</option></select></label></div>
          <RewardsPanel :brand-id="selectedBrand" :brand-name="selected?.name || ''" :locale="locale" @failure="handleFailure" />
        </template>
        <template v-else>
          <div class="brand-picker"><label>{{ lang.brandLabel }}<select :value="selectedBrand" @change="chooseBrand(($event.target as HTMLSelectElement).value)"><option value="">— {{ lang.selectBrand }} —</option><option v-for="brand in brands" :key="brand.id" :value="brand.id">{{ brand.name }} · {{ brand.code }}</option></select></label></div>
          <WalletPanel :brand-id="selectedBrand" :member-id="walletMember" :locale="locale" @failure="handleFailure" />
        </template>
      </main>
    </section>
    <div v-if="modal" class="modal-scrim" @click.self="!busy && (modal = false)"><section class="modal-card" role="dialog" aria-modal="true" :aria-label="confirming ? lang.confirmation : lang.createBrand">
      <template v-if="!confirming"><div class="modal-heading"><div><div class="eyebrow">{{ lang.platformDirectory }}</div><h2>{{ lang.createBrand }}</h2></div><button class="icon-button" @click="modal = false">×</button></div><p class="modal-intro">{{ lang.createDescription }}</p>
        <form class="brand-form" @submit.prevent="reviewCreate"><label>{{ lang.code }}<input v-model.trim="form.code" required pattern="[a-z][a-z0-9_]{0,47}" :placeholder="lang.codePlaceholder" /></label><label>{{ lang.name }}<input v-model.trim="form.name" required maxlength="120" /></label><div class="form-row"><label>{{ lang.locale }}<select v-model="form.default_locale"><option value="en">{{ lang.localeEnglish }}</option><option value="zh-CN">{{ lang.localeChinese }}</option></select></label><label>{{ lang.timezone }}<input v-model.trim="form.timezone" required :placeholder="lang.timezonePlaceholder" /></label></div><label>{{ lang.reason }}<textarea v-model.trim="form.reason" required maxlength="500" rows="3"></textarea></label><div class="form-actions"><button type="button" class="secondary" @click="modal = false">{{ lang.cancel }}</button><button class="primary">{{ lang.review }} <span>→</span></button></div></form>
      </template><template v-else><div class="modal-heading"><div><div class="eyebrow">{{ lang.confirmation }}</div><h2>{{ lang.confirmation }}</h2></div><button class="icon-button" :disabled="busy" :aria-label="lang.close" @click="modal = false">×</button></div><p class="modal-intro">{{ lang.confirmCopy }}</p><dl class="confirm-list"><div><dt>{{ lang.code }}</dt><dd>{{ retryPayload?.body.code || form.code }}</dd></div><div><dt>{{ lang.name }}</dt><dd>{{ retryPayload?.body.name || form.name }}</dd></div><div><dt>{{ lang.locale }}</dt><dd>{{ retryPayload?.body.default_locale || form.default_locale }}</dd></div><div><dt>{{ lang.timezone }}</dt><dd>{{ retryPayload?.body.timezone || form.timezone }}</dd></div><div><dt>{{ lang.reason }}</dt><dd>{{ retryPayload?.body.reason || form.reason }}</dd></div></dl><p v-if="retryPayload && error" class="message error">{{ error }}</p><div class="form-actions"><button v-if="!retryPayload" class="secondary" :disabled="busy" @click="confirming = false">{{ lang.cancel }}</button><button class="primary" :disabled="busy" @click="submitCreate">{{ busy ? lang.loading : retryPayload ? lang.retry : lang.confirm }}</button></div></template>
    </section></div>
  </div>
</template>
