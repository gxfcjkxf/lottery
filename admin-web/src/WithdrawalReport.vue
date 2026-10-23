<script setup lang="ts">
import FilterAssociationSelect from "@lottery/shared/FilterAssociationSelect.vue";
import { filterDateBoundary } from "@lottery/shared";
import { associationGroupLabel } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { AssociationName } from "@lottery/shared";
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import PlatformTemporalInput from "@lottery/shared/PlatformTemporalInput.vue";
import { vGlobalLoading } from "@lottery/shared";
import { formatDisplayDateTime } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { lookupAdminMember } from "./member-lookup";
import { adminKey, useAdminI18n } from "./i18n";
import { createWithdrawalReportApi, withdrawalReportPermissions, type WithdrawalGroup, type WithdrawalReport, type WithdrawalReportQuery } from "./withdrawal-report-api";

const props=defineProps<{account:AdminAccount;brandId:string}>();
const emit=defineEmits<{(event:"session-invalid"):void}>();
const api=createWithdrawalReportApi();
const {t,locale}=useAdminI18n();
const permissions=computed(()=>withdrawalReportPermissions(props.account,props.brandId));
const toLocal=(d:Date)=>{const p=(n:number)=>String(n).padStart(2,"0");return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`};
const roundedNow=new Date(Math.ceil(Date.now()/1000)*1000);const toDraft=ref(toLocal(roundedNow).slice(0,10));const start=new Date(roundedNow);start.setDate(start.getDate()-6);const fromDraft=ref(toLocal(start).slice(0,10));
const memberDraft=ref("");const group=ref<WithdrawalGroup>("day");const result=ref<WithdrawalReport|null>(null);const error = ref<string | import("@lottery/shared").LocalizedMessage>("");const exportError = ref<string | import("@lottery/shared").LocalizedMessage>("");const busy=ref(false);const exportBusy=ref(false);
const offset=ref(0);
let ticket=0;let exportTicket=0;let alive=true;
const scope=computed(()=>JSON.stringify({brand:props.brandId,id:props.account.id,super:props.account.super_admin,brands:[...(props.account.brand_ids??[])].sort(),permissions:[...(props.account.permissions??[])].sort(),platform:[...(props.account.platform_permissions??[])].sort(),byBrand:[...(props.account.permissions_by_brand?.[props.brandId]??[])].sort(),allowed:permissions.value}));
function clear(){result.value=null;error.value="";busy.value=false;ticket++;}
async function queryRange(){const from=filterDateBoundary(fromDraft.value,false,'local'),to=filterDateBoundary(toDraft.value,true,'local');if(!Number.isFinite(from.getTime())||!Number.isFinite(to.getTime())||to<=from||to.getTime()-from.getTime()>93*86400000)throw new Error(t('admin.copy.bddf70ec'));const username=memberDraft.value.trim();const member=username?{id:username}:null;return {from:from.toISOString(),to:to.toISOString(),group_by:group.value,...(member?{member_id:member.id}:{})};}
async function run(nextOffset=0,base?:WithdrawalReport["query"]){const local=scope.value;const id=++ticket;offset.value=nextOffset;result.value=null;error.value="";exportError.value="";if(!permissions.value.view){error.value=t('admin.copy.997ceebe');return;}let query:WithdrawalReportQuery;try{query=base?{from:base.from,to:base.to,group_by:base.group_by,limit:base.limit,offset:nextOffset,...(base.member_id?{member_id:base.member_id}:{})}:{...(await queryRange()),limit:20,offset:nextOffset}}catch(e){error.value=e instanceof Error?publicErrorMessage(e, 'read'):t('admin.copy.c9f30e1d');return;}busy.value=true;try{const data=await api.report(props.brandId,query);if(alive&&ticket===id&&scope.value===local)result.value=data;}catch(e){if(alive&&ticket===id&&scope.value===local){result.value=null;error.value=e instanceof Error?publicErrorMessage(e, 'read'):t('admin.copy.96d475f6');if(e instanceof AdminApiError&&e.status===401)emit("session-invalid");}}finally{if(ticket===id)busy.value=false;}}
async function exportAll(){if(!permissions.value.export||!result.value||!alive)return;const id=++exportTicket,local=scope.value,saved=result.value;exportBusy.value=true;exportError.value="";try{const file=await api.exportCsv(props.brandId,{from:saved.query.from,to:saved.query.to,group_by:saved.query.group_by,...(saved.query.member_id?{member_id:saved.query.member_id}:{})});if(!alive||id!==exportTicket||local!==scope.value||saved!==result.value)return;const url=URL.createObjectURL(new Blob([file.bytes],{type:"text/csv;charset=utf-8"}));const a=document.createElement("a");a.href=url;a.download=file.filename;a.click();setTimeout(()=>URL.revokeObjectURL(url),0);}catch(e){if(alive&&id===exportTicket&&local===scope.value){exportError.value=e instanceof Error?publicErrorMessage(e, 'read'):"导出失败 / Export failed.";if(e instanceof AdminApiError&&e.status===401)emit("session-invalid");}}finally{if(id===exportTicket)exportBusy.value=false;}}
const metrics=(v:WithdrawalReport["summary"])=>[[t('admin.copy.79efface'),v.order_count],[t('admin.copy.0aaf1409'),v.requested_points],[t('admin.copy.fa3883bb'),v.reviewing_count],[t('admin.copy.0dac5b25'),v.reviewing_points],[t('admin.copy.2492a9a3'),v.processing_count],[t('admin.copy.4a6bf99f'),v.processing_points],[t('admin.copy.108f4d31'),v.paid_count],[t('admin.copy.443866cd'),v.paid_points],[t('admin.copy.00c6b9f6'),v.rejected_count],[t('admin.copy.d082eb84'),v.rejected_points],[t('admin.copy.1feabec0'),v.failed_count],[t('admin.copy.3292260a'),v.failed_points],[t('admin.copy.a454aa98'),v.cancelled_count],[t('admin.copy.03befd3c'),v.cancelled_points]] as const;
function exact(v:string){try{return BigInt(v).toLocaleString(locale.value==="en"?"en-US":"zh-CN")}catch{return v}}
function hasNext(value:WithdrawalReport){try{return BigInt(value.query.offset)<1_000_000n&&BigInt(value.query.offset)+BigInt(value.items.length)<BigInt(value.total_groups)}catch{return false}}
 function groupLabel(key:string){if(result.value?.query.group_by!=="state")return key;const labels:Record<string, string>={reviewing:'admin.copy.fa3883bb',processing:'admin.copy.2492a9a3',paid:'admin.copy.108f4d31',rejected:'admin.copy.00c6b9f6',failed:'admin.copy.1feabec0',cancelled:'admin.copy.a454aa98'};const pair=labels[key];return pair?t(adminKey(pair)):key}
watch(scope,()=>{clear();offset.value=0;exportTicket++;exportBusy.value=false;exportError.value="";if(!permissions.value.view)error.value=t('admin.copy.997ceebe')},{flush:"sync"});
onBeforeUnmount(()=>{alive=false;ticket++;exportTicket++;});
</script>

<template>
  <section class="withdrawal-report" aria-labelledby="withdrawal-report-title">
    <header><div><h3 id="withdrawal-report-title">{{t('admin.copy.90a6f04b')}}</h3></div></header>
    <p class="definition">{{t('admin.copy.f309994e')}}</p>
    <div class="filters"><label>{{ t('filter.fromDate') }}<PlatformTemporalInput v-model="fromDraft" type="date" :locale="displayLocale" /></label><label>{{ t('filter.toDate') }}<PlatformTemporalInput v-model="toDraft" type="date" :locale="displayLocale" /></label><label>{{ t('filter.member') }}<FilterAssociationSelect v-model="memberDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_withdrawal" kind="member" :label="t('filter.member')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label><label>{{t('admin.copy.6cc02c91')}}<select v-model="group" :aria-label="t('admin.copy.6cc02c91')"><option value="day">{{t('admin.copy.f91541ed')}}</option><option value="member">{{t('admin.copy.5073d20c')}}</option><option value="state">{{t('admin.copy.a42f4620')}}</option></select></label><button class="button button-primary" type="button" :disabled="busy" @click="withGlobalLoading(run)">{{t('admin.copy.2bd50b3e')}}</button>
    </div>
    <p v-if="props.account.super_admin" class="definition">{{t('admin.copy.4063fd9f')}}</p>
    <p v-if="error" class="error" role="alert">{{ displayPublicError(error) }}</p><p v-if="exportError" class="error" role="alert">{{ displayPublicError(exportError) }}</p>
    <template v-if="result">
      <div class="context"><span>{{t('admin.copy.9c32661d')}} <AssociationName :record="result" kind="brand" :id="result.brand_id" :locale="associationLocale" /></span><span>{{t('admin.copy.82d4e9bd')}} {{formatDisplayDateTime(result.snapshot_at, displayLocale)}}</span><span>{{t('admin.copy.c6bf9750')}} {{result.timezone}}</span><span>{{t('admin.copy.343763ae')}} [{{result.query.from}}, {{result.query.to}})</span><span>{{t('admin.copy.56b2d19b')}} <template v-if="result.query.member_id"><AssociationName :record="result.query" kind="member" :id="result.query.member_id" :locale="associationLocale" /></template><span v-else>{{ t('admin.copy.ef22c1d2') }}</span></span><span>{{t('admin.copy.a0c04ae0')}} {{exact(result.total_groups)}}</span></div>
      <div class="actions"><button type="button" :disabled="!permissions.export||exportBusy" @click="withGlobalLoading(exportAll)">{{exportBusy?t('admin.copy.1c800a70'):t('admin.copy.800c75fc')}}</button><span v-if="!permissions.export">{{t('admin.copy.fcd324d4')}}</span></div>
      <div class="metrics"><div v-for="m in metrics(result.summary)" :key="m[0]"><span>{{m[0]}}</span><strong>{{exact(m[1])}}</strong></div></div>
      <div v-if="result.items.length" class="table-wrap"><table><thead><tr><th>{{t('admin.copy.b96f43c6')}}</th><th v-for="m in metrics(result.items[0]!.totals)" :key="m[0]">{{m[0]}}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><td>{{groupLabel(associationGroupLabel(item,result.query.group_by,item.key,item.label,associationLocale))}}</td><td v-for="m in metrics(item.totals)" :key="m[0]">{{exact(m[1])}}</td></tr></tbody></table></div>
      <p v-else class="definition empty-state">{{t('admin.copy.898225da')}}</p>
      <nav class="pages"><span>{{result.query.offset+1}}–{{result.query.offset+result.items.length}} / {{exact(result.total_groups)}}</span><button type="button" :disabled="busy||result.query.offset===0" @click="withGlobalLoading(run, Math.max(0,result.query.offset-20), result.query)">{{t('admin.copy.dbe7a5d7')}}</button><button type="button" :disabled="busy||!hasNext(result)" @click="withGlobalLoading(run, result.query.offset+20, result.query)">{{t('admin.copy.2e9c9e7a')}}</button></nav>
    </template>
    <p v-global-loading v-else-if="busy" class="definition" aria-live="polite">{{t('admin.copy.27342bf3')}}</p>
  </section>
</template>

<style scoped>
.withdrawal-report{display:grid;gap:12px;min-width:0;border:1px solid var(--line);border-radius:12px;padding:16px;background:var(--surface);color:var(--ink)}.withdrawal-report header,.actions,.pages{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}.withdrawal-report h3,.withdrawal-report header p{margin:0}.withdrawal-report header p,.definition,.context,.actions span{color:var(--muted);font-size:.86rem;line-height:1.55}.filters{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}.filters label{display:grid;gap:5px;min-width:0;font-size:.86rem}.filters input,.filters select{box-sizing:border-box;width:100%;min-width:0;padding:9px;border:1px solid var(--line);border-radius:8px;font:inherit}.withdrawal-report button{min-height:40px;padding:8px 12px;border:1px solid #c5ceda;border-radius:8px;background:var(--surface);color:inherit;font:inherit}.withdrawal-report button:disabled{opacity:.5}.context{display:flex;flex-wrap:wrap;gap:4px 14px;overflow-wrap:anywhere}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px}.metrics div{display:grid;gap:5px;min-width:0;border:1px solid #e5eaf0;border-radius:8px;padding:10px}.metrics span{color:var(--muted);font-size:.8rem}.metrics strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.table-wrap{overflow:auto;border:1px solid #e2e7ef;border-radius:8px}table{border-collapse:collapse;min-width:980px;width:100%;font-variant-numeric:tabular-nums}th,td{padding:8px;border-bottom:1px solid var(--line-subtle);text-align:left;white-space:nowrap}th{position:sticky;top:0;background:var(--canvas)}.error{margin:0;padding:10px;border-radius:8px;background:#fff0ee;color:#a32e1a}@media(max-width:700px){.withdrawal-report{padding:12px}.filters{grid-template-columns:minmax(0,1fr)}.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.table-wrap{max-height:60vh}.actions button{width:100%}}
</style>
