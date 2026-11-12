<script setup lang="ts">
import { formatPoints } from '@lottery/shared'

const showSummary = ref(true), showDetails = ref(true)
import {vInlineValidation} from '@lottery/shared'
const filterReset=ref(0)
import ManagementList from "@lottery/shared/ManagementList.vue"
import TablePagination from "@lottery/shared/TablePagination.vue"
import FilterAssociationSearch from "@lottery/shared/FilterAssociationSearch.vue";
const memberSearchPending = ref(false);
import SelectControl from '@lottery/shared/SelectControl.vue'
import { useOperationToast } from "@lottery/shared";
import { filterDateBoundary } from "@lottery/shared";
import { associationGroupLabel } from "@lottery/shared";
import { useAdminI18n as useAssociationI18n } from "./i18n";
const associationLocale = useAssociationI18n().locale;
import { useAdminI18n as usePublicErrorI18n } from "./i18n";
const publicErrorI18n = usePublicErrorI18n();
const publicErrorMessage = publicErrorI18n.errorMessage;
const displayPublicError = publicErrorI18n.displayError;
import { withGlobalLoading } from '@lottery/shared';
import PlatformTemporalInput from "@lottery/shared/PlatformTemporalInput.vue";
import { vGlobalLoading } from "@lottery/shared";
const displayLocale = useAdminI18n().locale;
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { adminKey, useAdminI18n } from "./i18n";
import { createWithdrawalReportApi, withdrawalReportPermissions, type WithdrawalGroup, type WithdrawalReport, type WithdrawalReportQuery } from "./withdrawal-report-api";
const pageSize = ref(20)


const props=defineProps<{account:AdminAccount;brandId:string}>();
const emit=defineEmits<{(event:"session-invalid"):void}>();
const api=createWithdrawalReportApi();
const {t,locale}=useAdminI18n();
const permissions=computed(()=>withdrawalReportPermissions(props.account,props.brandId));
const toLocal=(d:Date)=>{const p=(n:number)=>String(n).padStart(2,"0");return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`};
const roundedNow=new Date(Math.ceil(Date.now()/1000)*1000);const toDraft=ref(toLocal(roundedNow).slice(0,10));const start=new Date(roundedNow);start.setDate(start.getDate()-6);const fromDraft=ref(toLocal(start).slice(0,10));
const memberDraft=ref("");const group=ref<WithdrawalGroup>("day");const result=ref<WithdrawalReport|null>(null);const error = ref<string | import("@lottery/shared").LocalizedMessage>("");const busy=ref(false);
const offset=ref(0);
let ticket=0;let alive=true;
const scope=computed(()=>JSON.stringify({brand:props.brandId,id:props.account.id,super:props.account.super_admin,brands:[...(props.account.brand_ids??[])].sort(),permissions:[...(props.account.permissions??[])].sort(),platform:[...(props.account.platform_permissions??[])].sort(),byBrand:[...(props.account.permissions_by_brand?.[props.brandId]??[])].sort(),allowed:permissions.value}));
function clear(){result.value=null;error.value="";busy.value=false;ticket++;}
async function queryRange(){const from=filterDateBoundary(fromDraft.value,false,'local'),to=filterDateBoundary(toDraft.value,true,'local');if(!Number.isFinite(from.getTime())||!Number.isFinite(to.getTime())||to<=from||to.getTime()-from.getTime()>93*86400000)throw new Error(t('admin.copy.bddf70ec'));const username=memberDraft.value.trim();const member=username?{id:username}:null;return {from:from.toISOString(),to:to.toISOString(),group_by:group.value,...(member?{member_id:member.id}:{})};}
async function run(nextOffset=0,base?:WithdrawalReport["query"]){const local=scope.value;const id=++ticket;error.value="";if(!permissions.value.view){error.value=t('admin.copy.997ceebe');return;}let query:WithdrawalReportQuery;try{query=base?{from:base.from,to:base.to,group_by:base.group_by,limit:pageSize.value,offset:nextOffset,...(base.member_id?{member_id:base.member_id}:{})}:{...(await queryRange()),limit: pageSize.value,offset:nextOffset}}catch(e){error.value=e instanceof Error?publicErrorMessage(e, 'read'):t('admin.copy.c9f30e1d');return;}busy.value=true;try{const data=await api.report(props.brandId,query);if(alive&&ticket===id&&scope.value===local){result.value=data;offset.value=nextOffset;}}catch(e){if(alive&&ticket===id&&scope.value===local){result.value=null;error.value=e instanceof Error?publicErrorMessage(e, 'read'):t('admin.copy.96d475f6');if(e instanceof AdminApiError&&e.status===401)emit("session-invalid");}}finally{if(ticket===id)busy.value=false;}}
const metrics=(v:WithdrawalReport["summary"])=>[[t('admin.copy.79efface'),v.order_count],[t('admin.copy.0aaf1409'),v.requested_points],[t('admin.copy.fa3883bb'),v.reviewing_count],[t('admin.copy.0dac5b25'),v.reviewing_points],[t('admin.copy.2492a9a3'),v.processing_count],[t('admin.copy.4a6bf99f'),v.processing_points],[t('admin.copy.108f4d31'),v.paid_count],[t('admin.copy.443866cd'),v.paid_points],[t('admin.copy.00c6b9f6'),v.rejected_count],[t('admin.copy.d082eb84'),v.rejected_points],[t('admin.copy.1feabec0'),v.failed_count],[t('admin.copy.3292260a'),v.failed_points],[t('admin.copy.a454aa98'),v.cancelled_count],[t('admin.copy.03befd3c'),v.cancelled_points]] as const;
function exact(value: string) { return formatPoints(value, locale.value); }
function hasNext(value:WithdrawalReport){try{return BigInt(value.query.offset)<1_000_000n&&BigInt(value.query.offset)+BigInt(value.items.length)<BigInt(value.total_groups)}catch{return false}}
 function groupLabel(key:string){if(result.value?.query.group_by!=="state")return key;const labels:Record<string, string>={reviewing:'admin.copy.fa3883bb',processing:'admin.copy.2492a9a3',paid:'admin.copy.108f4d31',rejected:'admin.copy.00c6b9f6',failed:'admin.copy.1feabec0',cancelled:'admin.copy.a454aa98'};const pair=labels[key];return pair?t(adminKey(pair)):key}
watch(scope,()=>{clear();offset.value=0;if(!permissions.value.view)error.value=t('admin.copy.997ceebe');},{flush:"sync"});
onMounted(() => { if (permissions.value.view) void withGlobalLoading(run); });
onBeforeUnmount(()=>{alive=false;ticket++;});

useOperationToast(() => error.value, value => displayPublicError(value), 'error');

</script>

<template>
  <ManagementList class="withdrawal-report report-view" :aria-label="t('admin.copy.90a6f04b')">
    <template #filters><form v-inline-validation novalidate @submit.prevent="withGlobalLoading(run)" class="filters report-filters ui-filter-bar"><label><PlatformTemporalInput :label="t('filter.fromDate')" :placeholder="t('filter.fromDate')" v-model="fromDraft" type="date" :locale="displayLocale" /></label><label><PlatformTemporalInput :label="t('filter.toDate')" :placeholder="t('filter.toDate')" v-model="toDraft" type="date" :locale="displayLocale" /></label><label><FilterAssociationSearch :reset-key="filterReset" v-model="memberDraft" :brand-id="props.brandId" surface="admin" :enabled="permissions.view" resource="report_withdrawal"  v-model:pending="memberSearchPending" :label="t('filter.member')" :locale="displayLocale" :scope-key="JSON.stringify(props.account)" /></label><label><SelectControl :placeholder="t('admin.copy.6cc02c91')" v-model="group" :label="t('admin.copy.6cc02c91')" :options="[{ value: 'day', label: (String(t('admin.copy.f91541ed'))).trim() }, { value: 'member', label: (String(t('admin.copy.5073d20c'))).trim() }, { value: 'state', label: (String(t('admin.copy.a42f4620'))).trim() }]" /></label><label class="report-display-option"><input v-model="showSummary" type="checkbox" />{{ t('admin.reports.showSummary') }}</label><label class="report-display-option"><input v-model="showDetails" type="checkbox" />{{ t('admin.reports.showDetails') }}</label><button class="button button-primary" type="submit" :disabled="memberSearchPending || busy">{{t('admin.copy.2bd50b3e')}}</button>
    </form></template>

    <div v-if="showSummary || showDetails" class="report-results">

    <template v-if="result">
      <dl v-if="showSummary" class="metrics report-metrics report-summary-tags"><div v-for="m in metrics(result.summary)" :key="m[0]"><dt><span>{{m[0]}}</span></dt><dd><strong>{{exact(m[1])}}</strong></dd></div></dl>
      <div v-if="showDetails" class="table-wrap"><table class="console-table console-responsive-table"><thead><tr><th>{{t('admin.copy.b96f43c6')}}</th><th v-for="m in metrics(result.summary)" :key="m[0]">{{m[0]}}</th></tr></thead><tbody><tr v-for="item in result.items" :key="item.key"><td :data-label="String(t('admin.copy.b96f43c6'))">{{groupLabel(associationGroupLabel(item,result.query.group_by,item.key,item.label,associationLocale))}}</td><td :data-label="String(m[0])" v-for="m in metrics(item.totals)" :key="m[0]">{{exact(m[1])}}</td></tr><tr v-if="!result.items.length"><td :colspan="metrics(result.summary).length + 1" class="empty-state">{{ t('admin.copy.24e33023') }}</td></tr></tbody></table></div>

    </template>
    <p v-global-loading v-else-if="busy" class="definition" aria-live="polite">{{t('admin.copy.27342bf3')}}</p>
    </div>
  <template #pagination v-if="showDetails && (result)"><TablePagination class="pages" :total="result.total_groups" :page="Math.floor(result.query.offset / pageSize + 1)" :count="result.items.length" :locale="associationLocale" :page-size="pageSize" :previous-disabled="busy||result.query.offset===0" :next-disabled="busy||!hasNext(result)" :previous-label="t('admin.copy.dbe7a5d7')" :next-label="t('admin.copy.2e9c9e7a')" :go-to="(target: number) => run((target - 1) * pageSize, result!.query)" :change-page-size="async (size: number) => { pageSize = size; await run(0, result!.query) }" :busy="busy" /></template></ManagementList>
</template>

<style scoped>
.withdrawal-report header,.actions{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}.withdrawal-report h3,.withdrawal-report header p{margin:0}.withdrawal-report header p,.definition,.context,.actions span{color:var(--muted);font-size:var(--body-font-size);line-height:1.55}.filters{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}.filters label{display:grid;gap:5px;min-width:0;font-size:var(--body-font-size)}.filters input{box-sizing:border-box;width:100%;min-width:0;padding:9px;border:1px solid var(--line);border-radius:8px;font:inherit}.withdrawal-report button:disabled{opacity:.5}.context{display:flex;flex-wrap:wrap;gap:4px 14px;overflow-wrap:anywhere}.metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:8px}.metrics div{display:grid;gap:5px;min-width:0;border:1px solid var(--line);border-radius:8px;padding:10px}.metrics span{color:var(--muted);font-size:var(--body-font-size)}.metrics strong{overflow-wrap:anywhere;font-variant-numeric:tabular-nums}.table-wrap{overflow:auto;border:1px solid var(--line);border-radius:8px}table{border-collapse:collapse;min-width:980px;width:100%;font-variant-numeric:tabular-nums}th,td{padding:8px;border-bottom:1px solid var(--line-subtle);text-align:left;white-space:nowrap}th{position:sticky;top:0;background:var(--canvas)}.error{margin:0;padding:10px;border-radius:8px;background:var(--danger-bg);color:var(--danger-text)}@media(max-width:700px){.withdrawal-report{padding:12px}.filters{grid-template-columns:minmax(0,1fr)}.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.table-wrap{max-height:60vh}.actions button{width:100%}}
</style>
