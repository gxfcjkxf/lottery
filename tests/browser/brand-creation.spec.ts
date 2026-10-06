import {test,expect,type Page} from "@playwright/test";

const origin="http://localhost:5174",api=`${origin}/api/v1/admin`;

async function goBrands(page:Page,mobile:boolean) {
 if(mobile){await page.locator(".mobile-nav button").last().click();await page.locator(".mobile-more-menu").getByRole("button",{name:/品牌和域名/}).click()}
 else await page.locator(".side-nav").getByRole("button",{name:/品牌和域名/}).click();
 await expect(page.getByRole("heading",{name:"品牌和域名",exact:true})).toBeVisible();
}

test("platform brand creation retains an uncertain committed request across navigation and replays once",async({page},info)=>{
 test.skip(!process.env.TEST_PLATFORM_ADMIN_USERNAME||!process.env.TEST_PLATFORM_ADMIN_PASSWORD,"Provide isolated platform administrator credentials");
 test.setTimeout(60000);page.setDefaultTimeout(10000);
 const mobile=info.project.name==="mobile";
 const login=await page.request.post(`${api}/auth/login`,{headers:{Origin:origin,"Idempotency-Key":crypto.randomUUID()},data:{identifier:process.env.TEST_PLATFORM_ADMIN_USERNAME,password:process.env.TEST_PLATFORM_ADMIN_PASSWORD}});
 expect(login.status(),"platform login").toBe(200);
 const me=await page.request.get(`${api}/me`);expect(me.status()).toBe(200);
 const account=(await me.json()).data.account;
 expect(account.platform_permissions).toContain("brand.create.platform");
 const before=await page.request.get(`${api}/brands`);expect(before.status()).toBe(200);
 if(process.env.LOTTERY_BROWSER_BRANDLESS==="yes") expect((await before.json()).data.items).toHaveLength(0);
 await page.goto(origin);await goBrands(page,mobile);
 const panel=page.locator(".brand-creation");await expect(panel.getByRole("heading",{name:"新建品牌",exact:true})).toBeVisible();
 const code=`brand_${crypto.randomUUID().replaceAll("-","").slice(0,12)}`;
 await panel.getByLabel(/^品牌代码/).fill(code);
 await panel.getByLabel(/^品牌名称/).fill("移动和桌面创建测试品牌");
 await panel.getByRole("combobox",{name:"默认语言",exact:true}).selectOption("zh-CN");
 await panel.getByLabel(/^时区/).fill("Asia/Manila");
 await panel.getByLabel(/^创建原因/).fill("Verify real audited platform creation");
 await panel.getByRole("button",{name:"核对并继续",exact:true}).click();
 const frozen:string[]=[],keys:string[]=[];let dropped=false;let receipt:{id:string;audit_log_id:string;code:string;status:string;version:number}|undefined;
 await page.route("**/api/v1/admin/brands",async route=>{
  if(route.request().method()!=="POST"){await route.continue();return}
  frozen.push(route.request().postData()??"");keys.push(route.request().headers()["idempotency-key"]??"");
  expect(route.request().headers()["x-brand-id"]).toBeUndefined();
  if(!dropped){dropped=true;const response=await route.fetch();expect(response.status()).toBe(201);receipt=(await response.json()).data;await route.abort("failed");return}
  await route.continue();
 });
 await panel.getByRole("button",{name:"确认创建品牌",exact:true}).click();
 // The UI freezes immediately, even before the server replies. Wait for the
 // independently captured real commit before testing a lost acknowledgment.
 await expect.poll(()=>Boolean(receipt)).toBe(true);
 await expect(panel.getByRole("alert")).toContainText("提交结果未知");
 await expect(panel.getByRole("button",{name:"返回修改",exact:true})).toHaveCount(0);
 if(mobile) await page.locator(".mobile-nav button").nth(1).click();
 else await page.locator(".side-nav").getByRole("button",{name:/用户和成员/}).click();
 await expect(panel).toHaveCount(0);await goBrands(page,mobile);
 await expect(panel.getByRole("alert")).toContainText("提交结果未知");
 await panel.getByRole("button",{name:"使用原请求重试",exact:true}).click();
 await expect(panel.locator(".brand-creation__success")).toContainText("已核验创建回执");
 expect(frozen).toHaveLength(2);expect(frozen[1]).toBe(frozen[0]);expect(keys[1]).toBe(keys[0]);
 expect(receipt).toMatchObject({code,status:"paused",version:1});expect(receipt?.audit_log_id).toMatch(/^[0-9a-f-]{36}$/);
 const brands=await page.request.get(`${api}/brands`);expect(brands.status()).toBe(200);
 expect((await brands.json()).data.items.filter((b:{code:string})=>b.code===code)).toHaveLength(1);
 const current=await page.request.get(`${api}/brand-operation`,{headers:{"X-Brand-ID":receipt!.id}});expect(current.status()).toBe(200);
 expect((await current.json()).data).toMatchObject({brand_id:receipt!.id,status:"paused",version:1});
 expect((await page.request.get(`${api}/me`).then(r=>r.json())).data.account.brand_ids).not.toContain(receipt!.id);
 await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
 if(process.env.LOTTERY_BROWSER_SCREENSHOT) await page.screenshot({path:process.env.LOTTERY_BROWSER_SCREENSHOT,fullPage:true});
});
