<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import * as QRCode from 'qrcode'
import SiteIcon from './SiteIcon'
import { createCatalogI18n } from './localization'
import { registrationLinkCatalog } from './registration-link-catalog'
const props=defineProps<{url:string;shareTitle:string;locale:'en'|'zh-CN';copyLabel:string;copiedLabel:string;failedLabel:string;disabled:boolean;unavailableLabel?:string;label?:string}>()
const i18n=createCatalogI18n(registrationLinkCatalog,props.locale),t=i18n.t
watch(()=>props.locale,value=>i18n.setLocale(value))
const status=ref<''|'copied'|'failed'|'shareUnsupported'|'shareFailed'>(''),copying=ref(false),qr=ref(''),qrFailed=ref(false),sharing=ref(false)
let generation=0
watch(()=>[props.url,props.disabled],async()=>{
 const ticket=++generation;status.value='';copying.value=false;sharing.value=false;qr.value='';qrFailed.value=false
 if(props.disabled)return
 try{const value=await QRCode.toDataURL(props.url,{width:768,margin:4,errorCorrectionLevel:'M'});if(ticket===generation)qr.value=value}
 catch{if(ticket===generation)qrFailed.value=true}
},{immediate:true})
onBeforeUnmount(()=>generation++)
async function copy(){
 if(props.disabled||copying.value)return
 const ticket=generation;copying.value=true;status.value=''
 try{await navigator.clipboard.writeText(props.url);if(ticket===generation)status.value='copied'}
 catch{if(ticket===generation)status.value='failed'}
 finally{if(ticket===generation)copying.value=false}
}
async function share(){
 if(props.disabled||sharing.value)return
 status.value=''
 if(typeof navigator.share!=='function'){status.value='shareUnsupported';return}
 const ticket=generation;sharing.value=true
 try{await navigator.share({title:props.shareTitle,url:props.url})}
 catch(cause){if(ticket===generation&&!(cause instanceof Error&&cause.name==='AbortError'))status.value='shareFailed'}
 finally{if(ticket===generation)sharing.value=false}
}
</script>
<template>
 <article class="registration-link">
  <strong v-if="label">{{label}}</strong>
  <div class="registration-link-address"><code>{{url}}</code><button type="button" class="button button-secondary registration-link-copy" :disabled="disabled||copying" :aria-label="copyLabel" :title="disabled?unavailableLabel:copyLabel" @click="copy"><SiteIcon name="copy" /></button></div>
  <img v-if="qr" class="registration-qr" :src="qr" :alt="t('qrAlt')" width="240" height="240" />
  <p v-if="qrFailed" class="form-error" role="alert">{{t('qrFailed')}}</p>
  <div class="registration-link-footer">
   <div class="registration-link-feedback"><p v-if="qr" class="registration-qr-hint">{{t('qrHint')}}</p><p v-if="status" :class="status==='copied'?'success-note':'form-error'" :role="status==='copied'?'status':'alert'">{{status==='copied'?copiedLabel:status==='failed'?failedLabel:t(status)}}</p></div>
   <div class="registration-link-actions"><button type="button" class="button button-primary registration-link-share" :disabled="disabled||sharing" :title="disabled?unavailableLabel:undefined" @click="share">{{t('share')}}</button><a v-if="qr" class="button button-secondary registration-qr-save" :href="qr" download="registration-qr.png">{{t('saveQr')}}</a></div>
  </div>
 </article>
</template>
<style scoped>
.registration-link {display:grid;gap:10px;min-width:0;}
.registration-link-address {display:flex;align-items:center;gap:8px;min-width:0;padding:4px 4px 4px 12px;border-radius:var(--radius-sm);background:var(--canvas);}
#app .registration-link .registration-link-copy {display:inline-flex;align-items:center;justify-content:center;flex:0 0 44px;width:44px;min-width:44px;min-height:44px;padding:0;}
.registration-link code {display:block;flex:1;min-width:0;color:var(--ink);font-size:13px;line-height:1.6;overflow-wrap:anywhere;user-select:all;}
.registration-qr {display:block;justify-self:center;width:240px;max-width:100%;height:auto;aspect-ratio:1;}
.registration-qr-hint {display:none;color:var(--muted);}
.registration-link-footer {display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:10px;min-width:0;}
.registration-link-feedback {flex:1;min-width:0;display:grid;gap:4px;}
.registration-link-actions {display:flex;justify-content:flex-end;flex-wrap:wrap;gap:10px;margin-inline-start:auto;}
.registration-link-actions :deep(.button) {min-width:0;min-height:44px;justify-content:center;text-decoration:none;}
.registration-link p {margin:0;overflow-wrap:anywhere;}
.registration-link .form-error {color:var(--danger);}
.registration-link .success-note {color:var(--success-text);}
#app .registration-link .registration-link-actions .registration-link-share {display:none;}
@media (max-width:700px), (pointer:coarse) {
 #app .registration-link .registration-link-actions .registration-link-share {display:inline-flex;}
 .registration-qr-hint {display:block;}
 #app .registration-link .registration-link-actions .registration-qr-save {display:none;}
}
</style>
