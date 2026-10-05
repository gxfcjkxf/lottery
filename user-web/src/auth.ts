import { ApiError } from "../../shared/src/api";
import {
  isBrandJoinRequired,
  type AuthProfile,
  type AuthSession,
} from "../../shared/src/auth";

export { ApiError, isBrandJoinRequired };
export type { AuthProfile, AuthSession };

export function normalizeIdentifier(value: string): {
  username?: string;
  phone?: string;
  identifier: string;
} {
  const trimmed = value.trim();
  const phoneCandidate = trimmed.replace(/[\s().-]/g, "");
  if (/^\+[1-9]\d{6,14}$/.test(phoneCandidate)) {
    const phone = phoneCandidate;
    return { phone, identifier: phone };
  }
  const username = trimmed.toLowerCase();
  return { username, identifier: username };
}

export interface TelegramAuthResult {
  id_token?: string;
  error?: string;
}

declare global {
  interface Window {
    Telegram?: {
      Login?: {
        auth: (
          options: { client_id: number; nonce: string; scope: string[] },
          callback: (result: TelegramAuthResult) => void,
        ) => void;
      };
    };
  }
}

const telegramSdkUrl = "https://oauth.telegram.org/js/telegram-login.js";
let telegramSdkPromise: Promise<void> | undefined;

export function isTelegramClientIdConfigured(
  value: string | undefined,
): value is string {
  return Boolean(
    value && /^[1-9]\d*$/.test(value) && Number.isSafeInteger(Number(value)),
  );
}

export function loadTelegramLoginSdk(): Promise<void> {
  if (typeof window !== "undefined" && window.Telegram?.Login?.auth)
    return Promise.resolve();
  if (telegramSdkPromise) return telegramSdkPromise;

  telegramSdkPromise = new Promise<void>((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${telegramSdkUrl}"]`,
    );
    const script = existing ?? document.createElement("script");
    if (existing?.dataset.telegramLoginLoaded === "true") {
      reject(new Error("Telegram sign-in library did not initialize."));
      return;
    }
    const cleanup = () => {
      clearTimeout(timer);
      script.removeEventListener("load", onLoad);
      script.removeEventListener("error", onError);
    };
    const onLoad = () => {
      cleanup();
      if (window.Telegram?.Login?.auth) {
        script.dataset.telegramLoginLoaded = "true";
        resolve(undefined);
      } else reject(new Error("Telegram sign-in library did not initialize."));
    };
    const onError = () => {
      cleanup();
      script.remove();
      reject(new Error("Telegram sign-in library could not be loaded."));
    };
    const timer = setTimeout(() => {
      cleanup();
      script.remove();
      reject(new Error("Telegram sign-in library timed out. Please retry."));
    }, 10_000);
    script.addEventListener("load", onLoad, { once: true });
    script.addEventListener("error", onError, { once: true });
    if (!existing) {
      script.src = telegramSdkUrl;
      script.async = true;
      script.dataset.telegramLoginSdk = "true";
      document.head.append(script);
    }
  }).catch((error) => {
    telegramSdkPromise = undefined;
    throw error;
  });
  return telegramSdkPromise;
}

export function requestTelegramIdToken(
  clientId: string,
  nonce: string,
): Promise<string> {
  if (!isTelegramClientIdConfigured(clientId))
    return Promise.reject(new Error("Telegram client ID is not configured."));
  const numericClientId = Number(clientId);
  if (!window.Telegram?.Login?.auth)
    return Promise.reject(new Error("Telegram sign-in is not ready."));
  return new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error("Telegram sign-in timed out. Please retry.")),
      90_000,
    );
    try {
      window.Telegram!.Login!.auth(
        { client_id: numericClientId, nonce, scope: ["profile"] },
        (result) => {
          clearTimeout(timer);
          if (result.error) reject(new Error(result.error));
          else if (result.id_token) resolve(result.id_token);
          else reject(new Error("Telegram did not return an ID token."));
        },
      );
    } catch (error) {
      clearTimeout(timer);
      reject(error);
    }
  });
}

export function passwordByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function authErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401)
      return "The username, phone number, or password is incorrect.";
    if (error.status === 404)
      return "Authentication service is not available yet. Please try again later.";
    return error.message;
  }
  return error instanceof Error
    ? error.message
    : "Something went wrong. Please try again.";
}

export function discardAccessToken(session: AuthSession): AuthProfile {
  return { user: session.user, member: session.member };
}
