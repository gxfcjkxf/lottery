<script setup lang="ts">
import {withGlobalLoading} from "@lottery/shared";
import { computed } from "vue";
import { RouterLink, useRouter } from "vue-router";
import CashWallet from "../../shared/src/CashWallet.vue";
import { useUserI18n } from "./i18n";
import { createWalletApi } from "./wallet-api";
const props = defineProps<{brandCode?: string; member: {id:string; brand_id:string}|null; mode:"overview"|"deposit"|"withdrawal"}>();
const emit = defineEmits<{"auth-expired": []}>();
const {t,locale,errorText}=useUserI18n();
const router=useRouter();
const apiBase=computed(()=>props.brandCode ? `/api/v1/b/${encodeURIComponent(props.brandCode)}` : "/api/v1");
async function refreshPoints(){return (await createWalletApi({brandCode:props.brandCode}).wallet()).available_points}
</script>
<template>
 <CashWallet :api-base="apiBase" :read-available-points="refreshPoints" :member="member" :mode="mode" :t="t" :locale="locale" :error-text="errorText" @auth-expired="emit('auth-expired')" @deposit="router.push('/recharge')" @withdrawal="router.push('/withdraw')" @exchanged="withGlobalLoading(refreshPoints)" @manage-accounts="currency => router.push({ path: '/account/payment-accounts', query: { cashCurrency: currency, cashReturn: 'withdraw' } })" >
   <template v-if="mode === 'overview'" #actions-start>
     <RouterLink to="/wallet/buckets" class="button button-secondary">{{ t('user.wallet.buckets') }}</RouterLink>
     <RouterLink to="/wallet/ledger" class="button button-secondary">{{ t('user.wallet.ledger') }}</RouterLink>
   </template>
 </CashWallet>
</template>
