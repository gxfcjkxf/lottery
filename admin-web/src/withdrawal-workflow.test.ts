import { afterEach, describe, expect, it, vi } from 'vitest';
import { createRenderer, nextTick, ssrContextKey, type App } from 'vue';
import WithdrawalManagement from './WithdrawalManagement.vue';
import { adminI18nKey, createAdminI18n } from './i18n';
import type { WithdrawalOrder } from '@lottery/shared';

const brand = '11111111-1111-4111-8111-111111111111';
const id = '22222222-2222-4222-8222-222222222222';
const member = '33333333-3333-4333-8333-333333333333';
const at = '2026-10-20T00:00:00Z';
function order(state: 'reviewing' | 'processing' = 'reviewing'): WithdrawalOrder {
  return { id, brand_id: brand, member_id: member, account_id: '44444444-4444-4444-8444-444444444444', points: '1', state, version: state === 'reviewing' ? 1 : 2,
    source_allocation: [{source: 'winning', state: 'available', points: '1'}], reserve_entry_id: '55555555-5555-4555-8555-555555555555', release_entry_id: null, paid_entry_id: null,
    cycle_from_at: null, cycle_from_version: '0', reserve_version: '1', created_at: at, updated_at: at, reviewed_at: state === 'reviewing' ? null : at, completed_at: null, decision_reason: '', audit_log_id: '66666666-6666-4666-8666-666666666666' };
}
const ok = (data: unknown) => new Response(JSON.stringify({success: true, data}), {status: 200});
const account = { id: '77777777-7777-4777-8777-777777777777', super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: {[brand]: ['withdrawal.view.brand', 'withdrawal.approve.brand']} };
// Exercise the real setup and reactive workflow without a native-dialog DOM.
const renderer = createRenderer<object, object>({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  setText() {}, setElementText() {}, patchProp() {}, insert() {}, remove() {}, parentNode: () => null, nextSibling: () => null,
});
type Workflow = { page: WithdrawalOrder[]; receipt: WithdrawalOrder | null; pending: unknown; error: unknown; begin: (value: WithdrawalOrder, action: 'approve') => void; perform: (action: 'approve') => Promise<void> };
const apps: App[] = [];
function mount(active = true) {
  const app = renderer.createApp({...WithdrawalManagement, render: () => null}, {account, brandId: brand, active});
  app.provide(ssrContextKey, {modules: new Set()});
  app.provide(adminI18nKey, createAdminI18n()); app.mount({}); apps.push(app);
  return { app, workflow: (app._instance as unknown as {setupState: Workflow}).setupState };
}
async function flush() { for (let i = 0; i < 25; i++) await Promise.resolve(); await nextTick(); }
afterEach(() => { apps.splice(0).forEach(app => app.unmount()); vi.unstubAllGlobals(); });
function server(failReload = false) {
  let current = order(); let reads = 0;
  const fetcher = vi.fn<typeof fetch>(async (_url, init) => {
    if (init?.method === 'POST') { current = order('processing'); return ok(current); }
    reads++;
    if (failReload && reads > 1) return new Response(JSON.stringify({success: false, error: {code: 'UNAVAILABLE', message: 'List unavailable'}}), {status: 503});
    return ok({brand_id: brand, items: [current], limit: 100, offset: 0, has_more: false});
  });
  vi.stubGlobal('fetch', fetcher); return fetcher;
}
describe('withdrawal review workflow', () => {
  it('reloads the list after a confirmed blank-reason approval and shows the new state', async () => {
    const fetcher = server(); const {workflow} = mount(); await flush();
    expect(workflow.page[0]?.state).toBe('reviewing');
    workflow.begin(workflow.page[0]!, 'approve'); await workflow.perform('approve');
    expect(workflow.page[0]?.state).toBe('processing');
    expect(workflow.receipt?.state).toBe('processing'); expect(workflow.pending).toBeNull();
    const posts = fetcher.mock.calls.filter(([, init]) => init?.method === 'POST');
    expect(posts).toHaveLength(1); expect(JSON.parse(String(posts[0]![1]!.body))).toEqual({version: 1, reason: ''});
    expect(fetcher.mock.calls).toHaveLength(3);
  });
  it('preserves the confirmed receipt when the follow-up list read fails', async () => {
    const fetcher = server(true); const {workflow} = mount(); await flush();
    workflow.begin(workflow.page[0]!, 'approve'); await workflow.perform('approve');
    expect(workflow.receipt?.state).toBe('processing'); expect(workflow.pending).toBeNull(); expect(workflow.error).toBeTruthy();
    expect(fetcher.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(1);
  });
  it('does not preload the hidden tab', async () => {
    const fetcher = server(); mount(false); await flush(); expect(fetcher).not.toHaveBeenCalled();
  });
});
