import { test } from "node:test";
import assert from "node:assert/strict";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { composeDocument } from "../../scripts/openapi-lib.mjs";
import { operations, schemas } from "../../scripts/openapi-report-archive-policy.mjs";
import { schemas as taskSchemas } from "../../scripts/openapi-report-archive-tasks.mjs";

const route = "PUT /api/v1/admin/report-archive-policy";
const doc = composeDocument([{ schemas: { ...taskSchemas, ...schemas }, operations }], [route].map(value => {
  const [method, path] = value.split(" "); return { method, path };
}));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: "urn:lottery:report-archive-policy", components: { schemas: { ...taskSchemas, ...schemas } } });
const validate = name => ajv.compile({ $ref: `urn:lottery:report-archive-policy#/components/schemas/${name}` });

test("policy write fragment documents only the scoped idempotent PUT", () => {
  assert.deepEqual(operations.map(({ method, path }) => `${method} ${path}`), [route]);
  assert.deepEqual(Object.keys(schemas), ["ReportArchivePolicyUpdateInput"]);
  const operation = doc.paths["/api/v1/admin/report-archive-policy"].put;
  assert.equal(operation.operationId, "updateReportArchiveAutomaticPolicy");
  assert.deepEqual(operation.security, [{ adminBearer: [] }, { adminCookie: [] }]);
  assert.deepEqual(operation["x-permissions"], ["report_archive.view.brand", "report_archive_policy.write.brand"]);
  assert.equal(operation["x-idempotent-operation"], true);
  assert.equal(operation.responses["200"].content["application/json"].schema.properties.data.$ref, "#/components/schemas/ReportArchivePolicy");
  assert.equal(operation.parameters.find(parameter => parameter.name === "X-Brand-ID").required, true);
  assert.equal(operation.parameters.find(parameter => parameter.name === "Idempotency-Key").required, true);
  assert.equal(operation.parameters.find(parameter => parameter.name === "X-Report-Archive-Actor-ID").required, true);
  assert.match(operation.description, /current brand membership/i);
  assert.match(operation.description, /non-super-admin/i);
  assert.match(operation.description, /version\+1/i);
  assert.match(operation.description, /same body and idempotency key/i);
  assert.match(operation.description, /does not automatically replay/i);
  assert.match(operation.description, /database clock/i);
  assert.match(operation.description, /current brand-local day/i);
  assert.match(operation.description, /current brand-local month/i);
  assert.match(operation.description, /only after that period ends/i);
  assert.match(operation.description, /first activation does not backfill earlier periods/i);
  assert.match(operation.description, /retains any existing starts/i);
});

test("closed input schema rejects extra fields, invalid versions, flags, and reasons", () => {
  const schema = schemas.ReportArchivePolicyUpdateInput;
  const check = validate("ReportArchivePolicyUpdateInput");
  const valid = { version: 4, daily_enabled: true, monthly_enabled: false, reason: "Enable daily archives" };
  assert.ok(check(valid), JSON.stringify(check.errors));
  assert.equal(schema.additionalProperties, false);
  assert.deepEqual(schema.required, ["version", "daily_enabled", "monthly_enabled", "reason"]);
  assert.equal(schema.properties.version.maximum, 9007199254740990);
  assert.equal(schema.properties.daily_enabled.type, "boolean");
  assert.equal(schema.properties.monthly_enabled.type, "boolean");
  for (const value of [
    { ...valid, extra: true }, { version: 0, daily_enabled: true, monthly_enabled: false, reason: "Reason" },
    { ...valid, version: 9007199254740991 }, { ...valid, daily_enabled: 1 }, { ...valid, monthly_enabled: "false" },
    { ...valid, reason: " padded " }, { ...valid, reason: "control\u0085" }, { ...valid, reason: "x".repeat(501) },
  ]) assert.ok(!check(value), JSON.stringify({ value, errors: check.errors }));
  assert.match(schema.properties.reason.description, /500 UTF-8 bytes/);
  assert.deepEqual(Object.keys(schema.properties), ["version", "daily_enabled", "monthly_enabled", "reason"]);
});
