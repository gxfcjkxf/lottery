<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import { useAdminI18n } from "./i18n";
import type { LocalizedMessage } from "@lottery/shared";
import { createReportArchivesApi, reportArchivesPermissions, type ReportArchiveRecord } from "./report-archives-api";
import { createArchiveIntent, getArchiveIntent, setArchiveIntent, clearArchiveIntent, markArchiveConflict, isArchiveConflict, clearAllArchiveIntents, archiveSessionGeneration, archiveIntentChanges, classifyArchiveWriteFailure, type ArchiveIntent } from "./report-archives-state";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message } = useAdminI18n();
const api = createReportArchivesApi(), pageSize = 20;
const rights = computed(() => reportArchivesPermissions(props.account, props.brandId));
const scope = computed(() => JSON.stringify([props.account.id, props.brandId, props.account.super_admin, props.account.brand_ids, props.account.permissions_by_brand, props.account.platform_permissions, rights.value]));
const items = ref<ReportArchiveRecord[]>([]), total = ref("0"), offset = ref(0);
const selected = ref<ReportArchiveRecord | null>(null), receipt = ref<ReportArchiveRecord | null>(null);
const kind = ref<"daily" | "monthly">("daily"), periodKey = ref(""), expectedRevision = ref(0), reason = ref("");
const review = ref<ArchiveIntent | null>(null), confirmed = ref(false), pending = ref<ArchiveIntent | null>(null);
const conflict = ref(false), conflictRefreshed = ref(false), discardChecked = ref(false);
const loading = ref(false), reading = ref(false), writing = ref(false), downloading = ref(false);
const error = ref<string | LocalizedMessage>(""), notice = ref<string | LocalizedMessage>("");
const listError = ref<string | LocalizedMessage>(""), recordError = ref<string | LocalizedMessage>("");
let alive = true, listTicket = 0, readTicket = 0, writeTicket = 0, downloadTicket = 0;
const input = () => ({ kind: kind.value, period_key: periodKey.value, expected_revision: expectedRevision.value, reason: reason.value });
const formValid = computed(() => createArchiveIntent(props.account.id, props.brandId, input(), "archive-validation-key") !== null);
const canCreate = computed(() => rights.value.create && !writing.value && !pending.value && formValid.value);
const nextPage = computed(() => BigInt(offset.value) + BigInt(pageSize) < BigInt(total.value) && offset.value + pageSize <= 1_000_000);
const blocks = computed(() => selected.value ? [
  { name: t("投注状态观察", "Betting state observation"), values: selected.value.snapshot.betting },
  { name: t("实际积分入账", "Actual ledger postings"), values: selected.value.snapshot.ledger },
  { name: t("观察时钱包余额", "Wallet balances at observation"), values: selected.value.snapshot.wallet_snapshot.balances },
  { name: t("提现申请状态", "Withdrawal request states"), values: selected.value.snapshot.withdrawals },
  { name: t("实际佣金入账", "Actual commission postings"), values: selected.value.snapshot.commissions },
  { name: t("实际奖励入账", "Actual reward postings"), values: selected.value.snapshot.rewards },
  { name: t("奖励订单状态", "Reward order states"), values: selected.value.snapshot.reward_orders },
] : []);
const metricLabels: Record<string, [string, string]> = {
  order_count: ["记录数量", "Record count"], stake_points: ["投注积分", "Stake points"],
  placed_count: ["待结算注单数", "Unsettled orders"], won_count: ["中奖注单数", "Winning orders"], lost_count: ["未中奖注单数", "Losing orders"],
  abnormal_count: ["异常注单数", "Abnormal orders"], cancelled_count: ["取消记录数", "Cancelled records"], refund_points: ["退款积分", "Refund points"],
  settled_stake_points: ["已最终结算投注积分", "Finalized stake points"], unfinalized_stake_points: ["未最终结算投注积分", "Unfinalized stake points"],
  abnormal_stake_points: ["异常投注积分", "Abnormal stake points"], current_prize_points: ["当前派奖积分", "Current prize points"], correction_open_count: ["更正处理中注单数", "Orders under correction"],
  entry_count: ["入账笔数", "Posting count"], net_points: ["入账净积分", "Net posted points"], recharge_points: ["充值入账积分", "Recharge credits"],
  prize_credit_points: ["派奖入账积分", "Prize credits"], prize_reversal_points: ["派奖冲回积分", "Prize reversals"],
  account_count: ["钱包账户数", "Wallet accounts"], available_points: ["可用积分", "Available points"], frozen_points: ["冻结积分", "Frozen points"],
  withdrawal_points: ["提现预留积分", "Withdrawal reserved points"], total_points: ["显示总积分", "Total displayed points"],
  requested_points: ["申请积分", "Requested points"], reviewing_count: ["审核中笔数", "Under review count"], reviewing_points: ["审核中积分", "Under review points"],
  processing_count: ["提现处理中笔数", "Processing count"], processing_points: ["提现处理中积分", "Processing points"],
  paid_count: ["已完成笔数", "Completed count"], paid_points: ["已完成积分", "Completed points"], rejected_count: ["驳回笔数", "Rejected count"], rejected_points: ["驳回积分", "Rejected points"],
  failed_count: ["失败笔数", "Failed count"], failed_points: ["失败积分", "Failed points"], cancelled_points: ["取消积分", "Cancelled points"],
  paid_entry_count: ["原派发入账笔数", "Original payout postings"], adjustment_entry_count: ["人工修正笔数", "Adjustment postings"],
  adjustment_credit_points: ["人工补发积分", "Adjustment credits"], adjustment_debit_points: ["人工扣回积分", "Adjustment debits"],
  correction_entry_count: ["结果更正入账笔数", "Correction postings"], correction_credit_points: ["结果更正补发积分", "Correction credits"], correction_debit_points: ["结果更正扣回积分", "Correction debits"],
  grant_entry_count: ["发放入账笔数", "Grant postings"], grant_points: ["发放积分", "Granted points"], reversal_entry_count: ["撤销入账笔数", "Reversal postings"], reversal_points: ["撤销冲回积分", "Reversed points"],
  original_points: ["原奖励积分", "Original reward points"], granted_count: ["已发放订单数", "Granted orders"], granted_points: ["已发放积分", "Granted points"],
  pending_count: ["撤销待处理订单数", "Pending revocation orders"], pending_points: ["撤销待处理积分", "Pending revocation points"], revoked_count: ["已撤销订单数", "Revoked orders"], revoked_points: ["已撤销积分", "Revoked points"],
};
function metricLabel(field: string) { const labels = metricLabels[field]; return labels ? t(...labels) : field; }

function refreshPending() {
  pending.value = getArchiveIntent(props.account.id, props.brandId);
  conflict.value = pending.value !== null && isArchiveConflict(pending.value);
}
function current(ticket: number, lane: "list" | "read" | "write" | "download", captured: string, generation: number) {
  const actual = lane === "list" ? listTicket : lane === "read" ? readTicket : lane === "write" ? writeTicket : downloadTicket;
  return alive && actual === ticket && captured === scope.value && generation === archiveSessionGeneration();
}
function sessionInvalid() { clearAllArchiveIntents(); emit("session-invalid"); }
function readFailure(problem: unknown) {
  if (problem instanceof AdminApiError && problem.status === 401) { sessionInvalid(); return; }
  return problem instanceof Error ? problem.message : message("归档读取失败，请重新读取。", "Archive read failed. Reload it.");
}
async function loadList(next = offset.value) {
  if (!rights.value.view) return false;
  const ticket = ++listTicket, captured = scope.value, generation = archiveSessionGeneration();
  loading.value = true; listError.value = "";
  try {
    const page = await api.list(props.brandId, pageSize, next);
    if (!current(ticket, "list", captured, generation)) return false;
    items.value = page.items; total.value = page.total_count; offset.value = page.offset; return true;
  } catch (problem) { if (current(ticket, "list", captured, generation)) listError.value = readFailure(problem) ?? ""; return false; }
  finally { if (current(ticket, "list", captured, generation)) loading.value = false; }
}
async function loadRecord(id: string) {
  if (!rights.value.view) return false;
  const ticket = ++readTicket, captured = scope.value, generation = archiveSessionGeneration();
  downloadTicket++; downloading.value = false; selected.value = null; reading.value = true; recordError.value = "";
  try {
    const record = await api.read(props.brandId, id);
    if (!current(ticket, "read", captured, generation)) return false;
    selected.value = record; return true;
  } catch (problem) { if (current(ticket, "read", captured, generation)) recordError.value = readFailure(problem) ?? ""; return false; }
  finally { if (current(ticket, "read", captured, generation)) reading.value = false; }
}
function beginReview(replay = false) {
  if (!rights.value.create || writing.value || conflict.value) return;
  const intent = replay ? getArchiveIntent(props.account.id, props.brandId) : canCreate.value ? createArchiveIntent(props.account.id, props.brandId, input(), createIdempotencyKey()) : null;
  if (!intent) return;
  review.value = intent; confirmed.value = false; error.value = "";
}
function appendVersion() {
  if (!selected.value || pending.value || writing.value || !rights.value.create) return;
  kind.value = selected.value.window.kind; periodKey.value = selected.value.window.period_key; expectedRevision.value = selected.value.revision;
}
async function submit() {
  const intent = review.value;
  if (!intent || !confirmed.value || !rights.value.create || writing.value || conflict.value || !setArchiveIntent(intent)) return;
  const ticket = ++writeTicket, captured = scope.value, generation = archiveSessionGeneration();
  refreshPending(); writing.value = true; review.value = null; confirmed.value = false; error.value = ""; notice.value = "";
  try {
    const record = await api.create(intent.brandId, intent.body, intent.key, intent.actorId);
    if (!current(ticket, "write", captured, generation)) return;
    clearArchiveIntent(intent.actorId, intent.brandId, intent.key); refreshPending(); receipt.value = record;
    notice.value = message("服务器已确认原回执；列表与详情将独立重新读取。", "The server confirmed the original receipt. The list and detail are reloaded separately.");
    await Promise.all([loadList(offset.value), loadRecord(record.id)]);
  } catch (problem) {
    // A late result must never clear or mark an intent from a later login.
    if (generation !== archiveSessionGeneration()) return;
    const status = problem instanceof AdminApiError ? problem.status : undefined;
    if (status === 401) { if (current(ticket, "write", captured, generation)) sessionInvalid(); return; }
    const classification = classifyArchiveWriteFailure(status);
    if (classification === "conflict") markArchiveConflict(intent);
    else if (classification === "definitive") clearArchiveIntent(intent.actorId, intent.brandId, intent.key);
    if (!current(ticket, "write", captured, generation)) return;
    refreshPending(); conflictRefreshed.value = false; discardChecked.value = false;
    error.value = classification === "unknown"
      ? message("写入结果未知。原输入和幂等键已保留；刷新不能确认此请求，请核对后重放原请求。", "Write result unknown. The original input and idempotency key are retained. Refreshing cannot confirm it; review and replay the original request.")
      : classification === "conflict" ? message("版本或请求冲突。必须重新读取相关状态并人工确认丢弃原请求。", "Version or request conflict. Reload related state and explicitly confirm discarding the original request.")
      : problem instanceof Error ? problem.message : message("服务器拒绝此请求。", "The server rejected this request.");
  } finally { if (current(ticket, "write", captured, generation)) writing.value = false; }
}
async function refreshConflict() {
  if (!pending.value || !conflict.value || loading.value || reading.value) return;
  const captured = scope.value, generation = archiveSessionGeneration(), key = pending.value.key, selectedID = selected.value?.id;
  conflictRefreshed.value = false; discardChecked.value = false;
  const results = await Promise.all([loadList(offset.value), selectedID ? loadRecord(selectedID) : Promise.resolve(true)]);
  if (alive && captured === scope.value && generation === archiveSessionGeneration() && pending.value?.key === key) conflictRefreshed.value = results.every(Boolean);
}
function discardConflict() {
  if (!pending.value || !conflict.value || !conflictRefreshed.value || !discardChecked.value || writing.value) return;
  clearArchiveIntent(pending.value.actorId, pending.value.brandId, pending.value.key); refreshPending(); error.value = ""; conflictRefreshed.value = discardChecked.value = false;
}
async function download() {
  if (!selected.value || !rights.value.download || downloading.value) return;
  // Vue wraps selected records in a Proxy, which structuredClone cannot copy.
  // The closed DTO contains strings and safe revision numbers, never BigInt.
  const record = JSON.parse(JSON.stringify(selected.value)) as ReportArchiveRecord, ticket = ++downloadTicket, captured = scope.value, generation = archiveSessionGeneration();
  downloading.value = true; error.value = "";
  try {
    const file = await api.download(props.brandId, record.id, record);
    if (!current(ticket, "download", captured, generation) || selected.value?.id !== record.id || !rights.value.download) return;
    const url = URL.createObjectURL(new Blob([file.bytes.slice().buffer], { type: "application/json; charset=utf-8" }));
    const link = document.createElement("a"); link.href = url; link.download = file.metadata.filename; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    notice.value = message("完整文件长度与 SHA-256 已验证，已开始下载。", "The complete file length and SHA-256 were verified. Download started.");
  } catch (problem) { if (current(ticket, "download", captured, generation)) error.value = readFailure(problem) ?? ""; }
  finally { if (current(ticket, "download", captured, generation)) downloading.value = false; }
}
function reset() {
  listTicket++; readTicket++; writeTicket++; downloadTicket++;
  items.value = []; total.value = "0"; offset.value = 0; selected.value = receipt.value = null;
  loading.value = reading.value = writing.value = downloading.value = false;
  kind.value = "daily"; periodKey.value = ""; expectedRevision.value = 0; reason.value = "";
  review.value = null; confirmed.value = conflictRefreshed.value = discardChecked.value = false; error.value = notice.value = listError.value = recordError.value = ""; refreshPending();
}
watch(scope, () => { reset(); if (rights.value.view) void loadList(0); }, { immediate: true });
let observedGeneration = archiveSessionGeneration();
watch(archiveIntentChanges, () => {
  if (observedGeneration !== archiveSessionGeneration()) { observedGeneration = archiveSessionGeneration(); reset(); }
  else refreshPending();
});
onBeforeUnmount(() => { alive = false; listTicket++; readTicket++; writeTicket++; downloadTicket++; });
</script>

<template>
  <section class="archives panel">
    <header><div><h2>{{ t("日月归档", "Daily and monthly archives") }}</h2><p>{{ t("按品牌时区保存不可变观察，后续更正追加版本。归档不改变积分，也不是停止记账的封账。", "Immutable observations by brand timezone; corrections append versions. Archiving does not change points or close financial posting.") }}</p></div><button class="button button-secondary" data-testid="archive-refresh" :disabled="!rights.view || loading" @click="loadList()">{{ t("刷新归档", "Refresh archives") }}</button></header>
    <p v-if="!rights.view" role="alert">{{ t("没有此品牌的归档查看权限。", "No archive viewing permission for this brand.") }}</p>
    <template v-else>
      <p v-if="error" role="alert">{{ t(error) }}</p><p v-if="notice" role="status">{{ t(notice) }}</p>
      <p v-if="listError" role="alert" data-testid="archive-list-error">{{ t("归档列表读取失败：", "Archive list read failed:") }} {{ t(listError) }}</p>
      <p v-if="recordError" role="alert" data-testid="archive-record-error">{{ t("所选归档读取失败：", "Selected archive read failed:") }} {{ t(recordError) }}</p>
      <div v-if="pending" data-testid="archive-unknown" class="archive-warning">
        <h3>{{ conflict ? t("原请求存在冲突", "Original request conflict") : t("尚未确定的创建请求", "Unresolved creation request") }}</h3>
        <p>{{ t("刷新或离页不能清除原请求；新创建已锁定。这里只保留当前登录会话内的请求，不写入浏览器存储。", "Refreshing or leaving does not clear the original request. New creation is locked. The request is kept only in the current login session, not browser storage.") }}</p>
        <dl><dt>{{ t("周期", "Period") }}</dt><dd>{{ pending.body.kind }} / {{ pending.body.period_key }}</dd><dt>{{ t("确认版本", "Confirmed revision") }}</dt><dd>{{ pending.body.expected_revision }}</dd><dt>{{ t("操作原因", "Reason for operation") }}</dt><dd>{{ pending.body.reason }}</dd><dt>{{ t("原幂等键", "Original idempotency key") }}</dt><dd>{{ pending.key }}</dd></dl>
        <button v-if="!conflict" class="button button-secondary" data-testid="archive-replay" :disabled="!rights.create || writing" @click="beginReview(true)">{{ t("核对并重放原请求", "Review and replay original request") }}</button>
        <div v-else data-testid="archive-conflict"><button class="button button-secondary" data-testid="archive-conflict-refresh" :disabled="loading || reading" @click="refreshConflict">{{ t("重新读取相关状态", "Reload related state") }}</button><label><input v-model="discardChecked" data-testid="archive-discard-check" type="checkbox" :disabled="!conflictRefreshed" />{{ t("我已核对重新读取的列表及所选归档，确认丢弃冲突原请求。", "I reviewed the reloaded list and selected archive and confirm discarding the conflicting request.") }}</label><button class="button button-secondary" data-testid="archive-discard" :disabled="!conflictRefreshed || !discardChecked || writing" @click="discardConflict">{{ t("确认丢弃冲突原请求", "Confirm discarding conflicting request") }}</button></div>
      </div>
      <div class="archive-form">
        <h3>{{ t("创建新的观察版本", "Create a new observation version") }}</h3>
        <p>{{ t("仅可归档已经结束的日或月。首版确认版本为 0；追加时须核对原范围的最新版本，过期版本会被拒绝。", "Only elapsed days or months can be archived. Use confirmed revision 0 for the first version; verify the latest revision for an append. Stale revisions are rejected.") }}</p>
        <label>{{ t("归档类型", "Archive type") }}<select v-model="kind" data-testid="archive-kind" :aria-label="t('归档类型', 'Archive type')" :disabled="!!pending || writing || !!review"><option value="daily">{{ t("日归档", "Daily") }}</option><option value="monthly">{{ t("月归档", "Monthly") }}</option></select></label>
        <label>{{ t("归档周期", "Archive period") }}<input v-model="periodKey" data-testid="archive-period-key" :aria-label="t('归档周期', 'Archive period')" :placeholder="kind === 'daily' ? 'YYYY-MM-DD' : 'YYYY-MM'" :disabled="!!pending || writing || !!review" /></label>
        <label>{{ t("确认的原最新版本", "Confirmed previous latest revision") }}<input v-model.number="expectedRevision" data-testid="archive-expected-revision" :aria-label="t('确认的原最新版本', 'Confirmed previous latest revision')" type="number" min="0" step="1" :disabled="!!pending || writing || !!review" /></label>
        <label class="archive-wide">{{ t("操作原因", "Reason for operation") }}<textarea v-model="reason" data-testid="archive-reason" :aria-label="t('操作原因', 'Reason for operation')" :disabled="!!pending || writing || !!review" /></label>
        <button class="button" data-testid="archive-review-button" :disabled="!canCreate" @click="beginReview()">{{ t("核对并创建归档", "Review and create archive") }}</button>
        <p v-if="!rights.create">{{ t("当前账号仅可查看，不能创建归档。", "This account can view but cannot create archives.") }}</p>
      </div>
      <div v-if="receipt" data-testid="archive-receipt" class="archive-receipt"><h3>{{ t("原提交回执", "Original submission receipt") }}</h3><p>{{ receipt.id }} · v{{ receipt.revision }}</p><p>{{ receipt.reason }}</p><p>{{ t("这是原请求的服务器确认，不会被后来版本或查询失败替换。", "This confirms the original request and is not replaced by later versions or a failed query.") }}</p></div>
      <div class="archive-list" data-testid="archive-list"><h3>{{ t("归档历史", "Archive history") }}</h3><p>{{ t("总记录数", "Total records") }}: {{ total }} · {{ t("偏移", "Offset") }}: {{ offset }}</p><p v-if="loading" role="status">{{ t("读取中", "Loading") }}</p><p v-else-if="items.length === 0">{{ t("当前页没有归档记录。", "No archive records on this page.") }}</p><ul><li v-for="item in items" :key="item.id"><button class="button button-secondary" :disabled="reading" @click="loadRecord(item.id)">{{ item.window.period_key }} · {{ item.window.kind }} · v{{ item.revision }}<small>{{ item.id }}</small></button><small>{{ item.snapshot_at }}</small><small>{{ item.reason }}</small></li></ul><div class="archive-actions"><button class="button button-secondary" :disabled="loading || offset === 0" @click="loadList(Math.max(0, offset - pageSize))">{{ t("上一页", "Previous page") }}</button><button class="button button-secondary" :disabled="loading || !nextPage" @click="loadList(offset + pageSize)">{{ t("下一页", "Next page") }}</button></div></div>
      <p v-if="reading" role="status">{{ t("正在读取所选归档", "Loading selected archive") }}</p>
      <div v-if="selected" data-testid="archive-detail" class="archive-detail">
        <h3>{{ t("所选不可变归档", "Selected immutable archive") }}</h3>
        <dl>
          <dt>ID</dt><dd>{{ selected.id }}</dd>
          <dt>{{ t("版本", "Revision") }}</dt><dd>{{ selected.revision }}</dd>
          <dt>{{ t("上版 ID", "Previous ID") }}</dt><dd>{{ selected.previous_id ?? '—' }}</dd>
          <dt>{{ t("品牌时区", "Brand timezone") }}</dt><dd>{{ selected.window.timezone }}</dd>
          <dt>{{ t("原时间范围", "Original interval") }}</dt><dd>[{{ selected.window.from }}, {{ selected.window.to }})</dd>
          <dt>{{ t("观察时刻", "Observation time") }}</dt><dd>{{ selected.snapshot_at }}</dd>
          <dt>SHA-256</dt><dd>{{ selected.payload_sha256 }}</dd>
          <dt>{{ t("创建人", "Created by") }}</dt><dd>{{ selected.created_by }}</dd>
          <dt>{{ t("审计 ID", "Audit ID") }}</dt><dd>{{ selected.audit_log_id }}</dd>
          <dt>{{ t("操作原因", "Reason for operation") }}</dt><dd>{{ selected.reason }}</dd>
        </dl>
        <p>{{ t("钱包是观察时刻的余额，不是原周期日末余额。财务按实际入账时间，业务订单按原创建范围观察状态。", "Wallet balances are from the observation time, not the original period end. Financial totals use posting time; business states are observed for the original creation cohort.") }}</p>
        <div class="archive-actions">
          <button class="button button-secondary" :disabled="!rights.create || !!pending || writing" @click="appendVersion">{{ t("以此范围准备追加版本", "Prepare an append for this interval") }}</button>
          <button class="button" data-testid="archive-download" :disabled="!rights.download || downloading" @click="download">{{ t("校验并下载原 JSON", "Verify and download original JSON") }}</button>
        </div>
        <div class="archive-blocks">
          <section v-for="block in blocks" :key="block.name">
            <h4>{{ block.name }}</h4>
            <dl><template v-for="(value, field) in block.values" :key="field"><dt>{{ metricLabel(field) }}</dt><dd>{{ value }}</dd></template></dl>
          </section>
        </div>
      </div>
    </template>
    <div v-if="review" class="archive-modal" role="dialog" aria-modal="true" :aria-label="t('确认归档创建', 'Confirm archive creation')" data-testid="archive-review"><div><h3>{{ t("确认归档创建", "Confirm archive creation") }}</h3><dl><dt>{{ t("品牌", "Brand") }}</dt><dd>{{ review.brandId }}</dd><dt>{{ t("提交管理员", "Submitting administrator") }}</dt><dd>{{ review.actorId }}</dd><dt>{{ t("周期", "Period") }}</dt><dd>{{ review.body.kind }} / {{ review.body.period_key }}</dd><dt>{{ t("确认版本", "Confirmed revision") }}</dt><dd>{{ review.body.expected_revision }} → {{ review.body.expected_revision + 1 }}</dd><dt>{{ t("操作原因", "Reason for operation") }}</dt><dd>{{ review.body.reason }}</dd><dt>{{ t("原幂等键", "Original idempotency key") }}</dt><dd>{{ review.key }}</dd></dl><label><input v-model="confirmed" type="checkbox" data-testid="archive-confirm" />{{ t("我已核对品牌、周期、版本和原因，确认提交此原请求。", "I reviewed the brand, period, revision and reason and confirm submitting this original request.") }}</label><div class="archive-actions"><button class="button button-secondary" @click="review = null">{{ t("取消", "Cancel") }}</button><button class="button" data-testid="archive-submit" :disabled="!confirmed || writing || !rights.create" @click="submit">{{ t("确认并提交", "Confirm and submit") }}</button></div></div></div>
  </section>
</template>

<style scoped>
.archives{min-width:0;overflow-wrap:anywhere;padding:1.25rem}.archives header{display:flex;justify-content:space-between;align-items:flex-start;gap:1rem}.archives p{line-height:1.6;color:var(--muted,#64748b)}.archives label{display:grid;gap:.5rem}.archives input,.archives select,.archives textarea{width:100%;min-width:0;box-sizing:border-box;padding:.65rem;border:1px solid var(--border,#d5dce2);border-radius:.5rem;background:var(--surface,#fff);color:inherit}.archives input[type=checkbox]{width:auto;justify-self:start}.archives textarea{min-height:5rem}.archive-form{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:1rem;margin:1.5rem 0}.archive-form h3,.archive-form>p,.archive-wide{grid-column:1/-1}.archives dl{display:grid;grid-template-columns:minmax(100px,1fr) minmax(0,2fr);gap:.5rem 1rem}.archives dt{color:var(--muted,#64748b)}.archives dd{margin:0;min-width:0;white-space:pre-wrap;overflow-wrap:anywhere}.archive-warning,.archive-receipt{padding:1rem;border:1px solid #d5b05b;border-radius:.75rem;background:#fbf7ed;margin:1rem 0}.archive-list ul{list-style:none;padding:0;display:grid;gap:.75rem}.archive-list li{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:.5rem;padding:.75rem;border:1px solid var(--border,#d5dce2);border-radius:.65rem}.archive-list small{white-space:normal;overflow-wrap:anywhere}.archive-list li small:last-child{grid-column:1/-1}.archive-actions{display:flex;gap:.75rem;flex-wrap:wrap;align-items:center}.archives button{white-space:normal;max-width:100%}.archive-blocks{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem;margin-top:1rem}.archive-blocks section{border:1px solid var(--border,#d5dce2);border-radius:.65rem;padding:1rem;min-width:0}.archive-blocks dl{grid-template-columns:minmax(0,1.5fr) minmax(0,1fr)}.archive-modal{position:fixed;inset:0;background:#17253699;z-index:90;display:flex;align-items:center;justify-content:center;padding:1rem}.archive-modal>div{background:var(--surface,#fff);border-radius:1rem;padding:1.25rem;width:36rem;max-width:100%;max-height:85dvh;overflow:auto}.archive-modal .archive-actions{margin-top:1rem}@media(max-width:700px){.archives{padding:1rem}.archives header{display:block}.archive-form,.archive-blocks{grid-template-columns:minmax(0,1fr)}.archive-list li{grid-template-columns:minmax(0,1fr)}.archives dl{grid-template-columns:minmax(0,1fr)}.archives dt{font-size:.85rem}.archive-blocks dl{grid-template-columns:minmax(0,1.5fr) minmax(0,1fr)}.archive-modal{padding:.5rem}.archive-modal>div{padding:1rem}}
</style>
