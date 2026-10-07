import { createRenderer, h, nextTick, reactive, ssrContextKey } from "vue";
import { describe, expect, it, vi } from "vitest";
import RechargePanel from "./RechargePanel.vue";

type Node = { type: string; text: string; props: Record<string, unknown>; children: Node[]; parent: Node | null };
const host = {
  createElement: (type: string): Node => ({ type, text: "", props: {}, children: [], parent: null }),
  createText: (text: string): Node => ({ type: "#text", text, props: {}, children: [], parent: null }),
  createComment: (text: string): Node => ({ type: "#comment", text, props: {}, children: [], parent: null }),
  setText: (node: Node, text: string) => { node.text = text; },
  setElementText: (node: Node, text: string) => { node.text = text; node.children = []; },
  patchProp: (node: Node, key: string, _previous: unknown, value: unknown) => { node.props[key] = value; },
  insert: (node: Node, parent: Node, anchor: Node | null = null) => {
    if (node.parent) { const old = node.parent.children.indexOf(node); if (old >= 0) node.parent.children.splice(old, 1); }
    node.parent = parent;
    const index = anchor ? parent.children.indexOf(anchor) : -1;
    parent.children.splice(index < 0 ? parent.children.length : index, 0, node);
  },
  remove: (node: Node) => { if (node.parent) { const index = node.parent.children.indexOf(node); if (index >= 0) node.parent.children.splice(index, 1); node.parent = null; } },
  parentNode: (node: Node) => node.parent,
  nextSibling: (node: Node) => { if (!node.parent) return null; return node.parent.children[node.parent.children.indexOf(node) + 1] ?? null; },
};
const renderer = createRenderer<Node, Node>(host);
const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const member = "22222222-2222-4222-8222-222222222222";
const otherMember = "33333333-3333-4333-8333-333333333333";
const at = "2026-10-07T12:00:00Z";
const id = "44444444-4444-4444-8444-444444444444";
const finalPageId = "00000000-0000-4000-8000-000000000000";
const row = (overrides: Record<string, unknown> = {}) => ({ id, brand_id: brand, member_id: member, points: "9007199254740993", state: "pending", version: "1", created_at: at, confirmed_at: null, ledger_entry_id: null, ...overrides });
const page = (items: unknown[] = [], overrides: Record<string, unknown> = {}) => ({ brand_id: brand, member_id: member, snapshot_at: at, state: null, items, limit: 20, offset: 0, total_count: String(items.length), ...overrides });
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { status: 200 });
function text(node: Node): string { return node.text + node.children.map(text).join(""); }
async function flush() { for (let index = 0; index < 12; index++) await Promise.resolve(); await nextTick(); }
function mount(fetcher: typeof fetch, props: Record<string, unknown> = {}) {
  const root: Node = { type: "root", text: "", props: {}, children: [], parent: null };
  const state = reactive({ locale: "en" as "en" | "zh", brandCode: undefined as string | undefined, member: { id: member, brand_id: brand } as { id: string; brand_id: string } | null, ...props });
  const emitted: string[] = [];
  let instance: any;
  vi.stubGlobal("fetch", fetcher);
  // Vite's Node transform provides ssrRender, not a browser render function.
  // Mount the actual setup/lifecycle with an empty host render; below we call
  // its real SSR template explicitly. Real DOM behavior is covered in Chrome.
  const mountedComponent = Object.assign({}, RechargePanel, { render: () => null });
  const app = renderer.createApp({ render: () => h(mountedComponent, { ...state, onVnodeMounted: (vnode) => { instance = vnode.component; }, onVnodeUpdated: (vnode) => { instance = vnode.component; }, onAuthExpired: () => emitted.push("auth-expired"), onSignIn: () => emitted.push("sign-in") }) });
  app.provide(ssrContextKey, { modules: new Set<string>() });
  app.mount(root);
  const render = () => {
    let output = "";
    instance.type.ssrRender(instance.proxy, (chunk: string) => { output += chunk; }, null, {}, instance.props, instance.setupState, instance.data, instance.appContext.config.globalProperties);
    root.text = output;
  };
  return { root, state, emitted, get setup() { return instance.setupState; }, emit: (name: string) => instance.emit(name), render, unmount: () => app.unmount() };
}

describe("RechargePanel Vue rendering and async scope", () => {
  it("renders real empty results, bilingual copy and a sign-in action without a router", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(page()));
    const app = mount(fetcher);
    await flush();
    app.render();
    expect(text(app.root)).toContain("Recharge records");
    expect(text(app.root)).toContain("No recharge records yet.");
    expect(text(app.root)).toContain("No real payment is initiated here.");
    expect(text(app.root)).toContain("current status only");
    app.state.member = null;
    await flush();
    app.render();
    expect(text(app.root)).toContain("Sign in to view your recharge records.");
    app.emit("sign-in");
    expect(app.emitted).toContain("sign-in");
  });

  it("switches state filters, pages by 20 and fetches a validated detail on demand", async () => {
    const rows = Array.from({ length: 20 }, (_, index) => row({ id: `00000000-0000-4000-8000-${String(20 - index).padStart(12, "0")}`, state: "confirmed", confirmed_at: at, ledger_entry_id: "55555555-5555-4555-8555-555555555555" }));
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes("offset=20")) return ok(page([row({ id: finalPageId, state: "confirmed", confirmed_at: at, ledger_entry_id: "55555555-5555-4555-8555-555555555555" })], { state: "confirmed", offset: 20, total_count: "21" }));
      if (url.includes("state=confirmed")) return ok(page(rows, { state: "confirmed", total_count: "21" }));
      if (url.endsWith(`/recharges/${finalPageId}`)) return ok(row({ id: finalPageId, state: "confirmed", confirmed_at: at, ledger_entry_id: "55555555-5555-4555-8555-555555555555" }));
      return ok(page(rows, { total_count: "21" }));
    });
    const app = mount(fetcher);
    await flush();
    app.render();
    expect(text(app.root)).toContain("9,007,199,254,740,993 pts");
    app.setup.selectedState = "confirmed";
    await flush();
    app.render();
    expect(String(fetcher.mock.calls.at(-1)?.[0])).toContain("state=confirmed");
    expect(text(app.root)).toContain("Confirmed");
    app.setup.nextPage();
    await flush();
    app.render();
    expect(String(fetcher.mock.calls.at(-1)?.[0])).toContain("offset=20");
    app.setup.openDetail(app.setup.page.items[0]);
    await flush();
    app.render();
    expect(String(fetcher.mock.calls.at(-1)?.[0])).toContain(`/recharges/${finalPageId}`);
    expect(text(app.root)).toContain("Confirmed");
    expect(text(app.root)).toContain("9,007,199,254,740,993 pts");
    expect(text(app.root)).toContain("Version");
    expect(text(app.root)).toContain(finalPageId);
    expect(text(app.root)).toMatch(/Version<\/dt><dd[^>]*>1<\/dd>/);
    expect(text(app.root)).toContain("Ledger entry");
    expect(text(app.root)).toContain("55555555-5555-4555-8555-555555555555");
    expect(text(app.root)).toContain("UTC");
    expect(text(app.root)).toContain("Queried at");
  });

  it("ignores late results after brand/account changes and clears a cookie identity mismatch", async () => {
    let resolveFirst!: (response: Response) => void;
    const delayed = new Promise<Response>((resolve) => { resolveFirst = resolve; });
    let calls = 0;
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => {
      calls++;
      if (calls === 1) return delayed;
      if (calls === 2) return ok(page([row({ id: otherMember, brand_id: otherBrand, member_id: otherMember, points: "22" })], { brand_id: otherBrand, member_id: otherMember }));
      return ok(page([row({ member_id: otherMember, points: "999" })]));
    });
    const app = mount(fetcher);
    await flush();
    app.render();
    app.state.brandCode = "other";
    app.state.member = { id: otherMember, brand_id: otherBrand };
    await flush();
    resolveFirst(ok(page([row({ points: "11" })])));
    await flush();
    app.render();
    expect(text(app.root)).toContain("22 pts");
    expect(text(app.root)).not.toContain("11 pts");
    expect(text(app.root)).not.toContain("999 pts");
    void app.setup.load();
    await flush();
    app.render();
    expect(app.emitted).toContain("auth-expired");
    expect(text(app.root)).toContain("Sign in again");
    app.state.locale = "zh";
    await nextTick();
    app.render();
    expect(text(app.root)).toContain("请重新登录以查看正确账户");
  });

  it("emits auth-expired on 401 and clears the record view", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { message: "Sign in required" } }), { status: 401 }));
    const app = mount(fetcher);
    await flush();
    app.render();
    expect(app.emitted).toContain("auth-expired");
    expect(text(app.root)).not.toContain("No recharge records yet.");
  });

  it("discards a slow list response after unmount", async () => {
    let resolve!: (response: Response) => void;
    const fetcher = vi.fn<typeof fetch>().mockReturnValue(new Promise<Response>((done) => { resolve = done; }));
    const app = mount(fetcher);
    const setup = app.setup;
    app.unmount();
    resolve(ok(page([row()])));
    await flush();
    expect(setup.page).toBeNull();
    expect(app.emitted).toEqual([]);
  });

  it("clears a detail and its spinner when a list refresh returns 401", async () => {
    let resolveDetail!: (response: Response) => void;
    const delayedDetail = new Promise<Response>((resolve) => { resolveDetail = resolve; });
    let call = 0;
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async (input) => {
      call++;
      if (String(input).includes(`/recharges/${id}`)) return delayedDetail;
      if (call === 3) return new Response(JSON.stringify({ success: false, error: { message: "Sign in required" } }), { status: 401 });
      return ok(page([row()]));
    });
    const app = mount(fetcher);
    await flush();
    void app.setup.openDetail(app.setup.page.items[0]);
    await flush();
    expect(app.setup.detailLoading).toBe(true);
    void app.setup.load();
    await flush();
    expect(app.setup.detailLoading).toBe(false);
    expect(app.setup.detail).toBeNull();
    expect(app.emitted).toContain("auth-expired");
    resolveDetail(ok(row({ state: "confirmed", confirmed_at: at, ledger_entry_id: "55555555-5555-4555-8555-555555555555" })));
    await flush();
    expect(app.setup.detailLoading).toBe(false);
    expect(app.setup.detail).toBeNull();
    expect(call).toBeGreaterThanOrEqual(3);
  });

  it("localizes status labels while keeping large points exact", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(page([row({ state: "confirmed", confirmed_at: at, ledger_entry_id: "55555555-5555-4555-8555-555555555555" })])));
    const app = mount(fetcher);
    await flush();
    app.render();
    app.state.locale = "zh";
    await nextTick();
    app.render();
    expect(text(app.root)).toContain("已确认");
    expect(text(app.root)).toContain("9,007,199,254,740,993 积分");
  });

  it("localizes built-in validation errors but leaves a server message unchanged", async () => {
    let call = 0;
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => {
      call++;
      if (call === 1) return ok({ ...page(), internal_reason: "closed DTO violation" });
      return new Response(JSON.stringify({ success: false, error: { message: "Exact upstream maintenance detail" } }), { status: 503 });
    });
    const app = mount(fetcher);
    await flush();
    app.state.locale = "zh";
    await nextTick();
    app.render();
    expect(text(app.root)).toContain("无法读取充值记录。");
    void app.setup.load();
    await flush();
    app.render();
    expect(text(app.root)).toContain("Exact upstream maintenance detail");
    app.state.locale = "en";
    await nextTick();
    app.render();
    expect(text(app.root)).toContain("Exact upstream maintenance detail");
  });
});
