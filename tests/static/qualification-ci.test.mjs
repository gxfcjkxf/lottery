import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

test('qualification CI uses real synthetic stakes and isolated no-retry financial workflows',()=>{
  const workflow=readFileSync(new URL('../../.github/workflows/ci.yaml',import.meta.url),'utf8');
  const job=workflow.slice(workflow.indexOf('  qualification-browser:'),workflow.indexOf('  reconciliation-browser:'));
  assert.match(job,/POSTGRES_DB: lottery_withdrawal_qualification_s9/);
  assert.match(job,/WITHDRAWAL_QUAL_FIXTURE_CONFIRM: owned_synthetic_database/);
  assert.match(job,/go run -tags browserfixture \.\/cmd\/qualification-fixture/);
  assert.match(job,/playwright test --config playwright\.qualification\.config\.ts/);
  assert.match(job,/PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH/);
  const config=readFileSync(new URL('../../playwright.qualification.config.ts',import.meta.url),'utf8');
  for (const width of [360,768,1024,1440]) assert.ok(config.includes(`width: ${width},`));
  assert.match(config,/workers: 1/);
  assert.match(config,/retries: 0/);
  assert.match(config,/fullyParallel: false/);
  assert.match(config,/testMatch: \['withdrawal-qualification.spec.ts'\]/);
  const browser=readFileSync(new URL('../browser/withdrawal-qualification.spec.ts',import.meta.url),'utf8');
  for (const username of ['qual_user','qual_mobile_user','qual_tablet_user','qual_laptop_user']) {
    assert.ok(browser.includes(username));
  }
  const fixture=readFileSync(new URL('../../backend/cmd/qualification-fixture/main.go',import.meta.url),'utf8');
  assert.match(fixture,/\/\/go:build browserfixture/);
  assert.match(fixture,/environment != "test"/);
  assert.match(fixture,/\.Place\(/);
  assert.match(fixture,/ProcessSettlements\(/);
  assert.ok(!fixture.includes('Eligibility: checker'));
});
