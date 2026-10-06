<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, createIdempotencyKey, type AdminAccount } from "./admin-api";
import {
  createReconciliationApi, reconciliationPermissions,
  type ReconciliationJob, type ReconciliationOutcome, type ReconciliationTarget,
} from "./reconciliation-api";
import {
  classifyReconciliationWriteFailure, clearPendingReconciliationWrite, freezeReconciliationBody,
  getPendingReconciliationWrite, listPendingReconciliationWrites, reconciliationSessionGeneration,
  setPendingReconciliationWrite, type PendingReconciliationWrite, type ReconciliationWriteScope,
} from "./reconciliation-state";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const PAGE_SIZE = 20;
const api = createReconciliationApi();
const rights = computed(() => reconciliationPermissions(props.account, props.brandId));
const jobs = ref<ReconciliationJob[]>([]), jobTotal = ref("0"), offset = ref(0);
const job = ref<ReconciliationJob | null>(null), receipt = ref<ReconciliationJob | null>(null);
const targets = ref<ReconciliationTarget[]>([]), targetTotal = ref("0"), targetOffset = ref(0);
const outcomeFilter = ref<ReconciliationOutcome | "">("");
const reason = ref(""), retryReason = ref("");
const review = ref<PendingReconciliationWrite | null>(null), confirmed = ref(false);
const pendingCreate = ref<PendingReconciliationWrite | null>(null), pendingRetries = ref<PendingReconciliationWrite[]>([]);
const loading = ref(false), detailLoading = ref(false), targetLoading = ref(false), writing = ref(false);
const listError = ref(""), detailError = ref(""), targetError = ref(""), writeError = ref(""), notice = ref("");
let alive = true, listTicket = 0, detailTicket = 0, targetTicket = 0, writeTicket = 0;

const permissionScope = computed(() => JSON.stringify([props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand, props.account.platform_permissions, props.brandId, rights.value]));
const createScope = computed<ReconciliationWriteScope>(() => ({ accountId: props.account.id, brandId: props.brandId, operation: "create" }));
const retryScope = computed<ReconciliationWriteScope | null>(() => job.value ? ({ accountId: props.account.id, brandId: props.brandId, operation: "retry", jobId: job.value.id }) : null);
const reasonValid = computed(() => validReasonText(reason.value));
const retryReasonValid = computed(() => validReasonText(retryReason.value));
const reasonByteLength = computed(() => new TextEncoder().encode(reason.value).length);
const canCreate = computed(() => rights.value.run && !props.account.super_admin && !writing.value && !pendingCreate.value && reasonValid.value);
const canRetry = computed(() => Boolean(rights.value.retry && !props.account.super_admin && job.value?.state === "failed" && job.value.can_retry &&
  retryScope.value && !getPendingReconciliationWrite(retryScope.value) && retryReasonValid.value && !writing.value));
const totalJobPages = computed(() => Math.max(1, Math.ceil(Number(jobTotal.value) / PAGE_SIZE)));
const totalTargetPages = computed(() => Math.max(1, Math.ceil(Number(targetTotal.value) / PAGE_SIZE)));

function current(ticket: number, lane: "list" | "detail" | "targets", scope: string, generation: number): boolean {
  const laneTicket = lane === "list" ? listTicket : lane === "detail" ? detailTicket : targetTicket;
  return alive && ticket === laneTicket && scope === permissionScope.value && generation === reconciliationSessionGeneration() && rights.value.view;
}
function currentWrite(ticket: number, scope: string, generation: number): boolean {
  return alive && ticket === writeTicket && scope === permissionScope.value && generation === reconciliationSessionGeneration();
}
function refreshPending() {
  const intents = listPendingReconciliationWrites(props.account.id, props.brandId);
  pendingCreate.value = intents.find((item) => item.operation === "create") ?? null;
  pendingRetries.value = intents.filter((item) => item.operation === "retry");
}
function resetView() {
  listTicket++; detailTicket++; targetTicket++; writeTicket++;
  loading.value = detailLoading.value = targetLoading.value = writing.value = false;
  jobs.value = []; jobTotal.value = "0"; job.value = null; receipt.value = null;
  targets.value = []; targetTotal.value = "0"; offset.value = targetOffset.value = 0;
  outcomeFilter.value = ""; listError.value = detailError.value = targetError.value = writeError.value = notice.value = "";
  review.value = null; confirmed.value = false; refreshPending();
}
async function loadList(next = offset.value) {
  if (!rights.value.view) { resetView(); return false; }
  const ticket = ++listTicket, captured = permissionScope.value, generation = reconciliationSessionGeneration();
  loading.value = true; listError.value = "";
  try {
    const page = await api.list(props.brandId, PAGE_SIZE, next);
    if (!current(ticket, "list", captured, generation)) return false;
    jobs.value = page.items; jobTotal.value = page.total_count; offset.value = page.offset;
    if (job.value && !page.items.some((item) => item.id === job.value?.id)) void loadJob(job.value.id);
    return true;
  } catch (problem) {
    if (current(ticket, "list", captured, generation)) listError.value = message(problem);
    return false;
  } finally { if (current(ticket, "list", captured, generation)) loading.value = false; }
}
async function loadTargets(next = 0) {
  if (!job.value || !rights.value.view) return false;
  const id = job.value.id, ticket = ++targetTicket, captured = permissionScope.value, generation = reconciliationSessionGeneration();
  targetLoading.value = true; targetError.value = "";
  targets.value = []; targetTotal.value = "0";
  try {
    const page = await api.targets(props.brandId, id, outcomeFilter.value || null, PAGE_SIZE, next);
    if (!current(ticket, "targets", captured, generation) || !job.value || job.value.id !== id) return false;
    targets.value = page.items; targetTotal.value = page.total_count; targetOffset.value = page.offset; return true;
  } catch (problem) {
    if (current(ticket, "targets", captured, generation) && job.value?.id === id) targetError.value = message(problem);
    return false;
  } finally { if (current(ticket, "targets", captured, generation)) targetLoading.value = false; }
}
async function loadJob(id: string) {
  const ticket = ++detailTicket, captured = permissionScope.value, generation = reconciliationSessionGeneration();
  detailLoading.value = true; detailError.value = ""; job.value = null; refreshPending(); targetTicket++; targets.value = []; targetTotal.value = "0";
  try {
    const value = await api.read(props.brandId, id);
    if (!current(ticket, "detail", captured, generation)) return false;
    job.value = value; refreshPending(); await loadTargets(0); return true;
  } catch (problem) {
    if (current(ticket, "detail", captured, generation)) detailError.value = message(problem);
    return false;
  } finally { if (current(ticket, "detail", captured, generation)) detailLoading.value = false; }
}
function message(problem: unknown): string {
  if (problem instanceof AdminApiError && problem.status === 0) return "网络或响应不可用，当前状态尚未读取。可稍后重新读取。";
  return problem instanceof Error ? problem.message : "请求失败。";
}
function validReasonText(value: string): boolean {
  if (value.trim() !== value || value.length === 0 || new TextEncoder().encode(value).length > 500 || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(i + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
      i++;
    } else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}
function selectedForCreate(): PendingReconciliationWrite | null {
  const existing = getPendingReconciliationWrite(createScope.value);
  if (existing) return existing;
  if (!reasonValid.value) return null;
  const body = freezeReconciliationBody({ reason: reason.value });
  return Object.freeze({ ...createScope.value, body, key: createIdempotencyKey() });
}
function selectedForRetry(): PendingReconciliationWrite | null {
  const scope = retryScope.value;
  if (!scope) return null;
  const existing = getPendingReconciliationWrite(scope);
  if (existing) return existing;
  if (!job.value || !retryReasonValid.value) return null;
  const body = freezeReconciliationBody({ version: job.value.version, reason: retryReason.value });
  return Object.freeze({ ...scope, body, key: createIdempotencyKey() });
}
function beginReview(intent: PendingReconciliationWrite | null) {
  if (!intent || writing.value) return;
  review.value = intent; confirmed.value = false; writeError.value = "";
}
async function submit(intent: PendingReconciliationWrite) {
  const capturedPermission = permissionScope.value, generation = reconciliationSessionGeneration();
  const ticket = ++writeTicket;
  const knownJob = job.value;
  const previous = knownJob && knownJob.id === intent.jobId && knownJob.state === "failed" &&
    knownJob.version === (intent.body as { version: number }).version ? knownJob : undefined;
  if (!getPendingReconciliationWrite(intent)) setPendingReconciliationWrite(intent, intent);
  refreshPending(); writing.value = true; review.value = null; confirmed.value = false; writeError.value = ""; notice.value = "";
  try {
    const result = intent.operation === "create"
      ? await api.create(intent.brandId, intent.body, intent.key, intent.accountId)
      : await api.retry(intent.brandId, intent.jobId!, intent.body as { version: number; reason: string }, intent.key, previous);
    if (!currentWrite(ticket, capturedPermission, generation)) return;
    clearPendingReconciliationWrite(intent, intent.key);
    receipt.value = result; notice.value = "服务器已确认原始回执。正在独立读取当前任务状态。"; refreshPending();
    // Receipt is immutable evidence of this request; current state is a separate read.
    job.value = result;
    await Promise.all([loadJob(result.id), loadList(offset.value)]);
  } catch (problem) {
    if (problem instanceof AdminApiError && problem.status === 401) {
      if (currentWrite(ticket, capturedPermission, generation)) emit("session-invalid");
      return;
    }
    const failure = classifyReconciliationWriteFailure(problem instanceof AdminApiError ? problem.status : undefined);
    if (failure === "unknown") {
      if (currentWrite(ticket, capturedPermission, generation)) {
        setPendingReconciliationWrite(intent, intent); refreshPending(); writeError.value = "写入结果未知。原请求体和幂等键已保留；只读读取不能确认或清除此操作。请显式重试同一请求。";
      }
    } else {
      if (currentWrite(ticket, capturedPermission, generation)) {
        clearPendingReconciliationWrite(intent, intent.key); refreshPending();
        writeError.value = `服务器明确拒绝此请求。${message(problem)} 请读取最新状态后再创建新请求。`;
      }
    }
  } finally { if (currentWrite(ticket, capturedPermission, generation)) writing.value = false; }
}
function confirmWrite() {
  if (!review.value || !confirmed.value || writing.value) return;
  void submit(review.value);
}
function confirmRecovery(intent: PendingReconciliationWrite) { beginReview(intent); }
function selectJob(id: string) { receipt.value = null; void loadJob(id); }
function setOutcome(value: string) { outcomeFilter.value = value as ReconciliationOutcome | ""; void loadTargets(0); }
function pageList(delta: number) { if (!loading.value) void loadList(Math.max(0, offset.value + delta * PAGE_SIZE)); }
function pageTargets(delta: number) { if (!targetLoading.value) void loadTargets(Math.max(0, targetOffset.value + delta * PAGE_SIZE)); }
function statusLabel(value: string) { return ({ pending: "待处理", running: "执行中", completed: "已完成", failed: "失败", checked: "已检查", consistent: "一致", repairable: "可修复", corrupt: "异常" } as Record<string, string>)[value] ?? value; }
function short(value: string) { return `${value.slice(0, 8)}…${value.slice(-4)}`; }
function json(value: unknown) { return JSON.stringify(value, null, 2); }
function onOutcomeChange(event: Event) { setOutcome((event.target as HTMLSelectElement).value); }
function retryVersion(intent: PendingReconciliationWrite) { return (intent.body as { version: number }).version; }

watch(permissionScope, () => { resetView(); if (rights.value.view) void loadList(0); }, { immediate: true });
watch(() => job.value?.id, () => { refreshPending(); retryReason.value = ""; });
onBeforeUnmount(() => { alive = false; listTicket++; detailTicket++; targetTicket++; writeTicket++; });
</script>

<template>
  <main class="recon" aria-labelledby="recon-title">
    <header class="recon-head">
      <div><p class="eyebrow">WALLET CONTROL</p><h1 id="recon-title">余额对账任务</h1><p class="sub">按任务提交时的账户范围记录历史观察结果；不会自动修复钱包或声称资金操作成功。</p></div>
      <button class="quiet" :disabled="loading || !rights.view" @click="loadList(offset)">刷新任务</button>
    </header>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <p v-if="writeError" class="error write-error" role="alert">{{ writeError }}</p>

    <section v-if="!rights.view" class="panel muted-panel">当前账号没有查看此品牌钱包对账的权限。</section>

    <template v-else>
      <section v-if="receipt" class="panel receipt-panel" role="status">
        <div class="section-title"><div><h2>服务器已确认请求回执</h2><p>这是该次写入返回的原始回执；当前任务状态会通过独立只读读取更新。</p></div><span class="tag">回执 v{{ receipt.version }}</span></div>
        <p>任务 {{ receipt.id }} · {{ statusLabel(receipt.state) }} · 范围 {{ receipt.target_count }} 个账户 · 待处理 {{ receipt.pending_count }}</p>
      </section>
      <section class="panel create-panel">
        <div class="section-title"><div><h2>提交当前品牌检查</h2><p>任务范围在创建时捕获；最多 100,000 个账户。超限或已有活动任务时，服务器不会创建任务。</p></div><span class="tag">只读检查</span></div>
        <label class="field">操作原因<textarea v-model="reason" aria-label="操作原因" aria-describedby="recon-reason-count" maxlength="500" rows="2" placeholder="说明为什么需要检查"></textarea><small id="recon-reason-count">{{ reasonByteLength }} / 500 bytes</small></label>
        <div class="actions"><button class="primary" :disabled="!canCreate" @click="beginReview(selectedForCreate())">检查并确认提交</button><span v-if="!rights.run" class="sub">此账号只能查看。</span></div>
      </section>

      <section v-if="pendingCreate || pendingRetries.length" class="panel pending-panel" aria-live="polite">
        <div class="section-title"><div><h2>尚未确定的写入</h2><p>即使任务列表或详情不可用，冻结的请求仍可从此处恢复。</p></div><span class="tag warning">{{ writing ? "正在发送，等待回执" : "结果未知 / 待重放" }}</span></div>
        <article v-if="pendingCreate" class="pending-item"><b>创建当前品牌任务</b><code>{{ pendingCreate.key }}</code><p>原因：{{ pendingCreate.body.reason }}</p><button class="primary" :disabled="writing" @click="confirmRecovery(pendingCreate)">确认并重放原创建请求</button></article>
        <article v-for="intent in pendingRetries" :key="intent.key" class="pending-item"><b>重试任务 {{ short(intent.jobId!) }}</b><code>{{ intent.key }}</code><p>版本 v{{ retryVersion(intent) }} · {{ intent.body.reason }}</p><button class="primary" :disabled="writing" @click="confirmRecovery(intent)">确认并重放原重试请求</button></article>
      </section>

      <div class="columns">
        <section class="panel jobs-panel">
          <div class="section-title"><div><h2>任务记录</h2><p>{{ jobTotal }} 条记录</p></div><span v-if="loading" class="tag">读取中</span></div>
          <p v-if="listError" class="error" role="alert">{{ listError }}</p>
          <div v-if="!loading && !listError && jobs.length === 0" class="empty">暂无对账任务。</div>
          <div v-for="item in jobs" :key="item.id" class="job-row" :class="{ selected: job?.id === item.id }">
            <button class="job-select" :aria-label="`查看对账任务 ${item.id}`" @click="selectJob(item.id)"><span class="job-title">{{ short(item.id) }} <i :class="['state', item.state]">{{ statusLabel(item.state) }}</i></span><span class="sub">{{ item.created_at }} · v{{ item.version }}</span><span class="sub">{{ item.checked_count }} / {{ item.target_count }} 已检查 · {{ item.pending_count }} 待处理</span></button>
          </div>
          <div class="pager"><button class="quiet" :disabled="loading || offset <= 0" @click="pageList(-1)">上一页</button><span>{{ Math.floor(offset / PAGE_SIZE) + 1 }} / {{ totalJobPages }}</span><button class="quiet" :disabled="loading || offset + PAGE_SIZE >= Number(jobTotal)" @click="pageList(1)">下一页</button></div>
        </section>

        <section class="panel detail-panel">
          <template v-if="job || detailLoading || detailError">
            <div class="section-title"><div><h2>任务详情</h2><p v-if="job">{{ job.id }}</p></div><span v-if="detailLoading" class="tag">读取中</span></div>
            <p v-if="detailError" class="error" role="alert">当前任务状态读取失败：{{ detailError }}</p>
            <template v-if="job">
              <div class="stats"><div><b>{{ job.target_count }}</b><span>范围账户</span></div><div><b>{{ job.checked_count }}</b><span>已检查</span></div><div><b>{{ job.pending_count }}</b><span>待处理</span></div><div><b>{{ job.failed_count }}</b><span>检查失败</span></div></div>
              <div class="metrics"><span>一致 <b>{{ job.consistent_count }}</b></span><span>可修复 <b>{{ job.repairable_count }}</b></span><span>异常 <b>{{ job.corrupt_count }}</b></span></div>
              <dl class="facts"><dt>状态 / 版本</dt><dd>{{ statusLabel(job.state) }} · v{{ job.version }}</dd><dt>提交人</dt><dd>{{ job.created_by }}</dd><dt>创建时间</dt><dd>{{ job.created_at }}</dd><dt>开始时间</dt><dd>{{ job.started_at ?? "尚未开始" }}</dd><dt>完成时间</dt><dd>{{ job.completed_at ?? "尚未完成" }}</dd><dt>原因</dt><dd>{{ job.reason }}</dd><dt>创建审计记录</dt><dd>{{ job.creation_audit_log_id }}</dd><dt>最近错误</dt><dd>{{ job.last_error_code ?? "无" }}</dd></dl>
              <div v-if="job.state === 'failed' && job.can_retry" class="retry-box"><h3>重试失败任务</h3><p>已检查账户保留原观察结果；失败账户重置为待处理。提交使用当前已读版本 v{{ job.version }}。</p><label class="field">重试原因<textarea v-model="retryReason" maxlength="500" rows="2"></textarea></label><button class="primary" :disabled="!canRetry" @click="beginReview(selectedForRetry())">检查并确认重试</button></div>
              <div class="targets-head"><div><h3>账户检查结果</h3><p>{{ targetTotal }} 条{{ outcomeFilter ? ` · ${statusLabel(outcomeFilter)}` : "" }}</p></div><label>结果筛选<select aria-label="结果筛选" :value="outcomeFilter" @change="onOutcomeChange"><option value="">全部</option><option value="pending">待处理</option><option value="failed">检查失败</option><option value="consistent">一致</option><option value="repairable">可修复</option><option value="corrupt">异常</option></select></label></div>
              <p v-if="targetError" class="error" role="alert">{{ targetError }}</p><p v-if="targetLoading" class="sub">正在读取账户结果…</p>
              <div v-if="!targetLoading && !targetError && targets.length === 0" class="empty">此筛选下没有账户记录。</div>
              <article v-for="item in targets" :key="item.id" class="target-card">
                <div class="target-title"><span><b>{{ item.member_id }}</b><small>账户 {{ item.account_id }}</small></span><i :class="['state', item.outcome ?? item.state]">{{ statusLabel(item.outcome ?? item.state) }}</i></div>
                <p class="sub">尝试 {{ item.attempt_count }} 次 · {{ item.checked_at ?? "未检查" }} · {{ item.error_code ?? "无错误" }}</p>
                <details v-if="item.preview"><summary>查看此任务保存的只读余额观察快照</summary><p class="sub">这是任务检查时观察到的历史数据。需要处理差异时，请前往「资金与账本」中的「余额差错处理」，重新获取实时预览后单独确认。</p><div class="preview-grid"><div><h4>观察到的余额（稀疏实际值）</h4><pre>{{ json(item.preview.actual) }}</pre></div><div><h4>流水期望余额</h4><pre>{{ json(item.preview.expected) }}</pre></div></div><p>钱包版本 {{ item.preview.version }} · 流水版本 {{ item.preview.ledger_version }} · {{ item.preview.consistent ? "一致" : item.preview.repairable ? "可修复" : "存在异常" }}</p><ul v-if="item.preview.issues.length"><li v-for="issue in item.preview.issues" :key="issue">{{ issue }}</li></ul></details>
              </article>
              <div class="pager"><button class="quiet" :disabled="targetLoading || targetOffset <= 0" @click="pageTargets(-1)">上一页</button><span>{{ Math.floor(targetOffset / PAGE_SIZE) + 1 }} / {{ totalTargetPages }}</span><button class="quiet" :disabled="targetLoading || targetOffset + PAGE_SIZE >= Number(targetTotal)" @click="pageTargets(1)">下一页</button></div>
            </template>
          </template>
          <div v-else class="empty detail-empty">选择一个任务查看保存的只读检查结果。</div>
        </section>
      </div>
    </template>

    <div v-if="review" class="scrim" role="presentation" @click.self="review = null; confirmed = false">
      <section class="confirm panel" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
        <h2 id="confirm-title">确认唯一操作员提交</h2><p>请核对冻结的请求内容。点击确认后只发送一次该操作；若结果未知，只能使用同一请求体和幂等键重放。</p>
        <dl class="facts"><dt>操作</dt><dd>{{ review.operation === "create" ? "创建对账任务" : `重试任务 ${review.jobId}` }}</dd><dt>原因</dt><dd>{{ review.body.reason }}</dd><dt>版本</dt><dd>{{ "version" in review.body ? `v${review.body.version}` : "新任务 v1" }}</dd><dt>幂等键</dt><dd><code>{{ review.key }}</code></dd></dl>
        <label class="confirm-check"><input v-model="confirmed" type="checkbox">我已核对范围与原因，确认由我提交此请求。</label>
        <div class="actions"><button class="quiet" @click="review = null; confirmed = false">返回</button><button class="primary" :disabled="!confirmed || writing" @click="confirmWrite">确认并提交</button></div>
      </section>
    </div>
  </main>
</template>

<style scoped>
.recon{width:100%;max-width:1500px;min-width:0;margin:0 auto;padding:clamp(14px,2.4vw,30px);color:#252a36;overflow-wrap:anywhere}.recon *{min-width:0}.recon-head,.section-title,.actions,.pager,.targets-head,.target-title{display:flex;align-items:center;justify-content:space-between;gap:12px}.recon-head{margin-bottom:20px}.recon h1{font-size:clamp(22px,3vw,30px);margin:0 0 7px}.recon h2{font-size:17px;margin:0 0 5px}.recon h3{font-size:14px;margin:0 0 8px}.recon h4{font-size:12px;margin:8px 0}.recon p{margin:4px 0 10px}.eyebrow{font-size:10px;letter-spacing:1.2px;color:#6674da;font-weight:700}.sub,.recon small{color:#737b8c;font-size:11px}.panel{background:#fff;border:1px solid #e9ebf0;border-radius:12px;padding:18px;box-shadow:0 2px 8px #1e2a5008;margin-bottom:16px}.muted-panel,.empty{color:#737b8c}.section-title{align-items:flex-start;margin-bottom:12px}.tag,.state{display:inline-flex;border-radius:999px;padding:4px 9px;background:#f0f2f7;color:#687083;font-size:10px;font-style:normal;white-space:nowrap}.tag.warning{background:#fff2dc;color:#945b00}.field{display:flex;flex-direction:column;gap:5px;max-width:720px;margin:12px 0}.field textarea,.targets-head select{border:1px solid #dfe2ea;border-radius:8px;padding:10px;background:#fff;color:inherit;resize:vertical}.quiet,.primary{border:1px solid #dfe2ea;border-radius:8px;padding:9px 13px;background:#fff}.primary{background:#5969df;color:#fff;border-color:#5969df;font-weight:650}.actions{justify-content:flex-start;flex-wrap:wrap}.columns{display:grid;grid-template-columns:minmax(260px,.72fr) minmax(0,1.5fr);gap:16px;align-items:start}.jobs-panel,.detail-panel{min-width:0}.job-row{border-top:1px solid #eef0f4}.job-row.selected{background:#f5f6ff}.job-select{display:flex;flex-direction:column;align-items:flex-start;gap:5px;width:100%;padding:12px 8px;border:0;background:transparent;text-align:left;overflow-wrap:anywhere}.job-title{display:flex;gap:8px;align-items:center;font-weight:700}.state.completed,.state.consistent{background:#e6f5ed;color:#1e7954}.state.failed,.state.corrupt{background:#fff0ed;color:#ab4236}.state.repairable{background:#fff4df;color:#936000}.state.running{background:#eaf0ff;color:#4259c8}.pager{margin-top:14px;font-size:11px;color:#737b8c}.stats{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px}.stats>div{background:#f7f8fb;border-radius:9px;padding:11px}.stats b,.stats span{display:block}.stats b{font-size:19px}.stats span{font-size:10px;color:#737b8c;margin-top:3px}.metrics{display:flex;gap:18px;flex-wrap:wrap;padding:13px 0;border-bottom:1px solid #eef0f4}.facts{display:grid;grid-template-columns:minmax(110px,.45fr) minmax(0,1fr);gap:9px 14px;font-size:11px}.facts dt{color:#737b8c}.facts dd{margin:0;overflow-wrap:anywhere}.retry-box{background:#fffaf0;border:1px solid #f2e5c8;border-radius:10px;padding:14px;margin:16px 0}.targets-head{align-items:flex-end;margin-top:20px;padding-top:14px;border-top:1px solid #e9ebf0}.targets-head label{display:flex;align-items:center;gap:7px;font-size:11px}.target-card{border:1px solid #e9ebf0;border-radius:9px;padding:12px;margin-top:9px}.target-title small{display:block;margin-top:4px}.target-card details{border-top:1px solid #eef0f4;padding-top:9px;margin-top:9px}.target-card summary{cursor:pointer;font-weight:650;font-size:11px}.preview-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.preview-grid pre{max-height:260px;overflow:auto;background:#f7f8fb;border-radius:7px;padding:10px;font-size:10px;white-space:pre-wrap;overflow-wrap:anywhere}.error{color:#a63732;background:#fff2f0;border-radius:8px;padding:10px}.notice{position:fixed;right:18px;bottom:18px;max-width:min(520px,calc(100vw - 36px));background:#e8f5ee;color:#236548;padding:12px 15px;border-radius:9px;box-shadow:0 8px 30px #0002;z-index:20}.write-error{position:fixed;right:18px;bottom:18px;max-width:min(520px,calc(100vw - 36px));z-index:21}.pending-panel{border-color:#f0d8a9;background:#fffdf8}.pending-item{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px 12px;padding:12px 0;border-top:1px solid #f2e8d5}.pending-item code{overflow-wrap:anywhere}.pending-item p{grid-column:1/-1;margin:0}.pending-item button{grid-column:1/-1;justify-self:start}.scrim{position:fixed;inset:0;background:#17203388;z-index:30;display:grid;place-items:center;padding:16px}.confirm{width:min(560px,100%);max-height:90vh;overflow:auto;margin:0}.confirm-check{display:flex;gap:9px;align-items:flex-start;font-size:12px;margin:16px 0}.confirm-check input{margin-top:2px}.detail-empty{padding:35px 10px;text-align:center}
@media(max-width:760px){.columns{grid-template-columns:minmax(0,1fr)}.recon-head{align-items:flex-start}.stats{grid-template-columns:repeat(2,minmax(0,1fr))}.preview-grid{grid-template-columns:minmax(0,1fr)}.panel{padding:14px}.targets-head{align-items:flex-start;flex-direction:column}.targets-head label{width:100%;justify-content:space-between}.targets-head select{max-width:65%}}
@media(max-width:420px){.recon{padding:12px}.recon-head{gap:6px}.recon-head .quiet{padding:8px;font-size:10px}.pending-item{grid-template-columns:minmax(0,1fr)}.pending-item code,.pending-item p,.pending-item button{grid-column:1}.facts{grid-template-columns:minmax(88px,.4fr) minmax(0,1fr);gap:8px}.pager{gap:6px}.pager button{padding:8px}}
.notice,.write-error{position:static;max-width:100%;z-index:auto;right:auto;bottom:auto;margin-bottom:16px}
.recon .field{height:auto;border:0;background:transparent;padding:0;font-size:12px}
</style>
