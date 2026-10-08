const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = properties => ({ type: "object", properties, required: Object.keys(properties), additionalProperties: false });
const uuid = ref("UUID"), date = ref("DateTime"), nullable = schema => ({ anyOf: [schema, { type: "null" }] });
const safeVersion = { type: "integer", minimum: 1, maximum: 9007199254740991 };
const windowSchema = obj({
  kind: { type: "string", enum: ["daily", "monthly"] },
  period_key: { type: "string", pattern: "^(?:[0-9]{4}-[0-9]{2}-[0-9]{2}|[0-9]{4}-[0-9]{2})$" },
  timezone: { type: "string", minLength: 1 }, from: date, to: date,
});
windowSchema.description = "Persisted frozen half-open instant range. Clients validate the canonical period key, named timezone label, real timestamp syntax, and from < to; they must not recalculate boundaries from the current timezone database.";
const policy = obj({
  brand_id: uuid, version: safeVersion, daily_enabled: { type: "boolean" }, monthly_enabled: { type: "boolean" },
  daily_start_period: nullable({ type: "string", pattern: "^[0-9]{4}-[0-9]{2}-[0-9]{2}$" }),
  monthly_start_period: nullable({ type: "string", pattern: "^[0-9]{4}-[0-9]{2}$" }), timezone: { type: "string", minLength: 1 },
  audit_log_id: nullable(uuid), updated_at: date,
});
policy.description = "Closed AutomaticPolicy DTO. This endpoint is read-only; it does not enable automatic archiving.";
policy.allOf = [
  { if: { properties: { audit_log_id: { type: "null" } }, required: ["audit_log_id"] }, then: { properties: { version: { const: 1 }, daily_enabled: { const: false }, monthly_enabled: { const: false }, daily_start_period: { type: "null" }, monthly_start_period: { type: "null" } } } },
  { if: { properties: { version: { const: 1 } }, required: ["version"] }, then: { properties: { audit_log_id: { type: "null" }, daily_enabled: { const: false }, monthly_enabled: { const: false }, daily_start_period: { type: "null" }, monthly_start_period: { type: "null" } } } },
  { if: { properties: { version: { minimum: 2 } }, required: ["version"] }, then: { properties: { audit_log_id: uuid } } },
];
const task = obj({
  id: uuid, brand_id: uuid, policy_version: safeVersion, window: ref("ReportArchiveTaskWindow"),
  state: { type: "string", enum: ["pending", "completed", "skipped", "failed"] }, version: safeVersion,
  attempt_count: { type: "integer", minimum: 0, maximum: 9007199254740991 }, archive_id: nullable(uuid),
  last_error_code: nullable({ type: "string", minLength: 1 }), creation_audit_log_id: uuid, last_audit_log_id: uuid,
  created_at: date, updated_at: date,
});
task.description = "Closed AutomaticTask DTO. Initial pending tasks have version 1 and zero attempts; after n processing attempts, pending version is 2n+1 and terminal version is 2n. Completed and skipped tasks carry an archive_id and null last_error_code; failed tasks carry null archive_id and ARCHIVE_FAILED; pending tasks carry neither. Persisted window timestamps are frozen and must not be recalculated from the current timezone database; to must be no later than created_at.";
task.allOf = [
  { if: { properties: { state: { const: "pending" } }, required: ["state"] }, then: { properties: { archive_id: { type: "null" }, last_error_code: { type: "null" } } } },
  { if: { properties: { state: { const: "completed" } }, required: ["state"] }, then: { properties: { archive_id: uuid, last_error_code: { type: "null" }, attempt_count: { minimum: 1 } } } },
  { if: { properties: { state: { const: "skipped" } }, required: ["state"] }, then: { properties: { archive_id: uuid, last_error_code: { type: "null" }, attempt_count: { minimum: 1 } } } },
  { if: { properties: { state: { const: "failed" } }, required: ["state"] }, then: { properties: { archive_id: { type: "null" }, last_error_code: { const: "ARCHIVE_FAILED" }, attempt_count: { minimum: 1 } } } },
];
const page = obj({
  brand_id: uuid, items: { type: "array", items: ref("ReportArchiveTask"), maxItems: 100 },
  total_count: { type: "string", pattern: "^(0|[1-9][0-9]*)$", description: "Exact nonnegative decimal count; never convert to JavaScript Number." },
  limit: { type: "integer", minimum: 1, maximum: 100 }, offset: { type: "integer", minimum: 0, maximum: 1000000 },
});
page.description = "Closed AutomaticTaskPage DTO. Items sort by created_at descending, then id descending.";
page.description += " Items length equals min(limit, max(total_count - offset, 0)).";

const view = ["report_archive.view.brand", "report_archive.view.platform"];
const routeDescription = "Requires the explicit X-Brand-ID header and current brand scope. No brand path alias is provided. Request bodies and ForceQuery are rejected where not declared; query parameters must be unique and canonical.";
const response = schema => ({ data: schema, successDescription: "Matching successful response envelope." });
export const schemas = {
  ReportArchiveTaskWindow: windowSchema,
  ReportArchivePolicy: policy,
  ReportArchiveTask: task,
  ReportArchiveTaskPage: page,
  ReportArchiveTaskRetryInput: obj({
    version: { type: "integer", minimum: 1, maximum: 9007199254740990 },
    reason: { type: "string", minLength: 1, maxLength: 500, pattern: "^(?=.*\\S)(?!\\s)(?![\\s\\S]*\\s$)[^\\u0000-\\u001F\\u007F-\\u009F]*$", description: "Trimmed nonempty UTF-8, at most 500 UTF-8 bytes; C0/C1 controls are forbidden. Byte length is enforced by the handler." },
  }),
};

export const operations = [
  { method: "GET", path: "/api/v1/admin/report-archive-policy", operationId: "getReportArchiveAutomaticPolicy", summary: "Read automatic report archive policy", tag: "report archive tasks", auth: "admin", brandHeader: true, permissions: view, ...response(ref("ReportArchivePolicy")), description: `${routeDescription} Read only; no policy write or automatic enablement route is documented.` },
  { method: "GET", path: "/api/v1/admin/report-archive-tasks", operationId: "listReportArchiveAutomaticTasks", summary: "List automatic report archive tasks", tag: "report archive tasks", auth: "admin", brandHeader: true, permissions: view,
    parameters: [
      { name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 } },
      { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 } },
    ], ...response(ref("ReportArchiveTaskPage")), description: routeDescription },
  { method: "GET", path: "/api/v1/admin/report-archive-tasks/{id}", operationId: "readReportArchiveAutomaticTask", summary: "Read an automatic report archive task", tag: "report archive tasks", auth: "admin", brandHeader: true, permissions: view, ...response(ref("ReportArchiveTask")), description: routeDescription },
  { method: "POST", path: "/api/v1/admin/report-archive-tasks/{id}/retry", operationId: "retryReportArchiveAutomaticTask", summary: "Retry a failed automatic report archive task", tag: "report archive tasks", auth: "admin", brandHeader: true,
    permissions: ["report_archive.view.brand", "report_archive_task.retry.brand"], idempotency: true, successStatus: 200,
    parameters: [{ name: "X-Report-Archive-Actor-ID", in: "header", required: true, schema: uuid, description: "Must equal the authenticated current administrator ID." }],
    requestBody: ref("ReportArchiveTaskRetryInput"), ...response(ref("ReportArchiveTask")), description: `${routeDescription} Retry requires brand membership, report_archive.view.brand, and report_archive_task.retry.brand; super admins cannot retry. The 200 body acknowledges only a version+1 pending task and does not claim the archive has completed.` },
];
