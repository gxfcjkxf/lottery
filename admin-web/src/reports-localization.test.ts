import { afterEach, describe, expect, it, vi } from "vitest";
import { createRenderer, h, nextTick, type Component } from "vue";
import * as VueRuntime from "vue";
import { readFileSync } from "node:fs";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import * as ReportsApi from "./reports-api";
import * as ReportExportApi from "./report-export-api";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";

function component(): Component {
  const filename = "ReportsManagement.vue", source = readFileSync(new URL(`./${filename}`, import.meta.url), "utf8");
  const descriptor = parse(source, { filename }).descriptor;
  const js = ts.transpileModule(compileScript(descriptor, { id: "reports-locale-test", inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const empty = { render: () => null };
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./reports-api": ReportsApi, "./report-export-api": ReportExportApi, "./WithdrawalReport.vue": empty, "./CommissionReport.vue": empty, "./RewardReports.vue": empty, "./AttributionReportManagement.vue": empty };
  const body = js.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`)
    .replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, name: string, specifier: string) => `const ${name}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}

type Node = { tag: string; text: string; props: Record<string, unknown>; children: Node[]; parent?: Node; value?: string; style: Record<string, string>; addEventListener: () => void; removeEventListener: () => void; getRootNode: () => Node; options: Node[] };
function element(tag: string): Node {
  return { tag, text: "", props: {}, children: [], value: "", style: {}, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter(n => n.tag === "option"); } };
}
const renderer = createRenderer<Node, Node>({
  createElement: element, createText: text => ({ ...element("#text"), text }), createComment: () => element("#comment"),
  setText: (n, text) => { n.text = text; }, setElementText: (n, text) => { n.text = text; n.children = []; },
  patchProp: (n, key, _old, value) => { n.props[key] = value; if (key === "value") n.value = String(value ?? ""); },
  insert(n, parent, anchor) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1); n.parent = parent; const index = anchor ? parent.children.indexOf(anchor) : -1; if (index < 0) parent.children.push(n); else parent.children.splice(index, 0, n); },
  remove(n) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1); }, parentNode: n => n.parent ?? null, nextSibling: n => n.parent?.children[n.parent.children.indexOf(n) + 1] ?? null,
});
function nodes(root: Node): Node[] { return [root, ...root.children.flatMap(nodes)]; }
function text(root: Node): string { return root.text + root.children.map(text).join(""); }
function button(root: Node, label: string) { const n = nodes(root).find(n => n.tag === "button" && text(n) === label); expect(n).toBeDefined(); return n!; }
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
const brand = "11111111-1111-4111-8111-111111111111";
const account: AdminAccount = { id: "22222222-2222-4222-8222-222222222222", super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["report_betting.view.brand", "report_ledger.view.brand"] } };
const mounted: { unmount: () => void }[] = [];
function mount() {
  const root = element("root"), i18n = createAdminI18n();
  const target = component();
  const app = renderer.createApp({ setup: () => () => h(target, { account, brandId: brand }) });
  app.provide(adminI18nKey, i18n); app.mount(root); mounted.push(app); return { root, i18n };
}
afterEach(() => { mounted.splice(0).forEach(app => app.unmount()); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
describe("report localization", () => {
  it("switches actual controls without altering filter values or issuing requests", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response("", { status: 503 }));
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const { root, i18n } = mount(); await flush();
    expect(text(root)).toContain("运营报表");
    const inputs = nodes(root).filter(n => n.tag === "input"), values = inputs.map(n => n.value);
    const selects = nodes(root).filter(n => n.tag === "select"), options = selects.map(n => n.options.map(o => o.props.value));
    i18n.setLocale("en"); await flush();
    expect(text(root)).toContain("Operational reports"); expect(text(root)).not.toMatch(/\p{Script=Han}/u);
    expect(inputs.map(n => n.value)).toEqual(values); expect(selects.map(n => n.options.map(o => o.props.value))).toEqual(options);
    expect(fetcher).not.toHaveBeenCalled();
    const member = inputs.find(n => n.props.placeholder === "Optional")!;
    (member.props["onUpdate:modelValue"] as (v: string) => void)("not-a-uuid");
    (button(root, "Query reports").props.onClick as () => void)(); await flush();
    expect(text(root)).toContain("The member filter UUID is invalid."); expect(fetcher).not.toHaveBeenCalled();
    i18n.setLocale("zh-CN"); await flush(); expect(text(root)).toContain("会员筛选 UUID 格式无效。");
    expect(member.value).toBe("not-a-uuid");
  });
});
