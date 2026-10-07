import {test,expect,type Page} from "@playwright/test";

const origin=process.env.TEST_ADMIN_ORIGIN??"http://localhost:5174",api=`${origin}/api/v1/admin`;
async function visit(page:Page,mobile:boolean){if(mobile){await page.locator(".mobile-nav button").last().click();await page.locator(".mobile-more-menu").getByRole("button",{name:/风控与合规/}).click()}else await page.locator(".side-nav").getByRole("button",{name:/风控与合规/}).click();await expect(page.getByRole("heading",{name:"风控与合规",exact:true})).toBeVisible()}
test("audited compliance configuration and explicit stub check replay across navigation",async({page},info)=>{
 const username=process.env.TEST_COMPLIANCE_ADMIN_USERNAME??process.env.TEST_HARBOR_ADMIN_USERNAME,password=process.env.TEST_COMPLIANCE_ADMIN_PASSWORD??process.env.TEST_HARBOR_ADMIN_PASSWORD;
 test.skip(!username||!password,"Provide isolated brand administrator credentials");
 test.setTimeout(60000);page.setDefaultTimeout(10000);
 const mobile=info.project.name==="mobile";
 const login=await page.request.post(`${api}/auth/login`,{headers:{Origin:origin,"Idempotency-Key":crypto.randomUUID()},data:{identifier:username,password}});expect(login.status()).toBe(200);
 const me=await page.request.get(`${api}/me`);expect(me.status()).toBe(200);const account=(await me.json()).data.account;
 const brands=await page.request.get(`${api}/brands`);expect(brands.status()).toBe(200);const brand=(await brands.json()).data.items.find((b:{code:string})=>b.code==="harbor");expect(brand).toBeTruthy();expect(account.permissions_by_brand[brand.id]).toContain("compliance_policy.write.brand");
 const headers={"X-Brand-ID":brand.id};const original=(await page.request.get(`${api}/compliance-policy`,{headers}).then(r=>r.json())).data;
 let policyChanged=false;
 try {
 await page.goto(origin);await page.getByTestId("admin-language").selectOption("zh-CN");await page.locator(".directory-brand-bar select").selectOption(brand.id);await visit(page,mobile);
 const panel=page.locator(".compliance");await expect(panel.getByRole("alert").first()).toContainText("启用但适配器未配置时会拒绝新业务");
 await panel.getByLabel("启用身份检查").check();const reason=`Explicit browser configuration ${crypto.randomUUID()}`;
 await panel.getByLabel("政策变更原因").fill(reason);await panel.getByRole("button",{name:"核对政策变更",exact:true}).click();
 let receipt:unknown;let dropped=false;const bodies:string[]=[],keys:string[]=[];
 await page.route("**/api/v1/admin/compliance-policy",async route=>{if(route.request().method()!=="PUT"){await route.continue();return}bodies.push(route.request().postData()??"");keys.push(route.request().headers()["idempotency-key"]??"");if(!dropped){dropped=true;const response=await route.fetch();expect(response.status()).toBe(200);policyChanged=true;receipt=(await response.json()).data;await route.abort("failed");return}await route.continue()});
 await panel.getByRole("button",{name:"确认并提交政策",exact:true}).click();await expect.poll(()=>Boolean(receipt)).toBe(true);await expect(panel.getByRole("button",{name:"用原键重试",exact:true})).toBeEnabled();
 if(mobile) await page.locator(".mobile-nav button").nth(1).click();else await page.locator(".side-nav").getByRole("button",{name:/用户和成员/}).click();await expect(panel).toHaveCount(0);await visit(page,mobile);
 await panel.getByRole("button",{name:"用原键重试",exact:true}).click();await expect(panel.locator(".notice")).toContainText("政策写入回执已核验");expect(bodies).toHaveLength(2);expect(bodies[1]).toBe(bodies[0]);expect(keys[1]).toBe(keys[0]);await page.unroute("**/api/v1/admin/compliance-policy");
 let current=(await page.request.get(`${api}/compliance-policy`,{headers}).then(r=>r.json())).data;expect(current.version).toBe(original.version+1);expect(current.config.identity_enabled).toBe(true);
 // A real, new anonymous user request must be rejected by the server; the
 // administrative simulation below is not evidence of this business gate.
 const regKey=crypto.randomUUID(),registrationUsername=`gate_${crypto.randomUUID().replaceAll("-","").slice(0,12)}`;
 const regBody={username:registrationUsername,password:`local-test-${crypto.randomUUID()}`,privacy_policy_version:"dev-1",service_terms_version:"dev-1"};
 const rejected=await page.request.post(`${origin}/api/v1/b/harbor/auth/register`,{headers:{Origin:origin,"Idempotency-Key":regKey},data:regBody});expect(rejected.status()).toBe(409);expect((await rejected.json()).error.code).toBe("COMPLIANCE_REVIEW_REQUIRED");
 expect((await page.request.post(`${origin}/api/v1/b/harbor/auth/register`,{headers:{Origin:origin,"Idempotency-Key":regKey},data:regBody})).status()).toBe(409);
 const gatePage=(await page.request.get(`${api}/compliance-gates?operation=registration`,{headers}).then(r=>r.json())).data;
 const record=gatePage.items.find((r:{action:string;policy_version:number})=>r.action==="register"&&r.policy_version===current.version);expect(record).toMatchObject({actor_type:"anonymous",actor_id:null,member_id:null,decision:"review",adapter_mode:"stub"});
 expect(gatePage.items.filter((r:{policy_version:number})=>r.policy_version===current.version)).toHaveLength(1);
 const gatePanel=panel.locator(".card").filter({has:page.getByRole("heading",{name:"真实业务闸门拒绝记录",exact:true})});await gatePanel.getByRole("button",{name:"刷新",exact:true}).click();await expect(gatePanel).toContainText(`注册 · 注册 · 复核 · v${current.version}`);
 await panel.getByLabel("操作类型").selectOption("betting");const checkReason=`Explicit browser stub ${crypto.randomUUID()}`;await panel.getByLabel("检查原因",{exact:true}).fill(checkReason);await panel.getByRole("button",{name:"核对伪检查",exact:true}).click();
 let checkReceipt:{id:string;policy_version:number;decision:string;adapter_mode:string;audit_log_id:string}|undefined;let checkDropped=false;const checkBodies:string[]=[],checkKeys:string[]=[];
 await page.route("**/api/v1/admin/compliance-checks",async route=>{if(route.request().method()!=="POST"){await route.continue();return}checkBodies.push(route.request().postData()??"");checkKeys.push(route.request().headers()["idempotency-key"]??"");if(!checkDropped){checkDropped=true;const response=await route.fetch();expect(response.status()).toBe(201);checkReceipt=(await response.json()).data;await route.abort("failed");return}await route.continue()});
 await panel.getByRole("button",{name:"确认并运行 stub 检查",exact:true}).click();await expect.poll(()=>Boolean(checkReceipt)).toBe(true);await expect(panel.getByRole("button",{name:"用原键重试",exact:true})).toBeEnabled();
 if(mobile) await page.locator(".mobile-nav button").nth(1).click();else await page.locator(".side-nav").getByRole("button",{name:/用户和成员/}).click();await expect(panel).toHaveCount(0);await visit(page,mobile);
 await panel.getByRole("button",{name:"用原键重试",exact:true}).click();await expect(panel.locator(".notice")).toContainText("伪适配器决定已记录");expect(checkBodies).toHaveLength(2);expect(checkBodies[1]).toBe(checkBodies[0]);expect(checkKeys[1]).toBe(checkKeys[0]);expect(checkReceipt).toMatchObject({decision:"review",adapter_mode:"stub",policy_version:current.version});await page.unroute("**/api/v1/admin/compliance-checks");
 const decisions=(await page.request.get(`${api}/compliance-checks?operation=betting`,{headers}).then(r=>r.json())).data;expect(decisions.items.filter((d:{reason:string})=>d.reason===checkReason)).toHaveLength(1);await expect(panel.locator(".records")).toContainText(checkReason);
 await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);if(process.env.LOTTERY_BROWSER_SCREENSHOT)await page.screenshot({path:process.env.LOTTERY_BROWSER_SCREENSHOT,fullPage:true});
 } finally {
  await page.unrouteAll({behavior:"wait"});
  // Restore even after an assertion fails, so later real business scenarios
  // cannot inherit this test's deliberately enabled, unconfigured KYC gate.
  if(policyChanged){const current=(await page.request.get(`${api}/compliance-policy`,{headers}).then(r=>r.json())).data;const restored=await page.request.put(`${api}/compliance-policy`,{headers:{...headers,Origin:origin,"Idempotency-Key":crypto.randomUUID()},data:{version:current.version,config:original.config,reason:"Restore explicit browser compliance configuration"}});expect(restored.status()).toBe(200);
  const final=(await page.request.get(`${api}/compliance-policy`,{headers}).then(r=>r.json())).data;expect(final.config).toEqual(original.config);}
 }
});
