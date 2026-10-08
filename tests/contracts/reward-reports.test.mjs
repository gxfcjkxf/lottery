import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
const doc = JSON.parse(readFileSync(new URL("../../docs/openapi.json", import.meta.url), "utf8"));
const ajv = new Ajv2020({ strict: false, allErrors: true }); addFormats(ajv);
ajv.addSchema({ $id: "urn:lottery:reward-reports", components: doc.components });
const check = name => ajv.compile({ $ref: `urn:lottery:reward-reports#/components/schemas/${name}` });
test("reward reports expose independent read-only JSON and complete audited CSV contracts", () => {
  for (const path of ["rewards", "reward-orders"]) {
    const view = doc.paths[`/api/v1/admin/reports/${path}`].get;
    const exp = doc.paths[`/api/v1/admin/reports/${path}.csv`].get;
    assert.deepEqual(view["x-permissions"], ["report_reward.view.brand", "report_reward.view.platform"]);
    assert.deepEqual(exp["x-permissions"], [...view["x-permissions"], "report_reward.export.brand", "report_reward.export.platform"]);
    assert.deepEqual(view.parameters.filter(p => p.in === "query").map(p => p.name), ["from", "to", "group_by", "member_id", "order_id", "limit", "offset"]);
    assert.deepEqual(exp.parameters.filter(p => p.in === "query").map(p => p.name), ["from", "to", "group_by", "member_id", "order_id"]);
    assert.ok(exp.responses["413"]);
    assert.ok(exp.responses["200"].content["text/csv"]);
    for (const header of ["X-Report-Brand-ID", "X-Report-Kind", "X-Report-Snapshot-At", "X-Report-Timezone", "X-Report-SHA256", "X-Report-Byte-Count", "X-Report-Group-Count", "X-Report-Audit-ID", "X-Report-From", "X-Report-To", "X-Report-Group-By"]) assert.ok(exp.responses["200"].headers[header]);
    assert.equal(view.requestBody, undefined);
    assert.match(exp.description, /10000 groups or 4MiB/);
    assert.match(exp.description, /after audit waits/);
  }
});
test("real Go reward report DTOs preserve signed and above-int64 totals in closed schemas", () => {
  const run = spawnSync(process.env.LOTTERY_GO_BIN ?? "go", ["run", "-buildvcs=false", "./cmd/contract-examples"], { cwd: new URL("../../backend/", import.meta.url), encoding: "utf8", env: { ...process.env, CGO_ENABLED: "0" } });
  assert.equal(run.status, 0, run.stderr);
  const examples = JSON.parse(run.stdout);
  for (const name of ["RewardReport", "RewardOrderReport"]) {
    const value = examples[name], validate = check(name);
    assert.ok(validate(value), JSON.stringify(validate.errors));
    assert.ok(!validate({ ...value, wallet: "private" }));
    assert.ok(!validate({ ...value, summary: { ...value.summary, hidden: "value" } }));
    assert.ok(!validate({ ...value, query: { ...value.query, group_by: "agent" } }));
    for (const field of Object.keys(value.summary)) {
      assert.ok(!validate({ ...value, summary: { ...value.summary, [field]: 1 } }));
      assert.ok(!validate({ ...value, summary: { ...value.summary, [field]: "01" } }));
    }
  }
  assert.equal(examples.RewardReport.summary.net_points, "-9223372036854775807");
  assert.equal(examples.RewardOrderReport.summary.original_points, "18446744073709551617");
  assert.match(examples.RewardReport.query.from, /\.123456789Z$/);
});
