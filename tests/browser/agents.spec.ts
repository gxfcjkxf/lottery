import {test,expect,type APIRequestContext} from "@playwright/test";
import {rememberAdminSession,restoreAdminSession} from "./support/admin-session";

test("real agent policy tree and direct-child configuration retain audited idempotent receipts without money writes",async({page,context},info)=>{
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME||!process.env.TEST_HARBOR_ADMIN_PASSWORD,"Provide isolated Harbor admin credentials");
  test.setTimeout(45_000);page.setDefaultTimeout(8000);
  const brand="0199a000-0000-7000-8000-000000000002",origin="http://localhost:5173",admin=`${origin}/api/v1/admin`,pub=`${origin}/api/v1/b/harbor`;
  const headers={Origin:origin,"X-Brand-ID":brand};const uid=()=>crypto.randomUUID();
  if(!await restoreAdminSession(context,process.env.TEST_HARBOR_ADMIN_USERNAME!,brand)) {
    const r=await page.request.post(`${admin}/auth/login`,{headers:{...headers,"Idempotency-Key":uid()},data:{identifier:process.env.TEST_HARBOR_ADMIN_USERNAME,password:process.env.TEST_HARBOR_ADMIN_PASSWORD}});expect(r.status(),await r.text()).toBe(200);
  }
  rememberAdminSession(process.env.TEST_HARBOR_ADMIN_USERNAME!,await context.cookies(`${admin}/me`));
  const adminCall=async(path:string,method:"GET"|"POST"|"PUT",body?:unknown,status=200)=>{
    const r=await page.request.fetch(`${admin}${path}`,{method,headers:{...headers,...(method==="GET"?{}:{"Idempotency-Key":uid()})},...(body===undefined?{}:{data:body})});expect(r.status(),await r.text()).toBe(status);return(await r.json()).data;
  };
  const register=await page.request.post(`${pub}/auth/register`,{headers:{Origin:origin,"Idempotency-Key":uid()},data:{username:`agency_${uid().replaceAll("-","").slice(0,12)}`,password:"agent-test-only-password-2026",privacy_policy_version:"dev-1",service_terms_version:"dev-1"}});
  expect(register.status(),await register.text()).toBe(201);const user=(await register.json()).data,member=user.member.id;
  const before=await adminCall(`/wallets/${member}`,"GET");const beforeLedger=await adminCall(`/wallets/${member}/ledger`,"GET");
  const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
  await page.goto("http://localhost:5174");await page.getByLabel("选择真实后台品牌",{exact:true}).selectOption(brand);
  if(info.project.name==="mobile"){await page.locator(".mobile-nav button").nth(4).click();await page.locator(".mobile-more-menu").getByRole("button",{name:/代理树/}).click()}
  else await page.locator(".side-nav").getByRole("button",{name:/代理树/}).click();
  const panel=page.locator(".agent-management");await expect(panel.getByRole("heading",{name:"代理管理",exact:true})).toBeVisible();
  await panel.getByLabel("代理管理启用",{exact:true}).check();await panel.getByLabel("代理最大层级",{exact:true}).fill("3");
  const cycle=await panel.getByLabel("佣金周期",{exact:true}).inputValue()==="weekly"?"monthly":"weekly";
  await panel.getByLabel("品牌比例上限",{exact:true}).fill("0.1");await panel.getByLabel("品牌佣金模式",{exact:true}).selectOption("loss");await panel.getByLabel("佣金周期",{exact:true}).selectOption(cycle);
  await panel.getByLabel("代理政策变更原因",{exact:true}).fill("Enable isolated agent configuration without payouts");
  const policyWrites:Array<{body:string|null,key:string|undefined}>=[];
  await page.route("**/api/v1/admin/agent-policy",async route=>{if(route.request().method()!=="PUT"){await route.continue();return};policyWrites.push({body:route.request().postData(),key:route.request().headers()["idempotency-key"]});const r=await route.fetch();expect(r.status(),await r.text()).toBe(200);if(policyWrites.length===1)await route.abort("failed");else await route.fulfill({response:r})});
  await panel.getByRole("button",{name:"核对并保存",exact:true}).click();const review=panel.locator(".am-review");await review.getByRole("checkbox").check();await review.getByRole("button",{name:"确认",exact:true}).click();
  await expect(panel.getByRole("status")).toContainText("未知");await panel.getByRole("button",{name:"查询",exact:true}).first().click();await expect(review).toBeVisible();
  await review.getByRole("button",{name:"按原键重试",exact:true}).click();await expect(review).toHaveCount(0);expect(policyWrites).toHaveLength(2);expect(policyWrites[0]).toEqual(policyWrites[1]);await page.unroute("**/api/v1/admin/agent-policy");
  const policy=await adminCall("/agent-policy","GET");expect(policy).toMatchObject({config:{enabled:true,max_depth:3,ratio_cap:"0.1",cycle}});
  const root=await adminCall("/agents","POST",{policy_version:policy.version,member_id:member,parent_id:null,parent_version:null,config:{ratio:"0.08",mode:null,status:"active",can_create_children:true},reason:"real root agent fixture"},201);
  const childMember=await adminCall("/users","POST",{username:`agency_child_${uid().replaceAll("-","").slice(0,12)}`,password:"agent-child-test-password-2026",reason:"isolated direct child fixture"},201);
  const child=await adminCall("/agents","POST",{policy_version:policy.version,member_id:childMember.member_id,parent_id:root.id,parent_version:root.version,config:{ratio:"0.04",mode:null,status:"active",can_create_children:false},reason:"real direct child fixture"},201);
  await panel.getByRole("button",{name:"查询",exact:true}).first().click();await expect(panel.locator(".am-node").filter({hasText:member})).toBeVisible();
  await panel.locator(".am-node").filter({hasText:member}).getByRole("button",{name:"下级",exact:true}).click();await expect(panel.locator(".am-node").filter({hasText:child.member_id})).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await panel.screenshot({path:info.outputPath("s6-e-admin-agents.png")});
  const userCookies=(await context.cookies(pub)).filter(c=>c.name===`lottery_user_${brand.replaceAll("-","")}`);expect(userCookies).toHaveLength(1);await context.addCookies(userCookies.map(c=>({...c,domain:"harbor.localhost"})));
  const userPage=await context.newPage();userPage.on("pageerror",e=>errors.push(e.message));
  await userPage.route("http://harbor.localhost:5173/api/**",async route=>{const r=await route.fetch({url:route.request().url().replace("harbor.localhost","localhost"),headers:{...(await route.request().allHeaders()),host:"harbor.localhost:5173"}});await route.fulfill({response:r})});
  await userPage.goto("http://harbor.localhost:5173/agent");const agent=userPage.locator(".agent-panel");await expect(agent).toContainText(child.member_id);
  await agent.getByLabel("Ratio fraction",{exact:true}).fill("0.03");await agent.getByLabel("Review reason",{exact:true}).fill("User adjusts only the direct child ratio");
  const childWrites:Array<{body:string|null,key:string|undefined}>=[];
  await userPage.route(`**/agent/children/${child.id}/config`,async route=>{childWrites.push({body:route.request().postData(),key:route.request().headers()["idempotency-key"]});const r=await route.fetch({url:route.request().url().replace("harbor.localhost","localhost"),headers:{...(await route.request().allHeaders()),host:"harbor.localhost:5173"}});expect(r.status(),await r.text()).toBe(200);if(childWrites.length===1)await route.abort("failed");else await route.fulfill({response:r})});
  await agent.getByRole("button",{name:"Review change",exact:true}).click();await agent.getByRole("button",{name:"Confirm update",exact:true}).click();await expect(agent.getByRole("button",{name:"Retry the same request",exact:true})).toBeVisible();
  await agent.getByRole("button",{name:"Reload",exact:true}).click();await expect(agent.getByRole("button",{name:"Retry the same request",exact:true})).toBeVisible();
  await agent.getByRole("button",{name:"Retry the same request",exact:true}).click();await expect(agent.getByRole("button",{name:"Retry the same request",exact:true})).toHaveCount(0);
  expect(childWrites).toHaveLength(2);expect(childWrites[0]).toEqual(childWrites[1]);
  const currentChild=await adminCall(`/agents/${child.id}`,"GET");expect(currentChild).toMatchObject({version:2,config:{ratio:"0.03",mode:null},parent_id:root.id});
  const history=await adminCall(`/agents/${child.id}/history`,"GET");expect(history.items.map((v:{version:number})=>v.version)).toEqual([2,1]);expect(history.items[0].actor_type).toBe("user");
  const denied=await page.request.put(`${pub}/agent/children/${root.id}/config`,{headers:{Origin:origin,Authorization:`Bearer ${user.access_token}`,"Idempotency-Key":uid()},data:{version:1,policy_version:policy.version,parent_version:1,ratio:"0.02",mode:null,reason:"not a direct child"}});expect(denied.status()).toBe(403);
  expect(await adminCall(`/wallets/${member}`,"GET")).toEqual(before);expect(await adminCall(`/wallets/${member}/ledger`,"GET")).toEqual(beforeLedger);
  expect(await userPage.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await agent.screenshot({path:info.outputPath("s6-e-user-agent.png")});expect(errors).toEqual([]);
  await userPage.unrouteAll({behavior:"wait"});
});
