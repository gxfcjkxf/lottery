import { test } from 'node:test'
import assert from 'node:assert/strict'
import { loadingActionCandidates, loadingActionCandidatesInSource } from '../../scripts/loading-action-audit.mjs'
const audit = (script, event = '@keydown="navigate"') => loadingActionCandidatesInSource('fixture.vue', `<script setup>${script}</script><template><button ${event}>Action</button></template>`)

test('local menu handlers can await nextTick through declared helpers in any order', () => {
  for (const helper of ['async function show() { await nextTick() }', 'const show = async () => { await nextTick() }']) {
    assert.deepEqual(audit(`async function navigate() { await show() } ${helper}`), [])
  }
  assert.deepEqual(loadingActionCandidates('shared/src/SelectControl.vue'), [])
})
test('a data request stays audited when it delegates through local helpers', () => {
  assert.equal(audit('async function navigate() { await show() } async function show() { await api.read() }').length, 1)
  assert.equal(audit('async function navigate() { await nextTick(); await api.read() }').length, 1)
  assert.equal(audit('const navigate = async () => { await api.read() }').length, 1)
})
test('unknown waits and recursive call chains are not treated as DOM-only updates', () => {
  assert.equal(audit('async function navigate() { await unknown() }').length, 1)
  assert.equal(audit('async function navigate() { await show() } async function show() { await navigate() }').length, 1)
})
test('submit and refresh actions still require foreground tracking', () => {
  const local = 'async function navigate() { await nextTick() }'
  assert.equal(audit(local, '@submit="navigate"').length, 1)
  assert.equal(audit(local, 'v-refresh-button="label" @click="navigate"').length, 1)
  assert.deepEqual(audit('async function navigate() { await api.read() }', '@click="withGlobalLoading(navigate)"'), [])
})
