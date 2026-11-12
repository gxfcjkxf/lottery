import { PlatformApiError, readPlatformEnvelope } from './platform-api';
export interface AuthSettings { version: number; inherit_platform: boolean; captcha_enabled: boolean; telegram_enabled: boolean; telegram_client_id: string; audit_log_id?: string }
export type SecurityKind = 'auth';
export interface SecurityIntent { kind: SecurityKind; body: string; key: string; phase: 'review' | 'unknown' | 'conflict' }
let epoch = 0;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const pending = new Map<string, SecurityIntent>();
export const securityIntents = {
  epoch: () => epoch,
  clearAll: () => { epoch++; pending.clear(); },
  get: (scope: string) => pending.get(scope) ?? null,
  set: (scope: string, intent: SecurityIntent) => pending.set(scope, Object.freeze({ ...intent })),
  clear: (scope: string) => pending.delete(scope),
};
function validAuth(value: unknown): value is AuthSettings {
  if (!value || typeof value !== 'object') return false;
  const v = value as Record<string, unknown>;
  const keys = ['version', 'inherit_platform', 'captcha_enabled', 'telegram_enabled', 'telegram_client_id'];
  if (Object.keys(v).some(key => ![...keys, 'audit_log_id'].includes(key))) return false;
  return typeof v.inherit_platform === 'boolean' && Number.isSafeInteger(v.version) && Number(v.version) > 0 && typeof v.captcha_enabled === 'boolean' && typeof v.telegram_enabled === 'boolean' && typeof v.telegram_client_id === 'string' && (!v.telegram_enabled && v.telegram_client_id === '' || /^[1-9][0-9]*$/.test(v.telegram_client_id) && Number.isSafeInteger(Number(v.telegram_client_id))) && (v.audit_log_id === undefined || typeof v.audit_log_id === 'string' && UUID.test(v.audit_log_id));
}
export function createSecuritySettingsApi(fetcher: typeof fetch = fetch) {
  async function request(_kind: SecurityKind, brand: string, intent?: SecurityIntent) {
    if (brand !== '' && !UUID.test(brand)) throw new PlatformApiError('Invalid brand', 400, 'REQUEST_INVALID');
    const headers = new Headers({ Accept: 'application/json' });
    if (brand) headers.set('X-Brand-ID',brand);
    if (intent) { headers.set('Content-Type', 'application/json'); headers.set('Idempotency-Key', intent.key); }
    let response: Response;
    try { response = await fetcher(`/api/v1/platform/auth-settings`, { method: intent ? 'PATCH' : 'GET', credentials: 'same-origin', headers, ...(intent ? { body: intent.body } : {}) }); }
    catch { throw new PlatformApiError('Network request failed', 0, 'NETWORK_ERROR'); }
    const { data } = await readPlatformEnvelope(response);
    if (response.status !== 200 || !validAuth(data)) throw new PlatformApiError('Invalid settings receipt', 0, 'INVALID_RESPONSE');
    if (intent) {
      const expected = JSON.parse(intent.body), receipt = data as AuthSettings;
      const matches = receipt.inherit_platform === expected.inherit_platform && (expected.inherit_platform || receipt.captcha_enabled === expected.captcha_enabled && receipt.telegram_enabled === expected.telegram_enabled && receipt.telegram_client_id === expected.telegram_client_id);
      if (receipt.version !== expected.version + 1 || !receipt.audit_log_id || !UUID.test(receipt.audit_log_id) || !matches) throw new PlatformApiError('Invalid write receipt', 0, 'INVALID_RESPONSE');
    }
    return data as AuthSettings;
  }
  return { read: (kind: SecurityKind, brand: string) => request(kind, brand), write: (brand: string, intent: SecurityIntent) => request(intent.kind, brand, intent) };
}
