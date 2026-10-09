import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { composeDocument } from "../../scripts/openapi-lib.mjs";
import { operations, schemas } from "../../scripts/openapi-workbench.mjs";

const route = "/api/v1/admin/workbench";
const id = "11111111-1111-4111-8111-111111111111";
const doc = composeDocument([{ operations, schemas }], [{ method: "GET", path: route }]);
const operation = doc.paths[route].get;
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: "https://example.test/workbench-commissions", components: { schemas: doc.components.schemas } });
const validate = name => ajv.compile({ $ref: `https://example.test/workbench-commissions#/components/schemas/${name}` });

const keys = [
  "discovery_pending_count", "discovery_failed_count",
  "cycle_processing_count", "cycle_waiting_count", "cycle_ready_count", "cycle_stale_count", "cycle_failed_count",
  "payment_awaiting_approval_count", "payment_processing_count", "payment_blocked_count", "payment_failed_count",
  "plan_processing_count", "plan_ready_count", "plan_blocked_count", "plan_failed_count",
  "execution_awaiting_approval_count", "execution_processing_count", "execution_paused_count", "execution_failed_count",
];
const zeroCounts = () => Object.fromEntries(keys.map(key => [key, "0"]));
const ready = data => ({ status: "ready", data });
const snapshot = () => ({
  brand_id: id,
  snapshot_at: "2026-10-06T12:30:00Z",
  timezone: "Asia/Singapore",
  day_from: "2026-10-05T16:00:00Z",
  brand: { status: "ready", data: { name: "Example", code: "example", state: "active" } },
  periods: { status: "forbidden", data: null },
  orders: { status: "forbidden", data: null },
  today_bets: { status: "forbidden", data: null },
  settlement: { status: "forbidden", data: null },
  recharges: { status: "forbidden", data: null },
  ledger: { status: "forbidden", data: null },
  balances: { status: "forbidden", data: null },
  reconciliation: { status: "forbidden", data: null },
  sources: { status: "forbidden", data: null },
  withdrawals: { status: "forbidden", data: null },
  commissions: ready(zeroCounts()),
  rewards: { status: "forbidden", data: null },
});

test("commissions DTO is closed and uses exactly the all-current aggregate count keys", () => {
  const dto = doc.components.schemas.AdminWorkbenchCommissions;
  assert.ok(dto);
  assert.deepEqual(Object.keys(dto.properties), keys);
  assert.deepEqual(dto.required, keys);
  assert.equal(dto.additionalProperties, false);

  const check = validate("AdminWorkbenchCommissions");
  assert.ok(check(zeroCounts()), JSON.stringify(check.errors));
  const large = Object.fromEntries(keys.map(key => [key, "999999999999999999999999999999999999999999"]));
  assert.ok(check(large), JSON.stringify(check.errors));
  for (const invalid of [1, 1.5, "01", "-1", "1e3", ""]) {
    assert.ok(!check({ ...zeroCounts(), discovery_pending_count: invalid }), `accepted ${JSON.stringify(invalid)}`);
  }
  assert.ok(!check({ ...zeroCounts(), unexpected_count: "0" }));
  const missing = zeroCounts();
  delete missing.execution_failed_count;
  assert.ok(!check(missing));
});

test("commissions section accepts current ready/forbidden states and nullable legacy snapshots only", () => {
  const check = validate("AdminWorkbenchCommissionsSection");
  assert.ok(check(ready(zeroCounts())), JSON.stringify(check.errors));
  assert.ok(check({ status: "forbidden", data: null }));
  assert.ok(check({ status: "not_implemented", data: null }), "old stored snapshot remains readable");
  for (const value of [
    { status: "ready", data: null },
    { status: "ready", data: { ...zeroCounts(), extra: "0" } },
    { status: "forbidden", data: zeroCounts() },
    { status: "not_implemented", data: zeroCounts() },
    { status: "unknown", data: null },
    { status: "not_implemented", data: null, extra: true },
  ]) assert.ok(!check(value), JSON.stringify(value));

  const checkSnapshot = validate("AdminWorkbenchSnapshot");
  assert.ok(checkSnapshot(snapshot()), JSON.stringify(checkSnapshot.errors));
  assert.ok(checkSnapshot({ ...snapshot(), commissions: { status: "not_implemented", data: null } }));
});

test("commissions are independently gated by exact brand or explicit platform read grants on the existing read route", () => {
  assert.deepEqual(Object.keys(doc.paths), [route]);
  assert.deepEqual(Object.keys(doc.paths[route]), ["get"]);
  assert.equal(operation.operationId, "adminGetWorkbench");
  assert.deepEqual(operation.security, [{ adminBearer: [] }, { adminCookie: [] }]);
  assert.deepEqual(operation.parameters.filter(parameter => parameter.in === "query"), []);
  assert.deepEqual(operation["x-permissions"], [
    "brand.view.brand", "period.view.brand", "bet.view.brand", "report_betting.view.brand", "settlement.view.brand",
    "recharge.view.brand", "report_ledger.view.brand", "wallet.view.brand", "draw_source.view.brand", "withdrawal.view.brand",
    "brand.view.platform", "period.view.platform", "bet.view.platform", "report_betting.view.platform", "settlement.view.platform",
    "recharge.view.platform", "report_ledger.view.platform", "wallet.view.platform", "draw_source.view.platform", "withdrawal.view.platform",
    "reward.view.brand", "reward.view.platform", "commission.view.brand", "commission.view.platform",
  ]);
  assert.ok(operation["x-permissions"].includes("commission.view.brand"));
  assert.ok(operation["x-permissions"].includes("commission.view.platform"));
  assert.deepEqual(operation["x-permissions"].filter(permission => permission.startsWith("commission.")), [
    "commission.view.brand", "commission.view.platform",
  ]);
  assert.ok(!operation["x-permissions"].some(permission => /report_commission|super|identity/i.test(permission)));
  assert.ok(!operation["x-permissions"].some(permission => permission.startsWith("commission.") && !permission.startsWith("commission.view.")));

  const section = doc.components.schemas.AdminWorkbenchCommissionsSection;
  assert.match(section.description, /commission\.view\.platform/);
  assert.match(section.description, /exact selected brand/);
  assert.match(operation.description, /cycle_ready_count includes only ready runs matching the current evidence epoch/);
  assert.match(operation.description, /cycle_stale_count includes ready runs without matching proof and is disjoint/);
  assert.match(operation.description, /discovery_pending_count includes future waiting work/);
  assert.match(operation.description, /does not establish that funds are owed/);
  assert.match(operation.description, /not today's earnings, wallet values or payout authorization/);
  assert.match(operation.description, /only older stored snapshots may use not_implemented/);
});

test("authoritative Go workbench examples serialize current and legacy commissions under their actual keys", () => {
  const result = spawnSync(process.env.LOTTERY_GO_BIN ?? "go", ["run", "-buildvcs=false", "./cmd/contract-examples"], {
    cwd: new URL("../../backend/", import.meta.url),
    encoding: "utf8",
    env: { ...process.env, CGO_ENABLED: "0" },
  });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  const examples = JSON.parse(result.stdout);
  assert.ok(examples.AdminWorkbench);
  assert.ok(examples.AdminWorkbenchLegacy);
  assert.equal(examples.AdminWorkbenchSnapshot, undefined);
  assert.equal(examples.AdminWorkbenchLegacySnapshot, undefined);

  const current = examples.AdminWorkbench.commissions;
  assert.equal(current.status, "ready");
  assert.deepEqual(Object.keys(current.data), keys);
  assert.equal(current.data.cycle_ready_count, "9007199254740993");
  assert.ok(validate("AdminWorkbenchSnapshot")(examples.AdminWorkbench));

  const legacy = examples.AdminWorkbenchLegacy.commissions;
  assert.deepEqual(legacy, { status: "not_implemented", data: null });
  assert.ok(validate("AdminWorkbenchSnapshot")(examples.AdminWorkbenchLegacy));
});
