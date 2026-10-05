import { ApiError, createApiClient, type ApiClientOptions } from "./api";

export interface AuthUser {
  id: string;
  username?: string;
  phone?: string;
  telegram_user_id?: string;
  status: string;
}

export interface AuthMember {
  id: string;
  brand_id: string;
  status: string;
  display_name: string;
  joined_at: string;
}

export interface AuthSession {
  access_token: string;
  token_type: "Bearer";
  expires_at: string;
  user: AuthUser;
  member: AuthMember;
}

export interface AuthProfile {
  user: AuthUser;
  member: AuthMember;
}

export interface AuthTerms {
  privacy_policy_version: string;
  service_terms_version: string;
}

export interface RegisterInput extends AuthTerms {
  username?: string;
  phone?: string;
  password: string;
  captcha_id?: string;
  captcha_answer?: string;
}

export interface LoginInput {
  identifier: string;
  password: string;
  privacy_policy_version?: string;
  service_terms_version?: string;
  captcha_id?: string;
  captcha_answer?: string;
}

export interface AuthChallenge {
  id: string;
  svg: string;
  expires_at: string;
}

export interface AuthFeatures {
  captcha_enabled: boolean;
  telegram_enabled: boolean;
  telegram_client_id?: string;
}

export interface TelegramChallenge {
  id: string;
  nonce: string;
  expires_at: string;
}

export interface TelegramLoginInput extends AuthTerms {
  id_token: string;
  challenge_id: string;
  nonce: string;
}

export interface ProfileInput {
  username?: string;
  phone?: string;
}

export function createAuthClient(options: ApiClientOptions = {}) {
  const api = createApiClient(options);
  const base = options.brandCode
    ? `/api/v1/b/${encodeURIComponent(options.brandCode)}`
    : "/api/v1";

  return {
    getAuthFeatures: async () => {
      const path = options.brandCode
        ? `/api/v1/b/${encodeURIComponent(options.brandCode)}/context`
        : "/api/v1/context";
      const context = await api.request<{ auth?: Partial<AuthFeatures> }>(path);
      return {
        captcha_enabled: context.auth?.captcha_enabled === true,
        telegram_enabled: context.auth?.telegram_enabled === true,
        telegram_client_id:
          typeof context.auth?.telegram_client_id === "string"
            ? context.auth.telegram_client_id
            : undefined,
      };
    },
    getChallenge: () => api.request<AuthChallenge>(`${base}/auth/challenge`),
    getTelegramChallenge: () =>
      api.request<TelegramChallenge>(`${base}/auth/telegram/challenge`),
    register: (input: RegisterInput) =>
      api.request<AuthSession>(`${base}/auth/register`, jsonRequest(input)),
    login: (input: LoginInput) =>
      api.request<AuthSession>(`${base}/auth/login`, jsonRequest(input)),
    logout: () => api.request<unknown>(`${base}/auth/logout`, jsonRequest({})),
    me: () => api.request<AuthProfile>(`${base}/me`),
    updateProfile: (input: ProfileInput) =>
      api.request<{ audit_log_id: string }>(
        `${base}/me/profile`,
        jsonRequest(input, "PATCH"),
      ),
    telegram: (input: TelegramLoginInput) =>
      api.request<AuthSession>(`${base}/auth/telegram`, jsonRequest(input)),
  };
}

function jsonRequest(body: unknown, method = "POST"): RequestInit {
  return {
    method,
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      "Idempotency-Key": crypto.randomUUID(),
    },
    body: JSON.stringify(body),
  };
}

export function isBrandJoinRequired(error: unknown): error is ApiError {
  return (
    error instanceof ApiError &&
    error.code?.toUpperCase() === "BRAND_JOIN_REQUIRED"
  );
}
