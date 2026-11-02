<script setup lang="ts">
import ConfiguredOrdersPanel from '@lottery/shared/ConfiguredOrdersPanel.vue'
import type {PlatformDisplaySettings} from './platform-format'
const props=defineProps<{brandId:string;gameId:string;locale:'en'|'zh-CN';displaySettings:PlatformDisplaySettings}>()
const emit=defineEmits<{failure:[cause:unknown]}>()
function failed(cause:unknown){emit('failure',cause)}
</script>
<template><ConfiguredOrdersPanel surface="platform" :brand-id="brandId" :game-id="gameId" :locale="props.locale" @failure="failed" /></template>
