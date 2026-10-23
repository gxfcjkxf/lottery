<script setup lang="ts">
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { computed, onBeforeUnmount, ref, watch } from "vue";
import type { LocalizedMessage } from "@lottery/shared";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createBusinessInventoryApi, businessInventoryPermissions, type BrandBusinessInventory, type BusinessInventoryIssue } from "./business-inventory-api";
import { reconciliationSessionGeneration } from "./reconciliation-state";
import { useAdminI18n } from "./i18n";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createBusinessInventoryApi();
const { t, display } = useAdminI18n();
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
    if (problem.status === 0) return t('admin.copy.d1be62f8');
    if (problem.status === 401) return t('admin.copy.c57d4226');
    if (problem.status === 403) return t('admin.copy.12145bbb');
    if (problem.code === "INVALID_RESPONSE" || problem.status === 502) return t('admin.copy.ee110037');
    return t('admin.copy.8b43c5e8', { status: problem.status });
  }
  return t('admin.copy.3bfb36d2');
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
        <h2 id="business-inventory-title">{{ t('admin.copy.347b90c9') }}</h2>
      </div>
      <button v-refresh-button="loading ? t('admin.copy.27342bf3') : inventory ? t('admin.copy.1379ffc5') : t('admin.copy.a76c5df1')" class="button button-quiet" :disabled="loading || !rights.view" @click="withGlobalLoading(loadInventory)">
        {{ loading ? t('admin.copy.27342bf3') : inventory ? t('admin.copy.1379ffc5') : t('admin.copy.a76c5df1') }}
      </button>
    </div>

    <p v-if="!rights.view" class="inventory-empty">{{ t('admin.copy.5f3b7b75') }}</p>
    <p v-global-loading v-else-if="loading" class="inventory-empty" role="status">{{ t('admin.copy.74959336') }}</p>
    <p v-else-if="error" class="error" role="alert">{{ display(error) }}</p>
    <p v-else-if="!inventory" class="inventory-empty">{{ t('admin.copy.c622e9d1') }}</p>

    <template v-else>
      <p class="scope-note">{{ t('admin.copy.656f0e38', { time: inventory.snapshot_at }) }}</p>
      <div class="inventory-stats" aria-label="Inventory totals">
        <div><b>{{ inventory.source_row_count }}</b><span>{{ t('admin.copy.5ce26a46') }}</span></div>
        <div><b>{{ inventory.reference_count }}</b><span>{{ t('admin.copy.63c00d12') }}</span></div>
        <div><b>{{ inventory.issue_count }}</b><span>{{ t('admin.copy.367b01e2') }}</span></div>
        <div><b>{{ inventory.consistent ? t('admin.copy.8e4b873e') : t('admin.copy.f09bd430') }}</b><span>{{ t('admin.copy.caea7002') }}</span></div>
      </div>
      <p v-if="inventory.issues_truncated" class="truncated" role="status">
        {{ t('admin.copy.7e32f9e9', { total: inventory.issue_count, shown: inventory.issues.length, hidden: (BigInt(inventory.issue_count) - BigInt(inventory.issues.length)).toString() }) }}
      </p>
      <p v-else class="sub issue-count-note">{{ t('admin.copy.0b8fd7f7', { count: inventory.issues.length }) }}</p>

      <details class="inventory-details" open>
        <summary>{{ t('admin.copy.ea86a364', { count: inventory.coverage.length }) }}</summary>
        <div class="coverage-table" role="table" :aria-label="t('admin.copy.0c264ce5')">
          <div class="coverage-row coverage-header" role="row"><span role="columnheader">{{ t('admin.copy.441d67c9') }}</span><span role="columnheader">{{ t('admin.copy.5ce26a46') }}</span><span role="columnheader">{{ t('admin.copy.63c00d12') }}</span><span role="columnheader">{{ t('admin.copy.b9534a45') }}</span></div>
          <div v-for="row in inventory.coverage" :key="row.source_table" class="coverage-row" role="row"><code role="cell">{{ row.source_table }}</code><span role="cell">{{ row.source_row_count }}</span><span role="cell">{{ row.reference_count }}</span><span role="cell">{{ row.issue_count }}</span></div>
        </div>
      </details>

      <details class="inventory-details" open>
        <summary>{{ t('admin.copy.9cfc504a', { count: inventory.issues.length }) }}</summary>
        <p v-if="inventory.issues.length === 0" class="inventory-empty empty-state">{{ t('admin.copy.603194ca') }}</p>
        <div v-else class="issue-list">
          <article v-for="(issue, index) in inventory.issues" :key="issueKey(issue, index)" class="issue-row">
            <b>{{ issue.code }}</b>
            <dl><dt>{{ t('admin.copy.ae9cd82b') }}</dt><dd><code>{{ issue.source_table }}</code> · <code>{{ issue.source_id }}</code></dd>
              <dt>{{ t('admin.copy.c2ef4b16') }}</dt><dd><code>{{ issue.reference_key }}</code></dd>
              <dt>{{ t('admin.copy.ef35ee7d') }}</dt><dd><code>{{ issue.parent_table }}</code></dd></dl>
          </article>
        </div>
      </details>

      <details class="inventory-details metadata">
        <summary>{{ t('admin.copy.97ba8d3c') }}</summary>
        <p>{{ t('admin.copy.b53c2bc9') }} {{ inventory.schema_version }} · {{ t('admin.copy.ec6ca803') }}</p>
        <code class="fingerprint"></code>
      </details>
    </template>
  </section>
</template>

<style scoped>
.inventory{color:#252a36}.inventory-head{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.inventory h2{font-size:17px;margin:0 0 5px}.eyebrow{font-size:11px;letter-spacing:1.2px;color:#6674da;font-weight:700;margin:0 0 5px}.sub,.inventory small{color:var(--muted);font-size:11px}.button-quiet{flex-shrink:0;border:1px solid #dfe2ea;border-radius:8px;padding:9px 13px;background:var(--surface);color:inherit}.inventory-empty{color:var(--muted);padding:10px 0}.error{color:#a63732;background:#fff2f0;border-radius:8px;padding:10px}.scope-note{padding:10px 12px;border-radius:8px;background:var(--canvas);color:#60697a;font-size:11px;line-height:1.55}.inventory-stats{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px;margin:14px 0}.inventory-stats>div{background:var(--canvas);border-radius:9px;padding:11px}.inventory-stats b,.inventory-stats span{display:block}.inventory-stats b{font-size:18px;overflow-wrap:anywhere}.inventory-stats span{font-size:11px;color:var(--muted);margin-top:3px}.truncated{padding:10px;background:#fff5df;color:#875700;border-radius:8px;font-size:11px}.issue-count-note{margin:10px 0}.inventory-details{border-top:1px solid var(--line);padding-top:12px;margin-top:14px}.inventory-details summary{cursor:pointer;font-size:12px;font-weight:700}.coverage-table{margin-top:10px}.coverage-row{display:grid;grid-template-columns:minmax(170px,2fr) repeat(3,minmax(70px,.7fr));gap:10px;padding:7px 8px;border-top:1px solid #eef0f4;font-size:11px}.coverage-header{background:var(--canvas);color:var(--muted);font-weight:650;border-radius:6px}.coverage-row code,.issue-row code,.fingerprint{overflow-wrap:anywhere}.issue-list{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,330px),1fr));gap:9px;margin-top:10px}.issue-row{border:1px solid var(--line);border-radius:8px;padding:10px;font-size:11px}.issue-row>b{font-size:11px;color:#a63732}.issue-row dl{display:grid;grid-template-columns:minmax(110px,.6fr) minmax(0,1.4fr);gap:7px 10px;margin:9px 0 0}.issue-row dt{color:var(--muted)}.issue-row dd{margin:0;overflow-wrap:anywhere}.metadata p{font-size:11px;color:var(--muted)}.fingerprint{display:block;font-size:11px;color:var(--muted);word-break:break-all}
@media(max-width:600px){.inventory-head{flex-direction:column}.inventory-stats{grid-template-columns:repeat(2,minmax(0,1fr))}.coverage-row{grid-template-columns:minmax(110px,1.7fr) repeat(3,minmax(42px,.6fr));gap:5px;padding:7px 4px;font-size:11px}.issue-row dl{grid-template-columns:minmax(90px,.7fr) minmax(0,1.3fr)}}
</style>
