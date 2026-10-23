import { describe, expect, it, vi } from 'vitest';
import { JOIN_LINK_TTL, joinLinkFields, joinLinkKey, resolveJoinLink } from './join-attribution';
const agent = 'A'.repeat(24), referral = 'B'.repeat(24);
function storage() { const data = new Map<string, string>(); return { data, getItem: (key: string) => data.get(key) ?? null, setItem: (key: string, value: string) => { data.set(key, value); }, removeItem: (key: string) => { data.delete(key); } }; }
describe('URL invitation attribution', () => {
  it('uses different fields and prioritizes the agent, including when its input is invalid', async () => {
    const store = storage(), validate = vi.fn().mockResolvedValue(true);
    const result = await resolveJoinLink(store, 'brand', new URLSearchParams({ agent_code: agent.toLowerCase(), referral_code: referral }), validate, 0);
    expect(joinLinkFields(result.link)).toEqual({ agent_code: agent });
    expect(validate).toHaveBeenCalledOnce();
    validate.mockClear();
    expect(await resolveJoinLink(store, 'brand', new URLSearchParams({ agent_code: 'bad', referral_code: referral }), validate, 1)).toEqual({ link: null, unavailable: true });
    expect(validate).not.toHaveBeenCalled();
    expect(store.getItem(joinLinkKey('brand'))).toBeNull();
  });
  it('persists for exactly seven days across routes and reloads without sliding expiry', async () => {
    const store = storage(), validate = vi.fn().mockResolvedValue(true), query = new URLSearchParams({ referral_code: referral });
    const first = await resolveJoinLink(store, 'brand', query, validate, 100);
    expect(joinLinkFields(first.link)).toEqual({ referral_code: referral });
    expect(JSON.parse(store.getItem(joinLinkKey('brand'))!).expiresAt).toBe(100 + JOIN_LINK_TTL);
    expect((await resolveJoinLink(store, 'brand', new URLSearchParams(), validate, 200)).link).toEqual(first.link);
    await resolveJoinLink(store, 'brand', query, validate, 300);
    expect(JSON.parse(store.getItem(joinLinkKey('brand'))!).expiresAt).toBe(100 + JOIN_LINK_TTL);
    expect(await resolveJoinLink(store, 'brand', new URLSearchParams(), validate, 100 + JOIN_LINK_TTL)).toEqual({ link: null, unavailable: false });
    expect(store.getItem(joinLinkKey('brand'))).toBeNull();
  });
  it('does not leak cached links across brands and clears revoked codes', async () => {
    const store = storage(), validate = vi.fn().mockResolvedValue(true);
    await resolveJoinLink(store, 'one', new URLSearchParams({ agent_code: agent }), validate, 0);
    expect((await resolveJoinLink(store, 'two', new URLSearchParams(), validate, 1)).link).toBeNull();
    validate.mockResolvedValue(false);
    expect(await resolveJoinLink(store, 'one', new URLSearchParams(), validate, 1)).toEqual({ link: null, unavailable: true });
    expect(store.getItem(joinLinkKey('one'))).toBeNull();
  });
  it('replaces a prior cache with a newly validated link and rejects duplicate parameters', async () => {
    const store = storage(), validate = vi.fn().mockResolvedValue(true);
    await resolveJoinLink(store, 'brand', new URLSearchParams({ agent_code: agent }), validate, 0);
    expect((await resolveJoinLink(store, 'brand', new URLSearchParams({ referral_code: referral }), validate, 1)).link?.kind).toBe('referral');
    expect((await resolveJoinLink(store, 'brand', new URLSearchParams(`agent_code=${agent}&agent_code=${agent}`), validate, 2)).unavailable).toBe(true);
  });
  it('fails explicitly on storage or validation errors rather than registering with a lost source', async () => {
    const store = storage();
    await expect(resolveJoinLink(store, 'brand', new URLSearchParams({ agent_code: agent }), async () => { throw new Error('network'); })).rejects.toThrow('network');
    expect(store.data.size).toBe(0);
    await expect(resolveJoinLink({ ...store, setItem: () => { throw new Error('storage'); } }, 'brand', new URLSearchParams({ agent_code: agent }), async () => true)).rejects.toThrow('storage');
  });
});
