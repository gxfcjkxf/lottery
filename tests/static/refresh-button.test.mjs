import { readFileSync, readdirSync } from 'node:fs';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { refreshButtonCandidates, isRecoveryRead } from '../../scripts/refresh-button-audit.mjs';
const read = path => readFileSync(new URL('../../' + path, import.meta.url), 'utf8');

test('all data-refresh actions use the shared icon directive and translated label', () => {
  const candidates = refreshButtonCandidates();
  assert.ok(candidates.length > 0, 'the refresh audit must find application controls');
  const missing = candidates.filter(({ node }) => !node.props.some(prop => prop.type === 7 && prop.name === 'refresh-button')).map(node => `${node.path}:${node.line}`);
  assert.deepEqual(missing, []);
  for (const { node, path, label } of candidates) {
    assert.ok(label.trim(), path);
    assert.ok(node.props.some(prop => prop.type === 7 && prop.arg?.content === 'click'), path);
    assert.doesNotMatch(node.loc.source, /<svg\b/, `${path}: shared directive owns the Lucide icon`);
  }
});
test('three apps load one refresh-button visual contract', () => {
  for (const app of ['user-web', 'admin-web', 'platform-web']) assert.match(read(`${app}/src/main.ts`), /import ['"]@lottery\/shared\/refresh-button\.css['"]/);
  const css = read('shared/src/refresh-button.css');
  for (const rule of ['width: var(--icon-control-size, 44px)', 'height: var(--icon-control-size, 44px)', 'width: 20px', 'height: 20px', 'gap: 12px', 'margin: 0', ':focus-visible', ':disabled', '[data-refresh-icon]']) assert.ok(css.includes(rule), rule);
  assert.doesNotMatch(css, /mask: url\(/);
  assert.match(read('shared/src/SiteIcon.ts'), /from '@lucide\/vue'/);
  assert.doesNotMatch(css, /font-size:\s*(?:0|[1-9]|10)px\b/);
  assert.ok(read('docs/48-ui-architecture-and-design-system.md').includes('三端数据刷新按钮'));
});
test('business styles cannot override the reserved refresh-button contract', () => {
  for (const dir of ['user-web/src', 'admin-web/src', 'platform-web/src', 'shared/src']) {
    for (const file of readdirSync(new URL('../../' + dir, import.meta.url)).filter(name => /\.(vue|css)$/.test(name) && name !== 'refresh-button.css')) {
      const source = read(`${dir}/${file}`);
      const css = file.endsWith('.vue') ? source.match(/<style\b[^>]*>([\s\S]*?)<\/style>/g)?.join('\n') ?? '' : source;
      const refreshTargets = css.replace(/:not\(\[data-refresh-button[^\]]*\]\)/g, '');
      assert.doesNotMatch(refreshTargets, /\[data-refresh-button[^\]]*\][^{}]*\{/, `${dir}/${file}`);
    }
  }
});

test('text recovery exception requires an explicit recovery label and a failure/conflict context', () => {
  assert.equal(isRecoveryRead("t('admin.settings.readAgain')", ['error']), true);
  assert.equal(isRecoveryRead("t('brandOperation.readLatest')", ['readFailed']), true);
  assert.equal(isRecoveryRead("t('security.reread')", ["pending.phase === 'conflict'"]), true);
  assert.equal(isRecoveryRead("t('admin.settings.readAgain')", []), false);
  assert.equal(isRecoveryRead("t('admin.copy.1935ecf7')", ['error']), false);
  assert.equal(isRecoveryRead("t('security.retry')", ['error']), false);
  for (const condition of ['!error', '!readFailed', '!conflictReloaded', "pending.phase !== 'conflict'"]) {
    assert.equal(isRecoveryRead("t('admin.settings.readAgain')", [condition]), false);
  }
});
