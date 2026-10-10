<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import {
  assignableRoles as getAssignableRoles,
  canEditAccountScope,
  canManage,
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
const { t, message } = useAdminI18n();
const section = ref<"roles" | "accounts">(
  canManage(props.account, props.brandId, "role") ||
    canWrite(props.account, props.brandId, "role")
    ? "roles"
    : "accounts",
);
const roles = ref<RoleRecord[]>([]);
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
  reason: "",
});
const accountForm = ref({
  id: "",
  version: 0,
  username: "",
  password: "",
  role_ids: [] as string[],
  status: "active",
  reason: "",
  reset: false,
});
const accountPasswordBytes = computed(
  () => new TextEncoder().encode(accountForm.value.password).length,
);
const roleKeyFor = createBodyKeyTracker();
const accountKeyFor = createBodyKeyTracker();
const canRoles = computed(() =>
  canManage(props.account, props.brandId, "role"),
);
const canAccounts = computed(() =>
  canManage(props.account, props.brandId, "admin"),
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
    ? rolePermissionDisplay(target).join(" · ") || t("无权限", "No permissions")
    : t("无法计算：缺少角色查看权限", "Cannot calculate: role view permission is missing");

function handleError(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401)
    emit("session-invalid");
  const serverError = cause instanceof Error ? cause.message : null;
  error.value = cause instanceof AdminApiError && cause.status === 409
    ? serverError
      ? message("{serverError}；数据版本已变化，请刷新后重试。", "{serverError} The data version changed; refresh and try again.", { serverError })
      : message("数据版本已变化，请刷新后重试。", "The data version changed; refresh and try again.")
    : serverError ?? message("请求失败", "Request failed");
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
        roleCatalogMessage.value = message(
          "角色或权限目录读取失败；目录加载成功后才能配置角色权限。",
          "Could not load the role or permission catalog. Role permissions can be configured after the catalog loads.",
        );
        handleError(cause);
      }
    } else {
      roles.value = [];
      registeredPermissions.value = [];
      roleCatalogLoaded.value = false;
      roleCatalogMessage.value = message(
        "缺少 role.view 权限，无法读取角色与服务端权限目录。",
        "Missing role.view permission: the role and server permission catalogs cannot be loaded.",
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
            roleCatalogMessage.value = message(
              "角色目录读取失败；账号仍可查看，但角色并集无法计算，依赖角色目录的账号变更不可用。",
              "Could not load the role catalog. Accounts remain viewable, but role permissions cannot be calculated and account changes that depend on the catalog are unavailable.",
            );
            handleError(cause);
          }),
      );
    } else {
      roles.value = [];
      roleCatalogLoaded.value = false;
      roleCatalogMessage.value = message(
        "缺少 role.view 权限；账号目录仍可查看，但角色并集无法计算，角色分配、创建账号和角色/状态变更不可用；密码重置仍按账号写权限处理。",
        "Missing role.view permission. The account list remains viewable, but role permissions cannot be calculated; role assignment, account creation, and role/status changes are unavailable. Password resets still follow account write permission.",
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
onMounted(() => void load());
watch(
  () => props.brandId,
  () => {
    roles.value = [];
    accounts.value = [];
    registeredPermissions.value = [];
    roleCatalogLoaded.value = false;
    roleCatalogMessage.value = "";
    offset.value = 0;
    newRole();
    newAccount();
    void load();
  },
);
watch(section, () => {
  offset.value = 0;
  void load();
});
function newRole() {
  roleForm.value = {
    id: "",
    version: 0,
    code: "",
    name: "",
    status: "active",
    permissions: [],
    reason: "",
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
    reason: "",
  };
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
    !form.reason.trim() ||
    (!form.id && form.code.trim().length > 48)
  )
    return;
  const body = form.id
    ? {
        version: form.version,
        name: form.name.trim(),
        status: form.status,
        permissions: [...form.permissions].sort(),
        reason: form.reason.trim(),
      }
    : {
        code: form.code.trim(),
        name: form.name.trim(),
        permissions: [...form.permissions].sort(),
        reason: form.reason.trim(),
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
          reason: string;
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
          reason: string;
        },
        roleKeyFor(body),
      );
    roleKeyFor.clear();
    notice.value = form.id
      ? message("角色已保存。", "Role saved.")
      : message("角色已创建。", "Role created.");
    newRole();
    await load();
  } catch (cause) {
    handleError(cause);
  } finally {
    busy.value = false;
  }
}
function newAccount() {
  accountForm.value = {
    id: "",
    version: 0,
    username: "",
    password: "",
    role_ids: [],
    status: "active",
    reason: "",
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
    reason: "",
    reset,
  };
}
async function saveAccount() {
  const form = accountForm.value;
  if (
    busy.value ||
    !form.reason.trim() ||
    (!form.reset && !roleCatalogLoaded.value)
  )
    return;
  const body = form.id
    ? form.reset
      ? {
          version: form.version,
          password: form.password,
          reason: form.reason.trim(),
        }
      : {
          version: form.version,
          role_ids: [...form.role_ids],
          status: form.status,
          reason: form.reason.trim(),
        }
    : {
        username: form.username.trim(),
        password: form.password,
        role_ids: [...form.role_ids],
        reason: form.reason.trim(),
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
          reason: string;
        },
        accountKeyFor(body),
      );
      emit("created", saved);
      notice.value = message("管理员账号已创建。", "Administrator account created.");
    } else if (form.reset) {
      await api.resetAccountPassword(
        props.brandId,
        form.id,
        body as { version: number; password: string; reason: string },
        accountKeyFor(body),
      );
      notice.value = message("密码重置已提交。", "Password reset submitted.");
    } else {
      await api.updateAccount(
        props.brandId,
        form.id,
        body as {
          version: number;
          role_ids: string[];
          status: string;
          reason: string;
        },
        accountKeyFor(body),
      );
      notice.value = message("管理员账号已更新。", "Administrator account updated.");
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
function changePage(direction: -1 | 1) {
  offset.value = Math.max(0, offset.value + direction * 50);
  void load();
}
</script>

<template>
  <section class="access-page">
    <header class="heading">
      <div>
        <p class="eyebrow">{{ t("ACCESS CONTROL", "ACCESS CONTROL") }}</p>
        <h1>{{ t("账号与权限", "Accounts and permissions") }}</h1>
        <p>{{ t("管理当前品牌的角色权限和后台管理员。", "Manage roles, permissions, and administrator accounts for the current brand.") }}</p>
      </div>
      <span class="brand-tag">{{ t("品牌范围", "Brand scope") }} · {{ brandId || t("未选择", "None selected") }}</span>
    </header>
    <div class="tabs" role="tablist">
      <button
        role="tab"
        :aria-selected="section === 'roles'"
        :disabled="!(canRoles || writeRoles)"
        @click="section = 'roles'"
      >
        {{ t("角色权限", "Roles and permissions") }}</button
      ><button
        role="tab"
        :aria-selected="section === 'accounts'"
        :disabled="!(canAccounts || writeAccounts)"
        @click="section = 'accounts'"
      >
        {{ t("管理员账号", "Administrator accounts") }}
      </button>
    </div>
    <p
      v-if="!(canRoles || writeRoles || canAccounts || writeAccounts)"
      class="callout"
    >
      {{ t("当前账号没有此品牌的角色或管理员查看、写入权限。", "This account cannot view or modify roles or administrators for this brand.") }}
    </p>
    <p v-if="error" class="message error" role="alert">
      {{ t(error) }} <button type="button" @click="load">{{ t("刷新", "Refresh") }}</button>
    </p>
    <p v-if="notice" class="message success" role="status">{{ t(notice) }}</p>

    <template v-if="section === 'roles' && (canRoles || writeRoles)">
      <div class="columns">
        <form v-if="writeRoles" class="panel form" @submit.prevent="saveRole">
          <div class="panel-title">
            <div>
              <h2>{{ roleForm.id ? t("编辑角色", "Edit role") : t("新建角色", "Create role") }}</h2>
              <p>
                {{
                  t("仅可授予当前品牌账号已拥有的登记权限。", "You can grant only registered permissions already held by an account in this brand.")
                }}
              </p>
            </div>
            <button
              v-if="roleForm.id"
              type="button"
              class="text-button"
              @click="newRole"
            >
              {{ t("取消", "Cancel") }}
            </button>
          </div>
          <label v-if="!roleForm.id"
            >{{ t("角色代码（最多 48 字符）", "Role code (up to 48 characters)") }}<input
              v-model.trim="roleForm.code"
              required
              maxlength="48"
              autocomplete="off" /></label
          ><label
            >{{ t("显示名称（最多 120 字节）", "Display name (up to 120 bytes)") }}<input
              v-model.trim="roleForm.name"
              required
            /><small>{{ roleNameBytes }} / 120 {{ t("字节", "bytes") }}</small></label
          >
          <fieldset class="permissions">
            <legend>{{ t("品牌权限", "Brand permissions") }}</legend>
            <label
              v-for="permission in roleChoices"
              :key="permission"
              class="check"
              ><input
                v-model="roleForm.permissions"
                type="checkbox"
                :value="permission"
              />{{ permission }}</label
            >
            <p v-if="!roleChoices.length" class="muted">
              {{ t("没有可登记的授权权限。", "There are no grantable registered permissions.") }}
            </p>
          </fieldset>
          <label v-if="roleForm.id"
            >{{ t("状态", "Status") }}<select v-model="roleForm.status">
              <option value="active">{{ t("启用", "Active") }}</option>
              <option value="disabled">{{ t("停用", "Disabled") }}</option>
            </select></label
          ><label
            >{{ t("变更原因", "Reason for change") }}<textarea
              v-model.trim="roleForm.reason"
              required
              rows="2"
              maxlength="500"
            />
          </label>
          <button
            class="primary"
            :disabled="
              busy ||
              loading ||
              !roleCatalogLoaded ||
              !roleForm.reason.trim() ||
              !roleForm.name.trim() ||
              roleNameBytes > 120 ||
              (!roleForm.id && !roleForm.code.trim())
            "
          >
            {{ busy ? t("保存中…", "Saving…") : roleForm.id ? t("保存角色", "Save role") : t("创建角色", "Create role") }}
          </button>
        </form>
        <div v-if="canRoles" class="panel listing">
          <div class="panel-title">
            <div>
              <h2>{{ t("品牌角色", "Brand roles") }}</h2>
              <p>{{ t("每页 50 条；bootstrap 角色只读。", "50 per page; bootstrap roles are read-only.") }}</p>
            </div>
            <button
              v-if="writeRoles"
              class="text-button"
              type="button"
              @click="newRole"
            >
              {{ t("新建角色", "Create role") }}
            </button>
          </div>
          <p v-if="loading" class="muted">{{ t("正在读取…", "Loading…") }}</p>
          <p v-else-if="!roles.length" class="muted">{{ t("暂无角色。", "No roles yet.") }}</p>
          <article v-for="role in roles" :key="role.id" class="record">
            <div class="record-top">
              <div>
                <b>{{ role.name }}</b
                ><code>{{ role.code }}</code>
              </div>
              <span class="pill" :class="role.status">{{
                role.is_bootstrap
                  ? t("系统只读", "System read-only")
                  : role.status === "active"
                    ? t("启用", "Active")
                    : t("停用", "Disabled")
              }}</span>
            </div>
            <p class="permission-summary">
              {{
                role.permissions.length
                  ? role.permissions.join(" · ")
                  : t("无权限", "No permissions")
              }}
            </p>
            <div class="record-bottom">
              <small
                >{{ t("版本", "Version") }} {{ role.version
                }}<span v-if="role.audit_log_id">
                  · {{ t("审计", "Audit") }} {{ role.audit_log_id }}</span
                ></small
              ><button
                v-if="editableRole(role)"
                class="text-button"
                type="button"
                @click="editRole(role)"
              >
                {{ t("编辑", "Edit") }}</button
              ><span v-else class="muted">{{ t("只读", "Read-only") }}</span>
            </div>
          </article>
          <footer class="pager">
            <span>{{ t("第 {page} 页", "Page {page}", { page: offset / 50 + 1 }) }}</span>
            <div>
              <button
                :disabled="offset === 0 || loading"
                @click="changePage(-1)"
              >
                {{ t("上一页", "Previous") }}</button
              ><button
                :disabled="!pageHasNext || loading"
                @click="changePage(1)"
              >
                {{ t("下一页", "Next") }}
              </button>
            </div>
          </footer>
        </div>
        <p v-else class="callout">
          {{
            t(roleCatalogMessage || message("缺少 role.view 权限；角色列表与完整权限目录不可读取。", "Missing role.view permission: the role list and full permission catalog cannot be loaded."))
          }}
        </p>
      </div>
    </template>

    <template
      v-else-if="section === 'accounts' && (canAccounts || writeAccounts)"
    >
      <div class="columns">
        <form
          v-if="writeAccounts && (roleCatalogLoaded || accountForm.reset)"
          class="panel form"
          @submit.prevent="saveAccount"
        >
          <div class="panel-title">
            <div>
              <h2>
                {{
                  accountForm.id
                    ? accountForm.reset
                      ? t("重置管理员密码", "Reset administrator password")
                      : t("变更管理员", "Edit administrator")
                    : t("创建管理员", "Create administrator")
                }}
              </h2>
              <p>{{ t("新账号仅限当前品牌，不支持创建超级管理员。", "New accounts are limited to this brand; super administrators cannot be created here.") }}</p>
            </div>
            <button
              v-if="accountForm.id"
              class="text-button"
              type="button"
              @click="newAccount"
            >
              {{ t("取消", "Cancel") }}
            </button>
          </div>
          <template v-if="!accountForm.id"
            ><label
              >{{ t("用户名（最多 32 字符）", "Username (up to 32 characters)") }}<input
                v-model.trim="accountForm.username"
                required
                maxlength="32"
                autocomplete="username" /></label
            ><label
              >{{ t("初始密码（16–128 字节）", "Initial password (16–128 bytes)") }}<input
                v-model="accountForm.password"
                type="password"
                required
                autocomplete="new-password"
              /><small>{{ accountPasswordBytes }} {{ t("字节", "bytes") }}</small></label
            ></template
          >
          <template v-else-if="accountForm.reset"
            ><label
              >{{ t("新密码（16–128 字节）", "New password (16–128 bytes)") }}<input
                v-model="accountForm.password"
                type="password"
                required
                autocomplete="new-password"
              /><small>{{ accountPasswordBytes }} {{ t("字节", "bytes") }}</small></label
            ></template
          >
          <template v-else
            ><p class="readonly">
              {{ t("账号", "Account") }}: <b>{{ accountForm.username }}</b>
            </p>
            <label
              >{{ t("状态", "Status") }}<select v-model="accountForm.status">
                <option value="active">{{ t("启用", "Active") }}</option>
                <option value="disabled">{{ t("停用", "Disabled") }}</option>
              </select></label
            ></template
          >
          <fieldset v-if="!accountForm.reset" class="permissions">
            <legend>{{ t("品牌角色（可多选）", "Brand roles (multiple selection)") }}</legend>
            <label v-for="role in assignableRoles" :key="role.id" class="check"
              ><input
                v-model="accountForm.role_ids"
                type="checkbox"
                :value="role.id"
              />{{ role.name }} <code>{{ role.code }}</code></label
            >
            <p v-if="!assignableRoles.length" class="muted">
              {{ t("没有当前账号可安全分配的启用角色。", "There are no active roles this account can safely assign.") }}
            </p>
          </fieldset>
          <div v-if="!accountForm.reset" class="union">
            <b>{{ t("所选角色权限并集", "Combined permissions of selected roles") }}</b
            ><span>{{
              selectedRolePermissionUnion.length
                ? selectedRolePermissionUnion.join(" · ")
                : t("无权限", "No permissions")
            }}</span
            ><small
              >{{ t("权限按角色合并并去重计算；账号不可获得当前操作者尚未拥有的权限。", "Permissions are combined across roles and deduplicated. An account cannot receive permissions the current operator does not have.") }}</small
            >
          </div>
          <label
            >{{ t("变更原因", "Reason for change") }}<textarea
              v-model.trim="accountForm.reason"
              required
              rows="2"
              maxlength="500"
            /></label
          ><button
            class="primary"
            :disabled="
              busy ||
              !accountForm.reason.trim() ||
              ((!accountForm.id || accountForm.reset) &&
                (accountPasswordBytes < 16 || accountPasswordBytes > 128))
            "
          >
            {{
              busy
                ? t("提交中…", "Submitting…")
                : accountForm.id
                  ? accountForm.reset
                    ? t("重置密码", "Reset password")
                    : t("保存变更", "Save changes")
                  : t("创建管理员", "Create administrator")
            }}
          </button>
        </form>
        <p
          v-if="(canAccounts || writeAccounts) && !roleCatalogLoaded"
          class="callout"
        >
          {{
            t(roleCatalogMessage || message("缺少 role.view 权限；管理员目录是否可读取取决于 admin.view，角色并集无法计算。依赖角色目录的创建和变更暂不可用。", "Missing role.view permission: the administrator list depends on admin.view, combined role permissions cannot be calculated, and changes that depend on the role catalog are unavailable."))
          }}
        </p>
        <p v-if="writeAccounts && !canAccounts" class="callout">
          {{ t("当前账号有管理员写权限但没有 admin.view，无法读取管理员目录；此处只可创建新账号，不能选择或查看现有账号。", "This account has administrator write permission but lacks admin.view, so the administrator list cannot be loaded. You can create a new account here, but cannot select or view existing accounts.") }}
        </p>
        <div v-if="canAccounts" class="panel listing">
          <div class="panel-title">
            <div>
              <h2>{{ t("品牌管理员", "Brand administrators") }}</h2>
              <p>{{ t("本人、超级管理员和跨品牌账号只读。", "Your own account, super administrators, and cross-brand accounts are read-only.") }}</p>
            </div>
            <button
              v-if="writeAccounts && roleCatalogLoaded"
              class="text-button"
              type="button"
              @click="newAccount"
            >
              {{ t("新建管理员", "Create administrator") }}
            </button>
          </div>
          <p v-if="loading" class="muted">{{ t("正在读取…", "Loading…") }}</p>
          <p v-else-if="!accounts.length" class="muted">{{ t("暂无管理员。", "No administrators yet.") }}</p>
          <article v-for="target in accounts" :key="target.id" class="record">
            <div class="record-top">
              <div>
                <b>{{ target.username }}</b
                ><code>{{ target.id }}</code>
              </div>
              <span class="pill" :class="target.status">{{
                target.super_admin ? t("超级管理员", "Super administrator") : target.status === "active" ? t("启用", "Active") : t("停用", "Disabled")
              }}</span>
            </div>
            <p class="permission-summary">
              {{ t("角色", "Roles") }}: {{ target.role_codes.join(" · ") || t("未分配", "Unassigned") }}
            </p>
            <p class="permission-summary">
              {{ t("权限并集", "Combined permissions") }}: {{ accountPermissionDisplay(target) }}
            </p>
            <div class="record-bottom">
              <small
                >{{ t("版本", "Version") }} {{ target.version }} ·
                {{ t("{count} 个品牌", "{count} brands", { count: target.brand_ids.length }) }}</small
              >
              <div v-if="writeAccounts" class="inline-actions">
                <button
                  v-if="editableAccount(target)"
                  class="text-button"
                  type="button"
                  @click="editAccount(target)"
                >
                  {{ t("编辑", "Edit") }}</button
                ><button
                  v-if="canResetTarget(target)"
                  class="text-button"
                  type="button"
                  @click="editAccount(target, true)"
                >
                  {{ t("重置密码", "Reset password") }}</button
                ><span
                  v-if="!editableAccount(target) && !canResetTarget(target)"
                  class="muted"
                  >{{ t("只读", "Read-only") }}</span
                >
              </div>
              <span v-else class="muted">{{ t("只读", "Read-only") }}</span>
            </div>
          </article>
          <footer class="pager">
            <span>{{ t("第 {page} 页", "Page {page}", { page: offset / 50 + 1 }) }}</span>
            <div>
              <button
                :disabled="offset === 0 || loading"
                @click="changePage(-1)"
              >
                {{ t("上一页", "Previous") }}</button
              ><button
                :disabled="!pageHasNext || loading"
                @click="changePage(1)"
              >
                {{ t("下一页", "Next") }}
              </button>
            </div>
          </footer>
        </div>
        <p v-else class="callout">
          {{ t("缺少 admin.view 权限；管理员目录不可读取。具备写权限时仍需 role.view 权限才能安全分配角色。", "Missing admin.view permission: the administrator list cannot be loaded. Even with write permission, role.view is required to assign roles safely.") }}
        </p>
      </div>
    </template>
  </section>
</template>

<style scoped>
.access-page {
  --ink: #252a36;
  --sub: #737b8c;
  --line: #e9ebf0;
  --primary: #5969df;
  color: var(--ink);
  width: 100%;
  min-width: 0;
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
  font-size: 10px;
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
  grid-template-columns: minmax(260px, 0.86fr) minmax(0, 1.3fr);
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
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 14px;
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
  background: #fff;
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
  font-size: 10px;
  color: #777f90;
  overflow-wrap: anywhere;
}
.primary {
  border: 0;
  border-radius: 7px;
  background: var(--primary);
  color: white;
  font-weight: 700;
  padding: 10px 12px;
}
.primary:hover {
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
  font-size: 10px;
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
