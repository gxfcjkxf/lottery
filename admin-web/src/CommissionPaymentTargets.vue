<script setup lang="ts">
import ManagementList from "@lottery/shared/ManagementList.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
const agentSearchPending = ref(false);
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue";
import SelectControl from '@lottery/shared/SelectControl.vue'
import {vInlineValidation} from "@lottery/shared";
import {vRefreshButton} from "@lottery/shared";
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { AssociationName, formatDisplayDateTime, useOperationToast, withGlobalLoading, vGlobalLoading } from '@lottery/shared'
import ManagementTable from '@lottery/shared/ManagementTable.vue'
import { AdminApiError, type AdminAccount } from './admin-api'
import { createCommissionAdjustmentsApi, commissionAdjustmentPermissions, type CommissionPaymentBeneficiaryPage } from './commission-adjustments-api'
import { useAdminI18n } from './i18n'
const pageSize = ref(20)

const props=defineProps<{account:AdminAccount;brandId:string;paymentId:string}>()
const emit=defineEmits<{'session-invalid':[]}>()
const {t,locale,displayError,errorMessage}=useAdminI18n()
const api=createCommissionAdjustmentsApi()
const page=ref<CommissionPaymentBeneficiaryPage|null>(null),busy=ref(false),error=ref(''),agent=ref(''),state=ref('')
const committed=ref<{agent_id?:string;state?:string}>({})
const rights=computed(()=>commissionAdjustmentPermissions(props.account,props.brandId))
const scope=computed(()=>JSON.stringify([props.account,props.brandId,props.paymentId,rights.value]))
let generation=0,alive=true
const columns=computed(()=>[
 {key:'agent',label:t('admin.commission.ui.agent')}, {key:'original',label:t('admin.commission.ui.original'),cellClass:'numeric'},
 {key:'adjusted',label:t('admin.commission.ui.adjusted'),cellClass:'numeric'}, {key:'state',label:t('admin.commission.ui.targetStatus')},
 {key:'paidAt',label:t('admin.commission.ui.paidAt')},
])
async function load(offset=0) {
 if (!rights.value.view) return
 const ticket=++generation,captured=scope.value
 error.value='';busy.value=true
 try {
  const result=await api.listBeneficiaries(props.brandId,props.paymentId, pageSize.value,offset,committed.value)
  if(alive && generation===ticket && scope.value===captured) page.value=result
 } catch(cause) {
  if(!alive || generation!==ticket || scope.value!==captured)return
  error.value=displayError(errorMessage(cause))
  if(cause instanceof AdminApiError && cause.status===401)emit('session-invalid')
 } finally {if(generation===ticket)busy.value=false}
}
async function search(){
  if (agentSearchPending.value) return;committed.value={...(agent.value?{agent_id:agent.value}:{}),...(state.value?{state:state.value}:{})};await load()}
watch(scope,()=>{generation++;page.value=null;agent.value=state.value='';committed.value={};if(rights.value.view)void load()},{immediate:true})
onBeforeUnmount(()=>{alive=false;generation++})
useOperationToast(error,v=>v,'error')

</script>
<template>
 <ManagementList embedded class="commission-detail" data-testid="commission-payment-targets">
  <template #filters><form v-inline-validation novalidate class="commission-toolbar ui-filter-bar" @submit.prevent="withGlobalLoading(search)">
   <label><FilterAssociationSearch v-model="agent" v-model:pending="agentSearchPending" :brand-id="brandId" surface="admin" resource="commission" kind="agent" :label="t('admin.commission.ui.agent')" :locale="locale" :scope-key="scope" :enabled="rights.view" /></label>
   <label><SelectControl :placeholder="t('admin.commission.ui.targetStatus')" v-model="state" :label="String(t('admin.commission.ui.targetStatus'))" :options="[{ value: '', label: (String(t('admin.commission.ui.all'))).trim() }, { value: 'pending', label: (String(t('admin.commission.ui.targetPending'))).trim() }, { value: 'paid', label: (String(t('admin.commission.ui.targetPaid'))).trim() }]" /></label>
   <button class="button button-primary" type="submit" :disabled="agentSearchPending || busy">{{ t('admin.commission.ui.query') }}</button>
</form></template>
  <p v-if="error" class="commission-warning" role="alert">{{ error }} <button v-refresh-button="t('admin.commission.ui.retryRead')" class="button" @click="withGlobalLoading(load)">{{ t('admin.commission.ui.retryRead') }}</button></p>
  <p v-if="busy" v-global-loading>{{ t('admin.copy.ab7239f3') }}</p>
  <ManagementTable :entries="page?.items??[]" :columns="columns" :empty="!!page && !page.items.length" :empty-label="t('admin.commission.ui.empty')">
   <template #agent="{entry}"><AssociationName :record="entry" kind="agent" :id="entry.agent_id" :locale="locale" /></template>
   <template #original="{entry}">{{ entry.original_points }}</template><template #adjusted="{entry}">{{ entry.adjusted_points??'—' }}</template>
   <template #state="{entry}"><span class="commission-status" :class="entry.state">{{ entry.state==='paid'?t('admin.commission.ui.targetPaid'):t('admin.commission.ui.targetPending') }}</span></template>
   <template #paidAt="{entry}">{{ formatDisplayDateTime(entry.paid_at,locale) }}</template>
  </ManagementTable>

 <template #pagination><TablePagination v-if="page" class="commission-pager" :total="page.total_count" :page="Math.floor(page.offset / pageSize + 1)" :count="page.items.length" :locale="locale" :page-size="pageSize" :previous-disabled="busy || page.offset===0" :next-disabled="busy || BigInt(page.offset+pageSize)>=BigInt(page.total_count)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => load((target - 1) * pageSize)" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" :busy="busy" /></template></ManagementList>
</template>
