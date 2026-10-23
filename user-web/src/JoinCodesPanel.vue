<script setup lang="ts">
import RegistrationLink from "@lottery/shared/RegistrationLink.vue"
import { registrationShareLink } from "@lottery/shared"
import { useUserI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import { vGlobalLoading } from "@lottery/shared";
import { computed, onUnmounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { createAuthClient } from "../../shared/src/auth";
import { useUserI18n } from './i18n';
import {
  createJoinCodesApi,
  type JoinCode,
  type JoinCodePage,
} from "./join-codes-api";

const props = defineProps<{
  locale: "en" | "zh";
  brandName: string;
  brandCode?: string;
}>();
const localeCode=computed(()=>props.locale==='en'?'en':'zh-CN');
const loading = ref(false);
const error = ref<string | import("@lottery/shared").LocalizedMessage>("");
const signedIn = ref(false);
const currentPage = ref<JoinCodePage | null>(null);
const codes = ref<JoinCode[]>([]);
let generation = 0;
let disposed = false;

const ui = useUserI18n().t;
const t = () => ({
  title: ui('user.join.title'), loading: ui('user.join.loading'), signIn: ui('user.join.signIn'),
  signInAction: ui('user.join.signInAction'), retry: ui('user.join.retry'), noCodes: ui('user.join.noCodes'),
  unavailable: ui('user.join.unavailable'), copyLink: ui('user.join.copyLink'), share: ui('user.join.share'),
  copied: ui('user.join.copied'), copyFailed: ui('user.join.copyFailed'), previous: ui('user.join.previous'),
  next: ui('user.join.next'), page: ui('user.join.page'), referral: ui('user.join.referral'),
});
const pageNumber = () =>
  currentPage.value ? Math.floor(currentPage.value.offset / currentPage.value.limit) + 1 : 1;
const hasPrevious = () => Boolean(currentPage.value?.offset);
const hasNext = () => Boolean(
  currentPage.value &&
    BigInt(currentPage.value.offset + currentPage.value.items.length) <
      BigInt(currentPage.value.total_count),
);

function registerShareLink(code:JoinCode):string {return registrationShareLink(window.location.origin,'referral',code.code)}

async function load(offset = 0): Promise<void> {
  const requestGeneration = ++generation;
  const requestBrandCode = props.brandCode;
  codes.value = [];
  currentPage.value = null;
  error.value = "";
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
    const page = await joinApi.list(context, 20, offset);
    if (
      disposed ||
      requestGeneration !== generation ||
      requestBrandCode !== props.brandCode
    ) return;
    currentPage.value = page;
    codes.value = page.items.filter(code => code.kind === 'referral');
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
      error.value = cause instanceof Error ? publicErrorMessage(cause) : ui('user.common.requestFailed');
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
    <div class="page-heading"><h1>{{ t().title }}</h1></div>
    <p v-global-loading v-if="loading" class="detail-panel" role="status">{{ t().loading }}</p>
    <div v-else-if="error" class="detail-panel">
      <p :class="signedIn ? 'auth-error' : 'muted'" role="alert">{{ displayPublicError(error) }}</p>
      <RouterLink v-if="!signedIn" to="/login" class="button button-primary">{{ t().signInAction }}</RouterLink>
      <button v-else type="button" class="button button-secondary" @click="withGlobalLoading(load)">{{ t().retry }}</button>
    </div>
    <template v-else>
      <section class="detail-panel join-code-section">
        <p v-if="codes.length === 0" class="muted empty-state">{{ t().noCodes }}</p>
        <RegistrationLink :share-title="brandName" :locale="localeCode" v-for="code in codes" :key="code.id" class="join-code-card" :url="registerShareLink(code)" :disabled="!code.usable" :unavailable-label="t().unavailable" :copy-label="t().copyLink" :copied-label="t().copied" :failed-label="t().copyFailed" />
        <div v-if="currentPage && (hasPrevious() || hasNext())" class="join-code-pagination">
          <button type="button" class="button button-secondary" :disabled="!hasPrevious()" @click="withGlobalLoading(load, Math.max(0, currentPage.offset - currentPage.limit))">{{ t().previous }}</button>
          <span>{{ t().page }} {{ pageNumber() }}</span>
          <button type="button" class="button button-secondary" :disabled="!hasNext()" @click="withGlobalLoading(load, currentPage.offset + currentPage.limit)">{{ t().next }}</button>
        </div>
      </section>
    </template>
  </section>
</template>

<style scoped>
.join-codes-page { max-width: 580px; margin-inline: auto; }
.join-code-card + .join-code-card { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--line); }
.join-code-pagination { display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: 10px; margin-top: 16px; }
@media (max-width: 620px) {
  .join-codes-page > .page-heading { display: none; }
  .join-code-section { padding: 16px; }
}
</style>
