<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { compliancePermissions, createCompliancePolicyApi, validComplianceConfig, validComplianceReason, type ComplianceConfig, type ComplianceDecision, type ComplianceGateRecord, type ComplianceOperation, type CompliancePolicy, type CompliancePolicyRevision } from "./compliance-api";
import { classifyComplianceFailure, clearPendingComplianceIntent, complianceSessionGeneration, createComplianceRequestGuard, getPendingComplianceIntent, setPendingComplianceIntent, updatePendingCompliancePhase, type PendingComplianceCheck, type PendingComplianceIntent, type PendingPolicyWrite } from "./compliance-state";
import { useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const { t } = useAdminI18n();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createCompliancePolicyApi();
const policyGuard = createComplianceRequestGuard(), historyGuard = createComplianceRequestGuard(), decisionsGuard = createComplianceRequestGuard(), gatesGuard = createComplianceRequestGuard(), writeGuard = createComplianceRequestGuard();
const rights = computed(() => compliancePermissions(props.account, props.brandId));
const scope = () => ({ accountId: props.account.id, brandId: props.brandId });
const policy = ref<CompliancePolicy | null>(null), draft = ref<ComplianceConfig | null>(null), reason = ref("");
const history = ref<CompliancePolicyRevision[]>([]), historyOffset = ref(0), historyTotal = ref("0");
const decisions = ref<ComplianceDecision[]>([]), decisionOffset = ref(0), decisionTotal = ref("0");
const gates = ref<ComplianceGateRecord[]>([]), gateOffset = ref(0), gateTotal = ref("0");
const operation = ref<ComplianceOperation>("registration"), checkReason = ref("");
const pendingPolicy = ref<PendingPolicyWrite | null>(null), pendingCheck = ref<PendingComplianceCheck | null>(null);
const loadingPolicy = ref(false), loadingHistory = ref(false), loadingDecisions = ref(false), loadingGates = ref(false), busy = ref(false);
const conflictPolicyRead = ref(false);
const policyError = ref(""), historyError = ref(""), decisionsError = ref(""), gatesError = ref(""), actionError = ref(""), notice = ref("");
const reasonBytes = computed(() => new TextEncoder().encode(reason.value).length), checkReasonBytes = computed(() => new TextEncoder().encode(checkReason.value).length);
const policyEditable = computed(() => rights.value.writePolicy && Boolean(policy.value) && !pendingPolicy.value && !busy.value);
const checkEditable = computed(() => rights.value.runCheck && Boolean(policy.value) && !pendingCheck.value && !busy.value);
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
const operationLabel = (value: string) => value === "registration" ? t("注册", "Registration") : value === "betting" ? t("投注", "Betting") : value === "withdrawal" ? t("提现", "Withdrawal") : value;
const actionLabel = (value: string) => value === "register" ? t("注册", "Register") : value === "join" ? t("加入品牌", "Join brand") : value === "operator_join" ? t("管理员加入品牌", "Operator brand join") : value === "bet_preview" ? t("投注预览", "Bet preview") : value === "bet_place" ? t("提交投注", "Place bet") : value;
const decisionLabel = (value: string) => value === "allow" ? t("允许", "Allow") : value === "deny" ? t("拒绝", "Deny") : value === "review" ? t("复核", "Review") : value;
const checkLabel = (value: string) => value === "age" ? t("年龄", "Age") : value === "region" ? t("地区", "Region") : value === "identity" ? t("身份", "Identity") : value === "account_risk" ? t("账号关联风险", "Account linkage risk") : value === "betting_risk" ? t("投注风险", "Betting risk") : value === "exclusion" ? t("排除名单", "Exclusion") : value === "responsible_gambling" ? t("负责任博彩", "Responsible gambling") : value;
function localizedMessage(value: string): string {
  const translations: Record<string, [string, string]> = {
    "读取政策失败": ["读取政策失败", "Failed to load policy"], "读取政策历史失败": ["读取政策历史失败", "Failed to load policy history"],
    "读取检查记录失败": ["读取检查记录失败", "Failed to load check records"], "读取业务闸门记录失败": ["读取业务闸门记录失败", "Failed to load business gate records"],
    "请核对配置，并填写无首尾空白、不含控制字符且最多500 UTF-8字节的原因。": ["请核对配置，并填写无首尾空白、不含控制字符且最多500 UTF-8字节的原因。", "Review the configuration and enter a reason with no surrounding whitespace or control characters, up to 500 UTF-8 bytes."],
    "已有未解决的政策意图；请先使用原键处理，不能替换其正文或键。": ["已有未解决的政策意图；请先使用原键处理，不能替换其正文或键。", "An unresolved policy intent exists. Resolve it with its original key; its body and key cannot be replaced."],
    "请填写无首尾空白、不含控制字符且最多500 UTF-8字节的检查原因。": ["请填写无首尾空白、不含控制字符且最多500 UTF-8字节的检查原因。", "Enter a check reason with no surrounding whitespace or control characters, up to 500 UTF-8 bytes."],
    "已有未解决的检查意图；请先使用原键处理，不能替换其正文或键。": ["已有未解决的检查意图；请先使用原键处理，不能替换其正文或键。", "An unresolved check intent exists. Resolve it with its original key; its body and key cannot be replaced."],
    "政策写入回执已核验；正在独立刷新政策与历史。": ["政策写入回执已核验；正在独立刷新政策与历史。", "The policy write receipt was verified. Refreshing policy and history independently."],
    "伪适配器决定已记录；不代表真实注册、投注或提现拦截。正在独立刷新检查记录。": ["伪适配器决定已记录；不代表真实注册、投注或提现拦截。正在独立刷新检查记录。", "The stub adapter decision was recorded; it does not represent real registration, betting, or withdrawal enforcement. Refreshing check records independently."],
    "提交结果未知。正文和原键已冻结于当前内存会话；请用原键重试，切换页面不会解除。": ["提交结果未知。正文和原键已冻结于当前内存会话；请用原键重试，切换页面不会解除。", "The outcome is unknown. The body and original key are frozen in this session; retry with the original key. Navigating away does not resolve it."],
    "品牌状态冲突（例如品牌已停用）。意图仍被冻结；不会修改政策或品牌状态。请核实品牌状态并读取最新政策后，再明确放弃旧意图。": ["品牌状态冲突（例如品牌已停用）。意图仍被冻结；不会修改政策或品牌状态。请核实品牌状态并读取最新政策后，再明确放弃旧意图。", "Brand state conflict (for example, the brand is disabled). The intent remains frozen; policy and brand state were not changed. Verify brand state and load the latest policy before explicitly discarding the old intent."],
    "政策版本冲突。意图仍被冻结；不会自动套用草稿或换键重试。读取最新政策后可明确放弃旧意图。": ["政策版本冲突。意图仍被冻结；不会自动套用草稿或换键重试。读取最新政策后可明确放弃旧意图。", "Policy version conflict. The intent remains frozen; the draft will not be applied or retried with a new key automatically. Load the latest policy before explicitly discarding the old intent."],
    "请求被拒绝。": ["请求被拒绝。", "The request was rejected."],
  };
  const pair = translations[value];
  return pair ? t(pair[0], pair[1]) : value;
}
let live = true;
function current(guard: ReturnType<typeof createComplianceRequestGuard>, token: number, accountId: string, brandId: string, epoch: number) { return live && guard.isCurrent(token) && epoch === complianceSessionGeneration() && props.account.id === accountId && props.brandId === brandId; }
function sessionInvalid(cause: unknown, guard: ReturnType<typeof createComplianceRequestGuard>, token: number, accountId: string, brandId: string, epoch: number) { if (cause instanceof AdminApiError && cause.status === 401 && current(guard, token, accountId, brandId, epoch)) emit("session-invalid"); }
async function readPolicy() {
  const { accountId, brandId } = scope(), epoch = complianceSessionGeneration(), token = policyGuard.begin(); loadingPolicy.value = true; policyError.value = "";
  try { const value = await api.get(brandId); if (!current(policyGuard, token, accountId, brandId, epoch)) return false; policy.value = value; if (!pendingPolicy.value) draft.value = clone(value.config); return true; }
  catch (cause) { if (current(policyGuard, token, accountId, brandId, epoch)) { policyError.value = cause instanceof Error ? cause.message : "读取政策失败"; sessionInvalid(cause, policyGuard, token, accountId, brandId, epoch); } return false; }
  finally { if (current(policyGuard, token, accountId, brandId, epoch)) loadingPolicy.value = false; }
}
async function readHistory(offset = historyOffset.value) {
  const { accountId, brandId } = scope(), epoch = complianceSessionGeneration(), token = historyGuard.begin(); loadingHistory.value = true; historyError.value = "";
  try { const value = await api.history(brandId, 20, offset); if (!current(historyGuard, token, accountId, brandId, epoch)) return; history.value = value.items; historyOffset.value = value.offset; historyTotal.value = value.total_count; }
  catch (cause) { if (current(historyGuard, token, accountId, brandId, epoch)) { historyError.value = cause instanceof Error ? cause.message : "读取政策历史失败"; sessionInvalid(cause, historyGuard, token, accountId, brandId, epoch); } }
  finally { if (current(historyGuard, token, accountId, brandId, epoch)) loadingHistory.value = false; }
}
async function readDecisions(offset = decisionOffset.value) {
  const { accountId, brandId } = scope(), epoch = complianceSessionGeneration(), token = decisionsGuard.begin(); loadingDecisions.value = true; decisionsError.value = "";
  try { const value = await api.decisions(brandId, 20, offset); if (!current(decisionsGuard, token, accountId, brandId, epoch)) return; decisions.value = value.items; decisionOffset.value = value.offset; decisionTotal.value = value.total_count; }
  catch (cause) { if (current(decisionsGuard, token, accountId, brandId, epoch)) { decisionsError.value = cause instanceof Error ? cause.message : "读取检查记录失败"; sessionInvalid(cause, decisionsGuard, token, accountId, brandId, epoch); } }
  finally { if (current(decisionsGuard, token, accountId, brandId, epoch)) loadingDecisions.value = false; }
}
async function readGates(offset = gateOffset.value) {
  const { accountId, brandId } = scope(), epoch = complianceSessionGeneration(), token = gatesGuard.begin(); loadingGates.value = true; gatesError.value = "";
  try { const value = await api.gates(brandId, 20, offset); if (!current(gatesGuard, token, accountId, brandId, epoch)) return; gates.value = value.items; gateOffset.value = value.offset; gateTotal.value = value.total_count; }
  catch (cause) { if (current(gatesGuard, token, accountId, brandId, epoch)) { gatesError.value = cause instanceof Error ? cause.message : "读取业务闸门记录失败"; sessionInvalid(cause, gatesGuard, token, accountId, brandId, epoch); } }
  finally { if (current(gatesGuard, token, accountId, brandId, epoch)) loadingGates.value = false; }
}
function preparePolicy() {
  actionError.value = ""; notice.value = "";
  if (!policyEditable.value || !policy.value || !draft.value || !validComplianceConfig(draft.value) || !validComplianceReason(reason.value)) { actionError.value = "请核对配置，并填写无首尾空白、不含控制字符且最多500 UTF-8字节的原因。"; return; }
  const accepted = setPendingComplianceIntent({ ...scope(), kind: "policy", body: { version: policy.value.version, config: clone(draft.value), reason: reason.value }, key: createIdempotencyKey(), phase: "review" });
  if (!accepted) { actionError.value = "已有未解决的政策意图；请先使用原键处理，不能替换其正文或键。"; return; }
  pendingPolicy.value = getPendingComplianceIntent(scope(), "policy");
}
function prepareCheck() {
  actionError.value = ""; notice.value = "";
  if (!checkEditable.value || !policy.value || !validComplianceReason(checkReason.value)) { actionError.value = "请填写无首尾空白、不含控制字符且最多500 UTF-8字节的检查原因。"; return; }
  const accepted = setPendingComplianceIntent({ ...scope(), kind: "check", body: { version: policy.value.version, operation: operation.value, reason: checkReason.value }, key: createIdempotencyKey(), phase: "review" });
  if (!accepted) { actionError.value = "已有未解决的检查意图；请先使用原键处理，不能替换其正文或键。"; return; }
  pendingCheck.value = getPendingComplianceIntent(scope(), "check");
}
function updateMinimumAge(event: Event) { if (!draft.value) return; const value = (event.target as HTMLInputElement).value; draft.value.minimum_age = value === "" ? null : Number(value); }
function cancel(kind: "policy" | "check") { const intent = kind === "policy" ? pendingPolicy.value : pendingCheck.value; if (busy.value || !intent || intent.phase === "unknown" || intent.phase === "conflict" && !conflictPolicyRead.value) return; clearPendingComplianceIntent(scope(), kind, intent.key); if (kind === "policy") { pendingPolicy.value = null; if (policy.value) draft.value = clone(policy.value.config); } else pendingCheck.value = null; conflictPolicyRead.value = false; }
async function send(kind: "policy" | "check", retry = false) {
  const intent = kind === "policy" ? pendingPolicy.value : pendingCheck.value;
  if (!intent || busy.value || retry && intent.phase !== "unknown" || !retry && intent.phase !== "review") return;
  if (kind === "policy" ? !rights.value.writePolicy : !rights.value.runCheck) return;
  const origin = { accountId: intent.accountId, brandId: intent.brandId };
  const epoch = complianceSessionGeneration(), token = writeGuard.begin(); busy.value = true; actionError.value = ""; notice.value = "";
  updatePendingCompliancePhase(origin, kind, intent.key, "unknown");
  if (kind === "policy") pendingPolicy.value = getPendingComplianceIntent(origin, "policy"); else pendingCheck.value = getPendingComplianceIntent(origin, "check");
  try {
    if (intent.kind === "policy") await api.put(intent.brandId, intent.body, intent.key);
    else await api.run(intent.brandId, intent.body, intent.key);
    if (epoch !== complianceSessionGeneration()) return;
    clearPendingComplianceIntent({ accountId: intent.accountId, brandId: intent.brandId }, kind, intent.key);
    if (!current(writeGuard, token, intent.accountId, intent.brandId, epoch)) return;
    pendingPolicy.value = getPendingComplianceIntent(scope(), "policy"); pendingCheck.value = getPendingComplianceIntent(scope(), "check");
    notice.value = kind === "policy" ? "政策写入回执已核验；正在独立刷新政策与历史。" : "伪适配器决定已记录；不代表真实注册、投注或提现拦截。正在独立刷新检查记录。";
    if (kind === "policy") { await readPolicy(); await readHistory(0); }
    else await readDecisions(0);
  } catch (cause) {
    if (epoch !== complianceSessionGeneration()) return;
    const result = classifyComplianceFailure(cause instanceof AdminApiError ? cause.status : undefined, cause instanceof AdminApiError ? cause.code : undefined);
    if (result !== "definitive") updatePendingCompliancePhase(origin, kind, intent.key, result);
    else clearPendingComplianceIntent(origin, kind, intent.key);
    if (!current(writeGuard, token, intent.accountId, intent.brandId, epoch)) return;
    pendingPolicy.value = getPendingComplianceIntent(scope(), "policy"); pendingCheck.value = getPendingComplianceIntent(scope(), "check");
    actionError.value = result === "unknown" ? "提交结果未知。正文和原键已冻结于当前内存会话；请用原键重试，切换页面不会解除。" : result === "conflict" ? cause instanceof AdminApiError && cause.code === "COMPLIANCE_STATE_CONFLICT" ? "品牌状态冲突（例如品牌已停用）。意图仍被冻结；不会修改政策或品牌状态。请核实品牌状态并读取最新政策后，再明确放弃旧意图。" : "政策版本冲突。意图仍被冻结；不会自动套用草稿或换键重试。读取最新政策后可明确放弃旧意图。" : cause instanceof Error ? cause.message : "请求被拒绝。";
    if (result === "conflict") { conflictPolicyRead.value = false; conflictPolicyRead.value = await readPolicy(); }
    sessionInvalid(cause, writeGuard, token, intent.accountId, intent.brandId, epoch);
  } finally { if (current(writeGuard, token, intent.accountId, intent.brandId, epoch)) busy.value = false; }
}
watch(() => [props.account.id, props.brandId], () => {
  policyGuard.invalidate(); historyGuard.invalidate(); decisionsGuard.invalidate(); gatesGuard.invalidate(); writeGuard.invalidate();
  policy.value = null; history.value = []; decisions.value = []; gates.value = []; historyOffset.value = 0; decisionOffset.value = 0; gateOffset.value = 0; conflictPolicyRead.value = false;
  policyError.value = ""; historyError.value = ""; decisionsError.value = ""; gatesError.value = ""; actionError.value = ""; notice.value = ""; loadingPolicy.value = false; loadingHistory.value = false; loadingDecisions.value = false; loadingGates.value = false; busy.value = false;
  pendingPolicy.value = getPendingComplianceIntent(scope(), "policy") as PendingPolicyWrite | null; pendingCheck.value = getPendingComplianceIntent(scope(), "check") as PendingComplianceCheck | null;
  draft.value = pendingPolicy.value ? clone(pendingPolicy.value.body.config) : null;
  reason.value = pendingPolicy.value?.body.reason ?? "";
  checkReason.value = pendingCheck.value?.body.reason ?? "";
  if (rights.value.viewPolicy) { void readPolicy(); void readHistory(0); }
  if (rights.value.viewChecks) void readDecisions(0);
  if (rights.value.viewChecks) void readGates(0);
}, { immediate: true });
onUnmounted(() => { live = false; policyGuard.invalidate(); historyGuard.invalidate(); decisionsGuard.invalidate(); gatesGuard.invalidate(); writeGuard.invalidate(); });
</script>

<template>
  <section class="compliance" aria-labelledby="compliance-title">
    <header><div><small>RISK / COMPLIANCE</small><h1 id="compliance-title">{{ t("风控与合规", "Risk and compliance") }}</h1><p>{{ t("包含品牌政策、管理员显式运行的 stub 检查，以及真实新业务请求的服务端闸门拒绝记录。", "Includes brand policy, administrator-run stub checks, and server-side gate rejection records for real new business requests.") }}</p></div><button type="button" :disabled="loadingPolicy || !rights.viewPolicy" @click="readPolicy">{{ t("重新读取政策", "Reload policy") }}</button></header>
<aside class="warning" role="alert"><b>{{ t("真实注册/首次加入品牌及投注预览/提交已执行服务端检查；启用但适配器未配置时会拒绝新业务。", "Real registration/first-brand-join and bet preview/submission requests run server-side checks; enabled checks reject new business when their adapter is unconfigured.") }}</b><p>{{ t("七项管理员显式运行的伪检查仅记录配置结果：关闭返回 allow/CHECK_DISABLED，开启返回 review/ADAPTER_NOT_CONFIGURED；不进行真实验证，不收集个人数据，不会自动冻结。若七项配置全关闭，跳过检查不代表用户已通过验证。没有人工复核队列。闸门拒绝历史与管理员显式运行的伪检查决定是两类记录。请勿输入用户证件或其他个人信息。国家代码语法不证明该地区合法。", "The seven administrator-run pseudo-checks only report the configured result: disabled returns allow/CHECK_DISABLED, enabled returns review/ADAPTER_NOT_CONFIGURED. They do not perform real verification, collect personal data, or automatically freeze accounts. If all seven settings are off, skipping checks does not mean a user passed verification. There is no manual review queue. Gate rejection history and administrator-run pseudo-check decisions are separate records. Do not enter identity documents or other personal information. A syntactically valid country code does not establish local legality.") }}</p><p>{{ t("提现积分流程已接入，但真实支付及身份验证未接入。", "Withdrawal point workflows are connected, but real payments and identity verification are not.") }}</p></aside>
    <div class="columns">
      <section class="card"><h2>{{ t("品牌政策", "Brand policy") }}</h2><p v-if="!rights.viewPolicy" role="status">{{ t("当前账号没有此品牌政策查看权限。", "This account cannot view policy for this brand.") }}</p><template v-else>
        <p v-if="policy">{{ t("品牌", "Brand") }} {{ policy.brand_id }} · v{{ policy.version }} · {{ t("更新于", "Updated") }} {{ policy.updated_at }}</p><p v-if="loadingPolicy" role="status">{{ t("读取政策中…", "Loading policy…") }}</p><p v-if="policyError" class="error" role="alert">{{ localizedMessage(policyError) }}</p>
        <form v-if="draft" @submit.prevent="preparePolicy">
          <label class="toggle"><input v-model="draft.age_enabled" type="checkbox" :disabled="!policyEditable"> {{ t("启用最低年龄检查", "Enable minimum age check") }}</label>
          <label>{{ t("最低年龄（18–120）", "Minimum age (18–120)") }}<input :aria-label="t('最低年龄（18–120）', 'Minimum age (18–120)')" :value="draft.minimum_age ?? ''" type="number" min="18" max="120" step="1" :disabled="!policyEditable" @input="updateMinimumAge"></label><small>{{ t("关闭年龄检查时可留空；开启时必须为18–120的整数。", "Leave blank when age checks are off; when enabled, enter an integer from 18 to 120.") }}</small>
          <label class="toggle"><input v-model="draft.region_enabled" type="checkbox" :disabled="!policyEditable"> {{ t("启用国家/地区允许名单", "Enable country/region allowlist") }}</label>
          <label>{{ t("允许国家代码（每行一个，两位大写字母，最多250个）", "Allowed country codes (one per line, two uppercase letters, up to 250)") }}<textarea :value="draft.allowed_countries.join('\n')" rows="5" :disabled="!policyEditable" @input="draft.allowed_countries = ($event.target as HTMLTextAreaElement).value.split(/\r?\n/).filter(Boolean).sort()"></textarea></label>
          <label class="toggle"><input v-model="draft.identity_enabled" type="checkbox" :disabled="!policyEditable"> {{ t("启用身份检查", "Enable identity check") }}</label>
          <label class="toggle"><input v-model="draft.account_risk_enabled" type="checkbox" :disabled="!policyEditable"> {{ t("账号关联风险：设备/IP/账号关联（伪检查）", "Account linkage risk: device/IP/account links (pseudo-check)") }}</label>
          <label class="toggle"><input v-model="draft.betting_risk_enabled" type="checkbox" :disabled="!policyEditable"> {{ t("投注风险：异常投注/奖金滥用/套利/多账号（伪检查）", "Betting risk: abnormal bets/bonus abuse/arbitrage/multiple accounts (pseudo-check)") }}</label>
          <label class="toggle"><input v-model="draft.exclusion_enabled" type="checkbox" :disabled="!policyEditable"> {{ t("排除名单：黑名单/自我排除（伪检查）", "Exclusion: blacklist/self-exclusion (pseudo-check)") }}</label>
          <label class="toggle"><input v-model="draft.responsible_gambling_enabled" type="checkbox" :disabled="!policyEditable"> {{ t("负责任博彩：责任提示/冷静期（伪检查）", "Responsible gambling: responsibility/cooling-off (pseudo-check)") }}</label>
          <label>{{ t("政策变更原因", "Policy change reason") }}<textarea v-model="reason" rows="3" maxlength="500" :disabled="!policyEditable" :placeholder="t('说明政策变更原因', 'Explain the policy change')"></textarea><small>{{ reasonBytes }} {{ t("/ 500 UTF-8字节", "/ 500 UTF-8 bytes") }}</small></label>
          <button type="submit" :disabled="!policyEditable">{{ t("核对政策变更", "Review policy change") }}</button>
        </form>
        <div v-if="pendingPolicy" class="intent"><b>{{ t("政策意图：", "Policy intent: ") }}{{ pendingPolicy.phase === 'unknown' ? t("结果未知，已冻结", "Outcome unknown and frozen") : pendingPolicy.phase === 'conflict' ? t("版本或品牌状态冲突；当前页面只读", "Version or brand state conflict; this page is read-only") : t("等待确认", "Awaiting confirmation") }}</b><p>{{ t("品牌", "Brand") }} {{ pendingPolicy.brandId }} · {{ t("意图版本", "Intent version") }} {{ pendingPolicy.body.version }} · {{ t("当前版本", "Current version") }} {{ policy?.version ?? t('未读取', 'Not loaded') }} · {{ pendingPolicy.body.reason }}</p><pre>{{ JSON.stringify(pendingPolicy.body.config, null, 2) }}</pre><div class="actions"><button v-if="pendingPolicy.phase === 'review'" :disabled="busy" @click="send('policy')">{{ t("确认并提交政策", "Confirm and submit policy") }}</button><button v-if="pendingPolicy.phase === 'unknown'" :disabled="busy" @click="send('policy', true)">{{ t("用原键重试", "Retry with original key") }}</button><button v-if="pendingPolicy.phase === 'review'" :disabled="busy" @click="cancel('policy')">{{ t("取消确认", "Cancel review") }}</button><button v-if="pendingPolicy.phase === 'conflict'" :disabled="busy || !conflictPolicyRead" @click="cancel('policy')">{{ t("放弃旧政策意图并使用已读取版本", "Discard old policy intent and use the loaded version") }}</button></div></div>
      </template></section>
      <section class="card"><h2>{{ t("管理员显式运行的 stub 检查", "Administrator-run stub checks") }}</h2><p v-if="!rights.viewChecks && !rights.runCheck && !pendingCheck" role="status">{{ t("当前账号没有检查权限。", "This account does not have check permissions.") }}</p><template v-else>
        <p v-if="!rights.viewChecks && rights.runCheck" role="status">{{ t("可运行经确认的 stub 检查，但没有读取历史记录的权限。", "Confirmed stub checks can be run, but history cannot be viewed.") }}</p><p v-else>{{ t("只有点击确认后才创建检查记录。决策仅反映当前 stub 规则。", "A check record is created only after confirmation. Decisions reflect the current stub rules only.") }}</p>
        <form v-if="rights.runCheck" @submit.prevent="prepareCheck"><label>{{ t("操作类型", "Operation") }}<select v-model="operation" :disabled="!checkEditable"><option value="registration">{{ t("注册", "Registration") }}</option><option value="betting">{{ t("投注", "Betting") }}</option><option value="withdrawal">{{ t("提现", "Withdrawal") }}</option></select></label><label>{{ t("检查原因", "Check reason") }}<textarea :aria-label="t('检查原因', 'Check reason')" v-model="checkReason" rows="3" maxlength="500" :disabled="!checkEditable" :placeholder="t('说明检查原因', 'Explain the reason for this check')"></textarea><small>{{ checkReasonBytes }} {{ t("/ 500 UTF-8字节", "/ 500 UTF-8 bytes") }}</small></label><button type="submit" :disabled="!checkEditable">{{ t("核对伪检查", "Review stub check") }}</button></form>
        <div v-if="pendingCheck" class="intent"><b>{{ t("检查意图：", "Check intent: ") }}{{ pendingCheck.phase === 'unknown' ? t("结果未知，已冻结", "Outcome unknown and frozen") : pendingCheck.phase === 'conflict' ? t("版本或品牌状态冲突", "Version or brand state conflict") : t("等待确认", "Awaiting confirmation") }}</b><p>{{ pendingCheck.brandId }} · {{ t("意图版本", "Intent version") }} {{ pendingCheck.body.version }} · {{ t("当前版本", "Current version") }} {{ policy?.version ?? t('未读取', 'Not loaded') }} · {{ operationLabel(pendingCheck.body.operation) }} · {{ pendingCheck.body.reason }}</p><div class="actions"><button v-if="pendingCheck.phase === 'review'" :disabled="busy" @click="send('check')">{{ t("确认并运行 stub 检查", "Confirm and run stub check") }}</button><button v-if="pendingCheck.phase === 'unknown'" :disabled="busy" @click="send('check', true)">{{ t("用原键重试", "Retry with original key") }}</button><button v-if="pendingCheck.phase === 'review'" :disabled="busy" @click="cancel('check')">{{ t("取消确认", "Cancel review") }}</button><button v-if="pendingCheck.phase === 'conflict'" :disabled="busy || !conflictPolicyRead" @click="cancel('check')">{{ t("放弃旧检查意图", "Discard old check intent") }}</button></div></div>
      </template></section>
    </div>
    <p v-if="actionError" class="error" role="alert">{{ localizedMessage(actionError) }}</p><p v-if="notice" class="notice" role="status">{{ localizedMessage(notice) }}</p>
    <div class="columns records">
      <section class="card"><header><h2>{{ t("政策历史", "Policy history") }}</h2><button :disabled="loadingHistory || !rights.viewPolicy" @click="readHistory(0)">{{ t("刷新", "Refresh") }}</button></header><p v-if="historyError" class="error" role="alert">{{ historyError }}</p><p v-if="loadingHistory" role="status">{{ t("读取政策历史中…", "Loading policy history…") }}</p><p v-else-if="rights.viewPolicy && !history.length">{{ t("暂无记录。", "No records.") }}</p><article v-for="row in history" :key="row.id"><b>v{{ row.version }}</b><time>{{ row.created_at }}</time><p>{{ row.reason }}</p><small>{{ t("操作人", "Actor") }} {{ row.changed_by ?? t('系统', 'System') }} · {{ t("审计", "Audit") }} {{ row.audit_log_id ?? t('无', 'None') }}</small><details><summary>{{ t("政策快照", "Policy snapshot") }}</summary><ul><li>{{ t("账号关联风险", "Account linkage risk") }}: {{ row.config.account_risk_enabled ? t("开启", "On") : t("关闭", "Off") }}</li><li>{{ t("投注风险", "Betting risk") }}: {{ row.config.betting_risk_enabled ? t("开启", "On") : t("关闭", "Off") }}</li><li>{{ t("排除名单", "Exclusion") }}: {{ row.config.exclusion_enabled ? t("开启", "On") : t("关闭", "Off") }}</li><li>{{ t("负责任博彩", "Responsible gambling") }}: {{ row.config.responsible_gambling_enabled ? t("开启", "On") : t("关闭", "Off") }}</li></ul><pre>{{ JSON.stringify(row.config, null, 2) }}</pre></details></article><div class="actions"><button :disabled="historyOffset < 20 || loadingHistory" @click="readHistory(Math.max(0, historyOffset - 20))">{{ t("较新记录", "Newer records") }}</button><span>{{ historyOffset + 1 }}–{{ historyOffset + history.length }} / {{ historyTotal }}</span><button :disabled="historyOffset + history.length >= Number(historyTotal) || loadingHistory" @click="readHistory(historyOffset + 20)">{{ t("较旧记录", "Older records") }}</button></div></section>
      <section class="card"><header><h2>{{ t("检查记录", "Check records") }}</h2><button :disabled="loadingDecisions || !rights.viewChecks" @click="readDecisions(0)">{{ t("刷新", "Refresh") }}</button></header><p v-if="!rights.viewChecks" role="status">{{ t("当前账号没有检查记录查看权限。", "This account cannot view check records.") }}</p><template v-else><p v-if="decisionsError" class="error" role="alert">{{ decisionsError }}</p><p v-if="loadingDecisions" role="status">{{ t("读取检查记录中…", "Loading check records…") }}</p><p v-else-if="!decisions.length">{{ t("暂无记录。", "No records.") }}</p><article v-for="item in decisions" :key="item.id"><b>{{ operationLabel(item.operation) }} · {{ decisionLabel(item.decision) }} · v{{ item.policy_version }}</b><time>{{ item.created_at }}</time><p>{{ item.reason }} · adapter={{ item.adapter_mode }}</p><ul><li v-for="check in item.checks" :key="check.check">{{ checkLabel(check.check) }}: {{ check.enabled ? decisionLabel(check.decision) : t('关闭', 'Off') }} ({{ check.reason_code }})</li></ul><small>{{ t("创建人", "Created by") }} {{ item.created_by }} · {{ t("审计", "Audit") }} {{ item.audit_log_id }}</small></article><div class="actions"><button :disabled="decisionOffset < 20 || loadingDecisions" @click="readDecisions(Math.max(0, decisionOffset - 20))">{{ t("较新记录", "Newer records") }}</button><span>{{ decisionOffset + 1 }}–{{ decisionOffset + decisions.length }} / {{ decisionTotal }}</span><button :disabled="decisionOffset + decisions.length >= Number(decisionTotal) || loadingDecisions" @click="readDecisions(decisionOffset + 20)">{{ t("较旧记录", "Older records") }}</button></div></template></section>
      <section class="card"><header><h2>{{ t("真实业务闸门拒绝记录", "Real business gate rejections") }}</h2><button :disabled="loadingGates || !rights.viewChecks" @click="readGates(0)">{{ t("刷新", "Refresh") }}</button></header><p v-if="!rights.viewChecks" role="status">{{ t("当前账号没有检查记录查看权限。", "This account cannot view check records.") }}</p><template v-else><p>{{ t("仅显示当前品牌的真实注册/首次入品牌与投注业务请求拒绝记录；不包含个人资料。", "Shows only real registration/first-brand-join and bet request rejections for this brand; no personal information is included.") }}</p><p v-if="gatesError" class="error" role="alert">{{ gatesError }}</p><p v-if="loadingGates" role="status">{{ t("读取闸门记录中…", "Loading gate records…") }}</p><p v-else-if="!gates.length">{{ t("暂无记录。", "No records.") }}</p><article v-for="item in gates" :key="item.id"><b>{{ operationLabel(item.operation) }} · {{ actionLabel(item.action) }} · {{ decisionLabel(item.decision) }} · v{{ item.policy_version }}</b><time>{{ item.created_at }}</time><ul><li v-for="check in item.checks" :key="check.check">{{ checkLabel(check.check) }}: {{ check.enabled ? decisionLabel(check.decision) : t('关闭', 'Off') }} ({{ check.reason_code }})</li></ul></article><div class="actions"><button :disabled="gateOffset < 20 || loadingGates" @click="readGates(Math.max(0, gateOffset - 20))">{{ t("较新记录", "Newer records") }}</button><span>{{ gateOffset + 1 }}–{{ gateOffset + gates.length }} / {{ gateTotal }}</span><button :disabled="gateOffset + gates.length >= Number(gateTotal) || loadingGates" @click="readGates(gateOffset + 20)">{{ t("较旧记录", "Older records") }}</button></div></template></section>
    </div>
  </section>
</template>

<style scoped>
.compliance{padding:24px;max-width:1440px;margin:auto;color:var(--ink,#233044)}header{display:flex;align-items:flex-start;justify-content:space-between;gap:14px}h1{margin:5px 0}h2{margin:0 0 12px}p{line-height:1.5;color:#536174}small{color:#64748b;overflow-wrap:anywhere}button{min-height:42px;padding:8px 13px;background:#fff;border:1px solid #cbd5e1;border-radius:8px;color:inherit;font:inherit;cursor:pointer}button:disabled{opacity:.5;cursor:not-allowed}.warning{margin:18px 0;padding:16px;border:2px solid #b42318;border-radius:10px;background:#fff1f0;color:#7a271a}.warning p{margin-bottom:0;color:#7a271a}.columns{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.card{min-width:0;padding:18px;border:1px solid #e0e6ee;border-radius:12px;background:#fff}.card label{display:grid;gap:7px;margin:14px 0}.card input:not([type=checkbox]),select,textarea{box-sizing:border-box;width:100%;min-width:0;padding:10px;border:1px solid #cbd5e1;border-radius:7px;font:inherit}.toggle{display:flex!important;align-items:center}.intent{margin-top:16px;padding:14px;border:1px solid #d0d8e3;border-radius:9px;background:#f8fafc;overflow-wrap:anywhere}pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px}.actions{display:flex;align-items:center;flex-wrap:wrap;gap:10px;margin:12px 0}.error{padding:11px;background:#fff0f0;color:#982c2c}.notice{padding:11px;background:#eef5ff;color:#315780}.records{margin-top:18px}.records .card>header{align-items:center}.records article{padding:13px 0;border-bottom:1px solid #edf0f4;overflow-wrap:anywhere}.records time{display:block;margin-top:6px;font-size:12px;color:#64748b}.records details{margin:10px 0}
@media(max-width:760px){.compliance{padding:14px}.columns{grid-template-columns:1fr}header{flex-direction:column}header>button{width:100%}}
</style>
