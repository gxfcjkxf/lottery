<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import AccessManagement from "./AccessManagement.vue";
import MemberProvision from "./MemberProvision.vue";
import AuthSettings from "./AuthSettings.vue";
import {
  previewCorrection,
  resolveWithdrawal,
  reviewRule,
  simulateRule,
  submitRuleForReview,
  type RuleDraft,
} from "./workflows";
import {
  AdminApiError,
  createAdminApi,
  createIdempotencyKey,
  type AdminAccount,
  type AdminBrand,
  type AdminAuditRecord,
  type Member,
  type MemberStatus,
} from "./admin-api";

type Page =
  | "工作台"
  | "品牌和域名"
  | "用户和成员"
  | "代理树"
  | "规则配置"
  | "期次和开奖"
  | "注单和异常"
  | "资金与账本"
  | "佣金和奖励"
  | "报表和对账"
  | "账号与权限"
  | "审计日志";
const nav: { name: Page; icon: string; group: string }[] = [
  { name: "工作台", icon: "▦", group: "概览" },
  { name: "品牌和域名", icon: "◇", group: "平台" },
  { name: "用户和成员", icon: "♙", group: "平台" },
  { name: "代理树", icon: "⌘", group: "平台" },
  { name: "规则配置", icon: "⌗", group: "运营" },
  { name: "期次和开奖", icon: "◷", group: "运营" },
  { name: "注单和异常", icon: "▤", group: "运营" },
  { name: "资金与账本", icon: "◈", group: "资金" },
  { name: "佣金和奖励", icon: "↗", group: "资金" },
  { name: "报表和对账", icon: "▥", group: "管理" },
  { name: "账号与权限", icon: "♧", group: "管理" },
  { name: "审计日志", icon: "≡", group: "管理" },
];
const page = ref<Page>("工作台");
const demoBrand = ref("Aurora");
const brandMenu = ref(false);
const mobileMore = ref(false);
const search = ref("");
const notice = ref("");
const showDemoNotice = ref(true);
const api = createAdminApi();
const account = ref<AdminAccount | null>(null);
const authLoading = ref(true);
const authError = ref("");
const loginIdentifier = ref("");
const loginPassword = ref("");
const loginBusy = ref(false);
const loginIdempotencyKey = ref(createIdempotencyKey());
const adminBrands = ref<AdminBrand[]>([]);
const selectedBrandId = ref("");
const brand = computed(
  () =>
    adminBrands.value.find((item) => item.id === selectedBrandId.value)?.name ??
    demoBrand.value,
);
const members = ref<Member[]>([]);
const membersLoading = ref(false);
const membersError = ref("");
const memberOffset = ref(0);
const auditRecords = ref<AdminAuditRecord[]>([]);
const auditLoading = ref(false);
const auditError = ref("");
const editTarget = ref<Member | null>(null);
const editStatus = ref<MemberStatus>("normal");
const editNotes = ref("");
const editReason = ref("");
const editIdempotencyKey = ref("");
const editBusy = ref(false);
const kickTarget = ref<Member | null>(null);
const kickReason = ref("");
const kickIdempotencyKey = ref("");
const kickBusy = ref(false);
const resetTarget = ref<Member | null>(null);
const resetPasswordValue = ref("");
const resetReason = ref("");
const resetImpactConfirmed = ref(false);
const resetIdempotencyKey = ref("");
const resetBusy = ref(false);
interface ContextBrand {
  id: string;
  code: string;
  name: string;
  status: string;
  default_locale: string;
  timezone: string;
  theme: Record<string, unknown> | null;
  config_version: number;
}
const contextBrand = ref<ContextBrand | null>(null);
const contextState = ref<"loading" | "connected" | "unavailable">("loading");
const loadBrandContext = async () => {
  try {
    const response = await fetch("/api/v1/context", {
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
    const body = (await response.json()) as {
      success?: boolean;
      data?: { brand?: ContextBrand };
    };
    if (!response.ok || body.success === false || !body.data?.brand)
      throw new Error("品牌上下文不可用");
    contextBrand.value = body.data.brand;
    contextState.value = "connected";
  } catch {
    contextBrand.value = null;
    contextState.value = "unavailable";
  }
};
const isMobile = ref(
  typeof window !== "undefined" &&
    window.matchMedia("(max-width: 700px)").matches,
);
const updateMobile = () => {
  isMobile.value = window.matchMedia("(max-width: 700px)").matches;
};
onMounted(() => {
  window.addEventListener("resize", updateMobile);
  void loadBrandContext();
  void restoreAdminSession();
});
onUnmounted(() => window.removeEventListener("resize", updateMobile));
const groups = computed(() => [...new Set(nav.map((item) => item.group))]);
const currentIcon = computed(
  () => nav.find((item) => item.name === page.value)?.icon ?? "▦",
);
const go = (target: Page) => {
  page.value = target;
  notice.value = "";
  mobileMore.value = false;
};
const toast = (text: string) => {
  notice.value = text;
  window.setTimeout(() => {
    if (notice.value === text) notice.value = "";
  }, 3200);
};
const hasPermission = (permission: string) => {
  if (!account.value) return false;
  const grants =
    account.value.permissions_by_brand?.[selectedBrandId.value] ??
    (account.value.permissions_by_brand ? [] : account.value.permissions);
  return grants.includes(permission);
};
const canEditUsers = computed(() =>
  Boolean(
    account.value &&
    !account.value.super_admin &&
    hasPermission("user.write.brand"),
  ),
);
const canKickUsers = computed(() =>
  Boolean(
    account.value &&
    !account.value.super_admin &&
    hasPermission("user.kick.brand"),
  ),
);
const canResetPasswords = computed(() =>
  Boolean(
    account.value &&
    !account.value.super_admin &&
    hasPermission("user.password_reset.brand"),
  ),
);
const statusLabel: Record<MemberStatus, string> = {
  normal: "正常",
  frozen: "冻结",
  disabled: "已停用",
  expired: "已过期",
  cancelled: "已注销",
};
const statusClass = (status: MemberStatus) =>
  status === "normal"
    ? "badge-success"
    : status === "frozen"
      ? "badge-danger"
      : "badge-neutral";
const apiErrorText = (error: unknown) =>
  error instanceof Error ? error.message : "请求失败，请重试";
const clearAdminData = () => {
  account.value = null;
  adminBrands.value = [];
  selectedBrandId.value = "";
  members.value = [];
  auditRecords.value = [];
};
const loadAdminResources = async () => {
  authError.value = "";
  try {
    const result = await api.brands();
    adminBrands.value = result.items;
  } catch (error) {
    if (error instanceof AdminApiError && error.status === 401)
      clearAdminData();
    authError.value = apiErrorText(error);
  }
};
const restoreAdminSession = async () => {
  authLoading.value = true;
  try {
    const result = await api.me();
    account.value = result.account;
    await loadAdminResources();
  } catch (error) {
    clearAdminData();
    if (!(error instanceof AdminApiError && error.status === 401))
      authError.value = apiErrorText(error);
  } finally {
    authLoading.value = false;
  }
};
const login = async () => {
  if (loginBusy.value) return;
  loginBusy.value = true;
  authError.value = "";
  try {
    await api.login(
      loginIdentifier.value.trim(),
      loginPassword.value,
      loginIdempotencyKey.value,
    );
    loginPassword.value = "";
    const result = await api.me();
    account.value = result.account;
    selectedBrandId.value = "";
    loginIdempotencyKey.value = createIdempotencyKey();
    await loadAdminResources();
    toast("已登录管理员账号");
  } catch (error) {
    clearAdminData();
    authError.value = apiErrorText(error);
  } finally {
    loginBusy.value = false;
  }
};
const logout = async () => {
  try {
    await api.logout(createIdempotencyKey());
  } catch (error) {
    authError.value = apiErrorText(error);
  }
  clearAdminData();
  toast("已退出管理员账号");
};
const selectBrand = async (brandId: string) => {
  selectedBrandId.value = brandId;
  brandMenu.value = false;
  memberOffset.value = 0;
  members.value = [];
  auditRecords.value = [];
  await Promise.all([loadMembers(), loadAudit()]);
};
const loadMembers = async () => {
  if (!account.value || !selectedBrandId.value) return;
  membersLoading.value = true;
  membersError.value = "";
  try {
    members.value = (
      await api.users(selectedBrandId.value, 100, memberOffset.value)
    ).items;
  } catch (error) {
    membersError.value = apiErrorText(error);
    if (error instanceof AdminApiError && error.status === 401)
      clearAdminData();
  } finally {
    membersLoading.value = false;
  }
};
const loadAudit = async () => {
  if (!account.value || !selectedBrandId.value) return;
  auditLoading.value = true;
  auditError.value = "";
  try {
    auditRecords.value = (await api.audit(selectedBrandId.value)).items;
  } catch (error) {
    auditError.value = apiErrorText(error);
    if (error instanceof AdminApiError && error.status === 401)
      clearAdminData();
  } finally {
    auditLoading.value = false;
  }
};
const openEdit = (member: Member) => {
  editTarget.value = member;
  editStatus.value = member.status;
  editNotes.value = member.notes;
  editReason.value = "";
  editIdempotencyKey.value = createIdempotencyKey();
};
const saveMember = async () => {
  if (
    !editTarget.value ||
    !selectedBrandId.value ||
    !editReason.value.trim() ||
    editBusy.value
  )
    return;
  editBusy.value = true;
  try {
    await api.updateUser(
      selectedBrandId.value,
      editTarget.value.id,
      {
        status: editStatus.value,
        notes: editNotes.value,
        reason: editReason.value.trim(),
      },
      editIdempotencyKey.value,
    );
    editTarget.value = null;
    await Promise.all([loadMembers(), loadAudit()]);
    toast("成员资料已更新");
  } catch (error) {
    toast(apiErrorText(error));
  } finally {
    editBusy.value = false;
  }
};
const openKick = (member: Member) => {
  kickTarget.value = member;
  kickReason.value = "";
  kickIdempotencyKey.value = createIdempotencyKey();
};
const kickMember = async () => {
  if (
    !kickTarget.value ||
    !selectedBrandId.value ||
    !kickReason.value.trim() ||
    kickBusy.value
  )
    return;
  kickBusy.value = true;
  try {
    await api.kickUser(
      selectedBrandId.value,
      kickTarget.value.id,
      kickReason.value.trim(),
      kickIdempotencyKey.value,
    );
    kickTarget.value = null;
    await Promise.all([loadMembers(), loadAudit()]);
    toast("该成员的品牌会话已踢出");
  } catch (error) {
    toast(apiErrorText(error));
  } finally {
    kickBusy.value = false;
  }
};
const openReset = (member: Member) => {
  resetTarget.value = member;
  resetPasswordValue.value = "";
  resetReason.value = "";
  resetImpactConfirmed.value = false;
  resetIdempotencyKey.value = createIdempotencyKey();
};
const resetMemberPassword = async () => {
  if (
    !resetTarget.value ||
    !selectedBrandId.value ||
    !resetPasswordValue.value ||
    !resetReason.value.trim() ||
    !resetImpactConfirmed.value ||
    resetBusy.value
  )
    return;
  resetBusy.value = true;
  try {
    await api.resetPassword(
      selectedBrandId.value,
      resetTarget.value.id,
      resetPasswordValue.value,
      resetReason.value.trim(),
      resetIdempotencyKey.value,
    );
    resetTarget.value = null;
    await Promise.all([loadMembers(), loadAudit()]);
    toast("全局密码已重置，所有品牌会话已撤销");
  } catch (error) {
    toast(apiErrorText(error));
  } finally {
    resetBusy.value = false;
  }
};
const changeMemberPage = async (direction: -1 | 1) => {
  memberOffset.value = Math.max(0, memberOffset.value + direction * 100);
  await loadMembers();
};
const withdrawals = ref([
  {
    id: "WD-2601048",
    user: "Lin Q.",
    points: "3,200",
    source: "充值 2,000 · 赢分 1,200",
    submitted: "10-05 09:42",
    status: "待审核",
    reason: "",
  },
  {
    id: "WD-2601044",
    user: "Ari L.",
    points: "850",
    source: "赢分 850",
    submitted: "10-05 08:56",
    status: "待审核",
    reason: "",
  },
  {
    id: "WD-2601039",
    user: "J. Tan",
    points: "120",
    source: "充值 120",
    submitted: "10-04 21:17",
    status: "待审核",
    reason: "",
  },
]);
const reviewTarget = ref<(typeof withdrawals.value)[number] | null>(null);
const reviewReason = ref("");
const finishWithdrawal = (decision: "approved" | "rejected") => {
  if (!reviewTarget.value) return;
  try {
    const result = resolveWithdrawal(
      reviewTarget.value.id,
      decision,
      reviewReason.value,
    );
    reviewTarget.value.status =
      decision === "approved" ? "已通过（演示）" : "已驳回（演示）";
    reviewTarget.value.reason = result.reason;
    toast(
      `${reviewTarget.value.id}：${result.status === "approved" ? "已通过" : "已驳回"}（仅本次演示）`,
    );
    reviewTarget.value = null;
    reviewReason.value = "";
  } catch (error) {
    toast(error instanceof Error ? error.message : "审核失败");
  }
};
const ruleDraft = ref<RuleDraft>({
  creator: "林岚",
  reviewer: "",
  status: "draft",
  testSelection: "07 18 29",
  testResult: "07 12 29",
  unitPoints: "2",
  multiplier: "3",
});
const ruleStep = ref(1);
const simulation = computed(() => simulateRule(ruleDraft.value));
const simulate = () =>
  toast(
    `模拟完成：${simulation.value.explanation}，预计积分 ${simulation.value.points}`,
  );
const submitRule = () => {
  try {
    ruleDraft.value = submitRuleForReview({
      ...ruleDraft.value,
      reviewer: "周宁",
    });
    toast("规则已提交给周宁审核（演示）");
  } catch (error) {
    toast(error instanceof Error ? error.message : "提交失败");
  }
};
const decisionRule = (decision: "approve" | "reject") => {
  try {
    ruleDraft.value = reviewRule(ruleDraft.value, "周宁", decision);
    toast(
      decision === "approve"
        ? "审核通过；待发布确认（演示）"
        : "规则已驳回（演示）",
    );
  } catch (error) {
    toast(error instanceof Error ? error.message : "审核失败");
  }
};
const periodResult = ref("07 18 29 33 41 + 06");
const correctionResult = ref("07 18 29 33 41 + 09");
const correctionReason = ref("");
const correctionOpen = ref(false);
const correctionPreview = computed(() =>
  previewCorrection(periodResult.value, correctionResult.value, 126),
);
const correctResult = () => {
  if (!correctionReason.value.trim()) return toast("纠正原因必填");
  toast(
    `纠正已创建：预计回溯 ${correctionPreview.value.affectedOrders} 笔注单、重新结算 ${correctionPreview.value.estimatedSettlements} 项（演示）`,
  );
  periodResult.value = correctionResult.value;
  correctionOpen.value = false;
  correctionReason.value = "";
};
const manualOpen = ref(false);
const manualReason = ref("");
const confirmManual = () => {
  if (!manualReason.value.trim()) return toast("人工开奖原因必填");
  toast("人工结果已锁定本期（演示）");
  manualOpen.value = false;
  manualReason.value = "";
};
const queryTab = ref("全部");
const ledgerTab = ref("账本流水");
const selectedRange = ref("近 7 天");

const orders = [
  {
    id: "BO-61048219",
    member: "Lin Q.",
    game: "星彩 6+1",
    period: "20261005032",
    picks: "07, 18, 29, 33, 41 + 06",
    amount: "15",
    status: "待开奖",
    time: "10-05 10:26",
  },
  {
    id: "BO-61048196",
    member: "Mia W.",
    game: "幸运三位",
    period: "20261005031",
    picks: "百位 2 · 十位 7 · 个位 1",
    amount: "8",
    status: "异常注单",
    time: "10-05 10:18",
  },
  {
    id: "BO-61047982",
    member: "Ari L.",
    game: "星彩 6+1",
    period: "20261005032",
    picks: "03, 12, 21, 28, 39 + 02",
    amount: "25",
    status: "待开奖",
    time: "10-05 09:52",
  },
  {
    id: "BO-61047212",
    member: "J. Tan",
    game: "幸运三位",
    period: "20261005030",
    picks: "百位 4 · 十位 4 · 个位 9",
    amount: "12",
    status: "已中奖",
    time: "10-05 08:03",
  },
];
const ledger = [
  {
    id: "LE-883140",
    type: "投注扣减",
    source: "注单 BO-61048219",
    before: "2,495",
    change: "-15",
    after: "2,480",
    actor: "系统演示",
    time: "10-05 10:26",
  },
  {
    id: "LE-883102",
    type: "中奖入账",
    source: "注单 BO-61047212",
    before: "2,483",
    change: "+12",
    after: "2,495",
    actor: "结算演示",
    time: "10-05 08:10",
  },
  {
    id: "LE-882988",
    type: "人工充值",
    source: "RC-2601041",
    before: "483",
    change: "+2,000",
    after: "2,483",
    actor: "周宁",
    time: "10-04 18:43",
  },
];
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <a class="brand-lockup" href="#" @click.prevent="go('工作台')"
        ><span class="brand-mark">N</span
        ><span><b>northstar</b><small>OPERATIONS CONSOLE</small></span></a
      >
      <div class="demo-chip">
        <span class="pulse"></span>资金与游戏为演示
        <span class="demo-chip-end">·</span>
      </div>
      <div class="brand-switch-wrap">
        <button
          class="brand-switch"
          @click="brandMenu = !brandMenu"
          aria-label="切换品牌"
        >
          <span class="brand-avatar">{{ brand.slice(0, 1) }}</span
          ><span class="brand-switch-text"
            ><small>{{ account ? "真实品牌" : "演示品牌" }}</small
            ><b>{{ account && !selectedBrandId ? "选择品牌" : brand }}</b></span
          ><span class="chevron">⌄</span>
        </button>
        <div v-if="brandMenu" class="brand-dropdown">
          <template v-if="account"
            ><button
              v-for="item in adminBrands"
              :key="item.id"
              @click="selectBrand(item.id)"
            >
              {{ item.name }} <span v-if="selectedBrandId === item.id">✓</span
              ><small>{{ item.code }} · {{ item.status }}</small>
            </button>
            <p v-if="!adminBrands.length">没有可访问的品牌</p></template
          ><template v-else
            ><button
              v-for="name in ['Aurora', 'Harbor']"
              :key="name"
              @click="
                demoBrand = name;
                brandMenu = false;
                toast(`当前本地原型品牌：${name}`);
              "
            >
              {{ name }}<small>仅原型演示</small>
            </button></template
          >
        </div>
      </div>
      <nav class="side-nav" aria-label="主导航">
        <template v-for="group in groups" :key="group"
          ><p class="nav-heading">{{ group }}</p>
          <button
            v-for="item in nav.filter((entry) => entry.group === group)"
            :key="item.name"
            class="nav-item"
            :class="{ active: page === item.name }"
            @click="go(item.name)"
          >
            <span class="nav-icon">{{ item.icon }}</span
            ><span>{{ item.name }}</span
            ><span v-if="item.name === '期次和开奖'" class="nav-count">2</span>
          </button></template
        >
      </nav>
      <div class="sidebar-bottom">
        <button class="support-link" @click="toast('帮助中心为演示入口')">
          ◌ <span>帮助与反馈</span><span class="external">↗</span>
        </button>
        <div class="profile">
          <div class="profile-avatar">
            {{ account ? account.id.slice(0, 1).toUpperCase() : "演" }}
          </div>
          <span class="profile-name"
            ><b>{{ account?.id ?? "未登录" }}</b
            ><small>{{
              account
                ? account.super_admin
                  ? "超级管理员"
                  : "管理员"
                : "原型演示"
            }}</small></span
          ><button
            v-if="account"
            class="dots"
            aria-label="退出登录"
            @click="logout"
          >
            退出</button
          ><button
            v-else
            class="dots"
            aria-label="账号菜单"
            @click="go('账号与权限')"
          >
            ···
          </button>
        </div>
      </div>
    </aside>

    <main class="main-shell">
      <header class="topbar">
        <div class="breadcrumbs">
          <span>运营控制台</span><span class="crumb-slash">/</span
          ><b>{{ page }}</b
          ><span
            v-if="
              page !== '用户和成员' &&
              page !== '审计日志' &&
              page !== '账号与权限'
            "
            class="prototype-badge"
            >演示原型</span
          >
        </div>
        <div class="top-actions">
          <label class="search-box"
            ><span>⌕</span
            ><input
              v-model="search"
              placeholder="搜索用户、注单、期次"
              aria-label="搜索"
            /><kbd>⌘ K</kbd></label
          ><button
            class="icon-button"
            aria-label="通知"
            @click="toast('当前没有新的演示通知')"
          >
            ♧<i class="notification-dot"></i></button
          ><span class="top-divider"></span
          ><button class="help-button" @click="toast('帮助中心为演示入口')">
            帮助中心 ↗
          </button>
        </div>
      </header>
      <div
        v-if="
          showDemoNotice &&
          page !== '用户和成员' &&
          page !== '审计日志' &&
          page !== '账号与权限'
        "
        class="demo-banner"
      >
        <span class="banner-icon">ⓘ</span
        ><span
          ><b>交互演示 · 非生产环境</b
          ><span class="banner-copy">
            积分、玩法审核、域名主题仍为虚构演示，不会写入后台。账号、成员和认证设置已接入真实
            API。</span
          ></span
        ><button aria-label="关闭说明" @click="showDemoNotice = false">
          ×
        </button>
      </div>
      <div class="context-strip" :class="contextState">
        <span class="context-indicator"></span
        ><template v-if="contextState === 'connected' && contextBrand"
          ><b>后端品牌上下文</b
          ><span>{{ contextBrand.name }} · {{ contextBrand.code }}</span
          ><span>{{ contextBrand.status }}</span
          ><span>{{ contextBrand.default_locale }}</span
          ><span>{{ contextBrand.timezone }}</span
          ><span>配置版本 {{ contextBrand.config_version }}</span></template
        ><template v-else-if="contextState === 'loading'"
          ><b>后端品牌上下文</b
          ><span>正在读取 GET /api/v1/context</span></template
        ><template v-else
          ><b>后端品牌上下文未连接</b
          ><span
            >只读接口 GET /api/v1/context
            暂不可用；该接口仅用于只读配置状态，不影响管理员认证和真实用户目录。</span
          ></template
        >
      </div>
      <div v-if="account" class="directory-brand-bar">
        <span>真实后台品牌</span
        ><select
          :value="selectedBrandId"
          aria-label="选择真实后台品牌"
          @change="selectBrand(($event.target as HTMLSelectElement).value)"
        >
          <option value="" disabled>请选择品牌</option>
          <option v-for="item in adminBrands" :key="item.id" :value="item.id">
            {{ item.name }} · {{ item.code }}
          </option></select
        ><small v-if="selectedBrandId">X-Brand-ID: {{ selectedBrandId }}</small
        ><button class="text-button" @click="logout">退出登录</button>
      </div>
      <div v-if="notice" class="toast" role="status">✓ &nbsp;{{ notice }}</div>

      <section v-if="page === '工作台'" class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">
              MONDAY, OCTOBER 5, 2026 <span>·</span> {{ brand.split("·")[0] }}
            </div>
            <h1>早上好，林岚 <span class="wave">✳</span></h1>
            <p>这是今天的运营概况，所有数据均为交互演示。</p>
          </div>
          <button
            class="button button-secondary"
            @click="toast('概览已刷新（演示数据保持不变）')"
          >
            ↻ <span>刷新概览</span>
          </button>
        </div>
        <div class="stats-grid">
          <article class="stat-card">
            <div class="stat-top">
              <span class="stat-label">进行中期次</span
              ><span class="stat-icon blue">◷</span>
            </div>
            <div class="stat-value">08 <small>期</small></div>
            <div class="stat-foot">
              <span class="status-dot green-dot"></span>投注开放中
              <span class="foot-muted">· 2 期即将截止</span>
            </div>
          </article>
          <article class="stat-card">
            <div class="stat-top">
              <span class="stat-label">今日投注量</span
              ><span class="stat-icon violet">▤</span>
            </div>
            <div class="stat-value">128,450<small> 分</small></div>
            <div class="stat-foot trend">
              ↗ 12.8% <span class="foot-muted">较昨日同期</span>
            </div>
          </article>
          <article class="stat-card">
            <div class="stat-top">
              <span class="stat-label">待处理事项</span
              ><span class="stat-icon amber">◉</span>
            </div>
            <div class="stat-value">14<small> 项</small></div>
            <div class="stat-foot">
              <span class="foot-muted">提现 </span><b>3</b
              ><span class="foot-muted">　异常注单 </span><b>2</b
              ><span class="foot-muted">　待开奖 </span><b>9</b>
            </div>
          </article>
          <article class="stat-card">
            <div class="stat-top">
              <span class="stat-label">账本对账差异</span
              ><span class="stat-icon rose">≋</span>
            </div>
            <div class="stat-value">0<small> 分</small></div>
            <div class="stat-foot">
              <span class="status-dot green-dot"></span>最近检查 10:32
              <span class="foot-muted">· 一切正常</span>
            </div>
          </article>
        </div>
        <div class="dashboard-grid">
          <article class="panel periods-panel">
            <div class="panel-header">
              <div>
                <h2>期次监控</h2>
                <p>当前品牌 · 实时状态演示</p>
              </div>
              <button class="text-button" @click="go('期次和开奖')">
                全部期次 <span>→</span>
              </button>
            </div>
            <div
              class="monitor-row"
              v-for="(p, i) in [
                {
                  game: '星彩 6+1',
                  period: '20261005032',
                  remain: '00:18:42',
                  orders: '1,284',
                  state: '投注中',
                },
                {
                  game: '幸运三位',
                  period: '20261005031',
                  remain: '00:06:18',
                  orders: '846',
                  state: '即将截止',
                },
                {
                  game: '星彩 6+1',
                  period: '20261005031',
                  remain: '待录入结果',
                  orders: '1,106',
                  state: '待开奖',
                },
              ]"
              :key="p.period + i"
            >
              <div class="game-avatar" :class="i === 1 ? 'mint' : ''">
                {{ i === 1 ? "3D" : "6+" }}
              </div>
              <div class="monitor-game">
                <b>{{ p.game }}</b
                ><small>{{ p.period }}</small>
              </div>
              <div class="monitor-orders">
                <b>{{ p.orders }}</b
                ><small>注单</small>
              </div>
              <div class="monitor-timer" :class="{ warn: i === 1 }">
                <b>{{ p.remain }}</b
                ><small>{{ i === 2 ? "等待开奖" : "剩余时间" }}</small>
              </div>
              <span
                class="badge"
                :class="
                  i === 2
                    ? 'badge-neutral'
                    : i === 1
                      ? 'badge-warn'
                      : 'badge-success'
                "
                >{{ p.state }}</span
              >
            </div>
          </article>
          <article class="panel action-panel">
            <div class="panel-header">
              <div>
                <h2>待处理</h2>
                <p>需要你关注的事项</p>
              </div>
              <span class="count-pill">14</span>
            </div>
            <button class="task-row" @click="go('资金与账本')">
              <span class="task-icon task-amber">↓</span
              ><span class="task-copy"
                ><b>提现申请待审核</b><small>3 笔申请 · 最近 09:42</small></span
              ><span class="task-arrow">›</span></button
            ><button class="task-row" @click="go('注单和异常')">
              <span class="task-icon task-rose">!</span
              ><span class="task-copy"
                ><b>异常注单待处理</b><small>2 笔订单需要核查</small></span
              ><span class="task-arrow">›</span></button
            ><button class="task-row" @click="go('期次和开奖')">
              <span class="task-icon task-blue">◷</span
              ><span class="task-copy"
                ><b>期次等待开奖</b><small>9 期期待结果确认</small></span
              ><span class="task-arrow">›</span>
            </button>
          </article>
        </div>
        <div class="lower-grid">
          <article class="panel volume-panel">
            <div class="panel-header">
              <div>
                <h2>投注趋势</h2>
                <p>每日总投注积分</p>
              </div>
              <select v-model="selectedRange" aria-label="趋势时间范围">
                <option>近 7 天</option>
                <option>近 30 天</option>
              </select>
            </div>
            <div class="chart">
              <div class="chart-y">
                <span>160k</span><span>120k</span><span>80k</span
                ><span>40k</span><span>0</span>
              </div>
              <div class="chart-plot">
                <div class="chart-gridline" v-for="i in 5" :key="i"></div>
                <svg
                  viewBox="0 0 700 176"
                  preserveAspectRatio="none"
                  role="img"
                  aria-label="近七日投注量趋势"
                >
                  <defs>
                    <linearGradient id="area" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0" stop-color="#6c7cf4" stop-opacity=".2" />
                      <stop offset="1" stop-color="#6c7cf4" stop-opacity="0" />
                    </linearGradient>
                  </defs>
                  <path
                    d="M0 133 C45 125 64 107 104 114 S168 127 207 91 S275 102 311 74 S375 82 415 64 S485 80 520 40 S588 59 622 24 S672 40 700 11 L700 176 L0 176Z"
                    fill="url(#area)"
                  />
                  <path
                    d="M0 133 C45 125 64 107 104 114 S168 127 207 91 S275 102 311 74 S375 82 415 64 S485 80 520 40 S588 59 622 24 S672 40 700 11"
                    fill="none"
                    stroke="#6979ef"
                    stroke-width="2.5"
                    vector-effect="non-scaling-stroke"
                  />
                </svg>
                <div class="chart-x">
                  <span>09/29</span><span>09/30</span><span>10/01</span
                  ><span>10/02</span><span>10/03</span><span>10/04</span
                  ><span>10/05</span>
                </div>
              </div>
            </div>
          </article>
          <article class="panel source-panel">
            <div class="panel-header">
              <div>
                <h2>开奖源健康</h2>
                <p>数据源连接状态演示</p>
              </div>
              <span class="live-pill"><i></i>正常</span>
            </div>
            <div class="source-row">
              <span class="source-mark">A</span
              ><span><b>主开奖源</b><small>api.drawsource.example</small></span
              ><span class="source-latency">142 ms</span
              ><span class="status-dot green-dot"></span>
            </div>
            <div class="source-row">
              <span class="source-mark source-alt">B</span
              ><span
                ><b>备用开奖源</b><small>backup.results.example</small></span
              ><span class="source-latency">208 ms</span
              ><span class="status-dot green-dot"></span>
            </div>
            <div class="source-foot">
              最近检查 <b>10:32:18</b><span>·</span>连续可用 <b>99.98%</b>
            </div>
          </article>
        </div>
      </section>

      <section v-else-if="page === '品牌和域名'" class="page-content">
        <AuthSettings
          v-if="account && selectedBrandId"
          :key="selectedBrandId"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div class="page-heading">
          <div>
            <div class="eyebrow">PLATFORM / BRAND CONFIG</div>
            <h1>品牌和域名</h1>
            <p>管理品牌主题、域名及生效版本</p>
          </div>
          <button
            class="button button-primary"
            @click="toast('新建品牌为演示入口')"
          >
            ＋ 新建品牌
          </button>
        </div>
        <article class="panel">
          <div class="table-toolbar">
            <div class="filter-tabs">
              <button class="selected">全部品牌 <span>3</span></button
              ><button>运行中</button><button>已暂停</button>
            </div>
            <button
              class="button button-secondary"
              @click="toast('配置导出成功（演示）')"
            >
              导出配置 ↓
            </button>
          </div>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>品牌</th>
                  <th>主域名</th>
                  <th>状态</th>
                  <th>默认语言</th>
                  <th>时区</th>
                  <th>配置版本</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="b in [
                    {
                      name: 'Aurora',
                      domain: 'aurora.demo',
                      state: '运行中',
                      locale: '简体中文',
                      tz: 'Asia/Singapore',
                      version: 'v12',
                    },
                    {
                      name: 'Harbor · 月湾',
                      domain: 'harbor.demo',
                      state: '运行中',
                      locale: 'English',
                      tz: 'Asia/Kuala_Lumpur',
                      version: 'v8',
                    },
                    {
                      name: 'Harbor Secondary · 晴野',
                      domain: 'sunfield.demo',
                      state: '已暂停',
                      locale: '繁體中文',
                      tz: 'Asia/Taipei',
                      version: 'v4',
                    },
                  ]"
                  :key="b.name"
                >
                  <td>
                    <span class="table-brand"
                      ><i>{{ b.name[0] }}</i
                      ><b>{{ b.name }}</b></span
                    >
                  </td>
                  <td class="mono">{{ b.domain }}</td>
                  <td>
                    <span
                      class="badge"
                      :class="
                        b.state === '运行中' ? 'badge-success' : 'badge-neutral'
                      "
                      >{{ b.state }}</span
                    >
                  </td>
                  <td>{{ b.locale }}</td>
                  <td>{{ b.tz }}</td>
                  <td>
                    <span class="version-tag">{{ b.version }}</span>
                  </td>
                  <td>
                    <button
                      class="text-button"
                      @click="toast(`打开 ${b.name} 配置（演示）`)"
                    >
                      管理 →
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="panel-foot">
            平台默认 → 品牌覆盖　·　配置发布后清理缓存并返回新版本号
            <span>数据仅用于演示</span>
          </div>
        </article>
      </section>

      <section
        v-else-if="page === '用户和成员'"
        class="page-content directory-page"
      >
        <div class="page-heading">
          <div>
            <div class="eyebrow">MEMBERS / LIVE DIRECTORY</div>
            <h1>用户和成员</h1>
            <p>
              此页显示真实成员；账号权限和认证设置也已接入，资金与游戏页面仍为原型。
            </p>
          </div>
          <span v-if="account" class="live-pill"
            >已登录 · {{ account.id }}</span
          >
        </div>
        <div v-if="authLoading" class="panel directory-state">
          正在检查管理员登录状态…
        </div>
        <article v-else-if="!account" class="panel auth-panel">
          <div class="auth-copy">
            <div class="eyebrow">ADMIN AUTHENTICATION</div>
            <h2>管理员登录</h2>
            <p>
              使用后台管理员账号登录。会话由同源 HttpOnly Cookie
              维护，此页面不会保存访问令牌。
            </p>
          </div>
          <form class="auth-form" @submit.prevent="login">
            <label class="modal-label"
              >账号<input
                v-model="loginIdentifier"
                class="field"
                autocomplete="username"
                required /></label
            ><label class="modal-label"
              >密码<input
                v-model="loginPassword"
                class="field"
                type="password"
                autocomplete="current-password"
                required
            /></label>
            <p v-if="authError" class="form-error" role="alert">
              {{ authError }}
            </p>
            <button class="button button-primary" :disabled="loginBusy">
              {{ loginBusy ? "登录中…" : "登录并加载真实成员" }}
            </button>
          </form>
        </article>
        <template v-else>
          <div v-if="authError" class="directory-error" role="alert">
            {{ authError }}
          </div>
          <div v-if="!selectedBrandId" class="panel directory-state">
            <b>请选择真实品牌</b
            ><span
              >成员请求不会使用后端默认品牌。请从侧栏品牌选择器中选择一个有权访问的品牌。</span
            ><button class="button button-secondary" @click="brandMenu = true">
              选择品牌
            </button>
          </div>
          <template v-else>
            <MemberProvision
              :key="selectedBrandId"
              :account="account"
              :brand-id="selectedBrandId"
              @created="loadMembers"
              @session-invalid="clearAdminData"
            />
            <div class="directory-toolbar">
              <div>
                <span>当前真实品牌</span><b>{{ brand }}</b
                ><small>{{ selectedBrandId }}</small>
              </div>
              <button
                class="button button-secondary"
                :disabled="membersLoading"
                @click="loadMembers"
              >
                {{ membersLoading ? "读取中…" : "刷新成员" }}
              </button>
            </div>
            <div v-if="membersError" class="directory-error" role="alert">
              {{ membersError }}
            </div>
            <article class="panel">
              <div class="table-toolbar">
                <div class="filter-tabs">
                  <span class="selected"
                    >品牌成员 <span>{{ members.length }} 条本页</span></span
                  >
                </div>
                <div class="toolbar-controls">
                  <input
                    class="field search-field"
                    placeholder="搜索用户名 / 手机号 / ID"
                    v-model="search"
                    aria-label="搜索成员"
                  />
                </div>
              </div>
              <div v-if="membersLoading" class="directory-state">
                正在读取品牌成员…
              </div>
              <div v-else-if="!members.length" class="directory-state">
                该品牌当前没有可显示的成员。
              </div>
              <div v-else class="table-wrap">
                <table class="member-directory-table">
                  <thead>
                    <tr>
                      <th>成员</th>
                      <th>成员 ID</th>
                      <th>手机号</th>
                      <th>状态</th>
                      <th>加入时间</th>
                      <th>备注</th>
                      <th>标签</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="m in members.filter(
                        (item) =>
                          !search ||
                          `${item.display_name} ${item.username} ${item.phone} ${item.id} ${item.global_user_id}`
                            .toLowerCase()
                            .includes(search.toLowerCase()),
                      )"
                      :key="m.id"
                    >
                      <td>
                        <span class="member-cell"
                          ><i>{{
                            (m.display_name || m.username || "?").slice(0, 1)
                          }}</i
                          ><span
                            ><b>{{ m.display_name || m.username }}</b
                            ><small
                              >@{{ m.username }} · {{ m.global_user_id }}</small
                            ></span
                          ></span
                        >
                      </td>
                      <td class="mono">{{ m.id }}</td>
                      <td>{{ m.phone || "—" }}</td>
                      <td>
                        <span class="badge" :class="statusClass(m.status)">{{
                          statusLabel[m.status]
                        }}</span>
                      </td>
                      <td>{{ new Date(m.joined_at).toLocaleString() }}</td>
                      <td class="member-notes">{{ m.notes || "—" }}</td>
                      <td>
                        <span
                          v-for="tag in m.tags"
                          :key="tag"
                          class="member-tag"
                          >{{ tag }}</span
                        ><span v-if="!m.tags.length">—</span>
                      </td>
                      <td>
                        <div class="member-actions">
                          <button
                            class="text-button"
                            :disabled="!canEditUsers"
                            :title="
                              account.super_admin
                                ? '超级管理员不可编辑成员状态或备注'
                                : !canEditUsers
                                  ? '缺少 user.write.brand 权限'
                                  : ''
                            "
                            @click="openEdit(m)"
                          >
                            编辑</button
                          ><button
                            class="text-button"
                            :disabled="!canKickUsers"
                            :title="
                              account.super_admin
                                ? '超级管理员不可踢出成员'
                                : !canKickUsers
                                  ? '缺少 user.kick.brand 权限'
                                  : ''
                            "
                            @click="openKick(m)"
                          >
                            踢出</button
                          ><button
                            class="text-button"
                            :disabled="!canResetPasswords"
                            :title="
                              !canResetPasswords
                                ? '缺少 user.password_reset.brand 权限'
                                : ''
                            "
                            @click="openReset(m)"
                          >
                            重置密码
                          </button>
                        </div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <div class="pagination">
                <span
                  >第 {{ Math.floor(memberOffset / 100) + 1 }} 页 · 每页最多 100
                  条</span
                ><button
                  :disabled="memberOffset === 0 || membersLoading"
                  @click="changeMemberPage(-1)"
                >
                  上一页</button
                ><button
                  :disabled="members.length < 100 || membersLoading"
                  @click="changeMemberPage(1)"
                >
                  下一页
                </button>
              </div>
            </article>
          </template>
        </template>
      </section>

      <section v-else-if="page === '代理树'" class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">MEMBERS / AGENT NETWORK</div>
            <h1>代理树</h1>
            <p>品牌内代理关系与团队表现</p>
          </div>
          <button
            class="button button-primary"
            @click="toast('创建代理为演示入口')"
          >
            ＋ 新建代理
          </button>
        </div>
        <div class="agent-summary">
          <div class="panel agent-tree">
            <div class="panel-header">
              <div>
                <h2>团队关系</h2>
                <p>共 384 位代理 · 点击展开下级</p>
              </div>
              <button class="text-button" @click="toast('已切换为紧凑视图')">
                ⊞ 紧凑视图
              </button>
            </div>
            <div class="tree-root">
              <div class="tree-node platform-node">
                <i>NS</i
                ><span><b>Aurora 平台</b><small>24,861 位成员</small></span
                ><span class="node-level">平台</span>
              </div>
              <div class="tree-branch">
                <div class="tree-node">
                  <i class="agent-indigo">周</i
                  ><span
                    ><b>周宁 · 星河-01</b
                    ><small>直属 86 人 · 团队 1,842 人</small></span
                  ><button @click="toast('代理详情（演示）')">⌄</button>
                </div>
                <div class="tree-node">
                  <i class="agent-teal">陈</i
                  ><span
                    ><b>陈浩 · 海风-03</b
                    ><small>直属 42 人 · 团队 956 人</small></span
                  ><button @click="toast('代理详情（演示）')">⌄</button>
                </div>
                <div class="tree-node">
                  <i class="agent-orange">林</i
                  ><span
                    ><b>林美 · 晨光-12</b
                    ><small>直属 28 人 · 团队 408 人</small></span
                  ><button @click="toast('代理详情（演示）')">⌄</button>
                </div>
              </div>
            </div>
          </div>
          <div class="agent-side">
            <article class="panel agent-kpi">
              <div class="eyebrow">本周期 · 演示</div>
              <h2>佣金概况</h2>
              <strong>18,420 <small>分</small></strong>
              <p>较上周期 <span class="trend">↗ 8.4%</span></p>
              <button class="button button-secondary" @click="go('佣金和奖励')">
                查看佣金明细 →
              </button>
            </article>
            <article class="panel agent-alert">
              <span>ⓘ</span>
              <div>
                <b>代理层级提示</b>
                <p>当前品牌最多支持 5 级代理，请在演示配置中查看层级策略。</p>
              </div>
            </article>
          </div>
        </div>
      </section>

      <section v-else-if="page === '规则配置'" class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">GAMES / RULE WORKBENCH</div>
            <h1>规则配置</h1>
            <p>创建、模拟并提交不可变规则版本供品牌管理员审核</p>
          </div>
          <button
            class="button button-secondary"
            @click="toast('已打开规则版本列表（演示）')"
          >
            版本历史 ↗
          </button>
        </div>
        <div class="rule-layout">
          <article class="panel rule-editor">
            <div class="rule-top">
              <div>
                <span class="badge badge-neutral">草稿 · v13</span>
                <h2>星彩 6+1 · 特别号命中</h2>
                <p>适用品牌：{{ brand }} <span>·</span> 生效方式：下一期期次</p>
              </div>
              <button
                class="save-status"
                @click="toast('草稿保存在本次页面会话中')"
              >
                ● 自动保存演示
              </button>
            </div>
            <div v-if="isMobile" class="mobile-readonly">
              ⌁ 移动端规则编辑为只读，可查看模拟并进行审核操作。
            </div>
            <div class="stepper">
              <button
                v-for="i in 8"
                :key="i"
                :class="{ done: i < ruleStep, active: i === ruleStep }"
                @click="ruleStep = i"
              >
                <span>{{ i < ruleStep ? "✓" : i }}</span
                ><small>{{
                  [
                    "彩种模型",
                    "号码配置",
                    "选号规则",
                    "中奖条件",
                    "奖级赔率",
                    "限额取消",
                    "测试案例",
                    "模拟结果",
                  ][i - 1]
                }}</small>
              </button>
            </div>
            <fieldset :disabled="isMobile" class="rule-fieldset">
              <template v-if="ruleStep === 1"
                ><div class="form-section">
                  <h3>1. 选择彩种模型</h3>
                  <p>模型确定号码池结构和基本校验方式。</p>
                  <div class="model-options">
                    <button class="model-option selected" @click.prevent>
                      <b>X + Y</b><small>普通号码与特别号码分别选择</small
                      ><span>当前模型</span></button
                    ><button class="model-option" @click.prevent>
                      <b>M 选 N</b><small>从号码池中选取指定数量</small></button
                    ><button class="model-option" @click.prevent>
                      <b>0–9 数字位</b><small>按位置选择有序数字</small>
                    </button>
                  </div>
                </div>
                <div class="form-grid">
                  <label>彩种名称<input class="field" value="星彩 6+1" /></label
                  ><label
                    >玩法名称<input class="field" value="特别号命中"
                  /></label></div
              ></template>
              <template v-else-if="ruleStep === 2"
                ><div class="form-section">
                  <h3>2. 配置号码池和位数</h3>
                  <p>设置合法取值、重复规则及普通 / 特别号码数量。</p>
                </div>
                <div class="form-grid">
                  <label
                    >普通号码范围<input class="field" value="1 – 49" /></label
                  ><label>普通号码个数<input class="field" value="6" /></label
                  ><label
                    >特别号码范围<input class="field" value="1 – 10" /></label
                  ><label>特别号码个数<input class="field" value="1" /></label>
                </div>
                <label class="check-row"
                  ><input type="checkbox" checked /> 不允许号码重复</label
                ></template
              >
              <template v-else-if="ruleStep === 3"
                ><div class="form-section">
                  <h3>3. 配置选号与组合方式</h3>
                  <p>设置排除、复式展开、号码属性和倍投规则。</p>
                </div>
                <div class="form-grid">
                  <label
                    >选号方式<select class="field">
                      <option>手动选号 / 复式</option>
                      <option>单式选号</option>
                    </select></label
                  ><label
                    >单注积分<input
                      class="field"
                      v-model="ruleDraft.unitPoints"
                      inputmode="numeric" /></label
                  ><label>倍数下限<input class="field" value="1" /></label
                  ><label>倍数上限<input class="field" value="100" /></label>
                </div>
                <label class="check-row"
                  ><input type="checkbox" checked /> 支持排除号码　　<input
                    type="checkbox"
                    checked
                  />
                  支持奇偶属性</label
                ></template
              >
              <template v-else-if="ruleStep === 4"
                ><div class="form-section">
                  <h3>4. 配置中奖条件和结果特征</h3>
                  <p>仅允许使用规则组件，不执行自定义脚本。</p>
                </div>
                <div class="condition-row">
                  <span class="condition-index">01</span
                  ><select class="field">
                    <option>特别号码命中</option></select
                  ><select class="field">
                    <option>等于</option>
                    <option>包含</option></select
                  ><span class="condition-value">特别号码</span
                  ><button class="more-action" @click.prevent>···</button>
                </div>
                <button class="button button-secondary" @click.prevent>
                  ＋ 添加条件组件
                </button></template
              >
              <template v-else-if="ruleStep === 5"
                ><div class="form-section">
                  <h3>5. 配置奖级、赔率和舍入</h3>
                  <p>奖级优先级、排他性、封顶值和舍入策略。</p>
                </div>
                <div class="tier-row">
                  <b>特别号命中</b><span>倍率</span
                  ><input class="field" value="40" /><label
                    ><input type="checkbox" checked /> 排他</label
                  ><input class="field" value="封顶 50,000" />
                </div>
                <div class="tier-row">
                  <b>普通号命中</b><span>倍率</span
                  ><input class="field" value="2" /><label
                    ><input type="checkbox" /> 排他</label
                  ><input class="field" value="不封顶" />
                </div>
                <div class="rounding-note">
                  舍入规则 <b>四舍五入 · 整数积分</b>
                </div></template
              >
              <template v-else-if="ruleStep === 6"
                ><div class="form-section">
                  <h3>6. 设置限额与取消规则</h3>
                  <p>提供用户、单注和单期期次边界。</p>
                </div>
                <div class="form-grid">
                  <label>单注最低<input class="field" value="1" /></label
                  ><label>单注最高<input class="field" value="10,000" /></label
                  ><label
                    >每人每期限额<input class="field" value="50,000" /></label
                  ><label
                    >截止前取消<select class="field">
                      <option>允许，原路返还</option>
                      <option>不允许</option>
                    </select></label
                  >
                </div></template
              >
              <template v-else-if="ruleStep === 7"
                ><div class="form-section">
                  <h3>7. 输入测试选号和开奖结果</h3>
                  <p>
                    本地原型模拟，不是真实规则引擎；不会创建注单或写入数据。
                  </p>
                </div>
                <div class="form-grid">
                  <label
                    >测试选号（空格分隔）<input
                      class="field"
                      v-model="ruleDraft.testSelection" /></label
                  ><label
                    >测试开奖结果<input
                      class="field"
                      v-model="ruleDraft.testResult" /></label
                  ><label
                    >倍数<input
                      class="field"
                      v-model="ruleDraft.multiplier"
                      inputmode="numeric" /></label
                  ><label
                    >单注积分（整数）<input
                      class="field"
                      v-model="ruleDraft.unitPoints"
                      inputmode="numeric"
                  /></label>
                </div>
                <button class="button button-primary" @click.prevent="simulate">
                  ▷ 运行模拟
                </button></template
              >
              <template v-else
                ><div class="simulation-result">
                  <div class="simulation-head">
                    <span class="success-check">✓</span
                    ><span
                      ><b>规则模拟完成 · 本地原型</b
                      ><small
                        >不是实际规则引擎；结果不会写入注单或账本。</small
                      ></span
                    >
                  </div>
                  <div class="sim-stats">
                    <div><small>规则校验</small><b>通过</b></div>
                    <div>
                      <small>展开组合数</small
                      ><b>{{ simulation.combinations }}</b>
                    </div>
                    <div>
                      <small>命中说明</small><b>{{ simulation.matches }} 项</b>
                    </div>
                    <div>
                      <small>预计积分</small
                      ><b>{{ simulation.points }} <small>分</small></b>
                    </div>
                  </div>
                  <div class="simulation-explain">
                    <b>条件节点</b><span>✓ 号码范围与重复检查</span
                    ><span>✓ {{ simulation.explanation }}</span
                    ><span>✓ 封顶后积分与整数舍入</span>
                  </div>
                </div></template
              >
            </fieldset>
            <div class="rule-actions">
              <button
                class="button button-secondary"
                @click="ruleStep = Math.max(1, ruleStep - 1)"
              >
                ← 上一步</button
              ><span>步骤 {{ ruleStep }} / 8</span>
              <div>
                <button
                  class="button button-secondary"
                  @click="toast('规则草稿已暂存（当前页面演示）')"
                >
                  保存草稿</button
                ><button
                  v-if="ruleStep < 8"
                  class="button button-primary"
                  @click="ruleStep++"
                >
                  下一步 →</button
                ><button
                  v-else
                  class="button button-primary"
                  :disabled="ruleDraft.status === 'pending_review'"
                  @click="submitRule"
                >
                  提交审核 →
                </button>
              </div>
            </div>
          </article>
          <aside class="rule-side">
            <article class="panel approval-card">
              <div class="side-card-icon">✓</div>
              <h3>审核与发布</h3>
              <p>规则创建者不能审核自己的版本。审核通过后还需单独确认发布。</p>
              <div class="approval-line">
                <span class="avatar-small">林</span
                ><span
                  ><b>创建人：林岚</b
                  ><small>{{
                    ruleDraft.status === "draft" ? "当前操作者" : "已提交审核"
                  }}</small></span
                >
              </div>
              <div class="approval-line">
                <span class="avatar-small reviewer">周</span
                ><span
                  ><b>审核人：周宁</b
                  ><small>{{
                    ruleDraft.status === "pending_review"
                      ? "待审核 · 与创建者不同"
                      : "待分配审核"
                  }}</small></span
                >
              </div>
              <template v-if="ruleDraft.status === 'pending_review'"
                ><button
                  class="button button-primary full-button"
                  @click="decisionRule('approve')"
                >
                  ✓ 审核通过</button
                ><button
                  class="button button-secondary full-button"
                  @click="decisionRule('reject')"
                >
                  驳回并填写意见
                </button></template
              ><button
                v-if="ruleDraft.status === 'approved'"
                class="button button-primary full-button"
                @click="toast('已创建待确认发布版本；演示不会改变实际配置')"
              >
                确认发布（演示）
              </button>
              <div class="approval-note">
                ⓘ 发布后新版本仅作用于新注单，历史订单继续使用原版本。
              </div>
            </article>
            <article class="panel version-card">
              <h3>版本信息</h3>
              <div><span>当前生效版本</span><b>v12</b></div>
              <div><span>新建草稿</span><b>v13</b></div>
              <div><span>生效方式</span><b>下一期期次</b></div>
              <button
                class="text-button"
                @click="toast('查看版本差异（演示）')"
              >
                对比版本差异 →
              </button>
            </article>
          </aside>
        </div>
      </section>

      <section v-else-if="page === '期次和开奖'" class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">DRAW / PERIOD CONTROL</div>
            <h1>期次和开奖</h1>
            <p>查看期次时间线、来源校验与开奖操作</p>
          </div>
          <div class="heading-actions">
            <button class="button button-secondary" @click="manualOpen = true">
              ＋ 人工开奖</button
            ><button
              class="button button-primary"
              @click="toast('已生成下一期草稿（演示）')"
            >
              ⟳ 生成期次
            </button>
          </div>
        </div>
        <div class="period-summary">
          <div class="panel period-main">
            <div class="period-title">
              <div>
                <span class="badge badge-success">投注中</span>
                <h2>星彩 6+1 <small>20261005032</small></h2>
              </div>
              <button class="text-button" @click="toast('已复制期次编号')">
                复制编号 ⧉
              </button>
            </div>
            <div class="timeline">
              <div class="timeline-stage complete">
                <i>✓</i
                ><span><b>投注开始</b><small>10-05 08:00:00</small></span>
              </div>
              <div class="timeline-connector active"></div>
              <div class="timeline-stage current">
                <i>2</i
                ><span
                  ><b>投注截止</b
                  ><small>10-05 11:00:00 · 18 分钟后</small></span
                >
              </div>
              <div class="timeline-connector"></div>
              <div class="timeline-stage">
                <i>3</i
                ><span><b>开奖时间</b><small>10-05 11:10:00</small></span>
              </div>
              <div class="timeline-connector"></div>
              <div class="timeline-stage">
                <i>4</i><span><b>结算完成</b><small>等待开奖</small></span>
              </div>
            </div>
            <div class="period-kpis">
              <div>
                <small>投注订单</small><b>1,284 <small>笔</small></b>
              </div>
              <div>
                <small>投注积分</small><b>28,450 <small>分</small></b>
              </div>
              <div>
                <small>关联规则</small><b>v12 <small>· 6+1</small></b>
              </div>
              <div>
                <small>品牌时区</small><b>UTC+08:00 <small>· 新加坡</small></b>
              </div>
            </div>
          </div>
          <aside class="panel source-health">
            <div class="panel-header">
              <div>
                <h2>开奖来源</h2>
                <p>优先级与最近检查时间</p>
              </div>
              <button
                class="text-button"
                @click="toast('来源切换记录（演示）')"
              >
                切换记录
              </button>
            </div>
            <div class="source-row">
              <span class="source-mark">1</span
              ><span><b>主来源 · API</b><small>上次校验 10:32:18</small></span
              ><span class="badge badge-success">健康</span>
            </div>
            <div class="source-row">
              <span class="source-mark source-alt">2</span
              ><span><b>备用来源 · API</b><small>上次校验 10:31:54</small></span
              ><span class="badge badge-success">健康</span>
            </div>
            <div class="source-row">
              <span class="source-mark source-manual">M</span
              ><span
                ><b>人工录入</b><small>单人操作，必须记录审计原因</small></span
              ><button class="text-button" @click="manualOpen = true">
                录入
              </button>
            </div>
          </aside>
        </div>
        <div class="result-layout">
          <article class="panel result-card">
            <div class="panel-header">
              <div>
                <h2>当前开奖结果</h2>
                <p>上一期 · 20261005031</p>
              </div>
              <span class="badge badge-success">已确认</span>
            </div>
            <div class="result-balls">
              <span v-for="n in ['07', '18', '29', '33', '41']" :key="n">{{
                n
              }}</span
              ><i>+</i><span class="special-ball">06</span>
            </div>
            <div class="result-meta">
              <span>来源：主开奖源</span
              ><span>校验项：号码范围 ✓　重复校验 ✓　签名 ✓</span
              ><span>确认时间：10-05 08:10:14</span>
            </div>
            <div class="result-actions">
              <button
                class="button button-secondary"
                @click="correctionOpen = true"
              >
                ↻ 纠正开奖结果</button
              ><button
                class="button button-primary"
                @click="toast('结算任务已加入队列（演示）')"
              >
                确认并结算 →
              </button>
            </div>
          </article>
          <article class="panel timeline-card">
            <h2>状态时间线</h2>
            <div class="audit-timeline">
              <div>
                <i class="event-green">✓</i
                ><span
                  ><b>开奖结果已确认</b><small>周宁 · 10-05 08:10</small></span
                >
              </div>
              <div>
                <i>↻</i
                ><span
                  ><b>来源结果通过校验</b
                  ><small>系统演示 · 10-05 08:09</small></span
                >
              </div>
              <div>
                <i>◷</i
                ><span
                  ><b>期次投注已截止</b
                  ><small>系统演示 · 10-05 08:00</small></span
                >
              </div>
            </div>
          </article>
        </div>
        <div class="danger-note">
          ⚠
          人工开奖和结果纠正均属于高风险演示操作。确认前必须查看影响范围并填写原因。
        </div>
      </section>

      <section v-else-if="page === '注单和异常'" class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">ORDERS / MONITORING</div>
            <h1>注单和异常</h1>
            <p>筛选品牌注单，检查异常并查看结算状态</p>
          </div>
          <button
            class="button button-secondary"
            @click="toast('已导出当前筛选（演示）')"
          >
            导出 ↓
          </button>
        </div>
        <div class="metric-strip">
          <div>
            <small>今日注单</small><b>18,642</b><span>总投注 128,450 分</span>
          </div>
          <div><small>待开奖</small><b>1,284</b><span>关联 8 个期次</span></div>
          <div>
            <small>异常注单</small><b class="danger-number">2</b
            ><span>需人工检查</span>
          </div>
          <div>
            <small>结算失败</small><b>0</b
            ><span class="live-text">● 无失败任务</span>
          </div>
        </div>
        <article class="panel">
          <div class="table-toolbar">
            <div class="filter-tabs">
              <button
                v-for="t in [
                  '全部',
                  '待开奖',
                  '已中奖',
                  '未中奖',
                  '异常注单',
                  '已取消',
                ]"
                :key="t"
                :class="{ selected: queryTab === t }"
                @click="queryTab = t"
              >
                {{ t }} <span v-if="t === '异常注单'">2</span>
              </button>
            </div>
          </div>
          <div class="toolbar-controls order-filters">
            <input
              class="field search-field"
              placeholder="注单号 / 用户 ID"
              v-model="search"
            /><select class="field">
              <option>全部彩种</option>
              <option>星彩 6+1</option>
              <option>幸运三位</option></select
            ><select class="field">
              <option>最近 7 天</option>
              <option>今天</option></select
            ><button class="button button-secondary">⌕ 筛选</button>
          </div>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>注单号 / 用户</th>
                  <th>彩种 / 期次</th>
                  <th>选号快照</th>
                  <th>投注积分</th>
                  <th>状态</th>
                  <th>下单时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="o in orders.filter(
                    (item) =>
                      (!search ||
                        `${item.id} ${item.member}`
                          .toLowerCase()
                          .includes(search.toLowerCase())) &&
                      (queryTab === '全部' || item.status === queryTab),
                  )"
                  :key="o.id"
                >
                  <td>
                    <b class="mono">{{ o.id }}</b
                    ><small class="cell-sub">{{ o.member }}</small>
                  </td>
                  <td>
                    {{ o.game
                    }}<small class="cell-sub mono">{{ o.period }}</small>
                  </td>
                  <td class="selection-cell">{{ o.picks }}</td>
                  <td class="amount">{{ o.amount }} <small>分</small></td>
                  <td>
                    <span
                      class="badge"
                      :class="
                        o.status === '异常注单'
                          ? 'badge-danger'
                          : o.status === '已中奖'
                            ? 'badge-success'
                            : 'badge-neutral'
                      "
                      >{{ o.status }}</span
                    >
                  </td>
                  <td>{{ o.time }}</td>
                  <td>
                    <button
                      class="text-button"
                      @click="toast(`${o.id} 详情已打开（演示）`)"
                    >
                      {{ o.status === "异常注单" ? "检查" : "详情" }} →
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="panel-foot">
            注单保留原始选号和规则版本快照，历史订单不可修改
            <span>共 18,642 条</span>
          </div>
        </article>
      </section>

      <section v-else-if="page === '资金与账本'" class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">POINTS / LEDGER & REVIEWS</div>
            <h1>资金与账本</h1>
            <p>积分账本为追加记录；审核动作需要理由（仅演示）</p>
          </div>
          <button
            class="button button-secondary"
            @click="toast('人工调整需创建独立调整流水（演示入口）')"
          >
            ＋ 创建调整流水
          </button>
        </div>
        <div class="money-cards">
          <article class="panel money-card">
            <span>积分余额汇总 <i>ⓘ</i></span
            ><b>8,428,640 <small>分</small></b
            ><small>3,284 个品牌成员账户</small>
          </article>
          <article class="panel money-card">
            <span>今日充值</span><b>124,800 <small>分</small></b
            ><small class="positive">↑ 8.2% 较昨日</small>
          </article>
          <article class="panel money-card">
            <span>待审提现 <i class="count-pill">3</i></span
            ><b>4,170 <small>分</small></b
            ><button class="inline-link" @click="ledgerTab = '提现审核'">
              查看待审核 →
            </button>
          </article>
          <article class="panel money-card">
            <span>账本对账状态</span><b class="balance-ok"><i>✓</i> 一致</b
            ><small>最近检查 10-05 10:32</small>
          </article>
        </div>
        <article class="panel finance-panel">
          <div class="table-toolbar">
            <div class="filter-tabs">
              <button
                v-for="t in ['账本流水', '充值审核', '提现审核']"
                :key="t"
                :class="{ selected: ledgerTab === t }"
                @click="ledgerTab = t"
              >
                {{ t }} <span v-if="t === '提现审核'">3</span>
              </button>
            </div>
            <button
              class="button button-secondary"
              @click="toast('流水报表导出完成（演示）')"
            >
              导出流水 ↓
            </button>
          </div>
          <template v-if="ledgerTab === '提现审核'"
            ><div class="withdraw-note">
              提现积分冻结至审核结束。批准、驳回均要求填写原因；审核结果仅在当前演示会话中生效。
            </div>
            <div class="withdraw-list">
              <div v-for="w in withdrawals" :key="w.id" class="withdraw-row">
                <div class="withdraw-person">
                  <i>{{ w.user[0] }}</i
                  ><span
                    ><b>{{ w.user }}</b
                    ><small class="mono"
                      >{{ w.id }} · {{ w.submitted }}</small
                    ></span
                  >
                </div>
                <div>
                  <small>申请积分</small
                  ><b class="withdraw-amount"
                    >{{ w.points }} <small>分</small></b
                  >
                </div>
                <div>
                  <small>积分来源</small><b>{{ w.source }}</b>
                </div>
                <span
                  class="badge"
                  :class="
                    w.status === '待审核'
                      ? 'badge-warn'
                      : w.status.startsWith('已通过')
                        ? 'badge-success'
                        : 'badge-neutral'
                  "
                  >{{ w.status }}</span
                >
                <div v-if="w.status === '待审核'" class="withdraw-actions">
                  <button
                    class="button button-secondary"
                    @click="reviewTarget = w"
                  >
                    审核
                  </button>
                </div>
                <div v-else class="withdraw-actions">
                  <button
                    class="text-button"
                    @click="toast(`审核原因：${w.reason}`)"
                  >
                    查看原因
                  </button>
                </div>
              </div>
            </div></template
          ><template v-else-if="ledgerTab === '账本流水'"
            ><div class="toolbar-controls ledger-filters">
              <input
                class="field search-field"
                placeholder="用户 / 来源 ID / Request ID"
                v-model="search"
              /><select class="field">
                <option>全部业务类型</option>
                <option>投注扣减</option>
                <option>人工充值</option></select
              ><button class="button button-secondary">筛选</button>
            </div>
            <div class="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>业务 / 来源</th>
                    <th>变动前可用</th>
                    <th>变动</th>
                    <th>变动后可用</th>
                    <th>操作人 / 请求 ID</th>
                    <th>时间</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="entry in ledger.filter(
                      (item) =>
                        !search ||
                        `${item.id} ${item.source} ${item.type}`
                          .toLowerCase()
                          .includes(search.toLowerCase()),
                    )"
                    :key="entry.id"
                  >
                    <td>
                      <b>{{ entry.type }}</b
                      ><small class="cell-sub"
                        >{{ entry.source }} ·
                        <span class="mono">{{ entry.id }}</span></small
                      >
                    </td>
                    <td class="amount">{{ entry.before }}</td>
                    <td
                      class="amount"
                      :class="entry.change.startsWith('+') ? 'positive' : ''"
                    >
                      {{ entry.change }}
                    </td>
                    <td class="amount">{{ entry.after }}</td>
                    <td>
                      {{ entry.actor
                      }}<small class="cell-sub mono"
                        >REQ-DEMO-{{ entry.id.slice(-4) }}</small
                      >
                    </td>
                    <td>{{ entry.time }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="panel-foot">
              显示积分为整数字符串格式；明细中的变动前/后含显示、可用、冻结及提现积分
              <span>只读演示</span>
            </div></template
          ><template v-else
            ><div class="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>充值单</th>
                    <th>成员</th>
                    <th>充值积分</th>
                    <th>凭证 / 备注</th>
                    <th>申请时间</th>
                    <th>状态</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="r in [
                      {
                        id: 'RC-2601043',
                        user: 'Mia W.',
                        amount: '2,000',
                        proof: '已上传凭证',
                        time: '10-05 09:12',
                        status: '待确认',
                      },
                      {
                        id: 'RC-2601041',
                        user: 'Lin Q.',
                        amount: '2,000',
                        proof: '银行转账 · 尾号 4821',
                        time: '10-04 18:41',
                        status: '待确认',
                      },
                    ]"
                    :key="r.id"
                  >
                    <td class="mono">{{ r.id }}</td>
                    <td>{{ r.user }}</td>
                    <td class="amount">{{ r.amount }} 分</td>
                    <td>{{ r.proof }}</td>
                    <td>{{ r.time }}</td>
                    <td>
                      <span class="badge badge-warn">{{ r.status }}</span>
                    </td>
                    <td>
                      <button
                        class="text-button"
                        @click="toast(`${r.id} 已确认并写入演示账本`)"
                      >
                        核实入账 →
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div></template
          >
        </article>
      </section>

      <section v-else-if="page === '佣金和奖励'" class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">AGENTS / COMMISSION & REWARDS</div>
            <h1>佣金和奖励</h1>
            <p>周期规则、试算与记录调整概览</p>
          </div>
          <button
            class="button button-primary"
            @click="toast('新建佣金规则草稿（演示）')"
          >
            ＋ 新建佣金规则
          </button>
        </div>
        <div class="money-cards commission-cards">
          <article class="panel money-card">
            <span>本周期预计佣金</span><b>18,420 <small>分</small></b
            ><small>周结算 · 10/01 – 10/07</small>
          </article>
          <article class="panel money-card">
            <span>待结算记录</span><b>1,284 <small>条</small></b
            ><small>覆盖 142 位代理</small>
          </article>
          <article class="panel money-card">
            <span>已结算佣金</span><b>86,230 <small>分</small></b
            ><small>本月累计 · 演示数据</small>
          </article>
        </div>
        <article class="panel">
          <div class="panel-header">
            <div>
              <h2>佣金规则版本</h2>
              <p>修改会创建新版本，已结算历史记录保留原始金额</p>
            </div>
            <button class="text-button" @click="toast('佣金版本历史（演示）')">
              版本历史 →
            </button>
          </div>
          <div class="rule-table-row">
            <span
              ><b>输赢佣金 · 星河代理组</b
              ><small>只计算有效输钱注单 · 品牌范围</small></span
            ><span>输赢模式</span><span>比例 <b>8.00%</b></span
            ><span>周期 <b>每周</b></span
            ><span><span class="badge badge-success">生效中 · v4</span></span
            ><button class="text-button" @click="toast('佣金规则详情（演示）')">
              详情 →
            </button>
          </div>
          <div class="rule-table-row">
            <span
              ><b>流水佣金 · 海风代理组</b
              ><small>基于有效投注流水 · 品牌范围</small></span
            ><span>流水模式</span><span>比例 <b>1.20%</b></span
            ><span>周期 <b>每月</b></span
            ><span><span class="badge badge-success">生效中 · v2</span></span
            ><button class="text-button" @click="toast('佣金规则详情（演示）')">
              详情 →
            </button>
          </div>
        </article>
        <article class="panel commission-records">
          <div class="panel-header">
            <div>
              <h2>近期佣金记录</h2>
              <p>人工修正会建立独立 adjustment 记录</p>
            </div>
            <button
              class="button button-secondary"
              @click="toast('记录报表导出完成（演示）')"
            >
              导出 ↓
            </button>
          </div>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>代理</th>
                  <th>周期</th>
                  <th>模式</th>
                  <th>计算基数</th>
                  <th>比例</th>
                  <th>佣金积分</th>
                  <th>状态</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="r in [
                    {
                      name: '周宁 · 星河-01',
                      period: '09/28 – 10/04',
                      mode: '输赢',
                      base: '84,200',
                      ratio: '8.00%',
                      points: '6,736',
                      status: '待结算',
                    },
                    {
                      name: '陈浩 · 海风-03',
                      period: '09/28 – 10/04',
                      mode: '流水',
                      base: '342,800',
                      ratio: '1.20%',
                      points: '4,113',
                      status: '待结算',
                    },
                    {
                      name: '林美 · 晨光-12',
                      period: '09/21 – 09/27',
                      mode: '输赢',
                      base: '48,200',
                      ratio: '6.00%',
                      points: '2,892',
                      status: '已结算',
                    },
                  ]"
                  :key="r.name"
                >
                  <td>{{ r.name }}</td>
                  <td>{{ r.period }}</td>
                  <td>{{ r.mode }}</td>
                  <td class="amount">{{ r.base }}</td>
                  <td>{{ r.ratio }}</td>
                  <td class="amount">{{ r.points }} 分</td>
                  <td>
                    <span
                      class="badge"
                      :class="
                        r.status === '已结算' ? 'badge-success' : 'badge-warn'
                      "
                      >{{ r.status }}</span
                    >
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </article>
      </section>

      <section v-else-if="page === '报表和对账'" class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">ANALYTICS / RECONCILIATION</div>
            <h1>报表和对账</h1>
            <p>运营指标与账本差异检查 · 当前为静态演示快照</p>
          </div>
          <div class="heading-actions">
            <select v-model="selectedRange" class="field">
              <option>今日</option>
              <option>近 7 天</option>
              <option>近 30 天</option></select
            ><button
              class="button button-secondary"
              @click="toast('报表文件导出完成（演示）')"
            >
              导出报表 ↓
            </button>
          </div>
        </div>
        <div class="metric-strip report-metrics">
          <div>
            <small>投注积分</small><b>892,460</b><span>↑ 12.4% 较上周期</span>
          </div>
          <div>
            <small>派彩积分</small><b>648,220</b><span>派彩率 72.6%</span>
          </div>
          <div>
            <small>充值积分</small><b>284,800</b><span>1,248 笔充值</span>
          </div>
          <div><small>提现积分</small><b>146,420</b><span>差异 0 分</span></div>
        </div>
        <div class="reports-grid">
          <article class="panel report-chart">
            <div class="panel-header">
              <div>
                <h2>投注与派彩趋势</h2>
                <p>按日聚合 · 单位：积分</p>
              </div>
              <div class="legend">
                <i></i>投注 <i class="legend-purple"></i>派彩
              </div>
            </div>
            <div class="chart report-bars">
              <div
                class="bar-group"
                v-for="(n, i) in [62, 76, 57, 86, 70, 94, 79]"
                :key="i"
              >
                <div class="bars">
                  <i :style="{ height: `${n}%` }"></i
                  ><b :style="{ height: `${n * 0.68}%` }"></b>
                </div>
                <small>{{
                  [
                    "09/29",
                    "09/30",
                    "10/01",
                    "10/02",
                    "10/03",
                    "10/04",
                    "10/05",
                  ][i]
                }}</small>
              </div>
            </div>
          </article>
          <article class="panel reconcile-card">
            <div class="panel-header">
              <div>
                <h2>账本对账</h2>
                <p>最近一次检查</p>
              </div>
              <span class="badge badge-success">一致</span>
            </div>
            <div class="reconcile-amount">0 <small>分差异</small></div>
            <div class="reconcile-line">
              <span>积分账户余额</span><b>8,428,640</b>
            </div>
            <div class="reconcile-line">
              <span>账本汇总余额</span><b>8,428,640</b>
            </div>
            <div class="reconcile-line">
              <span>检查时间</span><b>10-05 10:32:18</b>
            </div>
            <button
              class="button button-secondary full-button"
              @click="toast('对账任务完成：未发现差异（演示）')"
            >
              ↻ 重新运行对账
            </button>
          </article>
        </div>
      </section>

      <section v-else-if="page === '账号与权限'" class="page-content">
        <AccessManagement
          v-if="account && selectedBrandId"
          :key="selectedBrandId"
          :account="account"
          :brand-id="selectedBrandId"
          @session-invalid="clearAdminData"
        />
        <div v-else class="panel directory-state">
          <h1>账号与权限</h1>
          <p>
            {{
              account
                ? "请先选择真实后台品牌。"
                : "请先登录后台账号，才能管理真实角色和权限。"
            }}
          </p>
          <button
            v-if="!account"
            class="button button-primary"
            @click="go('用户和成员')"
          >
            进入管理员登录
          </button>
        </div>
      </section>

      <section v-else class="page-content">
        <div class="page-heading">
          <div>
            <div class="eyebrow">SECURITY / AUDIT TRAIL</div>
            <h1>审计日志</h1>
            <p>
              {{
                account
                  ? "按选中品牌读取真实后台日志。"
                  : "未登录：以下是静态演示样例，不是后台记录。"
              }}
            </p>
          </div>
          <button
            v-if="account"
            class="button button-secondary"
            :disabled="auditLoading || !selectedBrandId"
            @click="loadAudit"
          >
            刷新日志
          </button>
        </div>
        <div v-if="account && !selectedBrandId" class="panel directory-state">
          请先从侧栏选择品牌；审计请求始终携带明确的 X-Brand-ID。
        </div>
        <div
          v-else-if="account && auditError"
          class="directory-error"
          role="alert"
        >
          {{ auditError }}
        </div>
        <article v-if="account && selectedBrandId" class="panel">
          <div v-if="auditLoading" class="directory-state">
            正在读取审计日志…
          </div>
          <div v-else-if="!auditRecords.length" class="directory-state">
            所选品牌没有可显示的审计记录。
          </div>
          <div v-else class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>操作</th>
                  <th>资源</th>
                  <th>操作人</th>
                  <th>原因</th>
                  <th>Request ID</th>
                  <th>IP</th>
                  <th>时间</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="entry in auditRecords" :key="entry.id">
                  <td>{{ entry.action }}</td>
                  <td>{{ entry.resource_type }} · {{ entry.resource_id }}</td>
                  <td>{{ entry.actor_type }} · {{ entry.actor_id }}</td>
                  <td>{{ entry.reason }}</td>
                  <td>{{ entry.request_id }}</td>
                  <td>{{ entry.ip_address }}</td>
                  <td>{{ new Date(entry.created_at).toLocaleString() }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </article>
        <div v-if="!account" class="panel directory-state">
          演示日志不会显示为真实后台记录。
        </div>
      </section>

      <footer class="page-footer">
        <span>Aurora Operations Console <b>·</b> Prototype v0.1</span
        ><span>{{
          page === "用户和成员" && account
            ? "成员创建与管理为真实操作；资金与游戏仍为演示。"
            : page === "审计日志" && account
              ? "审计日志为真实后台数据；资金与游戏仍为演示。"
              : page === "账号与权限" && account
                ? "账号与角色变更为真实操作；资金与游戏仍为演示。"
                : page === "品牌和域名" && account
                  ? "认证设置为真实配置；域名、主题与资金仍为演示。"
                  : "此页业务数据为演示；账号、成员和认证设置已接入后台。"
        }}</span>
      </footer>
      <div
        v-if="editTarget"
        class="modal-backdrop"
        @click.self="editTarget = null"
      >
        <section
          class="modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="member-edit-title"
        >
          <div class="modal-heading">
            <div>
              <span class="eyebrow">REAL MEMBER · {{ selectedBrandId }}</span>
              <h2 id="member-edit-title">编辑成员状态和备注</h2>
            </div>
            <button
              class="modal-close"
              aria-label="关闭"
              @click="editTarget = null"
            >
              ×
            </button>
          </div>
          <div class="modal-summary">
            <div>
              <small>成员</small
              ><b>{{ editTarget.display_name || editTarget.username }}</b>
            </div>
            <div>
              <small>用户 ID</small><b>{{ editTarget.global_user_id }}</b>
            </div>
          </div>
          <label class="modal-label"
            >状态<select v-model="editStatus" class="field">
              <option
                v-for="(label, status) in statusLabel"
                :key="status"
                :value="status"
              >
                {{ label }}
              </option>
            </select></label
          ><label class="modal-label"
            >备注<textarea
              v-model="editNotes"
              rows="3"
              maxlength="2000"
            ></textarea></label
          ><label class="modal-label"
            >操作原因 <span>必填</span
            ><textarea v-model="editReason" rows="2" required></textarea>
          </label>
          <div class="modal-actions">
            <button class="button button-secondary" @click="editTarget = null">
              取消</button
            ><button
              class="button button-primary"
              :disabled="editBusy || !editReason.trim()"
              @click="saveMember"
            >
              {{ editBusy ? "保存中…" : "保存到后台" }}
            </button>
          </div>
        </section>
      </div>
      <div
        v-if="kickTarget"
        class="modal-backdrop"
        @click.self="kickTarget = null"
      >
        <section
          class="modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="member-kick-title"
        >
          <div class="modal-heading">
            <div>
              <span class="eyebrow">REAL MEMBER · {{ selectedBrandId }}</span>
              <h2 id="member-kick-title">踢出品牌会话</h2>
            </div>
            <button
              class="modal-close"
              aria-label="关闭"
              @click="kickTarget = null"
            >
              ×
            </button>
          </div>
          <p class="danger-note">
            这会调用后台踢出接口，撤销该用户在当前品牌的会话。
          </p>
          <label class="modal-label"
            >原因 <span>必填</span
            ><textarea v-model="kickReason" rows="3" required></textarea>
          </label>
          <div class="modal-actions">
            <button class="button button-secondary" @click="kickTarget = null">
              取消</button
            ><button
              class="button button-danger"
              :disabled="kickBusy || !kickReason.trim()"
              @click="kickMember"
            >
              {{ kickBusy ? "处理中…" : "确认踢出" }}
            </button>
          </div>
        </section>
      </div>
      <div
        v-if="resetTarget"
        class="modal-backdrop"
        @click.self="resetTarget = null"
      >
        <section
          class="modal"
          role="dialog"
          aria-modal="true"
          aria-labelledby="member-reset-title"
        >
          <div class="modal-heading">
            <div>
              <span class="eyebrow">GLOBAL PASSWORD · HIGH IMPACT</span>
              <h2 id="member-reset-title">重置全局密码</h2>
            </div>
            <button
              class="modal-close"
              aria-label="关闭"
              @click="resetTarget = null"
            >
              ×
            </button>
          </div>
          <div class="danger-note">
            此密码属于全局用户，将影响该用户在所有品牌的登录，并撤销其全部会话。后台要求你对该用户加入的每个品牌都有密码重置权限，否则拒绝操作。
          </div>
          <div class="modal-summary">
            <div>
              <small>成员</small
              ><b>{{ resetTarget.display_name || resetTarget.username }}</b>
            </div>
            <div>
              <small>全局用户 ID</small><b>{{ resetTarget.global_user_id }}</b>
            </div>
          </div>
          <label class="modal-label"
            >新密码 <span>必填</span
            ><input
              v-model="resetPasswordValue"
              class="field"
              type="password"
              autocomplete="new-password"
              required /></label
          ><label class="modal-label"
            >重置原因 <span>必填</span
            ><textarea
              v-model="resetReason"
              rows="2"
              required
            ></textarea></label
          ><label class="impact-confirm"
            ><input
              v-model="resetImpactConfirmed"
              type="checkbox"
            />我确认此重置影响该全局账号在所有品牌的密码和会话。</label
          >
          <div class="modal-actions">
            <button class="button button-secondary" @click="resetTarget = null">
              取消</button
            ><button
              class="button button-danger"
              :disabled="
                resetBusy ||
                !resetPasswordValue ||
                !resetReason.trim() ||
                !resetImpactConfirmed
              "
              @click="resetMemberPassword"
            >
              {{ resetBusy ? "处理中…" : "确认重置全局密码" }}
            </button>
          </div>
        </section>
      </div>
    </main>

    <div
      v-if="mobileMore"
      class="mobile-more-backdrop"
      @click.self="mobileMore = false"
    >
      <nav class="mobile-more-menu" aria-label="全部管理页面">
        <div>
          <b>全部管理页面</b
          ><button aria-label="关闭导航" @click="mobileMore = false">×</button>
        </div>
        <button
          v-for="item in nav.filter(
            (entry) =>
              !(
                ['工作台', '用户和成员', '期次和开奖', '资金与账本'] as Page[]
              ).includes(entry.name),
          )"
          :key="item.name"
          @click="go(item.name)"
        >
          <span>{{ item.icon }}</span
          >{{ item.name }}<i>›</i>
        </button>
      </nav>
    </div>
    <nav class="mobile-nav" aria-label="移动端主导航">
      <button
        v-for="target in [
          '工作台',
          '用户和成员',
          '期次和开奖',
          '资金与账本',
        ] as Page[]"
        :key="target"
        :class="{ active: page === target }"
        @click="go(target)"
      >
        <span>{{ nav.find((item) => item.name === target)?.icon }}</span
        ><small>{{
          target === "用户和成员"
            ? "用户"
            : target === "期次和开奖"
              ? "期次"
              : target === "资金与账本"
                ? "审核"
                : target
        }}</small></button
      ><button
        :class="{ active: mobileMore }"
        @click="mobileMore = !mobileMore"
      >
        <span>☰</span><small>更多</small>
      </button>
    </nav>

    <div
      v-if="reviewTarget"
      class="modal-backdrop"
      @click.self="reviewTarget = null"
    >
      <section
        class="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="withdraw-title"
      >
        <div class="modal-heading">
          <div>
            <span class="eyebrow">WITHDRAWAL REVIEW · DEMO</span>
            <h2 id="withdraw-title">审核提现申请</h2>
          </div>
          <button class="modal-close" @click="reviewTarget = null">×</button>
        </div>
        <div class="modal-summary">
          <div>
            <small>申请编号</small><b class="mono">{{ reviewTarget.id }}</b>
          </div>
          <div>
            <small>申请人</small><b>{{ reviewTarget.user }}</b>
          </div>
          <div>
            <small>申请积分</small><b>{{ reviewTarget.points }} 分</b>
          </div>
          <div>
            <small>积分来源</small><b>{{ reviewTarget.source }}</b>
          </div>
        </div>
        <label class="modal-label"
          >审核意见 / 原因 <span>必填</span
          ><textarea
            v-model="reviewReason"
            placeholder="说明来源核验结论或驳回原因"
            rows="3"
          ></textarea>
        </label>
        <p class="modal-hint">
          批准仅表示演示状态转为处理中；不会向外部付款或修改真实账本。
        </p>
        <div class="modal-actions">
          <button class="button button-secondary" @click="reviewTarget = null">
            取消</button
          ><button
            class="button button-danger"
            @click="finishWithdrawal('rejected')"
          >
            驳回申请</button
          ><button
            class="button button-primary"
            @click="finishWithdrawal('approved')"
          >
            通过审核
          </button>
        </div>
      </section>
    </div>
    <div
      v-if="correctionOpen"
      class="modal-backdrop"
      @click.self="correctionOpen = false"
    >
      <section
        class="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="correction-title"
      >
        <div class="modal-heading">
          <div>
            <span class="eyebrow">RESULT CORRECTION · DEMO</span>
            <h2 id="correction-title">纠正开奖结果</h2>
          </div>
          <button class="modal-close" @click="correctionOpen = false">×</button>
        </div>
        <div class="correction-values">
          <label
            >原结果<input
              class="field"
              v-model="periodResult"
              readonly /></label
          ><span>→</span
          ><label
            >新结果<input class="field" v-model="correctionResult"
          /></label>
        </div>
        <div class="impact-preview">
          <div class="impact-head">
            <b>影响范围预览</b><span>预估值 · 非实际回溯结果</span>
          </div>
          <div>
            <span>待回溯注单</span
            ><b>{{ correctionPreview.affectedOrders }} 笔</b>
          </div>
          <div>
            <span>预计重新结算</span
            ><b>{{ correctionPreview.estimatedSettlements }} 项</b>
          </div>
          <div><span>账本处理方式</span><b>保留原记录并创建冲正</b></div>
        </div>
        <label class="modal-label"
          >纠正原因 <span>必填</span
          ><textarea
            v-model="correctionReason"
            rows="3"
            placeholder="记录结果来源、校验依据及纠正原因"
          ></textarea>
        </label>
        <div class="modal-actions">
          <button
            class="button button-secondary"
            @click="correctionOpen = false"
          >
            取消</button
          ><button class="button button-danger" @click="correctResult">
            确认纠正并创建回溯（演示）
          </button>
        </div>
      </section>
    </div>
    <div
      v-if="manualOpen"
      class="modal-backdrop"
      @click.self="manualOpen = false"
    >
      <section
        class="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="manual-title"
      >
        <div class="modal-heading">
          <div>
            <span class="eyebrow">MANUAL DRAW · DEMO</span>
            <h2 id="manual-title">人工录入开奖结果</h2>
          </div>
          <button class="modal-close" @click="manualOpen = false">×</button>
        </div>
        <div class="form-grid">
          <label>期次<input class="field" value="20261005032" readonly /></label
          ><label
            >来源<select class="field">
              <option>人工录入（单人操作）</option>
            </select></label
          ><label class="span-two"
            >开奖结果<input class="field" v-model="correctionResult"
          /></label>
        </div>
        <div class="impact-preview">
          <div class="impact-head">
            <b>提交后影响</b><span>演示人工结果生效，本期停用外部来源</span>
          </div>
          <div><span>校验项</span><b>号码范围 · 重复 · 期次状态</b></div>
          <div><span>后续操作</span><b>记录操作者和原因，进入结算流程</b></div>
        </div>
        <label class="modal-label"
          >人工录入原因 <span>必填</span
          ><textarea
            v-model="manualReason"
            rows="3"
            placeholder="说明人工开奖原因和数据核验依据"
          ></textarea>
        </label>
        <div class="modal-actions">
          <button class="button button-secondary" @click="manualOpen = false">
            取消</button
          ><button class="button button-primary" @click="confirmManual">
            确认人工结果（演示）
          </button>
        </div>
      </section>
    </div>
  </div>
</template>
