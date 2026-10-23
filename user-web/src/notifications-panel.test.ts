import SiteIcon from "@lottery/shared/SiteIcon";
import { createRenderer, h, nextTick } from 'vue';
import * as Vue from 'vue';
import { compileScript, parse } from 'vue/compiler-sfc';
import { readFileSync } from 'node:fs';
import ts from 'typescript';
import { afterEach, expect, it, vi } from 'vitest';
import * as I18n from './i18n';
import * as Shared from '@lottery/shared';

const list = vi.fn(), markRead = vi.fn();
const descriptor = parse(readFileSync(new URL('./NotificationsPanel.vue', import.meta.url), 'utf8')).descriptor;
const js = ts.transpileModule(compileScript(descriptor, { id: 'notifications-test', inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
const modules = {
  "@lottery/shared/SiteIcon": SiteIcon,
  '@lottery/shared': Shared,
  vue: Vue, './i18n': I18n,
  './notification-api': { createNotificationApi: () => ({ list, markRead }) },
  './notification-presentation': { renderNotification: (item: any, locale: string) => ({ ...item.content[locale], createdAt: item.created_at }) },
  '@lottery/shared/DetailModal.vue': { props: ['title', 'closeLabel'], setup: (props: any, { slots, emit }: any) => () => h('dialog', {}, [h('h2', props.title), slots.default?.(), slots.footer?.(), h('button', { onClick: () => emit('close') }, props.closeLabel)]) },
};
const body = js.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings, specifier) => `const {${bindings.replace(/\s+as\s+/g, ': ')}}=modules[${JSON.stringify(specifier)}];`)
  .replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, binding, specifier) => `const ${binding}=modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, 'return ');
const Panel = new Function('modules', body)(modules);
type Node = { tag: string; text: string; props: any; children: Node[]; parent: Node | null };
const node = (tag: string, text = ''): Node => ({ tag, text, props: {}, children: [], parent: null });
const refreshTarget = node('nav');
const renderer = createRenderer<Node, Node>({
  querySelector: () => refreshTarget,
  createElement: tag => node(tag), createText: text => node('#text', text), createComment: text => node('#comment', text),
  setText: (n, text) => { n.text = text }, setElementText: (n, text) => { n.text = text; n.children = [] },
  patchProp: (n, key, _old, value) => { n.props[key] = value },
  insert(n, parent, anchor) { n.parent = parent; const i = anchor ? parent.children.indexOf(anchor) : -1; if (i < 0) parent.children.push(n); else parent.children.splice(i, 0, n) },
  remove(n) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1) },
  parentNode: n => n.parent, nextSibling: n => n.parent?.children[n.parent.children.indexOf(n) + 1] ?? null,
});
const text = (n: Node): string => n.text + n.children.filter(c => c.tag !== '#comment').map(text).join('');
const find = (n: Node, match: (n: Node) => boolean): Node[] => [...(match(n) ? [n] : []), ...n.children.flatMap(c => find(c, match))];
const summaries = (root: Node) => find(root, n => n.props.class === 'notification-summary');
const apps: Array<{ unmount(): void }> = [];
let intersect: (entries: any[]) => void;
const message = (id: number) => ({ id: String(id), created_at: '2026-10-17T08:21:00Z', read_at: null, content: { en: { title: `Title ${id}`, body: 'Secret body' }, 'zh-CN': { title: `标题 ${id}`, body: '消息正文' } } });
const page = (offset = 0, count = 20, member = 'member') => ({ brand_id: 'brand', member_id: member, offset, unread_count: '50', items: Array.from({ length: count }, (_, i) => message(offset + i)) });
async function flush() { for (let i = 0; i < 25; i++) await Promise.resolve(); await nextTick() }
function mount(mobile = true) {
  vi.stubGlobal('window', { matchMedia: () => ({ matches: mobile, addEventListener: vi.fn(), removeEventListener: vi.fn() }) });
  vi.stubGlobal('navigator', { languages: ['en'], language: 'en' });
  vi.stubGlobal('IntersectionObserver', class { constructor(cb: typeof intersect) { intersect = cb } observe() {} disconnect() {} });
  const root = node('root'), i18n = I18n.createUserI18n();
  const app = renderer.createApp({ setup: () => () => h(Panel, { locale: i18n.locale.value }) }).provide(I18n.userI18nKey, i18n);
  app.mount(root); apps.push(app); return { root, i18n };
}
afterEach(() => { apps.splice(0).forEach(a => a.unmount()); vi.unstubAllGlobals(); list.mockReset(); markRead.mockReset() });

it('shows only summary fields, marks opened messages read and offers a close button', async () => {
  markRead.mockResolvedValue({ brand_id: 'brand', member_id: 'member', ids: ['0'], unread_count: '0' });
  list.mockResolvedValue(page(0, 1)); const { root, i18n } = mount(); await flush();
  expect(text(root)).toContain('Title 0'); expect(text(root)).not.toContain('Secret body');
  expect(text(root)).not.toContain('No more messages');
  expect(text(summaries(root)[0])).not.toContain(i18n.t('user.copy.25fe7670'));
  expect(find(summaries(root)[0], n => n.props.role === 'img')[0].props['aria-label']).toBe(i18n.t('user.copy.25fe7670'));
  summaries(root)[0].props.onClick(); await flush();
  expect(text(root)).toContain('Secret body'); expect(markRead).toHaveBeenCalledOnce();
  expect(find(root, n => n.tag === 'button' && text(n) === 'Mark as read')).toHaveLength(0);
  expect(find(root, n => n.tag === 'button' && text(n) === 'Close')).toHaveLength(1);
  i18n.setLocale('zh-CN'); await flush(); expect(text(root)).toContain('消息正文'); expect(text(root)).toContain('关闭消息');
  find(root, n => n.tag === 'button' && text(n) === '关闭消息')[0].props.onClick(); await flush(); expect(text(root)).not.toContain('消息正文');
});
it('shows the empty-state message only when the list has no messages', async () => {
  list.mockResolvedValue(page(0, 0)); const { root, i18n } = mount(); await flush();
  expect(text(root)).toContain('No new messages right now'); expect(text(root)).not.toContain('No more messages');
  i18n.setLocale('zh-CN'); await flush(); expect(text(root)).toContain('当前没有新消息');
});
it('appends mobile pages and preserves the cursor when loading fails', async () => {
  list.mockResolvedValueOnce(page()).mockRejectedValueOnce({ status: 503 }).mockResolvedValueOnce(page(20, 1));
  const { root } = mount(); await flush(); intersect([{ isIntersecting: true }]); await flush();
  expect(summaries(root)).toHaveLength(20); expect(text(root)).toContain('Retry loading');
  intersect([{ isIntersecting: true }]); await flush(); expect(list).toHaveBeenCalledTimes(2);
  find(root, n => n.tag === 'button' && text(n) === 'Retry loading')[0].props.onClick(); await flush();
  expect(list.mock.calls.map(c => c[1])).toEqual([0, 20, 20]); expect(summaries(root)).toHaveLength(21); expect(text(root)).toContain('No more messages');
});
it('uses confirmed automatic-read receipts without discarding accumulated messages', async () => {
  list.mockResolvedValueOnce(page()).mockResolvedValueOnce(page(20, 1));
  markRead.mockResolvedValue({ brand_id: 'brand', member_id: 'member', ids: ['0'], unread_count: '49' });
  const { root } = mount(); await flush(); intersect([{ isIntersecting: true }]); await flush();
  summaries(root)[0].props.onClick(); await flush();
  expect(markRead).toHaveBeenCalledTimes(1); expect(summaries(root)).toHaveLength(21); expect(find(summaries(root)[0], n => n.props.role === 'img')[0].props['aria-label']).toBe('Read'); expect(text(summaries(root)[0])).not.toContain('Read'); expect(list).toHaveBeenCalledTimes(2);
});
it('keeps desktop pagination and replaces the current page', async () => {
  list.mockResolvedValueOnce(page()).mockResolvedValueOnce(page(20, 1));
  const { root } = mount(false); await flush();
  find(root, n => n.tag === 'button' && text(n) === 'Next')[0].props.onClick(); await flush(); expect(summaries(root)).toHaveLength(1); expect(text(root)).toContain('Title 20');
});
it('keeps failed automatic reads unread and retries the exact request only on explicit action', async () => {
  list.mockResolvedValue(page(0, 1));
  markRead.mockRejectedValueOnce({ status: 503 }).mockResolvedValueOnce({ brand_id: 'brand', member_id: 'member', ids: ['0'], unread_count: '0' });
  const { root, i18n } = mount(); await flush(); summaries(root)[0].props.onClick(); await flush();
  expect(find(summaries(root)[0], n => n.props.role === 'img')[0].props['aria-label']).toBe(i18n.t('user.copy.25fe7670'));
  find(root, n => n.tag === 'button' && text(n) === 'Close')[0].props.onClick(); await flush();
  expect(markRead).toHaveBeenCalledTimes(1);
  find(root, n => n.tag === 'button' && text(n) === i18n.t('user.copy.4da8cfa4'))[0].props.onClick(); await flush();
  expect(markRead).toHaveBeenCalledTimes(2); expect(markRead.mock.calls[1]).toEqual(markRead.mock.calls[0]);
  summaries(root)[0].props.onClick(); await flush(); expect(markRead).toHaveBeenCalledTimes(2);
});
it('deduplicates overlapping pages and limits bulk reads to 100 IDs', async () => {
  list.mockImplementation((_limit: number, offset: number) => Promise.resolve(page(offset)));
  markRead.mockImplementation((ids: string[]) => Promise.resolve({ brand_id: 'brand', member_id: 'member', ids, unread_count: '20' }));
  const { root } = mount(); await flush();
  for (let i = 0; i < 5; i++) { intersect([{ isIntersecting: true }]); await flush() }
  expect(summaries(root)).toHaveLength(120);
  find(root, n => n.tag === 'button' && n.props['aria-label'] === 'Mark 100 loaded messages read')[0].props.onClick(); await flush();
  expect(markRead.mock.calls[0][0]).toHaveLength(100); expect(summaries(root)).toHaveLength(120);
  list.mockResolvedValueOnce({ ...page(120, 2), items: [message(119), message(120)] });
  intersect([{ isIntersecting: true }]); await flush(); expect(summaries(root)).toHaveLength(121);
});
it('clears old membership messages and modal when the account changes', async () => {
  markRead.mockResolvedValue({ brand_id: 'brand', member_id: 'member', ids: ['1'], unread_count: '49' });
  list.mockResolvedValueOnce(page()).mockResolvedValueOnce(page(20, 1, 'other')).mockResolvedValueOnce(page(0, 1, 'other'));
  const { root } = mount(); await flush(); summaries(root)[1].props.onClick(); await flush(); intersect([{ isIntersecting: true }]); await flush();
  expect(summaries(root)).toHaveLength(1); expect(find(root, n => n.tag === 'dialog')).toHaveLength(0); expect(list.mock.calls.map(c => c[1])).toEqual([0, 20, 0]);
});
