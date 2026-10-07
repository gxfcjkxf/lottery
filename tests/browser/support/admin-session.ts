import type { BrowserContext } from "@playwright/test";

// Worker-local memory only. These are genuine server-issued test cookies, not
// credentials or a bypass; never persist or log them. Auth-focused scenarios
// retain their own login/logout operations instead of using this helper.
type Cookie = Awaited<ReturnType<BrowserContext["cookies"]>>[number];
type Session = { cookies: Cookie[]; accountId?: string };
const sessions = new Map<string, Session>();
export function rememberAdminSession(username: string, cookies: Cookie[], origin = "http://localhost:5174", accountId?: string) {
  const cacheKey = `${origin}\n${username}`;
  sessions.set(cacheKey, {
    cookies: cookies.filter(c => c.name === "lottery_admin").map(c => ({ ...c })),
    accountId: accountId ?? sessions.get(cacheKey)?.accountId,
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
      account.brand_ids?.includes(brandId) && (!session.accountId || account.id === session.accountId)) {
      session.accountId = account.id;
      return true;
    }
  }
  sessions.delete(cacheKey);
  await context.clearCookies({ name: "lottery_admin" });
  return false;
}
