import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { test } from 'node:test'

const rawError = /\b(?:cause|error|err|raw|e|problem|failure|exception)\??\.message\b/
const codeInPrompt = /(?:error\.value\s*=.*(?:cause|error)\.code|\$\{(?:cause|error)\.code\})/

test('error presentation guard rejects raw interface messages and appended codes', () => {
  assert.ok(rawError.test('error.value = cause.message'))
  assert.ok(rawError.test('return e?.message'))
  assert.ok(codeInPrompt.test('error.value = `${cause.code}: failed`'))
  assert.equal(rawError.test('error.value = publicErrorMessage(cause)'), false)
})

test('three surfaces do not render raw interface errors or technical unknown-code prompts', () => {
  const failures = []
  // These editors only parse local rule input; they do not call an API.
  const localEditors = new Set(['RuleDefinitionEditor.vue', 'RuleModelEditor.vue', 'RuleCasesEditor.vue'])
  for (const root of ['user-web/src', 'admin-web/src', 'platform-web/src']) {
    const directory = new URL(`../../${root}/`, import.meta.url)
    for (const name of readdirSync(directory)) {
      if (!name.endsWith('.vue')) continue
      const source = readFileSync(new URL(name, directory), 'utf8')
      const script = source.split('</script>')[0]
      for (const [index, line] of script.split('\n').entries()) {
        const localDrawPreview = name === 'DrawManagement.vue' && line.includes("t('admin.copy.32c6ed4c')")
        if (rawError.test(line) && !localEditors.has(name) && !localDrawPreview || codeInPrompt.test(line)) failures.push(`${root}/${name}:${index + 1}`)
        assert.doesNotMatch(line, /\$\{publicErrorMessage\(/, 'Render bilingual messages with displayError, not string interpolation')
      }
      assert.doesNotMatch(source, /未识别的接口错误|Unrecognized API error/)
    }
  }
  assert.deepEqual(failures, [])
})

test('unknown platform and shared lottery errors use the shared public presentation', () => {
  const platform = readFileSync(new URL('../../platform-web/src/platform-error-message.ts', import.meta.url), 'utf8')
  const lottery = readFileSync(new URL('../../shared/src/PlatformLottery.vue', import.meta.url), 'utf8')
  assert.match(platform, /publicApiErrorText\(cause, locale\)/)
  assert.match(lottery, /i18n\.errorText\(/)
  assert.doesNotMatch(lottery, /\$\{error\.value\}/)
})
