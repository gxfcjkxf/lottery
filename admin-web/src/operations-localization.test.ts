import {ManagementListTestComponent,MemberActionsMenuTestComponent} from "./management-list-test-helper"
import { SelectControlTestComponent } from './select-control-test-helper'
import { operationToastText } from "./operation-toast-test-helper";
import { BusinessDetailsTestComponent as BusinessDetails } from "./business-details-test-helper";
import * as DisplayShared from "@lottery/shared";
DisplayShared.setPlatformDisplaySettings({ timezone: "Asia/Singapore", date_format: "YYYY-MM-DD", time_format: "HH:mm:ss" });
import * as FormValidation from '../../shared/src/form-validation';
import { readFileSync } from "node:fs";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createRenderer, createSSRApp, h, nextTick, type Component } from "vue";
import { renderToString } from "vue/server-renderer";
import * as VueRuntime from "vue";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import * as JoinCodeApi from "./join-codes-api";
import * as JoinCodeState from "./join-codes-state";
import type { AdminAccount } from "./admin-api";
import type { JoinCodePage } from "./join-codes-api";
import { adminI18nKey, createAdminI18n } from "./i18n";
import { clearAllPendingJoinCodeWrites } from "./join-codes-state";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import BrandDomains from "./BrandDomains.vue";
import AgentManagement from "./AgentManagement.vue";
import JoinCodeManagement from "./JoinCodeManagement.vue";

const brand = "11111111-1111-4111-8111-111111111111";
const actor = "22222222-2222-4222-8222-222222222222";
const owner = "33333333-3333-4333-8333-333333333333";
const account: AdminAccount = {
  id: actor, super_admin: false, brand_ids: [brand], permissions: [],
  permissions_by_brand: { [brand]: ["join_code.view.brand", "join_code.write.brand"] },
};

describe("operations locale rendering", () => {
  it("renders English chrome and switches back to Chinese without translating customer data", async () => {
    const pages: Array<[Component, Record<string, unknown>, string, string]> = [
      [BrandDomains, { account, brandId: brand }, "brand domain view access", "域名查看权限"],
      [AgentManagement, { account, brandId: brand }, "Agent list", "代理列表"],
      [JoinCodeManagement, { account, brandId: brand, memberId: owner, kind: "referral", ownerRecord: {} }, "Referral code", "推荐码"],
    ];
    for (const [page, props, english, chinese] of pages) {
      const context = createAdminI18n();
      context.setLocale("en");
      const render = () => {
        const app = createSSRApp({ render: () => h(page, props) });
        app.provide(adminI18nKey, context);
        return renderToString(app);
      };
    const englishHtml = await render();
    expect(englishHtml).toContain(english);
    context.setLocale("zh-CN");
    const chineseHtml = await render();
    expect(chineseHtml).toContain(chinese);
    }
  });
});

type HostNode = {
  focus: () => void; scrollIntoView: () => void;
  tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode;
  value?: string; checked?: boolean; addEventListener: (...args: unknown[]) => void;
  removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[];
};
function element(tag: string): HostNode { return { focus: vi.fn(), scrollIntoView: vi.fn(), tag, props: {}, children: [], text: "", value: "", checked: false, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } }; }
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

const list = vi.fn<(...args: unknown[]) => Promise<JoinCodePage>>();
const create = vi.fn<(...args: unknown[]) => Promise<unknown>>();
const api = { list, create, update: vi.fn(), get: vi.fn(), history: vi.fn() };
function compileJoinCodePage(): Component {
  const source = readFileSync(new URL("./JoinCodeManagement.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "JoinCodeManagement.vue" }).descriptor;
  const script = compileScript(descriptor, { id: "operations-localization-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(script, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const apiModule = { ...JoinCodeApi, createJoinCodesApi: () => api };
  const modules: Record<string, unknown> = {"@lottery/shared/ManagementList.vue":ManagementListTestComponent,"@lottery/shared/MemberActionsMenu.vue":MemberActionsMenuTestComponent, "@lottery/shared/SelectControl.vue": SelectControlTestComponent, "./SelectControl.vue": SelectControlTestComponent, "@lottery/shared/BusinessDetails.vue": BusinessDetails,
    "@lottery/shared/DetailModal.vue": { setup(_props: unknown, { slots }: { slots: Record<string, () => unknown> }) { return () => h("dialog", slots.default?.() as never); } },
    "@lottery/shared": { ...DisplayShared, ...FormValidation },
    vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n,
    "./join-codes-api": apiModule, "./join-codes-state": JoinCodeState,
    "./member-lookup": { selectAdminAssociation: async (_brand: string, username: string) => ({ id: owner, member_id: owner, brand_id: brand, username }) },
  };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) =>
    `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body.replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, binding: string, specifier: string) => `const ${binding}=__modules[${JSON.stringify(specifier)}];`))(modules) as Component;
}
const JoinCodePageComponent = compileJoinCodePage();
function find(node: HostNode, predicate: (item: HostNode) => boolean): HostNode | null {
  if (predicate(node)) return node;
  for (const child of node.children) { const found = find(child, predicate); if (found) return found; }
  return null;
}
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
async function flush() { for (let i = 0; i < 20; i++) await Promise.resolve(); await nextTick(); }
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
const page: JoinCodePage = { brand_id: brand, kind: "referral", owner_member_id: owner, items: [], limit: 20, offset: 0, total_count: "0" };

afterEach(() => { clearAllPendingJoinCodeWrites(); vi.clearAllMocks(); vi.unstubAllGlobals(); });

describe("JoinCodeManagement bound code display", () => {
  it("ignores a previous owner's slow response and renders a single code without list actions", async () => {
    const firstRead = deferred<JoinCodePage>();
    const nextOwner = "44444444-4444-4444-8444-444444444444";
    const record = { id: actor, brand_id: brand, owner_member_id: nextOwner, agent_id: null, kind: "referral" as const, code: "0123456789ABCDEF01234567", status: "active" as const, usable: true, version: 1, starts_at: null, expires_at: null, created_at: "2026-10-24T00:00:00Z", updated_at: "2026-10-24T00:00:00Z" };
    list.mockReturnValueOnce(firstRead.promise).mockResolvedValueOnce({ ...page, owner_member_id: nextOwner, items: [record], total_count: "1" });
    const member = VueRuntime.ref(owner);
    const root = element("root"), context = createAdminI18n();
    const app = renderer.createApp({ render: () => h(JoinCodePageComponent, { account, brandId: brand, memberId: member.value, kind: "referral" }) });
    app.provide(adminI18nKey, context); app.mount(root);
    member.value = nextOwner; await flush();
    expect(textOf(root) + operationToastText()).toContain(record.code);
    expect(find(root, node => node.tag === "button")).toBeNull();
    expect(textOf(root) + operationToastText()).not.toContain("共");
    context.setLocale("en"); await nextTick();
    expect(textOf(root) + operationToastText()).toContain("Referral code");
    firstRead.resolve(page); await flush();
    expect(textOf(root) + operationToastText()).toContain(record.code);
    expect(api.get).not.toHaveBeenCalled(); expect(api.history).not.toHaveBeenCalled(); expect(create).not.toHaveBeenCalled();
    app.unmount();
  });
});
