import { test } from "node:test";
import assert from "node:assert/strict";
import { composeDocument } from "../../scripts/openapi-lib.mjs";
import { operations, schemas } from "../../scripts/openapi-commission-analysis.mjs";

const jsonPath = "/api/v1/admin/reports/commission-analysis";
const csvPath = `${jsonPath}.csv`;
const csvColumns = [
  "record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "agent_id", "member_id", "cycle_id", "key", "label",
  "selected_cycle_count", "ready_cycle_count", "unready_cycle_count",
  "observed_calculated_points", "calculated_points", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points",
  "correction_entry_count", "correction_credit_points", "correction_debit_points", "posting_entry_count", "actual_net_points", "manual_adjustment_net_points",
  "effective_target_points", "calculation_minus_actual_points", "effective_minus_actual_points", "calculation_complete", "effective_target_complete",
];

test("documents only the JSON and CSV cycle analysis routes with explicit dual permissions", () => {
  assert.deepEqual(operations.map(({ method, path }) => `${method} ${path}`), [`GET ${jsonPath}`, `GET ${csvPath}`]);
  const [json, csv] = operations;
  assert.deepEqual(json.permissions, [
    "commission.view.brand", "commission.view.platform", "report_commission.view.brand", "report_commission.view.platform",
  ]);
  assert.deepEqual(csv.permissions, [...json.permissions, "report_commission.export.brand", "report_commission.export.platform"]);
  assert.equal(json.data.$ref, "#/components/schemas/CommissionAnalysisReport");
  assert.equal(csv.successHeaders["X-Report-Kind"].schema.const, "commission_analysis");
  assert.equal(csv.successHeaders["X-Report-Format-Version"].schema.const, "1");
  assert.ok(csv.additionalErrorStatuses.includes(413));
});

test("closes report, query, coverage, and totals objects to the documented exact keys", () => {
  const exact = schema => {
    assert.equal(schema.type, "object");
    assert.equal(schema.additionalProperties, false);
    assert.deepEqual(schema.required, Object.keys(schema.properties));
    return Object.keys(schema.properties);
  };
  assert.deepEqual(exact(schemas.CommissionAnalysisReport), ["brand_id", "snapshot_at", "timezone", "query", "coverage", "summary", "items", "total_groups"]);
  assert.deepEqual(exact(schemas.CommissionAnalysisReport.properties.query), ["from", "to", "group_by", "limit", "offset", "agent_id", "member_id", "cycle_id"]);
  assert.deepEqual(exact(schemas.CommissionAnalysisCoverage), ["selected_cycle_count", "ready_cycle_count", "unready_cycle_count"]);
  assert.deepEqual(exact(schemas.CommissionAnalysisTotals), [
    "observed_calculated_points", "calculated_points", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points",
    "correction_entry_count", "correction_credit_points", "correction_debit_points", "posting_entry_count", "actual_net_points", "manual_adjustment_net_points",
    "effective_target_points", "calculation_minus_actual_points", "effective_minus_actual_points", "calculation_complete", "effective_target_complete",
  ]);
  assert.deepEqual(exact(schemas.CommissionAnalysisGroup), ["key", "label", "totals"]);
  for (const field of ["calculated_points", "effective_target_points", "calculation_minus_actual_points", "effective_minus_actual_points"]) {
    assert.ok(schemas.CommissionAnalysisTotals.properties[field].anyOf.some(type => type.type === "null"));
  }
  assert.equal(schemas.CommissionAnalysisTotals.properties.calculation_complete.type, "boolean");
  assert.equal(schemas.CommissionAnalysisTotals.properties.effective_target_complete.type, "boolean");
});

test("describes the cycle window, strict bounded query, and all 33 CSV columns", () => {
  const [json, csv] = operations;
  assert.match(json.description, /cycle\.window_to/);
  assert.match(json.description, /93 days/);
  assert.match(json.description, /super-admin identity does not imply/);
  assert.deepEqual(json.parameters.filter(p => p.in === "query").map(p => p.name), ["from", "to", "group_by", "agent_id", "member_id", "cycle_id", "limit", "offset"]);
  assert.deepEqual(csv.parameters.filter(p => p.in === "query").map(p => p.name), ["from", "to", "group_by", "agent_id", "member_id", "cycle_id"]);
  assert.match(csv.description, /cycle\.window_to/);
  assert.match(csv.description, /apostrophe safety prefix/);
  assert.match(csv.description, /10000 groups or 4MiB/);
  assert.equal(csvColumns.length, 33);
  assert.ok(csv.description.includes(csvColumns.join(", ")));
});

test("operation schemas resolve with the repository OpenAPI contract builder", () => {
  const routes = operations.map(({ method, path }) => ({ method, path }));
  const doc = composeDocument([{ schemas, operations }], routes);
  assert.equal(doc.paths[jsonPath].get.responses[200].content["application/json"].schema.properties.data.$ref, "#/components/schemas/CommissionAnalysisReport");
  assert.equal(doc.paths[csvPath].get.responses[200].content["text/csv"].schema.format, "binary");
  assert.ok(doc.paths[csvPath].get.responses[409]);
  assert.ok(doc.paths[csvPath].get.responses[413]);
});
