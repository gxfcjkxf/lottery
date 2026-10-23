import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8')
test('archive details are inline business reports on both administrative surfaces', () => {
  for (const path of ['platform-web/src/ArchivesPanel.vue', 'admin-web/src/ReportArchivesManagement.vue']) {
    const source = read(path)
    assert.match(source, /@lottery\/shared\/ArchiveReport.vue/)
    assert.doesNotMatch(source, /DetailModal/)
    assert.match(source, /@click="backToList"/)
  }
  const report = read('shared/src/ArchiveReport.vue')
  assert.doesNotMatch(report, /<details[^>]*class="archive-report-technical"/)
  assert.doesNotMatch(report, /payload_sha256|audit_log_id|snapshot\.format_version/)
  assert.doesNotMatch(report, /<details[^>]*\sopen(?:\s|=|>)/)
  assert.match(report, /grid-template-columns: minmax\(0, 1fr\) minmax\(0, 1fr\)/)
  assert.match(report, /withdrawalRows/)
  assert.match(report, /repeat\(auto-fit, minmax\(min\(100%, 160px\), 1fr\)\)/)
  assert.match(report, /columns: 300px/)
  assert.match(report, /class="archive-report-table"/)
  assert.doesNotMatch(report, /archive-report-withdrawals|repeat\([23], minmax/)
  assert.match(report, /BigInt\(input\)\.toLocaleString/)
})
