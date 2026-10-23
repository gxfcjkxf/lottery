<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { vInlineValidation } from '@lottery/shared'
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createRuleVersionsApi, type GameRecord } from "./rule-versions-api";
import { adminKey, useAdminI18n } from "./i18n";
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
const { t, display: render } = useAdminI18n();
const ui = (key: string) => t(adminKey(key));
const labels: Record<string, string> = {
  "投注策略": "admin.copy.c29852b8", "品牌与彩种投注限额": "admin.copy.bf8aa701",
  "最低单注积分": "admin.copy.3890e98a", "单注上限积分": "admin.copy.1083c018",
  "单期上限积分": "admin.copy.1455f7ed", "单用户单期上限积分": "admin.copy.46ccdedd",
};
const textEn: Record<string, string> = {
  "品牌策略响应与当前品牌不匹配。": "admin.copy.de368a44", "读取品牌投注策略失败。": "admin.copy.e0817861",
  "彩种策略响应与当前品牌或彩种不匹配。": "admin.copy.9560b971", "读取彩种投注策略失败。": "admin.copy.ea846a99",
  "读取彩种目录失败。": "admin.copy.c052ffbf", "保存响应与当前策略不匹配，请重新读取确认结果。": "admin.copy.c721cb8d",
  "投注策略已保存并生效。": "admin.copy.97f13a5b",
  "策略版本已变化。已尝试读取最新版本；请核对后再次提交，当前未保存的输入已保留。": "admin.copy.3142ca3b",
  "保存结果尚未确认。请按原请求重试，或刷新读取以核对版本；重试会复用原请求内容和幂等键。": "admin.copy.fb62e6a5",
  "保存投注策略失败。": "admin.copy.5861c31c", "正在读取最新版本；表单输入会保留。": "admin.copy.0d5783ad",
};
const display = (value: string | import("@lottery/shared").LocalizedMessage) => {
  if (typeof value !== "string") return displayPublicError(value);
  const key = textEn[value] ?? labels[value];
  return key ? t(adminKey(key)) : render(value);
};

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
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
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
  if (cause instanceof Error && publicErrorMessage(cause)) return publicErrorMessage(cause);
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
  new TextEncoder().encode(value.trim()).length <= 500;
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
async function retryUncertain() {
  if (!uncertain.value || saving.value) return;
  await submitMutation(uncertain.value);
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
async function changeSelectedGame(id: string) {
  gameRead++;
  gamePolicy.value = null;
  gameDraft.value = emptyGameDraft();
  gameId.value = id;
  if (id) await loadGame(id);
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
        <h2>{{ display("品牌与彩种投注限额") }}</h2>
        <p>{{ t('admin.copy.8caceb4b') }}</p>
      </div>
      <span class="brand-pill">{{ t('admin.copy.55bdd7a5') }}<AssociationName kind="brand" :id="brandId" :locale="associationLocale" /></span>
    </header>

    <p v-if="account.super_admin" class="callout">
      {{ t('admin.copy.2599abb5') }}
    </p>
    <p class="callout">
      {{ t('admin.copy.8f1f1315') }}
    </p>
    <p v-if="!rights.policyView" class="callout" role="status">
      {{ t('admin.copy.aebcd8b6') }}
    </p>
    <section v-if="!rights.policyView && rights.gameView" class="policy-card">
      <div class="card-heading">
        <div>
          <h3>{{ t('admin.copy.6b42b097') }}</h3>
        </div>
        <button v-refresh-button="loadingGames ? t('admin.copy.27342bf3') : t('admin.copy.f8a8e3bf')"
          class="button button-secondary"
          type="button"
          :disabled="loadingGames"
          @click="withGlobalLoading(loadGames)"
        >
          {{ loadingGames ? t('admin.copy.27342bf3') : t('admin.copy.f8a8e3bf') }}
        </button>
      </div>
      <p v-global-loading v-if="loadingGames && !games.length" class="muted" role="status">
        {{ t('admin.copy.4d221807') }}
      </p>
      <p v-else-if="!games.length" class="muted empty-state">{{ t('admin.copy.a493ddef') }}</p>
      <ul v-else class="catalog-list">
        <li v-for="game in games" :key="game.id">
          {{ game.name }}
        </li>
      </ul>
    </section>
    <template v-if="rights.policyView">
      <div v-if="error" class="message error" role="alert">
        <span>{{ display(error) }}</span>
        <button v-refresh-button="t('admin.copy.3dc463ff')"
          v-if="!uncertain"
          type="button"
          class="button button-secondary"
          @click="withGlobalLoading(async () => { return () => loadBrand() })"
        >
          {{ t('admin.copy.3dc463ff') }}
        </button>
      </div>
      <p v-if="notice" class="message success" role="status">{{ display(notice) }}</p>
      <div v-if="uncertain" class="message warning" role="alert">
        <span
          >{{ t('admin.copy.932f172b') }}</span
        >
        <button
          v-write-guard="Boolean(saving)"
          type="button"
          class="button button-primary"
          :disabled="!!saving"
          @click="withGlobalLoading(retryUncertain)"
        >
          {{ t('admin.copy.c6a6aa0c') }}
        </button>
        <button v-refresh-button="t('admin.copy.515305da')"
          type="button"
          class="button button-secondary"
          :disabled="!!saving"
          @click="withGlobalLoading(reconcile)"
        >
          {{ t('admin.copy.515305da') }}
        </button>
      </div>

      <section class="policy-card">
        <div class="card-heading">
          <div>
            <h3>{{ t('admin.copy.5172b25d') }}</h3>
            <p v-if="brandPolicy">
               · <AssociationName :record="brandPolicy" kind="brand" :id="brandPolicy.brand_id" :locale="associationLocale" />
            </p>
          </div>
          <button v-refresh-button="loadingBrand ? t('admin.copy.27342bf3') : t('admin.copy.894e2a7d')"
            class="button button-secondary"
            type="button"
            :disabled="loadingBrand || !!saving || !!uncertain"
            @click="withGlobalLoading(async () => { return () => loadBrand() })"
          >
            {{ loadingBrand ? t('admin.copy.27342bf3') : t('admin.copy.894e2a7d') }}
          </button>
        </div>
        <p v-global-loading v-if="loadingBrand && !brandPolicy" class="muted" role="status">
          {{ t('admin.copy.da04c68a') }}
        </p>
        <p v-else-if="!brandPolicy && !error" class="muted empty-state">
          {{ t('admin.copy.5e311882') }}
        </p>
        <form v-write-guard="Boolean(saving)" novalidate v-inline-validation
          v-if="brandPolicy"
          class="editor-grid"
          @submit.prevent="withGlobalLoading(saveBrand)"
        >
          <label
            >{{ t('admin.copy.ab573088') }}<input
              v-model.trim="brandDraft.min_bet_points"
              inputmode="numeric"
              autocomplete="off"
              :disabled="!canWrite || !!uncertain || reconciling"
          /></label>
          <label
            >{{ t('admin.copy.3a263c82') }}<input
              v-model.trim="brandDraft.max_bet_points"
              inputmode="numeric"
              autocomplete="off"
              :placeholder="t('admin.copy.374fa8f7')"
              :disabled="!canWrite || !!uncertain || reconciling"
          /></label>
          <label
            >{{ t('admin.copy.b23a9c93') }}<input
              v-model.trim="brandDraft.max_period_points"
              inputmode="numeric"
              autocomplete="off"
              :placeholder="t('admin.copy.374fa8f7')"
              :disabled="!canWrite || !!uncertain || reconciling"
          /></label>
          <label
            >{{ t('admin.copy.61dba6bb') }}<input
              v-model.trim="brandDraft.max_user_period_points"
              inputmode="numeric"
              autocomplete="off"
              :placeholder="t('admin.copy.374fa8f7')"
              :disabled="!canWrite || !!uncertain || reconciling"
          /></label>
          <label class="check-label"
            ><input
              v-model="brandDraft.user_cancel_allowed"
              type="checkbox"
              :disabled="!canWrite || !!uncertain || reconciling"
            />{{ t('admin.copy.f6d733a8') }}</label
          >

          <p v-if="!brandInputsValid" class="field-error wide" role="alert">
            {{ t('admin.copy.331a9f86') }}
          </p>
          <p
            v-if="!reasonValid(brandDraft.reason) && brandDraft.reason"
            class="field-error wide"
            role="alert"
          >
            {{ t('admin.copy.9f181711') }}
          </p>
          <p v-if="brandPolicy && !canWrite" class="muted wide">
            {{ t('admin.copy.486eece3') }}
          </p>
          <div v-if="canWrite" class="wide action-row">
            <button class="button button-primary" type="submit" :disabled="!brandCanSave">
              {{ saving === "brand" ? t('admin.copy.2462134b') : t('admin.copy.cce06086') }}
            </button>
          </div>
        </form>
      </section>

      <section class="policy-card game-card">
        <div class="card-heading">
          <div>
            <h3>{{ t('admin.copy.5c087e13') }}</h3>
            <p>
              {{ t('admin.copy.06508683') }}
            </p>
          </div>
        </div>
        <div class="game-picker">
          <label v-if="rights.gameView"
            >{{ t('admin.copy.68ccd146') }}
            <select
              :value="gameId"
              :disabled="loadingGames || !!saving || !!uncertain"
              @change="withGlobalLoading(changeSelectedGame, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">{{ t('admin.copy.3aba80cd') }}</option>
              <option v-for="game in games" :key="game.id" :value="game.id">
                {{ game.name }}
              </option>
            </select>
          </label>
          <button v-refresh-button="loadingGames ? t('admin.copy.b1808de5') : t('admin.copy.f8a8e3bf')"
            v-if="rights.gameView"
            class="button button-secondary"
            type="button"
            :disabled="loadingGames || !!saving || !!uncertain"
            @click="withGlobalLoading(loadGames)"
          >
            {{ loadingGames ? t('admin.copy.b1808de5') : t('admin.copy.f8a8e3bf') }}
          </button>
          <p
            v-if="rights.gameView && !games.length && !loadingGames"
            class="muted empty-state"
          >
            {{ t('admin.copy.a493ddef') }}
          </p>
          <p v-if="!rights.gameView" class="muted">
            {{ t('admin.copy.a60157b8') }}
          </p>
        </div>
        <form novalidate v-inline-validation
          class="game-id-form"
          @submit.prevent="withGlobalLoading(changeSelectedGame, directGameId.trim())"
        >
          <label
            >{{ t('admin.copy.43d91d0a') }}<input
              v-model.trim="directGameId"
              autocomplete="off"
              :placeholder="t('admin.copy.e143b771')"
              :disabled="!!saving || !!uncertain"
          /></label>
          <button
            class="button button-secondary"
            type="submit"
            :disabled="
              !directGameId.trim() || loadingGame || !!saving || !!uncertain
            "
          >
            {{ loadingGame ? t('admin.copy.27342bf3') : t('admin.copy.373b0a3d') }}
          </button>
        </form>
        <p v-global-loading v-if="loadingGame && !gamePolicy" class="muted" role="status">
          {{ t('admin.copy.739dd9f0') }}
        </p>
        <p v-else-if="gameId && !gamePolicy && !error" class="muted">
          {{ t('admin.copy.beecc9a2') }}
        </p>
        <form v-write-guard="Boolean(saving)" novalidate v-inline-validation
          v-if="gamePolicy"
          class="editor-grid game-editor"
          @submit.prevent="withGlobalLoading(saveGame)"
        >
          <p class="policy-version wide">
            {{ t('admin.copy.9661b2ca') }}<AssociationName :record="gamePolicy" kind="game" :id="gamePolicy.game_id" :locale="associationLocale" />
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
              >{{ t('admin.copy.9661b2ca') }}{{ display(field.label) }}
              <select
                :aria-label="`${t('admin.copy.9661b2ca')}${display(field.label)}`"
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
                <option value="inherit">{{ t('admin.copy.e6cd5a15') }}</option>
                <option value="value">{{ t('admin.copy.94924ab6') }}</option>
                <option v-if="!field.minimum" value="unlimited">{{ t('admin.copy.374fa8f7') }}</option>
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
              >{{ t('admin.copy.9661b2ca') }}{{ display(field.label) }}{{ t('admin.copy.46eb2008') }}
              <input
                :aria-label="`${t('admin.copy.9661b2ca')}${display(field.label)}${t('admin.copy.46eb2008')}`"
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
            >{{ t('admin.copy.ee6240ba') }}<select
              v-model="gameDraft.user_cancel_allowed"
              :disabled="!canWrite || !!uncertain || reconciling"
            >
              <option value="inherit">{{ t('admin.copy.e6cd5a15') }}</option>
              <option value="allow">{{ t('admin.copy.068bad15') }}</option>
              <option value="deny">{{ t('admin.copy.02430d7c') }}</option>
            </select></label
          >

          <p v-if="!gameInputsValid" class="field-error wide" role="alert">
            {{ t('admin.copy.1e61ff65') }}
          </p>
          <p v-if="gamePolicy && !canWrite" class="muted wide">
            {{ t('admin.copy.2ee5cdd3') }}
          </p>
          <div v-if="canWrite" class="wide action-row">
            <button class="button button-primary" type="submit" :disabled="!gameCanSave">
              {{ saving === "game" ? t('admin.copy.2462134b') : t('admin.copy.90512a84') }}
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
  color: var(--muted);
  margin: 0;
  overflow-wrap: anywhere;
}
.eyebrow {
  font-size: 11px;
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
  background: var(--surface);
  border: 1px solid var(--line);
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
  background: var(--surface);
  padding: 9px 10px;
  color: #252a36;
  font: inherit;
}
.editor-grid input:disabled,
.editor-grid select:disabled,
.editor-grid textarea:disabled,
.game-id-form input:disabled {
  background: #f6f7f9;
  color: var(--muted);
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
  border-bottom: 1px solid var(--line);
}
.policy-version {
  font-size: 11px;
  color: var(--muted);
  margin: 0;
}
.button-primary,
.button-secondary {
  border: 0;
  border-radius: 7px;
  padding: 9px 12px;
  font: inherit;
  font-size: 11px;
  cursor: pointer;
  white-space: nowrap;
}
.button-primary {
  background: #405bd6;
  color: var(--surface);
}
.button-secondary {
  background: #eef1f7;
  color: #455064;
}
.button-primary:disabled,
.button-secondary:disabled {
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
