import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { permissionLabel } from './permission-labels'

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8')
const bootstrap = read('../../backend/cmd/platform/admin.go')
const keys = new Set([...bootstrap.matchAll(/"([a-z_]+\.[a-z_]+\.(?:brand|platform))"/g)].map(match => match[1]))

describe('administrative permission catalog', () => {
  it('covers every documented permission with an explicit initialization policy', () => {
    const contract = read('../../docs/openapi.json')
    for (const match of contract.matchAll(/"([a-z_]+\.[a-z_]+\.(?:brand|platform))"/g)) {
      expect(keys.has(match[1]), `Missing bootstrap permission: ${match[1]}`).toBe(true)
    }
  })

  it('translates the complete bootstrap and baseline catalogs in both languages', () => {
    const baseline = read('../../backend/migrations/0001_baseline.up.sql')
    const catalog = new Set([...keys, ...[...baseline.matchAll(/'([a-z_]+\.[a-z_]+\.(?:brand|platform))'/g)].map(match => match[1])])
    for (const key of catalog) {
      for (const locale of ['en', 'zh-CN'] as const) {
        expect(permissionLabel(key, locale), `${key}: ${locale}`).toBeTruthy()
      }
    }
  })
})
