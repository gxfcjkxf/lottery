import { readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import assert from 'node:assert/strict';
import { test } from 'node:test';

const roots = ['user-web/src', 'admin-web/src', 'platform-web/src', 'shared/src'];
function files(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const path = resolve(directory, entry.name);
    return entry.isDirectory() ? files(path) : [path];
  });
}
test('all three surfaces explicitly disable native form bubbles and keep inline validation', () => {
  let count = 0;
  for (const file of roots.flatMap(files).filter(file => file.endsWith('.vue'))) {
    const source = readFileSync(file, 'utf8');
    for (const [form] of source.matchAll(/<form\b[^>]*>/g)) {
      count++;
      assert.match(form, /\bnovalidate\b/, file);
      if (file.endsWith('/user-web/src/Page.vue') && form.includes('class="auth-form"')) {
        assert.match(form, /@submit\.prevent="withGlobalLoading\(authSubmit\)"/);
      } else assert.match(form, /\bv-inline-validation\b/, file);
    }
  }
  assert.ok(count > 0, 'the form audit must scan application forms');
  for (const surface of ['user-web', 'admin-web', 'platform-web']) {
    const main = readFileSync(`${surface}/src/main.ts`, 'utf8');
    assert.match(main, /setInlineValidationLocale\(locale\)/);
    assert.match(main, /@lottery\/shared\/form-validation\.css/);
  }
});
test('production code never invokes browser validity popups or uses browser-language error strings', () => {
  for (const file of roots.flatMap(files).filter(file => /\.(vue|ts)$/.test(file) && !file.endsWith('.test.ts'))) {
    assert.doesNotMatch(readFileSync(file, 'utf8'), /\.(?:reportValidity|checkValidity)\s*\(|\.validationMessage\b/, file);
  }
});
