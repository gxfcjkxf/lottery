import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { createRequire } from 'node:module'
import { redundantAnnotation } from '../ui-copy-contract.mjs'
const require = createRequire(new URL('../../shared/package.json', import.meta.url))
const ts = require('typescript')
const { parse } = require('vue/compiler-sfc')
function files(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => entry.isDirectory() ? files(join(dir, entry.name)) : /\.(ts|vue)$/.test(entry.name) && !entry.name.includes('.test.') ? [join(dir, entry.name)] : [])
}
function violations(source, file = 'catalog.ts') {
  const tree = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true), issues = []
  function visit(node) {
    if ((ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) && redundantAnnotation.test(node.text)) issues.push(node.text)
    ts.forEachChild(node, visit)
  }
  visit(tree)
  return issues
}
test('four apps and shared copy omit redundant parenthetical instructions', () => {
  const issues = []
  for (const dir of ['platform-web/src', 'admin-web/src', 'agent-web/src', 'user-web/src', 'shared/src']) for (const file of files(dir)) {
    const source = readFileSync(file, 'utf8')
    const scripts = file.endsWith('.vue') ? (() => { const { descriptor } = parse(source); return [descriptor.script?.content, descriptor.scriptSetup?.content].filter(Boolean) })() : [source]
    for (const script of scripts) for (const copy of violations(script)) issues.push(`${file}: ${copy}`)
  }
  assert.deepEqual(issues, [])
})
test('copy check reads escaped and multiline catalog values and preserves necessary meanings', () => {
  assert.equal(violations(String.raw`const copy = {name: '\u663e\u793a\u540d\u79f0\uff08\u6700\u591a 120 \u5b57\u7b26\uff09',
    member: "Member account (optional)", config: 'Full configuration (details)', numbers: 'Numbers (comma-separated)'};`).length, 4)
  for (const copy of ['开奖前开盘（秒）', 'To (exclusive)', '周期就绪（证据过期）', 'Number separator (blank for array or one value per element)', 'Position (1 based, blank for full scope)']) assert.equal(redundantAnnotation.test(copy), false, copy)
})
