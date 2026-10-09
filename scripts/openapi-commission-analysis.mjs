const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({ type: "object", properties, required, additionalProperties: false });
const nullable = schema => ({ anyOf: [schema, { type: "null" }] });
const count = { type: "string", pattern: "^(0|[1-9][0-9]*)$", description: "Exact nonnegative decimal integer string." };
const signed = { type: "string", pattern: "^(0|-?[1-9][0-9]*)$", description: "Exact signed decimal integer string." };
const uuid = ref("UUID");
const date = ref("DateTime");
const groupBy = { type: "string", enum: ["cycle", "agent"] };

const totals = obj({
  observed_calculated_points: count,
  calculated_points: nullable(count),
  paid_entry_count: count,
  paid_points: count,
  adjustment_entry_count: count,
  adjustment_credit_points: count,
  adjustment_debit_points: count,
  correction_entry_count: count,
  correction_credit_points: count,
  correction_debit_points: count,
  posting_entry_count: count,
  actual_net_points: signed,
  manual_adjustment_net_points: signed,
  effective_target_points: nullable(count),
  calculation_minus_actual_points: nullable(signed),
  effective_minus_actual_points: nullable(signed),
  calculation_complete: { type: "boolean" },
  effective_target_complete: { type: "boolean" },
});
totals.description = "Exact cycle-analysis totals. actual_net_points equals paid_points plus adjustment credits minus adjustment debits plus correction credits minus correction debits. Null denotes an incomplete explanation, never zero. Difference fields are mathematical comparisons and do not authorize financial action.";

const query = obj({
  from: date,
  to: date,
  group_by: groupBy,
  limit: { type: "integer", minimum: 1, maximum: 100 },
  offset: { type: "integer", minimum: 0, maximum: 1000000 },
  agent_id: nullable(uuid),
  member_id: nullable(uuid),
  cycle_id: nullable(uuid),
});
query.description = "Normalized query. from is inclusive and to exclusive; selection is by saved cycle.window_to, not posting time. Interval is at most 93 days. IDs are canonical lowercase UUIDs scoped to this brand. JSON defaults to limit 20 and offset 0.";

const coverage = obj({
  selected_cycle_count: count,
  ready_cycle_count: count,
  unready_cycle_count: count,
});
coverage.description = "selected_cycle_count equals ready_cycle_count plus unready_cycle_count.";

export const schemas = {
  CommissionAnalysisCoverage: coverage,
  CommissionAnalysisTotals: totals,
  CommissionAnalysisGroup: obj({ key: uuid, label: uuid, totals: ref("CommissionAnalysisTotals") }),
  CommissionAnalysisReport: obj({
    brand_id: uuid,
    snapshot_at: date,
    timezone: { type: "string" },
    query,
    coverage: ref("CommissionAnalysisCoverage"),
    summary: ref("CommissionAnalysisTotals"),
    items: { type: "array", items: ref("CommissionAnalysisGroup"), maxItems: 100 },
    total_groups: count,
  }),
};

const queryParameters = [
  { name: "from", in: "query", required: true, schema: date, description: "RFC3339 lower bound, inclusive; applies to saved cycle.window_to." },
  { name: "to", in: "query", required: true, schema: date, description: "RFC3339 upper bound, exclusive; at most 93 days after from." },
  { name: "group_by", in: "query", required: true, schema: groupBy },
  { name: "agent_id", in: "query", required: false, schema: uuid },
  { name: "member_id", in: "query", required: false, schema: uuid },
  { name: "cycle_id", in: "query", required: false, schema: uuid },
];
const permissions = ["commission.view.brand", "commission.view.platform", "report_commission.view.brand", "report_commission.view.platform"];
const description = "Read-only cycle analysis selected by saved cycle.window_to, including all historical real postings attached to selected cycles regardless of posting time. Requires an authenticated admin session and both commission.view and report_commission.view, each via explicit brand or platform grants; super-admin identity does not imply either grant. Export additionally requires report_commission.export. One primary SQL statement supplies each complete report snapshot. Current authorization is rechecked after the query, and report data is released only after its audit commits. Incomplete current calculation evidence produces null targets; inconsistent financial witnesses fail closed with 409 COMMISSION_ANALYSIS_INTEGRITY.";

export const operations = [
  {
    method: "GET",
    path: "/api/v1/admin/reports/commission-analysis",
    operationId: "adminGetCommissionAnalysisReport",
    summary: "Read commission cycle analysis",
    tag: "reports",
    auth: "admin",
    data: ref("CommissionAnalysisReport"),
    brandHeader: true,
    permissions,
    parameters: [...queryParameters,
      { name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 } },
      { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 } },
    ],
    description: description + " The complete cycle.window_to range is limited to 93 days. Strict query parsing rejects unknown, duplicate, empty, noncanonical IDs, GET bodies, and values outside the closed query contract.",
  },
  {
    method: "GET",
    path: "/api/v1/admin/reports/commission-analysis.csv",
    operationId: "adminExportCommissionAnalysisCSV",
    summary: "Export commission cycle analysis CSV",
    tag: "reports",
    auth: "admin",
    brandHeader: true,
    permissions: [...permissions, "report_commission.export.brand", "report_commission.export.platform"],
    parameters: queryParameters,
    additionalErrorStatuses: [413],
    successDescription: "Complete UTF-8 CSV after its export audit commits.",
    successContent: { "text/csv": { schema: { type: "string", format: "binary" } } },
    successHeaders: {
      "Content-Disposition": { description: "Complete CSV download filename.", schema: { type: "string" } },
      "X-Report-Brand-ID": { description: "Selected report brand.", schema: uuid },
      "X-Report-Kind": { description: "Report identifier.", schema: { type: "string", const: "commission_analysis" } },
      "X-Report-Snapshot-At": { description: "Database snapshot timestamp.", schema: date },
      "X-Report-Timezone": { description: "Brand IANA timezone.", schema: { type: "string" } },
      "X-Report-Group-Count": { description: "Complete number of exported groups.", schema: count },
      "X-Report-Byte-Count": { description: "Exact response body byte count.", schema: count },
      "X-Report-SHA256": { description: "SHA-256 digest of the response body bytes, including BOM.", schema: { type: "string", pattern: "^[a-f0-9]{64}$" } },
      "X-Report-Format-Version": { description: "CSV layout version.", schema: { type: "string", const: "1" } },
      "X-Report-Audit-ID": { description: "Committed export audit record ID.", schema: uuid },
    },
    description: description + " Export rejects limit and offset. CSV v1 is UTF-8 with BOM and exactly 33 columns: record_type, brand_id, snapshot_at, timezone, from, to, group_by, agent_id, member_id, cycle_id, key, label, selected_cycle_count, ready_cycle_count, unready_cycle_count, observed_calculated_points, calculated_points, paid_entry_count, paid_points, adjustment_entry_count, adjustment_credit_points, adjustment_debit_points, correction_entry_count, correction_credit_points, correction_debit_points, posting_entry_count, actual_net_points, manual_adjustment_net_points, effective_target_points, calculation_minus_actual_points, effective_minus_actual_points, calculation_complete, effective_target_complete. Summary comes first, followed by groups in key order. Nullable values are empty cells; negative signed values have an apostrophe safety prefix. More than 10000 groups or 4MiB returns 413 without a partial file.",
  },
];
