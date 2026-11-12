<script setup lang="ts">
import { ref, watch } from 'vue'
import { pendingManualRecharges, type ManualRechargeIntent } from './manual-recharge-state'
import { vWriteGuard, vInlineValidation, withGlobalLoading, formValidationText } from '@lottery/shared'
import RechargeQuotaPanel from '@lottery/shared/RechargeQuotaPanel.vue'
import { useOperationToast } from '@lottery/shared'
import { AdminApiError } from './admin-api'
import { createFinanceApi, FinanceApiError, isPositiveAmount } from './finance-api'
import { useAdminI18n } from './i18n'
const props=defineProps<{accountId:string;brandId:string;memberId:string;username:string}>()
const emit=defineEmits<{credited:[];sessionInvalid:[];pending:[value:boolean]}>()
const {t,locale,errorMessage,displayError}=useAdminI18n()
const api=createFinanceApi(),quota=ref<InstanceType<typeof RechargeQuotaPanel>|null>(null)
const amount=ref(''),remark=ref(''),busy=ref(false),error=ref<ReturnType<typeof errorMessage>|''>(''),notice=ref(false)
const scope = `${props.accountId}:${props.brandId}:${props.memberId}`
const frozen=ref<ManualRechargeIntent|null>(pendingManualRecharges.get(scope) ?? null)
if(frozen.value){amount.value=frozen.value.body.points;remark.value=frozen.value.body.remark}
watch(frozen,value=>{if(value)pendingManualRecharges.set(scope,value);else pendingManualRecharges.delete(scope);emit('pending',!!value)},{flush:'sync',immediate:true})
function updateAmount(event:Event){
 const input=event.target as HTMLInputElement
 if(/^\d*(?:\.\d{0,2})?$/.test(input.value))amount.value=input.value
 else input.value=amount.value
}
async function submit(){
 if(busy.value||!props.memberId)return
 if(!frozen.value){if(!isPositiveAmount(amount.value))return;frozen.value={brand:props.brandId,body:{member_id:props.memberId,points:amount.value,remark:remark.value,reason:''},key:crypto.randomUUID()}}
 const request=frozen.value;busy.value=true;error.value='';notice.value=false
 try{
  await api.manualRecharge(request.brand,request.body,request.key)
  frozen.value=null;amount.value='';remark.value='';notice.value=true
  emit('credited');await quota.value?.reload()
 }catch(cause){
  if((cause instanceof FinanceApiError || cause instanceof AdminApiError)&&cause.status>=400&&cause.status<500)frozen.value=null
  if((cause instanceof FinanceApiError || cause instanceof AdminApiError)&&cause.status===401)emit('sessionInvalid')
  error.value=errorMessage(cause)
 }finally{busy.value=false}
}
useOperationToast(() => error.value, value => displayError(value), 'error')
useOperationToast(() => notice.value, () => t('admin.recharge.credited'), 'success')
</script>
<template>
 <div class="console-stack" data-testid="manual-recharge-form">
  <p><strong>{{t('admin.recharge.member')}}</strong> {{username}}</p>
  <form v-write-guard="busy" novalidate v-inline-validation class="manual-form ui-form" @submit.prevent="withGlobalLoading(submit)">
   <div class="recharge-amount-row">
    <label>{{t('admin.workbench.field.rechargePoints')}}<input :value="amount" @input="updateAmount" inputmode="decimal" required pattern="(0|[1-9][0-9]*)([.][0-9]{1,2})?" :data-validation-format="formValidationText('positivePoints')" :disabled="busy||!!frozen" /></label>
    <RechargeQuotaPanel external-feedback summary-only ref="quota" :brand-id="brandId" :locale="locale" base-path="/api/v1/admin" @failure="cause => { error = errorMessage(cause,'read') }" />
   </div>
   <label>{{t('admin.copy.fd2d2c8b')}}<input v-model.trim="remark" maxlength="500" :disabled="busy||!!frozen" /></label>
   <button type="submit" class="button button-primary" :disabled="busy||(!frozen&&!isPositiveAmount(amount))">{{t(frozen?'admin.recharge.retry':'admin.recharge.credit')}}</button>
  </form>
 </div>
</template>
<style scoped>
.manual-form {display:grid;gap:12px;min-width:0;}
.manual-form label {display:grid;gap:6px;font-size:var(--body-font-size);min-width:0;}
.manual-form input {width:100%;box-sizing:border-box;min-width:0;}
.recharge-amount-row {display:flex;flex-wrap:wrap;align-items:end;gap:6px 12px;min-width:0;}
.recharge-amount-row label {width:140px;max-width:100%;}
.manual-form .button {justify-self:start;}
p {margin:0;overflow-wrap:anywhere;}
</style>
