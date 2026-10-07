import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";
import * as CommissionPolicyApi from "./commission-policy-api";
import * as CommissionPolicyState from "./commission-policy-state";
import type { CommissionPolicy } from "./commission-policy-api";

const brandA = "11111111-1111-4111-8111-111111111111";
const brandB = "22222222-2222-4222-8222-222222222222";
const actorA = "33333333-3333-4333-8333-333333333333";
const actorB = "44444444-4444-4444-8444-444444444444";
const revisionId = "55555555-5555-4555-8555-555555555555";
const auditId = "66666666-6666-4666-8666-666666666666";
const timestamp = "2026-10-06T00:00:00Z";
const CommissionPolicySettings = compileClientComponent();
const baseAccount: AdminAccount = {
  id: actorA,
  super_admin: false,
  brand_ids: [brandA, brandB],
  permissions: [],
  permissions_by_brand: {
    [brandA]: ["commission_policy.view.brand", "commission_policy.write.brand"],
    [brandB]: ["commission_policy.view.brand", "commission_policy.write.brand"],
  },
};

type HostNode = { tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode; value?: string; checked?: boolean; selected?: boolean; addEventListener: (...args: unknown[]) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[] };
function element(tag: string): HostNode {
  return {
    tag, props: {}, children: [], text: "", value: "", checked: false, selected: false,
    addEventListener() {}, removeEventListener() {}, getRootNode() { return this; },
    get options() { return this.children.filter((child) => child.tag === "option"); },
  };
}
const renderer = createRenderer<HostNode, HostNode>({
  createElement: element,
  createText: (text) => ({ ...element("#text"), text }),
  createComment: (text) => ({ ...element("#comment"), text }),
  setText: (node, text) => { node.text = text; },
  setElementText: (node, text) => { node.text = text; node.children = []; },
  patchProp: (node, key, _oldValue, value) => { node.props[key] = value; if (key === "value") node.value = String(value ?? ""); },
  insert: (node, parent, anchor) => {
    if (node.parent) {
      const oldIndex = node.parent.children.indexOf(node);
      if (oldIndex >= 0) node.parent.children.splice(oldIndex, 1);
    }
    node.parent = parent;
    const index = anchor ? parent.children.indexOf(anchor) : -1;
    if (index < 0) parent.children.push(node);
    else parent.children.splice(index, 0, node);
  },
  remove: (node) => {
    if (!node.parent) return;
    const index = node.parent.children.indexOf(node);
    if (index >= 0) node.parent.children.splice(index, 1);
    node.parent = undefined;
  },
  parentNode: (node) => node.parent ?? null,
  nextSibling: (node) => {
    if (!node.parent) return null;
    return node.parent.children[node.parent.children.indexOf(node) + 1] ?? null;
  },
});

function compileClientComponent() {
  const source = readFileSync(new URL("./CommissionPolicySettings.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "CommissionPolicySettings.vue" }).descriptor;
  const compiled = compileScript(descriptor, { id: "commission-policy-component-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(compiled, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = {
    vue: VueRuntime,
    "./admin-api": AdminApi,
    "./i18n": AdminI18n,
    "./commission-policy-api": CommissionPolicyApi,
    "./commission-policy-state": CommissionPolicyState,
  };
  const moduleBody = javascript
    .replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) => {
      const pattern = specifier;
      if (!(pattern in modules)) throw new Error(`Unmapped test component import: ${pattern}`);
      return `const {${bindings.replace(/\s+as\s+/g, ": ")}} = __modules[${JSON.stringify(pattern)}];`;
    })
    .replace(/export\s+default\s+/, "return ");
  return new Function("__modules", moduleBody)(modules) as Component;
}

function policy(brandId: string, version: number): CommissionPolicy {
  return { brand_id: brandId, version, config: { enabled: false, calendar: null, payout_mode: "manual" }, created_at: timestamp, updated_at: timestamp, revision_id: revisionId, ...(version > 1 ? { audit_log_id: auditId } : {}) };
}

function ok(data: unknown) { return new Response(JSON.stringify({ success: true, data }), { status: 200 }); }
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
function findNode(node: HostNode, match: (candidate: HostNode) => boolean): HostNode | null {
  if (match(node)) return node;
  for (const child of node.children) { const found = findNode(child, match); if (found) return found; }
  return null;
}
async function flush() { for (let i = 0; i < 8; i++) await Promise.resolve(); await nextTick(); }

function mount(account: AdminAccount, brandId: string) {
  const scope = ref({ account, brandId });
  const container = element("root");
  const app = renderer.createApp({ setup: () => () => h(CommissionPolicySettings, scope.value) });
  app.provide(adminI18nKey, createAdminI18n());
  app.mount(container);
  return { app, container, scope };
}

afterEach(() => vi.unstubAllGlobals());

describe("CommissionPolicySettings stale-scope handling", () => {
  it.each(["brand", "account", "permission"] as const)("drops stale asynchronous reads after %s scope changes", async (change) => {
    const pendingRead = deferred<Response>();
    let firstPolicyRead = true;
    vi.stubGlobal("fetch", vi.fn<typeof fetch>((input, init) => {
      const url = String(input);
      if (url.includes("/history?")) return Promise.resolve(ok({ items: [], limit: 20, offset: 0 }));
      const requestedBrand = new Headers(init?.headers).get("X-Brand-ID") ?? brandA;
      if (firstPolicyRead) { firstPolicyRead = false; return pendingRead.promise; }
      return Promise.resolve(ok(policy(requestedBrand, requestedBrand === brandB ? 7 : 4)));
    }));
    const mounted = mount(baseAccount, brandA);
    await flush();
    if (change === "brand") mounted.scope.value = { account: baseAccount, brandId: brandB };
    if (change === "account") mounted.scope.value = { account: { ...baseAccount, id: actorB }, brandId: brandA };
    if (change === "permission") mounted.scope.value = { account: { ...baseAccount, permissions_by_brand: { [brandA]: [], [brandB]: [] } }, brandId: brandA };
    await flush();
    pendingRead.resolve(ok(policy(brandA, 99)));
    await flush();
    const rendered = textOf(mounted.container);
    if (change === "brand") expect(rendered).toContain("版本 7");
    else if (change === "account") expect(rendered).toContain("版本 4");
    else expect(rendered).not.toContain("99");
    mounted.app.unmount();
  });

  it.each(["brand", "account", "permission"] as const)("does not apply a stale write response after %s scope changes", async (change) => {
    vi.stubGlobal("Document", class {});
    vi.stubGlobal("ShadowRoot", class {});
    const write = deferred<Response>();
    const writeStarted = deferred<void>();
    let policyReads = 0;
    vi.stubGlobal("fetch", vi.fn<typeof fetch>((input, init) => {
      const url = String(input);
      if (init?.method === "PUT") { writeStarted.resolve(); return write.promise; }
      if (url.includes("/history?")) return Promise.resolve(ok({ items: [], limit: 20, offset: 0 }));
      const requestedBrand = new Headers(init?.headers).get("X-Brand-ID") ?? brandA;
      policyReads++;
      return Promise.resolve(ok(policy(requestedBrand, policyReads === 1 ? 1 : requestedBrand === brandB ? 7 : 4)));
    }));
    const mounted = mount(baseAccount, brandA);
    await flush();
    const textarea = findNode(mounted.container, (node) => node.tag === "textarea");
    expect(textarea).not.toBeNull();
    (textarea!.props["onUpdate:modelValue"] as (value: string) => void)("Reviewed commission policy");
    await nextTick();
    const form = findNode(mounted.container, (node) => node.tag === "form");
    (form!.props.onSubmit as (event: { preventDefault(): void }) => void)({ preventDefault() {} });
    await nextTick();
    const confirm = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("确认提交"));
    expect(confirm).not.toBeNull();
    (confirm!.props.onClick as () => void)();
    await writeStarted.promise;

    if (change === "brand") mounted.scope.value = { account: baseAccount, brandId: brandB };
    if (change === "account") mounted.scope.value = { account: { ...baseAccount, id: actorB }, brandId: brandA };
    if (change === "permission") mounted.scope.value = { account: { ...baseAccount, permissions_by_brand: { [brandA]: [], [brandB]: [] } }, brandId: brandA };
    await flush();
    write.resolve(ok({ ...policy(brandA, 2), audit_log_id: auditId }));
    await flush();
    const rendered = textOf(mounted.container);
    if (change === "brand") expect(rendered).toContain("版本 7");
    else if (change === "account") expect(rendered).toContain("版本 4");
    else expect(rendered).not.toContain("2");
    mounted.app.unmount();
  });

  it("normalizes an HH:mm browser time and confirms every frozen calendar field", async () => {
    vi.stubGlobal("Document", class {});
    vi.stubGlobal("ShadowRoot", class {});
    const submitted: { value?: Record<string, any> } = {};
    vi.stubGlobal("fetch", vi.fn<typeof fetch>((input, init) => {
      const url = String(input);
      if (init?.method === "PUT") {
        const body = JSON.parse(String(init.body)) as Record<string, any>;
        submitted.value = body;
        return Promise.resolve(ok({ ...policy(brandA, 2), config: body.config, audit_log_id: auditId }));
      }
      if (url.includes("/history?")) return Promise.resolve(ok({ items: [], limit: 20, offset: 0 }));
      return Promise.resolve(ok(policy(brandA, 1)));
    }));
    const mounted = mount(baseAccount, brandA);
    await flush();
    const configure = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("明确配置日历"));
    (configure!.props.onClick as () => void)();
    await nextTick();

    const timezone = findNode(mounted.container, (node) => node.tag === "input" && String(node.props.placeholder).includes("Asia/Singapore"));
    (timezone!.props["onUpdate:modelValue"] as (value: string) => void)("Asia/Singapore");
    const cycle = findNode(mounted.container, (node) => node.tag === "select" && node.children.some((child) => child.props.value === "weekly") && node.children.some((child) => child.props.value === "monthly"));
    (cycle!.props["onUpdate:modelValue"] as (value: string) => void)("weekly");
    await nextTick();
    const boundary = findNode(mounted.container, (node) => node.tag === "input" && node.props.type === "time");
    (boundary!.props["onUpdate:modelValue"] as (value: string) => void)("18:30");
    const weekday = findNode(mounted.container, (node) => node.tag === "select" && textOf(node).includes("星期日") && textOf(node).includes("星期六"));
    (weekday!.props["onUpdate:modelValue"] as (value: string) => void)("1");
    const enabled = findNode(mounted.container, (node) => node.tag === "input" && node.props.type === "checkbox");
    (enabled!.props["onUpdate:modelValue"] as (value: boolean) => void)(true);
    const textarea = findNode(mounted.container, (node) => node.tag === "textarea");
    (textarea!.props["onUpdate:modelValue"] as (value: string) => void)("Reviewed weekly commission settings");
    await nextTick();

    const form = findNode(mounted.container, (node) => node.tag === "form");
    (form!.props.onSubmit as (event: { preventDefault(): void }) => void)({ preventDefault() {} });
    await nextTick();
    const confirmation = findNode(mounted.container, (node) => node.tag === "section" && textOf(node).includes("确认佣金策略变更"));
    expect(textOf(confirmation!)).toContain("已启用");
    expect(textOf(confirmation!)).toContain("人工");
    expect(textOf(confirmation!)).toContain("Asia/Singapore");
    expect(textOf(confirmation!)).toContain("每周");
    expect(textOf(confirmation!)).toContain("18:30:00");
    expect(textOf(confirmation!)).toContain("星期一");
    expect(textOf(confirmation!)).toContain("Reviewed weekly commission settings");
    const confirm = findNode(confirmation!, (node) => node.tag === "button" && textOf(node).includes("确认提交"));
    (confirm!.props.onClick as () => void)();
    await flush();
    expect(submitted.value?.config.calendar.boundary_time).toBe("18:30:00");
    expect(submitted.value?.config.calendar.weekday).toBe(1);
    mounted.app.unmount();
  });
});
