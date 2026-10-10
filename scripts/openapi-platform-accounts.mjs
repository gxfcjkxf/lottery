const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required) => ({ type: 'object', properties, required, additionalProperties: false });
const array = items => ({ type: 'array', items });
const string = (extra = {}) => ({ type: 'string', ...extra });
const integer = () => ({ type: 'integer', format: 'int64', minimum: 1 });
const op = (method, path, operationId, summary, tag, auth, idempotency, data, extra = {}) => ({ method, path, operationId, summary, tag, auth, idempotency, data, ...extra });

export const schemas = {
  PlatformAccountCreateRequest: obj({ username: string({ pattern: '^[a-z][a-z0-9_]{2,31}$' }), password: string({ minLength: 16, maxLength: 128 }), role_ids: { ...array(ref('UUID')), minItems: 1, maxItems: 100 }, reason: ref('Reason') }, ['username', 'password', 'role_ids', 'reason']),
  PlatformAccountUpdateRequest: obj({ version: integer(), status: string({ enum: ['active', 'disabled'] }), role_ids: { ...array(ref('UUID')), minItems: 1, maxItems: 100 }, reason: ref('Reason') }, ['version', 'status', 'role_ids', 'reason']),
  PlatformAccountPasswordResetRequest: obj({ version: integer(), password: string({ minLength: 16, maxLength: 128 }), reason: ref('Reason') }, ['version', 'password', 'reason']),
  PlatformAccountRole: obj({ id: ref('UUID'), brand_id: string({ const: '' }), code: string(), name: string(), status: string({ enum: ['active', 'disabled'] }), version: integer(), is_bootstrap: { type: 'boolean' }, permissions: array(string()) }, ['id', 'brand_id', 'code', 'name', 'status', 'version', 'is_bootstrap', 'permissions']),
  PlatformAccountRoleList: obj({ items: array(ref('PlatformAccountRole')) }, ['items']),
};
const page = [{ name: 'limit', in: 'query', schema: { type: 'integer', minimum: 1, maximum: 100, default: 50 } }, { name: 'offset', in: 'query', schema: { type: 'integer', minimum: 0, maximum: 1000000, default: 0 } }];
const base = '/api/v1/platform';
const description = 'Independent platform entry: only super administrator accounts are accepted; brand staff accounts are rejected. Requires the exact platform grant. No X-Brand-ID. This manages platform administrator accounts, not brand members or funds. Writes are versioned, audited and idempotent; session and permission are checked before cached receipts too. Self update/reset is rejected. Role delegation cannot exceed the actor’s platform grants.';
export const operations = [
  op('GET', `${base}/platform-accounts`, 'listPlatformAdministratorAccounts', 'List platform administrator accounts', 'administration', 'admin', false, ref('AdminAccountList'), { parameters: page, permissions: ['admin.view.platform'], description }),
  op('GET', `${base}/platform-roles`, 'listPlatformAdministratorRoles', 'List platform administrator roles', 'administration', 'admin', false, ref('PlatformAccountRoleList'), { parameters: page, permissions: ['role.view.platform'], description }),
  op('POST', `${base}/platform-accounts`, 'createPlatformAdministratorAccount', 'Create platform administrator account', 'administration', 'admin', true, ref('AdminAccount'), { requestBody: ref('PlatformAccountCreateRequest'), successStatus: 201, permissions: ['admin.write.platform'], description }),
  op('PATCH', `${base}/platform-accounts/{id}`, 'updatePlatformAdministratorAccount', 'Update platform administrator status and roles', 'administration', 'admin', true, ref('AdminAccount'), { requestBody: ref('PlatformAccountUpdateRequest'), permissions: ['admin.write.platform'], description }),
  op('POST', `${base}/platform-accounts/{id}/reset-password`, 'resetPlatformAdministratorPassword', 'Reset platform administrator password', 'administration', 'admin', true, ref('IdentityAuditResult'), { requestBody: ref('PlatformAccountPasswordResetRequest'), permissions: ['admin.write.platform'], description }),
];
