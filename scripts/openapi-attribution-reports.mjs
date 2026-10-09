const ref=name=>({$ref:`#/components/schemas/${name}`});
const obj=properties=>({type:"object",properties,required:Object.keys(properties),additionalProperties:false});
const nullable=schema=>({anyOf:[schema,{type:"null"}]});
const uuid=ref("UUID"),date=ref("DateTime");
const uint={type:"string",pattern:"^(0|[1-9][0-9]*)$",description:"Exact nonnegative decimal integer string; aggregates have no int64 bound."};
const groupBy={type:"string",enum:["day","game","member","agent","join_method"]};
const joinMethod={type:"string",enum:["domain","operator","agent_code","referral_code","legacy"]};
const metricFields=["order_count","stake_points","placed_count","won_count","lost_count","abnormal_count","cancelled_count","refund_points","settled_stake_points","unfinalized_stake_points","abnormal_stake_points","current_prize_points","correction_open_count","final_lost_stake_points","legacy_attribution_count"];
const totals=obj(Object.fromEntries(metricFields.map(field=>[field,uint])));
totals.description="Current operational projection over BET rows in the placement-time cohort, classified by current order generation/state. These figures are not a validated commission base, payout entitlement, or financial authorization. final_lost_stake_points sums current final LOST stake only; winning stakes are never netted against prizes. legacy_attribution_count identifies permanently legacy/unattributed historical rows and is not backfilled.";
const query=obj({
  from:date,to:date,group_by:groupBy,
  limit:{type:"integer",minimum:1,maximum:100},offset:{type:"integer",minimum:0,maximum:1000000},
  game_id:nullable(uuid),member_id:nullable(uuid),agent_id:nullable(uuid),
  agent_scope:{type:"string",enum:["direct","downline"]},join_method:nullable(joinMethod),
});
query.description="Echoes every query field. from is inclusive, to exclusive, normalized to UTC, and the interval is at most 93 days. IDs are validated within the selected brand. JSON pagination defaults to limit 20 and offset 0; agent_scope defaults to direct. Under downline filtering, groups by agent still use each bet's saved direct agent, so groups remain disjoint.";
const group=obj({key:{type:"string"},label:{type:"string"},totals:ref("AttributionReportTotals")});
const report=obj({brand_id:uuid,snapshot_at:date,timezone:{type:"string"},query,summary:ref("AttributionReportTotals"),items:{type:"array",items:ref("AttributionReportGroup"),maxItems:100},total_groups:uint});
export const schemas={AttributionReportTotals:totals,AttributionReportQuery:query,AttributionReportGroup:group,AttributionReport:report};

const permissions=action=>[`report_attribution.${action}.brand`,`report_attribution.${action}.platform`];
const filters=[
  {name:"from",in:"query",required:true,schema:date,description:"RFC3339Nano placement-time lower bound, inclusive."},
  {name:"to",in:"query",required:true,schema:date,description:"RFC3339Nano placement-time upper bound, exclusive; at most 93 days after from."},
  {name:"group_by",in:"query",required:true,schema:groupBy},
  {name:"game_id",in:"query",required:false,schema:uuid},
  {name:"member_id",in:"query",required:false,schema:uuid},
  {name:"agent_id",in:"query",required:false,schema:uuid,description:"Historical agent filter: direct snapshot agent, or saved membership in the snapshot agent chain when agent_scope=downline."},
  {name:"agent_scope",in:"query",required:false,schema:{type:"string",enum:["direct","downline"],default:"direct"},description:"downline requires agent_id."},
  {name:"join_method",in:"query",required:false,schema:joinMethod},
];
const pagination=[
  {name:"limit",in:"query",required:false,schema:{type:"integer",minimum:1,maximum:100,default:20}},
  {name:"offset",in:"query",required:false,schema:{type:"integer",minimum:0,maximum:1000000,default:0}},
];
const description="Read-only operational report grouped from immutable BET attribution_snapshot and placed_at. It does not join current mutable agent relationships and does not authorize commission payouts or other financial actions. One primary-database statement supplies summary, groups, count, timezone, and snapshot. Current session and permissions are rechecked before release, including after audit waits; audit commits before a JSON or CSV response. Requires independent report_attribution.view rights; CSV also requires report_attribution.export rights, and super-admin status alone grants neither. Unknown, duplicate, empty, and unsupported query parameters and GET bodies are rejected. Day keys use the brand timezone; game groups use UUID/name; member groups use the brand member UUID; agent groups use the saved direct agent UUID, none, or legacy; join_method groups use the saved method enumeration. Legacy records remain legacy and are never backfilled.";
export const operations=[
  {method:"GET",path:"/api/v1/admin/reports/attribution",operationId:"adminGetAttributionReport",summary:"Read historical attribution report",tag:"reports",auth:"admin",brandHeader:true,permissions:permissions("view"),parameters:[...filters,...pagination],data:ref("AttributionReport"),description},
  {method:"GET",path:"/api/v1/admin/reports/attribution/export",operationId:"adminExportAttributionReportCSV",summary:"Export complete attribution report CSV",tag:"reports",auth:"admin",brandHeader:true,permissions:[...permissions("view"),...permissions("export")],parameters:filters,additionalErrorStatuses:[413],successDescription:"Complete audited UTF-8 BOM CSV.",successContent:{"text/csv":{schema:{type:"string",format:"binary"}}},successHeaders:{
    "Content-Disposition":{schema:{type:"string"}},"X-Report-Brand-ID":{schema:uuid},"X-Report-Kind":{schema:{type:"string",const:"attribution"}},
    "X-Report-Snapshot-At":{schema:date},"X-Report-Timezone":{schema:{type:"string"}},"X-Report-Group-Count":{schema:uint},
    "X-Report-Byte-Count":{schema:uint},"X-Report-SHA256":{schema:{type:"string",pattern:"^[a-f0-9]{64}$"}},
    "X-Report-Format-Version":{schema:{type:"string",const:"1"}},"X-Report-Audit-ID":{schema:uuid},
    "X-Report-From":{schema:date},"X-Report-To":{schema:date},"X-Report-Group-By":{schema:groupBy},
    "X-Report-Game-ID":{schema:uuid},"X-Report-Member-ID":{schema:uuid},"X-Report-Agent-ID":{schema:uuid},
    "X-Report-Agent-Scope":{schema:{type:"string",enum:["direct","downline"]}},"X-Report-Join-Method":{schema:joinMethod},
  },description:description+` Export has no pagination and fails above 10000 groups or 4MiB without a truncated file. CSV v1 columns are record_type,brand_id,snapshot_at,timezone,from,to,group_by,game_id,member_id,agent_id,agent_scope,join_method,key,label,${metricFields.join(",")}. It contains a summary row followed by every group; no balances row.`},
];
