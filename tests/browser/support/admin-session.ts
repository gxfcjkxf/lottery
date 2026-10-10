import type { BrowserContext } from "@playwright/test";

// Worker-local memory only. These are genuine server-issued test cookies, not
// credentials or a bypass; never persist or log them. Auth-focused scenarios
// retain their own login/logout operations instead of using this helper.
type Cookie = Awaited<ReturnType<BrowserContext["cookies"]>>[number];
type Session = { cookies: Cookie[]; accountId: string };
const sessions = new Map<string, Session>();
export async function rememberAdminSession(context: BrowserContext, username: string, origin = "http://localhost:5174"): Promise<void> {
  const cacheKey = `${origin}\n${username}`;
  sessions.delete(cacheKey);
  const response = await context.request.get(`${origin}/api/v1/admin/me`);
  if (response.status() !== 200) throw new Error("Cannot save an unauthenticated administrator session");
  const result = await response.json();
  const accountId = result.data?.account?.id;
  if (result.success !== true || typeof accountId !== "string" || !accountId) {
    throw new Error("Cannot save an administrator session without a verified account ID");
  }
  const cookies = (await context.cookies(`${origin}/api/v1/admin/me`)).filter(c => c.name === "lottery_admin");
  if (!cookies.length) throw new Error("Cannot save an administrator session without its cookie");
  sessions.set(cacheKey, {
    cookies: cookies.map(c => ({ ...c })),
    accountId,
  });
}
export async function restoreAdminSession(context: BrowserContext, username: string, brandId: string, origin = "http://localhost:5174"): Promise<boolean> {
  const cacheKey = `${origin}\n${username}`;
  const session = sessions.get(cacheKey);
  if (!session?.cookies.length) return false;
  await context.addCookies(session.cookies);
  const response = await context.request.get(`${origin}/api/v1/admin/me`, { headers: { "X-Brand-ID": brandId } });
  if (response.status() === 200) {
    const result = await response.json();
    const account = result.data?.account;
    if (result.success === true && typeof account?.id === "string" && account.id.length > 0 &&
      account.brand_ids?.includes(brandId) && account.id === session.accountId) {
      return true;
    }
  }
  sessions.delete(cacheKey);
  await context.clearCookies({ name: "lottery_admin" });
  return false;
}
