import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { createRequire } from 'node:module';
const root = new URL('../', import.meta.url);
const require = createRequire(new URL('user-web/package.json', root));
const { parse } = require('vue/compiler-sfc');

// Settings failures/conflicts deliberately offer a text recovery action (UI spec 48).
// The same label outside a failure context is still a regular refresh candidate.
export function isRecoveryRead(label, conditions, path = '') {
  if (path === 'shared/src/DrawScheduleSettings.vue' && /t\(['"]reload['"]\)/.test(label)
    && conditions.some(condition => /^failed\s*&&\s*!pending$/.test(condition))) return true;
  return /['"](?:admin\.settings\.readAgain|security\.reread|brandOperation\.readLatest)['"]/.test(label)
    && conditions.some(condition => /(?:^|&&|\|\||\()\s*(?:(?:error|readFailed|conflict|conflictedWrite)\s*(?=$|&&|\|\||\))|[\w.]+\.phase\s*===\s*['"]conflict['"])/.test(condition));
}

function filesUnder(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => entry.isDirectory()
    ? filesUnder(new URL(entry.name + '/', dir)) : [new URL(entry.name, dir)]);
}
function addRefreshKeys(source, keys) {
  for (const match of source.matchAll(/(?:['"]([^'"]+)['"]|\b([A-Za-z_$][\w$]*)):\s*['"]([^'"\n]+)['"]/g)) {
    if (/^(刷新|重新加载|手动刷新|Refresh\b|Reload\b|Explicitly reload)/i.test(match[3])) keys.add(match[1] ?? match[2]);
  }
}

export function refreshButtonCandidates() {
  const surfaceKeys = new Map();
  for (const surface of ['user-web', 'admin-web', 'platform-web', 'agent-web']) {
    const keys = new Set();
    surfaceKeys.set(surface, keys);
    const dir = new URL(`${surface}/src/`, root);
    for (const file of readdirSync(dir).filter(file => /(?:i18n.*|configured-rule-catalog)\.ts$/.test(file))) {
      addRefreshKeys(readFileSync(new URL(file, dir), 'utf8'), keys);
    }
  }
  const result = [];
  for (const surface of ['user-web', 'admin-web', 'platform-web', 'agent-web', 'shared']) {
    const dir = new URL(`${surface}/src/`, root);
    for (const file of filesUnder(dir).filter(file => file.pathname.endsWith('.vue'))) {
      const path = file.pathname.slice(root.pathname.length), source = readFileSync(file, 'utf8');
      const localKeys = new Set(surfaceKeys.get(surface) ?? []);
      // Shared business catalogs also live in modules such as draw-schedule.ts.
      // Resolve each component's own imports so generic keys cannot leak between catalogs.
      for (const match of source.matchAll(/from\s+['"](\.[^'"]+)['"]/g)) {
        const imported = new URL(match[1].endsWith('.ts') ? match[1] : match[1] + '.ts', file);
        if (existsSync(imported)) addRefreshKeys(readFileSync(imported, 'utf8'), localKeys);
      }
      const descriptor = parse(source).descriptor;
      if (!descriptor.template) continue;
      function visit(node, conditions = [], ancestors = []) {
        const condition = node.props?.find(prop => prop.type === 7 && ['if', 'else-if'].includes(prop.name))?.exp?.content;
        const context = condition ? [...conditions, condition] : conditions;
        if (node.type === 1 && node.tag === 'button') {
          const binding = name => node.props.find(prop => prop.type === 7 && prop.arg?.content === name)?.exp?.content;
          const handler = binding('click') ?? '';
          const text = node.children.filter(child => child.type === 5).map(child => child.content.content).join(' ');
          const label = binding('aria-label') ?? text;
          const translated = [...localKeys].some(key => label.includes(`'${key}'`) || label.includes(`"${key}"`) || text.includes(`'${key}'`) || text.includes(`"${key}"`));
          const refresh = translated || /\b[\w.()]+\.refresh\b|\bt\(['"]refresh['"]\)/.test(text);
          // Do not disguise a discard, confirmation, retry or initial history
          // navigation action as a refresh control.
          const otherAction = node.props.some(prop => prop.type === 7 && prop.name === 'write-guard') || /discard|confirm|cancelReview|refreshJoinLink/.test(handler)
            || /policyHistoryShown|historyOpen/.test(text);
          if (refresh && !otherAction && !isRecoveryRead(label, context, path)) result.push({ path, source, node, label, handler, line: node.loc.start.line, ancestors });
        }
        for (const child of node.children ?? []) visit(child, context, [...ancestors, node]);
      }
      visit(descriptor.template.ast);
    }
  }
  return result;
}
