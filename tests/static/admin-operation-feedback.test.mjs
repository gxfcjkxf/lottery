import { readFileSync, readdirSync } from 'node:fs'
import assert from 'node:assert/strict'
import { test } from 'node:test'
const read = path => readFileSync(path, 'utf8')

test('brand operations use the shared bottom-right Toast host without legacy positioning', () => {
 const app = read('admin-web/src/App.vue'), css = read('admin-web/src/style.css')
 assert.match(app, /<OperationToastHost\s*\/>/)
 assert.doesNotMatch(css, /(?:^|\n)\s*\.toast\b|@keyframes\s+toastin/)
 assert.match(read('admin-web/src/OperationToastHost.vue'), /@lottery\/shared\/OperationToastHost\.vue/)
 const host = read('shared/src/OperationToastHost.vue')
 assert.match(host, /import Toast from '\.\/Toast\.vue'/)
 assert.match(host, /right:20px; bottom:max\(20px/)
 assert.match(host, /activeModalDialog/)
 for (const name of readdirSync('admin-web/src').filter(name => name.endsWith('.vue') && name !== 'OperationToastHost.vue')) {
  const source = read(`admin-web/src/${name}`)
  assert.doesNotMatch(source, /class="toast"|<Toast\b/, name)
  assert.doesNotMatch(source, /<(?:p|div)\b[^>]*v-if="notice"[^>]*>/, name)
 }
 assert.match(app, /@credited="rechargeMember = null; withGlobalLoading\(loadMembers\)"/)
 assert.match(app, /@created="memberCreateOpen = false; withGlobalLoading\(loadMembers\)"/)
})

test('trend selection uses primary button tokens rather than losing its background to secondary styles', () => {
 const overview = read('shared/src/ManagementOverview.vue')
 assert.match(overview, /metric === option\.key \? 'button-primary' : 'button-secondary'/)
 assert.match(overview, /:aria-pressed="metric === option\.key"/)
})

test('all four entry points host operation feedback and have no private floating Toast', () => {
 for (const surface of ['user','admin','platform','agent']) {
  assert.match(read(`${surface}-web/src/App.vue`), /<OperationToastHost/)
  for (const name of readdirSync(`${surface}-web/src`).filter(name => name.endsWith('.vue') && name !== 'OperationToastHost.vue')) {
   assert.doesNotMatch(read(`${surface}-web/src/${name}`), /class="toast"|<Toast\b|configured-bet-notice/, `${surface}/${name}`)
  }
 }
})
