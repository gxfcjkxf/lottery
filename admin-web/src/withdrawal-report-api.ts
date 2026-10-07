import { AdminApiError, type AdminAccount } from "./admin-api";

const BASE = "/api/v1/admin/reports/withdrawal";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const INTEGER = /^(0|[1-9]\d{0,127})$/;
const DATETIME = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/;
const STATES = ["reviewing", "processing", "paid", "rejected", "failed", "cancelled"] as const;
export type WithdrawalState = typeof STATES[number];
export type WithdrawalGroup = "day" | "member" | "state";
export interface WithdrawalReportQuery { from: string; to: string; group_by: WithdrawalGroup; limit?: number; offset?: number; member_id?: string }
export interface WithdrawalTotals {
  order_count:string; requested_points:string;
  reviewing_count:string; reviewing_points:string; processing_count:string; processing_points:string;
  paid_count:string; paid_points:string; rejected_count:string; rejected_points:string;
  failed_count:string; failed_points:string; cancelled_count:string; cancelled_points:string;
}
export interface WithdrawalReport {
  brand_id:string; snapshot_at:string; timezone:string;
  query:{from:string;to:string;group_by:WithdrawalGroup;limit:number;offset:number;game_id:null;member_id:string|null};
  summary:WithdrawalTotals; items:{key:string;label:string;totals:WithdrawalTotals}[]; total_groups:string;
}
const fields = ["order_count","requested_points","reviewing_count","reviewing_points","processing_count","processing_points","paid_count","paid_points","rejected_count","rejected_points","failed_count","failed_points","cancelled_count","cancelled_points"] as const;
const isObject=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==="object"&&!Array.isArray(v);
const exactKeys=(v:Record<string,unknown>,keys:readonly string[])=>Object.keys(v).length===keys.length&&Object.keys(v).every(k=>keys.includes(k));
const invalid=(msg="提现报表响应格式无效。"):never=>{throw new AdminApiError(msg,502,"INVALID_RESPONSE")};
const bad=(msg="提现报表筛选无效。"):never=>{throw new AdminApiError(msg,0,"INVALID_INPUT")};
function totals(v:unknown):WithdrawalTotals {
  if(!isObject(v)||Object.keys(v).length!==fields.length||fields.some(k=>typeof v[k]!=="string"||!INTEGER.test(v[k] as string))) return invalid();
  const t=v as unknown as WithdrawalTotals;
  const count=STATES.reduce((n,s)=>n+BigInt(t[`${s}_count` as keyof WithdrawalTotals]),0n);
  const points=STATES.reduce((n,s)=>n+BigInt(t[`${s}_points` as keyof WithdrawalTotals]),0n);
  if(count!==BigInt(t.order_count)||points!==BigInt(t.requested_points)) return invalid("提现状态分项与总计不一致。");
  return t;
}
function epochNs(s:string):bigint {
  const m=DATETIME.exec(s);if(!m)return bad("from 和 to 必须是有效 RFC3339 时间。");
  const [,ys,mos,ds,hs,mis,ss,fraction="",zone,,zh="0",zm="0"]=m;
  const y=Number(ys),mo=Number(mos),d=Number(ds),h=Number(hs),mi=Number(mis),sec=Number(ss),days=mo===2?(y%4===0&&(y%100!==0||y%400===0)?29:28):([4,6,9,11].includes(mo)?30:31);
  if(y<1||mo<1||mo>12||d<1||d>days||h>23||mi>59||sec>59||zone!=="Z"&&(Number(zh)>14||Number(zm)>59||Number(zh)===14&&Number(zm)!==0))return bad("from 和 to 必须是有效 RFC3339 时间。");
  const millis=Date.parse(s.replace(/\.\d+(?=Z|[+-]\d{2}:\d{2}$)/,""));if(!Number.isFinite(millis))return bad();
  return BigInt(millis)*1_000_000n+BigInt(fraction.padEnd(9,"0")||"0");
}
function canonicalUtc(ns:bigint):string {
  const sec=ns>=0n?ns/1_000_000_000n:(ns-999_999_999n)/1_000_000_000n,rem=ns-sec*1_000_000_000n;
  const base=new Date(Number(sec*1_000n)).toISOString().slice(0,19);
  const fraction=rem.toString().padStart(9,"0").replace(/0+$/g,"");
  return `${base}${fraction?`.${fraction}`:""}Z`;
}
function canonical(q:WithdrawalReportQuery) {
  if(!q)bad("起止时间必须为有效日期。");
  if(Object.keys(q).some(k=>!["from","to","group_by","limit","offset","member_id"].includes(k))) bad("提现报表不支持此筛选参数。");
  const start=epochNs(q.from),end=epochNs(q.to);
  if(end<=start||end-start>93n*24n*60n*60n*1_000_000_000n) bad("时间范围必须大于零且不超过93天。");
  if(!["day","member","state"].includes(q.group_by)) bad("提现报表分组无效。");
  if(q.member_id!==undefined&&!UUID.test(q.member_id)) bad("会员 UUID 无效。");
  const limit=q.limit??20,offset=q.offset??0;
  if(!Number.isSafeInteger(limit)||limit<1||limit>100||!Number.isSafeInteger(offset)||offset<0||offset>1000000) bad("分页参数无效。");
  return {from:canonicalUtc(start),to:canonicalUtc(end),group_by:q.group_by,limit,offset,game_id:null,member_id:q.member_id??null};
}
export function withdrawalReportPermissions(account:AdminAccount,brand:string) {
  const valid=UUID.test(account.id)&&UUID.test(brand);
  const inBrand=(account.brand_ids??[]).some(id=>id.toLowerCase()===brand.toLowerCase());
  const grants=new Set(account.permissions_by_brand?.[brand]??[]),platform=new Set(account.platform_permissions??[]);
  const view=valid&&(platform.has("report_withdrawal.view.platform")||inBrand&&grants.has("report_withdrawal.view.brand"));
  const exportAllowed=view&&(platform.has("report_withdrawal.export.platform")||inBrand&&grants.has("report_withdrawal.export.brand"));
  return {view,export:exportAllowed};
}
type FetchLike=typeof fetch;
function validCsv(text:string,brand:string,query:ReturnType<typeof canonical>,snapshot:string,groupCount:string):boolean {
  try {
    const columns=["record_type","brand_id","snapshot_at","timezone","from","to","group_by","member_id","key","label",...fields];
    const rows=text.slice(1).split(/\r?\n/);if(rows.at(-1)==="")rows.pop();
    if(BigInt(rows.length)!==BigInt(groupCount)+2n||rows[0]!==columns.join(","))return false;
    const data=rows.slice(1).map(line=>line.split(","));if(data.some(row=>row.length!==columns.length||row.some(cell=>cell.includes('"'))))return false;
    const member=query.member_id??"",summary=data[0]!;
    const validMeta=(row:string[])=>row[1]===brand&&row[2]===snapshot&&row[4]===query.from&&row[5]===query.to&&row[6]===query.group_by&&row[7]===member&&!!row[3]&&(()=>{try{new Intl.DateTimeFormat("en",{timeZone:row[3]});return true}catch{return false}})();
    if(summary[0]!=="summary"||!validMeta(summary)||summary[8]!==""||summary[9]!=="")return false;
    const amount=(row:string[])=>{const values=row.slice(10);if(values.length!==fields.length||values.some(v=>!INTEGER.test(v)))return null;const counts=STATES.reduce((n,s)=>n+BigInt(values[fields.indexOf(`${s}_count` as typeof fields[number])!]),0n),points=STATES.reduce((n,s)=>n+BigInt(values[fields.indexOf(`${s}_points` as typeof fields[number])!]),0n);return counts===BigInt(values[0]!)&&points===BigInt(values[1]!)?values.map(BigInt):null};
    const summaryValues=amount(summary);if(!summaryValues)return false;
    const sums=fields.map(()=>0n);let last="";
    for(const row of data.slice(1)){
      if(row[0]!=="group"||!validMeta(row)||!row[8]||row[8]!==row[9]||row[8]<=last)return false;
      last=row[8]!;
      if(query.group_by==="day"&&!/^\d{4}-\d{2}-\d{2}$/.test(last)||query.group_by==="member"&&!UUID.test(last)||query.group_by==="state"&&!STATES.includes(last as WithdrawalState))return false;
      const values=amount(row);if(!values)return false;values.forEach((v,i)=>sums[i]+=v);
    }
    return summaryValues.every((v,i)=>v===sums[i]);
  }catch{return false}
}
export function createWithdrawalReportApi(fetcher:FetchLike=fetch) {
  async function get(brand:string,q:WithdrawalReportQuery,exporting=false):Promise<WithdrawalReport> {
    if(!UUID.test(brand)) bad("品牌 UUID 无效。");
    const requested=canonical(q);
    if(exporting&&(q.limit!==undefined||q.offset!==undefined)) bad("CSV 导出不接受分页参数。");
    const params=new URLSearchParams();
    for(const [k,v] of Object.entries(requested)) if(v!==null) params.set(k,String(v));
    const response=await fetcher(`${BASE}${exporting?"/export":""}?${params}`,{method:"GET",credentials:"same-origin",headers:{"X-Brand-ID":brand,Accept:"application/json"}});
    let body:unknown; try{body=await response.json()}catch{body=null}
    if(!response.ok) { const error=isObject(body)&&isObject(body.error)?body.error:{}; throw new AdminApiError(typeof error.message==="string"?error.message:`Request failed (${response.status})`,response.status,typeof error.code==="string"?error.code:undefined); }
    if(!isObject(body)||!exactKeys(body,["success","data","request_id"])||body.success!==true||typeof body.request_id!=="string"||!/^[a-zA-Z0-9_.:-]{1,80}$/.test(body.request_id)||!isObject(body.data)) return invalid();
    const d=body.data;
    if(!exactKeys(d,["brand_id","snapshot_at","timezone","query","summary","items","total_groups"])||d.brand_id!==brand||typeof d.snapshot_at!=="string"||!Number.isFinite(Date.parse(d.snapshot_at))||typeof d.timezone!=="string"||!d.timezone||!isObject(d.query)||!Array.isArray(d.items)||typeof d.total_groups!=="string"||!INTEGER.test(d.total_groups)) return invalid();
    try{new Intl.DateTimeFormat("en",{timeZone:d.timezone})}catch{return invalid("提现报表时区无效。")}
    const echo=d.query;
    if(!exactKeys(echo,["from","to","group_by","limit","offset","game_id","member_id"]))return invalid();
    if(echo.from!==requested.from||echo.to!==requested.to||echo.group_by!==requested.group_by||echo.limit!==requested.limit||echo.offset!==requested.offset||echo.game_id!==null||echo.member_id!==requested.member_id) return invalid("提现报表回显范围与请求不一致。");
    const summary=totals(d.summary);
    const items=d.items.map(raw=>{
      if(!isObject(raw)||!exactKeys(raw,["key","label","totals"])||typeof raw.key!=="string"||!raw.key||typeof raw.label!=="string"||raw.label!==raw.key||/[\u0000-\u001f\u007f]/.test(raw.label)||!isObject(raw.totals)) return invalid();
      if(requested.group_by==="day"&&!/^\d{4}-\d{2}-\d{2}$/.test(raw.key)||requested.group_by==="member"&&!UUID.test(raw.key)||requested.group_by==="state"&&!STATES.includes(raw.key as WithdrawalState))return invalid();
      return {key:raw.key,label:raw.label,totals:totals(raw.totals)};
    });
    if(items.length>requested.limit||BigInt(d.total_groups)<BigInt(items.length)) return invalid();
    const sums=fields.map(()=>0n);
    for(let i=0;i<items.length;i++) { const item=items[i]!; if(i&&items[i-1]!.key>=item.key) return invalid(); fields.forEach((f,j)=>sums[j]+=BigInt(item.totals[f])); }
    if(requested.offset===0&&BigInt(items.length)===BigInt(d.total_groups)&&fields.some((f,j)=>sums[j]!==BigInt(summary[f]))) return invalid("提现分组与汇总不一致。");
    return {brand_id:d.brand_id,snapshot_at:d.snapshot_at,timezone:d.timezone,query:echo as WithdrawalReport["query"],summary,items,total_groups:d.total_groups};
  }
  async function exportCsv(brand:string,q:WithdrawalReportQuery) {
    if(!UUID.test(brand)) bad("品牌 UUID 无效。");
    const requested=canonical(q);
    if(q.limit!==undefined||q.offset!==undefined) bad("CSV 导出不接受分页参数。");
    const params=new URLSearchParams(); for(const [k,v] of Object.entries(requested)) if(v!==null&&k!=="limit"&&k!=="offset") params.set(k,String(v));
    const response=await fetcher(`${BASE}/export?${params}`,{method:"GET",credentials:"same-origin",headers:{"X-Brand-ID":brand,Accept:"text/csv"}});
    if(!response.ok){let message=`Request failed (${response.status})`,code:string|undefined;try{const b:unknown=await response.json();if(isObject(b)&&isObject(b.error)){if(typeof b.error.message==="string")message=b.error.message;if(typeof b.error.code==="string")code=b.error.code}}catch{}throw new AdminApiError(message,response.status,code)}
    const headers=response.headers,bytes=new Uint8Array(await response.arrayBuffer());
    const digest=await crypto.subtle.digest("SHA-256",bytes);const hex=[...new Uint8Array(digest)].map(x=>x.toString(16).padStart(2,"0")).join("");
    const snapshot=headers.get("X-Report-Snapshot-At")??"",groups=headers.get("X-Report-Group-Count")??"",audit=headers.get("X-Report-Audit-ID")??"";
    const filename=`lottery-withdrawal-${brand}-${snapshot.replace(/[-:]/g,"").replace(/\.\d+Z$/, "Z")}.csv`;
    const columns=["record_type","brand_id","snapshot_at","timezone","from","to","group_by","member_id","key","label",...fields];
    const text=new TextDecoder("utf-8",{ignoreBOM:true}).decode(bytes);
    const checks={size:bytes.length<=4*1024*1024,bom:text.charCodeAt(0)===0xfeff,columns:text.startsWith("\uFEFF"+columns.join(",")+"\n")||text.startsWith("\uFEFF"+columns.join(",")+"\r\n"),brand:headers.get("X-Report-Brand-ID")===brand,kind:headers.get("X-Report-Kind")==="withdrawal",sha:headers.get("X-Report-SHA256")===hex,version:headers.get("X-Report-Format-Version")==="1",audit:headers.get("X-Report-Audit-ID")===audit&&UUID.test(audit),groups:INTEGER.test(groups)&&BigInt(groups)<=10000n&&headers.get("X-Report-Group-Count")===groups,length:headers.get("Content-Length")===String(bytes.length),cache:headers.get("Cache-Control")==="no-store",type:headers.get("Content-Type")==="text/csv; charset=utf-8",disposition:headers.get("Content-Disposition")===`attachment; filename="${filename}"`,contents:validCsv(text,brand,requested,snapshot,groups)};
    if(Object.values(checks).some(ok=>!ok)) return invalid(`提现 CSV 校验失败：${Object.entries(checks).filter(([,ok])=>!ok).map(([name])=>name).join(", ")}`);
    return {filename,bytes};
  }
  return {report:(brand:string,q:WithdrawalReportQuery)=>get(brand,q),exportCsv};
}
