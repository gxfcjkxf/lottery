<script setup lang="ts">
import { readPaginationTotal } from '@lottery/shared'
const rolesTotal = ref('0')
const accountsTotal = ref('0')

import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import { vWriteGuard } from "@lottery/shared";
import { AssociationName } from '@lottery/shared'
import { permissionLabel } from '@lottery/shared'
import DetailModal from '@lottery/shared/DetailModal.vue'
import PermissionPicker from '@lottery/shared/PermissionPicker.vue'
import ManagementList from "@lottery/shared/ManagementList.vue"
import MemberActionsMenu from "@lottery/shared/MemberActionsMenu.vue"
import ManagementTable from '@lottery/shared/ManagementTable.vue'
import PaginationControls from '@lottery/shared/PaginationControls.vue'
const associationLocale = useAdminI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vInlineValidation } from '@lottery/shared'
import { computed, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import {
  assignableRoles as getAssignableRoles,
  canEditAccountScope,
  canView,
  canWrite,
  createBodyKeyTracker,
  createManagementApi,
  effectivePermissions,
  rolePermissionChoices,
  rolePermissionUnion,
  type AdminRecord,
  type ManagementAccount,
  type RoleRecord,
} from "./management-api";
const pageSize = ref(50)


const props = defineProps<{
  account: AdminAccount & Partial<ManagementAccount>;
  brandId: string;
  section: "roles" | "accounts";
}>();
const emit = defineEmits<{
  (event: "session-invalid"): void;
  (event: "created", saved: AdminRecord): void;
}>();
const api = createManagementApi();
const { t, message, display } = useAdminI18n();
const section = computed(() => props.section);
const roleEditorOpen = ref(false);
const accountEditorOpen = ref(false);
function startRole() { newRole(); error.value = ""; roleEditorOpen.value = true; }
function startAccount() { newAccount(); error.value = ""; accountEditorOpen.value = true; }
const roles = ref<RoleRecord[]>([]);
const viewedRole = ref<RoleRecord | null>(null);
const viewedAccount = ref<AdminRecord | null>(null);
const accounts = ref<AdminRecord[]>([]);
const registeredPermissions = ref<string[]>([]);
const roleCatalogLoaded = ref(false);
const roleCatalogMessage = ref<string | LocalizedMessage>("");
const offset = ref(0);
const busy = ref(false);
const loading = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const roleForm = ref({
  id: "",
  version: 0,
  code: "",
  name: "",
  status: "active" as "active" | "disabled",
  permissions: [] as string[],
});
const accountForm = ref({
  id: "",
  version: 0,
  username: "",
  password: "",
  role_ids: [] as string[],
  status: "active",
  reset: false,
});
const accountPasswordCharacters = computed(
  () => Array.from(accountForm.value.password).length,
);
const roleKeyFor = createBodyKeyTracker();
const accountKeyFor = createBodyKeyTracker();
const canRoles = computed(() =>
  canView(props.account, props.brandId, "role"),
);
const canAccounts = computed(() =>
  canView(props.account, props.brandId, "admin"),
);
const writeRoles = computed(() =>
  canWrite(props.account, props.brandId, "role"),
);
const writeAccounts = computed(() =>
  canWrite(props.account, props.brandId, "admin"),
);
const actorPermissions = computed(() =>
  effectivePermissions(props.account, props.brandId),
);
const roleChoices = computed(() =>
  rolePermissionChoices(
    props.account,
    props.brandId,
    roleCatalogLoaded.value ? registeredPermissions.value : null,
  ),
);
const assignableRoles = computed(() =>
  getAssignableRoles(props.account, props.brandId, roles.value),
);
const pageHasNext = computed(() =>
  section.value === "roles"
    ? BigInt(offset.value + roles.value.length) < BigInt(rolesTotal.value)
    : BigInt(offset.value + accounts.value.length) < BigInt(accountsTotal.value),
);
const editableAccount = (target: AdminRecord) =>
  canEditAccountScope(props.account, props.brandId, target) &&
  roleCatalogLoaded.value &&
  target.role_ids.every(
    (id) =>
      roles.value.find((role) => role.id === id) !== undefined &&
      roles.value
        .find((role) => role.id === id)
        ?.permissions.every((permission) =>
          actorPermissions.value.has(permission),
        ),
  );
const canResetTarget = (target: AdminRecord) =>
  canEditAccountScope(props.account, props.brandId, target);
const editableRole = (role: RoleRecord) =>
  writeRoles.value &&
  !role.is_bootstrap &&
  role.permissions.every((permission) =>
    actorPermissions.value.has(permission),
  );
const roleNameCharacters = computed(
  () => Array.from(roleForm.value.name).length,
);
const selectedRoles = computed(() =>
  roles.value.filter((role) => accountForm.value.role_ids.includes(role.id)),
);
const selectedRolePermissionUnion = computed(() =>
  rolePermissionUnion(selectedRoles.value),
);
const rolePermissionDisplay = (target: AdminRecord) =>
  rolePermissionUnion(
    roles.value.filter((role) => target.role_ids.includes(role.id)),
  );
const accountPermissionDisplay = (target: AdminRecord) =>
  roleCatalogLoaded.value
    ? rolePermissionDisplay(target).map(permission => permissionLabel(permission, associationLocale.value)).join(" · ") || t('admin.copy.d04292ff')
    : t('admin.copy.d0b2463d');

function handleError(cause: unknown) {
  if (cause instanceof AdminApiError && cause.code === 'ROLE_ASSIGNED') {
    error.value = message('admin.error.ROLE_ASSIGNED');
    return;
  }
  if (cause instanceof AdminApiError && cause.status === 401)
    emit("session-invalid");
  error.value = publicErrorMessage(cause);
}
let loadVersion = 0;
async function load(requestedOffset = offset.value) {
  const version = ++loadVersion;
  const requestBrandId = props.brandId;
  const requestSection = section.value;
  const isCurrentLoad = () => version === loadVersion;
  roleCatalogLoaded.value = false;
  roleCatalogMessage.value = "";
  roles.value = [];
  registeredPermissions.value = [];
  if (
    !requestBrandId ||
    !(
      canRoles.value ||
      canAccounts.value ||
      writeRoles.value ||
      writeAccounts.value
    )
  ) {
    loading.value = false;
    return;
  }
  loading.value = true;
  error.value = "";
  if (requestSection === "roles") {
    if (canRoles.value) {
      try {
        const [listed, perms] = await Promise.all([
          api.roles(requestBrandId, pageSize.value, requestedOffset),
          api.permissions(requestBrandId),
        ]);
        if (!isCurrentLoad()) return;
        [roles.value, rolesTotal.value] = [listed.items, readPaginationTotal(listed)];
        offset.value = requestedOffset;
        registeredPermissions.value = perms.items;
        roleCatalogLoaded.value = true;
        roleCatalogMessage.value = "";
      } catch (cause) {
        if (!isCurrentLoad()) return;
        registeredPermissions.value = [];
        roleCatalogLoaded.value = false;
        roleCatalogMessage.value = message('admin.copy.afb348bc',
        );
        handleError(cause);
      }
    } else {
      [roles.value,rolesTotal.value]=[[],'0'];
      registeredPermissions.value = [];
      roleCatalogLoaded.value = false;
      roleCatalogMessage.value = message('admin.copy.56c87a94',
      );
    }
  }
  if (requestSection === "accounts") {
    const tasks: Promise<void>[] = [];
    if (canAccounts.value)
      tasks.push(
        api
          .accounts(requestBrandId, pageSize.value, requestedOffset)
          .then((listed) => {
            if (isCurrentLoad()) { [accounts.value, accountsTotal.value] = [listed.items, readPaginationTotal(listed)]; offset.value = requestedOffset; }
          })
          .catch((cause: unknown) => {
            if (isCurrentLoad()) handleError(cause);
          }),
      );
    if (canRoles.value) {
      tasks.push(
        loadRoleCatalog(requestBrandId)
          .then((catalog) => {
            if (!isCurrentLoad()) return;
            roles.value = catalog;
            roleCatalogLoaded.value = true;
            roleCatalogMessage.value = "";
          })
          .catch((cause: unknown) => {
            if (!isCurrentLoad()) return;
            roleCatalogLoaded.value = false;
            roleCatalogMessage.value = message('admin.copy.50575ea4',
            );
            handleError(cause);
          }),
      );
    } else {
      [roles.value,rolesTotal.value]=[[],'0'];
      roleCatalogLoaded.value = false;
      roleCatalogMessage.value = message('admin.copy.48fe4041',
      );
    }
    await Promise.all(tasks);
  }
  if (isCurrentLoad()) loading.value = false;
}
async function loadRoleCatalog(brandId: string): Promise<RoleRecord[]> {
  const all: RoleRecord[] = [];
  for (let pageOffset = 0; ; pageOffset += 50) {
    const page = await api.roles(brandId, 50, pageOffset);
    all.push(...page.items);
    if (page.items.length < 50) return all;
  }
}
onMounted(() => void withGlobalLoading(load));
watch(
  [() => props.brandId, section, actorPermissions],
  () => {
    roleEditorOpen.value = false;
    accountEditorOpen.value = false;
    viewedRole.value = null;
    [roles.value,rolesTotal.value]=[[],'0'];
    [accounts.value,accountsTotal.value]=[[],'0'];
    registeredPermissions.value = [];
    roleCatalogLoaded.value = false;
    roleCatalogMessage.value = "";
    offset.value = 0;
    newRole();
    newAccount();
    void withGlobalLoading(load);
  },
);
function newRole() {
  roleEditorOpen.value = false;
  roleForm.value = {
    id: "",
    version: 0,
    code: "",
    name: "",
    status: "active",
    permissions: [],
  };
}
function editRole(role: RoleRecord) {
  if (!editableRole(role)) return;
  roleForm.value = {
    id: role.id,
    version: role.version,
    code: role.code,
    name: role.name,
    status: role.status,
    permissions: [...role.permissions],
  };
  error.value = ""; roleEditorOpen.value = true;
}
async function saveRole() {
  const form = roleForm.value;
  if (
    busy.value ||
    loading.value ||
    !writeRoles.value ||
    !roleCatalogLoaded.value ||
    !form.name.trim() ||
    roleNameCharacters.value > 120 ||
    (!form.id && form.code.trim().length > 48)
  )
    return;
  const body = form.id
    ? {
        version: form.version,
        name: form.name.trim(),
        status: form.status,
        permissions: [...form.permissions].sort(),
      }
    : {
        code: form.code.trim(),
        name: form.name.trim(),
        permissions: [...form.permissions].sort(),
      };
  busy.value = true;
  error.value = "";
  try {
    if (form.id)
      await api.updateRole(
        props.brandId,
        form.id,
        body as {
          version: number;
          name: string;
          status: "active" | "disabled";
          permissions: string[];
          reason?: string;
        },
        roleKeyFor(body),
      );
    else
      await api.createRole(
        props.brandId,
        body as {
          code: string;
          name: string;
          permissions: string[];
          reason?: string;
        },
        roleKeyFor(body),
      );
    roleKeyFor.clear();
    notice.value = form.id
      ? message('admin.copy.b42c6c01')
      : message('admin.copy.94e00f54');
    newRole();
    await load();
  } catch (cause) {
    handleError(cause);
  } finally {
    busy.value = false;
  }
}
function newAccount() {
  accountEditorOpen.value = false;
  accountForm.value = {
    id: "",
    version: 0,
    username: "",
    password: "",
    role_ids: [],
    status: "active",
    reset: false,
  };
}
function editAccount(target: AdminRecord, reset = false) {
  if (reset ? !canResetTarget(target) : !editableAccount(target)) return;
  accountForm.value = {
    id: target.id,
    version: target.version,
    username: target.username,
    password: "",
    role_ids: [...target.role_ids],
    status: target.status,
    reset,
  };
  error.value = ""; accountEditorOpen.value = true;
}
async function saveAccount() {
  const form = accountForm.value;
  if (
    busy.value ||
    (!form.reset && !roleCatalogLoaded.value)
  )
    return;
  const body = form.id
    ? form.reset
      ? {
          version: form.version,
          password: form.password,
        }
      : {
          version: form.version,
          role_ids: [...form.role_ids],
          status: form.status,
        }
    : {
        username: form.username.trim(),
        password: form.password,
        role_ids: [...form.role_ids],
      };
  busy.value = true;
  error.value = "";
  try {
    if (!form.id) {
      const saved = await api.createAccount(
        props.brandId,
        body as {
          username: string;
          password: string;
          role_ids: string[];
          reason?: string;
        },
        accountKeyFor(body),
      );
      emit("created", saved);
      notice.value = message('admin.copy.2b0119ea');
    } else if (form.reset) {
      await api.resetAccountPassword(
        props.brandId,
        form.id,
        body as { version: number; password: string; reason?: string },
        accountKeyFor(body),
      );
      notice.value = message('admin.copy.5c42969c');
    } else {
      await api.updateAccount(
        props.brandId,
        form.id,
        body as {
          version: number;
          role_ids: string[];
          status: string;
          reason?: string;
        },
        accountKeyFor(body),
      );
      notice.value = message('admin.copy.c1434a1e');
    }
    accountKeyFor.clear();
    newAccount();
    await load();
  } catch (cause) {
    handleError(cause);
  } finally {
    busy.value = false;
  }
}
async function changePage(direction: number) {
  await load(Math.max(0, offset.value + direction * pageSize.value));
}

useOperationToast(() => error.value, value => display(value), 'error');
useOperationToast(() => notice.value, value => display(value), 'success');
</script>

<template>
  <section class="access-page">
    <header class="console-page-heading heading">
      <div>
        <h1 class="console-page-title">{{ section === 'roles' ? t('admin.copy.78c84d5e') : t('admin.copy.f8e4afe4') }}</h1>
      </div>
    </header>
    <p
      v-if="!(canRoles || writeRoles || canAccounts || writeAccounts)"
      class="callout"
    >
      {{ t('admin.copy.ce325bac') }}
    </p>
    <p v-if="error && !roleEditorOpen && !accountEditorOpen" class="operation-recovery-actions">
       <button class="button button-secondary" v-refresh-button="t('admin.copy.894e2a7d')" type="button" @click="withGlobalLoading(load)">{{ t('admin.copy.894e2a7d') }}</button>
    </p>

    <template v-if="section === 'roles' && (canRoles || writeRoles)">
      <div class="columns">
        <button v-if="writeRoles && !canRoles" class="button button-secondary" type="button" @click="startRole">{{ t('admin.copy.8c7fddfb') }}</button>
        <DetailModal v-if="writeRoles && roleEditorOpen" :title="roleForm.id ? t('admin.copy.0233b2e8') : t('admin.copy.8c7fddfb')" :close-label="t('association.close')" :close-disabled="busy" @close="!busy && (roleEditorOpen = false)">

        <form id="brand-role-editor" v-write-guard="busy" novalidate v-inline-validation class="form role-form ui-form" @submit.prevent="withGlobalLoading(saveRole)">
          <div class="role-identity">
            <label>{{ t('admin.copy.6404dc43') }}
              <input v-model.trim="roleForm.name" required />
              <small>{{ roleNameCharacters }} / 120 {{ t('admin.copy.b2ca2f37') }}</small>
            </label>
            <label v-if="!roleForm.id">{{ t('admin.copy.8c47c503') }}
              <input v-model.trim="roleForm.code" required maxlength="48" autocomplete="off" />
            </label>
            <label v-if="roleForm.id">{{ t('admin.copy.a765b3ca') }}
              <SelectControl v-model="roleForm.status" :label="String(t('admin.copy.a765b3ca'))" :options="[{ value: 'active', label: (String(t('admin.copy.98d24810'))).trim() }, { value: 'disabled', label: (String(t('admin.copy.5e3326db'))).trim() }]" />
            </label>
          </div>
          <PermissionPicker v-model="roleForm.permissions" :options="roleChoices" :locale="associationLocale" />
        </form>
        <template #footer>
          <button
            form="brand-role-editor"
            type="submit"
            class="button button-primary"
            :disabled="busy || loading || !roleCatalogLoaded || !roleForm.name.trim() || roleNameCharacters > 120 || (!roleForm.id && !roleForm.code.trim())"
          >
            {{ busy ? t('admin.copy.2462134b') : roleForm.id ? t('admin.copy.383c86a7') : t('admin.copy.80f397e0') }}
          </button>
        </template>
        </DetailModal>
        <ManagementList v-if="canRoles" :busy="loading" class="listing">
          <template v-if="writeRoles" #actions><button v-if="writeRoles" class="button button-primary" type="button" @click="startRole">{{ t('admin.copy.8c7fddfb') }}</button></template>
          <ManagementTable :entries="roles" :columns="[{key:'name',label:t('admin.copy.6404dc43')},{key:'status',label:t('admin.copy.a765b3ca')},{key:'actions',label:t('admin.copy.d21fcbef')}]" :empty="!loading && !error && !roles.length" :empty-label="t('admin.copy.5c979929')">
            <template #name="{entry:role}"><button type="button" class="text-button" @click="viewedRole = role"><AssociationName :record="role" kind="role" :id="role.id" :locale="associationLocale" /></button></template>
            <template #status="{entry:role}"><span class="status-pill" :class="role.status">{{ role.status === 'active' ? t('admin.copy.98d24810') : t('admin.copy.5e3326db') }}</span></template>
            <template #actions="{entry:role}"><MemberActionsMenu :id="'brand-role-actions-' + role.id" :label="t('admin.copy.d21fcbef')"><button type="button" role="menuitem" @click="viewedRole = role">{{ t('admin.copy.01db40da') }}</button><button v-if="editableRole(role)" type="button" role="menuitem" @click="editRole(role)">{{ t('admin.copy.24dfbf6c') }}</button></MemberActionsMenu></template>
          </ManagementTable>

        <template #pagination><PaginationControls
            :total="rolesTotal" :page="Math.floor(offset / pageSize + 1)" :count="roles.length" :locale="associationLocale"
            :previous-label="t('admin.copy.dbe7a5d7')"
            :next-label="t('admin.copy.2e9c9e7a')"
            :busy="loading"
            :has-previous="offset > 0"
            :has-next="pageHasNext"
           :page-size="pageSize" :go-to="(target: number) => changePage(target - (Math.floor(offset / pageSize + 1)))" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" /></template></ManagementList>
        <p v-else-if="roleCatalogMessage" class="callout">
          {{
            display(roleCatalogMessage)
          }}
        </p>
      </div>
    </template>

    <template
      v-else-if="section === 'accounts' && (canAccounts || writeAccounts)"
    >
      <div class="columns">
        <button v-if="writeAccounts && !canAccounts && roleCatalogLoaded" class="button button-secondary" type="button" @click="startAccount">{{ t('admin.copy.0f043095') }}</button>
        <DetailModal v-if="writeAccounts && accountEditorOpen && (roleCatalogLoaded || accountForm.reset)" :title="accountForm.id ? accountForm.reset ? t('admin.copy.1945c635') : t('admin.copy.b74a3dd7') : t('admin.copy.d2a40bda')" :close-label="t('association.close')" :close-disabled="busy" @close="!busy && (accountEditorOpen = false)">

        <form v-write-guard="busy" novalidate v-inline-validation
          v-if="writeAccounts && (roleCatalogLoaded || accountForm.reset)"
          class="panel form ui-form"
          @submit.prevent="withGlobalLoading(saveAccount)"
        >
          <template v-if="!accountForm.id"
            ><label
              >{{ t('admin.copy.91eab520') }}<input
                v-model.trim="accountForm.username"
                required
                maxlength="32"
                autocomplete="username" /></label
            ><label
              >{{ t('admin.copy.f1a553c9') }}<input
                v-model="accountForm.password" data-min-characters="6" data-max-characters="30"
                type="password"
                required
                autocomplete="new-password"
              /><small>{{ accountPasswordCharacters }} / 30 {{ t('admin.console.characters') }}</small></label
            ></template
          >
          <template v-else-if="accountForm.reset"
            ><label
              >{{ t('admin.copy.683caabd') }}<input
                v-model="accountForm.password" data-min-characters="6" data-max-characters="30"
                type="password"
                required
                autocomplete="new-password"
              /><small>{{ accountPasswordCharacters }} / 30 {{ t('admin.console.characters') }}</small></label
            ></template
          >
          <template v-else
            ><p class="readonly">
              {{ t('admin.copy.851c6121') }}: <b>{{ accountForm.username }}</b>
            </p>
            <label
              >{{ t('admin.copy.a765b3ca') }}<SelectControl v-model="accountForm.status" :label="String(t('admin.copy.a765b3ca'))" :options="[{ value: 'active', label: (String(t('admin.copy.98d24810'))).trim() }, { value: 'disabled', label: (String(t('admin.copy.5e3326db'))).trim() }]" /></label
            ></template
          >
          <fieldset v-if="!accountForm.reset" class="permissions">
            <legend>{{ t('admin.copy.31ff8470') }}</legend>
            <label v-for="role in assignableRoles" :key="role.id" class="check"
              ><input
                v-model="accountForm.role_ids"
                type="checkbox"
                :value="role.id"
              /><AssociationName :record="role" kind="role" :id="role.id" :locale="associationLocale" /></label
            >
            <p v-if="!assignableRoles.length" class="muted empty-state">
              {{ t('admin.copy.222e48f4') }}
            </p>
          </fieldset>
          <div v-if="!accountForm.reset" class="union">
            <b>{{ t('admin.copy.54bf1d77') }}</b
            ><span>{{
              selectedRolePermissionUnion.length
                ? selectedRolePermissionUnion.map(permission => permissionLabel(permission, associationLocale)).join(" · ")
                : t('admin.copy.d04292ff')
            }}</span
            >
          </div>
          <button
            class="button button-primary"
            :disabled="
              busy ||
              ((!accountForm.id || accountForm.reset) &&
                (accountPasswordCharacters < 6 || accountPasswordCharacters > 30))
            "
          >
            {{
              busy
                ? t('admin.copy.46511dc0')
                : accountForm.id
                  ? accountForm.reset
                    ? t('admin.copy.d1e2ba79')
                    : t('admin.copy.56c2cb94')
                  : t('admin.copy.d2a40bda')
            }}
          </button>
        </form>
        </DetailModal>
        <p
          v-if="(canAccounts || writeAccounts) && roleCatalogMessage"
          class="callout"
        >
          {{
            display(roleCatalogMessage)
          }}
        </p>
        <p v-if="writeAccounts && !canAccounts" class="callout">
          {{ t('admin.copy.f738dce4') }}
        </p>
        <ManagementList v-if="canAccounts" :busy="loading" class="listing">
          <template v-if="writeAccounts && roleCatalogLoaded" #actions><button v-if="writeAccounts && roleCatalogLoaded" class="button button-primary" type="button" @click="startAccount">{{ t('admin.copy.0f043095') }}</button></template>
          <ManagementTable
            table-class="account-directory-table"
            :entries="accounts"
            :columns="[
              { key: 'account', label: t('admin.copy.851c6121') },
              { key: 'roles', label: t('admin.copy.cebd8508') },
              { key: 'status', label: t('admin.copy.a765b3ca') },
              { key: 'actions', label: t('admin.copy.d21fcbef') },
            ]"
            :empty="!loading && !error && !accounts.length"
            :empty-label="t('admin.copy.3a1d4b47')"
          >
            <template #account="{ entry: target }"><button type="button" class="text-button" @click="viewedAccount = target">{{ target.username }}</button></template>
            <template #roles="{ entry: target }">
              <div v-for="roleId in target.role_ids" :key="roleId"><AssociationName :record="target" kind="role" :id="roleId" :locale="associationLocale" /></div>
              <span v-if="!target.role_ids.length">{{ t('admin.copy.1b957d47') }}</span>
            </template>
            <template #status="{ entry: target }">
              <span class="status-pill" :class="target.status">{{ target.status === 'active' ? t('admin.copy.98d24810') : t('admin.copy.5e3326db') }}</span>
            </template>
            <template #actions="{ entry: target }"><MemberActionsMenu :id="'brand-account-actions-' + target.id" :label="t('admin.copy.d21fcbef')"><button type="button" role="menuitem" @click="viewedAccount = target">{{ t('admin.copy.01db40da') }}</button><button v-if="writeAccounts && editableAccount(target)" type="button" role="menuitem" @click="editAccount(target)">{{ t('admin.copy.24dfbf6c') }}</button><button v-if="writeAccounts && canResetTarget(target)" type="button" role="menuitem" @click="editAccount(target, true)">{{ t('admin.copy.d1e2ba79') }}</button></MemberActionsMenu></template>
          </ManagementTable>

        <template #pagination><PaginationControls
            :total="accountsTotal" :page="Math.floor(offset / pageSize + 1)" :count="accounts.length" :locale="associationLocale"
            :previous-label="t('admin.copy.dbe7a5d7')"
            :next-label="t('admin.copy.2e9c9e7a')"
            :busy="loading"
            :has-previous="offset > 0"
            :has-next="pageHasNext"
           :page-size="pageSize" :go-to="(target: number) => changePage(target - (Math.floor(offset / pageSize + 1)))" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" /></template></ManagementList>
        <p v-else class="callout">
          {{ t('admin.copy.1a880f7b') }}
        </p>
      </div>
    </template>
    <DetailModal v-if="viewedRole && canRoles" :title="t('association.roleDetails')" :close-label="t('association.close')" @close="viewedRole = null">
      <dl>
        <div><dt>{{ t('admin.copy.6404dc43') }}</dt><dd><AssociationName :record="viewedRole" kind="role" :id="viewedRole.id" :locale="associationLocale" /></dd></div>
        <div><dt>{{ t('admin.copy.a765b3ca') }}</dt><dd>{{ viewedRole.status === 'active' ? t('admin.copy.98d24810') : t('admin.copy.5e3326db') }}</dd></div>
        <div><dt>{{ t('admin.copy.0ede5b86') }}</dt><dd><ul v-if="viewedRole.permissions.length"><li v-for="permission in viewedRole.permissions" :key="permission">{{ permissionLabel(permission, associationLocale) }}</li></ul><span v-else>{{ t('admin.copy.d04292ff') }}</span></dd></div>
      </dl>
    </DetailModal>
  </section>
    <DetailModal v-if="viewedAccount" :title="viewedAccount.username" :close-label="t('association.close')" @close="viewedAccount = null"><dl class="console-facts"><div><dt>{{ t('admin.copy.cebd8508') }}</dt><dd><div v-for="roleId in viewedAccount.role_ids" :key="roleId"><AssociationName :record="viewedAccount" kind="role" :id="roleId" :locale="associationLocale" /></div></dd></div><div><dt>{{ t('admin.copy.abc7e339') }}</dt><dd>{{ accountPermissionDisplay(viewedAccount) }}</dd></div><div><dt>{{ t('admin.copy.a765b3ca') }}</dt><dd>{{ viewedAccount.status === 'active' ? t('admin.copy.98d24810') : t('admin.copy.5e3326db') }}</dd></div></dl></DetailModal>

</template>

<style scoped>

.access-page {
  color: var(--ink);
  width: 100%;
  min-width: 0;
}
.editor-heading {
  scroll-margin-block-start: 24px;
}
.heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin: 4px 0 20px;
}
.heading h1 {
  font-size: 24px;
  margin: 3px 0 5px;
  letter-spacing: -0.5px;
}
.heading p {
  color: var(--muted);
  margin: 0;
}
.eyebrow {
  font-size: var(--body-font-size);
  letter-spacing: 1.2px;
  color: var(--muted);
  font-weight: 700;
}
.brand-tag,
.pill {
  border-radius: 20px;
  background: var(--canvas);
  padding: 7px 11px;
  color: var(--ink-secondary);
  font-size: var(--body-font-size);
  white-space: nowrap;
}

.columns {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
.panel {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 11px;
  padding: 17px;
  min-width: 0;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--ink) 3%, transparent);
}
.panel-title {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 14px;
}
.panel-title > div {
  flex: 1 1 180px;
  min-width: 0;
}
.panel-title > button {
  flex-shrink: 0;
  white-space: normal;
}
.panel-title h2 {
  font-size: 15px;
  margin: 0 0 4px;
}
.panel-title p,
.muted {
  font-size: 11px;
  color: var(--muted);
  margin: 0;
  line-height: 1.6;
}
.form {
  display: grid;
  gap: 13px;
}
.form > label {
  display: grid;
  gap: 6px;
  font-size: var(--body-font-size);
  font-weight: 600;
}
.form input:not([type="checkbox"]), .form textarea {
  width: 100%;
  min-width: 0;
  border: 1px solid var(--line);
  border-radius: 7px;
  background: var(--surface);
  padding: 9px 10px;
  color: var(--ink);
}
.form textarea {
  resize: vertical;
}
.role-form { min-width: 0; }
.role-identity { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; align-items: start; }
.role-identity > label { display: grid; gap: 6px; min-width: 0; }
.role-identity small { color: var(--muted); font-weight: 400; }
.role-record { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 12px; }
.access-page .role-record .record-bottom { margin: 0; }
.role-record .text-button { white-space: normal; overflow-wrap: anywhere; text-align: left; }
.role-record .pill { background: var(--canvas); color: var(--muted); padding: 4px 10px; }
.role-record .pill.active { background: var(--success-bg); color: var(--success-text); }
@media (max-width: 620px) { .role-identity { grid-template-columns: minmax(0, 1fr); } }
.permissions {
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 10px;
  display: grid;
  gap: 8px;
  min-width: 0;
  max-height: 225px;
  overflow: auto;
}
.permissions legend {
  font-size: var(--body-font-size);
  font-weight: 700;
  padding: 0 4px;
}
.check {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  min-width: 0;
  font-size: var(--body-font-size);
  overflow-wrap: anywhere;
}
.check input {
  margin: 1px 0 0;
  flex: none;
}
.check code,
code {
  font-size: var(--body-font-size);
  color: var(--muted);
  overflow-wrap: anywhere;
}
.button-primary {
  border: 0;
  border-radius: 7px;
  background: var(--primary);
  color: var(--on-primary);
  font-weight: 700;
  padding: 10px 12px;
}
.button-primary:hover {
  background: var(--primary-dark);
}
.text-button {
  border: 0;
  background: none;
  color: var(--primary);
  padding: 4px;
  font-size: var(--body-font-size);
  white-space: nowrap;
}
.record {
  padding: 13px 0;
  border-bottom: 1px solid var(--line-subtle);
  min-width: 0;
}
.record-top,
.record-bottom {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.record-top > div {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
  flex-wrap: wrap;
}
.record-top b {
  font-size: var(--body-font-size);
}
.permission-summary {
  font-size: var(--body-font-size);
  line-height: 1.55;
  color: var(--ink-secondary);
  overflow-wrap: anywhere;
  margin: 9px 0;
}
.record-bottom small {
  color: var(--muted);
  font-size: var(--body-font-size);
  overflow-wrap: anywhere;
}
.pill.active {
  background: var(--success-bg);
  color: var(--success-text);
}
.pill.disabled {
  background: var(--canvas);
  color: var(--muted);
}
.inline-actions {
  display: flex;
  gap: 8px;
}
.message,
.callout {
  border-radius: 8px;
  padding: 10px 12px;
  font-size: var(--body-font-size);
  overflow-wrap: anywhere;
}
.message button {
  margin-left: 8px;
  border: 0;
  background: none;
  color: inherit;
  text-decoration: underline;
}
.error {
  background: var(--danger-bg);
  color: var(--danger-text);
}
.success {
  background: var(--success-bg);
  color: var(--success-text);
}
.callout {
  background: var(--canvas);
  color: var(--muted);
}
.union {
  display: grid;
  gap: 6px;
  border-radius: 8px;
  padding: 11px;
  background: var(--canvas);
  font-size: var(--body-font-size);
  overflow-wrap: anywhere;
}
.union span {
  line-height: 1.55;
}
.union small,
.readonly {
  color: var(--muted);
  line-height: 1.5;
}
.readonly {
  font-size: var(--body-font-size);
  margin: 0;
}
@media (max-width: 760px) {
  .columns {
    grid-template-columns: minmax(0, 1fr);
  }
  .heading {
    flex-direction: column;
  }
  .brand-tag {
    white-space: normal;
  }
  .panel {
    padding: 14px;
  }
  .inline-actions {
    gap: 2px;
  }
}
</style>
