<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ApiError } from '../../shared/src/api'
import { createCatalogI18n } from '../../shared/src/localization'
import { announcementsCatalog, type AnnouncementKey } from '../../shared/src/announcements-catalog'
import { createAnnouncementsApi, type Announcement } from '../../shared/src/announcements'
import { modalDialogs } from '../../shared/src/modal-state'
import { useOperationToast, withGlobalLoading } from '@lottery/shared'
import AnnouncementContent from '../../shared/src/AnnouncementContent.vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import SiteIcon from '@lottery/shared/SiteIcon'
const props=defineProps<{brandCode?:string;brandId:string;memberId?:string;locale:'en'|'zh-CN';path:string;list?:boolean}>()
const emit=defineEmits<{'auth-expired':[]}>()
const i18n=createCatalogI18n(announcementsCatalog,props.locale),t=i18n.t
watch(()=>props.locale,v=>i18n.setLocale(v))
const sessionKey='lottery-announcement-session'
let session=sessionStorage.getItem(sessionKey)
if(!session){session=crypto.randomUUID();sessionStorage.setItem(sessionKey,session)}
const api=computed(()=>createAnnouncementsApi(props.brandCode?`/api/v1/b/${encodeURIComponent(props.brandCode)}`:'/api/v1',undefined,session!))
const dialog=ref<InstanceType<typeof DetailModal>|null>(null)
const rows=ref<Announcement[]>([]),queue=ref<Announcement[]>([]),index=ref(0),open=ref(false),automatic=ref(false),loading=ref(false),writing=ref(false),error=ref<AnnouncementKey|null>(null),mute=ref(false)
const selected=computed(()=>queue.value[index.value]),content=computed(()=>selected.value?.config.content[props.locale])
const allowMute=computed(()=>automatic.value&&queue.value.length>0&&queue.value.every(v=>v.config.allow_mute))
const entry=computed(()=>['/','/login','/register'].includes(props.path))
let ticket=0,scopeGeneration=0,alive=true,lastTrigger=''
const viewed=new Set<string>()
function failed(e:unknown,key:AnnouncementKey){error.value=key;if(e instanceof ApiError&&e.status===401)emit('auth-expired')}
async function seen(){const epoch=scopeGeneration,client=api.value;const item=selected.value;if(!item||!automatic.value)return;const key=`${props.brandId}:${props.memberId??'guest'}:${item.id}:${item.generation}`;if(viewed.has(key))return
 await nextTick();await new Promise<void>(resolve=>requestAnimationFrame(()=>resolve()));if(epoch!==scopeGeneration||!open.value||!alive||selected.value?.id!==item.id||!dialog.value?.$el.open)return
 try{await client.receipt('view',[{id:item.id,generation:item.generation}]);if(alive&&epoch===scopeGeneration)viewed.add(key)}catch(e){if(alive&&epoch===scopeGeneration)failed(e,'receiptFailure')}
}
async function show(items:Announcement[],auto:boolean){queue.value=items;index.value=0;automatic.value=auto;mute.value=false;open.value=items.length>0;if(open.value)await seen()}
async function load(){const current=++ticket,client=api.value,member=props.memberId;loading.value=true;error.value=null
 try{const feed=await client.list();if(current!==ticket||!alive)return;if(member&&feed.length)await client.receipt('sync');if(current!==ticket||!alive)return;rows.value=feed;await tryAutomatic()}catch(e){if(alive&&current===ticket)failed(e,'failure')}finally{if(alive&&current===ticket)loading.value=false}}
async function tryAutomatic(){if(!entry.value||open.value||modalDialogs.length)return;const trigger=`${props.brandId}:${props.memberId??'guest'}:${props.path}`;if(lastTrigger===trigger)return;const eligible=rows.value.filter(v=>v.eligible);if(!eligible.length)return;lastTrigger=trigger;await show(eligible,true)}
async function dismiss(){if(writing.value)return;const epoch=scopeGeneration;if(mute.value&&allowMute.value){writing.value=true;try{await api.value.receipt('mute',queue.value.map(v=>({id:v.id,generation:v.generation})))}catch(e){if(alive&&epoch===scopeGeneration)failed(e,'receiptFailure');return}finally{if(alive&&epoch===scopeGeneration)writing.value=false}}
 if(!alive||epoch!==scopeGeneration)return;open.value=false;mute.value=false}
async function move(delta:number){index.value+=delta;await nextTick();dialog.value?.$el.querySelector('.detail-modal-content')?.scrollTo({top:0});await seen()}
watch(()=>[props.brandId,props.memberId,props.brandCode],()=>{scopeGeneration++;ticket++;open.value=false;rows.value=[];queue.value=[];loading.value=writing.value=false;error.value=null;lastTrigger='';void load()},{immediate:true,flush:'sync'})
watch(()=>props.path,()=>{scopeGeneration++;ticket++;open.value=false;loading.value=writing.value=false;lastTrigger='';if(entry.value||props.list)void load()},{flush:'sync'})
watch(()=>modalDialogs.length,n=>{if(n===0)void tryAutomatic()})
onBeforeUnmount(()=>{alive=false;ticket++;scopeGeneration++})
useOperationToast(()=>error.value,key=>key?t(key):'','error')
</script>
<template>
 <Teleport v-if="list" to="#announcements-directory"><section class="page-section user-record-list announcement-directory" :aria-label="t('title')" :aria-busy="loading">
  <div v-if="error" class="announcement-error" role="alert"><span>{{t(error)}}</span><button type="button" class="button button-secondary" @click="withGlobalLoading(load)">{{t('retry')}}</button></div>
  <p v-else-if="!loading&&!rows.length" class="empty-state">{{t('empty')}}</p>
  <button v-for="item in rows" :key="item.id" type="button" class="announcement-row" aria-haspopup="dialog" @click="withGlobalLoading(show,[item],false)"><span class="announcement-row-copy"><strong>{{item.config.content[locale].title}}</strong><span class="announcement-row-meta"><span>{{t(item.brand_id?'brand':'platform')}}</span><span v-if="item.config.pinned" class="announcement-row-pin">{{t('pinned')}}</span></span></span><SiteIcon name="next" /></button>
 </section></Teleport>
 <DetailModal ref="dialog" v-if="open&&selected&&content" class="user-announcement-dialog" :title="content.title" :close-label="t('close')" :close-disabled="writing" @close="withGlobalLoading(dismiss)">
  <AnnouncementContent :content="content" />
  <template #footer>
   <div class="announcement-footer">
    <div v-if="queue.length>1" class="announcement-progress" aria-live="polite">{{index+1}} / {{queue.length}}</div>
    <label v-if="allowMute" class="announcement-mute"><input v-model="mute" type="checkbox" :disabled="writing">{{t('muteToday')}}</label>
    <div v-if="error==='receiptFailure'" role="alert" class="announcement-error"><span>{{t(error)}}</span><button type="button" class="button button-secondary" @click="withGlobalLoading(seen)">{{t('retry')}}</button></div>
    <div class="announcement-buttons" data-sequence-navigation><button v-if="queue.length>1" type="button" class="button button-secondary" :disabled="index===0||writing" @click="withGlobalLoading(move,-1)">{{t('previous')}}</button><button v-if="index<queue.length-1" type="button" class="button button-primary" :disabled="writing" @click="withGlobalLoading(move,1)">{{t('next')}}</button><button v-else type="button" class="button button-primary" :disabled="writing" @click="withGlobalLoading(dismiss)">{{t('done')}}</button></div>
   </div>
  </template>
 </DetailModal>
</template>
<style scoped>
.announcement-directory{display:grid;width:100%;min-width:0;box-sizing:border-box}
.announcement-row{display:flex;align-items:center;justify-content:space-between;gap:12px;text-align:left;width:100%;min-width:0;min-height:var(--control-height);padding:8px 12px;border:0;background:transparent;color:var(--ink);font:inherit;cursor:pointer;box-sizing:border-box}
.announcement-row + .announcement-row{border-top:1px solid var(--line)}
.announcement-row:first-child{border-start-start-radius:var(--radius);border-start-end-radius:var(--radius)}
.announcement-row:last-child{border-end-start-radius:var(--radius);border-end-end-radius:var(--radius)}
.announcement-row:hover{background:var(--canvas)}
.announcement-row:focus-visible{outline:2px solid var(--primary);outline-offset:-2px}
.announcement-row-copy{display:grid;gap:4px;min-width:0;flex:1;overflow-wrap:anywhere}
.announcement-row strong{font-size:var(--body-font-size);font-weight:650;line-height:1.5}
.announcement-row-meta{display:flex;flex-wrap:wrap;gap:4px 12px;color:var(--muted);font-size:var(--body-font-size)}
.announcement-row-pin{color:var(--primary)}
.announcement-row :deep(.site-icon){color:var(--muted)}
.announcement-directory > .announcement-error{padding:8px 12px}
.announcement-footer{display:grid;gap:8px;width:100%;min-width:0}.announcement-progress{text-align:center;font-variant-numeric:tabular-nums;color:var(--muted)}.announcement-mute{display:flex;align-items:center;gap:8px;min-height:var(--control-height);font-size:var(--body-font-size)}.announcement-buttons{display:flex;justify-content:flex-end;gap:8px}.announcement-error{display:flex;align-items:center;justify-content:space-between;gap:8px;flex-wrap:wrap;color:var(--danger)}
</style>
