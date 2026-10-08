import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { commonSchemas, composeDocument } from "../../scripts/openapi-lib.mjs";
import { operations, schemas } from "../../scripts/openapi-report-archives.mjs";

const backend = new URL("../../backend/", import.meta.url);
const go = process.env.LOTTERY_GO_BIN ?? "go";
const expectedRoutes = [
  "GET /api/v1/admin/report-archives",
  "POST /api/v1/admin/report-archives",
  "GET /api/v1/admin/report-archives/{id}",
  "GET /api/v1/admin/report-archives/{id}/download",
];
const declaredDoc = composeDocument([{ schemas, operations }], expectedRoutes.map(route => {
  const [method, path] = route.split(" ");
  return { method, path };
}));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: "urn:lottery:report-archives", components: { schemas: { ...commonSchemas, ...schemas } } });
const validate = name => ajv.compile({ $ref: `urn:lottery:report-archives#/components/schemas/${name}` });

test("report archive fragment documents exactly the four bounded admin routes", () => {
  assert.deepEqual(operations.map(({ method, path }) => `${method} ${path}`).sort(), [...expectedRoutes].sort());
  assert.equal(Object.keys(declaredDoc.paths).length, 3);
  const routeRun = spawnSync(go, ["run", "-buildvcs=false", "./cmd/route-inventory"], {
    cwd: backend, encoding: "utf8", env: { ...process.env, CGO_ENABLED: "0" },
  });
  assert.equal(routeRun.status, 0, routeRun.stderr || routeRun.error?.message);
  const actual = new Set(JSON.parse(routeRun.stdout).map(({ method, path }) => `${method} ${path}`));
  for (const route of expectedRoutes) assert.ok(actual.has(route), `backend route missing: ${route}`);
  const doc = JSON.parse(readFileSync(new URL("../../docs/openapi.json", import.meta.url), "utf8"));
  for (const route of expectedRoutes) {
    const [, path] = route.split(" ");
    assert.ok(doc.paths[path], `generated docs route missing: ${route}`);
  }
});

test("archive operations preserve exact scope, paging, mutation, and download rules", () => {
  const byRoute = new Map(operations.map(op => [`${op.method} ${op.path}`, op]));
  const list = byRoute.get("GET /api/v1/admin/report-archives");
  const create = byRoute.get("POST /api/v1/admin/report-archives");
  const detail = byRoute.get("GET /api/v1/admin/report-archives/{id}");
  const download = byRoute.get("GET /api/v1/admin/report-archives/{id}/download");
  for (const operation of [list, create, detail, download]) {
    assert.equal(operation.brandHeader, true);
    assert.deepEqual(operation.permissions.slice(0, 2), ["report_archive.view.brand", "report_archive.view.platform"]);
    const openapiOperation = declaredDoc.paths[operation.path][operation.method.toLowerCase()];
    const brandHeader = openapiOperation.parameters.find(p => p.name === "X-Brand-ID");
    assert.equal(brandHeader.required, true);
    assert.deepEqual(brandHeader.schema, { $ref: "#/components/schemas/UUID" });
  }
  for (const operation of [list, detail, download]) {
    assert.equal(operation.requestBody, undefined, "GET must have no body");
    assert.equal(declaredDoc.paths[operation.path].get.requestBody, undefined, "GET must have no body");
  }
  assert.deepEqual(list.parameters.filter(p => p.in === "query").map(p => p.name), ["limit", "offset"]);
  assert.deepEqual(list.parameters.find(p => p.name === "limit").schema, { type: "integer", minimum: 1, maximum: 100, default: 20 });
  assert.equal(list.parameters.find(p => p.name === "offset").schema.maximum, 1000000);
  assert.equal(create.successStatus, 201);
  assert.equal(create.idempotency, true);
  assert.deepEqual(create.permissions, ["report_archive.view.brand", "report_archive.view.platform", "report_archive.create.brand"]);
  assert.equal(create.parameters.find(p => p.name === "X-Report-Archive-Actor-ID").required, true);
  assert.deepEqual(create.parameters.find(p => p.name === "X-Report-Archive-Actor-ID").schema, { $ref: "#/components/schemas/UUID" });
  assert.equal(declaredDoc.paths[create.path].post.parameters.find(p => p.name === "Idempotency-Key").required, true);
  assert.equal(schemas.ReportArchiveCreateInput.additionalProperties, false);
  assert.deepEqual(Object.keys(schemas.ReportArchiveCreateInput.properties), ["kind", "period_key", "expected_revision", "reason"]);
  assert.equal(schemas.ReportArchiveCreateInput.properties.expected_revision.maximum, 9007199254740990);
  const validateInput = validate("ReportArchiveCreateInput");
  const validInput = { kind: "daily", period_key: "2026-10-07", expected_revision: 0, reason: "monthly reporting" };
  assert.ok(validateInput(validInput), JSON.stringify(validateInput.errors));
  assert.ok(!validateInput({ ...validInput, from: "2026-10-07T00:00:00Z", to: "2026-10-08T00:00:00Z" }));
  assert.ok(!validateInput({ ...validInput, kind: "monthly" }));
  assert.ok(!validateInput({ ...validInput, expected_revision: 9007199254740991 }));
  assert.ok(!validateInput({ ...validInput, reason: " padded " }));
  assert.ok(!validateInput({ ...validInput, reason: "control\ncharacter" }));
  assert.ok(!validateInput({ ...validInput, reason: "x".repeat(501) }));
  assert.equal(schemas.ReportArchiveRecord.properties.revision.minimum, 1);
  assert.equal(schemas.ReportArchiveRecord.properties.revision.maximum, 9007199254740991);
  for (const name of ["ReportArchiveLedgerTotals", "ReportArchiveCommissionTotals", "ReportArchiveRewardTotals"])
    assert.equal(schemas[name].properties.net_points.pattern, "^(0|-?[1-9][0-9]*)$");
  assert.equal(download.permissions.includes("report_archive.download.brand"), true);
  assert.equal(download.permissions.includes("report_archive.download.platform"), true);
  assert.deepEqual(Object.keys(download.successContent), ["application/json"]);
  assert.match(download.successDescription, /audit commit.*reauthentication/i);
  for (const name of ["Content-Disposition", "Content-Length", "X-Content-SHA256", "X-Archive-Revision", "X-Archive-Format-Version"])
    assert.ok(download.successHeaders[name], `missing download header ${name}`);
  assert.equal(download.successHeaders["X-Archive-Format-Version"].schema.const, "1");
  assert.equal(download.successHeaders["X-Content-SHA256"].schema.pattern, "^[a-f0-9]{64}$");
  assert.equal(download.successHeaders["X-Archive-Revision"].schema.pattern, "^[1-9][0-9]*$");
  assert.match(list.description, /ForceQuery/);
  for (const operation of [list, create, detail, download]) {
    const expected = [...(operation.successStatus === 201 ? ["201"] : ["200"]), "400", "401", "403", "404", "409", "415", "429", "500", "503"].sort();
    assert.deepEqual(Object.keys(declaredDoc.paths[operation.path][operation.method.toLowerCase()].responses).sort(), expected);
  }
});

test("real Go report archive DTO serialization conforms to closed schemas", () => {
  const dir = mkdtempSync(join(fileURLToPath(backend), "report-archive-contract-"));
  const source = `package main
import (
  "crypto/sha256"
  "encoding/hex"
  "encoding/json"
  "fmt"
  "time"
  "github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
  "github.com/gxfcjkxf/lottery/backend/internal/reporting"
)
func main() {
  id := "11111111-1111-4111-8111-111111111111"
  now := time.Date(2026, 10, 7, 8, 9, 10, 123456789, time.UTC)
  zeroBetting := reporting.BettingTotals{OrderCount:"0",StakePoints:"0",PlacedCount:"0",WonCount:"0",LostCount:"0",AbnormalCount:"0",CancelledCount:"0",RefundPoints:"0",SettledStakePoints:"0",UnfinalizedStakePoints:"0",AbnormalStakePoints:"0",CurrentPrizePoints:"0",CorrectionOpenCount:"0"}
  zeroLedger := reporting.LedgerTotals{EntryCount:"0",NetPoints:"-1",RechargePoints:"0",PrizeCreditPoints:"0",PrizeReversalPoints:"0",RefundPoints:"0"}
  zeroBalances := reporting.Balances{AccountCount:"0",AvailablePoints:"0",FrozenPoints:"0",WithdrawalPoints:"0",TotalPoints:"0"}
  zeroWithdrawal := reporting.WithdrawalTotals{OrderCount:"0",RequestedPoints:"0",ReviewingCount:"0",ReviewingPoints:"0",ProcessingCount:"0",ProcessingPoints:"0",PaidCount:"0",PaidPoints:"0",RejectedCount:"0",RejectedPoints:"0",FailedCount:"0",FailedPoints:"0",CancelledCount:"0",CancelledPoints:"0"}
  zeroCommission := reporting.CommissionTotals{EntryCount:"0",PaidEntryCount:"0",PaidPoints:"0",AdjustmentEntryCount:"0",AdjustmentCreditPoints:"0",AdjustmentDebitPoints:"0",CorrectionEntryCount:"0",CorrectionCreditPoints:"0",CorrectionDebitPoints:"0",NetPoints:"-2"}
  zeroReward := reporting.RewardTotals{EntryCount:"0",GrantEntryCount:"0",GrantPoints:"0",ReversalEntryCount:"0",ReversalPoints:"0",NetPoints:"-3"}
  zeroRewardOrders := reporting.RewardOrderTotals{OrderCount:"0",OriginalPoints:"0",GrantedCount:"0",GrantedPoints:"0",PendingCount:"0",PendingPoints:"0",RevokedCount:"0",RevokedPoints:"0"}
  snapshot := reporting.ArchiveSnapshot{BrandID:id,FormatVersion:1,SnapshotAt:now,Timezone:"Asia/Singapore",From:now.Add(-24*time.Hour),To:now,Betting:zeroBetting,Ledger:zeroLedger,WalletSnapshot:reporting.ArchiveWallet{AtSnapshot:now,Balances:zeroBalances},Withdrawals:zeroWithdrawal,Commissions:zeroCommission,Rewards:zeroReward,RewardOrders:zeroRewardOrders}
  record := reportarchive.Record{ID:id,BrandID:id,Window:reportarchive.Window{Kind:"daily",PeriodKey:"2026-10-07",Timezone:"Asia/Singapore",From:snapshot.From,To:snapshot.To},Revision:1,PreviousID:nil,SnapshotAt:now,CreatedBy:id,Reason:"contract example",PayloadSHA256:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",AuditLogID:id,CreatedAt:now,Snapshot:snapshot}
  page := reportarchive.Page{BrandID:id,Items:[]reportarchive.Record{record},TotalCount:"1",Limit:1,Offset:0}
  dto, err := json.Marshal(snapshot); if err != nil { panic(err) }
  var payload map[string]any
  if err = json.Unmarshal(dto, &payload); err != nil { panic(err) }
  raw, err := json.Marshal(payload); if err != nil { panic(err) }
  rawHash := sha256.Sum256(raw); dtoHash := sha256.Sum256(dto)
  out := map[string]any{"record":record,"page":page,"snapshot":snapshot,"canonical_payload":string(raw),"canonical_sha256":hex.EncodeToString(rawHash[:]),"dto_snapshot_sha256":hex.EncodeToString(dtoHash[:])}
  encoded, err := json.Marshal(out); if err != nil { panic(err) }; fmt.Println(string(encoded))
}`;
  try {
    writeFileSync(join(dir, "main.go"), source);
    const run = spawnSync(go, ["run", "-buildvcs=false", "./" + relative(fileURLToPath(backend), dir)], {
      cwd: backend, encoding: "utf8", env: { ...process.env, CGO_ENABLED: "0" },
    });
    assert.equal(run.status, 0, run.stderr || run.error?.message);
    const examples = JSON.parse(run.stdout);
    for (const [name, value] of [["ReportArchiveRecord", examples.record], ["ReportArchivePage", examples.page], ["ReportArchiveSnapshot", examples.snapshot]]) {
      const check = validate(name);
      assert.ok(check(value), `${name}: ${JSON.stringify(check.errors)}`);
    }
    assert.deepEqual(Object.keys(examples.record).sort(), Object.keys(schemas.ReportArchiveRecord.properties).sort());
    assert.deepEqual(Object.keys(examples.snapshot).sort(), Object.keys(schemas.ReportArchiveSnapshot.properties).sort());
    assert.deepEqual(Object.keys(examples.page).sort(), Object.keys(schemas.ReportArchivePage.properties).sort());
    assert.deepEqual(Object.keys(examples.record.window).sort(), Object.keys(schemas.ReportArchiveWindow.properties).sort());
    for (const [dtoField, schemaName] of [
      ["betting", "ReportArchiveBettingTotals"], ["ledger", "ReportArchiveLedgerTotals"],
      ["wallet_snapshot", "ReportArchiveWallet"], ["withdrawals", "ReportArchiveWithdrawalTotals"],
      ["commissions", "ReportArchiveCommissionTotals"], ["rewards", "ReportArchiveRewardTotals"],
      ["reward_orders", "ReportArchiveRewardOrderTotals"],
    ]) assert.deepEqual(Object.keys(examples.snapshot[dtoField]).sort(), Object.keys(schemas[schemaName].properties).sort(), dtoField);
    const canonicalHash = createHash("sha256").update(examples.canonical_payload, "utf8").digest("hex");
    assert.deepEqual(JSON.parse(examples.canonical_payload), examples.snapshot);
    assert.equal(canonicalHash, examples.canonical_sha256);
    assert.notEqual(canonicalHash, examples.dto_snapshot_sha256, "canonical payload bytes are not a Go DTO re-marshalling");
    assert.equal(examples.snapshot.format_version, 1);
    assert.equal(examples.snapshot.ledger.net_points, "-1");
    assert.equal(examples.snapshot.commissions.net_points, "-2");
    assert.equal(examples.snapshot.rewards.net_points, "-3");
    assert.equal(examples.record.previous_id, null);
    assert.match(examples.snapshot.snapshot_at, /\.123456789Z$/);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
