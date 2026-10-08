const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({
  type: "object", properties, required, additionalProperties: false,
});
const uuid = ref("UUID"), date = ref("DateTime");
const exactCount = { type: "string", pattern: "^(0|[1-9][0-9]*)$", description: "Exact nonnegative decimal aggregate; never convert to JavaScript Number." };
const exactSigned = { type: "string", pattern: "^(0|-?[1-9][0-9]*)$", description: "Exact signed decimal aggregate; net totals may be negative and must never be converted to JavaScript Number." };
const nullable = schema => ({ anyOf: [schema, { type: "null" }] });
const aggregate = fields => obj(Object.fromEntries(fields.map(field => [field, exactCount])));
const signedAggregate = (fields, signedFields) => obj(Object.fromEntries(fields.map(field => [field, signedFields.includes(field) ? exactSigned : exactCount])));
const totals = {
  betting: ["order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points", "correction_open_count"],
  ledger: ["entry_count", "net_points", "recharge_points", "prize_credit_points", "prize_reversal_points", "refund_points"],
  balances: ["account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"],
  withdrawals: ["order_count", "requested_points", "reviewing_count", "reviewing_points", "processing_count", "processing_points", "paid_count", "paid_points", "rejected_count", "rejected_points", "failed_count", "failed_points", "cancelled_count", "cancelled_points"],
  commissions: ["entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points", "net_points"],
  rewards: ["entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points"],
  reward_orders: ["order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points"],
};
const archiveSnapshot = obj({
  brand_id: uuid,
  format_version: { type: "integer", const: 1 },
  snapshot_at: date,
  timezone: { type: "string", minLength: 1 },
  from: date,
  to: date,
  betting: ref("ReportArchiveBettingTotals"),
  ledger: ref("ReportArchiveLedgerTotals"),
  wallet_snapshot: ref("ReportArchiveWallet"),
  withdrawals: ref("ReportArchiveWithdrawalTotals"),
  commissions: ref("ReportArchiveCommissionTotals"),
  rewards: ref("ReportArchiveRewardTotals"),
  reward_orders: ref("ReportArchiveRewardOrderTotals"),
});
archiveSnapshot.description = "Exact reporting.ArchiveSnapshot field set. Aggregate amounts remain decimal strings, including signed ledger and commission net totals.";
const automationEvidence = obj({
  task_id: uuid,
  policy_version: { type: "integer", minimum: 1, maximum: 9007199254740991 },
});
const record = obj({
  id: uuid,
  brand_id: uuid,
  window: ref("ReportArchiveWindow"),
  revision: { type: "integer", minimum: 1, maximum: 9007199254740991 },
  previous_id: nullable(uuid),
  snapshot_at: date,
  created_by: nullable(uuid),
  reason: { type: "string" },
  payload_sha256: { type: "string", pattern: "^[a-f0-9]{64}$" },
  audit_log_id: uuid,
  created_at: date,
  snapshot: ref("ReportArchiveSnapshot"),
  automation: ref("ReportArchiveAutomationEvidence"),
}, ["id", "brand_id", "window", "revision", "previous_id", "snapshot_at", "created_by", "reason", "payload_sha256", "audit_log_id", "created_at", "snapshot"]);
record.oneOf = [
  { properties: { created_by: uuid }, not: { required: ["automation"] } },
  { properties: { created_by: { type: "null" } }, required: ["automation"] },
];
record.description = "Manual records retain their exact 12-field JSON shape. Automatic records have those fields plus automation, with created_by null. No other metadata is accepted.";
const strictControlFreeReason = {
  type: "string", minLength: 1, maxLength: 500,
  pattern: "^(?=.*\\S)(?!\\s)(?![\\s\\S]*\\s$)[^\\u0000-\\u001F\\u007F-\\u009F]*$",
  description: "Trimmed nonempty UTF-8, at most 500 UTF-8 bytes, with no C0/C1 control characters. The byte bound is enforced by the handler; JSON Schema maxLength counts characters.",
};
const input = obj({
  kind: { type: "string", enum: ["daily", "monthly"] },
  period_key: { type: "string", description: "Canonical YYYY-MM-DD for daily or YYYY-MM for monthly; no caller-supplied from/to interval." },
  expected_revision: { type: "integer", minimum: 0, maximum: 9007199254740990 },
  reason: strictControlFreeReason,
});
input.allOf = [
  { if: { properties: { kind: { const: "daily" } }, required: ["kind"] }, then: { properties: { period_key: { pattern: "^[0-9]{4}-[0-9]{2}-[0-9]{2}$" } } } },
  { if: { properties: { kind: { const: "monthly" } }, required: ["kind"] }, then: { properties: { period_key: { pattern: "^[0-9]{4}-[0-9]{2}$" } } } },
];
input.description = "Exactly these four members are accepted. Unknown members, including from/to, are rejected.";
const window = obj({
  kind: { type: "string", enum: ["daily", "monthly"] },
  period_key: { type: "string", pattern: "^(?:[0-9]{4}-[0-9]{2}-[0-9]{2}|[0-9]{4}-[0-9]{2})$" },
  timezone: { type: "string", minLength: 1 },
  from: date,
  to: date,
});
const page = obj({
  brand_id: uuid,
  items: { type: "array", items: ref("ReportArchiveRecord"), maxItems: 100 },
  total_count: exactCount,
  limit: { type: "integer", minimum: 1, maximum: 100 },
  offset: { type: "integer", minimum: 0, maximum: 1000000 },
});
const downloadHeaders = {
  "Content-Disposition": { description: "Original archive JSON attachment filename.", schema: { type: "string", pattern: "^attachment; filename=\\\"report-archive-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}-v[0-9]+\\.json\\\"$" } },
  "Content-Length": { description: "Byte length of the canonical UTF-8 JSONB payload.", schema: { type: "integer", minimum: 1 } },
  "X-Content-SHA256": { description: "SHA-256 of the exact canonical response body bytes.", schema: { type: "string", pattern: "^[a-f0-9]{64}$" } },
  "X-Archive-Revision": { description: "Immutable positive record revision.", schema: { type: "string", pattern: "^[1-9][0-9]*$" } },
  "X-Archive-Format-Version": { description: "Archive snapshot format version.", schema: { type: "string", const: "1" } },
};
const viewPermissions = ["report_archive.view.brand", "report_archive.view.platform"];
const operationDescription = "All operations require an explicit X-Brand-ID and current brand scope. GET bodies and ForceQuery (including a trailing empty '?') are rejected. Unknown, duplicate, empty, and noncanonical query parameters are rejected. Session, roles, brand scope, and permissions are reauthenticated before release; query audit must commit before returning data.";
export const schemas = {
  ReportArchiveBettingTotals: aggregate(totals.betting),
  ReportArchiveLedgerTotals: signedAggregate(totals.ledger, ["net_points"]),
  ReportArchiveBalances: aggregate(totals.balances),
  ReportArchiveWallet: obj({ at_snapshot: date, balances: ref("ReportArchiveBalances") }),
  ReportArchiveWithdrawalTotals: aggregate(totals.withdrawals),
  ReportArchiveCommissionTotals: signedAggregate(totals.commissions, ["net_points"]),
  ReportArchiveRewardTotals: signedAggregate(totals.rewards, ["net_points"]),
  ReportArchiveRewardOrderTotals: aggregate(totals.reward_orders),
  ReportArchiveSnapshot: archiveSnapshot,
  ReportArchiveWindow: window,
  ReportArchiveAutomationEvidence: automationEvidence,
  ReportArchiveRecord: record,
  ReportArchivePage: page,
  ReportArchiveCreateInput: input,
};

const path = "/api/v1/admin/report-archives";
const pagination = [
  { name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 } },
  { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 } },
];
const downloadSuccess = {
  "application/json": { schema: ref("ReportArchiveSnapshot") },
};
export const operations = [
  {
    method: "GET", path, operationId: "adminListReportArchives", summary: "List immutable report archives", tag: "report archives", auth: "admin", data: ref("ReportArchivePage"),
    brandHeader: true, permissions: viewPermissions, parameters: pagination, description: operationDescription,
  },
  {
    method: "POST", path, operationId: "adminCreateReportArchive", summary: "Capture immutable report archive", tag: "report archives", auth: "admin", data: ref("ReportArchiveRecord"),
    brandHeader: true, permissions: [...viewPermissions, "report_archive.create.brand"], idempotency: true, successStatus: 201,
    requestBody: ref("ReportArchiveCreateInput"),
    parameters: [{ name: "X-Report-Archive-Actor-ID", in: "header", required: true, schema: uuid, description: "Must equal the authenticated current administrator ID." }],
    description: `${operationDescription} Actor header must match the current authenticated actor. Archive period is resolved by the server from kind/period_key and brand timezone. 201 returns the original immutable Record. A lock-busy 503 creates no idempotency receipt.`,
  },
  {
    method: "GET", path: `${path}/{id}`, operationId: "adminGetReportArchive", summary: "Read immutable report archive", tag: "report archives", auth: "admin", data: ref("ReportArchiveRecord"),
    brandHeader: true, permissions: viewPermissions, description: operationDescription,
  },
  {
    method: "GET", path: `${path}/{id}/download`, operationId: "adminDownloadReportArchive", summary: "Download canonical report archive payload", tag: "report archives", auth: "admin",
    brandHeader: true, permissions: [...viewPermissions, "report_archive.download.brand", "report_archive.download.platform"],
    successDescription: "Exact original canonical payload::text UTF-8 bytes after integrity verification, full audit commit, and session/role/brand/permission reauthentication. This is the bare JSON payload, not an API success envelope or a re-marshalled DTO.",
    successContent: downloadSuccess, successHeaders: downloadHeaders,
    description: `${operationDescription} Download independently requires report_archive.download.brand OR report_archive.download.platform in addition to view access. The complete payload integrity is verified, the download audit is committed, and actor/session/roles/scope/permissions are reauthenticated before any bytes are written. Response body is the canonical payload itself, without the standard JSON envelope. The payload SHA-256 can differ from a hash of a DTO re-marshalling; X-Content-SHA256 identifies the actual response bytes.`,
  },
];
