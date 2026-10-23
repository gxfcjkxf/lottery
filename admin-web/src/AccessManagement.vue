<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { AssociationName } from '@lottery/shared'
import { permissionLabel } from '@lottery/shared'
import DetailModal from '@lottery/shared/DetailModal.vue'
import PermissionPicker from '@lottery/shared/PermissionPicker.vue'
const associationLocale = useAdminI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
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

const props = defineProps<{
  account: AdminAccount & Partial<ManagementAccount>;
  brandId: string;
}>();
const emit = defineEmits<{
  (event: "session-invalid"): void;
  (event: "created", saved: AdminRecord): void;
}>();
const api = createManagementApi();
const { t, message, display } = useAdminI18n();
const section = ref<"roles" | "accounts">(
  canView(props.account, props.brandId, "role") ||
    canWrite(props.account, props.brandId, "role")
    ? "roles"
    : "accounts",
);
const roleEditorOpen = ref(false);
const accountEditorOpen = ref(false);
function startRole() { newRole(); error.value = ""; roleEditorOpen.value = true; }
function startAccount() { newAccount(); error.value = ""; accountEditorOpen.value = true; }
const roles = ref<RoleRecord[]>([]);
const viewedRole = ref<RoleRecord | null>(null);
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
const accountPasswordBytes = computed(
  () => new TextEncoder().encode(accountForm.value.password).length,
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
    ? roles.value.length === 50
    : accounts.value.length === 50,
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
const roleNameBytes = computed(
  () => new TextEncoder().encode(roleForm.value.name).length,
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
async function load() {
  const requestBrandId = props.brandId;
  if (
    !requestBrandId ||
    !(
      canRoles.value ||
      canAccounts.value ||
      writeRoles.value ||
      writeAccounts.value
    )
  )
    return;
  loading.value = true;
  error.value = "";
  if (section.value === "roles") {
    if (canRoles.value) {
      try {
        const [listed, perms] = await Promise.all([
          api.roles(requestBrandId, 50, offset.value),
          api.permissions(requestBrandId),
        ]);
        if (props.brandId !== requestBrandId) return;
        roles.value = listed.items;
        registeredPermissions.value = perms.items;
        roleCatalogLoaded.value = true;
        roleCatalogMessage.value = "";
      } catch (cause) {
        if (props.brandId !== requestBrandId) return;
        roles.value = [];
        registeredPermissions.value = [];
        roleCatalogLoaded.value = false;
        roleCatalogMessage.value = message('admin.copy.afb348bc',
        );
        handleError(cause);
      }
    } else {
      roles.value = [];
      registeredPermissions.value = [];
      roleCatalogLoaded.value = false;
      roleCatalogMessage.value = message('admin.copy.56c87a94',
      );
    }
  }
  if (section.value === "accounts") {
    const tasks: Promise<void>[] = [];
    if (canAccounts.value)
      tasks.push(
        api
          .accounts(requestBrandId, 50, offset.value)
          .then((listed) => {
            if (props.brandId === requestBrandId) accounts.value = listed.items;
          })
          .catch((cause: unknown) => {
            if (props.brandId === requestBrandId) handleError(cause);
          }),
      );
    if (canRoles.value) {
      tasks.push(
        loadRoleCatalog(requestBrandId)
          .then((catalog) => {
            if (props.brandId !== requestBrandId) return;
            roles.value = catalog;
            roleCatalogLoaded.value = true;
            roleCatalogMessage.value = "";
          })
          .catch((cause: unknown) => {
            if (props.brandId !== requestBrandId) return;
            roleCatalogLoaded.value = false;
            roleCatalogMessage.value = message('admin.copy.50575ea4',
            );
            handleError(cause);
          }),
      );
    } else {
      roles.value = [];
      roleCatalogLoaded.value = false;
      roleCatalogMessage.value = message('admin.copy.48fe4041',
      );
    }
    await Promise.all(tasks);
  }
  loading.value = false;
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
  () => props.brandId,
  () => {
    viewedRole.value = null;
    roles.value = [];
    accounts.value = [];
    registeredPermissions.value = [];
    roleCatalogLoaded.value = false;
    roleCatalogMessage.value = "";
    offset.value = 0;
    newRole();
    newAccount();
    void withGlobalLoading(load);
  },
);
watch(section, () => {
  roleEditorOpen.value = false; accountEditorOpen.value = false;
  viewedRole.value = null;
  offset.value = 0;
  void load();
});
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
    roleNameBytes.value > 120 ||
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
async function changePage(direction: -1 | 1) {
  offset.value = Math.max(0, offset.value + direction * 50);
  await load();
}
</script>

<template>
  <section class="access-page">
    <header class="heading">
      <div>
        <h1>{{ t('admin.copy.9c6ee39c') }}</h1>
      </div>
    </header>
    <div class="tabs" role="tablist">
      <button
        role="tab"
        :aria-selected="section === 'roles'"
        :disabled="!(canRoles || writeRoles)"
        @click="section = 'roles'"
      >
        {{ t('admin.copy.78c84d5e') }}</button
      ><button
        role="tab"
        :aria-selected="section === 'accounts'"
        :disabled="!(canAccounts || writeAccounts)"
        @click="section = 'accounts'"
      >
        {{ t('admin.copy.f8e4afe4') }}
      </button>
    </div>
    <p
      v-if="!(canRoles || writeRoles || canAccounts || writeAccounts)"
      class="callout"
    >
      {{ t('admin.copy.ce325bac') }}
    </p>
    <p v-if="error && !roleEditorOpen && !accountEditorOpen" class="message error" role="alert">
      {{ display(error) }} <button v-refresh-button="t('admin.copy.894e2a7d')" type="button" @click="withGlobalLoading(load)">{{ t('admin.copy.894e2a7d') }}</button>
    </p>
    <p v-if="notice" class="message success" role="status">{{ display(notice) }}</p>

    <template v-if="section === 'roles' && (canRoles || writeRoles)">
      <div class="columns">
        <button v-if="writeRoles && !canRoles" class="button button-secondary" type="button" @click="startRole">{{ t('admin.copy.8c7fddfb') }}</button>
        <DetailModal v-if="writeRoles && roleEditorOpen" :title="roleForm.id ? t('admin.copy.0233b2e8') : t('admin.copy.8c7fddfb')" :close-label="t('association.close')" @close="!busy && (roleEditorOpen = false)">
          <p v-if="error" class="message error" role="alert">{{ display(error) }}</p>
        <form v-write-guard="busy" novalidate v-inline-validation v-if="writeRoles" class="panel form" @submit.prevent="withGlobalLoading(saveRole)">
          <label v-if="!roleForm.id"
            >{{ t('admin.copy.8c47c503') }}<input
              v-model.trim="roleForm.code"
              required
              maxlength="48"
              autocomplete="off" /></label
          ><label
            >{{ t('admin.copy.6404dc43') }}<input
              v-model.trim="roleForm.name"
              required
            /><small>{{ roleNameBytes }} / 120 {{ t('admin.copy.b2ca2f37') }}</small></label
          >
          <PermissionPicker v-model="roleForm.permissions" :options="roleChoices" :locale="associationLocale" />
          <label v-if="roleForm.id"
            >{{ t('admin.copy.a765b3ca') }}<select v-model="roleForm.status">
              <option value="active">{{ t('admin.copy.98d24810') }}</option>
              <option value="disabled">{{ t('admin.copy.5e3326db') }}</option>
            </select></label>
          <button
            class="button button-primary"
            :disabled="
              busy ||
              loading ||
              !roleCatalogLoaded ||
              !roleForm.name.trim() ||
              roleNameBytes > 120 ||
              (!roleForm.id && !roleForm.code.trim())
            "
          >
            {{ busy ? t('admin.copy.2462134b') : roleForm.id ? t('admin.copy.383c86a7') : t('admin.copy.80f397e0') }}
          </button>
        </form>
        </DetailModal>
        <div v-if="canRoles" class="panel listing">
          <div class="panel-title">
            <div>
              <h2>{{ t('admin.copy.efb1fda4') }}</h2>
            </div>
            <button
              v-if="writeRoles"
              class="text-button"
              type="button"
              @click="startRole"
            >
              {{ t('admin.copy.8c7fddfb') }}
            </button>
          </div>
          <p v-if="!roles.length" class="muted empty-state">{{ t('admin.copy.5c979929') }}</p>
          <article v-for="role in roles" :key="role.id" class="record">
            <div class="record-top">
              <div>
                <button type="button" class="text-button" @click="viewedRole = role"><AssociationName :record="role" kind="role" :id="role.id" :locale="associationLocale" /></button>
              </div>
              <span class="pill" :class="role.status">{{
                 role.status === "active"
                    ? t('admin.copy.98d24810')
                    : t('admin.copy.5e3326db')
              }}</span>
            </div>
            <div class="record-bottom">
              <button
                v-if="editableRole(role)"
                class="text-button"
                type="button"
                @click="editRole(role)"
              >
                {{ t('admin.copy.24dfbf6c') }}</button
              >
            </div>
          </article>
          <footer class="pager">
            <span>{{ t('admin.copy.f3d61d27', { page: offset / 50 + 1 }) }}</span>
            <div>
              <button
                :disabled="offset === 0 || loading"
                @click="withGlobalLoading(changePage, -1)"
              >
                {{ t('admin.withdrawal.previous') }}</button
              ><button
                :disabled="!pageHasNext || loading"
                @click="withGlobalLoading(changePage, 1)"
              >
                {{ t('admin.withdrawal.next') }}
              </button>
            </div>
          </footer>
        </div>
        <p v-else class="callout">
          {{
            display(roleCatalogMessage || message('admin.copy.4f2155ad'))
          }}
        </p>
      </div>
    </template>

    <template
      v-else-if="section === 'accounts' && (canAccounts || writeAccounts)"
    >
      <div class="columns">
        <button v-if="writeAccounts && !canAccounts && roleCatalogLoaded" class="button button-secondary" type="button" @click="startAccount">{{ t('admin.copy.0f043095') }}</button>
        <DetailModal v-if="writeAccounts && accountEditorOpen && (roleCatalogLoaded || accountForm.reset)" :title="accountForm.id ? accountForm.reset ? t('admin.copy.1945c635') : t('admin.copy.b74a3dd7') : t('admin.copy.d2a40bda')" :close-label="t('association.close')" @close="!busy && (accountEditorOpen = false)">
          <p v-if="error" class="message error" role="alert">{{ display(error) }}</p>
        <form v-write-guard="busy" novalidate v-inline-validation
          v-if="writeAccounts && (roleCatalogLoaded || accountForm.reset)"
          class="panel form"
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
                v-model="accountForm.password"
                type="password"
                required
                autocomplete="new-password"
              /><small>{{ accountPasswordBytes }} {{ t('admin.copy.b2ca2f37') }}</small></label
            ></template
          >
          <template v-else-if="accountForm.reset"
            ><label
              >{{ t('admin.copy.683caabd') }}<input
                v-model="accountForm.password"
                type="password"
                required
                autocomplete="new-password"
              /><small>{{ accountPasswordBytes }} {{ t('admin.copy.b2ca2f37') }}</small></label
            ></template
          >
          <template v-else
            ><p class="readonly">
              {{ t('admin.copy.851c6121') }}: <b>{{ accountForm.username }}</b>
            </p>
            <label
              >{{ t('admin.copy.a765b3ca') }}<select v-model="accountForm.status">
                <option value="active">{{ t('admin.copy.98d24810') }}</option>
                <option value="disabled">{{ t('admin.copy.5e3326db') }}</option>
              </select></label
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
            ><small
              >{{ t('admin.copy.ebec9012') }}</small
            >
          </div>
          <button
            class="button button-primary"
            :disabled="
              busy ||
              ((!accountForm.id || accountForm.reset) &&
                (accountPasswordBytes < 6 || accountPasswordBytes > 30))
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
          v-if="(canAccounts || writeAccounts) && !roleCatalogLoaded"
          class="callout"
        >
          {{
            display(roleCatalogMessage || message('admin.copy.d6eded98'))
          }}
        </p>
        <p v-if="writeAccounts && !canAccounts" class="callout">
          {{ t('admin.copy.f738dce4') }}
        </p>
        <div v-if="canAccounts" class="panel listing">
          <div class="panel-title">
            <div>
              <h2>{{ t('admin.copy.09cda972') }}</h2>
            </div>
            <button
              v-if="writeAccounts && roleCatalogLoaded"
              class="text-button"
              type="button"
              @click="startAccount"
            >
              {{ t('admin.copy.0f043095') }}
            </button>
          </div>
          <p v-if="!accounts.length" class="muted empty-state">{{ t('admin.copy.3a1d4b47') }}</p>
          <article v-for="target in accounts" :key="target.id" class="record">
            <div class="record-top">
              <div>
                <b>{{ target.username }}</b
                >
              </div>
              <span class="pill" :class="target.status">{{
                target.status === "active" ? t('admin.copy.98d24810') : t('admin.copy.5e3326db')
              }}</span>
            </div>
            <p class="permission-summary">
              {{ t('admin.copy.cebd8508') }}: <span v-for="roleId in target.role_ids" :key="roleId" class="role-name"><AssociationName :record="target" kind="role" :id="roleId" :locale="associationLocale" /> </span><span v-if="!target.role_ids.length">{{ t('admin.copy.1b957d47') }}</span>
            </p>
            <details class="permission-summary">
              <summary>{{ t('admin.copy.abc7e339') }}</summary>
              <p>{{ accountPermissionDisplay(target) }}</p>
            </details>
            <div class="record-bottom">

              <div v-if="writeAccounts" class="inline-actions">
                <button
                  v-if="editableAccount(target)"
                  class="text-button"
                  type="button"
                  @click="editAccount(target)"
                >
                  {{ t('admin.copy.24dfbf6c') }}</button
                ><button
                  v-if="canResetTarget(target)"
                  class="text-button"
                  type="button"
                  @click="editAccount(target, true)"
                >
                  {{ t('admin.copy.d1e2ba79') }}</button
                ><span
                  v-if="!editableAccount(target) && !canResetTarget(target)"
                  class="muted"
                  >{{ t('admin.copy.9be4e033') }}</span
                >
              </div>
              <span v-else class="muted">{{ t('admin.copy.9be4e033') }}</span>
            </div>
          </article>
          <footer class="pager">
            <span>{{ t('admin.copy.f3d61d27', { page: offset / 50 + 1 }) }}</span>
            <div>
              <button
                :disabled="offset === 0 || loading"
                @click="withGlobalLoading(changePage, -1)"
              >
                {{ t('admin.copy.dbe7a5d7') }}</button
              ><button
                :disabled="!pageHasNext || loading"
                @click="withGlobalLoading(changePage, 1)"
              >
                {{ t('admin.copy.2e9c9e7a') }}
              </button>
            </div>
          </footer>
        </div>
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
</template>

<style scoped>
.access-page {
  --ink: #252a36;
  --sub: var(--muted);
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
  color: var(--sub);
  margin: 0;
}
.eyebrow {
  font-size: 11px;
  letter-spacing: 1.2px;
  color: #8991a2;
  font-weight: 700;
}
.brand-tag,
.pill {
  border-radius: 20px;
  background: #f1f3f8;
  padding: 7px 11px;
  color: #626b7c;
  font-size: 11px;
  white-space: nowrap;
}
.tabs {
  display: flex;
  gap: 6px;
  border-bottom: 1px solid var(--line);
  margin-bottom: 16px;
}
.tabs button {
  border: 0;
  background: none;
  padding: 11px 14px;
  color: var(--sub);
  border-bottom: 2px solid transparent;
}
.tabs button[aria-selected="true"] {
  color: var(--primary);
  border-bottom-color: var(--primary);
  font-weight: 700;
}
.columns {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
.panel {
  background: white;
  border: 1px solid var(--line);
  border-radius: 11px;
  padding: 17px;
  min-width: 0;
  box-shadow: 0 2px 8px #1e2a5008;
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
  color: var(--sub);
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
  font-size: 11px;
  font-weight: 600;
}
.form input:not([type="checkbox"]),
.form select,
.form textarea {
  width: 100%;
  min-width: 0;
  border: 1px solid #dfe3eb;
  border-radius: 7px;
  background: var(--surface);
  padding: 9px 10px;
  color: var(--ink);
}
.form textarea {
  resize: vertical;
}
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
  font-size: 11px;
  font-weight: 700;
  padding: 0 4px;
}
.check {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  min-width: 0;
  font-size: 11px;
  overflow-wrap: anywhere;
}
.check input {
  margin: 1px 0 0;
  flex: none;
}
.check code,
code {
  font-size: 11px;
  color: #777f90;
  overflow-wrap: anywhere;
}
.button-primary {
  border: 0;
  border-radius: 7px;
  background: var(--primary);
  color: white;
  font-weight: 700;
  padding: 10px 12px;
}
.button-primary:hover {
  background: #4858cf;
}
.text-button {
  border: 0;
  background: none;
  color: var(--primary);
  padding: 4px;
  font-size: 11px;
  white-space: nowrap;
}
.record {
  padding: 13px 0;
  border-bottom: 1px solid #f0f1f4;
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
  font-size: 12px;
}
.permission-summary {
  font-size: 11px;
  line-height: 1.55;
  color: #626b7c;
  overflow-wrap: anywhere;
  margin: 9px 0;
}
.record-bottom small {
  color: #969dac;
  font-size: 11px;
  overflow-wrap: anywhere;
}
.pill.active {
  background: #e9f6f0;
  color: #258763;
}
.pill.disabled {
  background: #f3f3f5;
  color: #818896;
}
.pager {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 13px;
  color: var(--sub);
  font-size: 11px;
}
.pager div,
.inline-actions {
  display: flex;
  gap: 8px;
}
.pager button {
  border: 1px solid var(--line);
  background: white;
  border-radius: 6px;
  padding: 6px 9px;
  font-size: 11px;
}
.pager button:disabled {
  opacity: 0.45;
}
.message,
.callout {
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 12px;
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
  background: #fff0ef;
  color: #a83d36;
}
.success {
  background: #edf8f2;
  color: #287553;
}
.callout {
  background: #f5f6fa;
  color: #697184;
}
.union {
  display: grid;
  gap: 6px;
  border-radius: 8px;
  padding: 11px;
  background: #f6f7fc;
  font-size: 11px;
  overflow-wrap: anywhere;
}
.union span {
  line-height: 1.55;
}
.union small,
.readonly {
  color: var(--sub);
  line-height: 1.5;
}
.readonly {
  font-size: 11px;
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
