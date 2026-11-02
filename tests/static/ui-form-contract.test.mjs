import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { createRequire } from 'node:module'
const require = createRequire(new URL('../../user-web/package.json', import.meta.url))
const { parse } = require('vue/compiler-sfc')
function vueFiles(dir) { return readdirSync(dir, { withFileTypes: true }).flatMap(entry => entry.isDirectory() ? vueFiles(join(dir, entry.name)) : entry.name.endsWith('.vue') ? [join(dir, entry.name)] : []) }
function formIssues(source) {
  const { descriptor } = parse(source), issues = []
  function walk(node) {
    if (node.tag === 'form') {
      const classes = node.props.find(prop => prop.name === 'class')?.value?.content.split(/\s+/) ?? []
      if (!classes.some(name => ['ui-form', 'ui-filter-bar', 'login-entry__form', 'auth-form'].includes(name))) issues.push(node.loc.start.line)
    }
    for (const child of node.children ?? []) walk(child)
  }
  if (descriptor.template) walk(descriptor.template.ast)
  return issues
}
test('business forms in all four apps use the shared form or filter contract', () => {
  const missing = []
  for (const dir of ['admin-web/src', 'platform-web/src', 'agent-web/src', 'user-web/src', 'shared/src']) for (const path of vueFiles(dir)) {
    for (const line of formIssues(readFileSync(path, 'utf8'))) missing.push(`${path}:${line}`)
  }
  assert.deepEqual(missing, [])
})
test('form checks inspect actual attributes and ignore commented markup', () => {
  assert.deepEqual(formIssues('<template><!-- <form> --><form class="ui-form" /></template>'), [])
  assert.equal(formIssues('<template><form data-class="ui-form" /></template>').length, 1)
})
