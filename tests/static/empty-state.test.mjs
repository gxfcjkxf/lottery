import { readFileSync, readdirSync } from 'node:fs';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { emptyStateCandidates } from '../../scripts/empty-state-audit.mjs';
const read = path => readFileSync(new URL('../../' + path, import.meta.url), 'utf8');

test('three apps import one empty-state visual contract', () => {
  for (const app of ['user-web', 'admin-web', 'platform-web']) assert.match(read(`${app}/src/main.ts`), /import ['"]@lottery\/shared\/empty-state\.css['"]/);
  const css = read('shared/src/empty-state.css');
  for (const rule of ['min-height: 96px', 'padding: 16px', 'gap: 12px', 'margin: 0', 'text-align: center', 'justify-content: center', 'display: table-cell', 'vertical-align: middle']) assert.ok(css.includes(rule), rule);
  assert.match(css, /\.empty-state\[data-global-loading="true"\]\s*\{\s*display: none !important/);
  assert.ok(read('docs/48-ui-architecture-and-design-system.md').includes('三端统一空状态'));
});

test('standalone empty lists use the shared class without changing business copy', () => {
  const candidates = emptyStateCandidates();
  assert.ok(candidates.length > 110);
  const missing = candidates.filter(node => !node.classes.split(/\s+/).includes('empty-state')).map(node => `${node.path}:${node.line}: ${node.text}`);
  assert.deepEqual(missing, []);
});

test('pages cannot redefine the reserved empty-state appearance', () => {
  for (const dir of ['user-web/src', 'admin-web/src', 'platform-web/src', 'shared/src']) {
    for (const file of readdirSync(new URL('../../' + dir, import.meta.url)).filter(name => /\.(vue|css)$/.test(name) && name !== 'empty-state.css')) {
      const source = read(`${dir}/${file}`);
      const css = file.endsWith('.vue') ? source.match(/<style\b[^>]*>([\s\S]*?)<\/style>/g)?.join('\n') ?? '' : source;
      assert.doesNotMatch(css.replaceAll(':not(.empty-state)', ''), /\.empty-state\b[^{}]*\{/, `${dir}/${file}: shared empty-state.css owns this class`);
    }
  }
});
