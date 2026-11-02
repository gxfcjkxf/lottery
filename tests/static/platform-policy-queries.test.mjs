import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');

test('archive configuration retains its own scoped write intent', () => {
 const config=read('platform-web/src/ArchiveTasksPanel.vue');
 assert.match(config,/getPlatformArchiveIntent\(props.accountId\)/);
 assert.match(config,/getPlatformArchiveIntent\(intent.actor\)!==intent/);
 assert.match(config,/onBeforeUnmount\(\(\)=>\{alive=false;ticket\+\+/);
 assert.match(config,/api.update\(intent.body,intent.key,intent.actor\)/);
 assert.match(config,/await load\(\)/);
 assert.doesNotMatch(config,/brandId|task-list|api\.retry/);
 const api=read('platform-web/src/platform-archive-policy-api.ts');
 assert.match(api,/report-archive-policy/);assert.match(api,/X-Report-Archive-Actor-ID/);assert.doesNotMatch(api,/X-Brand-ID/);
});

test('policy browser preserves real configuration, tests saved schedules, and never loads archive tasks', () => {
 const spec=read('tests/review/platform-policy-queries.spec.ts');
 assert.match(spec,/archive configuration shows saved schedules without requesting task lists/);
assert.match(spec,/expect\(tasks\)\.toBe\(0\)/);
assert.doesNotMatch(spec,/auth_rate_limits|X-Forwarded-For|DELETE FROM|force: true/);
});
