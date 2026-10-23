import { createRenderer, h, nextTick, ref } from 'vue';
import * as VueRuntime from 'vue';
import { compileScript, parse } from 'vue/compiler-sfc';
import { readFileSync } from 'node:fs';
import ts from 'typescript';
import * as Shared from '@lottery/shared';
import * as WalletApi from './wallet-api';
import * as UserI18n from './i18n';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createUserI18n, userI18nKey } from './i18n';
import { WALLET_SNAPSHOT_EVENT, type Wallet } from './wallet-api';

const descriptor = parse(readFileSync(new URL('./HeaderPoints.vue', import.meta.url), 'utf8')).descriptor;
const javascript = ts.transpileModule(compileScript(descriptor, { id: 'header-points-test', inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
const modules = { vue: VueRuntime, '@lottery/shared': Shared, './wallet-api': WalletApi, './i18n': UserI18n };
const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm,
  (_match, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ': ')}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, 'return ');
const HeaderPoints = new Function('__modules', body)(modules);

type Node = { tag: string; text: string; props: Record<string, unknown>; children: Node[]; parent: Node | null };
const node = (tag: string, text = ''): Node => ({ tag, text, props: {}, children: [], parent: null });
const renderer = createRenderer<Node, Node>({
  createElement: tag => node(tag), createText: text => node('#text', text), createComment: text => node('#comment', text),
  setText: (n, text) => { n.text = text }, setElementText: (n, text) => { n.text = text; n.children = [] },
  patchProp: (n, key, _old, value) => { n.props[key] = value },
  insert(n, parent, anchor) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1); n.parent = parent; const i = anchor ? parent.children.indexOf(anchor) : -1; if (i < 0) parent.children.push(n); else parent.children.splice(i, 0, n) },
  remove(n) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1) },
  parentNode: n => n.parent, nextSibling: n => n.parent?.children[n.parent.children.indexOf(n) + 1] ?? null,
});
const text = (n: Node): string => n.text + n.children.map(text).join('');
const apps: Array<{ unmount(): void }> = [];
afterEach(() => { for (const app of apps.splice(0)) app.unmount(); vi.unstubAllGlobals(); vi.useRealTimers() });
const brand = '22222222-2222-4222-8222-222222222222', member = '33333333-3333-4333-8333-333333333333';
function wallet(available = '9007199254740993', version = 1): Wallet {
  return {
    account_id: '11111111-1111-4111-8111-111111111111', brand_id: brand, member_id: member, version,
    display_points: available, available_points: available, recharge_points: available,
    frozen_points: '0', withdrawal_points: '0', winning_points: '0', gift_points: '0', commission_points: '0', manual_frozen_points: '0', system_frozen_points: '0',
    by_source: Object.fromEntries(['recharge', 'winning', 'gift', 'commission'].map(source => [source, { available: source === 'recharge' ? available : '0', manual_frozen: '0', system_frozen: '0', withdrawal: '0' }])) as Wallet['by_source'],
  };
}
const ok = (data: Wallet) => new Response(JSON.stringify({ success: true, data }));
async function flush() { for (let i = 0; i < 40; i++) await Promise.resolve(); await nextTick() }
function mount(fetcher: typeof fetch) {
  vi.useFakeTimers(); vi.stubGlobal('window', new EventTarget());
  vi.stubGlobal('navigator', { languages: ['en'], language: 'en' }); vi.stubGlobal('fetch', fetcher);
  const scope = ref({ brandId: brand, memberId: member, routeKey: '/account' });
  const root = node('root'), expired = vi.fn(), pointsChange = vi.fn(), i18n = createUserI18n();
  const app = renderer.createApp({ setup: () => () => h(HeaderPoints, { ...scope.value, onAuthExpired: expired, onPointsChange: pointsChange }) }).provide(userI18nKey, i18n);
  app.mount(root); apps.push(app); return { root, scope, expired, pointsChange, i18n, app };
}
describe('header available points', () => {
  it('shares the exact current membership balance with the profile without an extra wallet request', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(wallet('100')));
    const m = mount(fetcher); await flush();
    expect(m.pointsChange).toHaveBeenLastCalledWith({ brandId: brand, memberId: member, amount: '100', description: 'Points: 100' });
    window.dispatchEvent(new CustomEvent(WALLET_SNAPSHOT_EVENT, { detail: wallet('80', 2) })); await flush();
    expect(m.pointsChange).toHaveBeenLastCalledWith(expect.objectContaining({ amount: '80' })); expect(fetcher).toHaveBeenCalledOnce();
    m.i18n.setLocale('zh-CN'); await flush();
    expect(m.pointsChange).toHaveBeenLastCalledWith(expect.objectContaining({ description: '积分：80' }));
    m.app.unmount(); expect(m.pointsChange).toHaveBeenLastCalledWith(null);
  });
  it('displays exact available points and follows the selected locale', async () => {
    const m = mount(vi.fn<typeof fetch>().mockResolvedValue(ok(wallet()))); await flush();
    expect(text(m.root)).toBe('9,007,199,254,740,993');
    expect(m.root.children[0].props['aria-label']).toBe('Points: 9,007,199,254,740,993');
    m.i18n.setLocale('zh-CN'); await nextTick();
    expect(text(m.root)).toBe('9,007,199,254,740,993');
    expect(m.root.children[0].props['aria-label']).toBe('积分：9,007,199,254,740,993');
  });
  it('shows real zero, but never invents zero when loading fails', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(wallet('0'))).mockRejectedValue(new Error('offline'));
    const m = mount(fetcher); await flush(); expect(text(m.root)).toContain('0');
    m.scope.value.routeKey = '/help'; await flush(); expect(text(m.root)).toBe('—');
    expect(m.root.children[0].props['aria-label']).toBe('Wallet is temporarily unavailable.');
  });
  it('updates after wallet reads, ignores lower versions and other memberships', async () => {
    const m = mount(vi.fn<typeof fetch>().mockResolvedValue(ok(wallet('100')))); await flush();
    window.dispatchEvent(new CustomEvent(WALLET_SNAPSHOT_EVENT, { detail: wallet('80', 2) })); await nextTick();
    expect(text(m.root)).toContain('80');
    window.dispatchEvent(new CustomEvent(WALLET_SNAPSHOT_EVENT, { detail: wallet('100', 1) }));
    window.dispatchEvent(new CustomEvent(WALLET_SNAPSHOT_EVENT, { detail: { ...wallet('999', 3), brand_id: 'other-brand' } })); await nextTick();
    expect(text(m.root)).toContain('80'); expect(text(m.root)).not.toContain('999');
  });
  it('clears balances during membership changes and ignores stale responses', async () => {
    let resolve!: (value: Response) => void;
    const fetcher = vi.fn<typeof fetch>().mockImplementationOnce(() => new Promise<Response>(r => { resolve = r })).mockResolvedValue(ok({ ...wallet('50'), member_id: '44444444-4444-4444-8444-444444444444' }));
    const m = mount(fetcher); m.scope.value.memberId = '44444444-4444-4444-8444-444444444444'; await flush();
    resolve(ok(wallet('999'))); await flush(); expect(text(m.root)).toContain('50'); expect(text(m.root)).not.toContain('999');
  });
  it('rejects wrong-owner wallet replies and clears expired sessions', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok({ ...wallet(), member_id: '44444444-4444-4444-8444-444444444444' }))
      .mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: 'AUTH_SESSION_REPLACED', message: 'Session replaced' } }), { status: 401 }));
    const m = mount(fetcher); await flush(); expect(text(m.root)).toBe('—');
    m.scope.value.routeKey = '/help'; await flush(); expect(m.expired).toHaveBeenCalledOnce();
  });
});
