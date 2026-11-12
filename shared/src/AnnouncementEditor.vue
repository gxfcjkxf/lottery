<script setup lang="ts">
import {withGlobalLoading} from './global-loading'
import { ref, nextTick } from 'vue'
import { createCatalogI18n } from './localization'
import { announcementsCatalog } from './announcements-catalog'
import { watch } from 'vue'
const props=defineProps<{modelValue:string;locale:'en'|'zh-CN';disabled:boolean;upload:(file:File)=>Promise<string>}>()
const emit=defineEmits<{'update:modelValue':[value:string];failure:[error:unknown]}>()
const i18n=createCatalogI18n(announcementsCatalog,props.locale),t=i18n.t
watch(()=>props.locale,v=>i18n.setLocale(v))
const input=ref<HTMLTextAreaElement|null>(null),fileInput=ref<HTMLInputElement|null>(null),uploading=ref(false),linkOpen=ref(false),url=ref('')
const escape=(s:string)=>s.replaceAll('&','&amp;').replaceAll('"','&quot;').replaceAll('<','&lt;').replaceAll('>','&gt;')
async function insert(before:string,after=''){const field=input.value!;const start=field.selectionStart,end=field.selectionEnd;emit('update:modelValue',props.modelValue.slice(0,start)+before+props.modelValue.slice(start,end)+after+props.modelValue.slice(end));await nextTick();field.focus();field.setSelectionRange(start+before.length,end+before.length)}
async function uploadFile(event:Event){const target=event.target as HTMLInputElement,file=target.files?.[0];if(!file)return;uploading.value=true;try{const asset=await props.upload(file);await insert(`<img src="${asset}" alt="">`)}catch(e){emit('failure',e)}finally{uploading.value=false;target.value=''}}
</script>
<template>
 <div class="announcement-editor">
  <div class="editor-tools" role="toolbar" :aria-label="t('body')">
   <button v-for="tool in ([['bold','strong'],['italic','em'],['subtitle','h2'],['emphasis','mark'],['list','ul']] as const)" :key="tool[0]" type="button" class="button button-secondary" :disabled="disabled || uploading" @click="insert(tool[1]==='ul'?'<ul><li>':`<${tool[1]}>`,tool[1]==='ul'?'</li></ul>':`</${tool[1]}>`)">{{t(tool[0])}}</button>
   <button type="button" class="button button-secondary" :disabled="disabled || uploading" @click="linkOpen=!linkOpen">{{t('link')}}</button>
   <button type="button" class="button button-secondary" :disabled="disabled || uploading" @click="fileInput?.click()">{{t(uploading?'uploading':'image')}}</button>
   <input ref="fileInput" type="file" accept="image/png,image/jpeg,image/webp" hidden :disabled="disabled || uploading" @change="event=>withGlobalLoading(uploadFile,event)">
  </div>
  <div v-if="linkOpen" class="editor-link"><input v-model="url" type="url" :aria-label="t('url')" :placeholder="t('url')" :disabled="disabled"><button type="button" class="button button-secondary" :disabled="disabled || !url" @click="insert(`<a href=&quot;${escape(url)}&quot;>`, '</a>');linkOpen=false;url=''">{{t('link')}}</button></div>
  <textarea ref="input" :value="modelValue" :aria-label="t('body')" rows="6" maxlength="60000" :disabled="disabled || uploading" spellcheck="false" @input="emit('update:modelValue',($event.target as HTMLTextAreaElement).value)" />
 </div>
</template>
<style scoped>
.announcement-editor{display:grid;gap:6px;min-width:0}.editor-tools,.editor-link{display:flex;gap:6px;flex-wrap:wrap}.announcement-editor textarea{box-sizing:border-box;width:100%;min-width:0;resize:vertical;font-family:var(--font-family);font-size:var(--body-font-size)}.editor-link input{min-width:0;flex:1}.editor-tools button{font-size:var(--body-font-size)}
</style>
