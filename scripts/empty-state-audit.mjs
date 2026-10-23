import { readFileSync, readdirSync } from 'node:fs';
import { createRequire } from 'node:module';

const root = new URL('../', import.meta.url);
const require = createRequire(new URL('user-web/package.json', root));
const { parse } = require('vue/compiler-sfc');

// Audit standalone collection/selection placeholders, not inline field values,
// loading messages, permission notices or infinite-scroll end markers.
export function emptyStateCandidates() {
  const result = [];
  const emptyKeys = new Set();
  for (const surface of ['user-web', 'admin-web', 'platform-web']) {
    const dir = new URL(`${surface}/src/`, root);
    for (const file of readdirSync(dir).filter(file => /i18n.*\.ts$/.test(file))) {
      const catalog = readFileSync(new URL(file, dir), 'utf8');
      for (const match of catalog.matchAll(/['"]([^'"]+)['"]:\s*['"]([^'"\n]+)['"]/g)) {
        if (/^(暂无|尚无|所选事件暂无|当前注单没有|当前期次没有|此期次还没有|此页没有|本页没有分组)/.test(match[2]) && !match[2].includes('初始草稿')) emptyKeys.add(match[1]);
      }
    }
  }
  for (const surface of ['user-web', 'admin-web', 'platform-web', 'shared']) {
    const dir = new URL(`${surface}/src/`, root);
    for (const name of readdirSync(dir).filter(name => name.endsWith('.vue'))) {
      const path = `${surface}/src/${name}`;
      const source = readFileSync(new URL(name, dir), 'utf8');
      const template = parse(source).descriptor.template;
      if (!template) continue;
      function visit(node, parentLoading = false) {
        if (node.type === 1 && ['p', 'div', 'td', 'li'].includes(node.tag) && !node.children.some(child => child.type === 1)) {
          const text = node.children.filter(child => child.type === 5).map(child => child.content.content).join(' ');
          const condition = node.props.find(prop => prop.type === 7 && ['if', 'else-if'].includes(prop.name))?.exp?.content ?? '';
          const classes = node.props.find(prop => prop.type === 6 && prop.name === 'class')?.value?.content ?? '';
          const loading = parentLoading || node.props.some(prop => prop.type === 7 && prop.name === 'global-loading' && !prop.exp);
          const inline = (name === 'BrandDomains.vue' && node.tag === 'li') || /mobile-draw-empty/.test(classes) || /noEligibility|noBrand|noView|noRun|noMore/.test(text);
          const collection = /!\s*[\w?.]+\.length|\.length\s*===\s*0/.test(condition);
          const emptyText = /\bempty\b|emptyLabel|\.empty[A-Z]|noHistory|noResult|noGames|noOrders|noCodes|noPlayableOptions|chooseGameHistory/.test(text)
            || [...emptyKeys].some(key => text.includes(`'${key}'`) || text.includes(`"${key}"`));
          const emptyClass = /(?:^|\s)(?:empty|empty-state|empty-cases|detail-empty|nt-empty|nd-empty|notifications-empty|order-history-empty)(?:\s|$)/.test(classes);
          if (text && !loading && !inline && (collection || emptyText || emptyClass)) {
            result.push({ path, line: node.loc.start.line, classes, text });
          }
        }
        const pureLoading = parentLoading || node.props?.some(prop => prop.type === 7 && prop.name === 'global-loading' && !prop.exp);
        for (const child of node.children ?? []) visit(child, pureLoading);
      }
      visit(template.ast);
    }
  }
  return result;
}

