import { createPlatformApi, PlatformApiError } from './platform-api'

const api = createPlatformApi()

export async function lookupPlatformMember(brandId: string, username: string) {
  const { items } = await api.users(brandId, 2, 0, username)
  if (items.length === 0) throw new PlatformApiError('No member matches this username', 404, 'MEMBER_NOT_FOUND')
  if (items.length !== 1) throw new PlatformApiError('More than one member matches this username', 409, 'MEMBER_NOT_UNIQUE')
  return items[0]
}
