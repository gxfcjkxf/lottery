<script setup lang="ts">
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
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
const displayDate = (value: string | null | undefined) => formatDisplayDateTime(value, displayLocale.value);
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { vInlineValidation } from '@lottery/shared'
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import {
  AdminApiError,
  createIdempotencyKey,
  type AdminAccount,
} from "./admin-api";
import { createRuleVersionsApi, type GameRecord } from "./rule-versions-api";
import { useAdminI18n } from "./i18n";
import { WALLET_SOURCES } from "@lottery/shared";
import type { LocalizedMessage } from "@lottery/shared";
import {
  createWithdrawalPolicyApi,
  isValidTurnoverMultiple,
  withdrawalPolicyPermissions,
  type BrandWithdrawalConfig,
  type BrandWithdrawalPolicy,
  type GameWithdrawalConfig,
  type GameWithdrawalPolicy,
  type PolicyRevision,
  type WithdrawalSource,
  type UpdateBrandWithdrawalPolicyBody,
  type UpdateGameWithdrawalPolicyBody,
} from "./withdrawal-policy-api";
import {
  classifyPolicyWriteFailure,
  findPendingPolicyWrite,
  freezePolicyBody,
  getPendingPolicyWrite,
  policyBodyFingerprint,
  clearPendingPolicyWrite,
  setPendingPolicyWrite,
} from "./withdrawal-policy-state";

type HistoryPage = { items: PolicyRevision[]; limit: number; offset: number };
type Lane = "brand" | "game";
type BrandBody = UpdateBrandWithdrawalPolicyBody;
type GameBody = UpdateGameWithdrawalPolicyBody;
type FrozenMutation = {
  actorId: string;
  lane: Lane;
  brandId: string;
  gameId: string;
  body: Readonly<BrandBody | GameBody>;
  key: string;
  fingerprint: string;
};
type BrandDraft = Omit<BrandWithdrawalConfig, "max_points"> & {
  max_points_input: string;
  reason: string;
};
type GameDraft = { turnover_multiple: string; reason: string };
class LocalizedPolicyError extends Error {
  constructor(readonly copy: LocalizedMessage) {
    super();
  }
}

const DEFAULT_BRAND_WITHDRAWAL_CONFIG: BrandWithdrawalConfig = {
  enabled: false,
  min_points: "1",
  max_points: null,
  allowed_sources: ["recharge", "winning", "gift"],
  review_mode: "manual",
  turnover_multiple: "1",
};

const props = defineProps<{ account: AdminAccount; brandId: string }>();
const emit = defineEmits<{ (event: "session-invalid"): void }>();
const api = createWithdrawalPolicyApi();
const gameApi = createRuleVersionsApi();
const { t, message } = useAdminI18n();
const rights = computed(() =>
  withdrawalPolicyPermissions(props.account, props.brandId),
);
const brandPolicy = ref<BrandWithdrawalPolicy | null>(null);
const gamePolicy = ref<GameWithdrawalPolicy | null>(null);
const games = ref<GameRecord[]>([]);
const gameId = ref("");
const directGameId = ref("");
const brandDraft = ref<BrandDraft>(emptyBrandDraft());
const gameDraft = ref<GameDraft>(emptyGameDraft());
const brandHistory = ref<HistoryPage | null>(null);
const gameHistory = ref<HistoryPage | null>(null);
const brandHistoryOffset = ref(0);
const gameHistoryOffset = ref(0);
const loadingBrand = ref(false);
const loadingGame = ref(false);
const loadingGames = ref(false);
const loadingBrandHistory = ref(false);
const loadingGameHistory = ref(false);
const saving = ref<Lane | "">("");
const reconciling = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const confirmation = ref<FrozenMutation | null>(null);
const uncertain = ref<FrozenMutation | null>(null);
let generation = 0;
let brandRead = 0;
let gameRead = 0;
let gamesRead = 0;
let brandHistoryRead = 0;
let gameHistoryRead = 0;
let live = true;

function emptyBrandDraft(): BrandDraft {
  return {
    ...structuredClone(DEFAULT_BRAND_WITHDRAWAL_CONFIG),
    max_points_input: "",
    reason: "",
  };
}
function emptyGameDraft(): GameDraft {
  return { turnover_multiple: "", reason: "" };
}
function contextKey(actorId: string, brandId: string, lane: Lane, game = "") {
  return JSON.stringify([actorId, brandId, lane, game]);
}
function getPending(
  actorId = props.account.id,
  brandId = props.brandId,
  lane?: Lane,
  game = "",
) {
  if (lane)
    return getPendingPolicyWrite<FrozenMutation>(
      contextKey(actorId, brandId, lane, game),
    );
  return (
    getPendingPolicyWrite<FrozenMutation>(
      contextKey(actorId, brandId, "brand"),
    ) ??
    (game
      ? getPendingPolicyWrite<FrozenMutation>(
          contextKey(actorId, brandId, "game", game),
        )
      : null) ??
    null
  );
}
function current(
  ticket: number,
  brandId: string,
  actorId: string,
  requireView = true,
) {
  return (
    live &&
    ticket === generation &&
    props.brandId === brandId &&
    props.account.id === actorId &&
    (!requireView || rights.value.view)
  );
}
function sameContext(ticket: number, brandId: string, actorId: string) {
  return (
    live &&
    ticket === generation &&
    props.brandId === brandId &&
    props.account.id === actorId
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
function explain(cause: unknown, fallback: LocalizedMessage) {
  if (cause instanceof LocalizedPolicyError) return cause.copy;
  return cause instanceof Error && publicErrorMessage(cause) ? publicErrorMessage(cause) : fallback;
}
function displayMessage(value: string | LocalizedMessage) {
  return typeof value === "string" ? value : t(value);
}
function clearVisibleData() {
  brandPolicy.value = null;
  gamePolicy.value = null;
  games.value = [];
  gameId.value = "";
  directGameId.value = "";
  brandDraft.value = emptyBrandDraft();
  gameDraft.value = emptyGameDraft();
  brandHistory.value = null;
  gameHistory.value = null;
  brandHistoryOffset.value = 0;
  gameHistoryOffset.value = 0;
  confirmation.value = null;
  uncertain.value = null;
  loadingBrand.value = false;
  loadingGame.value = false;
  loadingGames.value = false;
  loadingBrandHistory.value = false;
  loadingGameHistory.value = false;
  saving.value = "";
  reconciling.value = false;
  notice.value = "";
}

function buildBrandConfig(): BrandWithdrawalConfig {
  return {
    enabled: brandDraft.value.enabled,
    min_points: brandDraft.value.min_points,
    max_points:
      brandDraft.value.max_points_input === ""
        ? null
        : brandDraft.value.max_points_input,
    allowed_sources: WALLET_SOURCES.filter((source) => brandDraft.value.allowed_sources.includes(source)),
    review_mode: brandDraft.value.review_mode,
    turnover_multiple: brandDraft.value.turnover_multiple,
  };
}
function isPositiveInt64(value: string): boolean {
  if (!/^[1-9][0-9]*$/.test(value)) return false;
  try {
    return BigInt(value) <= 9223372036854775807n;
  } catch {
    return false;
  }
}
const brandInputsValid = computed(() => {
  const config = buildBrandConfig();
  return (
    isPositiveInt64(config.min_points) &&
    (config.max_points === null ||
      (isPositiveInt64(config.max_points) &&
        BigInt(config.min_points) <= BigInt(config.max_points))) &&
    config.allowed_sources.length > 0 &&
    new Set(config.allowed_sources).size === config.allowed_sources.length &&
    config.allowed_sources.every((source) =>
      (WALLET_SOURCES as readonly string[]).includes(source),
    ) &&
    isValidTurnoverMultiple(config.turnover_multiple)
  );
});
const gameInputValid = computed(
  () =>
    gameDraft.value.turnover_multiple === "" ||
    isValidTurnoverMultiple(gameDraft.value.turnover_multiple),
);
const reasonValid = (value: string) =>
  new TextEncoder().encode(value.trim()).length <= 500;
const canWrite = computed(
  () => rights.value.write && !props.account.super_admin,
);
const pendingInContext = computed(
  () =>
    uncertain.value ??
    getPending(props.account.id, props.brandId, "game", gameId.value) ??
    getPending(props.account.id, props.brandId, "brand"),
);
const brandCanSave = computed(
  () =>
    canWrite.value &&
    brandPolicy.value !== null &&
    brandInputsValid.value &&
    reasonValid(brandDraft.value.reason) &&
    !saving.value &&
    !uncertain.value &&
    !confirmation.value &&
    !reconciling.value &&
    !loadingBrand.value,
);
const gameCanSave = computed(
  () =>
    canWrite.value &&
    gamePolicy.value !== null &&
    gameInputValid.value &&
    reasonValid(gameDraft.value.reason) &&
    !saving.value &&
    !uncertain.value &&
    !confirmation.value &&
    !reconciling.value &&
    !loadingGame.value,
);

async function loadBrand(preserveDraft = false) {
  if (!rights.value.view || !props.brandId) return;
  const brandId = props.brandId;
  const actorId = props.account.id;
  const ticket = generation;
  const read = ++brandRead;
  loadingBrand.value = true;
  error.value = "";
  try {
    const item = await api.getBrandPolicy(brandId);
    if (!current(ticket, brandId, actorId) || read !== brandRead) return;
    if (item.brand_id !== brandId)
      throw new LocalizedPolicyError(message('admin.copy.b9a1214f'));
    brandPolicy.value = item;
    if (
      !getPendingPolicyWrite(contextKey(actorId, brandId, "brand")) &&
      !preserveDraft
    )
      applyBrandDraft(item);
  } catch (cause) {
    if (!current(ticket, brandId, actorId) || read !== brandRead) return;
    brandPolicy.value = null;
    sessionExpired(cause);
    error.value = explain(cause, message('admin.copy.8fbd2a77'));
  } finally {
    if (current(ticket, brandId, actorId) && read === brandRead)
      loadingBrand.value = false;
  }
}
function applyBrandDraft(item: BrandWithdrawalPolicy) {
  brandDraft.value = {
    ...structuredClone(item.config),
    max_points_input: item.config.max_points ?? "",
    allowed_sources: [...item.config.allowed_sources],
    reason: "",
  };
}
function applyGameDraft(item: GameWithdrawalPolicy) {
  gameDraft.value = {
    turnover_multiple: item.config.turnover_multiple ?? "",
    reason: "",
  };
}
async function loadGame(id = gameId.value, preserveDraft = false) {
  const normalizedId = id.trim();
  if (!rights.value.view || !normalizedId || !props.brandId) return;
  gameId.value = normalizedId;
  const brandId = props.brandId;
  const actorId = props.account.id;
  const ticket = generation;
  const read = ++gameRead;
  loadingGame.value = true;
  gamePolicy.value = null;
  gameHistory.value = null;
  gameHistoryOffset.value = 0;
  gameHistoryRead++;
  error.value = "";
  try {
    const item = await api.getGamePolicy(brandId, normalizedId);
    if (
      !current(ticket, brandId, actorId) ||
      read !== gameRead ||
      gameId.value !== normalizedId
    )
      return;
    if (item.brand_id !== brandId || item.game_id !== normalizedId)
      throw new LocalizedPolicyError(message('admin.copy.dc9b1fa2'));
    gamePolicy.value = item;
    if (
      !getPendingPolicyWrite(
        contextKey(actorId, brandId, "game", normalizedId),
      ) &&
      !preserveDraft
    )
      applyGameDraft(item);
  } catch (cause) {
    if (
      !current(ticket, brandId, actorId) ||
      read !== gameRead ||
      gameId.value !== normalizedId
    )
      return;
    gamePolicy.value = null;
    sessionExpired(cause);
    error.value = explain(cause, message('admin.copy.1a5ae7cf'));
  } finally {
    if (
      current(ticket, brandId, actorId) &&
      read === gameRead &&
      gameId.value === normalizedId
    )
      loadingGame.value = false;
  }
}
async function loadGames() {
  if (!rights.value.gameView || !props.brandId) return;
  const brandId = props.brandId;
  const actorId = props.account.id;
  const ticket = generation;
  const read = ++gamesRead;
  loadingGames.value = true;
  try {
    const result = await gameApi.getGames(brandId);
    if (
      !sameContext(ticket, brandId, actorId) ||
      read !== gamesRead ||
      !rights.value.gameView
    )
      return;
    games.value = result.games.filter((game) => game.brand_id === brandId);
  } catch (cause) {
    if (
      !sameContext(ticket, brandId, actorId) ||
      read !== gamesRead ||
      !rights.value.gameView
    )
      return;
    games.value = [];
    sessionExpired(cause);
    error.value = explain(cause, message('admin.copy.c052ffbf'));
  } finally {
    if (sameContext(ticket, brandId, actorId) && read === gamesRead)
      loadingGames.value = false;
  }
}
async function loadHistory(lane: Lane, offset: number) {
  if (
    !rights.value.view ||
    !props.brandId ||
    (lane === "game" && !gameId.value)
  )
    return;
  const brandId = props.brandId;
  const actorId = props.account.id;
  const selectedGameId = gameId.value;
  const ticket = generation;
  const read = lane === "brand" ? ++brandHistoryRead : ++gameHistoryRead;
  if (lane === "brand") loadingBrandHistory.value = true;
  else loadingGameHistory.value = true;
  try {
    const page =
      lane === "brand"
        ? await api.getBrandHistory(brandId, 50, offset)
        : await api.getGameHistory(brandId, selectedGameId, 50, offset);
    if (
      !current(ticket, brandId, actorId) ||
      (lane === "brand"
        ? read !== brandHistoryRead
        : read !== gameHistoryRead) ||
      (lane === "game" && gameId.value !== selectedGameId)
    )
      return;
    if (
      page.items.some(
        (item) =>
          item.brand_id !== brandId ||
          (lane === "brand"
            ? item.game_id !== ""
            : item.game_id !== selectedGameId),
      )
    )
      throw new LocalizedPolicyError(message('admin.copy.b5e0ce9c'));
    if (lane === "brand") {
      brandHistory.value = page;
      brandHistoryOffset.value = offset;
    } else {
      gameHistory.value = page;
      gameHistoryOffset.value = offset;
    }
  } catch (cause) {
    if (
      !current(ticket, brandId, actorId) ||
      (lane === "brand"
        ? read !== brandHistoryRead
        : read !== gameHistoryRead) ||
      (lane === "game" && gameId.value !== selectedGameId)
    )
      return;
    sessionExpired(cause);
    error.value = explain(cause, message('admin.copy.126ed15f'));
  } finally {
    if (current(ticket, brandId, actorId)) {
      if (lane === "brand" && read === brandHistoryRead)
        loadingBrandHistory.value = false;
      if (lane === "game" && read === gameHistoryRead)
        loadingGameHistory.value = false;
    }
  }
}
async function changeSelectedGame(id: string) {
  confirmation.value = null;
  gameRead++;
  gameHistoryRead++;
  gamePolicy.value = null;
  gameHistory.value = null;
  gameDraft.value = emptyGameDraft();
  gameId.value = id.trim();
  if (gameId.value) {
    uncertain.value =
      getPending(props.account.id, props.brandId, "game", gameId.value) ??
      getPending(props.account.id, props.brandId, "brand");
    await loadGame(gameId.value, Boolean(uncertain.value));
  } else {
    uncertain.value = getPending(props.account.id, props.brandId, "brand");
  }
}

function requestBrandConfirmation() {
  if (!brandCanSave.value || !brandPolicy.value) return;
  const body = freezePolicyBody({
    version: brandPolicy.value.version,
    config: buildBrandConfig(),
    reason: brandDraft.value.reason.trim(),
  }) as Readonly<BrandBody>;
  createConfirmation("brand", "", body);
}
function requestGameConfirmation() {
  if (!gameCanSave.value || !gamePolicy.value) return;
  const body = freezePolicyBody({
    version: gamePolicy.value.version,
    config: {
      turnover_multiple:
        gameDraft.value.turnover_multiple === ""
          ? null
          : gameDraft.value.turnover_multiple,
    },
    reason: gameDraft.value.reason.trim(),
  }) as Readonly<GameBody>;
  createConfirmation("game", gameId.value, body);
}
function createConfirmation(
  lane: Lane,
  selectedGame: string,
  body: Readonly<BrandBody | GameBody>,
) {
  const fingerprint = policyBodyFingerprint(body);
  confirmation.value = {
    actorId: props.account.id,
    lane,
    brandId: props.brandId,
    gameId: selectedGame,
    body,
    key: createIdempotencyKey(),
    fingerprint,
  };
}
function cancelConfirmation() {
  confirmation.value = null;
}
async function confirmAndSubmit() {
  const mutation = confirmation.value;
  if (
    !mutation ||
    saving.value ||
    mutation.actorId !== props.account.id ||
    mutation.brandId !== props.brandId
  )
    return;
  if (policyBodyFingerprint(mutation.body) !== mutation.fingerprint) {
    confirmation.value = null;
    error.value = message('admin.copy.a858f2bf');
    return;
  }
  confirmation.value = null;
  await submitMutation(mutation);
}
async function retryUncertain() {
  const mutation = uncertain.value;
  if (!mutation || saving.value || !canWrite.value) return;
  await submitMutation(mutation);
}
async function submitMutation(mutation: FrozenMutation) {
  const registryKey = contextKey(
    mutation.actorId,
    mutation.brandId,
    mutation.lane,
    mutation.gameId,
  );
  const previous = getPendingPolicyWrite<FrozenMutation>(registryKey);
  if (saving.value || (previous && previous.key !== mutation.key)) return;
  const ticket = generation;
  saving.value = mutation.lane;
  error.value = "";
  notice.value = "";
  uncertain.value = mutation;
  setPendingPolicyWrite(registryKey, mutation);
  try {
    const item =
      mutation.lane === "brand"
        ? await api.updateBrandPolicy(
            mutation.brandId,
            mutation.body as Readonly<BrandBody>,
            mutation.key,
          )
        : await api.updateGamePolicy(
            mutation.brandId,
            mutation.gameId,
            mutation.body as Readonly<GameBody>,
            mutation.key,
          );
    clearPendingPolicyWrite(registryKey);
    if (
      !current(ticket, mutation.brandId, mutation.actorId) ||
      (mutation.lane === "game" && gameId.value !== mutation.gameId)
    )
      return;
    if (
      item.brand_id !== mutation.brandId ||
      (mutation.lane === "game" &&
        (item as GameWithdrawalPolicy).game_id !== mutation.gameId)
    ) {
      error.value = message('admin.copy.aed6d0e9');
      uncertain.value = null;
      return;
    }
    uncertain.value = null;
    if (mutation.lane === "brand") {
      brandPolicy.value = item as BrandWithdrawalPolicy;
      applyBrandDraft(item as BrandWithdrawalPolicy);
      brandHistory.value = null;
      void loadHistory("brand", 0);
      if (gameId.value) void loadGame(gameId.value, true);
    } else {
      gamePolicy.value = item as GameWithdrawalPolicy;
      applyGameDraft(item as GameWithdrawalPolicy);
      gameHistory.value = null;
      void loadHistory("game", 0);
    }
    notice.value = message('admin.copy.a0a25fb6');
  } catch (cause) {
    const status = cause instanceof AdminApiError ? cause.status : 0;
    const outcome = classifyPolicyWriteFailure(status);
    if (
      !sameContext(ticket, mutation.brandId, mutation.actorId) ||
      (mutation.lane === "game" && gameId.value !== mutation.gameId)
    ) {
      if (outcome !== "uncertain") clearPendingPolicyWrite(registryKey);
      return;
    }
    if (sessionExpired(cause)) {
      clearPendingPolicyWrite(registryKey);
      return;
    }
    if (outcome === "conflict") {
      clearPendingPolicyWrite(registryKey);
      uncertain.value = null;
      reconciling.value = true;
      if (mutation.lane === "brand") await loadBrand(true);
      else await loadGame(mutation.gameId, true);
      if (sameContext(ticket, mutation.brandId, mutation.actorId)) {
        error.value = message('admin.copy.8c257e14');
        reconciling.value = false;
      }
    } else if (outcome === "uncertain") {
      setPendingPolicyWrite(registryKey, mutation);
      uncertain.value = mutation;
      error.value = message('admin.copy.e63ba0cd');
    } else {
      clearPendingPolicyWrite(registryKey);
      uncertain.value = null;
      error.value = explain(
        cause,
        message('admin.copy.6f7c7600'),
      );
    }
  } finally {
    if (
      ticket === generation &&
      props.brandId === mutation.brandId &&
      props.account.id === mutation.actorId
    )
      saving.value = "";
  }
}
async function reconcileUncertain() {
  const mutation = uncertain.value;
  if (!mutation || reconciling.value) return;
  const ticket = generation;
  reconciling.value = true;
  if (mutation.lane === "brand") await loadBrand(true);
  else await loadGame(mutation.gameId, true);
  if (
    !sameContext(ticket, mutation.brandId, mutation.actorId) ||
    (mutation.lane === "game" && gameId.value !== mutation.gameId)
  )
    return;
  // A read can show current state but provides no actor/request proof for an unknown write.
  if (
    getPendingPolicyWrite(
      contextKey(
        mutation.actorId,
        mutation.brandId,
        mutation.lane,
        mutation.gameId,
      ),
    )
  ) {
    error.value = message('admin.copy.2826c8fd');
  }
  reconciling.value = false;
}
async function historyPrevious(lane: Lane) {
  const offset =
    lane === "brand" ? brandHistoryOffset.value : gameHistoryOffset.value;
  if (offset > 0) await loadHistory(lane, Math.max(0, offset - 50));
}
async function historyNext(lane: Lane) {
  const page = lane === "brand" ? brandHistory.value : gameHistory.value;
  const offset =
    lane === "brand" ? brandHistoryOffset.value : gameHistoryOffset.value;
  if (page && page.items.length === 50) await loadHistory(lane, offset + 50);
}
function formatSources(sources: WithdrawalSource[]) {
  return sources
    .map(
      (source) =>
        ({
          recharge: t('admin.copy.6ffe1179'),
          winning: t('admin.copy.99c31497'),
          gift: t('admin.copy.2b8ae17c'),
          commission: t('admin.copy.101e2e66'),
        })[source],
    )
    .join(t('admin.copy.b71594e8'));
}
function configJson(config: unknown) {
  return JSON.stringify(config, null, 2);
}

watch(
  () => [
    props.brandId,
    props.account,
    rights.value.view,
    rights.value.write,
    rights.value.gameView,
  ],
  () => {
    generation++;
    brandRead++;
    gameRead++;
    gamesRead++;
    brandHistoryRead++;
    gameHistoryRead++;
    clearVisibleData();
    error.value = "";
    uncertain.value =
      getPending() ??
      findPendingPolicyWrite<FrozenMutation>(
        (pending) =>
          pending.actorId === props.account.id &&
          pending.brandId === props.brandId,
      );
    if (rights.value.view) {
      void loadBrand();
      if (uncertain.value?.lane === "game" && uncertain.value.gameId) {
        gameId.value = uncertain.value.gameId;
        directGameId.value = uncertain.value.gameId;
        void loadGame(uncertain.value.gameId, true);
      }
    }
    if (rights.value.gameView) void loadGames();
  },
  { deep: true, flush: "sync" },
);
onMounted(() => {
  uncertain.value =
    getPending() ??
    findPendingPolicyWrite<FrozenMutation>(
      (pending) =>
        pending.actorId === props.account.id &&
        pending.brandId === props.brandId,
    );
  if (rights.value.view) void loadBrand();
  if (
    rights.value.view &&
    uncertain.value?.lane === "game" &&
    uncertain.value.gameId
  ) {
    gameId.value = uncertain.value.gameId;
    directGameId.value = uncertain.value.gameId;
    void loadGame(uncertain.value.gameId, true);
  }
  if (rights.value.gameView) void loadGames();
});
onUnmounted(() => {
  live = false;
  generation++;
});
</script>

<template>
  <section class="withdrawal-policy" data-testid="withdrawal-policy-settings">
    <header class="policy-header">
      <div>
        <p class="eyebrow">{{ t('admin.copy.b258cb23') }}</p>
        <h2>{{ t('admin.copy.b5eed7bd') }}</h2>
      </div>
      <span class="brand-pill">{{ t('admin.copy.9c32661d') }} · <AssociationName kind="brand" :id="brandId" :locale="associationLocale" /></span>
    </header>

    <p class="callout warning" role="note">
      {{ t('admin.copy.eb850a76') }}
    </p>
    <p v-if="account.super_admin" class="callout">
      {{ t('admin.copy.bdc5d412') }}
    </p>
    <p v-if="!rights.view" class="callout" role="status">
      {{ t('admin.copy.bf76c8c5') }}
    </p>

    <section v-if="!rights.view && rights.gameView" class="policy-card">
      <div class="card-heading">
        <div>
          <h3>{{ t('admin.copy.6b42b097') }}</h3>
        </div>
        <button
          class="button button-secondary"
          type="button"
          :disabled="loadingGames"
          @click="withGlobalLoading(loadGames)"
        >
          {{ loadingGames ? t('admin.copy.27342bf3') : t('admin.copy.5d10b2de') }}
        </button>
      </div>
      <p v-global-loading v-if="loadingGames" class="muted" role="status">{{ t('admin.copy.4d221807') }}</p>
      <p v-else-if="!games.length" class="muted empty-state">{{ t('admin.copy.a493ddef') }}</p>
      <ul v-else class="catalog-list">
        <li v-for="game in games" :key="game.id">
          {{ game.name }}
        </li>
      </ul>
    </section>

    <template v-if="rights.view">
      <div v-if="error" class="message error" role="alert">
        <span>{{ displayMessage(error) }}</span>
      </div>
      <p v-if="notice" class="message success" role="status">{{ displayMessage(notice) }}</p>
      <div v-if="pendingInContext" class="message warning" role="alert">
        <span
          >{{ t('admin.copy.cca19357') }}</span
        >
        <button
          v-write-guard="Boolean(saving)"
          class="button button-primary"
          type="button"
          :disabled="!!saving || !canWrite"
          @click="withGlobalLoading(retryUncertain)"
        >
          {{ t('admin.copy.c6a6aa0c') }}
        </button>
        <button
          class="button button-secondary"
          type="button"
          :disabled="!!saving || reconciling"
          @click="withGlobalLoading(reconcileUncertain)"
        >
          {{ t('admin.copy.5d79fbcc') }}
        </button>
      </div>
      <section class="policy-card">
        <div class="card-heading">
          <div>
            <h3>{{ t('admin.copy.06fe3b08') }}</h3>
            <p v-if="brandPolicy">
               ·
              <AssociationName :record="brandPolicy" kind="brand" :id="brandPolicy.brand_id" :locale="associationLocale" /> {{ t('admin.copy.c6f1ca9e') }} {{ displayDate(brandPolicy.updated_at) }}
            </p>
          </div>
          <button v-refresh-button="loadingBrand ? t('admin.copy.27342bf3') : t('admin.copy.655019c3')"
            class="button button-secondary"
            type="button"
            :disabled="loadingBrand || !!saving || !!pendingInContext"
            @click="withGlobalLoading(async () => { return () => loadBrand() })"
          >
            {{ loadingBrand ? t('admin.copy.27342bf3') : t('admin.copy.655019c3') }}
          </button>
        </div>
        <p v-global-loading v-if="loadingBrand && !brandPolicy" class="muted" role="status">
          {{ t('admin.copy.9ad33899') }}
        </p>
        <p v-else-if="!brandPolicy && !error" class="muted empty-state">
          {{ t('admin.copy.6a43bda0') }}
        </p>
        <template v-if="brandPolicy">
          <dl class="saved-summary">
            <div>
              <dt>{{ t('admin.copy.a765b3ca') }}</dt>
              <dd>
                {{ brandPolicy.config.enabled ? t('admin.copy.c418ddd0') : t('admin.copy.a298c1aa') }}
              </dd>
            </div>
            <div>
              <dt>{{ t('admin.copy.cb3b3680') }}</dt>
              <dd>
                {{ brandPolicy.config.min_points }} {{ t('admin.copy.2284cf7b') }}
                {{ brandPolicy.config.max_points ?? t('admin.copy.5302e1a7') }}
              </dd>
            </div>
            <div>
              <dt>{{ t('admin.copy.4364767b') }}</dt>
              <dd>{{ formatSources(brandPolicy.config.allowed_sources) }}</dd>
            </div>
            <div>
              <dt>{{ t('admin.copy.85964287') }}</dt>
              <dd>
                {{
                  brandPolicy.config.review_mode === "manual"
                    ? t('admin.copy.f1e1066f')
                    : t('admin.copy.19cf2be7')
                }}
              </dd>
            </div>
            <div>
              <dt>{{ t('admin.copy.64f2bdc4') }}</dt>
              <dd>{{ brandPolicy.config.turnover_multiple }}</dd>
            </div>
          </dl>
          <form novalidate v-inline-validation class="editor-grid" @submit.prevent="withGlobalLoading(requestBrandConfirmation)">
            <label for="withdraw-brand-enabled"
              >{{ t('admin.copy.b79e77f2') }}<input
                id="withdraw-brand-enabled"
                v-model="brandDraft.enabled"
                type="checkbox"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
            /></label>
            <label for="withdraw-brand-min"
              >{{ t('admin.copy.f3a168b4') }}<input
                id="withdraw-brand-min"
                v-model.trim="brandDraft.min_points"
                inputmode="numeric"
                autocomplete="off"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
            /></label>
            <label for="withdraw-brand-max"
              >{{ t('admin.copy.a99f767f') }}<input
                id="withdraw-brand-max"
                v-model.trim="brandDraft.max_points_input"
                inputmode="numeric"
                autocomplete="off"
                :placeholder="t('admin.copy.5302e1a7')"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
            /></label>
            <fieldset class="source-field wide">
              <legend>{{ t('admin.copy.c0bdf463') }}</legend>
              <label for="withdraw-source-recharge"
                ><input
                  id="withdraw-source-recharge"
                  v-model="brandDraft.allowed_sources"
                  type="checkbox"
                  value="recharge"
                  :disabled="
                    !canWrite ||
                    !!pendingInContext ||
                    !!confirmation ||
                    reconciling
                  "
                />{{ t('admin.copy.6ffe1179') }}</label
              >
              <label for="withdraw-source-winning"
                ><input
                  id="withdraw-source-winning"
                  v-model="brandDraft.allowed_sources"
                  type="checkbox"
                  value="winning"
                  :disabled="
                    !canWrite ||
                    !!pendingInContext ||
                    !!confirmation ||
                    reconciling
                  "
                />{{ t('admin.copy.99c31497') }}</label
              >
              <label for="withdraw-source-gift"
                ><input
                  id="withdraw-source-gift"
                  v-model="brandDraft.allowed_sources"
                  type="checkbox"
                  value="gift"
                  :disabled="
                    !canWrite ||
                    !!pendingInContext ||
                    !!confirmation ||
                    reconciling
                  "
                />{{ t('admin.copy.2b8ae17c') }}</label
              >
              <label for="withdraw-source-commission"
                ><input
                  id="withdraw-source-commission"
                  v-model="brandDraft.allowed_sources"
                  type="checkbox"
                  value="commission"
                  :disabled="!canWrite || !!pendingInContext || !!confirmation || reconciling"
                />{{ t('admin.copy.101e2e66') }}</label
              >
            </fieldset>
            <label for="withdraw-brand-review"
              >{{ t('admin.copy.0af1bc63') }}<select
                id="withdraw-brand-review"
                v-model="brandDraft.review_mode"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
              >
                <option value="manual">{{ t('admin.copy.f1e1066f') }}</option>
                <option value="automatic">{{ t('admin.copy.19cf2be7') }}</option>
              </select></label
            >
            <label for="withdraw-brand-multiple"
              >{{ t('admin.copy.c104d4d5') }}<input
                id="withdraw-brand-multiple"
                v-model.trim="brandDraft.turnover_multiple"
                inputmode="decimal"
                autocomplete="off"
                :placeholder="t('admin.copy.c7a62fd5')"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
            /></label>

            <p v-if="!brandInputsValid" class="field-error wide" role="alert">
              {{ t('admin.copy.f9f48fa7') }}
            </p>
            <p
              v-if="brandDraft.reason && !reasonValid(brandDraft.reason)"
              class="field-error wide"
              role="alert"
            >
              {{ t('admin.copy.82d98d17') }}
            </p>
            <p v-if="!canWrite" class="muted wide">
              {{ t('admin.copy.29035ab8') }}
            </p>
            <div v-if="canWrite" class="wide action-row">
              <button class="button button-primary" type="submit" :disabled="!brandCanSave">
                {{ t('admin.copy.041b3e8e') }}
              </button>
            </div>
          </form>
        </template>
      </section>

      <section class="policy-card">
        <div class="card-heading">
          <div>
            <h3>{{ t('admin.copy.c42e6ae8') }}</h3>
            <p>{{ t('admin.copy.b7c3d9f6') }}</p>
          </div>
        </div>
        <div class="game-picker">
          <label v-if="rights.gameView" for="withdraw-game-select"
            >{{ t('admin.copy.68ccd146') }}<select
              id="withdraw-game-select"
          :aria-label="t('admin.copy.260664bf')"
              :value="gameId"
              :disabled="
                loadingGames || !!saving || !!pendingInContext || !!confirmation
              "
              @change="withGlobalLoading(changeSelectedGame, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">{{ t('admin.copy.3aba80cd') }}</option>
              <option
                v-if="
                  gamePolicy &&
                  gameId &&
                  !games.some((game) => game.id === gameId)
                "
                :value="gameId"
              >
                {{ t('admin.copy.c8c2e3f4') }} · {{ gameId }}
              </option>
              <option v-for="game in games" :key="game.id" :value="game.id">
                {{ game.name }}
              </option>
            </select></label
          >
          <button v-refresh-button="loadingGames ? t('admin.copy.b1808de5') : t('admin.copy.f8a8e3bf')"
            v-if="rights.gameView"
            class="button button-secondary"
            type="button"
            :disabled="loadingGames || !!saving"
            @click="withGlobalLoading(loadGames)"
          >
            {{ loadingGames ? t('admin.copy.b1808de5') : t('admin.copy.f8a8e3bf') }}
          </button>
          <span
            v-if="rights.gameView && !games.length && !loadingGames"
            class="muted"
            >{{ t('admin.copy.a493ddef') }}</span
          >
          <span v-if="!rights.gameView" class="muted"
            >{{ t('admin.copy.e013e750') }}</span
          >
        </div>
        <form novalidate v-inline-validation
          class="game-id-form"
          @submit.prevent="withGlobalLoading(changeSelectedGame, directGameId)"
        >
          <label for="withdraw-game-id"
            >{{ t('admin.copy.1341137a') }}<input
              id="withdraw-game-id"
              v-model.trim="directGameId"
              autocomplete="off"
              :placeholder="t('admin.copy.6c390a48')"
              :disabled="!!saving || !!pendingInContext || !!confirmation"
          /></label>
          <button
            class="button button-secondary"
            type="submit"
            :disabled="
              !directGameId.trim() ||
              loadingGame ||
              !!saving ||
              !!pendingInContext ||
              !!confirmation
            "
          >
            {{ loadingGame ? t('admin.copy.27342bf3') : t('admin.copy.bc479ccf') }}
          </button>
        </form>
        <p v-global-loading v-if="loadingGame && !gamePolicy" class="muted" role="status">
          {{ t('admin.copy.739dd9f0') }}
        </p>
        <p v-else-if="gameId && !gamePolicy && !error" class="muted empty-state">
          {{ t('admin.copy.ba17d22a') }}
        </p>
        <template v-if="gamePolicy">
          <dl class="saved-summary">
            <div>
              <dt>{{ t('admin.copy.23d2b485') }}</dt>
              <dd><AssociationName :record="gamePolicy" kind="game" :id="gamePolicy.game_id" :locale="associationLocale" /></dd>
            </div>

            <div>
              <dt>{{ t('admin.copy.eced7c37') }}</dt>
              <dd>{{ gamePolicy.effective.turnover_multiple }}</dd>
            </div>
            <div>
              <dt>{{ t('admin.copy.2bda19de') }}</dt>
              <dd>
                {{
                  gamePolicy.effective.source === "brand"
                    ? t('admin.copy.a70308e4')
                    : t('admin.copy.99af8a49')
                }}
              </dd>
            </div>

          </dl>
          <form novalidate v-inline-validation class="editor-grid" @submit.prevent="withGlobalLoading(requestGameConfirmation)">
            <label class="wide" for="withdraw-game-multiple"
              >{{ t('admin.copy.72ddec59') }}<input
                id="withdraw-game-multiple"
                v-model.trim="gameDraft.turnover_multiple"
                inputmode="decimal"
                autocomplete="off"
                :placeholder="t('admin.copy.e6cd5a15')"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
            /></label>
            <p v-if="!gameInputValid" class="field-error wide" role="alert">
              {{ t('admin.copy.85dd702c') }}
            </p>

            <p v-if="!canWrite" class="muted wide">
              {{ t('admin.copy.e9ac506e') }}
            </p>
            <div v-if="canWrite" class="wide action-row">
              <button class="button button-primary" type="submit" :disabled="!gameCanSave">
                {{ t('admin.copy.48a1572c') }}
              </button>
            </div>
          </form>
          <section class="history-panel">
            <div class="card-heading">
              <div>
              <h4>{{ t('admin.copy.8752bd11') }}</h4>
              </div>
              <button
                class="button button-secondary"
                type="button"
                :disabled="loadingGameHistory"
                @click="withGlobalLoading(loadHistory, 'game', 0)"
              >
                {{ loadingGameHistory ? t('admin.copy.27342bf3') : t('admin.copy.5cedae54') }}
              </button>
            </div>
            <p v-global-loading
              v-if="loadingGameHistory && !gameHistory"
              class="muted"
              role="status"
            >
              {{ t('admin.copy.b7842e6d') }}
            </p>
            <p
              v-else-if="gameHistory && !gameHistory.items.length"
              class="muted empty-state"
            >
              {{ t('admin.copy.0a4b249a') }}
            </p>
            <ol v-if="gameHistory?.items.length" class="history-list">
              <li v-for="item in gameHistory.items" :key="item.id">
                <h5> {{ displayDate(item.created_at) }}</h5>
                <p>
                  {{ t('admin.copy.76bff253') }}<AssociationName v-if="item.changed_by" :record="item" kind="admin" :id="item.changed_by" :locale="associationLocale" /><span v-else>{{ t('admin.copy.532e2e04') }}</span><span v-if="item.reason"> {{ t('admin.copy.2809a1e6') }}{{ item.reason }}</span>
                </p>
                <BusinessDetails :value="item.config" :locale="associationLocale" />
              </li>
            </ol>
            <div v-if="gameHistory" class="history-pagination">
              <button
                class="button button-secondary"
                type="button"
                :disabled="gameHistoryOffset === 0 || loadingGameHistory"
                @click="withGlobalLoading(historyPrevious, 'game')"
              >
                {{ t('admin.copy.dbe7a5d7') }}</button
              ><span
                >{{ t('admin.copy.4ffb01e2') }} {{ gameHistoryOffset }} · {{ t('admin.copy.62e6982d') }}
                {{ gameHistory.limit }} {{ t('admin.copy.067fed38') }}</span
              ><button
                class="button button-secondary"
                type="button"
                :disabled="gameHistory.items.length < 50 || loadingGameHistory"
                @click="withGlobalLoading(historyNext, 'game')"
              >
                {{ t('admin.copy.2e9c9e7a') }}
              </button>
            </div>
          </section>
        </template>
      </section>

      <section class="policy-card history-panel">
        <div class="card-heading">
          <div>
            <h3>{{ t('admin.copy.b7274a55') }}</h3>
          </div>
          <button
            class="button button-secondary"
            type="button"
            :disabled="loadingBrandHistory"
            @click="withGlobalLoading(loadHistory, 'brand', 0)"
          >
            {{ loadingBrandHistory ? t('admin.copy.27342bf3') : t('admin.copy.794f6f6a') }}
          </button>
        </div>
        <p v-global-loading
          v-if="loadingBrandHistory && !brandHistory"
          class="muted"
          role="status"
        >
          {{ t('admin.copy.c2a09339') }}
        </p>
        <p v-else-if="brandHistory && !brandHistory.items.length" class="muted empty-state">
          {{ t('admin.copy.0a4b249a') }}
        </p>
        <ol v-if="brandHistory?.items.length" class="history-list">
          <li v-for="item in brandHistory.items" :key="item.id">
            <h5> {{ displayDate(item.created_at) }}</h5>
            <p>
              {{ t('admin.copy.76bff253') }}<AssociationName v-if="item.changed_by" :record="item" kind="admin" :id="item.changed_by" :locale="associationLocale" /><span v-else>{{ t('admin.copy.532e2e04') }}</span><span v-if="item.reason"> {{ t('admin.copy.2809a1e6') }}{{ item.reason }}</span>
            </p>
            <BusinessDetails :value="item.config" :locale="associationLocale" />
          </li>
        </ol>
        <div v-if="brandHistory" class="history-pagination">
          <button
            class="button button-secondary"
            type="button"
            :disabled="brandHistoryOffset === 0 || loadingBrandHistory"
            @click="withGlobalLoading(historyPrevious, 'brand')"
          >
            {{ t('admin.copy.dbe7a5d7') }}</button
          ><span
            >{{ t('admin.copy.4ffb01e2') }} {{ brandHistoryOffset }} · {{ t('admin.copy.62e6982d') }}
            {{ brandHistory.limit }} {{ t('admin.copy.067fed38') }}</span
          ><button
            class="button button-secondary"
            type="button"
            :disabled="brandHistory.items.length < 50 || loadingBrandHistory"
            @click="withGlobalLoading(historyNext, 'brand')"
          >
            {{ t('admin.copy.2e9c9e7a') }}
          </button>
        </div>
      </section>
    </template>

    <section
      v-if="confirmation"
      class="confirmation-panel"
      aria-labelledby="withdraw-confirm-title"
    >
      <h3 id="withdraw-confirm-title">{{ t('admin.copy.bfe5b2ed') }}</h3>
      <p>
        {{ t('admin.copy.1cf64566') }}
        <AssociationName v-if="confirmation.lane === 'brand'" :record="brandPolicy" kind="brand" :id="confirmation.brandId" :locale="associationLocale" />
        <AssociationName v-else :record="gamePolicy" kind="game" :id="confirmation.gameId" :locale="associationLocale" />
      </p>
      <p v-if="confirmation.body.reason">{{ t('admin.copy.2809a1e6') }}{{ confirmation.body.reason }}</p>
      <BusinessDetails :value="confirmation.body.config" :locale="associationLocale" />
      <div class="action-row">
        <button class="button button-secondary" type="button" @click="cancelConfirmation">
          {{ t('admin.copy.1377553a') }}</button
        ><button
          v-write-guard="Boolean(saving)"
          class="button button-primary"
          type="button"
          :disabled="!!saving"
          @click="withGlobalLoading(confirmAndSubmit)"
        >
          {{ saving ? t('admin.copy.46511dc0') : t('admin.finance.savePolicy') }}
        </button>
      </div>
    </section>
  </section>
</template>

<style scoped>
.withdrawal-policy {
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
  overflow-wrap: anywhere;
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
.policy-card,
.confirmation-panel {
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
.card-heading h3,
.confirmation-panel h3 {
  font-size: 14px;
  margin: 0 0 4px;
}
.card-heading h4 {
  font-size: 13px;
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
.editor-grid input[type="checkbox"] {
  justify-self: start;
}
.wide {
  grid-column: 1/-1;
  min-width: 0;
}
.source-field {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  border: 1px solid var(--line);
  border-radius: 7px;
  padding: 10px;
  min-width: 0;
}
.source-field legend {
  font-size: 11px;
  font-weight: 600;
  padding: 0 4px;
}
.source-field label {
  display: flex;
  align-items: center;
  gap: 6px;
}
.field-error {
  font-size: 11px;
  color: #a3312c;
  margin: 0;
}
.action-row {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}
.button-primary,
.button-secondary {
  border: 0;
  border-radius: 7px;
  padding: 9px 12px;
  font: inherit;
  font-size: 11px;
  cursor: pointer;
  white-space: normal;
  overflow-wrap: anywhere;
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
.saved-summary {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin: 0 0 14px;
  padding: 12px;
  background: #f8f9fc;
  border-radius: 8px;
}
.saved-summary div {
  min-width: 0;
}
.saved-summary dt {
  font-size: 11px;
  color: var(--muted);
}
.saved-summary dd {
  font-size: 12px;
  font-weight: 600;
  margin: 3px 0 0;
  overflow-wrap: anywhere;
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
.history-panel {
  margin-top: 15px;
}
.history-list {
  padding-left: 20px;
}
.history-list li {
  border-top: 1px solid var(--line);
  padding: 10px 0;
  overflow-wrap: anywhere;
}
.history-list h5 {
  font-size: 11px;
  margin: 0 0 4px;
}
.history-list p {
  font-size: 11px;
  margin: 0 0 8px;
  color: #626b7c;
}
.history-list pre,
.confirmation-panel pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
  background: var(--canvas);
  border-radius: 7px;
  padding: 10px;
  font-size: 11px;
  max-width: 100%;
  overflow: auto;
}
.history-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 11px;
  flex-wrap: wrap;
}
.confirmation-panel {
  border-color: #aab6ee;
  background: #fbfcff;
}
.confirmation-panel p {
  font-size: 11px;
  overflow-wrap: anywhere;
}
.confirmation-panel code {
  overflow-wrap: anywhere;
  word-break: break-all;
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
  .saved-summary {
    grid-template-columns: minmax(0, 1fr);
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
