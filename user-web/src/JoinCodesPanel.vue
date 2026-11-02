<script setup lang="ts">
import TablePagination from "@lottery/shared/TablePagination.vue"
import RegistrationLink from "@lottery/shared/RegistrationLink.vue"
import { formatDisplayDateTime, registrationShareLink } from "@lottery/shared"
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
  type ReferralPage,
  type ReferredMember,
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
const activeTab = ref<'list' | 'codes'>('list');
const referrals = ref<ReferredMember[]>([]);
const currentPage = ref<JoinCodePage | ReferralPage | null>(null);
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

function moveTab(event: KeyboardEvent): void {
  activeTab.value = activeTab.value === 'list' ? 'codes' : 'list';
  (event.currentTarget as HTMLElement).parentElement?.querySelector<HTMLButtonElement>(`#referral-tab-${activeTab.value}`)?.focus();
}

function registerShareLink(code:JoinCode):string {return registrationShareLink(window.location.origin,'referral',code.code)}

async function load(offset = 0): Promise<void> {
  const requestGeneration = ++generation;
  const requestBrandCode = props.brandCode;
  const requestTab = activeTab.value;
  error.value = "";
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
    const page = requestTab === 'list' ? await joinApi.referrals(context, 20, offset) : await joinApi.list(context, 20, offset);
    if (
      disposed ||
      requestGeneration !== generation ||
      requestBrandCode !== props.brandCode
    ) return;
    currentPage.value = page;
    if (requestTab === 'list') referrals.value = (page as ReferralPage).items;
    else codes.value = (page as JoinCodePage).items.filter(code => code.kind === 'referral');
  } catch (cause) {
    if (
      disposed ||
      requestGeneration !== generation ||
      requestBrandCode !== props.brandCode
    ) return;
    if (cause instanceof Error && "status" in cause && cause.status === 401) {
      signedIn.value = false; currentPage.value = null; referrals.value = []; codes.value = [];
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
  () => [props.brandCode, activeTab.value],
  () => { codes.value = []; referrals.value = []; currentPage.value = null; signedIn.value = false; void load(); },
  { immediate: true },
);
onUnmounted(() => {
  disposed = true;
  generation++;
});
</script>

<template>
  <section class="page-section join-codes-page user-record-list">

    <div class="join-tabs" role="tablist" :aria-label="t().title">
      <button v-for="tab in (['list', 'codes'] as const)" :id="`referral-tab-${tab}`" :key="tab" type="button" role="tab" :aria-selected="activeTab === tab" :aria-controls="`referral-panel-${tab}`" :tabindex="activeTab === tab ? 0 : -1" :class="['button', activeTab === tab ? 'button-primary' : 'button-secondary']" @click="activeTab = tab" @keydown.right.prevent="moveTab" @keydown.left.prevent="moveTab">{{ ui(tab === 'list' ? 'user.join.listTab' : 'user.join.codeTab') }}</button>
    </div>
    <div :id="`referral-panel-${activeTab}`" role="tabpanel" :aria-labelledby="`referral-tab-${activeTab}`">
    <p v-global-loading v-if="loading" class="detail-panel" role="status">{{ ui('user.join.loadingContent') }}</p>
    <div v-if="error" class="detail-panel">
      <p :class="signedIn ? 'auth-error' : 'muted'" role="alert">{{ displayPublicError(error) }}</p>
      <RouterLink v-if="!signedIn" to="/login" class="button button-primary">{{ t().signInAction }}</RouterLink>
      <button v-else type="button" class="button button-secondary" @click="withGlobalLoading(load)">{{ t().retry }}</button>
    </div>
    <template v-if="signedIn && currentPage">
      <section class="join-code-section">
        <template v-if="activeTab === 'list'">
          <p v-if="referrals.length === 0" class="muted empty-state">{{ ui('user.join.noReferrals') }}</p>
          <ul v-else class="referral-list">
            <li v-for="member in referrals" :key="member.id">
              <strong>{{ member.display_name || ui('user.join.unnamedMember') }}</strong>
              <span class="muted">{{ ui('user.join.joinedAt') }} <time :datetime="member.joined_at">{{ formatDisplayDateTime(member.joined_at, locale) }}</time></span>
            </li>
          </ul>
        </template>
        <template v-else>
        <p v-if="codes.length === 0" class="muted empty-state">{{ t().noCodes }}</p>
        <RegistrationLink :share-title="brandName" :locale="localeCode" v-for="code in codes" :key="code.id" class="join-code-card" :url="registerShareLink(code)" :disabled="!code.usable" :unavailable-label="t().unavailable" :copy-label="t().copyLink" :copied-label="t().copied" :failed-label="t().copyFailed" />
        </template>
        <TablePagination show-empty v-if="currentPage" class="join-code-pagination" :total="currentPage?.total_count ?? '0'" :page="Math.floor(pageNumber())" :count="activeTab === 'list' ? referrals.length : codes.length" :locale="localeCode" :page-size="currentPage.limit" :previous-disabled="loading || !hasPrevious()" :next-disabled="loading || !hasNext()" :previous-label="t().previous" :next-label="t().next" :go-to="(target: number) => load((target - 1) * currentPage!.limit)" />
      </section>
    </template>
    </div>
  </section>
</template>

<style scoped>
.join-codes-page { width: 100%; min-width: 0; }
.join-tabs { display: flex; gap: 8px; margin-bottom: 12px; }
.join-tabs button { flex: 1; min-width: 0; }
.referral-list { list-style: none; padding: 0; margin: 0; }
.referral-list li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 4px 12px; padding-block: 10px; overflow-wrap: anywhere; }
.referral-list li + li { border-top: 1px solid var(--line); }
.referral-list strong { min-width: 0; }
.referral-list .muted { font-size: var(--body-font-size); }
.join-code-card + .join-code-card { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--line); }
@media (max-width: 620px) {
  .join-codes-page > .page-heading { display: none; }
  .join-code-section { padding: 16px; }
}
</style>
