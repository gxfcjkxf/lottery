import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

test('commission browser date input uses canonical zero seconds and preserves the exact boundary', () => {
  const source = readFileSync(new URL('../browser/commission-analysis.spec.ts', import.meta.url), 'utf8');
  const body = source.match(/function localInput\(date: Date\): string \{([\s\S]*?)\n\}/)?.[1];
  assert.ok(body, 'The browser fixture date formatter must be available');
  // Execute this small trusted repository helper, not a copied approximation.
  const format = new Function('date', body.replace('(n: number)', '(n)'));
  for (const [iso, length] of [['2026-10-09T15:29:00.000Z', 16], ['2026-10-09T15:28:59.000Z', 19]]) {
    const date = new Date(iso);
    const value = format(date);
    assert.equal(value.length, length);
    assert.equal(new Date(value).getTime(), date.getTime());
  }
});
