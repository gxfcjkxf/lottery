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
