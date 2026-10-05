<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
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
const error = ref("");
const notice = ref("");
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
  error.value = cause instanceof Error ? cause.message : "请求失败，请重试。";
}
function checkBrand<T extends { brand_id: string }>(record: T): T {
  if (record.brand_id !== props.brandId)
    throw new Error("服务端返回记录的品牌与当前品牌不匹配，请重新读取。");
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
        throw new Error("排期响应的彩种与当前选择不匹配，请重新读取。");
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
      throw new Error("期数响应包含其他彩种记录，请重新读取。");
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
    throw new Error(`${label}请用逗号分隔，不要在单项中加入空格。`);
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
      throw new Error("保存响应的彩种与当前选择不匹配，请重新读取确认。");
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
    notice.value = `已保存为不可变排期修订版 ${result.revision}。`;
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
      throw new Error("生成响应包含其他彩种记录，请重新读取确认。");
    activeGenerateWrite = null;
    generating.value = false;
    generatedTotals.value = { created: result.created, existing: result.existing };
    notice.value = "生成请求已完成，统计来自服务端真实结果。";
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
    pending: "待开始",
    betting: "投注中",
    closed: "已截止",
    waiting_draw: "待开奖",
    drawn: "已开奖",
    settling: "结算中",
    settled: "已结算",
    bet_cancelled: "投注取消",
    judged_cancelled: "判定取消",
  };
  return labels[status] ?? status;
}
function displayTime(value: string) {
  const parsed = new Date(value);
  return Number.isFinite(parsed.valueOf()) ? parsed.toLocaleString() : value;
}
function movePage(direction: -1 | 1) {
  const next = Math.max(0, pageOffset.value + direction * pageSize);
  void loadPeriods(next);
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
        <p class="eyebrow">PERIOD SCHEDULES</p>
        <h2>开奖排期与期数</h2>
        <p>排期按修订版保存；已生成的期数不可在此编辑。</p>
      </div>
      <span class="brand-tag">品牌 · {{ brandId || "未选择" }}</span>
    </header>
    <p class="server-note">管理操作由服务端权限和版本校验保护。此页面不提供手动激活入口。</p>

    <p v-if="!canSeeAnything" class="callout">当前账号没有此品牌的排期或期数查看权限。</p>
    <template v-else>
      <div v-if="error" class="notice error" role="alert">
        {{ error }}
        <button type="button" :disabled="loadingGames || loadingSchedule || loadingPeriods" @click="loadGames()">重新读取</button>
      </div>
      <p v-if="notice" class="notice success" role="status">{{ notice }}</p>
      <p v-if="!rights.gameCatalogView" class="callout">当前账号缺少 game.view.brand / game.view.platform 彩种目录查看权限；排期和期数读写不会替代目录权限。</p>
      <div v-else-if="loadingGames && !games.length" class="callout" role="status">正在读取彩种…</div>
      <div v-if="rights.gameCatalogView && games.length" class="game-picker">
        <label for="period-game">彩种</label>
        <select id="period-game" v-model="gameId">
          <option v-for="game in games" :key="game.id" :value="game.id">{{ game.name }} · {{ game.code }}</option>
        </select>
        <span v-if="selectedGame" class="muted">时区 {{ selectedGame.timezone }} · 彩种版本 {{ selectedGame.version }}</span>
        <span class="catalog-actions"><button type="button" class="secondary" :disabled="loadingGames || catalogOffset === 0" @click="loadGames(Math.max(0, catalogOffset - catalogLimit))">上一页彩种</button><span class="muted">目录偏移 {{ catalogOffset }}</span><button type="button" class="secondary" :disabled="loadingGames || !catalogHasNext" @click="loadGames(catalogOffset + catalogLimit, true)">下一页彩种</button><button type="button" class="secondary" :disabled="loadingGames" @click="loadGames(0)">刷新彩种</button></span>
      </div>
      <p v-if="rights.gameCatalogView && !games.length && !loadingGames" class="callout">此目录页没有彩种记录。可以使用刷新或翻页重新读取。</p>

      <template v-if="selectedGame">
        <section v-if="rights.scheduleView || rights.scheduleWrite" class="panel">
          <div class="section-head">
            <div>
              <h3>排期配置</h3>
              <p v-if="schedule">当前修订版 {{ schedule.revision }} · 记录 {{ schedule.id }} · 彩种版本证据 {{ schedule.game_version }}</p>
              <p v-else-if="rights.scheduleView">尚无已保存排期；以下为基于彩种时区的初始草稿。</p>
              <p v-else>当前账号没有读取已保存排期的权限；下方表单不能预填现有配置。</p>
            </div>
            <button type="button" class="secondary" :disabled="loadingSchedule" @click="loadSchedule">{{ loadingSchedule ? "读取中…" : "重新读取排期" }}</button>
          </div>
          <div v-if="loadingSchedule && !schedule" class="callout">正在读取排期…</div>
          <div v-if="!rights.scheduleView" class="callout">当前账号可以保存排期，但不能读取现有配置；提交会创建一个新的不可变修订版。请确认下方全部字段。</div>
          <div class="form-grid">
            <label>彩种时区（IANA）
              <input v-model="spec.timezone" readonly aria-readonly="true">
            </label>
            <label>排期模式
              <select v-model="spec.mode" :disabled="!rights.scheduleWrite || loadingSchedule">
                <option value="daily">每日固定时间</option>
                <option value="interval">固定间隔</option>
              </select>
            </label>
            <label v-if="spec.mode === 'daily'" class="wide">每日开奖时间（HH:MM:SS，以逗号分隔）
              <input v-model="drawTimes" :disabled="!rights.scheduleWrite || loadingSchedule" placeholder="12:00:00, 19:00:00">
            </label>
            <label v-else>基准间隔（秒）
              <input v-model.number="spec.interval_seconds" type="number" min="30" max="86400" step="1" :disabled="!rights.scheduleWrite || loadingSchedule">
            </label>
            <label>投注开放提前秒数
              <input v-model.number="spec.bet_open_before_seconds" type="number" min="1" max="86400" step="1" :disabled="!rights.scheduleWrite || loadingSchedule">
            </label>
            <label>投注截止提前秒数
              <input v-model.number="spec.bet_close_before_seconds" type="number" min="0" :max="spec.bet_open_before_seconds - 1" step="1" :disabled="!rights.scheduleWrite || loadingSchedule">
            </label>
            <label class="wide">开奖星期（0 周日至 6 周六）</label>
            <div class="weekday-list wide">
              <label v-for="day in [{n:0,l:'周日'},{n:1,l:'周一'},{n:2,l:'周二'},{n:3,l:'周三'},{n:4,l:'周四'},{n:5,l:'周五'},{n:6,l:'周六'}]" :key="day.n" class="check-label">
                <input v-model="spec.weekdays" type="checkbox" :value="day.n" :disabled="!rights.scheduleWrite || loadingSchedule">{{ day.l }}
              </label>
            </div>
            <label>暂停日期（YYYY-MM-DD，逗号分隔）
              <input v-model="pauseDates" :disabled="!rights.scheduleWrite || loadingSchedule" placeholder="2026-12-25">
            </label>
            <label>节假日日期（YYYY-MM-DD，逗号分隔）
              <input v-model="holidayDates" :disabled="!rights.scheduleWrite || loadingSchedule" placeholder="2026-10-01">
            </label>
            <label>节假日策略
              <select v-model="spec.holiday_policy" :disabled="!rights.scheduleWrite || loadingSchedule">
                <option value="skip">跳过开奖</option>
                <option value="normal">照常开奖</option>
              </select>
            </label>
          </div>
          <div v-if="spec.mode === 'interval'" class="subpanel">
            <div class="section-head"><div><h4>繁忙时段</h4><p>各时段覆盖基准间隔；开始须早于结束，时段不能重叠，最多 128 行。</p></div><button v-if="rights.scheduleWrite" type="button" class="secondary" :disabled="loadingSchedule || spec.busy_windows.length >= 128" @click="addBusyWindow">添加时段</button></div>
            <p v-if="!spec.busy_windows.length" class="muted">未配置繁忙时段。</p>
            <div v-for="(window, index) in spec.busy_windows" :key="index" class="busy-row">
              <label>开始<input v-model="window.start" type="time" step="1" :disabled="!rights.scheduleWrite || loadingSchedule"></label>
              <label>结束<input v-model="window.end" type="time" step="1" :disabled="!rights.scheduleWrite || loadingSchedule"></label>
              <label>间隔（秒）<input v-model.number="window.interval_seconds" type="number" min="1" step="1" :disabled="!rights.scheduleWrite || loadingSchedule"></label>
              <button v-if="rights.scheduleWrite" type="button" class="danger-link" :disabled="loadingSchedule" @click="spec.busy_windows.splice(index, 1)">移除</button>
            </div>
          </div>
          <div v-if="rights.scheduleWrite" class="save-row">
            <label class="reason-field">操作原因（必填）<input v-model="scheduleReason" :disabled="savingSchedule" maxlength="500" placeholder="说明本次排期调整"></label>
            <button type="button" :disabled="savingSchedule || loadingSchedule || !scheduleReason.trim()" @click="saveSchedule">{{ savingSchedule ? "保存中…" : "保存新修订版" }}</button>
          </div>
        </section>

        <section v-if="rights.periodGenerate" class="panel">
          <div class="section-head"><div><h3>生成期数</h3><p>输入按 {{ selectedGame.timezone }} 解读；提交时转换为 UTC。范围最多 7 天，已存在期数由服务端统计。</p></div></div>
          <div class="form-grid generate-grid">
            <label>开始（彩种本地时间）<input v-model="generation.from" type="datetime-local" step="1"></label>
            <label>结束（彩种本地时间）<input v-model="generation.to" type="datetime-local" step="1"></label>
            <label class="wide">操作原因（必填）<input v-model="generation.reason" maxlength="500" placeholder="说明本次期数生成"></label>
          </div>
          <div class="save-row"><button type="button" :disabled="generating || !generation.reason.trim() || !generation.from || !generation.to" @click="generatePeriods">{{ generating ? "生成中…" : "生成期数" }}</button><span v-if="generatedTotals" class="muted">服务端结果：新建 {{ generatedTotals.created }} · 已存在 {{ generatedTotals.existing }}</span></div>
        </section>

        <section v-if="rights.periodView" class="panel">
          <div class="section-head"><div><h3>期数记录</h3><p>只读分页历史；以下列表是独立读取的记录页，不代表单次生成响应中的全部记录。</p></div><button type="button" class="secondary" :disabled="loadingPeriods" @click="loadPeriods(pageOffset)">{{ loadingPeriods ? "读取中…" : "刷新" }}</button></div>
          <div v-if="loadingPeriods && !periods.length" class="callout" role="status">正在读取期数…</div>
          <div v-else-if="!periods.length" class="callout">此页没有期数记录。</div>
          <div v-else class="period-list">
            <article v-for="period in periods" :key="period.id" class="period-card">
              <div class="period-title"><strong>{{ period.period_no }}</strong><span class="status-chip">{{ statusLabel(period.status) }}</span></div>
              <dl>
                <div><dt>序号 / 版本</dt><dd>{{ period.sequence }} / {{ period.version }}</dd></div>
                <div><dt>投注开始</dt><dd>{{ displayTime(period.bet_start_at) }}</dd></div>
                <div><dt>投注截止</dt><dd>{{ displayTime(period.bet_end_at) }}</dd></div>
                <div><dt>开奖时间</dt><dd>{{ displayTime(period.draw_at) }}</dd></div>
                <div><dt>排期记录</dt><dd>{{ period.schedule_id || "—" }}</dd></div>
                <div><dt>状态原因</dt><dd>{{ period.state_reason || "—" }}</dd></div>
              </dl>
            </article>
          </div>
          <div class="pagination"><span>显示 {{ periods.length }} 条 · 偏移 {{ pageOffset }}</span><div><button type="button" class="secondary" :disabled="pageOffset === 0 || loadingPeriods" @click="movePage(-1)">上一页</button><button type="button" class="secondary" :disabled="periods.length < pageSize || loadingPeriods" @click="movePage(1)">下一页</button></div></div>
        </section>
      </template>
    </template>
  </section>
</template>

<style scoped>
.period-schedules { display: grid; gap: 1rem; min-width: 0; color: #172b3a; }
.page-head, .section-head, .save-row, .game-picker, .pagination, .period-title { display: flex; align-items: center; justify-content: space-between; gap: .8rem; }
.page-head, .section-head { flex-wrap: wrap; }
.page-head h2, .section-head h3, .subpanel h4, .page-head p, .section-head p, .subpanel p { margin: .2rem 0; }
.page-head h2 { font-size: 1.45rem; }
.eyebrow { color: #597486; font-size: .72rem; font-weight: 750; letter-spacing: .12em; }
.brand-tag, .status-chip { border-radius: 999px; background: #e8f0f5; color: #315469; padding: .35rem .7rem; font-size: .82rem; white-space: nowrap; }
.server-note, .callout { padding: .8rem 1rem; background: #f0f5f8; border-radius: .65rem; color: #496274; }
.callout { margin: 0; }
.notice { padding: .75rem 1rem; border-radius: .6rem; }
.notice button { margin-left: .75rem; }
.error { background: #fff0ed; color: #8b2f22; }
.success { background: #eaf8ef; color: #25633e; }
.panel, .subpanel, .period-card { min-width: 0; border: 1px solid #d9e3e9; border-radius: .8rem; background: #fff; padding: 1rem; }
.subpanel { margin-top: 1rem; background: #f8fafb; }
.game-picker { justify-content: flex-start; flex-wrap: wrap; }
.catalog-actions { display: flex; flex-wrap: wrap; align-items: center; gap: .45rem; }
.game-picker select { min-width: min(22rem, 100%); }
.muted, .section-head p { color: #647987; font-size: .9rem; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .85rem 1rem; margin-top: .9rem; }
label { display: grid; gap: .35rem; min-width: 0; font-size: .88rem; font-weight: 650; }
input, select { box-sizing: border-box; width: 100%; min-width: 0; border: 1px solid #bacad4; border-radius: .45rem; padding: .6rem .7rem; background: #fff; color: inherit; font: inherit; }
input:disabled, select:disabled { background: #f2f5f7; color: #758692; }
.wide { grid-column: 1 / -1; }
.weekday-list { display: flex; flex-wrap: wrap; gap: .55rem; }
.check-label { display: inline-flex; align-items: center; gap: .35rem; border: 1px solid #d6e0e6; border-radius: .45rem; padding: .4rem .55rem; }
.check-label input { width: auto; }
.busy-row { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)) auto; align-items: end; gap: .7rem; padding: .65rem 0; border-top: 1px solid #e2e9ed; }
.save-row { justify-content: flex-start; flex-wrap: wrap; margin-top: 1rem; }
.reason-field { flex: 1 1 18rem; }
button { border: 0; border-radius: .48rem; padding: .62rem .9rem; background: #176b86; color: #fff; font: inherit; font-weight: 700; cursor: pointer; }
button.secondary { border: 1px solid #bdcbd3; background: #fff; color: #315469; }
button:disabled { opacity: .55; cursor: not-allowed; }
.danger-link { background: transparent; color: #a2352b; padding: .5rem; }
.period-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 23rem), 1fr)); gap: .75rem; }
.period-card { padding: .85rem; }
.period-title { justify-content: flex-start; }
.period-title strong { overflow-wrap: anywhere; }
dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .6rem; margin: .8rem 0 0; }
dl div { min-width: 0; }
dt { color: #70828d; font-size: .76rem; }
dd { margin: .15rem 0 0; overflow-wrap: anywhere; font-size: .87rem; }
.pagination { margin-top: .9rem; flex-wrap: wrap; color: #647987; font-size: .88rem; }
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
