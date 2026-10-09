const historyPaths = new Set([
  "/api/v1/admin/audit",
  "/api/v1/admin/notification-templates/{key}/history",
  "/api/v1/admin/compliance-policy/history",
  "/api/v1/admin/brand-presentation/history",
  "/api/v1/admin/brand-domains/history",
  "/api/v1/admin/brand-operation/history",
]);

export function addHistoryReadHeaders(operations) {
  for (const operation of operations) {
    if (operation.method !== "GET" || !historyPaths.has(operation.path)) continue;
    operation.successHeaders = {
      ...operation.successHeaders,
      "X-Read-Source": { description: "Data source, after fresh primary authorization and successful primary audit commit. Not a client routing control.", schema: { type: "string", enum: ["primary", "replica"] } },
      "X-Read-Reason": { description: "Fixed routing diagnostic; does not disclose connection details.", schema: { type: "string", enum: ["not_configured", "primary_only", "wal_fenced"] } },
      "X-Read-Replica": { description: "Present only for replica data: one-based index in server-configured read nodes. Not an address or a user-supplied selector.", schema: { type: "string", pattern: "^[1-8]$" } },
    };
    operation.description = `${operation.description ?? ""} This immutable-history endpoint may read a physical standby only after its replay reaches the request's primary WAL fence. Authorization and audit always use primary. When replicas are configured, an unhealthy, lagging or failed selected replica returns an error; it does not fall back to primary or another replica. Without configured replicas, reads use primary. The result is not a current financial or configuration projection.`.trim();
  }
}
