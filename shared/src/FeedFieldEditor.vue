<script setup lang="ts">
import {feedMappingCatalog,type FeedField,type FeedMapping} from './feed-mapping'
const props=defineProps<{field:FeedField;title:string;mapping:FeedMapping;locale:'en'|'zh-CN'}>()
const t=(key:keyof typeof feedMappingCatalog.en)=>feedMappingCatalog[props.locale][key]
function sourceChanged(e:Event){props.field.response=(e.target as HTMLSelectElement).value;props.field.attribute='';props.field.path=''}
</script>
<template><div class="feed-field-inputs"><label v-if="mapping.format==='browser'">{{title}} · {{t('valueSource')}}<select :value="field.response??''" @change="sourceChanged"><option value="">{{t('documentDOM')}}</option><option v-for="q in mapping.browser?.json_requests" :key="q.id" :value="q.id">{{q.id}}</option></select></label><label>{{title}} · {{t('path')}}<input v-model="field.path" required maxlength="512" /></label><label v-if="mapping.format!=='json'&&!field.response">{{title}} · {{t('attribute')}}<input v-model="field.attribute" maxlength="120" /></label><label>{{title}} · {{t('capture')}}<input v-model="field.capture" maxlength="512" /></label></div></template>
<style scoped>.feed-field-inputs{display:contents}</style>
