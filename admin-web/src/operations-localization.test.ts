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
import * as DeliveryApi from "./notification-delivery-api";
import * as DeliveryState from "./notification-delivery-state";
import type { AdminAccount } from "./admin-api";
import type { JoinCodePage } from "./join-codes-api";
import { adminI18nKey, createAdminI18n } from "./i18n";
import { clearAllPendingJoinCodeWrites, setPendingJoinCodeWrite, type PendingJoinCodeWrite } from "./join-codes-state";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import BrandDomains from "./BrandDomains.vue";
import AgentManagement from "./AgentManagement.vue";
import JoinCodeManagement from "./JoinCodeManagement.vue";
import CompliancePolicy from "./CompliancePolicy.vue";
import NotificationDeliveries from "./NotificationDeliveries.vue";
import NotificationTemplates from "./NotificationTemplates.vue";

const brand = "11111111-1111-4111-8111-111111111111";
const actor = "22222222-2222-4222-8222-222222222222";
const owner = "33333333-3333-4333-8333-333333333333";
const account: AdminAccount = {
  id: actor, super_admin: false, brand_ids: [brand], permissions: [],
  permissions_by_brand: { [brand]: ["join_code.view.brand", "join_code.write.brand"] },
};
const deliveryAccount: AdminAccount = {
  ...account,
  permissions_by_brand: { [brand]: ["notification.view.brand", "notification.retry.brand"] },
};

describe("operations locale rendering", () => {
  it("renders English chrome and switches back to Chinese without translating customer data", async () => {
    const pages: Array<[Component, Record<string, unknown>, string, string]> = [
      [BrandDomains, { account, brandId: brand }, "brand domain view access", "域名查看权限"],
      [AgentManagement, { account, brandId: brand }, "Agent list", "代理列表"],
      [JoinCodeManagement, { account, brandId: brand, memberId: owner, kind: "referral", ownerRecord: {} }, "Referral code", "推荐码"],
      [CompliancePolicy, { account, brandId: brand }, "Risk and compliance", "风控与合规"],
      [NotificationDeliveries, { account, brandId: brand }, "Notification deliveries", "通知投递记录"],
      [NotificationTemplates, { account, brandId: brand }, "Notification templates", "通知模板"],
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
    if (page === CompliancePolicy) {
      expect(englishHtml).toContain(
        "Configuration check",
      );
    }
    context.setLocale("zh-CN");
    const chineseHtml = await render();
    expect(chineseHtml).toContain(chinese);
    if (page === CompliancePolicy) {
      expect(chineseHtml).toContain("配置检查");
    }
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
  const modules: Record<string, unknown> = { "@lottery/shared/BusinessDetails.vue": BusinessDetails,
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

describe("JoinCodeManagement slow-read confirmation", () => {
  it("blocks replay during a list read and preserves the frozen request and approval across locale changes", async () => {
    const frozen: PendingJoinCodeWrite = {
      accountId: actor, brandId: brand, operation: "create", resourceId: null,
      identity: { kind: "referral", code: null, owner_member_id: owner, agent_id: null },
      body: { kind: "referral", owner_member_id: owner, agent_id: null, starts_at: null, expires_at: null, reason: "approved test reason" },
      key: "frozen-key-for-join-code-retry",
    };
    setPendingJoinCodeWrite({ accountId: actor, brandId: brand }, frozen);
    const firstRead = deferred<JoinCodePage>();
    list.mockReturnValueOnce(firstRead.promise).mockResolvedValueOnce(page);
    create.mockResolvedValueOnce(undefined);
    const ComponentUnderTest = JoinCodePageComponent;
    const root = element("root"), context = createAdminI18n();
    const app = renderer.createApp({ render: () => h(ComponentUnderTest, { account, brandId: brand, memberId: owner, kind: "referral", ownerRecord: {} }) });
    app.provide(adminI18nKey, context);
    app.mount(root);
    await flush();

    const confirmation = find(root, (node) => node.props["data-testid"] === "join-code-frozen-confirmation");
    const retry = find(root, (node) => node.props["data-testid"] === "join-code-retry");
    const confirm = find(root, (node) => typeof node.props.onClick === "function" && textOf(node).includes("提交"));
    expect(confirmation?.props.disabled).toBe(true);
    expect(retry?.props.disabled).toBe(true);
    expect(confirm?.props.disabled).toBe(true);
    (confirm?.props.onClick as (() => void) | undefined)?.();
    (retry?.props.onClick as (() => void) | undefined)?.();
    await flush();
    expect(create).not.toHaveBeenCalled();

    context.setLocale("en");
    await nextTick();
    expect(textOf(root)).toContain("Referral code");
    expect(textOf(root)).toContain("I reviewed the immutable ownership and request content above");
    expect(textOf(root)).toContain("Start time");
    expect(textOf(root)).not.toContain('"starts_at"');
    expect(textOf(root)).not.toContain(frozen.key);

    firstRead.resolve(page);
    await flush();
    const readyConfirmation = find(root, (node) => node.props["data-testid"] === "join-code-frozen-confirmation");
    expect(readyConfirmation?.props.disabled).toBe(false);
    (readyConfirmation?.props["onUpdate:modelValue"] as ((value: boolean) => void) | undefined)?.(true);
    await nextTick();
    context.setLocale("zh-CN");
    await nextTick();
    expect(textOf(root)).toContain("推荐码");
    expect(textOf(root)).toContain("我已核对上方不可变归属与请求内容");
    const retryAfterLocaleChange = find(root, (node) => node.props["data-testid"] === "join-code-retry");
    expect(retryAfterLocaleChange?.props.disabled).toBe(false);
    (retryAfterLocaleChange?.props.onClick as (() => void) | undefined)?.();
    await flush();

    expect(create).toHaveBeenCalledTimes(1);
    expect(create).toHaveBeenCalledWith(brand, frozen.body, frozen.key);
    expect(JSON.stringify(frozen.body)).toBe(JSON.stringify({ kind: "referral", owner_member_id: owner, agent_id: null, starts_at: null, expires_at: null, reason: "approved test reason" }));
    app.unmount();
  });
});

function compileDeliveryPage(): Component {
  const source = readFileSync(new URL("./NotificationDeliveries.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "NotificationDeliveries.vue" }).descriptor;
  const script = compileScript(descriptor, { id: "operations-delivery-localization-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(script, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { "@lottery/shared/BusinessDetails.vue": BusinessDetails,
    "@lottery/shared/DetailModal.vue": { setup(_props: unknown, { slots }: { slots: Record<string, () => unknown> }) { return () => h("dialog", slots.default?.() as never); } },
    "@lottery/shared": { ...DisplayShared, ...FormValidation },
    vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n,
    "./notification-delivery-api": DeliveryApi, "./notification-delivery-state": DeliveryState,
  };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) =>
    `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body.replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, binding: string, specifier: string) => `const ${binding}=__modules[${JSON.stringify(specifier)}];`))(modules) as Component;
}

describe("NotificationDeliveries locale-aware notices", () => {
  it("re-renders an unknown retry notice and safe errors after locale changes", async () => {
    const eventId = "44444444-4444-4444-8444-444444444444";
    const failedDelivery = {
      event_id: eventId, brand_id: brand, status: "failed", attempt_count: 2,
      last_error: "DELIVERY_TIMEOUT", next_attempt_at: "2026-10-07T00:00:00Z", sent_at: null,
    };
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") throw new Error("raw transport failure");
      return new Response(JSON.stringify({ success: true, data: { items: [failedDelivery] } }), { status: 200 });
    });
    vi.stubGlobal("Document", class {});
    vi.stubGlobal("ShadowRoot", class {});
    vi.stubGlobal("fetch", fetchMock);
    const ComponentUnderTest = compileDeliveryPage();
    const root = element("root"), context = createAdminI18n();
    const app = renderer.createApp({ render: () => h(ComponentUnderTest, { account: deliveryAccount, brandId: brand }) });
    app.provide(adminI18nKey, context);
    app.mount(root);
    await flush();

    const reason = find(root, (node) => node.props.id === "nd-retry-reason");
    (reason?.props["onUpdate:modelValue"] as ((value: string) => void) | undefined)?.("manual retry reason");
    await nextTick();
    const startReview = find(root, (node) => String(node.props["aria-label"] ?? "").includes(eventId));
    (startReview?.props.onClick as (() => void) | undefined)?.();
    await nextTick();
    const checkbox = find(root, (node) => node.tag === "input" && node.props.type === "checkbox");
    (checkbox?.props["onUpdate:modelValue"] as ((value: boolean) => void) | undefined)?.(true);
    await nextTick();
    const submit = find(root, (node) => typeof node.props.onClick === "function" && textOf(node).includes("确认重试"));
    (submit?.props.onClick as (() => void) | undefined)?.();
    await flush();

    expect(textOf(root)).toContain("重试结果未知");
    expect(textOf(root)).toContain("无法连接，请检查网络。");
    expect(textOf(root)).not.toContain("raw transport failure");
    context.setLocale("en");
    await nextTick();
    expect(textOf(root)).toContain("The result is not yet confirmed");
    expect(textOf(root)).toContain("Unable to connect.");
    expect(textOf(root)).not.toContain("raw transport failure");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    app.unmount();
  });
});
