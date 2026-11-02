<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import PuzzleCaptcha from './PuzzleCaptcha.vue'
import { captchaCatalog } from './captcha-catalog'
import { vRefreshButton } from './refresh-button'
import { withGlobalLoading } from './global-loading'
import type { AuthChallenge } from './auth'
import { useCaptchaRefresh } from './captcha-refresh'
const props=defineProps<{locale:'en'|'zh-CN';configUrl:string;challengeUrl:string;disabled?:boolean}>()
const emit=defineEmits<{'update:ready':[value:boolean]}>()
const text=computed(()=>captchaCatalog[props.locale])
const enabled=ref(false),loaded=ref(false),busy=ref(false),failed=ref(false),answer=ref(''),challenge=ref<AuthChallenge|null>(null)
const now=useCaptchaRefresh(()=>challenge.value,refresh,()=>busy.value||Boolean(props.disabled))
const ready=computed(()=>loaded.value&&!busy.value&&!failed.value&&(!enabled.value||Boolean(challenge.value&&Date.parse(challenge.value.expires_at)>now.value&&answer.value.trim())))
watch(ready,value=>emit('update:ready',value),{immediate:true})
async function read<T>(url:string):Promise<T>{
 const response=await fetch(url,{credentials:'same-origin',headers:{Accept:'application/json'}})
 const body=await response.json()
 if(!response.ok||body.success!==true||!body.data)throw new Error('Invalid captcha response')
 return body.data as T
}
async function refresh(){
 if(busy.value)return
 busy.value=true;failed.value=false;answer.value='';challenge.value=null
 try{
  const cfg=await read<{captcha_enabled:boolean;auth?:{captcha_enabled:boolean}}>(props.configUrl)
  const value=props.configUrl.endsWith('/context')?cfg.auth?.captcha_enabled:cfg.captcha_enabled
  if(typeof value!=='boolean')throw new Error('Invalid captcha configuration')
  enabled.value=value;loaded.value=true
  if(value){
   const next=await read<AuthChallenge>(props.challengeUrl)
   if(typeof next.id!=='string'||!next.puzzle||typeof next.puzzle.image!=='string'||typeof next.puzzle.piece!=='string'||!Number.isFinite(Date.parse(next.expires_at)))throw new Error('Invalid captcha challenge')
   challenge.value=next
  }
 }catch{failed.value=true}finally{busy.value=false}
}
function input(){return enabled.value?{captcha_id:challenge.value?.id,captcha_answer:answer.value.trim()}:{} }
defineExpose({input,refresh})
onMounted(refresh)
</script>
<template>
 <div v-if="enabled||failed" class="login-captcha">
  <div class="login-captcha__heading"><span>{{text.title}}</span><button v-refresh-button="text.refresh" class="button button-secondary" type="button" :disabled="busy||disabled" @click="withGlobalLoading(refresh)">{{text.refresh}}</button></div>
  <PuzzleCaptcha v-if="challenge" v-model="answer" :challenge="challenge" :locale="locale" :disabled="busy||disabled" />
  <p v-if="failed" role="alert">{{text.failed}}</p>
 </div>
</template>
<style scoped>
.login-captcha{display:grid;gap:8px;min-width:0;padding:12px;border:1px solid var(--line);border-radius:var(--radius-sm);background:var(--canvas)}
.login-captcha__heading{display:flex;align-items:center;justify-content:space-between;gap:8px;font-weight:650}
.login-captcha p{margin:0;color:var(--danger-text)}
</style>
