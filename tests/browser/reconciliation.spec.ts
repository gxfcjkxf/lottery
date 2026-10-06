import { test, expect, type APIRequestContext, type Page } from "@playwright/test";
import { rememberAdminSession } from "./support/admin-session";

const brand = "0199a000-0000-7000-8000-000000000002";
const origin = "http://localhost:5174";
const admin = `${origin}/api/v1/admin`;
const username = process.env.TEST_RECONCILIATION_ADMIN_USERNAME;
const password = process.env.TEST_RECONCILIATION_ADMIN_PASSWORD;

type Job = {
  id: string;
  brand_id: string;
  state: string;
  version: number;
  target_count: string;
  checked_count: string;
  consistent_count: string;
  repairable_count: string;
  corrupt_count: string;
  failed_count: string;
  pending_count: string;
  reason: string;
};

type Target = {
  account_id: string;
  member_id: string;
  outcome: string | null;
  preview: {
    actual: Record<string, Record<string, string>>;
    expected: Record<string, Record<string, string>>;
    issues: string[];
    consistent: boolean;
    repairable: boolean;
  } | null;
};

async function data<T>(response: Awaited<ReturnType<APIRequestContext["get"]>>, status = 200): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  return JSON.parse(body).data as T;
}

async function getData<T>(page: Page, path: string): Promise<T> {
  return data<T>(await page.request.get(`${admin}${path}`, {
    headers: { "X-Brand-ID": brand },
  }));
}

async function targetRows(page: Page, jobId: string): Promise<Target[]> {
  const result = await getData<{ items: Target[] }>(page,
    `/reconciliations/${jobId}/targets?limit=20&offset=0`);
  return result.items;
}

async function fingerprint(page: Page, memberId: string): Promise<string> {
  const read=async(path:string)=>{
    const r=await page.request.get(admin+path,{headers:{"X-Brand-ID":brand}});
    const value=await r.json();
    // The intentionally missing bucket may make the existing wallet endpoint
    // refuse a read. Preserve that real rejection instead of fabricating a
    // successful balance; request IDs are not economic data.
    expect([200,409]).toContain(r.status());
    if(r.status()===409)expect(value.error.code).toBe("POINTS_RECONCILIATION_REQUIRED");
    return {status:r.status(),success:value.success,data:value.data,error:value.error};
  };
  return JSON.stringify({wallet:await read(`/wallets/${memberId}`),ledger:await read(`/wallets/${memberId}/ledger?limit=100&offset=0`)});
}

async function navigateTo(page: Page, project: string, destination: "批量对账" | "报表和对账") {
  if (project === "mobile") {
    await page.locator(".mobile-nav button").nth(4).click();
    await page.locator(".mobile-more-menu").getByRole("button", { name: new RegExp(destination) }).click();
  } else {
    await page.locator(".side-nav").getByRole("button", { name: new RegExp(destination) }).click();
  }
  return page.locator(".recon");
}

test("real reconciliation keeps an unknown create pending until the original request is replayed", async ({ page, context }, info) => {
  test.setTimeout(60_000);
  test.skip(!username || !password, "Provide isolated reconciliation administrator credentials");

  const pageErrors: string[] = [];
  page.on("pageerror", error => pageErrors.push(error.message));

  const login = await page.request.post(`${admin}/auth/login`, {
    headers: { Origin: origin, "Idempotency-Key": crypto.randomUUID() },
    data: { identifier: username, password },
  });
  expect(login.status(), await login.text()).toBe(200);
  rememberAdminSession(username!, await context.cookies(`${admin}/me`));

  await page.goto(origin);
  await page.getByLabel("选择真实后台品牌", { exact: true }).selectOption(brand);
  const panel = await navigateTo(page, info.project.name, "批量对账");
  await expect(panel.getByRole("heading", { name: "余额对账任务", exact: true })).toBeVisible();

  const reason = `Isolated reconciliation browser ${info.project.name} ${crypto.randomUUID()}`;
  await panel.getByLabel("操作原因", { exact: true }).fill(reason);

  let firstBody: string | undefined;
  let firstKey: string | undefined;
  let firstJobId: string | undefined;
  let baseline = new Map<string, string>();
  let createPosts = 0;
  let repairPosts = 0;

  await page.route("**/api/v1/admin/reconciliations", async route => {
    if (route.request().method() !== "POST") return route.continue();
    createPosts++;
    const body = route.request().postData()!;
    const key = route.request().headers()["idempotency-key"];
    if (createPosts === 1) {
      firstBody = body;
      firstKey = key;
      expect(JSON.parse(body)).toEqual({ reason });
      expect(key).toMatch(/^[A-Za-z0-9_:.-]{8,128}$/);
      const actual = await route.fetch();
      const actualBody = await actual.text();
      expect(actual.status(), actualBody).toBe(201);
      const receipt = JSON.parse(actualBody).data as Job;
      firstJobId = receipt.id;

      // Keep real state evidence from the seeded accounts. The route only
      // obscures the create receipt after the server has committed it.
      const rows = await targetRows(page, receipt.id);
      expect(rows).toHaveLength(3);
      baseline = new Map(await Promise.all(rows.map(async row => [
        row.member_id,
        await fingerprint(page, row.member_id),
      ] as const)));
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: '{"success":true,"data":{}}',
      });
      return;
    }
    expect(body).toBe(firstBody);
    expect(key).toBe(firstKey);
    await route.continue();
  });
  await page.route("**/api/v1/admin/wallets/*/repair", async route => {
    if (route.request().method() === "POST") repairPosts++;
    await route.continue();
  });

  await panel.getByRole("button", { name: "检查并确认提交", exact: true }).click();
  const confirmation = page.getByRole("dialog");
  await expect(confirmation.getByRole("heading", { name: "确认唯一操作员提交", exact: true })).toBeVisible();
  await expect(confirmation).toContainText(reason);
  await confirmation.getByRole("checkbox", { name: "我已核对范围与原因，确认由我提交此请求。", exact: true }).check();
  await confirmation.getByRole("button", { name: "确认并提交", exact: true }).click();

  await expect(panel.getByRole("alert")).toContainText("写入结果未知");
  await expect(panel.getByRole("heading", { name: "尚未确定的写入", exact: true })).toBeVisible();
  expect(createPosts).toBe(1);
  expect(firstJobId).toBeTruthy();

  // A read refresh must leave the unresolved create intent available.
  await panel.getByRole("button", { name: "刷新任务", exact: true }).click();
  await expect(panel.getByRole("heading", { name: "尚未确定的写入", exact: true })).toBeVisible();
  const jobs = await getData<{ items: Job[]; total_count: string }>(page,
    "/reconciliations?limit=100&offset=0");
  const matching = jobs.items.filter(item => item.reason === reason);
  expect(matching).toHaveLength(1);
  expect(matching[0]!.target_count).toBe("3");

  await navigateTo(page, info.project.name, "报表和对账");
  const returnedPanel = await navigateTo(page, info.project.name, "批量对账");
  await expect(returnedPanel.getByRole("heading", { name: "尚未确定的写入", exact: true })).toBeVisible();
  await expect(returnedPanel.getByRole("button", { name: "确认并重放原创建请求", exact: true })).toBeVisible();

  // An explicit, separately confirmed replay uses the byte-for-byte request
  // and key retained after the malformed 201 response.
  await returnedPanel.getByRole("button", { name: "确认并重放原创建请求", exact: true }).click();
  const replayDialog = page.getByRole("dialog");
  await expect(replayDialog).toContainText(reason);
  await replayDialog.getByRole("checkbox", { name: "我已核对范围与原因，确认由我提交此请求。", exact: true }).check();
  await replayDialog.getByRole("button", { name: "确认并提交", exact: true }).click();

  const receiptPanel = returnedPanel.locator(".receipt-panel");
  await expect(receiptPanel.getByRole("heading", { name: "服务器已确认请求回执", exact: true })).toBeVisible();
  await expect(receiptPanel.locator(".tag")).toHaveText("回执 v1");
  await expect(receiptPanel).toContainText("范围 3 个账户 · 待处理 3");
  expect(createPosts).toBe(2);

  const jobId = firstJobId!;
  await expect.poll(async () => {
    const job = await getData<Job>(page, `/reconciliations/${jobId}`);
    return job.state;
  }, { timeout: 45_000 }).toBe("completed");

  const finalJob = await getData<Job>(page, `/reconciliations/${jobId}`);
  expect(finalJob.brand_id).toBe(brand);
  expect(finalJob.reason).toBe(reason);
  expect(finalJob.version).toBeGreaterThanOrEqual(2);
  expect(finalJob.target_count).toBe("3");
  expect(finalJob.checked_count).toBe("3");
  expect(finalJob.pending_count).toBe("0");
  expect(finalJob.consistent_count).toBe("1");
  expect(finalJob.repairable_count).toBe("1");
  expect(finalJob.corrupt_count).toBe("1");

  await returnedPanel.getByRole("button",{name:`查看对账任务 ${jobId}`,exact:true}).click();
  await expect(returnedPanel.locator(".target-card")).toHaveCount(3);
  const rows = await targetRows(page, jobId);
  expect(rows).toHaveLength(3);
  expect(new Set(rows.map(row => row.member_id)).size).toBe(3);
  expect(rows.map(row => row.outcome).sort()).toEqual(["consistent", "corrupt", "repairable"]);
  expect(rows.every(row => row.preview !== null)).toBe(true);
  const consistent = rows.find(row => row.outcome === "consistent")!;
  expect(consistent.preview!.consistent).toBe(true);
  expect(consistent.preview!.actual.gift?.available).toBe("11");
  const repairable = rows.find(row => row.outcome === "repairable")!;
  expect(repairable.preview!.repairable).toBe(true);
  expect(repairable.preview!.actual.gift?.available).toBe("99");
  expect(repairable.preview!.expected.gift.available).toBe("7");
  const corrupt = rows.find(row => row.outcome === "corrupt")!;
  expect(corrupt.preview!.issues.join(" ")).toMatch(/ledger integrity/i);

  const outcome = returnedPanel.getByLabel("结果筛选", { exact: true });
  await outcome.selectOption("repairable");
  const repairableCard = returnedPanel.locator(".target-card").filter({ hasText: repairable.member_id });
  await expect(repairableCard).toBeVisible();
  await repairableCard.locator("details > summary").click();
  await expect(repairableCard).toContainText("这是任务检查时观察到的历史数据");
  await expect(repairableCard.locator(".preview-grid")).toContainText('"available": "99"');
  await expect(repairableCard.locator(".preview-grid")).toContainText('"available": "7"');
  await outcome.selectOption("");
  await expect(returnedPanel.locator(".target-card")).toHaveCount(3);
  await expect(returnedPanel.locator(".target-card").filter({hasText:consistent.member_id}).locator(".state")).toHaveText("一致");
  await expect(returnedPanel.locator(".target-card").filter({hasText:repairable.member_id}).locator(".state")).toHaveText("可修复");
  await expect(returnedPanel.locator(".target-card").filter({hasText:corrupt.member_id}).locator(".state")).toHaveText("异常");

  for (const row of rows) {
    expect(baseline.has(row.member_id)).toBe(true);
    expect(await fingerprint(page, row.member_id)).toBe(baseline.get(row.member_id));
  }
  expect(repairPosts).toBe(0);

  await expect.poll(async () => {
    const list = await getData<{ items: Job[]; total_count: string }>(page,
      "/reconciliations?limit=100&offset=0");
    return list.items.filter(item => item.reason === reason).length;
  }).toBe(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await returnedPanel.screenshot({ path: info.outputPath(`reconciliation-${info.project.name}-panel.png`) });
  await page.screenshot({ path: info.outputPath(`reconciliation-${info.project.name}-viewport.png`) });
  expect(pageErrors).toEqual([]);
});
