import {ManagementListTestComponent,MemberActionsMenuTestComponent} from "./management-list-test-helper"
import { SelectControlTestComponent } from './select-control-test-helper'
import { h as temporalTestH } from "vue";
import * as DisplayShared from "@lottery/shared";
DisplayShared.setPlatformDisplaySettings({ timezone: "Asia/Singapore", date_format: "YYYY-MM-DD", time_format: "HH:mm:ss" });
import * as FormValidation from '../../shared/src/form-validation';
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
  const modules: Record<string, unknown> = {"@lottery/shared/ManagementList.vue":ManagementListTestComponent,"@lottery/shared/MemberActionsMenu.vue":MemberActionsMenuTestComponent, "@lottery/shared/SelectControl.vue": SelectControlTestComponent, "./SelectControl.vue": SelectControlTestComponent, "@lottery/shared/TimeZoneSelect.vue": { props: ["modelValue", "locale", "disabled", "required"], setup: (p: any, { emit }: any) => () => h("select-control", { "data-timezone-select": true, value: p.modelValue, disabled: p.disabled, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) }, "@lottery/shared/PlatformTemporalInput.vue": { props: ["modelValue", "type", "required", "disabled"], setup: (p: any, { emit }: any) => () => temporalTestH("input", { type: p.type, value: p.modelValue, required: p.required, disabled: p.disabled, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) }, "@lottery/shared": { ...DisplayShared, ...FormValidation },
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
  return new Function("__modules", moduleBody.replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, binding: string, specifier: string) => `const ${binding}=__modules[${JSON.stringify(specifier)}];`))(modules) as Component;
}

function policy(brandId: string, version: number): CommissionPolicy {
  return { brand_id: brandId, version, config: { enabled: false, calendar: null, payout_mode: "manual" }, created_at: timestamp, updated_at: new Date(Date.parse(timestamp) + version * 60_000).toISOString(), revision_id: revisionId };
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

describe("CommissionPolicySettings inherited settings", () => {
 it("shows inherited values without configuration controls or updated time", async () => {
  vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockResolvedValue(ok(policy(brandA, 1))));
  const mounted = mount(baseAccount, brandA); await flush();
  expect(textOf(mounted.container)).not.toContain('继承平台佣金设置');
  for (const tag of ['form', 'input', 'select-control', 'button', 'time']) expect(findNode(mounted.container, n => n.tag === tag)).toBeNull();
  mounted.app.unmount();
 });
 it.each(['brand', 'account', 'permission'] as const)('drops stale reads on %s change', async change => {
  const pending = deferred<Response>(); let first = true;
  vi.stubGlobal('fetch', vi.fn<typeof fetch>((_input, init) => {
   if(first) { first = false; return pending.promise }
   const brand = new Headers(init?.headers).get('X-Brand-ID')!;
   return Promise.resolve(ok(policy(brand, 2)));
  }));
  const mounted = mount(baseAccount, brandA); await flush();
  if(change === 'brand') mounted.scope.value = {account: baseAccount, brandId: brandB};
  if(change === 'account') mounted.scope.value = {account: {...baseAccount, id: actorB}, brandId: brandA};
  if(change === 'permission') mounted.scope.value = {account: {...baseAccount, permissions_by_brand: {}}, brandId: brandA};
  await flush(); pending.resolve(ok({...policy(brandA, 1), config: {...policy(brandA,1).config, payout_mode: 'automatic'}})); await flush();
  expect(textOf(mounted.container)).not.toContain('自动'); mounted.app.unmount();
 });
});
