import { readAdminEnvelope } from './admin-api'
import { AdminApiError, createAdminApi } from './admin-api';

const api = createAdminApi();

export async function selectAdminAssociation(brandId: string, username: string, resource: 'agent' | 'join_code' | 'commission' | 'report_commission' | 'report_attribution' | 'reward', kind: 'member' | 'agent' = 'member', action: 'view' | 'write' | 'grant' = 'write') {
  const normalized = username.trim().toLowerCase();
  const query = new URLSearchParams({kind,resource,action,username:normalized});
  const response = await fetch(`/api/v1/admin/association-selection?${query}`, {credentials:'same-origin',headers:{Accept:'application/json','X-Brand-ID':brandId}});
  const envelope = await readAdminEnvelope(response);
  const value: unknown = envelope?.data;
  if (!envelope?.success || !value || typeof value !== 'object') throw new AdminApiError('Invalid association selection',502,'INVALID_RESPONSE');
  const item = value as {id:string;member_id:string;brand_id:string;username:string};
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
  if (!uuid.test(item.id) || !uuid.test(item.member_id) || item.brand_id!==brandId || item.username!==normalized) throw new AdminApiError('Invalid association selection',502,'INVALID_RESPONSE');
  return item;
}

export async function lookupAdminMember(brandId: string, username: string) {
  const items = (await api.users(brandId, 2, 0, username)).items;
  if (items.length === 0) throw new AdminApiError("No member matches this username", 404, "MEMBER_NOT_FOUND");
  if (items.length !== 1) throw new AdminApiError("More than one member matches this username", 409, "MEMBER_NOT_UNIQUE");
  return items[0];
}
