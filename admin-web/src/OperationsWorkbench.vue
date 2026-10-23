<script setup lang="ts">
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
import { reportsPermissions } from "./reports-api";
import { createWorkbenchApi, workbenchPermissions, type SectionStatus, type WorkbenchSnapshot } from "./workbench-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void; (event: "navigate", destination: string, section: string): void }>();
const { t, message, display } = useAdminI18n();
const api = createWorkbenchApi();
const snapshot = ref<WorkbenchSnapshot | null>(null);
const loading = ref(false);
const error = ref<string | ReturnType<typeof message> | null>(null);
const controller = ref<AbortController | null>(null);
let generation = 0;

const permissionKey = computed(() => JSON.stringify({ id: props.account.id, brand_ids: props.account.brand_ids,
  permissions: props.account.permissions, permissions_by_brand: props.account.permissions_by_brand,
  platform_permissions: props.account.platform_permissions }));
const permissions = computed(() => workbenchPermissions(props.account, props.brandId));
const reportAccess = computed(() => reportsPermissions(props.account, props.brandId));
const scopeAvailable = computed(() => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(props.brandId));

const sections = computed(() => [
  { key: "brand", title: t('admin.copy.9c32661d'), fields: [["name", "admin.workbench.field.brandName"], ["code", "admin.workbench.field.brandCode"], ["state", "admin.workbench.field.state"]] },
  { key: "periods", title: t('admin.copy.ce59921f'), fields: [["pending", "admin.workbench.field.pending"], ["betting", "admin.workbench.field.betting"], ["closed", "admin.workbench.field.closed"], ["waiting_draw", "admin.workbench.field.awaitingDraw"], ["drawn", "admin.workbench.field.drawn"], ["settling", "admin.workbench.field.settling"], ["refund_pending", "admin.workbench.field.refundPending"], ["refund_failed", "admin.workbench.field.refundFailed"]] },
  { key: "orders", title: t('admin.copy.15883cbb'), fields: [["placed", "admin.workbench.field.placed"], ["abnormal", "admin.workbench.field.abnormal"]] },
  { key: "today_bets", title: t('admin.copy.3d16f47a'), fields: [["order_count", "admin.workbench.field.submittedOrders"], ["stake_points", "admin.workbench.field.submittedStake"], ["cancelled_count", "admin.workbench.field.cancelled"], ["abnormal_count", "admin.workbench.field.abnormal"]], note: t('admin.copy.d105e458') },
  { key: "settlement", title: t('admin.copy.ab6c95d2'), fields: [["processing", "admin.workbench.field.processing"], ["awaiting_approval", "admin.workbench.field.awaitingApproval"], ["paying", "admin.workbench.field.paying"], ["failed", "admin.workbench.field.failed"]] },
  { key: "recharges", title: t('admin.copy.56f4d2be'), fields: [["pending_count", "admin.workbench.field.pendingCount"], ["pending_points", "admin.workbench.field.pendingPoints"]] },
  { key: "ledger", title: t('admin.copy.cec2cc74'), fields: [["entry_count", "admin.workbench.field.entries"], ["net_points", "admin.workbench.field.netPoints"], ["recharge_points", "admin.workbench.field.rechargePoints"], ["prize_credit_points", "admin.workbench.field.prizeCredits"], ["prize_reversal_points", "admin.workbench.field.prizeReversals"], ["refund_points", "admin.workbench.field.refundPoints"]] },
  { key: "balances", title: t('admin.copy.ef212e9c'), fields: [["account_count", "admin.workbench.field.accounts"], ["available_points", "admin.workbench.field.availablePoints"], ["frozen_points", "admin.workbench.field.frozenPoints"], ["withdrawal_points", "admin.workbench.field.withdrawalPoints"], ["total_points", "admin.workbench.field.totalPoints"]] },
  { key: "reconciliation", title: t('admin.copy.0ff02f56'), fields: [] },
  { key: "sources", title: t('admin.copy.616fbf2e'), fields: [["adapter_state", "admin.workbench.field.adapterState"], ["configured_games", "admin.workbench.field.configuredGames"], ["enabled_api_sources", "admin.workbench.field.enabledApiSources"], ["enabled_dom_sources", "admin.workbench.field.enabledDomSources"], ["attempts_today", "admin.workbench.field.attemptsToday"], ["failed_today", "admin.workbench.field.failedToday"], ["no_data_today", "admin.workbench.field.noDataToday"], ["last_attempt_at", "admin.workbench.field.lastAttempt"]] },
  { key: "withdrawals", title: t('admin.copy.58989201'), fields: [["reviewing_count", "admin.workbench.field.awaitingReviewCount"], ["reviewing_points", "admin.workbench.field.awaitingReviewPoints"], ["processing_count", "admin.workbench.field.processingCount"], ["processing_points", "admin.workbench.field.processingPoints"]], note: t('admin.copy.4fccafac') },
  { key: "commissions", title: t('admin.copy.74828b0f'), fields: [
    ["discovery_pending_count", "admin.workbench.field.discoveryPending"], ["discovery_failed_count", "admin.workbench.field.discoveryFailed"],
    ["cycle_processing_count", "admin.workbench.field.cyclesProcessing"], ["cycle_waiting_count", "admin.workbench.field.cyclesWaiting"],
    ["cycle_ready_count", "admin.workbench.field.cyclesReady"], ["cycle_stale_count", "admin.workbench.field.cyclesStale"], ["cycle_failed_count", "admin.workbench.field.cyclesFailed"],
    ["payment_awaiting_approval_count", "admin.workbench.field.paymentsAwaitingApproval"], ["payment_processing_count", "admin.workbench.field.paymentsProcessing"],
    ["payment_blocked_count", "admin.workbench.field.paymentsBlocked"], ["payment_failed_count", "admin.workbench.field.paymentsFailed"],
    ["plan_processing_count", "admin.workbench.field.plansProcessing"], ["plan_ready_count", "admin.workbench.field.plansReady"],
    ["plan_failed_count", "admin.workbench.field.plansFailed"],
    ["execution_awaiting_approval_count", "admin.workbench.field.executionsAwaitingApproval"], ["execution_processing_count", "admin.workbench.field.executionsProcessing"],
    ["execution_paused_count", "admin.workbench.field.executionsPaused"], ["execution_failed_count", "admin.workbench.field.executionsFailed"],
  ], note: t('admin.copy.2fd75064') },
  { key: "rewards", title: t('admin.copy.b78baedb'), fields: [["granted_count", "admin.workbench.field.grantedOrders"], ["pending_count", "admin.workbench.field.revocationsPending"], ["revoked_count", "admin.workbench.field.revokedOrders"]], note: t('admin.copy.78ecbe33') },
] as const);

function clearScope(): void {
  generation += 1;
  controller.value?.abort();
  controller.value = null;
  snapshot.value = null;
  error.value = null;
  loading.value = false;
}

async function refresh(): Promise<void> {
  clearScope();
  if (!scopeAvailable.value || !props.brandId) return;
  const ticket = generation;
  const accountId = props.account.id;
  const brandId = props.brandId;
  const permissionSnapshot = permissionKey.value;
  const abort = new AbortController();
  controller.value = abort;
  loading.value = true;
  try {
    const result = await api.get(brandId, abort.signal);
    if (generation !== ticket || accountId !== props.account.id || brandId !== props.brandId || permissionSnapshot !== permissionKey.value) return;
    snapshot.value = result;
  } catch (cause) {
    if (generation !== ticket || abort.signal.aborted || accountId !== props.account.id || brandId !== props.brandId || permissionSnapshot !== permissionKey.value) return;
    if (cause instanceof AdminApiError && cause.status === 401) emit("session-invalid");
    error.value = cause instanceof AdminApiError && cause.status === 503
      ? message('admin.copy.59267a02')
      : cause instanceof AdminApiError && cause.code === "INVALID_RESPONSE"
        ? message('admin.copy.8d6ecbd4')
        : message('admin.copy.fbccf560');
  } finally {
    if (generation === ticket) { loading.value = false; controller.value = null; }
  }
}

watch(() => [props.account.id, props.brandId, permissionKey.value], () => { clearScope(); void refresh(); }, { immediate: true });
onBeforeUnmount(clearScope);

const accessBySection = (key: string) => permissions.value[key as keyof typeof permissions.value] ?? false;
const section = (key: string) => snapshot.value?.[key as keyof WorkbenchSnapshot] as { status: SectionStatus; data: Record<string, unknown> | null } | undefined;
const displayValue = (value: unknown, field?: string) => {
  if (field === 'state') {
    const keys = { active: 'admin.copy.e410d276', paused: 'admin.copy.a32d8f3f', disabled: 'admin.copy.1b7158cb' } as const;
    return t(adminKey(keys[value as keyof typeof keys]));
  }
  if (field === 'adapter_state') return t('admin.console.sourceAdapterNotConnected');
  if (value === null) return t('admin.copy.e3d4f04c');
  if (typeof value === "string" && /^\d{4}-\d\d-\d\dT/.test(value)) {
    return formatDisplayDateTime(value, displayLocale.value);
  }
  return String(value);
};
function jobSummary(): string {
  const job = section("reconciliation")?.data?.latest_job as Record<string, unknown> | null | undefined;
  if (!job) return t('admin.copy.30b0b7f4');
  const labels: Record<string, string> = { pending: 'admin.copy.99f247fd', running: 'admin.copy.19f2f373', completed: 'admin.copy.293e79cc', failed: 'admin.copy.1feabec0' };
  const key = labels[String(job.state)];
  return `${key ? t(adminKey(key)) : String(job.state)} · ${job.checked_count} / ${job.target_count}`;
}
function jobField(field: string): unknown {
  const data = section("reconciliation")?.data;
  const job = data && "latest_job" in data ? data.latest_job : null;
  return job && typeof job === "object" && field in job ? (job as Record<string, unknown>)[field] : "";
}
function destination(key: string): string | null {
  const routes: Record<string, string> = { periods: "期次和开奖", orders: "注单和异常", today_bets: "报表和对账", settlement: "期次和开奖", recharges: "资金与账本", ledger: "资金与账本", balances: "资金与账本", withdrawals: "资金与账本", reconciliation: "批量对账", sources: "期次和开奖", commissions: "佣金和奖励", rewards: "人工奖励" };
  return routes[key] ?? null;
}
function canNavigate(key: string): boolean {
  if (!accessBySection(key)) return false;
  const route = destination(key);
  if (route === "期次和开奖") return permissions.value.periods;
  if (route === "注单和异常") return permissions.value.orders;
  if (route === "批量对账") return permissions.value.reconciliation;
  if (route === "报表和对账") return reportAccess.value.betting;
  return true;
}
function canOpenReport(key: string): boolean {
  if (key === "orders") return reportAccess.value.betting;
  if (key === "ledger") return reportAccess.value.ledger;
  return false;
}
const snapshotTime = computed(() => snapshot.value ? displayValue(snapshot.value.snapshot_at) : "");
</script>

<template>
  <main class="workbench">
    <header class="workbench-head">
      <div><div class="eyebrow">{{ t('admin.copy.cb99bfa4') }}</div><h1>{{ t('admin.copy.61893b78') }}</h1>
        <p v-if="snapshot">{{ t('admin.copy.9c32661d') }} · <AssociationName :record="snapshot" kind="brand" :id="snapshot.brand_id" :locale="associationLocale" /> <span class="dot">·</span> {{ t('admin.copy.67887c27') }} {{ snapshotTime }} ({{ snapshot.timezone }})</p>
        <p v-else>{{ t('admin.copy.7b153b88') }}</p>
      </div>
      <button v-refresh-button="loading ? t('admin.copy.27342bf3') : t('admin.copy.894e2a7d')" class="refresh" type="button" :disabled="loading || !scopeAvailable" @click="withGlobalLoading(refresh)">{{ loading ? t('admin.copy.27342bf3') : t('admin.copy.894e2a7d') }}</button>
    </header>

    <div v-if="!scopeAvailable" class="notice">{{ t('admin.copy.ab986cb9') }}</div>
    <div v-else-if="error" class="notice is-error" role="alert">{{ display(error) }}</div>
    <div v-global-loading v-else-if="loading && !snapshot" class="notice" role="status">{{ t('admin.copy.41a9eab7') }}</div>
    <div v-else-if="!snapshot" class="notice">{{ t('admin.copy.5eab7aaf') }}</div>

    <template v-if="snapshot">
      <div class="scope-note">{{ t('admin.copy.8da74fbb') }}: [{{ displayValue(snapshot.day_from) }}, {{ snapshotTime }}) · {{ snapshot.timezone }}
        <span>{{ t('admin.copy.7e9b1efc') }}</span>
      </div>
      <div class="cards">
        <article v-for="item in sections" :key="item.key" class="card">
          <header class="card-head"><h2>{{ item.title }}</h2>
            <span class="badge" :class="`status-${section(item.key)?.status}`">{{ section(item.key)?.status === 'ready' ? t('admin.copy.f762d445') : section(item.key)?.status === 'forbidden' ? t('admin.copy.58b6f086') : t('admin.copy.a312b5e2') }}</span>
          </header>
          <template v-if="section(item.key)?.status === 'ready' && section(item.key)?.data">
            <template v-if="item.key === 'reconciliation'">
              <div class="job-state">{{ jobSummary() }}</div>
              <dl v-if="section(item.key)?.data?.latest_job" class="metrics">
                <template v-for="field in [['repairable_count','admin.workbench.repairable'],['corrupt_count','admin.workbench.corrupt'],['failed_count','admin.copy.1feabec0']]" :key="field[0]">
                  <div><dt>{{ t(adminKey(field[1])) }}</dt><dd>{{ jobField(field[0]) }}</dd></div>
                </template>
              </dl>
            </template>
            <dl v-else class="metrics">
              <div v-for="field in item.fields" :key="field[0]"><dt>{{ t(adminKey(field[1])) }}</dt><dd>{{ displayValue(section(item.key)?.data?.[field[0]], field[0]) }}</dd></div>
            </dl>
            <p v-if="item.key === 'sources'" class="footnote">{{ t('admin.copy.d4a778b3') }}</p>
            <p v-if="'note' in item" class="footnote">{{ item.note }}</p>
            <button v-if="destination(item.key)" class="action-link" type="button" :disabled="!canNavigate(item.key)" @click="emit('navigate', destination(item.key)!, item.key)">{{ t('admin.copy.32931190') }} <span aria-hidden="true">→</span></button>
            <button v-if="canOpenReport(item.key)" class="action-link report-link" type="button" @click="emit('navigate', '报表和对账', item.key)">{{ t('admin.copy.f67efc87') }} <span aria-hidden="true">→</span></button>
          </template>
          <p v-else class="unavailable">{{ t('admin.copy.7e3861b6') }}</p>
        </article>
      </div>
    </template>
  </main>
</template>

<style scoped>
.workbench{max-width:1440px;margin:0 auto;padding:0 0 32px;color:var(--ink)}.workbench-head{display:flex;justify-content:space-between;align-items:flex-start;gap:24px;margin-bottom:22px}.eyebrow{color:var(--primary);font-size:11px;font-weight:750;letter-spacing:.1em;text-transform:uppercase}.workbench h1{font-size:clamp(22px,2.2vw,30px);line-height:1.2;letter-spacing:-.035em;margin:7px 0 8px}.workbench-head p{color:var(--muted);margin:0;line-height:1.55}.dot{padding:0 5px;color:#b1b7c2}.scope-note,.notice{border:1px solid var(--line);background:var(--surface);border-radius:10px;padding:13px 16px;color:#626b7b;line-height:1.55;margin-bottom:16px}.scope-note span{display:block;color:#9097a4;font-size:12px;margin-top:4px}.notice{margin-top:20px}.is-error{border-color:#f0c8c8;background:#fffafa;color:#9e3f43}.cards{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px}.card{min-width:0;background:var(--surface);border:1px solid var(--line);border-radius:12px;padding:17px;box-shadow:0 3px 12px #1e2a5008}.card-head{display:flex;justify-content:space-between;align-items:center;gap:10px;margin-bottom:14px}.card h2{font-size:15px;margin:0;letter-spacing:-.01em}.badge{font-size:11px;font-weight:700;border-radius:99px;padding:5px 8px;white-space:nowrap}.status-ready{color:#237354;background:#eaf6f0}.status-forbidden{color:#767e8d;background:#f1f2f5}.metrics{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:13px 10px;margin:0}.metrics div{min-width:0}.metrics dt{font-size:11px;color:#858d9b;line-height:1.4}.metrics dd{font-size:18px;font-variant-numeric:tabular-nums;overflow-wrap:anywhere;font-weight:700;margin:4px 0 0}.job-state{font-size:15px;font-weight:700;margin:2px 0 15px}.unavailable{font-size:13px;color:#8b929f;line-height:1.55;min-height:44px;margin:0}.footnote{font-size:11px;color:#858d9b;line-height:1.5;margin:13px 0 0}.action-link{margin-top:15px;padding:0;border:0;background:transparent;color:var(--primary);font-size:12px;font-weight:700}.action-link:disabled{color:#9298a5;background:none;opacity:.6}.report-link{margin-left:12px}
@media(max-width:1000px){.cards{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:560px){.workbench{padding:0 0 24px}.workbench-head{gap:12px}.workbench-head p{font-size:12px}.cards{grid-template-columns:1fr;gap:10px}.card{padding:15px}.metrics dd{font-size:17px}}
</style>
