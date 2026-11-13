import { addHistoryReadHeaders } from "./history-reads.mjs";
import { schemas as platformAccountSchemas, operations as platformAccountOperations } from "../../scripts/openapi-platform-accounts.mjs";
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
const nullable = (schema) => ({ anyOf: [schema, { type: "null" }] });
const textString = (maxCharacters, extra = {}) => string({ maxLength: maxCharacters, description: `Maximum ${maxCharacters} Unicode characters.`, ...extra });
const joinCode = string({ pattern: "^\\s*(?:[A-Fa-f0-9]{24})?\\s*$", description: "Empty or whitespace-only values mean not supplied; otherwise the server trims, uppercases, and requires exactly 24 hexadecimal characters." });
const mutuallyExclusiveNonemptyJoinCodes = () => ({
  allOf: [
    { if: { properties: { agent_code: { pattern: "\\S" } }, required: ["agent_code"] }, then: { properties: { referral_code: { pattern: "^\\s*$" } } } },
    { if: { properties: { referral_code: { pattern: "\\S" } }, required: ["referral_code"] }, then: { properties: { agent_code: { pattern: "^\\s*$" } } } },
  ],
});
const auditID = { audit_log_id: ref("UUID") };
const auditResult = obj(auditID, ["audit_log_id"]);
const brandDomainName = string({ maxLength: 253, pattern: "^(?=.{1,253}$)(?=.*\\.)(?=.*\\.[a-z0-9-]*[a-z][a-z0-9-]*$)(?!.*\\.(?:localhost|local|localdomain|internal)$)(?!.*\\.0x[0-9a-f]+$)[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$", description: "Canonical lower-case ASCII dotted DNS hostname. New values cannot be URLs, IP addresses, wildcard or local names, localhost, or include a port or trailing dot; each label is at most 63 bytes and the hostname at most 253 bytes." });
const emptyBody = ref("EmptyObject");
const pagination = [
  { name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 50 }, description: "Page size (default 50)." },
  { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of records to skip." },
];
const memberUsername = { name: "username", in: "query", required: false, schema: string({ minLength: 1 }), description: "Exact member username filter, case-insensitive." };
const brandHeader = (required = true) => ({
  name: "X-Brand-ID",
  in: "header",
  required,
  schema: ref("UUID"),
  description: required ? "Target brand; must be an authorized brand even with platform scope." : "Optional brand filter. Omit only when using the platform-scope permission to query across brands.",
});
const auditDateParameters = [
  { name: "from", in: "query", required: false, schema: ref("DateTime"), description: "RFC3339Nano UTC lower bound with literal Z suffix, inclusive. Must be supplied with to." },
  { name: "to", in: "query", required: false, schema: ref("DateTime"), description: "RFC3339Nano UTC upper bound with literal Z suffix, exclusive. Must be supplied with from; the interval may not exceed 31 days." },
];
const auditFilters = [
  ...auditDateParameters,
  { name: "action", in: "query", required: false, schema: string({ minLength: 1 }) },
  { name: "actor", in: "query", required: false, schema: {type: "string", maxLength: 128}, description: "Case-insensitive literal substring of the operator username." },
  { name: "action_type", in: "query", required: false, schema: {type: "string", enum: ["create", "view", "query", "edit", "delete"]} },
  { name: "actor_id", in: "query", required: false, schema: ref("UUID") },
  { name: "resource_type", in: "query", required: false, schema: string({ minLength: 1 }) },
  { name: "resource_id", in: "query", required: false, schema: ref("UUID") },
  { name: "request_id", in: "query", required: false, schema: string({ minLength: 1 }) },
];
const auditReadParameters = [
  ...auditFilters,
  { name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 200, default: 100 } },
  { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 100000, default: 0 } },
];
const auditExportParameters = auditFilters.map(parameter => parameter.name === "from" || parameter.name === "to" ? { ...parameter, required: true } : parameter);
const auditReadDescription = "Requires audit.view.brand for the selected brand or audit.view.platform. An authorized platform viewer may omit X-Brand-ID to query across brands; when supplied it is a UUID. actor matches a case-insensitive literal username substring; action_type groups operations as create, view, query, edit or delete. Other filters are exact matches. from and to must be supplied together as RFC3339Nano UTC values ending in literal Z (numeric offsets are rejected), satisfy from < to, and span at most 31 days; the time window is half-open [from,to). limit defaults to 100 (1..200) and offset defaults to 0 (0..100000). Unknown, duplicate, or empty query values are rejected with 400. Results are ordered by created_at descending then id descending. If the selected records exceed the 4 MiB aggregate JSON cap, return 422 AUDIT_QUERY_TOO_LARGE as JSON and omit items; no partial list is returned. The successful response remains {items: AdminAuditRecord[]}; each record includes a required nullable brand_id, which is null for platform records, and a required actor_username, empty when the administrator, member or brand-scoped agent username cannot be resolved. Clients should use actor_username for account display and actor_type text for actors without a resolved account; retain actor_id only as the technical UUID. before_json and after_json are arbitrary JSON values, including null. The read audit is committed before response data is released.";
const auditExportDescription = "Requires an explicit X-Brand-ID UUID even for platform-authorized exports. The selected brand must have both audit.view.brand or audit.view.platform AND audit.export.brand or audit.export.platform; the view and export grants must apply to that same brand. Requires both RFC3339Nano UTC from and to values ending in literal Z (numeric offsets are rejected), with from < to and a maximum span of 31 days; filtering is half-open [from,to). Accepts the same optional exact action, actor_id, resource_type, resource_id, and request_id filters as GET /admin/audit. Unknown, duplicate, or empty query values, including limit or offset, return 400. Reads the primary database in one snapshot, selects every matching current-window row ordered by created_at descending then id descending, and fails with 422 AUDIT_EXPORT_TOO_LARGE as JSON without a file above 10000 rows or 4 MiB. The CSV is UTF-8 with BOM, RFC 4180 escaped, and uses UTC timestamps. Formula injection is neutralized when the first non-space character is +, -, =, or @, or when a cell contains a tab, CR, or LF. Columns, in order: id, brand_id, action, actor_type, actor_id, resource_type, resource_id, reason, request_id, created_at, ip_address, before_json, after_json. Headers include the explicit brand, RFC3339Nano snapshot time, row count, SHA-256 of the complete CSV bytes including BOM, format version 1, and export UUID. Current session and matching view/export grants are freshly checked before releasing the snapshot; the audit.export record is written on the primary and committed before any CSV bytes are sent. A client must validate complete metadata and digest before starting a blob download.";

export const schemas = {
  BrandOverview: obj({
    brand_id: ref('UUID'), snapshot_at: ref('DateTime'), timezone: string({ minLength: 1 }), today: string({ format: 'date' }),
    ...Object.fromEntries(['total_members','online_members','withdrawal_count'].map(key => [key,string({ pattern: '^(0|[1-9][0-9]*)$' })])),
    ...Object.fromEntries(['unsettled_points','available_points','frozen_points','withdrawal_points','yesterday_same_time_stake'].map(key => [key,string({ pattern: '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' })])),
    days: array(ref('PlatformOverviewDay'), { minItems: 7, maxItems: 7 }),
  }, ['brand_id','snapshot_at','timezone','today','total_members','online_members','unsettled_points','available_points','frozen_points','withdrawal_points','withdrawal_count','yesterday_same_time_stake','days']),
  PlatformOverviewDay: obj({
    day: string({ format: 'date' }),
    ...Object.fromEntries(['stake_points','order_count','bettor_count','refund_points','new_members'].map(key => [key,string({ pattern: '^(0|[1-9][0-9]*)$' })])),
    net_points: string({ pattern: '^(0|-?[1-9][0-9]*)$' }),
  }, ['day','stake_points','net_points','order_count','bettor_count','refund_points','new_members']),
  PlatformOverviewBrand: obj({ brand_id: ref('UUID'), brand_name: string({ minLength: 1 }), stake_points: string({ pattern: '^(0|[1-9][0-9]*)$' }), net_points: string({ pattern: '^(0|-?[1-9][0-9]*)$' }) }, ['brand_id','brand_name','stake_points','net_points']),
  PlatformOverview: obj({
    snapshot_at: ref('DateTime'), timezone: string({ minLength: 1 }), today: string({ format: 'date' }),
    ...Object.fromEntries(['total_members','online_members','withdrawal_count'].map(key => [key,string({ pattern: '^(0|[1-9][0-9]*)$' })])),
    ...Object.fromEntries(['unsettled_points','available_points','frozen_points','withdrawal_points','yesterday_same_time_stake'].map(key => [key,string({ pattern: '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$' })])),
    days: array(ref('PlatformOverviewDay'), { minItems: 7, maxItems: 7 }), brands: array(ref('PlatformOverviewBrand'), { maxItems: 10 }),
  }, ['snapshot_at','timezone','today','total_members','online_members','unsettled_points','available_points','frozen_points','withdrawal_points','withdrawal_count','yesterday_same_time_stake','days','brands']),
  ...platformAccountSchemas,
  IdentityChallenge: obj({
    id: ref("UUID"),
    puzzle: obj({image:string(),piece:string(),width:integer(),height:integer(),piece_width:integer(),piece_height:integer(),y:integer()},["image","piece","width","height","piece_width","piece_height","y"],{description:"GoCaptcha raster images and piece geometry; the target X is never sent to the client."}),
    nonce: string({ description: "One-time OIDC nonce; present for Telegram challenges." }),
    expires_at: ref("DateTime"),
  }, ["id", "expires_at"]),
  IdentityUser: obj({
    id: ref("UUID"),
    username: string(),
    phone: string(),
    wechat: string(),
    telegram: string(),
    whatsapp: string(),
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
    captcha_answer: string({ maxLength: 128, description: "Puzzle piece X coordinate in source-image pixels, as a decimal integer." }),
    username: string({ minLength: 3, maxLength: 32, pattern: "^[A-Za-z][A-Za-z0-9_]{2,31}$" }),
    phone: string({ pattern: "^\\+[1-9][0-9]{6,14}$" }),
    password: string({ minLength: 6, maxLength: 30, description: "6–30 characters." }),
  }, ["password"], {
    anyOf: [{ required: ["username"] }, { required: ["phone"] }],
    ...mutuallyExclusiveNonemptyJoinCodes(),
    description: "At least one of username or phone is required. Empty attribution fields mean not supplied; non-empty agent_code and referral_code are mutually exclusive. A supplied code is trimmed, uppercased, and must contain 24 hexadecimal characters. Unknown keys and null values are rejected.",
  }),
  IdentityLoginRequest: obj({
    agent_code: joinCode,
    referral_code: joinCode,
    captcha_id: ref("UUID"),
    captcha_answer: string({ maxLength: 128, description: "Puzzle piece X coordinate in source-image pixels, as a decimal integer." }),
    identifier: string({ minLength: 1 }),
    password: string({ minLength: 1 }),
  }, ["identifier", "password"], {
    ...mutuallyExclusiveNonemptyJoinCodes(),
    description: "Unknown keys and null values are rejected. Empty attribution fields mean not supplied; non-empty codes are mutually exclusive and apply only to a first-time brand join.",
  }),
  IdentityProfileRequest: obj({
    username: string({ minLength: 3, maxLength: 32, pattern: "^[A-Za-z][A-Za-z0-9_]{2,31}$" }),
    phone: string({ pattern: "^([0-9]{7,15}|\\+[1-9][0-9]{6,14})$" }),
    wechat: string({ maxLength: 120 }),
    telegram: string({ maxLength: 120 }),
    whatsapp: string({ maxLength: 120 }),
  }, [], { anyOf: ["username", "phone", "wechat", "telegram", "whatsapp"].map(key => ({ required: [key] })), description: "All contact fields are optional. Only previously unset fields can be supplied; at least one must be non-empty. Telegram contact is independent of Telegram sign-in binding." }),
  IdentityTelegramRequest: obj({
    agent_code: joinCode,
    referral_code: joinCode,
    id_token: string({ minLength: 1 }),
    challenge_id: ref("UUID"),
    nonce: string({ minLength: 1 }),
    bind: { type: "boolean" },
  }, ["id_token", "challenge_id", "nonce"], {
    ...mutuallyExclusiveNonemptyJoinCodes(),
    description: "Requires a real Telegram OIDC id_token verified by the server against its configured client ID and this one-time nonce. bind=true requires an existing user session. Empty attribution fields mean not supplied; non-empty codes are mutually exclusive. Policy versions are required only when creating a global account or first joining this brand; existing members can log in without them. Telegram login is disabled until configured.",
  }),
  IdentityAuditResult: auditResult,
  IdentityContextBrand: obj({
    id: ref("UUID"), code: string(), name: string(), status: string(), default_locale: string(), timezone: string(),
    theme: ref("AdminBrandPresentationEffective"), config_version: integer({ minimum: 1 }),
  }, ["id", "code", "name", "status", "default_locale", "timezone", "theme", "config_version"]),
  IdentityContext: obj({
    brand: ref("IdentityContextBrand"),
    available_locales: array(string({ enum: ["en", "zh-CN"] }), { minItems: 1, uniqueItems: true }),
    features: obj({ pwa: { type: "boolean" }, real_payments: { type: "boolean" } }, ["pwa", "real_payments"]),
    auth: obj({ captcha_enabled: { type: "boolean" }, telegram_enabled: { type: "boolean" }, telegram_client_id: string() }, ["captcha_enabled", "telegram_enabled"]),
  }, ["brand", "available_locales", "features", "auth"]),
  IdentityHealth: obj({ status: string({ enum: ["alive", "ready"] }) }, ["status"]),
  PublicDisplaySettings: obj({ theme: string({ enum: ['jade','vermilion','ocean','obsidian'] }), timezone: string(), date_format: string({ enum: ['YYYY-MM-DD', 'DD/MM/YYYY', 'MM/DD/YYYY'] }), time_format: string({ enum: ['HH:mm:ss', 'hh:mm:ss A'] }) }, ['theme', 'timezone', 'date_format', 'time_format']),

  AdminAuthentication: obj({ access_token: string(), token_type: string({ enum: ["Bearer"] }), expires_at: ref("DateTime") }, ["access_token", "token_type", "expires_at"]),
  AdminLoginRequest: obj({
    agent_code: string({ description: "Accepted by the shared login DTO but ignored by admin authentication." }),
    referral_code: string({ description: "Accepted by the shared login DTO but ignored by admin authentication." }),
    captcha_id: ref("UUID"), captcha_answer: string({ maxLength: 128, description: "Puzzle piece X coordinate in source-image pixels, as a decimal integer." }),
    identifier: string({ minLength: 1 }), password: string({ minLength: 1 }),
  }, ["identifier", "password"], { description: "Unknown keys and null values are rejected. CAPTCHA proof is required when the brand effective authentication settings enable it. Other additional fields are ignored by admin authentication." }),
  AdminSelfPasswordChangeRequest: obj({
    current_password: string({ minLength: 6, maxLength: 30, description: "6–30 characters." }),
    new_password: string({ minLength: 6, maxLength: 30, description: "6–30 characters." }),
  }, ["current_password", "new_password"], { description: "Changes only the authenticated administrator's password. The new password is 6–30 characters; successful change revokes the current administrative session." }),
  AdminMe: obj({ account: obj({
    id: ref("UUID"), username: string(), super_admin: { type: "boolean" }, brand_ids: array(ref("UUID")), permissions: array(string()),
    version: integer(), permissions_by_brand: { type: "object", additionalProperties: array(string()) }, platform_permissions: array(string()),
  }, ["id", "username", "super_admin", "brand_ids", "permissions", "version", "permissions_by_brand", "platform_permissions"]) }, ["account"]),
  AdminBrand: obj({ id: ref("UUID"), code: string(), name: string(), allow_custom_games: {type:"boolean",default:false}, has_custom_games: {type:"boolean"}, status: string() }, ["id", "code", "name", "status", "allow_custom_games", "has_custom_games"]),
  PlatformBrandSummary: obj({id:ref("UUID"),code:string(),name:string(),status:string(),member_count:string({pattern:"^(0|[1-9][0-9]*)$"}),agent_count:string({pattern:"^(0|[1-9][0-9]*)$"}),recharge_quota_available:nullable(ref("NonnegativePoints")),created_at:ref("DateTime")},["id","code","name","status","member_count","agent_count","recharge_quota_available","created_at"]),
  PlatformBrandSummaryList: obj({items:array(ref("PlatformBrandSummary"))},["items"]),
  AdminBrandList: obj({ items: array(ref("AdminBrand")) }, ["items"]),
  AdminBrandCreationInput: obj({
    code: string({pattern:"^[a-z][a-z0-9_]{0,47}$",minLength:1,maxLength:48}),
    name: textString(120,{description:"Nonempty, no surrounding whitespace or Unicode control characters; maximum 120 characters."}),
    default_locale: string({enum:["en","zh-CN"]}),
    timezone: textString(80,{description:"Valid named IANA timezone or UTC; no empty/Local/surrounding whitespace/control characters."}),
    reason: textString(500,{description:"Nonempty, no surrounding whitespace or Unicode control characters; maximum 500 characters."}),
  },["code","name","default_locale","timezone"],{description:"Business keys are required; audit reason is optional. Unknown, duplicate and null fields rejected. No input status, brand id, initial money or administrator fields."}),
  AdminBrandCreationReceipt: obj({
    id:ref("UUID"),code:string(),name:string(),status:string({const:"paused"}),default_locale:string({enum:["en","zh-CN"]}),timezone:string(),version:integer({const:1}),created_at:ref("DateTime"),audit_log_id:ref("UUID"),
  },["id","code","name","status","default_locale","timezone","version","created_at","audit_log_id"],{description:"Immutable creation snapshot, not current status/version after subsequent configuration or resume. No auto account/scope/domain/game/fund creation."}),
  AdminBrandDeletionReceipt: obj({ id: ref("UUID") }, ["id"], { description: "Deletion receipt containing the permanently deleted brand ID. All brand-scoped related data is deleted in the same transaction." }),
  AdminMember: obj({
    id: ref("UUID"), global_user_id: ref("UUID"), username: string(), phone: string(), display_name: string(), notes: string(),
    status: string({ enum: ["normal", "frozen", "disabled", "expired", "cancelled"] }), joined_at: ref("DateTime"), brand_id: ref("UUID"), tags: array(string()), online: {type:"boolean",description:"Valid, unrevoked session active within five minutes in this brand."}, display_points: {anyOf:[string({pattern:"^(0|[1-9][0-9]*)$"}),{type:"null"}],description:"Available plus frozen points as an exact integer string. Null without wallet view permission."},
  }, ["id", "global_user_id", "username", "phone", "display_name", "notes", "status", "joined_at", "brand_id", "tags", "online", "display_points"]),
  AdminMemberList: obj({ items: array(ref("AdminMember")), limit: integer(), offset: integer(), total_count: string({pattern:"^(0|[1-9][0-9]*)$"}) }, ["items", "limit", "offset", "total_count"]),
  AdminMemberUpdateRequest: obj({ status: string({ enum: ["normal", "frozen", "disabled", "expired", "cancelled"] }), notes: string({ maxLength: 2000 }), reason: ref("Reason"), password: string({ minLength: 6, maxLength: 30 }) }, ["status"], { description: "PATCH uses status, notes and reason; password is accepted in the shared request shape but used only by reset-password. Reason is required. Unknown keys are rejected." }),
  AdminMemberKickRequest: obj({ status: string(), notes: string({ maxLength: 2000 }), reason: ref("Reason"), password: string() }, [], { description: "Only reason is used; the handler's shared closed request DTO also accepts the other listed fields. notes, if supplied, cannot exceed 2000 characters." }),
  AdminMemberDeleteRequest: obj({}, [], { description: "Empty request body. Deletion cancels the membership and revokes its brand sessions while retaining historical records." }),
  PlatformUserDeleteRequest: obj({ brand_id: ref("UUID") }, ["brand_id"], { description: "Selected brand is required in the body. No brand header is accepted." }),
  PlatformUserDeleteResult: obj({ id: ref("UUID"), account_name: string({ minLength: 1 }), audit_log_id: ref("UUID") }, ["id", "account_name", "audit_log_id"]),
  AdminMemberPasswordResetRequest: obj({ status: string(), notes: string({ maxLength: 2000 }), reason: ref("Reason"), password: string({ minLength: 6, maxLength: 30, description: "6–30 characters." }) }, ["password"], { description: "Only password and reason are used; shared handler DTO accepts status and notes. notes, if supplied, cannot exceed 2000 characters. Reset requires password_reset permission for every brand the global identity belongs to." }),
  AdminAuditEntry: obj({
    id: ref("UUID"), brand_id: nullable(ref("UUID")), action: string(), actor_type: string(), actor_id: string(), actor_username: string(), resource_type: string(), resource_id: string(),
    reason: string(), request_id: string(), created_at: ref("DateTime"), ip_address: string(), before_json: ref("ArbitraryJSON"), after_json: ref("ArbitraryJSON"),
  }, ["id", "brand_id", "action", "actor_type", "actor_id", "actor_username", "resource_type", "resource_id", "reason", "request_id", "created_at", "ip_address", "before_json", "after_json"]),
  AdminAuditList: obj({ items: array(ref("AdminAuditEntry")) }, ["items"]),
  AdminPermissionList: obj({ items: array(string()) }, ["items"]),
  AdminRole: obj({
    id: ref("UUID"), brand_id: ref("UUID"), code: string(), name: string(), status: string({ enum: ["active", "disabled"] }),
    version: integer(), is_bootstrap: { type: "boolean" }, permissions: array(string()), audit_log_id: ref("UUID"),
  }, ["id", "brand_id", "code", "name", "status", "version", "is_bootstrap", "permissions"]),
  AdminRoleList: obj({ items: array(ref("AdminRole")) }, ["items"]),
  AdminRoleCreateRequest: obj({
    version: integer({ minimum: 0 }), code: string({ pattern: "^[a-z][a-z0-9_]{2,47}$" }), name: string({ minLength: 1, maxLength: 120 }),
    status: string({ enum: ["active", "disabled"] }), permissions: array(string({ pattern: "^[a-z][a-z0-9_]*\\.[a-z][a-z0-9_]*\\.brand$" })), reason: ref("Reason"),
  }, ["code", "name"], { description: "status defaults to active; version is ignored at creation. Only brand-scoped permissions can be assigned." }),
  AdminRoleUpdateRequest: obj({
    version: integer({ minimum: 1 }), code: string({ maxLength: 0 }), name: string({ minLength: 1, maxLength: 120 }),
    status: string({ enum: ["active", "disabled"] }), permissions: array(string()), reason: ref("Reason"),
  }, ["version", "name", "status"], { description: "code cannot be changed and must be omitted or empty. permissions may be empty; only brand-scoped registered permissions are accepted." }),
  AdminRoleDeleteRequest: obj({
    version: integer({ minimum: 1 }),
  }, ["version"], { description: "Deletes an unassigned custom role. The brand bootstrap administrator role cannot be deleted." }),
  AdminAccount: obj({
    id: ref("UUID"), username: string(), status: string({ enum: ["active", "disabled"] }), version: integer(), super_admin: { type: "boolean" },
    brand_ids: array(ref("UUID")), role_ids: array(ref("UUID")), role_codes: array(string()), audit_log_id: ref("UUID"),
  }, ["id", "username", "status", "version", "super_admin", "brand_ids", "role_ids", "role_codes"]),
  AdminAccountList: obj({ items: array(ref("AdminAccount")) }, ["items"]),
  AdminAccountCreateRequest: obj({
    username: string({ pattern: "^[a-z][a-z0-9_]{2,31}$" }), password: string({ minLength: 6, maxLength: 30, description: "6–30 characters." }),
    role_ids: array(ref("UUID"), { minItems: 1, maxItems: 100 }), reason: ref("Reason"),
  }, ["username", "password", "role_ids"], { description: "Creates a normal, active, non-super-admin account scoped to the selected brand." }),
  AccountDeleteRequest: obj({
    version: integer({ minimum: 1 }),
  }, ["version"], { description: "Deletes a non-super-admin brand account. No reason is required; the selected brand must belong to the account." }),
  AdminAccountUpdateRequest: obj({
    version: integer({ minimum: 1 }), status: string({ enum: ["active", "disabled"] }), role_ids: array(ref("UUID"), { minItems: 1, maxItems: 100 }), reason: ref("Reason"),
  }, ["version", "status", "role_ids"], { description: "The username is immutable. Updating an account revokes its active sessions; self-edit and super-admin targets are denied." }),
  AdminAccountPasswordResetRequest: obj({ version: integer({ minimum: 1 }), password: string({ minLength: 6, maxLength: 30, description: "6–30 characters." }), reason: ref("Reason") }, ["version", "password"], { description: "Resets password and revokes all target admin sessions. Super-admin targets are denied." }),
  AdminOperatorCreateRequest: obj({
    agent_code: joinCode, referral_code: joinCode,
    username: string({ pattern: "^[A-Za-z][A-Za-z0-9_]{2,31}$" }), phone: string({ pattern: "^\\+[1-9][0-9]{6,14}$" }),
    password: string({ minLength: 6, maxLength: 30, description: "6–30 characters." }), display_name: string({ maxLength: 120 }), notes: string({ maxLength: 2000 }), reason: ref("Reason"),
  }, ["password"], { anyOf: [{ required: ["username"] }, { required: ["phone"] }], ...mutuallyExclusiveNonemptyJoinCodes(), description: "At least one of username or phone is required. Creates a new global identity and brand member; it does not reuse existing identities or create a session. Empty attribution fields mean not supplied; non-empty codes are mutually exclusive." }),
  AdminOperatorCreateResult: obj({ user_id: ref("UUID"), member_id: ref("UUID"), brand_id: ref("UUID"), audit_log_id: ref("UUID") }, ["user_id", "member_id", "brand_id", "audit_log_id"]),
  AdminAuthSettings: obj({ inherit_platform:{type:"boolean"}, version: integer(), captcha_enabled: { type: "boolean" }, telegram_enabled: { type: "boolean" }, telegram_client_id: string(), }, ["inherit_platform", "version", "captcha_enabled", "telegram_enabled", "telegram_client_id"]),
  AdminAuthSettingsRequest: obj({
    inherit_platform:{type:"boolean"}, version: integer({ minimum: 1 }), captcha_enabled: { type: "boolean" }, telegram_enabled: { type: "boolean" }, telegram_client_id: string(), reason: ref("Reason"),
  }, ["version", "captcha_enabled", "telegram_enabled", "telegram_client_id"], { description: "Client ID is an empty string when disabled or a positive safe-integer decimal string without leading zeroes. Enabling Telegram requires a configured client ID." }),
  AdminAuthSettingsUpdated: obj({ inherit_platform:{type:"boolean"}, version: integer(), captcha_enabled: { type: "boolean" }, telegram_enabled: { type: "boolean" }, telegram_client_id: string(), audit_log_id: ref("UUID") }, ["inherit_platform", "version", "captcha_enabled", "telegram_enabled", "telegram_client_id", "audit_log_id"]),
  AdminBrandOperation: obj({
    brand_id: ref("UUID"), version: integer({ minimum: 1 }), name: string(), status: string({ enum: ["active", "paused", "disabled"] }),
    updated_at: ref("DateTime"), audit_log_id: ref("UUID"), pause_starts_at: ref("DateTime"), pause_ends_at: ref("DateTime"),
  }, ["brand_id", "version", "name", "status", "updated_at"]),
  AdminBrandOperationUpdateRequest: obj({
    version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused", "disabled"] }),
    reason: string({ maxLength: 500, pattern: "^(?:$|\\S(?:[\\s\\S]*\\S)?)$", description: "Optional; omission and an empty string are allowed. Nonempty reasons must already be trimmed. Maximum 500 Unicode characters; null is rejected." }),
    pause_starts_at: ref("DateTime"), pause_ends_at: ref("DateTime"),
  }, ["version", "status"], {
    dependentRequired: { pause_starts_at: ["pause_ends_at"], pause_ends_at: ["pause_starts_at"] },
    allOf: [{ if: { anyOf: [{ required: ["pause_starts_at"] }, { required: ["pause_ends_at"] }] }, then: { properties: { status: { const: "paused" } } } }],
    description: "Without a pause window, changes apply immediately. A window requires status=paused and both RFC3339 timestamps; its end must be later than its start and the database current time. Future windows require a currently active brand and return active until the start; a window already started returns paused. A disabled brand cannot receive a timed pause. Without a current or requested window, an unchanged status returns 409 BRAND_OPERATION_STATE_CONFLICT. Any transition from or to disabled requires brand_operation.write.platform. Manual changes replace or cancel the existing window.",
  }),
  AdminBrandOperationRevision: obj({
    id: ref("UUID"), brand_id: ref("UUID"), version: integer({ minimum: 1 }), previous_status: string({ enum: ["active", "paused", "disabled"] }),
    status: string({ enum: ["active", "paused", "disabled"] }), changed_by: ref("UUID"), reason: string({  maxLength: 500 }), audit_log_id: ref("UUID"), created_at: ref("DateTime"), pause_starts_at: ref("DateTime"), pause_ends_at: ref("DateTime"),
  }, ["id", "brand_id", "version", "previous_status", "status", "changed_by", "reason", "audit_log_id", "created_at"]),
  AdminBrandOperationHistory: obj({ items: array(ref("AdminBrandOperationRevision")), limit: integer({ minimum: 1, maximum: 100 }), offset: integer({ minimum: 0 }) }, ["items", "limit", "offset"]),
  AdminBrandDomain: obj({ id: ref("UUID"), domain: string({ minLength: 1, maxLength: 253, description: "Stored hostname. Historical localhost/IP bindings may be returned and toggled, but cannot be renamed or reassigned. Deletion releases the live binding while retaining audit history." }), enabled: { type: "boolean" } }, ["id", "domain", "enabled"]),
  AdminBrandDomainRecord: obj({ brand_id: ref("UUID"), version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused", "disabled"] }), domains: array(ref("AdminBrandDomain"), { maxItems: 100, description: "Includes enabled and disabled bindings, sorted by domain then id." }), updated_at: ref("DateTime"), audit_log_id: ref("UUID") }, ["brand_id", "version", "status", "domains", "updated_at"]),
  AdminBrandDomainReceipt: obj({ brand_id: ref("UUID"), version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused", "disabled"] }), domains: array(ref("AdminBrandDomain"), { maxItems: 100, description: "Includes enabled and disabled bindings, sorted by domain then id." }), updated_at: ref("DateTime"), audit_log_id: ref("UUID") }, ["brand_id", "version", "status", "domains", "updated_at", "audit_log_id"]),
  AdminBrandDomainCreateRequest: obj({ version: integer({ minimum: 1 }), domain: brandDomainName, enabled: { type: "boolean" }, reason: ref("Reason") }, ["version", "domain", "enabled"]),
  AdminBrandDomainDeleteRequest: obj({ version: integer({ minimum: 1 }) }, ["version"]),
  AdminBrandDomainUpdateRequest: obj({ version: integer({ minimum: 1 }), enabled: { type: "boolean" }, reason: ref("Reason") }, ["version", "enabled"]),
  AdminBrandDomainRevision: obj({ id: ref("UUID"), brand_id: ref("UUID"), version: integer({ minimum: 1 }), changed_by: ref("UUID"), reason: ref("Reason"), audit_log_id: ref("UUID"), created_at: ref("DateTime"), before_domains: array(ref("AdminBrandDomain"), { maxItems: 100, description: "Audited pre-change snapshot, sorted by domain then id." }), domains: array(ref("AdminBrandDomain"), { maxItems: 100, description: "Audited post-change snapshot, sorted by domain then id; there may be zero or one primary domain." }) }, ["id", "brand_id", "version", "changed_by", "reason", "audit_log_id", "created_at", "before_domains", "domains"]),
  AdminBrandDomainHistory: obj({ items: array(ref("AdminBrandDomainRevision")), limit: integer({ minimum: 1, maximum: 100 }), offset: integer({ minimum: 0, maximum: 1000000 }) }, ["items", "limit", "offset"]),
  AdminBrandPresentationLocaleText: obj({ tagline: nullable(textString(160)), announcement: nullable(textString(2000, { description: "Maximum 2000 characters; rendered as plain text, never as markup." })) }, ["tagline", "announcement"]),
  AdminBrandPresentationContent: obj({ en: ref("AdminBrandPresentationLocaleText"), "zh-CN": ref("AdminBrandPresentationLocaleText") }, ["en", "zh-CN"]),
  AdminBrandPresentationConfig: obj({
    display_name: nullable(textString(80)),
    logo_text: nullable(textString(32)),
    logo_url: nullable(textString(512, { description: "Maximum 512 characters. Allowed assets end in .png, .jpg, .jpeg, .webp, or .ico; SVG is not accepted. Use either HTTPS with a syntactically valid DNS hostname (not an IP address, localhost, or a local hostname), with no port, user info, query, or fragment, or a /icons/ or /brand-assets/ relative path without query or fragment. The server validates hostname syntax but does not resolve DNS or guarantee a public IP. Values may not have surrounding whitespace, backslashes, percent signs, CR/LF/tab, or a '..' sequence." })),
    favicon_url: nullable(textString(512, { description: "Maximum 512 characters. Allowed assets end in .png, .jpg, .jpeg, .webp, or .ico; SVG is not accepted. Use either HTTPS with a syntactically valid DNS hostname (not an IP address, localhost, or a local hostname), with no port, user info, query, or fragment, or a /icons/ or /brand-assets/ relative path without query or fragment. The server validates hostname syntax but does not resolve DNS or guarantee a public IP. Values may not have surrounding whitespace, backslashes, percent signs, CR/LF/tab, or a '..' sequence." })),
    default_locale: nullable(string({ enum: ["en", "zh-CN"] })),
    available_locales: nullable(array(string({ enum: ["en", "zh-CN"] }), { minItems: 1, uniqueItems: true })),
    content: nullable(ref("AdminBrandPresentationContent")),
  }, ["display_name", "logo_text", "logo_url", "favicon_url", "default_locale", "available_locales", "content"], { description: "Every key is required. Null means inherit the brand's base value. The server rejects unknown keys, malformed URLs, invalid enum values, and strings exceeding their stated Unicode character limits." }),
  AdminBrandPresentationEffective: obj({
    display_name: string(), logo_text: string(), logo_url: nullable(string()), favicon_url: nullable(string()),
    default_locale: string({ enum: ["en", "zh-CN"] }), available_locales: array(string({ enum: ["en", "zh-CN"] }), { minItems: 1, uniqueItems: true }),
    content: obj({ en: obj({ tagline: string(), announcement: string() }, ["tagline", "announcement"]), "zh-CN": obj({ tagline: string(), announcement: string() }, ["tagline", "announcement"]) }, ["en", "zh-CN"]),
  }, ["display_name", "logo_text", "logo_url", "favicon_url", "default_locale", "available_locales", "content"], { description: "Fully resolved values: all fields except logo_url and favicon_url are non-null. Content always contains both supported locales with non-null tagline and announcement strings." }),
  AdminBrandPresentationRecord: obj({
    brand_id: ref("UUID"), version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused", "disabled"] }), base_name: string(),
    config: ref("AdminBrandPresentationConfig"), effective: ref("AdminBrandPresentationEffective"), updated_at: ref("DateTime"), audit_log_id: ref("UUID"),
  }, ["brand_id", "version", "status", "base_name", "config", "effective", "updated_at"]),
  AdminBrandPresentationReceipt: obj({
    brand_id: ref("UUID"), version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused", "disabled"] }), base_name: string(),
    config: ref("AdminBrandPresentationConfig"), effective: ref("AdminBrandPresentationEffective"), updated_at: ref("DateTime"), audit_log_id: ref("UUID"),
  }, ["brand_id", "version", "status", "base_name", "config", "effective", "updated_at", "audit_log_id"]),
  AdminBrandPresentationPutRequest: obj({ version: integer({ minimum: 1 }), config: ref("AdminBrandPresentationConfig"), reason: textString(500) }, ["version", "config"]),
  AdminBrandPresentationRevision: obj({
    id: ref("UUID"), brand_id: ref("UUID"), version: integer({ minimum: 1 }), config: ref("AdminBrandPresentationConfig"), effective: ref("AdminBrandPresentationEffective"),
    changed_by: ref("UUID"), reason: textString(500), audit_log_id: ref("UUID"), created_at: ref("DateTime"),
  }, ["id", "brand_id", "version", "config", "effective", "changed_by", "reason", "audit_log_id", "created_at"]),
  AdminBrandPresentationHistory: obj({ items: array(ref("AdminBrandPresentationRevision")), limit: integer({ minimum: 1, maximum: 100 }), offset: integer({ minimum: 0, maximum: 1000000 }) }, ["items", "limit", "offset"]),
};

const op = (method, path, operationId, summary, tag, auth, idempotency, data, extra = {}) => ({
  method, path, operationId, summary, tag, auth, idempotency, ...(data === undefined ? {} : {data}),
  successStatus: 200, parameters: [], permissions: [], brandHeader: false, description: "",
  ...extra,
});

export const operations = [

  { method: 'GET', path: '/api/v1/admin/overview', operationId: 'getBrandOverview', summary: 'Read the current brand dashboard', tag: 'administration', auth: 'admin', brandHeader: true, idempotency: false, permissions: ['overview.view.brand'], data: ref('BrandOverview'), successStatus: 200, parameters: [], description: 'Requires overview.view.brand and an X-Brand-ID within the current account scope. A registered central entry can select an authorized brand; a brand-bound entry must match its host brand. Rejects all query parameters. Returns a single statement snapshot of the current brand only: membership, valid session activity within five minutes, four betting engines, exact point balances and exception tasks. New members use first brand join time; order totals use placement, net win uses settlement, refunds use refund time. Uses platform calendar days. Released Niu risk reserves are not refunds or profit. No cross-brand ranking and no financial mutations.' },
  { method: 'GET', path: '/api/v1/platform/overview', operationId: 'getPlatformOverview', summary: 'Read the cross-brand platform overview', tag: 'administration', auth: 'admin', brandHeader: false, idempotency: false, permissions: ['overview.view.platform'], data: ref('PlatformOverview'), successStatus: 200, parameters: [], description: 'Platform administrator entry only. Requires overview.view.platform; rejects X-Brand-ID and all query parameters. One PostgreSQL snapshot across all four betting engines, exact decimal strings, seven platform-local calendar days, brand memberships summed across brands without cross-brand deduplication, new members by first brand join time, valid unrevoked member sessions active within five minutes counted once per brand member, current point balances, review withdrawals and exception tasks. Stakes use placement time, results settlement time, refunds actual refund time. Niu uses actual loss minus member winnings and excludes released risk reserves from refunds. Top ten brands ranked by today stake points. No business or financial mutations.' },
  ...platformAccountOperations,
  {method:"POST",path:"/api/v1/admin/brands",operationId:"createBrand",summary:"Create an initially paused brand",tag:"administration",auth:"admin",idempotency:true,brandHeader:false,permissions:["brand.create.platform"],requestBody:ref("AdminBrandCreationInput"),data:ref("AdminBrandCreationReceipt"),successStatus:201,description:"Global checked mutation independent of a selected brand. X-Brand-ID and query parameters rejected. Explicit platform permission, not super flag or brand grant alone. Code uniqueness, seven initial policy/presentation rows, immutable audited creation and encrypted platform idempotency are atomic. Existing configured platform entry permits administration even without an active brand. Replays recheck session/permission and return the same creation snapshot; no automatic scope grants, admins, domains, money or settlement enablement."},
  op("DELETE", "/api/v1/admin/brands/{id}", "deleteBrand", "Permanently delete a brand and its related data", "administration", "admin", true, ref("AdminBrandDeletionReceipt"), { requestBody: emptyBody, brandHeader: false, permissions: ["brand.delete.platform"], description: "Global checked platform mutation independent of a selected brand. X-Brand-ID and query parameters are rejected. Requires brand.delete.platform. Permanently deletes the selected brand and all related brand-scoped data in one transaction; the operation is audited with the operator and object information, and unknown results must be replayed with the same idempotency key." }),
  op("GET", "/health/live", "healthLive", "Check process liveness", "system", "public", false, ref("IdentityHealth"), { description: "Returns alive while the HTTP process is serving." }),
  op("GET", "/health/ready", "healthReady", "Check service readiness", "system", "public", false, ref("IdentityHealth"), { description: "Checks the configured readiness dependency with a two-second timeout; unavailable readiness returns 503." }),
  op('GET', '/api/v1/display-settings', 'getPublicDisplaySettings', 'Read platform date and time display policy', 'system', 'public', false, ref('PublicDisplaySettings'), { brandAlias: false, description: 'Shared by all four frontends. Returns the global theme, timezone and display formats; no administrative metadata. Responses are not cached.' }),
  op("GET", "/api/v1/context", "getPublicContext", "Get the current brand and public configuration", "identity", "public", false, ref("IdentityContext"), { description: "Resolves the brand from the request host or generated platform brand path. Includes current authentication switches and policy versions; no secrets are returned." }),

  op("POST", "/api/v1/auth/register", "registerIdentity", "Register a user and join the current brand", "identity", "public", true, ref("IdentityAuthentication"), { requestBody: ref("IdentityRegisterRequest"), successStatus: 201, description: "Creates a new global identity and brand member. Accepts only current policy versions and issues a user session. Existing identities are not reused; attribution codes are optional and mutually exclusive." }),
  op("POST", "/api/v1/auth/login", "loginIdentity", "Log in with username or phone", "identity", "public", true, ref("IdentityAuthentication"), { requestBody: ref("IdentityLoginRequest"), description: "Authenticates the global account for the resolved brand." }),
  op("POST", "/api/v1/auth/logout", "logoutIdentity", "Revoke the current user session", "identity", "session", true, ref("IdentityAuditResult"), { requestBody: emptyBody, description: "Revokes the current brand-scoped user session and returns its audit log ID." }),
  op("GET", "/api/v1/me", "getCurrentIdentity", "Get the current user and brand membership", "identity", "user", false, ref("IdentityView"), { description: "Requires an active user session for the host-resolved brand." }),
  op("PATCH", "/api/v1/me/profile", "fillIdentityProfile", "Fill previously unset profile fields", "identity", "user", true, ref("IdentityAuditResult"), { requestBody: ref("IdentityProfileRequest"), description: "Only fills previously unset username, phone, WeChat, Telegram or WhatsApp fields; set values are immutable. Returns an audit log ID." }),
  op("GET", "/api/v1/auth/challenge", "getCaptchaChallenge", "Create a CAPTCHA challenge", "identity", "public", false, ref("IdentityChallenge"), { description: "Returns a one-time slider puzzle CAPTCHA challenge with a five-minute expiry. The handler exposes this endpoint regardless of the captcha_enabled setting; subject to persistent authentication rate limits." }),
  op("GET", "/api/v1/auth/telegram/challenge", "getTelegramChallenge", "Create a Telegram OIDC nonce challenge", "identity", "public", false, ref("IdentityChallenge"), { description: "Requires Telegram login to be enabled and configured for the resolved brand. Returns a one-time nonce and five-minute expiry." }),
  op("POST", "/api/v1/auth/telegram", "loginWithTelegram", "Authenticate using Telegram OIDC", "identity", "public", true, ref("IdentityAuthentication"), { requestBody: ref("IdentityTelegramRequest"), description: "The server verifies the supplied real Telegram id_token against the configured client ID and one-time challenge nonce. No token-availability mock or arbitrary identity assertion is accepted. bind=true requires a current user session and binding cannot be replaced or removed." }),

  op("GET", "/api/v1/admin/auth/config", "getBrandAdminLoginConfig", "Read brand administrator CAPTCHA configuration", "administration", "public", false, obj({captcha_enabled:{type:"boolean"}},["captcha_enabled"]),{description:"A registered central management host returns the platform CAPTCHA switch without selecting a brand. A brand-bound development entry returns its effective brand configuration."}),
  op("GET", "/api/v1/admin/auth/challenge", "getBrandAdminCaptcha", "Create a brand administrator CAPTCHA", "administration", "public", false, ref("IdentityChallenge")),
  op("POST", "/api/v1/admin/auth/login", "adminLogin", "Log in to the administration API", "administration", "public", true, ref("AdminAuthentication"), { requestBody: ref("AdminLoginRequest"), description: "Authenticates an administrative account and issues an independent 12-hour admin session. The host must resolve to a configured brand; no X-Brand-ID header is used." }),
  op("POST", "/api/v1/admin/auth/logout", "adminLogout", "Revoke the current admin session", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: emptyBody, description: "Revokes the current admin session and returns an audit log ID." }),
  op("GET", "/api/v1/admin/me", "getAdminIdentity", "Get the current admin account and permissions", "administration", "admin", false, ref("AdminMe"), { description: "Returns explicit brand and platform permission sets plus a compatibility flat union. Every successful read is audited." }),
  op("POST", "/api/v1/admin/me/password", "changeAdminPassword", "Change the current admin password", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminSelfPasswordChangeRequest"), description: "Changes only the authenticated administrator's password. The current password is verified, the new password must be 6–30 characters, and all administrative sessions are revoked on success." }),
  op("GET", "/api/v1/platform/brands", "listPlatformBrandSummaries", "List authorized brands with member, agent and unused quota totals", "administration", "admin", false, ref("PlatformBrandSummaryList"), { permissions:["brand.view.platform"], description:"Counts all member and independent agent records once in each authorized brand, regardless of current status. Returns the real brand creation time and remaining recharge quota. Quota is null without recharge.view.platform permission. Counts and balances are exact decimal strings; no joins multiply counts. The read is audited." }),
  op("GET", "/api/v1/admin/brands", "listAdminBrands", "List brands visible to the current admin", "administration", "admin", false, ref("AdminBrandList"), { description: "Brand staff receive the basic directory of their explicitly assigned admin_brand_scopes without requiring brand.view.brand. This is session context discovery and grants no business read or write permissions. Unassigned brands are excluded. The separate platform directory retains its existing platform permission checks. The read is audited." }),
  op("GET", "/api/v1/admin/users", "listBrandMembers", "List brand members", "administration", "admin", false, ref("AdminMemberList"), { brandHeader: false, parameters: [brandHeader(false), ...pagination, memberUsername, {name:"q",in:"query",schema:string(),description:"Case-insensitive literal substring search across account, display name and phone before pagination."}, {name:"online",in:"query",schema:{type:"boolean"},description:"Filter recent active sessions before pagination."}], permissions: ["user.view.brand", "user.view.platform"], description: "Requires either user.view.brand for the selected brand or user.view.platform. A platform viewer may omit X-Brand-ID to list across brands. An optional exact username filter is case-insensitive. Results are paginated and the read is audited." }),
  op("PATCH", "/api/v1/admin/users/{id}", "updateBrandMember", "Update a brand member status and notes", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminMemberUpdateRequest"), brandHeader: true, permissions: ["user.write.brand"], description: "The path ID is a brand-member UUID. Requires user.write.brand. reason is required; notes are limited to 2000 characters. Status is normal, frozen, disabled, expired, or cancelled. Password and other brands are not changed." }),
  op("POST", "/api/v1/admin/users/{id}/kick", "kickBrandMember", "Revoke a member's sessions for this brand", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminMemberKickRequest"), brandHeader: true, permissions: ["user.kick.brand"], description: "The path ID is a brand-member UUID. Requires user.kick.brand and a reason. Revokes only sessions for this member in the selected brand." }),
  op("POST", "/api/v1/admin/users/{id}/delete", "deleteBrandMember", "Delete a brand member", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminMemberDeleteRequest"), brandHeader: true, permissions: ["user.delete.brand", "user.delete.platform"], description: "Cancels the membership and revokes sessions for the selected brand. Historical bets, wallet records and the global identity are retained. No reason is required." }),
  op("POST", "/api/v1/platform/global-users/{id}/delete", "deletePlatformUserAccount", "Remove member access to the selected brand", "administration", "admin", true, ref("PlatformUserDeleteResult"), { requestBody: ref("PlatformUserDeleteRequest"), brandHeader: false, permissions: ["user.delete.platform"], description: "The path ID is a global-user UUID, not a brand-member UUID. Requires a platform administrator with an explicit user.delete.platform grant. Rejects X-Brand-ID. Requires brand_id in the request body. Atomically cancels only that membership and revokes only sessions in that brand. Other memberships, credentials and sessions remain intact. Only when no uncancelled membership remains is the global identity set to deleted and its password removed. Historical identity identifiers, bets, balances and ledger records are retained; username/phone/Telegram identifiers remain reserved and cannot reactivate the identity. Returns the exact account name and audit evidence. The same idempotency key replays the original receipt after rechecking current authorization; a new deletion of an already-cancelled membership returns 404. No reason is required. The old platform brand-membership deletion route is not registered." }),
  op("POST", "/api/v1/admin/users/{id}/reset-password", "resetBrandMemberPassword", "Reset the global user's password", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminMemberPasswordResetRequest"), brandHeader: true, permissions: ["user.password_reset.brand"], description: "The path ID is a brand-member UUID. Requires password-reset permission for every brand the global identity has joined; super-admins are denied. Replaces the global password and revokes all user sessions across brands." }),
  op("GET", "/api/v1/admin/audit", "listAdminAudit", "List audit records", "administration", "admin", false, ref("AdminAuditList"), { brandHeader: false, parameters: [brandHeader(false), ...auditReadParameters], permissions: ["audit.view.brand", "audit.view.platform"], additionalErrorStatuses: [422], description: auditReadDescription }),
  { method: "GET", path: "/api/v1/admin/audit/export", operationId: "exportAdminAudit", summary: "Export audit records as CSV", tag: "administration", auth: "admin", brandHeader: true,
    permissions: ["audit.view.brand", "audit.view.platform", "audit.export.brand", "audit.export.platform"], parameters: auditExportParameters,
    additionalErrorStatuses: [422], successDescription: "Complete, bounded UTF-8 CSV v1, emitted only after the export audit record commits.",
    successContent: { "text/csv": { schema: { type: "string", format: "binary", description: "UTF-8 CSV with BOM, RFC 4180 quoting and CRLF rows. The server returns the complete matching snapshot or a JSON error; it never returns a partial file." } } },
    successHeaders: {
      "Content-Type": { description: "CSV media type.", schema: { type: "string", const: "text/csv; charset=utf-8" } },
      "Content-Length": { description: "Complete CSV byte length; maximum 4194304.", schema: { type: "string", pattern: "^(0|[1-9][0-9]*)$" } },
      "Content-Disposition": { description: "Download filename in audit-<UUID>-v1.csv form; the UUID is the selected brand ID.", schema: { type: "string", pattern: "^attachment; filename=\"audit-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}-v1\\.csv\"$" } },
      "Cache-Control": { description: "Exports are never cached.", schema: { type: "string", const: "no-store" } },
      "X-Content-Type-Options": { description: "Disable MIME sniffing.", schema: { type: "string", const: "nosniff" } },
      "X-Audit-Brand-ID": { description: "Explicitly selected brand, including platform-authorized exports.", schema: ref("UUID") },
      "X-Audit-Snapshot-At": { description: "Primary database snapshot timestamp in RFC3339Nano UTC.", schema: { ...ref("DateTime"), description: "RFC3339Nano UTC timestamp." } },
      "X-Audit-Row-Count": { description: "Number of matching audit rows included in the complete CSV.", schema: { type: "string", pattern: "^(0|[1-9][0-9]*)$" } },
      "X-Audit-SHA256": { description: "Lowercase SHA-256 digest of the complete response bytes, including BOM.", schema: { type: "string", pattern: "^[a-f0-9]{64}$" } },
      "X-Audit-Format-Version": { description: "CSV format version.", schema: { type: "string", const: "1" } },
      "X-Audit-Export-ID": { description: "Unique UUID of this export audit record.", schema: ref("UUID") },
    },
    description: auditExportDescription,
  },
  op("GET", "/api/v1/admin/permissions", "listAdminPermissions", "List registered brand permission keys", "administration", "admin", false, ref("AdminPermissionList"), { brandHeader: true, permissions: ["role.view.brand", "role.view.platform"], description: "Requires role.view for the selected brand or platform scope. Returns registered brand-scoped permission keys in items; the read is audited." }),
  op("GET", "/api/v1/admin/roles", "listAdminRoles", "List brand roles", "administration", "admin", false, ref("AdminRoleList"), { brandHeader: true, parameters: [...pagination], permissions: ["role.view.brand", "role.view.platform"], description: "Requires role.view for the selected brand or platform scope. Results are paginated and the read is audited." }),
  op("POST", "/api/v1/admin/roles", "createAdminRole", "Create a brand role", "administration", "admin", true, ref("AdminRole"), { requestBody: ref("AdminRoleCreateRequest"), successStatus: 201, brandHeader: true, permissions: ["role.write.brand", "role.write.platform"], description: "Requires role.write in the selected brand or platform scope. Roles are brand-scoped; only registered brand permission keys may be assigned. Creation defaults status to active; success includes audit_log_id." }),
  op("PATCH", "/api/v1/admin/roles/{id}", "updateAdminRole", "Update a brand role", "administration", "admin", true, ref("AdminRole"), { requestBody: ref("AdminRoleUpdateRequest"), brandHeader: true, permissions: ["role.write.brand", "role.write.platform"], description: "Requires role.write in the selected brand or platform scope. Bootstrap roles, roles containing grants the actor does not hold, and roles currently assigned to the actor cannot be modified. Updates revoke sessions of accounts assigned to the role; success includes audit_log_id." }),
  op("POST", "/api/v1/admin/roles/{id}/delete", "deleteAdminRole", "Delete a brand role", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminRoleDeleteRequest"), brandHeader: true, permissions: ["role.delete.brand", "role.delete.platform"], description: "Requires role.delete for the selected brand or platform scope. Only unassigned custom roles can be deleted; the single bootstrap administrator role is protected. Success includes audit_log_id." }),
  op("GET", "/api/v1/admin/accounts", "listAdminAccounts", "List administrative accounts for a brand", "administration", "admin", false, ref("AdminAccountList"), { brandHeader: true, parameters: [...pagination], permissions: ["admin.view.brand", "admin.view.platform"], description: "Requires admin.view for the selected brand or platform scope. Results are paginated and the read is audited." }),
  op("POST", "/api/v1/admin/accounts", "createAdminAccount", "Create a brand-scoped admin account", "administration", "admin", true, ref("AdminAccount"), { requestBody: ref("AdminAccountCreateRequest"), successStatus: 201, brandHeader: true, permissions: ["admin.write.brand", "admin.write.platform"], description: "Requires admin.write in the selected brand or platform scope. Creates an ordinary active account scoped to that brand; callers cannot create a super-admin. Success includes audit_log_id." }),
  op("PATCH", "/api/v1/admin/accounts/{id}", "updateAdminAccount", "Update an admin account's status and roles", "administration", "admin", true, ref("AdminAccount"), { requestBody: ref("AdminAccountUpdateRequest"), brandHeader: true, permissions: ["admin.write.brand", "admin.write.platform"], description: "Requires admin.write in the selected brand or platform scope. Username and brand scopes are immutable here. Self, super-admin, multi-brand targets without platform scope, and targets with undelegated roles cannot be changed. Revokes target sessions; success includes audit_log_id." }),
  op("POST", "/api/v1/admin/accounts/{id}/delete", "deleteAdminAccount", "Delete a brand account", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AccountDeleteRequest"), brandHeader: true, permissions: ["admin.delete.platform"], description: "Requires admin.delete.platform for total-management deletion. Permanently retires a non-super-admin account, clears its password, sessions, brand scopes and role assignments; no reason is required. Historical business records and actor references remain intact, and the username stays reserved. Deleted accounts are excluded from management and cannot be reactivated or reset. The selected brand must belong to the account." }),
  op("POST", "/api/v1/admin/accounts/{id}/reset-password", "resetAdminAccountPassword", "Reset an admin account password", "administration", "admin", true, ref("IdentityAuditResult"), { requestBody: ref("AdminAccountPasswordResetRequest"), brandHeader: true, permissions: ["admin.write.brand", "admin.write.platform"], description: "Requires admin.write in the selected brand or platform scope. Super-admin targets, self-reset, and unauthorized multi-brand targets are denied. Resets the password and revokes all target admin sessions." }),
  op("POST", "/api/v1/admin/users", "createUserForBrand", "Provision a new user and brand member", "administration", "admin", true, ref("AdminOperatorCreateResult"), { requestBody: ref("AdminOperatorCreateRequest"), successStatus: 201, brandHeader: true, permissions: ["user.create.brand"], description: "Requires user.create.brand and is denied to super-admins. Existing usernames or phones are not linked. At least one username or phone is required. No session is issued; success returns an audit log ID." }),
  op("GET", "/api/v1/admin/auth-settings", "getAdminAuthSettings", "Get brand authentication settings", "administration", "admin", false, ref("AdminAuthSettings"), { brandHeader: true, permissions: ["auth_config.view.platform"], description: "Platform-owned settings. Brand staff requests are denied; use the platform entry with auth_config.view.platform. Returns current configuration version and read-only policy versions; the read is audited." }),
  op("PATCH", "/api/v1/admin/auth-settings", "updateAdminAuthSettings", "Update brand authentication settings", "administration", "admin", true, ref("AdminAuthSettingsUpdated"), { requestBody: ref("AdminAuthSettingsRequest"), brandHeader: true, permissions: ["auth_config.write.platform"], description: "Platform-owned settings. Brand staff requests are denied; use the platform entry with auth_config.write.platform. Current permission and disabled state are rechecked even on replay. Updates CAPTCHA/Telegram public login configuration only; policy text and versions are read-only. Telegram enablement requires a valid client ID. Version conflicts return 409 CONFIG_VERSION_CONFLICT; success includes audit_log_id." }),
  op("GET", "/api/v1/admin/brand-operation", "getAdminBrandOperation", "Get the selected brand's operating status", "administration", "admin", false, ref("AdminBrandOperation"), { brandHeader: true, permissions: ["brand_operation.view.brand", "brand_operation.view.platform"], description: "Requires an explicit brand_operation.view.brand grant for the selected brand or brand_operation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. The selected brand is identified only by X-Brand-ID." }),
  op("PATCH", "/api/v1/admin/brand-operation", "updateAdminBrandOperation", "Change or schedule the selected brand operating status", "administration", "admin", true, ref("AdminBrandOperation"), { requestBody: ref("AdminBrandOperationUpdateRequest"), brandHeader: true, permissions: ["brand_operation.write.brand", "brand_operation.write.platform"], description: "Requires an explicit brand_operation.write.brand grant for the selected brand or brand_operation.write.platform grant. Any transition to or from disabled requires the platform grant. The reason is optional and may be empty (maximum 500 Unicode characters); no reason field is shown in the current management UI. Both pause timestamps must be supplied together only with status=paused; the end must be later than the start and database current time. Future windows require a currently active brand; disabled brands cannot receive timed pauses. Unchanged status without a current or requested window returns 409 BRAND_OPERATION_STATE_CONFLICT. Invalid decoded request bodies return 400 REQUEST_INVALID; service-level invalid input returns 400 BRAND_OPERATION_INPUT_INVALID. Optional pause_starts_at/pause_ends_at schedule a pause and automatic resume; future pauses return the current active status with the scheduled window. Manual changes replace or cancel the window. The period worker processes due windows every second. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Idempotent replay returns the original successful receipt (200), which may describe an older version after later writes; issue GET to retrieve current state. Version conflicts return 409 BRAND_OPERATION_VERSION_CONFLICT; brand operation shares the configuration version with brand and authentication configuration updates. The selected brand is identified only by X-Brand-ID." }),
  op("GET", "/api/v1/admin/brand-operation/history", "listAdminBrandOperationHistory", "List operating-status revisions for the selected brand", "administration", "admin", false, ref("AdminBrandOperationHistory"), { brandHeader: true, parameters: [{ name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 }, description: "Page size (default 20; maximum 100)." }, { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of revisions to skip (maximum 1000000)." }], permissions: ["brand_operation.view.brand", "brand_operation.view.platform"], description: "Requires an explicit brand_operation.view.brand grant for the selected brand or brand_operation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Returns newest revisions first for the brand selected by X-Brand-ID; there is no brand-ID path alias. Unknown or repeated query parameters are rejected." }),
  op("GET", "/api/v1/platform/brand-domains", "getAdminBrandDomains", "Get domains bound to the selected brand", "administration", "admin", false, ref("AdminBrandDomainRecord"), { brandHeader: true, permissions: ["brand_domains.view.platform"], description: "Requires an explicit brand_domains.view.platform grant. Super-admin status alone grants no access. X-Brand-ID is the only brand selector. Domains include disabled bindings and platform-reserved domains are not included." }),
  op("POST", "/api/v1/platform/brand-domains", "createPlatformBrandDomain", "Add a domain binding to the selected brand", "administration", "admin", true, ref("AdminBrandDomainReceipt"), { requestBody: ref("AdminBrandDomainCreateRequest"), successStatus: 201, brandHeader: true, permissions: ["brand_domains.write.platform"], description: "Requires an explicit brand_domains.write.platform grant. Super-admin status alone grants no access. The version shares brands.config_version. New domains must be canonical lower-case ASCII dotted DNS names (maximum 253 bytes, labels maximum 63 bytes); URLs, IP addresses, wildcard/local names, localhost, ports, and trailing dots are rejected. A binding cannot take a domain reserved by any brand (including disabled bindings) or the platform; a collision returns 409 BRAND_DOMAIN_CONFLICT. At most 100 bindings per brand, including disabled bindings. Existing bindings may be toggled or deleted, but never renamed or reassigned. No DNS/TLS provisioning or ownership verification is performed. Paused brands remain writable; disabled brands are read-only. Unknown write outcomes must be replayed with the exact frozen key and body; after a 409, reread state and abandon that intent rather than retrying with a new key automatically. Errors: BRAND_DOMAIN_INPUT_INVALID (400), BRAND_DOMAIN_NOT_FOUND (404), PERMISSION_DENIED (403), BRAND_DOMAIN_VERSION_CONFLICT, BRAND_DOMAIN_STATE_CONFLICT, or BRAND_DOMAIN_CONFLICT (409). Successful responses include audit_log_id." }),
  op("DELETE", "/api/v1/platform/brand-domains/{domainID}", "deletePlatformBrandDomain", "Delete a brand domain binding and release its hostname", "administration", "admin", true, ref("AdminBrandDomainReceipt"), { requestBody: ref("AdminBrandDomainDeleteRequest"), brandHeader: true, permissions: ["brand_domains.write.platform"], description: "Deletes only the selected brand binding using the current shared configuration version. The domain immediately stops resolving to that brand and can be added by any brand. Enabled bindings can also be deleted. Audit snapshots remain immutable. Idempotent replay returns the original receipt and rechecks current permissions and brand state. Disabled brands are read-only. Errors: BRAND_DOMAIN_INPUT_INVALID (400), BRAND_DOMAIN_NOT_FOUND (404), PERMISSION_DENIED (403), BRAND_DOMAIN_VERSION_CONFLICT or BRAND_DOMAIN_STATE_CONFLICT (409)." }),
  op("PATCH", "/api/v1/platform/brand-domains/{domainID}", "updatePlatformBrandDomain", "Change a domain binding's enabled state", "administration", "admin", true, ref("AdminBrandDomainReceipt"), { requestBody: ref("AdminBrandDomainUpdateRequest"), brandHeader: true, permissions: ["brand_domains.write.platform"], description: "Requires an explicit brand_domains.write.platform grant. Super-admin status alone grants no access. The version shares brands.config_version. Domains cannot be renamed or reassigned. Deletion releases a domain for rebinding. A no-op update returns 409 BRAND_DOMAIN_STATE_CONFLICT. No DNS/TLS provisioning or ownership verification is performed. Paused brands remain writable; disabled brands are read-only. Cached replay rechecks current permissions and brand state. Unknown write outcomes must be replayed with the exact frozen key and body; after a 409, reread state and abandon that intent rather than retrying with a new key automatically. Errors: BRAND_DOMAIN_INPUT_INVALID (400), BRAND_DOMAIN_NOT_FOUND (404), PERMISSION_DENIED (403), BRAND_DOMAIN_VERSION_CONFLICT, BRAND_DOMAIN_STATE_CONFLICT, or BRAND_DOMAIN_CONFLICT (409). Successful responses include audit_log_id." }),
  op("GET", "/api/v1/platform/brand-domains/history", "listAdminBrandDomainHistory", "List domain-binding revisions for the selected brand", "administration", "admin", false, ref("AdminBrandDomainHistory"), { brandHeader: true, parameters: [{ name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 }, description: "Page size (default 20; maximum 100)." }, { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of revisions to skip (maximum 1000000)." }], permissions: ["brand_domains.view.platform"], description: "Requires an explicit brand_domains.view.platform grant. Super-admin status alone grants no access. Returns newest revisions first for the brand selected only by X-Brand-ID. Before and after domain snapshots are sorted by domain then id. Versions may have gaps because config_version is shared. Unknown or repeated query parameters are rejected." }),
  op('GET','/api/v1/brand-assets/{file}','getBrandLogoAsset','Read an uploaded brand image','system','public',false,undefined,{brandAlias:false,successContent:{'image/png':{schema:{type:'string',format:'binary'}},'image/jpeg':{schema:{type:'string',format:'binary'}}},parameters:[{name:'file',in:'path',required:true,schema:{type:'string',pattern:'^[a-f0-9]{64}\\.(png|jpg)$'}}],description:'Immutable public brand logo bytes. The filename is a brand-scoped content hash. No administrative data is returned.'}),
  op('GET','/api/v1/admin/brand-presentation/defaults','getBrandPresentationDefaults','Read default presentation settings','administration','admin',false,ref('AdminBrandPresentationEffective'),{brandHeader:true,permissions:['brand_presentation.view.brand','brand_presentation.view.platform'],description:'Returns server-resolved defaults using the base brand name. Does not modify saved overrides.'}),
  op('POST','/api/v1/admin/brand-presentation/logo','uploadBrandLogo','Upload a brand logo','administration','admin',false,obj({url:{type:'string'}},['url']),{brandHeader:true,permissions:['brand_presentation.write.brand'],requestContent:{'image/png':{schema:{type:'string',format:'binary'}},'image/jpeg':{schema:{type:'string',format:'binary'}}},description:'Raw PNG or JPEG, at most 1 MiB and 4096 pixels per dimension. Validates the complete raster. Current brand write permission and enabled brand state are rechecked transactionally. The asset URL is stable for the same bytes within the same brand. Uploads do not publish presentation settings; save the returned URL through the versioned presentation update.'}),
  op("GET", "/api/v1/admin/brand-presentation", "getAdminBrandPresentation", "Get brand presentation configuration", "administration", "admin", false, ref("AdminBrandPresentationRecord"), { brandHeader: true, permissions: ["brand_presentation.view.brand", "brand_presentation.view.platform"], description: "Requires an explicit brand_presentation.view.brand grant for the selected brand or brand_presentation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Returns the stored nullable overrides and the fully resolved effective configuration for the brand selected only by X-Brand-ID. Disabled brands are read-only; no brand-ID path alias is accepted." }),
  op("PUT", "/api/v1/admin/brand-presentation", "updateAdminBrandPresentation", "Update brand presentation configuration", "administration", "admin", true, ref("AdminBrandPresentationReceipt"), { requestBody: ref("AdminBrandPresentationPutRequest"), brandHeader: true, permissions: ["brand_presentation.write.brand", "brand_presentation.write.platform"], description: "Requires an explicit brand_presentation.write.brand grant for the selected brand or brand_presentation.write.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. The request config is validated as a whole and must match version, which shares brands.config_version and may advance for other brand/auth changes; a successful update returns version before+1 and the matching submitted config. Version conflicts return 409 BRAND_PRESENTATION_VERSION_CONFLICT. A same-key cached replay returns the original receipt, which may be older than current state; issue GET after replay or an uncertain acknowledgment. Disabled brands are read-only. Authorization and the disabled-brand check apply on replay too." }),
  op("GET", "/api/v1/admin/brand-presentation/history", "listAdminBrandPresentationHistory", "List brand presentation revisions", "administration", "admin", false, ref("AdminBrandPresentationHistory"), { brandHeader: true, parameters: [{ name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 }, description: "Page size (default 20; maximum 100)." }, { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of revisions to skip (maximum 1000000)." }], permissions: ["brand_presentation.view.brand", "brand_presentation.view.platform"], description: "Requires an explicit brand_presentation.view.brand grant for the selected brand or brand_presentation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Returns newest presentation revisions first for the brand selected only by X-Brand-ID. Versions may have gaps because the version is shared with other brand/auth configuration. Unknown or repeated query parameters are rejected; no brand-ID path alias is accepted." }),
];

addHistoryReadHeaders(operations);

for (const source of operations.filter(op=>['/api/v1/admin/auth-settings'].includes(op.path))) {
 operations.push({...source,path:source.path.replace('/admin/','/platform/'),operationId:source.operationId+'Platform',brandHeader:false,parameters:[brandHeader(false)],description:'Platform administrator only, with the explicit platform permission. Omit X-Brand-ID to read or update platform defaults; provide it to read effective brand settings or change the brand override/inherit_platform choice. Existing brands keep explicit settings; new brands inherit platform defaults. CAPTCHA applies to user, brand administrator and independent agent password login. Version checks, permission rechecks and idempotent replay apply.'});
}
