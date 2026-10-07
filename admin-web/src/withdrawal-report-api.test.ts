import {describe,expect,it,vi} from "vitest";
import {type AdminAccount} from "./admin-api";
import {createWithdrawalReportApi,withdrawalReportPermissions,type WithdrawalTotals} from "./withdrawal-report-api";

const brand="018f47a1-7b2c-7abc-8def-0123456789ab";
const member="018f47a1-7b2c-7abc-8def-0123456789ad";
const from="2026-01-01T00:00:00Z",to="2026-01-02T00:00:00Z";
const huge="922337203685477580812345678901234567890";
const totals:WithdrawalTotals={order_count:"1",requested_points:huge,reviewing_count:"0",reviewing_points:"0",processing_count:"0",processing_points:"0",paid_count:"1",paid_points:huge,rejected_count:"0",rejected_points:"0",failed_count:"0",failed_points:"0",cancelled_count:"0",cancelled_points:"0"};
const payload=(overrides:Record<string,unknown>={})=>({success:true,request_id:"018f47a1-7b2c-7abc-8def-0123456789ac",data:{brand_id:brand,snapshot_at:"2026-01-02T01:00:00Z",timezone:"Asia/Singapore",query:{from,to,group_by:"day",limit:20,offset:0,game_id:null,member_id:null},summary:totals,items:[{key:"2026-01-01",label:"2026-01-01",totals}],total_groups:"1",...overrides}});
const response=(data:unknown)=>new Response(JSON.stringify(data),{status:200,headers:{"content-type":"application/json"}});

describe("withdrawal report authorization",()=>{
  const account:AdminAccount={id:brand,super_admin:true,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:["report_withdrawal.view.brand"]}};
  it("requires view plus export, and enforces selected-brand membership for brand grants",()=>{
    expect(withdrawalReportPermissions(account,brand)).toEqual({view:true,export:false});
    expect(withdrawalReportPermissions({...account,permissions_by_brand:{[brand]:["report_withdrawal.export.brand"]}},brand)).toEqual({view:false,export:false});
    expect(withdrawalReportPermissions({...account,permissions_by_brand:{[brand]:["report_withdrawal.view.brand","report_withdrawal.export.brand"]}},brand)).toEqual({view:true,export:true});
    expect(withdrawalReportPermissions({...account,brand_ids:[],permissions_by_brand:{[brand]:["report_withdrawal.view.brand","report_withdrawal.export.brand"]}},brand)).toEqual({view:false,export:false});
    expect(withdrawalReportPermissions({...account,brand_ids:[],permissions_by_brand:{},platform_permissions:["report_withdrawal.view.platform","report_withdrawal.export.platform"]},brand)).toEqual({view:true,export:true});
    expect(withdrawalReportPermissions({...account,brand_ids:[],permissions_by_brand:{[brand]:["report_withdrawal.export.brand"]},platform_permissions:["report_withdrawal.view.platform"]},brand)).toEqual({view:true,export:false});
    expect(withdrawalReportPermissions({...account,permissions_by_brand:{},platform_permissions:["report_withdrawal.export.platform"]},brand)).toEqual({view:false,export:false});
    expect(withdrawalReportPermissions({...account,permissions:["report_withdrawal.view.brand"],permissions_by_brand:undefined},brand).view).toBe(false);
  });
});

describe("withdrawal report API",()=>{
  it("sends exact range and brand and retains arbitrary precision strings",async()=>{
    const fetcher=vi.fn<typeof fetch>().mockResolvedValue(response(payload()));
    const report=await createWithdrawalReportApi(fetcher).report(brand,{from,to,group_by:"day"});
    expect(report.summary.requested_points).toBe(huge);
    const [url,init]=fetcher.mock.calls[0]!;
    expect(new URL(String(url),"http://localhost").searchParams.get("game_id")).toBeNull();
    expect(new URL(String(url),"http://localhost").searchParams.get("limit")).toBe("20");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
  });
  it("rejects changed range echoes, inconsistent status math, unsupported filters and invalid bounds",async()=>{
    await expect(createWithdrawalReportApi(vi.fn<typeof fetch>().mockResolvedValue(response(payload({query:{from:"2026-01-01T00:00:00.001Z",to,group_by:"day",limit:20,offset:0,game_id:null,member_id:null}})))).report(brand,{from,to,group_by:"day"})).rejects.toMatchObject({code:"INVALID_RESPONSE"});
    await expect(createWithdrawalReportApi(vi.fn<typeof fetch>().mockResolvedValue(response(payload({summary:{...totals,paid_count:"2"}})))).report(brand,{from,to,group_by:"day"})).rejects.toMatchObject({code:"INVALID_RESPONSE"});
    await expect(createWithdrawalReportApi(vi.fn<typeof fetch>().mockResolvedValue(response({...payload(),unexpected:true}))).report(brand,{from,to,group_by:"day"})).rejects.toMatchObject({code:"INVALID_RESPONSE"});
    const missingRequestId=payload() as Record<string,unknown>;delete missingRequestId.request_id;
    await expect(createWithdrawalReportApi(vi.fn<typeof fetch>().mockResolvedValue(response(missingRequestId))).report(brand,{from,to,group_by:"day"})).rejects.toMatchObject({code:"INVALID_RESPONSE"});
    const api=createWithdrawalReportApi(vi.fn<typeof fetch>());
    await expect(api.report(brand,{from,to,group_by:"day",game_id:brand} as never)).rejects.toMatchObject({code:"INVALID_INPUT"});
    await expect(api.report(brand,{from,to:"2026-04-05T00:00:00.000Z",group_by:"day"})).rejects.toMatchObject({code:"INVALID_INPUT"});
  });
  it("keeps a real filtered summary on an empty page beyond the last group",async()=>{
    const data=payload({query:{from,to,group_by:"day",limit:20,offset:40,game_id:null,member_id:null},items:[]});
    await expect(createWithdrawalReportApi(vi.fn<typeof fetch>().mockResolvedValue(response(data))).report(brand,{from,to,group_by:"day",offset:40})).resolves.toMatchObject({summary:{order_count:"1",requested_points:huge},items:[],total_groups:"1"});
  });
  it.each(["\r\n","\n"])("validates complete CSV media, digest, scope, shape and audit metadata with %j line endings",async(lineEnding)=>{
    const header="record_type,brand_id,snapshot_at,timezone,from,to,group_by,member_id,key,label,order_count,requested_points,reviewing_count,reviewing_points,processing_count,processing_points,paid_count,paid_points,rejected_count,rejected_points,failed_count,failed_points,cancelled_count,cancelled_points\r\n";
    const snapshot="2026-01-02T01:00:00Z";const csv=`\uFEFF${header}summary,${brand},${snapshot},Asia/Singapore,${from},${to},day,${member},,,1,${huge},0,0,0,0,1,${huge},0,0,0,0,0,0\r\ngroup,${brand},${snapshot},Asia/Singapore,${from},${to},day,${member},2026-01-01,2026-01-01,1,${huge},0,0,0,0,1,${huge},0,0,0,0,0,0\r\n`;
    const bytes=new TextEncoder().encode(csv.replaceAll("\r\n",lineEnding));const hash=[...new Uint8Array(await crypto.subtle.digest("SHA-256",bytes))].map(x=>x.toString(16).padStart(2,"0")).join("");
    const headers=new Headers({"X-Report-Brand-ID":brand,"X-Report-Kind":"withdrawal","X-Report-Snapshot-At":snapshot,"X-Report-Group-Count":"1","X-Report-SHA256":hash,"X-Report-Format-Version":"1","X-Report-Audit-ID":brand,"Content-Length":String(bytes.length),"Content-Type":"text/csv; charset=utf-8","Content-Disposition":`attachment; filename="lottery-withdrawal-${brand}-20260102T010000Z.csv"`,"Cache-Control":"no-store"});
    const fetcher=vi.fn<typeof fetch>().mockResolvedValue(new Response(bytes,{status:200,headers}));
    await expect(createWithdrawalReportApi(fetcher).exportCsv(brand,{from,to,group_by:"day",member_id:member})).resolves.toMatchObject({filename:`lottery-withdrawal-${brand}-20260102T010000Z.csv`});
  });
});
