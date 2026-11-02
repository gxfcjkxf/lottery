<script setup lang="ts">
import OperationToastHost from '@lottery/shared/OperationToastHost.vue'
import { useUserI18n } from './i18n'
const { locale } = useUserI18n()
</script>
<template><RouterView /><OperationToastHost :locale="locale" /></template>
