<script setup lang="ts">
import { onUnmounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { createAuthClient } from "../../shared/src/auth";
import {
  createJoinCodesApi,
  type JoinCode,
  type JoinCodePage,
  type MemberAttribution,
} from "./join-codes-api";

const props = defineProps<{
  locale: "en" | "zh";
  brandName: string;
  brandCode?: string;
}>();
const loading = ref(false);
const error = ref("");
const notice = ref("");
const signedIn = ref(false);
const currentPage = ref<JoinCodePage | null>(null);
const codes = ref<JoinCode[]>([]);
const attribution = ref<MemberAttribution | null>(null);
let generation = 0;
let disposed = false;

const text = {
  en: {
    loading: "Loading your join codes…",
    signIn: "Sign in to view join codes for this membership.",
    retry: "Retry",
    attribution: "How you joined this brand",
    codes: "Your join codes",
    noAttribution: "No join code was used for this membership.",
    noCodes: "No join codes are assigned to this membership.",
    usable: "Usable",
    unavailable: "Unavailable",
    active: "Active",
    disabled: "Disabled",
    copyCode: "Copy code",
    share: "Copy invite text",
    copied: "Invite text copied.",
    copyFailed: "Could not copy. Select and copy the code manually.",
    previous: "Previous",
    next: "Next",
    page: "Page",
    agent: "Agent code",
    referral: "Referral code",
    domain: "Brand domain",
    operator: "Added by operations",
    starts: "Starts",
    expires: "Expires",
  },
  zh: {
    loading: "正在读取您的加入码…",
    signIn: "登录后可查看当前会员的加入码。",
    retry: "重试",
    attribution: "您加入此品牌的方式",
    codes: "您的加入码",
    noAttribution: "此会员没有使用加入码。",
    noCodes: "当前会员没有分配加入码。",
    usable: "可用",
    unavailable: "不可用",
    active: "启用",
    disabled: "停用",
    copyCode: "复制代码",
    share: "复制邀请文字",
    copied: "邀请文字已复制。",
    copyFailed: "复制失败，请手动选择并复制代码。",
    previous: "上一页",
    next: "下一页",
    page: "页",
    agent: "代理码",
    referral: "推荐码",
    domain: "品牌域名加入",
    operator: "运营添加",
    starts: "生效",
    expires: "到期",
  },
};
const t = () => text[props.locale];
const pageNumber = () =>
  currentPage.value ? Math.floor(currentPage.value.offset / currentPage.value.limit) + 1 : 1;
const hasPrevious = () => Boolean(currentPage.value?.offset);
const hasNext = () => Boolean(
  currentPage.value &&
    BigInt(currentPage.value.offset + currentPage.value.items.length) <
      BigInt(currentPage.value.total_count),
);

function methodLabel(method: MemberAttribution["join_method"]): string {
  return ({
    agent_code: t().agent,
    referral_code: t().referral,
    domain: t().domain,
    operator: t().operator,
  })[method];
}

function registerShareLink(): string | null {
  try {
    const url = new URL("/register", window.location.origin);
    if (!/^https?:$/.test(url.protocol) || url.origin !== window.location.origin) return null;
    return url.toString();
  } catch {
    return null;
  }
}

async function copyText(value: string): Promise<void> {
  const requestGeneration = generation;
  const requestBrandCode = props.brandCode;
  try {
    await navigator.clipboard.writeText(value);
    if (!disposed && requestGeneration === generation && requestBrandCode === props.brandCode) {
      notice.value = t().copied;
    }
  } catch {
    if (!disposed && requestGeneration === generation && requestBrandCode === props.brandCode) {
      notice.value = t().copyFailed;
    }
  }
}

async function copyInvite(code: JoinCode): Promise<void> {
  const link = registerShareLink();
  if (!code.usable || !link) return;
  const kind = code.kind === "agent" ? t().agent : t().referral;
  await copyText(`${props.brandName} · ${kind}: ${code.code}\n${link}`);
}

async function load(offset = 0): Promise<void> {
  const requestGeneration = ++generation;
  const requestBrandCode = props.brandCode;
  codes.value = [];
  currentPage.value = null;
  attribution.value = null;
  error.value = "";
  notice.value = "";
  signedIn.value = false;
  loading.value = true;
  try {
    const authApi = createAuthClient({ brandCode: requestBrandCode });
    const profile = await authApi.me();
    if (
      disposed ||
      requestGeneration !== generation ||
      requestBrandCode !== props.brandCode
    ) return;
    const context = {
      brand_id: profile.member.brand_id,
      member_id: profile.member.id,
    };
    signedIn.value = true;
    const joinApi = createJoinCodesApi({ brandCode: requestBrandCode });
    const [page, ownAttribution] = await Promise.all([
      joinApi.list(context, 20, offset),
      joinApi.attribution(context),
    ]);
    if (
      disposed ||
      requestGeneration !== generation ||
      requestBrandCode !== props.brandCode
    ) return;
    currentPage.value = page;
    codes.value = page.items;
    attribution.value = ownAttribution;
  } catch (cause) {
    if (
      disposed ||
      requestGeneration !== generation ||
      requestBrandCode !== props.brandCode
    ) return;
    if (cause instanceof Error && "status" in cause && cause.status === 401) {
      signedIn.value = false;
      error.value = t().signIn;
    } else {
      error.value = cause instanceof Error ? cause.message : "Request failed";
    }
  } finally {
    if (
      !disposed &&
      requestGeneration === generation &&
      requestBrandCode === props.brandCode
    ) loading.value = false;
  }
}

watch(
  () => props.brandCode,
  () => void load(),
  { immediate: true },
);
onUnmounted(() => {
  disposed = true;
  generation++;
});
</script>

<template>
  <section class="page-section narrow-page join-codes-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">{{ locale === "en" ? "MEMBERSHIP" : "会员" }}</div>
        <h1>{{ locale === "en" ? "Join codes" : "加入码" }}</h1>
        <p>{{ locale === "en" ? "Your codes and recorded join source for this brand." : "当前品牌的加入码及已记录来源。" }}</p>
      </div>
      <button class="button button-secondary" :disabled="loading" @click="load()">
        {{ loading ? t().loading : t().retry }}
      </button>
    </div>

    <p v-if="loading" class="detail-panel" role="status">{{ t().loading }}</p>
    <div v-else-if="error" class="detail-panel">
      <p :class="signedIn ? 'auth-error' : 'muted'" role="status">{{ error }}</p>
      <RouterLink v-if="!signedIn" to="/login" class="button button-primary">{{ locale === "en" ? "Sign in" : "登录" }} →</RouterLink>
    </div>
    <template v-else>
      <section class="detail-panel join-code-section">
        <h2>{{ t().attribution }}</h2>
        <template v-if="attribution">
          <p>{{ methodLabel(attribution.join_method) }}</p>
          <p v-if="attribution.source_code" class="join-source-code">{{ attribution.source_code }}</p>
          <p v-if="!attribution.code_id" class="muted">{{ t().noAttribution }}</p>
        </template>
      </section>

      <section class="detail-panel join-code-section">
        <h2>{{ t().codes }}</h2>
        <p v-if="codes.length === 0" class="muted">{{ t().noCodes }}</p>
        <article v-for="code in codes" :key="code.id" class="join-code-card">
          <div class="join-code-heading">
            <strong>{{ code.kind === "agent" ? t().agent : t().referral }}</strong>
            <span :class="code.usable ? 'join-code-usable' : 'muted'">{{ code.usable ? t().usable : t().unavailable }}</span>
          </div>
          <code>{{ code.code }}</code>
          <p class="muted">{{ code.status === "active" ? t().active : t().disabled }}<span v-if="code.starts_at"> · {{ t().starts }} {{ new Date(code.starts_at).toLocaleString() }}</span><span v-if="code.expires_at"> · {{ t().expires }} {{ new Date(code.expires_at).toLocaleString() }}</span></p>
          <div v-if="code.usable && registerShareLink()" class="join-code-actions">
            <button class="button button-secondary" @click="copyText(code.code)">{{ t().copyCode }}</button>
            <button class="button button-primary" @click="copyInvite(code)">{{ t().share }}</button>
          </div>
        </article>
        <div v-if="currentPage && (hasPrevious() || hasNext())" class="join-code-pagination">
          <button class="button button-secondary" :disabled="!hasPrevious()" @click="load(Math.max(0, currentPage.offset - currentPage.limit))">{{ t().previous }}</button>
          <span>{{ t().page }} {{ pageNumber() }}</span>
          <button class="button button-secondary" :disabled="!hasNext()" @click="load(currentPage.offset + currentPage.limit)">{{ t().next }}</button>
        </div>
      </section>
      <p v-if="notice" class="success-note" role="status">{{ notice }}</p>
    </template>
  </section>
</template>

<style scoped>
.join-codes-page { max-width: 760px; margin-inline: auto; }
.join-code-section + .join-code-section { margin-top: 18px; }
.join-code-section h2 { margin: 0 0 12px; font: 700 17px Manrope, sans-serif; }
.join-code-card { padding: 16px 0; border-top: 1px solid #e9ede8; }
.join-code-heading, .join-code-actions, .join-code-pagination { display: flex; align-items: center; gap: 10px; }
.join-code-heading { justify-content: space-between; }
.join-code-card code, .join-source-code { display: inline-block; margin-top: 12px; padding: 9px 12px; border-radius: 9px; background: #f1f4ef; letter-spacing: .08em; overflow-wrap: anywhere; }
.join-code-card p { margin: 10px 0; }
.join-code-usable { color: var(--brand-primary); font-weight: 700; }
.join-code-pagination { justify-content: center; margin-top: 12px; }
@media (max-width: 600px) {
  .join-codes-page { padding-inline: 16px; }
  .join-code-actions { align-items: stretch; flex-direction: column; }
  .join-code-actions .button { width: 100%; }
}
</style>
