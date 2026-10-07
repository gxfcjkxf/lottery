const ref = (name) => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({
  type: "object", properties, required, additionalProperties: false,
});
const nullable = (schema) => ({ anyOf: [schema, { type: "null" }] });
const uuid = ref("UUID");
const dateTime = ref("DateTime");
const positiveAmount = ref("PositiveInt64String");
const amount = ref("NonnegativeInt64String");
const safeVersion = {
  type: "integer", minimum: 1, maximum: 9007199254740991,
  description: "Positive version exactly representable as a JSON/JavaScript integer.",
};
const state = { type: "string", enum: ["reviewing", "processing", "paid", "rejected", "failed", "cancelled"] };
const sources = { type: "string", enum: ["recharge", "winning", "gift"] };
const pagination = [
  { name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 }, description: "Page size (default 20; maximum 100)." },
  { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of records to skip (default 0; maximum 1000000)." },
];
const mutation = (requestBody, successStatus = 200) => ({ requestBody, idempotency: true, successStatus });
const user = (method, path, operationId, summary, data, extra = {}) => ({
  method, path: `/api/v1${path}`, operationId, summary, tag: "withdrawals", auth: "user", data,
  description: "Requires a current user session bound to the actual brand member. Strict query parsing rejects unknown, duplicate, and empty parameters.",
  ...extra,
});
const admin = (method, path, operationId, summary, data, permission, extra = {}) => ({
  method, path: `/api/v1/admin${path}`, operationId, summary, tag: "withdrawals", auth: "admin", data,
  brandHeader: true,
  permissions: method === "GET"
    ? [`${permission}.brand`, `${permission}.platform`]
    : [`${permission}.brand`],
  description: method === "GET"
    ? `Requires ${permission}.brand or ${permission}.platform for the selected brand. Reads are audited in the same transaction before the response is returned.`
    : `Requires the exact ${permission}.brand grant for the selected brand. Platform grants and SUPER_ADMIN status do not authorize writes. Cached write receipts recheck current authorization.`,
  ...extra,
});

const allocation = obj({ source: sources, state: { type: "string", const: "available" }, points: positiveAmount });
const order = obj({
  id: uuid, brand_id: uuid, member_id: uuid, account_id: uuid, points: positiveAmount,
  state, version: safeVersion, source_allocation: { type: "array", items: ref("WithdrawalSourceAllocation"), minItems: 1, maxItems: 3 },
  reserve_entry_id: uuid, release_entry_id: nullable(uuid), paid_entry_id: nullable(uuid),
  cycle_from_at: nullable(dateTime), cycle_from_version: ref("NonnegativeInt64String"), reserve_version: ref("PositiveInt64String"),
  created_at: dateTime, updated_at: dateTime, reviewed_at: nullable(dateTime), completed_at: nullable(dateTime),
  decision_reason: { type: "string" }, audit_log_id: uuid,
}, undefined);
order.properties.source_allocation.description = "Only positive available allocations; sources are unique and sorted recharge, winning, gift, and their points sum exactly to the order points.";
order.description = "Sanitized order view. Policy snapshots, eligibility evidence, actor IDs, and client idempotency keys are never exposed. User reads redact decision_reason except for rejection, failure, or cancellation; administrator reads include the full reason.";

const transition = obj({
  id: uuid, version: safeVersion,
  from_state: { type: "string", enum: ["", "reviewing", "processing", "paid", "rejected", "failed", "cancelled"] },
  to_state: state, reason: { type: "string" },
  actor_type: { type: "string", enum: ["user", "admin", "system"] }, created_at: dateTime, audit_log_id: uuid,
});
transition.description = "History omits actor IDs. The first transition has an empty from_state. User reads redact reason except for rejection, failure, or cancellation; administrator reads include the full reason.";

export const schemas = {
  WithdrawalSourceAllocation: allocation,
  WithdrawalOrder: order,
  WithdrawalOrderPage: {
    ...obj({
      brand_id: uuid,
      items: { type: "array", items: ref("WithdrawalOrder"), maxItems: 100 },
      limit: { type: "integer", minimum: 1, maximum: 100 },
      offset: { type: "integer", minimum: 0, maximum: 1000000 },
      has_more: { type: "boolean" },
    }),
    allOf: [{
      if: { properties: { has_more: { const: true } }, required: ["has_more"] },
      then: { properties: { items: { minItems: 1 } } },
    }],
    description: "Items never exceed limit. When has_more is true, items contains a full page (items.length equals limit). The response is closed to future fields.",
  },
  WithdrawalTransition: transition,
  WithdrawalHistory: obj({ brand_id: uuid, order_id: uuid, items: { type: "array", items: ref("WithdrawalTransition"), maxItems: 3 } }),
  WithdrawalAvailability: obj({
    brand_id: uuid, member_id: uuid, policy_enabled: { type: "boolean" }, eligibility_configured: { type: "boolean" },
    can_apply: { type: "boolean" },
    reason_code: { type: "string", enum: ["AVAILABLE", "WITHDRAWAL_DISABLED", "WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED", "WITHDRAWAL_ACCOUNT_RESTRICTED"] },
    min_points: positiveAmount, max_points: nullable(positiveAmount), allowed_sources: { type: "array", items: sources, minItems: 1, maxItems: 3, uniqueItems: true },
    real_payments: { type: "boolean", const: false }, actor_context: { type: "string", pattern: "^[0-9a-f]{64}$" },
  }),
  WithdrawalCreateRequest: obj({
    points: positiveAmount,
    source_allocation: { type: "array", items: ref("WithdrawalSourceAllocation"), minItems: 1, maxItems: 3 },
  }),
  WithdrawalActionRequest: obj({ version: safeVersion, reason: ref("Reason") }),
};
schemas.WithdrawalCreateRequest.properties.source_allocation.description = "Positive available allocations only. Sources must be unique, sorted recharge then winning then gift, and sum exactly to points.";

export const operations = [
  user("GET", "/withdrawal-availability", "getWithdrawalAvailability", "Get withdrawal availability", ref("WithdrawalAvailability"), {
    description: "Returns current policy and eligibility availability for the authenticated member, plus an opaque actor_context token bound to the actual global user and brand member. The token must be sent unchanged when creating an order; it is not a client-selected identity.",
  }),
  user("GET", "/withdrawals", "listWithdrawalOrders", "List the authenticated member's withdrawal orders", ref("WithdrawalOrderPage"), {
    parameters: [...pagination, { name: "state", in: "query", required: false, schema: state }],
    description: "Accepts only limit, offset, and state query parameters. Unknown, duplicate, or empty query parameters are rejected. User decision reasons are shown only for rejection, failure, or cancellation.",
  }),
  user("POST", "/withdrawals", "createWithdrawalOrder", "Create a withdrawal order", ref("WithdrawalOrder"), {
    ...mutation(ref("WithdrawalCreateRequest"), 201),
    parameters: [{ name: "X-Withdrawal-Actor-Context", in: "header", required: true, schema: { type: "string", pattern: "^[0-9a-f]{64}$" }, description: "Exact actor_context value returned by withdrawal-availability. Binds this intent to the same global user and brand member even if the session cookie changes. Missing or changed value returns 403 WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED. Checked for new requests and cached idempotent replays." }],
    description: "Body contains only points and source_allocation; client_key and qualification claims are not accepted. Revalidates the actual brand member and current identity on both new writes and cached receipt replays. Missing or changed X-Withdrawal-Actor-Context returns 403 WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED. Eligibility-not-configured returns 409 WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED before reserving points. When qualification is configured and passes, manual review creates a reviewing v1 order; saved automatic review mode advances it to processing v2. Neither path marks the order paid or completed. Idempotent retries reuse the same key and body.",
  }),
  user("GET", "/withdrawals/{id}", "getWithdrawalOrder", "Get a withdrawal order", ref("WithdrawalOrder"), {
    description: "No query parameters. Reads only the authenticated member's order and redacts decision_reason except for rejection, failure, or cancellation.",
  }),
  user("GET", "/withdrawals/{id}/history", "getWithdrawalOrderHistory", "Get withdrawal order history", ref("WithdrawalHistory"), {
    description: "No query parameters. Returns privacy-sanitized transitions without actor IDs; user-visible reasons are limited to rejection, failure, or cancellation.",
  }),
  admin("GET", "/withdrawals", "adminListWithdrawalOrders", "List withdrawal orders", ref("WithdrawalOrderPage"), "withdrawal.view", {
    parameters: [...pagination, { name: "state", in: "query", required: false, schema: state }, { name: "member_id", in: "query", required: false, schema: uuid }],
    description: "Accepts only limit, offset, state, and member_id query parameters. Unknown, duplicate, or empty query parameters are rejected. Read permission accepts withdrawal.view.brand or withdrawal.view.platform for the selected brand; each read is audited before the response body is returned.",
  }),
  admin("GET", "/withdrawals/{id}", "adminGetWithdrawalOrder", "Get a withdrawal order", ref("WithdrawalOrder"), "withdrawal.view", {
    description: "No query parameters. Requires withdrawal.view.brand or withdrawal.view.platform. The audited administrator view includes the full decision_reason; policy and qualification evidence remain omitted.",
  }),
  admin("GET", "/withdrawals/{id}/history", "adminGetWithdrawalOrderHistory", "Get withdrawal order history", ref("WithdrawalHistory"), "withdrawal.view", {
    description: "No query parameters. Requires withdrawal.view.brand or withdrawal.view.platform. The audited administrator view includes full transition reasons and actor_type, never actor IDs.",
  }),
  ...["approve", "reject", "cancel", "fail", "mark-paid"].map((action) => admin(
    "POST", `/withdrawals/{id}/${action}`, `admin${action.split("-").map((part) => part[0].toUpperCase() + part.slice(1)).join("")}WithdrawalOrder`, `${action} a withdrawal order`,
    ref("WithdrawalOrder"), `withdrawal.${action === "mark-paid" ? "mark_paid" : action}`,
    {
      ...mutation(ref("WithdrawalActionRequest")),
      description: `Requires the exact withdrawal.${action === "mark-paid" ? "mark_paid" : action}.brand grant for the selected brand; platform permissions and SUPER_ADMIN status never authorize writes. Cached idempotent replays recheck authorization. Success returns the resulting audited order view.`,
    },
  )),
];
