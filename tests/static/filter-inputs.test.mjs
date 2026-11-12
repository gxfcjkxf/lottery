import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
function vueFiles(dir) {
  return readdirSync(dir, {withFileTypes:true}).flatMap(entry => entry.isDirectory() ? vueFiles(join(dir,entry.name)) : entry.name.endsWith('.vue') ? [join(dir,entry.name)] : [])
}
test('search inputs across all surfaces use the shared clear control', () => {
  for (const root of ['user-web/src','admin-web/src','platform-web/src','agent-web/src','shared/src']) {
    for (const path of vueFiles(root)) {
      if (path === 'shared/src/ClearableInput.vue') continue
      assert.doesNotMatch(readFileSync(path,'utf8'), /<input\b[^>]*type="search"/, path)
    }
  }
})
test('account search has no duplicate clear action or no-match message', () => {
  const source=readFileSync('shared/src/FilterAssociationSearch.vue','utf8')
  assert.doesNotMatch(source,/No matching accounts|未找到匹配账号|t\('clear'\)/)
  assert.match(source,/<SelectControl\b[^>]*:search=/)
  assert.match(readFileSync('shared/src/SelectControl.vue','utf8'),/<ClearableInput\b/)
  assert.doesNotMatch(source,/association-search-results/)
})
