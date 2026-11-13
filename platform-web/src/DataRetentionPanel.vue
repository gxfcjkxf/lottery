<script setup lang="ts">
import {computed,onBeforeUnmount,ref,shallowRef,watch} from 'vue'
import {useOperationToast,withGlobalLoading,vInlineValidation,vWriteGuard,vRefreshButton} from '@lottery/shared'
import ManagementList from '@lottery/shared/ManagementList.vue'
import ManagementTable from '@lottery/shared/ManagementTable.vue'
import TablePagination from '@lottery/shared/TablePagination.vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import SelectControl from '@lottery/shared/SelectControl.vue'
import PlatformTimeInput from '@lottery/shared/PlatformTimeInput.vue'
import {usePlatformI18n} from './i18n'
import {formatPlatformDateTime,type PlatformDisplaySettings} from './platform-format'
import {PlatformApiError} from './platform-api'
import {createDataRetentionApi,getRetentionIntent,setRetentionIntent,discardRetentionIntent,validRetentionInput,retentionFamilies,type RetentionInput,type RetentionPolicy,type RetentionTask,type RetentionPage,type RetentionIntent} from './data-retention-api'
const props=defineProps<{brandId:string;accountId:string;canManage:boolean;canView:boolean;locale:'en'|'zh-CN';displaySettings:PlatformDisplaySettings}>()
const emit=defineEmits<{failure:[cause:unknown];'view-change':[view:'records'|'policy']}>()
const {t,errorMessage,displayError}=usePlatformI18n(),api=createDataRetentionApi()
const tab=ref<'records'|'policy'>('records'),policy=ref<RetentionPolicy|null>(null),draft=ref<RetentionInput|null>(null),page=ref<RetentionPage|null>(null),detail=ref<RetentionTask|null>(null)
const pending=shallowRef<RetentionIntent|null>(getRetentionIntent(props.accountId)),reading=ref(false),writing=ref(false),error=ref<ReturnType<typeof errorMessage>|''>(''),notice=ref(''),pageSize=ref(50)
let generation=0,alive=true
const busy=computed(()=>reading.value||writing.value)
const locked=computed(()=>busy.value||Boolean(pending.value)),unchanged=computed(()=>!!draft.value&&!!policy.value&&draft.value.enabled===policy.value.enabled&&draft.value.execution_day===policy.value.execution_day&&draft.value.execution_time===policy.value.execution_time)
const columns=computed(()=>[{key:'brand',label:t('retention.brand')},{key:'cutoff',label:t('retention.cutoff')},{key:'state',label:t('retention.state')},{key:'deleted',label:t('retention.deleted')},{key:'receipts',label:t('retention.receipts')},{key:'deferred',label:t('retention.deferredCount')},{key:'finished',label:t('retention.finished')},{key:'actions',label:t('common.actions')}])
const time=(value:string|null,timezone:string)=>formatPlatformDateTime(value,{...props.displaySettings,timezone},props.locale)
const total=(task:RetentionTask,key:'deleted'|'receipts'|'deferred')=>Object.values(task.counts).reduce((n,c)=>n+BigInt(c[key]),0n).toString()
function setPolicy(value:RetentionPolicy){policy.value=value;draft.value={version:value.version,enabled:value.enabled,execution_day:value.execution_day,execution_time:value.execution_time}}
async function load(offset=0){const ticket=++generation,brand=props.brandId;if(!props.canView){reading.value=false;error.value=errorMessage(new PlatformApiError('',403,'PERMISSION_DENIED'));return;}reading.value=true;error.value='';try{const [p,list]=await Promise.all([api.policy(),api.tasks(brand,pageSize.value,offset)]);if(!alive||ticket!==generation)return;setPolicy(p);page.value=list}catch(cause){if(alive&&ticket===generation){error.value=errorMessage(cause);emit('failure',cause)}}finally{if(alive&&ticket===generation)reading.value=false}}
async function submit(intent:RetentionIntent){const actor=props.accountId,brand=props.brandId;const current=()=>alive&&actor===props.accountId&&brand===props.brandId;if(busy.value||!props.canManage||!props.canView||intent.conflict)return;pending.value=intent;setRetentionIntent(intent);writing.value=true;error.value='';try{await api.write(intent);if(getRetentionIntent(intent.actor)!==intent)return;discardRetentionIntent(intent);if(!current())return;pending.value=null;notice.value=t('retention.saved');await load(page.value?.offset??0)}catch(cause){if(!current())return;if(cause instanceof PlatformApiError&&cause.status>=400&&cause.status<500){if(cause.status===409)intent.conflict=true;else{discardRetentionIntent(intent);pending.value=null}}error.value=errorMessage(cause);emit('failure',cause)}finally{if(alive&&actor===props.accountId)writing.value=false}}
async function save(){if(!draft.value||!validRetentionInput(draft.value))return;await submit(pending.value??{kind:'policy',actor:props.accountId,key:crypto.randomUUID(),body:Object.freeze({...draft.value}),conflict:false})}
async function retry(task:RetentionTask){await submit(pending.value??{kind:'retry',id:task.id,actor:props.accountId,key:crypto.randomUUID(),conflict:false})}
async function reloadConflict(){const intent=pending.value;if(!intent?.conflict)return;await load(page.value?.offset??0);if(!error.value){discardRetentionIntent(intent);pending.value=null}}
watch(()=>[props.brandId,props.accountId,props.canManage,props.canView],(next,previous)=>{generation++;reading.value=false;if(previous&&next[1]!==previous[1])writing.value=false;pending.value=getRetentionIntent(props.accountId);detail.value=null;page.value=null;policy.value=null;draft.value=null;error.value=notice.value='';void withGlobalLoading(load,0)},{immediate:true,flush:'sync'})
watch(tab,view=>emit('view-change',view),{immediate:true})
onBeforeUnmount(()=>{alive=false;generation++})
useOperationToast(error,v=>displayError(v),'error');useOperationToast(notice,v=>v,'success')
</script>
<template>
 <div class="retention-workspace" data-testid="data-retention">
  <ManagementList :busy="busy">
   <template #tabs><div class="console-tabs" role="tablist" :aria-label="t('retention.title')"><button v-for="view in (['records','policy'] as const)" :key="view" type="button" role="tab" :aria-selected="tab===view" :class="{selected:tab===view}" @click="tab=view">{{t('retention.'+view)}}</button></div></template>
   <template v-if="error&&!pending" #actions><button v-refresh-button="t('retention.reload')" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load,page?.offset??0)">{{t('retention.reload')}}</button></template>
   <template v-if="tab==='records'">
    <ManagementTable :entries="page?.items??[]" :columns="columns" :empty="Boolean(page)&&!page?.items.length" :empty-label="t('retention.empty')">
     <template #brand="{entry}">{{entry.brand_name||t('retention.platform')}}</template><template #cutoff="{entry}">{{time(entry.cutoff_at,entry.timezone)}}</template>
     <template #state="{entry}"><span class="status-pill" :class="entry.state==='completed'?'active':entry.state==='pending'||entry.state==='deferred'?'paused':'retention-failed'">{{t('retention.'+entry.state)}}</span></template>
     <template #deleted="{entry}">{{total(entry,'deleted')}}</template><template #receipts="{entry}">{{total(entry,'receipts')}}</template><template #deferred="{entry}">{{total(entry,'deferred')}}</template><template #finished="{entry}">{{time(entry.finished_at,entry.timezone)}}</template>
     <template #actions="{entry}"><div class="table-actions"><button class="button button-secondary" type="button" @click="detail=entry">{{t('retention.detail')}}</button><button v-if="canManage&&entry.state==='failed'" class="button button-secondary" type="button" :disabled="locked" @click="withGlobalLoading(retry,entry)">{{t('retention.retry')}}</button></div></template>
    </ManagementTable>
   </template>
   <form v-else-if="draft&&policy" novalidate v-inline-validation v-write-guard="busy" class="retention-form ui-form" @submit.prevent="withGlobalLoading(save)">
    <label class="checkbox-label"><input v-model="draft.enabled" type="checkbox" :disabled="locked||!canManage" />{{t('retention.enabled')}}</label>
    <div class="form-row"><label>{{t('retention.day')}}<SelectControl v-model="draft.execution_day" :options="Array.from({length:28},(_,i)=>({value:i+1,label:String(i+1)}))" :label="t('retention.day')" :disabled="locked||!canManage" /></label><label>{{t('retention.time')}}<PlatformTimeInput v-model="draft.execution_time" :display-settings="displaySettings" :locale="locale" :label="t('retention.time')" minute-precision required :disabled="locked||!canManage" /></label></div>
    <dl class="retention-facts"><div><dt>{{t('retention.timezone')}}</dt><dd>{{policy.timezone}}</dd></div><template v-if="unchanged&&policy.next_run"><div><dt>{{t('retention.next')}}</dt><dd>{{time(policy.next_run.execute_at,policy.timezone)}}</dd></div><div><dt>{{t('retention.cutoff')}}</dt><dd>{{time(policy.next_run.cutoff_at,policy.timezone)}}</dd></div></template></dl>
    <div class="ui-form-actions"><button v-if="canManage&&!pending" type="submit" class="button button-primary" :disabled="busy||unchanged||!validRetentionInput(draft)">{{t('retention.save')}}</button></div>
   </form>
   <template v-if="tab==='records'&&page?.items.length" #pagination><TablePagination :total="page.total_count" :page="Math.floor(page.offset/page.limit)+1" :count="page.items.length" :locale="locale" :page-size="pageSize" :busy="busy" :previous-disabled="busy||page.offset===0" :next-disabled="busy||BigInt(page.offset+page.items.length)>=BigInt(page.total_count)" :go-to="target=>load((target-1)*pageSize)" :change-page-size="async size=>{pageSize=size;await load(0)}" /></template>
  </ManagementList>
  <div v-if="pending" class="message"><span>{{t(pending.conflict?'retention.conflict':'retention.unknown')}}</span><button v-if="pending.conflict" v-refresh-button="t('retention.reload')" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(reloadConflict)">{{t('retention.reload')}}</button><button v-else class="button button-secondary" :disabled="busy" @click="withGlobalLoading(submit,pending)">{{t('retention.retryOriginal')}}</button></div>
  <DetailModal v-if="detail" :title="t('retention.detail')" :close-label="t('retention.close')" @close="detail=null">
   <dl class="retention-facts"><div><dt>{{t('retention.brand')}}</dt><dd>{{detail.brand_name||t('retention.platform')}}</dd></div><div><dt>{{t('retention.cutoff')}}</dt><dd>{{time(detail.cutoff_at,detail.timezone)}}</dd></div><div><dt>{{t('retention.timezone')}}</dt><dd>{{detail.timezone}}</dd></div><div><dt>{{t('retention.state')}}</dt><dd>{{t('retention.'+detail.state)}}</dd></div><div v-if="detail.error_code"><dt>{{t('retention.error')}}</dt><dd>{{t('retention.batchFailed')}}</dd></div></dl>
   <ManagementTable :entries="retentionFamilies.map(id=>({id,counts:detail!.counts[id]})).filter(row=>row.counts)" :columns="[{key:'family',label:t('retention.family')},{key:'deleted',label:t('retention.deleted')},{key:'receipts',label:t('retention.receipts')},{key:'deferred',label:t('retention.deferredCount')}]" :empty="false" :empty-label="t('retention.empty')"><template #family="{entry}">{{t('retention.family.'+entry.id)}}</template><template #deleted="{entry}">{{entry.counts.deleted}}</template><template #receipts="{entry}">{{entry.counts.receipts}}</template><template #deferred="{entry}">{{entry.counts.deferred}}</template></ManagementTable>
  </DetailModal>
 </div>
</template>
<style scoped>
.retention-failed{background:var(--danger-bg);color:var(--danger-text)}.retention-workspace{display:grid;gap:var(--console-block-gap);min-width:0}.retention-form{padding:12px;display:grid;gap:12px;max-width:640px}.retention-facts{display:grid;gap:8px;margin:0}.retention-facts>div{display:grid;grid-template-columns:minmax(90px,140px) minmax(0,1fr);gap:12px}.retention-facts dt{color:var(--muted)}.retention-facts dd{margin:0;overflow-wrap:anywhere}.checkbox-label{display:flex;align-items:center;gap:8px}.table-actions{display:flex;gap:8px;flex-wrap:wrap}.message{display:flex;gap:12px;align-items:center;flex-wrap:wrap}@media(max-width:480px){.retention-form .form-row{grid-template-columns:1fr}.retention-facts>div{grid-template-columns:100px minmax(0,1fr)}}
</style>
