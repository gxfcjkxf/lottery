import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { test } from 'node:test';

const root = new URL('../../', import.meta.url);
const source = name => readFileSync(new URL(name, root), 'utf8');
const vueFiles = directory => readdirSync(new URL(directory, root)).filter(name => name.endsWith('.vue'));

test('all three web applications mount an explicit i18n provider', () => {
  assert.match(source('platform-web/src/main.ts'), /provide\(platformI18nKey, i18n\)/);
  assert.match(source('platform-web/src/i18n.ts'), /createCatalogI18n/);
  assert.match(source('user-web/src/main.ts'), /provide\(userI18nKey, i18n\)/);
  assert.match(source('user-web/src/i18n.ts'), /createCatalogI18n/);
  assert.match(source('admin-web/src/main.ts'), /provide\(adminI18nKey, i18n\)/);
  assert.match(source('admin-web/src/i18n.ts'), /createAdminI18n/);
});

test('platform and user templates do not branch interface copy with locale ternaries', () => {
  const offenders = [];
  for (const directory of ['platform-web/src', 'user-web/src']) {
    for (const filename of vueFiles(directory)) {
      const content = source(`${directory}/${filename}`);
      const template = /<template>([\s\S]*?)<\/template>/.exec(content)?.[1] ?? '';
      if (/(?:locale|props\.locale)\s*===\s*['"](?:en|zh|zh-CN)['"]\s*\?/.test(template)) offenders.push(`${directory}/${filename}`);
    }
  }
  assert.deepEqual(offenders, []);
});

test('platform shell and status modal derive copy reactively from the active catalog locale', () => {
  const app = source('platform-web/src/App.vue');
  assert.match(app, /function createLang\(\)/);
  assert.match(app, /const lang = computed<PlatformCopy>\(createLang\)/);
  assert.doesNotMatch(app, /const lang = computed\(\(\) => locale\.value === ['"]en['"] \? en : zh\)/);
  assert.doesNotMatch(app, /const en = \{/);
  assert.doesNotMatch(app, /const zh = \{/);

  const operation = source('platform-web/src/BrandOperationPanel.vue');
  assert.match(operation, /const copy = computed\(\(\) =>/);
  assert.doesNotMatch(operation, /const en = \{/);
  assert.doesNotMatch(operation, /const zh = \{/);
});

test('platform document title follows the configured platform branding', () => {
  const html = source('platform-web/index.html');
  const app = source('platform-web/src/App.vue');
  assert.doesNotMatch(html, /<title>[^<]+<\/title>/);
  assert.match(app, /const value = platformPresentation\.value; applyBrowserBranding\(value\.display_name, value\.logo_text, value\.logo_url,/);
  assert.match(source('shared/src/presentation.ts'), /document\.title\s*=\s*name/);
});
