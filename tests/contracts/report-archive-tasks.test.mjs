import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { commonSchemas, composeDocument } from "../../scripts/openapi-lib.mjs";
import { operations, schemas } from "../../scripts/openapi-report-archive-tasks.mjs";

const backend = new URL("../../backend/", import.meta.url);
const go = process.env.LOTTERY_GO_BIN ?? "go";
const routes = [
  "GET /api/v1/admin/report-archive-policy",
  "GET /api/v1/admin/report-archive-tasks",
  "GET /api/v1/admin/report-archive-tasks/{id}",
  "POST /api/v1/admin/report-archive-tasks/{id}/retry",
];
const doc = composeDocument([{ schemas, operations }], routes.map(route => {
  const [method, path] = route.split(" "); return { method, path };
}));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: "urn:lottery:report-archive-tasks", components: { schemas: { ...commonSchemas, ...schemas } } });
const validate = name => ajv.compile({ $ref: `urn:lottery:report-archive-tasks#/components/schemas/${name}` });

test("task fragment documents only the four admin contracts with explicit brand scope", () => {
  assert.deepEqual(operations.map(({ method, path }) => `${method} ${path}`).sort(), [...routes].sort());
  assert.deepEqual(Object.keys(schemas).sort(), ["ReportArchivePolicy", "ReportArchiveTask", "ReportArchiveTaskPage", "ReportArchiveTaskRetryInput", "ReportArchiveTaskWindow"].sort());
  for (const operation of operations) {
    const openapi = doc.paths[operation.path][operation.method.toLowerCase()];
    assert.equal(operation.brandHeader, true);
    assert.deepEqual(openapi.parameters.find(p => p.name === "X-Brand-ID").schema, { $ref: "#/components/schemas/UUID" });
    assert.equal(openapi.parameters.find(p => p.name === "X-Brand-ID").required, true);
    assert.equal(operation.path.startsWith("/api/v1/admin/"), true);
    assert.equal(operation.path.includes("{brandCode}"), false);
  }
  const retry = operations.find(op => op.method === "POST");
  assert.equal(retry.successStatus, 200);
  assert.deepEqual(retry.permissions, ["report_archive.view.brand", "report_archive_task.retry.brand"]);
  assert.equal(retry.idempotency, true);
  assert.equal(retry.parameters.find(p => p.name === "X-Report-Archive-Actor-ID").required, true);
  assert.equal(doc.paths[retry.path].post.parameters.find(p => p.name === "Idempotency-Key").required, true);
  assert.equal(schemas.ReportArchiveTaskRetryInput.additionalProperties, false);
  assert.equal(schemas.ReportArchiveTaskRetryInput.properties.version.maximum, 9007199254740990);
  const list = operations.find(op => op.path.endsWith("report-archive-tasks") && op.method === "GET");
  assert.deepEqual(list.parameters.filter(p => p.in === "query").map(p => [p.name, p.schema.default, p.schema.minimum, p.schema.maximum]), [
    ["limit", 20, 1, 100], ["offset", 0, 0, 1000000],
  ]);
  assert.match(operations[0].description, /Read only/);
  assert.match(retry.description, /does not claim the archive has completed/);
});

test("closed OpenAPI schemas reject unknown fields, unsafe versions, and invalid retry reasons", () => {
  const checkPolicy = validate("ReportArchivePolicy"), checkTask = validate("ReportArchiveTask");
  const checkPage = validate("ReportArchiveTaskPage"), checkRetry = validate("ReportArchiveTaskRetryInput");
  const validRetry = { version: 1, reason: "reviewed and retrying" };
  assert.ok(checkRetry(validRetry), JSON.stringify(checkRetry.errors));
  for (const value of [
    { ...validRetry, extra: true }, { version: 9007199254740991, reason: "reason" }, { version: 1, reason: " padded " },
    { version: 1, reason: "control\u0085" }, { version: 1, reason: "é".repeat(501) },
  ]) assert.ok(!checkRetry(value), JSON.stringify(value));
  assert.equal(schemas.ReportArchivePolicy.additionalProperties, false);
  assert.equal(schemas.ReportArchiveTask.additionalProperties, false);
  assert.equal(schemas.ReportArchiveTaskPage.additionalProperties, false);
  assert.equal(schemas.ReportArchiveTaskPage.properties.total_count.type, "string");
  assert.equal(schemas.ReportArchiveTaskPage.properties.total_count.pattern, "^(0|[1-9][0-9]*)$");
  assert.equal(checkPolicy({}), false);
  assert.equal(checkTask({}), false);
  assert.equal(checkPage({}), false);
  assert.equal(schemas.ReportArchiveTask.properties.version.maximum, 9007199254740991);
  assert.equal(schemas.ReportArchiveTask.properties.attempt_count.minimum, 0);
  assert.match(schemas.ReportArchiveTaskWindow.description, /must not recalculate boundaries/i);
  assert.match(schemas.ReportArchiveTask.description, /skipped tasks carry an archive_id/i);
  assert.match(schemas.ReportArchiveTask.description, /ARCHIVE_FAILED/);
  assert.match(schemas.ReportArchiveTaskPage.description, /max\(total_count - offset, 0\)/);
});

test("actual Go AutomaticPolicy, AutomaticTask, and AutomaticTaskPage marshals match the closed DTO schemas", () => {
  const dir = mkdtempSync(join(fileURLToPath(backend), "report-archive-task-contract-"));
  const source = `package main
import (
  "encoding/json"
  "fmt"
  "time"
  "github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
)
func main() {
  brand := "11111111-1111-4111-8111-111111111111"
  taskID := "22222222-2222-4222-8222-222222222222"
  audit := "33333333-3333-4333-8333-333333333333"
  now := time.Date(2026, 10, 7, 8, 9, 10, 123456789, time.UTC)
  start := "2026-10-06"
  policy := reportarchive.AutomaticPolicy{BrandID:brand,Version:1,DailyEnabled:false,MonthlyEnabled:false,DailyStartPeriod:nil,MonthlyStartPeriod:nil,Timezone:"Asia/Singapore",AuditLogID:nil,UpdatedAt:now}
  task := reportarchive.AutomaticTask{ID:taskID,BrandID:brand,PolicyVersion:2,Window:reportarchive.Window{Kind:"daily",PeriodKey:start,Timezone:"Asia/Singapore",From:time.Date(2026,10,5,16,0,0,0,time.UTC),To:time.Date(2026,10,6,16,0,0,0,time.UTC)},State:"pending",Version:1,AttemptCount:0,ArchiveID:nil,LastErrorCode:nil,CreationAuditLogID:audit,LastAuditLogID:audit,CreatedAt:now,UpdatedAt:now}
  page := reportarchive.AutomaticTaskPage{BrandID:brand,Items:[]reportarchive.AutomaticTask{task},TotalCount:"1",Limit:20,Offset:0}
  out := map[string]any{"policy":policy,"task":task,"page":page}
  raw, err := json.Marshal(out); if err != nil { panic(err) }; fmt.Println(string(raw))
}`;
  try {
    writeFileSync(join(dir, "main.go"), source);
    const run = spawnSync(go, ["run", "-buildvcs=false", "./" + relative(fileURLToPath(backend), dir)], {
      cwd: backend, encoding: "utf8", env: { ...process.env, CGO_ENABLED: "0" },
    });
    assert.equal(run.status, 0, run.stderr || run.error?.message);
    const values = JSON.parse(run.stdout);
    for (const [name, value] of [["ReportArchivePolicy", values.policy], ["ReportArchiveTask", values.task], ["ReportArchiveTaskPage", values.page]]) {
      const check = validate(name); assert.ok(check(value), `${name}: ${JSON.stringify(check.errors)}`);
      assert.deepEqual(Object.keys(value).sort(), Object.keys(schemas[name].properties).sort(), `${name} field set`);
    }
    assert.deepEqual(Object.keys(values.task.window).sort(), Object.keys(schemas.ReportArchiveTaskWindow.properties).sort());
    assert.deepEqual(Object.keys(values.page.items[0]).sort(), Object.keys(schemas.ReportArchiveTask.properties).sort());
    assert.equal(values.page.total_count, "1");
    const checkPolicy = validate("ReportArchivePolicy"), checkTask = validate("ReportArchiveTask");
    assert.ok(!checkPolicy({ ...values.policy, daily_enabled: true }), "initial policy cannot be enabled");
    assert.ok(!checkPolicy({ ...values.policy, audit_log_id: "44444444-4444-4444-8444-444444444444" }), "initial policy has a null audit id");
    assert.ok(!checkPolicy({ ...values.policy, version: 2 }), "later policy requires an audit id");
    assert.ok(checkPolicy({ ...values.policy, version: 2, audit_log_id: "44444444-4444-4444-8444-444444444444", monthly_start_period: "2024-02" }), "disabled policy may retain a saved start");
    const archiveId = "55555555-5555-4555-8555-555555555555";
    assert.ok(checkTask({ ...values.task, state: "skipped", version: 2, attempt_count: 1, archive_id: archiveId, last_error_code: null }), "skipped carries the existing archive");
    assert.ok(checkTask({ ...values.task, state: "failed", version: 2, attempt_count: 1, archive_id: null, last_error_code: "ARCHIVE_FAILED" }), "failed carries the fixed error code");
    assert.ok(!checkTask({ ...values.task, state: "skipped", version: 2, attempt_count: 1, archive_id: null, last_error_code: "ARCHIVE_FAILED" }));
    assert.ok(!checkTask({ ...values.task, state: "failed", version: 2, attempt_count: 1, archive_id: null, last_error_code: "OTHER" }));
    for (const [name, value] of [["ReportArchivePolicy", { ...values.policy, unknown: true }], ["ReportArchiveTask", { ...values.task, unknown: true }], ["ReportArchiveTaskPage", { ...values.page, unknown: true }]]) {
      assert.ok(!validate(name)(value), `${name} accepted an unknown field`);
    }
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
