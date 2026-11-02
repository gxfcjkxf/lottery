<script setup lang="ts">
import OperationToastHost from '@lottery/shared/OperationToastHost.vue'
import { useAdminI18n } from './i18n'
const { locale } = useAdminI18n()
</script>
<template><OperationToastHost :locale="locale" /></template>
