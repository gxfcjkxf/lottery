import { readFileSync } from "node:fs";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createRenderer, h, nextTick, reactive, type Component } from "vue";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import * as VueRuntime from "vue";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import * as InventoryApi from "./business-inventory-api";
import * as ReconciliationState from "./reconciliation-state";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { adminI18nKey, createAdminI18n } from "./i18n";
import { clearPendingReconciliationWrites } from "./reconciliation-state";
import { BUSINESS_INVENTORY_SOURCES, type BrandBusinessInventory } from "./business-inventory-api";

const brand = "11111111-1111-4111-8111-111111111111";
const anotherBrand = "33333333-3333-4333-8333-333333333333";
const actor = "22222222-2222-4222-8222-222222222222";
const account: AdminAccount = {
  id: actor, super_admin: false, brand_ids: [brand], permissions: [],
  permissions_by_brand: { [brand]: ["wallet.view.brand"] },
};

type HostNode = {
  tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode;
  value?: string; checked?: boolean; addEventListener: (...args: unknown[]) => void;
  removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[];
};
function element(tag: string): HostNode {
  return { tag, props: {}, children: [], text: "", value: "", checked: false, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } };
}
const renderer = createRenderer<HostNode, HostNode>({
  createElement: element,
  createText: (text) => ({ ...element("#text"), text }),
  createComment: (text) => ({ ...element("#comment"), text }),
  setText: (node, text) => { node.text = text; },
  setElementText: (node, text) => { node.text = text; node.children = []; },
  patchProp: (node, key, _oldValue, value) => { node.props[key] = value; if (key === "value") node.value = String(value ?? ""); },
  insert: (node, parent, anchor) => {
    if (node.parent) { const index = node.parent.children.indexOf(node); if (index >= 0) node.parent.children.splice(index, 1); }
    node.parent = parent;
    const index = anchor ? parent.children.indexOf(anchor) : -1;
    if (index < 0) parent.children.push(node); else parent.children.splice(index, 0, node);
  },
  remove: (node) => { if (!node.parent) return; const index = node.parent.children.indexOf(node); if (index >= 0) node.parent.children.splice(index, 1); node.parent = undefined; },
  parentNode: (node) => node.parent ?? null,
  nextSibling: (node) => node.parent ? node.parent.children[node.parent.children.indexOf(node) + 1] ?? null : null,
});

function find(node: HostNode, predicate: (item: HostNode) => boolean): HostNode | null {
  if (predicate(node)) return node;
  for (const child of node.children) { const found = find(child, predicate); if (found) return found; }
  return null;
}
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
async function flush() { for (let i = 0; i < 24; i++) await Promise.resolve(); await nextTick(); }

function compileComponent(api: unknown): Component {
  const source = readFileSync(new URL("./BrandBusinessInventory.vue", import.meta.url), "utf8");
  const script = compileScript(parse(source, { filename: "BrandBusinessInventory.vue" }).descriptor, { id: "brand-business-inventory-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(script, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const inventoryModule = { ...InventoryApi, createBusinessInventoryApi: () => api };
  const modules: Record<string, unknown> = {
    vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n,
    "./business-inventory-api": inventoryModule, "./reconciliation-state": ReconciliationState,
  };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) =>
    `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}

function sample(patch: Partial<BrandBusinessInventory> = {}): BrandBusinessInventory {
  const issue = { source_table: "bet_orders", source_id: "brand-1/order-0", code: "MISSING_PARENT_REFERENCE" as const, reference_key: "reference_000", parent_table: "members" } as const;
  return {
    brand_id: brand, snapshot_at: "2026-10-09T08:30:00.123456Z", schema_version: 1, source_row_count: "17",
    reference_count: "104", issue_count: "1", issues_truncated: false, consistent: false, fingerprint: "f".repeat(64),
    coverage: BUSINESS_INVENTORY_SOURCES.map((source) => ({ source_table: source,
      source_row_count: source === "bet_orders" ? "17" : "0", reference_count: source === "bet_orders" ? "104" : "0", issue_count: source === "bet_orders" ? "1" : "0" })),
    issues: [issue], ...patch,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function mount(api: unknown, initialAccount: AdminAccount = account) {
  const ComponentUnderTest = compileComponent(api);
  const scope = reactive({ account: initialAccount, brandId: brand });
  const emitted: string[] = [];
  const root = element("root"), context = createAdminI18n();
  context.setLocale("en");
  const app = renderer.createApp({ setup: () => () => h(ComponentUnderTest, { ...scope, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, context);
  app.mount(root);
  return { app, root, context, scope, emitted };
}

function button(root: HostNode, label: string): HostNode {
  const result = find(root, (node) => node.tag === "button" && textOf(node).includes(label));
  if (!result) throw new Error(`Button not found: ${label}`);
  return result;
}
function click(node: HostNode) { (node.props.onClick as (() => void) | undefined)?.(); }

afterEach(() => { clearPendingReconciliationWrites(); vi.clearAllMocks(); vi.unstubAllGlobals(); });

describe("BrandBusinessInventory real Vue component", () => {
  it("waits for explicit load and renders bilingual coverage, composite issue IDs, and the full issue count", async () => {
    const allShown = Array.from({ length: 100 }, (_, index) => ({
      source_table: "bet_orders" as const,
      source_id: `brand-1/order-${Math.floor(index / 34)}`,
      code: "MISSING_PARENT_REFERENCE" as const,
      reference_key: `reference_${String(index).padStart(3, "0")}`,
      parent_table: "members",
    }));
    const read = vi.fn().mockResolvedValue(sample({ issue_count: "103", issues_truncated: true, issues: allShown,
      coverage: sample().coverage.map((row) => row.source_table === "bet_orders" ? { ...row, issue_count: "103" } : row) }));
    const mounted = mount({ read });
    await flush();
    expect(read).not.toHaveBeenCalled();
    click(button(mounted.root, "Load observation"));
    await flush();
    let output = textOf(mounted.root);
    expect(read).toHaveBeenCalledWith(brand);
    expect(output).toContain("Brand business reference inventory");
    expect(output).toContain("104");
    expect(output).toContain("103 issues total");
    expect(output).toContain("with 3 more not displayed");
    expect(output).toContain("brand-1/order-0");
    expect(output).toContain("members");
    expect(output).toContain("41 tables");
    expect(output).toContain("not a complete wallet or financial proof");
    expect(output).not.toContain("parent_id");
    mounted.context.setLocale("zh-CN");
    await nextTick();
    output = textOf(mounted.root);
    expect(output).toContain("品牌业务引用检查");
    expect(output).toContain("另有 3 个未显示");
    expect(output).toContain("不是完整钱包或财务证明");
    mounted.app.unmount();
  });

  it("clears on refresh and drops a late response after the brand scope changes", async () => {
    const pending = deferred<BrandBusinessInventory>();
    const read = vi.fn().mockResolvedValueOnce(sample()).mockReturnValueOnce(pending.promise);
    const mounted = mount({ read });
    click(button(mounted.root, "Load observation"));
    await flush();
    expect(textOf(mounted.root)).toContain("brand-1/order-0");
    click(button(mounted.root, "Load a fresh observation"));
    await nextTick();
    expect(textOf(mounted.root)).not.toContain("brand-1/order-0");
    expect(textOf(mounted.root)).toContain("Loading the current brand observation");
    mounted.scope.brandId = anotherBrand;
    await flush();
    pending.reject(new AdminApiError("expired", 401));
    await flush();
    expect(textOf(mounted.root)).not.toContain("brand-1/order-0");
    expect(mounted.emitted).toEqual([]);
    expect(read).toHaveBeenCalledTimes(2);
    mounted.app.unmount();
  });

  it("clears the old snapshot on a failed refresh and emits session-invalid only for a current 401", async () => {
    const pending = deferred<BrandBusinessInventory>();
    const read = vi.fn().mockResolvedValueOnce(sample()).mockReturnValueOnce(pending.promise);
    const mounted = mount({ read });
    click(button(mounted.root, "Load observation"));
    await flush();
    click(button(mounted.root, "Load a fresh observation"));
    await nextTick();
    expect(textOf(mounted.root)).not.toContain("brand-1/order-0");
    pending.reject(new AdminApiError("expired", 401));
    await flush();
    expect(textOf(mounted.root)).toContain("The admin session has expired");
    expect(mounted.emitted).toEqual(["session-invalid"]);
    mounted.app.unmount();
  });

  it("discards a late 401 after the admin session generation changes", async () => {
    const pending = deferred<BrandBusinessInventory>();
    const read = vi.fn().mockReturnValue(pending.promise);
    const mounted = mount({ read });
    click(button(mounted.root, "Load observation"));
    await nextTick();
    clearPendingReconciliationWrites();
    pending.reject(new AdminApiError("expired", 401));
    await flush();
    expect(textOf(mounted.root)).not.toContain("The admin session has expired");
    expect(mounted.emitted).toEqual([]);
    expect(textOf(mounted.root)).toContain("Choose “Load observation”");
    mounted.app.unmount();
  });

  it("honors view-only and platform view grants without inferring access from super-admin", async () => {
    const read = vi.fn().mockResolvedValue(sample());
    const noView = { ...account, super_admin: true, permissions_by_brand: {}, platform_permissions: [] };
    const mounted = mount({ read }, noView);
    await flush();
    expect(read).not.toHaveBeenCalled();
    expect(textOf(mounted.root)).toContain("lacks wallet.view.brand or wallet.view.platform");
    expect(button(mounted.root, "Load observation").props.disabled).toBe(true);
    mounted.scope.account = { ...noView, platform_permissions: ["wallet.view.platform"] };
    await flush();
    expect(textOf(mounted.root)).toContain("Choose “Load observation”");
    expect(read).not.toHaveBeenCalled();
    click(button(mounted.root, "Load observation"));
    await flush();
    expect(read).toHaveBeenCalledWith(brand);
    expect(textOf(mounted.root)).toContain("brand-1/order-0");
    mounted.app.unmount();
  });
});
