import { readdirSync, readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { permissionLabel } from '@lottery/shared'

const migrationDirectory = new URL('../../backend/migrations/', import.meta.url)
const bootstrapSource = readFileSync(new URL('../../backend/cmd/platform/admin.go', import.meta.url), 'utf8')
const registeredPermissions = [...new Set(
  [...readdirSync(migrationDirectory)
    .filter(name => name.endsWith('.sql'))
    .flatMap(name => {
      const source = readFileSync(new URL(name, migrationDirectory), 'utf8')
      const permissions: string[] = []
      for (const match of source.matchAll(/'([a-z][a-z0-9_]*\.[a-z][a-z0-9_]*\.(?:brand|platform))'/g)) permissions.push(match[1])
      return permissions
    }), ...Array.from(bootstrapSource.matchAll(/"([a-z][a-z0-9_]*\.[a-z][a-z0-9_]*\.(?:brand|platform))"/g), match => match[1])],
)].sort()

describe('permission labels', () => {
  it('translates every migration and bootstrap permission in both supported locales', () => {
    expect(registeredPermissions.length).toBeGreaterThan(0)
    for (const permission of registeredPermissions) {
      const zh = permissionLabel(permission, 'zh-CN')
      const en = permissionLabel(permission, 'en')
      expect(zh, permission).not.toBe(permission)
      expect(en, permission).not.toBe(permission)
      expect(en, permission).not.toMatch(/[\u3400-\u9fff]/u)
    }
  })
  it('names the member session revocation permission without exposing its code', () => {
    expect(permissionLabel('user.kick.brand', 'zh-CN')).toBe('会员 · 强制下线')
    expect(permissionLabel('user.kick.brand', 'en')).toBe('Members · Force sign out')
  })
})
