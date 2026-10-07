const ref=name=>({$ref:`#/components/schemas/${name}`});
const obj=(properties,required=Object.keys(properties))=>({type:"object",properties,required,additionalProperties:false});
const nullable=schema=>({anyOf:[schema,{type:"null"}]});
const uuid=ref("UUID"),date=ref("DateTime");
const exactUnsigned={type:"string",pattern:"^(0|[1-9][0-9]*)$",description:"Exact nonnegative decimal integer string; never convert to a JavaScript Number."};
const exactSigned={type:"string",pattern:"^(0|-?[1-9][0-9]*)$",description:"Exact signed decimal integer string; only net_points may be negative."};
const groupBy={type:"string",enum:["day","agent","cycle"]};
const totals=obj({
  entry_count:exactUnsigned,
  paid_entry_count:exactUnsigned,
  paid_points:exactUnsigned,
  adjustment_entry_count:exactUnsigned,
  adjustment_credit_points:exactUnsigned,
  adjustment_debit_points:exactUnsigned,
  net_points:exactSigned,
});
totals.description="Immutable commission ledger postings by posting time, not bet/cycle cohort, current wallet, forecast, or verified current earnings. entry_count = paid_entry_count + adjustment_entry_count; net_points = paid_points + adjustment_credit_points - adjustment_debit_points. A period may contain a debit whose credit occurred earlier, so net_points may be negative.";
const query=obj({
  from:date,to:date,group_by:groupBy,
  limit:{type:"integer",minimum:1,maximum:100},offset:{type:"integer",minimum:0,maximum:1000000},
  agent_id:nullable(uuid),member_id:nullable(uuid),cycle_id:nullable(uuid),
});
query.description="Echoed normalized query. from is inclusive and to exclusive; maximum interval 93 days. IDs are validated within the selected brand. JSON report pagination defaults to limit 20 and offset 0.";
export const schemas={
  CommissionReportTotals:totals,
  CommissionReportGroup:obj({key:{type:"string"},label:{type:"string"},totals:ref("CommissionReportTotals")}),
  CommissionReport:obj({brand_id:uuid,snapshot_at:date,timezone:{type:"string"},query,summary:ref("CommissionReportTotals"),items:{type:"array",items:ref("CommissionReportGroup"),maxItems:100},total_groups:exactUnsigned}),
};
const windowParameters=[
  {name:"from",in:"query",required:true,schema:date,description:"RFC3339 ledger-posting-time lower bound, inclusive."},
  {name:"to",in:"query",required:true,schema:date,description:"RFC3339 ledger-posting-time upper bound, exclusive; at most 93 days after from."},
  {name:"group_by",in:"query",required:true,schema:groupBy},
  {name:"agent_id",in:"query",required:false,schema:uuid},
  {name:"member_id",in:"query",required:false,schema:uuid},
  {name:"cycle_id",in:"query",required:false,schema:uuid},
];
const pagination=[
  {name:"limit",in:"query",required:false,schema:{type:"integer",minimum:1,maximum:100,default:20}},
  {name:"offset",in:"query",required:false,schema:{type:"integer",minimum:0,maximum:1000000,default:0}},
];
const permission=verb=>[`report_commission.${verb}.brand`,`report_commission.${verb}.platform`];
const description="Read-only immutable commission business-ledger posting report. It includes true paid-target and true adjustment postings exactly once, and retains past credit postings after later cycle blocking or correction. It is not a bet/cycle-period cohort, current wallet balance, forecast, or verified current earnings. It does not invent an allocation of rounded cycle points per lottery. Days use the selected brand IANA timezone; keys are YYYY-MM-DD for day and UUIDs for agent/cycle, with label equal to key. Strict parsing rejects unknown, duplicate, empty, and unsupported parameters. Primary-database Read Committed transaction rechecks current admin session/roles after the query and commits its audit before any response data. Super-admin status does not grant report access.";
export const operations=[
  {method:"GET",path:"/api/v1/admin/reports/commission",operationId:"adminGetCommissionReport",summary:"Read commission posting report",tag:"reports",auth:"admin",data:ref("CommissionReport"),brandHeader:true,permissions:permission("view"),parameters:[...windowParameters,...pagination],description},
  {method:"GET",path:"/api/v1/admin/reports/commission.csv",operationId:"adminExportCommissionReportCSV",summary:"Export commission posting report CSV",tag:"reports",auth:"admin",brandHeader:true,permissions:[...permission("view"),...permission("export")],parameters:windowParameters,additionalErrorStatuses:[413],successDescription:"Complete UTF-8 CSV after its audit record commits.",successContent:{"text/csv":{schema:{type:"string",format:"binary"}}},successHeaders:{
    "Content-Disposition":{description:"Complete CSV download filename.",schema:{type:"string"}},
    "X-Report-Brand-ID":{description:"Selected report brand.",schema:uuid},
    "X-Report-Kind":{description:"Report identifier.",schema:{type:"string",const:"commission"}},
    "X-Report-Snapshot-At":{description:"Database snapshot timestamp.",schema:date},
    "X-Report-Timezone":{description:"Brand IANA timezone used for day grouping.",schema:{type:"string"}},
    "X-Report-Group-Count":{description:"Complete number of exported groups.",schema:exactUnsigned},
    "X-Report-Byte-Count":{description:"Exact response body byte count.",schema:{type:"string",pattern:"^(0|[1-9][0-9]*)$"}},
    "X-Report-SHA256":{description:"SHA-256 digest of response body bytes including UTF-8 BOM.",schema:{type:"string",pattern:"^[a-f0-9]{64}$"}},
    "X-Report-Format-Version":{description:"CSV layout version.",schema:{type:"string",const:"1"}},
    "X-Report-Audit-ID":{description:"Committed export audit record ID.",schema:uuid},
  },description:description+" Export has no pagination parameters and fails above 10000 groups or 4MiB. CSV columns, in order: record_type, brand_id, snapshot_at, timezone, from, to, group_by, agent_id, member_id, cycle_id, key, label, entry_count, paid_entry_count, paid_points, adjustment_entry_count, adjustment_credit_points, adjustment_debit_points, net_points. It includes a UTF-8 BOM and formula-injection neutralization; negative net_points cells are apostrophe-prefixed for spreadsheet safety. A zero-group export contains its summary row and no group rows."},
];
