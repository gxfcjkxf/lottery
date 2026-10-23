<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import FilterAssociationSelect from "@lottery/shared/FilterAssociationSelect.vue";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import type { LocalizedMessage } from "@lottery/shared";
import { adminKey, useAdminI18n } from "./i18n";
import {
  commissionCyclePermissions, createCommissionCyclesApi,
  type CommissionCycleRecord, type CommissionCyclePage, type CommissionEarningPage, type CommissionRunEarningPage,
  type CommissionAllocationPage,
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
const { t, locale, message, display } = useAdminI18n();
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
const runEarnings = ref<CommissionRunEarningPage | null>(null), runEarningOffset = ref(0);
const allocations = ref<CommissionAllocationPage | null>(null), allocationOffset = ref(0);
const allocationAgentFilter = ref(""), allocationOrderFilter = ref("");
const discoveries = ref<CommissionDiscovery[]>([]), discoveryTotal = ref("0"), discoveryOffset = ref(0);
const selectedDiscovery = ref<CommissionDiscovery | null>(null);

const anchorOrderId = ref(""), createReason = ref(""), retryReason = ref("");
const review = ref<PendingCommissionCycleWrite | null>(null), confirmed = ref(false);
const pendingIntents = ref<PendingCommissionCycleWrite[]>([]);
const conflictKeys = ref<string[]>([]), reloadedConflictKeys = ref<string[]>([]), reviewedConflictKeys = ref<string[]>([]);
const loading = ref(false), detailLoading = ref(false), earningsLoading = ref(false), runsLoading = ref(false);
const calculationsLoading = ref(false), runEarningsLoading = ref(false), allocationsLoading = ref(false), discoveryLoading = ref(false), writing = ref(false);
const conflictReloading = ref(false);
const listError = ref<string | import("@lottery/shared").LocalizedMessage>(""), detailError = ref<string | import("@lottery/shared").LocalizedMessage>(""), earningsError = ref<string | import("@lottery/shared").LocalizedMessage>(""), runsError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const calculationsError = ref<string | import("@lottery/shared").LocalizedMessage>(""), runEarningsError = ref<string | import("@lottery/shared").LocalizedMessage>(""), allocationsError = ref<string | import("@lottery/shared").LocalizedMessage>(""), discoveryError = ref<string | import("@lottery/shared").LocalizedMessage>(""), writeError = ref<string | import("@lottery/shared").LocalizedMessage>(""), notice = ref<string | LocalizedMessage>("");
let alive = true, generation = 0, cycleTicket = 0, detailTicket = 0, earningTicket = 0, runTicket = 0;
let calculationTicket = 0, runEarningTicket = 0, allocationTicket = 0, discoveryTicket = 0, writeTicket = 0;
let conflictReloadTicket = 0;

const canCreate = computed(() => rights.value.run && !props.account.super_admin && !writing.value && !pendingIntents.value.some((item) => item.operation === "create") && validReason(createReason.value) && validUuid(anchorOrderId.value.trim()));
const canRetryCycle = computed(() => Boolean(rights.value.retry && !props.account.super_admin && selectedCycle.value?.state === "failed" && selectedCycle.value.version < Number.MAX_SAFE_INTEGER && validReason(retryReason.value) && !writing.value && !pendingFor("retry_cycle", selectedCycle.value.id)));
const canRetryDiscovery = computed(() => Boolean(rights.value.retry && !props.account.super_admin && selectedDiscovery.value?.state === "failed" && selectedDiscovery.value.version < Number.MAX_SAFE_INTEGER && validReason(retryReason.value) && !writing.value && !pendingFor("retry_discovery", selectedDiscovery.value.id)));
const createReasonBytes = computed(() => new TextEncoder().encode(createReason.value).length);
const cycleHasNext = computed(() => BigInt(cycleOffset.value + PAGE_SIZE) < BigInt(cycleTotal.value));
const discoveryHasNext = computed(() => BigInt(discoveryOffset.value + PAGE_SIZE) < BigInt(discoveryTotal.value));

function validUuid(value: string): boolean { return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value); }
function validReason(value: string): boolean {
  if (value.trim() !== value || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) { const next = value.charCodeAt(++i); if (!(next >= 0xdc00 && next <= 0xdfff)) return false; }
    else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}
function statusText(value: string): string {
  const labels: Record<string, string> = {
    enumerating: 'admin.copy.ff7dfb62', waiting: 'admin.copy.1d137025', calculating: 'admin.copy.a5bd9ef8',
    summarizing: 'admin.copy.dd284fd4', ready: 'admin.copy.b66cb80d', failed: 'admin.copy.1feabec0',
    pending: 'admin.copy.99f247fd', registered: 'admin.copy.96d29932', abandoned: 'admin.copy.0d07ce4c',
    won: 'admin.copy.638b1e06', lost: 'admin.copy.e2f3b9bc', abnormal: 'admin.copy.fd212679', bet_cancelled: 'admin.copy.c320e9ec', judged_cancelled: 'admin.copy.c8b7c1fe',
    loss: 'admin.copy.a13e79ef', turnover: 'admin.copy.89fd862d',
    eligible: 'admin.copy.3ced3db2', policy_disabled: 'admin.copy.217c96f8', unattributed: 'admin.copy.012590cd',
    other_cycle: 'admin.copy.937d4835', cancelled: 'admin.copy.a454aa98',
  };
 return labels[value] ? t(adminKey(labels[value]!)) : value;
}
function date(value: string | null | undefined): string{ return formatDisplayDateTime(value, displayLocale.value); }
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
  runEarnings.value = null; allocations.value = null; allocationAgentFilter.value = allocationOrderFilter.value = "";
  discoveries.value = []; discoveryTotal.value = "0"; discoveryOffset.value = 0; selectedDiscovery.value = null;
  anchorOrderId.value = ""; createReason.value = ""; retryReason.value = ""; review.value = null; confirmed.value = false;
  listError.value = detailError.value = earningsError.value = runsError.value = calculationsError.value = runEarningsError.value = allocationsError.value = discoveryError.value = writeError.value = notice.value = "";
  loading.value = detailLoading.value = earningsLoading.value = runsLoading.value = calculationsLoading.value = runEarningsLoading.value = allocationsLoading.value = discoveryLoading.value = writing.value = false;
  refreshPending();
}
function resetScope() {
  conflictReloadTicket++; conflictReloading.value = false;
  reloadedConflictKeys.value = []; reviewedConflictKeys.value = [];
  generation++; cycleTicket++; detailTicket++; earningTicket++; runTicket++; calculationTicket++; runEarningTicket++; allocationTicket++; discoveryTicket++; writeTicket++;
  clearVisible();
  if (rights.value.view) { void loadCycles(0); void loadDiscoveries(0); }
}
function sessionExpired(cause: unknown, captured: string, session: number) {
  if (cause instanceof AdminApiError && cause.status === 401 && alive && captured === scopeStamp.value && session === commissionCycleSessionGeneration()) {
    generation++; cycleTicket++; detailTicket++; earningTicket++; runTicket++; calculationTicket++; runEarningTicket++; allocationTicket++; discoveryTicket++; writeTicket++;
    clearVisible();
    emit("session-invalid");
    return true;
  }
  return false;
}
function readFailure(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 0) return t('admin.copy.ce90986e');
  return cause instanceof Error ? publicErrorMessage(cause) : t('admin.copy.466e55a6');
}
function writeFailure(cause: unknown) { return cause instanceof Error ? publicErrorMessage(cause) : t('admin.copy.f39a8107'); }

async function refreshAll() {
  await Promise.all([loadCycles(cycleOffset.value), loadDiscoveries(discoveryOffset.value)]);
}
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
  earningTicket++; runTicket++; calculationTicket++; runEarningTicket++; allocationTicket++;
  earnings.value = null; runs.value = null; calculations.value = null; selectedRunId.value = "";
  runEarnings.value = null; allocations.value = null; allocationAgentFilter.value = allocationOrderFilter.value = "";
  earningsError.value = runsError.value = calculationsError.value = runEarningsError.value = allocationsError.value = ""; refreshPending();
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
      earningsError.value = t('admin.copy.f79386fa');
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
    if (page.cycle_id !== cycleId || page.items.some((row) => row.cycle_id !== cycleId)) throw new Error(t('admin.copy.4d720ae2'));
    runs.value = page; runOffset.value = page.offset;
    const selected = page.items.find((run) => run.id === selectedRunId.value) ?? page.items[0];
    if (selected) selectRun(selected.id);
  } catch (cause) { if (current(ticket, runTicket, captured, session) && selectedCycle.value?.id === cycleId) { sessionExpired(cause, captured, session); runsError.value = readFailure(cause); } }
  finally { if (current(ticket, runTicket, captured, session)) runsLoading.value = false; }
}
async function selectRun(runId: string) {
  if (!selectedCycle.value || !runs.value?.items.some((item) => item.id === runId)) return;
  selectedRunId.value = runId; calculationOffset.value = runEarningOffset.value = allocationOffset.value = 0;
  allocationAgentFilter.value = allocationOrderFilter.value = "";
  calculations.value = null; runEarnings.value = null; allocations.value = null;
  calculationsError.value = runEarningsError.value = allocationsError.value = "";
  await Promise.all([loadCalculations(selectedCycle.value.id, runId, 0), loadRunEarnings(selectedCycle.value.id, runId, 0), loadAllocations(selectedCycle.value.id, runId, 0)]);
}

async function loadRunEarnings(cycleId: string, runId: string, offset = runEarningOffset.value) {
  const ticket = ++runEarningTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  runEarningsLoading.value = true; runEarningsError.value = ""; runEarnings.value = null;
  try {
    const page = await api.runEarnings(props.brandId, cycleId, runId, PAGE_SIZE, offset);
    if (!current(ticket, runEarningTicket, captured, session) || selectedCycle.value?.id !== cycleId || selectedRunId.value !== runId) return;
    if (page.cycle_id !== cycleId || page.run_id !== runId || page.items.some((row) => row.cycle_id !== cycleId || row.run_id !== runId)) throw new Error(t('admin.copy.1718d6a6'));
    runEarnings.value = page; runEarningOffset.value = page.offset;
  } catch (cause) { if (current(ticket, runEarningTicket, captured, session) && selectedCycle.value?.id === cycleId && selectedRunId.value === runId) { sessionExpired(cause, captured, session); runEarningsError.value = readFailure(cause); } }
  finally { if (current(ticket, runEarningTicket, captured, session)) runEarningsLoading.value = false; }
}

async function loadAllocations(cycleId: string, runId: string, offset = allocationOffset.value) {
  const ticket = ++allocationTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  allocationsLoading.value = true; allocationsError.value = ""; allocations.value = null;
  try {
    const agentId = allocationAgentFilter.value.trim(), orderId = allocationOrderFilter.value.trim();
    const page = await api.allocations(props.brandId, cycleId, runId, { limit: PAGE_SIZE, offset, ...(agentId ? { agent_id: agentId } : {}), ...(orderId ? { order_id: orderId } : {}) });
    if (!current(ticket, allocationTicket, captured, session) || selectedCycle.value?.id !== cycleId || selectedRunId.value !== runId) return;
    if (page.cycle_id !== cycleId || page.run_id !== runId || page.items.some((row) => row.cycle_id !== cycleId || row.run_id !== runId)) throw new Error(t('admin.copy.c1622f82'));
    allocations.value = page; allocationOffset.value = page.offset;
  } catch (cause) { if (current(ticket, allocationTicket, captured, session) && selectedCycle.value?.id === cycleId && selectedRunId.value === runId) { sessionExpired(cause, captured, session); allocationsError.value = readFailure(cause); } }
  finally { if (current(ticket, allocationTicket, captured, session)) allocationsLoading.value = false; }
}

async function filterAllocations() {
  if (!selectedCycle.value || !selectedRunId.value) return;
  for (const [value, label] of [[allocationAgentFilter.value.trim(), t('admin.copy.b863c1ed')], [allocationOrderFilter.value.trim(), t('admin.copy.f292714a')]] as const) {
    if (value && !validUuid(value)) { allocationTicket++; allocations.value = null; allocationsError.value = t('admin.commission.invalidUuid', { field: label }); return; }
  }
  allocationOffset.value = 0; await loadAllocations(selectedCycle.value.id, selectedRunId.value, 0);
}
function allocationFilterChanged() {
  allocationTicket++; allocations.value = null; allocationsError.value = ""; allocationsLoading.value = false;
}
async function loadCalculations(cycleId: string, runId: string, offset = calculationOffset.value) {
  const ticket = ++calculationTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  calculationsLoading.value = true; calculationsError.value = ""; calculations.value = null;
  try {
    const page: CommissionCalculationPage = await api.calculations(props.brandId, cycleId, runId, PAGE_SIZE, offset);
    if (!current(ticket, calculationTicket, captured, session) || selectedCycle.value?.id !== cycleId || selectedRunId.value !== runId) return;
    if (page.cycle_id !== cycleId || page.run_id !== runId || page.items.some((row) => row.cycle_id !== cycleId || row.run_id !== runId)) throw new Error(t('admin.copy.69608838'));
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
    notice.value = message('admin.copy.3589a5eb');
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
      writeError.value = t('admin.copy.53a96ffc');
    } else if (outcome === "conflict") {
      setPendingCommissionCycleWrite(scope, intent); refreshPending();
      knownConflictKeys.add(intent.key);
      if (!conflictKeys.value.includes(intent.key)) conflictKeys.value = [...conflictKeys.value, intent.key];
      reloadedConflictKeys.value = reloadedConflictKeys.value.filter((key) => key !== intent.key);
      reviewedConflictKeys.value = reviewedConflictKeys.value.filter((key) => key !== intent.key);
      writeError.value = t('admin.copy.914a16d2');
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
      earningTicket++; runTicket++; calculationTicket++; runEarningTicket++; allocationTicket++;
      earnings.value = null; runs.value = null; calculations.value = null; selectedRunId.value = "";
      runEarnings.value = null; allocations.value = null; allocationAgentFilter.value = allocationOrderFilter.value = "";
      void loadEarnings(item.id, 0); void loadRuns(item.id, 0);
    } else {
      const latest = await api.discoveries(props.brandId, PAGE_SIZE, discoveryOffset.value);
      if (!stillCurrent()) return;
      const found = latest.items.find((item) => item.id === intent.targetId);
      discoveries.value = latest.items; discoveryTotal.value = latest.total_count; discoveryOffset.value = latest.offset;
      if (!found) throw new Error(t('admin.copy.ba60f5a5'));
      selectedDiscovery.value = found;
    }
    if (stillCurrent()) {
      reloadedConflictKeys.value = [...new Set([...reloadedConflictKeys.value, intent.key])];
      writeError.value = t('admin.copy.300aa93a');
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
async function confirmWrite() { if (review.value && confirmed.value && !writing.value) await submit(review.value); }
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
async function pageCycles(delta: number) { if (!loading.value) await loadCycles(Math.max(0, cycleOffset.value + delta * PAGE_SIZE)); }
async function pageDiscoveries(delta: number) { if (!discoveryLoading.value) await loadDiscoveries(Math.max(0, discoveryOffset.value + delta * PAGE_SIZE)); }
async function pageEarnings(delta: number) { if (selectedCycle.value && earnings.value) await loadEarnings(selectedCycle.value.id, Math.max(0, earningOffset.value + delta * PAGE_SIZE)); }
async function pageRuns(delta: number) { if (selectedCycle.value) await loadRuns(selectedCycle.value.id, Math.max(0, runOffset.value + delta * PAGE_SIZE)); }
async function pageCalculations(delta: number) { if (selectedCycle.value && selectedRunId.value) await loadCalculations(selectedCycle.value.id, selectedRunId.value, Math.max(0, calculationOffset.value + delta * PAGE_SIZE)); }
async function pageRunEarnings(delta: number) { if (selectedCycle.value && selectedRunId.value) await loadRunEarnings(selectedCycle.value.id, selectedRunId.value, Math.max(0, runEarningOffset.value + delta * PAGE_SIZE)); }
async function pageAllocations(delta: number) { if (selectedCycle.value && selectedRunId.value) await loadAllocations(selectedCycle.value.id, selectedRunId.value, Math.max(0, allocationOffset.value + delta * PAGE_SIZE)); }
function recoveryReason(intent: PendingCommissionCycleWrite) { return intent.body.reason; }
function intentLabel(intent: PendingCommissionCycleWrite) { return intent.operation === "create" ? t('admin.copy.379b72cb') : intent.operation === "retry_cycle" ? t('admin.copy.8efee3ae') : t('admin.copy.a33c324d'); }
function operationText(operation: CommissionWriteScope["operation"]) { return operation === "create" ? t('admin.copy.379b72cb') : operation === "retry_cycle" ? t('admin.copy.8efee3ae') : t('admin.copy.a33c324d'); }
function receiptState() { return receipt.value ? receipt.value.record.state : ""; }
function receiptAuditId() {
  const record = receipt.value?.record;
  if (!record) return t('admin.copy.c2b41b33');
  return "creation_audit_log_id" in record ? record.creation_audit_log_id : record.last_audit_log_id ?? t('admin.copy.c2b41b33');
}

watch(scopeStamp, resetScope, { immediate: true });
onUnmounted(() => { alive = false; generation++; cycleTicket++; detailTicket++; earningTicket++; runTicket++; calculationTicket++; discoveryTicket++; writeTicket++; });
function closeDetails() {
  detailTicket++; selectedCycle.value = null; detailLoading.value = false; detailError.value = '';
}
</script>

<template>
  <main class="cycles commission-cycles" aria-labelledby="cycles-title">
    <header class="head"><div><h1 id="cycles-title">{{ t('admin.copy.a0194752') }}</h1></div><button v-refresh-button="t('admin.copy.894e2a7d')" type="button" :disabled="loading || discoveryLoading || !rights.view" @click="withGlobalLoading(refreshAll)">{{ t('admin.copy.894e2a7d') }}</button></header>
    <p v-if="notice" class="notice" role="status">{{ display(notice) }}</p><p v-if="writeError" class="error" role="alert">{{ displayPublicError(writeError) }}</p>
    <section v-if="!rights.view" class="panel" role="status">{{ t('admin.copy.8edc5180') }}</section>
    <template v-else>
      <section class="panel intro"><h2>{{ t('admin.copy.91e6e77e') }}</h2></section>

      <section v-if="receipt" class="panel commission-receipt" role="status" aria-live="polite"><div class="section-title"><div><h2>{{ t('admin.copy.1b3685a6') }}</h2></div><span class="tag good">{{ t('admin.copy.5f313d26') }}</span></div><dl class="facts"><dt>{{ t('admin.copy.015371dd') }}</dt><dd>{{ operationText(receipt.operation) }}</dd><dt>{{ t('admin.copy.788448f7') }}</dt><dd class="id">{{ receipt.record.id }}</dd><dt>{{ t('admin.copy.e59b5282') }}</dt><dd>{{ statusText(receiptState()) }}</dd></dl></section>

      <section v-if="pendingIntents.length" class="panel pending" aria-live="polite">
        <div class="section-title"><div><h2>{{ t('admin.copy.eef84c8f') }}</h2><p>{{ t('admin.copy.56be4d08') }}</p></div><span class="tag warn">{{ t('admin.copy.020ec9b6') }}</span></div>
        <article v-for="intent in pendingIntents" :key="intent.key" class="pending-item"><b>{{ intentLabel(intent) }} · {{ intent.targetId ?? "" }}</b><code></code><p>{{ recoveryReason(intent) }}</p><p v-if="isConflict(intent)" class="error" role="alert">{{ t('admin.copy.2827a14b') }}</p><div class="actions"><button v-refresh-button="t('admin.copy.586a2411')" v-if="isConflict(intent)" type="button" class="button button-quiet" :disabled="writing || conflictReloading || !rights.view" @click="withGlobalLoading(reloadConflictedIntent, intent)">{{ t('admin.copy.586a2411') }}</button><button v-else type="button" class="button button-primary" :disabled="writing || !(intent.operation === 'create' ? rights.run : rights.retry) || account.super_admin" @click="recover(intent)">{{ t('admin.copy.cb6f5b32') }}</button><label v-if="isConflict(intent) && conflictReloaded(intent)" class="confirm-check"><input type="checkbox" :checked="conflictReviewed(intent)" @change="markConflictReviewed(intent, $event)" />{{ t('admin.copy.2a662c1c') }}</label><button v-if="isConflict(intent) && conflictReloaded(intent)" type="button" class="button button-quiet" :disabled="writing || !conflictReviewed(intent)" @click="discardIntent(intent)">{{ t('admin.copy.095e078e') }}</button></div></article>
      </section>

      <section class="panel create"><div class="section-title"><div><h2>{{ t('admin.copy.57db9093') }}</h2></div><span class="tag">{{ rights.run && !account.super_admin ? t('admin.copy.ecb11581') : t('admin.copy.18dca8cc') }}</span></div>
        <div class="form-grid"><label>{{ t('admin.copy.10812c43') }}<input v-model="anchorOrderId" autocomplete="off" spellcheck="false" :aria-label="t('admin.copy.10812c43')" /></label></div>
        <button type="button" class="button button-primary" :disabled="!canCreate" @click="beginReview(createIntent())">{{ t('admin.copy.2e889bb3') }}</button><p v-if="!rights.run || account.super_admin" class="sub">{{ t('admin.copy.e5d76776') }}</p>
      </section>

      <div class="layout">
        <section class="panel list-panel"><div class="section-title"><div><h2>{{ t('admin.copy.c073de34') }}</h2></div><span v-global-loading v-if="loading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div>
          <p v-if="listError" class="error" role="alert">{{ displayPublicError(listError) }}</p><div v-if="!loading && !listError && cycles.length === 0" class="empty empty-state">{{ t('admin.copy.16181fd2') }}</div>
          <button v-for="item in cycles" :key="item.id" :data-cycle-id="item.id" type="button" class="row" :class="{ selected: selectedCycle?.id === item.id }" @click="withGlobalLoading(loadCycle, item.id)"><span class="row-head"><b>{{ statusText(item.state) }}</b><i v-if="item.state === 'ready'" class="tag" :class="item.evidence_current ? 'good' : 'warn'">{{ item.evidence_current ? t('admin.copy.db51c9e6') : t('admin.copy.b72ae766') }}</i></span><span class="sub id">{{ item.id }}</span><span class="sub">{{ date(item.window_from) }} – {{ date(item.window_to) }}</span><span class="sub"> {{ t('admin.copy.ce5a9aec') }} {{ item.current_generation ?? t('admin.copy.c2b41b33') }}</span></button>
          <div class="pager"><span>{{ exact(cycleTotal) }} {{ t('admin.copy.d9009591') }}</span><button type="button" :disabled="loading || cycleOffset === 0" @click="withGlobalLoading(pageCycles, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ Math.floor(cycleOffset / PAGE_SIZE) + 1 }} / {{ pageCount(cycleTotal) }}</span><button type="button" :disabled="loading || !cycleHasNext" @click="withGlobalLoading(pageCycles, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
        </section>

        <DetailModal v-if="!!selectedCycle || detailLoading || !!detailError" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" :open="!review" @close="closeDetails">
<section class="panel detail"><template v-if="selectedCycle || detailLoading || detailError"><div class="section-title"><div><h2>{{ t('admin.copy.c3995df5') }}</h2><p v-if="selectedCycle" class="id">{{ selectedCycle.id }}</p></div><span v-global-loading v-if="detailLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div><p v-if="detailError" class="error" role="alert">{{ displayPublicError(detailError) }}</p>
          <template v-if="selectedCycle"><div class="callout" :class="selectedCycle.evidence_current ? 'current' : 'stale'"><b>{{ selectedCycle.evidence_current ? t('admin.copy.db51c9e6') : t('admin.copy.b72ae766') }}</b><span v-if="selectedCycle.state === 'ready'">{{ t('admin.copy.24fec27b') }}</span></div>
            <dl class="facts"><dt>{{ t('admin.copy.9c32661d') }}</dt><dd><AssociationName :record="selectedCycle" kind="brand" :id="selectedCycle.brand_id" :locale="associationLocale" /></dd><dt>{{ t('admin.copy.99f8690d') }}</dt><dd>{{ date(selectedCycle.window_from) }} – {{ date(selectedCycle.window_to) }}</dd><dt>{{ t('admin.copy.4459ddf7') }}</dt><dd class="id">{{ selectedCycle.anchor_order_id }}</dd><dt>{{ t('admin.copy.1534357d') }}</dt><dd>{{ selectedCycle.creation_actor_type === "system" ? t('admin.copy.e1e295dc') : t('admin.copy.37b47e95') }}</dd><dt>{{ t('admin.copy.b14f15bd') }}</dt><dd>{{ exact(selectedCycle.target_count) }} / {{ exact(selectedCycle.calculated_count) }} / {{ exact(selectedCycle.earning_count) }}</dd><dt>{{ t('admin.copy.68ab2005') }}</dt><dd>{{ exact(selectedCycle.total_points) }}</dd><dt>{{ t('admin.copy.ceae8b8b') }}</dt><dd>{{ selectedCycle.reason }} · {{ selectedCycle.last_error_code ? displayPublicError(publicErrorMessage({ code: selectedCycle.last_error_code })) : t('admin.copy.c2b41b33') }}</dd><dt>{{ t('admin.copy.19bf47ac') }}</dt><dd>{{ date(selectedCycle.updated_at) }}</dd></dl>
            <div v-if="selectedCycle.state === 'failed' && rights.retry && !account.super_admin" class="retry-box"><h3>{{ t('admin.copy.6571d2d9') }}</h3><button type="button" class="button button-primary" :disabled="!canRetryCycle" @click="beginReview(retryIntent('retry_cycle'))">{{ t('admin.copy.a6f031a3') }}</button></div>
            <section class="subsection"><div class="section-title"><div><h3>{{ t('admin.copy.027b4a69') }}</h3></div><span v-global-loading v-if="earningsLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div><p v-if="earningsError" class="error" role="alert">{{ displayPublicError(earningsError) }}</p><div v-for="row in earnings?.items ?? []" :key="row.id" class="data-row"><b class="id"><AssociationName :record="row" kind="agent" :id="row.agent_id" :locale="associationLocale" /></b><span>{{ t('admin.copy.56b2d19b') }} <AssociationName :record="row" kind="member" :id="row.member_id" :locale="associationLocale" /></span><span>{{ t('admin.copy.54e851cb') }} {{ exact(row.points) }}</span><span>{{ t('admin.copy.d7344f9c') }} {{ exact(row.exact_amount) }}</span></div><p v-if="earnings && !earnings.items.length" class="empty empty-state">{{ t('admin.copy.93e436d4') }}</p><div v-if="earnings" class="pager"><button type="button" :disabled="earningsLoading || earningOffset === 0" @click="withGlobalLoading(pageEarnings, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ pageLabel(earningOffset, earnings.items.length) }} / {{ exact(earnings.total_count) }}</span><button type="button" :disabled="earningsLoading || BigInt(earningOffset + PAGE_SIZE) >= BigInt(earnings.total_count)" @click="withGlobalLoading(pageEarnings, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div></section>
            <section class="subsection"><div class="section-title"><div><h3>{{ t('admin.copy.c29e7175') }}</h3></div><span v-global-loading v-if="runsLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div><p v-if="runsError" class="error" role="alert">{{ displayPublicError(runsError) }}</p><button v-for="run in runs?.items ?? []" :key="run.id" :data-run-id="run.id" type="button" class="run-row" :class="{ selected: selectedRunId === run.id }" @click="withGlobalLoading(selectRun, run.id)"><b>{{ statusText(run.state) }} · {{ t('admin.copy.c8648ca9') }} {{ run.generation }}</b><span class="id">{{ run.id }}</span><span>{{ t('admin.copy.57546450') }} {{ exact(run.calculated_count) }} · {{ t('admin.copy.043f32b3') }} {{ exact(run.earning_count) }} · {{ t('admin.copy.ec082fa7') }} {{ exact(run.total_points) }}</span></button><div v-if="runs" class="pager"><button type="button" :disabled="runsLoading || runOffset === 0" @click="withGlobalLoading(pageRuns, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ pageLabel(runOffset, runs.items.length) }} / {{ exact(runs.total_count) }}</span><button type="button" :disabled="runsLoading || BigInt(runOffset + PAGE_SIZE) >= BigInt(runs.total_count)" @click="withGlobalLoading(pageRuns, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
              <div v-if="selectedRunId" class="calculations"><h4>{{ t('admin.copy.685b8e96') }} · {{ selectedRunId }}</h4><p v-if="calculationsError" class="error" role="alert">{{ displayPublicError(calculationsError) }}</p><span v-global-loading v-if="calculationsLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span><article v-for="item in calculations?.items ?? []" :key="item.id" class="calc-row"><b>{{ statusText(item.reason) }} · {{ statusText(item.status) }}</b><span class="id">{{ item.order_id }} · <AssociationName :record="item" kind="member" :id="item.member_id" :locale="associationLocale" /></span><span>{{ t('admin.copy.96730052') }} {{ exact(item.stake_points) }} · {{ t('admin.copy.31452fe8') }} {{ exact(item.prize_points) }} · {{ t('admin.copy.ef515018') }} {{ exact(item.base_points) }}</span><small>{{ item.job_id ? `Job ${item.job_id}` : t('admin.copy.e97f74ac') }} · {{ item.calculation_id ?? t('admin.copy.906cccce') }}</small></article><div v-if="calculations" class="pager"><button type="button" :disabled="calculationsLoading || calculationOffset === 0" @click="withGlobalLoading(pageCalculations, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ pageLabel(calculationOffset, calculations.items.length) }} / {{ exact(calculations.total_count) }}</span><button type="button" :disabled="calculationsLoading || BigInt(calculationOffset + PAGE_SIZE) >= BigInt(calculations.total_count)" @click="withGlobalLoading(pageCalculations, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
                <section class="subsection historical"><h4>{{ t('admin.copy.666c29a4') }}</h4><p class="sub">{{ t('admin.copy.7639c727') }}</p>
                  <div class="section-title"><h4>{{ t('admin.copy.e6563a7a') }}</h4><span v-global-loading v-if="runEarningsLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div><p v-if="runEarningsError" class="error" role="alert">{{ displayPublicError(runEarningsError) }}</p><article v-for="row in runEarnings?.items ?? []" :key="row.id" class="run-earning-row"><b class="id"><AssociationName :record="row" kind="agent" :id="row.agent_id" :locale="associationLocale" /></b><span>{{ t('admin.copy.56b2d19b') }} <AssociationName :record="row" kind="member" :id="row.member_id" :locale="associationLocale" /> {{ t('admin.copy.54e851cb') }} {{ exact(row.points) }}</span><span>{{ t('admin.copy.d7344f9c') }} {{ exact(row.exact_amount) }}</span></article><p v-if="runEarnings && !runEarnings.items.length" class="empty empty-state">{{ t('admin.copy.a2f78367') }}</p><div v-if="runEarnings" class="pager"><button type="button" :disabled="runEarningsLoading || runEarningOffset === 0" @click="withGlobalLoading(pageRunEarnings, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ pageLabel(runEarningOffset, runEarnings.items.length) }} / {{ exact(runEarnings.total_count) }}</span><button type="button" :disabled="runEarningsLoading || BigInt(runEarningOffset + PAGE_SIZE) >= BigInt(runEarnings.total_count)" @click="withGlobalLoading(pageRunEarnings, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
                  <div class="section-title allocation-heading"><h4>{{ t('admin.copy.d4543586') }}</h4><span v-global-loading v-if="allocationsLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div>
                  <div class="allocation-filters">
                    <label>{{ t('filter.agent') }}<FilterAssociationSelect v-model="allocationAgentFilter" :brand-id="brandId" surface="admin" resource="commission" kind="agent" :label="t('filter.agent')" :locale="displayLocale" :scope-key="JSON.stringify(account)" :enabled="rights.view" @update:model-value="allocationFilterChanged" /></label>
                    <label>{{ t('filter.bet_order') }}<FilterAssociationSelect v-model="allocationOrderFilter" :brand-id="brandId" surface="admin" resource="commission" kind="bet_order" :label="t('filter.bet_order')" :locale="displayLocale" :scope-key="JSON.stringify(account)" :enabled="rights.view" @update:model-value="allocationFilterChanged" /></label>
                    <button type="button" class="button button-quiet" :disabled="allocationsLoading" @click="withGlobalLoading(filterAllocations)">{{ t('admin.copy.b8972a75') }}</button>
                  </div>
                  <p v-if="allocationsError" class="error" role="alert">{{ displayPublicError(allocationsError) }}</p>
                  <article v-for="(row, index) in allocations?.items ?? []" :key="`${row.order_id}:${row.agent_id}:${index}`" class="allocation-row">
                    <b>{{ t('admin.copy.f292714a') }} <span class="id">{{ row.order_id }}</span> {{ t('admin.copy.b863c1ed') }} <span class="id"><AssociationName :record="row" kind="agent" :id="row.agent_id" :locale="associationLocale" /></span></b>
                    <span>{{ t('admin.copy.3298568f') }} <AssociationName :record="row" kind="member" :id="row.member_id" :locale="associationLocale" /> {{ t('admin.copy.4c082700') }} <AssociationName :record="row" kind="member" :id="row.bettor_member_id" :locale="associationLocale" /></span>
                    <span>{{ t('admin.copy.ef515018') }} {{ exact(row.base_points) }} · {{ statusText(row.mode) }} · {{ t('admin.copy.7beeeccb') }} {{ row.agent_ratio }} · {{ t('admin.copy.07830cda') }} {{ row.downstream_ratio }} · {{ t('admin.copy.26964f6f') }} {{ row.difference_ratio }}</span>
                    <span>{{ t('admin.copy.ca243f72') }} {{ exact(row.exact_amount) }}</span>
                    <small>{{ t('admin.copy.f5703e2b') }} {{ row.calculation_id }} · {{ date(row.created_at) }}</small>
                  </article>
                  <p v-if="allocations && !allocations.items.length" class="empty empty-state">{{ t('admin.copy.ae2a5503') }}</p>
                  <div v-if="allocations" class="pager"><button type="button" :disabled="allocationsLoading || allocationOffset === 0" @click="withGlobalLoading(pageAllocations, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ pageLabel(allocationOffset, allocations.items.length) }} / {{ exact(allocations.total_count) }}</span><button type="button" :disabled="allocationsLoading || BigInt(allocationOffset + PAGE_SIZE) >= BigInt(allocations.total_count)" @click="withGlobalLoading(pageAllocations, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
                </section>
              </div>
            </section>
          </template></template><div v-else class="empty detail-empty empty-state">{{ t('admin.copy.4e572ef3') }}</div></section>
</DetailModal>
      </div>

      <section class="panel discoveries"><div class="section-title"><div><h2>{{ t('admin.copy.df3d1c5d') }}</h2></div><span v-global-loading v-if="discoveryLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div><p v-if="discoveryError" class="error" role="alert">{{ displayPublicError(discoveryError) }}</p><div v-if="!discoveryLoading && !discoveryError && !discoveries.length" class="empty empty-state">{{ t('admin.copy.6f161c9e') }}</div><article v-for="item in discoveries" :key="item.id" :data-discovery-id="item.id" class="discovery-row" :class="{ selected: selectedDiscovery?.id === item.id }"><button type="button" class="discovery-select" @click="selectDiscovery(item)"><span class="row-head"><b>{{ statusText(item.state) }}</b><span class="id">{{ item.id }}</span></span><span>{{ t('admin.copy.11c28a36') }} <span v-if="item.cycle_id" class="id"><AssociationName :record="item" kind="cycle" :id="item.cycle_id" :locale="associationLocale" /></span><span v-else>{{ t('admin.copy.941137c1') }}</span></span><span>{{ item.window_from ? `${date(item.window_from)} – ${date(item.window_to)}` : t('admin.copy.6a0bb6fb') }}</span><span v-if="item.last_error_code" class="error-text">{{ t('admin.copy.c0a94db8') }}: {{ item.last_error_code ? displayPublicError(publicErrorMessage({ code: item.last_error_code })) : "—" }}</span><span>{{ t('admin.copy.c4397333') }} {{ date(item.next_check_at) }}</span></button><button v-if="item.state === 'failed' && rights.retry && !account.super_admin" type="button" class="button button-quiet" @click="selectDiscovery(item); retryReason = ''">{{ t('admin.copy.e57b7cb7') }}</button></article>
        <div class="pager"><button type="button" :disabled="discoveryLoading || discoveryOffset === 0" @click="withGlobalLoading(pageDiscoveries, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><span>{{ Math.floor(discoveryOffset / PAGE_SIZE) + 1 }} / {{ pageCount(discoveryTotal) }}</span><button type="button" :disabled="discoveryLoading || !discoveryHasNext" @click="withGlobalLoading(pageDiscoveries, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div>
        <DetailModal v-if="!!selectedDiscovery" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" :open="!review" @close="selectedDiscovery = null">
<div v-if="selectedDiscovery" class="retry-box"><h3>{{ t('admin.copy.c1fe5e47') }} · {{ selectedDiscovery.id }}</h3><p>{{ selectedDiscovery.last_error_code ? displayPublicError(publicErrorMessage({ code: selectedDiscovery.last_error_code })) : t('admin.copy.85b71678') }} · {{ t('admin.copy.c4397333') }} {{ date(selectedDiscovery.next_check_at) }}</p><button v-if="selectedDiscovery.state === 'failed' && rights.retry && !account.super_admin" type="button" class="button button-primary" :disabled="!canRetryDiscovery" @click="beginReview(retryIntent('retry_discovery'))">{{ t('admin.copy.4dfcbb0b') }}</button></div>
</DetailModal>
      </section>
    </template>

      <div v-if="review" class="scrim" role="presentation" @click.self="review = null; confirmed = false"><section class="dialog panel" role="dialog" aria-modal="true" aria-labelledby="review-title"><h2 id="review-title">{{ t('admin.copy.77d8945f') }}</h2><p>{{ t('admin.copy.6d4a02e0') }}</p><dl class="facts"><dt>{{ t('admin.copy.015371dd') }}</dt><dd>{{ intentLabel(review) }}</dd><dt>{{ t('admin.copy.559998d7') }}</dt><dd class="id">{{ review.targetId ?? (review.body as CommissionCreateBody).anchor_order_id }}</dd><dt v-if="review.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="review.body.reason">{{ review.body.reason }}</dd></dl><label class="confirm-check"><input v-model="confirmed" type="checkbox" />{{ t('admin.copy.01a94dd5') }}</label><div class="actions"><button type="button" class="button button-quiet" @click="review = null; confirmed = false">{{ t('admin.copy.b1d257a2') }}</button><button v-write-guard="writing" type="button" class="button button-primary" :disabled="!confirmed || writing || (review.operation === 'create' ? !rights.run : !rights.retry) || account.super_admin" @click="withGlobalLoading(confirmWrite)">{{ t('admin.copy.3135aeca') }}</button></div></section></div>
  </main>
</template>

<style scoped>
.cycles{width:100%;max-width:1540px;min-width:0;margin:auto;padding:clamp(12px,2.4vw,30px);color:#242a38;overflow-wrap:anywhere}.cycles *{box-sizing:border-box;min-width:0}.head,.section-title,.row-head,.pager,.actions{display:flex;align-items:center;justify-content:space-between;gap:10px}.head{align-items:flex-start;margin-bottom:18px}.head h1{font-size:clamp(22px,3vw,31px);margin:0 0 7px}.head p,.panel p{margin:5px 0 10px}.eyebrow{font-size:11px;letter-spacing:1.2px;color:#5868d9;font-weight:700}.panel{border:1px solid #e8eaf0;background:var(--surface);border-radius:12px;padding:clamp(13px,1.8vw,19px);margin-bottom:15px;box-shadow:0 2px 8px #1b285008}.panel h2{font-size:17px;margin:0}.panel h3{font-size:14px;margin:0}.panel h4{font-size:13px}.sub,.panel small{font-size:11px;color:#747c8d}.tag{display:inline-flex;border-radius:999px;padding:4px 9px;background:#eff1f6;color:#687083;font-size:11px;font-style:normal;white-space:nowrap}.tag.warn{background:#fff0d9;color:#8a5700}.tag.good{background:#e5f5ec;color:#23724e}.intro{background:#f7f8ff;border-color:#dfe3ff}.layout{display:grid;grid-template-columns:minmax(260px,.72fr) minmax(0,1.45fr);gap:15px;align-items:start}.list-panel,.detail{min-width:0}.row,.run-row{display:flex;flex-direction:column;align-items:stretch;text-align:left;width:100%;padding:12px 9px;border:0;border-top:1px solid var(--line);background:transparent;gap:5px;color:inherit;cursor:pointer}.row.selected,.run-row.selected,.discovery-row.selected{background:#f4f5ff}.row-head{justify-content:flex-start;flex-wrap:wrap}.id{overflow-wrap:anywhere;word-break:break-word;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.94em}.pager{margin-top:12px;font-size:11px;color:#737b8a}.pager button,.head button,.button-quiet,.button-primary{border:1px solid #dfe2e9;border-radius:8px;background:white;color:inherit;padding:8px 11px;cursor:pointer}.button-primary{background:#5969dc;color:var(--surface);border-color:#5969dc;font-weight:650}.pager button:disabled,.button-primary:disabled,.button-quiet:disabled,.head button:disabled{opacity:.48;cursor:not-allowed}.facts{display:grid;grid-template-columns:minmax(105px,.4fr) minmax(0,1fr);gap:8px 12px;font-size:11px}.facts dt{color:#737b8b}.facts dd{margin:0;overflow-wrap:anywhere}.callout{display:flex;flex-direction:column;gap:4px;border-radius:9px;padding:11px;margin:12px 0;font-size:12px}.callout.current{background:#e8f6ee;color:#1e6847}.callout.stale{background:#fff1dc;color:#815100}.subsection{border-top:1px solid var(--line);padding-top:15px;margin-top:16px}.subsection .section-title{align-items:flex-start}.data-row,.calc-row,.discovery-row{display:flex;flex-direction:column;gap:5px;padding:11px 8px;border-top:1px solid var(--line);font-size:11px}.run-row{border:1px solid #e7e9ef;border-radius:8px;margin:7px 0;font-size:11px}.calculations{background:#fafbfe;border-radius:9px;padding:12px;margin-top:12px}.calc-row{background:var(--surface);margin-top:7px;border:1px solid var(--line-subtle);border-radius:8px}.retry-box{margin-top:14px;padding:13px;border:1px solid #f1dfbe;background:#fffaf1;border-radius:9px}.retry-box label,.form-grid label{display:flex;flex-direction:column;gap:5px;font-size:12px;margin:9px 0}.retry-box textarea,.form-grid input,.form-grid textarea{width:100%;border:1px solid #dfe2e9;border-radius:7px;padding:9px;font:inherit;color:inherit;resize:vertical;background:var(--surface)}.form-grid{display:grid;grid-template-columns:minmax(0,.85fr) minmax(0,1.15fr);gap:12px}.discovery-row{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;border:1px solid var(--line);border-radius:9px;margin-top:8px;padding:9px;gap:7px 10px}.discovery-select{display:flex;flex-direction:column;align-items:stretch;gap:5px;text-align:left;border:0;background:transparent;color:inherit;font:inherit;grid-row:span 2;cursor:pointer}.error-text{color:#a13b32}.error{background:#fff0ee;color:#a3342c;border-radius:8px;padding:9px;font-size:12px}.notice{background:#e8f6ed;color:#236a49;border-radius:8px;padding:10px;font-size:12px}.pending{border-color:#eedbb8;background:#fffdf8}.pending-item{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:7px 12px;padding:12px 0;border-top:1px solid #eee4d3}.pending-item code{overflow-wrap:anywhere}.pending-item p,.pending-item .actions{grid-column:1/-1;margin:0;justify-content:flex-start;flex-wrap:wrap}.scrim{position:fixed;inset:0;z-index:50;background:#18203388;display:grid;place-items:center;padding:15px}.dialog{width:min(590px,100%);max-height:92vh;overflow:auto;margin:0}.confirm-check{display:flex;gap:8px;align-items:flex-start;font-size:12px;margin:16px 0}.confirm-check input{margin-top:2px;flex:0 0 auto}
.run-earning-row,.allocation-row{display:flex;flex-direction:column;gap:5px;padding:11px 8px;margin-top:7px;border:1px solid var(--line-subtle);border-radius:8px;background:var(--surface);font-size:11px}
 .allocation-filters{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr) auto;align-items:end;gap:8px;margin:10px 0}.allocation-filters label{display:flex;min-width:0;flex-direction:column;gap:4px;font-size:11px}.allocation-filters input{width:100%;min-width:0;border:1px solid #dfe2e9;border-radius:7px;padding:8px;font:inherit}.allocation-heading{margin-top:14px}
@media(max-width:800px){.layout{grid-template-columns:minmax(0,1fr)}.form-grid{grid-template-columns:minmax(0,1fr)}.section-title{align-items:flex-start;flex-wrap:wrap}}
@media(max-width:420px){.head{gap:6px}.head button{padding:7px;font-size:11px}.panel{padding:12px}.facts{grid-template-columns:minmax(82px,.38fr) minmax(0,1fr);gap:7px}.discovery-row{grid-template-columns:minmax(0,1fr)}.discovery-select{grid-row:auto}.pager{gap:5px}.pager button{padding:7px;font-size:11px}.pending-item{grid-template-columns:minmax(0,1fr)}.pending-item .actions{grid-column:1}.actions{flex-wrap:wrap}}
@media(max-width:520px){.allocation-filters{grid-template-columns:minmax(0,1fr)}.allocation-filters button{width:100%}}
.layout { grid-template-columns: minmax(0, 1fr); }
</style>
