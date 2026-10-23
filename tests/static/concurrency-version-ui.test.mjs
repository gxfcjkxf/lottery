import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { test } from 'node:test';

const require = createRequire(new URL('../../admin-web/package.json', import.meta.url));
const { parse, compileTemplate } = require('vue/compiler-sfc');
const { parse: parseTemplate } = require(require.resolve('@vue/compiler-dom', { paths: [require.resolve('vue/compiler-sfc')] }));
const technicalVersion = /\.(?:version|(?:order|old_order|period|policy|game|brand|ledger|adjustment|financial|plan|point_policy|reserve|cycle_from|reset_order|new_job|current_job)_version|revision|expected_revision)\b|\.policy_versions\.(?:brand|game)\b/;

for (const directory of ['user-web/src', 'admin-web/src', 'platform-web/src', 'shared/src']) {
  test(`${directory} keeps concurrency tokens out of rendered fields and inputs`, () => {
    for (const file of readdirSync(new URL(`../../${directory}/`, import.meta.url)).filter(name => name.endsWith('.vue'))) {
      const filename = `${directory}/${file}`;
      const source = readFileSync(new URL(`../../${filename}`, import.meta.url), 'utf8');
      const { descriptor } = parse(source);
      if (!descriptor.template) continue;
      const result = compileTemplate({ source: descriptor.template.content, filename, id: filename });
      assert.deepEqual(result.errors, [], filename);
      function visit(node) {
        // Vue keys, disabled expressions and request bodies still need the tokens.
        if (node.type === 5) {
          const expression = node.content.content.replace(/\b(?:copy|labels)\.version\b/g, '').replace(/(['"])(?:\\.|(?!\1).)*\1/g, '');
          assert.doesNotMatch(expression, technicalVersion, `${filename}: ${node.loc.source}`);
        }
        for (const prop of node.props ?? []) {
          if (prop.name === 'model') assert.doesNotMatch(prop.exp?.content ?? '', /version|revision/i, filename);
          if (prop.name === 'bind' && ['title', 'aria-label'].includes(prop.arg?.content)) {
            assert.doesNotMatch(prop.exp?.content ?? '', technicalVersion, filename);
          }
        }
        for (const child of node.children ?? []) visit(child);
      }
      visit(parseTemplate(descriptor.template.content));
    }
  });
}
