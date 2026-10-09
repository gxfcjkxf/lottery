import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";

const workflow=readFileSync(new URL("../../.github/workflows/ci.yaml",import.meta.url),"utf8");
const withdrawalPanel=readFileSync(new URL("../../user-web/src/WithdrawalPanel.vue",import.meta.url),"utf8");
const match=/^  withdrawal-browser:\n([\s\S]*?)(?=^  [a-z][a-z-]*:\n|(?![\s\S]))/m.exec(workflow);

test("withdrawal intent uses the required native UUID without a legacy key fallback",()=>{
  assert.match(withdrawalPanel,/key: crypto\.randomUUID\(\), actorContext:/);
  assert.doesNotMatch(withdrawalPanel,/function makeKey|getRandomValues|Date\.now\(\)/);
});

test("withdrawal CI uses an explicitly owned database and nonproduction fixture",()=>{
  assert.ok(match,"dedicated withdrawal-browser job is required");
  const job=match[1];
  assert.match(job,/POSTGRES_DB: lottery_withdrawal_ui_s8\b/);
  assert.match(job,/APP_ENV: test\b/);
  assert.match(job,/DATABASE_URL: postgres:\/\/lottery_test:[^\s]+@localhost:5432\/lottery_withdrawal_ui_s8\?sslmode=disable/);
  for(const key of ["WITHDRAWAL_FIXTURE_CONFIRM","WITHDRAWAL_FIXTURE_ADMIN_PASSWORD","WITHDRAWAL_FIXTURE_USER_PASSWORD","TEST_WITHDRAWAL_ADMIN_USERNAME","TEST_WITHDRAWAL_ADMIN_PASSWORD","TEST_WITHDRAWAL_USER_PASSWORD"])assert.match(job,new RegExp(`^      ${key}: .+$`,"m"));
  const environment=new Map([...job.matchAll(/^      ([A-Z_]+): (.+)$/gm)].map(x=>[x[1],x[2]]));
  assert.equal(environment.get("TEST_WITHDRAWAL_ADMIN_USERNAME"),"withdraw_admin");
  assert.equal(environment.get("WITHDRAWAL_FIXTURE_ADMIN_PASSWORD"),environment.get("TEST_WITHDRAWAL_ADMIN_PASSWORD"));
  assert.equal(environment.get("WITHDRAWAL_FIXTURE_USER_PASSWORD"),environment.get("TEST_WITHDRAWAL_USER_PASSWORD"));
  assert.match(job,/WITHDRAWAL_FIXTURE_CONFIRM: owned_synthetic_database/);
  assert.match(job,/go run -tags browserfixture \.\/cmd\/withdrawal-fixture/);
  assert.match(job,/\.\/bin\/platform worker/);
  assert.doesNotMatch(job,/Eligibility|ELIGIBILITY|AUTH_KEY:|create-admin|\.\/bin\/platform seed/);
});

test("withdrawal CI runs both financial workflows before readonly verification without retries",()=>{
  assert.ok(match);
  const job=match[1];
  const runs=[...job.matchAll(/^      - run: (pnpm exec playwright test .+)$/gm)].map(x=>x[1]);
  assert.deepEqual(runs,[
    "pnpm exec playwright test tests/browser/withdrawal-orders.spec.ts --project=desktop --workers=1 --retries=0",
    "pnpm exec playwright test tests/browser/withdrawal-orders.spec.ts --project=mobile --workers=1 --retries=0",
    "pnpm exec playwright test tests/browser/withdrawal-observability.spec.ts --workers=1 --retries=0",
  ]);
  // Retrying a test which already committed its terminal withdrawal cannot
  // recreate the original fixture; failures retain traces rather than reset it.
  assert.doesNotMatch(job,/continue-on-error|strategy:|matrix:|DROP DATABASE|TRUNCATE|reset/);
  assert.match(job,/if: failure\(\)/);
  assert.match(job,/path: test-results\//);
});
