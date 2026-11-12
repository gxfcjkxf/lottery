import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');

test('retention configuration keeps the original actor and write intent', () => {
 const config=read('platform-web/src/DataRetentionPanel.vue'),api=read('platform-web/src/data-retention-api.ts');
 assert.match(config,/getRetentionIntent\(props.accountId\)/);
 assert.match(config,/getRetentionIntent\(intent.actor\)!==intent/);
 assert.match(config,/onBeforeUnmount/);assert.match(config,/api.write\(intent\)/);
 assert.match(api,/X-Data-Retention-Actor-ID/);assert.doesNotMatch(api,/headers.set\('X-Brand-ID'/);
});

test('policy browser reads the monthly configuration without using retired routes', () => {
 const spec=read('tests/review/platform-policy-queries.spec.ts');
 assert.match(spec,/monthly retention policy and execution records use new endpoints/);
assert.match(spec,/expect\(legacyRequests\)\.toBe\(0\)/);
assert.doesNotMatch(spec,/auth_rate_limits|X-Forwarded-For|DELETE FROM|force: true/);
});
