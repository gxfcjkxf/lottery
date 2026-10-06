<script setup lang="ts">
import {computed,onUnmounted,ref,watch} from "vue";
import {AdminApiError,createIdempotencyKey,type AdminAccount} from "./admin-api";
import PresentationFields from "./PresentationFields.vue";
import {brandPresentationPermissions,createBrandPresentationApi,validBrandPresentationConfig,type BrandPresentationConfig,type BrandPresentationRecord,type BrandPresentationRevision} from "./brand-presentation-api";
import {brandPresentationSessionGeneration,classifyBrandPresentationFailure,clearPendingPresentationWrite,createBrandPresentationRequestGuard,getPendingPresentationWrite,setPendingPresentationWrite,updatePendingPresentationPhase,type PendingBrandPresentationWrite} from "./brand-presentation-state";

const props=defineProps<{account:AdminAccount;brandId:string}>();
const emit=defineEmits<{(event:"session-invalid"):void;(event:"loaded",value:{accountId:string;record:BrandPresentationRecord}):void}>();
const api=createBrandPresentationApi();
const readGuard=createBrandPresentationRequestGuard(),historyGuard=createBrandPresentationRequestGuard(),writeGuard=createBrandPresentationRequestGuard();
const rights=computed(()=>brandPresentationPermissions(props.account,props.brandId));
const record=ref<BrandPresentationRecord|null>(null),draft=ref<BrandPresentationConfig|null>(null),reason=ref("");
const rows=ref<BrandPresentationRevision[]>([]),offset=ref(0),hasMore=ref(false);
const pending=ref<PendingBrandPresentationWrite|null>(null);
const reasonBytes=computed(()=>new TextEncoder().encode(reason.value).length);
const loading=ref(false),historyLoading=ref(false),busy=ref(false),error=ref(""),notice=ref(""),conflictRead=ref(false);
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
 catch(cause){if(current(readGuard,token,accountId,brandId,epoch)){error.value=cause instanceof Error?cause.message:"读取展示配置失败";sessionError(cause,readGuard,token,accountId,brandId,epoch)}return false}
 finally{if(current(readGuard,token,accountId,brandId,epoch))loading.value=false}
}
async function readHistory(nextOffset=offset.value){const {accountId,brandId}=scope(),epoch=brandPresentationSessionGeneration(),token=historyGuard.begin();historyLoading.value=true;
 try{const next=await api.history(brandId,20,nextOffset);if(!current(historyGuard,token,accountId,brandId,epoch))return;rows.value=next.items;offset.value=next.offset;hasMore.value=next.items.length===20}
 catch(cause){if(current(historyGuard,token,accountId,brandId,epoch)){error.value=cause instanceof Error?cause.message:"读取展示历史失败";sessionError(cause,historyGuard,token,accountId,brandId,epoch)}}
 finally{if(current(historyGuard,token,accountId,brandId,epoch))historyLoading.value=false}
}
function prepare(){error.value="";notice.value="";if(!editable.value||!record.value||!draft.value)return;
 if(!validBrandPresentationConfig(draft.value)||!reason.value.trim()||new TextEncoder().encode(reason.value).length>500){error.value="请核对完整配置、语言和素材地址，并填写不超过500字节的原因。";return}
 const intent:PendingBrandPresentationWrite={...scope(),body:{version:record.value.version,config:clone(draft.value),reason:reason.value},key:createIdempotencyKey(),phase:"review"};setPendingPresentationWrite(scope(),intent);pending.value=getPendingPresentationWrite(scope());conflictRead.value=false;
}
function cancel(){const intent=pending.value;if(busy.value||!intent||(intent.phase!=="review"&&!(intent.phase==="conflict"&&conflictRead.value)))return;clearPendingPresentationWrite(scope(),intent.key);pending.value=getPendingPresentationWrite(scope());if(record.value)draft.value=clone(record.value.config);notice.value="已明确放弃旧请求；请基于最新版本重新核对。"}
function retain(intent:PendingBrandPresentationWrite,phase:"unknown"|"conflict"){
 const own={accountId:intent.accountId,brandId:intent.brandId};const existing=getPendingPresentationWrite(own);
 if(!existing)setPendingPresentationWrite(own,{...intent,phase});else if(existing.key===intent.key)updatePendingPresentationPhase(own,intent.key,phase);
}
async function send(retry=false){const intent=pending.value;if(!intent||busy.value||!rights.value.write||record.value?.status==="disabled"||(retry?intent.phase!=="unknown":intent.phase!=="review"))return;
 const epoch=brandPresentationSessionGeneration(),token=writeGuard.begin();busy.value=true;error.value="";notice.value="";retain(intent,"unknown");pending.value=getPendingPresentationWrite(scope());
 try{await api.put(intent.brandId,intent.body,intent.key);if(epoch!==brandPresentationSessionGeneration())return;
  clearPendingPresentationWrite({accountId:intent.accountId,brandId:intent.brandId},intent.key);
  if(!current(writeGuard,token,intent.accountId,intent.brandId,epoch))return;pending.value=getPendingPresentationWrite(scope());notice.value="原回执已核验，正在独立读取最新配置。";
  const refreshed=await readLatest();await readHistory(0);if(current(writeGuard,token,intent.accountId,intent.brandId,epoch))notice.value=refreshed?"回执已核验；最新展示配置已独立读取。":"回执已核验，但最新配置读取失败；不会重复提交，请重新读取。";
 }catch(cause){if(epoch!==brandPresentationSessionGeneration())return;const status=cause instanceof AdminApiError?cause.status:undefined;const kind=classifyBrandPresentationFailure(status);
  if(kind!=="definitive")retain(intent,kind);else clearPendingPresentationWrite({accountId:intent.accountId,brandId:intent.brandId},intent.key);
  if(!current(writeGuard,token,intent.accountId,intent.brandId,epoch))return;pending.value=getPendingPresentationWrite(scope());
  error.value=kind==="unknown"?"提交结果未知。配置和原键已冻结，刷新或切换页面不会解除；请使用原键重试。":kind==="conflict"?"共享版本或状态冲突。重新读取后明确放弃旧请求，不自动换键重试。":cause instanceof Error?cause.message:"请求被拒绝";
  if(kind==="conflict"){conflictRead.value=false;await readLatest()}sessionError(cause,writeGuard,token,intent.accountId,intent.brandId,epoch);
 }finally{if(current(writeGuard,token,intent.accountId,intent.brandId,epoch))busy.value=false}
}
watch(()=>[props.account.id,props.brandId],()=>{readGuard.invalidate();historyGuard.invalidate();writeGuard.invalidate();record.value=null;rows.value=[];offset.value=0;hasMore.value=false;loading.value=false;historyLoading.value=false;busy.value=false;error.value="";notice.value="";pending.value=getPendingPresentationWrite(scope());draft.value=pending.value?clone(pending.value.body.config):null;reason.value=pending.value?.body.reason??"";conflictRead.value=false;if(rights.value.view){void readLatest();void readHistory(0)}},{immediate:true});
onUnmounted(()=>{live=false;readGuard.invalidate();historyGuard.invalidate();writeGuard.invalidate()});
</script>

<template>
 <section class="brand-presentation" aria-labelledby="brand-presentation-title">
  <header><div><small>BRAND / PRESENTATION</small><h2 id="brand-presentation-title">品牌展示配置</h2><p>平台默认 → 品牌覆盖；仅发布展示配置，不修改用户、积分、投注规则或域名。共享版本也可能被认证设置或运行状态变更推进。</p></div><button type="button" :disabled="loading||!rights.view" @click="readLatest">重新读取配置</button></header>
  <p v-if="!rights.view" role="status">当前账号没有品牌展示配置查看权限。</p>
  <template v-else>
   <p v-if="record" class="presentation-summary">{{ record.base_name }} · 当前共享版本 {{ record.version }} · {{ record.status }}；生效名称 {{ record.effective.display_name }}</p>
   <p v-if="loading&&!record" role="status">正在读取配置…</p>
   <p v-if="record?.status==='disabled'" role="status">停用品牌的展示配置只读。</p>
   <p v-if="unknown" role="alert">提交结果未知；原正文和原键保留，不能另建请求。</p>
   <p v-if="conflict" role="alert">旧请求已冲突；读取最新配置后可明确放弃。</p>
   <form v-if="record&&draft" @submit.prevent="prepare">
    <PresentationFields v-model:config="draft" :effective="record.effective" :disabled="!editable" />
    <label class="presentation-reason"><span>操作原因</span><textarea v-model="reason" aria-label="操作原因" rows="3" :disabled="!editable" maxlength="500"/><small>{{reasonBytes}} / 500 字节</small></label>
    <button type="submit" :disabled="!editable||!reason.trim()">核对展示配置</button>
   </form>
   <div v-if="pending" class="presentation-review">
    <p>冻结意图：{{pending.brandId}}，共享版本 {{pending.body.version}}；{{pending.body.reason}}</p>
    <details><summary>查看冻结配置 JSON</summary><pre>{{JSON.stringify(pending.body.config,null,2)}}</pre></details>
    <div class="presentation-actions">
     <button v-if="reviewing" type="button" :disabled="busy" @click="send(false)">确认提交</button>
     <button v-if="unknown" type="button" :disabled="busy" @click="send(true)">使用原键重试</button>
     <button v-if="reviewing||(conflict&&conflictRead)" type="button" :disabled="busy" @click="cancel">{{conflict?'放弃旧请求并返回编辑':'取消确认'}}</button>
    </div>
   </div>
   <p v-if="error" role="alert" class="presentation-error">{{error}}</p><p v-if="notice" role="status" class="presentation-notice">{{notice}}</p>
   <section class="presentation-history"><header><h3>展示配置历史</h3><button type="button" :disabled="historyLoading" @click="readHistory(0)">刷新记录</button></header>
    <p v-if="historyLoading" role="status">正在读取历史…</p><p v-else-if="!rows.length">暂无发布记录。</p>
    <article v-for="row in rows" :key="row.id"><b>v{{row.version}} · {{row.effective.display_name}}</b><time :datetime="row.created_at">{{new Date(row.created_at).toLocaleString()}}</time><p>{{row.reason}}</p><small>操作人 {{row.changed_by}} · 审计 {{row.audit_log_id}}</small><details><summary>查看当时覆盖与生效配置</summary><pre>{{JSON.stringify({config:row.config,effective:row.effective},null,2)}}</pre></details></article>
    <div class="presentation-actions"><button type="button" :disabled="offset===0||historyLoading" @click="readHistory(Math.max(0,offset-20))">较新记录</button><span>{{offset+1}}–{{offset+rows.length}}</span><button type="button" :disabled="!hasMore||historyLoading||offset>=1000000" @click="readHistory(offset+20)">较旧记录</button></div>
   </section>
  </template>
 </section>
</template>

<style scoped>
.brand-presentation{min-width:0;margin-top:22px;padding:20px;border:1px solid var(--line,#e3e8ee);border-radius:var(--radius,14px);background:#fff;color:var(--ink,#233044)}
header{display:flex;align-items:flex-start;justify-content:space-between;gap:14px}header p{max-width:760px;color:var(--sub,#64748b);line-height:1.5}small{color:var(--sub,#64748b);overflow-wrap:anywhere}h2{margin:6px 0}button{min-height:44px;max-width:100%;padding:9px 13px;border:1px solid var(--line,#cbd4df);border-radius:8px;background:white;color:var(--primary,#34445a);font:inherit;cursor:pointer}button:disabled{opacity:.55;cursor:not-allowed}.presentation-summary{padding:12px;background:#f7f9fc;overflow-wrap:anywhere}.presentation-reason{display:grid;gap:8px;margin:18px 0;font-weight:600}textarea{box-sizing:border-box;width:100%;min-width:0;padding:10px;border:1px solid var(--line,#cbd4df);border-radius:8px;font:inherit}.presentation-review{margin:18px 0;padding:15px;background:#fafbfd;border:1px solid var(--line,#d8e1ec);border-radius:10px;overflow-wrap:anywhere}.presentation-actions{display:flex;flex-wrap:wrap;align-items:center;gap:12px;margin:12px 0}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-width:100%;font-size:12px}.presentation-error{padding:12px;background:#fff0f0;color:var(--danger,#9a2f2f)}.presentation-notice{padding:12px;background:#eef5ff;color:var(--primary,#315780)}.presentation-history{border-top:1px solid var(--line,#e7ebf0);margin-top:24px;padding-top:18px}.presentation-history article{padding:15px 0;border-bottom:1px solid var(--line,#edf0f4);overflow-wrap:anywhere}.presentation-history time{display:block;font-size:12px;color:var(--sub,#718096);margin:8px 0}details{margin:10px 0}
@media(max-width:700px){.brand-presentation{padding:14px}header{flex-direction:column}header button{width:100%}.presentation-actions button{flex:1 1 120px}}
</style>
