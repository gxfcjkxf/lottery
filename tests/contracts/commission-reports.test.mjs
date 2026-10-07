import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";

const doc=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const paths=doc.paths,schemas=doc.components.schemas;

test("commission report schema keeps exact signed posting totals and a closed response",()=>{
  const totals=schemas.CommissionReportTotals;
  assert.equal(totals.additionalProperties,false);
  assert.deepEqual(Object.keys(totals.properties),["entry_count","paid_entry_count","paid_points","adjustment_entry_count","adjustment_credit_points","adjustment_debit_points","net_points"]);
  for(const field of Object.entries(totals.properties).filter(([name])=>name!=="net_points").map(([,schema])=>schema))assert.match(field.pattern,/^\^\(0\|\[1-9\]\[0-9\]\*\)\$$/);
  assert.equal(totals.properties.net_points.pattern,"^(0|-?[1-9][0-9]*)$");
  assert.deepEqual(schemas.CommissionReportGroup.properties.totals,{$ref:"#/components/schemas/CommissionReportTotals"});
  assert.deepEqual(schemas.CommissionReport.properties.query.properties.group_by.enum,["day","agent","cycle"]);
  assert.match(totals.description,/net_points may be negative/);
  assert.match(totals.description,/not bet\/cycle cohort/);
});

test("commission report routes require explicit scoped permissions and export all groups",()=>{
  const view=paths["/api/v1/admin/reports/commission"].get;
  const exp=paths["/api/v1/admin/reports/commission.csv"].get;
  assert.deepEqual(view["x-permissions"],["report_commission.view.brand","report_commission.view.platform"]);
  assert.deepEqual(exp["x-permissions"],["report_commission.view.brand","report_commission.view.platform","report_commission.export.brand","report_commission.export.platform"]);
  assert.deepEqual(view.parameters.filter(p=>p.in==="query").map(p=>p.name),["from","to","group_by","agent_id","member_id","cycle_id","limit","offset"]);
  assert.deepEqual(exp.parameters.filter(p=>p.in==="query").map(p=>p.name),["from","to","group_by","agent_id","member_id","cycle_id"]);
  assert.equal(view.parameters.find(p=>p.name==="limit").schema.default,20);
  assert.ok(exp.responses["200"].content["text/csv"]);
  assert.ok(exp.responses["413"]);
  assert.match(exp.description,/apostrophe-prefixed/);
  assert.match(exp.description,/zero-group export contains its summary row/);
});
