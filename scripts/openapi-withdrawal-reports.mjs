const ref=name=>({$ref:`#/components/schemas/${name}`});
const obj=(properties,required=Object.keys(properties))=>({type:"object",properties,required,additionalProperties:false});
const uuid=ref("UUID"),date=ref("DateTime");
const integer={type:"string",pattern:"^(0|[1-9][0-9]*)$",description:"Exact nonnegative decimal integer string. Aggregates may exceed int64; never convert to JavaScript Number."};
const groups={type:"string",enum:["day","member","state"]};
const fields={order_count:integer,requested_points:integer,reviewing_count:integer,reviewing_points:integer,processing_count:integer,processing_points:integer,paid_count:integer,paid_points:integer,rejected_count:integer,rejected_points:integer,failed_count:integer,failed_points:integer,cancelled_count:integer,cancelled_points:integer};
const totals=obj(fields);totals.description="Current withdrawal_orders.state projection for orders created in the requested half-open [from,to) interval. It is not immutable closeout data, a payout-occurrence measure, or external transfer evidence.";
const query=obj({from:date,to:date,group_by:groups,limit:{type:"integer",minimum:1,maximum:100},offset:{type:"integer",minimum:0,maximum:1000000},game_id:{type:"null"},member_id:{anyOf:[uuid,{type:"null"}]} });
query.description="Echoed normalized query. game_id is always null because withdrawals have no lottery game.";
export const schemas={
  WithdrawalReportTotals:totals,
  WithdrawalReportGroup:obj({key:{type:"string"},label:{type:"string"},totals:ref("WithdrawalReportTotals")}),
  WithdrawalReport:obj({brand_id:uuid,snapshot_at:date,timezone:{type:"string"},query,summary:ref("WithdrawalReportTotals"),items:{type:"array",items:ref("WithdrawalReportGroup"),maxItems:100},total_groups:integer}),
};
const windowParameters=[
  {name:"from",in:"query",required:true,schema:date,description:"RFC3339 application-created-at lower bound, inclusive."},
  {name:"to",in:"query",required:true,schema:date,description:"RFC3339 application-created-at upper bound, exclusive; at most 93 days after from."},
  {name:"group_by",in:"query",required:true,schema:groups},
  {name:"member_id",in:"query",required:false,schema:uuid},
];
const pagination=[{name:"limit",in:"query",required:false,schema:{type:"integer",minimum:1,maximum:100,default:20}},{name:"offset",in:"query",required:false,schema:{type:"integer",minimum:0,maximum:1000000,default:0}}];
const permission=(verb)=>[`report_withdrawal.${verb}.brand`,`report_withdrawal.${verb}.platform`];
const description="Read-only withdrawal application report. Filters withdrawal_orders.created_at in half-open [from,to), groups days using the brand IANA timezone and reports the current state projection from one SQL statement. State changes after request creation are reflected in past request cohorts. It is not immutable closeout, payout time, or external transfer data. Strict parsing rejects game_id, unknown/duplicate/empty query parameters. Super-admin status does not grant report access. Primary-database Read Committed transaction locks and rechecks current admin session/roles before and after the query; audit commits before any response data.";
export const operations=[
 {method:"GET",path:"/api/v1/admin/reports/withdrawal",operationId:"adminGetWithdrawalReport",summary:"Read withdrawal application report",tag:"reports",auth:"admin",data:ref("WithdrawalReport"),brandHeader:true,permissions:permission("view"),parameters:[...windowParameters,...pagination],description},
 {
   method:"GET", path:"/api/v1/admin/reports/withdrawal/export", operationId:"adminExportWithdrawalReport",
   summary:"Export withdrawal application report CSV", tag:"reports", auth:"admin",
   successDescription:"Complete UTF-8 CSV after its audit record commits.",
   successContent:{"text/csv":{"schema":{"type":"string","format":"binary"}}},
   successHeaders:{
     "Content-Disposition":{description:"Complete CSV download filename.",schema:{type:"string"}},
     "X-Report-Brand-ID":{description:"Selected report brand.",schema:uuid},
     "X-Report-Kind":{description:"Report identifier.",schema:{type:"string",const:"withdrawal"}},
     "X-Report-Snapshot-At":{description:"Database snapshot timestamp.",schema:date},
     "X-Report-Group-Count":{description:"Complete number of groups in the export.",schema:integer},
     "X-Report-SHA256":{description:"SHA-256 digest of the response body.",schema:{type:"string",pattern:"^[a-f0-9]{64}$"}},
     "X-Report-Format-Version":{description:"CSV layout version.",schema:{type:"string",const:"1"}},
     "X-Report-Audit-ID":{description:"Committed export audit record ID.",schema:uuid},
   },
   brandHeader:true, permissions:[...permission("view"),...permission("export")], parameters:windowParameters,
   description:description+" Export has no pagination parameters and is rejected above 10000 groups or 4MiB. CSV columns are record_type, brand_id, snapshot_at, timezone, from, to, group_by, member_id, key, label, order_count, requested_points, reviewing_count, reviewing_points, processing_count, processing_points, paid_count, paid_points, rejected_count, rejected_points, failed_count, failed_points, cancelled_count, cancelled_points. It includes a BOM, formula-injection neutralization and SHA-256 audit metadata; game_id is not present.",
 },
];
