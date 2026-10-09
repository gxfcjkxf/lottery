import { AdminApiError, type AdminAccount } from "./admin-api";
import { reconciliationPermissions } from "./reconciliation-api";

export const BUSINESS_INVENTORY_SOURCES = [
  "bet_order_exceptions", "bet_order_judgments", "bet_orders", "commission_adjustment_heads", "commission_adjustments",
  "commission_allocations", "commission_calculations", "commission_correction_balance_heads", "commission_correction_cycle_holds",
  "commission_correction_execution_steps", "commission_correction_execution_targets", "commission_correction_executions",
  "commission_correction_plan_steps", "commission_correction_plan_targets", "commission_correction_plans", "commission_cycle_steps",
  "commission_cycle_targets", "commission_cycles", "commission_earnings", "commission_payment_targets", "commission_payments",
  "commission_runs", "draw_correction_failures", "draw_correction_targets", "draw_corrections", "period_cancellation_targets",
  "period_cancellations", "point_accounts", "point_buckets", "point_ledger_entries", "recharge_orders", "reward_order_actions",
  "reward_orders", "settlement_calculations", "settlement_failures", "settlement_jobs", "settlement_targets",
  "withdrawal_operation_receipts", "withdrawal_order_transitions", "withdrawal_orders", "withdrawal_turnover_cycles",
] as const;

export type BusinessInventorySource = typeof BUSINESS_INVENTORY_SOURCES[number];
export type BusinessInventoryIssueCode = "MISSING_PARENT_REFERENCE" | "MISSING_REQUIRED_LEDGER_REFERENCE";

export interface BusinessInventoryCoverage {
  source_table: BusinessInventorySource;
  source_row_count: string;
  reference_count: string;
  issue_count: string;
}

export interface BusinessInventoryIssue {
  source_table: BusinessInventorySource;
  source_id: string;
  code: BusinessInventoryIssueCode;
  reference_key: string;
  parent_table: string;
}

/** The closed data object returned by the success/data admin API envelope. */
export interface BrandBusinessInventory {
  brand_id: string;
  snapshot_at: string;
  schema_version: 1;
  source_row_count: string;
  reference_count: string;
  issue_count: string;
  issues_truncated: boolean;
  consistent: boolean;
  fingerprint: string;
  coverage: BusinessInventoryCoverage[];
  issues: BusinessInventoryIssue[];
}

export interface BusinessInventoryApi {
  read(brandId: string): Promise<BrandBusinessInventory>;
}

const ENDPOINT = "/api/v1/admin/reconciliations/business-inventory";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const DECIMAL_RE = /^(0|[1-9][0-9]*)$/;
const IDENTIFIER_RE = /^[a-z][a-z0-9_]{0,62}$/;
const ISSUE_CODES: readonly BusinessInventoryIssueCode[] = ["MISSING_PARENT_REFERENCE", "MISSING_REQUIRED_LEDGER_REFERENCE"];
const INVENTORY_FIELDS = ["brand_id", "snapshot_at", "schema_version", "source_row_count", "reference_count", "issue_count", "issues_truncated", "consistent", "fingerprint", "coverage", "issues"] as const;

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function exact(value: Record<string, unknown>, fields: readonly string[]): boolean {
  const actual = Object.keys(value).sort(), expected = [...fields].sort();
  return actual.length === expected.length && actual.every((field, index) => field === expected[index]);
}

function decimal(value: unknown): value is string {
  return typeof value === "string" && value.length <= 200 && DECIMAL_RE.test(value);
}

function uuid(value: unknown): value is string {
  return typeof value === "string" && UUID_RE.test(value);
}

function sameUuid(left: unknown, right: string): boolean {
  return typeof left === "string" && left.toLowerCase() === right.toLowerCase();
}

function normalizedUtc(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?Z$/.exec(value);
  if (!match) return false;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText] = match;
  const year = Number(yearText), month = Number(monthText), day = Number(dayText);
  const daysInMonth = month === 2
    ? year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28
    : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return year > 0 && month >= 1 && month <= 12 && day >= 1 && day <= daysInMonth &&
    Number(hourText) <= 23 && Number(minuteText) <= 59 && Number(secondText) <= 59 && Number.isFinite(Date.parse(value));
}

function compareText(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0;
}

function issueOrder(issue: BusinessInventoryIssue): readonly string[] {
  return [issue.source_table, issue.source_id, issue.reference_key, issue.code, issue.parent_table];
}

function issuesAreStrictlySorted(issues: readonly BusinessInventoryIssue[]): boolean {
  for (let index = 1; index < issues.length; index++) {
    const previous = issueOrder(issues[index - 1]!);
    const current = issueOrder(issues[index]!);
    for (let field = 0; field < previous.length; field++) {
      const order = compareText(previous[field]!, current[field]!);
      if (order > 0) return false;
      if (order < 0) break;
      if (field === previous.length - 1) return false;
    }
  }
  return true;
}

function invalidInput(): never {
  throw new AdminApiError("Business inventory request parameters are invalid.", 0, "INVALID_INPUT");
}

function invalidResponse(): never {
  throw new AdminApiError("Business inventory response is invalid.", 502, "INVALID_RESPONSE");
}

function validateInventory(value: unknown, brandId: string): BrandBusinessInventory {
  if (!isRecord(value) || !exact(value, INVENTORY_FIELDS) || !sameUuid(value.brand_id, brandId) ||
      !normalizedUtc(value.snapshot_at) || value.schema_version !== 1 ||
      !decimal(value.source_row_count) || BigInt(value.source_row_count) > 100_000n ||
      !decimal(value.reference_count) || !decimal(value.issue_count) ||
      typeof value.issues_truncated !== "boolean" || typeof value.consistent !== "boolean" ||
      typeof value.fingerprint !== "string" || !/^[0-9a-f]{64}$/.test(value.fingerprint) ||
      !Array.isArray(value.coverage) || value.coverage.length !== BUSINESS_INVENTORY_SOURCES.length ||
      !Array.isArray(value.issues) || value.issues.length > 100) invalidResponse();

  let sourceRows = 0n, references = 0n, issues = 0n;
  const coverage = value.coverage as unknown[];
  for (let index = 0; index < BUSINESS_INVENTORY_SOURCES.length; index++) {
    const row = coverage[index];
    if (!isRecord(row) || !exact(row, ["source_table", "source_row_count", "reference_count", "issue_count"]) ||
        row.source_table !== BUSINESS_INVENTORY_SOURCES[index] || !decimal(row.source_row_count) ||
        !decimal(row.reference_count) || !decimal(row.issue_count) ||
        BigInt(row.issue_count) > BigInt(row.reference_count) ||
        (row.source_row_count === "0" && row.reference_count !== "0")) invalidResponse();
    sourceRows += BigInt(row.source_row_count);
    references += BigInt(row.reference_count);
    issues += BigInt(row.issue_count);
  }
  if (sourceRows !== BigInt(value.source_row_count) || references !== BigInt(value.reference_count) || issues !== BigInt(value.issue_count) ||
      BigInt(value.issue_count) > BigInt(value.reference_count)) invalidResponse();

  const totalIssues = BigInt(value.issue_count);
  if (value.issues.length !== Number(totalIssues < 100n ? totalIssues : 100n) ||
      value.issues_truncated !== (totalIssues > 100n) || value.consistent !== (totalIssues === 0n)) invalidResponse();

  const validIssues: BusinessInventoryIssue[] = [];
  const coverageBySource = new Map((coverage as BusinessInventoryCoverage[]).map((row) => [row.source_table, row]));
  const displayedBySource = new Map<BusinessInventorySource, bigint>();
  for (const issue of value.issues) {
    if (!isRecord(issue) || !exact(issue, ["source_table", "source_id", "code", "reference_key", "parent_table"]) ||
        !(BUSINESS_INVENTORY_SOURCES as readonly unknown[]).includes(issue.source_table) ||
        typeof issue.source_id !== "string" || !/^[A-Za-z0-9_./:-]{1,500}$/.test(issue.source_id) ||
        !(ISSUE_CODES as readonly unknown[]).includes(issue.code) ||
        typeof issue.reference_key !== "string" || !IDENTIFIER_RE.test(issue.reference_key) ||
        typeof issue.parent_table !== "string" || !IDENTIFIER_RE.test(issue.parent_table)) invalidResponse();
    const typedIssue = issue as unknown as BusinessInventoryIssue;
    validIssues.push(typedIssue);
    displayedBySource.set(typedIssue.source_table, (displayedBySource.get(typedIssue.source_table) ?? 0n) + 1n);
  }
  if (!issuesAreStrictlySorted(validIssues) || [...displayedBySource].some(([source, count]) => count > BigInt(coverageBySource.get(source)!.issue_count))) invalidResponse();
  return value as unknown as BrandBusinessInventory;
}

export function businessInventoryPermissions(account: AdminAccount, brandId: string): { view: boolean } {
  return { view: reconciliationPermissions(account, brandId).view };
}

export function createBusinessInventoryApi(fetchImpl: typeof fetch = fetch): BusinessInventoryApi {
  return {
    async read(brandId) {
      if (!uuid(brandId)) invalidInput();
      const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brandId });
      let response: Response;
      try {
        response = await fetchImpl(ENDPOINT, { method: "GET", credentials: "same-origin", headers });
      } catch (cause) {
        throw new AdminApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "NETWORK_ERROR");
      }
      let envelope: unknown;
      try { envelope = await response.json(); }
      catch {
        if (!response.ok) throw new AdminApiError(`Request failed (${response.status})`, response.status);
        invalidResponse();
      }
      if (!response.ok || !isRecord(envelope) || envelope.success !== true || envelope.data === undefined) {
        const detail = isRecord(envelope) && isRecord(envelope.error) ? envelope.error : undefined;
        const error = isRecord(envelope) && typeof envelope.error === "string" ? envelope.error
          : typeof detail?.message === "string" ? detail.message : `Request failed (${response.status})`;
        if (!response.ok) throw new AdminApiError(error, response.status, typeof detail?.code === "string" ? detail.code : undefined);
        invalidResponse();
      }
      return validateInventory(envelope.data, brandId);
    },
  };
}
