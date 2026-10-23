import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
test('all four surfaces present business details instead of raw technical payloads', () => {
  for (const dir of ['user-web/src', 'agent-web/src', 'admin-web/src', 'platform-web/src', 'shared/src']) {
    for (const name of readdirSync(new URL(`../../${dir}`, import.meta.url)).filter(name => name.endsWith('.vue'))) {
      const source = readFileSync(new URL(`../../${dir}/${name}`, import.meta.url), 'utf8')
      const template = source.match(/<template>([\s\S]*?)<\/template>\s*<style/)?.[1] ?? source.slice(source.indexOf('<template>'))
      assert.doesNotMatch(template, /<pre\b|\{\{\s*(?:JSON\.stringify|prettyTrace|pretty|stringify|json|configJson|reviewSnapshot)\s*\(/, `${dir}/${name}: raw payload rendering`)
      assert.doesNotMatch(template, /\{\{\s*(?:(?:pending|review|dialog|confirmation|intent|unknownIntent)\.key|[\w.]+\.(?:request_id|[a-z_]*audit_log_id|[a-z_]*_hash|payload_sha256))\s*\}\}/, `${dir}/${name}: internal identifier rendering`)
    }
  }
})
