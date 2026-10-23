import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { assertReviewIsolation } from '../../scripts/review-isolation.mjs';
const url = name => `postgres://lottery_test@127.0.0.1:55432/${name}?sslmode=disable`;
test('review allows only explicitly owned isolated test databases', () => {
  for (const name of ['desktop1440', 'mobilepixel7', 'local_cleanup']) assert.doesNotThrow(() => assertReviewIsolation({ APP_ENV: 'test', DATABASE_URL: url('lottery_admin_entries_'+name) }));
  for (const name of ['lottery_review_self_review_20261010', 'lottery', 'postgres']) assert.throws(() => assertReviewIsolation({ APP_ENV: 'test', DATABASE_URL: url(name) }), /isolated/);
  assert.throws(() => assertReviewIsolation({ APP_ENV: 'development', DATABASE_URL: url('lottery_admin_entries_desktop1440') }), /APP_ENV=test/);
  assert.throws(() => assertReviewIsolation({ APP_ENV: 'test' }), /DATABASE_URL/);
  assert.throws(() => assertReviewIsolation({ APP_ENV: 'test', DATABASE_URL: 'invalid' }), /Invalid/);
});
test('rule self-review owns a temporary brand and never targets the normal Aurora brand', () => {
  const fixture = readFileSync(new URL('../review/self-review-fixture.ts', import.meta.url), 'utf8');
  const scenario = readFileSync(new URL('../review/rule-self-review.spec.ts', import.meta.url), 'utf8');
  assert.match(scenario, /from '\.\/self-review-fixture'/);
  assert.match(scenario, /const brand = selfReviewBrand\.id/);
  assert.doesNotMatch(scenario, /0199a000-0000-7000-8000-000000000001/);
  assert.match(fixture, /await post\('\/brands'/);
  assert.match(fixture, /finally[\s\S]*platform\.delete/);
});
