import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const doc = JSON.parse(readFileSync(new URL("../../docs/openapi.json", import.meta.url), "utf8"));
const ajv = new Ajv2020({ strict: false, allErrors: true }); addFormats(ajv);
ajv.addSchema({ $id: "urn:lottery:attribution-reports", components: doc.components });
const check = name => ajv.compile({ $ref: `urn:lottery:attribution-reports#/components/schemas/${name}` });

test("attribution report exposes the closed read and export query contracts", () => {
  const path = "/api/v1/admin/reports/attribution";
  const view = doc.paths[path].get;
  const exp = doc.paths[`${path}/export`].get;
  assert.deepEqual(view["x-permissions"], ["report_attribution.view.brand", "report_attribution.view.platform"]);
  assert.deepEqual(exp["x-permissions"], [...view["x-permissions"], "report_attribution.export.brand", "report_attribution.export.platform"]);
  assert.deepEqual(view.parameters.filter(p => p.in === "query").map(p => p.name), ["from", "to", "group_by", "game_id", "member_id", "agent_id", "agent_scope", "join_method", "limit", "offset"]);
  assert.deepEqual(exp.parameters.filter(p => p.in === "query").map(p => p.name), ["from", "to", "group_by", "game_id", "member_id", "agent_id", "agent_scope", "join_method"]);
  assert.ok(exp.responses["413"]);
  assert.ok(exp.responses["200"].content["text/csv"]);
  assert.ok(exp.responses["200"].headers["X-Report-Agent-Scope"]);
  assert.ok(exp.responses["200"].headers["X-Report-Join-Method"]);
  assert.match(view.description, /GET bodies are rejected/);
  assert.equal(view.requestBody, undefined);
  assert.match(view.description, /immutable BET attribution_snapshot/);
  assert.match(view.description, /does not authorize commission payouts/);
  assert.match(exp.description, /10000 groups or 4MiB/);
});

test("real Go attribution report DTOs match the closed schema and exact decimal fields", () => {
  const run = spawnSync(process.env.LOTTERY_GO_BIN ?? "go", ["run", "-buildvcs=false", "./cmd/contract-examples"], { cwd: new URL("../../backend/", import.meta.url), encoding: "utf8", env: { ...process.env, CGO_ENABLED: "0" } });
  assert.equal(run.status, 0, run.stderr || run.error?.message);
  const examples = JSON.parse(run.stdout);
  const value = examples.AttributionReport;
  assert.ok(value, "contract-examples must serialize the real attribution report DTO");
  const validate = check("AttributionReport");
  assert.ok(validate(value), JSON.stringify(validate.errors));
  assert.ok(!validate({ ...value, financial_authorization: true }));
  assert.ok(!validate({ ...value, summary: { ...value.summary, unexpected: "1" } }));
  assert.ok(!validate({ ...value, query: { ...value.query, agent_scope: "all" } }));
  for (const field of Object.keys(value.summary)) {
    assert.ok(!validate({ ...value, summary: { ...value.summary, [field]: 1 } }));
    assert.ok(!validate({ ...value, summary: { ...value.summary, [field]: "01" } }));
  }
  assert.equal(Object.keys(value.summary).length, 14);
  assert.equal(value.query.agent_scope, "direct");
  assert.equal(value.query.join_method, null);
});
