<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import type { LocalizedMessage } from "@lottery/shared";
import { useAdminI18n } from "./i18n";
import {
  commissionCyclePermissions, createCommissionCyclesApi,
  type CommissionCycleRecord, type CommissionCyclePage, type CommissionEarningPage,
  type CommissionRun, type CommissionRunPage, type CommissionCalculationPage,
  type CommissionDiscovery, type CommissionDiscoveryPage, type CommissionCreateBody, type CommissionRetryBody,
} from "./commission-cycles-api";
import {
  classifyCommissionCycleWriteFailure, clearPendingCommissionCycleWrite,
  commissionCycleSessionGeneration, commissionCycleWriteScopeKey,
  freezeCommissionCycleBody, getPendingCommissionCycleWrite,
  listPendingCommissionCycleWrites, setPendingCommissionCycleWrite,
  type CommissionWriteScope, type PendingCommissionCycleWrite,
} from "./commission-cycles-state";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, locale, message } = useAdminI18n();
const api = createCommissionCyclesApi();
const PAGE_SIZE = 20;
const knownConflictKeys = new Set<string>();
const rights = computed(() => commissionCyclePermissions(props.account, props.brandId));
const scopeStamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, rights.value]));

const cycles = ref<CommissionCycleRecord[]>([]), cycleTotal = ref("0"), cycleOffset = ref(0);
const selectedCycle = ref<CommissionCycleRecord | null>(null);
const receipt = ref<{ record: CommissionCycleRecord | CommissionDiscovery; operation: CommissionWriteScope["operation"] } | null>(null);
const earnings = ref<CommissionEarningPage | null>(null), earningOffset = ref(0);
const runs = ref<CommissionRunPage | null>(null), runOffset = ref(0);
const selectedRunId = ref("");
const calculations = ref<CommissionCalculationPage | null>(null), calculationOffset = ref(0);
const discoveries = ref<CommissionDiscovery[]>([]), discoveryTotal = ref("0"), discoveryOffset = ref(0);
const selectedDiscovery = ref<CommissionDiscovery | null>(null);

const anchorOrderId = ref(""), createReason = ref(""), retryReason = ref("");
const review = ref<PendingCommissionCycleWrite | null>(null), confirmed = ref(false);
const pendingIntents = ref<PendingCommissionCycleWrite[]>([]);
const conflictKeys = ref<string[]>([]), reloadedConflictKeys = ref<string[]>([]), reviewedConflictKeys = ref<string[]>([]);
const loading = ref(false), detailLoading = ref(false), earningsLoading = ref(false), runsLoading = ref(false);
const calculationsLoading = ref(false), discoveryLoading = ref(false), writing = ref(false);
const conflictReloading = ref(false);
const listError = ref(""), detailError = ref(""), earningsError = ref(""), runsError = ref("");
const calculationsError = ref(""), discoveryError = ref(""), writeError = ref(""), notice = ref<string | LocalizedMessage>("");
let alive = true, generation = 0, cycleTicket = 0, detailTicket = 0, earningTicket = 0, runTicket = 0;
let calculationTicket = 0, discoveryTicket = 0, writeTicket = 0;
let conflictReloadTicket = 0;

const canCreate = computed(() => rights.value.run && !props.account.super_admin && !writing.value && !pendingIntents.value.some((item) => item.operation === "create") && validReason(createReason.value) && validUuid(anchorOrderId.value.trim()));
const canRetryCycle = computed(() => Boolean(rights.value.retry && !props.account.super_admin && selectedCycle.value?.state === "failed" && selectedCycle.value.version < Number.MAX_SAFE_INTEGER && validReason(retryReason.value) && !writing.value && !pendingFor("retry_cycle", selectedCycle.value.id)));
const canRetryDiscovery = computed(() => Boolean(rights.value.retry && !props.account.super_admin && selectedDiscovery.value?.state === "failed" && selectedDiscovery.value.version < Number.MAX_SAFE_INTEGER && validReason(retryReason.value) && !writing.value && !pendingFor("retry_discovery", selectedDiscovery.value.id)));
const createReasonBytes = computed(() => new TextEncoder().encode(createReason.value).length);
const cycleHasNext = computed(() => BigInt(cycleOffset.value + PAGE_SIZE) < BigInt(cycleTotal.value));
const discoveryHasNext = computed(() => BigInt(discoveryOffset.value + PAGE_SIZE) < BigInt(discoveryTotal.value));

function validUuid(value: string): boolean { return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value); }
function validReason(value: string): boolean {
  if (!value || value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; }
    else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}
function statusText(value: string): string {
  const labels: Record<string, [string, string]> = {
    enumerating: ["登记中", "Registering"], waiting: ["等待证据", "Waiting for evidence"], calculating: ["核算中", "Calculating"],
    summarizing: ["汇总中", "Summarizing"], ready: ["核算就绪", "Calculated"], failed: ["失败", "Failed"],
    pending: ["待处理", "Pending"], registered: ["已登记", "Registered"], abandoned: ["已停止的代次", "Abandoned run"],
    won: ["中奖", "Won"], lost: ["未中奖", "Lost"], abnormal: ["异常", "Abnormal"], bet_cancelled: ["投注已取消", "Bet cancelled"], judged_cancelled: ["开奖已取消", "Draw cancelled"],
    eligible: ["符合核算条件", "Eligible"], policy_disabled: ["政策已停用", "Policy disabled"], unattributed: ["无代理归属", "Unattributed"],
    other_cycle: ["属于其他周期", "Another cycle"], cancelled: ["已取消", "Cancelled"],
  };
  return labels[value] ? t(...labels[value]) : value;
}
function date(value: string | null | undefined): string {
  if (!value) return t("无", "None");
  const parsed = new Date(value);
  if (!Number.isFinite(parsed.getTime())) return value;
  return new Intl.DateTimeFormat(locale.value === "en" ? "en" : "zh-CN", { dateStyle: "medium", timeStyle: "medium", timeZone: "UTC" }).format(parsed) + " UTC";
}
function exact(value: string | { numerator: string; denominator: string }) { return typeof value === "string" ? value : `${value.numerator}/${value.denominator}`; }
function pageLabel(offset: number, count: number) { return count === 0 ? "0–0" : `${offset + 1}–${offset + count}`; }
function pageCount(total: string) {
  const n = BigInt(total), size = BigInt(PAGE_SIZE);
  return n === 0n ? "1" : ((n + size - 1n) / size).toString();
}
function cycleScope(): CommissionWriteScope { return { accountId: props.account.id, brandId: props.brandId, operation: "create" }; }
function retryScope(operation: "retry_cycle" | "retry_discovery", targetId: string): CommissionWriteScope { return { accountId: props.account.id, brandId: props.brandId, operation, targetId }; }
function pendingFor(operation: CommissionWriteScope["operation"], targetId?: string) {
  return pendingIntents.value.find((item) => item.operation === operation && item.targetId === targetId);
}
function refreshPending() {
  pendingIntents.value = listPendingCommissionCycleWrites(props.account.id, props.brandId);
  conflictKeys.value = pendingIntents.value.filter((intent) => knownConflictKeys.has(intent.key)).map((intent) => intent.key);
}
function isConflict(intent: PendingCommissionCycleWrite) { return conflictKeys.value.includes(intent.key); }
function conflictReloaded(intent: PendingCommissionCycleWrite) { return reloadedConflictKeys.value.includes(intent.key); }
function conflictReviewed(intent: PendingCommissionCycleWrite) { return reviewedConflictKeys.value.includes(intent.key); }
function markConflictReviewed(intent: PendingCommissionCycleWrite, event: Event) {
  const keys = new Set(reviewedConflictKeys.value), checked = (event.target as HTMLInputElement).checked;
  if (checked) keys.add(intent.key); else keys.delete(intent.key);
  reviewedConflictKeys.value = [...keys];
}
function current(ticket: number, laneTicket: number, captured: string, session: number) {
  return alive && ticket === laneTicket && captured === scopeStamp.value && session === commissionCycleSessionGeneration() && rights.value.view;
}
function currentWrite(ticket: number, captured: string, session: number) {
  return alive && ticket === writeTicket && captured === scopeStamp.value && session === commissionCycleSessionGeneration();
}
function clearVisible() {
  cycles.value = []; cycleTotal.value = "0"; cycleOffset.value = 0; selectedCycle.value = null;
  receipt.value = null;
  earnings.value = null; runs.value = null; selectedRunId.value = ""; calculations.value = null;
  discoveries.value = []; discoveryTotal.value = "0"; discoveryOffset.value = 0; selectedDiscovery.value = null;
  anchorOrderId.value = ""; createReason.value = ""; retryReason.value = ""; review.value = null; confirmed.value = false;
  listError.value = detailError.value = earningsError.value = runsError.value = calculationsError.value = discoveryError.value = writeError.value = notice.value = "";
  loading.value = detailLoading.value = earningsLoading.value = runsLoading.value = calculationsLoading.value = discoveryLoading.value = writing.value = false;
  refreshPending();
}
function resetScope() {
  conflictReloadTicket++; conflictReloading.value = false;
  reloadedConflictKeys.value = []; reviewedConflictKeys.value = [];
  generation++; cycleTicket++; detailTicket++; earningTicket++; runTicket++; calculationTicket++; discoveryTicket++; writeTicket++;
  clearVisible();
  if (rights.value.view) { void loadCycles(0); void loadDiscoveries(0); }
}
function sessionExpired(cause: unknown, captured: string, session: number) {
  if (cause instanceof AdminApiError && cause.status === 401 && alive && captured === scopeStamp.value && session === commissionCycleSessionGeneration()) {
    generation++; cycleTicket++; detailTicket++; earningTicket++; runTicket++; calculationTicket++; discoveryTicket++; writeTicket++;
    clearVisible();
    emit("session-invalid");
    return true;
  }
  return false;
}
function readFailure(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 0) return t("网络或响应不可用，当前数据尚未读取。", "The network or response is unavailable; current data was not loaded.");
  return cause instanceof Error ? cause.message : t("读取失败。", "Unable to load data.");
}
function writeFailure(cause: unknown) { return cause instanceof Error ? cause.message : t("写入失败。", "Write failed."); }

async function loadCycles(offset = cycleOffset.value) {
  if (!rights.value.view) return;
  const ticket = ++cycleTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  loading.value = true; listError.value = "";
  try {
    const page: CommissionCyclePage = await api.list(props.brandId, PAGE_SIZE, offset);
    if (!current(ticket, cycleTicket, captured, session)) return;
    cycles.value = page.items; cycleTotal.value = page.total_count; cycleOffset.value = page.offset;
    if (selectedCycle.value) {
      const selectedId = selectedCycle.value.id, listed = page.items.find((item) => item.id === selectedId);
      const changed = !listed || listed.version !== selectedCycle.value.version || listed.state !== selectedCycle.value.state ||
        listed.current_run_id !== selectedCycle.value.current_run_id || listed.current_generation !== selectedCycle.value.current_generation ||
        listed.evidence_epoch !== selectedCycle.value.evidence_epoch || listed.evidence_current !== selectedCycle.value.evidence_current ||
        listed.calculated_count !== selectedCycle.value.calculated_count || listed.earning_count !== selectedCycle.value.earning_count || listed.total_points !== selectedCycle.value.total_points;
      if (changed) void loadCycle(selectedId);
      else if (listed) selectedCycle.value = listed;
    }
  } catch (cause) { if (current(ticket, cycleTicket, captured, session)) { sessionExpired(cause, captured, session); listError.value = readFailure(cause); } }
  finally { if (current(ticket, cycleTicket, captured, session)) loading.value = false; }
}
async function loadDiscoveries(offset = discoveryOffset.value) {
  if (!rights.value.view) return;
  const ticket = ++discoveryTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  discoveryLoading.value = true; discoveryError.value = "";
  try {
    const page: CommissionDiscoveryPage = await api.discoveries(props.brandId, PAGE_SIZE, offset);
    if (!current(ticket, discoveryTicket, captured, session)) return;
    discoveries.value = page.items; discoveryTotal.value = page.total_count; discoveryOffset.value = page.offset;
    if (selectedDiscovery.value) selectedDiscovery.value = page.items.find((item) => item.id === selectedDiscovery.value?.id) ?? null;
  } catch (cause) { if (current(ticket, discoveryTicket, captured, session)) { sessionExpired(cause, captured, session); discoveryError.value = readFailure(cause); } }
  finally { if (current(ticket, discoveryTicket, captured, session)) discoveryLoading.value = false; }
}
async function loadCycle(id: string) {
  const ticket = ++detailTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  detailLoading.value = true; detailError.value = ""; selectedCycle.value = null; retryReason.value = "";
  earningTicket++; runTicket++; calculationTicket++; earnings.value = null; runs.value = null; calculations.value = null; selectedRunId.value = "";
  earningsError.value = runsError.value = calculationsError.value = ""; refreshPending();
  try {
    const item = await api.read(props.brandId, id);
    if (!current(ticket, detailTicket, captured, session) || item.id !== id) return;
    selectedCycle.value = item;
    await Promise.all([loadEarnings(id, 0), loadRuns(id, 0)]);
  } catch (cause) { if (current(ticket, detailTicket, captured, session)) { sessionExpired(cause, captured, session); detailError.value = readFailure(cause); } }
  finally { if (current(ticket, detailTicket, captured, session)) detailLoading.value = false; }
}
async function loadEarnings(cycleId: string, offset = earningOffset.value) {
  const ticket = ++earningTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  earningsLoading.value = true; earningsError.value = ""; earnings.value = null;
  try {
    const page: CommissionEarningPage = await api.earnings(props.brandId, cycleId, PAGE_SIZE, offset);
    if (!current(ticket, earningTicket, captured, session) || selectedCycle.value?.id !== cycleId) return;
    const currentRunId = selectedCycle.value.current_run_id;
    if (page.cycle_id !== cycleId || page.items.some((row) => row.cycle_id !== cycleId || row.run_id !== currentRunId)) {
      earnings.value = null;
      earningsError.value = t("核算代次已变化，请重新读取周期。", "The calculation generation changed. Reload the cycle.");
      return;
    }
    earnings.value = page; earningOffset.value = page.offset;
  } catch (cause) { if (current(ticket, earningTicket, captured, session) && selectedCycle.value?.id === cycleId) { sessionExpired(cause, captured, session); earningsError.value = readFailure(cause); } }
  finally { if (current(ticket, earningTicket, captured, session)) earningsLoading.value = false; }
}
async function loadRuns(cycleId: string, offset = runOffset.value) {
  const ticket = ++runTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  runsLoading.value = true; runsError.value = ""; runs.value = null; selectedRunId.value = ""; calculationTicket++; calculations.value = null;
  try {
    const page: CommissionRunPage = await api.runs(props.brandId, cycleId, PAGE_SIZE, offset);
    if (!current(ticket, runTicket, captured, session) || selectedCycle.value?.id !== cycleId) return;
    if (page.cycle_id !== cycleId || page.items.some((row) => row.cycle_id !== cycleId)) throw new Error(t("核算代次与所选周期不匹配。", "Runs do not match the selected cycle."));
    runs.value = page; runOffset.value = page.offset;
    const selected = page.items.find((run) => run.id === selectedRunId.value) ?? page.items[0];
    if (selected) selectRun(selected.id);
  } catch (cause) { if (current(ticket, runTicket, captured, session) && selectedCycle.value?.id === cycleId) { sessionExpired(cause, captured, session); runsError.value = readFailure(cause); } }
  finally { if (current(ticket, runTicket, captured, session)) runsLoading.value = false; }
}
async function selectRun(runId: string) {
  if (!selectedCycle.value || !runs.value?.items.some((item) => item.id === runId)) return;
  selectedRunId.value = runId; await loadCalculations(selectedCycle.value.id, runId, 0);
}
async function loadCalculations(cycleId: string, runId: string, offset = calculationOffset.value) {
  const ticket = ++calculationTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  calculationsLoading.value = true; calculationsError.value = ""; calculations.value = null;
  try {
    const page: CommissionCalculationPage = await api.calculations(props.brandId, cycleId, runId, PAGE_SIZE, offset);
    if (!current(ticket, calculationTicket, captured, session) || selectedCycle.value?.id !== cycleId || selectedRunId.value !== runId) return;
    if (page.cycle_id !== cycleId || page.run_id !== runId || page.items.some((row) => row.cycle_id !== cycleId || row.run_id !== runId)) throw new Error(t("计算轨迹与明确选择的代次不匹配。", "Calculations do not match the explicitly selected run."));
    calculations.value = page; calculationOffset.value = page.offset;
  } catch (cause) { if (current(ticket, calculationTicket, captured, session) && selectedCycle.value?.id === cycleId && selectedRunId.value === runId) { sessionExpired(cause, captured, session); calculationsError.value = readFailure(cause); } }
  finally { if (current(ticket, calculationTicket, captured, session)) calculationsLoading.value = false; }
}
function selectDiscovery(item: CommissionDiscovery) { selectedDiscovery.value = item; retryReason.value = ""; refreshPending(); }

function createIntent(): PendingCommissionCycleWrite | null {
  const scope = cycleScope(), old = getPendingCommissionCycleWrite(scope);
  if (old) return old;
  if (!canCreate.value) return null;
  const body = freezeCommissionCycleBody({ anchor_order_id: anchorOrderId.value.trim(), reason: createReason.value });
  return Object.freeze({ ...scope, body, key: createIdempotencyKey() });
}
function retryIntent(operation: "retry_cycle" | "retry_discovery"): PendingCommissionCycleWrite | null {
  const targetId = operation === "retry_cycle" ? selectedCycle.value?.id : selectedDiscovery.value?.id;
  const target = operation === "retry_cycle" ? selectedCycle.value : selectedDiscovery.value;
  if (!targetId || !target || target.state !== "failed" || target.version >= Number.MAX_SAFE_INTEGER) return null;
  const scope = retryScope(operation, targetId), old = getPendingCommissionCycleWrite(scope);
  if (old) return old;
  if (!rights.value.retry || props.account.super_admin || !validReason(retryReason.value) || writing.value) return null;
  const body = freezeCommissionCycleBody({ version: target.version, reason: retryReason.value }) as Readonly<CommissionRetryBody>;
  return Object.freeze({ ...scope, body, key: createIdempotencyKey() });
}
function beginReview(intent: PendingCommissionCycleWrite | null) {
  if (!intent || intent.accountId !== props.account.id || intent.brandId !== props.brandId || writing.value ||
    !(intent.operation === "create" ? rights.value.run : rights.value.retry) || props.account.super_admin) return;
  review.value = intent; confirmed.value = false; writeError.value = "";
}
async function submit(intent: PendingCommissionCycleWrite) {
  if (intent.accountId !== props.account.id || intent.brandId !== props.brandId || !rights.value.view || props.account.super_admin || writing.value ||
    !(intent.operation === "create" ? rights.value.run : rights.value.retry)) return;
  const captured = scopeStamp.value, session = commissionCycleSessionGeneration(), ticket = ++writeTicket;
  const scope: CommissionWriteScope = { accountId: intent.accountId, brandId: intent.brandId, operation: intent.operation, ...(intent.targetId ? { targetId: intent.targetId } : {}) };
  const existing = getPendingCommissionCycleWrite(scope);
  if (existing && existing.key !== intent.key) return;
  if (!existing) setPendingCommissionCycleWrite(scope, intent);
  writing.value = true; review.value = null; confirmed.value = false; writeError.value = ""; notice.value = "";
  try {
    const writeReceipt = intent.operation === "create"
      ? await api.create(intent.brandId, intent.body as Readonly<CommissionCreateBody>, intent.key, intent.accountId)
      : intent.operation === "retry_cycle"
        ? await api.retryCycle(intent.brandId, intent.targetId!, intent.body as Readonly<CommissionRetryBody>, intent.key)
        : await api.retryDiscovery(intent.brandId, intent.targetId!, intent.body as Readonly<CommissionRetryBody>, intent.key);
    // The API has validated the receipt. Clear only the exact intent whose key was sent.
    if (!currentWrite(ticket, captured, session)) return;
    clearPendingCommissionCycleWrite(scope, intent.key);
    receipt.value = { record: writeReceipt, operation: intent.operation };
    notice.value = message("服务器已确认原始请求回执；当前状态将通过独立只读读取更新。", "The server confirmed the original request receipt. Current state will be refreshed through separate read-only requests.");
    refreshPending();
    if (intent.operation === "create" || intent.operation === "retry_cycle") {
      const cycleId = writeReceipt.id;
      await Promise.all([loadCycles(cycleOffset.value), loadCycle(cycleId)]);
    } else {
      // Keep the validated write receipt separate from current list state. Refresh only this visible page.
      await loadDiscoveries(discoveryOffset.value);
    }
  } catch (cause) {
    if (!currentWrite(ticket, captured, session)) return;
    if (sessionExpired(cause, captured, session)) return;
    const status = cause instanceof AdminApiError ? cause.status : 0;
    const outcome = classifyCommissionCycleWriteFailure(status);
    if (outcome === "unknown") {
      setPendingCommissionCycleWrite(scope, intent); refreshPending();
      writeError.value = t("写入结果未知。冻结的原请求体与幂等键已保留；只读刷新和 409 不会清除此请求。请明确确认并重放同一请求。", "The write outcome is unknown. The frozen body and idempotency key are retained; read-only refreshes and 409 responses do not clear it. Explicitly confirm and replay this same request.");
    } else if (outcome === "conflict") {
      setPendingCommissionCycleWrite(scope, intent); refreshPending();
      knownConflictKeys.add(intent.key);
      if (!conflictKeys.value.includes(intent.key)) conflictKeys.value = [...conflictKeys.value, intent.key];
      reloadedConflictKeys.value = reloadedConflictKeys.value.filter((key) => key !== intent.key);
      reviewedConflictKeys.value = reviewedConflictKeys.value.filter((key) => key !== intent.key);
      writeError.value = t("服务器返回 409。原请求体与幂等键已保留且不会替换；请先明确重新读取受影响的数据，再审阅最新状态后决定是否丢弃旧请求并重新填写。", "The server returned 409. The original body and key are retained and will not be replaced. Explicitly reload the affected data, review the latest state, then decide whether to discard the old request and enter a new one.");
    } else {
      clearPendingCommissionCycleWrite(scope, intent.key); refreshPending();
      knownConflictKeys.delete(intent.key);
      conflictKeys.value = conflictKeys.value.filter((key) => key !== intent.key);
      reloadedConflictKeys.value = reloadedConflictKeys.value.filter((key) => key !== intent.key);
      writeError.value = writeFailure(cause);
    }
  } finally { if (currentWrite(ticket, captured, session)) writing.value = false; }
}
async function reloadConflictedIntent(intent: PendingCommissionCycleWrite) {
  if (!rights.value.view || writing.value || conflictReloading.value || !isConflict(intent)) return;
  const captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  const reloadTicket = ++conflictReloadTicket;
  const listTicket = intent.operation === "retry_discovery" ? cycleTicket : ++cycleTicket;
  const targetTicket = intent.operation === "retry_discovery" ? ++discoveryTicket : intent.operation === "retry_cycle" ? ++detailTicket : detailTicket;
  const stillCurrent = () => alive && reloadTicket === conflictReloadTicket && captured === scopeStamp.value && session === commissionCycleSessionGeneration() && rights.value.view &&
    (intent.operation === "retry_discovery" ? targetTicket === discoveryTicket : listTicket === cycleTicket && (intent.operation !== "retry_cycle" || targetTicket === detailTicket));
  conflictReloading.value = true;
  reloadedConflictKeys.value = reloadedConflictKeys.value.filter(key => key !== intent.key);
  reviewedConflictKeys.value = reviewedConflictKeys.value.filter(key => key !== intent.key);
  writeError.value = "";
  try {
    if (intent.operation === "create") {
      const page = await api.list(props.brandId, PAGE_SIZE, 0);
      if (!stillCurrent()) return;
      cycles.value = page.items; cycleTotal.value = page.total_count; cycleOffset.value = page.offset;
    } else if (intent.operation === "retry_cycle") {
      const [item, page] = await Promise.all([api.read(props.brandId, intent.targetId!), api.list(props.brandId, PAGE_SIZE, cycleOffset.value)]);
      if (!stillCurrent()) return;
      selectedCycle.value = item; cycles.value = page.items; cycleTotal.value = page.total_count; cycleOffset.value = page.offset;
      earningTicket++; runTicket++; calculationTicket++;
      earnings.value = null; runs.value = null; calculations.value = null; selectedRunId.value = "";
      void loadEarnings(item.id, 0); void loadRuns(item.id, 0);
    } else {
      const latest = await api.discoveries(props.brandId, PAGE_SIZE, discoveryOffset.value);
      if (!stillCurrent()) return;
      const found = latest.items.find((item) => item.id === intent.targetId);
      discoveries.value = latest.items; discoveryTotal.value = latest.total_count; discoveryOffset.value = latest.offset;
      if (!found) throw new Error(t("目标不在当前页。请手动翻页找到此记录，再重新读取。", "The target is not on the current page. Use pagination to find it, then reload again."));
      selectedDiscovery.value = found;
    }
    if (stillCurrent()) {
      reloadedConflictKeys.value = [...new Set([...reloadedConflictKeys.value, intent.key])];
      writeError.value = t("最新状态已重新读取。请检查上方详情，然后明确丢弃旧请求；之后重新填写并审阅新的请求。", "The latest state has been reloaded. Review the details above, explicitly discard the old request, then fill in and review a new request.");
    }
  } catch (cause) {
    if (stillCurrent()) {
      sessionExpired(cause, captured, session);
      writeError.value = readFailure(cause);
    }
  } finally {
    if (alive && reloadTicket === conflictReloadTicket) conflictReloading.value = false;
  }
}
function recover(intent: PendingCommissionCycleWrite) { beginReview(intent); }
function confirmWrite() { if (review.value && confirmed.value && !writing.value) void submit(review.value); }
function discardIntent(intent: PendingCommissionCycleWrite) {
  if (!isConflict(intent) || !conflictReloaded(intent) || !conflictReviewed(intent)) return;
  const scope: CommissionWriteScope = { accountId: intent.accountId, brandId: intent.brandId, operation: intent.operation, ...(intent.targetId ? { targetId: intent.targetId } : {}) };
  clearPendingCommissionCycleWrite(scope, intent.key); refreshPending();
  knownConflictKeys.delete(intent.key);
  if (intent.operation === "create") { anchorOrderId.value = ""; createReason.value = ""; }
  else retryReason.value = "";
  conflictKeys.value = conflictKeys.value.filter((key) => key !== intent.key);
  reloadedConflictKeys.value = reloadedConflictKeys.value.filter((key) => key !== intent.key);
  reviewedConflictKeys.value = reviewedConflictKeys.value.filter((key) => key !== intent.key);
  if (review.value?.key === intent.key) { review.value = null; confirmed.value = false; }
}
function pageCycles(delta: number) { if (!loading.value) void loadCycles(Math.max(0, cycleOffset.value + delta * PAGE_SIZE)); }
function pageDiscoveries(delta: number) { if (!discoveryLoading.value) void loadDiscoveries(Math.max(0, discoveryOffset.value + delta * PAGE_SIZE)); }
function pageEarnings(delta: number) { if (selectedCycle.value && earnings.value) void loadEarnings(selectedCycle.value.id, Math.max(0, earningOffset.value + delta * PAGE_SIZE)); }
function pageRuns(delta: number) { if (selectedCycle.value) void loadRuns(selectedCycle.value.id, Math.max(0, runOffset.value + delta * PAGE_SIZE)); }
function pageCalculations(delta: number) { if (selectedCycle.value && selectedRunId.value) void loadCalculations(selectedCycle.value.id, selectedRunId.value, Math.max(0, calculationOffset.value + delta * PAGE_SIZE)); }
function recoveryReason(intent: PendingCommissionCycleWrite) { return intent.body.reason; }
function intentLabel(intent: PendingCommissionCycleWrite) { return intent.operation === "create" ? t("手动登记周期", "Register cycle") : intent.operation === "retry_cycle" ? t("重试周期", "Retry cycle") : t("重试发现记录", "Retry discovery"); }
function operationText(operation: CommissionWriteScope["operation"]) { return operation === "create" ? t("手动登记周期", "Register cycle") : operation === "retry_cycle" ? t("重试周期", "Retry cycle") : t("重试发现记录", "Retry discovery"); }
function receiptState() { return receipt.value ? receipt.value.record.state : ""; }
function receiptVersion() { return receipt.value ? receipt.value.record.version : ""; }
function receiptAuditId() {
  const record = receipt.value?.record;
  if (!record) return t("无", "None");
  return "creation_audit_log_id" in record ? record.creation_audit_log_id : record.last_audit_log_id ?? t("无", "None");
}

watch(scopeStamp, resetScope, { immediate: true });
onUnmounted(() => { alive = false; generation++; cycleTicket++; detailTicket++; earningTicket++; runTicket++; calculationTicket++; discoveryTicket++; writeTicket++; });
</script>

<template>
  <main class="cycles commission-cycles" aria-labelledby="cycles-title">
    <header class="head"><div><p class="eyebrow">COMMISSION OPERATIONS</p><h1 id="cycles-title">{{ t("佣金周期核算", "Commission cycle calculations") }}</h1><p>{{ t("查看真实投注产生的周期、核算证据与发现队列。核算金额不是余额；本页不提供派发操作。", "Review actual bet driven cycles, calculation evidence, and discovery records. Calculated amounts are not balances; this page does not provide payout actions.") }}</p></div><button type="button" :disabled="loading || discoveryLoading || !rights.view" @click="loadCycles(cycleOffset); loadDiscoveries(discoveryOffset)">{{ t("刷新", "Refresh") }}</button></header>
    <p v-if="notice" class="notice" role="status">{{ t(notice) }}</p><p v-if="writeError" class="error" role="alert">{{ writeError }}</p>
    <section v-if="!rights.view" class="panel" role="status">{{ t("当前账号没有此品牌佣金周期查看权限。", "This account cannot view commission cycles for this brand.") }}</section>
    <template v-else>
      <section class="panel intro"><h2>{{ t("核算页不提供派发操作", "Payout actions are not available on the calculation page") }}</h2><p>{{ t("ready 仅表示本代次的计算与代理收入汇总已完成，不代表派发状态已查询。派发状态请在独立佣金派发后台读取。", "ready means only that calculations and agent earnings are complete for this generation; it does not mean payout state was queried. Read payout state in the separate commission payouts console.") }}</p></section>

      <section v-if="receipt" class="panel commission-receipt" role="status" aria-live="polite"><div class="section-title"><div><h2>{{ t("服务器确认的原始回执", "Server-confirmed original receipt") }}</h2><p>{{ t("以下是写入返回并经 API 校验的原始版本；它与列表中的当前状态分开显示。", "This is the original version returned by the write and validated by the API, shown separately from current list state.") }}</p></div><span class="tag good">{{ t("已确认", "Confirmed") }}</span></div><dl class="facts"><dt>{{ t("操作", "Operation") }}</dt><dd>{{ operationText(receipt.operation) }}</dd><dt>{{ t("记录编号", "Record ID") }}</dt><dd class="id">{{ receipt.record.id }}</dd><dt>{{ t("回执状态 / 版本", "Receipt state / version") }}</dt><dd>{{ statusText(receiptState()) }} · v{{ receiptVersion() }}</dd><dt>{{ t("回执审计记录", "Receipt audit record") }}</dt><dd class="id">{{ receiptAuditId() }}</dd></dl></section>

      <section v-if="pendingIntents.length" class="panel pending" aria-live="polite">
        <div class="section-title"><div><h2>{{ t("待恢复的原请求", "Original requests to recover") }}</h2><p>{{ t("即使离开页面，未知结果仍保留在当前会话内存中。恢复时仍需核对并确认原正文和原幂等键。", "Unknown outcomes remain in this session if you leave the page. Recovery still requires reviewing and confirming the original body and key.") }}</p></div><span class="tag warn">{{ t("需人工确认", "Confirmation required") }}</span></div>
        <article v-for="intent in pendingIntents" :key="intent.key" class="pending-item"><b>{{ intentLabel(intent) }} · {{ intent.targetId ?? "" }}</b><code>{{ intent.key }}</code><p>{{ recoveryReason(intent) }}</p><p v-if="isConflict(intent)" class="error" role="alert">{{ t("409 冲突：原请求仍被冻结。重新读取后，先检查对应周期或发现记录，再丢弃并重新填写。", "409 conflict: the original request remains frozen. Reload and review the cycle or discovery record before discarding and entering a new request.") }}</p><div class="actions"><button v-if="isConflict(intent)" type="button" class="quiet" :disabled="writing || conflictReloading || !rights.view" @click="reloadConflictedIntent(intent)">{{ t("明确重新读取目标状态", "Explicitly reload target state") }}</button><button v-else type="button" class="primary" :disabled="writing || !(intent.operation === 'create' ? rights.run : rights.retry) || account.super_admin" @click="recover(intent)">{{ t("检查并恢复原请求", "Review and recover original request") }}</button><label v-if="isConflict(intent) && conflictReloaded(intent)" class="confirm-check"><input type="checkbox" :checked="conflictReviewed(intent)" @change="markConflictReviewed(intent, $event)" />{{ t("我已检查重新读取的最新状态，并决定丢弃旧请求。", "I reviewed the reloaded state and choose to discard the old request.") }}</label><button v-if="isConflict(intent) && conflictReloaded(intent)" type="button" class="quiet" :disabled="writing || !conflictReviewed(intent)" @click="discardIntent(intent)">{{ t("重新读取并审阅后，明确丢弃", "Discard after reload and review") }}</button></div></article>
      </section>

      <section class="panel create"><div class="section-title"><div><h2>{{ t("登记真实投注对应的周期", "Register a cycle for an actual bet") }}</h2><p>{{ t("仅指定已有注单作为锚点。服务端根据保存的投注快照与周期策略确定周期。", "Choose an existing bet as the anchor. The server resolves the cycle from saved bet snapshots and cycle policy.") }}</p></div><span class="tag">{{ rights.run && !account.super_admin ? t("可提交登记", "Registration available") : t("只读", "Read only") }}</span></div>
        <div class="form-grid"><label>{{ t("锚点注单 UUID", "Anchor bet order UUID") }}<input v-model="anchorOrderId" autocomplete="off" spellcheck="false" :aria-label="t('锚点注单 UUID', 'Anchor bet order UUID')" /></label><label>{{ t("登记原因", "Registration reason") }}<textarea v-model="createReason" maxlength="500" rows="2" :aria-label="t('登记原因', 'Registration reason')"></textarea><small>{{ createReasonBytes }} / 500 bytes</small></label></div>
        <button type="button" class="primary" :disabled="!canCreate" @click="beginReview(createIntent())">{{ t("检查并确认登记", "Review and confirm registration") }}</button><p v-if="!rights.run || account.super_admin" class="sub">{{ t("此账号不能登记周期。", "This account cannot register a cycle.") }}</p>
      </section>

      <div class="layout">
        <section class="panel list-panel"><div class="section-title"><div><h2>{{ t("佣金周期", "Commission cycles") }}</h2><p>{{ exact(cycleTotal) }} {{ t("条记录", "records") }}</p></div><span v-if="loading" class="tag">{{ t("读取中", "Loading") }}</span></div>
          <p v-if="listError" class="error" role="alert">{{ listError }}</p><div v-if="!loading && !listError && cycles.length === 0" class="empty">{{ t("暂无周期。周期来自符合条件的真实投注或明确登记。", "No cycles. Cycles come from qualifying real bets or explicit registration.") }}</div>
          <button v-for="item in cycles" :key="item.id" :data-cycle-id="item.id" type="button" class="row" :class="{ selected: selectedCycle?.id === item.id }" @click="loadCycle(item.id)"><span class="row-head"><b>{{ statusText(item.state) }}</b><i v-if="item.state === 'ready'" class="tag" :class="item.evidence_current ? 'good' : 'warn'">{{ item.evidence_current ? t("证据当前", "Evidence current") : t("证据已过期", "Evidence stale") }}</i></span><span class="sub id">{{ item.id }}</span><span class="sub">{{ date(item.window_from) }} – {{ date(item.window_to) }}</span><span class="sub">{{ t("版本", "Version") }} {{ item.version }} · {{ t("代次", "Run") }} {{ item.current_generation ?? t("无", "None") }}</span></button>
          <div class="pager"><button type="button" :disabled="loading || cycleOffset === 0" @click="pageCycles(-1)">{{ t("上一页", "Previous") }}</button><span>{{ Math.floor(cycleOffset / PAGE_SIZE) + 1 }} / {{ pageCount(cycleTotal) }}</span><button type="button" :disabled="loading || !cycleHasNext" @click="pageCycles(1)">{{ t("下一页", "Next") }}</button></div>
        </section>

        <section class="panel detail"><template v-if="selectedCycle || detailLoading || detailError"><div class="section-title"><div><h2>{{ t("周期详情", "Cycle details") }}</h2><p v-if="selectedCycle" class="id">{{ selectedCycle.id }}</p></div><span v-if="detailLoading" class="tag">{{ t("读取中", "Loading") }}</span></div><p v-if="detailError" class="error" role="alert">{{ detailError }}</p>
          <template v-if="selectedCycle"><div class="callout" :class="selectedCycle.evidence_current ? 'current' : 'stale'"><b>{{ selectedCycle.evidence_current ? t("证据当前", "Evidence current") : t("证据已过期", "Evidence stale") }}</b><span>{{ selectedCycle.state === 'ready' ? t("本代次核算已完成；本页不提供派发操作，派发状态未在此查询。", "Calculation completed for this generation; this page has no payout actions and payout state was not queried here.") : t("周期状态不代表资金授权。", "Cycle state does not authorize financial action.") }}</span></div>
            <dl class="facts"><dt>{{ t("品牌", "Brand") }}</dt><dd>{{ selectedCycle.brand_id }}</dd><dt>{{ t("窗口（UTC）", "Window (UTC)") }}</dt><dd>{{ date(selectedCycle.window_from) }} – {{ date(selectedCycle.window_to) }}</dd><dt>{{ t("锚点注单", "Anchor bet") }}</dt><dd class="id">{{ selectedCycle.anchor_order_id }}</dd><dt>{{ t("来源", "Created by") }}</dt><dd>{{ selectedCycle.creation_actor_type === "system" ? t("自动发现", "Discovery") : t("管理员", "Admin") }}</dd><dt>{{ t("扫描 / 计算 / 收入", "Scanned / calculated / earnings") }}</dt><dd>{{ exact(selectedCycle.target_count) }} / {{ exact(selectedCycle.calculated_count) }} / {{ exact(selectedCycle.earning_count) }}</dd><dt>{{ t("汇总积分", "Calculated points") }}</dt><dd>{{ exact(selectedCycle.total_points) }}</dd><dt>{{ t("原因 / 最近错误", "Reason / last error") }}</dt><dd>{{ selectedCycle.reason }} · {{ selectedCycle.last_error_code ?? t("无", "None") }}</dd><dt>{{ t("更新时间", "Updated") }}</dt><dd>{{ date(selectedCycle.updated_at) }}</dd></dl>
            <div v-if="selectedCycle.state === 'failed' && rights.retry && !account.super_admin" class="retry-box"><h3>{{ t("重试失败周期", "Retry failed cycle") }}</h3><label>{{ t("重试原因", "Retry reason") }}<textarea v-model="retryReason" :aria-label="t('重试原因', 'Retry reason')" maxlength="500" rows="2"></textarea></label><button type="button" class="primary" :disabled="!canRetryCycle" @click="beginReview(retryIntent('retry_cycle'))">{{ t("检查并确认重试", "Review and confirm retry") }}</button></div>
            <section class="subsection"><div class="section-title"><div><h3>{{ t("代理收入汇总", "Agent earnings") }}</h3><p>{{ t("核算值，不是账户余额或已派发资金。", "Calculated amounts, not account balances or paid funds.") }}</p></div><span v-if="earningsLoading" class="tag">{{ t("读取中", "Loading") }}</span></div><p v-if="earningsError" class="error" role="alert">{{ earningsError }}</p><div v-for="row in earnings?.items ?? []" :key="row.id" class="data-row"><b class="id">{{ row.agent_id }}</b><span>{{ t("会员", "Member") }} {{ row.member_id }}</span><span>{{ t("积分", "Points") }} {{ exact(row.points) }}</span><span>{{ t("精确金额", "Exact amount") }} {{ exact(row.exact_amount) }}</span></div><p v-if="earnings && !earnings.items.length" class="empty">{{ t("暂无代理收入。", "No agent earnings.") }}</p><div v-if="earnings" class="pager"><button type="button" :disabled="earningsLoading || earningOffset === 0" @click="pageEarnings(-1)">{{ t("上一页", "Previous") }}</button><span>{{ pageLabel(earningOffset, earnings.items.length) }} / {{ exact(earnings.total_count) }}</span><button type="button" :disabled="earningsLoading || BigInt(earningOffset + PAGE_SIZE) >= BigInt(earnings.total_count)" @click="pageEarnings(1)">{{ t("下一页", "Next") }}</button></div></section>
            <section class="subsection"><div class="section-title"><div><h3>{{ t("核算代次", "Calculation runs") }}</h3><p>{{ t("选择具体代次查看其保留轨迹。", "Select a specific generation to inspect its preserved calculations.") }}</p></div><span v-if="runsLoading" class="tag">{{ t("读取中", "Loading") }}</span></div><p v-if="runsError" class="error" role="alert">{{ runsError }}</p><button v-for="run in runs?.items ?? []" :key="run.id" :data-run-id="run.id" type="button" class="run-row" :class="{ selected: selectedRunId === run.id }" @click="selectRun(run.id)"><b>{{ statusText(run.state) }} · {{ t("代次", "Generation") }} {{ run.generation }}</b><span class="id">{{ run.id }}</span><span>{{ t("核算", "Calculated") }} {{ exact(run.calculated_count) }} · {{ t("收入", "Earnings") }} {{ exact(run.earning_count) }} · {{ t("积分", "Points") }} {{ exact(run.total_points) }}</span></button><div v-if="runs" class="pager"><button type="button" :disabled="runsLoading || runOffset === 0" @click="pageRuns(-1)">{{ t("上一页", "Previous") }}</button><span>{{ pageLabel(runOffset, runs.items.length) }} / {{ exact(runs.total_count) }}</span><button type="button" :disabled="runsLoading || BigInt(runOffset + PAGE_SIZE) >= BigInt(runs.total_count)" @click="pageRuns(1)">{{ t("下一页", "Next") }}</button></div>
              <div v-if="selectedRunId" class="calculations"><h4>{{ t("所选代次的计算轨迹", "Calculations for selected run") }} · {{ selectedRunId }}</h4><p v-if="calculationsError" class="error" role="alert">{{ calculationsError }}</p><span v-if="calculationsLoading" class="tag">{{ t("读取中", "Loading") }}</span><article v-for="item in calculations?.items ?? []" :key="item.id" class="calc-row"><b>{{ statusText(item.reason) }} · {{ statusText(item.status) }}</b><span class="id">{{ item.order_id }} · {{ item.member_id }}</span><span>{{ t("投注", "Stake") }} {{ exact(item.stake_points) }} · {{ t("奖金", "Prize") }} {{ exact(item.prize_points) }} · {{ t("基数", "Base") }} {{ exact(item.base_points) }}</span><small>{{ item.job_id ? `Job ${item.job_id}` : t("无结算任务", "No settlement job") }} · {{ item.calculation_id ?? t("无计算记录", "No calculation") }}</small></article><div v-if="calculations" class="pager"><button type="button" :disabled="calculationsLoading || calculationOffset === 0" @click="pageCalculations(-1)">{{ t("上一页", "Previous") }}</button><span>{{ pageLabel(calculationOffset, calculations.items.length) }} / {{ exact(calculations.total_count) }}</span><button type="button" :disabled="calculationsLoading || BigInt(calculationOffset + PAGE_SIZE) >= BigInt(calculations.total_count)" @click="pageCalculations(1)">{{ t("下一页", "Next") }}</button></div></div>
            </section>
          </template></template><div v-else class="empty detail-empty">{{ t("选择一个周期查看它的当前状态、收入汇总与明确代次的计算轨迹。", "Select a cycle to inspect its current state, earnings, and a specific run’s calculations.") }}</div></section>
      </div>

      <section class="panel discoveries"><div class="section-title"><div><h2>{{ t("投注发现队列", "Bet discovery queue") }}</h2><p>{{ t("队列自动发现真实启用快照的投注；登记成功链接到周期。", "The queue discovers bets with real enabled snapshots and links successful registrations to cycles.") }}</p></div><span v-if="discoveryLoading" class="tag">{{ t("读取中", "Loading") }}</span></div><p v-if="discoveryError" class="error" role="alert">{{ discoveryError }}</p><div v-if="!discoveryLoading && !discoveryError && !discoveries.length" class="empty">{{ t("暂无发现记录。", "No discovery records.") }}</div><article v-for="item in discoveries" :key="item.id" :data-discovery-id="item.id" class="discovery-row" :class="{ selected: selectedDiscovery?.id === item.id }"><button type="button" class="discovery-select" @click="selectDiscovery(item)"><span class="row-head"><b>{{ statusText(item.state) }}</b><span class="id">{{ item.id }}</span></span><span>{{ t("周期", "Cycle") }} <span v-if="item.cycle_id" class="id">{{ item.cycle_id }}</span><span v-else>{{ t("尚未登记", "Not registered") }}</span></span><span>{{ item.window_from ? `${date(item.window_from)} – ${date(item.window_to)}` : t("周期窗口尚未确定", "Cycle window not resolved") }}</span><span v-if="item.last_error_code" class="error-text">{{ t("最近错误", "Last error") }}: {{ item.last_error_code }}</span><span>{{ t("下次检查", "Next check") }} {{ date(item.next_check_at) }}</span></button><button v-if="item.state === 'failed' && rights.retry && !account.super_admin" type="button" class="quiet" @click="selectDiscovery(item); retryReason = ''">{{ t("选择以重试", "Select to retry") }}</button></article>
        <div class="pager"><button type="button" :disabled="discoveryLoading || discoveryOffset === 0" @click="pageDiscoveries(-1)">{{ t("上一页", "Previous") }}</button><span>{{ Math.floor(discoveryOffset / PAGE_SIZE) + 1 }} / {{ pageCount(discoveryTotal) }}</span><button type="button" :disabled="discoveryLoading || !discoveryHasNext" @click="pageDiscoveries(1)">{{ t("下一页", "Next") }}</button></div>
        <div v-if="selectedDiscovery" class="retry-box"><h3>{{ t("所选发现记录", "Selected discovery") }} · {{ selectedDiscovery.id }}</h3><p>{{ selectedDiscovery.last_error_code ?? t("无错误", "No error") }} · {{ t("下次检查", "Next check") }} {{ date(selectedDiscovery.next_check_at) }}</p><label>{{ t("重试原因", "Retry reason") }}<textarea v-model="retryReason" maxlength="500" rows="2"></textarea></label><button v-if="selectedDiscovery.state === 'failed' && rights.retry && !account.super_admin" type="button" class="primary" :disabled="!canRetryDiscovery" @click="beginReview(retryIntent('retry_discovery'))">{{ t("检查并确认重试发现记录", "Review and confirm discovery retry") }}</button></div>
      </section>
    </template>

      <div v-if="review" class="scrim" role="presentation" @click.self="review = null; confirmed = false"><section class="dialog panel" role="dialog" aria-modal="true" aria-labelledby="review-title"><h2 id="review-title">{{ t("确认佣金周期操作", "Confirm commission cycle operation") }}</h2><p>{{ t("核对冻结的请求正文和目标。确认后提交一次；结果未知时仅可用完全相同的正文和幂等键恢复。", "Review the frozen body and target. Confirmation submits once; if the outcome is unknown, recovery uses exactly the same body and idempotency key.") }}</p><dl class="facts"><dt>{{ t("操作", "Operation") }}</dt><dd>{{ intentLabel(review) }}</dd><dt>{{ t("目标", "Target") }}</dt><dd class="id">{{ review.targetId ?? (review.body as CommissionCreateBody).anchor_order_id }}</dd><dt>{{ t("原因", "Reason") }}</dt><dd>{{ review.body.reason }}</dd><dt v-if="'version' in review.body">{{ t("版本", "Version") }}</dt><dd v-if="'version' in review.body">v{{ review.body.version }}</dd><dt>{{ t("幂等键", "Idempotency key") }}</dt><dd class="id">{{ review.key }}</dd></dl><label class="confirm-check"><input v-model="confirmed" type="checkbox" />{{ t("我已核对目标、操作原因和冻结请求内容，确认提交。", "I have checked the target, reason, and frozen request and confirm submission.") }}</label><div class="actions"><button type="button" class="quiet" @click="review = null; confirmed = false">{{ t("返回", "Back") }}</button><button type="button" class="primary" :disabled="!confirmed || writing || (review.operation === 'create' ? !rights.run : !rights.retry) || account.super_admin" @click="confirmWrite">{{ t("确认并提交", "Confirm and submit") }}</button></div></section></div>
  </main>
</template>

<style scoped>
.cycles{width:100%;max-width:1540px;min-width:0;margin:auto;padding:clamp(12px,2.4vw,30px);color:#242a38;overflow-wrap:anywhere}.cycles *{box-sizing:border-box;min-width:0}.head,.section-title,.row-head,.pager,.actions{display:flex;align-items:center;justify-content:space-between;gap:10px}.head{align-items:flex-start;margin-bottom:18px}.head h1{font-size:clamp(22px,3vw,31px);margin:0 0 7px}.head p,.panel p{margin:5px 0 10px}.eyebrow{font-size:10px;letter-spacing:1.2px;color:#5868d9;font-weight:700}.panel{border:1px solid #e8eaf0;background:#fff;border-radius:12px;padding:clamp(13px,1.8vw,19px);margin-bottom:15px;box-shadow:0 2px 8px #1b285008}.panel h2{font-size:17px;margin:0}.panel h3{font-size:14px;margin:0}.panel h4{font-size:13px}.sub,.panel small{font-size:11px;color:#747c8d}.tag{display:inline-flex;border-radius:999px;padding:4px 9px;background:#eff1f6;color:#687083;font-size:10px;font-style:normal;white-space:nowrap}.tag.warn{background:#fff0d9;color:#8a5700}.tag.good{background:#e5f5ec;color:#23724e}.intro{background:#f7f8ff;border-color:#dfe3ff}.layout{display:grid;grid-template-columns:minmax(260px,.72fr) minmax(0,1.45fr);gap:15px;align-items:start}.list-panel,.detail{min-width:0}.row,.run-row{display:flex;flex-direction:column;align-items:stretch;text-align:left;width:100%;padding:12px 9px;border:0;border-top:1px solid #eceef3;background:transparent;gap:5px;color:inherit;cursor:pointer}.row.selected,.run-row.selected,.discovery-row.selected{background:#f4f5ff}.row-head{justify-content:flex-start;flex-wrap:wrap}.id{overflow-wrap:anywhere;word-break:break-word;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.94em}.pager{margin-top:12px;font-size:11px;color:#737b8a}.pager button,.head button,.quiet,.primary{border:1px solid #dfe2e9;border-radius:8px;background:white;color:inherit;padding:8px 11px;cursor:pointer}.primary{background:#5969dc;color:#fff;border-color:#5969dc;font-weight:650}.pager button:disabled,.primary:disabled,.quiet:disabled,.head button:disabled{opacity:.48;cursor:not-allowed}.empty{padding:14px;color:#777f8e;font-size:12px}.detail-empty{text-align:center;padding:38px 12px}.facts{display:grid;grid-template-columns:minmax(105px,.4fr) minmax(0,1fr);gap:8px 12px;font-size:11px}.facts dt{color:#737b8b}.facts dd{margin:0;overflow-wrap:anywhere}.callout{display:flex;flex-direction:column;gap:4px;border-radius:9px;padding:11px;margin:12px 0;font-size:12px}.callout.current{background:#e8f6ee;color:#1e6847}.callout.stale{background:#fff1dc;color:#815100}.subsection{border-top:1px solid #e9ebf0;padding-top:15px;margin-top:16px}.subsection .section-title{align-items:flex-start}.data-row,.calc-row,.discovery-row{display:flex;flex-direction:column;gap:5px;padding:11px 8px;border-top:1px solid #eceef3;font-size:11px}.run-row{border:1px solid #e7e9ef;border-radius:8px;margin:7px 0;font-size:11px}.calculations{background:#fafbfe;border-radius:9px;padding:12px;margin-top:12px}.calc-row{background:#fff;margin-top:7px;border:1px solid #edf0f4;border-radius:8px}.retry-box{margin-top:14px;padding:13px;border:1px solid #f1dfbe;background:#fffaf1;border-radius:9px}.retry-box label,.form-grid label{display:flex;flex-direction:column;gap:5px;font-size:12px;margin:9px 0}.retry-box textarea,.form-grid input,.form-grid textarea{width:100%;border:1px solid #dfe2e9;border-radius:7px;padding:9px;font:inherit;color:inherit;resize:vertical;background:#fff}.form-grid{display:grid;grid-template-columns:minmax(0,.85fr) minmax(0,1.15fr);gap:12px}.discovery-row{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;border:1px solid #e9ebf0;border-radius:9px;margin-top:8px;padding:9px;gap:7px 10px}.discovery-select{display:flex;flex-direction:column;align-items:stretch;gap:5px;text-align:left;border:0;background:transparent;color:inherit;font:inherit;grid-row:span 2;cursor:pointer}.error-text{color:#a13b32}.error{background:#fff0ee;color:#a3342c;border-radius:8px;padding:9px;font-size:12px}.notice{background:#e8f6ed;color:#236a49;border-radius:8px;padding:10px;font-size:12px}.pending{border-color:#eedbb8;background:#fffdf8}.pending-item{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:7px 12px;padding:12px 0;border-top:1px solid #eee4d3}.pending-item code{overflow-wrap:anywhere}.pending-item p,.pending-item .actions{grid-column:1/-1;margin:0;justify-content:flex-start;flex-wrap:wrap}.scrim{position:fixed;inset:0;z-index:50;background:#18203388;display:grid;place-items:center;padding:15px}.dialog{width:min(590px,100%);max-height:92vh;overflow:auto;margin:0}.confirm-check{display:flex;gap:8px;align-items:flex-start;font-size:12px;margin:16px 0}.confirm-check input{margin-top:2px;flex:0 0 auto}
@media(max-width:800px){.layout{grid-template-columns:minmax(0,1fr)}.form-grid{grid-template-columns:minmax(0,1fr)}.section-title{align-items:flex-start;flex-wrap:wrap}}
@media(max-width:420px){.head{gap:6px}.head button{padding:7px;font-size:11px}.panel{padding:12px}.facts{grid-template-columns:minmax(82px,.38fr) minmax(0,1fr);gap:7px}.discovery-row{grid-template-columns:minmax(0,1fr)}.discovery-select{grid-row:auto}.pager{gap:5px}.pager button{padding:7px;font-size:11px}.pending-item{grid-template-columns:minmax(0,1fr)}.pending-item .actions{grid-column:1}.actions{flex-wrap:wrap}}
</style>
