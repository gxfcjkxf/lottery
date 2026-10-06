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
const utf8String = (maxBytes, extra = {}) => string({ maxLength: maxBytes, "x-maxUtf8Bytes": maxBytes, description: `Maximum ${maxBytes} UTF-8 bytes.`, ...extra });
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
const brandHeader = (required = true) => ({
  name: "X-Brand-ID",
  in: "header",
  required,
  schema: ref("UUID"),
  description: required ? "Target brand; must be an authorized brand even with platform scope." : "Optional brand filter. Omit only when using the platform-scope permission to query across brands.",
});

export const schemas = {
  ComplianceGateRecord: obj({
    id:ref("UUID"),brand_id:ref("UUID"),policy_version:integer({minimum:1}),
    config:{allOf:[ref("ComplianceConfig"),{anyOf:[{properties:{age_enabled:{const:true}}},{properties:{region_enabled:{const:true}}},{properties:{identity_enabled:{const:true}}}]}]},
    operation:string({enum:["registration","betting"]}),action:string({enum:["register","join","operator_join","bet_preview","bet_place"]}),
    decision:string({const:"review"}),checks:array(ref("ComplianceCheck"),{minItems:3,maxItems:3}),adapter_mode:string({const:"stub"}),
    actor_type:string({enum:["anonymous","user","admin"]}),actor_id:nullable(ref("UUID")),member_id:nullable(ref("UUID")),
    request_id:string({minLength:1,maxLength:80}),audit_log_id:ref("UUID"),created_at:ref("DateTime")
  },["id","brand_id","policy_version","config","operation","action","decision","checks","adapter_mode","actor_type","actor_id","member_id","request_id","audit_log_id","created_at"],{
    oneOf:[
      {properties:{operation:{const:"registration"},action:{const:"register"},actor_type:{const:"anonymous"},actor_id:{type:"null"},member_id:{type:"null"}}},
      {properties:{operation:{const:"registration"},action:{const:"operator_join"},actor_type:{const:"admin"},actor_id:ref("UUID"),member_id:{type:"null"}}},
      {properties:{actor_type:{const:"user"},actor_id:ref("UUID")},oneOf:[
        {properties:{operation:{const:"registration"},action:{const:"join"}}},
        {properties:{operation:{const:"betting"},action:{enum:["bet_preview","bet_place"]},member_id:ref("UUID")}}
      ]}
    ],
    description:"Immutable actual business admission rejection, distinct from administrative simulations. Actor ids refer to existing identities only; no usernames, credentials, documents, raw keys or IP in DTO. At least one check enabled; current stub review prevents new business."
  }),
  ComplianceGatesPage: obj({brand_id:ref("UUID"),operation:nullable(string({enum:["registration","betting"]})),items:array(ref("ComplianceGateRecord")),limit:integer({minimum:1,maximum:100}),offset:integer({minimum:0,maximum:1000000}),total_count:string({pattern:"^(0|[1-9][0-9]*)$"})},["brand_id","operation","items","limit","offset","total_count"]),
  ComplianceConfig: obj({age_enabled:{type:"boolean"},minimum_age:nullable(integer({minimum:18,maximum:120})),region_enabled:{type:"boolean"},allowed_countries:array(string({pattern:"^[A-Z]{2}$"}),{maxItems:250,uniqueItems:true,description:"Strictly sorted canonical uppercase two-letter tokens; syntax is not a legal jurisdiction approval."}),identity_enabled:{type:"boolean"}},["age_enabled","minimum_age","region_enabled","allowed_countries","identity_enabled"],{allOf:[{if:{properties:{age_enabled:{const:true}},required:["age_enabled"]},then:{properties:{minimum_age:integer({minimum:18,maximum:120})}}},{if:{properties:{region_enabled:{const:true}},required:["region_enabled"]},then:{properties:{allowed_countries:{minItems:1}}}}],description:"All five fields required. Default all flags disabled, age null and countries empty. Enabled unconfigured checks reject new registration/brand admission and betting; disabled checks are skipped, not real verification. Existing login/reads/refunds remain available; withdrawal not implemented."}),
  CompliancePolicy: obj({brand_id:ref("UUID"),version:integer({minimum:1}),config:ref("ComplianceConfig"),updated_at:ref("DateTime"),audit_log_id:ref("UUID")},["brand_id","version","config","updated_at"]),
  CompliancePolicyInput: obj({version:integer({minimum:1}),config:ref("ComplianceConfig"),reason:utf8String(500,{minLength:1,description:"Nonempty UTF-8, no surrounding whitespace or control characters; maximum 500 bytes."})},["version","config","reason"]),
  ComplianceCheckInput: obj({version:integer({minimum:1}),operation:string({enum:["registration","betting","withdrawal"]}),reason:utf8String(500,{minLength:1})},["version","operation","reason"],{description:"Explicit administrative stub check against current policy version. Rejects personal data, subjects, arbitrary adapter decisions and omitted fields."}),
  ComplianceCheck: obj({check:string({enum:["age","region","identity"]}),enabled:{type:"boolean"},decision:string({enum:["allow","review","deny","freeze"]}),reason_code:string({enum:["CHECK_DISABLED","ADAPTER_NOT_CONFIGURED"]})},["check","enabled","decision","reason_code"],{description:"Current stub returns allow/CHECK_DISABLED if disabled, review/ADAPTER_NOT_CONFIGURED if enabled. deny/freeze reserved, never money/member mutation."}),
  ComplianceDecision: obj({id:ref("UUID"),brand_id:ref("UUID"),policy_version:integer({minimum:1}),config:ref("ComplianceConfig"),operation:string({enum:["registration","betting","withdrawal"]}),decision:string({enum:["allow","review","deny","freeze"]}),checks:array(ref("ComplianceCheck"),{minItems:3,maxItems:3}),adapter_mode:string({const:"stub"}),created_by:ref("UUID"),reason:utf8String(500,{minLength:1}),audit_log_id:ref("UUID"),created_at:ref("DateTime")},["id","brand_id","policy_version","config","operation","decision","checks","adapter_mode","created_by","reason","audit_log_id","created_at"],{description:"Immutable explicit check snapshot, not user verification, review queue, live enforcement, wallet freeze or legal approval. Check order age/region/identity."}),
  ComplianceRevision: obj({id:ref("UUID"),brand_id:ref("UUID"),version:integer({minimum:1}),config:ref("ComplianceConfig"),changed_by:nullable(ref("UUID")),reason:string(),audit_log_id:nullable(ref("UUID")),created_at:ref("DateTime")},["id","brand_id","version","config","changed_by","reason","audit_log_id","created_at"]),
  ComplianceHistoryPage: obj({brand_id:ref("UUID"),items:array(ref("ComplianceRevision")),limit:integer({minimum:1,maximum:100}),offset:integer({minimum:0,maximum:1000000}),total_count:string({pattern:"^(0|[1-9][0-9]*)$"})},["brand_id","items","limit","offset","total_count"]),
  ComplianceDecisionPage: obj({brand_id:ref("UUID"),operation:nullable(string({enum:["registration","betting","withdrawal"]})),items:array(ref("ComplianceDecision")),limit:integer({minimum:1,maximum:100}),offset:integer({minimum:0,maximum:1000000}),total_count:string({pattern:"^(0|[1-9][0-9]*)$"})},["brand_id","operation","items","limit","offset","total_count"]),
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
    theme: ref("AdminBrandPresentationEffective"), config_version: integer({ minimum: 1 }),
  }, ["id", "code", "name", "status", "default_locale", "timezone", "theme", "config_version"]),
  IdentityContext: obj({
    brand: ref("IdentityContextBrand"),
    available_locales: array(string({ enum: ["en", "zh-CN"] }), { minItems: 1, uniqueItems: true }),
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
  AdminBrandCreationInput: obj({
    code: string({pattern:"^[a-z][a-z0-9_]{0,47}$",minLength:1,maxLength:48}),
    name: utf8String(120,{description:"Nonempty, no surrounding whitespace or Unicode control characters; maximum 120 UTF-8 bytes."}),
    default_locale: string({enum:["en","zh-CN"]}),
    timezone: utf8String(80,{description:"Valid named IANA timezone or UTC; no empty/Local/surrounding whitespace/control characters."}),
    reason: utf8String(500,{description:"Nonempty, no surrounding whitespace or Unicode control characters; maximum 500 UTF-8 bytes."}),
  },["code","name","default_locale","timezone","reason"],{description:"All five keys are required. Unknown, duplicate and null fields rejected. No input status, brand id, initial money or administrator fields."}),
  AdminBrandCreationReceipt: obj({
    id:ref("UUID"),code:string(),name:string(),status:string({const:"paused"}),default_locale:string({enum:["en","zh-CN"]}),timezone:string(),version:integer({const:1}),created_at:ref("DateTime"),audit_log_id:ref("UUID"),
  },["id","code","name","status","default_locale","timezone","version","created_at","audit_log_id"],{description:"Immutable creation snapshot, not current status/version after subsequent configuration or resume. No auto account/scope/domain/game/fund creation."}),
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
  AdminBrandDomain: obj({ id: ref("UUID"), domain: string({ minLength: 1, maxLength: 253, description: "Stored hostname. Historical localhost/IP bindings may be returned and toggled, but cannot be renamed, reassigned, or deleted." }), enabled: { type: "boolean" }, is_primary: { type: "boolean" } }, ["id", "domain", "enabled", "is_primary"]),
  AdminBrandDomainRecord: obj({ brand_id: ref("UUID"), version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused", "disabled"] }), domains: array(ref("AdminBrandDomain"), { maxItems: 100, description: "Includes enabled and disabled bindings, sorted by domain then id. There may be zero or one primary domain." }), updated_at: ref("DateTime"), audit_log_id: ref("UUID") }, ["brand_id", "version", "status", "domains", "updated_at"]),
  AdminBrandDomainReceipt: obj({ brand_id: ref("UUID"), version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused", "disabled"] }), domains: array(ref("AdminBrandDomain"), { maxItems: 100, description: "Includes enabled and disabled bindings, sorted by domain then id. There may be zero or one primary domain." }), updated_at: ref("DateTime"), audit_log_id: ref("UUID") }, ["brand_id", "version", "status", "domains", "updated_at", "audit_log_id"]),
  AdminBrandDomainCreateRequest: obj({ version: integer({ minimum: 1 }), domain: brandDomainName, enabled: { type: "boolean" }, is_primary: { type: "boolean" }, reason: ref("Reason") }, ["version", "domain", "enabled", "is_primary", "reason"]),
  AdminBrandDomainUpdateRequest: obj({ version: integer({ minimum: 1 }), enabled: { type: "boolean" }, is_primary: { type: "boolean" }, reason: ref("Reason") }, ["version", "enabled", "is_primary", "reason"]),
  AdminBrandDomainRevision: obj({ id: ref("UUID"), brand_id: ref("UUID"), version: integer({ minimum: 1 }), changed_by: ref("UUID"), reason: ref("Reason"), audit_log_id: ref("UUID"), created_at: ref("DateTime"), before_domains: array(ref("AdminBrandDomain"), { maxItems: 100, description: "Audited pre-change snapshot, sorted by domain then id." }), domains: array(ref("AdminBrandDomain"), { maxItems: 100, description: "Audited post-change snapshot, sorted by domain then id; there may be zero or one primary domain." }) }, ["id", "brand_id", "version", "changed_by", "reason", "audit_log_id", "created_at", "before_domains", "domains"]),
  AdminBrandDomainHistory: obj({ items: array(ref("AdminBrandDomainRevision")), limit: integer({ minimum: 1, maximum: 100 }), offset: integer({ minimum: 0, maximum: 1000000 }) }, ["items", "limit", "offset"]),
  AdminBrandPresentationLocaleText: obj({ tagline: nullable(utf8String(160)), announcement: nullable(utf8String(2000, { description: "Maximum 2000 UTF-8 bytes; rendered as plain text, never as markup." })) }, ["tagline", "announcement"]),
  AdminBrandPresentationContent: obj({ en: ref("AdminBrandPresentationLocaleText"), "zh-CN": ref("AdminBrandPresentationLocaleText") }, ["en", "zh-CN"]),
  AdminBrandPresentationConfig: obj({
    display_name: nullable(utf8String(80)),
    logo_text: nullable(utf8String(32)),
    logo_url: nullable(utf8String(512, { description: "Maximum 512 UTF-8 bytes. Allowed assets end in .png, .jpg, .jpeg, .webp, or .ico; SVG is not accepted. Use either HTTPS with a syntactically valid DNS hostname (not an IP address, localhost, or a local hostname), with no port, user info, query, or fragment, or a /icons/ or /brand-assets/ relative path without query or fragment. The server validates hostname syntax but does not resolve DNS or guarantee a public IP. Values may not have surrounding whitespace, backslashes, percent signs, CR/LF/tab, or a '..' sequence." })),
    favicon_url: nullable(utf8String(512, { description: "Maximum 512 UTF-8 bytes. Allowed assets end in .png, .jpg, .jpeg, .webp, or .ico; SVG is not accepted. Use either HTTPS with a syntactically valid DNS hostname (not an IP address, localhost, or a local hostname), with no port, user info, query, or fragment, or a /icons/ or /brand-assets/ relative path without query or fragment. The server validates hostname syntax but does not resolve DNS or guarantee a public IP. Values may not have surrounding whitespace, backslashes, percent signs, CR/LF/tab, or a '..' sequence." })),
    primary_color: nullable(string({ pattern: "^#[A-Fa-f0-9]{6}$" })),
    accent_color: nullable(string({ pattern: "^#[A-Fa-f0-9]{6}$" })),
    success_color: nullable(string({ pattern: "^#[A-Fa-f0-9]{6}$" })),
    warning_color: nullable(string({ pattern: "^#[A-Fa-f0-9]{6}$" })),
    danger_color: nullable(string({ pattern: "^#[A-Fa-f0-9]{6}$" })),
    font_family: nullable(string({ enum: ["system", "serif", "mono"] })),
    font_scale: nullable(string({ enum: ["compact", "standard", "large"] })),
    radius: nullable(string({ enum: ["square", "soft", "round"] })),
    shadow: nullable(string({ enum: ["none", "subtle", "lifted"] })),
    default_locale: nullable(string({ enum: ["en", "zh-CN"] })),
    available_locales: nullable(array(string({ enum: ["en", "zh-CN"] }), { minItems: 1, uniqueItems: true })),
    content: nullable(ref("AdminBrandPresentationContent")),
  }, ["display_name", "logo_text", "logo_url", "favicon_url", "primary_color", "accent_color", "success_color", "warning_color", "danger_color", "font_family", "font_scale", "radius", "shadow", "default_locale", "available_locales", "content"], { description: "Every key is required. Null means inherit the brand's base value. The server rejects unknown keys, malformed URLs, invalid enum values, and strings exceeding their stated UTF-8 byte limits." }),
  AdminBrandPresentationEffective: obj({
    display_name: string(), logo_text: string(), logo_url: nullable(string()), favicon_url: nullable(string()),
    primary_color: string({ pattern: "^#[A-Fa-f0-9]{6}$" }), accent_color: string({ pattern: "^#[A-Fa-f0-9]{6}$" }),
    success_color: string({ pattern: "^#[A-Fa-f0-9]{6}$" }), warning_color: string({ pattern: "^#[A-Fa-f0-9]{6}$" }), danger_color: string({ pattern: "^#[A-Fa-f0-9]{6}$" }),
    font_family: string({ enum: ["system", "serif", "mono"] }), font_scale: string({ enum: ["compact", "standard", "large"] }),
    radius: string({ enum: ["square", "soft", "round"] }), shadow: string({ enum: ["none", "subtle", "lifted"] }),
    default_locale: string({ enum: ["en", "zh-CN"] }), available_locales: array(string({ enum: ["en", "zh-CN"] }), { minItems: 1, uniqueItems: true }),
    content: obj({ en: obj({ tagline: string(), announcement: string() }, ["tagline", "announcement"]), "zh-CN": obj({ tagline: string(), announcement: string() }, ["tagline", "announcement"]) }, ["en", "zh-CN"]),
  }, ["display_name", "logo_text", "logo_url", "favicon_url", "primary_color", "accent_color", "success_color", "warning_color", "danger_color", "font_family", "font_scale", "radius", "shadow", "default_locale", "available_locales", "content"], { description: "Fully resolved values: all fields except logo_url and favicon_url are non-null. Content always contains both supported locales with non-null tagline and announcement strings." }),
  AdminBrandPresentationRecord: obj({
    brand_id: ref("UUID"), version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused", "disabled"] }), base_name: string(),
    config: ref("AdminBrandPresentationConfig"), effective: ref("AdminBrandPresentationEffective"), updated_at: ref("DateTime"), audit_log_id: ref("UUID"),
  }, ["brand_id", "version", "status", "base_name", "config", "effective", "updated_at"]),
  AdminBrandPresentationReceipt: obj({
    brand_id: ref("UUID"), version: integer({ minimum: 1 }), status: string({ enum: ["active", "paused", "disabled"] }), base_name: string(),
    config: ref("AdminBrandPresentationConfig"), effective: ref("AdminBrandPresentationEffective"), updated_at: ref("DateTime"), audit_log_id: ref("UUID"),
  }, ["brand_id", "version", "status", "base_name", "config", "effective", "updated_at", "audit_log_id"]),
  AdminBrandPresentationPutRequest: obj({ version: integer({ minimum: 1 }), config: ref("AdminBrandPresentationConfig"), reason: ref("Reason") }, ["version", "config", "reason"]),
  AdminBrandPresentationRevision: obj({
    id: ref("UUID"), brand_id: ref("UUID"), version: integer({ minimum: 1 }), config: ref("AdminBrandPresentationConfig"), effective: ref("AdminBrandPresentationEffective"),
    changed_by: ref("UUID"), reason: ref("Reason"), audit_log_id: ref("UUID"), created_at: ref("DateTime"),
  }, ["id", "brand_id", "version", "config", "effective", "changed_by", "reason", "audit_log_id", "created_at"]),
  AdminBrandPresentationHistory: obj({ items: array(ref("AdminBrandPresentationRevision")), limit: integer({ minimum: 1, maximum: 100 }), offset: integer({ minimum: 0, maximum: 1000000 }) }, ["items", "limit", "offset"]),
};

const op = (method, path, operationId, summary, tag, auth, idempotency, data, extra = {}) => ({
  method, path, operationId, summary, tag, auth, idempotency, data,
  successStatus: 200, parameters: [], permissions: [], brandHeader: false, description: "",
  ...extra,
});

export const operations = [
  {method:"GET",path:"/api/v1/admin/compliance-gates",operationId:"getComplianceAdmissionRejections",summary:"List actual compliance admission rejections",tag:"administration",auth:"admin",brandHeader:true,idempotency:false,permissions:["compliance_check.view.brand","compliance_check.view.platform"],data:ref("ComplianceGatesPage"),successStatus:200,parameters:[{name:"limit",in:"query",required:false,schema:integer({minimum:1,maximum:100,default:20})},{name:"offset",in:"query",required:false,schema:integer({minimum:0,maximum:1000000,default:0})},{name:"operation",in:"query",required:false,schema:string({enum:["registration","betting"]})}],description:"Brand-scoped audited readonly immutable actual registration/join/operator-provisioning and betting admission rejections. Strict paging/filter, repeatable-read count/rows. Not a manual review queue or real verification/freeze. Evidence and encrypted negative idempotency receipt commit after business rollback; cached replays create no new evidence. Previews are separate non-idempotent observations."},
  ...[
    ["GET","/compliance-policy","getCompliancePolicy","compliance_policy.view.brand","CompliancePolicy",null],
    ["PUT","/compliance-policy","updateCompliancePolicy","compliance_policy.write.brand","CompliancePolicy","CompliancePolicyInput"],
    ["GET","/compliance-policy/history","getCompliancePolicyHistory","compliance_policy.view.brand","ComplianceHistoryPage",null],
    ["POST","/compliance-checks","runComplianceStubCheck","compliance_check.run.brand","ComplianceDecision","ComplianceCheckInput"],
    ["GET","/compliance-checks","getComplianceStubChecks","compliance_check.view.brand","ComplianceDecisionPage",null],
  ].map(([method,path,operationId,permission,data,body])=>({method,path:`/api/v1/admin${path}`,operationId,summary:operationId,tag:"administration",auth:"admin",brandHeader:true,idempotency:method!=="GET",permissions:permission.endsWith("view.brand")?[permission,permission.replace(".brand",".platform")]:[permission],data:ref(data),...(body?{requestBody:ref(body)}:{}),successStatus:method==="POST"?201:200,parameters:((path.endsWith("/history")||method==="GET"&&path==="/compliance-checks")?[{name:"limit",in:"query",required:false,schema:integer({minimum:1,maximum:100,default:20})},{name:"offset",in:"query",required:false,schema:integer({minimum:0,maximum:1000000,default:0})}]:[]).concat(method==="GET"&&path==="/compliance-checks"?[{name:"operation",in:"query",required:false,schema:string({enum:["registration","betting","withdrawal"]})}]:[]),description:"Brand-scoped compliance configuration and explicit administrative stub checks only. Separate explicit administrative simulations from actual registration/join/betting rejection gates. No real verification or withdrawal flow. Independent policy version, immutable audited history and decisions, checked encrypted idempotency. Super administrators may read only with explicit platform grants; writes and runs denied. Disabled brands readonly. All-disabled means checks skipped, not a verified user; enabled means review/ADAPTER_NOT_CONFIGURED."})),
  {method:"POST",path:"/api/v1/admin/brands",operationId:"createBrand",summary:"Create an initially paused brand",tag:"administration",auth:"admin",idempotency:true,brandHeader:false,permissions:["brand.create.platform"],requestBody:ref("AdminBrandCreationInput"),data:ref("AdminBrandCreationReceipt"),successStatus:201,description:"Global checked mutation independent of a selected brand. X-Brand-ID and query parameters rejected. Explicit platform permission, not super flag or brand grant alone. Code uniqueness, seven initial policy/presentation rows, immutable audited creation and encrypted platform idempotency are atomic. Existing configured platform entry permits administration even without an active brand. Replays recheck session/permission and return the same creation snapshot; no automatic scope grants, admins, domains, money or settlement enablement."},
  op("GET", "/health/live", "healthLive", "Check process liveness", "system", "public", false, ref("IdentityHealth"), { description: "Returns alive while the HTTP process is serving." }),
  op("GET", "/health/ready", "healthReady", "Check service readiness", "system", "public", false, ref("IdentityHealth"), { description: "Checks the configured readiness dependency with a two-second timeout; unavailable readiness returns 503." }),
  op("GET", "/api/v1/context", "getPublicContext", "Get the current brand and public configuration", "identity", "public", false, ref("IdentityContext"), { description: "Resolves the brand from the request host or generated platform brand path. Includes current authentication switches and policy versions; no secrets are returned." }),

  op("POST", "/api/v1/auth/register", "registerIdentity", "Register a user and join the current brand", "identity", "public", true, ref("IdentityAuthentication"), { requestBody: ref("IdentityRegisterRequest"), successStatus: 201, description: "Creates a new global identity and brand member only after compliance admission. Enabled unconfigured checks return COMPLIANCE_REVIEW_REQUIRED without identity/member/session writes. Accepts only current policy versions and issues a user session. Existing identities are not reused; attribution codes are optional and mutually exclusive." }),
  op("POST", "/api/v1/auth/login", "loginIdentity", "Log in with username or phone", "identity", "public", true, ref("IdentityAuthentication"), { requestBody: ref("IdentityLoginRequest"), description: "Authenticates the global account for the resolved brand. Joining a new brand or accepting a provisioned pending member also passes current compliance admission; enabled unconfigured checks reject the new admission. Existing accepted member login is not blocked." }),
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
  op("POST", "/api/v1/admin/users", "createUserForBrand", "Provision a new user and pending brand member", "administration", "admin", true, ref("AdminOperatorCreateResult"), { requestBody: ref("AdminOperatorCreateRequest"), successStatus: 201, brandHeader: true, permissions: ["user.create.brand"], description: "Requires user.create.brand and is denied to super-admins. Compliance admission prevents operator provisioning from bypassing enabled unconfigured checks. Rejected provisioning rolls back even a new global identity and preserves scoped evidence. Existing usernames or phones are not linked. At least one username or phone is required. No session is issued; terms_accepted is false and success returns an audit log ID." }),
  op("GET", "/api/v1/admin/auth-settings", "getAdminAuthSettings", "Get brand authentication settings", "administration", "admin", false, ref("AdminAuthSettings"), { brandHeader: true, permissions: ["auth_config.view.brand", "auth_config.view.platform"], description: "Requires auth_config.view in the selected brand or platform scope. Returns current configuration version and read-only policy versions; the read is audited." }),
  op("PATCH", "/api/v1/admin/auth-settings", "updateAdminAuthSettings", "Update brand authentication settings", "administration", "admin", true, ref("AdminAuthSettingsUpdated"), { requestBody: ref("AdminAuthSettingsRequest"), brandHeader: true, permissions: ["auth_config.write.brand", "auth_config.write.platform"], description: "Requires auth_config.write in the selected brand or platform scope. Updates CAPTCHA/Telegram public login configuration only; policy text and versions are read-only. Telegram enablement requires a valid client ID. Version conflicts return 409 CONFIG_VERSION_CONFLICT; success includes audit_log_id." }),
  op("GET", "/api/v1/admin/brand-operation", "getAdminBrandOperation", "Get the selected brand's operating status", "administration", "admin", false, ref("AdminBrandOperation"), { brandHeader: true, permissions: ["brand_operation.view.brand", "brand_operation.view.platform"], description: "Requires an explicit brand_operation.view.brand grant for the selected brand or brand_operation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. The selected brand is identified only by X-Brand-ID." }),
  op("PATCH", "/api/v1/admin/brand-operation", "updateAdminBrandOperation", "Pause or resume the selected brand", "administration", "admin", true, ref("AdminBrandOperation"), { requestBody: ref("AdminBrandOperationUpdateRequest"), brandHeader: true, permissions: ["brand_operation.write.brand", "brand_operation.write.platform"], description: "Requires an explicit brand_operation.write.brand grant for the selected brand or brand_operation.write.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Idempotent replay returns the original successful receipt (200), which may describe an older version after later writes; issue GET to retrieve current state. Version conflicts return 409 CONFIG_VERSION_CONFLICT; brand operation shares the configuration version with brand and authentication configuration updates. The selected brand is identified only by X-Brand-ID." }),
  op("GET", "/api/v1/admin/brand-operation/history", "listAdminBrandOperationHistory", "List operating-status revisions for the selected brand", "administration", "admin", false, ref("AdminBrandOperationHistory"), { brandHeader: true, parameters: [{ name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 }, description: "Page size (default 20; maximum 100)." }, { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of revisions to skip (maximum 1000000)." }], permissions: ["brand_operation.view.brand", "brand_operation.view.platform"], description: "Requires an explicit brand_operation.view.brand grant for the selected brand or brand_operation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Returns newest revisions first for the brand selected by X-Brand-ID; there is no brand-ID path alias. Unknown or repeated query parameters are rejected." }),
  op("GET", "/api/v1/admin/brand-domains", "getAdminBrandDomains", "Get domains bound to the selected brand", "administration", "admin", false, ref("AdminBrandDomainRecord"), { brandHeader: true, permissions: ["brand_domains.view.brand", "brand_domains.view.platform"], description: "Requires an explicit brand_domains.view.brand grant for the selected brand or brand_domains.view.platform grant. Super-admin status alone grants no access. X-Brand-ID is the only brand selector. Domains include disabled bindings and platform-reserved domains are not included." }),
  op("POST", "/api/v1/admin/brand-domains", "createAdminBrandDomain", "Add a domain binding to the selected brand", "administration", "admin", true, ref("AdminBrandDomainReceipt"), { requestBody: ref("AdminBrandDomainCreateRequest"), successStatus: 201, brandHeader: true, permissions: ["brand_domains.write.brand", "brand_domains.write.platform"], description: "Requires an explicit brand_domains.write.brand grant for the selected brand or brand_domains.write.platform grant. Super-admin status alone grants no access. The version shares brands.config_version. New domains must be canonical lower-case ASCII dotted DNS names (maximum 253 bytes, labels maximum 63 bytes); URLs, IP addresses, wildcard/local names, localhost, ports, and trailing dots are rejected. A binding cannot take a domain reserved by any brand (including disabled bindings) or the platform; a collision returns 409 BRAND_DOMAIN_CONFLICT. At most 100 bindings per brand, including disabled bindings. A primary domain must be enabled; setting one demotes the previous primary in the same transaction. Legacy localhost/IP bindings may be toggled but never renamed, reassigned, or deleted. No DNS/TLS provisioning or ownership verification is performed. Paused brands remain writable; disabled brands are read-only. Unknown write outcomes must be replayed with the exact frozen key and body; after a 409, reread state and abandon that intent rather than retrying with a new key automatically. Errors: BRAND_DOMAIN_INPUT_INVALID (400), BRAND_DOMAIN_NOT_FOUND (404), PERMISSION_DENIED (403), BRAND_DOMAIN_VERSION_CONFLICT, BRAND_DOMAIN_STATE_CONFLICT, or BRAND_DOMAIN_CONFLICT (409). Successful responses include audit_log_id." }),
  op("PATCH", "/api/v1/admin/brand-domains/{domainID}", "updateAdminBrandDomain", "Change a domain binding's enabled or primary state", "administration", "admin", true, ref("AdminBrandDomainReceipt"), { requestBody: ref("AdminBrandDomainUpdateRequest"), brandHeader: true, permissions: ["brand_domains.write.brand", "brand_domains.write.platform"], description: "Requires an explicit brand_domains.write.brand grant for the selected brand or brand_domains.write.platform grant. Super-admin status alone grants no access. The version shares brands.config_version. Domains cannot be renamed, reassigned, or deleted. A primary domain must be enabled; selecting it demotes the previous primary in the same transaction, and zero or one primary domain is allowed. A no-op update returns 409 BRAND_DOMAIN_STATE_CONFLICT. No DNS/TLS provisioning or ownership verification is performed. Paused brands remain writable; disabled brands are read-only. Cached replay rechecks current permissions and brand state. Unknown write outcomes must be replayed with the exact frozen key and body; after a 409, reread state and abandon that intent rather than retrying with a new key automatically. Errors: BRAND_DOMAIN_INPUT_INVALID (400), BRAND_DOMAIN_NOT_FOUND (404), PERMISSION_DENIED (403), BRAND_DOMAIN_VERSION_CONFLICT, BRAND_DOMAIN_STATE_CONFLICT, or BRAND_DOMAIN_CONFLICT (409). Successful responses include audit_log_id." }),
  op("GET", "/api/v1/admin/brand-domains/history", "listAdminBrandDomainHistory", "List domain-binding revisions for the selected brand", "administration", "admin", false, ref("AdminBrandDomainHistory"), { brandHeader: true, parameters: [{ name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 }, description: "Page size (default 20; maximum 100)." }, { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of revisions to skip (maximum 1000000)." }], permissions: ["brand_domains.view.brand", "brand_domains.view.platform"], description: "Requires an explicit brand_domains.view.brand grant for the selected brand or brand_domains.view.platform grant. Super-admin status alone grants no access. Returns newest revisions first for the brand selected only by X-Brand-ID. Before and after domain snapshots are sorted by domain then id; each may have zero or one primary. Versions may have gaps because config_version is shared. Unknown or repeated query parameters are rejected." }),
  op("GET", "/api/v1/admin/brand-presentation", "getAdminBrandPresentation", "Get brand presentation configuration", "administration", "admin", false, ref("AdminBrandPresentationRecord"), { brandHeader: true, permissions: ["brand_presentation.view.brand", "brand_presentation.view.platform"], description: "Requires an explicit brand_presentation.view.brand grant for the selected brand or brand_presentation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Returns the stored nullable overrides and the fully resolved effective configuration for the brand selected only by X-Brand-ID. Disabled brands are read-only; no brand-ID path alias is accepted." }),
  op("PUT", "/api/v1/admin/brand-presentation", "updateAdminBrandPresentation", "Update brand presentation configuration", "administration", "admin", true, ref("AdminBrandPresentationReceipt"), { requestBody: ref("AdminBrandPresentationPutRequest"), brandHeader: true, permissions: ["brand_presentation.write.brand", "brand_presentation.write.platform"], description: "Requires an explicit brand_presentation.write.brand grant for the selected brand or brand_presentation.write.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. The request config is validated as a whole and must match version, which shares brands.config_version and may advance for other brand/auth changes; a successful update returns version before+1 and the matching submitted config. Version conflicts return 409 BRAND_PRESENTATION_VERSION_CONFLICT. A same-key cached replay returns the original receipt, which may be older than current state; issue GET after replay or an uncertain acknowledgment. Disabled brands are read-only. Authorization and the disabled-brand check apply on replay too." }),
  op("GET", "/api/v1/admin/brand-presentation/history", "listAdminBrandPresentationHistory", "List brand presentation revisions", "administration", "admin", false, ref("AdminBrandPresentationHistory"), { brandHeader: true, parameters: [{ name: "limit", in: "query", required: false, schema: { type: "integer", minimum: 1, maximum: 100, default: 20 }, description: "Page size (default 20; maximum 100)." }, { name: "offset", in: "query", required: false, schema: { type: "integer", minimum: 0, maximum: 1000000, default: 0 }, description: "Number of revisions to skip (maximum 1000000)." }], permissions: ["brand_presentation.view.brand", "brand_presentation.view.platform"], description: "Requires an explicit brand_presentation.view.brand grant for the selected brand or brand_presentation.view.platform grant. Super-admin status alone grants no access; a super-admin with the exact required grant is authorized. Returns newest presentation revisions first for the brand selected only by X-Brand-ID. Versions may have gaps because the version is shared with other brand/auth configuration. Unknown or repeated query parameters are rejected; no brand-ID path alias is accepted." }),
];
