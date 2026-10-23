<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import type { LocalizedMessage } from "@lottery/shared";
import { computed, onUnmounted, ref, watch } from "vue";
import { adminKey, useAdminI18n } from "./i18n";
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

const props = defineProps<{ account: AdminAccount; brandId: string; active: boolean }>();
const emit = defineEmits<{ (event: "session-invalid"): void; (event: "select-period", periodId: string): void }>();
const { t, message: localized, display } = useAdminI18n();
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
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const manualError = ref<string | LocalizedMessage>("");
const sourceError = ref<string | LocalizedMessage>("");
const gameNext = ref(false);
const periodNext = ref(false);
const selectedGame = computed(
  () => games.value.find((item) => item.id === gameId.value) ?? null,
);
const selectedPeriod = computed(
  () => periods.value.find((item) => item.id === periodId.value) ?? null,
);
const currentDraw = computed(() => drawPage.value?.current ?? null);
watch(() => [props.active, selectedPeriod.value?.id] as const, ([active, id]) => { if (active) emit("select-period", id ?? ""); });
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
    return cause instanceof Error ? cause.message : t('admin.copy.32c6ed4c');
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
    throw new Error(t('admin.copy.911918b9'));
  return record;
}
function checkGame(record: { game_id: string }) {
  if (record.game_id !== gameId.value)
    throw new Error(t('admin.copy.7a9b4d30'));
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
    throw new Error(t('admin.copy.04523b78'));
}
function periodStatusLabel(value: string) {
  const labels: Record<string, string> = {
    pending: t('admin.copy.997b3d22'), betting: t('admin.copy.3a20b720'),
    closed: t('admin.copy.3198ad9b'), waiting_draw: t('admin.copy.aab1777a'),
    drawn: t('admin.copy.a16e8013'), settling: t('admin.copy.01f90196'),
    settled: t('admin.copy.037164c2'), bet_cancelled: t('admin.copy.a1834cc2'),
    judged_cancelled: t('admin.copy.46a60c2c'),
  };
  return labels[value] ?? value;
}
function drawKindLabel(value: string) {
  return value === "manual" ? t('admin.copy.1e41277a') : value === "automatic" ? t('admin.copy.993400b8') : value;
}
function attemptStatusLabel(value: string) {
  const labels: Record<string, string> = {
    succeeded: t('admin.copy.4d71c40d'), failed: t('admin.copy.1feabec0'),
    pending: t('admin.copy.99f247fd'), skipped: t('admin.copy.9d70d1bf'),
  };
  return labels[value] ?? value;
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
    error.value = cause instanceof Error ? publicErrorMessage(cause) : localized('admin.copy.4fb9cc4e');
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
        throw new Error(t('admin.copy.954e14fc'));
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
        cause instanceof Error ? publicErrorMessage(cause) : localized('admin.copy.a32a8eb9');
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
        throw new Error(t('admin.copy.0073fada'));
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
        throw new Error(t('admin.copy.facb4cf9'));
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
async function selectPeriod(id: string) {
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
  if (rights.value.drawView) await loadHistory(0);
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
      cause instanceof Error ? publicErrorMessage(cause) : localized('admin.copy.eb21f657');
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
      throw new Error(t('admin.copy.6650877e'));
    sourceSet.value = result;
    sources.value = structuredClone(result.sources);
    sourceReason.value = "";
    games.value = games.value.map((item) =>
      item.id === game.id ? { ...item, version: result.game_version } : item,
    );
    notice.value =
      localized('admin.copy.14edbd12');
  } catch (cause) {
    if (current(ticket, rights.value.sourceWrite && rights.value.sourceView)) {
      sourceError.value =
        cause instanceof Error ? publicErrorMessage(cause) : localized('admin.copy.fc98aa46');
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
      cause instanceof Error ? publicErrorMessage(cause) : localized('admin.copy.3091b4b5');
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
    notice.value = localized('admin.copy.8b77a74b');
    const submittedPeriodId = period.id;
    await loadPeriods(periodOffset.value);
    if (periodId.value === submittedPeriodId)
      await loadHistory(drawHistoryOffset.value);
  } catch (cause) {
    if (current(ticket, rights.value.manualCreate && rights.value.drawView)) {
      manualError.value =
        cause instanceof Error ? publicErrorMessage(cause) : localized('admin.copy.1e9aa543');
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
function timestamp(value: string){ return formatDisplayDateTime(value, displayLocale.value); }

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
    if (rights.value.gamesView && props.brandId) void withGlobalLoading(loadGames, 0);
  },
  { immediate: true },
);
onUnmounted(() => guard.invalidate());
</script>

<template>
  <section class="draw-management">
    <header class="page-head">
      <div>
        <h2>{{ t('admin.copy.26bac4a5') }}</h2>
      </div>
      <span class="brand-tag">{{ t('admin.copy.9c32661d') }} · <AssociationName kind="brand" :id="brandId" :locale="associationLocale" /></span>
    </header>
    <p class="server-note">
      {{ t('admin.copy.5c64d079') }}
    </p>
    <div v-if="error" class="notice error" role="alert">{{ display(error) }}</div>
    <p v-if="notice" class="notice success" role="status">{{ display(notice) }}</p>
    <p v-if="!rights.gamesView" class="callout">
      {{ t('admin.copy.3d88d4bd') }}
    </p>
    <div v-else class="draw-content">
      <section class="panel">
        <div class="section-head">
          <div>
            <h3>{{ t('admin.copy.db14a8b4') }}</h3>
          </div>
          <button v-refresh-button="t('admin.copy.4422e4ac')"
            class="button button-secondary"
            :disabled="!!busy.games"
            @click="withGlobalLoading(loadGames, gameOffset)"
          >
            {{ t('admin.copy.4422e4ac') }}
          </button>
        </div>
        <div class="picker-grid">
          <label
            >{{ t('admin.copy.7492bbfb') }}<select
              :value="gameId"
              :disabled="!!busy.games"
              @change="withGlobalLoading(chooseGame, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">{{ t('admin.copy.68ccd146') }}</option>
              <option v-for="game in games" :key="game.id" :value="game.id">
                {{ game.name }}
              </option>
            </select></label
          >
          <div class="page-actions">
            <span>{{ games.length }} {{ t('admin.copy.1015fa3f') }} {{ gameOffset }}</span
            ><button
              class="button button-secondary"
              :disabled="!!busy.games || gameOffset === 0"
              @click="withGlobalLoading(loadGames, Math.max(0, gameOffset - pageSize))"
            >
              {{ t('admin.copy.dbe7a5d7') }}</button
            ><button
              class="button button-secondary"
              :disabled="!!busy.games || !gameNext"
              @click="withGlobalLoading(loadGames, gameOffset + pageSize)"
            >
              {{ t('admin.copy.2e9c9e7a') }}
            </button>
          </div>
          <p v-if="selectedGame" class="muted model-meta">
            {{ t(adminKey('admin.console.model.' + selectedGame.model.model)) }} · {{ selectedGame.timezone }}
          </p>
          <label v-if="rights.periodsView && selectedGame"
            >{{ t('admin.copy.da3c8007') }}<select
              :value="periodId"
              :disabled="!!busy.periods"
              @change="withGlobalLoading(selectPeriod, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">{{ t('admin.copy.d972b3a0') }}</option>
              <option
                v-for="period in periods"
                :key="period.id"
                :value="period.id"
              >
                {{ period.period_no }} · {{ periodStatusLabel(period.status) }}
              </option>
            </select></label
          >
          <div v-if="rights.periodsView && selectedGame" class="page-actions">
            <span>{{ periods.length }} {{ t('admin.copy.1015fa3f') }} {{ periodOffset }}</span
            ><button
              class="button button-secondary"
              :disabled="!!busy.periods || periodOffset === 0"
              @click="withGlobalLoading(loadPeriods, Math.max(0, periodOffset - pageSize))"
            >
              {{ t('admin.copy.dbe7a5d7') }}</button
            ><button
              class="button button-secondary"
              :disabled="!!busy.periods || !periodNext"
              @click="withGlobalLoading(loadPeriods, periodOffset + pageSize)"
            >
              {{ t('admin.copy.2e9c9e7a') }}</button
            ><button v-refresh-button="t('admin.copy.72fdf4a4')"
              class="button button-secondary"
              :disabled="!!busy.periods"
              @click="withGlobalLoading(loadPeriods, periodOffset)"
            >
              {{ t('admin.copy.72fdf4a4') }}
            </button>
          </div>
          <p v-else-if="selectedGame" class="callout">
            {{ t('admin.copy.17c75777') }}
          </p>
          <p v-if="rights.periodsView && selectedGame && !busy.periods && !periods.length" class="callout empty-state">{{ t('admin.copy.30f48614') }}</p>
        </div>
      </section>

      <section
        v-if="selectedGame && (rights.sourceView || rights.sourceWrite)"
        class="panel"
      >
        <div class="section-head">
          <div>
            <h3>{{ t('admin.copy.4ff2b556') }}</h3>
            <p>
              {{ t('admin.copy.3b52011c') }}
            </p>
          </div>
          <button v-refresh-button="t('admin.copy.deca021e')"
            class="button button-secondary"
            :disabled="!!busy.sources"
            @click="withGlobalLoading(loadSources)"
          >
            {{ t('admin.copy.deca021e') }}
          </button>
        </div>
        <p v-if="!rights.sourceView" class="callout">
          {{ t('admin.copy.f1464125') }}
        </p>
        <p v-else-if="sourceSet" class="muted">
          {{ timestamp(sourceSet.created_at) }}
        </p>
        <p v-else-if="!busy.sources" class="muted empty-state">{{ t('admin.copy.71263f39') }}</p>
        <div v-if="rights.sourceView" class="source-list">
          <article
            v-for="(source, index) in sources"
            :key="source.id"
            class="source-card"
          >
            <div class="source-heading">
              <strong>{{ t('admin.copy.d2089b75') }} {{ index + 1 }}</strong
              ><code>{{ source.id }}</code
              ><button
                v-if="rights.sourceWrite"
                class="danger-link"
                :disabled="!!busy['source-write']"
                @click="removeSource(index)"
              >
                {{ t('admin.copy.ad76fa40') }}
              </button>
            </div>
            <div class="source-fields">
              <label
                >{{ t('admin.copy.e19e3eeb') }}<input
                  v-model="source.name"
                  maxlength="120"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
              /></label>
              <label
                >{{ t('admin.copy.214be851') }}<select
                  v-model="source.type"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
                >
                  <option value="api">{{ t('admin.ui.api') }}</option>
                  <option value="dom">{{ t('admin.ui.dom') }}</option>
                </select></label
              >
              <label
                >{{ t('admin.copy.e9b0cdbe') }}<input
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
                >{{ t('admin.copy.5eb01613') }}<input
                  v-model="source.enabled"
                  type="checkbox"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
              /></label>
              <label class="wide"
                >{{ t('admin.copy.e144becb') }}<input
                  v-model="source.endpoint"
                  type="url"
                  maxlength="2048"
                  placeholder="https://feeds.example.com/draw"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
              /></label>
              <label v-if="source.type === 'dom'" class="wide"
                >{{ t('admin.copy.c4efb4da') }}<input
                  v-model="source.selector"
                  maxlength="500"
                  :disabled="!rights.sourceWrite || !!busy['source-write']"
              /></label>
              <label class="wide"
                >{{ t('admin.copy.2f8c8c3b') }}<input
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
              class="button button-secondary"
              :disabled="sources.length >= 16 || !!busy['source-write']"
              @click="addSource"
            >
              {{ t('admin.copy.df8f0555') }}</button
            ><button
              v-write-guard="Boolean(busy['source-write'])"
              :disabled="!!busy['source-write'] "
              @click="withGlobalLoading(saveSources)"
            >
              {{ busy["source-write"] ? t('admin.copy.2462134b') : t('admin.copy.e13b5300') }}
            </button>
          </div>
          <p v-if="sourceError" class="inline-error" role="alert">
            {{ display(sourceError) }}
          </p>
        </div>
      </section>

      <section v-if="selectedPeriod && rights.drawView" class="panel">
        <div class="section-head">
          <div>
            <h3>{{ t('admin.copy.17425e40') }}</h3>
          </div>
          <button v-refresh-button="t('admin.copy.deca021e')"
            class="button button-secondary"
            :disabled="!!busy.history"
            @click="withGlobalLoading(loadHistory, drawHistoryOffset)"
          >
            {{ t('admin.copy.deca021e') }}
          </button>
        </div>
        <div class="current-result">
          <b>{{ t('admin.copy.c56aa572') }}</b
          ><span v-if="currentDraw"
            >{{ resultText(currentDraw) }} · {{ drawKindLabel(currentDraw.kind) }} ·
            {{ timestamp(currentDraw.drawn_at) }}</span
          ><span v-else>{{ t('admin.copy.1867cad7') }}</span>
        </div>
        <h4>
          {{ t('admin.copy.becd7fe2') }} {{ drawPage?.history.length ?? 0 }} {{ t('admin.copy.1015fa3f') }}
          {{ drawHistoryOffset }}
        </h4>
        <div v-if="drawPage?.history.length" class="history-list">
          <article v-for="row in drawPage.history" :key="row.id">
            <div>
              <b>{{ resultText(row) }}</b
              ><span>{{ drawKindLabel(row.kind) }} · {{ timestamp(row.drawn_at) }}</span>
            </div>

          </article>
        </div>
        <p v-else class="muted empty-state">{{ t('admin.copy.3407e578') }}</p>
        <div class="page-actions">
          <span
            >{{ t('admin.copy.7044ea84') }} {{ drawPage?.history.length ?? 0 }} {{ t('admin.copy.c3590ada') }}
            {{ drawPage?.attempts.length ?? 0 }} {{ t('admin.copy.067fed38') }}</span
          ><button
            class="button button-secondary"
            :disabled="!!busy.history || drawHistoryOffset === 0"
            @click="withGlobalLoading(loadHistory, Math.max(0, drawHistoryOffset - 50))"
          >
            {{ t('admin.copy.dbe7a5d7') }}</button
          ><button
            class="button button-secondary"
            :disabled="
              !!busy.history ||
              ((drawPage?.history.length ?? 0) < 50 &&
                (drawPage?.attempts.length ?? 0) < 50)
            "
            @click="withGlobalLoading(loadHistory, drawHistoryOffset + 50)"
          >
            {{ t('admin.copy.2e9c9e7a') }}
          </button>
        </div>
        <h4>{{ t('admin.copy.6103ce3e') }} {{ drawPage?.attempts.length ?? 0 }} {{ t('admin.copy.067fed38') }}</h4>
        <div v-if="drawPage?.attempts.length" class="history-list">
          <article v-for="batch in drawPage.attempts" :key="batch.id">
            <div>
              <b>{{ attemptStatusLabel(batch.status) }}</b
              ><span>{{ timestamp(batch.created_at) }}</span>
            </div>
            <small>{{
              batch.attempts
                .map(
                  (attempt) =>
                    `${attempt.source_id}: ${attemptStatusLabel(attempt.status)}`,
                )
                .join(" · ") || t('admin.copy.2b0430fe')
            }}</small>
          </article>
        </div>
        <p v-else class="muted empty-state">{{ t('admin.copy.ab81f982') }}</p>
        <div v-if="rights.manualCreate" class="manual-entry">
          <h4>{{ t('admin.copy.900b7645') }}</h4>
          <p v-if="currentDraw?.kind === 'manual'" class="callout">
            {{ t('admin.copy.a86f5a0c') }}
          </p>
          <p v-else-if="!canOpenManual" class="callout">
            {{ t('admin.copy.191b0541') }}
          </p>
          <template v-else>
            <div v-if="selectedGame" class="draw-input-grid">
              <label v-if="selectedGame.model.model !== 'DIGITS_0_9'"
                >{{ t('admin.copy.d05b3af0') }}{{
                  selectedGame.model.regular_count
                }}
                {{ t('admin.copy.5a1d35c7') }}<input
                  v-model="drawInput.regular"
                  :disabled="!!busy['manual-write']"
                  placeholder="1, 2, 3"
              /></label>
              <label
                v-if="
                  selectedGame.model.model !== 'DIGITS_0_9' &&
                  selectedGame.model.special_count > 0
                "
                >{{ t('admin.copy.f5a73e76') }}{{ selectedGame.model.special_count }} {{ t('admin.copy.5a1d35c7') }}<input
                  v-model="drawInput.special"
                  :disabled="!!busy['manual-write']"
                  placeholder="4"
              /></label>
              <label
                v-if="selectedGame.model.model === 'DIGITS_0_9'"
                class="wide"
                >{{ t('admin.copy.0168dccf') }}{{ selectedGame.model.length }} {{ t('admin.copy.5c4e146f') }}<input
                  v-model="drawInput.digits"
                  :disabled="!!busy['manual-write']"
                  placeholder="0, 1, 2"
              /></label>
              <label class="wide"
                >{{ t('admin.copy.b78bd29c') }}
                {{ timestamp(selectedPeriod.draw_at) }}）<input
                  v-model="manual.drawnAt"
                  :disabled="!!busy['manual-write']"
                  placeholder="2026-10-06T12:30:00Z"
              /></label>

            </div>
            <p
              class="validation"
              :class="{ invalid: parsedDraw && !parsedDraw.startsWith('{') }"
            >
              <BusinessDetails v-if="parsedDraw.startsWith('{')" :value="JSON.parse(parsedDraw)" :locale="displayLocale" />
              <span v-else-if="parsedDraw">{{ displayPublicError(publicErrorMessage({ status: 400 })) }}</span>
            </p>
            <label class="confirm"
              ><input
                v-model="manual.confirmed"
                type="checkbox"
                :disabled="!!busy['manual-write']"
              />{{ t('admin.copy.2595be5c') }}</label
            >
            <div class="form-actions">
              <button
                v-write-guard="Boolean(busy['manual-write'])"
                :disabled="
                  !!busy['manual-write'] ||
                  !manual.confirmed  ||
                  !parsedDraw.startsWith('{')
                "
                @click="withGlobalLoading(submitManualDraw)"
              >
                {{ busy["manual-write"] ? t('admin.copy.46511dc0') : t('admin.copy.7221edab') }}
              </button>
            </div>
          </template>
          <p v-if="manualError" class="inline-error" role="alert">
            {{ display(manualError) }}
          </p>
        </div>
      </section>
      <p v-else-if="selectedPeriod" class="callout">
        {{ t('admin.copy.52a3d234') }}
      </p>
    </div>
  </section>
</template>

<style scoped>
.draw-content { display: grid; gap: 16px; min-width: 0; }
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
  color: var(--muted);
  font-size: 11px;
  line-height: 1.6;
  margin: 4px 0 0;
}
.eyebrow {
  color: var(--primary);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 1.1px;
  margin: 0;
}
.brand-tag {
  border: 1px solid var(--line);
  border-radius: 14px;
  padding: 5px 9px;
  color: var(--muted);
  font-size: 11px;
  white-space: nowrap;
}
.server-note,
.callout {
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 7px;
  background: var(--surface);
}
.panel {
  min-width: 0;
  padding: 16px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--surface);
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
  font-size: 11px;
}
.picker-grid label { min-width: 0; width: 100%; }
.picker-grid select { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
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
  background: var(--surface);
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
  color: var(--muted);
  font-size: 11px;
}
.page-actions span {
  margin-right: auto;
}
.page-actions button,
.button-secondary,
.form-actions button,
.danger-link {
  border: 1px solid #dfe2e9;
  border-radius: 5px;
  background: var(--surface);
  padding: 7px 9px;
  font-size: 11px;
}
.form-actions > button:last-child {
  border-color: var(--primary);
  background: var(--primary);
  color: var(--surface);
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
  font-size: 11px;
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
  color: var(--muted);
  font-size: 11px;
  overflow-wrap: anywhere;
}
.manual-entry {
  margin-top: 20px;
  padding-top: 15px;
  border-top: 1px solid var(--line);
}
.manual-entry > p {
  color: var(--muted);
  font-size: 11px;
  line-height: 1.6;
}
.validation {
  overflow-wrap: anywhere;
  color: #27714f;
  font-size: 11px;
}
.validation.invalid {
  color: #a94d49;
}
.confirm {
  margin: 12px 0;
  color: #454b59;
  font-size: 11px;
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
  :global(#app .draw-management .picker-grid select) {
    width: 100% !important;
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
