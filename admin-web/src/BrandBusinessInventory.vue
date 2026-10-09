<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import type { LocalizedMessage } from "@lottery/shared";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createBusinessInventoryApi, businessInventoryPermissions, type BrandBusinessInventory, type BusinessInventoryIssue } from "./business-inventory-api";
import { reconciliationSessionGeneration } from "./reconciliation-state";
import { useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createBusinessInventoryApi();
const { t } = useAdminI18n();
const rights = computed(() => businessInventoryPermissions(props.account, props.brandId));
const permissionScope = computed(() => JSON.stringify([
  props.account.id, props.account.super_admin, props.account.brand_ids,
  props.account.permissions, props.account.permissions_by_brand,
  props.account.platform_permissions, props.brandId, rights.value,
]));
const inventory = ref<BrandBusinessInventory | null>(null);
const loading = ref(false);
const error = ref<string | LocalizedMessage>("");
let alive = true;
let ticket = 0;

function reset() {
  ticket++;
  loading.value = false;
  inventory.value = null;
  error.value = "";
}

function current(requestTicket: number, scope: string, generation: number): boolean {
  return alive && requestTicket === ticket && scope === permissionScope.value && generation === reconciliationSessionGeneration() && rights.value.view;
}

function failureText(problem: unknown): string | LocalizedMessage {
  if (problem instanceof AdminApiError) {
    if (problem.status === 0) return t("网络或响应不可用，未取得当前观察结果。请重新加载。", "The network or response is unavailable. No current observation was received. Load again.");
    if (problem.status === 401) return t("后台会话已失效。", "The admin session has expired.");
    if (problem.status === 403) return t("此账号没有查看当前品牌业务引用的权限。", "This account cannot view business references for the current brand.");
    if (problem.code === "INVALID_RESPONSE" || problem.status === 502) return t("服务器响应不符合品牌业务引用检查契约，未显示旧结果。", "The server response did not match the business inventory contract. No previous result is shown.");
    return t("读取品牌业务引用失败（HTTP {status}）。", "Could not load brand business references (HTTP {status}).", { status: problem.status });
  }
  return t("读取品牌业务引用失败。", "Could not load brand business references.");
}

async function loadInventory() {
  if (!rights.value.view) { reset(); return; }
  const requestTicket = ++ticket;
  const scope = permissionScope.value;
  const generation = reconciliationSessionGeneration();
  loading.value = true;
  inventory.value = null;
  error.value = "";
  try {
    const value = await api.read(props.brandId);
    if (current(requestTicket, scope, generation)) inventory.value = value;
  } catch (problem) {
    if (current(requestTicket, scope, generation)) {
      inventory.value = null;
      error.value = failureText(problem);
      if (problem instanceof AdminApiError && problem.status === 401) emit("session-invalid");
    }
  } finally {
    if (alive && requestTicket === ticket && scope === permissionScope.value) {
      if (generation !== reconciliationSessionGeneration()) {
        inventory.value = null;
        error.value = "";
      }
      loading.value = false;
    }
  }
}

function issueKey(issue: BusinessInventoryIssue, index: number): string {
  return `${issue.source_table}/${issue.source_id}/${issue.code}/${issue.reference_key}/${issue.parent_table}/${index}`;
}

watch(permissionScope, reset, { immediate: true });
onBeforeUnmount(() => { alive = false; ticket++; });
</script>

<template>
  <section class="inventory panel" aria-labelledby="business-inventory-title">
    <div class="inventory-head">
      <div>
        <p class="eyebrow">BRAND BUSINESS INVENTORY</p>
        <h2 id="business-inventory-title">{{ t("品牌业务引用检查", "Brand business reference inventory") }}</h2>
        <p class="sub">{{ t("从当前品牌业务记录检查已声明的结构引用和必需账本指针。它不核对金额链，也不会执行修复。", "Checks declared structural references and required ledger pointers from this brand’s business records. It does not verify amount flows or perform repairs.") }}</p>
      </div>
      <button class="quiet" :disabled="loading || !rights.view" @click="loadInventory">
        {{ loading ? t("读取中…", "Loading…") : inventory ? t("重新加载观察", "Load a fresh observation") : t("加载观察", "Load observation") }}
      </button>
    </div>

    <p v-if="!rights.view" class="inventory-empty">{{ t("当前账号没有此品牌的 wallet.view.brand 权限。", "This account lacks wallet.view.brand for this brand.") }}</p>
    <p v-else-if="loading" class="inventory-empty" role="status">{{ t("正在读取当前品牌观察…", "Loading the current brand observation…") }}</p>
    <p v-else-if="error" class="error" role="alert">{{ t(error) }}</p>
    <p v-else-if="!inventory" class="inventory-empty">{{ t("选择“加载观察”后才会读取；本页面不会自动执行检查。", "Choose “Load observation” to read it. This page does not run the check automatically.") }}</p>

    <template v-else>
      <p class="scope-note">{{ t("这是 {time} 对已声明引用范围的只读观察，不是完整钱包或财务证明。来源行数与引用数不是金额或资金笔数。", "This is a read-only observation of declared references at {time}, not a complete wallet or financial proof. Source rows and references are not amounts or counts of money movements.", { time: inventory.snapshot_at }) }}</p>
      <div class="inventory-stats" aria-label="Inventory totals">
        <div><b>{{ inventory.source_row_count }}</b><span>{{ t("来源行", "Source rows") }}</span></div>
        <div><b>{{ inventory.reference_count }}</b><span>{{ t("引用", "References") }}</span></div>
        <div><b>{{ inventory.issue_count }}</b><span>{{ t("问题总数", "Total issues") }}</span></div>
        <div><b>{{ inventory.consistent ? t("未发现", "None found") : t("已发现", "Found") }}</b><span>{{ t("本版声明范围内的问题", "Issues in this declared scope") }}</span></div>
      </div>
      <p v-if="inventory.issues_truncated" class="truncated" role="status">
        {{ t("共 {total} 个问题；显示前 {shown} 个，另有 {hidden} 个未显示。", "{total} issues total; showing the first {shown}, with {hidden} more not displayed.", { total: inventory.issue_count, shown: inventory.issues.length, hidden: (BigInt(inventory.issue_count) - BigInt(inventory.issues.length)).toString() }) }}
      </p>
      <p v-else class="sub issue-count-note">{{ t("已显示全部 {count} 个问题。", "All {count} issues are shown.", { count: inventory.issues.length }) }}</p>

      <details class="inventory-details" open>
        <summary>{{ t("来源覆盖（{count} 张表）", "Source coverage ({count} tables)", { count: inventory.coverage.length }) }}</summary>
        <div class="coverage-table" role="table" :aria-label="t('来源覆盖', 'Source coverage')">
          <div class="coverage-row coverage-header" role="row"><span role="columnheader">{{ t("来源表", "Source table") }}</span><span role="columnheader">{{ t("来源行", "Source rows") }}</span><span role="columnheader">{{ t("引用", "References") }}</span><span role="columnheader">{{ t("问题", "Issues") }}</span></div>
          <div v-for="row in inventory.coverage" :key="row.source_table" class="coverage-row" role="row"><code role="cell">{{ row.source_table }}</code><span role="cell">{{ row.source_row_count }}</span><span role="cell">{{ row.reference_count }}</span><span role="cell">{{ row.issue_count }}</span></div>
        </div>
      </details>

      <details class="inventory-details" open>
        <summary>{{ t("引用问题（{count} 项）", "Reference issues ({count})", { count: inventory.issues.length }) }}</summary>
        <p v-if="inventory.issues.length === 0" class="inventory-empty">{{ t("本版声明范围内没有报告问题。", "No issues were reported in this declared scope.") }}</p>
        <div v-else class="issue-list">
          <article v-for="(issue, index) in inventory.issues" :key="issueKey(issue, index)" class="issue-row">
            <b>{{ issue.code }}</b>
            <dl><dt>{{ t("来源表 / 来源标识", "Source table / source ID") }}</dt><dd><code>{{ issue.source_table }}</code> · <code>{{ issue.source_id }}</code></dd>
              <dt>{{ t("引用字段", "Reference key") }}</dt><dd><code>{{ issue.reference_key }}</code></dd>
              <dt>{{ t("父表", "Parent table") }}</dt><dd><code>{{ issue.parent_table }}</code></dd></dl>
          </article>
        </div>
      </details>

      <details class="inventory-details metadata">
        <summary>{{ t("观察元数据", "Observation metadata") }}</summary>
        <p>{{ t("结构版本", "Schema version") }} {{ inventory.schema_version }} · {{ t("指纹（仅观察标识，不是修复令牌）", "Fingerprint (observation identifier only; not a repair token)") }}</p>
        <code class="fingerprint">{{ inventory.fingerprint }}</code>
      </details>
    </template>
  </section>
</template>

<style scoped>
.inventory{color:#252a36}.inventory-head{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.inventory h2{font-size:17px;margin:0 0 5px}.eyebrow{font-size:10px;letter-spacing:1.2px;color:#6674da;font-weight:700;margin:0 0 5px}.sub,.inventory small{color:#737b8c;font-size:11px}.quiet{flex-shrink:0;border:1px solid #dfe2ea;border-radius:8px;padding:9px 13px;background:#fff;color:inherit}.inventory-empty{color:#737b8c;padding:10px 0}.error{color:#a63732;background:#fff2f0;border-radius:8px;padding:10px}.scope-note{padding:10px 12px;border-radius:8px;background:#f7f8fb;color:#60697a;font-size:11px;line-height:1.55}.inventory-stats{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;margin:14px 0}.inventory-stats>div{background:#f7f8fb;border-radius:9px;padding:11px}.inventory-stats b,.inventory-stats span{display:block}.inventory-stats b{font-size:18px;overflow-wrap:anywhere}.inventory-stats span{font-size:10px;color:#737b8c;margin-top:3px}.truncated{padding:10px;background:#fff5df;color:#875700;border-radius:8px;font-size:11px}.issue-count-note{margin:10px 0}.inventory-details{border-top:1px solid #e9ebf0;padding-top:12px;margin-top:14px}.inventory-details summary{cursor:pointer;font-size:12px;font-weight:700}.coverage-table{margin-top:10px}.coverage-row{display:grid;grid-template-columns:minmax(170px,2fr) repeat(3,minmax(70px,.7fr));gap:10px;padding:7px 8px;border-top:1px solid #eef0f4;font-size:11px}.coverage-header{background:#f7f8fb;color:#737b8c;font-weight:650;border-radius:6px}.coverage-row code,.issue-row code,.fingerprint{overflow-wrap:anywhere}.issue-list{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,330px),1fr));gap:9px;margin-top:10px}.issue-row{border:1px solid #e9ebf0;border-radius:8px;padding:10px;font-size:11px}.issue-row>b{font-size:10px;color:#a63732}.issue-row dl{display:grid;grid-template-columns:minmax(110px,.6fr) minmax(0,1.4fr);gap:7px 10px;margin:9px 0 0}.issue-row dt{color:#737b8c}.issue-row dd{margin:0;overflow-wrap:anywhere}.metadata p{font-size:11px;color:#737b8c}.fingerprint{display:block;font-size:10px;color:#667085;word-break:break-all}
@media(max-width:600px){.inventory-head{flex-direction:column}.inventory-stats{grid-template-columns:repeat(2,minmax(0,1fr))}.coverage-row{grid-template-columns:minmax(110px,1.7fr) repeat(3,minmax(42px,.6fr));gap:5px;padding:7px 4px;font-size:10px}.issue-row dl{grid-template-columns:minmax(90px,.7fr) minmax(0,1.3fr)}}
</style>
