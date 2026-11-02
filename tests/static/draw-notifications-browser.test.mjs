import {test} from 'node:test';import assert from 'node:assert/strict';import {readFileSync} from 'node:fs';
test('configured order notices validate actual immutable order and prize ledgers',()=>{
 const source=readFileSync('backend/internal/notification/configured_bet.go','utf8');assert.match(source,/FROM configured_orders/);assert.match(source,/configured-prize:/);assert.doesNotMatch(source,/settlement_jobs|settlement_calculations/);
 const cases=readFileSync('backend/internal/betting/configured_admission_test.go','utf8');assert.match(cases,/TestConfiguredOrderNotificationsUseCurrentFinancialEvidence/);assert.match(cases,/seen\["bet.order.won"\] != 1/);
});
