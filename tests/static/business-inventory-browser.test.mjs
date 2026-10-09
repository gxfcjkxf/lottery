import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

const read=path=>readFileSync(new URL('../../'+path,import.meta.url),'utf8');
test('inventory CI keeps owned viewport databases, original workflow and independent financial proof without retries',()=>{
 const job=read('.github/workflows/ci.yaml').split('  reconciliation-browser:')[1].split('  api-contract:')[0];
 assert.match(job,/project: \[desktop, mobile\]/);
 assert.match(job,/POSTGRES_DB: lottery_reconciliation_browser/);
 assert.match(job,/RECONCILIATION_FIXTURE_CONFIRM: owned_synthetic_database/);
 const migrate=job.indexOf('./bin/platform migrate'),seed=job.indexOf('./bin/platform seed'),admin=job.indexOf('./bin/platform create-admin');
 const fixture=job.indexOf('go run -tags browserfixture ./cmd/reconciliation-fixture\n');
 const before=job.indexOf('reconciliation-fingerprint-before.txt'),tests=job.indexOf('pnpm exec playwright test'),after=job.indexOf('reconciliation-fingerprint-after.txt');
 assert.ok(migrate>=0&&seed>migrate&&admin>seed&&fixture>admin&&before>fixture&&tests>before&&after>tests);
 assert.match(job,/tests\/browser\/reconciliation\.spec\.ts tests\/browser\/business-inventory\.spec\.ts --project=\$\{\{ matrix\.project \}\} --workers=1 --retries=0/);
 assert.match(job,/cmp ..\/reconciliation-fingerprint-before\.txt ..\/reconciliation-fingerprint-after\.txt/);
});

test('inventory browser and tagged verifier preserve declared readonly and fixture boundaries',()=>{
 const browser=read('tests/browser/business-inventory.spec.ts'),fixture=read('backend/cmd/reconciliation-fixture/main.go');
 assert.match(browser,/expect\(reads\)\.toBe\(0\)/);
 assert.match(browser,/expect\(writes\)\.toBe\(0\)/);
 assert.match(browser,/data\.fingerprint/);
 assert.match(browser,/MISSING_REQUIRED_LEDGER_REFERENCE/);
 assert.match(browser,/reward_orders/);
 assert.match(fixture,/^\/\/go:build browserfixture/);
 assert.match(fixture,/owned_synthetic_database/);
 assert.match(fixture,/u\.Path != "\/lottery_reconciliation_browser"/);
 assert.match(fixture,/pgx\.RepeatableRead, AccessMode: pgx\.ReadOnly/);
 assert.match(fixture,/reconciliation\.InventorySources\(\), "outbox_events"/);
 assert.match(fixture,/session_replication_role=origin/);
});
