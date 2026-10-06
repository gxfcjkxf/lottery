<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import {
  createBettingApi,
  createBetKey,
  snapshotBetIntent,
  type BetInput,
  type BetOrder,
  type BetQuote,
  type CatalogGame,
  type CatalogPlay,
  type PolicyVersions,
} from "./betting-api";
import type {
  RuleDefinition,
  RuleTicketSelection,
} from "../../shared/src/rules";
import type { Language } from "../../shared/src/brand";
import { createAuthClient, type AuthProfile } from "../../shared/src/auth";
import BetSelection from "./BetSelection.vue";
import SelectionSummary from "./SelectionSummary.vue";
import { createWalletApi } from "./wallet-api";

const props = defineProps<{
  locale: Language;
  brandCode?: string;
  auth?: AuthProfile | null;
  embeddedCatalog?: boolean;
}>();
const emit = defineEmits<{ "auth-expired": [] }>();
const route = useRoute();
const router = useRouter();
const api = createBettingApi({ brandCode: props.brandCode });
const authApi = createAuthClient({ brandCode: props.brandCode });
const zh = computed(() => props.locale === "zh");
const catalog = ref<CatalogGame[]>([]);
const catalogLoading = ref(false);
const catalogError = ref("");
const catalogGeneration = ref(0);
const detailLoading = ref(false);
const detailError = ref("");
const currentGame = ref<CatalogGame | null>(null);
const plays = ref<CatalogPlay[]>([]);
const period = ref<import("./betting-api").Period | null>(null);
const policy = ref<Record<string, any> | null>(null);
const brandStatus = ref("");
const policyVersions = ref<PolicyVersions | undefined>();
const serverOffset = ref(0);
const now = ref(Date.now());
const chosenPlayId = ref("");
const selection = ref<RuleTicketSelection>(emptySelection());
const multiplier = ref("1");
const quote = ref<BetQuote | null>(null);
const quoteLoading = ref(false);
const quoteError = ref("");
const review = ref<{
  body: Readonly<BetInput>;
  quote: BetQuote;
  key: string;
  actor: AuthIdentity;
  definition: RuleDefinition;
} | null>(null);
const placing = ref(false);
const placedOrder = ref<BetOrder | null>(null);
const availablePoints = ref<string | null>(null);
const memberStatus = ref(props.auth?.member.status ?? "");
const authRequired = ref(false);
const orders = ref<BetOrder[]>([]);
const ordersLoading = ref(false);
const orderError = ref("");
const cancelReason = ref("");
const cancelBusy = ref(false);
const cancelReasonBytesValid = computed(
  () =>
    new TextEncoder().encode(cancelReason.value.trim()).length > 0 &&
    new TextEncoder().encode(cancelReason.value.trim()).length <= 500,
);
const cancelIntent = ref<{
  orderId: string;
  body: { version: number; reason: string };
  key: string;
} | null>(null);
const uncertainPlace = ref(false);
const previewGeneration = ref(0);
const previewBody = ref<BetInput | null>(null);
type AuthIdentity = { userId: string; memberId: string; brandId: string };
const previewActor = ref<AuthIdentity | null>(null);
const notice = ref("");
const fetchGeneration = ref(0);
const orderGeneration = ref(0);
const detailOrderGeneration = ref(0);
const loadedGameFingerprint = ref("");
let pollTimer: ReturnType<typeof setInterval> | undefined;
let clockTimer: ReturnType<typeof setInterval> | undefined;

function emptySelection(): RuleTicketSelection {
  return {
    regular: [],
    special: [],
    digits: [],
    exclude: [],
    attributes: {},
    features: {},
  };
}
function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}
function freezeDeep<T>(value: T): T {
  if (value && typeof value === "object" && !Object.isFrozen(value)) {
    Object.freeze(value);
    for (const child of Object.values(value as Record<string, unknown>))
      freezeDeep(child);
  }
  return value;
}
function message(error: unknown, protectedRequest = false): string {
  const raw = error as { status?: number; code?: string; message?: string };
  if (raw?.status === 401 && protectedRequest) {
    authRequired.value = true;
    emit("auth-expired");
    return zh.value
      ? "登录已过期，请重新登录。"
      : "Your session expired. Please sign in again.";
  }
  if (
    raw?.status === 409 ||
    /CONFLICT|CLOSED|VERSION|POLICY|CUTOFF|RULE/i.test(raw?.code ?? "")
  ) {
    invalidateReview();
    return zh.value
      ? "期次、规则或投注政策已变化。请刷新信息并重新获取报价。"
      : "The period, rules, or betting policy changed. Refresh and request a new quote.";
  }
  if (raw?.status === 0 && raw.code !== "invalid_multiplier")
    return zh.value
      ? "网络结果不确定。请重试同一操作；系统会保留请求编号。"
      : "The network result is uncertain. Retry the same action; its request key is preserved.";
  return (
    raw?.message ||
    (zh.value
      ? "服务暂时不可用，请稍后重试。"
      : "The service is temporarily unavailable. Please try again.")
  );
}
function invalidateReview() {
  previewGeneration.value++;
  previewBody.value = null;
  previewActor.value = null;
  review.value = null;
  quote.value = null;
  uncertainPlace.value = false;
  quoteLoading.value = false;
}
function getItems(result: any): CatalogGame[] {
  return Array.isArray(result)
    ? result
    : Array.isArray(result?.items)
      ? result.items
      : [];
}
function exactAmount(value: unknown): string {
  return typeof value === "string"
    ? value
    : value == null
      ? "0"
      : String(value);
}
const gameId = computed(() => String(route.params.gameId ?? ""));
const selectedPlay = computed(
  () =>
    plays.value.find((item) => item.id === chosenPlayId.value) ??
    plays.value[0] ??
    null,
);
const definition = computed(() => selectedPlay.value?.definition ?? null);
const memberFrozen = computed(
  () =>
    ["frozen", "disabled"].includes(memberStatus.value) ||
    ["frozen", "disabled"].includes(props.auth?.user.status ?? ""),
);
const memberAuthorized = computed(
  () => memberStatus.value === "normal" && !memberFrozen.value,
);
const isPaused = computed(() =>
  Boolean(
    now.value &&
      (!currentGame.value ||
        currentGame.value.status !== "active" ||
        brandStatus.value !== "active" ||
        memberFrozen.value ||
        !period.value ||
        period.value.status !== "betting" ||
        Date.now() + serverOffset.value >=
          Date.parse(period.value?.bet_end_at ?? "")),
  ),
);
const canBet = computed(() =>
  Boolean(
    period.value &&
      selectedPlay.value &&
      !isPaused.value &&
      memberAuthorized.value &&
      multiplier.value.length > 0 &&
      !quoteLoading.value &&
      !uncertainPlace.value,
  ),
);
const reviewExpired = computed(() => {
  now.value;
  return Boolean(
    review.value &&
      Date.now() + serverOffset.value >=
        Date.parse(review.value.quote.period.bet_end_at),
  );
});
const cutoffText = computed(() => {
  now.value;
  if (!period.value?.bet_end_at) return "—";
  const left = Math.max(
    0,
    Date.parse(period.value.bet_end_at) - (Date.now() + serverOffset.value),
  );
  const seconds = Math.floor(left / 1000);
  return `${String(Math.floor(seconds / 3600)).padStart(2, "0")}:${String(Math.floor((seconds % 3600) / 60)).padStart(2, "0")}:${String(seconds % 60).padStart(2, "0")}`;
});
const rulesTiers = computed(() => definition.value?.prize_tiers ?? []);
const placedOrderRouteId = computed(() => String(route.params.id ?? ""));
const confirmationGameId = computed(
  () =>
    review.value?.quote.period.game_id ??
    currentGame.value?.id ??
    placedOrder.value?.game_id ??
    "",
);

async function loadCatalog() {
  const generation = ++catalogGeneration.value;
  catalogLoading.value = catalog.value.length === 0;
  try {
    const result = await api.games();
    if (generation !== catalogGeneration.value) return;
    catalog.value = getItems(result);
    catalogError.value = "";
  } catch (error) {
    if (generation === catalogGeneration.value)
      catalogError.value = message(error);
  } finally {
    if (generation === catalogGeneration.value) catalogLoading.value = false;
  }
}
async function loadGame(id: string, passive = false) {
  const generation = ++fetchGeneration.value;
  if (!passive) detailLoading.value = true;
  detailError.value = "";
  try {
    const result = await api.game(id);
    if (generation !== fetchGeneration.value || id !== gameId.value) return;
    currentGame.value = result.game;
    plays.value = Array.isArray(result.plays) ? result.plays : [];
    period.value = result.period ?? null;
    policy.value = result.policy ?? null;
    policyVersions.value = result.policy_versions;
    const nextFingerprint = JSON.stringify({
      game: result.game,
      plays: result.plays,
      period: result.period,
      policy: result.policy,
      versions: result.policy_versions,
      brand: result.brand_status,
    });
    const changed = Boolean(
      loadedGameFingerprint.value &&
        loadedGameFingerprint.value !== nextFingerprint,
    );
    brandStatus.value = result.brand_status;
    if (changed && !uncertainPlace.value) {
      invalidateReview();
      selection.value = emptySelection();
      multiplier.value = "1";
    }
    loadedGameFingerprint.value = nextFingerprint;
    const received = Date.parse(result.server_time ?? "");
    if (Number.isFinite(received)) serverOffset.value = received - Date.now();
    if (
      !uncertainPlace.value &&
      !plays.value.some((item) => item.id === chosenPlayId.value)
    ) {
      const requestedPlay = String(route.query.play ?? "");
      chosenPlayId.value =
        plays.value.find((p) => p.id === requestedPlay)?.id ??
        plays.value[0]?.id ??
        "";
    }
    if (!selectedPlay.value) selection.value = emptySelection();
  } catch (error) {
    if (generation === fetchGeneration.value)
      detailError.value = message(error);
  } finally {
    if (generation === fetchGeneration.value) detailLoading.value = false;
  }
}
function onIntentChange() {
  if (uncertainPlace.value) return;
  invalidateReview();
  quoteError.value = "";
}
watch([selection, multiplier, chosenPlayId], onIntentChange, {
  deep: true,
  flush: "sync",
});
watch(
  () => route.path,
  () => {
    notice.value = "";
  },
);
watch(gameId, (id, previous) => {
  if (uncertainPlace.value) {
    if (id) void loadGame(id);
    return;
  }
  if (id && id !== previous) {
    selection.value = emptySelection();
    multiplier.value = "1";
    chosenPlayId.value = "";
    loadedGameFingerprint.value = "";
    invalidateReview();
    void loadGame(id);
  }
});
async function refreshMember() {
  if (props.auth) {
    memberStatus.value = props.auth.member.status;
    return;
  }
  try {
    const profile = await authApi.me();
    memberStatus.value = profile.member.status;
    authRequired.value = false;
  } catch (error) {
    if ((error as { status?: number })?.status === 401) {
      authRequired.value = true;
      emit("auth-expired");
    }
  }
}

async function requestQuote() {
  if (uncertainPlace.value) return;
  quoteError.value = "";
  if (!selectedPlay.value || !period.value) {
    quoteError.value = zh.value
      ? "当前没有开放期次。"
      : "There is no open period.";
    return;
  }
  if (!/^[1-9][0-9]*$/.test(multiplier.value)) {
    quoteError.value = zh.value
      ? "请输入正整数倍数。"
      : "Enter a positive whole-number multiplier.";
    return;
  }
  invalidateReview();
  quoteLoading.value = true;
  const generation = ++previewGeneration.value;
  let profile: AuthProfile;
  try {
    profile = await authApi.me();
    if (generation !== previewGeneration.value) return;
    memberStatus.value = profile.member.status;
    authRequired.value = false;
    previewActor.value = {
      userId: profile.user.id,
      memberId: profile.member.id,
      brandId: profile.member.brand_id,
    };
    if (
      profile.member.status !== "normal" ||
      profile.user.status !== "active"
    ) {
      quoteError.value = zh.value
        ? "当前账户状态不允许投注。"
        : "This account is not eligible to place bets.";
      quoteLoading.value = false;
      return;
    }
  } catch (error) {
    if (generation !== previewGeneration.value) return;
    authRequired.value = (error as { status?: number })?.status === 401;
    if (authRequired.value) emit("auth-expired");
    quoteError.value = message(error, true);
    quoteLoading.value = false;
    return;
  }
  const body: BetInput = {
    period_id: String(period.value.id),
    play_id: selectedPlay.value.id,
    rule_version_id: selectedPlay.value.rule_version_id,
    selection: clone(selection.value),
    multiplier: multiplier.value,
    ...(policyVersions.value
      ? { policy_versions: clone(policyVersions.value) }
      : {}),
  };
  if (generation !== previewGeneration.value) return;
  previewBody.value = clone(body);
  try {
    const result = await api.preview(body);
    if (generation !== previewGeneration.value) return;
    const currentProfile = await authApi.me();
    if (generation !== previewGeneration.value) return;
    if (
      currentProfile.user.id !== previewActor.value?.userId ||
      currentProfile.member.id !== previewActor.value?.memberId ||
      currentProfile.member.brand_id !== previewActor.value?.brandId
    ) {
      invalidateReview();
      quoteError.value = zh.value
        ? "登录账户已变化，请重新获取报价。"
        : "The signed-in account changed. Request a new quote.";
      return;
    }
    quote.value = result;
  } catch (error) {
    if (generation === previewGeneration.value)
      quoteError.value = message(error, true);
  } finally {
    if (generation === previewGeneration.value) quoteLoading.value = false;
  }
}
function beginReview() {
  if (
    !quote.value ||
    !previewBody.value ||
    !previewActor.value ||
    uncertainPlace.value
  )
    return;
  const snapshot = snapshotBetIntent(previewBody.value, quote.value);
  review.value = {
    body: freezeDeep(snapshot.body),
    quote: freezeDeep(clone(quote.value)),
    key: snapshot.key,
    actor: Object.freeze({ ...previewActor.value }),
    definition: freezeDeep(clone(definition.value!)),
  };
  void router.push("/bet/confirm");
}
async function placeOrder() {
  const current = review.value;
  if (!current || placing.value) return;
  placing.value = true;
  quoteError.value = "";
  try {
    const profile = await authApi.me();
    if (
      profile.user.id !== current.actor.userId ||
      profile.member.id !== current.actor.memberId ||
      profile.member.brand_id !== current.actor.brandId
    ) {
      if (!uncertainPlace.value) invalidateReview();
      quoteError.value = zh.value
        ? "当前登录账户已变化。请用原账户继续，或重新获取报价。"
        : "The signed-in account changed. Continue with the original account or request a fresh quote.";
      return;
    }
    if (
      !uncertainPlace.value &&
      (profile.member.status !== "normal" || profile.user.status !== "active")
    ) {
      quoteError.value = zh.value
        ? "当前账户状态不允许投注。"
        : "This account is not eligible to place bets.";
      return;
    }
    const result = await api.place(
      clone(current.body) as BetInput,
      current.key,
    );
    placedOrder.value = result;
    invalidateReview();
    await Promise.allSettled([loadOrders(), refreshWalletEvent()]);
    void router.push(`/orders/${encodeURIComponent(result.id)}`);
  } catch (error) {
    const status = (error as { status?: number })?.status;
    if (status === 0 || (status != null && status >= 500))
      uncertainPlace.value = true;
    else if (status === 401) {
      authRequired.value = true;
      invalidateReview();
    } else if (status === 409) invalidateReview();
    quoteError.value = message(error, true);
  } finally {
    placing.value = false;
  }
}
async function refreshWalletEvent() {
  try {
    const wallet = await createWalletApi({
      brandCode: props.brandCode,
    }).wallet();
    availablePoints.value = wallet.available_points;
    window.dispatchEvent(new CustomEvent("wallet:refresh", { detail: wallet }));
  } catch (error) {
    if ((error as { status?: number })?.status === 401) {
      authRequired.value = true;
      emit("auth-expired");
    }
  }
}

async function loadOrders() {
  const generation = ++orderGeneration.value;
  ordersLoading.value = true;
  orderError.value = "";
  try {
    const result: any = await api.orders();
    if (generation === orderGeneration.value)
      orders.value = Array.isArray(result)
        ? result
        : Array.isArray(result?.items)
          ? result.items
          : [];
  } catch (error) {
    if (generation === orderGeneration.value) {
      orders.value = [];
      orderError.value = message(error, true);
      authRequired.value = (error as { status?: number })?.status === 401;
    }
  } finally {
    if (generation === orderGeneration.value) ordersLoading.value = false;
  }
}
async function loadOrder(id: string) {
  const generation = ++detailOrderGeneration.value;
  orderError.value = "";
  try {
    const result = await api.order(id);
    if (
      generation !== detailOrderGeneration.value ||
      id !== placedOrderRouteId.value
    )
      return;
    const changed =
      placedOrder.value?.id !== result.id ||
      placedOrder.value.version !== result.version;
    placedOrder.value = result;
    if (
      result.status !== "placed" &&
      cancelIntent.value?.orderId === result.id
    ) {
      cancelIntent.value = null;
      cancelReason.value = "";
    }
    if (changed || availablePoints.value === null) await refreshWalletEvent();
  } catch (error) {
    if (generation === detailOrderGeneration.value) {
      placedOrder.value = null;
      availablePoints.value = null;
      orderError.value = message(error, true);
      authRequired.value = (error as { status?: number })?.status === 401;
    }
  }
}
async function cancelOrder(order: BetOrder) {
  if (cancelBusy.value || !cancelReasonBytesValid.value) return;
  cancelBusy.value = true;
  orderError.value = "";
  if (!cancelIntent.value) {
    cancelIntent.value = {
      orderId: order.id,
      body: { version: order.version, reason: cancelReason.value.trim() },
      key: createBetKey("cancel"),
    };
  }
  try {
    if (cancelIntent.value.orderId !== order.id) return;
    placedOrder.value = await api.cancel(
      order.id,
      cancelIntent.value.body,
      cancelIntent.value.key,
    );
    cancelReason.value = "";
    cancelIntent.value = null;
    await Promise.allSettled([
      loadOrder(order.id),
      loadOrders(),
      refreshWalletEvent(),
    ]);
  } catch (error) {
    const status = (error as { status?: number })?.status;
    if (status === 409 || status === 400) cancelIntent.value = null;
    orderError.value = message(error, true);
  } finally {
    cancelBusy.value = false;
  }
}
function openBet(game: CatalogGame, play?: CatalogPlay) {
  void router.push({
    path: `/games/${encodeURIComponent(game.id)}/bet`,
    query: play ? { play: play.id } : {},
  });
}
function periodTitle(value: any) {
  return (
    value?.period_no ??
    value?.id ??
    (zh.value ? "无开放期次" : "No open period")
  );
}
function modelName(value?: string | { model?: string }) {
  const model = typeof value === "string" ? value : value?.model;
  return model === "X_PLUS_Y"
    ? "X + Y"
    : model === "M_SELECT_N"
      ? "M select N"
      : model === "DIGITS_0_9"
        ? "0–9 digits"
        : (model ?? "");
}
function detailsStatus(value: string) {
  return value.replaceAll("_", " ");
}
function isCancelAllowed(order: BetOrder) {
  return order.status === "placed" && order.policy_snapshot.user_cancel_allowed;
}
function formatExactPoints(value: unknown) {
  try {
    return BigInt(exactAmount(value)).toLocaleString(zh.value ? "zh-CN" : "en");
  } catch {
    return exactAmount(value);
  }
}

onMounted(() => {
  void loadCatalog();
  if (gameId.value) void loadGame(gameId.value);
  if (route.path === "/orders") void loadOrders();
  if (route.path.startsWith("/orders/") && placedOrderRouteId.value)
    void loadOrder(placedOrderRouteId.value);
  if (route.path.endsWith("/bet") || route.path.startsWith("/orders"))
    void refreshMember();
  pollTimer = setInterval(() => {
    void loadCatalog();
    if (gameId.value) void loadGame(gameId.value, true);
    if (route.path === "/orders") void loadOrders();
    if (route.path.startsWith("/orders/") && placedOrderRouteId.value)
      void loadOrder(placedOrderRouteId.value);
  }, 15_000);
  clockTimer = setInterval(() => {
    now.value = Date.now();
  }, 1000);
});
watch(
  () => route.path,
  (path) => {
    if (path.startsWith("/orders/")) {
      placedOrder.value = null;
      availablePoints.value = null;
      cancelIntent.value = null;
      cancelReason.value = "";
    }
    if (path === "/orders") void loadOrders();
    if (path.startsWith("/orders/") && placedOrderRouteId.value)
      void loadOrder(placedOrderRouteId.value);
    if (path.endsWith("/bet") || path.startsWith("/orders"))
      void refreshMember();
  },
);
watch(
  () => props.auth,
  (profile) => {
    if (profile) {
      memberStatus.value = profile.member.status;
      authRequired.value = false;
    }
  },
);
watch(
  () => route.query.play,
  (id) => {
    if (!uncertainPlace.value && plays.value.some((p) => p.id === id))
      chosenPlayId.value = String(id);
  },
);
onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer);
  if (clockTimer) clearInterval(clockTimer);
  catalogGeneration.value++;
  fetchGeneration.value++;
  orderGeneration.value++;
  detailOrderGeneration.value++;
  previewGeneration.value++;
});
</script>

<template>
  <section
    v-if="embeddedCatalog"
    class="catalog-embed"
    aria-label="Featured games"
    data-testid="catalog-featured"
  >
    <div v-if="catalogLoading" class="state-card" role="status">
      {{ zh ? "正在加载彩种…" : "Loading games…" }}
    </div>
    <div v-else-if="catalogError" class="state-card error" role="alert">
      {{ catalogError }}
      <button @click="loadCatalog">{{ zh ? "重试" : "Retry" }}</button>
    </div>
    <div v-else-if="!catalog.length" class="state-card">
      {{ zh ? "暂无开放彩种。" : "No games are available." }}
    </div>
    <div v-else class="catalog-grid compact">
      <article
        v-for="game in catalog.slice(0, 3)"
        :key="game.id"
        class="catalog-card"
      >
        <span class="model-pill">{{ modelName(game.model) }}</span>
        <h3>{{ game.name }}</h3>
        <p>{{ game.code }} · {{ detailsStatus(game.status) }}</p>
        <RouterLink
          :to="`/games/${encodeURIComponent(game.id)}`"
          class="panel-button"
          >{{ zh ? "查看彩种" : "View game" }} →</RouterLink
        >
      </article>
    </div>
  </section>

  <section
    v-else-if="route.path === '/games'"
    class="betting-panel"
    data-testid="games-catalog"
  >
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ zh ? "真实彩种目录" : "GAME CATALOG" }}</p>
        <h1>{{ zh ? "全部彩种" : "All games" }}</h1>
      </div>
      <button class="quiet-button" @click="loadCatalog">
        {{ zh ? "刷新" : "Refresh" }}
      </button>
    </div>
    <div v-if="catalogLoading" class="state-card" role="status">
      {{ zh ? "正在加载彩种…" : "Loading games…" }}
    </div>
    <div v-else-if="catalogError" class="state-card error" role="alert">
      {{ catalogError }}
      <button @click="loadCatalog">{{ zh ? "重试" : "Retry" }}</button>
    </div>
    <div v-else-if="!catalog.length" class="state-card">
      {{ zh ? "暂无开放彩种。" : "No games are available." }}
    </div>
    <div v-else class="catalog-grid">
      <article
        v-for="game in catalog"
        :key="game.id"
        class="catalog-card"
        data-testid="catalog-game"
      >
        <span class="model-pill">{{ modelName(game.model) }}</span
        ><span class="status-label">{{ detailsStatus(game.status) }}</span>
        <h2>{{ game.name }}</h2>
        <p>{{ game.code }} · {{ game.timezone }}</p>
        <RouterLink
          :to="`/games/${encodeURIComponent(game.id)}`"
          class="panel-button"
          >{{ zh ? "查看彩种" : "View game" }} →</RouterLink
        >
      </article>
    </div>
  </section>

  <section
    v-else-if="route.path.startsWith('/games/') && !route.path.endsWith('/bet')"
    class="betting-panel"
    data-testid="game-detail"
  >
    <div class="panel-heading">
      <div>
        <p class="eyebrow">
          {{ currentGame ? modelName(currentGame.model) : "" }}
        </p>
        <h1>{{ currentGame?.name ?? (zh ? "彩种详情" : "Game details") }}</h1>
      </div>
      <RouterLink to="/games" class="text-link"
        >← {{ zh ? "全部彩种" : "All games" }}</RouterLink
      >
    </div>
    <div v-if="detailLoading" class="state-card" role="status">
      {{ zh ? "正在加载规则和期次…" : "Loading rules and period…" }}
    </div>
    <div v-else-if="detailError" class="state-card error" role="alert">
      {{ detailError }}
      <button @click="loadGame(gameId)">{{ zh ? "重试" : "Retry" }}</button>
    </div>
    <template v-else-if="currentGame">
      <div class="period-card">
        <div>
          <span>{{ zh ? "当前期次" : "Current period" }}</span
          ><strong>{{ periodTitle(period) }}</strong>
        </div>
        <div>
          <span>{{ zh ? "状态" : "Status" }}</span
          ><strong>{{
            period?.status
              ? detailsStatus(period.status)
              : zh
                ? "暂无开放期次"
                : "No open period"
          }}</strong>
        </div>
        <div>
          <span>{{ zh ? "截止倒计时" : "Time remaining" }}</span
          ><strong class="timer">{{ cutoffText }}</strong>
        </div>
      </div>
      <div v-if="plays.length" class="play-list">
        <article v-for="play in plays" :key="play.id" class="play-card">
          <div>
            <h2>{{ play.name }}</h2>
            <p>
              {{ play.code }} · {{ zh ? "规则版本" : "Rule version" }}
              {{ play.rule_version_id }}
            </p>
            <p>
              {{ zh ? "每注" : "Unit" }} {{ play.definition.unit_points }}
              {{ zh ? "积分" : "points" }} ·
              {{ zh ? "最高倍数" : "Max multiplier" }}
              {{ play.definition.limits.max_multiplier }}
            </p>
            <div class="tier-list">
              <span
                v-for="tier in play.definition.prize_tiers ?? []"
                :key="tier.code"
                >{{ tier.code }} · ×{{ tier.odds }}</span
              >
            </div>
          </div>
          <button
            class="panel-button"
            :disabled="
              !period || ['paused', 'frozen'].includes(currentGame.status)
            "
            @click="openBet(currentGame, play)"
          >
            {{ zh ? "开始选号" : "Choose numbers" }} →
          </button>
        </article>
      </div>
      <p v-else class="state-card">
        {{
          zh
            ? "此彩种暂时没有可投注玩法。"
            : "No playable options are available."
        }}
      </p>
    </template>
  </section>

  <section
    v-else-if="route.path.endsWith('/bet')"
    class="betting-panel"
    data-testid="bet-selection-page"
  >
    <div class="panel-heading">
      <div>
        <p class="eyebrow">
          {{ currentGame ? modelName(currentGame.model) : "" }}
        </p>
        <h1>{{ currentGame?.name ?? (zh ? "投注选号" : "Place a bet") }}</h1>
      </div>
      <RouterLink :to="`/games/${encodeURIComponent(gameId)}`" class="text-link"
        >← {{ zh ? "返回彩种" : "Back to game" }}</RouterLink
      >
    </div>
    <div v-if="detailLoading" class="state-card" role="status">
      {{ zh ? "正在加载规则…" : "Loading rules…" }}
    </div>
    <div v-else-if="detailError" class="state-card error" role="alert">
      {{ detailError }}
    </div>
    <template v-else-if="definition && selectedPlay">
      <div class="period-card">
        <div>
          <span>{{ zh ? "当前期次" : "Current period" }}</span
          ><strong>{{ periodTitle(period) }}</strong>
        </div>
        <div>
          <span>{{ zh ? "截止倒计时" : "Time remaining" }}</span
          ><strong class="timer">{{ cutoffText }}</strong>
        </div>
        <div>
          <span>{{ zh ? "状态" : "Status" }}</span
          ><strong>{{
            isPaused
              ? zh
                ? "暂停"
                : "Paused"
              : period
                ? detailsStatus(period.status)
                : zh
                  ? "未开放"
                  : "Unavailable"
          }}</strong>
        </div>
      </div>
      <p v-if="isPaused" class="notice-card">
        {{
          zh
            ? "当前彩种或期次暂停投注。"
            : "Betting is paused for this game or period."
        }}
      </p>
      <label class="field-label"
        >{{ zh ? "玩法" : "Play"
        }}<select
          v-model="chosenPlayId"
          :disabled="uncertainPlace"
          aria-label="Play"
          data-testid="play-select"
        >
          <option v-for="play in plays" :key="play.id" :value="play.id">
            {{ play.name }} · {{ play.code }}
          </option>
        </select></label
      >
      <div class="rule-summary">
        <span
          >{{ zh ? "单位投注" : "Unit stake" }}
          {{ definition.unit_points }}</span
        ><span
          >{{ zh ? "最高倍数" : "Maximum multiplier" }}
          {{ definition.limits.max_multiplier }}</span
        ><span
          >{{ zh ? "规则摘要" : "Rule hash" }}
          {{ selectedPlay.definition_hash }}</span
        >
      </div>
      <BetSelection
        v-model="selection"
        :definition="definition"
        :locale="locale"
        :disabled="isPaused || uncertainPlace"
      />
      <label class="field-label"
        >{{ zh ? "倍数" : "Multiplier"
        }}<input
          v-model="multiplier"
          inputmode="numeric"
          autocomplete="off"
          pattern="[0-9]*"
          :disabled="isPaused || uncertainPlace"
          data-testid="multiplier-input"
      /></label>
      <div class="tier-list odds-list">
        <strong>{{ zh ? "奖级及赔率" : "Prize tiers and odds" }}</strong
        ><span v-for="tier in rulesTiers" :key="tier.code"
          >{{ tier.code }} · ×{{ tier.odds }}</span
        >
      </div>
      <div class="action-row">
        <button
          class="panel-button"
          :disabled="!canBet"
          data-testid="preview-button"
          @click="requestQuote"
        >
          {{
            quoteLoading
              ? zh
                ? "计算中…"
                : "Calculating…"
              : zh
                ? "获取服务器报价"
                : "Get server quote"
          }}
        </button>
      </div>
      <p v-if="quoteError" class="error-text" role="alert">{{ quoteError }}</p>
      <div v-if="quote" class="quote-card" data-testid="bet-quote">
        <strong>{{ zh ? "服务器报价" : "Server quote" }}</strong
        ><span
          >{{ zh ? "注数" : "Combinations" }}
          {{ quote.combination_count }}</span
        ><span>{{ zh ? "每注" : "Unit points" }} {{ quote.unit_points }}</span
        ><span>{{ zh ? "总积分" : "Total points" }} {{ quote.bet_points }}</span
        ><span>{{ zh ? "期次" : "Period" }} {{ quote.period_id }}</span
        ><span
          >{{ zh ? "规则摘要" : "Rule hash" }} {{ quote.definition_hash }}</span
        ><button
          class="panel-button"
          data-testid="review-bet"
          @click="beginReview"
        >
          {{ zh ? "审核并确认" : "Review bet" }}
        </button>
      </div>
    </template>
  </section>

  <section
    v-else-if="route.path === '/bet/confirm'"
    class="betting-panel"
    data-testid="bet-confirmation"
  >
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ zh ? "确认投注" : "CONFIRM BET" }}</p>
        <h1>{{ zh ? "核对投注详情" : "Review your bet" }}</h1>
      </div>
    </div>
    <div v-if="!review" class="state-card">
      {{
        zh
          ? "报价已失效或不存在，请返回重新获取。"
          : "The quote is no longer available. Return and request a fresh quote."
      }}
      <RouterLink
        v-if="confirmationGameId"
        :to="`/games/${encodeURIComponent(confirmationGameId)}/bet`"
        >{{ zh ? "返回选号" : "Return to selection" }}</RouterLink
      ><RouterLink v-else to="/games">{{
        zh ? "浏览彩种" : "Browse games"
      }}</RouterLink>
    </div>
    <template v-else
      ><div class="quote-card">
        <strong>{{ currentGame?.name ?? review.quote.period.game_id }}</strong
        ><span
          >{{ zh ? "注数" : "Combinations" }}
          {{ review.quote.combination_count }}</span
        ><span
          >{{ zh ? "倍数" : "Multiplier" }} {{ review.body.multiplier }}</span
        ><span
          >{{ zh ? "总积分" : "Total points" }}
          {{ review.quote.bet_points }}</span
        ><span
          >{{ zh ? "期次" : "Period" }}
          {{ review.quote.period.period_no }}</span
        ><span
          >{{ zh ? "规则摘要" : "Rule hash" }}
          {{ review.quote.definition_hash }}</span
        ><span
          >{{ zh ? "奖级赔率" : "Prize odds" }}
          {{
            review.definition.prize_tiers
              ?.map((tier) => `${tier.code} ×${tier.odds}`)
              .join(" · ")
          }}</span
        ><span
          >{{ zh ? "品牌/游戏政策版本" : "Brand/game policy versions" }}
          {{ review.quote.policy_versions.brand }} /
          {{ review.quote.policy_versions.game }}</span
        >
      </div>
      <SelectionSummary :selection="review.quote.normalized" :locale="locale" />
      <p
        v-if="reviewExpired && !uncertainPlace"
        class="error-text"
        role="status"
      >
        {{
          zh
            ? "投注已截止，请返回查看新期次。"
            : "Betting has closed. Return to select a new period."
        }}
      </p>
      <p class="notice-card">
        {{
          zh
            ? "确认将按上述报价提交不可变投注请求。网络不确定时重试会使用同一请求编号。"
            : "Confirming submits this immutable request. If the network result is uncertain, retry uses the same request key."
        }}
      </p>
      <div class="action-row">
        <RouterLink
          v-if="!uncertainPlace && confirmationGameId"
          :to="`/games/${encodeURIComponent(confirmationGameId)}/bet`"
          class="quiet-button"
          >{{ zh ? "返回修改" : "Back to edit" }}</RouterLink
        ><button
          class="panel-button"
          :disabled="placing || (reviewExpired && !uncertainPlace)"
          data-testid="confirm-bet"
          @click="placeOrder"
        >
          {{
            placing
              ? zh
                ? "提交中…"
                : "Submitting…"
              : uncertainPlace
                ? zh
                  ? "重试确认"
                  : "Retry confirm"
                : zh
                  ? "确认投注"
                  : "Confirm bet"
          }}
        </button>
      </div>
      <p v-if="quoteError" class="error-text" role="alert">
        {{ quoteError }}
        <RouterLink v-if="authRequired" to="/login">{{
          zh ? "登录" : "Sign in"
        }}</RouterLink>
      </p></template
    >
  </section>

  <section
    v-else-if="route.path === '/orders'"
    class="betting-panel"
    data-testid="orders-list"
  >
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ zh ? "真实注单" : "BET HISTORY" }}</p>
        <h1>{{ zh ? "我的注单" : "My orders" }}</h1>
      </div>
      <button class="quiet-button" @click="loadOrders">
        {{ zh ? "刷新" : "Refresh" }}
      </button>
    </div>
    <div v-if="authRequired" class="state-card">
      {{ zh ? "请登录查看投注记录。" : "Sign in to view your bet history." }}
      <RouterLink to="/login">{{ zh ? "登录" : "Sign in" }}</RouterLink>
    </div>
    <div v-else-if="ordersLoading" class="state-card" role="status">
      {{ zh ? "正在加载注单…" : "Loading orders…" }}
    </div>
    <p v-else-if="orderError" class="state-card error" role="alert">
      {{ orderError }}
    </p>
    <p v-else-if="!orders.length" class="state-card">
      {{ zh ? "暂无投注记录。" : "You have no bet orders yet." }}
    </p>
    <article v-for="order in orders" :key="order.id" class="order-card">
      <div>
        <strong>{{ order.game_id }}</strong
        ><span>{{ order.period_id }} · {{ detailsStatus(order.status) }}</span
        ><span
          >{{ zh ? "积分" : "Points" }} {{ exactAmount(order.total_points) }} ·
          ×{{ exactAmount(order.multiplier) }}</span
        >
      </div>
      <RouterLink
        :to="`/orders/${encodeURIComponent(order.id)}`"
        class="text-link"
        >{{ zh ? "详情" : "Details" }} →</RouterLink
      >
    </article>
  </section>

  <section
    v-else-if="route.path.startsWith('/orders/')"
    class="betting-panel"
    data-testid="order-detail"
  >
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ zh ? "注单详情" : "ORDER DETAILS" }}</p>
        <h1>{{ placedOrder?.id ?? placedOrderRouteId }}</h1>
      </div>
      <RouterLink to="/orders" class="text-link"
        >← {{ zh ? "我的注单" : "My orders" }}</RouterLink
      >
    </div>
    <div v-if="!placedOrder && !orderError" class="state-card" role="status">
      {{ zh ? "正在加载注单…" : "Loading order…" }}
    </div>
    <p v-if="orderError" class="state-card error" role="alert">
      {{ orderError }}
    </p>
    <div v-if="availablePoints !== null" class="wallet-balance">
      <span>{{ zh ? "最新可用积分" : "Current available points" }}</span
      ><strong>{{ formatExactPoints(availablePoints) }}</strong>
    </div>
    <div v-if="placedOrder" class="quote-card">
      <strong
        >{{ zh ? "状态" : "Status" }} ·
        {{ detailsStatus(placedOrder.status) }}</strong
      ><span>{{ zh ? "期次" : "Period" }} {{ placedOrder.period_id }}</span
      ><span>{{ zh ? "玩法" : "Play" }} {{ placedOrder.play_id }}</span
      ><span
        >{{ zh ? "总积分" : "Total points" }}
        {{ exactAmount(placedOrder.total_points) }}</span
      ><span
        >{{ zh ? "倍数" : "Multiplier" }}
        {{ exactAmount(placedOrder.multiplier) }}</span
      ><span
        >{{ zh ? "赔率规则摘要" : "Rule hash" }}
        {{ placedOrder.definition_hash }}</span
      >
      <SelectionSummary
        :selection="placedOrder.selection_normalized"
        :locale="locale"
      />
    </div>
    <form
      v-if="placedOrder && isCancelAllowed(placedOrder)"
      class="cancel-form"
      @submit.prevent="cancelOrder(placedOrder)"
    >
      <label class="field-label"
        >{{ zh ? "取消原因（必填）" : "Cancellation reason (required)"
        }}<textarea
          v-model="cancelReason"
          required
          maxlength="500"
          rows="3"
          :disabled="Boolean(cancelIntent)"
        ></textarea>
      </label>
      <p v-if="cancelReason && !cancelReasonBytesValid" class="error-text">
        {{
          zh
            ? "原因须为 500 字节以内的 UTF-8 文本。"
            : "Reason must be valid UTF-8 within 500 bytes."
        }}
      </p>
      <button
        class="quiet-button"
        :disabled="cancelBusy || (!cancelIntent && !cancelReasonBytesValid)"
      >
        {{
          cancelBusy
            ? zh
              ? "处理中…"
              : "Processing…"
            : cancelIntent
              ? zh
                ? "重试取消"
                : "Retry cancellation"
              : zh
                ? "取消此注单"
                : "Cancel this order"
        }}
      </button>
    </form>
  </section>
</template>

<style scoped>
.betting-panel,
.catalog-embed,
.panel-heading > div,
.play-card > div,
.order-card > div {
  min-width: 0;
}
.panel-heading h1,
.quote-card,
.order-card,
.period-card,
.play-card,
.state-card {
  overflow-wrap: anywhere;
}
.wallet-balance {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  flex-wrap: wrap;
  padding: 0.8rem;
  border: 1px solid #dce6dc;
  border-radius: 10px;
  background: #edf4eb;
}
.betting-panel,
.catalog-embed {
  color: var(--ink, #1c2a22);
  max-width: 1100px;
  margin: 0 auto;
}
.betting-panel {
  padding: clamp(1rem, 3vw, 2rem);
  display: grid;
  gap: 1.2rem;
}
.panel-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}
.panel-heading h1 {
  margin: 0.25rem 0;
  font-size: clamp(1.6rem, 4vw, 2.35rem);
}
.eyebrow {
  margin: 0;
  color: var(--brand-primary);
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.12em;
}
.catalog-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 250px), 1fr));
  gap: 1rem;
}
.catalog-card,
.period-card,
.play-card,
.state-card,
.quote-card,
.order-card,
.notice-card {
  padding: 1.1rem;
  background: #fff;
  border: 1px solid #e3eae2;
  border-radius: 16px;
  box-shadow: 0 8px 24px #1d39230a;
}
.catalog-card h2,
.catalog-card h3 {
  margin: 0.8rem 0 0.3rem;
}
.catalog-card p,
.play-card p {
  color: #708078;
}
.catalog-grid.compact {
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 190px), 1fr));
}
.catalog-embed {
  width: 100%;
}
.model-pill,
.status-label {
  display: inline-flex;
  padding: 0.32rem 0.55rem;
  border-radius: 999px;
  background: #edf4eb;
  color: #395d46;
  font-size: 0.75rem;
  font-weight: 700;
}
.status-label {
  float: right;
  text-transform: capitalize;
}
.panel-button,
.quiet-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  min-height: 44px;
  padding: 0.65rem 1rem;
  border: 0;
  border-radius: 10px;
  background: var(--brand-primary, #347555);
  color: white;
  font: inherit;
  font-weight: 700;
  text-decoration: none;
  cursor: pointer;
}
.panel-button:disabled,
.quiet-button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.quiet-button {
  background: #edf2ed;
  color: #30473a;
  border: 1px solid #dce6dc;
}
.text-link {
  color: var(--brand-primary);
  font-weight: 700;
}
.period-card {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1rem;
}
.period-card div,
.quote-card {
  display: grid;
  gap: 0.45rem;
}
.period-card span,
.quote-card span,
.order-card span {
  color: #748078;
  font-size: 0.88rem;
}
.timer {
  font-variant-numeric: tabular-nums;
}
.play-list {
  display: grid;
  gap: 0.8rem;
}
.play-card,
.order-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}
.play-card h2 {
  margin: 0.3rem 0;
}
.tier-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.7rem;
}
.tier-list span {
  padding: 0.35rem 0.55rem;
  border-radius: 8px;
  background: #f0f4ef;
  font-size: 0.8rem;
}
.rule-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}
.rule-summary span {
  padding: 0.5rem 0.7rem;
  border: 1px solid #e2e9e1;
  border-radius: 9px;
  overflow-wrap: anywhere;
  font-size: 0.86rem;
}
.field-label {
  display: grid;
  gap: 0.45rem;
  font-weight: 700;
}
.field-label input,
.field-label select,
.field-label textarea {
  width: 100%;
  min-height: 46px;
  padding: 0.7rem;
  border: 1px solid #ccd8cc;
  border-radius: 10px;
  background: white;
  color: inherit;
  font: inherit;
}
.action-row {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
}
.quote-card {
  gap: 0.65rem;
}
.error,
.error-text {
  color: #a33333;
}
.state-card button {
  margin-left: 0.6rem;
}
.notice-card {
  background: #fbf7e9;
}
.cancel-form {
  display: grid;
  gap: 0.8rem;
}
.order-card {
  margin-top: 0.7rem;
}
.order-card > div {
  display: grid;
  gap: 0.3rem;
}
.catalog-embed .state-card {
  margin: 0.5rem 0;
}
@media (max-width: 600px) {
  .period-card {
    grid-template-columns: 1fr 1fr;
  }
  .period-card div:last-child {
    grid-column: 1/-1;
  }
  .play-card,
  .order-card {
    align-items: flex-start;
    flex-direction: column;
  }
  .panel-heading {
    align-items: flex-start;
  }
  .betting-panel {
    padding: 0.8rem;
  }
  .rule-summary {
    display: grid;
    grid-template-columns: 1fr;
  }
}
</style>
