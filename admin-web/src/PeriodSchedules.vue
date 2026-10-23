<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import SiteIcon from "@lottery/shared/SiteIcon";
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import PlatformTemporalInput from "@lottery/shared/PlatformTemporalInput.vue";
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import type { LocalizedMessage } from "@lottery/shared";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useAdminI18n } from "./i18n";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  buildGeneratePeriodsBody,
  buildUpdateScheduleBody,
  createPeriodScheduleKeyTracker,
  createPeriodScheduleRequestGuard,
  createPeriodSchedulesApi,
  defaultScheduleSpec,
  periodSchedulePermissions,
  type BusyWindow,
  type Game,
  type Period,
  type ScheduleRecord,
  type ScheduleSpec,
} from "./period-schedules-api";

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const { t, message: localized, display } = useAdminI18n();
const api = createPeriodSchedulesApi();
const guard = createPeriodScheduleRequestGuard();
const keyFor = createPeriodScheduleKeyTracker();
const games = ref<Game[]>([]);
const gameId = ref("");
const schedule = ref<ScheduleRecord | null>(null);
const spec = ref<ScheduleSpec>(defaultScheduleSpec("UTC"));
const drawTimes = ref("19:00:00");
const pauseDates = ref("");
const holidayDates = ref("");
const loadingGames = ref(false);
const loadingSchedule = ref(false);
const loadingPeriods = ref(false);
const savingSchedule = ref(false);
const generating = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const scheduleReason = ref("");
const generation = ref({ from: "", to: "", reason: "" });
const periods = ref<Period[]>([]);
const pageOffset = ref(0);
const pageSize = 25;
const generatedTotals = ref<{ created: number; existing: number } | null>(null);
const catalogOffset = ref(0);
const catalogLimit = 25;
const catalogHasNext = ref(false);
let activeScheduleWrite: ReturnType<typeof guard.capture> | null = null;
let activeGenerateWrite: ReturnType<typeof guard.capture> | null = null;

const rights = computed(() =>
  periodSchedulePermissions(props.account, props.brandId),
);
const selectedGame = computed(
  () => games.value.find((item) => item.id === gameId.value) ?? null,
);
const canSeeAnything = computed(
  () => rights.value.scheduleView || rights.value.periodView || rights.value.periodGenerate || rights.value.scheduleWrite,
);

function permissionFingerprint() {
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
function scope(lane: string): string {
  const base: unknown[] = [permissionFingerprint()];
  if (lane === "games") base.push(catalogOffset.value);
  if (lane !== "games") base.push(gameId.value);
  if (lane === "schedule-write")
    base.push(spec.value, drawTimes.value, pauseDates.value, holidayDates.value, scheduleReason.value);
  if (lane === "generate-write") base.push(generation.value);
  return JSON.stringify(base);
}
function current(ticket: ReturnType<typeof guard.capture>, permitted: boolean) {
  return guard.isCurrent(ticket, scope(ticket.lane), permitted);
}
function showError(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401)
    emit("session-invalid");
  error.value = cause instanceof Error ? publicErrorMessage(cause) : localized('admin.copy.4fb9cc4e');
}
function checkBrand<T extends { brand_id: string }>(record: T): T {
  if (record.brand_id !== props.brandId)
    throw new Error(t('admin.copy.52d94929'));
  return record;
}
function resetEditor() {
  const timezone = selectedGame.value?.timezone ?? "UTC";
  schedule.value = null;
  spec.value = defaultScheduleSpec(timezone);
  drawTimes.value = spec.value.daily_draw_times.join(", ");
  pauseDates.value = "";
  holidayDates.value = "";
  scheduleReason.value = "";
  generation.value = { from: "", to: "", reason: "" };
  periods.value = [];
  generatedTotals.value = null;
  loadingSchedule.value = false;
  loadingPeriods.value = false;
  savingSchedule.value = false;
  generating.value = false;
  activeScheduleWrite = null;
  activeGenerateWrite = null;
  pageOffset.value = 0;
  guard.invalidate("schedule-write");
  guard.invalidate("generate-write");
}

async function loadGames(offset = 0, append = false) {
  guard.invalidate("games");
  const permitted = rights.value.gameCatalogView;
  if (!permitted || !props.brandId) {
    games.value = [];
    gameId.value = "";
    loadingGames.value = false;
    resetEditor();
    return;
  }
  catalogOffset.value = offset;
  const brandId = props.brandId;
  const ticket = guard.capture("games", scope("games"));
  loadingGames.value = true;
  error.value = "";
  try {
    const result = await api.getGames(brandId, catalogLimit, offset);
    if (!current(ticket, rights.value.gameCatalogView)) return;
    result.games.forEach(checkBrand);
    const merged = append
      ? [...games.value.filter((item) => !result.games.some((next) => next.id === item.id)), ...result.games]
      : result.games;
    games.value = merged;
    catalogHasNext.value = result.games.length === catalogLimit;
    if (!merged.some((item) => item.id === gameId.value))
      gameId.value = merged[0]?.id ?? "";
  } catch (cause) {
    if (current(ticket, rights.value.gameCatalogView)) {
      games.value = [];
      gameId.value = "";
      showError(cause);
    }
  } finally {
    if (current(ticket, rights.value.gameCatalogView)) loadingGames.value = false;
  }
}

async function loadSchedule() {
  const game = selectedGame.value;
  guard.invalidate("schedule");
  if (!game || !rights.value.scheduleView) {
    schedule.value = null;
    if (game && rights.value.scheduleWrite) resetEditor();
    loadingSchedule.value = false;
    return;
  }
  const brandId = props.brandId;
  const ticket = guard.capture("schedule", scope("schedule"));
  loadingSchedule.value = true;
  error.value = "";
  try {
    const result = await api.getSchedule(brandId, game.id);
    if (!current(ticket, rights.value.scheduleView)) return;
    if (result) {
      checkBrand(result);
      if (result.game_id !== game.id)
        throw new Error(t('admin.copy.f48cde32'));
      schedule.value = result;
      games.value = games.value.map((item) =>
        item.id === game.id ? { ...item, version: result.game_version } : item,
      );
      spec.value = structuredClone(result.spec);
    } else {
      schedule.value = null;
      spec.value = defaultScheduleSpec(game.timezone);
    }
    drawTimes.value = spec.value.daily_draw_times.join(", ");
    pauseDates.value = spec.value.pause_dates.join(", ");
    holidayDates.value = spec.value.holiday_dates.join(", ");
    scheduleReason.value = "";
  } catch (cause) {
    if (current(ticket, rights.value.scheduleView)) {
      schedule.value = null;
      showError(cause);
    }
  } finally {
    if (current(ticket, rights.value.scheduleView)) loadingSchedule.value = false;
  }
}

async function loadPeriods(offset = pageOffset.value) {
  const game = selectedGame.value;
  guard.invalidate("periods");
  if (!game || !rights.value.periodView) {
    periods.value = [];
    loadingPeriods.value = false;
    return;
  }
  const brandId = props.brandId;
  const ticket = guard.capture("periods", scope("periods"));
  loadingPeriods.value = true;
  error.value = "";
  try {
    const result = await api.getPeriods(brandId, game.id, pageSize, offset);
    if (!current(ticket, rights.value.periodView)) return;
    result.periods.forEach(checkBrand);
    if (result.periods.some((period) => period.game_id !== game.id))
      throw new Error(t('admin.copy.a3ab9cc6'));
    periods.value = result.periods;
    pageOffset.value = result.offset;
  } catch (cause) {
    if (current(ticket, rights.value.periodView)) {
      periods.value = [];
      showError(cause);
    }
  } finally {
    if (current(ticket, rights.value.periodView)) loadingPeriods.value = false;
  }
}

function splitCsv(value: string, label: string): string[] {
  const entries = value.split(",").map((item) => item.trim()).filter(Boolean);
  if (entries.some((item) => item.includes(" ")))
    throw new Error(`${label}${t('admin.copy.7d86b09f')}`);
  return entries;
}

function formSpec(): ScheduleSpec {
  return {
    ...spec.value,
    timezone: spec.value.timezone.trim(),
    daily_draw_times: spec.value.mode === "daily" ? splitCsv(drawTimes.value, "每日开奖时间") : [],
    interval_seconds: spec.value.mode === "daily" ? 0 : spec.value.interval_seconds,
    busy_windows: spec.value.mode === "daily" ? [] : spec.value.busy_windows.map((window) => ({ ...window })),
    pause_dates: splitCsv(pauseDates.value, "暂停日期"),
    holiday_dates: splitCsv(holidayDates.value, "节假日"),
  };
}

async function saveSchedule() {
  const game = selectedGame.value;
  if (!game || !rights.value.scheduleWrite || savingSchedule.value) return;
  const brandId = props.brandId;
  let body;
  try {
    body = buildUpdateScheduleBody(game.version, formSpec(), scheduleReason.value);
  } catch (cause) {
    showError(cause);
    return;
  }
  const ticket = guard.capture("schedule-write", scope("schedule-write"));
  activeScheduleWrite = ticket;
  const key = keyFor(brandId, "schedule.update", game.id, body);
  savingSchedule.value = true;
  error.value = "";
  notice.value = "";
  try {
    const result = await api.updateSchedule(brandId, game.id, body, key);
    if (!current(ticket, rights.value.scheduleWrite)) return;
    checkBrand(result);
    if (result.game_id !== game.id)
      throw new Error(t('admin.copy.6326514c'));
    activeScheduleWrite = null;
    savingSchedule.value = false;
    schedule.value = result;
    games.value = games.value.map((item) =>
      item.id === game.id ? { ...item, version: result.game_version } : item,
    );
    spec.value = structuredClone(result.spec);
    drawTimes.value = spec.value.daily_draw_times.join(", ");
    pauseDates.value = spec.value.pause_dates.join(", ");
    holidayDates.value = spec.value.holiday_dates.join(", ");
    scheduleReason.value = "";
    notice.value = localized('admin.versionDisplay.scheduleSaved');
  } catch (cause) {
    if (current(ticket, rights.value.scheduleWrite)) showError(cause);
  } finally {
    if (activeScheduleWrite === ticket) {
      activeScheduleWrite = null;
      savingSchedule.value = false;
    }
  }
}

async function generatePeriods() {
  const game = selectedGame.value;
  if (!game || !rights.value.periodGenerate || generating.value) return;
  const brandId = props.brandId;
  let body;
  try {
    body = buildGeneratePeriodsBody(
      generation.value.from,
      generation.value.to,
      game.timezone,
      generation.value.reason,
    );
  } catch (cause) {
    showError(cause);
    return;
  }
  const ticket = guard.capture("generate-write", scope("generate-write"));
  activeGenerateWrite = ticket;
  const key = keyFor(brandId, "period.generate", game.id, body);
  generating.value = true;
  error.value = "";
  notice.value = "";
  generatedTotals.value = null;
  try {
    const result = await api.generatePeriods(brandId, game.id, body, game.timezone, key);
    if (!current(ticket, rights.value.periodGenerate)) return;
    result.periods.forEach(checkBrand);
    if (result.periods.some((period) => period.game_id !== game.id))
      throw new Error(t('admin.copy.e8c93a63'));
    activeGenerateWrite = null;
    generating.value = false;
    generatedTotals.value = { created: result.created, existing: result.existing };
    notice.value = localized('admin.copy.5faf1bb0');
    generation.value.reason = "";
    pageOffset.value = 0;
    if (rights.value.periodView) await loadPeriods(0);
  } catch (cause) {
    if (current(ticket, rights.value.periodGenerate)) showError(cause);
  } finally {
    if (activeGenerateWrite === ticket) {
      activeGenerateWrite = null;
      generating.value = false;
    }
  }
}

function addBusyWindow() {
  spec.value.busy_windows.push({ start: "12:00:00", end: "13:00:00", interval_seconds: 3600 });
}
function statusLabel(status: string) {
  const labels: Record<string, string> = {
    pending: t('admin.copy.997b3d22'),
    betting: t('admin.copy.3a20b720'),
    closed: t('admin.copy.3198ad9b'),
    waiting_draw: t('admin.copy.aab1777a'),
    drawn: t('admin.copy.a16e8013'),
    settling: t('admin.copy.01f90196'),
    settled: t('admin.copy.037164c2'),
    bet_cancelled: t('admin.copy.a1834cc2'),
    judged_cancelled: t('admin.copy.46a60c2c'),
  };
  return labels[status] ?? status;
}
function displayTime(value: string){ return formatDisplayDateTime(value, displayLocale.value); }
async function movePage(direction: -1 | 1) {
  const next = Math.max(0, pageOffset.value + direction * pageSize);
  await loadPeriods(next);
}

watch(
  () => [props.brandId, permissionFingerprint()] as const,
  () => {
    guard.invalidate();
    games.value = [];
    gameId.value = "";
    catalogOffset.value = 0;
    catalogHasNext.value = false;
    resetEditor();
    error.value = "";
    notice.value = "";
    void loadGames();
  },
);
watch(gameId, () => {
  resetEditor();
  error.value = "";
  notice.value = "";
  if (rights.value.scheduleView) void loadSchedule();
  if (rights.value.periodView) void loadPeriods(0);
});
watch(
  () => scope("schedule-write"),
  () => {
    if (activeScheduleWrite) {
      guard.invalidate("schedule-write");
      activeScheduleWrite = null;
      savingSchedule.value = false;
    }
  },
  { flush: "sync" },
);
watch(
  () => scope("generate-write"),
  () => {
    if (activeGenerateWrite) {
      guard.invalidate("generate-write");
      activeGenerateWrite = null;
      generating.value = false;
    }
  },
  { flush: "sync" },
);
watch(
  () => spec.value.mode,
  (mode) => {
    if (mode === "daily") {
      spec.value.interval_seconds = 0;
      spec.value.busy_windows = [];
    } else {
      spec.value.daily_draw_times = [];
      if (spec.value.interval_seconds < 30) spec.value.interval_seconds = 3600;
    }
  },
);
onMounted(() => void loadGames());
onUnmounted(() => guard.invalidate());
</script>

<template>
  <section class="period-schedules">
    <header class="page-head">
      <div>
        <h2>{{ t('admin.copy.18d26d2c') }}</h2>
        <p>{{ t('admin.copy.1d7831b8') }}</p>
      </div>
      <span class="brand-tag">{{ t('admin.copy.9c32661d') }} · <AssociationName kind="brand" :id="brandId" :locale="associationLocale" /></span>
    </header>

    <p v-if="!canSeeAnything" class="callout">{{ t('admin.copy.33b6b172') }}</p>
    <template v-else>
      <div v-if="error" class="notice error" role="alert">
        {{ display(error) }}
        <button v-refresh-button="t('admin.copy.deca021e')" type="button" :disabled="loadingGames || loadingSchedule || loadingPeriods" @click="withGlobalLoading(loadGames)">{{ t('admin.copy.deca021e') }}</button>
      </div>
      <p v-if="notice" class="notice success" role="status">{{ display(notice) }}</p>
      <p v-if="!rights.gameCatalogView" class="callout">{{ t('admin.copy.5adcddb8') }}</p>
      <div v-global-loading v-else-if="loadingGames && !games.length" class="callout" role="status">{{ t('admin.copy.24fadb90') }}</div>
      <div v-if="rights.gameCatalogView && games.length" class="game-picker">
        <label for="period-game">{{ t('admin.copy.7492bbfb') }}</label>
        <select id="period-game" v-model="gameId">
          <option v-for="game in games" :key="game.id" :value="game.id">{{ game.name }}</option>
        </select>
        <span v-if="selectedGame" class="muted">{{ t('admin.copy.f257539c') }} {{ selectedGame.timezone }}</span>
        <span class="catalog-actions"><button type="button" class="button button-secondary" :disabled="loadingGames || catalogOffset === 0" @click="withGlobalLoading(loadGames, Math.max(0, catalogOffset - catalogLimit))">{{ t('admin.copy.a0844ef2') }}</button><span class="muted">{{ t('admin.copy.48392060') }} {{ catalogOffset }}</span><button type="button" class="button button-secondary" :disabled="loadingGames || !catalogHasNext" @click="withGlobalLoading(loadGames, catalogOffset + catalogLimit, true)">{{ t('admin.copy.239a81b3') }}</button><button v-refresh-button="t('admin.copy.4422e4ac')" type="button" class="button button-secondary" :disabled="loadingGames" @click="withGlobalLoading(loadGames, 0)">{{ t('admin.copy.4422e4ac') }}</button></span>
      </div>
      <p v-if="rights.gameCatalogView && !games.length && !loadingGames" class="callout empty-state">{{ t('admin.copy.81366f21') }}</p>

      <template v-if="selectedGame">
        <section v-if="rights.periodView" class="panel">
          <div class="section-head"><div><h3>{{ t('admin.copy.e0d09058') }}</h3></div><button v-refresh-button="loadingPeriods ? t('admin.copy.27342bf3') : t('admin.copy.894e2a7d')" type="button" class="button button-secondary" :disabled="loadingPeriods" @click="withGlobalLoading(loadPeriods, pageOffset)">{{ loadingPeriods ? t('admin.copy.27342bf3') : t('admin.copy.894e2a7d') }}</button></div>
          <div v-global-loading v-if="loadingPeriods && !periods.length" class="callout" role="status">{{ t('admin.copy.caaf8db2') }}</div>
          <div v-else-if="!periods.length" class="callout empty-state">{{ t('admin.copy.30f48614') }}</div>
          <div v-else class="period-list">
            <article v-for="period in periods" :key="period.id" class="period-card">
              <div class="period-title"><strong>{{ period.period_no }}</strong><span class="status-chip">{{ statusLabel(period.status) }}</span></div>
              <dl>
                <div><dt>{{ t('admin.copy.a04dd7be') }}</dt><dd>{{ period.sequence }}</dd></div>
                <div><dt>{{ t('admin.copy.0718d24b') }}</dt><dd>{{ displayTime(period.bet_start_at) }}</dd></div>
                <div><dt>{{ t('admin.copy.1155ce54') }}</dt><dd>{{ displayTime(period.bet_end_at) }}</dd></div>
                <div><dt>{{ t('admin.copy.f3f01ff2') }}</dt><dd>{{ displayTime(period.draw_at) }}</dd></div>
                <div><dt>{{ t('admin.copy.8034df47') }}</dt><dd>{{ period.schedule_id || "—" }}</dd></div>
                <div><dt>{{ t('admin.copy.8cf33349') }}</dt><dd>{{ period.state_reason || "—" }}</dd></div>
              </dl>
            </article>
          </div>
          <div class="pagination"><span>{{ t('admin.copy.4554742a') }} {{ periods.length }} {{ t('admin.copy.067fed38') }} · {{ t('admin.copy.4ffb01e2') }} {{ pageOffset }}</span><div><button type="button" class="button button-secondary" :disabled="pageOffset === 0 || loadingPeriods" @click="withGlobalLoading(movePage, -1)">{{ t('admin.copy.dbe7a5d7') }}</button><button type="button" class="button button-secondary" :disabled="periods.length < pageSize || loadingPeriods" @click="withGlobalLoading(movePage, 1)">{{ t('admin.copy.2e9c9e7a') }}</button></div></div>
        </section>
        <details v-if="rights.scheduleView || rights.scheduleWrite" class="console-disclosure">
          <summary>{{ t('admin.console.scheduleConfiguration') }}<SiteIcon name="down" /></summary>
        <section class="panel">
          <div class="section-head">
            <div>
              <h3>{{ t('admin.copy.96d2d5a8') }}</h3>
              <p v-if="schedule">{{ t('admin.copy.5f1f6b9f') }} {{ schedule.id }}</p>
              <p v-else-if="rights.scheduleView">{{ t('admin.copy.2fb1b321') }}</p>
              <p v-else>{{ t('admin.copy.8f8e9812') }}</p>
            </div>
            <button v-refresh-button="loadingSchedule ? t('admin.copy.27342bf3') : t('admin.copy.b7dc54dc')" type="button" class="button button-secondary" :disabled="loadingSchedule" @click="withGlobalLoading(loadSchedule)">{{ loadingSchedule ? t('admin.copy.27342bf3') : t('admin.copy.b7dc54dc') }}</button>
          </div>
          <div v-global-loading v-if="loadingSchedule && !schedule" class="callout">{{ t('admin.copy.92bf783b') }}</div>
          <div v-if="!rights.scheduleView" class="callout">{{ t('admin.copy.9375bcce') }}</div>
          <div class="form-grid">
            <label>{{ t('admin.copy.83daf4d6') }}
              <input v-model="spec.timezone" readonly aria-readonly="true">
            </label>
            <label>{{ t('admin.copy.05dbb122') }}
              <select v-model="spec.mode" :disabled="!rights.scheduleWrite || loadingSchedule">
                <option value="daily">{{ t('admin.copy.b8853696') }}</option>
                <option value="interval">{{ t('admin.copy.7495dc0c') }}</option>
              </select>
            </label>
            <label v-if="spec.mode === 'daily'" class="wide">{{ t('admin.copy.0d03eb72') }}
              <input v-model="drawTimes" :disabled="!rights.scheduleWrite || loadingSchedule" placeholder="12:00:00, 19:00:00">
            </label>
            <label v-else>{{ t('admin.copy.bdba3e64') }}
              <input v-model.number="spec.interval_seconds" type="number" min="30" max="86400" step="1" :disabled="!rights.scheduleWrite || loadingSchedule">
            </label>
            <label>{{ t('admin.copy.c1582779') }}
              <input v-model.number="spec.bet_open_before_seconds" type="number" min="1" max="86400" step="1" :disabled="!rights.scheduleWrite || loadingSchedule">
            </label>
            <label>{{ t('admin.copy.7df28047') }}
              <input v-model.number="spec.bet_close_before_seconds" type="number" min="0" :max="spec.bet_open_before_seconds - 1" step="1" :disabled="!rights.scheduleWrite || loadingSchedule">
            </label>
            <label class="wide">{{ t('admin.copy.6f87cdad') }}</label>
            <div class="weekday-list wide">
              <label v-for="day in [{n:0,l:t('admin.copy.5cb9ee44')},{n:1,l:t('admin.copy.741629ff')},{n:2,l:t('admin.copy.6a3487ef')},{n:3,l:t('admin.copy.d3866a1a')},{n:4,l:t('admin.copy.052a82ef')},{n:5,l:t('admin.copy.df2b59b2')},{n:6,l:t('admin.copy.aae507ce')}]" :key="day.n" class="check-label">
                <input v-model="spec.weekdays" type="checkbox" :value="day.n" :disabled="!rights.scheduleWrite || loadingSchedule">{{ day.l }}
              </label>
            </div>
            <label>{{ t('admin.copy.64331095') }}
              <input v-model="pauseDates" :disabled="!rights.scheduleWrite || loadingSchedule" placeholder="2026-12-25">
            </label>
            <label>{{ t('admin.copy.64546e21') }}
              <input v-model="holidayDates" :disabled="!rights.scheduleWrite || loadingSchedule" placeholder="2026-10-01">
            </label>
            <label>{{ t('admin.copy.4c6acdce') }}
              <select v-model="spec.holiday_policy" :disabled="!rights.scheduleWrite || loadingSchedule">
                <option value="skip">{{ t('admin.copy.e683b33a') }}</option>
                <option value="normal">{{ t('admin.copy.ba1b301d') }}</option>
              </select>
            </label>
          </div>
          <div v-if="spec.mode === 'interval'" class="subpanel">
            <div class="section-head"><div><h4>{{ t('admin.copy.fa048b16') }}</h4><p>{{ t('admin.copy.ffb2a0c6') }}</p></div><button v-if="rights.scheduleWrite" type="button" class="button button-secondary" :disabled="loadingSchedule || spec.busy_windows.length >= 128" @click="addBusyWindow">{{ t('admin.copy.c36778c0') }}</button></div>
            <p v-if="!spec.busy_windows.length" class="muted empty-state">{{ t('admin.copy.6a6c3d22') }}</p>
            <div v-for="(window, index) in spec.busy_windows" :key="index" class="busy-row">
              <label>{{ t('admin.copy.515ed3b8') }}<PlatformTemporalInput v-model="window.start" type="time" :disabled="!rights.scheduleWrite || loadingSchedule" :locale="displayLocale" /></label>
              <label>{{ t('admin.copy.1138be4a') }}<PlatformTemporalInput v-model="window.end" type="time" :disabled="!rights.scheduleWrite || loadingSchedule" :locale="displayLocale" /></label>
              <label>{{ t('admin.copy.2003e1d3') }}<input v-model.number="window.interval_seconds" type="number" min="1" step="1" :disabled="!rights.scheduleWrite || loadingSchedule"></label>
              <button v-if="rights.scheduleWrite" type="button" class="danger-link" :disabled="loadingSchedule" @click="spec.busy_windows.splice(index, 1)">{{ t('admin.copy.cd87c66a') }}</button>
            </div>
          </div>
          <div v-if="rights.scheduleWrite" class="save-row">

            <button v-write-guard="savingSchedule" type="button" :disabled="savingSchedule || loadingSchedule " @click="withGlobalLoading(saveSchedule)">{{ savingSchedule ? t('admin.copy.2462134b') : t('admin.copy.b33c1f08') }}</button>
          </div>
        </section>
        </details>

        <details v-if="rights.periodGenerate" class="console-disclosure">
          <summary>{{ t('admin.copy.22320ad6') }}<SiteIcon name="down" /></summary>
        <section class="panel">
          <div class="section-head"><div><h3>{{ t('admin.copy.22320ad6') }}</h3><p>{{ t('admin.copy.0df28e10') }} {{ selectedGame.timezone }}{{ t('admin.copy.5a1b6e12') }}</p></div></div>
          <div class="form-grid generate-grid">
            <label>{{ t('admin.copy.bcb53f32') }}<PlatformTemporalInput v-model="generation.from" type="datetime-local" :locale="displayLocale" /></label>
            <label>{{ t('admin.copy.69df62ec') }}<PlatformTemporalInput v-model="generation.to" type="datetime-local" :locale="displayLocale" /></label>

          </div>
          <div class="save-row"><button v-write-guard="generating" type="button" :disabled="generating  || !generation.from || !generation.to" @click="withGlobalLoading(generatePeriods)">{{ generating ? t('admin.copy.a3028803') : t('admin.copy.22320ad6') }}</button><span v-if="generatedTotals" class="muted">{{ t('admin.copy.501266cd') }} {{ generatedTotals.created }} · {{ t('admin.copy.31913043') }} {{ generatedTotals.existing }}</span></div>
        </section>
        </details>

      </template>
    </template>
  </section>
</template>

<style scoped>
.period-schedules { display: grid; gap: 1rem; min-width: 0; color: var(--ink); }
.page-head, .section-head, .save-row, .game-picker, .pagination, .period-title { display: flex; align-items: center; justify-content: space-between; gap: .8rem; }
.page-head, .section-head { flex-wrap: wrap; }
.page-head h2, .section-head h3, .subpanel h4, .page-head p, .section-head p, .subpanel p { margin: .2rem 0; }
.page-head h2 { font-size: 1.45rem; }
.eyebrow { color: var(--muted); font-size: .72rem; font-weight: 750; letter-spacing: .12em; }
.brand-tag, .status-chip { border-radius: 999px; background: var(--canvas); color: var(--ink-secondary); padding: .35rem .7rem; font-size: .82rem; white-space: nowrap; }
.server-note, .callout { padding: .8rem 1rem; background: var(--canvas); border-radius: .65rem; color: var(--ink-secondary); }
.callout { margin: 0; }
.notice { padding: .75rem 1rem; border-radius: .6rem; }
.notice button { margin-left: .75rem; }
.error { background: var(--danger-bg); color: var(--danger-text); }
.success { background: var(--success-bg); color: var(--success-text); }
.panel, .subpanel, .period-card { min-width: 0; border: 1px solid var(--line); border-radius: .8rem; background: var(--surface); padding: 1rem; }
.subpanel { margin-top: 1rem; background: var(--canvas); }
.game-picker { justify-content: flex-start; flex-wrap: wrap; }
.catalog-actions { display: flex; flex-wrap: wrap; align-items: center; gap: .45rem; }
.game-picker select { min-width: min(22rem, 100%); }
.muted, .section-head p { color: var(--muted); font-size: .9rem; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .85rem 1rem; margin-top: .9rem; }
label { display: grid; gap: .35rem; min-width: 0; font-size: .88rem; font-weight: 650; }
input, select { box-sizing: border-box; width: 100%; min-width: 0; border: 1px solid var(--line); border-radius: .45rem; padding: .6rem .7rem; background: var(--surface); color: inherit; font: inherit; }
input:disabled, select:disabled { background: var(--canvas); color: var(--muted); }
.wide { grid-column: 1 / -1; }
.weekday-list { display: flex; flex-wrap: wrap; gap: .55rem; }
.check-label { display: inline-flex; align-items: center; gap: .35rem; border: 1px solid var(--line); border-radius: .45rem; padding: .4rem .55rem; }
.check-label input { width: auto; }
.busy-row { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)) auto; align-items: end; gap: .7rem; padding: .65rem 0; border-top: 1px solid var(--line); }
.save-row { justify-content: flex-start; flex-wrap: wrap; margin-top: 1rem; }
.reason-field { flex: 1 1 18rem; }
button { border: 0; border-radius: .48rem; padding: .62rem .9rem; background: var(--primary); color: var(--surface); font: inherit; font-weight: 700; cursor: pointer; }
button.secondary { border: 1px solid var(--line); background: var(--surface); color: var(--ink-secondary); }
button:disabled { opacity: .55; cursor: not-allowed; }
.danger-link { background: transparent; color: var(--danger); padding: .5rem; }
.period-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 23rem), 1fr)); gap: .75rem; }
.period-card { padding: .85rem; }
.period-title { justify-content: flex-start; }
.period-title strong { overflow-wrap: anywhere; }
dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .6rem; margin: .8rem 0 0; }
dl div { min-width: 0; }
dt { color: #70828d; font-size: .76rem; }
dd { margin: .15rem 0 0; overflow-wrap: anywhere; font-size: .87rem; }
.pagination { margin-top: .9rem; flex-wrap: wrap; color: var(--muted); font-size: .88rem; }
.pagination div { display: flex; gap: .5rem; }
@media (max-width: 680px) {
  .form-grid { grid-template-columns: minmax(0, 1fr); }
  .wide { grid-column: auto; }
  .busy-row { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .busy-row button { justify-self: start; }
  .section-head > button { width: 100%; }
  dl { grid-template-columns: minmax(0, 1fr); }
}
</style>
