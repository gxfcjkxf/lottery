<script setup lang="ts">
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
  return cause instanceof Error && cause.message ? cause.message : fallback;
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
  Boolean(value.trim()) && new TextEncoder().encode(value.trim()).length <= 500;
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
      throw new LocalizedPolicyError(message("品牌策略响应与当前品牌不匹配。", "The brand policy response does not match the selected brand."));
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
    error.value = explain(cause, message("读取品牌提现策略失败。", "Failed to load the brand withdrawal policy."));
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
      throw new LocalizedPolicyError(message("彩种策略响应与当前品牌或彩种不匹配。", "The game policy response does not match the selected brand or game."));
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
    error.value = explain(cause, message("读取彩种提现策略失败。", "Failed to load the game withdrawal policy."));
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
    error.value = explain(cause, message("读取彩种目录失败。", "Failed to load the game catalog."));
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
      throw new LocalizedPolicyError(message("策略历史响应与当前范围不匹配。", "The policy history response does not match the selected scope."));
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
    error.value = explain(cause, message("读取策略历史失败。", "Failed to load policy history."));
  } finally {
    if (current(ticket, brandId, actorId)) {
      if (lane === "brand" && read === brandHistoryRead)
        loadingBrandHistory.value = false;
      if (lane === "game" && read === gameHistoryRead)
        loadingGameHistory.value = false;
    }
  }
}
function changeSelectedGame(id: string) {
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
    void loadGame(gameId.value, Boolean(uncertain.value));
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
    error.value = message("待提交内容与已核对内容不一致，请重新核对。", "The pending content differs from the reviewed content. Review it again.");
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
      error.value = message("保存响应与当前范围不匹配；写请求已获得响应，请重新读取确认状态。", "The save response does not match the current scope. The write request received a response; reload to confirm the current state.");
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
    notice.value = message("配置已保存。此页面只保存策略，不代表提现功能已上线。", "Settings saved. This page only saves policies; it does not mean withdrawals are available.");
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
        error.value = message("版本冲突：已读取当前配置。请核对新版本后重新发起确认；原请求未自动重放。", "Version conflict: the current settings were loaded. Review the new version and start a new confirmation; the original request was not replayed.");
        reconciling.value = false;
      }
    } else if (outcome === "uncertain") {
      setPendingPolicyWrite(registryKey, mutation);
      uncertain.value = mutation;
      error.value = message("服务器结果尚未确认。相同配置可能已提交；读取到相同值不能证明是本次请求提交。请使用原请求重试（复用原幂等键），或只读核对后仍保留待确认状态。", "The server outcome is unconfirmed. The same settings may have been saved, but reading the same values does not prove this request caused it. Retry the original request with its original idempotency key, or perform a read-only check while keeping the request unconfirmed.");
    } else {
      clearPendingPolicyWrite(registryKey);
      uncertain.value = null;
      error.value = explain(
        cause,
        message("保存提现策略失败；旧请求意图已解除。请修正后重新确认。", "Failed to save the withdrawal policy. The previous request intent was cleared; correct the settings and review them again."),
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
    error.value = message("已重新读取当前版本；由于读取结果不含本次请求凭据，原写入仍待确认，可用同一幂等键重试。", "The current version was reloaded. Because the read does not include proof of this request, the original write remains unconfirmed; retry with the same idempotency key.");
  }
  reconciling.value = false;
}
function historyPrevious(lane: Lane) {
  const offset =
    lane === "brand" ? brandHistoryOffset.value : gameHistoryOffset.value;
  if (offset > 0) void loadHistory(lane, Math.max(0, offset - 50));
}
function historyNext(lane: Lane) {
  const page = lane === "brand" ? brandHistory.value : gameHistory.value;
  const offset =
    lane === "brand" ? brandHistoryOffset.value : gameHistoryOffset.value;
  if (page && page.items.length === 50) void loadHistory(lane, offset + 50);
}
function formatSources(sources: WithdrawalSource[]) {
  return sources
    .map(
      (source) =>
        ({
          recharge: t("充值", "Recharge"),
          winning: t("中奖", "Winnings"),
          gift: t("赠送", "Gift"),
          commission: t("佣金", "Commission"),
        })[source],
    )
    .join(t("、", ", "));
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
        <p class="eyebrow">{{ t("提现策略", "Withdrawal policy") }}</p>
        <h2>{{ t("品牌与彩种提现配置", "Withdrawal policy settings") }}</h2>
        <p>{{ t("本页面仅保存规则；申请、来源占用和流水资格由独立提现流程处理。品牌提现策略默认关闭，未接入真实支付。", "This page only saves rules. Applications, source holds, and turnover qualification are handled by the separate withdrawal workflow. Brand withdrawal policy is disabled by default; real payments are not connected.") }}</p>
      </div>
      <span class="brand-pill">{{ t("品牌", "Brand") }} · {{ brandId || t("未选择", "Not selected") }}</span>
    </header>

    <p class="callout warning" role="note">
      {{ t("已确认提现流水口径：每笔有效投注按投注时 N 折算达标额度（投注额 ÷ N），跨彩种精确累加后与占用前全部可用充值＋赠送余额比较。充值、中奖、赠送及佣金来源投注均计入，每笔只计一次；佣金不计入基础金额。N 必须大于 0，修改仅影响新投注；旧零值历史仍可读，不可新设。平台资格入口与只读预览已接入，品牌仍须明确启用；预览不是授权，申请时会重新检查流水、余额和合规。", "Confirmed turnover rule: divide each valid stake by its bet-time N, sum exact credits across games, and compare with all available recharge plus gift points before reservation. Recharge-, winnings-, gift-, and commission-funded stakes count once per valid bet; commission does not count toward the base amount. N must be positive; changes affect new bets only. Legacy zero history remains readable, but new zero values are prohibited. The platform eligibility checker and read-only preview are connected; the brand must explicitly enable withdrawals. A preview is not authorization: application rechecks turnover, funds, and compliance.") }}
    </p>
    <p v-if="account.super_admin" class="callout">
      {{ t("超级管理员仅可按显式读取权限查看，不能保存配置。", "Super administrators may view settings only when explicitly granted read access; they cannot save settings.") }}
    </p>
    <p v-if="!rights.view" class="callout" role="status">
      {{ t("当前账号没有提现策略读取权限；不会请求策略或历史数据。写权限不会隐含读取权限。", "This account cannot read withdrawal policies. Policy or history data will not be requested. Write permission does not imply read permission.") }}
    </p>

    <section v-if="!rights.view && rights.gameView" class="policy-card">
      <div class="card-heading">
        <div>
          <h3>{{ t("彩种目录", "Game catalog") }}</h3>
          <p>{{ t("目录读取权限独立于策略读取权限。", "Catalog read permission is separate from policy read permission.") }}</p>
        </div>
        <button
          class="secondary"
          type="button"
          :disabled="loadingGames"
          @click="loadGames"
        >
          {{ loadingGames ? t("读取中…", "Loading…") : t("读取彩种目录", "Load game catalog") }}
        </button>
      </div>
      <p v-if="loadingGames" class="muted" role="status">{{ t("正在读取彩种目录…", "Loading game catalog…") }}</p>
      <p v-else-if="!games.length" class="muted">{{ t("彩种目录为空，或尚未读取。", "The game catalog is empty or has not been loaded.") }}</p>
      <ul v-else class="catalog-list">
        <li v-for="game in games" :key="game.id">
          {{ game.name }} · {{ game.id }}
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
          >{{ t("此范围有写入结果待确认。配置已冻结；相同内容不代表已证明本次请求成功。待确认意图仅保存在当前页面会话内。", "A write in this scope has an unconfirmed outcome. The settings are frozen; matching values do not prove this request succeeded. The pending intent is held only for the current page session.") }}</span
        >
        <button
          class="primary"
          type="button"
          :disabled="!!saving || !canWrite"
          @click="retryUncertain"
        >
          {{ t("使用原请求重试", "Retry original request") }}
        </button>
        <button
          class="secondary"
          type="button"
          :disabled="!!saving || reconciling"
          @click="reconcileUncertain"
        >
          {{ t("只读核对", "Read-only check") }}
        </button>
      </div>
      <section class="policy-card">
        <div class="card-heading">
          <div>
            <h3>{{ t("品牌默认配置", "Brand default settings") }}</h3>
            <p v-if="brandPolicy">
              {{ t("已保存版本", "Saved version") }} {{ brandPolicy.version }} ·
              {{ brandPolicy.brand_id }} · {{ t("更新于", "Updated") }} {{ brandPolicy.updated_at }}
            </p>
          </div>
          <button
            class="secondary"
            type="button"
            :disabled="loadingBrand || !!saving || !!pendingInContext"
            @click="() => loadBrand()"
          >
            {{ loadingBrand ? t("读取中…", "Loading…") : t("刷新品牌配置", "Refresh brand settings") }}
          </button>
        </div>
        <p v-if="loadingBrand && !brandPolicy" class="muted" role="status">
          {{ t("正在读取品牌配置…", "Loading brand settings…") }}
        </p>
        <p v-else-if="!brandPolicy && !error" class="muted">
          {{ t("尚无可显示的已保存品牌配置。", "There are no saved brand settings to display.") }}
        </p>
        <template v-if="brandPolicy">
          <dl class="saved-summary">
            <div>
              <dt>{{ t("状态", "Status") }}</dt>
              <dd>
                {{ brandPolicy.config.enabled ? t("启用配置", "Enabled") : t("停用配置", "Disabled") }}
              </dd>
            </div>
            <div>
              <dt>{{ t("提现积分范围", "Withdrawal points range") }}</dt>
              <dd>
                {{ brandPolicy.config.min_points }} {{ t("至", "to") }}
                {{ brandPolicy.config.max_points ?? t("无上限", "No limit") }}
              </dd>
            </div>
            <div>
              <dt>{{ t("允许来源", "Allowed sources") }}</dt>
              <dd>{{ formatSources(brandPolicy.config.allowed_sources) }}</dd>
            </div>
            <div>
              <dt>{{ t("审核模式", "Review mode") }}</dt>
              <dd>
                {{
                  brandPolicy.config.review_mode === "manual"
                    ? t("人工审核", "Manual review")
                    : t("自动审核", "Automatic review")
                }}
              </dd>
            </div>
            <div>
              <dt>{{ t("已保存流水倍数 N", "Saved turnover multiple N") }}</dt>
              <dd>{{ brandPolicy.config.turnover_multiple }}</dd>
            </div>
          </dl>
          <form class="editor-grid" @submit.prevent="requestBrandConfirmation">
            <label for="withdraw-brand-enabled"
              >{{ t("品牌：启用提现配置", "Brand: enable withdrawal settings") }}<input
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
              >{{ t("品牌：最低提现积分", "Brand: minimum withdrawal points") }}<input
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
              >{{ t("品牌：最高提现积分（留空表示无上限）", "Brand: maximum withdrawal points (leave blank for no limit)") }}<input
                id="withdraw-brand-max"
                v-model.trim="brandDraft.max_points_input"
                inputmode="numeric"
                autocomplete="off"
                :placeholder="t('无上限', 'No limit')"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
            /></label>
            <fieldset class="source-field wide">
              <legend>{{ t("品牌：允许提现来源（至少选择一项）", "Brand: allowed withdrawal sources (select at least one)") }}</legend>
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
                />{{ t("充值", "Recharge") }}</label
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
                />{{ t("中奖", "Winnings") }}</label
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
                />{{ t("赠送", "Gift") }}</label
              >
              <label for="withdraw-source-commission"
                ><input
                  id="withdraw-source-commission"
                  v-model="brandDraft.allowed_sources"
                  type="checkbox"
                  value="commission"
                  :disabled="!canWrite || !!pendingInContext || !!confirmation || reconciling"
                />{{ t("佣金", "Commission") }}</label
              >
            </fieldset>
            <label for="withdraw-brand-review"
              >{{ t("品牌：审核方式", "Brand: review mode") }}<select
                id="withdraw-brand-review"
                v-model="brandDraft.review_mode"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
              >
                <option value="manual">{{ t("人工审核", "Manual review") }}</option>
                <option value="automatic">{{ t("自动审核", "Automatic review") }}</option>
              </select></label
            >
            <label for="withdraw-brand-multiple"
              >{{ t("品牌：默认流水倍数 N", "Brand: default turnover multiple N") }}<input
                id="withdraw-brand-multiple"
                v-model.trim="brandDraft.turnover_multiple"
                inputmode="decimal"
                autocomplete="off"
                :placeholder="t('例如 1.25', 'For example, 1.25')"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
            /></label>
            <label class="wide" for="withdraw-brand-reason"
              >{{ t("品牌：变更原因（必填，最多 500 UTF-8 字节）", "Brand: reason for change (required, up to 500 UTF-8 bytes)") }}<textarea
                id="withdraw-brand-reason"
                v-model="brandDraft.reason"
                rows="3"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
              />
            </label>
            <p v-if="!brandInputsValid" class="field-error wide" role="alert">
              {{ t("积分范围须为正的 int64 整数字符串且最低值不高于最高值；来源至少一项且不能重复；N 须大于 0 且不超过 1,000,000，最多 6 位小数并使用规范十进制格式。", "Points must be positive int64 integer strings, with the minimum no greater than the maximum. Select at least one unique source. N must be greater than 0 and at most 1,000,000, with at most 6 decimal places in canonical decimal format.") }}
            </p>
            <p
              v-if="brandDraft.reason && !reasonValid(brandDraft.reason)"
              class="field-error wide"
              role="alert"
            >
              {{ t("原因须非空且最多 500 UTF-8 字节。", "Enter a reason of up to 500 UTF-8 bytes.") }}
            </p>
            <p v-if="!canWrite" class="muted wide">
              {{ t("当前账号仅可查看品牌配置。", "This account can only view brand settings.") }}
            </p>
            <div v-if="canWrite" class="wide action-row">
              <button class="primary" type="submit" :disabled="!brandCanSave">
                {{ t("核对品牌配置并继续", "Review brand settings and continue") }}
              </button>
            </div>
          </form>
        </template>
      </section>

      <section class="policy-card">
        <div class="card-heading">
          <div>
            <h3>{{ t("彩种流水倍数覆盖", "Game turnover multiple override") }}</h3>
            <p>{{ t("彩种仅可指定 N 或留空继承；有效值与来源来自服务端记录。", "A game can specify N or leave it blank to inherit. The effective value and source come from the server record.") }}</p>
          </div>
        </div>
        <div class="game-picker">
          <label v-if="rights.gameView" for="withdraw-game-select"
            >{{ t("选择彩种", "Select a game") }}<select
              id="withdraw-game-select"
          :aria-label="t('选择彩种', 'Select game')"
              :value="gameId"
              :disabled="
                loadingGames || !!saving || !!pendingInContext || !!confirmation
              "
              @change="
                changeSelectedGame(($event.target as HTMLSelectElement).value)
              "
            >
              <option value="">{{ t("请选择彩种", "Select a game") }}</option>
              <option
                v-if="
                  gamePolicy &&
                  gameId &&
                  !games.some((game) => game.id === gameId)
                "
                :value="gameId"
              >
                {{ t("直接查询", "Direct lookup") }} · {{ gameId }}
              </option>
              <option v-for="game in games" :key="game.id" :value="game.id">
                {{ game.name }} · {{ game.id }}
              </option>
            </select></label
          >
          <button
            v-if="rights.gameView"
            class="secondary"
            type="button"
            :disabled="loadingGames || !!saving"
            @click="loadGames"
          >
            {{ loadingGames ? t("读取目录中…", "Loading catalog…") : t("刷新彩种目录", "Refresh game catalog") }}
          </button>
          <span
            v-if="rights.gameView && !games.length && !loadingGames"
            class="muted"
            >{{ t("彩种目录为空，或尚未读取。", "The game catalog is empty or has not been loaded.") }}</span
          >
          <span v-if="!rights.gameView" class="muted"
            >{{ t("无彩种目录读取权限；可通过 ID 直接查询，不会因此读取目录。", "No permission to read the game catalog. You can look up a game by ID without loading the catalog.") }}</span
          >
        </div>
        <form
          class="game-id-form"
          @submit.prevent="changeSelectedGame(directGameId)"
        >
          <label for="withdraw-game-id"
            >{{ t("彩种：直接输入 ID", "Game: enter ID directly") }}<input
              id="withdraw-game-id"
              v-model.trim="directGameId"
              autocomplete="off"
              :placeholder="t('彩种 ID，可输入 UUID', 'Game ID; UUID accepted')"
              :disabled="!!saving || !!pendingInContext || !!confirmation"
          /></label>
          <button
            class="secondary"
            type="submit"
            :disabled="
              !directGameId.trim() ||
              loadingGame ||
              !!saving ||
              !!pendingInContext ||
              !!confirmation
            "
          >
            {{ loadingGame ? t("读取中…", "Loading…") : t("读取此彩种策略", "Load this game policy") }}
          </button>
        </form>
        <p v-if="loadingGame && !gamePolicy" class="muted" role="status">
          {{ t("正在读取彩种策略…", "Loading game policy…") }}
        </p>
        <p v-else-if="gameId && !gamePolicy && !error" class="muted">
          {{ t("尚无可显示的已保存彩种策略。", "There are no saved game settings to display.") }}
        </p>
        <template v-if="gamePolicy">
          <dl class="saved-summary">
            <div>
              <dt>{{ t("已保存彩种", "Saved game") }}</dt>
              <dd>{{ gamePolicy.game_id }}</dd>
            </div>
            <div>
              <dt>{{ t("彩种配置版本", "Game settings version") }}</dt>
              <dd>{{ gamePolicy.version }}</dd>
            </div>
            <div>
              <dt>{{ t("已保存有效 N", "Saved effective N") }}</dt>
              <dd>{{ gamePolicy.effective.turnover_multiple }}</dd>
            </div>
            <div>
              <dt>{{ t("有效值来源", "Effective value source") }}</dt>
              <dd>
                {{
                  gamePolicy.effective.source === "brand"
                    ? t("品牌默认", "Brand default")
                    : t("彩种覆盖", "Game override")
                }}
              </dd>
            </div>
            <div>
              <dt>{{ t("品牌配置版本", "Brand settings version") }}</dt>
              <dd>{{ gamePolicy.effective.brand_version }}</dd>
            </div>
            <div>
              <dt>{{ t("彩种配置版本", "Game settings version") }}</dt>
              <dd>{{ gamePolicy.effective.game_version }}</dd>
            </div>
          </dl>
          <form class="editor-grid" @submit.prevent="requestGameConfirmation">
            <label class="wide" for="withdraw-game-multiple"
              >{{ t("彩种：流水倍数 N（必须大于 0；留空继承品牌默认）", "Game: turnover multiple N (must be greater than 0; leave blank to inherit brand default)") }}<input
                id="withdraw-game-multiple"
                v-model.trim="gameDraft.turnover_multiple"
                inputmode="decimal"
                autocomplete="off"
                :placeholder="t('继承品牌默认', 'Inherit brand default')"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
            /></label>
            <p v-if="!gameInputValid" class="field-error wide" role="alert">
              {{ t("N 须大于 0 且不超过 1,000,000，最多 6 位小数并使用规范十进制格式；留空表示继承。", "N must be greater than 0 and at most 1,000,000, with at most 6 decimal places in canonical decimal format. Leave blank to inherit.") }}
            </p>
            <label class="wide" for="withdraw-game-reason"
              >{{ t("彩种：变更原因（必填，最多 500 UTF-8 字节）", "Game: reason for change (required, up to 500 UTF-8 bytes)") }}<textarea
                id="withdraw-game-reason"
                v-model="gameDraft.reason"
                rows="3"
                :disabled="
                  !canWrite ||
                  !!pendingInContext ||
                  !!confirmation ||
                  reconciling
                "
              />
            </label>
            <p v-if="!canWrite" class="muted wide">
              {{ t("当前账号仅可查看彩种配置。", "This account can only view game settings.") }}
            </p>
            <div v-if="canWrite" class="wide action-row">
              <button class="primary" type="submit" :disabled="!gameCanSave">
                {{ t("核对彩种配置并继续", "Review game settings and continue") }}
              </button>
            </div>
          </form>
          <section class="history-panel">
            <div class="card-heading">
              <div>
              <h4>{{ t("彩种不可变历史", "Immutable game history") }}</h4>
                <p>{{ t("仅查看本彩种版本记录。", "Showing revisions for this game only.") }}</p>
              </div>
              <button
                class="secondary"
                type="button"
                :disabled="loadingGameHistory"
                @click="loadHistory('game', 0)"
              >
                {{ loadingGameHistory ? t("读取中…", "Loading…") : t("读取彩种历史", "Load game history") }}
              </button>
            </div>
            <p
              v-if="loadingGameHistory && !gameHistory"
              class="muted"
              role="status"
            >
              {{ t("正在读取彩种历史…", "Loading game history…") }}
            </p>
            <p
              v-else-if="gameHistory && !gameHistory.items.length"
              class="muted"
            >
              {{ t("此页没有历史记录。", "There are no history entries on this page.") }}
            </p>
            <ol v-if="gameHistory?.items.length" class="history-list">
              <li v-for="item in gameHistory.items" :key="item.id">
                <h5>{{ t("版本", "Version") }} {{ item.version }} · {{ item.created_at }}</h5>
                <p>
                  {{ t("操作人：", "Changed by: ") }}{{ item.changed_by || t("系统", "System") }} · {{ t("原因：", "Reason: ") }}{{
                    item.reason
                  }}
                </p>
                <pre>{{ configJson(item.config) }}</pre>
              </li>
            </ol>
            <div v-if="gameHistory" class="history-pagination">
              <button
                class="secondary"
                type="button"
                :disabled="gameHistoryOffset === 0 || loadingGameHistory"
                @click="historyPrevious('game')"
              >
                {{ t("上一页", "Previous") }}</button
              ><span
                >{{ t("偏移", "Offset") }} {{ gameHistoryOffset }} · {{ t("每页", "per page") }}
                {{ gameHistory.limit }} {{ t("条", "items") }}</span
              ><button
                class="secondary"
                type="button"
                :disabled="gameHistory.items.length < 50 || loadingGameHistory"
                @click="historyNext('game')"
              >
                {{ t("下一页", "Next") }}
              </button>
            </div>
          </section>
        </template>
      </section>

      <section class="policy-card history-panel">
        <div class="card-heading">
          <div>
            <h3>{{ t("品牌不可变历史", "Immutable brand history") }}</h3>
            <p>{{ t("仅查看当前品牌的版本记录。", "Showing revisions for the current brand only.") }}</p>
          </div>
          <button
            class="secondary"
            type="button"
            :disabled="loadingBrandHistory"
            @click="loadHistory('brand', 0)"
          >
            {{ loadingBrandHistory ? t("读取中…", "Loading…") : t("读取品牌历史", "Load brand history") }}
          </button>
        </div>
        <p
          v-if="loadingBrandHistory && !brandHistory"
          class="muted"
          role="status"
        >
          {{ t("正在读取品牌历史…", "Loading brand history…") }}
        </p>
        <p v-else-if="brandHistory && !brandHistory.items.length" class="muted">
          {{ t("此页没有历史记录。", "There are no history entries on this page.") }}
        </p>
        <ol v-if="brandHistory?.items.length" class="history-list">
          <li v-for="item in brandHistory.items" :key="item.id">
            <h5>{{ t("版本", "Version") }} {{ item.version }} · {{ item.created_at }}</h5>
            <p>
              {{ t("操作人：", "Changed by: ") }}{{ item.changed_by || t("系统", "System") }} · {{ t("原因：", "Reason: ") }}{{ item.reason }}
            </p>
            <pre>{{ configJson(item.config) }}</pre>
          </li>
        </ol>
        <div v-if="brandHistory" class="history-pagination">
          <button
            class="secondary"
            type="button"
            :disabled="brandHistoryOffset === 0 || loadingBrandHistory"
            @click="historyPrevious('brand')"
          >
            {{ t("上一页", "Previous") }}</button
          ><span
            >{{ t("偏移", "Offset") }} {{ brandHistoryOffset }} · {{ t("每页", "per page") }}
            {{ brandHistory.limit }} {{ t("条", "items") }}</span
          ><button
            class="secondary"
            type="button"
            :disabled="brandHistory.items.length < 50 || loadingBrandHistory"
            @click="historyNext('brand')"
          >
            {{ t("下一页", "Next") }}
          </button>
        </div>
      </section>
    </template>

    <section
      v-if="confirmation"
      class="confirmation-panel"
      aria-labelledby="withdraw-confirm-title"
    >
      <h3 id="withdraw-confirm-title">{{ t("请核对将要保存的配置", "Review the settings to be saved") }}</h3>
      <p>
        {{ t("范围：", "Scope: ") }}{{
          confirmation.lane === "brand"
            ? `${t("品牌", "Brand")} ${confirmation.brandId}`
            : `${t("彩种", "Game")} ${confirmation.gameId}`
        }}
        · {{ t("版本", "Version") }} {{ confirmation.body.version }}
      </p>
      <p>{{ t("原因：", "Reason: ") }}{{ confirmation.body.reason }}</p>
      <pre>{{ configJson(confirmation.body.config) }}</pre>
      <p class="muted">
        {{ t("幂等键：", "Idempotency key: ") }}<code>{{ confirmation.key }}</code>
      </p>
      <div class="action-row">
        <button class="secondary" type="button" @click="cancelConfirmation">
          {{ t("返回编辑", "Back to editing") }}</button
        ><button
          class="primary"
          type="button"
          :disabled="!!saving"
          @click="confirmAndSubmit"
        >
          {{ saving ? t("提交中…", "Submitting…") : t("确认并提交冻结请求", "Confirm and submit frozen request") }}
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
  border: 1px solid #e9ebf0;
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
.primary,
.secondary {
  border: 0;
  border-radius: 7px;
  padding: 9px 12px;
  font: inherit;
  font-size: 11px;
  cursor: pointer;
  white-space: normal;
  overflow-wrap: anywhere;
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
  font-size: 10px;
  color: #737b8c;
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
  border-bottom: 1px solid #eceef3;
}
.history-panel {
  margin-top: 15px;
}
.history-list {
  padding-left: 20px;
}
.history-list li {
  border-top: 1px solid #eceef3;
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
  background: #f7f8fb;
  border-radius: 7px;
  padding: 10px;
  font-size: 10px;
  max-width: 100%;
  overflow: auto;
}
.history-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 10px;
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
