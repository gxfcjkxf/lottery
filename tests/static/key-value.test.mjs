import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import assert from 'node:assert/strict';
const read = path => readFileSync(new URL('../../' + path, import.meta.url), 'utf8');

test('all three apps use the same compact display label/value contract', () => {
  for (const app of ['user-web', 'admin-web', 'platform-web']) {
    assert.match(read(`${app}/src/main.ts`), /import ['"]@lottery\/shared\/key-value\.css['"]/);
  }
  const css = read('shared/src/key-value.css');
  for (const rule of ['dt + dd', '.key-value-row', 'fit-content(45%)', 'minmax(0, 1fr)', 'overflow-wrap: anywhere', '.key-value-wide']) assert.ok(css.includes(rule), rule);
  assert.doesNotMatch(css, /font-size:|text-overflow:|overflow(?:-x)?:\s*hidden/);
  assert.ok(read('docs/48-ui-architecture-and-design-system.md').includes('展示型标题和值：优先同行'));
});
