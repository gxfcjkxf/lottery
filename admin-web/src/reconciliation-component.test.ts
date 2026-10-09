import { readFileSync } from "node:fs";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createRenderer, h, nextTick, type Component } from "vue";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import * as VueRuntime from "vue";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import * as BusinessInventoryApi from "./business-inventory-api";
import * as ReconciliationApi from "./reconciliation-api";
import * as ReconciliationState from "./reconciliation-state";
import type { AdminAccount } from "./admin-api";
import { adminI18nKey, createAdminI18n } from "./i18n";
import { clearPendingReconciliationWrites } from "./reconciliation-state";

const brand = "11111111-1111-4111-8111-111111111111";
const actor = "22222222-2222-4222-8222-222222222222";
const account: AdminAccount = {
  id: actor, super_admin: false, brand_ids: [brand], permissions: [],
  permissions_by_brand: { [brand]: ["wallet.view.brand", "wallet.reconcile.brand"] },
};

type HostNode = {
  tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode;
  value?: string; checked?: boolean; addEventListener: (...args: unknown[]) => void;
  removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[];
};
function element(tag: string): HostNode { return { tag, props: {}, children: [], text: "", value: "", checked: false, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } }; }
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
function compileInventoryComponent(): Component {
  const source = readFileSync(new URL("./BrandBusinessInventory.vue", import.meta.url), "utf8");
  const script = compileScript(parse(source, { filename: "BrandBusinessInventory.vue" }).descriptor, { id: "brand-business-inventory-in-reconciliation-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(script, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = {
    vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n,
    "./business-inventory-api": BusinessInventoryApi, "./reconciliation-state": ReconciliationState,
  };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) =>
    `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
function compilePage(api: unknown): Component {
  const source = readFileSync(new URL("./ReconciliationManagement.vue", import.meta.url), "utf8");
  const script = compileScript(parse(source, { filename: "ReconciliationManagement.vue" }).descriptor, { id: "reconciliation-component-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(script, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = {
    vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n,
    "./reconciliation-api": { ...ReconciliationApi, createReconciliationApi: () => api },
    "./reconciliation-state": ReconciliationState,
    "./BrandBusinessInventory.vue": { default: compileInventoryComponent() },
  };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) =>
    `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}

afterEach(() => { clearPendingReconciliationWrites(); vi.clearAllMocks(); vi.unstubAllGlobals(); });

describe("ReconciliationManagement scope confirmation", () => {
  it("freezes full scope into an unknown create and locks the selector until exact replay", async () => {
    const create = vi.fn().mockRejectedValue(new AdminApi.AdminApiError("offline", 0, "NETWORK_ERROR"));
    const api = { list: vi.fn().mockResolvedValue({ items: [], total_count: "0", limit: 20, offset: 0 }), create };
    const ComponentUnderTest = compilePage(api);
    vi.stubGlobal("Document", class {});
    vi.stubGlobal("ShadowRoot", class {});
    const root = element("root"), context = createAdminI18n();
    context.setLocale("en");
    const app = renderer.createApp({ render: () => h(ComponentUnderTest, { account, brandId: brand }) });
    app.provide(adminI18nKey, context);
    app.mount(root);
    await flush();
    expect(textOf(root)).toContain("Brand business reference inventory");
    expect(textOf(root)).toContain("Choose “Load observation” to read it.");

    const selector = find(root, (node) => node.tag === "select" && String(node.props["aria-label"]).includes("Check scope"));
    (selector?.props["onUpdate:modelValue"] as ((value: string) => void) | undefined)?.("wallet_and_business");
    const reason = find(root, (node) => node.tag === "textarea" && String(node.props["aria-label"]).includes("Reason for operation"));
    (reason?.props["onUpdate:modelValue"] as ((value: string) => void) | undefined)?.("explicit business association audit");
    await flush();
    const review = find(root, (node) => node.tag === "button" && typeof node.props.onClick === "function" && textOf(node).includes("Review and confirm submission"));
    (review?.props.onClick as (() => void) | undefined)?.();
    await nextTick();
    expect(textOf(root)).toContain("Wallet + business associations");

    const checkbox = find(root, (node) => node.tag === "input" && node.props.type === "checkbox");
    (checkbox?.props["onUpdate:modelValue"] as ((value: boolean) => void) | undefined)?.(true);
    await nextTick();
    const submit = find(root, (node) => node.tag === "button" && typeof node.props.onClick === "function" && textOf(node).includes("Confirm and submit"));
    (submit?.props.onClick as (() => void) | undefined)?.();
    await flush();

    expect(create).toHaveBeenCalledTimes(1);
    expect(create.mock.calls[0]?.[0]).toBe(brand);
    expect(create.mock.calls[0]?.[1]).toEqual({ reason: "explicit business association audit", check_scope: "wallet_and_business" });
    expect(create.mock.calls[0]?.[3]).toBe(actor);
    const locked = find(root, (node) => node.tag === "select" && String(node.props["aria-label"]).includes("Check scope"));
    expect(locked?.props.disabled).toBe(true);
    expect(textOf(root)).toContain("The write result is unknown");
    expect(textOf(root)).toContain("Wallet + business associations");
    app.unmount();
  });
});
