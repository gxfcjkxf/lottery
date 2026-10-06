import { test,expect } from "@playwright/test";
import { restoreAdminSession } from "./support/admin-session";

// UI-state simulation only. Real retry authorization, transactional audit and
// replay are verified by Go HTTP/PG tests; these mocked receipts prove no money.
test("notification retry UI retains unknown intent across remount (mocked delivery responses)",async({page,context},info)=>{
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME||!process.env.TEST_HARBOR_ADMIN_PASSWORD,"Provide isolated Harbor admin credentials");
  const brand="0199a000-0000-7000-8000-000000000002";
  if(!await restoreAdminSession(context,process.env.TEST_HARBOR_ADMIN_USERNAME!,brand)) {
    const response=await page.request.post("http://localhost:5174/api/v1/admin/auth/login",{headers:{Origin:"http://localhost:5174","X-Brand-ID":brand,"Idempotency-Key":crypto.randomUUID()},data:{identifier:process.env.TEST_HARBOR_ADMIN_USERNAME,password:process.env.TEST_HARBOR_ADMIN_PASSWORD}});
    expect(response.status(),await response.text()).toBe(200);
  }
  const event="0199a000-0000-7000-8000-000000000099";
  const at="2026-10-06T04:00:00Z";
  const initial={event_id:event,brand_id:brand,status:"failed",attempt_count:1,last_error:"INVALID_EVENT",next_attempt_at:at,sent_at:null};
  const receipt={...initial,status:"pending"};
  const live={...initial,status:"sent",attempt_count:2,last_error:null,sent_at:at};
  let attempted=false;
  const writes:Array<{body:string|null,key:string|undefined}>=[];
  await page.route("**/api/v1/admin/notification-deliveries?**",route=>route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({success:true,data:{items:[attempted?live:initial]}})}));
  await page.route(`**/api/v1/admin/notification-deliveries/${event}/retry`,async route=>{
    writes.push({body:route.request().postData(),key:route.request().headers()["idempotency-key"]});
    attempted=true;
    if(writes.length===1) await route.abort("failed");
    else await route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({success:true,data:receipt})});
  });
  const errors:string[]=[];page.on("pageerror",error=>errors.push(error.message));
  await page.goto("http://localhost:5174");
  await page.getByLabel("选择真实后台品牌",{exact:true}).selectOption(brand);
  await page.getByRole("button",{name:"通知",exact:true}).click();
  const panel=page.locator(".notification-deliveries");
  await expect(panel.locator(".status-failed")).toBeVisible();
  await panel.getByLabel(/^重试原因/).fill("mocked UI intent only; no external delivery");
  await panel.getByRole("button",{name:`核对重试事件 ${event}`,exact:true}).click();
  await panel.locator(".nd-confirm").getByRole("checkbox").check();
  await panel.getByRole("button",{name:"确认重试",exact:true}).click();
  await expect(panel.locator(".nd-pending")).toBeVisible();
  await panel.getByRole("button",{name:"只读刷新通知投递记录",exact:true}).click();
  await expect(panel.locator(".status-sent")).toBeVisible();
  await expect(panel.locator(".nd-pending")).toBeVisible();
  // Remount through real navigation, not a reload that would discard page memory.
  if(info.project.name==="mobile") await page.locator(".mobile-nav button").nth(0).click();
  else await page.locator(".side-nav").getByRole("button",{name:/工作台/}).click();
  await page.getByRole("button",{name:"通知",exact:true}).click();
  await expect(panel.locator(".nd-pending")).toBeVisible();
  await panel.getByRole("button",{name:"按原请求重试",exact:true}).click();
  await expect(panel.locator(".nd-pending")).toHaveCount(0);
  await expect(panel.locator(".status-sent")).toBeVisible();
  expect(writes).toHaveLength(2);expect(writes[0]).toEqual(writes[1]);
  expect(writes[0].key).toBeTruthy();
  expect(errors).toEqual([]);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await panel.screenshot({path:info.outputPath("notification-client-state-mock.png")});
});
