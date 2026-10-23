export type JoinLink = { kind: 'agent' | 'referral'; code: string };
export const JOIN_LINK_TTL = 7 * 24 * 60 * 60 * 1000;
type Storage = Pick<globalThis.Storage, 'getItem' | 'setItem' | 'removeItem'>;
const validCode = (code: string) => /^[0-9A-F]{24}$/.test(code);
export const joinLinkKey = (brand: string) => `lottery.join-link.${brand}`;
export function joinLinkFields(link: JoinLink | null): { agent_code?: string; referral_code?: string } {
  return link ? link.kind === 'agent' ? { agent_code: link.code } : { referral_code: link.code } : {};
}

export async function resolveJoinLink(storage: Storage, brand: string, query: URLSearchParams, validate: (link: JoinLink) => Promise<boolean>, now = Date.now()): Promise<{ link: JoinLink | null; unavailable: boolean }> {
  const key = joinLinkKey(brand);
  const raw = storage.getItem(key);
  let cached: (JoinLink & { expiresAt: number }) | null = raw === null ? null : JSON.parse(raw);
  if (cached && (!['agent', 'referral'].includes(cached.kind) || typeof cached.code !== 'string' || !validCode(cached.code) || !Number.isFinite(cached.expiresAt))) throw new Error('Invalid cached join link');
  if (cached && cached.expiresAt <= now) { storage.removeItem(key); cached = null; }
  const parameter = query.has('agent_code') ? 'agent_code' : query.has('referral_code') ? 'referral_code' : null;
  let candidate: JoinLink | null = cached;
  if (parameter) {
    const values = query.getAll(parameter);
    const code = values.length === 1 ? values[0]!.trim().toUpperCase() : '';
    if (!validCode(code)) { storage.removeItem(key); return { link: null, unavailable: true }; }
    candidate = { kind: parameter === 'agent_code' ? 'agent' : 'referral', code };
  }
  if (!candidate) return { link: null, unavailable: false };
  if (!await validate(candidate)) { storage.removeItem(key); return { link: null, unavailable: true }; }
  if (!cached || candidate.kind !== cached.kind || candidate.code !== cached.code) storage.setItem(key, JSON.stringify({ ...candidate, expiresAt: now + JOIN_LINK_TTL }));
  return { link: { kind: candidate.kind, code: candidate.code }, unavailable: false };
}
