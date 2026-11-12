<script setup lang="ts">
import { computed } from 'vue'
import SiteIcon from './SiteIcon'
import { themeCatalog, useTheme } from './theme'
const props = defineProps<{locale: 'en' | 'zh-CN'}>()
const {mode, themeIcon, cycleTheme} = useTheme()
const label = computed(() => themeCatalog[props.locale][mode.value])
</script>
<template><button type="button" class="icon-button theme-switcher" :aria-label="label" :title="label" @click="cycleTheme"><SiteIcon :name="themeIcon" /></button></template>
<style scoped>.theme-switcher { display: inline-flex; align-items: center; justify-content: center; width: var(--icon-control-size, 44px); height: var(--icon-control-size, 44px); flex-shrink: 0; border: 1px solid var(--line); border-radius: 10px; background: var(--surface); color: var(--ink); }</style>
