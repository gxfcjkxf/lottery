<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import {
  createApiClient,
  defaultBrand,
  formatPoints,
  type Language,
} from "@lottery/shared";
import {
  selectionCount,
  totalPoints,
  validateSelection,
  type SelectionDraft,
  type SelectionModel,
} from "./selection";
import { isValidWholeAmount } from "./withdrawal";
import WalletSummary from "./WalletSummary.vue";
import BettingPanel from "./BettingPanel.vue";
import DrawResultsPanel from "./DrawResultsPanel.vue";
import NotificationsPanel from "./NotificationsPanel.vue";
import {
  createAuthClient,
  type AuthChallenge,
  type AuthFeatures,
  type TelegramChallenge,
} from "../../shared/src/auth";
import {
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
const savedLanguage = localStorage.getItem("luma-language") as Language | null;
const locale = ref<Language>(savedLanguage || "en");
const api = createApiClient({
  brandCode: import.meta.env.VITE_BRAND_CODE || undefined,
});
const authApi = createAuthClient({
  brandCode: import.meta.env.VITE_BRAND_CODE || undefined,
});
const connection = ref<"checking" | "connected" | "offline">("checking");
const connectionMessage = ref("Checking service");
const brandName = ref(defaultBrand.name);
const brandPaused = ref(false);
const mobileMenu = ref(false);
const notice = ref("");
const busy = ref(false);
const agreed = ref(false);
const privacyAgreed = ref(false);
const serviceTermsAgreed = ref(false);
const joinTermsAgreed = ref(false);
const joinTermsPrompt = ref(false);
const authBusy = ref(false);
const authError = ref("");
const identifier = ref("");
const password = ref("");
const authProfile = ref<AuthProfile | null>(null);
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
const withdrawAmount = ref("");
const withdrawSubmitted = ref(false);
const excluding = ref(false);
let tickTimer: ReturnType<typeof setInterval> | undefined;
onUnmounted(() => {
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
    balance: "Available points",
    demo: "Prototype · demo data",
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
    selected: "Selected",
    excluded: "Excluded",
    multiplier: "Multiplier",
    clear: "Clear",
    random: "Quick pick",
    next: "Review selection",
    unit: "per line",
    lines: "lines",
    total: "Total points",
    issue: "Current draw",
    cutoff: "Closes in 02:18:42",
    open: "Open for picks",
    rules: "Rules",
    confirm: "Confirm picks",
    snapshot: "Your selection is locked for this review.",
    submitted: "Demo order created locally. No points were moved.",
    cancel: "Cancel demo order",
    cancelled: "Demo order marked cancelled. No points were moved.",
    status: "Status",
    noChanges:
      "This prototype does not change real account balances or submit transactions.",
    demoAccount: "Demo account",
    walletNote: "Sample figures only · not connected to a real wallet",
    withdraw: "Withdrawal request",
    eligible: "Eligibility preview",
    progress: "Play-through progress",
    source: "Eligible points only",
    amount: "Amount",
    submit: "Preview request",
    requestSaved: "Preview saved locally. No withdrawal was submitted.",
    ledger: "Points activity",
    recharge: "Add points",
    manual: "First version uses manual top-up requests.",
    notices: "You’re all caught up",
    helpTitle: "Play with clarity",
    detail: "How it works",
    loginTitle: "Welcome back",
    registerTitle: "Make yourself at home",
    username: "Username or mobile",
    password: "Password",
    telegram: "Telegram (optional)",
    verification: "Verification code enabled",
    terms: "I agree to the terms and play responsibly.",
    continue: "Continue",
    demoLogin: "Demo only: authentication is not connected.",
    resultTitle: "Recent results",
    resultInfo: "Published draws appear here when the service is connected.",
    openGame: "View game",
    back: "Back to games",
    expiry: "Draw status",
    ruleVersion: "Rules version 1.2",
    notificationHelp: "Notifications are sample content in this prototype.",
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
    balance: "可用积分",
    demo: "原型 · 演示数据",
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
    selected: "已选",
    excluded: "已排除",
    multiplier: "倍数",
    clear: "清空",
    random: "机选",
    next: "查看选号",
    unit: "每注",
    lines: "注",
    total: "总积分",
    issue: "当前期次",
    cutoff: "距截止 02:18:42",
    open: "投注开放中",
    rules: "玩法规则",
    confirm: "确认选号",
    snapshot: "此页面展示的是选号快照。",
    submitted: "演示注单已保存在本机，未扣除积分。",
    cancel: "取消演示注单",
    cancelled: "演示注单已标记取消，未退还或变更积分。",
    status: "状态",
    noChanges: "此原型不会更改真实账户余额或提交交易。",
    demoAccount: "演示账户",
    walletNote: "仅供演示 · 未连接真实钱包",
    withdraw: "提现申请",
    eligible: "资格预览",
    progress: "流水进度",
    source: "仅限符合条件的积分",
    amount: "金额",
    submit: "预览申请",
    requestSaved: "预览已保存在本机，没有提交提现。",
    ledger: "积分明细",
    recharge: "充值",
    manual: "首期采用人工充值申请。",
    notices: "目前没有新消息",
    helpTitle: "清晰地享受娱乐",
    detail: "玩法说明",
    loginTitle: "欢迎回来",
    registerTitle: "欢迎加入",
    username: "用户名或手机号",
    password: "密码",
    telegram: "Telegram（选填）",
    verification: "启用验证码",
    terms: "我同意相关条款并承诺理性参与。",
    continue: "继续",
    demoLogin: "仅为演示：身份验证尚未连接。",
    resultTitle: "近期结果",
    resultInfo: "服务连接后，已开奖期次将在此显示。",
    openGame: "查看彩种",
    back: "返回彩种",
    expiry: "期次状态",
    ruleVersion: "规则版本 1.2",
    notificationHelp: "此原型中的通知为示例内容。",
  },
};
const t = computed(() => copy[locale.value]);
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
const gameId = computed(() => String(route.params.gameId || "classic-6"));
interface GameConfig {
  id: string;
  name: string;
  type: string;
  pool: number;
  pick: number;
  unit: number;
  draw: string;
  color: string;
  drawResult: string;
  kind: "regular-special" | "digits";
  specialPool?: number;
  specialPick?: number;
}
const games: GameConfig[] = [
  {
    id: "classic-6",
    name: "Classic 6/49",
    type: "SIX + SPECIAL · 1–49 / 1–10",
    kind: "regular-special",
    pool: 49,
    pick: 6,
    specialPool: 10,
    specialPick: 1,
    unit: 2,
    draw: "No. 20261005-184",
    color: "mint",
    drawResult: "04 · 11 · 18 · 26 · 33 · 42 + 07",
  },
  {
    id: "lucky-5",
    name: "Lucky 5/35",
    type: "FIVE + SPECIAL · 1–35 / 1–12",
    kind: "regular-special",
    pool: 35,
    pick: 5,
    specialPool: 12,
    specialPick: 1,
    unit: 3,
    draw: "No. 20261005-092",
    color: "peach",
    drawResult: "02 · 09 · 17 · 24 · 31 + 08",
  },
  {
    id: "daily-3",
    name: "Daily 3",
    type: "THREE POSITIONAL DIGITS · 0–9",
    kind: "digits",
    pool: 10,
    pick: 3,
    unit: 1,
    draw: "No. 20261005-036",
    color: "blue",
    drawResult: "3 · 6 · 8",
  },
];
const game = computed(
  () => games.find((item) => item.id === gameId.value) ?? games[0]!,
);
const model = computed<SelectionModel>(() => ({
  id: game.value.id,
  kind: game.value.kind,
  regularPick: game.value.pick,
  regularPool: game.value.pool,
  specialPick: game.value.specialPick,
  specialPool: game.value.specialPool,
  digitCount: game.value.kind === "digits" ? game.value.pick : undefined,
  unitPoints: BigInt(game.value.unit),
  minMultiplier: 1,
  maxMultiplier: 20,
}));
const selected = ref<number[]>([]);
const selectedSpecial = ref<number[]>([]);
const excluded = ref<number[]>([]);
const digits = ref<Array<number | null>>([null, null, null]);
const digitPosition = ref(0);
const multiplier = ref(1);
const draft = computed<SelectionDraft>(() => ({
  regular: selected.value,
  special: selectedSpecial.value,
  digits: digits.value,
}));
const count = computed(() => selectionCount(model.value, draft.value));
const cost = computed(() => {
  try {
    return totalPoints(model.value, draft.value, multiplier.value);
  } catch {
    return 0n;
  }
});
const selectionError = computed(() =>
  validateSelection(model.value, draft.value, multiplier.value),
);
const currentPickCount = computed(() =>
  game.value.kind === "digits"
    ? digits.value.filter((value) => value !== null).length
    : selected.value.length,
);
const displayPoints = (n: bigint | number | string) =>
  formatPoints(String(n), locale.value === "zh" ? "zh-CN" : "en");
const orders = ref<
  Array<{
    id: string;
    game: string;
    numbers: number[];
    special?: number[];
    digits?: number[];
    multiplier: number;
    points: string;
    status: string;
    time: string;
  }>
>([]);
const activeOrder = computed(() =>
  orders.value.find((order) => order.id === String(route.params.id)),
);

function readOrders() {
  try {
    orders.value = JSON.parse(localStorage.getItem("luma-demo-orders") || "[]");
  } catch {
    orders.value = [];
  }
  if (!orders.value.length)
    orders.value = [
      {
        id: "LP-18426",
        game: "Classic 6/49",
        numbers: [4, 11, 18, 26, 33, 42],
        multiplier: 1,
        points: "2",
        status: "Open",
        time: "Today · 10:42",
      },
    ];
}
function saveOrders() {
  localStorage.setItem("luma-demo-orders", JSON.stringify(orders.value));
}
function toggleNumber(n: number) {
  const target = excluding.value ? excluded : selected;
  const other = excluding.value ? selected : excluded;
  other.value = other.value.filter((item) => item !== n);
  target.value = target.value.includes(n)
    ? target.value.filter((item) => item !== n)
    : [...target.value, n].sort((a, b) => a - b);
}
function toggleSpecial(n: number) {
  selectedSpecial.value = selectedSpecial.value.includes(n)
    ? selectedSpecial.value.filter((item) => item !== n)
    : [...selectedSpecial.value, n].sort((a, b) => a - b);
}
function gameArtNumbers(item: GameConfig, index: number): string[] {
  return Array.from({ length: 3 }, (_, slot) => {
    const number =
      item.kind === "digits"
        ? [1, 2, 1][slot]!
        : ((index * 7 + slot * 11 + 4) % item.pool) + 1;
    return item.kind === "digits"
      ? String(number)
      : String(number).padStart(2, "0");
  });
}
function setDigit(n: number) {
  const next = [...digits.value];
  next[digitPosition.value] = n;
  digits.value = next;
  digitPosition.value = Math.min(
    digitPosition.value + 1,
    digits.value.length - 1,
  );
}
function clearPicks() {
  selected.value = [];
  selectedSpecial.value = [];
  excluded.value = [];
  digits.value = [null, null, null];
  digitPosition.value = 0;
  multiplier.value = 1;
}
function quickPick() {
  if (game.value.kind === "digits") {
    digits.value = Array.from({ length: game.value.pick }, () =>
      Math.floor(Math.random() * 10),
    );
    digitPosition.value = game.value.pick - 1;
    return;
  }
  const available = Array.from(
    { length: game.value.pool },
    (_, i) => i + 1,
  ).filter((n) => !excluded.value.includes(n));
  for (let i = available.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [available[i], available[j]] = [available[j]!, available[i]!];
  }
  selected.value = available
    .slice(0, Math.max(game.value.pick, selected.value.length))
    .sort((a, b) => a - b);
  const specialPool = Array.from(
    { length: game.value.specialPool ?? 0 },
    (_, i) => i + 1,
  );
  selectedSpecial.value = game.value.specialPick
    ? [specialPool[Math.floor(Math.random() * specialPool.length)]!]
    : [];
}
function continueToBet(id: string) {
  clearPicks();
  router.push(`/games/${id}/bet`);
}
function reviewPicks() {
  if (brandPaused.value) {
    notice.value =
      locale.value === "en"
        ? "This brand is paused. Picks are unavailable."
        : "当前品牌已暂停，暂时无法选号。";
    return;
  }
  if (selectionError.value) {
    notice.value = selectionError.value;
    return;
  }
  sessionStorage.setItem(
    "luma-demo-draft",
    JSON.stringify({
      game: game.value.name,
      gameId: game.value.id,
      numbers: selected.value,
      regular: selected.value,
      special: selectedSpecial.value,
      digits: digits.value,
      excluded: excluded.value,
      multiplier: multiplier.value,
      points: cost.value.toString(),
      issue: game.value.draw,
      ruleVersion: "1.2",
    }),
  );
  router.push("/bet/confirm");
}
function placeDemoOrder() {
  if (brandPaused.value) {
    notice.value =
      locale.value === "en"
        ? "This brand is paused. Picks are unavailable."
        : "当前品牌已暂停，暂时无法选号。";
    return;
  }
  busy.value = true;
  window.setTimeout(() => {
    let draft: {
      game: string;
      numbers: number[];
      regular?: number[];
      special?: number[];
      digits?: Array<number | null>;
      multiplier: number;
      points: string;
    };
    try {
      draft = JSON.parse(sessionStorage.getItem("luma-demo-draft") || "null");
    } catch {
      notice.value = "Selection expired. Please pick again.";
      busy.value = false;
      return;
    }
    if (!draft) {
      notice.value = "Selection expired. Please pick again.";
      busy.value = false;
      return;
    }
    const order = {
      id: `DEMO-${Date.now().toString().slice(-6)}`,
      game: draft.game,
      numbers: draft.digits?.length
        ? draft.digits.filter((digit): digit is number => digit !== null)
        : (draft.regular ?? draft.numbers),
      special: draft.special,
      digits: draft.digits?.filter((digit): digit is number => digit !== null),
      multiplier: draft.multiplier,
      points: draft.points,
      status: "Open",
      time: "Just now",
    };
    orders.value.unshift(order);
    saveOrders();
    sessionStorage.removeItem("luma-demo-draft");
    busy.value = false;
    notice.value = t.value.submitted;
    router.push(`/orders/${order.id}`);
  }, 300);
}
function cancelOrder(id: string) {
  const found = orders.value.find((order) => order.id === id);
  if (found?.status === "Open") {
    found.status = "Cancelled (demo)";
    saveOrders();
    notice.value = t.value.cancelled;
  }
}
function submitWithdrawal() {
  const raw = withdrawAmount.value.trim();
  if (!isValidWholeAmount(raw, 8200n)) {
    notice.value =
      locale.value === "en"
        ? "Enter a whole number from 1 to 8,200."
        : "请输入 1 至 8,200 之间的整数。";
    return;
  }
  withdrawSubmitted.value = true;
  notice.value = t.value.requestSaved;
}
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
  authProfile.value = discardAccessToken(session);
  await router.push("/account");
}
async function authSubmit() {
  authError.value = "";
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
  authBusy.value = true;
  try {
    const session =
      route.path === "/register"
        ? await authApi.register({
            ...authTerms(),
            ...captchaInput(),
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
    handleAuthError(error);
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
  authError.value = "";
  authBusy.value = true;
  try {
    const normalized = normalizeIdentifier(identifier.value);
    const session = await authApi.login({
      identifier: normalized.identifier,
      password: password.value,
      ...authTerms(),
      ...captchaInput(),
    });
    joinTermsPrompt.value = false;
    await finishAuth(session);
  } catch (error) {
    handleAuthError(error);
    await refreshCaptcha();
  } finally {
    authBusy.value = false;
  }
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
  profileLoading.value = true;
  profileError.value = "";
  profileNotice.value = "";
  try {
    const profile = await authApi.me();
    authProfile.value = profile;
    profileUsername.value = profile.user.username ?? "";
    profilePhone.value = profile.user.phone ?? "";
  } catch (error) {
    authProfile.value = null;
    if (error instanceof Error && "status" in error && error.status === 401)
      profileError.value =
        locale.value === "en"
          ? "Sign in to view your account."
          : "请登录以查看账户。";
    else profileError.value = authErrorMessage(error);
  } finally {
    profileLoading.value = false;
  }
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
    authProfile.value = null;
    await router.push("/login");
  } catch (error) {
    profileError.value = authErrorMessage(error);
  }
}
function closeNotice() {
  notice.value = "";
}
function readDraft(): {
  game: string;
  gameId: string;
  numbers: number[];
  regular: number[];
  special: number[];
  digits: Array<number | null>;
  excluded: number[];
  multiplier: number;
  points: string;
  issue: string;
  ruleVersion: string;
} | null {
  try {
    return JSON.parse(sessionStorage.getItem("luma-demo-draft") || "null");
  } catch {
    return null;
  }
}
function orderNumbers(order: {
  numbers: number[];
  special?: number[];
  digits?: number[];
}) {
  return order.digits?.length
    ? order.digits.join("")
    : `${order.numbers.map((n) => String(n).padStart(2, "0")).join(" · ")}${order.special?.length ? ` + ${order.special.map((n) => String(n).padStart(2, "0")).join(" · ")}` : ""}`;
}

onMounted(async () => {
  try {
    const context = await api.getContext();
    connection.value = "connected";
    connectionMessage.value = t.value.online;
    if (context?.brand?.name) brandName.value = context.brand.name;
    brandPaused.value = context.paused;
    if (context.terms) termsVersions.value = context.terms;
    await loadAuthFeatures();
    if (!savedLanguage && context?.brand?.defaultLanguage)
      locale.value = context.brand.defaultLanguage;
    if (context?.brand?.primary)
      document.documentElement.style.setProperty(
        "--brand-primary",
        context.brand.primary,
      );
    if (context?.brand?.accent)
      document.documentElement.style.setProperty(
        "--brand-accent",
        context.brand.accent,
      );
  } catch (error) {
    connection.value = "offline";
    connectionMessage.value =
      error instanceof Error
        ? `${t.value.offline} · ${error.message}`
        : t.value.offline;
  }
  if (route.path === "/account" || route.path === "/notifications") void loadProfile();
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
    if (path === "/account" || path === "/notifications") void loadProfile();
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
    >
      <RouterLink to="/" class="brand" @click="mobileMenu = false"
        ><span class="brand-mark">l</span
        ><span>{{ brandName }}<small>PLAY WELL</small></span></RouterLink
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
              ><span class="brand-mark">l</span
              ><span>{{ brandName }}<small>PLAY WELL</small></span></RouterLink
            >
            <div class="auth-copy">
              <div class="eyebrow">A MOMENT FOR YOU</div>
              <h1>{{ t.welcome }}</h1>
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
                  minlength="10"
                  maxlength="128"
                  placeholder="••••••••••"
              /></label>
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
                :disabled="authBusy || !privacyAgreed || !serviceTermsAgreed"
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
              <h1>{{ t.welcome }}</h1>
              <p>{{ t.subtitle }}</p>
              <RouterLink to="/games" class="button button-dark"
                >{{ t.play }} <span>↗</span></RouterLink
              >
              <div class="hero-note">
                <span class="tiny-spark">✳</span> {{ t.announcementBody }}
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
          <section class="announcement">
            <div class="announcement-icon">✳</div>
            <div>
              <div class="eyebrow">A NOTE FROM LUMA</div>
              <h3>{{ t.announcement }}</h3>
              <p>{{ t.announcementBody }}</p>
            </div>
            <RouterLink
              to="/help"
              class="announcement-link"
              :aria-label="t.help"
              >→</RouterLink
            >
          </section>
          <section class="lower-grid">
            <div class="mini-panel balance-panel">
              <div class="panel-icon">◈</div>
              <div class="eyebrow">{{ t.demoAccount }}</div>
              <h3>{{ displayPoints("12840") }} <small>pts</small></h3>
              <p>{{ t.walletNote }}</p>
              <RouterLink to="/wallet" class="text-link"
                >{{ t.wallet }} <span>→</span></RouterLink
              >
            </div>
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
            route.path === '/games' ||
            (route.path.startsWith('/games/') && !route.path.endsWith('/bet'))
          "
          class="page-section"
        >
          <div class="page-heading">
            <div>
              <div class="eyebrow">{{ game.type }}</div>
              <h1>{{ game.name }}</h1>
              <p>{{ t.noChanges }}</p>
            </div>
            <span class="status-chip"><i></i>{{ t.open }}</span>
          </div>
          <div class="game-detail-layout">
            <div class="detail-card">
              <div class="eyebrow">{{ t.issue }}</div>
              <h2>{{ game.draw }}</h2>
              <div class="countdown-row">
                <div>
                  <span class="tiny-label">{{ t.expiry }}</span
                  ><strong>{{ t.open }}</strong>
                </div>
                <div>
                  <span class="tiny-label">{{ t.cutoff }}</span
                  ><strong class="countdown"
                    >02:18:{{
                      String(42 - (tick % 42)).padStart(2, "0")
                    }}</strong
                  >
                </div>
              </div>
              <p class="rule-description">
                {{
                  locale === "en"
                    ? `Choose ${game.pick} or more numbers from the pool. Every ${game.pick}-number combination is one line.`
                    : `从号码池中至少选择 ${game.pick} 个号码，每 ${game.pick} 个号码组成一注。`
                }}
              </p>
              <div class="detail-actions">
                <button
                  class="button button-dark"
                  @click="continueToBet(game.id)"
                >
                  {{ t.play }} <span>→</span></button
                ><RouterLink to="/results" class="text-link"
                  >{{ t.results }} →</RouterLink
                >
              </div>
            </div>
            <div class="result-card">
              <div class="eyebrow">{{ t.resultTitle }}</div>
              <h3>{{ game.draw }}</h3>
              <div class="result-balls">
                <span v-for="num in game.drawResult.split(' · ')" :key="num">{{
                  num
                }}</span>
              </div>
              <p>{{ t.resultInfo }}</p>
            </div>
          </div>
          <div class="model-picker">
            <div class="eyebrow">{{ t.games }}</div>
            <div class="compact-game-grid">
              <button
                v-for="item in games"
                :key="item.id"
                class="compact-game"
                @click="router.push(`/games/${item.id}`)"
              >
                <span>{{ item.name }}</span
                ><span>↗</span
                ><small
                  >{{ item.pick }} / {{ item.pool }} ·
                  {{ item.unit }} pts</small
                >
              </button>
            </div>
          </div>
        </section>

        <section
          v-else-if="route.path.endsWith('/bet')"
          class="page-section bet-page"
        >
          <div class="page-heading bet-heading">
            <div>
              <div class="eyebrow">{{ game.type }} · {{ t.ruleVersion }}</div>
              <h1>{{ t.choose }}</h1>
              <p>
                {{ game.name }} <span class="sep-dot">·</span> {{ game.draw }}
                <span class="status-chip small-chip"><i></i>{{ t.open }}</span>
              </p>
            </div>
            <RouterLink :to="`/games/${game.id}`" class="back-link"
              >← {{ t.back }}</RouterLink
            >
          </div>
          <div class="bet-layout">
            <div class="bet-main">
              <div class="pick-toolbar">
                <div>
                  <span class="tiny-label">{{ t.issue }}</span
                  ><strong>{{ game.draw }}</strong>
                </div>
                <div class="cutoff-note">
                  <span class="pulse-dot"></span><span>{{ t.cutoff }}</span>
                </div>
              </div>
              <div class="number-head">
                <div>
                  <h2>{{ t.choose }}</h2>
                  <p v-if="game.kind === 'digits'">
                    Choose one digit for each position; repeats are allowed.
                  </p>
                  <p v-else>
                    Choose regular numbers and a separate special number.
                  </p>
                </div>
                <button
                  v-if="game.kind === 'regular-special'"
                  class="exclude-toggle"
                  :class="{ selected: excluding }"
                  @click="excluding = !excluding"
                  :aria-pressed="excluding"
                >
                  ⊘ {{ t.excluded }}
                </button>
              </div>
              <div v-if="game.kind === 'digits'" class="digit-picker">
                <div
                  class="digit-slots"
                  role="group"
                  aria-label="Digit positions"
                >
                  <button
                    v-for="(_, index) in digits"
                    :key="index"
                    :class="{ current: digitPosition === index }"
                    :aria-pressed="digitPosition === index"
                    @click="digitPosition = index"
                  >
                    <span>Digit {{ index + 1 }}</span
                    ><strong>{{
                      digits[index] === null ? "–" : digits[index]
                    }}</strong>
                  </button>
                </div>
                <div
                  class="number-grid digit-grid"
                  role="group"
                  aria-label="Choose a digit from 0 to 9"
                >
                  <button
                    v-for="n in 10"
                    :key="n"
                    class="number-button"
                    :class="{ picked: digits.includes(n - 1) }"
                    :aria-label="`Choose digit ${n - 1} for position ${digitPosition + 1}`"
                    @click="setDigit(n - 1)"
                  >
                    {{ n - 1 }}
                  </button>
                </div>
              </div>
              <template v-else
                ><div class="pick-group">
                  <div class="pick-group-label">
                    REGULAR · CHOOSE {{ game.pick }}+
                  </div>
                  <div
                    class="number-grid"
                    role="group"
                    aria-label="Choose regular numbers"
                  >
                    <button
                      v-for="n in game.pool"
                      :key="n"
                      class="number-button"
                      :class="{
                        picked: selected.includes(n),
                        excluded: excluded.includes(n),
                      }"
                      :aria-pressed="
                        selected.includes(n) || excluded.includes(n)
                      "
                      :aria-label="`${n}${selected.includes(n) ? ', selected' : excluded.includes(n) ? ', excluded' : ''}`"
                      @click="toggleNumber(n)"
                    >
                      {{ String(n).padStart(2, "0") }}
                    </button>
                  </div>
                </div>
                <div class="pick-group special-group">
                  <div class="pick-group-label">
                    SPECIAL · CHOOSE {{ game.specialPick }}
                  </div>
                  <div
                    class="number-grid special-grid"
                    role="group"
                    aria-label="Choose a special number"
                  >
                    <button
                      v-for="n in game.specialPool"
                      :key="n"
                      class="number-button special-number"
                      :class="{ picked: selectedSpecial.includes(n) }"
                      :aria-pressed="selectedSpecial.includes(n)"
                      :aria-label="`Special number ${n}${selectedSpecial.includes(n) ? ', selected' : ''}`"
                      @click="toggleSpecial(n)"
                    >
                      {{ String(n).padStart(2, "0") }}
                    </button>
                  </div>
                </div></template
              >
              <div class="pick-footer">
                <span
                  >{{ t.selected }} <strong>{{ currentPickCount }}</strong
                  ><span class="tiny-label"> / {{ game.pick }}</span></span
                ><span v-if="excluded.length" class="excluded-count"
                  >{{ excluded.length }} {{ t.excluded.toLowerCase() }}</span
                ><span class="pick-attrs"
                  ><span
                    v-if="game.kind === 'regular-special'"
                    class="attribute-tag"
                    >ODD / EVEN</span
                  ><span
                    v-if="game.kind === 'regular-special'"
                    class="attribute-tag"
                    >HIGH / LOW</span
                  ><span v-else class="attribute-tag"
                    >POSITIONAL · REPEATS OK</span
                  ></span
                >
              </div>
              <div class="rule-inline">
                <span class="rule-icon">i</span
                ><span>{{
                  game.kind === "digits"
                    ? "Three ordered digits · each position accepts 0–9 · repeats allowed"
                    : `Regular 1–${game.pool}, special 1–${game.specialPool} · ${t.ruleVersion} · ${game.unit} points ${t.unit}`
                }}</span
                ><button
                  aria-label="More about rules"
                  @click="router.push('/help')"
                >
                  ?
                </button>
              </div>
            </div>
            <aside class="bet-summary">
              <div class="summary-title">
                <div>
                  <div class="eyebrow">YOUR PLAY</div>
                  <h3>{{ game.name }}</h3>
                </div>
                <span class="summary-star">✳</span>
              </div>
              <div class="summary-issue">
                <span class="tiny-label">{{ t.issue }}</span
                ><strong>{{ game.draw }}</strong
                ><span class="status-chip small-chip"><i></i>{{ t.open }}</span>
              </div>
              <div
                v-if="game.kind === 'digits'"
                class="summary-numbers digit-summary"
              >
                <span v-for="(digit, index) in digits" :key="index">{{
                  digit === null ? "–" : digit
                }}</span>
              </div>
              <div v-else class="summary-numbers">
                <span v-for="n in selected" :key="`r${n}`">{{
                  String(n).padStart(2, "0")
                }}</span
                ><span
                  v-for="n in selectedSpecial"
                  :key="`s${n}`"
                  class="special-summary-number"
                  >★{{ String(n).padStart(2, "0") }}</span
                ><em v-if="!selected.length && !selectedSpecial.length">{{
                  locale === "en"
                    ? "Your picks appear here"
                    : "所选号码将显示于此"
                }}</em>
              </div>
              <label class="multiplier-label"
                >{{ t.multiplier
                }}<span class="multiplier-input"
                  ><button
                    aria-label="Decrease multiplier"
                    @click="multiplier = Math.max(1, multiplier - 1)"
                  >
                    −</button
                  ><input
                    v-model.number="multiplier"
                    type="number"
                    min="1"
                    max="20"
                    aria-label="Multiplier"
                  /><button
                    aria-label="Increase multiplier"
                    @click="multiplier = Math.min(20, multiplier + 1)"
                  >
                    +
                  </button></span
                ></label
              >
              <div class="summary-calc">
                <div>
                  <span>{{ t.selected }}</span
                  ><strong>{{ displayPoints(count) }} {{ t.lines }}</strong>
                </div>
                <div>
                  <span>{{ t.unit }}</span
                  ><strong>{{ game.unit }} pts</strong>
                </div>
                <div class="summary-total">
                  <span>{{ t.total }}</span
                  ><strong>{{ displayPoints(cost) }} <small>pts</small></strong>
                </div>
              </div>
              <div class="available-balance">
                <span>{{ t.balance }}</span
                ><strong>{{ displayPoints("12840") }} pts</strong>
              </div>
              <p v-if="selectionError" class="inline-error" role="status">
                {{ selectionError }}
              </p>
              <div class="summary-buttons">
                <button class="button button-secondary" @click="clearPicks">
                  {{ t.clear }}</button
                ><button class="button button-secondary" @click="quickPick">
                  ✳ {{ t.random }}
                </button>
              </div>
              <button
                class="button button-primary full-button"
                :disabled="!!selectionError"
                @click="reviewPicks"
              >
                {{ t.next }} <span>→</span>
              </button>
              <p class="no-debit-note">{{ t.noChanges }}</p>
            </aside>
          </div>
        </section>

        <section
          v-else-if="route.path === '/bet/confirm'"
          class="page-section narrow-page"
        >
          <div class="page-heading">
            <div>
              <div class="eyebrow">FINAL REVIEW</div>
              <h1>{{ t.confirm }}</h1>
              <p>{{ t.snapshot }}</p>
            </div>
          </div>
          <div class="confirmation-card">
            <div class="confirm-top">
              <div>
                <span class="tiny-label">{{ t.issue }}</span>
                <h2>{{ readDraft()?.game || "Selection" }}</h2>
              </div>
              <span class="status-chip"><i></i>{{ t.open }}</span>
            </div>
            <template v-if="readDraft()?.digits?.length"
              ><div class="confirm-group">
                <span class="tiny-label">POSITIONAL DIGITS</span>
                <div class="confirm-balls">
                  <span
                    v-for="(n, index) in readDraft()?.digits"
                    :key="index"
                    >{{ n === null ? "–" : n }}</span
                  >
                </div>
              </div></template
            ><template v-else
              ><div class="confirm-group">
                <span class="tiny-label">REGULAR</span>
                <div class="confirm-balls">
                  <span v-for="n in readDraft()?.regular || []" :key="n">{{
                    n
                  }}</span>
                </div>
              </div>
              <div v-if="readDraft()?.special?.length" class="confirm-group">
                <span class="tiny-label">SPECIAL</span>
                <div class="confirm-balls">
                  <span v-for="n in readDraft()?.special" :key="n">{{
                    n
                  }}</span>
                </div>
              </div></template
            >
            <div class="confirm-row">
              <span>{{ t.multiplier }}</span
              ><strong>{{ readDraft()?.multiplier || 1 }}×</strong>
            </div>
            <div class="confirm-row">
              <span>{{ t.total }}</span
              ><strong
                >{{ displayPoints(readDraft()?.points || "0") }} pts</strong
              >
            </div>
            <div class="confirm-row">
              <span>{{ t.rules }}</span
              ><strong>v{{ readDraft()?.ruleVersion || "1.2" }}</strong>
            </div>
            <div class="demo-callout">
              <span>ⓘ</span>
              <p>{{ t.noChanges }}</p>
            </div>
            <button
              class="button button-primary full-button"
              :disabled="busy || !readDraft()"
              @click="placeDemoOrder"
            >
              {{ busy ? (locale === "en" ? "Saving…" : "保存中…") : t.confirm }}
              <span>→</span></button
            ><button
              class="button button-quiet full-button"
              @click="router.back()"
            >
              {{ locale === "en" ? "Go back" : "返回修改" }}
            </button>
          </div>
        </section>

        <section v-else-if="route.path === '/orders'" class="page-section">
          <div class="page-heading">
            <div>
              <div class="eyebrow">YOUR ACTIVITY</div>
              <h1>{{ t.orders }}</h1>
              <p>{{ t.noChanges }}</p>
            </div>
            <select class="filter-select" aria-label="Filter orders">
              <option>{{ locale === "en" ? "All orders" : "全部注单" }}</option>
              <option>Open</option>
              <option>Cancelled</option>
            </select>
          </div>
          <div class="order-list">
            <article v-for="order in orders" :key="order.id" class="order-row">
              <div class="order-game-mark">✳</div>
              <div class="order-info">
                <RouterLink :to="`/orders/${order.id}`"
                  ><strong>{{ order.game }}</strong></RouterLink
                ><span>{{ order.id }} · {{ order.time }}</span
                ><span class="order-numbers">{{ orderNumbers(order) }}</span>
              </div>
              <div class="order-cost">
                <strong>{{ displayPoints(order.points) }} pts</strong
                ><span
                  class="status-chip"
                  :class="
                    order.status.toLowerCase().startsWith('cancel')
                      ? 'neutral'
                      : ''
                  "
                  ><i></i>{{ order.status }}</span
                >
              </div>
              <RouterLink
                :to="`/orders/${order.id}`"
                class="round-link"
                :aria-label="`View ${order.id}`"
                >↗</RouterLink
              >
            </article>
          </div>
          <p class="list-footnote">{{ t.demo }}</p>
        </section>

        <section
          v-else-if="route.path.startsWith('/orders/')"
          class="page-section narrow-page"
        >
          <div class="page-heading">
            <div>
              <div class="eyebrow">ORDER DETAIL</div>
              <h1>{{ activeOrder?.id || String(route.params.id) }}</h1>
            </div>
            <RouterLink to="/orders" class="back-link"
              >← {{ t.orders }}</RouterLink
            >
          </div>
          <div v-if="activeOrder" class="detail-panel">
            <div class="detail-panel-head">
              <div>
                <span class="tiny-label">{{ t.status }}</span
                ><span class="status-chip"
                  ><i></i>{{ activeOrder.status }}</span
                >
              </div>
              <strong>{{ activeOrder.game }}</strong>
            </div>
            <div class="order-numbers large-numbers">
              {{ orderNumbers(activeOrder) }}
            </div>
            <dl class="detail-list">
              <div>
                <dt>{{ t.issue }}</dt>
                <dd>20261005 · Demo</dd>
              </div>
              <div>
                <dt>{{ t.multiplier }}</dt>
                <dd>{{ activeOrder.multiplier }}×</dd>
              </div>
              <div>
                <dt>{{ t.total }}</dt>
                <dd>{{ activeOrder.points }} pts</dd>
              </div>
              <div>
                <dt>{{ t.demo }}</dt>
                <dd>{{ activeOrder.time }}</dd>
              </div>
              <div>
                <dt>{{ t.rules }}</dt>
                <dd>v1.2</dd>
              </div>
            </dl>
            <div class="demo-callout">
              <span>ⓘ</span>
              <p>{{ t.noChanges }}</p>
            </div>
            <button
              v-if="activeOrder.status === 'Open'"
              class="button button-secondary full-button"
              @click="cancelOrder(activeOrder.id)"
            >
              {{ t.cancel }}
            </button>
          </div>
          <div v-else class="empty-state">
            <span>⌕</span>
            <h2>{{ locale === "en" ? "Order not found" : "未找到注单" }}</h2>
            <RouterLink to="/orders" class="text-link"
              >{{ t.orders }} →</RouterLink
            >
          </div>
        </section>

        <section
          v-else-if="
            route.path === '/wallet' || route.path === '/wallet/ledger'
          "
          class="page-section"
        >
          <WalletSummary :brand-code="walletBrandCode" :locale="locale" />
        </section>

        <section
          v-else-if="route.path === '/recharge'"
          class="page-section narrow-page"
        >
          <div class="page-heading">
            <div>
              <div class="eyebrow">POINTS · DEMO</div>
              <h1>{{ t.recharge }}</h1>
              <p>{{ t.manual }}</p>
            </div>
          </div>
          <div class="detail-panel">
            <div class="demo-callout">
              <span>ⓘ</span>
              <p>{{ t.noChanges }}</p>
            </div>
            <h2>{{ locale === "en" ? "Request a top-up" : "提交充值申请" }}</h2>
            <p class="muted">
              {{
                locale === "en"
                  ? "A team member would review the payment reference in a live service."
                  : "正式服务中将由工作人员核对付款凭证。"
              }}
            </p>
            <label class="field-label"
              >{{ t.amount
              }}<input type="number" min="1" placeholder="100" /></label
            ><label class="field-label"
              >{{ locale === "en" ? "Payment reference" : "付款凭证编号"
              }}<input placeholder="e.g. receipt reference" /></label
            ><button
              class="button button-primary full-button"
              @click="notice = t.requestSaved"
            >
              {{ t.submit }} <span>→</span>
            </button>
          </div>
        </section>

        <section
          v-else-if="route.path === '/withdraw'"
          class="page-section narrow-page"
        >
          <div class="page-heading">
            <div>
              <div class="eyebrow">POINTS · DEMO</div>
              <h1>{{ t.withdraw }}</h1>
              <p>{{ t.walletNote }}</p>
            </div>
          </div>
          <div class="detail-panel">
            <div class="eligibility">
              <div>
                <span class="tiny-label">{{ t.eligible }}</span
                ><strong>8,200 pts</strong>
              </div>
              <span class="status-chip"
                ><i></i
                >{{ locale === "en" ? "Preview only" : "仅供预览" }}</span
              >
            </div>
            <div class="progress-label">
              <span>{{ t.progress }}</span
              ><strong>68%</strong>
            </div>
            <div class="progress-track"><span style="width: 68%"></span></div>
            <p class="muted">
              {{ t.source }} ·
              {{
                locale === "en"
                  ? "Sample qualification: 6,800 / 10,000 pts"
                  : "示例进度：6,800 / 10,000 积分"
              }}
            </p>
            <div class="demo-callout">
              <span>ⓘ</span>
              <p>{{ t.noChanges }}</p>
            </div>
            <form @submit.prevent="submitWithdrawal">
              <label class="field-label"
                >{{ t.amount }}
                <div class="amount-input">
                  <input
                    v-model="withdrawAmount"
                    type="number"
                    min="1"
                    max="8200"
                    step="1"
                    inputmode="numeric"
                    placeholder="0"
                    required
                  /><span>pts</span>
                </div></label
              >
              <p v-if="withdrawSubmitted" class="success-note" role="status">
                {{ t.requestSaved }}
              </p>
              <button class="button button-primary full-button" type="submit">
                {{ t.submit }} <span>→</span>
              </button>
            </form>
          </div>
        </section>

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
                      ? "Choose at least the required number of unique numbers. Choosing extra numbers creates a system play, expanding into every matching combination."
                      : "选择不少于规定数量的不同号码。多选会形成复式，系统将展开全部组合。"
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
                      ? "Points are a platform unit. Values on this prototype are fictional examples and do not represent money or a real account."
                      : "积分是平台单位。本原型中的数值均为虚构示例，不代表现金或真实账户。"
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
                      ? "Eligibility can depend on point source and play-through requirements. This screen only illustrates a possible status and submits no request."
                      : "提现资格可能取决于积分来源和流水要求。此页面仅展示可能的状态，不会提交申请。"
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
            ·
            {{
              locale === "en"
                ? "Live betting · real wallet"
                : "真实投注 · 实时钱包"
            }}</template
          ><template v-else-if="route.path === '/results'">
            ·
            {{
              locale === "en" ? "Live public draw records" : "真实公开开奖记录"
            }} </template
          ><template v-else-if="route.path === '/notifications'">
            · {{ locale === "en" ? "Live in-app inbox" : "真实站内消息" }}
          </template><template v-else>
            ·
            {{
              route.path.startsWith("/wallet")
                ? locale === "en"
                  ? "Live wallet"
                  : "实时钱包"
                : t.demo
            }}</template
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
