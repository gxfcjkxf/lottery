<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { readApiJson } from './association-names'
import { withGlobalLoading } from './global-loading'
import { createCatalogI18n } from './localization'
import SiteIcon from './SiteIcon'
import { notifySessionReplaced } from './session-replacement'
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
  const params=new URLSearchParams({resource:props.resource,kind:props.kind,limit:'100',offset:String(offset)})
  const response=await fetch(`/api/v1/${props.surface}/filter-options?${params}`,{signal,credentials:'same-origin',cache:'no-store',headers:{Accept:'application/json','X-Brand-ID':props.brandId}})
  const result=await readApiJson(response),data=result?.data
  if(!response.ok)notifySessionReplaced(result?.error?.code)
  if(!response.ok||result.success!==true||data?.brand_id!==props.brandId||data.limit!==100||data.offset!==offset||!Array.isArray(data.items)||data.items.some((v:any)=>typeof v.id!=='string'||!v.id||typeof v.name!=='string'||!v.name))throw Error('Invalid filter options')
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
  <select :value="modelValue" :aria-label="label" :disabled="disabled||busy||failed||!brandId||enabled===false" @change="emit('update:modelValue',($event.target as HTMLSelectElement).value)"><option value="">{{ t('all') }}</option><option v-for="item in items" :key="item.id" :value="item.id">{{ item.name }}</option></select>
  <button v-if="more&&!failed" type="button" class="icon-button" :disabled="busy" :aria-label="t('more')" @click="withGlobalLoading(load,items.length)"><SiteIcon name="more" /></button>
  <span v-if="failed" class="association-filter-error" role="alert">{{ t('failed') }} <button type="button" @click="withGlobalLoading(load,0)">{{ t('retry') }}</button></span>
 </span>
</template>
<style scoped>
.association-filter{display:inline-flex;align-items:center;gap:4px;min-width:0;max-width:100%}.association-filter select{max-width:100%}.association-filter-error{color:var(--danger);font-size:12px;max-width:240px}.association-filter button{flex-shrink:0}
</style>
