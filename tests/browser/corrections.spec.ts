import { test, expect, type APIRequestContext, type Page, type Route } from "@playwright/test";
import type { Wallet, LedgerEntry } from "../../admin-web/src/finance-api";
import type { Period } from "../../admin-web/src/period-schedules-api";
import type { SettlementJob, SettlementTarget } from "../../admin-web/src/settlement-job-api";
import { rememberAdminSession } from "./support/admin-session";
import { formatDateTimeLocal } from "./support/datetime-local";

const brandId = "0199a000-0000-7000-8000-000000000002";
const origin = process.env.TEST_PUBLIC_ORIGIN ?? "http://localhost:5173";
const adminOrigin = process.env.TEST_ADMIN_ORIGIN ?? "http://localhost:5174";
const apiOrigin = process.env.TEST_API_ORIGIN ?? "http://127.0.0.1:8080";
const harborOrigin = process.env.TEST_USER_ORIGIN ?? "http://harbor.localhost:5173";
const adminBase = `${origin}/api/v1/admin`;
const publicBase = `${origin}/api/v1/b/harbor`;
const key = () => crypto.randomUUID();
const unique = () => crypto.randomUUID().replaceAll("-", "").slice(0, 12);
type Envelope<T> = { success: boolean; data: T; error?: unknown };

async function api<T>(
  request: APIRequestContext,
  path: string,
  method: "GET" | "POST" | "PUT",
  token: string,
  body?: unknown,
  status = 200,
) {
  const response = await request.fetch(`${adminBase}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      "X-Brand-ID": brandId,
      Origin: origin,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      ...(method === "GET" ? {} : { "Idempotency-Key": key() }),
    },
    ...(body === undefined ? {} : { data: body }),
  });
  const text = await response.text();
  expect(response.status(), `${method} ${path}: ${text}`).toBe(status);
  const envelope = JSON.parse(text) as Envelope<T>;
  expect(envelope.success, `${method} ${path}: ${text}`).toBe(true);
  return envelope.data;
}

async function login(request: APIRequestContext, username: string, password: string) {
  const response = await request.post(`${adminBase}/auth/login`, {
    headers: { "X-Brand-ID": brandId, Origin: origin, "Idempotency-Key": key() },
    data: { identifier: username, password },
  });
  const text = await response.text();
  expect(response.status(), `admin login: ${text}`).toBe(200);
  const envelope = JSON.parse(text) as Envelope<{ access_token: string }>;
  expect(envelope.success).toBe(true);
  return envelope.data.access_token;
}

function captchaAnswer(svg: string) {
  return svg.match(/<text\b[^>]*>([^<]+)<\/text>/)?.[1] ?? "";
}

async function publicApi<T>(request: APIRequestContext, path: string, token?: string, body?: unknown, status = 200) {
  const response = await request.fetch(`${publicBase}${path}`, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      Origin: origin,
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body === undefined ? {} : { "Content-Type": "application/json", "Idempotency-Key": key() }),
    },
    ...(body === undefined ? {} : { data: body }),
  });
  const text = await response.text();
  expect(response.status(), `${path}: ${text}`).toBe(status);
  const envelope = JSON.parse(text) as Envelope<T>;
  expect(envelope.success, `${path}: ${text}`).toBe(true);
  return envelope.data;
}

async function goToPeriods(page: Page, projectName: string) {
  if (projectName === "mobile") await page.locator(".mobile-nav button").nth(2).click();
  else await page.locator(".side-nav").getByRole("button", { name: /期次和开奖/ }).click();
}

test("real Harbor winning settlement can be corrected, reversed, and manually resettled", async ({ page, context }, info) => {
  test.setTimeout(65_000);
  page.setDefaultTimeout(8_000);
  const pageErrors:string[]=[];
  page.on("pageerror",e=>pageErrors.push(e.message));
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME || !process.env.TEST_HARBOR_ADMIN_PASSWORD ||
      !process.env.TEST_RULE_REVIEWER_USERNAME || !process.env.TEST_RULE_REVIEWER_PASSWORD,
    "Provide the isolated Harbor creator and distinct rule reviewer credentials",
  );

  const creator = await login(page.request, process.env.TEST_HARBOR_ADMIN_USERNAME!, process.env.TEST_HARBOR_ADMIN_PASSWORD!);
  const creatorCookies = (await context.cookies(`${adminBase}/me`)).filter(cookie => cookie.name === "lottery_admin");
  expect(creatorCookies).toHaveLength(1);
  rememberAdminSession(process.env.TEST_HARBOR_ADMIN_USERNAME!,creatorCookies,adminOrigin);
  const reviewer = await login(page.request, process.env.TEST_RULE_REVIEWER_USERNAME!, process.env.TEST_RULE_REVIEWER_PASSWORD!);
  const reviewerCookies = (await context.cookies(`${adminBase}/me`)).filter(cookie => cookie.name === "lottery_admin");
  expect(reviewerCookies).toHaveLength(1);
  rememberAdminSession(process.env.TEST_RULE_REVIEWER_USERNAME!, reviewerCookies,adminOrigin);

  const authContext = await publicApi<{ auth?: { captcha_enabled?: boolean } }>(page.request, "/context");
  const username = `corr_${unique()}`;
  const password = `real-correction-${unique()}-Pass9`;
  let captcha: Record<string, string> = {};
  if (authContext.auth?.captcha_enabled) {
    const challenge = await publicApi<{ id: string; svg: string }>(page.request, "/auth/challenge");
    const answer = captchaAnswer(challenge.svg);
    expect(answer).toMatch(/^[A-Za-z0-9]+$/);
    captcha = { captcha_id: challenge.id, captcha_answer: answer };
  }
  const registered = await page.request.post(`${publicBase}/auth/register`, {
    headers: { Origin: origin, "Idempotency-Key": key() },
    data: { username, password, privacy_policy_version: "dev-1", service_terms_version: "dev-1", ...captcha },
  });
  const registrationText = await registered.text();
  expect(registered.status(), registrationText).toBe(201);
  const registration = JSON.parse(registrationText) as Envelope<{ access_token: string; member: { id: string } }>;
  expect(registration.success).toBe(true);
  const userToken = registration.data.access_token;
  const memberId = registration.data.member.id;

  const recharge = await api<{ id: string; version: number }>(page.request, "/recharges", "POST", creator, {
    member_id: memberId, points: "100", proof_reference: `corr-${unique()}`, remark: "correction smoke funding", reason: "fund isolated correction member",
  }, 201);
  await api(page.request, `/recharges/${recharge.id}/confirm`, "POST", creator, { version: recharge.version, reason: "confirm correction smoke funding" });

  const model = {
    model: "DIGITS_0_9", regular_pool: { min: 0, max: 0, values: [], allow_repeat: false },
    special_pool: { min: 0, max: 0, values: [], allow_repeat: false }, regular_count: 0, special_count: 0,
    pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true,
  };
  const game = await api<{ id: string; version: number }>(page.request, "/games", "POST", creator, {
    code: `e2e_corr_${unique()}`, name: "Correction end to end smoke", model, timezone: "UTC", reason: "real correction API and UI smoke",
  }, 201);
  const play = await api<{ id: string }>(page.request, `/games/${game.id}/plays`, "POST", creator, {
    code: "position_match", name: "Position match", reason: "correction smoke play",
  }, 201);
  const definition = {
    schema_version: 1, model,
    selection: { mode: "numbers", regular_count: 0, special_count: 0, exclude_count: 0, attribute_groups: [], feature_choices: {} },
    number_attributes: {}, unit_points: "1",
    prize_tiers: [{ code: "WIN_POSITION_MATCH", condition: { op: "equals", field: "position_match", value: 3 }, odds: "10", exclusive: true, cap_points: null }],
    mixed_tier_policy: "max_all", cap_points: null, rounding: "half_up", rounding_scope: "order",
    limits: { max_combinations: 1000, max_multiplier: "1000", max_bet_points: null },
  };
  const rule = await api<{ id: string; version: number }>(page.request, "/rule-versions", "POST", creator, {
    play_id: play.id, definition, effect_mode: "immediate", reason: "create correction smoke rule",
  }, 201);
  const validation = await api<{ version: number; validation?: { passed: boolean } }>(page.request, `/rule-versions/${rule.id}/validate`, "POST", creator, {
    version: rule.version,
    cases: [{ name: "three positional matches", selection: { regular: [], special: [], digits: [[1], [2], [1]], exclude: [], attributes: {}, features: {} }, draw: { regular: [], special: [], digits: [1, 2, 1] }, multiplier: "1", expected_bet_points: "1", expected_prize_points: "10", expected_won: true }],
    reason: "validate exact winning case",
  });
  expect(validation.validation?.passed).toBe(true);
  const submitted = await api<{ version: number }>(page.request, `/rule-versions/${rule.id}/submit-review`, "POST", creator, { version: validation.version, reason: "request distinct reviewer approval" });
  await api(page.request, `/rule-versions/${rule.id}/approve`, "POST", reviewer, { version: submitted.version, reason: "approve correction smoke rule", warnings_acknowledged: true });

  const policy = await api<{ version: number; mode: string | null }>(page.request, "/settlement-policy", "GET", creator);
  const automatic = await api<{ version: number; mode: string }>(page.request, "/settlement-policy", "PUT", creator, { version: policy.version, mode: "automatic", reason: "enable automatic initial correction smoke settlement" });
  expect(automatic).toMatchObject({ version: policy.version + 1, mode: "automatic" });

  const drawAt = new Date(Date.now() + 12_000);
  drawAt.setUTCMilliseconds(0);
  await api(page.request, `/games/${game.id}/schedule`, "PUT", creator, {
    version: game.version, reason: "open a short UTC correction smoke window",
    spec: { timezone: "UTC", mode: "daily", daily_draw_times: [drawAt.toISOString().slice(11, 19)], interval_seconds: 0, busy_windows: [], bet_open_before_seconds: 60, bet_close_before_seconds: 2, pause_dates: [], weekdays: [0, 1, 2, 3, 4, 5, 6], holiday_dates: [], holiday_policy: "normal" },
  });
  const generated = await api<{ periods: Period[] }>(page.request, `/games/${game.id}/periods/generate`, "POST", creator, { from: new Date(Date.now() - 1_000).toISOString(), to: new Date(Date.now() + 60_000).toISOString(), reason: "generate correction smoke window" });
  expect(generated.periods).toHaveLength(1);
  const periodId = generated.periods[0].id;
  await expect.poll(async () => (await api<Period>(page.request, `/periods/${periodId}`, "GET", creator)).status, { timeout: 8_000 }).toBe("betting");
  const catalog = await publicApi<{ period: { id: string; status: string } | null; plays: Array<{ id: string; rule_version_id: string }>; policy_versions: { brand: number; game: number } }>(page.request, `/games/${game.id}`, userToken);
  expect(catalog.period?.id).toBe(periodId);
  const betInput = { period_id: periodId, play_id: play.id, rule_version_id: catalog.plays[0].rule_version_id, selection: { regular: [], special: [], digits: [[1], [2], [1]], exclude: [], attributes: {}, features: {} }, multiplier: "1", policy_versions: catalog.policy_versions };
  const quote = await publicApi<{ actor_context: string }>(page.request, "/bet-previews", userToken, betInput);
  const order = await publicApi<{ id: string }>(page.request, "/bet-orders", userToken, { ...betInput, actor_context: quote.actor_context }, 201);
  await expect.poll(async () => (await api<Period>(page.request, `/periods/${periodId}`, "GET", creator)).status, { timeout: 22_000 }).toBe("waiting_draw");
  const beforeDraw = await api<Period>(page.request, `/periods/${periodId}`, "GET", creator);
  const draw = await api<{ id: string }>(page.request, `/periods/${periodId}/manual-draw`, "POST", creator, {
    version: beforeDraw.version, period_no: beforeDraw.period_no, result: { regular: [], special: [], digits: [1, 2, 1] }, drawn_at: drawAt.toISOString(), reason: "record actual winning correction smoke draw",
  }, 201);
  const settlementContext = await api<{period_version:number;policy_version:number;draw_result_id:string}>(page.request,`/periods/${periodId}/settlement-context`,"GET",creator);
  await api(page.request,`/periods/${periodId}/settle`,"POST",creator,{version:settlementContext.period_version,policy_version:settlementContext.policy_version,draw_result_id:settlementContext.draw_result_id,reason:"explicitly start original winning settlement"},201);
  const originalDrawRecord = await publicApi<{ item: { id: string; result: { regular: number[]; special: number[]; digits: number[] } } }>(page.request, `/draw-results/${draw.id}`);
  expect(originalDrawRecord.item).toMatchObject({ id: draw.id, result: { regular: [], special: [], digits: [1, 2, 1] } });
  await expect.poll(async () => (await api<{ settlement: { state: string } | null }>(page.request, `/periods/${periodId}/settlement`, "GET", creator)).settlement?.state, { timeout: 12_000 }).toBe("completed");

  const initialWallet = await publicApi<Wallet>(page.request, "/wallet", userToken);
  expect(initialWallet.available_points).toBe("109");
  expect(initialWallet.by_source.recharge.available).toBe("99");
  expect(initialWallet.by_source.winning.available).toBe("10");
  const ledgerBefore = await api<{ items: LedgerEntry[] }>(page.request, `/wallets/${memberId}/ledger?limit=100`, "GET", creator);
  const paidOrder = await api<{id:string;settlement_calculation_id:string;payout_entry_id:string}>(page.request,`/bet-orders/${order.id}`,"GET",creator);
  const originalPrize = ledgerBefore.items.find(entry => entry.entry_type === "prize" && entry.id === paidOrder.payout_entry_id && entry.reference_id === paidOrder.settlement_calculation_id);
  expect(originalPrize).toBeTruthy();
  const originalJob = (await api<{ settlement: SettlementJob }>(page.request, `/periods/${periodId}/settlement`, "GET", creator)).settlement;
  const originalAdminHistory=await api<{history:Array<{id:string;result:unknown}>}>(page.request,`/periods/${periodId}/draw`,"GET",creator);
  const originalAdminDraw=originalAdminHistory.history.find(v=>v.id===draw.id);
  expect(originalAdminDraw).toBeTruthy();
  expect(originalJob).toMatchObject({ current: true, state: "completed", generation: 1, prize_points: "10", paid_points: "10" });
  const manualPolicy = await api<{ version: number; mode: string }>(page.request, "/settlement-policy", "PUT", creator, { version: automatic.version, mode: "manual", reason: "require approval for correction child settlement" });
  expect(manualPolicy).toMatchObject({ version: automatic.version + 1, mode: "manual" });

  await context.addCookies(creatorCookies);
  const adminPage = await context.newPage();
  adminPage.on("pageerror",e=>pageErrors.push(e.message));
  adminPage.setDefaultTimeout(8_000);
  await adminPage.goto(adminOrigin);
  await adminPage.getByTestId("admin-language").selectOption("zh-CN");
  await adminPage.getByLabel("选择真实后台品牌", { exact: true }).selectOption(brandId);
  await goToPeriods(adminPage, info.project.name);
  const correction = adminPage.locator(".correction-management");
  await correction.getByLabel("期次 UUID", { exact: true }).fill(periodId);
  await expect(correction).toContainText(draw.id);
  await correction.getByLabel("数字结果（逗号分隔）", { exact: true }).fill("1,2,2");
  await correction.getByLabel("操作原因（UTF-8 不超过 500 字节）", { exact: true }).fill("Correct the recorded third digit after source reconciliation.");
  await correction.getByRole("button", { name: "核对并更正、重新结算", exact: true }).click();
  await correction.locator(".cm-confirm").getByRole("checkbox").check();

  const correctionRequests: Array<{ key: string; body: string | null }> = [];
  let correctionReceipt: Record<string, unknown> | undefined;
  const correctRoute = `**/draw-results/${draw.id}/correct`;
  await adminPage.route(correctRoute, async (route: Route) => {
    if (route.request().method() !== "POST") { await route.continue(); return; }
    correctionRequests.push({ key: route.request().headers()["idempotency-key"] ?? "", body: route.request().postData() });
    // Forward the real request directly to the local API, preserving browser
    // Host/Origin/Cookie. Avoid a second connection through the dev proxy after
    // intentionally aborting the first browser response.
    const response = await route.fetch({
      url: route.request().url().replace(adminOrigin, apiOrigin),
      headers: { ...(await route.request().allHeaders()), host: new URL(adminOrigin).host },
    });
    expect(response.status(), await response.text()).toBe(201);
    const envelope = await response.json() as Envelope<Record<string, unknown>>;
    expect(envelope.success).toBe(true);
    correctionReceipt = envelope.data;
    if (correctionRequests.length === 1) await route.abort("failed");
    else await route.fulfill({ response });
  });
  await correction.getByRole("button", { name: "确认提交", exact: true }).click();
  await expect(correction.locator(".cm-pending")).toContainText("写入结果未知");
  await correction.getByRole("button", { name: "只读刷新", exact: true }).click();
  await expect(correction.locator(".cm-pending")).toContainText("写入结果未知");
  const correctionId = correctionReceipt!.id as string;
  await expect(correction.getByRole("region", { name: "更正详情", exact: true })).toContainText(correctionId);
  await expect(correction.getByRole("status")).toContainText("存在结果未知的写入。");

  // Receipt acknowledgement precedes follow-up reads. Hold a real detail
  // response to prove the intent clears while refresh remains disabled, then
  // await the real history response rather than racing a still-busy button.
  let releaseDetail!: () => void;
  let detailReached!: (status: number) => void;
  const heldDetail = new Promise<void>(resolve => { releaseDetail = resolve; });
  const detailResponseReached = new Promise<number>(resolve => { detailReached = resolve; });
  const detailRoute = `**/api/v1/admin/corrections/${correctionId}`;
  await adminPage.route(detailRoute, async route => {
    const response = await route.fetch();
    detailReached(response.status());
    await heldDetail;
    await route.fulfill({ response });
  }, { times: 1 });
  // The overall 65-second test deadline bounds this observation. A read is not
  // required to finish within the locator's shorter click deadline.
  const historyAfterReceipt = adminPage.waitForResponse(response =>
    response.request().method() === "GET" &&
    new URL(response.url()).pathname === `/api/v1/admin/periods/${periodId}/corrections`,
    { timeout: 0 },
  );
  try {
    await correction.getByRole("button", { name: "原样重试同一请求", exact: true }).click();
    await expect(correction.locator(".cm-pending")).toHaveCount(0);
    expect(await detailResponseReached).toBe(200);
    await expect(correction.getByRole("button", { name: "只读刷新", exact: true })).toBeDisabled();
  } finally {
    releaseDetail();
  }
  const refreshedHistory = await historyAfterReceipt;
  expect(refreshedHistory.status()).toBe(200);
  expect(await refreshedHistory.finished()).toBeNull();
  await expect(correction.getByRole("status")).toContainText("更正已确认，并已读取最新服务器状态。");
  await expect(correction.getByRole("button", { name: "只读刷新", exact: true })).toBeEnabled();
  await adminPage.unroute(detailRoute);
  expect(correctionRequests).toHaveLength(2);
  expect(correctionRequests[0]).toEqual(correctionRequests[1]);
  await adminPage.unroute(correctRoute);
  expect(correctionReceipt).toMatchObject({ previous_job_id: originalJob.id, result: { regular: [], special: [], digits: [1, 2, 2] }, state: "reversing" });
  expect(correctionReceipt!.previous_draw_result_id).toBe(draw.id);
  expect(correctionReceipt!.draw_result_id).not.toBe(draw.id);
  type CorrectionData={id:string;state:string;new_job_id:string|null;new_job_state:string|null};
  await expect.poll(async () => (await api<CorrectionData>(page.request, `/corrections/${correctionId}`, "GET", creator)).new_job_state, { timeout: 10_000 }).toBe("awaiting_approval");
  const correctionData=await api<CorrectionData>(page.request,`/corrections/${correctionId}`,"GET",creator);
  expect(correctionData.state).toBe("resettling");expect(correctionData.new_job_id).not.toBeNull();
  await correction.getByRole("button",{name:"只读刷新",exact:true}).click();

  const afterCorrectionWallet = await publicApi<Wallet>(page.request, "/wallet", userToken);
  expect(afterCorrectionWallet.available_points).toBe("99");
  const oldJobAfter = await api<SettlementJob>(page.request, `/settlement-jobs/${originalJob.id}`, "GET", creator);
  expect(oldJobAfter).toMatchObject({
    id: originalJob.id, current: false, state: originalJob.state, generation: originalJob.generation,
    draw_result_id: originalJob.draw_result_id, period_version: originalJob.period_version,
    policy_version: originalJob.policy_version, mode: originalJob.mode, target_count: originalJob.target_count,
    prize_points: originalJob.prize_points, paid_points: originalJob.paid_points, created_by: originalJob.created_by,
    created_at: originalJob.created_at, reason: originalJob.reason,
  });
  const afterAdminHistory=await api<typeof originalAdminHistory>(page.request,`/periods/${periodId}/draw`,"GET",creator);
  expect(afterAdminHistory.history.find(v=>v.id===draw.id)).toEqual(originalAdminDraw);
  const history = await api<{ items: Array<{ id: string; state: string }> }>(page.request, `/periods/${periodId}/corrections?limit=20&offset=0`, "GET", creator);
  expect(history.items.some(item => item.id === correctionId)).toBe(true);

  await correction.getByRole("button", { name: "打开新结算任务", exact: true }).click();
  const settlement = adminPage.locator(".settlement-management");
  await expect(settlement).toContainText("第 2 代");
  await expect(settlement).toContainText("待运营批准");
  const childJob = await api<{ settlement: { id: string; generation: number; state: string; current: boolean; prize_points: string } }>(page.request, `/periods/${periodId}/settlement`, "GET", creator);
  expect(childJob.settlement).toMatchObject({ generation: 2, state: "awaiting_approval", current: true, prize_points: "0" });
  const childTargets = await api<{ items: SettlementTarget[] }>(page.request, `/settlement-jobs/${childJob.settlement.id}/targets?limit=20&offset=0`, "GET", creator);
  expect(childTargets.items).toHaveLength(1);
  expect(childTargets.items[0]).toMatchObject({ order_id: order.id, won: false, prize_points: "0", payout_entry_id: null });
  await settlement.getByLabel("运营批准原因（UTF-8 不超过 500 字节）", { exact: true }).fill("Approve the corrected losing result after ledger reversal.");
  await settlement.getByRole("button", { name: "核对并批准，允许 worker 入账", exact: true }).click();
  await settlement.locator(".sm-confirm").getByRole("checkbox").check();
  await settlement.getByRole("button", { name: "确认提交结算操作", exact: true }).click();
  await expect.poll(async () => (await api<{ state: string }>(page.request, `/settlement-jobs/${childJob.settlement.id}`, "GET", creator)).state, { timeout: 15_000 }).toBe("completed");
  expect((await api<CorrectionData>(page.request,`/corrections/${correctionId}`,"GET",creator)).state).toBe("completed");
  expect(await api<{status:string;prize_points:string}>(page.request,`/bet-orders/${order.id}`,"GET",creator)).toMatchObject({status:"lost",prize_points:"0"});

  const finalJob = await api<{ id: string; generation: number; current: boolean; state: string; prize_points: string; paid_points: string }>(page.request, `/settlement-jobs/${childJob.settlement.id}`, "GET", creator);
  expect(finalJob).toMatchObject({ generation: 2, current: true, state: "completed", prize_points: "0", paid_points: "0" });
  const finalWallet = await publicApi<Wallet>(page.request, "/wallet", userToken);
  expect(finalWallet.available_points).toBe("99");
  expect(finalWallet.by_source.recharge.available).toBe("99");
  expect(finalWallet.by_source.winning.available).toBe("0");
  const ledgerAfter = await api<{ items: LedgerEntry[] }>(page.request, `/wallets/${memberId}/ledger?limit=100`, "GET", creator);
  const prizeEntries = ledgerAfter.items.filter(entry => entry.entry_type === "prize" && entry.reference_id === paidOrder.settlement_calculation_id);
  const reversals = ledgerAfter.items.filter(entry => entry.entry_type === "prize_reversal" && entry.reference_id === correctionId);
  expect(prizeEntries).toHaveLength(1);
  expect(reversals).toHaveLength(1);
  expect(reversals[0].reversal_of).toBe(originalPrize!.id);
  expect(ledgerAfter.items.filter(entry => entry.entry_type === "prize")).toHaveLength(1);
  expect(await api<{ id: string; current: boolean }>(page.request, `/settlement-jobs/${originalJob.id}`, "GET", creator)).toMatchObject({ id: originalJob.id, current: false });
  await correction.getByRole("button",{name:"只读刷新",exact:true}).click();
  await expect(correction).toContainText("更正完成");
  expect(pageErrors).toEqual([]);
  await correction.screenshot({path:info.outputPath("s5-c3-correction-panel.png")});
  await adminPage.evaluate(() => window.scrollTo(0, 0));
  const dimensions = await adminPage.evaluate(() => ({ documentWidth: document.documentElement.scrollWidth, viewportWidth: document.documentElement.clientWidth }));
  expect(dimensions.documentWidth).toBeLessThanOrEqual(dimensions.viewportWidth);
  await adminPage.screenshot({ path: info.outputPath("s5-c3-correction.png"), fullPage: true });

  // The inbox records actual wallet postings, not calculation previews or the
  // current order projection (which is now lost with zero winning balance).
  type Message = {id:string;event_type:string;payload:{resource_id:string;points:string|null}};
  await expect.poll(async()=>{
    const inbox=await publicApi<{items:Message[]}>(page.request,"/notifications?limit=100",userToken);
    return inbox.items.filter(n=>["bet.order.won","bet.order.prize_reversed"].includes(n.event_type)).length;
  }).toBe(2);
  const inboxFacts=await publicApi<{items:Message[]}>(page.request,"/notifications?limit=100",userToken);
  const prizeFacts=inboxFacts.items.filter(n=>["bet.order.won","bet.order.prize_reversed"].includes(n.event_type));
  expect(prizeFacts.map(n=>n.event_type).sort()).toEqual(["bet.order.prize_reversed","bet.order.won"]);
  for(const fact of prizeFacts) expect(fact.payload).toEqual({resource_id:order.id,points:"10"});

  const userCookies=(await context.cookies(publicBase)).filter(c=>c.name===`lottery_user_${brandId.replaceAll("-","")}`);
  expect(userCookies).toHaveLength(1);
  await context.addCookies(userCookies.map(c=>({...c,domain:"harbor.localhost"})));
  await page.route(`${harborOrigin}/api/**`,async route=>{
    const response=await route.fetch({url:route.request().url().replace("harbor.localhost","localhost"),headers:{...(await route.request().allHeaders()),host:new URL(harborOrigin).host}});
    await route.fulfill({response});
  });
  await page.goto(`${harborOrigin}/notifications`);
  const inboxPanel=page.locator(".notifications-panel");
  await expect(inboxPanel.getByRole("heading",{name:"Prize credit recorded",exact:true})).toBeVisible();
  await expect(inboxPanel.getByRole("heading",{name:"Prize reversal recorded",exact:true})).toBeVisible();
  await expect(inboxPanel).toContainText("not your current wallet balance");
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await inboxPanel.screenshot({path:info.outputPath("s6-c-prize-inbox.png")});

  await adminPage.getByRole("button",{name:"通知",exact:true}).click();
  const deliveries=adminPage.locator(".notification-deliveries");
  await expect(deliveries.getByRole("heading",{name:"通知投递记录",exact:true})).toBeVisible();
  await expect(deliveries.locator(".status-sent").first()).toBeVisible();
  await deliveries.getByRole("button",{name:"只读刷新通知投递记录",exact:true}).click();
  await expect(deliveries.locator(".status-sent").first()).toBeVisible();
  expect(await adminPage.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await deliveries.screenshot({path:info.outputPath("s6-c-deliveries.png")});

  if(info.project.name==="mobile") {
    await adminPage.locator(".mobile-nav button").nth(4).click();
    await adminPage.locator(".mobile-more-menu").getByRole("button",{name:/报表和对账/}).click();
  } else await adminPage.locator(".side-nav").getByRole("button",{name:/报表和对账/}).click();
  const reports=adminPage.locator(".reports-management");
  await expect(reports.getByRole("heading",{name:"运营报表",exact:true})).toBeVisible();
  const reportNow=await adminPage.evaluate(()=>Date.now());
  const reportWindow={from:await adminPage.evaluate(formatDateTimeLocal,reportNow-3600000),to:await adminPage.evaluate(formatDateTimeLocal,reportNow+3600000)};
  await reports.getByLabel("报表开始时间",{exact:true}).fill(reportWindow.from);
  await reports.getByLabel("报表结束时间",{exact:true}).fill(reportWindow.to);
  await reports.getByLabel("会员筛选 UUID",{exact:true}).fill(memberId);
  await reports.getByLabel("彩种筛选 UUID",{exact:true}).fill(game.id);
  await reports.getByLabel("投注分组",{exact:true}).selectOption("game");
  await reports.getByRole("button",{name:"查询报表",exact:true}).click();
  const betReport=reports.locator(".reports-panel").filter({has:adminPage.getByRole("heading",{name:"投注报表",exact:true})});
  await expect(betReport.locator(".reports-summary>div").filter({hasText:"当前最终代次奖金"}).locator("strong")).toHaveText("0");
  await expect(betReport.locator(".reports-summary>div").filter({hasText:"注单数"}).locator("strong")).toHaveText("1");
  await expect(betReport.locator(".reports-summary>div").filter({hasText:"已结算投注"}).locator("strong")).toHaveText("1");
  await expect(betReport.locator(".reports-context")).toContainText(game.id);
  expect(await adminPage.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await betReport.screenshot({path:info.outputPath("s6-d-betting-report.png")});
  // Draft changes do not silently alter the displayed cohort or a read-only refresh.
  await reports.getByLabel("彩种筛选 UUID",{exact:true}).fill(brandId);
  await reports.getByRole("button",{name:"只读刷新报表",exact:true}).click();
  await expect(betReport.locator(".reports-context")).toContainText(game.id);
  await reports.getByLabel("彩种筛选 UUID",{exact:true}).fill(game.id);
  await reports.getByRole("button",{name:"账本报表",exact:true}).click();
  await reports.getByLabel("账本分组",{exact:true}).selectOption("entry_type");
  await reports.getByRole("button",{name:"查询报表",exact:true}).click();
  const ledgerReport=reports.locator(".reports-panel").filter({has:adminPage.getByRole("heading",{name:"账本报表",exact:true})});
  for(const [label,value] of [["派奖入账","10"],["派奖冲正","10"],["净变动","99"]]) {
    await expect(ledgerReport.locator(".reports-summary>div").filter({has:adminPage.locator("span").filter({hasText:new RegExp(`^${label}$`)})}).locator("strong")).toHaveText(value);
  }
  await expect(ledgerReport.locator(".reports-balance")).toContainText("99");
  expect(await adminPage.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await ledgerReport.screenshot({path:info.outputPath("s6-d-ledger-report.png")});
  expect(await publicApi<Wallet>(page.request,"/wallet",userToken)).toEqual(finalWallet);
  expect(await api<{items:LedgerEntry[]}>(page.request,`/wallets/${memberId}/ledger?limit=100`,"GET",creator)).toEqual(ledgerAfter);
  expect(pageErrors).toEqual([]);
  // Drain the host-preserving real fetch callback before fixture teardown. The
  // user shell can refresh /me concurrently when another tab restores a session.
  await page.unrouteAll({behavior:"wait"});
});
