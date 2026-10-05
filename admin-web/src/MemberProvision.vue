<script setup lang="ts">
import { computed, ref } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  createBodyKeyTracker,
  createManagementApi,
  effectivePermissions,
  type CreatedMember,
  type ManagementAccount,
} from "./management-api";

const props = defineProps<{
  account: AdminAccount & Partial<ManagementAccount>;
  brandId: string;
}>();
const emit = defineEmits<{
  (event: "session-invalid"): void;
  (event: "created", saved: CreatedMember): void;
}>();
const api = createManagementApi();
const keyForBody = createBodyKeyTracker();
const username = ref("");
const phone = ref("");
const password = ref("");
const displayName = ref("");
const notes = ref("");
const reason = ref("");
const busy = ref(false);
const error = ref("");
const saved = ref<CreatedMember | null>(null);
const passwordBytes = computed(
  () => new TextEncoder().encode(password.value).length,
);
const allowed = computed(
  () =>
    !props.account.super_admin &&
    effectivePermissions(props.account, props.brandId).has("user.create.brand"),
);

async function submit() {
  if (
    busy.value ||
    !allowed.value ||
    passwordBytes.value < 10 ||
    passwordBytes.value > 128 ||
    !reason.value.trim()
  )
    return;
  const body: {
    username?: string;
    phone?: string;
    password: string;
    display_name?: string;
    notes?: string;
    reason: string;
  } = { password: password.value, reason: reason.value.trim() };
  if (username.value.trim()) body.username = username.value.trim();
  if (phone.value.trim()) body.phone = phone.value.trim();
  if (displayName.value.trim()) body.display_name = displayName.value.trim();
  if (notes.value.trim()) body.notes = notes.value.trim();
  busy.value = true;
  error.value = "";
  saved.value = null;
  try {
    const result = await api.createMember(
      props.brandId,
      body,
      keyForBody(body),
    );
    keyForBody.clear();
    saved.value = result;
    emit("created", result);
    // Do not retain a credential after the server accepted the one-time create operation.
    password.value = "";
    username.value = "";
    phone.value = "";
    displayName.value = "";
    notes.value = "";
    reason.value = "";
  } catch (cause) {
    if (cause instanceof AdminApiError && cause.status === 401)
      emit("session-invalid");
    error.value = cause instanceof Error ? cause.message : "创建失败";
    if (cause instanceof AdminApiError && cause.status === 409)
      error.value += "；该身份已存在或发生冲突，请核对后再提交。";
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section class="provision">
    <header>
      <div>
        <p class="eyebrow">MEMBER PROVISIONING</p>
        <h1>新建品牌成员</h1>
        <p>建立全局新身份与当前品牌成员关系。</p>
      </div>
      <span class="brand">品牌 · {{ brandId || "未选择" }}</span>
    </header>
    <p v-if="!allowed" class="notice">当前账号没有此品牌的成员创建权限。</p>
    <template v-else>
      <div class="layout">
        <form class="card form" @submit.prevent="submit">
          <h2>成员资料</h2>
          <p class="hint">
            所有资料写入后台真实接口。服务端拒绝已存在的身份，不会覆盖或关联旧账号。
          </p>
          <label
            >用户名 <span>可选</span
            ><input v-model.trim="username" maxlength="32" autocomplete="off"
          /></label>
          <label
            >手机号 <span>可选</span
            ><input
              v-model.trim="phone"
              maxlength="32"
              inputmode="tel"
              autocomplete="off"
          /></label>
          <label
            >初始密码 <b class="required">必填 · 10–128 字节</b
            ><input
              v-model="password"
              type="password"
              required
              autocomplete="new-password"
            /><small
              >当前长度：{{
                passwordBytes
              }}
              字节；密码只在提交期间保存在页面内存。</small
            ></label
          >
          <label
            >显示名称 <span>可选</span
            ><input v-model.trim="displayName" maxlength="120"
          /></label>
          <label
            >内部备注 <span>可选</span
            ><textarea v-model.trim="notes" rows="3" maxlength="1000" />
          </label>
          <label
            >创建原因 <b class="required">必填</b
            ><textarea
              v-model.trim="reason"
              required
              rows="2"
              maxlength="500"
            />
          </label>
          <button
            class="primary"
            :disabled="
              busy ||
              !reason.trim() ||
              passwordBytes < 10 ||
              passwordBytes > 128
            "
          >
            {{ busy ? "正在创建…" : "创建成员" }}
          </button>
        </form>
        <aside class="card facts">
          <h2>创建后的状态</h2>
          <ul>
            <li>仅创建一个全局新用户身份和当前品牌成员。</li>
            <li>新成员余额为零，不会创建资金流水。</li>
            <li>不会登录或签发会话。</li>
            <li>运营人员不能代替用户接受条款。</li>
          </ul>
          <div class="consent">
            <b>首次登录时由本人确认</b>
            <p>
              用户首次登录后须自行阅读并接受当时有效的服务条款和隐私政策，才可继续使用。
            </p>
          </div>
          <p class="hint">
            若提交响应丢失，请使用同一内容重试以沿用幂等键。编辑任一字段后会生成新键；已存在身份会由服务端以
            409 拒绝。
          </p>
        </aside>
      </div>
      <p v-if="error" class="feedback error" role="alert">{{ error }}</p>
      <div v-if="saved" class="feedback success" role="status">
        <b>成员已创建</b
        ><span
          >成员 ID：{{ saved.member_id }} · 用户 ID：{{ saved.user_id }}</span
        ><span>品牌：{{ saved.brand_id }} · 条款已接受：否</span
        ><span>审计记录：{{ saved.audit_log_id }}</span>
      </div>
    </template>
  </section>
</template>

<style scoped>
.provision {
  width: 100%;
  min-width: 0;
  color: #252a36;
}
.provision header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
  margin: 4px 0 18px;
}
.provision h1 {
  font-size: 24px;
  margin: 3px 0 5px;
  letter-spacing: -0.5px;
}
.provision header p {
  color: #737b8c;
  margin: 0;
}
.eyebrow {
  font-size: 10px;
  letter-spacing: 1.2px;
  color: #8991a2;
  font-weight: 700;
}
.brand {
  border-radius: 20px;
  background: #f1f3f8;
  padding: 7px 11px;
  color: #626b7c;
  font-size: 11px;
  overflow-wrap: anywhere;
}
.layout {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(250px, 0.85fr);
  gap: 16px;
  align-items: start;
}
.card {
  background: white;
  border: 1px solid #e9ebf0;
  border-radius: 11px;
  padding: 18px;
  min-width: 0;
  box-shadow: 0 2px 8px #1e2a5008;
}
.form {
  display: grid;
  gap: 13px;
}
.card h2 {
  font-size: 15px;
  margin: 0;
}
.hint {
  font-size: 11px;
  color: #737b8c;
  line-height: 1.6;
  margin: 0;
}
.form label {
  display: grid;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
}
.form label > span {
  font-weight: 400;
  color: #8a91a0;
  display: inline;
}
.form label > b {
  font-size: 10px;
}
.form input,
.form textarea {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  border: 1px solid #dfe3eb;
  border-radius: 7px;
  background: white;
  padding: 9px 10px;
  color: #252a36;
  font: inherit;
}
.form textarea {
  resize: vertical;
}
.form small {
  font-weight: 400;
  color: #8991a2;
}
.required {
  color: #a45540;
}
.primary {
  border: 0;
  border-radius: 7px;
  background: #5969df;
  color: white;
  font-weight: 700;
  padding: 10px 12px;
}
.primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.facts ul {
  padding-left: 19px;
  color: #596172;
  font-size: 12px;
  line-height: 1.8;
}
.consent {
  border-left: 3px solid #5969df;
  background: #f6f7fc;
  border-radius: 0 7px 7px 0;
  padding: 11px;
  margin: 15px 0;
}
.consent b {
  font-size: 12px;
}
.consent p {
  font-size: 11px;
  color: #626b7c;
  line-height: 1.6;
  margin: 5px 0 0;
}
.notice,
.feedback {
  border-radius: 8px;
  padding: 11px 13px;
  font-size: 12px;
  overflow-wrap: anywhere;
}
.notice {
  background: #f5f6fa;
  color: #697184;
}
.feedback {
  margin-top: 13px;
  display: grid;
  gap: 5px;
}
.error {
  background: #fff0ef;
  color: #a83d36;
}
.success {
  background: #edf8f2;
  color: #287553;
}
@media (max-width: 720px) {
  .layout {
    grid-template-columns: minmax(0, 1fr);
  }
  .provision header {
    flex-direction: column;
  }
  .brand {
    white-space: normal;
  }
  .card {
    padding: 14px;
  }
}
</style>
