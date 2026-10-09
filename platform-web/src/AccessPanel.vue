<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { createPlatformAccessApi, PlatformAccessApiError } from './access-api'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN' }>()
const emit = defineEmits<{ failure: [cause: unknown] }>()
const api = createPlatformAccessApi()
type Tab = 'accounts' | 'roles' | 'permissions'
type AccessAccount = Awaited<ReturnType<typeof api.accounts>>[number]
type AccessRole = Awaited<ReturnType<typeof api.roles>>[number]
type AccessItem = AccessAccount | AccessRole

const tab = ref<Tab>('accounts')
const accounts = ref<AccessAccount[]>([])
const roles = ref<AccessRole[]>([])
const permissions = ref<string[]>([])
const selected = ref<AccessItem | null>(null)
const offset = ref(0)
const more = ref(false)
const loading = ref(false)
const error = ref('')
let generation = 0

const copy = computed(() => props.locale === 'en' ? {
  title: 'Access and permissions', accounts: 'Accounts', roles: 'Roles', permissionsTab: 'Permission directory',
  loading: 'Loading…', emptyAccounts: 'No accounts found.', emptyRoles: 'No roles found.', emptyPermissions: 'No brand permission keys returned.',
  previous: 'Previous page', next: 'Next page', page: 'Page', refresh: 'Refresh', brand: 'Brand', id: 'ID', username: 'Username',
  name: 'Name', code: 'Code', status: 'Status', version: 'Version', superAdmin: 'Super admin', yes: 'Yes', no: 'No',
  roleIds: 'Role IDs', roleCodes: 'Role codes', globalBrands: 'Globally authorized brands', bootstrap: 'Bootstrap role',
  grants: 'Machine permission keys', detail: 'List snapshot details', snapshotNote: 'Details come from this loaded list snapshot. No single-record request was made.',
  directoryNote: 'This is the brand-scoped permission directory returned by the server. It is not the selected account’s grants.',
  noBrand: 'Select a brand to view access records.',
} : {
  title: '访问控制与权限', accounts: '账户', roles: '角色', permissionsTab: '权限目录',
  loading: '加载中…', emptyAccounts: '暂无账户。', emptyRoles: '暂无角色。', emptyPermissions: '接口未返回品牌权限键。',
  previous: '上一页', next: '下一页', page: '页码', refresh: '刷新', brand: '品牌', id: '编号', username: '用户名',
  name: '名称', code: '代码', status: '状态', version: '版本', superAdmin: '超级管理员', yes: '是', no: '否',
  roleIds: '角色编号', roleCodes: '角色代码', globalBrands: '全局授权品牌', bootstrap: '引导角色',
  grants: '机器权限键', detail: '列表快照详情', snapshotNote: '详情来自当前已加载的列表快照；没有请求单条记录接口。',
  directoryNote: '这是服务器返回的品牌范围权限目录，不代表当前账户已获授的权限。',
  noBrand: '请选择品牌以查看访问控制记录。',
})

function stateLabel(value: string): string {
  if (props.locale === 'en') {
    if (value === 'active') return 'Active'
    if (value === 'disabled') return 'Disabled'
    if (value === 'paused') return 'Paused'
    return value
  }
  if (value === 'active') return '启用'
  if (value === 'disabled') return '停用'
  if (value === 'paused') return '暂停'
  return value
}

function report(cause: unknown) {
  if (cause instanceof PlatformAccessApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else error.value = cause instanceof Error ? cause.message : String(cause)
}

async function load() {
  const request = ++generation
  const brand = props.brandId
  const currentTab = tab.value
  const currentOffset = offset.value
  accounts.value = []
  roles.value = []
  permissions.value = []
  selected.value = null
  more.value = false
  error.value = ''
  if (!brand) { loading.value = false; return }
  loading.value = true
  try {
    if (currentTab === 'accounts') {
      const result = await api.accounts(brand, 51, currentOffset)
      if (request !== generation || props.brandId !== brand || tab.value !== currentTab || offset.value !== currentOffset) return
      accounts.value = result.slice(0, 50)
      more.value = result.length > 50
    } else if (currentTab === 'roles') {
      const result = await api.roles(brand, 51, currentOffset)
      if (request !== generation || props.brandId !== brand || tab.value !== currentTab || offset.value !== currentOffset) return
      roles.value = result.slice(0, 50)
      more.value = result.length > 50
    } else {
      const result = await api.permissions(brand)
      if (request !== generation || props.brandId !== brand || tab.value !== currentTab) return
      permissions.value = result
    }
  } catch (cause) {
    if (request === generation && props.brandId === brand && tab.value === currentTab) report(cause)
  } finally {
    if (request === generation) loading.value = false
  }
}

function chooseTab(next: Tab) {
  if (tab.value === next) return
  tab.value = next
  offset.value = 0
  selected.value = null
  error.value = ''
}

function page(nextOffset: number) {
  offset.value = nextOffset
  selected.value = null
  error.value = ''
}

function selectAccount(item: AccessAccount) { selected.value = item }
function selectRole(item: AccessRole) { selected.value = item }
function isAccount(item: AccessItem): item is AccessAccount { return 'username' in item }

watch(() => [props.brandId, tab.value, offset.value], () => { void load() }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation })
</script>

<template>
  <div data-testid="platform-access">
    <div class="page-heading"><div><h1>{{ copy.title }}</h1></div></div>
    <div class="wallet-pagination" role="tablist" :aria-label="copy.title" data-testid="platform-access-tabs">
      <button :class="tab === 'accounts' ? 'primary' : 'secondary'" role="tab" :aria-selected="tab === 'accounts'" @click="chooseTab('accounts')">{{ copy.accounts }}</button>
      <button :class="tab === 'roles' ? 'primary' : 'secondary'" role="tab" :aria-selected="tab === 'roles'" @click="chooseTab('roles')">{{ copy.roles }}</button>
      <button :class="tab === 'permissions' ? 'primary' : 'secondary'" role="tab" :aria-selected="tab === 'permissions'" @click="chooseTab('permissions')">{{ copy.permissionsTab }}</button>
      <button class="secondary" :disabled="loading || !brandId" @click="load">{{ copy.refresh }}</button>
    </div>
    <p v-if="error" class="message error" role="alert">{{ error }}</p>
    <p v-if="loading" class="loading-line" aria-live="polite">{{ copy.loading }}</p>
    <p v-if="!brandId" class="wallet-pagination">{{ copy.noBrand }}</p>

    <template v-if="tab === 'accounts'">
      <section class="panel reward-detail" data-testid="platform-access-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.username }}</th><th>{{ copy.status }}</th><th>{{ copy.superAdmin }}</th><th>{{ copy.version }}</th></tr></thead><tbody>
          <tr v-for="account in accounts" :key="account.id"><td><button class="row-action mono" @click="selectAccount(account)">{{ account.id }}</button><small>{{ account.username }}</small></td><td><span class="status-pill" :class="account.status">{{ stateLabel(account.status) }}</span></td><td>{{ account.super_admin ? copy.yes : copy.no }}</td><td>{{ account.version }}</td></tr>
          <tr v-if="!accounts.length"><td colspan="4" class="empty-state">{{ copy.emptyAccounts }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-access-pages"><button class="secondary" :disabled="loading || offset === 0 || !brandId" @click="page(offset - 50)">{{ copy.previous }}</button><span>{{ copy.page }} {{ Math.floor(offset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !more" @click="page(offset + 50)">{{ copy.next }}</button></div>
      </section>
    </template>
    <template v-else-if="tab === 'roles'">
      <section class="panel reward-detail" data-testid="platform-access-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.name }}</th><th>{{ copy.code }}</th><th>{{ copy.status }}</th><th>{{ copy.version }}</th></tr></thead><tbody>
          <tr v-for="role in roles" :key="role.id"><td><button class="row-action mono" @click="selectRole(role)">{{ role.id }}</button><small>{{ role.name }}</small></td><td class="mono">{{ role.code }}</td><td><span class="status-pill" :class="role.status">{{ stateLabel(role.status) }}</span></td><td>{{ role.version }}</td></tr>
          <tr v-if="!roles.length"><td colspan="4" class="empty-state">{{ copy.emptyRoles }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-access-pages"><button class="secondary" :disabled="loading || offset === 0 || !brandId" @click="page(offset - 50)">{{ copy.previous }}</button><span>{{ copy.page }} {{ Math.floor(offset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !more" @click="page(offset + 50)">{{ copy.next }}</button></div>
      </section>
    </template>
    <template v-else>
      <section class="panel reward-detail" data-testid="platform-permission-directory">
        <div class="panel-heading"><div><h2>{{ copy.permissionsTab }}</h2><p>{{ copy.directoryNote }}</p></div><span class="count-chip">{{ permissions.length }}</span></div>
        <div class="table-wrap"><table><thead><tr><th>{{ copy.grants }}</th></tr></thead><tbody>
          <tr v-for="permission in permissions" :key="permission"><td class="mono">{{ permission }}</td></tr>
          <tr v-if="!permissions.length"><td class="empty-state">{{ copy.emptyPermissions }}</td></tr>
        </tbody></table></div>
      </section>
    </template>

    <section v-if="selected" class="panel reward-detail" data-testid="platform-access-detail">
      <div class="panel-heading"><div><h2>{{ copy.detail }}</h2><p>{{ copy.snapshotNote }}</p></div><span class="status-pill" :class="selected.status">{{ stateLabel(selected.status) }} · v{{ selected.version }}</span></div>
      <dl v-if="isAccount(selected)" class="confirm-list reward-fields">
        <div><dt>{{ copy.id }}</dt><dd class="mono">{{ selected.id }}</dd></div><div><dt>{{ copy.username }}</dt><dd>{{ selected.username }}</dd></div>
        <div><dt>{{ copy.status }}</dt><dd>{{ stateLabel(selected.status) }}</dd></div><div><dt>{{ copy.version }}</dt><dd>{{ selected.version }}</dd></div><div><dt>{{ copy.superAdmin }}</dt><dd>{{ selected.super_admin ? copy.yes : copy.no }}</dd></div>
        <div><dt>{{ copy.brand }}</dt><dd class="mono">{{ brandId }}</dd></div><div><dt>{{ copy.roleIds }} / {{ copy.roleCodes }}</dt><dd><span v-for="(roleId, index) in selected.role_ids" :key="roleId" class="mono">{{ roleId }} · {{ selected.role_codes[index] }}<br></span><span v-if="!selected.role_ids.length">—</span></dd></div>
        <div><dt>{{ copy.globalBrands }}</dt><dd><span v-for="id in selected.brand_ids" :key="id" class="mono">{{ id }}<br></span><span v-if="!selected.brand_ids.length">—</span></dd></div>
      </dl>
      <dl v-else-if="!isAccount(selected)" class="confirm-list reward-fields">
        <div><dt>{{ copy.id }}</dt><dd class="mono">{{ selected.id }}</dd></div><div><dt>{{ copy.brand }}</dt><dd class="mono">{{ selected.brand_id }}</dd></div>
        <div><dt>{{ copy.name }}</dt><dd>{{ selected.name }}</dd></div><div><dt>{{ copy.code }}</dt><dd class="mono">{{ selected.code }}</dd></div>
        <div><dt>{{ copy.status }}</dt><dd>{{ stateLabel(selected.status) }}</dd></div><div><dt>{{ copy.version }}</dt><dd>{{ selected.version }}</dd></div><div><dt>{{ copy.bootstrap }}</dt><dd>{{ selected.is_bootstrap ? copy.yes : copy.no }}</dd></div>
        <div><dt>{{ copy.grants }}</dt><dd><span v-for="permission in selected.permissions" :key="permission" class="mono">{{ permission }}<br></span><span v-if="!selected.permissions.length">—</span></dd></div>
      </dl>
    </section>
  </div>
</template>
