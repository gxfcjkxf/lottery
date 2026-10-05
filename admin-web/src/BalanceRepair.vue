<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { AdminAccount } from "./admin-api";
import { AdminApiError } from "./admin-api";
import {
  brandPermissions,
  canViewWallet,
  createBodyKeyTracker,
} from "./finance-api";
import {
  createRepairApi,
  type RepairPreview,
  type RepairHistory,
} from "./repair-api";
const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (e: "session-invalid"): void }>();
const api = createRepairApi();
const keyFor = createBodyKeyTracker();
const member = ref("");
const loaded = ref("");
const preview = ref<RepairPreview | null>(null);
const history = ref<RepairHistory[]>([]);
const reason = ref("");
const acknowledge = ref(false);
const busy = ref(false);
const error = ref("");
const notice = ref("");
let generation = 0;
const view = computed(() => canViewWallet(props.account, props.brandId));
const write = computed(
  () =>
    !props.account.super_admin &&
    brandPermissions(props.account, props.brandId).has("wallet.repair.brand"),
);
function failed(e: unknown) {
  error.value = e instanceof Error ? e.message : "差错处理失败";
  if (e instanceof AdminApiError && e.status === 401) emit("session-invalid");
}
watch(
  () => props.brandId,
  () => {
    generation++;
    preview.value = null;
    history.value = [];
    loaded.value = "";
    reason.value = "";
    acknowledge.value = false;
    notice.value = "";
    error.value = "";
    busy.value = false;
    keyFor.clear();
  },
);
watch(member, () => {
  preview.value = null;
  acknowledge.value = false;
  notice.value = "";
});
async function inspect() {
  const id = member.value.trim();
  if (!/^[0-9a-f-]{36}$/i.test(id)) {
    error.value = "请输入会员 UUID";
    return;
  }
  const gen = ++generation,
    brand = props.brandId;
  busy.value = true;
  error.value = "";
  notice.value = "";
  preview.value = null;
  acknowledge.value = false;
  try {
    const [p, h] = await Promise.all([
      api.preview(brand, id),
      api.history(brand, id),
    ]);
    if (gen !== generation || member.value.trim() !== id) return;
    loaded.value = id;
    preview.value = p;
    history.value = h.items;
  } catch (e) {
    if (gen === generation) failed(e);
  } finally {
    if (gen === generation) busy.value = false;
  }
}
async function repair() {
  const p = preview.value;
  if (
    !p?.repairable ||
    !write.value ||
    !acknowledge.value ||
    !reason.value.trim() ||
    member.value.trim() !== loaded.value
  )
    return;
  const brand = props.brandId,
    id = loaded.value,
    gen = ++generation,
    body = { version: p.version, token: p.token, reason: reason.value.trim() };
  busy.value = true;
  error.value = "";
  try {
    const result = await api.repair(
      brand,
      id,
      body,
      keyFor({ brand, id, ...body }),
    );
    if (gen !== generation) return;
    const [next, h] = await Promise.all([
      api.preview(brand, id),
      api.history(brand, id),
    ]);
    if (gen !== generation) return;
    preview.value = next;
    history.value = h.items;
    notice.value = `余额重建已提交，修复记录 ${result.id}`;
    reason.value = "";
    acknowledge.value = false;
    keyFor.clear();
  } catch (e) {
    if (gen === generation) {
      failed(e);
      preview.value = null;
      acknowledge.value = false;
    }
  } finally {
    if (gen === generation) busy.value = false;
  }
}
</script>
<template>
  <article v-if="view" class="balance-repair">
    <h2>余额差错处理</h2>
    <p>
      仅依据原账本重建余额缓存，不改旧流水，不接受自填余额。账本不完整时必须停止并调查。
    </p>
    <form @submit.prevent="inspect">
      <label
        >差错会员 UUID<input
          v-model="member"
          autocomplete="off"
          :disabled="busy" /></label
      ><button :disabled="busy">检查差错与修复记录</button>
    </form>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="notice" role="status">{{ notice }}</p>
    <section v-if="preview">
      <p>
        {{
          preview.consistent
            ? "账本与余额一致，无需修复"
            : preview.repairable
              ? "账本完整，可以重建余额"
              : "账本完整性无法证明，禁止修复"
        }}
      </p>
      <p>
        当前版本 {{ preview.version }} · 原账本版本 {{ preview.ledger_version }}
      </p>
      <ul v-if="preview.issues.length">
        <li v-for="issue in preview.issues" :key="issue">{{ issue }}</li>
      </ul>
      <div class="snapshots">
        <section>
          <h3>当前余额（含缺失分项）</h3>
          <pre>{{ JSON.stringify(preview.actual, null, 2) }}</pre>
        </section>
        <section>
          <h3>原账本重建值</h3>
          <pre>{{ JSON.stringify(preview.expected, null, 2) }}</pre>
        </section>
      </div>
      <form v-if="write && preview.repairable" @submit.prevent="repair">
        <label
          >余额修复原因<textarea
            v-model="reason"
            maxlength="500"
            required
            :disabled="busy"
          />
        </label>
        <label class="ack"
          ><input
            v-model="acknowledge"
            type="checkbox"
            :disabled="busy"
          />我已核对原账本，确认仅重建上述余额</label
        ><button :disabled="busy || !acknowledge || !reason.trim()">
          确认按原账本修复
        </button>
      </form>
    </section>
    <h3>近期修复记录（最多 50 条）</h3>
    <p v-if="!history.length">暂无修复记录。</p>
    <details v-for="item in history" :key="item.id">
      <summary>{{ item.created_at }} · {{ item.reason }}</summary>
      <p>
        记录 {{ item.id }} · 操作人 {{ item.actor_id }} · 请求
        {{ item.request_id }}
      </p>
      <pre>{{
        JSON.stringify(
          { before: item.before_snapshot, after: item.after_snapshot },
          null,
          2,
        )
      }}</pre>
    </details>
  </article>
</template>
<style scoped>
.balance-repair {
  background: white;
  padding: 20px;
  border: 1px solid #e5e8f2;
  border-radius: 12px;
  display: grid;
  gap: 14px;
  color: #30364a;
}
.balance-repair h2,
.balance-repair h3,
.balance-repair p {
  margin: 0;
}
.balance-repair p {
  font-size: 13px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
form,
label {
  display: grid;
  gap: 10px;
}
input,
textarea {
  width: 100%;
  padding: 10px;
  border: 1px solid #ccd2e2;
  border-radius: 6px;
  box-sizing: border-box;
}
button {
  justify-self: start;
  padding: 10px 14px;
  border: 0;
  border-radius: 6px;
  background: #6366e9;
  color: white;
  cursor: pointer;
}
button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.snapshots {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  min-width: 0;
}
.snapshots section {
  min-width: 0;
}
pre {
  background: #f5f6fa;
  padding: 12px;
  border-radius: 6px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
  max-height: 400px;
  overflow: auto;
}
.ack {
  display: flex;
  align-items: center;
  font-size: 13px;
}
.ack input {
  width: auto;
}
details {
  min-width: 0;
}
summary {
  overflow-wrap: anywhere;
  font-size: 13px;
  cursor: pointer;
}
@media (max-width: 700px) {
  .snapshots {
    grid-template-columns: 1fr;
  }
  .balance-repair {
    padding: 14px;
  }
}
</style>
