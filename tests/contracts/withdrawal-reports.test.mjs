import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";

const doc=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const paths=doc.paths,schemas=doc.components.schemas;

test("withdrawal report schema preserves exact current-state projection",()=>{
  const totals=schemas.WithdrawalReportTotals;
  assert.equal(totals.additionalProperties,false);
  assert.deepEqual(Object.keys(totals.properties),["order_count","requested_points","reviewing_count","reviewing_points","processing_count","processing_points","paid_count","paid_points","rejected_count","rejected_points","failed_count","failed_points","cancelled_count","cancelled_points"]);
  for(const field of Object.values(totals.properties))assert.match(field.pattern,/^\^\(0\|\[1-9\]\[0-9\]\*\)\$$/);
  assert.deepEqual(schemas.WithdrawalReportGroup.properties.totals,{$ref:"#/components/schemas/WithdrawalReportTotals"});
  assert.equal(schemas.WithdrawalReport.properties.query.properties.game_id.type,"null");
  assert.deepEqual(schemas.WithdrawalReport.properties.query.properties.group_by.enum,["day","member","state"]);
});

test("withdrawal report route has separate explicit view/export grants and strict paging",()=>{
  const view=paths["/api/v1/admin/reports/withdrawal"].get;
  const exp=paths["/api/v1/admin/reports/withdrawal/export"].get;
  assert.deepEqual(view["x-permissions"],["report_withdrawal.view.brand","report_withdrawal.view.platform"]);
  assert.deepEqual(exp["x-permissions"],["report_withdrawal.view.brand","report_withdrawal.view.platform","report_withdrawal.export.brand","report_withdrawal.export.platform"]);
  assert.deepEqual(view.parameters.filter(p=>p.in==="query").map(p=>p.name),["from","to","group_by","member_id","limit","offset"]);
  assert.deepEqual(exp.parameters.filter(p=>p.in==="query").map(p=>p.name),["from","to","group_by","member_id"]);
  assert.equal(view.parameters.find(p=>p.name==="limit").schema.default,20);
  assert.equal(view.parameters.find(p=>p.name==="offset").schema.maximum,1000000);
  assert.ok(exp.responses["200"].content["text/csv"]);
  assert.match(view.description,/created_at/);
  assert.match(view.description,/not immutable closeout/i);
  assert.match(exp.description,/10000 groups or 4MiB/i);
});
