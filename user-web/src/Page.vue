<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import {
  createApiClient,
  defaultBrand,
  applyBrandPresentation,
  type Language,
} from "@lottery/shared";
import WithdrawalPanel from "./WithdrawalPanel.vue";
import WalletSummary from "./WalletSummary.vue";
import HomeWalletCard from "./HomeWalletCard.vue";
import RechargePanel from "./RechargePanel.vue";
import BettingPanel from "./BettingPanel.vue";
import DrawResultsPanel from "./DrawResultsPanel.vue";
import NotificationsPanel from "./NotificationsPanel.vue";
import AgentPanel from "./AgentPanel.vue";
import JoinCodesPanel from "./JoinCodesPanel.vue";
import { clearAllAgentUpdates } from "./agent-state";
import {
  createAuthClient,
  normalizeJoinCodeFields,
  type AuthChallenge,
  type AuthFeatures,
  type TelegramChallenge,
} from "../../shared/src/auth";
import {
  ApiError,
  authErrorMessage,
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
const savedLanguage = localStorage.getItem("luma-language");
const validSavedLanguage = savedLanguage === "en" || savedLanguage === "zh" ? savedLanguage : null;
const locale = ref<Language>(validSavedLanguage || "en");
const api = createApiClient({
  brandCode: import.meta.env.VITE_BRAND_CODE || undefined,
});
const authApi = createAuthClient({
  brandCode: import.meta.env.VITE_BRAND_CODE || undefined,
});
const connection = ref<"checking" | "connected" | "offline">("checking");
const connectionMessage = ref("Checking service");
const brandName = ref(defaultBrand.name);
const brandLogoText = ref(defaultBrand.logoText);
const brandLogoUrl = ref<string | null>(null);
const brandLogoFailed = ref(false);
const availableLanguages = ref<Language[]>([]);
const contextLoaded = ref(false);
const brandContent = ref(defaultBrand.content!);
const brandPaused = ref(false);
const mobileMenu = ref(false);
const notice = ref("");
const agreed = ref(false);
const privacyAgreed = ref(false);
const serviceTermsAgreed = ref(false);
const joinTermsAgreed = ref(false);
const joinTermsPrompt = ref(false);
const joinAttributionFixed = ref(false);
const joinSource = ref<"none" | "agent" | "referral">("none");
const joinCode = ref("");
const pendingBrandJoin = ref<{
  identifier: string;
  password: string;
  source: "none" | "agent" | "referral";
  fields: { agent_code?: string; referral_code?: string };
} | null>(null);
const authBusy = ref(false);
const authError = ref("");
const identifier = ref("");
const password = ref("");
const authProfile = ref<AuthProfile | null>(null);
let profileReadGeneration = 0;
const notificationUnreadCount = ref<string | null>(null);
const profileLoading = ref(false);
const profileSaving = ref(false);
const profileUsername = ref("");
const profilePhone = ref("");
const profileError = ref("");
const profileNotice = ref("");
const termsVersions = ref({
  privacy_policy_version: "dev-1",
  service_terms_version: "dev-1",
});
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
const telegramError = ref("");
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
let tickTimer: ReturnType<typeof setInterval> | undefined;
let pageGeneration = 0;
onUnmounted(() => {
  pageGeneration++;
  if (tickTimer) clearInterval(tickTimer);
});

const copy = {
  en: {
    home: "Home",
    orders: "My orders",
    wallet: "Points wallet",
    results: "Results",
    notifications: "Updates",
    help: "Help centre",
    login: "Sign in",
    register: "Create account",
    games: "Games",
    play: "Play now",
    pointsPayments: "Platform points · payments not connected",
    online: "Service connected",
    offline: "Service unavailable",
    checking: "Checking service",
    welcome: "A little luck, a brighter day.",
    subtitle: "Pick your numbers. Keep it light.",
    featured: "Today’s picks",
    viewAll: "Explore all games",
    announcement: "A calmer way to play",
    announcementBody: "Set your pace, check the rules, and enjoy the moment.",
    choose: "Choose your numbers",
    notices: "You’re all caught up",
    helpTitle: "Play with clarity",
    loginTitle: "Welcome back",
    registerTitle: "Make yourself at home",
    username: "Username or mobile",
    password: "Password",
    telegram: "Telegram (optional)",
    verification: "Verification code enabled",
    terms: "I agree to the terms and play responsibly.",
    continue: "Continue",
    invites: "Join codes",
    joinSourceLabel: "Optional join source",
    joinSourceNone: "No code",
    joinSourceAgent: "Agent code",
    joinSourceReferral: "Referral code",
    joinCodeLabel: "24-character code",
    joinSourceRequired: "Enter a 24-character hexadecimal code, or choose no code.",
    joinSourceReview: "Join source to apply",
    joinSourceNoCode: "No code selected",
  },
  zh: {
    home: "首页",
    orders: "我的注单",
    wallet: "积分钱包",
    results: "开奖结果",
    notifications: "消息",
    help: "帮助中心",
    login: "登录",
    register: "创建账户",
    games: "彩种",
    play: "立即选号",
    pointsPayments: "平台积分 · 支付尚未接入",
    online: "服务已连接",
    offline: "服务暂不可用",
    checking: "正在检查服务",
    welcome: "一点好运，让今天更明亮。",
    subtitle: "选好号码，轻松享受。",
    featured: "今日推荐",
    viewAll: "浏览全部彩种",
    announcement: "更从容的娱乐方式",
    announcementBody: "量力而行，了解规则，享受当下。",
    choose: "选择号码",
    notices: "目前没有新消息",
    helpTitle: "清晰地享受娱乐",
    loginTitle: "欢迎回来",
    registerTitle: "欢迎加入",
    username: "用户名或手机号",
    password: "密码",
    telegram: "Telegram（选填）",
    verification: "启用验证码",
    terms: "我同意相关条款并承诺理性参与。",
    continue: "继续",
    invites: "加入码",
    joinSourceLabel: "可选加入来源",
    joinSourceNone: "不使用代码",
    joinSourceAgent: "代理码",
    joinSourceReferral: "推荐码",
    joinCodeLabel: "24 位代码",
    joinSourceRequired: "请输入 24 位十六进制代码，或选择不使用代码。",
    joinSourceReview: "将使用的加入来源",
    joinSourceNoCode: "未选择代码",
  },
};
const t = computed(() => copy[locale.value]);
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
  localStorage.setItem("luma-language", value);
  document.documentElement.lang = value === "zh" ? "zh-CN" : "en";
});

const nav = computed(() => [
  { to: "/", label: t.value.home, icon: "⌂" },
  { to: "/orders", label: t.value.orders, icon: "▤" },
  { to: "/wallet", label: t.value.wallet, icon: "◈" },
  { to: "/results", label: t.value.results, icon: "◷" },
  { to: "/notifications", label: t.value.notifications, icon: "◌" },
  { to: "/agent", label: locale.value === "en" ? "Agent settings" : "代理设置", icon: "⌘" },
  { to: "/invites", label: t.value.invites, icon: "⌁" },
  { to: "/help", label: t.value.help, icon: "?" },
]);
const isAuth = computed(() => ["/login", "/register"].includes(route.path));
const activePage = computed(() => route.path);
const isBettingRoute = computed(
  () =>
    route.path === "/games" ||
    route.path.startsWith("/games/") ||
    route.path === "/bet/confirm" ||
    route.path === "/orders" ||
    route.path.startsWith("/orders/"),
);
function authTerms() {
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
    authError.value = authErrorMessage(error);
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
        telegramError.value = authErrorMessage(error);
      }
    } else {
      telegramSdkReady.value = false;
      telegramChallenge.value = null;
      telegramError.value = "";
    }
  } catch (error) {
    authError.value = authErrorMessage(error);
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
    telegramError.value = authErrorMessage(error);
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
function captureJoinChoice() {
  if (joinSource.value === "none") return { source: "none" as const, fields: {} };
  const field = joinSource.value === "agent" ? "agent_code" : "referral_code";
  try {
    const fields = normalizeJoinCodeFields({ [field]: joinCode.value });
    if (!fields.agent_code && !fields.referral_code) {
      throw new TypeError(t.value.joinSourceRequired);
    }
    return { source: joinSource.value, fields };
  } catch {
    throw new TypeError(t.value.joinSourceRequired);
  }
}
function passwordError() {
  const bytes = passwordByteLength(password.value);
  if (bytes < 10 || bytes > 128)
    return locale.value === "en"
      ? "Password must be 10–128 bytes."
      : "密码长度须为 10 至 128 字节。";
  return "";
}
function handleAuthError(error: unknown) {
  if (isBrandJoinRequired(error)) {
    joinAttributionFixed.value = false;
    const pending = pendingBrandJoin.value;
    if (pending) {
      identifier.value = pending.identifier;
      password.value = pending.password;
      joinSource.value = pending.source;
      joinCode.value = pending.fields.agent_code || pending.fields.referral_code || "";
    }
    joinTermsPrompt.value = true;
    joinTermsAgreed.value = false;
    authError.value =
      locale.value === "en"
        ? "Please review and accept this brand’s current terms to join."
        : "请阅读并同意当前品牌条款后加入。";
    return;
  }
  authError.value = authErrorMessage(error);
}
async function finishAuth(
  session: import("../../shared/src/auth").AuthSession,
) {
  pendingBrandJoin.value = null;
  joinTermsPrompt.value = false;
  joinAttributionFixed.value = false;
  password.value = "";
  authProfile.value = discardAccessToken(session);
  await router.push("/account");
}
async function authSubmit() {
  if (joinTermsPrompt.value) return;
  authError.value = "";
  joinAttributionFixed.value = false;
  joinTermsPrompt.value = false;
  if (!authConfigurationLoaded.value) {
    authError.value =
      locale.value === "en"
        ? "Authentication settings are unavailable. Retry loading them before continuing."
        : "身份验证配置暂不可用，请重试加载后继续。";
    return;
  }
  const lengthError = passwordError();
  if (lengthError) {
    authError.value = lengthError;
    return;
  }
  if (!captchaReady.value) {
    authError.value =
      locale.value === "en"
        ? "Load the verification image and enter its code."
        : "请加载验证码图片并输入验证码。";
    return;
  }
  if (
    route.path === "/register" &&
    (!privacyAgreed.value || !serviceTermsAgreed.value)
  ) {
    authError.value =
      locale.value === "en"
        ? "Accept both policies to create your account."
        : "创建账户前请同意隐私政策和服务条款。";
    return;
  }
  const normalized = normalizeIdentifier(identifier.value);
  if (!normalized.identifier) {
    authError.value =
      locale.value === "en"
        ? "Enter a username or phone number."
        : "请输入用户名或手机号。";
    return;
  }
  let choice: ReturnType<typeof captureJoinChoice>;
  try {
    choice = captureJoinChoice();
  } catch (error) {
    authError.value = error instanceof Error ? error.message : t.value.joinSourceRequired;
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
    else authError.value = authErrorMessage(error);
    await refreshCaptcha();
  } finally {
    authBusy.value = false;
  }
}
async function acceptBrandTermsAndLogin() {
  if (!joinTermsAgreed.value) return;
  if (!authConfigurationLoaded.value) {
    authError.value =
      locale.value === "en"
        ? "Authentication settings are unavailable. Retry loading them before continuing."
        : "身份验证配置暂不可用，请重试加载后继续。";
    return;
  }
  if (!captchaReady.value) {
    authError.value =
      locale.value === "en"
        ? "Load a new verification image and enter its code."
        : "请加载新的验证码图片并输入验证码。";
    return;
  }
  const pending = pendingBrandJoin.value;
  if (!pending) {
    authError.value =
      locale.value === "en"
        ? "Please sign in again before joining this brand."
        : "请重新登录后再加入此品牌。";
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
      authError.value =
        locale.value === "en"
          ? "This membership already has a fixed join source. Cancel this attempt to start a new sign-in without a code; its existing attribution will remain unchanged."
          : "此会员已有固定加入来源。取消本次操作后可不使用代码重新登录；现有归属不会更改。";
    } else if (isBrandJoinRequired(error)) {
      handleAuthError(error);
    } else {
      authError.value = authErrorMessage(error);
    }
    await refreshCaptcha();
  } finally {
    authBusy.value = false;
  }
}
function cancelBrandJoin(): void {
  pendingBrandJoin.value = null;
  joinTermsPrompt.value = false;
  joinTermsAgreed.value = false;
  joinAttributionFixed.value = false;
  joinSource.value = "none";
  joinCode.value = "";
  authError.value = "";
}
async function startTelegramAuth() {
  if (!telegramReady.value || authBusy.value) return;
  if (!privacyAgreed.value || !serviceTermsAgreed.value) {
    authError.value =
      locale.value === "en"
        ? "Accept both policies before continuing with Telegram."
        : "使用 Telegram 前请同意隐私政策和服务条款。";
    return;
  }
  authBusy.value = true;
  authError.value = "";
  try {
    const choice = captureJoinChoice();
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
    telegramError.value = authErrorMessage(error);
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
    authProfile.value = null;
    if (error instanceof Error && "status" in error && error.status === 401)
      profileError.value =
        locale.value === "en"
          ? "Sign in to view your account."
          : "请登录以查看账户。";
    else profileError.value = authErrorMessage(error);
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
async function saveProfile() {
  profileError.value = "";
  profileNotice.value = "";
  const next: { username?: string; phone?: string } = {};
  if (!authProfile.value?.user.username && profileUsername.value.trim())
    next.username = profileUsername.value.trim().toLowerCase();
  if (!authProfile.value?.user.phone && profilePhone.value.trim()) {
    const normalized = normalizeIdentifier(profilePhone.value);
    if (!normalized.phone) {
      profileError.value =
        locale.value === "en"
          ? "Enter a valid phone number."
          : "请输入有效手机号。";
      return;
    }
    next.phone = normalized.phone;
  }
  if (!Object.keys(next).length) {
    profileNotice.value =
      locale.value === "en"
        ? "There are no profile details to add."
        : "没有需要补充的资料。";
    return;
  }
  profileSaving.value = true;
  try {
    await authApi.updateProfile(next);
    const profile = await authApi.me();
    authProfile.value = profile;
    profileUsername.value = profile.user.username ?? "";
    profilePhone.value = profile.user.phone ?? "";
    profileNotice.value =
      locale.value === "en" ? "Profile updated." : "资料已更新。";
  } catch (error) {
    profileError.value = authErrorMessage(error);
  } finally {
    profileSaving.value = false;
  }
}
async function logout() {
  profileError.value = "";
  try {
    await authApi.logout();
    clearAllAgentUpdates();
    authProfile.value = null;
    await router.push("/login");
  } catch (error) {
    profileError.value = authErrorMessage(error);
  }
}
function closeNotice() {
  notice.value = "";
}
onMounted(async () => {
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
    if (context.terms) termsVersions.value = context.terms;
    locale.value = validSavedLanguage && context.brand.languages.includes(validSavedLanguage)
      ? validSavedLanguage
      : context.brand.defaultLanguage;
    applyBrandPresentation(context);
    await loadAuthFeatures();
    if (generation !== pageGeneration) return;
  } catch (error) {
    if (generation !== pageGeneration) return;
    connection.value = "offline";
    connectionMessage.value =
      error instanceof Error
        ? `${t.value.offline} · ${error.message}`
        : t.value.offline;
  }
  if (route.path === "/account" || route.path === "/notifications" || route.path === "/withdraw" || route.path === "/recharge") void loadProfile();
  tickTimer = setInterval(() => tick.value++, 1000);
});
watch(locale, () => {
  connectionMessage.value =
    connection.value === "connected"
      ? t.value.online
      : connection.value === "offline"
        ? t.value.offline
        : t.value.checking;
});
watch(
  () => route.path,
  (path) => {
    if (path !== "/login") {
      pendingBrandJoin.value = null;
      joinTermsPrompt.value = false;
      joinTermsAgreed.value = false;
      joinAttributionFixed.value = false;
      joinSource.value = "none";
      joinCode.value = "";
    }
    if (path === "/account" || path === "/notifications" || path === "/withdraw" || path === "/recharge") void loadProfile();
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
  <div class="app-frame" :class="{ 'auth-frame': isAuth }">
    <aside
      v-if="!isAuth"
      class="sidebar"
      :class="{ 'sidebar-open': mobileMenu }"
      @keydown.esc="mobileMenu = false"
    >
      <button class="drawer-close" type="button" aria-label="Close navigation" @click="mobileMenu = false">×</button>
      <RouterLink to="/" class="brand" @click="mobileMenu = false"
        ><span class="brand-mark" :class="{ 'brand-mark-text': !displayLogoUrl && longLogoText }"><img v-if="displayLogoUrl" :src="displayLogoUrl" :alt="brandLogoText" crossorigin="anonymous" referrerpolicy="no-referrer" @error="brandLogoFailed = true" /><template v-else>{{ brandLogoText }}</template></span
        ><span>{{ brandName }}</span></RouterLink
      >
      <div class="nav-label">MENU</div>
      <nav aria-label="Main navigation" class="side-nav">
        <RouterLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          :class="{
            active:
              activePage === item.to ||
              (item.to === '/wallet' && activePage.startsWith('/wallet')),
          }"
          @click="mobileMenu = false"
          ><span class="nav-icon" aria-hidden="true">{{ item.icon }}</span
          >{{ item.label
          }}<span
            v-if="item.to === '/notifications' && notificationUnreadCount !== null && notificationUnreadCount !== '0'"
            class="nav-dot"
            :aria-label="locale === 'en' ? `${notificationUnreadCount} unread notifications` : `${notificationUnreadCount} 条未读消息`"
          ></span
        ></RouterLink>
      </nav>
      <div class="sidebar-bottom">
        <div class="responsible-mark">✳</div>
        <p>
          {{
            locale === "en"
              ? "A good time starts with balance."
              : "理性参与，享受乐趣。"
          }}
        </p>
        <RouterLink to="/help"
          >{{ locale === "en" ? "Play responsibly" : "理性娱乐" }}
          <span aria-hidden="true">↗</span></RouterLink
        >
      </div>
      <div class="sidebar-footer">© 2026 {{ brandName }}</div>
    </aside>

    <div class="main-column">
      <header class="topbar">
        <button
          v-if="!isAuth"
          class="menu-toggle icon-button"
          aria-label="Open navigation"
          :aria-expanded="mobileMenu"
          @click="mobileMenu = !mobileMenu"
        >
          ☰
        </button>
        <div class="breadcrumbs">
          <RouterLink to="/">{{ t.home }}</RouterLink
          ><span v-if="route.path !== '/'">/</span
          ><span v-if="route.path !== '/'">{{
            route.meta.label ||
            (route.path.includes("bet")
              ? t.choose
              : route.path.slice(1).split("/").filter(Boolean).join(" · "))
          }}</span>
        </div>
        <div class="top-actions">
          <span
            class="connection-pill"
            :class="connection"
            :title="connectionMessage"
            ><i></i
            ><span>{{
              connection === "connected"
                ? t.online
                : connection === "checking"
                  ? t.checking
                  : t.offline
            }}</span></span
          >
          <button
            v-if="contextLoaded && availableLanguages.length > 1"
            class="language-button"
            :aria-label="
              locale === 'en' ? 'Switch to Chinese' : 'Switch to English'
            "
            @click="locale = locale === 'en' ? 'zh' : 'en'"
          >
            {{ locale === "en" ? "中" : "EN" }}
          </button>
          <RouterLink
            class="avatar-link"
            :to="authProfile ? '/account' : '/login'"
            :aria-label="
              authProfile ? (locale === 'en' ? 'Account' : '账户') : t.login
            "
            ><span class="avatar">{{
              authProfile?.user.username?.slice(0, 1).toUpperCase() || "L"
            }}</span
            ><span class="avatar-name">{{
              authProfile ? (locale === "en" ? "Account" : "账户") : t.login
            }}</span></RouterLink
          >
        </div>
      </header>

      <main id="main-content" class="content" @click="mobileMenu = false">
        <div
          v-if="connection === 'offline' || brandPaused"
          class="connection-alert"
          role="status"
        >
          <strong>{{
            brandPaused
              ? locale === "en"
                ? "Brand paused"
                : "品牌已暂停"
              : t.offline
          }}</strong
          ><span>{{
            brandPaused
              ? locale === "en"
                ? "This brand is temporarily unavailable for play."
                : "当前品牌暂时无法参与。"
              : connectionMessage
          }}</span>
        </div>
        <div v-if="notice" class="toast" role="status">
          <span>{{ notice }}</span
          ><button @click="closeNotice" aria-label="Dismiss message">×</button>
        </div>

        <BettingPanel
          v-if="isBettingRoute"
          :locale="locale"
          :brand-code="walletBrandCode"
          :auth="authProfile"
          @auth-expired="authProfile = null"
        />

        <section
          v-else-if="route.path === '/login' || route.path === '/register'"
          class="auth-layout"
        >
          <div class="auth-visual">
            <RouterLink to="/" class="brand brand-light"
            ><span class="brand-mark" :class="{ 'brand-mark-text': !displayLogoUrl && longLogoText }"><img v-if="displayLogoUrl" :src="displayLogoUrl" :alt="brandLogoText" crossorigin="anonymous" referrerpolicy="no-referrer" @error="brandLogoFailed = true" /><template v-else>{{ brandLogoText }}</template></span
              ><span>{{ brandName }}</span></RouterLink
            >
            <div class="auth-copy">
              <div class="eyebrow">A MOMENT FOR YOU</div>
              <h1>{{ displayTagline }}</h1>
              <p>{{ t.subtitle }}</p>
            </div>
            <div class="auth-orbit orbit-one"></div>
            <div class="auth-orbit orbit-two"></div>
            <span class="auth-stamp">LUCK<br />LOOKS<br />GOOD ON YOU</span>
          </div>
          <div class="auth-form-wrap">
            <form class="auth-form" @submit.prevent="authSubmit">
              <div class="eyebrow">
                {{ brandName.toUpperCase() }} ·
                {{ route.path === "/login" ? t.login : t.register }}
              </div>
              <h2>
                {{ route.path === "/login" ? t.loginTitle : t.registerTitle }}
              </h2>
              <p class="form-subtitle">
                {{
                  locale === "en"
                    ? "Sign in securely with your username or phone number."
                    : "使用用户名或手机号安全登录。"
                }}
              </p>
              <label
                >{{
                  route.path === "/login"
                    ? locale === "en"
                      ? "Username or phone"
                      : "用户名或手机号"
                    : locale === "en"
                      ? "Choose a username or phone"
                      : "用户名或手机号"
                }}<input
                  v-model="identifier"
                  required
                  autocomplete="username"
                  :readonly="joinTermsPrompt"
                  :placeholder="
                    locale === 'en'
                      ? 'Username or +65 8123 4567'
                      : '用户名或 +65 8123 4567'
                  "
              /></label>
              <label
                >{{ t.password
                }}<input
                  v-model="password"
                  required
                  type="password"
                  autocomplete="current-password"
                  :readonly="joinTermsPrompt"
                  maxlength="128"
                  placeholder="••••••••••"
              /></label>
              <div class="join-code-choice">
                <label
                  >{{ t.joinSourceLabel }}
                  <select v-model="joinSource" :disabled="joinTermsPrompt">
                    <option value="none">{{ t.joinSourceNone }}</option>
                    <option value="agent">{{ t.joinSourceAgent }}</option>
                    <option value="referral">{{ t.joinSourceReferral }}</option>
                  </select>
                </label>
                <label v-if="joinSource !== 'none'">
                  {{ t.joinCodeLabel }}
                  <input
                    v-model="joinCode"
                    :readonly="joinTermsPrompt"
                    autocomplete="off"
                    autocapitalize="characters"
                    spellcheck="false"
                    maxlength="64"
                    inputmode="text"
                    placeholder="A1B2C3D4E5F607182930ABCD"
                  />
                </label>
              </div>
              <div v-if="authConfiguration.captcha_enabled" class="captcha-box">
                <div class="captcha-heading">
                  <strong>{{
                    locale === "en" ? "Security check" : "安全验证"
                  }}</strong
                  ><button
                    type="button"
                    class="captcha-refresh"
                    :disabled="captchaLoading"
                    @click="refreshCaptcha"
                  >
                    {{
                      captchaLoading
                        ? locale === "en"
                          ? "Loading…"
                          : "加载中…"
                        : locale === "en"
                          ? "Refresh image"
                          : "刷新图片"
                    }}
                  </button>
                </div>
                <img
                  v-if="captchaImageSource"
                  class="captcha-image"
                  :src="captchaImageSource"
                  :alt="
                    locale === 'en' ? 'Verification code image' : '验证码图片'
                  "
                />
                <p v-else class="muted">
                  {{
                    locale === "en"
                      ? "Loading verification image…"
                      : "正在加载验证码图片…"
                  }}
                </p>
                <label
                  >{{
                    locale === "en"
                      ? "Enter the characters shown"
                      : "输入图中字符"
                  }}<input
                    v-model="captchaAnswer"
                    required
                    autocomplete="off"
                    inputmode="text"
                    maxlength="16"
                /></label>
                <small>{{
                  locale === "en"
                    ? `Expires ${captchaChallenge?.expires_at || ""}. A new image is required after an unsuccessful attempt.`
                    : `有效期至 ${captchaChallenge?.expires_at || ""}。提交失败后请使用新图片。`
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
                <label class="check-row"
                  ><input v-model="privacyAgreed" type="checkbox" />{{
                    locale === "en" ? "I accept the" : "我已阅读并同意"
                  }}
                  <RouterLink to="/privacy" target="_blank">{{
                    locale === "en" ? "Privacy Policy" : "隐私政策"
                  }}</RouterLink>
                  <span
                    >({{ termsVersions.privacy_policy_version }})</span
                  ></label
                >
                <label class="check-row"
                  ><input v-model="serviceTermsAgreed" type="checkbox" />{{
                    locale === "en" ? "I accept the" : "我已阅读并同意"
                  }}
                  <RouterLink to="/terms" target="_blank">{{
                    locale === "en" ? "Service Terms" : "服务条款"
                  }}</RouterLink>
                  <span
                    >({{ termsVersions.service_terms_version }})</span
                  ></label
                >
              </template>
              <div
                v-if="route.path === '/login' && joinTermsPrompt"
                class="join-terms"
              >
                <p>
                  {{
                    locale === "en"
                      ? "This account already exists. Joining this brand requires accepting its terms:"
                      : "此账户已存在。加入当前品牌需同意以下条款："
                  }}
                </p>
                <div class="join-source-review">
                  <strong>{{ t.joinSourceReview }}</strong>
                  <span v-if="pendingBrandJoin?.source === 'none'">{{ t.joinSourceNoCode }}</span>
                  <span v-else>{{ pendingBrandJoin?.source === 'agent' ? t.joinSourceAgent : t.joinSourceReferral }} · {{ pendingBrandJoin?.fields.agent_code || pendingBrandJoin?.fields.referral_code }}</span>
                </div>
                <label class="check-row"
                  ><input v-model="joinTermsAgreed" type="checkbox" />{{
                    locale === "en" ? "I accept the" : "我已阅读并同意"
                  }}
                  <RouterLink to="/terms" target="_blank">{{
                    locale === "en"
                      ? "Privacy Policy and Service Terms"
                      : "隐私政策和服务条款"
                  }}</RouterLink>
                  <span
                    >({{ termsVersions.privacy_policy_version }} ·
                    {{ termsVersions.service_terms_version }})</span
                  ></label
                >
                <button
                  type="button"
                  class="button button-secondary full-button"
                  :disabled="!joinTermsAgreed || authBusy || !captchaReady"
                  @click="acceptBrandTermsAndLogin"
                >
                  {{
                    locale === "en" ? "Accept and join brand" : "同意并加入品牌"
                  }}
                  <span>→</span>
                </button>
              </div>
              <p v-if="authError" class="auth-error" role="alert">
                {{ authError }}
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
                    ? locale === "en"
                      ? "Cancel and choose no code"
                      : "取消并选择不使用代码"
                    : locale === "en"
                      ? "Cancel and clear join choice"
                      : "取消并清除加入来源"
                }}
              </button>
              <button
                v-if="!authConfigurationLoaded"
                type="button"
                class="button button-secondary full-button"
                :disabled="authConfigurationLoading"
                @click="loadAuthFeatures"
              >
                {{
                  authConfigurationLoading
                    ? locale === "en"
                      ? "Loading settings…"
                      : "正在加载配置…"
                    : locale === "en"
                      ? "Retry loading sign-in settings"
                      : "重试加载登录配置"
                }}
              </button>
              <button
                v-if="!joinTermsPrompt"
                class="button button-primary full-button"
                type="submit"
                :disabled="authBusy || !captchaReady"
              >
                {{
                  authBusy
                    ? locale === "en"
                      ? "Please wait…"
                      : "请稍候…"
                    : t.continue
                }}
                <span>→</span>
              </button>
              <button
                v-if="telegramReady"
                class="button button-secondary full-button"
                type="button"
                :disabled="authBusy || !privacyAgreed || !serviceTermsAgreed || joinTermsPrompt"
                @click="startTelegramAuth"
              >
                {{
                  locale === "en"
                    ? "Continue with Telegram"
                    : "使用 Telegram 登录"
                }}
                <span>↗</span>
              </button>
              <button
                v-else
                class="button button-secondary full-button"
                type="button"
                disabled
                :title="
                  telegramError ||
                  (locale === 'en'
                    ? 'Bot authorization is not configured.'
                    : 'Telegram 尚未配置机器人授权。')
                "
              >
                {{
                  locale === "en"
                    ? "Telegram sign-in unavailable"
                    : "Telegram 登录暂不可用"
                }}
              </button>
              <button
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
                @click="refreshTelegramChallenge"
              >
                {{
                  locale === "en"
                    ? "Refresh Telegram challenge"
                    : "刷新 Telegram 授权挑战"
                }}
              </button>
              <p v-if="!telegramReady" class="tiny-note">
                {{
                  locale === "en"
                    ? "Telegram sign-in requires configured bot authorization. A Telegram username alone cannot sign you in."
                    : "Telegram 登录需要配置机器人授权。仅凭 Telegram 用户名不能登录。"
                }}
              </p>
              <p v-else class="tiny-note">
                {{
                  locale === "en"
                    ? `Telegram challenge expires ${telegramChallenge?.expires_at || ""}. Accept both current policies before opening the authorization popup.`
                    : `Telegram 验证挑战有效期至 ${telegramChallenge?.expires_at || ""}。打开授权窗口前请同意当前条款。`
                }}
              </p>
              <p v-if="telegramError" class="auth-error" role="alert">
                {{ telegramError }}
              </p>
              <p class="auth-switch">
                {{ route.path === "/login" ? t.registerTitle : t.loginTitle }}
                <RouterLink
                  :to="route.path === '/login' ? '/register' : '/login'"
                  >{{
                    route.path === "/login" ? t.register : t.login
                  }}</RouterLink
                >
              </p>
            </form>
          </div>
        </section>

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
          <div class="page-heading">
            <div>
              <div class="eyebrow">
                {{ locale === "en" ? "YOUR ACCOUNT" : "账户信息" }}
              </div>
              <h1>{{ locale === "en" ? "Account" : "账户" }}</h1>
              <p>
                {{
                  locale === "en"
                    ? "Your profile details for this brand."
                    : "当前品牌的账户资料。"
                }}
              </p>
            </div>
            <button
              v-if="authProfile"
              class="button button-secondary"
              :disabled="authBusy"
              @click="logout"
            >
              {{ locale === "en" ? "Sign out" : "退出登录" }}
            </button>
          </div>
          <div v-if="profileLoading" class="detail-panel">
            {{ locale === "en" ? "Loading account…" : "正在读取账户…" }}
          </div>
          <div v-else-if="!authProfile" class="detail-panel">
            <p class="auth-error" role="status">{{ profileError }}</p>
            <RouterLink to="/login" class="button button-primary"
              >{{ t.login }} →</RouterLink
            >
          </div>
          <div v-else class="detail-panel">
            <div class="account-identity">
              <span class="avatar">{{
                authProfile.user.username?.slice(0, 1).toUpperCase() || "L"
              }}</span>
              <div>
                <strong>{{
                  authProfile.member.display_name ||
                  authProfile.user.username ||
                  authProfile.user.phone
                }}</strong
                ><small
                  >{{ authProfile.member.status }} ·
                  {{
                    authProfile.member.joined_at
                      ? new Date(
                          authProfile.member.joined_at,
                        ).toLocaleDateString()
                      : ""
                  }}</small
                >
              </div>
            </div>
            <label class="field-label"
              >{{ locale === "en" ? "Username" : "用户名"
              }}<input
                v-model="profileUsername"
                autocomplete="username"
                :disabled="Boolean(authProfile.user.username)"
                :placeholder="
                  locale === 'en' ? 'Add a username' : '添加用户名'
                " /></label
            ><label class="field-label"
              >{{ locale === "en" ? "Phone" : "手机号"
              }}<input
                v-model="profilePhone"
                autocomplete="tel"
                :disabled="Boolean(authProfile.user.phone)"
                :placeholder="
                  locale === 'en' ? 'Add a phone number' : '添加手机号'
                "
            /></label>
            <p
              v-if="!authProfile.user.username || !authProfile.user.phone"
              class="muted"
            >
              {{
                locale === "en"
                  ? "Missing details can be added once. Saved details cannot be changed here."
                  : "尚未填写的资料可补充一次，保存后不可在此更改。"
              }}
            </p>
            <p v-if="profileError" class="auth-error" role="alert">
              {{ profileError }}
            </p>
            <p v-if="profileNotice" class="success-note" role="status">
              {{ profileNotice }}
            </p>
            <button
              v-if="!authProfile.user.username || !authProfile.user.phone"
              class="button button-primary full-button"
              :disabled="profileSaving"
              @click="saveProfile"
            >
              {{
                profileSaving
                  ? locale === "en"
                    ? "Saving…"
                    : "正在保存…"
                  : locale === "en"
                    ? "Save profile"
                    : "保存资料"
              }}
              <span>→</span>
            </button>
          </div>
        </section>

        <section
          v-else-if="route.path === '/terms' || route.path === '/privacy'"
          class="page-section narrow-page"
        >
          <div class="page-heading">
            <div>
              <div class="eyebrow">
                {{
                  route.path === "/terms" ? "SERVICE TERMS" : "PRIVACY POLICY"
                }}
                ·
                {{
                  route.path === "/terms"
                    ? termsVersions.service_terms_version
                    : termsVersions.privacy_policy_version
                }}
              </div>
              <h1>
                {{
                  route.path === "/terms"
                    ? locale === "en"
                      ? "Service Terms"
                      : "服务条款"
                    : locale === "en"
                      ? "Privacy Policy"
                      : "隐私政策"
                }}
              </h1>
              <p>
                {{
                  locale === "en"
                    ? "Current mock version dev-1"
                    : "当前模拟版本 dev-1"
                }}
              </p>
            </div>
          </div>
          <div class="detail-panel policy-copy">
            <p>
              {{
                locale === "en"
                  ? "This page is a development placeholder so you can review the version attached to your consent. The production policy text will be provided by the service."
                  : "此页面为开发占位内容，供您查看本次同意所关联的版本。正式政策文本将由服务提供。"
              }}
            </p>
            <p>
              {{
                locale === "en"
                  ? "No real account, payment, or lottery transaction is simulated by this policy page."
                  : "本政策页面不会模拟真实账户、支付或彩票交易。"
              }}
            </p>
            <RouterLink to="/register" class="text-link"
              >{{
                locale === "en" ? "Back to account creation" : "返回创建账户"
              }}
              →</RouterLink
            >
          </div>
        </section>

        <template v-else-if="route.path === '/'">
          <section class="hero">
            <div class="hero-copy">
              <div class="eyebrow">
                <span class="eyebrow-dot"></span> YOUR DAILY MOMENT
              </div>
              <h1>{{ displayTagline }}</h1>
              <p>{{ t.subtitle }}</p>
              <RouterLink to="/games" class="button button-dark"
                >{{ t.play }} <span>↗</span></RouterLink
              >
              <div v-if="displayAnnouncement" class="hero-note">
                <span v-if="displayAnnouncement" class="tiny-spark">✳</span><template v-if="displayAnnouncement">{{ displayAnnouncement }}</template>
              </div>
            </div>
            <div class="hero-art" aria-hidden="true">
              <div class="hero-sun"></div>
              <div class="hero-ball ball-a">7</div>
              <div class="hero-ball ball-b">18</div>
              <div class="hero-ball ball-c">32</div>
              <div class="hero-ball ball-d">✳</div>
              <div class="hero-arc"></div>
              <div class="hero-art-label">
                A BRIGHTER<br />WAY TO PLAY <span>↗</span>
              </div>
            </div>
            <div class="hero-count"><strong>01</strong><span>—</span> 03</div>
          </section>
          <section class="section-block">
            <div class="section-heading">
              <div>
                <div class="eyebrow">A GOOD PLACE TO START</div>
                <h2>{{ t.featured }}</h2>
              </div>
              <RouterLink to="/games" class="text-link"
                >{{ t.viewAll }} <span>→</span></RouterLink
              >
            </div>
            <BettingPanel
              :locale="locale"
              :brand-code="walletBrandCode"
              :auth="authProfile"
              embedded-catalog
              @auth-expired="authProfile = null"
            />
          </section>
          <section v-if="displayAnnouncement" class="announcement">
            <div class="announcement-icon">✳</div>
            <div>
              <div class="eyebrow">{{ locale === 'en' ? 'ANNOUNCEMENT' : '公告' }}</div>
              <h3>{{ displayAnnouncement }}</h3>
            </div>
            <RouterLink
              to="/help"
              class="announcement-link"
              :aria-label="t.help"
              >→</RouterLink
            >
          </section>
          <section class="lower-grid">
            <HomeWalletCard :brand-code="walletBrandCode" :locale="locale" @auth-expired="authProfile = null" />
            <div class="mini-panel responsible-panel">
              <div class="eyebrow">A QUICK REMINDER</div>
              <h3>
                {{
                  locale === "en"
                    ? "Play for the joy of it."
                    : "享受过程，量力而行。"
                }}
              </h3>
              <p>
                {{
                  locale === "en"
                    ? "Keep it fun, set a limit, and take breaks."
                    : "保持乐趣，设定限额，适时休息。"
                }}
              </p>
              <RouterLink to="/help" class="text-link"
                >{{ t.help }} <span>→</span></RouterLink
              >
            </div>
          </section>
        </template>

        <section
          v-else-if="
            route.path === '/wallet' || route.path === '/wallet/ledger'
          "
          class="page-section"
        >
          <WalletSummary :brand-code="walletBrandCode" :locale="locale" />
        </section>

        <RechargePanel
          v-else-if="route.path === '/recharge'"
          :key="`${authProfile?.member.brand_id ?? 'signed-out'}:${authProfile?.member.id ?? 'signed-out'}`"
          :locale="locale"
          :brand-code="walletBrandCode"
          :member="authProfile?.member ?? null"
          @auth-expired="expireRechargeProfile"
          @sign-in="router.push('/login')"
        />

        <WithdrawalPanel
          v-else-if="route.path === '/withdraw'"
          :key="`${authProfile?.member.brand_id ?? 'signed-out'}:${authProfile?.member.id ?? 'signed-out'}`"
          :locale="locale"
          :brand-code="walletBrandCode"
          :member="authProfile?.member ?? null"
          @auth-expired="authProfile = null"
          @context-changed="loadProfile"
        />

        <DrawResultsPanel
          v-else-if="route.path === '/results'"
          :locale="locale"
          :brand-code="walletBrandCode"
        />

        <NotificationsPanel
          v-else-if="route.path === '/notifications'"
          :locale="locale"
          :brand-code="walletBrandCode"
          @unread-count="notificationUnreadCount = $event"
          @auth-expired="authProfile = null"
        />

        <AgentPanel v-else-if="route.path === '/agent'" :brand-code="walletBrandCode" :locale="locale"
          @auth-expired="authProfile = null" />

        <section v-else-if="route.path === '/help'" class="page-section">
          <div class="page-heading">
            <div>
              <div class="eyebrow">HERE WHEN YOU NEED US</div>
              <h1>{{ t.helpTitle }}</h1>
              <p>{{ t.announcementBody }}</p>
            </div>
          </div>
          <div class="help-layout">
            <div class="help-intro">
              <div class="help-symbol">✳</div>
              <h2>
                {{
                  locale === "en" ? "Keep it enjoyable." : "让娱乐保持轻松。"
                }}
              </h2>
              <p>
                {{
                  locale === "en"
                    ? "Lottery play is for adults and should remain entertainment. Only spend what you can afford, decide on a limit before you play, and step away if it stops being fun."
                    : "彩票仅供成年人参与，并应以娱乐为目的。请量力而行，提前设定限额；如果不再感到愉快，请暂停参与。"
                }}
              </p>
              <a href="mailto:support@example.invalid" class="text-link"
                >{{
                  locale === "en" ? "Contact support" : "联系支持团队"
                }}
                ↗</a
              >
            </div>
            <div class="faq-list">
              <details open>
                <summary>
                  {{
                    locale === "en" ? "How do number picks work?" : "如何选号？"
                  }}
                </summary>
                <p>
                  {{
                    locale === "en"
                      ? "X+Y and M-select-N games use the configured ordinary and special-number pools. In digit-position games, choose digits from 0–9 for each position; the game rules decide whether repeated digits such as 111 are allowed. Extra selections form compound bets. Check the server quote for the combination count, multiplier and total points before submitting. Attribute, feature and exclusion picks follow the selected play rules."
                      : "X+Y 和 M选N 玩法按配置的普通号、特别号号码池选号。数字位玩法为每一位选择 0–9，是否允许 111 这样的重复数字由游戏规则决定。多选形成复式；提交前请核对服务器报价中的注数、倍数和总积分。属性、特征和排除选号按所选玩法规则执行。"
                  }}
                </p>
              </details>
              <details>
                <summary>
                  {{ locale === "en" ? "What are points?" : "什么是积分？" }}
                </summary>
                <p>
                  {{
                    locale === "en"
                      ? "Points are a platform unit shown as whole numbers. Current balances and source breakdowns come from the live wallet module. Points are not cash, and external payments are not connected."
                      : "积分是平台单位，以整数显示。当前余额和来源明细来自实时钱包模块。积分不代表现金，外部支付尚未接入。"
                  }}
                </p>
              </details>
              <details>
                <summary>
                  {{
                    locale === "en"
                      ? "How does withdrawal eligibility work?"
                      : "提现资格如何计算？"
                  }}
                </summary>
                <p>
                  {{
                    locale === "en"
                      ? "Eligibility depends on point source and turnover requirements shown for your account. You can submit an internal withdrawal request and follow its status. First-phase processing is internal; external payments are not connected."
                      : "提现资格取决于账户显示的积分来源和流水要求。你可以提交内部提现申请并查看处理状态。第一阶段采用内部处理，外部支付尚未接入。"
                  }}
                </p>
              </details>
              <details>
                <summary>
                  {{
                    locale === "en"
                      ? "How do I play responsibly?"
                      : "如何理性参与？"
                  }}
                </summary>
                <p>
                  {{
                    locale === "en"
                      ? "Set a time and spending limit, never chase losses, take regular breaks, and ask for support if play affects your wellbeing."
                      : "设定时间和支出限额，不要追逐损失，定期休息。如果参与影响身心健康，请寻求帮助。"
                  }}
                </p>
              </details>
            </div>
          </div>
        </section>
      </main>

      <footer v-if="!isAuth" class="page-footer">
        <span
          >© 2026 {{ brandName
          }}<template v-if="isBettingRoute">
            · {{ t.pointsPayments }}</template
          ><template v-else-if="route.path === '/results'">
            ·
            {{
              locale === "en" ? "Live public draw records" : "真实公开开奖记录"
            }} </template
          ><template v-else-if="route.path === '/notifications'">
            · {{ locale === "en" ? "Live in-app inbox" : "真实站内消息" }}
          </template><template v-else>
            · {{ t.pointsPayments }}</template
          ></span
        >
        <nav>
          <RouterLink to="/help">{{ t.help }}</RouterLink
          ><RouterLink to="/help">{{
            locale === "en" ? "Responsible play" : "理性娱乐"
          }}</RouterLink
          ><span class="footer-online"
            ><i></i
            >{{ connection === "connected" ? t.online : t.offline }}</span
          >
        </nav>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.join-code-choice {
  display: grid;
  gap: 10px;
  margin: 14px 0;
}
.join-code-choice label {
  display: grid;
  gap: 7px;
  color: #596a5e;
  font-size: 9px;
  font-weight: 600;
}
.join-code-choice select {
  width: 100%;
  min-height: 42px;
  padding: 0 12px;
  border: 1px solid #e3e9e2;
  border-radius: 9px;
  background: #fff;
  color: #35483b;
  font-size: 11px;
}
.join-source-review {
  display: grid;
  gap: 6px;
  margin: 10px 0 12px;
  padding: 10px;
  border-radius: 9px;
  background: #eef3eb;
  color: #425c4a;
  font-size: 10px;
  overflow-wrap: anywhere;
}
</style>
