import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { createRenderer, h, nextTick, type Component } from "vue";
import * as VueRuntime from "vue";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import * as TemplateApi from "./notification-templates-api";
import * as TemplateState from "./notification-templates-state";
import type { AdminAccount } from "./admin-api";
import { notificationTemplateKeys, type NotificationTemplate, type NotificationTemplateContent } from "./notification-templates-api";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";

const brand = "11111111-1111-4111-8111-111111111111";
const actor = "22222222-2222-4222-8222-222222222222";
const at = "2026-10-07T00:00:00Z";
const account: AdminAccount = {
  id: actor, super_admin: false, brand_ids: [brand], permissions: [],
  permissions_by_brand: { [brand]: ["notification_template.view.brand", "notification_template.write.brand"] },
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
  createElement: element, createText: (text) => ({ ...element("#text"), text }), createComment: (text) => ({ ...element("#comment"), text }),
  setText: (node, text) => { node.text = text; }, setElementText: (node, text) => { node.text = text; node.children = []; },
  patchProp: (node, key, _oldValue, value) => { node.props[key] = value; if (key === "value") node.value = String(value ?? ""); },
  insert: (node, parent, anchor) => { if (node.parent) { const index = node.parent.children.indexOf(node); if (index >= 0) node.parent.children.splice(index, 1); } node.parent = parent; const index = anchor ? parent.children.indexOf(anchor) : -1; if (index < 0) parent.children.push(node); else parent.children.splice(index, 0, node); },
  remove: (node) => { if (!node.parent) return; const index = node.parent.children.indexOf(node); if (index >= 0) node.parent.children.splice(index, 1); node.parent = undefined; },
  parentNode: (node) => node.parent ?? null,
  nextSibling: (node) => node.parent ? node.parent.children[node.parent.children.indexOf(node) + 1] ?? null : null,
});

function compile(): Component {
  const source = readFileSync(new URL("./NotificationTemplates.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "NotificationTemplates.vue" }).descriptor;
  const script = compileScript(descriptor, { id: "notification-templates-component-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(script, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = {
    vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n,
    "./notification-templates-api": TemplateApi, "./notification-templates-state": TemplateState,
  };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) =>
    `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
const ComponentUnderTest = compile();
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
function find(node: HostNode, predicate: (item: HostNode) => boolean): HostNode | null {
  if (predicate(node)) return node;
  for (const child of node.children) { const found = find(child, predicate); if (found) return found; }
  return null;
}
async function flush() { for (let i = 0; i < 20; i++) await Promise.resolve(); await nextTick(); }
function templateContent(key: string): NotificationTemplateContent {
  const title = `Editable ${key}`;
  if (key === "member.joined") {
    return { en: { title, body: "Custom English welcome copy." }, "zh-CN": { title: `可编辑 ${key}`, body: "自定义中文欢迎文案。" } };
  }
  return {
    en: { title, body: `Custom English copy {points} for {resource_id}.` },
    "zh-CN": { title: `可编辑 ${key}`, body: `自定义中文文案 {points}，编号 {resource_id}。` },
  };
}
function templates(): NotificationTemplate[] {
  return notificationTemplateKeys.map((key) => ({ brand_id: brand, key, version: 1, content: templateContent(key), updated_at: at, audit_log_id: null }));
}
function response(data: unknown) { return new Response(JSON.stringify({ success: true, data }), { status: 200 }); }
function mount() {
  const root = element("root"), app = renderer.createApp({ setup: () => () => h(ComponentUnderTest, { account, brandId: brand }) });
  app.provide(adminI18nKey, createAdminI18n()); app.mount(root);
  return { app, root };
}

afterEach(() => vi.unstubAllGlobals());

describe("NotificationTemplates commission history", () => {
  it("lists sixteen templates and keeps bilingual commission facts fixed beside editable copy", async () => {
    vi.stubGlobal("Document", class {});
    vi.stubGlobal("ShadowRoot", class {});
    const fetcher = vi.fn<typeof fetch>(async (input) => String(input).includes("/history?") ? response({ items: [] }) : response({ items: templates() }));
    vi.stubGlobal("fetch", fetcher);
    const mounted = mount();
    await flush();
    const select = find(mounted.root, (node) => node.props.id === "nt-key");
    expect(select?.options.filter((option) => option.value !== "").map((option) => option.value)).toHaveLength(16);

    for (const [key, en, zh] of [
      ["commission.paid", "not a new income forecast or an external payment", "不是新收入预测或外部付款承诺"],
      ["commission.adjusted", "signed commission adjustment", "带正负方向的佣金调整"],
    ] as const) {
      (select?.props.onChange as ((event: { target: { value: string } }) => void) | undefined)?.({ target: { value: key } });
      await flush();
      const note = find(mounted.root, (node) => node.props["data-testid"] === "commission-facts-note");
      expect(note).not.toBeNull();
      expect(textOf(note!)).toContain(en);
      expect(textOf(note!)).toContain(zh);
      expect(textOf(mounted.root)).toContain("Custom English copy 12345 for 00000000-0000-4000-8000-000000000099.");
      const editor = find(mounted.root, (node) => node.props.id === "nt-body-en");
      expect(editor).not.toBeNull();
      expect(textOf(note!)).not.toContain("Custom English copy");
    }
    mounted.app.unmount();
  });
});
