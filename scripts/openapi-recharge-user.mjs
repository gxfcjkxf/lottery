const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = properties => ({ type: "object", properties, required: Object.keys(properties), additionalProperties: false });
const nullable = schema => ({ anyOf: [schema, { type: "null" }] });
const state = { type: "string", enum: ["pending", "confirmed", "cancelled"] };
const utcDateTime = {
  type: "string", format: "date-time", pattern: "Z$",
  description: "RFC3339 timestamp in UTC, serialized with the Z suffix.",
};

const recharge = obj({
  id: ref("UUID"),
  brand_id: ref("UUID"),
  member_id: ref("UUID"),
  points: ref("PositiveInt64String"),
  state,
  version: ref("PositiveInt64String"),
  created_at: utcDateTime,
  confirmed_at: nullable(utcDateTime),
  ledger_entry_id: nullable(ref("UUID")),
});
recharge.description = "Privacy-limited current-state projection of a real recharge record. It omits account identifiers, proof references, remarks, administrator identity, reasons, audit data, and full ledger entries. It is not a historical snapshot, payment receipt, or current balance.";
recharge.allOf = [
  {
    if: { properties: { state: { const: "confirmed" } }, required: ["state"] },
    then: {
      properties: {
        confirmed_at: { allOf: [utcDateTime] },
        ledger_entry_id: { allOf: [ref("UUID")] },
      },
    },
  },
  {
    if: { properties: { state: { enum: ["pending", "cancelled"] } }, required: ["state"] },
    then: {
      properties: {
        confirmed_at: { type: "null" },
        ledger_entry_id: { type: "null" },
      },
    },
  },
];

export const schemas = {
  MemberRecharge: recharge,
  MemberRechargePage: {
    ...obj({
      brand_id: ref("UUID"),
      member_id: ref("UUID"),
      snapshot_at: utcDateTime,
      state: nullable(state),
      items: { type: "array", items: ref("MemberRecharge"), maxItems: 100 },
      limit: { type: "integer", minimum: 1, maximum: 100 },
      offset: { type: "integer", minimum: 0, maximum: 1000000 },
      total_count: ref("NonnegativeInt64String"),
    }),
    description: "Current-state projection for the authenticated member, read from the primary database. Items are ordered by created_at descending then id descending. This page is not a historical snapshot, payment receipt, or current balance; total_count is an exact canonical decimal string.",
  },
};

const pagination = [
  { name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 } },
  { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 } },
  { name: "state", in: "query", required: false, schema: state },
];

const userRead = (path, operationId, summary, data, extra = {}) => ({
  method: "GET",
  path: `/api/v1${path}`,
  operationId,
  summary,
  tag: "finance",
  auth: "user",
  data,
  description: "Same-origin user-authenticated read, scoped only to the current session's verified brand member. The brand host/cookie and, for the brand-path alias, the resolved brand must agree with that identity. The handler reauthenticates in the read transaction and reads only from the primary database. Successful and not-found reads are sanitized and audited in that same transaction before any data is returned; audit failure returns 503 without data. AuthenticateTx permits reads for paused brands and frozen members; it rejects disabled brands, disabled members, disabled global users, missing terms consent, and unavailable sessions.",
  ...extra,
});

export const operations = [
  userRead("/recharges", "listMyRecharges", "List the authenticated member's recharge records", ref("MemberRechargePage"), {
    parameters: pagination,
    description: "Accepts only limit, offset, and state. Defaults are limit=20 and offset=0; limit is 1..100 and offset is 0..1000000. Unknown, empty, duplicate, and noncanonical numeric query values (including +20 and 01), ForceQuery, and request bodies are rejected. There is no member selector. Returns only current recharge state, ordered by created_at DESC then id DESC; it is not a historical snapshot, payment receipt, or current balance. No user-facing create, confirm, or payment endpoint is exposed. Same-origin user authentication is scoped to the current verified member; host and brand cookie must agree, and a brand path cannot override the resolved brand. AuthenticateTx permits reads for paused brands and frozen members; it rejects disabled brands, disabled members, disabled global users, missing terms consent, and unavailable sessions. The handler reauthenticates in the transaction and reads only from the primary database. Successful and not-found reads write sanitized audit records in the same transaction before returning; audit failure returns 503 without data.",
  }),
  userRead("/recharges/{id}", "getMyRecharge", "Get an authenticated member's recharge record", ref("MemberRecharge"), {
    description: "Accepts no query string, including an empty ForceQuery, and no request body. The UUID record is visible only to the authenticated member in the resolved brand; another member's or brand's record returns 404. Returns current state, not a historical snapshot, payment receipt, or current balance. No user-facing create, confirm, or payment endpoint is exposed. Same-origin user authentication is scoped to the current verified member; host and brand cookie must agree, and a brand path cannot override the resolved brand. AuthenticateTx permits reads for paused brands and frozen members; it rejects disabled brands, disabled members, disabled global users, missing terms consent, and unavailable sessions. The handler reauthenticates in the transaction and reads only from the primary database. Successful and not-found reads write sanitized audit records in the same transaction before returning; audit failure returns 503 without data.",
  }),
];
