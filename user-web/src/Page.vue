<script setup lang="ts">
import LoginEntry from '@lottery/shared/LoginEntry.vue'
import LoginCredentials from '@lottery/shared/LoginCredentials.vue'
import { vWriteGuard } from "@lottery/shared";
import { useUserI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import SiteIcon from "@lottery/shared/SiteIcon";
import { vRefreshButton } from '@lottery/shared';
import { useSessionReplacement, vGlobalLoading, formatDisplayDateTime } from "@lottery/shared";
import LanguageSwitcher from '@lottery/shared/LanguageSwitcher.vue';
import DetailModal from '@lottery/shared/DetailModal.vue';
import BettingPreferencesSettings from './BettingPreferencesSettings.vue';
import PolicyContent from './PolicyContent.vue';
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import {
  createApiClient,
  defaultBrand,
  applyBrandPresentation,
  nativeLocaleName,
  type Language,
} from "@lottery/shared";
import WithdrawalPanel from "./WithdrawalPanel.vue";
import WalletSummary from "./WalletSummary.vue";
import HeaderPoints from './HeaderPoints.vue';
import UserOverview from './UserOverview.vue';
import { loadUserGames, gamePath, rememberGame, recalledGame, type UserGame } from './user-games';
import RechargePanel from "./RechargePanel.vue";
import ConfiguredBetting from './ConfiguredBetting.vue';
import BettingPanel from "./BettingPanel.vue";
import DrawArchive from './DrawArchive.vue';
import NotificationsPanel from "./NotificationsPanel.vue";
import JoinCodesPanel from "./JoinCodesPanel.vue";
import { setUserLocale, useUserI18n } from "./i18n";
import { parentUserPage } from './navigation';
import { resolveJoinLink, joinLinkFields, joinLinkKey, type JoinLink } from './join-attribution';
import {
  createAuthClient,
  type AuthChallenge,
  type AuthFeatures,
  type TelegramChallenge,
} from "../../shared/src/auth";
import {
  ApiError,
  discardAccessToken,
  isBrandJoinRequired,
  isTelegramClientIdConfigured,
  loadTelegramLoginSdk,
  normalizeIdentifier,
  passwordByteLength,
  requestTelegramIdToken,
  type AuthProfile,
} from "./auth";

const route = useRoute();
const walletBrandCode = import.meta.env.VITE_BRAND_CODE || undefined;
const router = useRouter();
const i18n = useUserI18n();
const localeCode = i18n.locale;
const locale = computed<Language>(() => localeCode.value === "zh-CN" ? "zh" : "en");
const ui = i18n.t;
function setLocale(value: string) { setUserLocale(i18n, value === "zh" ? "zh-CN" : value); }
function languageLabel(code: Language) { return nativeLocaleName(code === "en" ? "en" : "zh-CN"); }
const api = createApiClient({
  brandCode: import.meta.env.VITE_BRAND_CODE || undefined,
});
const authApi = createAuthClient({
  brandCode: import.meta.env.VITE_BRAND_CODE || undefined,
});
const connection = ref<"checking" | "connected" | "offline">("checking");
const connectionMessage = ref(ui('user.page.checking'));
const brandName = ref(defaultBrand.name);
const brandLogoText = ref(defaultBrand.logoText);
const brandLogoUrl = ref<string | null>(null);
const brandLogoFailed = ref(false);
const availableLanguages = ref<Language[]>([]);
const contextLoaded = ref(false);
const brandContent = ref(defaultBrand.content!);
const brandPaused = ref(false);
const notice = ref("");
const agreed = ref(false);
const privacyAgreed = ref(false);
const serviceTermsAgreed = ref(false);
const policiesAgreed = computed({ get: () => privacyAgreed.value && serviceTermsAgreed.value, set: value => { privacyAgreed.value = value; serviceTermsAgreed.value = value; } });
const policyDialog = ref<'privacy' | 'terms' | null>(null);
const joinTermsAgreed = ref(false);
const joinTermsPrompt = ref(false);
const joinAttributionFixed = ref(false);
const activeJoinLink = ref<JoinLink | null>(null);
const joinLinkBrand = ref('');
const joinLinkChecking = ref(true);
const joinLinkFailed = ref(false);
const joinLinkUnavailable = ref(false);
let joinLinkGeneration = 0;
const pendingBrandJoin = ref<{
  identifier: string;
  password: string;
  source: "none" | "agent" | "referral";
  fields: { agent_code?: string; referral_code?: string };
} | null>(null);
const authBusy = ref(false);
const authError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const identifier = ref("");
const password = ref("");
const authProfile = ref<AuthProfile | null>(null);
const headerPoints = ref<{ brandId: string; memberId: string; amount: string; description: string } | null>(null);
const profilePoints = computed(() => headerPoints.value?.brandId === authProfile.value?.member.brand_id && headerPoints.value?.memberId === authProfile.value?.member.id ? headerPoints.value : null);
let profileReadGeneration = 0;
const notificationUnreadCount = ref<string | null>(null);
const profileLoading = ref(true);
const profileSaving = ref(false);
const profileUsername = ref("");
const profilePhone = ref("");
const profileError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const profileNotice = ref("");
const termsVersions = ref<{ privacy_policy_version: string; service_terms_version: string } | null>(null);
const authConfiguration = ref<AuthFeatures>({
  captcha_enabled: false,
  telegram_enabled: false,
});
const authConfigurationLoaded = ref(false);
const authConfigurationLoading = ref(false);
const captchaChallenge = ref<AuthChallenge | null>(null);
const captchaAnswer = ref("");
const captchaLoading = ref(false);
const telegramChallenge = ref<TelegramChallenge | null>(null);
const telegramSdkReady = ref(false);
const telegramError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const tick = ref(0);
const captchaImageSource = computed(() =>
  captchaChallenge.value
    ? `data:image/svg+xml;base64,${encodeBase64(captchaChallenge.value.svg)}`
    : "",
);
const captchaExpired = computed(() => {
  tick.value;
  return (
    !captchaChallenge.value ||
    Date.parse(captchaChallenge.value.expires_at) <= Date.now()
  );
});
const captchaReady = computed(
  () =>
    authConfigurationLoaded.value &&
    (!authConfiguration.value.captcha_enabled ||
      (!captchaLoading.value &&
        !captchaExpired.value &&
        captchaAnswer.value.trim().length > 0)),
);
const telegramReady = computed(() => {
  tick.value;
  return (
    authConfigurationLoaded.value &&
    authConfiguration.value.telegram_enabled &&
    isTelegramClientIdConfigured(authConfiguration.value.telegram_client_id) &&
    telegramSdkReady.value &&
    Boolean(telegramChallenge.value?.id && telegramChallenge.value.nonce) &&
    Date.parse(telegramChallenge.value?.expires_at ?? "") > Date.now()
  );
});
let workspaceSyncTimer: ReturnType<typeof setInterval> | undefined;
let tickTimer: ReturnType<typeof setInterval> | undefined;
let pageGeneration = 0;
onUnmounted(() => {
  pageGeneration++;
  joinLinkGeneration++;
  if (tickTimer) clearInterval(tickTimer);
  if (workspaceSyncTimer) clearInterval(workspaceSyncTimer);
});

const t = computed(() => ({
  home: ui('user.page.home'),
  orders: ui('user.page.orders'),
  wallet: ui('user.page.wallet'),
  results: ui('user.page.results'),
  notifications: ui('user.page.notifications'),
  help: ui('user.page.help'),
  login: ui('user.page.login'),
  register: ui('user.page.register'),
  games: ui('user.page.games'),
  play: ui('user.page.play'),
  pointsPayments: ui('user.page.pointsPayments'),
  online: ui('user.page.online'),
  offline: ui('user.page.offline'),
  checking: ui('user.page.checking'),
  welcome: ui('user.page.welcome'),
  subtitle: ui('user.page.subtitle'),
  featured: ui('user.page.featured'),
  viewAll: ui('user.page.viewAll'),
  announcement: ui('user.page.announcement'),
  announcementBody: ui('user.page.announcementBody'),
  choose: ui('user.page.choose'),
  notices: ui('user.page.notices'),
  helpTitle: ui('user.page.helpTitle'),
  loginTitle: ui('user.page.loginTitle'),
  registerTitle: ui('user.page.registerTitle'),
  username: ui('user.page.username'),
  password: ui('user.page.password'),
  telegram: ui('user.page.telegram'),
  verification: ui('user.page.verification'),
  terms: ui('user.page.terms'),
  continue: ui('user.page.continue'),
  invites: ui('user.page.invites'),
  joinSourceLabel: ui('user.page.joinSourceLabel'),
  joinSourceNone: ui('user.page.joinSourceNone'),
  joinSourceAgent: ui('user.page.joinSourceAgent'),
  joinSourceReferral: ui('user.page.joinSourceReferral'),
  joinCodeLabel: ui('user.page.joinCodeLabel'),
  joinSourceRequired: ui('user.page.joinSourceRequired'),
  joinSourceReview: ui('user.page.joinSourceReview'),
  joinSourceNoCode: ui('user.page.joinSourceNoCode'),
}));
const localizedBrandContent = computed(() => brandContent.value[locale.value] ?? { tagline: "", announcement: "" });
const displayTagline = computed(() => localizedBrandContent.value.tagline);
const displayAnnouncement = computed(() => localizedBrandContent.value.announcement);
const displayLogoUrl = computed(() => brandLogoFailed.value ? null : brandLogoUrl.value);
const longLogoText = computed(() => Array.from(brandLogoText.value).length > 3);
function encodeBase64(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}
watch(locale, (value) => {
  document.documentElement.lang = value === "zh" ? "zh-CN" : "en";
});

const nav = computed(() => [
  { to: "/", label: t.value.home, icon: "home" as const },
  { to: "/orders", label: t.value.orders, icon: "ticket" as const },
  { to: "/wallet", label: t.value.wallet, icon: "wallet" as const },
  { to: "/results", label: t.value.results, icon: "calendar" as const },
  { to: "/notifications", label: t.value.notifications, icon: "bell" as const },
  { to: "/invites", label: t.value.invites, icon: "link" as const },
  { to: "/help", label: t.value.help, icon: "help" as const },
]);
const mobileNav = computed(() => [
  { to: '/', label: ui('user.nav.home'), icon: "home" as const },
  { to: '/orders', label: ui('user.nav.orders'), icon: "ticket" as const },
  { to: '/wallet', label: ui('user.nav.wallet'), icon: "wallet" as const },
  { to: '/account', label: ui('user.nav.personal'), icon: "user" as const },
]);
const desktopNav = computed(() => [...nav.value, mobileNav.value[3]!]);
const accountTools = computed(() => nav.value.filter(item => ['/invites', '/help'].includes(item.to)));
const helpFaqs = [
  ['user.help.bettingQuestion', 'user.help.bettingAnswer'],
  ['common.numberPicksQuestion', 'common.numberPicksAnswer'],
  ['user.help.ordersQuestion', 'user.help.ordersAnswer'],
  ['common.pointsQuestion', 'user.help.pointsAnswer'],
  ['user.help.referralQuestion', 'user.help.referralAnswer'],
  ['common.withdrawalQuestion', 'common.withdrawalAnswer'],
] as const;
const bettingGameTitle = ref('');
const pageTitle = computed(() => {
  const path = route.path;
  const item = nav.value.find(item => item.to === path);
  if (item) return item.label;
  if (path === '/account') return ui('user.nav.personal');
  if (path === '/login') return t.value.login;
  if (path === '/register') return ui('user.auth.registerAction');
  if (path === '/terms') return ui('common.serviceTerms');
  if (path === '/privacy') return ui('common.privacyPolicy');
  if (path === '/recharge') return ui('user.recharge.title');
  if (path === '/withdraw') return ui('user.withdrawal.title');
  if (path === '/wallet/buckets') return ui('user.wallet.buckets');
  if (path === '/wallet/ledger') return ui('user.wallet.ledger');
  if (path.startsWith('/orders/')) return t.value.orders;
  if (path === '/games') return t.value.games;
  if (path.startsWith('/configured/')) return bettingGameTitle.value;
  if (path.startsWith('/games/') || path === '/bet/confirm') return t.value.choose;
  throw new Error(`Unsupported user page: ${path}`);
});
const isAuth = computed(() => ["/login", "/register"].includes(route.path));

const allUserGames = ref<UserGame[]>([]);
const chosenGame = ref<UserGame | null>(null);
const gamesError = ref<string | import("@lottery/shared").LocalizedMessage>("");
const gamesReady = ref(false);
const gamePickerOpen = ref(false);
const menuOpen = ref(false);
const overviewOpen = ref(false);
const currentGame = computed(() => {
  const id = route.params.gameId ?? (route.path === '/results' ? route.query.platform_game_id ?? route.query.game_id : undefined);
  return id ? allUserGames.value.find(game => game.id === id) ?? null : chosenGame.value;
});
const resultsGame = computed(() => {
  if (route.query.platform_game_id !== undefined) return { id: typeof route.query.platform_game_id === 'string' ? route.query.platform_game_id : '', kind: 'configured' as const };
  if (route.query.game_id !== undefined) return { id: typeof route.query.game_id === 'string' ? route.query.game_id : '', kind: 'standard' as const };
  return currentGame.value;
});
async function loadWorkspaceGames() {
  const profile = authProfile.value;
  if (!profile) return;
  gamesError.value = '';
  try {
    const games = await loadUserGames(walletBrandCode);
    if (authProfile.value !== profile) return;
    allUserGames.value = games;
    const routeGame = games.find(g => g.id === String(route.params.gameId ?? ''));
    const stored = localStorage.getItem(`lottery:last-game:${joinLinkBrand.value}`);
    const saved = recalledGame(localStorage, joinLinkBrand.value, games);
    if (routeGame) chosenGame.value = routeGame;
    else if (saved) chosenGame.value = saved;
    else if (stored !== null) chosenGame.value = null;
    else chosenGame.value = games[0] ?? null;
    if (chosenGame.value) rememberGame(localStorage, joinLinkBrand.value, chosenGame.value);
    gamesReady.value = true;
  } catch { gamesError.value = ui('workspace.gamesFailed'); }
}
function selectWorkspaceGame(game: UserGame) {
  chosenGame.value = game;
  rememberGame(localStorage, joinLinkBrand.value, game);
  gamePickerOpen.value = false; gamesError.value = '';
  if (route.path === '/results') void router.push({ path: '/results', query: game.kind === 'configured' ? { platform_game_id: game.id } : { game_id: game.id } });
  else if (route.path !== '/') void router.push(gamePath(game));
}
function workspaceParent() {
  menuOpen.value = false; gamePickerOpen.value = false;
  void router.push(['/orders','/wallet','/account'].includes(route.path) ? '/' : parentUserPage(route.path));
}
watch(() => [contextLoaded.value, isAuth.value, authProfile.value], () => {
  if (contextLoaded.value && !isAuth.value && authProfile.value && !gamesReady.value) void withGlobalLoading(loadWorkspaceGames);
});
watch(() => route.fullPath, () => {
  menuOpen.value = false; gamePickerOpen.value = false;
  const game = allUserGames.value.find(g => g.id === String(route.params.gameId ?? ''));
  if (game) { chosenGame.value = game; rememberGame(localStorage, joinLinkBrand.value, game); }
});

const activePage = computed(() => route.path);
function isNavItemActive(path: string, includeAccountTools = true) {
  return activePage.value === path ||
    (includeAccountTools && path === '/account' && accountTools.value.some(item => item.to === activePage.value)) ||
    (path === '/' && (activePage.value === '/games' || activePage.value.startsWith('/games/') || activePage.value.startsWith('/configured/') || activePage.value === '/bet/confirm')) ||
    (path === "/games" && activePage.value.startsWith("/games/")) ||
    (path === "/orders" && activePage.value.startsWith("/orders/")) ||
    (path === "/wallet" && (activePage.value.startsWith("/wallet/") || ['/recharge', '/withdraw'].includes(activePage.value)));
}
const isBettingRoute = computed(
  () =>
    route.path === "/games" ||
    route.path.startsWith("/games/") ||
    route.path === "/bet/confirm" ||
    route.path === "/orders" ||
    route.path.startsWith("/orders/"),
);
function authTerms() {
  if (!termsVersions.value) throw new Error('Policy versions have not loaded');
  return { ...termsVersions.value };
}
async function refreshCaptcha() {
  if (
    !authConfigurationLoaded.value ||
    !authConfiguration.value.captcha_enabled ||
    captchaLoading.value
  )
    return;
  captchaLoading.value = true;
  captchaChallenge.value = null;
  captchaAnswer.value = "";
  try {
    captchaChallenge.value = await authApi.getChallenge();
  } catch (error) {
    authError.value = publicErrorMessage(error);
  } finally {
    captchaLoading.value = false;
  }
}
async function loadAuthFeatures() {
  if (authConfigurationLoading.value) return;
  authConfigurationLoading.value = true;
  authConfigurationLoaded.value = false;
  try {
    authConfiguration.value = await authApi.getAuthFeatures();
    authConfigurationLoaded.value = true;
    authError.value = "";
    if (authConfiguration.value.captcha_enabled && isAuth.value)
      await refreshCaptcha();
    if (
      authConfiguration.value.telegram_enabled &&
      isTelegramClientIdConfigured(authConfiguration.value.telegram_client_id)
    ) {
      try {
        await loadTelegramLoginSdk();
        telegramSdkReady.value = true;
        await refreshTelegramChallenge();
      } catch (error) {
        telegramSdkReady.value = false;
        telegramChallenge.value = null;
        telegramError.value = publicErrorMessage(error);
      }
    } else {
      telegramSdkReady.value = false;
      telegramChallenge.value = null;
      telegramError.value = "";
    }
  } catch (error) {
    authError.value = publicErrorMessage(error);
  } finally {
    authConfigurationLoading.value = false;
  }
}
async function refreshTelegramChallenge() {
  if (
    !authConfiguration.value.telegram_enabled ||
    !isTelegramClientIdConfigured(authConfiguration.value.telegram_client_id)
  )
    return;
  telegramChallenge.value = null;
  try {
    telegramChallenge.value = await authApi.getTelegramChallenge();
    telegramError.value = "";
  } catch (error) {
    telegramError.value = publicErrorMessage(error);
  }
}
function captchaInput() {
  return authConfiguration.value.captcha_enabled && captchaChallenge.value
    ? {
        captcha_id: captchaChallenge.value.id,
        captcha_answer: captchaAnswer.value.trim(),
      }
    : {};
}
async function refreshJoinLink() {
  const generation = ++joinLinkGeneration;
  joinLinkChecking.value = true;
  joinLinkFailed.value = false;
  joinLinkUnavailable.value = false;
  activeJoinLink.value = null;
  try {
    const result = await resolveJoinLink(window.localStorage, joinLinkBrand.value, new URLSearchParams(window.location.search), async link => {
      const valid = await authApi.validateJoinCode(joinLinkFields(link));
      if (generation !== joinLinkGeneration) throw new Error('Invitation check was superseded');
      return valid;
    });
    if (generation === joinLinkGeneration) {
      activeJoinLink.value = result.link;
      joinLinkUnavailable.value = result.unavailable;
    }
  } catch {
    if (generation === joinLinkGeneration) joinLinkFailed.value = true;
  } finally {
    if (generation === joinLinkGeneration) joinLinkChecking.value = false;
  }
}
function captureJoinChoice() {
  if (joinLinkChecking.value || joinLinkFailed.value) throw new Error(ui('user.auth.joinLinkFailed'));
  if (joinLinkUnavailable.value) throw new Error(ui('user.auth.joinLinkUnavailable'));
  return { source: activeJoinLink.value?.kind ?? 'none' as 'none' | 'agent' | 'referral', fields: joinLinkFields(activeJoinLink.value) };
}
function passwordError() {
  const bytes = passwordByteLength(password.value);
  if (bytes < 6 || bytes > 30) return ui('user.auth.passwordBytes');
  return "";
}
function handleAuthError(error: unknown) {
  if (isBrandJoinRequired(error)) {
    joinAttributionFixed.value = false;
    const pending = pendingBrandJoin.value;
    if (pending) {
      identifier.value = pending.identifier;
      password.value = pending.password;
    }
    joinTermsPrompt.value = true;
    joinTermsAgreed.value = false;
    authError.value = ui('user.auth.joinTerms');
    return;
  }
  authError.value = publicErrorMessage(error);
}
async function finishAuth(
  session: import("../../shared/src/auth").AuthSession,
) {
  pendingBrandJoin.value = null;
  joinTermsPrompt.value = false;
  joinAttributionFixed.value = false;
  password.value = "";
  authProfile.value = discardAccessToken(session);
  await router.replace("/");
}
async function authSubmit() {
  sessionReplaced.value = false;
  if (joinTermsPrompt.value) return;
  authError.value = "";
  joinAttributionFixed.value = false;
  joinTermsPrompt.value = false;
  if (!authConfigurationLoaded.value) {
    authError.value = ui('user.auth.settingsUnavailable');
    return;
  }
  const normalized = normalizeIdentifier(identifier.value);
  if (!normalized.identifier) {
    authError.value = ui('user.auth.identifier');
    return;
  }
  if (!password.value) {
    authError.value = ui('user.auth.passwordRequired');
    return;
  }
  const lengthError = passwordError();
  if (lengthError) {
    authError.value = lengthError;
    return;
  }
  if (!captchaReady.value) {
    authError.value = ui('user.auth.captcha');
    return;
  }
  if (
    route.path === "/register" &&
    (!privacyAgreed.value || !serviceTermsAgreed.value)
  ) {
    authError.value = ui('user.auth.acceptPolicies');
    return;
  }
  let choice: ReturnType<typeof captureJoinChoice>;
  try {
    choice = captureJoinChoice();
  } catch (error) {
    authError.value = error instanceof Error ? publicErrorMessage(error) : t.value.joinSourceRequired;
    return;
  }
  if (route.path === "/login") {
    pendingBrandJoin.value = {
      identifier: normalized.identifier,
      password: password.value,
      ...choice,
    };
  }
  authBusy.value = true;
  try {
    const session =
      route.path === "/register"
        ? await authApi.register({
            ...authTerms(),
            ...captchaInput(),
            ...choice.fields,
            password: password.value,
            ...(normalized.phone
              ? { phone: normalized.phone }
              : { username: normalized.username }),
          })
        : await authApi.login({
            identifier: normalized.identifier,
            password: password.value,
            ...captchaInput(),
          });
    await finishAuth(session);
  } catch (error) {
    if (isBrandJoinRequired(error) && route.path === "/login") handleAuthError(error);
    else if (route.path === "/register" && error instanceof ApiError && error.code === "IDENTITY_EXISTS")
      authError.value = ui('user.auth.accountExists');
    else authError.value = publicErrorMessage(error);
    await refreshCaptcha();
  } finally {
    authBusy.value = false;
  }
}
async function acceptBrandTermsAndLogin() {
  if (!joinTermsAgreed.value) return;
  if (!authConfigurationLoaded.value) {
    authError.value = ui('user.auth.settingsUnavailable');
    return;
  }
  if (!captchaReady.value) {
    authError.value = ui('user.auth.captchaRetry');
    return;
  }
  const pending = pendingBrandJoin.value;
  if (!pending) {
    authError.value = ui('user.auth.signInAgain');
    joinTermsPrompt.value = false;
    return;
  }
  authError.value = "";
  authBusy.value = true;
  try {
    const session = await authApi.login({
      identifier: pending.identifier,
      password: pending.password,
      ...authTerms(),
      ...captchaInput(),
      ...pending.fields,
    });
    joinTermsPrompt.value = false;
    await finishAuth(session);
  } catch (error) {
    if (
      error instanceof ApiError &&
      error.code?.toUpperCase() === "JOIN_ATTRIBUTION_FIXED"
    ) {
      joinAttributionFixed.value = true;
      authError.value = ui('user.auth.joinFixed');
    } else if (isBrandJoinRequired(error)) {
      handleAuthError(error);
    } else {
      authError.value = publicErrorMessage(error);
    }
    await refreshCaptcha();
  } finally {
    authBusy.value = false;
  }
}
async function useDirectEntry() {
  try {
    window.localStorage.removeItem(joinLinkKey(joinLinkBrand.value));
  } catch {
    joinLinkFailed.value = true;
    return;
  }
  activeJoinLink.value = null;
  joinLinkUnavailable.value = false;
  const query = {...route.query};
  delete query.agent_code;
  delete query.referral_code;
  await router.replace({path: route.path, query});
}
function cancelBrandJoin(): void {
  pendingBrandJoin.value = null;
  joinTermsPrompt.value = false;
  joinTermsAgreed.value = false;
  joinAttributionFixed.value = false;
  void useDirectEntry();
  authError.value = "";
}
async function startTelegramAuth() {
  sessionReplaced.value = false;
  if (!telegramReady.value || authBusy.value || joinLinkChecking.value || joinLinkFailed.value || joinLinkUnavailable.value) return;
  if (!privacyAgreed.value || !serviceTermsAgreed.value) {
    authError.value = ui('user.auth.acceptTelegramPolicies');
    return;
  }
  authBusy.value = true;
  authError.value = "";
  try {
    const choice = route.path === '/register' ? captureJoinChoice() : { source: 'none' as const, fields: {} };
    const challenge = telegramChallenge.value!;
    const idToken = await requestTelegramIdToken(
      authConfiguration.value.telegram_client_id!,
      challenge.nonce,
    );
    const session = await authApi.telegram({
      id_token: idToken,
      challenge_id: challenge.id,
      nonce: challenge.nonce,
      ...authTerms(),
      ...choice.fields,
    });
    await finishAuth(session);
  } catch (error) {
    telegramError.value = publicErrorMessage(error);
    await refreshTelegramChallenge();
  } finally {
    authBusy.value = false;
  }
}
async function loadProfile() {
  const requestGeneration = ++profileReadGeneration;
  const mountedGeneration = pageGeneration;
  profileLoading.value = true;
  profileError.value = "";
  profileNotice.value = "";
  try {
    const profile = await authApi.me();
    if (requestGeneration !== profileReadGeneration || mountedGeneration !== pageGeneration) return;
    authProfile.value = profile;
    profileUsername.value = profile.user.username ?? "";
    profilePhone.value = profile.user.phone ?? "";
  } catch (error) {
    if (requestGeneration !== profileReadGeneration || mountedGeneration !== pageGeneration) return;
    if (error instanceof Error && "status" in error && error.status === 401) {
      authProfile.value = null;
      profileError.value = ui('user.auth.profileSignIn');
      await router.replace({ path: '/login', query: route.query });
    } else profileError.value = publicErrorMessage(error);
  } finally {
    if (requestGeneration === profileReadGeneration && mountedGeneration === pageGeneration) profileLoading.value = false;
  }
}
function expireRechargeProfile() {
  // Do not let an earlier profile GET remount an invalidated recharge scope.
  profileReadGeneration++;
  authProfile.value = null;
  profileLoading.value = false;
  profileUsername.value = "";
  profilePhone.value = "";
}
watch(authProfile, profile => {
  if (profile) return;
  allUserGames.value = [];
  chosenGame.value = null;
  gamesReady.value = false;
  gamePickerOpen.value = false;
  menuOpen.value = false;
  if (!['/login', '/register', '/terms', '/privacy'].includes(route.path)) {
    void router.replace({ path: '/login', query: route.query });
  }
}, { flush: 'sync' });
const sessionReplaced = useSessionReplacement(() => !!authProfile.value, () => authApi.me(), () => {
  expireRechargeProfile();
  void router.replace('/login');
});
async function saveProfile() {
  profileError.value = "";
  profileNotice.value = "";
  const next: { username?: string; phone?: string } = {};
  if (!authProfile.value?.user.username && profileUsername.value.trim())
    next.username = profileUsername.value.trim().toLowerCase();
  if (!authProfile.value?.user.phone && profilePhone.value.trim()) {
    const normalized = normalizeIdentifier(profilePhone.value);
    if (!normalized.phone) {
      profileError.value = ui('user.auth.phone');
      return;
    }
    next.phone = normalized.phone;
  }
  if (!Object.keys(next).length) {
    profileNotice.value = ui('user.auth.profileEmpty');
    return;
  }
  profileSaving.value = true;
  try {
    await authApi.updateProfile(next);
    const profile = await authApi.me();
    authProfile.value = profile;
    profileUsername.value = profile.user.username ?? "";
    profilePhone.value = profile.user.phone ?? "";
    profileNotice.value = ui('user.auth.profileUpdated');
  } catch (error) {
    profileError.value = publicErrorMessage(error);
  } finally {
    profileSaving.value = false;
  }
}
async function logout() {
  profileError.value = "";
  try {
    await authApi.logout();
    authProfile.value = null;
    await router.push("/login");
  } catch (error) {
    profileError.value = publicErrorMessage(error);
  }
}
function closeNotice() {
  notice.value = "";
}
onMounted(() => withGlobalLoading(async () => {
  const generation = ++pageGeneration;
  try {
    const context = await api.getContext();
    if (generation !== pageGeneration) return;
    connection.value = "connected";
    connectionMessage.value = t.value.online;
    if (context?.brand?.name) brandName.value = context.brand.name;
    brandLogoText.value = context.brand.logoText || context.brand.name;
    brandLogoUrl.value = context.brand.logoUrl ?? null;
    brandLogoFailed.value = false;
    brandContent.value = {
      en: context.brand.content?.en ?? { tagline: "", announcement: "" },
      zh: context.brand.content?.zh ?? { tagline: "", announcement: "" },
    };
    availableLanguages.value = context.brand.languages;
    contextLoaded.value = true;
    brandPaused.value = context.paused;
    if (!context.terms) throw new Error('Missing policy versions');
    termsVersions.value = context.terms;
    const requestedLanguage: Language = localeCode.value === "zh-CN" ? "zh" : "en";
    const selectedLanguage = context.brand.languages.includes(requestedLanguage)
      ? requestedLanguage
      : context.brand.defaultLanguage;
    setUserLocale(i18n, selectedLanguage === "zh" ? "zh-CN" : "en");
    applyBrandPresentation(context);
    if (!context.brand.id) throw new Error('Missing brand identity');
    joinLinkBrand.value = context.brand.id;
    await refreshJoinLink();
    await loadAuthFeatures();
    if (generation !== pageGeneration) return;
  } catch (error) {
    if (generation !== pageGeneration) return;
    connection.value = "offline";
    connectionMessage.value = publicErrorI18n.errorText(error);
  }
  if (!isAuth.value) void loadProfile();
  tickTimer = setInterval(() => tick.value++, 1000);
  workspaceSyncTimer = setInterval(() => { if (authProfile.value && !isAuth.value && document.visibilityState === 'visible') void loadWorkspaceGames(); }, 5000);
}));
watch(locale, () => {
  connectionMessage.value =
    connection.value === "connected"
      ? t.value.online
      : connection.value === "offline"
        ? t.value.offline
        : t.value.checking;
});
watch(() => route.fullPath, () => { if (joinLinkBrand.value) void refreshJoinLink(); });
watch(
  () => route.path,
  (path) => {
    policyDialog.value = null;
    bettingGameTitle.value = '';
    if (path !== "/login") {
      pendingBrandJoin.value = null;
      joinTermsPrompt.value = false;
      joinTermsAgreed.value = false;
      joinAttributionFixed.value = false;
    }
    if (path !== '/login' && path !== '/register') void loadProfile();
    if (path === "/login" || path === "/register") {
      if (authConfigurationLoaded.value) void refreshCaptcha();
      else void loadAuthFeatures();
    } else {
      captchaChallenge.value = null;
      captchaAnswer.value = "";
    }
  },
);
</script>

<template>
  <GlobalLoadingOverlay :active="!isAuth && (profileLoading || profileSaving)" :label="route.path === '/account' && profileLoading ? ui('common.loadingAccount') : profileSaving ? ui('common.saving') : ui('common.loading')" />
  <LoginEntry v-if="isAuth" :brand-name="brandName" :logo-text="brandLogoText" :logo-url="displayLogoUrl" :title="pageTitle" title-id="user-login-title" :locale="locale" :language-label="ui('common.language')" :languages="contextLoaded ? availableLanguages.map(code => ({code,label:languageLabel(code)})) : []" @update:locale="setLocale" @logo-error="brandLogoFailed = true">
    <template #status>
      <div v-if="connection === 'offline' || brandPaused" class="connection-alert" role="status"><strong class="footer-online">{{brandPaused ? ui('common.brandPaused') : t.offline}}</strong><span>{{brandPaused ? ui('common.brandUnavailable') : connectionMessage}}</span></div>
      <div v-if="notice" class="toast" role="status"><span>{{notice}}</span><button @click="closeNotice" :aria-label="ui('common.dismissMessage')"><SiteIcon name="close" /></button></div>
    </template>
            <form v-write-guard="authBusy" class="auth-form" novalidate :aria-describedby="authError || sessionReplaced ? 'auth-feedback' : undefined" @submit.prevent="withGlobalLoading(authSubmit)">
              <LoginCredentials v-model:identifier="identifier" v-model:password="password" :identifier-label="ui('user.auth.accountLabel')" :password-label="t.password" :readonly="joinTermsPrompt" :password-maxlength="128" />
              <template v-if="route.path === '/register' || joinTermsPrompt || joinLinkUnavailable || joinLinkFailed">
                <p v-global-loading v-if="joinLinkChecking" class="tiny-note" role="status">{{ ui('user.auth.joinLinkChecking') }}</p>
                <div v-if="joinLinkUnavailable" class="auth-error" role="alert">{{ ui('user.auth.joinLinkUnavailable') }}<button type="button" class="captcha-refresh" @click="withGlobalLoading(useDirectEntry)">{{ ui('user.auth.useDirectEntry') }}</button></div>
                <div v-if="joinLinkFailed" class="auth-error" role="alert">{{ ui('user.auth.joinLinkFailed') }}<button type="button" class="captcha-refresh" @click="withGlobalLoading(refreshJoinLink)">{{ ui('user.auth.joinLinkRetry') }}</button></div>
              </template>
              <div v-if="authConfiguration.captcha_enabled" class="captcha-box">
                <div class="captcha-heading">
                  <strong>{{
                    ui("common.securityCheck")
                  }}</strong
                  ><button v-refresh-button="captchaLoading ? ui('common.loading') : ui('common.refreshImage')"
                    type="button"
                    class="captcha-refresh"
                    :disabled="captchaLoading"
                    @click="withGlobalLoading(refreshCaptcha)"
                  >
                    {{
                      captchaLoading ? ui("common.loading") : ui("common.refreshImage")
                    }}
                  </button>
                </div>
                <img
                  v-if="captchaImageSource"
                  class="captcha-image"
                  :src="captchaImageSource"
                  :alt="
                    ui('common.verificationImage')
                  "
                />
                <p v-global-loading v-else-if="captchaLoading" class="muted">
                  {{
                    ui("common.loadingVerification")
                  }}
                </p>
                <label
                  >{{
                    ui("common.enterVerification")
                  }}<input
                    v-model="captchaAnswer"
                    required
                    autocomplete="off"
                    inputmode="text"
                    maxlength="16"
                /></label>
                <small>{{
                  ui("common.captchaExpiry", { expires: formatDisplayDateTime(captchaChallenge?.expires_at, localeCode) })
                }}</small>
              </div>
              <template
                v-if="
                  route.path === '/register' ||
                  (authConfiguration.telegram_enabled &&
                    isTelegramClientIdConfigured(
                      authConfiguration.telegram_client_id,
                    ))
                "
              >
                <label class="check-row combined-terms"
                  ><input v-model="policiesAgreed" type="checkbox" /><span>{{
                    ui("common.accept")
                  }}
                  <a href="/privacy" class="policy-trigger" aria-haspopup="dialog" @click.stop.prevent="policyDialog = 'privacy'">{{
                    ui("common.privacyPolicy")
                  }}</a>
                  {{ ui('user.auth.and') }}
                  <a href="/terms" class="policy-trigger" aria-haspopup="dialog" @click.stop.prevent="policyDialog = 'terms'">{{
                    ui("common.serviceTerms")
                  }}</a>
                  </span>
                </label>
              </template>
              <div
                v-if="route.path === '/login' && joinTermsPrompt"
                class="join-terms"
              >
                <p>
                  {{
                    ui("common.existingAccountTerms")
                  }}
                </p>
                <div class="join-source-review">
                  <strong>{{ t.joinSourceReview }}</strong>
                  <span v-if="pendingBrandJoin?.source === 'none'">{{ t.joinSourceNoCode }}</span>
                  <span v-else>{{ pendingBrandJoin?.source === 'agent' ? t.joinSourceAgent : t.joinSourceReferral }} · {{ pendingBrandJoin?.fields.agent_code || pendingBrandJoin?.fields.referral_code }}</span>
                </div>
                <label class="check-row"
                  ><input v-model="joinTermsAgreed" type="checkbox" />{{
                    ui("common.accept")
                  }}
                  <a href="/terms" class="policy-trigger" aria-haspopup="dialog" @click.stop.prevent="policyDialog = 'terms'">{{
                    ui("common.joinTermsLink")
                  }}</a>
                  <span
                    >({{ termsVersions?.privacy_policy_version }} ·
                    {{ termsVersions?.service_terms_version }})</span
                  ></label
                >
                <button
                  v-write-guard="authBusy"
                  type="button"
                  class="button button-secondary full-button"
                :disabled="!joinTermsAgreed || authBusy || !captchaReady || joinLinkChecking || joinLinkFailed || joinLinkUnavailable"
                  @click="withGlobalLoading(acceptBrandTermsAndLogin)"
                >
                  {{
                    ui("common.acceptAndJoin")
                  }}
                  <span>→</span>
                </button>
              </div>
              <p v-if="sessionReplaced" id="auth-feedback" class="auth-error" role="alert">{{ ui('auth.sessionReplaced') }}</p>
              <p v-else-if="authError" id="auth-feedback" class="auth-error" role="alert">
                {{ displayPublicError(authError) }}
              </p>
              <button
                v-if="route.path === '/login' && joinTermsPrompt"
                type="button"
                class="button button-secondary full-button"
                :disabled="authBusy"
                @click="cancelBrandJoin"
              >
                {{
                  joinAttributionFixed
                      ? ui("common.cancelNoCode")
                      : ui("common.cancelJoinChoice")
                }}
              </button>
              <button
                v-if="!authConfigurationLoaded"
                type="button"
                class="button button-secondary full-button"
                :disabled="authConfigurationLoading"
                @click="withGlobalLoading(loadAuthFeatures)"
              >
                {{
                  authConfigurationLoading
                    ? ui("common.loadingSettings")
                    : ui("common.retrySignInSettings")
                }}
              </button>
              <div v-if="!joinTermsPrompt" class="auth-actions" :class="{ 'auth-login-actions': route.path === '/login' }">
              <button
                class="button button-primary full-button"
                type="submit"
                :disabled="authBusy || !authConfigurationLoaded || captchaLoading || joinLinkChecking || joinLinkFailed || joinLinkUnavailable"
              >
                {{
                  authBusy
                    ? ui("common.pleaseWait")
                    : route.path === '/login' ? ui('user.auth.loginAction') : ui('user.auth.registerAction')
                }}
              </button>
              <RouterLink v-if="route.path === '/login'" to="/register" class="button button-secondary auth-register-button">{{ ui('user.auth.registerAction') }}</RouterLink>
              <RouterLink v-else to="/login" class="button button-secondary auth-login-button">{{ ui('user.auth.loginAction') }}</RouterLink>
              </div>
              <button
                v-if="telegramReady"
                class="button button-secondary full-button"
                type="button"
                :disabled="authBusy || !privacyAgreed || !serviceTermsAgreed || joinTermsPrompt || joinLinkChecking || joinLinkFailed || joinLinkUnavailable"
                v-write-guard="authBusy" @click="withGlobalLoading(startTelegramAuth)"
              >
                {{
                  ui("common.continueTelegram")
                }}
                <SiteIcon name="external" />
              </button>
              <button v-refresh-button="ui('common.refreshTelegram')"
                v-if="
                  authConfiguration.telegram_enabled &&
                  isTelegramClientIdConfigured(
                    authConfiguration.telegram_client_id,
                  ) &&
                  !telegramReady
                "
                class="captcha-refresh"
                type="button"
                :disabled="authBusy"
                @click="withGlobalLoading(refreshTelegramChallenge)"
              >
                {{
                  ui("common.refreshTelegram")
                }}
              </button>
              <p v-if="authConfiguration.telegram_enabled && isTelegramClientIdConfigured(authConfiguration.telegram_client_id) && !telegramReady" class="tiny-note">
                {{
                  ui("common.telegramRequirement")
                }}
              </p>
              <p v-else-if="telegramReady" class="tiny-note">
                {{
                  ui("common.telegramExpiry", { expires: formatDisplayDateTime(telegramChallenge?.expires_at, localeCode) })
                }}
              </p>
              <p v-if="telegramError" class="auth-error" role="alert">
                {{ displayPublicError(telegramError) }}
              </p>
            </form>
  </LoginEntry>
  <div v-else class="app-frame workspace-frame">
    <aside class="workspace-sidebar">
      <RouterLink to="/" class="workspace-sidebar-brand">
        <img v-if="displayLogoUrl" :src="displayLogoUrl" alt="" @error="brandLogoFailed = true" />
        <strong>{{ brandName }}</strong>
      </RouterLink>
      <nav class="side-nav" :aria-label="ui('user.nav.primary')">
        <RouterLink v-for="item in desktopNav" :key="item.to" :to="item.to" :class="{ active: isNavItemActive(item.to, false) }" :aria-current="isNavItemActive(item.to, false) ? 'page' : undefined">
          <span class="nav-icon" aria-hidden="true"><SiteIcon :name="item.icon" /></span>
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>
    </aside>
    <div class="main-column">
      <header v-if="!isAuth" class="workspace-topbar" :class="{ 'workspace-guest': !profileLoading && !authProfile }">
        <div class="workspace-leading"><span v-if="route.path === '/'" class="workspace-brand">{{ brandName }}</span><button v-else class="icon-button" type="button" :aria-label="ui('user.nav.back')" @click="workspaceParent"><SiteIcon name="back" /></button></div>
        <div class="workspace-game-heading"><button type="button" class="workspace-game-trigger" :aria-label="ui('workspace.games')" :aria-expanded="gamePickerOpen" @click="gamePickerOpen = !gamePickerOpen; menuOpen = false"><strong>{{ currentGame?.name ?? ui('workspace.games') }}</strong><SiteIcon name="down" /></button><div id="user-game-help"></div></div>
        <RouterLink v-if="!profileLoading && !authProfile" to="/login" class="button button-quiet workspace-login">{{ t.login }}</RouterLink>
        <button type="button" class="icon-button" :aria-label="ui('workspace.menu')" :aria-expanded="menuOpen" @click="menuOpen = true; gamePickerOpen = false"><SiteIcon name="menu" /></button>
        <div v-if="gamePickerOpen" class="workspace-game-picker"><button v-for="game in allUserGames" :key="game.id" type="button" :aria-pressed="currentGame?.id === game.id" @click="selectWorkspaceGame(game)">{{ game.name }}</button><p v-if="gamesReady && !allUserGames.length" class="empty-state workspace-game-empty">{{ ui('betting.noGames') }}</p></div>
      </header>

      <template v-if="!isAuth">
        <template v-if="authProfile">
        <div v-show="overviewOpen" class="workspace-overview">
          <UserOverview :brand-code="walletBrandCode" :brand-id="authProfile.member.brand_id" :member-id="authProfile.member.id">
            <HeaderPoints :brand-code="walletBrandCode" :brand-id="authProfile.member.brand_id" :member-id="authProfile.member.id" :route-key="route.fullPath" @points-change="headerPoints = $event" @auth-expired="authProfile = null" />
          </UserOverview>
        </div>
        <div class="workspace-overview-control"><button type="button" class="workspace-overview-toggle" :aria-label="ui('workspace.overview')" :aria-expanded="overviewOpen" @click="overviewOpen = !overviewOpen"><SiteIcon name="down" :class="{ expanded: overviewOpen }" /></button></div>
        </template>
        <div v-show="menuOpen" class="workspace-menu-layer" role="dialog" :aria-modal="menuOpen ? true : undefined" :aria-label="ui('workspace.menu')" @keydown.esc="menuOpen = false">
          <button type="button" class="workspace-menu-backdrop" :aria-label="ui('draw.close')" @click="menuOpen = false"></button>
          <aside class="workspace-menu" :aria-label="ui('workspace.menu')"><div class="workspace-menu-heading"><strong>{{ brandName }}</strong><div class="workspace-menu-actions"><div class="workspace-menu-tools"><LanguageSwitcher v-if="contextLoaded && availableLanguages.length > 1" :model-value="locale" :label="ui('common.language')" :options="availableLanguages.map(code => ({code,label:languageLabel(code)}))" @update:model-value="setLocale" /><div id="user-page-refresh" class="user-page-refresh"></div></div><button type="button" class="icon-button workspace-menu-close" :aria-label="ui('draw.close')" @click="menuOpen = false"><SiteIcon name="close" /></button></div></div>
            <RouterLink v-if="currentGame" :to="{path:'/results',query:currentGame.kind === 'standard' ? {game_id:currentGame.id} : {platform_game_id:currentGame.id}}"><SiteIcon name="calendar" />{{ ui('draw.results') }}</RouterLink>
            <RouterLink to="/notifications"><SiteIcon name="bell" />{{ t.notifications }}</RouterLink>
            <RouterLink to="/help"><SiteIcon name="help" />{{ t.help }}</RouterLink>
          </aside>
        </div>
      </template>

      <main id="main-content" class="content" :class="{ 'compact-content': route.path === '/' || route.path.startsWith('/configured/') }">
        <div
          v-if="connection === 'offline' || brandPaused"
          class="connection-alert"
          role="status"
        >
          <strong class="footer-online">{{
            brandPaused
              ? ui("common.brandPaused")
              : t.offline
          }}</strong
          ><span>{{
            brandPaused
              ? ui("common.brandUnavailable")
              : connectionMessage
          }}</span>
        </div>
        <div v-if="notice" class="toast" role="status">
          <span>{{ notice }}</span
          ><button @click="closeNotice" :aria-label="ui('common.dismissMessage')"><SiteIcon name="close" /></button>
        </div>

        <template v-if="route.path === '/'">
          <div v-if="gamesError" class="message error" role="alert"><span>{{ displayPublicError(gamesError) }}</span> <button type="button" class="button button-secondary" @click="withGlobalLoading(loadWorkspaceGames)">{{ ui('betting.retry') }}</button></div>
          <p v-else-if="gamesReady && !currentGame" class="empty-state">{{ ui('betting.noGames') }}</p>
          <component :is="currentGame.kind === 'configured' ? ConfiguredBetting : BettingPanel" v-if="currentGame" :key="currentGame.id" v-bind="currentGame.kind === 'standard' ? { embeddedGameId: currentGame.id } : { gameId: currentGame.id }" :brand-code="walletBrandCode" :locale="locale" :auth="authProfile" @game-title="bettingGameTitle = $event" @auth-expired="authProfile = null" />
        </template>
        <ConfiguredBetting v-else-if="route.path.startsWith('/configured/')" :game-id="String(route.params.gameId)" :brand-code="walletBrandCode" :locale="locale" :auth="authProfile" @game-title="bettingGameTitle=$event" @auth-expired="authProfile=null" />
        <BettingPanel
          v-else-if="isBettingRoute"
          :locale="locale"
          :brand-code="walletBrandCode"
          :auth="authProfile"
          @auth-expired="authProfile = null"
        />


        <section
          v-else-if="route.path === '/invites'"
          class="page-section"
        >
          <JoinCodesPanel
            :locale="locale"
            :brand-name="brandName"
            :brand-code="walletBrandCode"
          />
        </section>

        <section
          v-else-if="route.path === '/account'"
          class="page-section narrow-page account-page"
        >
          <div class="page-heading"><h1>{{ ui('common.accountTitle') }}</h1></div>
          <div v-if="!profileLoading && !authProfile" class="detail-panel">
            <p v-if="profileError" class="auth-error" role="status">{{ displayPublicError(profileError) }}</p>
            <RouterLink to="/login" class="button button-primary"
              >{{ t.login }} <SiteIcon name="next" /> </RouterLink
            >
          </div>
          <div v-else-if="!profileLoading && authProfile" class="detail-panel">
            <div class="account-identity">
              <span class="avatar">{{
                authProfile.user.username?.slice(0, 1).toUpperCase() || "L"
              }}</span>
              <div>
                <strong>{{
                  authProfile.member.display_name ||
                  authProfile.user.username ||
                  authProfile.user.phone
                }}</strong><small>{{ brandName }}</small>
              </div>
            </div>
            <dl class="account-details">
              <div v-if="authProfile.user.username"><dt>{{ ui('user.auth.accountLabel') }}</dt><dd>{{ authProfile.user.username }}</dd></div>
              <div class="account-points"><dt>{{ ui('user.account.points') }}</dt><dd role="status" :aria-label="profilePoints?.description ?? ui('user.wallet.reading')"><strong>{{ profilePoints?.amount ?? '…' }}</strong></dd></div>
              <div v-if="authProfile.user.phone"><dt>{{ ui('common.phone') }}</dt><dd>{{ authProfile.user.phone }}</dd></div>
            </dl>
            <details v-if="!authProfile.user.username || !authProfile.user.phone" class="account-complete">
              <summary>{{ ui('user.account.completeProfile') }}</summary>
              <label v-if="!authProfile.user.username" class="field-label">{{ ui('user.auth.accountLabel') }}<input v-model="profileUsername" autocomplete="username" :placeholder="ui('common.addUsername')" /></label>
              <label v-if="!authProfile.user.phone" class="field-label">{{ ui('common.phone') }}<input v-model="profilePhone" autocomplete="tel" :placeholder="ui('common.addPhone')" /></label>
              <button v-write-guard="profileSaving" type="button" class="button button-primary full-button" :disabled="profileSaving" @click="withGlobalLoading(saveProfile)">{{ profileSaving ? ui('common.saving') : ui('common.saveProfile') }}</button>
            </details>
            <p v-if="profileError" class="auth-error" role="alert">
              {{ displayPublicError(profileError) }}
            </p>
            <p v-if="profileNotice" class="success-note" role="status">
              {{ profileNotice }}
            </p>

          </div>
          <BettingPreferencesSettings :brand-code="walletBrandCode" v-if="authProfile" :brand-id="authProfile.member.brand_id" :member-id="authProfile.member.id" :locale="localeCode" />
          <nav class="account-tools" :aria-label="ui('user.nav.services')">
            <RouterLink v-for="item in accountTools" :key="item.to" :to="item.to"><span class="account-tool-icon" aria-hidden="true"><SiteIcon :name="item.icon" /></span><span>{{ item.label }}</span><SiteIcon name="next" /></RouterLink>
          </nav>
          <button v-write-guard="authBusy" v-if="authProfile" type="button" class="button button-secondary account-signout" :disabled="authBusy" @click="withGlobalLoading(logout)">{{ ui('common.signOut') }}</button>
        </section>

        <section
          v-else-if="route.path === '/terms' || route.path === '/privacy'"
          class="page-section narrow-page"
        >
          <div class="page-heading">
            <div>
              <div class="eyebrow">
                {{
                  route.path === "/terms" ? ui("common.serviceTermsUpper") : ui("common.privacyPolicyUpper")
                }}
                ·
                {{
                  route.path === "/terms"
                    ? termsVersions?.service_terms_version
                    : termsVersions?.privacy_policy_version
                }}
              </div>
              <h1>
                {{
                  route.path === "/terms"
                    ? ui("common.serviceTerms")
                    : ui("common.privacyPolicy")
                }}
              </h1>
            </div>
          </div>
          <div class="detail-panel policy-copy">
            <PolicyContent />
            <RouterLink to="/register" class="text-link"
              >{{
                ui("common.backToAccount")
              }} <SiteIcon name="next" /> </RouterLink
            >
          </div>
        </section>



        <section
          v-else-if="
            route.path === '/wallet' || route.path === '/wallet/buckets' || route.path === '/wallet/ledger'
          "
          class="page-section"
        >
          <WalletSummary :key="route.path" :view="route.path === '/wallet' ? 'overview' : route.path === '/wallet/buckets' ? 'buckets' : 'ledger'" :brand-code="walletBrandCode" :locale="locale" />
        </section>

        <template v-else-if="route.path === '/recharge'">
        <RechargePanel
          v-if="!profileLoading"
          :key="`${authProfile?.member.brand_id ?? 'signed-out'}:${authProfile?.member.id ?? 'signed-out'}`"
          :locale="locale"
          :brand-code="walletBrandCode"
          :member="authProfile?.member ?? null"
          @auth-expired="expireRechargeProfile"
          @sign-in="router.push('/login')"
        />
        </template>

        <template v-else-if="route.path === '/withdraw'">
        <WithdrawalPanel
          v-if="!profileLoading"
          :key="`${authProfile?.member.brand_id ?? 'signed-out'}:${authProfile?.member.id ?? 'signed-out'}`"
          :locale="locale"
          :brand-code="walletBrandCode"
          :member="authProfile?.member ?? null"
          @auth-expired="authProfile = null"
          @context-changed="withGlobalLoading(loadProfile)"
        />
        </template>

        <template v-else-if="route.path === '/results'">
        <DrawArchive v-if="resultsGame" :key="`${resultsGame.kind}:${resultsGame.id}`" :game-id="resultsGame.id" :kind="resultsGame.kind" :brand-code="walletBrandCode" :locale="locale" />
        <p v-else-if="gamesReady" class="empty-state">{{ ui('draw.chooseGame') }}</p>
        </template>

        <NotificationsPanel
          v-else-if="route.path === '/notifications'"
          :locale="locale"
          :brand-code="walletBrandCode"
          @unread-count="notificationUnreadCount = $event"
          @auth-expired="authProfile = null"
        />

<section v-else-if="route.path === '/help'" class="page-section narrow-page help-page" :aria-label="pageTitle">
          <div class="faq-list">
            <details v-for="([question, answer], index) in helpFaqs" :key="question" :open="index === 0">
              <summary>{{ ui(question) }}<SiteIcon name="down" /></summary>
              <p>{{ ui(answer) }}</p>
            </details>
          </div>
          <p class="help-responsible">{{ ui('user.help.responsibleReminder') }}</p>
        </section>
      </main>

      <nav v-if="!isAuth" class="mobile-primary-nav" :aria-label="ui('user.nav.primary')">
        <RouterLink
          v-for="item in mobileNav"
          :key="item.to"
          :to="item.to"
          :class="{ active: isNavItemActive(item.to) }"
          :aria-current="isNavItemActive(item.to) ? 'page' : undefined"
        >
          <span class="mobile-primary-nav-icon" aria-hidden="true">
            <SiteIcon :name="item.icon" />
          </span>
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>
    </div>
  </div>
  <DetailModal v-if="policyDialog" class="policy-modal" :title="policyDialog === 'privacy' ? ui('common.privacyPolicy') : ui('common.serviceTerms')" :close-label="ui('user.auth.closePolicy')" @close="policyDialog = null">
    <PolicyContent />
  </DetailModal>
</template>

<style scoped>
.join-source-review {
  display: grid;
  gap: 6px;
  margin: 10px 0 12px;
  padding: 10px;
  border-radius: 9px;
  background: var(--line-subtle);
  color: var(--ink);
  font-size: 11px;
  overflow-wrap: anywhere;
}
</style>
