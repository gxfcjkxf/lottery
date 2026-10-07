import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

test('commission UI CI isolates viewports and advances real owned workflows without retries',()=>{
  const workflow=readFileSync(new URL('../../.github/workflows/ci.yaml',import.meta.url),'utf8');
  const start=workflow.indexOf('  commission-cycles-browser:');
  const end=workflow.indexOf('  withdrawal-browser:',start);
  assert.ok(start>=0&&end>start);
  const job=workflow.slice(start,end);
  assert.match(job,/viewport: \[desktop, mobile\]/);
  assert.match(job,/POSTGRES_DB: lottery_commission_ui_\$\{\{ matrix\.viewport \}\}_s16/);
  assert.match(job,/COMMISSION_FIXTURE_CONFIRM: owned_synthetic_database/);
  assert.match(job,/go test -count=1 -tags browserfixture \.\/cmd\/commission-fixture/);
  assert.match(job,/\.\/bin\/commission-fixture init/);
  assert.match(job,/commission-cycles\.spec\.ts --project=\$\{\{ matrix\.viewport \}\} --workers=1 --retries=0/);
  assert.match(job,/commission-payments\.spec\.ts --project=\$\{\{ matrix\.viewport \}\} --workers=1 --retries=0/);
  assert.match(job,/commission-adjustments\.spec\.ts --project=\$\{\{ matrix\.viewport \}\} --workers=1 --retries=0/);
  assert.match(job,/commission-reports\.spec\.ts --project=\$\{\{ matrix\.viewport \}\} --workers=1 --retries=0/);
  assert.ok(job.indexOf('test tests/browser/commission-cycles.spec.ts') < job.indexOf('test tests/browser/commission-payments.spec.ts'));
  assert.ok(job.indexOf('test tests/browser/commission-payments.spec.ts') < job.indexOf('test tests/browser/commission-adjustments.spec.ts'));
  assert.ok(job.indexOf('test tests/browser/commission-adjustments.spec.ts') < job.indexOf('test tests/browser/commission-reports.spec.ts'));
  assert.doesNotMatch(job,/\.\/bin\/platform worker\s*>/);
  const fixture=readFileSync(new URL('../../backend/cmd/commission-fixture/main.go',import.meta.url),'utf8');
  assert.match(fixture,/\/\/go:build browserfixture/);
  assert.match(fixture,/economic_fingerprint/);
  assert.match(fixture,/ActOnSettlement/);
  assert.doesNotMatch(fixture,/UPDATE periods SET status|INSERT INTO commission_earnings|INSERT INTO settlement_calculations/);
});
