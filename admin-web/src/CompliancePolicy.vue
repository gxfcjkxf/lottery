<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import DetailModal from '@lottery/shared/DetailModal.vue';
import { vGlobalLoading } from "@lottery/shared";
import { vInlineValidation } from '@lottery/shared'
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { compliancePermissions, createCompliancePolicyApi, validComplianceConfig, validComplianceReason, type ComplianceConfig, type ComplianceDecision, type ComplianceOperation, type CompliancePolicy } from "./compliance-api";
import { classifyComplianceFailure, clearPendingComplianceIntent, complianceSessionGeneration, createComplianceRequestGuard, getPendingComplianceIntent, setPendingComplianceIntent, updatePendingCompliancePhase, type PendingComplianceCheck, type PendingComplianceIntent, type PendingPolicyWrite } from "./compliance-state";
import { adminKey, useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const { t, message, display } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createCompliancePolicyApi();
const policyGuard = createComplianceRequestGuard(), writeGuard = createComplianceRequestGuard();
const rights = computed(() => compliancePermissions(props.account, props.brandId));
const scope = () => ({ accountId: props.account.id, brandId: props.brandId });
const policy = ref<CompliancePolicy | null>(null), draft = ref<ComplianceConfig | null>(null), reason = ref("");
const checkResult = ref<ComplianceDecision | null>(null), readFailed = ref(false);
const confirmationKind = ref<"policy" | "check" | null>(null);
const operation = ref<ComplianceOperation>("registration"), checkReason = ref("");
const pendingPolicy = ref<PendingPolicyWrite | null>(null), pendingCheck = ref<PendingComplianceCheck | null>(null);
const loadingPolicy = ref(false), busy = ref(false);
const conflictPolicyRead = ref(false);
const policyError = ref<string | import("@lottery/shared").LocalizedMessage>(""), actionError = ref<string | import("@lottery/shared").LocalizedMessage>(""), notice = ref<string | import("@lottery/shared").LocalizedMessage>("");
const activeIntent = computed(() => confirmationKind.value === 'policy' ? pendingPolicy.value : confirmationKind.value === 'check' ? pendingCheck.value : null);
const policyEditable = computed(() => rights.value.writePolicy && Boolean(policy.value) && !pendingPolicy.value && !busy.value && !loadingPolicy.value && !readFailed.value);
const checkEditable = computed(() => rights.value.runCheck && Boolean(policy.value) && !pendingCheck.value && !busy.value && !loadingPolicy.value && !readFailed.value);
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
const operationLabel = (value: string) => value === "registration" ? t('admin.copy.f59482ae') : value === "betting" ? t('admin.copy.00b6328f') : value === "withdrawal" ? t('admin.copy.58c4aae8') : value;
const decisionLabel = (value: string) => value === "allow" ? t('admin.copy.068bad15') : value === "deny" ? t('admin.copy.7bdeb5ba') : value === "review" ? t('admin.copy.7d8c6fd4') : value;
const checkLabel = (value: string) => value === "age" ? t('admin.copy.33133506') : value === "region" ? t('admin.copy.c0bb2de5') : value === "identity" ? t('admin.copy.8b4d44c1') : value === "account_risk" ? t('admin.copy.f7227153') : value === "betting_risk" ? t('admin.copy.8c4c1db1') : value === "exclusion" ? t('admin.copy.bdbcd659') : value === "responsible_gambling" ? t('admin.copy.a72f31f4') : value;
function localizedMessage(value: string | import("@lottery/shared").LocalizedMessage): string {
  if (typeof value !== "string") return displayPublicError(value);
  const translations: Record<string, string> = {
    "读取政策失败": 'admin.copy.d7ce9f3b', "读取政策历史失败": 'admin.copy.4ac2348f',
    "读取检查记录失败": 'admin.copy.1322b70e', "读取业务闸门记录失败": 'admin.copy.190df221',
    "请核对配置，并填写无首尾空白、不含控制字符且最多500 UTF-8字节的原因。": 'admin.copy.f6a413ae',
    "已有未解决的政策意图；请先使用原键处理，不能替换其正文或键。": 'admin.copy.ed7a4b4f',
    "请填写无首尾空白、不含控制字符且最多500 UTF-8字节的检查原因。": 'admin.copy.5be4d6ff',
    "已有未解决的检查意图；请先使用原键处理，不能替换其正文或键。": 'admin.copy.9d607ff9',
    "政策写入回执已核验；正在独立刷新政策与历史。": 'admin.copy.ae6eb096',
    "伪适配器决定已记录；不代表真实注册、投注或提现拦截。正在独立刷新检查记录。": 'admin.copy.2d092d9c',
    "提交结果未知。正文和原键已冻结于当前内存会话；请用原键重试，切换页面不会解除。": 'admin.copy.c4b2869e',
    "品牌状态冲突（例如品牌已停用）。意图仍被冻结；不会修改政策或品牌状态。请核实品牌状态并读取最新政策后，再明确放弃旧意图。": 'admin.copy.823388d9',
    "政策版本冲突。意图仍被冻结；不会自动套用草稿或换键重试。读取最新政策后可明确放弃旧意图。": 'admin.copy.3a604de0',
    "请求被拒绝。": 'admin.copy.29e78fed',
  };
  const pair = translations[value];
  return pair ? t(adminKey(pair)) : value;
}
let live = true;
function current(guard: ReturnType<typeof createComplianceRequestGuard>, token: number, accountId: string, brandId: string, epoch: number) { return live && guard.isCurrent(token) && epoch === complianceSessionGeneration() && props.account.id === accountId && props.brandId === brandId; }
function sessionInvalid(cause: unknown, guard: ReturnType<typeof createComplianceRequestGuard>, token: number, accountId: string, brandId: string, epoch: number) { if (cause instanceof AdminApiError && cause.status === 401 && current(guard, token, accountId, brandId, epoch)) emit("session-invalid"); }
async function readPolicy() {
  const { accountId, brandId } = scope(), epoch = complianceSessionGeneration(), token = policyGuard.begin(); loadingPolicy.value = true; policyError.value = "";
  try { const value = await api.get(brandId); if (!current(policyGuard, token, accountId, brandId, epoch)) return false; policy.value = value; readFailed.value = false; if (!pendingPolicy.value) draft.value = clone(value.config); return true; }
  catch (cause) { if (current(policyGuard, token, accountId, brandId, epoch)) { readFailed.value = true; policyError.value = cause instanceof Error ? publicErrorMessage(cause, "read") : "读取政策失败"; sessionInvalid(cause, policyGuard, token, accountId, brandId, epoch); } return false; }
  finally { if (current(policyGuard, token, accountId, brandId, epoch)) loadingPolicy.value = false; }
}
function preparePolicy() {
  actionError.value = ""; notice.value = "";
  if (!policyEditable.value || !policy.value || !draft.value || !validComplianceConfig(draft.value) || !validComplianceReason(reason.value)) { actionError.value = "请核对配置，并填写无首尾空白、不含控制字符且最多500 UTF-8字节的原因。"; return; }
  const accepted = setPendingComplianceIntent({ ...scope(), kind: "policy", body: { version: policy.value.version, config: clone(draft.value), reason: reason.value }, key: createIdempotencyKey(), phase: "review" });
  if (!accepted) { actionError.value = "已有未解决的政策意图；请先使用原键处理，不能替换其正文或键。"; return; }
  pendingPolicy.value = getPendingComplianceIntent(scope(), "policy");confirmationKind.value="policy";
}
function prepareCheck() {
  actionError.value = ""; notice.value = "";
  if (!checkEditable.value || !policy.value || !validComplianceReason(checkReason.value)) { actionError.value = "请填写无首尾空白、不含控制字符且最多500 UTF-8字节的检查原因。"; return; }
  const accepted = setPendingComplianceIntent({ ...scope(), kind: "check", body: { version: policy.value.version, operation: operation.value, reason: checkReason.value }, key: createIdempotencyKey(), phase: "review" });
  if (!accepted) { actionError.value = "已有未解决的检查意图；请先使用原键处理，不能替换其正文或键。"; return; }
  pendingCheck.value = getPendingComplianceIntent(scope(), "check");confirmationKind.value="check";checkResult.value=null;
}
function updateMinimumAge(event: Event) { if (!draft.value) return; const value = (event.target as HTMLInputElement).value; draft.value.minimum_age = value === "" ? null : Number(value); }
function cancel(kind: "policy" | "check") { const intent = kind === "policy" ? pendingPolicy.value : pendingCheck.value; if (busy.value || !intent || intent.phase === "unknown" || intent.phase === "conflict" && !conflictPolicyRead.value) return; clearPendingComplianceIntent(scope(), kind, intent.key); if (kind === "policy") { pendingPolicy.value = null; if (intent.phase === "conflict" && policy.value) draft.value = clone(policy.value.config); } else pendingCheck.value = null; conflictPolicyRead.value = false;confirmationKind.value=null; }
function closeConfirmation(){if(busy.value)return;const kind=confirmationKind.value;if(kind&&activeIntent.value?.phase==='review')cancel(kind);else confirmationKind.value=null}
async function send(kind: "policy" | "check", retry = false) {
  const intent = kind === "policy" ? pendingPolicy.value : pendingCheck.value;
  if (!intent || busy.value || retry && intent.phase !== "unknown" || !retry && intent.phase !== "review") return;
  if (kind === "policy" ? !rights.value.writePolicy : !rights.value.runCheck) return;
  const origin = { accountId: intent.accountId, brandId: intent.brandId };
  const epoch = complianceSessionGeneration(), token = writeGuard.begin(); busy.value = true; actionError.value = ""; notice.value = "";
  updatePendingCompliancePhase(origin, kind, intent.key, "unknown");
  if (kind === "policy") pendingPolicy.value = getPendingComplianceIntent(origin, "policy"); else pendingCheck.value = getPendingComplianceIntent(origin, "check");
  try {
    const result = intent.kind === "policy" ? await api.put(intent.brandId, intent.body, intent.key) : await api.run(intent.brandId, intent.body, intent.key);
    if (epoch !== complianceSessionGeneration()) return;
    clearPendingComplianceIntent({ accountId: intent.accountId, brandId: intent.brandId }, kind, intent.key);
    if (!current(writeGuard, token, intent.accountId, intent.brandId, epoch)) return;
    pendingPolicy.value = getPendingComplianceIntent(scope(), "policy"); pendingCheck.value = getPendingComplianceIntent(scope(), "check");
    confirmationKind.value=null;
    if (kind === "policy") {
      const refreshed=await readPolicy();
      notice.value=message(refreshed?'admin.settings.saved':'admin.settings.savedReadFailed');
    } else { checkResult.value=result as ComplianceDecision;notice.value=message('admin.settings.checkComplete'); }
  } catch (cause) {
    if (epoch !== complianceSessionGeneration()) return;
    const result = classifyComplianceFailure(cause instanceof AdminApiError ? cause.status : undefined, cause instanceof AdminApiError ? cause.code : undefined);
    if (result !== "definitive") updatePendingCompliancePhase(origin, kind, intent.key, result);
    else clearPendingComplianceIntent(origin, kind, intent.key);
    if (!current(writeGuard, token, intent.accountId, intent.brandId, epoch)) return;
    pendingPolicy.value = getPendingComplianceIntent(scope(), "policy"); pendingCheck.value = getPendingComplianceIntent(scope(), "check");
    if (!activeIntent.value) confirmationKind.value=null;
    actionError.value = result === "unknown" ? "提交结果未知。正文和原键已冻结于当前内存会话；请用原键重试，切换页面不会解除。" : result === "conflict" ? cause instanceof AdminApiError && cause.code === "COMPLIANCE_STATE_CONFLICT" ? "品牌状态冲突（例如品牌已停用）。意图仍被冻结；不会修改政策或品牌状态。请核实品牌状态并读取最新政策后，再明确放弃旧意图。" : "政策版本冲突。意图仍被冻结；不会自动套用草稿或换键重试。读取最新政策后可明确放弃旧意图。" : cause instanceof Error ? publicErrorMessage(cause) : "请求被拒绝。";
    if (result === "conflict") { conflictPolicyRead.value = false; conflictPolicyRead.value = await readPolicy(); }
    sessionInvalid(cause, writeGuard, token, intent.accountId, intent.brandId, epoch);
  } finally { if (current(writeGuard, token, intent.accountId, intent.brandId, epoch)) busy.value = false; }
}
watch(() => [props.account.id, props.brandId], () => {
  policyGuard.invalidate(); writeGuard.invalidate();
  policy.value = null; checkResult.value=null;readFailed.value=false; conflictPolicyRead.value = false;
  policyError.value = ""; actionError.value = ""; notice.value = ""; loadingPolicy.value = false; busy.value = false;
  pendingPolicy.value = getPendingComplianceIntent(scope(), "policy") as PendingPolicyWrite | null; pendingCheck.value = getPendingComplianceIntent(scope(), "check") as PendingComplianceCheck | null;
  draft.value = pendingPolicy.value ? clone(pendingPolicy.value.body.config) : null;
  reason.value = pendingPolicy.value?.body.reason ?? "";
  checkReason.value = pendingCheck.value?.body.reason ?? "";
  confirmationKind.value=pendingPolicy.value?'policy':pendingCheck.value?'check':null;
  if (rights.value.viewPolicy) void readPolicy();
}, { immediate: true });
onUnmounted(() => { live = false; policyGuard.invalidate(); writeGuard.invalidate(); });
</script>

<template>
  <section class="compliance brand-settings-panel" :aria-label="t('admin.copy.777116f9')">
    <div class="columns">
      <section class="card"><h2>{{ t('admin.copy.4eea70b6') }}</h2><p v-if="!rights.viewPolicy" role="status">{{ t('admin.copy.17da948e') }}</p><template v-else>
<p v-global-loading v-if="loadingPolicy" role="status">{{ t('admin.copy.358f517a') }}</p><p v-if="policyError" class="error" role="alert">{{ localizedMessage(policyError) }}</p><button v-if="readFailed" class="button button-secondary" type="button" :disabled="loadingPolicy" @click="withGlobalLoading(readPolicy)">{{t('admin.settings.readAgain')}}</button>
        <form novalidate v-inline-validation v-if="draft" @submit.prevent="withGlobalLoading(preparePolicy)">
          <label class="toggle"><input v-model="draft.age_enabled" type="checkbox" :disabled="!policyEditable"> {{ t('admin.copy.54fdbb89') }}</label>
          <label>{{ t('admin.copy.ed9cfee4') }}<input :aria-label="t('admin.copy.ed9cfee4')" :value="draft.minimum_age ?? ''" type="number" min="18" max="120" step="1" :disabled="!policyEditable" @input="updateMinimumAge"></label><small>{{ t('admin.copy.ba3d45d3') }}</small>
          <label class="toggle"><input v-model="draft.region_enabled" type="checkbox" :disabled="!policyEditable"> {{ t('admin.copy.891fba58') }}</label>
          <label>{{ t('admin.copy.17f1e53b') }}<textarea :value="draft.allowed_countries.join('\n')" rows="5" :disabled="!policyEditable" @input="draft.allowed_countries = ($event.target as HTMLTextAreaElement).value.split(/\r?\n/).filter(Boolean).sort()"></textarea></label>
          <label class="toggle"><input v-model="draft.identity_enabled" type="checkbox" :disabled="!policyEditable"> {{ t('admin.copy.82093bf2') }}</label>
          <label class="toggle"><input v-model="draft.account_risk_enabled" type="checkbox" :disabled="!policyEditable"> {{ t('admin.copy.ce9d84ea') }}</label>
          <label class="toggle"><input v-model="draft.betting_risk_enabled" type="checkbox" :disabled="!policyEditable"> {{ t('admin.copy.8ca638d3') }}</label>
          <label class="toggle"><input v-model="draft.exclusion_enabled" type="checkbox" :disabled="!policyEditable"> {{ t('admin.copy.ba23b299') }}</label>
          <label class="toggle"><input v-model="draft.responsible_gambling_enabled" type="checkbox" :disabled="!policyEditable"> {{ t('admin.copy.da4340e2') }}</label>

          <button class="button button-primary" type="submit" :disabled="!policyEditable">{{ t('admin.copy.a324ef2b') }}</button>
        </form>
        <button v-if="pendingPolicy" class="button button-secondary" type="button" @click="confirmationKind='policy'">{{t('admin.settings.reviewPending')}}</button>
      </template></section>
      <section class="card"><h2>{{ t('admin.settings.checkTitle') }}</h2><p v-if="!rights.viewChecks && !rights.runCheck && !pendingCheck" role="status">{{ t('admin.copy.37fe96cf') }}</p><template v-else>
        <form novalidate v-inline-validation v-if="rights.runCheck" @submit.prevent="withGlobalLoading(prepareCheck)"><label>{{ t('admin.copy.55a14d43') }}<select v-model="operation" :disabled="!checkEditable"><option value="registration">{{ t('admin.copy.f59482ae') }}</option><option value="betting">{{ t('admin.copy.00b6328f') }}</option><option value="withdrawal">{{ t('admin.copy.58c4aae8') }}</option></select></label><button class="button button-primary" type="submit" :disabled="!checkEditable">{{ t('admin.copy.c5dd602b') }}</button></form>
        <button v-if="pendingCheck" class="button button-secondary" type="button" @click="confirmationKind='check'">{{t('admin.settings.reviewPending')}}</button>
        <div v-if="checkResult" class="check-result" role="status">
          <strong>{{operationLabel(checkResult.operation)}} · {{decisionLabel(checkResult.decision)}}</strong>
          <ul><li v-for="check in checkResult.checks" :key="check.check">{{checkLabel(check.check)}}: {{check.enabled?decisionLabel(check.decision):t('admin.copy.1ba29434')}}</li></ul>
        </div>
      </template></section>
    </div>
    <p v-if="actionError&&!confirmationKind" class="error" role="alert">{{ localizedMessage(actionError) }}</p><p v-if="notice" class="notice" role="status">{{ localizedMessage(notice) }}</p>
    <DetailModal v-if="activeIntent&&confirmationKind" class="brand-settings-dialog" :title="t('admin.settings.confirmSave')" :close-label="t('association.close')" @close="closeConfirmation">
      <p>{{activeIntent.kind==='policy'?t('admin.settings.confirmPolicy'):t('admin.settings.confirmCheck',{operation:operationLabel(activeIntent.body.operation)})}}</p>
      <p v-if="activeIntent.body.reason">{{activeIntent.body.reason}}</p>
      <p v-if="activeIntent.phase==='unknown'&&!busy" class="message" role="alert">{{t('admin.settings.unknown')}}</p>
      <p v-if="activeIntent.phase==='conflict'" class="message" role="alert">{{t('admin.settings.conflict')}}</p>
      <p v-if="actionError&&activeIntent.phase==='review'" class="error" role="alert">{{localizedMessage(actionError)}}</p>
      <template #footer>
        <button v-if="activeIntent.phase==='review'" class="button button-secondary" type="button" :disabled="busy" @click="cancel(confirmationKind)">{{t('admin.copy.96f851ea')}}</button>
        <button v-write-guard="busy" v-if="activeIntent.phase==='review'" class="button button-primary" type="button" :disabled="busy" @click="withGlobalLoading(send,confirmationKind)">{{t('admin.copy.a1a8013d')}}</button>
        <button v-write-guard="busy" v-if="activeIntent.phase==='unknown'" class="button button-primary" type="button" :disabled="busy" @click="withGlobalLoading(send,confirmationKind,true)">{{t('admin.settings.retryOperation')}}</button>
        <button v-if="activeIntent.phase==='conflict'&&!conflictPolicyRead" class="button button-secondary" type="button" :disabled="loadingPolicy||busy" @click="withGlobalLoading(async()=>{conflictPolicyRead=await readPolicy()})">{{t('admin.settings.readAgain')}}</button>
        <button v-if="activeIntent.phase==='conflict'&&conflictPolicyRead" class="button button-secondary" type="button" :disabled="busy" @click="cancel(confirmationKind)">{{t('admin.settings.startAgain')}}</button>
      </template>
    </DetailModal>
  </section>
</template>

<style scoped>
.columns { display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px;min-width:0; }
.card { min-width:0; }
h2 { margin:0 0 12px;font-size:16px; }
p,small { color:var(--muted);overflow-wrap:anywhere; }
.card form { display:grid;gap:10px; }
.card label { display:grid;gap:6px;min-width:0; }
.card input:not([type=checkbox]),.card textarea { box-sizing:border-box;width:100%;min-width:0; }
.card form > button { justify-self:start; }
.toggle { display:flex!important;align-items:center;gap:8px;min-height:44px; }
.toggle input { width:18px;height:18px;flex:0 0 18px;accent-color:var(--primary); }
.error { color:var(--danger-text);background:var(--danger-bg);padding:12px;border-radius:var(--radius-sm); }
.notice { color:var(--primary-dark); }
.check-result { margin-top:12px;padding:12px;background:var(--canvas);border-radius:var(--radius-sm); }
.check-result ul { padding-left:20px; }
@media(max-width:760px){.columns{grid-template-columns:minmax(0,1fr)}}
</style>
