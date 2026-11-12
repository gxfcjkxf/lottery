<script setup lang="ts">
import { readPaginationTotal } from '@lottery/shared'
const recordTotal = ref('0')

import ManagementList from "@lottery/shared/ManagementList.vue"
import BatchDelete from "@lottery/shared/BatchDelete.vue"
import MemberActionsMenu from "@lottery/shared/MemberActionsMenu.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import { formValidationText } from '@lottery/shared'
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
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { createPlatformAccessApi, PlatformAccessApiError } from './access-api'
import { permissionLabel } from '@lottery/shared'
import { platformErrorMessage } from './platform-error-message'
const pageSize = ref(50)


const props = defineProps<{ brandId: string; locale: 'en' | 'zh-CN'; scope: 'brand' | 'platform'; accountId: string; grants: string[]; initialTab?: Tab }>()
const emit = defineEmits<{ failure: [cause: unknown]; locked: [value: boolean] }>()
const api = createPlatformAccessApi()
type Tab = 'accounts' | 'roles'
type AccessAccount = Awaited<ReturnType<typeof api.accounts>>['items'][number]
type AccessRole = Awaited<ReturnType<typeof api.roles>>['items'][number]
type AccessItem = AccessAccount | AccessRole

const tab = ref<Tab>(props.initialTab ?? 'accounts')
const accounts = ref<AccessAccount[]>([])
const roles = ref<AccessRole[]>([])
const entries = computed<AccessItem[]>(() => tab.value === 'accounts' ? accounts.value : roles.value)
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
const batchOpen = ref(false)
const locked = computed(() => saving.value || pending.value !== null || batchOpen.value)
const writeCopy = computed(() => props.locale === 'en' ? {
  create: ui('platform.copy.1f19a013'), edit: ui('platform.copy.497d7839'), reset: ui('platform.copy.6dbd20b5'), delete: ui('platform.copy.9851501a'), actions: ui('platform.copy.412ae2c3'), createRole: ui('platform.copy.8aa1740f'), editRole: ui('platform.copy.26c6cda4'), password: ui('platform.copy.cb6bdf7d'), save: ui('platform.copy.57b298e5'), cancel: ui('platform.copy.2230b1a7'), retry: ui('platform.copy.0ffed4b3'), saved: ui('platform.copy.33341f71'), unknown: ui('common.uncertainRequest'), deleteUnknown: ui('common.uncertainRequest'), roleNameLocked: ui('common.bootstrapRoleNameLocked'), chooseRoles: ui('platform.copy.c4aa8d0d'), self: ui('platform.copy.d7d2e55c'),
} : {
  create: ui('platform.copy.1f19a013'), edit: ui('platform.copy.497d7839'), reset: ui('platform.copy.6dbd20b5'), delete: ui('platform.copy.9851501a'), actions: ui('platform.copy.412ae2c3'), createRole: ui('platform.copy.8aa1740f'), editRole: ui('platform.copy.26c6cda4'), password: ui('platform.copy.cb6bdf7d'), save: ui('platform.copy.57b298e5'), cancel: ui('platform.copy.2230b1a7'), retry: ui('platform.copy.0ffed4b3'), saved: ui('platform.copy.33341f71'), unknown: ui('common.uncertainRequest'), deleteUnknown: ui('common.uncertainRequest'), roleNameLocked: ui('common.bootstrapRoleNameLocked'), chooseRoles: ui('platform.copy.c4aa8d0d'), self: ui('platform.copy.d7d2e55c'),
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

async function load(requestedOffset = offset.value) {
  const request = ++generation
  const brand = props.brandId
  const currentTab = tab.value
  const currentOffset = requestedOffset
  selected.value = null
  error.value = ''; errorToast.value = ''
  if (!ready.value) { loading.value = false; return }
  loading.value = true
  try {
    if (currentTab === 'accounts') {
      const result = await (props.scope === 'platform' ? api.platformAccounts(pageSize.value, currentOffset) : api.accounts(brand, pageSize.value, currentOffset))
      if (request !== generation || props.brandId !== brand || tab.value !== currentTab) return
      [accounts.value, recordTotal.value] = [result.items, readPaginationTotal(result)]
      offset.value = currentOffset
      more.value = BigInt(currentOffset + result.items.length) < BigInt(recordTotal.value)
    } else if (currentTab === 'roles') {
      const result = await (props.scope === 'platform' ? api.platformRoles(pageSize.value, currentOffset) : api.roles(brand, pageSize.value, currentOffset))
      if (request !== generation || props.brandId !== brand || tab.value !== currentTab) return
      [roles.value, recordTotal.value] = [result.items, readPaginationTotal(result)]
      offset.value = currentOffset
      more.value = BigInt(currentOffset + result.items.length) < BigInt(recordTotal.value)
    }
  } catch (cause) {
    if (request === generation && props.brandId === brand && tab.value === currentTab) report(cause)
  } finally {
    if (request === generation) loading.value = false
  }
}

async function deleteBatchItem(item: AccessItem, key: string) {
  const platform = props.scope === 'platform'
  if (isAccount(item)) return platform ? api.deletePlatformAccount(item.id, {version:item.version}, key) : api.deleteAccount(props.brandId, item.id, {version:item.version}, key)
  return platform ? api.deletePlatformRole(item.id, {version:item.version}, key) : api.deleteRole(props.brandId, item.id, {version:item.version}, key)
}
function batchDeleted(item: AccessItem) {
  if (isAccount(item)) accounts.value = accounts.value.filter(entry => entry.id !== item.id)
  else roles.value = roles.value.filter(entry => entry.id !== item.id)
  recordTotal.value = String(BigInt(recordTotal.value) - 1n)
}

async function refreshAfterBatch() {
  await load(BigInt(offset.value) >= BigInt(recordTotal.value) ? Math.max(0, offset.value - pageSize.value) : offset.value)
}

function chooseTab(next: Tab) {
  if (tab.value === next) return
  offset.value = 0
  tab.value = next
  selected.value = null
  error.value = ''
}

async function page(nextOffset: number) { await load(nextOffset) }

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
      roleOptions.value = result.items.filter(role => role.status === 'active' || form.value.role_ids.includes(role.id))
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
        : { key: formMode.value === 'account-create' ? 'access.accountCreated' : formMode.value === 'account-edit' ? 'access.accountUpdated' : formMode.value === 'password' ? 'access.passwordReset' : formMode.value === 'role-create' ? 'access.roleCreated' : 'access.roleUpdated', name: formMode.value === 'account-create' ? form.value.username : target && isAccount(target) ? target.username : form.value.name }
    const deleted = formMode.value === 'account-delete' || formMode.value === 'role-delete'
    if (deleted && target) batchDeleted(target)
    pending.value = null; formMode.value = ''; form.value.password = ''
    saving.value = false
    await refreshAfterBatch()
  } catch (cause) {
    if (request !== generation) return
    const unknown = cause instanceof PlatformAccessApiError && (cause.status >= 500 || cause.code === 'NETWORK_ERROR' || cause.code === 'INVALID_RESPONSE')
    if (!unknown) pending.value = null
    report(cause)
  } finally { if (request === generation || !pending.value) saving.value = false }
}

watch(() => [props.brandId, props.scope, props.accountId, props.initialTab], () => { [accounts.value,recordTotal.value]=[[],'0']; [roles.value,recordTotal.value]=[[],'0']; more.value = false; tab.value = props.initialTab ?? 'accounts'; offset.value = 0; formMode.value = ''; form.value.password = ''; pending.value = null; notice.value = null; errorToast.value = ''; optionsLoading.value = false }, { flush: 'sync' })
watch(() => [props.brandId, props.scope, props.accountId, tab.value], () => { void withGlobalLoading(load) }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++generation; form.value.password = ''; pending.value = null; emit('locked', false) })
useOperationToast(errorToast, value => displayError(value), 'error');
useOperationToast(notice, value => ui(value.key, { name: value.name }), 'success');

useOperationToast(error, value => displayError(value), 'error');
</script>

<template>
  <div data-testid="platform-access">
    <ManagementList :busy="loading" data-testid="platform-access-list">
      <template #tabs><div class="console-tabs" role="tablist" :aria-label="copy.title" data-testid="platform-access-tabs">
        <button :disabled="locked" :class="tab === 'accounts' ? 'button button-primary' : 'button button-secondary'" role="tab" :aria-selected="tab === 'accounts'" @click="chooseTab('accounts')">{{ copy.accounts }}</button>
        <button :disabled="locked" :class="tab === 'roles' ? 'button button-primary' : 'button button-secondary'" role="tab" :aria-selected="tab === 'roles'" @click="chooseTab('roles')">{{ copy.roles }}</button>
      </div></template>
      <template v-if="error || (tab === 'accounts' ? canWriteAccounts : canWriteRoles)" #actions>
        <button v-if="tab === 'accounts' && canWriteAccounts" class="button button-primary" :disabled="loading || locked" @click="withGlobalLoading(beginForm, 'account-create')">{{ writeCopy.create }}</button>
        <button v-if="tab === 'roles' && canWriteRoles" class="button button-primary" :disabled="loading || locked" @click="withGlobalLoading(beginForm, 'role-create')">{{ writeCopy.createRole }}</button>
        <button v-if="error" v-refresh-button="copy.refresh" class="button button-secondary" :disabled="loading || !ready || locked" @click="withGlobalLoading(load)">{{ copy.refresh }}</button>
      </template>
      <BatchDelete :key="scope + ':' + brandId + ':' + tab" :subject="tab" :entry-label="item => isAccount(item) ? item.username : item.name" :scope-key="String(offset)" :entries="entries" :selectable="item => isAccount(item) ? item.id !== accountId : !item.is_bootstrap" :enabled="tab === 'accounts' ? canDeleteAccounts : canDeleteRoles" :locale="locale" :loading="loading" :delete-entry="deleteBatchItem" :error-text="cause => platformErrorMessage(cause, locale)" :is-unknown="cause => !(cause instanceof PlatformAccessApiError) || cause.status === 0 || cause.status >= 500 || cause.code === 'INVALID_RESPONSE'" @locked="batchOpen = $event" @deleted="batchDeleted" @finished="withGlobalLoading(refreshAfterBatch)" v-slot="{selected: checked, toggle, selectLabel, locked: batchLocked}">
        <ManagementTable table-class="account-directory-table" :entries="entries" :columns="[{key:'account',label:tab === 'accounts' ? copy.username : copy.name}, ...(tab === 'accounts' ? [{key:'roles',label:copy.roles}] : []), {key:'status',label:copy.status},{key:'actions',label:writeCopy.actions,cellClass:'access-actions'}]" :empty="!loading && !error && !(entries).length" :empty-label="tab === 'accounts' ? copy.emptyAccounts : copy.emptyRoles">
          <template #account="{entry:item}"><span class="member-account-select"><label v-if="(isAccount(item) ? canDeleteAccounts && item.id !== accountId : canDeleteRoles && !item.is_bootstrap)" class="member-select-control"><input type="checkbox" :checked="checked.includes(item.id)" :aria-label="selectLabel(isAccount(item) ? item.username : item.name)" :disabled="batchLocked" @change="toggle(item.id)" /></label><button class="text-button row-action" :disabled="batchLocked" @click="selected = item"><template v-if="isAccount(item)">{{ item.username }}</template><AssociationName v-else :record="item" kind="role" :id="item.id" :locale="locale" /></button></span></template>
          <template #roles="{entry:item}"><template v-if="isAccount(item)"><div v-for="roleId in item.role_ids" :key="roleId"><AssociationName :record="item" kind="role" :id="roleId" :locale="locale" /></div><span v-if="!item.role_ids.length">—</span></template></template>
          <template #status="{entry:item}"><span class="status-pill" :class="item.status">{{ stateLabel(item.status) }}</span></template>
          <template #actions="{entry:item}"><MemberActionsMenu :id="'access-actions-' + item.id" :label="writeCopy.actions" :disabled="batchLocked"><button type="button" role="menuitem" @click="selected = item">{{ copy.detail }}</button><template v-if="isAccount(item) && item.id !== accountId"><button v-if="canWriteAccounts" type="button" role="menuitem" @click="withGlobalLoading(beginForm, 'account-edit', item)">{{ writeCopy.edit }}</button><button v-if="canWriteAccounts" type="button" role="menuitem" @click="withGlobalLoading(beginForm, 'password', item)">{{ writeCopy.reset }}</button><button v-if="canDeleteAccounts" type="button" role="menuitem" class="danger-text" @click="withGlobalLoading(beginForm, 'account-delete', item)">{{ writeCopy.delete }}</button></template><template v-else-if="!isAccount(item) && !item.is_bootstrap"><button v-if="canWriteRoles" type="button" role="menuitem" @click="withGlobalLoading(beginForm, 'role-edit', item)">{{ writeCopy.editRole }}</button><button v-if="canDeleteRoles" type="button" role="menuitem" class="danger-text" @click="withGlobalLoading(beginForm, 'role-delete', item)">{{ writeCopy.delete }}</button></template></MemberActionsMenu></template>
        </ManagementTable>

      </BatchDelete>
    <template #pagination><TablePagination class="wallet-pagination" data-testid="platform-access-pages" :total="recordTotal" :page="Math.floor(offset / pageSize + 1)" :count="(entries).length" :locale="locale" :page-size="pageSize" :previous-disabled="loading || offset === 0 || !ready || locked || batchOpen" :next-disabled="loading || !more || locked || batchOpen" :previous-label="copy.previous" :next-label="copy.next" :go-to="(target: number) => page((target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" :busy="loading || locked || batchOpen" /></template></ManagementList>
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
    <DetailModal v-if="formMode" class="access-editor-dialog" :class="{ 'access-role-modal': formMode === 'role-create' || formMode === 'role-edit', 'access-delete-modal': formMode === 'account-delete' || formMode === 'role-delete' }" :title="formTitle" :close-label="copy.close" :close-disabled="locked" @close="!locked && (formMode = '', form.password = '')">
        <p v-if="formTarget">{{ isAccount(formTarget) ? formTarget.username : formTarget.name }}</p>
        <p v-if="pending && !saving" class="message" role="status">{{ formMode === 'account-delete' ? writeCopy.deleteUnknown : writeCopy.unknown }}</p>
        <form v-write-guard="saving" novalidate v-inline-validation class="brand-form ui-form" @submit.prevent="withGlobalLoading(saveForm)">
          <div class="access-form-fields">
          <fieldset :disabled="locked || optionsLoading">
            <label v-if="formMode === 'account-create'">{{ copy.username }}<input v-model="form.username" pattern="[a-z][a-z0-9_]{2,31}" :data-validation-format="formValidationText('accountFormat')" required autocomplete="off" /></label>
            <label v-if="formMode === 'account-create' || formMode === 'password'">{{ writeCopy.password }}<input v-model="form.password" data-min-characters="6" data-max-characters="30" type="password" maxlength="128" required autocomplete="new-password" /></label>
            <label v-if="formMode === 'account-edit'"><BusinessDetails :value="copy.status" :locale="props.locale" /><SelectControl v-model="form.status" :label="copy.status" :options="[{ value: 'active', label: (String(stateLabel('active'))).trim() }, { value: 'disabled', label: (String(stateLabel('disabled'))).trim() }]" /></label>
            <div v-if="formMode === 'account-create' || formMode === 'account-edit'" class="access-options">
              <p>{{ writeCopy.chooseRoles }}</p>
              <label v-for="role in roleOptions" :key="role.id"><input v-model="form.role_ids" type="checkbox" :value="role.id" :disabled="role.status !== 'active' && !form.role_ids.includes(role.id)" /> <AssociationName :record="role" kind="role" :id="role.id" :locale="locale" /> · {{ stateLabel(role.status) }}</label>
            </div>
            <div v-if="formMode === 'role-create' || formMode === 'role-edit'" class="role-identity">
              <label>{{ copy.name }}<input v-model="form.name" maxlength="120" required /></label>
              <label>{{ copy.code }}<input v-model="form.code" pattern="[a-z][a-z0-9_]{2,47}" :data-validation-format="formValidationText('codeFormat', {min: 3, max: 48})" required :readonly="formMode === 'role-edit'" :class="{'readonly-field': formMode === 'role-edit'}" /></label>
              <label><BusinessDetails :value="copy.status" :locale="props.locale" /><SelectControl v-model="form.status" :label="copy.status" :options="[{ value: 'active', label: (String(stateLabel('active'))).trim() }, { value: 'disabled', label: (String(stateLabel('disabled'))).trim() }]" /></label>
            </div>
            <PermissionPicker v-if="formMode === 'role-create' || formMode === 'role-edit'" v-model="form.permissions" :options="permissionOptions" :locale="locale" />
          </fieldset>
          </div>
          <div class="form-actions ui-form-actions">
            <p v-if="error" class="message error inline-validation-error" role="alert">{{ displayError(error) }}</p>

            <button type="submit" :class="formMode === 'account-delete' || formMode === 'role-delete' ? 'button button-danger' : 'button button-primary'" :disabled="saving || !optionsReady || ((formMode === 'account-create' || formMode === 'account-edit') && !form.role_ids.length)">{{ saving ? copy.loading : pending ? writeCopy.retry : formMode === 'account-delete' || formMode === 'role-delete' ? writeCopy.delete : writeCopy.save }}</button>
          </div>
        </form>
    </DetailModal>
  </div>
</template>

<style scoped>
.table-wrap table { table-layout: fixed; white-space: normal; }
.table-wrap :is(th, td) { position: static; min-width: 0; overflow-wrap: anywhere; }
.table-wrap .row-action { max-width: 100%; white-space: normal; overflow-wrap: anywhere; }
fieldset { border: 0; padding: 0; margin: 0; display: grid; gap: 14px; min-width: 0; }
.access-options { max-height: 200px; overflow: auto; border: 1px solid var(--line); border-radius: 6px; padding: 10px; }
.access-options label { display: flex; flex-direction: row; align-items: center; gap: 8px; overflow-wrap: anywhere; }
.access-options input { width: auto; min-height: 18px; flex-shrink: 0; }
.modal-card { max-height: 90vh; overflow: hidden; display: flex; flex-direction: column; }
.access-delete-modal { width: min(480px, calc(100vw - 24px)); padding: 0; }
.access-delete-modal > h2 { margin: 0 0 12px; }
.access-delete-modal > p { margin: 0 0 12px; color: var(--text-muted); }
.access-delete-modal .brand-form { gap: 10px; }
.access-role-modal { padding: 0; }
.access-role-modal fieldset > label { min-width: 0; }
.access-role-modal fieldset > label :is(input) { min-width: 0; width: 100%; }
.access-role-modal > p { margin: 0 0 8px; }
.access-role-modal fieldset { grid-template-columns: minmax(0, 1fr); gap: 14px; }
.role-identity { display:grid; grid-template-columns:minmax(0,1fr) minmax(0,1fr) minmax(130px,.55fr); gap:12px; }
.role-identity label { display:grid; gap:6px; min-width:0; font-size:var(--body-font-size); }
.role-identity :is(input) { min-width:0; width:100%; min-height:44px; }
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
.permission-chip { display: flex; min-width: 0; align-items: center; gap: 8px; padding: 5px 7px; border: 1px solid var(--border-light); border-radius: var(--radius-sm); background: var(--bg-subtle); font-size: var(--body-font-size); font-weight: 600; line-height: 1.35; overflow-wrap: anywhere; }
.permission-chip input { width: auto; min-height: 18px; margin: 0; flex: 0 0 auto; }
.access-role-modal .access-options { max-height: none; padding: 6px; }
@media (max-width: 720px) { .role-detail-fields { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 480px) { .role-detail-fields { grid-template-columns: 1fr; } .role-permissions-field { grid-column: auto; } }
@media (max-width: 900px) {
  .access-role-modal :is(button), .access-role-modal fieldset > label input { min-height: 44px; }
  .access-role-modal .permission-chip { min-height: 44px; }
}
@media (max-width: 620px) {
  .access-role-modal { padding: 0; }
  .access-role-modal fieldset { grid-template-columns: minmax(0, 1fr); }
  .role-identity { grid-template-columns:minmax(0,1fr); }
}
</style>
