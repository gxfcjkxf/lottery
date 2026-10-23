import { readFileSync, readdirSync } from 'node:fs';
import { test } from 'node:test';
import assert from 'node:assert/strict';
const read = path => readFileSync(new URL('../../' + path, import.meta.url), 'utf8');

test('three applications share Lucide icons without page-specific drawings or sizes', () => {
  for (const app of ['user-web', 'admin-web', 'platform-web', 'shared']) {
    for (const name of readdirSync(new URL(`../../${app}/src`, import.meta.url)).filter(name => name.endsWith('.vue'))) {
      const path = `${app}/src/${name}`, source = read(path);
      assert.doesNotMatch(source, /<svg\b/, `${path}: use the shared Lucide component for UI icons`);
      assert.doesNotMatch(source, /<SiteIcon\b[^>]*\s:size=/, `${path}: icons share one size`);
      assert.doesNotMatch(source, /[👤📑🔍◈♧⌕]/u, `${path}: do not use font glyphs as UI icons`);
    }
  }
  const icon = read('shared/src/SiteIcon.ts');
  assert.match(icon, /from '@lucide\/vue'/);
  assert.match(icon, /size: 20, strokeWidth: 1\.75/);
  assert.match(icon, /var\(--site-icon-size, 20px\)/);
  assert.match(read('shared/src/management.css'), /--site-icon-size: 16px/);
  assert.match(icon, /language: Languages/);
});
