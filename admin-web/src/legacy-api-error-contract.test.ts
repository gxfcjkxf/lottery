import { describe, expect, it, vi } from "vitest";
import { createManagementApi } from "./management-api";
import { createReportsApi } from "./reports-api";
import { createReportExportApi } from "./report-export-api";
import { createSettlementPreviewApi } from "./settlement-preview-api";
import { createCommissionCyclesApi } from "./commission-cycles-api";
import { createSettlementJobApi } from "./settlement-job-api";
import { createBrandOperationApi } from "./brand-operation-api";
import { createPeriodCancellationApi } from "./period-cancellation-api";
import { createDrawManagementApi } from "./draw-management-api";
import { createAgentsApi } from "./agents-api";
import { buildRuleSimulationRequest, createRuleSimulationApi, DEFAULT_RULE_SIMULATION_FORM } from "./rule-simulation-api";
import { createNotificationDeliveryApi } from "./notification-delivery-api";
import { createBetManagementApi } from "./bet-management-api";
import { createNotificationTemplatesApi } from "./notification-templates-api";
import { createReconciliationApi } from "./reconciliation-api";
import { createJoinCodesApi } from "./join-codes-api";
import { createBrandPresentationApi } from "./brand-presentation-api";
import { createReportArchivesApi } from "./report-archives-api";
import { createPeriodSchedulesApi } from "./period-schedules-api";
import { createCompliancePolicyApi } from "./compliance-api";
import { createBusinessInventoryApi } from "./business-inventory-api";
import { createCommissionPolicyApi } from "./commission-policy-api";
import { createBrandDomainsApi } from "./brand-domains-api";
import { createCommissionPaymentsApi } from "./commissionPayments-api";

const brand = "00000000-0000-4000-8000-000000000001";
const target = "00000000-0000-4000-8000-000000000002";
const oldStringError = () => vi.fn<typeof fetch>().mockResolvedValue(
  new Response(JSON.stringify({ success: false, error: "legacy denial" }), { status: 400 }),
);

describe("admin API error envelope contract", () => {
  it("rejects old string errors while preserving current structured business errors", async () => {
    const reportQuery = { from: "2026-01-01T00:00:00Z", to: "2026-01-02T00:00:00Z", group_by: "day" as const };
    const ruleBody = buildRuleSimulationRequest("special", DEFAULT_RULE_SIMULATION_FORM);
    const calls: Array<[string, (fetcher: typeof fetch) => Promise<unknown>]> = [
      ["management", (fetcher) => createManagementApi(fetcher).permissions(brand)],
      ["reports", (fetcher) => createReportsApi(fetcher).betting(brand, reportQuery)],
      ["report export", (fetcher) => createReportExportApi(fetcher).betting(brand, reportQuery)],
      ["settlement preview", (fetcher) => createSettlementPreviewApi({ fetch: fetcher }).context(brand, target)],
      ["commission cycles", (fetcher) => createCommissionCyclesApi(fetcher).list(brand)],
      ["commission cycle write", (fetcher) => createCommissionCyclesApi(fetcher).retryCycle(brand, target, { version: 1, reason: "Retry" }, "retry-key")],
      ["settlement job", (fetcher) => createSettlementJobApi({ fetch: fetcher }).policy(brand)],
      ["brand operation", (fetcher) => createBrandOperationApi(fetcher).get(brand)],
      ["period cancellation", (fetcher) => createPeriodCancellationApi(fetcher).getCancellation(brand, target)],
      ["draw management", (fetcher) => createDrawManagementApi(fetcher).getGames(brand)],
      ["agents", (fetcher) => createAgentsApi(fetcher).policy(brand)],
      ["rule simulation", (fetcher) => createRuleSimulationApi(fetcher).simulate(brand, ruleBody, "simulation-key")],
      ["notification delivery", (fetcher) => createNotificationDeliveryApi(fetcher).list(brand)],
      ["bet management", (fetcher) => createBetManagementApi(fetcher).getOrders(brand)],
      ["notification templates", (fetcher) => createNotificationTemplatesApi(fetcher).list(brand)],
      ["reconciliation", (fetcher) => createReconciliationApi(fetcher).list(brand)],
      ["join codes", (fetcher) => createJoinCodesApi(fetcher).list(brand)],
      ["brand presentation", (fetcher) => createBrandPresentationApi(fetcher).get(brand)],
      ["report archives", (fetcher) => createReportArchivesApi(fetcher).list(brand)],
      ["report archive write", (fetcher) => createReportArchivesApi(fetcher).create(brand, { kind: "daily", period_key: "2026-01-01", expected_revision: 0, reason: "Create" }, "archive-key", target)],
      ["period schedules", (fetcher) => createPeriodSchedulesApi(fetcher).getGames(brand)],
      ["compliance", (fetcher) => createCompliancePolicyApi(fetcher).get(brand)],
      ["business inventory", (fetcher) => createBusinessInventoryApi(fetcher).read(brand)],
      ["commission policy", (fetcher) => createCommissionPolicyApi(fetcher).getPolicy(brand)],
      ["brand domains", (fetcher) => createBrandDomainsApi(fetcher).get(brand)],
      ["commission payments", (fetcher) => createCommissionPaymentsApi(fetcher).getPolicy(brand)],
    ];

    for (const [name, call] of calls) {
      await expect(call(oldStringError()), name).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
      const currentError = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: "PERMISSION_DENIED", message: "Current denial" } }), { status: 403 }));
      await expect(call(currentError), name).rejects.toMatchObject({ status: 403, code: "PERMISSION_DENIED", message: "Current denial" });
    }
  });
});
