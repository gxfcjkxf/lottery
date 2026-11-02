import { describe, expect, it, vi } from "vitest";
import { createManagementApi } from "./management-api";
import { createReportsApi } from "./reports-api";
import { createReportExportApi } from "./report-export-api";
import { createCommissionCyclesApi } from "./commission-cycles-api";
import { createBrandOperationApi } from "./brand-operation-api";
import { createAgentsApi } from "./agents-api";
import { createJoinCodesApi } from "./join-codes-api";
import { createBrandPresentationApi } from "./brand-presentation-api";
import { createCommissionPolicyApi } from "./commission-policy-api";
import { createBrandDomainsApi } from "./brand-domains-api";
import { createCommissionPaymentsApi } from "./commissionPayments-api";

import { createCommissionReportApi } from "./commission-report-api";
import { createAttributionReportApi } from "./attribution-report-api";

const brand = "00000000-0000-4000-8000-000000000001";
const target = "00000000-0000-4000-8000-000000000002";
const oldStringError = () => vi.fn<typeof fetch>().mockResolvedValue(
  new Response(JSON.stringify({ success: false, error: "legacy denial" }), { status: 400 }),
);

describe("admin API error envelope contract", () => {
  it("rejects old string errors while preserving current structured business errors", async () => {
    const reportQuery = { from: "2026-01-01T00:00:00Z", to: "2026-01-02T00:00:00Z", group_by: "day" as const };
    const calls: Array<[string, (fetcher: typeof fetch) => Promise<unknown>]> = [
      ["management", (fetcher) => createManagementApi(fetcher).permissions(brand)],
      ["reports", (fetcher) => createReportsApi(fetcher).betting(brand, reportQuery)],
      ["report export", (fetcher) => createReportExportApi(fetcher).betting(brand, reportQuery)],
      ["commission cycles", (fetcher) => createCommissionCyclesApi(fetcher).list(brand)],
      ["commission cycle write", (fetcher) => createCommissionCyclesApi(fetcher).retryCycle(brand, target, { version: 1, reason: "Retry" }, "retry-key")],
      ["brand operation", (fetcher) => createBrandOperationApi(fetcher).get(brand)],
      ["agents", (fetcher) => createAgentsApi(fetcher).policy(brand)],
      ["join codes", (fetcher) => createJoinCodesApi(fetcher).list(brand)],
      ["brand presentation", (fetcher) => createBrandPresentationApi(fetcher).get(brand)],
      ["commission policy", (fetcher) => createCommissionPolicyApi(fetcher).getPolicy(brand)],
      ["brand domains", (fetcher) => createBrandDomainsApi(fetcher).get(brand)],
      ["commission report", (fetcher) => createCommissionReportApi(fetcher).report(brand, reportQuery)],
      ["commission CSV", (fetcher) => createCommissionReportApi(fetcher).exportCsv(brand, reportQuery)],
      ["attribution report", (fetcher) => createAttributionReportApi(fetcher).report(brand, reportQuery)],
      ["attribution CSV", (fetcher) => createAttributionReportApi(fetcher).exportCsv(brand, reportQuery)],
      ["commission payments", (fetcher) => createCommissionPaymentsApi(fetcher).getPolicy(brand)],
    ];

    for (const [name, call] of calls) {
      await expect(call(oldStringError()), name).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
      const currentError = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: "PERMISSION_DENIED", message: "Current denial" } }), { status: 403 }));
      await expect(call(currentError), name).rejects.toMatchObject({ status: 403, code: "PERMISSION_DENIED", message: "Current denial" });
    }
  });
});
