import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const rootPackage = JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8'))
const userReview = readFileSync(new URL('../../user-web/vite.review.config.ts', import.meta.url), 'utf8')
const adminReview = readFileSync(new URL('../../admin-web/vite.review.config.ts', import.meta.url), 'utf8')
const userDev = readFileSync(new URL('../../user-web/vite.config.ts', import.meta.url), 'utf8')
const adminDev = readFileSync(new URL('../../admin-web/vite.config.ts', import.meta.url), 'utf8')

test('review Vite configs extend normal configs and pin loopback ports and API proxy', () => {
  for (const [config, port] of [[userReview, 5183], [adminReview, 5184]]) {
    assert.match(config, /mergeConfig\(config,/)
    assert.match(config, /host: '127\.0\.0\.1'/)
    assert.match(config, new RegExp(`port: ${port}`))
    assert.match(config, /strictPort: true/)
    assert.match(config, /target: 'http:\/\/127\.0\.0\.1:8085'/)
    assert.match(config, /changeOrigin: false/)
    assert.doesNotMatch(config, /process\.env|import\.meta\.env|localhost|0\.0\.0\.0/)
  }
})

test('root review scripts select the matching review config while normal dev configs stay unchanged', () => {
  assert.equal(rootPackage.scripts['review:user'], 'pnpm --filter @lottery/user-web exec vite --config vite.review.config.ts')
  assert.equal(rootPackage.scripts['review:admin'], 'pnpm --filter @lottery/admin-web exec vite --config vite.review.config.ts')
  assert.match(userDev, /port: 5173/)
  assert.match(adminDev, /port: 5174/)
  assert.match(userDev, /target: 'http:\/\/localhost:8080'/)
  assert.match(adminDev, /target: 'http:\/\/localhost:8080'/)
  assert.doesNotMatch(userDev + adminDev, /5183|5184|127\.0\.0\.1:8085/)
})
