const ref = name => ({ $ref: `#/components/schemas/${name}` });
const uuid = ref("UUID");
const obj = properties => ({
  type: "object",
  required: Object.keys(properties),
  additionalProperties: false,
  properties,
});

const reason = {
  type: "string",
  minLength: 1,
  maxLength: 500,
  pattern: "^(?=.*\\S)(?!\\s)(?![\\s\\S]*\\s$)[^\\u0000-\\u001F\\u007F-\\u009F]*$",
  description: "Trimmed nonempty valid UTF-8, at most 500 UTF-8 bytes; C0/C1 controls are rejected by the handler.",
};

export const schemas = {
  ReportArchivePolicyUpdateInput: obj({
    version: { type: "integer", minimum: 1, maximum: 9007199254740990 },
    daily_enabled: { type: "boolean" },
    monthly_enabled: { type: "boolean" },
    reason,
  }),
};

export const operations = [
  {
    method: "PUT",
    path: "/api/v1/admin/report-archive-policy",
    operationId: "updateReportArchiveAutomaticPolicy",
    summary: "Update automatic report archive policy",
    tag: "report archive policy",
    auth: "admin",
    brandHeader: true,
    permissions: ["report_archive.view.brand", "report_archive_policy.write.brand"],
    idempotency: true,
    successStatus: 200,
    parameters: [{
      name: "X-Report-Archive-Actor-ID",
      in: "header",
      required: true,
      schema: uuid,
      description: "Must equal the authenticated current administrator ID.",
    }],
    requestBody: ref("ReportArchivePolicyUpdateInput"),
    data: ref("ReportArchivePolicy"),
    description: "Requires current brand membership, report_archive.view.brand, report_archive_policy.write.brand, and a non-super-admin account. The 200 AutomaticPolicy acknowledgement must be scoped to X-Brand-ID, carry version+1 and the requested enablement flags, and include the original server-derived first-enable period when enabled: the current brand-local day for daily archiving or current brand-local month for monthly archiving, using the database clock. The first archive is captured only after that period ends; first activation does not backfill earlier periods. Disabling or reenabling retains any existing starts; clients cannot submit starts or timezone. Unknown write outcomes must be retried with the exact same body and idempotency key; the server does not automatically replay the operation.",
  },
];
