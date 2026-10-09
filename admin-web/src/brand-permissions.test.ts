import { describe, expect, it } from 'vitest';
import { brandPermissionSet } from './brand-permissions';

describe('brand staff permission boundary', () => {
  const staff = { super_admin: false, brand_ids: ['a', 'b'], permissions_by_brand: { a: ['wallet.view.brand', 'wallet.view.brand'], b: ['user.view.brand'] } };
  it('deduplicates mapped grants while isolating each authorized brand', () => {
    expect([...brandPermissionSet(staff, 'a')]).toEqual(['wallet.view.brand']);
    expect([...brandPermissionSet(staff, 'b')]).toEqual(['user.view.brand']);
    expect([...brandPermissionSet(staff, 'c')]).toEqual([]);
  });
  it('rejects platform actors and missing mappings without flat-grant fallback', () => {
    expect([...brandPermissionSet({ ...staff, super_admin: true }, 'a')]).toEqual([]);
    expect([...brandPermissionSet({ super_admin: false, brand_ids: ['a'], permissions: ['wallet.view.brand'] } as never, 'a')]).toEqual([]);
    expect([...brandPermissionSet({ ...staff, brand_ids: [] }, 'a')]).toEqual([]);
  });
});
