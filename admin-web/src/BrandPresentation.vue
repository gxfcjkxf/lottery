<script setup lang="ts">
import {computed,onUnmounted,ref,watch} from "vue";
import {AdminApiError,createIdempotencyKey,type AdminAccount} from "./admin-api";
import {useAdminI18n} from "./i18n";
import type {LocalizedMessage} from "@lottery/shared";
import PresentationFields from "./PresentationFields.vue";
import {brandPresentationPermissions,createBrandPresentationApi,validBrandPresentationConfig,type BrandPresentationConfig,type BrandPresentationRecord,type BrandPresentationRevision} from "./brand-presentation-api";
import {brandPresentationSessionGeneration,classifyBrandPresentationFailure,clearPendingPresentationWrite,createBrandPresentationRequestGuard,getPendingPresentationWrite,setPendingPresentationWrite,updatePendingPresentationPhase,type PendingBrandPresentationWrite} from "./brand-presentation-state";

const props=defineProps<{account:AdminAccount;brandId:string}>();
const emit=defineEmits<{(event:"session-invalid"):void;(event:"loaded",value:{accountId:string;record:BrandPresentationRecord}):void}>();
const api=createBrandPresentationApi();
const {t,message}=useAdminI18n();
const readGuard=createBrandPresentationRequestGuard(),historyGuard=createBrandPresentationRequestGuard(),writeGuard=createBrandPresentationRequestGuard();
const rights=computed(()=>brandPresentationPermissions(props.account,props.brandId));
const statusText=(value:string)=>value==="active"?t("运行中","Active"):value==="paused"?t("已暂停","Paused"):value==="disabled"?t("已停用","Disabled"):value;
const record=ref<BrandPresentationRecord|null>(null),draft=ref<BrandPresentationConfig|null>(null),reason=ref("");
const rows=ref<BrandPresentationRevision[]>([]),offset=ref(0),hasMore=ref(false);
const pending=ref<PendingBrandPresentationWrite|null>(null);
const reasonBytes=computed(()=>new TextEncoder().encode(reason.value).length);
const loading=ref(false),historyLoading=ref(false),busy=ref(false),error=ref<string|LocalizedMessage>(""),notice=ref<string|LocalizedMessage>(""),conflictRead=ref(false);
// Drafts may be Vue proxies; this bounded model consists solely of JSON data.
const clone=<T,>(v:T):T=>JSON.parse(JSON.stringify(v)) as T;
const scope=()=>({accountId:props.account.id,brandId:props.brandId});
const unknown=computed(()=>pending.value?.phase==="unknown"),conflict=computed(()=>pending.value?.phase==="conflict"),reviewing=computed(()=>pending.value?.phase==="review");
const editable=computed(()=>rights.value.write&&record.value?.status!=="disabled"&&!pending.value&&!busy.value);
let live=true;
function current(guard:ReturnType<typeof createBrandPresentationRequestGuard>,token:number,accountId:string,brandId:string,epoch:number){return live&&guard.isCurrent(token)&&epoch===brandPresentationSessionGeneration()&&accountId===props.account.id&&brandId===props.brandId}
function sessionError(cause:unknown,guard:ReturnType<typeof createBrandPresentationRequestGuard>,token:number,id:string,brand:string,epoch:number){if(cause instanceof AdminApiError&&cause.status===401&&current(guard,token,id,brand,epoch))emit("session-invalid")}
async function readLatest(){const {accountId,brandId}=scope(),epoch=brandPresentationSessionGeneration(),token=readGuard.begin();loading.value=true;
 try{const next=await api.get(brandId);if(!current(readGuard,token,accountId,brandId,epoch))return false;record.value=next;if(!pending.value)draft.value=clone(next.config);if(conflict.value)conflictRead.value=true;emit("loaded",{accountId,record:next});return true}
 catch(cause){if(current(readGuard,token,accountId,brandId,epoch)){error.value=cause instanceof Error?cause.message:message("读取展示配置失败","Failed to load presentation settings");sessionError(cause,readGuard,token,accountId,brandId,epoch)}return false}
 finally{if(current(readGuard,token,accountId,brandId,epoch))loading.value=false}
}
async function readHistory(nextOffset=offset.value){const {accountId,brandId}=scope(),epoch=brandPresentationSessionGeneration(),token=historyGuard.begin();historyLoading.value=true;
 try{const next=await api.history(brandId,20,nextOffset);if(!current(historyGuard,token,accountId,brandId,epoch))return;rows.value=next.items;offset.value=next.offset;hasMore.value=next.items.length===20}
 catch(cause){if(current(historyGuard,token,accountId,brandId,epoch)){error.value=cause instanceof Error?cause.message:message("读取展示历史失败","Failed to load presentation history");sessionError(cause,historyGuard,token,accountId,brandId,epoch)}}
 finally{if(current(historyGuard,token,accountId,brandId,epoch))historyLoading.value=false}
}
function prepare(){error.value="";notice.value="";if(!editable.value||!record.value||!draft.value)return;
 if(!validBrandPresentationConfig(draft.value)||!reason.value.trim()||new TextEncoder().encode(reason.value).length>500){error.value=message("请核对完整配置、语言和素材地址，并填写不超过500字节的原因。","Check the configuration, locales, and asset URLs, and enter a reason of no more than 500 bytes.");return}
 const intent:PendingBrandPresentationWrite={...scope(),body:{version:record.value.version,config:clone(draft.value),reason:reason.value},key:createIdempotencyKey(),phase:"review"};setPendingPresentationWrite(scope(),intent);pending.value=getPendingPresentationWrite(scope());conflictRead.value=false;
}
function cancel(){const intent=pending.value;if(busy.value||!intent||(intent.phase!=="review"&&!(intent.phase==="conflict"&&conflictRead.value)))return;clearPendingPresentationWrite(scope(),intent.key);pending.value=getPendingPresentationWrite(scope());if(record.value)draft.value=clone(record.value.config);notice.value=message("已明确放弃旧请求；请基于最新版本重新核对。","The previous request was explicitly discarded. Review the change against the latest version.")}
function retain(intent:PendingBrandPresentationWrite,phase:"unknown"|"conflict"){
 const own={accountId:intent.accountId,brandId:intent.brandId};const existing=getPendingPresentationWrite(own);
 if(!existing)setPendingPresentationWrite(own,{...intent,phase});else if(existing.key===intent.key)updatePendingPresentationPhase(own,intent.key,phase);
}
async function send(retry=false){const intent=pending.value;if(!intent||busy.value||!rights.value.write||record.value?.status==="disabled"||(retry?intent.phase!=="unknown":intent.phase!=="review"))return;
 const epoch=brandPresentationSessionGeneration(),token=writeGuard.begin();busy.value=true;error.value="";notice.value="";retain(intent,"unknown");pending.value=getPendingPresentationWrite(scope());
 try{await api.put(intent.brandId,intent.body,intent.key);if(epoch!==brandPresentationSessionGeneration())return;
  clearPendingPresentationWrite({accountId:intent.accountId,brandId:intent.brandId},intent.key);
  if(!current(writeGuard,token,intent.accountId,intent.brandId,epoch))return;pending.value=getPendingPresentationWrite(scope());notice.value=message("原回执已核验，正在独立读取最新配置。","The original receipt was verified. Reading the latest configuration independently.");
  const refreshed=await readLatest();await readHistory(0);if(current(writeGuard,token,intent.accountId,intent.brandId,epoch))notice.value=refreshed?message("回执已核验；最新展示配置已独立读取。","The receipt was verified and the latest presentation settings were loaded independently."):message("回执已核验，但最新配置读取失败；不会重复提交，请重新读取。","The receipt was verified, but the latest settings could not be loaded. The request will not be resubmitted; reload the settings.");
 }catch(cause){if(epoch!==brandPresentationSessionGeneration())return;const status=cause instanceof AdminApiError?cause.status:undefined;const kind=classifyBrandPresentationFailure(status);
  if(kind!=="definitive")retain(intent,kind);else clearPendingPresentationWrite({accountId:intent.accountId,brandId:intent.brandId},intent.key);
  if(!current(writeGuard,token,intent.accountId,intent.brandId,epoch))return;pending.value=getPendingPresentationWrite(scope());
  error.value=kind==="unknown"?message("提交结果未知。配置和原键已冻结，刷新或切换页面不会解除；请使用原键重试。","The submission result is unknown. The configuration and original key are frozen; reloading or switching pages will not unfreeze them. Retry with the original key."):kind==="conflict"?message("共享版本或状态冲突。重新读取后明确放弃旧请求，不自动换键重试。","Shared version or status conflict. Reload the settings and explicitly discard the old request; do not retry with a new key."):cause instanceof Error?cause.message:message("请求被拒绝","Request rejected");
  if(kind==="conflict"){conflictRead.value=false;await readLatest()}sessionError(cause,writeGuard,token,intent.accountId,intent.brandId,epoch);
 }finally{if(current(writeGuard,token,intent.accountId,intent.brandId,epoch))busy.value=false}
}
watch(()=>[props.account.id,props.brandId],()=>{readGuard.invalidate();historyGuard.invalidate();writeGuard.invalidate();record.value=null;rows.value=[];offset.value=0;hasMore.value=false;loading.value=false;historyLoading.value=false;busy.value=false;error.value="";notice.value="";pending.value=getPendingPresentationWrite(scope());draft.value=pending.value?clone(pending.value.body.config):null;reason.value=pending.value?.body.reason??"";conflictRead.value=false;if(rights.value.view){void readLatest();void readHistory(0)}},{immediate:true});
onUnmounted(()=>{live=false;readGuard.invalidate();historyGuard.invalidate();writeGuard.invalidate()});
</script>

<template>
 <section class="brand-presentation" aria-labelledby="brand-presentation-title">
  <header><div><small>{{t("BRAND / PRESENTATION","BRAND / PRESENTATION")}}</small><h2 id="brand-presentation-title">{{t("品牌展示配置","Brand presentation settings")}}</h2><p>{{t("平台默认 → 品牌覆盖；仅发布展示配置，不修改用户、积分、投注规则或域名。共享版本也可能被认证设置或运行状态变更推进。","Platform defaults → brand overrides. Publishes presentation settings only; it does not change users, points, betting rules, or domains. The shared version may also advance when authentication settings or operation status changes.")}}</p></div><button type="button" :disabled="loading||!rights.view" @click="readLatest">{{t("重新读取配置","Reload settings")}}</button></header>
  <p v-if="!rights.view" role="status">{{t("当前账号没有品牌展示配置查看权限。","This account cannot view brand presentation settings.")}}</p>
  <template v-else>
   <p v-if="record" class="presentation-summary">{{ record.base_name }} · {{ t("当前共享版本","Current shared version") }} {{ record.version }} · {{ statusText(record.status) }}；{{ t("生效名称","Effective name") }} {{ record.effective.display_name }}</p>
   <p v-if="loading&&!record" role="status">{{t("正在读取配置…","Loading settings…")}}</p>
   <p v-if="record?.status==='disabled'" role="status">{{t("停用品牌的展示配置只读。","Presentation settings are read-only for disabled brands.")}}</p>
   <p v-if="unknown" role="alert">{{t("提交结果未知；原正文和原键保留，不能另建请求。","The submission result is unknown. The original payload and key are retained; do not create another request.")}}</p>
   <p v-if="conflict" role="alert">{{t("旧请求已冲突；读取最新配置后可明确放弃。","The previous request conflicted. Reload the latest settings, then explicitly discard it.")}}</p>
   <form v-if="record&&draft" @submit.prevent="prepare">
    <PresentationFields v-model:config="draft" :effective="record.effective" :disabled="!editable" />
    <label class="presentation-reason"><span>{{t("操作原因","Reason for operation")}}</span><textarea v-model="reason" :aria-label="t('操作原因','Reason for operation')" rows="3" :disabled="!editable" maxlength="500"/><small>{{reasonBytes}} / 500 {{t("字节","bytes")}}</small></label>
    <button type="submit" :disabled="!editable||!reason.trim()">{{t("核对展示配置","Review presentation settings")}}</button>
   </form>
   <div v-if="pending" class="presentation-review">
    <p>{{t("冻结意图：{brandId}，共享版本 {version}；{reason}","Frozen intent: {brandId}, shared version {version}; {reason}",{brandId:pending.brandId,version:pending.body.version,reason:pending.body.reason})}}</p>
    <details><summary>{{t("查看冻结配置 JSON","View frozen configuration JSON")}}</summary><pre>{{JSON.stringify(pending.body.config,null,2)}}</pre></details>
    <div class="presentation-actions">
     <button v-if="reviewing" type="button" :disabled="busy" @click="send(false)">{{t("确认提交","Confirm submission")}}</button>
     <button v-if="unknown" type="button" :disabled="busy" @click="send(true)">{{t("使用原键重试","Retry with original key")}}</button>
     <button v-if="reviewing||(conflict&&conflictRead)" type="button" :disabled="busy" @click="cancel">{{conflict?t('放弃旧请求并返回编辑','Discard old request and return to editing'):t('取消确认','Cancel confirmation')}}</button>
    </div>
   </div>
   <p v-if="error" role="alert" class="presentation-error">{{t(error)}}</p><p v-if="notice" role="status" class="presentation-notice">{{t(notice)}}</p>
   <section class="presentation-history"><header><h3>{{t("展示配置历史","Presentation history")}}</h3><button type="button" :disabled="historyLoading" @click="readHistory(0)">{{t("刷新记录","Refresh history")}}</button></header>
    <p v-if="historyLoading" role="status">{{t("正在读取历史…","Loading history…")}}</p><p v-else-if="!rows.length">{{t("暂无发布记录。","No published revisions yet.")}}</p>
    <article v-for="row in rows" :key="row.id"><b>v{{row.version}} · {{row.effective.display_name}}</b><time :datetime="row.created_at">{{new Date(row.created_at).toLocaleString()}}</time><p>{{row.reason}}</p><small>{{t("操作人","Changed by")}} {{row.changed_by}} · {{t("审计","Audit")}} {{row.audit_log_id}}</small><details><summary>{{t("查看当时覆盖与生效配置","View the overrides and effective settings at that time")}}</summary><pre>{{JSON.stringify({config:row.config,effective:row.effective},null,2)}}</pre></details></article>
    <div class="presentation-actions"><button type="button" :disabled="offset===0||historyLoading" @click="readHistory(Math.max(0,offset-20))">{{t("较新记录","Newer")}}</button><span>{{offset+1}}–{{offset+rows.length}}</span><button type="button" :disabled="!hasMore||historyLoading||offset>=1000000" @click="readHistory(offset+20)">{{t("较旧记录","Older")}}</button></div>
   </section>
  </template>
 </section>
</template>

<style scoped>
.brand-presentation{min-width:0;margin-top:22px;padding:20px;border:1px solid var(--line,#e3e8ee);border-radius:var(--radius,14px);background:#fff;color:var(--ink,#233044)}
header{display:flex;align-items:flex-start;justify-content:space-between;gap:14px}header p{max-width:760px;color:var(--sub,#64748b);line-height:1.5}small{color:var(--sub,#64748b);overflow-wrap:anywhere}h2{margin:6px 0}button{min-height:44px;max-width:100%;padding:9px 13px;border:1px solid var(--line,#cbd4df);border-radius:8px;background:white;color:var(--primary,#34445a);font:inherit;cursor:pointer}button:disabled{opacity:.55;cursor:not-allowed}.presentation-summary{padding:12px;background:#f7f9fc;overflow-wrap:anywhere}.presentation-reason{display:grid;gap:8px;margin:18px 0;font-weight:600}textarea{box-sizing:border-box;width:100%;min-width:0;padding:10px;border:1px solid var(--line,#cbd4df);border-radius:8px;font:inherit}.presentation-review{margin:18px 0;padding:15px;background:#fafbfd;border:1px solid var(--line,#d8e1ec);border-radius:10px;overflow-wrap:anywhere}.presentation-actions{display:flex;flex-wrap:wrap;align-items:center;gap:12px;margin:12px 0}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-width:100%;font-size:12px}.presentation-error{padding:12px;background:#fff0f0;color:var(--danger,#9a2f2f)}.presentation-notice{padding:12px;background:#eef5ff;color:var(--primary,#315780)}.presentation-history{border-top:1px solid var(--line,#e7ebf0);margin-top:24px;padding-top:18px}.presentation-history article{padding:15px 0;border-bottom:1px solid var(--line,#edf0f4);overflow-wrap:anywhere}.presentation-history time{display:block;font-size:12px;color:var(--sub,#718096);margin:8px 0}details{margin:10px 0}
@media(max-width:700px){.brand-presentation{padding:14px}header{flex-direction:column}header button{width:100%}.presentation-actions button{flex:1 1 120px}}
</style>
