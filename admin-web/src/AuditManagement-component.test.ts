import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import { readFileSync } from "node:fs";
import ts from "typescript";
import { compileScript, parse } from "vue/compiler-sfc";
import * as AdminApi from "./admin-api";
import type { AdminAccount } from "./admin-api";
import * as AuditApi from "./audit-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";

const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const actor = "33333333-3333-4333-8333-333333333333";
const id = "44444444-4444-4444-8444-444444444444";
const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand, otherBrand], permissions: [], permissions_by_brand: { [brand]: ["audit.view.brand", "audit.export.brand"], [otherBrand]: ["audit.view.brand", "audit.export.brand"] } };
type RecordRow = AuditApi.AdminAuditRecord;
const row: RecordRow = { id, brand_id: brand, action: "user.update", actor_type: "admin", actor_id: actor, resource_type: "user", resource_id: "resource-1", reason: "reviewed", request_id: "req-1", created_at: "2026-10-01T12:00:00Z", ip_address: "127.0.0.1", before_json: { email: "old@example.test", password: "secret" }, after_json: { email: "new@example.test" } };
let apiStub: { list: Mock<(...args: any[]) => Promise<{ items: RecordRow[] }>>; exportCsv: Mock<(...args: any[]) => Promise<unknown>> };
let apps: Array<{ unmount: () => void }> = [];

type HostNode = { tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode; value?: string; listeners: Record<string, Array<(event: any) => void>>; addEventListener: (name: string, listener: (event: any) => void) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[] };
function element(tag: string): HostNode {
  const node = { tag, props: {}, children: [], text: "", value: "", listeners: {}, addEventListener(name: string, listener: (event: any) => void) { (this.listeners[name] ??= []).push(listener); }, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter(child => child.tag === "option"); } } as HostNode;
  return node;
}
const renderer = createRenderer<HostNode, HostNode>({
  createElement: element, createText: text => ({ ...element("#text"), text }), createComment: text => ({ ...element("#comment"), text }),
  setText: (node, text) => { node.text = text; }, setElementText: (node, text) => { node.text = text; node.children = []; },
  patchProp: (node, key, _old, value) => { node.props[key] = value; if (key === "value") node.value = String(value ?? ""); },
  insert(node, parent, anchor) { if (node.parent) { const old = node.parent.children.indexOf(node); if (old >= 0) node.parent.children.splice(old, 1); } node.parent = parent; const index = anchor ? parent.children.indexOf(anchor) : -1; if (index < 0) parent.children.push(node); else parent.children.splice(index, 0, node); },
  remove(node) { if (!node.parent) return; const index = node.parent.children.indexOf(node); if (index >= 0) node.parent.children.splice(index, 1); node.parent = undefined; },
  parentNode: node => node.parent ?? null, nextSibling: node => node.parent ? node.parent.children[node.parent.children.indexOf(node) + 1] ?? null : null,
});
function compileComponent(): Component {
  const source = readFileSync(new URL("./AuditManagement.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "AuditManagement.vue" }).descriptor;
  const content = compileScript(descriptor, { id: "audit-management-component-test", inlineTemplate: true }).content;
  const js = ts.transpileModule(content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./audit-api": AuditApi, "./i18n": AdminI18n };
  const body = js.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
let ComponentUnderTest: Component;
const mounted: Array<{ root: HostNode; scope: ReturnType<typeof ref>; i18n: ReturnType<typeof createAdminI18n>; emitted: number[] }> = [];
function find(root: HostNode, predicate: (node: HostNode) => boolean): HostNode | null { if (predicate(root)) return root; for (const child of root.children) { const found = find(child, predicate); if (found) return found; } return null; }
function all(root: HostNode): HostNode[] { return [root, ...root.children.flatMap(all)]; }
function text(root: HostNode): string { return root.text + root.children.map(text).join(""); }
function input(root: HostNode, type: string, index = 0): HostNode { return all(root).filter(node => node.tag === "input" && node.props.type === type)[index]!; }
function setInput(node: HostNode, value: string) { (node.props["onUpdate:modelValue"] as (value: string) => void)(value); node.value = value; }
function click(root: HostNode, label: string) { const node = all(root).find(candidate => candidate.tag === "button" && text(candidate) === label); expect(node).toBeDefined(); (node!.props.onClick as (() => void) | undefined)?.(); }
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
function mount() {
  vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
  const scope = ref<{ account: AdminAccount; brandId: string }>({ account, brandId: brand });
  const root = element("root"), i18n = createAdminI18n(), emitted: number[] = [];
  const app = renderer.createApp({ setup: () => () => h(ComponentUnderTest, { ...scope.value, onSessionInvalid: () => emitted.push(1) }) });
  app.provide(adminI18nKey, i18n); app.mount(root); apps.push(app);
  const mountedScope = { root, scope, i18n, emitted }; mounted.push(mountedScope); return mountedScope;
}
function fillDateRange(root: HostNode) { const dates = all(root).filter(node => node.tag === "input" && node.props.type === "datetime-local"); setInput(dates[0]!, "2026-10-01T00:00:00"); setInput(dates[1]!, "2026-10-02T00:00:00"); }
function submit(root: HostNode) { const form = find(root, node => node.tag === "form")!; (form.props.onSubmit as (event: { preventDefault: () => void }) => void)({ preventDefault() {} }); }

beforeEach(() => {
  apiStub = { list: vi.fn<(...args: any[]) => Promise<{ items: RecordRow[] }>>(async () => ({ items: [row] })), exportCsv: vi.fn<(...args: any[]) => Promise<unknown>>(async () => { throw new Error("unexpected export"); }) };
  vi.spyOn(AuditApi, "createAuditApi").mockImplementation(() => apiStub as never);
  ComponentUnderTest = compileComponent();
});
afterEach(() => { apps.splice(0).forEach(app => app.unmount()); mounted.splice(0); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("audit management UI", () => {
  it("waits for explicit query, renders redacted details, and offers separate export", async () => {
    const view = mount(); await flush();
    expect(apiStub.list).not.toHaveBeenCalled();
    fillDateRange(view.root); submit(view.root); await flush();
    expect(apiStub.list).toHaveBeenCalledTimes(1);
    expect(apiStub.list.mock.calls[0]![0]).toBe(brand);
    expect(apiStub.list.mock.calls[0]![1]).toMatchObject({ from: "2026-10-01T00:00:00.000Z", to: "2026-10-02T00:00:00.000Z", limit: 100, offset: 0 });
    expect(text(view.root)).toContain("user.update");
    const details = all(view.root).find(node => node.tag === "details");
    expect(text(details!)).toContain("[REDACTED]");
    expect(text(details!)).not.toContain("secret");
    expect(all(view.root).some(node => node.tag === "button" && text(node).includes("导出完整 CSV"))).toBe(true);
  });

  it("drops a pending read after language and brand scope changes", async () => {
    let resolve!: (value: { items: RecordRow[] }) => void;
    apiStub.list = vi.fn<(...args: any[]) => Promise<{ items: RecordRow[] }>>(() => new Promise(resolveResult => { resolve = resolveResult; }));
    const view = mount(); await flush(); fillDateRange(view.root); submit(view.root); await flush();
    expect(apiStub.list).toHaveBeenCalledTimes(1);
    view.i18n.setLocale("en"); await flush();
    expect(text(view.root)).toContain("Audit log");
    view.scope.value = { account, brandId: otherBrand }; await flush();
    resolve({ items: [{ ...row, brand_id: otherBrand }] }); await flush();
    expect(text(view.root)).not.toContain("user.update");
    expect(text(view.root)).toContain("Query audit records");
  });

  it("shows English and Chinese controls and does not retry a failed read", async () => {
    apiStub.list = vi.fn<(...args: any[]) => Promise<{ items: RecordRow[] }>>(async () => { throw new Error("network unavailable"); });
    const view = mount(); await flush();
    expect(text(view.root)).toContain("审计日志");
    view.i18n.setLocale("en"); await flush();
    expect(text(view.root)).toContain("Audit log");
    view.i18n.setLocale("zh-CN"); await flush();
    fillDateRange(view.root); submit(view.root); await flush();
    expect(apiStub.list).toHaveBeenCalledTimes(1);
    expect(text(view.root)).toContain("network unavailable");
    await flush(); expect(apiStub.list).toHaveBeenCalledTimes(1);
    expect(view.emitted).toHaveLength(0);
  });

  it("does not grant a super-admin rights for the wrong brand", async () => {
    const view = mount();
    view.scope.value = { account: { ...account, super_admin: true, brand_ids: [otherBrand], permissions_by_brand: { [otherBrand]: ["audit.view.brand", "audit.export.brand"] } }, brandId: brand };
    await flush();
    expect(text(view.root)).toContain("当前账号没有该品牌的审计查看权限");
    expect(apiStub.list).not.toHaveBeenCalled();
  });

  it("treats datetime-local values as UTC and rejects calendar rollover dates", async () => {
    const oldTimezone = process.env.TZ;
    process.env.TZ = "America/Los_Angeles";
    try {
      const view = mount(); await flush();
      view.i18n.setLocale("en"); await flush();
      const dates = all(view.root).filter(node => node.tag === "input" && node.props.type === "datetime-local");
      setInput(dates[0]!, "2025-02-29T00:00:00"); setInput(dates[1]!, "2025-03-01T00:00:00"); submit(view.root); await flush();
      expect(apiStub.list).not.toHaveBeenCalled();
      expect(text(view.root)).toContain("Choose a valid start and end within 31 days.");
      setInput(dates[0]!, "2024-02-29T00:00:00"); setInput(dates[1]!, "2024-03-01T00:00:00"); submit(view.root); await flush();
      expect(apiStub.list).toHaveBeenCalledTimes(1);
      expect(apiStub.list.mock.calls[0]![1]).toMatchObject({ from: "2024-02-29T00:00:00.000Z", to: "2024-03-01T00:00:00.000Z" });
    } finally {
      if (oldTimezone === undefined) delete process.env.TZ;
      else process.env.TZ = oldTimezone;
    }
  });

  it("keeps compact layouts and readable controls on mobile widths", () => {
    const source = readFileSync(new URL("./AuditManagement.vue", import.meta.url), "utf8");
    expect(source).toContain("@media(max-width:760px)");
    expect(source).toContain("@media(max-width:420px)");
    expect(source).toContain("aria-labelledby=\"audit-title\"");
    expect(source).toContain("aria-live=\"polite\"");
  });

  it("keeps a page heading and the demo-only explanation for signed-out navigation", () => {
    const app = readFileSync(new URL("./App.vue", import.meta.url), "utf8");
    expect(app).toContain("<div v-if=\"!account\" class=\"panel directory-state\">");
    expect(app).toContain("<h1>{{ ui(\"审计日志\") }}</h1>");
    expect(app).toContain("{{ ui(\"未登录：以下是静态演示样例，不是后台记录。\") }}");
  });
});
