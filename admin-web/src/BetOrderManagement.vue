<script setup lang="ts">
import ConfiguredOrdersPanel from '@lottery/shared/ConfiguredOrdersPanel.vue'
import {useAdminI18n} from './i18n'
import type {AdminAccount} from './admin-api'
const {locale}=useAdminI18n()
defineProps<{account:AdminAccount;brandId:string}>()
const emit=defineEmits<{'session-invalid':[]}>()
function failed(cause:unknown){if((cause as {status?:number}).status===401)emit('session-invalid')}
</script>
<template><ConfiguredOrdersPanel surface="admin" :brand-id="brandId"  :locale="locale" @failure="failed" /></template>
