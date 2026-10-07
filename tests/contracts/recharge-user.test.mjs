import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { composeDocument, documentedRoutes } from "../../scripts/openapi-lib.mjs";
import { operations, schemas } from "../../scripts/openapi-recharge-user.mjs";

const base = JSON.parse(readFileSync(new URL("../../docs/openapi.json", import.meta.url), "utf8"));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({
  $id: "urn:lottery:member-recharge-contract",
  components: { schemas: { ...base.components.schemas, ...schemas } },
});
const validate = name => ajv.compile({ $ref: `urn:lottery:member-recharge-contract#/components/schemas/${name}` });

const brand = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const member = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
const rechargeId = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
const ledgerEntry = "dddddddd-dddd-4ddd-8ddd-dddddddddddd";
const timestamp = "2026-10-08T01:02:03Z";

const recharge = (state, confirmedAt = null, ledgerEntryId = null) => ({
  id: rechargeId,
  brand_id: brand,
  member_id: member,
  points: "9007199254740993",
  state,
  version: "9007199254740993",
  created_at: timestamp,
  confirmed_at: confirmedAt,
  ledger_entry_id: ledgerEntryId,
});

test("documents exactly two canonical user reads and the builder's two brand aliases", () => {
  assert.deepEqual(operations.map(({ method, path }) => `${method} ${path}`).sort(), [
    "GET /api/v1/recharges",
    "GET /api/v1/recharges/{id}",
  ]);
  const routes = [
    { method: "GET", path: "/api/v1/recharges" },
    { method: "GET", path: "/api/v1/recharges/{id}" },
    { method: "GET", path: "/api/v1/b/{brandCode}/recharges" },
    { method: "GET", path: "/api/v1/b/{brandCode}/recharges/{id}" },
  ];
  const doc = composeDocument([{ schemas, operations }], routes);
  assert.deepEqual(documentedRoutes(doc), routes.map(route => `${route.method} ${route.path}`).sort());
  for (const path of ["/api/v1/recharges", "/api/v1/recharges/{id}", "/api/v1/b/{brandCode}/recharges", "/api/v1/b/{brandCode}/recharges/{id}"]) {
    const operation = doc.paths[path].get;
    assert.deepEqual(operation.security, [{ userBearer: [] }]);
    assert.ok(!operation.requestBody);
    assert.match(operation.description, /same-origin user authentication/i);
    assert.match(operation.description, /primary database/i);
    assert.match(operation.description, /reauthenticates in the transaction/i);
    assert.match(operation.description, /sanitized audit records in the same transaction/i);
    assert.match(operation.description, /503 without data/i);
  }
  assert.equal(doc.paths["/api/v1/b/{brandCode}/recharges"].get.operationId, "listMyRechargesByBrand");
  assert.equal(doc.paths["/api/v1/b/{brandCode}/recharges/{id}"].get.operationId, "getMyRechargeByBrand");
  assert.ok(!Object.keys(doc.paths).some(path => path.includes("/admin/recharges")));
});

test("list query contract is strict, member-scoped, and has exact defaults", () => {
  const list = operations.find(operation => operation.path === "/api/v1/recharges");
  assert.deepEqual(list.parameters.map(({ name, in: location, required }) => [name, location, required]), [
    ["limit", "query", false],
    ["offset", "query", false],
    ["state", "query", false],
  ]);
  assert.equal(list.parameters[0].schema.default, 20);
  assert.equal(list.parameters[0].schema.minimum, 1);
  assert.equal(list.parameters[0].schema.maximum, 100);
  assert.equal(list.parameters[1].schema.default, 0);
  assert.equal(list.parameters[1].schema.minimum, 0);
  assert.equal(list.parameters[1].schema.maximum, 1000000);
  assert.deepEqual(list.parameters[2].schema.enum, ["pending", "confirmed", "cancelled"]);
  assert.match(list.description, /unknown, empty, duplicate, and noncanonical numeric query values/i);
  assert.match(list.description, /ForceQuery, and request bodies are rejected/i);
  assert.match(list.description, /There is no member selector/i);

  const detail = operations.find(operation => operation.path === "/api/v1/recharges/{id}");
  assert.ok(!detail.parameters?.some(parameter => parameter.in === "query"));
  assert.match(detail.description, /no query string, including an empty ForceQuery/i);
  assert.match(detail.description, /another member's or brand's record returns 404/i);
});

test("DTOs are closed, exact, and exclude private recharge and ledger data", () => {
  assert.deepEqual(Object.keys(schemas.MemberRecharge.properties), [
    "id", "brand_id", "member_id", "points", "state", "version", "created_at", "confirmed_at", "ledger_entry_id",
  ]);
  assert.deepEqual(Object.keys(schemas.MemberRechargePage.properties), [
    "brand_id", "member_id", "snapshot_at", "state", "items", "limit", "offset", "total_count",
  ]);
  for (const schema of Object.values(schemas)) assert.equal(schema.additionalProperties, false);
  assert.deepEqual(schemas.MemberRecharge.properties.points, { $ref: "#/components/schemas/PositiveInt64String" });
  assert.deepEqual(schemas.MemberRecharge.properties.version, { $ref: "#/components/schemas/PositiveInt64String" });
  assert.deepEqual(schemas.MemberRecharge.properties.confirmed_at.anyOf, [
    { type: "string", format: "date-time", pattern: "Z$", description: "RFC3339 timestamp in UTC, serialized with the Z suffix." },
    { type: "null" },
  ]);
  assert.deepEqual(schemas.MemberRecharge.properties.ledger_entry_id.anyOf, [
    { $ref: "#/components/schemas/UUID" }, { type: "null" },
  ]);
  assert.deepEqual(schemas.MemberRechargePage.properties.state.anyOf, [
    { type: "string", enum: ["pending", "confirmed", "cancelled"] }, { type: "null" },
  ]);
  assert.deepEqual(schemas.MemberRechargePage.properties.total_count, { $ref: "#/components/schemas/NonnegativeInt64String" });
  assert.match(schemas.MemberRecharge.description, /account identifiers, proof references, remarks, administrator identity, reasons, audit data, and full ledger entries/);
  assert.match(schemas.MemberRechargePage.description, /current-state projection/i);
  assert.match(schemas.MemberRechargePage.description, /created_at descending then id descending/i);
  assert.match(schemas.MemberRechargePage.description, /not a historical snapshot, payment receipt, or current balance/i);
});

test("recharge DTO validation enforces confirmation nullability and preserves precise decimal strings", () => {
  const checkRecharge = validate("MemberRecharge");
  const checkPage = validate("MemberRechargePage");
  const pending = recharge("pending");
  const cancelled = recharge("cancelled");
  const confirmed = recharge("confirmed", timestamp, ledgerEntry);

  for (const value of [pending, cancelled, confirmed]) assert.ok(checkRecharge(value), JSON.stringify(checkRecharge.errors));
  assert.ok(!checkRecharge(recharge("confirmed")), "confirmed records require both confirmation fields");
  assert.ok(!checkRecharge(recharge("pending", timestamp, ledgerEntry)), "pending records require both confirmation fields to be null");
  assert.ok(!checkRecharge(recharge("cancelled", timestamp, null)), "cancelled records require both confirmation fields to be null");
  assert.ok(!checkRecharge({ ...pending, points: 9007199254740993 }), "points must remain a decimal string, never a lossy JS number");
  assert.ok(!checkRecharge({ ...pending, version: 9007199254740993 }), "version must remain a decimal string");
  assert.ok(!checkRecharge({ ...pending, confirmed_at: "2026-10-08T01:02:03+00:00" }), "timestamps must use UTC Z notation");
  assert.ok(!checkRecharge({ ...pending, proof_reference: "private" }));
  assert.ok(!checkRecharge({ ...pending, account_id: brand }));

  const page = {
    brand_id: brand,
    member_id: member,
    snapshot_at: timestamp,
    state: null,
    items: [pending, confirmed],
    limit: 20,
    offset: 0,
    total_count: "9007199254740993",
  };
  assert.ok(checkPage(page), JSON.stringify(checkPage.errors));
  assert.ok(!checkPage({ ...page, total_count: 9007199254740993 }), "counts must retain exact decimal strings");
  assert.ok(!checkPage({ ...page, state: "" }));
  assert.ok(!checkPage({ ...page, private_audit_id: brand }));
});

test("documentation states this is not a customer recharge or payment API", () => {
  const combined = JSON.stringify({ operations, schemas });
  assert.match(combined, /No user-facing create, confirm, or payment endpoint is exposed/i);
  assert.ok(!operations.some(operation => operation.method !== "GET"));
});
