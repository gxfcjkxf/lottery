<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useUserI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName, copyAssociationNames } from "@lottery/shared";
import { useUserI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { vInlineValidation } from '@lottery/shared'
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
import BettingHistory from "./BettingHistory.vue";
import BetSelection from "./BetSelection.vue";
import SelectionSummary from "@lottery/shared/SelectionSummary.vue";
import OrderPrize from "@lottery/shared/OrderPrize.vue";
import { createWalletApi } from "./wallet-api";
import DrawArchive from './DrawArchive.vue';
import PlayRules from "./PlayRules.vue";
import UserPeriodStrip from './UserPeriodStrip.vue';
import BettingSheet from './BettingSheet.vue';
import {standardQuoteChanged,selectionReady} from './bet-confirmation';
import {createDrawsApi,type PublicDrawResult} from './draws-api';
import {readStandardIntent,saveStandardIntent,clearStandardIntent} from './standard-bet-intent';
import { useUserI18n } from "./i18n";

const props = defineProps<{
  locale: Language;
  brandCode?: string;
  auth?: AuthProfile | null;
  embeddedCatalog?: boolean;
  embeddedGameId?: string;
}>();
const emit = defineEmits<{ "auth-expired": []; "game-title": [title:string] }>();
const route = useRoute();
const router = useRouter();
const api = createBettingApi({ brandCode: props.brandCode });
const authApi = createAuthClient({ brandCode: props.brandCode });
const ui = useUserI18n().t;
const catalog = ref<CatalogGame[]>([]);
const previousDraw=ref<PublicDrawResult|null>(null);
const draftOpen=ref(false);
const submittingStandard=ref(false),standardChanged=ref(false);
const hasSelection=computed(()=>[...(selection.value.regular??[]),...(selection.value.special??[]),...(selection.value.digits??[]).flat(),...(selection.value.exclude??[]),...Object.values(selection.value.attributes??{}).flat(),...Object.values(selection.value.features??{}).flat()].length>0);
const catalogLoading = ref(false);
const catalogError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const catalogGeneration = ref(0);
const detailLoading = ref(false);
const detailError = ref<string | import("@lottery/shared").LocalizedMessage>("");
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
const quoteError = ref<string | import("@lottery/shared").LocalizedMessage>("");
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
const orderError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const cancelReason = ref("");
const cancelBusy = ref(false);
const cancelReasonBytesValid = computed(
  () =>
    new TextEncoder().encode(cancelReason.value.trim()).length <= 500,
);
const cancelIntent = ref<{
  orderId: string;
  body: { version: number; reason: string };
  key: string;
} | null>(null);
const uncertainPlace = ref(false);
const intentReadFailed = ref(false);
watch(selection,()=>{if(!uncertainPlace.value)draftOpen.value=hasSelection.value},{deep:true});
const previewGeneration = ref(0);
const previewBody = ref<BetInput | null>(null);
type AuthIdentity = { userId: string; memberId: string; brandId: string };
const previewActor = ref<AuthIdentity | null>(null);
const notice = ref("");
const fetchGeneration = ref(0);
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
  const copy = JSON.parse(JSON.stringify(value)) as T;
  copyAssociationNames(value,copy);
  return copy;
}
function freezeDeep<T>(value: T): T {
  if (value && typeof value === "object" && !Object.isFrozen(value)) {
    Object.freeze(value);
    for (const child of Object.values(value as Record<string, unknown>))
      freezeDeep(child);
  }
  return value;
}
function message(error: unknown, protectedRequest = false): string | import("@lottery/shared").LocalizedMessage {
  const raw = error as { status?: number; code?: string; message?: string };
  if (raw?.status === 401 && protectedRequest) {
    authRequired.value = true;
    emit("auth-expired");
    return ui("betting.sessionExpired");
  }
  if (
    raw?.status === 409 ||
    /CONFLICT|CLOSED|VERSION|POLICY|CUTOFF|RULE/i.test(raw?.code ?? "")
  ) {
    invalidateReview();
    return ui("betting.stateChanged");
  }
  if (raw?.status === 0 && raw.code !== "invalid_multiplier")
    return ui("betting.uncertainNetwork");
  return (
    publicErrorMessage(raw) ||
    ui("betting.serviceUnavailable")
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
const gameId = computed(() => props.embeddedGameId ?? String(route.params.gameId ?? ""));
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
      !intentReadFailed.value &&
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

async function loadCatalog(passive = false) {
  const generation = ++catalogGeneration.value;
  if (!passive) catalogLoading.value = true;
  try {
    const result = await api.games();
    if (generation !== catalogGeneration.value) return;
    catalog.value = result.items;
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
    const [result,draws] = await Promise.all([api.game(id),createDrawsApi(props.brandCode).listResults({gameId:id,limit:1,offset:0})]);
    if (generation !== fetchGeneration.value || id !== gameId.value) return;
    currentGame.value = result.game;
    previousDraw.value = draws.items[0] ?? null;
    emit("game-title",result.game.name);
    plays.value = result.plays;
    period.value = result.period;
    policy.value = result.policy;
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
    serverOffset.value = Date.parse(result.server_time) - Date.now();
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
  standardChanged.value=false;
  if(definition.value && hasSelection.value && selectionReady(definition.value,selection.value) && props.auth && !isPaused.value && /^[1-9][0-9]*$/.test(multiplier.value))void requestQuote();
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
    quoteError.value = ui("betting.noOpenPeriodError");
    return;
  }
  if (!/^[1-9][0-9]*$/.test(multiplier.value)) {
    quoteError.value = ui("betting.invalidMultiplier");
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
      quoteError.value = ui("betting.accountCannotBet");
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
      quoteError.value = ui("betting.accountChanged");
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
  draftOpen.value = true;
}
async function submitStandard(){
 if(submittingStandard.value||placing.value||!canBet.value||!quote.value)return;
 const shown=clone(quote.value);submittingStandard.value=true;standardChanged.value=false;
 try{await requestQuote();if(!quote.value)return;if(standardQuoteChanged(shown,quote.value)){standardChanged.value=true;return}beginReview();await placeOrder()}finally{submittingStandard.value=false}
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
      quoteError.value = ui("betting.accountChangedContinue");
      return;
    }
    if (
      !uncertainPlace.value &&
      (profile.member.status !== "normal" || profile.user.status !== "active")
    ) {
      quoteError.value = ui("betting.accountCannotBet");
      return;
    }
    try { saveStandardIntent(sessionStorage,current.quote.period.game_id,current); }
    catch { quoteError.value=ui('betting.serviceUnavailable'); return; }
    const result = await api.place(
      clone(current.body) as BetInput,
      current.key,
    );
    clearStandardIntent(sessionStorage,current.quote.period.game_id,current);
    if(props.auth?.user.id!==current.actor.userId||props.auth.member.id!==current.actor.memberId||props.auth.member.brand_id!==current.actor.brandId)return;
    placedOrder.value = result;
    invalidateReview();
    await Promise.allSettled([refreshWalletEvent()]);
    selection.value=emptySelection(); draftOpen.value=false;
    notice.value=ui("workspace.betSuccess");
  } catch (error) {
    const status = (error as { status?: number })?.status;
    if(status && status>=400 && status<500 && status!==401)clearStandardIntent(sessionStorage,current.quote.period.game_id,current);
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
function restoreStandardReview(){
 if(!props.auth||!gameId.value)return;
 const actor={userId:props.auth.user.id,memberId:props.auth.member.id,brandId:props.auth.member.brand_id};
 try{const saved=readStandardIntent(sessionStorage,actor,gameId.value);if(saved){uncertainPlace.value=true;review.value=freezeDeep(saved);draftOpen.value=true}}
 catch{intentReadFailed.value=true;quoteError.value=ui('betting.serviceUnavailable')}
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
function periodTitle(value: import("./betting-api").Period | null) {
  return value ? value.period_no : ui("betting.noOpenPeriod");
}
function modelName(value: RuleDefinition['model']) {
  const labels: Record<string, string> = { X_PLUS_Y: ui('user.bet.model.xPlusY'), M_SELECT_N: ui('user.bet.model.mSelectN'), DIGITS_0_9: ui('user.bet.model.digits') };
  if (!labels[value.model]) throw new Error('Unsupported number model');
  return labels[value.model];
}
function detailsStatus(value: string) {
  return value.replaceAll("_", " ");
}
function isCancelAllowed(order: BetOrder) {
  return order.status === "placed" && order.policy_snapshot.user_cancel_allowed;
}
function formatExactPoints(value: string) {
  return BigInt(value).toLocaleString(props.locale === "zh" ? "zh-CN" : "en");
}

onMounted(() => {
  restoreStandardReview();
  if (!gameId.value && route.path !== "/orders") void loadCatalog();
  if (gameId.value) void loadGame(gameId.value);
  if (route.path.startsWith("/orders/") && placedOrderRouteId.value)
    void loadOrder(placedOrderRouteId.value);
  if (props.embeddedGameId || route.path.endsWith("/bet") || route.path.startsWith("/orders/"))
    void refreshMember();
  pollTimer = setInterval(() => {
    if (!gameId.value && route.path !== "/orders") void loadCatalog(true);
    if (gameId.value) void loadGame(gameId.value, true);
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
    if (path.startsWith("/orders/") && placedOrderRouteId.value)
      void loadOrder(placedOrderRouteId.value);
    if (path.endsWith("/bet") || path.startsWith("/orders/"))
      void refreshMember();
  },
);
watch(
  () => props.auth,
  (profile) => {
    if(review.value && (!profile || profile.user.id!==review.value.actor.userId || profile.member.id!==review.value.actor.memberId || profile.member.brand_id!==review.value.actor.brandId)) {
      invalidateReview(); selection.value=emptySelection(); draftOpen.value=false;
    }
    if (profile) {
      memberStatus.value = profile.member.status;
      authRequired.value = false;
      restoreStandardReview();
      if(!uncertainPlace.value && definition.value && hasSelection.value && selectionReady(definition.value,selection.value))void requestQuote();
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
  detailOrderGeneration.value++;
  previewGeneration.value++;
});
</script>

<template>
  <BettingHistory v-if="route.path === '/orders'" :brand-code="brandCode" :locale="locale" :auth="auth" @auth-expired="emit('auth-expired')" />
  <section
    v-if="embeddedCatalog"
    class="catalog-embed"
    :aria-label="ui('betting.allGames')"
    data-testid="catalog-featured"
  >
    <div v-global-loading v-if="catalogLoading" class="state-card" role="status">
      {{ ui("betting.loadingGames") }}
    </div>
    <div v-else-if="catalogError" class="state-card error" role="alert">
      {{ displayPublicError(catalogError) }}
      <button @click="withGlobalLoading(loadCatalog)">{{ ui("betting.retry") }}</button>
    </div>
    <div v-else-if="!catalog.length" class="state-card empty-state">
      {{ ui("betting.noGames") }}
    </div>
    <div v-else class="catalog-grid compact">
      <article
        v-for="game in catalog.slice(0, 3)"
        :key="game.id"
        class="catalog-card"
      >
        <span class="model-pill">{{ modelName(game.model) }}</span>
        <h3>{{ game.name }}</h3>

        <RouterLink
          :to="`/games/${encodeURIComponent(game.id)}`"
          class="panel-button"
          >{{ ui("betting.viewGame") }} <SiteIcon name="next" /> </RouterLink
        >
        <RouterLink :to="{ path: '/results', query: { game_id: game.id } }" class="text-link game-results-link">{{ ui('user.page.results') }}</RouterLink>
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
        <h1>{{ ui("betting.allGames") }}</h1>
      </div>
      <Teleport defer to="#user-page-refresh"><button v-refresh-button="ui('betting.refresh')" class="quiet-button" @click="withGlobalLoading(loadCatalog)">
        {{ ui("betting.refresh") }}
      </button></Teleport>
    </div>
    <div v-global-loading v-if="catalogLoading" class="state-card" role="status">
      {{ ui("betting.loadingGames") }}
    </div>
    <div v-else-if="catalogError" class="state-card error" role="alert">
      {{ displayPublicError(catalogError) }}
      <button @click="withGlobalLoading(loadCatalog)">{{ ui("betting.retry") }}</button>
    </div>
    <div v-else-if="!catalog.length" class="state-card empty-state">
      {{ ui("betting.noGames") }}
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

        <RouterLink
          :to="`/games/${encodeURIComponent(game.id)}`"
          class="panel-button"
          >{{ ui("betting.viewGame") }} <SiteIcon name="next" /> </RouterLink
        >
        <RouterLink :to="{ path: '/results', query: { game_id: game.id } }" class="text-link game-results-link">{{ ui('user.page.results') }}</RouterLink>
      </article>
    </div>
  </section>

  <section
    v-else-if="!embeddedGameId && route.path.startsWith('/games/') && !route.path.endsWith('/bet')"
    class="betting-panel"
    data-testid="game-detail"
  >
    <div class="panel-heading">
      <div>
        <p class="eyebrow">
          {{ currentGame ? modelName(currentGame.model) : "" }}
        </p>
        <h1>{{ currentGame?.name ?? (ui("betting.gameDetails")) }}</h1>
      </div>
      <RouterLink to="/games" class="text-link"
        > <SiteIcon name="back" /> {{ ui("betting.allGames") }}</RouterLink
      >
    </div>
    <div v-global-loading v-if="detailLoading" class="state-card" role="status">
      {{ ui("betting.loadingRulesAndPeriod") }}
    </div>
    <div v-else-if="detailError" class="state-card error" role="alert">
      {{ displayPublicError(detailError) }}
      <button @click="withGlobalLoading(loadGame, gameId)">{{ ui("betting.retry") }}</button>
    </div>
    <template v-else-if="currentGame">
      <div class="period-card">
        <div>
          <span>{{ ui("betting.currentPeriod") }}</span
          ><strong>{{ periodTitle(period) }}</strong>
        </div>
        <div>
          <span>{{ ui("betting.status") }}</span
          ><strong>{{
            period?.status
              ? detailsStatus(period.status)
              : ui("betting.noOpenPeriod")
          }}</strong>
        </div>
        <div>
          <span>{{ ui("betting.timeRemaining") }}</span
          ><strong class="timer">{{ cutoffText }}</strong>
        </div>
      </div>
      <div v-if="plays.length" class="play-list">
        <article v-for="play in plays" :key="play.id" class="play-card">
          <div>
            <h2>{{ play.name }}</h2>

            <p>
              {{ ui("betting.unit") }} {{ play.definition.unit_points }}
              {{ ui("betting.points") }} ·
              {{ ui("betting.maxMultiplier") }}
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
            {{ ui("betting.chooseNumbers") }} <SiteIcon name="next" /> </button>
        </article>
      </div>
      <p v-else class="state-card empty-state">
        {{
          ui("betting.noPlayableOptions")
        }}
      </p>
      <DrawArchive kind="standard"
        :key="`${props.brandCode ?? ''}:${currentGame.id}`"
        :locale="props.locale"
        :brand-code="props.brandCode"
        :game-id="currentGame.id"
        compact
      />
    </template>
  </section>

  <section
    v-else-if="embeddedGameId || route.path.endsWith('/bet')"
    class="betting-panel workspace-standard"
    data-testid="bet-selection-page"
  >
    <div v-global-loading v-if="detailLoading" class="state-card" role="status">
      {{ ui("betting.loadingRules") }}
    </div>
    <div v-else-if="detailError" class="state-card error" role="alert">
      {{ displayPublicError(detailError) }}
    </div>
    <template v-else-if="definition && selectedPlay">
      <Teleport defer to="#user-page-refresh"><button v-refresh-button="ui('betting.refresh')" class="button button-secondary" :disabled="detailLoading || placing" @click="withGlobalLoading(loadGame, gameId)">{{ ui('betting.refresh') }}</button></Teleport>
      <p v-if="notice" role="status">{{ notice }}</p>
      <p v-if="intentReadFailed" role="alert">{{ displayPublicError(quoteError) }}</p>
      <UserPeriodStrip :previous-no="previousDraw?.period.period_no" :period-no="period?.period_no" :close-at="period?.bet_end_at" :draw-at="period?.draw_at" :now="now" :clock-offset="serverOffset"><template v-if="previousDraw"><span v-for="(n,index) in [...previousDraw.result.regular,...previousDraw.result.special,...previousDraw.result.digits]" :key="index" class="standard-result-ball">{{ n }}</span></template></UserPeriodStrip>
      <p v-if="isPaused" class="notice-card">
        {{
          ui("betting.bettingPaused")
        }}
      </p>
      <PlayRules :locale="locale" :name="selectedPlay.name" :definition="definition" />
      <div class="standard-play-layout"><nav class="standard-play-groups" :aria-label="ui('betting.play')"><button v-for="play in plays" :key="play.id" type="button" :aria-pressed="chosenPlayId===play.id" :disabled="uncertainPlace||submittingStandard||placing" @click="chosenPlayId=play.id">{{ play.name }}</button></nav><div class="standard-play-options">
      <BetSelection
        v-model="selection"
        :definition="definition"
        :locale="locale"
        :disabled="isPaused || uncertainPlace || intentReadFailed || submittingStandard || placing"
      />
      </div></div>
      <button v-if="hasSelection || uncertainPlace" class="button button-primary selection-reopen" @click="draftOpen=true">{{ ui('betting.reviewBet') }}</button>
      <BettingSheet v-if="draftOpen && (hasSelection || uncertainPlace)" :title="ui('betting.confirm')" :close-label="ui('workspace.continue')" @close="draftOpen=false">
      <template v-if="!uncertainPlace">
      <SelectionSummary :selection="selection" :locale="locale" />
      <label class="field-label"
        >{{ ui("betting.multiplier")
        }}<input
          v-model="multiplier"
          inputmode="numeric"
          autocomplete="off"
          pattern="[0-9]*"
          :disabled="isPaused || uncertainPlace || submittingStandard || placing"
          data-testid="multiplier-input"
      /></label>
      <div class="tier-list odds-list">
        <strong>{{ ui("betting.prizeTiersOdds") }}</strong
        ><span v-for="tier in rulesTiers" :key="tier.code"
          >{{ tier.code }} · ×{{ tier.odds }}</span
        >
      </div>
      <p v-if="quoteError" class="error-text" role="alert">{{ displayPublicError(quoteError) }}</p>
      <p v-if="standardChanged" role="status">{{ ui('workspace.quoteChanged') }}</p>
      <div v-if="quote" class="sheet-totals" data-testid="bet-quote"><span>{{ ui('workspace.betCount') }} <strong>{{ quote.combination_count }}</strong></span><span>{{ ui('workspace.betPoints') }} <strong class="summary-points">{{ quote.bet_points }}</strong></span></div>
      </template>
      <template v-else-if="review"><SelectionSummary :selection="review.quote.normalized" :locale="locale" /><strong>{{ ui('workspace.betPoints') }} {{ review.quote.bet_points }}</strong><p>{{ ui('betting.period') }} {{ review.quote.period.period_no }}</p><p v-if="quoteError" role="alert">{{ displayPublicError(quoteError) }}</p></template>
      <div class="sheet-summary"><div class="dock-actions"><button type="button" class="button button-secondary" :disabled="submittingStandard||placing||uncertainPlace" @click="selection=emptySelection()">{{ ui('workspace.clear') }}</button><button v-write-guard="placing || submittingStandard" v-if="uncertainPlace" class="button button-primary" :disabled="placing" data-testid="confirm-bet" @click="withGlobalLoading(placeOrder)">{{ ui('betting.retryConfirm') }}</button><button v-write-guard="placing || submittingStandard" v-else type="button" class="button button-primary" :disabled="!canBet || !quote || submittingStandard || placing" data-testid="confirm-bet" @click="withGlobalLoading(submitStandard)">{{ standardChanged?ui('workspace.confirmChanges'):ui('betting.confirm') }}</button><button type="button" class="button button-secondary sheet-continue" @click="draftOpen=false">{{ ui('workspace.continue') }}</button></div></div>
      </BettingSheet>
    </template>
  </section>

  <section
    v-else-if="route.path === '/bet/confirm'"
    class="betting-panel"
    data-testid="bet-confirmation"
  >
    <div class="panel-heading">
      <div>
        <p class="eyebrow">{{ ui("betting.confirmEyebrow") }}</p>
        <h1>{{ ui("betting.reviewYourBet") }}</h1>
      </div>
    </div>
    <div v-if="!review" class="state-card">
      {{
        ui("betting.quoteInvalid")
      }}
      <RouterLink
        v-if="confirmationGameId"
        :to="`/games/${encodeURIComponent(confirmationGameId)}/bet`"
        >{{ ui("betting.returnSelection") }}</RouterLink
      ><RouterLink v-else to="/games">{{
        ui("betting.browseGames")
      }}</RouterLink>
    </div>
    <template v-else
      ><div class="quote-card">
        <strong>{{ currentGame?.name ?? ui('association.unavailable') }}</strong
        ><span
          >{{ ui("betting.combinations") }}
          {{ review.quote.combination_count }}</span
        ><span
          >{{ ui("betting.multiplier") }} {{ review.body.multiplier }}</span
        ><span
          >{{ ui("betting.totalPoints") }}
          {{ review.quote.bet_points }}</span
        ><span
          >{{ ui("betting.period") }}
          {{ review.quote.period.period_no }}</span
        ><span
          >{{ ui("betting.prizeOdds") }}
          {{
            review.definition.prize_tiers
              ?.map((tier) => `${tier.code} ×${tier.odds}`)
              .join(" · ")
          }}</span
        >
      </div>
      <SelectionSummary :selection="review.quote.normalized" :locale="locale" />
      <p
        v-if="reviewExpired && !uncertainPlace"
        class="error-text"
        role="status"
      >
        {{
          ui("betting.bettingClosed")
        }}
      </p>
      <p class="notice-card">
        {{
          ui("betting.confirmImmutable")
        }}
      </p>
      <div class="action-row">
        <RouterLink
          v-if="!uncertainPlace && confirmationGameId"
          :to="`/games/${encodeURIComponent(confirmationGameId)}/bet`"
          class="quiet-button"
          >{{ ui("betting.backToEdit") }}</RouterLink
        ><button
          v-write-guard="placing || submittingStandard"
          class="panel-button"
          :disabled="placing || (reviewExpired && !uncertainPlace)"
          data-testid="confirm-bet"
          @click="withGlobalLoading(placeOrder)"
        >
          {{
            placing
              ? ui("betting.submitting")
              : uncertainPlace
                ? ui("betting.retryConfirm")
                : ui("betting.confirm")
          }}
        </button>
      </div>
      <p v-if="quoteError" class="error-text" role="alert">
        {{ displayPublicError(quoteError) }}
        <RouterLink v-if="authRequired" to="/login">{{
          ui("betting.signIn")
        }}</RouterLink>
      </p></template
    >
  </section>

  <section
    v-else-if="route.path.startsWith('/orders/')"
    class="betting-panel"
    data-testid="order-detail"
  >
    <div class="panel-heading">
      <div>
        <h1>{{ placedOrder?.id ?? placedOrderRouteId }}</h1>
      </div>
      <RouterLink to="/orders" class="text-link"
        > <SiteIcon name="back" /> {{ ui("betting.myOrders") }}</RouterLink
      >
    </div>
    <div v-if="!placedOrder && !orderError" class="state-card" role="status">
      {{ ui("betting.loadingOrder") }}
    </div>
    <p v-if="orderError" class="state-card error" role="alert">
      {{ displayPublicError(orderError) }}
    </p>
    <div v-if="availablePoints !== null" class="wallet-balance">
      <span>{{ ui("betting.currentAvailablePoints") }}</span
      ><strong>{{ formatExactPoints(availablePoints) }}</strong>
    </div>
    <div v-if="placedOrder" class="quote-card">
      <strong
        >{{ ui("betting.status") }} ·
        {{ detailsStatus(placedOrder.status) }}</strong
      ><span>{{ ui("betting.period") }} <AssociationName :record="placedOrder" kind="period" :id="placedOrder.period_id" :locale="associationLocale" /></span
      ><OrderPrize :order="placedOrder" :locale="locale" />
      <span>{{ ui("betting.play") }} <AssociationName :record="placedOrder" kind="play" :id="placedOrder.play_id" :locale="associationLocale" /></span
      ><span
        >{{ ui("betting.totalPoints") }}
        {{ placedOrder.total_points }}</span
      ><span
        >{{ ui("betting.multiplier") }}
        {{ placedOrder.multiplier }}</span
      >
      <SelectionSummary
        :selection="placedOrder.selection_normalized"
        :locale="locale"
      />
    </div>
    <form v-write-guard="cancelBusy" novalidate v-inline-validation
      v-if="placedOrder && isCancelAllowed(placedOrder)"
      class="cancel-form"
      @submit.prevent="withGlobalLoading(cancelOrder, placedOrder)"
    >

      <p v-if="cancelReason && !cancelReasonBytesValid" class="error-text">
        {{
          ui("betting.reasonBytes")
        }}
      </p>
      <button
        class="quiet-button"
        :disabled="cancelBusy || (!cancelIntent && !cancelReasonBytesValid)"
      >
        {{
          cancelBusy
            ? ui("betting.processing")
            : cancelIntent
              ? ui("betting.retryCancellation")
              : ui("betting.cancelOrder")
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
  color: var(--ink);
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
  background: var(--surface);
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
  background: var(--brand-primary);
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
