<script setup lang="ts">
import { computed,watch } from 'vue'
import { createCatalogI18n } from './localization'
import { markSixCatalog,markSixWave,type MarkSixAttributes } from './mark-six'
const props=defineProps<{attributes:Pick<MarkSixAttributes,'waves'|'zodiac'|'elements'>;value:number;locale:'en'|'zh-CN';special?:boolean;small?:boolean}>()
const i18n=createCatalogI18n(markSixCatalog,props.locale);watch(()=>props.locale,v=>i18n.setLocale(v));const t=i18n.t
const wave=computed(()=>markSixWave(props.value,props.attributes.waves)),label=computed(()=>`${String(props.value).padStart(2,'0')} · ${t(wave.value)} · ${t(props.attributes.zodiac[String(props.value).padStart(2,'0')])} · ${t(props.attributes.elements[String(props.value).padStart(2,'0')])}`)
</script>
<template><span class="mark-six-ball" :class="[wave,{special,small}]" :title="label" :aria-label="label">{{ String(value).padStart(2,'0') }}</span></template>
<style src="./mark-six-balls.css"></style>
