import {test,expect} from "@playwright/test";
import {restoreAdminSession} from "./support/admin-session";

test("real ledger report separates transfer flows from current balances and clears foreign filters",async({page,context},info)=>{
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME||!process.env.TEST_HARBOR_ADMIN_PASSWORD,"Provide isolated Harbor administrator credentials");
  const brand="0199a000-0000-7000-8000-000000000002",base="http://localhost:5174/api/v1/admin";
  const headers={Origin:"http://localhost:5174","X-Brand-ID":brand};
  if(!await restoreAdminSession(context,process.env.TEST_HARBOR_ADMIN_USERNAME!,brand)) {
    const r=await page.request.post(`${base}/auth/login`,{headers:{...headers,"Idempotency-Key":crypto.randomUUID()},data:{identifier:process.env.TEST_HARBOR_ADMIN_USERNAME,password:process.env.TEST_HARBOR_ADMIN_PASSWORD}});
    expect(r.status(),await r.text()).toBe(200);
  }
  const post=async(path:string,body:unknown,status=200)=>{const r=await page.request.post(`${base}${path}`,{headers:{...headers,"Idempotency-Key":crypto.randomUUID()},data:body});expect(r.status(),await r.text()).toBe(status);return(await r.json()).data};
  const member=await post("/users",{username:`report_${crypto.randomUUID().replaceAll("-","").slice(0,12)}`,password:"report-test-only-password-2026",reason:"isolated read-only report fixture"},201);
  const pending=await post("/recharges",{member_id:member.member_id,points:"25",proof_reference:"isolated report test",remark:"report fixture",reason:"report fixture funding"},201);
  await post(`/recharges/${pending.id}/confirm`,{version:pending.version,reason:"confirm report fixture"});
  const walletR=await page.request.get(`${base}/wallets/${member.member_id}`,{headers});expect(walletR.status()).toBe(200);
  const wallet=(await walletR.json()).data;
  await post(`/wallets/${member.member_id}/freeze`,{points:"5",reason:"report transfer fixture"});
  const before=await page.request.get(`${base}/wallets/${member.member_id}`,{headers});const beforeWallet=(await before.json()).data;
  const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
  await page.goto("http://localhost:5174");await page.getByLabel("选择真实后台品牌",{exact:true}).selectOption(brand);
  if(info.project.name==="mobile") {await page.locator(".mobile-nav button").nth(4).click();await page.locator(".mobile-more-menu").getByRole("button",{name:/报表和对账/}).click()}
  else await page.locator(".side-nav").getByRole("button",{name:/报表和对账/}).click();
  const panel=page.locator(".reports-management");
  await panel.getByLabel("会员筛选 UUID",{exact:true}).fill(member.member_id);
  const bound=await page.evaluate(()=>{const d=new Date(Date.now()+3600000),p=(n:number)=>String(n).padStart(2,"0");return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`});
  await panel.getByLabel("报表结束时间",{exact:true}).fill(bound);
  await panel.getByRole("button",{name:"账本报表",exact:true}).click();
  await panel.getByLabel("账本分组",{exact:true}).selectOption("entry_type");
  await panel.getByRole("button",{name:"查询报表",exact:true}).click();
  const ledger=panel.locator(".reports-panel").filter({has:page.getByRole("heading",{name:"账本报表",exact:true})});
  const metric=(label:string)=>ledger.locator(".reports-summary>div").filter({has:page.locator("span").filter({hasText:new RegExp(`^${label}$`)})}).locator("strong");
  await expect(metric("账本笔数")).toHaveText("2");await expect(metric("净变动")).toHaveText("25");await expect(metric("充值入账")).toHaveText("25");
  await expect(ledger.locator(".reports-balance")).toContainText("20");await expect(ledger.locator(".reports-balance")).toContainText("5");
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await ledger.screenshot({path:info.outputPath("s6-d-real-ledger-report.png")});
  await panel.getByLabel("会员筛选 UUID",{exact:true}).fill("0199a000-0000-7000-8000-000000000001");
  await panel.getByRole("button",{name:"查询报表",exact:true}).click();
  await expect(ledger.getByRole("alert")).toContainText("未找到");await expect(ledger.locator(".reports-summary")).toHaveCount(0);
  const after=await page.request.get(`${base}/wallets/${member.member_id}`,{headers});expect((await after.json()).data).toEqual(beforeWallet);
  expect(errors).toEqual([]);
});
