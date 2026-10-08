const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = properties => ({ type: "object", properties, required: Object.keys(properties), additionalProperties: false });
const uuid = ref("UUID"), date = ref("DateTime");
const uint = { type: "string", pattern: "^(0|[1-9][0-9]*)$", description: "Exact nonnegative decimal integer string; aggregates have no int64 bound." };
const sint = { type: "string", pattern: "^(0|-?[1-9][0-9]*)$" };
const nullable = schema => ({ anyOf: [schema, { type: "null" }] });
const grouping = kind => ({ type: "string", enum: kind === "rewards" ? ["day", "member", "order"] : ["day", "member", "state"] });
const query = kind => obj({ from: date, to: date, group_by: grouping(kind), limit: { type: "integer", minimum: 1, maximum: 100 }, offset: { type: "integer", minimum: 0, maximum: 1000000 }, member_id: nullable(uuid), order_id: nullable(uuid) });
const postingFields = ["entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points"];
const orderFields = ["order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points"];
const totals = fields => obj(Object.fromEntries(fields.map(field => [field, field === "net_points" ? sint : uint])));
const group = name => obj({ key: { type: "string" }, label: { type: "string" }, totals: ref(name) });
const report = (queryName, totalsName, groupName) => obj({ brand_id: uuid, snapshot_at: date, timezone: { type: "string" }, query: ref(queryName), summary: ref(totalsName), items: { type: "array", items: ref(groupName), maxItems: 100 }, total_groups: uint });
export const schemas = {
  RewardReportQuery: query("rewards"), RewardOrderReportQuery: query("reward_orders"),
  RewardReportTotals: { ...totals(postingFields), description: "Actual immutable gift-source reward grant and full reversal postings by ledger posting time. entry_count=grant_entry_count+reversal_entry_count; net_points=grant_points-reversal_points. Reversal-only windows may have negative net_points. Pending attempts do not post." },
  RewardOrderReportTotals: { ...totals(orderFields), description: "Orders created in the selected time cohort, classified by CURRENT state at snapshot time, once per order. Counts and original amounts partition into granted, revocation_pending and revoked. Pending points are original order amounts, NOT reserved or deducted points; revoked points are NOT reversal postings in this time window." },
  RewardReportGroup: group("RewardReportTotals"), RewardOrderReportGroup: group("RewardOrderReportTotals"),
  RewardReport: report("RewardReportQuery", "RewardReportTotals", "RewardReportGroup"),
  RewardOrderReport: report("RewardOrderReportQuery", "RewardOrderReportTotals", "RewardOrderReportGroup"),
};
const permissions = action => [`report_reward.${action}.brand`, `report_reward.${action}.platform`];
const parameters = kind => [
  { name: "from", in: "query", required: true, schema: date, description: "Inclusive posting-time bound for rewards; inclusive order-created-time bound for reward_orders." },
  { name: "to", in: "query", required: true, schema: date, description: "Exclusive upper bound; maximum window 93 days. Offset timestamps normalize to UTC, preserving nanoseconds." },
  { name: "group_by", in: "query", required: true, schema: grouping(kind) },
  ...["member_id", "order_id"].map(name => ({ name, in: "query", required: false, schema: uuid, description: "Optional ID within the selected brand; never joins foreign-brand data." })),
];
const pagination = [{ name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 } }, { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 } }];
const headers = kind => ({
  "Content-Disposition": { schema: { type: "string" }, description: "lottery-kind-brand-UTCsnapshot.csv attachment filename." },
  "Content-Length": { schema: uint },
  "X-Report-Brand-ID": { schema: uuid }, "X-Report-Kind": { schema: { type: "string", const: kind } },
  "X-Report-Snapshot-At": { schema: date }, "X-Report-Timezone": { schema: { type: "string" } },
  "X-Report-Group-Count": { schema: uint }, "X-Report-Byte-Count": { schema: uint },
  "X-Report-SHA256": { schema: { type: "string", pattern: "^[a-f0-9]{64}$" }, description: "SHA-256 of complete response bytes including UTF-8 BOM." },
  "X-Report-Format-Version": { schema: { type: "string", const: "1" } }, "X-Report-Audit-ID": { schema: uuid },
  "X-Report-From": { schema: date }, "X-Report-To": { schema: date }, "X-Report-Group-By": { schema: grouping(kind) },
  "X-Report-Member-ID": { schema: uuid, description: "Present only when filtered by member." }, "X-Report-Order-ID": { schema: uuid, description: "Present only when filtered by order." },
});
const description = "Read-only, primary-database single-statement snapshot with independent report_reward.view.brand or explicit platform rights; reward.view, wallet.view and super-admin identity alone grant no report access. CSV also requires matching report_reward.export rights. Current session and roles are rechecked before data release, including after audit waits; audit must commit before any DTO/CSV response. Unknown, duplicate, empty parameters, GET bodies and unsupported read-routing controls are rejected. Groups use brand IANA timezone for day keys, machine IDs for member/order, or canonical states, sorted using C collation; labels equal keys. No wallet or business-order mutation.";
export const operations = ["rewards", "reward_orders"].flatMap(kind => {
  const path = kind === "rewards" ? "/api/v1/admin/reports/rewards" : "/api/v1/admin/reports/reward-orders";
  const name = kind === "rewards" ? "Reward" : "RewardOrder";
  return [
    { method: "GET", path, operationId: `adminGet${name}Report`, summary: `Read ${kind} report`, tag: "reports", auth: "admin", brandHeader: true, permissions: permissions("view"), parameters: [...parameters(kind), ...pagination], data: ref(`${name}Report`), description },
    { method: "GET", path: `${path}.csv`, operationId: `adminExport${name}ReportCSV`, summary: `Export complete ${kind} CSV`, tag: "reports", auth: "admin", brandHeader: true, permissions: [...permissions("view"), ...permissions("export")], parameters: parameters(kind), additionalErrorStatuses: [413], successDescription: "Complete audited UTF-8 BOM CSV.", successContent: { "text/csv": { schema: { type: "string", format: "binary" } } }, successHeaders: headers(kind), description: description + ` Export has no pagination; over 10000 groups or 4MiB fails without partial data. CSV columns: record_type,brand_id,snapshot_at,timezone,from,to,group_by,member_id,order_id,key,label,${(kind === "rewards" ? postingFields : orderFields).join(",")}. Header then summary then all groups; signed negative net_points is apostrophe-prefixed for spreadsheet formula safety. Zero-group export retains its summary row.` },
  ];
});
