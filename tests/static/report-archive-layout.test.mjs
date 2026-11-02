import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8')
test('archive details reuse business reports and platform details preserve the list in a dialog', () => {
  for (const path of ['platform-web/src/ArchivesPanel.vue']) {
    const source = read(path)
    assert.match(source, /@lottery\/shared\/ArchiveReport.vue/)
    assert.match(source, /@close="backToList"/)
  }
  const platform = read('platform-web/src/ArchivesPanel.vue')
  assert.match(platform, /<DetailModal v-if="selectedId"/)
  assert.doesNotMatch(platform, /<section v-if="!selectedId"/)
  const report = read('shared/src/ArchiveReport.vue')
  assert.doesNotMatch(report, /<details[^>]*class="archive-report-technical"/)
  assert.doesNotMatch(report, /payload_sha256|audit_log_id|snapshot\.format_version/)
  assert.doesNotMatch(report, /<details[^>]*\sopen(?:\s|=|>)/)
  assert.match(report, /grid-template-columns: minmax\(0, 1fr\) minmax\(0, 1fr\)/)
  assert.match(report, /withdrawalRows/)
  assert.match(report, /repeat\(auto-fit, minmax\(min\(100%, 240px\), 1fr\)\)/)
  assert.match(report, /columns: 300px/)
  assert.match(report, /class="[^"]*\barchive-report-table\b[^"]*"/)
  assert.doesNotMatch(report, /archive-report-withdrawals|repeat\([23], minmax/)
  assert.match(report, /BigInt\(input\)\.toLocaleString/)
})
