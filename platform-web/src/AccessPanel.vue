<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { createPlatformAccessApi, PlatformAccessApiError } from './access-api'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; scope: 'brand' | 'platform'; accountId: string; grants: string[] }>()
const emit = defineEmits<{ failure: [cause: unknown]; locked: [value: boolean] }>()
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
const ready = computed(() => props.scope === 'platform' || Boolean(props.brandId))
const canWriteAccounts = computed(() => props.grants.includes('admin.write.platform'))
const canWriteRoles = computed(() => props.scope === 'brand' && props.grants.includes('role.write.platform'))
type FormMode = '' | 'account-create' | 'account-edit' | 'password' | 'role-create' | 'role-edit'
const formMode = ref<FormMode>('')
const form = ref({ username: '', password: '', status: 'active' as 'active' | 'disabled', role_ids: [] as string[], code: '', name: '', permissions: [] as string[], reason: '' })
const formTarget = ref<AccessItem | null>(null)
const roleOptions = ref<AccessRole[]>([])
const permissionOptions = ref<string[]>([])
const optionsLoading = ref(false)
const optionsReady = ref(false)
const saving = ref(false)
const notice = ref('')
const pending = ref<null | { run: () => Promise<unknown> }>(null)
const locked = computed(() => saving.value || pending.value !== null)
const writeCopy = computed(() => props.locale === 'en' ? {
  create: 'New account', edit: 'Edit status and roles', reset: 'Reset password', createRole: 'New role', editRole: 'Edit role', password: 'Password (16–128 bytes)', reason: 'Reason', save: 'Save', cancel: 'Cancel', retry: 'Retry original request', saved: 'Saved. The account or role change is recorded in the audit log.', unknown: 'The request may have committed. Retry only this original request; do not submit a new one.', roleLimit: 'The selector shows up to 100 active roles.', chooseRoles: 'Assigned roles', self: 'You cannot edit or reset your own account here.',
} : {
  create: '新建账号', edit: '修改状态和角色', reset: '重置密码', createRole: '新建角色', editRole: '修改角色', password: '密码（16–128 字节）', reason: '操作原因', save: '保存', cancel: '取消', retry: '重试原请求', saved: '已保存，账号或角色变更已记录审计。', unknown: '请求可能已经提交。只能重试这笔原请求，不要另建请求。', roleLimit: '选择器最多显示100个启用角色。', chooseRoles: '分配角色', self: '不能在这里修改或重置自己的账号。',
})
const formTitle = computed(() => formMode.value === 'account-create' ? writeCopy.value.create : formMode.value === 'account-edit' ? writeCopy.value.edit : formMode.value === 'password' ? writeCopy.value.reset : formMode.value === 'role-create' ? writeCopy.value.createRole : writeCopy.value.editRole)
watch(locked, value => emit('locked', value), { immediate: true })

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
  if (!ready.value) { loading.value = false; return }
  loading.value = true
  try {
    if (currentTab === 'accounts') {
      const result = await (props.scope === 'platform' ? api.platformAccounts(51, currentOffset) : api.accounts(brand, 51, currentOffset))
      if (request !== generation || props.brandId !== brand || tab.value !== currentTab || offset.value !== currentOffset) return
      accounts.value = result.slice(0, 50)
      more.value = result.length > 50
    } else if (currentTab === 'roles') {
      const result = await (props.scope === 'platform' ? api.platformRoles(51, currentOffset) : api.roles(brand, 51, currentOffset))
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

async function beginForm(mode: FormMode, item: AccessItem | null = null) {
  if (locked.value || !ready.value) return
  formMode.value = mode; formTarget.value = item; notice.value = ''; error.value = ''
  form.value = { username: item && isAccount(item) ? item.username : '', password: '', status: item?.status ?? 'active', role_ids: item && isAccount(item) ? [...item.role_ids] : [], code: item && !isAccount(item) ? item.code : '', name: item && !isAccount(item) ? item.name : '', permissions: item && !isAccount(item) ? [...item.permissions] : [], reason: '' }
  roleOptions.value = []; permissionOptions.value = []; optionsReady.value = mode === 'password'
  if (mode === 'password') return
  const request = generation; optionsLoading.value = true
  try {
    if (mode === 'account-create' || mode === 'account-edit') {
      const result = await (props.scope === 'platform' ? api.platformRoles(100, 0) : api.roles(props.brandId, 100, 0))
      if (request !== generation) return
      roleOptions.value = result.filter(role => role.status === 'active' || form.value.role_ids.includes(role.id))
    } else {
      const result = await api.permissions(props.brandId)
      if (request !== generation) return
      permissionOptions.value = result
    }
    optionsReady.value = true
  } catch (cause) { if (request === generation) report(cause) }
  finally { if (request === generation) optionsLoading.value = false }
}

async function saveForm() {
  if (saving.value || !formMode.value || !optionsReady.value) return
  const request = generation
  if (!pending.value) {
    const key = crypto.randomUUID(), brand = props.brandId, target = formTarget.value, platform = props.scope === 'platform'
    const reason = form.value.reason.trim()
    if (formMode.value === 'account-create') {
      const body = { username: form.value.username, password: form.value.password, role_ids: [...form.value.role_ids], reason }
      pending.value = { run: () => platform ? api.createPlatformAccount(body, key) : api.createAccount(brand, body, key) }
    } else if (formMode.value === 'account-edit' && target && isAccount(target)) {
      const body = { version: target.version, status: form.value.status, role_ids: [...form.value.role_ids], reason }
      pending.value = { run: () => platform ? api.updatePlatformAccount(target.id, body, key) : api.updateAccount(brand, target.id, body, key) }
    } else if (formMode.value === 'password' && target && isAccount(target)) {
      const body = { version: target.version, password: form.value.password, reason }
      pending.value = { run: () => platform ? api.resetPlatformPassword(target.id, body, key) : api.resetPassword(brand, target.id, body, key) }
    } else if (formMode.value === 'role-create') {
      const body = { code: form.value.code, name: form.value.name, status: form.value.status, permissions: [...form.value.permissions], reason }
      pending.value = { run: () => api.createRole(brand, body, key) }
    } else if (formMode.value === 'role-edit' && target && !isAccount(target)) {
      const body = { version: target.version, name: form.value.name, status: form.value.status, permissions: [...form.value.permissions], reason }
      pending.value = { run: () => api.updateRole(brand, target.id, body, key) }
    }
  }
  if (!pending.value) return
  saving.value = true; error.value = ''
  try {
    await pending.value.run()
    if (request !== generation) return
    pending.value = null; formMode.value = ''; form.value.password = ''; notice.value = writeCopy.value.saved
    saving.value = false
    await load()
  } catch (cause) {
    if (request !== generation) return
    const unknown = cause instanceof PlatformAccessApiError && (cause.status >= 500 || cause.code === 'NETWORK_ERROR' || cause.code === 'INVALID_RESPONSE')
    if (!unknown) pending.value = null
    report(cause)
  } finally { if (request === generation || !pending.value) saving.value = false }
}

watch(() => [props.brandId, props.scope, props.accountId], () => { tab.value = 'accounts'; offset.value = 0; formMode.value = ''; form.value.password = ''; pending.value = null; notice.value = ''; optionsLoading.value = false }, { flush: 'sync' })
watch(() => [props.brandId, props.scope, props.accountId, tab.value, offset.value], () => { void load() }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation; form.value.password = ''; pending.value = null; emit('locked', false) })
</script>

<template>
  <div data-testid="platform-access">
    <div class="page-heading"><div><h1>{{ copy.title }}</h1></div></div>
    <div class="wallet-pagination" role="tablist" :aria-label="copy.title" data-testid="platform-access-tabs">
      <button :disabled="locked" :class="tab === 'accounts' ? 'primary' : 'secondary'" role="tab" :aria-selected="tab === 'accounts'" @click="chooseTab('accounts')">{{ copy.accounts }}</button>
      <button :disabled="locked" :class="tab === 'roles' ? 'primary' : 'secondary'" role="tab" :aria-selected="tab === 'roles'" @click="chooseTab('roles')">{{ copy.roles }}</button>
      <button v-if="scope === 'brand'" :disabled="locked" :class="tab === 'permissions' ? 'primary' : 'secondary'" role="tab" :aria-selected="tab === 'permissions'" @click="chooseTab('permissions')">{{ copy.permissionsTab }}</button>
      <button class="secondary" :disabled="loading || !ready || locked" @click="load">{{ copy.refresh }}</button>
      <button v-if="tab === 'accounts' && canWriteAccounts" class="primary" :disabled="!ready || loading || locked" @click="beginForm('account-create')">{{ writeCopy.create }}</button>
      <button v-if="tab === 'roles' && canWriteRoles" class="primary" :disabled="!ready || loading || locked" @click="beginForm('role-create')">{{ writeCopy.createRole }}</button>
    </div>
    <p v-if="error" class="message error" role="alert">{{ error }}</p>
    <p v-if="loading" class="loading-line" aria-live="polite">{{ copy.loading }}</p>
    <p v-if="notice" class="message success" role="status">{{ notice }}</p>
    <p v-if="!ready" class="wallet-pagination">{{ copy.noBrand }}</p>

    <template v-if="tab === 'accounts'">
      <section class="panel reward-detail" data-testid="platform-access-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.username }}</th><th>{{ copy.roles }}</th><th>{{ copy.status }}</th><th>{{ copy.superAdmin }}</th><th>{{ copy.version }}</th></tr></thead><tbody>
          <tr v-for="account in accounts" :key="account.id"><td><button class="row-action mono" :aria-label="`${copy.detail}: ${account.username}`" @click="selectAccount(account)">{{ account.username }}</button><small>{{ account.id }}</small></td><td><span v-for="roleCode in account.role_codes" :key="roleCode" class="mono">{{ roleCode }}<br></span><span v-if="!account.role_codes.length">—</span></td><td><span class="status-pill" :class="account.status">{{ stateLabel(account.status) }}</span></td><td>{{ account.super_admin ? copy.yes : copy.no }}</td><td>{{ account.version }}</td></tr>
          <tr v-if="!accounts.length"><td colspan="5" class="empty-state">{{ copy.emptyAccounts }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-access-pages"><button class="secondary" :disabled="loading || offset === 0 || !ready || locked" @click="page(offset - 50)">{{ copy.previous }}</button><span>{{ copy.page }} {{ Math.floor(offset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !more || locked" @click="page(offset + 50)">{{ copy.next }}</button></div>
      </section>
    </template>
    <template v-else-if="tab === 'roles'">
      <section class="panel reward-detail" data-testid="platform-access-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.name }}</th><th>{{ copy.code }}</th><th>{{ copy.status }}</th><th>{{ copy.version }}</th></tr></thead><tbody>
          <tr v-for="role in roles" :key="role.id"><td><button class="row-action mono" @click="selectRole(role)">{{ role.id }}</button><small>{{ role.name }}</small></td><td class="mono">{{ role.code }}</td><td><span class="status-pill" :class="role.status">{{ stateLabel(role.status) }}</span></td><td>{{ role.version }}</td></tr>
          <tr v-if="!roles.length"><td colspan="4" class="empty-state">{{ copy.emptyRoles }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-access-pages"><button class="secondary" :disabled="loading || offset === 0 || !ready || locked" @click="page(offset - 50)">{{ copy.previous }}</button><span>{{ copy.page }} {{ Math.floor(offset / 50) + 1 }}</span><button class="secondary" :disabled="loading || !more || locked" @click="page(offset + 50)">{{ copy.next }}</button></div>
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
      <div class="wallet-pagination">
        <template v-if="isAccount(selected) && canWriteAccounts">
          <button class="secondary" :disabled="locked || selected.id === accountId" @click="beginForm('account-edit', selected)">{{ writeCopy.edit }}</button>
          <button class="secondary" :disabled="locked || selected.id === accountId" @click="beginForm('password', selected)">{{ writeCopy.reset }}</button>
          <span v-if="selected.id === accountId">{{ writeCopy.self }}</span>
        </template>
        <button v-if="!isAccount(selected) && canWriteRoles" class="secondary" :disabled="locked || selected.is_bootstrap" @click="beginForm('role-edit', selected)">{{ writeCopy.editRole }}</button>
      </div>
    </section>
    <div v-if="formMode" class="modal-scrim">
      <section class="modal-card" role="dialog" aria-modal="true" :aria-label="formTitle">
        <h2>{{ formTitle }}</h2>
        <p v-if="formTarget">{{ isAccount(formTarget) ? formTarget.username : formTarget.name }} · {{ formTarget.id }}</p>
        <p v-if="optionsLoading">{{ copy.loading }}</p>
        <p v-if="pending" class="message" role="status">{{ writeCopy.unknown }}</p>
        <p v-if="error" class="message error" role="alert">{{ error }}</p>
        <form class="brand-form" @submit.prevent="saveForm">
          <fieldset :disabled="locked || optionsLoading">
            <label v-if="formMode === 'account-create'">{{ copy.username }}<input v-model="form.username" pattern="[a-z][a-z0-9_]{2,31}" required autocomplete="off" /></label>
            <label v-if="formMode === 'account-create' || formMode === 'password'">{{ writeCopy.password }}<input v-model="form.password" type="password" maxlength="128" required autocomplete="new-password" /></label>
            <label v-if="formMode === 'account-edit' || formMode === 'role-create' || formMode === 'role-edit'">{{ copy.status }}<select v-model="form.status"><option value="active">{{ stateLabel('active') }}</option><option value="disabled">{{ stateLabel('disabled') }}</option></select></label>
            <div v-if="formMode === 'account-create' || formMode === 'account-edit'" class="access-options">
              <p>{{ writeCopy.chooseRoles }} · {{ writeCopy.roleLimit }}</p>
              <label v-for="role in roleOptions" :key="role.id"><input v-model="form.role_ids" type="checkbox" :value="role.id" :disabled="role.status !== 'active' && !form.role_ids.includes(role.id)" /> {{ role.name }} · {{ role.code }} · {{ stateLabel(role.status) }}</label>
            </div>
            <label v-if="formMode === 'role-create'">{{ copy.code }}<input v-model="form.code" pattern="[a-z][a-z0-9_]{2,47}" required /></label>
            <label v-if="formMode === 'role-create' || formMode === 'role-edit'">{{ copy.name }}<input v-model="form.name" maxlength="120" required /></label>
            <div v-if="formMode === 'role-create' || formMode === 'role-edit'" class="access-options"><p>{{ copy.grants }}</p><label v-for="key in permissionOptions" :key="key"><input v-model="form.permissions" type="checkbox" :value="key" /> {{ key }}</label></div>
            <label>{{ writeCopy.reason }}<textarea v-model="form.reason" required maxlength="500" /></label>
          </fieldset>
          <div class="form-actions">
            <button type="button" class="secondary" :disabled="locked" @click="formMode = ''; form.password = ''">{{ writeCopy.cancel }}</button>
            <button type="submit" class="primary" :disabled="saving || !optionsReady || ((formMode === 'account-create' || formMode === 'account-edit') && !form.role_ids.length)">{{ saving ? copy.loading : pending ? writeCopy.retry : writeCopy.save }}</button>
          </div>
        </form>
      </section>
    </div>
  </div>
</template>

<style scoped>
fieldset { border: 0; padding: 0; margin: 0; display: grid; gap: 14px; min-width: 0; }
.access-options { max-height: 200px; overflow: auto; border: 1px solid #e5eaf0; border-radius: 6px; padding: 10px; }
.access-options label { display: flex; align-items: center; gap: 8px; overflow-wrap: anywhere; }
.access-options input { width: auto; min-height: 18px; flex-shrink: 0; }
.modal-card { max-height: 90vh; overflow-y: auto; }
textarea { width: 100%; min-height: 70px; resize: vertical; box-sizing: border-box; }
</style>
