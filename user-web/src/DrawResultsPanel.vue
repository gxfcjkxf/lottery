<script setup lang="ts">
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import DetailModal from "@lottery/shared/DetailModal.vue"
import { computed, onUnmounted, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { createBettingApi, type CatalogGame } from "./betting-api";
import {
  createDrawsApi,
  type PublicDrawResult,
  type PublicPeriod,
} from "./draws-api";
import {
  formatDrawDateTime,
  isUuid,
  isValidPeriodFilter,
  isCancelledPeriodStatus,
  periodStatusLabel,
  resultNumberGroups,
  shiftedPageOffset,
} from "./draw-results-state";
import { useUserI18n } from "./i18n";

const props = withDefaults(
  defineProps<{
    brandCode: string | undefined;
    locale: "zh" | "en";
    gameId?: string;
    compact?: boolean;
  }>(),
  { compact: false },
);

const route = useRoute();
const ui = useUserI18n().t;
const detailLabels = computed(() => ({ title: ui("draw.details"), close: ui("draw.close"), result: ui("draw.resultDetails") }));
const activeTab = ref<"results" | "history">("results");
const selectedGameId = ref("");
const periodInput = ref("");
const games = ref<CatalogGame[]>([]);
const periodGame = ref<CatalogGame | null>(null);
const rows = ref<PublicDrawResult[]>([]);
const periods = ref<PublicPeriod[]>([]);
const detail = ref<PublicDrawResult | null>(null);
const selectedResultId = ref("");
const catalogLoading = ref(false);
const catalogError = ref("");
const loading = ref(false);
const error = ref("");
const detailLoading = ref(false);
const detailError = ref("");
const pageOffset = ref(0);
const hasMore = ref(false);
const brandStatus = ref<"active" | "paused" | "">("");
const periodError = ref("");
const gameFilterError = ref("");
const invalidInitialPeriod = ref(false);
let contextGeneration = 0;
let listGeneration = 0;
let catalogGeneration = 0;
let detailGeneration = 0;
let disposed = false;
const pageSize = 50;

const selectedGame = computed(
  () =>
    games.value.find((game) => game.id === selectedGameId.value) ??
    (periodGame.value?.id === selectedGameId.value ? periodGame.value : null),
);
const gameId = computed(() => selectedGameId.value);
const filterValid = computed(() => isValidPeriodFilter(periodInput.value));
const canPrevious = computed(() => pageOffset.value > 0 && !loading.value);
const canNext = computed(() => hasMore.value && !loading.value);

function initialFilters() {
  const routeGameProvided = route.query.game_id !== undefined;
  const routeGameValid =
    typeof route.query.game_id === "string" && isUuid(route.query.game_id);
  const propGameProvided = props.gameId !== undefined;
  const propGameValid = isUuid(props.gameId);
  const routePeriodProvided = route.query.period_no !== undefined;
  const routePeriodValid =
    typeof route.query.period_no === "string" &&
    isValidPeriodFilter(route.query.period_no);
  return {
    gameId: props.compact
      ? propGameValid
        ? props.gameId!
        : ""
      : routeGameValid
        ? (route.query.game_id as string)
        : routeGameProvided
          ? ""
          : propGameValid
            ? props.gameId!
            : "",
    invalidGame: props.compact
      ? !propGameValid
      : routeGameProvided
        ? !routeGameValid
        : propGameProvided && !propGameValid,
    periodNo:
      !props.compact &&
      routePeriodProvided &&
      typeof route.query.period_no === "string"
        ? route.query.period_no
        : "",
    invalidPeriod: !props.compact && routePeriodProvided && !routePeriodValid,
  };
}

function clearDisplay() {
  detailGeneration++;
  rows.value = [];
  periods.value = [];
  periodGame.value = null;
  detail.value = null;
  selectedResultId.value = "";
  detailError.value = "";
  detailLoading.value = false;
  hasMore.value = false;
  error.value = "";
}

function resetResults() {
  clearDisplay();
  pageOffset.value = 0;
}

function apiErrorMessage(cause: unknown): string {
  const status = (cause as { status?: number } | null)?.status;
  if (status === 400)
    return ui("draw.invalidFilter");
  if (status === 404)
    return ui("draw.notFound");
  if (status === 503)
    return ui("draw.serviceUnavailable");
  if (status === 0)
    return ui("draw.networkFailed");
  return ui("draw.unavailable");
}

async function loadGames(generation: number) {
  const currentCatalog = ++catalogGeneration;
  games.value = [];
  catalogError.value = "";
  catalogLoading.value = true;
  try {
    const api = createBettingApi({ brandCode: props.brandCode });
    const all: CatalogGame[] = [];
    let offset = 0;
    while (offset <= 1_000_000) {
      const page = await api.games(100, offset);
      if (
        disposed ||
        generation !== contextGeneration ||
        currentCatalog !== catalogGeneration
      )
        return;
      const items = Array.isArray(page.items) ? page.items : [];
      all.push(...items);
      if (items.length < 100) break;
      offset += items.length;
    }
    if (
      generation === contextGeneration &&
      currentCatalog === catalogGeneration
    )
      games.value = all;
  } catch (cause) {
    if (
      generation === contextGeneration &&
      currentCatalog === catalogGeneration
    )
      catalogError.value = apiErrorMessage(cause);
  } finally {
    if (
      generation === contextGeneration &&
      currentCatalog === catalogGeneration
    )
      catalogLoading.value = false;
  }
}

async function loadCurrent() {
  const generation = ++listGeneration;
  const context = contextGeneration;
  clearDisplay();
  if (
    !props.compact &&
    activeTab.value === "history" &&
    !isUuid(selectedGameId.value)
  ) {
    loading.value = false;
    return;
  }
  if (
    gameFilterError.value ||
    (props.compact && !isUuid(selectedGameId.value))
  ) {
    error.value =
      gameFilterError.value ||
      ui("draw.invalidGame");
    loading.value = false;
    return;
  }
  if (invalidInitialPeriod.value || !filterValid.value) {
    periodError.value = ui("draw.invalidPeriod");
    loading.value = false;
    return;
  }
  periodError.value = "";
  loading.value = true;
  error.value = "";
  try {
    const api = createDrawsApi(props.brandCode);
    if (!props.compact && activeTab.value === "history") {
      const response = await api.listPeriods(selectedGameId.value, {
        periodNo: periodInput.value || undefined,
        limit: pageSize,
        offset: pageOffset.value,
      });
      if (
        disposed ||
        context !== contextGeneration ||
        generation !== listGeneration
      )
        return;
      periods.value = response.items;
      periodGame.value = response.game;
      hasMore.value = response.has_more;
      brandStatus.value = response.brand_status;
    } else {
      const response = await api.listResults({
        gameId: isUuid(selectedGameId.value) ? selectedGameId.value : undefined,
        periodNo: periodInput.value || undefined,
        limit: props.compact ? 1 : pageSize,
        offset: props.compact ? 0 : pageOffset.value,
      });
      if (
        disposed ||
        context !== contextGeneration ||
        generation !== listGeneration
      )
        return;
      rows.value = response.items;
      hasMore.value = response.has_more;
      brandStatus.value = response.brand_status;
    }
  } catch (cause) {
    if (
      !disposed &&
      context === contextGeneration &&
      generation === listGeneration
    )
      error.value = apiErrorMessage(cause);
  } finally {
    if (
      !disposed &&
      context === contextGeneration &&
      generation === listGeneration
    )
      loading.value = false;
  }
}

async function openResult(id: string) {
  if (!isUuid(id)) return;
  const generation = ++detailGeneration;
  const context = contextGeneration;
  selectedResultId.value = id;
  detail.value = null;
  detailError.value = "";
  detailLoading.value = true;
  try {
    const response = await createDrawsApi(props.brandCode).getResult(id);
    if (
      disposed ||
      context !== contextGeneration ||
      generation !== detailGeneration
    )
      return;
    detail.value = response.item;
    brandStatus.value = response.brand_status;
  } catch (cause) {
    if (
      !disposed &&
      context === contextGeneration &&
      generation === detailGeneration
    )
      detailError.value = apiErrorMessage(cause);
  } finally {
    if (
      !disposed &&
      context === contextGeneration &&
      generation === detailGeneration
    )
      detailLoading.value = false;
  }
}

function closeDetail() {
  detailGeneration++;
  selectedResultId.value = "";
  detail.value = null;
  detailError.value = "";
  detailLoading.value = false;
}

async function changeTab(tab: "results" | "history") {
  activeTab.value = tab;
  pageOffset.value = 0;
  detailGeneration++;
  detail.value = null;
  detailError.value = "";
  await loadCurrent();
}

async function setPeriod(value: string) {
  // Enter and the subsequent blur/change can submit the same filter. Do not
  // restart a live read for an unchanged value (Refresh is the explicit retry).
  if (value === periodInput.value && !invalidInitialPeriod.value) return;
  periodInput.value = value;
  invalidInitialPeriod.value = false;
  pageOffset.value = 0;
  await loadCurrent();
}

async function setGame(value: string) {
  gameFilterError.value = "";
  selectedGameId.value = isUuid(value) ? value : "";
  pageOffset.value = 0;
  detailGeneration++;
  detail.value = null;
  detailError.value = "";
  await loadCurrent();
}

async function movePage(direction: -1 | 1) {
  if (
    (direction < 0 && !canPrevious.value) ||
    (direction > 0 && !canNext.value)
  )
    return;
  pageOffset.value = shiftedPageOffset(pageOffset.value, direction, pageSize);
  await loadCurrent();
}

async function retry() {
  const reads: Promise<void>[] = [];
  if (catalogError.value) reads.push(loadGames(contextGeneration));
  reads.push(loadCurrent());
  await Promise.all(reads);
}

watch(
  () => [
    props.brandCode,
    props.gameId,
    props.compact,
    route.query.game_id,
    route.query.period_no,
  ],
  () => {
    const generation = ++contextGeneration;
    listGeneration++;
    detailGeneration++;
    resetResults();
    brandStatus.value = "";
    const filters = initialFilters();
    selectedGameId.value = filters.gameId;
    periodInput.value = filters.periodNo;
    gameFilterError.value = filters.invalidGame
      ? ui("draw.invalidGame")
      : "";
    periodError.value = filters.invalidPeriod
      ? ui("draw.invalidPeriod")
      : "";
    invalidInitialPeriod.value = filters.invalidPeriod;
    activeTab.value = "results";
    if (!props.compact) void loadGames(generation);
    else {
      games.value = [];
      catalogError.value = "";
    }
    void loadCurrent();
  },
  { immediate: true },
);

watch(
  () => props.locale,
  () => {
    if (
      periodError.value &&
      (invalidInitialPeriod.value || !filterValid.value)
    ) {
      periodError.value = ui("draw.periodTooLong");
    }
  },
);

onUnmounted(() => {
  disposed = true;
  contextGeneration++;
  listGeneration++;
  catalogGeneration++;
  detailGeneration++;
});
</script>

<template>
  <section class="draw-results" :class="{ 'draw-results--compact': compact }">
    <header class="dr-header">
      <div>
        <h2>{{ ui("draw.results") }}</h2>
      </div>
      <Teleport defer to="#user-page-refresh" :disabled="compact"><button v-refresh-button="loading ? ui('draw.loading') : ui('draw.refresh')"
        class="dr-button dr-refresh"
        type="button"
        :disabled="loading"
        @click="withGlobalLoading(retry)"
      >
        {{ loading ? (ui("draw.loading")) : ui("draw.refresh") }}
      </button></Teleport>
    </header>

    <div v-if="!compact" class="dr-controls">
      <div
        class="dr-tabs"
        role="tablist"
        :aria-label="ui('draw.resultsView')"
      >
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === 'results'"
          :class="{ 'is-active': activeTab === 'results' }"
          @click="withGlobalLoading(changeTab, 'results')"
        >
          {{ ui("draw.resultsTab") }}
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === 'history'"
          :class="{ 'is-active': activeTab === 'history' }"
          @click="withGlobalLoading(changeTab, 'history')"
        >
          {{ ui("draw.historyTab") }}
        </button>
      </div>
      <label class="dr-field">
        <select
          :value="selectedGameId"
          :aria-label="ui('draw.game')"
          @change="withGlobalLoading(setGame, ($event.target as HTMLSelectElement).value)"
        >
          <option value="">{{ ui("draw.allGames") }}</option>
          <option v-for="game in games" :key="game.id" :value="game.id">
            {{ game.name }}
          </option>
        </select>
      </label>
      <label class="dr-field dr-period-field">
        <input
          :value="periodInput"
          :aria-label="ui('draw.exactPeriod')"
          type="text"
          maxlength="80"
          :placeholder="ui('draw.periodPlaceholder')"
          @change="withGlobalLoading(setPeriod, ($event.target as HTMLInputElement).value)"
          @keydown.enter="withGlobalLoading(setPeriod, ($event.target as HTMLInputElement).value)"
        />
      </label>
    </div>

    <p v-if="catalogError" class="dr-message dr-error" role="alert">
      {{ catalogError }}
      <button
        class="dr-inline-button"
        type="button"
        @click="withGlobalLoading(loadGames, contextGeneration)"
      >
        {{ ui("draw.retryCatalog") }}
      </button>
    </p>
    <p v-if="periodError" class="dr-message dr-error" role="alert">
      {{ periodError }}
    </p>
    <p v-if="gameFilterError" class="dr-message dr-error" role="alert">
      {{ gameFilterError }}
    </p>
    <p v-if="brandStatus === 'paused'" class="dr-message dr-paused">
      {{
        ui("draw.brandPaused")
      }}
    </p>
    <p v-if="error" class="dr-message dr-error" role="alert">
      {{ error }}
      <button class="dr-inline-button" type="button" @click="withGlobalLoading(retry)">
        {{ ui("betting.retry") }}
      </button>
    </p>

    <div v-global-loading v-if="loading" class="dr-state" role="status">
      {{ ui("draw.loadingPublicRecords") }}
    </div>
    <div v-global-loading
      v-else-if="catalogLoading && !compact && !games.length && !catalogError"
      class="dr-state"
      role="status"
    >
      {{ ui("draw.loadingCatalog") }}
    </div>
    <div
      v-else-if="
        !error && !periodError && activeTab === 'history' && !selectedGameId
      "
      class="dr-state empty-state"
    >
      {{
        ui("draw.chooseGameHistory")
      }}
    </div>
    <div
      v-else-if="
        !error && !periodError && activeTab === 'history' && !periods.length
      "
      class="dr-state empty-state"
    >
      {{ ui("draw.noHistory") }}
    </div>
    <div
      v-else-if="
        !error && !periodError && activeTab === 'results' && !rows.length
      "
      class="dr-state empty-state"
    >
      {{ ui("draw.noResults") }}
    </div>

    <div
      v-if="
        !loading &&
        !error &&
        !periodError &&
        activeTab === 'results' &&
        rows.length
      "
      class="dr-list"
    >
      <article v-for="item in rows" :key="item.id" class="dr-card">
        <button class="dr-card-main" type="button" @click="withGlobalLoading(openResult, item.id)">
          <span class="dr-card-heading">
            <span class="dr-game-name">{{
              item.game.name
            }}</span>
            <span class="dr-period">{{ item.period.period_no }}</span>
          </span>
          <span class="dr-meta">
            <span
              >{{ ui("draw.scheduled") }}:
              {{
                formatDrawDateTime(item.period.draw_at, locale)
              }}</span
            >
            <span
              >{{ ui("draw.drawnAt") }}:
              {{
                formatDrawDateTime(item.drawn_at, locale)
              }}</span
            >
          </span>
          <span
            class="dr-period-status"
            :class="{
              'is-cancelled': isCancelledPeriodStatus(item.period.status),
            }"
            >{{ periodStatusLabel(item.period.status, locale) }}</span
          >
          <span class="dr-result-groups">
            <span
              v-for="group in resultNumberGroups(item.result, locale)"
              :key="group.key"
              class="dr-number-group"
            >
              <span class="dr-group-label">{{ group.label }}</span>
              <span v-if="group.key !== 'digits'" class="dr-balls">
                <span
                  v-for="(number, index) in group.positions"
                  :key="`${number}-${index}`"
                  class="dr-ball"
                  >{{ number }}</span
                >
              </span>
              <span v-else class="dr-digit-positions">
                <span
                  v-for="(number, index) in group.positions"
                  :key="index"
                  class="dr-position"
                >
                  <span class="dr-position-label">{{
                    ui("draw.position", { position: index + 1 })
                  }}</span>
                  <span class="dr-balls"
                    ><span class="dr-ball">{{ number }}</span></span
                  >
                </span>
              </span>
            </span>
          </span>
        </button>
      </article>
    </div>

    <div
      v-if="
        !loading &&
        !error &&
        !periodError &&
        activeTab === 'history' &&
        periods.length
      "
      class="dr-list"
    >
      <article
        v-for="entry in periods"
        :key="entry.period.id"
        class="dr-card dr-history-card"
      >
        <div class="dr-card-main">
          <span class="dr-card-heading">
            <span class="dr-game-name">{{
              selectedGame?.name
            }}</span>
            <span class="dr-period">{{ entry.period.period_no }}</span>
          </span>
          <span class="dr-meta">
            <span
              >{{ ui("draw.scheduled") }}:
              {{
                formatDrawDateTime(entry.period.draw_at, locale)
              }}</span
            >
            <span v-if="entry.draw"
              >{{ ui("draw.drawnAt") }}:
              {{
                formatDrawDateTime(entry.draw.drawn_at, locale)
              }}</span
            >
          </span>
          <span
            class="dr-period-status"
            :class="{
              'is-cancelled': isCancelledPeriodStatus(entry.period.status),
            }"
            >{{ periodStatusLabel(entry.period.status, locale) }}</span
          >
          <button
            v-if="entry.draw"
            class="dr-link-button"
            type="button"
            @click="withGlobalLoading(openResult, entry.draw.id)"
          >
            {{
              ui("draw.viewResultDetails")
            }} <SiteIcon name="next" /> </button>
        </div>
      </article>
    </div>

    <nav
      v-if="
        !compact && !error && !periodError && (rows.length || periods.length)
      "
      class="dr-pagination"
      :aria-label="ui('draw.pagination')"
    >
      <button
        type="button"
        class="dr-button"
        :disabled="!canPrevious"
        @click="withGlobalLoading(movePage, -1)"
      >
        {{ ui("draw.previous") }}
      </button>
      <span
        >{{ pageOffset + 1 }}–{{
          pageOffset + (activeTab === "history" ? periods.length : rows.length)
        }}</span
      >
      <button
        type="button"
        class="dr-button"
        :disabled="!canNext"
        @click="withGlobalLoading(movePage, 1)"
      >
        {{ ui("draw.next") }}
      </button>
    </nav>

    <RouterLink
      v-if="compact && isUuid(gameId)"
      class="dr-history-link"
      :to="`/results?game_id=${encodeURIComponent(gameId)}`"
      >{{ ui("draw.viewHistory") }} <SiteIcon name="next" /> </RouterLink
    >

    <DetailModal v-if="detailLoading || !!detailError || !!detail" :title="detailLabels.title" :close-label="detailLabels.close" @close="closeDetail()">
<div
      v-if="detailLoading || detailError || detail"
      class="dr-detail"
      aria-live="polite"
    >
      <div class="dr-detail-heading">
        <h3>{{ detailLabels.result }}</h3>
        <button type="button" class="dr-inline-button" @click="closeDetail">
          {{ ui("draw.close") }}
        </button>
      </div>
      <p v-global-loading v-if="detailLoading" class="dr-state">
        {{
          ui("draw.verifying")
        }}
      </p>
      <p v-else-if="detailError" class="dr-message dr-error" role="alert">
        {{ detailError }}
        <button
          class="dr-inline-button"
          type="button"
          @click="withGlobalLoading(openResult, selectedResultId)"
        >
          {{ ui("betting.retry") }}
        </button>
      </p>
      <template v-else-if="detail">
        <p class="dr-detail-period">
          {{ detail.game.name }} ·
          {{ detail.period.period_no }}
        </p>
        <p class="dr-detail-id">
          <span>{{ ui("draw.publicRecordId") }}</span
          ><code>{{ detail.id }}</code>
        </p>
        <p class="dr-meta">
          {{ ui("draw.scheduled") }}:
          {{
            formatDrawDateTime(detail.period.draw_at, locale)
          }}<br />{{ ui("draw.drawnAt") }}:
          {{
            formatDrawDateTime(detail.drawn_at, locale)
          }}
        </p>
        <div class="dr-result-groups">
          <span
            v-for="group in resultNumberGroups(detail.result, locale)"
            :key="group.key"
            class="dr-number-group"
          >
            <span class="dr-group-label">{{ group.label }}</span>
            <span v-if="group.key !== 'digits'" class="dr-balls"
              ><span
                v-for="(number, index) in group.positions"
                :key="`${number}-${index}`"
                class="dr-ball"
                >{{ number }}</span
              ></span
            >
            <span v-else class="dr-digit-positions"
              ><span
                v-for="(number, index) in group.positions"
                :key="index"
                class="dr-position"
                ><span class="dr-position-label">{{
                  ui("draw.position", { position: index + 1 })
                }}</span
                ><span class="dr-balls"
                  ><span class="dr-ball">{{ number }}</span></span
                ></span
              ></span
            >
          </span>
        </div>
        <p
          class="dr-period-status"
          :class="{
            'is-cancelled': isCancelledPeriodStatus(detail.period.status),
          }"
        >
          {{ periodStatusLabel(detail.period.status, locale) }}
        </p>
      </template>
    </div>
</DetailModal>

  </section>
</template>

<style scoped>
.draw-results {
  --dr-ink: var(--ink);
  --dr-muted: var(--muted);
  --dr-border: var(--line);
  --dr-accent: #059669;
  --dr-surface: var(--surface);
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  color: var(--dr-ink);
  overflow-wrap: anywhere;
}
.draw-results *,
.draw-results *::before,
.draw-results *::after {
  box-sizing: border-box;
}
.dr-header,
.dr-card-heading,
.dr-detail-heading,
.dr-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.dr-header {
  margin-bottom: 18px;
}
.dr-eyebrow {
  margin: 0 0 4px;
  color: var(--dr-accent);
  font-size: 11px;
  font-weight: 750;
  letter-spacing: 0.13em;
}
.dr-header h2,
.dr-detail h3 {
  margin: 0;
  font-size: clamp(1.15rem, 3vw, 1.5rem);
  line-height: 1.25;
}
.dr-button,
.dr-tabs button,
.dr-inline-button,
.dr-link-button {
  font: inherit;
}
.dr-button {
  min-height: 40px;
  padding: 8px 14px;
  border: 1px solid var(--dr-border);
  border-radius: 10px;
  background: var(--dr-surface);
  color: var(--dr-ink);
  cursor: pointer;
}
.dr-button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.dr-controls {
  display: grid;
  grid-template-columns: auto minmax(150px, 0.8fr) minmax(190px, 1fr);
  align-items: end;
  gap: 12px;
  margin-bottom: 16px;
}
.dr-tabs {
  display: flex;
  gap: 4px;
  padding: 4px;
  border-radius: 12px;
  background: #eef4f2;
}
.dr-tabs button {
  min-height: 38px;
  padding: 7px 12px;
  border: 0;
  border-radius: 9px;
  color: var(--dr-muted);
  background: transparent;
  cursor: pointer;
  white-space: nowrap;
}
.dr-tabs button.is-active {
  color: var(--surface);
  background: var(--dr-accent);
}
.dr-field {
  display: grid;
  gap: 5px;
  min-width: 0;
  color: var(--dr-muted);
  font-size: 0.78rem;
}
.dr-field select,
.dr-field input {
  width: 100%;
  min-width: 0;
  min-height: 42px;
  padding: 8px 10px;
  border: 1px solid var(--dr-border);
  border-radius: 9px;
  background: var(--surface);
  color: var(--dr-ink);
  font: inherit;
}
.dr-state,
.dr-message {
  margin: 14px 0;
  padding: 14px 16px;
  border: 1px solid var(--dr-border);
  border-radius: 12px;
  color: var(--dr-muted);
  background: #f8fbfa;
}
.dr-error {
  color: #8e342d;
  border-color: #efd0cb;
  background: #fff8f6;
}
.dr-paused {
  color: #705614;
  border-color: #eadba9;
  background: #fffaf0;
}
.dr-inline-button,
.dr-link-button {
  padding: 0;
  border: 0;
  color: var(--dr-accent);
  background: transparent;
  text-decoration: underline;
  text-underline-offset: 3px;
  cursor: pointer;
}
.dr-inline-button {
  margin-inline-start: 8px;
}
.dr-list {
  display: grid;
  gap: 10px;
}
.dr-card {
  min-width: 0;
  border: 1px solid var(--dr-border);
  border-radius: 14px;
  background: var(--dr-surface);
  box-shadow: 0 3px 12px rgb(24 43 42 / 4%);
}
.dr-card-main {
  display: grid;
  width: 100%;
  gap: 12px;
  padding: 16px;
  border: 0;
  border-radius: inherit;
  color: inherit;
  text-align: start;
  background: transparent;
  cursor: pointer;
}
.dr-history-card .dr-card-main {
  cursor: default;
}
.dr-game-name {
  min-width: 0;
  font-weight: 700;
}
.dr-period {
  min-width: 0;
  color: var(--dr-muted);
  font-variant-numeric: tabular-nums;
  text-align: end;
}
.dr-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 5px 18px;
  color: var(--dr-muted);
  font-size: 0.8rem;
  line-height: 1.55;
  font-variant-numeric: tabular-nums;
}
.dr-result-groups {
  display: grid;
  gap: 12px;
  min-width: 0;
}
.dr-number-group {
  display: grid;
  gap: 7px;
  min-width: 0;
}
.dr-group-label,
.dr-position-label {
  color: var(--dr-muted);
  font-size: 0.74rem;
  font-weight: 650;
}
.dr-balls {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.dr-ball {
  display: inline-grid;
  min-width: 36px;
  height: 36px;
  padding: 0 8px;
  place-items: center;
  border-radius: 50%;
  border: none;
  color: var(--surface);
  background: radial-gradient(circle at 35% 35%, #ecfdf5 0%, #10b981 60%, #059669 100%);
  font-size: 0.95rem;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  box-shadow: 0 4px 10px rgba(5, 150, 105, 0.35);
}
.dr-number-group:nth-child(2) .dr-ball {
  color: var(--surface);
  border: none;
  background: radial-gradient(circle at 35% 35%, #fffbeb 0%, #f59e0b 60%, #d97706 100%);
  box-shadow: 0 4px 10px rgba(245, 158, 11, 0.35);
}
.dr-digit-positions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.dr-position {
  display: grid;
  gap: 6px;
}
.dr-pagination {
  justify-content: center;
  margin: 16px 0;
  color: var(--dr-muted);
  font-size: 0.82rem;
  font-variant-numeric: tabular-nums;
}
.dr-history-link {
  display: inline-block;
  margin: 14px 0;
  color: var(--dr-accent);
  font-weight: 650;
  text-underline-offset: 3px;
}
.dr-detail {
  display: grid;
  gap: 12px;
  margin-top: 16px;
  padding: 16px;
  border: 1px solid #b9d9d2;
  border-radius: 14px;
  background: #f7fcfa;
}
.dr-detail-heading h3 {
  font-size: 1rem;
}
.dr-detail-period {
  margin: 0;
  font-weight: 700;
  overflow-wrap: anywhere;
}
.dr-detail-id {
  display: grid;
  gap: 4px;
  margin: 0;
  color: var(--dr-muted);
  font-size: 0.76rem;
}
.dr-detail-id code {
  color: var(--dr-ink);
  font:
    0.8rem/1.5 ui-monospace,
    SFMono-Regular,
    Menlo,
    monospace;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.dr-detail .dr-meta {
  display: block;
}
.dr-period-status {
  color: var(--dr-accent);
  font-size: 0.8rem;
  font-weight: 700;
}
.dr-period-status.is-cancelled {
  color: #8e342d;
}
.draw-results--compact .dr-header {
  margin-bottom: 12px;
}
@media (max-width: 680px) {
  .dr-controls {
    grid-template-columns: 1fr 1fr;
  }
  .dr-tabs {
    grid-column: 1 / -1;
    width: fit-content;
    max-width: 100%;
  }
  .dr-period-field {
    grid-column: 1 / -1;
  }
  .dr-card-main {
    padding: 13px;
  }
}
@media (max-width: 390px) {
  .dr-controls {
    grid-template-columns: 1fr;
  }
  .dr-tabs,
  .dr-period-field {
    grid-column: auto;
  }
  .dr-card-heading {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }
  .dr-period {
    text-align: start;
  }
  .dr-refresh {
    padding-inline: 10px;
  }
}
.dr-tabs button {
  min-height: 38px;
  padding: 7px 12px;
  border: 0;
  border-radius: 9px;
  color: var(--dr-muted);
  background: transparent;
  cursor: pointer;
  white-space: nowrap;
}
.dr-tabs button.is-active {
  color: var(--surface);
  background: var(--dr-accent);
}
.dr-field {
  display: grid;
  gap: 5px;
  min-width: 0;
  color: var(--dr-muted);
  font-size: 0.78rem;
}
.dr-field select,
.dr-field input {
  width: 100%;
  min-width: 0;
  min-height: 42px;
  padding: 8px 10px;
  border: 1px solid var(--dr-border);
  border-radius: 9px;
  background: var(--surface);
  color: var(--dr-ink);
  font: inherit;
}
.dr-state,
.dr-message {
  margin: 14px 0;
  padding: 14px 16px;
  border: 1px solid var(--dr-border);
  border-radius: 12px;
  color: var(--dr-muted);
  background: #f8fbfa;
}
.dr-error {
  color: #8e342d;
  border-color: #efd0cb;
  background: #fff8f6;
}
.dr-paused {
  color: #705614;
  border-color: #eadba9;
  background: #fffaf0;
}
.dr-inline-button,
.dr-link-button {
  padding: 0;
  border: 0;
  color: var(--dr-accent);
  background: transparent;
  text-decoration: underline;
  text-underline-offset: 3px;
  cursor: pointer;
}
.dr-inline-button {
  margin-inline-start: 8px;
}
.dr-list {
  display: grid;
  gap: 10px;
}
.dr-card {
  min-width: 0;
  border: 1px solid var(--dr-border);
  border-radius: 14px;
  background: var(--dr-surface);
  box-shadow: 0 3px 12px rgb(24 43 42 / 4%);
}
.dr-card-main {
  display: grid;
  width: 100%;
  gap: 12px;
  padding: 16px;
  border: 0;
  border-radius: inherit;
  color: inherit;
  text-align: start;
  background: transparent;
  cursor: pointer;
}
.dr-history-card .dr-card-main {
  cursor: default;
}
.dr-game-name {
  min-width: 0;
  font-weight: 700;
}
.dr-period {
  min-width: 0;
  color: var(--dr-muted);
  font-variant-numeric: tabular-nums;
  text-align: end;
}
.dr-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 5px 18px;
  color: var(--dr-muted);
  font-size: 0.8rem;
  line-height: 1.55;
  font-variant-numeric: tabular-nums;
}
.dr-result-groups {
  display: grid;
  gap: 12px;
  min-width: 0;
}
.dr-number-group {
  display: grid;
  gap: 7px;
  min-width: 0;
}
.dr-group-label,
.dr-position-label {
  color: var(--dr-muted);
  font-size: 0.74rem;
  font-weight: 650;
}
.dr-balls {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.dr-ball {
  display: inline-grid;
  min-width: 36px;
  height: 36px;
  padding: 0 8px;
  place-items: center;
  border-radius: 50%;
  border: none;
  color: var(--surface);
  background: radial-gradient(circle at 35% 35%, #ecfdf5 0%, #10b981 60%, #059669 100%);
  font-size: 0.95rem;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  box-shadow: 0 4px 10px rgba(5, 150, 105, 0.35);
}
.dr-number-group:nth-child(2) .dr-ball {
  color: var(--surface);
  border: none;
  background: radial-gradient(circle at 35% 35%, #fffbeb 0%, #f59e0b 60%, #d97706 100%);
  box-shadow: 0 4px 10px rgba(245, 158, 11, 0.35);
}
.dr-digit-positions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.dr-position {
  display: grid;
  gap: 6px;
}
.dr-pagination {
  justify-content: center;
  margin: 16px 0;
  color: var(--dr-muted);
  font-size: 0.82rem;
  font-variant-numeric: tabular-nums;
}
.dr-history-link {
  display: inline-block;
  margin: 14px 0;
  color: var(--dr-accent);
  font-weight: 650;
  text-underline-offset: 3px;
}
.dr-detail {
  display: grid;
  gap: 12px;
  margin-top: 16px;
  padding: 16px;
  border: 1px solid #b9d9d2;
  border-radius: 14px;
  background: #f7fcfa;
}
.dr-detail-heading h3 {
  font-size: 1rem;
}
.dr-detail-period {
  margin: 0;
  font-weight: 700;
  overflow-wrap: anywhere;
}
.dr-detail-id {
  display: grid;
  gap: 4px;
  margin: 0;
  color: var(--dr-muted);
  font-size: 0.76rem;
}
.dr-detail-id code {
  color: var(--dr-ink);
  font:
    0.8rem/1.5 ui-monospace,
    SFMono-Regular,
    Menlo,
    monospace;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.dr-detail .dr-meta {
  display: block;
}
.dr-period-status {
  color: var(--dr-accent);
  font-size: 0.8rem;
  font-weight: 700;
}
.dr-period-status.is-cancelled {
  color: #8e342d;
}
.draw-results--compact .dr-header {
  margin-bottom: 12px;
}
@media (max-width: 680px) {
  .dr-controls {
    grid-template-columns: 1fr 1fr;
  }
  .dr-tabs {
    grid-column: 1 / -1;
    width: fit-content;
    max-width: 100%;
  }
  .dr-period-field {
    grid-column: 1 / -1;
  }
  .dr-card-main {
    padding: 13px;
  }
}
@media (max-width: 390px) {
  .dr-controls {
    grid-template-columns: 1fr;
  }
  .dr-tabs,
  .dr-period-field {
    grid-column: auto;
  }
  .dr-card-heading {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }
  .dr-period {
    text-align: start;
  }
  .dr-refresh {
    padding-inline: 10px;
  }
}
@media (max-width: 620px) {
  .draw-results:not(.draw-results--compact) > .dr-header { display: none; }
}
</style>
