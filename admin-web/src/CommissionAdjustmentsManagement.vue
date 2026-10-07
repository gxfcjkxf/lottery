<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import { commissionAdjustmentPermissions, createCommissionAdjustmentsApi, validCommissionAdjustmentDelta, validCommissionAdjustmentPoints, validCommissionAdjustmentReason, type CommissionAdjustment, type CommissionAdjustmentTarget } from "./commission-adjustments-api";
import { classifyCommissionAdjustmentFailure, clearAllPendingCommissionAdjustments, clearPendingCommissionAdjustmentIntent, commissionAdjustmentSessionGeneration, createCommissionAdjustmentIntent, getPendingCommissionAdjustmentIntent, isCommissionAdjustmentConflict, listPendingCommissionAdjustmentIntents, markCommissionAdjustmentConflict, setPendingCommissionAdjustmentIntent, type CommissionAdjustmentIntentScope, type PendingCommissionAdjustmentIntent } from "./commission-adjustments-state";

const props = defineProps<{ account: AdminAccount; brandId: string; paymentId: string; paymentState: string; brandStatus?: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t } = useAdminI18n();
const api = createCommissionAdjustmentsApi(fetch);
const PAGE = 20;
const SCAN_PAGE = 100;
const rights = computed(() => commissionAdjustmentPermissions(props.account, props.brandId));
const stamp = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids, props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, props.paymentId, props.paymentState, props.brandStatus, rights.value]));
const targets = ref<CommissionAdjustmentTarget[]>([]), targetTotal = ref("0"), targetOffset = ref(0), targetLimit = ref(PAGE), selectedId = ref("");
const history = ref<CommissionAdjustment[]>([]), historyTotal = ref("0"), historyOffset = ref(0);
const loading = ref(false), historyLoading = ref(false), writing = ref(false);
const error = ref(""), historyError = ref(""), writeError = ref(""), notice = ref("");
const points = ref(""), reason = ref(""), dialog = ref<PendingCommissionAdjustmentIntent | null>(null), confirmed = ref(false);
const unknown = ref<PendingCommissionAdjustmentIntent | null>(null), conflict = ref<PendingCommissionAdjustmentIntent | null>(null);
const conflictRefreshed = ref(false), conflictTargetMissing = ref(false), discardChecked = ref(false), conflictRefreshBusy = ref(false);
const receipt = ref<CommissionAdjustment | null>(null);
let alive = true, generation = 0, targetTicket = 0, historyTicket = 0, writeTicket = 0, readLane = 0, conflictTicket = 0;
const selected = computed(() => targets.value.find((row) => row.id === selectedId.value) ?? null);
const available = computed(() => Boolean(rights.value.write && props.brandStatus !== "disabled" && props.paymentState === "paid" && selected.value?.state === "paid" && selected.value.adjustment_version !== null && selected.value.adjustment_version < Number.MAX_SAFE_INTEGER && !writing.value && !unknown.value && !conflict.value && !loading.value && !historyLoading.value && !error.value && !historyError.value));
const reasonValid = computed(() => validCommissionAdjustmentReason(reason.value.trim()));
const deltaPreview = computed(() => { if (!selected.value || !/^(?:0|[1-9][0-9]*)$/.test(points.value)) return ""; try { const delta = BigInt(points.value) - BigInt(selected.value.adjusted_points ?? "0"); return delta.toString(); } catch { return ""; } });
const canPrepare = computed(() => available.value && Boolean(selected.value && !getPendingCommissionAdjustmentIntent(scopeFor(selected.value.id))) && validCommissionAdjustmentPoints(points.value) && Boolean(selected.value && validCommissionAdjustmentDelta(selected.value.adjusted_points ?? "0",points.value)) && reasonValid.value);
const hasTargetNext = computed(() => BigInt(targetOffset.value + targets.value.length) < BigInt(targetTotal.value));
const hasHistoryNext = computed(() => BigInt(historyOffset.value + history.value.length) < BigInt(historyTotal.value));
const hasTargetPages = computed(() => BigInt(targetTotal.value) > BigInt(targetLimit.value));
const hasHistoryPages = computed(() => BigInt(historyTotal.value) > BigInt(PAGE));
const scopeFor = (targetId: string): CommissionAdjustmentIntentScope => ({ accountId: props.account.id, brandId: props.brandId, paymentId: props.paymentId, targetId });
function active(_ticket: number, captured: string, session: number) { return alive && captured === stamp.value && session === commissionAdjustmentSessionGeneration() && rights.value.view; }
function readFailure(cause: unknown) { return cause instanceof Error ? cause.message : t("读取失败。", "Unable to load data."); }
function reset() {
  generation++; readLane++; targetTicket++; historyTicket++; writeTicket++; conflictTicket++;
  targets.value = []; history.value = []; targetTotal.value = historyTotal.value = "0"; targetOffset.value = historyOffset.value = 0; targetLimit.value = PAGE; selectedId.value = ""; points.value = ""; reason.value = ""; dialog.value = null; confirmed.value = false; error.value = historyError.value = writeError.value = notice.value = ""; loading.value = historyLoading.value = writing.value = false; conflictRefreshed.value = conflictTargetMissing.value = discardChecked.value = conflictRefreshBusy.value = false; receipt.value = null;
  const pending = listPendingCommissionAdjustmentIntents(props.account.id, props.brandId);
  const forPayment = pending.filter((item) => item.paymentId === props.paymentId);
  conflict.value = forPayment.find(isCommissionAdjustmentConflict) ?? null; unknown.value = conflict.value ? null : forPayment[0] ?? null;
  if (unknown.value) { selectedId.value = unknown.value.targetId; writeError.value = t("有一个待确认请求。请核对冻结的正文和幂等键后，使用原请求重试。", "A request needs confirmation. Review its frozen body and key, then retry the original request."); }
  if (rights.value.view) void loadTargets(0, unknown.value?.targetId);
}
function sessionExpired(cause: unknown, captured: string, session: number) {
  if (cause instanceof AdminApiError && cause.status === 401 && captured === stamp.value && session === commissionAdjustmentSessionGeneration()) {
    clearAllPendingCommissionAdjustments(); generation++; readLane++; targetTicket++; historyTicket++; writeTicket++; conflictTicket++;
    targets.value = []; history.value = []; targetTotal.value = historyTotal.value = "0"; selectedId.value = ""; unknown.value = null; conflict.value = null; dialog.value = null; receipt.value = null;
    loading.value = historyLoading.value = writing.value = false; emit("session-invalid"); return true;
  }
  return false;
}
async function loadTargets(offset = targetOffset.value, locateTargetId?: string) {
  if (!rights.value.view) return;
  conflictTicket++; conflictRefreshBusy.value = false; conflictRefreshed.value = false; discardChecked.value = false;
  const ticket = ++targetTicket, captured = stamp.value, session = commissionAdjustmentSessionGeneration(), lane = ++readLane; loading.value = true; error.value = "";
  try {
    let page = await api.listTargets(props.brandId, props.paymentId, PAGE, offset);
    if (!active(ticket, captured, session) || lane !== readLane) return;
    if (locateTargetId && !page.items.some((row) => row.id === locateTargetId)) {
      let scanOffset = 0, located = false;
      while (true) {
        const scan = await api.listTargets(props.brandId,props.paymentId,SCAN_PAGE,scanOffset);
        if (!active(ticket,captured,session) || lane !== readLane) return;
        const index = scan.items.findIndex((row) => row.id === locateTargetId);
        if (index >= 0) {
          const desiredOffset = scan.offset + Math.floor(index / PAGE) * PAGE;
          page = await api.listTargets(props.brandId,props.paymentId,PAGE,desiredOffset);
          if (!active(ticket,captured,session) || lane !== readLane) return;
          located = page.items.some((row) => row.id === locateTargetId);
          break;
        }
        scanOffset += scan.limit;
        if (!scan.items.length || BigInt(scanOffset) >= BigInt(scan.total_count)) break;
      }
      if (!located) notice.value = t("待确认请求对应的目标当前未出现在分页结果中；原请求仍保留。", "The target for the pending request was not found in the current pages; the original request is still retained.");
    }
    targets.value = page.items; targetTotal.value = page.total_count; targetOffset.value = page.offset; targetLimit.value = page.limit;
    if (locateTargetId && page.items.some((row) => row.id === locateTargetId)) selectedId.value = locateTargetId;
    else if (!page.items.some((row) => row.id === selectedId.value)) selectedId.value = page.items[0]?.id ?? "";
    if (selectedId.value) void loadHistory(selectedId.value);
  } catch (cause) { if (active(ticket, captured, session) && lane === readLane) { sessionExpired(cause,captured,session); error.value = readFailure(cause); } }
  finally { if (active(ticket,captured,session) && ticket === targetTicket) loading.value = false; }
}
async function loadHistory(id: string, offset = historyOffset.value) {
  if (!rights.value.view || !id) return;
  conflictTicket++; conflictRefreshBusy.value = false; conflictRefreshed.value = false; discardChecked.value = false;
  const ticket = ++historyTicket, captured = stamp.value, session = commissionAdjustmentSessionGeneration(), lane = readLane; historyLoading.value = true; historyError.value = "";
  try {
    const page = await api.listAdjustments(props.brandId,id,PAGE,offset);
    if (!active(generation,captured,session) || ticket !== historyTicket || lane !== readLane || selectedId.value !== id || page.items.some((item) => item.target_id !== id || item.payment_id !== props.paymentId)) throw new AdminApiError("佣金修正历史响应范围无效。", 502, "INVALID_RESPONSE");
    history.value = page.items; historyTotal.value = page.total_count; historyOffset.value = page.offset;
  } catch (cause) { if (active(generation,captured,session) && ticket === historyTicket && lane === readLane) { sessionExpired(cause,captured,session); historyError.value = readFailure(cause); } }
  finally { if (ticket === historyTicket && active(generation,captured,session)) historyLoading.value = false; }
}
function chooseTarget(id: string) {
  selectedId.value = id; points.value = ""; reason.value = ""; historyOffset.value = 0; dialog.value = null; receipt.value = null;
  const pending = getPendingCommissionAdjustmentIntent(scopeFor(id));
  conflict.value = pending && isCommissionAdjustmentConflict(pending) ? pending : null;
  unknown.value = pending && !conflict.value ? pending : null;
  void loadHistory(id,0);
}
function pageTargets(next: number) { if (next >= 0 && BigInt(next) < BigInt(targetTotal.value)) void loadTargets(next); }
function pageHistory(next: number) { if (next >= 0 && BigInt(next) < BigInt(historyTotal.value) && selectedId.value) void loadHistory(selectedId.value,next); }
function prepare() {
  if (!canPrepare.value || !selected.value) return;
  const intent = createCommissionAdjustmentIntent(scopeFor(selected.value.id), props.account.id, selected.value.adjusted_points!, { version: selected.value.adjustment_version!, points: points.value, reason: reason.value.trim() }, createIdempotencyKey());
  if (!intent) return; dialog.value = intent; confirmed.value = false; writeError.value = "";
}
function recover() { const intent = unknown.value; if (!intent || !rights.value.write || props.brandStatus === "disabled") return; dialog.value = intent; confirmed.value = false; }
function validScope(intent: PendingCommissionAdjustmentIntent) { return intent.accountId === props.account.id && intent.brandId === props.brandId && intent.paymentId === props.paymentId && intent.actorId === props.account.id; }
async function submit() {
  const intent = dialog.value; if (!intent || !confirmed.value || writing.value || !validScope(intent) || !rights.value.write || props.brandStatus === "disabled") return;
  const recovery = unknown.value?.key === intent.key;
  if (!recovery && (!selected.value || selected.value.id !== intent.targetId || selected.value.adjustment_version !== intent.body.version || !canPrepare.value)) return;
  const intentScope = scopeFor(intent.targetId), existing = getPendingCommissionAdjustmentIntent(intentScope);
  if (existing && existing.key !== intent.key) { writeError.value = t("此目标已有待确认请求；请先处理原请求。", "This target already has a pending request. Resolve the original request first."); return; }
  if (!setPendingCommissionAdjustmentIntent(intentScope,intent)) return;
  const captured = stamp.value, session = commissionAdjustmentSessionGeneration(), ticket = ++writeTicket; writing.value = true; writeError.value = "";
  try {
    const acknowledged = await api.createAdjustment(intent.brandId,intent.targetId,intent.paymentId,intent.body,intent.actorId,intent.key);
    clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key);
    if (!active(ticket,captured,session) || ticket !== writeTicket) return;
    receipt.value = acknowledged; dialog.value = null; unknown.value = null; confirmed.value = false; notice.value = t("服务器确认了这条不可变更正回执；当前目标与历史已另行重读。", "The server acknowledged this immutable adjustment receipt; target and history are being read separately.");
    // Reload targets first; its own follow-up reads the selected history.
    // Concurrent target/history refreshes must not race the same read lane.
    await loadTargets(targetOffset.value, intent.targetId);
  } catch (cause) {
    const status = cause instanceof AdminApiError ? cause.status : undefined, result = classifyCommissionAdjustmentFailure(status);
    if (!active(ticket,captured,session) || ticket !== writeTicket) { if (result === "definitive") clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key); return; }
    if (sessionExpired(cause,captured,session)) { clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key); return; }
    if (result === "unknown") { setPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent); unknown.value = intent; dialog.value = null; writeError.value = t("提交结果未知；原请求正文和幂等键已保留。请不要编辑，按原请求重试。", "The outcome is unknown. The original body and key are retained. Do not edit them; retry the original request."); }
    else if (result === "conflict") { markCommissionAdjustmentConflict(intent); unknown.value = null; conflict.value = intent; dialog.value = null; conflictRefreshed.value = discardChecked.value = false; writeError.value = t("服务器返回 409。请重读目标和历史，核对后勾选丢弃旧请求，才能创建新请求。", "The server returned 409. Reload the target and history; review them and confirm discarding the old request before creating another."); }
    else { clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key); dialog.value = null; writeError.value = cause instanceof Error ? cause.message : t("提交失败。", "Submission failed."); }
  } finally { if (active(ticket,captured,session)) writing.value = false; }
}
async function refreshConflict() {
  const intent = conflict.value; if (!intent || conflictRefreshBusy.value) return;
  targetTicket++; historyTicket++; loading.value = historyLoading.value = false;
  const marker = ++conflictTicket, captured = stamp.value, session = commissionAdjustmentSessionGeneration(), lane = ++readLane; conflictRefreshBusy.value = true; conflictRefreshed.value = false; conflictTargetMissing.value = false; discardChecked.value = false;
  try {
    let offset = 0, targetPage: Awaited<ReturnType<typeof api.listTargets>> | null = null, found = false, targetPageOffset = 0;
    do {
      const page = await api.listTargets(intent.brandId,intent.paymentId,SCAN_PAGE,offset);
      if (marker !== conflictTicket || captured !== stamp.value || lane !== readLane || session !== commissionAdjustmentSessionGeneration() || !rights.value.view) return;
      const targetIndex = page.items.findIndex((item) => item.id === intent.targetId);
      if (targetIndex >= 0) { targetPage = page; targetPageOffset = page.offset + Math.floor(targetIndex / PAGE) * PAGE; found = true; break; }
      offset += page.limit;
      if (!page.items.length || BigInt(offset) >= BigInt(page.total_count)) break;
    } while (true);
    if (!found || !targetPage) { conflictTargetMissing.value = true; return; }
    targetPage = await api.listTargets(intent.brandId,intent.paymentId,PAGE,targetPageOffset);
    if (marker !== conflictTicket || captured !== stamp.value || lane !== readLane || session !== commissionAdjustmentSessionGeneration() || !rights.value.view || !targetPage.items.some((item) => item.id === intent.targetId)) { conflictTargetMissing.value = true; return; }
    const historyPage = await api.listAdjustments(intent.brandId,intent.targetId,PAGE,0);
    if (marker !== conflictTicket || captured !== stamp.value || lane !== readLane || session !== commissionAdjustmentSessionGeneration() || !rights.value.view || historyPage.items.some((item) => item.target_id !== intent.targetId || item.payment_id !== intent.paymentId)) return;
    targets.value = targetPage.items; targetTotal.value = targetPage.total_count; targetOffset.value = targetPage.offset; targetLimit.value = targetPage.limit;
    history.value = historyPage.items; historyTotal.value = historyPage.total_count; historyOffset.value = 0; selectedId.value = intent.targetId; conflictRefreshed.value = true;
  } catch (cause) { if (marker === conflictTicket && captured === stamp.value) { sessionExpired(cause,captured,session); historyError.value = readFailure(cause); } }
  finally { if (marker === conflictTicket) conflictRefreshBusy.value = false; }
}
function discardConflict() {
  const intent = conflict.value; if (!intent || !conflictRefreshed.value || !discardChecked.value || conflictRefreshBusy.value) return;
  clearPendingCommissionAdjustmentIntent(scopeFor(intent.targetId),intent.key); conflict.value = null; conflictRefreshed.value = false; discardChecked.value = false; writeError.value = ""; notice.value = t("已丢弃旧请求。请按当前版本重新审核；新请求会使用新幂等键。", "The old request was discarded. Review the current version; the new request will use a new key.");
}
function cancelDialog() { dialog.value = null; confirmed.value = false; }
watch(stamp, reset, { immediate: true });
onUnmounted(() => { alive = false; generation++; targetTicket++; historyTicket++; writeTicket++; readLane++; });
</script>

<template>
  <section class="commission-adjustments" :data-testid="'commission-adjustments-management'">
    <header><div><h2>{{ t("佣金目标修正", "Commission target adjustments") }}</h2><p>{{ t("按差额调整可用佣金积分；保留原始佣金记录，不修改其他积分来源。", "Applies the difference to available commission points. The original commission record and other point sources are preserved.") }}</p></div><button type="button" data-testid="adjustments-refresh" :disabled="loading" @click="loadTargets(0)">{{ t("刷新", "Refresh") }}</button></header>
    <p v-if="!rights.view" role="status">{{ t("没有佣金查看权限。", "Commission view permission is required.") }}</p>
    <template v-else>
      <p v-if="!rights.write" role="status">{{ t("当前为只读权限。", "Read-only access.") }}</p>
      <p v-if="props.brandStatus === 'disabled'" role="status">{{ t("品牌已禁用，修正操作不可用。", "The brand is disabled; adjustments are unavailable.") }}</p>
      <p v-if="error" role="alert">{{ error }}</p><p v-if="writeError" role="alert">{{ writeError }}</p><p v-if="notice" role="status">{{ notice }}</p>
      <p v-if="unknown" class="pending-intent">{{ t("待确认请求", "Pending request") }} · {{ unknown.targetId }} · {{ unknown.body.points }} · v{{ unknown.body.version }} · {{ unknown.key }} <button type="button" @click="recover">{{ t("使用原请求重试", "Review original request") }}</button></p>
      <div class="target-list" aria-label="Commission targets">
        <button v-for="target in targets" :key="target.id" type="button" class="target-card" :aria-pressed="selectedId === target.id" @click="chooseTarget(target.id)">
          <strong>{{ t("目标", "Target") }} {{ target.id }}</strong><span>{{ t("原始", "Original") }}: {{ target.original_points }}</span><span>{{ t("当前净额", "Adjusted") }}: {{ target.adjusted_points ?? "—" }}</span><span>v{{ target.adjustment_version ?? "—" }}</span>
        </button>
      </div>
      <nav v-if="hasTargetPages" class="pagination" aria-label="Target pages"><button type="button" :disabled="targetOffset === 0 || loading" @click="pageTargets(Math.max(0,targetOffset-targetLimit))">{{ t("上一页", "Previous") }}</button><span>{{ targetOffset + 1 }}–{{ targetOffset + targets.length }} / {{ targetTotal }}</span><button type="button" :disabled="!hasTargetNext || loading" @click="pageTargets(targetOffset+targetLimit)">{{ t("下一页", "Next") }}</button></nav>
      <p v-if="!loading && targets.length === 0">{{ t("没有已派发目标。", "No paid targets.") }}</p>
      <article v-if="selected" class="target-detail">
        <h3>{{ t("目标详情", "Target details") }}</h3>
        <dl><dt>{{ t("原始佣金积分", "Original commission points") }}</dt><dd>{{ selected.original_points }}</dd><dt>{{ t("当前净佣金积分", "Current net commission points") }}</dt><dd>{{ selected.adjusted_points ?? "—" }}</dd><dt>{{ t("修正版本", "Adjustment version") }}</dt><dd>{{ selected.adjustment_version ?? "—" }}</dd><dt>{{ t("派发 ledger ID", "Payout ledger ID") }}</dt><dd>{{ selected.ledger_entry_id ?? "—" }}</dd><dt>{{ t("最后修正 ID", "Last adjustment ID") }}</dt><dd>{{ selected.last_adjustment_id ?? "—" }}</dd></dl>
        <form @submit.prevent="prepare"><label>{{ t("修正后净佣金积分", "Net commission points after adjustment") }}<input v-model="points" inputmode="numeric" autocomplete="off" data-testid="adjustment-points" :disabled="!available"></label><label>{{ t("原因", "Reason") }}<textarea v-model="reason" maxlength="500" data-testid="adjustment-reason" :disabled="!available" /></label><p>{{ t("原始", "Original") }} {{ selected.adjusted_points ?? "—" }} → {{ points || "—" }} · Δ {{ deltaPreview || "—" }}</p><button type="submit" data-testid="adjustment-review" :disabled="!canPrepare">{{ t("复核修正", "Review adjustment") }}</button></form>
      </article>
      <section class="adjustment-history"><h3>{{ t("不可变修正历史", "Immutable adjustment history") }}</h3><p v-if="historyError" role="alert">{{ historyError }}</p><p v-if="historyLoading">{{ t("读取中…", "Loading…") }}</p><p v-else-if="history.length === 0">{{ t("暂无修正记录。", "No adjustment records.") }}</p><article v-for="item in history" :key="item.id" class="history-card"><strong>v{{ item.version }} · {{ item.delta_points }}</strong><dl><dt>{{ t("之前", "Before") }}</dt><dd>{{ item.points_before }}</dd><dt>{{ t("之后", "After") }}</dt><dd>{{ item.points_after }}</dd><dt>{{ t("操作人", "Actor") }}</dt><dd>{{ item.created_by }}</dd><dt>{{ t("原因", "Reason") }}</dt><dd>{{ item.reason }}</dd><dt>Ledger ID</dt><dd>{{ item.ledger_entry_id }}</dd><dt>Audit ID</dt><dd>{{ item.audit_log_id }}</dd></dl></article><nav v-if="hasHistoryPages" class="pagination" aria-label="Adjustment history pages"><button type="button" :disabled="historyOffset === 0 || historyLoading" @click="pageHistory(Math.max(0,historyOffset-PAGE))">{{ t("上一页", "Previous") }}</button><span>{{ historyOffset + 1 }}–{{ historyOffset + history.length }} / {{ historyTotal }}</span><button type="button" :disabled="!hasHistoryNext || historyLoading" @click="pageHistory(historyOffset+PAGE)">{{ t("下一页", "Next") }}</button></nav></section>
      <article v-if="receipt" class="receipt" data-testid="adjustment-receipt"><h3>{{ t("服务器回执", "Server receipt") }}</h3><p>{{ receipt.id }} · v{{ receipt.version }} · {{ receipt.points_before }} → {{ receipt.points_after }} · Δ {{ receipt.delta_points }}</p><p>Ledger {{ receipt.ledger_entry_id }} · Audit {{ receipt.audit_log_id }}</p><p>{{ t("回执是这次写入的凭证；上方当前状态来自独立重读。", "This receipt acknowledges the write; current state above comes from a separate read.") }}</p></article>
      <article v-if="conflict" data-testid="adjustment-conflict" role="alert"><p>{{ t("409 原请求冻结", "409 request frozen") }} · {{ conflict.key }}</p><p v-if="conflictRefreshed">{{ t("目标和历史已成功重读，请复核后明确丢弃。", "Target and history were reloaded. Review them before discarding the old request.") }}</p><p v-if="conflictTargetMissing">{{ t("未在目标分页中找到原目标；无法授权丢弃。", "The original target was not found across target pages; discard remains unavailable.") }}</p><button type="button" :disabled="conflictRefreshBusy" @click="refreshConflict">{{ t("重读目标和历史", "Reload target and history") }}</button><label><input type="checkbox" v-model="discardChecked" :disabled="!conflictRefreshed || conflictRefreshBusy">{{ t("我已核对并确认丢弃409原请求", "I reviewed the state and confirm discarding the 409 request") }}</label><button type="button" :disabled="!conflictRefreshed || !discardChecked || conflictRefreshBusy" @click="discardConflict">{{ t("确认丢弃并允许重新审核", "Confirm discard and allow a new review") }}</button></article>
    </template>
    <div v-if="dialog" class="review-backdrop" role="presentation"><section class="review-dialog" role="dialog" aria-modal="true" data-testid="adjustment-review-dialog"><h3>{{ t("确认佣金修正", "Confirm commission adjustment") }}</h3><dl><dt>{{ t("操作人", "Actor") }}</dt><dd>{{ dialog.actorId }}</dd><dt>{{ t("品牌", "Brand") }}</dt><dd>{{ dialog.brandId }}</dd><dt>{{ t("目标", "Target") }}</dt><dd>{{ dialog.targetId }}</dd><dt>{{ t("修正前（冻结请求）", "Before (frozen request)") }}</dt><dd>{{ dialog.pointsBefore }}</dd><dt>{{ t("修正后", "After") }}</dt><dd>{{ dialog.body.points }}</dd><dt>Δ</dt><dd>{{ (BigInt(dialog.body.points) - BigInt(dialog.pointsBefore)).toString() }}</dd><dt>{{ t("原因", "Reason") }}</dt><dd>{{ dialog.body.reason }}</dd><dt>{{ t("正文版本", "Body version") }}</dt><dd>{{ dialog.body.version }}</dd><dt>Idempotency-Key</dt><dd data-testid="adjustment-review-key">{{ dialog.key }}</dd></dl><label><input type="checkbox" v-model="confirmed" data-testid="adjustment-confirmed">{{ t("我已核对上述请求并确认提交", "I reviewed this request and confirm submission") }}</label><button type="button" data-testid="adjustment-confirm-submit" :disabled="!confirmed || writing" @click="submit">{{ t("提交修正", "Submit adjustment") }}</button><button type="button" @click="cancelDialog">{{ t("取消", "Cancel") }}</button></section></div>
    <small>{{ targetTotal }} · {{ historyTotal }}</small>
  </section>
</template>

<style scoped>
.commission-adjustments button{min-height:40px;border:1px solid #c5ceda;border-radius:8px;padding:8px 12px;font:inherit;color:inherit;background:#fff;cursor:pointer;overflow-wrap:anywhere}
.commission-adjustments button:disabled{opacity:.55;cursor:not-allowed}
.commission-adjustments input:not([type=checkbox]),.commission-adjustments textarea{box-sizing:border-box;width:100%;min-height:40px;border:1px solid #c5ceda;border-radius:8px;padding:9px 10px;font:inherit;background:#fff;color:inherit}
.commission-adjustments textarea{resize:vertical;min-height:88px}
.commission-adjustments h2,.commission-adjustments h3{margin:0 0 .6rem}.commission-adjustments header p{color:#667085;line-height:1.55;margin:.35rem 0 0}
.commission-adjustments .target-card,.commission-adjustments .target-detail,.commission-adjustments .history-card,.commission-adjustments .receipt{border-color:#d9e0ea;border-radius:12px;background:#fff}
.commission-adjustments .target-card strong{overflow-wrap:anywhere}.commission-adjustments .target-card[aria-pressed=true]{background:#f4f8ff}
.commission-adjustments dt{color:#667085;font-size:.86rem}.commission-adjustments dd{line-height:1.5}
.commission-adjustments .adjustment-history{display:grid;gap:12px;grid-template-columns:repeat(auto-fit,minmax(min(100%,18rem),1fr))}
.commission-adjustments .adjustment-history>h3,.commission-adjustments .adjustment-history>p,.commission-adjustments .adjustment-history>.pagination{grid-column:1/-1}
.commission-adjustments .receipt{overflow-wrap:anywhere}
.commission-adjustments{display:grid;gap:1rem;color:inherit}.commission-adjustments header{display:flex;align-items:flex-start;justify-content:space-between;gap:1rem}.target-list{display:grid;grid-template-columns:repeat(auto-fit,minmax(14rem,1fr));gap:.75rem}.target-card{display:grid;text-align:left;gap:.35rem;padding:.8rem;border:1px solid #8b8b8b;border-radius:.5rem;background:transparent;color:inherit}.target-card[aria-pressed=true]{outline:2px solid #2563eb}.target-detail,.history-card,.receipt,.review-dialog,.adjustment-conflict{border:1px solid #8b8b8b;border-radius:.5rem;padding:1rem}.target-detail dl,.history-card dl,.review-dialog dl{display:grid;grid-template-columns:minmax(9rem,auto) minmax(0,1fr);gap:.35rem .8rem}.target-detail dd,.history-card dd,.review-dialog dd{margin:0;overflow-wrap:anywhere}.target-detail form{display:grid;gap:.8rem;max-width:38rem}.target-detail label{display:grid;gap:.3rem}.pagination{display:flex;align-items:center;justify-content:center;gap:1rem}.review-backdrop{position:fixed;inset:0;z-index:50;background:#0008;display:grid;place-items:center;padding:1rem}.review-dialog{width:min(42rem,100%);max-height:90vh;overflow:auto;background:Canvas;color:CanvasText;display:grid;gap:.75rem}.adjustment-conflict{display:grid;gap:.7rem}.pending-intent{overflow-wrap:anywhere}@media(max-width:600px){.commission-adjustments header{display:grid}.target-detail dl,.history-card dl,.review-dialog dl{grid-template-columns:1fr}.target-detail dd,.history-card dd,.review-dialog dd{margin-bottom:.35rem}}
</style>
