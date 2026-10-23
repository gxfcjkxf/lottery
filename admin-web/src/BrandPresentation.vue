<script setup lang="ts">
import { vWriteGuard } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import DetailModal from '@lottery/shared/DetailModal.vue';
import { vGlobalLoading } from "@lottery/shared";
import { vInlineValidation } from '@lottery/shared'
import {computed,onUnmounted,ref,watch} from "vue";
import {AdminApiError,createIdempotencyKey,type AdminAccount} from "./admin-api";
import {useAdminI18n} from "./i18n";
import type {LocalizedMessage} from "@lottery/shared";
import PresentationFields from "./PresentationFields.vue";
import {brandPresentationPermissions,createBrandPresentationApi,validBrandPresentationConfig,type BrandPresentationEffective,type BrandPresentationConfig,type BrandPresentationRecord} from "./brand-presentation-api";
import {brandPresentationSessionGeneration,classifyBrandPresentationFailure,clearPendingBrandPresentationWrite,createBrandPresentationRequestGuard,getPendingBrandPresentationWrite,setPendingBrandPresentationWrite,updatePendingBrandPresentationPhase,type PendingBrandPresentationWrite} from "./brand-presentation-state";

const props=defineProps<{account:AdminAccount;brandId:string}>();
const emit=defineEmits<{(event:"session-invalid"):void;(event:"loaded",value:{accountId:string;record:BrandPresentationRecord}):void}>();
const api=createBrandPresentationApi();
const {t,message,display}=useAdminI18n();
const readGuard=createBrandPresentationRequestGuard(),writeGuard=createBrandPresentationRequestGuard();
const rights=computed(()=>brandPresentationPermissions(props.account,props.brandId));
const record=ref<BrandPresentationRecord|null>(null),draft=ref<BrandPresentationConfig|null>(null),reason=ref("");
const uploadBusy=ref(false);
const defaults=ref<BrandPresentationEffective|null>(null);
const confirmationOpen=ref(false),readFailed=ref(false);
const pending=ref<PendingBrandPresentationWrite|null>(null);
const loading=ref(false),busy=ref(false),error=ref<string|LocalizedMessage>(""),notice=ref<string|LocalizedMessage>(""),conflictRead=ref(false);
// Drafts may be Vue proxies; this bounded model consists solely of JSON data.
const clone=<T,>(v:T):T=>JSON.parse(JSON.stringify(v)) as T;
const scope=()=>({accountId:props.account.id,brandId:props.brandId});
const unknown=computed(()=>pending.value?.phase==="unknown"),conflict=computed(()=>pending.value?.phase==="conflict"),reviewing=computed(()=>pending.value?.phase==="review");
const editable=computed(()=>rights.value.write&&record.value?.status!=="disabled"&&!pending.value&&!uploadBusy.value&&!busy.value&&!loading.value&&!readFailed.value);
let live=true;
function current(guard:ReturnType<typeof createBrandPresentationRequestGuard>,token:number,accountId:string,brandId:string,epoch:number){return live&&guard.isCurrent(token)&&epoch===brandPresentationSessionGeneration()&&accountId===props.account.id&&brandId===props.brandId}
function sessionError(cause:unknown,guard:ReturnType<typeof createBrandPresentationRequestGuard>,token:number,id:string,brand:string,epoch:number){if(cause instanceof AdminApiError&&cause.status===401&&current(guard,token,id,brand,epoch))emit("session-invalid")}
async function readLatest(){const {accountId,brandId}=scope(),epoch=brandPresentationSessionGeneration(),token=readGuard.begin();loading.value=true;error.value="";
 try{const [next,base]=await Promise.all([api.get(brandId),api.defaults(brandId)]);if(!current(readGuard,token,accountId,brandId,epoch))return false;record.value=next;defaults.value=base;readFailed.value=false;if(!pending.value)draft.value=clone(next.config);if(conflict.value)conflictRead.value=true;emit("loaded",{accountId,record:next});return true}
 catch(cause){if(current(readGuard,token,accountId,brandId,epoch)){readFailed.value=true;error.value=cause instanceof Error?publicErrorMessage(cause,'read'):message('admin.copy.9e490389');sessionError(cause,readGuard,token,accountId,brandId,epoch)}return false}
 finally{if(current(readGuard,token,accountId,brandId,epoch))loading.value=false}
}
function prepare(){error.value="";notice.value="";if(!editable.value||!record.value||!draft.value)return;
 if(!validBrandPresentationConfig(draft.value)){error.value=message('admin.copy.e8d079a3');return}
 const intent:PendingBrandPresentationWrite={...scope(),body:{version:record.value.version,config:clone(draft.value),reason:reason.value},key:createIdempotencyKey(),phase:"review"};setPendingBrandPresentationWrite(scope(),intent);pending.value=getPendingBrandPresentationWrite(scope());conflictRead.value=false;confirmationOpen.value=true;
}
function cancel(){const intent=pending.value;if(busy.value||!intent||(intent.phase!=="review"&&!(intent.phase==="conflict"&&conflictRead.value)))return;clearPendingBrandPresentationWrite(scope(),intent.key);pending.value=getPendingBrandPresentationWrite(scope());if(intent.phase==="conflict"&&record.value)draft.value=clone(record.value.config);confirmationOpen.value=false;notice.value=""}
function closeConfirmation(){if(busy.value)return;if(reviewing.value)cancel();else confirmationOpen.value=false}
function retain(intent:PendingBrandPresentationWrite,phase:"unknown"|"conflict"){
 const own={accountId:intent.accountId,brandId:intent.brandId};const existing=getPendingBrandPresentationWrite(own);
 if(!existing)setPendingBrandPresentationWrite(own,{...intent,phase});else if(existing.key===intent.key)updatePendingBrandPresentationPhase(own,intent.key,phase);
}
async function send(retry=false){const intent=pending.value;if(!intent||busy.value||!rights.value.write||record.value?.status==="disabled"||(retry?intent.phase!=="unknown":intent.phase!=="review"))return;
 const epoch=brandPresentationSessionGeneration(),token=writeGuard.begin();busy.value=true;error.value="";notice.value="";retain(intent,"unknown");pending.value=getPendingBrandPresentationWrite(scope());
 try{await api.put(intent.brandId,intent.body,intent.key);if(epoch!==brandPresentationSessionGeneration())return;
  clearPendingBrandPresentationWrite({accountId:intent.accountId,brandId:intent.brandId},intent.key);
  if(!current(writeGuard,token,intent.accountId,intent.brandId,epoch))return;pending.value=getPendingBrandPresentationWrite(scope());confirmationOpen.value=false;notice.value=message('admin.settings.saved');
  const refreshed=await readLatest();if(current(writeGuard,token,intent.accountId,intent.brandId,epoch))notice.value=refreshed?message('admin.settings.saved'):message('admin.settings.savedReadFailed');
 }catch(cause){if(epoch!==brandPresentationSessionGeneration())return;const status=cause instanceof AdminApiError?cause.status:undefined;const kind=classifyBrandPresentationFailure(status);
  if(kind!=="definitive")retain(intent,kind);else clearPendingBrandPresentationWrite({accountId:intent.accountId,brandId:intent.brandId},intent.key);
  if(!current(writeGuard,token,intent.accountId,intent.brandId,epoch))return;pending.value=getPendingBrandPresentationWrite(scope());if(!pending.value)confirmationOpen.value=false;
  error.value=kind==="unknown"?message('admin.copy.f6ff251f'):kind==="conflict"?message('admin.copy.f97a87a8'):cause instanceof Error?publicErrorMessage(cause):message('admin.copy.38943df1');
  if(kind==="conflict"){conflictRead.value=false;await readLatest()}sessionError(cause,writeGuard,token,intent.accountId,intent.brandId,epoch);
 }finally{if(current(writeGuard,token,intent.accountId,intent.brandId,epoch))busy.value=false}
}
watch(()=>[props.account.id,props.brandId],()=>{readGuard.invalidate();writeGuard.invalidate();record.value=null;defaults.value=null;readFailed.value=false;loading.value=false;busy.value=false;error.value="";notice.value="";pending.value=getPendingBrandPresentationWrite(scope());draft.value=pending.value?clone(pending.value.body.config):null;reason.value=pending.value?.body.reason??"";confirmationOpen.value=Boolean(pending.value);conflictRead.value=false;if(rights.value.view){void readLatest()}},{immediate:true});
onUnmounted(()=>{live=false;readGuard.invalidate();writeGuard.invalidate()});
</script>

<template>
 <section class="brand-presentation brand-settings-panel" :aria-label="t('admin.copy.5f829ca8')">
  <p v-if="!rights.view" role="status">{{t('admin.copy.d8a49c2f')}}</p>
  <template v-else>
   <p v-global-loading v-if="loading&&!record" role="status">{{t('admin.copy.efd6ec13')}}</p>
   <p v-if="record?.status==='disabled'" role="status">{{t('admin.copy.81b3d5fa')}}</p>
   <form novalidate v-inline-validation v-if="record&&draft&&defaults" @submit.prevent="withGlobalLoading(prepare)">
    <PresentationFields v-model:config="draft" :defaults="defaults" :brand-id="brandId" @session-invalid="emit('session-invalid')" @uploading="uploadBusy=$event" :disabled="!editable">
     <template #actions><button class="button button-primary" type="submit" :disabled="!editable">{{t('presentation.save')}}</button></template>
    </PresentationFields>
   </form>
   <button v-if="pending" class="button button-secondary" type="button" @click="confirmationOpen=true">{{t('admin.settings.reviewPending')}}</button>
   <button v-if="readFailed" class="button button-secondary" type="button" :disabled="loading" @click="withGlobalLoading(readLatest)">{{t('admin.settings.readAgain')}}</button>
   <DetailModal v-if="pending&&confirmationOpen" class="brand-settings-dialog" :title="t('admin.settings.confirmSave')" :close-label="t('association.close')" @close="closeConfirmation">
    <p>{{t('admin.settings.confirmPresentation')}}</p>
    <p v-if="pending.body.reason">{{pending.body.reason}}</p>
    <p v-if="unknown" class="message" role="alert">{{t('admin.settings.unknown')}}</p>
    <p v-if="conflict" class="message" role="alert">{{t('admin.settings.conflict')}}</p>
    <template #footer>
     <button v-if="reviewing" class="button button-secondary" type="button" :disabled="busy" @click="cancel">{{t('admin.copy.96f851ea')}}</button>
     <button v-write-guard="busy" v-if="reviewing" class="button button-primary" type="button" :disabled="busy" @click="withGlobalLoading(send, false)">{{t('admin.copy.a1a8013d')}}</button>
     <button v-write-guard="busy" v-if="unknown" class="button button-primary" type="button" :disabled="busy" @click="withGlobalLoading(send, true)">{{t('admin.settings.retryOperation')}}</button>
     <button v-if="conflict&&!conflictRead" class="button button-secondary" type="button" :disabled="busy||loading" @click="withGlobalLoading(readLatest)">{{t('admin.settings.readAgain')}}</button>
     <button v-if="conflict&&conflictRead" class="button button-secondary" type="button" :disabled="busy" @click="cancel">{{t('admin.settings.startAgain')}}</button>
    </template>
   </DetailModal>
   <p v-if="error&&!confirmationOpen" role="alert" class="presentation-error">{{display(error)}}</p><p v-if="notice" role="status" class="presentation-notice">{{display(notice)}}</p>
  </template>
 </section>
</template>

<style scoped>
.presentation-actions { display:flex; flex-wrap:wrap; gap:12px; }
.presentation-error { color:var(--danger-text); background:var(--danger-bg); padding:12px; border-radius:var(--radius-sm); }
.presentation-notice { color:var(--primary-dark); }
.brand-presentation form { display:grid; gap:12px; min-width:0; }
</style>
