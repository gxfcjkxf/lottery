import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

test('qualification CI uses real synthetic stakes and isolated no-retry financial workflows',()=>{
  const workflow=readFileSync(new URL('../../.github/workflows/ci.yaml',import.meta.url),'utf8');
  const job=workflow.slice(workflow.indexOf('  qualification-browser:'),workflow.indexOf('  reconciliation-browser:'));
  assert.match(job,/POSTGRES_DB: lottery_withdrawal_qualification_s9/);
  assert.match(job,/WITHDRAWAL_QUAL_FIXTURE_CONFIRM: owned_synthetic_database/);
  assert.match(job,/go run -tags browserfixture \.\/cmd\/qualification-fixture/);
  assert.match(job,/withdrawal-qualification\.spec\.ts --project=desktop --workers=1 --retries=0/);
  assert.match(job,/withdrawal-qualification\.spec\.ts --project=mobile --workers=1 --retries=0/);
  const fixture=readFileSync(new URL('../../backend/cmd/qualification-fixture/main.go',import.meta.url),'utf8');
  assert.match(fixture,/\/\/go:build browserfixture/);
  assert.match(fixture,/environment != "test"/);
  assert.match(fixture,/\.Place\(/);
  assert.match(fixture,/ProcessSettlements\(/);
  assert.ok(!fixture.includes('Eligibility: checker'));
});
