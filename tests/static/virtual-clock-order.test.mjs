import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'

const require = createRequire(new URL('../../user-web/package.json', import.meta.url))
const ts = require('typescript')

function lateInstallations(source, name) {
  const file = ts.createSourceFile(name, source, ts.ScriptTarget.Latest, true)
  const errors = []
  function visit(node) {
    if (ts.isCallExpression(node) && /(?:^|\.)clock\.install$|^vi\.useFakeTimers$/.test(node.expression.getText(file))) {
      let owner = node.parent
      while (owner && !ts.isFunctionLike(owner)) owner = owner.parent
      let expression = owner?.body
      if (expression && ts.isBlock(expression)) {
        const first = expression.statements[0]
        expression = first && ts.isExpressionStatement(first) ? first.expression : undefined
      }
      if (expression && ts.isAwaitExpression(expression)) expression = expression.expression
      if (expression !== node) errors.push(`${name}:${file.getLineAndCharacterOfPosition(node.getStart(file)).line + 1}`)
    }
    ts.forEachChild(node, visit)
  }
  visit(file)
  return errors
}

test('virtual-clock order check rejects installation after navigation or timer startup', () => {
  assert.deepEqual(lateInstallations('test(async ({page}) => { await page.clock.install(); await page.goto("/"); })', 'early.ts'), [])
  assert.deepEqual(lateInstallations('function mount() { vi.useFakeTimers(); startPolling(); }', 'early-unit.ts'), [])
  assert.deepEqual(lateInstallations('beforeEach(() => vi.useFakeTimers())', 'early-hook.ts'), [])
  assert.deepEqual(lateInstallations('test(async ({page}) => { await page.goto("/"); await page.clock.install(); })', 'late.ts'), ['late.ts:1'])
  assert.deepEqual(lateInstallations('test(() => { startPolling(); vi.useFakeTimers(); })', 'late-unit.ts'), ['late-unit.ts:1'])
})

test('repository virtual clocks install at the start of their test or setup function', () => {
  const errors = []
  for (const root of ['tests', 'shared/src', 'user-web/src', 'admin-web/src', 'platform-web/src']) {
    const directory = new URL(`../../${root}/`, import.meta.url)
    for (const name of readdirSync(directory, { recursive: true })) {
      if (!/\.(?:ts|mjs|js)$/.test(name)) continue
      if (root !== 'tests' && !/\.(?:test|spec)\./.test(name)) continue
      errors.push(...lateInstallations(readFileSync(new URL(name, directory), 'utf8'), `${root}/${name}`))
    }
  }
  assert.deepEqual(errors, [], 'Install clocks before navigation, component mounting or starting timers; do not extend timeouts to mask late installation.')
})
