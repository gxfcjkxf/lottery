import { test } from "node:test";
import assert from "node:assert/strict";
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
ajv.addSchema({ $id: "https://example.test/workbench-contract", components: { schemas: doc.components.schemas } });
const validate = name => ajv.compile({ $ref: `https://example.test/workbench-contract#/components/schemas/${name}` });

const ready = data => ({ status: "ready", data });
const unavailable = () => ({ status: "forbidden", data: null });
const snapshot = () => ({
  brand_id: id, snapshot_at: "2026-10-06T12:30:00Z", timezone: "Asia/Singapore", day_from: "2026-10-05T16:00:00Z",
  brand: ready({ name: "Example", code: "example", state: "active" }),
  periods: ready({ pending: "0", betting: "1", closed: "0", waiting_draw: "0", drawn: "0", settling: "0", refund_pending: "0", refund_failed: "0" }),
  orders: ready({ placed: "0", abnormal: "0" }),
  today_bets: ready({ order_count: "99999999999999999999", stake_points: "99999999999999999999", cancelled_count: "0", abnormal_count: "0" }),
  settlement: ready({ processing: "0", awaiting_approval: "0", paying: "0", failed: "0" }),
  recharges: ready({ pending_count: "0", pending_points: "0" }),
  ledger: ready({ entry_count: "1", net_points: "-99999999999999999999", recharge_points: "0", prize_credit_points: "0", prize_reversal_points: "0", refund_points: "0" }),
  balances: ready({ account_count: "0", available_points: "0", frozen_points: "0", withdrawal_points: "0", total_points: "0" }),
  reconciliation: ready({ latest_job: null }),
  sources: ready({ adapter_state: "stub", configured_games: "0", enabled_api_sources: "0", enabled_dom_sources: "0", attempts_today: "0", failed_today: "0", no_data_today: "0", last_attempt_at: null }),
  withdrawals: ready({ reviewing_count: "2", reviewing_points: "500", processing_count: "1", processing_points: "900719925474099312345" }), commissions: unavailable(), rewards: ready({ granted_count: "1", pending_count: "2", revoked_count: "3" }),
});

test("workbench operation is authenticated, brand selected, and has no query or routing controls", () => {
  assert.equal(operation.operationId, "adminGetWorkbench");
  assert.deepEqual(operation.security, [{ adminBearer: [] }, { adminCookie: [] }]);
  assert.deepEqual(operation.parameters.filter(p => p.in === "header").map(p => [p.name, p.required]), [["X-Request-ID", false], ["X-Brand-ID", true]]);
  assert.deepEqual(operation.parameters.filter(p => p.in === "query"), []);
  assert.ok(!Object.keys(operation.responses["200"].headers).some(name => name.startsWith("X-Read-")));
  assert.ok(!Object.keys(operation.responses["200"].headers).includes("X-Read-Source"));
  assert.ok(operation.responses["401"]);
  const unauthorized = validate("ErrorResponse");
  assert.ok(unauthorized({ success: false, error: { code: "UNAUTHORIZED", message: "Authentication required" }, request_id: "r1" }));
  assert.ok(!unauthorized({ success: true, data: snapshot(), request_id: "r1" }));
});

test("authoritative workbench snapshot schema enforces ready data, unavailable nulls, strings, and closed DTOs", () => {
  const check = validate("AdminWorkbenchSnapshot");
  const value = snapshot();
  const sectionNames = Object.keys(doc.components.schemas.AdminWorkbenchSnapshot.properties);
  for (const name of sectionNames.slice(4)) {
    const sectionName = doc.components.schemas.AdminWorkbenchSnapshot.properties[name].$ref.split("/").at(-1);
    assert.deepEqual(doc.components.schemas[sectionName].properties.status.enum, ["ready", "forbidden"]);
  }
  assert.ok(check(value), JSON.stringify(check.errors));
  assert.ok(!check({ ...value, unexpected: true }));
  assert.ok(!check({ ...value, brand: { status: "ready", data: null } }));
  assert.ok(!check({ ...value, brand: { status: "forbidden", data: { name: "Example", code: "example", state: "active" } } }));
  assert.ok(!check({ ...value, brand: { status: "not_implemented", data: null } }));
  assert.ok(!check({ ...value, brand: { status: "ready", data: { name: "Example", code: "example", state: "unknown" } } }));
  assert.ok(!check({ ...value, orders: { status: "ready", data: { placed: 1, abnormal: "0" } } }));
  assert.ok(!check({ ...value, orders: { status: "ready", data: { placed: "01", abnormal: "0" } } }));
  assert.ok(!check({ ...value, ledger: { ...value.ledger, data: { ...value.ledger.data, net_points: "-0" } } }));
  assert.ok(!check({ ...value, ledger: { ...value.ledger, data: { ...value.ledger.data, unexpected: "x" } } }));
  assert.ok(!check({ ...value, reconciliation: ready({ latest_job: { id, state: "unknown", created_at: "2026-10-06T10:30:00Z", completed_at: null, target_count: "1", checked_count: "0", repairable_count: "0", corrupt_count: "0", failed_count: "0" } }) }));
  assert.ok(!check({ ...value, sources: ready({ ...value.sources.data, adapter_state: "connected" }) }));
  assert.ok(check({ ...value, withdrawals: { status: "forbidden", data: null } }));
  for (const amount of ["01", "-1", "1e3"]) {
    assert.ok(!check({ ...value, withdrawals: ready({ ...value.withdrawals.data, processing_points: amount }) }));
  }
  assert.ok(check({ ...value, rewards: { status: "forbidden", data: null } }));
  assert.ok(check({ ...value, rewards: unavailable() }));
  assert.ok(!check({ ...value, rewards: { status: "not_implemented", data: null } }));
  assert.ok(!check({ ...value, rewards: ready({ ...value.rewards.data, gift_points: "5" }) }));
  assert.ok(check({ ...value, commissions: unavailable() }));
  assert.ok(!check({ ...value, commissions: { status: "not_implemented", data: { count: "0" } } }));
  assert.ok(!check({ ...value, commissions: { status: "not_implemented", data: null } }));
  assert.ok(!check({ ...value, commissions: { status: "not_implemented", data: null, count: "0" } }));
  assert.ok(!check({ ...value, brand_id: "not-a-uuid" }));
});

test("route is exactly GET /api/v1/admin/workbench with no inferred brand path alias", () => {
  assert.deepEqual(Object.keys(doc.paths), [route]);
  assert.deepEqual(Object.keys(doc.paths[route]), ["get"]);
  assert.equal(operation.brandHeader, undefined);
  assert.deepEqual(operation.parameters.find(p => p.name === "X-Brand-ID").schema, { $ref: "#/components/schemas/UUID" });
});
