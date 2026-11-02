import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
const files = directory => readdirSync(directory, {withFileTypes:true}).flatMap(entry => entry.isDirectory() ? files(`${directory}/${entry.name}`) : [`${directory}/${entry.name}`])
test('four applications and shared components only use the unified selection control', () => {
 for (const root of ['user-web','agent-web','admin-web','platform-web','shared']) for (const file of files(`${root}/src`).filter(file=>file.endsWith('.vue'))) {
  const source=readFileSync(file,'utf8')
  assert.doesNotMatch(source, /<select\b/i, file)
  assert.doesNotMatch(source, /HTMLSelectElement/, file)
 }
})

test('native select styles and event adapters cannot remain after migration', () => {
 for (const root of ['user-web','agent-web','admin-web','platform-web','shared']) for (const file of files(`${root}/src`).filter(file=>/\.(vue|css|ts)$/.test(file) && !file.endsWith('.test.ts'))) {
  const source=readFileSync(file,'utf8')
  assert.doesNotMatch(source, /HTMLSelectElement|['"]SELECT['"]/, file)
  const styles=file.endsWith('.css') ? [source] : [...source.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/g)].map(match=>match[1])
  for (const css of styles) assert.doesNotMatch(css.replace(/\/\*[\s\S]*?\*\//g,''), /(?:^|[\s>+~,(])select(?=$|[\s>+~.#:[,){])/i, file)
 }
})
