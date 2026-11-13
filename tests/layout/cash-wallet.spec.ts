import { test, expect, type Page } from "@playwright/test";
import {
  installLayoutApi,
  installOrdersLayoutApi,
  origins,
  gameId,
  expectNoPageOverflow,
} from "./fixture";
import {
  currencies,
  exchangeResult,
  type CashConfig,
  type CashOrder,
} from "../../shared/src/cash";
const config = (): CashConfig => ({
  version: 1,
  platform_rules: currencies.map((currency) => ({ currency, rate: "7", deposit_min: "1", deposit_max: "10000", withdrawal_min: "1", withdrawal_max: "10000", fee: "0", mode: "count", threshold: "1", inherit: false, enabled: true })),
  rules: currencies.map((currency) => ({
    currency,
    rate: "7",
    deposit_min: "1",
    deposit_max: "10000",
    withdrawal_min: "1",
    withdrawal_max: "10000",
    fee: "0",
    mode: "count",
    threshold: "2",
    inherit: false,
    enabled: true,
  })),
});
const address = "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7";
const destination = (currency = "USDT") => ({
  id: gameId,
  currency,
  channel: currency === "USDT" ? "TRON/TRC20" : "bank",
  holder: currency === "USDT" ? "" : "Cash recipient",
  institution: currency === "USDT" ? "" : "Cash bank",
  number: currency === "USDT" ? address : "12345678901234567890",
  enabled: true,
  version: 1,
});
const order = (
  kind: CashOrder["kind"],
  currency: CashOrder["currency"],
  amount: string,
): CashOrder => ({
  automatic:false,
  credited_at:"",
  sender:"",
  id: gameId,
  brand_id: gameId,
  member_id: gameId,
  member_account: "layout-member",
  kind,
  currency,
  amount,
  requested_amount: amount,
  points: "0",
  fee: "0",
  rate: "",
  state: "pending",
  version: 1,
  destination: destination(currency) as CashOrder["destination"],
  proof: "",
  reference: "",
  created_at: "2026-11-11T00:00:00Z",
  expires_at: new Date(Date.now()+30*60*1000).toISOString(),
});
async function userFixture(page: Page, locale: "en" | "zh-CN", agent = false) {
  await installLayoutApi(page, locale);
  await installOrdersLayoutApi(page);
  const state = {
    auto_usdt:false,
    actor_context: "a".repeat(64),
    brand_id: gameId,
    member_id: gameId,
    balances: currencies.map((currency) => ({
      currency,
      deposited: currency === "USDT" ? "123.456789" : "10",
      withdrawable: currency === "CNY" ? "2" : "0",
      reserved: "0",
    })),
    destinations: [] as ReturnType<typeof destination>[],
    platform: config(),
    brand: config(),
  };
  const orders: CashOrder[] = [];
  const writes: { body: string | null; key: string | undefined }[] = [];
  let lose = false;
  if (agent) await page.route("**/api/v1/agent/me", route => route.fulfill({json:{success:true,data:{account:{agent_account_id:gameId,username:'cash_agent'},agent:{id:gameId,brand_id:gameId,depth:1,config:{can_create_children:false}},code:null,user_origin:origins.user,brand_code:''}}}));
  if (agent) await page.route('**/api/v1/agent/wallet', route => route.fulfill({json:{success:true,data:{available_points:'70',commission_points:'0'}}}));
  const base = agent ? "/api/v1/agent" : "/api/v1";
  await page.route(`**${base}/cash**`, async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname;
    const reply = (data: unknown) =>
      route.fulfill({ json: { success: true, data } });
    if (path === base + "/cash") return reply(state);
    if (path === base + "/cash/orders")
      return reply({ items: orders, total_count: String(orders.length) });
    expect(req.headers()["x-cash-actor-context"]).toBe(state.actor_context);
    const body = req.postDataJSON();
    writes.push({
      body: req.postData(),
      key: req.headers()["idempotency-key"],
    });
    if (lose) { lose=false; return route.abort("failed"); }
    if (path.endsWith("/exchanges")) {
      const converted=exchangeResult(body.amount,body.rate,body.direction==='to_points',body.currency);
      return reply({
        ...order(body.direction, body.currency, body.direction==='to_points'?body.amount:converted),
        state: "completed",
        points: body.direction==='to_points'?converted:body.amount,
        rate: body.rate,
        destination: {
          id: "",
          currency: "",
          channel: "",
          holder: "",
          institution: "",
          number: "",
          enabled: false,
          version: 0,
        },
      });
    }
    if (path.endsWith("/deposits")) {
      const o = order("deposit", body.currency, body.amount);
      o.automatic=state.auto_usdt&&body.currency==='USDT';
      if (o.automatic) o.fee='0.000137';
      orders.push(o);
      return reply(o);
    }
    if (path.endsWith('/destinations/delete')) {
      const index = state.destinations.findIndex(d => d.id === body.id);
      if (index < 0 || state.destinations[index].version !== body.version) return route.fulfill({ status: 409, json: { success: false, error: { code: 'CASH_STATE_CHANGED', message: 'Changed' } } });
      const [d] = state.destinations.splice(index, 1);
      return reply({ ...d, enabled: false, version: d.version + 1 });
    }
    if (path.endsWith("/destinations")) {
      const previous = state.destinations.find(d => d.id === body.id);
      if (!previous && state.destinations.some(d => d.currency === body.currency)) return route.fulfill({ status: 409, json: { success: false, error: { code: 'CASH_ACCOUNT_EXISTS', message: 'Already bound' } } });
      const d = { ...body, id: previous?.id ?? crypto.randomUUID(), version: (previous?.version ?? 0) + 1 };
      if (previous) Object.assign(previous, d); else state.destinations.push(d);
      return reply(d);
    }
    if (path.endsWith("/withdrawals")) {
      const o = {
        ...order("withdrawal", body.currency, body.amount),
        state: "reviewing",
      };
      orders.push(o);
      return reply(o);
    }
    if(path.endsWith('/submit')) { const o=orders[0]; o.state='submitted';o.version++;return reply(o); }
    if(path.endsWith('/cancel')) { const o=orders[0]; o.state='cancelled';o.version++;return reply(o); }
    return route.abort("failed");
  });
  return { state, orders, writes, loseNext: () => (lose = true) };
}
async function widths(
  page: Page,
  reflow: boolean,
  screenshot: (width: number) => Promise<unknown>,
) {
  const preset = page.viewportSize()!;
  for (const width of reflow ? [320, 480, 768, 1280] : [preset.width]) {
    if (reflow) await page.setViewportSize({ width, height: 800 });
    await expectNoPageOverflow(page);
    await page.evaluate(() => window.scrollTo(0, 0));
    await screenshot(width);
  }
}
for (const locale of ['en', 'zh-CN'] as const) test(`cash records use the shared empty table state ${locale}`, async ({ page }, info) => {
  const fixture = await userFixture(page, locale);
  await page.goto(origins.user + '/wallet');
  const empty = page.locator('.cash-record-table td.empty-state');
  await expect(empty).toBeVisible();
  await widths(page, info.project.name.includes('reflow'), async width => {
    await expect(empty).toHaveCSS('display', 'table-cell');
    await expect(empty.locator('..')).toHaveCSS('display', 'table-row');
    const table = await page.locator('.cash-record-table').boundingBox();
    const cell = await empty.boundingBox();
    expect(Math.abs(cell!.width - table!.width)).toBeLessThanOrEqual(2);
    await page.screenshot({ path: info.outputPath(`cash-empty-${locale}-${width}.png`), fullPage: true });
  });
  expect(fixture.writes).toHaveLength(0);
});

test("wallet only shows enabled brand currencies", async ({ page }, info) => {
  const fixture = await userFixture(page, "en");
  fixture.state.brand.rules.forEach((rule) => { rule.enabled = rule.currency === "USDT"; });
  for (const path of ["/wallet", "/recharge", "/withdraw"]) {
    await page.goto(origins.user + path);
    const cash = page.locator(".currency-wallet");
    await expect(cash.locator(".cash-balance")).toHaveCount(path === '/recharge' ? 0 : 1);
    if (path !== '/recharge') {
      await expect(cash.locator(".cash-balance")).toContainText("USDT");
      await expect(cash.locator(".cash-balance")).toHaveAttribute("aria-pressed", "true");
    }
    await cash.getByRole("button", { name: "Exchange", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("combobox", { name: "Currency", exact: true }).click();
    await expect(page.getByRole("option")).toHaveCount(1);
    await expect(page.getByRole("option")).toContainText("USDT");
    await page.keyboard.press("Escape");
    await widths(page, info.project.name.includes("reflow"), (width) =>
      page.screenshot({ path: info.outputPath(`enabled-${path.slice(1)}-${width}.png`), fullPage: true }),
    );
  }
  fixture.state.brand.rules.forEach((rule) => { rule.enabled = false; });
  await page.goto(origins.user + "/withdraw");
  const cash = page.locator(".currency-wallet");
  await expect(cash.locator(".cash-balance")).toHaveCount(0);
  await expect(cash.getByRole("button", { name: "Add payment account", exact: true })).toHaveCount(0);
  expect(fixture.writes).toHaveLength(0);
});
for (const locale of ["en", "zh-CN"] as const) {
  const zh = locale === "zh-CN";
  test(`deposit amount hint remains when switching currencies ${locale}`, async ({ page }, info) => {
    const fixture = await userFixture(page, locale);
    fixture.state.auto_usdt = true;
    await page.goto(origins.user + '/recharge');
    await page.locator('.currency-wallet').getByRole('button', { name: zh ? '充值' : 'Deposit', exact: true }).click();
    const dialog = page.getByRole('dialog');
    const amount = dialog.getByLabel(zh ? '金额' : 'Amount', { exact: true });
    for (const currency of ['THB', 'USDT', 'THB']) {
      await dialog.getByRole('combobox', { name: zh ? '币种' : 'Currency', exact: true }).click();
      await page.getByRole('option', { name: currency === 'THB' ? (zh ? '泰铢' : 'THB') : 'USDT', exact: true }).click();
      const hint = dialog.locator('.muted').filter({ hasText: '100.03' });
      await expect(hint).toBeVisible();
      await expect(amount).toHaveAttribute('aria-describedby', await hint.getAttribute('id') as string);
    }
    await widths(page, info.project.name.includes('reflow'), width => page.screenshot({ path: info.outputPath(`deposit-hint-${locale}-${width}.png`), fullPage: true }));
    expect(fixture.writes).toHaveLength(0);
  });
  test(`dedicated USDT deposit explains automatic crediting ${locale}`,async({page},info)=>{
    const fixture=await userFixture(page,locale);fixture.state.auto_usdt=true;fixture.state.brand.rules.forEach(r=>r.enabled=r.currency==='USDT');
    await page.goto(origins.user+'/recharge');const cash=page.locator('.currency-wallet');
    await cash.getByRole('button',{name:zh?'充值':'Deposit'}).click();
    let dialog=page.getByRole('dialog');await expect(dialog).toContainText('100.03');
    await dialog.getByLabel(zh?'金额':'Amount',{exact:true}).fill('10');await dialog.getByRole('button',{name:zh?'确认':'Confirm',exact:true}).click();
    dialog=page.getByRole('dialog');await expect(dialog).toContainText(zh?'链上确认后自动':'automatically after on-chain confirmation');
    await expect(dialog).toContainText(zh?'入账方式':'Crediting');
    await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`auto-deposit-${locale}-${width}.png`),fullPage:true}));
    await dialog.getByRole('button',{name:zh?'我已转账':'I have transferred',exact:true}).click();
    await expect(dialog).toHaveCount(0);await cash.locator('.cash-records').getByRole('button',{name:zh?'详情':'Details',exact:true}).click();
    await expect(page.getByRole('dialog')).toContainText(zh?'等待链上确认':'Waiting for on-chain confirmation');
    await page.getByRole('dialog').getByRole('button',{name:zh?'关闭':'Close',exact:true}).click();
    fixture.state.auto_usdt=false;await page.reload();await cash.locator('.cash-records').getByRole('button',{name:zh?'详情':'Details',exact:true}).click();
    await expect(page.getByRole('dialog')).toContainText(zh?'自动入账已暂停':'Automatic crediting is paused');
  });
  test(`agent own deposit and frozen retry ${locale}`, async ({page}, info) => {
    const fixture=await userFixture(page,locale,true);
    await page.goto(origins.agent + '/#/recharge');
    const cash=page.locator('.currency-wallet');
    await expect(cash.locator('.cash-balance')).toHaveCount(0);
    await expect(cash.getByRole('button',{name:zh?'兑换':'Exchange',exact:true})).toBeVisible();
    await cash.getByRole('button',{name:zh?'充值':'Deposit'}).click();
    let dialog=page.getByRole('dialog');
    await expect(dialog).toContainText('100.03');
    await dialog.getByLabel(zh?'金额':'Amount',{exact:true}).fill('10');
    fixture.loseNext();
    await dialog.getByRole('button',{name:zh?'确认':'Confirm',exact:true}).click();
    await expect(dialog.getByRole('button',{name:zh?'重试原操作':'Retry original operation'})).toBeVisible();
    await dialog.getByRole('button',{name:zh?'重试原操作':'Retry original operation'}).click();
    expect(fixture.writes[0]).toEqual(fixture.writes[1]);
    await expect(dialog).toContainText(zh?'收款人':'Recipient');
    await dialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click();
    await expect(cash.locator('.management-list tbody tr')).toHaveCount(1);
    await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`agent-deposit-${locale}-${width}.png`),fullPage:true}));
    await cash.locator('.management-list').getByRole('button',{name:zh?'继续充值':'Continue deposit',exact:true}).click();
    dialog=page.getByRole('dialog');
    await dialog.getByRole('button',{name:zh?'取消充值':'Cancel deposit'}).click();
    await expect(dialog).toHaveCount(0);
  });
  test(`deposit records provide continuation only for unpaid unexpired orders ${locale}`,async({page},info)=>{
    const fixture=await userFixture(page,locale);
    for(const [index,state] of ['pending','submitted','expired','confirmed','pending','cancelled'].entries())fixture.orders.push({...order('deposit','USDT','12.34'),id:`bbbbbbbb-bbbb-4bbb-8bbb-${String(index+1).padStart(12,'0')}`,state,expires_at:index===4?new Date(Date.now()-60000).toISOString():new Date(Date.now()+1800000).toISOString()});
    await page.goto(origins.user+'/recharge');
    const cash=page.locator('.currency-wallet'),records=cash.locator('.cash-records');
    await expect(records.getByRole('button',{name:zh?'继续充值':'Continue deposit',exact:true})).toHaveCount(1);
    await expect(cash.getByRole('heading',{name:zh?'充值历史':'Deposit history',exact:true})).toBeVisible();
    await expect(cash.locator('.cash-balances')).toHaveCount(0);
    await expect(records.locator('tbody tr').last().getByRole('button')).toHaveCount(0);
    await expect(records.getByRole('button',{name:zh?'详情':'Details',exact:true})).toHaveCount(5);
    expect(await records.locator('thead th').allTextContents()).toEqual(zh?['金额','类型','渠道','状态','创建时间','详情','操作']:['Amount','Type','Channel','Status','Created','Details','Actions']);
    await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`continue-deposit-${locale}-${width}.png`),fullPage:true}));
    await cash.getByRole('button',{name:zh?'充值':'Deposit',exact:true}).click();
    await expect(page.getByRole('dialog').getByLabel(zh?'金额':'Amount',{exact:true})).toBeVisible();
    await page.getByRole('dialog').getByRole('button',{name:zh?'关闭':'Close',exact:true}).click();
    await records.getByRole('button',{name:zh?'继续充值':'Continue deposit',exact:true}).click();
    await expect(page.getByRole('dialog')).toContainText('12.34');
    await expect(page.getByRole('dialog')).toContainText(address);
    expect(fixture.writes).toHaveLength(0);
  });
  test(`recharge and withdrawal lock their own exchange direction ${locale}`, async ({page},info) => {
    const fixture=await userFixture(page,locale);
    for(const mode of ['deposit','withdrawal'] as const){
      await page.goto(origins.user+(mode==='deposit'?'/recharge':'/withdraw'));
      const cash=page.locator('.currency-wallet');
      await expect(cash.locator('.cash-flow-help')).toHaveText(mode==='deposit'?(zh?'充值到账后，请点“兑换”，将余额兑换为积分后投注。':'After your deposit is credited, select Exchange to convert the balance to points for betting.'):(zh?'提现前，请点“兑换”，将积分转为可提现余额。':'Before withdrawing, select Exchange to convert points to withdrawable balance.'));
      if(mode==='deposit') await expect(cash.getByRole('button',{name:zh?'充值':'Deposit',exact:true})).toBeVisible();
      await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`cash-help-${mode}-${locale}-${width}.png`),fullPage:true}));
      await cash.getByRole('button',{name:zh?'兑换':'Exchange',exact:true}).click();
      const dialog=page.getByRole('dialog');
      const kind=dialog.getByRole('combobox',{name:zh?'类型':'Type',exact:true});
      const first=mode==='deposit'?(zh?'资金转积分':'Convert funds to points'):(zh?'积分转资金':'Convert points to currency');
      await expect(kind).toContainText(first);
      await expect(kind).toBeDisabled();
      if(mode==='withdrawal')await dialog.getByLabel(zh?'积分':'Points',{exact:true}).fill('100');
      await page.route('**/api/v1/cash/exchanges', route => route.fulfill({status:422,json:{success:false,error:{code:mode==='deposit'?'CASH_INSUFFICIENT':'POINTS_INSUFFICIENT',message:'Insufficient balance'}}}), {times:1});
      await dialog.getByRole('button',{name:zh?'确认':'Confirm',exact:true}).click();
      await expect(page.getByRole('alert').filter({hasText:zh?(mode==='deposit'?'可用资金不足，请先充值再兑换。':'可用积分不足，请先获取积分再兑换。'):(mode==='deposit'?'Insufficient available funds. Deposit before exchanging.':'Insufficient available points. Earn points before exchanging.')})).toBeVisible();
      await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`exchange-direction-${mode}-${locale}-${width}.png`),fullPage:true}));
      await dialog.getByRole('button',{name:zh?'确认':'Confirm',exact:true}).click();
      await expect(dialog).toHaveCount(0);
      expect(JSON.parse(fixture.writes.at(-1)!.body!).direction).toBe(mode==='deposit'?'to_points':'from_points');
    }
  });
  test(`exchange shows current source balance ${locale}`, async ({page}, info) => {
    await userFixture(page, locale);
    await page.goto(origins.user + '/wallet');
    await page.locator('.currency-wallet').getByRole('button', {name:zh?'兑换':'Exchange',exact:true}).click();
    const dialog=page.getByRole('dialog');
    await expect(dialog.locator('.cash-exchange-balance')).toHaveText(zh?'当前资金：12 CNY':'Current funds: 12 CNY');
    await dialog.getByRole('combobox',{name:zh?'类型':'Type',exact:true}).click();
    await page.getByRole('option',{name:zh?'积分转资金':'Convert points to currency',exact:true}).click();
    await expect(dialog.locator('.cash-exchange-balance')).toHaveText(zh?'当前积分：0':'Current points: 0');
    await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`exchange-balance-${locale}-${width}.png`),fullPage:true}));
    await dialog.getByRole('combobox',{name:zh?'类型':'Type',exact:true}).click();
    await page.getByRole('option',{name:zh?'资金转积分':'Convert funds to points',exact:true}).click();
    await dialog.getByRole('combobox',{name:zh?'币种':'Currency',exact:true}).click();
    await page.getByRole('option',{name:'USDT',exact:true}).click();
    await expect(dialog.locator('.cash-exchange-balance')).toHaveText(zh?'当前资金：123.456789 USDT':'Current funds: 123.456789 USDT');
  });
  test(`wallet exact default and frozen retry ${locale}`, async ({
    page,
  }, info) => {
    const fixture = await userFixture(page, locale);
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto(origins.user + "/recharge");
    const cash = page.locator(".currency-wallet");
    await expect(cash.locator(".cash-balance")).toHaveCount(0);
    await cash
      .getByRole("button", { name: zh ? "兑换" : "Exchange", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole('combobox',{name:zh?'币种':'Currency',exact:true}).click();
    await page.getByRole('option',{name:'USDT',exact:true}).click();
    await expect(
      dialog.getByLabel(zh ? "金额" : "Amount", { exact: true }),
    ).toHaveValue("123.456789");
    await expect(dialog).toContainText("864.19");
    fixture.loseNext();
    await dialog
      .getByRole("button", { name: zh ? "确认" : "Confirm", exact: true })
      .click();
    await expect(
      dialog.getByRole("button", {
        name: zh ? "重试原操作" : "Retry original operation",
      }),
    ).toBeVisible();
    await expect(
      dialog.getByLabel(zh ? "金额" : "Amount", { exact: true }),
    ).toBeDisabled();
    await widths(page, info.project.name.includes("reflow"), (width) =>
      page.screenshot({
        path: info.outputPath(`exchange-${locale}-${width}.png`),
        fullPage: true,
      }),
    );
    await dialog
      .getByRole("button", {
        name: zh ? "重试原操作" : "Retry original operation",
      })
      .click();
    await expect(dialog).toHaveCount(0);
    expect(fixture.writes[0]).toEqual(fixture.writes[1]);
    expect(errors).toEqual([]);
  });
  test(`USDT receipt and responsive QR ${locale}`, async ({ page }, info) => {
    const fixture = await userFixture(page, locale);
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto(origins.user + "/wallet");
    await expectNoPageOverflow(page);
    await page.screenshot({ path: info.outputPath(`deposit-entry-${locale}.png`), fullPage: true });
    await page.locator(".currency-wallet").getByRole("button", { name: zh ? "充值" : "Deposit", exact: true }).click();
    await expect(page).toHaveURL(/\/recharge$/);
    const cash = page.locator(".currency-wallet");
    await expect(cash.getByRole('button',{name:zh?'重新读取':'Reload',exact:true})).toHaveCount(0);
    await expect(cash.locator('.cash-balances')).toHaveCount(0);
    await cash
      .getByRole("button", { name: zh ? "充值" : "Deposit" })
      .click();
    let dialog = page.getByRole("dialog");
    await dialog.getByRole('combobox',{name:zh?'币种':'Currency',exact:true}).click();
    await page.getByRole('option',{name:'USDT',exact:true}).click();
    await expect(dialog).toContainText("100.03");
    await widths(page, info.project.name.includes("reflow"), width => page.screenshot({path:info.outputPath(`deposit-draft-${locale}-${width}.png`),fullPage:true}));
    await dialog.getByLabel(zh ? "金额" : "Amount", { exact: true }).fill("10");
    await dialog
      .getByRole("button", { name: zh ? "确认" : "Confirm", exact: true })
      .click();
    dialog = page.getByRole("dialog");
    await expect(dialog.getByLabel(zh ? '交易哈希' : 'Transaction hash',{exact:true})).toHaveCount(0);
    await expect(dialog.locator('.cash-transfer-guide li')).toHaveCount(4);
    await expect(dialog.locator('.cash-transfer-guide')).toContainText(zh?'核账通过':'after verification');
    await expect(dialog.locator('.cash-detail').first()).toContainText('TRON/TRC20');
    await expect(cash.locator('.cash-records')).toContainText('TRON/TRC20');
    await expect(dialog.locator(".cash-qr")).toBeVisible();
    await expect(dialog).toContainText(address);
    const dangerColor = await dialog.evaluate(el => {
      const swatch = document.createElement('span');
      swatch.style.color = 'var(--danger-text)';
      el.append(swatch);
      const color = getComputedStyle(swatch).color;
      swatch.remove();
      return color;
    });
    await expect(dialog.locator('.cash-transfer-support')).toHaveCSS('color', dangerColor);
    await expect(dialog.locator('.cash-transfer-support')).toHaveCSS('font-weight','600');
    const pagesBefore=page.context().pages().length, urlBefore=page.url();
    await dialog.getByRole('button',{name:zh?'视频教程':'Video tutorial',exact:true}).click();
    const player=page.getByRole('dialog',{name:zh?'视频教程':'Video tutorial',exact:true});
    await expect(player).toBeVisible();
    await expect(player.locator('video')).toHaveAttribute('src','/videos/buyusdt.mp4');
    await expect(player.locator('video')).toHaveJSProperty('controls',true);
    const media=await player.locator('video').elementHandle();
    await media!.evaluate(async el=>{const video=el as HTMLVideoElement;video.muted=true;await video.play()});
    await expect.poll(()=>media!.evaluate(el=>(el as HTMLVideoElement).paused)).toBe(false);
    await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`tutorial-${locale}-${width}.png`),fullPage:true}));
    await player.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click();
    await expect(player).toHaveCount(0);
    expect(await media!.evaluate(el=>(el as HTMLVideoElement).paused)).toBe(true);
    await media!.dispose();
    expect(page.context().pages()).toHaveLength(pagesBefore);
    expect(page.url()).toBe(urlBefore);
    await expect(page.getByRole('dialog')).toContainText(address);
    const video = await page.request.get(`${origins.user}/videos/buyusdt.mp4`, {
      headers: { Range: "bytes=0-1023" },
    });
    expect(video.status()).toBe(206);
    expect(video.headers()["content-type"]).toContain("video/mp4");
    expect((await video.body()).byteLength).toBe(1024);
    await widths(page, info.project.name.includes("reflow"), (width) =>
      page.screenshot({
        path: info.outputPath(`deposit-${locale}-${width}.png`),
        fullPage: true,
      }),
    );
    await dialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click();
    await cash.locator('.cash-records').getByRole('button',{name:zh?'继续充值':'Continue deposit',exact:true}).click();
    await expect(page.getByRole('dialog')).toContainText(address);
    expect(fixture.writes).toHaveLength(1);
    await page.getByRole('dialog').getByRole('button',{name:zh?'我已转账':'I have transferred',exact:true}).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    expect(JSON.parse(fixture.writes[1]!.body!)).toMatchObject({reference:'',proof:''});
    await expect(cash.getByRole('button',{name:zh?'继续充值':'Continue deposit',exact:true})).toHaveCount(0);
    await cash.locator('.cash-records').getByRole('button',{name:zh?'详情':'Details',exact:true}).click();
    await expect(page.getByRole('dialog')).toContainText(zh?'待核账':'Awaiting verification');
    expect(errors).toEqual([]);
  });
  test(`withdrawal account completion returns to draft ${locale}`, async ({
    page,
  }, info) => {
    const fixture = await userFixture(page, locale);
    await page.goto(origins.user + "/withdraw");
    const cash = page.locator(".currency-wallet");
    await cash
      .getByRole("button", { name: zh ? "提现" : "Withdrawal", exact: true })
      .click();
    let dialog = page.getByRole("dialog");
    await dialog
      .getByLabel(zh ? "金额" : "Amount", { exact: true })
      .fill("1.25");
    await dialog
      .getByRole("button", {
        name: zh ? "添加收款账号" : "Add payment account",
      })
      .click();
    await expect(page).toHaveURL(/\/account\/payment-accounts\?/);
    dialog = page.getByRole("dialog");
    await dialog
      .getByLabel(zh ? "收款人" : "Recipient", { exact: true })
      .fill("Any recipient");
    await dialog
      .getByLabel(zh ? "银行" : "Bank", { exact: true })
      .fill("Test bank");
    await dialog
      .getByLabel(zh ? "收款账号" : "Payment account", { exact: true })
      .fill("123456");
    await expect(dialog.getByRole('checkbox')).toHaveCount(0);
    await expect(dialog.getByLabel(zh ? '渠道' : 'Channel', { exact: true })).toHaveCount(0);
    await widths(page, info.project.name.includes("reflow"), width =>
      page.screenshot({ path: info.outputPath(`account-${locale}-${width}.png`), fullPage: true }),
    );
    await dialog.getByRole('button', {name: zh ? '保存' : 'Save', exact: true}).click();
    await expect(page).toHaveURL(/\/withdraw$/);
    dialog = page.getByRole("dialog");
    await expect(dialog.locator('.cash-withdrawal-account')).toContainText('123456');
    await expect(dialog.getByRole('combobox', { name: zh ? '收款账号' : 'Payment accounts', exact: true })).toHaveCount(0);
    await expect(
      dialog.getByLabel(zh ? "金额" : "Amount", { exact: true }),
    ).toHaveValue("1.25");
    for (const toast of await page.locator('.toast-close').all()) await toast.click();
    await widths(page, info.project.name.includes("reflow"), (width) =>
      page.screenshot({
        path: info.outputPath(`withdrawal-${locale}-${width}.png`),
        fullPage: true,
      }),
    );
    await dialog
      .getByRole("button", { name: zh ? "确认" : "Confirm", exact: true })
      .click();
    expect(fixture.writes.map((w) => JSON.parse(w.body!))).toContainEqual({
      currency: "CNY",
      amount: "1.25",
      destination_id: fixture.state.destinations[0].id,
      destination_version: 1,
      fee: "0",
    });
  });
  test(`brand currency settings and account editing ${locale}`, async ({
    page,
  }, info) => {
    page.on('request',request=>{expect(request.postData()??'').not.toContain('abandon')});
    await installLayoutApi(page, locale);
    let cfg = config();
    cfg.platform_rules!.find(rule => rule.currency === "USD")!.enabled = false;
    const ds = [destination("CNY"), destination()];
    const permissions = [
      "point_policy.view.brand",
      "point_policy.write.brand",
      "brand.view.brand",
    ];
    await page.route("**/api/v1/admin/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const reply = (data: unknown) =>
        route.fulfill({ json: { success: true, data } });
      if (path.endsWith("/me"))
        return reply({
          account: {
            id: gameId,
            username: "cash_reviewer",
            super_admin: false,
            brand_ids: [gameId],
            permissions: [],
            permissions_by_brand: { [gameId]: permissions },
            platform_permissions: [],
          },
        });
      if (path.endsWith("/brands"))
        return reply({
          items: [
            {
              id: gameId,
              name: "Aurora",
              code: "aurora",
              status: "active",
              member_count: "1",
              agent_count: "0",
              recharge_quota_available: "1000",
              created_at: "2026-11-11T00:00:00Z",
            },
          ],
          total_count: "1",
        });
      if (path.endsWith("/cash/config")) {
        if (route.request().method() === "PUT")
          cfg = { ...route.request().postDataJSON(), version: cfg.version + 1 };
        return reply(cfg);
      }
      if (path.endsWith("/cash/destinations")) return reply(ds);
      if (path.endsWith("/point-policy"))
        return reply({
          brand_id: gameId,
          version: 1,
          max_balance_points: null,
          max_recharge_points: null,
          max_adjustment_points: null,
        });
      return route.fallback();
    });
    await page.goto(origins.brand);
    await expect(
      page.locator(".mobile-nav:visible,.side-nav:visible").first(),
    ).toBeVisible();
    if (await page.locator(".mobile-nav").isVisible()) {
      await page.locator(".mobile-nav button").last().click();
      await page
        .locator(".mobile-more-menu")
        .getByRole("button", {
          name: zh ? "品牌设置" : "Brand settings",
          exact: true,
        })
        .click();
    } else
      await page
        .locator(".side-nav")
        .getByRole("button", {
          name: zh ? "品牌设置" : "Brand settings",
          exact: true,
        })
        .click();
    await page
      .locator(".console-tabs")
      .getByRole("button", { name: zh ? "资金" : "Funds", exact: true })
      .click();
    const console = page.locator(".cash-console");
    await expect(console.locator(".cash-rule")).toHaveCount(3);
    const firstRule = console.locator('.cash-rule').first();
    const enabled = firstRule.getByRole('checkbox');
    await enabled.uncheck();
    await console.getByRole('button', { name: zh ? '保存' : 'Save', exact: true }).click();
    await expect.poll(() => cfg.rules[0].deposit_min).toBe('0');
    expect(cfg.rules[0].inherit).toBe(true);
    await enabled.check();
    const selection = firstRule.getByRole('combobox', { name: zh ? '配置' : 'Configuration', exact: true });
    await expect(selection).toContainText(zh ? '继承' : 'Inherit');
    const inheritedFields = firstRule.locator('input[readonly]');
    await expect(inheritedFields).toHaveCount(5);
    for (const [index, value] of ['1', '10000', '1', '10000', '0'].entries()) await expect(inheritedFields.nth(index)).toHaveValue(value);
    await selection.click();
    await page.getByRole('option', { name: zh ? '自定义' : 'Custom', exact: true }).click();
    const minimum = firstRule.getByLabel(zh ? '最低充值' : 'Minimum deposit', { exact: true });
    await expect(minimum).toHaveValue('1');
    await expect(minimum).toBeEditable();
    await expect(firstRule.locator('input[readonly]')).toHaveCount(0);
    await minimum.fill('2');
    await selection.click();
    await page.getByRole('option', { name: zh ? '继承' : 'Inherit', exact: true }).click();
    await expect(minimum).toHaveValue('1');
    await expect(minimum).not.toBeEditable();
    await console.getByRole('button', { name: zh ? '保存' : 'Save', exact: true }).click();
    await expect.poll(() => cfg.rules[0].deposit_min).toBe('1');
    await expect(console.locator('.cash-tron-mode')).toHaveCount(0);
    await expect(console.getByRole('button',{name:zh?'配置收款钱包':'Configure receiving wallet',exact:true})).toHaveCount(0);
    await widths(page, info.project.name.includes("reflow"), (width) =>
      page.screenshot({
        path: info.outputPath(`brand-${locale}-${width}.png`),
        fullPage: true,
      }),
    );
    await console
      .getByRole("button", { name: zh ? "编辑" : "Edit", exact: true })
      .last()
      .click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByLabel(zh ? "收款地址" : "Receiving address", { exact: true }),
    ).toHaveValue(address);
    await expectNoPageOverflow(page);
  });
}

for (const locale of ["en", "zh-CN"] as const)
  test(`platform currency rates ${locale}`, async ({ page }, info) => {
    const zh = locale === "zh-CN";
    await installLayoutApi(page, locale);
    let cfg = config();
    let canManage = true;
    const writes: unknown[] = [];
    const queryKeys:any[]=[];const keyWrites:{body:string|null;key:string|undefined}[]=[];let loseKey=true;
    await page.route("**/api/v1/platform/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const reply = (data: unknown) =>
        route.fulfill({ json: { success: true, data } });
      if (path.endsWith("/presentation"))
        return reply({
          display_name: "Trial platform",
          logo_text: "T",
          logo_url: "",
        });
      if (path.endsWith("/me"))
        return reply({
          account: {
            id: gameId,
            username: "cash_platform_test",
            super_admin: true,
            platform_permissions: [
              "platform_settings.view.platform",
              ...(canManage ? ["platform_settings.write.platform"] : []),
            ],
          },
        });
      if (path.endsWith("/platform-settings"))
        return reply({
          version: 1,
          theme: "jade",
          display_name: "Trial platform",
          logo_text: "T",
          logo_url: "",
          timezone: "Asia/Singapore",
          date_format: "YYYY-MM-DD",
          time_format: "HH:mm:ss",
          updated_at: "2026-11-11T00:00:00Z",
        });
      if (path.endsWith("/brands")) return reply({ items: [] });
      if (path.endsWith("/cash/config")) {
        expect(route.request().headers()["x-brand-id"]).toBeUndefined();
        if (route.request().method() === "PUT") {
          writes.push(route.request().postDataJSON());
          cfg = { ...route.request().postDataJSON(), version: cfg.version + 1 };
        }
        return reply(cfg);
      }
      return route.fallback();
    });
    await page.goto(origins.platform);
    await expect(
      page.locator(".platform-mobile-nav:visible,.sidebar:visible").first(),
    ).toBeVisible();
    if (!(await page.locator(".sidebar").isVisible()))
      await page.locator(".platform-mobile-nav button").last().click();
    await page.locator(".sidebar > button.nav-item").last().click();
    await page
      .locator(".platform-settings-tabs")
      .getByRole("button", {
        name: zh ? "币种设置" : "Currency settings",
        exact: true,
      })
      .click();
    const panel = page.locator(".cash-console");
    await expect(panel.locator(".cash-rule")).toHaveCount(4);
    await panel
      .getByLabel(zh ? "每单位兑换积分" : "Points per currency unit", {
        exact: true,
      })
      .first()
      .fill("8.25");
    await panel.locator('.cash-rule').nth(1).getByRole('checkbox').uncheck();
    await panel
      .getByRole("button", { name: zh ? "保存" : "Save", exact: true })
      .click();
    await expect.poll(() => writes.length).toBe(1);
    expect(cfg.rules[1].enabled).toBe(false);
    expect(cfg.rules[1].deposit_min).toBe('0');
    expect(cfg.rules[1].rate).toBe('');
    await expect(panel.getByRole('button',{name:zh?'添加 API Key':'Add API key',exact:true})).toHaveCount(0);
    await widths(page, info.project.name.includes("reflow"), (width) =>
      page.screenshot({
        path: info.outputPath(`platform-${locale}-${width}.png`),
        fullPage: true,
      }),
    );
    if (info.project.name === "chromium-reflow") {
      canManage = false;
      await page.reload();
      await expect(page.locator(".sidebar")).toBeVisible();
      await page.locator(".sidebar > button.nav-item").last().click();
      await page
        .locator(".platform-settings-tabs")
        .getByRole("button", {
          name: zh ? "币种设置" : "Currency settings",
          exact: true,
        })
        .click();
      await expect(panel.locator(".cash-rule")).toHaveCount(4);
      await expect(panel.locator(".cash-rule input")).toHaveCount(0);
      await expect(panel).toContainText("8.25");
      await expect(
        panel.getByRole("button", { name: zh ? "保存" : "Save", exact: true }),
      ).toHaveCount(0);
      await expectNoPageOverflow(page);
    }
  });

for(const locale of ['zh-CN','en'] as const){
 test(`cash settings manual USDT only ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN';
    await installLayoutApi(page, locale);
    let cfg = config();
    cfg.rules.forEach(rule=>rule.enabled=rule.currency==='USDT');
    let enabled=false;
    const ds = [destination("CNY"), destination()];
    const permissions = [
      "point_policy.view.brand",
      "point_policy.write.brand",
      "brand.view.brand",
    ];
    await page.route("**/api/v1/admin/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const reply = (data: unknown) =>
        route.fulfill({ json: { success: true, data } });
      if (path.endsWith("/me"))
        return reply({
          account: {
            id: gameId,
            username: "cash_reviewer",
            super_admin: false,
            brand_ids: [gameId],
            permissions: [],
            permissions_by_brand: { [gameId]: permissions },
            platform_permissions: [],
          },
        });
      if (path.endsWith("/brands"))
        return reply({
          items: [
            {
              id: gameId,
              name: "Aurora",
              code: "aurora",
              status: "active",
              member_count: "1",
              agent_count: "0",
              recharge_quota_available: "1000",
              created_at: "2026-11-11T00:00:00Z",
            },
          ],
          total_count: "1",
        });
      if (path.endsWith("/cash/config")) {
        if (route.request().method() === "PUT")
          cfg = { ...route.request().postDataJSON(), version: cfg.version + 1 };
        return reply(cfg);
      }
      if (path.endsWith("/cash/destinations")) return reply(ds);
      if (path.endsWith("/point-policy"))
        return reply({
          brand_id: gameId,
          version: 1,
          max_balance_points: null,
          max_recharge_points: null,
          max_adjustment_points: null,
        });
      return route.fallback();
    });
    await page.goto(origins.brand);
    await expect(
      page.locator(".mobile-nav:visible,.side-nav:visible").first(),
    ).toBeVisible();
    if (await page.locator(".mobile-nav").isVisible()) {
      await page.locator(".mobile-nav button").last().click();
      await page
        .locator(".mobile-more-menu")
        .getByRole("button", {
          name: zh ? "品牌设置" : "Brand settings",
          exact: true,
        })
        .click();
    } else
      await page
        .locator(".side-nav")
        .getByRole("button", {
          name: zh ? "品牌设置" : "Brand settings",
          exact: true,
        })
        .click();
    await page
      .locator(".console-tabs")
      .getByRole("button", { name: zh ? "资金" : "Funds", exact: true })
      .click();
    const console = page.locator(".cash-console");
    await expect(console.locator(".cash-rule")).toHaveCount(4);

    await expect(console.locator('.cash-rule-fields')).toHaveCount(1);
    await expect(console.getByRole('combobox',{name:zh?'USDT 充值方式':'USDT deposit method',exact:true})).toHaveCount(0);
    await expect(console.getByRole('button',{name:zh?'配置收款钱包':'Configure receiving wallet',exact:true})).toHaveCount(0);
    await expect(console.getByRole('heading',{name:zh?'收款账号':'Payment accounts',exact:true})).toBeVisible();
    await expect(console).toContainText(address);
    await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`manual-only-${locale}-${width}.png`),fullPage:true}));

 });
}

for (const locale of ['en','zh-CN'] as const) for (const agent of [false,true]) test(`unique USDT amount and fee ${locale} ${agent?'agent':'user'}`,async({page},info)=>{
  const zh=locale==='zh-CN';const fixture=await userFixture(page,locale,agent);fixture.state.auto_usdt=true;fixture.state.brand.rules.forEach(r=>r.enabled=r.currency==='USDT');
  await page.goto(agent?origins.agent+'/#/recharge':origins.user+'/recharge');
  const cash=page.locator('.currency-wallet');
  await cash.getByRole('button',{name:zh?'充值':'Deposit',exact:true}).click();
  let dialog=page.getByRole('dialog');await dialog.getByLabel(zh?'金额':'Amount',{exact:true}).fill('100');
  await dialog.getByRole('button',{name:zh?'确认':'Confirm',exact:true}).click();
  dialog=page.getByRole('dialog');
  await expect(dialog).toContainText('100.000137 USDT');await expect(dialog).toContainText('0.000137 USDT');
  await expect(dialog.getByRole('button',{name:zh?'复制金额':'Copy amount',exact:true})).toBeVisible();
  await expect(dialog).toContainText(zh?'请按应付金额精确转账':'Transfer the exact amount shown');
  await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`unique-usdt-${agent}-${locale}-${width}.png`),fullPage:true}));
  // A confirmed chain event refreshes the same open order and principal balance.
  fixture.orders[0].state='confirmed';fixture.orders[0].sender=address;fixture.orders[0].version++;fixture.orders[0].credited_at=new Date().toISOString();
  fixture.state.balances[3].deposited='223.456789';
  await expect(dialog.getByText(zh?'已到账':'Credited',{exact:true})).toBeVisible({timeout:15000});
  await expect(dialog.getByRole('button',{name:zh?'我已转账':'I have transferred',exact:true})).toHaveCount(0);
  await expect(dialog).toContainText('100.000137 USDT');
  expect(fixture.writes).toHaveLength(1);
});

for (const locale of ['en','zh-CN'] as const) test(`unique USDT console receipt ${locale}`,async({page},info)=>{
 const {resolve}=await import('node:path');const zh=locale==='zh-CN';
 for(const surface of ['brand','platform'] as const){
  const origin=origins[surface],dir=surface==='brand'?'admin-web':'platform-web',scope=surface==='brand'?'admin':'platform';
  const source=await(await page.request.get(origin+'/src/App.vue')).text();const vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1];
  const o={...order('deposit','USDT','100'),automatic:true,fee:'0.000137'};
  await page.route(`**/api/v1/${scope}/cash/orders?*`,route=>route.fulfill({json:{success:true,data:{items:[o],total_count:'1'}}}));
  const props={basePath:`/api/v1/${scope}`,brandId:gameId,actorId:gameId,locale,platform:surface==='platform',mode:'deposit',permissions:[`recharge.view.${surface}`,`recharge.confirm.${surface}`]};
  const url=origin+'/__unique-console';
  await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import C from'/@fs${resolve('shared/src/CashConsole.vue')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve(dir+'/src/style.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/key-value.css')}';import'/@fs${resolve('shared/src/controls.css')}';createApp(C,${JSON.stringify(props)}).mount('#app')</script></html>`}));
  await page.goto(url);await page.getByRole('button',{name:zh?'详情':'Details',exact:true}).click();
  const dialog=page.getByRole('dialog');await expect(dialog).toContainText('100.000137 USDT');await expect(dialog).toContainText('0.000137 USDT');
  await widths(page,info.project.name.includes('reflow'),width=>page.screenshot({path:info.outputPath(`unique-console-${surface}-${locale}-${width}.png`),fullPage:true}));
  if(surface==='brand'){
   await dialog.getByRole('button',{name:zh?'确认到账':'Confirm receipt',exact:true}).click();
   await expect(dialog.getByLabel(zh?'实际到账金额':'Actual amount received',{exact:true})).toHaveValue('100.000137');
  }
 }
});

for (const locale of ['en', 'zh-CN'] as const) for (const agent of [false, true]) test(`profile payment account CRUD ${locale} ${agent ? 'agent' : 'user'}`, async ({page}, info) => {
  const zh = locale === 'zh-CN';
  const fixture = await userFixture(page, locale, agent);
  await page.goto(agent ? origins.agent + '/#/profile' : origins.user + '/account');
  await expect(page.locator('.cash-accounts')).toHaveCount(0);
  const entry = agent ? page.getByRole('button', {name:zh?'收款账号':'Payment accounts',exact:true}) : page.getByRole('link', {name:zh?'收款账号':'Payment accounts',exact:true});
  await expect(entry).toHaveCount(1);
  await entry.click();
  await expect(page).toHaveURL(agent ? /#\/payment-accounts$/ : /\/account\/payment-accounts$/);
  await expect(page.getByRole('heading', {name:zh?'收款账号':'Payment accounts',exact:true})).toBeVisible();
  const manager = page.locator('.cash-accounts');
  await expect(manager.locator('tbody tr')).toHaveCount(4);
  const row = (index: number) => manager.locator('tbody tr').nth(index);
  for (const [index, number] of [[0, '1234567890123456'], [3, address]] as const) {
    await row(index).getByRole('button', {name: zh ? '添加收款账号' : 'Add payment account', exact:true}).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('combobox')).toHaveCount(0);
    if (index === 0) {
      await dialog.getByLabel(zh ? '收款人' : 'Recipient', {exact:true}).fill('Test recipient');
      await dialog.getByLabel(zh ? '银行' : 'Bank', {exact:true}).fill('Test bank');
    } else {
      await expect(dialog).toContainText('TRON/TRC20');
      await expect(dialog.getByLabel(zh ? '收款人' : 'Recipient', {exact:true})).toHaveCount(0);
      await dialog.getByLabel(zh ? '收款地址' : 'Receiving address', {exact:true}).fill('invalid');
      await dialog.getByRole('button', {name:zh?'保存':'Save', exact:true}).click();
      expect(fixture.writes.filter(w => w.body?.includes('invalid'))).toHaveLength(0);
    }
    await dialog.getByLabel(index === 0 ? zh?'收款账号':'Payment account' : zh?'收款地址':'Receiving address', {exact:true}).fill(number);
    if (index === 0) fixture.loseNext();
    await dialog.getByRole('button', {name:zh?'保存':'Save',exact:true}).click();
    if (index === 0) {
      await expect(dialog.getByRole('button', {name:zh?'关闭':'Close', exact:true})).toBeDisabled();
      await dialog.getByRole('button', {name:zh?'重试原操作':'Retry original operation', exact:true}).click();
      expect(fixture.writes[0]).toEqual(fixture.writes[1]);
    }
    await expect(dialog).toHaveCount(0);
    await expect(row(index).getByRole('button', {name:zh?'添加收款账号':'Add payment account',exact:true})).toHaveCount(0);
    await expect(row(index)).toContainText(number.slice(-6));
    await expect(row(index)).not.toContainText(number);
  }
  for (const toast of await page.locator('.toast-close').all()) await toast.click();
  await widths(page,info.project.name.includes('reflow'),async width => {
    if (!agent) {
      const title = await page.locator('.workspace-page-title').evaluate(node => ({height:node.getBoundingClientRect().height,lineHeight:parseFloat(getComputedStyle(node).lineHeight)}));
      expect(title.height).toBeLessThanOrEqual(title.lineHeight * 2 + 1);
    }
    await page.screenshot({path:info.outputPath(`profile-accounts-${locale}-${agent}-${width}.png`),fullPage:true});
  });
  await row(0).getByRole('button', {name:zh?'编辑':'Edit',exact:true}).click();
  let dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('checkbox')).toHaveCount(0);
  await expect(dialog.getByLabel(zh?'收款账号':'Payment account',{exact:true})).toHaveValue('1234567890123456');
  await dialog.getByLabel(zh?'收款账号':'Payment account',{exact:true}).fill('888888999999');
  await dialog.getByRole('button',{name:zh?'保存':'Save',exact:true}).click();
  await expect(row(0)).toContainText('999999');
  await row(0).getByRole('button',{name:zh?'删除收款账号':'Delete payment account',exact:true}).click();
  dialog=page.getByRole('dialog');
  await dialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click();
  expect(fixture.state.destinations).toHaveLength(2);
  await row(0).getByRole('button',{name:zh?'删除收款账号':'Delete payment account',exact:true}).click();
  fixture.loseNext();
  await dialog.getByRole('button',{name:zh?'确认':'Confirm',exact:true}).click();
  await dialog.getByRole('button',{name:zh?'重试原操作':'Retry original operation',exact:true}).click();
  expect(fixture.writes.at(-1)).toEqual(fixture.writes.at(-2));
  await expect(row(0)).toContainText(zh?'未绑定':'Not bound');
  expect(fixture.state.destinations).toHaveLength(1);
  await expect(row(3)).toContainText(address.slice(-6));
});

for (const locale of ['en', 'zh-CN'] as const) test(`agent withdrawal account returns to draft ${locale}`, async ({page}) => {
  const zh=locale==='zh-CN';const fixture=await userFixture(page,locale,true);
  await page.goto(origins.agent+'/#/withdrawal');
  await page.locator('.currency-wallet').getByRole('button',{name:zh?'提现':'Withdrawal',exact:true}).click();
  let dialog=page.getByRole('dialog');
  await dialog.getByLabel(zh?'金额':'Amount',{exact:true}).fill('1.25');
  await expect(dialog.getByRole('button',{name:zh?'确认':'Confirm',exact:true})).toBeDisabled();
  await dialog.getByRole('button',{name:zh?'添加收款账号':'Add payment account',exact:true}).click();
  await expect(page).toHaveURL(/#\/payment-accounts\?/);
  dialog=page.getByRole('dialog');
  await dialog.getByLabel(zh?'收款人':'Recipient',{exact:true}).fill('Agent');
  await dialog.getByLabel(zh?'银行':'Bank',{exact:true}).fill('Bank');
  await dialog.getByLabel(zh?'收款账号':'Payment account',{exact:true}).fill('123456');
  await dialog.getByRole('button',{name:zh?'保存':'Save',exact:true}).click();
  await expect(page).toHaveURL(/#\/withdrawal$/);
  dialog=page.getByRole('dialog');
  await expect(dialog.getByLabel(zh?'金额':'Amount',{exact:true})).toHaveValue('1.25');
  await expect(dialog.locator('.cash-withdrawal-account')).toContainText('123456');
  expect(fixture.writes).toHaveLength(1);
});
for (const locale of ['en', 'zh-CN'] as const) test(`withdrawal shows only matching currency account ${locale}`, async ({page},info) => {
  const zh=locale==='zh-CN';const fixture=await userFixture(page,locale);
  fixture.state.destinations.push({...destination('CNY'),id:'aaaaaaaa-aaaa-4aaa-8aaa-000000000002'},destination('USDT'));
  await page.goto(origins.user+'/withdraw');
  await expect(page.locator('.cash-account-list')).toHaveCount(0);
  await page.locator('.currency-wallet').getByRole('button',{name:zh?'提现':'Withdrawal',exact:true}).click();
  const dialog=page.getByRole('dialog');
  await expect(dialog.locator('.cash-withdrawal-account')).toContainText('12345678901234567890');
  for(const currency of ['USDT','THB','CNY']){
    await dialog.getByRole('combobox',{name:zh?'币种':'Currency',exact:true}).click();
    await page.getByRole('option',{name:currency==='THB'?zh?'泰铢':'THB':currency==='CNY'?zh?'人民币':'CNY':currency,exact:true}).click();
    if(currency==='THB'){
      await expect(dialog.locator('.cash-withdrawal-account')).toHaveCount(0);
      await expect(dialog.getByRole('button',{name:zh?'确认':'Confirm',exact:true})).toBeDisabled();
    }else{
      await expect(dialog.locator('.cash-withdrawal-account')).toContainText(currency==='USDT'?address:'12345678901234567890');
      await expect(dialog.getByRole('button',{name:zh?'确认':'Confirm',exact:true})).toBeEnabled();
    }
  }
  if(info.project.name.includes('reflow')) await page.setViewportSize({width:320,height:800});
  await expectNoPageOverflow(page);
  expect(fixture.writes).toHaveLength(0);
});
