import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("only the six implemented immutable history reads advertise routing headers", async () => {
  const doc = JSON.parse(await readFile(new URL("../../docs/openapi.json", import.meta.url), "utf8"));
  const expected = new Set([
    "/api/v1/admin/audit",
    "/api/v1/admin/notification-templates/{key}/history",
    "/api/v1/admin/compliance-policy/history",
    "/api/v1/admin/brand-operation/history",
    "/api/v1/admin/brand-presentation/history",
    "/api/v1/admin/brand-domains/history",
    "/api/v1/platform/audit",
    "/api/v1/platform/notification-templates/{key}/history",
    "/api/v1/platform/compliance-policy/history",
    "/api/v1/platform/brand-operation/history",
    "/api/v1/platform/brand-presentation/history",
    "/api/v1/platform/brand-domains/history",
  ]);
  const actual = new Set();
  let routingHeaderCount = 0;
  for (const [path, item] of Object.entries(doc.paths)) {
    for (const [method, operation] of Object.entries(item)) {
      if (!operation.responses) continue;
      const headers = operation.responses["200"]?.headers;
      if (headers?.["X-Read-Source"]) {
        assert.equal(method, "get");
        actual.add(path);
        routingHeaderCount += 1;
        assert.deepEqual(headers["X-Read-Source"].schema.enum, ["primary", "replica"]);
        assert.equal(headers["X-Read-Replica"].schema.pattern, "^[1-8]$");
        assert.ok(!headers["X-Read-Reason"].schema.enum.includes("fence_unavailable"));
      }
      for (const [status, response] of Object.entries(operation.responses)) {
        if (Number(status) >= 400) assert.ok(!response.headers?.["X-Read-Source"]);
      }
      assert.ok(!operation.parameters.some(p => p.name.startsWith("X-Read-")), "routing is not a client control");
    }
  }
  assert.deepEqual(actual, expected);
  assert.equal(routingHeaderCount, 12);
});
