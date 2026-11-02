import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import test from 'node:test';
test('configured attribution is checked with real configured money, without old rulebook fixtures',()=>{
 const source=readFileSync(new URL('../../backend/internal/betting/configured_finance_test.go',import.meta.url),'utf8');
 assert.match(source,/reports.Attribution/);
 assert.match(source,/configuredPlace/);
 assert.match(source,/FinalLostStakePoints/);
 const ci=readFileSync(new URL('../../.github/workflows/ci.yaml',import.meta.url),'utf8');
 assert.doesNotMatch(ci,/attribution-reports-browser:/);
});
