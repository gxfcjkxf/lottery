import {test} from 'node:test';import assert from 'node:assert/strict';import {readFileSync,existsSync} from 'node:fs';
test('CI covers configured commission money without old engine fixtures',()=>{
 const workflow=readFileSync('.github/workflows/ci.yaml','utf8');assert.match(workflow,/go test -race.*internal\/betting -run '\^TestCommission'/);assert.doesNotMatch(workflow,/cmd\/commission-fixture|COMMISSION_FIXTURE_BIN/);
 const cases=readFileSync('backend/internal/betting/configured_finance_test.go','utf8');assert.match(cases,/TestCommissionConfiguredAttributedLossPaysOnce/);assert.match(cases,/ProcessPayments/);assert.match(cases,/credits != "1"/);assert.equal(existsSync('backend/cmd/commission-fixture/main.go'),false);
});
