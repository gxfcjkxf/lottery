<script setup lang="ts">
import SelectControl from './SelectControl.vue'
import { onBeforeUnmount, ref, watch } from 'vue'
import { readFilterOptions } from './filter-options'
import { withGlobalLoading } from './global-loading'
import { createCatalogI18n } from './localization'
import SiteIcon from './SiteIcon'
const props=defineProps<{ modelValue:string; brandId:string; surface:'admin'|'platform'; resource:string; kind:string; label:string; locale:'en'|'zh-CN'; disabled?:boolean; enabled?:boolean; scopeKey?:string }>()
const emit=defineEmits<{ 'update:modelValue':[value:string] }>()
const i18n=createCatalogI18n({en:{all:'All',more:'More options',retry:'Retry',failed:'Unable to load options. Please retry.'},'zh-CN':{all:'全部',more:'更多选项',retry:'重试',failed:'无法加载选项，请重试。'}},props.locale)
watch(()=>props.locale,v=>i18n.setLocale(v));const t=i18n.t
const items=ref<{id:string;name:string}[]>([]),busy=ref(false),failed=ref(false),more=ref(false)
let generation=0, controller: AbortController | null = null
async function load(offset=0){
 controller?.abort();controller=new AbortController();const signal=controller.signal
 const ticket=++generation;busy.value=true;failed.value=false
 try{
  const data=await readFilterOptions({surface:props.surface,brandId:props.brandId,resource:props.resource,kind:props.kind,signal},offset)
  if(ticket!==generation)return
  items.value=offset?[...items.value,...data.items]:data.items;more.value=data.items.length===100
 }catch{if(ticket===generation)failed.value=true}finally{if(ticket===generation)busy.value=false}
}
watch(()=>[props.brandId,props.surface,props.resource,props.kind,props.scopeKey,props.enabled],(_scope,previous)=>{
 generation++;controller?.abort();items.value=[];more.value=false;failed.value=false;busy.value=false;if(previous)emit('update:modelValue','')
 if(props.brandId && props.enabled!==false)void withGlobalLoading(load,0)
},{immediate:true})
onBeforeUnmount(()=>{generation++;controller?.abort()})
</script>
<template>
 <span class="association-filter">
  <SelectControl :model-value="modelValue" :label="label" :placeholder="label" :disabled="disabled||busy||failed||!brandId||enabled===false" @change="emit('update:modelValue',$event)" :options="[{ value: '', label: (String(t('all'))).trim() }, ...((items).map((item) => ({ value: item.id, label: (String(item.name)).trim() })))]" />
  <button v-if="more&&!failed" type="button" class="icon-button" :disabled="busy" :aria-label="t('more')" @click="withGlobalLoading(load,items.length)"><SiteIcon name="more" /></button>
  <span v-if="failed" class="association-filter-error" role="alert">{{ t('failed') }} <button type="button" @click="withGlobalLoading(load,0)">{{ t('retry') }}</button></span>
 </span>
</template>
<style scoped>
.association-filter{display:inline-flex;align-items:center;gap:4px;min-width:0;max-width:100%}.association-filter-error{color:var(--danger);font-size:var(--body-font-size);max-width:240px}.association-filter button{flex-shrink:0}
</style>
