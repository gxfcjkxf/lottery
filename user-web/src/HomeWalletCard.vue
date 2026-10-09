<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { RouterLink } from 'vue-router';
import { createWalletApi, formatIntegerAmount, WalletApiError, type Wallet } from './wallet-api';

const props = defineProps<{ brandCode?: string; locale: 'en' | 'zh' }>();
const emit = defineEmits<{ 'auth-expired': [] }>();
const wallet = ref<Wallet | null>(null);
const state = ref<'loading' | 'ready' | 'signed-out' | 'error'>('loading');
const error = ref('');
let generation = 0;
const copy = computed(() => props.locale === 'en' ? {
  title: 'Your points', loading: 'Loading your wallet…', signIn: 'Sign in', register: 'Create account',
  signedOut: 'Sign in to view your balance. No payment is connected.', refresh: 'Refresh',
  available: 'Available', frozen: 'Frozen', withdrawal: 'Pending withdrawal', wallet: 'View wallet',
  note: 'Balances come from your current brand wallet.', points: 'pts',
} : {
  title: '我的积分', loading: '正在读取钱包…', signIn: '登录', register: '创建账户',
  signedOut: '登录后查看余额。当前未接入支付。', refresh: '刷新',
  available: '可用积分', frozen: '冻结积分', withdrawal: '待提现积分', wallet: '查看钱包',
  note: '余额来自当前品牌的真实钱包。', points: '积分',
});

async function refresh() {
  const request = ++generation;
  wallet.value = null;
  state.value = 'loading';
  error.value = '';
  try {
    const result = await createWalletApi({ brandCode: props.brandCode }).wallet();
    if (request !== generation) return;
    wallet.value = result;
    state.value = 'ready';
  } catch (cause) {
    if (request !== generation) return;
    if (cause instanceof WalletApiError && cause.status === 401) { state.value = 'signed-out'; emit('auth-expired'); }
    else {
      state.value = 'error';
      error.value = cause instanceof Error ? cause.message : String(cause);
    }
  }
}
watch(() => props.brandCode, () => { void refresh(); }, { immediate: true, flush: 'sync' });
onBeforeUnmount(() => { generation++; });
</script>

<template>
  <div class="mini-panel balance-panel" data-testid="home-wallet-card">
    <div class="panel-icon">◈</div>
    <div class="eyebrow">{{ copy.title }}</div>
    <p v-if="state === 'loading'" role="status">{{ copy.loading }}</p>
    <template v-else-if="state === 'signed-out'">
      <p>{{ copy.signedOut }}</p>
      <RouterLink to="/login" class="text-link">{{ copy.signIn }} →</RouterLink>
      <RouterLink to="/register" class="text-link">{{ copy.register }} →</RouterLink>
    </template>
    <template v-else-if="wallet">
      <h3 data-testid="home-wallet-balance">{{ formatIntegerAmount(wallet.display_points) }} <small>{{ copy.points }}</small></h3>
      <dl class="home-wallet-fields">
        <div><dt>{{ copy.available }}</dt><dd>{{ formatIntegerAmount(wallet.available_points) }}</dd></div>
        <div><dt>{{ copy.frozen }}</dt><dd>{{ formatIntegerAmount(wallet.frozen_points) }}</dd></div>
        <div><dt>{{ copy.withdrawal }}</dt><dd>{{ formatIntegerAmount(wallet.withdrawal_points) }}</dd></div>
      </dl>
      <p>{{ copy.note }}</p>
      <RouterLink to="/wallet" class="text-link">{{ copy.wallet }} →</RouterLink>
    </template>
    <p v-else-if="state === 'error'" role="alert">{{ error }}</p>
    <button class="text-link home-wallet-refresh" :disabled="state === 'loading'" @click="refresh">{{ copy.refresh }}</button>
  </div>
</template>

<style scoped>
.home-wallet-fields { margin: 12px 0; font-size: 12px; }
.home-wallet-fields > div { display: flex; justify-content: space-between; gap: 12px; margin: 8px 0; }
.home-wallet-fields dd { margin: 0; overflow-wrap: anywhere; }
.home-wallet-refresh { display: block; border: 0; background: transparent; padding: 8px 0 0; cursor: pointer; }
.balance-panel h3 { overflow-wrap: anywhere; }
</style>
