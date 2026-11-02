<script setup lang="ts">
import {nextTick} from "vue"
import MemberActionsMenu from "@lottery/shared/MemberActionsMenu.vue"
import ManagementList from "@lottery/shared/ManagementList.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
const agentDraftSearchPending = ref(false);
const accountSearchReset = ref(0);
const allocationAgentFilterSearchPending = ref(false);
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue";
import SelectControl from '@lottery/shared/SelectControl.vue'
import './commission-management.css';
import ManagementTable from '@lottery/shared/ManagementTable.vue';
import PlatformTemporalInput from '@lottery/shared/PlatformTemporalInput.vue';
import { commissionDateRange, type CommissionListQuery } from './commission-list-query';
import { useOperationToast } from "@lottery/shared";
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
import { vInlineValidation } from "@lottery/shared";
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
  type CommissionRunPage, type CommissionCalculationPage,
  type CommissionDiscovery, type CommissionDiscoveryPage, type CommissionCreateBody, type CommissionRetryBody,
} from "./commission-cycles-api";
import {
  classifyCommissionCycleWriteFailure, clearPendingCommissionCycleWrite,
  commissionCycleSessionGeneration,
  freezeCommissionCycleBody, getPendingCommissionCycleWrite,
  listPendingCommissionCycleWrites, setPendingCommissionCycleWrite,
  type CommissionWriteScope, type PendingCommissionCycleWrite,
} from "./commission-cycles-state";

const props = defineProps<{ account: AdminAccount; brandId: string; initialCycleId?: string; brandStatus?: string; covered?: boolean }>();
const emit = defineEmits<{ (event: "session-invalid"): void; (event: "open-payments", cycleId: string): void; (event: "open-corrections", cycleId: string): void }>();
const { t, locale, message, display } = useAdminI18n();
const api = createCommissionCyclesApi();
const PAGE_SIZE = 20;
const fromDraft = ref(''), toDraft = ref(''), stateDraft = ref(''), agentDraft = ref('');
const committedFilter = ref<CommissionListQuery>({});
const searched = ref(true), showCreate = ref(false), showTasks = ref(false);
const detailTab = ref<'result'|'details'|'history'>('result');
const columns = computed(() => [
 {key:'cycle',label:t('admin.commission.ui.cycle')}, {key:'state',label:t('admin.commission.ui.state')},
 {key:'agents',label:t('admin.commission.ui.agents'),cellClass:'numeric'}, {key:'points',label:t('admin.commission.ui.points'),cellClass:'numeric'},
 {key:'updated',label:t('admin.commission.ui.updated')}, {key:'actions',label:t('admin.commission.ui.actions')},
]);
const earningColumns = computed(() => [ {key:'agent',label:t('admin.commission.ui.agent')}, {key:'points',label:t('admin.commission.ui.points'),cellClass:'numeric'} ]);
function cycleState(item: CommissionCycleRecord) { return item.state === 'ready' && !item.evidence_current ? 'stale' : item.state; }
function cycleStateLabel(item: CommissionCycleRecord) { return cycleState(item) === 'stale' ? t('admin.commission.ui.stale') : item.state === 'ready' ? t('admin.commission.ui.ready') : statusText(item.state); }
function cycleComplete(item: CommissionCycleRecord) { return item.state === 'ready' && item.evidence_current; }
async function applyFilters() {
  if (agentDraftSearchPending.value) return;
 try {
  const range = commissionDateRange(fromDraft.value, toDraft.value);
  committedFilter.value = {...range, ...(stateDraft.value?{state:stateDraft.value}:{}), ...(agentDraft.value?{agent_id:agentDraft.value}:{})};
  searched.value=true; closeDetails(); await loadCycles(0);
 } catch { listError.value=t('admin.commission.ui.dateError'); }
}
async function resetFilters() {agentDraftSearchPending.value=false;accountSearchReset.value++; fromDraft.value=toDraft.value=stateDraft.value=agentDraft.value=''; await applyFilters(); }
function dirtyFilters() { cycleTicket++; cycles.value=[]; cycleTotal.value='0'; cycleOffset.value=0; loading.value=false; searched.value=false; }
async function openTab(tab: 'result'|'details'|'history') {
 if (tab !== detailTab.value) { allocationAgentFilter.value = allocationOrderFilter.value = ''; allocationAgentFilterSearchPending.value = false; }
 detailTab.value=tab;
 const cycle=selectedCycle.value; if (!cycle) return;
 if (tab==='history' && !runs.value) await loadRuns(cycle.id,0);
 if (tab==='details' && cycle.current_run_id) {
  selectedRunId.value=cycle.current_run_id;
  await Promise.all([loadAllocations(cycle.id,cycle.current_run_id,0),loadCalculations(cycle.id,cycle.current_run_id,0)]);
 }
}
async function openTasks() { showTasks.value=true; await loadDiscoveries(0); }

const knownConflictKeys = new Set<string>();
const rights = computed(() => commissionCyclePermissions(props.account, props.brandId));
const scopeStamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.brandStatus, rights.value]));

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
const discoveriesLoaded = ref(false);
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

const canCreate = computed(() => rights.value.run && props.brandStatus !== "disabled" && !props.account.super_admin && !writing.value && !pendingIntents.value.some((item) => item.operation === "create") && validReason(createReason.value) && validUuid(anchorOrderId.value.trim()));
const canRetryCycle = computed(() => Boolean(rights.value.retry && props.brandStatus !== "disabled" && !props.account.super_admin && selectedCycle.value?.state === "failed" && selectedCycle.value.version < Number.MAX_SAFE_INTEGER && validReason(retryReason.value) && !writing.value && !pendingFor("retry_cycle", selectedCycle.value.id)));
const canRetryDiscovery = computed(() => Boolean(rights.value.retry && props.brandStatus !== "disabled" && !props.account.super_admin && selectedDiscovery.value?.state === "failed" && selectedDiscovery.value.version < Number.MAX_SAFE_INTEGER && validReason(retryReason.value) && !writing.value && !pendingFor("retry_discovery", selectedDiscovery.value.id)));
const cycleHasNext = computed(() => BigInt(cycleOffset.value + PAGE_SIZE) < BigInt(cycleTotal.value));
const discoveryHasNext = computed(() => BigInt(discoveryOffset.value + PAGE_SIZE) < BigInt(discoveryTotal.value));

function validUuid(value: string): boolean { return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value); }
function validReason(value: string): boolean {
  if (value.trim() !== value || Array.from(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
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
function percent(value: string) { const [whole, fraction=''] = value.split('.'); const scaled = BigInt(whole + fraction.padEnd(6,'0')) * 100n; const digits=scaled.toString().padStart(7,'0'); const decimals=digits.slice(-6).replace(/0+$/,''); return digits.slice(0,-6)+(decimals?'.'+decimals:'')+'%'; }
function exact(value: string | { numerator: string; denominator: string }) { return typeof value === "string" ? value : `${value.numerator}/${value.denominator}`; }
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
  discoveriesLoaded.value = false; discoveries.value = []; discoveryTotal.value = "0"; discoveryOffset.value = 0; selectedDiscovery.value = null;
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
  fromDraft.value=toDraft.value=stateDraft.value=agentDraft.value='';
  committedFilter.value=props.initialCycleId?{cycle_id:props.initialCycleId}:{}; searched.value=true; showCreate.value=showTasks.value=false;
  if (rights.value.view) { void loadCycles(0); if (props.initialCycleId) void loadCycle(props.initialCycleId); }
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

async function loadCycles(offset = cycleOffset.value) {
  if (!rights.value.view) return;
  const ticket = ++cycleTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  loading.value = true; listError.value = "";
  try {
    const page: CommissionCyclePage = await api.list(props.brandId, PAGE_SIZE, offset, committedFilter.value);
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
    discoveriesLoaded.value = true; discoveries.value = page.items; discoveryTotal.value = page.total_count; discoveryOffset.value = page.offset;
    if (selectedDiscovery.value) selectedDiscovery.value = page.items.find((item) => item.id === selectedDiscovery.value?.id) ?? null;
  } catch (cause) { if (current(ticket, discoveryTicket, captured, session)) { sessionExpired(cause, captured, session); discoveryError.value = readFailure(cause); } }
  finally { if (current(ticket, discoveryTicket, captured, session)) discoveryLoading.value = false; }
}
async function loadCycle(id: string) {
  const ticket = ++detailTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  detailLoading.value = true; detailTab.value="result"; detailError.value = ""; selectedCycle.value = null; retryReason.value = "";
  earningTicket++; runTicket++; calculationTicket++; runEarningTicket++; allocationTicket++;
  earnings.value = null; runs.value = null; calculations.value = null; selectedRunId.value = "";
  runEarnings.value = null; allocations.value = null; allocationAgentFilter.value = allocationOrderFilter.value = "";
  earningsError.value = runsError.value = calculationsError.value = runEarningsError.value = allocationsError.value = ""; refreshPending();
  try {
    const item = await api.read(props.brandId, id);
    if (!current(ticket, detailTicket, captured, session) || item.id !== id) return;
    selectedCycle.value = item;
    await loadEarnings(id, 0);
  } catch (cause) { if (current(ticket, detailTicket, captured, session)) { sessionExpired(cause, captured, session); detailError.value = readFailure(cause); } }
  finally { if (current(ticket, detailTicket, captured, session)) detailLoading.value = false; }
}
async function loadEarnings(cycleId: string, offset = earningOffset.value) {
  const ticket = ++earningTicket, captured = scopeStamp.value, session = commissionCycleSessionGeneration();
  earningsLoading.value = true; earningsError.value = "";
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
  runsLoading.value = true; runsError.value = "";
  try {
    const page: CommissionRunPage = await api.runs(props.brandId, cycleId, PAGE_SIZE, offset);
    if (!current(ticket, runTicket, captured, session) || selectedCycle.value?.id !== cycleId) return;
    if (page.cycle_id !== cycleId || page.items.some((row) => row.cycle_id !== cycleId)) throw new Error(t('admin.copy.4d720ae2'));
    runs.value = page; runOffset.value = page.offset;
    const selected = page.items.find((run) => run.id === selectedRunId.value) ?? page.items[0];
    if (selected) await selectRun(selected.id);
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
  runEarningsLoading.value = true; runEarningsError.value = "";
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
  allocationsLoading.value = true; allocationsError.value = "";
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
  if (allocationAgentFilterSearchPending.value) return;
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
  calculationsLoading.value = true; calculationsError.value = "";
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
  if (!intent || intent.accountId !== props.account.id || intent.brandId !== props.brandId || props.brandStatus === "disabled" || writing.value ||
    !(intent.operation === "create" ? rights.value.run : rights.value.retry) || props.account.super_admin) return;
  showCreate.value=false; review.value = intent; confirmed.value = false; writeError.value = "";
}
async function submit(intent: PendingCommissionCycleWrite) {
  if (intent.accountId !== props.account.id || intent.brandId !== props.brandId || !rights.value.view || props.brandStatus === "disabled" || props.account.super_admin || writing.value ||
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

watch([fromDraft,toDraft,stateDraft,agentDraft],dirtyFilters,{flush:'sync'});
watch(scopeStamp, resetScope, { immediate: true });
onUnmounted(() => { alive = false; generation++; cycleTicket++; detailTicket++; earningTicket++; runTicket++; calculationTicket++; discoveryTicket++; writeTicket++; });
function closeDetails() {
  detailTicket++; earningTicket++; runTicket++; calculationTicket++; runEarningTicket++; allocationTicket++; selectedCycle.value = null; detailLoading.value = false; detailError.value = '';
}

useOperationToast(() => notice.value, value => display(value), 'info');
useOperationToast(() => writeError.value, value => displayPublicError(value), 'error');
useOperationToast(() => listError.value, value => displayPublicError(value), 'error');
useOperationToast(() => detailError.value, value => displayPublicError(value), 'error');
useOperationToast(() => earningsError.value, value => displayPublicError(value), 'error');
useOperationToast(() => runsError.value, value => displayPublicError(value), 'error');
useOperationToast(() => calculationsError.value, value => displayPublicError(value), 'error');
useOperationToast(() => runEarningsError.value, value => displayPublicError(value), 'error');
useOperationToast(() => allocationsError.value, value => displayPublicError(value), 'error');
useOperationToast(() => discoveryError.value, value => displayPublicError(value), 'error');
defineExpose({ openCycle: loadCycle })
const allocationReset=ref(0)
async function clearAllocationFilters(){allocationReset.value++;allocationAgentFilter.value=allocationOrderFilter.value='';await nextTick();await filterAllocations()}
</script>

<template>
 <main class="commission-workspace commission-cycles" aria-labelledby="cycles-title">
  <header class="console-page-heading commission-heading">
   <h1 id="cycles-title" class="console-page-title">{{ t('admin.commission.ui.management') }}</h1>

  </header>
  <p v-if="!rights.view" class="empty-state">{{ t('admin.copy.8edc5180') }}</p>
  <template v-else>
      <section v-if="receipt" class="panel commission-receipt" role="status" aria-live="polite"><div class="section-title"><div><h2>{{ t('admin.copy.1b3685a6') }}</h2></div><span class="tag good">{{ t('admin.copy.5f313d26') }}</span></div><dl class="facts"><dt>{{ t('admin.copy.015371dd') }}</dt><dd>{{ operationText(receipt.operation) }}</dd><dt>{{ t('admin.copy.788448f7') }}</dt><dd class="id">{{ receipt.record.id }}</dd><dt>{{ t('admin.copy.e59b5282') }}</dt><dd>{{ statusText(receiptState()) }}</dd></dl></section>

      <section v-if="pendingIntents.length" class="panel pending" aria-live="polite">
        <div class="section-title"><div><h2>{{ t('admin.copy.eef84c8f') }}</h2><p>{{ t('admin.copy.56be4d08') }}</p></div><span class="tag warn">{{ t('admin.copy.020ec9b6') }}</span></div>
        <article v-for="intent in pendingIntents" :key="intent.key" class="pending-item"><b>{{ intentLabel(intent) }} · {{ intent.targetId ?? "" }}</b><code></code><p>{{ recoveryReason(intent) }}</p><p v-if="isConflict(intent)" class="error" role="alert">{{ t('admin.copy.2827a14b') }}</p><div class="actions ui-form-actions"><button v-refresh-button="t('admin.copy.586a2411')" v-if="isConflict(intent)" type="button" class="button button-quiet" :disabled="writing || conflictReloading || !rights.view" @click="withGlobalLoading(reloadConflictedIntent, intent)">{{ t('admin.copy.586a2411') }}</button><button v-else type="button" class="button button-primary" :disabled="writing || brandStatus === 'disabled' || !(intent.operation === 'create' ? rights.run : rights.retry) || account.super_admin" @click="recover(intent)">{{ t('admin.copy.cb6f5b32') }}</button><label v-if="isConflict(intent) && conflictReloaded(intent)" class="confirm-check"><input type="checkbox" :checked="conflictReviewed(intent)" @change="markConflictReviewed(intent, $event)" />{{ t('admin.copy.2a662c1c') }}</label><button v-if="isConflict(intent) && conflictReloaded(intent)" type="button" class="button button-quiet" :disabled="writing || !conflictReviewed(intent)" @click="discardIntent(intent)">{{ t('admin.copy.095e078e') }}</button></div></article>
      </section>



    <ManagementList :busy="loading"><template #actions>   <div class="commission-actions console-page-actions commission-list-actions">
    <button v-if="rights.run && !account.super_admin && brandStatus !== 'disabled'" type="button" class="button" @click="showCreate=true">{{ t('admin.commission.ui.manual') }}</button>
    <button v-if="rights.view" type="button" class="button" aria-haspopup="dialog" @click="withGlobalLoading(openTasks)">{{ t('admin.commission.ui.tasks') }}</button>

   </div></template><template #filters>    <form novalidate v-inline-validation class="commission-toolbar ui-filter-bar" @submit.prevent="withGlobalLoading(applyFilters)">
     <label><PlatformTemporalInput :label="t('admin.commission.ui.from')" :placeholder="t('admin.commission.ui.from')" v-model="fromDraft" type="date" :locale="locale" /></label>
     <label><PlatformTemporalInput :label="t('admin.commission.ui.to')" :placeholder="t('admin.commission.ui.to')" v-model="toDraft" type="date" :locale="locale" /></label>
     <label><SelectControl :placeholder="t('admin.commission.ui.state')" v-model="stateDraft" :label="String(t('admin.commission.ui.state'))" :options="[{ value: '', label: (String(t('admin.commission.ui.all'))).trim() }, ...((['enumerating', 'waiting', 'calculating', 'summarizing', 'ready', 'stale', 'failed']).map((state) => ({ value: state, label: (String(state === 'stale' ? t('admin.commission.ui.stale') : state === 'ready' ? t('admin.commission.ui.ready') : statusText(state))).trim() })))]" /></label>
     <label><FilterAssociationSearch v-model="agentDraft" v-model:pending="agentDraftSearchPending" :reset-key="accountSearchReset" :brand-id="brandId" surface="admin" resource="commission" kind="agent" :label="t('admin.commission.ui.agent')" :locale="locale" :scope-key="scopeStamp" :enabled="rights.view" /></label>
     <div class="commission-actions ui-form-actions"><button class="button button-primary" type="submit" :disabled="agentDraftSearchPending || loading">{{ t('admin.commission.ui.query') }}</button><button class="button" type="button" @click="withGlobalLoading(resetFilters)">{{ t('admin.commission.ui.reset') }}</button></div>
    </form></template>
    <p v-if="!searched" class="commission-note">{{ t('admin.commission.ui.filtersChanged') }}</p>
    <p v-if="listError" role="alert" class="commission-warning">{{ t('admin.commission.ui.readFailed') }} <button :disabled="agentDraftSearchPending" class="button" v-refresh-button="t('admin.commission.ui.retryRead')" @click="withGlobalLoading(applyFilters)">{{ t('admin.commission.ui.retryRead') }}</button></p>
    <p v-if="loading" v-global-loading>{{ t('admin.copy.ab7239f3') }}</p>
    <ManagementTable :entries="cycles" :columns="columns" :empty="searched && !loading && !listError && !cycles.length" :empty-label="t('admin.commission.ui.empty')">
     <template #cycle="{entry}"><button class="text-button commission-link" :data-cycle-id="entry.id" :data-cycle-version="entry.version" @click="withGlobalLoading(loadCycle,entry.id)">{{ date(entry.window_from) }} – {{ date(entry.window_to) }}</button></template>
     <template #state="{entry}"><span class="commission-status" :class="cycleState(entry)">{{ cycleStateLabel(entry) }}</span></template>
     <template #agents="{entry}">{{ cycleComplete(entry)?exact(entry.earning_count):'—' }}</template>
     <template #points="{entry}">{{ cycleComplete(entry)?exact(entry.total_points):t('admin.commission.ui.incomplete') }}</template>
     <template #updated="{entry}">{{ date(entry.updated_at) }}</template>
     <template #actions="{entry}"><MemberActionsMenu :id="'CommissionCyclesManagement-'+entry.id" :label="t('admin.copy.d21fcbef')"><button type="button" role="menuitem" class="button" @click="withGlobalLoading(loadCycle,entry.id)">{{ t('admin.commission.ui.view') }}</button><button type="button" role="menuitem" class="button" :data-cycle-payments="entry.id" :disabled="!entry.current_run_id" aria-haspopup="dialog" @click="emit('open-payments',entry.id)">{{ t('admin.commission.ui.paymentsAction') }}</button><button type="button" role="menuitem" class="button" :data-cycle-corrections="entry.id" :disabled="!entry.current_run_id" aria-haspopup="dialog" @click="emit('open-corrections',entry.id)">{{ t('admin.commission.ui.correctionsAction') }}</button></MemberActionsMenu></template>
    </ManagementTable>
    <template #pagination><TablePagination :show-empty="searched && !loading && !listError" class="commission-pager" :total="cycleTotal" :page="Math.floor(cycleOffset / PAGE_SIZE + 1)" :count="cycles.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="loading || cycleOffset===0" :next-disabled="loading || !cycleHasNext" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageCycles(target - (Math.floor(cycleOffset / PAGE_SIZE + 1)))" /></template></ManagementList>
      <DetailModal v-if="showTasks" :title="t('admin.commission.ui.tasks')" :close-label="t('association.close')" :open="!selectedDiscovery && !review" @close="showTasks=false; selectedDiscovery=null"><ManagementList embedded class="discoveries"><header class="console-page-heading"><div class="console-page-actions"><button v-refresh-button="t('admin.copy.894e2a7d')" type="button" class="button" :disabled="discoveryLoading || !rights.view" @click="withGlobalLoading(loadDiscoveries, discoveryOffset)">{{ t('admin.copy.894e2a7d') }}</button></div></header><div v-if="!discoveryLoading && !discoveryError && !discoveries.length" class="empty empty-state">{{ t('admin.copy.6f161c9e') }}</div><article v-for="item in discoveries" :key="item.id" :data-discovery-id="item.id" class="discovery-row" :class="{ selected: selectedDiscovery?.id === item.id }"><button type="button" class="discovery-select" @click="selectDiscovery(item)"><span class="row-head"><b>{{ statusText(item.state) }}</b><span class="id">{{ item.id }}</span></span><span>{{ t('admin.copy.11c28a36') }} <span v-if="item.cycle_id" class="id"><AssociationName :record="item" kind="cycle" :id="item.cycle_id" :locale="associationLocale" /></span><span v-else>{{ t('admin.copy.941137c1') }}</span></span><span>{{ item.window_from ? `${date(item.window_from)} – ${date(item.window_to)}` : t('admin.copy.6a0bb6fb') }}</span><span v-if="item.last_error_code" class="error-text">{{ t('admin.copy.c0a94db8') }}: {{ item.last_error_code ? displayPublicError(publicErrorMessage({ code: item.last_error_code })) : "—" }}</span><span>{{ t('admin.copy.c4397333') }} {{ date(item.next_check_at) }}</span></button><button v-if="item.state === 'failed' && rights.retry && !account.super_admin" type="button" class="button button-quiet" @click="selectDiscovery(item); retryReason = ''">{{ t('admin.copy.e57b7cb7') }}</button></article>


      <template #pagination><TablePagination :show-empty="discoveriesLoaded" class="pager" :total="discoveryTotal" :page="Math.floor(discoveryOffset / PAGE_SIZE + 1)" :count="discoveries.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="discoveryLoading || discoveryOffset === 0" :next-disabled="discoveryLoading || !discoveryHasNext" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageDiscoveries(target - (Math.floor(discoveryOffset / PAGE_SIZE + 1)))" /></template></ManagementList></DetailModal>
        <DetailModal v-if="!!selectedDiscovery" :title="t('admin.copy.01db40da')" :close-label="t('admin.copy.2bb2cb1d')" :open="!review" @close="selectedDiscovery = null">
<div v-if="selectedDiscovery" class="retry-box"><header class="console-page-heading"><div class="console-page-actions"><button v-refresh-button="t('admin.copy.894e2a7d')" type="button" class="button" :disabled="discoveryLoading || !rights.view" @click="withGlobalLoading(loadDiscoveries, discoveryOffset)">{{ t('admin.copy.894e2a7d') }}</button></div></header><h3>{{ t('admin.copy.c1fe5e47') }} · {{ selectedDiscovery.id }}</h3><p>{{ selectedDiscovery.last_error_code ? displayPublicError(publicErrorMessage({ code: selectedDiscovery.last_error_code })) : t('admin.copy.85b71678') }} · {{ t('admin.copy.c4397333') }} {{ date(selectedDiscovery.next_check_at) }}</p><div v-if="selectedDiscovery.state === 'failed' && rights.retry && !account.super_admin" class="ui-form-actions"><button type="button" class="button button-primary" :disabled="!canRetryDiscovery" @click="beginReview(retryIntent('retry_discovery'))">{{ t('admin.copy.4dfcbb0b') }}</button></div></div>
</DetailModal>

   <DetailModal v-if="selectedCycle || detailLoading || detailError" :open="!review && !covered" :title="t('admin.nav.commissionCycles')" :close-label="t('association.close')" @close="closeDetails"><div class="commission-detail">
    <header class="console-page-heading commission-heading"><div v-if="selectedCycle" class="commission-actions console-page-actions ui-form-actions"><button class="button" v-refresh-button="t('admin.copy.894e2a7d')" @click="withGlobalLoading(loadCycle,selectedCycle.id)">{{ t('admin.copy.894e2a7d') }}</button><button class="button" @click="emit('open-payments',selectedCycle.id)">{{ t('admin.commission.ui.openPayments') }}</button><button class="button" :disabled="!selectedCycle.current_run_id" @click="emit('open-corrections',selectedCycle.id)">{{ t('admin.commission.ui.correctionsAction') }}</button></div></header>
    <p v-if="detailLoading" v-global-loading>{{ t('admin.copy.ab7239f3') }}</p>
    <p v-if="detailError" class="commission-warning" role="alert">{{ displayPublicError(detailError) }}</p>
    <template v-if="selectedCycle">
     <div class="commission-heading"><b>{{ date(selectedCycle.window_from) }} – {{ date(selectedCycle.window_to) }}</b><span class="commission-status" :class="cycleState(selectedCycle)">{{ cycleStateLabel(selectedCycle) }}</span></div>
     <p v-if="selectedCycle.last_error_code" class="commission-warning">{{ displayPublicError(publicErrorMessage({code:selectedCycle.last_error_code})) }}</p>
     <button v-if="selectedCycle.state==='failed' && rights.retry && !account.super_admin && brandStatus !== 'disabled'" class="button button-primary" :disabled="!canRetryCycle" @click="beginReview(retryIntent('retry_cycle'))">{{ t('admin.copy.a6f031a3') }}</button>
     <nav class="commission-tabs" role="tablist"><button v-for="tab in (['result','details','history'] as const)" :key="tab" class="button" role="tab" :aria-selected="detailTab===tab" @click="withGlobalLoading(openTab,tab)">{{ t(adminKey('admin.commission.ui.'+tab)) }}</button></nav>
     <ManagementList embedded v-if="detailTab==='result'" class="commission-detail" role="tabpanel">
      <dl class="commission-metrics"><div><dt>{{ t('admin.commission.ui.points') }}</dt><dd>{{ cycleComplete(selectedCycle)?exact(selectedCycle.total_points):t('admin.commission.ui.incomplete') }}</dd></div><div><dt>{{ t('admin.commission.ui.agents') }}</dt><dd>{{ cycleComplete(selectedCycle)?exact(selectedCycle.earning_count):'—' }}</dd></div></dl>
      <ManagementTable :entries="earnings?.items ?? []" :columns="earningColumns" :empty="!!earnings && !earnings.items.length" :empty-label="t('admin.commission.ui.empty')"><template #agent="{entry}"><AssociationName :record="entry" kind="agent" :id="entry.agent_id" :locale="locale" /></template><template #points="{entry}">{{ cycleComplete(selectedCycle)?exact(entry.points):t('admin.commission.ui.incomplete') }}</template></ManagementTable>

     <template #pagination><TablePagination :show-empty="Boolean(earnings)" v-if="earnings" class="commission-pager" :total="earnings.total_count" :page="Math.floor(earningOffset / PAGE_SIZE + 1)" :count="earnings.items.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="earningsLoading || earningOffset===0" :next-disabled="earningsLoading || BigInt(earningOffset+PAGE_SIZE)>=BigInt(earnings.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageEarnings(target - (Math.floor(earningOffset / PAGE_SIZE + 1)))" /></template></ManagementList>
     <section v-if="detailTab==='details'" class="commission-detail" role="tabpanel">
      <ManagementList embedded><template #filters><form v-inline-validation novalidate class="commission-toolbar ui-filter-bar" @submit.prevent="withGlobalLoading(filterAllocations)"><label><FilterAssociationSearch :reset-key="allocationReset" v-model="allocationAgentFilter" v-model:pending="allocationAgentFilterSearchPending" :brand-id="brandId" surface="admin" resource="commission" kind="agent" :label="t('filter.agent')" :locale="locale" :scope-key="scopeStamp" :enabled="rights.view" @update:model-value="allocationFilterChanged" /></label><label><FilterAssociationSelect v-model="allocationOrderFilter" :brand-id="brandId" surface="admin" resource="commission" kind="bet_order" :label="t('filter.bet_order')" :locale="locale" :scope-key="scopeStamp" :enabled="rights.view" @update:model-value="allocationFilterChanged" /></label><button class="button button-primary" :disabled="allocationAgentFilterSearchPending || allocationsLoading || !selectedRunId" >{{ t('admin.commission.ui.query') }}</button><button type="button" class="button button-secondary" :disabled="allocationsLoading" @click="withGlobalLoading(clearAllocationFilters)">{{t('admin.copy.b089734d')}}</button></form></template>
      <article v-for="(row,index) in allocations?.items ?? []" :key="index" class="panel">
       <b><AssociationName :record="row" kind="agent" :id="row.agent_id" :locale="locale" /></b>
       <dl class="commission-facts"><dt>{{ t('admin.commission.ui.order') }}</dt><dd>{{ row.order_id }}</dd><dt>{{ t('filter.member') }}</dt><dd><AssociationName :record="row" kind="member" :id="row.bettor_member_id" :locale="locale" /></dd><dt>{{ t('admin.commission.ui.base') }}</dt><dd>{{ exact(row.base_points) }}</dd><dt>{{ t('admin.commission.ui.ratio') }}</dt><dd>{{ percent(row.agent_ratio) }}</dd><dt>{{ t('admin.commission.ui.downstream') }}</dt><dd>{{ percent(row.downstream_ratio) }}</dd><dt>{{ t('admin.commission.ui.difference') }}</dt><dd>{{ percent(row.difference_ratio) }}</dd><dt>{{ t('admin.commission.ui.exact') }}</dt><dd>{{ exact(row.exact_amount) }}</dd></dl>
      </article>
      <p v-if="allocations && !allocations.items.length" class="empty-state">{{ t('admin.commission.ui.empty') }}</p>
      <template #pagination><TablePagination :show-empty="Boolean(allocations)" v-if="allocations" class="commission-pager" :total="allocations.total_count" :page="Math.floor(allocationOffset / PAGE_SIZE + 1)" :count="allocations.items.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="allocationsLoading || allocationOffset===0" :next-disabled="allocationsLoading || BigInt(allocationOffset+PAGE_SIZE)>=BigInt(allocations.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageAllocations(target - (Math.floor(allocationOffset / PAGE_SIZE + 1)))" /></template></ManagementList>

     <details class="commission-technical"><summary>{{ t('admin.commission.ui.sources') }}</summary><ManagementList embedded><article v-for="item in calculations?.items ?? []" :key="item.id" class="panel"><b>{{ statusText(item.reason) }} · {{ statusText(item.status) }}</b><p>{{ item.order_id }}</p><p>{{ t('admin.copy.96730052') }} {{ item.stake_points }} · {{ t('admin.copy.31452fe8') }} {{ item.prize_points }} · {{ t('admin.commission.ui.base') }} {{ item.base_points }}</p></article><template #pagination><TablePagination :show-empty="Boolean(calculations)" v-if="calculations" class="commission-pager" :total="calculations.total_count" :page="Math.floor(calculationOffset / PAGE_SIZE + 1)" :count="calculations.items.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="calculationsLoading || calculationOffset===0" :next-disabled="calculationsLoading || BigInt(calculationOffset+PAGE_SIZE)>=BigInt(calculations.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageCalculations(target - (Math.floor(calculationOffset / PAGE_SIZE + 1)))" /></template></ManagementList></details>
     </section>
            <section v-if="detailTab === 'history'" class="subsection"><ManagementList embedded><div class="section-title"><div><h3>{{ t('admin.copy.c29e7175') }}</h3></div><span v-global-loading v-if="runsLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div><button v-for="run in runs?.items ?? []" :key="run.id" :data-run-id="run.id" type="button" class="run-row" :class="{ selected: selectedRunId === run.id }" @click="withGlobalLoading(selectRun, run.id)"><b>{{ statusText(run.state) }} · {{ t('admin.copy.c8648ca9') }} {{ run.generation }}</b><span class="id">{{ run.id }}</span><span>{{ t('admin.copy.57546450') }} {{ exact(run.calculated_count) }} · {{ t('admin.copy.043f32b3') }} {{ exact(run.earning_count) }} · {{ t('admin.copy.ec082fa7') }} {{ exact(run.total_points) }}</span></button><template #pagination><TablePagination :show-empty="Boolean(runs)" v-if="runs" class="pager" :total="runs.total_count" :page="Math.floor(runOffset / PAGE_SIZE + 1)" :count="runs.items.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="runsLoading || runOffset === 0" :next-disabled="runsLoading || BigInt(runOffset + PAGE_SIZE) >= BigInt(runs.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageRuns(target - (Math.floor(runOffset / PAGE_SIZE + 1)))" /></template></ManagementList>
              <div v-if="selectedRunId" class="calculations"><ManagementList embedded><h4>{{ t('admin.copy.685b8e96') }} · {{ selectedRunId }}</h4><span v-global-loading v-if="calculationsLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span><article v-for="item in calculations?.items ?? []" :key="item.id" class="calc-row"><b>{{ statusText(item.reason) }} · {{ statusText(item.status) }}</b><span class="id">{{ item.order_id }} · <AssociationName :record="item" kind="member" :id="item.member_id" :locale="associationLocale" /></span><span>{{ t('admin.copy.96730052') }} {{ exact(item.stake_points) }} · {{ t('admin.copy.31452fe8') }} {{ exact(item.prize_points) }} · {{ t('admin.copy.ef515018') }} {{ exact(item.base_points) }}</span></article><template #pagination><TablePagination :show-empty="Boolean(calculations)" v-if="calculations" class="pager" :total="calculations.total_count" :page="Math.floor(calculationOffset / PAGE_SIZE + 1)" :count="calculations.items.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="calculationsLoading || calculationOffset === 0" :next-disabled="calculationsLoading || BigInt(calculationOffset + PAGE_SIZE) >= BigInt(calculations.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageCalculations(target - (Math.floor(calculationOffset / PAGE_SIZE + 1)))" /></template></ManagementList>
                <section class="subsection historical"><ManagementList embedded><h4>{{ t('admin.copy.666c29a4') }}</h4>
                  <div class="section-title"><h4>{{ t('admin.copy.e6563a7a') }}</h4><span v-global-loading v-if="runEarningsLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div><article v-for="row in runEarnings?.items ?? []" :key="row.id" class="run-earning-row"><b class="id"><AssociationName :record="row" kind="agent" :id="row.agent_id" :locale="associationLocale" /></b><span>{{ t('admin.copy.56b2d19b') }} <AssociationName :record="row" kind="member" :id="row.member_id" :locale="associationLocale" /> {{ t('admin.copy.54e851cb') }} {{ exact(row.points) }}</span><span>{{ t('admin.copy.d7344f9c') }} {{ exact(row.exact_amount) }}</span></article><p v-if="runEarnings && !runEarnings.items.length" class="empty empty-state">{{ t('admin.copy.a2f78367') }}</p><template #pagination><TablePagination :show-empty="Boolean(runEarnings)" v-if="runEarnings" class="pager" :total="runEarnings.total_count" :page="Math.floor(runEarningOffset / PAGE_SIZE + 1)" :count="runEarnings.items.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="runEarningsLoading || runEarningOffset === 0" :next-disabled="runEarningsLoading || BigInt(runEarningOffset + PAGE_SIZE) >= BigInt(runEarnings.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageRunEarnings(target - (Math.floor(runEarningOffset / PAGE_SIZE + 1)))" /></template></ManagementList>
                  <ManagementList embedded><div class="section-title allocation-heading"><h4>{{ t('admin.copy.d4543586') }}</h4><span v-global-loading v-if="allocationsLoading" class="tag">{{ t('admin.copy.ab7239f3') }}</span></div>
                  <template #filters><form v-inline-validation novalidate class="allocation-filters ui-filter-bar" @submit.prevent="withGlobalLoading(filterAllocations)">
                    <label><FilterAssociationSearch :reset-key="allocationReset" v-model="allocationAgentFilter" v-model:pending="allocationAgentFilterSearchPending" :brand-id="brandId" surface="admin" resource="commission" kind="agent" :label="t('filter.agent')" :locale="displayLocale" :scope-key="JSON.stringify(account)" :enabled="rights.view" @update:model-value="allocationFilterChanged" /></label>
                    <label><FilterAssociationSelect v-model="allocationOrderFilter" :brand-id="brandId" surface="admin" resource="commission" kind="bet_order" :label="t('filter.bet_order')" :locale="displayLocale" :scope-key="JSON.stringify(account)" :enabled="rights.view" @update:model-value="allocationFilterChanged" /></label>
                    <button type="submit" class="button button-primary" :disabled="allocationAgentFilterSearchPending || allocationsLoading">{{ t('admin.copy.b8972a75') }}</button><button type="button" class="button button-secondary" :disabled="allocationsLoading" @click="withGlobalLoading(clearAllocationFilters)">{{t('admin.copy.b089734d')}}</button>
                  </form></template>

                  <article v-for="(row, index) in allocations?.items ?? []" :key="`${row.order_id}:${row.agent_id}:${index}`" class="allocation-row">
                    <b>{{ t('admin.copy.f292714a') }} <span class="id">{{ row.order_id }}</span> {{ t('admin.copy.b863c1ed') }} <span class="id"><AssociationName :record="row" kind="agent" :id="row.agent_id" :locale="associationLocale" /></span></b>
                    <span>{{ t('admin.copy.3298568f') }} <AssociationName :record="row" kind="member" :id="row.member_id" :locale="associationLocale" /> {{ t('admin.copy.4c082700') }} <AssociationName :record="row" kind="member" :id="row.bettor_member_id" :locale="associationLocale" /></span>
                    <span>{{ t('admin.copy.ef515018') }} {{ exact(row.base_points) }} · {{ statusText(row.mode) }} · {{ t('admin.copy.7beeeccb') }} {{ row.agent_ratio }} · {{ t('admin.copy.07830cda') }} {{ row.downstream_ratio }} · {{ t('admin.copy.26964f6f') }} {{ row.difference_ratio }}</span>
                    <span>{{ t('admin.copy.ca243f72') }} {{ exact(row.exact_amount) }}</span>
                    <small>{{ t('admin.copy.f5703e2b') }} {{ row.calculation_id }} · {{ date(row.created_at) }}</small>
                  </article>
                  <p v-if="allocations && !allocations.items.length" class="empty empty-state">{{ t('admin.copy.ae2a5503') }}</p>
                  <template #pagination><TablePagination :show-empty="Boolean(allocations)" v-if="allocations" class="pager" :total="allocations.total_count" :page="Math.floor(allocationOffset / PAGE_SIZE + 1)" :count="allocations.items.length" :locale="locale" :page-size="PAGE_SIZE" :previous-disabled="allocationsLoading || allocationOffset === 0" :next-disabled="allocationsLoading || BigInt(allocationOffset + PAGE_SIZE) >= BigInt(allocations.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => pageAllocations(target - (Math.floor(allocationOffset / PAGE_SIZE + 1)))" /></template></ManagementList>
                </section>
              </div>
            </section>

    </template>
   </div></DetailModal>
   <DetailModal v-if="showCreate" class="ui-form-dialog" :open="!review" :title="t('admin.commission.ui.manual')" :close-label="t('association.close')" @close="showCreate=false">
    <div class="ui-form">
     <label>{{ t('admin.commission.ui.order') }}<FilterAssociationSelect v-model="anchorOrderId" :brand-id="brandId" surface="admin" resource="commission" kind="anchor_order" :label="t('admin.commission.ui.order')" :locale="locale" :scope-key="scopeStamp" :enabled="rights.run" /></label>
     <div class="ui-form-actions"><button type="button" class="button button-primary" :disabled="!canCreate" @click="beginReview(createIntent())">{{ t('admin.copy.2e889bb3') }}</button></div>
    </div>
   </DetailModal>
  </template>
      <DetailModal v-if="review" class="confirmation-dialog dialog" :title="t('admin.copy.77d8945f')" title-id="review-title" :close-label="t('association.close')" :close-disabled="writing" @close="if (!(writing)) { review = null; confirmed = false; }" ><p>{{ t('admin.copy.6d4a02e0') }}</p><dl class="facts"><dt>{{ t('admin.copy.015371dd') }}</dt><dd>{{ intentLabel(review) }}</dd><dt>{{ t('admin.copy.559998d7') }}</dt><dd class="id">{{ review.targetId ?? (review.body as CommissionCreateBody).anchor_order_id }}</dd><dt v-if="review.body.reason">{{ t('admin.copy.5ec7c8ba') }}</dt><dd v-if="review.body.reason">{{ review.body.reason }}</dd></dl><label class="confirm-check"><input v-model="confirmed" type="checkbox" />{{ t('admin.copy.01a94dd5') }}</label><div class="actions ui-form-actions"><button v-write-guard="writing" type="button" class="button button-primary" :disabled="!confirmed || writing || (review.operation === 'create' ? !rights.run : !rights.retry) || account.super_admin" @click="withGlobalLoading(confirmWrite)">{{ t('admin.copy.3135aeca') }}</button></div></DetailModal>
 </main>
</template>
<style scoped>
.commission-list-toolbar { display:flex; align-items:flex-end; flex-wrap:wrap; gap:12px; }
#app .commission-list-toolbar > .ui-filter-bar { flex:1 1 736px; margin:0; }
.commission-list-actions { margin-inline-start:auto; justify-content:flex-end; }
.subsection { display:grid; gap:10px; }
.section-title,.row-head,.actions { display:flex;align-items:center;gap:8px;flex-wrap:wrap;justify-content:space-between; }
.run-row,.data-row,.calc-row,.run-earning-row,.allocation-row,.discovery-select { display:grid;gap:6px;text-align:left;overflow-wrap:anywhere; }
.run-row,.calc-row,.run-earning-row,.allocation-row,.discovery-row { padding:10px;border:1px solid var(--line);border-radius:8px;background:var(--surface); }
.run-row.selected { border-color:var(--primary);background:var(--primary-light); }
.id { overflow-wrap:anywhere; }
.pending { border-color:var(--warning); }
.pending-item { padding-block:8px; }
.calculations,.historical { display:grid;gap:8px; }
.allocation-filters { display:flex;align-items:end;gap:8px;flex-wrap:wrap; }
.allocation-filters label { display:grid;gap:4px;max-width:100%; }
.discoveries { display:grid;gap:8px; }
.discoveries .discovery-select { width:100%;border:0;background:transparent;color:inherit;font:inherit;cursor:pointer; }
.facts { display:grid;grid-template-columns:auto minmax(0,1fr);gap:6px 12px; }.facts dd{margin:0;overflow-wrap:anywhere;}
.confirm-check { display:flex;align-items:center;gap:8px;min-height:44px; }
</style>
