import {test} from 'node:test';import assert from 'node:assert/strict';import {readFileSync,existsSync} from 'node:fs';
test('configured qualification tests use actual settled order credit',()=>{
 const source=readFileSync('backend/internal/betting/configured_finance_test.go','utf8');assert.match(source,/TestConfiguredFinancialFactsAndTurnover/);assert.match(source,/TurnoverChecker/);assert.match(source,/configuredPublishDraw/);assert.match(source,/tc.valid/);
 assert.doesNotMatch(readFileSync('.github/workflows/ci.yaml','utf8'),/cmd\/qualification-fixture/);assert.equal(existsSync('backend/cmd/qualification-fixture/main.go'),false);
});
