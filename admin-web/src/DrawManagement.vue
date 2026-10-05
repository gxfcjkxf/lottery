<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  buildDrawResult,
  buildManualDrawBody,
  buildSourceSetBody,
  createDrawManagementApi,
  createDrawManagementKeyTracker,
  createDrawManagementRequestGuard,
  drawManagementPermissions,
  type DrawGame,
  type DrawHistoryPage,
  type DrawResult,
  type SourceConfig,
  type SourceSet,
} from "./draw-management-api";
import type { Period } from "./period-schedules-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createDrawManagementApi();
const guard = createDrawManagementRequestGuard();
const keyFor = createDrawManagementKeyTracker();
const rights = computed(() =>
  drawManagementPermissions(props.account, props.brandId),
);
const games = ref<DrawGame[]>([]);
const gameId = ref("");
const periods = ref<Period[]>([]);
const periodId = ref("");
const sourceSet = ref<SourceSet | null>(null);
const sources = ref<SourceConfig[]>([]);
const drawPage = ref<DrawHistoryPage | null>(null);
const drawHistoryOffset = ref(0);
const drawInput = ref({ regular: "", special: "", digits: "" });
const manual = ref({
  drawnAt: new Date().toISOString(),
  reason: "",
  confirmed: false,
});
const sourceReason = ref("");
const gameOffset = ref(0);
const periodOffset = ref(0);
const pageSize = 25;
const busy = ref<Record<string, string>>({});
const error = ref("");
const notice = ref("");
const manualError = ref("");
const sourceError = ref("");
const gameNext = ref(false);
const periodNext = ref(false);
const selectedGame = computed(
  () => games.value.find((item) => item.id === gameId.value) ?? null,
);
const selectedPeriod = computed(
  () => periods.value.find((item) => item.id === periodId.value) ?? null,
);
const currentDraw = computed(() => drawPage.value?.current ?? null);
const canOpenManual = computed(() => {
  const period = selectedPeriod.value;
  const current = currentDraw.value;
  return Boolean(
    rights.value.manualCreate &&
    rights.value.drawView &&
    period &&
    ((period.status === "waiting_draw" && !current) ||
      (period.status === "drawn" && current && current.kind !== "manual")) &&
    !(current?.kind === "manual"),
  );
});
const parsedDraw = computed(() => {
  if (!selectedGame.value) return "";
  try {
    const draw = buildDrawResult(
      selectedGame.value.model,
      drawInput.value.regular,
      drawInput.value.special,
      drawInput.value.digits,
    );
    return JSON.stringify(draw);
  } catch (cause) {
    return cause instanceof Error ? cause.message : "开奖结果输入无效。";
  }
});

function permissionScope() {
  return JSON.stringify([
    props.account.id,
    props.account.super_admin,
    props.account.permissions,
    props.account.permissions_by_brand,
    props.account.platform_permissions,
    props.brandId,
    rights.value,
  ]);
}
function scope(lane: string) {
  return JSON.stringify([
    permissionScope(),
    lane === "games" ? gameOffset.value : gameId.value,
    lane === "periods" ? periodOffset.value : "",
    lane === "history" ? [periodId.value, drawHistoryOffset.value] : "",
    lane === "manual-write" ? periodId.value : "",
  ]);
}
function current(ticket: ReturnType<typeof guard.capture>, permitted: boolean) {
  return guard.isCurrent(ticket, scope(ticket.lane), permitted);
}
function begin(lane: string) {
  const owner = crypto.randomUUID();
  busy.value = { ...busy.value, [lane]: owner };
  return owner;
}
function finish(lane: string, owner: string) {
  if (busy.value[lane] === owner) {
    const next = { ...busy.value };
    delete next[lane];
    busy.value = next;
  }
}
function checkBrand<T extends { brand_id: string }>(record: T): T {
  if (record.brand_id !== props.brandId)
    throw new Error("响应品牌与当前品牌不匹配，请重新读取。");
  return record;
}
function checkGame(record: { game_id: string }) {
  if (record.game_id !== gameId.value)
    throw new Error("响应彩种与当前选择不匹配，请重新读取。");
}
function checkPeriod(record: {
  period_id?: string;
  id?: string;
  game_id: string;
}) {
  if (
    record.game_id !== gameId.value ||
    (record.period_id && record.period_id !== periodId.value)
  )
    throw new Error("响应期数与当前选择不匹配，请重新读取。");
}
function resetManualForm() {
  drawInput.value = { regular: "", special: "", digits: "" };
  manual.value = {
    drawnAt: new Date().toISOString(),
    reason: "",
    confirmed: false,
  };
}
function clearSelection(resetGameOffset = false) {
  guard.invalidate("sources");
  guard.invalidate("periods");
  guard.invalidate("history");
  guard.invalidate("source-write");
  guard.invalidate("manual-write");
  gameId.value = "";
  periodId.value = "";
  periods.value = [];
  sourceSet.value = null;
  sources.value = [];
  drawPage.value = null;
  sourceReason.value = "";
  resetManualForm();
  if (resetGameOffset) gameOffset.value = 0;
  periodOffset.value = 0;
  drawHistoryOffset.value = 0;
  for (const lane of [
    "sources",
    "periods",
    "history",
    "source-write",
    "manual-write",
  ])
    delete busy.value[lane];
}
function reportError(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401) {
    guard.invalidate();
    games.value = [];
    clearSelection(true);
    busy.value = {};
    error.value = "";
    notice.value = "";
    sourceError.value = "";
    manualError.value = "";
    emit("session-invalid");
    return;
  }
  error.value = cause instanceof Error ? cause.message : "请求失败，请重试。";
}

async function loadGames(offset = 0) {
  guard.invalidate("games");
  gameOffset.value = offset;
  if (!rights.value.gamesView || !props.brandId) {
    games.value = [];
    gameNext.value = false;
    clearSelection(true);
    return;
  }
  const brand = props.brandId;
  const ticket = guard.capture("games", scope("games"));
  const owner = begin("games");
  error.value = "";
  try {
    const result = await api.getGames(brand, pageSize, offset);
    if (!current(ticket, rights.value.gamesView)) return;
    result.games.forEach(checkBrand);
    games.value = result.games;
    gameNext.value = result.games.length === pageSize;
    if (!result.games.some((game) => game.id === gameId.value)) {
      clearSelection();
      gameOffset.value = offset;
      const first = result.games[0];
      if (first) await chooseGame(first.id);
    }
  } catch (cause) {
    if (current(ticket, rights.value.gamesView)) {
      games.value = [];
      clearSelection();
      reportError(cause);
    }
  } finally {
    finish("games", owner);
  }
}
async function loadSources() {
  guard.invalidate("sources");
  const game = selectedGame.value;
  if (!game || !rights.value.sourceView) {
    sourceSet.value = null;
    sources.value = [];
    finish("sources", busy.value.sources ?? "");
    return;
  }
  const brand = props.brandId;
  const ticket = guard.capture("sources", scope("sources"));
  const owner = begin("sources");
  sourceError.value = "";
  try {
    const result = await api.getSourceSet(brand, game.id);
    if (!current(ticket, rights.value.sourceView)) return;
    if (result) {
      checkBrand(result);
      if (result.game_id !== game.id)
        throw new Error("来源集彩种与当前选择不匹配。");
      sourceSet.value = result;
      sources.value = structuredClone(result.sources);
      games.value = games.value.map((item) =>
        item.id === game.id ? { ...item, version: result.game_version } : item,
      );
    } else {
      sourceSet.value = null;
      sources.value = [];
    }
  } catch (cause) {
    if (current(ticket, rights.value.sourceView)) {
      sourceSet.value = null;
      sources.value = [];
      sourceError.value =
        cause instanceof Error ? cause.message : "来源读取失败。";
      reportError(cause);
    }
  } finally {
    finish("sources", owner);
  }
}
async function loadPeriods(offset = 0) {
  guard.invalidate("periods");
  periodOffset.value = offset;
  const game = selectedGame.value;
  if (!game || !rights.value.periodsView) {
    periods.value = [];
    periodId.value = "";
    periodNext.value = false;
    return;
  }
  const brand = props.brandId;
  const ticket = guard.capture("periods", scope("periods"));
  const owner = begin("periods");
  error.value = "";
  try {
    const result = await api.getPeriods(brand, game.id, pageSize, offset);
    if (!current(ticket, rights.value.periodsView)) return;
    result.periods.forEach((period) => {
      checkBrand(period);
      if (period.game_id !== game.id)
        throw new Error("期数彩种与当前选择不匹配。");
    });
    periods.value = result.periods;
    periodNext.value = result.periods.length === pageSize;
    if (!result.periods.some((period) => period.id === periodId.value)) {
      guard.invalidate("history");
      periodId.value = result.periods[0]?.id ?? "";
      drawPage.value = null;
      drawHistoryOffset.value = 0;
      if (periodId.value) await loadHistory(0);
    }
  } catch (cause) {
    if (current(ticket, rights.value.periodsView)) {
      periods.value = [];
      periodId.value = "";
      reportError(cause);
    }
  } finally {
    finish("periods", owner);
  }
}
async function loadHistory(offset = 0) {
  guard.invalidate("history");
  drawHistoryOffset.value = offset;
  const game = selectedGame.value;
  const period = selectedPeriod.value;
  if (!game || !period || !rights.value.drawView) {
    drawPage.value = null;
    return;
  }
  const brand = props.brandId;
  const ticket = guard.capture("history", scope("history"));
  const owner = begin("history");
  error.value = "";
  try {
    const result = await api.getDrawHistory(brand, game, period.id, 50, offset);
    if (!current(ticket, rights.value.drawView)) return;
    const rows = [
      ...result.history,
      ...(result.current ? [result.current] : []),
    ];
    rows.forEach((row) => {
      checkBrand(row);
      checkGame(row);
      checkPeriod(row);
    });
    result.attempts.forEach((row) => {
      checkBrand(row);
      checkGame(row);
      if (row.period_id !== period.id)
        throw new Error("尝试批次期数与当前选择不匹配。");
    });
    drawPage.value = result;
  } catch (cause) {
    if (current(ticket, rights.value.drawView)) {
      drawPage.value = null;
      reportError(cause);
    }
  } finally {
    finish("history", owner);
  }
}
async function chooseGame(id: string) {
  if (!games.value.some((item) => item.id === id)) return;
  guard.invalidate("sources");
  guard.invalidate("periods");
  guard.invalidate("history");
  guard.invalidate("source-write");
  guard.invalidate("manual-write");
  gameId.value = id;
  periodId.value = "";
  periods.value = [];
  sourceSet.value = null;
  sources.value = [];
  drawPage.value = null;
  periodOffset.value = 0;
  drawHistoryOffset.value = 0;
  sourceReason.value = "";
  resetManualForm();
  error.value = "";
  notice.value = "";
  sourceError.value = "";
  manualError.value = "";
  delete busy.value["source-write"];
  delete busy.value["manual-write"];
  if (rights.value.sourceView) void loadSources();
  if (rights.value.periodsView) void loadPeriods(0);
}
function selectPeriod(id: string) {
  if (!periods.value.some((item) => item.id === id)) return;
  guard.invalidate("history");
  guard.invalidate("manual-write");
  periodId.value = id;
  drawPage.value = null;
  drawHistoryOffset.value = 0;
  resetManualForm();
  error.value = "";
  notice.value = "";
  sourceError.value = "";
  manualError.value = "";
  delete busy.value["manual-write"];
  if (rights.value.drawView) void loadHistory(0);
}
function addSource() {
  if (sources.value.length >= 16) return;
  sources.value.push({
    id: crypto.randomUUID(),
    name: "",
    type: "api",
    priority: sources.value.length + 1,
    enabled: true,
    endpoint: "",
  });
}
function removeSource(index: number) {
  sources.value.splice(index, 1);
}
function reorderPriority(index: number, priority: number) {
  if (!Number.isSafeInteger(priority) || priority < 1) return;
  const other = sources.value.findIndex(
    (source, itemIndex) => itemIndex !== index && source.priority === priority,
  );
  const previous = sources.value[index].priority;
  sources.value[index].priority = priority;
  if (other >= 0) sources.value[other].priority = previous;
}
async function saveSources() {
  const game = selectedGame.value;
  if (
    !game ||
    !rights.value.sourceWrite ||
    !rights.value.sourceView ||
    busy.value["source-write"]
  )
    return;
  sourceError.value = "";
  error.value = "";
  const version = game.version;
  let body;
  try {
    body = buildSourceSetBody(version, sources.value, sourceReason.value);
  } catch (cause) {
    sourceError.value =
      cause instanceof Error ? cause.message : "来源配置无效。";
    return;
  }
  const key = keyFor(props.brandId, "draw-sources.put", game.id, body);
  const ticket = guard.capture("source-write", scope("source-write"));
  const owner = begin("source-write");
  try {
    const result = await api.updateSourceSet(props.brandId, game.id, body, key);
    if (!current(ticket, rights.value.sourceWrite && rights.value.sourceView))
      return;
    checkBrand(result);
    if (result.game_id !== game.id)
      throw new Error("保存响应的彩种与当前选择不匹配。");
    sourceSet.value = result;
    sources.value = structuredClone(result.sources);
    sourceReason.value = "";
    games.value = games.value.map((item) =>
      item.id === game.id ? { ...item, version: result.game_version } : item,
    );
    notice.value =
      "来源配置已保存为新修订版。API / DOM 适配器尚未连接；此页面不会向来源发起真实网络请求。";
  } catch (cause) {
    if (current(ticket, rights.value.sourceWrite && rights.value.sourceView)) {
      sourceError.value =
        cause instanceof Error ? cause.message : "来源保存失败。";
      reportError(cause);
    }
  } finally {
    finish("source-write", owner);
  }
}
async function submitManualDraw() {
  const game = selectedGame.value;
  const period = selectedPeriod.value;
  if (!game || !period || !canOpenManual.value || busy.value["manual-write"])
    return;
  manualError.value = "";
  error.value = "";
  let body;
  try {
    body = buildManualDrawBody({
      version: period.version,
      period,
      model: game.model,
      regularInput: drawInput.value.regular,
      specialInput: drawInput.value.special,
      digitsInput: drawInput.value.digits,
      drawnAt: manual.value.drawnAt,
      reason: manual.value.reason,
      confirmed: manual.value.confirmed,
    });
  } catch (cause) {
    manualError.value =
      cause instanceof Error ? cause.message : "手动开奖结果无效。";
    return;
  }
  const key = keyFor(props.brandId, "manual-draw.post", period.id, body);
  const ticket = guard.capture("manual-write", scope("manual-write"));
  const owner = begin("manual-write");
  try {
    const result = await api.createManualDraw(
      props.brandId,
      period,
      body,
      key,
      game,
    );
    if (!current(ticket, rights.value.manualCreate && rights.value.drawView))
      return;
    checkBrand(result);
    checkGame(result);
    checkPeriod(result);
    resetManualForm();
    notice.value = "手动开奖结果已保存，原有开奖记录仍保留在历史中。";
    const submittedPeriodId = period.id;
    await loadPeriods(periodOffset.value);
    if (periodId.value === submittedPeriodId)
      await loadHistory(drawHistoryOffset.value);
  } catch (cause) {
    if (current(ticket, rights.value.manualCreate && rights.value.drawView)) {
      manualError.value =
        cause instanceof Error ? cause.message : "手动开奖保存失败。";
      reportError(cause);
    }
  } finally {
    finish("manual-write", owner);
  }
}
function resultText(draw: DrawResult) {
  if (draw.result.digits.length) return draw.result.digits.join(" ");
  return [...draw.result.regular, ...draw.result.special].join(" ");
}
function timestamp(value: string) {
  return new Date(value).toLocaleString();
}

watch(
  permissionScope,
  () => {
    guard.invalidate();
    games.value = [];
    clearSelection(true);
    drawPage.value = null;
    error.value = "";
    notice.value = "";
    sourceError.value = "";
    manualError.value = "";
    if (rights.value.gamesView && props.brandId) void loadGames(0);
  },
  { immediate: true },
);
onUnmounted(() => guard.invalidate());
</script>

<template>
  <section class="draw-management">
    <header class="page-head">
      <div>
        <p class="eyebrow">DRAW MANAGEMENT</p>
        <h2>开奖管理</h2>
        <p>配置来源、查看开奖历史，并按权限手动录入开奖结果。</p>
      </div>
      <span class="brand-tag">品牌 · {{ brandId || "未选择" }}</span>
    </header>
    <p class="server-note">
      API / DOM
      适配器尚未接入，本页目前只保存来源配置，不会向来源发起真实网络请求，也不会执行实际采集。
    </p>
    <div v-if="error" class="notice error" role="alert">{{ error }}</div>
    <p v-if="notice" class="notice success" role="status">{{ notice }}</p>
    <p v-if="!rights.gamesView" class="callout">
      需要 game.view.brand 或 game.view.platform
      才能读取彩种目录；目录权限与来源、期数、开奖权限相互独立。
    </p>
    <div v-else class="draw-content">
      <section class="panel">
        <div class="section-head">
          <div>
            <h3>彩种与期数</h3>
            <p>彩种分页每页最多 25 条；期数分页每页最多 25 条。</p>
          </div>
          <button
            class="secondary"
            :disabled="!!busy.games"
            @click="loadGames(gameOffset)"
          >
            刷新彩种
          </button>
        </div>
        <div class="picker-grid">
          <label
            >彩种<select
              :value="gameId"
              :disabled="!!busy.games"
              @change="chooseGame(($event.target as HTMLSelectElement).value)"
            >
              <option value="">选择彩种</option>
              <option v-for="game in games" :key="game.id" :value="game.id">
                {{ game.name }} · {{ game.code }}
              </option>
            </select></label
          >
          <div class="page-actions">
            <span>{{ games.length }} 条 · 偏移 {{ gameOffset }}</span
            ><button
              class="secondary"
              :disabled="!!busy.games || gameOffset === 0"
              @click="loadGames(Math.max(0, gameOffset - pageSize))"
            >
              上一页</button
            ><button
              class="secondary"
              :disabled="!!busy.games || !gameNext"
              @click="loadGames(gameOffset + pageSize)"
            >
              下一页
            </button>
          </div>
          <p v-if="selectedGame" class="muted model-meta">
            {{ selectedGame.model.model }} · {{ selectedGame.timezone }} ·
            彩种版本 {{ selectedGame.version }}
          </p>
          <label v-if="rights.periodsView && selectedGame"
            >期数<select
              :value="periodId"
              :disabled="!!busy.periods"
              @change="selectPeriod(($event.target as HTMLSelectElement).value)"
            >
              <option value="">选择期数</option>
              <option
                v-for="period in periods"
                :key="period.id"
                :value="period.id"
              >
                {{ period.period_no }} · {{ period.status }}
              </option>
            </select></label
          >
          <div v-if="rights.periodsView && selectedGame" class="page-actions">
            <span>{{ periods.length }} 条 · 偏移 {{ periodOffset }}</span
            ><button
              class="secondary"
              :disabled="!!busy.periods || periodOffset === 0"
              @click="loadPeriods(Math.max(0, periodOffset - pageSize))"
            >
              上一页</button
            ><button
              class="secondary"
              :disabled="!!busy.periods || !periodNext"
              @click="loadPeriods(periodOffset + pageSize)"
            >
              下一页</button
            ><button
              class="secondary"
              :disabled="!!busy.periods"
              @click="loadPeriods(periodOffset)"
            >
              刷新期数
            </button>
          </div>
          <p v-else-if="selectedGame" class="callout">
            当前账号缺少 period.view.brand / period.view.platform 期数查看权限。
          </p>
        </div>
      </section>

      <section
        v-if="selectedGame && (rights.sourceView || rights.sourceWrite)"
        class="panel"
      >
        <div class="section-head">
          <div>
            <h3>自动开奖来源</h3>
            <p>
              最多 16
              个来源；完整集合保存会创建新修订版。移除来源不会修改不可变历史。
            </p>
          </div>
          <button
            class="secondary"
            :disabled="!!busy.sources"
            @click="loadSources"
          >
            重新读取
          </button>
        </div>
        <p v-if="!rights.sourceView" class="callout">
          需要 draw_source.view.brand 或 draw_source.view.platform
          才能读取并安全更新完整来源集。
        </p>
        <p v-else-if="sourceSet" class="muted">
          修订 {{ sourceSet.revision }} · 彩种版本
          {{ sourceSet.game_version }} · {{ timestamp(sourceSet.created_at) }}
        </p>
        <p v-else-if="!busy.sources" class="muted">尚无来源配置。</p>
        <div v-if="rights.sourceView" class="source-list">
          <article
            v-for="(source, index) in sources"
            :key="source.id"
            class="source-card"
          >
            <div class="source-heading">
              <strong>来源 {{ index + 1 }}</strong
              ><code>{{ source.id }}</code
              ><button
                v-if="rights.sourceWrite"
                class="danger-link"
                :disabled="!!busy['source-write']"
                @click="removeSource(index)"
              >
                从新修订中移除
              </button>
            </div>
            <div class="source-fields">
              <label
                >名称<input
                  v-model="source.name"
                  maxlength="120"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
              /></label>
              <label
                >类型<select
                  v-model="source.type"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
                >
                  <option value="api">API</option>
                  <option value="dom">DOM</option>
                </select></label
              >
              <label
                >优先级<input
                  type="number"
                  min="1"
                  step="1"
                  :value="source.priority"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
                  @change="
                    reorderPriority(
                      index,
                      Number(($event.target as HTMLInputElement).value),
                    )
                  "
              /></label>
              <label class="check"
                >启用<input
                  v-model="source.enabled"
                  type="checkbox"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
              /></label>
              <label class="wide"
                >HTTPS 公网 DNS 地址（仅 443 端口）<input
                  v-model="source.endpoint"
                  type="url"
                  maxlength="2048"
                  placeholder="https://feeds.example.com/draw"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
              /></label>
              <label v-if="source.type === 'dom'" class="wide"
                >DOM 选择器<input
                  v-model="source.selector"
                  maxlength="500"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
              /></label>
              <label class="wide"
                >凭据引用（仅标识符；不得填写实际密钥）<input
                  v-model="source.credential_ref"
                  maxlength="120"
                  autocomplete="off"
                  placeholder="draw/provider-token"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
              /></label>
            </div>
          </article>
          <div v-if="rights.sourceWrite" class="form-actions">
            <button
              class="secondary"
              :disabled="sources.length >= 16 || !!busy['source-write']"
              @click="addSource"
            >
              添加来源</button
            ><label class="reason"
              >修订原因<input
                v-model="sourceReason"
                maxlength="500"
                :disabled="!!busy['source-write']"
                placeholder="说明本次配置变更" /></label
            ><button
              :disabled="!!busy['source-write'] || !sourceReason.trim()"
              @click="saveSources"
            >
              {{ busy["source-write"] ? "保存中…" : "保存新修订" }}
            </button>
          </div>
          <p v-if="sourceError" class="inline-error" role="alert">
            {{ sourceError }}
          </p>
        </div>
      </section>

      <section v-if="selectedPeriod && rights.drawView" class="panel">
        <div class="section-head">
          <div>
            <h3>开奖结果与尝试历史</h3>
            <p>
              按 50 条分页读取实际历史。worker 尝试记录只读；没有手动抓取入口。
            </p>
          </div>
          <button
            class="secondary"
            :disabled="!!busy.history"
            @click="loadHistory(drawHistoryOffset)"
          >
            重新读取
          </button>
        </div>
        <div class="current-result">
          <b>当前开奖结果</b
          ><span v-if="currentDraw"
            >{{ resultText(currentDraw) }} · {{ currentDraw.kind }} ·
            {{ timestamp(currentDraw.drawn_at) }}</span
          ><span v-else>暂无当前开奖结果</span>
        </div>
        <h4>
          开奖记录 · {{ drawPage?.history.length ?? 0 }} 条 · 偏移
          {{ drawHistoryOffset }}
        </h4>
        <div v-if="drawPage?.history.length" class="history-list">
          <article v-for="row in drawPage.history" :key="row.id">
            <div>
              <b>{{ resultText(row) }}</b
              ><span>{{ row.kind }} · {{ timestamp(row.drawn_at) }}</span>
            </div>
            <small
              >{{ row.result_hash
              }}<template v-if="row.corrected_from_id">
                · 修正自 {{ row.corrected_from_id }}</template
              ></small
            >
          </article>
        </div>
        <p v-else class="muted">此页没有历史记录。</p>
        <div class="page-actions">
          <span
            >开奖 {{ drawPage?.history.length ?? 0 }} 条 · 尝试
            {{ drawPage?.attempts.length ?? 0 }} 条</span
          ><button
            class="secondary"
            :disabled="!!busy.history || drawHistoryOffset === 0"
            @click="loadHistory(Math.max(0, drawHistoryOffset - 50))"
          >
            上一页</button
          ><button
            class="secondary"
            :disabled="
              !!busy.history ||
              ((drawPage?.history.length ?? 0) < 50 &&
                (drawPage?.attempts.length ?? 0) < 50)
            "
            @click="loadHistory(drawHistoryOffset + 50)"
          >
            下一页
          </button>
        </div>
        <h4>Worker 尝试批次 · {{ drawPage?.attempts.length ?? 0 }} 条</h4>
        <div v-if="drawPage?.attempts.length" class="history-list">
          <article v-for="batch in drawPage.attempts" :key="batch.id">
            <div>
              <b>{{ batch.status }}</b
              ><span>{{ timestamp(batch.created_at) }}</span>
            </div>
            <small>{{
              batch.attempts
                .map(
                  (attempt) =>
                    `${attempt.source_id}: ${attempt.status} (${attempt.code})`,
                )
                .join(" · ") || "无来源尝试详情"
            }}</small>
          </article>
        </div>
        <p v-else class="muted">此页没有 worker 尝试批次。</p>
        <div v-if="rights.manualCreate" class="manual-entry">
          <h4>手动录入 / 外部覆盖</h4>
          <p v-if="currentDraw?.kind === 'manual'" class="callout">
            当前开奖结果已由人工录入，不能覆盖；未来修正需等待专门的修正流程。
          </p>
          <p v-else-if="!canOpenManual" class="callout">
            仅 waiting_draw，或已有非人工结果的 drawn 期数可录入；settling
            状态不可操作。手动入口同时受当前结果可见权限约束。
          </p>
          <template v-else>
            <div v-if="selectedGame" class="draw-input-grid">
              <label v-if="selectedGame.model.model !== 'DIGITS_0_9'"
                >普通号码（逗号分隔，{{
                  selectedGame.model.regular_count
                }}
                个）<input
                  v-model="drawInput.regular"
                  :disabled="!!busy['manual-write']"
                  placeholder="1, 2, 3"
              /></label>
              <label
                v-if="
                  selectedGame.model.model !== 'DIGITS_0_9' &&
                  selectedGame.model.special_count > 0
                "
                >特别号码（{{ selectedGame.model.special_count }} 个）<input
                  v-model="drawInput.special"
                  :disabled="!!busy['manual-write']"
                  placeholder="4"
              /></label>
              <label
                v-if="selectedGame.model.model === 'DIGITS_0_9'"
                class="wide"
                >数字位置（逗号分隔，{{ selectedGame.model.length }} 位，每位
                0–9）<input
                  v-model="drawInput.digits"
                  :disabled="!!busy['manual-write']"
                  placeholder="0, 1, 2"
              /></label>
              <label class="wide"
                >开奖时间（UTC RFC3339；不早于期数计划开奖时间
                {{ timestamp(selectedPeriod.draw_at) }}）<input
                  v-model="manual.drawnAt"
                  :disabled="!!busy['manual-write']"
                  placeholder="2026-10-06T12:30:00Z"
              /></label>
              <label class="wide"
                >操作原因（必填）<textarea
                  v-model="manual.reason"
                  maxlength="500"
                  :disabled="!!busy['manual-write']"
                  rows="2"
                  placeholder="说明外部开奖结果来源及覆盖原因"
                ></textarea>
              </label>
            </div>
            <p
              class="validation"
              :class="{ invalid: parsedDraw && !parsedDraw.startsWith('{') }"
            >
              {{
                parsedDraw.startsWith("{")
                  ? `将录入：${parsedDraw}`
                  : parsedDraw
              }}
            </p>
            <label class="confirm"
              ><input
                v-model="manual.confirmed"
                type="checkbox"
                :disabled="!!busy['manual-write']"
              />我确认这是核实后的真实开奖结果，并理解该记录会持久保存和审计。</label
            >
            <div class="form-actions">
              <button
                :disabled="
                  !!busy['manual-write'] ||
                  !manual.confirmed ||
                  !manual.reason.trim() ||
                  !parsedDraw.startsWith('{')
                "
                @click="submitManualDraw"
              >
                {{ busy["manual-write"] ? "提交中…" : "确认并持久保存" }}
              </button>
            </div>
          </template>
          <p v-if="manualError" class="inline-error" role="alert">
            {{ manualError }}
          </p>
        </div>
      </section>
      <p v-else-if="selectedPeriod" class="callout">
        当前账号缺少 draw.view.brand / draw.view.platform 开奖历史查看权限。
      </p>
    </div>
  </section>
</template>

<style scoped>
.draw-management {
  display: grid;
  gap: 14px;
  padding-bottom: 24px;
  min-width: 0;
}
.page-head,
.section-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
}
.page-head h2 {
  margin: 2px 0 5px;
  font-size: 20px;
}
.page-head p:last-child,
.section-head p,
.server-note,
.muted {
  color: #737b8c;
  font-size: 11px;
  line-height: 1.6;
  margin: 4px 0 0;
}
.eyebrow {
  color: #5969df;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 1.1px;
  margin: 0;
}
.brand-tag {
  border: 1px solid #e9ebf0;
  border-radius: 14px;
  padding: 5px 9px;
  color: #737b8c;
  font-size: 10px;
  white-space: nowrap;
}
.server-note,
.callout {
  padding: 10px 12px;
  border: 1px solid #e9ebf0;
  border-radius: 7px;
  background: #fff;
}
.panel {
  min-width: 0;
  padding: 16px;
  border: 1px solid #e9ebf0;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 2px 8px #1e2a8008;
}
.section-head {
  align-items: center;
  margin-bottom: 14px;
}
.section-head h3,
.manual-entry h4 {
  margin: 0;
  font-size: 14px;
}
.picker-grid,
.source-fields,
.draw-input-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  align-items: end;
}
.picker-grid label,
.source-fields label,
.draw-input-grid label,
.reason {
  display: grid;
  gap: 5px;
  color: #555e6e;
  font-size: 10px;
}
.picker-grid select,
.source-fields input,
.source-fields select,
.draw-input-grid input,
.draw-input-grid textarea,
.reason input {
  width: 100%;
  min-width: 0;
  border: 1px solid #dfe2e9;
  border-radius: 5px;
  padding: 8px;
  background: #fff;
  color: #252a36;
  font-size: 11px;
}
.wide {
  grid-column: 1/-1;
}
.page-actions,
.form-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.page-actions {
  align-self: end;
  color: #737b8c;
  font-size: 10px;
}
.page-actions span {
  margin-right: auto;
}
.page-actions button,
.secondary,
.form-actions button,
.danger-link {
  border: 1px solid #dfe2e9;
  border-radius: 5px;
  background: #fff;
  padding: 7px 9px;
  font-size: 10px;
}
.form-actions > button:last-child {
  border-color: #5969df;
  background: #5969df;
  color: #fff;
}
.model-meta {
  grid-column: 1/-1;
}
.source-list {
  display: grid;
  gap: 10px;
}
.source-card {
  min-width: 0;
  padding: 12px;
  border: 1px solid #eceef2;
  border-radius: 7px;
}
.source-heading {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 10px;
}
.source-heading code {
  min-width: 0;
  overflow-wrap: anywhere;
  color: #8c93a0;
  font-size: 9px;
}
.source-heading .danger-link {
  margin-left: auto;
  color: #aa5653;
}
.source-fields {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}
.source-fields .check,
.confirm {
  display: flex;
  align-items: center;
  gap: 7px;
}
.source-fields .check input,
.confirm input {
  width: auto;
}
.reason {
  min-width: 220px;
  flex: 1;
}
.inline-error,
.notice.error {
  color: #a94d49;
}
.inline-error {
  margin: 0;
  font-size: 11px;
}
.notice {
  padding: 10px;
  border-radius: 6px;
}
.notice.success {
  color: #217452;
  background: #eff9f4;
}
.notice.error {
  background: #fff4f2;
}
.current-result {
  display: flex;
  justify-content: space-between;
  gap: 14px;
  padding: 12px;
  border-radius: 6px;
  background: #f6f7fb;
  font-size: 11px;
}
.panel h4 {
  margin: 16px 0 8px;
  font-size: 12px;
}
.history-list {
  display: grid;
  gap: 7px;
}
.history-list article {
  display: grid;
  gap: 5px;
  padding: 10px;
  border: 1px solid #eceef2;
  border-radius: 6px;
}
.history-list article > div {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
}
.history-list span,
.history-list small {
  color: #737b8c;
  font-size: 10px;
  overflow-wrap: anywhere;
}
.manual-entry {
  margin-top: 20px;
  padding-top: 15px;
  border-top: 1px solid #e9ebf0;
}
.manual-entry > p {
  color: #737b8c;
  font-size: 10px;
  line-height: 1.6;
}
.validation {
  overflow-wrap: anywhere;
  color: #27714f;
  font-size: 10px;
}
.validation.invalid {
  color: #a94d49;
}
.confirm {
  margin: 12px 0;
  color: #454b59;
  font-size: 10px;
  line-height: 1.6;
}
button:disabled,
input:disabled,
select:disabled,
textarea:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
@media (max-width: 700px) {
  .panel {
    padding: 12px;
  }
  .page-head,
  .section-head {
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .picker-grid,
  .source-fields,
  .draw-input-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .wide,
  .model-meta {
    grid-column: auto;
  }
  .page-actions {
    width: 100%;
  }
  .page-actions span {
    flex-basis: 100%;
  }
  .current-result {
    flex-direction: column;
  }
  .source-heading code {
    flex-basis: 100%;
    order: 3;
  }
  .source-heading .danger-link {
    margin-left: 0;
  }
  .reason {
    min-width: 100%;
  }
  .form-actions {
    align-items: stretch;
  }
  .form-actions > * {
    flex: 1 1 100%;
  }
}
</style>
