import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import ts from "typescript";
import { compileScript, parse } from "vue/compiler-sfc";
import * as AdminApi from "./admin-api";
import { AdminApiError, type AdminAccount } from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import * as ArchiveApi from "./report-archives-api";
import type { ReportArchiveCreateInput, ReportArchiveRecord } from "./report-archives-api";
import * as ArchiveState from "./report-archives-state";

const brand = "11111111-1111-4111-8111-111111111111";
const actor = "33333333-3333-4333-8333-333333333333";
const archiveId = "44444444-4444-4444-8444-444444444444";
const auditId = "55555555-5555-4555-8555-555555555555";
const stamp = "2026-10-03T00:00:00Z";
const permissions = ["report_archive.view.brand", "report_archive.create.brand", "report_archive.download.brand"];
const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: permissions } };
const calls: Array<{ method: string; args: unknown[] }> = [];
let record: ReportArchiveRecord;
let nextCreate: (() => Promise<unknown>) | null = null;
let listFailure: unknown = null;
let readFailure: unknown = null;
let delayedDownload: ReturnType<typeof deferred<ArchiveApi.ReportArchiveDownload>> | null = null;

function fullRecord(): ReportArchiveRecord {
  const zeros = (names: string[]) => Object.fromEntries(names.map((name) => [name, "0"])) as Record<string, string>;
  const snapshot = {
    brand_id: brand, format_version: 1 as const, snapshot_at: stamp, timezone: "Asia/Singapore",
    from: "2026-10-01T16:00:00Z", to: "2026-10-02T16:00:00Z",
    betting: { ...zeros(["order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points", "correction_open_count"]) },
    ledger: { ...zeros(["entry_count", "recharge_points", "prize_credit_points", "prize_reversal_points", "refund_points"]), net_points: "0" },
    wallet_snapshot: { at_snapshot: stamp, balances: zeros(["account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"]) },
    withdrawals: zeros(["order_count", "requested_points", "reviewing_count", "reviewing_points", "processing_count", "processing_points", "paid_count", "paid_points", "rejected_count", "rejected_points", "failed_count", "failed_points", "cancelled_count", "cancelled_points"]),
    commissions: { ...zeros(["entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count", "correction_credit_points", "correction_debit_points"]), net_points: "0" },
    rewards: { ...zeros(["entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points"]), net_points: "0" },
    reward_orders: zeros(["order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points"]),
  } as unknown as ReportArchiveRecord["snapshot"];
  return {
    id: archiveId, brand_id: brand,
    window: { kind: "daily", period_key: "2026-10-01", timezone: "Asia/Singapore", from: snapshot.from, to: snapshot.to },
    revision: 1, previous_id: null, snapshot_at: stamp, created_by: actor, reason: "Monthly finance review",
    payload_sha256: createHash("sha256").update(JSON.stringify(snapshot)).digest("hex"), audit_log_id: auditId, created_at: stamp, snapshot,
  };
}

function fresh() {
  record = fullRecord(); calls.length = 0; nextCreate = null; listFailure = null; readFailure = null; delayedDownload = null;
}
function page(items: ReportArchiveRecord[] = [record], offset = 0) { return { brand_id: brand, items, total_count: String(items.length), limit: 20, offset }; }
function apiStub() {
  return {
    list: async (...args: unknown[]) => { calls.push({ method: "list", args }); if (listFailure) throw listFailure; return page([record], Number(args[2])); },
    read: async (...args: unknown[]) => { calls.push({ method: "read", args }); if (readFailure) throw readFailure; return record; },
    create: async (...args: unknown[]) => { calls.push({ method: "create", args }); return nextCreate ? nextCreate() : record; },
    download: async (...args: unknown[]) => { calls.push({ method: "download", args }); if (delayedDownload) return delayedDownload.promise; return { bytes: new TextEncoder().encode(JSON.stringify(record.snapshot)), metadata: { sha256: record.payload_sha256, revision: record.revision, format_version: 1, filename: `report-archive-${archiveId}-v1.json` } }; },
  };
}

type HostNode = { tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode; value?: string; checked?: boolean; addEventListener: (...args: unknown[]) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[] };
function element(tag: string): HostNode { return { tag, props: {}, children: [], text: "", value: "", checked: false, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } }; }
const renderer = createRenderer<HostNode, HostNode>({
  createElement: element, createText: (text) => ({ ...element("#text"), text }), createComment: (text) => ({ ...element("#comment"), text }),
  setText: (node, text) => { node.text = text; }, setElementText: (node, text) => { node.text = text; node.children = []; },
  patchProp: (node, key, _old, value) => { node.props[key] = value; if (key === "value") node.value = String(value ?? ""); },
  insert: (node, parent, anchor) => { if (node.parent) { const i = node.parent.children.indexOf(node); if (i >= 0) node.parent.children.splice(i, 1); } node.parent = parent; const i = anchor ? parent.children.indexOf(anchor) : -1; if (i < 0) parent.children.push(node); else parent.children.splice(i, 0, node); },
  remove: (node) => { if (!node.parent) return; const i = node.parent.children.indexOf(node); if (i >= 0) node.parent.children.splice(i, 1); node.parent = undefined; },
  parentNode: (node) => node.parent ?? null, nextSibling: (node) => node.parent ? node.parent.children[node.parent.children.indexOf(node) + 1] ?? null : null,
});
function compileComponent(): Component {
  const source = readFileSync(new URL("./ReportArchivesManagement.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "ReportArchivesManagement.vue" }).descriptor;
  const compiled = compileScript(descriptor, { id: "report-archives-component-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(compiled, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./report-archives-api": ArchiveApi, "./report-archives-state": ArchiveState };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => { if (!(specifier in modules)) throw new Error(`Unmapped import ${specifier}`); return `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`; }).replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, binding: string, specifier: string) => { if (!(specifier in modules)) throw new Error(`Unmapped import ${specifier}`); return `const ${binding}=__modules[${JSON.stringify(specifier)}].default;`; }).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
const ComponentApi = vi.spyOn(ArchiveApi, "createReportArchivesApi").mockImplementation(() => apiStub() as never);
vi.spyOn(AdminApi, "createIdempotencyKey").mockReturnValue("archive-idempotency-001");
const ComponentUnderTest = compileComponent();
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
function findNode(node: HostNode, predicate: (candidate: HostNode) => boolean): HostNode | null { if (predicate(node)) return node; for (const child of node.children) { const found = findNode(child, predicate); if (found) return found; } return null; }
function byTestId(root: HostNode, id: string): HostNode | null { return findNode(root, (node) => node.props["data-testid"] === id); }
function button(root: HostNode, id: string): HostNode | null { return byTestId(root, id); }
function click(node: HostNode | null) { expect(node).not.toBeNull(); (node!.props.onClick as (() => void) | undefined)?.(); }
function model(node: HostNode | null, value: unknown) { expect(node).not.toBeNull(); (node!.props["onUpdate:modelValue"] as ((next: unknown) => void) | undefined)?.(value); }
function checked(node: HostNode | null, value: boolean) { model(node, value); }
function setInput(node: HostNode | null, value: string) { model(node, value); }
function deferred<T>() { let resolve!: (value: T) => void, reject!: (reason?: unknown) => void; const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; }); return { promise, resolve, reject }; }
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
function mount(user = account, brandId = brand) {
  vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
  const scope = ref({ account: user, brandId }), container = element("root"), emitted: string[] = [], i18n = createAdminI18n();
  const app = renderer.createApp({ setup: () => () => h(ComponentUnderTest, { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, i18n); app.mount(container); return { app, scope, container, emitted, i18n };
}
function prepareCreate(root: HostNode, reason = "Finance review for October") {
  setInput(byTestId(root, "archive-period-key"), "2026-10-01");
  setInput(byTestId(root, "archive-reason"), reason);
}
async function reviewAndSubmit(mounted: ReturnType<typeof mount>) {
  click(button(mounted.container, "archive-review-button")); await flush();
  expect(byTestId(mounted.container, "archive-review")).not.toBeNull();
  checked(byTestId(mounted.container, "archive-confirm"), true); await flush();
  click(button(mounted.container, "archive-submit")); await flush();
}
beforeEach(() => { fresh(); ArchiveState.clearAllArchiveIntents(); });
afterEach(() => { ArchiveState.clearAllArchiveIntents(); calls.length = 0; vi.clearAllMocks(); vi.unstubAllGlobals(); });

describe("ReportArchivesManagement", () => {
  it("freezes the reviewed body, actor and key, and keeps an unknown write across unmount and refresh", async () => {
    const first = mount(); await flush(); prepareCreate(first.container); await flush(); click(button(first.container, "archive-review-button")); await flush();
    const reviewedText = textOf(byTestId(first.container, "archive-review")!);
    expect(reviewedText).toContain(actor); expect(reviewedText).toContain("archive-idempotency-001"); expect(reviewedText).toContain("Finance review for October");
    checked(byTestId(first.container, "archive-confirm"), true); await flush();
    nextCreate = async () => { throw new TypeError("connection dropped after submit"); };
    click(button(first.container, "archive-submit")); await flush();
    const createCall = calls.find((call) => call.method === "create")!;
    expect(createCall.args).toEqual([brand, { kind: "daily", period_key: "2026-10-01", expected_revision: 0, reason: "Finance review for October" }, "archive-idempotency-001", actor]);
    expect(Object.isFrozen(ArchiveState.getArchiveIntent(actor, brand))).toBe(true);
    expect(Object.isFrozen(ArchiveState.getArchiveIntent(actor, brand)?.body)).toBe(true);
    expect(byTestId(first.container, "archive-unknown")).not.toBeNull();
    click(button(first.container, "archive-refresh")); await flush(); expect(byTestId(first.container, "archive-unknown")).not.toBeNull();
    first.app.unmount();
    const second = mount(); await flush();
    expect(byTestId(second.container, "archive-unknown")).not.toBeNull();
    expect(textOf(byTestId(second.container, "archive-unknown")!)).toContain("archive-idempotency-001");
    expect(button(second.container, "archive-review-button")?.props.disabled).toBe(true);
    second.app.unmount();
  });

  it("keeps the server ACK receipt when its follow-up list and detail reads fail", async () => {
    const mounted = mount(); await flush(); prepareCreate(mounted.container); await flush();
    nextCreate = async () => record; listFailure = new AdminApiError("list follow-up failed", 503); readFailure = new AdminApiError("detail follow-up failed", 503);
    await reviewAndSubmit(mounted);
    expect(byTestId(mounted.container, "archive-receipt")).not.toBeNull();
    expect(textOf(byTestId(mounted.container, "archive-receipt")!)).toContain(archiveId);
    expect(textOf(byTestId(mounted.container, "archive-list-error")!)).toContain("list follow-up failed");
    expect(textOf(byTestId(mounted.container, "archive-record-error")!)).toContain("detail follow-up failed");
    expect(textOf(byTestId(mounted.container, "archive-receipt")!)).toContain("Monthly finance review");
    expect(ArchiveState.getArchiveIntent(actor, brand)).toBeNull();
    expect(calls.filter((call) => call.method === "create")).toHaveLength(1);
    mounted.app.unmount();
  });

  it("requires successful conflict refresh and explicit human confirmation before discarding", async () => {
    const mounted = mount(); await flush(); prepareCreate(mounted.container); await flush();
    nextCreate = async () => { throw new AdminApiError("revision conflict", 409); };
    await reviewAndSubmit(mounted);
    expect(byTestId(mounted.container, "archive-conflict")).not.toBeNull();
    expect((byTestId(mounted.container, "archive-discard")!.props.disabled)).toBe(true);
    listFailure = new AdminApiError("refresh failed", 503);
    click(button(mounted.container, "archive-conflict-refresh")); await flush();
    expect(byTestId(mounted.container, "archive-discard-check")!.props.disabled).toBe(true);
    expect(byTestId(mounted.container, "archive-discard")!.props.disabled).toBe(true);
    listFailure = null; click(button(mounted.container, "archive-conflict-refresh")); await flush();
    expect(byTestId(mounted.container, "archive-discard-check")!.props.disabled).toBe(false);
    expect(byTestId(mounted.container, "archive-discard")!.props.disabled).toBe(true);
    checked(byTestId(mounted.container, "archive-discard-check"), true); await flush();
    expect(byTestId(mounted.container, "archive-discard")!.props.disabled).toBe(false);
    click(button(mounted.container, "archive-discard")); await flush();
    expect(ArchiveState.getArchiveIntent(actor, brand)).toBeNull();
    mounted.app.unmount();
  });

  it("clears all in-memory intents and emits session-invalid for a current 401", async () => {
    ArchiveState.setArchiveIntent(ArchiveState.createArchiveIntent(actor, brand, { kind: "daily", period_key: "2026-10-01", expected_revision: 0, reason: "Existing request" }, "existing-key-01")!);
    const mounted = mount(); await flush();
    listFailure = new AdminApiError("expired", 401); click(button(mounted.container, "archive-refresh")); await flush();
    expect(ArchiveState.getArchiveIntent(actor, brand)).toBeNull(); expect(mounted.emitted).toEqual(["session-invalid"]);
    mounted.app.unmount();
  });

  it("does not let an old-generation late 401 clear a new session intent", async () => {
    const pending = deferred<unknown>(); const mounted = mount(); await flush(); prepareCreate(mounted.container); await flush();
    nextCreate = () => pending.promise; await reviewAndSubmit(mounted);
    expect(ArchiveState.getArchiveIntent(actor, brand)).not.toBeNull();
    ArchiveState.clearAllArchiveIntents();
    const newIntent = ArchiveState.createArchiveIntent(actor, brand, { kind: "monthly", period_key: "2026-09", expected_revision: 1, reason: "New login request" }, "new-session-key-01")!;
    ArchiveState.setArchiveIntent(newIntent);
    pending.reject(new AdminApiError("old session expired", 401)); await flush();
    expect(ArchiveState.getArchiveIntent(actor, brand)?.key).toBe("new-session-key-01");
    expect(mounted.emitted).toEqual([]); mounted.app.unmount();
  });

  it("retains a late conflict against the matching off-page intent without replacing it", async () => {
    const pending = deferred<unknown>(); const originalMount = mount(); await flush(); prepareCreate(originalMount.container); await flush();
    nextCreate = () => pending.promise; await reviewAndSubmit(originalMount);
    const original = ArchiveState.getArchiveIntent(actor, brand)!;
    originalMount.app.unmount();
    const remounted = mount(); await flush();
    expect(byTestId(remounted.container, "archive-unknown")).not.toBeNull();
    pending.reject(new AdminApiError("stale revision", 409)); await flush();
    expect(ArchiveState.getArchiveIntent(actor, brand)?.key).toBe(original.key);
    expect(ArchiveState.isArchiveConflict(ArchiveState.getArchiveIntent(actor, brand)!)).toBe(true);
    expect(byTestId(remounted.container, "archive-conflict")).not.toBeNull();
    expect(button(remounted.container, "archive-replay")).toBeNull();
    expect(byTestId(remounted.container, "archive-discard")!.props.disabled).toBe(true);
    remounted.app.unmount();
  });

  it("denies super-admin brand access and allows a mapped reader to download without create permission", async () => {
    const platformSuper: AdminAccount = { id: actor, super_admin: true, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: permissions }, platform_permissions: ["report_archive.view.platform", "report_archive.download.platform"] };
    const denied = mount(platformSuper); denied.i18n.setLocale("en"); await flush();
    expect(textOf(denied.container)).toContain("No archive viewing permission for this brand");
    expect(button(denied.container, "archive-review-button")).toBeNull();
    expect(calls.some((call) => call.method === "list")).toBe(false);
    expect(calls.some((call) => call.method === "download")).toBe(false);
    denied.app.unmount();

    const mappedReader: AdminAccount = { ...account, permissions_by_brand: { [brand]: ["report_archive.view.brand", "report_archive.download.brand"] } };
    const mounted = mount(mappedReader); mounted.i18n.setLocale("en"); await flush();
    expect(textOf(mounted.container)).toContain("can view but cannot create archives");
    expect(button(mounted.container, "archive-review-button")!.props.disabled).toBe(true);
    expect(calls.some((call) => call.method === "list")).toBe(true);
    const row = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("2026-10-01"));
    click(row); await flush();
    expect(button(mounted.container, "archive-download")!.props.disabled).toBe(false);
    click(button(mounted.container, "archive-download")); await flush();
    expect(calls.some((call) => call.method === "download")).toBe(true);
    expect(calls.some((call) => call.method === "create")).toBe(false);
    mounted.app.unmount();
  });

  it("passes a plain cloned record to the SDK and suppresses late downloads after selection, scope, rights or unmount changes", async () => {
    const createObjectURL = vi.fn(() => "blob:test"), clickLink = vi.fn();
    vi.stubGlobal("URL", { createObjectURL, revokeObjectURL: vi.fn() });
    vi.stubGlobal("document", { createElement: vi.fn(() => ({ href: "", download: "", click: clickLink })) });
    const mounted = mount(); await flush();
    const rowButton = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("2026-10-01"));
    click(rowButton); await flush(); click(button(mounted.container, "archive-download")); await flush();
    const downloadCall = calls.find((call) => call.method === "download")!;
    const earlyClone = downloadCall.args[2] as ReportArchiveRecord;
    expect(earlyClone).not.toBe(record);
    expect(Object.getPrototypeOf(earlyClone)).toBe(Object.prototype);
    expect(() => structuredClone(earlyClone)).not.toThrow();
    expect(createObjectURL).toHaveBeenCalledTimes(1); expect(clickLink).toHaveBeenCalledTimes(1);

    for (const change of ["selection", "brand", "permissions", "unmount"] as const) {
      const delayed = deferred<ArchiveApi.ReportArchiveDownload>(); delayedDownload = delayed;
      const currentMount = change === "unmount" ? mounted : mount(); await flush();
      if (currentMount !== mounted) {
        const row = findNode(currentMount.container, (node) => node.tag === "button" && textOf(node).includes("2026-10-01")); click(row); await flush();
      } else {
        // The first selection remains selected after its successful download.
      }
      click(button(currentMount.container, "archive-download")); await flush();
      if (change === "selection") { const row = findNode(currentMount.container, (node) => node.tag === "button" && textOf(node).includes("2026-10-01")); click(row); await flush(); }
      if (change === "brand") { currentMount.scope.value = { account, brandId: "22222222-2222-4222-8222-222222222222" }; await flush(); }
      if (change === "permissions") { currentMount.scope.value = { account: { ...account, permissions_by_brand: { [brand]: ["report_archive.view.brand"] } }, brandId: brand }; await flush(); }
      if (change === "unmount") currentMount.app.unmount();
      const before = createObjectURL.mock.calls.length, beforeClicks = clickLink.mock.calls.length;
      delayed.resolve({ bytes: new TextEncoder().encode("{}"), metadata: { sha256: "a".repeat(64), revision: 1, format_version: 1, filename: "archive.json" } }); await flush();
      expect(createObjectURL).toHaveBeenCalledTimes(before);
      expect(clickLink).toHaveBeenCalledTimes(beforeClicks);
      if (currentMount !== mounted) currentMount.app.unmount();
      delayedDownload = null;
    }
    mounted.app.unmount();
  });

  it("renders translated English labels and locale changes do not refetch", async () => {
    ArchiveState.setArchiveIntent(ArchiveState.createArchiveIntent(actor, brand, { kind: "daily", period_key: "2026-10-01", expected_revision: 0, reason: "Reviewed by finance" }, "locale-pending-key-01")!);
    const mounted = mount(); await flush();
    const before = calls.length; mounted.i18n.setLocale("en"); await flush();
    expect(textOf(mounted.container)).toContain("Daily and monthly archives");
    expect(textOf(mounted.container)).toContain("Create a new observation version");
    expect(textOf(mounted.container)).toContain("Archive history");
    expect(textOf(mounted.container)).toContain("Unresolved creation request");
    expect(textOf(mounted.container)).toContain("Refreshing or leaving does not clear the original request.");
    expect(textOf(mounted.container)).toContain("Archive type");
    expect(textOf(mounted.container)).toContain("Archive period");
    expect(textOf(mounted.container)).toContain("Confirmed previous latest revision");
    expect(textOf(mounted.container)).toContain("Reason for operation");
    expect(textOf(mounted.container)).toContain("Review and replay original request");
    expect(textOf(mounted.container)).not.toContain("日月归档");
    expect(calls).toHaveLength(before);
    mounted.app.unmount();
  });
});
