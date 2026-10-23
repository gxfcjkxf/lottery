import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8')
test('new lottery pages follow shared pagination, visual model and configured time rules', () => {
  const source = read('shared/src/PlatformLottery.vue')
  const header = source.match(/<div class="platform-lottery-toolbar">[\s\S]*?<\/div>/)?.[0]
  assert.ok(header, 'shared lottery toolbar must be present')
  assert.match(header, /platform-lottery-filter/)
  assert.match(header, /t\('create'\)/)
  assert.doesNotMatch(header, /@click="refresh"/)
  assert.match(source, /wallet-pagination[^\n]*@click="withGlobalLoading\(refresh\)"/)
  assert.match(source, /<NumberModelEditor/)
  assert.doesNotMatch(source, /type="datetime-local"|t\('inputTimezone'|<pre>\{\{ JSON.stringify\(detail.game.model/)
  assert.match(source, /<PlatformDateInput[^\n]*:display-settings="displaySettings!"/)
  assert.match(read('shared/src/PlatformDateInput.vue'), /props\.displaySettings\.date_format/)
  assert.match(source, /formatDisplayDateTime\(value, props.locale\)/)
  assert.match(read('platform-web/src/App.vue'), /<PlatformLottery[^\n]*:display-settings="displaySettings"/)
  const archives = read('admin-web/src/ReportArchivesManagement.vue')
  assert.doesNotMatch(archives.split('\n').find(line => line.includes('<header>')), /archive-refresh/)
  assert.match(archives.split('\n').find(line => line.includes('archive-list"')), /archive-refresh/)
})
