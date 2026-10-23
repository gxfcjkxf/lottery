<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { formatPoints } from '@lottery/shared';
import { useUserI18n } from './i18n';
import { createWalletApi, WALLET_SNAPSHOT_EVENT, type Wallet } from './wallet-api';

const props = defineProps<{ brandCode?: string; brandId: string; memberId: string; routeKey: string }>();
const emit = defineEmits<{
  authExpired: [];
  pointsChange: [state: { brandId: string; memberId: string; amount: string; description: string } | null];
}>();
const { t, locale } = useUserI18n();
const wallet = ref<Wallet | null>(null);
const failed = ref(false);
let generation = 0;
let disposed = false;
let timer: ReturnType<typeof setInterval>;
const amount = computed(() => wallet.value ? formatPoints(wallet.value.available_points, locale.value) : failed.value ? '—' : '…');
const description = computed(() => failed.value ? t('user.wallet.networkError') : wallet.value
  ? t('user.header.pointsLabel', { points: amount.value }) : t('user.wallet.reading'));
watch([amount, description, () => props.brandId, () => props.memberId], () => {
  emit('pointsChange', { brandId: props.brandId, memberId: props.memberId, amount: amount.value, description: description.value });
}, { immediate: true });

function accept(value: Wallet) {
  if (value.brand_id !== props.brandId || value.member_id !== props.memberId) return false;
  if (wallet.value && value.version < wallet.value.version) return true;
  generation++;
  wallet.value = value;
  failed.value = false;
  return true;
}
function snapshot(event: Event) { accept((event as CustomEvent<Wallet>).detail); }
async function refresh() {
  const request = ++generation;
  try {
    const value = await createWalletApi({ brandCode: props.brandCode }).wallet();
    if (disposed || request !== generation) return;
    if (!accept(value)) throw new Error('Wallet identity mismatch');
  } catch (cause) {
    if (disposed || request !== generation) return;
    wallet.value = null;
    failed.value = true;
    if (cause && typeof cause === 'object' && 'status' in cause && cause.status === 401) emit('authExpired');
  }
}
watch(() => [props.brandCode, props.brandId, props.memberId], () => {
  generation++; wallet.value = null; failed.value = false;
  void refresh();
}, { immediate: true });
watch(() => props.routeKey, () => void refresh());
onMounted(() => {
  window.addEventListener(WALLET_SNAPSHOT_EVENT, snapshot);
  window.addEventListener('focus', refresh);
  timer = setInterval(refresh, 30000);
});
onUnmounted(() => {
  emit('pointsChange', null);
  disposed = true; generation++;
  window.removeEventListener(WALLET_SNAPSHOT_EVENT, snapshot);
  window.removeEventListener('focus', refresh);
  clearInterval(timer);
});
</script>

<template>
  <div class="header-points" role="status" :aria-label="description" :title="description">
    <strong>{{ amount }}</strong>
  </div>
</template>
