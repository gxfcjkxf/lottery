const ref = (name) => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = [], extra = {}) => ({
  type: "object",
  properties,
  ...(required.length ? { required } : {}),
  additionalProperties: false,
  ...extra,
});
const string = (extra = {}) => ({ type: "string", ...extra });
const integer = (extra = {}) => ({ type: "integer", format: "int64", ...extra });
const array = (items, extra = {}) => ({ type: "array", items, ...extra });
const joinCode = string({ pattern: "^\\s*(?:[A-Fa-f0-9]{24})?\\s*$", description: "Empty or whitespace-only values mean not supplied; otherwise the server trims, uppercases, and requires exactly 24 hexadecimal characters." });
const mutuallyExclusiveNonemptyJoinCodes = () => ({
  allOf: [
    { if: { properties: { agent_code: { pattern: "\\S" } }, required: ["agent_code"] }, then: { properties: { referral_code: { pattern: "^\\s*$" } } } },
    { if: { properties: { referral_code: { pattern: "\\S" } }, required: ["referral_code"] }, then: { properties: { agent_code: { pattern: "^\\s*$" } } } },
  ],
});
const auditID = { audit_log_id: ref("UUID") };
const auditResult = obj(auditID, ["audit_log_id"]);
const emptyBody = ref("EmptyObject");
const pagination = [
  { name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 50 }, description: "Page size (default 50)." },
  { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of records to skip." },
];
const brandHeader = (required = true) => ({
  name: "X-Brand-ID",
  in: "header",
  required,
  schema: ref("UUID"),
  description: required ? "Target brand; must be an authorized brand even with platform scope." : "Optional brand filter. Omit only when using the platform-scope permission to query across brands.",
});

export const schemas = {
  IdentityChallenge: obj({
    id: ref("UUID"),
    svg: string({ description: "Inline SVG image for a CAPTCHA challenge; present for CAPTCHA challenges." }),
    nonce: string({ description: "One-time OIDC nonce; present for Telegram challenges." }),
    expires_at: ref("DateTime"),
  }, ["id", "expires_at"]),
  IdentityUser: obj({
    id: ref("UUID"),
    username: string(),
    phone: string(),
    telegram_user_id: string(),
    status: string(),
  }, ["id", "status"]),
  IdentityMember: obj({
    id: ref("UUID"),
    brand_id: ref("UUID"),
    status: string(),
    display_name: string(),
    joined_at: ref("DateTime"),
  }, ["id", "brand_id", "status", "display_name", "joined_at"]),
  IdentityView: obj({ user: ref("IdentityUser"), member: ref("IdentityMember") }, ["user", "member"]),
  IdentityAuthentication: obj({
    user: ref("IdentityUser"),
    member: ref("IdentityMember"),
    access_token: string(),
    token_type: string({ enum: ["Bearer"] }),
    expires_at: ref("DateTime"),
  }, ["user", "member", "access_token", "token_type", "expires_at"]),
  IdentityRegisterRequest: obj({
    agent_code: joinCode,
    referral_code: joinCode,
    captcha_id: ref("UUID"),
    captcha_answer: string({ maxLength: 128 }),
    username: string({ minLength: 3, maxLength: 32, pattern: "^[A-Za-z][A-Za-z0-9_]{2,31}$" }),
    phone: string({ pattern: "^\\+[1-9][0-9]{6,14}$" }),
    password: string({ minLength: 10, maxLength: 128, description: "10–128 UTF-8 bytes." }),
    privacy_policy_version: string({ minLength: 1 }),
    service_terms_version: string({ minLength: 1 }),
  }, ["password", "privacy_policy_version", "service_terms_version"], {
    anyOf: [{ required: ["username"] }, { required: ["phone"] }],
    ...mutuallyExclusiveNonemptyJoinCodes(),
    description: "At least one of username or phone is required. Empty attribution fields mean not supplied; non-empty agent_code and referral_code are mutually exclusive. A supplied code is trimmed, uppercased, and must contain 24 hexadecimal characters. Unknown keys and null values are rejected.",
  }),
  IdentityLoginRequest: obj({
    agent_code: joinCode,
    referral_code: joinCode,
    captcha_id: ref("UUID"),
    captcha_answer: string({ maxLength: 128 }),
    identifier: string({ minLength: 1 }),
    password: string({ minLength: 1 }),
    privacy_policy_version: string(),
    service_terms_version: string(),
  }, ["identifier", "password"], {
    ...mutuallyExclusiveNonemptyJoinCodes(),
    description: "Unknown keys and null values are rejected. Empty attribution fields mean not supplied; non-empty codes are mutually exclusive and apply only to a first-time brand join.",
  }),
  IdentityProfileRequest: obj({
    username: string({ minLength: 3, maxLength: 32, pattern: "^[A-Za-z][A-Za-z0-9_]{2,31}$" }),
    phone: string({ pattern: "^\\+[1-9][0-9]{6,14}$" }),
  }, [], { anyOf: [{ required: ["username"] }, { required: ["phone"] }], description: "Only previously unset username/phone fields can be supplied; at least one must be non-empty." }),
  IdentityTelegramRequest: obj({
    agent_code: joinCode,
    referral_code: joinCode,
    id_token: string({ minLength: 1 }),
    challenge_id: ref("UUID"),
    nonce: string({ minLength: 1 }),
    privacy_policy_version: string({ minLength: 1 }),
    service_terms_version: string({ minLength: 1 }),
    bind: { type: "boolean" },
  }, ["id_token", "challenge_id", "nonce", "privacy_policy_version", "service_terms_version"], {
    ...mutuallyExclusiveNonemptyJoinCodes(),
    description: "Requires a real Telegram OIDC id_token verified by the server against its configured client ID and this one-time nonce. bind=true requires an existing user session. Empty attribution fields mean not supplied; non-empty codes are mutually exclusive. Telegram login is disabled until configured.",
  }),
  IdentityAuditResult: auditResult,
  IdentityContextBrand: obj({
    id: ref("UUID"), code: string(), name: string(), status: string(), default_locale: string(), timezone: string(),
    theme: ref("ArbitraryJSON"), config_version: integer(),
  }, ["id", "code", "name", "status", "default_locale", "timezone", "theme", "config_version"]),
  IdentityContext: obj({
    brand: ref("IdentityContextBrand"),
    available_locales: array(string()),
    features: obj({ pwa: { type: "boolean" }, real_payments: { type: "boolean" } }, ["pwa", "real_payments"]),
    auth: obj({ captcha_enabled: { type: "boolean" }, telegram_enabled: { type: "boolean" }, telegram_client_id: string() }, ["captcha_enabled", "telegram_enabled"]),
    terms: obj({ privacy_policy_version: string(), service_terms_version: string() }, ["privacy_policy_version", "service_terms_version"]),
  }, ["brand", "available_locales", "features", "auth", "terms"]),
  IdentityHealth: obj({ status: string({ enum: ["alive", "ready"] }) }, ["status"]),

  AdminAuthentication: obj({ access_token: string(), token_type: string({ enum: ["Bearer"] }), expires_at: ref("DateTime") }, ["access_token", "token_type", "expires_at"]),
  AdminLoginRequest: obj({
    agent_code: string({ description: "Accepted by the shared login DTO but ignored by admin authentication." }),
    referral_code: string({ description: "Accepted by the shared login DTO but ignored by admin authentication." }),
    captcha_id: ref("UUID"), captcha_answer: string({ maxLength: 128 }),
    identifier: string({ minLength: 1 }), password: string({ minLength: 1 }),
    privacy_policy_version: string(), service_terms_version: string(),
  }, ["identifier", "password"], { description: "Unknown keys and null values are rejected. The additional fields are accepted by the shared LoginInput decoder but ignored by admin authentication." }),
  AdminMe: obj({ account: obj({
    id: ref("UUID"), super_admin: { type: "boolean" }, brand_ids: array(ref("UUID")), permissions: array(string()),
    version: integer(), permissions_by_brand: { type: "object", additionalProperties: array(string()) }, platform_permissions: array(string()),
  }, ["id", "super_admin", "brand_ids", "permissions", "version", "permissions_by_brand", "platform_permissions"]) }, ["account"]),
  AdminBrand: obj({ id: ref("UUID"), code: string(), name: string(), status: string() }, ["id", "code", "name", "status"]),
  AdminBrandList: obj({ items: array(ref("AdminBrand")) }, ["items"]),
  AdminMember: obj({
    id: ref("UUID"), global_user_id: ref("UUID"), username: string(), phone: string(), display_name: string(), notes: string(),
    status: string({ enum: ["normal", "frozen", "disabled", "expired", "cancelled"] }), joined_at: ref("DateTime"), brand_id: ref("UUID"), tags: array(string()),
  }, ["id", "global_user_id", "username", "phone", "display_name", "notes", "status", "joined_at", "brand_id", "tags"]),
  AdminMemberList: obj({ items: array(ref("AdminMember")) }, ["items"]),
  AdminMemberUpdateRequest: obj({ status: string({ enum: ["normal", "frozen", "disabled", "expired", "cancelled"] }), notes: string({ maxLength: 2000 }), reason: ref("Reason"), password: string({ minLength: 10, maxLength: 128 }) }, ["status", "reason"], { description: "PATCH uses status, notes and reason; password is accepted in the shared request shape but used only by reset-password. Reason is required. Unknown keys are rejected." }),
  AdminMemberKickRequest: obj({ status: string(), notes: string({ maxLength: 2000 }), reason: ref("Reason"), password: string() }, ["reason"], { description: "Only reason is used; the handler's shared closed request DTO also accepts the other listed fields. notes, if supplied, cannot exceed 2000 bytes." }),
  AdminMemberPasswordResetRequest: obj({ status: string(), notes: string({ maxLength: 2000 }), reason: ref("Reason"), password: string({ minLength: 10, maxLength: 128, description: "10–128 UTF-8 bytes." }) }, ["reason", "password"], { description: "Only password and reason are used; shared handler DTO accepts status and notes. notes, if supplied, cannot exceed 2000 bytes. Reset requires password_reset permission for every brand the global identity belongs to." }),
  AdminAuditEntry: obj({
    id: ref("UUID"), action: string(), actor_type: string(), actor_id: string(), resource_type: string(), resource_id: string(),
    reason: string(), request_id: string(), created_at: ref("DateTime"), ip_address: string(), before_json: ref("ArbitraryJSON"), after_json: ref("ArbitraryJSON"),
  }, ["id", "action", "actor_type", "actor_id", "resource_type", "resource_id", "reason", "request_id", "created_at", "ip_address", "before_json", "after_json"]),
  AdminAuditList: obj({ items: array(ref("AdminAuditEntry")) }, ["items"]),
  AdminPermissionList: obj({ items: array(string()) }, ["items"]),
  AdminRole: obj({
    id: ref("UUID"), brand_id: ref("UUID"), code: string(), name: string(), status: string({ enum: ["active", "disabled"] }),
    version: integer(), is_bootstrap: { type: "boolean" }, permissions: array(string()), audit_log_id: ref("UUID"),
  }, ["id", "brand_id", "code", "name", "status", "version", "is_bootstrap", "permissions"]),
  AdminRoleList: obj({ items: array(ref("AdminRole")) }, ["items"]),
  AdminRoleCreateRequest: obj({
    version: integer({ minimum: 0 }), code: string({ pattern: "^[a-z][a-z0-9_]{2,47}$" }), name: string({ minLength: 1, maxLength: 120 }),
    status: string({ enum: ["active", "disabled"] }), permissions: array(string({ pattern: "^[a-z][a-z0-9_]*\\.[a-z][a-z0-9_]*\\.brand$" }), { maxItems: 100 }), reason: ref("Reason"),
  }, ["code", "name", "reason"], { description: "status defaults to active; version is ignored at creation. Only brand-scoped permissions can be assigned." }),
  AdminRoleUpdateRequest: obj({
    version: integer({ minimum: 1 }), code: string({ maxLength: 0 }), name: string({ minLength: 1, maxLength: 120 }),
    status: string({ enum: ["active", "disabled"] }), permissions: array(string(), { maxItems: 100 }), reason: ref("Reason"),
  }, ["version", "name", "status", "reason"], { description: "code cannot be changed and must be omitted or empty. permissions may be empty; only brand-scoped registered permissions are accepted." }),
  AdminAccount: obj({
    id: ref("UUID"), username: string(), status: string({ enum: ["active", "disabled"] }), version: integer(), super_admin: { type: "boolean" },
    brand_ids: array(ref("UUID")), role_ids: array(ref("UUID")), role_codes: array(string()), audit_log_id: ref("UUID"),
  }, ["id", "username", "status", "version", "super_admin", "brand_ids", "role_ids", "role_codes"]),
  AdminAccountList: obj({ items: array(ref("AdminAccount")) }, ["items"]),
  AdminAccountCreateRequest: obj({
    username: string({ pattern: "^[a-z][a-z0-9_]{2,31}$" }), password: string({ minLength: 16, maxLength: 128 }),
    role_ids: array(ref("UUID"), { minItems: 1, maxItems: 100 }), reason: ref("Reason"),
  }, ["username", "password", "role_ids", "reason"], { description: "Creates a normal, active, non-super-admin account scoped to the selected brand." }),
  AdminAccountUpdateRequest: obj({
    version: integer({ minimum: 1 }), status: string({ enum: ["active", "disabled"] }), role_ids: array(ref("UUID"), { minItems: 1, maxItems: 100 }), reason: ref("Reason"),
  }, ["version", "status", "role_ids", "reason"], { description: "The username is immutable. Updating an account revokes its active sessions; self-edit and super-admin targets are denied." }),
  AdminAccountPasswordResetRequest: obj({ version: integer({ minimum: 1 }), password: string({ minLength: 16, maxLength: 128 }), reason: ref("Reason") }, ["version", "password", "reason"], { description: "Resets password and revokes all target admin sessions. Super-admin targets are denied." }),
  AdminOperatorCreateRequest: obj({
    agent_code: joinCode, referral_code: joinCode,
    username: string({ pattern: "^[A-Za-z][A-Za-z0-9_]{2,31}$" }), phone: string({ pattern: "^\\+[1-9][0-9]{6,14}$" }),
    password: string({ minLength: 10, maxLength: 128 }), display_name: string({ maxLength: 120 }), notes: string({ maxLength: 2000 }), reason: ref("Reason"),
  }, ["password", "reason"], { anyOf: [{ required: ["username"] }, { required: ["phone"] }], ...mutuallyExclusiveNonemptyJoinCodes(), description: "At least one of username or phone is required. Creates a new global identity and pending-consent member; it does not reuse existing identities or create a session. Empty attribution fields mean not supplied; non-empty codes are mutually exclusive." }),
  AdminOperatorCreateResult: obj({ user_id: ref("UUID"), member_id: ref("UUID"), brand_id: ref("UUID"), terms_accepted: { type: "boolean", enum: [false] }, audit_log_id: ref("UUID") }, ["user_id", "member_id", "brand_id", "terms_accepted", "audit_log_id"]),
  AdminAuthSettings: obj({ version: integer(), captcha_enabled: { type: "boolean" }, telegram_enabled: { type: "boolean" }, telegram_client_id: string(), privacy_policy_version: string(), service_terms_version: string() }, ["version", "captcha_enabled", "telegram_enabled", "telegram_client_id", "privacy_policy_version", "service_terms_version"]),
  AdminAuthSettingsRequest: obj({
    version: integer({ minimum: 1 }), captcha_enabled: { type: "boolean" }, telegram_enabled: { type: "boolean" }, telegram_client_id: string(), reason: ref("Reason"),
  }, ["version", "captcha_enabled", "telegram_enabled", "telegram_client_id", "reason"], { description: "Client ID is an empty string when disabled or a positive safe-integer decimal string without leading zeroes. Enabling Telegram requires a configured client ID." }),
  AdminAuthSettingsUpdated: obj({ version: integer(), captcha_enabled: { type: "boolean" }, telegram_enabled: { type: "boolean" }, telegram_client_id: string(), privacy_policy_version: string(), service_terms_version: string(), audit_log_id: ref("UUID") }, ["version", "captcha_enabled", "telegram_enabled", "telegram_client_id", "privacy_policy_version", "service_terms_version", "audit_log_id"]),
  AdminBrandOperation: obj({
    brand_id: ref("UUID"), version: integer({ minimum: 1 }), name: string(), status: string({ enum: ["active", "paused"] }),
    updated_at: ref("DateTime"), audit_log_id: ref("UUID"),
  }, ["brand_id", "version", "name", "status", "updated_at"]),
  AdminBrandOperationUpdateRequest: obj({
    version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused"] }), reason: ref("Reason"),
  }, ["version", "status", "reason"]),
  AdminBrandOperationRevision: obj({
    id: ref("UUID"), brand_id: ref("UUID"), version: integer({ minimum: 1 }), previous_status: string({ enum: ["active", "paused"] }),
    status: string({ enum: ["active", "paused"] }), changed_by: ref("UUID"), reason: ref("Reason"), audit_log_id: ref("UUID"), created_at: ref("DateTime"),
  }, ["id", "brand_id", "version", "previous_status", "status", "changed_by", "reason", "audit_log_id", "created_at"]),
  AdminBrandOperationHistory: obj({ items: array(ref("AdminBrandOperationRevision")), limit: integer({ minimum: 1, maximum: 100 }), offset: integer({ minimum: 0 }) }, ["items", "limit", "offset"]),
};

const op = (method, path, operationId, summary, tag, auth, idempotency, data, extra = {}) => ({
  method, path, operationId, summary, tag, auth, idempotency, data,
  successStatus: 200, parameters: [], permissions: [], brandHeader: false, description: "",
  ...extra,
});

export const operations = [
  op("GET", "/health/live", "healthLive", "Check process liveness", "system", "public", false, ref("IdentityHealth"), { description: "Returns alive while the HTTP process is serving." }),
  op("GET", "/health/ready", "healthReady", "Check service readiness", "system", "public", false, ref("IdentityHealth"), { description: "Checks the configured readiness dependency with a two-second timeout; unavailable readiness returns 503." }),
  op("GET", "/api/v1/context", "getPublicContext", "Get the current brand and public configuration", "identity", "public", false, ref("IdentityContext"), { description: "Resolves the brand from the request host or generated platform brand path. Includes current authentication switches and policy versions; no secrets are returned." }),

  op("POST", "/api/v1/auth/register", "registerIdentity", "Register a user and join the current brand", "identity", "public", true, ref("IdentityAuthentication"), { requestBody: ref("IdentityRegisterRequest"), successStatus: 201, description: "Creates a new global identity and brand member, accepts only current policy versions, and issues a user session. Existing identities are not reused; attribution codes are optional and mutually exclusive." }),
  op("POST", "/api/v1/auth/login", "loginIdentity", "Log in with username or phone", "identity", "public", true, ref("IdentityAuthentication"), { requestBody: ref("IdentityLoginRequest"), description: "Authenticates the global account for the resolved brand. Joining a new brand requires current policy consent; a prior account is not implicitly linked to a new member." }),
  op("POST", "/api/v1/auth/logout", "logoutIdentity", "Revoke the current user session", "identity", "session", true, ref("IdentityAuditResult"), { requestBody: emptyBody, description: "Revokes the current brand-scoped user session and returns its audit log ID." }),
  op("GET", "/api/v1/me", "getCurrentIdentity", "Get the current user and brand membership", "identity", "user", false, ref("IdentityView"), { description: "Requires an active user session for the host-resolved brand." }),
  op("PATCH", "/api/v1/me/profile", "fillIdentityProfile", "Fill previously unset profile fields", "identity", "user", true, ref("IdentityAuditResult"), { requestBody: ref("IdentityProfileRequest"), description: "Only fills a username or phone that has never been set; set values are immutable. Returns an audit log ID." }),
  op("GET", "/api/v1/auth/challenge", "getCaptchaChallenge", "Create a CAPTCHA challenge", "identity", "public", false, ref("IdentityChallenge"), { description: "Returns a one-time SVG CAPTCHA challenge with a five-minute expiry. The handler exposes this endpoint regardless of the captcha_enabled setting; subject to persistent authentication rate limits." }),
  op("GET", "/api/v1/auth/telegram/challenge", "getTelegramChallenge", "Create a Telegram OIDC nonce challenge", "identity", "public", false, ref("IdentityChallenge"), { description: "Requires Telegram login to be enabled and configured for the resolved brand. Returns a one-time nonce and five-minute expiry." }),
  op("POST", "/api/v1/auth/telegram", "loginWithTelegram", "Authenticate using Telegram OIDC", "identity", "public", true, ref("IdentityAuthentication"), { requestBody: ref("IdentityTelegramRequest"), description: "The server verifies the supplied real Telegram id_token against the configured client ID and one-time challenge nonce. No token-availability mock or arbitrary identity assertion is accepted. bind=true requires a current user session and binding cannot be replaced or removed." }),

  op("POST", "/api/v1/admin/auth/login", "adminLogin", "Log in to the administration API", "administration", "public", true, ref("AdminAuthentication"), { requestBody: ref("AdminLoginRequest"), description: "Authenticates an administrative account and issues an independent 12-hour admin session. The host must resolve to a configured brand; no X-Brand-ID header is used." }),
  op("POST", "/api/v1/admin/auth/logout", "adminLogout", "Revoke the current admin session", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: emptyBody, description: "Revokes the current admin session and returns an audit log ID." }),
  op("GET", "/api/v1/admin/me", "getAdminIdentity", "Get the current admin account and permissions", "administration", "admin", false, ref("AdminMe"), { description: "Returns explicit brand and platform permission sets plus a compatibility flat union. Every successful read is audited." }),
  op("GET", "/api/v1/admin/brands", "listAdminBrands", "List brands visible to the current admin", "administration", "admin", false, ref("AdminBrandList"), { description: "Lists authorized brands, or brands visible under platform brand.view permission. The read is audited." }),
  op("GET", "/api/v1/admin/users", "listBrandMembers", "List brand members", "administration", "admin", false, ref("AdminMemberList"), { brandHeader: false, parameters: [brandHeader(false), ...pagination], permissions: ["user.view.brand", "user.view.platform"], description: "Requires either user.view.brand for the selected brand or user.view.platform. A platform viewer may omit X-Brand-ID to list across brands. Results are paginated and the read is audited." }),
  op("PATCH", "/api/v1/admin/users/{id}", "updateBrandMember", "Update a brand member status and notes", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminMemberUpdateRequest"), brandHeader: true, permissions: ["user.write.brand"], description: "The path ID is a brand-member UUID. Requires user.write.brand. reason is required; notes are limited to 2000 bytes. Status is normal, frozen, disabled, expired, or cancelled. Password and other brands are not changed." }),
  op("POST", "/api/v1/admin/users/{id}/kick", "kickBrandMember", "Revoke a member's sessions for this brand", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminMemberKickRequest"), brandHeader: true, permissions: ["user.kick.brand"], description: "The path ID is a brand-member UUID. Requires user.kick.brand and a reason. Revokes only sessions for this member in the selected brand." }),
  op("POST", "/api/v1/admin/users/{id}/reset-password", "resetBrandMemberPassword", "Reset the global user's password", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminMemberPasswordResetRequest"), brandHeader: true, permissions: ["user.password_reset.brand"], description: "The path ID is a brand-member UUID. Requires password-reset permission for every brand the global identity has joined; super-admins are denied. Replaces the global password and revokes all user sessions across brands." }),
  op("GET", "/api/v1/admin/audit", "listAdminAudit", "List audit records", "administration", "admin", false, ref("AdminAuditList"), { brandHeader: false, parameters: [brandHeader(false), ...pagination], permissions: ["audit.view.brand", "audit.view.platform"], description: "An admin with audit.view.platform may omit X-Brand-ID for all brands; otherwise a valid authorized brand and audit.view.brand are required. The response reflects the SQL projection actually emitted: items only, with before_json and after_json as arbitrary JSON values (including null). The read itself is appended to the audit log." }),
  op("GET", "/api/v1/admin/permissions", "listAdminPermissions", "List registered brand permission keys", "administration", "admin", false, ref("AdminPermissionList"), { brandHeader: true, permissions: ["role.view.brand", "role.view.platform"], description: "Requires role.view for the selected brand or platform scope. Returns registered brand-scoped permission keys in items; the read is audited." }),
  op("GET", "/api/v1/admin/roles", "listAdminRoles", "List brand roles", "administration", "admin", false, ref("AdminRoleList"), { brandHeader: true, parameters: [...pagination], permissions: ["role.view.brand", "role.view.platform"], description: "Requires role.view for the selected brand or platform scope. Results are paginated and the read is audited." }),
  op("POST", "/api/v1/admin/roles", "createAdminRole", "Create a brand role", "administration", "admin", true, ref("AdminRole"), { requestBody: ref("AdminRoleCreateRequest"), successStatus: 201, brandHeader: true, permissions: ["role.write.brand", "role.write.platform"], description: "Requires role.write in the selected brand or platform scope. Roles are brand-scoped; only registered brand permission keys may be assigned. Creation defaults status to active; success includes audit_log_id." }),
  op("PATCH", "/api/v1/admin/roles/{id}", "updateAdminRole", "Update a brand role", "administration", "admin", true, ref("AdminRole"), { requestBody: ref("AdminRoleUpdateRequest"), brandHeader: true, permissions: ["role.write.brand", "role.write.platform"], description: "Requires role.write in the selected brand or platform scope. Bootstrap roles, roles containing grants the actor does not hold, and roles currently assigned to the actor cannot be modified. Updates revoke sessions of accounts assigned to the role; success includes audit_log_id." }),
  op("GET", "/api/v1/admin/accounts", "listAdminAccounts", "List administrative accounts for a brand", "administration", "admin", false, ref("AdminAccountList"), { brandHeader: true, parameters: [...pagination], permissions: ["admin.view.brand", "admin.view.platform"], description: "Requires admin.view for the selected brand or platform scope. Results are paginated and the read is audited." }),
  op("POST", "/api/v1/admin/accounts", "createAdminAccount", "Create a brand-scoped admin account", "administration", "admin", true, ref("AdminAccount"), { requestBody: ref("AdminAccountCreateRequest"), successStatus: 201, brandHeader: true, permissions: ["admin.write.brand", "admin.write.platform"], description: "Requires admin.write in the selected brand or platform scope. Creates an ordinary active account scoped to that brand; callers cannot create a super-admin. Success includes audit_log_id." }),
  op("PATCH", "/api/v1/admin/accounts/{id}", "updateAdminAccount", "Update an admin account's status and roles", "administration", "admin", true, ref("AdminAccount"), { requestBody: ref("AdminAccountUpdateRequest"), brandHeader: true, permissions: ["admin.write.brand", "admin.write.platform"], description: "Requires admin.write in the selected brand or platform scope. Username and brand scopes are immutable here. Self, super-admin, multi-brand targets without platform scope, and targets with undelegated roles cannot be changed. Revokes target sessions; success includes audit_log_id." }),
  op("POST", "/api/v1/admin/accounts/{id}/reset-password", "resetAdminAccountPassword", "Reset an admin account password", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminAccountPasswordResetRequest"), brandHeader: true, permissions: ["admin.write.brand", "admin.write.platform"], description: "Requires admin.write in the selected brand or platform scope. Super-admin targets, self-reset, and unauthorized multi-brand targets are denied. Resets the password and revokes all target admin sessions." }),
  op("POST", "/api/v1/admin/users", "createUserForBrand", "Provision a new user and pending brand member", "administration", "admin", true, ref("AdminOperatorCreateResult"), { requestBody: ref("AdminOperatorCreateRequest"), successStatus: 201, brandHeader: true, permissions: ["user.create.brand"], description: "Requires user.create.brand and is denied to super-admins. Creates a new global identity and member only; existing usernames or phones are not linked. At least one username or phone is required. No session is issued; terms_accepted is false and success returns an audit log ID." }),
  op("GET", "/api/v1/admin/auth-settings", "getAdminAuthSettings", "Get brand authentication settings", "administration", "admin", false, ref("AdminAuthSettings"), { brandHeader: true, permissions: ["auth_config.view.brand", "auth_config.view.platform"], description: "Requires auth_config.view in the selected brand or platform scope. Returns current configuration version and read-only policy versions; the read is audited." }),
  op("PATCH", "/api/v1/admin/auth-settings", "updateAdminAuthSettings", "Update brand authentication settings", "administration", "admin", true, ref("AdminAuthSettingsUpdated"), { requestBody: ref("AdminAuthSettingsRequest"), brandHeader: true, permissions: ["auth_config.write.brand", "auth_config.write.platform"], description: "Requires auth_config.write in the selected brand or platform scope. Updates CAPTCHA/Telegram public login configuration only; policy text and versions are read-only. Telegram enablement requires a valid client ID. Version conflicts return 409 CONFIG_VERSION_CONFLICT; success includes audit_log_id." }),
  op("GET", "/api/v1/admin/brand-operation", "getAdminBrandOperation", "Get the selected brand's operating status", "administration", "admin", false, ref("AdminBrandOperation"), { brandHeader: true, permissions: ["brand_operation.view.brand", "brand_operation.view.platform"], description: "Requires an explicit brand_operation.view.brand grant for the selected brand or brand_operation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. The selected brand is identified only by X-Brand-ID." }),
  op("PATCH", "/api/v1/admin/brand-operation", "updateAdminBrandOperation", "Pause or resume the selected brand", "administration", "admin", true, ref("AdminBrandOperation"), { requestBody: ref("AdminBrandOperationUpdateRequest"), brandHeader: true, permissions: ["brand_operation.write.brand", "brand_operation.write.platform"], description: "Requires an explicit brand_operation.write.brand grant for the selected brand or brand_operation.write.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Idempotent replay returns the original successful receipt (200), which may describe an older version after later writes; issue GET to retrieve current state. Version conflicts return 409 CONFIG_VERSION_CONFLICT; brand operation shares the configuration version with brand and authentication configuration updates. The selected brand is identified only by X-Brand-ID." }),
  op("GET", "/api/v1/admin/brand-operation/history", "listAdminBrandOperationHistory", "List operating-status revisions for the selected brand", "administration", "admin", false, ref("AdminBrandOperationHistory"), { brandHeader: true, parameters: [{ name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 }, description: "Page size (default 20; maximum 100)." }, { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of revisions to skip (maximum 1000000)." }], permissions: ["brand_operation.view.brand", "brand_operation.view.platform"], description: "Requires an explicit brand_operation.view.brand grant for the selected brand or brand_operation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Returns newest revisions first for the brand selected by X-Brand-ID; there is no brand-ID path alias. Unknown or repeated query parameters are rejected." }),
];
