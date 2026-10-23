<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { AssociationName } from '@lottery/shared'
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vInlineValidation } from '@lottery/shared'
import { usePlatformI18n } from "./i18n"
const ui = usePlatformI18n().t
import DetailModal from "@lottery/shared/DetailModal.vue"
import PermissionPicker from "@lottery/shared/PermissionPicker.vue"
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { createPlatformAccessApi, PlatformAccessApiError } from './access-api'
import { permissionLabel } from '@lottery/shared'
import Toast from '@lottery/shared/Toast.vue'
import { platformErrorMessage } from './platform-error-message'

const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; scope: 'brand' | 'platform'; accountId: string; grants: string[]; initialTab?: Tab }>()
const emit = defineEmits<{ failure: [cause: unknown]; locked: [value: boolean] }>()
const api = createPlatformAccessApi()
type Tab = 'accounts' | 'roles'
type AccessAccount = Awaited<ReturnType<typeof api.accounts>>[number]
type AccessRole = Awaited<ReturnType<typeof api.roles>>[number]
type AccessItem = AccessAccount | AccessRole

const tab = ref<Tab>(props.initialTab ?? 'accounts')
const accounts = ref<AccessAccount[]>([])
const roles = ref<AccessRole[]>([])
const selected = ref<AccessItem | null>(null)
const offset = ref(0)
const more = ref(false)
const loading = ref(false)
type AccessError = string | { cause: unknown }
const error = ref<AccessError>('')
const errorToast = ref<AccessError>('')
let generation = 0
const ready = computed(() => props.scope === 'platform' || Boolean(props.brandId))
const canWriteAccounts = computed(() => props.grants.includes('admin.write.platform'))
const canDeleteAccounts = computed(() => props.grants.includes('admin.delete.platform'))
const canWriteRoles = computed(() => props.grants.includes('role.write.platform'))
const canDeleteRoles = computed(() => props.grants.includes('role.delete.brand') || props.grants.includes('role.delete.platform'))
type FormMode = '' | 'account-create' | 'account-edit' | 'account-delete' | 'password' | 'role-create' | 'role-edit' | 'role-delete'
const formMode = ref<FormMode>('')
const form = ref({ username: '', password: '', status: 'active' as 'active' | 'disabled', role_ids: [] as string[], code: '', name: '', permissions: [] as string[] })
const formTarget = ref<AccessItem | null>(null)
const roleOptions = ref<AccessRole[]>([])
const permissionOptions = ref<string[]>([])
const optionsLoading = ref(false)
const optionsReady = ref(false)
const saving = ref(false)
const notice = ref<{ key: string; name: string } | null>(null)
const pending = ref<null | { run: () => Promise<unknown> }>(null)
const locked = computed(() => saving.value || pending.value !== null)
const writeCopy = computed(() => props.locale === 'en' ? {
  create: ui('platform.copy.1f19a013'), edit: ui('platform.copy.497d7839'), reset: ui('platform.copy.6dbd20b5'), delete: ui('platform.copy.9851501a'), actions: ui('platform.copy.412ae2c3'), createRole: ui('platform.copy.8aa1740f'), editRole: ui('platform.copy.26c6cda4'), password: ui('platform.copy.cb6bdf7d'), save: ui('platform.copy.57b298e5'), cancel: ui('platform.copy.2230b1a7'), retry: ui('platform.copy.0ffed4b3'), saved: ui('platform.copy.33341f71'), unknown: ui('common.uncertainRequest'), deleteUnknown: ui('common.uncertainRequest'), roleNameLocked: ui('common.bootstrapRoleNameLocked'), roleLimit: ui('platform.copy.b1e44031'), chooseRoles: ui('platform.copy.c4aa8d0d'), self: ui('platform.copy.d7d2e55c'),
} : {
  create: ui('platform.copy.1f19a013'), edit: ui('platform.copy.497d7839'), reset: ui('platform.copy.6dbd20b5'), delete: ui('platform.copy.9851501a'), actions: ui('platform.copy.412ae2c3'), createRole: ui('platform.copy.8aa1740f'), editRole: ui('platform.copy.26c6cda4'), password: ui('platform.copy.cb6bdf7d'), save: ui('platform.copy.57b298e5'), cancel: ui('platform.copy.2230b1a7'), retry: ui('platform.copy.0ffed4b3'), saved: ui('platform.copy.33341f71'), unknown: ui('common.uncertainRequest'), deleteUnknown: ui('common.uncertainRequest'), roleNameLocked: ui('common.bootstrapRoleNameLocked'), roleLimit: ui('platform.copy.b1e44031'), chooseRoles: ui('platform.copy.c4aa8d0d'), self: ui('platform.copy.d7d2e55c'),
})
const formTitle = computed(() => formMode.value === 'account-create' ? writeCopy.value.create : formMode.value === 'account-edit' ? writeCopy.value.edit : formMode.value === 'account-delete' || formMode.value === 'role-delete' ? writeCopy.value.delete : formMode.value === 'password' ? writeCopy.value.reset : formMode.value === 'role-create' ? writeCopy.value.createRole : writeCopy.value.editRole)
watch(locked, value => emit('locked', value), { immediate: true })

const copy = computed(() => props.locale === 'en' ? {
  title: ui('platform.copy.e44a8e33'), accounts: ui('platform.copy.198d86ac'), roles: ui('platform.copy.94ddefde'),
  loading: ui('platform.copy.fa02e7b1'), emptyAccounts: ui('platform.copy.c518e2f9'), emptyRoles: ui('platform.copy.c196b15b'), emptyPermissions: ui('platform.copy.698dfd7b'),
  previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), page: ui('platform.copy.6695ff1c'), refresh: ui('platform.copy.256fe8b3'), brand: ui('platform.copy.d5b68c0f'), id: ui('platform.copy.55d8fcaf'), username: ui('platform.copy.eeaa7275'),
  name: ui('platform.copy.00d0ada7'), code: ui('platform.copy.ba1081a4'), status: ui('platform.copy.c7260510'), superAdmin: ui('platform.copy.432a5216'), yes: ui('platform.copy.ab57ddad'), no: ui('platform.copy.502adcd8'),
  roleIds: ui('platform.copy.70143702'), roleCodes: ui('platform.copy.a22eb37d'), globalBrands: ui('platform.copy.21a5fdcc'), bootstrap: ui('platform.copy.597ea988'),
  grants: ui('platform.copy.143b5c26'), detail: ui('platform.copy.b14bf4ef'), accountDetail: ui('platform.copy.7f98c4a1'), roleDetail: ui('platform.copy.4baf6e21'), close: ui('platform.copy.27b2fd29'), selectAll: ui('platform.copy.65d89eae'), clear: ui('platform.copy.2e88de26'),
} : {
  title: ui('platform.copy.e44a8e33'), accounts: ui('platform.copy.198d86ac'), roles: ui('platform.copy.94ddefde'),
  loading: ui('platform.copy.fa02e7b1'), emptyAccounts: ui('platform.copy.c518e2f9'), emptyRoles: ui('platform.copy.c196b15b'), emptyPermissions: ui('platform.copy.698dfd7b'),
  previous: ui('platform.copy.7f5aba9a'), next: ui('platform.copy.282be575'), page: ui('platform.copy.6695ff1c'), refresh: ui('platform.copy.256fe8b3'), brand: ui('platform.copy.d5b68c0f'), id: ui('platform.copy.55d8fcaf'), username: ui('platform.copy.eeaa7275'),
  name: ui('platform.copy.00d0ada7'), code: ui('platform.copy.ba1081a4'), status: ui('platform.copy.c7260510'), superAdmin: ui('platform.copy.432a5216'), yes: ui('platform.copy.ab57ddad'), no: ui('platform.copy.502adcd8'),
  roleIds: ui('platform.copy.70143702'), roleCodes: ui('platform.copy.a22eb37d'), globalBrands: ui('platform.copy.21a5fdcc'), bootstrap: ui('platform.copy.597ea988'),
  grants: ui('platform.copy.143b5c26'), detail: ui('platform.copy.b14bf4ef'), accountDetail: ui('platform.copy.7f98c4a1'), roleDetail: ui('platform.copy.4baf6e21'), close: ui('platform.copy.27b2fd29'), selectAll: ui('platform.copy.65d89eae'), clear: ui('platform.copy.2e88de26'),
})

function stateLabel(value: string): string {
  if (value === 'active') return ui('platform.access.active')
  if (value === 'disabled') return ui('platform.access.disabled')
  if (value === 'paused') return ui('platform.access.paused')
  return value
}

function displayError(value: AccessError): string {
  return typeof value === 'string' ? value : platformErrorMessage(value.cause, props.locale)
}

function showError(message: AccessError) {
  error.value = message
  errorToast.value = message
}

function report(cause: unknown) {
  if (cause instanceof PlatformAccessApiError && (cause.status === 401 || cause.code === 'PLATFORM_ADMIN_REQUIRED')) emit('failure', cause)
  else showError({ cause })
}

async function load() {
  const request = ++generation
  const brand = props.brandId
  const currentTab = tab.value
  const currentOffset = offset.value
  accounts.value = []
  roles.value = []
  selected.value = null
  more.value = false
  error.value = ''; errorToast.value = ''
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
  errorToast.value = ''
  if ((mode === 'role-edit' || mode === 'role-delete') && item && !isAccount(item) && item.is_bootstrap) { showError(ui('association.builtinReadOnly')); return }
  if (!ready.value) { showError(ui('platform.error.BRAND_REQUIRED')); return }
  if (locked.value) { showError(ui('common.uncertainRequest')); return }
  if (loading.value) { showError(ui('access.waitForLoad')); return }
  formMode.value = mode; formTarget.value = item; notice.value = null; error.value = ''
  form.value = { username: item && isAccount(item) ? item.username : '', password: '', status: item?.status ?? 'active', role_ids: item && isAccount(item) ? [...item.role_ids] : [], code: item && !isAccount(item) ? item.code : '', name: item && !isAccount(item) ? item.name : '', permissions: item && !isAccount(item) ? [...item.permissions] : [] }
  roleOptions.value = []; permissionOptions.value = []; optionsReady.value = mode === 'password' || mode === 'account-delete' || mode === 'role-delete'
  if (mode === 'password' || mode === 'account-delete' || mode === 'role-delete') return
  const request = generation; optionsLoading.value = true
  try {
    if (mode === 'account-create' || mode === 'account-edit') {
      const result = await (props.scope === 'platform' ? api.platformRoles(100, 0) : api.roles(props.brandId, 100, 0))
      if (request !== generation) return
      roleOptions.value = result.filter(role => role.status === 'active' || form.value.role_ids.includes(role.id))
      if (!roleOptions.value.length) showError(ui('access.noAssignableRoles'))
    } else {
      const result = props.scope === 'platform' ? await api.platformPermissions() : await api.permissions(props.brandId)
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
    if (formMode.value === 'account-create') {
      const body = { username: form.value.username, password: form.value.password, role_ids: [...form.value.role_ids] }
      pending.value = { run: () => platform ? api.createPlatformAccount(body, key) : api.createAccount(brand, body, key) }
    } else if (formMode.value === 'account-edit' && target && isAccount(target)) {
      const body = { version: target.version, status: form.value.status, role_ids: [...form.value.role_ids] }
      pending.value = { run: () => platform ? api.updatePlatformAccount(target.id, body, key) : api.updateAccount(brand, target.id, body, key) }
    } else if (formMode.value === 'password' && target && isAccount(target)) {
      const body = { version: target.version, password: form.value.password }
      pending.value = { run: () => platform ? api.resetPlatformPassword(target.id, body, key) : api.resetPassword(brand, target.id, body, key) }
    } else if (formMode.value === 'account-delete' && target && isAccount(target)) {
      const body = { version: target.version }
      pending.value = { run: () => platform ? api.deletePlatformAccount(target.id, body, key) : api.deleteAccount(brand, target.id, body, key) }
    } else if (formMode.value === 'role-delete' && target && !isAccount(target)) {
      const body = { version: target.version }
      pending.value = { run: () => platform ? api.deletePlatformRole(target.id, body, key) : api.deleteRole(brand, target.id, body, key) }
    } else if (formMode.value === 'role-create') {
      const body = { code: form.value.code, name: form.value.name, status: form.value.status, permissions: [...form.value.permissions] }
      pending.value = { run: () => platform ? api.createPlatformRole(body, key) : api.createRole(brand, body, key) }
    } else if (formMode.value === 'role-edit' && target && !isAccount(target)) {
      const body = { version: target.version, name: form.value.name, status: form.value.status, permissions: [...form.value.permissions] }
      pending.value = { run: () => platform ? api.updatePlatformRole(target.id, body, key) : api.updateRole(brand, target.id, body, key) }
    }
  }
  if (!pending.value) return
  saving.value = true; error.value = ''; errorToast.value = ''
  try {
    await pending.value.run()
    if (request !== generation) return
    const target = formTarget.value
    notice.value = formMode.value === 'account-delete' && target && isAccount(target)
      ? { key: 'access.accountDeleted', name: target.username }
      : formMode.value === 'role-delete' && target && !isAccount(target)
        ? { key: 'access.roleDeleted', name: target.name }
        : { key: 'platform.copy.33341f71', name: '' }
    pending.value = null; formMode.value = ''; form.value.password = ''
    saving.value = false
    await load()
  } catch (cause) {
    if (request !== generation) return
    const unknown = cause instanceof PlatformAccessApiError && (cause.status >= 500 || cause.code === 'NETWORK_ERROR' || cause.code === 'INVALID_RESPONSE')
    if (!unknown) pending.value = null
    report(cause)
  } finally { if (request === generation || !pending.value) saving.value = false }
}

watch(() => [props.brandId, props.scope, props.accountId, props.initialTab], () => { tab.value = props.initialTab ?? 'accounts'; offset.value = 0; formMode.value = ''; form.value.password = ''; pending.value = null; notice.value = null; errorToast.value = ''; optionsLoading.value = false }, { flush: 'sync' })
watch(() => [props.brandId, props.scope, props.accountId, tab.value, offset.value], () => { void withGlobalLoading(load) }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation; form.value.password = ''; pending.value = null; emit('locked', false) })
</script>

<template>
  <div data-testid="platform-access">
    <div class="wallet-pagination" role="tablist" :aria-label="copy.title" data-testid="platform-access-tabs">
      <button :disabled="locked" :class="tab === 'accounts' ? 'button button-primary' : 'button button-secondary'" role="tab" :aria-selected="tab === 'accounts'" @click="chooseTab('accounts')">{{ copy.accounts }}</button>
      <button :disabled="locked" :class="tab === 'roles' ? 'button button-primary' : 'button button-secondary'" role="tab" :aria-selected="tab === 'roles'" @click="chooseTab('roles')">{{ copy.roles }}</button>
      <button v-if="tab === 'accounts' && canWriteAccounts" class="button button-primary" @click="withGlobalLoading(beginForm, 'account-create')">{{ writeCopy.create }}</button>
      <button v-if="tab === 'roles' && canWriteRoles" class="button button-primary" @click="withGlobalLoading(beginForm, 'role-create')">{{ writeCopy.createRole }}</button>
    </div>
    <p v-if="error" class="message error" role="alert">{{ displayError(error) }}</p>
    <Toast v-if="errorToast" tone="error" :title="ui('common.operationFailed')" :message="displayError(errorToast)" :close-label="copy.close" @dismiss="errorToast = ''" />
    <Toast v-else-if="notice" :title="ui('common.operationSuccess')" :message="ui(notice.key, { name: notice.name })" :close-label="copy.close" @dismiss="notice = null" />
    <template v-if="tab === 'accounts'">
      <section class="panel reward-detail" data-testid="platform-access-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.username }}</th><th>{{ copy.roles }}</th><th><BusinessDetails :value="copy.status" :locale="props.locale" /></th><th v-if="canDeleteAccounts" class="access-actions">{{ writeCopy.actions }}</th></tr></thead><tbody>
          <tr v-for="account in accounts" :key="account.id"><td><button class="row-action" :aria-label="`${copy.detail}: ${account.username}`" @click="selectAccount(account)">{{ account.username }}</button></td><td><span v-for="roleId in account.role_ids" :key="roleId"><AssociationName :record="account" kind="role" :id="roleId" :locale="locale" /><br></span><span v-if="!account.role_ids.length">—</span></td><td><span class="status-pill" :class="account.status">{{ stateLabel(account.status) }}</span></td><td v-if="canDeleteAccounts" class="access-actions"><button v-if="account.id !== accountId" class="row-action danger-text" @click.stop="withGlobalLoading(beginForm, 'account-delete', account)">{{ writeCopy.delete }}</button></td></tr>
          <tr v-if="!accounts.length"><td :colspan="3 + (canDeleteAccounts ? 1 : 0)" class="empty-state">{{ copy.emptyAccounts }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-access-pages"><button class="button button-secondary" :disabled="loading || offset === 0 || !ready || locked" @click="page(offset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !more || locked" @click="page(offset + 50)">{{ copy.next }}</button><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !ready || locked" @click="withGlobalLoading(load)">{{ copy.refresh }}</button></div>
      </section>
    </template>
    <template v-else-if="tab === 'roles'">
      <section class="panel reward-detail" data-testid="platform-access-list">
        <div class="table-wrap"><table><thead><tr><th>{{ copy.name }}</th><th><BusinessDetails :value="copy.status" :locale="props.locale" /></th><th v-if="canDeleteRoles" class="access-actions">{{ writeCopy.actions }}</th></tr></thead><tbody>
          <tr v-for="role in roles" :key="role.id"><td><button class="row-action" @click="selectRole(role)"><AssociationName :record="role" kind="role" :id="role.id" :locale="locale" /></button></td><td><span class="status-pill" :class="role.status">{{ stateLabel(role.status) }}</span></td><td v-if="canDeleteRoles" class="access-actions"><button v-if="!role.is_bootstrap" class="row-action danger-text" @click.stop="withGlobalLoading(beginForm, 'role-delete', role)">{{ writeCopy.delete }}</button></td></tr>
          <tr v-if="!roles.length"><td :colspan="canDeleteRoles ? 3 : 2" class="empty-state">{{ copy.emptyRoles }}</td></tr>
        </tbody></table></div>
        <div class="wallet-pagination" data-testid="platform-access-pages"><button class="button button-secondary" :disabled="loading || offset === 0 || !ready || locked" @click="page(offset - 50)">{{ copy.previous }}</button><span>{{ Math.floor(offset / 50) + 1 }}</span><button class="button button-secondary" :disabled="loading || !more || locked" @click="page(offset + 50)">{{ copy.next }}</button><button v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !ready || locked" @click="withGlobalLoading(load)">{{ copy.refresh }}</button></div>
      </section>
    </template>
    <DetailModal v-if="selected" :title="isAccount(selected) ? copy.accountDetail : copy.roleDetail" :close-label="copy.close" :open="!formMode" @close="selected = null">
<section v-if="selected" class="detail-section" data-testid="platform-access-detail">
      <dl v-if="isAccount(selected)" class="confirm-list reward-fields">
        <div><dt>{{ copy.username }}</dt><dd>{{ selected.username }}</dd></div>
        <div><dt><BusinessDetails :value="copy.status" :locale="props.locale" /></dt><dd>{{ stateLabel(selected.status) }}</dd></div>
        <div><dt>{{ copy.roles }}</dt><dd><span v-for="roleId in selected.role_ids" :key="roleId"><AssociationName :record="selected" kind="role" :id="roleId" :locale="locale" /><br></span><span v-if="!selected.role_ids.length">—</span></dd></div>
        <div v-if="selected.brand_ids.length"><dt>{{ copy.globalBrands }}</dt><dd><span v-for="id in selected.brand_ids" :key="id"><AssociationName :record="selected" kind="brand" :id="id" :locale="locale" /><br></span></dd></div>
      </dl>
      <dl v-else-if="!isAccount(selected)" class="confirm-list reward-fields role-detail-fields">
        <div><dt>{{ copy.name }}</dt><dd><AssociationName :record="selected" kind="role" :id="selected.id" :locale="locale" /></dd></div>
        <div><dt><BusinessDetails :value="copy.status" :locale="props.locale" /></dt><dd>{{ stateLabel(selected.status) }}</dd></div>
        <div class="role-permissions-field"><dt>{{ copy.grants }}</dt><dd><div v-if="selected.permissions.length" class="permission-grid"><span v-for="permission in selected.permissions" :key="permission" class="permission-chip">{{ permissionLabel(permission, locale) }}</span></div><span v-else>—</span></dd></div>
      </dl>
    </section>
    <template #footer v-if="(isAccount(selected) && canWriteAccounts && selected.id !== accountId) || (!isAccount(selected) && !selected.is_bootstrap && canWriteRoles)">
      <template v-if="isAccount(selected) && canWriteAccounts && selected.id !== accountId">
        <button class="button button-secondary" :disabled="locked" @click="withGlobalLoading(beginForm, 'account-edit', selected)">{{ writeCopy.edit }}</button>
        <button class="button button-secondary" :disabled="locked" @click="withGlobalLoading(beginForm, 'password', selected)">{{ writeCopy.reset }}</button>
        <button v-if="canDeleteAccounts" class="button button-danger" :disabled="locked" @click="withGlobalLoading(beginForm, 'account-delete', selected)">{{ writeCopy.delete }}</button>
      </template>
      <button v-if="!isAccount(selected) && !selected.is_bootstrap && canWriteRoles" class="button button-secondary" :disabled="locked" @click="withGlobalLoading(beginForm, 'role-edit', selected)">{{ writeCopy.editRole }}</button>
    </template>
</DetailModal>
    <div v-if="formMode" class="modal-scrim">
      <section class="modal-card" :class="{ 'access-role-modal': formMode === 'role-create' || formMode === 'role-edit', 'access-delete-modal': formMode === 'account-delete' || formMode === 'role-delete' }" role="dialog" aria-modal="true" :aria-label="formTitle">
        <h2>{{ formTitle }}</h2>
        <p v-if="formTarget">{{ isAccount(formTarget) ? formTarget.username : formTarget.name }}</p>
        <p v-if="pending && !saving" class="message" role="status">{{ formMode === 'account-delete' ? writeCopy.deleteUnknown : writeCopy.unknown }}</p>
        <form v-write-guard="saving" novalidate v-inline-validation class="brand-form" @submit.prevent="withGlobalLoading(saveForm)">
          <div class="access-form-fields">
          <fieldset :disabled="locked || optionsLoading">
            <label v-if="formMode === 'account-create'">{{ copy.username }}<input v-model="form.username" pattern="[a-z][a-z0-9_]{2,31}" required autocomplete="off" /></label>
            <label v-if="formMode === 'account-create' || formMode === 'password'">{{ writeCopy.password }}<input v-model="form.password" type="password" maxlength="128" required autocomplete="new-password" /></label>
            <label v-if="formMode === 'account-edit'"><BusinessDetails :value="copy.status" :locale="props.locale" /><select v-model="form.status"><option value="active">{{ stateLabel('active') }}</option><option value="disabled">{{ stateLabel('disabled') }}</option></select></label>
            <div v-if="formMode === 'account-create' || formMode === 'account-edit'" class="access-options">
              <p>{{ writeCopy.chooseRoles }} · {{ writeCopy.roleLimit }}</p>
              <label v-for="role in roleOptions" :key="role.id"><input v-model="form.role_ids" type="checkbox" :value="role.id" :disabled="role.status !== 'active' && !form.role_ids.includes(role.id)" /> <AssociationName :record="role" kind="role" :id="role.id" :locale="locale" /> · {{ stateLabel(role.status) }}</label>
            </div>
            <div v-if="formMode === 'role-create' || formMode === 'role-edit'" class="role-identity">
              <label>{{ copy.name }}<input v-model="form.name" maxlength="120" required /></label>
              <label>{{ copy.code }}<input v-model="form.code" pattern="[a-z][a-z0-9_]{2,47}" required :readonly="formMode === 'role-edit'" :class="{'readonly-field': formMode === 'role-edit'}" /></label>
              <label><BusinessDetails :value="copy.status" :locale="props.locale" /><select v-model="form.status"><option value="active">{{ stateLabel('active') }}</option><option value="disabled">{{ stateLabel('disabled') }}</option></select></label>
            </div>
            <PermissionPicker v-if="formMode === 'role-create' || formMode === 'role-edit'" v-model="form.permissions" :options="permissionOptions" :locale="locale" />
          </fieldset>
          </div>
          <div class="form-actions">
            <p v-if="error" class="message error inline-validation-error" role="alert">{{ displayError(error) }}</p>
            <button type="button" class="button button-secondary" :disabled="locked" @click="formMode = ''; form.password = ''">{{ writeCopy.cancel }}</button>
            <button type="submit" :class="formMode === 'account-delete' || formMode === 'role-delete' ? 'button button-danger' : 'button button-primary'" :disabled="saving || !optionsReady || ((formMode === 'account-create' || formMode === 'account-edit') && !form.role_ids.length)">{{ saving ? copy.loading : pending ? writeCopy.retry : formMode === 'account-delete' || formMode === 'role-delete' ? writeCopy.delete : writeCopy.save }}</button>
          </div>
        </form>
      </section>
    </div>
  </div>
</template>

<style scoped>
.table-wrap table { table-layout: fixed; white-space: normal; }
.table-wrap :is(th, td) { position: static; min-width: 0; overflow-wrap: anywhere; }
.table-wrap .row-action { max-width: 100%; white-space: normal; overflow-wrap: anywhere; }
fieldset { border: 0; padding: 0; margin: 0; display: grid; gap: 14px; min-width: 0; }
.access-options { max-height: 200px; overflow: auto; border: 1px solid #e5eaf0; border-radius: 6px; padding: 10px; }
.access-options label { display: flex; flex-direction: row; align-items: center; gap: 8px; overflow-wrap: anywhere; }
.access-options input { width: auto; min-height: 18px; flex-shrink: 0; }
.modal-card { max-height: 90vh; overflow: hidden; display: flex; flex-direction: column; }
.access-delete-modal { width: min(480px, 100%); padding: 18px 20px 16px; }
.access-delete-modal > h2 { margin: 0 0 12px; }
.access-delete-modal > p { margin: 0 0 12px; color: var(--text-muted); }
.access-delete-modal .brand-form { gap: 10px; }
.access-role-modal { width: min(1000px, 100%); max-width: 100%; padding: 0 24px 20px; }
.access-role-modal fieldset > label { min-width: 0; }
.access-role-modal fieldset > label :is(input, select) { min-width: 0; width: 100%; }
.access-role-modal > p { margin: 0 0 8px; }
.access-role-modal fieldset { grid-template-columns: minmax(0, 1fr); gap: 14px; }
.role-identity { display:grid; grid-template-columns:minmax(0,1fr) minmax(0,1fr) minmax(130px,.55fr); gap:12px; }
.role-identity label { display:grid; gap:6px; min-width:0; font-size:13px; }
.role-identity :is(input,select) { min-width:0; width:100%; min-height:44px; }
.modal-card > .brand-form { min-height: 0; overflow: hidden; display: flex; flex-direction: column; }
.modal-card > .brand-form > .access-form-fields { min-height: 0; overflow-y: auto; padding-right: 4px; }
.modal-card > .brand-form > .form-actions { flex: 0 0 auto; margin: 0; padding-top: 10px; border-top: 1px solid var(--border-light); background: var(--bg-surface); }
.access-role-modal > .brand-form > .access-form-fields { overflow-y: scroll; scrollbar-gutter: stable; }
input.readonly-field { background: var(--bg-subtle); color: var(--text-muted); cursor: not-allowed; }
textarea { width: 100%; min-height: 56px; resize: vertical; box-sizing: border-box; }
.access-actions { text-align: right; }
.role-detail-fields { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.role-permissions-field { grid-column: 1 / -1; }
.permission-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); gap: 6px; }
.permission-chip { display: flex; min-width: 0; align-items: center; gap: 8px; padding: 5px 7px; border: 1px solid var(--border-light); border-radius: var(--radius-sm); background: var(--bg-subtle); font-size: 12px; font-weight: 600; line-height: 1.35; overflow-wrap: anywhere; }
.permission-chip input { width: auto; min-height: 18px; margin: 0; flex: 0 0 auto; }
.access-role-modal .access-options { max-height: none; padding: 6px; }
@media (max-width: 720px) { .role-detail-fields { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 480px) { .role-detail-fields { grid-template-columns: 1fr; } .role-permissions-field { grid-column: auto; } }
@media (max-width: 900px) {
  .access-role-modal :is(button, select), .access-role-modal fieldset > label input { min-height: 44px; }
  .access-role-modal .permission-chip { min-height: 44px; }
}
@media (max-width: 620px) {
  .access-role-modal { padding: 0 16px 16px; }
  .access-role-modal fieldset { grid-template-columns: minmax(0, 1fr); }
  .role-identity { grid-template-columns:minmax(0,1fr); }
}
</style>
