import { PlatformApiError, readPlatformEnvelope } from './platform-api';
export interface CompliancePolicy {
  inherit_platform: boolean
  brand_id: string
  version: number
  config: {
    age_enabled: boolean
    minimum_age: number | null
    region_enabled: boolean
    allowed_countries: string[]
    identity_enabled: boolean
    account_risk_enabled: boolean
    betting_risk_enabled: boolean
    exclusion_enabled: boolean
    responsible_gambling_enabled: boolean
  }
  updated_at: string
  audit_log_id?: string
}
type Obj = Record<string, unknown>;
function object(value: unknown): value is Obj { return Boolean(value && typeof value === 'object' && !Array.isArray(value)) }
function exactKeys(value: Obj, keys: string[]): boolean {
  const actual = Object.keys(value).sort(), expected = [...keys].sort()
  return actual.length === expected.length && actual.every((key, i) => key === expected[i])
}
function version(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) > 0 }
function dateTime(value: unknown): value is string {
  return typeof value === 'string' && /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(value) && Number.isFinite(Date.parse(value))
}
function auditShape(value: Obj): boolean {
  if (!('audit_log_id' in value)) return true
  return typeof value.audit_log_id === 'string' && UUID.test(value.audit_log_id)
}
function policyEnvelope(value: unknown, brand: string, keys: string[]): value is Obj {
  return object(value) && (exactKeys(value, keys) || exactKeys(value, [...keys, 'audit_log_id'])) && value.brand_id === brand && version(value.version) && dateTime(value.updated_at) && auditShape(value)
}
function validCompliance(value: unknown, brand: string): value is CompliancePolicy {
  if (!policyEnvelope(value, brand, ['inherit_platform', 'brand_id', 'version', 'config', 'updated_at']) || typeof value.inherit_platform !== 'boolean' || !object(value.config) || !exactKeys(value.config, ['age_enabled', 'minimum_age', 'region_enabled', 'allowed_countries', 'identity_enabled', 'account_risk_enabled', 'betting_risk_enabled', 'exclusion_enabled', 'responsible_gambling_enabled'])) return false
  const c = value.config
  const countries = c.allowed_countries
  if (typeof c.age_enabled !== 'boolean' || typeof c.region_enabled !== 'boolean' || typeof c.identity_enabled !== 'boolean' ||
    typeof c.account_risk_enabled !== 'boolean' || typeof c.betting_risk_enabled !== 'boolean' || typeof c.exclusion_enabled !== 'boolean' || typeof c.responsible_gambling_enabled !== 'boolean' ||
    !(c.minimum_age === null || Number.isInteger(c.minimum_age) && Number(c.minimum_age) >= 18 && Number(c.minimum_age) <= 120) || c.age_enabled && c.minimum_age === null ||
    !Array.isArray(countries) || countries.length > 250 || c.region_enabled && countries.length === 0) return false
  return countries.every((country, index) => typeof country === 'string' && /^[A-Z]{2}$/.test(country) && (index === 0 || country > countries[index - 1]))
}
export interface AuthSettings { version: number; inherit_platform: boolean; captcha_enabled: boolean; telegram_enabled: boolean; telegram_client_id: string; privacy_policy_version: string; service_terms_version: string; audit_log_id?: string }
export type SecurityKind = 'auth' | 'compliance';
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
  const keys = ['version', 'inherit_platform', 'captcha_enabled', 'telegram_enabled', 'telegram_client_id', 'privacy_policy_version', 'service_terms_version'];
  if (Object.keys(v).some(key => ![...keys, 'audit_log_id'].includes(key))) return false;
  return typeof v.inherit_platform === 'boolean' && Number.isSafeInteger(v.version) && Number(v.version) > 0 && typeof v.captcha_enabled === 'boolean' && typeof v.telegram_enabled === 'boolean' && typeof v.telegram_client_id === 'string' && (!v.telegram_enabled && v.telegram_client_id === '' || /^[1-9][0-9]*$/.test(v.telegram_client_id) && Number.isSafeInteger(Number(v.telegram_client_id))) && (v.audit_log_id === undefined || typeof v.audit_log_id === 'string' && UUID.test(v.audit_log_id)) && typeof v.privacy_policy_version === 'string' && typeof v.service_terms_version === 'string';
}
export function createSecuritySettingsApi(fetcher: typeof fetch = fetch) {
  async function request(kind: SecurityKind, brand: string, intent?: SecurityIntent) {
    if (brand !== '' && !UUID.test(brand)) throw new PlatformApiError('Invalid brand', 400, 'REQUEST_INVALID');
    const headers = new Headers({ Accept: 'application/json' });
    if (brand) headers.set('X-Brand-ID',brand);
    if (intent) { headers.set('Content-Type', 'application/json'); headers.set('Idempotency-Key', intent.key); }
    let response: Response;
    try { response = await fetcher(`/api/v1/platform/${kind === 'auth' ? 'auth-settings' : 'compliance-policy'}`, { method: intent ? kind === 'auth' ? 'PATCH' : 'PUT' : 'GET', credentials: 'same-origin', headers, ...(intent ? { body: intent.body } : {}) }); }
    catch { throw new PlatformApiError('Network request failed', 0, 'NETWORK_ERROR'); }
    const { data } = await readPlatformEnvelope(response);
    if (response.status !== 200 || !(kind === 'auth' ? validAuth(data) : validCompliance(data, brand))) throw new PlatformApiError('Invalid settings receipt', 0, 'INVALID_RESPONSE');
    if (intent) {
      const expected = JSON.parse(intent.body), receipt = data as AuthSettings & CompliancePolicy;
      const matches = kind === 'auth' ? receipt.inherit_platform === expected.inherit_platform && (expected.inherit_platform || receipt.captcha_enabled === expected.captcha_enabled && receipt.telegram_enabled === expected.telegram_enabled && receipt.telegram_client_id === expected.telegram_client_id)
        : receipt.inherit_platform === expected.inherit_platform && (expected.inherit_platform || Object.keys(expected.config).every(key => JSON.stringify(receipt.config[key as keyof CompliancePolicy['config']]) === JSON.stringify(expected.config[key])));
      if (receipt.version !== expected.version + 1 || !receipt.audit_log_id || !UUID.test(receipt.audit_log_id) || !matches) throw new PlatformApiError('Invalid write receipt', 0, 'INVALID_RESPONSE');
    }
    return data as AuthSettings | CompliancePolicy;
  }
  return { read: (kind: SecurityKind, brand: string) => request(kind, brand), write: (brand: string, intent: SecurityIntent) => request(intent.kind, brand, intent) };
}
