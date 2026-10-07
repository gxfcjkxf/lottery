<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import {
  commissionPolicyPermissions,
  createCommissionPolicyApi,
  type CommissionCalendar,
  type CommissionPolicy,
  type CommissionPolicyConfig,
  type CommissionPolicyHistory,
  type CommissionPolicyRevision,
  type CommissionCycle,
  type CommissionPayoutMode,
  type UpdateCommissionPolicyBody,
} from "./commission-policy-api";
import {
  classifyCommissionWriteFailure,
  clearPendingCommissionWrite,
  commissionPolicyBodyFingerprint,
  commissionPolicyContextKey,
  commissionPolicyScopeMatches,
  findPendingCommissionWrite,
  freezeCommissionPolicyBody,
  clearAllPendingCommissionWrites,
  getPendingCommissionWrite,
  setPendingCommissionWrite,
} from "./commission-policy-state";

type Draft = {
  enabled: boolean;
  calendarSelected: boolean;
  timezone: string;
  cycle: CommissionCycle | "";
  boundaryTime: string;
  weekday: string;
  monthDay: string;
  shortMonth: "" | "last_day" | "skip";
  payoutMode: CommissionPayoutMode;
  reason: string;
};
type FrozenMutation = {
  actorId: string;
  brandId: string;
  body: Readonly<UpdateCommissionPolicyBody>;
  key: string;
  fingerprint: string;
};
type LocalHistory = CommissionPolicyHistory;

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createCommissionPolicyApi();
const { t } = useAdminI18n();
const rights = computed(() => commissionPolicyPermissions(props.account, props.brandId));
const policy = ref<CommissionPolicy | null>(null);
const history = ref<LocalHistory | null>(null);
const draft = ref<Draft>(emptyDraft());
const loading = ref(false);
const loadingHistory = ref(false);
const saving = ref(false);
const reconciling = ref(false);
const error = ref("");
const notice = ref("");
const confirmation = ref<FrozenMutation | null>(null);
const uncertain = ref<FrozenMutation | null>(null);
const conflict = ref(false);
let generation = 0;
let policyRead = 0;
let historyRead = 0;
let live = true;

function emptyDraft(): Draft {
  return { enabled: false, calendarSelected: false, timezone: "", cycle: "", boundaryTime: "", weekday: "", monthDay: "", shortMonth: "", payoutMode: "manual", reason: "" };
}

function contextKey(actorId = props.account.id, brandId = props.brandId) {
  return commissionPolicyContextKey(actorId, brandId);
}

function current(ticket: number, brandId: string, actorId: string, requireView = true) {
  return commissionPolicyScopeMatches({ ticket, currentTicket: generation, actorId, currentActorId: props.account.id, brandId, currentBrandId: props.brandId, canView: !requireView || rights.value.view, live });
}

function explain(cause: unknown, fallback: string) {
  return cause instanceof Error && cause.message ? cause.message : fallback;
}

function sessionExpired(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401) {
    generation++;
    clearAllPendingCommissionWrites();
    clearVisibleData();
    emit("session-invalid");
    return true;
  }
  return false;
}

function clearVisibleData() {
  policy.value = null;
  history.value = null;
  draft.value = emptyDraft();
  confirmation.value = null;
  uncertain.value = null;
  conflict.value = false;
  loading.value = false;
  loadingHistory.value = false;
  saving.value = false;
  reconciling.value = false;
  notice.value = "";
}

function applyDraft(config: CommissionPolicyConfig) {
  const calendar = config.calendar;
  draft.value = {
    enabled: config.enabled,
    calendarSelected: calendar !== null,
    timezone: calendar?.timezone ?? "",
    cycle: calendar?.cycle ?? "",
    boundaryTime: calendar?.boundary_time ?? "",
    weekday: calendar?.weekday === null || calendar?.weekday === undefined ? "" : String(calendar.weekday),
    monthDay: calendar?.month_day === null || calendar?.month_day === undefined ? "" : String(calendar.month_day),
    shortMonth: calendar?.short_month ?? "",
    payoutMode: config.payout_mode,
    reason: "",
  };
}

function buildConfig(): CommissionPolicyConfig {
  const boundaryTime = normalizeBoundaryTime(draft.value.boundaryTime) ?? draft.value.boundaryTime;
  let calendar: CommissionCalendar | null = null;
  if (draft.value.calendarSelected) {
    calendar = draft.value.cycle === "weekly"
      ? { timezone: draft.value.timezone.trim(), cycle: "weekly", boundary_time: boundaryTime, weekday: draft.value.weekday === "" ? null : Number(draft.value.weekday), month_day: null, short_month: "" }
      : { timezone: draft.value.timezone.trim(), cycle: "monthly", boundary_time: boundaryTime, weekday: null, month_day: draft.value.monthDay === "" ? null : Number(draft.value.monthDay), short_month: draft.value.shortMonth };
  }
  return { enabled: draft.value.enabled, calendar, payout_mode: draft.value.payoutMode };
}

function normalizeBoundaryTime(value: string): string | null {
  if (/^([01]\d|2[0-3]):[0-5]\d$/.test(value)) return `${value}:00`;
  return /^([01]\d|2[0-3]):[0-5]\d:[0-5]\d$/.test(value) ? value : null;
}

const reasonValid = computed(() => {
  const reason = draft.value.reason.trim();
  return Boolean(reason) && new TextEncoder().encode(reason).length <= 500 && !reason.includes("\0");
});
const calendarValid = computed(() => {
  if (!draft.value.calendarSelected) return !draft.value.enabled;
  const calendar = buildConfig().calendar;
  if (!calendar || !calendar.timezone || !calendar.cycle || !normalizeBoundaryTime(draft.value.boundaryTime)) return false;
  try { new Intl.DateTimeFormat("en", { timeZone: calendar.timezone }); } catch { return false; }
  if (calendar.cycle === "weekly") return calendar.weekday !== null && calendar.weekday >= 0 && calendar.weekday <= 6;
  return calendar.month_day !== null && calendar.month_day >= 1 && calendar.month_day <= 31 && ["last_day", "skip"].includes(calendar.short_month);
});
const canSave = computed(() => rights.value.write && policy.value !== null && policy.value.version < Number.MAX_SAFE_INTEGER && !saving.value && !loading.value && !uncertain.value && !confirmation.value && !conflict.value && calendarValid.value && reasonValid.value);
const pendingInScope = computed(() => uncertain.value ?? getPendingCommissionWrite<FrozenMutation>(contextKey()));

async function loadPolicy(preserveDraft = false) {
  if (!rights.value.view || !props.brandId) return;
  const brandId = props.brandId;
  const actorId = props.account.id;
  const ticket = generation;
  const read = ++policyRead;
  loading.value = true;
  error.value = "";
  try {
    const item = await api.getPolicy(brandId);
    if (!current(ticket, brandId, actorId) || read !== policyRead) return;
    policy.value = item;
    if (!getPendingCommissionWrite(contextKey(actorId, brandId)) && !preserveDraft) applyDraft(item.config);
  } catch (cause) {
    if (!current(ticket, brandId, actorId) || read !== policyRead) return;
    policy.value = null;
    sessionExpired(cause);
    error.value = explain(cause, t("读取佣金财务策略失败。", "Failed to load commission financial policy."));
  } finally {
    if (current(ticket, brandId, actorId) && read === policyRead) loading.value = false;
  }
}

async function loadHistory(offset = 0) {
  if (!rights.value.view || !props.brandId) return;
  const brandId = props.brandId;
  const actorId = props.account.id;
  const ticket = generation;
  const read = ++historyRead;
  loadingHistory.value = true;
  try {
    const result = await api.getHistory(brandId, 20, offset);
    if (!current(ticket, brandId, actorId) || read !== historyRead) return;
    if (result.items.some((item) => item.brand_id !== brandId)) throw new Error(t("佣金策略历史与当前品牌不匹配。", "Commission policy history does not match the selected brand."));
    history.value = result;
  } catch (cause) {
    if (!current(ticket, brandId, actorId) || read !== historyRead) return;
    sessionExpired(cause);
    error.value = explain(cause, t("读取佣金策略历史失败。", "Failed to load commission policy history."));
  } finally {
    if (current(ticket, brandId, actorId) && read === historyRead) loadingHistory.value = false;
  }
}

function requestConfirmation() {
  if (!canSave.value || !policy.value) return;
  const body = freezeCommissionPolicyBody({ version: policy.value.version, config: buildConfig(), reason: draft.value.reason.trim() }) as Readonly<UpdateCommissionPolicyBody>;
  const fingerprint = commissionPolicyBodyFingerprint(body);
  confirmation.value = Object.freeze({ actorId: props.account.id, brandId: props.brandId, body, key: createIdempotencyKey(), fingerprint });
}

function startCalendar() {
  draft.value.calendarSelected = true;
}

function clearCalendar() {
  draft.value.calendarSelected = false;
  draft.value.timezone = "";
  draft.value.cycle = "";
  draft.value.boundaryTime = "";
  draft.value.weekday = "";
  draft.value.monthDay = "";
  draft.value.shortMonth = "";
}

function validMutation(mutation: FrozenMutation) {
  return mutation.actorId === props.account.id && mutation.brandId === props.brandId &&
    commissionPolicyBodyFingerprint(mutation.body) === mutation.fingerprint;
}

async function submit(mutation: FrozenMutation) {
  if (saving.value || !rights.value.write || !validMutation(mutation)) return;
  const key = contextKey(mutation.actorId, mutation.brandId);
  const previous = getPendingCommissionWrite<FrozenMutation>(key);
  if (previous && previous.key !== mutation.key) return;
  const ticket = generation;
  saving.value = true;
  error.value = "";
  notice.value = "";
  setPendingCommissionWrite(key, mutation);
  try {
    const saved = await api.updatePolicy(mutation.brandId, mutation.body, mutation.key);
    clearPendingCommissionWrite(key, mutation.key);
    if (!current(ticket, mutation.brandId, mutation.actorId)) return;
    policy.value = saved;
    uncertain.value = null;
    applyDraft(saved.config);
    history.value = null;
    notice.value = t("策略已保存。更改仅适用于新投注；周期批次和派发工作进程尚未实现。", "Policy saved. Changes apply to new bets only; cycle batches and the payout worker are not implemented yet.");
    void loadHistory(0);
  } catch (cause) {
    const status = cause instanceof AdminApiError ? cause.status : 0;
    const outcome = classifyCommissionWriteFailure(status);
    if (!current(ticket, mutation.brandId, mutation.actorId, false)) {
      if (outcome !== "uncertain") clearPendingCommissionWrite(key, mutation.key);
      return;
    }
    if (sessionExpired(cause)) {
      clearPendingCommissionWrite(key, mutation.key);
      return;
    }
    if (outcome === "conflict") {
      clearPendingCommissionWrite(key, mutation.key);
      uncertain.value = null;
      conflict.value = true;
      error.value = t("服务器返回冲突（409）。请重新读取佣金策略并核对代理周期设置；原请求不会自动重放。", "The server returned a conflict (409). Reload the commission policy and verify the agency cycle settings; the original request will not be replayed automatically.");
    } else if (outcome === "uncertain") {
      setPendingCommissionWrite(key, mutation);
      uncertain.value = mutation;
      error.value = t("服务器结果尚未确认。请使用原请求重试；请求内容和幂等键保持不变。", "The server outcome is unconfirmed. Retry the original request; its body and idempotency key stay unchanged.");
    } else {
      clearPendingCommissionWrite(key, mutation.key);
      uncertain.value = null;
      error.value = explain(cause, t("保存佣金策略失败。请修正后重新确认。", "Failed to save commission policy. Correct the settings and review them again."));
    }
  } finally {
    if (ticket === generation && props.brandId === mutation.brandId && props.account.id === mutation.actorId) saving.value = false;
  }
}

async function confirmAndSubmit() {
  const mutation = confirmation.value;
  if (!mutation || !rights.value.write || !validMutation(mutation)) return;
  confirmation.value = null;
  await submit(mutation);
}

async function retryUncertain() {
  const mutation = uncertain.value ?? getPendingCommissionWrite<FrozenMutation>(contextKey());
  if (!mutation || !rights.value.write || saving.value) return;
  uncertain.value = mutation;
  await submit(mutation);
}

async function reloadAfterConflict() {
  if (reconciling.value) return;
  conflict.value = false;
  reconciling.value = true;
  const ticket = generation;
  await Promise.all([loadPolicy(false), loadHistory(0)]);
  if (ticket === generation && rights.value.view && policy.value) notice.value = t("已重新读取当前策略；请核对配置和代理周期后，再创建新的变更。", "The current policy was reloaded. Review the configuration and agency cycle before creating a new change.");
  reconciling.value = false;
}

function showCalendar(config: CommissionPolicyConfig) {
  if (!config.calendar) return t("未配置", "Not configured");
  const calendar = config.calendar;
  const cycle = calendar.cycle === "weekly" ? t("每周", "Weekly") : t("每月", "Monthly");
  return `${cycle} · ${calendar.timezone} · ${calendar.boundary_time}`;
}

function confirmationRows(config: CommissionPolicyConfig): { label: string; value: string }[] {
  const rows = [
    { label: t("策略状态", "Policy status"), value: config.enabled ? t("已启用", "Enabled") : t("已停用", "Disabled") },
    { label: t("派发模式", "Payout mode"), value: config.payout_mode === "manual" ? t("人工", "Manual") : t("自动（尚无工作进程）", "Automatic (no worker yet)") },
  ];
  const calendar = config.calendar;
  if (!calendar) return [...rows, { label: t("结算日历", "Cycle calendar"), value: t("未配置", "Not configured") }];
  rows.push(
    { label: t("时区", "Timezone"), value: calendar.timezone },
    { label: t("周期", "Cycle"), value: calendar.cycle === "weekly" ? t("每周", "Weekly") : t("每月", "Monthly") },
    { label: t("边界时间", "Boundary time"), value: calendar.boundary_time },
  );
  if (calendar.cycle === "weekly") {
    const weekdays = ["星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"];
    const englishWeekdays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
    rows.push({ label: t("周边界日", "Week boundary day"), value: calendar.weekday === null ? "" : t(weekdays[calendar.weekday], englishWeekdays[calendar.weekday]) });
  } else {
    rows.push(
      { label: t("月边界日", "Month boundary day"), value: calendar.month_day === null ? "" : String(calendar.month_day) },
      { label: t("短月份处理", "Short month handling"), value: calendar.short_month === "last_day" ? t("使用当月最后一天", "Use the last day of the month") : calendar.short_month === "skip" ? t("跳过该月", "Skip that month") : "" },
    );
  }
  return rows;
}

function historyPage(delta: number) {
  if (!history.value) return;
  const offset = Math.max(0, history.value.offset + delta * history.value.limit);
  void loadHistory(offset);
}

watch(() => [props.brandId, props.account, rights.value.view, rights.value.write], () => {
  generation++;
  policyRead++;
  historyRead++;
  clearVisibleData();
  error.value = "";
  const pending = getPendingCommissionWrite<FrozenMutation>(contextKey()) ??
    findPendingCommissionWrite<FrozenMutation>((item) => item.actorId === props.account.id && item.brandId === props.brandId);
  uncertain.value = pending;
  if (rights.value.view) {
    void loadPolicy(Boolean(pending));
    void loadHistory(0);
  }
}, { deep: true, flush: "sync" });

onMounted(() => {
  generation++;
  const pending = getPendingCommissionWrite<FrozenMutation>(contextKey());
  uncertain.value = pending;
  if (rights.value.view) {
    void loadPolicy(Boolean(pending));
    void loadHistory(0);
  }
});

onUnmounted(() => {
  live = false;
  generation++;
  policyRead++;
  historyRead++;
});
</script>

<template>
  <article class="panel commission-policy-panel">
    <header class="commission-policy-header">
      <div>
        <h2>{{ t("佣金财务策略", "Commission financial policy") }}</h2>
        <p>{{ t("版本化配置；只影响新投注。", "Versioned settings; changes apply to new bets only.") }}</p>
      </div>
      <span v-if="policy" class="badge-neutral">{{ t("版本", "Version") }} {{ policy.version }}</span>
    </header>

    <div class="commission-policy-notice" role="note">
      {{ t("策略已接入；周期批次和派发工作进程尚未实现。自动派发模式目前只是策略设置，不代表自动派发已运行。", "The policy is implemented; cycle batches and the payout worker are not yet implemented. Automatic payout is currently a setting and does not mean automatic payouts are running.") }}
    </div>

    <p v-if="!rights.view" class="directory-state">
      {{ t("当前账号没有此品牌的佣金策略查看权限。", "This account cannot view commission policy for this brand.") }}
    </p>
    <p v-else-if="loading && !policy" class="directory-state" role="status">
      {{ t("正在读取佣金策略…", "Loading commission policy…") }}
    </p>
    <div v-if="rights.view && error && !policy" class="commission-policy-error" role="alert">{{ error }}</div>
    <p v-else-if="!policy && !error" class="directory-state">
      {{ t("当前品牌没有可显示的佣金策略。", "No commission policy is available for this brand.") }}
    </p>

    <template v-if="rights.view && policy">
      <div v-if="error" class="commission-policy-error" role="alert">{{ error }}</div>
      <div v-if="notice" class="commission-policy-success" role="status">{{ notice }}</div>
      <p v-if="!rights.write" class="commission-policy-readonly">{{ t("只读查看。平台查看权限和超级管理员账号均不能写入品牌策略。", "Read-only. Platform view access and super administrator accounts cannot write brand policy.") }}</p>
      <p v-else-if="policy.version >= Number.MAX_SAFE_INTEGER" class="commission-policy-readonly">{{ t("版本号超出浏览器安全整数范围；当前策略仅支持查看。", "The version exceeds the browser's safe integer range; this policy is view-only here.") }}</p>

      <form class="commission-policy-form" @submit.prevent="requestConfirmation">
        <label class="commission-policy-check">
          <input v-model="draft.enabled" type="checkbox" :disabled="!rights.write || Boolean(uncertain) || conflict" />
          <span>{{ t("启用佣金策略", "Enable commission policy") }}</span>
        </label>
        <p class="commission-policy-help">{{ t("启用前必须配置周期，并且代理策略需启用相同的周/月周期。服务器会校验该条件。", "A calendar is required to enable this policy, and the agency policy must use the same weekly or monthly cycle. The server validates this requirement.") }}</p>

        <section class="commission-calendar-section">
          <div class="commission-section-heading">
            <div><h3>{{ t("结算周期日历", "Commission cycle calendar") }}</h3><p>{{ t("初始状态不预设周期、边界时间或时区。", "No cycle, boundary time, or timezone is preselected.") }}</p></div>
            <button v-if="!draft.calendarSelected" class="button" type="button" :disabled="!rights.write || Boolean(uncertain) || conflict" @click="startCalendar">{{ t("明确配置日历", "Configure calendar") }}</button>
            <button v-else class="text-button" type="button" :disabled="!rights.write || Boolean(uncertain) || conflict" @click="clearCalendar">{{ t("清除日历", "Clear calendar") }}</button>
          </div>
          <div v-if="draft.calendarSelected" class="commission-policy-grid">
            <label>{{ t("时区（IANA）", "Timezone (IANA)") }}
              <input v-model="draft.timezone" type="text" autocomplete="off" :placeholder="t('例如 Asia/Singapore', 'For example, Asia/Singapore')" :disabled="!rights.write || Boolean(uncertain) || conflict" />
            </label>
            <label>{{ t("周期", "Cycle") }}
              <select v-model="draft.cycle" :disabled="!rights.write || Boolean(uncertain) || conflict">
                <option value="">{{ t("请选择周期", "Choose a cycle") }}</option>
                <option value="weekly">{{ t("每周", "Weekly") }}</option>
                <option value="monthly">{{ t("每月", "Monthly") }}</option>
              </select>
            </label>
            <label>{{ t("周期边界时间", "Cycle boundary time") }}
              <input v-model="draft.boundaryTime" type="time" step="1" :disabled="!rights.write || Boolean(uncertain) || conflict" />
            </label>
            <label v-if="draft.cycle === 'weekly'">{{ t("周边界日", "Week boundary day") }}
              <select v-model="draft.weekday" :disabled="!rights.write || Boolean(uncertain) || conflict">
                <option value="">{{ t("请选择星期", "Choose a weekday") }}</option>
                <option value="0">{{ t("星期日", "Sunday") }}</option><option value="1">{{ t("星期一", "Monday") }}</option><option value="2">{{ t("星期二", "Tuesday") }}</option><option value="3">{{ t("星期三", "Wednesday") }}</option><option value="4">{{ t("星期四", "Thursday") }}</option><option value="5">{{ t("星期五", "Friday") }}</option><option value="6">{{ t("星期六", "Saturday") }}</option>
              </select>
            </label>
            <template v-if="draft.cycle === 'monthly'">
              <label>{{ t("月边界日", "Month boundary day") }}
                <select v-model="draft.monthDay" :disabled="!rights.write || Boolean(uncertain) || conflict">
                  <option value="">{{ t("请选择日期", "Choose a day") }}</option>
                  <option v-for="day in 31" :key="day" :value="String(day)">{{ day }}</option>
                </select>
              </label>
              <label>{{ t("短月份处理", "Short month handling") }}
                <select v-model="draft.shortMonth" :disabled="!rights.write || Boolean(uncertain) || conflict">
                  <option value="">{{ t("请选择处理方式", "Choose how to handle short months") }}</option>
                  <option value="last_day">{{ t("使用当月最后一天", "Use the last day of the month") }}</option>
                  <option value="skip">{{ t("跳过该月", "Skip that month") }}</option>
                </select>
              </label>
            </template>
          </div>
          <p v-if="draft.enabled && !draft.calendarSelected" class="commission-policy-error">{{ t("启用策略前请明确配置日历。", "Configure a calendar before enabling the policy.") }}</p>
          <p v-else-if="draft.calendarSelected && !calendarValid" class="commission-policy-help">{{ t("请填写有效的 IANA 时区、周期及边界值。", "Enter a valid IANA timezone, cycle, and boundary values.") }}</p>
        </section>

        <label class="commission-policy-field">{{ t("派发模式", "Payout mode") }}
          <select v-model="draft.payoutMode" :disabled="!rights.write || Boolean(uncertain) || conflict">
            <option value="manual">{{ t("人工", "Manual") }}</option>
            <option value="automatic">{{ t("自动（仅保存策略）", "Automatic (policy setting only)") }}</option>
          </select>
        </label>
        <p class="commission-policy-help">{{ t("选择自动模式不会启动派发；周期批次和派发工作进程尚未实现。", "Selecting automatic does not start payouts; cycle batches and the payout worker are not yet implemented.") }}</p>
        <label class="commission-policy-field">{{ t("操作原因", "Reason for change") }}
          <textarea v-model="draft.reason" rows="3" maxlength="500" :disabled="!rights.write || Boolean(uncertain) || conflict" />
        </label>
        <div class="commission-policy-actions">
          <button class="button button-primary" type="submit" :disabled="!canSave">{{ t("核对并保存", "Review and save") }}</button>
          <button v-if="conflict" class="button" type="button" :disabled="reconciling" @click="reloadAfterConflict">{{ reconciling ? t("正在重新读取…", "Reloading…") : t("重新读取策略", "Reload policy") }}</button>
          <button v-if="pendingInScope && rights.write" class="button" type="button" :disabled="saving" @click="retryUncertain">{{ saving ? t("正在重试…", "Retrying…") : t("使用原请求重试", "Retry original request") }}</button>
        </div>
        <p v-if="pendingInScope" class="commission-policy-help">{{ t("待确认请求只保存在当前页面会话中；重试会复用原配置和幂等键。", "The unconfirmed request exists only in this page session; retry reuses its original body and idempotency key.") }}</p>
      </form>

      <section v-if="confirmation" class="commission-policy-confirm" role="dialog" aria-modal="true" :aria-label="t('确认佣金策略', 'Confirm commission policy')">
        <h3>{{ t("确认佣金策略变更", "Confirm commission policy change") }}</h3>
        <p>{{ t("核对版本、周期和派发模式后提交。", "Review the version, calendar, and payout mode before submitting.") }}</p>
        <dl><dt>{{ t("提交版本", "Submitted version") }}</dt><dd>{{ confirmation.body.version }}</dd><template v-for="row in confirmationRows(confirmation.body.config)" :key="row.label"><dt>{{ row.label }}</dt><dd>{{ row.value }}</dd></template><dt>{{ t("原因", "Reason") }}</dt><dd>{{ confirmation.body.reason }}</dd></dl>
        <div class="commission-policy-actions"><button class="button button-primary" type="button" :disabled="saving" @click="confirmAndSubmit">{{ t("确认提交", "Confirm and submit") }}</button><button class="button" type="button" :disabled="saving" @click="confirmation = null">{{ t("返回修改", "Back to edit") }}</button></div>
      </section>

      <section class="commission-policy-history">
          <div class="commission-section-heading"><div><h3>{{ t("策略历史", "Policy history") }}</h3><p>{{ t("按版本倒序显示审计原因。", "Audit reasons are listed in descending version order.") }}</p></div><button class="text-button" type="button" :disabled="loadingHistory" @click="loadHistory(history?.offset ?? 0)">{{ loadingHistory ? t("读取中…", "Loading…") : t("刷新历史", "Refresh history") }}</button></div>
        <p v-if="loadingHistory && !history" class="directory-state">{{ t("正在读取历史…", "Loading history…") }}</p>
        <p v-else-if="history && history.items.length === 0" class="directory-state">{{ t("暂无策略历史。", "No policy history yet.") }}</p>
        <ol v-else-if="history" class="commission-history-list">
          <li v-for="item in history.items as CommissionPolicyRevision[]" :key="item.id">
            <div class="commission-history-heading"><b>{{ t("版本", "Version") }} {{ item.version }}</b><time>{{ item.created_at }}</time></div>
            <p>{{ showCalendar(item.config) }} · {{ item.config.enabled ? t("已启用", "Enabled") : t("已停用", "Disabled") }} · {{ item.config.payout_mode === "manual" ? t("人工", "Manual") : t("自动", "Automatic") }}</p>
            <p>{{ t("原因", "Reason") }}：{{ item.reason }}</p>
            <small>{{ t("操作人", "Changed by") }}：{{ item.changed_by ?? t("未知", "Unknown") }}<template v-if="item.audit_log_id"> · {{ t("审计记录", "Audit log") }}：{{ item.audit_log_id }}</template></small>
          </li>
        </ol>
        <div v-if="history" class="commission-policy-actions"><button class="button" type="button" :disabled="loadingHistory || history.offset === 0" @click="historyPage(-1)">{{ t("上一页", "Previous") }}</button><span>{{ history.offset + 1 }}–{{ history.offset + history.items.length }}</span><button class="button" type="button" :disabled="loadingHistory || history.items.length < history.limit" @click="historyPage(1)">{{ t("下一页", "Next") }}</button></div>
      </section>
    </template>
  </article>
</template>

<style scoped>
.commission-policy-panel { display: grid; gap: 16px; }
.commission-policy-header, .commission-section-heading, .commission-history-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.commission-policy-header h2, .commission-section-heading h3 { margin: 0; }
.commission-policy-header p, .commission-section-heading p, .commission-policy-help { margin: 5px 0 0; color: var(--muted, #64748b); font-size: 12px; }
.commission-policy-notice { padding: 12px 14px; border: 1px solid var(--warning, #d97706); border-radius: 10px; background: color-mix(in srgb, var(--warning, #d97706) 8%, transparent); line-height: 1.55; }
.commission-policy-error, .commission-policy-success, .commission-policy-readonly { padding: 10px 12px; border-radius: 8px; }
.commission-policy-error { color: var(--danger, #b42318); background: color-mix(in srgb, var(--danger, #b42318) 8%, transparent); }
.commission-policy-success { color: var(--success, #157347); background: color-mix(in srgb, var(--success, #157347) 8%, transparent); }
.commission-policy-readonly { background: var(--surface-muted, #f1f5f9); }
.commission-policy-form { display: grid; gap: 14px; }
.commission-policy-check { display: flex; align-items: center; gap: 9px; font-weight: 650; }
.commission-calendar-section, .commission-policy-history { border-top: 1px solid var(--border, #e2e8f0); padding-top: 16px; }
.commission-policy-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-top: 14px; }
.commission-policy-grid label, .commission-policy-field { display: grid; gap: 6px; font-size: 13px; font-weight: 600; }
.commission-policy-grid input, .commission-policy-grid select, .commission-policy-field select, .commission-policy-field textarea { width: 100%; min-width: 0; padding: 9px 10px; border: 1px solid var(--border, #cbd5e1); border-radius: 8px; background: var(--surface, #fff); color: inherit; font: inherit; }
.commission-policy-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
.commission-policy-confirm { padding: 16px; border: 1px solid var(--border, #cbd5e1); border-radius: 12px; background: var(--surface, #fff); }
.commission-policy-confirm dl { display: grid; grid-template-columns: minmax(110px, auto) 1fr; gap: 8px 14px; }
.commission-policy-confirm dt { color: var(--muted, #64748b); }
.commission-policy-confirm dd { margin: 0; overflow-wrap: anywhere; }
.commission-policy-history { margin-top: 4px; }
.commission-history-list { display: grid; gap: 10px; padding: 0; list-style: none; }
.commission-history-list li { padding: 12px; border: 1px solid var(--border, #e2e8f0); border-radius: 9px; }
.commission-history-list p { margin: 8px 0 0; overflow-wrap: anywhere; }
.commission-history-list small { display: block; margin-top: 8px; color: var(--muted, #64748b); overflow-wrap: anywhere; }
.commission-history-heading time { color: var(--muted, #64748b); font-size: 12px; }
@media (max-width: 640px) { .commission-policy-grid { grid-template-columns: 1fr; } .commission-policy-header, .commission-section-heading, .commission-history-heading { align-items: flex-start; } }
</style>
