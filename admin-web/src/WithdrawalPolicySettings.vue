<script setup lang="ts">
import { isPoints, pointsMinor } from '@lottery/shared'

import SelectControl from '@lottery/shared/SelectControl.vue'
import { vRefreshButton } from "@lottery/shared";
import { dismissOperationToast, useOperationToast } from "@lottery/shared";
import DetailModal from '@lottery/shared/DetailModal.vue';
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
import { withGlobalLoading } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { vInlineValidation } from '@lottery/shared'
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import {
  AdminApiError,
  createIdempotencyKey,
  type AdminAccount,
} from "./admin-api";
import { readAllFilterOptions, FilterOptionsError, type FilterOption } from "@lottery/shared/filter-options";
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
  points_step: string;
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
const { t, message } = useAdminI18n();
const rights = computed(() =>
  withdrawalPolicyPermissions(props.account, props.brandId),
);
const brandPolicy = ref<BrandWithdrawalPolicy | null>(null);
const gamePolicy = ref<GameWithdrawalPolicy | null>(null);
const games = ref<FilterOption[]>([]);
const gameId = ref("");
const brandDraft = ref<BrandDraft>(emptyBrandDraft());
const gameDraft = ref<GameDraft>(emptyGameDraft());
const loadingBrand = ref(false);
const loadingGame = ref(false);
const loadingGames = ref(false);
const catalogFailed = ref(false);
const saving = ref<Lane | "">("");
const reconciling = ref(false);
const error = ref<string | LocalizedMessage>("");
const notice = ref<string | LocalizedMessage>("");
const confirmation = ref<FrozenMutation | null>(null);
const confirmationDetails = computed(() => {
  const current = confirmation.value;
  if (!current) return null;
  if (current.lane === 'game') return current.body.config;
  const body = current.body as BrandBody;
  return {
    enabled: body.config.enabled,
    min_points: body.config.min_points,
    max_points: body.config.max_points,
    points_step: body.points_step,
    allowed_sources: body.config.allowed_sources,
    review_mode: body.config.review_mode,
    turnover_multiple: body.config.turnover_multiple,
  };
});
const uncertain = ref<FrozenMutation | null>(null);
let generation = 0;
let brandRead = 0;
let gameRead = 0;
let gamesRead = 0;
let live = true;

function emptyBrandDraft(): BrandDraft {
  return {
    ...structuredClone(DEFAULT_BRAND_WITHDRAWAL_CONFIG),
    points_step: "1",
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
  if ((cause instanceof AdminApiError || cause instanceof FilterOptionsError) && cause.status === 401) {
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
  brandDraft.value = emptyBrandDraft();
  gameDraft.value = emptyGameDraft();
  confirmation.value = null;
  uncertain.value = null;
  loadingBrand.value = false;
  loadingGame.value = false;
  loadingGames.value = false;
  catalogFailed.value = false;
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
function isPositivePoints(value: unknown): value is string {
  return isPoints(value) && pointsMinor(value) > 0n;
}
const brandInputsValid = computed(() => {
  const config = buildBrandConfig();
  return (
    isPositivePoints(brandDraft.value.points_step) &&
    isPositivePoints(config.min_points) &&
    (config.max_points === null ||
      (isPositivePoints(config.max_points) &&
        pointsMinor(config.min_points) <= pointsMinor(config.max_points))) &&
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
  Array.from(value.trim()).length <= 500;
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
    points_step: item.points_step,
    max_points_input: item.config.max_points ?? "",
    allowed_sources: [...item.config.allowed_sources],
    reason: "",
  };
}
function restorePendingDraft() {
  const pending = uncertain.value;
  if (!pending) return;
  if (pending.lane === "brand") {
    const config = pending.body.config as BrandWithdrawalConfig;
    brandDraft.value = { ...structuredClone(config), points_step: (pending.body as BrandBody).points_step, max_points_input: config.max_points ?? "", reason: pending.body.reason };
  } else {
    const config = pending.body.config as GameWithdrawalConfig;
    gameDraft.value = { turnover_multiple: config.turnover_multiple ?? "", reason: pending.body.reason };
  }
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
  if (!rights.value.view || !props.brandId) return;
  const brandId = props.brandId;
  const actorId = props.account.id;
  const ticket = generation;
  const read = ++gamesRead;
  loadingGames.value = true;
  catalogFailed.value = false;
  try {
    const result = await readAllFilterOptions({ surface: 'admin', brandId, resource: 'withdrawal_policy', kind: 'game' });
    if (
      !sameContext(ticket, brandId, actorId) ||
      read !== gamesRead ||
      !rights.value.view
    )
      return;
    games.value = result;
  } catch (cause) {
    if (
      !sameContext(ticket, brandId, actorId) ||
      read !== gamesRead ||
      !rights.value.view
    )
      return;
    games.value = [];
    catalogFailed.value = true;
    sessionExpired(cause);
    error.value = explain(cause, message('admin.copy.c052ffbf'));
  } finally {
    if (sameContext(ticket, brandId, actorId) && read === gamesRead)
      loadingGames.value = false;
  }
}
async function changeSelectedGame(id: string) {
  confirmation.value = null;
  gameRead++;
  gamePolicy.value = null;
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
    points_step: brandDraft.value.points_step,
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
  dismissOperationToast();
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
      await loadBrand();
      if (gameId.value) void loadGame(gameId.value, true);
    } else {
      gamePolicy.value = item as GameWithdrawalPolicy;
      applyGameDraft(item as GameWithdrawalPolicy);
      await loadGame(mutation.gameId);
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

watch(
  () => [
    props.brandId,
    props.account,
    rights.value.view,
    rights.value.write,
  ],
  () => {
    generation++;
    brandRead++;
    gameRead++;
    gamesRead++;
    clearVisibleData();
    error.value = "";
    uncertain.value =
      getPending() ??
      findPendingPolicyWrite<FrozenMutation>(
        (pending) =>
          pending.actorId === props.account.id &&
          pending.brandId === props.brandId,
      );
    restorePendingDraft();
    if (rights.value.view) {
      void loadBrand();
      if (uncertain.value?.lane === "game" && uncertain.value.gameId) {
        gameId.value = uncertain.value.gameId;
        void loadGame(uncertain.value.gameId, true);
      }
    }
    if (rights.value.view) void loadGames();
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
  restorePendingDraft();
  if (rights.value.view) void loadBrand();
  if (
    rights.value.view &&
    uncertain.value?.lane === "game" &&
    uncertain.value.gameId
  ) {
    gameId.value = uncertain.value.gameId;
    void loadGame(uncertain.value.gameId, true);
  }
  if (rights.value.view) void loadGames();
});
onUnmounted(() => {
  live = false;
  generation++;
});

useOperationToast(() => notice.value, value => displayMessage(value), 'success');
useOperationToast(() => error.value, value => displayMessage(value), 'error');
</script>

<template>
  <section class="withdrawal-policy brand-settings-panel" data-testid="withdrawal-policy-settings">

    <p v-if="!rights.view" class="callout" role="status">
      {{ t('admin.copy.bf76c8c5') }}
    </p>

    <template v-if="rights.view">

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
        <button v-refresh-button="t('admin.copy.deca021e')" v-if="error && !pendingInContext" type="button" class="button button-secondary" :disabled="loadingBrand || !!saving" @click="withGlobalLoading(loadBrand)">{{ t('admin.copy.deca021e') }}</button>
        <p v-global-loading v-if="loadingBrand && !brandPolicy" class="muted" role="status">
          {{ t('admin.copy.9ad33899') }}
        </p>
        <p v-else-if="!brandPolicy && !error" class="muted empty-state">
          {{ t('admin.copy.6a43bda0') }}
        </p>
        <template v-if="brandPolicy">
          <dl v-if="!canWrite" class="saved-summary">
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
            <div><dt>{{ t('admin.withdrawal.pointsStep') }}</dt><dd>{{ brandPolicy.points_step }}</dd></div>
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
          <form v-if="canWrite" novalidate v-inline-validation class="editor-grid ui-form" @submit.prevent="withGlobalLoading(requestBrandConfirmation)">
            <label class="withdrawal-toggle wide" for="withdraw-brand-enabled"
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
            <label for="withdraw-brand-step">{{ t('admin.withdrawal.pointsStep') }}<input id="withdraw-brand-step" :aria-label="t('admin.withdrawal.pointsStep')" aria-describedby="withdraw-brand-step-hint" v-model.trim="brandDraft.points_step" inputmode="decimal" autocomplete="off" :disabled="!canWrite || !!pendingInContext || !!confirmation || reconciling" /><small id="withdraw-brand-step-hint">{{ t('admin.withdrawal.pointsStepHint') }}</small></label>
            <label for="withdraw-brand-min"
              >{{ t('admin.copy.f3a168b4') }}<input
                id="withdraw-brand-min"
                v-model.trim="brandDraft.min_points"
                inputmode="decimal"
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
                inputmode="decimal"
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
              >{{ t('admin.copy.0af1bc63') }}<SelectControl id="withdraw-brand-review" v-model="brandDraft.review_mode" :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                " :label="String(t('admin.copy.0af1bc63'))" :options="[{ value: 'manual', label: (String(t('admin.copy.f1e1066f'))).trim() }, { value: 'automatic', label: (String(t('admin.copy.19cf2be7'))).trim() }]" /></label
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

            <div v-if="canWrite" class="wide action-row ui-form-actions">
              <button class="button button-primary" type="submit" :disabled="!brandCanSave">
                {{ t('admin.copy.780ef330') }}
              </button>
            </div>
          </form>
        </template>
      </section>

      <section v-if="loadingGames || games.length || catalogFailed || gamePolicy" class="policy-card">
        <div class="card-heading">
          <div>
            <h3>{{ t('admin.withdrawal.customGameOverride') }}</h3>
          </div>
        </div>
        <div class="game-picker">
          <label v-if="rights.view" for="withdraw-game-select"
            >{{ t('admin.copy.68ccd146') }}<SelectControl id="withdraw-game-select" :label="t('admin.copy.260664bf')" :model-value="gameId" :disabled="
                loadingGames || !!saving || !!pendingInContext || !!confirmation
              " @change="withGlobalLoading(changeSelectedGame, $event)" :options="[{ value: '', label: (String(t('admin.copy.3aba80cd'))).trim() }, ...((gamePolicy &&
        gameId &&
        !games.some((game) => game.id === gameId)) ? [{ value: gameId, label: (String(t('admin.copy.c8c2e3f4'))).trim() }] : []), ...((games).map((game) => ({ value: game.id, label: (String(game.name)).trim() })))]" /></label
          >
          <button
            v-refresh-button="loadingGames ? t('admin.copy.b1808de5') : t('admin.copy.f8a8e3bf')"
            v-if="catalogFailed"
            class="button button-secondary"
            type="button"
            :disabled="loadingGames || !!saving"
            @click="withGlobalLoading(loadGames)"
          >
            {{ loadingGames ? t('admin.copy.b1808de5') : t('admin.copy.f8a8e3bf') }}
          </button>
          <span
            v-if="rights.view && !games.length && !loadingGames"
            class="muted"
            >{{ t('admin.copy.a493ddef') }}</span
          >
          <span v-if="!rights.view" class="muted"
            >{{ t('admin.copy.e013e750') }}</span
          >
        </div>
        <p v-global-loading v-if="loadingGame && !gamePolicy" class="muted" role="status">
          {{ t('admin.copy.739dd9f0') }}
        </p>
        <p v-else-if="gameId && !gamePolicy && !error" class="muted empty-state">
          {{ t('admin.copy.ba17d22a') }}
        </p>
        <template v-if="gamePolicy">
          <dl v-if="!canWrite" class="saved-summary">
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
          <form v-if="canWrite" novalidate v-inline-validation class="editor-grid ui-form" @submit.prevent="withGlobalLoading(requestGameConfirmation)">
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


            <div v-if="canWrite" class="wide action-row ui-form-actions">
              <button class="button button-primary" type="submit" :disabled="!gameCanSave">
                {{ t('admin.copy.780ef330') }}
              </button>
            </div>
          </form>
        </template>
      </section>

    </template>

    <DetailModal v-if="confirmation" class="brand-settings-dialog" :title="t('admin.copy.bfe5b2ed')" :close-label="t('admin.copy.1377553a')" :close-disabled="!!saving" @close="cancelConfirmation">
      <p>
        {{ t('admin.copy.1cf64566') }}
        <AssociationName v-if="confirmation.lane === 'brand'" :record="brandPolicy" kind="brand" :id="confirmation.brandId" :locale="associationLocale" />
        <AssociationName v-else :record="gamePolicy" kind="game" :id="confirmation.gameId" :locale="associationLocale" />
      </p>
      <p v-if="confirmation.body.reason">{{ t('admin.copy.2809a1e6') }}{{ confirmation.body.reason }}</p>
      <BusinessDetails :value="confirmationDetails" :locale="associationLocale" />
      <template #footer>
        <button
          v-write-guard="Boolean(saving)"
          class="button button-primary"
          type="button"
          :disabled="!!saving"
          @click="withGlobalLoading(confirmAndSubmit)"
        >
          {{ saving ? t('admin.copy.46511dc0') : t('admin.finance.savePolicy') }}
        </button>
      </template>
    </DetailModal>
  </section>
</template>

<style scoped>
.policy-header h2 { margin: 0; font-size: 16px; }
.policy-card { display: grid; gap: 12px; min-width: 0; }
.card-heading h3 { margin: 0; font-size: var(--body-font-size); }
.editor-grid, .saved-summary { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; min-width: 0; margin: 0; }
.editor-grid label, .game-picker label { display: grid; gap: 6px; min-width: 0; }
.editor-grid input:not([type="checkbox"]) { width: 100%; min-width: 0; box-sizing: border-box; }
.editor-grid input[type="checkbox"] { justify-self: start; }
.wide { grid-column: 1 / -1; min-width: 0; }
.editor-grid .withdrawal-toggle { display: flex; align-items: center; gap: 8px; }
.withdrawal-toggle input { order: -1; }
.source-field { display: flex; flex-wrap: wrap; gap: 12px; border: 1px solid var(--line); border-radius: var(--radius-sm); padding: 10px; min-width: 0; margin: 0; }
.source-field legend { padding: 0 4px; }
.source-field label { display: flex; align-items: center; gap: 6px; }
.field-error { color: var(--danger); margin: 0; }
.game-picker, .message { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; min-width: 0; }
.game-picker label { flex: 1; }
.saved-summary dt, .muted { color: var(--muted); }
.saved-summary dd { margin: 4px 0 0; font-weight: 600; overflow-wrap: anywhere; }
@media (max-width: 620px) { .editor-grid, .saved-summary { grid-template-columns: minmax(0, 1fr); } }
</style>
