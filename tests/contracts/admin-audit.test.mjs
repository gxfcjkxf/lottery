import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import * as identity from "../../docs/openapi/identity.mjs";
import { commonSchemas, composeDocument } from "../../scripts/openapi-lib.mjs";

const auditOperations = identity.operations.filter(operation => operation.path === "/api/v1/admin/audit" || operation.path === "/api/v1/admin/audit/export");
const routes = auditOperations.map(({ method, path }) => ({ method, path }));
const document = composeDocument([{ schemas: identity.schemas, operations: auditOperations }], routes);
const examples = () => {
  const result = spawnSync(process.env.LOTTERY_GO_BIN ?? "go", ["run", "-buildvcs=false", "./cmd/contract-examples"], {
    cwd: new URL("../../backend/", import.meta.url), encoding: "utf8", env: { ...process.env, CGO_ENABLED: "0" },
  });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  return JSON.parse(result.stdout);
};

test("audit list keeps its items envelope and adds only an optional nullable brand_id", () => {
  const record = identity.schemas.AdminAuditEntry;
  assert.equal(record.additionalProperties, false);
  assert.ok(!record.required.includes("brand_id"));
  assert.deepEqual(record.properties.brand_id, { anyOf: [{ $ref: "#/components/schemas/UUID" }, { type: "null" }] });
  assert.deepEqual(identity.schemas.AdminAuditList.required, ["items"]);
  const list = document.paths["/api/v1/admin/audit"].get;
  assert.deepEqual(list.responses["200"].content["application/json"].schema.properties.data, { $ref: "#/components/schemas/AdminAuditList" });
  assert.deepEqual(list.parameters.filter(parameter => parameter.in === "query").map(parameter => parameter.name), ["from", "to", "action", "actor_id", "resource_type", "resource_id", "request_id", "limit", "offset"]);
  assert.equal(list.parameters.find(parameter => parameter.name === "limit").schema.default, 100);
  assert.equal(list.parameters.find(parameter => parameter.name === "limit").schema.maximum, 200);
  assert.equal(list.parameters.find(parameter => parameter.name === "offset").schema.default, 0);
  assert.equal(list.parameters.find(parameter => parameter.name === "offset").schema.maximum, 100000);
  assert.match(list.description, /Unknown, duplicate, or empty query values are rejected with 400/);
  assert.match(list.description, /half-open \[from,to\)/);
  assert.match(list.description, /literal Z \(numeric offsets are rejected\)/);
  assert.match(list.description, /422 AUDIT_QUERY_TOO_LARGE as JSON and omit items/);
  assert.deepEqual(Object.keys(list.responses["422"].content), ["application/json"]);
  assert.equal(list.responses["422"].content["application/json"].schema.$ref, "#/components/schemas/ErrorResponse");
  assert.match(list.responses["422"].description, /JSON error.*no partial data/);
});

test("audit export declares a bounded raw CSV with exact authorization, filters, and receipt headers", () => {
  const operation = document.paths["/api/v1/admin/audit/export"].get;
  assert.deepEqual(operation["x-permissions"], ["audit.view.brand", "audit.view.platform", "audit.export.brand", "audit.export.platform"]);
  assert.equal(operation.parameters.find(parameter => parameter.name === "X-Brand-ID").required, true);
  assert.deepEqual(operation.parameters.filter(parameter => parameter.in === "query").map(parameter => parameter.name), ["from", "to", "action", "actor_id", "resource_type", "resource_id", "request_id"]);
  assert.ok(operation.parameters.filter(parameter => ["from", "to"].includes(parameter.name)).every(parameter => parameter.required));
  assert.ok(!operation.parameters.some(parameter => ["limit", "offset"].includes(parameter.name)));
  assert.deepEqual(Object.keys(operation.responses["200"].content), ["text/csv"]);
  assert.equal(operation.responses["422"].content["application/json"].schema.$ref, "#/components/schemas/ErrorResponse");
  for (const header of ["Content-Disposition", "X-Audit-Brand-ID", "X-Audit-Snapshot-At", "X-Audit-Row-Count", "X-Audit-SHA256", "X-Audit-Format-Version", "X-Audit-Export-ID", "Cache-Control", "X-Content-Type-Options"]) assert.ok(operation.responses["200"].headers[header], `missing ${header}`);
  assert.match(operation.description, /10000 rows or 4 MiB/);
  assert.match(operation.description, /AUDIT_EXPORT_TOO_LARGE/);
  assert.match(operation.description, /literal Z \(numeric offsets are rejected\)/);
  assert.deepEqual(Object.keys(operation.responses["422"].content), ["application/json"]);
  assert.match(operation.description, /committed before any CSV bytes/);
  assert.match(operation.description, /before starting a blob download/);
  assert.match(operation.description, /id, brand_id, action, actor_type, actor_id, resource_type, resource_id, reason, request_id, created_at, ip_address, before_json, after_json/);
});

test("Go audit contract examples validate both new and legacy audit records", () => {
  const ajv = new Ajv2020({ strict: false, allErrors: true });
  addFormats(ajv);
  ajv.addSchema({ $id: "urn:lottery:admin-audit-contract", components: { schemas: { ...commonSchemas, ...identity.schemas } } });
  const schema = name => ajv.compile({ $ref: `urn:lottery:admin-audit-contract#/components/schemas/${name}` });
  const values = examples();
  assert.ok(schema("AdminAuditEntry")(values.AdminAuditRecord), JSON.stringify(schema("AdminAuditEntry").errors));
  assert.ok(schema("AdminAuditEntry")(values.AdminAuditRecordLegacy), JSON.stringify(schema("AdminAuditEntry").errors));
  assert.ok(schema("AdminAuditList")(values.AdminAuditList), JSON.stringify(schema("AdminAuditList").errors));
  assert.equal(Object.hasOwn(values.AdminAuditRecordLegacy, "brand_id"), false);
  assert.equal(values.AdminAuditRecord.brand_id, "22222222-2222-4222-8222-222222222222");
});
