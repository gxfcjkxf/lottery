<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createRuleVersionsApi, type GameRecord } from "./rule-versions-api";
import { useAdminI18n } from "./i18n";
import {
  createBetManagementApi,
  createBetMutationKeyTracker,
  isPositivePoints,
  betManagementPermissions,
  type BrandBetPolicy,
  type GameBetPolicy,
  type BrandBetPolicyConfig,
  type GameBetPolicyConfig,
  type LimitOverride,
} from "./bet-management-api";

type Limit = string | null;
type Override = LimitOverride;
type BrandConfig = BrandBetPolicyConfig;
type GameConfig = GameBetPolicyConfig;
type BrandPolicy = BrandBetPolicy;
type GamePolicy = GameBetPolicy;
type DraftBrand = {
  min_bet_points: string;
  max_bet_points: string;
  max_period_points: string;
  max_user_period_points: string;
  user_cancel_allowed: boolean;
  reason: string;
};
type DraftGame = {
  min_bet_points: OverrideDraft;
  max_bet_points: OverrideDraft;
  max_period_points: OverrideDraft;
  max_user_period_points: OverrideDraft;
  user_cancel_allowed: "inherit" | "allow" | "deny";
  reason: string;
};
type OverrideDraft = { mode: Override["mode"]; points: string };
type Lane = "brand" | "game";
type FrozenMutation = {
  lane: Lane;
  brandId: string;
  gameId: string;
  body: { version: number; config: BrandConfig | GameConfig; reason: string };
  key: string;
};

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const { t } = useAdminI18n();
const ui = (zh: string, en: string = zh) => t(zh, en);
const labels: Record<string, string> = {
  "投注策略": "Betting policy", "品牌与彩种投注限额": "Brand and game betting limits",
  "最低单注积分": "Minimum points per bet", "单注上限积分": "Maximum points per bet",
  "单期上限积分": "Maximum points per period", "单用户单期上限积分": "Maximum points per user per period",
};
const textEn: Record<string, string> = {
  "品牌策略响应与当前品牌不匹配。": "The brand-policy response does not match the current brand.", "读取品牌投注策略失败。": "Failed to load brand betting policy.",
  "彩种策略响应与当前品牌或彩种不匹配。": "The game-policy response does not match the current brand or game.", "读取彩种投注策略失败。": "Failed to load game betting policy.",
  "读取彩种目录失败。": "Failed to load the game catalog.", "保存响应与当前策略不匹配，请重新读取确认结果。": "The save response does not match the current policy. Reload to confirm the result.",
  "投注策略已保存并生效。": "Betting policy saved and applied.",
  "策略版本已变化。已尝试读取最新版本；请核对后再次提交，当前未保存的输入已保留。": "The policy version changed. The latest version was reloaded; review it before submitting again. Your unsaved input has been preserved.",
  "保存结果尚未确认。请按原请求重试，或刷新读取以核对版本；重试会复用原请求内容和幂等键。": "The save result is unconfirmed. Retry the original request or reload to check the version; a retry reuses the original request body and idempotency key.",
  "保存投注策略失败。": "Failed to save betting policy.", "正在读取最新版本；表单输入会保留。": "Loading the latest version; form input will be preserved.",
};
const display = (value: string) => ui(value, textEn[value] ?? labels[value] ?? value);
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createBetManagementApi();
const ruleBook = createRuleVersionsApi();
const keyForMutation = createBetMutationKeyTracker();
const rights = computed(() =>
  betManagementPermissions(props.account, props.brandId),
);
const brandPolicy = ref<BrandPolicy | null>(null);
const gamePolicy = ref<GamePolicy | null>(null);
const games = ref<GameRecord[]>([]);
const gameId = ref("");
const directGameId = ref("");
const brandDraft = ref<DraftBrand>(emptyBrandDraft());
const gameDraft = ref<DraftGame>(emptyGameDraft());
const loadingBrand = ref(false);
const loadingGame = ref(false);
const loadingGames = ref(false);
const saving = ref<Lane | "">("");
const reconciling = ref(false);
const error = ref("");
const notice = ref("");
const uncertain = ref<FrozenMutation | null>(null);
let generation = 0;
let brandRead = 0;
let gameRead = 0;
let gamesRead = 0;
let live = true;

function emptyBrandDraft(): DraftBrand {
  return {
    min_bet_points: "1",
    max_bet_points: "",
    max_period_points: "",
    max_user_period_points: "",
    user_cancel_allowed: false,
    reason: "",
  };
}
function inheritOverride(): OverrideDraft {
  return { mode: "inherit", points: "" };
}
function emptyGameDraft(): DraftGame {
  return {
    min_bet_points: inheritOverride(),
    max_bet_points: inheritOverride(),
    max_period_points: inheritOverride(),
    max_user_period_points: inheritOverride(),
    user_cancel_allowed: "inherit",
    reason: "",
  };
}
function cloneOverride(item: Override): OverrideDraft {
  return { mode: item.mode, points: item.points ?? "" };
}
function applyBrandDraft(item: BrandPolicy) {
  brandDraft.value = {
    min_bet_points: item.config.min_bet_points,
    max_bet_points: item.config.max_bet_points ?? "",
    max_period_points: item.config.max_period_points ?? "",
    max_user_period_points: item.config.max_user_period_points ?? "",
    user_cancel_allowed: item.config.user_cancel_allowed,
    reason: "",
  };
}
function applyGameDraft(item: GamePolicy) {
  gameDraft.value = {
    min_bet_points: cloneOverride(item.config.min_bet_points),
    max_bet_points: cloneOverride(item.config.max_bet_points),
    max_period_points: cloneOverride(item.config.max_period_points),
    max_user_period_points: cloneOverride(item.config.max_user_period_points),
    user_cancel_allowed:
      item.config.user_cancel_allowed === null
        ? "inherit"
        : item.config.user_cancel_allowed
          ? "allow"
          : "deny",
    reason: "",
  };
}
function current(ticket: number, brandId: string, actor: string) {
  return (
    live &&
    ticket === generation &&
    props.brandId === brandId &&
    props.account.id === actor &&
    rights.value.policyView
  );
}
function sameContext(ticket: number, brandId: string, actor: string) {
  return (
    live &&
    ticket === generation &&
    props.brandId === brandId &&
    props.account.id === actor
  );
}
function sessionExpired(cause: unknown) {
  if (cause instanceof AdminApiError && cause.status === 401) {
    generation++;
    clearVisibleData();
    emit("session-invalid");
    return true;
  }
  return false;
}
function explain(cause: unknown, fallback: string) {
  if (cause instanceof Error && cause.message) return cause.message;
  return fallback;
}
function clearVisibleData() {
  brandPolicy.value = null;
  gamePolicy.value = null;
  games.value = [];
  gameId.value = "";
  directGameId.value = "";
  brandDraft.value = emptyBrandDraft();
  gameDraft.value = emptyGameDraft();
  uncertain.value = null;
  loadingBrand.value = false;
  loadingGame.value = false;
  loadingGames.value = false;
  saving.value = "";
  reconciling.value = false;
  notice.value = "";
}

const brandInputsValid = computed(() => {
  const minValue = brandDraft.value.min_bet_points;
  if (!isPositivePoints(minValue)) return false;
  const caps = [
    brandDraft.value.max_bet_points,
    brandDraft.value.max_period_points,
    brandDraft.value.max_user_period_points,
  ];
  if (!caps.every((value) => !value || isPositivePoints(value))) return false;
  const min = BigInt(minValue);
  return caps.every((value) => !value || min <= BigInt(value));
});
function draftOverrideValid(value: OverrideDraft, minimum: boolean) {
  if (value.mode === "inherit") return true;
  if (value.mode === "unlimited") return !minimum;
  return isPositivePoints(value.points);
}
const gameInputsValid = computed(() => {
  const d = gameDraft.value;
  const defaults = brandPolicy.value?.config;
  if (!defaults || !isPositivePoints(defaults.min_bet_points)) return false;
  if (
    !draftOverrideValid(d.min_bet_points, true) ||
    !draftOverrideValid(d.max_bet_points, false) ||
    !draftOverrideValid(d.max_period_points, false) ||
    !draftOverrideValid(d.max_user_period_points, false)
  )
    return false;
  const min =
    d.min_bet_points.mode === "value"
      ? BigInt(d.min_bet_points.points)
      : BigInt(defaults.min_bet_points);
  const capValues: Array<string | null> = [
    resolveDraftCap(d.max_bet_points, defaults.max_bet_points),
    resolveDraftCap(d.max_period_points, defaults.max_period_points),
    resolveDraftCap(d.max_user_period_points, defaults.max_user_period_points),
  ];
  return capValues.every(
    (cap) => cap === null || (isPositivePoints(cap) && min <= BigInt(cap)),
  );
});
function resolveDraftCap(
  value: OverrideDraft,
  brandValue: string | null,
): string | null {
  if (value.mode === "unlimited") return null;
  if (value.mode === "value") return value.points;
  return brandValue || null;
}
const reasonValid = (value: string) =>
  Boolean(value.trim()) && new TextEncoder().encode(value.trim()).length <= 500;
const brandCanSave = computed(
  () =>
    canWrite.value &&
    brandPolicy.value !== null &&
    brandInputsValid.value &&
    reasonValid(brandDraft.value.reason) &&
    !saving.value &&
    !uncertain.value &&
    !reconciling.value &&
    !loadingBrand.value,
);
const gameCanSave = computed(
  () =>
    canWrite.value &&
    gamePolicy.value !== null &&
    gameInputsValid.value &&
    reasonValid(gameDraft.value.reason) &&
    !saving.value &&
    !uncertain.value &&
    !reconciling.value &&
    !loadingGame.value,
);
const canWrite = computed(
  () => rights.value.policyWrite && !props.account.super_admin,
);

async function loadBrand(preserveDraft = false) {
  if (!rights.value.policyView || !props.brandId) return;
  const brandId = props.brandId;
  const actor = props.account.id;
  const ticket = generation;
  const read = ++brandRead;
  loadingBrand.value = true;
  error.value = "";
  try {
    const item = await api.getBrandPolicy(brandId);
    if (!current(ticket, brandId, actor) || read !== brandRead) return;
    if (item.brand_id !== brandId)
      throw new Error("品牌策略响应与当前品牌不匹配。");
    brandPolicy.value = item;
    if (!uncertain.value && !preserveDraft) applyBrandDraft(item);
  } catch (cause) {
    if (!current(ticket, brandId, actor) || read !== brandRead) return;
    brandPolicy.value = null;
    sessionExpired(cause);
    error.value = explain(cause, "读取品牌投注策略失败。");
  } finally {
    if (current(ticket, brandId, actor) && read === brandRead)
      loadingBrand.value = false;
  }
}
async function loadGame(id = gameId.value, preserveDraft = false) {
  const normalizedId = id.trim();
  if (!rights.value.policyView || !normalizedId) return;
  gameId.value = normalizedId;
  const brandId = props.brandId;
  const actor = props.account.id;
  const ticket = generation;
  const read = ++gameRead;
  loadingGame.value = true;
  gamePolicy.value = null;
  error.value = "";
  try {
    const item = await api.getGamePolicy(brandId, normalizedId);
    if (
      !current(ticket, brandId, actor) ||
      read !== gameRead ||
      gameId.value !== normalizedId
    )
      return;
    if (item.brand_id !== brandId || item.game_id !== normalizedId)
      throw new Error("彩种策略响应与当前品牌或彩种不匹配。");
    gamePolicy.value = item;
    if (!preserveDraft) applyGameDraft(item);
  } catch (cause) {
    if (
      !current(ticket, brandId, actor) ||
      read !== gameRead ||
      gameId.value !== normalizedId
    )
      return;
    gamePolicy.value = null;
    sessionExpired(cause);
    error.value = explain(cause, "读取彩种投注策略失败。");
  } finally {
    if (
      current(ticket, brandId, actor) &&
      read === gameRead &&
      gameId.value === normalizedId
    )
      loadingGame.value = false;
  }
}
async function loadGames() {
  if (!rights.value.gameView || !props.brandId) return;
  const brandId = props.brandId;
  const actor = props.account.id;
  const ticket = generation;
  const read = ++gamesRead;
  loadingGames.value = true;
  try {
    const result = await ruleBook.getGames(brandId);
    if (
      !sameContext(ticket, brandId, actor) ||
      read !== gamesRead ||
      !rights.value.gameView
    )
      return;
    games.value = result.games.filter((game) => game.brand_id === brandId);
  } catch (cause) {
    if (
      !sameContext(ticket, brandId, actor) ||
      read !== gamesRead ||
      !rights.value.gameView
    )
      return;
    games.value = [];
    sessionExpired(cause);
    error.value = explain(cause, "读取彩种目录失败。");
  } finally {
    if (sameContext(ticket, brandId, actor) && read === gamesRead)
      loadingGames.value = false;
  }
}

function buildBrandConfig(): BrandConfig {
  return {
    min_bet_points: brandDraft.value.min_bet_points,
    max_bet_points: brandDraft.value.max_bet_points || null,
    max_period_points: brandDraft.value.max_period_points || null,
    max_user_period_points: brandDraft.value.max_user_period_points || null,
    user_cancel_allowed: brandDraft.value.user_cancel_allowed,
  };
}
function buildOverride(value: OverrideDraft): Override {
  return {
    mode: value.mode,
    points: value.mode === "value" ? value.points : null,
  };
}
function buildGameConfig(): GameConfig {
  return {
    min_bet_points: buildOverride(gameDraft.value.min_bet_points),
    max_bet_points: buildOverride(gameDraft.value.max_bet_points),
    max_period_points: buildOverride(gameDraft.value.max_period_points),
    max_user_period_points: buildOverride(
      gameDraft.value.max_user_period_points,
    ),
    user_cancel_allowed:
      gameDraft.value.user_cancel_allowed === "inherit"
        ? null
        : gameDraft.value.user_cancel_allowed === "allow",
  };
}
function retryUncertain() {
  if (!uncertain.value || saving.value) return;
  void submitMutation(uncertain.value);
}
async function saveBrand() {
  if (!brandCanSave.value || !brandPolicy.value) return;
  const body = {
    version: brandPolicy.value.version,
    config: buildBrandConfig(),
    reason: brandDraft.value.reason.trim(),
  };
  const key = keyForMutation({
    brandId: props.brandId,
    entityId: "",
    operation: "brand",
    body,
  });
  const mutation: FrozenMutation = {
    lane: "brand",
    brandId: props.brandId,
    gameId: "",
    body,
    key,
  };
  await submitMutation(mutation);
}
async function saveGame() {
  if (!gameCanSave.value || !gamePolicy.value) return;
  const body = {
    version: gamePolicy.value.version,
    config: buildGameConfig(),
    reason: gameDraft.value.reason.trim(),
  };
  const key = keyForMutation({
    brandId: props.brandId,
    entityId: gameId.value,
    operation: "game",
    body,
  });
  await submitMutation({
    lane: "game",
    brandId: props.brandId,
    gameId: gameId.value,
    body,
    key,
  });
}
async function submitMutation(mutation: FrozenMutation) {
  if (saving.value) return;
  const ticket = generation;
  const actor = props.account.id;
  saving.value = mutation.lane;
  error.value = "";
  notice.value = "";
  uncertain.value = mutation;
  try {
    const item =
      mutation.lane === "brand"
        ? await api.updateBrandPolicy(
            mutation.brandId,
            mutation.body as {
              version: number;
              config: BrandConfig;
              reason: string;
            },
            mutation.key,
          )
        : await api.updateGamePolicy(
            mutation.brandId,
            mutation.gameId,
            mutation.body as {
              version: number;
              config: GameConfig;
              reason: string;
            },
            mutation.key,
          );
    if (!current(ticket, mutation.brandId, actor)) return;
    if (
      item.brand_id !== mutation.brandId ||
      (mutation.lane === "game" &&
        (item as GamePolicy).game_id !== mutation.gameId)
    ) {
      error.value = "保存响应与当前策略不匹配，请重新读取确认结果。";
      return;
    }
    uncertain.value = null;
    if (mutation.lane === "brand") {
      brandPolicy.value = item as BrandPolicy;
      applyBrandDraft(item as BrandPolicy);
    } else {
      gamePolicy.value = item as GamePolicy;
      applyGameDraft(item as GamePolicy);
    }
    notice.value = "投注策略已保存并生效。";
  } catch (cause) {
    if (!current(ticket, mutation.brandId, actor)) return;
    const status = cause instanceof AdminApiError ? cause.status : 0;
    sessionExpired(cause);
    if (status === 409) {
      uncertain.value = null;
      reconciling.value = true;
      if (mutation.lane === "brand") await loadBrand(true);
      else await loadGame(mutation.gameId, true);
      if (current(ticket, mutation.brandId, actor)) {
        error.value =
          "策略版本已变化。已尝试读取最新版本；请核对后再次提交，当前未保存的输入已保留。";
        reconciling.value = false;
      }
    } else if (status === 0 || status >= 500) {
      uncertain.value = mutation;
      error.value =
        "保存结果尚未确认。请按原请求重试，或刷新读取以核对版本；重试会复用原请求内容和幂等键。";
    } else {
      uncertain.value = null;
      error.value = explain(cause, "保存投注策略失败。");
    }
  } finally {
    if (
      ticket === generation &&
      props.brandId === mutation.brandId &&
      props.account.id === actor
    )
      saving.value = "";
  }
}

function reconcile() {
  if (!uncertain.value) return;
  const { lane, gameId: pendingGameId } = uncertain.value;
  uncertain.value = null;
  reconciling.value = true;
  notice.value = "正在读取最新版本；表单输入会保留。";
  void (async () => {
    if (lane === "brand") await loadBrand(true);
    else await loadGame(pendingGameId || gameId.value, true);
    reconciling.value = false;
  })();
}
function changeSelectedGame(id: string) {
  gameRead++;
  gamePolicy.value = null;
  gameDraft.value = emptyGameDraft();
  gameId.value = id;
  if (id) void loadGame(id);
}

watch(
  () => [
    props.brandId,
    props.account,
    rights.value.policyView,
    rights.value.policyWrite,
    rights.value.gameView,
  ],
  () => {
    generation++;
    brandRead++;
    gameRead++;
    gamesRead++;
    clearVisibleData();
    error.value = "";
    if (rights.value.policyView) void loadBrand();
    if (rights.value.gameView) void loadGames();
  },
  { deep: true, flush: "sync" },
);
onMounted(() => {
  if (rights.value.policyView) void loadBrand();
  if (rights.value.gameView) void loadGames();
});
onUnmounted(() => {
  live = false;
  generation++;
});
</script>

<template>
  <section class="bet-policy-settings" data-testid="bet-policy-settings">
    <header class="policy-header">
      <div>
        <p class="eyebrow">{{ display("投注策略") }}</p>
        <h2>{{ display("品牌与彩种投注限额") }}</h2>
        <p>{{ t("彩种设置覆盖品牌默认值；单注、单期和单用户单期上限分别生效。", "Game settings override brand defaults. Per-bet, per-period, and per-user per-period limits apply independently.") }}</p>
      </div>
      <span class="brand-pill">{{ t("品牌 · ", "Brand · ") }}{{ brandId || t("未选择", "Not selected") }}</span>
    </header>

    <p v-if="account.super_admin" class="callout">
      {{ t("超级管理员仅可按显式权限查看，不能修改投注策略。", "Super administrators may view only with explicit permission and cannot modify betting policies.") }}
    </p>
    <p class="callout">
      {{ t("所有积分均按整数字符串处理。品牌限额留空表示不限；彩种可继承、指定数值或设为不限。", "Points are handled as integer strings. Blank brand limits mean unlimited; games can inherit, set a value, or be unlimited.") }}
    </p>
    <p v-if="!rights.policyView" class="callout" role="status">
      {{ t("当前账号缺少品牌投注策略读取权限。", "This account lacks permission to view brand betting policies.") }}
    </p>
    <section v-if="!rights.policyView && rights.gameView" class="policy-card">
      <div class="card-heading">
        <div>
          <h3>{{ t("彩种目录", "Game catalog") }}</h3>
          <p>{{ t("仅展示当前账号获准查看的彩种目录，不读取彩种投注策略。", "Shows only the game catalog this account is allowed to view; game betting policies are not loaded here.") }}</p>
        </div>
        <button
          class="secondary"
          type="button"
          :disabled="loadingGames"
          @click="loadGames"
        >
          {{ loadingGames ? t("读取中…", "Loading…") : t("刷新彩种目录", "Refresh game catalog") }}
        </button>
      </div>
      <p v-if="loadingGames && !games.length" class="muted" role="status">
        {{ t("正在读取彩种目录…", "Loading game catalog…") }}
      </p>
      <p v-else-if="!games.length" class="muted">{{ t("彩种目录为空，或尚未读取。", "The game catalog is empty or has not been loaded.") }}</p>
      <ul v-else class="catalog-list">
        <li v-for="game in games" :key="game.id">
          {{ game.name }} · {{ game.id }}
        </li>
      </ul>
    </section>
    <template v-if="rights.policyView">
      <div v-if="error" class="message error" role="alert">
        <span>{{ display(error) }}</span>
        <button
          v-if="!uncertain"
          type="button"
          class="secondary"
          @click="() => loadBrand()"
        >
          {{ t("重新读取品牌策略", "Reload brand policy") }}
        </button>
      </div>
      <p v-if="notice" class="message success" role="status">{{ display(notice) }}</p>
      <div v-if="uncertain" class="message warning" role="alert">
        <span
          >{{ t("有一项写入结果待确认。表单已冻结，不能编辑或创建新的写入请求。", "A write result is pending confirmation. The form is frozen; you cannot edit it or create another write request.") }}</span
        >
        <button
          type="button"
          class="primary"
          :disabled="!!saving"
          @click="retryUncertain"
        >
          {{ t("使用原请求重试", "Retry original request") }}
        </button>
        <button
          type="button"
          class="secondary"
          :disabled="!!saving"
          @click="reconcile"
        >
          {{ t("重新读取核对", "Reload and reconcile") }}
        </button>
      </div>

      <section class="policy-card">
        <div class="card-heading">
          <div>
            <h3>{{ t("品牌默认策略", "Brand default policy") }}</h3>
            <p v-if="brandPolicy">
              {{ t("版本", "Version") }} {{ brandPolicy.version }} · {{ brandPolicy.brand_id }}
            </p>
          </div>
          <button
            class="secondary"
            type="button"
            :disabled="loadingBrand || !!saving || !!uncertain"
            @click="() => loadBrand()"
          >
            {{ loadingBrand ? t("读取中…", "Loading…") : t("刷新", "Refresh") }}
          </button>
        </div>
        <p v-if="loadingBrand && !brandPolicy" class="muted" role="status">
          {{ t("正在读取品牌策略…", "Loading brand policy…") }}
        </p>
        <p v-else-if="!brandPolicy && !error" class="muted">
          {{ t("尚无可显示的品牌策略。", "No brand policy is available to display.") }}
        </p>
        <form
          v-if="brandPolicy"
          class="editor-grid"
          @submit.prevent="saveBrand"
        >
          <label
            >{{ t("品牌：最低单注积分", "Brand: minimum points per bet") }}<input
              v-model.trim="brandDraft.min_bet_points"
              inputmode="numeric"
              autocomplete="off"
              :disabled="!canWrite || !!uncertain || reconciling"
          /></label>
          <label
            >{{ t("品牌：单注上限积分（留空不限）", "Brand: maximum points per bet (blank for unlimited)") }}<input
              v-model.trim="brandDraft.max_bet_points"
              inputmode="numeric"
              autocomplete="off"
              :placeholder="t('不限', 'Unlimited')"
              :disabled="!canWrite || !!uncertain || reconciling"
          /></label>
          <label
            >{{ t("品牌：单期上限积分（留空不限）", "Brand: maximum points per period (blank for unlimited)") }}<input
              v-model.trim="brandDraft.max_period_points"
              inputmode="numeric"
              autocomplete="off"
              :placeholder="t('不限', 'Unlimited')"
              :disabled="!canWrite || !!uncertain || reconciling"
          /></label>
          <label
            >{{ t("品牌：单用户单期上限积分（留空不限）", "Brand: maximum points per user per period (blank for unlimited)") }}<input
              v-model.trim="brandDraft.max_user_period_points"
              inputmode="numeric"
              autocomplete="off"
              :placeholder="t('不限', 'Unlimited')"
              :disabled="!canWrite || !!uncertain || reconciling"
          /></label>
          <label class="check-label"
            ><input
              v-model="brandDraft.user_cancel_allowed"
              type="checkbox"
              :disabled="!canWrite || !!uncertain || reconciling"
            />{{ t("品牌：允许用户撤销投注", "Brand: allow users to cancel bets") }}</label
          >
          <label class="wide"
            >{{ t("品牌：变更原因（必填，最多 500 UTF-8 字节）", "Brand: change reason (required, up to 500 UTF-8 bytes)") }}<textarea
              v-model="brandDraft.reason"
              rows="3"
              maxlength="500"
              :disabled="!canWrite || !!uncertain || reconciling"
            />
          </label>
          <p v-if="!brandInputsValid" class="field-error wide" role="alert">
            {{ t("请填写正整数；最低单注积分不得高于任一有限上限。", "Enter positive whole numbers. The minimum per bet cannot exceed any finite limit.") }}
          </p>
          <p
            v-if="!reasonValid(brandDraft.reason) && brandDraft.reason"
            class="field-error wide"
            role="alert"
          >
            {{ t("原因必须非空，且不超过 500 UTF-8 字节。", "The reason is required and must be no longer than 500 UTF-8 bytes.") }}
          </p>
          <p v-if="brandPolicy && !canWrite" class="muted wide">
            {{ t("当前账号仅有读取权限，无法修改品牌策略。", "This account has view-only access and cannot modify the brand policy.") }}
          </p>
          <div v-if="canWrite" class="wide action-row">
            <button class="primary" type="submit" :disabled="!brandCanSave">
              {{ saving === "brand" ? t("保存中…", "Saving…") : t("保存品牌策略", "Save brand policy") }}
            </button>
          </div>
        </form>
      </section>

      <section class="policy-card game-card">
        <div class="card-heading">
          <div>
            <h3>{{ t("彩种策略覆盖", "Game policy overrides") }}</h3>
            <p>
              {{ t("可用彩种目录仅在具备彩种读取权限时显示；也可输入彩种 ID 直接读取策略。", "The game catalog appears only with game-view permission. You can also enter a game ID to load its policy directly.") }}
            </p>
          </div>
        </div>
        <div class="game-picker">
          <label v-if="rights.gameView"
            >{{ t("选择彩种", "Select a game") }}
            <select
              :value="gameId"
              :disabled="loadingGames || !!saving || !!uncertain"
              @change="
                changeSelectedGame(($event.target as HTMLSelectElement).value)
              "
            >
              <option value="">{{ t("请选择彩种", "Select a game") }}</option>
              <option v-for="game in games" :key="game.id" :value="game.id">
                {{ game.name }} · {{ game.id }}
              </option>
            </select>
          </label>
          <button
            v-if="rights.gameView"
            class="secondary"
            type="button"
            :disabled="loadingGames || !!saving || !!uncertain"
            @click="loadGames"
          >
            {{ loadingGames ? t("读取目录中…", "Loading catalog…") : t("刷新彩种目录", "Refresh game catalog") }}
          </button>
          <p
            v-if="rights.gameView && !games.length && !loadingGames"
            class="muted"
          >
            {{ t("彩种目录为空，或尚未读取。", "The game catalog is empty or has not been loaded.") }}
          </p>
          <p v-if="!rights.gameView" class="muted">
            {{ t("当前账号没有彩种目录读取权限；可以用下方 ID 直接读取策略。", "This account cannot view the game catalog; use the ID below to load a policy directly.") }}
          </p>
        </div>
        <form
          class="game-id-form"
          @submit.prevent="changeSelectedGame(directGameId.trim())"
        >
          <label
            >{{ t("游戏：彩种 ID", "Game ID") }}<input
              v-model.trim="directGameId"
              autocomplete="off"
              :placeholder="t('输入彩种 ID', 'Enter game ID')"
              :disabled="!!saving || !!uncertain"
          /></label>
          <button
            class="secondary"
            type="submit"
            :disabled="
              !directGameId.trim() || loadingGame || !!saving || !!uncertain
            "
          >
            {{ loadingGame ? t("读取中…", "Loading…") : t("读取彩种策略", "Load game policy") }}
          </button>
        </form>
        <p v-if="loadingGame && !gamePolicy" class="muted" role="status">
          {{ t("正在读取彩种策略…", "Loading game policy…") }}
        </p>
        <p v-else-if="gameId && !gamePolicy && !error" class="muted">
          {{ t("未读取到此彩种策略。", "No policy was found for this game.") }}
        </p>
        <form
          v-if="gamePolicy"
          class="editor-grid game-editor"
          @submit.prevent="saveGame"
        >
          <p class="policy-version wide">
            {{ t("游戏：", "Game: ") }}{{ gamePolicy.game_id }} · {{ t("版本", "Version") }} {{ gamePolicy.version }}
          </p>
          <template
            v-for="field in [
              { key: 'min_bet_points', label: '最低单注积分', minimum: true },
              { key: 'max_bet_points', label: '单注上限积分', minimum: false },
              {
                key: 'max_period_points',
                label: '单期上限积分',
                minimum: false,
              },
              {
                key: 'max_user_period_points',
                label: '单用户单期上限积分',
                minimum: false,
              },
            ]"
            :key="field.key"
          >
            <label
              >{{ t("游戏：", "Game: ") }}{{ display(field.label) }}
              <select
                :aria-label="`${t('游戏：', 'Game: ')}${display(field.label)}`"
                v-model="
                  gameDraft[
                    field.key as keyof Pick<
                      DraftGame,
                      | 'min_bet_points'
                      | 'max_bet_points'
                      | 'max_period_points'
                      | 'max_user_period_points'
                    >
                  ].mode
                "
                :disabled="!canWrite || !!uncertain || reconciling"
              >
                <option value="inherit">{{ t("继承品牌默认", "Inherit brand default") }}</option>
                <option value="value">{{ t("指定数值", "Set a value") }}</option>
                <option v-if="!field.minimum" value="unlimited">{{ t("不限", "Unlimited") }}</option>
              </select>
            </label>
            <label
              v-if="
                gameDraft[
                  field.key as keyof Pick<
                    DraftGame,
                    | 'min_bet_points'
                    | 'max_bet_points'
                    | 'max_period_points'
                    | 'max_user_period_points'
                  >
                ].mode === 'value'
              "
              >{{ t("游戏：", "Game: ") }}{{ display(field.label) }}{{ t("数值", "value") }}
              <input
                :aria-label="`${t('游戏：', 'Game: ')}${display(field.label)}${t('数值', 'value')}`"
                v-model.trim="
                  gameDraft[
                    field.key as keyof Pick<
                      DraftGame,
                      | 'min_bet_points'
                      | 'max_bet_points'
                      | 'max_period_points'
                      | 'max_user_period_points'
                    >
                  ].points
                "
                inputmode="numeric"
                autocomplete="off"
                :disabled="!canWrite || !!uncertain || reconciling"
              />
            </label>
          </template>
          <label
            >{{ t("游戏：用户撤销权限", "Game: user cancellation") }}<select
              v-model="gameDraft.user_cancel_allowed"
              :disabled="!canWrite || !!uncertain || reconciling"
            >
              <option value="inherit">{{ t("继承品牌默认", "Inherit brand default") }}</option>
              <option value="allow">{{ t("允许", "Allow") }}</option>
              <option value="deny">{{ t("禁止", "Deny") }}</option>
            </select></label
          >
          <label class="wide"
            >{{ t("游戏：变更原因（必填，最多 500 UTF-8 字节）", "Game: change reason (required, up to 500 UTF-8 bytes)") }}<textarea
              v-model="gameDraft.reason"
              rows="3"
              maxlength="500"
              :disabled="!canWrite || !!uncertain || reconciling"
            />
          </label>
          <p v-if="!gameInputsValid" class="field-error wide" role="alert">
            {{ t("覆盖值必须为正整数；生效后的最低单注积分不得高于任一有限上限。", "Override values must be positive whole numbers. The effective minimum per bet cannot exceed any finite limit.") }}
          </p>
          <p v-if="gamePolicy && !canWrite" class="muted wide">
            {{ t("当前账号仅有读取权限，无法修改彩种策略。", "This account has view-only access and cannot modify the game policy.") }}
          </p>
          <div v-if="canWrite" class="wide action-row">
            <button class="primary" type="submit" :disabled="!gameCanSave">
              {{ saving === "game" ? t("保存中…", "Saving…") : t("保存彩种策略", "Save game policy") }}
            </button>
          </div>
        </form>
      </section>
    </template>
  </section>
</template>

<style scoped>
.bet-policy-settings {
  width: 100%;
  min-width: 0;
  color: #252a36;
  overflow-wrap: anywhere;
}
.policy-header,
.card-heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  min-width: 0;
}
.policy-header {
  margin: 4px 0 15px;
}
.policy-header h2 {
  font-size: 19px;
  margin: 3px 0 5px;
}
.policy-header p,
.card-heading p,
.muted {
  font-size: 11px;
  line-height: 1.55;
  color: #737b8c;
  margin: 0;
  overflow-wrap: anywhere;
}
.eyebrow {
  font-size: 10px;
  letter-spacing: 1.2px;
  color: #8991a2;
  font-weight: 700;
  margin: 0;
}
.brand-pill {
  flex: none;
  border-radius: 20px;
  background: #f1f3f8;
  padding: 7px 11px;
  color: #626b7c;
  font-size: 11px;
}
.callout,
.message {
  border-radius: 8px;
  background: #f4f6fc;
  color: #626b7c;
  padding: 10px 12px;
  font-size: 11px;
  line-height: 1.6;
  margin: 12px 0;
  overflow-wrap: anywhere;
}
.message {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.error {
  background: #fff0ef;
  color: #a3312c;
}
.success {
  background: #edf8f0;
  color: #28623b;
}
.warning {
  background: #fff7e6;
  color: #745417;
}
.policy-card {
  background: #fff;
  border: 1px solid #e9ebf0;
  border-radius: 11px;
  padding: 16px;
  min-width: 0;
  box-shadow: 0 2px 8px #1e2a5008;
  margin: 13px 0;
}
.card-heading {
  margin-bottom: 14px;
}
.card-heading h3 {
  font-size: 14px;
  margin: 0 0 4px;
}
.editor-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  min-width: 0;
}
.editor-grid label,
.game-id-form label,
.game-picker label {
  display: grid;
  gap: 6px;
  font-size: 11px;
  font-weight: 600;
  min-width: 0;
}
.editor-grid input:not([type="checkbox"]),
.editor-grid select,
.editor-grid textarea,
.game-id-form input,
.game-picker select {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  border: 1px solid #dfe3eb;
  border-radius: 7px;
  background: #fff;
  padding: 9px 10px;
  color: #252a36;
  font: inherit;
}
.editor-grid input:disabled,
.editor-grid select:disabled,
.editor-grid textarea:disabled,
.game-id-form input:disabled {
  background: #f6f7f9;
  color: #737b8c;
}
.check-label {
  display: flex !important;
  align-items: center;
  gap: 8px;
}
.check-label input {
  margin: 0;
}
.wide {
  grid-column: 1/-1;
  min-width: 0;
}
.field-error {
  font-size: 11px;
  color: #a3312c;
  margin: 0;
}
.action-row {
  display: flex;
  justify-content: flex-end;
}
.game-card {
  margin-top: 16px;
}
.game-picker,
.game-id-form {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  min-width: 0;
  margin: 10px 0;
}
.game-picker label,
.game-id-form label {
  flex: 1;
}
.game-id-form {
  padding-bottom: 12px;
  border-bottom: 1px solid #eceef3;
}
.policy-version {
  font-size: 11px;
  color: #737b8c;
  margin: 0;
}
.primary,
.secondary {
  border: 0;
  border-radius: 7px;
  padding: 9px 12px;
  font: inherit;
  font-size: 11px;
  cursor: pointer;
  white-space: nowrap;
}
.primary {
  background: #405bd6;
  color: #fff;
}
.secondary {
  background: #eef1f7;
  color: #455064;
}
.primary:disabled,
.secondary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.catalog-list {
  margin: 0;
  padding-left: 20px;
  font-size: 11px;
  line-height: 1.8;
  overflow-wrap: anywhere;
}
@media (max-width: 620px) {
  .policy-header {
    flex-direction: column;
  }
  .editor-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .wide {
    grid-column: auto;
  }
  .game-picker,
  .game-id-form {
    align-items: stretch;
    flex-direction: column;
  }
  .message {
    align-items: flex-start;
    flex-direction: column;
  }
  .brand-pill {
    white-space: normal;
  }
}
</style>
