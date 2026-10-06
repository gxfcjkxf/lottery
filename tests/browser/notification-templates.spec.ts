import {test,expect,type Page,type APIRequestContext} from "@playwright/test";
const brand="0199a000-0000-7000-8000-000000000002",admin="http://localhost:5174/api/v1/admin",origin="http://localhost:5174";
const userBase="http://localhost:5173/api/v1/b/harbor";
const username=process.env.TEST_TEMPLATE_ADMIN_USERNAME,password=process.env.TEST_TEMPLATE_ADMIN_PASSWORD;
type Content={en:{title:string;body:string};"zh-CN":{title:string;body:string}};
type Template={brand_id:string;key:string;version:number;content:Content;audit_log_id:string|null};
type Message={id:string;template_version:number;content:Content|null;payload:{resource_id:string;points:string|null}};
const key=()=>crypto.randomUUID();
async function unwrap<T>(r:Awaited<ReturnType<APIRequestContext["get"]>>,status=200):Promise<T>{
 const text=await r.text();expect(r.status(),text).toBe(status);return JSON.parse(text).data;
}
async function read(page:Page):Promise<Template>{
 const v=await unwrap<{items:Template[]}>(await page.request.get(admin+"/notification-templates",{headers:{"X-Brand-ID":brand}}));
 return v.items.find(t=>t.key==="member.joined")!;
}
async function restore(page:Page,content:Content){
 const live=await read(page);
 if(JSON.stringify(live.content)===JSON.stringify(content))return;
 const intent={version:live.version,content,reason:"Restore notification template after isolated browser verification"},id=key();
 const send=()=>page.request.put(admin+"/notification-templates/member.joined",{headers:{Origin:origin,"X-Brand-ID":brand,"Idempotency-Key":id},data:intent});
 let r=await send();if(r.status()>=500)r=await send();
 const v=await unwrap<Template>(r);expect(v.version).toBe(live.version+1);expect(v.content).toEqual(content);
}
async function register(page:Page){
 return unwrap<{access_token:string;member:{id:string}}>(await page.request.post(userBase+"/auth/register",{
  headers:{Origin:"http://localhost:5173","Idempotency-Key":key()},data:{username:"template_"+key().replaceAll("-","").slice(0,12),password:"isolated-notification-user-test-password-2026",privacy_policy_version:"dev-1",service_terms_version:"dev-1"}
 }),201);
}
async function inbox(page:Page,token:string){
 return unwrap<{items:Message[]}>(await page.request.get(userBase+"/notifications?limit=100",{headers:{Authorization:"Bearer "+token}}));
}
async function navigateTemplates(page:Page,project:string){
 if(project==="mobile"){await page.locator(".mobile-nav button").nth(4).click();await page.locator(".mobile-more-menu").getByRole("button",{name:/通知模板/}).click()}
 else await page.locator(".side-nav").getByRole("button",{name:/通知模板/}).click();
 return page.locator(".nt-page");
}
test("real bilingual template publication survives lost receipt and preserves historical user messages",async({page,context},info)=>{
 test.skip(!username||!password,"Provide isolated template administrator credentials");
 const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
 await unwrap(await page.request.post(admin+"/auth/login",{headers:{Origin:origin,"Idempotency-Key":key()},data:{identifier:username,password}}));
 const original=await read(page),prior=await register(page);
 await expect.poll(async()=> (await inbox(page,prior.access_token)).items.length).toBe(1);
 const oldMessage=(await inbox(page,prior.access_token)).items[0]!;
 expect(oldMessage.template_version).toBe(original.version);
 await page.goto(origin);await page.getByLabel("选择真实后台品牌",{exact:true}).selectOption(brand);
 let panel=await navigateTemplates(page,info.project.name);
 await panel.getByLabel("通知事件",{exact:true}).selectOption("member.joined");
 await expect(panel.locator("#nt-title-en")).toHaveValue(original.content.en.title);
 const content:Content={en:{title:"Welcome "+info.project.name,body:"Hello {resource_id}.\nWelcome to this brand."},"zh-CN":{title:"品牌欢迎 "+info.project.name,body:"欢迎会员 {resource_id}。\n这是本品牌的站内通知。"}};
 try{
  await panel.locator("#nt-title-en").fill(content.en.title);await panel.locator("#nt-body-en").fill(content.en.body);
  await panel.locator("#nt-title-zh-CN").fill(content["zh-CN"].title);await panel.locator("#nt-body-zh-CN").fill(content["zh-CN"].body);
  await panel.getByLabel("修改原因（最多 500 UTF-8 字节）",{exact:true}).fill("Publish bilingual welcome for isolated browser verification");
  await expect(panel.locator(".nt-preview").first()).toContainText("00000000-0000-4000-8000-000000000099");
  await panel.getByRole("button",{name:"核对并继续",exact:true}).click();
  await panel.getByRole("checkbox",{name:"我已核对品牌、事件、版本、中英文内容和原因，并确认保存",exact:true}).check();
  let firstBody:string|undefined,firstKey:string|undefined,attempts=0;
  await page.route("**/api/v1/admin/notification-templates/member.joined",async route=>{
   if(route.request().method()!=="PUT")return route.continue();
   attempts++;const body=route.request().postData()!,id=route.request().headers()["idempotency-key"];
   if(!firstBody){firstBody=body;firstKey=id;const actual=await route.fetch();expect(actual.status()).toBe(200);
    // The real update committed. Drop the useful receipt by delivering an
    // invalid success envelope; never fabricate a successful business result.
    await route.fulfill({status:200,contentType:"application/json",body:'{"success":true,"data":{}}'})}
   else{expect(body).toBe(firstBody);expect(id).toBe(firstKey);await route.continue()}
  });
  await panel.getByRole("button",{name:"确认保存",exact:true}).click();
  await expect(panel.getByRole("heading",{name:"保存结果未知",exact:true})).toBeVisible();
  const committed=await read(page);expect(committed.version).toBe(original.version+1);expect(committed.content).toEqual(content);
  await panel.getByRole("button",{name:"只读刷新通知模板",exact:true}).click();
  await expect(panel.getByRole("heading",{name:"保存结果未知",exact:true})).toBeVisible();
  if(info.project.name==="mobile"){await page.locator(".mobile-nav button").nth(4).click();await page.locator(".mobile-more-menu").getByRole("button",{name:/报表和对账/}).click()}
  else await page.locator(".side-nav").getByRole("button",{name:/报表和对账/}).click();
  panel=await navigateTemplates(page,info.project.name);
  await panel.getByLabel("通知事件",{exact:true}).selectOption("member.joined");
  await expect(panel.getByRole("heading",{name:"保存结果未知",exact:true})).toBeVisible();
  await panel.getByRole("button",{name:"按原请求重试",exact:true}).click();
  await expect(panel.locator(".nt-receipt")).toHaveText("已确认保存 v"+committed.version);
  expect(attempts).toBe(2);
  const history=await unwrap<{items:{version:number}[]}>(await page.request.get(admin+"/notification-templates/member.joined/history?limit=100&offset=0",{headers:{"X-Brand-ID":brand}}));
  expect(history.items.filter(r=>r.version===committed.version)).toHaveLength(1);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await panel.screenshot({path:info.outputPath("s7l-template-"+info.project.name+".png")});
  const joined=await register(page);
  await expect.poll(async()=> (await inbox(page,joined.access_token)).items.length).toBe(1);
  const fresh=(await inbox(page,joined.access_token)).items[0]!;
  expect(fresh.template_version).toBe(committed.version);expect(fresh.content).toEqual(content);
  expect((await inbox(page,prior.access_token)).items[0]).toEqual(oldMessage);
  const cookies=(await context.cookies(userBase)).filter(c=>c.name===`lottery_user_${brand.replaceAll("-","")}`);
  expect(cookies).toHaveLength(1);await context.addCookies(cookies.map(c=>({...c,domain:"harbor.localhost"})));
  const user=await context.newPage();
  user.on("pageerror",e=>errors.push(e.message));
  await user.route("http://harbor.localhost:5173/api/**",async route=>{
   const r=await route.fetch({url:route.request().url().replace("harbor.localhost","localhost"),headers:{...(await route.request().allHeaders()),host:"harbor.localhost:5173"}});
   await route.fulfill({response:r});
  });
  await user.goto("http://harbor.localhost:5173/notifications");
  const userPanel=user.locator(".notifications-panel");
  await expect(userPanel.getByRole("heading",{name:content.en.title,exact:true})).toBeVisible();
  await expect(userPanel).toContainText("Hello "+joined.member.id+".");
  await user.getByRole("button",{name:"Switch to Chinese",exact:true}).click();
  await expect(userPanel.getByRole("heading",{name:content["zh-CN"].title,exact:true})).toBeVisible();
  await expect(userPanel).toContainText("欢迎会员 "+joined.member.id+"。");
  expect(await user.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await userPanel.screenshot({path:info.outputPath("s7l-inbox-"+info.project.name+".png")});await user.close();
  await restore(page,original.content);
  expect((await inbox(page,joined.access_token)).items[0]).toEqual(fresh);
 }finally{
  await page.unroute("**/api/v1/admin/notification-templates/member.joined");
  await restore(page,original.content);
 }
 expect(errors).toEqual([]);
});
