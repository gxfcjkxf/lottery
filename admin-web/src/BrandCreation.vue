<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { brandCreationPermission, createBrandCreationApi, type BrandCreationBody, type BrandCreationLocale, type CreatedBrandReceipt } from "./brand-creation-api";
import { classifyBrandCreationFailure, clearAllPendingBrandCreationWrites, clearPendingBrandCreationWrite, createBrandCreationRequestGuard, getPendingBrandCreationWrite, retainPendingBrandCreationWrite, updatePendingBrandCreationPhase, type PendingBrandCreationWrite } from "./brand-creation-state";
import { useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount }>();
const { t } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void; (event: "created", receipt: CreatedBrandReceipt): void }>();
const api = createBrandCreationApi();
const writeGuard = createBrandCreationRequestGuard();
const CONTROL_OR_LINE_SEPARATOR_RE = /[\p{Cc}\u2028\u2029]/u;
const code = ref(""), name = ref(""), locale = ref<BrandCreationLocale>("en"), timezone = ref("Asia/Manila"), reason = ref("");
const pending = ref<PendingBrandCreationWrite | null>(null);
const busy = ref(false), error = ref(""), notice = ref("");
const canCreate = computed(() => brandCreationPermission(props.account));
const codeValid = computed(() => /^[a-z][a-z0-9_]{0,47}$/.test(code.value));
const bytes = (value: string) => new TextEncoder().encode(value).length;
const validTimezone = computed(() => {
  if (!timezone.value || timezone.value === "Local" || bytes(timezone.value) > 80 || (timezone.value !== "UTC" && !timezone.value.includes("/"))) return false;
  try { new Intl.DateTimeFormat("en", { timeZone: timezone.value }); return true; } catch { return false; }
});
const validForm = computed(() => codeValid.value && name.value.trim().length > 0 && bytes(name.value) <= 120 &&
  !CONTROL_OR_LINE_SEPARATOR_RE.test(name.value) && (locale.value === "en" || locale.value === "zh-CN") && validTimezone.value &&
  reason.value.trim().length > 0 && !CONTROL_OR_LINE_SEPARATOR_RE.test(reason.value) && bytes(reason.value) <= 500);

function syncPending(): void { pending.value = getPendingBrandCreationWrite(props.account.id); }
watch(() => props.account.id, () => { writeGuard.invalidate(); busy.value = false; error.value = ""; notice.value = ""; syncPending(); }, { immediate: true });
onUnmounted(() => writeGuard.invalidate());

function restoreDraft(body: Readonly<BrandCreationBody>): void {
  code.value = body.code; name.value = body.name; locale.value = body.default_locale; timezone.value = body.timezone; reason.value = body.reason;
}
function localizedText(value: string): string {
  const messages: Record<string, [string, string]> = {
    "当前账号已有待处理创建请求；请先处理该请求。": ["当前账号已有待处理创建请求；请先处理该请求。", "This account already has an unresolved creation request. Resolve it first."],
    "已核验创建回执：该品牌创建时为暂停状态、版本 1。此回执是创建快照，不代表品牌当前状态。": ["已核验创建回执：该品牌创建时为暂停状态、版本 1。此回执是创建快照，不代表品牌当前状态。", "Creation receipt verified: the brand was created paused at version 1. This is a creation snapshot, not the brand's current status."],
    "提交结果未知。请求内容及幂等键已冻结；重试将使用完全相同的请求。": ["提交结果未知。请求内容及幂等键已冻结；重试将使用完全相同的请求。", "The outcome is unknown. The request body and idempotency key are frozen; retry will use the exact same request."],
    "创建请求发生冲突。旧请求已冻结。": ["创建请求发生冲突。旧请求已冻结。", "The creation request conflicted. The original request is frozen."],
    "品牌创建失败。": ["品牌创建失败。", "Brand creation failed."],
  };
  const pair = messages[value];
  return pair ? t(pair[0], pair[1]) : value;
}

function review(): void {
  if (!canCreate.value || !validForm.value || busy.value) return;
  error.value = ""; notice.value = "";
  const body: BrandCreationBody = { code: code.value, name: name.value.trim(), default_locale: locale.value, timezone: timezone.value, reason: reason.value.trim() };
  const candidate: PendingBrandCreationWrite = { accountId: props.account.id, body, key: createIdempotencyKey(), phase: "review" };
  if (!retainPendingBrandCreationWrite(candidate)) { syncPending(); error.value = "当前账号已有待处理创建请求；请先处理该请求。"; return; }
  syncPending();
}
function cancelReview(): void {
  if (!pending.value || pending.value.phase !== "review" || busy.value) return;
  restoreDraft(pending.value.body);
  clearPendingBrandCreationWrite(props.account.id, pending.value.key); syncPending();
}
function discardConflict(): void {
  if (!pending.value || pending.value.phase !== "conflict" || busy.value) return;
  restoreDraft(pending.value.body);
  clearPendingBrandCreationWrite(props.account.id, pending.value.key); syncPending(); error.value = "";
}

async function sendFrozenIntent(retry: boolean): Promise<void> {
  const frozen = pending.value;
  if (!frozen || frozen.accountId !== props.account.id || !canCreate.value || busy.value) return;
  if (retry ? frozen.phase !== "unknown" : frozen.phase !== "review") return;
  const token = writeGuard.begin(); busy.value = true; error.value = ""; notice.value = "";
  updatePendingBrandCreationPhase(frozen.accountId, frozen.key, "unknown");
  syncPending();
  try {
    const receipt = await api.create(frozen.body as BrandCreationBody, frozen.key);
    if (!writeGuard.isCurrent(token) || props.account.id !== frozen.accountId) return;
    clearPendingBrandCreationWrite(frozen.accountId, frozen.key); syncPending();
    notice.value = "已核验创建回执：该品牌创建时为暂停状态、版本 1。此回执是创建快照，不代表品牌当前状态。";
    emit("created", receipt);
  } catch (cause) {
    if (!writeGuard.isCurrent(token) || props.account.id !== frozen.accountId) return;
    if (cause instanceof AdminApiError && cause.status === 401) {
      clearAllPendingBrandCreationWrites(); pending.value = null; emit("session-invalid"); return;
    }
    const status = cause instanceof AdminApiError ? cause.status : undefined;
    const codeValue = cause instanceof AdminApiError ? cause.code : undefined;
    const classification = classifyBrandCreationFailure(status, codeValue);
    if (classification === "unknown") {
      updatePendingBrandCreationPhase(frozen.accountId, frozen.key, "unknown");
      error.value = "提交结果未知。请求内容及幂等键已冻结；重试将使用完全相同的请求。";
    } else if (classification === "conflict") {
      updatePendingBrandCreationPhase(frozen.accountId, frozen.key, "conflict");
      error.value = cause instanceof Error ? cause.message : "创建请求发生冲突。旧请求已冻结。";
    } else {
      clearPendingBrandCreationWrite(frozen.accountId, frozen.key);
      error.value = cause instanceof Error ? cause.message : "品牌创建失败。";
    }
    syncPending();
  } finally {
    if (writeGuard.isCurrent(token)) busy.value = false;
  }
}
</script>

<template>
  <section class="brand-creation" aria-labelledby="brand-creation-title">
    <header class="brand-creation__header">
      <div><div class="brand-creation__eyebrow">PLATFORM / BRAND</div><h2 id="brand-creation-title">{{ t("新建品牌", "Create brand") }}</h2>
        <p>{{ t("新品牌创建后固定为暂停状态，版本 1。", "New brands are created paused at version 1.") }}</p></div>
    </header>
    <p class="brand-creation__policy" role="note">{{ t("创建不会自动添加管理员或成员，也不会配置域名、游戏、资金或结算。创建后需分别完成授权和配置。", "Creation does not add admins or members or configure domains, games, funds, or settlement. Complete access and configuration separately after creation.") }}</p>
    <p v-if="!canCreate" class="brand-creation__notice" role="status">{{ t("当前账号没有平台权限 brand.create.platform。", "This account does not have the platform permission brand.create.platform.") }}</p>
    <template v-else>
      <p v-if="pending?.phase === 'unknown'" class="brand-creation__warning" role="alert">{{ t("提交结果未知：请求内容和幂等键已冻结。只能使用原请求重试。", "The outcome is unknown. The request body and idempotency key are frozen; retry only the original request.") }}</p>
      <p v-else-if="pending?.phase === 'conflict'" class="brand-creation__warning" role="alert">{{ localizedText(error || t("该请求发生冲突，原请求已冻结。", "This request conflicted and is frozen.")) }}</p>
      <p v-else-if="pending?.phase === 'review'" class="brand-creation__notice" role="status">{{ t("请核对以下内容。确认后才会发送创建请求。", "Review the details below. The creation request is sent only after confirmation.") }}</p>
      <form v-if="!pending" class="brand-creation__form" @submit.prevent="review">
        <label class="brand-creation__field" for="brand-create-code"><span>{{ t("品牌代码", "Brand code") }}</span><input id="brand-create-code" v-model="code" autocomplete="off" maxlength="48" :disabled="busy" aria-describedby="brand-create-code-help"><small id="brand-create-code-help">{{ t("小写字母开头，仅小写字母、数字和下划线；不能自动转换大小写。", "Start with a lowercase letter; use only lowercase letters, digits, and underscores. Case is not auto-converted.") }}</small></label>
        <label class="brand-creation__field" for="brand-create-name"><span>{{ t("品牌名称", "Brand name") }}</span><input id="brand-create-name" v-model="name" maxlength="120" :disabled="busy"><small>{{ bytes(name) }} {{ t("/ 120 字节（UTF-8）", "/ 120 UTF-8 bytes") }}</small></label>
        <label class="brand-creation__field" for="brand-create-locale"><span>{{ t("默认语言", "Default locale") }}</span><select id="brand-create-locale" v-model="locale" :disabled="busy"><option value="en">English (en)</option><option value="zh-CN">{{ t("简体中文", "Simplified Chinese") }} (zh-CN)</option></select></label>
        <label class="brand-creation__field" for="brand-create-timezone"><span>{{ t("时区", "Time zone") }}</span><input id="brand-create-timezone" v-model="timezone" autocomplete="off" maxlength="80" :disabled="busy"><small>{{ t("请输入有效的 IANA 时区，例如 Asia/Manila 或 UTC。", "Enter a valid IANA time zone, such as Asia/Manila or UTC.") }}</small></label>
        <label class="brand-creation__field brand-creation__field--wide" for="brand-create-reason"><span>{{ t("创建原因", "Creation reason") }}</span><textarea id="brand-create-reason" v-model="reason" rows="3" maxlength="500" :disabled="busy"></textarea><small>{{ bytes(reason) }} {{ t("/ 500 字节（UTF-8）；不得包含换行或控制字符", "/ 500 UTF-8 bytes; line breaks and control characters are not allowed") }}</small></label>
        <button class="brand-creation__primary" type="submit" :disabled="!validForm || busy">{{ t("核对并继续", "Review and continue") }}</button>
      </form>
      <div v-else-if="pending.phase === 'review'" class="brand-creation__review" role="region" :aria-label="t('核对品牌创建信息', 'Review brand creation details')">
        <dl><div><dt>{{ t("品牌代码", "Brand code") }}</dt><dd>{{ pending.body.code }}</dd></div><div><dt>{{ t("品牌名称", "Brand name") }}</dt><dd>{{ pending.body.name }}</dd></div><div><dt>{{ t("默认语言", "Default locale") }}</dt><dd>{{ pending.body.default_locale }}</dd></div><div><dt>{{ t("时区", "Time zone") }}</dt><dd>{{ pending.body.timezone }}</dd></div><div><dt>{{ t("创建原因", "Creation reason") }}</dt><dd>{{ pending.body.reason }}</dd></div><div><dt>{{ t("初始状态", "Initial status") }}</dt><dd>{{ t("暂停 · 版本 1", "Paused · version 1") }}</dd></div></dl>
        <div class="brand-creation__actions"><button class="brand-creation__primary" type="button" :disabled="busy" @click="sendFrozenIntent(false)">{{ t("确认创建品牌", "Confirm brand creation") }}</button><button class="brand-creation__secondary" type="button" :disabled="busy" @click="cancelReview">{{ t("返回修改", "Back to edit") }}</button></div>
      </div>
      <div v-else-if="pending.phase === 'unknown'" class="brand-creation__actions"><button class="brand-creation__primary" type="button" :disabled="busy" @click="sendFrozenIntent(true)">{{ t("使用原请求重试", "Retry original request") }}</button></div>
      <div v-else-if="pending.phase === 'conflict'" class="brand-creation__actions"><button class="brand-creation__secondary" type="button" :disabled="busy" @click="discardConflict">{{ t("放弃冲突请求并重新填写", "Discard conflicted request and start over") }}</button></div>
    </template>
    <p v-if="error && (!pending || pending.phase === 'review')" class="brand-creation__error" role="alert">{{ localizedText(error) }}</p>
    <p v-if="notice" class="brand-creation__success" role="status">{{ localizedText(notice) }}</p>
  </section>
</template>

<style scoped>
.brand-creation { box-sizing: border-box; min-width: 0; padding: 20px; border: 1px solid #e3e8ee; border-radius: 14px; background: #fff; color: #233044; }
.brand-creation__header { display: flex; justify-content: space-between; gap: 12px; }
.brand-creation__eyebrow { color: #77859a; font-size: 11px; font-weight: 700; letter-spacing: .12em; }
.brand-creation h2 { margin: 5px 0 7px; font-size: 20px; }
.brand-creation p { line-height: 1.5; overflow-wrap: anywhere; }
.brand-creation__header p { margin: 0; color: #64748b; }
.brand-creation__policy, .brand-creation__warning, .brand-creation__notice, .brand-creation__error, .brand-creation__success { padding: 10px 12px; border-radius: 8px; }
.brand-creation__policy { margin: 14px 0; background: #f4f7fb; color: #46566c; }
.brand-creation__warning { background: #fff5df; color: #805600; }.brand-creation__notice { background: #eef5ff; color: #315780; }.brand-creation__error { background: #fff0f0; color: #9a2f2f; }.brand-creation__success { background: #edf8f0; color: #25633b; }
.brand-creation__form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; margin-top: 16px; }
.brand-creation__field { display: flex; min-width: 0; flex-direction: column; gap: 6px; font-weight: 600; }
.brand-creation__field input, .brand-creation__field select, .brand-creation__field textarea { box-sizing: border-box; width: 100%; min-width: 0; border: 1px solid #cbd4df; border-radius: 8px; padding: 9px 10px; background: #fff; color: inherit; font: inherit; }
.brand-creation__field textarea { resize: vertical; }.brand-creation__field small { color: #718096; font-size: 12px; font-weight: 400; overflow-wrap: anywhere; }
.brand-creation__field--wide { grid-column: 1 / -1; }
.brand-creation__primary, .brand-creation__secondary { box-sizing: border-box; max-width: 100%; min-height: 40px; border-radius: 8px; padding: 8px 13px; font: inherit; cursor: pointer; }
.brand-creation__primary { border: 1px solid #245fce; background: #245fce; color: #fff; }.brand-creation__secondary { border: 1px solid #cbd4df; background: #fff; color: #34445a; }
.brand-creation button:disabled { cursor: not-allowed; opacity: .58; }
.brand-creation__review { display: grid; gap: 14px; margin-top: 16px; padding: 14px; border: 1px solid #d8e1ec; border-radius: 10px; background: #fafbfd; }
.brand-creation__review dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin: 0; }
.brand-creation__review dl > div { min-width: 0; }.brand-creation__review dt { color: #718096; font-size: 12px; }.brand-creation__review dd { margin: 4px 0 0; overflow-wrap: anywhere; white-space: pre-wrap; }
.brand-creation__actions { display: flex; flex-wrap: wrap; gap: 10px; }
@media (max-width: 520px) { .brand-creation { padding: 15px; }.brand-creation__form, .brand-creation__review dl { grid-template-columns: minmax(0, 1fr); }.brand-creation__field--wide { grid-column: auto; }.brand-creation__actions { flex-direction: column; }.brand-creation__actions button { width: 100%; } }
</style>
