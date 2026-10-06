import { notificationTemplateKeys, validNotificationTemplateContent, type NotificationTemplateKey, type NotificationTemplatePutBody } from "./notification-templates-api";

export interface NotificationTemplateWriteScope { accountId: string; brandId: string; templateKey: NotificationTemplateKey }
export interface PendingNotificationTemplateWrite extends NotificationTemplateWriteScope {
  readonly body: Readonly<NotificationTemplatePutBody>;
  readonly key: string;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const pending = new Map<string, PendingNotificationTemplateWrite>();
let sessionGeneration = 0;

export function notificationTemplateScopeKey(accountId: string, brandId: string, templateKey: NotificationTemplateKey): string {
  return JSON.stringify([accountId, brandId, templateKey]);
}
function scopeKey(scope: NotificationTemplateWriteScope): string {
  return notificationTemplateScopeKey(scope.accountId, scope.brandId, scope.templateKey);
}
function copyContent(content: NotificationTemplatePutBody["content"]): NotificationTemplatePutBody["content"] {
  const copy = { en: { ...content.en }, "zh-CN": { ...content["zh-CN"] } };
  Object.freeze(copy.en); Object.freeze(copy["zh-CN"]); return Object.freeze(copy);
}
function validIntent(value: unknown, scope: NotificationTemplateWriteScope): value is PendingNotificationTemplateWrite {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const candidate = value as Record<string, unknown>;
  const body = candidate.body;
  if (!body || typeof body !== "object" || Array.isArray(body)) return false;
  const request = body as Record<string, unknown>;
  return candidate.accountId === scope.accountId && candidate.brandId === scope.brandId && candidate.templateKey === scope.templateKey &&
    UUID_RE.test(scope.accountId) && UUID_RE.test(scope.brandId) &&
    notificationTemplateKeys.includes(scope.templateKey) &&
    Number.isSafeInteger(request.version) && Number(request.version) > 0 && Number(request.version) < Number.MAX_SAFE_INTEGER &&
    validNotificationTemplateContent(request.content, scope.templateKey) && typeof request.reason === "string" &&
    request.reason.trim() === request.reason && request.reason.trim().length > 0 && new TextEncoder().encode(request.reason).length <= 500 &&
    typeof candidate.key === "string" && /^[A-Za-z0-9_:.-]{8,128}$/.test(candidate.key);
}
function copyIntent(value: PendingNotificationTemplateWrite): PendingNotificationTemplateWrite {
  return Object.freeze({ accountId: value.accountId, brandId: value.brandId, templateKey: value.templateKey, key: value.key,
    body: Object.freeze({ version: value.body.version, content: copyContent(value.body.content), reason: value.body.reason }) });
}
export function freezeNotificationTemplateBody(body: NotificationTemplatePutBody): Readonly<NotificationTemplatePutBody> {
  return Object.freeze({ version: body.version, content: copyContent(body.content), reason: body.reason });
}
export function getPendingNotificationTemplateWrite(scope: NotificationTemplateWriteScope): PendingNotificationTemplateWrite | null {
  if (!UUID_RE.test(scope.accountId) || !UUID_RE.test(scope.brandId)) return null;
  const intent = pending.get(scopeKey(scope));
  return intent && validIntent(intent, scope) ? intent : null;
}
export function setPendingNotificationTemplateWrite(scope: NotificationTemplateWriteScope, intent: PendingNotificationTemplateWrite): void {
  if (validIntent(intent, scope)) pending.set(scopeKey(scope), copyIntent(intent));
}
export function clearPendingNotificationTemplateWrite(scope: NotificationTemplateWriteScope, expectedKey?: string): void {
  const key = scopeKey(scope);
  if (expectedKey === undefined || pending.get(key)?.key === expectedKey) pending.delete(key);
}
/** Parent logout can clear every unresolved write in the page session. */
export function clearAllPendingTemplateWrites(): void { pending.clear(); sessionGeneration += 1; }
export function notificationTemplateSessionGeneration(): number { return sessionGeneration; }
export function classifyNotificationTemplateFailure(status: number | undefined): "unknown" | "definitive" {
  return status === undefined || status === 0 || status >= 500 ? "unknown" : "definitive";
}
