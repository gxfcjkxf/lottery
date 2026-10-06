import type { BrowserContext } from "@playwright/test";

// Worker-local memory only. These are genuine server-issued test cookies, not
// credentials or a bypass; never persist or log them. Auth-focused scenarios
// retain their own login/logout operations instead of using this helper.
type Cookie = Awaited<ReturnType<BrowserContext["cookies"]>>[number];
const sessions = new Map<string, Cookie[]>();
export function rememberAdminSession(username: string, cookies: Cookie[], origin = "http://localhost:5174") {
  sessions.set(`${origin}\n${username}`, cookies.filter(c=>c.name==="lottery_admin").map(c=>({...c})));
}
export async function restoreAdminSession(context: BrowserContext,username:string,brandId:string,origin = "http://localhost:5174"):Promise<boolean> {
  const cacheKey=`${origin}\n${username}`;
  const cookies=sessions.get(cacheKey);if(!cookies?.length)return false;
  await context.addCookies(cookies);
  const response=await context.request.get(`${origin}/api/v1/admin/me`,{headers:{"X-Brand-ID":brandId}});
  if(response.status()===200){const result=await response.json();if(result.success===true&&result.data?.account?.brand_ids?.includes(brandId))return true;}
  sessions.delete(cacheKey);await context.clearCookies({name:"lottery_admin"});return false;
}
