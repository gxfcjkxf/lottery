import { readFileSync, readdirSync } from 'node:fs';
import { createRequire } from 'node:module';
const root = new URL('../', import.meta.url);
const require = createRequire(new URL('user-web/package.json', root));
const { parse } = require('vue/compiler-sfc');

// Settings failures/conflicts deliberately offer a text recovery action (UI spec 48).
// The same label outside a failure context is still a regular refresh candidate.
export function isRecoveryRead(label, conditions) {
  return /['"](?:admin\.settings\.readAgain|security\.reread|brandOperation\.readLatest)['"]/.test(label)
    && conditions.some(condition => /(?:^|&&|\|\||\()\s*(?:(?:error|readFailed|conflict|conflictedWrite)\s*(?=$|&&|\|\||\))|[\w.]+\.phase\s*===\s*['"]conflict['"])/.test(condition));
}

export function refreshButtonCandidates() {
  const keys = new Set();
  for (const surface of ['user-web', 'admin-web', 'platform-web']) {
    const dir = new URL(`${surface}/src/`, root);
    for (const file of readdirSync(dir).filter(file => /i18n.*\.ts$/.test(file))) {
      for (const match of readFileSync(new URL(file, dir), 'utf8').matchAll(/['"]([^'"]+)['"]:\s*['"]([^'"\n]+)['"]/g)) {
        if (/^(刷新|重新加载|手动刷新|Refresh\b|Reload\b|Explicitly reload)/i.test(match[2])) keys.add(match[1]);
      }
    }
  }
  const result = [];
  for (const surface of ['user-web', 'admin-web', 'platform-web', 'shared']) {
    const dir = new URL(`${surface}/src/`, root);
    for (const file of readdirSync(dir).filter(file => file.endsWith('.vue'))) {
      const path = `${surface}/src/${file}`, source = readFileSync(new URL(file, dir), 'utf8');
      const descriptor = parse(source).descriptor;
      if (!descriptor.template) continue;
      function visit(node, conditions = []) {
        const condition = node.props?.find(prop => prop.type === 7 && ['if', 'else-if'].includes(prop.name))?.exp?.content;
        const context = condition ? [...conditions, condition] : conditions;
        if (node.type === 1 && node.tag === 'button') {
          const binding = name => node.props.find(prop => prop.type === 7 && prop.arg?.content === name)?.exp?.content;
          const handler = binding('click') ?? '';
          const text = node.children.filter(child => child.type === 5).map(child => child.content.content).join(' ');
          const label = binding('aria-label') ?? text;
          const translated = [...keys].some(key => label.includes(`'${key}'`) || label.includes(`"${key}"`) || text.includes(`'${key}'`) || text.includes(`"${key}"`));
          const refresh = translated || /\b[\w.()]+\.refresh\b|\bt\(['"]refresh['"]\)/.test(text);
          // Do not disguise a discard, confirmation, retry or initial history
          // navigation action as a refresh control.
          const otherAction = /discard|confirm|cancelReview|refreshJoinLink/.test(handler)
            || /policyHistoryShown|historyOpen/.test(text);
          if (refresh && !otherAction && !isRecoveryRead(label, context)) result.push({ path, source, node, label, handler, line: node.loc.start.line });
        }
        for (const child of node.children ?? []) visit(child, context);
      }
      visit(descriptor.template.ast);
    }
  }
  return result;
}
