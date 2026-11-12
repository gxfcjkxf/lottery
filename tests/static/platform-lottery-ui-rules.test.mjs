import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8')
test('new lottery pages follow shared pagination, visual model and configured time rules', () => {
  const source = read('shared/src/PlatformLottery.vue')
  const filters = source.match(/<template v-if="surface !== 'user'" #filters>[\s\S]*?<\/form><\/template>/)?.[0]
  assert.ok(filters, 'management catalogue uses the shared filters slot')
  assert.match(filters, /platform-lottery-filter/)
  assert.match(filters, /t\('filterSearch'\)/)
  assert.doesNotMatch(filters, /@click="(?:withGlobalLoading\()?refresh/)
  assert.match(source, /<template v-if="canManagePlatform" #actions>[\s\S]*?t\('create'\)/)
  assert.match(source, /<Teleport v-if="surface === 'user'"[\s\S]*?@click="withGlobalLoading\(refresh\)"/)
  assert.match(source, /<NumberModelEditor/)
  assert.doesNotMatch(source, /type="datetime-local"|t\('inputTimezone'|<pre>\{\{ JSON.stringify\(detail.game.model/)
  assert.match(source, /<PlatformDateInput[^\n]*:display-settings="displaySettings!"/)
  assert.match(read('shared/src/PlatformDateInput.vue'), /props\.displaySettings\.date_format/)
  assert.match(source, /formatDisplayDateTime\(value, props.locale\)/)
  assert.match(read('platform-web/src/App.vue'), /<PlatformLottery[^\n]*:display-settings="displaySettings"/)
  const archives = read('platform-web/src/DataRetentionPanel.vue')
  assert.match(archives, /<ManagementTable/)
  assert.match(archives, /v-refresh-button/)
  assert.match(archives, /<TablePagination/)
  assert.doesNotMatch(archives, /<input[^>]*type="(?:date|time|datetime-local)"/)
})
